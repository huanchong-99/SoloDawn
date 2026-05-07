use std::collections::{HashMap, HashSet};

use chrono::{DateTime, Utc};
use executors::{
    actions::{ExecutorAction, ExecutorActionType},
    profile::ExecutorProfileId,
};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sqlx::{FromRow, Sqlite, SqlitePool, Type};
use thiserror::Error;
use ts_rs::TS;
use uuid::Uuid;

use super::{
    execution_process_repo_state::{CreateExecutionProcessRepoState, ExecutionProcessRepoState},
    project::Project,
    repo::Repo,
    session::Session,
    task::Task,
    workspace::Workspace,
    workspace_repo::WorkspaceRepo,
};

#[derive(Debug, Error)]
pub enum ExecutionProcessError {
    #[error(transparent)]
    Database(#[from] sqlx::Error),
    #[error("Execution process not found")]
    ExecutionProcessNotFound,
    #[error("Failed to create execution process: {0}")]
    CreateFailed(String),
    #[error("Failed to update execution process: {0}")]
    UpdateFailed(String),
    #[error("Invalid executor action format")]
    InvalidExecutorAction,
    #[error("Validation error: {0}")]
    ValidationError(String),
}

#[derive(Debug, Clone, Type, Serialize, Deserialize, PartialEq, TS)]
#[sqlx(type_name = "execution_process_status", rename_all = "lowercase")]
#[serde(rename_all = "lowercase")]
#[ts(use_ts_enum)]
pub enum ExecutionProcessStatus {
    Running,
    Completed,
    Failed,
    Killed,
}

/// Describes why an execution process was launched.
///
/// **Design Decision (P29-G04):** Quality scans are intentionally NOT modelled
/// as `ExecutionProcess` records. They have their own lifecycle tracked in the
/// `quality_run` table (see `crates/db/src/models/quality_run.rs`).
/// The `QualityScan` variant is reserved here so that, if a future integration
/// needs to spawn a long-running quality-analysis process inside a PTY, the
/// run-reason taxonomy already accommodates it without a migration.
#[derive(Debug, Clone, Type, Serialize, Deserialize, PartialEq, TS)]
#[sqlx(type_name = "execution_process_run_reason", rename_all = "lowercase")]
#[serde(rename_all = "lowercase")]
pub enum ExecutionProcessRunReason {
    SetupScript,
    CleanupScript,
    CodingAgent,
    DevServer,
    /// Reserved for future quality-scan process integration.
    /// Currently quality scans are tracked separately in `quality_run`.
    QualityScan,
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct ExecutionProcess {
    pub id: Uuid,
    pub session_id: Uuid,
    pub run_reason: ExecutionProcessRunReason,
    #[ts(type = "ExecutorAction")]
    pub executor_action: sqlx::types::Json<ExecutorActionField>,
    pub status: ExecutionProcessStatus,
    pub exit_code: Option<i64>,
    /// dropped: true if this process is excluded from the current
    /// history view (due to restore/trimming). Hidden from logs/timeline;
    /// still listed in the Processes tab.
    pub dropped: bool,
    pub started_at: DateTime<Utc>,
    pub completed_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateExecutionProcess {
    pub session_id: Uuid,
    pub executor_action: ExecutorAction,
    pub run_reason: ExecutionProcessRunReason,
}

#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[allow(dead_code)]
pub struct UpdateExecutionProcess {
    pub status: Option<ExecutionProcessStatus>,
    pub exit_code: Option<i64>,
    pub completed_at: Option<DateTime<Utc>>,
}

#[derive(Debug)]
pub struct ExecutionContext {
    pub execution_process: ExecutionProcess,
    pub session: Session,
    pub workspace: Workspace,
    pub task: Task,
    pub project: Project,
    pub repos: Vec<Repo>,
}

/// Summary info about the latest execution process for a workspace
#[derive(Debug, Clone, FromRow)]
pub struct LatestProcessInfo {
    pub workspace_id: Uuid,
    pub execution_process_id: Uuid,
    pub session_id: Uuid,
    pub status: ExecutionProcessStatus,
    pub completed_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum ExecutorActionField {
    ExecutorAction(ExecutorAction),
    Other(Value),
}

#[derive(Debug, Clone, FromRow)]
pub struct MissingBeforeContext {
    pub id: Uuid,
    pub session_id: Uuid,
    pub workspace_id: Uuid,
    pub repo_id: Uuid,
    pub prev_after_head_commit: Option<String>,
    pub target_branch: String,
    pub repo_path: Option<String>,
}

impl ExecutionProcess {
    /// Find execution process by ID
    pub async fn find_by_id(pool: &SqlitePool, id: Uuid) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                    ep.id as "id!: Uuid",
                    ep.session_id as "session_id!: Uuid",
                    ep.run_reason as "run_reason!: ExecutionProcessRunReason",
                    ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                    ep.status as "status!: ExecutionProcessStatus",
                    ep.exit_code,
                    ep.dropped as "dropped!: bool",
                    ep.started_at as "started_at!: DateTime<Utc>",
                    ep.completed_at as "completed_at?: DateTime<Utc>",
                    ep.created_at as "created_at!: DateTime<Utc>",
                    ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep WHERE ep.id = ?"#,
            id
        )
        .fetch_optional(pool)
        .await
    }

    /// Context for backfilling before_head_commit for legacy rows
    /// List processes that have after_head_commit set but missing before_head_commit, with join context
    // NOTE(W2-15-05): This backfill path now uses a window function instead of
    // a correlated subquery, so it no longer re-scans `execution_processes`
    // per row. It remains intentionally unbounded because startup backfill
    // should clear the full outstanding set in one sweep.
    pub async fn list_missing_before_context(
        pool: &SqlitePool,
    ) -> Result<Vec<MissingBeforeContext>, sqlx::Error> {
        sqlx::query_as::<Sqlite, MissingBeforeContext>(
            r"WITH repo_history AS (
                   SELECT
                       ep.id,
                       ep.session_id,
                       s.workspace_id,
                       eprs.repo_id,
                       eprs.before_head_commit,
                       eprs.after_head_commit,
                       LAG(eprs.after_head_commit) OVER (
                           PARTITION BY ep.session_id, eprs.repo_id
                           ORDER BY ep.created_at ASC, ep.id ASC
                       ) AS prev_after_head_commit,
                       wr.target_branch,
                       r.path AS repo_path
                   FROM execution_processes ep
                   JOIN sessions s ON s.id = ep.session_id
                   JOIN execution_process_repo_states eprs
                     ON eprs.execution_process_id = ep.id
                   JOIN repos r ON r.id = eprs.repo_id
                   JOIN workspace_repos wr
                     ON wr.workspace_id = s.workspace_id
                    AND wr.repo_id = eprs.repo_id
               )
               SELECT
                   id,
                   session_id,
                   workspace_id,
                   repo_id,
                   prev_after_head_commit,
                   target_branch,
                   repo_path
               FROM repo_history
               WHERE before_head_commit IS NULL
                 AND after_head_commit IS NOT NULL",
        )
        .fetch_all(pool)
        .await
    }

    /// Find execution process by rowid
    pub async fn find_by_rowid(pool: &SqlitePool, rowid: i64) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                    ep.id as "id!: Uuid",
                    ep.session_id as "session_id!: Uuid",
                    ep.run_reason as "run_reason!: ExecutionProcessRunReason",
                    ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                    ep.status as "status!: ExecutionProcessStatus",
                    ep.exit_code,
                    ep.dropped as "dropped!: bool",
                    ep.started_at as "started_at!: DateTime<Utc>",
                    ep.completed_at as "completed_at?: DateTime<Utc>",
                    ep.created_at as "created_at!: DateTime<Utc>",
                    ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep WHERE ep.rowid = ?"#,
            rowid
        )
        .fetch_optional(pool)
        .await
    }

    /// Find execution processes for a session (optionally include soft-deleted).
    ///
    /// W2-15-03: capped at [`Self::FIND_BY_SESSION_ID_MAX_ROWS`] rows. Callers
    /// that need arbitrary depth should use `find_by_session_id_paginated`.
    /// The cap is high enough that a typical session (≤ a few hundred
    /// processes) returns everything, while a pathological long-running
    /// session can no longer OOM a client.
    pub async fn find_by_session_id(
        pool: &SqlitePool,
        session_id: Uuid,
        show_soft_deleted: bool,
    ) -> Result<Vec<Self>, sqlx::Error> {
        let limit: i64 = Self::FIND_BY_SESSION_ID_MAX_ROWS;
        sqlx::query_as::<_, ExecutionProcess>(
            r"SELECT
                      ep.id,
                      ep.session_id,
                      ep.run_reason,
                      ep.executor_action,
                      ep.status,
                      ep.exit_code,
                      ep.dropped,
                      ep.started_at,
                      ep.completed_at,
                      ep.created_at,
                      ep.updated_at
               FROM execution_processes ep
               WHERE ep.session_id = ?
                 AND (? OR ep.dropped = FALSE)
               ORDER BY ep.created_at ASC
               LIMIT ?",
        )
        .bind(session_id)
        .bind(show_soft_deleted)
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    /// Hard cap used by `find_by_session_id` (W2-15-03).
    pub const FIND_BY_SESSION_ID_MAX_ROWS: i64 = 5000;

    /// Find execution processes for a session with pagination (most recent first).
    pub async fn find_by_session_id_paginated(
        pool: &SqlitePool,
        session_id: Uuid,
        show_soft_deleted: bool,
        limit: i64,
        offset: i64,
    ) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                      ep.id              as "id!: Uuid",
                      ep.session_id      as "session_id!: Uuid",
                      ep.run_reason      as "run_reason!: ExecutionProcessRunReason",
                      ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                      ep.status          as "status!: ExecutionProcessStatus",
                      ep.exit_code,
                      ep.dropped as "dropped!: bool",
                      ep.started_at      as "started_at!: DateTime<Utc>",
                      ep.completed_at    as "completed_at?: DateTime<Utc>",
                      ep.created_at      as "created_at!: DateTime<Utc>",
                      ep.updated_at      as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep
               WHERE ep.session_id = ?
                 AND (? OR ep.dropped = FALSE)
               ORDER BY ep.created_at ASC
               LIMIT ? OFFSET ?"#,
            session_id,
            show_soft_deleted,
            limit,
            offset
        )
        .fetch_all(pool)
        .await
    }

    /// Find running execution processes
    // NOTE(W2-15-04): Startup orphan cleanup needs the full running set, so
    // this query remains unbounded by design. Migration
    // `20260417010000_add_perf_indexes.sql` adds
    // `idx_exec_proc_status_running` to keep the status filter cheap.
    pub async fn find_running(pool: &SqlitePool) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                    ep.id as "id!: Uuid",
                    ep.session_id as "session_id!: Uuid",
                    ep.run_reason as "run_reason!: ExecutionProcessRunReason",
                    ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                    ep.status as "status!: ExecutionProcessStatus",
                    ep.exit_code,
                    ep.dropped as "dropped!: bool",
                    ep.started_at as "started_at!: DateTime<Utc>",
                    ep.completed_at as "completed_at?: DateTime<Utc>",
                    ep.created_at as "created_at!: DateTime<Utc>",
                    ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep WHERE ep.status = 'running' ORDER BY ep.created_at ASC"#,
        )
        .fetch_all(pool)
        .await
    }

    /// Find running dev servers for a specific project
    pub async fn find_running_dev_servers_by_project(
        pool: &SqlitePool,
        project_id: Uuid,
    ) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT ep.id as "id!: Uuid", ep.session_id as "session_id!: Uuid", ep.run_reason as "run_reason!: ExecutionProcessRunReason", ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                      ep.status as "status!: ExecutionProcessStatus", ep.exit_code,
                      ep.dropped as "dropped!: bool", ep.started_at as "started_at!: DateTime<Utc>", ep.completed_at as "completed_at?: DateTime<Utc>", ep.created_at as "created_at!: DateTime<Utc>", ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep
               JOIN sessions s ON ep.session_id = s.id
               JOIN workspaces w ON s.workspace_id = w.id
               JOIN tasks t ON w.task_id = t.id
               WHERE ep.status = 'running' AND ep.run_reason = 'devserver' AND t.project_id = ?
               ORDER BY ep.created_at ASC"#,
            project_id
        )
        .fetch_all(pool)
        .await
    }

    /// Check if there are running processes (excluding dev servers) for a workspace (across all sessions)
    pub async fn has_running_non_dev_server_processes_for_workspace(
        pool: &SqlitePool,
        workspace_id: Uuid,
    ) -> Result<bool, sqlx::Error> {
        let count: i64 = sqlx::query_scalar!(
            r#"SELECT COUNT(*) as "count!: i64"
               FROM execution_processes ep
               JOIN sessions s ON ep.session_id = s.id
               WHERE s.workspace_id = $1
                 AND ep.status = 'running'
                 AND ep.run_reason != 'devserver'"#,
            workspace_id
        )
        .fetch_one(pool)
        .await?;
        Ok(count > 0)
    }

    /// Find running dev servers for a specific workspace (across all sessions)
    pub async fn find_running_dev_servers_by_workspace(
        pool: &SqlitePool,
        workspace_id: Uuid,
    ) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"
        SELECT
            ep.id as "id!: Uuid",
            ep.session_id as "session_id!: Uuid",
            ep.run_reason as "run_reason!: ExecutionProcessRunReason",
            ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
            ep.status as "status!: ExecutionProcessStatus",
            ep.exit_code,
            ep.dropped as "dropped!: bool",
            ep.started_at as "started_at!: DateTime<Utc>",
            ep.completed_at as "completed_at?: DateTime<Utc>",
            ep.created_at as "created_at!: DateTime<Utc>",
            ep.updated_at as "updated_at!: DateTime<Utc>"
        FROM execution_processes ep
        JOIN sessions s ON ep.session_id = s.id
        WHERE s.workspace_id = ?
          AND ep.status = 'running'
          AND ep.run_reason = 'devserver'
        ORDER BY ep.created_at DESC
        "#,
            workspace_id
        )
        .fetch_all(pool)
        .await
    }

    /// Find latest coding_agent_turn agent_session_id by session (simple scalar query)
    pub async fn find_latest_coding_agent_turn_session_id(
        pool: &SqlitePool,
        session_id: Uuid,
    ) -> Result<Option<String>, sqlx::Error> {
        tracing::info!(
            "Finding latest coding agent turn session id for session {}",
            session_id
        );
        let row = sqlx::query!(
            r#"SELECT cat.agent_session_id
               FROM execution_processes ep
               JOIN coding_agent_turns cat ON ep.id = cat.execution_process_id
               WHERE ep.session_id = $1
                 AND ep.run_reason = 'codingagent'
                 AND ep.dropped = FALSE
                 AND cat.agent_session_id IS NOT NULL
               ORDER BY ep.created_at DESC
               LIMIT 1"#,
            session_id
        )
        .fetch_optional(pool)
        .await?;

        tracing::info!("Latest coding agent turn session id: {:?}", row);

        Ok(row.and_then(|r| r.agent_session_id))
    }

    /// Find latest execution process by session and run reason
    pub async fn find_latest_by_session_and_run_reason(
        pool: &SqlitePool,
        session_id: Uuid,
        run_reason: &ExecutionProcessRunReason,
    ) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                    ep.id as "id!: Uuid",
                    ep.session_id as "session_id!: Uuid",
                    ep.run_reason as "run_reason!: ExecutionProcessRunReason",
                    ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                    ep.status as "status!: ExecutionProcessStatus",
                    ep.exit_code,
                    ep.dropped as "dropped!: bool",
                    ep.started_at as "started_at!: DateTime<Utc>",
                    ep.completed_at as "completed_at?: DateTime<Utc>",
                    ep.created_at as "created_at!: DateTime<Utc>",
                    ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep
               WHERE ep.session_id = ? AND ep.run_reason = ? AND ep.dropped = FALSE
               ORDER BY ep.created_at DESC LIMIT 1"#,
            session_id,
            run_reason
        )
        .fetch_optional(pool)
        .await
    }

    /// Find latest execution process by workspace and run reason (across all sessions)
    pub async fn find_latest_by_workspace_and_run_reason(
        pool: &SqlitePool,
        workspace_id: Uuid,
        run_reason: &ExecutionProcessRunReason,
    ) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                    ep.id as "id!: Uuid",
                    ep.session_id as "session_id!: Uuid",
                    ep.run_reason as "run_reason!: ExecutionProcessRunReason",
                    ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                    ep.status as "status!: ExecutionProcessStatus",
                    ep.exit_code,
                    ep.dropped as "dropped!: bool",
                    ep.started_at as "started_at!: DateTime<Utc>",
                    ep.completed_at as "completed_at?: DateTime<Utc>",
                    ep.created_at as "created_at!: DateTime<Utc>",
                    ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep
               JOIN sessions s ON ep.session_id = s.id
               WHERE s.workspace_id = ? AND ep.run_reason = ? AND ep.dropped = FALSE
               ORDER BY ep.created_at DESC LIMIT 1"#,
            workspace_id,
            run_reason
        )
        .fetch_optional(pool)
        .await
    }

    /// Create a new execution process
    ///
    /// Note: We intentionally avoid using a transaction here. SQLite update
    /// hooks fire during transactions (before commit), and the hook spawns an
    /// async task that queries `find_by_rowid` on a different connection.
    /// If we used a transaction, that query would not see the uncommitted row,
    /// causing the WebSocket event to be lost.
    pub async fn create(
        pool: &SqlitePool,
        data: &CreateExecutionProcess,
        process_id: Uuid,
        repo_states: &[CreateExecutionProcessRepoState],
    ) -> Result<Self, sqlx::Error> {
        let now = Utc::now();
        let executor_action_json = sqlx::types::Json(&data.executor_action);

        sqlx::query!(
            r#"INSERT INTO execution_processes (
                    id, session_id, run_reason, executor_action,
                    status, exit_code, started_at, completed_at, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
            process_id,
            data.session_id,
            data.run_reason,
            executor_action_json,
            ExecutionProcessStatus::Running,
            None::<i64>,
            now,
            None::<DateTime<Utc>>,
            now,
            now
        )
        .execute(pool)
        .await?;

        ExecutionProcessRepoState::create_many(pool, process_id, repo_states).await?;

        Self::find_by_id(pool, process_id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)
    }

    pub async fn was_stopped(pool: &SqlitePool, id: Uuid) -> bool {
        if let Ok(exp_process) = Self::find_by_id(pool, id).await
            && exp_process.is_some_and(|ep| {
                ep.status == ExecutionProcessStatus::Killed
                    || ep.status == ExecutionProcessStatus::Completed
            })
        {
            return true;
        }
        false
    }

    /// Update execution process status and completion info
    pub async fn update_completion(
        pool: &SqlitePool,
        id: Uuid,
        status: ExecutionProcessStatus,
        exit_code: Option<i64>,
    ) -> Result<(), sqlx::Error> {
        let completed_at = if matches!(status, ExecutionProcessStatus::Running) {
            None
        } else {
            Some(Utc::now())
        };

        sqlx::query(
            r"UPDATE execution_processes
               SET status = ?, exit_code = ?, completed_at = ?, updated_at = datetime('now')
               WHERE id = ?",
        )
        .bind(status)
        .bind(exit_code)
        .bind(completed_at)
        .bind(id)
        .execute(pool)
        .await?;

        Ok(())
    }

    pub fn executor_action(&self) -> Result<&ExecutorAction, anyhow::Error> {
        match &self.executor_action.0 {
            ExecutorActionField::ExecutorAction(action) => Ok(action),
            ExecutorActionField::Other(_) => Err(anyhow::anyhow!(
                "Executor action is not a valid ExecutorAction JSON object"
            )),
        }
    }

    /// Soft-drop processes at and after the specified boundary (inclusive)
    pub async fn drop_at_and_after(
        pool: &SqlitePool,
        session_id: Uuid,
        boundary_process_id: Uuid,
    ) -> Result<i64, sqlx::Error> {
        let result = sqlx::query!(
            r#"UPDATE execution_processes
               SET dropped = TRUE
             WHERE session_id = $1
               AND created_at >= (SELECT created_at FROM execution_processes WHERE id = $2)
               AND dropped = FALSE"#,
            session_id,
            boundary_process_id
        )
        .execute(pool)
        .await?;
        let affected = i64::try_from(result.rows_affected())
            .map_err(|_| sqlx::Error::Protocol("rows_affected overflowed i64".into()))?;
        Ok(affected)
    }

    /// Find the previous process's after_head_commit before the given boundary process
    /// for a specific repository
    pub async fn find_prev_after_head_commit(
        pool: &SqlitePool,
        session_id: Uuid,
        boundary_process_id: Uuid,
        repo_id: Uuid,
    ) -> Result<Option<String>, sqlx::Error> {
        let result = sqlx::query_scalar!(
            r#"SELECT eprs.after_head_commit
               FROM execution_process_repo_states eprs
               JOIN execution_processes ep ON ep.id = eprs.execution_process_id
              WHERE ep.session_id = $1
                AND eprs.repo_id = $2
                AND ep.created_at < (SELECT created_at FROM execution_processes WHERE id = $3)
              ORDER BY ep.created_at DESC
              LIMIT 1"#,
            session_id,
            repo_id,
            boundary_process_id
        )
        .fetch_optional(pool)
        .await?;
        Ok(result.flatten())
    }

    /// Get the parent Session for this execution process
    pub async fn parent_session(&self, pool: &SqlitePool) -> Result<Option<Session>, sqlx::Error> {
        Session::find_by_id(pool, self.session_id).await
    }

    /// Get both the parent Workspace and Session for this execution process
    pub async fn parent_workspace_and_session(
        &self,
        pool: &SqlitePool,
    ) -> Result<Option<(Workspace, Session)>, sqlx::Error> {
        let Some(session) = Session::find_by_id(pool, self.session_id).await? else {
            return Ok(None);
        };
        let Some(workspace) = Workspace::find_by_id(pool, session.workspace_id).await? else {
            return Ok(None);
        };
        Ok(Some((workspace, session)))
    }

    /// Load execution context with related session, workspace, task, project, and repos
    pub async fn load_context(
        pool: &SqlitePool,
        exec_id: Uuid,
    ) -> Result<ExecutionContext, sqlx::Error> {
        let execution_process = Self::find_by_id(pool, exec_id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let session = Session::find_by_id(pool, execution_process.session_id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let workspace = Workspace::find_by_id(pool, session.workspace_id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let task = Task::find_by_id(pool, workspace.task_id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let project = Project::find_by_id(pool, task.project_id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let repos = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;

        Ok(ExecutionContext {
            execution_process,
            session,
            workspace,
            task,
            project,
            repos,
        })
    }

    /// Fetch the latest CodingAgent executor profile for a session.
    /// Returns None if no CodingAgent execution process exists for this session.
    pub async fn latest_executor_profile_for_session(
        pool: &SqlitePool,
        session_id: Uuid,
    ) -> Result<Option<ExecutorProfileId>, ExecutionProcessError> {
        // Find the latest CodingAgent execution process for this session
        let latest_execution_process = sqlx::query_as!(
            ExecutionProcess,
            r#"SELECT
                    ep.id as "id!: Uuid",
                    ep.session_id as "session_id!: Uuid",
                    ep.run_reason as "run_reason!: ExecutionProcessRunReason",
                    ep.executor_action as "executor_action!: sqlx::types::Json<ExecutorActionField>",
                    ep.status as "status!: ExecutionProcessStatus",
                    ep.exit_code,
                    ep.dropped as "dropped!: bool",
                    ep.started_at as "started_at!: DateTime<Utc>",
                    ep.completed_at as "completed_at?: DateTime<Utc>",
                    ep.created_at as "created_at!: DateTime<Utc>",
                    ep.updated_at as "updated_at!: DateTime<Utc>"
               FROM execution_processes ep
               WHERE ep.session_id = ? AND ep.run_reason = ? AND ep.dropped = FALSE
               ORDER BY ep.created_at DESC LIMIT 1"#,
            session_id,
            ExecutionProcessRunReason::CodingAgent
        )
        .fetch_optional(pool)
        .await?;

        let Some(latest_execution_process) = latest_execution_process else {
            return Ok(None);
        };

        let action = latest_execution_process
            .executor_action()
            .map_err(|e| ExecutionProcessError::ValidationError(e.to_string()))?;

        match &action.typ {
            ExecutorActionType::CodingAgentInitialRequest(request) => {
                Ok(Some(request.executor_profile_id.clone()))
            }
            ExecutorActionType::CodingAgentFollowUpRequest(request) => {
                Ok(Some(request.executor_profile_id.clone()))
            }
            ExecutorActionType::ReviewRequest(request) => {
                Ok(Some(request.executor_profile_id.clone()))
            }
            ExecutorActionType::ScriptRequest(_) => Err(ExecutionProcessError::ValidationError(
                "Couldn't find profile from initial request".to_string(),
            )),
        }
    }

    /// Fetch latest execution process info for all workspaces with the given archived status.
    /// Returns a map of workspace_id -> LatestProcessInfo for the most recent
    /// non-dropped execution process (excluding dev servers).
    pub async fn find_latest_for_workspaces(
        pool: &SqlitePool,
        archived: bool,
    ) -> Result<HashMap<Uuid, LatestProcessInfo>, sqlx::Error> {
        let rows: Vec<LatestProcessInfo> = sqlx::query_as!(
            LatestProcessInfo,
            r#"
            SELECT
                s.workspace_id as "workspace_id!: Uuid",
                ep.id as "execution_process_id!: Uuid",
                ep.session_id as "session_id!: Uuid",
                ep.status as "status!: ExecutionProcessStatus",
                ep.completed_at as "completed_at?: DateTime<Utc>"
            FROM execution_processes ep
            JOIN sessions s ON ep.session_id = s.id
            JOIN workspaces w ON s.workspace_id = w.id
            WHERE w.archived = $1
              AND ep.run_reason IN ('codingagent', 'setupscript', 'cleanupscript')
              AND ep.dropped = FALSE
              AND ep.created_at = (
                  SELECT MAX(ep2.created_at)
                  FROM execution_processes ep2
                  JOIN sessions s2 ON ep2.session_id = s2.id
                  WHERE s2.workspace_id = s.workspace_id
                    AND ep2.run_reason IN ('codingagent', 'setupscript', 'cleanupscript')
                    AND ep2.dropped = FALSE
              )
            "#,
            archived
        )
        .fetch_all(pool)
        .await?;

        let result = rows
            .into_iter()
            .map(|info| (info.workspace_id, info))
            .collect();

        Ok(result)
    }

    /// Find all workspaces with running dev servers, filtered by archived status.
    /// Returns a set of workspace IDs that have at least one running dev server.
    pub async fn find_workspaces_with_running_dev_servers(
        pool: &SqlitePool,
        archived: bool,
    ) -> Result<HashSet<Uuid>, sqlx::Error> {
        let rows: Vec<Uuid> = sqlx::query_scalar!(
            r#"
            SELECT DISTINCT s.workspace_id as "workspace_id!: Uuid"
            FROM execution_processes ep
            JOIN sessions s ON ep.session_id = s.id
            JOIN workspaces w ON s.workspace_id = w.id
            WHERE w.archived = $1
              AND ep.status = 'running'
              AND ep.run_reason = 'devserver'
            "#,
            archived
        )
        .fetch_all(pool)
        .await?;

        Ok(rows.into_iter().collect())
    }
}
