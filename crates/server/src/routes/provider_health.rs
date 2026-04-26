//! Provider health monitoring API routes.
//!
//! Exposes per-workflow provider status and circuit-breaker reset endpoints.

use axum::{
    Json, Router,
    extract::{Path, State},
    routing::{get, post},
};
use db::models::ModelConfig;
use deployment::Deployment;
use serde::Serialize;
use utils::response::ApiResponse;

use crate::DeploymentImpl;

/// Health status for a single provider.
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderStatus {
    pub name: String,
    pub is_active: bool,
    pub is_dead: bool,
    pub consecutive_failures: u32,
    pub total_requests: u64,
    pub total_failures: u64,
}

/// Aggregated provider health response.
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderHealthResponse {
    pub providers: Vec<ProviderStatus>,
    pub active_provider: String,
}

/// GET /workflows/:workflow_id/providers/status
///
/// Returns provider health information for the given workflow.
/// If the workflow has an active orchestrator agent, returns live
/// circuit-breaker metrics. Otherwise falls back to DB model configs
/// as baseline provider entries.
pub async fn get_provider_status(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<String>,
) -> Json<ApiResponse<ProviderHealthResponse>> {
    // Check if the workflow exists
    let workflow = db::models::Workflow::find_by_id(&deployment.db().pool, &workflow_id).await;
    if workflow.as_ref().is_ok_and(std::option::Option::is_none) {
        return Json(ApiResponse::success(ProviderHealthResponse {
            providers: vec![],
            active_provider: "none".to_string(),
        }));
    }

    // Try to get live data from the running orchestrator agent
    let runtime = deployment.orchestrator_runtime();
    if let Some(reports) = runtime.get_provider_status(&workflow_id).await {
        let active_provider = reports
            .iter()
            .find(|r| r.is_active)
            .map_or_else(|| "none".to_string(), |r| r.name.clone());

        let providers: Vec<ProviderStatus> = reports
            .into_iter()
            .map(|r| ProviderStatus {
                name: r.name,
                is_active: r.is_active,
                is_dead: r.is_dead,
                consecutive_failures: r.consecutive_failures,
                total_requests: r.total_requests,
                total_failures: r.total_failures,
            })
            .collect();

        return Json(ApiResponse::success(ProviderHealthResponse {
            providers,
            active_provider,
        }));
    }

    // No active orchestrator — return configured providers with unknown status
    let configs = ModelConfig::find_all(&deployment.db().pool)
        .await
        .unwrap_or_default();

    let providers: Vec<ProviderStatus> = configs
        .iter()
        .map(|c| ProviderStatus {
            name: c.display_name.clone(),
            is_active: false,
            is_dead: false,
            consecutive_failures: 0,
            total_requests: 0,
            total_failures: 0,
        })
        .collect();

    Json(ApiResponse::success(ProviderHealthResponse {
        providers,
        active_provider: "none".to_string(),
    }))
}

/// POST /workflows/:workflow_id/providers/:provider_name/reset
///
/// Resets the circuit breaker for the named provider in a running workflow.
pub async fn reset_provider(
    State(deployment): State<DeploymentImpl>,
    Path((workflow_id, provider_name)): Path<(String, String)>,
) -> Json<ApiResponse<serde_json::Value>> {
    let runtime = deployment.orchestrator_runtime();

    match runtime.reset_provider(&workflow_id, &provider_name).await {
        Ok(true) => Json(ApiResponse::success(serde_json::json!({
            "status": "ok",
            "provider": provider_name,
            "message": "Circuit breaker reset successfully"
        }))),
        Ok(false) => Json(ApiResponse::success(serde_json::json!({
            "status": "not_found",
            "provider": provider_name,
            "message": "Provider not found in active configuration"
        }))),
        Err(_) => Json(ApiResponse::success(serde_json::json!({
            "status": "not_running",
            "provider": provider_name,
            "message": "Workflow is not currently running; no circuit breaker to reset"
        }))),
    }
}

/// Build the provider health sub-router.
pub fn provider_health_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/{workflow_id}/providers/status", get(get_provider_status))
        .route(
            "/{workflow_id}/providers/{provider_name}/reset",
            post(reset_provider),
        )
}
