//! Planning draft models for orchestrated workspace mode.

use aes_gcm::{
    Aes256Gcm, Nonce,
    aead::{Aead, AeadCore, KeyInit, OsRng},
};
use base64::{Engine as _, engine::general_purpose};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PlanningDraft {
    pub id: String,
    pub project_id: Uuid,
    pub name: String,
    pub status: String,
    pub requirement_summary: Option<String>,
    pub technical_spec: Option<String>,
    pub workflow_seed: Option<String>,
    pub planner_model_id: Option<String>,
    pub planner_api_type: Option<String>,
    pub planner_base_url: Option<String>,
    #[serde(skip_serializing)]
    pub planner_api_key: Option<String>,
    pub confirmed_at: Option<DateTime<Utc>>,
    pub materialized_workflow_id: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Valid status transitions: gathering -> spec_ready -> confirmed -> materialized
pub const PLANNING_DRAFT_STATUSES: [&str; 5] =
    ["gathering", "spec_ready", "confirmed", "materialized", "cancelled"];

impl PlanningDraft {
    const ENCRYPTION_KEY_ENV: &str = "GITCORTEX_ENCRYPTION_KEY";

    /// Get encryption key from environment variable (same as Workflow).
    fn get_encryption_key() -> anyhow::Result<[u8; 32]> {
        let key_str = std::env::var(Self::ENCRYPTION_KEY_ENV).map_err(|_| {
            anyhow::anyhow!(
                "Encryption key not found. Please set {} environment variable with a 32-byte value.",
                Self::ENCRYPTION_KEY_ENV
            )
        })?;

        if key_str.len() != 32 {
            return Err(anyhow::anyhow!(
                "Invalid encryption key length: got {} bytes, expected exactly 32 bytes",
                key_str.len()
            ));
        }

        key_str
            .as_bytes()
            .try_into()
            .map_err(|_| anyhow::anyhow!("Invalid encryption key format"))
    }

    /// Encrypt and store the planner API key.
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

        self.planner_api_key = Some(general_purpose::STANDARD.encode(&combined));
        Ok(())
    }

    /// Decrypt and return the planner API key.
    pub fn get_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.planner_api_key {
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

    pub fn new(project_id: Uuid, name: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            project_id,
            name: name.to_string(),
            status: "gathering".to_string(),
            requirement_summary: None,
            technical_spec: None,
            workflow_seed: None,
            planner_model_id: None,
            planner_api_type: None,
            planner_base_url: None,
            planner_api_key: None,
            confirmed_at: None,
            materialized_workflow_id: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    pub async fn insert(pool: &SqlitePool, draft: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO planning_draft (
                id, project_id, name, status,
                requirement_summary, technical_spec, workflow_seed,
                planner_model_id, planner_api_type, planner_base_url, planner_api_key,
                confirmed_at, materialized_workflow_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15)
            ",
        )
        .bind(&draft.id)
        .bind(draft.project_id)
        .bind(&draft.name)
        .bind(&draft.status)
        .bind(&draft.requirement_summary)
        .bind(&draft.technical_spec)
        .bind(&draft.workflow_seed)
        .bind(&draft.planner_model_id)
        .bind(&draft.planner_api_type)
        .bind(&draft.planner_base_url)
        .bind(&draft.planner_api_key)
        .bind(draft.confirmed_at)
        .bind(&draft.materialized_workflow_id)
        .bind(draft.created_at)
        .bind(draft.updated_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM planning_draft WHERE id = ?1")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    pub async fn find_by_project(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM planning_draft WHERE project_id = ?1 ORDER BY created_at DESC",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await
    }

    pub async fn update_status(pool: &SqlitePool, id: &str, status: &str) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE planning_draft SET status = ?2, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(status)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn update_spec(
        pool: &SqlitePool,
        id: &str,
        requirement_summary: Option<&str>,
        technical_spec: Option<&str>,
        workflow_seed: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                requirement_summary = ?2,
                technical_spec = ?3,
                workflow_seed = ?4,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(requirement_summary)
        .bind(technical_spec)
        .bind(workflow_seed)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn set_confirmed(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                status = 'confirmed',
                confirmed_at = datetime('now'),
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn set_materialized(
        pool: &SqlitePool,
        id: &str,
        workflow_id: &str,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                status = 'materialized',
                materialized_workflow_id = ?2,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(workflow_id)
        .execute(pool)
        .await?;
        Ok(())
    }
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PlanningDraftMessage {
    pub id: String,
    pub draft_id: String,
    pub role: String,
    pub content: String,
    pub created_at: DateTime<Utc>,
}

impl PlanningDraftMessage {
    pub fn new(draft_id: &str, role: &str, content: &str) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            draft_id: draft_id.to_string(),
            role: role.to_string(),
            content: content.to_string(),
            created_at: Utc::now(),
        }
    }

    pub async fn insert(pool: &SqlitePool, message: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"
            INSERT INTO planning_draft_message (id, draft_id, role, content, created_at)
            VALUES (?1, ?2, ?3, ?4, ?5)
            ",
        )
        .bind(&message.id)
        .bind(&message.draft_id)
        .bind(&message.role)
        .bind(&message.content)
        .bind(message.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn list_by_draft(pool: &SqlitePool, draft_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM planning_draft_message WHERE draft_id = ?1 ORDER BY created_at ASC",
        )
        .bind(draft_id)
        .fetch_all(pool)
        .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn planning_draft_new_defaults_to_gathering() {
        let draft = PlanningDraft::new(Uuid::new_v4(), "Test Draft");
        assert_eq!(draft.status, "gathering");
        assert_eq!(draft.name, "Test Draft");
        assert!(draft.requirement_summary.is_none());
        assert!(draft.technical_spec.is_none());
        assert!(draft.workflow_seed.is_none());
        assert!(draft.confirmed_at.is_none());
        assert!(draft.materialized_workflow_id.is_none());
    }

    #[test]
    fn planning_draft_status_constants_include_all_states() {
        assert!(PLANNING_DRAFT_STATUSES.contains(&"gathering"));
        assert!(PLANNING_DRAFT_STATUSES.contains(&"spec_ready"));
        assert!(PLANNING_DRAFT_STATUSES.contains(&"confirmed"));
        assert!(PLANNING_DRAFT_STATUSES.contains(&"materialized"));
        assert!(PLANNING_DRAFT_STATUSES.contains(&"cancelled"));
    }

    #[test]
    fn planning_draft_message_new_sets_role_and_content() {
        let msg = PlanningDraftMessage::new("draft-123", "user", "Build a blog");
        assert_eq!(msg.draft_id, "draft-123");
        assert_eq!(msg.role, "user");
        assert_eq!(msg.content, "Build a blog");
    }
}
