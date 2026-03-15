use anyhow::Result;
use futures_util::{SinkExt, StreamExt};
use std::sync::Arc;
use tokio::sync::{mpsc, Mutex, RwLock};
use tokio::task::JoinHandle;
use tokio_tungstenite::tungstenite::Message;
use tokio_util::sync::CancellationToken;
use tracing;

use crate::auth::FeishuAuth;
use crate::events::FeishuEvent;
use crate::types::{ClientConfig, FeishuConfig};

pub struct FeishuClient {
    auth: Arc<FeishuAuth>,
    config: Arc<RwLock<ClientConfig>>,
    event_tx: mpsc::Sender<FeishuEvent>,
    connected: Arc<RwLock<bool>>,
    /// G32-004: Saved ping task handle for cleanup on disconnect.
    ping_handle: Mutex<Option<JoinHandle<()>>>,
    /// G32-003: Token to cancel the ping loop immediately on disconnect.
    cancel_token: CancellationToken,
}

impl FeishuClient {
    pub fn new(feishu_config: FeishuConfig) -> (Self, mpsc::Receiver<FeishuEvent>) {
        let (tx, rx) = mpsc::channel(100);
        let auth = Arc::new(FeishuAuth::new(feishu_config));
        let client = Self {
            auth,
            config: Arc::new(RwLock::new(ClientConfig::default())),
            event_tx: tx,
            connected: Arc::new(RwLock::new(false)),
            ping_handle: Mutex::new(None),
            cancel_token: CancellationToken::new(),
        };
        (client, rx)
    }

    /// Get a reference to the auth module (for creating FeishuMessenger)
    pub fn auth(&self) -> &Arc<FeishuAuth> {
        &self.auth
    }

    /// Start the WebSocket connection loop (runs until disconnected)
    pub async fn connect(&self) -> Result<()> {
        let endpoint = self.auth.acquire_ws_endpoint().await?;
        let data = endpoint
            .data
            .ok_or_else(|| anyhow::anyhow!("No endpoint data in response (code={})", endpoint.code))?;

        if let Some(cfg) = data.client_config {
            *self.config.write().await = cfg;
        }

        tracing::info!(url = %data.url, "Connecting to Feishu WebSocket");

        let (ws_stream, _) = tokio_tungstenite::connect_async(&data.url).await?;
        // G32-002: Set connected=true only after connect_async succeeds
        *self.connected.write().await = true;

        let (write, mut read) = ws_stream.split();
        let write = Arc::new(Mutex::new(write));

        // G32-003: Create a new cancellation token for this connection
        let cancel = self.cancel_token.child_token();

        // Spawn ping loop with CancellationToken
        let ping_config = self.config.clone();
        let ping_write = write.clone();
        let ping_cancel = cancel.clone();
        let handle = tokio::spawn(async move {
            loop {
                let interval = ping_config.read().await.ping_interval;
                // G32-003: Use select! so cancellation breaks out immediately
                tokio::select! {
                    () = tokio::time::sleep(tokio::time::Duration::from_secs(interval)) => {}
                    () = ping_cancel.cancelled() => {
                        break;
                    }
                }
                let mut w = ping_write.lock().await;
                if w.send(Message::Ping(vec![].into())).await.is_err() {
                    break;
                }
            }
        });

        // G32-004: Save the ping task handle for cleanup
        *self.ping_handle.lock().await = Some(handle);

        // Message receive loop
        let event_tx = self.event_tx.clone();
        let config = self.config.clone();
        while let Some(msg) = read.next().await {
            match msg {
                Ok(Message::Text(text)) => {
                    match serde_json::from_str::<FeishuEvent>(&text) {
                        Ok(event) => {
                            if event_tx.send(event).await.is_err() {
                                tracing::warn!("Event channel closed");
                                break;
                            }
                        }
                        Err(e) => {
                            // G32-012: Warn-level because a parse failure may indicate
                            // an unexpected event schema change from Feishu.
                            tracing::warn!(error = %e, raw = %text, "Failed to parse Feishu event");
                        }
                    }
                }
                Ok(Message::Pong(payload)) => {
                    // Feishu may send updated ClientConfig in pong frames
                    if let Ok(text) = std::str::from_utf8(&payload) {
                        if let Ok(cfg) = serde_json::from_str::<ClientConfig>(text) {
                            tracing::debug!(?cfg, "Updated client config from pong");
                            *config.write().await = cfg;
                        }
                    }
                }
                Ok(Message::Close(_)) => {
                    tracing::info!("Feishu WebSocket closed by server");
                    break;
                }
                Err(e) => {
                    tracing::error!(error = %e, "Feishu WebSocket error");
                    break;
                }
                _ => {}
            }
        }

        *self.connected.write().await = false;
        // G32-003 + G32-004: Cancel ping loop and await its handle
        self.cancel_token.cancel();
        if let Some(handle) = self.ping_handle.lock().await.take() {
            handle.abort();
            let _ = handle.await;
        }
        Ok(())
    }

    pub async fn is_connected(&self) -> bool {
        *self.connected.read().await
    }
}
