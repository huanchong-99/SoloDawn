//! Terminal Model
//!
//! Stores terminal configuration and state for each task.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool, Type};
use strum_macros::{Display, EnumString};
use ts_rs::TS;
use uuid::Uuid;

/// Terminal Status Enum
#[derive(
    Debug,
    Clone,
    Copy,
    PartialEq,
    Eq,
    Type,
    Serialize,
    Deserialize,
    TS,
    EnumString,
    Display,
    Default,
)]
#[sqlx(type_name = "terminal_status", rename_all = "snake_case")]
#[serde(rename_all = "snake_case")]
#[strum(serialize_all = "snake_case")]
pub enum TerminalStatus {
    /// Not started
    #[default]
    NotStarted,
    /// Starting
    Starting,
    /// Waiting (started, waiting for instructions)
    Waiting,
    /// Working
    Working,
    /// Completed
    Completed,
    /// Failed
    Failed,
    /// Cancelled
    Cancelled,
    /// Review passed
    ReviewPassed,
    /// Review rejected
    ReviewRejected,
    /// Quality gate pending
    QualityPending,
}

/// Terminal
///
/// Corresponds to database table: terminal
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct Terminal {
    /// Primary key ID (UUID as String)
    pub id: String,

    /// Associated workflow task ID
    pub workflow_task_id: String,

    /// CLI type ID
    pub cli_type_id: String,

    /// Model config ID
    pub model_config_id: String,

    /// Custom API Base URL
    pub custom_base_url: Option<String>,

    /// Custom API Key (encrypted storage)
    #[serde(skip)]
    #[ts(skip)]
    pub custom_api_key: Option<String>,

    /// Role, e.g., 'coder', 'reviewer', 'fixer'
    pub role: Option<String>,

    /// Role description
    pub role_description: Option<String>,

    /// Execution order within task
    pub order_index: i32,

    /// Status (stored as TEXT in SQLite).
    ///
    /// # Database Constraint Note
    /// The `terminal` table currently has no CHECK constraint on this column.
    /// Adding `CHECK(status IN ('not_started','starting','waiting','working',
    /// 'completed','failed','cancelled','review_passed','review_rejected',
    /// 'quality_pending'))` requires a new migration. Application-layer
    /// validation is enforced via `TerminalStatus` enum and CAS methods.
    pub status: String,

    /// OS process ID
    pub process_id: Option<i32>,

    /// PTY session ID
    pub pty_session_id: Option<String>,

    /// Associated session ID (NEW FIELD)
    pub session_id: Option<String>,

    /// Associated execution process ID (NEW FIELD)
    pub execution_process_id: Option<String>,

    /// Associated solodawn session ID
    pub vk_session_id: Option<Uuid>,

    /// Auto-confirm mode: skip CLI permission prompts
    /// When enabled, CLI will be launched with auto-confirm flags:
    /// - Claude Code: --dangerously-skip-permissions
    /// - Codex: --yolo
    /// - Gemini: --yolo
    pub auto_confirm: bool,

    /// Last Git commit hash
    pub last_commit_hash: Option<String>,

    /// Last Git commit message
    pub last_commit_message: Option<String>,

    /// Started timestamp
    pub started_at: Option<DateTime<Utc>>,

    /// Completed timestamp
    pub completed_at: Option<DateTime<Utc>>,

    /// Created timestamp
    pub created_at: DateTime<Utc>,

    /// Updated timestamp
    pub updated_at: DateTime<Utc>,
}

/// Terminal Log Type
#[derive(
    Debug, Clone, Copy, PartialEq, Eq, Type, Serialize, Deserialize, TS, EnumString, Display,
)]
#[sqlx(type_name = "terminal_log_type", rename_all = "lowercase")]
#[serde(rename_all = "snake_case")]
#[strum(serialize_all = "lowercase")]
pub enum TerminalLogType {
    Stdout,
    Stderr,
    System,
    GitEvent,
}

/// Terminal Log
///
/// Corresponds to database table: terminal_log
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct TerminalLog {
    /// Primary key ID
    pub id: String,

    /// Associated terminal ID
    pub terminal_id: String,

    /// Log type
    pub log_type: String,

    /// Log content
    pub content: String,

    /// Created timestamp
    pub created_at: DateTime<Utc>,
}

/// Terminal Detail (includes associated CLI and model info)
///
/// For API response with complete terminal information
#[derive(Debug, Clone, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct TerminalDetail {
    /// Terminal basic info
    #[serde(flatten)]
    #[ts(flatten)]
    pub terminal: Terminal,

    /// CLI type info
    pub cli_type: super::cli_type::CliType,

    /// Model config info
    pub model_config: super::cli_type::ModelConfig,
}

impl Terminal {
    /// Set custom API key with encryption
    pub fn set_custom_api_key(&mut self, plaintext: &str) -> anyhow::Result<()> {
        self.custom_api_key = Some(crate::encryption::encrypt(plaintext)?);
        Ok(())
    }

    /// Get custom API key with decryption
    pub fn get_custom_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.custom_api_key {
            None => Ok(None),
            Some(encoded) => crate::encryption::decrypt(encoded).map(Some),
        }
    }

    /// Create terminal
    pub async fn create(pool: &SqlitePool, terminal: &Terminal) -> sqlx::Result<Self> {
        sqlx::query_as::<_, Terminal>(
            r"
            INSERT INTO terminal (
                id, workflow_task_id, cli_type_id, model_config_id,
                custom_base_url, custom_api_key, role, role_description,
                order_index, status, auto_confirm, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)
            RETURNING *
            ",
        )
        .bind(&terminal.id)
        .bind(&terminal.workflow_task_id)
        .bind(&terminal.cli_type_id)
        .bind(&terminal.model_config_id)
        .bind(&terminal.custom_base_url)
        .bind(&terminal.custom_api_key)
        .bind(&terminal.role)
        .bind(&terminal.role_description)
        .bind(terminal.order_index)
        .bind(&terminal.status)
        .bind(terminal.auto_confirm)
        .bind(terminal.created_at)
        .bind(terminal.updated_at)
        .fetch_one(pool)
        .await
    }

    /// Find terminal by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Terminal>(r"SELECT * FROM terminal WHERE id = ?")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Find terminals by task
    pub async fn find_by_task(
        pool: &SqlitePool,
        workflow_task_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Terminal>(
            r"
            SELECT * FROM terminal
            WHERE workflow_task_id = ?
            ORDER BY order_index ASC
            ",
        )
        .bind(workflow_task_id)
        .fetch_all(pool)
        .await
    }

    /// Find terminals by workflow (across tasks)
    pub async fn find_by_workflow(pool: &SqlitePool, workflow_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Terminal>(
            r"
            SELECT t.* FROM terminal t
            INNER JOIN workflow_task wt ON t.workflow_task_id = wt.id
            WHERE wt.workflow_id = ?
            ORDER BY wt.order_index ASC, t.order_index ASC
            ",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    /// Update terminal status
    ///
    /// When the new status is a terminal state (`failed` or `cancelled`),
    /// `completed_at` is set automatically to prevent dangling incomplete
    /// records.
    pub async fn update_status(pool: &SqlitePool, id: &str, status: &str) -> sqlx::Result<()> {
        let now = Utc::now();
        let is_terminal_state = status == "failed" || status == "cancelled";
        if is_terminal_state {
            sqlx::query(
                r"
                UPDATE terminal
                SET status = ?, completed_at = COALESCE(completed_at, ?), updated_at = ?
                WHERE id = ?
                ",
            )
            .bind(status)
            .bind(now)
            .bind(now)
            .bind(id)
            .execute(pool)
            .await?;
        } else {
            sqlx::query(
                r"
                UPDATE terminal
                SET status = ?, updated_at = ?
                WHERE id = ?
                ",
            )
            .bind(status)
            .bind(now)
            .bind(id)
            .execute(pool)
            .await?;
        }
        Ok(())
    }

    /// Fully reset runtime-related state so a terminal can be safely restarted.
    pub async fn reset_for_restart(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        let now = Utc::now();
        sqlx::query(
            r"
            UPDATE terminal
            SET status = 'not_started',
                process_id = NULL,
                pty_session_id = NULL,
                session_id = NULL,
                execution_process_id = NULL,
                started_at = NULL,
                completed_at = NULL,
                last_commit_hash = NULL,
                last_commit_message = NULL,
                updated_at = ?
            WHERE id = ?
            ",
        )
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Compare-and-set terminal status.
    ///
    /// Returns `Ok(true)` when the transition succeeds, `Ok(false)` when the
    /// current status does not match `expected_status`.
    pub async fn update_status_cas(
        pool: &SqlitePool,
        id: &str,
        expected_status: &str,
        next_status: &str,
    ) -> sqlx::Result<bool> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE terminal
            SET status = ?, updated_at = ?
            WHERE id = ? AND status = ?
            ",
        )
        .bind(next_status)
        .bind(now)
        .bind(id)
        .bind(expected_status)
        .execute(pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }

    /// Update terminal process info
    pub async fn update_process(
        pool: &SqlitePool,
        id: &str,
        process_id: Option<i32>,
        pty_session_id: Option<&str>,
    ) -> sqlx::Result<()> {
        let now = Utc::now();
        sqlx::query(
            r"
            UPDATE terminal
            SET process_id = ?, pty_session_id = ?, updated_at = ?
            WHERE id = ?
            ",
        )
        .bind(process_id)
        .bind(pty_session_id)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update terminal session binding
    pub async fn update_session(
        pool: &SqlitePool,
        id: &str,
        session_id: Option<&str>,
        execution_process_id: Option<&str>,
    ) -> sqlx::Result<()> {
        let now = Utc::now();
        sqlx::query(
            r"
            UPDATE terminal
            SET session_id = ?, execution_process_id = ?, updated_at = ?
            WHERE id = ?
            ",
        )
        .bind(session_id)
        .bind(execution_process_id)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update terminal last commit info
    pub async fn update_last_commit(
        pool: &SqlitePool,
        id: &str,
        commit_hash: &str,
        commit_message: &str,
    ) -> sqlx::Result<()> {
        let now = Utc::now();
        sqlx::query(
            r"
            UPDATE terminal
            SET last_commit_hash = ?, last_commit_message = ?, updated_at = ?
            WHERE id = ?
            ",
        )
        .bind(commit_hash)
        .bind(commit_message)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Set terminal to starting status (CAS: only from `not_started`).
    ///
    /// Returns `true` when the transition succeeds, `false` when the terminal
    /// is not in `not_started` state (e.g. already starting or further along).
    pub async fn set_starting(pool: &SqlitePool, id: &str) -> sqlx::Result<bool> {
        let now = Utc::now();
        let status = TerminalStatus::Starting.to_string();
        let result = sqlx::query(
            r"
            UPDATE terminal
            SET status = ?, updated_at = ?
            WHERE id = ? AND status = 'not_started'
            ",
        )
        .bind(status)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(result.rows_affected() > 0)
    }

    /// Set terminal to waiting status (CAS: only from `starting`).
    ///
    /// Returns `true` when the transition succeeds, `false` when the terminal
    /// is not in `starting` state.
    pub async fn set_waiting(pool: &SqlitePool, id: &str) -> sqlx::Result<bool> {
        let now = Utc::now();
        let status = TerminalStatus::Waiting.to_string();
        let result = sqlx::query(
            r"
            UPDATE terminal
            SET status = ?, started_at = ?, updated_at = ?
            WHERE id = ? AND status = 'starting'
            ",
        )
        .bind(status)
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(result.rows_affected() > 0)
    }

    /// Backward-compatible alias for set_waiting.
    pub async fn set_started(pool: &SqlitePool, id: &str) -> sqlx::Result<bool> {
        Self::set_waiting(pool, id).await
    }

    /// Set terminal completed
    pub async fn set_completed(pool: &SqlitePool, id: &str, status: &str) -> sqlx::Result<()> {
        let now = Utc::now();
        sqlx::query(
            r"
            UPDATE terminal
            SET status = ?, completed_at = ?, updated_at = ?
            WHERE id = ?
            ",
        )
        .bind(status)
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Compare-and-set terminal completion status.
    ///
    /// Returns `Ok(true)` when the transition succeeds, `Ok(false)` when the
    /// current status does not match `expected_status`.
    pub async fn set_completed_cas(
        pool: &SqlitePool,
        id: &str,
        expected_status: &str,
        next_status: &str,
    ) -> sqlx::Result<bool> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE terminal
            SET status = ?, completed_at = ?, updated_at = ?
            WHERE id = ? AND status = ?
            ",
        )
        .bind(next_status)
        .bind(now)
        .bind(now)
        .bind(id)
        .bind(expected_status)
        .execute(pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }

    /// Set terminal completion status only when terminal is not finalized yet.
    ///
    /// Returns `Ok(true)` when the transition succeeds, `Ok(false)` when the
    /// terminal is already finalized (completed/failed/cancelled or completed_at set).
    pub async fn set_completed_if_unfinished(
        pool: &SqlitePool,
        id: &str,
        status: &str,
    ) -> sqlx::Result<bool> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE terminal
            SET status = ?, completed_at = ?, updated_at = ?
            WHERE id = ?
              AND completed_at IS NULL
              AND status != 'completed'
              AND status != 'failed'
              AND status != 'cancelled'
            ",
        )
        .bind(status)
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }
}

impl TerminalLog {
    /// Add terminal log
    pub async fn create(
        pool: &SqlitePool,
        terminal_id: &str,
        log_type: &str,
        content: &str,
    ) -> sqlx::Result<Self> {
        let id = Uuid::new_v4().to_string();
        let now = Utc::now();
        sqlx::query_as::<_, TerminalLog>(
            r"
            INSERT INTO terminal_log (id, terminal_id, log_type, content, created_at)
            VALUES (?1, ?2, ?3, ?4, ?5)
            RETURNING *
            ",
        )
        .bind(&id)
        .bind(terminal_id)
        .bind(log_type)
        .bind(content)
        .bind(now)
        .fetch_one(pool)
        .await
    }

    /// Find logs by terminal
    pub async fn find_by_terminal(
        pool: &SqlitePool,
        terminal_id: &str,
        limit: Option<i32>,
    ) -> sqlx::Result<Vec<Self>> {
        let limit = limit.unwrap_or(1000);
        sqlx::query_as::<_, TerminalLog>(
            r"
            SELECT * FROM terminal_log
            WHERE terminal_id = ?
            ORDER BY created_at DESC
            LIMIT ?
            ",
        )
        .bind(terminal_id)
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    /// Delete old logs for a terminal, keeping only the most recent `keep` entries.
    pub async fn cleanup_old_logs(
        pool: &SqlitePool,
        terminal_id: &str,
        keep: i64,
    ) -> sqlx::Result<u64> {
        let result = sqlx::query(
            r"
            DELETE FROM terminal_log
            WHERE terminal_id = ?1
              AND id NOT IN (
                SELECT id FROM terminal_log
                WHERE terminal_id = ?1
                ORDER BY created_at DESC
                LIMIT ?2
              )
            ",
        )
        .bind(terminal_id)
        .bind(keep)
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }

    /// Delete all logs for terminals belonging to completed/failed/cancelled workflows.
    pub async fn cleanup_finished_workflow_logs(pool: &SqlitePool) -> sqlx::Result<u64> {
        let result = sqlx::query(
            r"
            DELETE FROM terminal_log
            WHERE terminal_id IN (
                SELECT t.id FROM terminal t
                JOIN workflow_task wt ON t.workflow_task_id = wt.id
                JOIN workflow w ON wt.workflow_id = w.id
                WHERE w.status IN ('completed', 'failed', 'cancelled')
            )
            ",
        )
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use serial_test::serial;

    use super::*;
    use crate::models::workflow::CreateTerminalRequest;

    fn with_var<F>(key: &str, value: Option<&str>, f: F)
    where
        F: FnOnce(),
    {
        if let Some(v) = value {
            unsafe { std::env::set_var(key, v) };
        } else {
            unsafe { std::env::remove_var(key) };
        }
        f();
        if value.is_some() {
            unsafe { std::env::remove_var(key) };
        }
    }

    #[test]
    fn test_create_terminal_request_auto_confirm_defaults_to_true() {
        let request: CreateTerminalRequest = serde_json::from_value(json!({
            "cliTypeId": "claude-code",
            "modelConfigId": "model-1",
            "orderIndex": 0
        }))
        .expect("deserialization should succeed");

        assert!(
            request.auto_confirm,
            "auto_confirm should default to true when not specified"
        );
    }

    #[test]
    fn test_create_terminal_request_auto_confirm_respects_explicit_false() {
        let request: CreateTerminalRequest = serde_json::from_value(json!({
            "cliTypeId": "claude-code",
            "modelConfigId": "model-1",
            "orderIndex": 0,
            "autoConfirm": false
        }))
        .expect("deserialization should succeed");

        assert!(
            !request.auto_confirm,
            "auto_confirm should respect explicit false value"
        );
    }

    #[test]
    fn test_create_terminal_request_auto_confirm_respects_explicit_true() {
        let request: CreateTerminalRequest = serde_json::from_value(json!({
            "cliTypeId": "claude-code",
            "modelConfigId": "model-1",
            "orderIndex": 0,
            "autoConfirm": true
        }))
        .expect("deserialization should succeed");

        assert!(
            request.auto_confirm,
            "auto_confirm should respect explicit true value"
        );
    }

    #[test]
    #[serial]
    fn test_custom_api_key_encryption_roundtrip() {
        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let mut terminal = Terminal {
                    id: Uuid::new_v4().to_string(),
                    workflow_task_id: "task-1".to_string(),
                    cli_type_id: "cli-1".to_string(),
                    model_config_id: "model-1".to_string(),
                    custom_base_url: Some("https://api.test.com".to_string()),
                    custom_api_key: None,
                    role: Some("coder".to_string()),
                    role_description: None,
                    order_index: 0,
                    status: "not_started".to_string(),
                    process_id: None,
                    pty_session_id: None,
                    session_id: None,
                    execution_process_id: None,
                    vk_session_id: None,
                    auto_confirm: false,
                    last_commit_hash: None,
                    last_commit_message: None,
                    started_at: None,
                    completed_at: None,
                    created_at: Utc::now(),
                    updated_at: Utc::now(),
                };

                let original_key = "sk-test-terminal-key-12345";

                // Encrypt the API key
                terminal
                    .set_custom_api_key(original_key)
                    .expect("Encryption should succeed");

                // Verify the stored value is encrypted (not plaintext)
                assert!(terminal.custom_api_key.is_some());
                let stored = terminal.custom_api_key.as_ref().unwrap();
                assert_ne!(stored, original_key);
                assert!(!stored.contains("sk-test"));

                // Decrypt and verify
                let decrypted_key = terminal
                    .get_custom_api_key()
                    .expect("Decryption should succeed")
                    .expect("Decrypted key should exist");
                assert_eq!(decrypted_key, original_key);
            },
        );
    }

    #[test]
    #[serial]
    fn test_custom_api_key_encryption_missing_env_key() {
        with_var("SOLODAWN_ENCRYPTION_KEY", Option::<&str>::None, || {
            let mut terminal = Terminal {
                id: Uuid::new_v4().to_string(),
                workflow_task_id: "task-1".to_string(),
                cli_type_id: "cli-1".to_string(),
                model_config_id: "model-1".to_string(),
                custom_base_url: None,
                custom_api_key: None,
                role: None,
                role_description: None,
                order_index: 0,
                status: "not_started".to_string(),
                process_id: None,
                pty_session_id: None,
                session_id: None,
                execution_process_id: None,
                vk_session_id: None,
                auto_confirm: false,
                last_commit_hash: None,
                last_commit_message: None,
                started_at: None,
                completed_at: None,
                created_at: Utc::now(),
                updated_at: Utc::now(),
            };

            // Should fail without encryption key
            let result = terminal.set_custom_api_key("sk-test");
            assert!(result.is_err());
            assert!(
                result
                    .unwrap_err()
                    .to_string()
                    .contains("SOLODAWN_ENCRYPTION_KEY")
            );
        });
    }

    #[test]
    #[serial]
    fn test_custom_api_key_encryption_invalid_key_length() {
        with_var("SOLODAWN_ENCRYPTION_KEY", Some("short"), || {
            let mut terminal = Terminal {
                id: Uuid::new_v4().to_string(),
                workflow_task_id: "task-1".to_string(),
                cli_type_id: "cli-1".to_string(),
                model_config_id: "model-1".to_string(),
                custom_base_url: None,
                custom_api_key: None,
                role: None,
                role_description: None,
                order_index: 0,
                status: "not_started".to_string(),
                process_id: None,
                pty_session_id: None,
                session_id: None,
                execution_process_id: None,
                vk_session_id: None,
                auto_confirm: false,
                last_commit_hash: None,
                last_commit_message: None,
                started_at: None,
                completed_at: None,
                created_at: Utc::now(),
                updated_at: Utc::now(),
            };

            let result = terminal.set_custom_api_key("sk-test");
            assert!(result.is_err());
            assert!(result.unwrap_err().to_string().contains("32 bytes"));
        });
    }

    #[test]
    #[serial]
    fn test_custom_api_key_none_returns_none() {
        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let terminal = Terminal {
                    id: Uuid::new_v4().to_string(),
                    workflow_task_id: "task-1".to_string(),
                    cli_type_id: "cli-1".to_string(),
                    model_config_id: "model-1".to_string(),
                    custom_base_url: None,
                    custom_api_key: None,
                    role: None,
                    role_description: None,
                    order_index: 0,
                    status: "not_started".to_string(),
                    process_id: None,
                    pty_session_id: None,
                    session_id: None,
                    execution_process_id: None,
                    vk_session_id: None,
                    auto_confirm: false,
                    last_commit_hash: None,
                    last_commit_message: None,
                    started_at: None,
                    completed_at: None,
                    created_at: Utc::now(),
                    updated_at: Utc::now(),
                };

                let key = terminal.get_custom_api_key().unwrap();
                assert!(key.is_none());
            },
        );
    }

    #[test]
    #[serial]
    fn test_custom_api_key_serialization_skips_encrypted() {
        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let mut terminal = Terminal {
                    id: Uuid::new_v4().to_string(),
                    workflow_task_id: "task-1".to_string(),
                    cli_type_id: "cli-1".to_string(),
                    model_config_id: "model-1".to_string(),
                    custom_base_url: None,
                    custom_api_key: None,
                    role: None,
                    role_description: None,
                    order_index: 0,
                    status: "not_started".to_string(),
                    process_id: None,
                    pty_session_id: None,
                    session_id: None,
                    execution_process_id: None,
                    vk_session_id: None,
                    auto_confirm: false,
                    last_commit_hash: None,
                    last_commit_message: None,
                    started_at: None,
                    completed_at: None,
                    created_at: Utc::now(),
                    updated_at: Utc::now(),
                };

                terminal.set_custom_api_key("sk-test").unwrap();

                // Serialize to JSON
                let json = serde_json::to_string(&terminal).unwrap();

                // Encrypted field should not be in JSON (due to #[serde(skip)])
                assert!(!json.contains("custom_api_key"));
                assert!(!json.contains("sk-test"));
            },
        );
    }

    #[test]
    #[serial]
    fn test_custom_api_key_dto_masks_sensitive_data() {
        // Test that DTO never exposes API keys
        // Note: This test will be implemented in the server crate where TerminalDto is defined
        // Here we just verify the encryption/decryption works

        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let mut terminal = Terminal {
                    id: "term-1".to_string(),
                    workflow_task_id: "task-1".to_string(),
                    cli_type_id: "cli-1".to_string(),
                    model_config_id: "model-1".to_string(),
                    custom_base_url: Some("https://api.test.com".to_string()),
                    custom_api_key: None,
                    role: Some("coder".to_string()),
                    role_description: None,
                    order_index: 0,
                    status: "not_started".to_string(),
                    process_id: None,
                    pty_session_id: None,
                    session_id: None,
                    execution_process_id: None,
                    vk_session_id: None,
                    auto_confirm: false,
                    last_commit_hash: None,
                    last_commit_message: None,
                    started_at: None,
                    completed_at: None,
                    created_at: Utc::now(),
                    updated_at: Utc::now(),
                };

                terminal.set_custom_api_key("sk-secret-key").unwrap();

                // Verify the key is encrypted in storage
                assert!(terminal.custom_api_key.is_some());
                let stored = terminal.custom_api_key.as_ref().unwrap();
                assert!(!stored.contains("sk-secret-key"));

                // Verify we can decrypt it
                let decrypted = terminal.get_custom_api_key().unwrap().unwrap();
                assert_eq!(decrypted, "sk-secret-key");

                // Verify serialization doesn't expose it (due to #[serde(skip)])
                let json = serde_json::to_string(&terminal).unwrap();
                assert!(!json.contains("sk-secret-key"));
                assert!(!json.contains("custom_api_key"));
            },
        );
    }

    #[tokio::test]
    async fn test_update_status_cas_transitions_and_miss() {
        let pool = SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE terminal (
                id TEXT PRIMARY KEY,
                status TEXT NOT NULL,
                updated_at TEXT,
                completed_at TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query("INSERT INTO terminal (id, status) VALUES (?1, ?2)")
            .bind("term-cas-1")
            .bind("waiting")
            .execute(&pool)
            .await
            .unwrap();

        let transitioned = Terminal::update_status_cas(&pool, "term-cas-1", "waiting", "working")
            .await
            .unwrap();
        assert!(transitioned);

        let status: String = sqlx::query_scalar("SELECT status FROM terminal WHERE id = ?1")
            .bind("term-cas-1")
            .fetch_one(&pool)
            .await
            .unwrap();
        assert_eq!(status, "working");

        let cas_miss = Terminal::update_status_cas(&pool, "term-cas-1", "waiting", "completed")
            .await
            .unwrap();
        assert!(!cas_miss);

        let status_after_miss: String =
            sqlx::query_scalar("SELECT status FROM terminal WHERE id = ?1")
                .bind("term-cas-1")
                .fetch_one(&pool)
                .await
                .unwrap();
        assert_eq!(status_after_miss, "working");
    }

    #[tokio::test]
    async fn test_set_completed_cas_sets_completed_at() {
        let pool = SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE terminal (
                id TEXT PRIMARY KEY,
                status TEXT NOT NULL,
                updated_at TEXT,
                completed_at TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query("INSERT INTO terminal (id, status, completed_at) VALUES (?1, ?2, NULL)")
            .bind("term-cas-2")
            .bind("working")
            .execute(&pool)
            .await
            .unwrap();

        let transitioned =
            Terminal::set_completed_cas(&pool, "term-cas-2", "working", "completed")
                .await
                .unwrap();
        assert!(transitioned);

        let status: String = sqlx::query_scalar("SELECT status FROM terminal WHERE id = ?1")
            .bind("term-cas-2")
            .fetch_one(&pool)
            .await
            .unwrap();
        assert_eq!(status, "completed");

        let completed_exists: i64 =
            sqlx::query_scalar("SELECT CASE WHEN completed_at IS NULL THEN 0 ELSE 1 END FROM terminal WHERE id = ?1")
                .bind("term-cas-2")
                .fetch_one(&pool)
                .await
                .unwrap();
        assert_eq!(completed_exists, 1);

        let cas_miss = Terminal::set_completed_cas(&pool, "term-cas-2", "working", "failed")
            .await
            .unwrap();
        assert!(!cas_miss);
    }

    #[tokio::test]
    async fn test_set_completed_if_unfinished_respects_final_states() {
        let pool = SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE terminal (
                id TEXT PRIMARY KEY,
                status TEXT NOT NULL,
                updated_at TEXT,
                completed_at TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query("INSERT INTO terminal (id, status, completed_at) VALUES (?1, ?2, NULL)")
            .bind("term-fallback-1")
            .bind("waiting")
            .execute(&pool)
            .await
            .unwrap();

        let updated = Terminal::set_completed_if_unfinished(&pool, "term-fallback-1", "failed")
            .await
            .unwrap();
        assert!(updated);

        let waiting_to_failed: String =
            sqlx::query_scalar("SELECT status FROM terminal WHERE id = ?1")
                .bind("term-fallback-1")
                .fetch_one(&pool)
                .await
                .unwrap();
        assert_eq!(waiting_to_failed, "failed");

        sqlx::query("INSERT INTO terminal (id, status, completed_at) VALUES (?1, ?2, ?3)")
            .bind("term-fallback-2")
            .bind("completed")
            .bind(chrono::Utc::now())
            .execute(&pool)
            .await
            .unwrap();

        let finalized_skip =
            Terminal::set_completed_if_unfinished(&pool, "term-fallback-2", "failed")
                .await
                .unwrap();
        assert!(!finalized_skip);

        let completed_stays: String = sqlx::query_scalar("SELECT status FROM terminal WHERE id = ?1")
            .bind("term-fallback-2")
            .fetch_one(&pool)
            .await
            .unwrap();
        assert_eq!(completed_stays, "completed");

        sqlx::query("INSERT INTO terminal (id, status, completed_at) VALUES (?1, ?2, NULL)")
            .bind("term-fallback-3")
            .bind("cancelled")
            .execute(&pool)
            .await
            .unwrap();

        let cancelled_skip =
            Terminal::set_completed_if_unfinished(&pool, "term-fallback-3", "failed")
                .await
                .unwrap();
        assert!(!cancelled_skip);

        let cancelled_stays: String = sqlx::query_scalar("SELECT status FROM terminal WHERE id = ?1")
            .bind("term-fallback-3")
            .fetch_one(&pool)
            .await
            .unwrap();
        assert_eq!(cancelled_stays, "cancelled");
    }
}
