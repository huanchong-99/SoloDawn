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
    ExpectedColumn {
        table: "workflow",
        column: "orchestrator_state",
        col_type: "TEXT",
        default: None,
    },
    ExpectedColumn {
        table: "workflow",
        column: "orchestrator_api_key_encrypted",
        col_type: "TEXT",
        default: None,
    },
    ExpectedColumn {
        table: "workflow",
        column: "git_watcher_enabled",
        col_type: "INTEGER",
        default: Some("1"),
    },
    ExpectedColumn {
        table: "workflow",
        column: "execution_mode",
        col_type: "TEXT",
        default: Some("'diy'"),
    },
    ExpectedColumn {
        table: "workflow",
        column: "initial_goal",
        col_type: "TEXT",
        default: None,
    },
    ExpectedColumn {
        table: "workflow",
        column: "pause_reason",
        col_type: "TEXT",
        default: None,
    },
    ExpectedColumn {
        table: "model_config",
        column: "encrypted_api_key",
        col_type: "TEXT",
        default: None,
    },
    ExpectedColumn {
        table: "model_config",
        column: "base_url",
        col_type: "TEXT",
        default: None,
    },
    ExpectedColumn {
        table: "model_config",
        column: "api_type",
        col_type: "TEXT",
        default: None,
    },
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
            let default_clause = exp
                .default
                .map_or_else(String::new, |d| format!(" DEFAULT {d}"));
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
        tracing::info!(
            "Startup self-check: schema verification passed ({} columns verified)",
            SCHEMA_EXPECTATIONS.len()
        );
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
                    tracing::info!(
                        applied = new_migrations,
                        "New migrations applied, running schema self-check"
                    );
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
                    // Update the checksum in _sqlx_migrations to match the current file
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
    use sqlx::Row;

    use super::*;

    #[tokio::test]
    async fn workflow_project_created_index_exists() -> Result<(), sqlx::Error> {
        let pool = SqlitePool::connect("sqlite::memory:").await?;
        run_migrations(&pool).await?;

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
}
