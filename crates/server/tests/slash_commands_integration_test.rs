//! Integration tests for Slash Commands
//!
//! These tests verify the complete slash commands functionality:
//! - CRUD operations for slash command presets
//! - Template rendering with custom parameters
//! - Workflow integration with slash commands
//! - End-to-end command execution flow

use db::models::{
    CreateWorkflowRequest, SlashCommandPreset, TerminalConfig, Workflow, WorkflowCommand,
    WorkflowCommandRequest,
};
use server::{Deployment, DeploymentImpl};
use uuid::Uuid;

/// Helper: Setup test environment
async fn setup_test() -> (DeploymentImpl, String) {
    let deployment: DeploymentImpl = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");

    // Create a test project via raw SQL (Project struct uses Uuid, not String)
    let project_id = Uuid::new_v4();
    let project_id_str = project_id.to_string();
    sqlx::query("INSERT INTO projects (id, name) VALUES (?1, ?2)")
        .bind(project_id)
        .bind("Test Project")
        .execute(&deployment.db().pool)
        .await
        .expect("Failed to create project");

    // Create CLI type via raw SQL (CliType has no create method)
    sqlx::query(
        r"INSERT INTO cli_type (id, name, display_name, detect_command, is_system, created_at)
          VALUES (?1, ?2, ?3, ?4, 0, datetime('now'))",
    )
    .bind("test-cli")
    .bind("test-cli")
    .bind("Test CLI")
    .bind("echo test")
    .execute(&deployment.db().pool)
    .await
    .expect("Failed to create CLI type");

    // Create model config via raw SQL (ModelConfig has no create method)
    sqlx::query(
        r"INSERT OR IGNORE INTO model_config (id, cli_type_id, name, display_name, is_default, is_official, created_at, updated_at)
          VALUES (?1, ?2, ?3, ?4, 0, 0, datetime('now'), datetime('now'))"
    )
    .bind("test-model")
    .bind("test-cli")
    .bind("test-model")
    .bind("Test Model")
    .execute(&deployment.db().pool)
    .await
    .expect("Failed to create model config");

    (deployment, project_id_str)
}

/// Helper: Create a slash command preset
async fn create_test_preset(
    pool: &sqlx::SqlitePool,
    command: &str,
    description: &str,
    template: &str,
) -> SlashCommandPreset {
    SlashCommandPreset::create(pool, command, description, Some(template))
        .await
        .expect("Failed to create test preset")
}

#[tokio::test]
async fn test_crud_slash_command_presets() {
    let (deployment, _) = setup_test().await;
    let pool = &deployment.db().pool;

    // Create
    let preset = create_test_preset(
        pool,
        "/test-crud",
        "Test CRUD command",
        "Template for {{action}}",
    )
    .await;

    assert_eq!(preset.command, "/test-crud");
    assert_eq!(preset.description, "Test CRUD command");
    assert_eq!(
        preset.prompt_template,
        Some("Template for {{action}}".to_string())
    );
    assert!(!preset.is_system);

    // Read
    let found = SlashCommandPreset::find_by_id(pool, &preset.id)
        .await
        .expect("Failed to find preset")
        .expect("Preset not found");
    assert_eq!(found.id, preset.id);

    // Update
    let updated = SlashCommandPreset::update(
        pool,
        &preset.id,
        Some("/test-crud-updated"),
        Some("Updated description"),
        Some("Updated template"),
    )
    .await
    .expect("Failed to update preset");
    assert_eq!(updated.command, "/test-crud-updated");
    assert_eq!(updated.description, "Updated description");

    // Delete
    SlashCommandPreset::delete(pool, &preset.id)
        .await
        .expect("Failed to delete preset");

    let deleted = SlashCommandPreset::find_by_id(pool, &preset.id)
        .await
        .expect("Failed to query after delete");
    assert!(deleted.is_none(), "Preset should be deleted");
}

#[tokio::test]
async fn test_multiple_presets() {
    let (deployment, _) = setup_test().await;
    let pool = &deployment.db().pool;

    let _preset1 = create_test_preset(
        pool,
        "/multi-cmd-1",
        "First command",
        "Template 1: {{var1}}",
    )
    .await;

    let _preset2 = create_test_preset(
        pool,
        "/multi-cmd-2",
        "Second command",
        "Template 2: {{var2}}",
    )
    .await;

    let all = SlashCommandPreset::find_all(pool)
        .await
        .expect("Failed to list presets");

    let test_presets: Vec<_> = all
        .iter()
        .filter(|p| p.command.starts_with("/multi-cmd"))
        .collect();
    assert_eq!(test_presets.len(), 2, "Should have 2 test presets");
}

#[tokio::test]
async fn test_find_all_presets_is_capped() {
    let (deployment, _) = setup_test().await;
    let pool = &deployment.db().pool;
    let test_run = Uuid::new_v4().simple().to_string();

    for idx in 0..510 {
        create_test_preset(
            pool,
            &format!("/limit-cmd-{test_run}-{idx:03}"),
            "Bounded list test preset",
            "Template",
        )
        .await;
    }

    let all = SlashCommandPreset::find_all(pool)
        .await
        .expect("Failed to list presets");

    assert_eq!(
        all.len(),
        500,
        "find_all should cap slash command presets at the palette ceiling"
    );
}

#[tokio::test]
async fn test_workflow_commands_with_presets() {
    let (deployment, project_id) = setup_test().await;
    let pool = &deployment.db().pool;

    // Create presets
    let preset1 = create_test_preset(
        pool,
        "/wf-cmd-write",
        "Write code",
        "Write code at {{code_path}}",
    )
    .await;

    let preset2 = create_test_preset(
        pool,
        "/wf-cmd-test",
        "Run tests",
        "Test {{module}} with {{coverage}} coverage",
    )
    .await;

    // Create workflow
    let workflow_id = Uuid::new_v4().to_string();
    let now = chrono::Utc::now();

    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id: Uuid::parse_str(&project_id).expect("valid project id"),
        name: "Commands Workflow".to_string(),
        description: Some("Workflow with commands".to_string()),
        status: "created".to_string(),
        execution_mode: "diy".to_string(),
        initial_goal: None,
        use_slash_commands: true,
        orchestrator_enabled: false,
        orchestrator_api_type: None,
        orchestrator_base_url: None,
        orchestrator_api_key: None,
        orchestrator_model: None,
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
        created_at: now,
        updated_at: now,
        pause_reason: None,
    };

    Workflow::create(pool, &workflow)
        .await
        .expect("Failed to create workflow");

    // Create workflow commands
    WorkflowCommand::create(
        pool,
        &workflow_id,
        &preset1.id,
        0,
        Some(r#"{"code_path": "src/main.rs"}"#),
    )
    .await
    .expect("Failed to create workflow command 1");

    WorkflowCommand::create(
        pool,
        &workflow_id,
        &preset2.id,
        1,
        Some(r#"{"module": "auth", "coverage": "80%"}"#),
    )
    .await
    .expect("Failed to create workflow command 2");

    // Verify commands
    let commands = WorkflowCommand::find_by_workflow(pool, &workflow_id)
        .await
        .expect("Failed to fetch commands");

    assert_eq!(commands.len(), 2, "Should have 2 commands");

    // Verify first command
    assert_eq!(commands[0].preset_id, preset1.id);
    assert_eq!(commands[0].order_index, 0);
    assert_eq!(
        commands[0].custom_params,
        Some(r#"{"code_path": "src/main.rs"}"#.to_string())
    );

    // Verify second command
    assert_eq!(commands[1].preset_id, preset2.id);
    assert_eq!(commands[1].order_index, 1);
    assert_eq!(
        commands[1].custom_params,
        Some(r#"{"module": "auth", "coverage": "80%"}"#.to_string())
    );
}

#[tokio::test]
async fn test_template_rendering() {
    use services::services::template_renderer::{TemplateRenderer, WorkflowContext};

    let renderer = TemplateRenderer::new();

    // Test 1: Simple template with custom params
    let template = "Review the code at {{path}}";
    let custom_params = r#"{"path": "src/lib.rs"}"#;
    let result = renderer
        .render(template, Some(custom_params), None)
        .expect("Failed to render template");
    assert_eq!(result, "Review the code at src/lib.rs");

    // Test 2: Template with workflow context
    let template = "Working on {{workflow.name}} targeting {{workflow.targetBranch}}";
    let workflow_ctx = WorkflowContext::new(
        "My Workflow".to_string(),
        Some("Test workflow".to_string()),
        "main".to_string(),
    );
    let result = renderer
        .render(template, None, Some(&workflow_ctx))
        .expect("Failed to render template");
    assert_eq!(result, "Working on My Workflow targeting main");

    // Test 3: Combined custom params and workflow context
    let template = "{{greeting}} {{workflow.name}}: {{instruction}}";
    let custom_params = r#"{"greeting": "Please", "instruction": "do the work"}"#;
    let workflow_ctx = WorkflowContext::new("Test WF".to_string(), None, "dev".to_string());
    let result = renderer
        .render(template, Some(custom_params), Some(&workflow_ctx))
        .expect("Failed to render template");
    assert_eq!(result, "Please Test WF: do the work");

    // Test 4: Invalid JSON should fail
    let template = "Hello {{name}}";
    let result = renderer.render(template, Some("invalid json"), None);
    assert!(result.is_err(), "Should fail with invalid JSON");

    // Test 5: Missing variable should fail (strict mode)
    let template = "Hello {{name}}";
    let result = renderer.render(template, None, None);
    assert!(
        result.is_err(),
        "Should fail with missing variable in strict mode"
    );
}

#[tokio::test]
async fn test_full_workflow_with_commands_api() {
    let (deployment, project_id) = setup_test().await;
    let pool = &deployment.db().pool;

    // Create test preset
    let preset = create_test_preset(
        pool,
        "/deploy",
        "Deploy to production",
        "Deploy {{service}} to {{env}} with {{strategy}} strategy",
    )
    .await;

    // Create workflow request with commands
    let workflow_id = Uuid::new_v4().to_string();
    let request = CreateWorkflowRequest {
        project_id: project_id.clone(),
        name: "Deploy Workflow".to_string(),
        description: Some("Deployment workflow".to_string()),
        execution_mode: "agent_planned".to_string(),
        initial_goal: Some("Deploy the configured service safely".to_string()),
        use_slash_commands: true,
        commands: Some(vec![WorkflowCommandRequest {
            preset_id: preset.id.clone(),
            custom_params: Some(
                r#"{"service": "api", "env": "prod", "strategy": "blue-green"}"#.to_string(),
            ),
        }]),
        orchestrator_config: None,
        error_terminal_config: None,
        merge_terminal_config: TerminalConfig {
            cli_type_id: "test-cli".to_string(),
            model_config_id: "test-model".to_string(),
            model_config: None,
            custom_base_url: None,
            custom_api_key: None,
        },
        target_branch: Some("main".to_string()),
        git_watcher_enabled: Some(true),
        tasks: vec![],
    };

    // Create workflow
    let now = chrono::Utc::now();
    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id: Uuid::parse_str(&project_id).expect("valid project id"),
        name: request.name.clone(),
        description: request.description.clone(),
        status: "created".to_string(),
        execution_mode: request.execution_mode.clone(),
        initial_goal: request.initial_goal.clone(),
        use_slash_commands: request.use_slash_commands,
        orchestrator_enabled: request.orchestrator_config.is_some(),
        orchestrator_api_type: request
            .orchestrator_config
            .as_ref()
            .map(|c| c.api_type.clone()),
        orchestrator_base_url: request
            .orchestrator_config
            .as_ref()
            .map(|c| c.base_url.clone()),
        orchestrator_api_key: None,
        orchestrator_model: request
            .orchestrator_config
            .as_ref()
            .map(|c| c.model.clone()),
        error_terminal_enabled: request.error_terminal_config.is_some(),
        error_terminal_cli_id: request
            .error_terminal_config
            .as_ref()
            .map(|c| c.cli_type_id.clone()),
        error_terminal_model_id: request
            .error_terminal_config
            .as_ref()
            .map(|c| c.model_config_id.clone()),
        merge_terminal_cli_id: request.merge_terminal_config.cli_type_id.clone(),
        merge_terminal_model_id: request.merge_terminal_config.model_config_id.clone(),
        target_branch: request.target_branch.unwrap_or_else(|| "main".to_string()),
        git_watcher_enabled: request.git_watcher_enabled.unwrap_or(true),
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
        pause_reason: None,
    };

    Workflow::create(pool, &workflow)
        .await
        .expect("Failed to create workflow");

    // Create commands
    if let Some(commands) = request.commands {
        for (index, cmd_req) in commands.iter().enumerate() {
            WorkflowCommand::create(
                pool,
                &workflow_id,
                &cmd_req.preset_id,
                index as i32,
                cmd_req.custom_params.as_deref(),
            )
            .await
            .expect("Failed to create workflow command");
        }
    }

    // Verify workflow was created with commands
    let commands = WorkflowCommand::find_by_workflow(pool, &workflow_id)
        .await
        .expect("Failed to fetch commands");

    assert_eq!(commands.len(), 1, "Should have 1 command");
    assert_eq!(commands[0].preset_id, preset.id);
    assert_eq!(
        commands[0].custom_params,
        Some(r#"{"service": "api", "env": "prod", "strategy": "blue-green"}"#.to_string())
    );
}

#[tokio::test]
async fn test_workflow_without_commands() {
    let (deployment, project_id) = setup_test().await;
    let pool = &deployment.db().pool;

    // Create workflow without commands
    let workflow_id = Uuid::new_v4().to_string();
    let now = chrono::Utc::now();

    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id: Uuid::parse_str(&project_id).expect("valid project id"),
        name: "No Commands Workflow".to_string(),
        description: None,
        status: "created".to_string(),
        execution_mode: "diy".to_string(),
        initial_goal: None,
        use_slash_commands: false,
        orchestrator_enabled: false,
        orchestrator_api_type: None,
        orchestrator_base_url: None,
        orchestrator_api_key: None,
        orchestrator_model: None,
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
        created_at: now,
        updated_at: now,
        pause_reason: None,
    };

    Workflow::create(pool, &workflow)
        .await
        .expect("Failed to create workflow");

    // Verify no commands
    let commands = WorkflowCommand::find_by_workflow(pool, &workflow_id)
        .await
        .expect("Failed to fetch commands");

    assert_eq!(commands.len(), 0, "Should have 0 commands");
}

#[tokio::test]
async fn test_system_preset_protection() {
    let (deployment, _) = setup_test().await;
    let pool = &deployment.db().pool;

    // Create a system preset via raw SQL
    let system_id = Uuid::new_v4().to_string();
    sqlx::query(
        r"INSERT INTO slash_command_preset (id, command, description, prompt_template, is_system, created_at, updated_at)
          VALUES (?1, ?2, ?3, ?4, 1, datetime('now'), datetime('now'))"
    )
    .bind(&system_id)
    .bind("/system-cmd")
    .bind("System command")
    .bind("System template")
    .execute(pool)
    .await
    .expect("Failed to create system preset");

    // List all presets - system presets should be included
    let presets = SlashCommandPreset::find_all(pool)
        .await
        .expect("Failed to list presets");

    // System presets are included in full list
    let system_presets: Vec<_> = presets.iter().filter(|p| p.is_system).collect();
    assert_eq!(system_presets.len(), 1, "Should have 1 system preset");
}

#[tokio::test]
async fn test_complex_template_rendering() {
    use services::services::template_renderer::TemplateRenderer;

    let renderer = TemplateRenderer::new();

    // Test nested JSON in custom params
    let template = "User: {{user.name}}, Email: {{user.email}}";
    let custom_params = r#"{"user": {"name": "Alice", "email": "alice@example.com"}}"#;
    let result = renderer
        .render(template, Some(custom_params), None)
        .expect("Failed to render template");
    assert_eq!(result, "User: Alice, Email: alice@example.com");

    // Test array access
    let template = "First item: {{items.0}}, Second item: {{items.1}}";
    let custom_params = r#"{"items": ["apple", "banana"]}"#;
    let result = renderer
        .render(template, Some(custom_params), None)
        .expect("Failed to render template");
    assert_eq!(result, "First item: apple, Second item: banana");

    // Test template with newlines and special characters
    let template = "Instructions:\n1. {{step1}}\n2. {{step2}}\n\nResult: {{result}}";
    let custom_params = r#"{"step1": "Prepare", "step2": "Execute", "result": "Success!"}"#;
    let result = renderer
        .render(template, Some(custom_params), None)
        .expect("Failed to render template");
    assert!(result.contains("Prepare"));
    assert!(result.contains("Execute"));
    assert!(result.contains("Success!"));
}
