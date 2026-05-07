use std::collections::HashMap;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, QueryBuilder, Sqlite, SqlitePool};
use thiserror::Error;
use ts_rs::TS;
use uuid::Uuid;

/// Maximum length for auto-generated workspace names (derived from first user prompt)
const WORKSPACE_NAME_MAX_LEN: usize = 60;

use super::{
    project::Project,
    task::Task,
    workspace_repo::{RepoWithTargetBranch, WorkspaceRepo},
};

#[derive(Debug, Error)]
pub enum WorkspaceError {
    #[error(transparent)]
    Database(#[from] sqlx::Error),
    #[error("Task not found")]
    TaskNotFound,
    #[error("Project not found")]
    ProjectNotFound,
    #[error("Validation error: {0}")]
    ValidationError(String),
    #[error("Branch not found: {0}")]
    BranchNotFound(String),
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ContainerInfo {
    pub workspace_id: Uuid,
    pub task_id: Uuid,
    pub project_id: Uuid,
}

#[derive(Debug, FromRow)]
struct WorkspacePromptRow {
    workspace_id: Uuid,
    prompt: String,
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct Workspace {
    pub id: Uuid,
    pub task_id: Uuid,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub container_ref: Option<String>,
    pub branch: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent_working_dir: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub setup_completed_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub archived: bool,
    pub pinned: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceWithStatus {
    #[serde(flatten)]
    #[ts(flatten)]
    pub workspace: Workspace,
    pub is_running: bool,
    pub is_errored: bool,
}

impl std::ops::Deref for WorkspaceWithStatus {
    type Target = Workspace;
    fn deref(&self) -> &Self::Target {
        &self.workspace
    }
}

/// GitHub PR creation parameters
pub struct CreatePrParams<'a> {
    pub workspace_id: Uuid,
    pub task_id: Uuid,
    pub project_id: Uuid,
    pub github_token: &'a str,
    pub title: &'a str,
    pub body: Option<&'a str>,
    pub base_branch: Option<&'a str>,
}

#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateFollowUpAttempt {
    pub prompt: String,
}

/// Context data for resume operations (simplified)
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AttemptResumeContext {
    pub execution_history: String,
    pub cumulative_diffs: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceContext {
    pub workspace: Workspace,
    pub task: Task,
    pub project: Project,
    pub workspace_repos: Vec<RepoWithTargetBranch>,
}

#[derive(Debug, Deserialize, TS)]
pub struct CreateWorkspace {
    pub branch: String,
    pub agent_working_dir: Option<String>,
}

impl Workspace {
    pub async fn parent_task(&self, pool: &SqlitePool) -> Result<Option<Task>, sqlx::Error> {
        Task::find_by_id(pool, self.task_id).await
    }

    /// Fetch all workspaces, optionally filtered by task_id. Newest first.
    pub async fn fetch_all(
        pool: &SqlitePool,
        task_id: Option<Uuid>,
    ) -> Result<Vec<Self>, WorkspaceError> {
        let workspaces = match task_id {
            Some(tid) => sqlx::query_as!(
                Workspace,
                r#"SELECT id AS "id!: Uuid",
                              task_id AS "task_id!: Uuid",
                              container_ref,
                              branch,
                              agent_working_dir,
                              setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                              created_at AS "created_at!: DateTime<Utc>",
                              updated_at AS "updated_at!: DateTime<Utc>",
                              archived AS "archived!: bool",
                              pinned AS "pinned!: bool",
                              name
                       FROM workspaces
                       WHERE task_id = $1
                       ORDER BY created_at DESC"#,
                tid
            )
            .fetch_all(pool)
            .await
            .map_err(WorkspaceError::Database)?,
            None => sqlx::query_as!(
                Workspace,
                r#"SELECT id AS "id!: Uuid",
                              task_id AS "task_id!: Uuid",
                              container_ref,
                              branch,
                              agent_working_dir,
                              setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                              created_at AS "created_at!: DateTime<Utc>",
                              updated_at AS "updated_at!: DateTime<Utc>",
                              archived AS "archived!: bool",
                              pinned AS "pinned!: bool",
                              name
                       FROM workspaces
                       ORDER BY created_at DESC"#
            )
            .fetch_all(pool)
            .await
            .map_err(WorkspaceError::Database)?,
        };

        Ok(workspaces)
    }

    /// Load workspace with full validation - ensures workspace belongs to task and task belongs to project
    pub async fn load_context(
        pool: &SqlitePool,
        workspace_id: Uuid,
        task_id: Uuid,
        project_id: Uuid,
    ) -> Result<WorkspaceContext, WorkspaceError> {
        let workspace = sqlx::query_as!(
            Workspace,
            r#"SELECT  w.id                AS "id!: Uuid",
                       w.task_id           AS "task_id!: Uuid",
                       w.container_ref,
                       w.branch,
                       w.agent_working_dir,
                       w.setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                       w.created_at        AS "created_at!: DateTime<Utc>",
                       w.updated_at        AS "updated_at!: DateTime<Utc>",
                       w.archived          AS "archived!: bool",
                       w.pinned            AS "pinned!: bool",
                       w.name
               FROM    workspaces w
               JOIN    tasks t ON w.task_id = t.id
               JOIN    projects p ON t.project_id = p.id
               WHERE   w.id = $1 AND t.id = $2 AND p.id = $3"#,
            workspace_id,
            task_id,
            project_id
        )
        .fetch_optional(pool)
        .await?
        .ok_or(WorkspaceError::TaskNotFound)?;

        // Load task and project (we know they exist due to JOIN validation)
        let task = Task::find_by_id(pool, task_id)
            .await?
            .ok_or(WorkspaceError::TaskNotFound)?;

        let project = Project::find_by_id(pool, project_id)
            .await?
            .ok_or(WorkspaceError::ProjectNotFound)?;

        let workspace_repos =
            WorkspaceRepo::find_repos_with_target_branch_for_workspace(pool, workspace_id).await?;

        Ok(WorkspaceContext {
            workspace,
            task,
            project,
            workspace_repos,
        })
    }

    /// Update container reference
    pub async fn update_container_ref(
        pool: &SqlitePool,
        workspace_id: Uuid,
        container_ref: &str,
    ) -> Result<(), sqlx::Error> {
        let now = Utc::now();
        sqlx::query!(
            "UPDATE workspaces SET container_ref = $1, updated_at = $2 WHERE id = $3",
            container_ref,
            now,
            workspace_id
        )
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn clear_container_ref(
        pool: &SqlitePool,
        workspace_id: Uuid,
    ) -> Result<(), sqlx::Error> {
        sqlx::query!(
            "UPDATE workspaces SET container_ref = NULL, updated_at = datetime('now') WHERE id = ?",
            workspace_id
        )
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update the workspace's updated_at timestamp to prevent cleanup.
    /// Call this when the workspace is accessed (e.g., opened in editor).
    pub async fn touch(pool: &SqlitePool, workspace_id: Uuid) -> Result<(), sqlx::Error> {
        sqlx::query!(
            "UPDATE workspaces SET updated_at = datetime('now', 'subsec') WHERE id = ?",
            workspace_id
        )
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(pool: &SqlitePool, id: Uuid) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Workspace,
            r#"SELECT  id                AS "id!: Uuid",
                       task_id           AS "task_id!: Uuid",
                       container_ref,
                       branch,
                       agent_working_dir,
                       setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                       created_at        AS "created_at!: DateTime<Utc>",
                       updated_at        AS "updated_at!: DateTime<Utc>",
                       archived          AS "archived!: bool",
                       pinned            AS "pinned!: bool",
                       name
               FROM    workspaces
               WHERE   id = $1"#,
            id
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn find_by_rowid(pool: &SqlitePool, rowid: i64) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Workspace,
            r#"SELECT  id                AS "id!: Uuid",
                       task_id           AS "task_id!: Uuid",
                       container_ref,
                       branch,
                       agent_working_dir,
                       setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                       created_at        AS "created_at!: DateTime<Utc>",
                       updated_at        AS "updated_at!: DateTime<Utc>",
                       archived          AS "archived!: bool",
                       pinned            AS "pinned!: bool",
                       name
               FROM    workspaces
               WHERE   rowid = $1"#,
            rowid
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn container_ref_exists(
        pool: &SqlitePool,
        container_ref: &str,
    ) -> Result<bool, sqlx::Error> {
        let result = sqlx::query!(
            r#"SELECT EXISTS(SELECT 1 FROM workspaces WHERE container_ref = ?) as "exists!: bool""#,
            container_ref
        )
        .fetch_one(pool)
        .await?;

        Ok(result.exists)
    }

    /// Find workspaces that are expired and eligible for cleanup.
    /// Uses accelerated cleanup (1 hour) for archived workspaces OR tasks not in progress/review.
    /// Uses standard cleanup (72 hours) only for non-archived workspaces on active tasks.
    pub async fn find_expired_for_cleanup(
        pool: &SqlitePool,
    ) -> Result<Vec<Workspace>, sqlx::Error> {
        sqlx::query(
            r"
            SELECT
                w.id,
                w.task_id,
                w.container_ref,
                w.branch,
                w.agent_working_dir,
                w.setup_completed_at,
                w.created_at,
                w.updated_at,
                w.archived,
                w.pinned,
                w.name
            FROM workspaces w
            JOIN tasks t ON w.task_id = t.id
            LEFT JOIN sessions s ON w.id = s.workspace_id
            LEFT JOIN execution_processes ep ON s.id = ep.session_id AND ep.completed_at IS NOT NULL
            WHERE w.container_ref IS NOT NULL
                -- NOTE(E38-12): `SELECT DISTINCT s2.workspace_id` is not
                -- backed by a dedicated covering index. If this query shows up
                -- in slow-query traces, add an index on
                -- sessions(workspace_id) combined with
                -- execution_processes(session_id, completed_at).
                AND w.id NOT IN (
                    SELECT DISTINCT s2.workspace_id
                    FROM sessions s2
                    JOIN execution_processes ep2 ON s2.id = ep2.session_id
                    WHERE ep2.completed_at IS NULL
                )
            GROUP BY w.id, w.container_ref, w.updated_at
            HAVING datetime('now',
                CASE
                    WHEN w.archived = 1 OR t.status NOT IN ('inprogress', 'inreview')
                    THEN '-1 hours'
                    ELSE '-72 hours'
                END
            ) > datetime(
                MAX(
                    COALESCE(
                        datetime(ep.completed_at),
                        datetime(w.updated_at)
                    )
                )
            )
            ORDER BY MAX(
                CASE
                    WHEN ep.completed_at IS NOT NULL THEN ep.completed_at
                    ELSE w.updated_at
                END
            ) ASC
            ",
        )
        .try_map(|row: sqlx::sqlite::SqliteRow| {
            use sqlx::Row;

            let branch: String = row.try_get("branch")?;
            Ok(Workspace {
                id: row.try_get("id")?,
                task_id: row.try_get("task_id")?,
                container_ref: row.try_get("container_ref")?,
                branch,
                agent_working_dir: row.try_get("agent_working_dir")?,
                setup_completed_at: row.try_get("setup_completed_at")?,
                created_at: row.try_get("created_at")?,
                updated_at: row.try_get("updated_at")?,
                archived: row.try_get("archived")?,
                pinned: row.try_get("pinned")?,
                name: row.try_get("name")?,
            })
        })
        .fetch_all(pool)
        .await
    }

    pub async fn create(
        pool: &SqlitePool,
        data: &CreateWorkspace,
        id: Uuid,
        task_id: Uuid,
    ) -> Result<Self, WorkspaceError> {
        Ok(sqlx::query_as!(
            Workspace,
            r#"INSERT INTO workspaces (id, task_id, container_ref, branch, agent_working_dir, setup_completed_at)
               VALUES ($1, $2, $3, $4, $5, $6)
               RETURNING id as "id!: Uuid", task_id as "task_id!: Uuid", container_ref, branch, agent_working_dir, setup_completed_at as "setup_completed_at: DateTime<Utc>", created_at as "created_at!: DateTime<Utc>", updated_at as "updated_at!: DateTime<Utc>", archived as "archived!: bool", pinned as "pinned!: bool", name"#,
            id,
            task_id,
            Option::<String>::None,
            data.branch,
            data.agent_working_dir,
            Option::<DateTime<Utc>>::None
        )
        .fetch_one(pool)
        .await?)
    }

    pub async fn update_branch_name(
        pool: &SqlitePool,
        workspace_id: Uuid,
        new_branch_name: &str,
    ) -> Result<(), WorkspaceError> {
        sqlx::query!(
            "UPDATE workspaces SET branch = $1, updated_at = datetime('now') WHERE id = $2",
            new_branch_name,
            workspace_id,
        )
        .execute(pool)
        .await?;

        Ok(())
    }

    pub async fn resolve_container_ref(
        pool: &SqlitePool,
        container_ref: &str,
    ) -> Result<ContainerInfo, sqlx::Error> {
        let result = sqlx::query!(
            r#"SELECT w.id as "workspace_id!: Uuid",
                      w.task_id as "task_id!: Uuid",
                      t.project_id as "project_id!: Uuid"
               FROM workspaces w
               JOIN tasks t ON w.task_id = t.id
               WHERE w.container_ref = ?"#,
            container_ref
        )
        .fetch_optional(pool)
        .await?
        .ok_or(sqlx::Error::RowNotFound)?;

        Ok(ContainerInfo {
            workspace_id: result.workspace_id,
            task_id: result.task_id,
            project_id: result.project_id,
        })
    }

    /// Find workspace by path, also trying the parent directory.
    /// Used by VSCode extension which may open a repo subfolder (single-repo case)
    /// rather than the workspace root directory (multi-repo case).
    pub async fn resolve_container_ref_by_prefix(
        pool: &SqlitePool,
        path: &str,
    ) -> Result<ContainerInfo, sqlx::Error> {
        // First try exact match
        if let Ok(info) = Self::resolve_container_ref(pool, path).await {
            return Ok(info);
        }

        if let Some(parent) = std::path::Path::new(path).parent()
            && let Some(parent_str) = parent.to_str()
            && let Ok(info) = Self::resolve_container_ref(pool, parent_str).await
        {
            return Ok(info);
        }

        Err(sqlx::Error::RowNotFound)
    }

    pub async fn set_archived(
        pool: &SqlitePool,
        workspace_id: Uuid,
        archived: bool,
    ) -> Result<(), sqlx::Error> {
        sqlx::query!(
            "UPDATE workspaces SET archived = $1, updated_at = datetime('now', 'subsec') WHERE id = $2",
            archived,
            workspace_id
        )
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update workspace fields. Only non-None values will be updated.
    /// For `name`, pass `Some("")` to clear the name, `Some("foo")` to set it, or `None` to leave unchanged.
    pub async fn update(
        pool: &SqlitePool,
        workspace_id: Uuid,
        archived: Option<bool>,
        pinned: Option<bool>,
        name: Option<&str>,
    ) -> Result<(), sqlx::Error> {
        // Convert empty string to None for name field (to store as NULL)
        let name_value = name.filter(|s| !s.is_empty());
        let name_provided = name.is_some();

        sqlx::query!(
            r#"UPDATE workspaces SET
                archived = COALESCE($1, archived),
                pinned = COALESCE($2, pinned),
                name = CASE WHEN $3 THEN $4 ELSE name END,
                updated_at = datetime('now', 'subsec')
            WHERE id = $5"#,
            archived,
            pinned,
            name_provided,
            name_value,
            workspace_id
        )
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn get_first_user_message(
        pool: &SqlitePool,
        workspace_id: Uuid,
    ) -> Result<Option<String>, sqlx::Error> {
        let result = sqlx::query!(
            r#"SELECT cat.prompt
               FROM sessions s
               JOIN execution_processes ep ON ep.session_id = s.id
               JOIN coding_agent_turns cat ON cat.execution_process_id = ep.id
               WHERE s.workspace_id = $1
                 AND s.executor IS NOT NULL
                 AND cat.prompt IS NOT NULL
               ORDER BY s.created_at ASC, ep.created_at ASC
               LIMIT 1"#,
            workspace_id
        )
        .fetch_optional(pool)
        .await?;
        Ok(result.and_then(|r| r.prompt))
    }

    async fn list_generated_names(
        pool: &SqlitePool,
        workspace_ids: &[Uuid],
    ) -> Result<Vec<(Uuid, String)>, sqlx::Error> {
        if workspace_ids.is_empty() {
            return Ok(Vec::new());
        }

        let mut query_builder = QueryBuilder::<Sqlite>::new(
            r"SELECT workspace_id, prompt
               FROM (
                   SELECT
                       s.workspace_id AS workspace_id,
                       cat.prompt AS prompt,
                       ROW_NUMBER() OVER (
                           PARTITION BY s.workspace_id
                           ORDER BY s.created_at ASC, ep.created_at ASC, ep.id ASC
                       ) AS row_num
                   FROM sessions s
                   JOIN execution_processes ep ON ep.session_id = s.id
                   JOIN coding_agent_turns cat ON cat.execution_process_id = ep.id
                   WHERE s.executor IS NOT NULL
                     AND cat.prompt IS NOT NULL
                     AND s.workspace_id IN (",
        );

        let mut separated = query_builder.separated(", ");
        for workspace_id in workspace_ids {
            separated.push_bind(workspace_id);
        }
        query_builder.push(
            r")
               ) ranked
               WHERE row_num = 1",
        );

        let rows: Vec<WorkspacePromptRow> = query_builder.build_query_as().fetch_all(pool).await?;
        Ok(rows
            .into_iter()
            .filter_map(|row| {
                let name = Self::truncate_to_name(&row.prompt, WORKSPACE_NAME_MAX_LEN);
                (!name.is_empty()).then_some((row.workspace_id, name))
            })
            .collect())
    }

    async fn persist_generated_names(
        pool: &SqlitePool,
        generated_names: &[(Uuid, String)],
    ) -> Result<(), sqlx::Error> {
        if generated_names.is_empty() {
            return Ok(());
        }

        let mut query_builder =
            QueryBuilder::<Sqlite>::new("WITH generated_names(workspace_id, name) AS (");
        query_builder.push_values(generated_names.iter(), |mut builder, (workspace_id, name)| {
            builder.push_bind(*workspace_id).push_bind(name.as_str());
        });
        query_builder.push(
            r")
            UPDATE workspaces
            SET name = (
                    SELECT generated_names.name
                    FROM generated_names
                    WHERE generated_names.workspace_id = workspaces.id
                ),
                updated_at = datetime('now', 'subsec')
            WHERE id IN (SELECT workspace_id FROM generated_names)",
        );

        query_builder.build().execute(pool).await?;
        Ok(())
    }

    pub fn truncate_to_name(prompt: &str, max_len: usize) -> String {
        let trimmed = prompt.trim();
        if trimmed.chars().count() <= max_len {
            trimmed.to_string()
        } else {
            let truncated: String = trimmed.chars().take(max_len).collect();
            if let Some(last_space) = truncated.rfind(' ') {
                format!("{}...", &truncated[..last_space])
            } else {
                format!("{truncated}...")
            }
        }
    }

    pub async fn find_all_with_status(
        pool: &SqlitePool,
        archived: Option<bool>,
        limit: Option<i64>,
    ) -> Result<Vec<WorkspaceWithStatus>, sqlx::Error> {
        // Build archived filter: NULL means no filter, otherwise match exact value.
        // SQLite stores booleans as 0/1, so we pass an i32.
        let archived_filter: Option<i32> = archived.map(i32::from);

        let records = sqlx::query!(
            r#"SELECT
                w.id AS "id!: Uuid",
                w.task_id AS "task_id!: Uuid",
                w.container_ref,
                w.branch,
                w.agent_working_dir,
                w.setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                w.created_at AS "created_at!: DateTime<Utc>",
                w.updated_at AS "updated_at!: DateTime<Utc>",
                w.archived AS "archived!: bool",
                w.pinned AS "pinned!: bool",
                w.name,

                CASE WHEN EXISTS (
                    SELECT 1
                    FROM sessions s
                    JOIN execution_processes ep ON ep.session_id = s.id
                    WHERE s.workspace_id = w.id
                      AND ep.status = 'running'
                      AND ep.dropped = FALSE
                      AND ep.run_reason IN ('setupscript','cleanupscript','codingagent')
                    LIMIT 1
                ) THEN 1 ELSE 0 END AS "is_running!: i64",

                CASE WHEN (
                    SELECT ep.status
                    FROM sessions s
                    JOIN execution_processes ep ON ep.session_id = s.id
                    WHERE s.workspace_id = w.id
                      AND ep.dropped = FALSE
                      AND ep.run_reason IN ('setupscript','cleanupscript','codingagent')
                    ORDER BY ep.created_at DESC
                    LIMIT 1
                ) IN ('failed','killed') THEN 1 ELSE 0 END AS "is_errored!: i64"

            FROM workspaces w
            WHERE (?1 IS NULL OR w.archived = ?1)
            ORDER BY w.updated_at DESC"#,
            archived_filter,
        )
        .fetch_all(pool)
        .await?;

        let mut workspaces: Vec<WorkspaceWithStatus> = records
            .into_iter()
            .map(|rec| WorkspaceWithStatus {
                workspace: Workspace {
                    id: rec.id,
                    task_id: rec.task_id,
                    container_ref: rec.container_ref,
                    branch: rec.branch,
                    agent_working_dir: rec.agent_working_dir,
                    setup_completed_at: rec.setup_completed_at,
                    created_at: rec.created_at,
                    updated_at: rec.updated_at,
                    archived: rec.archived,
                    pinned: rec.pinned,
                    name: rec.name,
                },
                is_running: rec.is_running != 0,
                is_errored: rec.is_errored != 0,
            })
            .collect();

        // Apply limit if provided (already sorted by updated_at DESC from query)
        if let Some(lim) = limit {
            if let Ok(limit) = usize::try_from(lim) {
                workspaces.truncate(limit);
            } else {
                workspaces.clear();
            }
        }

        let unnamed_ids: Vec<Uuid> = workspaces
            .iter()
            .filter(|ws| ws.workspace.name.is_none())
            .map(|ws| ws.workspace.id)
            .collect();
        let generated_names = Self::list_generated_names(pool, &unnamed_ids).await?;
        Self::persist_generated_names(pool, &generated_names).await?;

        let generated_names: HashMap<Uuid, String> = generated_names.into_iter().collect();
        for ws in &mut workspaces {
            if let Some(name) = generated_names.get(&ws.workspace.id) {
                ws.workspace.name = Some(name.clone());
            }
        }

        Ok(workspaces)
    }

    /// Delete a workspace by ID
    pub async fn delete(pool: &SqlitePool, id: Uuid) -> Result<u64, sqlx::Error> {
        let result = sqlx::query!("DELETE FROM workspaces WHERE id = $1", id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }

    /// Count total workspaces across all projects
    pub async fn count_all(pool: &SqlitePool) -> Result<i64, WorkspaceError> {
        sqlx::query_scalar!(r#"SELECT COUNT(*) as "count!: i64" FROM workspaces"#)
            .fetch_one(pool)
            .await
            .map_err(WorkspaceError::Database)
    }

    pub async fn find_by_id_with_status(
        pool: &SqlitePool,
        id: Uuid,
    ) -> Result<Option<WorkspaceWithStatus>, sqlx::Error> {
        let rec = sqlx::query!(
            r#"SELECT
                w.id AS "id!: Uuid",
                w.task_id AS "task_id!: Uuid",
                w.container_ref,
                w.branch,
                w.agent_working_dir,
                w.setup_completed_at AS "setup_completed_at: DateTime<Utc>",
                w.created_at AS "created_at!: DateTime<Utc>",
                w.updated_at AS "updated_at!: DateTime<Utc>",
                w.archived AS "archived!: bool",
                w.pinned AS "pinned!: bool",
                w.name,

                CASE WHEN EXISTS (
                    SELECT 1
                    FROM sessions s
                    JOIN execution_processes ep ON ep.session_id = s.id
                    WHERE s.workspace_id = w.id
                      AND ep.status = 'running'
                      AND ep.dropped = FALSE
                      AND ep.run_reason IN ('setupscript','cleanupscript','codingagent')
                    LIMIT 1
                ) THEN 1 ELSE 0 END AS "is_running!: i64",

                CASE WHEN (
                    SELECT ep.status
                    FROM sessions s
                    JOIN execution_processes ep ON ep.session_id = s.id
                    WHERE s.workspace_id = w.id
                      AND ep.dropped = FALSE
                      AND ep.run_reason IN ('setupscript','cleanupscript','codingagent')
                    ORDER BY ep.created_at DESC
                    LIMIT 1
                ) IN ('failed','killed') THEN 1 ELSE 0 END AS "is_errored!: i64"

            FROM workspaces w
            WHERE w.id = $1"#,
            id
        )
        .fetch_optional(pool)
        .await?;

        let Some(rec) = rec else {
            return Ok(None);
        };

        let mut ws = WorkspaceWithStatus {
            workspace: Workspace {
                id: rec.id,
                task_id: rec.task_id,
                container_ref: rec.container_ref,
                branch: rec.branch,
                agent_working_dir: rec.agent_working_dir,
                setup_completed_at: rec.setup_completed_at,
                created_at: rec.created_at,
                updated_at: rec.updated_at,
                archived: rec.archived,
                pinned: rec.pinned,
                name: rec.name,
            },
            is_running: rec.is_running != 0,
            is_errored: rec.is_errored != 0,
        };

        if ws.workspace.name.is_none()
            && let Some(prompt) = Self::get_first_user_message(pool, ws.workspace.id).await?
        {
            let name = Self::truncate_to_name(&prompt, WORKSPACE_NAME_MAX_LEN);
            Self::update(pool, ws.workspace.id, None, None, Some(&name)).await?;
            ws.workspace.name = Some(name);
        }

        Ok(Some(ws))
    }
}

#[cfg(test)]
mod tests {
    use sqlx::SqlitePool;

    use super::*;
    use crate::{
        models::{
            project::{CreateProject, Project},
            session::{CreateSession, Session},
            task::{CreateTask, Task, TaskStatus},
        },
        run_migrations,
    };

    async fn setup_pool() -> SqlitePool {
        let pool = SqlitePool::connect("sqlite::memory:").await.unwrap();
        run_migrations(&pool).await.unwrap();
        pool
    }

    async fn create_workspace_fixture(
        pool: &SqlitePool,
        project_id: Uuid,
        title: &str,
    ) -> (Uuid, Uuid, Uuid, Uuid) {
        let task_id = Uuid::new_v4();
        let workspace_id = Uuid::new_v4();
        let session_id = Uuid::new_v4();

        Task::create(
            pool,
            &CreateTask {
                project_id,
                title: title.to_string(),
                description: None,
                status: Some(TaskStatus::Todo),
                parent_workspace_id: None,
                image_ids: None,
                shared_task_id: None,
            },
            task_id,
        )
        .await
        .unwrap();

        Workspace::create(
            pool,
            &CreateWorkspace {
                branch: format!("branch-{title}"),
                agent_working_dir: None,
            },
            workspace_id,
            task_id,
        )
        .await
        .unwrap();

        Session::create(
            pool,
            &CreateSession {
                executor: Some("test-executor".to_string()),
                model_config_id: None,
            },
            session_id,
            workspace_id,
        )
        .await
        .unwrap();

        (task_id, workspace_id, session_id, project_id)
    }

    async fn insert_process(
        pool: &SqlitePool,
        session_id: Uuid,
        status: &str,
        dropped: bool,
        created_at: &str,
    ) {
        sqlx::query(
            "INSERT INTO execution_processes (id, session_id, run_reason, status, dropped, created_at) VALUES (?, ?, 'codingagent', ?, ?, ?)",
        )
        .bind(Uuid::new_v4())
        .bind(session_id)
        .bind(status)
        .bind(dropped)
        .bind(created_at)
        .execute(pool)
        .await
        .unwrap();
    }

    #[tokio::test]
    async fn workspace_status_queries_exclude_dropped_processes() {
        let pool = setup_pool().await;

        let project_id = Uuid::new_v4();
        Project::create(
            &pool,
            &CreateProject {
                name: "project-for-workspace-status".to_string(),
                repositories: vec![],
            },
            project_id,
        )
        .await
        .unwrap();

        let (_, dropped_workspace_id, dropped_session_id, _) =
            create_workspace_fixture(&pool, project_id, "dropped-only").await;
        let (_, active_workspace_id, active_session_id, _) =
            create_workspace_fixture(&pool, project_id, "active").await;

        insert_process(
            &pool,
            dropped_session_id,
            "completed",
            false,
            "2026-01-02 00:00:00.000",
        )
        .await;
        insert_process(
            &pool,
            dropped_session_id,
            "failed",
            true,
            "2026-01-02 00:00:01.000",
        )
        .await;
        insert_process(
            &pool,
            dropped_session_id,
            "running",
            true,
            "2026-01-02 00:00:02.000",
        )
        .await;

        insert_process(
            &pool,
            active_session_id,
            "running",
            false,
            "2026-01-02 00:00:03.000",
        )
        .await;

        let all = Workspace::find_all_with_status(&pool, None, None)
            .await
            .unwrap();
        let dropped_workspace = all
            .iter()
            .find(|entry| entry.workspace.id == dropped_workspace_id)
            .unwrap();
        assert!(!dropped_workspace.is_running);
        assert!(!dropped_workspace.is_errored);

        let active_workspace = all
            .iter()
            .find(|entry| entry.workspace.id == active_workspace_id)
            .unwrap();
        assert!(active_workspace.is_running);
        assert!(!active_workspace.is_errored);

        let single = Workspace::find_by_id_with_status(&pool, dropped_workspace_id)
            .await
            .unwrap()
            .unwrap();
        assert!(!single.is_running);
        assert!(!single.is_errored);
    }
}
