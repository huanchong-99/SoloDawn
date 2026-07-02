use std::{
    collections::HashMap,
    io,
    path::{Path, PathBuf},
    str::FromStr,
    sync::Arc,
    time::Duration,
};

use anyhow::anyhow;
use async_trait::async_trait;
use command_group::AsyncGroupChild;
use db::{
    DBService,
    models::{
        coding_agent_turn::CodingAgentTurn,
        execution_process::{
            ExecutionContext, ExecutionProcess, ExecutionProcessRunReason, ExecutionProcessStatus,
        },
        execution_process_repo_state::ExecutionProcessRepoState,
        repo::Repo,
        scratch::{DraftFollowUpData, Scratch, ScratchType},
        task::{Task, TaskStatus},
        workspace::Workspace,
        workspace_repo::WorkspaceRepo,
    },
};
use deployment::DeploymentError;
use executors::{
    actions::{
        Executable, ExecutorAction, ExecutorActionType,
        coding_agent_follow_up::CodingAgentFollowUpRequest,
        coding_agent_initial::CodingAgentInitialRequest,
    },
    approvals::{ExecutorApprovalService, NoopExecutorApprovalService},
    env::ExecutionEnv,
    executors::{
        BaseCodingAgent, CodingAgent, ExecutorExitResult, ExecutorExitSignal, InterruptSender,
    },
    logs::{NormalizedEntryType, utils::patch::extract_normalized_entry_from_patch},
    profile::{ExecutorConfigs, ExecutorProfileId},
};
use futures::{FutureExt, TryStreamExt, stream::select};
use services::services::{
    approvals::{Approvals, executor_approvals::ExecutorApprovalBridge},
    config::Config,
    container::{ContainerError, ContainerRef, ContainerService},
    diff_stream::{self, DiffStreamHandle},
    git::{GitCli, GitService},
    image::ImageService,
    notification::NotificationService,
    queued_message::QueuedMessageService,
    workspace_manager::{RepoWorkspaceInput, WorkspaceManager},
};
use tokio::{sync::RwLock, task::JoinHandle};
use tokio_util::io::ReaderStream;
use utils::{
    log_msg::LogMsg,
    msg_store::MsgStore,
    text::{git_branch_id, short_uuid, truncate_to_char_boundary},
};
use uuid::Uuid;

use crate::{command, copy};

// ============================================================================
// S5 — Interactive transcript tailer tuning
// ============================================================================

/// Poll cadence for tailing the interactive session transcript JSONL.
/// Matches the 250ms cadence already used by `wait_for_exit_status`.
const TRANSCRIPT_POLL_INTERVAL: Duration = Duration::from_millis(250);

/// Idle ticks (each = `TRANSCRIPT_POLL_INTERVAL`) of no new transcript bytes
/// required AFTER the `turn_duration` marker before pushing `Finished`
/// (short debounce: 4 * 250ms = 1s).
const TRANSCRIPT_MARKER_DEBOUNCE_TICKS: u32 = 4;

/// Idle ticks with no new bytes that force `Finished` as a LAST-RESORT safety
/// net only. S6 drives `Finished` off the genuine `claude` child exit (see
/// `spawn_interactive_completion_watcher`), so this fallback exists purely to
/// reap an orphaned tailer if the child-exit signal is ever lost. PROBE found a
/// 10s idle can fire mid-long-tool, so it is set deliberately long (480 *
/// 250ms = 120s of total silence) to never pre-empt a still-running turn.
const TRANSCRIPT_IDLE_TIMEOUT_TICKS: u32 = 480;

#[derive(Clone)]
pub struct LocalContainerService {
    db: DBService,
    child_store: Arc<RwLock<HashMap<Uuid, Arc<RwLock<AsyncGroupChild>>>>>,
    interrupt_senders: Arc<RwLock<HashMap<Uuid, InterruptSender>>>,
    msg_stores: Arc<RwLock<HashMap<Uuid, Arc<MsgStore>>>>,
    config: Arc<RwLock<Config>>,
    git: GitService,
    image_service: ImageService,
    approvals: Approvals,
    queued_message_service: QueuedMessageService,
    notification_service: NotificationService,
    /// [CONCURRENCY-014] Shutdown signal + handle for the periodic workspace
    /// cleanup task spawned in [`Self::spawn_workspace_cleanup`].
    ///
    /// Previously the cleanup task's `JoinHandle` was silently dropped by
    /// `tokio::spawn`, so on process shutdown the runtime would abort it at an
    /// arbitrary await point — potentially in the middle of a worktree
    /// deletion, leaving half-cleaned directories behind.
    ///
    /// We now (a) retain the handle so callers can `await` orderly teardown
    /// via [`Self::shutdown_cleanup`], and (b) feed the task a shutdown signal
    /// via a `tokio::sync::watch` channel so it breaks out of its loop only
    /// at a transaction boundary (between `cleanup_expired` calls), avoiding
    /// partial filesystem cleanups.
    ///
    /// `std::sync::Mutex` is used for `cleanup_handle` (rather than
    /// `tokio::sync::RwLock`) so that `spawn_workspace_cleanup` can install
    /// the handle synchronously from the (sync) constructor. The critical
    /// section is trivial (a single `Option::replace`) and never `.await`s,
    /// so a sync lock is both safe and correct here.
    cleanup_shutdown_tx: Arc<tokio::sync::watch::Sender<bool>>,
    cleanup_handle: Arc<std::sync::Mutex<Option<JoinHandle<()>>>>,
}

impl LocalContainerService {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        db: DBService,
        msg_stores: Arc<RwLock<HashMap<Uuid, Arc<MsgStore>>>>,
        config: Arc<RwLock<Config>>,
        git: GitService,
        image_service: ImageService,
        approvals: Approvals,
        queued_message_service: QueuedMessageService,
    ) -> Self {
        let child_store = Arc::new(RwLock::new(HashMap::new()));
        let interrupt_senders = Arc::new(RwLock::new(HashMap::new()));
        let notification_service = NotificationService::new(config.clone());

        // [CONCURRENCY-014] watch channel initialized to `false` (not yet
        // shutting down). Cloning the `Sender` is cheap, so we wrap it in an
        // `Arc` to satisfy `Clone` on the service without re-threading borrows.
        let (cleanup_shutdown_tx, _) = tokio::sync::watch::channel(false);

        let container = LocalContainerService {
            db,
            child_store,
            interrupt_senders,
            msg_stores,
            config,
            git,
            image_service,
            approvals,
            queued_message_service,
            notification_service,
            cleanup_shutdown_tx: Arc::new(cleanup_shutdown_tx),
            cleanup_handle: Arc::new(std::sync::Mutex::new(None)),
        };

        container.spawn_workspace_cleanup();

        container
    }

    /// Gracefully stop the periodic workspace cleanup task.
    ///
    /// [CONCURRENCY-014] Signals the cleanup loop to exit at the next iteration
    /// boundary (never mid-`cleanup_expired`), then awaits the task so callers
    /// can be sure no worktree cleanup is in flight after this returns.
    /// Idempotent: safe to call multiple times.
    pub async fn shutdown_cleanup(&self) {
        // Ignore send error: receivers may already be gone.
        let _ = self.cleanup_shutdown_tx.send(true);
        let handle = { self.cleanup_handle.lock().unwrap().take() };
        if let Some(h) = handle {
            let _ = h.await;
        }
    }

    pub async fn get_child_from_store(&self, id: &Uuid) -> Option<Arc<RwLock<AsyncGroupChild>>> {
        let map = self.child_store.read().await;
        map.get(id).cloned()
    }

    pub async fn add_child_to_store(&self, id: Uuid, exec: AsyncGroupChild) {
        let mut map = self.child_store.write().await;
        map.insert(id, Arc::new(RwLock::new(exec)));
    }

    pub async fn remove_child_from_store(&self, id: &Uuid) {
        let mut map = self.child_store.write().await;
        map.remove(id);
    }

    async fn add_interrupt_sender(&self, id: Uuid, sender: InterruptSender) {
        let mut map = self.interrupt_senders.write().await;
        map.insert(id, sender);
    }

    async fn take_interrupt_sender(&self, id: &Uuid) -> Option<InterruptSender> {
        let mut map = self.interrupt_senders.write().await;
        map.remove(id)
    }

    pub async fn cleanup_workspace(db: &DBService, workspace: &Workspace) {
        let Some(container_ref) = &workspace.container_ref else {
            return;
        };
        let workspace_dir = PathBuf::from(container_ref);

        let repositories = WorkspaceRepo::find_repos_for_workspace(&db.pool, workspace.id)
            .await
            .unwrap_or_default();

        if repositories.is_empty() {
            tracing::warn!(
                "No repositories found for workspace {}, cleaning up workspace directory only",
                workspace.id
            );
            if workspace_dir.exists()
                && let Err(e) = tokio::fs::remove_dir_all(&workspace_dir).await
            {
                tracing::warn!("Failed to remove workspace directory: {}", e);
            }
        } else {
            WorkspaceManager::cleanup_workspace(&workspace_dir, &repositories)
                .await
                .unwrap_or_else(|e| {
                    tracing::warn!(
                        "Failed to clean up workspace for workspace {}: {}",
                        workspace.id,
                        e
                    );
                });
        }

        // S4/S6 — logical-session teardown for the no-`-p` interactive transport.
        // Interactive CLAUDE homes (`claude-isession-<session_uuid>`) are EXEMPT
        // from terminal-end delete so follow-ups can `--resume` the same
        // transcript; their RB-37 secret cleanup is deferred to here (workspace
        // teardown). Delete each one keyed on the workspace's agent session ids.
        // (`-p` session ids won't have a matching dir → harmless no-op.)
        match CodingAgentTurn::agent_session_ids_for_workspace(&db.pool, workspace.id).await {
            Ok(session_ids) => {
                for session_id in session_ids {
                    let home = services::services::cc_switch::interactive_isolated_home_path(
                        &session_id,
                    );
                    if home.exists() {
                        services::terminal::ProcessManager::cleanup_logical_session_home(&home);
                    }
                }
            }
            Err(e) => {
                tracing::warn!(
                    workspace_id = %workspace.id,
                    error = %e,
                    "failed to enumerate interactive session ids for teardown"
                );
            }
        }

        // Clear container_ref so this workspace won't be picked up again
        let _ = Workspace::clear_container_ref(&db.pool, workspace.id).await;
    }

    pub async fn cleanup_expired_workspaces(db: &DBService) -> Result<(), DeploymentError> {
        // Reap stale isolated executor homes the `-p` path leaks. These ws-* dirs
        // (created by `resolve_executor_env_vars`) hold copied credentials, are
        // keyed on an unrecorded random UUID, and are never otherwise removed.
        // Age-based (they cannot be matched to a live run) and guarded on the
        // temp-dir prefix. Runs on EVERY sweep, before the expired-workspace
        // early-return below.
        Self::reap_stale_executor_homes();

        let expired_workspaces = Workspace::find_expired_for_cleanup(&db.pool).await?;
        if expired_workspaces.is_empty() {
            tracing::debug!("No expired workspaces found");
            return Ok(());
        }
        tracing::info!(
            "Found {} expired workspaces to clean up",
            expired_workspaces.len()
        );
        for workspace in &expired_workspaces {
            Self::cleanup_workspace(db, workspace).await;
        }
        Ok(())
    }

    /// Age in seconds after which a leaked isolated executor home (ws-*) is
    /// reaped. Generous so a long-running `-p` agent run can never have its
    /// in-use credential home deleted out from under it.
    const STALE_EXECUTOR_HOME_MAX_AGE_SECS: u64 = 24 * 60 * 60;

    /// Best-effort sweep of leaked per-run isolated executor homes.
    ///
    /// `resolve_executor_env_vars` creates `<temp>/solodawn/{claude,codex}-workspaces/ws-<uuid>`
    /// (with copied credentials) for the `-p` path on every run and never removes
    /// them. They are keyed on a random UUID that is never persisted, so they
    /// cannot be matched to a live run; instead we remove any `ws-*` subdir whose
    /// mtime is older than [`Self::STALE_EXECUTOR_HOME_MAX_AGE_SECS`]. Every
    /// `remove_dir_all` is guarded on the `get_solodawn_temp_dir()` prefix so a
    /// misconfigured `SOLODAWN_TEMP_DIR` cannot widen the delete (same safety
    /// pattern as `ProcessManager::cleanup_isolated_home`). All errors are logged,
    /// never propagated — this is opportunistic reclamation.
    fn reap_stale_executor_homes() {
        let temp_dir = utils::path::get_solodawn_temp_dir();
        let now = std::time::SystemTime::now();
        let max_age = Duration::from_secs(Self::STALE_EXECUTOR_HOME_MAX_AGE_SECS);

        for sub in ["claude-workspaces", "codex-workspaces"] {
            let base = temp_dir.join(sub);
            // Safety: never recurse outside the temp dir, even if `base` is a symlink.
            if !base.starts_with(&temp_dir) {
                continue;
            }
            // Dir not created yet (or already gone) — nothing to reap.
            let Ok(entries) = std::fs::read_dir(&base) else {
                continue;
            };
            for entry in entries.flatten() {
                let path = entry.path();
                let name = entry.file_name();
                let Some(name) = name.to_str() else { continue };
                // Only touch the homes this code creates (ws-<uuid>).
                if !name.starts_with("ws-") {
                    continue;
                }
                // Re-assert the prefix guard on the concrete path before deleting.
                if !path.starts_with(&temp_dir) {
                    continue;
                }
                let too_old = entry
                    .metadata()
                    .and_then(|m| m.modified())
                    .ok()
                    .and_then(|mtime| now.duration_since(mtime).ok())
                    .is_some_and(|age| age >= max_age);
                if !too_old {
                    continue;
                }
                match std::fs::remove_dir_all(&path) {
                    Ok(()) => tracing::info!(
                        home = %path.display(),
                        "Reaped stale isolated executor home"
                    ),
                    Err(e) if e.kind() == io::ErrorKind::NotFound => {}
                    Err(e) => tracing::warn!(
                        home = %path.display(),
                        error = %e,
                        "Failed to reap stale isolated executor home"
                    ),
                }
            }
        }
    }

    pub fn spawn_workspace_cleanup(&self) {
        let db = self.db.clone();
        let cleanup_expired = Self::cleanup_expired_workspaces;
        // [CONCURRENCY-014] Subscribe to the shutdown watch so the task can
        // break out of its loop at a clean boundary (between
        // `cleanup_expired` invocations) instead of being aborted mid-call by
        // the runtime when the JoinHandle is dropped on process exit.
        let mut shutdown_rx = self.cleanup_shutdown_tx.subscribe();
        let handle = tokio::spawn(async move {
            WorkspaceManager::cleanup_orphan_workspaces(&db.pool).await;

            let mut cleanup_interval =
                tokio::time::interval(tokio::time::Duration::from_secs(1800)); // 30 minutes
            loop {
                // Use `select!` so a shutdown signal wins over the next tick.
                // We only check the signal *between* cleanup runs, never
                // during one — so an in-flight `cleanup_expired` always
                // completes (no half-deleted worktrees).
                tokio::select! {
                    _ = cleanup_interval.tick() => {
                        tracing::info!("Starting periodic workspace cleanup...");
                        cleanup_expired(&db).await.unwrap_or_else(|e| {
                            tracing::error!("Failed to clean up expired workspaces: {}", e);
                        });
                    }
                    // `changed()` resolves when the sender writes `true`.
                    _ = shutdown_rx.changed() => {
                        if *shutdown_rx.borrow() {
                            tracing::info!(
                                "Workspace cleanup task received shutdown signal, exiting loop"
                            );
                            break;
                        }
                    }
                }
            }
        });

        // [CONCURRENCY-014] Retain the JoinHandle so callers can await orderly
        // teardown via `shutdown_cleanup`. Installed synchronously under the
        // (sync) `std::sync::Mutex` so it is visible immediately after
        // `spawn_workspace_cleanup` returns — no race where a concurrent
        // `shutdown_cleanup` could observe `None` and skip awaiting the task.
        *self.cleanup_handle.lock().unwrap() = Some(handle);
    }

    /// Record the current HEAD commit for each repository as the "after" state.
    /// Errors are silently ignored since this runs after the main execution completes
    /// and failure should not block process finalization.
    async fn update_after_head_commits(&self, exec_id: Uuid) {
        if let Ok(ctx) = ExecutionProcess::load_context(&self.db.pool, exec_id).await {
            let workspace_root = self.workspace_to_current_dir(&ctx.workspace);
            for repo in &ctx.repos {
                let repo_path = workspace_root.join(&repo.name);
                if let Ok(head) = self.git().get_head_info(&repo_path) {
                    let _ = ExecutionProcessRepoState::update_after_head_commit(
                        &self.db.pool,
                        exec_id,
                        repo.id,
                        &head.oid,
                    )
                    .await;
                }
            }
        }
    }

    /// Get the commit message based on the execution run reason.
    async fn get_commit_message(&self, ctx: &ExecutionContext) -> String {
        match ctx.execution_process.run_reason {
            ExecutionProcessRunReason::CodingAgent => {
                // Try to retrieve the task summary from the coding agent turn
                // otherwise fallback to default message
                match CodingAgentTurn::find_by_execution_process_id(
                    &self.db().pool,
                    ctx.execution_process.id,
                )
                .await
                {
                    Ok(Some(turn)) if turn.summary.is_some() => turn.summary.unwrap(),
                    Ok(_) => {
                        tracing::debug!(
                            "No summary found for execution process {}, using default message",
                            ctx.execution_process.id
                        );
                        format!(
                            "Commit changes from coding agent for workspace {}",
                            ctx.workspace.id
                        )
                    }
                    Err(e) => {
                        tracing::debug!(
                            "Failed to retrieve summary for execution process {}: {}",
                            ctx.execution_process.id,
                            e
                        );
                        format!(
                            "Commit changes from coding agent for workspace {}",
                            ctx.workspace.id
                        )
                    }
                }
            }
            ExecutionProcessRunReason::CleanupScript => {
                format!("Cleanup script changes for workspace {}", ctx.workspace.id)
            }
            _ => format!(
                "Changes from execution process {}",
                ctx.execution_process.id
            ),
        }
    }

    /// Check which repos have uncommitted changes. Fails if any repo is inaccessible.
    fn check_repos_for_changes(
        workspace_root: &Path,
        repos: &[Repo],
    ) -> Result<Vec<(Repo, PathBuf)>, ContainerError> {
        let git = GitCli::new();
        let mut repos_with_changes = Vec::new();

        for repo in repos {
            let worktree_path = workspace_root.join(&repo.name);

            match git.has_changes(&worktree_path) {
                Ok(true) => {
                    repos_with_changes.push((repo.clone(), worktree_path));
                }
                Ok(false) => {
                    tracing::debug!("No changes in repo '{}'", repo.name);
                }
                Err(e) => {
                    return Err(ContainerError::Other(anyhow!(
                        "Pre-flight check failed for repo '{}': {}",
                        repo.name,
                        e
                    )));
                }
            }
        }

        Ok(repos_with_changes)
    }

    /// Commit changes to each repo. Logs failures but continues with other repos.
    fn commit_repos(&self, repos_with_changes: Vec<(Repo, PathBuf)>, message: &str) -> bool {
        let mut any_committed = false;

        for (repo, worktree_path) in repos_with_changes {
            tracing::debug!(
                "Committing changes for repo '{}' at {:?}",
                repo.name,
                &worktree_path
            );

            match self.git().commit(&worktree_path, message) {
                Ok(true) => {
                    any_committed = true;
                    tracing::info!("Committed changes in repo '{}'", repo.name);
                }
                Ok(false) => {
                    tracing::warn!("No changes committed in repo '{}' (unexpected)", repo.name);
                }
                Err(e) => {
                    tracing::warn!("Failed to commit in repo '{}': {}", repo.name, e);
                }
            }
        }

        any_committed
    }

    /// Spawn a background task that polls the child process for completion and
    /// cleans up the execution entry when it exits.
    pub fn spawn_exit_monitor(
        &self,
        exec_id: &Uuid,
        exit_signal: Option<ExecutorExitSignal>,
    ) -> JoinHandle<()> {
        let exec_id = *exec_id;
        let child_store = self.child_store.clone();
        let msg_stores = self.msg_stores.clone();
        let db = self.db.clone();
        let container = self.clone();

        let mut process_exit_rx = self.spawn_os_exit_watcher(exec_id);

        tokio::spawn(async move {
            let mut exit_signal_future = exit_signal.map_or_else(
                || std::future::pending().boxed(), // no signal, stall forever
                futures::FutureExt::boxed,         // wait for result
            );

            let status_result: std::io::Result<std::process::ExitStatus>;

            // Wait for process to exit, or exit signal from executor
            tokio::select! {
                // Exit signal with result.
                // Some coding agent processes do not automatically exit after processing the user request; instead the executor
                // signals when processing has finished to gracefully kill the process.
                exit_result = &mut exit_signal_future => {
                    // Executor signaled completion: kill group and use the provided result.
                    // Pre-scope the `child_store` read guard so it is dropped BEFORE the
                    // multi-second `kill_process_group` (which holds the per-child write
                    // lock + sleeps): holding the shared map read lock across the kill
                    // would stall every other execution-process registration/lookup
                    // behind a queued writer (write-preferring RwLock). Mirrors the
                    // sibling pattern in `spawn_os_exit_watcher` / the interactive poll.
                    let child_lock = {
                        let map = child_store.read().await;
                        map.get(&exec_id).cloned()
                    };
                    if let Some(child_lock) = child_lock {
                        let mut child = child_lock.write().await;
                        if let Err(err) = command::kill_process_group(&mut child).await {
                            tracing::error!("Failed to kill process group after exit signal: {} {}", exec_id, err);
                        }
                    }

                    // Map the exit result to appropriate exit status
                    status_result = match exit_result {
                        Ok(ExecutorExitResult::Failure) => Ok(failure_exit_status()),
                        Ok(ExecutorExitResult::Success) | Err(_) => {
                            Ok(success_exit_status()) // Channel closed, assume success
                        }
                    };
                }
                // Process exit
                exit_status_result = &mut process_exit_rx => {
                    status_result = exit_status_result.unwrap_or_else(|e| Err(std::io::Error::other(e)));
                }
            }

            let (exit_code, status) = match status_result {
                Ok(exit_status) => {
                    let code = i64::from(exit_status.code().unwrap_or(-1));
                    let status = if exit_status.success() {
                        ExecutionProcessStatus::Completed
                    } else {
                        ExecutionProcessStatus::Failed
                    };
                    (Some(code), status)
                }
                Err(_) => (None, ExecutionProcessStatus::Failed),
            };

            if !ExecutionProcess::was_stopped(&db.pool, exec_id).await
                && let Err(e) =
                    ExecutionProcess::update_completion(&db.pool, exec_id, status, exit_code).await
            {
                tracing::error!("Failed to update execution process completion: {}", e);
            }

            if let Ok(ctx) = ExecutionProcess::load_context(&db.pool, exec_id).await {
                // Update executor session summary if available
                if let Err(e) = container.update_executor_session_summary(&exec_id).await {
                    tracing::warn!("Failed to update executor session summary: {}", e);
                }

                let success = matches!(
                    ctx.execution_process.status,
                    ExecutionProcessStatus::Completed
                ) && exit_code == Some(0);

                let cleanup_done = matches!(
                    ctx.execution_process.run_reason,
                    ExecutionProcessRunReason::CleanupScript
                ) && !matches!(
                    ctx.execution_process.status,
                    ExecutionProcessStatus::Running
                );

                if success || cleanup_done {
                    // Commit changes (if any) and get feedback about whether changes were made
                    let changes_committed = match container.try_commit_changes(&ctx).await {
                        Ok(committed) => committed,
                        Err(e) => {
                            tracing::error!("Failed to commit changes after execution: {}", e);
                            // Treat commit failures as if changes were made to be safe
                            true
                        }
                    };

                    let should_start_next = if matches!(
                        ctx.execution_process.run_reason,
                        ExecutionProcessRunReason::CodingAgent
                    ) {
                        changes_committed
                    } else {
                        true
                    };

                    if should_start_next {
                        // If the process exited successfully, start the next action
                        if let Err(e) = container.try_start_next_action(&ctx).await {
                            tracing::error!("Failed to start next action after completion: {}", e);
                        }
                    } else {
                        tracing::info!(
                            "Skipping cleanup script for workspace {} - no changes made by coding agent",
                            ctx.workspace.id
                        );

                        // Manually finalize task since we're bypassing normal execution flow
                        container.finalize_task(&ctx).await;
                    }
                }

                if container.should_finalize(&ctx) {
                    // Only execute queued messages if the execution succeeded
                    // If it failed or was killed, just clear the queue and finalize
                    let should_execute_queued = !matches!(
                        ctx.execution_process.status,
                        ExecutionProcessStatus::Failed | ExecutionProcessStatus::Killed
                    );

                    if let Some(queued_msg) =
                        container.queued_message_service.take_queued(ctx.session.id)
                    {
                        if should_execute_queued {
                            tracing::info!(
                                "Found queued message for session {}, starting follow-up execution",
                                ctx.session.id
                            );

                            // Delete the scratch since we're consuming the queued message
                            if let Err(e) = Scratch::delete(
                                &db.pool,
                                ctx.session.id,
                                &ScratchType::DraftFollowUp,
                            )
                            .await
                            {
                                tracing::warn!(
                                    "Failed to delete scratch after consuming queued message: {}",
                                    e
                                );
                            }

                            // Execute the queued follow-up
                            if let Err(e) = container
                                .start_queued_follow_up(&ctx, &queued_msg.data)
                                .await
                            {
                                tracing::error!("Failed to start queued follow-up: {}", e);
                                // Fall back to finalization if follow-up fails
                                container.finalize_task(&ctx).await;
                            }
                        } else {
                            // Execution failed or was killed - discard the queued message and finalize
                            tracing::info!(
                                "Discarding queued message for session {} due to execution status {:?}",
                                ctx.session.id,
                                ctx.execution_process.status
                            );
                            container.finalize_task(&ctx).await;
                        }
                    } else {
                        container.finalize_task(&ctx).await;
                    }
                }

            }

            // Now that commit/next-action/finalization steps for this process are complete,
            // capture the HEAD OID as the definitive "after" state (best-effort).
            container.update_after_head_commits(exec_id).await;

            // Cleanup msg store
            if let Some(msg_arc) = msg_stores.write().await.remove(&exec_id) {
                msg_arc.push_finished();
                tokio::time::sleep(Duration::from_millis(50)).await; // Wait for the finish message to propogate
                match Arc::try_unwrap(msg_arc) {
                    Ok(inner) => drop(inner),
                    Err(arc) => tracing::error!(
                        "There are still {} strong Arcs to MsgStore for {}",
                        Arc::strong_count(&arc),
                        exec_id
                    ),
                }
            }

            // Cleanup child handle
            child_store.write().await.remove(&exec_id);
        })
    }

    pub fn spawn_os_exit_watcher(
        &self,
        exec_id: Uuid,
    ) -> tokio::sync::oneshot::Receiver<std::io::Result<std::process::ExitStatus>> {
        let (tx, rx) = tokio::sync::oneshot::channel::<std::io::Result<std::process::ExitStatus>>();
        let child_store = self.child_store.clone();
        tokio::spawn(async move {
            loop {
                let child_lock = {
                    let map = child_store.read().await;
                    map.get(&exec_id).cloned()
                };
                if let Some(child_lock) = child_lock {
                    let mut child_handler = child_lock.write().await;
                    match child_handler.try_wait() {
                        Ok(Some(status)) => {
                            let _ = tx.send(Ok(status));
                            break;
                        }
                        Ok(None) => {}
                        Err(e) => {
                            let _ = tx.send(Err(e));
                            break;
                        }
                    }
                } else {
                    let _ = tx.send(Err(io::Error::other(format!(
                        "Child handle missing for {exec_id}"
                    ))));
                    break;
                }
                tokio::time::sleep(Duration::from_millis(250)).await;
            }
        });
        rx
    }

    pub fn dir_name_from_workspace(workspace_id: &Uuid, task_title: &str) -> String {
        let task_title_id = git_branch_id(task_title);
        format!("{}-{}", short_uuid(workspace_id), task_title_id)
    }

    async fn track_child_msgs_in_store(
        &self,
        id: Uuid,
        child: &mut AsyncGroupChild,
    ) -> Result<(), ContainerError> {
        let store = Arc::new(MsgStore::new());

        let out = child
            .inner()
            .stdout
            .take()
            .ok_or_else(|| ContainerError::Other(anyhow!("child has no stdout")))?;
        let err = child
            .inner()
            .stderr
            .take()
            .ok_or_else(|| ContainerError::Other(anyhow!("child has no stderr")))?;

        // Map stdout bytes -> LogMsg::Stdout
        let out = ReaderStream::new(out)
            .map_ok(|chunk| LogMsg::Stdout(String::from_utf8_lossy(&chunk).into_owned()));

        // Map stderr bytes -> LogMsg::Stderr
        let err = ReaderStream::new(err)
            .map_ok(|chunk| LogMsg::Stderr(String::from_utf8_lossy(&chunk).into_owned()));

        // If you have a JSON Patch source, map it to LogMsg::JsonPatch too, then select all three.

        // Merge and forward into the store
        let merged = select(out, err); // Stream<Item = Result<LogMsg, io::Error>>
        store.clone().spawn_forwarder(merged);

        let mut map = self.msg_stores().write().await;
        map.insert(id, store);
        Ok(())
    }

    /// S5 (no-`-p` interactive transport): spawn a transcript tailer that feeds
    /// the genuine `claude` interactive session's on-disk JSONL into a fresh
    /// per-execution `MsgStore`, then runs the EXISTING `ClaudeLogProcessor`
    /// pipeline over it unchanged.
    ///
    /// Contract (see docs/developed/plans/2026-06-15-no-p-interactive-transport.md):
    /// - push `LogMsg::SessionId(uuid)` immediately (known a priori),
    /// - tail the JSONL and push each COMPLETE (newline-terminated) line as
    ///   `LogMsg::Stdout` — byte-identical to how `track_child_msgs_in_store`
    ///   maps child stdout, so `ClaudeLogProcessor` parses it with zero changes,
    /// - push `LogMsg::Finished` when a `type=system,subtype=turn_duration` line
    ///   is seen AND a short idle debounce elapses; the PROBE found that marker
    ///   may not appear in 2.1.177, so an idle-timeout fallback also drives
    ///   `Finished` (NOT `type=result` — it never appears).
    ///
    /// Returns the registered `MsgStore` so callers can subscribe to normalized
    /// entries exactly as for the `-p` path.
    ///
    /// NOTE: normalization (`ClaudeLogProcessor::process_logs`) is intentionally
    /// NOT run here. The interactive store is registered under `exec_id` in the
    /// shared `msg_stores` map, and the services-layer `start_execution` runs the
    /// SINGLE normalization pass over it (`executor.normalize_logs`, identical to
    /// the `-p` path). Running it here too would double-process the transcript
    /// and emit duplicate `/entries/N` patches. The tailer only ever pushes raw
    /// `Stdout`/`SessionId`/`Finished`, which is exactly what that single pass —
    /// and `spawn_stream_raw_logs_to_db` — consume.
    pub async fn spawn_interactive_transcript_tailer(
        &self,
        exec_id: Uuid,
        transcript_path: PathBuf,
        session_uuid: String,
        child_exited: Arc<std::sync::atomic::AtomicBool>,
    ) -> Result<Arc<MsgStore>, ContainerError> {
        let store = Arc::new(MsgStore::new());

        // Known a priori — emit before any transcript bytes (mirrors -p SessionId).
        store.push_session_id(session_uuid);

        // Tail the JSONL into the store on a background task.
        Self::spawn_transcript_tail_task(store.clone(), transcript_path, child_exited);

        let mut map = self.msg_stores().write().await;
        map.insert(exec_id, store.clone());
        Ok(store)
    }

    /// Background poll-tailer for an interactive session transcript JSONL.
    ///
    /// Polls every [`TRANSCRIPT_POLL_INTERVAL`], reads any newly-appended bytes
    /// (tracking a byte offset), and pushes each complete newline-terminated line
    /// as `LogMsg::Stdout`. This task is the SINGLE owner of `LogMsg::Finished`
    /// for the interactive transport: it pushes it on the `turn_duration`
    /// completion marker + idle debounce, when the completion watcher signals the
    /// genuine `claude` child has exited (`child_exited`), or — as the
    /// PROBE-verified last resort — on an idle timeout with no new bytes. In every
    /// case it first drains ALL remaining complete lines so the agent's final
    /// assistant message is never dropped ahead of `Finished`.
    fn spawn_transcript_tail_task(
        store: Arc<MsgStore>,
        transcript_path: PathBuf,
        child_exited: Arc<std::sync::atomic::AtomicBool>,
    ) -> JoinHandle<()> {
        tokio::spawn(async move {
            let mut offset: usize = 0;
            // Undecoded trailing bytes carried across polls: a 250ms poll routinely
            // lands mid multi-byte UTF-8 char (CJK/emoji), so we decode only up to
            // the last valid char boundary and retain the incomplete tail for the
            // next poll instead of lossily mangling it with per-slice from_utf8_lossy.
            let mut carry: Vec<u8> = Vec::new();
            let mut pending = String::new();
            let mut saw_turn_duration = false;
            // Ticks with no new bytes since the last data; resets on any new bytes.
            let mut idle_ticks: u32 = 0;

            loop {
                let mut got_new_bytes = false;

                match tokio::fs::read(&transcript_path).await {
                    Ok(bytes) => {
                        let total = bytes.len();
                        if total > offset {
                            // Only decode the freshly-appended tail, carrying any
                            // incomplete trailing multi-byte char to the next poll.
                            let fresh = &bytes[offset..];
                            offset = total;
                            got_new_bytes = true;
                            carry.extend_from_slice(fresh);
                            let valid = match std::str::from_utf8(&carry) {
                                Ok(s) => s.len(),
                                Err(e) => e.valid_up_to(),
                            };
                            if valid > 0 {
                                // `carry[..valid]` is valid UTF-8 by construction.
                                pending.push_str(
                                    std::str::from_utf8(&carry[..valid]).unwrap_or_default(),
                                );
                                carry.drain(..valid);
                            }

                            // Emit every complete (newline-terminated) line.
                            while let Some(nl) = pending.find('\n') {
                                let line: String = pending.drain(..=nl).collect();
                                let trimmed = line.trim_end_matches(['\n', '\r']);
                                if trimmed.is_empty() {
                                    continue;
                                }
                                if Self::is_turn_duration_marker(trimmed) {
                                    saw_turn_duration = true;
                                }
                                store.push_stdout(line);
                            }
                        } else if total < offset {
                            // File truncated/rotated (e.g. resume rewrote it) — restart.
                            offset = 0;
                            carry.clear();
                            pending.clear();
                            got_new_bytes = true;
                        }
                    }
                    Err(e) if e.kind() == io::ErrorKind::NotFound => {
                        // Transcript not created yet — keep waiting (claude writes it
                        // a moment after launch). Counts as idle.
                    }
                    Err(e) => {
                        tracing::warn!(
                            transcript = %transcript_path.display(),
                            error = %e,
                            "interactive transcript tail read failed; will retry"
                        );
                    }
                }

                if got_new_bytes {
                    idle_ticks = 0;
                } else {
                    idle_ticks = idle_ticks.saturating_add(1);
                }

                // Completion: the child has exited (authoritative — the completion
                // watcher set the latch), OR the `turn_duration` marker + short idle
                // debounce, OR a longer idle timeout with no activity (PROBE: marker
                // may be absent in 2.1.177).
                let exited = child_exited.load(std::sync::atomic::Ordering::Acquire);
                let marker_done = saw_turn_duration && idle_ticks >= TRANSCRIPT_MARKER_DEBOUNCE_TICKS;
                let idle_done = idle_ticks >= TRANSCRIPT_IDLE_TIMEOUT_TICKS;
                if exited || marker_done || idle_done {
                    // On a child-exit signal, do one final full read so any complete
                    // lines written between the last poll and exit are drained BEFORE
                    // `Finished` (the watcher no longer pushes `Finished` itself).
                    if exited && let Ok(bytes) = tokio::fs::read(&transcript_path).await {
                        let total = bytes.len();
                        if total > offset {
                            carry.extend_from_slice(&bytes[offset..]);
                        }
                        // Decode all remaining bytes (lossy on a genuinely torn tail
                        // at EOF — the process is gone, nothing more will arrive).
                        if !carry.is_empty() {
                            pending.push_str(&String::from_utf8_lossy(&carry));
                            carry.clear();
                        }
                        while let Some(nl) = pending.find('\n') {
                            let line: String = pending.drain(..=nl).collect();
                            if line.trim_end_matches(['\n', '\r']).is_empty() {
                                continue;
                            }
                            store.push_stdout(line);
                        }
                    }
                    // Flush any trailing line that never got a newline.
                    let tail = pending.trim_end_matches(['\n', '\r']);
                    if !tail.is_empty() {
                        store.push_stdout(std::mem::take(&mut pending));
                    }
                    store.push_finished();
                    tracing::debug!(
                        transcript = %transcript_path.display(),
                        exited,
                        marker_done,
                        idle_done,
                        "interactive transcript tail complete"
                    );
                    break;
                }

                tokio::time::sleep(TRANSCRIPT_POLL_INTERVAL).await;
            }
        })
    }

    /// True if a transcript line is the `type=system,subtype=turn_duration`
    /// completion marker. Parses leniently (the line is a full camelCase envelope;
    /// only the two top-level fields matter here).
    fn is_turn_duration_marker(line: &str) -> bool {
        serde_json::from_str::<serde_json::Value>(line)
            .ok()
            .is_some_and(|v| {
                v.get("type").and_then(|t| t.as_str()) == Some("system")
                    && v.get("subtype").and_then(|s| s.as_str()) == Some("turn_duration")
            })
    }

    /// Guarded entry for the no-`-p` interactive transport (native-OAuth only).
    ///
    /// Spawns the GENUINE `claude` binary interactively per the PROBE mechanics:
    /// a PIPED one-shot (NO PTY needed for a single turn) with stdin closed, which
    /// makes claude run exactly ONE turn and exit cleanly (RC=0) after writing the
    /// transcript. NO `-p`, NO stream-json, NO control protocol — this path never
    /// touches `ProtocolPeer` and so never draws on the Agent SDK credit pool.
    ///
    /// `command_parts` come from `ClaudeCode::build_interactive_command_parts()`
    /// (initial) or `build_interactive_follow_up_command_parts()` (follow-up,
    /// `--resume` WITHOUT `--fork-session`). `env` MUST include the isolated home
    /// vars (`CLAUDE_CONFIG_DIR` + `CLAUDE_HOME`) from cc_switch's interactive
    /// home so the transcript + credentials are redirected (PROBE: `CLAUDE_HOME`
    /// alone is a no-op in 2.1.177; `CLAUDE_CONFIG_DIR` is the real redirect).
    /// `transcript_path` + `session_uuid` come from
    /// `cc_switch::InteractiveHome` and drive the S5 tailer.
    ///
    /// Selection wiring (choosing this path for native-OAuth users) is deferred to
    /// S6; this function is the exposed building block and is NOT yet on any
    /// default code path.
    #[allow(clippy::too_many_arguments)]
    pub async fn spawn_interactive_claude(
        &self,
        exec_id: Uuid,
        command_parts: (PathBuf, Vec<String>),
        prompt: String,
        working_dir: PathBuf,
        env: HashMap<String, String>,
        env_unset: Vec<String>,
        transcript_path: PathBuf,
        session_uuid: String,
    ) -> Result<(Arc<MsgStore>, AsyncGroupChild), ContainerError> {
        use std::process::Stdio;

        use command_group::AsyncCommandGroup;

        let (program, mut args) = command_parts;
        // The prompt is passed positionally to interactive `claude` (PROBE: the
        // bare positional argument is the single-turn prompt).
        args.push(prompt);

        let mut command = tokio::process::Command::new(&program);
        command
            .kill_on_drop(true)
            // Closed/empty stdin => non-TTY one-turn-then-exit (PROBE-verified).
            .stdin(Stdio::null())
            // The authoritative log source for this path is the on-disk transcript
            // tailer (the genuine `claude` writes structured output to the JSONL,
            // NOT stdout); nothing ever drains these pipes. Piping them would let a
            // large turn (a final assistant message >64KB) fill the bounded OS pipe
            // buffer and block the child's write() forever — it would never finish
            // its turn, never exit, and the run would hang. `null` sidesteps the
            // (platform-dependent) pipe-buffer deadlock on both Linux and Windows.
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .current_dir(&working_dir)
            .args(&args);

        for key in &env_unset {
            command.env_remove(key);
        }
        for (key, value) in &env {
            command.env(key, value);
        }

        let child = command
            .group_spawn()
            .map_err(|e| ContainerError::Other(anyhow!("failed to spawn interactive claude: {e}")))?;

        // Shared child-exit latch coordinating the tailer and the completion
        // watcher. The watcher (which observes the genuine `claude` exit) SETS it;
        // the tailer OWNS `Finished`, so on seeing it set the tailer does one final
        // ordered transcript read and pushes `Finished` exactly once AFTER draining
        // every remaining line. This closes the race where `Finished` used to be
        // pushed by the watcher BEFORE the tailer drained the final (complete)
        // lines, silently dropping the agent's final assistant message.
        let child_exited = Arc::new(std::sync::atomic::AtomicBool::new(false));

        // Wire the transcript tailer to feed the per-execution MsgStore. The
        // genuine `claude` writes structured output to the on-disk JSONL (not
        // stdout), so the tailer — not the child's stdout — is the log source.
        let _ = working_dir; // working_dir no longer needed by the tailer (single normalization pass lives in the services layer)
        let store = self
            .spawn_interactive_transcript_tailer(
                exec_id,
                transcript_path,
                session_uuid,
                child_exited.clone(),
            )
            .await?;

        // S6 completion-on-exit: the genuine `claude` piped one-shot emits NO
        // `type=result`/`turn_duration` in 2.1.177, and a short transcript idle
        // can fire mid-long-tool. Drive `Finished` off the child's ACTUAL exit:
        // when the child is reaped, SIGNAL the tailer (which then does the final
        // read + push of `Finished`). The tailer's idle timeout remains only as a
        // long (~120s) safety net should this signal ever be lost.
        self.spawn_interactive_completion_watcher(exec_id, child_exited);

        Ok((store, child))
    }

    /// S6 completion-on-exit: SIGNAL the transcript tailer as soon as the
    /// interactive `claude` child process exits (the reliable completion signal
    /// for the piped one-shot — `type=result`/`turn_duration` never appear in
    /// 2.1.177).
    ///
    /// Polls `child_store` for `exec_id` (mirrors `spawn_os_exit_watcher`'s
    /// pattern) and observes the cached exit status via `try_wait`, so it does
    /// NOT race the exit monitor's own `try_wait` (command-group caches the
    /// status; both pollers converge on `Some`). On exit it sets the shared
    /// `child_exited` latch and returns: the tailer (the SINGLE owner of
    /// `Finished`) then does one final ordered transcript read — draining every
    /// remaining COMPLETE line plus the unterminated tail — before pushing
    /// `Finished` exactly once. Pushing `Finished` here instead would race the
    /// tailer and could land BEFORE those final lines were drained (all consumers
    /// stop at the first `Finished`), silently dropping the agent's final message.
    fn spawn_interactive_completion_watcher(
        &self,
        exec_id: Uuid,
        child_exited: Arc<std::sync::atomic::AtomicBool>,
    ) -> JoinHandle<()> {
        let child_store = self.child_store.clone();
        tokio::spawn(async move {
            // Wait for the child to be registered (the caller adds it to
            // child_store immediately after spawn_interactive_claude returns),
            // then poll for its exit. Bail out if it never appears.
            let mut waited_for_registration: u32 = 0;
            loop {
                let child_lock = {
                    let map = child_store.read().await;
                    map.get(&exec_id).cloned()
                };
                if let Some(child_lock) = child_lock {
                    let exited = {
                        let mut child = child_lock.write().await;
                        matches!(child.try_wait(), Ok(Some(_)) | Err(_))
                    };
                    if exited {
                        break;
                    }
                } else {
                    // Not yet registered (or already cleaned up). Give the
                    // caller a brief grace window, then give up so the tailer
                    // safety-net handles completion.
                    waited_for_registration = waited_for_registration.saturating_add(1);
                    if waited_for_registration > 40 {
                        tracing::debug!(
                            exec_id = %exec_id,
                            "interactive completion watcher: child never registered; \
                             deferring to tailer idle safety-net"
                        );
                        return;
                    }
                }
                tokio::time::sleep(TRANSCRIPT_POLL_INTERVAL).await;
            }

            // Signal the tailer to do its final ordered flush + push `Finished`.
            // `Release` pairs with the tailer's `Acquire` load so the tailer
            // observes the child's last transcript writes.
            child_exited.store(true, std::sync::atomic::Ordering::Release);
            tracing::debug!(
                exec_id = %exec_id,
                "interactive transport: signaled tailer on child exit"
            );
        })
    }

    /// Create a live diff log stream for ongoing attempts for WebSocket
    /// Returns a stream that owns the filesystem watcher - when dropped, watcher is cleaned up
    fn create_live_diff_stream(
        args: &diff_stream::DiffStreamArgs,
    ) -> Result<DiffStreamHandle, ContainerError> {
        diff_stream::create(args).map_err(|e| ContainerError::Other(anyhow!("{e}")))
    }

    /// Extract the last assistant message from the MsgStore history
    fn extract_last_assistant_message(&self, exec_id: &Uuid) -> Option<String> {
        // Get the MsgStore for this execution
        let msg_stores = match self.msg_stores.try_read() {
            Ok(guard) => guard,
            Err(e) => {
                tracing::debug!(
                    exec_id = %exec_id,
                    error = %e,
                    "msg_stores try_read failed; skipping last assistant message extraction"
                );
                return None;
            }
        };
        let msg_store = msg_stores.get(exec_id)?;

        // Get the history and scan in reverse for the last assistant message
        let history = msg_store.get_history();

        for msg in history.iter().rev() {
            if let LogMsg::JsonPatch(patch) = msg {
                // Try to extract a NormalizedEntry from the patch
                if let Some((_, entry)) = extract_normalized_entry_from_patch(patch)
                    && matches!(entry.entry_type, NormalizedEntryType::AssistantMessage)
                {
                    let content = entry.content.trim();
                    if !content.is_empty() {
                        const MAX_SUMMARY_LENGTH: usize = 4096;
                        if content.len() > MAX_SUMMARY_LENGTH {
                            let truncated = truncate_to_char_boundary(content, MAX_SUMMARY_LENGTH);
                            return Some(format!("{truncated}..."));
                        }
                        return Some(content.to_string());
                    }
                }
            }
        }

        None
    }

    /// Update the coding agent turn summary with the final assistant message
    async fn update_executor_session_summary(&self, exec_id: &Uuid) -> Result<(), anyhow::Error> {
        // Check if there's a coding agent turn for this execution process
        let turn = CodingAgentTurn::find_by_execution_process_id(&self.db.pool, *exec_id).await?;

        if let Some(turn) = turn {
            // Only update if summary is not already set
            if turn.summary.is_none() {
                if let Some(summary) = self.extract_last_assistant_message(exec_id) {
                    CodingAgentTurn::update_summary(&self.db.pool, *exec_id, &summary).await?;
                } else {
                    tracing::debug!("No assistant message found for execution {}", exec_id);
                }
            }
        }

        Ok(())
    }

    /// Copy project files and images to the workspace.
    /// Skips files/images that already exist (fast no-op if all exist).
    async fn copy_files_and_images(
        &self,
        workspace_dir: &Path,
        workspace: &Workspace,
    ) -> Result<(), ContainerError> {
        let repos = WorkspaceRepo::find_repos_with_copy_files(&self.db.pool, workspace.id).await?;

        for repo in &repos {
            if let Some(copy_files) = &repo.copy_files
                && !copy_files.trim().is_empty()
            {
                let worktree_path = workspace_dir.join(&repo.name);
                self.copy_project_files(&repo.path, &worktree_path, copy_files)
                    .await
                    .unwrap_or_else(|e| {
                        tracing::warn!(
                            "Failed to copy project files for repo '{}': {}",
                            repo.name,
                            e
                        );
                    });
            }
        }

        if let Err(e) = self
            .image_service
            .copy_images_by_task_to_worktree(
                workspace_dir,
                workspace.task_id,
                workspace.agent_working_dir.as_deref(),
            )
            .await
        {
            tracing::warn!("Failed to copy task images to workspace: {}", e);
        }

        Ok(())
    }

    /// Create workspace-level CLAUDE.md and AGENTS.md files that import from each repo.
    /// Uses the @import syntax to reference each repo's config files.
    /// Skips creating files if they already exist or if no repos have the source file.
    async fn create_workspace_config_files(
        workspace_dir: &Path,
        repos: &[Repo],
    ) -> Result<(), ContainerError> {
        const CONFIG_FILES: [&str; 2] = ["CLAUDE.md", "AGENTS.md"];

        for config_file in CONFIG_FILES {
            let workspace_config_path = workspace_dir.join(config_file);

            if workspace_config_path.exists() {
                tracing::debug!(
                    "Workspace config file {} already exists, skipping",
                    config_file
                );
                continue;
            }

            let mut import_lines = Vec::new();
            for repo in repos {
                let repo_config_path = workspace_dir.join(&repo.name).join(config_file);
                if repo_config_path.exists() {
                    import_lines.push(format!("@{}/{}", repo.name, config_file));
                }
            }

            if import_lines.is_empty() {
                tracing::debug!(
                    "No repos have {}, skipping workspace config creation",
                    config_file
                );
                continue;
            }

            let content = import_lines.join("\n") + "\n";
            if let Err(e) = tokio::fs::write(&workspace_config_path, &content).await {
                tracing::warn!(
                    "Failed to create workspace config file {}: {}",
                    config_file,
                    e
                );
                continue;
            }

            tracing::info!(
                "Created workspace {} with {} import(s)",
                config_file,
                import_lines.len()
            );
        }

        Ok(())
    }

    /// Start a follow-up execution from a queued message
    async fn start_queued_follow_up(
        &self,
        ctx: &ExecutionContext,
        queued_data: &DraftFollowUpData,
    ) -> Result<ExecutionProcess, ContainerError> {
        // Get executor from the latest CodingAgent process, or fall back to session's executor
        let base_executor = if let Some(profile) =
            ExecutionProcess::latest_executor_profile_for_session(&self.db.pool, ctx.session.id)
                .await
                .map_err(|e| {
                    ContainerError::Other(anyhow!("Failed to get executor profile: {e}"))
                })? {
            profile.executor
        } else {
            // No prior execution - use session's executor field
            let executor_str = ctx.session.executor.as_ref().ok_or_else(|| {
                ContainerError::Other(anyhow!(
                    "No prior execution and no executor configured on session"
                ))
            })?;
            BaseCodingAgent::from_str(&executor_str.replace('-', "_").to_ascii_uppercase())
                .map_err(|_| ContainerError::Other(anyhow!("Invalid executor: {executor_str}")))?
        };

        let executor_profile_id = ExecutorProfileId {
            executor: base_executor,
            variant: queued_data.variant.clone(),
        };

        // Get latest agent session ID for session continuity (from coding agent turns)
        let latest_agent_session_id = ExecutionProcess::find_latest_coding_agent_turn_session_id(
            &self.db.pool,
            ctx.session.id,
        )
        .await?;

        let repos =
            WorkspaceRepo::find_repos_for_workspace(&self.db.pool, ctx.workspace.id).await?;
        let cleanup_action = self.cleanup_actions_for_repos(&repos);

        let working_dir = ctx
            .workspace
            .agent_working_dir
            .as_ref()
            .filter(|dir| !dir.is_empty())
            .cloned();

        let action_type = if let Some(agent_session_id) = latest_agent_session_id {
            ExecutorActionType::CodingAgentFollowUpRequest(CodingAgentFollowUpRequest {
                prompt: queued_data.message.clone(),
                session_id: agent_session_id,
                executor_profile_id: executor_profile_id.clone(),
                working_dir: working_dir.clone(),
                allow_user_questions: true,
            })
        } else {
            ExecutorActionType::CodingAgentInitialRequest(CodingAgentInitialRequest {
                prompt: queued_data.message.clone(),
                executor_profile_id: executor_profile_id.clone(),
                working_dir,
                allow_user_questions: true,
            })
        };

        let action = ExecutorAction::new(action_type, cleanup_action.map(Box::new));

        self.start_execution(
            &ctx.workspace,
            &ctx.session,
            &action,
            &ExecutionProcessRunReason::CodingAgent,
        )
        .await
    }
}

fn failure_exit_status() -> std::process::ExitStatus {
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;
        ExitStatusExt::from_raw(256) // Exit code 1 (shifted by 8 bits)
    }
    #[cfg(windows)]
    {
        use std::os::windows::process::ExitStatusExt;
        ExitStatusExt::from_raw(1)
    }
}

impl LocalContainerService {
    /// Interactive transport router (ALL ClaudeCode modes: native-OAuth,
    /// official-key, AND relay).
    ///
    /// Returns `Ok(true)` when the run was spawned via the no-`-p` interactive
    /// transport (caller must `return` — execution is fully wired). Returns
    /// `Ok(false)` to fall through to the existing `-p` path UNCHANGED.
    ///
    /// Selection criteria (all must hold):
    /// - `SOLODAWN_NO_POOL` is NOT set (kill-switch keeps the `-p` path),
    /// - the action is a ClaudeCode coding-agent request (initial or follow-up).
    ///
    /// Unlike the original S6 gate, this NO LONGER restricts to native OAuth.
    /// The resolved `(api_key, base_url)` (from the SAME `ModelConfig` resolution
    /// the `-p` path uses — WHICH credential a user gets is unchanged) selects the
    /// per-mode auth setup in `cc_switch::setup_interactive_auth`:
    /// native (no key) -> subscription plan; key (no base_url) -> pay-as-you-go;
    /// key+base_url -> relay endpoint. Only `SOLODAWN_NO_POOL` (and non-ClaudeCode
    /// executors) falls through to the unchanged `-p` path.
    ///
    /// See docs/developed/plans/2026-06-15-no-p-interactive-transport.md.
    async fn try_spawn_interactive_native_oauth(
        &self,
        exec_id: Uuid,
        current_dir: &Path,
        executor_action: &ExecutorAction,
        model_config_id: Option<&str>,
    ) -> Result<bool, ContainerError> {
        // Kill-switch: keep the proven `-p` path (accepts pool draw).
        if std::env::var_os("SOLODAWN_NO_POOL").is_some() {
            return Ok(false);
        }

        // Only ClaudeCode coding-agent requests are eligible. Extract the
        // prompt, optional working-dir, optional resume session-id, and profile.
        let (prompt, working_dir_rel, follow_up_session_id, profile_id) = match executor_action
            .typ()
        {
            ExecutorActionType::CodingAgentInitialRequest(req) => (
                req.prompt.clone(),
                req.working_dir.clone(),
                None,
                req.executor_profile_id.clone(),
            ),
            ExecutorActionType::CodingAgentFollowUpRequest(req) => (
                req.prompt.clone(),
                req.working_dir.clone(),
                Some(req.session_id.clone()),
                req.executor_profile_id.clone(),
            ),
            // Review runs are ALSO a ClaudeCode execution entry point. The
            // acceptance criterion requires EVERY entry point (initial,
            // follow-up, AND review) to use the no-`-p` interactive transport —
            // a review left on `-p` would leak a subscription user to the Agent
            // SDK credit. A review with a `session_id` resumes that session
            // (`--resume`); without one it starts fresh (`--session-id`),
            // mirroring `ReviewRequest`'s own spawn dispatch.
            ExecutorActionType::ReviewRequest(req) => (
                req.prompt.clone(),
                req.working_dir.clone(),
                req.session_id.clone(),
                req.executor_profile_id.clone(),
            ),
            ExecutorActionType::ScriptRequest(_) => return Ok(false),
        };
        if profile_id.executor != BaseCodingAgent::ClaudeCode {
            return Ok(false);
        }

        // Resolve the SAME `(api_key, base_url)` the `-p` path resolves so the
        // user gets the IDENTICAL credential — only the transport changes. The
        // `ModelConfig` query has no dir side effects (unlike
        // resolve_executor_env_vars). `model_for_settings` mirrors the model the
        // executor profile passes via `--model`, falling back to the model_config.
        let model_config = db::models::ModelConfig::resolve_preferred_or_default(
            &self.db.pool,
            model_config_id,
            "cli-claude-code",
        )
        .await
        .ok()
        .flatten();

        // Native-credential signal: the `.credentials.json` file is the correct
        // native marker (both `build_launch_config` and `setup_interactive_auth`
        // key on it), NOT `config.json`. Resolve it up front because it gates the
        // credential-precedence decision below.
        let native_claude_dir = dirs::home_dir().map(|h| h.join(".claude"));
        let has_native_creds = native_claude_dir
            .as_ref()
            .is_some_and(|d| d.join(".credentials.json").exists());

        // CREDENTIAL PRECEDENCE (must mirror the `-p` paths so WHICH credential a
        // user gets is unchanged — HARD RULE 5). The `-p` workspace path
        // (`resolve_executor_env_vars`) and the canonical `build_launch_config`
        // both prefer global/native auth and use a stored model_config key ONLY as
        // a fallback. `resolve_preferred_or_default` with `config_id = None` falls
        // THROUGH to `find_with_credentials_for_cli`, which returns ANY config
        // with a non-null key (e.g. a saved-but-unselected api-key/relay config) —
        // so a subscription user with native creds plus any saved key would be
        // wrongly routed to OfficialKey/Relay (pay-as-you-go / relay) instead of
        // their subscription plan. To match the `-p` precedence, treat a stored
        // key as authoritative ONLY when the config was EXPLICITLY selected
        // (`model_config_id.is_some()`). On a fallthrough (`None`), prefer native
        // whenever native creds exist and ignore the fallen-through key.
        let key_is_authoritative = model_config_id.is_some() || !has_native_creds;
        let resolved_api_key = if key_is_authoritative {
            model_config
                .as_ref()
                .and_then(|m| m.get_api_key().ok().flatten())
        } else {
            None
        };
        let resolved_base_url = if key_is_authoritative {
            model_config.as_ref().and_then(|m| m.base_url.clone())
        } else {
            None
        };

        // Native mode requires the genuine OAuth credentials file to exist; if a
        // user has neither an api-key nor native creds, there is nothing to
        // authenticate with via the interactive transport -> fall back to `-p`
        // (which has its own fallback chain). api-key / relay modes do not need
        // the native creds file.
        if resolved_api_key.is_none() && !has_native_creds {
            return Ok(false);
        }

        // Resolve the effective working directory (validates path traversal).
        let working_dir = executors::actions::validate_working_dir(current_dir, &working_dir_rel)
            .map_err(|e| ContainerError::Other(anyhow!("invalid working_dir: {e}")))?;

        // Fetch the ClaudeCode executor config (model, router, cmd overrides).
        let Some(CodingAgent::ClaudeCode(claude_cfg)) =
            ExecutorConfigs::get_cached().get_coding_agent(&profile_id)
        else {
            return Ok(false);
        };

        // Model for settings.json (non-native modes): prefer the executor
        // profile's model (== `--model`), else the model_config's api_model_id /
        // name. (Native mode ignores `model` for auth but it is still threaded.)
        let model_for_settings = claude_cfg
            .model
            .clone()
            .or_else(|| model_config.as_ref().and_then(|m| m.api_model_id.clone()))
            .or_else(|| model_config.as_ref().map(|m| m.name.clone()))
            .unwrap_or_default();

        // Provision (or re-open) the interactive home. Native-OAuth
        // (subscription, no resolved api key) reuses the user's GLOBAL `~/.claude`
        // — already authorized AND onboarded — so the worker terminal does not hit
        // a fresh-config login prompt + first-run model picker (the isolated copy
        // only carried `.credentials.json`/`settings.json`, never the
        // `~/.claude.json` onboarding/account state). API-key / relay modes keep an
        // isolated per-session home (their key is a secret that must be scrubbed on
        // teardown). On a follow-up, reuse the same session UUID so `--resume`
        // appends to the same transcript.
        let use_global_home = resolved_api_key.is_none();
        let home = if use_global_home {
            services::services::cc_switch::create_interactive_global_home(
                follow_up_session_id.as_deref(),
                &working_dir,
            )
        } else {
            services::services::cc_switch::create_interactive_isolated_home(
                follow_up_session_id.as_deref(),
                &working_dir,
            )
        }
        .map_err(|e| ContainerError::Other(anyhow!("create interactive home failed: {e}")))?;

        // Unified 3-mode auth setup: writes per-mode files into the home (native
        // creds copy / config.json + settings.json) and returns the env set/unset
        // map + optional `--settings` path. Reuses the SAME cc_switch credential
        // constructions as the `-p` path; scrubs the other modes' auth vars so a
        // stray ambient var cannot redirect billing.
        let native_src = native_claude_dir.unwrap_or_else(|| std::path::PathBuf::from(".claude"));
        let auth = services::services::cc_switch::setup_interactive_auth(
            &home,
            resolved_api_key.as_deref(),
            resolved_base_url.as_deref(),
            &model_for_settings,
            &native_src,
        )
        .map_err(|e| ContainerError::Other(anyhow!("interactive auth setup failed: {e}")))?;
        let env: HashMap<String, String> = auth.set;
        let env_unset: Vec<String> = auth.unset;

        // Build the interactive argv. Force interactive mode + the session UUID;
        // preserve the user's model and cmd overrides from the resolved config.
        // (Mutate a clone — `ClaudeCode` has private fields, so struct-update
        // syntax from outside its module is not possible; the interactive fields
        // are `pub` so direct assignment works.)
        let mut interactive_cfg = claude_cfg;
        interactive_cfg.interactive = Some(true);
        interactive_cfg.interactive_session_id = Some(home.session_uuid.clone());
        let command_parts = if follow_up_session_id.is_some() {
            interactive_cfg
                .build_interactive_follow_up_command_parts(&home.session_uuid)
                .map_err(|e| {
                    ContainerError::Other(anyhow!("build interactive follow-up argv failed: {e}"))
                })?
        } else {
            interactive_cfg
                .build_interactive_command_parts()
                .map_err(|e| ContainerError::Other(anyhow!("build interactive argv failed: {e}")))?
        };
        let (program, mut args) = command_parts
            .into_resolved()
            .await
            .map_err(ContainerError::ExecutorError)?;
        // Non-native modes pass `--settings <path>` (native OAuth omits it,
        // mirroring the `-p` path — `--settings`/key env are not used for native).
        if let Some(settings_path) = auth.settings_arg.as_ref() {
            args.push("--settings".to_string());
            args.push(settings_path.to_string_lossy().to_string());
        }
        let resolved = (program, args);

        tracing::info!(
            exec_id = %exec_id,
            session_uuid = %home.session_uuid,
            transcript = %home.transcript_path.display(),
            is_follow_up = follow_up_session_id.is_some(),
            auth_mode = ?auth.mode,
            "routing ClaudeCode run through interactive (no-`-p`) transport"
        );

        // Spawn the interactive child; this registers the per-execution MsgStore
        // (tailer pushes SessionId/Stdout/Finished) and wires completion-on-exit.
        let (_store, child) = self
            .spawn_interactive_claude(
                exec_id,
                resolved,
                prompt,
                working_dir,
                env,
                env_unset,
                home.transcript_path,
                home.session_uuid,
            )
            .await?;

        // Register the child + exit monitor exactly like the `-p` path so the
        // execution lifecycle (completion status, next-action chaining) is
        // unchanged. The SessionId pushed by the tailer is persisted to
        // coding_agent_turn.agent_session_id by the services-layer consumer, so
        // follow-ups resume the same logical session.
        self.add_child_to_store(exec_id, child).await;
        let _hn = self.spawn_exit_monitor(&exec_id, None);

        Ok(true)
    }

    /// Resolve executor-specific environment variables for workspace mode.
    ///
    /// In workflow mode, `CCSwitchService` handles this via isolated CODEX_HOME/auth setup.
    /// Workspace mode bypassed that path, so we inject the minimal set of env vars needed
    /// for the executor CLI to authenticate with the configured API provider.
    ///
    /// Priority chain:
    /// 1. Global CLI authentication (~/.claude/, ~/.codex/) — highest
    /// 2. model_config stored credentials — fallback
    /// 3. No credentials → log warning
    async fn resolve_executor_env_vars(
        base_executor: BaseCodingAgent,
        _executor_action: &ExecutorAction,
        pool: &sqlx::SqlitePool,
        model_config_id: Option<&str>,
    ) -> HashMap<String, String> {
        let mut vars = HashMap::new();

        if matches!(base_executor, BaseCodingAgent::ClaudeCode) {
            // Create an isolated CLAUDE_HOME and copy auth from global ~/.claude/
            let global_claude_home = dirs::home_dir()
                .map(|h| h.join(".claude"))
                .filter(|p| p.exists());

            let home_id = format!("ws-{}", uuid::Uuid::new_v4().as_simple());
            let claude_home = utils::path::get_solodawn_temp_dir()
                .join("claude-workspaces")
                .join(&home_id);

            if let Err(e) = std::fs::create_dir_all(&claude_home) {
                tracing::warn!(error = %e, "Failed to create workspace CLAUDE_HOME");
                return vars;
            }

            let mut has_global_auth = false;

            // Copy config.json, settings.json, and the native OAuth credential file
            // (.credentials.json) from global if they exist. The native-auth marker
            // is .credentials.json — mirroring build_launch_config /
            // setup_interactive_auth and has_native_creds above — NOT config.json,
            // which is only onboarding state and may not carry any key.
            if let Some(ref global_home) = global_claude_home {
                for filename in &["config.json", "settings.json", ".credentials.json"] {
                    let src = global_home.join(filename);
                    if src.exists() {
                        if let Err(e) = std::fs::copy(&src, claude_home.join(filename)) {
                            tracing::warn!(error = %e, file = filename, "Failed to copy Claude config to workspace");
                        }
                        if *filename == ".credentials.json" {
                            has_global_auth = true;
                        }
                    }
                }
            }

            // Fallback: inject credentials from model_config if no global auth
            if !has_global_auth {
                match db::models::ModelConfig::resolve_preferred_or_default(
                    pool,
                    model_config_id,
                    "cli-claude-code",
                )
                .await
                {
                    Ok(Some(model_config)) => {
                        if let Ok(Some(api_key)) = model_config.get_api_key() {
                            vars.insert("ANTHROPIC_API_KEY".to_string(), api_key.clone());
                            vars.insert("ANTHROPIC_AUTH_TOKEN".to_string(), api_key);
                            tracing::info!(
                                "Injected API key from model_config for Claude Code workspace"
                            );

                            if let Some(ref base_url) = model_config.base_url {
                                vars.insert("ANTHROPIC_BASE_URL".to_string(), base_url.clone());
                                tracing::info!(base_url = %base_url, "Injected base URL for Claude Code workspace");
                            }

                            // Write primaryApiKey into config.json for Claude Code CLI
                            let config_path = claude_home.join("config.json");
                            let mut config_json = if config_path.exists() {
                                std::fs::read_to_string(&config_path)
                                    .ok()
                                    .and_then(|s| {
                                        serde_json::from_str::<serde_json::Value>(&s).ok()
                                    })
                                    .unwrap_or_else(|| serde_json::json!({}))
                            } else {
                                serde_json::json!({})
                            };
                            if let Some(obj) = config_json.as_object_mut() {
                                obj.insert(
                                    "primaryApiKey".to_string(),
                                    serde_json::json!(vars.get("ANTHROPIC_API_KEY").unwrap()),
                                );
                                if let Some(base_url) = model_config.base_url.as_ref() {
                                    obj.insert(
                                        "apiBaseUrl".to_string(),
                                        serde_json::json!(base_url),
                                    );
                                }
                            }
                            if let Err(e) = std::fs::write(
                                &config_path,
                                serde_json::to_string_pretty(&config_json).unwrap_or_default(),
                            ) {
                                tracing::warn!(error = %e, "Failed to write Claude config.json with API key");
                            }
                        }
                    }
                    Ok(None) => {
                        tracing::warn!(
                            "No global Claude auth and no model_config credentials found for workspace mode"
                        );
                    }
                    Err(e) => {
                        tracing::warn!(error = %e, "Failed to query model_config credentials for Claude Code");
                    }
                }
            }

            vars.insert(
                "CLAUDE_HOME".to_string(),
                claude_home.to_string_lossy().to_string(),
            );
            tracing::info!(
                claude_home = %claude_home.display(),
                global_claude_home = ?global_claude_home,
                "Injected CLAUDE_HOME for workspace executor"
            );
        }

        if matches!(base_executor, BaseCodingAgent::Codex) {
            // Reuse the canonical codex_home() resolver from the executors crate.
            let global_codex_home = match executors::executors::codex::codex_home() {
                Some(p) if p.exists() => p,
                _ => {
                    // No global codex home — try model_config fallback
                    match db::models::ModelConfig::resolve_preferred_or_default(
                        pool,
                        model_config_id,
                        "cli-codex",
                    )
                    .await
                    {
                        Ok(Some(model_config)) => {
                            if let Ok(Some(api_key)) = model_config.get_api_key() {
                                vars.insert("OPENAI_API_KEY".to_string(), api_key);
                                tracing::info!(
                                    "Injected API key from model_config for Codex workspace"
                                );
                                if let Some(ref base_url) = model_config.base_url {
                                    vars.insert("OPENAI_BASE_URL".to_string(), base_url.clone());
                                }
                            }
                        }
                        Ok(None) => {
                            tracing::warn!(
                                "Codex home not found and no model_config credentials available"
                            );
                        }
                        Err(e) => {
                            tracing::warn!(error = %e, "Failed to query model_config credentials for Codex");
                        }
                    }
                    return vars;
                }
            };

            // Create an isolated CODEX_HOME and copy auth + config from global
            let home_id = format!("ws-{}", uuid::Uuid::new_v4().as_simple());
            let codex_home = utils::path::get_solodawn_temp_dir()
                .join("codex-workspaces")
                .join(&home_id);

            if let Err(e) = std::fs::create_dir_all(&codex_home) {
                tracing::warn!(error = %e, "Failed to create workspace CODEX_HOME");
                return vars;
            }

            // Copy auth.json
            let global_auth = global_codex_home.join("auth.json");
            if global_auth.exists() {
                if let Err(e) = std::fs::copy(&global_auth, codex_home.join("auth.json")) {
                    tracing::warn!(error = %e, "Failed to copy Codex auth.json to workspace");
                }
            }

            // Copy config.toml
            let global_config = global_codex_home.join("config.toml");
            if global_config.exists() {
                if let Err(e) = std::fs::copy(&global_config, codex_home.join("config.toml")) {
                    tracing::warn!(error = %e, "Failed to copy Codex config.toml to workspace");
                }
            }

            vars.insert(
                "CODEX_HOME".to_string(),
                codex_home.to_string_lossy().to_string(),
            );
            tracing::info!(
                codex_home = %codex_home.display(),
                global_codex_home = %global_codex_home.display(),
                "Injected CODEX_HOME for workspace executor (copied from global config)"
            );
        }

        if matches!(base_executor, BaseCodingAgent::Gemini) {
            // Inject Gemini credentials from model_config
            match db::models::ModelConfig::resolve_preferred_or_default(
                pool,
                model_config_id,
                "cli-gemini-cli",
            )
            .await
            {
                Ok(Some(model_config)) => {
                    if let Ok(Some(api_key)) = model_config.get_api_key() {
                        vars.insert("GEMINI_API_KEY".to_string(), api_key);
                        tracing::info!("Injected API key from model_config for Gemini workspace");
                        if let Some(ref base_url) = model_config.base_url {
                            vars.insert("GOOGLE_GEMINI_BASE_URL".to_string(), base_url.clone());
                        }
                    }
                }
                Ok(None) => {
                    tracing::debug!("No model_config credentials found for Gemini CLI");
                }
                Err(e) => {
                    tracing::warn!(error = %e, "Failed to query model_config credentials for Gemini");
                }
            }
        }

        vars
    }
}

#[async_trait]
impl ContainerService for LocalContainerService {
    fn msg_stores(&self) -> &Arc<RwLock<HashMap<Uuid, Arc<MsgStore>>>> {
        &self.msg_stores
    }

    fn db(&self) -> &DBService {
        &self.db
    }

    fn git(&self) -> &GitService {
        &self.git
    }

    fn notification_service(&self) -> &NotificationService {
        &self.notification_service
    }

    async fn git_branch_prefix(&self) -> String {
        self.config.read().await.git_branch_prefix.clone()
    }

    fn workspace_to_current_dir(&self, workspace: &Workspace) -> PathBuf {
        if let Some(path) = workspace.container_ref.clone() {
            PathBuf::from(path)
        } else {
            tracing::warn!(
                workspace_id = %workspace.id,
                "workspace has no container_ref; falling back to empty path"
            );
            PathBuf::new()
        }
    }

    async fn create(&self, workspace: &Workspace) -> Result<ContainerRef, ContainerError> {
        let task = workspace
            .parent_task(&self.db.pool)
            .await?
            .ok_or(sqlx::Error::RowNotFound)?;

        let workspace_dir_name =
            LocalContainerService::dir_name_from_workspace(&workspace.id, &task.title);
        let workspace_dir = WorkspaceManager::get_workspace_base_dir().join(&workspace_dir_name);

        let workspace_repos =
            WorkspaceRepo::find_by_workspace_id(&self.db.pool, workspace.id).await?;
        if workspace_repos.is_empty() {
            return Err(ContainerError::Other(anyhow!(
                "Workspace has no repositories configured"
            )));
        }

        let repositories =
            WorkspaceRepo::find_repos_for_workspace(&self.db.pool, workspace.id).await?;

        let target_branches: HashMap<_, _> = workspace_repos
            .iter()
            .map(|wr| (wr.repo_id, wr.target_branch.clone()))
            .collect();

        let workspace_inputs: Vec<RepoWorkspaceInput> = repositories
            .iter()
            .map(|repo| {
                let target_branch = if let Some(branch) = target_branches.get(&repo.id).cloned() {
                    branch
                } else {
                    tracing::warn!(
                        repo_id = %repo.id,
                        workspace_id = %workspace.id,
                        "no target_branch found for repo; using empty string"
                    );
                    String::new()
                };
                RepoWorkspaceInput::new(repo.clone(), target_branch)
            })
            .collect();

        let created_workspace = WorkspaceManager::create_workspace(
            &workspace_dir,
            &workspace_inputs,
            &workspace.branch,
        )
        .await?;

        // Copy project files and images to workspace
        self.copy_files_and_images(&created_workspace.workspace_dir, workspace)
            .await?;

        Self::create_workspace_config_files(&created_workspace.workspace_dir, &repositories)
            .await?;

        Workspace::update_container_ref(
            &self.db.pool,
            workspace.id,
            &created_workspace.workspace_dir.to_string_lossy(),
        )
        .await?;

        Ok(created_workspace
            .workspace_dir
            .to_string_lossy()
            .to_string())
    }

    async fn delete(&self, workspace: &Workspace) -> Result<(), ContainerError> {
        self.try_stop(workspace, true).await;
        Self::cleanup_workspace(&self.db, workspace).await;
        Ok(())
    }

    async fn ensure_container_exists(
        &self,
        workspace: &Workspace,
    ) -> Result<ContainerRef, ContainerError> {
        Workspace::touch(&self.db.pool, workspace.id).await?;
        // Use the target-branch-aware query so recovery can recreate lost branches.
        let repos_with_branch =
            WorkspaceRepo::find_repos_with_target_branch_for_workspace(&self.db.pool, workspace.id)
                .await?;

        if repos_with_branch.is_empty() {
            return Err(ContainerError::Other(anyhow!(
                "Workspace has no repositories configured"
            )));
        }

        let workspace_dir = if let Some(container_ref) = &workspace.container_ref {
            PathBuf::from(container_ref)
        } else {
            let task = workspace
                .parent_task(&self.db.pool)
                .await?
                .ok_or(sqlx::Error::RowNotFound)?;
            let workspace_dir_name =
                LocalContainerService::dir_name_from_workspace(&workspace.id, &task.title);
            WorkspaceManager::get_workspace_base_dir().join(&workspace_dir_name)
        };

        WorkspaceManager::ensure_workspace_exists_with_recovery(
            &workspace_dir,
            &repos_with_branch,
            &workspace.branch,
        )
        .await?;

        if workspace.container_ref.is_none() {
            Workspace::update_container_ref(
                &self.db.pool,
                workspace.id,
                &workspace_dir.to_string_lossy(),
            )
            .await?;
        }

        // Copy project files and images (fast no-op if already exist)
        self.copy_files_and_images(&workspace_dir, workspace)
            .await?;

        let repositories: Vec<Repo> = repos_with_branch.iter().map(|r| r.repo.clone()).collect();
        Self::create_workspace_config_files(&workspace_dir, &repositories).await?;

        Ok(workspace_dir.to_string_lossy().to_string())
    }

    async fn is_container_clean(&self, workspace: &Workspace) -> Result<bool, ContainerError> {
        let Some(container_ref) = &workspace.container_ref else {
            return Ok(true);
        };

        let workspace_dir = PathBuf::from(container_ref);
        if !workspace_dir.exists() {
            return Ok(true);
        }

        let repositories =
            WorkspaceRepo::find_repos_for_workspace(&self.db.pool, workspace.id).await?;

        for repo in &repositories {
            let worktree_path = workspace_dir.join(&repo.name);
            if worktree_path.exists() && !self.git().is_worktree_clean(&worktree_path)? {
                return Ok(false);
            }
        }

        Ok(true)
    }

    async fn start_execution_inner(
        &self,
        workspace: &Workspace,
        execution_process: &ExecutionProcess,
        executor_action: &ExecutorAction,
        model_config_id: Option<&str>,
    ) -> Result<(), ContainerError> {
        // Get the worktree path
        let container_ref = workspace
            .container_ref
            .as_ref()
            .ok_or(ContainerError::Other(anyhow!(
                "Container ref not found for workspace"
            )))?;
        let current_dir = PathBuf::from(container_ref);

        let approvals_service: Arc<dyn ExecutorApprovalService> =
            match executor_action.base_executor() {
                Some(
                    BaseCodingAgent::Codex
                    | BaseCodingAgent::ClaudeCode
                    | BaseCodingAgent::Gemini
                    | BaseCodingAgent::QwenCode
                    | BaseCodingAgent::Opencode,
                ) => ExecutorApprovalBridge::new(
                    self.approvals.clone(),
                    self.db.clone(),
                    self.notification_service.clone(),
                    execution_process.id,
                ),
                _ => Arc::new(NoopExecutorApprovalService {}),
            };

        // Build ExecutionEnv with VK_* variables
        let mut env = ExecutionEnv::new();

        // Load task and project context for environment variables
        let task = workspace
            .parent_task(&self.db.pool)
            .await?
            .ok_or(ContainerError::Other(anyhow!(
                "Task not found for workspace"
            )))?;
        let project = task
            .parent_project(&self.db.pool)
            .await?
            .ok_or(ContainerError::Other(anyhow!("Project not found for task")))?;

        env.insert("VK_PROJECT_NAME", &project.name);
        env.insert("VK_PROJECT_ID", project.id.to_string());
        env.insert("VK_TASK_ID", task.id.to_string());
        env.insert("VK_WORKSPACE_ID", workspace.id.to_string());
        env.insert("VK_WORKSPACE_BRANCH", &workspace.branch);

        // S6 — no-`-p` interactive transport selection. For native-OAuth
        // (subscription) ClaudeCode coding-agent runs, route through the genuine
        // `claude` interactive binary (transcript-tailing) instead of the metered
        // `-p` stream-json path, UNLESS `SOLODAWN_NO_POOL` is set (kill-switch:
        // accept pool draw, keep proven `-p` path). API-key/relay users and all
        // other executors keep the `-p` path unchanged — it is byte-for-byte
        // identical below.
        //
        // This decision MUST run BEFORE `resolve_executor_env_vars` below:
        // that helper creates + populates an isolated workspace CLAUDE_HOME (with
        // copied credentials) on disk, but the interactive path builds its OWN
        // auth env and discards `env` entirely — so calling it first would leak a
        // credential-bearing temp dir on every native-OAuth run. The interactive
        // router builds its auth independently of `env`, so this reorder is safe.
        if self
            .try_spawn_interactive_native_oauth(
                execution_process.id,
                &current_dir,
                executor_action,
                model_config_id,
            )
            .await?
        {
            return Ok(());
        }

        // Inject executor-specific authentication environment variables.
        // Without this, CLIs in workspace mode cannot authenticate because the
        // isolated workspace environment doesn't inherit global CLI configs.
        // Falls back to model_config stored credentials when global auth is missing.
        if let Some(base_executor) = executor_action.base_executor() {
            let profile_vars = Self::resolve_executor_env_vars(
                base_executor,
                executor_action,
                &self.db.pool,
                model_config_id,
            )
            .await;
            env.merge(&profile_vars);
        }

        // Create the child and stream, add to execution tracker with timeout
        let mut spawned = tokio::time::timeout(
            Duration::from_secs(30),
            executor_action.spawn(&current_dir, approvals_service, &env),
        )
        .await
        .map_err(|_| {
            ContainerError::Other(anyhow!(
                "Timeout: process took more than 30 seconds to start"
            ))
        })??;

        self.track_child_msgs_in_store(execution_process.id, &mut spawned.child)
            .await?;

        self.add_child_to_store(execution_process.id, spawned.child)
            .await;

        // Store interrupt sender for graceful shutdown
        if let Some(interrupt_sender) = spawned.interrupt_sender {
            self.add_interrupt_sender(execution_process.id, interrupt_sender)
                .await;
        }

        // Spawn unified exit monitor: watches OS exit and optional executor signal
        let _hn = self.spawn_exit_monitor(&execution_process.id, spawned.exit_signal);

        Ok(())
    }

    async fn stop_execution(
        &self,
        execution_process: &ExecutionProcess,
        status: ExecutionProcessStatus,
    ) -> Result<(), ContainerError> {
        let child = self
            .get_child_from_store(&execution_process.id)
            .await
            .ok_or_else(|| {
                ContainerError::Other(anyhow!("Child process not found for execution"))
            })?;
        let exit_code = if status == ExecutionProcessStatus::Completed {
            Some(0)
        } else {
            None
        };

        ExecutionProcess::update_completion(&self.db.pool, execution_process.id, status, exit_code)
            .await?;

        // Try graceful interrupt first, then force kill
        if let Some(interrupt_sender) = self.take_interrupt_sender(&execution_process.id).await {
            // Send interrupt signal (ignore error if receiver dropped)
            let _ = interrupt_sender.send(());

            // Wait for graceful exit with timeout
            let graceful_exit = {
                let mut child_guard = child.write().await;
                tokio::time::timeout(Duration::from_secs(5), child_guard.wait()).await
            };

            match graceful_exit {
                Ok(Ok(_)) => {
                    tracing::debug!(
                        "Process {} exited gracefully after interrupt",
                        execution_process.id
                    );
                }
                Ok(Err(e)) => {
                    tracing::info!("Error waiting for process {}: {}", execution_process.id, e);
                }
                Err(_) => {
                    tracing::debug!(
                        "Graceful shutdown timed out for process {}, force killing",
                        execution_process.id
                    );
                }
            }
        }

        // Kill the child process and remove from the store
        {
            let mut child_guard = child.write().await;
            if let Err(e) = command::kill_process_group(&mut child_guard).await {
                tracing::error!(
                    "Failed to stop execution process {}: {}",
                    execution_process.id,
                    e
                );
                return Err(e);
            }
        }
        self.remove_child_from_store(&execution_process.id).await;

        // Mark the process finished in the MsgStore
        if let Some(msg) = self.msg_stores.write().await.remove(&execution_process.id) {
            msg.push_finished();
        }

        // Update task status to InReview when execution is stopped
        if let Ok(ctx) = ExecutionProcess::load_context(&self.db.pool, execution_process.id).await
            && !matches!(
                ctx.execution_process.run_reason,
                ExecutionProcessRunReason::DevServer
            )
        {
            match Task::update_status(&self.db.pool, ctx.task.id, TaskStatus::InReview).await {
                Ok(()) => {
                    // Task status updated successfully
                }
                Err(e) => {
                    tracing::error!("Failed to update task status to InReview: {e}");
                }
            }
        }

        tracing::debug!(
            "Execution process {} stopped successfully",
            execution_process.id
        );

        // Record after-head commit OID (best-effort)
        self.update_after_head_commits(execution_process.id).await;

        Ok(())
    }

    async fn stream_diff(
        &self,
        workspace: &Workspace,
        stats_only: bool,
    ) -> Result<futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>>, ContainerError>
    {
        let workspace_repos =
            WorkspaceRepo::find_by_workspace_id(&self.db.pool, workspace.id).await?;
        let target_branches: HashMap<_, _> = workspace_repos
            .iter()
            .map(|wr| (wr.repo_id, wr.target_branch.clone()))
            .collect();

        let repositories =
            WorkspaceRepo::find_repos_for_workspace(&self.db.pool, workspace.id).await?;

        let mut streams = Vec::new();

        let container_ref = self.ensure_container_exists(workspace).await?;
        let workspace_root = PathBuf::from(container_ref);

        for repo in repositories {
            let worktree_path = workspace_root.join(&repo.name);
            let branch = &workspace.branch;

            let Some(target_branch) = target_branches.get(&repo.id) else {
                tracing::warn!(
                    "Skipping diff stream for repo {}: no target branch configured",
                    repo.name
                );
                continue;
            };

            let base_commit = match self
                .git()
                .get_base_commit(&repo.path, branch, target_branch)
            {
                Ok(c) => c,
                Err(e) => {
                    tracing::warn!(
                        "Skipping diff stream for repo {}: failed to get base commit: {}",
                        repo.name,
                        e
                    );
                    continue;
                }
            };

            let stream = Self::create_live_diff_stream(&diff_stream::DiffStreamArgs {
                git_service: self.git().clone(),
                db: self.db().clone(),
                workspace_id: workspace.id,
                repo_id: repo.id,
                repo_path: repo.path.clone(),
                worktree_path: worktree_path.clone(),
                branch: branch.clone(),
                target_branch: target_branch.clone(),
                base_commit: base_commit.clone(),
                stats_only,
                path_prefix: Some(repo.name.clone()),
            })?;

            streams.push(Box::pin(stream));
        }

        if streams.is_empty() {
            return Ok(Box::pin(futures::stream::empty()));
        }

        // Merge all streams into one
        Ok(Box::pin(futures::stream::select_all(streams)))
    }

    async fn try_commit_changes(&self, ctx: &ExecutionContext) -> Result<bool, ContainerError> {
        if !matches!(
            ctx.execution_process.run_reason,
            ExecutionProcessRunReason::CodingAgent | ExecutionProcessRunReason::CleanupScript,
        ) {
            return Ok(false);
        }

        let message = self.get_commit_message(ctx).await;

        let container_ref = ctx
            .workspace
            .container_ref
            .as_ref()
            .ok_or_else(|| ContainerError::Other(anyhow!("Container reference not found")))?;
        let workspace_root = PathBuf::from(container_ref);

        let repos_with_changes = Self::check_repos_for_changes(&workspace_root, &ctx.repos)?;
        if repos_with_changes.is_empty() {
            tracing::debug!("No changes to commit in any repository");
            return Ok(false);
        }

        Ok(self.commit_repos(repos_with_changes, &message))
    }

    /// Copy files from the original project directory to the worktree.
    /// Skips files that already exist at target with same size.
    async fn copy_project_files(
        &self,
        source_dir: &Path,
        target_dir: &Path,
        copy_files: &str,
    ) -> Result<(), ContainerError> {
        let source_dir = source_dir.to_path_buf();
        let target_dir = target_dir.to_path_buf();
        let copy_files = copy_files.to_string();

        tokio::time::timeout(
            std::time::Duration::from_secs(30),
            tokio::task::spawn_blocking(move || {
                copy::copy_project_files_impl(&source_dir, &target_dir, &copy_files)
            }),
        )
        .await
        .map_err(|_| ContainerError::Other(anyhow!("Copy project files timed out after 30s")))?
        .map_err(|e| ContainerError::Other(anyhow!("Copy files task failed: {e}")))?
    }

    async fn kill_all_running_processes(&self) -> Result<(), ContainerError> {
        tracing::info!("Killing all running processes");
        let running_processes = ExecutionProcess::find_running(&self.db.pool).await?;

        for process in running_processes {
            if let Err(error) = self
                .stop_execution(&process, ExecutionProcessStatus::Killed)
                .await
            {
                tracing::error!(
                    "Failed to cleanly kill running execution process {:?}: {:?}",
                    process,
                    error
                );
            }
        }

        Ok(())
    }
}
fn success_exit_status() -> std::process::ExitStatus {
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;
        ExitStatusExt::from_raw(0)
    }
    #[cfg(windows)]
    {
        use std::os::windows::process::ExitStatusExt;
        ExitStatusExt::from_raw(0)
    }
}
