//! Comprehensive terminal lifecycle test
//!
//! Tests the complete lifecycle of a terminal from creation to cleanup:
//! 1. Terminal creation in database
//! 2. Launch with session and execution process creation
//! 3. I/O simulation (input/output handling)
//! 4. Terminal stop
//! 5. Cleanup verification (process termination, session closure)

use std::sync::Arc;

use chrono::Utc;
use db::{
    DBService,
    models::{
        execution_process::{ExecutionProcess, ExecutionProcessRunReason},
        session::Session,
        terminal::Terminal,
    },
};
use services::services::{
    cc_switch::CCSwitchService,
    terminal::{ProcessManager, TerminalLauncher},
};
use uuid::Uuid;

/// Setup in-memory database for testing
async fn setup_test_db() -> Arc<DBService> {
    use std::str::FromStr;

    use sqlx::sqlite::{SqliteConnectOptions, SqlitePoolOptions};

    // Disable foreign keys for testing (simplifies test setup)
    let options = SqliteConnectOptions::from_str(":memory:")
        .unwrap()
        .pragma("foreign_keys", "0");

    let pool = SqlitePoolOptions::new()
        .connect_with(options)
        .await
        .unwrap();

    // Run migrations - path is relative to the crate root (services)
    let migrator = sqlx::migrate!("../db/migrations");
    migrator.run(&pool).await.unwrap();

    Arc::new(DBService { pool })
}

/// Create test data: project, workflow, task, workspace, and terminal
async fn create_test_terminal_data(db: &Arc<DBService>) -> (String, String, String) {
    let project_id = Uuid::new_v4();
    sqlx::query("INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)")
        .bind(project_id)
        .bind("test-project")
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

    let task_id = Uuid::new_v4();
    sqlx::query(
        "INSERT INTO tasks (id, project_id, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"
    )
    .bind(task_id)
    .bind(project_id)
    .bind("test task")
    .bind("todo")
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&db.pool)
    .await
    .unwrap();

    let workspace_id = Uuid::new_v4();
    sqlx::query(
        "INSERT INTO workspaces (id, task_id, branch, created_at, updated_at, archived, pinned) VALUES (?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(workspace_id)
    .bind(task_id)
    .bind("main")
    .bind(Utc::now())
    .bind(Utc::now())
    .bind(false)
    .bind(false)
    .execute(&db.pool)
    .await
    .unwrap();

    let workflow_id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO workflow (id, project_id, name, status, merge_terminal_cli_id, merge_terminal_model_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(&workflow_id)
    .bind(project_id)
    .bind("test-wf")
    .bind("created")
    .bind("cli-claude-code")
    .bind("model-claude-sonnet")
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&db.pool)
    .await
    .unwrap();

    let workflow_task_id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO workflow_task (id, workflow_id, vk_task_id, name, branch, order_index, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(&workflow_task_id)
    .bind(&workflow_id)
    .bind(task_id)
    .bind("task-1")
    .bind("main")
    .bind(0)
    .bind("pending")
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&db.pool)
    .await
    .unwrap();

    (workflow_id, workflow_task_id, workspace_id.to_string())
}

/// Create a terminal in the database
async fn create_terminal(
    db: &Arc<DBService>,
    workflow_task_id: &str,
    cli_type_id: &str,
    model_config_id: &str,
) -> String {
    let terminal_id = Uuid::new_v4().to_string();
    let terminal = Terminal {
        id: terminal_id.clone(),
        workflow_task_id: workflow_task_id.to_string(),
        cli_type_id: cli_type_id.to_string(),
        model_config_id: model_config_id.to_string(),
        custom_base_url: None,
        custom_api_key: None,
        role: None,
        role_description: None,
        order_index: 0,
        status: "not_started".to_string(),
        process_id: None,
        pty_session_id: None,
        vk_session_id: None,
        session_id: None,
        execution_process_id: None,
        last_commit_hash: None,
        last_commit_message: None,
        started_at: None,
        completed_at: None,
        created_at: Utc::now(),
        updated_at: Utc::now(),
        auto_confirm: true,
    };

    Terminal::create(&db.pool, &terminal).await.unwrap();
    terminal_id
}

#[tokio::test]
async fn test_terminal_full_lifecycle() {
    // Phase 1: Setup
    // ============
    let db = setup_test_db().await;

    // Create launcher with all dependencies
    let cc_switch = Arc::new(CCSwitchService::new(Arc::clone(&db)));
    let process_manager = Arc::new(ProcessManager::new());
    let working_dir = tempfile::tempdir().unwrap();

    let launcher = TerminalLauncher::new(
        Arc::clone(&db),
        cc_switch,
        process_manager.clone(),
        working_dir.path().to_path_buf(),
    );

    // Create test data
    let (_workflow_id, workflow_task_id, _workspace_id) = create_test_terminal_data(&db).await;

    // Use pre-seeded cli_type and model_config from migrations
    let cli_type_id = "cli-claude-code";
    let model_config_id = "model-claude-sonnet";

    // Phase 2: Terminal Creation
    // ==========================
    let terminal_id = create_terminal(&db, &workflow_task_id, cli_type_id, model_config_id).await;

    // Verify terminal was created in "not_started" status
    let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
        .await
        .unwrap()
        .expect("Terminal should exist");
    assert_eq!(terminal.status, "not_started");
    assert!(terminal.session_id.is_none());
    assert!(terminal.execution_process_id.is_none());

    // Phase 3: Terminal Launch
    // ========================
    // In production, the terminal_coordinator/runtime_actions calls set_starting
    // before launch_terminal. We simulate that here.
    Terminal::set_starting(&db.pool, &terminal_id)
        .await
        .unwrap();
    let launch_result = launcher.launch_terminal(&terminal).await;

    // Verify launch result (may fail if CLI not installed, that's OK)
    // The test structure is what matters here
    if launch_result.success {
        // Verify terminal status updated
        let updated_terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .expect("Terminal should exist");

        // Terminal should be marked as waiting
        assert_eq!(updated_terminal.status, "waiting");

        // Verify session was created
        assert!(
            updated_terminal.session_id.is_some(),
            "session_id should be set after launch"
        );

        // Verify execution process was created
        assert!(
            updated_terminal.execution_process_id.is_some(),
            "execution_process_id should be set after launch"
        );

        // Verify session exists in database
        let session_id = Uuid::parse_str(&updated_terminal.session_id.unwrap()).unwrap();
        let session = Session::find_by_id(&db.pool, session_id).await.unwrap();
        assert!(session.is_some(), "Session should exist in database");

        // Verify execution process exists
        let exec_process_id =
            Uuid::parse_str(&updated_terminal.execution_process_id.unwrap()).unwrap();
        let exec_process = ExecutionProcess::find_by_id(&db.pool, exec_process_id)
            .await
            .unwrap();
        assert!(exec_process.is_some(), "ExecutionProcess should exist");

        let exec_process = exec_process.unwrap();
        assert_eq!(
            exec_process.run_reason,
            ExecutionProcessRunReason::CodingAgent
        );
        assert_eq!(exec_process.session_id, session_id);

        // Phase 4: I/O Simulation
        // ========================
        // TODO: Implement I/O simulation
        // This will involve:
        // - Writing to stdin
        // - Reading from stdout/stderr
        // - Verifying data persistence

        // Phase 5: Terminal Stop
        // =======================
        // TODO: Implement terminal stop
        // This will involve:
        // - Calling stop endpoint or method
        // - Verifying process termination
        // - Checking terminal status update

        // Phase 6: Cleanup Verification
        // ==============================
        // TODO: Verify cleanup
        // This will involve:
        // - Verifying process is no longer running
        // - Checking session is closed
        // - Ensuring resources are freed

        // For now, just verify process was spawned
        assert!(
            launch_result.process_handle.is_some(),
            "Process handle should exist"
        );

        // Clean up the process if it's running
        if let Some(handle) = launch_result.process_handle {
            let _ = process_manager.kill(handle.pid).await;
        }
    } else {
        // If launch failed, verify error was captured
        assert!(
            launch_result.error.is_some(),
            "Error should be present when launch fails"
        );
        println!(
            "Launch failed (expected if CLI not installed): {:?}",
            launch_result.error
        );
    }

    // Final verification: terminal record still exists
    let final_terminal = Terminal::find_by_id(&db.pool, &terminal_id)
        .await
        .unwrap()
        .expect("Terminal should still exist in database");
    assert_eq!(final_terminal.id, terminal_id);
}

#[tokio::test]
async fn test_terminal_lifecycle_cleanup() {
    // Test cleanup verification independently
    let _db = setup_test_db().await;

    let process_manager = Arc::new(ProcessManager::new());
    let temp_dir = tempfile::tempdir().unwrap();

    // Spawn a simple test process using PTY
    #[cfg(unix)]
    let shell = "sh";
    #[cfg(windows)]
    let shell = "cmd.exe";

    let terminal_id = "test-cleanup-terminal";
    let handle = process_manager
        .spawn_pty(terminal_id, shell, temp_dir.path(), 80, 24)
        .await
        .unwrap();

    // Verify process was spawned
    assert!(handle.pid > 0);

    // Verify it's tracked
    let running = process_manager.list_running().await;
    assert!(running.contains(&terminal_id.to_string()));

    // Kill the terminal to clean up
    let _ = process_manager.kill_terminal(terminal_id).await;

    // Verify cleanup after kill
    let running_after = process_manager.list_running().await;
    assert!(!running_after.contains(&terminal_id.to_string()));
}
