//! Integration tests for workflow API endpoints
//!
//! These tests verify the full workflow lifecycle including:
//! - Creating workflows
//! - Starting workflows
//! - Status transitions

use std::sync::Arc;

use db::models::{
    Workflow,
    project::{CreateProject, Project},
};
use server::{Deployment, DeploymentImpl, routes::subscription_hub::SubscriptionHub};
use services::services::cli_health_monitor::{CliHealthMonitor, SharedCliHealthMonitor};
use uuid::Uuid;

/// Helper: Create a test subscription hub
fn create_test_hub() -> server::routes::SharedSubscriptionHub {
    Arc::new(SubscriptionHub::default())
}

/// Helper: Create a test CLI health monitor
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

/// Helper: Setup test environment
///
/// Returns the deployment, the project id, and the per-test unique `cli_type`
/// id and `model_config` id. All integration tests share one on-disk
/// `db.sqlite` (see `DBService::new` / `asset_dir`), so hardcoded ids would
/// collide on the `cli_type` PRIMARY KEY / UNIQUE(name) across parallel tests
/// and re-runs. Mirror the sibling suite (quality_gates_test) by using fresh
/// Uuids for every inserted identifier.
async fn setup_test() -> (DeploymentImpl, Uuid, String, String) {
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");

    // Create a test project
    let project_id = Uuid::new_v4();
    let request = CreateProject {
        name: "Test Project".to_string(),
        repositories: vec![],
    };
    Project::create(&deployment.db().pool, &request, project_id)
        .await
        .expect("Failed to create project");

    // Unique per-test ids so parallel tests / re-runs never collide on the
    // shared on-disk DB. `cli_type.id` and `cli_type.name` are both unique.
    let cli_type_id = format!("test-cli-{}", Uuid::new_v4());
    let model_config_id = format!("test-model-{}", Uuid::new_v4());

    // Create CLI type via raw SQL
    sqlx::query(
        r"INSERT INTO cli_type (id, name, display_name, detect_command, is_system, created_at)
          VALUES (?1, ?2, ?3, ?4, 0, ?5)",
    )
    .bind(&cli_type_id)
    .bind(&cli_type_id)
    .bind("Test CLI")
    .bind("echo --version")
    .bind(chrono::Utc::now())
    .execute(&deployment.db().pool)
    .await
    .expect("Failed to create CLI type");

    // Create model config via raw SQL (FK: cli_type_id -> cli_type.id)
    sqlx::query(
        r"INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official, created_at, updated_at)
          VALUES (?1, ?2, ?3, ?4, ?5, 1, 1, ?6, ?7)",
    )
    .bind(&model_config_id)
    .bind(&cli_type_id)
    .bind("test-model")
    .bind("Test Model")
    .bind("test-model")
    .bind(chrono::Utc::now())
    .bind(chrono::Utc::now())
    .execute(&deployment.db().pool)
    .await
    .expect("Failed to create model config");

    (deployment, project_id, cli_type_id, model_config_id)
}

/// Helper: Create a minimal workflow
async fn create_minimal_workflow(
    deployment: &DeploymentImpl,
    project_id: Uuid,
    orchestrator_enabled: bool,
    cli_type_id: &str,
    model_config_id: &str,
) -> String {
    let workflow_id = Uuid::new_v4().to_string();

    let orchestrator_fields = if orchestrator_enabled {
        (
            Some("openai-compatible".to_string()),
            Some("https://api.test.com".to_string()),
            Some("test-key".to_string()),
            Some("gpt-4".to_string()),
        )
    } else {
        (None, None, None, None)
    };

    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id,
        name: "Test Workflow".to_string(),
        description: Some("Test description".to_string()),
        status: "created".to_string(),
        execution_mode: "diy".to_string(),
        initial_goal: None,
        use_slash_commands: false,
        orchestrator_enabled,
        orchestrator_api_type: orchestrator_fields.0,
        orchestrator_base_url: orchestrator_fields.1,
        orchestrator_api_key: orchestrator_fields.2,
        orchestrator_model: orchestrator_fields.3,
        error_terminal_enabled: false,
        error_terminal_cli_id: None,
        error_terminal_model_id: None,
        merge_terminal_cli_id: cli_type_id.to_string(),
        merge_terminal_model_id: model_config_id.to_string(),
        target_branch: "main".to_string(),
        git_watcher_enabled: true,
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
        pause_reason: None,
        audit_plan: None,
    };

    Workflow::create(&deployment.db().pool, &workflow)
        .await
        .expect("Failed to create workflow");

    workflow_id
}

#[tokio::test]
async fn test_start_workflow_requires_ready_status() {
    // Setup: Create deployment and workflow in 'created' status
    let (deployment, project_id, cli_type_id, model_config_id) = setup_test().await;
    let workflow_id =
        create_minimal_workflow(&deployment, project_id, true, &cli_type_id, &model_config_id)
            .await;

    // Verify workflow is in 'created' status
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "created");

    // Attempt to start workflow in 'created' status - should fail
    use axum::{
        body::Body,
        http::{Request, StatusCode},
    };
    use tower::ServiceExt;

    let app = server::routes::build_router(
        deployment.clone(),
        create_test_hub(),
        server::feishu_handle::new_shared_handle(),
        create_test_cli_health_monitor(),
        create_test_concierge_agent(),
        create_test_concierge_broadcaster(),
    );

    let request = Request::builder()
        .method("POST")
        .uri(format!("/api/workflows/{}/start", workflow_id))
        .body(Body::empty())
        .expect("Failed to build request");

    let response = app.oneshot(request).await.expect("Failed to get response");

    // Should return error because workflow is not in 'ready' status
    assert_ne!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn test_start_workflow_with_ready_status() {
    // Setup: Create deployment and workflow, then set to 'ready'
    let (deployment, project_id, cli_type_id, model_config_id) = setup_test().await;
    let workflow_id =
        create_minimal_workflow(&deployment, project_id, true, &cli_type_id, &model_config_id)
            .await;

    // Update workflow status to 'ready'
    Workflow::update_status(&deployment.db().pool, &workflow_id, "ready")
        .await
        .expect("Failed to update workflow status");

    // Verify workflow is ready
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "ready");
}

#[tokio::test]
async fn test_workflow_status_transitions() {
    // Setup
    let (deployment, project_id, cli_type_id, model_config_id) = setup_test().await;
    let workflow_id =
        create_minimal_workflow(&deployment, project_id, true, &cli_type_id, &model_config_id)
            .await;

    // Verify initial status
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "created");

    // Transition to ready
    Workflow::update_status(&deployment.db().pool, &workflow_id, "ready")
        .await
        .expect("Failed to update status");

    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "ready");
}

#[tokio::test]
async fn test_start_workflow_without_orchestrator() {
    // Setup: Create workflow without orchestrator
    let (deployment, project_id, cli_type_id, model_config_id) = setup_test().await;
    let workflow_id =
        create_minimal_workflow(&deployment, project_id, false, &cli_type_id, &model_config_id)
            .await;

    // Update to ready status
    Workflow::update_status(&deployment.db().pool, &workflow_id, "ready")
        .await
        .expect("Failed to update workflow status to ready");

    // Verify workflow is ready but orchestrator is disabled
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "ready");
    assert!(!workflow.orchestrator_enabled);

    // Attempt to start workflow - should return 400 BadRequest
    use axum::{
        body::Body,
        http::{Request, StatusCode},
    };
    use tower::ServiceExt;

    let app = server::routes::build_router(
        deployment.clone(),
        create_test_hub(),
        server::feishu_handle::new_shared_handle(),
        create_test_cli_health_monitor(),
        create_test_concierge_agent(),
        create_test_concierge_broadcaster(),
    );

    let request = Request::builder()
        .method("POST")
        .uri(format!("/api/workflows/{}/start", workflow_id))
        .body(Body::empty())
        .expect("Failed to build request");

    let response = app.oneshot(request).await.expect("Failed to get response");

    // DIY mode (orchestrator disabled) is a first-class path: starting a ready
    // DIY workflow succeeds (no orchestrator agent required) and transitions it
    // to "running" — see start_workflow's `else` branch which calls
    // Workflow::set_started (status -> 'running').
    assert_eq!(response.status(), StatusCode::OK);

    // Verify workflow transitioned to 'running'
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "running");
}
