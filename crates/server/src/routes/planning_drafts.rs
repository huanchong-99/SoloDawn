//! Planning draft API for orchestrated workspace mode.

use std::collections::HashMap;

use axum::{
    Json, Router,
    extract::{Path, Query, State},
    response::Json as ResponseJson,
    routing::{get, post, put},
};
use db::models::planning_draft::{PlanningDraft, PlanningDraftMessage, PLANNING_DRAFT_STATUSES};
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use services::services::orchestrator::{
    LLMMessage, OrchestratorConfig, create_llm_client,
    config::{PromptProfile, system_prompt_for_profile},
};
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

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
    let project_id_str = params
        .get("project_id")
        .ok_or_else(|| ApiError::BadRequest("project_id is required".to_string()))?;
    let project_id = Uuid::parse_str(project_id_str)
        .map_err(|_| ApiError::BadRequest("project_id must be a valid UUID".to_string()))?;

    let drafts = PlanningDraft::find_by_project(&deployment.db().pool, project_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;

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

    // 1. Store user message
    let user_msg = PlanningDraftMessage::new(&draft_id, "user", req.message.trim());
    PlanningDraftMessage::insert(&deployment.db().pool, &user_msg)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to save message: {e}")))?;

    let mut result = vec![MessageResponse::from(user_msg)];

    // 2. Try to call LLM and store assistant reply
    if let Some(llm_client) = build_llm_client_from_draft(&draft) {
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
            }
            Err(e) => {
                tracing::warn!(draft_id = %draft_id, "LLM call failed for planning draft: {e}");
            }
        }
    } else {
        tracing::debug!(
            draft_id = %draft_id,
            "No LLM config on planning draft, skipping assistant reply"
        );
    }

    Ok(Json(ApiResponse::success(result)))
}

/// Build an LLM client from the draft's planner configuration.
/// Returns `None` when required fields (api_type, base_url, api_key, model) are missing.
fn build_llm_client_from_draft(
    draft: &PlanningDraft,
) -> Option<Box<dyn services::services::orchestrator::LLMClient>> {
    let decrypted_key = match draft.get_api_key() {
        Ok(key) => key,
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
    )?;
    match create_llm_client(&config) {
        Ok(client) => Some(client),
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
        initial_goal: Some(requirement_summary),
        use_slash_commands: false,
        orchestrator_enabled: true,
        orchestrator_api_type: draft.planner_api_type.clone(),
        orchestrator_base_url: draft.planner_base_url.clone(),
        orchestrator_api_key: None,
        orchestrator_model: draft.planner_model_id.clone(),
        error_terminal_enabled: false,
        error_terminal_cli_id: None,
        error_terminal_model_id: None,
        merge_terminal_cli_id: String::new(),
        merge_terminal_model_id: String::new(),
        target_branch: "main".to_string(),
        git_watcher_enabled: true,
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
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

    Ok(Json(ApiResponse::success(MaterializeResponse {
        draft_id,
        workflow_id,
        status: "materialized".to_string(),
    })))
}
