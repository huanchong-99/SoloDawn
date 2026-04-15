//! Planning draft API for orchestrated workspace mode.

use std::collections::HashMap;

use axum::{
    Extension, Json, Router,
    extract::{Path, Query, State},
    response::Json as ResponseJson,
    routing::{get, post, put},
};
use db::models::planning_draft::{PlanningDraft, PlanningDraftMessage, PLANNING_DRAFT_STATUSES};
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use services::services::orchestrator::{
    LLMMessage, OrchestratorConfig, create_claude_code_native_client, create_llm_client,
    config::{PromptProfile, system_prompt_for_profile},
};
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError, feishu_handle::SharedFeishuHandle};

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateDraftRequest {
    pub project_id: String,
    pub name: Option<String>,
    pub planner_model_id: Option<String>,
    pub planner_api_type: Option<String>,
    pub planner_base_url: Option<String>,
    pub planner_api_key: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SendMessageRequest {
    pub message: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSpecRequest {
    pub requirement_summary: Option<String>,
    pub technical_spec: Option<String>,
    pub workflow_seed: Option<String>,
    pub status: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DraftResponse {
    pub id: String,
    pub project_id: String,
    pub name: String,
    pub status: String,
    pub requirement_summary: Option<String>,
    pub technical_spec: Option<String>,
    pub workflow_seed: Option<String>,
    pub materialized_workflow_id: Option<String>,
    pub feishu_sync: bool,
    pub sync_tools: bool,
    pub sync_terminal: bool,
    pub sync_progress: bool,
    pub notify_on_completion: bool,
    pub created_at: String,
    pub updated_at: String,
}

impl From<PlanningDraft> for DraftResponse {
    fn from(d: PlanningDraft) -> Self {
        Self {
            id: d.id,
            project_id: d.project_id.to_string(),
            name: d.name,
            status: d.status,
            requirement_summary: d.requirement_summary,
            technical_spec: d.technical_spec,
            workflow_seed: d.workflow_seed,
            materialized_workflow_id: d.materialized_workflow_id,
            feishu_sync: d.feishu_sync,
            sync_tools: d.sync_tools,
            sync_terminal: d.sync_terminal,
            sync_progress: d.sync_progress,
            notify_on_completion: d.notify_on_completion,
            created_at: d.created_at.to_rfc3339(),
            updated_at: d.updated_at.to_rfc3339(),
        }
    }
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MessageResponse {
    pub id: String,
    pub draft_id: String,
    pub role: String,
    pub content: String,
    pub created_at: String,
}

impl From<PlanningDraftMessage> for MessageResponse {
    fn from(m: PlanningDraftMessage) -> Self {
        Self {
            id: m.id,
            draft_id: m.draft_id,
            role: m.role,
            content: m.content,
            created_at: m.created_at.to_rfc3339(),
        }
    }
}

pub fn planning_draft_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/", post(create_draft).get(list_drafts))
        .route("/{draft_id}", get(get_draft))
        .route("/{draft_id}/spec", put(update_spec))
        .route("/{draft_id}/confirm", post(confirm_draft))
        .route("/{draft_id}/materialize", post(materialize_draft))
        .route("/{draft_id}/feishu-sync", post(toggle_feishu_sync))
        .route(
            "/{draft_id}/messages",
            get(list_messages).post(send_message),
        )
}

async fn create_draft(
    State(deployment): State<DeploymentImpl>,
    Json(req): Json<CreateDraftRequest>,
) -> Result<ResponseJson<ApiResponse<DraftResponse>>, ApiError> {
    let project_id = Uuid::parse_str(&req.project_id)
        .map_err(|_| ApiError::BadRequest("project_id must be a valid UUID".to_string()))?;

    let mut draft = PlanningDraft::new(project_id, req.name.as_deref().unwrap_or(""));
    draft.planner_model_id = req.planner_model_id;
    draft.planner_api_type = req.planner_api_type;
    draft.planner_base_url = req.planner_base_url;
    if let Some(ref api_key) = req.planner_api_key {
        draft.set_api_key(api_key).map_err(|e| ApiError::Internal(format!("Failed to encrypt API key: {e}")))?;
    }

    PlanningDraft::insert(&deployment.db().pool, &draft)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create planning draft: {e}")))?;

    Ok(Json(ApiResponse::success(DraftResponse::from(draft))))
}

async fn list_drafts(
    State(deployment): State<DeploymentImpl>,
    Query(params): Query<HashMap<String, String>>,
) -> Result<ResponseJson<ApiResponse<Vec<DraftResponse>>>, ApiError> {
    let drafts = if let Some(project_id_str) = params.get("project_id") {
        let project_id = Uuid::parse_str(project_id_str)
            .map_err(|_| ApiError::BadRequest("project_id must be a valid UUID".to_string()))?;
        PlanningDraft::find_by_project(&deployment.db().pool, project_id)
            .await
            .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
    } else {
        PlanningDraft::find_all(&deployment.db().pool)
            .await
            .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
    };

    let dtos: Vec<DraftResponse> = drafts.into_iter().map(DraftResponse::from).collect();
    Ok(Json(ApiResponse::success(dtos)))
}

async fn get_draft(
    State(deployment): State<DeploymentImpl>,
    Path(draft_id): Path<String>,
) -> Result<ResponseJson<ApiResponse<DraftResponse>>, ApiError> {
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    Ok(Json(ApiResponse::success(DraftResponse::from(draft))))
}

async fn update_spec(
    State(deployment): State<DeploymentImpl>,
    Path(draft_id): Path<String>,
    Json(req): Json<UpdateSpecRequest>,
) -> Result<ResponseJson<ApiResponse<DraftResponse>>, ApiError> {
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    if draft.status == "materialized" || draft.status == "cancelled" {
        return Err(ApiError::BadRequest(format!(
            "Cannot update draft in status '{}'",
            draft.status
        )));
    }

    if let Some(ref new_status) = req.status {
        if !PLANNING_DRAFT_STATUSES.contains(&new_status.as_str()) {
            return Err(ApiError::BadRequest(format!(
                "Invalid status: {new_status}"
            )));
        }
        if new_status == "materialized" {
            return Err(ApiError::BadRequest(
                "Cannot set status to 'materialized' directly; use the materialize endpoint"
                    .to_string(),
            ));
        }
        // Enforce forward-only state transitions
        let valid_transitions = [
            ("gathering", "spec_ready"),
            ("spec_ready", "confirmed"),
        ];
        if new_status != &draft.status {
            let is_valid = valid_transitions
                .iter()
                .any(|(from, to)| draft.status == *from && new_status.as_str() == *to);
            if !is_valid {
                return Err(ApiError::BadRequest(format!(
                    "Invalid transition: {} \u{2192} {}",
                    draft.status, new_status
                )));
            }
        }
    }

    PlanningDraft::update_spec(
        &deployment.db().pool,
        &draft_id,
        req.requirement_summary.as_deref(),
        req.technical_spec.as_deref(),
        req.workflow_seed.as_deref(),
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to update spec: {e}")))?;

    if let Some(ref new_status) = req.status {
        PlanningDraft::update_status(&deployment.db().pool, &draft_id, new_status)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to update status: {e}")))?;
    }

    let updated = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Draft disappeared after update".to_string()))?;

    Ok(Json(ApiResponse::success(DraftResponse::from(updated))))
}

async fn confirm_draft(
    State(deployment): State<DeploymentImpl>,
    Path(draft_id): Path<String>,
) -> Result<ResponseJson<ApiResponse<DraftResponse>>, ApiError> {
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    if draft.status != "spec_ready" && draft.status != "gathering" {
        return Err(ApiError::BadRequest(format!(
            "Can only confirm drafts in 'gathering' or 'spec_ready' status, got '{}'",
            draft.status
        )));
    }

    PlanningDraft::set_confirmed(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to confirm draft: {e}")))?;

    let confirmed = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Draft disappeared after confirm".to_string()))?;

    Ok(Json(ApiResponse::success(DraftResponse::from(confirmed))))
}

async fn send_message(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
    Path(draft_id): Path<String>,
    Json(req): Json<SendMessageRequest>,
) -> Result<ResponseJson<ApiResponse<Vec<MessageResponse>>>, ApiError> {
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    if draft.status == "materialized" || draft.status == "cancelled" {
        return Err(ApiError::BadRequest(format!(
            "Cannot send messages to draft in status '{}'",
            draft.status
        )));
    }

    if req.message.trim().is_empty() {
        return Err(ApiError::BadRequest(
            "Message content cannot be empty".to_string(),
        ));
    }

    // 1. Auto-fill planner config from workflow_model_library if missing
    let mut draft = draft;
    let missing_planner = draft.planner_model_id.as_deref().map_or(true, str::is_empty)
        || draft.planner_base_url.as_deref().map_or(true, str::is_empty);
    if missing_planner {
        {
            let cfg = deployment.config().read().await;
            if let Some(model) = cfg.workflow_model_library.iter().find(|m| !m.api_key.is_empty()) {
                tracing::info!(
                    draft_id = %draft_id,
                    model_id = %model.model_id,
                    "Auto-filling planner config from workflow_model_library"
                );
                draft.planner_model_id = Some(model.model_id.clone());
                draft.planner_api_type = Some(model.api_type.clone());
                draft.planner_base_url = Some(model.base_url.clone());
                if let Err(e) = draft.set_api_key(&model.api_key) {
                    tracing::warn!(draft_id = %draft_id, "Failed to encrypt auto-filled API key: {e}");
                }
            }
        }
    }

    // 2. Store user message
    let user_msg = PlanningDraftMessage::new(&draft_id, "user", req.message.trim());
    PlanningDraftMessage::insert(&deployment.db().pool, &user_msg)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to save message: {e}")))?;

    let mut result = vec![MessageResponse::from(user_msg)];

    // 3. Try to call LLM and store assistant reply
    // Fallback chain: configured model → Claude Code native credentials
    // Native fallback model bumped to `claude-sonnet-4-6` (Sonnet 4.6) —
    // see the matching comment in agent.rs and the probe test
    // `test_probe_subscription_model_acceptance` for upstream confirmation.
    let llm_client = build_llm_client_from_draft(&draft).or_else(|| {
        tracing::info!(draft_id = %draft_id, "No model configured, trying Claude Code native credentials");
        create_claude_code_native_client("claude-sonnet-4-6")
    });
    if let Some(llm_client) = llm_client {
        let all_messages = PlanningDraftMessage::list_by_draft(&deployment.db().pool, &draft_id)
            .await
            .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;

        let system_prompt = system_prompt_for_profile(PromptProfile::WorkspacePlanning);
        let mut llm_messages = vec![LLMMessage {
            role: "system".to_string(),
            content: system_prompt,
        }];
        for m in &all_messages {
            llm_messages.push(LLMMessage {
                role: m.role.clone(),
                content: m.content.clone(),
            });
        }

        match llm_client.chat(llm_messages).await {
            Ok(response) => {
                let assistant_msg =
                    PlanningDraftMessage::new(&draft_id, "assistant", &response.content);
                if let Err(e) =
                    PlanningDraftMessage::insert(&deployment.db().pool, &assistant_msg).await
                {
                    tracing::warn!(draft_id = %draft_id, "Failed to save assistant reply: {e}");
                } else {
                    result.push(MessageResponse::from(assistant_msg));
                }

                // Auto-transition: if LLM produced a ```json PLANNING_SPEC block,
                // move draft from gathering → spec_ready and extract spec content.
                if draft.status == "gathering"
                    && (response.content.contains("```json\n") || response.content.contains("```\n"))
                    && response.content.contains("\"productGoal\"")
                {
                    // Extract the JSON block from the fenced code block
                    let json_block = response.content
                        .split("```json\n").nth(1)
                        .or_else(|| response.content.split("```\n").nth(1))
                        .and_then(|s| s.split("```").next())
                        .unwrap_or("");

                    let (req_summary, tech_spec) = if let Ok(spec) =
                        serde_json::from_str::<serde_json::Value>(json_block)
                    {
                        let goal = spec["productGoal"].as_str().unwrap_or("").to_string();
                        (goal, json_block.to_string())
                    } else {
                        (String::new(), json_block.to_string())
                    };

                    // Store extracted spec content
                    if let Err(e) = PlanningDraft::update_spec(
                        &deployment.db().pool,
                        &draft_id,
                        Some(&req_summary),
                        Some(&tech_spec),
                        None,
                    ).await {
                        tracing::warn!(draft_id = %draft_id, "Failed to save extracted spec: {e}");
                    }

                    if let Err(e) = PlanningDraft::update_status(
                        &deployment.db().pool,
                        &draft_id,
                        "spec_ready",
                    )
                    .await
                    {
                        tracing::warn!(draft_id = %draft_id, "Failed to auto-transition to spec_ready: {e}");
                    } else {
                        tracing::info!(draft_id = %draft_id, req_summary = %req_summary, "Auto-transitioned draft to spec_ready with extracted spec");
                    }
                }
            }
            Err(e) => {
                tracing::warn!(draft_id = %draft_id, "LLM call failed for planning draft: {e}");
                // Surface the error as an assistant message so the user sees it
                let error_content = format!(
                    "LLM call failed: {e}\n\nPlease check your model configuration (API key, base URL, model name) in Settings."
                );
                let error_msg =
                    PlanningDraftMessage::new(&draft_id, "assistant", &error_content);
                if let Ok(()) =
                    PlanningDraftMessage::insert(&deployment.db().pool, &error_msg).await
                {
                    result.push(MessageResponse::from(error_msg));
                }
            }
        }
    } else {
        tracing::warn!(
            draft_id = %draft_id,
            "No LLM config on planning draft — model credentials may be missing"
        );
        // Surface as an assistant message so the user knows what's wrong
        let error_content =
            "Model not configured for this workspace. Please check Settings → Models and ensure the selected model has a valid API key and base URL.";
        let error_msg =
            PlanningDraftMessage::new(&draft_id, "assistant", error_content);
        if let Ok(()) =
            PlanningDraftMessage::insert(&deployment.db().pool, &error_msg).await
        {
            result.push(MessageResponse::from(error_msg));
        }
    }

    // Push new messages to Feishu if sync is enabled
    if draft.feishu_sync {
        if let Some(ref chat_id) = draft.feishu_chat_id {
            let handle_guard = feishu_handle.read().await;
            if let Some(ref h) = *handle_guard {
                if *h.connected.read().await {
                    let messenger = h.messenger.clone();
                    let chat_id = chat_id.clone();
                    let messages_to_push: Vec<_> = result
                        .iter()
                        .map(|m| (m.role.clone(), m.content.clone()))
                        .collect();
                    drop(handle_guard);
                    tokio::spawn(async move {
                        for (role, content) in messages_to_push {
                            let prefix = if role == "user" { "[User]" } else { "[Assistant]" };
                            let text = format!("{prefix} {content}");
                            let truncated = if text.len() > 4000 {
                                let boundary = text.floor_char_boundary(4000);
                                format!("{}...(truncated)", &text[..boundary])
                            } else {
                                text
                            };
                            if let Err(e) = messenger.send_text(&chat_id, &truncated).await {
                                tracing::warn!("Failed to push planning message to Feishu: {e}");
                                break;
                            }
                        }
                    });
                }
            }
        }
    }

    Ok(Json(ApiResponse::success(result)))
}

/// Build an LLM client from the draft's planner configuration.
/// Returns `None` when required fields (api_type, base_url, api_key, model) are missing.
fn build_llm_client_from_draft(
    draft: &PlanningDraft,
) -> Option<Box<dyn services::services::orchestrator::LLMClient>> {
    tracing::info!(
        draft_id = %draft.id,
        api_type = ?draft.planner_api_type,
        base_url = ?draft.planner_base_url,
        model_id = ?draft.planner_model_id,
        has_api_key = draft.planner_api_key.is_some(),
        "Building LLM client from planning draft"
    );

    let decrypted_key = match draft.get_api_key() {
        Ok(key) => {
            tracing::debug!(
                draft_id = %draft.id,
                key_present = key.is_some(),
                "API key decryption result"
            );
            key
        }
        Err(e) => {
            tracing::warn!("Failed to decrypt planner API key for draft {}: {e}", draft.id);
            return None;
        }
    };

    let config = OrchestratorConfig::from_workflow(
        draft.planner_api_type.as_deref(),
        draft.planner_base_url.as_deref(),
        decrypted_key.as_deref(),
        draft.planner_model_id.as_deref(),
    );
    let config = match config {
        Some(c) => c,
        None => {
            tracing::warn!(
                draft_id = %draft.id,
                has_api_type = draft.planner_api_type.is_some(),
                has_base_url = draft.planner_base_url.is_some(),
                has_api_key = decrypted_key.is_some(),
                has_model_id = draft.planner_model_id.is_some(),
                "OrchestratorConfig creation returned None — missing required fields"
            );
            return None;
        }
    };

    match create_llm_client(&config) {
        Ok(client) => {
            tracing::info!(
                draft_id = %draft.id,
                "LLM client created successfully for planning draft"
            );
            Some(client)
        }
        Err(e) => {
            tracing::warn!("Failed to create LLM client for planning draft: {e}");
            None
        }
    }
}

async fn list_messages(
    State(deployment): State<DeploymentImpl>,
    Path(draft_id): Path<String>,
) -> Result<ResponseJson<ApiResponse<Vec<MessageResponse>>>, ApiError> {
    let _draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    let messages = PlanningDraftMessage::list_by_draft(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;

    let dtos: Vec<MessageResponse> = messages.into_iter().map(MessageResponse::from).collect();
    Ok(Json(ApiResponse::success(dtos)))
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FeishuSyncRequest {
    enabled: bool,
    sync_history: bool,
    chat_id: Option<String>,
}

async fn toggle_feishu_sync(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
    Path(draft_id): Path<String>,
    Json(req): Json<FeishuSyncRequest>,
) -> Result<ResponseJson<ApiResponse<DraftResponse>>, ApiError> {
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    if !req.enabled {
        // Turning off — just clear feishu_sync
        PlanningDraft::update_feishu_sync(&deployment.db().pool, &draft_id, false, None)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to update feishu sync: {e}")))?;
    } else {
        // Turning on — resolve chat_id from feishu handle
        let handle_guard = feishu_handle.read().await;
        let Some(ref h) = *handle_guard else {
            return Err(ApiError::Conflict(
                "Feishu connector is not running".to_string(),
            ));
        };

        if !*h.connected.read().await {
            return Err(ApiError::Conflict(
                "Feishu is not connected".to_string(),
            ));
        }

        let messenger = h.messenger.clone();

        // Resolve chat_id: explicit param → last received → DB binding
        let last_id = h.last_chat_id.try_read().ok().and_then(|g| g.clone());
        drop(handle_guard);

        // Also check any concierge session that already has a feishu_chat_id
        let session_chat_id = {
            use db::models::concierge::ConciergeSession;
            let sessions = ConciergeSession::list_all(&deployment.db().pool).await.unwrap_or_default();
            sessions.into_iter().find_map(|s| s.feishu_chat_id)
        };

        let chat_id = if let Some(id) = req.chat_id.clone() {
            id
        } else if let Some(id) = last_id {
            id
        } else if let Some(id) = session_chat_id {
            id
        } else {
            let binding = db::models::ExternalConversationBinding::find_latest_active(
                &deployment.db().pool,
                "feishu",
            )
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to query bindings: {e}")))?;

            if let Some(b) = binding {
                b.conversation_id
            } else if let Some(bot_chat) = messenger.first_bot_chat_id().await.unwrap_or(None) {
                bot_chat
            } else {
                return Err(ApiError::BadRequest(
                    "No Feishu chat found. Send a message to the bot in Feishu first."
                        .to_string(),
                ));
            }
        };

        PlanningDraft::update_feishu_sync(
            &deployment.db().pool,
            &draft_id,
            true,
            Some(&chat_id),
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to update feishu sync: {e}")))?;

        // If sync_history, push existing messages to Feishu
        if req.sync_history {
            let messages =
                PlanningDraftMessage::list_by_draft(&deployment.db().pool, &draft_id)
                    .await
                    .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;

            let chat_id_clone = chat_id.clone();
            let draft_name = draft.name.clone();
            tokio::spawn(async move {
                // Send a header message
                if let Err(e) = messenger
                    .send_text(
                        &chat_id_clone,
                        &format!("[Planning Draft: {}] Syncing conversation history...", draft_name),
                    )
                    .await
                {
                    tracing::warn!("Failed to send Feishu history header: {e}");
                    return;
                }
                for msg in &messages {
                    let prefix = if msg.role == "user" { "[User]" } else { "[Assistant]" };
                    let text = format!("{prefix} {}", msg.content);
                    // Truncate very long messages for Feishu
                    let truncated = if text.len() > 4000 {
                        let boundary = text.floor_char_boundary(4000);
                        format!("{}...(truncated)", &text[..boundary])
                    } else {
                        text
                    };
                    if let Err(e) = messenger.send_text(&chat_id_clone, &truncated).await {
                        tracing::warn!("Failed to push planning message to Feishu: {e}");
                        break;
                    }
                }
            });
        }
    }

    let updated = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Draft disappeared after update".to_string()))?;

    Ok(Json(ApiResponse::success(DraftResponse::from(updated))))
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MaterializeResponse {
    pub draft_id: String,
    pub workflow_id: String,
    pub status: String,
}

async fn materialize_draft(
    State(deployment): State<DeploymentImpl>,
    Path(draft_id): Path<String>,
) -> Result<ResponseJson<ApiResponse<MaterializeResponse>>, ApiError> {
    use db::models::workflow::Workflow;

    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;

    if draft.status != "confirmed" {
        return Err(ApiError::BadRequest(format!(
            "Only confirmed drafts can be materialized, current status is '{}'",
            draft.status
        )));
    }

    let now = chrono::Utc::now();
    let workflow_id = Uuid::new_v4().to_string();

    let requirement_summary = draft
        .requirement_summary
        .clone()
        .unwrap_or_default();

    let initial_goal = match (draft.requirement_summary.as_ref(), draft.technical_spec.as_ref()) {
        (Some(summary), Some(spec)) => {
            Some(format!("{summary}\n\n---\n\nTechnical Specification:\n{spec}"))
        }
        (Some(summary), None) => Some(summary.clone()),
        (None, Some(spec)) => Some(spec.clone()),
        (None, None) => None,
    };

    // Use the first user-configured model for merge terminal defaults.
    // Fallback to an official Claude Code model if no user models exist.
    let (default_cli_id, default_model_id) =
        db::models::ModelConfig::first_user_configured_ids(&deployment.db().pool)
            .await
            .ok()
            .flatten()
            .unwrap_or_else(|| ("cli-claude-code".to_string(), "model-claude-sonnet".to_string()));

    let mut workflow = Workflow {
        id: workflow_id.clone(),
        project_id: draft.project_id,
        name: if draft.name.is_empty() {
            "Orchestrated Workflow".to_string()
        } else {
            draft.name.clone()
        },
        description: Some(requirement_summary.clone()),
        status: "created".to_string(),
        execution_mode: "agent_planned".to_string(),
        initial_goal,
        use_slash_commands: false,
        orchestrator_enabled: true,
        orchestrator_api_type: draft.planner_api_type.clone(),
        orchestrator_base_url: draft.planner_base_url.clone(),
        orchestrator_api_key: None,
        orchestrator_model: draft.planner_model_id.clone(),
        error_terminal_enabled: false,
        error_terminal_cli_id: None,
        error_terminal_model_id: None,
        merge_terminal_cli_id: default_cli_id,
        merge_terminal_model_id: default_model_id,
        target_branch: "main".to_string(),
        git_watcher_enabled: true,
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
        pause_reason: None,
    };

    let decrypted_key = draft.get_api_key()
        .map_err(|e| ApiError::Internal(format!("Failed to decrypt planner API key: {e}")))?;
    if let Some(ref api_key) = decrypted_key {
        workflow.set_api_key(api_key).map_err(|e| ApiError::Internal(format!("Failed to encrypt API key: {e}")))?;
    }

    Workflow::create(&deployment.db().pool, &workflow)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create workflow: {e}")))?;

    PlanningDraft::set_materialized(&deployment.db().pool, &draft_id, &workflow_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to update draft: {e}")))?;

    tracing::info!(
        draft_id = %draft_id,
        workflow_id = %workflow_id,
        "materialized planning draft into workflow"
    );

    // Auto-prepare and auto-start the workflow so the orchestrator begins
    // immediately — the user should not need to manually call prepare+start.
    {
        let wf_uuid = uuid::Uuid::parse_str(&workflow_id)
            .map_err(|e| ApiError::Internal(format!("Invalid workflow UUID: {e}")))?;
        let dep = deployment.clone();
        tokio::spawn(async move {
            match crate::routes::workflows::auto_prepare_and_start(dep, &wf_uuid.to_string()).await {
                Ok(()) => tracing::info!(workflow_id = %wf_uuid, "Auto-started materialized workflow"),
                Err(e) => tracing::warn!(workflow_id = %wf_uuid, error = ?e, "Failed to auto-start workflow"),
            }
        });
    }

    Ok(Json(ApiResponse::success(MaterializeResponse {
        draft_id,
        workflow_id,
        status: "materialized".to_string(),
    })))
}
