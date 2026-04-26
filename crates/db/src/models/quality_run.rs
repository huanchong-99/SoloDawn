//! Quality Run Model
//!
//! Persists quality gate execution records for audit, status tracking,
//! and UI display. Each run corresponds to one quality gate evaluation
//! triggered by a checkpoint commit or branch/repo gate.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Quality Run
///
/// Corresponds to database table: quality_run
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityRun {
    pub id: String,
    pub workflow_id: String,
    pub task_id: Option<String>,
    pub terminal_id: Option<String>,
    pub commit_hash: Option<String>,
    /// terminal | branch | repo
    pub gate_level: String,
    /// pending | running | ok | warn | error | skipped
    pub gate_status: String,
    /// off | shadow | warn | enforce
    pub mode: String,
    pub total_issues: i32,
    pub blocking_issues: i32,
    pub new_issues: i32,
    pub duration_ms: i32,
    /// JSON array of provider names
    pub providers_run: Option<String>,
    /// Full serialized QualityReport
    pub report_json: Option<String>,
    /// Serialized QualityGateDecision
    pub decision_json: Option<String>,
    /// Error message if the run itself failed
    pub error_message: Option<String>,
    pub created_at: DateTime<Utc>,
    pub completed_at: Option<DateTime<Utc>>,
}

impl QualityRun {
    /// Create a new pending quality run
    pub fn new_pending(
        workflow_id: &str,
        task_id: Option<&str>,
        terminal_id: Option<&str>,
        commit_hash: Option<&str>,
        gate_level: &str,
        mode: &str,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            workflow_id: workflow_id.to_string(),
            task_id: task_id.map(std::string::ToString::to_string),
            terminal_id: terminal_id.map(std::string::ToString::to_string),
            commit_hash: commit_hash.map(std::string::ToString::to_string),
            gate_level: gate_level.to_string(),
            gate_status: "pending".to_string(),
            mode: mode.to_string(),
            total_issues: 0,
            blocking_issues: 0,
            new_issues: 0,
            duration_ms: 0,
            providers_run: None,
            report_json: None,
            decision_json: None,
            error_message: None,
            created_at: Utc::now(),
            completed_at: None,
        }
    }

    /// Insert a new quality run record
    pub async fn insert(pool: &SqlitePool, run: &QualityRun) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO quality_run (
                id, workflow_id, task_id, terminal_id, commit_hash,
                gate_level, gate_status, mode,
                total_issues, blocking_issues, new_issues, duration_ms,
                providers_run, report_json, decision_json, error_message,
                created_at, completed_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18)",
        )
        .bind(&run.id)
        .bind(&run.workflow_id)
        .bind(&run.task_id)
        .bind(&run.terminal_id)
        .bind(&run.commit_hash)
        .bind(&run.gate_level)
        .bind(&run.gate_status)
        .bind(&run.mode)
        .bind(run.total_issues)
        .bind(run.blocking_issues)
        .bind(run.new_issues)
        .bind(run.duration_ms)
        .bind(&run.providers_run)
        .bind(&run.report_json)
        .bind(&run.decision_json)
        .bind(&run.error_message)
        .bind(run.created_at)
        .bind(run.completed_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Mark the run as running
    pub async fn set_running(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query(r"UPDATE quality_run SET gate_status = 'running' WHERE id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(())
    }

    /// Complete a quality run with results
    #[allow(clippy::too_many_arguments)]
    pub async fn complete(
        pool: &SqlitePool,
        id: &str,
        gate_status: &str,
        total_issues: i32,
        blocking_issues: i32,
        new_issues: i32,
        duration_ms: i32,
        providers_run: Option<&str>,
        report_json: Option<&str>,
        decision_json: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"UPDATE quality_run
            SET gate_status = ?1, total_issues = ?2, blocking_issues = ?3,
                new_issues = ?4, duration_ms = ?5, providers_run = ?6,
                report_json = ?7, decision_json = ?8, completed_at = ?9
            WHERE id = ?10",
        )
        .bind(gate_status)
        .bind(total_issues)
        .bind(blocking_issues)
        .bind(new_issues)
        .bind(duration_ms)
        .bind(providers_run)
        .bind(report_json)
        .bind(decision_json)
        .bind(Utc::now())
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Mark the run as failed with an error message
    pub async fn set_failed(pool: &SqlitePool, id: &str, error_message: &str) -> sqlx::Result<()> {
        sqlx::query(
            r"UPDATE quality_run
            SET gate_status = 'error', error_message = ?1, completed_at = ?2
            WHERE id = ?3",
        )
        .bind(error_message)
        .bind(Utc::now())
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Find quality runs by workflow ID
    pub async fn find_by_workflow(pool: &SqlitePool, workflow_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, QualityRun>(
            r"SELECT * FROM quality_run WHERE workflow_id = ? ORDER BY created_at DESC",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    /// Find quality runs by terminal ID
    pub async fn find_by_terminal(pool: &SqlitePool, terminal_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, QualityRun>(
            r"SELECT * FROM quality_run WHERE terminal_id = ? ORDER BY created_at DESC",
        )
        .bind(terminal_id)
        .fetch_all(pool)
        .await
    }

    /// Find the latest quality run for a terminal
    pub async fn find_latest_by_terminal(
        pool: &SqlitePool,
        terminal_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, QualityRun>(
            r"SELECT * FROM quality_run WHERE terminal_id = ? ORDER BY created_at DESC LIMIT 1",
        )
        .bind(terminal_id)
        .fetch_optional(pool)
        .await
    }

    /// Find quality runs by task ID
    pub async fn find_by_task(pool: &SqlitePool, task_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, QualityRun>(
            r"SELECT * FROM quality_run WHERE task_id = ? ORDER BY created_at DESC",
        )
        .bind(task_id)
        .fetch_all(pool)
        .await
    }

    /// Find a quality run by terminal ID and commit hash (for dedup checks).
    ///
    /// Returns the first matching run regardless of status, allowing callers
    /// to detect duplicate checkpoint processing attempts.
    pub async fn find_by_terminal_and_commit(
        pool: &SqlitePool,
        terminal_id: &str,
        commit_hash: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, QualityRun>(
            r"SELECT * FROM quality_run
            WHERE terminal_id = ? AND commit_hash = ?
            ORDER BY created_at DESC LIMIT 1",
        )
        .bind(terminal_id)
        .bind(commit_hash)
        .fetch_optional(pool)
        .await
    }

    /// Find a quality run by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, QualityRun>(r"SELECT * FROM quality_run WHERE id = ?")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Backfill legacy workflows: create placeholder quality_run records for
    /// terminals that completed before the quality gate existed (status='completed'
    /// but no corresponding quality_run). This allows the UI to show a consistent view.
    pub async fn backfill_legacy_workflows(pool: &SqlitePool) -> sqlx::Result<u64> {
        let result = sqlx::query(
            r"INSERT INTO quality_run (
                id, workflow_id, task_id, terminal_id, commit_hash,
                gate_level, gate_status, mode,
                total_issues, blocking_issues, new_issues, duration_ms,
                created_at, completed_at
            )
            SELECT
                lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' ||
                    substr(hex(randomblob(2)),2) || '-' ||
                    substr('89ab', abs(random()) % 4 + 1, 1) ||
                    substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6))),
                t.workflow_id,
                t.task_id,
                t.id,
                NULL,
                'terminal',
                'skipped',
                'off',
                0, 0, 0, 0,
                t.created_at,
                t.updated_at
            FROM terminals t
            WHERE t.status = 'completed'
              AND NOT EXISTS (
                  SELECT 1 FROM quality_run qr WHERE qr.terminal_id = t.id
              )",
        )
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }

    /// Delete quality_run records (and cascading quality_issue records) older
    /// than the specified retention period.
    pub async fn cleanup_old_runs(pool: &SqlitePool, retention_days: i64) -> sqlx::Result<u64> {
        let result = sqlx::query(
            r"DELETE FROM quality_run
            WHERE created_at < datetime('now', '-' || ?1 || ' days')",
        )
        .bind(retention_days)
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }

    /// Return counts of quality runs grouped by age buckets: 7d, 30d, 90d, older.
    pub async fn count_runs_by_age(pool: &SqlitePool) -> sqlx::Result<Vec<AgeBucketCount>> {
        sqlx::query_as::<_, AgeBucketCount>(
            r"SELECT
                CASE
                    WHEN created_at >= datetime('now', '-7 days')  THEN '7d'
                    WHEN created_at >= datetime('now', '-30 days') THEN '30d'
                    WHEN created_at >= datetime('now', '-90 days') THEN '90d'
                    ELSE 'older'
                END AS bucket,
                COUNT(*) AS count
            FROM quality_run
            GROUP BY bucket
            ORDER BY
                CASE bucket
                    WHEN '7d'    THEN 1
                    WHEN '30d'   THEN 2
                    WHEN '90d'   THEN 3
                    ELSE 4
                END",
        )
        .fetch_all(pool)
        .await
    }
}

/// Helper struct for age-bucket aggregation
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
pub struct AgeBucketCount {
    pub bucket: String,
    pub count: i32,
}
