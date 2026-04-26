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
        orchestrator::{
            BusMessage, SharedMessageBus, constants::configured_max_concurrent_terminals,
        },
        terminal::{
            PromptWatcher, bridge::TerminalBridge, launcher::TerminalLauncher,
            process::ProcessManager,
        },
    },
    utils::generate_task_branch_name,
};

// [G15-007] "working" removed: a terminal in "working" status has a live PTY process.
// Re-launching it would spawn a duplicate process and corrupt orchestrator state.
// Stall-recovery must call close_terminal first (→ "cancelled") before re-launching.
const STARTABLE_TERMINAL_STATUSES: [&str; 4] = ["not_started", "failed", "cancelled", "waiting"];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum CliResolutionSource {
    Exact,
    Alias,
    Fallback,
}

#[derive(Debug, Clone)]
struct ResolvedCliType {
    cli_type: CliType,
    source: CliResolutionSource,
}

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

#[derive(Debug, Clone)]
pub enum StartTerminalOutcome {
    Started(Terminal),
    Queued(Terminal),
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

    async fn resolve_terminal_runtime_config(
        &self,
        requested_cli_type_id: &str,
        requested_model_config_id: &str,
    ) -> Result<(CliType, ModelConfig)> {
        let resolved_cli = self.resolve_cli_type(requested_cli_type_id).await?;
        let requested_model =
            ModelConfig::find_by_id(&self.db.pool, requested_model_config_id).await?;

        if let Some(model_config) = requested_model {
            if model_config.cli_type_id == resolved_cli.cli_type.id {
                return Ok((resolved_cli.cli_type, model_config));
            }

            if resolved_cli.source == CliResolutionSource::Fallback {
                let model_cli = CliType::find_by_id(&self.db.pool, &model_config.cli_type_id)
                    .await?
                    .ok_or_else(|| {
                        anyhow!(
                            "Model config {} references missing CLI type {}",
                            model_config.id,
                            model_config.cli_type_id
                        )
                    })?;
                tracing::warn!(
                    requested_cli_type_id = %requested_cli_type_id,
                    requested_model_config_id = %requested_model_config_id,
                    resolved_cli_type_id = %model_cli.id,
                    resolved_model_config_id = %model_config.id,
                    "LLM provided a role-like or unknown cli_type_id; using the valid model's CLI type"
                );
                return Ok((model_cli, model_config));
            }

            tracing::warn!(
                requested_cli_type_id = %requested_cli_type_id,
                requested_model_config_id = %requested_model_config_id,
                resolved_cli_type_id = %resolved_cli.cli_type.id,
                model_cli_type_id = %model_config.cli_type_id,
                "LLM provided mismatched cli_type_id/model_config_id; using a model that belongs to the resolved CLI type"
            );
        }

        let model_config = self
            .resolve_model_config_for_cli(&resolved_cli.cli_type.id, requested_model_config_id)
            .await?;
        Ok((resolved_cli.cli_type, model_config))
    }

    async fn resolve_cli_type(&self, requested_cli_type_id: &str) -> Result<ResolvedCliType> {
        if let Some(cli_type) = CliType::find_by_id(&self.db.pool, requested_cli_type_id).await? {
            return Ok(ResolvedCliType {
                cli_type,
                source: CliResolutionSource::Exact,
            });
        }

        if let Some(cli_type) = CliType::find_by_name(&self.db.pool, requested_cli_type_id).await? {
            tracing::warn!(
                requested_cli_type_id = %requested_cli_type_id,
                resolved_cli_type_id = %cli_type.id,
                "Resolved cli_type_id from CLI name"
            );
            return Ok(ResolvedCliType {
                cli_type,
                source: CliResolutionSource::Alias,
            });
        }

        let cli_types = CliType::find_all(&self.db.pool).await?;
        if let Some(cli_type) = cli_types
            .iter()
            .find(|cli_type| cli_hint_matches(requested_cli_type_id, cli_type))
            .cloned()
        {
            tracing::warn!(
                requested_cli_type_id = %requested_cli_type_id,
                resolved_cli_type_id = %cli_type.id,
                "Resolved cli_type_id from CLI alias"
            );
            return Ok(ResolvedCliType {
                cli_type,
                source: CliResolutionSource::Alias,
            });
        }

        if let Some((fallback_cli_id, _)) =
            ModelConfig::first_user_configured_ids(&self.db.pool).await?
        {
            if let Some(cli_type) = CliType::find_by_id(&self.db.pool, &fallback_cli_id).await? {
                tracing::warn!(
                    requested_cli_type_id = %requested_cli_type_id,
                    resolved_cli_type_id = %cli_type.id,
                    "LLM provided unknown cli_type_id; using first user-configured CLI"
                );
                return Ok(ResolvedCliType {
                    cli_type,
                    source: CliResolutionSource::Fallback,
                });
            }
        }

        if let Some(cli_type) = cli_types
            .iter()
            .find(|cli_type| cli_type.id == "cli-claude-code")
            .cloned()
        {
            tracing::warn!(
                requested_cli_type_id = %requested_cli_type_id,
                resolved_cli_type_id = %cli_type.id,
                "LLM provided unknown cli_type_id; using Claude Code fallback"
            );
            return Ok(ResolvedCliType {
                cli_type,
                source: CliResolutionSource::Fallback,
            });
        }

        if let Some(cli_type) = cli_types.into_iter().next() {
            tracing::warn!(
                requested_cli_type_id = %requested_cli_type_id,
                resolved_cli_type_id = %cli_type.id,
                "LLM provided unknown cli_type_id; using first registered CLI"
            );
            return Ok(ResolvedCliType {
                cli_type,
                source: CliResolutionSource::Fallback,
            });
        }

        Err(anyhow!(
            "No CLI types are configured; cannot resolve cli_type_id {}",
            requested_cli_type_id
        ))
    }

    async fn resolve_model_config_for_cli(
        &self,
        cli_type_id: &str,
        requested_model_config_id: &str,
    ) -> Result<ModelConfig> {
        if let Some(model_config) =
            ModelConfig::find_by_id(&self.db.pool, requested_model_config_id).await?
        {
            if model_config.cli_type_id == cli_type_id {
                return Ok(model_config);
            }
        }

        if let Some(model_config) =
            ModelConfig::find_with_credentials_for_cli(&self.db.pool, cli_type_id).await?
        {
            tracing::warn!(
                requested_model_config_id = %requested_model_config_id,
                resolved_model_config_id = %model_config.id,
                cli_type_id = %cli_type_id,
                "LLM provided missing or mismatched model_config_id; using credentialed model fallback"
            );
            return Ok(model_config);
        }

        if let Some(model_config) =
            ModelConfig::find_default_for_cli(&self.db.pool, cli_type_id).await?
        {
            tracing::warn!(
                requested_model_config_id = %requested_model_config_id,
                resolved_model_config_id = %model_config.id,
                cli_type_id = %cli_type_id,
                "LLM provided missing or mismatched model_config_id; using default model fallback"
            );
            return Ok(model_config);
        }

        let models = ModelConfig::find_by_cli_type(&self.db.pool, cli_type_id).await?;
        if let Some(model_config) = models.into_iter().next() {
            tracing::warn!(
                requested_model_config_id = %requested_model_config_id,
                resolved_model_config_id = %model_config.id,
                cli_type_id = %cli_type_id,
                "LLM provided missing or mismatched model_config_id; using registered model fallback"
            );
            return Ok(model_config);
        }

        Err(anyhow!(
            "No model configs are configured for CLI type {}; cannot resolve model_config_id {}",
            cli_type_id,
            requested_model_config_id
        ))
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
        let existing_branches: Vec<String> = existing_tasks
            .iter()
            .map(|task| task.branch.clone())
            .collect();
        let next_order_index = existing_tasks
            .iter()
            .map(|task| task.order_index)
            .max()
            .unwrap_or(-1)
            + 1;
        let branch = spec.branch.unwrap_or_else(|| {
            generate_task_branch_name(workflow_id, &spec.name, &existing_branches)
        });
        let task = WorkflowTask {
            id: spec
                .task_id
                .filter(|id| Uuid::parse_str(id).is_ok())
                .unwrap_or_else(|| Uuid::new_v4().to_string()),
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

        let (cli_type, model_config) = self
            .resolve_terminal_runtime_config(&spec.cli_type_id, &spec.model_config_id)
            .await?;

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
                .filter(|id| Uuid::parse_str(id).is_ok())
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

    pub async fn try_start_terminal(&self, terminal_id: &str) -> Result<StartTerminalOutcome> {
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
            let terminal = Terminal::find_by_id(&self.db.pool, &terminal.id)
                .await?
                .ok_or_else(|| anyhow!("Terminal {} disappeared while starting", terminal.id))?;
            return Ok(StartTerminalOutcome::Started(terminal));
        }

        let task = WorkflowTask::find_by_id(&self.db.pool, &terminal.workflow_task_id)
            .await?
            .ok_or_else(|| anyhow!("Task {} not found", terminal.workflow_task_id))?;
        let workflow_id = task.workflow_id.clone();

        let max_concurrent_terminals = configured_max_concurrent_terminals();
        let launch_slot_acquired = Terminal::try_set_starting_with_global_limit(
            &self.db.pool,
            &terminal.id,
            max_concurrent_terminals,
        )
        .await?;
        if !launch_slot_acquired {
            tracing::info!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                max_concurrent_terminals,
                "Terminal start queued because global launch limit is reached"
            );
            let terminal = Terminal::find_by_id(&self.db.pool, &terminal.id)
                .await?
                .unwrap_or(terminal);
            return Ok(StartTerminalOutcome::Queued(terminal));
        }

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
            .ok_or_else(|| anyhow!("Terminal {terminal_id} not found after launch"))?;
        self.publish_terminal_status(&workflow_id, &terminal.id, &terminal.status)
            .await?;
        Ok(StartTerminalOutcome::Started(terminal))
    }

    pub async fn start_terminal(&self, terminal_id: &str) -> Result<Terminal> {
        match self.try_start_terminal(terminal_id).await? {
            StartTerminalOutcome::Started(terminal) => Ok(terminal),
            StartTerminalOutcome::Queued(terminal) => Err(anyhow!(
                "Terminal {} queued because global launch limit is reached",
                terminal.id
            )),
        }
    }

    /// R8-B1 access shim: lets the orchestrator query "is this terminal's
    /// PTY in stop-hook shutdown right now?" without exposing the
    /// PromptWatcher field directly. Returns false on unknown terminals.
    pub async fn has_recent_stop_hook(&self, terminal_id: &str) -> bool {
        self.prompt_watcher.has_recent_stop_hook(terminal_id).await
    }

    /// R8-B2: clean-context relaunch of the SAME terminal record.
    ///
    /// Use this when a fix-prompt injection cannot succeed via the running
    /// PTY — typically because Claude Code has entered its post-turn
    /// "stop hook" shutdown sequence and the input box silently drops
    /// submit signals. Closing the existing PTY + resetting the runtime
    /// fields + starting a fresh PTY restores a usable session while
    /// preserving the per-terminal blocker history that R8-A's progress
    /// classifier depends on (terminal_id is unchanged, so previous
    /// quality_runs still count).
    ///
    /// Returns the freshly-launched Terminal record. Caller is responsible
    /// for re-delivering the fix prompt via `dispatch_terminal` (the
    /// dispatcher honours waiting/working transitions and quiet-window
    /// safety which raw `publish_terminal_input` does not).
    pub async fn relaunch_terminal_clean_context(&self, terminal_id: &str) -> Result<Terminal> {
        // Step 1: stop any running PTY + unregister bridge/prompt-watcher.
        // Use cancelled as the intermediate status so close_terminal cleans
        // up but reset_for_restart immediately wipes it back to not_started
        // for a fresh start.
        let _ = self.close_terminal(terminal_id, Some("cancelled")).await?;

        // Step 2: clear all runtime state on the row so STARTABLE_TERMINAL_STATUSES
        // accepts it again (reset_for_restart sets status='not_started' and
        // clears process/session/execution-process pointers).
        Terminal::reset_for_restart(&self.db.pool, terminal_id).await?;

        // Re-emit not_started so dashboard/event subscribers see the cycle.
        let task_workflow_id = {
            let term = Terminal::find_by_id(&self.db.pool, terminal_id)
                .await?
                .ok_or_else(|| {
                    anyhow!("Terminal {terminal_id} not found after reset_for_restart")
                })?;
            let task = WorkflowTask::find_by_id(&self.db.pool, &term.workflow_task_id)
                .await?
                .ok_or_else(|| anyhow!("Task {} not found", term.workflow_task_id))?;
            task.workflow_id
        };
        self.publish_terminal_status(&task_workflow_id, terminal_id, "not_started")
            .await?;

        // Step 3: spawn a fresh PTY via the normal start path. This re-runs
        // launcher.launch_terminal which re-applies cc-switch env, prompt-
        // watcher registration, MCP injection, etc.
        self.start_terminal(terminal_id).await
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

        // [G21-011] Publish Shutdown to both the legacy PTY-session topic AND the
        // terminal-input topic so all bridge subscribers receive the signal.
        if let Some(pty_session_id) = terminal.pty_session_id.as_deref() {
            let terminal_input_topic = format!("terminal.input.{}", terminal.id);
            let _ = self
                .message_bus
                .publish(pty_session_id, BusMessage::Shutdown)
                .await;
            let _ = self
                .message_bus
                .publish(&terminal_input_topic, BusMessage::Shutdown)
                .await;
        }
        if self.process_manager.is_running(&terminal.id).await {
            self.process_manager.kill_terminal(&terminal.id).await?;
        }
        // [G21-003] Unregister bridge to stop MessageBus -> PTY stdin forwarding.
        if let Some(pty_session_id) = terminal.pty_session_id.as_deref() {
            let bridge =
                TerminalBridge::new(self.message_bus.clone(), Arc::clone(&self.process_manager));
            bridge.unregister(pty_session_id).await;
        }
        self.prompt_watcher.unregister(&terminal.id).await;

        let target_status =
            final_status
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
            .ok_or_else(|| anyhow!("Terminal {terminal_id} not found after close"))?;
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

        let repo_dir =
            models::project_repo::ProjectRepo::find_repos_for_project(&self.db.pool, project.id)
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

fn normalize_runtime_config_hint(value: &str) -> String {
    value
        .chars()
        .filter(|ch| ch.is_ascii_alphanumeric())
        .map(|ch| ch.to_ascii_lowercase())
        .collect()
}

fn cli_hint_matches(requested_cli_type_id: &str, cli_type: &CliType) -> bool {
    let hint = normalize_runtime_config_hint(requested_cli_type_id);
    if hint.is_empty() {
        return false;
    }

    let id = normalize_runtime_config_hint(&cli_type.id);
    let name = normalize_runtime_config_hint(&cli_type.name);
    let display_name = normalize_runtime_config_hint(&cli_type.display_name);

    if hint == id || hint == name || hint == display_name {
        return true;
    }

    (hint.contains("claude")
        && (id.contains("claude") || name.contains("claude") || display_name.contains("claude")))
        || ((hint.contains("codex") || hint.contains("openai"))
            && (id.contains("codex") || name.contains("codex") || display_name.contains("codex")))
        || ((hint.contains("gemini") || hint.contains("google"))
            && (id.contains("gemini")
                || name.contains("gemini")
                || display_name.contains("gemini")))
        || (hint.contains("cursor")
            && (id.contains("cursor")
                || name.contains("cursor")
                || display_name.contains("cursor")))
        || (hint.contains("amp")
            && (id.contains("amp") || name.contains("amp") || display_name.contains("amp")))
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use chrono::Utc;
    use db::DBService;
    use sqlx::SqlitePool;

    use super::*;
    use crate::services::{
        orchestrator::message_bus::MessageBus,
        terminal::{PromptWatcher, process::ProcessManager},
    };

    fn test_cli(id: &str, name: &str, display_name: &str) -> CliType {
        CliType {
            id: id.to_string(),
            name: name.to_string(),
            display_name: display_name.to_string(),
            detect_command: format!("{name} --version"),
            install_command: None,
            install_guide_url: None,
            config_file_path: None,
            is_system: true,
            created_at: Utc::now(),
        }
    }

    #[test]
    fn cli_hint_matches_registered_cli_aliases() {
        let claude = test_cli("cli-claude-code", "claude-code", "Claude Code");
        let codex = test_cli("cli-codex", "codex", "Codex");

        assert!(cli_hint_matches("claude-code", &claude));
        assert!(cli_hint_matches("Claude Code", &claude));
        assert!(cli_hint_matches("openai-codex", &codex));
        assert!(cli_hint_matches("cli-codex", &codex));
    }

    #[test]
    fn role_like_cli_hint_does_not_match_registered_cli() {
        let claude = test_cli("cli-claude-code", "claude-code", "Claude Code");
        let codex = test_cli("cli-codex", "codex", "Codex");

        assert!(!cli_hint_matches("backend-engineer", &claude));
        assert!(!cli_hint_matches("qa-engineer", &codex));
        assert!(!cli_hint_matches("devops-engineer", &claude));
    }

    async fn setup_runtime_config_pool() -> SqlitePool {
        let pool = SqlitePool::connect("sqlite::memory:")
            .await
            .expect("create in-memory db");
        sqlx::query(
            r"
            CREATE TABLE cli_type (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                display_name TEXT NOT NULL,
                detect_command TEXT NOT NULL,
                install_command TEXT,
                install_guide_url TEXT,
                config_file_path TEXT,
                is_system INTEGER NOT NULL DEFAULT 1,
                created_at TEXT NOT NULL
            )
            ",
        )
        .execute(&pool)
        .await
        .expect("create cli_type");
        sqlx::query(
            r"
            CREATE TABLE model_config (
                id TEXT PRIMARY KEY,
                cli_type_id TEXT NOT NULL,
                name TEXT NOT NULL,
                display_name TEXT NOT NULL,
                api_model_id TEXT,
                is_default INTEGER NOT NULL DEFAULT 0,
                is_official INTEGER NOT NULL DEFAULT 1,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                encrypted_api_key TEXT,
                base_url TEXT,
                api_type TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .expect("create model_config");

        let now = Utc::now().to_rfc3339();
        for (id, name, display_name) in [
            ("cli-claude-code", "claude-code", "Claude Code"),
            ("cli-codex", "codex", "Codex"),
        ] {
            sqlx::query(
                r"
                INSERT INTO cli_type (
                    id, name, display_name, detect_command, is_system, created_at
                ) VALUES (?, ?, ?, ?, 1, ?)
                ",
            )
            .bind(id)
            .bind(name)
            .bind(display_name)
            .bind(format!("{name} --version"))
            .bind(&now)
            .execute(&pool)
            .await
            .expect("insert cli_type");
        }

        for (id, cli_type_id, name, display_name, is_default) in [
            (
                "model-claude-sonnet",
                "cli-claude-code",
                "sonnet",
                "Claude Sonnet",
                1,
            ),
            ("model-codex-gpt4o", "cli-codex", "gpt-4o", "GPT-4o", 1),
        ] {
            sqlx::query(
                r"
                INSERT INTO model_config (
                    id, cli_type_id, name, display_name, api_model_id,
                    is_default, is_official, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
                ",
            )
            .bind(id)
            .bind(cli_type_id)
            .bind(name)
            .bind(display_name)
            .bind(name)
            .bind(is_default)
            .bind(&now)
            .bind(&now)
            .execute(&pool)
            .await
            .expect("insert model_config");
        }

        pool
    }

    fn runtime_action_service_for_pool(pool: SqlitePool) -> RuntimeActionService {
        let db = Arc::new(DBService { pool });
        let message_bus = Arc::new(MessageBus::new(16));
        let process_manager = Arc::new(ProcessManager::new());
        let prompt_watcher = PromptWatcher::new(message_bus.clone(), process_manager.clone());
        RuntimeActionService::new(db, message_bus, process_manager, prompt_watcher)
    }

    #[tokio::test]
    async fn role_like_cli_id_uses_valid_requested_model_cli() {
        let service = runtime_action_service_for_pool(setup_runtime_config_pool().await);

        let (cli_type, model_config) = service
            .resolve_terminal_runtime_config("backend-engineer", "model-codex-gpt4o")
            .await
            .expect("resolve role-like cli with valid model");

        assert_eq!(cli_type.id, "cli-codex");
        assert_eq!(model_config.id, "model-codex-gpt4o");
    }

    #[tokio::test]
    async fn role_like_cli_id_and_missing_model_fall_back_to_registered_default() {
        let service = runtime_action_service_for_pool(setup_runtime_config_pool().await);

        let (cli_type, model_config) = service
            .resolve_terminal_runtime_config("qa-engineer", "qa-engineer")
            .await
            .expect("resolve role-like cli and model fallback");

        assert_eq!(cli_type.id, "cli-claude-code");
        assert_eq!(model_config.id, "model-claude-sonnet");
    }
}
