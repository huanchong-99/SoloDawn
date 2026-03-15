//! Terminal WebSocket routes with PTY support
//!
//! Provides WebSocket-based terminal I/O using portable-pty for cross-platform support.

use std::{
    error::Error as StdError,
    io::ErrorKind,
    sync::atomic::{AtomicBool, AtomicU64, Ordering},
    time::{Duration, SystemTime, UNIX_EPOCH},
};

use axum::{
    Router,
    extract::{
        Path, Query, State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    response::IntoResponse,
    routing::get,
};
use db::models::{Terminal, WorkflowTask};
use deployment::Deployment;
use futures_util::{SinkExt, StreamExt};
use once_cell::sync::Lazy;
use regex::Regex;
use serde::{Deserialize, Serialize};
use tokio::sync::mpsc;
use ts_rs::TS;

/// Returns current time as milliseconds since the Unix epoch (wraps to 0 on overflow).
#[inline]
fn now_millis() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or(0)
}

/// Elapsed milliseconds since `start_millis` (monotonic approximation via SystemTime).
#[inline]
fn elapsed_since_millis(start_millis: u64) -> Duration {
    let now = now_millis();
    Duration::from_millis(now.saturating_sub(start_millis))
}

use crate::{DeploymentImpl, error::ApiError};

// BACKLOG-002: Runner container separation
// ============================================================================
// Terminal I/O Abstraction
// ============================================================================

/// Terminal I/O abstraction for future RunnerClient migration.
/// Currently delegates to ProcessManager directly.
/// When RunnerClient is integrated, this will route through gRPC streams
/// instead of local ProcessHandle reader/writer pairs.
// BACKLOG-002: Runner container separation
#[allow(dead_code)]
pub(crate) struct TerminalIO {
    /// The terminal identifier this I/O handle is associated with.
    pub terminal_id: String,
    /// PTY writer handle, wrapped for shared access across reconnections.
    /// When RunnerClient is integrated, this will be replaced with a gRPC
    /// stream sender for forwarding input to the remote runner.
    pub writer: Option<
        std::sync::Arc<std::sync::Mutex<services::services::terminal::process::PtyWriter>>,
    >,
    // RUNNER_CLIENT_MIGRATION: Future fields for gRPC-based I/O:
    //   pub input_stream: Option<tonic::Streaming<TerminalInputChunk>>,
    //   pub output_stream: Option<tonic::Streaming<TerminalOutputChunk>>,
}

#[allow(dead_code)]
impl TerminalIO {
    /// Create a TerminalIO from an existing ProcessHandle.
    /// This is the current local-runner path. When RunnerClient is integrated,
    /// a separate constructor (e.g., `from_runner_stream`) will be added for
    /// gRPC-based terminal I/O.
    // BACKLOG-002: Runner container separation
    pub fn from_process_handle(
        terminal_id: String,
        handle: &services::services::terminal::process::ProcessHandle,
    ) -> Self {
        Self {
            terminal_id,
            writer: handle.writer.clone(),
        }
    }
}

// ============================================================================
// Constants
// ============================================================================

/// Idle timeout - close connection if no activity (5 minutes)
const WS_IDLE_TIMEOUT_SECS: u64 = 300;

/// Heartbeat interval for keep-alive (30 seconds)
const WS_HEARTBEAT_INTERVAL_SECS: u64 = 30;

// Compile-time sanity check for timeout ordering.
const _: () = {
    assert!(WS_HEARTBEAT_INTERVAL_SECS < WS_IDLE_TIMEOUT_SECS);
};

// ============================================================================
// UUID Validation
// ============================================================================

/// UUID v4 regex pattern (case-insensitive)
static UUID_REGEX: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$").unwrap()
});

/// Validates that terminal_id is a properly formatted UUID
pub fn validate_terminal_id(terminal_id: &str) -> anyhow::Result<()> {
    if UUID_REGEX.is_match(terminal_id) {
        Ok(())
    } else {
        Err(anyhow::anyhow!(
            "Invalid terminal_id format: expected UUID format (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)"
        ))
    }
}

// ============================================================================
// WebSocket Message Types
// ============================================================================

/// WebSocket message types
#[derive(Debug, Serialize, Deserialize, TS)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum WsMessage {
    /// Input from client (user keystrokes)
    Input { data: String },
    /// Output to client (terminal response)
    Output { data: String },
    /// Terminal resize request
    Resize { cols: u16, rows: u16 },
    /// Heartbeat from client for keep-alive
    Heartbeat,
    /// Error message
    Error { message: String },
}

fn is_benign_ws_disconnect(error: &axum::Error) -> bool {
    fn has_benign_io_kind(err: &(dyn StdError + 'static)) -> bool {
        if let Some(io_err) = err.downcast_ref::<std::io::Error>() {
            return matches!(
                io_err.kind(),
                ErrorKind::ConnectionAborted
                    | ErrorKind::ConnectionReset
                    | ErrorKind::BrokenPipe
                    | ErrorKind::UnexpectedEof
            );
        }

        err.source().is_some_and(has_benign_io_kind)
    }

    if has_benign_io_kind(error) {
        return true;
    }

    let message = error.to_string().to_ascii_lowercase();
    message.contains("connection aborted")
        || message.contains("connection reset")
        || message.contains("broken pipe")
        || message.contains("without closing handshake")
}

// ============================================================================
// Route Definition
// ============================================================================

/// Create terminal WebSocket routes
pub fn terminal_ws_routes() -> Router<DeploymentImpl> {
    Router::new().route("/{terminal_id}", get(terminal_ws_handler))
}

// ============================================================================
// Route Handlers
// ============================================================================

/// Optional query parameters for the terminal WebSocket endpoint.
///
/// `last_seq`: The last output sequence number the client received before disconnecting.
/// When provided, the server replays only the chunks with `seq > last_seq`, enabling
/// efficient reconnection without re-sending the full history. [G09-003]
#[derive(Debug, serde::Deserialize, Default)]
struct ResumeParams {
    last_seq: Option<u64>,
}

/// WebSocket handler for terminal connection
async fn terminal_ws_handler(
    ws: WebSocketUpgrade,
    Path(terminal_id): Path<String>,
    Query(params): Query<ResumeParams>,
    State(deployment): State<DeploymentImpl>,
) -> impl IntoResponse {
    // Validate terminal_id format before proceeding
    if let Err(e) = validate_terminal_id(&terminal_id) {
        tracing::warn!("Invalid terminal_id format: {} - {}", terminal_id, e);
        return ApiError::BadRequest(format!("Invalid terminal_id format: {e}")).into_response();
    }

    ws.on_upgrade(move |socket| {
        handle_terminal_socket(socket, terminal_id, deployment, params.last_seq)
    })
}

/// Handle terminal WebSocket connection with PTY.
///
/// `resume_from_seq`: when `Some(n)`, replay starts from `seq > n` (G09-003 resume).
/// When `None`, full replay from the earliest retained chunk is provided.
async fn handle_terminal_socket(
    socket: WebSocket,
    terminal_id: String,
    deployment: DeploymentImpl,
    resume_from_seq: Option<u64>,
) {
    if let Some(seq) = resume_from_seq {
        tracing::info!(
            terminal_id = %terminal_id,
            resume_from_seq = seq,
            "Terminal WebSocket connected (resume mode)"
        );
    } else {
        tracing::info!("Terminal WebSocket connected: {}", terminal_id);
    }

    // Split the WebSocket into sender and receiver
    let (mut ws_sender, mut ws_receiver) = socket.split();

    // Get process handle from ProcessManager
    let process_handle = deployment.process_manager().get_handle(&terminal_id).await;

    let process_handle = if let Some(handle) = process_handle { handle } else {
        let msg = WsMessage::Error {
            message: "Terminal process not running. Please start the terminal first."
                .to_string(),
        };
        let _ = ws_sender
            .send(Message::Text(serde_json::to_string(&msg).unwrap().into()))
            .await;
        let _ = ws_sender.close().await;
        tracing::warn!("Terminal {} has no running process", terminal_id);
        return;
    };

    sync_prompt_watcher_registration(
        &terminal_id,
        deployment.prompt_watcher().clone(),
        deployment.db().pool.clone(),
        Some(process_handle.session_id.as_str()),
    )
    .await;

    // Clone process_manager for output subscription and resize operations
    let process_manager = deployment.process_manager().clone();

    // Extract PTY writer (reader is now owned by background fanout task)
    let writer = process_handle.writer;

    // Proactively check if writer is available
    if writer.is_none() {
        let msg = WsMessage::Error {
            message: "Terminal process has no input writer available.".to_string(),
        };
        if let Ok(json) = serde_json::to_string(&msg) {
            let _ = ws_sender.send(Message::Text(json.into())).await;
        }
        let _ = ws_sender.close().await;
        tracing::warn!("Terminal {} has no PTY writer available", terminal_id);
        return;
    }

    // Subscribe to terminal output stream with seq-based resume (G09-003).
    // On reconnect, the client passes `last_seq` as a query parameter so only
    // chunks with seq > last_seq are replayed. When last_seq is None, the full
    // retained replay window is provided (first connect or forced full replay).
    let mut output_subscription = match process_manager.subscribe_output(&terminal_id, resume_from_seq).await {
        Ok(sub) => sub,
        Err(e) => {
            tracing::error!(
                terminal_id = %terminal_id,
                error = %e,
                "Failed to subscribe to terminal output"
            );
            let msg = WsMessage::Error {
                message: format!("Failed to subscribe to terminal output: {e}"),
            };
            if let Ok(json) = serde_json::to_string(&msg) {
                let _ = ws_sender.send(Message::Text(json.into())).await;
            }
            let _ = ws_sender.close().await;
            return;
        }
    };

    // Create channels for async communication
    // WebSocket input -> PTY
    let (ws_tx_input, mut ws_rx) = mpsc::channel::<Vec<u8>>(100);

    // Clone terminal_id for tasks
    let terminal_id_writer = terminal_id.clone();
    let terminal_id_heartbeat = terminal_id.clone();
    let terminal_id_recv = terminal_id.clone();
    let terminal_id_output = terminal_id.clone();

    // [G09-010] Track last activity time for idle timeout using AtomicU64 (Unix ms timestamp).
    // AtomicU64 is lock-free and avoids the overhead of RwLock for single-value timestamps.
    let last_activity = std::sync::Arc::new(AtomicU64::new(now_millis()));
    let last_activity_recv = last_activity.clone();
    let last_activity_heartbeat = last_activity.clone();

    // [G08-010] Track last client message time to detect half-open connections.
    // If the server receives no client messages for 90s, the heartbeat task will
    // close the connection (the client should send Heartbeat every 30s).
    let last_client_message = std::sync::Arc::new(AtomicU64::new(now_millis()));
    let last_client_message_recv = last_client_message.clone();
    let last_client_message_heartbeat = last_client_message.clone();

    let terminal_id_resize = terminal_id.clone();
    let idle_timeout_notifier = std::sync::Arc::new(tokio::sync::Notify::new());
    let idle_timeout_triggered = std::sync::Arc::new(AtomicBool::new(false));

    // Spawn output subscription task (fanout -> WebSocket)
    // Replaces old PTY reader/send task pipeline with shared fanout + replay support
    // Note: PromptWatcher now subscribes directly to fanout, no need to feed it here
    let output_task = {
        let last_activity_clone = last_activity.clone();
        let idle_timeout_notifier = idle_timeout_notifier.clone();
        let idle_timeout_triggered = idle_timeout_triggered.clone();
        let process_manager_output = process_manager.clone();
        tokio::spawn(async move {
            loop {
                tokio::select! {
                    () = idle_timeout_notifier.notified() => {
                        if idle_timeout_triggered.load(Ordering::Acquire) {
                            tracing::info!(
                                terminal_id = %terminal_id_output,
                                "Closing terminal WebSocket gracefully due to idle timeout"
                            );

                            if let Err(e) = ws_sender.send(Message::Close(None)).await {
                                if is_benign_ws_disconnect(&e) {
                                    tracing::debug!(
                                        terminal_id = %terminal_id_output,
                                        error = %e,
                                        "WebSocket already disconnected while sending idle-timeout close frame"
                                    );
                                } else {
                                    tracing::warn!(
                                        terminal_id = %terminal_id_output,
                                        error = %e,
                                        "Failed to send idle-timeout close frame"
                                    );
                                }
                            }

                            let _ = ws_sender.close().await;
                        }
                        break;
                    }
                    recv_result = output_subscription.recv() => {
                        match recv_result {
                            Ok(chunk) => {
                                // Update last activity
                                last_activity_clone.store(now_millis(), Ordering::Relaxed);

                                if !chunk.text.is_empty() {
                                    let msg = WsMessage::Output { data: chunk.text };
                                    match serde_json::to_string(&msg) {
                                        Ok(json) => {
                                            if let Err(e) = ws_sender.send(Message::Text(json.into())).await
                                            {
                                                tracing::debug!(
                                                    terminal_id = %terminal_id_output,
                                                    error = %e,
                                                    "Failed to send output to WebSocket"
                                                );
                                                break;
                                            }
                                        }
                                        Err(e) => {
                                            tracing::error!(
                                                terminal_id = %terminal_id_output,
                                                error = %e,
                                                "Failed to serialize terminal output message"
                                            );
                                            break;
                                        }
                                    }
                                }

                                // Log dropped bytes if any
                                if chunk.dropped_invalid_bytes > 0 {
                                    tracing::warn!(
                                        terminal_id = %terminal_id_output,
                                        seq = chunk.seq,
                                        dropped_bytes = chunk.dropped_invalid_bytes,
                                        "Dropped invalid UTF-8 bytes in terminal output"
                                    );
                                }
                            }
                            Err(e) => {
                                use tokio::sync::broadcast::error::RecvError;
                                match e {
                                    RecvError::Lagged(skipped) => {
                                        // [G09-001] Recoverable: output burst exceeded channel capacity.
                                        // Log warning and attempt to re-subscribe from last known seq
                                        // to recover any chunks still in the replay buffer.
                                        let resume_from = output_subscription.last_seq();
                                        tracing::warn!(
                                            terminal_id = %terminal_id_output,
                                            skipped_messages = skipped,
                                            resume_from_seq = resume_from,
                                            "Output subscription lagged, attempting replay recovery"
                                        );

                                        match process_manager_output.subscribe_output(&terminal_id_output, Some(resume_from)).await {
                                            Ok(new_sub) => {
                                                output_subscription = new_sub;
                                                tracing::info!(
                                                    terminal_id = %terminal_id_output,
                                                    resume_from_seq = resume_from,
                                                    "WS output subscription recovered after lag"
                                                );
                                            }
                                            Err(re_err) => {
                                                tracing::warn!(
                                                    terminal_id = %terminal_id_output,
                                                    error = %re_err,
                                                    "Failed to re-subscribe after lag, continuing with current subscription"
                                                );
                                            }
                                        }
                                        continue;
                                    }
                                    RecvError::Closed => {
                                        // Terminal output stream closed
                                        tracing::debug!(
                                            terminal_id = %terminal_id_output,
                                            "Output subscription closed"
                                        );
                                        break;
                                    }
                                }
                            }
                        }
                    }
                }
            }
        })
    };

    // Spawn PTY writer task (async channel -> blocking write)
    // Writer is shared via Arc<Mutex> to support WebSocket reconnection
    let writer_task = if let Some(pty_writer) = writer {
        Some(tokio::task::spawn_blocking(move || {
            while let Some(data) = ws_rx.blocking_recv() {
                // Lock the shared writer for each write operation
                // Recover from poisoned lock to allow continued input
                let mut writer_guard = match pty_writer.lock() {
                    Ok(guard) => guard,
                    Err(poisoned) => {
                        tracing::warn!(
                            "PTY writer lock was poisoned for terminal {}, recovering",
                            terminal_id_writer
                        );
                        poisoned.into_inner()
                    }
                };
                if let Err(e) = writer_guard.write_all(&data) {
                    tracing::error!("PTY write error for terminal {}: {}", terminal_id_writer, e);
                    break;
                }
                if let Err(e) = writer_guard.flush() {
                    tracing::error!("PTY flush error for terminal {}: {}", terminal_id_writer, e);
                    break;
                }
            }
        }))
    } else {
        tracing::warn!("No PTY writer available for terminal {}", terminal_id);
        None
    };

    // Spawn heartbeat task
    let heartbeat_task = {
        let idle_timeout_notifier = idle_timeout_notifier.clone();
        let idle_timeout_triggered = idle_timeout_triggered.clone();
        tokio::spawn(async move {
            let mut interval =
                tokio::time::interval(Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));
            loop {
                interval.tick().await;

                let last_ts = last_activity_heartbeat.load(Ordering::Relaxed);
                let idle_duration = elapsed_since_millis(last_ts);

                if idle_duration > Duration::from_secs(WS_IDLE_TIMEOUT_SECS) {
                    tracing::warn!(
                        "Terminal {} idle timeout after {}s",
                        terminal_id_heartbeat,
                        idle_duration.as_secs()
                    );
                    idle_timeout_triggered.store(true, Ordering::Release);
                    idle_timeout_notifier.notify_one();
                    return true;
                }

                // [G08-010] Detect half-open connections: if no client message
                // (input, resize, heartbeat) received for 90s, assume the client
                // is gone and close the connection server-side.
                let last_client_ts = last_client_message_heartbeat.load(Ordering::Relaxed);
                let client_silent_duration = elapsed_since_millis(last_client_ts);
                if client_silent_duration > Duration::from_secs(90) {
                    tracing::warn!(
                        terminal_id = %terminal_id_heartbeat,
                        silent_secs = client_silent_duration.as_secs(),
                        "Half-open connection detected: no client message for 90s, closing"
                    );
                    idle_timeout_triggered.store(true, Ordering::Release);
                    idle_timeout_notifier.notify_one();
                    return true;
                }

                tracing::trace!(
                    "Terminal {} heartbeat check, idle for {}s",
                    terminal_id_heartbeat,
                    idle_duration.as_secs()
                );
            }
        })
    };

    // Spawn WebSocket receive task (WebSocket input -> PTY)
    let recv_task = tokio::spawn(async move {
        loop {
            if let Some(result) = ws_receiver.next().await {
                // Update last activity time
                last_activity_recv.store(now_millis(), Ordering::Relaxed);
                // [G08-010] Update last client message time for half-open detection
                last_client_message_recv.store(now_millis(), Ordering::Relaxed);

                match result {
                    Ok(msg) => match msg {
                        Message::Text(text) => {
                            if let Ok(ws_msg) = serde_json::from_str::<WsMessage>(&text) {
                                match ws_msg {
                                    WsMessage::Input { data } => {
                                        // Send to PTY writer
                                        if let Err(e) =
                                            ws_tx_input.send(data.into_bytes()).await
                                        {
                                            tracing::error!("Failed to send to PTY: {}", e);
                                            break;
                                        }
                                    }
                                    WsMessage::Resize { cols, rows } => {
                                        // Resize PTY
                                        if let Err(e) = process_manager
                                            .resize(&terminal_id_resize, cols, rows)
                                            .await
                                        {
                                            tracing::warn!(
                                                "Failed to resize terminal {}: {}",
                                                terminal_id_resize,
                                                e
                                            );
                                        } else {
                                            tracing::debug!(
                                                "Terminal {} resized to {}x{}",
                                                terminal_id_resize,
                                                cols,
                                                rows
                                            );
                                        }
                                    }
                                    WsMessage::Heartbeat => {
                                        tracing::trace!("Received client heartbeat");
                                    }
                                    WsMessage::Output { .. } => {
                                        tracing::warn!("Client sent unexpected Output message");
                                    }
                                    WsMessage::Error { .. } => {
                                        tracing::warn!("Client sent unexpected Error message");
                                    }
                                }
                            } else {
                                tracing::warn!("Failed to parse WebSocket message: {}", text);
                            }
                        }
                        Message::Close(_) => {
                            tracing::info!("Client requested close");
                            break;
                        }
                        Message::Ping(data) => {
                            tracing::trace!("Received ping: {} bytes", data.len());
                        }
                        Message::Pong(data) => {
                            tracing::trace!("Received pong: {} bytes", data.len());
                        }
                        Message::Binary(data) => {
                            tracing::warn!(
                                "Received unexpected binary data: {} bytes",
                                data.len()
                            );
                        }
                    },
                    Err(e) => {
                        if is_benign_ws_disconnect(&e) {
                            tracing::debug!(
                                terminal_id = %terminal_id_recv,
                                error = %e,
                                "WebSocket disconnected"
                            );
                        } else {
                            tracing::error!(
                                terminal_id = %terminal_id_recv,
                                error = %e,
                                "WebSocket error"
                            );
                        }
                        break;
                    }
                }
            } else {
                tracing::debug!("WebSocket stream ended");
                break;
            }
        }
    });

    // Wait for any task to complete
    // Use conditional guards to prevent immediate exit when optional tasks are None
    let writer_task_active = writer_task.is_some();
    let mut output_task = output_task;
    let mut recv_task = recv_task;
    let mut heartbeat_task = heartbeat_task;
    let mut writer_task = writer_task;
    let mut terminated_by_idle_timeout = false;

    tokio::select! {
        _ = &mut output_task => {
            tracing::debug!("Output task completed for terminal {}", terminal_id);
        }
        _ = &mut recv_task => {
            tracing::debug!("Receive task completed for terminal {}", terminal_id);
        }
        heartbeat_result = &mut heartbeat_task => {
            match heartbeat_result {
                Ok(true) => {
                    terminated_by_idle_timeout = true;
                    tracing::debug!(
                        "Heartbeat task completed (idle timeout) for terminal {}",
                        terminal_id
                    );
                }
                Ok(false) => {
                    tracing::debug!("Heartbeat task completed for terminal {}", terminal_id);
                }
                Err(e) => {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        error = %e,
                        "Heartbeat task join failed"
                    );
                }
            }
        }
        () = async {
            if let Some(task) = &mut writer_task {
                let _ = task.await;
            }
        }, if writer_task_active => {
            tracing::debug!("PTY writer task completed for terminal {}", terminal_id);
        }
    }

    if terminated_by_idle_timeout {
        let graceful_shutdown_timeout = Duration::from_secs(2);

        if let Err(_elapsed) = tokio::time::timeout(graceful_shutdown_timeout, &mut output_task).await
        {
            tracing::debug!(
                terminal_id = %terminal_id,
                "Timed out waiting for output task to finish graceful idle-timeout close"
            );
        }

        if let Err(_elapsed) = tokio::time::timeout(graceful_shutdown_timeout, &mut recv_task).await
        {
            tracing::debug!(
                terminal_id = %terminal_id,
                "Timed out waiting for receive task to finish graceful idle-timeout close"
            );
        }
    }

    // Abort any remaining background tasks to avoid leaks
    output_task.abort();
    recv_task.abort();
    heartbeat_task.abort();
    if let Some(task) = writer_task {
        task.abort();
    }

    tracing::info!("Terminal WebSocket disconnected: {}", terminal_id);
}

async fn sync_prompt_watcher_registration(
    terminal_id: &str,
    prompt_watcher: services::services::terminal::prompt_watcher::PromptWatcher,
    db_pool: sqlx::SqlitePool,
    active_session_id: Option<&str>,
) {
    let terminal = match Terminal::find_by_id(&db_pool, terminal_id).await {
        Ok(Some(terminal)) => terminal,
        Ok(None) => {
            tracing::warn!(
                terminal_id = %terminal_id,
                "Skipped prompt watcher auto-registration: terminal not found in database"
            );
            return;
        }
        Err(e) => {
            tracing::warn!(
                terminal_id = %terminal_id,
                error = %e,
                "Failed to query terminal for prompt watcher auto-registration"
            );
            return;
        }
    };

    if prompt_watcher.is_registered(terminal_id).await {
        tracing::trace!(
            terminal_id = %terminal_id,
            "Terminal already registered for prompt watching"
        );
        return;
    }

    let task_id = terminal.workflow_task_id.clone();
    let workflow_id = match WorkflowTask::find_by_id(&db_pool, &task_id).await {
        Ok(Some(task)) => task.workflow_id,
        Ok(None) => {
            tracing::warn!(
                terminal_id = %terminal_id,
                workflow_task_id = %task_id,
                "Skipped prompt watcher auto-registration: workflow task not found"
            );
            return;
        }
        Err(e) => {
            tracing::warn!(
                terminal_id = %terminal_id,
                workflow_task_id = %task_id,
                error = %e,
                "Failed to query workflow task for prompt watcher auto-registration"
            );
            return;
        }
    };

    let pty_session_id = active_session_id
        .map(std::string::ToString::to_string)
        .filter(|session_id| !session_id.trim().is_empty())
        .or_else(|| {
            terminal
                .pty_session_id
                .filter(|session_id| !session_id.trim().is_empty())
        });

    let Some(session_id) = pty_session_id else {
        tracing::warn!(
            terminal_id = %terminal_id,
            task_id = %task_id,
            "Skipped prompt watcher auto-registration: missing pty_session_id"
        );
        return;
    };

    if let Err(e) = prompt_watcher
        .register(
            terminal_id,
            &workflow_id,
            &task_id,
            &session_id,
            terminal.auto_confirm,
        )
        .await
    {
        tracing::warn!(
            terminal_id = %terminal_id,
            workflow_id = %workflow_id,
            task_id = %task_id,
            error = %e,
            "Failed to auto-register terminal for prompt watching"
        );
    } else {
        tracing::info!(
            terminal_id = %terminal_id,
            workflow_id = %workflow_id,
            task_id = %task_id,
            pty_session_id = %session_id,
            "Auto-registered terminal for prompt watching"
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ws_message_serialization() {
        // Test Input message
        let input = WsMessage::Input {
            data: "ls -la".to_string(),
        };
        let json = serde_json::to_string(&input).unwrap();
        assert!(json.contains("input"));
        assert!(json.contains("ls -la"));

        // Test Output message
        let output = WsMessage::Output {
            data: "total 0".to_string(),
        };
        let json = serde_json::to_string(&output).unwrap();
        assert!(json.contains("output"));
        assert!(json.contains("total 0"));

        // Test Resize message
        let resize = WsMessage::Resize { cols: 80, rows: 24 };
        let json = serde_json::to_string(&resize).unwrap();
        assert!(json.contains("resize"));
        assert!(json.contains("80"));
        assert!(json.contains("24"));

        // Test Error message
        let error = WsMessage::Error {
            message: "Test error".to_string(),
        };
        let json = serde_json::to_string(&error).unwrap();
        assert!(json.contains("error"));
        assert!(json.contains("Test error"));

        // Test Heartbeat message
        let heartbeat = WsMessage::Heartbeat;
        let json = serde_json::to_string(&heartbeat).unwrap();
        assert!(json.contains("heartbeat"));
    }

    #[test]
    fn test_ws_message_deserialization() {
        // Test Input message
        let json = r#"{"type":"input","data":"test"}"#;
        let msg: WsMessage = serde_json::from_str(json).unwrap();
        match msg {
            WsMessage::Input { data } => {
                assert_eq!(data, "test");
            }
            _ => panic!("Expected Input message"),
        }

        // Test Resize message
        let json = r#"{"type":"resize","cols":120,"rows":30}"#;
        let msg: WsMessage = serde_json::from_str(json).unwrap();
        match msg {
            WsMessage::Resize { cols, rows } => {
                assert_eq!(cols, 120);
                assert_eq!(rows, 30);
            }
            _ => panic!("Expected Resize message"),
        }

        // Test Heartbeat message
        let json = r#"{"type":"heartbeat"}"#;
        let msg: WsMessage = serde_json::from_str(json).unwrap();
        assert!(matches!(msg, WsMessage::Heartbeat));
    }

    #[test]
    fn test_validate_terminal_id_valid() {
        assert!(validate_terminal_id("550e8400-e29b-41d4-a716-446655440000").is_ok());
        assert!(validate_terminal_id("550E8400-E29B-41D4-A716-446655440000").is_ok());
    }

    #[test]
    fn test_validate_terminal_id_invalid() {
        assert!(validate_terminal_id("invalid").is_err());
        assert!(validate_terminal_id("550e8400-e29b-41d4-a716").is_err());
        assert!(validate_terminal_id("").is_err());
    }
}
