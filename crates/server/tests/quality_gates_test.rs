//! Integration tests for the G2 quality-gate materialization hard-block and the
//! G3 per-project quality-policy CRUD endpoints.
//!
//! Harness mirrors `crates/server/tests/workflow_api_test.rs`:
//! - `DeploymentImpl::new()` builds a DB pool with all migrations applied.
//! - `server::routes::build_router(...)` builds the full axum app.
//! - requests are driven via `tower::ServiceExt::oneshot`.
//!
//! Auth note: `require_api_token` is a no-op when `SOLODAWN_API_TOKEN` is unset
//! (development mode), so these `/api/...` requests need no bearer token — the
//! same assumption `workflow_api_test.rs` relies on.

use std::sync::Arc;

use axum::{
    body::Body,
    http::{Request, StatusCode},
};
use db::models::{
    Workflow,
    planning_draft::PlanningDraft,
    project::{CreateProject, Project},
};
use http_body_util::BodyExt;
use server::{Deployment, DeploymentImpl, routes::subscription_hub::SubscriptionHub};
use services::services::cli_health_monitor::{CliHealthMonitor, SharedCliHealthMonitor};
use tower::ServiceExt;
use uuid::Uuid;

// ---------------------------------------------------------------------------
// Test harness helpers (cloned from workflow_api_test.rs)
// ---------------------------------------------------------------------------

fn create_test_hub() -> server::routes::SharedSubscriptionHub {
    Arc::new(SubscriptionHub::default())
}

fn create_test_cli_health_monitor() -> SharedCliHealthMonitor {
    Arc::new(CliHealthMonitor::new(0))
}

fn create_test_concierge_agent() -> Arc<services::services::concierge::ConciergeAgent> {
    let pool = sqlx::SqlitePool::connect_lazy("sqlite::memory:").unwrap();
    Arc::new(services::services::concierge::ConciergeAgent::new(
        pool,
        Arc::new(services::services::concierge::ConciergeBroadcaster::new()),
    ))
}

fn create_test_concierge_broadcaster() -> Arc<services::services::concierge::ConciergeBroadcaster> {
    Arc::new(services::services::concierge::ConciergeBroadcaster::new())
}

fn build_app(deployment: &DeploymentImpl) -> axum::Router {
    server::routes::build_router(
        deployment.clone(),
        create_test_hub(),
        server::feishu_handle::new_shared_handle(),
        create_test_cli_health_monitor(),
        create_test_concierge_agent(),
        create_test_concierge_broadcaster(),
    )
}

async fn setup() -> (DeploymentImpl, Uuid) {
    // Defensive: ensure dev-mode auth passthrough for these tests.
    unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };

    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");

    let project_id = Uuid::new_v4();
    let request = CreateProject {
        name: format!("QG Test Project {project_id}"),
        repositories: vec![],
    };
    Project::create(&deployment.db().pool, &request, project_id)
        .await
        .expect("Failed to create project");

    (deployment, project_id)
}

/// Insert a planning draft already advanced to `confirmed` (System-B audit
/// confirm) but with `gates_confirmed_at` still NULL (G2 not yet confirmed).
async fn insert_confirmed_draft(deployment: &DeploymentImpl, project_id: Uuid) -> String {
    let mut draft = PlanningDraft::new(project_id, "QG Draft");
    draft.requirement_summary = Some("do the thing".to_string());
    PlanningDraft::insert(&deployment.db().pool, &draft)
        .await
        .expect("insert draft");
    // status -> confirmed, confirmed_at set, gates_confirmed_at stays NULL.
    PlanningDraft::set_confirmed(&deployment.db().pool, &draft.id)
        .await
        .expect("set_confirmed");
    draft.id
}

async fn body_json(response: axum::response::Response) -> serde_json::Value {
    let bytes = response
        .into_body()
        .collect()
        .await
        .expect("collect body")
        .to_bytes();
    serde_json::from_slice(&bytes).unwrap_or(serde_json::Value::Null)
}

// ---------------------------------------------------------------------------
// G2: materialize hard-block
// ---------------------------------------------------------------------------

#[tokio::test]
async fn g2_materialize_400_until_gates_confirmed_then_200() {
    let (deployment, project_id) = setup().await;
    let draft_id = insert_confirmed_draft(&deployment, project_id).await;

    // Precondition: status=confirmed, gates_confirmed_at IS NULL.
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(draft.status, "confirmed");
    assert!(
        draft.gates_confirmed_at.is_none(),
        "precondition: gates not yet confirmed"
    );
    let confirmed_at_before = draft.confirmed_at;

    // 1) Materialize while gates NOT confirmed → 400, and NO workflow created.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri(format!("/api/planning-drafts/{draft_id}/materialize"))
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(
        resp.status(),
        StatusCode::BAD_REQUEST,
        "materialize must 400 while gates_confirmed_at IS NULL"
    );

    let workflows = Workflow::find_by_project(&deployment.db().pool, project_id)
        .await
        .unwrap();
    assert!(
        workflows.is_empty(),
        "no workflow row may exist after the 400 hard-block, found {}",
        workflows.len()
    );

    // 2) Confirm gates (no DIY body) → stamps gates_confirmed_at.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri(format!("/api/planning-drafts/{draft_id}/confirm-gates"))
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK, "confirm-gates should succeed");

    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id)
        .await
        .unwrap()
        .unwrap();
    assert!(
        draft.gates_confirmed_at.is_some(),
        "gates_confirmed_at must be stamped after confirm-gates"
    );
    // Distinct-column invariant: confirm-gates must NOT touch confirmed_at.
    assert_eq!(
        draft.confirmed_at, confirmed_at_before,
        "confirm-gates must not alias/overwrite confirmed_at (System-B)"
    );

    // 3) Materialize now → 200, workflow row exists.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri(format!("/api/planning-drafts/{draft_id}/materialize"))
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(
        resp.status(),
        StatusCode::OK,
        "materialize must succeed once gates are confirmed"
    );

    let workflows = Workflow::find_by_project(&deployment.db().pool, project_id)
        .await
        .unwrap();
    assert_eq!(
        workflows.len(),
        1,
        "exactly one workflow row must exist after a successful materialize"
    );
}

// ---------------------------------------------------------------------------
// G3: per-project quality-policy CRUD round-trip
// ---------------------------------------------------------------------------

#[tokio::test]
async fn g3_quality_policy_put_get_delete_round_trips() {
    let (deployment, project_id) = setup().await;
    let app_uri = format!("/api/projects/{project_id}/quality-policy");

    // Build a valid config to PUT: start from the default and flip mode to enforce.
    let mut config = quality::config::QualityGateConfig::default_config();
    config.mode = quality::config::QualityGateMode::Enforce;
    config.terminal_gate.name = "RoundTrip Terminal".to_string();
    let put_body = serde_json::to_vec(&config).unwrap();

    // 1) PUT the override → 200, source=project.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("PUT")
                .uri(&app_uri)
                .header("content-type", "application/json")
                .body(Body::from(put_body.clone()))
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK, "PUT policy should succeed");
    let json = body_json(resp).await;
    assert_eq!(json["success"], serde_json::json!(true));
    assert_eq!(
        json["data"]["source"], "project",
        "PUT response source must be 'project'"
    );

    // 2) GET → source=project and the same config (enforce + custom terminal name).
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("GET")
                .uri(&app_uri)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK);
    let json = body_json(resp).await;
    assert_eq!(
        json["data"]["source"], "project",
        "GET after PUT must resolve from the DB project policy"
    );
    assert_eq!(
        json["data"]["config"]["mode"], "enforce",
        "round-tripped config must preserve mode=enforce"
    );
    // NOTE: QualityGateConfig itself has no serde(rename_all), so its fields stay
    // snake_case (`terminal_gate`); only the QualityPolicyResponse wrapper is camelCase.
    assert_eq!(
        json["data"]["config"]["terminal_gate"]["name"], "RoundTrip Terminal",
        "round-tripped config must preserve the edited terminal gate name"
    );

    // 3) DELETE → 200, removes the override.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri(&app_uri)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK, "DELETE policy should succeed");

    // 4) GET after DELETE → source is NO LONGER 'project' (falls back to bundled).
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("GET")
                .uri(&app_uri)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK);
    let json = body_json(resp).await;
    assert_ne!(
        json["data"]["source"], "project",
        "after DELETE the resolver must fall back away from the DB project policy"
    );
    assert_eq!(
        json["data"]["source"], "bundled",
        "server-side GET fallback (no working dir) reports 'bundled'"
    );
}

/// A config with an invalid operator must be rejected with 400 by `validate()`.
#[tokio::test]
async fn g3_quality_policy_put_invalid_config_400() {
    let (deployment, project_id) = setup().await;
    let app_uri = format!("/api/projects/{project_id}/quality-policy");

    // Inject an invalid operator ("BADOP" is neither GT nor LT) into a condition.
    let mut config = quality::config::QualityGateConfig::default_config();
    if let Some(cond) = config.terminal_gate.conditions.first_mut() {
        cond.operator = "BADOP".to_string();
    }
    let put_body = serde_json::to_vec(&config).unwrap();

    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("PUT")
                .uri(&app_uri)
                .header("content-type", "application/json")
                .body(Body::from(put_body))
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(
        resp.status(),
        StatusCode::BAD_REQUEST,
        "PUT with an invalid operator must be rejected by validate()"
    );
}
