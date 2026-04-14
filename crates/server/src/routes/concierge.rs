//! Concierge Agent REST API routes.
//!
//! Provides session management and message handling for the Concierge Agent.

use std::sync::Arc;

use axum::{
    Extension, Json, Router,
    extract::{Path, Query, State},
    response::Json as ResponseJson,
    routing::{delete, get, post},
};
use db::models::concierge::{ConciergeMessage, ConciergeSession, ConciergeSessionChannel};
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use services::services::concierge::ConciergeAgent;
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError, feishu_handle::SharedFeishuHandle};

pub type SharedConciergeAgent = Arc<ConciergeAgent>;

// ============================================================================
// Route Definition
// ============================================================================

pub fn concierge_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/sessions", post(create_session))
        .route("/sessions", get(list_sessions))
        // Static route before {id} wildcard
        .route(
            "/sessions/feishu-channel",
            get(get_feishu_channel).post(switch_feishu_channel),
        )
        .route("/sessions/{id}", get(get_session).delete(delete_session))
        .route("/sessions/{id}/messages", post(send_message))
        .route("/sessions/{id}/messages", get(list_messages))
        .route("/sessions/{id}/channels", post(add_channel))
        .route(
            "/sessions/{id}/channels/{channel_id}",
            delete(remove_channel),
        )
        .route("/sessions/{id}/settings", post(update_settings))
}

// ============================================================================
// Request/Response Types
// ============================================================================

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateSessionRequest {
    pub name: Option<String>,
    pub llm_model_id: Option<String>,
    pub llm_api_type: Option<String>,
    pub llm_base_url: Option<String>,
    pub llm_api_key: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SendMessageRequest {
    pub message: String,
    pub source: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AddChannelRequest {
    pub provider: String,
    pub external_id: String,
    pub user_identifier: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSettingsRequest {
    pub feishu_sync: Option<bool>,
    pub sync_history: Option<bool>,
    pub progress_notifications: Option<bool>,
    pub sync_tools: Option<bool>,
    pub sync_terminal: Option<bool>,
    pub sync_progress: Option<bool>,
    pub notify_on_completion: Option<bool>,
    pub llm_model_id: Option<String>,
    pub llm_api_type: Option<String>,
    pub llm_base_url: Option<String>,
    pub llm_api_key: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SendMessageResponse {
    pub assistant_message: String,
    pub messages: Vec<ConciergeMessage>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ListMessagesQuery {
    pub cursor: Option<usize>,
    pub limit: Option<usize>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SwitchFeishuChannelRequest {
    pub session_id: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FeishuChannelStatus {
    pub active_session_id: Option<String>,
    pub active_session_name: Option<String>,
    pub chat_id: Option<String>,
}

// ============================================================================
// Handlers
// ============================================================================

// TODO(W2-18-09): No rate limit on concierge session creation or on the
// downstream LLM-backed endpoints in this module. A malicious or misbehaving
// client holding a valid API token could burn concierge LLM quota. Mirror
// the per-principal token-bucket pattern used in workflows.rs
// (`ORCHESTRATOR_RATE_LIMIT_WINDOW` / `ORCHESTRATOR_RATE_LIMIT_MAX_REQUESTS`
// + `ORCHESTRATOR_GOVERNANCE_STATE`) once a shared governance module is
// extracted, then apply it to `create_session` and any message-submit
// handlers in this file.
async fn create_session(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateSessionRequest>,
) -> Result<ResponseJson<ApiResponse<ConciergeSession>>, ApiError> {
    let pool = &deployment.db().pool;
    let name = payload.name.as_deref().unwrap_or("");
    let mut session = ConciergeSession::new(name);

    // Configure LLM if provided
    if let Some(ref api_key) = payload.llm_api_key {
        let encrypted = ConciergeSession::encrypt_api_key(api_key)
            .map_err(|e| ApiError::Internal(format!("Encryption failed: {e}")))?;
        session.llm_api_key_encrypted = Some(encrypted);
    }
    session.llm_model_id = payload.llm_model_id;
    session.llm_api_type = payload.llm_api_type;
    session.llm_base_url = payload.llm_base_url;

    ConciergeSession::insert(pool, &session)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create session: {e}")))?;

    Ok(ResponseJson(ApiResponse::success(session)))
}

async fn list_sessions(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<ConciergeSession>>>, ApiError> {
    let sessions = ConciergeSession::list_all(&deployment.db().pool)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to list sessions: {e}")))?;
    Ok(ResponseJson(ApiResponse::success(sessions)))
}

async fn delete_session(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    ConciergeSession::delete(&deployment.db().pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;
    Ok(ResponseJson(ApiResponse::success(())))
}

async fn get_session(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
) -> Result<ResponseJson<ApiResponse<ConciergeSession>>, ApiError> {
    let session = ConciergeSession::find_by_id(&deployment.db().pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;
    Ok(ResponseJson(ApiResponse::success(session)))
}

async fn send_message(
    State(deployment): State<DeploymentImpl>,
    Extension(concierge): Extension<SharedConciergeAgent>,
    Path(id): Path<String>,
    Json(payload): Json<SendMessageRequest>,
) -> Result<ResponseJson<ApiResponse<SendMessageResponse>>, ApiError> {
    let message = payload.message.trim();
    if message.is_empty() {
        return Err(ApiError::BadRequest("message is required".to_string()));
    }

    let source = payload.source.as_deref().unwrap_or("web");
    let pool = &deployment.db().pool;

    // Verify session exists
    let _session = ConciergeSession::find_by_id(pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;

    let assistant_response = concierge
        .process_message(&id, message, Some(source), None)
        .await
        .map_err(|e| ApiError::Internal(format!("Concierge error: {e}")))?;

    // Return all messages
    let messages = ConciergeMessage::list_by_session(pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;

    Ok(ResponseJson(ApiResponse::success(SendMessageResponse {
        assistant_message: assistant_response,
        messages,
    })))
}

async fn list_messages(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
    Query(query): Query<ListMessagesQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<ConciergeMessage>>>, ApiError> {
    let pool = &deployment.db().pool;
    let cursor = query.cursor.unwrap_or(0);
    let limit = query.limit.unwrap_or(100);

    let messages = ConciergeMessage::list_by_session_paginated(pool, &id, cursor, limit)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;
    Ok(ResponseJson(ApiResponse::success(messages)))
}

async fn add_channel(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
    Json(payload): Json<AddChannelRequest>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;
    // Verify session exists
    let _session = ConciergeSession::find_by_id(pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;

    ConciergeSessionChannel::upsert(
        pool,
        &id,
        &payload.provider,
        &payload.external_id,
        payload.user_identifier.as_deref(),
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to add channel: {e}")))?;

    Ok(ResponseJson(ApiResponse::success(())))
}

async fn remove_channel(
    State(deployment): State<DeploymentImpl>,
    Path((id, channel_id)): Path<(String, String)>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;

    // Verify the channel belongs to the requested session before deleting.
    let channel = ConciergeSessionChannel::find_by_id(pool, &channel_id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Channel not found".to_string()))?;

    if channel.session_id != id {
        return Err(ApiError::NotFound("Channel not found".to_string()));
    }

    ConciergeSessionChannel::delete_by_id(pool, &channel_id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;
    Ok(ResponseJson(ApiResponse::success(())))
}

async fn update_settings(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
    Path(id): Path<String>,
    Json(payload): Json<UpdateSettingsRequest>,
) -> Result<ResponseJson<ApiResponse<ConciergeSession>>, ApiError> {
    let pool = &deployment.db().pool;
    let session = ConciergeSession::find_by_id(pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;

    if let Some(feishu_sync) = payload.feishu_sync {
        ConciergeSession::update_feishu_sync(pool, &id, feishu_sync)
            .await
            .map_err(|e| ApiError::Internal(format!("{e}")))?;

        // When enabling feishu sync with sync_history, push existing messages to Feishu
        if feishu_sync && payload.sync_history.unwrap_or(false) {
            let handle_guard = feishu_handle.read().await;
            if let Some(ref h) = *handle_guard {
                if *h.connected.read().await {
                    let messenger = h.messenger.clone();
                    // Resolve chat_id: session's own → last received → DB binding → bot chat list
                    let last_id = h.last_chat_id.try_read().ok().and_then(|g| g.clone());
                    drop(handle_guard);

                    let chat_id = if let Some(ref cid) = session.feishu_chat_id {
                        Some(cid.clone())
                    } else if let Some(cid) = last_id {
                        Some(cid)
                    } else if let Some(b) = db::models::ExternalConversationBinding::find_latest_active(
                        pool, "feishu",
                    ).await.ok().flatten() {
                        Some(b.conversation_id)
                    } else {
                        messenger.first_bot_chat_id().await.unwrap_or(None)
                    };

                    if let Some(chat_id) = chat_id {
                        let messages = ConciergeMessage::list_by_session(pool, &id)
                            .await
                            .unwrap_or_default();
                        let session_name = session.name.clone();
                        tokio::spawn(async move {
                            if let Err(e) = messenger
                                .send_text(
                                    &chat_id,
                                    &format!(
                                        "[Concierge: {}] Syncing conversation history...",
                                        session_name
                                    ),
                                )
                                .await
                            {
                                tracing::warn!("Failed to send Feishu history header: {e}");
                                return;
                            }
                            for msg in &messages {
                                let prefix = match msg.role.as_str() {
                                    "user" => "[User]",
                                    "assistant" => "[Assistant]",
                                    "tool_call" => "[Tool Call]",
                                    "tool_result" => "[Tool Result]",
                                    _ => "[System]",
                                };
                                let text = format!("{prefix} {}", msg.content);
                                let truncated = if text.len() > 4000 {
                                    let boundary = text.floor_char_boundary(4000);
                                    format!("{}...(truncated)", &text[..boundary])
                                } else {
                                    text
                                };
                                if let Err(e) =
                                    messenger.send_text(&chat_id, &truncated).await
                                {
                                    tracing::warn!(
                                        "Failed to push concierge message to Feishu: {e}"
                                    );
                                    break;
                                }
                            }
                            tracing::info!(
                                "Feishu history sync complete: {} messages sent",
                                messages.len()
                            );
                        });
                    }
                } else {
                    drop(handle_guard);
                }
            }
        }
    }
    if let Some(progress) = payload.progress_notifications {
        ConciergeSession::update_progress_notifications(pool, &id, progress)
            .await
            .map_err(|e| ApiError::Internal(format!("{e}")))?;
    }
    // Update sync toggles if any are provided
    if payload.sync_tools.is_some()
        || payload.sync_terminal.is_some()
        || payload.sync_progress.is_some()
        || payload.notify_on_completion.is_some()
    {
        // Reload current values to merge partial updates
        let current = ConciergeSession::find_by_id(pool, &id)
            .await
            .map_err(|e| ApiError::Internal(format!("{e}")))?
            .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;
        ConciergeSession::update_sync_toggles(
            pool,
            &id,
            payload.sync_tools.unwrap_or(current.sync_tools),
            payload.sync_terminal.unwrap_or(current.sync_terminal),
            payload.sync_progress.unwrap_or(current.sync_progress),
            payload.notify_on_completion.unwrap_or(current.notify_on_completion),
        )
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;
    }
    if payload.llm_api_key.is_some() || payload.llm_model_id.is_some() {
        let encrypted_key = match &payload.llm_api_key {
            Some(key) => Some(
                ConciergeSession::encrypt_api_key(key)
                    .map_err(|e| ApiError::Internal(format!("Encryption failed: {e}")))?,
            ),
            None => None,
        };
        ConciergeSession::update_llm_config(
            pool,
            &id,
            payload.llm_model_id.as_deref(),
            payload.llm_api_type.as_deref(),
            payload.llm_base_url.as_deref(),
            encrypted_key.as_deref(),
        )
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;
    }

    let updated = ConciergeSession::find_by_id(pool, &id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;

    Ok(ResponseJson(ApiResponse::success(updated)))
}

async fn get_feishu_channel(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
) -> Result<ResponseJson<ApiResponse<FeishuChannelStatus>>, ApiError> {
    let pool = &deployment.db().pool;

    // Find active channel binding for provider "feishu"
    let active = sqlx::query_as::<_, ConciergeSessionChannel>(
        "SELECT * FROM concierge_session_channel WHERE provider = 'feishu' AND is_active = 1 LIMIT 1",
    )
    .fetch_optional(pool)
    .await
    .map_err(|e| ApiError::Internal(format!("{e}")))?;

    let (active_session_id, active_session_name) = if let Some(ref ch) = active {
        let session = ConciergeSession::find_by_id(pool, &ch.session_id)
            .await
            .map_err(|e| ApiError::Internal(format!("{e}")))?;
        (
            Some(ch.session_id.clone()),
            session.map(|s| s.name),
        )
    } else {
        (None, None)
    };

    // Get chat_id: session's feishu_chat_id → feishu handle last_chat_id
    let session_chat_id = if let Some(ref ch) = active {
        ConciergeSession::find_by_id(pool, &ch.session_id)
            .await
            .ok()
            .flatten()
            .and_then(|s| s.feishu_chat_id)
    } else {
        None
    };
    let chat_id = if session_chat_id.is_some() {
        session_chat_id
    } else {
        let handle_guard = feishu_handle.read().await;
        if let Some(ref h) = *handle_guard {
            h.last_chat_id.try_read().ok().and_then(|g| g.clone())
        } else {
            None
        }
    };

    Ok(ResponseJson(ApiResponse::success(FeishuChannelStatus {
        active_session_id,
        active_session_name,
        chat_id,
    })))
}

async fn switch_feishu_channel(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
    Json(payload): Json<SwitchFeishuChannelRequest>,
) -> Result<ResponseJson<ApiResponse<FeishuChannelStatus>>, ApiError> {
    let pool = &deployment.db().pool;
    let session_id = &payload.session_id;

    // Verify the session exists
    let session = ConciergeSession::find_by_id(pool, session_id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;

    // Resolve chat_id: session's feishu_chat_id → last_chat_id → DB binding
    let last_chat_id = {
        let handle_guard = feishu_handle.read().await;
        if let Some(ref h) = *handle_guard {
            h.last_chat_id.try_read().ok().and_then(|g| g.clone())
        } else {
            None
        }
    };

    let chat_id = if let Some(ref cid) = session.feishu_chat_id {
        cid.clone()
    } else if let Some(cid) = last_chat_id {
        cid
    } else if let Some(b) =
        db::models::ExternalConversationBinding::find_latest_active(pool, "feishu")
            .await
            .ok()
            .flatten()
    {
        b.conversation_id
    } else {
        return Err(ApiError::BadRequest(
            "No Feishu chat_id available. Send a message in Feishu first.".to_string(),
        ));
    };

    // Switch active session for this channel
    ConciergeSessionChannel::switch_active_session(pool, "feishu", &chat_id, session_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to switch channel: {e}")))?;

    // Enable feishu sync on the target session
    ConciergeSession::update_feishu_sync(pool, session_id, true)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;

    // Store the chat_id on the session
    ConciergeSession::update_feishu_chat_id(pool, session_id, &chat_id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?;

    Ok(ResponseJson(ApiResponse::success(FeishuChannelStatus {
        active_session_id: Some(session_id.clone()),
        active_session_name: Some(session.name),
        chat_id: Some(chat_id),
    })))
}
