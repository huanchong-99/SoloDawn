//! Quality gate REST API routes.
//!
//! Exposes quality run and issue data for frontend display.
//! - GET /workflows/:id/quality/runs       — list quality runs for a workflow
//! - GET /quality/runs/:run_id             — single quality run by ID
//! - GET /quality/runs/:run_id/issues      — issues for a quality run
//! - GET /terminals/:id/quality/latest     — latest quality run for a terminal

use axum::{
    Json, Router,
    extract::{Path, State},
    routing::get,
};
use deployment::Deployment;
use serde::Serialize;
use ts_rs::TS;
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError};

/// Summary response for a quality run, tailored for list views.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityRunSummary {
    pub id: String,
    pub workflow_id: String,
    pub task_id: Option<String>,
    pub terminal_id: Option<String>,
    pub commit_hash: Option<String>,
    pub gate_level: String,
    pub gate_status: String,
    pub mode: String,
    pub total_issues: i32,
    pub blocking_issues: i32,
    pub new_issues: i32,
    pub duration_ms: i32,
    pub error_message: Option<String>,
    pub created_at: String,
    pub completed_at: Option<String>,
}

impl From<db::models::QualityRun> for QualityRunSummary {
    fn from(r: db::models::QualityRun) -> Self {
        Self {
            id: r.id,
            workflow_id: r.workflow_id,
            task_id: r.task_id,
            terminal_id: r.terminal_id,
            commit_hash: r.commit_hash,
            gate_level: r.gate_level,
            gate_status: r.gate_status,
            mode: r.mode,
            total_issues: r.total_issues,
            blocking_issues: r.blocking_issues,
            new_issues: r.new_issues,
            duration_ms: r.duration_ms,
            error_message: r.error_message,
            created_at: r.created_at.to_rfc3339(),
            completed_at: r.completed_at.map(|t| t.to_rfc3339()),
        }
    }
}

/// Detail response for a single quality run, includes report JSON.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityRunDetail {
    #[serde(flatten)]
    pub summary: QualityRunSummary,
    pub providers_run: Option<serde_json::Value>,
    pub report_json: Option<serde_json::Value>,
    pub decision_json: Option<serde_json::Value>,
}

impl From<db::models::QualityRun> for QualityRunDetail {
    fn from(r: db::models::QualityRun) -> Self {
        let providers = r
            .providers_run
            .as_deref()
            .and_then(|s| serde_json::from_str(s).ok());
        let report = r
            .report_json
            .as_deref()
            .and_then(|s| serde_json::from_str(s).ok());
        let decision = r
            .decision_json
            .as_deref()
            .and_then(|s| serde_json::from_str(s).ok());
        Self {
            summary: QualityRunSummary::from(db::models::QualityRun {
                providers_run: None,
                report_json: None,
                decision_json: None,
                ..r
            }),
            providers_run: providers,
            report_json: report,
            decision_json: decision,
        }
    }
}

/// GET /workflows/:workflow_id/quality/runs
pub async fn list_quality_runs(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<String>,
) -> Result<Json<ApiResponse<Vec<QualityRunSummary>>>, ApiError> {
    let runs = db::models::QualityRun::find_by_workflow(&deployment.db().pool, &workflow_id)
        .await
        .map_err(ApiError::Database)?;

    let summaries: Vec<QualityRunSummary> = runs.into_iter().map(QualityRunSummary::from).collect();
    Ok(Json(ApiResponse::success(summaries)))
}

/// GET /quality/runs/:run_id
pub async fn get_quality_run(
    State(deployment): State<DeploymentImpl>,
    Path(run_id): Path<String>,
) -> Result<Json<ApiResponse<Option<QualityRunDetail>>>, ApiError> {
    let run = db::models::QualityRun::find_by_id(&deployment.db().pool, &run_id)
        .await
        .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(run.map(QualityRunDetail::from))))
}

/// GET /quality/runs/:run_id/issues
pub async fn get_quality_issues(
    State(deployment): State<DeploymentImpl>,
    Path(run_id): Path<String>,
) -> Result<Json<ApiResponse<Vec<db::models::QualityIssueRecord>>>, ApiError> {
    let issues = db::models::QualityIssueRecord::find_by_run(&deployment.db().pool, &run_id)
        .await
        .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(issues)))
}

/// GET /terminals/:terminal_id/quality/latest
pub async fn get_terminal_latest_quality(
    State(deployment): State<DeploymentImpl>,
    Path(terminal_id): Path<String>,
) -> Result<Json<ApiResponse<Option<QualityRunSummary>>>, ApiError> {
    let run = db::models::QualityRun::find_latest_by_terminal(&deployment.db().pool, &terminal_id)
        .await
        .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(run.map(QualityRunSummary::from))))
}

/// Quality routes nested under /workflows
pub fn quality_workflow_routes() -> Router<DeploymentImpl> {
    Router::new().route("/{workflow_id}/quality/runs", get(list_quality_runs))
}

/// Quality routes at /quality
pub fn quality_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/runs/{run_id}", get(get_quality_run))
        .route("/runs/{run_id}/issues", get(get_quality_issues))
}

/// Quality routes nested under /terminals
pub fn quality_terminal_routes() -> Router<DeploymentImpl> {
    Router::new().route(
        "/{terminal_id}/quality/latest",
        get(get_terminal_latest_quality),
    )
}
