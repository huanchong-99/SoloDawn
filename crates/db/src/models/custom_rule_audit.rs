//! Custom Rule Audit Model
//!
//! Append-only audit log of every lifecycle action on a custom rule. Rows are
//! NEVER updated or deleted, and the table is intentionally FK-LESS (`rule_id`
//! and `project_id` are bare BLOBs) so history survives rule deletion. See PRD
//! `docs/quality/PRD-ai-editable-quality-rules.md` §9.
//!
//! NOTE: dynamic `sqlx::query` / `query_as::<_, Self>` (not the compile-time
//! macros), matching the project convention for net-new tables (see `git_event.rs`).

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Custom Rule Audit
///
/// Corresponds to database table: custom_rule_audit
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CustomRuleAudit {
    pub id: Uuid,
    pub rule_id: Uuid,
    pub project_id: Option<Uuid>,
    /// create | update | enable | disable | delete | revalidate | promote.
    pub action: String,
    pub actor: Option<String>,
    pub from_version: Option<i64>,
    pub to_version: Option<i64>,
    pub diff_json: Option<String>,
    pub created_at: DateTime<Utc>,
}

/// Fields required to append a `CustomRuleAudit` row.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateCustomRuleAudit {
    pub rule_id: Uuid,
    pub project_id: Option<Uuid>,
    pub action: String,
    pub actor: Option<String>,
    pub from_version: Option<i64>,
    pub to_version: Option<i64>,
    pub diff_json: Option<String>,
}

impl CustomRuleAudit {
    /// Append an audit row. This table is append-only: there is intentionally no
    /// update or delete method.
    pub async fn insert(pool: &SqlitePool, data: &CreateCustomRuleAudit) -> sqlx::Result<Self> {
        let id = Uuid::new_v4();
        sqlx::query_as::<_, CustomRuleAudit>(
            r"INSERT INTO custom_rule_audit (
                id, rule_id, project_id, action, actor, from_version, to_version, diff_json
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            RETURNING *",
        )
        .bind(id)
        .bind(data.rule_id)
        .bind(data.project_id)
        .bind(&data.action)
        .bind(&data.actor)
        .bind(data.from_version)
        .bind(data.to_version)
        .bind(&data.diff_json)
        .fetch_one(pool)
        .await
    }

    /// Find the full audit trail for a rule, newest first.
    pub async fn find_by_rule(pool: &SqlitePool, rule_id: Uuid) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, CustomRuleAudit>(
            "SELECT * FROM custom_rule_audit WHERE rule_id = ?1 ORDER BY created_at DESC",
        )
        .bind(rule_id)
        .fetch_all(pool)
        .await
    }
}
