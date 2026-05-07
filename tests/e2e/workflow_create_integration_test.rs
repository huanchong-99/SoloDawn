use db::models::{Workflow, WorkflowTask, Terminal, CreateWorkflowRequest, CreateWorkflowTaskRequest, CreateTerminalRequest, TerminalConfig};
use sqlx::SqlitePool;
use uuid::Uuid;

#[sqlx::test]
async fn test_create_workflow_with_tasks_and_terminals(pool: SqlitePool) -> sqlx::Result<()> {
    // Use UUIDs instead of hardcoded IDs to avoid collisions across parallel test runs
    let cli_type_id = Uuid::new_v4().to_string();
    let model_config_id = Uuid::new_v4().to_string();

    // Setup: create CLI types and model configs
    sqlx::query(r"INSERT INTO cli_type (id, name, display_name, detect_command, is_system, created_at) VALUES (?, 'test', 'Test CLI', 'test --version', 1, datetime('now'))")
        .bind(&cli_type_id)
        .execute(&pool).await?;

    sqlx::query(r"INSERT INTO model_config (id, cli_type_id, name, display_name, is_default, is_official, created_at, updated_at) VALUES (?, ?, 'test', 'Test Model', 1, 1, datetime('now'), datetime('now'))")
        .bind(&model_config_id)
        .bind(&cli_type_id)
        .execute(&pool).await?;

    let req = CreateWorkflowRequest {
        project_id: Uuid::new_v4().to_string(),
        name: "Integration Test Workflow".to_string(),
        description: Some("Test workflow with tasks".to_string()),
        execution_mode: "diy".to_string(),
        initial_goal: None,
        use_slash_commands: false,
        commands: None,
        orchestrator_config: None,
        error_terminal_config: None,
        merge_terminal_config: TerminalConfig {
            cli_type_id: cli_type_id.clone(),
            model_config_id: model_config_id.clone(),
            model_config: None,
            custom_base_url: None,
            custom_api_key: None,
        },
        target_branch: Some("main".to_string()),
        git_watcher_enabled: Some(true),
        tasks: vec![
            CreateWorkflowTaskRequest {
                id: None,
                name: "Task 1".to_string(),
                description: Some("First task".to_string()),
                branch: None,
                order_index: 0,
                terminals: vec![
                    CreateTerminalRequest {
                        id: None,
                        cli_type_id: cli_type_id.clone(),
                        model_config_id: model_config_id.clone(),
                        custom_base_url: None,
                        custom_api_key: None,
                        role: Some("Writer".to_string()),
                        role_description: None,
                        auto_confirm: true,
                        order_index: 0,
                    }
                ],
            }
        ],
    };

    let workflow_id = Uuid::new_v4().to_string();
    let now = chrono::Utc::now();

    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id: Uuid::parse_str(&req.project_id).expect("valid project id"),
        name: req.name.clone(),
        description: req.description.clone(),
        status: "created".to_string(),
        execution_mode: req.execution_mode.clone(),
        initial_goal: req.initial_goal.clone(),
        use_slash_commands: req.use_slash_commands,
        orchestrator_enabled: false,
        orchestrator_api_type: None,
        orchestrator_base_url: None,
        orchestrator_api_key: None,
        orchestrator_model: None,
        error_terminal_enabled: false,
        error_terminal_cli_id: None,
        error_terminal_model_id: None,
        merge_terminal_cli_id: req.merge_terminal_config.cli_type_id.clone(),
        merge_terminal_model_id: req.merge_terminal_config.model_config_id.clone(),
        target_branch: req.target_branch.unwrap(),
        git_watcher_enabled: req.git_watcher_enabled.unwrap_or(true),
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
    };

    // Prepare tasks and terminals
    let mut task_rows: Vec<(WorkflowTask, Vec<Terminal>)> = Vec::new();

    for task_req in &req.tasks {
        let task_id = Uuid::new_v4().to_string();
        let branch = format!("workflow/{}/{}", workflow_id, 0);

        let task = WorkflowTask {
            id: task_id.clone(),
            workflow_id: workflow_id.clone(),
            vk_task_id: None,
            name: task_req.name.clone(),
            description: task_req.description.clone(),
            branch: branch.clone(),
            status: "pending".to_string(),
            order_index: task_req.order_index,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
        };

        let mut terminals: Vec<Terminal> = Vec::new();

        for terminal_req in &task_req.terminals {
            let terminal = Terminal {
                id: Uuid::new_v4().to_string(),
                workflow_task_id: task_id.clone(),
                cli_type_id: terminal_req.cli_type_id.clone(),
                model_config_id: terminal_req.model_config_id.clone(),
                custom_base_url: terminal_req.custom_base_url.clone(),
                custom_api_key: terminal_req.custom_api_key.clone(),
                role: terminal_req.role.clone(),
                role_description: terminal_req.role_description.clone(),
                order_index: terminal_req.order_index,
                status: "not_started".to_string(),
                process_id: None,
                session_id: None,
                started_at: None,
                completed_at: None,
                created_at: now,
                updated_at: now,
            };

            terminals.push(terminal);
        }

        task_rows.push((task, terminals));
    }

    // Execute transaction
    Workflow::create_with_tasks(&pool, &workflow, task_rows).await?;

    // Verify workflow created
    let saved_workflow = Workflow::find_by_id(&pool, &workflow_id).await?
        .ok_or(sqlx::Error::RowNotFound)?;

    assert_eq!(saved_workflow.name, "Integration Test Workflow");

    // Verify tasks created
    let tasks = WorkflowTask::find_by_workflow(&pool, &workflow_id).await?;
    assert_eq!(tasks.len(), 1);
    assert_eq!(tasks[0].name, "Task 1");

    // Verify terminals created
    let terminals = Terminal::find_by_task(&pool, &tasks[0].id).await?;
    assert_eq!(terminals.len(), 1);
    assert_eq!(terminals[0].role, Some("Writer".to_string()));

    Ok(())
}
