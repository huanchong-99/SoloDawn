//! Orchestrator agent loop and event handling.

use std::{
    collections::{HashMap, HashSet},
    sync::Arc,
    time::Instant,
};

use anyhow::anyhow;
use db::DBService;
#[cfg(unix)]
use nix::unistd::Pid;
use once_cell::sync::Lazy;
use regex::Regex;
use tokio::{
    sync::RwLock,
    time::{Duration, MissedTickBehavior, interval, sleep},
};

use super::{
    config::OrchestratorConfig,
    constants::{
        GIT_COMMIT_METADATA_SEPARATOR, TERMINAL_STATUS_COMPLETED, TERMINAL_STATUS_FAILED,
        TERMINAL_STATUS_REVIEW_PASSED, TERMINAL_STATUS_REVIEW_REJECTED, WORKFLOW_STATUS_COMPLETED,
        WORKFLOW_STATUS_FAILED, WORKFLOW_STATUS_MERGING, WORKFLOW_STATUS_RUNNING,
        WORKFLOW_TOPIC_PREFIX,
    },
    llm::{LLMClient, build_terminal_completion_prompt, create_llm_client},
    message_bus::{BusMessage, SharedMessageBus},
    prompt_handler::PromptHandler,
    runtime_actions::{RuntimeActionService, RuntimeTaskSpec, RuntimeTerminalSpec},
    state::{OrchestratorRunState, OrchestratorState, SharedOrchestratorState},
    types::{
        CodeIssue, OrchestratorInstruction, TerminalCompletionEvent, TerminalCompletionStatus,
        TerminalPromptEvent,
    },
};
use crate::services::{
    error_handler::ErrorHandler,
    template_renderer::{TemplateRenderer, WorkflowContext},
};

/// Coordinates workflow execution, message handling, and LLM interactions.
pub struct OrchestratorAgent {
    config: OrchestratorConfig,
    state: SharedOrchestratorState,
    message_bus: SharedMessageBus,
    llm_client: Box<dyn LLMClient>,
    db: Arc<DBService>,
    error_handler: ErrorHandler,
    prompt_handler: PromptHandler,
    runtime_actions: Option<Arc<RuntimeActionService>>,
}

static TASK_HINT_FROM_COMMIT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\btask(?:[_\s-]*id)?[_\s:=-]*([0-9a-f][0-9a-f-]{7,35})\b")
        .expect("task hint regex must be valid")
});

#[derive(Debug, Clone)]
struct InferredNoMetadataCompletion {
    task_id: String,
    terminal_id: String,
    terminal_index: usize,
    total_terminals: usize,
}

#[derive(Debug, Default)]
struct StallRecoveryTracker {
    last_recoveries: HashMap<String, Instant>,
}

impl StallRecoveryTracker {
    fn should_skip_due_to_cooldown(
        &self,
        terminal_id: &str,
        now: Instant,
        cooldown: Duration,
    ) -> bool {
        self.last_recoveries
            .get(terminal_id)
            .is_some_and(|last_recovered_at| now.duration_since(*last_recovered_at) < cooldown)
    }

    fn mark_recovered(&mut self, terminal_id: &str, now: Instant) {
        self.last_recoveries.insert(terminal_id.to_string(), now);
    }

    fn retain_active_terminals(&mut self, active_terminal_ids: &HashSet<String>) {
        self.last_recoveries
            .retain(|terminal_id, _| active_terminal_ids.contains(terminal_id));
    }
}

impl OrchestratorAgent {
    const NEXT_TERMINAL_WAIT_RETRY_ATTEMPTS: usize = 20;
    const NEXT_TERMINAL_WAIT_RETRY_INTERVAL: Duration = Duration::from_millis(500);
    const INITIAL_DISPATCH_DELAY: Duration = Duration::from_millis(2000);
    #[cfg(not(test))]
    const STALL_WATCHDOG_TICK: Duration = Duration::from_secs(5);
    #[cfg(test)]
    const STALL_WATCHDOG_TICK: Duration = Duration::from_millis(80);
    #[cfg(not(test))]
    const STALL_QUIET_WINDOW: Duration = Duration::from_secs(45);
    #[cfg(test)]
    const STALL_QUIET_WINDOW: Duration = Duration::from_millis(180);
    #[cfg(not(test))]
    const STALL_RECOVERY_COOLDOWN: Duration = Duration::from_secs(30);
    #[cfg(test)]
    const STALL_RECOVERY_COOLDOWN: Duration = Duration::from_millis(220);
    const STALL_RECOVERY_CLAUDE_SUBMIT_DELAY_MS: u64 = 260;
    const STALL_RECOVERY_SUFFIX: &str = "Watchdog notice: execution appears stalled. Resume this same task from current workspace state immediately and continue implementation; do not wait for a new task.";

    /// Builds a new orchestrator agent with a configured LLM client.
    pub fn new(
        config: OrchestratorConfig,
        workflow_id: String,
        message_bus: SharedMessageBus,
        db: Arc<DBService>,
    ) -> anyhow::Result<Self> {
        let llm_client = create_llm_client(&config)?;
        let state = Arc::new(RwLock::new(OrchestratorState::new(workflow_id)));
        let error_handler = ErrorHandler::new(db.clone(), message_bus.clone());
        let prompt_handler = PromptHandler::new(message_bus.clone());

        Ok(Self {
            config,
            state,
            message_bus,
            llm_client,
            db,
            error_handler,
            prompt_handler,
            runtime_actions: None,
        })
    }

    /// Create a new agent with a custom LLM client (for testing)
    #[cfg(test)]
    pub fn with_llm_client(
        config: OrchestratorConfig,
        workflow_id: String,
        message_bus: SharedMessageBus,
        db: Arc<DBService>,
        llm_client: Box<dyn LLMClient>,
    ) -> anyhow::Result<Self> {
        let state = Arc::new(RwLock::new(OrchestratorState::new(workflow_id)));
        let error_handler = ErrorHandler::new(db.clone(), message_bus.clone());
        let prompt_handler = PromptHandler::new(message_bus.clone());

        Ok(Self {
            config,
            state,
            message_bus,
            llm_client,
            db,
            error_handler,
            prompt_handler,
            runtime_actions: None,
        })
    }

    pub fn attach_runtime_actions(&mut self, runtime_actions: Arc<RuntimeActionService>) {
        self.runtime_actions = Some(runtime_actions);
    }

    fn runtime_actions(&self) -> anyhow::Result<Arc<RuntimeActionService>> {
        self.runtime_actions
            .clone()
            .ok_or_else(|| anyhow!("Runtime actions are not configured for this orchestrator"))
    }

    async fn load_workflow(&self) -> anyhow::Result<db::models::Workflow> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };
        db::models::Workflow::find_by_id(&self.db.pool, &workflow_id)
            .await
            .map_err(|e| anyhow!("Failed to load workflow {workflow_id}: {e}"))?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))
    }

    async fn is_agent_planned_workflow(&self) -> anyhow::Result<bool> {
        Ok(self.load_workflow().await?.execution_mode == "agent_planned")
    }

    async fn ensure_agent_planned_workflow(&self) -> anyhow::Result<()> {
        if !self.is_agent_planned_workflow().await? {
            return Err(anyhow!(
                "Runtime topology mutations are only allowed for agent_planned workflows"
            ));
        }
        Ok(())
    }

    async fn initialize_workflow_mode_state(&self) -> anyhow::Result<()> {
        if self.is_agent_planned_workflow().await? {
            let mut state = self.state.write().await;
            state.set_workflow_planning_complete(false);
        }
        Ok(())
    }

    async fn run_initial_agent_planning_if_needed(&self) -> anyhow::Result<()> {
        let workflow = self.load_workflow().await?;
        if workflow.execution_mode != "agent_planned" {
            return Ok(());
        }

        let tasks = db::models::WorkflowTask::find_by_workflow(&self.db.pool, &workflow.id)
            .await
            .map_err(|e| anyhow!("Failed to load workflow tasks for {}: {e}", workflow.id))?;
        if !tasks.is_empty() {
            return Ok(());
        }

        let prompt = self.build_initial_planning_prompt(&workflow).await?;
        let response = self.call_llm(&prompt).await?;
        self.execute_instruction(&response).await
    }

    async fn build_initial_planning_prompt(
        &self,
        workflow: &db::models::Workflow,
    ) -> anyhow::Result<String> {
        let goal = workflow
            .initial_goal
            .as_deref()
            .or(workflow.description.as_deref())
            .unwrap_or(&workflow.name);
        let context = self.build_agent_planned_context(workflow).await?;
        Ok(format!(
            "Workflow {} has just started in agent_planned mode with no predefined tasks.\n\nPrimary goal:\n{}\n\n{}\n\nPlan the initial execution graph now. If work should begin immediately, create tasks and terminals, then start those terminals in the same JSON array. When you are confident that no more tasks need to be created later, emit set_workflow_planning_complete.",
            workflow.id, goal, context
        ))
    }

    async fn build_agent_planned_context(
        &self,
        workflow: &db::models::Workflow,
    ) -> anyhow::Result<String> {
        let tasks = db::models::WorkflowTask::find_by_workflow(&self.db.pool, &workflow.id).await?;
        let terminals = db::models::Terminal::find_by_workflow(&self.db.pool, &workflow.id).await?;
        let cli_types = db::models::CliType::find_all(&self.db.pool).await?;
        let model_configs = db::models::ModelConfig::find_all(&self.db.pool).await?;
        let workflow_commands =
            db::models::WorkflowCommand::find_by_workflow(&self.db.pool, &workflow.id).await?;

        let planning_snapshot = {
            let state = self.state.read().await;
            let task_planning: HashMap<String, bool> = state
                .task_states
                .iter()
                .map(|(task_id, task_state)| (task_id.clone(), task_state.planning_complete))
                .collect();
            (state.workflow_planning_complete, task_planning)
        };

        let task_summary = if tasks.is_empty() {
            "Existing tasks: none".to_string()
        } else {
            let lines: Vec<String> = tasks
                .iter()
                .map(|task| {
                    let task_terminals: Vec<String> = terminals
                        .iter()
                        .filter(|terminal| terminal.workflow_task_id == task.id)
                        .map(|terminal| {
                            format!(
                                "{} [{}] cli={} model={} role={}",
                                terminal.id,
                                terminal.status,
                                terminal.cli_type_id,
                                terminal.model_config_id,
                                terminal.role.as_deref().unwrap_or("none")
                            )
                        })
                        .collect();
                    format!(
                        "- {} [{}] branch={} planning_complete={} terminals={}",
                        task.id,
                        task.status,
                        task.branch,
                        planning_snapshot
                            .1
                            .get(&task.id)
                            .copied()
                            .unwrap_or(true),
                        if task_terminals.is_empty() {
                            "none".to_string()
                        } else {
                            task_terminals.join(" | ")
                        }
                    )
                })
                .collect();
            format!("Existing tasks:\n{}", lines.join("\n"))
        };

        let cli_summary = cli_types
            .iter()
            .map(|cli| {
                let models: Vec<String> = model_configs
                    .iter()
                    .filter(|model| model.cli_type_id == cli.id)
                    .map(|model| format!("{} ({})", model.id, model.display_name))
                    .collect();
                format!(
                    "- {} ({}) => {}",
                    cli.id,
                    cli.display_name,
                    if models.is_empty() {
                        "no models".to_string()
                    } else {
                        models.join(", ")
                    }
                )
            })
            .collect::<Vec<_>>()
            .join("\n");

        let command_summary = if workflow_commands.is_empty() {
            "Workflow slash commands: none".to_string()
        } else {
            let mut commands = Vec::new();
            for command in workflow_commands {
                let label = if let Some(preset) =
                    db::models::SlashCommandPreset::find_by_id(&self.db.pool, &command.preset_id)
                        .await?
                {
                    preset.command
                } else {
                    command.preset_id
                };
                commands.push(format!(
                    "- order={} command={} params={}",
                    command.order_index,
                    label,
                    command.custom_params.as_deref().unwrap_or("{}")
                ));
            }
            format!("Workflow slash commands:\n{}", commands.join("\n"))
        };

        Ok(format!(
            "Workflow planning complete: {}\n{}\n\nAllowed CLI/model pool:\n{}\n\n{}\n\nAvailable runtime actions:\n- create_task\n- create_terminal\n- start_terminal\n- close_terminal\n- complete_task\n- set_workflow_planning_complete\n\nOnly use the listed cli_type_id/model_config_id values. Return raw JSON only. If later instructions refer to objects created in the same response, provide explicit task_id / terminal_id values in the create actions.",
            planning_snapshot.0,
            task_summary,
            cli_summary,
            command_summary
        ))
    }

    async fn task_planning_complete(&self, task_id: &str) -> bool {
        let state = self.state.read().await;
        state
            .task_states
            .get(task_id)
            .map(|task_state| task_state.planning_complete)
            .unwrap_or(false)
    }

    async fn sync_task_state_from_db(
        &self,
        task_id: &str,
        planning_complete: Option<bool>,
    ) -> anyhow::Result<()> {
        let terminals = db::models::Terminal::find_by_task(&self.db.pool, task_id)
            .await
            .map_err(|e| anyhow!("Failed to load terminals for task {task_id}: {e}"))?;
        let terminal_ids: Vec<String> = terminals
            .into_iter()
            .map(|terminal| terminal.id)
            .collect();
        let planning_complete = match planning_complete {
            Some(value) => value,
            None => {
                let state = self.state.read().await;
                state
                    .task_states
                    .get(task_id)
                    .map(|task_state| task_state.planning_complete)
                    .unwrap_or(true)
            }
        };

        let mut state = self.state.write().await;
        state.sync_task_terminals(task_id.to_string(), terminal_ids, planning_complete);
        Ok(())
    }

    async fn finalize_task_if_ready(&self, task_id: &str) -> anyhow::Result<()> {
        let (planning_complete, task_completed, task_failed, workflow_id) = {
            let state = self.state.read().await;
            (
                state
                    .task_states
                    .get(task_id)
                    .map(|task_state| task_state.planning_complete)
                    .unwrap_or(false),
                state.is_task_completed(task_id),
                state.task_has_failures(task_id),
                state.workflow_id.clone(),
            )
        };

        if !planning_complete || !task_completed {
            return Ok(());
        }

        let status = if task_failed { "failed" } else { "completed" };
        db::models::WorkflowTask::update_status(&self.db.pool, task_id, status)
            .await
            .map_err(|e| anyhow!("Failed to update task {} status to {}: {e}", task_id, status))?;
        self.message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TaskStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    task_id: task_id.to_string(),
                    status: status.to_string(),
                },
            )
            .await
            .map_err(|e| anyhow!("Failed to publish task status update: {e}"))?;
        Ok(())
    }

    /// Runs the orchestrator event loop until shutdown.
    pub async fn run(&self) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        let mut rx = self
            .message_bus
            .subscribe(&format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}"))
            .await;
        tracing::info!("Orchestrator started for workflow: {}", workflow_id);

        // Initialize system prompt and state before processing events.
        {
            let mut state = self.state.write().await;
            state.add_message("system", &self.config.system_prompt, &self.config);
            state.run_state = OrchestratorRunState::Idle;
        }
        if let Err(e) = self.initialize_workflow_mode_state().await {
            tracing::error!("Failed to initialize workflow mode state: {}", e);
        }

        // Execute slash commands if enabled for this workflow
        if let Err(e) = self.execute_slash_commands().await {
            tracing::error!("Failed to execute slash commands: {}", e);
            // Don't fail the workflow, just log the error
        }

        // Give freshly started PTY terminals a short warm-up window before first dispatch.
        // Some CLIs (notably Claude) may drop/garble the first prompt if input arrives too early.
        sleep(Self::INITIAL_DISPATCH_DELAY).await;

        // Auto-dispatch initial terminals for all tasks
        if let Err(e) = self.auto_dispatch_initial_tasks().await {
            tracing::error!("Failed to auto-dispatch initial tasks: {}", e);
            // Don't fail the workflow, just log the error
        }
        if let Err(e) = self.run_initial_agent_planning_if_needed().await {
            tracing::error!("Failed to run initial agent-planned cycle: {}", e);
        }

        // 濞存粌顑勫▎銏狀嚗椤忓棗绠?
        let mut stall_recovery_tracker = StallRecoveryTracker::default();
        let mut watchdog = interval(Self::STALL_WATCHDOG_TICK);
        watchdog.set_missed_tick_behavior(MissedTickBehavior::Delay);
        watchdog.tick().await;

        loop {
            tokio::select! {
                maybe_message = rx.recv() => {
                    let Some(message) = maybe_message else {
                        break;
                    };
                    let should_stop = self.handle_message(message).await?;
                    if should_stop {
                        break;
                    }
                }
                _ = watchdog.tick() => {
                    if let Err(error) = self
                        .recover_stalled_terminals(&mut stall_recovery_tracker)
                        .await
                    {
                        tracing::warn!(
                            workflow_id = %workflow_id,
                            error = %error,
                            "Failed while recovering stalled terminals"
                        );
                    }
                }
            }
        }

        tracing::info!("Orchestrator stopped for workflow: {}", workflow_id);
        Ok(())
    }

    /// Dispatches incoming bus messages and returns true if shutdown is requested.
    async fn handle_message(&self, message: BusMessage) -> anyhow::Result<bool> {
        match message {
            BusMessage::TerminalCompleted(event) => {
                self.handle_terminal_completed(event).await?;
            }
            BusMessage::TerminalPromptDetected(event) => {
                self.handle_terminal_prompt_detected(event).await?;
            }
            BusMessage::GitEvent {
                workflow_id,
                commit_hash,
                branch,
                message,
            } => {
                self.handle_git_event(&workflow_id, &commit_hash, &branch, &message)
                    .await?;
            }
            BusMessage::Shutdown => {
                return Ok(true);
            }
            _ => {}
        }
        Ok(false)
    }

    async fn recover_stalled_terminals(
        &self,
        tracker: &mut StallRecoveryTracker,
    ) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            if state.run_state != OrchestratorRunState::Idle {
                return Ok(());
            }
            state.workflow_id.clone()
        };

        let Some(workflow) = db::models::Workflow::find_by_id(&self.db.pool, &workflow_id).await?
        else {
            tracker.last_recoveries.clear();
            return Ok(());
        };

        if workflow.status != WORKFLOW_STATUS_RUNNING {
            tracker.last_recoveries.clear();
            return Ok(());
        }

        let tasks = db::models::WorkflowTask::find_by_workflow(&self.db.pool, &workflow_id).await?;
        if tasks.is_empty() {
            tracker.last_recoveries.clear();
            return Ok(());
        }

        let mut active_working_terminal_ids = HashSet::new();

        for task in tasks {
            if matches!(task.status.as_str(), "completed" | "failed" | "cancelled") {
                continue;
            }

            let terminals = db::models::Terminal::find_by_task(&self.db.pool, &task.id).await?;
            if terminals.is_empty() {
                continue;
            }

            for terminal in terminals
                .iter()
                .filter(|terminal| terminal.status == "working")
            {
                active_working_terminal_ids.insert(terminal.id.clone());

                if self
                    .remaining_terminal_quiet_duration(&terminal.id, Self::STALL_QUIET_WINDOW)
                    .await?
                    .is_some()
                {
                    continue;
                }

                let cooldown_check_at = Instant::now();
                if tracker.should_skip_due_to_cooldown(
                    &terminal.id,
                    cooldown_check_at,
                    Self::STALL_RECOVERY_COOLDOWN,
                ) {
                    continue;
                }

                if let Err(error) = self
                    .redispatch_stalled_terminal_instruction(
                        &workflow_id,
                        &task,
                        terminal,
                        terminals.len(),
                    )
                    .await
                {
                    tracing::warn!(
                        workflow_id = %workflow_id,
                        task_id = %task.id,
                        terminal_id = %terminal.id,
                        error = %error,
                        "Failed to re-dispatch stalled terminal"
                    );
                    continue;
                }

                tracker.mark_recovered(&terminal.id, Instant::now());
            }
        }

        tracker.retain_active_terminals(&active_working_terminal_ids);
        Ok(())
    }

    async fn redispatch_stalled_terminal_instruction(
        &self,
        workflow_id: &str,
        task: &db::models::WorkflowTask,
        terminal: &db::models::Terminal,
        total_terminals: usize,
    ) -> anyhow::Result<()> {
        let pty_session_id = terminal
            .pty_session_id
            .as_deref()
            .or(terminal.session_id.as_deref())
            .map(str::trim)
            .filter(|session_id| !session_id.is_empty())
            .map(str::to_string)
            .ok_or_else(|| {
                anyhow!(
                    "Terminal {} has no PTY/session binding for stall recovery",
                    terminal.id
                )
            })?;

        let instruction = format!(
            "{} | {}",
            Self::build_task_instruction(workflow_id, task, terminal, total_terminals),
            Self::STALL_RECOVERY_SUFFIX
        );

        if Self::needs_explicit_submit(terminal) {
            self.message_bus
                .publish_terminal_input(&terminal.id, &pty_session_id, &instruction, None)
                .await;
        } else {
            self.message_bus
                .publish(
                    &pty_session_id,
                    BusMessage::TerminalMessage {
                        message: instruction.clone(),
                    },
                )
                .await
                .map_err(|e| anyhow!("Failed to publish stalled-terminal instruction: {e}"))?;
        }

        for (attempt, delay_ms) in Self::stall_recovery_submit_keystroke_schedule_ms(terminal)
            .iter()
            .enumerate()
        {
            sleep(Duration::from_millis(*delay_ms)).await;
            self.message_bus
                .publish_terminal_input(&terminal.id, &pty_session_id, "", None)
                .await;
            tracing::debug!(
                workflow_id = %workflow_id,
                task_id = %task.id,
                terminal_id = %terminal.id,
                attempt = attempt + 1,
                delay_ms,
                "Sent submit keystroke after stalled-terminal re-dispatch"
            );
        }

        tracing::warn!(
            workflow_id = %workflow_id,
            task_id = %task.id,
            terminal_id = %terminal.id,
            "Recovered stalled terminal by re-dispatching current task instruction"
        );

        Ok(())
    }

    /// Handles terminal prompt detected events.
    async fn handle_terminal_prompt_detected(
        &self,
        event: TerminalPromptEvent,
    ) -> anyhow::Result<()> {
        if let Some(decision) = self.prompt_handler.handle_prompt_event(&event).await {
            tracing::info!(
                terminal_id = %event.terminal_id,
                prompt_kind = ?event.prompt.kind,
                decision = ?decision,
                "Handled terminal prompt event"
            );
        }
        Ok(())
    }

    /// Handles terminal completion events.
    async fn handle_terminal_completed(
        &self,
        event: TerminalCompletionEvent,
    ) -> anyhow::Result<()> {
        tracing::info!(
            "Terminal completed: {} with status {:?}",
            event.terminal_id,
            event.status
        );

        // Determine if terminal completed successfully
        let success = matches!(
            event.status,
            TerminalCompletionStatus::Completed | TerminalCompletionStatus::ReviewPass
        );

        self.ensure_task_state_initialized_for_completion(&event.task_id)
            .await?;

        let expected_terminal_id = {
            let next_index = {
                let state = self.state.read().await;
                state.get_next_terminal_for_task(&event.task_id)
            };

            if let Some(index) = next_index {
                let terminals =
                    db::models::Terminal::find_by_task(&self.db.pool, &event.task_id).await?;
                terminals.get(index).map(|terminal| terminal.id.clone())
            } else {
                None
            }
        };

        if let Some(expected_terminal_id) = expected_terminal_id {
            if expected_terminal_id != event.terminal_id {
                tracing::warn!(
                    task_id = %event.task_id,
                    terminal_id = %event.terminal_id,
                    expected_terminal_id = %expected_terminal_id,
                    status = ?event.status,
                    "Ignoring out-of-order terminal completion event"
                );
                return Ok(());
            }
        }

        let Some(existing_terminal) =
            db::models::Terminal::find_by_id(&self.db.pool, &event.terminal_id).await?
        else {
            tracing::warn!(
                terminal_id = %event.terminal_id,
                task_id = %event.task_id,
                completion_status = ?event.status,
                "Ignoring terminal completion event because terminal does not exist"
            );
            return Ok(());
        };

        if existing_terminal.completed_at.is_some() {
            tracing::info!(
                terminal_id = %event.terminal_id,
                task_id = %event.task_id,
                status = %existing_terminal.status,
                "Ignoring duplicate terminal completion event for finalized terminal"
            );
            return Ok(());
        }

        if success && existing_terminal.status != "working" {
            tracing::warn!(
                terminal_id = %event.terminal_id,
                task_id = %event.task_id,
                terminal_status = %existing_terminal.status,
                completion_status = ?event.status,
                "Ignoring terminal completion event because terminal is not working"
            );
            return Ok(());
        }

        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        if success {
            let quiet_window = Duration::from_secs(40);
            if let Some(remaining) = self
                .remaining_terminal_quiet_duration(&event.terminal_id, quiet_window)
                .await?
            {
                self.defer_terminal_completion(event.clone(), quiet_window, remaining)
                    .await?;
                tracing::info!(
                    terminal_id = %event.terminal_id,
                    task_id = %event.task_id,
                    remaining_secs = remaining.as_secs(),
                    "Deferring terminal completion until quiet window is satisfied"
                );
                return Ok(());
            }
        }

        let terminal_final_status = if success {
            TERMINAL_STATUS_COMPLETED
        } else {
            TERMINAL_STATUS_FAILED
        };

        let completion_updated = if success {
            match db::models::Terminal::set_completed_cas(
                &self.db.pool,
                &event.terminal_id,
                "working",
                terminal_final_status,
            )
            .await
            {
                Ok(true) => true,
                Ok(false) => {
                    match db::models::Terminal::set_completed_if_unfinished(
                        &self.db.pool,
                        &event.terminal_id,
                        terminal_final_status,
                    )
                    .await
                    {
                        Ok(true) => {
                            tracing::info!(
                                terminal_id = %event.terminal_id,
                                task_id = %event.task_id,
                                target_status = %terminal_final_status,
                                "Applied terminal completion fallback after CAS miss"
                            );
                            true
                        }
                        Ok(false) => {
                            tracing::info!(
                                terminal_id = %event.terminal_id,
                                task_id = %event.task_id,
                                expected_status = "working",
                                target_status = %terminal_final_status,
                                "Skipping terminal completion because terminal is already finalized"
                            );
                            false
                        }
                        Err(e) => {
                            tracing::error!(
                                terminal_id = %event.terminal_id,
                                task_id = %event.task_id,
                                target_status = %terminal_final_status,
                                error = %e,
                                "Failed to mark terminal completion after CAS miss fallback"
                            );
                            false
                        }
                    }
                }
                Err(e) => {
                    tracing::error!(
                        terminal_id = %event.terminal_id,
                        task_id = %event.task_id,
                        target_status = %terminal_final_status,
                        error = %e,
                        "Failed to mark terminal completion with CAS"
                    );
                    false
                }
            }
        } else {
            match db::models::Terminal::set_completed_cas(
                &self.db.pool,
                &event.terminal_id,
                "working",
                terminal_final_status,
            )
            .await
            {
                Ok(true) => true,
                Ok(false) => {
                    match db::models::Terminal::set_completed_if_unfinished(
                        &self.db.pool,
                        &event.terminal_id,
                        terminal_final_status,
                    )
                    .await
                    {
                        Ok(true) => true,
                        Ok(false) => {
                            tracing::info!(
                                terminal_id = %event.terminal_id,
                                task_id = %event.task_id,
                                target_status = %terminal_final_status,
                                "Skipping terminal completion fallback because terminal is already finalized"
                            );
                            false
                        }
                        Err(e) => {
                            tracing::error!(
                                terminal_id = %event.terminal_id,
                                task_id = %event.task_id,
                                target_status = %terminal_final_status,
                                error = %e,
                                "Failed to mark terminal completion after CAS miss fallback"
                            );
                            false
                        }
                    }
                }
                Err(e) => {
                    tracing::error!(
                        terminal_id = %event.terminal_id,
                        task_id = %event.task_id,
                        target_status = %terminal_final_status,
                        error = %e,
                        "Failed to mark terminal completion with CAS"
                    );
                    false
                }
            }
        };

        if !completion_updated {
            return Ok(());
        }

        if success {
            self.enforce_terminal_completion_shutdown(&workflow_id, &existing_terminal)
                .await;
        }

        let _ = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TerminalStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    terminal_id: event.terminal_id.clone(),
                    status: terminal_final_status.to_string(),
                },
            )
            .await;

        // Update state and get next terminal info
        let (next_terminal_index, task_completed, has_next, task_failed) = {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Processing;
            state.mark_terminal_completed(&event.task_id, &event.terminal_id, success);

            // Advance to next terminal if successful
            let has_next = if success {
                state.advance_terminal(&event.task_id)
            } else {
                false
            };

            let next_index = state.get_next_terminal_for_task(&event.task_id);
            let task_completed = state.is_task_completed(&event.task_id);
            let task_failed = state.task_has_failures(&event.task_id);

            (next_index, task_completed, has_next, task_failed)
        };

        // Update task status based on completion/failure
        // Fail fast: if terminal failed, mark task as failed immediately to avoid stalled tasks
        let task_final_status = if !success {
            tracing::warn!(
                "Task {} marked as failed due to terminal {} failure",
                event.task_id,
                event.terminal_id
            );
            Some("failed")
        } else if task_failed && task_completed {
            Some("failed")
        } else if task_completed {
            Some("completed")
        } else {
            None
        };

        if let Some(task_status) = task_final_status {
            if let Err(e) =
                db::models::WorkflowTask::update_status(&self.db.pool, &event.task_id, task_status)
                    .await
            {
                tracing::error!(
                    "Failed to mark task {} {}: {}",
                    event.task_id,
                    task_status,
                    e
                );
            }

            let _ = self
                .message_bus
                .publish_workflow_event(
                    &workflow_id,
                    BusMessage::TaskStatusUpdate {
                        workflow_id: workflow_id.clone(),
                        task_id: event.task_id.clone(),
                        status: task_status.to_string(),
                    },
                )
                .await;
        }

        // 闁哄瀚紓鎾诲箵閹邦喓浠涙鐐村劶閻ㄧ喖鏁?LLM
        let should_run_completion_llm = !(success && has_next && !task_failed);
        let mut completion_response: Option<String> = None;
        if should_run_completion_llm {
            let prompt = self.build_completion_prompt(&event).await?;
            completion_response = Some(self.call_llm(&prompt).await?);
        }

        // Parse and execute orchestrator instructions from completion response.
        if let Some(response) = completion_response.as_deref() {
            self.execute_instruction(response).await?;
        }

        // Auto-dispatch next terminal if successful, there's more to do, and task hasn't failed
        if success && has_next && !task_failed {
            if let Some(index) = next_terminal_index {
                if let Err(e) = self.dispatch_next_terminal(&event.task_id, index).await {
                    tracing::error!(
                        "Failed to dispatch next terminal for task {}: {}",
                        event.task_id,
                        e
                    );
                }
            }
        }

        self.auto_sync_workflow_completion(&workflow_id).await?;

        // Restore idle state before returning.
        {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Idle;
        }

        Ok(())
    }

    async fn ensure_task_state_initialized_for_completion(
        &self,
        task_id: &str,
    ) -> anyhow::Result<()> {
        {
            let state = self.state.read().await;
            if state.task_states.contains_key(task_id) {
                return Ok(());
            }
        }

        let terminals = db::models::Terminal::find_by_task(&self.db.pool, task_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to load terminals for task {task_id}: {e}"))?;

        if terminals.is_empty() {
            return Ok(());
        }

        let terminal_ids: Vec<String> = terminals.iter().map(|terminal| terminal.id.clone()).collect();
        let total_terminals = terminal_ids.len();
        let mut current_terminal_index = 0usize;
        let mut found_active_terminal = false;
        let mut completed_terminals = Vec::new();
        let mut failed_terminals = Vec::new();

        for (index, terminal) in terminals.iter().enumerate() {
            match terminal.status.as_str() {
                "completed" => completed_terminals.push(terminal.id.clone()),
                "failed" | "cancelled" => failed_terminals.push(terminal.id.clone()),
                _ => {
                    if !found_active_terminal {
                        current_terminal_index = index;
                        found_active_terminal = true;
                    }
                }
            }
        }

        if !found_active_terminal {
            current_terminal_index = total_terminals.saturating_sub(1);
        }

        let mut state = self.state.write().await;
        if state.task_states.contains_key(task_id) {
            return Ok(());
        }

        state.sync_task_terminals(task_id.to_string(), terminal_ids, true);

        if let Some(task_state) = state.task_states.get_mut(task_id) {
            task_state.current_terminal_index = current_terminal_index;
            task_state.completed_terminals = completed_terminals;
            task_state.failed_terminals = failed_terminals;
            task_state.is_completed = task_state.planning_complete
                && task_state.completed_terminals.len() + task_state.failed_terminals.len()
                    >= task_state.total_terminals;
        }

        Ok(())
    }

    async fn defer_terminal_completion(
        &self,
        event: TerminalCompletionEvent,
        quiet_window: Duration,
        initial_remaining: Duration,
    ) -> anyhow::Result<()> {
        {
            let mut state = self.state.write().await;
            if !state
                .pending_quiet_completion_checks
                .insert(event.terminal_id.clone())
            {
                tracing::debug!(
                    terminal_id = %event.terminal_id,
                    "Quiet-window check already in progress for terminal"
                );
                return Ok(());
            }
        }

        let db = self.db.clone();
        let message_bus = self.message_bus.clone();
        let state = self.state.clone();
        let terminal_id = event.terminal_id.clone();
        let workflow_id = event.workflow_id.clone();

        tokio::spawn(async move {
            let mut wait_for = if initial_remaining.is_zero() {
                Duration::from_millis(100)
            } else {
                initial_remaining
            };

            loop {
                sleep(wait_for).await;

                match Self::remaining_terminal_quiet_duration_from_db(
                    &db.pool,
                    &terminal_id,
                    quiet_window,
                )
                .await
                {
                    Ok(Some(remaining)) => {
                        wait_for = remaining.min(Duration::from_secs(5));
                    }
                    Ok(None) => {
                        {
                            let mut locked = state.write().await;
                            locked.pending_quiet_completion_checks.remove(&terminal_id);
                        }

                        if let Err(e) = message_bus
                            .publish_workflow_event(
                                &workflow_id,
                                BusMessage::TerminalCompleted(event.clone()),
                            )
                            .await
                        {
                            tracing::warn!(
                                terminal_id = %terminal_id,
                                workflow_id = %workflow_id,
                                error = %e,
                                "Failed to publish deferred terminal completion event"
                            );
                        }
                        break;
                    }
                    Err(e) => {
                        tracing::warn!(
                            terminal_id = %terminal_id,
                            workflow_id = %workflow_id,
                            error = %e,
                            "Failed to evaluate terminal quiet window; dropping deferred completion"
                        );
                        let mut locked = state.write().await;
                        locked.pending_quiet_completion_checks.remove(&terminal_id);
                        break;
                    }
                }
            }
        });

        Ok(())
    }

    async fn remaining_terminal_quiet_duration(
        &self,
        terminal_id: &str,
        quiet_window: Duration,
    ) -> anyhow::Result<Option<Duration>> {
        Self::remaining_terminal_quiet_duration_from_db(&self.db.pool, terminal_id, quiet_window)
            .await
    }

    async fn remaining_terminal_quiet_duration_from_db(
        pool: &sqlx::SqlitePool,
        terminal_id: &str,
        quiet_window: Duration,
    ) -> anyhow::Result<Option<Duration>> {
        if quiet_window.is_zero() {
            return Ok(None);
        }

        let latest_output_at: Option<chrono::DateTime<chrono::Utc>> = sqlx::query_scalar(
            r#"
            SELECT MAX(created_at)
            FROM terminal_log
            WHERE terminal_id = ?
            "#,
        )
        .bind(terminal_id)
        .fetch_one(pool)
        .await?;

        let last_activity_at = if let Some(last_output_at) = latest_output_at {
            Some(last_output_at)
        } else {
            sqlx::query_scalar(
                r#"
                SELECT COALESCE(started_at, updated_at, created_at)
                FROM terminal
                WHERE id = ?
                "#,
            )
            .bind(terminal_id)
            .fetch_optional(pool)
            .await?
            .flatten()
        };

        let Some(last_activity_at) = last_activity_at else {
            return Ok(Some(quiet_window));
        };

        let elapsed_ms = chrono::Utc::now()
            .signed_duration_since(last_activity_at)
            .num_milliseconds()
            .max(0);
        let quiet_ms = quiet_window.as_millis() as i64;

        if elapsed_ms >= quiet_ms {
            Ok(None)
        } else {
            Ok(Some(Duration::from_millis((quiet_ms - elapsed_ms) as u64)))
        }
    }

    /// Dispatches the next terminal in a task sequence.
    async fn dispatch_next_terminal(
        &self,
        task_id: &str,
        terminal_index: usize,
    ) -> anyhow::Result<()> {
        // Get task
        let task = db::models::WorkflowTask::find_by_id(&self.db.pool, task_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get task: {e}"))?
            .ok_or_else(|| anyhow::anyhow!("Task {task_id} not found"))?;

        // Get terminals
        let terminals = db::models::Terminal::find_by_task(&self.db.pool, task_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get terminals: {e}"))?;

        let terminal = terminals.get(terminal_index).cloned().ok_or_else(|| {
            anyhow::anyhow!("Terminal index {terminal_index} out of range for task {task_id}")
        })?;

        let terminal_id = terminal.id.clone();
        let mut active_terminal = terminal;

        for attempt in 0..=Self::NEXT_TERMINAL_WAIT_RETRY_ATTEMPTS {
            if active_terminal.status == "waiting" {
                break;
            }

            if matches!(
                active_terminal.status.as_str(),
                "working" | "completed" | "failed" | "cancelled"
            ) {
                tracing::debug!(
                    terminal_id = %active_terminal.id,
                    task_id = %task_id,
                    status = %active_terminal.status,
                    "Skipping next terminal dispatch because terminal already reached terminal state"
                );
                return Ok(());
            }

            if attempt == Self::NEXT_TERMINAL_WAIT_RETRY_ATTEMPTS {
                tracing::warn!(
                    terminal_id = %active_terminal.id,
                    task_id = %task_id,
                    status = %active_terminal.status,
                    attempts = Self::NEXT_TERMINAL_WAIT_RETRY_ATTEMPTS,
                    "Timed out waiting next terminal to become ready for dispatch"
                );
                return Ok(());
            }

            sleep(Self::NEXT_TERMINAL_WAIT_RETRY_INTERVAL).await;

            active_terminal = db::models::Terminal::find_by_id(&self.db.pool, &terminal_id)
                .await
                .map_err(|e| anyhow::anyhow!("Failed to refresh terminal status: {e}"))?
                .ok_or_else(|| anyhow::anyhow!("Terminal {terminal_id} not found"))?;
        }

        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // Build and dispatch instruction
        let instruction =
            Self::build_task_instruction(&workflow_id, &task, &active_terminal, terminals.len());
        self.dispatch_terminal(task_id, &active_terminal, &instruction)
            .await
    }

    /// Handles Git events emitted by the watcher.
    ///
    /// For commits with METADATA: routes to appropriate handler based on status.
    /// For commits without METADATA: wakes up orchestrator for decision making.
    pub async fn handle_git_event(
        &self,
        workflow_id: &str,
        commit_hash: &str,
        branch: &str,
        message: &str,
    ) -> anyhow::Result<()> {
        tracing::info!(
            "Git event: {} on branch {} - {}",
            commit_hash,
            branch,
            message
        );

        // Check if this commit was already processed (idempotency)
        {
            let state = self.state.read().await;
            if state.processed_commits.contains(commit_hash) {
                tracing::debug!("Commit {} already processed, skipping", commit_hash);
                return Ok(());
            }
        }

        // Persist git event to DB with 'pending' status
        let git_event =
            db::models::git_event::GitEvent::new_pending(workflow_id, commit_hash, branch, message);
        let event_id = git_event.id.clone();
        if let Err(e) = db::models::git_event::GitEvent::insert(&self.db.pool, &git_event).await {
            tracing::warn!("Failed to persist git_event: {}", e);
        }

        // 1. Try to parse commit metadata
        let metadata = match crate::services::git_watcher::parse_commit_metadata(message) {
            Ok(m) => m,
            Err(_) => {
                // No METADATA - wake up orchestrator for decision
                tracing::info!(
                    "Commit {} has no METADATA, waking orchestrator for decision",
                    commit_hash
                );
                self.handle_git_event_no_metadata(
                    workflow_id,
                    commit_hash,
                    branch,
                    message,
                    &event_id,
                )
                .await?;
                return Ok(());
            }
        };

        // 2. Validate workflow_id matches
        if metadata.workflow_id != workflow_id {
            tracing::warn!(
                "Workflow ID mismatch: expected {}, got {}",
                workflow_id,
                metadata.workflow_id
            );
            // Don't mark as processed - another workflow may need this commit
            return Ok(());
        }

        // Mark commit as processed after validation
        {
            let mut state = self.state.write().await;
            state.processed_commits.insert(commit_hash.to_string());
        }

        // Update git_event with parsed metadata and set status to 'processing'
        let metadata_json = serde_json::to_string(&metadata).ok();
        let _ = db::models::git_event::GitEvent::update_metadata(
            &self.db.pool,
            &event_id,
            &metadata.terminal_id,
            metadata_json.as_deref(),
        )
        .await;

        // 3. Route to handler based on status
        match metadata.status.as_str() {
            TERMINAL_STATUS_COMPLETED => {
                if Self::should_skip_completed_handoff(&metadata.next_action) {
                    tracing::info!(
                        commit_hash = %commit_hash,
                        terminal_id = %metadata.terminal_id,
                        task_id = %metadata.task_id,
                        next_action = %metadata.next_action,
                        "Skipping completed handoff for continue/retry commit"
                    );
                    return Ok(());
                }

                self.handle_git_terminal_completed(
                    &metadata.terminal_id,
                    &metadata.task_id,
                    commit_hash,
                    message,
                )
                .await?;
            }
            "review_pass" => {
                self.handle_git_review_pass(
                    &metadata.terminal_id,
                    &metadata.task_id,
                    &metadata
                        .reviewed_terminal
                        .ok_or_else(|| anyhow!("reviewed_terminal required for review_pass"))?,
                )
                .await?;
            }
            "review_reject" => {
                self.handle_git_review_reject(
                    &metadata.terminal_id,
                    &metadata.task_id,
                    &metadata
                        .reviewed_terminal
                        .ok_or_else(|| anyhow!("reviewed_terminal required for review_reject"))?,
                    &metadata
                        .issues
                        .ok_or_else(|| anyhow!("issues required for review_reject"))?,
                )
                .await?;
            }
            TERMINAL_STATUS_FAILED => {
                self.handle_git_terminal_failed(&metadata.terminal_id, &metadata.task_id, message)
                    .await?;
            }
            _ => {
                tracing::warn!("Unknown status in commit: {}", metadata.status);
                let _ = db::models::git_event::GitEvent::update_status(
                    &self.db.pool,
                    &event_id,
                    "failed",
                    None,
                )
                .await;
            }
        }

        // Mark git_event as processed after successful handling
        let _ = db::models::git_event::GitEvent::update_status(
            &self.db.pool,
            &event_id,
            "processed",
            None,
        )
        .await;

        Ok(())
    }

    fn should_skip_completed_handoff(next_action: &str) -> bool {
        matches!(
            next_action.trim().to_ascii_lowercase().as_str(),
            "continue" | "retry"
        )
    }

    /// Handle git event without METADATA - wake up orchestrator for decision.
    async fn handle_git_event_no_metadata(
        &self,
        workflow_id: &str,
        commit_hash: &str,
        branch: &str,
        message: &str,
        event_id: &str,
    ) -> anyhow::Result<()> {
        // Update git_event status to processing
        let _ = db::models::git_event::GitEvent::update_status(
            &self.db.pool,
            event_id,
            "processing",
            None,
        )
        .await;

        // Add to conversation history for context
        {
            let mut state = self.state.write().await;
            state.add_message(
                "system",
                &format!(
                    "Git commit detected on branch '{}': {} - {}",
                    branch,
                    &commit_hash[..8.min(commit_hash.len())],
                    message
                ),
                &self.config,
            );
        }

        let inferred = self
            .infer_no_metadata_completion(workflow_id, commit_hash, branch, message)
            .await?;

        let Some(inferred) = inferred else {
            let reason = "Unable to infer task/terminal from no-metadata commit; manual intervention required";
            tracing::warn!(
                workflow_id = %workflow_id,
                commit_hash = %commit_hash,
                branch = %branch,
                commit_message = %message,
                reason,
                "Skipping no-metadata commit because inference was ambiguous"
            );
            let _ = db::models::git_event::GitEvent::update_status(
                &self.db.pool,
                event_id,
                "failed",
                Some(reason),
            )
            .await;
            return Ok(());
        };

        // Align in-memory progress cursor with the inferred terminal before completion handling.
        self.align_task_state_for_no_metadata_completion(
            &inferred.task_id,
            inferred.terminal_index,
            inferred.total_terminals,
        )
        .await;

        if let Err(err) = self
            .handle_git_terminal_completed(
                &inferred.terminal_id,
                &inferred.task_id,
                commit_hash,
                message,
            )
            .await
        {
            let reason = format!(
                "Failed to process inferred no-metadata completion for terminal {}: {}",
                inferred.terminal_id, err
            );
            tracing::error!(
                workflow_id = %workflow_id,
                task_id = %inferred.task_id,
                terminal_id = %inferred.terminal_id,
                commit_hash = %commit_hash,
                error = %err,
                "Failed to handle no-metadata commit"
            );
            let _ = db::models::git_event::GitEvent::update_status(
                &self.db.pool,
                event_id,
                "failed",
                Some(&reason),
            )
            .await;
            return Ok(());
        }

        {
            let mut state = self.state.write().await;
            state.processed_commits.insert(commit_hash.to_string());
        }

        let _ = db::models::git_event::GitEvent::update_status(
            &self.db.pool,
            event_id,
            "processed",
            Some("Inferred terminal completion from no-metadata commit"),
        )
        .await;

        tracing::info!(
            workflow_id = %workflow_id,
            task_id = %inferred.task_id,
            terminal_id = %inferred.terminal_id,
            commit_hash = %commit_hash,
            "Processed no-metadata commit via inferred terminal completion"
        );

        Ok(())
    }

    fn extract_task_hint_from_commit_message(message: &str) -> Option<String> {
        TASK_HINT_FROM_COMMIT_RE
            .captures(message)
            .and_then(|caps| caps.get(1))
            .map(|m| m.as_str().to_ascii_lowercase())
    }

    fn looks_like_noop_handoff_commit(message: &str) -> bool {
        let normalized = message.to_ascii_lowercase();
        normalized.contains("empty commit")
            || normalized.contains("no changes needed")
            || normalized.contains("advance orchestrator")
            || (normalized.contains("handoff") && normalized.contains("orchestrator"))
    }

    async fn infer_no_metadata_completion(
        &self,
        workflow_id: &str,
        commit_hash: &str,
        branch: &str,
        message: &str,
    ) -> anyhow::Result<Option<InferredNoMetadataCompletion>> {
        let task_hint = Self::extract_task_hint_from_commit_message(message);
        let all_tasks = db::models::WorkflowTask::find_by_workflow(&self.db.pool, workflow_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to load workflow tasks for inference: {e}"))?;

        let mut candidate_tasks = all_tasks;
        if let Some(ref hint) = task_hint {
            candidate_tasks.retain(|task| task.id.to_ascii_lowercase().starts_with(hint));
        }

        let branch_name = branch.trim();
        if !branch_name.is_empty() {
            let branch_matched_tasks: Vec<_> = candidate_tasks
                .iter()
                .filter(|task| task.branch.eq_ignore_ascii_case(branch_name))
                .cloned()
                .collect();
            if !branch_matched_tasks.is_empty() {
                candidate_tasks = branch_matched_tasks;
            }
        }

        let mut inferred_candidates: Vec<(i32, i32, InferredNoMetadataCompletion)> = Vec::new();

        for task in candidate_tasks {
            let terminals = db::models::Terminal::find_by_task(&self.db.pool, &task.id)
                .await
                .map_err(|e| {
                    anyhow::anyhow!("Failed to load terminals for task {}: {e}", task.id)
                })?;

            let working_terminals: Vec<_> = terminals
                .iter()
                .enumerate()
                .filter(|(_, terminal)| terminal.status == "working")
                .collect();

            if working_terminals.len() > 1 {
                tracing::warn!(
                    workflow_id = %workflow_id,
                    task_id = %task.id,
                    working_count = working_terminals.len(),
                    "Skipping task during no-metadata inference because multiple terminals are working"
                );
                continue;
            }

            if let Some((terminal_index, terminal)) = working_terminals.into_iter().next() {
                inferred_candidates.push((
                    task.order_index,
                    terminal.order_index,
                    InferredNoMetadataCompletion {
                        task_id: task.id.clone(),
                        terminal_id: terminal.id.clone(),
                        terminal_index,
                        total_terminals: terminals.len(),
                    },
                ));
            }
        }

        if inferred_candidates.len() == 1 {
            let (_, _, inferred) = inferred_candidates
                .into_iter()
                .next()
                .expect("inferred candidate length checked");
            return Ok(Some(inferred));
        }

        // Secondary disambiguation for concurrent working terminals:
        // if a candidate terminal's recent logs contain this commit hash
        // (usually shown by the CLI right after commit), prefer that terminal.
        if inferred_candidates.len() > 1 {
            let mut hash_matched_candidates: Vec<(i32, i32, InferredNoMetadataCompletion)> =
                Vec::new();
            for (task_order, terminal_order, inferred) in &inferred_candidates {
                if self
                    .terminal_recent_logs_contain_commit_hash(&inferred.terminal_id, commit_hash)
                    .await
                {
                    hash_matched_candidates.push((*task_order, *terminal_order, inferred.clone()));
                }
            }

            if hash_matched_candidates.len() == 1 {
                let (_, _, inferred) = hash_matched_candidates
                    .into_iter()
                    .next()
                    .expect("hash-matched candidate length checked");
                tracing::info!(
                    workflow_id = %workflow_id,
                    commit_hash = %commit_hash,
                    task_id = %inferred.task_id,
                    terminal_id = %inferred.terminal_id,
                    "Resolved ambiguous no-metadata commit via terminal log hash hint"
                );
                return Ok(Some(inferred));
            }

            if hash_matched_candidates.len() > 1 {
                inferred_candidates = hash_matched_candidates;
            }
        }

        if inferred_candidates.len() > 1 && task_hint.is_none() {
            let candidate_count = inferred_candidates.len();
            inferred_candidates.sort_by(|lhs, rhs| {
                lhs.0
                    .cmp(&rhs.0)
                    .then(lhs.1.cmp(&rhs.1))
                    .then(lhs.2.terminal_id.cmp(&rhs.2.terminal_id))
            });
            let (_, _, inferred) = inferred_candidates
                .into_iter()
                .next()
                .expect("inferred candidate length checked");
            tracing::warn!(
                workflow_id = %workflow_id,
                branch = %branch_name,
                candidate_count,
                noop_handoff_pattern = Self::looks_like_noop_handoff_commit(message),
                "Ambiguous no-metadata commit without task hint; selecting deterministic candidate to avoid orchestrator stall"
            );
            return Ok(Some(inferred));
        }

        tracing::warn!(
            workflow_id = %workflow_id,
            commit_hash = %commit_hash,
            branch = %branch_name,
            task_hint = ?task_hint,
            candidate_count = inferred_candidates.len(),
            noop_handoff_pattern = Self::looks_like_noop_handoff_commit(message),
            "Cannot infer unique task/terminal from no-metadata commit"
        );
        Ok(None)
    }

    async fn terminal_recent_logs_contain_commit_hash(
        &self,
        terminal_id: &str,
        commit_hash: &str,
    ) -> bool {
        if commit_hash.trim().is_empty() {
            return false;
        }

        let short_hash: String = commit_hash
            .chars()
            .take(8)
            .collect::<String>()
            .to_ascii_lowercase();
        let full_hash = commit_hash.to_ascii_lowercase();

        let recent_logs = match db::models::terminal::TerminalLog::find_by_terminal(
            &self.db.pool,
            terminal_id,
            Some(240),
        )
        .await
        {
            Ok(logs) => logs,
            Err(error) => {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    commit_hash = %commit_hash,
                    error = %error,
                    "Failed to load terminal logs while resolving no-metadata commit"
                );
                return false;
            }
        };

        recent_logs.iter().any(|log| {
            let content = log.content.to_ascii_lowercase();
            content.contains(&full_hash)
                || (!short_hash.is_empty() && content.contains(&short_hash))
        })
    }

    async fn align_task_state_for_no_metadata_completion(
        &self,
        task_id: &str,
        terminal_index: usize,
        total_terminals: usize,
    ) {
        let mut state = self.state.write().await;
        let terminals = match db::models::Terminal::find_by_task(&self.db.pool, task_id).await {
            Ok(terminals) => terminals,
            Err(error) => {
                tracing::warn!(
                    task_id = %task_id,
                    error = %error,
                    "Failed to sync task terminals while aligning no-metadata completion"
                );
                return;
            }
        };
        let terminal_ids: Vec<String> = terminals.iter().map(|terminal| terminal.id.clone()).collect();
        state.sync_task_terminals(task_id.to_string(), terminal_ids, true);

        if let Some(task_state) = state.task_states.get_mut(task_id) {
            if task_state.total_terminals == 0 && total_terminals == 0 {
                task_state.current_terminal_index = 0;
            } else {
                let safe_total = task_state.total_terminals.max(total_terminals).max(1);
                task_state.current_terminal_index = terminal_index.min(safe_total.saturating_sub(1));
            }
            if task_state.is_completed {
                task_state.is_completed = false;
            }
        }
    }

    /// Handle terminal completed status from git event
    async fn handle_git_terminal_completed(
        &self,
        terminal_id: &str,
        task_id: &str,
        commit_hash: &str,
        commit_message: &str,
    ) -> anyhow::Result<()> {
        tracing::info!(
            "Terminal {} completed task {} (commit: {})",
            terminal_id,
            task_id,
            commit_hash
        );

        let workflow_id = self.state.read().await.workflow_id.clone();
        let event = TerminalCompletionEvent {
            terminal_id: terminal_id.to_string(),
            task_id: task_id.to_string(),
            workflow_id: workflow_id.clone(),
            status: TerminalCompletionStatus::Completed,
            commit_hash: Some(commit_hash.to_string()),
            commit_message: Some(commit_message.to_string()),
            metadata: None,
        };

        self.handle_terminal_completed(event).await?;

        Ok(())
    }

    /// Handle review passed status from git event
    async fn handle_git_review_pass(
        &self,
        reviewer_terminal_id: &str,
        _task_id: &str,
        reviewed_terminal_id: &str,
    ) -> anyhow::Result<()> {
        tracing::info!(
            "Terminal {} approved work from {}",
            reviewer_terminal_id,
            reviewed_terminal_id
        );

        // 1. Update reviewed terminal status
        db::models::Terminal::update_status(
            &self.db.pool,
            reviewed_terminal_id,
            TERMINAL_STATUS_REVIEW_PASSED,
        )
        .await?;

        // 2. Publish review passed event
        let workflow_id = self.state.read().await.workflow_id.clone();
        let event = BusMessage::TerminalStatusUpdate {
            workflow_id: workflow_id.clone(),
            terminal_id: reviewed_terminal_id.to_string(),
            status: TERMINAL_STATUS_REVIEW_PASSED.to_string(),
        };

        self.message_bus
            .publish_workflow_event(&workflow_id, event)
            .await?;

        // 3. Awaken orchestrator to process the event
        self.awaken().await;

        Ok(())
    }

    /// Handle review rejected status from git event
    async fn handle_git_review_reject(
        &self,
        reviewer_terminal_id: &str,
        _task_id: &str,
        reviewed_terminal_id: &str,
        issues: &[CodeIssue],
    ) -> anyhow::Result<()> {
        tracing::warn!(
            "Terminal {} rejected work from {}: {} issues found",
            reviewer_terminal_id,
            reviewed_terminal_id,
            issues.len()
        );

        // 1. Update reviewed terminal status
        db::models::Terminal::update_status(
            &self.db.pool,
            reviewed_terminal_id,
            TERMINAL_STATUS_REVIEW_REJECTED,
        )
        .await?;

        // 2. Publish review rejected event
        let workflow_id = self.state.read().await.workflow_id.clone();
        let event = BusMessage::TerminalStatusUpdate {
            workflow_id: workflow_id.clone(),
            terminal_id: reviewed_terminal_id.to_string(),
            status: TERMINAL_STATUS_REVIEW_REJECTED.to_string(),
        };

        self.message_bus
            .publish_workflow_event(&workflow_id, event)
            .await?;

        // 3. Awaken orchestrator to process the event
        self.awaken().await;

        Ok(())
    }

    /// Handle terminal failed status from git event
    async fn handle_git_terminal_failed(
        &self,
        terminal_id: &str,
        task_id: &str,
        error_message: &str,
    ) -> anyhow::Result<()> {
        tracing::error!(
            "Terminal {} failed task {}: {}",
            terminal_id,
            task_id,
            error_message
        );

        // 1. Update terminal status
        db::models::Terminal::update_status(&self.db.pool, terminal_id, TERMINAL_STATUS_FAILED)
            .await?;

        // 2. Publish failure event
        let workflow_id = self.state.read().await.workflow_id.clone();
        let event = BusMessage::Error {
            workflow_id: workflow_id.clone(),
            error: error_message.to_string(),
        };

        self.message_bus
            .publish_workflow_event(&workflow_id, event)
            .await?;

        // 3. Awaken orchestrator to process the event
        self.awaken().await;

        Ok(())
    }

    /// Awaken the orchestrator to process events
    async fn awaken(&self) {
        // Check if orchestrator is idle and needs to be awakened
        let state = self.state.read().await;
        if state.run_state == OrchestratorRunState::Idle {
            tracing::debug!("Orchestrator is idle, ensuring it processes pending events");
            // Drop the read lock before we potentially do anything else
            drop(state);
        }
        // The orchestrator's event loop will automatically process
        // any messages we published to the message bus
    }

    /// Builds the prompt for a terminal completion event.
    async fn build_completion_prompt(
        &self,
        event: &TerminalCompletionEvent,
    ) -> anyhow::Result<String> {
        let commit_hash = event.commit_hash.as_deref().unwrap_or("N/A");
        let commit_message = event.commit_message.as_deref().unwrap_or("No message");
        let mut prompt = build_terminal_completion_prompt(
            &event.terminal_id,
            &event.task_id,
            commit_hash,
            commit_message,
        );
        if self.is_agent_planned_workflow().await? {
            let workflow = self.load_workflow().await?;
            let context = self.build_agent_planned_context(&workflow).await?;
            prompt.push_str("\n\nAgent-planned runtime context:\n");
            prompt.push_str(&context);
            prompt.push_str(
                "\n\nDecide whether to add more tasks/terminals, start new terminals, close finished terminals, mark the current task complete, or mark workflow planning complete.",
            );
        }
        Ok(prompt)
    }

    fn normalize_instruction_payload(response: &str) -> &str {
        let trimmed = response.trim();
        if trimmed.starts_with("```") && trimmed.ends_with("```") {
            let without_opening = trimmed
                .trim_start_matches("```json")
                .trim_start_matches("```JSON")
                .trim_start_matches("```");
            return without_opening
                .strip_suffix("```")
                .map(str::trim)
                .unwrap_or(without_opening.trim());
        }
        trimmed
    }

    fn parse_instructions(response: &str) -> Option<Vec<OrchestratorInstruction>> {
        let normalized = Self::normalize_instruction_payload(response);
        serde_json::from_str::<Vec<OrchestratorInstruction>>(normalized)
            .ok()
            .or_else(|| {
                serde_json::from_str::<OrchestratorInstruction>(normalized)
                    .ok()
                    .map(|instruction| vec![instruction])
            })
    }

    /// Calls the LLM with the current conversation history.
    async fn call_llm(&self, prompt: &str) -> anyhow::Result<String> {
        let mut state = self.state.write().await;
        state.add_message("user", prompt, &self.config);

        let messages = state.conversation_history.clone();
        drop(state);

        let response = self.llm_client.chat(messages).await?;

        let mut state = self.state.write().await;
        state.add_message("assistant", &response.content, &self.config);
        if let Some(usage) = &response.usage {
            state.total_tokens_used += i64::from(usage.total_tokens);
        }

        Ok(response.content)
    }

    /// Executes orchestrator instructions returned by the LLM.
    pub async fn execute_instruction(&self, response: &str) -> anyhow::Result<()> {
        let Some(instructions) = Self::parse_instructions(response) else {
            tracing::warn!("LLM response did not contain a valid orchestrator instruction payload");
            return Ok(());
        };

        for instruction in instructions {
            self.execute_single_instruction(instruction).await?;
        }

        Ok(())
    }

    async fn execute_single_instruction(
        &self,
        instruction: OrchestratorInstruction,
    ) -> anyhow::Result<()> {
        match instruction {
            OrchestratorInstruction::CreateTask {
                task_id,
                name,
                description,
                branch,
                order_index,
            } => {
                self.ensure_agent_planned_workflow().await?;
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };
                let task = self
                    .runtime_actions()?
                    .create_task(
                        &workflow_id,
                        RuntimeTaskSpec {
                            task_id,
                            name,
                            description,
                            branch,
                            order_index,
                        },
                    )
                    .await?;
                let mut state = self.state.write().await;
                state.sync_task_terminals(task.id.clone(), Vec::new(), false);
            }
            OrchestratorInstruction::CreateTerminal {
                terminal_id,
                task_id,
                cli_type_id,
                model_config_id,
                custom_base_url,
                custom_api_key,
                role,
                role_description,
                order_index,
                auto_confirm,
            } => {
                self.ensure_agent_planned_workflow().await?;
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };
                let planning_complete = self.task_planning_complete(&task_id).await;
                self.runtime_actions()?
                    .create_terminal(
                        &workflow_id,
                        RuntimeTerminalSpec {
                            terminal_id,
                            task_id: task_id.clone(),
                            cli_type_id,
                            model_config_id,
                            custom_base_url,
                            custom_api_key,
                            role,
                            role_description,
                            order_index,
                            auto_confirm,
                        },
                    )
                    .await?;
                self.sync_task_state_from_db(&task_id, Some(planning_complete))
                    .await?;
            }
            OrchestratorInstruction::StartTerminal {
                terminal_id,
                instruction,
            } => {
                self.ensure_agent_planned_workflow().await?;
                let terminal = self.runtime_actions()?.start_terminal(&terminal_id).await?;
                let task_id = terminal.workflow_task_id.clone();
                let planning_complete = self.task_planning_complete(&task_id).await;
                self.sync_task_state_from_db(&task_id, Some(planning_complete))
                    .await?;
                self.dispatch_terminal(&task_id, &terminal, &instruction).await?;
            }
            OrchestratorInstruction::CloseTerminal {
                terminal_id,
                final_status,
            } => {
                let terminal = self
                    .runtime_actions()?
                    .close_terminal(&terminal_id, final_status.as_deref())
                    .await?;
                let task_id = terminal.workflow_task_id.clone();
                let planning_complete = self.task_planning_complete(&task_id).await;
                self.sync_task_state_from_db(&task_id, Some(planning_complete))
                    .await?;
                let mark_success = matches!(terminal.status.as_str(), "completed" | "cancelled");
                {
                    let mut state = self.state.write().await;
                    state.mark_terminal_completed(&task_id, &terminal.id, mark_success);
                }
                self.finalize_task_if_ready(&task_id).await?;
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };
                self.auto_sync_workflow_completion(&workflow_id).await?;
            }
            OrchestratorInstruction::CompleteTask { task_id, summary } => {
                self.ensure_agent_planned_workflow().await?;
                tracing::info!("Marking task {} planning complete: {}", task_id, summary);
                {
                    let mut state = self.state.write().await;
                    state.set_task_planning_complete(&task_id, true);
                }
                self.sync_task_state_from_db(&task_id, Some(true)).await?;
                self.finalize_task_if_ready(&task_id).await?;
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };
                self.auto_sync_workflow_completion(&workflow_id).await?;
            }
            OrchestratorInstruction::SetWorkflowPlanningComplete { summary } => {
                self.ensure_agent_planned_workflow().await?;
                tracing::info!(
                    "Workflow planning completed{}",
                    summary
                        .as_deref()
                        .map(|text| format!(": {text}"))
                        .unwrap_or_default()
                );
                {
                    let mut state = self.state.write().await;
                    state.set_workflow_planning_complete(true);
                }
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };
                self.auto_sync_workflow_completion(&workflow_id).await?;
            }
            OrchestratorInstruction::SendToTerminal {
                terminal_id,
                message,
            } => {
                tracing::info!("Sending to terminal {}: {}", terminal_id, message);

                // 1. Get terminal from database
                let terminal = db::models::Terminal::find_by_id(&self.db.pool, &terminal_id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to get terminal: {e}"))?
                    .ok_or_else(|| anyhow::anyhow!("Terminal {terminal_id} not found"))?;
                // Skip stale send instructions for terminals that are no longer active.
                if terminal.status != "working" {
                    tracing::info!(
                        terminal_id = %terminal.id,
                        status = %terminal.status,
                        "Skipping SendToTerminal instruction because terminal is not in working state"
                    );
                    return Ok(());
                }

                // 2. Get PTY session ID. Missing PTY can happen after process teardown;
                // skip this advisory message instead of crashing the orchestrator runtime.
                let pty_session_id = match terminal.pty_session_id.clone() {
                    Some(session_id) => session_id,
                    None => {
                        tracing::warn!(
                            terminal_id = %terminal.id,
                            status = %terminal.status,
                            "Skipping SendToTerminal instruction because terminal has no PTY session"
                        );
                        return Ok(());
                    }
                };

                // 3. Send message via message bus
                self.message_bus
                    .publish(
                        &pty_session_id,
                        BusMessage::TerminalMessage {
                            message: message.clone(),
                        },
                    )
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to send message: {e}"))?;

                // Fallback submit keystroke: some terminal TUIs keep pasted text in composer
                // until an additional Enter is sent.
                for (attempt, delay_ms) in Self::submit_keystroke_schedule_ms(&terminal, false)
                    .iter()
                    .enumerate()
                {
                    sleep(Duration::from_millis(*delay_ms)).await;
                    self.message_bus
                        .publish_terminal_input(&terminal.id, &pty_session_id, "", None)
                        .await;
                    tracing::debug!(
                        terminal_id = %terminal.id,
                        attempt = attempt + 1,
                        delay_ms,
                        "Sent submit keystroke after SendToTerminal dispatch"
                    );
                }

                tracing::debug!("Message sent to terminal {}", terminal_id);
            }
            OrchestratorInstruction::CompleteWorkflow { summary } => {
                tracing::info!("Completing workflow: {}", summary);

                // Get workflow ID from state
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };

                // Update workflow status to completed
                db::models::Workflow::update_status(
                    &self.db.pool,
                    &workflow_id,
                    WORKFLOW_STATUS_COMPLETED,
                )
                .await
                .map_err(|e| anyhow::anyhow!("Failed to update workflow status: {e}"))?;

                // Publish completion event
                self.message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::StatusUpdate {
                            workflow_id: workflow_id.clone(),
                            status: WORKFLOW_STATUS_COMPLETED.to_string(),
                        },
                    )
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to publish completion event: {e}"))?;

                // Transition to Idle
                self.state.write().await.run_state = OrchestratorRunState::Idle;

                tracing::info!("Workflow {} completed successfully", workflow_id);
            }
            OrchestratorInstruction::FailWorkflow { reason } => {
                tracing::error!("Failing workflow: {}", reason);

                // Get workflow ID from state
                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };

                // Update workflow status to failed
                db::models::Workflow::update_status(
                    &self.db.pool,
                    &workflow_id,
                    WORKFLOW_STATUS_FAILED,
                )
                .await
                .map_err(|e| anyhow::anyhow!("Failed to update workflow status: {e}"))?;

                // Publish failure event
                self.message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::Error {
                            workflow_id: workflow_id.clone(),
                            error: reason.clone(),
                        },
                    )
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to publish failure event: {e}"))?;

                // Transition to Idle
                self.state.write().await.run_state = OrchestratorRunState::Idle;

                tracing::error!("Workflow {} failed: {}", workflow_id, reason);
            }
            OrchestratorInstruction::StartTask {
                task_id,
                instruction,
            } => {
                tracing::info!("Starting task {}: {}", task_id, instruction);

                // 1. Get task from database
                let task = db::models::WorkflowTask::find_by_id(&self.db.pool, &task_id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to get task: {e}"))?
                    .ok_or_else(|| anyhow::anyhow!("Task {task_id} not found"))?;

                // 2. Get terminals for this task
                let terminals = db::models::Terminal::find_by_task(&self.db.pool, &task_id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to get terminals: {e}"))?;

                if terminals.is_empty() {
                    return Err(anyhow::anyhow!("No terminals found for task {task_id}"));
                }

                // 3. Initialize task state if not already done
                {
                    let mut state = self.state.write().await;
                    if !state.task_states.contains_key(&task_id) {
                        state.init_task(
                            task_id.clone(),
                            terminals.iter().map(|terminal| terminal.id.clone()).collect(),
                        );
                    } else {
                        state.sync_task_terminals(
                            task_id.clone(),
                            terminals.iter().map(|terminal| terminal.id.clone()).collect(),
                            true,
                        );
                    }
                }

                // 4. Get next terminal index
                let next_index = {
                    let state = self.state.read().await;
                    state.get_next_terminal_for_task(&task_id)
                };

                // 5. Dispatch the terminal
                if let Some(index) = next_index {
                    let terminal = terminals.get(index).cloned().ok_or_else(|| {
                        anyhow::anyhow!(
                            "Terminal index {index} out of range for task {task_id}"
                        )
                    })?;
                    self.dispatch_terminal(&task.id, &terminal, &instruction)
                        .await?;
                } else {
                    tracing::info!("No pending terminals for task {task_id}");
                }
            }
            OrchestratorInstruction::ReviewCode { .. }
            | OrchestratorInstruction::FixIssues { .. }
            | OrchestratorInstruction::MergeBranch { .. }
            | OrchestratorInstruction::PauseWorkflow { .. } => {
                tracing::warn!(
                    "Instruction variant is parsed but not yet implemented in execute_single_instruction"
                );
            }
        }

        Ok(())
    }

    /// Dispatches a terminal with the given instruction.
    ///
    /// Updates terminal and task status, then sends the instruction to the PTY session.
    /// If the terminal has no PTY session, marks both terminal and task as failed.
    /// Skips dispatch if terminal is not in "waiting" status.
    async fn dispatch_terminal(
        &self,
        task_id: &str,
        terminal: &db::models::Terminal,
        instruction: &str,
    ) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // 1. CAS update terminal status waiting -> working.
        let dispatch_acquired = db::models::Terminal::update_status_cas(
            &self.db.pool,
            &terminal.id,
            "waiting",
            "working",
        )
        .await
        .map_err(|e| anyhow::anyhow!("Failed to update terminal status with CAS: {e}"))?;

        if !dispatch_acquired {
            tracing::info!(
                terminal_id = %terminal.id,
                task_id = %task_id,
                "Skipping dispatch because terminal CAS waiting->working failed"
            );
            return Ok(());
        }

        // 2. Refresh terminal snapshot after CAS to avoid stale PTY/session metadata.
        let active_terminal = db::models::Terminal::find_by_id(&self.db.pool, &terminal.id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to reload terminal after CAS: {e}"))?
            .ok_or_else(|| {
                anyhow::anyhow!(
                    "Terminal {} not found after CAS dispatch acquisition",
                    terminal.id
                )
            })?;

        {
            let mut state = self.state.write().await;
            state.mark_terminal_dispatched(task_id, &active_terminal.id);
        }

        // 3. Get PTY session ID, fail if not available.
        let pty_session_id = match active_terminal.pty_session_id.as_deref() {
            Some(id) => id.to_string(),
            None => {
                let error_msg = format!(
                    "Terminal {} has no PTY session, marking as failed",
                    active_terminal.id
                );
                tracing::error!("{}", error_msg);

                // Mark terminal as failed
                let _ = db::models::Terminal::update_status(
                    &self.db.pool,
                    &active_terminal.id,
                    TERMINAL_STATUS_FAILED,
                )
                .await;

                // Broadcast terminal status update
                let _ = self
                    .message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::TerminalStatusUpdate {
                            workflow_id: workflow_id.clone(),
                            terminal_id: active_terminal.id.clone(),
                            status: TERMINAL_STATUS_FAILED.to_string(),
                        },
                    )
                    .await;

                // Mark task as failed
                let _ =
                    db::models::WorkflowTask::update_status(&self.db.pool, task_id, "failed").await;

                // Broadcast task status update
                let _ = self
                    .message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::TaskStatusUpdate {
                            workflow_id: workflow_id.clone(),
                            task_id: task_id.to_string(),
                            status: "failed".to_string(),
                        },
                    )
                    .await;

                // Broadcast error event for UI notification
                let _ = self
                    .message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::Error {
                            workflow_id: workflow_id.clone(),
                            error: error_msg.clone(),
                        },
                    )
                    .await;

                return Err(anyhow::anyhow!(
                    "Terminal {} has no PTY session",
                    active_terminal.id
                ));
            }
        };

        // 4. Update task status to running.
        db::models::WorkflowTask::update_status(&self.db.pool, task_id, "running")
            .await
            .map_err(|e| anyhow::anyhow!("Failed to update task status: {e}"))?;

        // 5. Broadcast live status updates for UI.
        let _ = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TerminalStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    terminal_id: active_terminal.id.clone(),
                    status: "working".to_string(),
                },
            )
            .await;
        let _ = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TaskStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    task_id: task_id.to_string(),
                    status: "running".to_string(),
                },
            )
            .await;

        if Self::needs_explicit_submit(&active_terminal) {
            self.message_bus
                .publish_terminal_input(&active_terminal.id, &pty_session_id, instruction, None)
                .await;
        } else {
            // 6. Send instruction to PTY session.
            self.message_bus
                .publish(
                    &pty_session_id,
                    BusMessage::TerminalMessage {
                        message: instruction.to_string(),
                    },
                )
                .await
                .map_err(|e| anyhow::anyhow!("Failed to send instruction to terminal: {e}"))?;
        }

        // Fallback submit keystroke: some terminal TUIs keep pasted text in composer
        // until an additional Enter is sent.
        for (attempt, delay_ms) in Self::submit_keystroke_schedule_ms(&active_terminal, true)
            .iter()
            .enumerate()
        {
            sleep(Duration::from_millis(*delay_ms)).await;
            self.message_bus
                .publish_terminal_input(&active_terminal.id, &pty_session_id, "", None)
                .await;
            tracing::debug!(
                terminal_id = %active_terminal.id,
                cli_type_id = %active_terminal.cli_type_id,
                attempt = attempt + 1,
                delay_ms,
                "Sent submit keystroke after terminal instruction dispatch"
            );
        }

        tracing::info!(
            "Dispatched terminal {} for task {} with instruction: {}",
            active_terminal.id,
            task_id,
            instruction
        );

        Ok(())
    }

    async fn enforce_terminal_completion_shutdown(
        &self,
        workflow_id: &str,
        terminal: &db::models::Terminal,
    ) {
        let pty_session_id = terminal
            .pty_session_id
            .as_deref()
            .map(str::trim)
            .filter(|session_id| !session_id.is_empty())
            .map(str::to_string);

        if let Some(session_id) = pty_session_id.as_deref() {
            let _ = self
                .message_bus
                .publish(
                    session_id,
                    BusMessage::TerminalInput {
                        terminal_id: terminal.id.clone(),
                        session_id: session_id.to_string(),
                        input: "\u{3}".to_string(),
                        decision: None,
                    },
                )
                .await;

            let _ = self
                .message_bus
                .publish(session_id, BusMessage::Shutdown)
                .await;
            tracing::info!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                pty_session_id = %session_id,
                "Issued completion shutdown signals for terminal"
            );
        }

        if let Some(pid) = terminal.process_id {
            if let Err(error) = self.force_terminate_terminal_process(pid) {
                tracing::warn!(
                    terminal_id = %terminal.id,
                    workflow_id = %workflow_id,
                    process_id = pid,
                    error = %error,
                    "Failed to force terminate terminal process during completion"
                );
            } else {
                tracing::info!(
                    terminal_id = %terminal.id,
                    workflow_id = %workflow_id,
                    process_id = pid,
                    "Force-terminated terminal process during completion"
                );
            }
        }

        if let Err(e) =
            db::models::Terminal::update_process(&self.db.pool, &terminal.id, None, None).await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                error = %e,
                "Failed to clear terminal process binding after completion"
            );
        }

        if let Err(e) =
            db::models::Terminal::update_session(&self.db.pool, &terminal.id, None, None).await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                error = %e,
                "Failed to clear terminal session binding after completion"
            );
        }
    }

    fn force_terminate_terminal_process(&self, pid: i32) -> anyhow::Result<()> {
        if pid <= 0 {
            return Err(anyhow!("invalid process id: {}", pid));
        }

        #[cfg(unix)]
        {
            use nix::sys::signal::{self, Signal};

            let target_pid = Pid::from_raw(pid);

            if signal::kill(target_pid, None).is_err() {
                return Ok(());
            }

            signal::kill(target_pid, Signal::SIGTERM)
                .map_err(|e| anyhow!("failed to send SIGTERM to {}: {}", pid, e))?;
            std::thread::sleep(std::time::Duration::from_millis(150));
            let _ = signal::kill(target_pid, Signal::SIGKILL);
            return Ok(());
        }

        #[cfg(windows)]
        {
            let output = std::process::Command::new("taskkill")
                .args(["/PID", &pid.to_string(), "/T", "/F"])
                .output()
                .map_err(|e| anyhow!("failed to execute taskkill: {}", e))?;

            if output.status.success() {
                return Ok(());
            }

            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            if stderr.contains("not found")
                || stderr.contains("not running")
                || stderr.contains("No running instance")
                || stderr.contains("no running instance")
            {
                return Ok(());
            }

            return Err(anyhow!("taskkill failed for {}: {}", pid, stderr));
        }

        #[allow(unreachable_code)]
        Ok(())
    }

    fn needs_explicit_submit(terminal: &db::models::Terminal) -> bool {
        terminal.cli_type_id.to_ascii_lowercase().contains("codex")
    }

    fn is_claude_code_cli(terminal: &db::models::Terminal) -> bool {
        terminal
            .cli_type_id
            .to_ascii_lowercase()
            .contains("claude-code")
    }

    fn submit_keystroke_schedule_ms(
        terminal: &db::models::Terminal,
        is_initial_dispatch: bool,
    ) -> &'static [u64] {
        if Self::needs_explicit_submit(terminal) {
            &[120, 360, 900]
        } else if is_initial_dispatch && Self::is_claude_code_cli(terminal) {
            // Claude Code occasionally leaves the first pasted prompt in composer without
            // submission on cold start; send one delayed Enter only for initial dispatch.
            &[420]
        } else {
            // Non-Codex CLIs receive the instruction as a single message payload that already
            // includes a submit key. Extra synthetic Enter keystrokes can race startup TUIs and
            // accidentally submit partial/empty prompts.
            &[]
        }
    }

    fn stall_recovery_submit_keystroke_schedule_ms(
        terminal: &db::models::Terminal,
    ) -> &'static [u64] {
        if Self::is_claude_code_cli(terminal) {
            // Stall recovery means we've seen a prolonged quiet window while terminal is
            // still marked working. Claude can occasionally keep the injected recovery
            // instruction in composer; send one delayed submit to force execution.
            &[Self::STALL_RECOVERY_CLAUDE_SUBMIT_DELAY_MS]
        } else {
            Self::submit_keystroke_schedule_ms(terminal, false)
        }
    }

    /// Builds a task instruction from task and terminal information.
    fn build_task_instruction(
        workflow_id: &str,
        task: &db::models::WorkflowTask,
        terminal: &db::models::Terminal,
        total_terminals: usize,
    ) -> String {
        let mut parts = vec![format!("Start task: {} ({})", task.name, task.id)];

        if let Some(description) = &task.description {
            let normalized = description.split_whitespace().collect::<Vec<_>>().join(" ");
            if !normalized.is_empty() {
                if total_terminals > 1 {
                    let summary = Self::truncate_instruction_text(&normalized, 200);
                    parts.push(format!("Task objective: {}", summary));
                } else {
                    parts.push(format!("Task description: {}", normalized));
                }
            }
        }

        if let Some(role) = &terminal.role {
            let role = role.trim();
            if !role.is_empty() {
                parts.push(format!("Your role: {role}"));
            }
        }

        if let Some(role_description) = &terminal.role_description {
            let normalized = role_description
                .split_whitespace()
                .collect::<Vec<_>>()
                .join(" ");
            if !normalized.is_empty() {
                parts.push(format!("Role description: {}", normalized));
            }
        }

        if total_terminals > 1 {
            let terminal_order = (terminal.order_index + 1).max(1);
            parts.push(format!(
                "Execution context: terminal {terminal_order}/{total_terminals}."
            ));
            parts.push(
                "Focus only on your scoped role and do not take over work assigned to other terminals."
                    .to_string(),
            );
            parts.push(
                "When finished, leave concise handoff notes for the next terminal.".to_string(),
            );
        }

        let terminal_order = (terminal.order_index + 1).max(1);
        let commit_metadata_template = format!(
            "{separator}\\nworkflow_id: {workflow_id}\\ntask_id: {task_id}\\nterminal_id: {terminal_id}\\nterminal_order: {terminal_order}\\nstatus: completed\\nnext_action: handoff",
            separator = GIT_COMMIT_METADATA_SEPARATOR,
            task_id = task.id,
            terminal_id = terminal.id,
            terminal_order = terminal_order,
        );
        parts.push(
            "Completion contract: when your scoped work is done, you MUST create a git commit before stopping."
                .to_string(),
        );
        parts.push(format!(
            "Commit message must include this metadata block exactly: {commit_metadata_template}"
        ));
        parts.push(
            "If there are no file changes, create an empty commit with --allow-empty so GitWatcher/Orchestrator can advance."
                .to_string(),
        );
        parts.push(
            "If the current branch is already the integration branch, commit directly and do not create an extra branch or redundant self-merge."
                .to_string(),
        );

        parts.push("Please start implementing immediately.".to_string());

        parts.join(" | ")
    }

    fn truncate_instruction_text(input: &str, max_chars: usize) -> String {
        let char_count = input.chars().count();
        if char_count <= max_chars {
            return input.to_string();
        }

        let truncated: String = input.chars().take(max_chars).collect();
        format!("{truncated}...")
    }

    /// Auto-dispatches the first terminal for each task when workflow starts.
    ///
    /// This method is called after the workflow enters running state to automatically
    /// start execution of all tasks by dispatching their first terminals.
    async fn auto_dispatch_initial_tasks(&self) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // Get all tasks for this workflow
        let tasks = db::models::WorkflowTask::find_by_workflow(&self.db.pool, &workflow_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get workflow tasks: {e}"))?;

        if tasks.is_empty() {
            tracing::info!(
                "No tasks found for workflow {}, skipping auto-dispatch",
                workflow_id
            );
            return Ok(());
        }

        tracing::info!(
            "Auto-dispatching initial terminals for {} tasks in workflow {}",
            tasks.len(),
            workflow_id
        );

        for task in tasks {
            // Skip tasks that are already completed, failed, or cancelled
            if task.status == "completed" || task.status == "failed" || task.status == "cancelled" {
                tracing::debug!("Skipping task {} due to status {}", task.id, task.status);
                continue;
            }

            // Get terminals for this task
            let terminals = db::models::Terminal::find_by_task(&self.db.pool, &task.id)
                .await
                .map_err(|e| {
                    anyhow::anyhow!("Failed to get terminals for task {}: {e}", task.id)
                })?;

            if terminals.is_empty() {
                tracing::warn!("No terminals found for task {}, skipping", task.id);
                continue;
            }

            // Initialize task state
            {
                let mut state = self.state.write().await;
                if !state.task_states.contains_key(&task.id) {
                    state.init_task(
                        task.id.clone(),
                        terminals.iter().map(|terminal| terminal.id.clone()).collect(),
                    );
                } else {
                    state.sync_task_terminals(
                        task.id.clone(),
                        terminals.iter().map(|terminal| terminal.id.clone()).collect(),
                        true,
                    );
                }
            }

            // Get next terminal index (should be 0 for initial dispatch)
            let next_index = {
                let state = self.state.read().await;
                state.get_next_terminal_for_task(&task.id)
            };

            let Some(index) = next_index else {
                tracing::debug!("No pending terminals for task {}", task.id);
                continue;
            };

            let Some(terminal) = terminals.get(index).cloned() else {
                tracing::warn!("Terminal index {} out of range for task {}", index, task.id);
                continue;
            };

            // Only dispatch terminals in waiting status
            if terminal.status != "waiting" {
                tracing::debug!(
                    "Skipping terminal {} for task {} due to status {}",
                    terminal.id,
                    task.id,
                    terminal.status
                );
                continue;
            }

            // Build and dispatch instruction
            let instruction =
                Self::build_task_instruction(&workflow_id, &task, &terminal, terminals.len());
            if let Err(e) = self
                .dispatch_terminal(&task.id, &terminal, &instruction)
                .await
            {
                tracing::error!(
                    "Failed to auto-dispatch terminal {} for task {}: {}",
                    terminal.id,
                    task.id,
                    e
                );
                // Continue with other tasks even if one fails
            }
        }

        Ok(())
    }

    /// Broadcast workflow status update
    ///
    /// Updates the workflow status in the database and broadcasts
    /// a StatusUpdate message to the workflow's message bus topic.
    pub async fn broadcast_workflow_status(&self, status: &str) -> anyhow::Result<()> {
        // 1. Get workflow_id
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // 2. Update database (synchronously await result)
        db::models::Workflow::update_status(&self.db.pool, &workflow_id, status).await?;

        // 3. Publish to message bus (synchronously await result)
        let message = BusMessage::StatusUpdate {
            workflow_id: workflow_id.clone(),
            status: status.to_string(),
        };
        self.message_bus
            .publish_workflow_event(&workflow_id, message)
            .await?;

        tracing::debug!("Broadcast workflow status: {} -> {}", workflow_id, status);

        Ok(())
    }

    /// Broadcast terminal status update
    ///
    /// Updates the terminal status in the database and broadcasts
    /// a TerminalStatusUpdate message to the workflow's message bus topic.
    pub async fn broadcast_terminal_status(
        &self,
        terminal_id: &str,
        status: &str,
    ) -> anyhow::Result<()> {
        // 1. Get workflow_id
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // 2. Update database (synchronously await result)
        db::models::Terminal::update_status(&self.db.pool, terminal_id, status).await?;

        // 3. Publish to message bus (synchronously await result)
        let message = BusMessage::TerminalStatusUpdate {
            workflow_id: workflow_id.clone(),
            terminal_id: terminal_id.to_string(),
            status: status.to_string(),
        };
        self.message_bus
            .publish_workflow_event(&workflow_id, message)
            .await?;

        tracing::debug!("Broadcast terminal status: {} -> {}", terminal_id, status);

        Ok(())
    }

    /// Broadcast task status update
    ///
    /// Updates the task status in the database and broadcasts
    /// a TaskStatusUpdate message to the workflow's message bus topic.
    pub async fn broadcast_task_status(&self, task_id: &str, status: &str) -> anyhow::Result<()> {
        // 1. Get workflow_id
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // 2. Update database (synchronously await result)
        db::models::WorkflowTask::update_status(&self.db.pool, task_id, status).await?;

        // 3. Publish to message bus (synchronously await result)
        let message = BusMessage::TaskStatusUpdate {
            workflow_id: workflow_id.clone(),
            task_id: task_id.to_string(),
            status: status.to_string(),
        };
        self.message_bus
            .publish_workflow_event(&workflow_id, message)
            .await?;

        tracing::debug!("Broadcast task status: {} -> {}", task_id, status);

        Ok(())
    }

    async fn auto_sync_workflow_completion(&self, workflow_id: &str) -> anyhow::Result<()> {
        let Some(workflow) = db::models::Workflow::find_by_id(&self.db.pool, workflow_id).await?
        else {
            return Ok(());
        };

        if matches!(
            workflow.status.as_str(),
            WORKFLOW_STATUS_COMPLETED | WORKFLOW_STATUS_FAILED
        ) {
            return Ok(());
        }

        let planning_complete = {
            let state = self.state.read().await;
            state.workflow_planning_complete
        };
        if !planning_complete {
            return Ok(());
        }

        let tasks = db::models::WorkflowTask::find_by_workflow(&self.db.pool, workflow_id).await?;
        let terminals = db::models::Terminal::find_by_workflow(&self.db.pool, workflow_id).await?;
        let has_runnable_terminals = terminals.iter().any(|terminal| {
            matches!(
                terminal.status.as_str(),
                "not_started" | "starting" | "waiting" | "working"
            )
        });
        if has_runnable_terminals {
            return Ok(());
        }
        if !tasks.is_empty() && tasks.iter().any(|task| task.status != "completed") {
            return Ok(());
        }

        db::models::Workflow::update_status(&self.db.pool, workflow_id, WORKFLOW_STATUS_COMPLETED)
            .await?;

        let _ = self
            .message_bus
            .publish_workflow_event(
                workflow_id,
                BusMessage::StatusUpdate {
                    workflow_id: workflow_id.to_string(),
                    status: WORKFLOW_STATUS_COMPLETED.to_string(),
                },
            )
            .await;

        tracing::info!(
            workflow_id = %workflow_id,
            "Workflow auto-synced to completed after all tasks completed"
        );

        Ok(())
    }

    /// Triggers merge of all completed task branches into the target branch.
    ///
    /// Called when all terminals for a task have completed successfully.
    /// Merges each task branch into the target branch using squash merge.
    ///
    /// # Arguments
    /// * `task_branches` - Map of task_id to branch name for all completed tasks
    /// * `base_repo_path` - Path to the base repository
    /// * `target_branch` - Target branch name (e.g., "main")
    ///
    /// # Returns
    /// * `Ok(())` - All merges completed successfully
    /// * `Err(anyhow::Error)` - If any merge fails
    pub async fn trigger_merge(
        &self,
        task_branches: HashMap<String, String>,
        base_repo_path: &str,
        target_branch: &str,
    ) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        tracing::info!(
            "Triggering merge for {} task branches into {}",
            task_branches.len(),
            target_branch
        );

        let base_repo_path = std::path::Path::new(base_repo_path);
        let git_service = crate::services::git::GitService::new();

        // Merge each task branch
        for (task_id, task_branch) in task_branches {
            if task_branch.eq_ignore_ascii_case(target_branch) {
                tracing::info!(
                    "Skipping merge for task {} because task branch '{}' already equals target branch '{}'.",
                    task_id,
                    task_branch,
                    target_branch
                );
                continue;
            }

            tracing::info!("Merging task branch {} for task {}", task_branch, task_id);

            // Determine task worktree path
            let task_worktree_path = base_repo_path.join("worktrees").join(&task_branch);

            // Perform the merge
            let commit_message = format!("Merge task {} ({})", task_id, task_branch);
            match git_service.merge_changes(
                base_repo_path,
                &task_worktree_path,
                &task_branch,
                target_branch,
                &commit_message,
            ) {
                Ok(commit_sha) => {
                    tracing::info!(
                        "Successfully merged task branch {}: {}",
                        task_branch,
                        commit_sha
                    );

                    // Broadcast merge success for this task
                    let message = BusMessage::StatusUpdate {
                        workflow_id: workflow_id.clone(),
                        status: WORKFLOW_STATUS_COMPLETED.to_string(),
                    };
                    self.message_bus
                        .publish_workflow_event(&workflow_id, message)
                        .await?;
                }
                Err(e) => {
                    // Check if this is a merge conflict
                    let is_conflict =
                        matches!(e, crate::services::git::GitServiceError::MergeConflicts(_));

                    if is_conflict {
                        tracing::warn!(
                            "Merge conflict detected for task branch {}: {}",
                            task_branch,
                            e
                        );

                        // Update workflow status to "merging"
                        db::models::Workflow::update_status(
                            &self.db.pool,
                            &workflow_id,
                            WORKFLOW_STATUS_MERGING,
                        )
                        .await?;

                        // Broadcast merging status
                        let message = BusMessage::StatusUpdate {
                            workflow_id: workflow_id.clone(),
                            status: WORKFLOW_STATUS_MERGING.to_string(),
                        };
                        self.message_bus
                            .publish_workflow_event(&workflow_id, message)
                            .await?;

                        return Err(anyhow::anyhow!(
                            "Merge conflict detected for task branch {}: {}",
                            task_branch,
                            e
                        ));
                    }

                    // Other error - fail workflow
                    tracing::error!("Merge failed for task branch {}: {}", task_branch, e);

                    db::models::Workflow::update_status(
                        &self.db.pool,
                        &workflow_id,
                        WORKFLOW_STATUS_FAILED,
                    )
                    .await?;

                    let message = BusMessage::Error {
                        workflow_id: workflow_id.clone(),
                        error: format!("Merge failed for task {}: {}", task_id, e),
                    };
                    self.message_bus
                        .publish_workflow_event(&workflow_id, message)
                        .await?;

                    return Err(anyhow::anyhow!(
                        "Merge failed for task branch {}: {}",
                        task_branch,
                        e
                    ));
                }
            }
        }

        tracing::info!(
            "All task branches merged successfully into {}",
            target_branch
        );

        Ok(())
    }

    /// Handle terminal failure
    ///
    /// Wrapper around ErrorHandler::handle_terminal_failure that uses
    /// the agent's workflow_id, message_bus, and db.
    pub async fn handle_terminal_failure(
        &self,
        task_id: &str,
        terminal_id: &str,
        error_message: &str,
    ) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        self.error_handler
            .handle_terminal_failure(&workflow_id, task_id, terminal_id, error_message)
            .await
    }

    /// Handle user response for an interactive terminal prompt.
    ///
    /// Wrapper around PromptHandler::handle_user_approval that resolves
    /// workflow_id and terminal session_id from the agent/database context.
    pub async fn handle_user_prompt_response(
        &self,
        terminal_id: &str,
        user_response: &str,
    ) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        let terminal = db::models::Terminal::find_by_id(&self.db.pool, terminal_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get terminal: {e}"))?
            .ok_or_else(|| anyhow::anyhow!("Terminal {terminal_id} not found"))?;

        let workflow_task_id = terminal.workflow_task_id.clone();
        let task = db::models::WorkflowTask::find_by_id(&self.db.pool, &workflow_task_id)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get workflow task: {e}"))?
            .ok_or_else(|| {
                anyhow::anyhow!(
                    "Workflow task {} not found for terminal {}",
                    workflow_task_id,
                    terminal_id
                )
            })?;

        if task.workflow_id != workflow_id {
            return Err(anyhow!(
                "Terminal {} does not belong to workflow {}",
                terminal_id,
                workflow_id
            ));
        }

        let session_id = terminal
            .pty_session_id
            .or(terminal.session_id)
            .filter(|id| !id.trim().is_empty())
            .ok_or_else(|| {
                anyhow!(
                    "Terminal {} has no session_id for prompt response",
                    terminal_id
                )
            })?;

        let handled = self
            .prompt_handler
            .handle_user_prompt_response(terminal_id, &session_id, &workflow_id, user_response)
            .await;

        if !handled {
            return Err(anyhow!(
                "Terminal {} is not waiting for prompt approval in workflow {}",
                terminal_id,
                workflow_id
            ));
        }

        Ok(())
    }

    /// Execute slash commands for this workflow
    ///
    /// Loads all slash commands associated with the workflow, renders their
    /// templates with custom parameters and workflow context, and sends the
    /// rendered prompts to the LLM.
    ///
    /// This should be called once when the agent starts, before processing
    /// any terminal events.
    pub async fn execute_slash_commands(&self) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        // Load workflow to check if slash commands are enabled
        let workflow = db::models::Workflow::find_by_id(&self.db.pool, &workflow_id)
            .await?
            .ok_or_else(|| anyhow::anyhow!("Workflow {} not found", workflow_id))?;

        if !workflow.use_slash_commands {
            tracing::info!("Slash commands disabled for workflow {}", workflow_id);
            return Ok(());
        }

        // Load workflow commands
        let commands =
            db::models::WorkflowCommand::find_by_workflow(&self.db.pool, &workflow_id).await?;

        if commands.is_empty() {
            tracing::info!("No slash commands configured for workflow {}", workflow_id);
            return Ok(());
        }

        // Load all presets
        let all_presets = db::models::SlashCommandPreset::find_all(&self.db.pool).await?;

        tracing::info!(
            "Executing {} slash command(s) for workflow {}",
            commands.len(),
            workflow_id
        );

        // Create template renderer
        let renderer = TemplateRenderer::new();

        // Create workflow context
        let workflow_ctx = WorkflowContext::new(
            workflow.name.clone(),
            workflow.description.clone(),
            workflow.target_branch.clone(),
        );

        // Execute each command in order
        for (index, cmd) in commands.iter().enumerate() {
            // Find the preset for this command
            let preset = all_presets
                .iter()
                .find(|p| p.id == cmd.preset_id)
                .ok_or_else(|| anyhow::anyhow!("Preset {} not found for command", cmd.preset_id))?;

            let template = preset.prompt_template.as_deref().unwrap_or("");

            // Render the template with custom params and workflow context
            let rendered_prompt = renderer
                .render(template, cmd.custom_params.as_deref(), Some(&workflow_ctx))
                .map_err(|e| {
                    anyhow::anyhow!(
                        "Failed to render template for command {}: {} (index {})",
                        preset.command,
                        e,
                        index
                    )
                })?;

            tracing::info!(
                "Executing slash command {}: {} (index {})",
                preset.command,
                index,
                cmd.order_index
            );

            // Add rendered prompt as user message to conversation
            {
                let mut state = self.state.write().await;
                state.add_message("user", &rendered_prompt, &self.config);
            }

            // Send to LLM and get response
            let response = self
                .llm_client
                .chat({
                    let state = self.state.read().await;
                    state.conversation_history.clone()
                })
                .await?;

            // Add assistant response to conversation
            {
                let mut state = self.state.write().await;
                state.add_message("assistant", &response.content, &self.config);
                if let Some(usage) = &response.usage {
                    state.total_tokens_used += i64::from(usage.total_tokens);
                }
            }

            tracing::info!(
                "Slash command {} completed. LLM response: {} chars",
                preset.command,
                response.content.len()
            );
        }

        tracing::info!("All slash commands executed for workflow {}", workflow_id);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::{path::PathBuf, sync::Arc};

    use chrono::Utc;
    use db::DBService;
    use sqlx::sqlite::SqlitePoolOptions;
    use uuid::Uuid;

    use super::{OrchestratorAgent, StallRecoveryTracker};
    use crate::services::orchestrator::{
        BusMessage, MessageBus, MockLLMClient, OrchestratorConfig,
    };

    fn make_task(description: Option<&str>) -> db::models::WorkflowTask {
        let now = Utc::now();
        db::models::WorkflowTask {
            id: "task-1".to_string(),
            workflow_id: "workflow-1".to_string(),
            vk_task_id: None,
            name: "Implement feature".to_string(),
            description: description.map(str::to_string),
            branch: "workflow/workflow-1/implement-feature".to_string(),
            status: "pending".to_string(),
            order_index: 0,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
        }
    }

    fn make_terminal(order_index: i32) -> db::models::Terminal {
        let now = Utc::now();
        db::models::Terminal {
            id: format!("terminal-{order_index}"),
            workflow_task_id: "task-1".to_string(),
            cli_type_id: "cli-codex".to_string(),
            model_config_id: "model-1".to_string(),
            custom_base_url: None,
            custom_api_key: None,
            role: Some("backend developer".to_string()),
            role_description: Some("Implement backend service only".to_string()),
            order_index,
            status: "waiting".to_string(),
            process_id: None,
            pty_session_id: None,
            session_id: None,
            execution_process_id: None,
            vk_session_id: None,
            auto_confirm: true,
            last_commit_hash: None,
            last_commit_message: None,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
        }
    }

    #[test]
    fn build_task_instruction_for_multi_terminal_uses_objective_not_full_description() {
        let workflow_id = "workflow-1";
        let task = make_task(Some(
            "Build a local guestbook with frontend and backend, persist to local json file and display in UI",
        ));
        let terminal = make_terminal(0);

        let instruction =
            OrchestratorAgent::build_task_instruction(workflow_id, &task, &terminal, 3);

        assert!(instruction.contains("Task objective:"));
        assert!(!instruction.contains("Task description:"));
        assert!(instruction.contains("Execution context: terminal 1/3."));
        assert!(instruction.contains("Focus only on your scoped role"));
        assert!(instruction.contains("Completion contract:"));
        assert!(instruction.contains("workflow_id: workflow-1"));
        assert!(instruction.contains("task_id: task-1"));
        assert!(instruction.contains("terminal_id: terminal-0"));
        assert!(instruction.contains("status: completed"));
        assert!(instruction.contains("next_action: handoff"));
        assert!(instruction.contains("do not create an extra branch"));
    }

    #[test]
    fn build_task_instruction_for_single_terminal_keeps_full_description() {
        let workflow_id = "workflow-1";
        let task = make_task(Some("Complete full implementation end-to-end"));
        let terminal = make_terminal(0);

        let instruction =
            OrchestratorAgent::build_task_instruction(workflow_id, &task, &terminal, 1);

        assert!(instruction.contains("Task description: Complete full implementation end-to-end"));
        assert!(!instruction.contains("Task objective:"));
        assert!(!instruction.contains("Execution context:"));
        assert!(instruction.contains("Completion contract:"));
        assert!(instruction.contains("do not create an extra branch"));
    }

    #[test]
    fn truncate_instruction_text_limits_length_with_ellipsis() {
        let input = "a".repeat(260);
        let result = OrchestratorAgent::truncate_instruction_text(&input, 200);

        assert_eq!(result.chars().count(), 203);
        assert!(result.ends_with("..."));
    }

    #[test]
    fn should_skip_completed_handoff_for_continue_and_retry() {
        assert!(OrchestratorAgent::should_skip_completed_handoff("continue"));
        assert!(OrchestratorAgent::should_skip_completed_handoff("retry"));
        assert!(OrchestratorAgent::should_skip_completed_handoff(
            " Continue "
        ));
        assert!(!OrchestratorAgent::should_skip_completed_handoff("handoff"));
        assert!(!OrchestratorAgent::should_skip_completed_handoff(""));
    }

    async fn setup_stalled_recovery_fixture() -> (
        OrchestratorAgent,
        Arc<MessageBus>,
        Arc<DBService>,
        String,
        String,
    ) {
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });

        let workflow_id = Uuid::new_v4().to_string();
        let task_id = Uuid::new_v4().to_string();
        let terminal_id = Uuid::new_v4().to_string();
        let pty_session_id = Uuid::new_v4().to_string();

        let now = Utc::now();
        let stale_time = now - chrono::Duration::seconds(90);

        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("test-project")
        .bind(now)
        .bind(now)
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            INSERT INTO workflow (
                id, project_id, name, status, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            "#,
        )
        .bind(&workflow_id)
        .bind(project_id)
        .bind("test-workflow")
        .bind("running")
        .bind("main")
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(now)
        .bind(now)
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            INSERT INTO workflow_task (
                id, workflow_id, name, branch, status, order_index,
                started_at, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            "#,
        )
        .bind(&task_id)
        .bind(&workflow_id)
        .bind("test-task")
        .bind("feature/test")
        .bind("running")
        .bind(0)
        .bind(stale_time)
        .bind(now)
        .bind(now)
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            INSERT INTO terminal (
                id, workflow_task_id, cli_type_id, model_config_id,
                order_index, status, pty_session_id, started_at, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
            "#,
        )
        .bind(&terminal_id)
        .bind(&task_id)
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(0)
        .bind("working")
        .bind(&pty_session_id)
        .bind(stale_time)
        .bind(now)
        .bind(now)
        .execute(&pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id,
            message_bus.clone(),
            db.clone(),
            Box::new(MockLLMClient::new()),
        )
        .unwrap();

        (agent, message_bus, db, terminal_id, pty_session_id)
    }

    #[tokio::test]
    async fn recover_stalled_terminals_redispatches_instruction() {
        let (agent, message_bus, _db, terminal_id, pty_session_id) =
            setup_stalled_recovery_fixture().await;
        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;
        let mut tracker = StallRecoveryTracker::default();

        agent.recover_stalled_terminals(&mut tracker).await.unwrap();

        let message =
            tokio::time::timeout(std::time::Duration::from_millis(500), terminal_rx.recv())
                .await
                .expect("stalled terminal should receive a watchdog re-dispatch")
                .expect("message should exist");

        match message {
            BusMessage::TerminalMessage { message } => {
                assert!(message.contains("Watchdog notice"));
            }
            other => panic!("Expected TerminalMessage, got {other:?}"),
        }

        assert!(tracker.last_recoveries.contains_key(&terminal_id));
    }

    #[tokio::test]
    async fn recover_stalled_terminals_claude_sends_submit_keystroke_once() {
        let (agent, message_bus, _db, terminal_id, pty_session_id) =
            setup_stalled_recovery_fixture().await;
        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;
        let mut tracker = StallRecoveryTracker::default();

        agent.recover_stalled_terminals(&mut tracker).await.unwrap();

        let first = tokio::time::timeout(std::time::Duration::from_millis(500), terminal_rx.recv())
            .await
            .expect("stalled terminal should receive redispatch message")
            .expect("message should exist");
        match first {
            BusMessage::TerminalMessage { message } => {
                assert!(message.contains("Watchdog notice"));
            }
            other => panic!("Expected TerminalMessage, got {other:?}"),
        }

        let second = tokio::time::timeout(std::time::Duration::from_millis(1200), terminal_rx.recv())
            .await
            .expect("Claude stall recovery should emit a submit keystroke")
            .expect("message should exist");
        match second {
            BusMessage::TerminalInput {
                terminal_id: input_terminal_id,
                session_id,
                input,
                ..
            } => {
                assert_eq!(input_terminal_id, terminal_id);
                assert_eq!(session_id, pty_session_id);
                assert!(
                    input.is_empty(),
                    "stall recovery submit keystroke should use empty input payload"
                );
            }
            other => panic!("Expected TerminalInput, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn recover_stalled_terminals_respects_recovery_cooldown() {
        let (agent, message_bus, _db, _terminal_id, pty_session_id) =
            setup_stalled_recovery_fixture().await;
        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;
        let mut tracker = StallRecoveryTracker::default();

        agent.recover_stalled_terminals(&mut tracker).await.unwrap();
        agent.recover_stalled_terminals(&mut tracker).await.unwrap();

        let wait_window = std::time::Duration::from_millis(700);
        let poll_slice = std::time::Duration::from_millis(80);
        let deadline = tokio::time::Instant::now() + wait_window;
        let mut redispatch_count = 0usize;

        while tokio::time::Instant::now() < deadline {
            let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
            let timeout = remaining.min(poll_slice);
            match tokio::time::timeout(timeout, terminal_rx.recv()).await {
                Ok(Some(BusMessage::TerminalMessage { message })) => {
                    if message.contains("Watchdog notice") {
                        redispatch_count += 1;
                    }
                }
                Ok(Some(_)) => {}
                Ok(None) => break,
                Err(_) => {}
            }
        }

        assert_eq!(
            redispatch_count, 1,
            "cooldown should suppress duplicate immediate re-dispatches"
        );
    }

    #[tokio::test]
    async fn recover_stalled_terminals_clears_marker_after_terminal_state_change() {
        let (agent, _message_bus, db, terminal_id, _pty_session_id) =
            setup_stalled_recovery_fixture().await;
        let mut tracker = StallRecoveryTracker::default();

        agent.recover_stalled_terminals(&mut tracker).await.unwrap();
        assert!(tracker.last_recoveries.contains_key(&terminal_id));

        let now = Utc::now();
        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'completed', completed_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(now)
        .bind(&terminal_id)
        .execute(&db.pool)
        .await
        .unwrap();

        agent.recover_stalled_terminals(&mut tracker).await.unwrap();
        assert!(
            !tracker.last_recoveries.contains_key(&terminal_id),
            "non-working terminals should be removed from cooldown tracker"
        );
    }
}
