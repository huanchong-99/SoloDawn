//! Custom Rule Validation Model
//!
//! Authoring-time validation artifact for a custom rule ONLY -- do NOT conflate
//! with `quality_run` / `quality_issue` (those are enforcement-time audit). One
//! row per validation attempt: the empirical positive/negative results, the
//! round-trip judge verdict, and the adversary transcript. See PRD
//! `docs/quality/PRD-ai-editable-quality-rules.md` §9.
//!
//! NOTE: dynamic `sqlx::query` / `query_as::<_, Self>` (not the compile-time
//! macros), matching the project convention for net-new tables (see `git_event.rs`).

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Custom Rule Validation
///
/// Corresponds to database table: custom_rule_validation
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CustomRuleValidation {
    pub id: Uuid,
    pub rule_id: Uuid,
    pub rule_version: i64,
    /// pass | fail | error | pending.
    pub verdict: String,
    /// Judge verdict on reconstructed-NL vs original (NULL until run).
    pub roundtrip_ok: Option<bool>,
    /// `AuditScoreResult`-style total.
    pub judge_score: Option<f64>,
    pub examples_total: i64,
    pub examples_passed: i64,
    pub rounds_used: i64,
    /// Per-example {example_id, expected, actual, matched_spans}; + adversary transcript.
    pub results_json: Option<String>,
    pub error_message: Option<String>,
    pub validated_by: Option<String>,
    pub created_at: DateTime<Utc>,
}

/// Fields required to create a `CustomRuleValidation`.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateCustomRuleValidation {
    pub rule_id: Uuid,
    pub rule_version: i64,
    pub verdict: String,
    pub roundtrip_ok: Option<bool>,
    pub judge_score: Option<f64>,
    pub examples_total: i64,
    pub examples_passed: i64,
    pub rounds_used: i64,
    pub results_json: Option<String>,
    pub error_message: Option<String>,
    pub validated_by: Option<String>,
}

impl CustomRuleValidation {
    /// Insert a validation artifact row.
    pub async fn insert(
        pool: &SqlitePool,
        data: &CreateCustomRuleValidation,
    ) -> sqlx::Result<Self> {
        let id = Uuid::new_v4();
        sqlx::query_as::<_, CustomRuleValidation>(
            r"INSERT INTO custom_rule_validation (
                id, rule_id, rule_version, verdict, roundtrip_ok, judge_score,
                examples_total, examples_passed, rounds_used, results_json,
                error_message, validated_by
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
            RETURNING *",
        )
        .bind(id)
        .bind(data.rule_id)
        .bind(data.rule_version)
        .bind(&data.verdict)
        .bind(data.roundtrip_ok)
        .bind(data.judge_score)
        .bind(data.examples_total)
        .bind(data.examples_passed)
        .bind(data.rounds_used)
        .bind(&data.results_json)
        .bind(&data.error_message)
        .bind(&data.validated_by)
        .fetch_one(pool)
        .await
    }

    /// Find all validation artifacts for a rule, newest first.
    pub async fn find_by_rule(pool: &SqlitePool, rule_id: Uuid) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, CustomRuleValidation>(
            "SELECT * FROM custom_rule_validation WHERE rule_id = ?1 ORDER BY created_at DESC",
        )
        .bind(rule_id)
        .fetch_all(pool)
        .await
    }
}
