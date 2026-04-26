//! Git Event Model
//!
//! Persists Git commit events detected by GitWatcher for audit and status tracking.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

/// Git Event
///
/// Corresponds to database table: git_event
#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitEvent {
    pub id: String,
    pub workflow_id: String,
    pub terminal_id: Option<String>,
    pub commit_hash: String,
    pub branch: String,
    pub commit_message: String,
    pub metadata: Option<String>,
    pub process_status: String,
    pub agent_response: Option<String>,
    pub created_at: DateTime<Utc>,
    pub processed_at: Option<DateTime<Utc>>,
}

impl GitEvent {
    /// Insert a new git event record
    pub async fn insert(pool: &SqlitePool, event: &GitEvent) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO git_event (
                id, workflow_id, terminal_id, commit_hash, branch,
                commit_message, metadata, process_status, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)",
        )
        .bind(&event.id)
        .bind(&event.workflow_id)
        .bind(&event.terminal_id)
        .bind(&event.commit_hash)
        .bind(&event.branch)
        .bind(&event.commit_message)
        .bind(&event.metadata)
        .bind(&event.process_status)
        .bind(event.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update process status and optionally set agent_response
    pub async fn update_status(
        pool: &SqlitePool,
        id: &str,
        status: &str,
        agent_response: Option<&str>,
    ) -> sqlx::Result<()> {
        let processed_at = if status == "processed" || status == "failed" {
            Some(Utc::now())
        } else {
            None
        };
        sqlx::query(
            r"UPDATE git_event
            SET process_status = ?1, agent_response = ?2, processed_at = ?3
            WHERE id = ?4",
        )
        .bind(status)
        .bind(agent_response)
        .bind(processed_at)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update terminal_id and metadata after parsing
    pub async fn update_metadata(
        pool: &SqlitePool,
        id: &str,
        terminal_id: &str,
        metadata_json: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"UPDATE git_event
            SET terminal_id = ?1, metadata = ?2, process_status = 'processing'
            WHERE id = ?3",
        )
        .bind(terminal_id)
        .bind(metadata_json)
        .bind(id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Find git events by workflow ID
    pub async fn find_by_workflow(pool: &SqlitePool, workflow_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, GitEvent>(
            r"SELECT * FROM git_event WHERE workflow_id = ? ORDER BY created_at DESC",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    /// Create a new pending GitEvent instance
    pub fn new_pending(
        workflow_id: &str,
        commit_hash: &str,
        branch: &str,
        commit_message: &str,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            workflow_id: workflow_id.to_string(),
            terminal_id: None,
            commit_hash: commit_hash.to_string(),
            branch: branch.to_string(),
            commit_message: commit_message.to_string(),
            metadata: None,
            process_status: "pending".to_string(),
            agent_response: None,
            created_at: Utc::now(),
            processed_at: None,
        }
    }
}
