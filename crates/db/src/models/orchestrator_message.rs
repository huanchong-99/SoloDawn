//! Orchestrator chat persistence models.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowOrchestratorMessage {
    pub id: String,
    pub workflow_id: String,
    pub command_id: Option<String>,
    pub role: String,
    pub content: String,
    pub source: String,
    pub external_message_id: Option<String>,
    pub created_at: DateTime<Utc>,
}

impl WorkflowOrchestratorMessage {
    pub fn new(
        workflow_id: &str,
        command_id: Option<&str>,
        role: &str,
        content: &str,
        source: &str,
        external_message_id: Option<&str>,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            workflow_id: workflow_id.to_string(),
            command_id: command_id.map(ToString::to_string),
            role: role.to_string(),
            content: content.to_string(),
            source: source.to_string(),
            external_message_id: external_message_id.map(ToString::to_string),
            created_at: Utc::now(),
        }
    }

    pub async fn insert(pool: &SqlitePool, message: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r#"
            INSERT INTO workflow_orchestrator_message (
                id, workflow_id, command_id, role, content, source, external_message_id, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            "#,
        )
        .bind(&message.id)
        .bind(&message.workflow_id)
        .bind(&message.command_id)
        .bind(&message.role)
        .bind(&message.content)
        .bind(&message.source)
        .bind(&message.external_message_id)
        .bind(message.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn list_by_workflow_paginated(
        pool: &SqlitePool,
        workflow_id: &str,
        cursor: usize,
        limit: usize,
    ) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, WorkflowOrchestratorMessage>(
            r#"
            SELECT *
            FROM workflow_orchestrator_message
            WHERE workflow_id = ?1
            ORDER BY created_at ASC, id ASC
            LIMIT ?2 OFFSET ?3
            "#,
        )
        .bind(workflow_id)
        .bind(limit as i64)
        .bind(cursor as i64)
        .fetch_all(pool)
        .await
    }
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowOrchestratorCommand {
    pub id: String,
    pub workflow_id: String,
    pub source: String,
    pub external_message_id: Option<String>,
    pub request_message: String,
    pub status: String,
    pub error: Option<String>,
    pub retryable: bool,
    pub started_at: Option<DateTime<Utc>>,
    pub completed_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl WorkflowOrchestratorCommand {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        id: &str,
        workflow_id: &str,
        source: &str,
        external_message_id: Option<&str>,
        request_message: &str,
        status: &str,
        error: Option<&str>,
        retryable: bool,
    ) -> Self {
        let now = Utc::now();
        Self {
            id: id.to_string(),
            workflow_id: workflow_id.to_string(),
            source: source.to_string(),
            external_message_id: external_message_id.map(ToString::to_string),
            request_message: request_message.to_string(),
            status: status.to_string(),
            error: error.map(ToString::to_string),
            retryable,
            started_at: Some(now),
            completed_at: Some(now),
            created_at: now,
            updated_at: now,
        }
    }

    pub async fn insert(pool: &SqlitePool, command: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r#"
            INSERT INTO workflow_orchestrator_command (
                id, workflow_id, source, external_message_id, request_message, status,
                error, retryable, started_at, completed_at, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
            "#,
        )
        .bind(&command.id)
        .bind(&command.workflow_id)
        .bind(&command.source)
        .bind(&command.external_message_id)
        .bind(&command.request_message)
        .bind(&command.status)
        .bind(&command.error)
        .bind(command.retryable)
        .bind(command.started_at)
        .bind(command.completed_at)
        .bind(command.created_at)
        .bind(command.updated_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_external_message(
        pool: &SqlitePool,
        workflow_id: &str,
        source: &str,
        external_message_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, WorkflowOrchestratorCommand>(
            r#"
            SELECT *
            FROM workflow_orchestrator_command
            WHERE workflow_id = ?1
              AND source = ?2
              AND external_message_id = ?3
            ORDER BY created_at DESC
            LIMIT 1
            "#,
        )
        .bind(workflow_id)
        .bind(source)
        .bind(external_message_id)
        .fetch_optional(pool)
        .await
    }
}

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExternalConversationBinding {
    pub id: String,
    pub provider: String,
    pub conversation_id: String,
    pub workflow_id: String,
    pub created_by: Option<String>,
    pub is_active: bool,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl ExternalConversationBinding {
    pub async fn upsert(
        pool: &SqlitePool,
        provider: &str,
        conversation_id: &str,
        workflow_id: &str,
        created_by: Option<&str>,
    ) -> sqlx::Result<()> {
        let id = Uuid::new_v4().to_string();
        sqlx::query(
            r#"
            INSERT INTO external_conversation_binding (
                id, provider, conversation_id, workflow_id, created_by, is_active, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, 1, datetime('now'), datetime('now'))
            ON CONFLICT(provider, conversation_id)
            DO UPDATE SET
                workflow_id = excluded.workflow_id,
                created_by = excluded.created_by,
                is_active = 1,
                updated_at = datetime('now')
            "#,
        )
        .bind(id)
        .bind(provider)
        .bind(conversation_id)
        .bind(workflow_id)
        .bind(created_by)
        .execute(pool)
        .await?;
        Ok(())
    }
}
