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

async fn run_migrations(pool: &Pool<Sqlite>) -> Result<(), Error> {
    use std::collections::HashSet;

    let migrator = sqlx::migrate!("./migrations");
    let mut processed_versions: HashSet<i64> = HashSet::new();

    loop {
        match migrator.run(pool).await {
            Ok(()) => return Ok(()),
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
