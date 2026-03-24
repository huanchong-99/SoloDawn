//! Message broadcasting and cross-channel synchronization.

use std::sync::Arc;

use dashmap::DashMap;
use db::models::concierge::{ConciergeMessage, ConciergeSession};
use serde::{Deserialize, Serialize};
use tokio::sync::broadcast;
use tracing;

// ---------------------------------------------------------------------------
// Event types
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ConciergeEvent {
    NewMessage {
        message: ConciergeMessage,
    },
    ToolExecuting {
        tool: String,
        status: String,
    },
    SessionUpdated {
        session: ConciergeSession,
    },
}

// ---------------------------------------------------------------------------
// Broadcaster
// ---------------------------------------------------------------------------

/// Manages real-time event distribution across all channels bound to a session.
///
/// Web clients subscribe via WebSocket; Feishu channels are pushed via messenger.
pub struct ConciergeBroadcaster {
    /// session_id → broadcast sender for Web WS subscribers
    web_channels: DashMap<String, broadcast::Sender<ConciergeEvent>>,
    /// session_id → (messenger_fn, chat_id) for Feishu push
    feishu_channels: DashMap<String, FeishuTarget>,
}

/// Feishu push target: a callback that sends text to a specific chat.
#[derive(Clone)]
pub struct FeishuTarget {
    pub chat_id: String,
    /// Boxed async function to send text. We use a trait object to avoid
    /// importing feishu_connector types at the service layer.
    pub sender: Arc<dyn FeishuSender>,
}

#[async_trait::async_trait]
pub trait FeishuSender: Send + Sync {
    async fn send_text(&self, chat_id: &str, text: &str) -> anyhow::Result<String>;
}

impl ConciergeBroadcaster {
    pub fn new() -> Self {
        Self {
            web_channels: DashMap::new(),
            feishu_channels: DashMap::new(),
        }
    }

    /// Subscribe to a session's events (for Web WS clients).
    pub fn subscribe(&self, session_id: &str) -> broadcast::Receiver<ConciergeEvent> {
        let entry = self
            .web_channels
            .entry(session_id.to_string())
            .or_insert_with(|| broadcast::channel(256).0);
        entry.subscribe()
    }

    /// Register a Feishu chat as a push target for a session.
    pub fn register_feishu(&self, session_id: &str, target: FeishuTarget) {
        self.feishu_channels
            .insert(session_id.to_string(), target);
    }

    /// Remove a Feishu target for a session.
    pub fn unregister_feishu(&self, session_id: &str) {
        self.feishu_channels.remove(session_id);
    }

    /// Broadcast an event to all channels of a session.
    ///
    /// - Web WS: sends via broadcast channel
    /// - Feishu: sends text message (messages always if feishu_sync is on;
    ///   tool events only if sync_tools is also on)
    pub async fn broadcast(
        &self,
        session_id: &str,
        event: ConciergeEvent,
        feishu_sync: bool,
        source_provider: Option<&str>,
    ) {
        self.broadcast_with_toggles(session_id, event, feishu_sync, false, source_provider)
            .await;
    }

    /// Broadcast with granular sync toggles.
    ///
    /// - `feishu_sync`: master on/off for Feishu push
    /// - `sync_tools`: when true, also push `ToolExecuting` events to Feishu
    pub async fn broadcast_with_toggles(
        &self,
        session_id: &str,
        event: ConciergeEvent,
        feishu_sync: bool,
        sync_tools: bool,
        source_provider: Option<&str>,
    ) {
        // Push to Web WS subscribers
        if let Some(sender) = self.web_channels.get(session_id) {
            // Ignore send errors (no active subscribers)
            let _ = sender.send(event.clone());
        }

        // Push to Feishu (if sync enabled)
        if feishu_sync {
            match &event {
                ConciergeEvent::NewMessage { message } => {
                    // Don't echo back to Feishu if the message came from Feishu
                    let from_feishu = source_provider == Some("feishu");
                    if !from_feishu {
                        self.push_text_to_feishu(session_id, &message.content)
                            .await;
                    }
                }
                ConciergeEvent::ToolExecuting { tool, status } if sync_tools => {
                    let text = format!("\u{1f527} {tool}: {status}");
                    self.push_text_to_feishu(session_id, &text).await;
                }
                _ => {} // SessionUpdated, ToolExecuting without sync_tools — skip
            }
        }
    }

    /// Push a completion notification to all Feishu-synced sessions for a workflow.
    pub async fn push_completion_notification(
        &self,
        session_id: &str,
        text: &str,
    ) {
        self.push_text_to_feishu(session_id, text).await;
    }

    /// Internal helper: push text to the Feishu target for a session.
    async fn push_text_to_feishu(&self, session_id: &str, text: &str) {
        if let Some(target) = self.feishu_channels.get(session_id) {
            let text = text.to_string();
            let chat_id = target.chat_id.clone();
            let sender = target.sender.clone();
            tokio::spawn(async move {
                if let Err(e) = sender.send_text(&chat_id, &text).await {
                    tracing::warn!(
                        "Failed to push concierge message to Feishu: {e}"
                    );
                }
            });
        }
    }

    /// Cleanup: remove all channels for a session.
    pub fn remove_session(&self, session_id: &str) {
        self.web_channels.remove(session_id);
        self.feishu_channels.remove(session_id);
    }
}

impl Default for ConciergeBroadcaster {
    fn default() -> Self {
        Self::new()
    }
}
