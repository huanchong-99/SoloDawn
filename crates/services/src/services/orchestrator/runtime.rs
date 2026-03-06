//! Orchestrator Runtime Service
//!
//! Manages multiple OrchestratorAgent instances, one per active workflow.

use std::{
    collections::{HashMap, HashSet},
    path::PathBuf,
    sync::Arc,
};

use anyhow::{Result, anyhow};
use db::DBService;
use sqlx::Row;
use tokio::{
    sync::{Mutex, RwLock},
    task::JoinHandle,
    time::{Duration, sleep, timeout},
};
use tracing::{debug, error, info, warn};

use super::{
    OrchestratorAgent, OrchestratorConfig, SharedMessageBus,
    constants::{WORKFLOW_STATUS_FAILED, WORKFLOW_STATUS_READY},
    persistence::StatePersistence,
    runtime_actions::RuntimeActionService,
};
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
    persistence: StatePersistence,
    runtime_actions: Arc<RwLock<Option<Arc<RuntimeActionService>>>>,
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
            persistence,
            runtime_actions: Arc::new(RwLock::new(None)),
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
            persistence,
            runtime_actions: Arc::new(RwLock::new(None)),
        }
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
            match db::models::project::Project::find_by_id(&self.db.pool, workflow.project_id)
                .await?
            {
                Some(project) => project,
                None => {
                    warn!(
                        "Project {} not found for workflow {}, git watcher disabled",
                        workflow.project_id, workflow_id
                    );
                    return Ok(None);
                }
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
            return Err(anyhow!("Workflow {} is already running", workflow_id));
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
            .ok_or_else(|| anyhow!("Workflow {} not found", workflow_id))?;

        // Verify workflow is in ready state
        if workflow.status != WORKFLOW_STATUS_READY {
            return Err(anyhow!(
                "Workflow {} is not ready. Current status: {}",
                workflow_id,
                workflow.status
            ));
        }

        // Build orchestrator config from workflow settings
        let orchestrator_config = if workflow.orchestrator_enabled {
            // Decrypt API key if needed
            let api_key = workflow
                .get_api_key()?
                .ok_or_else(|| anyhow!("Orchestrator API key not configured"))?;

            Some(
                OrchestratorConfig::from_workflow(
                    workflow.orchestrator_api_type.as_deref(),
                    workflow.orchestrator_base_url.as_deref(),
                    Some(&api_key),
                    workflow.orchestrator_model.as_deref(),
                )
                .ok_or_else(|| anyhow!("Invalid orchestrator configuration"))?,
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
        let agent = Arc::new(agent);

        // Update workflow status to running AFTER agent is successfully created
        db::models::Workflow::set_started(&self.db.pool, workflow_id).await?;
        info!("Workflow {} marked as started", workflow_id);

        // Spawn agent task with error handling
        let agent_clone = agent.clone();
        let running_workflows = self.running_workflows.clone();
        let git_watchers = self.git_watchers.clone();
        let workflow_id_owned = workflow_id.to_string();
        let task_handle = tokio::spawn(async move {
            if let Err(e) = agent_clone.run().await {
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
                    .map(|entry| entry.task_handle.is_finished())
                    .unwrap_or(false);

                if can_remove {
                    running.remove(&workflow_id_owned);
                    removed_running = true;
                    break;
                }

                drop(running);
                sleep(Duration::from_millis(100)).await;
            }

            if removed_running {
                let git_watcher_handle = {
                    let mut watchers = git_watchers.lock().await;
                    watchers.remove(&workflow_id_owned)
                };

                if let Some(handle) = git_watcher_handle {
                    handle.watcher.stop();
                    let mut watcher_task = handle.task_handle;
                    let shutdown_result = timeout(Duration::from_secs(5), &mut watcher_task).await;
                    match shutdown_result {
                        Ok(_) => {}
                        Err(_) => {
                            watcher_task.abort();
                            watcher_task.await.ok();
                        }
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
                .ok_or_else(|| anyhow!("Workflow {} is not running", workflow_id))?;

            Arc::clone(&running_workflow.agent)
        };

        agent
            .handle_user_prompt_response(terminal_id, user_response)
            .await
            .map_err(|e| {
                anyhow!(
                    "Failed to submit user prompt response for workflow {} and terminal {}: {}",
                    workflow_id,
                    terminal_id,
                    e
                )
            })
    }

    /// Stop orchestrating a workflow
    ///
    /// Sends shutdown signal to the agent and waits for graceful shutdown.
    /// If shutdown doesn't complete within timeout, the task is aborted.
    pub async fn stop_workflow(&self, workflow_id: &str) -> Result<()> {
        // Stop GitWatcher first (non-blocking)
        self.stop_git_watcher(workflow_id).await;

        // Remove from running workflows
        let running_workflow = {
            let mut running = self.running_workflows.lock().await;
            running
                .remove(workflow_id)
                .ok_or_else(|| anyhow!("Workflow {} is not running", workflow_id))?
        };

        // Send shutdown signal via message bus
        self.message_bus
            .publish(
                &format!("workflow:{}", workflow_id),
                super::BusMessage::Shutdown,
            )
            .await?;

        info!("Shutdown signal sent for workflow {}", workflow_id);

        // Wait for graceful shutdown (5 second timeout)
        let mut task_handle = running_workflow.task_handle;
        let shutdown_result = timeout(Duration::from_secs(5), &mut task_handle).await;

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

    /// Check if a workflow is currently running
    pub async fn is_running(&self, workflow_id: &str) -> bool {
        let running = self.running_workflows.lock().await;
        running.contains_key(workflow_id)
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
    /// Finds all workflows with status 'running' and marks them as 'failed',
    /// as they were likely interrupted by a crash or restart.
    /// Should be called on service startup.
    pub async fn recover_running_workflows(&self) -> Result<()> {
        // Query for workflows with status 'running'
        // Note: We need to add this method to Workflow model, but for now use a workaround
        let pool = &self.db.pool;

        // Direct SQL query to find running workflows
        let rows = sqlx::query(
            r#"
            SELECT id
            FROM workflow
            WHERE status = 'running'
            "#,
        )
        .fetch_all(pool)
        .await?;

        if rows.is_empty() {
            info!("No running workflows to recover");
            return Ok(());
        }

        warn!("Found {} running workflows to recover", rows.len());

        for row in rows {
            let workflow_id: String = row.get("id");
            warn!("Recovering workflow {}", workflow_id);

            // Try to load persisted state
            match self.persistence.recover_workflow(&workflow_id).await {
                Ok(Some(state)) => {
                    info!(
                        "Successfully recovered state for workflow {} with {} tasks and {} messages",
                        workflow_id,
                        state.task_states.len(),
                        state.conversation_history.len()
                    );

                    // For now, we mark the workflow as failed since we can't automatically
                    // resume without more complex recovery logic
                    // In the future, this could restart the workflow with the recovered state
                    if let Err(e) = db::models::Workflow::update_status(
                        pool,
                        &workflow_id,
                        WORKFLOW_STATUS_FAILED,
                    )
                    .await
                    {
                        error!(
                            "Failed to mark workflow {} as failed during recovery: {}",
                            workflow_id, e
                        );
                    } else {
                        info!(
                            "Workflow {} marked as failed (state recovered but auto-resume not implemented)",
                            workflow_id
                        );
                    }
                }
                Ok(None) => {
                    warn!("No persisted state found for workflow {}", workflow_id);

                    // Mark as failed since we can't recover without state
                    if let Err(e) = db::models::Workflow::update_status(
                        pool,
                        &workflow_id,
                        WORKFLOW_STATUS_FAILED,
                    )
                    .await
                    {
                        error!(
                            "Failed to mark workflow {} as failed during recovery: {}",
                            workflow_id, e
                        );
                    } else {
                        info!(
                            "Workflow {} marked as failed (no state recovered)",
                            workflow_id
                        );
                    }
                }
                Err(e) => {
                    error!(
                        "Failed to recover state for workflow {}: {}",
                        workflow_id, e
                    );

                    // Still mark as failed even if state recovery failed
                    if let Err(e) = db::models::Workflow::update_status(
                        pool,
                        &workflow_id,
                        WORKFLOW_STATUS_FAILED,
                    )
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

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use chrono::Utc;
    use db::{DBService, models::Workflow};
    use tokio::sync::Barrier;
    use uuid::Uuid;

    use super::*;
    use crate::services::orchestrator::{MessageBus, MockLLMClient};

    async fn setup_runtime_with_ready_workflow() -> (Arc<OrchestratorRuntime>, String) {
        let pool = sqlx::SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r#"
            CREATE TABLE workflow (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                name TEXT NOT NULL,
                description TEXT,
                status TEXT NOT NULL,
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
                ready_at TEXT,
                started_at TEXT,
                completed_at TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            )
            "#,
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
        };
        Workflow::create(&pool, &workflow).await.unwrap();

        (runtime, workflow_id)
    }

    async fn setup_runtime_for_git_watcher_path_tests() -> Arc<OrchestratorRuntime> {
        let pool = sqlx::SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r#"
            CREATE TABLE projects (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                default_agent_working_dir TEXT,
                remote_project_id TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            )
            "#,
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
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
                updated_at TEXT NOT NULL
            )
            "#,
        )
        .execute(&pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            CREATE TABLE project_repos (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                repo_id TEXT NOT NULL
            )
            "#,
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
            r#"
            INSERT INTO projects (
                id,
                name,
                default_agent_working_dir,
                remote_project_id,
                created_at,
                updated_at
            )
            VALUES ($1, $2, $3, NULL, datetime('now'), datetime('now'))
            "#,
        )
        .bind(project_id)
        .bind(format!("Project-{project_id}"))
        .bind(default_agent_working_dir)
        .execute(pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
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
            "#,
        )
        .bind(repo_id)
        .bind(repo_path)
        .bind("repo")
        .bind("Repo")
        .execute(pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            INSERT INTO project_repos (id, project_id, repo_id)
            VALUES ($1, $2, $3)
            "#,
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
