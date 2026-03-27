//! Feishu (Lark) integration management API routes.
//!
//! Provides configuration management and status monitoring:
//! - GET  /api/integrations/feishu/status       — Connection status
//! - PUT  /api/integrations/feishu/config        — Update configuration
//! - POST /api/integrations/feishu/reconnect     — Trigger reconnection
//! - POST /api/integrations/feishu/test-send     — Send a test message
//! - POST /api/integrations/feishu/test-receive  — Wait for an incoming message

use axum::{
    Extension, Json, Router,
    extract::State,
    routing::{get, post, put},
};
use chrono::Utc;
use db::models::{feishu_config::FeishuAppConfig, system_settings::SystemSetting};
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use sqlx::SqlitePool;
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError, feishu_handle::SharedFeishuHandle};

/// Whether the Feishu integration feature is enabled (env var takes precedence, then database).
async fn is_feishu_enabled(pool: &SqlitePool) -> bool {
    SystemSetting::is_feishu_enabled(pool).await
}

// ---------------------------------------------------------------------------
// Response / request types
// ---------------------------------------------------------------------------

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FeishuStatusResponse {
    /// Whether the feature flag is enabled
    pub feature_enabled: bool,
    /// Whether a config record exists and is marked enabled
    pub config_enabled: bool,
    /// Connection state: "connected", "disconnected", or "not_configured"
    pub connection_status: String,
    /// Summary of the active configuration (app_id + base_url), if any
    pub config_summary: Option<FeishuConfigSummary>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FeishuConfigSummary {
    pub id: String,
    pub app_id: String,
    pub base_url: String,
    pub tenant_key: Option<String>,
    pub enabled: bool,
    pub updated_at: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateFeishuConfigRequest {
    pub app_id: String,
    pub app_secret: String,
    pub tenant_key: Option<String>,
    pub base_url: Option<String>,
    pub enabled: Option<bool>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateFeishuConfigResponse {
    pub id: String,
    pub message: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ReconnectResponse {
    pub status: String,
    pub message: String,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn config_to_summary(cfg: &FeishuAppConfig) -> FeishuConfigSummary {
    FeishuConfigSummary {
        id: cfg.id.clone(),
        app_id: cfg.app_id.clone(),
        base_url: cfg.base_url.clone(),
        tenant_key: cfg.tenant_key.clone(),
        enabled: cfg.enabled,
        updated_at: cfg.updated_at.to_rfc3339(),
    }
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

/// GET /api/integrations/feishu/status
///
/// Returns the current Feishu integration status including feature flag,
/// config state, and connection status.
async fn get_status(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
) -> Result<Json<ApiResponse<FeishuStatusResponse>>, ApiError> {
    let feature_enabled = is_feishu_enabled(&deployment.db().pool).await;

    let config = FeishuAppConfig::find_enabled(&deployment.db().pool)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to query feishu config: {e}")))?;

    let (config_enabled, connection_status, config_summary) = match config {
        Some(ref cfg) if cfg.enabled => {
            let handle_guard = feishu_handle.read().await;
            let status = match &*handle_guard {
                Some(h) => {
                    if *h.connected.read().await {
                        "connected"
                    } else {
                        "disconnected"
                    }
                }
                None => "disconnected",
            };
            (true, status.to_string(), Some(config_to_summary(cfg)))
        }
        Some(ref cfg) => (false, "not_configured".to_string(), Some(config_to_summary(cfg))),
        None => (false, "not_configured".to_string(), None),
    };

    Ok(Json(ApiResponse::success(FeishuStatusResponse {
        feature_enabled,
        config_enabled,
        connection_status,
        config_summary,
    })))
}

/// PUT /api/integrations/feishu/config
///
/// Creates or updates the Feishu app configuration. Only one active config
/// is supported — an upsert on the first record found, or a fresh insert.
async fn update_config(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<UpdateFeishuConfigRequest>,
) -> Result<Json<ApiResponse<UpdateFeishuConfigResponse>>, ApiError> {
    if !is_feishu_enabled(&deployment.db().pool).await {
        return Err(ApiError::Conflict(
            "Feishu integration is disabled. Enable it via system settings or set SOLODAWN_FEISHU_ENABLED=true.".to_string(),
        ));
    }

    if payload.app_id.trim().is_empty() || payload.app_secret.trim().is_empty() {
        return Err(ApiError::BadRequest(
            "app_id and app_secret are required".to_string(),
        ));
    }

    let base_url = payload
        .base_url
        .as_deref()
        .unwrap_or("https://open.feishu.cn")
        .to_string();
    let enabled = payload.enabled.unwrap_or(true);
    let encrypted_secret = FeishuAppConfig::encrypt_secret(&payload.app_secret)
        .map_err(|e| ApiError::Internal(format!("Failed to encrypt app secret: {e}")))?;

    let pool = &deployment.db().pool;

    // G32-007: Use find_first() instead of find_enabled() so that a disabled
    // config can still be found and updated (upsert semantics).
    let existing = FeishuAppConfig::find_first(pool)
        .await
        .map_err(|e| ApiError::Internal(format!("DB query failed: {e}")))?;

    let config_id = if let Some(existing) = existing {
        FeishuAppConfig::update_credentials(
            pool,
            &existing.id,
            &payload.app_id,
            &encrypted_secret,
            &base_url,
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to update feishu config: {e}")))?;

        // Update tenant_key if provided
        if let Some(ref tenant_key) = payload.tenant_key {
            sqlx::query(
                "UPDATE feishu_app_config SET tenant_key = ?2, updated_at = datetime('now') WHERE id = ?1",
            )
            .bind(&existing.id)
            .bind(tenant_key)
            .execute(pool)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to update tenant_key: {e}")))?;
        }

        FeishuAppConfig::update_enabled(pool, &existing.id, enabled)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to update enabled flag: {e}")))?;

        existing.id
    } else {
        let mut new_config = FeishuAppConfig::new(&payload.app_id, &encrypted_secret, &base_url);
        new_config.tenant_key = payload.tenant_key.clone();
        new_config.enabled = enabled;
        new_config.created_at = Utc::now();
        new_config.updated_at = Utc::now();

        FeishuAppConfig::insert(pool, &new_config)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to insert feishu config: {e}")))?;

        new_config.id
    };

    Ok(Json(ApiResponse::success(UpdateFeishuConfigResponse {
        id: config_id,
        message: "Feishu configuration updated successfully".to_string(),
    })))
}

/// POST /api/integrations/feishu/reconnect
///
/// Triggers a manual reconnection of the Feishu WebSocket client.
async fn reconnect(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
) -> Result<Json<ApiResponse<ReconnectResponse>>, ApiError> {
    if !is_feishu_enabled(&deployment.db().pool).await {
        return Err(ApiError::Conflict(
            "Feishu integration is disabled".to_string(),
        ));
    }

    let config = FeishuAppConfig::find_enabled(&deployment.db().pool)
        .await
        .map_err(|e| ApiError::Internal(format!("DB query failed: {e}")))?;

    if config.is_none() {
        return Err(ApiError::BadRequest(
            "No enabled Feishu configuration found. Configure via PUT /api/integrations/feishu/config first.".to_string(),
        ));
    }

    let handle_guard = feishu_handle.read().await;
    if let Some(ref h) = *handle_guard {
        if let Err(e) = h.reconnect_tx.try_send(()) {
            tracing::warn!(error = %e, "Failed to send Feishu reconnect signal");
            drop(handle_guard);
            // G32-009: Distinguish channel-full (a reconnect is already in
            // progress) from other send failures so the caller gets an
            // actionable status code instead of a misleading 200 OK.
            return match e {
                tokio::sync::mpsc::error::TrySendError::Full(()) => Err(ApiError::Conflict(
                    "A reconnect is already in progress. Please wait and try again.".to_string(),
                )),
                tokio::sync::mpsc::error::TrySendError::Closed(()) => Err(ApiError::Internal(
                    "Feishu reconnect channel is closed. The connector may have shut down.".to_string(),
                )),
            };
        }
    } else {
        return Err(ApiError::Conflict(
            "Feishu connector is not running. Restart the server with SOLODAWN_FEISHU_ENABLED=true.".to_string(),
        ));
    }
    drop(handle_guard);

    tracing::info!("Feishu reconnect requested via API");

    Ok(Json(ApiResponse::success(ReconnectResponse {
        status: "reconnecting".to_string(),
        message: "Reconnect signal sent to Feishu connector.".to_string(),
    })))
}

// ---------------------------------------------------------------------------
// Test request / response types
// ---------------------------------------------------------------------------

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TestSendRequest {
    pub chat_id: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TestResultResponse {
    pub success: bool,
    pub message: String,
}

// ---------------------------------------------------------------------------
// Test handlers
// ---------------------------------------------------------------------------

/// POST /api/integrations/feishu/test-send
///
/// Sends a test message ("Hello from SoloDawn") to the specified chat, or
/// to the most recently active conversation binding.
async fn test_send(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
    Json(payload): Json<TestSendRequest>,
) -> Result<Json<ApiResponse<TestResultResponse>>, ApiError> {
    if !is_feishu_enabled(&deployment.db().pool).await {
        return Err(ApiError::Conflict(
            "Feishu integration is disabled".to_string(),
        ));
    }

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
    drop(handle_guard);

    // Resolve chat_id: request param → last received → DB binding
    let handle_guard2 = feishu_handle.read().await;
    let last_id = handle_guard2.as_ref().and_then(|h| {
        // Try read without blocking — use try_read
        h.last_chat_id.try_read().ok().and_then(|g| g.clone())
    });
    drop(handle_guard2);

    let chat_id = if let Some(ref id) = payload.chat_id {
        id.clone()
    } else if let Some(id) = last_id {
        id
    } else {
        let binding = db::models::ExternalConversationBinding::find_latest_active(
            &deployment.db().pool,
            "feishu",
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to query bindings: {e}")))?;

        match binding {
            Some(b) => b.conversation_id,
            None => {
                return Ok(Json(ApiResponse::success(TestResultResponse {
                    success: false,
                    message: "No recent chat found. Please first click 'Receive Test', then send a message to the bot in Feishu.".to_string(),
                })));
            }
        }
    };

    match messenger.send_text(&chat_id, "Hello from SoloDawn / 你好，来自 SoloDawn 的测试消息").await {
        Ok(_) => Ok(Json(ApiResponse::success(TestResultResponse {
            success: true,
            message: "Test message sent successfully".to_string(),
        }))),
        Err(e) => Ok(Json(ApiResponse::success(TestResultResponse {
            success: false,
            message: format!("Failed to send message: {e}"),
        }))),
    }
}

/// POST /api/integrations/feishu/test-receive
///
/// Waits up to 30 seconds for an incoming message from Feishu.
async fn test_receive(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
) -> Result<Json<ApiResponse<TestResultResponse>>, ApiError> {
    if !is_feishu_enabled(&deployment.db().pool).await {
        return Err(ApiError::Conflict(
            "Feishu integration is disabled".to_string(),
        ));
    }

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

    let mut rx = h.event_tx.subscribe();
    let last_chat_id = h.last_chat_id.clone();
    drop(handle_guard);

    let timeout = tokio::time::Duration::from_secs(30);
    match tokio::time::timeout(timeout, async {
        loop {
            match rx.recv().await {
                Ok(event) => {
                    if let Some(ref header) = event.header {
                        if header.event_type == feishu_connector::events::EVENT_TYPE_MESSAGE {
                            if let Ok(msg) = feishu_connector::events::parse_message_event(&event) {
                                // Save chat_id for test-send
                                *last_chat_id.write().await = Some(msg.chat_id.clone());
                                let text = feishu_connector::events::parse_text_content(&msg.content);
                                return text;
                            }
                        }
                    }
                }
                Err(tokio::sync::broadcast::error::RecvError::Lagged(n)) => {
                    tracing::debug!(lagged = n, "Test-receive subscriber lagged");
                }
                Err(tokio::sync::broadcast::error::RecvError::Closed) => {
                    break;
                }
            }
        }
        String::new()
    })
    .await
    {
        Ok(text) if !text.is_empty() => Ok(Json(ApiResponse::success(TestResultResponse {
            success: true,
            message: format!("Received: {text}"),
        }))),
        Ok(_) => Ok(Json(ApiResponse::success(TestResultResponse {
            success: false,
            message: "Event channel closed unexpectedly".to_string(),
        }))),
        Err(_) => Ok(Json(ApiResponse::success(TestResultResponse {
            success: false,
            message: "No message received within 30 seconds".to_string(),
        }))),
    }
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

pub fn router() -> Router<DeploymentImpl> {
    Router::new()
        .route("/feishu/status", get(get_status))
        .route("/feishu/config", put(update_config))
        .route("/feishu/reconnect", post(reconnect))
        .route("/feishu/test-send", post(test_send))
        .route("/feishu/test-receive", post(test_receive))
}
