//! Per-project System-A quality-gate policy override.
//!
//! Priority-0 source of truth for the quality-gate configuration of a project.
//! `config_yaml` is opaque to this crate: it is `serde_yaml` of
//! `quality::config::QualityGateConfig`, round-tripped via `from_yaml`, keeping
//! the `db` crate free of any `quality` types.

use chrono::{DateTime, Utc};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

/// Per-project quality-gate policy.
///
/// Corresponds to database table: project_quality_policy
#[derive(Debug, Clone, FromRow)]
pub struct ProjectQualityPolicy {
    pub project_id: Uuid,
    pub config_yaml: String,
    pub mode: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl ProjectQualityPolicy {
    /// Find the policy override for a project, if one exists.
    pub async fn find_by_project(
        pool: &SqlitePool,
        project_id: Uuid,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM project_quality_policy WHERE project_id = ?1")
            .bind(project_id)
            .fetch_optional(pool)
            .await
    }

    /// Insert or update the policy override for a project.
    pub async fn upsert(
        pool: &SqlitePool,
        project_id: Uuid,
        config_yaml: &str,
        mode: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO project_quality_policy (project_id, config_yaml, mode, created_at, updated_at)
              VALUES (?1, ?2, ?3, datetime('now'), datetime('now'))
              ON CONFLICT(project_id) DO UPDATE SET
                config_yaml = excluded.config_yaml,
                mode        = excluded.mode,
                updated_at  = datetime('now')",
        )
        .bind(project_id)
        .bind(config_yaml)
        .bind(mode)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Delete the policy override for a project (resets to default resolution).
    pub async fn delete(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<()> {
        sqlx::query("DELETE FROM project_quality_policy WHERE project_id = ?1")
            .bind(project_id)
            .execute(pool)
            .await?;
        Ok(())
    }
}
