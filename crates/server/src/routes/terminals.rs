//! Terminal Routes
//!
//! API endpoints for terminal management and log retrieval

use std::sync::Arc;

use axum::{
    Router,
    extract::{Path, Query, State},
    response::Json as ResponseJson,
    routing::{get, post},
};
use db::models::terminal::{Terminal, TerminalLog};
use deployment::Deployment;
use serde::Deserialize;
use services::services::{
    cc_switch::CCSwitchService,
    orchestrator::BusMessage,
    terminal::{
        bridge::TerminalBridge,
        process::{DEFAULT_COLS, DEFAULT_ROWS},
    },
};
use tokio::process::Command;
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

// BACKLOG-002: Runner container separation
// ============================================================================
// RunnerClient Terminal Spawn Configuration
// ============================================================================

/// Terminal spawn configuration for the RunnerClient abstraction layer.
///
/// This struct bridges the existing `SpawnCommand` format (used by ProcessManager)
/// to the RunnerClient interface. When RunnerClient is fully integrated, terminal
/// spawn requests will be sent via gRPC using this configuration.
///
/// Import path (future): `crate::services::runner_client::TerminalSpawnConfig`
// BACKLOG-002: Runner container separation
#[allow(dead_code)]
pub(crate) struct TerminalSpawnConfig {
    /// Unique terminal identifier.
    pub terminal_id: String,
    /// Command to execute (e.g., "claude", "codex", "gemini").
    pub command: String,
    /// Command-line arguments.
    pub args: Vec<String>,
    /// Working directory for the child process.
    pub working_dir: std::path::PathBuf,
    /// Environment variables to set on the child process.
    pub env_set: std::collections::HashMap<String, String>,
    /// Environment variable keys to remove from the inherited environment.
    pub env_unset: Vec<String>,
    /// Terminal width in columns.
    pub cols: u16,
    /// Terminal height in rows.
    pub rows: u16,
}

/// Build TerminalSpawnConfig from CCSwitchService output.
/// This bridges the existing SpawnCommand format to the RunnerClient interface.
// BACKLOG-002: Runner container separation
#[allow(dead_code)]
fn spawn_command_to_runner_config(
    terminal_id: &str,
    spawn_config: &services::services::terminal::process::SpawnCommand,
    cols: u16,
    rows: u16,
) -> TerminalSpawnConfig {
    TerminalSpawnConfig {
        terminal_id: terminal_id.to_string(),
        command: spawn_config.command.clone(),
        args: spawn_config.args.clone(),
        working_dir: spawn_config.working_dir.clone(),
        env_set: spawn_config.env.set.clone(),
        env_unset: spawn_config.env.unset.clone(),
        cols,
        rows,
    }
}

async fn broadcast_terminal_status(
    deployment: &DeploymentImpl,
    terminal: &Terminal,
    status: &str,
) -> anyhow::Result<()> {
    let task =
        db::models::WorkflowTask::find_by_id(&deployment.db().pool, &terminal.workflow_task_id)
            .await?
            .ok_or_else(|| {
                anyhow::anyhow!("Workflow task {} not found", terminal.workflow_task_id)
            })?;

    let message = BusMessage::TerminalStatusUpdate {
        workflow_id: task.workflow_id.clone(),
        terminal_id: terminal.id.clone(),
        status: status.to_string(),
    };
    let topic = format!("workflow:{}", task.workflow_id);

    deployment
        .message_bus()
        .publish(&topic, message.clone())
        .await
        .map_err(|e| anyhow::anyhow!("Failed to publish terminal status: {e}"))?;
    deployment
        .message_bus()
        .broadcast(message)
        .map_err(|e| anyhow::anyhow!("Failed to broadcast terminal status: {e}"))?;

    Ok(())
}

/// Terminal state machine: statuses that can directly transition to starting.
/// For waiting/working, we check if process is actually running first.
const STARTABLE_TERMINAL_STATUSES: [&str; 5] =
    ["not_started", "failed", "cancelled", "waiting", "working"];
const CLOSABLE_TERMINAL_STATUSES: [&str; 3] = ["completed", "failed", "cancelled"];

/// Query parameters for terminal logs retrieval
#[derive(Debug, Deserialize)]
pub struct TerminalLogsQuery {
    /// Maximum number of logs to return (default: 1000)
    pub limit: Option<i32>,
}

/// Get extended PATH with common CLI installation directories
#[cfg(windows)]
fn get_extended_path() -> String {
    let current_path = std::env::var("PATH").unwrap_or_default();
    let mut paths: Vec<String> = vec![current_path];

    // Add common npm global paths
    if let Ok(appdata) = std::env::var("APPDATA") {
        paths.push(format!("{appdata}\\npm"));
    }

    // Add user local bin (for tools like claude)
    if let Ok(userprofile) = std::env::var("USERPROFILE") {
        paths.push(format!("{userprofile}\\.local\\bin"));
    }

    // Add common program files paths
    if let Ok(programfiles) = std::env::var("ProgramFiles") {
        paths.push(format!("{programfiles}\\nodejs"));
    }

    paths.join(";")
}

#[cfg(not(windows))]
fn get_extended_path() -> String {
    let current_path = std::env::var("PATH").unwrap_or_default();
    let mut paths: Vec<String> = vec![current_path];

    // Add common paths on Unix
    if let Ok(home) = std::env::var("HOME") {
        paths.push(format!("{}/.local/bin", home));
        paths.push(format!("{}/.npm-global/bin", home));
        paths.push(format!("{}/bin", home));
    }

    paths.push("/usr/local/bin".to_string());

    paths.join(":")
}

/// Find executable path for a command
async fn find_executable(cmd: &str) -> Option<String> {
    let extended_path = get_extended_path();

    #[cfg(unix)]
    {
        Command::new("which")
            .arg(cmd)
            .env("PATH", &extended_path)
            .output()
            .await
            .ok()
            .filter(|o| o.status.success())
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
    }

    #[cfg(windows)]
    {
        Command::new("where")
            .arg(cmd)
            .env("PATH", &extended_path)
            .output()
            .await
            .ok()
            .filter(|o| o.status.success())
            .map(|o| {
                String::from_utf8_lossy(&o.stdout)
                    .lines()
                    .next()
                    .unwrap_or("")
                    .to_string()
            })
    }
}

/// Get terminal logs endpoint
///
/// GET /api/terminals/:id/logs
///
/// Retrieves all logs for a specific terminal in chronological order
pub async fn get_terminal_logs(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<Uuid>,
    Query(query): Query<TerminalLogsQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<TerminalLog>>>, ApiError> {
    let id = id.to_string();
    // Fetch logs from database (already in DESC order by created_at)
    // G16-011: Clamp limit to [0, 10000] to reject negative values and cap upper bound.
    let clamped_limit = query.limit.map(|l| l.clamp(0, 10000));
    let mut logs = TerminalLog::find_by_terminal(&deployment.db().pool, &id, clamped_limit).await?;

    // Reverse to get chronological order (oldest first)
    logs.reverse();

    Ok(ResponseJson(ApiResponse::success(logs)))
}

/// Start terminal endpoint
///
/// POST /api/terminals/:id/start
///
/// Starts a terminal by spawning a PTY process with proper configuration
/// including auto-confirm flags and MessageBus bridge registration.
pub async fn start_terminal(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<serde_json::Value>>, ApiError> {
    let id = id.to_string();
    // Fetch terminal from database
    let terminal = Terminal::find_by_id(&deployment.db().pool, &id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("Terminal {id} not found")))?;

    // Validate terminal status before starting
    // NOTE(G16-015): Per-terminal concurrency is protected by the CAS-style status
    // transition below (only one caller can move from a startable status to "starting").
    // The is_running() check provides an additional safety net but is not the primary guard.
    if !STARTABLE_TERMINAL_STATUSES.contains(&terminal.status.as_str()) {
        return Err(ApiError::Conflict(format!(
            "Terminal {id} cannot be started from status '{}'",
            terminal.status
        )));
    }

    // Check if terminal is already running
    if deployment.process_manager().is_running(&id).await {
        return Err(ApiError::Conflict(format!(
            "Terminal {id} is already running"
        )));
    }

    // Get CLI type to determine shell command
    let cli_type =
        db::models::cli_type::CliType::find_by_id(&deployment.db().pool, &terminal.cli_type_id)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to fetch CLI type: {e}")))?
            .ok_or_else(|| {
                ApiError::NotFound(format!("CLI type {} not found", terminal.cli_type_id))
            })?;

    // Determine shell command based on CLI type
    let cmd_name = match cli_type.name.as_str() {
        "claude-code" => "claude",
        "gemini-cli" => "gemini",
        "codex" => "codex",
        "amp" => "amp",
        "cursor-agent" => "cursor",
        _ => &cli_type.name,
    };

    // Find the absolute path of the CLI executable
    let shell = find_executable(cmd_name).await.ok_or_else(|| {
        ApiError::BadRequest(format!(
            "CLI '{cmd_name}' not found in PATH. Please ensure it is installed and accessible."
        ))
    })?;

    tracing::info!("Found CLI executable at: {}", shell);

    // Get working directory from workflow task
    let working_dir = get_terminal_working_dir(&deployment, &terminal.workflow_task_id)
        .await
        .map_err(|e| {
            ApiError::Internal(format!(
                "Failed to resolve working directory for terminal {} (workflow_task_id={}): {}",
                id, terminal.workflow_task_id, e
            ))
        })?;

    if !working_dir.exists() {
        return Err(ApiError::BadRequest(format!(
            "Working directory does not exist: {}",
            working_dir.display()
        )));
    }
    if !working_dir.is_dir() {
        return Err(ApiError::BadRequest(format!(
            "Working directory is not a directory: {}",
            working_dir.display()
        )));
    }

    tracing::info!(terminal_id = %id, working_dir = %working_dir.display(), "Resolved terminal working directory");

    // G16-015: CAS transition to "starting" - only one caller can move from a startable status
    let cas_result = sqlx::query(
        r"
        UPDATE terminal
        SET status = 'starting', updated_at = datetime('now')
        WHERE id = ? AND status IN ('not_started', 'failed', 'cancelled', 'waiting', 'working')
        ",
    )
    .bind(&id)
    .execute(&deployment.db().pool)
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to update terminal status: {e}")))?;

    if cas_result.rows_affected() == 0 {
        return Err(ApiError::Conflict(format!(
            "Terminal {id} cannot be started: status changed concurrently"
        )));
    }
    if let Err(e) = broadcast_terminal_status(&deployment, &terminal, "starting").await {
        tracing::warn!(
            terminal_id = %id,
            error = %e,
            "Failed to broadcast starting terminal status"
        );
    }

    // Build spawn configuration with auto-confirm flags using CCSwitchService
    let cc_switch = CCSwitchService::new(Arc::new(deployment.db().clone()));
    let spawn_config = match cc_switch
        .build_launch_config(&terminal, &shell, &working_dir, terminal.auto_confirm)
        .await
    {
        Ok(config) => config,
        Err(e) => {
            // On config build failure, reset status
            let _ = Terminal::update_status(&deployment.db().pool, &id, "failed").await;
            if let Err(event_err) =
                broadcast_terminal_status(&deployment, &terminal, "failed").await
            {
                tracing::warn!(
                    terminal_id = %id,
                    error = %event_err,
                    "Failed to broadcast failed terminal status"
                );
            }
            return Err(ApiError::Internal(format!(
                "Failed to build launch config: {e}"
            )));
        }
    };

    // Spawn PTY process with configuration
    // RUNNER_CLIENT_MIGRATION: When RunnerClient is integrated, replace:
    //   deployment.process_manager().spawn_pty_with_config(&id, &spawn_config, cols, rows)
    // with:
    //   deployment.runner_client().spawn_terminal(TerminalSpawnConfig { ... })
    // The TerminalSpawnConfig is built from CCSwitchService::build_launch_config() output.
    // Use spawn_command_to_runner_config() to convert SpawnCommand -> TerminalSpawnConfig.
    let handle = match deployment
        .process_manager()
        .spawn_pty_with_config(&id, &spawn_config, DEFAULT_COLS, DEFAULT_ROWS)
        .await
    {
        Ok(handle) => handle,
        Err(e) => {
            // On spawn failure, set status to failed
            let _ = Terminal::update_status(&deployment.db().pool, &id, "failed").await;
            let _ = Terminal::update_process(&deployment.db().pool, &id, None, None).await;
            if let Err(event_err) =
                broadcast_terminal_status(&deployment, &terminal, "failed").await
            {
                tracing::warn!(
                    terminal_id = %id,
                    error = %event_err,
                    "Failed to broadcast failed terminal status"
                );
            }
            return Err(ApiError::Internal(format!(
                "Failed to spawn terminal process: {e}"
            )));
        }
    };

    // Update terminal status in database
    if let Err(e) = Terminal::set_waiting(&deployment.db().pool, &id).await {
        let _ = deployment.process_manager().kill_terminal(&id).await;
        let _ = Terminal::update_process(&deployment.db().pool, &id, None, None).await;
        let _ = Terminal::update_status(&deployment.db().pool, &id, "failed").await;
        if let Err(event_err) = broadcast_terminal_status(&deployment, &terminal, "failed").await {
            tracing::warn!(
                terminal_id = %id,
                error = %event_err,
                "Failed to broadcast failed terminal status"
            );
        }
        return Err(ApiError::Internal(format!(
            "Failed to update terminal status: {e}"
        )));
    }
    if let Err(e) = broadcast_terminal_status(&deployment, &terminal, "waiting").await {
        tracing::warn!(
            terminal_id = %id,
            error = %e,
            "Failed to broadcast waiting terminal status"
        );
    }

    let pid = i32::try_from(handle.pid).ok();
    if let Err(e) =
        Terminal::update_process(&deployment.db().pool, &id, pid, Some(&handle.session_id)).await
    {
        let _ = deployment.process_manager().kill_terminal(&id).await;
        let _ = Terminal::update_process(&deployment.db().pool, &id, None, None).await;
        let _ = Terminal::update_status(&deployment.db().pool, &id, "failed").await;
        if let Err(event_err) = broadcast_terminal_status(&deployment, &terminal, "failed").await {
            tracing::warn!(
                terminal_id = %id,
                error = %event_err,
                "Failed to broadcast failed terminal status"
            );
        }
        return Err(ApiError::Internal(format!(
            "Failed to update terminal process info: {e}"
        )));
    }

    // Attach terminal logger for output persistence (Phase 26)
    if let Err(e) = deployment
        .process_manager()
        .attach_terminal_logger(Arc::new(deployment.db().clone()), &id, "stdout", 1)
        .await
    {
        tracing::error!(
            terminal_id = %id,
            error = %e,
            "Failed to attach terminal logger; rolling back manual start"
        );

        let _ = deployment.process_manager().kill_terminal(&id).await;
        let _ = Terminal::update_process(&deployment.db().pool, &id, None, None).await;
        let _ = Terminal::update_status(&deployment.db().pool, &id, "failed").await;
        if let Err(event_err) = broadcast_terminal_status(&deployment, &terminal, "failed").await {
            tracing::warn!(
                terminal_id = %id,
                error = %event_err,
                "Failed to broadcast failed terminal status"
            );
        }

        return Err(ApiError::Internal(format!(
            "Failed to attach terminal logger: {e}"
        )));
    }
    tracing::debug!(terminal_id = %id, "Terminal logger attached for output persistence");

    // Register terminal bridge for MessageBus -> PTY stdin forwarding
    let terminal_bridge = TerminalBridge::new(
        deployment.message_bus().clone(),
        deployment.process_manager().clone(),
    );
    if let Err(e) = terminal_bridge.register(&id, &handle.session_id).await {
        tracing::warn!(
            terminal_id = %id,
            pty_session_id = %handle.session_id,
            error = %e,
            "Failed to register terminal bridge (non-fatal)"
        );
    } else {
        tracing::debug!(
            terminal_id = %id,
            pty_session_id = %handle.session_id,
            "Terminal bridge registered successfully"
        );
    }

    // Register PromptWatcher for background prompt detection
    // This enables both auto-confirm and AskUser prompt handling without WebSocket connection
    // Use handle.session_id (the actual PTY session) not terminal.pty_session_id (stale DB value)
    match db::models::WorkflowTask::find_by_id(&deployment.db().pool, &terminal.workflow_task_id)
        .await
    {
        Ok(Some(task)) => {
            if let Err(e) = deployment
                .prompt_watcher()
                .register(
                    &id,
                    &task.workflow_id,
                    &terminal.workflow_task_id,
                    &handle.session_id,
                    terminal.auto_confirm,
                )
                .await
            {
                tracing::warn!(
                    terminal_id = %id,
                    workflow_id = %task.workflow_id,
                    pty_session_id = %handle.session_id,
                    error = %e,
                    "Failed to register PromptWatcher for background prompt detection"
                );
            } else {
                tracing::info!(
                    terminal_id = %id,
                    workflow_id = %task.workflow_id,
                    pty_session_id = %handle.session_id,
                    "PromptWatcher registered for background prompt detection"
                );
            }
        }
        Ok(None) => {
            tracing::warn!(
                terminal_id = %id,
                workflow_task_id = %terminal.workflow_task_id,
                "Skipped PromptWatcher registration: workflow task not found"
            );
        }
        Err(e) => {
            tracing::warn!(
                terminal_id = %id,
                workflow_task_id = %terminal.workflow_task_id,
                error = %e,
                "Failed to query workflow task for PromptWatcher registration (non-fatal)"
            );
        }
    }

    tracing::info!(
        terminal_id = %id,
        pid = handle.pid,
        auto_confirm = terminal.auto_confirm,
        "Terminal started with auto-confirm={}", terminal.auto_confirm
    );

    Ok(ResponseJson(ApiResponse::success(serde_json::json!({
        "terminal_id": id,
        "pid": handle.pid,
        "session_id": handle.session_id,
        "status": "waiting",
        "auto_confirm": terminal.auto_confirm
    }))))
}

/// Get working directory for a terminal from its workflow task
async fn get_terminal_working_dir(
    deployment: &DeploymentImpl,
    workflow_task_id: &str,
) -> anyhow::Result<std::path::PathBuf> {
    // Get workflow_id from workflow_task
    let workflow_id: Option<String> =
        sqlx::query_scalar("SELECT workflow_id FROM workflow_task WHERE id = ?")
            .bind(workflow_task_id)
            .fetch_optional(&deployment.db().pool)
            .await?
            .flatten();

    let workflow_id = workflow_id
        .ok_or_else(|| anyhow::anyhow!("Workflow task {workflow_task_id} not found"))?;

    // Get project_id from workflow
    let project_id: Option<Vec<u8>> =
        sqlx::query_scalar("SELECT project_id FROM workflow WHERE id = ?")
            .bind(&workflow_id)
            .fetch_optional(&deployment.db().pool)
            .await?
            .flatten();

    let project_id =
        project_id.ok_or_else(|| anyhow::anyhow!("Workflow {workflow_id} not found"))?;

    // Convert project_id bytes to UUID string
    let project_uuid = uuid::Uuid::from_slice(&project_id)
        .map_err(|e| anyhow::anyhow!("Invalid project_id format: {e}"))?;

    // 1) Prefer project.default_agent_working_dir
    let working_dir: Option<String> =
        sqlx::query_scalar("SELECT default_agent_working_dir FROM projects WHERE id = ?")
            .bind(project_uuid)
            .fetch_optional(&deployment.db().pool)
            .await?
            .flatten();

    if let Some(dir) = working_dir.filter(|dir| !dir.trim().is_empty()) {
        return Ok(std::path::PathBuf::from(dir));
    }

    // 2) Fallback to first repo path in project
    let repo_working_dir: Option<String> = sqlx::query_scalar(
        r"
        SELECT r.path
        FROM repos r
        INNER JOIN project_repos pr ON pr.repo_id = r.id
        WHERE pr.project_id = ?
        ORDER BY r.display_name ASC
        LIMIT 1
        ",
    )
    .bind(project_uuid)
    .fetch_optional(&deployment.db().pool)
    .await?
    .flatten();

    if let Some(dir) = repo_working_dir.filter(|dir| !dir.trim().is_empty()) {
        return Ok(std::path::PathBuf::from(dir));
    }

    Err(anyhow::anyhow!(
        "Could not determine working directory: project {project_uuid} has no default_agent_working_dir and no repositories"
    ))
}

/// Stop terminal endpoint
///
/// POST /api/terminals/:id/stop
///
/// Stops a running terminal and resets its status to 'not_started' for restart
pub async fn stop_terminal(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<String>>, ApiError> {
    let id = id.to_string();
    // Ensure terminal exists first to avoid false success on nonexistent id
    let terminal = Terminal::find_by_id(&deployment.db().pool, &id)
        .await?
        .ok_or_else(|| {
            // G16-005: Descriptive NotFound message including terminal ID
            ApiError::NotFound(format!(
                "Terminal {id} not found: cannot stop a non-existent terminal"
            ))
        })?;
    // Best-effort kill in case the process is still running
    // RUNNER_CLIENT_MIGRATION: Replace process_manager().kill_terminal() with runner_client().kill_terminal()
    if let Err(e) = deployment.process_manager().kill_terminal(&id).await {
        tracing::warn!("Failed to kill terminal {}: {}", id, e);
    }

    // Ensure prompt watcher state/task is cleaned up for this terminal
    deployment.prompt_watcher().unregister(&id).await;

    // Reset terminal runtime/completion fields so next workflow round can run cleanly.
    Terminal::reset_for_restart(&deployment.db().pool, &id).await?;
    if let Err(e) = broadcast_terminal_status(&deployment, &terminal, "not_started").await {
        tracing::warn!(
            terminal_id = %id,
            error = %e,
            "Failed to broadcast not_started terminal status"
        );
    }

    tracing::info!("Terminal {} stopped and reset successfully", id);

    Ok(ResponseJson(ApiResponse::success(format!(
        "Terminal {id} stopped successfully"
    ))))
}

/// Close a completed/failed/cancelled terminal without resetting its final status.
pub async fn close_terminal(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<Terminal>>, ApiError> {
    let id = id.to_string();
    let terminal = Terminal::find_by_id(&deployment.db().pool, &id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("Terminal {id} not found")))?;

    if !CLOSABLE_TERMINAL_STATUSES.contains(&terminal.status.as_str()) {
        return Err(ApiError::Conflict(format!(
            "Terminal {id} cannot be closed from status '{}'",
            terminal.status
        )));
    }

    // RUNNER_CLIENT_MIGRATION: Replace process_manager().kill_terminal() with runner_client().kill_terminal()
    if let Err(e) = deployment.process_manager().kill_terminal(&id).await {
        tracing::warn!("Failed to kill terminal {} during close: {}", id, e);
    }

    if let Some(pty_session_id) = terminal.pty_session_id.as_deref() {
        let terminal_bridge = TerminalBridge::new(
            deployment.message_bus().clone(),
            deployment.process_manager().clone(),
        );
        terminal_bridge.unregister(pty_session_id).await;
    }

    deployment.prompt_watcher().unregister(&id).await;

    Terminal::update_process(&deployment.db().pool, &id, None, None).await?;

    let terminal = Terminal::find_by_id(&deployment.db().pool, &id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("Terminal {id} not found after close")))?;

    if let Err(e) = broadcast_terminal_status(&deployment, &terminal, &terminal.status).await {
        tracing::warn!(
            terminal_id = %id,
            error = %e,
            "Failed to broadcast terminal close status"
        );
    }

    Ok(ResponseJson(ApiResponse::success(terminal)))
}

/// Terminal routes router
///
/// Mounts all terminal-related API endpoints
pub fn terminal_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/{id}/logs", get(get_terminal_logs))
        .route("/{id}/start", post(start_terminal))
        .route("/{id}/stop", post(stop_terminal))
        .route("/{id}/close", post(close_terminal))
}
