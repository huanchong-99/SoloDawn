//! Integration tests for the AI-editable custom-rule REST API (PRD §10,
//! `docs/quality/PRD-ai-editable-quality-rules.md`).
//!
//! These cover the DETERMINISTIC, AI-free surface only — the admission gate
//! (regex compile + positive/negative example execution) and the CRUD lifecycle
//! (create → list → promote → delete). The `/author` + `/revalidate` routes drive
//! an LLM backend and are intentionally NOT exercised here: the services pipeline
//! already has `MockLLMClient`-backed unit tests (`rule_authoring::tests_pipeline`)
//! and wiring a mock backend at the route layer would require heavy scaffolding
//! (a custom `DeploymentImpl` + a seeded encrypted `model_config` row) for no
//! additional deterministic coverage.
//!
//! Harness mirrors `crates/server/tests/quality_gates_test.rs`:
//! - `DeploymentImpl::new()` builds a DB pool with all migrations applied.
//! - `server::routes::build_router(...)` builds the full axum app.
//! - requests are driven via `tower::ServiceExt::oneshot`.
//!
//! Auth note: `require_api_token` is a no-op when `SOLODAWN_API_TOKEN` is unset
//! (development mode), so these `/api/...` requests need no bearer token.

use std::sync::Arc;

use axum::{
    body::Body,
    http::{Request, StatusCode},
};
use db::models::{
    CustomRule,
    project::{CreateProject, Project},
};
use http_body_util::BodyExt;
use server::{Deployment, DeploymentImpl, routes::subscription_hub::SubscriptionHub};
use services::services::cli_health_monitor::{CliHealthMonitor, SharedCliHealthMonitor};
use tower::ServiceExt;
use uuid::Uuid;

// ---------------------------------------------------------------------------
// Test harness helpers (cloned from quality_gates_test.rs)
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
        name: format!("CR Test Project {project_id}"),
        repositories: vec![],
    };
    Project::create(&deployment.db().pool, &request, project_id)
        .await
        .expect("Failed to create project");

    (deployment, project_id)
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

/// POST a custom-rule create body to a project, returning the raw response.
async fn post_create(
    deployment: &DeploymentImpl,
    project_id: Uuid,
    body: serde_json::Value,
) -> axum::response::Response {
    let app = build_app(deployment);
    app.oneshot(
        Request::builder()
            .method("POST")
            .uri(format!("/api/projects/{project_id}/custom-rules"))
            .header("content-type", "application/json")
            .body(Body::from(serde_json::to_vec(&body).unwrap()))
            .unwrap(),
    )
    .await
    .unwrap()
}

/// A well-formed `CustomRuleInput` body banning `dbg!(` in Rust. The positive
/// example flags (contains `dbg!(`), the negative does not — so this passes the
/// admission gate by default. Tests tweak individual fields to force failures.
fn sound_rule_body() -> serde_json::Value {
    serde_json::json!({
        "nlRequest": "prohibit dbg! macro in committed Rust",
        "ruleFormat": "regex",
        "ruleBody": r"dbg!\(",
        "name": "no-dbg-macro",
        "description": "The dbg! macro must not be committed.",
        "message": "Remove the dbg! macro before committing.",
        "ruleType": "CodeSmell",
        "severity": "MAJOR",
        "languages": ["rust"],
        "extensions": ["rs"],
        "examples": [
            {
                "kind": "positive",
                "language": "rust",
                "snippet": "fn f() { dbg!(x); }",
                "note": "must flag the dbg! macro"
            },
            {
                "kind": "negative",
                "language": "rust",
                "snippet": "fn f() { let x = 1; }",
                "note": "ordinary code must not flag"
            }
        ]
    })
}

// ---------------------------------------------------------------------------
// Admission gate — rejection paths (400)
// ---------------------------------------------------------------------------

/// A rule whose regex does not compile is rejected with 400 before persist.
#[tokio::test]
async fn admission_gate_rejects_uncompilable_regex() {
    let (deployment, project_id) = setup().await;

    let mut body = sound_rule_body();
    // An unclosed group is an invalid Rust `regex` pattern (compile error).
    body["ruleBody"] = serde_json::json!(r"dbg!(\(");
    // Drop the examples so the failure is unambiguously the compile step.
    body["examples"] = serde_json::json!([]);

    let resp = post_create(&deployment, project_id, body).await;
    assert_eq!(
        resp.status(),
        StatusCode::BAD_REQUEST,
        "an uncompilable regex must be rejected with 400"
    );

    // Nothing was persisted.
    let rules = CustomRule::find_by_project(&deployment.db().pool, project_id)
        .await
        .unwrap();
    assert!(
        rules.is_empty(),
        "no rule may be persisted when the admission gate rejects the regex"
    );
}

/// A rule whose negative example actually flags is rejected with 400 (the
/// empirical contract: every negative MUST NOT fire).
#[tokio::test]
async fn admission_gate_rejects_when_negative_example_flags() {
    let (deployment, project_id) = setup().await;

    let mut body = sound_rule_body();
    // Make the negative example contain `dbg!(` so the (valid) rule fires on it,
    // violating the "negatives must not match" invariant.
    body["examples"] = serde_json::json!([
        {
            "kind": "positive",
            "language": "rust",
            "snippet": "fn f() { dbg!(x); }"
        },
        {
            "kind": "negative",
            "language": "rust",
            "snippet": "fn g() { dbg!(y); }"
        }
    ]);

    let resp = post_create(&deployment, project_id, body).await;
    assert_eq!(
        resp.status(),
        StatusCode::BAD_REQUEST,
        "a negative example that flags must fail the admission gate with 400"
    );

    let rules = CustomRule::find_by_project(&deployment.db().pool, project_id)
        .await
        .unwrap();
    assert!(
        rules.is_empty(),
        "no rule may be persisted when a negative example flags"
    );
}

/// A bad CHECK token (here an unknown severity) is rejected with 400.
#[tokio::test]
async fn admission_gate_rejects_invalid_severity_token() {
    let (deployment, project_id) = setup().await;

    let mut body = sound_rule_body();
    body["severity"] = serde_json::json!("SUPER_BAD");

    let resp = post_create(&deployment, project_id, body).await;
    assert_eq!(
        resp.status(),
        StatusCode::BAD_REQUEST,
        "an invalid severity token must be rejected with 400"
    );
}

// ---------------------------------------------------------------------------
// Admission gate — acceptance + CRUD lifecycle
// ---------------------------------------------------------------------------

/// A sound rule is accepted, persisted at `status='shadow'`, and round-trips
/// through GET; then PATCH promotes it shadow→warn and DELETE removes it.
#[tokio::test]
async fn sound_rule_creates_lists_promotes_and_deletes() {
    let (deployment, project_id) = setup().await;

    // 1) POST a sound rule → 200, persisted at status=shadow.
    let resp = post_create(&deployment, project_id, sound_rule_body()).await;
    assert_eq!(
        resp.status(),
        StatusCode::OK,
        "a sound rule must pass the admission gate and persist"
    );
    let json = body_json(resp).await;
    assert_eq!(json["success"], serde_json::json!(true));
    assert_eq!(
        json["data"]["status"], "shadow",
        "a freshly created rule must land at status=shadow"
    );
    assert_eq!(
        json["data"]["ruleBody"], r"dbg!\(",
        "the persisted rule body must round-trip the submitted pattern"
    );
    let rule_id = json["data"]["id"].as_str().unwrap().to_string();

    // 2) GET the project's rules → the new rule is listed.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("GET")
                .uri(format!("/api/projects/{project_id}/custom-rules"))
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK);
    let json = body_json(resp).await;
    let listed = json["data"].as_array().expect("data is an array");
    assert_eq!(listed.len(), 1, "exactly one rule must be listed");
    assert_eq!(listed[0]["id"], rule_id);
    assert_eq!(listed[0]["name"], "no-dbg-macro");

    // 3) PATCH status shadow → warn (promotion).
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("PATCH")
                .uri(format!(
                    "/api/projects/{project_id}/custom-rules/{rule_id}/status"
                ))
                .header("content-type", "application/json")
                .body(Body::from(
                    serde_json::to_vec(&serde_json::json!({ "status": "warn" })).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK, "promotion must succeed");
    let json = body_json(resp).await;
    assert_eq!(
        json["data"]["status"], "warn",
        "the rule status must be promoted to warn"
    );

    // 4) An invalid status token is rejected with 400.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("PATCH")
                .uri(format!(
                    "/api/projects/{project_id}/custom-rules/{rule_id}/status"
                ))
                .header("content-type", "application/json")
                .body(Body::from(
                    serde_json::to_vec(&serde_json::json!({ "status": "bogus" })).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(
        resp.status(),
        StatusCode::BAD_REQUEST,
        "an invalid status token must be rejected with 400"
    );

    // 5) DELETE the rule → 200, and it is gone from the DB.
    let app = build_app(&deployment);
    let resp = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri(format!(
                    "/api/projects/{project_id}/custom-rules/{rule_id}"
                ))
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();
    assert_eq!(resp.status(), StatusCode::OK, "delete must succeed");

    let rules = CustomRule::find_by_project(&deployment.db().pool, project_id)
        .await
        .unwrap();
    assert!(rules.is_empty(), "the rule must be gone after delete");
}
