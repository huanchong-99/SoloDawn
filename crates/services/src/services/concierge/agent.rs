//! ConciergeAgent: LLM-powered session-scoped assistant.

use std::{collections::HashMap, sync::Arc};

use anyhow::{Context, Result};
use db::models::concierge::{ConciergeMessage, ConciergeSession};
use sqlx::SqlitePool;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use tracing;

use super::{
    prompt::concierge_system_prompt,
    sync::{ConciergeBroadcaster, ConciergeEvent},
    tools::{execute_tool, parse_tool_call},
};
use crate::services::orchestrator::{
    config::OrchestratorConfig,
    llm::{LLMClient, create_llm_client},
    message_bus::{MessageBusBackend, SharedMessageBus},
    types::LLMMessage,
};

/// Maximum tool-call loop iterations per user message to prevent infinite loops.
const MAX_TOOL_ITERATIONS: usize = 5;

/// Maximum conversation history sent to LLM (sliding window).
const MAX_HISTORY_MESSAGES: usize = 40;

/// The Concierge Agent processes user messages, calls LLM with tool definitions,
/// executes tools, and broadcasts results across all bound channels.
pub struct ConciergeAgent {
    pool: SqlitePool,
    broadcaster: Arc<ConciergeBroadcaster>,
    shared_config: Option<Arc<tokio::sync::RwLock<crate::services::config::Config>>>,
    message_bus: Option<SharedMessageBus>,
    /// Cancellation tokens for active notification watchers, keyed by
    /// `"{session_id}:{workflow_id}"`. Cancelled when the session is cleaned up.
    watcher_tokens: Arc<Mutex<HashMap<String, CancellationToken>>>,
}

impl ConciergeAgent {
    pub fn new(pool: SqlitePool, broadcaster: Arc<ConciergeBroadcaster>) -> Self {
        Self {
            pool,
            broadcaster,
            shared_config: None,
            message_bus: None,
            watcher_tokens: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// Cancel all active notification watchers for a given session.
    ///
    /// Should be called when a session is deleted or disconnected.
    pub async fn cancel_watchers_for_session(&self, session_id: &str) {
        let mut tokens = self.watcher_tokens.lock().await;
        let keys_to_remove: Vec<String> = tokens
            .keys()
            .filter(|k| k.starts_with(&format!("{session_id}:")))
            .cloned()
            .collect();
        for key in keys_to_remove {
            if let Some(token) = tokens.remove(&key) {
                token.cancel();
            }
        }
    }

    /// Set the message bus for subscribing to workflow events.
    pub fn set_message_bus(&mut self, bus: SharedMessageBus) {
        self.message_bus = Some(bus);
    }

    /// Set the shared config for reading workflow model library.
    pub fn set_shared_config(
        &mut self,
        config: Arc<tokio::sync::RwLock<crate::services::config::Config>>,
    ) {
        self.shared_config = Some(config);
    }

    /// Process an incoming user message and return the final assistant response.
    ///
    /// This is the main entry point called by both Feishu and Web UI handlers.
    pub async fn process_message(
        &self,
        session_id: &str,
        user_message: &str,
        source_provider: Option<&str>,
        source_user: Option<&str>,
    ) -> Result<String> {
        // 1. Load session
        let session = ConciergeSession::find_by_id(&self.pool, session_id)
            .await?
            .context("Concierge session not found")?;

        // 2. Save user message
        let user_msg =
            ConciergeMessage::new_user(session_id, user_message, source_provider, source_user);
        ConciergeMessage::insert(&self.pool, &user_msg).await?;

        // 3. Broadcast user message to other channels
        self.broadcaster
            .broadcast(
                session_id,
                ConciergeEvent::NewMessage {
                    message: user_msg.clone(),
                },
                session.feishu_sync,
                source_provider,
            )
            .await;

        // 4. Build LLM client
        let llm_client = self.build_llm_client(&session)?;

        // 5. Tool-calling loop
        let mut final_response = String::new();
        for iteration in 0..MAX_TOOL_ITERATIONS {
            // Reload session each iteration (tools may update active_project/workflow)
            let session = ConciergeSession::find_by_id(&self.pool, session_id)
                .await?
                .context("Session not found during tool loop")?;

            // Load recent conversation history
            let history =
                ConciergeMessage::list_recent(&self.pool, session_id, MAX_HISTORY_MESSAGES).await?;
            let llm_messages = self.build_llm_messages(&session, &history);

            // Call LLM
            let response = llm_client.chat(llm_messages).await?;
            let response_text = response.content.clone();

            // Check for tool call
            if let Some(tool_call) = parse_tool_call(&response_text) {
                tracing::info!(
                    session_id = %session_id,
                    tool = %tool_call.tool,
                    iteration = iteration,
                    "Concierge executing tool"
                );

                // Save tool_call message
                let tc_msg = ConciergeMessage::new_tool_call(
                    session_id,
                    &tool_call.tool,
                    &serde_json::to_string(&tool_call).unwrap_or_default(),
                );
                let tool_call_id = tc_msg.tool_call_id.clone().unwrap_or_default();
                ConciergeMessage::insert(&self.pool, &tc_msg).await?;

                // Broadcast tool execution status
                self.broadcaster
                    .broadcast_with_toggles(
                        session_id,
                        ConciergeEvent::ToolExecuting {
                            tool: tool_call.tool.clone(),
                            status: "executing".to_string(),
                        },
                        session.feishu_sync,
                        session.sync_tools,
                        None,
                    )
                    .await;

                // Reload session from DB (tools may have updated it)
                let session = ConciergeSession::find_by_id(&self.pool, session_id)
                    .await?
                    .context("Session disappeared during tool execution")?;

                // Execute the tool
                let tool_result = self
                    .execute_tool_with_runtime(&session, &tool_call, self.shared_config.as_ref())
                    .await;

                let result_text = match &tool_result {
                    Ok(text) => text.clone(),
                    Err(e) => format!("Tool error: {e}"),
                };

                // Save tool_result message
                let tr_msg =
                    ConciergeMessage::new_tool_result(session_id, &tool_call_id, &result_text);
                ConciergeMessage::insert(&self.pool, &tr_msg).await?;

                // Broadcast tool completion
                self.broadcaster
                    .broadcast_with_toggles(
                        session_id,
                        ConciergeEvent::ToolExecuting {
                            tool: tool_call.tool.clone(),
                            status: "completed".to_string(),
                        },
                        session.feishu_sync,
                        session.sync_tools,
                        None,
                    )
                    .await;

                // Continue loop — LLM will see the tool result and decide next action
                continue;
            }

            // No tool call found. Check if the response looks incomplete
            // (ends with colon/ellipsis — LLM intended to call a tool next).
            let trimmed = response_text.trim();
            let looks_incomplete = trimmed.ends_with('：')
                || trimmed.ends_with(':')
                || trimmed.ends_with("...")
                || trimmed.ends_with("…");

            if looks_incomplete && iteration < MAX_TOOL_ITERATIONS - 1 {
                // Save as assistant message and let LLM continue in next iteration
                let partial_msg = ConciergeMessage::new_assistant(session_id, &response_text);
                ConciergeMessage::insert(&self.pool, &partial_msg).await?;
                tracing::debug!(
                    session_id = %session_id,
                    "Response looks incomplete, continuing tool loop"
                );
                continue;
            }

            // Final text response
            final_response = response_text;
            break;
        }

        if final_response.is_empty() {
            final_response =
                "I've completed the requested actions. Is there anything else?".to_string();
        }

        // 6. Reload session to pick up any mutations made by tools (e.g. feishu_sync toggle)
        let session = ConciergeSession::find_by_id(&self.pool, session_id)
            .await?
            .context("Session not found after tool loop")?;

        // 7. Save assistant response
        let assistant_msg = ConciergeMessage::new_assistant(session_id, &final_response);
        ConciergeMessage::insert(&self.pool, &assistant_msg).await?;

        // 8. Broadcast assistant response
        self.broadcaster
            .broadcast(
                session_id,
                ConciergeEvent::NewMessage {
                    message: assistant_msg,
                },
                session.feishu_sync,
                None, // assistant messages go to all channels
            )
            .await;

        // 9. Auto-name session from first message
        if session.name.is_empty() {
            let name = user_message.chars().take(50).collect::<String>();
            let _ = ConciergeSession::update_name(&self.pool, session_id, &name).await;
        }

        Ok(final_response)
    }

    /// Build LLM messages from conversation history.
    fn build_llm_messages(
        &self,
        session: &ConciergeSession,
        history: &[ConciergeMessage],
    ) -> Vec<LLMMessage> {
        let mut messages = Vec::with_capacity(history.len() + 2);

        // System prompt
        let mut system = concierge_system_prompt();

        // Inject current session context
        system.push_str("\n\n## Current Context\n");
        if let Some(ref pid) = session.active_project_id {
            system.push_str(&format!("- Active project: {pid}\n"));
        } else {
            system.push_str("- No active project\n");
        }
        if let Some(ref wid) = session.active_workflow_id {
            system.push_str(&format!("- Active workflow: {wid}\n"));
        } else {
            system.push_str("- No active workflow\n");
        }

        messages.push(LLMMessage {
            role: "system".to_string(),
            content: system,
        });

        // Conversation history
        for msg in history {
            let role = match msg.role.as_str() {
                "user" => "user",
                "assistant" => "assistant",
                "tool_call" => "assistant", // Tool calls are part of assistant turns
                "tool_result" => "user",    // Tool results feed back as user context
                "system" => "user",         // System notifications shown as context
                _ => "user",
            };
            messages.push(LLMMessage {
                role: role.to_string(),
                content: msg.content.clone(),
            });
        }

        messages
    }

    /// Build an LLM client from the session's configuration.
    fn build_llm_client(&self, session: &ConciergeSession) -> Result<Box<dyn LLMClient>> {
        let api_key = session.get_api_key()?.context(
            "No API key configured for this session. Please configure LLM settings first.",
        )?;
        let config = OrchestratorConfig::from_workflow(
            session.llm_api_type.as_deref(),
            session.llm_base_url.as_deref(),
            Some(&api_key),
            session.llm_model_id.as_deref(),
        )
        .context("Incomplete LLM configuration (need api_type, base_url, model)")?;
        create_llm_client(&config).context("Failed to create LLM client")
    }

    /// Execute a tool, handling runtime-level tools.
    async fn execute_tool_with_runtime(
        &self,
        session: &ConciergeSession,
        tool_call: &super::tools::ToolCall,
        shared_config: Option<&Arc<tokio::sync::RwLock<crate::services::config::Config>>>,
    ) -> Result<String> {
        let result = execute_tool(&self.pool, session, tool_call, shared_config).await?;

        // Runtime tools return a marker — resolve them here
        if result.starts_with("RUNTIME_TOOL:") {
            return self.handle_runtime_tool(session, &result).await;
        }

        Ok(result)
    }

    /// Handle runtime tools by calling the local HTTP API.
    ///
    /// These tools (prepare_workflow, start_workflow, send_to_orchestrator)
    /// need access to OrchestratorRuntime and TerminalCoordinator, which live
    /// in the server layer. We invoke our own REST endpoints via localhost.
    async fn handle_runtime_tool(
        &self,
        session: &ConciergeSession,
        runtime_marker: &str,
    ) -> Result<String> {
        let parts: Vec<&str> = runtime_marker.splitn(3, ':').collect();
        if parts.len() < 3 {
            return Err(anyhow::anyhow!("Invalid runtime tool marker"));
        }
        let tool_name = parts[1];
        let params: serde_json::Value = serde_json::from_str(parts[2]).unwrap_or_default();

        let port = std::env::var("BACKEND_PORT").unwrap_or_else(|_| "23456".to_string());
        let base = format!("http://127.0.0.1:{port}/api");

        // Read API token so internal HTTP calls pass the auth middleware.
        let api_token =
            utils::env_compat::var_opt_with_compat("SOLODAWN_API_TOKEN", "GITCORTEX_API_TOKEN");

        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(60))
            .build()?;

        /// Helper: attach Bearer auth header when an API token is configured.
        fn with_auth(
            builder: reqwest::RequestBuilder,
            token: Option<&String>,
        ) -> reqwest::RequestBuilder {
            match token {
                Some(t) if !t.trim().is_empty() => builder.bearer_auth(t),
                _ => builder,
            }
        }

        match tool_name {
            "send_to_orchestrator" => {
                let workflow_id = session
                    .active_workflow_id
                    .as_deref()
                    .context("No active workflow to send message to")?;
                let message = params["message"]
                    .as_str()
                    .context("Missing 'message' parameter")?;

                let resp = with_auth(
                    http.post(format!("{base}/workflows/{workflow_id}/orchestrator/chat"))
                        .json(&serde_json::json!({
                            "message": message,
                            "source": "concierge"
                        })),
                    api_token.as_ref(),
                )
                .send()
                .await?;

                if resp.status().is_success() {
                    Ok(format!(
                        "Message sent to orchestrator for workflow {workflow_id}."
                    ))
                } else {
                    let status = resp.status();
                    let body = resp.text().await.unwrap_or_default();
                    Ok(format!("Orchestrator error ({status}): {}", body.chars().take(200).collect::<String>()))
                }
            }
            "prepare_workflow" => {
                let workflow_id = params["workflow_id"]
                    .as_str()
                    .context("Missing 'workflow_id'")?;

                let resp = with_auth(
                    http.post(format!("{base}/workflows/{workflow_id}/prepare")),
                    api_token.as_ref(),
                )
                .send()
                .await?;

                if resp.status().is_success() {
                    Ok(format!(
                        "Workflow '{workflow_id}' preparation started. Terminals are being spawned."
                    ))
                } else {
                    let status = resp.status();
                    let body = resp.text().await.unwrap_or_default();
                    Ok(format!("Prepare failed ({status}): {}", body.chars().take(200).collect::<String>()))
                }
            }
            "start_workflow" => {
                let workflow_id = params["workflow_id"]
                    .as_str()
                    .context("Missing 'workflow_id'")?;

                let resp = with_auth(
                    http.post(format!("{base}/workflows/{workflow_id}/start")),
                    api_token.as_ref(),
                )
                .send()
                .await?;

                if resp.status().is_success() {
                    // Start notification watcher for this workflow
                    if let Some(bus) = &self.message_bus {
                        let topic = format!("workflow:{workflow_id}");
                        if let Ok(rx) = bus.subscribe_topic(&topic).await {
                            let sid = session.id.clone();
                            let pool = self.pool.clone();
                            let bc = self.broadcaster.clone();
                            let cancel = CancellationToken::new();
                            // Store token so we can cancel when the session is cleaned up
                            let watcher_key = format!("{}:{}", session.id, workflow_id);
                            self.watcher_tokens
                                .lock()
                                .await
                                .insert(watcher_key, cancel.clone());
                            tokio::spawn(super::notifications::watch_workflow_events(
                                sid,
                                workflow_id.to_string(),
                                pool,
                                bc,
                                rx,
                                cancel,
                            ));
                        }
                    }
                    Ok(format!(
                        "Workflow '{workflow_id}' started. The orchestrator agent is now running and will create tasks and assign terminals."
                    ))
                } else {
                    let status = resp.status();
                    let body = resp.text().await.unwrap_or_default();
                    Ok(format!("Start failed ({status}): {}", body.chars().take(200).collect::<String>()))
                }
            }
            _ => Err(anyhow::anyhow!("Unknown runtime tool: {tool_name}")),
        }
    }
}
