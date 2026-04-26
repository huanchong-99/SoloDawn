use axum::{Extension, extract::State, http::StatusCode, response::Json};
use db::models::{feishu_config::FeishuAppConfig, system_settings::SystemSetting};
use deployment::Deployment;
use serde_json::{Value, json};
use utils::response::ApiResponse;

use crate::{DeploymentImpl, feishu_handle::SharedFeishuHandle};

pub async fn health_check() -> Json<ApiResponse<String>> {
    Json(ApiResponse::success("OK".to_string()))
}

/// Liveness probe — stateless, always returns 200.
pub async fn healthz() -> Json<Value> {
    Json(json!({ "ok": true }))
}

/// Readiness probe — checks DB connectivity, asset dir, temp dir, and
/// optional Feishu integration health.
pub async fn readyz(
    State(deployment): State<DeploymentImpl>,
    Extension(feishu_handle): Extension<SharedFeishuHandle>,
) -> (StatusCode, Json<Value>) {
    let db_ok = sqlx::query("SELECT 1")
        .fetch_one(&deployment.db().pool)
        .await
        .is_ok();
    let asset_ok = utils::assets::asset_dir()
        .map(|p| p.exists())
        .unwrap_or(false);
    let temp_ok = {
        let dir = utils::path::get_solodawn_temp_dir();
        std::fs::create_dir_all(&dir).is_ok() && dir.exists()
    };

    // Feishu integration status (non-blocking, does not affect overall readiness)
    let feishu_status = resolve_feishu_health(&deployment, &feishu_handle).await;

    let ready = db_ok && asset_ok && temp_ok;
    let status = if ready {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    };
    (
        status,
        Json(json!({
            "ready": ready,
            "feishu": feishu_status,
        })),
    )
}

/// Resolve Feishu health information for the readiness probe.
///
/// G32-015: Queries the actual WebSocket connection status from the shared
/// `FeishuHandle` instead of hardcoding "disconnected".
///
/// Returns a JSON object with `enabled` and `connectionStatus` fields.
/// This is informational only and does not gate overall readiness.
async fn resolve_feishu_health(
    deployment: &DeploymentImpl,
    feishu_handle: &SharedFeishuHandle,
) -> Value {
    let feature_enabled = SystemSetting::is_feishu_enabled(&deployment.db().pool).await;

    if !feature_enabled {
        return json!({ "enabled": false, "connectionStatus": "disabled" });
    }

    let config = FeishuAppConfig::find_enabled(&deployment.db().pool).await;
    match config {
        Ok(Some(_)) => {
            // G32-015: Query actual connection status from the shared handle.
            let handle_guard = feishu_handle.read().await;
            let conn_status = match &*handle_guard {
                Some(h) => {
                    if *h.connected.read().await {
                        "connected"
                    } else {
                        "disconnected"
                    }
                }
                None => "disconnected",
            };
            json!({
                "enabled": true,
                "connectionStatus": conn_status,
            })
        }
        Ok(None) => json!({ "enabled": true, "connectionStatus": "not_configured" }),
        Err(_) => json!({ "enabled": true, "connectionStatus": "unknown" }),
    }
}
