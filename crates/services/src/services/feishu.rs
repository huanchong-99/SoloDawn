//! Feishu (Lark) integration service.
//!
//! Connects to Feishu via WebSocket, processes incoming events (messages,
//! slash commands), and forwards chat messages to the orchestrator.

use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use db::models::{ExternalConversationBinding, Workflow, feishu_config::FeishuAppConfig};
use feishu_connector::{
    client::FeishuClient,
    events::{self, FeishuEvent, ReceivedMessage},
    messages::FeishuMessenger,
    types::FeishuConfig,
};
use sqlx::SqlitePool;
use tokio::sync::mpsc;
use tracing;

use super::{chat_connector::ChatConnector, orchestrator::message_bus::SharedMessageBus};

const FEISHU_PROVIDER: &str = "feishu";
use feishu_connector::events::EVENT_TYPE_MESSAGE;

// ---------------------------------------------------------------------------
// FeishuService
// ---------------------------------------------------------------------------

/// Long-running service that maintains a Feishu WebSocket connection and
/// routes incoming events to the orchestrator.
pub struct FeishuService {
    client: FeishuClient,
    event_rx: mpsc::Receiver<FeishuEvent>,
    messenger: Arc<FeishuMessenger>,
    pool: SqlitePool,
    bus: SharedMessageBus,
    /// Optional broadcast sender for forwarding events to external subscribers
    /// (e.g. test-receive endpoint).
    event_broadcaster: Option<tokio::sync::broadcast::Sender<FeishuEvent>>,
    /// Optional Concierge Agent for unified conversational interface.
    concierge_agent: Option<Arc<super::concierge::ConciergeAgent>>,
    /// Shared config for reading workflow model library.
    shared_config: Option<Arc<tokio::sync::RwLock<super::config::Config>>>,
}

impl FeishuService {
    /// Build a new service from a [`FeishuConfig`].
    pub fn new(config: FeishuConfig, pool: SqlitePool, bus: SharedMessageBus) -> Self {
        let (client, event_rx) = FeishuClient::new(config.clone());
        let messenger = Arc::new(FeishuMessenger::new(
            client.auth().clone(),
            config.base_url.clone(),
        ));
        Self {
            client,
            event_rx,
            messenger,
            pool,
            bus,
            event_broadcaster: None,
            concierge_agent: None,
            shared_config: None,
        }
    }

    /// Set the Concierge Agent for unified conversational routing.
    pub fn set_concierge_agent(&mut self, agent: Arc<super::concierge::ConciergeAgent>) {
        self.concierge_agent = Some(agent);
    }

    /// Set the shared config for reading workflow model library.
    pub fn set_shared_config(&mut self, config: Arc<tokio::sync::RwLock<super::config::Config>>) {
        self.shared_config = Some(config);
    }

    /// Try to create a service from the enabled [`FeishuAppConfig`] row in the
    /// database. Returns `None` when no enabled config exists.
    ///
    /// `decrypt_secret` is a caller-provided closure that decrypts the
    /// stored `app_secret_encrypted` value.
    pub async fn from_db<F>(
        pool: SqlitePool,
        bus: SharedMessageBus,
        decrypt_secret: F,
    ) -> Result<Option<Self>>
    where
        F: FnOnce(&str) -> Result<String>,
    {
        let Some(cfg) = FeishuAppConfig::find_enabled(&pool).await? else {
            return Ok(None);
        };
        let app_secret = decrypt_secret(&cfg.app_secret_encrypted)?;
        let feishu_config = FeishuConfig {
            app_id: cfg.app_id,
            app_secret,
            base_url: cfg.base_url,
        };
        Ok(Some(Self::new(feishu_config, pool, bus)))
    }

    /// Start the WebSocket connection and event processing loop.
    ///
    /// This method runs until the connection is closed or an unrecoverable
    /// error occurs. Callers should wrap it in a retry/reconnect loop.
    pub async fn start(&mut self) -> Result<()> {
        let connect_fut = self.client.connect();

        // Take a mutable reference to event_rx separately so we don't hold
        // an immutable borrow on `self` across the select! branches.
        let event_rx = &mut self.event_rx;
        let pool = &self.pool;
        let bus = &self.bus;
        let messenger = &self.messenger;
        let broadcaster = self.event_broadcaster.as_ref();
        let concierge = self.concierge_agent.as_ref();
        let shared_config = self.shared_config.as_ref();

        tokio::select! {
            conn_result = connect_fut => {
                if let Err(e) = conn_result {
                    tracing::error!(error = %e, "Feishu WebSocket connection ended with error");
                }
            }
            () = Self::process_events_inner(event_rx, pool, bus, messenger, broadcaster, concierge, shared_config) => {
                tracing::info!("Feishu event processing loop ended");
            }
        }

        Ok(())
    }

    /// Internal event processing loop.
    async fn process_events_inner(
        event_rx: &mut mpsc::Receiver<FeishuEvent>,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
        broadcaster: Option<&tokio::sync::broadcast::Sender<FeishuEvent>>,
        concierge_agent: Option<&Arc<super::concierge::ConciergeAgent>>,
        shared_config: Option<&Arc<tokio::sync::RwLock<super::config::Config>>>,
    ) {
        while let Some(event) = event_rx.recv().await {
            if let Some(tx) = broadcaster {
                let count = tx.receiver_count();
                tracing::debug!(receiver_count = count, "Broadcasting event");
                if count > 0 {
                    // [W2-30-10] Log at debug when the broadcast drop happens so
                    // silent drops are observable. `send` only errors when there
                    // are no active receivers, but we may race with receiver
                    // close after the count check above.
                    if let Err(err) = tx.send(event.clone()) {
                        tracing::debug!(?err, "Feishu broadcast dropped: no active receivers");
                    }
                } else {
                    tracing::debug!("Feishu broadcast dropped: no receivers");
                }
            } else {
                tracing::debug!("No broadcaster configured");
            }
            if let Err(e) = Self::handle_event_inner(
                &event,
                pool,
                bus,
                messenger,
                concierge_agent,
                shared_config,
            )
            .await
            {
                tracing::warn!(error = %e, "Failed to handle Feishu event");
            }
        }
    }

    /// Route an incoming event by its `event_type`.
    async fn handle_event_inner(
        event: &FeishuEvent,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
        concierge_agent: Option<&Arc<super::concierge::ConciergeAgent>>,
        shared_config: Option<&Arc<tokio::sync::RwLock<super::config::Config>>>,
    ) -> Result<()> {
        let Some(header) = &event.header else {
            tracing::debug!("Ignoring Feishu event without header");
            return Ok(());
        };

        match header.event_type.as_str() {
            EVENT_TYPE_MESSAGE => {
                Self::handle_message_inner(
                    event,
                    pool,
                    bus,
                    messenger,
                    concierge_agent,
                    shared_config,
                )
                .await
            }
            other => {
                tracing::debug!(event_type = %other, "Ignoring unhandled Feishu event type");
                Ok(())
            }
        }
    }

    /// Handle an incoming chat message.
    ///
    /// Routes through the Concierge Agent if available, falling back to
    /// the legacy `/bind` + orchestrator-forwarding path.
    async fn handle_message_inner(
        event: &FeishuEvent,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
        concierge_agent: Option<&Arc<super::concierge::ConciergeAgent>>,
        shared_config: Option<&Arc<tokio::sync::RwLock<super::config::Config>>>,
    ) -> Result<()> {
        let msg = events::parse_message_event(event)?;

        // Only handle text messages.
        if msg.message_type != "text" {
            tracing::debug!(
                message_type = %msg.message_type,
                "Ignoring non-text Feishu message"
            );
            return Ok(());
        }

        let text = events::parse_text_content(&msg.content);
        let text = text.trim();

        if text.is_empty() {
            return Ok(());
        }

        // Legacy slash commands (backward compatible)
        if let Some(workflow_id) = text.strip_prefix("/bind ").map(str::trim) {
            return Self::handle_bind_inner(&msg, workflow_id, pool, messenger).await;
        }
        if text.eq_ignore_ascii_case("/unbind") {
            return Self::handle_unbind_inner(&msg, pool, messenger).await;
        }

        // Session management commands
        if text.eq_ignore_ascii_case("/help") {
            return Self::handle_help(&msg, messenger).await;
        }
        if text.eq_ignore_ascii_case("/list") {
            return Self::handle_list_sessions(&msg, pool, messenger).await;
        }
        if text.eq_ignore_ascii_case("/current") {
            return Self::handle_current_session(&msg, pool, messenger).await;
        }
        if let Some(name) = text.strip_prefix("/new ").map(str::trim) {
            return Self::handle_new_session(&msg, name, pool, messenger, shared_config).await;
        }
        if text.eq_ignore_ascii_case("/new") {
            return Self::handle_new_session(&msg, "New Session", pool, messenger, shared_config)
                .await;
        }
        if let Some(num_str) = text.strip_prefix("/switch ").map(str::trim) {
            return Self::handle_switch_session(&msg, num_str, pool, messenger).await;
        }

        // If Concierge Agent is available, route through it
        if let Some(concierge) = concierge_agent {
            return Self::handle_via_concierge(
                &msg,
                text,
                pool,
                messenger,
                concierge,
                shared_config,
            )
            .await;
        }

        // Fallback: legacy direct orchestrator forwarding
        Self::forward_to_orchestrator_inner(&msg, text, pool, bus, messenger).await
    }

    /// Route a message through the Concierge Agent.
    ///
    /// Finds or creates a ConciergeSession for this Feishu chat,
    /// processes the message, and sends the response back.
    async fn handle_via_concierge(
        msg: &ReceivedMessage,
        text: &str,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
        concierge: &Arc<super::concierge::ConciergeAgent>,
        shared_config: Option<&Arc<tokio::sync::RwLock<super::config::Config>>>,
    ) -> Result<()> {
        use db::models::concierge::{ConciergeMessage, ConciergeSession, ConciergeSessionChannel};

        // Find or create session for this Feishu chat
        let mut session =
            match ConciergeSession::find_by_channel(pool, FEISHU_PROVIDER, &msg.chat_id).await? {
                Some(s) => {
                    // Ensure chat_id is persisted on existing sessions
                    if s.feishu_chat_id.is_none() {
                        let _ = ConciergeSession::update_feishu_chat_id(pool, &s.id, &msg.chat_id)
                            .await;
                    }
                    s
                }
                None => {
                    let mut new_session =
                        ConciergeSession::new(&text.chars().take(50).collect::<String>());
                    new_session.feishu_sync = true; // Feishu-initiated = always sync
                    new_session.feishu_chat_id = Some(msg.chat_id.clone());
                    ConciergeSession::insert(pool, &new_session).await?;

                    ConciergeSessionChannel::upsert(
                        pool,
                        &new_session.id,
                        FEISHU_PROVIDER,
                        &msg.chat_id,
                        Some(&msg.sender_open_id),
                    )
                    .await?;

                    tracing::info!(
                        chat_id = %msg.chat_id,
                        session_id = %new_session.id,
                        "Created new Concierge session from Feishu"
                    );
                    new_session
                }
            };

        // ── LLM model selection flow ──
        // If session has no LLM config, we need to configure it first.
        if session.llm_api_key_encrypted.is_none() {
            let models = Self::get_available_models(shared_config).await?;

            if models.is_empty() {
                messenger
                    .reply_text(
                        &msg.message_id,
                        "没有可用的 AI 模型配置。请先在 SoloDawn 设置页面配置至少一个模型的 API Key。",
                    )
                    .await?;
                return Ok(());
            }

            if models.len() == 1 {
                // Only one model — use it directly, no need to ask
                Self::apply_model_to_session(&mut session, &models[0]);
                Self::persist_session_llm(pool, &session).await?;
                tracing::info!(
                    display_name = %models[0].display_name,
                    "Auto-selected single available model for Concierge"
                );
                // Send welcome on first connection, then fall through to process the message
                let welcome = format!(
                    "🤖 已自动选择 {}。\n\n{}",
                    models[0].display_name,
                    Self::build_help_text()
                );
                messenger.reply_text(&msg.message_id, &welcome).await?;
            } else {
                // Multiple models — check if user is replying with a selection number
                if let Some(idx) = Self::parse_model_selection(text, models.len()) {
                    Self::apply_model_to_session(&mut session, &models[idx]);
                    Self::persist_session_llm(pool, &session).await?;
                    let welcome = format!(
                        "✅ 已选择 {}，开始对话吧！\n\n{}",
                        models[idx].display_name,
                        Self::build_help_text()
                    );
                    messenger.reply_text(&msg.message_id, &welcome).await?;
                    return Ok(());
                }

                // Not a selection — save user's message, then ask them to pick
                let user_msg = ConciergeMessage::new_user(
                    &session.id,
                    text,
                    Some(FEISHU_PROVIDER),
                    Some(&msg.sender_open_id),
                );
                ConciergeMessage::insert(pool, &user_msg).await?;

                let prompt = Self::build_model_selection_prompt(&models);
                messenger.reply_text(&msg.message_id, &prompt).await?;
                return Ok(());
            }
        }

        // ── Normal message processing ──
        match concierge
            .process_message(
                &session.id,
                text,
                Some(FEISHU_PROVIDER),
                Some(&msg.sender_open_id),
            )
            .await
        {
            Ok(response) => {
                messenger.reply_text(&msg.message_id, &response).await?;
            }
            Err(e) => {
                tracing::warn!(
                    chat_id = %msg.chat_id,
                    error = %e,
                    "Concierge processing failed"
                );
                messenger
                    .reply_text(&msg.message_id, &format!("抱歉，出现了错误: {e}"))
                    .await?;
            }
        }

        Ok(())
    }

    /// Get all models from the workflow model library in config.
    async fn get_available_models(
        shared_config: Option<&Arc<tokio::sync::RwLock<super::config::Config>>>,
    ) -> Result<Vec<super::config::WorkflowModelLibraryItem>> {
        let config = shared_config
            .as_ref()
            .ok_or_else(|| anyhow::anyhow!("No shared config available"))?;
        let config = config.read().await;
        let models: Vec<_> = config
            .workflow_model_library
            .iter()
            .filter(|m| !m.api_key.is_empty())
            .cloned()
            .collect();
        Ok(models)
    }

    /// Apply a workflow model library item's credentials to a ConciergeSession.
    fn apply_model_to_session(
        session: &mut db::models::concierge::ConciergeSession,
        item: &super::config::WorkflowModelLibraryItem,
    ) {
        // Encrypt the API key before storing
        match db::models::concierge::ConciergeSession::encrypt_api_key(&item.api_key) {
            Ok(encrypted) => session.llm_api_key_encrypted = Some(encrypted),
            Err(e) => tracing::warn!("Failed to encrypt API key for concierge: {e}"),
        }
        session.llm_api_type = Some(item.api_type.clone());
        session.llm_base_url = Some(item.base_url.clone());
        session.llm_model_id = Some(item.model_id.clone());
    }

    /// Persist LLM config from a session object to the database.
    async fn persist_session_llm(
        pool: &SqlitePool,
        session: &db::models::concierge::ConciergeSession,
    ) -> Result<()> {
        db::models::concierge::ConciergeSession::update_llm_config(
            pool,
            &session.id,
            session.llm_model_id.as_deref(),
            session.llm_api_type.as_deref(),
            session.llm_base_url.as_deref(),
            session.llm_api_key_encrypted.as_deref(),
        )
        .await?;
        Ok(())
    }

    /// Build a user-facing model selection prompt.
    fn build_model_selection_prompt(models: &[super::config::WorkflowModelLibraryItem]) -> String {
        let mut prompt = String::from("请选择要使用的 AI 模型（输入数字）：\n\n");
        for (i, m) in models.iter().enumerate() {
            let verified_tag = if m.is_verified { " ✅" } else { "" };
            prompt.push_str(&format!(
                "  {}. {} ({}){}\n",
                i + 1,
                m.display_name,
                m.model_id,
                verified_tag
            ));
        }
        prompt.push_str("\n回复数字即可选择，例如回复 1");
        prompt
    }

    /// Try to parse a model selection response (e.g., "1", "2", "3").
    fn parse_model_selection(text: &str, max: usize) -> Option<usize> {
        let trimmed = text.trim();
        let num: usize = trimmed.parse().ok()?;
        if num >= 1 && num <= max {
            Some(num - 1) // 0-indexed
        } else {
            None
        }
    }

    // ── Session management command handlers ──

    fn build_help_text() -> String {
        [
            "📖 SoloDawn 飞书命令指南\n",
            "💬 会话管理：",
            "  /new <名称>  — 创建新会话（当前会话自动归档）",
            "  /list       — 显示所有会话列表",
            "  /switch <N> — 切换到第N个会话",
            "  /current    — 显示当前会话信息",
            "",
            "🔗 工作流绑定：",
            "  /bind <ID>  — 绑定到指定工作流",
            "  /unbind     — 解除工作流绑定",
            "",
            "ℹ️ 其他：",
            "  /help       — 显示此帮助信息",
            "",
            "直接发送消息即可与 AI 助手对话，AI 可以帮你创建项目、规划任务、监控工作流进度。",
        ]
        .join("\n")
    }

    async fn handle_help(msg: &ReceivedMessage, messenger: &Arc<FeishuMessenger>) -> Result<()> {
        messenger
            .reply_text(&msg.message_id, &Self::build_help_text())
            .await?;
        Ok(())
    }

    async fn handle_list_sessions(
        msg: &ReceivedMessage,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        use db::models::concierge::ConciergeSession;

        let sessions =
            ConciergeSession::find_all_by_channel(pool, FEISHU_PROVIDER, &msg.chat_id).await?;
        if sessions.is_empty() {
            messenger
                .reply_text(&msg.message_id, "暂无会话。发送任意消息开始新对话。")
                .await?;
            return Ok(());
        }

        let active = ConciergeSession::find_by_channel(pool, FEISHU_PROVIDER, &msg.chat_id).await?;
        let active_id = active.map(|s| s.id);

        let mut lines = vec!["📋 会话列表：\n".to_string()];
        for (i, session) in sessions.iter().enumerate() {
            let marker = if Some(&session.id) == active_id.as_ref() {
                "👉 "
            } else {
                "   "
            };
            let name = if session.name.is_empty() {
                &session.id[..8]
            } else {
                &session.name
            };
            lines.push(format!("{}{}. {}", marker, i + 1, name));
        }
        lines.push("\n使用 /switch <编号> 切换会话".to_string());

        messenger
            .reply_text(&msg.message_id, &lines.join("\n"))
            .await?;
        Ok(())
    }

    async fn handle_current_session(
        msg: &ReceivedMessage,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        use db::models::concierge::ConciergeSession;

        let session =
            ConciergeSession::find_by_channel(pool, FEISHU_PROVIDER, &msg.chat_id).await?;
        match session {
            Some(s) => {
                let name = if s.name.is_empty() {
                    s.id[..8].to_string()
                } else {
                    s.name.clone()
                };
                let wf = s.active_workflow_id.as_deref().unwrap_or("无");
                let text = format!(
                    "📌 当前会话：{}\n🔗 工作流：{}\n🤖 模型：{}",
                    name,
                    wf,
                    s.llm_model_id.as_deref().unwrap_or("未配置")
                );
                messenger.reply_text(&msg.message_id, &text).await?;
            }
            None => {
                messenger
                    .reply_text(
                        &msg.message_id,
                        "当前没有活跃会话。发送任意消息开始新对话。",
                    )
                    .await?;
            }
        }
        Ok(())
    }

    async fn handle_new_session(
        msg: &ReceivedMessage,
        name: &str,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
        shared_config: Option<&Arc<tokio::sync::RwLock<super::config::Config>>>,
    ) -> Result<()> {
        use db::models::concierge::{ConciergeSession, ConciergeSessionChannel};

        // Deactivate current channel binding
        ConciergeSessionChannel::deactivate(pool, FEISHU_PROVIDER, &msg.chat_id).await?;

        // Create new session
        let mut new_session = ConciergeSession::new(name);
        new_session.feishu_sync = true;

        // Auto-apply model if only one available
        if let Ok(models) = Self::get_available_models(shared_config).await {
            if models.len() == 1 {
                Self::apply_model_to_session(&mut new_session, &models[0]);
            }
        }

        ConciergeSession::insert(pool, &new_session).await?;
        ConciergeSessionChannel::upsert(
            pool,
            &new_session.id,
            FEISHU_PROVIDER,
            &msg.chat_id,
            Some(&msg.sender_open_id),
        )
        .await?;

        tracing::info!(session_id = %new_session.id, name = %name, "Created new session via /new command");

        let reply = if new_session.llm_api_key_encrypted.is_some() {
            format!("✅ 已创建新会话「{}」，可以开始对话了！", name)
        } else {
            format!("✅ 已创建新会话「{}」。请选择模型后开始对话。", name)
        };
        messenger.reply_text(&msg.message_id, &reply).await?;
        Ok(())
    }

    async fn handle_switch_session(
        msg: &ReceivedMessage,
        num_str: &str,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        use db::models::concierge::{ConciergeSession, ConciergeSessionChannel};

        let sessions =
            ConciergeSession::find_all_by_channel(pool, FEISHU_PROVIDER, &msg.chat_id).await?;
        let idx: usize = match num_str.trim().parse::<usize>() {
            Ok(n) if n >= 1 && n <= sessions.len() => n - 1,
            _ => {
                messenger
                    .reply_text(
                        &msg.message_id,
                        &format!(
                            "请输入 1-{} 之间的数字。使用 /list 查看会话列表。",
                            sessions.len()
                        ),
                    )
                    .await?;
                return Ok(());
            }
        };

        let target = &sessions[idx];
        ConciergeSessionChannel::switch_active_session(
            pool,
            FEISHU_PROVIDER,
            &msg.chat_id,
            &target.id,
        )
        .await?;

        let name = if target.name.is_empty() {
            &target.id[..8]
        } else {
            &target.name
        };
        messenger
            .reply_text(&msg.message_id, &format!("🔄 已切换到会话「{}」", name))
            .await?;
        Ok(())
    }

    /// `/bind <workflow_id>` -- create or update a conversation binding.
    ///
    /// G32-006: Validates that `workflow_id` is a well-formed UUID and that the
    /// referenced workflow actually exists in the database before creating the
    /// binding. This prevents dangling bindings to non-existent workflows.
    async fn handle_bind_inner(
        msg: &ReceivedMessage,
        workflow_id: &str,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        if workflow_id.is_empty() {
            messenger
                .reply_text(&msg.message_id, "Usage: /bind <workflow_id>")
                .await?;
            return Ok(());
        }

        // G32-006: Validate UUID format
        if uuid::Uuid::parse_str(workflow_id).is_err() {
            messenger
                .reply_text(
                    &msg.message_id,
                    "Invalid workflow_id format. Expected a UUID (e.g. 550e8400-e29b-41d4-a716-446655440000).",
                )
                .await?;
            return Ok(());
        }

        // G32-006: Verify workflow exists in database
        let workflow = Workflow::find_by_id(pool, workflow_id).await?;
        if workflow.is_none() {
            messenger
                .reply_text(
                    &msg.message_id,
                    &format!("Workflow {workflow_id} not found."),
                )
                .await?;
            return Ok(());
        }

        ExternalConversationBinding::upsert(
            pool,
            FEISHU_PROVIDER,
            &msg.chat_id,
            workflow_id,
            Some(&msg.sender_open_id),
        )
        .await?;

        let reply = format!("Bound to workflow {workflow_id}");
        messenger.reply_text(&msg.message_id, &reply).await?;
        tracing::info!(
            chat_id = %msg.chat_id,
            workflow_id = %workflow_id,
            "Feishu conversation bound"
        );
        Ok(())
    }

    /// `/unbind` -- deactivate the current conversation binding.
    async fn handle_unbind_inner(
        msg: &ReceivedMessage,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        let affected =
            ExternalConversationBinding::deactivate(pool, FEISHU_PROVIDER, &msg.chat_id).await?;

        let reply = if affected > 0 {
            "Conversation unbound".to_string()
        } else {
            "No active binding to remove".to_string()
        };
        messenger.reply_text(&msg.message_id, &reply).await?;
        tracing::info!(chat_id = %msg.chat_id, affected, "Feishu conversation unbound");
        Ok(())
    }

    /// Forward a regular chat message to the orchestrator for the bound workflow.
    async fn forward_to_orchestrator_inner(
        msg: &ReceivedMessage,
        text: &str,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        let binding =
            ExternalConversationBinding::find_active(pool, FEISHU_PROVIDER, &msg.chat_id).await?;

        let Some(binding) = binding else {
            messenger
                .reply_text(
                    &msg.message_id,
                    "This conversation is not bound. Use /bind <workflow_id> first.",
                )
                .await?;
            return Ok(());
        };

        // G32-016: Semantic mapping note — `BusMessage::TerminalMessage` is reused
        // here to carry external chat messages into the orchestrator's event loop.
        // The variant name refers to its original purpose (terminal stdin), but the
        // orchestrator treats the `message` field as opaque text. The `[feishu:...]`
        // prefix lets the orchestrator distinguish the source. A dedicated
        // `ExternalChatMessage` variant would be cleaner but requires coordinated
        // changes across the orchestrator agent, so we document the mapping instead.
        use super::orchestrator::message_bus::BusMessage;
        let instruction_msg = BusMessage::TerminalMessage {
            message: format!("[feishu:{}:{}] {}", msg.chat_id, msg.sender_open_id, text),
        };
        let topic = format!("workflow:{}", binding.workflow_id);
        bus.publish(&topic, instruction_msg).await?;

        tracing::info!(
            chat_id = %msg.chat_id,
            workflow_id = %binding.workflow_id,
            "Forwarded Feishu message to orchestrator"
        );
        Ok(())
    }

    /// Get a reference to the messenger (useful for building a [`FeishuConnector`]).
    pub fn messenger(&self) -> &Arc<FeishuMessenger> {
        &self.messenger
    }

    /// Set an event broadcaster for forwarding incoming events to external
    /// subscribers (e.g. test-receive endpoint).
    pub fn set_event_broadcaster(&mut self, tx: tokio::sync::broadcast::Sender<FeishuEvent>) {
        self.event_broadcaster = Some(tx);
    }

    /// Get the client's internal connected flag for sharing with FeishuHandle.
    pub fn connected_flag(&self) -> Arc<tokio::sync::RwLock<bool>> {
        self.client.connected_flag()
    }
}

// ---------------------------------------------------------------------------
// FeishuConnector — ChatConnector implementation
// ---------------------------------------------------------------------------

/// Wraps [`FeishuMessenger`] to implement the [`ChatConnector`] trait.
pub struct FeishuConnector {
    messenger: Arc<FeishuMessenger>,
    /// Live connection flag shared with the underlying [`FeishuClient`].
    /// The client sets this to `true` on WebSocket connect and `false` on
    /// disconnect, so `is_connected()` always reflects real transport state.
    ws_connected: Arc<tokio::sync::RwLock<bool>>,
}

impl FeishuConnector {
    /// Create a new connector from a shared messenger and the client's live
    /// connected flag (obtained via [`FeishuService::connected_flag`]).
    pub fn new(
        messenger: Arc<FeishuMessenger>,
        ws_connected: Arc<tokio::sync::RwLock<bool>>,
    ) -> Self {
        Self {
            messenger,
            ws_connected,
        }
    }
}

#[async_trait]
impl ChatConnector for FeishuConnector {
    async fn send_message(&self, conversation_id: &str, content: &str) -> anyhow::Result<String> {
        self.messenger.send_text(conversation_id, content).await
    }

    async fn send_reply(
        &self,
        _conversation_id: &str,
        message_id: &str,
        content: &str,
    ) -> anyhow::Result<String> {
        self.messenger.reply_text(message_id, content).await
    }

    fn provider_name(&self) -> &str {
        FEISHU_PROVIDER
    }

    fn is_connected(&self) -> bool {
        // Use try_read to avoid blocking in this synchronous trait method.
        // Falls back to `false` if the lock is currently held by a writer
        // (transient — the writer is the reconnect loop toggling the flag).
        self.ws_connected
            .try_read()
            .map(|guard| *guard)
            .unwrap_or(false)
    }
}
