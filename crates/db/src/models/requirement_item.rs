//! Requirement ledger (评分点账本) — one row per acceptance point, project-scoped.
//!
//! Each point is an independently verifiable acceptance criterion extracted
//! from the confirmed spec's functional-completeness rubric. When the
//! acceptance review turns a point green it writes back a compressed context
//! capsule (map, not encyclopedia: what was built / where it lives /
//! decisions & gotchas / extension notes) plus provenance, so follow-up
//! rounds start from the ledger instead of re-understanding the project.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Point lifecycle states.
pub const REQUIREMENT_STATUS_PENDING: &str = "pending";
pub const REQUIREMENT_STATUS_DELIVERED: &str = "delivered";
pub const REQUIREMENT_STATUS_REGRESSED: &str = "regressed";

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RequirementItem {
    pub id: String,
    pub project_id: Uuid,
    /// Stable point id rendered into rubric text (e.g. "RP-001"); unique per project.
    pub point_code: String,
    pub text: String,
    /// pending | delivered | regressed
    pub status: String,
    /// Planning draft (round) that introduced this point.
    pub origin_draft_id: Option<String>,
    /// Compressed context capsule written at scoring time (JSON string).
    pub context_capsule: Option<String>,
    pub provenance_workflow_id: Option<String>,
    pub provenance_commits: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub delivered_at: Option<DateTime<Utc>>,
}

impl RequirementItem {
    pub fn new(project_id: Uuid, point_code: &str, text: &str, origin_draft_id: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            project_id,
            point_code: point_code.to_string(),
            text: text.to_string(),
            status: REQUIREMENT_STATUS_PENDING.to_string(),
            origin_draft_id: Some(origin_draft_id.to_string()),
            context_capsule: None,
            provenance_workflow_id: None,
            provenance_commits: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
            delivered_at: None,
        }
    }

    pub async fn insert(pool: &SqlitePool, item: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO requirement_item (
                id, project_id, point_code, text, status,
                origin_draft_id, context_capsule,
                provenance_workflow_id, provenance_commits,
                created_at, updated_at, delivered_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
            ",
        )
        .bind(&item.id)
        .bind(item.project_id)
        .bind(&item.point_code)
        .bind(&item.text)
        .bind(&item.status)
        .bind(&item.origin_draft_id)
        .bind(&item.context_capsule)
        .bind(&item.provenance_workflow_id)
        .bind(&item.provenance_commits)
        .bind(item.created_at)
        .bind(item.updated_at)
        .bind(item.delivered_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM requirement_item WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// All points for a project, oldest first (stable rubric ordering).
    pub async fn find_by_project(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM requirement_item WHERE project_id = ?1 ORDER BY point_code ASC",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await
    }

    pub async fn find_by_project_and_status(
        pool: &SqlitePool,
        project_id: Uuid,
        status: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM requirement_item WHERE project_id = ?1 AND status = ?2 ORDER BY point_code ASC",
        )
        .bind(project_id)
        .bind(status)
        .fetch_all(pool)
        .await
    }

    /// Find a point by its rubric code (e.g. "RP-003") within a project.
    pub async fn find_by_point_code(
        pool: &SqlitePool,
        project_id: Uuid,
        point_code: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM requirement_item WHERE project_id = ?1 AND point_code = ?2",
        )
        .bind(project_id)
        .bind(point_code)
        .fetch_optional(pool)
        .await
    }

    /// Next free point code for a project ("RP-001", "RP-002", ...).
    ///
    /// Derived from the max existing numeric suffix rather than the row count
    /// so deleted points never cause code reuse.
    pub async fn next_point_code(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<String> {
        let codes: Vec<String> = sqlx::query_scalar(
            "SELECT point_code FROM requirement_item WHERE project_id = ?1",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await?;
        let max = codes
            .iter()
            .filter_map(|c| c.strip_prefix("RP-").and_then(|n| n.parse::<u32>().ok()))
            .max()
            .unwrap_or(0);
        Ok(format!("RP-{:03}", max + 1))
    }

    /// Mark a point delivered, storing the compressed capsule + provenance.
    pub async fn mark_delivered(
        pool: &SqlitePool,
        id: &str,
        context_capsule: Option<&str>,
        provenance_workflow_id: &str,
        provenance_commits: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE requirement_item SET
                status = 'delivered',
                context_capsule = COALESCE(?2, context_capsule),
                provenance_workflow_id = ?3,
                provenance_commits = ?4,
                delivered_at = datetime('now'),
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(context_capsule)
        .bind(provenance_workflow_id)
        .bind(provenance_commits)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Mark a previously delivered point as regressed (delta-round review
    /// found it broken). Keeps the capsule/provenance for repair context.
    pub async fn mark_regressed(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE requirement_item SET status = 'regressed', updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Edit the text of a pending point (pre-confirm curation). Delivered
    /// points are immutable through this path.
    pub async fn update_text_if_pending(
        pool: &SqlitePool,
        id: &str,
        text: &str,
    ) -> sqlx::Result<bool> {
        let result = sqlx::query(
            "UPDATE requirement_item SET text = ?2, updated_at = datetime('now') WHERE id = ?1 AND status = 'pending'",
        )
        .bind(id)
        .bind(text)
        .execute(pool)
        .await?;
        Ok(result.rows_affected() > 0)
    }

    /// Delete a pending point (pre-confirm curation). Delivered points are kept.
    pub async fn delete_if_pending(pool: &SqlitePool, id: &str) -> sqlx::Result<bool> {
        let result =
            sqlx::query("DELETE FROM requirement_item WHERE id = ?1 AND status = 'pending'")
                .bind(id)
                .execute(pool)
                .await?;
        Ok(result.rows_affected() > 0)
    }
}

/// Compressed context capsule content (stored as JSON in `context_capsule`).
///
/// Map, not encyclopedia: pointers + what the code cannot show. Never a copy
/// of code content (recoverable, goes stale); fields may be empty ("short and
/// accurate beats long and padded").
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, TS)]
#[serde(rename_all = "camelCase", default)]
pub struct ContextCapsule {
    /// What was built for this point, one or two sentences.
    pub built: String,
    /// Where it lives: files / modules touched (from the real diff).
    pub lives_where: String,
    /// Decisions made and traps discovered — the unrecoverable knowledge.
    pub decisions: String,
    /// Where to start when extending this point.
    pub extension_notes: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    async fn test_pool() -> SqlitePool {
        let pool = SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE requirement_item (
                id TEXT PRIMARY KEY,
                project_id BLOB NOT NULL,
                point_code TEXT NOT NULL,
                text TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'pending',
                origin_draft_id TEXT,
                context_capsule TEXT,
                provenance_workflow_id TEXT,
                provenance_commits TEXT,
                created_at DATETIME NOT NULL DEFAULT (datetime('now')),
                updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
                delivered_at DATETIME,
                UNIQUE(project_id, point_code)
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();
        pool
    }

    #[test]
    fn new_item_defaults_to_pending() {
        let item = RequirementItem::new(Uuid::new_v4(), "RP-001", "supports reminders", "draft-1");
        assert_eq!(item.status, REQUIREMENT_STATUS_PENDING);
        assert_eq!(item.point_code, "RP-001");
        assert!(item.context_capsule.is_none());
        assert!(item.delivered_at.is_none());
    }

    #[tokio::test]
    async fn insert_and_find_roundtrip() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        let item = RequirementItem::new(project, "RP-001", "memo CRUD", "draft-1");
        RequirementItem::insert(&pool, &item).await.unwrap();

        let found = RequirementItem::find_by_project(&pool, project).await.unwrap();
        assert_eq!(found.len(), 1);
        assert_eq!(found[0].text, "memo CRUD");

        let by_code = RequirementItem::find_by_point_code(&pool, project, "RP-001")
            .await
            .unwrap();
        assert!(by_code.is_some());
    }

    #[tokio::test]
    async fn next_point_code_skips_gaps_and_never_reuses() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        assert_eq!(
            RequirementItem::next_point_code(&pool, project).await.unwrap(),
            "RP-001"
        );
        for code in ["RP-001", "RP-002", "RP-007"] {
            let item = RequirementItem::new(project, code, "x", "d");
            RequirementItem::insert(&pool, &item).await.unwrap();
        }
        assert_eq!(
            RequirementItem::next_point_code(&pool, project).await.unwrap(),
            "RP-008"
        );
    }

    #[tokio::test]
    async fn mark_delivered_sets_capsule_and_provenance() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        let item = RequirementItem::new(project, "RP-001", "memo CRUD", "draft-1");
        RequirementItem::insert(&pool, &item).await.unwrap();

        let capsule = serde_json::to_string(&ContextCapsule {
            built: "CRUD endpoints".into(),
            lives_where: "src/memo/".into(),
            decisions: "sqlite over file store".into(),
            extension_notes: "add routes in src/memo/routes.rs".into(),
        })
        .unwrap();
        RequirementItem::mark_delivered(&pool, &item.id, Some(&capsule), "wf-1", Some("3 files"))
            .await
            .unwrap();

        let found = RequirementItem::find_by_id(&pool, &item.id).await.unwrap().unwrap();
        assert_eq!(found.status, REQUIREMENT_STATUS_DELIVERED);
        assert!(found.delivered_at.is_some());
        assert_eq!(found.provenance_workflow_id.as_deref(), Some("wf-1"));
        let parsed: ContextCapsule = serde_json::from_str(found.context_capsule.as_deref().unwrap()).unwrap();
        assert_eq!(parsed.lives_where, "src/memo/");
    }

    #[tokio::test]
    async fn mark_regressed_keeps_capsule() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        let item = RequirementItem::new(project, "RP-001", "memo CRUD", "draft-1");
        RequirementItem::insert(&pool, &item).await.unwrap();
        RequirementItem::mark_delivered(&pool, &item.id, Some("{}"), "wf-1", None)
            .await
            .unwrap();
        RequirementItem::mark_regressed(&pool, &item.id).await.unwrap();

        let found = RequirementItem::find_by_id(&pool, &item.id).await.unwrap().unwrap();
        assert_eq!(found.status, REQUIREMENT_STATUS_REGRESSED);
        assert!(found.context_capsule.is_some());
    }

    #[tokio::test]
    async fn pending_only_mutations_respect_status() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        let item = RequirementItem::new(project, "RP-001", "memo CRUD", "draft-1");
        RequirementItem::insert(&pool, &item).await.unwrap();

        assert!(RequirementItem::update_text_if_pending(&pool, &item.id, "memo CRUD v2")
            .await
            .unwrap());

        RequirementItem::mark_delivered(&pool, &item.id, None, "wf-1", None)
            .await
            .unwrap();

        assert!(!RequirementItem::update_text_if_pending(&pool, &item.id, "nope")
            .await
            .unwrap());
        assert!(!RequirementItem::delete_if_pending(&pool, &item.id).await.unwrap());
    }

    #[test]
    fn capsule_tolerates_missing_fields() {
        let parsed: ContextCapsule = serde_json::from_str(r#"{"built":"x"}"#).unwrap();
        assert_eq!(parsed.built, "x");
        assert_eq!(parsed.lives_where, "");
    }
}
