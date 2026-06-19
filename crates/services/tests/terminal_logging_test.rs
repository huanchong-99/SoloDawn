use std::sync::Arc;

use chrono::Utc;
use db::{
    DBService,
    models::{Terminal, terminal::TerminalLog},
};
use services::services::terminal::process::TerminalLogger;

async fn setup_db() -> Arc<DBService> {
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

async fn create_terminal(db: &DBService) -> String {
    use uuid::Uuid;

    let terminal_id = Uuid::new_v4().to_string();

    // Create project first (workflow requires project_id)
    let project_id = Uuid::new_v4();
    sqlx::query("INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)")
        .bind(project_id)
        .bind("test-project")
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

    // Create workflow
    let wf_id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO workflow (id, project_id, name, status, merge_terminal_cli_id, merge_terminal_model_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(&wf_id)
    .bind(project_id)
    .bind("test")
    .bind("created")
    .bind("cli-claude-code")
    .bind("model-claude-sonnet")
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&db.pool)
    .await
    .unwrap();

    // Create workflow task
    let task_id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO workflow_task (id, workflow_id, name, branch, order_index, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
    )
    .bind(&task_id)
    .bind(&wf_id)
    .bind("task-1")
    .bind("main")
    .bind(0)
    .bind("pending")
    .bind(Utc::now())
    .bind(Utc::now())
    .execute(&db.pool)
    .await
    .unwrap();

    let terminal = Terminal {
        id: terminal_id.clone(),
        workflow_task_id: task_id,
        cli_type_id: "cli-claude-code".to_string(),
        model_config_id: "model-claude-sonnet".to_string(),
        custom_base_url: None,
        custom_api_key: None,
        role: None,
        role_description: None,
        order_index: 0,
        status: "running".to_string(),
        auto_confirm: true,
        process_id: None,
        pty_session_id: None,
        vk_session_id: None,
        session_id: None,
        execution_process_id: None,
        last_commit_hash: None,
        last_commit_message: None,
        started_at: Some(Utc::now()),
        completed_at: None,
        created_at: Utc::now(),
        updated_at: Utc::now(),
    };
    Terminal::create(&db.pool, &terminal).await.unwrap();

    terminal_id
}

#[tokio::test]
async fn test_terminal_output_logged() {
    let db = setup_db().await;
    let terminal_id = create_terminal(&db).await;

    // Log output
    TerminalLog::create(&db.pool, &terminal_id, "stdout", "test output")
        .await
        .unwrap();

    // Verify log exists
    let logs = TerminalLog::find_by_terminal(&db.pool, &terminal_id, Some(10))
        .await
        .unwrap();
    assert_eq!(logs.len(), 1);
    assert_eq!(logs[0].content, "test output");
}

#[tokio::test]
async fn test_terminal_logger_flushes_on_full_buffer() {
    let db = setup_db().await;
    let terminal_id = create_terminal(&db).await;

    let logger =
        TerminalLogger::with_max_buffer_size(Arc::clone(&db), terminal_id.clone(), "stdout", 60, 3);

    logger.append("line 1").await;
    logger.append("line 2").await;
    logger.append("line 3").await;

    // Poll until the async buffer flush persists all 3 rows (or deadline),
    // instead of a fixed sleep: fast on healthy runs, still fails a real regression.
    let deadline = tokio::time::Instant::now() + tokio::time::Duration::from_millis(500);
    let mut logs = TerminalLog::find_by_terminal(&db.pool, &terminal_id, Some(10))
        .await
        .unwrap();
    while logs.len() < 3 && tokio::time::Instant::now() < deadline {
        tokio::time::sleep(tokio::time::Duration::from_millis(5)).await;
        logs = TerminalLog::find_by_terminal(&db.pool, &terminal_id, Some(10))
            .await
            .unwrap();
    }
    assert_eq!(logs.len(), 3);
    let contents: Vec<&str> = logs.iter().map(|log| log.content.as_str()).collect();
    assert!(contents.contains(&"line 1"));
    assert!(contents.contains(&"line 2"));
    assert!(contents.contains(&"line 3"));
}
