//! Concierge Agent models: shared conversation sessions across Feishu and Web UI.

use aes_gcm::{
    Aes256Gcm, Nonce,
    aead::{Aead, AeadCore, KeyInit, OsRng},
};
use base64::{Engine as _, engine::general_purpose};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

// ---------------------------------------------------------------------------
// ConciergeSession
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
#[allow(clippy::struct_excessive_bools)]
pub struct ConciergeSession {
    pub id: String,
    pub name: String,
    pub active_project_id: Option<String>,
    pub active_workflow_id: Option<String>,
    pub active_planning_draft_id: Option<String>,
    pub feishu_sync: bool,
    pub feishu_chat_id: Option<String>,
    pub progress_notifications: bool,
    pub llm_model_id: Option<String>,
    pub llm_api_type: Option<String>,
    pub llm_base_url: Option<String>,
    #[serde(skip_serializing)]
    pub llm_api_key_encrypted: Option<String>,
    /// Push tool call events to Feishu when true.
    pub sync_tools: bool,
    /// Push terminal status changes to Feishu when true.
    pub sync_terminal: bool,
    /// Push workflow progress events to Feishu when true.
    pub sync_progress: bool,
    /// Send completion report on workflow/task completion.
    pub notify_on_completion: bool,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl ConciergeSession {
    const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";
    const ENCRYPTION_KEY_ENV_LEGACY: &str = "GITCORTEX_ENCRYPTION_KEY";

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
            .map_err(|_: std::env::VarError| {
                anyhow::anyhow!(
                    "Encryption key not found. Set {} (32-byte value).",
                    Self::ENCRYPTION_KEY_ENV
                )
            })?;
        if key_str.len() != 32 {
            return Err(anyhow::anyhow!(
                "Invalid encryption key length: got {} bytes, expected 32",
                key_str.len()
            ));
        }
        key_str
            .as_bytes()
            .try_into()
            .map_err(|_| anyhow::anyhow!("Invalid encryption key format"))
    }

    pub fn encrypt_api_key(plaintext: &str) -> anyhow::Result<String> {
        let key = Self::get_encryption_key()?;
        let cipher = Aes256Gcm::new_from_slice(&key)
            .map_err(|e| anyhow::anyhow!("Invalid encryption key: {e}"))?;
        let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
        let ciphertext = cipher
            .encrypt(&nonce, plaintext.as_bytes())
            .map_err(|e| anyhow::anyhow!("Encryption failed: {e}"))?;
        let mut combined = nonce.to_vec();
        combined.extend_from_slice(&ciphertext);
        Ok(general_purpose::STANDARD.encode(&combined))
    }

    pub fn decrypt_api_key(encoded: &str) -> anyhow::Result<String> {
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
        String::from_utf8(plaintext_bytes)
            .map_err(|e| anyhow::anyhow!("Invalid UTF-8 in decrypted data: {e}"))
    }

    pub fn get_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.llm_api_key_encrypted {
            None => Ok(None),
            Some(encoded) => Self::decrypt_api_key(encoded).map(Some),
        }
    }

    pub fn new(name: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            name: name.to_string(),
            active_project_id: None,
            active_workflow_id: None,
            active_planning_draft_id: None,
            feishu_sync: false,
            feishu_chat_id: None,
            progress_notifications: false,
            llm_model_id: None,
            llm_api_type: None,
            llm_base_url: None,
            llm_api_key_encrypted: None,
            sync_tools: false,
            sync_terminal: false,
            sync_progress: false,
            notify_on_completion: true,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    pub async fn insert(pool: &SqlitePool, session: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO concierge_session (
                id, name, active_project_id, active_workflow_id, active_planning_draft_id,
                feishu_sync, feishu_chat_id, progress_notifications,
                llm_model_id, llm_api_type, llm_base_url, llm_api_key_encrypted,
                sync_tools, sync_terminal, sync_progress, notify_on_completion,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18)
            ",
        )
        .bind(&session.id)
        .bind(&session.name)
        .bind(&session.active_project_id)
        .bind(&session.active_workflow_id)
        .bind(&session.active_planning_draft_id)
        .bind(session.feishu_sync)
        .bind(&session.feishu_chat_id)
        .bind(session.progress_notifications)
        .bind(&session.llm_model_id)
        .bind(&session.llm_api_type)
        .bind(&session.llm_base_url)
        .bind(&session.llm_api_key_encrypted)
        .bind(session.sync_tools)
        .bind(session.sync_terminal)
        .bind(session.sync_progress)
        .bind(session.notify_on_completion)
        .bind(session.created_at)
        .bind(session.updated_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM concierge_session WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    pub async fn list_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM concierge_session ORDER BY updated_at DESC",
        )
        .fetch_all(pool)
        .await
    }

    /// Find the session bound to a specific channel (e.g. feishu chat_id).
    pub async fn find_by_channel(
        pool: &SqlitePool,
        provider: &str,
        external_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            r"
            SELECT s.* FROM concierge_session s
            JOIN concierge_session_channel c ON c.session_id = s.id
            WHERE c.provider = ?1 AND c.external_id = ?2 AND c.is_active = 1
            LIMIT 1
            ",
        )
        .bind(provider)
        .bind(external_id)
        .fetch_optional(pool)
        .await
    }

    /// Find ALL sessions (active + inactive) bound to a specific channel.
    pub async fn find_all_by_channel(
        pool: &SqlitePool,
        provider: &str,
        external_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            r"
            SELECT s.* FROM concierge_session s
            JOIN concierge_session_channel c ON c.session_id = s.id
            WHERE c.provider = ?1 AND c.external_id = ?2
            ORDER BY s.updated_at DESC
            ",
        )
        .bind(provider)
        .bind(external_id)
        .fetch_all(pool)
        .await
    }

    pub async fn update_active_project(
        pool: &SqlitePool,
        id: &str,
        project_id: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session SET active_project_id = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(project_id)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_active_workflow(
        pool: &SqlitePool,
        id: &str,
        workflow_id: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session SET active_workflow_id = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(workflow_id)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_feishu_sync(
        pool: &SqlitePool,
        id: &str,
        enabled: bool,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session SET feishu_sync = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(enabled)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_feishu_chat_id(
        pool: &SqlitePool,
        id: &str,
        chat_id: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session SET feishu_chat_id = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(chat_id)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_progress_notifications(
        pool: &SqlitePool,
        id: &str,
        enabled: bool,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session SET progress_notifications = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(enabled)
        .execute(pool)
        .await?;
        Ok(())
    }

    #[allow(clippy::fn_params_excessive_bools)]
    pub async fn update_sync_toggles(
        pool: &SqlitePool,
        id: &str,
        sync_tools: bool,
        sync_terminal: bool,
        sync_progress: bool,
        notify_on_completion: bool,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE concierge_session SET
                sync_tools = ?2, sync_terminal = ?3,
                sync_progress = ?4, notify_on_completion = ?5,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(sync_tools)
        .bind(sync_terminal)
        .bind(sync_progress)
        .bind(notify_on_completion)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_llm_config(
        pool: &SqlitePool,
        id: &str,
        model_id: Option<&str>,
        api_type: Option<&str>,
        base_url: Option<&str>,
        api_key_encrypted: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE concierge_session SET
                llm_model_id = ?2, llm_api_type = ?3,
                llm_base_url = ?4, llm_api_key_encrypted = ?5,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(model_id)
        .bind(api_type)
        .bind(base_url)
        .bind(api_key_encrypted)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_name(pool: &SqlitePool, id: &str, name: &str) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session SET name = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(name)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Find all sessions bound to a specific workflow with Feishu sync enabled.
    pub async fn find_by_workflow_with_feishu(
        pool: &SqlitePool,
        workflow_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            r"
            SELECT * FROM concierge_session
            WHERE active_workflow_id = ?1 AND feishu_sync = 1
            ",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    /// Find all sessions bound to a specific workflow with terminal sync enabled.
    pub async fn find_by_workflow_with_sync_terminal(
        pool: &SqlitePool,
        workflow_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            r"
            SELECT * FROM concierge_session
            WHERE active_workflow_id = ?1 AND sync_terminal = 1
            ",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    pub async fn delete(pool: &SqlitePool, id: &str) -> sqlx::Result<u64> {
        let rows = sqlx::query("DELETE FROM concierge_session WHERE id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(rows.rows_affected())
    }
}

// ---------------------------------------------------------------------------
// ConciergeSessionChannel
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConciergeSessionChannel {
    pub id: String,
    pub session_id: String,
    pub provider: String,
    pub external_id: String,
    pub user_identifier: Option<String>,
    pub is_active: bool,
    pub created_at: DateTime<Utc>,
}

impl ConciergeSessionChannel {
    pub fn new(session_id: &str, provider: &str, external_id: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            session_id: session_id.to_string(),
            provider: provider.to_string(),
            external_id: external_id.to_string(),
            user_identifier: None,
            is_active: true,
            created_at: Utc::now(),
        }
    }

    /// Insert or re-activate an existing channel binding.
    pub async fn upsert(
        pool: &SqlitePool,
        session_id: &str,
        provider: &str,
        external_id: &str,
        user_identifier: Option<&str>,
    ) -> sqlx::Result<()> {
        let id = Uuid::new_v4().to_string();
        sqlx::query(
            r"
            INSERT INTO concierge_session_channel (
                id, session_id, provider, external_id, user_identifier, is_active, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, 1, datetime('now'))
            ON CONFLICT(provider, external_id)
            DO UPDATE SET
                session_id = excluded.session_id,
                user_identifier = excluded.user_identifier,
                is_active = 1
            ",
        )
        .bind(id)
        .bind(session_id)
        .bind(provider)
        .bind(external_id)
        .bind(user_identifier)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_active_by_session(
        pool: &SqlitePool,
        session_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM concierge_session_channel WHERE session_id = ?1 AND is_active = 1",
        )
        .bind(session_id)
        .fetch_all(pool)
        .await
    }

    pub async fn deactivate(
        pool: &SqlitePool,
        provider: &str,
        external_id: &str,
    ) -> sqlx::Result<u64> {
        let rows = sqlx::query(
            "UPDATE concierge_session_channel SET is_active = 0 WHERE provider = ?1 AND external_id = ?2",
        )
        .bind(provider)
        .bind(external_id)
        .execute(pool)
        .await?;
        Ok(rows.rows_affected())
    }

    /// Switch: deactivate all channels for provider+external_id, then activate
    /// only the one for target_session_id.
    pub async fn switch_active_session(
        pool: &SqlitePool,
        provider: &str,
        external_id: &str,
        target_session_id: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE concierge_session_channel SET is_active = 0 WHERE provider = ?1 AND external_id = ?2",
        )
        .bind(provider)
        .bind(external_id)
        .execute(pool)
        .await?;
        sqlx::query(
            "UPDATE concierge_session_channel SET is_active = 1 WHERE provider = ?1 AND external_id = ?2 AND session_id = ?3",
        )
        .bind(provider)
        .bind(external_id)
        .bind(target_session_id)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn delete_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<u64> {
        let rows = sqlx::query("DELETE FROM concierge_session_channel WHERE id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(rows.rows_affected())
    }
}

// ---------------------------------------------------------------------------
// ConciergeMessage
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConciergeMessage {
    pub id: String,
    pub session_id: String,
    pub role: String,
    pub content: String,
    pub source_provider: Option<String>,
    pub source_user: Option<String>,
    pub tool_name: Option<String>,
    pub tool_call_id: Option<String>,
    pub created_at: DateTime<Utc>,
}

impl ConciergeMessage {
    pub fn new_user(
        session_id: &str,
        content: &str,
        source_provider: Option<&str>,
        source_user: Option<&str>,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            session_id: session_id.to_string(),
            role: "user".to_string(),
            content: content.to_string(),
            source_provider: source_provider.map(ToString::to_string),
            source_user: source_user.map(ToString::to_string),
            tool_name: None,
            tool_call_id: None,
            created_at: Utc::now(),
        }
    }

    pub fn new_assistant(session_id: &str, content: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            session_id: session_id.to_string(),
            role: "assistant".to_string(),
            content: content.to_string(),
            source_provider: None,
            source_user: None,
            tool_name: None,
            tool_call_id: None,
            created_at: Utc::now(),
        }
    }

    pub fn new_tool_call(session_id: &str, tool_name: &str, content: &str) -> Self {
        let call_id = Uuid::new_v4().to_string();
        Self {
            id: Uuid::new_v4().to_string(),
            session_id: session_id.to_string(),
            role: "tool_call".to_string(),
            content: content.to_string(),
            source_provider: None,
            source_user: None,
            tool_name: Some(tool_name.to_string()),
            tool_call_id: Some(call_id),
            created_at: Utc::now(),
        }
    }

    pub fn new_tool_result(session_id: &str, tool_call_id: &str, content: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            session_id: session_id.to_string(),
            role: "tool_result".to_string(),
            content: content.to_string(),
            source_provider: None,
            source_user: None,
            tool_name: None,
            tool_call_id: Some(tool_call_id.to_string()),
            created_at: Utc::now(),
        }
    }

    pub fn new_system(session_id: &str, content: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            session_id: session_id.to_string(),
            role: "system".to_string(),
            content: content.to_string(),
            source_provider: None,
            source_user: None,
            tool_name: None,
            tool_call_id: None,
            created_at: Utc::now(),
        }
    }

    pub async fn insert(pool: &SqlitePool, message: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO concierge_message (
                id, session_id, role, content,
                source_provider, source_user,
                tool_name, tool_call_id, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ",
        )
        .bind(&message.id)
        .bind(&message.session_id)
        .bind(&message.role)
        .bind(&message.content)
        .bind(&message.source_provider)
        .bind(&message.source_user)
        .bind(&message.tool_name)
        .bind(&message.tool_call_id)
        .bind(message.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn list_by_session(
        pool: &SqlitePool,
        session_id: &str,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM concierge_message WHERE session_id = ?1 ORDER BY created_at ASC, id ASC",
        )
        .bind(session_id)
        .fetch_all(pool)
        .await
    }

    #[allow(clippy::cast_possible_wrap)]
    pub async fn list_by_session_paginated(
        pool: &SqlitePool,
        session_id: &str,
        cursor: usize,
        limit: usize,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            r"
            SELECT * FROM concierge_message
            WHERE session_id = ?1
            ORDER BY created_at ASC, id ASC
            LIMIT ?2 OFFSET ?3
            ",
        )
        .bind(session_id)
        .bind(limit as i64)
        .bind(cursor as i64)
        .fetch_all(pool)
        .await
    }

    /// Fetch the last N messages for LLM context window.
    #[allow(clippy::cast_possible_wrap)]
    pub async fn list_recent(
        pool: &SqlitePool,
        session_id: &str,
        limit: usize,
    ) -> sqlx::Result<Vec<Self>> {
        // Sub-query to get last N rows, then re-order ascending.
        sqlx::query_as::<_, Self>(
            r"
            SELECT * FROM (
                SELECT * FROM concierge_message
                WHERE session_id = ?1
                ORDER BY created_at DESC, id DESC
                LIMIT ?2
            ) ORDER BY created_at ASC, id ASC
            ",
        )
        .bind(session_id)
        .bind(limit as i64)
        .fetch_all(pool)
        .await
    }
}
