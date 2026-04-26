use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::SqlitePool;

#[derive(Debug, Clone, Serialize, Deserialize, sqlx::FromRow)]
pub struct WorkflowEvent {
    pub id: String,
    pub workflow_id: String,
    pub event_type: String,
    pub summary: String,
    pub metadata: Option<String>,
    pub created_at: DateTime<Utc>,
}

impl WorkflowEvent {
    pub async fn insert(pool: &SqlitePool, event: &Self) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO workflow_event (id, workflow_id, event_type, summary, metadata, created_at)
              VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
        )
        .bind(&event.id)
        .bind(&event.workflow_id)
        .bind(&event.event_type)
        .bind(&event.summary)
        .bind(&event.metadata)
        .bind(event.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_workflow(pool: &SqlitePool, workflow_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM workflow_event WHERE workflow_id = ?1 ORDER BY created_at ASC",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }
}
