//! Shared handle for the running Feishu service.

use std::sync::Arc;
use tokio::sync::RwLock;

/// Shared state for the Feishu connector, accessible from route handlers.
#[derive(Clone)]
pub struct FeishuHandle {
    /// Whether the WebSocket connection is currently active.
    pub connected: Arc<RwLock<bool>>,
    /// Signal to trigger a reconnect (drop old task, spawn new one).
    pub reconnect_tx: tokio::sync::mpsc::Sender<()>,
}

pub type SharedFeishuHandle = Arc<RwLock<Option<FeishuHandle>>>;

pub fn new_shared_handle() -> SharedFeishuHandle {
    Arc::new(RwLock::new(None))
}
