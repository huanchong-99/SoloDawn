//! Quality Issue Model
//!
//! Persists individual quality issues found during a quality run.
//! Each issue maps to a row discovered by a provider (clippy, eslint, sonar, etc.).

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Quality Issue
///
/// Corresponds to database table: quality_issue
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityIssueRecord {
    pub id: String,
    pub quality_run_id: String,
    pub rule_id: String,
    /// BUG | VULNERABILITY | CODE_SMELL | SECURITY_HOTSPOT
    pub rule_type: String,
    /// INFO | MINOR | MAJOR | CRITICAL | BLOCKER
    pub severity: String,
    /// clippy | cargo-check | eslint | sonarqube | ...
    pub source: String,
    pub message: String,
    pub file_path: Option<String>,
    pub line: Option<i32>,
    pub end_line: Option<i32>,
    pub column_start: Option<i32>,
    pub column_end: Option<i32>,
    pub is_new: bool,
    pub is_blocking: bool,
    pub effort_minutes: Option<i32>,
    pub context: Option<String>,
    pub created_at: DateTime<Utc>,
}

impl QualityIssueRecord {
    /// Create a new quality issue record from domain fields
    pub fn new(
        quality_run_id: &str,
        rule_id: &str,
        rule_type: &str,
        severity: &str,
        source: &str,
        message: &str,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            quality_run_id: quality_run_id.to_string(),
            rule_id: rule_id.to_string(),
            rule_type: rule_type.to_string(),
            severity: severity.to_string(),
            source: source.to_string(),
            message: message.to_string(),
            file_path: None,
            line: None,
            end_line: None,
            column_start: None,
            column_end: None,
            is_new: true,
            is_blocking: false,
            effort_minutes: None,
            context: None,
            created_at: Utc::now(),
        }
    }

    /// Insert a single quality issue
    pub async fn insert(pool: &SqlitePool, issue: &QualityIssueRecord) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO quality_issue (
                id, quality_run_id, rule_id, rule_type, severity, source,
                message, file_path, line, end_line, column_start, column_end,
                is_new, is_blocking, effort_minutes, context, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)",
        )
        .bind(&issue.id)
        .bind(&issue.quality_run_id)
        .bind(&issue.rule_id)
        .bind(&issue.rule_type)
        .bind(&issue.severity)
        .bind(&issue.source)
        .bind(&issue.message)
        .bind(&issue.file_path)
        .bind(issue.line)
        .bind(issue.end_line)
        .bind(issue.column_start)
        .bind(issue.column_end)
        .bind(issue.is_new)
        .bind(issue.is_blocking)
        .bind(issue.effort_minutes)
        .bind(&issue.context)
        .bind(issue.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Batch insert quality issues within a transaction
    pub async fn insert_batch(
        pool: &SqlitePool,
        issues: &[QualityIssueRecord],
    ) -> sqlx::Result<()> {
        let mut tx = pool.begin().await?;
        for issue in issues {
            sqlx::query(
                r"INSERT INTO quality_issue (
                    id, quality_run_id, rule_id, rule_type, severity, source,
                    message, file_path, line, end_line, column_start, column_end,
                    is_new, is_blocking, effort_minutes, context, created_at
                ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)",
            )
            .bind(&issue.id)
            .bind(&issue.quality_run_id)
            .bind(&issue.rule_id)
            .bind(&issue.rule_type)
            .bind(&issue.severity)
            .bind(&issue.source)
            .bind(&issue.message)
            .bind(&issue.file_path)
            .bind(issue.line)
            .bind(issue.end_line)
            .bind(issue.column_start)
            .bind(issue.column_end)
            .bind(issue.is_new)
            .bind(issue.is_blocking)
            .bind(issue.effort_minutes)
            .bind(&issue.context)
            .bind(issue.created_at)
            .execute(&mut *tx)
            .await?;
        }
        tx.commit().await?;
        Ok(())
    }

    /// Find issues by quality run ID
    pub async fn find_by_run(
        pool: &SqlitePool,
        quality_run_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, QualityIssueRecord>(
            r"SELECT * FROM quality_issue WHERE quality_run_id = ? ORDER BY severity DESC, file_path ASC, line ASC",
        )
        .bind(quality_run_id)
        .fetch_all(pool)
        .await
    }

    /// Find blocking issues by quality run ID
    pub async fn find_blocking_by_run(
        pool: &SqlitePool,
        quality_run_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, QualityIssueRecord>(
            r"SELECT * FROM quality_issue WHERE quality_run_id = ? AND is_blocking = 1 ORDER BY severity DESC, file_path ASC, line ASC",
        )
        .bind(quality_run_id)
        .fetch_all(pool)
        .await
    }

    /// Count issues by severity for a quality run
    pub async fn count_by_severity(
        pool: &SqlitePool,
        quality_run_id: &str,
    ) -> sqlx::Result<Vec<SeverityCount>> {
        sqlx::query_as::<_, SeverityCount>(
            r"SELECT severity, COUNT(*) as count FROM quality_issue WHERE quality_run_id = ? GROUP BY severity ORDER BY severity DESC",
        )
        .bind(quality_run_id)
        .fetch_all(pool)
        .await
    }

    /// Delete issues belonging to a quality run (cascade handled by FK, but explicit for safety)
    pub async fn delete_by_run(pool: &SqlitePool, quality_run_id: &str) -> sqlx::Result<u64> {
        let result = sqlx::query(r"DELETE FROM quality_issue WHERE quality_run_id = ?")
            .bind(quality_run_id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }
}

/// Helper struct for severity count aggregation
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
pub struct SeverityCount {
    pub severity: String,
    pub count: i32,
}
