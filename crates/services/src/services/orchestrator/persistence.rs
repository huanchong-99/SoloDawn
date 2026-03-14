//! State Persistence and Recovery
//!
//! Provides persistence mechanisms for orchestrator state to enable recovery from crashes.

use std::{collections::HashMap, sync::Arc};

use anyhow::{Result, anyhow};
use chrono::{DateTime, Utc};
use db::DBService;
use serde::{Deserialize, Serialize};
use tracing::{debug, info, warn};

use super::{
    constants::WORKFLOW_STATUS_RUNNING,
    state::{OrchestratorState, TaskExecutionState},
    types::LLMMessage,
};

/// Persisted orchestrator state
///
/// Serializable version of OrchestratorState for database storage.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PersistedState {
    /// Workflow identifier
    pub workflow_id: String,

    /// Per-task execution state
    pub task_states: HashMap<String, PersistedTaskState>,

    /// Whether workflow planning has finished and no more tasks should be added.
    #[serde(default = "default_true")]
    pub workflow_planning_complete: bool,

    /// Conversation history for LLM context
    pub conversation_history: Vec<LLMMessage>,

    /// Total tokens consumed by the LLM
    pub total_tokens_used: i64,

    /// Total error count for this workflow run
    pub error_count: u32,

    /// Last update timestamp
    pub updated_at: DateTime<Utc>,
}

/// Persisted task execution state
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PersistedTaskState {
    pub task_id: String,
    pub current_terminal_index: usize,
    pub total_terminals: usize,
    #[serde(default)]
    pub terminal_ids: Vec<String>,
    pub completed_terminals: Vec<String>,
    pub failed_terminals: Vec<String>,
    #[serde(default = "default_true")]
    pub planning_complete: bool,
    pub is_completed: bool,
}

fn default_true() -> bool {
    true
}

impl From<TaskExecutionState> for PersistedTaskState {
    fn from(state: TaskExecutionState) -> Self {
        Self {
            task_id: state.task_id,
            current_terminal_index: state.current_terminal_index,
            total_terminals: state.total_terminals,
            terminal_ids: state.terminal_ids,
            completed_terminals: state.completed_terminals,
            failed_terminals: state.failed_terminals,
            planning_complete: state.planning_complete,
            is_completed: state.is_completed,
        }
    }
}

impl From<PersistedTaskState> for TaskExecutionState {
    fn from(state: PersistedTaskState) -> Self {
        Self {
            task_id: state.task_id,
            current_terminal_index: state.current_terminal_index,
            total_terminals: state.total_terminals,
            terminal_ids: state.terminal_ids,
            completed_terminals: state.completed_terminals,
            failed_terminals: state.failed_terminals,
            planning_complete: state.planning_complete,
            is_completed: state.is_completed,
        }
    }
}

impl From<OrchestratorState> for PersistedState {
    fn from(state: OrchestratorState) -> Self {
        Self {
            workflow_id: state.workflow_id,
            task_states: state
                .task_states
                .into_iter()
                .map(|(k, v)| (k, v.into()))
                .collect(),
            workflow_planning_complete: state.workflow_planning_complete,
            conversation_history: state.conversation_history,
            total_tokens_used: state.total_tokens_used,
            error_count: state.error_count,
            updated_at: Utc::now(),
        }
    }
}

impl From<&OrchestratorState> for PersistedState {
    fn from(state: &OrchestratorState) -> Self {
        Self {
            workflow_id: state.workflow_id.clone(),
            task_states: state
                .task_states
                .iter()
                .map(|(k, v)| (k.clone(), v.clone().into()))
                .collect(),
            workflow_planning_complete: state.workflow_planning_complete,
            conversation_history: state.conversation_history.clone(),
            total_tokens_used: state.total_tokens_used,
            error_count: state.error_count,
            updated_at: Utc::now(),
        }
    }
}

/// State Persistence Service
///
/// Manages persistence and recovery of orchestrator state.
#[derive(Clone)]
pub struct StatePersistence {
    db: Arc<DBService>,
}

impl StatePersistence {
    /// Create a new state persistence service
    pub fn new(db: Arc<DBService>) -> Self {
        Self { db }
    }

    /// Save orchestrator state
    ///
    /// Persists the current orchestrator state to the database.
    pub async fn save_state(&self, state: &OrchestratorState) -> Result<()> {
        let workflow_id = &state.workflow_id;
        let persisted: PersistedState = state.to_owned().into();
        let state_json = serde_json::to_string(&persisted)
            .map_err(|e| anyhow!("Failed to serialize state: {e}"))?;

        debug!("Saving state for workflow {}", workflow_id);

        // Store state in workflow metadata or a dedicated table
        // For now, we'll use a simple approach: update workflow with state JSON
        let query = r"
            UPDATE workflow
            SET orchestrator_state = ?1, updated_at = ?2
            WHERE id = ?3
        ";

        let now = Utc::now();
        sqlx::query(query)
            .bind(&state_json)
            .bind(now)
            .bind(workflow_id)
            .execute(&self.db.pool)
            .await
            .map_err(|e| anyhow!("Failed to save state to database: {e}"))?;

        debug!("State saved successfully for workflow {}", workflow_id);

        Ok(())
    }

    /// Load orchestrator state
    ///
    /// Loads the persisted orchestrator state from the database.
    pub async fn load_state(&self, workflow_id: &str) -> Result<Option<OrchestratorState>> {
        debug!("Loading state for workflow {}", workflow_id);

        let query = r"
            SELECT orchestrator_state
            FROM workflow
            WHERE id = ?1
        ";

        let row: Option<(Option<String>,)> = sqlx::query_as(query)
            .bind(workflow_id)
            .fetch_optional(&self.db.pool)
            .await
            .map_err(|e| anyhow!("Failed to load state from database: {e}"))?;

        if let Some((Some(state_json),)) = row {
            let persisted: PersistedState = serde_json::from_str(&state_json)
                .map_err(|e| anyhow!("Failed to deserialize state: {e}"))?;

            let mut state = OrchestratorState::new(workflow_id.to_string());
            state.task_states = persisted
                .task_states
                .into_iter()
                .map(|(k, v)| (k, v.into()))
                .collect();
            state.workflow_planning_complete = persisted.workflow_planning_complete;
            state.conversation_history = persisted.conversation_history;
            state.total_tokens_used = persisted.total_tokens_used;
            state.error_count = persisted.error_count;

            debug!("State loaded successfully for workflow {}", workflow_id);

            Ok(Some(state))
        } else {
            debug!("No persisted state found for workflow {}", workflow_id);
            Ok(None)
        }
    }

    /// Recover workflow
    ///
    /// Loads and restores workflow state for recovery after a crash or restart.
    /// Returns the restored state if available.
    pub async fn recover_workflow(&self, workflow_id: &str) -> Result<Option<OrchestratorState>> {
        info!("Attempting to recover workflow {}", workflow_id);

        // Load workflow to check status
        let workflow = db::models::Workflow::find_by_id(&self.db.pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))?;

        // Only recover if workflow is in "running" state
        if workflow.status != WORKFLOW_STATUS_RUNNING {
            debug!(
                "Workflow {} is not in running state (current: {}), skipping recovery",
                workflow_id, workflow.status
            );
            return Ok(None);
        }

        // Load persisted state
        let state = self.load_state(workflow_id).await?;

        if state.is_some() {
            info!("Successfully recovered state for workflow {}", workflow_id);
        } else {
            warn!(
                "No persisted state found for running workflow {}",
                workflow_id
            );
        }

        Ok(state)
    }

    /// Clear persisted state
    ///
    /// Removes the persisted state for a workflow (e.g., after completion).
    pub async fn clear_state(&self, workflow_id: &str) -> Result<()> {
        debug!("Clearing state for workflow {}", workflow_id);

        let query = r"
            UPDATE workflow
            SET orchestrator_state = NULL, updated_at = ?1
            WHERE id = ?2
        ";

        let now = Utc::now();
        sqlx::query(query)
            .bind(now)
            .bind(workflow_id)
            .execute(&self.db.pool)
            .await
            .map_err(|e| anyhow!("Failed to clear state from database: {e}"))?;

        debug!("State cleared successfully for workflow {}", workflow_id);

        Ok(())
    }

    /// Save task execution progress
    ///
    /// **Deprecated / placeholder** — this method is a no-op.
    /// Full state is persisted via `save_state()` which includes all task
    /// states. When a dedicated `task_progress` table is introduced, this
    /// method will perform incremental upserts. Do not rely on it for
    /// correctness; it exists only to reserve the API surface.
    #[allow(unused)]
    #[deprecated(note = "no-op placeholder; use save_state() for full persistence")]
    pub async fn save_task_progress(
        &self,
        workflow_id: &str,
        task_id: &str,
        _completed_terminals: &[String],
        _failed_terminals: &[String],
    ) -> Result<()> {
        debug!(
            "Saving task progress for workflow {} task {}",
            workflow_id, task_id
        );

        // This could be stored in a dedicated task progress table
        // For now, we'll rely on save_state() which includes all task states

        Ok(())
    }

    /// Restore conversation history
    ///
    /// Loads conversation history for a workflow.
    pub async fn restore_conversation_history(&self, workflow_id: &str) -> Result<Vec<LLMMessage>> {
        debug!(
            "Restoring conversation history for workflow {}",
            workflow_id
        );

        let state = self.load_state(workflow_id).await?;

        if let Some(state) = state {
            debug!(
                "Restored {} messages from history",
                state.conversation_history.len()
            );
            Ok(state.conversation_history)
        } else {
            debug!("No conversation history found for workflow {}", workflow_id);
            Ok(Vec::new())
        }
    }
}

#[cfg(test)]
mod tests {
    // Note: Integration tests are in persistence_test.rs
}
