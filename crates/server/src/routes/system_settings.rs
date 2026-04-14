use axum::{Extension, Json, Router, extract::State, http::HeaderMap, routing::get};
use db::models::system_settings::SystemSetting;
use deployment::Deployment;
use serde::Deserialize;
use serde_json::{Value, json};
use utils::response::ApiResponse;

use crate::{
    DeploymentImpl,
    error::ApiError,
    middleware::auth::{RequestContext, check_admin},
};

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

// TODO(W2-18-05 / G24): Restrict PUT /system-settings to admin principals.
// Today any bearer of a valid API token can toggle system-level flags
// (e.g. feishu_enabled). Enforcing this requires the auth-layer extension
// planned for G24 (role/claims propagated from the token into request
// extensions); once that is in place, reject non-admin callers here with
// ApiError::Forbidden before mutating.
async fn update_settings(
    State(deployment): State<DeploymentImpl>,
    headers: HeaderMap,
    ctx: Option<Extension<RequestContext>>,
    Json(body): Json<UpdateSettings>,
) -> Result<Json<ApiResponse<Value>>, ApiError> {
    // Defense-in-depth: opt-in admin gate. When SOLODAWN_ADMIN_TOKEN is unset,
    // this is a no-op. When set, require a matching X-Admin-Token header.
    let ctx = ctx.map(|Extension(c)| c).unwrap_or(RequestContext { authenticated: false });
    if check_admin(&ctx, &headers).is_err() {
        return Err(ApiError::Forbidden("admin token required".to_string()));
    }

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
