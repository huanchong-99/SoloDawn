//! Synced architecture knowledge documents. One row per markdown file pulled
//! from an [`ArchitectureSource`](super::architecture_source::ArchitectureSource);
//! `digest` holds the prompt-injectable extract, `content` the full document.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ArchitectureEntry {
    pub id: String,
    pub source_id: String,
    pub path: String,
    pub category: String,
    pub slug: String,
    pub title: String,
    /// JSON array of matching keywords extracted at sync time.
    pub keywords: String,
    pub digest: String,
    pub content: String,
    pub blob_sha: String,
    pub synced_at: DateTime<Utc>,
}

/// Listing projection without the heavyweight `content`/`digest` columns.
#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ArchitectureEntrySummary {
    pub id: String,
    pub source_id: String,
    pub path: String,
    pub category: String,
    pub slug: String,
    pub title: String,
    pub synced_at: DateTime<Utc>,
}

impl ArchitectureEntry {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        source_id: &str,
        path: &str,
        category: &str,
        slug: &str,
        title: &str,
        keywords: &[String],
        digest: &str,
        content: &str,
        blob_sha: &str,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            source_id: source_id.to_string(),
            path: path.to_string(),
            category: category.to_string(),
            slug: slug.to_string(),
            title: title.to_string(),
            keywords: serde_json::to_string(keywords).unwrap_or_else(|_| "[]".to_string()),
            digest: digest.to_string(),
            content: content.to_string(),
            blob_sha: blob_sha.to_string(),
            synced_at: Utc::now(),
        }
    }

    /// Parse the JSON `keywords` column; invalid JSON yields an empty list.
    pub fn keyword_list(&self) -> Vec<String> {
        serde_json::from_str(&self.keywords).unwrap_or_default()
    }

    /// Insert or refresh the entry for `(source_id, path)`.
    pub async fn upsert(pool: &SqlitePool, entry: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO architecture_entry (
                id, source_id, path, category, slug, title,
                keywords, digest, content, blob_sha, synced_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)
            ON CONFLICT(source_id, path) DO UPDATE SET
                category = excluded.category,
                slug = excluded.slug,
                title = excluded.title,
                keywords = excluded.keywords,
                digest = excluded.digest,
                content = excluded.content,
                blob_sha = excluded.blob_sha,
                synced_at = excluded.synced_at
            ",
        )
        .bind(&entry.id)
        .bind(&entry.source_id)
        .bind(&entry.path)
        .bind(&entry.category)
        .bind(&entry.slug)
        .bind(&entry.title)
        .bind(&entry.keywords)
        .bind(&entry.digest)
        .bind(&entry.content)
        .bind(&entry.blob_sha)
        .bind(entry.synced_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Existing `(path, blob_sha)` pairs for a source, used to diff trees.
    pub async fn sha_index(pool: &SqlitePool, source_id: &str) -> sqlx::Result<Vec<(String, String)>> {
        sqlx::query_as::<_, (String, String)>(
            "SELECT path, blob_sha FROM architecture_entry WHERE source_id = ?1",
        )
        .bind(source_id)
        .fetch_all(pool)
        .await
    }

    /// Remove entries whose paths vanished from the upstream tree.
    pub async fn delete_missing(
        pool: &SqlitePool,
        source_id: &str,
        keep_paths: &[String],
    ) -> sqlx::Result<u64> {
        // SQLite has no array binds; serialize to JSON and use json_each.
        let keep_json = serde_json::to_string(keep_paths).unwrap_or_else(|_| "[]".to_string());
        let result = sqlx::query(
            r"
            DELETE FROM architecture_entry
            WHERE source_id = ?1
              AND path NOT IN (SELECT value FROM json_each(?2))
            ",
        )
        .bind(source_id)
        .bind(keep_json)
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }

    /// Lightweight listing (no content/digest payloads).
    pub async fn list_summaries(
        pool: &SqlitePool,
        source_id: Option<&str>,
    ) -> sqlx::Result<Vec<ArchitectureEntrySummary>> {
        match source_id {
            Some(sid) => {
                sqlx::query_as::<_, ArchitectureEntrySummary>(
                    r"
                    SELECT id, source_id, path, category, slug, title, synced_at
                    FROM architecture_entry WHERE source_id = ?1
                    ORDER BY category, slug
                    ",
                )
                .bind(sid)
                .fetch_all(pool)
                .await
            }
            None => {
                sqlx::query_as::<_, ArchitectureEntrySummary>(
                    r"
                    SELECT id, source_id, path, category, slug, title, synced_at
                    FROM architecture_entry
                    ORDER BY category, slug
                    ",
                )
                .fetch_all(pool)
                .await
            }
        }
    }

    /// All entries from enabled sources, used for requirement matching.
    /// Excludes `content` via a projection onto the full struct with an
    /// empty content column to keep the scan cheap.
    pub async fn find_matchable(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            r"
            SELECT e.id, e.source_id, e.path, e.category, e.slug, e.title,
                   e.keywords, e.digest, '' AS content, e.blob_sha, e.synced_at
            FROM architecture_entry e
            JOIN architecture_source s ON s.id = e.source_id
            WHERE s.enabled = 1
            ORDER BY e.category, e.slug
            ",
        )
        .fetch_all(pool)
        .await
    }

    pub async fn count_by_source(pool: &SqlitePool, source_id: &str) -> sqlx::Result<i64> {
        let row: (i64,) =
            sqlx::query_as("SELECT COUNT(*) FROM architecture_entry WHERE source_id = ?1")
                .bind(source_id)
                .fetch_one(pool)
                .await?;
        Ok(row.0)
    }
}
