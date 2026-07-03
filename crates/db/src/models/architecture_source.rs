//! Architecture knowledge sources: GitHub repos the sync service pulls
//! architecture guidance from. The builtin source (awesome-architecture)
//! is ensured at startup; users may add further repos.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ArchitectureSource {
    pub id: String,
    pub name: String,
    pub owner: String,
    pub repo: String,
    pub branch: String,
    /// JSON array of path prefixes to sync (e.g. `["templates/"]`).
    pub include_paths: String,
    pub enabled: bool,
    pub builtin: bool,
    pub last_tree_sha: Option<String>,
    pub last_synced_at: Option<DateTime<Utc>>,
    pub last_sync_status: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl ArchitectureSource {
    pub fn new(name: &str, owner: &str, repo: &str, branch: &str, include_paths: &[&str]) -> Self {
        let now = Utc::now();
        Self {
            id: Uuid::new_v4().to_string(),
            name: name.to_string(),
            owner: owner.to_string(),
            repo: repo.to_string(),
            branch: branch.to_string(),
            include_paths: serde_json::to_string(include_paths)
                .unwrap_or_else(|_| "[]".to_string()),
            enabled: true,
            builtin: false,
            last_tree_sha: None,
            last_synced_at: None,
            last_sync_status: None,
            created_at: now,
            updated_at: now,
        }
    }

    /// Parse the JSON `include_paths` column; invalid JSON yields an empty list.
    pub fn include_path_list(&self) -> Vec<String> {
        serde_json::from_str(&self.include_paths).unwrap_or_default()
    }

    pub async fn insert(pool: &SqlitePool, source: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO architecture_source (
                id, name, owner, repo, branch, include_paths,
                enabled, builtin, last_tree_sha, last_synced_at, last_sync_status,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)
            ",
        )
        .bind(&source.id)
        .bind(&source.name)
        .bind(&source.owner)
        .bind(&source.repo)
        .bind(&source.branch)
        .bind(&source.include_paths)
        .bind(source.enabled)
        .bind(source.builtin)
        .bind(&source.last_tree_sha)
        .bind(source.last_synced_at)
        .bind(&source.last_sync_status)
        .bind(source.created_at)
        .bind(source.updated_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM architecture_source ORDER BY created_at ASC")
            .fetch_all(pool)
            .await
    }

    pub async fn find_enabled(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM architecture_source WHERE enabled = 1 ORDER BY created_at ASC",
        )
        .fetch_all(pool)
        .await
    }

    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM architecture_source WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    pub async fn find_by_coords(
        pool: &SqlitePool,
        owner: &str,
        repo: &str,
        branch: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM architecture_source WHERE owner = ?1 AND repo = ?2 AND branch = ?3",
        )
        .bind(owner)
        .bind(repo)
        .bind(branch)
        .fetch_optional(pool)
        .await
    }

    /// Update the user-editable fields.
    pub async fn update_settings(
        pool: &SqlitePool,
        id: &str,
        name: &str,
        branch: &str,
        include_paths: &str,
        enabled: bool,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE architecture_source
            SET name = ?2, branch = ?3, include_paths = ?4, enabled = ?5,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(name)
        .bind(branch)
        .bind(include_paths)
        .bind(enabled)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Record the outcome of a sync attempt.
    pub async fn record_sync(
        pool: &SqlitePool,
        id: &str,
        tree_sha: Option<&str>,
        status: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE architecture_source
            SET last_tree_sha = COALESCE(?2, last_tree_sha),
                last_synced_at = datetime('now'),
                last_sync_status = ?3,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(tree_sha)
        .bind(status)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Delete a non-builtin source. Returns the number of rows removed.
    pub async fn delete_custom(pool: &SqlitePool, id: &str) -> sqlx::Result<u64> {
        let result = sqlx::query("DELETE FROM architecture_source WHERE id = ?1 AND builtin = 0")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }
}
