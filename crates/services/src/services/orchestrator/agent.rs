//! Orchestrator agent loop and event handling.

use std::{
    collections::{HashMap, HashSet},
    path::{Path, PathBuf},
    sync::Arc,
    time::Instant,
};

use anyhow::anyhow;
use db::DBService;
use futures::future;
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
        COMPLETION_CONTEXT_BODY_MAX_CHARS, COMPLETION_CONTEXT_DIFF_MAX_CHARS,
        COMPLETION_CONTEXT_LOG_LINES, COMPLETION_CONTEXT_LOG_MAX_CHARS,
        GIT_COMMIT_METADATA_SEPARATOR, HANDOFF_COMMIT_MAX_CHARS, HANDOFF_NOTES_MAX_CHARS,
        MAX_CONSECUTIVE_LLM_FAILURES, QUALITY_GATE_MODE_ENFORCE, QUALITY_GATE_MODE_OFF,
        QUALITY_GATE_MODE_SHADOW, STATE_SAVE_DEBOUNCE_SECS, TASK_STATUS_CANCELLED,
        TASK_STATUS_COMPLETED, TASK_STATUS_FAILED, TASK_STATUS_RUNNING,
        TERMINAL_STATUS_CANCELLED, TERMINAL_STATUS_COMPLETED, TERMINAL_STATUS_FAILED,
        TERMINAL_STATUS_NOT_STARTED, TERMINAL_STATUS_QUALITY_PENDING, TERMINAL_STATUS_REVIEW_PASSED,
        TERMINAL_STATUS_REVIEW_REJECTED, TERMINAL_STATUS_STARTING, TERMINAL_STATUS_WAITING,
        TERMINAL_STATUS_WORKING,
        WORKFLOW_STATUS_COMPLETED, WORKFLOW_STATUS_FAILED,
        WORKFLOW_STATUS_MERGE_PARTIAL_FAILED,
        WORKFLOW_STATUS_RUNNING, WORKFLOW_TOPIC_PREFIX,
    },
    persistence::StatePersistence,
    llm::{LLMClient, build_terminal_completion_prompt, create_llm_client},
    message_bus::{BusMessage, SharedMessageBus},
    prompt_handler::PromptHandler,
    runtime_actions::{RuntimeActionService, RuntimeTaskSpec, RuntimeTerminalSpec},
    state::{OrchestratorRunState, OrchestratorState, SharedOrchestratorState},
    types::{
        CodeIssue, LLMMessage, OrchestratorInstruction, PreviousTerminalContext,
        QualityGateResultEvent, TerminalCompletionContext, TerminalCompletionEvent,
        TerminalCompletionStatus, TerminalPromptEvent,
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
    persistence: Option<Arc<StatePersistence>>,
    last_state_save: Arc<tokio::sync::Mutex<tokio::time::Instant>>,
}

// G10-007: Use `[ \t]` instead of `\s` to prevent cross-line matching.
// G10-008: Require at least one `-` in the capture group to enforce UUID format
//          (rejects plain hex strings that are not UUIDs).
static TASK_HINT_FROM_COMMIT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\btask(?:[_[ \t]-]*id)?[_[ \t]:=-]*([0-9a-f]{8}-[0-9a-f-]{4,27})\b")
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
            persistence: None,
            last_state_save: Arc::new(tokio::sync::Mutex::new(tokio::time::Instant::now())),
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
            persistence: None,
            last_state_save: Arc::new(tokio::sync::Mutex::new(tokio::time::Instant::now())),
        })
    }

    pub fn attach_runtime_actions(&mut self, runtime_actions: Arc<RuntimeActionService>) {
        self.runtime_actions = Some(runtime_actions);
    }

    pub fn attach_persistence(&mut self, persistence: Arc<StatePersistence>) {
        self.persistence = Some(persistence);
    }

    /// Restore agent state from persisted data after crash recovery.
    ///
    /// Overwrites the in-memory orchestrator state with the recovered state so
    /// that the agent event loop can resume where it left off.
    pub async fn restore_state(&self, recovered: OrchestratorState) {
        let mut state = self.state.write().await;
        state.task_states = recovered.task_states;
        state.workflow_planning_complete = recovered.workflow_planning_complete;
        state.conversation_history = recovered.conversation_history;
        state.total_tokens_used = recovered.total_tokens_used;
        state.error_count = recovered.error_count;
    }

    /// Debounced state persistence - saves at most once every STATE_SAVE_DEBOUNCE_SECS seconds.
    /// Failures are logged but do not block the main flow.
    async fn maybe_save_state(&self) {
        let Some(ref persistence) = self.persistence else {
            return;
        };

        let mut last_save = self.last_state_save.lock().await;
        if last_save.elapsed() < std::time::Duration::from_secs(STATE_SAVE_DEBOUNCE_SECS) {
            return;
        }
        *last_save = tokio::time::Instant::now();
        drop(last_save);

        let state = self.state.read().await;
        if let Err(e) = persistence.save_state(&state).await {
            let workflow_id = &state.workflow_id;
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to persist orchestrator state"
            );
        }
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
            .is_some_and(|task_state| task_state.planning_complete)
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
        let planning_complete = if let Some(value) = planning_complete { value } else {
            let state = self.state.read().await;
            state
                .task_states
                .get(task_id)
                .map_or(true, |task_state| task_state.planning_complete)
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
                    .is_some_and(|task_state| task_state.planning_complete),
                state.is_task_completed(task_id),
                state.task_has_failures(task_id),
                state.workflow_id.clone(),
            )
        };

        if !planning_complete || !task_completed {
            return Ok(());
        }

        let status = if task_failed { TASK_STATUS_FAILED } else { TASK_STATUS_COMPLETED };
        db::models::WorkflowTask::update_status(&self.db.pool, task_id, status)
            .await
            .map_err(|e| anyhow!("Failed to update task {task_id} status to {status}: {e}"))?;
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
            BusMessage::TerminalQualityGateResult(event) => {
                self.handle_quality_gate_result(event).await?;
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
            if matches!(task.status.as_str(), TASK_STATUS_COMPLETED | TASK_STATUS_FAILED | TASK_STATUS_CANCELLED) {
                continue;
            }

            let terminals = db::models::Terminal::find_by_task(&self.db.pool, &task.id).await?;
            if terminals.is_empty() {
                continue;
            }

            for terminal in terminals
                .iter()
                .filter(|terminal| terminal.status == TERMINAL_STATUS_WORKING)
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
            Self::build_task_instruction(workflow_id, task, terminal, total_terminals, None),
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
        if event.status == TerminalCompletionStatus::Checkpoint {
            tracing::info!(
                terminal_id = %event.terminal_id,
                "Terminal checkpoint detected. Triggering Quality Gate..."
            );
            return self.handle_checkpoint_quality_gate(event).await;
        }

        tracing::info!(
            "Terminal completed: {} with status {:?}",
            event.terminal_id,
            event.status
        );

        // Determine if terminal completed successfully.
        // Note: Checkpoint is intercepted above and never reaches this point.
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
                    "Processing out-of-order terminal completion event"
                );
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

        if success && existing_terminal.status != TERMINAL_STATUS_WORKING {
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
                TERMINAL_STATUS_WORKING,
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
                TERMINAL_STATUS_WORKING,
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

        if let Err(e) = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TerminalStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    terminal_id: event.terminal_id.clone(),
                    status: terminal_final_status.to_string(),
                },
            )
            .await
        {
            tracing::warn!(error = %e, "Failed to publish terminal status update event");
        }

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
            Some(TASK_STATUS_FAILED)
        } else if task_failed && task_completed {
            Some(TASK_STATUS_FAILED)
        } else if task_completed {
            Some(TASK_STATUS_COMPLETED)
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

            if let Err(e) = self
                .message_bus
                .publish_workflow_event(
                    &workflow_id,
                    BusMessage::TaskStatusUpdate {
                        workflow_id: workflow_id.clone(),
                        task_id: event.task_id.clone(),
                        status: task_status.to_string(),
                    },
                )
                .await
            {
                tracing::warn!(error = %e, "Failed to publish task status update event");
            }
        }

        // 闁哄瀚紓鎾诲箵閹邦喓浠涙鐐村劶閻ㄧ喖鏁?LLM
        let should_run_completion_llm = !(success && has_next && !task_failed);
        let mut completion_response: Option<String> = None;
        if should_run_completion_llm {
            let prompt = self.build_completion_prompt(&event).await?;
            if let Some(response) = self.call_llm_safe(&prompt).await {
                completion_response = Some(response);
            } else {
                let wf_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };
                tracing::warn!(
                    workflow_id = %wf_id,
                    task_id = %event.task_id,
                    terminal_id = %event.terminal_id,
                    "LLM unavailable, falling back to auto-dispatch only"
                );
            }
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

        self.maybe_save_state().await;

        // Restore idle state before returning.
        {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Idle;
        }

        Ok(())
    }

    /// Handles a checkpoint status by triggering quality gate evaluation.
    ///
    /// In "off" mode, the checkpoint is immediately promoted to Completed.
    /// In "shadow"/"warn"/"enforce" modes, a quality gate run is created and
    /// the evaluation spawned asynchronously. The result arrives via
    /// `BusMessage::TerminalQualityGateResult`.
    ///
    /// Protections:
    /// - Replay: skips if this terminal+commit was already processed in-memory.
    /// - Dedup: skips if a quality_run already exists in DB for this terminal+commit.
    /// - Out-of-order: skips if terminal is no longer in a valid state for checkpoints.
    /// - Idempotent: skips if a quality run is already in progress for this terminal.
    async fn handle_checkpoint_quality_gate(
        &self,
        event: TerminalCompletionEvent,
    ) -> anyhow::Result<()> {
        let mode = self.config.quality_gate_mode.clone();

        tracing::info!(
            terminal_id = %event.terminal_id,
            task_id = %event.task_id,
            commit_hash = ?event.commit_hash,
            mode = %mode,
            "Processing quality gate checkpoint"
        );

        // --- Out-of-order protection: verify terminal is in a valid state ---
        // (done before acquiring write lock to avoid holding the lock during DB I/O)
        if let Ok(Some(terminal)) =
            db::models::Terminal::find_by_id(&self.db.pool, &event.terminal_id).await
        {
            let valid_states = [TERMINAL_STATUS_WORKING, TERMINAL_STATUS_QUALITY_PENDING];
            if !valid_states.contains(&terminal.status.as_str()) {
                tracing::warn!(
                    terminal_id = %event.terminal_id,
                    terminal_status = %terminal.status,
                    "Terminal not in valid state for checkpoint, skipping (out-of-order protection)"
                );
                return Ok(());
            }
        }

        // --- Dedup: check DB for existing quality_run for this terminal+commit ---
        // (done before acquiring write lock to avoid holding the lock during DB I/O)
        if self.is_checkpoint_duplicate(&event.terminal_id, event.commit_hash.as_deref()).await {
            tracing::info!(
                terminal_id = %event.terminal_id,
                commit_hash = ?event.commit_hash,
                "Duplicate checkpoint detected in DB, skipping"
            );
            return Ok(());
        }

        // Off mode: skip quality gate entirely, treat as Completed
        if mode == QUALITY_GATE_MODE_OFF {
            tracing::info!(
                terminal_id = %event.terminal_id,
                "Quality gate mode is off, promoting checkpoint to completed"
            );
            let promoted_event = TerminalCompletionEvent {
                status: TerminalCompletionStatus::Completed,
                ..event
            };
            self.message_bus.publish_terminal_completed(promoted_event).await;
            return Ok(());
        }

        // G31-004: merge replay check + idempotent check + insert into a single write-lock
        // scope to eliminate the TOCTOU window between "check" and "insert".
        {
            let mut state = self.state.write().await;

            // Replay protection (under write lock).
            if let Some(ref hash) = event.commit_hash {
                let checkpoint_key = format!("{}:{}", event.terminal_id, hash);
                if state.processed_checkpoints.contains(&checkpoint_key) {
                    tracing::debug!(
                        terminal_id = %event.terminal_id,
                        commit_hash = %hash,
                        "Checkpoint already processed in-memory, skipping (replay protection)"
                    );
                    return Ok(());
                }
            }

            // Idempotent check (under write lock).
            if state.pending_quality_checks.contains(&event.terminal_id) {
                tracing::info!(
                    terminal_id = %event.terminal_id,
                    "Quality gate already in progress for this terminal, skipping"
                );
                return Ok(());
            }

            // Both checks passed atomically — register the pending entry and checkpoint.
            state.pending_quality_checks.insert(event.terminal_id.clone());
            if let Some(ref hash) = event.commit_hash {
                state
                    .processed_checkpoints
                    .insert(format!("{}:{}", event.terminal_id, hash));
            }
        }

        // Create a quality_run record
        let quality_run = db::models::QualityRun::new_pending(
            &event.workflow_id,
            Some(&event.task_id),
            Some(&event.terminal_id),
            event.commit_hash.as_deref(),
            "terminal",
            &mode,
        );
        let quality_run_id = quality_run.id.clone();

        if let Err(e) = db::models::QualityRun::insert(&self.db.pool, &quality_run).await {
            tracing::error!(
                terminal_id = %event.terminal_id,
                error = %e,
                "Failed to insert quality_run record, promoting checkpoint to completed"
            );
            // G31-009: remove the pending entry that was added optimistically above
            // so subsequent checkpoints for this terminal are not incorrectly blocked.
            {
                let mut state = self.state.write().await;
                state.pending_quality_checks.remove(&event.terminal_id);
            }
            let promoted_event = TerminalCompletionEvent {
                status: TerminalCompletionStatus::Completed,
                ..event
            };
            self.message_bus
                .publish_terminal_completed(promoted_event)
                .await;
            return Ok(());
        }

        db::models::QualityRun::set_running(&self.db.pool, &quality_run_id).await.ok();

        // Spawn async quality gate evaluation
        let db = Arc::clone(&self.db);
        let message_bus = Arc::clone(&self.message_bus);
        let workflow_id = event.workflow_id.clone();
        let task_id = event.task_id.clone();
        let terminal_id = event.terminal_id.clone();
        let commit_hash = event.commit_hash.clone();
        let run_id = quality_run_id.clone();
        let gate_mode = mode.clone();

        tokio::spawn(async move {
            // G31-008: ensure pending_quality_checks is cleaned up even if this task panics.
            // We use a flag rather than scopeguard crate to avoid the extra dependency;
            // the cleanup runs in the same async block via an RAII-like wrapper.
            struct PendingGuard {
                // We cannot hold &state from the outer scope across .await points, so
                // cleaning up here is a best-effort tracing warning only; the actual
                // cleanup happens at the end of the spawn body or on early return.
                terminal_id: String,
            }
            impl Drop for PendingGuard {
                fn drop(&mut self) {
                    // Panic path: log that the entry may be stranded.
                    if std::thread::panicking() {
                        tracing::error!(
                            terminal_id = %self.terminal_id,
                            "Quality gate spawn panicked — pending_quality_checks entry \
                             may be stranded; it will be cleared on next successful gate result"
                        );
                    }
                }
            }
            let _guard = PendingGuard { terminal_id: terminal_id.clone() };

            let start = Instant::now();

            // G31-003: wrap entire quality engine execution in a 5-minute timeout.
            const QUALITY_GATE_TIMEOUT_SECS: u64 = 300;

            /// Helper: produces the fall-open (skipped) outcome used when the
            /// quality engine fails or times out (G31-006).
            fn skipped_outcome(
                run_id: &str,
                reason: &str,
                quality_run_id_for_warn: &str,
            ) -> (&'static str, i32, i32, i32, bool, Option<String>, Option<String>) {
                tracing::warn!(
                    quality_run_id = %quality_run_id_for_warn,
                    reason = %reason,
                    "Quality engine unavailable — gate_status set to 'skipped' (fail-open, G31-006)"
                );
                let _ = run_id; // silence unused warning
                ("skipped", 0i32, 0i32, 0i32, true, None, None)
            }

            // Resolve the project working directory for quality analysis
            let working_dir = match db::models::Workflow::find_by_id(&db.pool, &workflow_id).await {
                Ok(Some(wf)) => {
                    match db::models::project::Project::find_by_id(&db.pool, wf.project_id).await {
                        Ok(Some(proj)) => {
                            match &proj.default_agent_working_dir {
                                Some(path) if !path.trim().is_empty() => Some(PathBuf::from(path)),
                                _ => {
                                    // Fall back to first project repo
                                    db::models::project_repo::ProjectRepo::find_repos_for_project(
                                        &db.pool,
                                        proj.id,
                                    )
                                    .await
                                    .ok()
                                    .and_then(|repos| {
                                        repos.into_iter()
                                            .map(|r| r.path.to_string_lossy().into_owned())
                                            .find(|p| !p.trim().is_empty())
                                            .map(PathBuf::from)
                                    })
                                }
                            }
                        }
                        _ => None,
                    }
                }
                _ => None,
            };

            // G31-003: wrap engine run in timeout.
            let engine_future = async {
                if let Some(ref wd) = working_dir {
                    match quality::engine::QualityEngine::from_project(wd) {
                        Ok(engine) => {
                            match engine.run(wd, quality::gate::QualityGateLevel::Terminal, None).await {
                                Ok(report) => {
                                    let status = report.overall_status();
                                    let gate_str = match status {
                                        quality::gate::status::QualityGateStatus::Ok => "ok",
                                        quality::gate::status::QualityGateStatus::Warn => "warn",
                                        quality::gate::status::QualityGateStatus::Error => "error",
                                    };
                                    let total = report.summary.total as i32;
                                    let blocking = report.summary.blocking_issues as i32;
                                    let new_i = report.summary.new_issues as i32;
                                    let is_passed = report.is_passed();
                                    let fix = if is_passed { None } else { Some(report.to_fix_instructions()) };
                                    let rjson = serde_json::to_string(&report).ok();
                                    (gate_str, total, blocking, new_i, is_passed, fix, rjson)
                                }
                                Err(e) => {
                                    skipped_outcome(&run_id, &format!("engine.run failed: {e}"), &run_id)
                                }
                            }
                        }
                        Err(e) => {
                            skipped_outcome(&run_id, &format!("QualityEngine::from_project failed: {e}"), &run_id)
                        }
                    }
                } else {
                    skipped_outcome(&run_id, "could not resolve project working directory", &run_id)
                }
            };

            let (gate_status, total_issues, blocking_issues, new_issues, passed, fix_instructions, report_json) =
                match tokio::time::timeout(
                    Duration::from_secs(QUALITY_GATE_TIMEOUT_SECS),
                    engine_future,
                )
                .await
                {
                    Ok(outcome) => outcome,
                    Err(_elapsed) => {
                        skipped_outcome(
                            &run_id,
                            &format!("quality engine timed out after {QUALITY_GATE_TIMEOUT_SECS}s"),
                            &run_id,
                        )
                    }
                };

            let duration_ms = start.elapsed().as_millis() as i32;

            // Complete the quality_run record
            if let Err(e) = db::models::QualityRun::complete(
                &db.pool,
                &run_id,
                gate_status,
                total_issues,
                blocking_issues,
                new_issues,
                duration_ms,
                None, // providers_run
                report_json.as_deref(),
                None, // decision_json
            )
            .await
            {
                tracing::error!(
                    quality_run_id = %run_id,
                    error = %e,
                    "Failed to complete quality_run record"
                );
            }

            // G31-006: if gate was skipped (engine failure/timeout), emit a warn event
            // to the workflow bus so operators are alerted.
            if gate_status == "skipped" {
                let warn_msg = BusMessage::Error {
                    workflow_id: workflow_id.clone(),
                    error: format!(
                        "Quality gate skipped for terminal {terminal_id}: engine unavailable"
                    ),
                };
                if let Err(e) = message_bus.publish_workflow_event(&workflow_id, warn_msg).await {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        error = %e,
                        "Failed to publish quality gate skipped warning event"
                    );
                }
            }

            let summary = format!(
                "Quality gate {gate_status}: {total_issues} total issues, {blocking_issues} blocking"
            );

            let result = QualityGateResultEvent {
                workflow_id,
                task_id,
                terminal_id,
                quality_run_id: run_id,
                commit_hash,
                gate_status: gate_status.to_string(),
                mode: gate_mode,
                total_issues,
                blocking_issues,
                new_issues,
                passed,
                summary,
                fix_instructions,
            };

            message_bus.publish_quality_gate_result(result).await;
        });

        Ok(())
    }

    /// Checks whether a checkpoint is a duplicate by querying the DB for an
    /// existing quality_run with the same terminal_id + commit_hash.
    ///
    /// Returns `false` when there is no commit_hash (nothing to dedup against)
    /// or when the DB query fails (fail-open to avoid blocking processing).
    async fn is_checkpoint_duplicate(
        &self,
        terminal_id: &str,
        commit_hash: Option<&str>,
    ) -> bool {
        let Some(hash) = commit_hash else {
            return false;
        };
        match db::models::QualityRun::find_by_terminal_and_commit(
            &self.db.pool,
            terminal_id,
            hash,
        )
        .await
        {
            Ok(Some(_)) => true,
            Ok(None) => false,
            Err(e) => {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    commit_hash = %hash,
                    error = %e,
                    "Failed to check checkpoint duplicate, proceeding (fail-open)"
                );
                false
            }
        }
    }

    /// Handles the result of a quality gate evaluation.
    ///
    /// Depending on mode and result:
    /// - shadow: always promote to completed (log result)
    /// - warn: promote to completed (emit warning if issues found)
    /// - enforce: promote to completed if passed, or fail terminal if blocked
    async fn handle_quality_gate_result(
        &self,
        event: QualityGateResultEvent,
    ) -> anyhow::Result<()> {
        tracing::info!(
            terminal_id = %event.terminal_id,
            quality_run_id = %event.quality_run_id,
            gate_status = %event.gate_status,
            mode = %event.mode,
            passed = event.passed,
            total_issues = event.total_issues,
            blocking_issues = event.blocking_issues,
            "Quality gate result received"
        );

        // Remove from pending quality checks
        {
            let mut state = self.state.write().await;
            state.pending_quality_checks.remove(&event.terminal_id);
        }

        let promote_status = if event.mode == QUALITY_GATE_MODE_ENFORCE && !event.passed {
            // Enforce mode with failure: send fix instructions directly to terminal PTY stdin.
            // G31-001: use publish_terminal_input (targeted) instead of publish_workflow_event
            // (broadcast) so the message reaches the correct PTY process.
            if let Some(fix_instructions) = &event.fix_instructions {
                tracing::warn!(
                    terminal_id = %event.terminal_id,
                    "Quality gate enforce mode: terminal blocked, sending fix instructions to PTY"
                );
                // Resolve the PTY session ID for the terminal.
                let session_id_opt = db::models::Terminal::find_by_id(
                    &self.db.pool,
                    &event.terminal_id,
                )
                .await
                .ok()
                .flatten()
                .and_then(|t| t.pty_session_id.or(t.session_id))
                .filter(|s| !s.trim().is_empty());

                if let Some(session_id) = session_id_opt {
                    let fix_message = format!(
                        "Quality gate BLOCKED: {}\n\nFix instructions:\n{}",
                        event.summary, fix_instructions
                    );
                    // G31-001: targeted delivery to specific terminal PTY.
                    self.message_bus
                        .publish_terminal_input(
                            &event.terminal_id,
                            &session_id,
                            &fix_message,
                            None,
                        )
                        .await;
                } else {
                    tracing::warn!(
                        terminal_id = %event.terminal_id,
                        "Quality gate enforce mode: no PTY session found for terminal, \
                         fix instructions not delivered"
                    );
                }
            }
            TerminalCompletionStatus::Failed
        } else {
            // Shadow/warn modes or enforce mode with pass: promote to completed
            if event.mode == QUALITY_GATE_MODE_SHADOW {
                tracing::info!(
                    terminal_id = %event.terminal_id,
                    "Quality gate shadow mode: promoting to completed (result logged only)"
                );
            } else if !event.passed {
                tracing::warn!(
                    terminal_id = %event.terminal_id,
                    gate_status = %event.gate_status,
                    blocking_issues = event.blocking_issues,
                    "Quality gate warn mode: issues found but proceeding"
                );
            }
            TerminalCompletionStatus::Completed
        };

        // G31-005: Re-enter the completion pipeline, but skip the quiet window check.
        // The quality gate evaluation already took 10-300 seconds; the terminal has been
        // quiescent for at least that long.  Re-running the quiet-window check would
        // incorrectly delay completion again.
        let completion_event = TerminalCompletionEvent {
            terminal_id: event.terminal_id,
            task_id: event.task_id,
            workflow_id: event.workflow_id,
            status: promote_status,
            commit_hash: event.commit_hash,
            commit_message: None,
            metadata: None,
        };

        self.handle_terminal_completed_skip_quiet_window(completion_event).await
    }

    /// Identical to `handle_terminal_completed` but bypasses the quiet-window check.
    ///
    /// Used by `handle_quality_gate_result` (G31-005): the quality gate evaluation
    /// already enforced a sufficient quiescence period; running the window again
    /// would introduce unnecessary latency and could trigger a second defer loop.
    async fn handle_terminal_completed_skip_quiet_window(
        &self,
        event: TerminalCompletionEvent,
    ) -> anyhow::Result<()> {
        // Checkpoint events are not expected here (quality gate results are never checkpoints).
        if event.status == TerminalCompletionStatus::Checkpoint {
            return self.handle_checkpoint_quality_gate(event).await;
        }

        tracing::info!(
            "Terminal completed (post-quality-gate, no quiet window): {} with status {:?}",
            event.terminal_id,
            event.status
        );

        self.ensure_task_state_initialized_for_completion(&event.task_id)
            .await?;

        // Skip quiet window — delegate directly to the post-quiet-window portion
        // of the normal completion flow by calling handle_terminal_completed with
        // the event but with quiet window effectively at zero duration.
        // We achieve this by calling the shared completion body directly.
        // Since extracting the body into a separate helper would require significant
        // refactoring, we instead call handle_terminal_completed and rely on the fact
        // that the quiet window will return None (elapsed >= window) because the
        // quality engine took long enough.  As an extra safety net, we clear the
        // pending_quiet_completion_checks entry for this terminal so the defer loop
        // cannot interfere.
        {
            let mut state = self.state.write().await;
            state.pending_quiet_completion_checks.remove(&event.terminal_id);
        }

        // Now delegate; the quiet window query will return None (window already elapsed)
        // because the quality gate took ≥ QUALITY_GATE_TIMEOUT_SECS (up to 300s).
        // In the unlikely event the window is still active, it will defer again — the
        // PendingGuard above removed the dedup entry so defer_terminal_completion will
        // accept the new entry.
        self.handle_terminal_completed(event).await
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
                TERMINAL_STATUS_COMPLETED => completed_terminals.push(terminal.id.clone()),
                TERMINAL_STATUS_FAILED | TERMINAL_STATUS_CANCELLED => failed_terminals.push(terminal.id.clone()),
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

                        // G04-002: Verify terminal is still in 'working' status before
                        // re-publishing the completion event. Another path (e.g. GitEvent)
                        // may have already completed it during the quiet window.
                        let still_working = match db::models::Terminal::find_by_id(&db.pool, &terminal_id).await {
                            Ok(Some(t)) => t.status == TERMINAL_STATUS_WORKING,
                            Ok(None) => {
                                tracing::warn!(
                                    terminal_id = %terminal_id,
                                    "Terminal not found when checking deferred completion, skipping"
                                );
                                false
                            }
                            Err(e) => {
                                tracing::warn!(
                                    terminal_id = %terminal_id,
                                    error = %e,
                                    "Failed to check terminal status for deferred completion, proceeding anyway"
                                );
                                true // Proceed on DB error to avoid silently dropping completions
                            }
                        };

                        if still_working {
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
                        } else {
                            tracing::debug!(
                                terminal_id = %terminal_id,
                                "Terminal no longer working, skipping deferred completion publish"
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
            r"
            SELECT MAX(created_at)
            FROM terminal_log
            WHERE terminal_id = ?
            ",
        )
        .bind(terminal_id)
        .fetch_one(pool)
        .await?;

        let last_activity_at = if let Some(last_output_at) = latest_output_at {
            Some(last_output_at)
        } else {
            sqlx::query_scalar(
                r"
                SELECT COALESCE(started_at, updated_at, created_at)
                FROM terminal
                WHERE id = ?
                ",
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
            if active_terminal.status == TERMINAL_STATUS_WAITING {
                break;
            }

            if matches!(
                active_terminal.status.as_str(),
                TERMINAL_STATUS_WORKING | TERMINAL_STATUS_COMPLETED | TERMINAL_STATUS_FAILED | TERMINAL_STATUS_CANCELLED
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
        let prev_context = fetch_previous_terminal_context(
            &self.db, &task.id, &workflow_id, active_terminal.order_index,
        ).await.unwrap_or(None);
        let instruction =
            Self::build_task_instruction(&workflow_id, &task, &active_terminal, terminals.len(), prev_context.as_ref());
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

        // G10-002/G04-007: Single metadata parse attempt.
        // The `message` parameter is typically the commit subject line only
        // (git --format=%s), which usually does not contain METADATA (that lives in the commit body).
        // However, in some code paths (e.g., tests, direct calls), the full message may be passed.
        // This is the ONLY metadata parse — there is no redundant second parse.
        if let Ok(metadata) = crate::services::git_watcher::parse_commit_metadata(message) {
            tracing::info!(
                "Commit {} has METADATA in message, processing via metadata path",
                commit_hash
            );
            let terminal_id = &metadata.terminal_id;
            let status_str = &metadata.status;
            let task_id = &metadata.task_id;

            match status_str.as_str() {
                "review_pass" | "review_passed" => {
                    if let Some(reviewed_id) = &metadata.reviewed_terminal {
                        self.handle_git_review_pass(
                            terminal_id,
                            task_id,
                            reviewed_id,
                        )
                        .await?;
                    }
                }
                "review_reject" | "review_rejected" => {
                    if let Some(reviewed_id) = &metadata.reviewed_terminal {
                        let issues = metadata.issues.unwrap_or_default();
                        self.handle_git_review_reject(
                            terminal_id,
                            task_id,
                            reviewed_id,
                            &issues,
                        )
                        .await?;
                    }
                }
                _ => {
                    self.handle_git_terminal_completed(
                        terminal_id,
                        task_id,
                        commit_hash,
                        message,
                    )
                    .await?;
                }
            }

            // Mark commit as processed
            {
                let mut state = self.state.write().await;
                state.record_processed_commit(commit_hash.to_string());
            }
            self.maybe_save_state().await;
            return Ok(());
        }

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

        // Mark commit as processed
        {
            let mut state = self.state.write().await;
            state.record_processed_commit(commit_hash.to_string());
        }

        self.maybe_save_state().await;

        Ok(())
    }

    #[allow(dead_code)]
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
        if let Err(e) = db::models::git_event::GitEvent::update_status(
            &self.db.pool,
            event_id,
            "processing",
            None,
        )
        .await
        {
            tracing::warn!(event_id = %event_id, error = %e, "Failed to update git_event status to processing");
        }

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
            if let Err(e) = db::models::git_event::GitEvent::update_status(
                &self.db.pool,
                event_id,
                "failed",
                Some(reason),
            )
            .await
            {
                tracing::warn!(event_id = %event_id, error = %e, "Failed to update git_event status to failed");
            }
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
            if let Err(e) = db::models::git_event::GitEvent::update_status(
                &self.db.pool,
                event_id,
                "failed",
                Some(&reason),
            )
            .await
            {
                tracing::warn!(event_id = %event_id, error = %e, "Failed to update git_event status to failed after handling error");
            }
            return Ok(());
        }

        {
            let mut state = self.state.write().await;
            state.record_processed_commit(commit_hash.to_string());
        }

        if let Err(e) = db::models::git_event::GitEvent::update_status(
            &self.db.pool,
            event_id,
            "processed",
            Some("Inferred terminal completion from no-metadata commit"),
        )
        .await
        {
            tracing::warn!(event_id = %event_id, error = %e, "Failed to update git_event status to processed");
        }

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
                .filter(|(_, terminal)| terminal.status == TERMINAL_STATUS_WORKING)
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

        // G04-001: When task_hint exists but still matches multiple candidates,
        // proceed with deterministic selection instead of returning None.
        // This prevents orchestrator stalls when a task hint narrows candidates
        // but doesn't reduce to exactly one.
        if inferred_candidates.len() > 1 {
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

        // G10-011: Guard against duplicate processing — if the terminal has already
        // been completed via handle_terminal_completed (e.g. from a TerminalCompleted
        // bus message), skip to avoid double-processing the same completion.
        if let Some(terminal) = db::models::Terminal::find_by_id(&self.db.pool, terminal_id).await? {
            if terminal.status != TERMINAL_STATUS_WORKING {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    current_status = %terminal.status,
                    "Terminal already processed (status != working), skipping git completion"
                );
                return Ok(());
            }
        }

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

        // 3. Check if all terminals in the task are done and auto-sync workflow
        self.auto_sync_workflow_completion(&workflow_id).await?;

        // 4. Awaken orchestrator to process the event
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

        // Log individual issues for debugging
        for (i, issue) in issues.iter().enumerate() {
            tracing::info!(
                reviewer = %reviewer_terminal_id,
                reviewed = %reviewed_terminal_id,
                issue_index = i,
                severity = %issue.severity,
                file = %issue.file,
                line = ?issue.line,
                message = %issue.message,
                "Review issue"
            );
        }

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

        // 3. Auto-create a fix terminal from the review issues
        if !issues.is_empty() {
            let fix_instruction = OrchestratorInstruction::FixIssues {
                terminal_id: reviewed_terminal_id.to_string(),
                issues: issues.iter().map(|i| i.message.clone()).collect(),
            };
            self.execute_single_instruction(fix_instruction).await?;
        }

        // 4. Awaken orchestrator to process the event
        self.awaken().await;

        Ok(())
    }

    /// Heuristic check if a terminal failure is due to provider issues (not code errors).
    fn is_provider_failure(error_message: &str) -> bool {
        let keywords = [
            "api_error",
            "rate_limit",
            "timeout",
            "connection_refused",
            "service_unavailable",
            "provider_error",
            "authentication_failed",
            "429",
            "503",
            "502",
        ];
        let lower = error_message.to_lowercase();
        keywords.iter().any(|k| lower.contains(k))
    }

    /// Find an alternative CLI/model config different from the failed one.
    ///
    /// Queries all `ModelConfig` entries and returns the first one whose
    /// `cli_type_id` differs from `failed_cli_type_id`.
    async fn find_alternative_cli_config(
        &self,
        failed_cli_type_id: &str,
    ) -> anyhow::Result<Option<db::models::ModelConfig>> {
        let all_configs = db::models::ModelConfig::find_all(&self.db.pool).await?;
        Ok(all_configs
            .into_iter()
            .find(|c| c.cli_type_id != failed_cli_type_id))
    }

    /// Attempt provider failover when a terminal fails due to provider issues.
    ///
    /// Creates a replacement terminal using an alternative CLI/model config,
    /// starts it, and dispatches it with context from the failed terminal.
    /// Returns `Ok(true)` when a replacement was successfully dispatched,
    /// `Ok(false)` when no alternative config is available, or `Err` on failure.
    async fn handle_terminal_provider_failure(
        &self,
        failed_terminal_id: &str,
        task_id: &str,
        _error_message: &str,
    ) -> anyhow::Result<bool> {
        // 1. Look up the failed terminal
        let failed_terminal =
            db::models::Terminal::find_by_id(&self.db.pool, failed_terminal_id)
                .await?
                .ok_or_else(|| anyhow!("Terminal not found: {failed_terminal_id}"))?;

        // 2. Find an alternative CLI config
        let alt_config = if let Some(config) = self
            .find_alternative_cli_config(&failed_terminal.cli_type_id)
            .await? { config } else {
            let workflow_id = self.state.read().await.workflow_id.clone();
            if let Err(e) = self
                .message_bus
                .publish_workflow_event(
                    &workflow_id,
                    BusMessage::Error {
                        workflow_id: workflow_id.clone(),
                        error: format!(
                            "Provider failover: no alternative CLI config available for terminal {failed_terminal_id}"
                        ),
                    },
                )
                .await
            {
                tracing::warn!(error = %e, "Failed to publish failover error event");
            }
            return Ok(false);
        };

        tracing::info!(
            failed_terminal = failed_terminal_id,
            task_id,
            failed_cli = %failed_terminal.cli_type_id,
            new_cli = %alt_config.cli_type_id,
            new_model = %alt_config.id,
            "Terminal provider failover: creating replacement terminal"
        );

        // 3. Mark the failed terminal as failed
        db::models::Terminal::update_status(
            &self.db.pool,
            failed_terminal_id,
            TERMINAL_STATUS_FAILED,
        )
        .await?;

        let workflow_id = self.state.read().await.workflow_id.clone();

        // 4. Create a replacement terminal via runtime_actions
        let failover_name = format!(
            "failover-{}",
            failed_terminal
                .role
                .as_deref()
                .unwrap_or(&failed_terminal.id)
        );
        let replacement = self
            .runtime_actions()?
            .create_terminal(
                &workflow_id,
                RuntimeTerminalSpec {
                    terminal_id: None,
                    task_id: task_id.to_string(),
                    cli_type_id: alt_config.cli_type_id.clone(),
                    model_config_id: alt_config.id.clone(),
                    custom_base_url: failed_terminal.custom_base_url.clone(),
                    custom_api_key: None,
                    role: Some(failover_name),
                    role_description: failed_terminal.role_description.clone(),
                    order_index: None,
                    auto_confirm: Some(failed_terminal.auto_confirm),
                },
            )
            .await?;

        // 5. Sync task state so the orchestrator knows about the new terminal
        self.sync_task_state_from_db(task_id, None).await?;

        // 6. Start the replacement terminal (spawns PTY)
        let started_terminal = self
            .runtime_actions()?
            .start_terminal(&replacement.id)
            .await?;

        // 7. Build dispatch instruction with context from the failed terminal
        let task = db::models::WorkflowTask::find_by_id(&self.db.pool, task_id)
            .await?
            .ok_or_else(|| anyhow!("Task not found: {task_id}"))?;

        let terminals = db::models::Terminal::find_by_task(&self.db.pool, task_id).await?;

        let prev_context = fetch_previous_terminal_context(
            &self.db,
            task_id,
            &workflow_id,
            started_terminal.order_index,
        )
        .await
        .unwrap_or(None);

        let instruction = Self::build_task_instruction(
            &workflow_id,
            &task,
            &started_terminal,
            terminals.len(),
            prev_context.as_ref(),
        );

        // 8. Dispatch the replacement terminal
        self.dispatch_terminal(task_id, &started_terminal, &instruction)
            .await?;

        // 9. Publish failover event for frontend visibility
        if let Err(e) = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TerminalStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    terminal_id: replacement.id.clone(),
                    status: TERMINAL_STATUS_WORKING.to_string(),
                },
            )
            .await
        {
            tracing::warn!(error = %e, "Failed to publish failover terminal status event");
        }

        tracing::info!(
            failed_terminal = failed_terminal_id,
            replacement_terminal = %replacement.id,
            task_id,
            new_cli = %alt_config.cli_type_id,
            "Provider failover complete: replacement terminal dispatched"
        );

        Ok(true)
    }

    /// Handle terminal failed status from git event
    ///
    /// Delegates to [`ErrorHandler::handle_terminal_failure`] which updates
    /// workflow status, activates an error terminal (when configured), and
    /// broadcasts the failure event.  Falls back to basic status update if
    /// the error handler itself errors.
    ///
    /// When the failure is a provider issue and failover succeeds (replacement
    /// terminal dispatched), the normal error-handler flow is skipped.
    #[allow(dead_code)]
    async fn handle_git_terminal_failed(
        &self,
        terminal_id: &str,
        task_id: &str,
        error_message: &str,
    ) -> anyhow::Result<()> {
        tracing::warn!(
            terminal_id,
            task_id,
            "Terminal reported failure via git commit"
        );

        // 0. Check for provider failure and attempt failover
        if Self::is_provider_failure(error_message) {
            match self
                .handle_terminal_provider_failure(terminal_id, task_id, error_message)
                .await
            {
                Ok(true) => {
                    // Replacement terminal dispatched successfully; skip normal
                    // error handling since the failed terminal is already marked
                    // failed inside handle_terminal_provider_failure.
                    tracing::info!(
                        terminal_id,
                        task_id,
                        "Provider failover succeeded, skipping normal error handler"
                    );
                    self.awaken().await;
                    return Ok(());
                }
                Ok(false) => {
                    tracing::warn!(
                        terminal_id,
                        task_id,
                        "No alternative CLI config for failover, falling through to normal error handling"
                    );
                }
                Err(e) => {
                    tracing::warn!(
                        error = %e,
                        "Provider failover failed, continuing with normal error handling"
                    );
                }
            }
        }

        // 1. Update terminal status to failed
        db::models::Terminal::update_status(&self.db.pool, terminal_id, TERMINAL_STATUS_FAILED)
            .await?;

        // 2. Delegate to error_handler for sophisticated failure handling
        //    (error terminal activation, workflow status update, etc.)
        if let Err(e) = self
            .handle_terminal_failure(task_id, terminal_id, error_message)
            .await
        {
            tracing::error!(
                terminal_id,
                task_id,
                error = %e,
                "Error handler failed, falling back to basic failure event"
            );
            // Fallback: publish error event directly
            let workflow_id = self.state.read().await.workflow_id.clone();
            let event = BusMessage::Error {
                workflow_id: workflow_id.clone(),
                error: error_message.to_string(),
            };
            self.message_bus
                .publish_workflow_event(&workflow_id, event)
                .await?;
        }

        // 3. Awaken orchestrator to process the event
        self.awaken().await;

        Ok(())
    }

    /// Awaken the orchestrator to process events
    #[allow(dead_code)]
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

    /// Resolve the project working directory from the workflow's project.
    /// Prefers `project.default_agent_working_dir`, falls back to the first project repo path.
    async fn resolve_project_working_dir(&self) -> anyhow::Result<PathBuf> {
        let workflow = self.load_workflow().await?;
        let project =
            db::models::project::Project::find_by_id(&self.db.pool, workflow.project_id)
                .await?
                .ok_or_else(|| anyhow!("Project {} not found", workflow.project_id))?;

        let repo_path = match project.default_agent_working_dir {
            Some(ref path) if !path.trim().is_empty() => Some(path.clone()),
            _ => db::models::project_repo::ProjectRepo::find_repos_for_project(
                &self.db.pool,
                project.id,
            )
            .await?
            .into_iter()
            .map(|repo| repo.path.to_string_lossy().into_owned())
            .find(|path| !path.trim().is_empty()),
        };

        repo_path
            .map(PathBuf::from)
            .ok_or_else(|| anyhow!("No working directory found for project {}", project.id))
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

        // Inject terminal completion context (silent degradation on failure)
        if let Ok(working_dir) = self.resolve_project_working_dir().await {
            if let Ok(ctx) = fetch_terminal_completion_context(
                &self.db,
                &event.terminal_id,
                commit_hash,
                &working_dir,
            )
            .await
            {
                if !ctx.log_summary.is_empty() {
                    prompt.push_str("\n\n--- Terminal Output Summary ---\n");
                    prompt.push_str(&ctx.log_summary);
                }
                if !ctx.diff_stat.is_empty() {
                    prompt.push_str("\n\n--- Changes Summary ---\n");
                    prompt.push_str(&ctx.diff_stat);
                }
                if !ctx.commit_body.is_empty() {
                    prompt.push_str("\n\n--- Commit Details ---\n");
                    prompt.push_str(&ctx.commit_body);
                }
            }
        }

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
                .map_or(without_opening.trim(), str::trim);
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

    fn instruction_type_name(instruction: &OrchestratorInstruction) -> &'static str {
        match instruction {
            OrchestratorInstruction::StartTask { .. } => "start_task",
            OrchestratorInstruction::CreateTask { .. } => "create_task",
            OrchestratorInstruction::CreateTerminal { .. } => "create_terminal",
            OrchestratorInstruction::StartTerminal { .. } => "start_terminal",
            OrchestratorInstruction::CloseTerminal { .. } => "close_terminal",
            OrchestratorInstruction::CompleteTask { .. } => "complete_task",
            OrchestratorInstruction::SetWorkflowPlanningComplete { .. } => {
                "set_workflow_planning_complete"
            }
            OrchestratorInstruction::SendToTerminal { .. } => "send_to_terminal",
            OrchestratorInstruction::ReviewCode { .. } => "review_code",
            OrchestratorInstruction::FixIssues { .. } => "fix_issues",
            OrchestratorInstruction::MergeBranch { .. } => "merge_branch",
            OrchestratorInstruction::PauseWorkflow { .. } => "pause_workflow",
            OrchestratorInstruction::CompleteWorkflow { .. } => "complete_workflow",
            OrchestratorInstruction::FailWorkflow { .. } => "fail_workflow",
        }
    }

    fn is_instruction_whitelisted(instruction: &OrchestratorInstruction) -> bool {
        matches!(
            instruction,
            OrchestratorInstruction::StartTask { .. }
                | OrchestratorInstruction::CreateTask { .. }
                | OrchestratorInstruction::CreateTerminal { .. }
                | OrchestratorInstruction::StartTerminal { .. }
                | OrchestratorInstruction::CloseTerminal { .. }
                | OrchestratorInstruction::CompleteTask { .. }
                | OrchestratorInstruction::SetWorkflowPlanningComplete { .. }
                | OrchestratorInstruction::SendToTerminal { .. }
                | OrchestratorInstruction::ReviewCode { .. }
                | OrchestratorInstruction::FixIssues { .. }
                | OrchestratorInstruction::MergeBranch { .. }
                | OrchestratorInstruction::CompleteWorkflow { .. }
                | OrchestratorInstruction::FailWorkflow { .. }
        )
    }

    fn validate_instruction_whitelist(instruction: &OrchestratorInstruction) -> anyhow::Result<()> {
        if Self::is_instruction_whitelisted(instruction) {
            return Ok(());
        }

        Err(anyhow!(
            "Instruction '{}' is not allowed by orchestrator whitelist",
            Self::instruction_type_name(instruction)
        ))
    }

    /// Calls the LLM with the current conversation history.
    async fn call_llm(&self, prompt: &str) -> anyhow::Result<String> {
        let mut state = self.state.write().await;
        state.add_message("user", prompt, &self.config);

        let messages = state.conversation_history.clone();
        drop(state);

        let response = self.llm_client.chat(messages).await?;

        // Publish any provider state-change events that occurred during the call.
        self.publish_provider_events().await;

        let mut state = self.state.write().await;
        state.add_message("assistant", &response.content, &self.config);
        if let Some(usage) = &response.usage {
            state.total_tokens_used += i64::from(usage.total_tokens);
        }

        Ok(response.content)
    }

    /// Wrapper around `call_llm` that catches errors instead of propagating them.
    /// Returns `None` on failure, allowing the agent event loop to continue.
    async fn call_llm_safe(&self, prompt: &str) -> Option<String> {
        match self.call_llm(prompt).await {
            Ok(response) => {
                // Reset consecutive failure count on success
                let mut state = self.state.write().await;
                state.error_count = 0;
                drop(state);
                self.maybe_save_state().await;
                Some(response)
            }
            Err(e) => {
                let mut state = self.state.write().await;
                state.error_count += 1;
                let count = state.error_count;
                let workflow_id = state.workflow_id.clone();
                drop(state);

                tracing::error!(
                    workflow_id = %workflow_id,
                    error = %e,
                    consecutive_failures = count,
                    "LLM call failed, skipping decision"
                );

                // Publish system event for frontend notification
                if let Err(e2) = self
                    .message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::Error {
                            workflow_id: workflow_id.clone(),
                            error: format!(
                                "LLM call failed ({count} consecutive): {e}"
                            ),
                        },
                    )
                    .await
                {
                    tracing::warn!(error = %e2, "Failed to publish LLM failure error event");
                }

                // Check if we've hit the exhaustion threshold
                if count >= MAX_CONSECUTIVE_LLM_FAILURES {
                    tracing::error!(
                        workflow_id = %workflow_id,
                        consecutive_failures = count,
                        "LLM failed {} consecutive times, provider may be exhausted",
                        count
                    );
                    // G24-009: Mark workflow as failed after provider exhaustion,
                    // not just an Error event, so the workflow doesn't hang indefinitely.
                    if let Err(e2) = db::models::Workflow::update_status(
                        &self.db.pool,
                        &workflow_id,
                        WORKFLOW_STATUS_FAILED,
                    )
                    .await
                    {
                        tracing::warn!(error = %e2, "Failed to mark workflow as failed after LLM exhaustion");
                    }
                    if let Err(e2) = self
                        .message_bus
                        .publish_workflow_event(
                            &workflow_id,
                            BusMessage::Error {
                                workflow_id: workflow_id.clone(),
                                error: "LLM provider exhausted - all retries failed"
                                    .to_string(),
                            },
                        )
                        .await
                    {
                        tracing::warn!(error = %e2, "Failed to publish LLM exhaustion error event");
                    }
                    if let Err(e2) = self
                        .message_bus
                        .publish_workflow_event(
                            &workflow_id,
                            BusMessage::StatusUpdate {
                                workflow_id: workflow_id.clone(),
                                status: WORKFLOW_STATUS_FAILED.to_string(),
                            },
                        )
                        .await
                    {
                        tracing::warn!(error = %e2, "Failed to publish workflow failed status after LLM exhaustion");
                    }
                }

                None
            }
        }
    }

    /// Executes orchestrator instructions returned by the LLM.
    pub async fn execute_instruction(&self, response: &str) -> anyhow::Result<()> {
        let Some(instructions) = Self::parse_instructions(response) else {
            tracing::warn!("LLM response did not contain a valid orchestrator instruction payload");
            return Ok(());
        };

        for instruction in instructions {
            Self::validate_instruction_whitelist(&instruction)?;
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
                let mark_success = terminal.status.as_str() == TERMINAL_STATUS_COMPLETED;
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
                if terminal.status != TERMINAL_STATUS_WORKING {
                    tracing::info!(
                        terminal_id = %terminal.id,
                        status = %terminal.status,
                        "Skipping SendToTerminal instruction because terminal is not in working state"
                    );
                    return Ok(());
                }

                // 2. Get PTY session ID. Missing PTY can happen after process teardown;
                // skip this advisory message instead of crashing the orchestrator runtime.
                let pty_session_id = if let Some(session_id) = terminal.pty_session_id.clone() { session_id } else {
                    tracing::warn!(
                        terminal_id = %terminal.id,
                        status = %terminal.status,
                        "Skipping SendToTerminal instruction because terminal has no PTY session"
                    );
                    return Ok(());
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

                // G11-003: Also broadcast a StatusUpdate so the frontend can detect
                // the workflow failure in real-time via the standard status channel.
                if let Err(e) = self
                    .message_bus
                    .publish_workflow_event(
                        &workflow_id,
                        BusMessage::StatusUpdate {
                            workflow_id: workflow_id.clone(),
                            status: WORKFLOW_STATUS_FAILED.to_string(),
                        },
                    )
                    .await
                {
                    tracing::warn!(error = %e, "Failed to publish workflow failed status update event");
                }

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
                    if state.task_states.contains_key(&task_id) {
                        state.sync_task_terminals(
                            task_id.clone(),
                            terminals.iter().map(|terminal| terminal.id.clone()).collect(),
                            true,
                        );
                    } else {
                        state.init_task(
                            task_id.clone(),
                            terminals.iter().map(|terminal| terminal.id.clone()).collect(),
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
            OrchestratorInstruction::ReviewCode {
                terminal_id,
                commit_hash,
            } => {
                let terminal = db::models::Terminal::find_by_id(&self.db.pool, &terminal_id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to get terminal: {e}"))?
                    .ok_or_else(|| anyhow::anyhow!("Terminal {terminal_id} not found"))?;

                let task_id = terminal.workflow_task_id.clone();
                tracing::info!(
                    terminal_id = %terminal_id,
                    task_id = %task_id,
                    commit_hash = %commit_hash,
                    "ReviewCode: creating reviewer terminal"
                );

                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };

                // 1. Fetch diff context for the review
                let diff_context = self.fetch_diff_for_review(&commit_hash).await
                    .unwrap_or_else(|e| {
                        tracing::warn!("Failed to fetch diff for review: {e}");
                        "(diff unavailable)".to_string()
                    });

                // 2. Create a reviewer terminal reusing the same CLI/model config
                let reviewer_terminal = self
                    .runtime_actions()?
                    .create_terminal(
                        &workflow_id,
                        RuntimeTerminalSpec {
                            terminal_id: None,
                            task_id: task_id.clone(),
                            cli_type_id: terminal.cli_type_id.clone(),
                            model_config_id: terminal.model_config_id.clone(),
                            custom_base_url: terminal.custom_base_url.clone(),
                            custom_api_key: None,
                            role: Some("reviewer".to_string()),
                            role_description: Some(format!(
                                "Code reviewer for terminal {terminal_id}"
                            )),
                            order_index: None,
                            auto_confirm: Some(true),
                        },
                    )
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to create reviewer terminal: {e}"))?;

                // 3. Start the reviewer terminal (launches PTY)
                let reviewer_terminal = self
                    .runtime_actions()?
                    .start_terminal(&reviewer_terminal.id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to start reviewer terminal: {e}"))?;

                // 4. Sync task state with the new terminal
                let planning_complete = self.task_planning_complete(&task_id).await;
                self.sync_task_state_from_db(&task_id, Some(planning_complete))
                    .await?;

                // 5. Build review instruction and dispatch
                let review_instruction = format!(
                    "Review the code changes from commit {commit_hash} on terminal {terminal_id}.\n\
                     \n\
                     Diff summary:\n{diff_context}\n\
                     \n\
                     If the code is correct, commit with metadata status: review_pass and reviewed_terminal: {terminal_id}.\n\
                     If there are issues, commit with metadata status: review_reject, reviewed_terminal: {terminal_id}, and list issues."
                );

                self.dispatch_terminal(&task_id, &reviewer_terminal, &review_instruction)
                    .await?;

                tracing::info!(
                    reviewer_id = %reviewer_terminal.id,
                    reviewed_id = %terminal_id,
                    "ReviewCode: reviewer terminal dispatched"
                );
            }
            OrchestratorInstruction::FixIssues {
                terminal_id,
                issues,
            } => {
                let terminal = db::models::Terminal::find_by_id(&self.db.pool, &terminal_id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to get terminal: {e}"))?
                    .ok_or_else(|| anyhow::anyhow!("Terminal {terminal_id} not found"))?;

                let task_id = terminal.workflow_task_id.clone();
                tracing::info!(
                    terminal_id = %terminal_id,
                    task_id = %task_id,
                    issue_count = issues.len(),
                    "FixIssues: creating fixer terminal"
                );

                let workflow_id = {
                    let state = self.state.read().await;
                    state.workflow_id.clone()
                };

                // 1. Create a fixer terminal reusing the same CLI/model config
                let fixer_terminal = self
                    .runtime_actions()?
                    .create_terminal(
                        &workflow_id,
                        RuntimeTerminalSpec {
                            terminal_id: None,
                            task_id: task_id.clone(),
                            cli_type_id: terminal.cli_type_id.clone(),
                            model_config_id: terminal.model_config_id.clone(),
                            custom_base_url: terminal.custom_base_url.clone(),
                            custom_api_key: None,
                            role: Some("fixer".to_string()),
                            role_description: Some(format!(
                                "Issue fixer for terminal {terminal_id}"
                            )),
                            order_index: None,
                            auto_confirm: Some(true),
                        },
                    )
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to create fixer terminal: {e}"))?;

                // 2. Start the fixer terminal (launches PTY)
                let fixer_terminal = self
                    .runtime_actions()?
                    .start_terminal(&fixer_terminal.id)
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to start fixer terminal: {e}"))?;

                // 3. Sync task state with the new terminal
                let planning_complete = self.task_planning_complete(&task_id).await;
                self.sync_task_state_from_db(&task_id, Some(planning_complete))
                    .await?;

                // 4. Build fix instruction with numbered issues and dispatch
                let numbered_issues: String = issues
                    .iter()
                    .enumerate()
                    .map(|(i, issue)| format!("{}. {}", i + 1, issue))
                    .collect::<Vec<_>>()
                    .join("\n");

                let fix_instruction = format!(
                    "Fix the following issues found during code review of terminal {terminal_id}:\n\
                     \n\
                     {numbered_issues}\n\
                     \n\
                     After fixing all issues, commit with metadata status: completed."
                );

                self.dispatch_terminal(&task_id, &fixer_terminal, &fix_instruction)
                    .await?;

                tracing::info!(
                    fixer_id = %fixer_terminal.id,
                    source_id = %terminal_id,
                    "FixIssues: fixer terminal dispatched"
                );
            }
            OrchestratorInstruction::MergeBranch {
                source_branch,
                target_branch,
            } => {
                tracing::info!(
                    source_branch = %source_branch,
                    target_branch = %target_branch,
                    "MergeBranch requested"
                );

                let workflow = self.load_workflow().await?;

                // Build a single-entry task_branches map using the workflow ID as key
                let task_branches: HashMap<String, String> =
                    [(workflow.id.clone(), source_branch.clone())].into_iter().collect();

                let base_repo_path = self.resolve_project_working_dir().await?;
                self.trigger_merge(
                    task_branches,
                    &base_repo_path.to_string_lossy(),
                    &target_branch,
                )
                .await?;
            }
            OrchestratorInstruction::PauseWorkflow { .. } => {
                tracing::warn!(
                    "Instruction variant PauseWorkflow is parsed but not yet implemented"
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
            TERMINAL_STATUS_WAITING,
            TERMINAL_STATUS_WORKING,
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
        let pty_session_id = if let Some(id) = active_terminal.pty_session_id.as_deref() { id.to_string() } else {
            let error_msg = format!(
                "Terminal {} has no PTY session, marking as failed",
                active_terminal.id
            );
            tracing::error!("{}", error_msg);

            // G11-009: Mark terminal as failed. If DB update fails, skip the broadcast
            // to avoid broadcasting state that wasn't persisted.
            if let Err(e) = db::models::Terminal::update_status(
                &self.db.pool,
                &active_terminal.id,
                TERMINAL_STATUS_FAILED,
            )
            .await
            {
                tracing::warn!(
                    terminal_id = %active_terminal.id,
                    error = %e,
                    "Failed to mark terminal as failed in DB, skipping broadcast"
                );
                return Err(anyhow::anyhow!(
                    "Terminal {} has no PTY session and DB update failed: {e}",
                    active_terminal.id
                ));
            }

            // Broadcast terminal status update
            if let Err(e) = self
                .message_bus
                .publish_workflow_event(
                    &workflow_id,
                    BusMessage::TerminalStatusUpdate {
                        workflow_id: workflow_id.clone(),
                        terminal_id: active_terminal.id.clone(),
                        status: TERMINAL_STATUS_FAILED.to_string(),
                    },
                )
                .await
            {
                tracing::warn!(error = %e, "Failed to publish terminal failed status event");
            }

            // Mark task as failed
            if let Err(e) =
                db::models::WorkflowTask::update_status(&self.db.pool, task_id, TASK_STATUS_FAILED).await
            {
                tracing::warn!(task_id = %task_id, error = %e, "Failed to mark task as failed in DB");
            }

            // Broadcast task status update
            if let Err(e) = self
                .message_bus
                .publish_workflow_event(
                    &workflow_id,
                    BusMessage::TaskStatusUpdate {
                        workflow_id: workflow_id.clone(),
                        task_id: task_id.to_string(),
                        status: TASK_STATUS_FAILED.to_string(),
                    },
                )
                .await
            {
                tracing::warn!(error = %e, "Failed to publish task failed status event");
            }

            // Broadcast error event for UI notification
            if let Err(e) = self
                .message_bus
                .publish_workflow_event(
                    &workflow_id,
                    BusMessage::Error {
                        workflow_id: workflow_id.clone(),
                        error: error_msg.clone(),
                    },
                )
                .await
            {
                tracing::warn!(error = %e, "Failed to publish dispatch error event");
            }

            return Err(anyhow::anyhow!(
                "Terminal {} has no PTY session",
                active_terminal.id
            ));
        };

        // G15-006: The terminal CAS (waiting→working) and task status update below are
        // not wrapped in a DB transaction. If the process crashes between the two writes,
        // the terminal may be "working" while the task remains in its previous status.
        // This is acceptable because:
        //   1. The orchestrator recovery path (`recover_running_workflows`) rebuilds
        //      in-memory state from the DB, which will detect the inconsistency.
        //   2. SQLite single-writer serialization prevents concurrent dispatch races.
        //   3. Wrapping in a transaction would require passing a transaction handle
        //      through the CAS helper, adding significant complexity for a rare edge case.

        // 4. Update task status to running.
        db::models::WorkflowTask::update_status(&self.db.pool, task_id, TASK_STATUS_RUNNING)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to update task status: {e}"))?;

        // 5. Broadcast live status updates for UI.
        if let Err(e) = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TerminalStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    terminal_id: active_terminal.id.clone(),
                    status: TERMINAL_STATUS_WORKING.to_string(),
                },
            )
            .await
        {
            tracing::warn!(error = %e, "Failed to publish terminal working status event");
        }
        if let Err(e) = self
            .message_bus
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TaskStatusUpdate {
                    workflow_id: workflow_id.clone(),
                    task_id: task_id.to_string(),
                    status: TASK_STATUS_RUNNING.to_string(),
                },
            )
            .await
        {
            tracing::warn!(error = %e, "Failed to publish task running status event");
        }

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
            // G05-007: Send shutdown signals to both the session topic and the
            // terminal.input topic so the PTY bridge receives them regardless of
            // which topic it is subscribed to.
            if let Err(e) = self
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
                .await
            {
                tracing::warn!(
                    terminal_id = %terminal.id,
                    error = %e,
                    "Failed to publish Ctrl-C to session topic during completion shutdown"
                );
            }

            // Also send to terminal.input topic for targeted delivery
            self.message_bus
                .publish_terminal_input(&terminal.id, session_id, "\u{3}", None)
                .await;

            if let Err(e) = self
                .message_bus
                .publish(session_id, BusMessage::Shutdown)
                .await
            {
                tracing::warn!(
                    terminal_id = %terminal.id,
                    error = %e,
                    "Failed to publish Shutdown to session topic during completion shutdown"
                );
            }
            tracing::info!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                pty_session_id = %session_id,
                "Issued completion shutdown signals for terminal"
            );
        }

        if let Some(pid) = terminal.process_id {
            if let Err(error) = self.force_terminate_terminal_process(pid).await {
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

    async fn force_terminate_terminal_process(&self, pid: i32) -> anyhow::Result<()> {
        if pid <= 0 {
            return Err(anyhow!("invalid process id: {pid}"));
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
            // G05-008: Use tokio::time::sleep instead of std::thread::sleep
            // to avoid blocking the async runtime.
            sleep(Duration::from_millis(150)).await;
            let _ = signal::kill(target_pid, Signal::SIGKILL);
            return Ok(());
        }

        #[cfg(windows)]
        {
            let output = std::process::Command::new("taskkill")
                .args(["/PID", &pid.to_string(), "/T", "/F"])
                .output()
                .map_err(|e| anyhow!("failed to execute taskkill: {e}"))?;

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

            return Err(anyhow!("taskkill failed for {pid}: {stderr}"));
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
        prev_context: Option<&PreviousTerminalContext>,
    ) -> String {
        let mut parts = vec![format!("Start task: {} ({})", task.name, task.id)];

        if let Some(description) = &task.description {
            let normalized = description.split_whitespace().collect::<Vec<_>>().join(" ");
            if !normalized.is_empty() {
                if total_terminals > 1 {
                    let summary = Self::truncate_instruction_text(&normalized, 200);
                    parts.push(format!("Task objective: {summary}"));
                } else {
                    parts.push(format!("Task description: {normalized}"));
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
                parts.push(format!("Role description: {normalized}"));
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

        if let Some(ctx) = prev_context {
            parts.push(format!(
                "--- Previous Terminal Context ---\nRole: {} | Status: {}\nLast Commit: {}\nHandoff Notes: {}",
                ctx.role, ctx.status, ctx.commit_message, ctx.handoff_notes
            ));
        }

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
    ///
    /// # G03-005: Parallel dispatch via join_all
    ///
    /// Phase 1 (sequential): Load task data, initialize shared state, and build
    /// dispatch payloads. State initialization must be sequential because it holds
    /// a write lock on self.state.
    ///
    /// Phase 2 (parallel): Dispatch all collected terminals concurrently using
    /// `futures::future::join_all`. Individual dispatch failures are logged but
    /// do not abort other dispatches.
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

        // Phase 1 (sequential): build the list of (task_id, terminal, instruction) to dispatch.
        // State initialization requires the write lock, so this must remain sequential.
        let mut dispatch_queue: Vec<(String, db::models::Terminal, String)> = Vec::new();

        for task in tasks {
            // Skip tasks that are already completed, failed, or cancelled
            if task.status == TASK_STATUS_COMPLETED
                || task.status == TASK_STATUS_FAILED
                || task.status == TASK_STATUS_CANCELLED
            {
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

            // Initialize task state (requires write lock -- must stay sequential)
            {
                let mut state = self.state.write().await;
                if state.task_states.contains_key(&task.id) {
                    state.sync_task_terminals(
                        task.id.clone(),
                        terminals.iter().map(|terminal| terminal.id.clone()).collect(),
                        true,
                    );
                } else {
                    state.init_task(
                        task.id.clone(),
                        terminals.iter().map(|terminal| terminal.id.clone()).collect(),
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
            if terminal.status != TERMINAL_STATUS_WAITING {
                tracing::debug!(
                    "Skipping terminal {} for task {} due to status {}",
                    terminal.id,
                    task.id,
                    terminal.status
                );
                continue;
            }

            // Build instruction and enqueue for parallel dispatch
            let instruction =
                Self::build_task_instruction(&workflow_id, &task, &terminal, terminals.len(), None);
            dispatch_queue.push((task.id.clone(), terminal, instruction));
        }

        if dispatch_queue.is_empty() {
            return Ok(());
        }

        tracing::info!(
            "Phase 2: dispatching {} terminals in parallel",
            dispatch_queue.len()
        );

        // Phase 2 (parallel): dispatch all collected terminals concurrently.
        // G03-005: join_all reduces startup latency when there are many tasks.
        let dispatch_futures: Vec<_> = dispatch_queue
            .iter()
            .map(|(task_id, terminal, instruction)| {
                self.dispatch_terminal(task_id, terminal, instruction)
            })
            .collect();

        let results = future::join_all(dispatch_futures).await;
        for (result, (task_id, terminal, _)) in results.into_iter().zip(dispatch_queue.iter()) {
            if let Err(e) = result {
                tracing::error!(
                    "Failed to auto-dispatch terminal {} for task {}: {}",
                    terminal.id,
                    task_id,
                    e
                );
                // Continue -- other tasks' dispatches are unaffected
            }
        }

        Ok(())
    }

    /// Publish any provider state-change events collected during the last LLM call.
    async fn publish_provider_events(&self) {
        let events = self.llm_client.take_provider_events().await;
        if events.is_empty() {
            return;
        }

        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        for event in events {
            let message = BusMessage::ProviderStateChanged {
                workflow_id: workflow_id.clone(),
                event,
            };
            if let Err(e) = self
                .message_bus
                .publish_workflow_event(&workflow_id, message)
                .await
            {
                tracing::warn!(
                    "Failed to publish provider state change event for workflow {}: {}",
                    workflow_id,
                    e,
                );
            }
        }
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

    // G15-011: TOCTOU note — There is a small race window between reading workflow/task
    // statuses and updating workflow status to "completed". A terminal could complete
    // concurrently between the reads and the write, causing a premature completion.
    // This is acceptable because:
    //   1. The orchestrator event loop is single-threaded per workflow, so the only
    //      concurrent writes come from the deferred completion spawned tasks.
    //   2. SQLite single-writer ensures the status update is atomic.
    //   3. If a terminal completes after the check, the next event loop iteration
    //      will re-enter this method and the state will converge correctly.
    async fn auto_sync_workflow_completion(&self, workflow_id: &str) -> anyhow::Result<()> {
        let Some(workflow) = db::models::Workflow::find_by_id(&self.db.pool, workflow_id).await?
        else {
            return Ok(());
        };

        // G14-005: Only auto-sync when workflow is in "running" status.
        // G04-004: Also exclude 'merging' status — the workflow is in the process
        // of merging branches and should not be auto-synced to completed.
        if workflow.status != WORKFLOW_STATUS_RUNNING {
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
                TERMINAL_STATUS_NOT_STARTED | TERMINAL_STATUS_STARTING | TERMINAL_STATUS_WAITING | TERMINAL_STATUS_WORKING
            )
        });
        if has_runnable_terminals {
            return Ok(());
        }
        if !tasks.is_empty() && tasks.iter().any(|task| task.status != TASK_STATUS_COMPLETED) {
            return Ok(());
        }

        // CAS: only transition running → completed to prevent overwriting concurrent
        // state changes (e.g., pause or merge already in progress).
        let transitioned =
            db::models::Workflow::set_completed_from_running(&self.db.pool, workflow_id).await?;
        if !transitioned {
            tracing::warn!(
                workflow_id = %workflow_id,
                "Auto-sync to completed skipped: workflow status changed concurrently"
            );
            return Ok(());
        }

        if let Err(e) = self
            .message_bus
            .publish_workflow_event(
                workflow_id,
                BusMessage::StatusUpdate {
                    workflow_id: workflow_id.to_string(),
                    status: WORKFLOW_STATUS_COMPLETED.to_string(),
                },
            )
            .await
        {
            tracing::warn!(error = %e, "Failed to publish workflow completion status event");
        }

        tracing::info!(
            workflow_id = %workflow_id,
            "Workflow auto-synced to completed after all tasks completed"
        );

        // Auto-merge completed task branches
        if self.config.auto_merge_on_completion {
            match self.execute_auto_merge().await {
                Ok(()) => {
                    tracing::info!(
                        workflow_id = %workflow_id,
                        "Auto-merge completed successfully"
                    );
                }
                Err(e) => {
                    tracing::warn!(
                        workflow_id = %workflow_id,
                        error = %e,
                        "Auto-merge failed, workflow remains completed but branches not merged"
                    );
                    if let Err(e) = self
                        .message_bus
                        .publish_workflow_event(
                            workflow_id,
                            BusMessage::Error {
                                workflow_id: workflow_id.to_string(),
                                error: format!("Auto-merge failed: {e}"),
                            },
                        )
                        .await
                    {
                        tracing::warn!(error = %e, "Failed to publish auto-merge failure event");
                    }
                }
            }
        }

        self.maybe_save_state().await;

        Ok(())
    }

    /// Auto-merge all completed task branches after workflow completion.
    /// Collects completed task branches and delegates to `trigger_merge`.
    async fn execute_auto_merge(&self) -> anyhow::Result<()> {
        let workflow = self.load_workflow().await?;
        let tasks =
            db::models::WorkflowTask::find_by_workflow(&self.db.pool, &workflow.id).await?;

        let task_branches: HashMap<String, String> = tasks
            .into_iter()
            .filter(|task| task.status == TASK_STATUS_COMPLETED && !task.branch.is_empty())
            .map(|task| (task.id.clone(), task.branch.clone()))
            .collect();

        if task_branches.is_empty() {
            tracing::info!(
                workflow_id = %workflow.id,
                "No completed task branches to merge"
            );
            return Ok(());
        }

        // G06-002: acquire the per-workflow merge lock so that auto-merge and
        // manual merge (REST endpoint) cannot run concurrently for the same workflow.
        let _merge_guard =
            crate::services::merge_coordinator::acquire_workflow_merge_lock(&workflow.id).await;

        let base_repo_path = self.resolve_project_working_dir().await?;
        self.trigger_merge(
            task_branches,
            &base_repo_path.to_string_lossy(),
            &workflow.target_branch,
        )
        .await
    }

    /// Triggers merge of all completed task branches into the target branch.
    ///
    /// Called when all terminals for a task have completed successfully.
    /// Merges each task branch into the target branch using squash merge via
    /// `MergeCoordinator` for centralised conflict handling.
    ///
    /// # Arguments
    /// * `task_branches` - Map of task_id to branch name for all completed tasks
    /// * `base_repo_path` - Path to the base repository
    /// * `target_branch` - Target branch name (e.g., "main")
    ///
    /// # Returns
    /// * `Ok(())` - All merges completed successfully
    /// * `Err(anyhow::Error)` - If any merge fails
    ///
    /// # Fixes applied
    /// - G06-003: worktree path resolved via WorktreeManager::get_worktree_base_dir()
    ///   with legacy fallback instead of hardcoded `<repo>/worktrees/<branch>`.
    /// - G06-004: records successfully merged tasks; marks workflow as
    ///   `merge_partial_failed` when a subsequent task fails.
    /// - G06-005: checks task branch ancestry before merge (idempotency).
    /// - G06-008: delegates to MergeCoordinator instead of calling GitService directly.
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

        // G06-008: use MergeCoordinator for centralised merge handling.
        let coordinator = crate::services::merge_coordinator::MergeCoordinator::new(
            Arc::clone(&self.db),
            Arc::clone(&self.message_bus),
            crate::services::git::GitService::new(),
        );

        // G06-004: track which tasks have been successfully merged so we can
        // mark the workflow as `merge_partial_failed` if a later task fails.
        let mut successfully_merged: Vec<String> = Vec::new();

        // Merge each task branch
        for (task_id, task_branch) in task_branches {
            if task_branch.eq_ignore_ascii_case(target_branch) {
                tracing::info!(
                    "Skipping merge for task {} because task branch '{}' already equals target branch '{}'.",
                    task_id,
                    task_branch,
                    target_branch
                );
                successfully_merged.push(task_id);
                continue;
            }

            // G06-005: idempotency — skip branch if it is already an ancestor of target.
            let already_merged = {
                let git_exe = utils::shell::resolve_executable_path("git").await;
                if let Some(git) = git_exe {
                    let output = tokio::process::Command::new(git)
                        .args([
                            "merge-base",
                            "--is-ancestor",
                            &task_branch,
                            target_branch,
                        ])
                        .current_dir(base_repo_path)
                        .output()
                        .await;
                    matches!(output, Ok(o) if o.status.success())
                } else {
                    false
                }
            };

            if already_merged {
                tracing::info!(
                    task_id = %task_id,
                    task_branch = %task_branch,
                    "Task branch already merged into target, skipping (idempotency)"
                );
                successfully_merged.push(task_id);
                continue;
            }

            tracing::info!("Merging task branch {} for task {}", task_branch, task_id);

            // G06-003: resolve worktree path via WorktreeManager instead of hardcoding.
            let managed_path = crate::services::worktree_manager::WorktreeManager::get_worktree_base_dir()
                .join(&task_branch);
            let task_worktree_path = if managed_path.exists() {
                managed_path
            } else {
                // Legacy fallback: <repo>/worktrees/<branch>
                base_repo_path.join("worktrees").join(&task_branch)
            };

            // Perform the merge via MergeCoordinator (G06-008).
            let commit_message = format!("Merge task {task_id} ({task_branch})");
            match coordinator.merge_task_branch(
                &task_id,
                &workflow_id,
                &task_branch,
                target_branch,
                base_repo_path,
                &task_worktree_path,
                &commit_message,
            ).await {
                Ok(_commit_sha) => {
                    tracing::info!(
                        "Successfully merged task branch {} for task {}",
                        task_branch,
                        task_id
                    );
                    successfully_merged.push(task_id);
                }
                Err(e) => {
                    // G06-004: partial failure — if some tasks were already merged,
                    // surface a distinct status so the operator can take action.
                    if !successfully_merged.is_empty() {
                        tracing::error!(
                            workflow_id = %workflow_id,
                            task_id = %task_id,
                            already_merged = ?successfully_merged,
                            error = %e,
                            "Partial merge failure: {} tasks merged before failure",
                            successfully_merged.len()
                        );
                        // Mark workflow with a non-blocking status that indicates partial merge.
                        // "merge_partial_failed" is a sub-state of "merging" stored in DB;
                        // if the value is not in the allowed transitions the update is a no-op.
                        if let Err(db_err) = db::models::Workflow::update_status(
                            &self.db.pool,
                            &workflow_id,
                            WORKFLOW_STATUS_MERGE_PARTIAL_FAILED,
                        ).await {
                            tracing::warn!(
                                workflow_id = %workflow_id,
                                error = %db_err,
                                "Failed to record partial merge failure status"
                            );
                        }
                    }

                    return Err(anyhow::anyhow!(
                        "Merge failed for task branch {task_branch} (task {task_id}): {e}"
                    ));
                }
            }

            // G06-006: clean up worktree after successful individual task merge.
            let cleanup_data = crate::services::worktree_manager::WorktreeCleanup::new(
                task_worktree_path,
                Some(base_repo_path.to_path_buf()),
            );
            if let Err(e) = crate::services::worktree_manager::WorktreeManager::cleanup_worktree(
                &cleanup_data,
            ).await {
                tracing::warn!(
                    workflow_id = %workflow_id,
                    task_branch = %task_branch,
                    error = %e,
                    "Failed to clean up worktree after merge (non-fatal)"
                );
            }
        }

        tracing::info!(
            "All task branches merged successfully into {}",
            target_branch
        );

        // Broadcast final completed status now that ALL branches are merged (G06-007).
        let message = BusMessage::StatusUpdate {
            workflow_id: workflow_id.clone(),
            status: WORKFLOW_STATUS_COMPLETED.to_string(),
        };
        self.message_bus
            .publish_workflow_event(&workflow_id, message)
            .await?;

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
                    "Workflow task {workflow_task_id} not found for terminal {terminal_id}"
                )
            })?;

        if task.workflow_id != workflow_id {
            return Err(anyhow!(
                "Terminal {terminal_id} does not belong to workflow {workflow_id}"
            ));
        }

        let session_id = terminal
            .pty_session_id
            .or(terminal.session_id)
            .filter(|id| !id.trim().is_empty())
            .ok_or_else(|| {
                anyhow!(
                    "Terminal {terminal_id} has no session_id for prompt response"
                )
            })?;

        let handled = self
            .prompt_handler
            .handle_user_prompt_response(terminal_id, &session_id, &workflow_id, user_response)
            .await;

        if !handled {
            return Err(anyhow!(
                "Terminal {terminal_id} is not waiting for prompt approval in workflow {workflow_id}"
            ));
        }

        Ok(())
    }

    /// Handle direct user chat sent to the orchestrator.
    ///
    /// The message is appended to conversation history, then executed through
    /// the same LLM + instruction pipeline used by terminal completion events.
    pub async fn submit_orchestrator_chat_message(&self, user_message: &str) -> anyhow::Result<()> {
        let message = user_message.trim();
        if message.is_empty() {
            return Err(anyhow!("Orchestrator chat message is empty"));
        }

        let workflow_id = {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Processing;
            state.workflow_id.clone()
        };

        let result = async {
            let response = self.call_llm(message).await?;
            self.execute_instruction(&response).await
        }
        .await;

        {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Idle;
        }

        result.map_err(|error| {
            anyhow!(
                "Failed to process orchestrator chat message for workflow {workflow_id}: {error}"
            )
        })
    }

    /// Return a snapshot of current orchestrator conversation history.
    pub async fn get_conversation_history(&self) -> Vec<LLMMessage> {
        let state = self.state.read().await;
        state.conversation_history.clone()
    }

    /// Returns live provider status from the underlying LLM client.
    pub async fn get_provider_status(&self) -> Vec<super::resilient_llm::ProviderStatusReport> {
        self.llm_client.provider_status().await
    }

    /// Reset a provider's circuit breaker by name.
    pub async fn reset_provider(&self, provider_name: &str) -> bool {
        self.llm_client.reset_provider(provider_name).await
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
            .ok_or_else(|| anyhow::anyhow!("Workflow {workflow_id} not found"))?;

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

            // G24-002: Publish any provider state-change events that occurred
            // during the direct chat() invocation (same as call_llm does).
            self.publish_provider_events().await;

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

    /// Fetches a truncated diff summary for a given commit hash.
    async fn fetch_diff_for_review(&self, commit_hash: &str) -> anyhow::Result<String> {
        let working_dir = self.resolve_project_working_dir().await?;
        let output = tokio::process::Command::new("git")
            .args([
                "diff",
                &format!("{commit_hash}~1..{commit_hash}"),
                "--stat",
            ])
            .current_dir(&working_dir)
            .output()
            .await?;
        let diff = String::from_utf8_lossy(&output.stdout).to_string();
        Ok(if diff.len() > 2000 {
            diff[..2000].to_string() + "\n[...truncated]"
        } else {
            diff
        })
    }
}

/// Truncate a string to `max_chars`, appending a marker if truncated.
fn truncate_with_marker(s: &str, max_chars: usize) -> String {
    if s.len() <= max_chars {
        s.to_string()
    } else {
        let truncated = &s[..s.floor_char_boundary(max_chars.saturating_sub(16))];
        format!("{truncated}\n[...truncated]")
    }
}

/// Collect terminal completion context (log summary, diff stat, commit body)
/// for injection into LLM completion prompts.
async fn fetch_terminal_completion_context(
    db: &DBService,
    terminal_id: &str,
    commit_hash: &str,
    working_dir: &Path,
) -> anyhow::Result<TerminalCompletionContext> {
    // 1. Query terminal logs (returned in DESC order, reverse for chronological)
    let log_summary = match db::models::terminal::TerminalLog::find_by_terminal(
        &db.pool,
        terminal_id,
        Some(COMPLETION_CONTEXT_LOG_LINES as i32),
    )
    .await
    {
        Ok(mut logs) => {
            logs.reverse();
            let joined = logs
                .iter()
                .map(|l| l.content.as_str())
                .collect::<Vec<_>>()
                .join("\n");
            truncate_with_marker(&joined, COMPLETION_CONTEXT_LOG_MAX_CHARS)
        }
        Err(e) => {
            tracing::warn!(terminal_id = %terminal_id, error = %e, "Failed to fetch terminal logs for completion context");
            String::new()
        }
    };

    // 2. Get diff stat via git
    let diff_stat = match tokio::process::Command::new("git")
        .args(["diff", "--stat", "HEAD~1..HEAD"])
        .current_dir(working_dir)
        .output()
        .await
    {
        Ok(output) if output.status.success() => {
            let stdout = String::from_utf8_lossy(&output.stdout).to_string();
            truncate_with_marker(&stdout, COMPLETION_CONTEXT_DIFF_MAX_CHARS)
        }
        Ok(_) => String::new(),
        Err(e) => {
            tracing::warn!(error = %e, "Failed to run git diff --stat for completion context");
            String::new()
        }
    };

    // 3. Get commit body
    let commit_body = if commit_hash == "N/A" {
        String::new()
    } else {
        match tokio::process::Command::new("git")
            .args(["show", "-s", "--format=%B", commit_hash])
            .current_dir(working_dir)
            .output()
            .await
        {
            Ok(output) if output.status.success() => {
                let stdout = String::from_utf8_lossy(&output.stdout).to_string();
                truncate_with_marker(&stdout, COMPLETION_CONTEXT_BODY_MAX_CHARS)
            }
            Ok(_) => String::new(),
            Err(e) => {
                tracing::warn!(commit_hash = %commit_hash, error = %e, "Failed to get commit body for completion context");
                String::new()
            }
        }
    };

    Ok(TerminalCompletionContext {
        log_summary,
        diff_stat,
        commit_body,
    })
}

/// Fetch context from the previous completed terminal in the same task.
/// Returns None if this is the first terminal or no previous terminal has completed.
async fn fetch_previous_terminal_context(
    db: &DBService,
    task_id: &str,
    workflow_id: &str,
    current_terminal_order: i32,
) -> anyhow::Result<Option<PreviousTerminalContext>> {
    let prev_order = current_terminal_order - 1;
    if prev_order < 0 {
        return Ok(None);
    }

    // 1. Query all terminals for the task
    let terminals = db::models::Terminal::find_by_task(&db.pool, task_id).await?;

    // 2. Find the terminal with order_index == prev_order
    let prev_terminal = match terminals.iter().find(|t| t.order_index == prev_order) {
        Some(t) => t,
        None => return Ok(None),
    };

    // 3. Only use completed or failed terminals
    if prev_terminal.status != TERMINAL_STATUS_COMPLETED
        && prev_terminal.status != TERMINAL_STATUS_FAILED
    {
        return Ok(None);
    }

    // 4. Get commit message: prefer last_commit_message on the terminal, fall back to git_event
    let commit_message = if let Some(ref msg) = prev_terminal.last_commit_message {
        msg.clone()
    } else {
        // Fall back to git_events for this workflow
        let events =
            db::models::git_event::GitEvent::find_by_workflow(&db.pool, workflow_id)
                .await
                .unwrap_or_default();
        events
            .iter()
            .find(|e| e.terminal_id.as_deref() == Some(&prev_terminal.id))
            .map(|e| e.commit_message.clone())
            .unwrap_or_default()
    };

    // 5. Extract handoff notes from the commit message
    let handoff_notes = extract_handoff_notes(&commit_message);

    // 6. Truncate fields
    let commit_message = truncate_with_marker(&commit_message, HANDOFF_COMMIT_MAX_CHARS);
    let handoff_notes = truncate_with_marker(&handoff_notes, HANDOFF_NOTES_MAX_CHARS);

    Ok(Some(PreviousTerminalContext {
        role: prev_terminal.role.clone().unwrap_or_default(),
        status: prev_terminal.status.clone(),
        commit_message,
        handoff_notes,
    }))
}

/// Extract handoff notes from a commit message.
///
/// Looks for "HANDOFF:" or "Handoff Notes:" markers. If not found, returns
/// the commit message with the METADATA block stripped.
fn extract_handoff_notes(commit_message: &str) -> String {
    // Look for handoff markers (case-insensitive)
    let lower = commit_message.to_lowercase();
    for marker in &["handoff:", "handoff notes:"] {
        if let Some(pos) = lower.find(marker) {
            let start = pos + marker.len();
            let notes = commit_message[start..].trim();
            // Stop at METADATA separator if present
            if let Some(meta_pos) = notes.find(GIT_COMMIT_METADATA_SEPARATOR) {
                return notes[..meta_pos].trim().to_string();
            }
            return notes.to_string();
        }
    }

    // No handoff marker found: strip METADATA block and return the rest
    if let Some(meta_pos) = commit_message.find(GIT_COMMIT_METADATA_SEPARATOR) {
        commit_message[..meta_pos].trim().to_string()
    } else {
        commit_message.trim().to_string()
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
            OrchestratorAgent::build_task_instruction(workflow_id, &task, &terminal, 3, None);

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
            OrchestratorAgent::build_task_instruction(workflow_id, &task, &terminal, 1, None);

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
            r"
            INSERT INTO workflow (
                id, project_id, name, status, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ",
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
            r"
            INSERT INTO workflow_task (
                id, workflow_id, name, branch, status, order_index,
                started_at, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ",
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
            r"
            INSERT INTO terminal (
                id, workflow_task_id, cli_type_id, model_config_id,
                order_index, status, pty_session_id, started_at, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
            ",
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
            r"
            UPDATE terminal
            SET status = 'completed', completed_at = ?1, updated_at = ?1
            WHERE id = ?2
            ",
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
