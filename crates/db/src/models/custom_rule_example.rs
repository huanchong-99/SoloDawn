//! Custom Rule Example Model
//!
//! The correctness oracle for a custom rule: positive snippets that SHOULD flag
//! and negative snippets that MUST NOT flag. The empirical admission-gate runs
//! the compiled rule against these before the rule is ever persisted as usable.
//! See PRD `docs/quality/PRD-ai-editable-quality-rules.md` §9.
//!
//! NOTE: dynamic `sqlx::query` / `query_as::<_, Self>` (not the compile-time
//! macros), matching the project convention for net-new tables (see `git_event.rs`).

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Custom Rule Example
///
/// Corresponds to database table: custom_rule_example
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CustomRuleExample {
    pub id: Uuid,
    pub rule_id: Uuid,
    /// positive (SHOULD flag) | negative (MUST NOT flag).
    pub kind: String,
    /// 'rust', 'typescript', NULL = agnostic.
    pub language: Option<String>,
    pub snippet: String,
    /// 1 = rule expected to fire on this snippet.
    pub expected_match: bool,
    pub note: Option<String>,
    pub created_at: DateTime<Utc>,
}

/// Fields required to create a `CustomRuleExample`.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateCustomRuleExample {
    pub rule_id: Uuid,
    pub kind: String,
    pub language: Option<String>,
    pub snippet: String,
    pub expected_match: bool,
    pub note: Option<String>,
}

impl CustomRuleExample {
    /// Insert a single example.
    pub async fn insert(pool: &SqlitePool, data: &CreateCustomRuleExample) -> sqlx::Result<Self> {
        let id = Uuid::new_v4();
        sqlx::query_as::<_, CustomRuleExample>(
            r"INSERT INTO custom_rule_example (
                id, rule_id, kind, language, snippet, expected_match, note
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
            RETURNING *",
        )
        .bind(id)
        .bind(data.rule_id)
        .bind(&data.kind)
        .bind(&data.language)
        .bind(&data.snippet)
        .bind(data.expected_match)
        .bind(&data.note)
        .fetch_one(pool)
        .await
    }

    /// Batch-insert examples within a single transaction (the `quality_issue.rs`
    /// `insert_batch` pattern).
    pub async fn insert_batch(
        pool: &SqlitePool,
        examples: &[CreateCustomRuleExample],
    ) -> sqlx::Result<()> {
        let mut tx = pool.begin().await?;
        for ex in examples {
            let id = Uuid::new_v4();
            sqlx::query(
                r"INSERT INTO custom_rule_example (
                    id, rule_id, kind, language, snippet, expected_match, note
                ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
            )
            .bind(id)
            .bind(ex.rule_id)
            .bind(&ex.kind)
            .bind(&ex.language)
            .bind(&ex.snippet)
            .bind(ex.expected_match)
            .bind(&ex.note)
            .execute(&mut *tx)
            .await?;
        }
        tx.commit().await?;
        Ok(())
    }

    /// Find all examples for a rule, positives before negatives.
    pub async fn find_by_rule(pool: &SqlitePool, rule_id: Uuid) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, CustomRuleExample>(
            "SELECT * FROM custom_rule_example WHERE rule_id = ?1 ORDER BY kind ASC, created_at ASC",
        )
        .bind(rule_id)
        .fetch_all(pool)
        .await
    }
}
