use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use utils::log_msg::LogMsg;
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct ExecutionProcessLogs {
    pub execution_id: Uuid,
    pub logs: String, // JSONL format
    pub byte_size: i64,
    pub inserted_at: DateTime<Utc>,
}

impl ExecutionProcessLogs {
    /// Hard cap used by `find_by_execution_id` (W2-15-06). Long-running
    /// processes accumulate thousands of log rows; capping here prevents
    /// a single caller from materialising megabytes at once. Callers that
    /// need to walk the full history should use `find_by_execution_id_page`.
    pub const FIND_BY_EXECUTION_ID_MAX_ROWS: i64 = 5000;

    /// Find logs by execution process ID.
    ///
    /// W2-15-06: capped at [`Self::FIND_BY_EXECUTION_ID_MAX_ROWS`] oldest
    /// chunks. The cap is well above a typical process's chunk count, so
    /// existing UI callers are unaffected; pathological cases no longer
    /// OOM.
    pub async fn find_by_execution_id(
        pool: &SqlitePool,
        execution_id: Uuid,
    ) -> Result<Vec<Self>, sqlx::Error> {
        let limit: i64 = Self::FIND_BY_EXECUTION_ID_MAX_ROWS;
        sqlx::query_as::<_, ExecutionProcessLogs>(
            r"SELECT
                execution_id,
                logs,
                byte_size,
                inserted_at
               FROM execution_process_logs
               WHERE execution_id = $1
               ORDER BY inserted_at ASC
               LIMIT $2",
        )
        .bind(execution_id)
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    /// Paginated variant of `find_by_execution_id`. Use an `inserted_at`
    /// cursor (exclusive lower bound) to walk the full log history in
    /// stable chronological order.
    pub async fn find_by_execution_id_page(
        pool: &SqlitePool,
        execution_id: Uuid,
        after: Option<DateTime<Utc>>,
        limit: i64,
    ) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as::<_, ExecutionProcessLogs>(
            r"SELECT
                execution_id,
                logs,
                byte_size,
                inserted_at
               FROM execution_process_logs
               WHERE execution_id = $1
                 AND ($2 IS NULL OR inserted_at > $2)
               ORDER BY inserted_at ASC
               LIMIT $3",
        )
        .bind(execution_id)
        .bind(after)
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    /// Parse JSONL logs back into Vec<LogMsg>
    pub fn parse_logs(records: &[Self]) -> Result<Vec<LogMsg>, serde_json::Error> {
        let mut messages = Vec::new();
        for line in records.iter().flat_map(|record| record.logs.lines()) {
            if !line.trim().is_empty() {
                let msg: LogMsg = serde_json::from_str(line)?;
                messages.push(msg);
            }
        }
        Ok(messages)
    }

    /// Append a JSONL line to the logs for an execution process
    pub async fn append_log_line(
        pool: &SqlitePool,
        execution_id: Uuid,
        jsonl_line: &str,
    ) -> Result<(), sqlx::Error> {
        let byte_size = i64::try_from(jsonl_line.len())
            .map_err(|_| sqlx::Error::Protocol("log line too large".into()))?;
        sqlx::query!(
            r#"INSERT INTO execution_process_logs (execution_id, logs, byte_size, inserted_at)
               VALUES ($1, $2, $3, datetime('now', 'subsec'))"#,
            execution_id,
            jsonl_line,
            byte_size
        )
        .execute(pool)
        .await?;

        Ok(())
    }
}
