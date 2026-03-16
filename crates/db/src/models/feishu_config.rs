//! Feishu App Config Model
//!
//! Stores Feishu (Lark) application configuration for connector integration.

use aes_gcm::{
    Aes256Gcm,
    aead::{Aead, AeadCore, KeyInit, OsRng},
};
use base64::{Engine as _, engine::general_purpose};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use std::fmt;
use uuid::Uuid;

/// Feishu App Config
///
/// Corresponds to database table: feishu_app_config
#[derive(Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FeishuAppConfig {
    /// Primary key UUID
    pub id: String,

    /// Feishu app ID
    pub app_id: String,

    /// Encrypted app secret (AES-256-GCM)
    pub app_secret_encrypted: String,

    /// Tenant key (optional, for tenant-scoped tokens)
    pub tenant_key: Option<String>,

    /// Feishu API base URL
    pub base_url: String,

    /// Whether this config is active
    pub enabled: bool,

    /// Created timestamp
    pub created_at: DateTime<Utc>,

    /// Updated timestamp
    pub updated_at: DateTime<Utc>,
}

impl fmt::Debug for FeishuAppConfig {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("FeishuAppConfig")
            .field("id", &self.id)
            .field("app_id", &self.app_id)
            .field("app_secret_encrypted", &"[REDACTED]")
            .field("tenant_key", &self.tenant_key)
            .field("base_url", &self.base_url)
            .field("enabled", &self.enabled)
            .field("created_at", &self.created_at)
            .field("updated_at", &self.updated_at)
            .finish()
    }
}

impl FeishuAppConfig {
    /// Create a new FeishuAppConfig instance (not yet persisted)
    pub fn new(app_id: &str, app_secret_encrypted: &str, base_url: &str) -> Self {
        let now = Utc::now();
        Self {
            id: Uuid::new_v4().to_string(),
            app_id: app_id.to_string(),
            app_secret_encrypted: app_secret_encrypted.to_string(),
            tenant_key: None,
            base_url: base_url.to_string(),
            enabled: false,
            created_at: now,
            updated_at: now,
        }
    }

    /// Insert a new feishu app config record
    pub async fn insert(pool: &SqlitePool, config: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO feishu_app_config (
                id, app_id, app_secret_encrypted, tenant_key,
                base_url, enabled, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            ",
        )
        .bind(&config.id)
        .bind(&config.app_id)
        .bind(&config.app_secret_encrypted)
        .bind(&config.tenant_key)
        .bind(&config.base_url)
        .bind(config.enabled)
        .bind(config.created_at)
        .bind(config.updated_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Find feishu app config by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM feishu_app_config WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Find the currently enabled feishu app config
    pub async fn find_enabled(pool: &SqlitePool) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM feishu_app_config WHERE enabled = 1 LIMIT 1",
        )
        .fetch_optional(pool)
        .await
    }

    /// G32-007: Find the first feishu app config regardless of enabled status.
    ///
    /// Used by upsert flows (e.g. PUT /config) where a disabled config should
    /// still be found and updated rather than creating a duplicate row.
    pub async fn find_first(pool: &SqlitePool) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM feishu_app_config ORDER BY created_at ASC LIMIT 1",
        )
        .fetch_optional(pool)
        .await
    }

    /// List all feishu app configs
    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM feishu_app_config ORDER BY created_at DESC",
        )
        .fetch_all(pool)
        .await
    }

    /// Update the enabled flag
    pub async fn update_enabled(
        pool: &SqlitePool,
        id: &str,
        enabled: bool,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE feishu_app_config SET enabled = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(enabled)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update app credentials
    pub async fn update_credentials(
        pool: &SqlitePool,
        id: &str,
        app_id: &str,
        app_secret_encrypted: &str,
        base_url: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE feishu_app_config SET
                app_id = ?2,
                app_secret_encrypted = ?3,
                base_url = ?4,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(app_id)
        .bind(app_secret_encrypted)
        .bind(base_url)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Delete a feishu app config
    pub async fn delete(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query("DELETE FROM feishu_app_config WHERE id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(())
    }

    /// Encrypt a plaintext secret using AES-256-GCM (same scheme as Workflow API keys).
    ///
    /// Reads the key from `GITCORTEX_ENCRYPTION_KEY` (must be exactly 32 bytes).
    pub fn encrypt_secret(plaintext: &str) -> anyhow::Result<String> {
        let key_str = std::env::var("GITCORTEX_ENCRYPTION_KEY").map_err(|_| {
            anyhow::anyhow!("GITCORTEX_ENCRYPTION_KEY is not set")
        })?;
        if key_str.len() != 32 {
            return Err(anyhow::anyhow!(
                "Invalid encryption key length: got {} bytes, expected 32",
                key_str.len()
            ));
        }
        let key_bytes: [u8; 32] = key_str.as_bytes().try_into().map_err(|_| {
            anyhow::anyhow!("Invalid encryption key format")
        })?;

        let cipher = Aes256Gcm::new_from_slice(&key_bytes)
            .map_err(|e| anyhow::anyhow!("Cipher init failed: {e}"))?;
        let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
        let ciphertext = cipher
            .encrypt(&nonce, plaintext.as_bytes())
            .map_err(|e| anyhow::anyhow!("Encryption failed: {e}"))?;

        let mut combined = nonce.to_vec();
        combined.extend_from_slice(&ciphertext);
        Ok(general_purpose::STANDARD.encode(&combined))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn new_defaults_to_disabled() {
        let config = FeishuAppConfig::new("app-123", "encrypted-secret", "https://open.feishu.cn");
        assert_eq!(config.app_id, "app-123");
        assert_eq!(config.app_secret_encrypted, "encrypted-secret");
        assert_eq!(config.base_url, "https://open.feishu.cn");
        assert!(!config.enabled);
        assert!(config.tenant_key.is_none());
    }
}
