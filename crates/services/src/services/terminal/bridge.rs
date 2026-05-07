//! Message bus -> PTY input bridge.
//!
//! Subscribes to PTY session topics and forwards terminal messages to stdin.
//! This enables the Orchestrator to send commands and confirmations to CLI tools.

use std::{collections::HashMap, sync::Arc, time::Duration};

use tokio::{
    sync::{RwLock, mpsc, oneshot},
    task::JoinHandle,
    time::MissedTickBehavior,
};

use super::process::ProcessManager;
use crate::services::orchestrator::message_bus::{BusMessage, SharedMessageBus};

// ============================================================================
// Constants
// ============================================================================

/// Channel capacity for PTY writer queue.
const BRIDGE_CHANNEL_CAPACITY: usize = 100;

/// Health check interval for terminal liveness (seconds).
const BRIDGE_HEALTH_INTERVAL_SECS: u64 = 5;

// ============================================================================
// Bridge Handle
// ============================================================================

/// Internal handle for tracking active bridge tasks.
struct BridgeHandle {
    terminal_id: String,
    task_handle: JoinHandle<()>,
}

// ============================================================================
// Terminal Bridge
// ============================================================================

/// Bridges MessageBus terminal topics to PTY stdin.
///
/// This component enables bidirectional communication between the Orchestrator
/// and CLI tools running in PTY terminals. When the Orchestrator publishes a
/// `TerminalMessage` to a PTY session topic, this bridge forwards it to the
/// terminal's stdin.
///
/// # Architecture
///
/// ```text
/// Orchestrator -> MessageBus -> TerminalBridge -> PTY stdin
/// ```
///
/// # Usage
///
/// ```ignore
/// let bridge = TerminalBridge::new(message_bus, process_manager);
/// bridge.register("terminal-123", "session-456").await?;
/// ```
#[derive(Clone)]
pub struct TerminalBridge {
    message_bus: SharedMessageBus,
    process_manager: Arc<ProcessManager>,
    active_sessions: Arc<RwLock<HashMap<String, BridgeHandle>>>,
}

impl TerminalBridge {
    /// Creates a new TerminalBridge instance.
    ///
    /// # Arguments
    ///
    /// * `message_bus` - Shared message bus for subscribing to topics
    /// * `process_manager` - Process manager for accessing PTY handles
    pub fn new(message_bus: SharedMessageBus, process_manager: Arc<ProcessManager>) -> Self {
        Self {
            message_bus,
            process_manager,
            active_sessions: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Registers a bridge task for a PTY session topic.
    ///
    /// This method subscribes to the PTY session topic and spawns a background
    /// task that forwards incoming `TerminalMessage` events to the PTY stdin.
    ///
    /// # Arguments
    ///
    /// * `terminal_id` - Terminal identifier for PTY handle lookup
    /// * `pty_session_id` - PTY session ID used as the message bus topic
    ///
    /// # Returns
    ///
    /// Returns `Ok(())` if registration succeeds, or an error if the session ID
    /// is empty or registration fails.
    pub async fn register(&self, terminal_id: &str, pty_session_id: &str) -> anyhow::Result<()> {
        // Backward-compatible shim: await the ready signal internally so existing
        // callers still observe the old blocking semantics.
        let ready_rx = self
            .register_with_ready(terminal_id, pty_session_id)
            .await?;
        // Await readiness with a bounded timeout. A dropped sender (`Err(_)`) is
        // benign — it means the bridge task was aborted (typically because another
        // registration for the same terminal won) and the caller should just
        // proceed. An elapsed timeout, however, indicates the task never started
        // executing within 2s, which is a real bug worth surfacing instead of
        // silently returning Ok(()).
        match tokio::time::timeout(Duration::from_secs(2), ready_rx).await {
            Ok(Ok(())) => Ok(()),
            Ok(Err(_)) => Ok(()), // task aborted before signalling — benign
            Err(_elapsed) => {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    pty_session_id = %pty_session_id,
                    "Terminal bridge did not become ready within 2s; proceeding anyway"
                );
                Ok(())
            }
        }
    }

    /// Registers a bridge task and returns a [`oneshot::Receiver`] that completes
    /// once the bridge task has started executing.
    ///
    /// Callers can await the receiver (ideally with a timeout) to obtain a
    /// deterministic "bridge ready" signal instead of polling or sleeping.
    ///
    /// Note: the receiver yields `Err(_)` if the bridge task is aborted before it
    /// can signal readiness (e.g. registration race with another caller).
    pub async fn register_with_ready(
        &self,
        terminal_id: &str,
        pty_session_id: &str,
    ) -> anyhow::Result<oneshot::Receiver<()>> {
        let session_id = pty_session_id.trim();
        if session_id.is_empty() {
            return Err(anyhow::anyhow!("pty_session_id is empty"));
        }
        if terminal_id.trim().is_empty() {
            return Err(anyhow::anyhow!("terminal_id is empty"));
        }

        // Helper: a pre-completed ready receiver for early-return paths where no
        // new bridge task is spawned (the bridge is effectively "already ready").
        let already_ready = || {
            let (tx, rx) = oneshot::channel::<()>();
            let _ = tx.send(());
            rx
        };

        // Keep exactly one active bridge per terminal.
        // A terminal restart may produce a new PTY session while an old bridge task
        // is still alive; evict stale bridge tasks eagerly.
        {
            let mut active = self.active_sessions.write().await;
            if active.contains_key(session_id) {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    pty_session_id = %session_id,
                    "Terminal bridge already registered"
                );
                return Ok(already_ready());
            }

            let stale_session_ids: Vec<String> = active
                .iter()
                .filter(|(existing_session_id, handle)| {
                    handle.terminal_id == terminal_id && existing_session_id.as_str() != session_id
                })
                .map(|(existing_session_id, _)| existing_session_id.clone())
                .collect();

            for stale_session_id in stale_session_ids {
                if let Some(stale_handle) = active.remove(&stale_session_id) {
                    stale_handle.task_handle.abort();
                    tracing::info!(
                        terminal_id = %terminal_id,
                        stale_pty_session_id = %stale_session_id,
                        new_pty_session_id = %session_id,
                        "Evicted stale terminal bridge before registering new session"
                    );
                }
            }
        }

        // Subscribe to both the new terminal-input topic and legacy PTY-session topic
        let terminal_input_topic = format!("terminal.input.{terminal_id}");
        let mut rx_terminal_input = self.message_bus.subscribe(&terminal_input_topic).await;
        let mut rx_legacy = self.message_bus.subscribe(session_id).await;

        let process_manager = Arc::clone(&self.process_manager);
        let session_id_owned = session_id.to_string();
        let terminal_input_topic_owned = terminal_input_topic.clone();
        let terminal_id_owned = terminal_id.to_string();
        let terminal_id_for_task = terminal_id.to_string();
        let active_sessions = Arc::clone(&self.active_sessions);

        // E26-07: channel-based ready signal. The bridge task sends `()` once it
        // begins executing, letting callers await a deterministic readiness
        // notification instead of polling `is_registered` after a fixed sleep.
        let (ready_tx, ready_rx) = oneshot::channel::<()>();

        // Spawn bridge task
        let task_handle = tokio::spawn(async move {
            // Signal readiness as soon as the task is scheduled. Ignore send errors
            // (receiver may have been dropped if the caller gave up waiting).
            let _ = ready_tx.send(());

            let result = Self::run_bridge(
                process_manager,
                terminal_id_for_task.clone(),
                session_id_owned.clone(),
                terminal_input_topic_owned.clone(),
                &mut rx_terminal_input,
                &mut rx_legacy,
            )
            .await;

            // Remove from active sessions on exit
            {
                let mut active = active_sessions.write().await;
                active.remove(&session_id_owned);
            }

            match result {
                Ok(()) => {
                    tracing::debug!(
                        terminal_id = %terminal_id_for_task,
                        pty_session_id = %session_id_owned,
                        "Terminal bridge stopped gracefully"
                    );
                }
                Err(e) => {
                    tracing::warn!(
                        terminal_id = %terminal_id_for_task,
                        pty_session_id = %session_id_owned,
                        error = %e,
                        "Terminal bridge stopped with error"
                    );
                }
            }
        });

        // Register the bridge handle (with race condition check)
        {
            let mut active = self.active_sessions.write().await;
            if active.contains_key(session_id) {
                // Another task registered while we were setting up
                task_handle.abort();
                tracing::debug!(
                    terminal_id = %terminal_id,
                    pty_session_id = %session_id,
                    "Terminal bridge registration race: existing session kept"
                );
                return Ok(already_ready());
            }

            active.insert(
                session_id.to_string(),
                BridgeHandle {
                    terminal_id: terminal_id_owned,
                    task_handle,
                },
            );
        }

        tracing::info!(
            terminal_id = %terminal_id,
            pty_session_id = %session_id,
            terminal_input_topic = %terminal_input_topic,
            "Terminal bridge registered"
        );

        Ok(ready_rx)
    }

    /// Unregisters a bridge task for a PTY session.
    ///
    /// # Arguments
    ///
    /// * `pty_session_id` - PTY session ID to unregister
    pub async fn unregister(&self, pty_session_id: &str) {
        let mut active = self.active_sessions.write().await;
        if let Some(handle) = active.remove(pty_session_id) {
            handle.task_handle.abort();
            tracing::info!(
                terminal_id = %handle.terminal_id,
                pty_session_id = %pty_session_id,
                "Terminal bridge unregistered"
            );
        }
    }

    /// Returns the number of active bridge sessions.
    pub async fn active_count(&self) -> usize {
        self.active_sessions.read().await.len()
    }

    /// Returns whether a PTY session currently has an active bridge task.
    pub async fn is_registered(&self, pty_session_id: &str) -> bool {
        self.active_sessions
            .read()
            .await
            .contains_key(pty_session_id)
    }

    /// Maximum input payload size accepted from the bus before forwarding to PTY stdin.
    /// Inputs larger than this (1 MiB) are dropped with a warning to guard against
    /// oversized/malicious payloads exhausting the PTY writer. See E26-11.
    const MAX_INPUT_LEN: usize = 1024 * 1024;

    /// Helper method to forward a bus message to PTY stdin.
    async fn forward_bus_message(
        tx: &mpsc::Sender<Vec<u8>>,
        terminal_id: &str,
        pty_session_id: &str,
        msg: Option<BusMessage>,
    ) -> anyhow::Result<bool> {
        // E26-11: validate input length at entry before any processing.
        if let Some(ref bus_msg) = msg {
            let incoming_len = match bus_msg {
                BusMessage::TerminalMessage { message } => message.len(),
                BusMessage::TerminalInput { input, .. } => input.len(),
                _ => 0,
            };
            if incoming_len > Self::MAX_INPUT_LEN {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    pty_session_id = %pty_session_id,
                    incoming_len,
                    max_len = Self::MAX_INPUT_LEN,
                    "Dropping oversized terminal input message"
                );
                return Ok(false);
            }
        }
        match msg {
            Some(BusMessage::TerminalMessage { message }) => {
                let payload = Self::normalize_message(&message);
                if payload.is_empty() {
                    return Ok(false);
                }
                tracing::debug!(
                    terminal_id = %terminal_id,
                    pty_session_id = %pty_session_id,
                    message_len = payload.len(),
                    "Forwarding legacy TerminalMessage to PTY stdin"
                );
                tx.send(payload.into_bytes())
                    .await
                    .map_err(|_| anyhow::anyhow!("PTY writer channel closed"))?;
                Ok(false)
            }
            Some(BusMessage::TerminalInput {
                terminal_id: message_terminal_id,
                session_id: message_session_id,
                input,
                ..
            }) => {
                // Strict routing:
                // - if session_id is present, it must match current PTY session;
                // - only when session_id is absent, fallback to terminal_id matching.
                if !message_session_id.trim().is_empty() {
                    if message_session_id != pty_session_id {
                        return Ok(false);
                    }
                } else if message_terminal_id != terminal_id {
                    return Ok(false);
                }

                let payload = Self::normalize_message(&input);
                if payload.is_empty() {
                    return Ok(false);
                }
                tracing::debug!(
                    terminal_id = %terminal_id,
                    pty_session_id = %pty_session_id,
                    message_len = payload.len(),
                    "Forwarding TerminalInput to PTY stdin"
                );
                tx.send(payload.into_bytes())
                    .await
                    .map_err(|_| anyhow::anyhow!("PTY writer channel closed"))?;
                Ok(false)
            }
            Some(BusMessage::Shutdown) => {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    pty_session_id = %pty_session_id,
                    "Terminal bridge received shutdown"
                );
                Ok(true)
            }
            Some(_) => Ok(false),
            None => {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    pty_session_id = %pty_session_id,
                    "Terminal bridge channel closed"
                );
                Ok(true)
            }
        }
    }

    /// Main bridge loop that forwards messages to PTY stdin.
    async fn run_bridge(
        process_manager: Arc<ProcessManager>,
        terminal_id: String,
        pty_session_id: String,
        terminal_input_topic: String,
        rx_terminal_input: &mut mpsc::Receiver<BusMessage>,
        rx_legacy: &mut mpsc::Receiver<BusMessage>,
    ) -> anyhow::Result<()> {
        // Get PTY handle
        let handle = process_manager
            .get_handle(&terminal_id)
            .await
            .ok_or_else(|| anyhow::anyhow!("Terminal not running: {terminal_id}"))?;

        if handle.session_id != pty_session_id {
            return Err(anyhow::anyhow!(
                "PTY session mismatch for terminal {}: expected {}, got {}",
                terminal_id,
                pty_session_id,
                handle.session_id
            ));
        }

        let writer = handle
            .writer
            .ok_or_else(|| anyhow::anyhow!("PTY writer unavailable for terminal {terminal_id}"))?;

        // Create channel for writer task
        let (tx, mut writer_rx) = mpsc::channel::<Vec<u8>>(BRIDGE_CHANNEL_CAPACITY);
        let terminal_id_writer = terminal_id.clone();

        // Spawn blocking writer task
        //
        // E26-04: the mutex guard is acquired *inside* the loop and explicitly
        // dropped after each write+flush, so the `PtyWriter` is never held across
        // iterations or await points. In addition, the `Arc<Mutex<PtyWriter>>`
        // itself is explicitly dropped once the channel closes so the writer's
        // ref count is released promptly rather than at task-exit unwind.
        let mut writer_task = tokio::task::spawn_blocking(move || -> anyhow::Result<()> {
            let result: anyhow::Result<()> = 'writer_loop: {
                while let Some(data) = writer_rx.blocking_recv() {
                    let mut writer_guard = match writer.lock() {
                        Ok(guard) => guard,
                        Err(poisoned) => {
                            tracing::warn!(
                                terminal_id = %terminal_id_writer,
                                "PTY writer lock poisoned; recovering"
                            );
                            poisoned.into_inner()
                        }
                    };

                    if let Err(e) = writer_guard.write_all(&data) {
                        break 'writer_loop Err(anyhow::anyhow!("PTY write error: {e}"));
                    }
                    if let Err(e) = writer_guard.flush() {
                        break 'writer_loop Err(anyhow::anyhow!("PTY flush error: {e}"));
                    }

                    // Explicitly drop the mutex guard before the next
                    // `blocking_recv()` so we never hold the lock while idle.
                    drop(writer_guard);
                }
                Ok(())
            };
            // Drop the `Arc<Mutex<PtyWriter>>` before returning so the ref count
            // is released immediately rather than waiting for task-exit unwind.
            drop(writer);
            result
        });

        let mut writer_finished = false;
        let mut health_interval =
            tokio::time::interval(Duration::from_secs(BRIDGE_HEALTH_INTERVAL_SECS));
        health_interval.set_missed_tick_behavior(MissedTickBehavior::Skip);

        tracing::debug!(
            terminal_id = %terminal_id,
            pty_session_id = %pty_session_id,
            terminal_input_topic = %terminal_input_topic,
            "Terminal bridge loop started"
        );

        loop {
            tokio::select! {
                msg = rx_terminal_input.recv() => {
                    if Self::forward_bus_message(&tx, &terminal_id, &pty_session_id, msg).await? {
                        break;
                    }
                }
                msg = rx_legacy.recv() => {
                    if Self::forward_bus_message(&tx, &terminal_id, &pty_session_id, msg).await? {
                        break;
                    }
                }
                _ = health_interval.tick() => {
                    // [G21-010] Removed process_manager.cleanup() here: global dead-process
                    // scanning belongs to ProcessManager's own periodic task, not to each
                    // individual bridge instance (which would cause N redundant full scans).

                    // Check if terminal is still running
                    if !process_manager.is_running(&terminal_id).await {
                        tracing::info!(
                            terminal_id = %terminal_id,
                            pty_session_id = %pty_session_id,
                            "Terminal process no longer running; stopping bridge"
                        );
                        break;
                    }

                    // Stop stale bridge if this terminal has switched to a newer PTY session.
                    let Some(current_handle) = process_manager.get_handle(&terminal_id).await else {
                        tracing::info!(
                            terminal_id = %terminal_id,
                            pty_session_id = %pty_session_id,
                            "Terminal handle missing during health check; stopping bridge"
                        );
                        break;
                    };

                    if current_handle.session_id != pty_session_id {
                        tracing::info!(
                            terminal_id = %terminal_id,
                            bridge_session_id = %pty_session_id,
                            active_session_id = %current_handle.session_id,
                            "Detected stale terminal bridge session; stopping bridge"
                        );
                        break;
                    }
                }
                result = &mut writer_task => {
                    writer_finished = true;
                    // [E26-03] Writer task has exited; its `writer_rx` is now dropped,
                    // so any messages still buffered in the mpsc channel will be
                    // discarded. Surface the count so we know when writes were lost.
                    let pending = tx.max_capacity().saturating_sub(tx.capacity());
                    if pending > 0 {
                        tracing::warn!(
                            terminal_id = %terminal_id,
                            pty_session_id = %pty_session_id,
                            discarded_messages = pending,
                            "Writer task exited with buffered messages still in channel; they will be dropped"
                        );
                    }
                    match result {
                        Ok(Ok(())) => {}
                        Ok(Err(e)) => return Err(e),
                        Err(e) => return Err(anyhow::anyhow!("Writer task join error: {e}")),
                    }
                    break;
                }
            }
        }

        // Clean shutdown: close sender and wait for writer task
        drop(tx);

        if !writer_finished {
            // [G21-004] Apply a timeout so the bridge does not hang indefinitely
            // if the blocking writer task is stuck after the PTY has exited.
            match tokio::time::timeout(Duration::from_secs(5), writer_task).await {
                Ok(Ok(Ok(()))) => {}
                Ok(Ok(Err(e))) => return Err(e),
                Ok(Err(e)) => return Err(anyhow::anyhow!("Writer task join error: {e}")),
                Err(_elapsed) => {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        pty_session_id = %pty_session_id,
                        "Writer task did not finish within 5s after bridge shutdown; abandoning"
                    );
                }
            }
        }

        Ok(())
    }

    /// Normalizes a message by ensuring it ends with an Enter key payload.
    ///
    /// PTY-based TUIs are generally Enter-key driven (`\r`) rather than
    /// line-feed-driven (`\n`/`\r\n`).
    ///
    /// # Newline Normalization Convention (G07-005)
    ///
    /// All messages forwarded to PTY stdin are normalized to end with a single
    /// carriage return (`\r`), which is the standard Enter key representation
    /// in terminal emulators. The rules are:
    /// - `\r\n` (CRLF) → `\r` (strip the LF)
    /// - `\n` (LF) → `\r` (replace with CR)
    /// - `\r` (CR) → `\r` (keep as-is)
    /// - no trailing newline → append `\r`
    /// - empty string → `\r` (bare Enter key)
    ///
    /// # Arguments
    ///
    /// * `message` - The message to normalize
    ///
    /// # Returns
    ///
    /// The normalized message with appropriate line ending.
    fn normalize_message(message: &str) -> String {
        let mut payload = message.to_string();

        if payload.is_empty() {
            return "\r".to_string();
        }

        if payload.ends_with("\r\n") {
            payload.truncate(payload.len().saturating_sub(2));
            payload.push('\r');
            return payload;
        }

        if payload.ends_with('\n') {
            payload.pop();
            payload.push('\r');
            return payload;
        }

        if payload.ends_with('\r') {
            return payload;
        }

        payload.push('\r');
        payload
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_normalize_message_adds_newline() {
        let result = TerminalBridge::normalize_message("hello");
        assert!(result.ends_with('\r'), "Should end with carriage return");
    }

    #[test]
    fn test_normalize_message_preserves_existing_newline() {
        let result = TerminalBridge::normalize_message("hello\n");
        assert_eq!(result, "hello\r", "LF should normalize to carriage return");
    }

    #[test]
    fn test_normalize_message_converts_existing_crlf() {
        let result = TerminalBridge::normalize_message("hello\r\n");
        assert_eq!(
            result, "hello\r",
            "CRLF should normalize to a single carriage return"
        );
    }

    #[test]
    fn test_normalize_message_empty_string() {
        let result = TerminalBridge::normalize_message("");
        assert_eq!(result, "\r", "Empty string should normalize to Enter key");
    }

    #[tokio::test]
    async fn test_forward_terminal_input_rejects_mismatched_non_empty_session() {
        let (tx, mut rx) = mpsc::channel::<Vec<u8>>(1);
        let should_stop = TerminalBridge::forward_bus_message(
            &tx,
            "term-1",
            "session-1",
            Some(BusMessage::TerminalInput {
                terminal_id: "term-1".to_string(),
                session_id: "session-old".to_string(),
                input: "hello".to_string(),
                decision: None,
            }),
        )
        .await
        .expect("forward should not error");

        assert!(!should_stop);
        assert!(
            rx.try_recv().is_err(),
            "mismatched session must not be forwarded"
        );
    }

    #[tokio::test]
    async fn test_forward_terminal_input_allows_terminal_fallback_when_session_missing() {
        let (tx, mut rx) = mpsc::channel::<Vec<u8>>(1);
        let should_stop = TerminalBridge::forward_bus_message(
            &tx,
            "term-1",
            "session-1",
            Some(BusMessage::TerminalInput {
                terminal_id: "term-1".to_string(),
                session_id: String::new(),
                input: "hello".to_string(),
                decision: None,
            }),
        )
        .await
        .expect("forward should not error");

        assert!(!should_stop);
        let forwarded = rx.recv().await.expect("expected forwarded payload");
        assert_eq!(forwarded, b"hello\r");
    }
}
