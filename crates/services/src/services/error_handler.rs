//! Error Handler Service
//!
//! Manages error terminal creation and activation when tasks fail.

use std::sync::Arc;

use anyhow::{Result, anyhow};
use chrono::Utc;
use db::DBService;
use tracing::{error, info};
use uuid::Uuid;

use crate::services::orchestrator::{
    constants::WORKFLOW_STATUS_FAILED,
    message_bus::{BusMessage, SharedMessageBus},
};

/// Error Handler Service
///
/// Manages error terminal creation and activation when tasks fail.
pub struct ErrorHandler {
    db: Arc<DBService>,
    message_bus: SharedMessageBus,
}

impl ErrorHandler {
    /// Create a new error handler
    pub fn new(db: Arc<DBService>, message_bus: SharedMessageBus) -> Self {
        Self { db, message_bus }
    }

    /// Handle terminal failure
    ///
    /// Updates workflow status to "failed", activates error terminal if configured,
    /// and broadcasts error events to message bus.
    pub async fn handle_terminal_failure(
        &self,
        workflow_id: &str,
        task_id: &str,
        terminal_id: &str,
        error_message: &str,
    ) -> Result<()> {
        error!(
            "Handling terminal failure: workflow={}, task={}, terminal={}, error={}",
            workflow_id, task_id, terminal_id, error_message
        );

        // 1. Update workflow status to failed
        db::models::Workflow::update_status(&self.db.pool, workflow_id, WORKFLOW_STATUS_FAILED)
            .await
            .map_err(|e| anyhow!("Failed to update workflow status: {e}"))?;

        info!("Workflow {} marked as failed", workflow_id);

        // 2. Get workflow to check if error terminal is enabled
        let workflow = db::models::Workflow::find_by_id(&self.db.pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))?;

        // 3. Activate error terminal if configured
        if workflow.error_terminal_enabled {
            self.activate_error_terminal(workflow_id, task_id, error_message)
                .await?;
        }

        // 4. Broadcast error event
        let event = BusMessage::Error {
            workflow_id: workflow_id.to_string(),
            error: error_message.to_string(),
        };

        self.message_bus
            .publish(&format!("workflow:{workflow_id}"), event)
            .await?;

        error!("Error handling complete for workflow {}", workflow_id);

        Ok(())
    }

    /// Activate error terminal
    ///
    /// Creates error terminal with role='error' if not exists,
    /// sets order_index to 999 (always last), and activates it.
    async fn activate_error_terminal(
        &self,
        workflow_id: &str,
        task_id: &str,
        error_message: &str,
    ) -> Result<()> {
        info!(
            "Activating error terminal for workflow {} task {}",
            workflow_id, task_id
        );

        // 1. Check if error terminal already exists for this workflow
        let existing_terminals =
            db::models::Terminal::find_by_workflow(&self.db.pool, workflow_id).await?;

        let error_terminal = existing_terminals
            .iter()
            .find(|t| t.role.as_deref() == Some("error"));

        if let Some(terminal) = error_terminal {
            // Error terminal exists, activate it
            info!("Found existing error terminal: {}", terminal.id);

            // Update terminal status to "waiting"
            db::models::Terminal::update_status(&self.db.pool, &terminal.id, "waiting").await?;

            // Send error message to terminal
            let pty_session_id = terminal
                .pty_session_id
                .as_ref()
                .ok_or_else(|| anyhow!("Error terminal has no PTY session"))?;

            let message = format!(
                "[ERROR] Task {task_id} failed: {error_message}\nPlease investigate and fix the error."
            );

            self.message_bus
                .publish(pty_session_id, BusMessage::TerminalMessage { message })
                .await?;

            info!("Error terminal {} activated", terminal.id);
        } else {
            // Create new error terminal
            info!("Creating new error terminal");

            // Get workflow task to associate error terminal with
            let tasks =
                db::models::WorkflowTask::find_by_workflow(&self.db.pool, workflow_id).await?;

            // Use the first task or create a task for error handling
            let task = tasks
                .first()
                .ok_or_else(|| anyhow!("No tasks found for workflow"))?;

            // Get default CLI type and model config (use system defaults)
            // For now, we'll use the first CLI type and model config we can find
            let cli_types = db::models::CliType::find_all(&self.db.pool).await?;
            let cli_type = cli_types
                .first()
                .ok_or_else(|| anyhow!("No CLI types available"))?;

            let model_configs = db::models::ModelConfig::find_all(&self.db.pool).await?;
            let model_config = model_configs
                .first()
                .ok_or_else(|| anyhow!("No model configs available"))?;

            // Create error terminal
            let terminal_id = Uuid::new_v4().to_string();
            let now = Utc::now();

            let terminal = db::models::Terminal {
                id: terminal_id.clone(),
                workflow_task_id: task.id.clone(),
                cli_type_id: cli_type.id.clone(),
                model_config_id: model_config.id.clone(),
                custom_base_url: None,
                custom_api_key: None,
                role: Some("error".to_string()),
                role_description: Some("Error investigation and fixing terminal".to_string()),
                order_index: 999, // Always last
                status: "waiting".to_string(),
                process_id: None,
                pty_session_id: None,
                session_id: None,
                execution_process_id: None,
                vk_session_id: None,
                auto_confirm: false,
                last_commit_hash: None,
                last_commit_message: None,
                started_at: Some(now),
                completed_at: None,
                created_at: now,
                updated_at: now,
            };

            db::models::Terminal::create(&self.db.pool, &terminal).await?;

            info!("Created error terminal: {}", terminal_id);

            // Note: The PTY session will be created when the terminal service starts the terminal
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    // Note: Integration tests are in error_handler_test.rs
}
