//! Requirement ledger REST API (评分点账本).
//!
//! Read + pre-confirm curation of the project-scoped acceptance points.
//! Points are created by the confirm-time ledger sync
//! (`requirement_ledger::sync_ledger_from_audit_plan`) and delivered/regressed
//! by the acceptance review — this API only lists them and lets the user edit
//! or remove points that are still `pending`.

use axum::{
    Json, Router,
    extract::{Path, State},
    routing::get,
};
use db::models::requirement_item::RequirementItem;
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RequirementItemResponse {
    pub id: String,
    pub project_id: String,
    pub point_code: String,
    pub text: String,
    pub status: String,
    pub origin_draft_id: Option<String>,
    pub context_capsule: Option<String>,
    pub provenance_workflow_id: Option<String>,
    pub provenance_commits: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    pub delivered_at: Option<String>,
}

impl From<RequirementItem> for RequirementItemResponse {
    fn from(item: RequirementItem) -> Self {
        Self {
            id: item.id,
            project_id: item.project_id.to_string(),
            point_code: item.point_code,
            text: item.text,
            status: item.status,
            origin_draft_id: item.origin_draft_id,
            context_capsule: item.context_capsule,
            provenance_workflow_id: item.provenance_workflow_id,
            provenance_commits: item.provenance_commits,
            created_at: item.created_at.to_rfc3339(),
            updated_at: item.updated_at.to_rfc3339(),
            delivered_at: item.delivered_at.map(|t| t.to_rfc3339()),
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateRequirementItemRequest {
    pub text: String,
}

pub fn requirement_items_project_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/{project_id}/requirement-items", get(list_items))
        .route(
            "/{project_id}/requirement-items/{item_id}",
            axum::routing::put(update_item).delete(delete_item),
        )
}

fn parse_project_id(project_id: &str) -> Result<Uuid, ApiError> {
    Uuid::parse_str(project_id)
        .map_err(|_| ApiError::BadRequest("project_id must be a valid UUID".to_string()))
}

async fn list_items(
    State(deployment): State<DeploymentImpl>,
    Path(project_id): Path<String>,
) -> Result<Json<ApiResponse<Vec<RequirementItemResponse>>>, ApiError> {
    let project_id = parse_project_id(&project_id)?;
    let items = RequirementItem::find_by_project(&deployment.db().pool, project_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;
    Ok(Json(ApiResponse::success(
        items.into_iter().map(RequirementItemResponse::from).collect(),
    )))
}

async fn update_item(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, item_id)): Path<(String, String)>,
    Json(req): Json<UpdateRequirementItemRequest>,
) -> Result<Json<ApiResponse<RequirementItemResponse>>, ApiError> {
    let project_id = parse_project_id(&project_id)?;
    let text = req.text.trim();
    if text.is_empty() {
        return Err(ApiError::BadRequest("text cannot be empty".to_string()));
    }

    let item = RequirementItem::find_by_id(&deployment.db().pool, &item_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Requirement item {item_id} not found")))?;
    if item.project_id != project_id {
        return Err(ApiError::NotFound(format!(
            "Requirement item {item_id} not found in this project"
        )));
    }

    let updated = RequirementItem::update_text_if_pending(&deployment.db().pool, &item_id, text)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;
    if !updated {
        return Err(ApiError::BadRequest(
            "Only pending points can be edited; delivered points are immutable".to_string(),
        ));
    }

    let item = RequirementItem::find_by_id(&deployment.db().pool, &item_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Item disappeared after update".to_string()))?;
    Ok(Json(ApiResponse::success(RequirementItemResponse::from(item))))
}

async fn delete_item(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, item_id)): Path<(String, String)>,
) -> Result<Json<ApiResponse<bool>>, ApiError> {
    let project_id = parse_project_id(&project_id)?;

    let item = RequirementItem::find_by_id(&deployment.db().pool, &item_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Requirement item {item_id} not found")))?;
    if item.project_id != project_id {
        return Err(ApiError::NotFound(format!(
            "Requirement item {item_id} not found in this project"
        )));
    }

    let deleted = RequirementItem::delete_if_pending(&deployment.db().pool, &item_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;
    if !deleted {
        return Err(ApiError::BadRequest(
            "Only pending points can be deleted; delivered points are part of the project's \
             acceptance history"
                .to_string(),
        ));
    }
    Ok(Json(ApiResponse::success(true)))
}
