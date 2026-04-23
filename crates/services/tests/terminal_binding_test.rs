use std::{
    str::FromStr,
    sync::{Arc, Mutex, MutexGuard},
};

use chrono::Utc;
use db::{
    DBService,
    models::{Terminal, execution_process::ExecutionProcess, session::Session},
};
use once_cell::sync::Lazy;
use services::services::terminal::TerminalLauncher;
use sqlx::sqlite::{SqliteConnectOptions, SqlitePoolOptions};
use uuid::Uuid;

/// Mutex to serialize environment variable access across tests in this file.
/// Ensures tests that mutate env vars run sequentially and restore on drop.
static ENV_LOCK: Lazy<Mutex<()>> = Lazy::new(|| Mutex::new(()));

/// RAII guard for environment variables. Restores the previous value on drop
/// and holds a global lock so concurrent tests cannot race on env mutation.
struct EnvVarGuard {
    key: &'static str,
    prev: Option<String>,
    _lock: MutexGuard<'static, ()>,
}

impl EnvVarGuard {
    fn set(key: &'static str, value: &str) -> Self {
        let lock = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        let prev = std::env::var(key).ok();
        unsafe { std::env::set_var(key, value) };
        Self {
            key,
            prev,
            _lock: lock,
        }
    }
}

impl Drop for EnvVarGuard {
    fn drop(&mut self) {
        match &self.prev {
            Some(value) => unsafe { std::env::set_var(self.key, value) },
            None => unsafe { std::env::remove_var(self.key) },
        }
    }
}

#[tokio::test]
async fn test_terminal_launch_creates_session() {
    // Setup encryption key for API key encryption (restored on drop).
    let _env = EnvVarGuard::set(
        "SOLODAWN_ENCRYPTION_KEY",
        "12345678901234567890123456789012",
    );

    // Setup: Create in-memory DB with migrations
    let options = SqliteConnectOptions::from_str(":memory:")
        .unwrap()
        .pragma("foreign_keys", "0");

    let pool = SqlitePoolOptions::new()
        .connect_with(options)
        .await
        .unwrap();

    // Run migrations
    let migrator = sqlx::migrate!("../db/migrations");
    migrator.run(&pool).await.unwrap();

    let db = Arc::new(DBService { pool: pool.clone() });
    let cc_switch = Arc::new(services::services::cc_switch::CCSwitchService::new(
        Arc::clone(&db),
    ));
    let process_manager = Arc::new(services::services::terminal::ProcessManager::new());
    let working_dir = std::env::temp_dir();
    let launcher = TerminalLauncher::new(Arc::clone(&db), cc_switch, process_manager, working_dir);

    // Create test project
    let project_id = Uuid::new_v4();
    sqlx::query("INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)")
        .bind(project_id)
        .bind("test-project")
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&pool)
        .await
        .unwrap();

    // Create task
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
    .execute(&pool)
    .await
    .unwrap();

    // Create workspace
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
    .execute(&pool)
    .await
    .unwrap();

    // Use pre-seeded CLI type and model config from migrations
    let cli_type_id = "cli-claude-code";
    let model_config_id = "model-claude-sonnet";

    // Create workflow (requires project_id foreign key)
    let wf_id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO workflow (id, project_id, name, status, merge_terminal_cli_id, merge_terminal_model_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(&wf_id)
    .bind(project_id)
    .bind("test-wf")
    .bind("created")
    .bind(cli_type_id)
    .bind(model_config_id)
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&pool)
    .await
    .unwrap();

    // Create workflow task
    let workflow_task_id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO workflow_task (id, workflow_id, vk_task_id, name, branch, order_index, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(&workflow_task_id)
    .bind(&wf_id)
    .bind(task_id)
    .bind("task-1")
    .bind("main")
    .bind(0)
    .bind("pending")
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&pool)
    .await
    .unwrap();

    // Create terminal
    let terminal_id = Uuid::new_v4().to_string();
    let mut terminal = Terminal {
        id: terminal_id.clone(),
        workflow_task_id,
        cli_type_id: cli_type_id.to_string(),
        model_config_id: model_config_id.to_string(),
        custom_base_url: None,
        custom_api_key: None,
        role: None,
        role_description: None,
        order_index: 0,
        status: "not_started".to_string(),
        auto_confirm: true,
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
    };
    // Set encrypted API key for cc_switch
    terminal
        .set_custom_api_key("test-api-key-12345")
        .expect("Failed to set API key");
    Terminal::create(&pool, &terminal).await.unwrap();

    // Execute launch_terminal
    let result = launcher.launch_terminal(&terminal).await;

    // Verify session was created
    let updated_terminal = Terminal::find_by_id(&pool, &terminal_id)
        .await
        .unwrap()
        .unwrap();

    assert!(
        updated_terminal.session_id.is_some(),
        "session_id should be set"
    );
    assert!(
        updated_terminal.execution_process_id.is_some(),
        "execution_process_id should be set"
    );

    // Verify session exists in database
    let session_id = Uuid::parse_str(&updated_terminal.session_id.unwrap()).unwrap();
    let session = Session::find_by_id(&pool, session_id).await.unwrap();
    assert!(session.is_some(), "Session should exist in database");
    let session = session.unwrap();
    assert_eq!(session.workspace_id, workspace_id);

    // Verify execution process exists
    let exec_process_id = Uuid::parse_str(&updated_terminal.execution_process_id.unwrap()).unwrap();
    let exec_process = ExecutionProcess::find_by_id(&pool, exec_process_id)
        .await
        .unwrap();
    assert!(exec_process.is_some(), "ExecutionProcess should exist");
    let exec_process = exec_process.unwrap();
    assert_eq!(exec_process.session_id, session_id);
    assert_eq!(
        exec_process.run_reason,
        db::models::execution_process::ExecutionProcessRunReason::CodingAgent
    );

    // Process should be spawned
    assert!(result.success, "Launch should succeed");
    assert!(
        result.process_handle.is_some(),
        "Process handle should exist"
    );
}
