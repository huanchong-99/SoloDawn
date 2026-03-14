use axum::{Router, routing::post, http::StatusCode, response::Json};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

/// Payload sent by the ci-notify.yml GitHub Actions workflow.
#[derive(Debug, Deserialize, Serialize)]
pub struct CiWebhookPayload {
    pub workflow: String,
    pub conclusion: String,
    pub sha: String,
    pub branch: String,
    pub run_id: u64,
    pub run_url: String,
}

/// POST /api/ci/webhook
///
/// Accepts CI workflow completion notifications from GitHub Actions.
/// This is a stub route - future phases will route results to the
/// orchestrator for automated status updates and failure handling.
// TODO(G35-009): Validate a shared webhook secret (e.g. HMAC signature) before
// accepting payloads. Without this, any caller can forge CI notifications.
pub async fn ci_webhook(
    Json(payload): Json<CiWebhookPayload>,
) -> (StatusCode, Json<Value>) {
    tracing::info!(
        workflow = %payload.workflow,
        conclusion = %payload.conclusion,
        sha = %payload.sha,
        branch = %payload.branch,
        run_id = %payload.run_id,
        "CI webhook received"
    );

    // TODO: Phase 30+ - Route to orchestrator for:
    // - Updating workflow status based on CI results
    // - Triggering auto-repair on failure
    // - Sending notifications via configured integrations

    (StatusCode::ACCEPTED, Json(json!({
        "status": "accepted",
        "message": "CI webhook notification received"
    })))
}

pub fn ci_webhook_routes<S: Clone + Send + Sync + 'static>() -> Router<S> {
    Router::new()
        .route("/webhook", post(ci_webhook))
}
