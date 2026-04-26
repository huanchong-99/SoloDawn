//! Terminal launcher
//!
//! Serial terminal launcher with model switching integration.

use std::{path::PathBuf, sync::Arc, time::Duration};

// Re-export types
pub use db::models::Terminal;
use db::{
    DBService,
    models::{
        cli_type,
        execution_process::{CreateExecutionProcess, ExecutionProcess, ExecutionProcessRunReason},
        session::Session,
    },
};
use executors::{
    actions::{
        ExecutorAction, ExecutorActionType, coding_agent_initial::CodingAgentInitialRequest,
    },
    executors::BaseCodingAgent,
    profile::ExecutorProfileId,
};
use uuid::Uuid;

#[cfg(test)]
use super::process::SpawnCommand;
use super::{
    bridge::TerminalBridge,
    process::{DEFAULT_COLS, DEFAULT_ROWS, ProcessHandle, ProcessManager},
    prompt_watcher::PromptWatcher,
};
use crate::services::{
    cc_switch::CCSwitchService,
    orchestrator::{BusMessage, SharedMessageBus, constants::WORKFLOW_TOPIC_PREFIX},
};

/// Terminal launcher for serial terminal startup
pub struct TerminalLauncher {
    db: Arc<DBService>,
    cc_switch: Arc<CCSwitchService>,
    process_manager: Arc<ProcessManager>,
    working_dir: PathBuf,
    message_bus: Option<SharedMessageBus>,
    /// Optional terminal bridge for MessageBus -> PTY stdin forwarding
    terminal_bridge: Option<TerminalBridge>,
    /// Optional prompt watcher for PTY output prompt detection
    prompt_watcher: Option<PromptWatcher>,
}

/// Result of a terminal launch operation
#[derive(Debug)]
pub struct LaunchResult {
    pub terminal_id: String,
    pub process_handle: Option<ProcessHandle>,
    pub success: bool,
    pub error: Option<String>,
}

impl TerminalLauncher {
    /// Create a new terminal launcher
    ///
    /// # Arguments
    /// * `db` - Database service
    /// * `cc_switch` - CC switch service for model switching
    /// * `process_manager` - Process manager for lifecycle management
    /// * `working_dir` - Working directory for spawned processes
    pub fn new(
        db: Arc<DBService>,
        cc_switch: Arc<CCSwitchService>,
        process_manager: Arc<ProcessManager>,
        working_dir: PathBuf,
    ) -> Self {
        Self {
            db,
            cc_switch,
            process_manager,
            working_dir,
            message_bus: None,
            terminal_bridge: None,
            prompt_watcher: None,
        }
    }

    /// Create a new terminal launcher with MessageBus bridge
    ///
    /// This enables Orchestrator -> PTY stdin communication for terminal messages.
    ///
    /// # Arguments
    /// * `db` - Database service
    /// * `cc_switch` - CC switch service for model switching
    /// * `process_manager` - Process manager for lifecycle management
    /// * `working_dir` - Working directory for spawned processes
    /// * `message_bus` - Shared message bus for terminal bridge
    /// * `prompt_watcher` - Shared prompt watcher instance from deployment
    pub fn with_message_bus(
        db: Arc<DBService>,
        cc_switch: Arc<CCSwitchService>,
        process_manager: Arc<ProcessManager>,
        working_dir: PathBuf,
        message_bus: SharedMessageBus,
        prompt_watcher: PromptWatcher,
    ) -> Self {
        let terminal_bridge =
            TerminalBridge::new(message_bus.clone(), Arc::clone(&process_manager));
        Self {
            db,
            cc_switch,
            process_manager,
            working_dir,
            message_bus: Some(message_bus.clone()),
            terminal_bridge: Some(terminal_bridge),
            prompt_watcher: Some(prompt_watcher),
        }
    }

    /// Launch all terminals for a workflow (serial execution)
    ///
    /// # Arguments
    /// * `workflow_id` - The workflow ID
    ///
    /// # Returns
    /// A vector of launch results for each terminal
    pub async fn launch_all(&self, workflow_id: &str) -> anyhow::Result<Vec<LaunchResult>> {
        let terminals = Terminal::find_by_workflow(&self.db.pool, workflow_id).await?;
        let mut results = Vec::new();

        tracing::info!("Launching prepared terminals for workflow {}", workflow_id);

        for terminal in terminals {
            if terminal.status != "starting" {
                tracing::info!(
                    terminal_id = %terminal.id,
                    workflow_id = %workflow_id,
                    status = %terminal.status,
                    "Skipping terminal launch because it has not acquired a launch slot"
                );
                continue;
            }

            let result = self.launch_terminal(&terminal).await;
            results.push(result);
            // No delay needed - environment variable injection is immediate
            // (removed 500ms delay that was required for global config file switching)
        }

        Ok(results)
    }

    /// Launch a single terminal
    ///
    /// # Arguments
    /// * `terminal` - The terminal configuration
    ///
    /// # Returns
    /// A launch result indicating success or failure
    pub async fn launch_terminal(&self, terminal: &Terminal) -> LaunchResult {
        let terminal_id = terminal.id.clone();
        let workflow_id = self
            .get_workflow_id_for_terminal(&terminal.workflow_task_id)
            .await
            .ok()
            .flatten();

        // 1. Get CLI type information
        let cli_type =
            match cli_type::CliType::find_by_id(&self.db.pool, &terminal.cli_type_id).await {
                Ok(Some(cli)) => cli,
                Ok(None) => {
                    return LaunchResult {
                        terminal_id,
                        process_handle: None,
                        success: false,
                        error: Some("CLI type not found".to_string()),
                    };
                }
                Err(e) => {
                    return LaunchResult {
                        terminal_id,
                        process_handle: None,
                        success: false,
                        error: Some(format!("Database error: {e}")),
                    };
                }
            };

        // 2. Create Session for execution context tracking
        // Get workflow task to find associated workspace
        let workspace_id = match self
            .get_workspace_for_terminal(&terminal.workflow_task_id)
            .await
        {
            Ok(Some(id)) => Some(id),
            Ok(None) => {
                tracing::warn!("No workspace found for terminal {}", terminal_id);
                None
            }
            Err(e) => {
                tracing::error!(
                    "Failed to get workspace for terminal {}: {}",
                    terminal_id,
                    e
                );
                None
            }
        };

        let (session_id, execution_process_id) = if let Some(ws_id) = workspace_id {
            match Session::create_for_terminal(
                &self.db.pool,
                ws_id,
                Some(cli_type.name.clone()),
                Some(terminal_id.clone()),
            )
            .await
            {
                Ok(session) => {
                    tracing::info!(
                        "Created session {} for terminal {}",
                        session.id,
                        terminal_id
                    );

                    // Create ExecutionProcess for tracking this terminal launch
                    // Create a simple coding agent action
                    let executor_action = ExecutorAction::new(
                        ExecutorActionType::CodingAgentInitialRequest(CodingAgentInitialRequest {
                            prompt: format!(
                                "Terminal launched for workflow task: {}",
                                terminal.workflow_task_id
                            ),
                            executor_profile_id: ExecutorProfileId::new(
                                BaseCodingAgent::ClaudeCode,
                            ),
                            working_dir: None,
                            allow_user_questions: false,
                        }),
                        None,
                    );

                    let create_exec_process = CreateExecutionProcess {
                        session_id: session.id,
                        executor_action,
                        run_reason: ExecutionProcessRunReason::CodingAgent,
                    };

                    let exec_process_result = ExecutionProcess::create(
                        &self.db.pool,
                        &create_exec_process,
                        Uuid::new_v4(),
                        &[], // Empty repo states for terminal launch
                    )
                    .await;

                    let (sess_id, exec_id) = match exec_process_result {
                        Ok(exec_process) => {
                            tracing::info!(
                                "Created execution process {} for terminal {}",
                                exec_process.id,
                                terminal_id
                            );
                            (
                                Some(session.id.to_string()),
                                Some(exec_process.id.to_string()),
                            )
                        }
                        Err(e) => {
                            tracing::error!(
                                "Failed to create execution process for terminal {}: {}",
                                terminal_id,
                                e
                            );
                            (Some(session.id.to_string()), None)
                        }
                    };

                    // Update terminal with session binding immediately after creation
                    // This ensures session is bound even if process spawn fails later
                    if let Err(e) = Terminal::update_session(
                        &self.db.pool,
                        &terminal_id,
                        sess_id.as_deref(),
                        exec_id.as_deref(),
                    )
                    .await
                    {
                        tracing::error!("Failed to update terminal session binding: {}", e);
                    }

                    (sess_id, exec_id)
                }
                Err(e) => {
                    tracing::error!(
                        "Failed to create session for terminal {}: {}",
                        terminal_id,
                        e
                    );
                    (None, None)
                }
            }
        } else {
            (None, None)
        };

        // 3. Get CLI command for the terminal
        let cli_command = self.get_cli_command(&cli_type.name);

        // 4. Build spawn configuration (process-level isolation, no global config changes)
        let spawn_config = match self
            .cc_switch
            .build_launch_config(
                terminal,
                &cli_command,
                &self.working_dir,
                terminal.auto_confirm,
            )
            .await
        {
            Ok(config) => config,
            Err(e) => {
                tracing::error!(
                    terminal_id = %terminal_id,
                    error = %e,
                    "Failed to build launch config for terminal"
                );
                return LaunchResult {
                    terminal_id,
                    process_handle: None,
                    success: false,
                    error: Some(format!("Config build failed: {e}")),
                };
            }
        };

        // 5. Spawn PTY process with environment variable injection
        match self
            .process_manager
            .spawn_pty_with_config(&terminal_id, &spawn_config, DEFAULT_COLS, DEFAULT_ROWS)
            .await
        {
            Ok(handle) => {
                // Update terminal status in database
                if let Err(e) = Terminal::set_waiting(&self.db.pool, &terminal_id).await {
                    return self
                        .rollback_launch_after_spawn(
                            &terminal_id,
                            format!("Failed to set terminal started status: {e}"),
                        )
                        .await;
                }
                self.broadcast_terminal_status(workflow_id.as_deref(), &terminal_id, "waiting")
                    .await;
                let pid = i32::try_from(handle.pid).ok();
                if let Err(e) = Terminal::update_process(
                    &self.db.pool,
                    &terminal_id,
                    pid,
                    Some(&handle.session_id),
                )
                .await
                {
                    return self
                        .rollback_launch_after_spawn(
                            &terminal_id,
                            format!("Failed to update terminal process binding: {e}"),
                        )
                        .await;
                }

                // Update terminal with session and execution process binding
                if session_id.is_some() || execution_process_id.is_some() {
                    if let Err(e) = Terminal::update_session(
                        &self.db.pool,
                        &terminal_id,
                        session_id.as_deref(),
                        execution_process_id.as_deref(),
                    )
                    .await
                    {
                        return self
                            .rollback_launch_after_spawn(
                                &terminal_id,
                                format!("Failed to update terminal session binding: {e}"),
                            )
                            .await;
                    }
                }

                // Attach terminal logger for output persistence
                if let Err(e) = self
                    .process_manager
                    .attach_terminal_logger(Arc::clone(&self.db), &terminal_id, "stdout", 1)
                    .await
                {
                    return self
                        .rollback_launch_after_spawn(
                            &terminal_id,
                            format!("Failed to attach terminal logger: {e}"),
                        )
                        .await;
                }

                // Register terminal bridge for MessageBus -> PTY stdin forwarding
                if let Some(ref bridge) = self.terminal_bridge {
                    if let Err(e) = bridge.register(&terminal_id, &handle.session_id).await {
                        return self
                            .rollback_launch_after_spawn(
                                &terminal_id,
                                format!("Failed to register terminal bridge: {e}"),
                            )
                            .await;
                    }
                    tokio::time::sleep(Duration::from_millis(50)).await;
                    if !bridge.is_registered(&handle.session_id).await {
                        return self
                            .rollback_launch_after_spawn(
                                &terminal_id,
                                "Terminal bridge registration verification failed".to_string(),
                            )
                            .await;
                    }
                    tracing::debug!(
                        terminal_id = %terminal_id,
                        pty_session_id = %handle.session_id,
                        "Terminal bridge registered and verified"
                    );
                }

                // Register prompt watcher for PTY output prompt detection
                if let Some(ref watcher) = self.prompt_watcher {
                    match self
                        .get_workflow_id_for_terminal(&terminal.workflow_task_id)
                        .await
                    {
                        Ok(Some(workflow_id)) => {
                            if let Err(e) = watcher
                                .register(
                                    &terminal_id,
                                    &workflow_id,
                                    &terminal.workflow_task_id,
                                    &handle.session_id,
                                    terminal.auto_confirm,
                                )
                                .await
                            {
                                if terminal.auto_confirm {
                                    return self
                                        .rollback_launch_after_spawn(
                                            &terminal_id,
                                            format!(
                                                "Prompt watcher registration failed for auto-confirm terminal: {e}"
                                            ),
                                        )
                                        .await;
                                }
                                tracing::warn!(
                                    terminal_id = %terminal_id,
                                    workflow_id = %workflow_id,
                                    auto_confirm = terminal.auto_confirm,
                                    error = %e,
                                    "Failed to register prompt watcher"
                                );
                            } else if !watcher.is_registered(&terminal_id).await {
                                if terminal.auto_confirm {
                                    return self
                                        .rollback_launch_after_spawn(
                                            &terminal_id,
                                            "Prompt watcher registration verification failed for auto-confirm terminal"
                                                .to_string(),
                                        )
                                        .await;
                                }
                                tracing::warn!(
                                    terminal_id = %terminal_id,
                                    workflow_id = %workflow_id,
                                    auto_confirm = terminal.auto_confirm,
                                    "Prompt watcher registration verification failed"
                                );
                            } else {
                                tracing::debug!(
                                    terminal_id = %terminal_id,
                                    workflow_id = %workflow_id,
                                    pty_session_id = %handle.session_id,
                                    auto_confirm = terminal.auto_confirm,
                                    "Prompt watcher registered successfully"
                                );
                            }
                        }
                        Ok(None) => {
                            if terminal.auto_confirm {
                                return self
                                    .rollback_launch_after_spawn(
                                        &terminal_id,
                                        "Could not resolve workflow_id for prompt watcher registration on auto-confirm terminal"
                                            .to_string(),
                                    )
                                    .await;
                            }
                            tracing::warn!(
                                terminal_id = %terminal_id,
                                workflow_task_id = %terminal.workflow_task_id,
                                auto_confirm = terminal.auto_confirm,
                                "Could not resolve workflow_id for prompt watcher registration"
                            );
                        }
                        Err(e) => {
                            if terminal.auto_confirm {
                                return self
                                    .rollback_launch_after_spawn(
                                        &terminal_id,
                                        format!(
                                            "Failed to resolve prompt watcher workflow binding for auto-confirm terminal: {e}"
                                        ),
                                    )
                                    .await;
                            }
                            tracing::warn!(
                                terminal_id = %terminal_id,
                                workflow_task_id = %terminal.workflow_task_id,
                                auto_confirm = terminal.auto_confirm,
                                error = %e,
                                "Failed to resolve prompt watcher workflow binding"
                            );
                        }
                    }
                }

                tracing::info!("Terminal {} started with PID {}", terminal_id, handle.pid);

                LaunchResult {
                    terminal_id,
                    process_handle: Some(handle),
                    success: true,
                    error: None,
                }
            }
            Err(e) => {
                tracing::error!("Failed to start terminal {}: {}", terminal_id, e);
                self.rollback_launch_after_spawn(&terminal_id, format!("Process spawn failed: {e}"))
                    .await
            }
        }
    }

    async fn rollback_launch_after_spawn(&self, terminal_id: &str, reason: String) -> LaunchResult {
        // [G02-002] Log warning when workflow_id resolution fails instead of silently swallowing
        let workflow_id = match self
            .get_workflow_id_for_terminal_by_terminal_id(terminal_id)
            .await
        {
            Ok(wf_id) => {
                if wf_id.is_none() {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        "Could not resolve workflow_id for terminal during rollback (no matching workflow_task)"
                    );
                }
                wf_id
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    error = %e,
                    "Failed to resolve workflow_id for terminal during rollback"
                );
                None
            }
        };
        tracing::error!(
            terminal_id = %terminal_id,
            error = %reason,
            "Terminal launch failed after spawn, performing rollback"
        );

        if let Err(e) = self.process_manager.kill_terminal(terminal_id).await {
            tracing::warn!(terminal_id = %terminal_id, error = %e, "Failed to kill terminal during rollback");
        }
        if let Err(e) = Terminal::update_process(&self.db.pool, terminal_id, None, None).await {
            tracing::warn!(terminal_id = %terminal_id, error = %e, "Failed to clear process binding during rollback");
        }
        if let Err(e) = Terminal::update_session(&self.db.pool, terminal_id, None, None).await {
            tracing::warn!(terminal_id = %terminal_id, error = %e, "Failed to clear session binding during rollback");
        }
        if let Err(e) = Terminal::update_status(&self.db.pool, terminal_id, "failed").await {
            tracing::warn!(terminal_id = %terminal_id, error = %e, "Failed to mark terminal failed during rollback");
        } else {
            self.broadcast_terminal_status(workflow_id.as_deref(), terminal_id, "failed")
                .await;
        }

        LaunchResult {
            terminal_id: terminal_id.to_string(),
            process_handle: None,
            success: false,
            error: Some(reason),
        }
    }

    async fn broadcast_terminal_status(
        &self,
        workflow_id: Option<&str>,
        terminal_id: &str,
        status: &str,
    ) {
        let Some(message_bus) = &self.message_bus else {
            return;
        };
        let Some(workflow_id) = workflow_id else {
            // [G11-005] Log warning when workflow_id is None so status broadcasts
            // are not silently skipped — aids debugging of orphaned terminals.
            tracing::warn!(
                terminal_id = %terminal_id,
                status = %status,
                "Skipping terminal status broadcast: workflow_id is None"
            );
            return;
        };

        let message = BusMessage::TerminalStatusUpdate {
            workflow_id: workflow_id.to_string(),
            terminal_id: terminal_id.to_string(),
            status: status.to_string(),
        };
        let topic = format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}");

        if let Err(e) = message_bus.publish(&topic, message.clone()).await {
            tracing::warn!(
                workflow_id = %workflow_id,
                terminal_id = %terminal_id,
                status = %status,
                error = %e,
                "Failed to publish terminal status update"
            );
        }

        if let Err(e) = message_bus.broadcast(message) {
            tracing::warn!(
                workflow_id = %workflow_id,
                terminal_id = %terminal_id,
                status = %status,
                error = %e,
                "Failed to broadcast terminal status update"
            );
        }
    }

    /// Get CLI command string for a CLI type
    ///
    /// # Arguments
    /// * `cli_name` - The CLI name
    ///
    /// # Returns
    /// The command string to execute
    fn get_cli_command(&self, cli_name: &str) -> String {
        match cli_name {
            "claude-code" => "claude".to_string(),
            "gemini-cli" => "gemini".to_string(),
            "codex" => "codex".to_string(),
            "amp" => "amp".to_string(),
            "cursor-agent" => "cursor".to_string(),
            _ => cli_name.to_string(),
        }
    }

    /// Get workflow ID for a terminal by querying workflow_task table
    async fn get_workflow_id_for_terminal(
        &self,
        workflow_task_id: &str,
    ) -> anyhow::Result<Option<String>> {
        let workflow_id: Option<String> =
            sqlx::query_scalar("SELECT workflow_id FROM workflow_task WHERE id = ?")
                .bind(workflow_task_id)
                .fetch_optional(&self.db.pool)
                .await?
                .flatten();

        Ok(workflow_id)
    }

    async fn get_workflow_id_for_terminal_by_terminal_id(
        &self,
        terminal_id: &str,
    ) -> anyhow::Result<Option<String>> {
        let workflow_id: Option<String> = sqlx::query_scalar(
            r"
            SELECT wt.workflow_id
            FROM workflow_task wt
            INNER JOIN terminal t ON t.workflow_task_id = wt.id
            WHERE t.id = ?
            LIMIT 1
            ",
        )
        .bind(terminal_id)
        .fetch_optional(&self.db.pool)
        .await?
        .flatten();

        Ok(workflow_id)
    }

    /// Get workspace ID for a terminal by traversing workflow_task -> task -> workspace
    async fn get_workspace_for_terminal(
        &self,
        workflow_task_id: &str,
    ) -> anyhow::Result<Option<uuid::Uuid>> {
        // Get vk_task_id from workflow_task - stored as BLOB (UUID bytes)
        let task_id: Option<uuid::Uuid> =
            sqlx::query_scalar("SELECT vk_task_id FROM workflow_task WHERE id = ?")
                .bind(workflow_task_id)
                .fetch_optional(&self.db.pool)
                .await?
                .flatten();

        let Some(task_uuid) = task_id else {
            return Ok(None);
        };

        // Get workspace_id from workspace table - id is also BLOB (UUID bytes)
        let workspace_id: Option<uuid::Uuid> =
            sqlx::query_scalar("SELECT id FROM workspaces WHERE task_id = ? LIMIT 1")
                .bind(task_uuid)
                .fetch_optional(&self.db.pool)
                .await?
                .flatten();

        Ok(workspace_id)
    }

    /// Stop all terminals for a workflow
    ///
    /// # Arguments
    /// * `workflow_id` - The workflow ID
    pub async fn stop_all(&self, workflow_id: &str) -> anyhow::Result<()> {
        let terminals = Terminal::find_by_workflow(&self.db.pool, workflow_id).await?;

        for terminal in terminals {
            let pty_session_id = terminal
                .pty_session_id
                .as_deref()
                .map(str::trim)
                .filter(|session_id| !session_id.is_empty())
                .map(str::to_string);

            if self.process_manager.is_running(&terminal.id).await {
                if let Err(e) = self.process_manager.kill_terminal(&terminal.id).await {
                    tracing::warn!(
                        terminal_id = %terminal.id,
                        workflow_id = %workflow_id,
                        error = %e,
                        "Failed to stop terminal via full cleanup chain"
                    );
                    continue;
                }
            } else if terminal.process_id.is_some() {
                tracing::debug!(
                    terminal_id = %terminal.id,
                    workflow_id = %workflow_id,
                    "Terminal has persisted process_id but no active tracked process"
                );
            }

            if let (Some(bridge), Some(session_id)) =
                (&self.terminal_bridge, pty_session_id.as_deref())
            {
                bridge.unregister(session_id).await;
            }

            if let Some(watcher) = &self.prompt_watcher {
                watcher.unregister(&terminal.id).await;
            }

            if terminal.status != "completed" {
                Terminal::update_process(&self.db.pool, &terminal.id, None, None).await?;
                Terminal::update_session(&self.db.pool, &terminal.id, None, None).await?;
            }

            Terminal::update_status(&self.db.pool, &terminal.id, "cancelled").await?;
            self.broadcast_terminal_status(Some(workflow_id), &terminal.id, "cancelled")
                .await;
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use chrono::Utc;

    use super::*;

    async fn seed_workflow_terminal(
        db: &Arc<DBService>,
        terminal_id: &str,
        status: &str,
        with_process_binding: bool,
    ) -> String {
        let project_id = uuid::Uuid::new_v4();
        sqlx::query("INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)")
            .bind(project_id)
            .bind("test-project")
            .bind(Utc::now())
            .bind(Utc::now())
            .execute(&db.pool)
            .await
            .unwrap();

        let workflow_id = uuid::Uuid::new_v4().to_string();
        sqlx::query(
            "INSERT INTO workflow (id, project_id, name, status, merge_terminal_cli_id, merge_terminal_model_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
        )
        .bind(&workflow_id)
        .bind(project_id)
        .bind("test-workflow")
        .bind("running")
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

        let workflow_task_id = uuid::Uuid::new_v4().to_string();
        sqlx::query(
            "INSERT INTO workflow_task (id, workflow_id, name, branch, order_index, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
        )
        .bind(&workflow_task_id)
        .bind(&workflow_id)
        .bind("task-1")
        .bind("main")
        .bind(0)
        .bind("running")
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

        let process_id = if with_process_binding {
            Some(1234_i32)
        } else {
            None
        };
        let pty_session_id = if with_process_binding {
            Some(uuid::Uuid::new_v4().to_string())
        } else {
            None
        };

        sqlx::query(
            "INSERT INTO terminal (id, workflow_task_id, cli_type_id, model_config_id, order_index, status, auto_confirm, process_id, pty_session_id, session_id, execution_process_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
        )
        .bind(terminal_id)
        .bind(&workflow_task_id)
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(0)
        .bind(status)
        .bind(true)
        .bind(process_id)
        .bind(pty_session_id)
        .bind(Some(uuid::Uuid::new_v4().to_string()))
        .bind(Some(uuid::Uuid::new_v4().to_string()))
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

        workflow_id
    }

    // Test helper - creates launcher with in-memory database
    async fn setup_launcher() -> (TerminalLauncher, Arc<DBService>) {
        use sqlx::{migrate::Migrator, sqlite::SqlitePoolOptions};

        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Get migrations path: from crates/services/ go to ../db/migrations
        let migrations_path: std::path::PathBuf =
            [env!("CARGO_MANIFEST_DIR"), "..", "db", "migrations"]
                .iter()
                .collect();

        // Manually run migrations
        let m = Migrator::new(migrations_path).await.unwrap();
        m.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool });
        let cc_switch = Arc::new(CCSwitchService::new(Arc::clone(&db)));
        let process_manager = Arc::new(ProcessManager::new());
        let working_dir = std::env::temp_dir();

        let launcher =
            TerminalLauncher::new(Arc::clone(&db), cc_switch, process_manager, working_dir);

        (launcher, db)
    }

    #[tokio::test]
    async fn test_launcher_new() {
        let (launcher, _) = setup_launcher().await;
        assert!(!launcher.working_dir.to_str().unwrap().is_empty());
    }

    #[tokio::test]
    async fn test_get_cli_command_claude() {
        let (launcher, _) = setup_launcher().await;
        let cmd = launcher.get_cli_command("claude-code");
        assert_eq!(cmd, "claude");
    }

    #[tokio::test]
    async fn test_get_cli_command_gemini() {
        let (launcher, _) = setup_launcher().await;
        let cmd = launcher.get_cli_command("gemini-cli");
        assert_eq!(cmd, "gemini");
    }

    #[tokio::test]
    async fn test_launch_terminal_missing_cli_type() {
        let (launcher, db) = setup_launcher().await;

        // Create a workflow
        let wf_id = uuid::Uuid::new_v4().to_string();
        let _ = sqlx::query(
            "INSERT INTO workflow (id, name, base_dir, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"
        )
        .bind(&wf_id)
        .bind("test-wf")
        .bind("/tmp")
        .bind("created")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&db.pool)
        .await;

        // Create a terminal with non-existent CLI type
        let terminal = Terminal {
            id: "test-term".to_string(),
            workflow_task_id: wf_id.clone(),
            cli_type_id: "non-existent-cli".to_string(),
            model_config_id: uuid::Uuid::new_v4().to_string(),
            custom_base_url: None,
            custom_api_key: None,
            role: None,
            role_description: None,
            order_index: 0,
            status: "not_started".to_string(),
            process_id: None,
            pty_session_id: None,
            session_id: None,
            execution_process_id: None,
            vk_session_id: None,
            auto_confirm: false,
            last_commit_hash: None,
            last_commit_message: None,
            started_at: None,
            completed_at: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
        };

        let result = launcher.launch_terminal(&terminal).await;
        assert!(!result.success);
        assert!(result.error.is_some());
        assert_eq!(result.terminal_id, "test-term");
    }

    #[tokio::test]
    async fn test_launch_result_structure() {
        let result = LaunchResult {
            terminal_id: "test".to_string(),
            process_handle: None,
            success: true,
            error: None,
        };

        assert_eq!(result.terminal_id, "test");
        assert!(result.success);
        assert!(result.process_handle.is_none());
        assert!(result.error.is_none());
    }

    #[tokio::test]
    async fn test_stop_all_clears_terminal_bindings_when_not_completed() {
        let (launcher, db) = setup_launcher().await;
        let workflow_id =
            seed_workflow_terminal(&db, "stop-all-terminal-1", "waiting", false).await;

        launcher.stop_all(&workflow_id).await.unwrap();

        let terminal = Terminal::find_by_id(&db.pool, "stop-all-terminal-1")
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "cancelled");
        assert!(terminal.process_id.is_none());
        assert!(terminal.pty_session_id.is_none());
        assert!(terminal.session_id.is_none());
        assert!(terminal.execution_process_id.is_none());
    }

    #[tokio::test]
    async fn test_stop_all_keeps_completed_terminal_binding() {
        let (launcher, db) = setup_launcher().await;
        let workflow_id =
            seed_workflow_terminal(&db, "stop-all-terminal-2", "completed", false).await;

        let before = Terminal::find_by_id(&db.pool, "stop-all-terminal-2")
            .await
            .unwrap()
            .unwrap();

        launcher.stop_all(&workflow_id).await.unwrap();

        let after = Terminal::find_by_id(&db.pool, "stop-all-terminal-2")
            .await
            .unwrap()
            .unwrap();

        assert_eq!(after.status, "cancelled");
        assert_eq!(after.session_id, before.session_id);
        assert_eq!(after.execution_process_id, before.execution_process_id);
    }

    #[tokio::test]
    async fn test_stop_all_kills_running_terminal_and_clears_tracking() {
        let (launcher, db) = setup_launcher().await;
        let terminal_id = "stop-all-running-terminal-1";
        let workflow_id = seed_workflow_terminal(&db, terminal_id, "waiting", false).await;

        let temp_dir = tempfile::tempdir().unwrap();
        #[cfg(windows)]
        let spawn_config = SpawnCommand::new("powershell", temp_dir.path()).with_args([
            "-NoLogo",
            "-NoProfile",
            "-Command",
            "Start-Sleep -Seconds 120",
        ]);
        #[cfg(unix)]
        let spawn_config = SpawnCommand::new("sleep", temp_dir.path()).with_arg("120");

        tokio::time::timeout(
            std::time::Duration::from_secs(10),
            launcher.process_manager.spawn_pty_with_config(
                terminal_id,
                &spawn_config,
                DEFAULT_COLS,
                DEFAULT_ROWS,
            ),
        )
        .await
        .expect("spawn_pty_with_config should not hang")
        .unwrap();

        assert!(launcher.process_manager.is_running(terminal_id).await);

        tokio::time::timeout(
            std::time::Duration::from_secs(10),
            launcher.stop_all(&workflow_id),
        )
        .await
        .expect("stop_all should not hang")
        .unwrap();

        assert!(
            !launcher.process_manager.is_running(terminal_id).await,
            "stop_all should remove running process tracking via kill_terminal"
        );

        let terminal = Terminal::find_by_id(&db.pool, terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "cancelled");
        assert!(terminal.process_id.is_none());
        assert!(terminal.pty_session_id.is_none());
        assert!(terminal.session_id.is_none());
        assert!(terminal.execution_process_id.is_none());
    }
}
