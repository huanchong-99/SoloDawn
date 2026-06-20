//! Custom Rule Model
//!
//! Editable, AI-authored, human-confirmed declarative quality-gate rule. A rule
//! is pure data (P1: scoped-regex JSON; P2: ast-grep YAML) -- never executable
//! code -- enforced deterministically and LLM-free at gate time. See PRD
//! `docs/quality/PRD-ai-editable-quality-rules.md` §9.
//!
//! NOTE: like `git_event.rs` / `project_quality_policy.rs`, these methods use the
//! dynamic `sqlx::query` / `query_as::<_, Self>` API rather than the compile-time
//! `query!` / `query_as!` macros. The macro variants require `DATABASE_URL` or a
//! committed `.sqlx/` offline cache at build time; the schema here is created from
//! embedded migrations at runtime, so `FromRow` + explicit `bind()` is the
//! project convention for net-new tables.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Custom Rule
///
/// Corresponds to database table: custom_rule
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CustomRule {
    pub id: Uuid,
    /// NULL = global/org rule (schema-allowed; v1 UI requires non-null, D4).
    pub project_id: Option<Uuid>,
    pub name: String,
    /// Original NL ask (round-trip compare + reproducibility).
    pub nl_request: String,
    /// `regex` (P1) | `ast_grep` (P2).
    pub rule_format: String,
    /// Scoped-regex JSON (P1) or ast-grep YAML (P2).
    pub rule_body: String,
    /// LLM-generated text powering the "!" tooltip.
    pub description: Option<String>,
    /// Bug | Vulnerability | CodeSmell | SecurityHotspot.
    pub rule_type: String,
    /// INFO | MINOR | MAJOR | CRITICAL | BLOCKER.
    pub severity: String,
    /// `MetricKey::as_str()` token; free text, NOT an FK.
    pub mapped_metric: Option<String>,
    pub enabled: bool,
    /// draft | shadow | warn | enforce | disabled.
    pub status: String,
    pub created_by: Option<String>,
    pub version: i64,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Fields required to create a `CustomRule`.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateCustomRule {
    pub project_id: Option<Uuid>,
    pub name: String,
    pub nl_request: String,
    pub rule_format: String,
    pub rule_body: String,
    pub description: Option<String>,
    pub rule_type: String,
    pub severity: String,
    pub mapped_metric: Option<String>,
    pub created_by: Option<String>,
}

/// Mutable fields of a `CustomRule` (body + metadata). Status and enabled flag
/// are changed through their own dedicated setters, never here.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct UpdateCustomRule {
    pub name: String,
    pub nl_request: String,
    pub rule_format: String,
    pub rule_body: String,
    pub description: Option<String>,
    pub rule_type: String,
    pub severity: String,
    pub mapped_metric: Option<String>,
}

impl CustomRule {
    /// Insert a new custom rule. `enabled` defaults to true and `status` to
    /// `shadow` (the DB defaults); `version` starts at 1.
    pub async fn create(pool: &SqlitePool, data: &CreateCustomRule) -> sqlx::Result<Self> {
        let id = Uuid::new_v4();
        sqlx::query_as::<_, CustomRule>(
            r"INSERT INTO custom_rule (
                id, project_id, name, nl_request, rule_format, rule_body,
                description, rule_type, severity, mapped_metric, created_by
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)
            RETURNING *",
        )
        .bind(id)
        .bind(data.project_id)
        .bind(&data.name)
        .bind(&data.nl_request)
        .bind(&data.rule_format)
        .bind(&data.rule_body)
        .bind(&data.description)
        .bind(&data.rule_type)
        .bind(&data.severity)
        .bind(&data.mapped_metric)
        .bind(&data.created_by)
        .fetch_one(pool)
        .await
    }

    /// Find a custom rule by its id.
    pub async fn find_by_id(pool: &SqlitePool, id: Uuid) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, CustomRule>("SELECT * FROM custom_rule WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Find all custom rules for a project, newest first.
    pub async fn find_by_project(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, CustomRule>(
            "SELECT * FROM custom_rule WHERE project_id = ?1 ORDER BY created_at DESC",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await
    }

    /// Find the project's enabled, non-draft, non-disabled rules -- the set the
    /// deterministic enforcement engine compiles each gate run.
    pub async fn find_enabled_by_project(
        pool: &SqlitePool,
        project_id: Uuid,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, CustomRule>(
            r"SELECT * FROM custom_rule
              WHERE project_id = ?1 AND enabled = 1 AND status NOT IN ('draft','disabled')
              ORDER BY created_at DESC",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await
    }

    /// Update the body + metadata of a rule and bump its `version`. Status and
    /// the enabled flag are left untouched (D8: the route layer decides whether a
    /// body edit triggers revalidation + drop-to-shadow).
    pub async fn update(pool: &SqlitePool, id: Uuid, data: &UpdateCustomRule) -> sqlx::Result<Self> {
        sqlx::query_as::<_, CustomRule>(
            r"UPDATE custom_rule SET
                name          = ?2,
                nl_request    = ?3,
                rule_format   = ?4,
                rule_body     = ?5,
                description   = ?6,
                rule_type     = ?7,
                severity      = ?8,
                mapped_metric = ?9,
                version       = version + 1,
                updated_at    = datetime('now','subsec')
              WHERE id = ?1
              RETURNING *",
        )
        .bind(id)
        .bind(&data.name)
        .bind(&data.nl_request)
        .bind(&data.rule_format)
        .bind(&data.rule_body)
        .bind(&data.description)
        .bind(&data.rule_type)
        .bind(&data.severity)
        .bind(&data.mapped_metric)
        .fetch_one(pool)
        .await
    }

    /// Set a rule's lifecycle status (e.g. shadow -> warn -> enforce). Caller is
    /// responsible for validating the transition against the CHECK enum.
    pub async fn set_status(pool: &SqlitePool, id: Uuid, status: &str) -> sqlx::Result<Self> {
        sqlx::query_as::<_, CustomRule>(
            r"UPDATE custom_rule
              SET status = ?2, updated_at = datetime('now','subsec')
              WHERE id = ?1
              RETURNING *",
        )
        .bind(id)
        .bind(status)
        .fetch_one(pool)
        .await
    }

    /// Toggle the enabled flag without changing lifecycle status.
    pub async fn set_enabled(pool: &SqlitePool, id: Uuid, enabled: bool) -> sqlx::Result<Self> {
        sqlx::query_as::<_, CustomRule>(
            r"UPDATE custom_rule
              SET enabled = ?2, updated_at = datetime('now','subsec')
              WHERE id = ?1
              RETURNING *",
        )
        .bind(id)
        .bind(enabled)
        .fetch_one(pool)
        .await
    }

    /// Delete a rule. Children in `custom_rule_example` / `custom_rule_validation`
    /// cascade; `custom_rule_audit` rows are intentionally FK-less and survive.
    pub async fn delete(pool: &SqlitePool, id: Uuid) -> sqlx::Result<u64> {
        let result = sqlx::query("DELETE FROM custom_rule WHERE id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }
}
