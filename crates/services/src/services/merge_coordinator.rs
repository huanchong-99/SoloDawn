//! Merge Coordinator Service
//!
//! Coordinates merging of task branches into the base branch with conflict detection.

use std::{collections::HashMap, path::Path, sync::Arc};

use anyhow::Result;
use tokio::sync::{Mutex, OwnedMutexGuard, RwLock};

use crate::services::{
    git::GitService,
    orchestrator::{
        constants::WORKFLOW_TOPIC_PREFIX,
        message_bus::{BusMessage, SharedMessageBus},
    },
};

/// Per-workflow merge lock registry.
///
/// Ensures that auto-merge (triggered by the orchestrator) and manual merge
/// (triggered via the REST API) cannot run concurrently for the same workflow.
/// G06-002: `Arc<Mutex<()>>` is used per workflow_id; the outer Mutex protects
/// the HashMap itself (held only briefly to clone the Arc).
type WorkflowMergeLocks = Arc<std::sync::Mutex<HashMap<String, Arc<Mutex<()>>>>>;

/// Returns the global per-workflow merge lock registry.
fn merge_locks() -> &'static WorkflowMergeLocks {
    use once_cell::sync::Lazy;
    static LOCKS: Lazy<WorkflowMergeLocks> =
        Lazy::new(|| Arc::new(std::sync::Mutex::new(HashMap::new())));
    &LOCKS
}

/// Acquires the per-workflow merge lock.
///
/// Returns an `OwnedMutexGuard` that releases the lock when dropped.
/// Use `let _guard = acquire_workflow_merge_lock(workflow_id).await` to hold the
/// lock for the duration of the merge operation.
///
/// G06-002: ensures auto-merge and manual merge cannot run concurrently for the
/// same workflow.
pub async fn acquire_workflow_merge_lock(workflow_id: &str) -> OwnedMutexGuard<()> {
    // Retrieve or create the per-workflow Mutex.
    let lock_arc: Arc<Mutex<()>> = {
        let mut map = merge_locks().lock().expect("merge lock map poisoned");
        map.entry(workflow_id.to_string())
            .or_insert_with(|| Arc::new(Mutex::new(())))
            .clone()
    };

    // `lock_owned()` returns an OwnedMutexGuard<()> which holds an Arc<Mutex<()>>
    // internally — no lifetime issues.
    lock_arc.lock_owned().await
}

/// Coordinates merging of completed task branches into the base branch.
///
/// The MergeCoordinator handles the final step of workflow execution:
/// merging all successfully completed task branches into the target branch.
/// It performs squash merges, detects conflicts, and updates workflow status.
pub struct MergeCoordinator {
    db: Arc<db::DBService>,
    message_bus: SharedMessageBus,
    git_service: Arc<RwLock<GitService>>,
}

impl MergeCoordinator {
    /// Creates a new MergeCoordinator.
    pub fn new(
        db: Arc<db::DBService>,
        message_bus: SharedMessageBus,
        git_service: GitService,
    ) -> Self {
        Self {
            db,
            message_bus,
            git_service: Arc::new(RwLock::new(git_service)),
        }
    }

    /// Merges a task branch into the base branch.
    ///
    /// Performs a squash merge of the task branch into the target branch.
    /// If conflicts are detected, updates workflow status to "merging" and
    /// returns an error.
    ///
    /// # Arguments
    /// * `task_id` - The ID of the task whose branch to merge
    /// * `workflow_id` - The ID of the workflow
    /// * `task_branch` - The name of the task branch
    /// * `target_branch` - The name of the target branch (e.g., "main")
    /// * `base_repo_path` - Path to the base repository
    /// * `task_worktree_path` - Path to the task worktree
    /// * `commit_message` - Commit message for the merge
    ///
    /// # Returns
    /// * `Ok(String)` - The commit SHA of the merge commit
    /// * `Err(anyhow::Error)` - If merge fails or conflicts are detected
    pub async fn merge_task_branch(
        &self,
        task_id: &str,
        workflow_id: &str,
        task_branch: &str,
        target_branch: &str,
        base_repo_path: &Path,
        task_worktree_path: &Path,
        commit_message: &str,
    ) -> Result<String> {
        // G06-002: acquire the per-workflow merge lock internally so callers
        // don't need to remember. The lock is held for the duration of the merge.
        let _merge_guard = acquire_workflow_merge_lock(workflow_id).await;

        tracing::info!(
            "Merging task branch {} into {} for task {}",
            task_branch,
            target_branch,
            task_id
        );

        // Acquire git service lock and perform merge
        let commit_sha = {
            let git_service = self.git_service.write().await;
            git_service.merge_changes(
                base_repo_path,
                task_worktree_path,
                task_branch,
                target_branch,
                commit_message,
            )
        };

        match commit_sha {
            Ok(sha) => {
                tracing::info!(
                    "Successfully merged task branch {} into {}: {}",
                    task_branch,
                    target_branch,
                    sha
                );

                // Broadcast merge success event (don't set workflow completed per-task;
                // the caller should set it after all tasks are merged — G06-007)
                self.broadcast_merge_success(workflow_id, task_id, &sha, false)
                    .await?;

                Ok(sha)
            }
            Err(e) => {
                // Check if error is due to merge conflicts
                let is_conflict =
                    matches!(e, crate::services::git::GitServiceError::MergeConflicts(_));

                if is_conflict {
                    tracing::warn!(
                        "Merge conflicts detected merging {} into {}",
                        task_branch,
                        target_branch
                    );

                    // Handle conflict state
                    self.handle_merge_conflict(workflow_id, task_id, &e.to_string())
                        .await?;

                    return Err(anyhow::anyhow!("Merge conflicts detected: {e}"));
                }

                // Other error - broadcast failure
                tracing::error!("Merge failed for task branch {}: {}", task_branch, e);

                self.broadcast_merge_failure(workflow_id, task_id, &e.to_string())
                    .await?;

                Err(anyhow::anyhow!("Merge failed: {e}"))
            }
        }
    }

    /// Handles merge conflict by updating workflow status.
    ///
    /// Sets the workflow status to "merging" to indicate that manual
    /// conflict resolution is needed.
    ///
    /// # Arguments
    /// * `workflow_id` - The ID of the workflow
    /// * `task_id` - The ID of the task that caused the conflict
    /// * `error_message` - Description of the conflict
    async fn handle_merge_conflict(
        &self,
        workflow_id: &str,
        task_id: &str,
        error_message: &str,
    ) -> Result<()> {
        tracing::warn!(
            "Handling merge conflict for workflow {} task {}: {}",
            workflow_id,
            task_id,
            error_message
        );

        // Update workflow status to "merging"
        db::models::Workflow::update_status(&self.db.pool, workflow_id, "merging").await?;

        // Broadcast status update
        let message = BusMessage::StatusUpdate {
            workflow_id: workflow_id.to_string(),
            status: "merging".to_string(),
        };
        let topic = format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}");
        self.message_bus.publish(&topic, message).await?;

        tracing::info!(
            "Workflow {} status updated to 'merging' due to conflicts",
            workflow_id
        );

        Ok(())
    }

    /// Completes a merge after conflict resolution.
    ///
    /// Called when conflicts have been manually resolved and the merge
    /// should be completed.
    ///
    /// # Arguments
    /// * `workflow_id` - The ID of the workflow
    /// * `task_id` - The ID of the task
    /// * `commit_sha` - The commit SHA of the resolved merge
    pub async fn resolve_and_complete_merge(
        &self,
        workflow_id: &str,
        task_id: &str,
        commit_sha: &str,
    ) -> Result<()> {
        tracing::info!(
            "Completing resolved merge for workflow {} task {}: {}",
            workflow_id,
            task_id,
            commit_sha
        );

        // Broadcast merge completion
        self.broadcast_merge_success(workflow_id, task_id, commit_sha, true)
            .await?;

        tracing::info!("Successfully completed resolved merge for task {}", task_id);

        Ok(())
    }

    /// Broadcasts a merge success event.
    ///
    /// # Arguments
    /// * `workflow_id` - The ID of the workflow
    /// * `task_id` - The ID of the task
    /// * `commit_sha` - The commit SHA of the merge
    /// * `set_workflow_completed` - Whether to set the workflow status to "completed".
    ///   Callers should pass `false` when merging individual tasks and only pass `true`
    ///   after all tasks have been merged successfully.
    async fn broadcast_merge_success(
        &self,
        workflow_id: &str,
        task_id: &str,
        _commit_sha: &str,
        set_workflow_completed: bool,
    ) -> Result<()> {
        tracing::debug!(
            "Broadcasting merge success for workflow {} task {}",
            workflow_id,
            task_id
        );

        if set_workflow_completed {
            // Update workflow status to "completed"
            db::models::Workflow::update_status(&self.db.pool, workflow_id, "completed").await?;

            // Publish success event
            let message = BusMessage::StatusUpdate {
                workflow_id: workflow_id.to_string(),
                status: "completed".to_string(),
            };
            let topic = format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}");
            self.message_bus.publish(&topic, message).await?;
        }

        Ok(())
    }

    /// Broadcasts a merge failure event.
    ///
    /// # Arguments
    /// * `workflow_id` - The ID of the workflow
    /// * `task_id` - The ID of the task
    /// * `error_message` - Description of the failure
    async fn broadcast_merge_failure(
        &self,
        workflow_id: &str,
        task_id: &str,
        error_message: &str,
    ) -> Result<()> {
        tracing::error!(
            "Broadcasting merge failure for workflow {} task {}: {}",
            workflow_id,
            task_id,
            error_message
        );

        // Update workflow status to "failed"
        db::models::Workflow::update_status(&self.db.pool, workflow_id, "failed").await?;

        // Publish error event
        let message = BusMessage::Error {
            workflow_id: workflow_id.to_string(),
            error: error_message.to_string(),
        };
        let topic = format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}");
        self.message_bus.publish(&topic, message).await?;

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn test_merge_coordinator_creation() {
        // This is a placeholder test to verify the module compiles
        // Real tests are in merge_coordinator_test.rs
    }
}
