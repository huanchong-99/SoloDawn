#![warn(clippy::pedantic)]
#![allow(
    clippy::doc_markdown,
    clippy::module_name_repetitions,
    clippy::must_use_candidate,
    clippy::missing_errors_doc,
    clippy::missing_panics_doc,
    clippy::similar_names,
    clippy::too_many_lines
)]

use std::{str::FromStr, sync::Arc, time::Duration};

use sqlx::{
    Error, Pool, Sqlite, SqlitePool,
    migrate::MigrateError,
    sqlite::{SqliteConnectOptions, SqliteConnection, SqliteJournalMode, SqlitePoolOptions},
};
use utils::assets::asset_dir;

pub mod encryption;
pub mod models;

struct ExpectedColumn {
    table: &'static str,
    column: &'static str,
    col_type: &'static str,
    default: Option<&'static str>,
}

const SCHEMA_EXPECTATIONS: &[ExpectedColumn] = &[
    ExpectedColumn { table: "workflow", column: "orchestrator_state", col_type: "TEXT", default: None },
    ExpectedColumn { table: "workflow", column: "orchestrator_api_key_encrypted", col_type: "TEXT", default: None },
    ExpectedColumn { table: "workflow", column: "git_watcher_enabled", col_type: "INTEGER", default: Some("1") },
    ExpectedColumn { table: "workflow", column: "execution_mode", col_type: "TEXT", default: Some("'diy'") },
    ExpectedColumn { table: "workflow", column: "initial_goal", col_type: "TEXT", default: None },
    ExpectedColumn { table: "workflow", column: "pause_reason", col_type: "TEXT", default: None },
    ExpectedColumn { table: "model_config", column: "encrypted_api_key", col_type: "TEXT", default: None },
    ExpectedColumn { table: "model_config", column: "base_url", col_type: "TEXT", default: None },
    ExpectedColumn { table: "model_config", column: "api_type", col_type: "TEXT", default: None },
];

async fn verify_schema(pool: &Pool<Sqlite>) -> Result<(), Error> {
    let mut healed = 0u32;

    for exp in SCHEMA_EXPECTATIONS {
        let exists: bool = sqlx::query_scalar::<_, i32>(&format!(
            "SELECT COUNT(*) FROM pragma_table_info('{}') WHERE name = '{}'",
            exp.table, exp.column
        ))
        .fetch_one(pool)
        .await
        .map(|n| n > 0)
        .unwrap_or(false);

        if !exists {
            let default_clause = exp.default.map_or_else(String::new, |d| format!(" DEFAULT {d}"));
            let sql = format!(
                "ALTER TABLE {} ADD COLUMN {} {}{}",
                exp.table, exp.column, exp.col_type, default_clause
            );

            match sqlx::query(&sql).execute(pool).await {
                Ok(_) => {
                    tracing::warn!(
                        table = exp.table,
                        column = exp.column,
                        "Startup self-check: missing column detected and auto-healed"
                    );
                    healed += 1;
                }
                Err(e) => {
                    tracing::error!(
                        table = exp.table,
                        column = exp.column,
                        error = %e,
                        "Startup self-check: failed to auto-heal missing column"
                    );
                    return Err(e);
                }
            }
        }
    }

    if healed > 0 {
        tracing::warn!(
            healed_count = healed,
            "Startup self-check: {healed} missing column(s) were auto-healed. \
             This indicates stale compiled migrations. Run: cargo clean -p db && cargo build"
        );
    } else {
        tracing::info!("Startup self-check: schema verification passed ({} columns verified)", SCHEMA_EXPECTATIONS.len());
    }

    Ok(())
}

async fn run_migrations(pool: &Pool<Sqlite>) -> Result<(), Error> {
    use std::collections::HashSet;

    let migrator = sqlx::migrate!("./migrations");
    let mut processed_versions: HashSet<i64> = HashSet::new();

    let before: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM _sqlx_migrations")
        .fetch_one(pool)
        .await
        .unwrap_or(0);

    loop {
        match migrator.run(pool).await {
            Ok(()) => {
                let after: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM _sqlx_migrations")
                    .fetch_one(pool)
                    .await
                    .unwrap_or(0);
                let new_migrations = after - before;

                if new_migrations > 0 || before == 0 {
                    tracing::info!(applied = new_migrations, "New migrations applied, running schema self-check");
                    verify_schema(pool).await?;
                }

                return Ok(());
            }
            Err(MigrateError::VersionMismatch(version)) => {
                if !cfg!(windows) {
                    // On non-Windows platforms, we do not attempt to auto-fix checksum mismatches.
                    // This keeps checksum drift visible in development and CI.
                    return Err(sqlx::Error::Migrate(Box::new(
                        MigrateError::VersionMismatch(version),
                    )));
                }

                // Guard against infinite loop
                if !processed_versions.insert(version) {
                    return Err(sqlx::Error::Migrate(Box::new(
                        MigrateError::VersionMismatch(version),
                    )));
                }

                // On Windows, there can be checksum mismatches due to line ending differences
                // or other platform-specific issues. Update the stored checksum and retry.
                tracing::warn!(
                    "Migration version {} has checksum mismatch, updating stored checksum (likely platform-specific difference)",
                    version
                );

                // Find the migration with the mismatched version and get its current checksum
                if let Some(migration) = migrator.iter().find(|m| m.version == version) {
                    // Windows line-ending workaround: CRLF on checkout causes embedded
                    // migration bytes to differ from the checksum stored in
                    // `_sqlx_migrations`, even when the logical SQL is unchanged.
                    // We use dynamic `sqlx::query` (not the `query!` macro) here on
                    // purpose so that this self-healing path does not depend on
                    // sqlx offline query caching (no `.sqlx/` entry is generated
                    // or required), keeping builds reproducible without a live
                    // database. The parameters are fully bound, so this is not
                    // vulnerable to SQL injection.
                    sqlx::query("UPDATE _sqlx_migrations SET checksum = ? WHERE version = ?")
                        .bind(&*migration.checksum)
                        .bind(version)
                        .execute(pool)
                        .await?;
                } else {
                    // Migration not found in current set, can't fix
                    return Err(sqlx::Error::Migrate(Box::new(
                        MigrateError::VersionMismatch(version),
                    )));
                }
            }
            Err(e) => return Err(e.into()),
        }
    }
}

#[derive(Clone)]
pub struct DBService {
    pub pool: Pool<Sqlite>,
}

impl DBService {
    pub async fn new() -> Result<DBService, Error> {
        let database_path = asset_dir()?.join("db.sqlite");
        let database_url = format!("sqlite://{}", database_path.to_string_lossy());
        let options = SqliteConnectOptions::from_str(&database_url)?
            .create_if_missing(true)
            .journal_mode(SqliteJournalMode::Delete);
        let pool = SqlitePool::connect_with(options).await?;
        run_migrations(&pool).await?;
        Ok(DBService { pool })
    }

    pub async fn new_with_after_connect<F>(after_connect: F) -> Result<DBService, Error>
    where
        F: for<'a> Fn(
                &'a mut SqliteConnection,
            ) -> std::pin::Pin<
                Box<dyn std::future::Future<Output = Result<(), Error>> + Send + 'a>,
            > + Send
            + Sync
            + 'static,
    {
        let pool = Self::create_pool(Some(Arc::new(after_connect))).await?;
        Ok(DBService { pool })
    }

    async fn create_pool<F>(after_connect: Option<Arc<F>>) -> Result<Pool<Sqlite>, Error>
    where
        F: for<'a> Fn(
                &'a mut SqliteConnection,
            ) -> std::pin::Pin<
                Box<dyn std::future::Future<Output = Result<(), Error>> + Send + 'a>,
            > + Send
            + Sync
            + 'static,
    {
        let database_path = asset_dir()?.join("db.sqlite");
        let database_url = format!("sqlite://{}", database_path.to_string_lossy());
        let options = SqliteConnectOptions::from_str(&database_url)?
            .create_if_missing(true)
            .journal_mode(SqliteJournalMode::Delete);

        // Configure connection pool for optimal performance
        // - max_connections: 10 (SQLite performs best with limited connections)
        // - min_connections: 2 (maintain baseline connectivity)
        // - acquire_timeout: 30s (prevent indefinite blocking)
        // - idle_timeout: 10min (release unused connections)
        // - max_lifetime: 1hr (refresh connections periodically)
        // - test_before_acquire: true (ensure connection health)
        let pool = if let Some(hook) = after_connect {
            SqlitePoolOptions::new()
                .max_connections(10)
                .min_connections(2)
                .acquire_timeout(Duration::from_secs(30))
                .idle_timeout(Duration::from_secs(600))
                .max_lifetime(Duration::from_secs(3600))
                .test_before_acquire(true)
                .after_connect(move |conn, _meta| {
                    let hook = hook.clone();
                    Box::pin(async move {
                        hook(conn).await?;
                        Ok(())
                    })
                })
                .connect_with(options)
                .await?
        } else {
            SqlitePoolOptions::new()
                .max_connections(10)
                .min_connections(2)
                .acquire_timeout(Duration::from_secs(30))
                .idle_timeout(Duration::from_secs(600))
                .max_lifetime(Duration::from_secs(3600))
                .test_before_acquire(true)
                .connect_with(options)
                .await?
        };

        run_migrations(&pool).await?;
        Ok(pool)
    }
}

#[cfg(test)]
mod tests {
    use chrono::Utc;
    use sqlx::Row;
    use uuid::Uuid;

    use super::*;
    use crate::models::{
        git_event::GitEvent,
        merge::Merge,
        project::{CreateProject, Project},
        task::{CreateTask, Task, TaskStatus},
        terminal::Terminal,
        workflow::{Workflow, WorkflowTask},
        workspace::{CreateWorkspace, Workspace},
    };

    async fn setup_pool() -> Result<SqlitePool, sqlx::Error> {
        let pool = SqlitePool::connect("sqlite::memory:").await?;
        run_migrations(&pool).await?;
        Ok(pool)
    }

    async fn create_merge_fixture(
        pool: &SqlitePool,
    ) -> Result<(Uuid, Uuid, Uuid), sqlx::Error> {
        let project_id = Uuid::new_v4();
        Project::create(
            pool,
            &CreateProject {
                name: "Merge Fixture Project".to_string(),
                repositories: vec![],
            },
            project_id,
        )
        .await?;

        let task_id = Uuid::new_v4();
        Task::create(
            pool,
            &CreateTask {
                project_id,
                title: "Merge Fixture Task".to_string(),
                description: None,
                status: Some(TaskStatus::Todo),
                parent_workspace_id: None,
                image_ids: None,
                shared_task_id: None,
            },
            task_id,
        )
        .await?;

        let workspace_id = Uuid::new_v4();
        Workspace::create(
            pool,
            &CreateWorkspace {
                branch: "main".to_string(),
                agent_working_dir: None,
            },
            workspace_id,
            task_id,
        )
        .await
        .expect("workspace fixture should be created");

        let repo_id = Uuid::new_v4();
        sqlx::query(
            r"INSERT INTO repos (id, path, name, display_name)
               VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(repo_id)
        .bind(format!("C:/tmp/repo-{repo_id}"))
        .bind("repo")
        .bind("Repo")
        .execute(pool)
        .await?;

        Merge::create_direct(pool, workspace_id, repo_id, "main", "deadbeef").await?;

        Ok((repo_id, workspace_id, task_id))
    }

    async fn seed_cli_and_model(
        pool: &SqlitePool,
        cli_type_id: &str,
        model_config_id: &str,
    ) -> Result<(), sqlx::Error> {
        sqlx::query(
            r"INSERT INTO cli_type (id, name, display_name, detect_command, is_system, created_at)
               VALUES (?1, ?2, ?3, ?4, 0, datetime('now'))",
        )
        .bind(cli_type_id)
        .bind(cli_type_id)
        .bind("Test CLI")
        .bind("echo test")
        .execute(pool)
        .await?;

        sqlx::query(
            r"INSERT INTO model_config (id, cli_type_id, name, display_name, is_default, is_official, created_at, updated_at)
               VALUES (?1, ?2, ?3, ?4, 0, 0, datetime('now'), datetime('now'))",
        )
        .bind(model_config_id)
        .bind(cli_type_id)
        .bind(model_config_id)
        .bind("Test Model")
        .execute(pool)
        .await?;

        Ok(())
    }

    async fn create_git_event_fixture(pool: &SqlitePool) -> Result<(String, String), sqlx::Error> {
        let project_id = Uuid::new_v4();
        Project::create(
            pool,
            &CreateProject {
                name: "Git Event Fixture Project".to_string(),
                repositories: vec![],
            },
            project_id,
        )
        .await?;

        let cli_type_id = format!("cli-{}", Uuid::new_v4().simple());
        let model_config_id = format!("model-{}", Uuid::new_v4().simple());
        seed_cli_and_model(pool, &cli_type_id, &model_config_id).await?;

        let now = Utc::now();
        let workflow = Workflow {
            id: Uuid::new_v4().to_string(),
            project_id,
            name: "Git Event Workflow".to_string(),
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
            merge_terminal_cli_id: cli_type_id.clone(),
            merge_terminal_model_id: model_config_id.clone(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: None,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
            pause_reason: None,
        };
        Workflow::create(pool, &workflow).await?;

        let workflow_task = WorkflowTask {
            id: Uuid::new_v4().to_string(),
            workflow_id: workflow.id.clone(),
            vk_task_id: None,
            name: "Git Event Task".to_string(),
            description: None,
            branch: "main".to_string(),
            status: "pending".to_string(),
            order_index: 0,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
        };
        WorkflowTask::create(pool, &workflow_task).await?;

        let terminal = Terminal {
            id: Uuid::new_v4().to_string(),
            workflow_task_id: workflow_task.id.clone(),
            cli_type_id: cli_type_id.clone(),
            model_config_id: model_config_id.clone(),
            custom_base_url: None,
            custom_api_key: None,
            role: None,
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
            created_at: now,
            updated_at: now,
        };
        Terminal::create(pool, &terminal).await?;

        let git_event = GitEvent {
            id: Uuid::new_v4().to_string(),
            workflow_id: workflow.id,
            terminal_id: Some(terminal.id.clone()),
            commit_hash: "deadbeef".to_string(),
            branch: "main".to_string(),
            commit_message: "Test commit".to_string(),
            metadata: None,
            process_status: "pending".to_string(),
            agent_response: None,
            created_at: now,
            processed_at: None,
        };
        GitEvent::insert(pool, &git_event).await?;

        Ok((terminal.id, git_event.id))
    }

    #[tokio::test]
    async fn workflow_project_created_index_exists() -> Result<(), sqlx::Error> {
        let pool = setup_pool().await?;

        let index_names: Vec<String> = sqlx::query("PRAGMA index_list('workflow')")
            .fetch_all(&pool)
            .await?
            .iter()
            .map(|row| row.get::<String, _>("name"))
            .collect();

        assert!(
            index_names.contains(&"idx_workflow_project_created".to_string()),
            "Expected idx_workflow_project_created in workflow indexes: {index_names:?}"
        );

        Ok(())
    }

    #[tokio::test]
    async fn performance_indexes_exist() -> Result<(), sqlx::Error> {
        let pool = setup_pool().await?;

        let execution_indexes: Vec<String> = sqlx::query("PRAGMA index_list('execution_processes')")
            .fetch_all(&pool)
            .await?
            .iter()
            .map(|row| row.get::<String, _>("name"))
            .collect();
        assert!(
            execution_indexes.contains(&"idx_exec_proc_status_running".to_string()),
            "Expected idx_exec_proc_status_running in execution_processes indexes: {execution_indexes:?}"
        );

        let task_indexes: Vec<String> = sqlx::query("PRAGMA index_list('tasks')")
            .fetch_all(&pool)
            .await?
            .iter()
            .map(|row| row.get::<String, _>("name"))
            .collect();
        assert!(
            task_indexes.contains(&"idx_tasks_shared_task_id".to_string()),
            "Expected idx_tasks_shared_task_id in tasks indexes: {task_indexes:?}"
        );

        let concierge_indexes: Vec<String> = sqlx::query("PRAGMA index_list('concierge_session')")
            .fetch_all(&pool)
            .await?
            .iter()
            .map(|row| row.get::<String, _>("name"))
            .collect();
        assert!(
            concierge_indexes.contains(&"idx_concierge_session_updated_at".to_string()),
            "Expected idx_concierge_session_updated_at in concierge_session indexes: {concierge_indexes:?}"
        );

        Ok(())
    }

    #[tokio::test]
    async fn deleting_repo_cascades_merges() -> Result<(), sqlx::Error> {
        let pool = setup_pool().await?;
        let (repo_id, _, _) = create_merge_fixture(&pool).await?;

        let before: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM merges WHERE repo_id = ?")
            .bind(repo_id)
            .fetch_one(&pool)
            .await?;
        assert_eq!(before, 1, "expected one merge row before deleting repo");

        sqlx::query("DELETE FROM repos WHERE id = ?")
            .bind(repo_id)
            .execute(&pool)
            .await?;

        let after: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM merges WHERE repo_id = ?")
            .bind(repo_id)
            .fetch_one(&pool)
            .await?;
        assert_eq!(after, 0, "merge rows should cascade away with the repo");

        Ok(())
    }

    #[tokio::test]
    async fn deleting_terminal_nulls_git_event_terminal_id() -> Result<(), sqlx::Error> {
        let pool = setup_pool().await?;
        let (terminal_id, git_event_id) = create_git_event_fixture(&pool).await?;

        sqlx::query("DELETE FROM terminal WHERE id = ?")
            .bind(&terminal_id)
            .execute(&pool)
            .await?;

        let terminal_ref: Option<String> =
            sqlx::query_scalar("SELECT terminal_id FROM git_event WHERE id = ?")
                .bind(&git_event_id)
                .fetch_one(&pool)
                .await?;
        assert!(
            terminal_ref.is_none(),
            "git_event.terminal_id should be cleared when the terminal is deleted"
        );

        Ok(())
    }
}
