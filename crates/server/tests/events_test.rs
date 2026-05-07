//! Integration tests for event broadcasting
//!
//! These tests verify that status updates (workflow, task, terminal)
//! are properly broadcast through the MessageBus for real-time client updates.

use std::{sync::Arc, time::Duration};

use db::models::{
    Terminal, Workflow, WorkflowTask,
    project::{CreateProject, Project},
};
use serial_test::serial;
use server::{Deployment, DeploymentImpl};
use services::orchestrator::{
    agent::OrchestratorAgent,
    config::OrchestratorConfig as AgentConfig,
    message_bus::{BusMessage, MessageBus},
};
use tokio::time::timeout;
use uuid::Uuid;

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

    (deployment, project_id)
}

/// Helper: Create a minimal workflow
async fn create_minimal_workflow(deployment: &DeploymentImpl, project_id: Uuid) -> String {
    let workflow_id = Uuid::new_v4().to_string();

    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id,
        name: "Test Workflow".to_string(),
        description: Some("Test description".to_string()),
        status: "ready".to_string(),
        execution_mode: "diy".to_string(),
        initial_goal: None,
        use_slash_commands: false,
        orchestrator_enabled: true,
        orchestrator_api_type: Some("openai-compatible".to_string()),
        orchestrator_base_url: Some("https://api.test.com".to_string()),
        orchestrator_api_key: Some("test-key".to_string()),
        orchestrator_model: Some("gpt-4".to_string()),
        error_terminal_enabled: false,
        error_terminal_cli_id: None,
        error_terminal_model_id: None,
        merge_terminal_cli_id: "cli-codex".to_string(),
        merge_terminal_model_id: "model-codex-gpt4o".to_string(),
        target_branch: "main".to_string(),
        git_watcher_enabled: true,
        ready_at: Some(chrono::Utc::now()),
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

fn test_agent_config() -> AgentConfig {
    AgentConfig {
        api_type: "openai-compatible".to_string(),
        base_url: "https://api.test.com".to_string(),
        api_key: "test-key".to_string(),
        model: "gpt-4".to_string(),
        system_prompt: "Test system prompt".to_string(),
        max_conversation_history: 50,
        timeout_secs: 120,
        max_retries: 3,
        retry_delay_ms: 1000,
        rate_limit_requests_per_second: 10,
        auto_merge_on_completion: false,
        fallback_providers: vec![],
        quality_gate_mode: "off".to_string(),
    }
}

fn create_test_agent(
    workflow_id: &str,
    message_bus: Arc<MessageBus>,
    deployment: &DeploymentImpl,
) -> OrchestratorAgent {
    let _ = rustls::crypto::ring::default_provider().install_default();

    OrchestratorAgent::new(
        test_agent_config(),
        workflow_id.to_string(),
        message_bus,
        Arc::new(deployment.db().clone()),
    )
    .expect("Failed to create agent")
}

#[tokio::test]
#[serial]
async fn test_workflow_status_broadcast() {
    // Setup: Create deployment and workflow
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id).await;

    // Prepare bus subscription
    let message_bus = Arc::new(MessageBus::new(100));

    // Subscribe to workflow events
    let topic = format!("workflow:{}", workflow_id);
    let mut subscriber = message_bus.subscribe(&topic).await;

    let agent = create_test_agent(&workflow_id, message_bus.clone(), &deployment);

    // Broadcast workflow status
    agent
        .broadcast_workflow_status("running")
        .await
        .expect("Failed to broadcast workflow status");

    // Verify the status update was received
    let message = timeout(Duration::from_millis(500), subscriber.recv())
        .await
        .expect("Timeout waiting for status update")
        .expect("No message received");

    match message {
        BusMessage::StatusUpdate {
            workflow_id: received_id,
            status,
        } => {
            assert_eq!(received_id, workflow_id);
            assert_eq!(status, "running");
        }
        _ => panic!("Expected StatusUpdate message, got {:?}", message),
    }

    // Verify database was updated
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found");
    assert_eq!(workflow.status, "running");
}

#[tokio::test]
#[serial]
async fn test_terminal_status_broadcast() {
    // Setup: Create deployment, workflow, and task
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id).await;

    // Create a workflow task
    let task_id = Uuid::new_v4().to_string();
    let task = WorkflowTask {
        id: task_id.clone(),
        workflow_id: workflow_id.clone(),
        vk_task_id: None,
        name: "Test Task".to_string(),
        description: None,
        branch: "test-branch".to_string(),
        status: "pending".to_string(),
        order_index: 0,
        started_at: None,
        completed_at: None,
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };
    WorkflowTask::create(&deployment.db().pool, &task)
        .await
        .expect("Failed to create task");

    // Create a terminal
    let terminal_id = Uuid::new_v4().to_string();
    let terminal = Terminal {
        id: terminal_id.clone(),
        workflow_task_id: task_id.clone(),
        cli_type_id: "cli-codex".to_string(),
        model_config_id: "model-codex-gpt4o".to_string(),
        custom_base_url: None,
        custom_api_key: None,
        role: Some("coder".to_string()),
        role_description: None,
        order_index: 0,
        status: "not_started".to_string(),
        process_id: None,
        pty_session_id: None,
        session_id: None,
        execution_process_id: None,
        vk_session_id: None,
        auto_confirm: true,
        last_commit_hash: None,
        last_commit_message: None,
        started_at: None,
        completed_at: None,
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };
    Terminal::create(&deployment.db().pool, &terminal)
        .await
        .expect("Failed to create terminal");

    let message_bus = Arc::new(MessageBus::new(100));

    // Subscribe to workflow events
    let topic = format!("workflow:{}", workflow_id);
    let mut subscriber = message_bus.subscribe(&topic).await;

    let agent = create_test_agent(&workflow_id, message_bus.clone(), &deployment);

    // Broadcast terminal status
    agent
        .broadcast_terminal_status(&terminal_id, "working")
        .await
        .expect("Failed to broadcast terminal status");

    // Verify the status update was received
    let message = timeout(Duration::from_millis(500), subscriber.recv())
        .await
        .expect("Timeout waiting for status update")
        .expect("No message received");

    match message {
        BusMessage::TerminalStatusUpdate {
            workflow_id: received_id,
            terminal_id: received_terminal_id,
            status,
        } => {
            assert_eq!(received_id, workflow_id);
            assert_eq!(received_terminal_id, terminal_id);
            assert_eq!(status, "working");
        }
        _ => panic!("Expected TerminalStatusUpdate message, got {:?}", message),
    }

    // Verify database was updated
    let terminal = Terminal::find_by_id(&deployment.db().pool, &terminal_id)
        .await
        .expect("Failed to query terminal")
        .expect("Terminal not found");
    assert_eq!(terminal.status, "working");
}

#[tokio::test]
#[serial]
async fn test_task_status_broadcast() {
    // Setup: Create deployment, workflow, and task
    let (deployment, project_id) = setup_test().await;
    let workflow_id = create_minimal_workflow(&deployment, project_id).await;

    // Create a workflow task
    let task_id = Uuid::new_v4().to_string();
    let task = WorkflowTask {
        id: task_id.clone(),
        workflow_id: workflow_id.clone(),
        vk_task_id: None,
        name: "Test Task".to_string(),
        description: None,
        branch: "test-branch".to_string(),
        status: "pending".to_string(),
        order_index: 0,
        started_at: None,
        completed_at: None,
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };
    WorkflowTask::create(&deployment.db().pool, &task)
        .await
        .expect("Failed to create task");

    let message_bus = Arc::new(MessageBus::new(100));

    // Subscribe to workflow events
    let topic = format!("workflow:{}", workflow_id);
    let mut subscriber = message_bus.subscribe(&topic).await;

    let agent = create_test_agent(&workflow_id, message_bus.clone(), &deployment);

    // Broadcast task status
    agent
        .broadcast_task_status(&task_id, "running")
        .await
        .expect("Failed to broadcast task status");

    // Verify the status update was received
    let message = timeout(Duration::from_millis(500), subscriber.recv())
        .await
        .expect("Timeout waiting for status update")
        .expect("No message received");

    match message {
        BusMessage::TaskStatusUpdate {
            workflow_id: received_id,
            task_id: received_task_id,
            status,
        } => {
            assert_eq!(received_id, workflow_id);
            assert_eq!(received_task_id, task_id);
            assert_eq!(status, "running");
        }
        _ => panic!("Expected TaskStatusUpdate message, got {:?}", message),
    }

    // Verify database was updated
    let task = WorkflowTask::find_by_id(&deployment.db().pool, &task_id)
        .await
        .expect("Failed to query task")
        .expect("Task not found");
    assert_eq!(task.status, "running");
}
