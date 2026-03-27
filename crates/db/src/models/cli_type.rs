//! CLI Type Model
//!
//! Stores supported AI coding agent CLI information like Claude Code, Gemini CLI, Codex, etc.

use aes_gcm::{
    Aes256Gcm, Nonce,
    aead::{Aead, AeadCore, KeyInit, OsRng},
};
use base64::{Engine as _, engine::general_purpose};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;

/// CLI Type
///
/// Corresponds to database table: cli_type
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CliType {
    /// Primary key ID, format: cli-{name}
    pub id: String,

    /// Internal name, e.g., 'claude-code'
    pub name: String,

    /// Display name, e.g., 'Claude Code'
    pub display_name: String,

    /// Detection command, e.g., 'claude --version'
    pub detect_command: String,

    /// Installation command (optional)
    pub install_command: Option<String>,

    /// Installation guide URL
    pub install_guide_url: Option<String>,

    /// Config file path template, e.g., '~/.claude/settings.json'
    pub config_file_path: Option<String>,

    /// Is system built-in
    #[serde(default)]
    pub is_system: bool,

    /// Created timestamp
    pub created_at: DateTime<Utc>,
}

/// Model Config
///
/// Corresponds to database table: model_config
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct ModelConfig {
    /// Primary key ID, format: model-{cli}-{name}
    pub id: String,

    /// Associated CLI type ID
    pub cli_type_id: String,

    /// Model internal name, e.g., 'sonnet'
    pub name: String,

    /// Display name, e.g., 'Claude Sonnet'
    pub display_name: String,

    /// API model ID, e.g., 'claude-sonnet-4-20250514'
    pub api_model_id: Option<String>,

    /// Is default model
    #[serde(default)]
    pub is_default: bool,

    /// Is official model
    #[serde(default)]
    pub is_official: bool,

    /// Created timestamp
    pub created_at: DateTime<Utc>,

    /// Updated timestamp
    pub updated_at: DateTime<Utc>,

    /// Encrypted API key for this model config (used in workspace mode)
    #[serde(skip)]
    #[ts(skip)]
    pub encrypted_api_key: Option<String>,

    /// Base URL for the API provider
    #[serde(skip_serializing_if = "Option::is_none")]
    #[ts(optional)]
    pub base_url: Option<String>,

    /// API type (openai, anthropic, google, openai-compatible)
    #[serde(skip_serializing_if = "Option::is_none")]
    #[ts(optional)]
    pub api_type: Option<String>,

    /// Whether this model config has an API key stored (computed, not in DB)
    #[sqlx(skip)]
    #[serde(default)]
    pub has_api_key: bool,
}

/// CLI Detection Status
///
/// For frontend display of CLI installation status
#[derive(Debug, Clone, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CliDetectionStatus {
    /// CLI type ID
    pub cli_type_id: String,

    /// CLI name
    pub name: String,

    /// Display name
    pub display_name: String,

    /// Is installed
    pub installed: bool,

    /// Version number (if installed)
    pub version: Option<String>,

    /// Executable file path (if installed)
    pub executable_path: Option<String>,

    /// Installation guide URL
    pub install_guide_url: Option<String>,
}

impl CliType {
    /// Get all CLI types from database
    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, CliType>(
            r"
            SELECT id, name, display_name, detect_command, install_command,
                   install_guide_url, config_file_path, is_system, created_at
            FROM cli_type
            ORDER BY is_system DESC, name ASC
            ",
        )
        .fetch_all(pool)
        .await
    }

    /// Find CLI type by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, CliType>(
            r"
            SELECT id, name, display_name, detect_command, install_command,
                   install_guide_url, config_file_path, is_system, created_at
            FROM cli_type
            WHERE id = ?
            ",
        )
        .bind(id)
        .fetch_optional(pool)
        .await
    }

    /// Find CLI type by name
    pub async fn find_by_name(pool: &SqlitePool, name: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, CliType>(
            r"
            SELECT id, name, display_name, detect_command, install_command,
                   install_guide_url, config_file_path, is_system, created_at
            FROM cli_type
            WHERE name = ?
            ",
        )
        .bind(name)
        .fetch_optional(pool)
        .await
    }
}

impl ModelConfig {
    const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";
    const ENCRYPTION_KEY_ENV_LEGACY: &str = "GITCORTEX_ENCRYPTION_KEY";

    /// Get encryption key from environment variable
    fn get_encryption_key() -> anyhow::Result<[u8; 32]> {
        let key_str = std::env::var(Self::ENCRYPTION_KEY_ENV)
            .or_else(|_| {
                let val = std::env::var(Self::ENCRYPTION_KEY_ENV_LEGACY)?;
                tracing::warn!(
                    new = Self::ENCRYPTION_KEY_ENV,
                    old = Self::ENCRYPTION_KEY_ENV_LEGACY,
                    "Deprecated env var used; please switch to the new name"
                );
                Ok(val)
            })
            .map_err(|_: std::env::VarError| anyhow::anyhow!(
                "Encryption key not found. Please set {} environment variable with a 32-byte value.",
                Self::ENCRYPTION_KEY_ENV
            ))?;

        key_str
            .as_bytes()
            .try_into()
            .map_err(|_| anyhow::anyhow!("Invalid encryption key format"))
    }

    /// Set API key with encryption
    pub fn set_api_key(&mut self, plaintext: &str) -> anyhow::Result<()> {
        let key = Self::get_encryption_key()?;
        let cipher = Aes256Gcm::new_from_slice(&key)
            .map_err(|e| anyhow::anyhow!("Invalid encryption key: {e}"))?;
        let nonce = Aes256Gcm::generate_nonce(&mut OsRng);

        let ciphertext = cipher
            .encrypt(&nonce, plaintext.as_bytes())
            .map_err(|e| anyhow::anyhow!("Encryption failed: {e}"))?;

        let mut combined = nonce.to_vec();
        combined.extend_from_slice(&ciphertext);

        self.encrypted_api_key = Some(general_purpose::STANDARD.encode(&combined));
        Ok(())
    }

    /// Get API key with decryption
    pub fn get_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.encrypted_api_key {
            None => Ok(None),
            Some(encoded) => {
                let key = Self::get_encryption_key()?;
                let combined = general_purpose::STANDARD
                    .decode(encoded)
                    .map_err(|e| anyhow::anyhow!("Base64 decode failed: {e}"))?;

                if combined.len() < 12 {
                    return Err(anyhow::anyhow!("Invalid encrypted data length"));
                }

                let (nonce_bytes, ciphertext) = combined.split_at(12);
                #[allow(deprecated)]
                let nonce = Nonce::from_slice(nonce_bytes);
                let cipher = Aes256Gcm::new_from_slice(&key)
                    .map_err(|e| anyhow::anyhow!("Invalid encryption key: {e}"))?;

                let plaintext_bytes = cipher
                    .decrypt(nonce, ciphertext)
                    .map_err(|e| anyhow::anyhow!("Decryption failed: {e}"))?;

                Ok(Some(String::from_utf8(plaintext_bytes).map_err(|e| {
                    anyhow::anyhow!("Invalid UTF-8 in decrypted data: {e}")
                })?))
            }
        }
    }

    /// Post-process query results to set `has_api_key` computed field
    fn with_has_api_key(mut self) -> Self {
        self.has_api_key = self.encrypted_api_key.is_some();
        self
    }

    /// Post-process a Vec of query results
    fn vec_with_has_api_key(items: Vec<Self>) -> Vec<Self> {
        items.into_iter().map(Self::with_has_api_key).collect()
    }

    /// Get all models for a CLI type
    pub async fn find_by_cli_type(pool: &SqlitePool, cli_type_id: &str) -> sqlx::Result<Vec<Self>> {
        let items = sqlx::query_as::<_, ModelConfig>(
            r"
            SELECT id, cli_type_id, name, display_name, api_model_id,
                   is_default, is_official, created_at, updated_at,
                   encrypted_api_key, base_url, api_type
            FROM model_config
            WHERE cli_type_id = ?
            ORDER BY is_default DESC, name ASC
            ",
        )
        .bind(cli_type_id)
        .fetch_all(pool)
        .await?;
        Ok(Self::vec_with_has_api_key(items))
    }

    /// Find model config by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        let item = sqlx::query_as::<_, ModelConfig>(
            r"
            SELECT id, cli_type_id, name, display_name, api_model_id,
                   is_default, is_official, created_at, updated_at,
                   encrypted_api_key, base_url, api_type
            FROM model_config
            WHERE id = ?
            ",
        )
        .bind(id)
        .fetch_optional(pool)
        .await?;
        Ok(item.map(Self::with_has_api_key))
    }

    /// Create a custom model config from inline data
    ///
    /// Used when frontend sends a temporary model_config_id that doesn't exist
    /// in the database, along with inline model configuration data.
    /// If the model config already exists (concurrent request), returns the existing one.
    pub async fn create_custom(
        pool: &SqlitePool,
        id: &str,
        cli_type_id: &str,
        display_name: &str,
        api_model_id: &str,
    ) -> sqlx::Result<Self> {
        let now = chrono::Utc::now();
        // Use the ID as the name for custom configs
        let name = id.to_string();

        // Insert or update model fields on conflict (ensures latest model ID is always stored)
        let item = sqlx::query_as::<_, ModelConfig>(
            r"
            INSERT INTO model_config (
                id, cli_type_id, name, display_name, api_model_id,
                is_default, is_official, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, 0, 0, ?6, ?7)
            ON CONFLICT(id) DO UPDATE SET
                display_name = excluded.display_name,
                api_model_id = excluded.api_model_id,
                updated_at = excluded.updated_at
            RETURNING id, cli_type_id, name, display_name, api_model_id,
                      is_default, is_official, created_at, updated_at,
                      encrypted_api_key, base_url, api_type
            ",
        )
        .bind(id)
        .bind(cli_type_id)
        .bind(&name)
        .bind(display_name)
        .bind(api_model_id)
        .bind(now)
        .bind(now)
        .fetch_one(pool)
        .await?;
        Ok(item.with_has_api_key())
    }

    /// Get default model for a CLI type
    pub async fn find_default_for_cli(
        pool: &SqlitePool,
        cli_type_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        let item = sqlx::query_as::<_, ModelConfig>(
            r"
            SELECT id, cli_type_id, name, display_name, api_model_id,
                   is_default, is_official, created_at, updated_at,
                   encrypted_api_key, base_url, api_type
            FROM model_config
            WHERE cli_type_id = ? AND is_default = 1
            LIMIT 1
            ",
        )
        .bind(cli_type_id)
        .fetch_optional(pool)
        .await?;
        Ok(item.map(Self::with_has_api_key))
    }

    /// Get only user-configured models (non-official) that have API keys.
    /// The agent must never see official preset models without real credentials.
    pub async fn find_user_configured(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        let items = sqlx::query_as::<_, ModelConfig>(
            r"
            SELECT id, cli_type_id, name, display_name, api_model_id,
                   is_default, is_official, created_at, updated_at,
                   encrypted_api_key, base_url, api_type
            FROM model_config
            WHERE is_official = 0 AND encrypted_api_key IS NOT NULL
            ORDER BY cli_type_id, is_default DESC, name ASC
            ",
        )
        .fetch_all(pool)
        .await?;
        Ok(Self::vec_with_has_api_key(items))
    }

    /// Return `(cli_type_id, model_config_id)` of the first user-configured
    /// model, or `None` if no user models exist. Uses `LIMIT 1`.
    pub async fn first_user_configured_ids(
        pool: &SqlitePool,
    ) -> sqlx::Result<Option<(String, String)>> {
        let row = sqlx::query_as::<_, (String, String)>(
            r"
            SELECT cli_type_id, id
            FROM model_config
            WHERE is_official = 0 AND encrypted_api_key IS NOT NULL
            ORDER BY cli_type_id, is_default DESC, name ASC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await?;
        Ok(row)
    }

    /// Get all model configs
    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        let items = sqlx::query_as::<_, ModelConfig>(
            r"
            SELECT id, cli_type_id, name, display_name, api_model_id,
                   is_default, is_official, created_at, updated_at,
                   encrypted_api_key, base_url, api_type
            FROM model_config
            ORDER BY cli_type_id, is_default DESC, name ASC
            ",
        )
        .fetch_all(pool)
        .await?;
        Ok(Self::vec_with_has_api_key(items))
    }

    /// Update credentials (API key, base URL, API type) for a model config
    pub async fn update_credentials(
        pool: &SqlitePool,
        id: &str,
        encrypted_api_key: &str,
        base_url: Option<&str>,
        api_type: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE model_config
            SET encrypted_api_key = ?, base_url = ?, api_type = ?, updated_at = ?
            WHERE id = ?
            ",
        )
        .bind(encrypted_api_key)
        .bind(base_url)
        .bind(api_type)
        .bind(chrono::Utc::now())
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Resolve a model config: prefer an explicitly selected config ID,
    /// fall back to the first config with credentials for the given CLI type.
    pub async fn resolve_preferred_or_default(
        pool: &SqlitePool,
        config_id: Option<&str>,
        cli_type_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        if let Some(id) = config_id {
            if let Some(mc) = Self::find_by_id(pool, id).await? {
                return Ok(Some(mc));
            }
        }
        Self::find_with_credentials_for_cli(pool, cli_type_id).await
    }

    /// Find the first model config with stored credentials for a given CLI type
    pub async fn find_with_credentials_for_cli(
        pool: &SqlitePool,
        cli_type_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        let item = sqlx::query_as::<_, ModelConfig>(
            r"
            SELECT id, cli_type_id, name, display_name, api_model_id,
                   is_default, is_official, created_at, updated_at,
                   encrypted_api_key, base_url, api_type
            FROM model_config
            WHERE cli_type_id = ? AND encrypted_api_key IS NOT NULL
            ORDER BY is_default DESC, updated_at DESC
            LIMIT 1
            ",
        )
        .bind(cli_type_id)
        .fetch_optional(pool)
        .await?;
        Ok(item.map(Self::with_has_api_key))
    }
}
