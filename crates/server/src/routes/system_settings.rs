use axum::{Json, Router, extract::State, routing::get};
use db::models::system_settings::SystemSetting;
use deployment::Deployment;
use serde::Deserialize;
use serde_json::{Value, json};
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError};

#[derive(Deserialize)]
struct UpdateSettings {
    feishu_enabled: Option<bool>,
}

pub fn router() -> Router<DeploymentImpl> {
    Router::new().route("/system-settings", get(get_settings).put(update_settings))
}

async fn get_settings(
    State(deployment): State<DeploymentImpl>,
) -> Result<Json<ApiResponse<Value>>, ApiError> {
    let pool = &deployment.db().pool;
    let settings = SystemSetting::find_all(pool)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to query system settings: {e}")))?;

    let mut map = serde_json::Map::new();
    for s in settings {
        map.insert(s.key, json!(s.value));
    }
    Ok(Json(ApiResponse::success(Value::Object(map))))
}

async fn update_settings(
    State(deployment): State<DeploymentImpl>,
    Json(body): Json<UpdateSettings>,
) -> Result<Json<ApiResponse<Value>>, ApiError> {
    let pool = &deployment.db().pool;
    if let Some(enabled) = body.feishu_enabled {
        SystemSetting::set(
            pool,
            "feishu_enabled",
            if enabled { "true" } else { "false" },
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to update feishu_enabled: {e}")))?;
    }
    // Return updated settings
    get_settings(State(deployment)).await
}
