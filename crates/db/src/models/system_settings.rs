use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
pub struct SystemSetting {
    pub key: String,
    pub value: String,
    pub description: Option<String>,
    pub updated_at: String,
}

impl SystemSetting {
    /// Fetch all system settings, ordered by key.
    pub async fn find_all(pool: &SqlitePool) -> anyhow::Result<Vec<Self>> {
        let rows = sqlx::query_as::<_, Self>("SELECT * FROM system_settings ORDER BY key")
            .fetch_all(pool)
            .await?;
        Ok(rows)
    }

    /// Get the value of a single setting by key, or `None` if not found.
    pub async fn get(pool: &SqlitePool, key: &str) -> anyhow::Result<Option<String>> {
        let row: Option<(String,)> =
            sqlx::query_as("SELECT value FROM system_settings WHERE key = ?1")
                .bind(key)
                .fetch_optional(pool)
                .await?;
        Ok(row.map(|r| r.0))
    }

    /// Get a boolean setting. Returns `true` if the stored value is `"true"` or `"1"`.
    pub async fn get_bool(pool: &SqlitePool, key: &str) -> anyhow::Result<bool> {
        let value = Self::get(pool, key).await?;
        Ok(value.is_some_and(|v| v.trim().eq_ignore_ascii_case("true") || v.trim() == "1"))
    }

    /// Upsert a setting value. Creates the row if it doesn't exist, otherwise updates it.
    pub async fn set(pool: &SqlitePool, key: &str, value: &str) -> anyhow::Result<()> {
        // E38-10: `INSERT OR REPLACE` is SQLite-specific; we intentionally keep
        // it here because this crate is SQLite-only (see `SqlitePool`).
        // If we ever support another backend, switch to
        // `ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = ...`.
        sqlx::query(
            "INSERT OR REPLACE INTO system_settings (key, value, updated_at) VALUES (?1, ?2, datetime('now'))",
        )
        .bind(key)
        .bind(value)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Check if Feishu is enabled via environment variable only (for sync contexts).
    ///
    /// Returns `Some(true/false)` if the env var is set, `None` otherwise.
    pub fn is_feishu_enabled_sync() -> Option<bool> {
        std::env::var("SOLODAWN_FEISHU_ENABLED")
            .or_else(|_| std::env::var("GITCORTEX_FEISHU_ENABLED"))
            .ok()
            .map(|v| v.trim().eq_ignore_ascii_case("true") || v.trim() == "1")
    }

    /// Check if Feishu is enabled. Env var takes precedence over database.
    pub async fn is_feishu_enabled(pool: &SqlitePool) -> bool {
        // 1. Check env var first (takes precedence)
        if let Some(env_val) = Self::is_feishu_enabled_sync() {
            // Surface a warning when the env var overrides an explicitly
            // stored, conflicting database setting so operators are not
            // surprised by a silent override. We use `get` (not `get_bool`)
            // so that an unset row does not spuriously trigger the warn.
            if let Ok(Some(raw)) = Self::get(pool, "feishu_enabled").await {
                let db_val =
                    raw.trim().eq_ignore_ascii_case("true") || raw.trim() == "1";
                if db_val != env_val {
                    tracing::warn!(
                        env = env_val,
                        db = db_val,
                        "SOLODAWN_FEISHU_ENABLED environment variable overrides conflicting `feishu_enabled` database setting"
                    );
                }
            }
            return env_val;
        }
        // 2. Fall back to database
        Self::get_bool(pool, "feishu_enabled")
            .await
            .unwrap_or(false)
    }
}
