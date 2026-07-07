//! Planning draft models for orchestrated workspace mode.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
#[allow(clippy::struct_excessive_bools)]
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
    pub gates_confirmed_at: Option<DateTime<Utc>>,
    pub materialized_workflow_id: Option<String>,
    /// Rounds: the draft this one continues (NULL for round 1 / standalone).
    /// A conversation is a linear chain of drafts linked by this pointer; each
    /// round keeps the 1:1 draft→workflow invariant.
    pub parent_draft_id: Option<String>,
    pub feishu_sync: bool,
    pub feishu_chat_id: Option<String>,
    /// Push tool call events to Feishu when true.
    pub sync_tools: bool,
    /// Push terminal status changes to Feishu when true.
    pub sync_terminal: bool,
    /// Push workflow progress events to Feishu when true.
    pub sync_progress: bool,
    /// Send completion report on workflow/task completion.
    pub notify_on_completion: bool,
    /// JSON-serialized AuditPlan generated at confirm time.
    pub audit_plan: Option<String>,
    /// Audit mode: "builtin" | "merged" | "custom"
    pub audit_mode: String,
    /// Path to user-uploaded audit document on disk.
    pub audit_doc_path: Option<String>,
    /// Selected design style slug; NULL falls back to the system default
    /// (`system_settings` key `default_design_style`), then to none.
    pub design_style_slug: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Valid status transitions: gathering -> spec_ready -> confirmed -> materialized
pub const PLANNING_DRAFT_STATUSES: [&str; 5] = [
    "gathering",
    "spec_ready",
    "confirmed",
    "materialized",
    "cancelled",
];

impl PlanningDraft {
    /// Encrypt and store the planner API key.
    pub fn set_api_key(&mut self, plaintext: &str) -> anyhow::Result<()> {
        let encrypted = crate::encryption::encrypt(plaintext)?;
        tracing::debug!(
            encrypted_len = encrypted.len(),
            "API key encrypted successfully"
        );
        self.planner_api_key = Some(encrypted);
        Ok(())
    }

    /// Decrypt and return the planner API key.
    pub fn get_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.planner_api_key {
            None => Ok(None),
            Some(encoded) => {
                let decrypted = crate::encryption::decrypt(encoded)?;
                tracing::debug!(key_len = decrypted.len(), "API key decrypted successfully");
                Ok(Some(decrypted))
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
            gates_confirmed_at: None,
            materialized_workflow_id: None,
            parent_draft_id: None,
            feishu_sync: false,
            feishu_chat_id: None,
            sync_tools: false,
            sync_terminal: false,
            sync_progress: false,
            notify_on_completion: true,
            audit_plan: None,
            audit_mode: "builtin".to_string(),
            audit_doc_path: None,
            design_style_slug: None,
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
                confirmed_at, gates_confirmed_at, materialized_workflow_id, parent_draft_id,
                feishu_sync, feishu_chat_id,
                sync_tools, sync_terminal, sync_progress, notify_on_completion,
                audit_plan, audit_mode, audit_doc_path, design_style_slug,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, ?24, ?25, ?26, ?27)
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
        .bind(draft.gates_confirmed_at)
        .bind(&draft.materialized_workflow_id)
        .bind(&draft.parent_draft_id)
        .bind(draft.feishu_sync)
        .bind(&draft.feishu_chat_id)
        .bind(draft.sync_tools)
        .bind(draft.sync_terminal)
        .bind(draft.sync_progress)
        .bind(draft.notify_on_completion)
        .bind(&draft.audit_plan)
        .bind(&draft.audit_mode)
        .bind(&draft.audit_doc_path)
        .bind(&draft.design_style_slug)
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

    pub async fn find_by_materialized_workflow(
        pool: &SqlitePool,
        workflow_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM planning_draft WHERE materialized_workflow_id = ?1 LIMIT 1",
        )
        .bind(workflow_id)
        .fetch_optional(pool)
        .await
    }

    /// The follow-up round continuing this draft, if any (linear chain: at
    /// most one child per draft is created through the continue endpoint).
    pub async fn find_child(pool: &SqlitePool, parent_id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>(
            "SELECT * FROM planning_draft WHERE parent_draft_id = ?1 ORDER BY created_at ASC LIMIT 1",
        )
        .bind(parent_id)
        .fetch_optional(pool)
        .await
    }

    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM planning_draft ORDER BY created_at DESC")
            .fetch_all(pool)
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

    /// Set or clear the draft's design style selection.
    pub async fn update_design_style(
        pool: &SqlitePool,
        id: &str,
        design_style_slug: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                design_style_slug = ?2,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(design_style_slug)
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

    /// Stamp the quality-gate confirmation timestamp.
    ///
    /// Distinct from `set_confirmed`: this updates ONLY `gates_confirmed_at`
    /// (System-A quality-gate confirm) and never touches `confirmed_at`
    /// (System-B audit confirm) or `status`.
    pub async fn set_gates_confirmed(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                gates_confirmed_at = datetime('now'),
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

    /// Hard-delete a planning draft (and its messages).
    ///
    /// `planning_draft_message` rows are removed automatically via
    /// `ON DELETE CASCADE`. Child rounds that continue this draft via
    /// `parent_draft_id` (a self-referential FK with no cascade) are detached
    /// first so the constraint does not block the delete — those rounds become
    /// standalone rather than being deleted. The `materialized_workflow_id`
    /// pointer is `ON DELETE SET NULL` from the workflow side and is unaffected
    /// here, so deleting a materialized draft leaves its workflow intact.
    ///
    /// Returns the number of draft rows deleted (0 if the id did not exist).
    pub async fn delete(pool: &SqlitePool, id: &str) -> sqlx::Result<u64> {
        sqlx::query("UPDATE planning_draft SET parent_draft_id = NULL WHERE parent_draft_id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        let result = sqlx::query("DELETE FROM planning_draft WHERE id = ?1")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn update_feishu_sync(
        pool: &SqlitePool,
        id: &str,
        enabled: bool,
        chat_id: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            "UPDATE planning_draft SET feishu_sync = ?2, feishu_chat_id = ?3, updated_at = datetime('now') WHERE id = ?1",
        )
        .bind(id)
        .bind(enabled)
        .bind(chat_id)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Update the audit plan, mode, and doc path for a planning draft.
    pub async fn update_audit_plan(
        pool: &SqlitePool,
        id: &str,
        audit_plan: Option<&str>,
        audit_mode: &str,
        audit_doc_path: Option<&str>,
    ) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                audit_plan = ?2,
                audit_mode = ?3,
                audit_doc_path = ?4,
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
        .bind(audit_plan)
        .bind(audit_mode)
        .bind(audit_doc_path)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Clear the audit document path and reset audit mode to "builtin".
    pub async fn delete_audit_doc(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query(
            r"
            UPDATE planning_draft SET
                audit_doc_path = NULL,
                audit_mode = 'builtin',
                updated_at = datetime('now')
            WHERE id = ?1
            ",
        )
        .bind(id)
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

    #[tokio::test]
    async fn parent_draft_id_roundtrip_and_find_child() {
        let pool = sqlx::SqlitePool::connect("sqlite::memory:").await.unwrap();
        crate::run_migrations(&pool).await.unwrap();
        let project_id = Uuid::new_v4();
        crate::models::project::Project::create(
            &pool,
            &crate::models::project::CreateProject {
                name: "rounds-test".to_string(),
                repositories: vec![],
            },
            project_id,
        )
        .await
        .unwrap();

        // Round 1: no parent.
        let parent = PlanningDraft::new(project_id, "memo app");
        PlanningDraft::insert(&pool, &parent).await.unwrap();
        let loaded = PlanningDraft::find_by_id(&pool, &parent.id)
            .await
            .unwrap()
            .unwrap();
        assert!(loaded.parent_draft_id.is_none());
        assert!(PlanningDraft::find_child(&pool, &parent.id)
            .await
            .unwrap()
            .is_none());

        // Round 2: linked to round 1 via parent_draft_id.
        let mut child = PlanningDraft::new(project_id, "memo app");
        child.parent_draft_id = Some(parent.id.clone());
        PlanningDraft::insert(&pool, &child).await.unwrap();

        let loaded_child = PlanningDraft::find_by_id(&pool, &child.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(loaded_child.parent_draft_id.as_deref(), Some(parent.id.as_str()));

        let found_child = PlanningDraft::find_child(&pool, &parent.id)
            .await
            .unwrap()
            .expect("child draft should be discoverable from its parent");
        assert_eq!(found_child.id, child.id);
    }

    #[tokio::test]
    async fn delete_cascades_messages_and_detaches_child_rounds() {
        let pool = sqlx::SqlitePool::connect("sqlite::memory:").await.unwrap();
        crate::run_migrations(&pool).await.unwrap();
        let project_id = Uuid::new_v4();
        crate::models::project::Project::create(
            &pool,
            &crate::models::project::CreateProject {
                name: "delete-test".to_string(),
                repositories: vec![],
            },
            project_id,
        )
        .await
        .unwrap();

        // Parent draft with a message, plus a child round linked by parent_draft_id.
        let parent = PlanningDraft::new(project_id, "memo app");
        PlanningDraft::insert(&pool, &parent).await.unwrap();
        PlanningDraftMessage::insert(
            &pool,
            &PlanningDraftMessage::new(&parent.id, "user", "Build a memo app"),
        )
        .await
        .unwrap();

        let mut child = PlanningDraft::new(project_id, "memo app v2");
        child.parent_draft_id = Some(parent.id.clone());
        PlanningDraft::insert(&pool, &child).await.unwrap();

        // Delete the parent.
        let rows = PlanningDraft::delete(&pool, &parent.id).await.unwrap();
        assert_eq!(rows, 1, "exactly one draft row deleted");

        // Parent is gone and its messages cascaded away.
        assert!(PlanningDraft::find_by_id(&pool, &parent.id)
            .await
            .unwrap()
            .is_none());
        assert!(PlanningDraftMessage::list_by_draft(&pool, &parent.id)
            .await
            .unwrap()
            .is_empty());

        // Child survives the delete but is detached from the removed parent, so
        // the self-referential FK does not block the delete.
        let surviving_child = PlanningDraft::find_by_id(&pool, &child.id)
            .await
            .unwrap()
            .expect("child round should outlive its deleted parent");
        assert!(surviving_child.parent_draft_id.is_none());

        // Deleting an unknown id is a no-op that reports zero rows.
        let rows = PlanningDraft::delete(&pool, "does-not-exist").await.unwrap();
        assert_eq!(rows, 0);
    }
}
