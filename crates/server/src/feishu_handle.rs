//! Shared handle for the running Feishu service.

use std::sync::Arc;
use tokio::sync::RwLock;

use feishu_connector::events::FeishuEvent;
use feishu_connector::messages::FeishuMessenger;

/// Shared state for the Feishu connector, accessible from route handlers.
#[derive(Clone)]
pub struct FeishuHandle {
    /// Whether the WebSocket connection is currently active.
    pub connected: Arc<RwLock<bool>>,
    /// Signal to trigger a reconnect (drop old task, spawn new one).
    pub reconnect_tx: tokio::sync::mpsc::Sender<()>,
    /// Messenger for sending messages to Feishu.
    pub messenger: Arc<FeishuMessenger>,
    /// Broadcast channel for incoming Feishu events (used by test-receive).
    pub event_tx: tokio::sync::broadcast::Sender<FeishuEvent>,
    /// Last received chat_id (auto-captured from incoming messages for test-send).
    pub last_chat_id: Arc<RwLock<Option<String>>>,
}

pub type SharedFeishuHandle = Arc<RwLock<Option<FeishuHandle>>>;

pub fn new_shared_handle() -> SharedFeishuHandle {
    Arc::new(RwLock::new(None))
}
