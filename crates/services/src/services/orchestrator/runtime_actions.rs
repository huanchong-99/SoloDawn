//! Runtime actions for agent-planned workflows.
//!
//! This service owns the database and PTY lifecycle operations required by
//! dynamic orchestration instructions such as creating tasks/terminals or
//! launching terminals after a workflow has already started.

use std::{path::PathBuf, sync::Arc};

use anyhow::{Result, anyhow};
use chrono::Utc;
use db::{
    DBService,
    models::{
        self,
        cli_type::{CliType, ModelConfig},
        project::Project,
        terminal::{Terminal, TerminalStatus},
        workflow::WorkflowTask,
    },
};
use uuid::Uuid;

use crate::{
    services::{
        cc_switch::CCSwitchService,
        orchestrator::{BusMessage, SharedMessageBus},
        terminal::{PromptWatcher, launcher::TerminalLauncher, process::ProcessManager},
    },
    utils::generate_task_branch_name,
};

const STARTABLE_TERMINAL_STATUSES: [&str; 5] =
    ["not_started", "failed", "cancelled", "waiting", "working"];

#[derive(Debug, Clone)]
pub struct RuntimeTaskSpec {
    pub task_id: Option<String>,
    pub name: String,
    pub description: Option<String>,
    pub branch: Option<String>,
    pub order_index: Option<i32>,
}

#[derive(Debug, Clone)]
pub struct RuntimeTerminalSpec {
    pub terminal_id: Option<String>,
    pub task_id: String,
    pub cli_type_id: String,
    pub model_config_id: String,
    pub custom_base_url: Option<String>,
    pub custom_api_key: Option<String>,
    pub role: Option<String>,
    pub role_description: Option<String>,
    pub order_index: Option<i32>,
    pub auto_confirm: Option<bool>,
}

#[derive(Clone)]
pub struct RuntimeActionService {
    db: Arc<DBService>,
    message_bus: SharedMessageBus,
    process_manager: Arc<ProcessManager>,
    prompt_watcher: PromptWatcher,
}

impl RuntimeActionService {
    pub fn new(
        db: Arc<DBService>,
        message_bus: SharedMessageBus,
        process_manager: Arc<ProcessManager>,
        prompt_watcher: PromptWatcher,
    ) -> Self {
        Self {
            db,
            message_bus,
            process_manager,
            prompt_watcher,
        }
    }

    pub async fn create_task(
        &self,
        workflow_id: &str,
        spec: RuntimeTaskSpec,
    ) -> Result<WorkflowTask> {
        let workflow = models::Workflow::find_by_id(&self.db.pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))?;

        let existing_tasks = WorkflowTask::find_by_workflow(&self.db.pool, workflow_id).await?;
        let existing_branches: Vec<String> =
            existing_tasks.iter().map(|task| task.branch.clone()).collect();
        let next_order_index = existing_tasks
            .iter()
            .map(|task| task.order_index)
            .max()
            .unwrap_or(-1)
            + 1;
        let branch = spec.branch.unwrap_or_else(|| {
            generate_task_branch_name(
                workflow_id,
                &spec.name,
                &existing_branches,
            )
        });
        let task = WorkflowTask {
            id: spec.task_id.unwrap_or_else(|| Uuid::new_v4().to_string()),
            workflow_id: workflow_id.to_string(),
            vk_task_id: None,
            name: spec.name,
            description: spec.description,
            branch,
            status: "pending".to_string(),
            order_index: spec.order_index.unwrap_or(next_order_index),
            started_at: None,
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        };

        let task = WorkflowTask::create(&self.db.pool, &task).await?;
        self.publish_task_status(&workflow.id, &task.id, &task.status)
            .await?;
        Ok(task)
    }

    pub async fn create_terminal(
        &self,
        workflow_id: &str,
        spec: RuntimeTerminalSpec,
    ) -> Result<Terminal> {
        let task = WorkflowTask::find_by_id(&self.db.pool, &spec.task_id)
            .await?
            .ok_or_else(|| anyhow!("Task {} not found", spec.task_id))?;
        if task.workflow_id != workflow_id {
            return Err(anyhow!(
                "Task {} does not belong to workflow {}",
                task.id,
                workflow_id
            ));
        }

        let cli_type = CliType::find_by_id(&self.db.pool, &spec.cli_type_id)
            .await?
            .ok_or_else(|| anyhow!("CLI type {} not found", spec.cli_type_id))?;
        let model_config = ModelConfig::find_by_id(&self.db.pool, &spec.model_config_id)
            .await?
            .ok_or_else(|| anyhow!("Model config {} not found", spec.model_config_id))?;
        if model_config.cli_type_id != cli_type.id {
            return Err(anyhow!(
                "Model config {} does not belong to CLI type {}",
                model_config.id,
                cli_type.id
            ));
        }

        let existing_terminals = Terminal::find_by_task(&self.db.pool, &task.id).await?;
        let next_order_index = existing_terminals
            .iter()
            .map(|terminal| terminal.order_index)
            .max()
            .unwrap_or(-1)
            + 1;

        let mut terminal = Terminal {
            id: spec
                .terminal_id
                .unwrap_or_else(|| Uuid::new_v4().to_string()),
            workflow_task_id: task.id.clone(),
            cli_type_id: cli_type.id,
            model_config_id: model_config.id,
            custom_base_url: spec.custom_base_url,
            custom_api_key: None,
            role: spec.role,
            role_description: spec.role_description,
            order_index: spec.order_index.unwrap_or(next_order_index),
            status: TerminalStatus::NotStarted.to_string(),
            process_id: None,
            pty_session_id: None,
            session_id: None,
            execution_process_id: None,
            vk_session_id: None,
            auto_confirm: spec.auto_confirm.unwrap_or(true),
            last_commit_hash: None,
            last_commit_message: None,
            started_at: None,
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        };
        if let Some(custom_api_key) = spec.custom_api_key.as_deref() {
            terminal.set_custom_api_key(custom_api_key)?;
        }

        let terminal = Terminal::create(&self.db.pool, &terminal).await?;
        self.publish_terminal_status(workflow_id, &terminal.id, &terminal.status)
            .await?;
        Ok(terminal)
    }

    pub async fn start_terminal(&self, terminal_id: &str) -> Result<Terminal> {
        let terminal = Terminal::find_by_id(&self.db.pool, terminal_id)
            .await?
            .ok_or_else(|| anyhow!("Terminal {terminal_id} not found"))?;
        if !STARTABLE_TERMINAL_STATUSES.contains(&terminal.status.as_str()) {
            return Err(anyhow!(
                "Terminal {} cannot be started from status {}",
                terminal.id,
                terminal.status
            ));
        }
        if self.process_manager.is_running(&terminal.id).await {
            return Terminal::find_by_id(&self.db.pool, &terminal.id)
                .await?
                .ok_or_else(|| anyhow!("Terminal {} disappeared while starting", terminal.id));
        }

        let task = WorkflowTask::find_by_id(&self.db.pool, &terminal.workflow_task_id)
            .await?
            .ok_or_else(|| anyhow!("Task {} not found", terminal.workflow_task_id))?;
        let workflow_id = task.workflow_id.clone();

        Terminal::set_starting(&self.db.pool, &terminal.id).await?;
        self.publish_terminal_status(&workflow_id, &terminal.id, "starting")
            .await?;

        let working_dir = self.resolve_workflow_working_dir(&workflow_id).await?;
        let launcher = TerminalLauncher::with_message_bus(
            self.db.clone(),
            Arc::new(CCSwitchService::new(self.db.clone())),
            self.process_manager.clone(),
            working_dir,
            self.message_bus.clone(),
            self.prompt_watcher.clone(),
        );
        let launch_result = launcher.launch_terminal(&terminal).await;
        if !launch_result.success {
            return Err(anyhow!(
                "Failed to launch terminal {}: {}",
                terminal.id,
                launch_result
                    .error
                    .as_deref()
                    .unwrap_or("unknown launch failure")
            ));
        }

        let terminal = Terminal::find_by_id(&self.db.pool, terminal_id)
            .await?
            .ok_or_else(|| anyhow!("Terminal {} not found after launch", terminal_id))?;
        self.publish_terminal_status(&workflow_id, &terminal.id, &terminal.status)
            .await?;
        Ok(terminal)
    }

    pub async fn close_terminal(
        &self,
        terminal_id: &str,
        final_status: Option<&str>,
    ) -> Result<Terminal> {
        let terminal = Terminal::find_by_id(&self.db.pool, terminal_id)
            .await?
            .ok_or_else(|| anyhow!("Terminal {terminal_id} not found"))?;
        let task = WorkflowTask::find_by_id(&self.db.pool, &terminal.workflow_task_id)
            .await?
            .ok_or_else(|| anyhow!("Task {} not found", terminal.workflow_task_id))?;
        let workflow_id = task.workflow_id.clone();

        if let Some(pty_session_id) = terminal.pty_session_id.as_deref() {
            let _ = self
                .message_bus
                .publish(pty_session_id, BusMessage::Shutdown)
                .await;
        }
        if self.process_manager.is_running(&terminal.id).await {
            self.process_manager.kill_terminal(&terminal.id).await?;
        }
        self.prompt_watcher.unregister(&terminal.id).await;

        let target_status = final_status
            .map(ToOwned::to_owned)
            .unwrap_or_else(|| match terminal.status.as_str() {
                "completed" | "failed" | "cancelled" => terminal.status.clone(),
                _ => TerminalStatus::Cancelled.to_string(),
            });
        if terminal.status != target_status {
            if matches!(target_status.as_str(), "completed" | "failed" | "cancelled") {
                Terminal::set_completed(&self.db.pool, &terminal.id, &target_status).await?;
            } else {
                Terminal::update_status(&self.db.pool, &terminal.id, &target_status).await?;
            }
        }

        let terminal = Terminal::find_by_id(&self.db.pool, terminal_id)
            .await?
            .ok_or_else(|| anyhow!("Terminal {} not found after close", terminal_id))?;
        self.publish_terminal_status(&workflow_id, &terminal.id, &terminal.status)
            .await?;
        Ok(terminal)
    }

    async fn resolve_workflow_working_dir(&self, workflow_id: &str) -> Result<PathBuf> {
        let workflow = models::Workflow::find_by_id(&self.db.pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))?;
        let project = Project::find_by_id(&self.db.pool, workflow.project_id)
            .await?
            .ok_or_else(|| anyhow!("Project {} not found", workflow.project_id))?;

        if let Some(dir) = project
            .default_agent_working_dir
            .as_ref()
            .filter(|dir| !dir.trim().is_empty())
        {
            return Ok(PathBuf::from(dir));
        }

        let repo_dir = models::project_repo::ProjectRepo::find_repos_for_project(
            &self.db.pool,
            project.id,
        )
        .await?
        .into_iter()
        .map(|repo| repo.path)
        .find(|path| !path.as_os_str().is_empty());

        repo_dir.ok_or_else(|| {
            anyhow!(
                "Project {} has no default_agent_working_dir or linked repositories",
                project.id
            )
        })
    }

    async fn publish_task_status(
        &self,
        workflow_id: &str,
        task_id: &str,
        status: &str,
    ) -> Result<()> {
        self.message_bus
            .publish_workflow_event(
                workflow_id,
                BusMessage::TaskStatusUpdate {
                    workflow_id: workflow_id.to_string(),
                    task_id: task_id.to_string(),
                    status: status.to_string(),
                },
            )
            .await
            .map(|_| ())
            .map_err(|e| anyhow!("Failed to publish task status: {e}"))
    }

    async fn publish_terminal_status(
        &self,
        workflow_id: &str,
        terminal_id: &str,
        status: &str,
    ) -> Result<()> {
        self.message_bus
            .publish_workflow_event(
                workflow_id,
                BusMessage::TerminalStatusUpdate {
                    workflow_id: workflow_id.to_string(),
                    terminal_id: terminal_id.to_string(),
                    status: status.to_string(),
                },
            )
            .await
            .map(|_| ())
            .map_err(|e| anyhow!("Failed to publish terminal status: {e}"))
    }
}
