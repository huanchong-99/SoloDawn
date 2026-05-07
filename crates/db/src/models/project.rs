use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{Executor, FromRow, Sqlite, SqlitePool};
use thiserror::Error;
use ts_rs::TS;
use uuid::Uuid;

use super::project_repo::CreateProjectRepo;

#[derive(Debug, Error)]
pub enum ProjectError {
    #[error(transparent)]
    Database(#[from] sqlx::Error),
    #[error("Project not found")]
    ProjectNotFound,
    #[error("Failed to create project: {0}")]
    CreateFailed(String),
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct Project {
    pub id: Uuid,
    pub name: String,
    pub default_agent_working_dir: Option<String>,
    pub remote_project_id: Option<Uuid>,
    #[ts(type = "Date")]
    pub created_at: DateTime<Utc>,
    #[ts(type = "Date")]
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateProject {
    pub name: String,
    pub repositories: Vec<CreateProjectRepo>,
}

#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct UpdateProject {
    pub name: Option<String>,
    pub default_agent_working_dir: Option<String>,
}

#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct SearchResult {
    pub path: String,
    pub is_file: bool,
    pub match_type: SearchMatchType,
    /// Ranking score based on git history (higher = more recently/frequently edited)
    #[serde(default)]
    pub score: i64,
}

#[derive(Debug, Clone, Serialize, TS)]
pub enum SearchMatchType {
    FileName,
    DirectoryName,
    FullPath,
}

impl Project {
    pub async fn count(pool: &SqlitePool) -> Result<i64, sqlx::Error> {
        sqlx::query_scalar!(r#"SELECT COUNT(*) as "count!: i64" FROM projects"#)
            .fetch_one(pool)
            .await
    }

    /// Hard cap used by `find_all` (W2-15-07).
    pub const FIND_ALL_MAX_ROWS: i64 = 1000;

    /// Return projects in `created_at DESC` order, capped at
    /// [`Self::FIND_ALL_MAX_ROWS`]. Typical tenants have well under this;
    /// use `find_page` if you genuinely need to paginate.
    pub async fn find_all(pool: &SqlitePool) -> Result<Vec<Self>, sqlx::Error> {
        let limit: i64 = Self::FIND_ALL_MAX_ROWS;
        sqlx::query_as::<_, Project>(
            r"SELECT id,
                      name,
                      default_agent_working_dir,
                      remote_project_id,
                      created_at,
                      updated_at
               FROM projects
               ORDER BY created_at DESC
               LIMIT ?",
        )
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    /// Keyset-friendly paginated variant of `find_all`. Pass `after`
    /// as the last-seen `(created_at, id)` tuple to walk older pages.
    pub async fn find_page(
        pool: &SqlitePool,
        after: Option<(DateTime<Utc>, Uuid)>,
        limit: i64,
    ) -> Result<Vec<Self>, sqlx::Error> {
        match after {
            Some((ts, id)) => {
                sqlx::query_as::<_, Project>(
                    r"SELECT id,
                              name,
                              default_agent_working_dir,
                              remote_project_id,
                              created_at,
                              updated_at
                       FROM projects
                       WHERE (created_at, id) < (?, ?)
                       ORDER BY created_at DESC, id DESC
                       LIMIT ?",
                )
                .bind(ts)
                .bind(id)
                .bind(limit)
                .fetch_all(pool)
                .await
            }
            None => {
                sqlx::query_as::<_, Project>(
                    r"SELECT id,
                              name,
                              default_agent_working_dir,
                              remote_project_id,
                              created_at,
                              updated_at
                       FROM projects
                       ORDER BY created_at DESC, id DESC
                       LIMIT ?",
                )
                .bind(limit)
                .fetch_all(pool)
                .await
            }
        }
    }

    /// Find the most actively used projects based on recent task activity
    pub async fn find_most_active(pool: &SqlitePool, limit: i32) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as::<_, Project>(
            "
            SELECT p.id, p.name,
                   p.default_agent_working_dir,
                   p.remote_project_id,
                   p.created_at, p.updated_at
            FROM projects p
            INNER JOIN (
                SELECT t.project_id, MAX(w.updated_at) as last_activity
                FROM tasks t
                INNER JOIN workspaces w ON w.task_id = t.id
                GROUP BY t.project_id
            ) recent ON p.id = recent.project_id
            ORDER BY recent.last_activity DESC
            LIMIT $1
            ",
        )
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    pub async fn find_by_id(pool: &SqlitePool, id: Uuid) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Project,
            r#"SELECT id as "id!: Uuid",
                      name,
                      default_agent_working_dir,
                      remote_project_id as "remote_project_id: Uuid",
                      created_at as "created_at!: DateTime<Utc>",
                      updated_at as "updated_at!: DateTime<Utc>"
               FROM projects
               WHERE id = $1"#,
            id
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn find_by_rowid(pool: &SqlitePool, rowid: i64) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Project,
            r#"SELECT id as "id!: Uuid",
                      name,
                      default_agent_working_dir,
                      remote_project_id as "remote_project_id: Uuid",
                      created_at as "created_at!: DateTime<Utc>",
                      updated_at as "updated_at!: DateTime<Utc>"
               FROM projects
               WHERE rowid = $1"#,
            rowid
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn find_by_remote_project_id(
        pool: &SqlitePool,
        remote_project_id: Uuid,
    ) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Project,
            r#"SELECT id as "id!: Uuid",
                      name,
                      default_agent_working_dir,
                      remote_project_id as "remote_project_id: Uuid",
                      created_at as "created_at!: DateTime<Utc>",
                      updated_at as "updated_at!: DateTime<Utc>"
               FROM projects
               WHERE remote_project_id = $1
               LIMIT 1"#,
            remote_project_id
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn create(
        executor: impl Executor<'_, Database = Sqlite>,
        data: &CreateProject,
        project_id: Uuid,
    ) -> Result<Self, sqlx::Error> {
        sqlx::query_as!(
            Project,
            r#"INSERT INTO projects (
                    id,
                    name
                ) VALUES (
                    $1, $2
                )
                RETURNING id as "id!: Uuid",
                          name,
                          default_agent_working_dir,
                          remote_project_id as "remote_project_id: Uuid",
                          created_at as "created_at!: DateTime<Utc>",
                          updated_at as "updated_at!: DateTime<Utc>""#,
            project_id,
            data.name,
        )
        .fetch_one(executor)
        .await
    }

    pub async fn update(
        pool: &SqlitePool,
        id: Uuid,
        payload: &UpdateProject,
    ) -> Result<Self, sqlx::Error> {
        let existing = Self::find_by_id(pool, id)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let name = payload.name.clone().unwrap_or(existing.name);
        let default_agent_working_dir = payload
            .default_agent_working_dir
            .clone()
            .or(existing.default_agent_working_dir);

        sqlx::query_as!(
            Project,
            r#"UPDATE projects
               SET name = $2,
                   default_agent_working_dir = $3
               WHERE id = $1
               RETURNING id as "id!: Uuid",
                         name,
                         default_agent_working_dir,
                         remote_project_id as "remote_project_id: Uuid",
                         created_at as "created_at!: DateTime<Utc>",
                         updated_at as "updated_at!: DateTime<Utc>""#,
            id,
            name,
            default_agent_working_dir,
        )
        .fetch_one(pool)
        .await
    }

    pub async fn set_remote_project_id(
        pool: &SqlitePool,
        id: Uuid,
        remote_project_id: Option<Uuid>,
    ) -> Result<(), sqlx::Error> {
        sqlx::query!(
            r#"UPDATE projects
               SET remote_project_id = $2
               WHERE id = $1"#,
            id,
            remote_project_id
        )
        .execute(pool)
        .await?;

        Ok(())
    }

    /// Transaction-compatible version of set_remote_project_id
    pub async fn set_remote_project_id_tx<'e, E>(
        executor: E,
        id: Uuid,
        remote_project_id: Option<Uuid>,
    ) -> Result<(), sqlx::Error>
    where
        E: Executor<'e, Database = Sqlite>,
    {
        sqlx::query!(
            r#"UPDATE projects
               SET remote_project_id = $2
               WHERE id = $1"#,
            id,
            remote_project_id
        )
        .execute(executor)
        .await?;

        Ok(())
    }

    pub async fn delete(pool: &SqlitePool, id: Uuid) -> Result<u64, sqlx::Error> {
        let result = sqlx::query!("DELETE FROM projects WHERE id = $1", id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }
}
