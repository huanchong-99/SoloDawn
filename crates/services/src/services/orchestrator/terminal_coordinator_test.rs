//! Terminal Coordinator Tests
//!
//! Test terminal preparation sequence with status transitions.
//! Note: Model configuration is now handled at spawn time via environment variables,
//! not by the coordinator.

use std::sync::Arc;

use chrono::Utc;
use db::{
    DBService,
    models::{
        Terminal,
        cli_type::{CliType, ModelConfig},
    },
};
use sqlx::sqlite::SqlitePoolOptions;
use uuid::Uuid;

use crate::services::orchestrator::TerminalCoordinator;
#[allow(dead_code)]
// Helper function to create test database with real migrations
async fn setup_test_db() -> DBService {
    // Create in-memory SQLite pool
    let pool = SqlitePoolOptions::new()
        .max_connections(1)
        .connect("sqlite::memory:")
        .await
        .expect("Failed to create in-memory pool");

    // Run real migrations from db crate
    let migrations_path =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../db/migrations");

    let migrator = sqlx::migrate::Migrator::new(migrations_path)
        .await
        .expect("Failed to load migrations");

    migrator.run(&pool).await.expect("Failed to run migrations");

    DBService { pool }
}

// Helper function to create CLI type and model config
#[allow(dead_code)]
async fn create_cli_and_model(
    db: &DBService,
    cli_name: &str,
    model_name: &str,
) -> (CliType, ModelConfig) {
    let cli_id = Uuid::new_v4().to_string();
    let model_id = Uuid::new_v4().to_string();
    let now = Utc::now();

    let cli = CliType {
        id: cli_id.clone(),
        name: cli_name.to_string(),
        display_name: format!("{} CLI", cli_name.to_uppercase()),
        detect_command: format!("which {cli_name}"),
        install_command: None,
        install_guide_url: None,
        config_file_path: None,
        is_system: false,
        created_at: now,
    };

    sqlx::query(
        r"
        INSERT INTO cli_type (id, name, display_name, detect_command, install_command, install_guide_url, config_file_path, is_system, created_at)
        VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
        "
    )
    .bind(&cli.id)
    .bind(&cli.name)
    .bind(&cli.display_name)
    .bind(&cli.detect_command)
    .bind(&cli.install_command)
    .bind(&cli.install_guide_url)
    .bind(&cli.config_file_path)
    .bind(cli.is_system)
    .bind(cli.created_at)
    .execute(&db.pool)
    .await
    .expect("Failed to create CLI type");

    let model = ModelConfig {
        id: model_id.clone(),
        cli_type_id: cli_id,
        name: model_name.to_string(),
        display_name: format!("{model_name} Model"),
        api_model_id: Some(format!("{model_name}-api-id")),
        is_default: true,
        is_official: true,
        created_at: now,
        updated_at: now,
        encrypted_api_key: None,
        base_url: None,
        api_type: None,
        has_api_key: false,
    };

    sqlx::query(
        r"
        INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official, created_at, updated_at)
        VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
        "
    )
    .bind(&model.id)
    .bind(&model.cli_type_id)
    .bind(&model.name)
    .bind(&model.display_name)
    .bind(&model.api_model_id)
    .bind(model.is_default)
    .bind(model.is_official)
    .bind(model.created_at)
    .bind(model.updated_at)
    .execute(&db.pool)
    .await
    .expect("Failed to create model config");

    (cli, model)
}

#[allow(dead_code)]
// Helper function to create workflow with tasks and terminals
async fn create_workflow_with_terminals(
    db: &DBService,
    num_tasks: usize,
    terminals_per_task: usize,
) -> (String, Vec<String>) {
    let workflow_id = Uuid::new_v4().to_string();
    let project_id = Uuid::new_v4();
    let now = Utc::now();

    // First create a project (required by workflow foreign key)
    sqlx::query(r"INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)")
        .bind(project_id)
        .bind("Test Project")
        .bind(now)
        .bind(now)
        .execute(&db.pool)
        .await
        .expect("Failed to create project");

    // Use pre-seeded cli_type and model_config from migrations
    let merge_cli_id = "cli-claude-code";
    let merge_model_id = "model-claude-sonnet";

    // Create workflow
    sqlx::query(
        r"
        INSERT INTO workflow (
            id, project_id, name, description, status,
            use_slash_commands, orchestrator_enabled,
            orchestrator_api_type, orchestrator_base_url,
            orchestrator_api_key, orchestrator_model,
            error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
            merge_terminal_cli_id, merge_terminal_model_id,
            target_branch, created_at, updated_at
        ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19)
        "
    )
    .bind(&workflow_id)
    .bind(project_id)
    .bind("Test Workflow")
    .bind("Test Description")
    .bind("created")
    .bind(false)
    .bind(false)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind(false)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind(merge_cli_id)
    .bind(merge_model_id)
    .bind("main")
    .bind(now)
    .bind(now)
    .execute(&db.pool)
    .await
    .expect("Failed to create workflow");

    let mut terminal_ids = Vec::new();

    // Create tasks and terminals
    for task_idx in 0..num_tasks {
        let task_id = Uuid::new_v4().to_string();

        // Create task
        sqlx::query(
            r"
            INSERT INTO workflow_task (
                id, workflow_id, vk_task_id, name, description,
                branch, status, order_index, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
            ",
        )
        .bind(&task_id)
        .bind(&workflow_id)
        .bind::<Option<String>>(None)
        .bind(format!("Task {task_idx}"))
        .bind(format!("Description for task {task_idx}"))
        .bind(format!("task-{task_idx}"))
        .bind("pending")
        .bind(task_idx as i32)
        .bind(now)
        .bind(now)
        .execute(&db.pool)
        .await
        .expect("Failed to create task");

        // Create terminals for this task using pre-seeded cli_type and model_config
        for term_idx in 0..terminals_per_task {
            let terminal_id = Uuid::new_v4().to_string();
            // Use pre-seeded cli_type and model_config from migrations
            let cli_type_id = "cli-claude-code";
            let model_config_id = "model-claude-sonnet";

            sqlx::query(
                r"
                INSERT INTO terminal (
                    id, workflow_task_id, cli_type_id, model_config_id,
                    custom_base_url, custom_api_key, role, role_description,
                    order_index, status, created_at, updated_at
                ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
                ",
            )
            .bind(&terminal_id)
            .bind(&task_id)
            .bind(cli_type_id)
            .bind(model_config_id)
            .bind("https://api.test.com")
            .bind("test-api-key")
            .bind(format!("role-{term_idx}"))
            .bind(format!("Role description {term_idx}"))
            .bind(term_idx as i32)
            .bind("not_started")
            .bind(now)
            .bind(now)
            .execute(&db.pool)
            .await
            .expect("Failed to create terminal");

            terminal_ids.push(terminal_id);
        }
    }

    (workflow_id, terminal_ids)
}

#[tokio::test]
async fn test_terminal_startup_sequence_succeeds() {
    let db = setup_test_db().await;

    // No longer need MockCCSwitch - config is applied at spawn time
    let coordinator = TerminalCoordinator::new(Arc::new(db.clone()));

    // Create workflow with 2 tasks, each with 2 terminals
    let (workflow_id, terminal_ids) = create_workflow_with_terminals(&db, 2, 2).await;

    // Start terminals for workflow
    let result: Result<(), anyhow::Error> =
        coordinator.start_terminals_for_workflow(&workflow_id).await;

    // Should succeed
    assert!(result.is_ok(), "Terminal startup should succeed");

    // Verify terminals are in "starting" status
    for terminal_id in terminal_ids {
        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .expect("Failed to query terminal")
            .expect("Terminal not found");
        assert_eq!(
            terminal.status, "starting",
            "Terminal should be in starting status"
        );
    }
}

#[tokio::test]
async fn test_terminal_startup_updates_all_terminals() {
    let db = setup_test_db().await;

    let coordinator = TerminalCoordinator::new(Arc::new(db.clone()));

    // Create workflow with 2 tasks, each with 2 terminals (4 terminals total)
    let (workflow_id, terminal_ids) = create_workflow_with_terminals(&db, 2, 2).await;

    // Start terminals for workflow
    let result: Result<(), anyhow::Error> =
        coordinator.start_terminals_for_workflow(&workflow_id).await;

    // Should succeed
    assert!(result.is_ok(), "Terminal startup should succeed");

    // Verify all terminals are in "starting" status
    for terminal_id in terminal_ids {
        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .expect("Failed to query terminal")
            .expect("Terminal not found");
        assert_eq!(
            terminal.status, "starting",
            "Terminal should be in starting status"
        );
    }
}

#[tokio::test]
async fn test_terminal_startup_with_limit_queues_excess_terminals() {
    let db = setup_test_db().await;

    let coordinator = TerminalCoordinator::new(Arc::new(db.clone()));
    let (workflow_id, terminal_ids) = create_workflow_with_terminals(&db, 3, 2).await;

    let result = coordinator
        .start_terminals_for_workflow_with_limit(&workflow_id, 4)
        .await;

    assert!(result.is_ok(), "limited terminal startup should succeed");

    let mut starting = 0;
    let mut queued = 0;
    for terminal_id in terminal_ids {
        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .expect("Failed to query terminal")
            .expect("Terminal not found");
        match terminal.status.as_str() {
            "starting" => starting += 1,
            "not_started" => queued += 1,
            status => panic!("unexpected terminal status: {status}"),
        }
    }

    assert_eq!(starting, 4);
    assert_eq!(queued, 2);
}

#[tokio::test]
async fn test_empty_workflow_no_terminals() {
    let db = setup_test_db().await;
    let coordinator = TerminalCoordinator::new(Arc::new(db.clone()));

    // Create workflow with no tasks
    let workflow_id = Uuid::new_v4().to_string();
    let project_id = Uuid::new_v4();
    let now = Utc::now();

    // First create a project (required by workflow foreign key)
    sqlx::query(r"INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)")
        .bind(project_id)
        .bind("Test Project")
        .bind(now)
        .bind(now)
        .execute(&db.pool)
        .await
        .expect("Failed to create project");

    // Use pre-seeded cli_type and model_config from migrations
    let merge_cli_id = "cli-claude-code";
    let merge_model_id = "model-claude-sonnet";

    sqlx::query(
        r"
        INSERT INTO workflow (
            id, project_id, name, description, status,
            use_slash_commands, orchestrator_enabled,
            orchestrator_api_type, orchestrator_base_url,
            orchestrator_api_key, orchestrator_model,
            error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
            merge_terminal_cli_id, merge_terminal_model_id,
            target_branch, created_at, updated_at
        ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19)
        "
    )
    .bind(&workflow_id)
    .bind(project_id)
    .bind("Test Workflow")
    .bind("Test Description")
    .bind("created")
    .bind(false)
    .bind(false)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind(false)
    .bind::<Option<String>>(None)
    .bind::<Option<String>>(None)
    .bind(merge_cli_id)
    .bind(merge_model_id)
    .bind("main")
    .bind(now)
    .bind(now)
    .execute(&db.pool)
    .await
    .expect("Failed to create workflow");

    // Should succeed with no terminals to start
    let result = coordinator.start_terminals_for_workflow(&workflow_id).await;
    assert!(result.is_ok(), "Should succeed with no terminals");
}

#[tokio::test]
async fn test_single_terminal_startup() {
    let db = setup_test_db().await;

    let coordinator = TerminalCoordinator::new(Arc::new(db.clone()));

    // Create workflow with 1 task with 1 terminal
    let (workflow_id, terminal_ids) = create_workflow_with_terminals(&db, 1, 1).await;

    // Start terminals for workflow
    let result: Result<(), anyhow::Error> =
        coordinator.start_terminals_for_workflow(&workflow_id).await;

    // Should succeed
    assert!(result.is_ok(), "Single terminal startup should succeed");

    // Verify terminal is in "starting" status
    let terminal = Terminal::find_by_id(&db.pool, &terminal_ids[0])
        .await
        .expect("Failed to query terminal")
        .expect("Terminal not found");
    assert_eq!(
        terminal.status, "starting",
        "Terminal should be in starting status"
    );
}
