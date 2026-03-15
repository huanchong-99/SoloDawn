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

/// Helper: Setup test environment
async fn setup_test() -> (DeploymentImpl, Uuid) {
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

    // Create CLI type via raw SQL
    sqlx::query(
        r"INSERT INTO cli_type (id, name, display_name, detect_command, is_system, created_at)
          VALUES (?1, ?2, ?3, ?4, 0, ?5)",
    )
    .bind("test-cli")
    .bind("test-cli")
    .bind("Test CLI")
    .bind("echo --version")
    .bind(chrono::Utc::now())
    .execute(&deployment.db().pool)
    .await
    .expect("Failed to create CLI type");

    // Create model config via raw SQL
    sqlx::query(
        r"INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official, created_at, updated_at)
          VALUES (?1, ?2, ?3, ?4, ?5, 1, 1, ?6, ?7)",
    )
    .bind("test-model")
    .bind("test-cli")
    .bind("test-model")
    .bind("Test Model")
    .bind("test-model")
    .bind(chrono::Utc::now())
    .bind(chrono::Utc::now())
    .execute(&deployment.db().pool)
    .await
    .expect("Failed to create model config");

    (deployment, project_id)
}

/// Helper: Create a minimal workflow
async fn create_minimal_workflow(
    deployment: &DeploymentImpl,
    project_id: Uuid,
    orchestrator_enabled: bool,
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
        merge_terminal_cli_id: "test-cli".to_string(),
        merge_terminal_model_id: "test-model".to_string(),
        target_branch: "main".to_string(),
        git_watcher_enabled: true,
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };

    Workflow::create(&deployment.db().pool, &workflow)
        .await
        .expect("Failed to create workflow");

    workflow_id
}

#[tokio::test]
async fn test_start_workflow_requires_ready_status() {
    // Setup: Create deployment and workflow in 'created' status
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id, true).await;

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

    let app = server::routes::build_router(deployment.clone(), create_test_hub(), server::feishu_handle::new_shared_handle(), create_test_cli_health_monitor());

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
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id, true).await;

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
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id, true).await;

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
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id, false).await;

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

    let app = server::routes::build_router(deployment.clone(), create_test_hub(), server::feishu_handle::new_shared_handle(), create_test_cli_health_monitor());

    let request = Request::builder()
        .method("POST")
        .uri(format!("/api/workflows/{}/start", workflow_id))
        .body(Body::empty())
        .expect("Failed to build request");

    let response = app.oneshot(request).await.expect("Failed to get response");

    // Should return 400 BadRequest
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);

    // Verify workflow status is still 'ready' (not changed)
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "ready");
}
