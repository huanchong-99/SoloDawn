use anyhow::Result;
use futures_util::{SinkExt, StreamExt};
use prost::Message as ProstMessage;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{mpsc, Mutex, RwLock};
use tokio::task::JoinHandle;
use tokio_tungstenite::tungstenite::Message;
use tokio_util::sync::CancellationToken;
use tracing;

use crate::auth::FeishuAuth;
use crate::events::FeishuEvent;
use crate::proto::{
    Frame, Header, HEADER_BIZ_RT, HEADER_MESSAGE_ID, HEADER_SEQ, HEADER_SUM, HEADER_TYPE,
    METHOD_CONTROL, METHOD_DATA, MSG_TYPE_EVENT, MSG_TYPE_PING, MSG_TYPE_PONG,
};
use crate::types::{ClientConfig, FeishuConfig};

/// Extract a header value by key from a Frame's headers.
fn get_header<'a>(headers: &'a [Header], key: &str) -> Option<&'a str> {
    headers.iter().find(|h| h.key == key).map(|h| h.value.as_str())
}

/// Build a ping Frame for the given service_id.
fn new_ping_frame(service_id: i32) -> Frame {
    Frame {
        method: METHOD_CONTROL,
        service: service_id,
        headers: vec![Header {
            key: HEADER_TYPE.to_string(),
            value: MSG_TYPE_PING.to_string(),
        }],
        ..Default::default()
    }
}

/// A single in-flight fragmented message being reassembled.
struct FragmentEntry {
    parts: Vec<Option<Vec<u8>>>,
    created: tokio::time::Instant,
}

/// Simple fragment cache with TTL for reassembling multi-part messages.
struct FragmentCache {
    entries: HashMap<String, FragmentEntry>,
}

impl FragmentCache {
    fn new() -> Self {
        Self {
            entries: HashMap::new(),
        }
    }

    /// Insert a fragment. Returns the reassembled payload when all parts arrive.
    fn insert(&mut self, msg_id: &str, sum: usize, seq: usize, data: Vec<u8>) -> Option<Vec<u8>> {
        let now = tokio::time::Instant::now();
        self.entries
            .retain(|_, e| now.duration_since(e.created).as_secs() < 5);

        let entry = self.entries.entry(msg_id.to_string()).or_insert_with(|| {
            FragmentEntry {
                parts: vec![None; sum],
                created: now,
            }
        });

        if seq < entry.parts.len() {
            entry.parts[seq] = Some(data);
        }

        if entry.parts.iter().all(|p| p.is_some()) {
            let entry = self.entries.remove(msg_id)?;
            let combined: Vec<u8> = entry
                .parts
                .into_iter()
                .flat_map(|p| p.unwrap_or_default())
                .collect();
            Some(combined)
        } else {
            None
        }
    }
}

pub struct FeishuClient {
    auth: Arc<FeishuAuth>,
    config: Arc<RwLock<ClientConfig>>,
    event_tx: mpsc::Sender<FeishuEvent>,
    connected: Arc<RwLock<bool>>,
    ping_handle: Mutex<Option<JoinHandle<()>>>,
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

        // Extract service_id from the WebSocket URL query params
        let service_id = url::Url::parse(&data.url)
            .ok()
            .and_then(|u| {
                u.query_pairs()
                    .find(|(k, _)| k == "service_id")
                    .and_then(|(_, v)| v.parse::<i32>().ok())
            })
            .unwrap_or(0);

        tracing::info!(url = %data.url, service_id, "Connecting to Feishu WebSocket");

        let (ws_stream, _) = tokio_tungstenite::connect_async(&data.url).await?;
        *self.connected.write().await = true;

        let (write, mut read) = ws_stream.split();
        let write = Arc::new(Mutex::new(write));

        let cancel = self.cancel_token.child_token();

        // Ping loop: send protobuf-encoded ping Frame
        let ping_config = self.config.clone();
        let ping_write = write.clone();
        let ping_cancel = cancel.clone();
        let handle = tokio::spawn(async move {
            loop {
                let interval = ping_config.read().await.ping_interval;
                tokio::select! {
                    () = tokio::time::sleep(tokio::time::Duration::from_secs(interval)) => {}
                    () = ping_cancel.cancelled() => {
                        break;
                    }
                }
                let frame = new_ping_frame(service_id);
                let bytes = frame.encode_to_vec();
                let mut w = ping_write.lock().await;
                if w.send(Message::Binary(bytes.into())).await.is_err() {
                    break;
                }
                tracing::debug!("Feishu ping sent");
            }
        });

        *self.ping_handle.lock().await = Some(handle);

        // Message receive loop
        let event_tx = self.event_tx.clone();
        let config = self.config.clone();
        let write_for_recv = write.clone();
        let mut fragment_cache = FragmentCache::new();

        while let Some(msg) = read.next().await {
            match &msg {
                Ok(m) => tracing::debug!(kind = ?m, len = m.len(), "WS message received"),
                Err(e) => tracing::warn!(error = %e, "WS recv error"),
            }
            match msg {
                Ok(Message::Binary(bytes)) => {
                    let frame = match Frame::decode(bytes.as_ref()) {
                        Ok(f) => f,
                        Err(e) => {
                            tracing::warn!(error = %e, "Failed to decode protobuf Frame");
                            continue;
                        }
                    };

                    match frame.method {
                        METHOD_CONTROL => {
                            Self::handle_control_frame(&frame, &config).await;
                        }
                        METHOD_DATA => {
                            Self::handle_data_frame(
                                frame,
                                &event_tx,
                                &write_for_recv,
                                &mut fragment_cache,
                            )
                            .await;
                        }
                        other => {
                            tracing::debug!(method = other, "Unknown frame method");
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
        self.cancel_token.cancel();
        if let Some(handle) = self.ping_handle.lock().await.take() {
            handle.abort();
            let _ = handle.await;
        }
        Ok(())
    }

    /// Handle a CONTROL frame (ping/pong).
    async fn handle_control_frame(frame: &Frame, config: &Arc<RwLock<ClientConfig>>) {
        let msg_type = get_header(&frame.headers, HEADER_TYPE).unwrap_or("");
        match msg_type {
            MSG_TYPE_PONG => {
                tracing::debug!("Feishu pong received");
                if !frame.payload.is_empty() {
                    if let Ok(text) = std::str::from_utf8(&frame.payload) {
                        if let Ok(cfg) = serde_json::from_str::<ClientConfig>(text) {
                            tracing::debug!(?cfg, "Updated client config from pong");
                            *config.write().await = cfg;
                        }
                    }
                }
            }
            MSG_TYPE_PING => {
                tracing::debug!("Feishu server ping received (ignoring)");
            }
            _ => {
                tracing::debug!(msg_type, "Unknown control frame type");
            }
        }
    }

    /// Handle a DATA frame (event/card).
    async fn handle_data_frame(
        mut frame: Frame,
        event_tx: &mpsc::Sender<FeishuEvent>,
        write: &Arc<Mutex<impl SinkExt<Message, Error = tokio_tungstenite::tungstenite::Error> + Unpin>>,
        fragment_cache: &mut FragmentCache,
    ) {
        let msg_type = get_header(&frame.headers, HEADER_TYPE).unwrap_or("");
        let msg_id = get_header(&frame.headers, HEADER_MESSAGE_ID)
            .unwrap_or("")
            .to_string();
        let sum: usize = get_header(&frame.headers, HEADER_SUM)
            .and_then(|v| v.parse().ok())
            .unwrap_or(1);
        let seq: usize = get_header(&frame.headers, HEADER_SEQ)
            .and_then(|v| v.parse().ok())
            .unwrap_or(0);

        // Reassemble fragmented payloads
        let payload = if sum > 1 {
            match fragment_cache.insert(&msg_id, sum, seq, frame.payload.clone()) {
                Some(combined) => combined,
                None => return, // Incomplete, wait for more parts
            }
        } else {
            frame.payload.clone()
        };

        let start = std::time::Instant::now();
        let mut response_code = 200i32;

        if msg_type == MSG_TYPE_EVENT {
            match std::str::from_utf8(&payload) {
                Ok(text) => {
                    tracing::debug!(
                        message_id = %msg_id,
                        "Received Feishu event"
                    );
                    match serde_json::from_str::<FeishuEvent>(text) {
                        Ok(event) => {
                            if event_tx.send(event).await.is_err() {
                                tracing::warn!("Event channel closed");
                            }
                        }
                        Err(e) => {
                            tracing::warn!(error = %e, "Failed to parse Feishu event JSON");
                            response_code = 500;
                        }
                    }
                }
                Err(e) => {
                    tracing::warn!(error = %e, "Invalid UTF-8 in event payload");
                    response_code = 500;
                }
            }
        }

        // Send response back (required by protocol)
        let elapsed_ms = start.elapsed().as_millis();
        frame.headers.push(Header {
            key: HEADER_BIZ_RT.to_string(),
            value: elapsed_ms.to_string(),
        });

        let resp_json = serde_json::json!({ "code": response_code });
        frame.payload = resp_json.to_string().into_bytes();

        let resp_bytes = frame.encode_to_vec();
        let mut w = write.lock().await;
        if let Err(e) = w.send(Message::Binary(resp_bytes.into())).await {
            tracing::warn!(error = %e, "Failed to send event response");
        }
    }

    pub async fn is_connected(&self) -> bool {
        *self.connected.read().await
    }

    /// Get a clone of the internal connected flag for sharing with external
    /// status monitors (e.g. FeishuHandle).
    pub fn connected_flag(&self) -> Arc<RwLock<bool>> {
        self.connected.clone()
    }
}
