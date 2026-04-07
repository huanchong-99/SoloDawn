//! Orchestrator Runtime Service
//!
//! Manages multiple OrchestratorAgent instances, one per active workflow.

use std::{
    collections::{HashMap, HashSet},
    path::PathBuf,
    sync::Arc,
};

use anyhow::{Result, anyhow};
use db::models::WorkflowOrchestratorCommand;
use db::DBService;
use sqlx::Row;
use tokio::{
    sync::{Mutex, RwLock},
    task::JoinHandle,
    time::{Duration, sleep, timeout},
};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use super::{
    OrchestratorAgent, OrchestratorConfig, SharedMessageBus,
    constants::{WORKFLOW_STATUS_FAILED, WORKFLOW_STATUS_PAUSED, WORKFLOW_STATUS_READY},
    persistence::StatePersistence,
    runtime_actions::RuntimeActionService,
    types::LLMMessage,
};
use crate::services::concierge::ConciergeBroadcaster;
use crate::services::git_watcher::{GitWatcher, GitWatcherConfig};

/// Configuration for the OrchestratorRuntime
#[derive(Debug, Clone)]
pub struct RuntimeConfig {
    /// Maximum number of concurrent workflows
    pub max_concurrent_workflows: usize,
    /// Message bus channel capacity
    pub message_bus_capacity: usize,
    /// Git watcher polling interval in milliseconds
    pub git_watch_poll_interval_ms: u64,
}

impl Default for RuntimeConfig {
    fn default() -> Self {
        Self {
            max_concurrent_workflows: 10,
            message_bus_capacity: 1000,
            git_watch_poll_interval_ms: 2000,
        }
    }
}

/// Execution status of a direct orchestrator chat command.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OrchestratorChatCommandStatus {
    Queued,
    Running,
    Succeeded,
    Failed,
    Cancelled,
}

impl OrchestratorChatCommandStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Queued => "queued",
            Self::Running => "running",
            Self::Succeeded => "succeeded",
            Self::Failed => "failed",
            Self::Cancelled => "cancelled",
        }
    }
}

/// Result snapshot of one direct orchestrator chat command.
#[derive(Debug, Clone)]
pub struct OrchestratorChatCommandResult {
    pub command_id: String,
    pub status: OrchestratorChatCommandStatus,
    pub error: Option<String>,
}

/// Workflow agent with its task handle
struct RunningWorkflow {
    agent: Arc<OrchestratorAgent>,
    task_handle: JoinHandle<()>,
}

/// Git watcher with its task handle for lifecycle management
struct GitWatcherHandle {
    watcher: Arc<GitWatcher>,
    task_handle: JoinHandle<()>,
}

/// Orchestrator Runtime Service
///
/// Manages the lifecycle of orchestrator agents for multiple workflows.
#[derive(Clone)]
pub struct OrchestratorRuntime {
    db: Arc<DBService>,
    message_bus: SharedMessageBus,
    config: RuntimeConfig,
    running_workflows: Arc<Mutex<HashMap<String, RunningWorkflow>>>,
    /// Workflows currently in start pipeline, keyed by workflow_id
    starting_workflows: Arc<Mutex<HashSet<String>>>,
    /// Git watchers for each workflow, keyed by workflow_id
    git_watchers: Arc<Mutex<HashMap<String, GitWatcherHandle>>>,
    /// Idempotency snapshots for orchestrator chat messages, keyed by workflow_id.
    orchestrator_chat_idempotency:
        Arc<Mutex<HashMap<String, HashMap<String, OrchestratorChatCommandResult>>>>,
    persistence: StatePersistence,
    runtime_actions: Arc<RwLock<Option<Arc<RuntimeActionService>>>>,
    concierge_broadcaster: Arc<RwLock<Option<Arc<ConciergeBroadcaster>>>>,
}

impl OrchestratorRuntime {
    /// Create a new runtime instance
    pub fn new(db: Arc<DBService>, message_bus: SharedMessageBus) -> Self {
        let persistence = StatePersistence::new(db.clone());

        Self {
            db,
            message_bus,
            config: RuntimeConfig::default(),
            running_workflows: Arc::new(Mutex::new(HashMap::new())),
            starting_workflows: Arc::new(Mutex::new(HashSet::new())),
            git_watchers: Arc::new(Mutex::new(HashMap::new())),
            orchestrator_chat_idempotency: Arc::new(Mutex::new(HashMap::new())),
            persistence,
            runtime_actions: Arc::new(RwLock::new(None)),
            concierge_broadcaster: Arc::new(RwLock::new(None)),
        }
    }

    /// Create a new runtime with custom config
    pub fn with_config(
        db: Arc<DBService>,
        message_bus: SharedMessageBus,
        config: RuntimeConfig,
    ) -> Self {
        let persistence = StatePersistence::new(db.clone());

        Self {
            db,
            message_bus,
            config,
            running_workflows: Arc::new(Mutex::new(HashMap::new())),
            starting_workflows: Arc::new(Mutex::new(HashSet::new())),
            git_watchers: Arc::new(Mutex::new(HashMap::new())),
            orchestrator_chat_idempotency: Arc::new(Mutex::new(HashMap::new())),
            persistence,
            runtime_actions: Arc::new(RwLock::new(None)),
            concierge_broadcaster: Arc::new(RwLock::new(None)),
        }
    }

    /// Attach a concierge broadcaster for terminal bridge messages.
    pub async fn set_concierge_broadcaster(&self, broadcaster: Arc<ConciergeBroadcaster>) {
        *self.concierge_broadcaster.write().await = Some(broadcaster);
    }

    pub async fn set_runtime_actions(&self, runtime_actions: Arc<RuntimeActionService>) {
        *self.runtime_actions.write().await = Some(runtime_actions);
    }

    /// Try to start a GitWatcher for the workflow.
    ///
    /// Returns None if:
    /// - Project not found
    /// - Project has no usable repo path
    /// - Path is not a valid git repository
    async fn try_start_git_watcher(
        &self,
        workflow_id: &str,
        workflow: &db::models::Workflow,
    ) -> Result<Option<GitWatcherHandle>> {
        // Check workflow-level git watcher toggle
        if !workflow.git_watcher_enabled {
            info!(
                "Git watcher disabled for workflow {} (git_watcher_enabled=false)",
                workflow_id
            );
            return Ok(None);
        }

        // Get project to find repo path
        let project =
            if let Some(project) = db::models::project::Project::find_by_id(&self.db.pool, workflow.project_id)
                .await? { project } else {
                warn!(
                    "Project {} not found for workflow {}, git watcher disabled",
                    workflow.project_id, workflow_id
                );
                return Ok(None);
            };

        // Resolve repo path: prefer project.default_agent_working_dir, then fallback to project repos
        let repo_path = match project.default_agent_working_dir.clone() {
            Some(path) if !path.trim().is_empty() => Some(path),
            _ => db::models::project_repo::ProjectRepo::find_repos_for_project(
                &self.db.pool,
                project.id,
            )
            .await?
            .into_iter()
            .map(|repo| repo.path.to_string_lossy().into_owned())
            .find(|path| !path.trim().is_empty()),
        };

        let Some(repo_path) = repo_path else {
            warn!(
                "Project {} has no usable repo path (default_agent_working_dir/project_repos); git watcher disabled for workflow {}",
                project.id, workflow_id
            );
            return Ok(None);
        };

        // Create GitWatcher config
        let config = GitWatcherConfig::new(
            PathBuf::from(&repo_path),
            self.config.git_watch_poll_interval_ms,
        );

        // Create GitWatcher
        let mut watcher = match GitWatcher::new(config, self.message_bus.as_ref().clone()) {
            Ok(watcher) => watcher,
            Err(e) => {
                warn!(
                    "Failed to create GitWatcher for workflow {} (repo {}): {}",
                    workflow_id, repo_path, e
                );
                return Ok(None);
            }
        };

        // Associate watcher with workflow
        watcher.set_workflow_id(workflow_id.to_string());

        let watcher = Arc::new(watcher);
        let watcher_clone = watcher.clone();
        let workflow_id_owned = workflow_id.to_string();

        // Spawn watcher task
        let task_handle = tokio::spawn(async move {
            if let Err(e) = watcher_clone.watch().await {
                error!(
                    "GitWatcher failed for workflow {}: {}",
                    workflow_id_owned, e
                );
            }
        });

        info!(
            "GitWatcher started for workflow {} (repo: {})",
            workflow_id, repo_path
        );

        Ok(Some(GitWatcherHandle {
            watcher,
            task_handle,
        }))
    }

    /// Stop the GitWatcher for a workflow if running.
    async fn stop_git_watcher(&self, workflow_id: &str) {
        let git_watcher_handle = {
            let mut watchers = self.git_watchers.lock().await;
            watchers.remove(workflow_id)
        };

        if let Some(handle) = git_watcher_handle {
            // Signal watcher to stop
            handle.watcher.stop();
            info!("GitWatcher stop requested for workflow {}", workflow_id);

            // Wait for graceful shutdown with timeout
            let mut task_handle = handle.task_handle;
            let shutdown_result = timeout(Duration::from_secs(5), &mut task_handle).await;

            match shutdown_result {
                Ok(Ok(())) => {
                    info!(
                        "GitWatcher for workflow {} shutdown gracefully",
                        workflow_id
                    );
                }
                Ok(Err(e)) => {
                    warn!(
                        "GitWatcher task failed for workflow {}: {:?}",
                        workflow_id, e
                    );
                }
                Err(_) => {
                    warn!(
                        "GitWatcher shutdown timeout for workflow {}, aborting",
                        workflow_id
                    );
                    task_handle.abort();
                    task_handle.await.ok();
                }
            }
        }
    }

    /// Reserve a start slot for a workflow to avoid concurrent duplicate starts.
    async fn reserve_start_slot(&self, workflow_id: &str) -> Result<()> {
        let running = self.running_workflows.lock().await;
        let mut starting = self.starting_workflows.lock().await;

        if running.len() + starting.len() >= self.config.max_concurrent_workflows {
            return Err(anyhow!(
                "Maximum concurrent workflows limit reached: {}",
                self.config.max_concurrent_workflows
            ));
        }

        if running.contains_key(workflow_id) || starting.contains(workflow_id) {
            return Err(anyhow!("Workflow {workflow_id} is already running"));
        }

        starting.insert(workflow_id.to_string());
        Ok(())
    }

    /// Release start slot after start attempt finishes.
    async fn release_start_slot(&self, workflow_id: &str) {
        let mut starting = self.starting_workflows.lock().await;
        starting.remove(workflow_id);
    }

    /// Start workflow after start slot has been reserved.
    async fn start_workflow_reserved(&self, workflow_id: &str) -> Result<()> {
        // Load workflow from database
        let workflow = db::models::Workflow::find_by_id(&self.db.pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))?;

        // Verify workflow is in ready state
        if workflow.status != WORKFLOW_STATUS_READY {
            return Err(anyhow!(
                "Workflow {} is not ready. Current status: {}",
                workflow_id,
                workflow.status
            ));
        }

        // Build orchestrator config from workflow settings.
        // When API key is missing, fall through with a partial config —
        // OrchestratorAgent::new() will try Claude Code native credentials.
        let orchestrator_config = if workflow.orchestrator_enabled {
            let api_key = workflow.get_api_key().ok().flatten().unwrap_or_default();
            OrchestratorConfig::from_workflow(
                workflow.orchestrator_api_type.as_deref(),
                workflow.orchestrator_base_url.as_deref(),
                if api_key.is_empty() { None } else { Some(&api_key) },
                workflow.orchestrator_model.as_deref(),
            )
        } else {
            None
        };

        // Create orchestrator agent FIRST before changing status
        let config = orchestrator_config.unwrap_or_default();
        let mut agent = match OrchestratorAgent::new(
            config,
            workflow_id.to_string(),
            self.message_bus.clone(),
            self.db.clone(),
        ) {
            Ok(agent) => agent,
            Err(e) => {
                // Agent creation failed, workflow stays in ready state
                error!(
                    "Failed to create orchestrator agent for workflow {}: {}",
                    workflow_id, e
                );
                return Err(e.context("Failed to create orchestrator agent"));
            }
        };
        if let Some(runtime_actions) = self.runtime_actions.read().await.clone() {
            agent.attach_runtime_actions(runtime_actions);
        }
        agent.attach_persistence(Arc::new(StatePersistence::new(self.db.clone())));
        if let Some(ref broadcaster) = *self.concierge_broadcaster.read().await {
            agent.attach_concierge_broadcaster(broadcaster.clone());
        }
        let agent = Arc::new(agent);

        // Update workflow status to running AFTER agent is successfully created
        db::models::Workflow::set_started(&self.db.pool, workflow_id).await?;
        info!("Workflow {} marked as started", workflow_id);

        // Spawn agent task with error handling
        let agent_clone = agent.clone();
        let running_workflows = self.running_workflows.clone();
        let git_watchers = self.git_watchers.clone();
        let chat_idempotency = self.orchestrator_chat_idempotency.clone();
        let workflow_id_owned = workflow_id.to_string();
        let task_handle = tokio::spawn(async move {
            if let Err(e) = Box::pin(agent_clone.run()).await {
                error!(
                    "Orchestrator agent failed for workflow {}: {}",
                    workflow_id_owned, e
                );
            }

            // Best-effort cleanup for naturally completed workflow runs.
            // Guard against removing a newly restarted workflow with the same ID.
            let mut removed_running = false;
            for _ in 0..5 {
                let mut running = running_workflows.lock().await;
                let can_remove = running
                    .get(&workflow_id_owned)
                    .is_some_and(|entry| entry.task_handle.is_finished());

                if can_remove {
                    running.remove(&workflow_id_owned);
                    removed_running = true;
                    break;
                }

                drop(running);
                sleep(Duration::from_millis(100)).await;
            }

            if removed_running {
                {
                    let mut idempotency = chat_idempotency.lock().await;
                    idempotency.remove(&workflow_id_owned);
                }

                let git_watcher_handle = {
                    let mut watchers = git_watchers.lock().await;
                    watchers.remove(&workflow_id_owned)
                };

                if let Some(handle) = git_watcher_handle {
                    handle.watcher.stop();
                    let mut watcher_task = handle.task_handle;
                    let shutdown_result = timeout(Duration::from_secs(5), &mut watcher_task).await;
                    if shutdown_result.is_err() {
                        watcher_task.abort();
                        watcher_task.await.ok();
                    }
                }
            }
        });

        // Insert into running workflows map immediately to prevent race condition
        let mut running = self.running_workflows.lock().await;
        running.insert(
            workflow_id.to_string(),
            RunningWorkflow { agent, task_handle },
        );
        drop(running); // Release lock before logging

        // Start GitWatcher for this workflow (non-blocking, failure is not fatal)
        match self.try_start_git_watcher(workflow_id, &workflow).await {
            Ok(Some(handle)) => {
                let mut watchers = self.git_watchers.lock().await;
                watchers.insert(workflow_id.to_string(), handle);
            }
            Ok(None) => {
                // GitWatcher not started (no repo path or invalid repo)
                debug!(
                    "GitWatcher not started for workflow {} (no valid repo)",
                    workflow_id
                );
            }
            Err(e) => {
                warn!(
                    "Failed to start GitWatcher for workflow {}: {}",
                    workflow_id, e
                );
            }
        }

        info!("Workflow {} started successfully", workflow_id);
        let running = self.running_workflows.lock().await;
        debug!("Total running workflows: {}", running.len());

        Ok(())
    }

    /// Start orchestrating a workflow
    ///
    /// Creates and starts an OrchestratorAgent for the given workflow.
    /// Returns an error if the workflow is already running or if the
    /// max_concurrent_workflows limit is reached.
    ///
    /// # Slot safety (G03-001)
    ///
    /// `reserve_start_slot` / `release_start_slot` form a logical RAII pair.
    /// A traditional `scopeguard` cannot be used here because the release
    /// operation is `async` and Rust does not support async `Drop`.
    ///
    /// Panic safety: in the tokio runtime, a panic inside
    /// `start_workflow_reserved` will unwind the **task**, not the process.
    /// The slot remains in `starting_workflows` but the workflow never enters
    /// `running_workflows`, so the only effect is one wasted slot until the
    /// runtime is restarted.  This is acceptable because:
    ///   1. Panics in this path indicate a logic bug that warrants restart.
    ///   2. `max_concurrent_workflows` is a soft limit; one leaked slot does
    ///      not block existing workflows.
    ///
    /// If stricter guarantees are needed in the future, wrap the call in
    /// `tokio::task::spawn` + `catch_unwind` and release the slot on
    /// `JoinError::is_panic()`.
    pub async fn start_workflow(&self, workflow_id: &str) -> Result<()> {
        self.reserve_start_slot(workflow_id).await?;
        let start_result = self.start_workflow_reserved(workflow_id).await;
        self.release_start_slot(workflow_id).await;
        start_result
    }

    /// Submit user response for an interactive prompt in a running workflow.
    ///
    /// Looks up the running agent by workflow_id and forwards the response.
    /// The runtime lock is released before awaiting agent handling.
    /// Returns an error when the workflow is not running or the terminal is not awaiting approval.
    pub async fn submit_user_prompt_response(
        &self,
        workflow_id: &str,
        terminal_id: &str,
        user_response: &str,
    ) -> Result<()> {
        let agent = {
            let running = self.running_workflows.lock().await;
            let running_workflow = running
                .get(workflow_id)
                .ok_or_else(|| anyhow!("Workflow {workflow_id} is not running"))?;

            Arc::clone(&running_workflow.agent)
        };

        agent
            .handle_user_prompt_response(terminal_id, user_response)
            .await
            .map_err(|e| {
                anyhow!(
                    "Failed to submit user prompt response for workflow {workflow_id} and terminal {terminal_id}: {e}"
                )
            })
    }

    /// Submit a direct chat message to the orchestrator agent of a running workflow.
    pub async fn submit_orchestrator_chat(
        &self,
        workflow_id: &str,
        message: &str,
        source: &str,
        external_message_id: Option<&str>,
    ) -> Result<OrchestratorChatCommandResult> {
        self.submit_orchestrator_chat_with_command_id(
            workflow_id,
            message,
            source,
            external_message_id,
            None,
        )
        .await
    }

    /// Submit a direct chat message with an optional caller-provided command id.
    pub async fn submit_orchestrator_chat_with_command_id(
        &self,
        workflow_id: &str,
        message: &str,
        source: &str,
        external_message_id: Option<&str>,
        command_id: Option<String>,
    ) -> Result<OrchestratorChatCommandResult> {
        let dedup_key = external_message_id.map(|value| format!("{source}:{value}"));
        if let Some(key) = dedup_key.as_ref() {
            let existing = {
                let idempotency = self.orchestrator_chat_idempotency.lock().await;
                idempotency
                    .get(workflow_id)
                    .and_then(|entry| entry.get(key))
                    .cloned()
            };
            if let Some(existing) = existing {
                info!(
                    workflow_id = %workflow_id,
                    source = %source,
                    external_message_id = %external_message_id.unwrap_or(""),
                    command_id = %existing.command_id,
                    "Ignoring duplicate orchestrator chat message and returning original command snapshot"
                );
                return Ok(existing);
            }
        }

        let mut command_result = OrchestratorChatCommandResult {
            command_id: command_id.unwrap_or_else(|| Uuid::new_v4().to_string()),
            status: OrchestratorChatCommandStatus::Queued,
            error: None,
        };

        let agent = {
            let running = self.running_workflows.lock().await;
            let running_workflow = running
                .get(workflow_id)
                .ok_or_else(|| anyhow!("Workflow {workflow_id} is not running"))?;

            Arc::clone(&running_workflow.agent)
        };

        command_result.status = OrchestratorChatCommandStatus::Running;

        match agent.submit_orchestrator_chat_message(message).await {
            Ok(()) => {
                command_result.status = OrchestratorChatCommandStatus::Succeeded;
            }
            Err(error) => {
                command_result.status = OrchestratorChatCommandStatus::Failed;
                command_result.error = Some(format!(
                    "Failed to submit orchestrator chat message for workflow {workflow_id}: {error}"
                ));
            }
        }

        if let Some(key) = dedup_key {
            let mut idempotency = self.orchestrator_chat_idempotency.lock().await;
            let entry = idempotency.entry(workflow_id.to_string()).or_default();
            entry.insert(key, command_result.clone());
            if entry.len() > 2048 {
                // LRU-style eviction: remove oldest half instead of clearing all entries
                let keys_to_remove: Vec<String> =
                    entry.keys().take(entry.len() / 2).cloned().collect();
                for k in &keys_to_remove {
                    entry.remove(k);
                }
                tracing::debug!(
                    workflow_id = %workflow_id,
                    evicted = keys_to_remove.len(),
                    remaining = entry.len(),
                    "Evicted oldest half of chat idempotency entries"
                );
            }
        }

        Ok(command_result)
    }

    /// Fetch orchestrator conversation history for a running workflow.
    pub async fn get_orchestrator_messages(&self, workflow_id: &str) -> Result<Vec<LLMMessage>> {
        let agent = {
            let running = self.running_workflows.lock().await;
            let running_workflow = running
                .get(workflow_id)
                .ok_or_else(|| anyhow!("Workflow {workflow_id} is not running"))?;

            Arc::clone(&running_workflow.agent)
        };

        Ok(agent.get_conversation_history().await)
    }

    /// Stop orchestrating a workflow
    ///
    /// Sends shutdown signal to the agent and waits for graceful shutdown.
    /// If shutdown doesn't complete within timeout, the task is aborted.
    ///
    /// # Shutdown ordering (G05-003 / G05-006)
    ///
    /// The `Shutdown` message is published to the message bus *before* the
    /// workflow is removed from `running_workflows`.  This ensures the agent
    /// task receives the shutdown signal and can begin draining its event
    /// loop before the runtime discards its handle.
    ///
    /// After publishing, the workflow is removed so that new API requests
    /// (e.g. `submit_orchestrator_chat`) immediately get "not running"
    /// errors, preventing new work from being enqueued during teardown.
    /// Any in-flight events already in the message bus channel will still
    /// be delivered to the agent's event loop, but the agent breaks out of
    /// its loop upon receiving `BusMessage::Shutdown`, so those trailing
    /// events are harmlessly discarded.
    pub async fn stop_workflow(&self, workflow_id: &str) -> Result<()> {
        // Stop GitWatcher first (non-blocking)
        self.stop_git_watcher(workflow_id).await;

        {
            let mut idempotency = self.orchestrator_chat_idempotency.lock().await;
            idempotency.remove(workflow_id);
        }

        // G05-003: Publish Shutdown BEFORE removing from running_workflows so
        // the agent task is guaranteed to receive the signal before we
        // discard the task handle.  The workflow topic is derived from the
        // workflow_id, so the agent's bus subscription will pick it up.
        if self.running_workflows.lock().await.contains_key(workflow_id) {
            self.message_bus
                .publish(
                    &format!("workflow:{workflow_id}"),
                    super::BusMessage::Shutdown,
                )
                .await?;
            info!("Shutdown signal sent for workflow {}", workflow_id);
        }

        // Remove from running workflows AFTER the shutdown signal has been published
        let running_workflow = {
            let mut running = self.running_workflows.lock().await;
            running
                .remove(workflow_id)
                .ok_or_else(|| anyhow!("Workflow {workflow_id} is not running"))?
        };

        // Wait for graceful shutdown.
        //
        // Timeout rationale (G05-003): 15 seconds accommodates the worst
        // case where the agent is mid-LLM call.  Most LLM providers have
        // their own request timeout (typically 30-60s), but the agent
        // checks for shutdown between iterations, so 15s is sufficient for
        // the current event-loop design while giving more headroom than the
        // previous 10s value.
        let mut task_handle = running_workflow.task_handle;
        let shutdown_result = timeout(Duration::from_secs(15), &mut task_handle).await;

        match shutdown_result {
            Ok(Ok(())) => {
                info!("Workflow {} shutdown gracefully", workflow_id);
            }
            Ok(Err(e)) => {
                warn!("Workflow {} task failed: {:?}", workflow_id, e);
            }
            Err(_) => {
                warn!("Workflow {} shutdown timeout, aborting", workflow_id);
                task_handle.abort();
                task_handle.await.ok(); // Await the abort
            }
        }

        info!("Workflow {} stopped successfully", workflow_id);

        let running = self.running_workflows.lock().await;
        debug!("Total running workflows: {}", running.len());

        Ok(())
    }

    /// Resume a paused workflow (G05-002)
    ///
    /// Atomically transitions the workflow from `paused` to `ready` via CAS,
    /// then delegates to `start_workflow` to create a fresh orchestrator agent.
    ///
    /// Returns `Err` if the workflow is not found, is not in `paused` state,
    /// or if the CAS update races with a concurrent status change.
    pub async fn resume_workflow(&self, workflow_id: &str) -> Result<()> {
        let pool = &self.db.pool;

        // Load workflow and verify it is paused
        let workflow = db::models::Workflow::find_by_id(pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found"))?;

        if workflow.status != WORKFLOW_STATUS_PAUSED {
            return Err(anyhow!(
                "Cannot resume workflow {workflow_id}: expected status '{}', got '{}'",
                WORKFLOW_STATUS_PAUSED,
                workflow.status
            ));
        }

        // CAS: paused → ready
        let now = chrono::Utc::now();
        let result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'ready', updated_at = ?
            WHERE id = ? AND status = 'paused'
            ",
        )
        .bind(now)
        .bind(workflow_id)
        .execute(pool)
        .await
        .map_err(|e| anyhow!("Failed to update workflow status during resume: {e}"))?;

        if result.rows_affected() == 0 {
            return Err(anyhow!(
                "Cannot resume workflow {workflow_id}: status changed concurrently"
            ));
        }

        info!(
            workflow_id = %workflow_id,
            "Workflow transitioned paused → ready for resume"
        );

        // Delegate to start_workflow which handles slot reservation and agent creation
        self.start_workflow(workflow_id).await
    }

    /// Check if a workflow is currently running
    pub async fn is_running(&self, workflow_id: &str) -> bool {
        let running = self.running_workflows.lock().await;
        running.contains_key(workflow_id)
    }

    /// Get live provider status for a running workflow.
    ///
    /// Returns `None` if the workflow is not currently running.
    pub async fn get_provider_status(
        &self,
        workflow_id: &str,
    ) -> Option<Vec<super::resilient_llm::ProviderStatusReport>> {
        let agent = {
            let running = self.running_workflows.lock().await;
            running
                .get(workflow_id)
                .map(|rw| Arc::clone(&rw.agent))
        };

        match agent {
            Some(agent) => Some(agent.get_provider_status().await),
            None => None,
        }
    }

    /// Reset a provider's circuit breaker for a running workflow.
    ///
    /// Returns `Ok(true)` if the provider was found and reset,
    /// `Ok(false)` if the provider name was not found,
    /// `Err` if the workflow is not running.
    pub async fn reset_provider(
        &self,
        workflow_id: &str,
        provider_name: &str,
    ) -> Result<bool> {
        let agent = {
            let running = self.running_workflows.lock().await;
            running
                .get(workflow_id)
                .map(|rw| Arc::clone(&rw.agent))
                .ok_or_else(|| anyhow!("Workflow {workflow_id} is not running"))?
        };

        Ok(agent.reset_provider(provider_name).await)
    }

    /// Get the count of running workflows
    pub async fn running_count(&self) -> usize {
        let running = self.running_workflows.lock().await;
        running.len()
    }

    /// Stop all running workflows
    pub async fn stop_all(&self) -> Result<()> {
        let workflow_ids: Vec<String> = {
            let running = self.running_workflows.lock().await;
            running.keys().cloned().collect()
        };

        info!("Stopping {} running workflows", workflow_ids.len());

        for workflow_id in workflow_ids {
            if let Err(e) = self.stop_workflow(&workflow_id).await {
                warn!("Failed to stop workflow {}: {}", workflow_id, e);
            }
        }

        info!("All workflows stopped");

        Ok(())
    }

    /// Recover workflows that were running at startup
    ///
    /// Finds all workflows with status 'running' and attempts to resume them
    /// from persisted state. Workflows without persisted state or whose resume
    /// fails are marked as 'failed'.
    /// Returns the number of interrupted workflows discovered.
    /// Should be called on service startup.
    ///
    /// # Recovery strategy limitations (G05-004)
    ///
    /// The current strategy is conservative: any workflow that cannot be
    /// fully resumed (missing persisted state, invalid config, agent
    /// creation failure) is marked as `failed`.  This means:
    ///
    ///   - Workflows that had active terminals with uncommitted work will
    ///     lose that in-progress work.  The terminals' PTY processes are
    ///     gone after restart, so there is no way to reconnect.
    ///
    ///   - Workflows with persisted state may still fail to resume if the
    ///     API key has been rotated or the LLM provider is unreachable at
    ///     recovery time.
    ///
    /// Future improvements could include:
    ///   - A `suspended` status that preserves the workflow for manual
    ///     retry instead of marking it `failed` immediately.
    ///   - Partial recovery: restart only the orchestrator agent and let
    ///     it re-evaluate which terminals need re-launching.
    ///   - Exponential back-off retry for transient provider failures
    ///     during recovery.
    pub async fn recover_running_workflows(&self) -> Result<usize> {
        let pool = &self.db.pool;

        let rows = sqlx::query(
            r"
            SELECT id
            FROM workflow
            WHERE status = 'running'
            ",
        )
        .fetch_all(pool)
        .await?;

        if rows.is_empty() {
            info!("No running workflows to recover");
            return Ok(0);
        }

        let discovered_count = rows.len();
        warn!("Found {} running workflows to recover", discovered_count);

        for row in rows {
            let workflow_id: String = row.get("id");
            warn!("Recovering workflow {}", workflow_id);

            match self.try_resume_workflow(&workflow_id).await {
                Ok(true) => {
                    info!(
                        workflow_id = %workflow_id,
                        "Workflow resumed from persisted state"
                    );
                }
                Ok(false) => {
                    // G05-004: Check task progress before giving up. If the workflow
                    // has completed tasks, it made meaningful progress before the
                    // restart. In that case, mark it as `paused` so the user can
                    // inspect results and optionally resume, rather than destroying
                    // all recorded progress by marking it `failed`.
                    let has_completed_tasks = match db::models::WorkflowTask::find_by_workflow(pool, &workflow_id).await {
                        Ok(tasks) => tasks.iter().any(|t| t.status == "completed"),
                        Err(e) => {
                            warn!(
                                workflow_id = %workflow_id,
                                error = %e,
                                "Failed to check task progress during recovery; defaulting to failed"
                            );
                            false
                        }
                    };

                    let recovery_status = if has_completed_tasks {
                        warn!(
                            workflow_id = %workflow_id,
                            "No persisted state found but workflow has completed tasks; marking as paused for manual resume"
                        );
                        WORKFLOW_STATUS_PAUSED
                    } else {
                        warn!(
                            workflow_id = %workflow_id,
                            "No persisted state found and no completed tasks, marking as failed"
                        );
                        WORKFLOW_STATUS_FAILED
                    };

                    if let Err(e) =
                        db::models::Workflow::update_status(pool, &workflow_id, recovery_status)
                            .await
                    {
                        error!(
                            "Failed to mark workflow {} as {} during recovery: {}",
                            workflow_id, recovery_status, e
                        );
                    }
                }
                Err(e) => {
                    error!(
                        workflow_id = %workflow_id,
                        error = %e,
                        "Recovery failed, marking as failed"
                    );
                    if let Err(e) =
                        db::models::Workflow::update_status(pool, &workflow_id, WORKFLOW_STATUS_FAILED)
                            .await
                    {
                        error!(
                            "Failed to mark workflow {} as failed during recovery: {}",
                            workflow_id, e
                        );
                    }
                }
            }
        }

        Ok(discovered_count)
    }

    /// Attempt to resume a single workflow from persisted state.
    ///
    /// Returns `Ok(true)` if the workflow was successfully resumed,
    /// `Ok(false)` if no persisted state was available, or `Err` on failure.
    async fn try_resume_workflow(&self, workflow_id: &str) -> Result<bool> {
        // Load persisted state; return false if none exists
        let recovered_state = match self.persistence.recover_workflow(workflow_id).await? {
            Some(state) => state,
            None => return Ok(false),
        };

        info!(
            workflow_id = %workflow_id,
            tasks = recovered_state.task_states.len(),
            messages = recovered_state.conversation_history.len(),
            tokens = recovered_state.total_tokens_used,
            "Persisted state loaded, attempting resume"
        );

        // Load workflow record to build orchestrator config
        let workflow = db::models::Workflow::find_by_id(&self.db.pool, workflow_id)
            .await?
            .ok_or_else(|| anyhow!("Workflow {workflow_id} not found during recovery"))?;

        // Build orchestrator config (mirrors start_workflow_reserved logic).
        // Missing API key falls through — native credentials handle it.
        let orchestrator_config = if workflow.orchestrator_enabled {
            let api_key = workflow.get_api_key().ok().flatten().unwrap_or_default();
            OrchestratorConfig::from_workflow(
                workflow.orchestrator_api_type.as_deref(),
                workflow.orchestrator_base_url.as_deref(),
                if api_key.is_empty() { None } else { Some(&api_key) },
                workflow.orchestrator_model.as_deref(),
            )
        } else {
            None
        };

        let config = orchestrator_config.unwrap_or_default();
        let mut agent = OrchestratorAgent::new(
            config,
            workflow_id.to_string(),
            self.message_bus.clone(),
            self.db.clone(),
        )?;

        if let Some(runtime_actions) = self.runtime_actions.read().await.clone() {
            agent.attach_runtime_actions(runtime_actions);
        }
        agent.attach_persistence(Arc::new(StatePersistence::new(self.db.clone())));
        if let Some(ref broadcaster) = *self.concierge_broadcaster.read().await {
            agent.attach_concierge_broadcaster(broadcaster.clone());
        }
        let agent = Arc::new(agent);

        // Inject the recovered state into the agent
        agent.restore_state(recovered_state).await;

        // G05-005: Start GitWatcher BEFORE spawning the agent task so the
        // watcher is already polling when the agent begins its event loop.
        // This eliminates the timing window where commits landing between
        // agent start and watcher start would be silently missed.
        match self.try_start_git_watcher(workflow_id, &workflow).await {
            Ok(Some(handle)) => {
                let mut watchers = self.git_watchers.lock().await;
                watchers.insert(workflow_id.to_string(), handle);
            }
            Ok(None) => {
                debug!(
                    "GitWatcher not started for recovered workflow {} (no valid repo)",
                    workflow_id
                );
            }
            Err(e) => {
                warn!(
                    "Failed to start GitWatcher for recovered workflow {}: {}",
                    workflow_id, e
                );
            }
        }

        // Spawn agent task (same pattern as start_workflow_reserved)
        let agent_clone = agent.clone();
        let running_workflows = self.running_workflows.clone();
        let git_watchers = self.git_watchers.clone();
        let chat_idempotency = self.orchestrator_chat_idempotency.clone();
        let workflow_id_owned = workflow_id.to_string();
        let task_handle = tokio::spawn(async move {
            if let Err(e) = Box::pin(agent_clone.run()).await {
                error!(
                    "Recovered orchestrator agent failed for workflow {}: {}",
                    workflow_id_owned, e
                );
            }

            // Best-effort cleanup (mirrors start_workflow_reserved)
            let mut removed_running = false;
            for _ in 0..5 {
                let mut running = running_workflows.lock().await;
                let can_remove = running
                    .get(&workflow_id_owned)
                    .is_some_and(|entry| entry.task_handle.is_finished());

                if can_remove {
                    running.remove(&workflow_id_owned);
                    removed_running = true;
                    break;
                }

                drop(running);
                sleep(Duration::from_millis(100)).await;
            }

            if removed_running {
                {
                    let mut idempotency = chat_idempotency.lock().await;
                    idempotency.remove(&workflow_id_owned);
                }

                let git_watcher_handle = {
                    let mut watchers = git_watchers.lock().await;
                    watchers.remove(&workflow_id_owned)
                };

                if let Some(handle) = git_watcher_handle {
                    handle.watcher.stop();
                    let mut watcher_task = handle.task_handle;
                    let shutdown_result = timeout(Duration::from_secs(5), &mut watcher_task).await;
                    if shutdown_result.is_err() {
                        watcher_task.abort();
                        watcher_task.await.ok();
                    }
                }
            }
        });

        // Register in running workflows map
        let mut running = self.running_workflows.lock().await;
        running.insert(
            workflow_id.to_string(),
            RunningWorkflow { agent, task_handle },
        );
        drop(running);

        Ok(true)
    }

    /// Recover incomplete orchestrator commands that were interrupted during restart.
    pub async fn recover_incomplete_orchestrator_commands(&self) -> Result<usize> {
        let recovered =
            match WorkflowOrchestratorCommand::recover_incomplete_commands(&self.db.pool).await {
                Ok(rows) => rows,
                Err(sqlx::Error::Database(db_error))
                    if db_error
                        .message()
                        .contains("no such table: workflow_orchestrator_command") =>
                {
                    // Older test fixtures may not include this table. Keep startup recovery resilient.
                    return Ok(0);
                }
                Err(e) => {
                    return Err(anyhow!(
                        "Failed to recover incomplete orchestrator commands: {e}"
                    ));
                }
            };
        Ok(recovered as usize)
    }
}

#[cfg(test)]
mod tests {
    use chrono::Utc;
    use db::{DBService, models::Workflow};
    use tokio::sync::Barrier;
    use uuid::Uuid;

    use super::*;
    use crate::services::orchestrator::{MessageBus, MockLLMClient, OrchestratorState};

    async fn setup_runtime_with_ready_workflow() -> (Arc<OrchestratorRuntime>, String) {
        let pool = sqlx::SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE workflow (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                name TEXT NOT NULL,
                description TEXT,
                status TEXT NOT NULL,
                execution_mode TEXT NOT NULL DEFAULT 'diy',
                initial_goal TEXT,
                use_slash_commands INTEGER NOT NULL DEFAULT 0,
                orchestrator_enabled INTEGER NOT NULL DEFAULT 0,
                orchestrator_api_type TEXT,
                orchestrator_base_url TEXT,
                orchestrator_api_key TEXT,
                orchestrator_model TEXT,
                error_terminal_enabled INTEGER NOT NULL DEFAULT 0,
                error_terminal_cli_id TEXT,
                error_terminal_model_id TEXT,
                merge_terminal_cli_id TEXT NOT NULL,
                merge_terminal_model_id TEXT NOT NULL,
                target_branch TEXT NOT NULL,
                git_watcher_enabled INTEGER NOT NULL DEFAULT 1,
                orchestrator_state TEXT,
                ready_at TEXT,
                started_at TEXT,
                completed_at TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                pause_reason TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });
        let message_bus = Arc::new(MessageBus::new(1000));
        let runtime = Arc::new(OrchestratorRuntime::new(db, message_bus));

        let workflow_id = Uuid::new_v4().to_string();
        let workflow = Workflow {
            id: workflow_id.clone(),
            project_id: Uuid::new_v4(),
            name: "Concurrent Start Workflow".to_string(),
            description: None,
            status: WORKFLOW_STATUS_READY.to_string(),
            execution_mode: "diy".to_string(),
            initial_goal: None,
            use_slash_commands: false,
            orchestrator_enabled: false,
            orchestrator_api_type: None,
            orchestrator_base_url: None,
            orchestrator_api_key: None,
            orchestrator_model: None,
            error_terminal_enabled: false,
            error_terminal_cli_id: None,
            error_terminal_model_id: None,
            merge_terminal_cli_id: "merge-cli".to_string(),
            merge_terminal_model_id: "merge-model".to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: Some(Utc::now()),
            started_at: None,
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
            pause_reason: None,
        };
        Workflow::create(&pool, &workflow).await.unwrap();

        (runtime, workflow_id)
    }

    async fn setup_runtime_for_git_watcher_path_tests() -> Arc<OrchestratorRuntime> {
        let pool = sqlx::SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE projects (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                default_agent_working_dir TEXT,
                remote_project_id TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                pause_reason TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r"
            CREATE TABLE repos (
                id TEXT PRIMARY KEY,
                path TEXT NOT NULL UNIQUE,
                name TEXT NOT NULL,
                display_name TEXT NOT NULL,
                setup_script TEXT,
                cleanup_script TEXT,
                copy_files TEXT,
                parallel_setup_script INTEGER NOT NULL DEFAULT 0,
                dev_server_script TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                pause_reason TEXT
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r"
            CREATE TABLE project_repos (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                repo_id TEXT NOT NULL
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });
        let message_bus = Arc::new(MessageBus::new(1000));
        let config = RuntimeConfig {
            max_concurrent_workflows: 10,
            message_bus_capacity: 1000,
            git_watch_poll_interval_ms: 10,
        };
        Arc::new(OrchestratorRuntime::with_config(db, message_bus, config))
    }

    async fn insert_project_repo_fixture(
        pool: &sqlx::SqlitePool,
        project_id: Uuid,
        default_agent_working_dir: Option<&str>,
        repo_path: &str,
    ) {
        let repo_id = Uuid::new_v4();

        sqlx::query(
            r"
            INSERT INTO projects (
                id,
                name,
                default_agent_working_dir,
                remote_project_id,
                created_at,
                updated_at
            )
            VALUES ($1, $2, $3, NULL, datetime('now'), datetime('now'))
            ",
        )
        .bind(project_id)
        .bind(format!("Project-{project_id}"))
        .bind(default_agent_working_dir)
        .execute(pool)
        .await
        .unwrap();

        sqlx::query(
            r"
            INSERT INTO repos (
                id,
                path,
                name,
                display_name,
                setup_script,
                cleanup_script,
                copy_files,
                parallel_setup_script,
                dev_server_script,
                created_at,
                updated_at
            )
            VALUES ($1, $2, $3, $4, NULL, NULL, NULL, 0, NULL, datetime('now'), datetime('now'))
            ",
        )
        .bind(repo_id)
        .bind(repo_path)
        .bind("repo")
        .bind("Repo")
        .execute(pool)
        .await
        .unwrap();

        sqlx::query(
            r"
            INSERT INTO project_repos (id, project_id, repo_id)
            VALUES ($1, $2, $3)
            ",
        )
        .bind(Uuid::new_v4())
        .bind(project_id)
        .bind(repo_id)
        .execute(pool)
        .await
        .unwrap();
    }

    fn build_workflow_for_project(project_id: Uuid) -> Workflow {
        Workflow {
            id: Uuid::new_v4().to_string(),
            project_id,
            name: "GitWatcher Path Resolution".to_string(),
            description: None,
            status: WORKFLOW_STATUS_READY.to_string(),
            execution_mode: "diy".to_string(),
            initial_goal: None,
            use_slash_commands: false,
            orchestrator_enabled: false,
            orchestrator_api_type: None,
            orchestrator_base_url: None,
            orchestrator_api_key: None,
            orchestrator_model: None,
            error_terminal_enabled: false,
            error_terminal_cli_id: None,
            error_terminal_model_id: None,
            merge_terminal_cli_id: "merge-cli".to_string(),
            merge_terminal_model_id: "merge-model".to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: Some(Utc::now()),
            started_at: None,
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
            pause_reason: None,
        }
    }

    async fn shutdown_git_watcher(handle: GitWatcherHandle) {
        handle.watcher.stop();
        let mut task_handle = handle.task_handle;
        if timeout(Duration::from_secs(1), &mut task_handle)
            .await
            .is_err()
        {
            task_handle.abort();
            task_handle.await.ok();
        }
    }

    #[tokio::test]
    async fn test_start_workflow_blocks_concurrent_duplicate_start() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let barrier = Arc::new(Barrier::new(3));

        let runtime_a = runtime.clone();
        let workflow_id_a = workflow_id.clone();
        let barrier_a = barrier.clone();
        let reserve_a = tokio::spawn(async move {
            barrier_a.wait().await;
            runtime_a.reserve_start_slot(&workflow_id_a).await
        });

        let runtime_b = runtime.clone();
        let workflow_id_b = workflow_id.clone();
        let barrier_b = barrier.clone();
        let reserve_b = tokio::spawn(async move {
            barrier_b.wait().await;
            runtime_b.reserve_start_slot(&workflow_id_b).await
        });

        barrier.wait().await;

        let result_a = reserve_a.await.unwrap();
        let result_b = reserve_b.await.unwrap();
        let results = [result_a, result_b];

        let success_count = results.iter().filter(|result| result.is_ok()).count();
        let already_running_error_count = results
            .iter()
            .filter(|result| {
                result
                    .as_ref()
                    .err()
                    .is_some_and(|error| error.to_string().contains("already running"))
            })
            .count();

        assert_eq!(
            success_count, 1,
            "Exactly one caller should reserve the start slot: {:?}",
            results
        );
        assert_eq!(
            already_running_error_count, 1,
            "Competing reserve should fail with already running: {:?}",
            results
        );
        assert!(
            runtime.running_count().await == 0,
            "Reserving start slot should not register running workflow instances"
        );

        runtime.release_start_slot(&workflow_id).await;
    }

    #[tokio::test]
    async fn test_submit_user_prompt_response_returns_error_when_workflow_not_running() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let result = runtime
            .submit_user_prompt_response(&workflow_id, "terminal-1", "approve")
            .await;

        let error = result.expect_err("workflow not running should return error");
        assert!(
            error
                .to_string()
                .contains(&format!("Workflow {} is not running", workflow_id))
        );
    }

    #[tokio::test]
    async fn test_submit_user_prompt_response_forwards_to_running_agent() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let agent = Arc::new(
            OrchestratorAgent::with_llm_client(
                OrchestratorConfig::default(),
                workflow_id.clone(),
                runtime.message_bus.clone(),
                runtime.db.clone(),
                Box::new(MockLLMClient::new()),
            )
            .expect("should create test agent"),
        );

        let task_handle = tokio::spawn(async {});
        {
            let mut running = runtime.running_workflows.lock().await;
            running.insert(workflow_id.clone(), RunningWorkflow { agent, task_handle });
        }

        let terminal_id = "terminal-missing";
        let result = runtime
            .submit_user_prompt_response(&workflow_id, terminal_id, "approve")
            .await;

        let error = result.expect_err("running workflow should forward to agent");
        let error_text = error.to_string();
        assert!(
            error_text.contains("Failed to submit user prompt response for workflow"),
            "unexpected error: {error_text}"
        );
        assert!(
            error_text.contains(terminal_id),
            "unexpected error: {error_text}"
        );
        assert!(
            !error_text.contains("is not running"),
            "unexpected error: {error_text}"
        );

        runtime.stop_workflow(&workflow_id).await.unwrap();
    }

    #[tokio::test]
    async fn test_submit_orchestrator_chat_returns_error_when_workflow_not_running() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let result = runtime
            .submit_orchestrator_chat(&workflow_id, "hello orchestrator", "web", None)
            .await;

        let error = result.expect_err("workflow not running should return error");
        assert!(
            error
                .to_string()
                .contains(&format!("Workflow {} is not running", workflow_id))
        );
    }

    #[tokio::test]
    async fn test_submit_orchestrator_chat_updates_running_agent_conversation() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let agent = Arc::new(
            OrchestratorAgent::with_llm_client(
                OrchestratorConfig::default(),
                workflow_id.clone(),
                runtime.message_bus.clone(),
                runtime.db.clone(),
                Box::new(MockLLMClient::new()),
            )
            .expect("should create test agent"),
        );

        let task_handle = tokio::spawn(async {});
        {
            let mut running = runtime.running_workflows.lock().await;
            running.insert(workflow_id.clone(), RunningWorkflow { agent, task_handle });
        }

        runtime
            .submit_orchestrator_chat(&workflow_id, "hello orchestrator", "web", None)
            .await
            .expect("orchestrator chat should be forwarded to running agent");
        let command = runtime
            .submit_orchestrator_chat(&workflow_id, "hello orchestrator 2", "web", None)
            .await
            .expect("second orchestrator chat should be forwarded to running agent");
        assert_eq!(command.status, OrchestratorChatCommandStatus::Succeeded);

        let messages = runtime
            .get_orchestrator_messages(&workflow_id)
            .await
            .expect("should fetch conversation messages");

        assert!(messages.iter().any(|message| {
            message.role == "user" && message.content == "hello orchestrator"
        }));
        assert!(messages.iter().any(|message| {
            message.role == "assistant" && message.content == "Mock response for testing"
        }));

        runtime.stop_workflow(&workflow_id).await.unwrap();
    }

    #[tokio::test]
    async fn test_submit_orchestrator_chat_ignores_duplicate_external_message_id() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let agent = Arc::new(
            OrchestratorAgent::with_llm_client(
                OrchestratorConfig::default(),
                workflow_id.clone(),
                runtime.message_bus.clone(),
                runtime.db.clone(),
                Box::new(MockLLMClient::new()),
            )
            .expect("should create test agent"),
        );

        let task_handle = tokio::spawn(async {});
        {
            let mut running = runtime.running_workflows.lock().await;
            running.insert(workflow_id.clone(), RunningWorkflow { agent, task_handle });
        }

        let first_command = runtime
            .submit_orchestrator_chat(
                &workflow_id,
                "hello orchestrator",
                "social",
                Some("external-1"),
            )
            .await
            .expect("first orchestrator chat should be forwarded");
        assert_eq!(first_command.status, OrchestratorChatCommandStatus::Succeeded);

        let duplicate_command = runtime
            .submit_orchestrator_chat(
                &workflow_id,
                "hello orchestrator",
                "social",
                Some("external-1"),
            )
            .await
            .expect("duplicate orchestrator chat should be ignored");
        assert_eq!(
            duplicate_command.command_id,
            first_command.command_id
        );
        assert_eq!(duplicate_command.status, first_command.status);

        let messages = runtime
            .get_orchestrator_messages(&workflow_id)
            .await
            .expect("should fetch conversation messages");

        let user_message_count = messages
            .iter()
            .filter(|message| {
                message.role == "user" && message.content == "hello orchestrator"
            })
            .count();

        assert_eq!(user_message_count, 1);

        runtime.stop_workflow(&workflow_id).await.unwrap();
    }

    #[tokio::test]
    async fn test_get_orchestrator_messages_returns_error_when_workflow_not_running() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        let result = runtime.get_orchestrator_messages(&workflow_id).await;

        let error = result.expect_err("workflow not running should return error");
        assert!(
            error
                .to_string()
                .contains(&format!("Workflow {} is not running", workflow_id))
        );
    }

    #[tokio::test]
    async fn test_recover_running_workflows_marks_interrupted_workflows_failed() {
        let (runtime, workflow_id) = setup_runtime_with_ready_workflow().await;

        Workflow::update_status(&runtime.db.pool, &workflow_id, "running")
            .await
            .expect("should mark workflow as running");

        let mut persisted_state = OrchestratorState::new(workflow_id.clone());
        persisted_state.set_workflow_planning_complete(false);
        runtime
            .persistence
            .save_state(&persisted_state)
            .await
            .expect("should persist orchestrator state");

        let recovered_count = runtime
            .recover_running_workflows()
            .await
            .expect("recovery should succeed");

        assert_eq!(recovered_count, 1);

        let workflow = Workflow::find_by_id(&runtime.db.pool, &workflow_id)
            .await
            .expect("should query recovered workflow")
            .expect("workflow should still exist");
        assert_eq!(workflow.status, WORKFLOW_STATUS_FAILED);
    }

    #[tokio::test]
    async fn test_try_start_git_watcher_falls_back_to_project_repo_when_default_dir_missing_or_blank()
     {
        let runtime = setup_runtime_for_git_watcher_path_tests().await;

        for default_agent_working_dir in [None, Some("   ")] {
            let repo_dir = tempfile::tempdir().unwrap();
            std::fs::create_dir_all(repo_dir.path().join(".git")).unwrap();
            let repo_path = repo_dir.path().to_string_lossy().into_owned();

            let project_id = Uuid::new_v4();
            insert_project_repo_fixture(
                &runtime.db.pool,
                project_id,
                default_agent_working_dir,
                repo_path.as_str(),
            )
            .await;

            let workflow = build_workflow_for_project(project_id);
            let watcher = runtime
                .try_start_git_watcher(&workflow.id, &workflow)
                .await
                .unwrap();

            assert!(
                watcher.is_some(),
                "GitWatcher should start using project_repos fallback when default_agent_working_dir is {:?}",
                default_agent_working_dir
            );

            if let Some(handle) = watcher {
                shutdown_git_watcher(handle).await;
            }
        }
    }

    #[tokio::test]
    async fn test_try_start_git_watcher_keeps_non_empty_default_dir_behavior() {
        let runtime = setup_runtime_for_git_watcher_path_tests().await;

        let fallback_repo_dir = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(fallback_repo_dir.path().join(".git")).unwrap();
        let fallback_repo_path = fallback_repo_dir.path().to_string_lossy().into_owned();

        let invalid_default_path = std::env::temp_dir()
            .join(format!("gitwatcher-missing-{}", Uuid::new_v4()))
            .to_string_lossy()
            .into_owned();

        let project_id = Uuid::new_v4();
        insert_project_repo_fixture(
            &runtime.db.pool,
            project_id,
            Some(invalid_default_path.as_str()),
            fallback_repo_path.as_str(),
        )
        .await;

        let workflow = build_workflow_for_project(project_id);
        let watcher = runtime
            .try_start_git_watcher(&workflow.id, &workflow)
            .await
            .unwrap();

        assert!(
            watcher.is_none(),
            "Non-empty default_agent_working_dir should remain primary and not fallback to project_repos"
        );
    }
}
