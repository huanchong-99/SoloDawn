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
    /// Find logs by execution process ID
    // TODO(W2-15-06): Unbounded — returns every log chunk for an execution.
    // Long-running processes accumulate MBs of log rows; this query has no
    // LIMIT / OFFSET and the whole result is held in memory by callers
    // (`parse_logs` further materialises them all). Add pagination
    // (inserted_at cursor) and verify that `execution_process_logs`
    // has an index on `(execution_id, inserted_at)` — without it this is a
    // full scan + sort.
    pub async fn find_by_execution_id(
        pool: &SqlitePool,
        execution_id: Uuid,
    ) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcessLogs,
            r#"SELECT 
                execution_id as "execution_id!: Uuid",
                logs,
                byte_size,
                inserted_at as "inserted_at!: DateTime<Utc>"
               FROM execution_process_logs 
               WHERE execution_id = $1
               ORDER BY inserted_at ASC"#,
            execution_id
        )
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
