//! CLI Install History Model
//!
//! Tracks individual CLI install/uninstall operations and their results,
//! and caches CLI detection status for quick lookups.

use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// CLI Install History
///
/// Corresponds to database table: cli_install_history
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CliInstallHistory {
    /// Primary key ID (UUID)
    pub id: String,

    /// Associated CLI type ID
    pub cli_type_id: String,

    /// Action: "install" | "uninstall"
    pub action: String,

    /// Status: "pending" | "running" | "success" | "failed" | "cancelled"
    pub status: String,

    /// When the operation started
    pub started_at: String,

    /// When the operation completed (if finished)
    pub completed_at: Option<String>,

    /// Process exit code (if completed)
    pub exit_code: Option<i32>,

    /// Command output
    pub output: Option<String>,

    /// Error message (if failed)
    pub error_message: Option<String>,

    /// Created timestamp
    pub created_at: String,
}

/// CLI Detection Cache
///
/// Corresponds to database table: cli_detection_cache
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CliDetectionCache {
    /// CLI type ID (primary key)
    pub cli_type_id: String,

    /// Whether the CLI is installed
    pub installed: bool,

    /// Detected version string
    pub version: Option<String>,

    /// Path to the executable
    pub executable_path: Option<String>,

    /// When the detection was performed
    pub detected_at: String,
}

impl CliInstallHistory {
    /// Create a new install history record
    pub async fn create(
        pool: &SqlitePool,
        cli_type_id: &str,
        action: &str,
    ) -> anyhow::Result<Self> {
        let id = Uuid::new_v4().to_string();

        let record = sqlx::query_as::<_, CliInstallHistory>(
            r"
            INSERT INTO cli_install_history (id, cli_type_id, action)
            VALUES (?1, ?2, ?3)
            RETURNING *
            ",
        )
        .bind(&id)
        .bind(cli_type_id)
        .bind(action)
        .fetch_one(pool)
        .await?;

        Ok(record)
    }

    /// Update the status of an install history record
    pub async fn update_status(
        pool: &SqlitePool,
        id: &str,
        status: &str,
        exit_code: Option<i32>,
        output: Option<&str>,
        error_message: Option<&str>,
    ) -> anyhow::Result<()> {
        sqlx::query(
            r"
            UPDATE cli_install_history
            SET status = ?1,
                exit_code = ?2,
                output = ?3,
                error_message = ?4,
                completed_at = CASE WHEN ?1 IN ('success', 'failed', 'cancelled') THEN datetime('now') ELSE completed_at END
            WHERE id = ?5
            ",
        )
        .bind(status)
        .bind(exit_code)
        .bind(output)
        .bind(error_message)
        .bind(id)
        .execute(pool)
        .await?;

        Ok(())
    }

    /// Get an install history record by ID
    pub async fn get_by_id(pool: &SqlitePool, id: &str) -> anyhow::Result<Option<Self>> {
        let record = sqlx::query_as::<_, CliInstallHistory>(
            r"
            SELECT id, cli_type_id, action, status, started_at,
                   completed_at, exit_code, output, error_message, created_at
            FROM cli_install_history
            WHERE id = ?
            ",
        )
        .bind(id)
        .fetch_optional(pool)
        .await?;

        Ok(record)
    }

    /// List install history records for a CLI type, ordered by most recent first
    pub async fn list_by_cli_type(
        pool: &SqlitePool,
        cli_type_id: &str,
        limit: i64,
    ) -> anyhow::Result<Vec<Self>> {
        let records = sqlx::query_as::<_, CliInstallHistory>(
            r"
            SELECT id, cli_type_id, action, status, started_at,
                   completed_at, exit_code, output, error_message, created_at
            FROM cli_install_history
            WHERE cli_type_id = ?1
            ORDER BY created_at DESC
            LIMIT ?2
            ",
        )
        .bind(cli_type_id)
        .bind(limit)
        .fetch_all(pool)
        .await?;

        Ok(records)
    }

    /// Get the latest install history record for a CLI type
    pub async fn get_latest_by_cli_type(
        pool: &SqlitePool,
        cli_type_id: &str,
    ) -> anyhow::Result<Option<Self>> {
        let record = sqlx::query_as::<_, CliInstallHistory>(
            r"
            SELECT id, cli_type_id, action, status, started_at,
                   completed_at, exit_code, output, error_message, created_at
            FROM cli_install_history
            WHERE cli_type_id = ?
            ORDER BY created_at DESC
            LIMIT 1
            ",
        )
        .bind(cli_type_id)
        .fetch_optional(pool)
        .await?;

        Ok(record)
    }
}

impl CliDetectionCache {
    /// Upsert a detection cache entry
    pub async fn upsert(
        pool: &SqlitePool,
        cli_type_id: &str,
        installed: bool,
        version: Option<&str>,
        executable_path: Option<&str>,
    ) -> anyhow::Result<()> {
        sqlx::query(
            r"
            INSERT INTO cli_detection_cache (cli_type_id, installed, version, executable_path, detected_at)
            VALUES (?1, ?2, ?3, ?4, datetime('now'))
            ON CONFLICT(cli_type_id) DO UPDATE SET
                installed = excluded.installed,
                version = excluded.version,
                executable_path = excluded.executable_path,
                detected_at = excluded.detected_at
            ",
        )
        .bind(cli_type_id)
        .bind(installed)
        .bind(version)
        .bind(executable_path)
        .execute(pool)
        .await?;

        Ok(())
    }

    /// Get all detection cache entries
    pub async fn get_all(pool: &SqlitePool) -> anyhow::Result<Vec<Self>> {
        let records = sqlx::query_as::<_, CliDetectionCache>(
            r"
            SELECT cli_type_id, installed, version, executable_path, detected_at
            FROM cli_detection_cache
            ",
        )
        .fetch_all(pool)
        .await?;

        Ok(records)
    }

    /// Get a detection cache entry by CLI type ID
    pub async fn get_by_cli_type(
        pool: &SqlitePool,
        cli_type_id: &str,
    ) -> anyhow::Result<Option<Self>> {
        let record = sqlx::query_as::<_, CliDetectionCache>(
            r"
            SELECT cli_type_id, installed, version, executable_path, detected_at
            FROM cli_detection_cache
            WHERE cli_type_id = ?
            ",
        )
        .bind(cli_type_id)
        .fetch_optional(pool)
        .await?;

        Ok(record)
    }

    /// Delete a detection cache entry
    pub async fn delete(pool: &SqlitePool, cli_type_id: &str) -> anyhow::Result<()> {
        sqlx::query(
            r"
            DELETE FROM cli_detection_cache
            WHERE cli_type_id = ?
            ",
        )
        .bind(cli_type_id)
        .execute(pool)
        .await?;

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_cli_install_history_create() {
        // TODO: Set up in-memory SQLite pool, run migrations, and test create
    }

    #[tokio::test]
    async fn test_cli_install_history_update_status() {
        // TODO: Create a record, update its status, verify changes
    }

    #[tokio::test]
    async fn test_cli_install_history_list_by_cli_type() {
        // TODO: Create multiple records, verify list ordering and limit
    }

    #[tokio::test]
    async fn test_cli_install_history_get_latest() {
        // TODO: Create multiple records, verify latest is returned
    }

    #[tokio::test]
    async fn test_cli_detection_cache_upsert() {
        // TODO: Insert a cache entry, upsert to update, verify changes
    }

    #[tokio::test]
    async fn test_cli_detection_cache_get_all() {
        // TODO: Insert multiple entries, verify get_all returns all
    }

    #[tokio::test]
    async fn test_cli_detection_cache_delete() {
        // TODO: Insert an entry, delete it, verify it's gone
    }
}
