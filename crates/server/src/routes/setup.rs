use axum::{
    Json, Router,
    extract::State,
    routing::{get, post},
};
use db::models::{project::Project, system_settings::SystemSetting};
use deployment::Deployment;
use serde_json::{Value, json};
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError};

pub fn router() -> Router<DeploymentImpl> {
    Router::new()
        .route("/setup/status", get(get_status))
        .route("/setup/complete", post(mark_complete))
}

async fn get_status(
    State(deployment): State<DeploymentImpl>,
) -> Result<Json<ApiResponse<Value>>, ApiError> {
    let pool = &deployment.db().pool;
    let setup_complete = SystemSetting::get_bool(pool, "setup_complete")
        .await
        .unwrap_or(false);
    let has_project = Project::count(pool).await.unwrap_or(0) > 0;

    let config = deployment.config().read().await;
    let has_model = config
        .workflow_model_library
        .iter()
        .any(|item| !item.model_id.trim().is_empty());

    Ok(Json(ApiResponse::success(json!({
        "complete": setup_complete,
        "checks": {
            "hasModelConfig": has_model,
            "hasProject": has_project
        }
    }))))
}

async fn mark_complete(
    State(deployment): State<DeploymentImpl>,
) -> Result<Json<ApiResponse<Value>>, ApiError> {
    let pool = &deployment.db().pool;
    SystemSetting::set(pool, "setup_complete", "true")
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to mark setup complete: {e}")))?;
    Ok(Json(ApiResponse::success(json!({ "complete": true }))))
}
