use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, QueryBuilder, Sqlite, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct Image {
    pub id: Uuid,
    pub file_path: String, // relative path within cache/images/
    pub original_name: String,
    pub mime_type: Option<String>,
    pub size_bytes: i64,
    pub hash: String, // SHA256 hash for deduplication
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateImage {
    pub file_path: String,
    pub original_name: String,
    pub mime_type: Option<String>,
    pub size_bytes: i64,
    pub hash: String,
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct TaskImage {
    pub id: Uuid,
    pub task_id: Uuid,
    pub image_id: Uuid,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct CreateTaskImage {
    pub task_id: Uuid,
    pub image_id: Uuid,
}

impl Image {
    pub async fn create(pool: &SqlitePool, data: &CreateImage) -> Result<Self, sqlx::Error> {
        let id = Uuid::new_v4();
        sqlx::query_as!(
            Image,
            r#"INSERT INTO images (id, file_path, original_name, mime_type, size_bytes, hash)
               VALUES ($1, $2, $3, $4, $5, $6)
               RETURNING id as "id!: Uuid", 
                         file_path as "file_path!", 
                         original_name as "original_name!", 
                         mime_type,
                         size_bytes as "size_bytes!",
                         hash as "hash!",
                         created_at as "created_at!: DateTime<Utc>", 
                         updated_at as "updated_at!: DateTime<Utc>""#,
            id,
            data.file_path,
            data.original_name,
            data.mime_type,
            data.size_bytes,
            data.hash,
        )
        .fetch_one(pool)
        .await
    }

    pub async fn find_by_hash(pool: &SqlitePool, hash: &str) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Image,
            r#"SELECT id as "id!: Uuid",
                      file_path as "file_path!",
                      original_name as "original_name!",
                      mime_type,
                      size_bytes as "size_bytes!",
                      hash as "hash!",
                      created_at as "created_at!: DateTime<Utc>",
                      updated_at as "updated_at!: DateTime<Utc>"
               FROM images
               WHERE hash = $1"#,
            hash
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn find_by_id(pool: &SqlitePool, id: Uuid) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Image,
            r#"SELECT id as "id!: Uuid",
                      file_path as "file_path!",
                      original_name as "original_name!",
                      mime_type,
                      size_bytes as "size_bytes!",
                      hash as "hash!",
                      created_at as "created_at!: DateTime<Utc>",
                      updated_at as "updated_at!: DateTime<Utc>"
               FROM images
               WHERE id = $1"#,
            id
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn find_by_file_path(
        pool: &SqlitePool,
        file_path: &str,
    ) -> Result<Option<Self>, sqlx::Error> {
        sqlx::query_as!(
            Image,
            r#"SELECT id as "id!: Uuid",
                      file_path as "file_path!",
                      original_name as "original_name!",
                      mime_type,
                      size_bytes as "size_bytes!",
                      hash as "hash!",
                      created_at as "created_at!: DateTime<Utc>",
                      updated_at as "updated_at!: DateTime<Utc>"
               FROM images
               WHERE file_path = $1"#,
            file_path
        )
        .fetch_optional(pool)
        .await
    }

    pub async fn find_by_task_id(
        pool: &SqlitePool,
        task_id: Uuid,
    ) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            Image,
            r#"SELECT i.id as "id!: Uuid",
                      i.file_path as "file_path!",
                      i.original_name as "original_name!",
                      i.mime_type,
                      i.size_bytes as "size_bytes!",
                      i.hash as "hash!",
                      i.created_at as "created_at!: DateTime<Utc>",
                      i.updated_at as "updated_at!: DateTime<Utc>"
               FROM images i
               JOIN task_images ti ON i.id = ti.image_id
               WHERE ti.task_id = $1
               ORDER BY ti.created_at"#,
            task_id
        )
        .fetch_all(pool)
        .await
    }

    pub async fn delete(pool: &SqlitePool, id: Uuid) -> Result<(), sqlx::Error> {
        sqlx::query!(r#"DELETE FROM images WHERE id = $1"#, id)
            .execute(pool)
            .await?;
        Ok(())
    }

    pub async fn find_orphaned_images(pool: &SqlitePool) -> Result<Vec<Self>, sqlx::Error> {
        sqlx::query_as!(
            Image,
            r#"SELECT i.id as "id!: Uuid",
                      i.file_path as "file_path!",
                      i.original_name as "original_name!",
                      i.mime_type,
                      i.size_bytes as "size_bytes!",
                      i.hash as "hash!",
                      i.created_at as "created_at!: DateTime<Utc>",
                      i.updated_at as "updated_at!: DateTime<Utc>"
               FROM images i
               LEFT JOIN task_images ti ON i.id = ti.image_id
               WHERE ti.task_id IS NULL"#
        )
        .fetch_all(pool)
        .await
    }
}

impl TaskImage {
    /// Associate multiple images with a task, skipping duplicates.
    ///
    /// Uses a single multi-row INSERT with `ON CONFLICT DO NOTHING`, relying on
    /// the `UNIQUE(task_id, image_id)` constraint on `task_images` to dedup.
    // NOTE(W2-15-02): Previous N+1 (one INSERT + one SELECT-for-dedup per
    // image) is replaced by a single multi-row INSERT with
    // `ON CONFLICT DO NOTHING`. `SQLITE_MAX_VARIABLE_NUMBER` is 32766 in
    // sqlite 3.32+ (3 binds per image, so effective cap ≈ 10_000). We chunk
    // to `MAX_IMAGES_PER_BATCH = 500` to stay well below that and keep
    // statement compile time reasonable. Callers no longer need to chunk.
    pub async fn associate_many_dedup(
        pool: &SqlitePool,
        task_id: Uuid,
        image_ids: &[Uuid],
    ) -> Result<(), sqlx::Error> {
        const MAX_IMAGES_PER_BATCH: usize = 500;

        if image_ids.is_empty() {
            return Ok(());
        }

        for chunk in image_ids.chunks(MAX_IMAGES_PER_BATCH) {
            let mut qb: QueryBuilder<Sqlite> =
                QueryBuilder::new("INSERT INTO task_images (id, task_id, image_id) ");
            qb.push_values(chunk.iter(), |mut b, image_id| {
                b.push_bind(Uuid::new_v4())
                    .push_bind(task_id)
                    .push_bind(*image_id);
            });
            qb.push(" ON CONFLICT(task_id, image_id) DO NOTHING");
            qb.build().execute(pool).await?;
        }
        Ok(())
    }

    pub async fn delete_by_task_id(pool: &SqlitePool, task_id: Uuid) -> Result<(), sqlx::Error> {
        sqlx::query!(r#"DELETE FROM task_images WHERE task_id = $1"#, task_id)
            .execute(pool)
            .await?;
        Ok(())
    }

    /// Check if an image is associated with a specific task.
    pub async fn is_associated(
        pool: &SqlitePool,
        task_id: Uuid,
        image_id: Uuid,
    ) -> Result<bool, sqlx::Error> {
        let result = sqlx::query_scalar!(
            r#"SELECT EXISTS(
                SELECT 1
                FROM task_images
                WHERE task_id = $1 AND image_id = $2
               ) AS "exists!: bool"
            "#,
            task_id,
            image_id
        )
        .fetch_one(pool)
        .await?;
        Ok(result)
    }
}
