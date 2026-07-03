//! Design style templates: named visual-direction prompts. Builtin presets
//! are seeded at startup from `crates/services/assets/design_styles/` and are
//! immutable apart from the `enabled` flag; user styles support full CRUD.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DesignStyle {
    pub id: String,
    pub slug: String,
    pub name: String,
    pub description: String,
    /// The design directive prompt injected for UI-related work.
    pub content: String,
    pub source_name: Option<String>,
    pub source_url: Option<String>,
    pub license: Option<String>,
    pub builtin: bool,
    pub enabled: bool,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl DesignStyle {
    pub fn new(slug: &str, name: &str, description: &str, content: &str) -> Self {
        let now = Utc::now();
        Self {
            id: Uuid::new_v4().to_string(),
            slug: slug.to_string(),
            name: name.to_string(),
            description: description.to_string(),
            content: content.to_string(),
            source_name: None,
            source_url: None,
            license: None,
            builtin: false,
            enabled: true,
            created_at: now,
            updated_at: now,
        }
    }

    pub async fn insert(pool: &SqlitePool, style: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO design_style (
                id, slug, name, description, content,
                source_name, source_url, license, builtin, enabled,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
            ",
        )
        .bind(&style.id)
        .bind(&style.slug)
        .bind(&style.name)
        .bind(&style.description)
        .bind(&style.content)
        .bind(&style.source_name)
        .bind(&style.source_url)
        .bind(&style.license)
        .bind(style.builtin)
        .bind(style.enabled)
        .bind(style.created_at)
        .bind(style.updated_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Upsert a builtin preset by slug, refreshing content and attribution
    /// while preserving the user's `enabled` choice on existing rows.
    #[allow(clippy::too_many_arguments)]
    pub async fn upsert_builtin(
        pool: &SqlitePool,
        slug: &str,
        name: &str,
        description: &str,
        content: &str,
        source_name: &str,
        source_url: &str,
        license: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO design_style (
                id, slug, name, description, content,
                source_name, source_url, license, builtin, enabled,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, 1, 1, datetime('now'), datetime('now'))
            ON CONFLICT(slug) DO UPDATE SET
                name = excluded.name,
                description = excluded.description,
                content = excluded.content,
                source_name = excluded.source_name,
                source_url = excluded.source_url,
                license = excluded.license,
                builtin = 1,
                updated_at = datetime('now')
            ",
        )
        .bind(Uuid::new_v4().to_string())
        .bind(slug)
        .bind(name)
        .bind(description)
        .bind(content)
        .bind(source_name)
        .bind(source_url)
        .bind(license)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM design_style ORDER BY builtin DESC, created_at ASC",
        )
        .fetch_all(pool)
        .await
    }

    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM design_style WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    pub async fn find_by_slug(pool: &SqlitePool, slug: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM design_style WHERE slug = ?1")
            .bind(slug)
            .fetch_optional(pool)
            .await
    }

    /// Resolve a slug to an enabled style, used at prompt-injection time.
    pub async fn find_enabled_by_slug(pool: &SqlitePool, slug: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM design_style WHERE slug = ?1 AND enabled = 1")
            .bind(slug)
            .fetch_optional(pool)
            .await
    }

    /// Update a user style. Builtin rows only accept the `enabled` flag; the
    /// caller enforces that split — this method writes whatever it is given
    /// for non-builtin rows.
    pub async fn update_custom(
        pool: &SqlitePool,
        id: &str,
        name: &str,
        description: &str,
        content: &str,
        enabled: bool,
    ) -> sqlx::Result<u64> {
        let result = sqlx::query(
            r"
            UPDATE design_style
            SET name = ?2, description = ?3, content = ?4, enabled = ?5,
                updated_at = datetime('now')
            WHERE id = ?1 AND builtin = 0
            ",
        )
        .bind(id)
        .bind(name)
        .bind(description)
        .bind(content)
        .bind(enabled)
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }

    pub async fn set_enabled(pool: &SqlitePool, id: &str, enabled: bool) -> sqlx::Result<u64> {
        let result = sqlx::query(
            "UPDATE design_style SET enabled = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(enabled)
        .execute(pool)
        .await?;
        Ok(result.rows_affected())
    }

    /// Delete a non-builtin style. Returns the number of rows removed.
    pub async fn delete_custom(pool: &SqlitePool, id: &str) -> sqlx::Result<u64> {
        let result = sqlx::query("DELETE FROM design_style WHERE id = ?1 AND builtin = 0")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }
}
