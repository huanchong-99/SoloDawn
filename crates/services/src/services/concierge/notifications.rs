//! Workflow event monitoring for completion reports and progress push.

use std::sync::Arc;

use sqlx::SqlitePool;
use tokio::sync::mpsc;
use tracing;

use super::sync::{ConciergeBroadcaster, ConciergeEvent};
use db::models::concierge::{ConciergeMessage, ConciergeSession};

/// Classify bus messages into notification categories for toggle filtering.
enum NotificationKind {
    /// Terminal or task completion — always pushed (core event).
    Completion,
    /// Terminal status transitions — controlled by `sync_terminal`.
    Terminal,
    /// Git commits and other progress — controlled by `sync_progress`.
    Progress,
}

/// Subscribe to a workflow's message bus and forward relevant events
/// to the Concierge session as system messages.
///
/// Respects independent sync toggles:
/// - Completion events (task/terminal completed/failed) are always saved locally;
///   pushed to Feishu only if `notify_on_completion` is true.
/// - Terminal status events are pushed only if `sync_terminal` is true.
/// - Progress events (git, etc.) are pushed only if `sync_progress` is true.
pub async fn watch_workflow_events(
    session_id: String,
    _workflow_id: String,
    pool: SqlitePool,
    broadcaster: Arc<ConciergeBroadcaster>,
    mut workflow_rx: mpsc::Receiver<crate::services::orchestrator::message_bus::BusMessage>,
) {
    use crate::services::orchestrator::message_bus::BusMessage;

    while let Some(bus_msg) = workflow_rx.recv().await {
        // Reload session to check current settings
        let session = match ConciergeSession::find_by_id(&pool, &session_id).await {
            Ok(Some(s)) => s,
            _ => continue,
        };

        let (text, kind) = match &bus_msg {
            BusMessage::TerminalCompleted(event) => {
                let status_str = format!("{:?}", event.status);
                (
                    format!(
                        "[Task Update] Terminal {} {} (task: {})",
                        event.terminal_id, status_str, event.task_id
                    ),
                    NotificationKind::Completion,
                )
            }
            BusMessage::TaskStatusUpdate {
                task_id, status, ..
            } if status == "completed" || status == "failed" => (
                format!("[Task Update] Task {task_id} {status}"),
                NotificationKind::Completion,
            ),
            BusMessage::GitEvent {
                commit_hash,
                branch,
                message,
                ..
            } => (
                format!(
                    "[Git] Commit {} on {}: {}",
                    &commit_hash[..8.min(commit_hash.len())],
                    branch,
                    message
                ),
                NotificationKind::Progress,
            ),
            BusMessage::TerminalStatusUpdate {
                terminal_id,
                status,
                ..
            } => (
                format!("[Terminal] {terminal_id} \u{2192} {status}"),
                NotificationKind::Terminal,
            ),
            BusMessage::Shutdown => {
                tracing::debug!(
                    session_id = %session_id,
                    "Workflow shut down, stopping notification watcher"
                );
                break;
            }
            _ => continue,
        };

        // Check toggle-based filtering
        let should_push_feishu = session.feishu_sync
            && match kind {
                NotificationKind::Completion => session.notify_on_completion,
                NotificationKind::Terminal => session.sync_terminal,
                NotificationKind::Progress => session.sync_progress,
            };

        // Legacy check: skip progress events if progress_notifications is disabled
        // (for backward compatibility with the old single toggle)
        let should_save = match kind {
            NotificationKind::Progress => session.progress_notifications || session.sync_progress,
            _ => true,
        };

        if !should_save && !should_push_feishu {
            continue;
        }

        // Save as system message and broadcast
        if should_save {
            let msg = ConciergeMessage::new_system(&session_id, &text);
            if let Err(e) = ConciergeMessage::insert(&pool, &msg).await {
                tracing::warn!("Failed to save notification message: {e}");
                continue;
            }

            // Always broadcast to Web WS; conditionally push to Feishu
            broadcaster
                .broadcast(
                    &session_id,
                    ConciergeEvent::NewMessage { message: msg },
                    should_push_feishu,
                    None,
                )
                .await;
        } else if should_push_feishu {
            // Only push to Feishu without saving locally
            broadcaster
                .push_completion_notification(&session_id, &text)
                .await;
        }
    }
}

/// Push a workflow completion summary to all Feishu-synced sessions.
///
/// Called by the orchestrator agent when a workflow completes.
pub async fn push_workflow_completion(
    pool: &SqlitePool,
    broadcaster: &ConciergeBroadcaster,
    workflow_id: &str,
    workflow_name: &str,
    status: &str,
    task_summary: &str,
) {
    let sessions = match ConciergeSession::find_by_workflow_with_feishu(pool, workflow_id).await {
        Ok(s) => s,
        Err(e) => {
            tracing::warn!("Failed to find sessions for completion notification: {e}");
            return;
        }
    };

    for session in &sessions {
        if !session.notify_on_completion {
            continue;
        }

        let text = format!(
            "\u{1f3c1} Workflow completed: {workflow_name}\nStatus: {status}\n{task_summary}"
        );

        // Save as system message
        let msg = ConciergeMessage::new_system(&session.id, &text);
        if let Err(e) = ConciergeMessage::insert(pool, &msg).await {
            tracing::warn!(
                session_id = %session.id,
                "Failed to save completion notification: {e}"
            );
            continue;
        }

        // Broadcast to web and Feishu
        broadcaster
            .broadcast(
                &session.id,
                ConciergeEvent::NewMessage { message: msg },
                true, // feishu_sync is guaranteed true from the query
                None,
            )
            .await;
    }
}
