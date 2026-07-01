//! openlark-backed adapter that preserves the legacy `FeishuClient` interface.
//!
//! Replaces the hand-rolled reverse-engineered pbbp2 WebSocket client. The
//! openlark SDK (`LarkWsClient`) provides a correct long-connection transport
//! (proper `SeqID`/`LogID`/`method` framing via `lark-websocket-protobuf`),
//! eliminating the proto3-drops-required-fields root cause.
//!
//! The public surface (`new`/`connect`/`connected_flag`/`sdk_config`) matches
//! what `services::services::feishu::FeishuService` expects, so the outer
//! `ReconnectPolicy` retry loop in `server::main` is unchanged.

// `LarkWsClient::open` requires `openlark_client::config::Config` (re-exported as
// `open_lark::Config`), which the SDK marks `#[deprecated]` in favour of CoreConfig.
// It is mandatory for the WS transport, so suppress the deprecation lint here.
#![allow(deprecated)]

use std::sync::Arc;
use std::time::Duration;

use anyhow::Result;
use tokio::sync::{RwLock, mpsc};

use open_lark::ws_client::{EventDispatcherHandler, EventHandler, LarkWsClient};
use open_lark::{Config, CoreConfig};

use crate::{
    events::{self, EVENT_TYPE_MESSAGE, FeishuEvent},
    types::FeishuConfig,
};

/// If no `im.message.receive_v1` event arrives within this many seconds after a
/// successful connect, log a diagnostic warning (the connection is up but the
/// Feishu developer-console side is likely misconfigured).
const FEISHU_NO_EVENT_WATCHDOG_SECS: u64 = 120;

/// `EventHandler` impl that forwards raw `im.message.receive_v1` payload bytes
/// into an unbounded channel drained by the adapter's pump task.
struct RawForwarder {
    tx: mpsc::UnboundedSender<Vec<u8>>,
}

impl EventHandler for RawForwarder {
    fn handle(&self, payload: &[u8]) -> std::result::Result<(), Box<dyn std::error::Error + Send + Sync>> {
        // Non-blocking send from the SDK's sync dispatch context.
        let _ = self.tx.send(payload.to_vec());
        Ok(())
    }
}

/// openlark-backed drop-in replacement for the legacy `FeishuClient`.
pub struct FeishuClient {
    cfg: Config,
    event_tx: mpsc::Sender<FeishuEvent>,
    connected: Arc<RwLock<bool>>,
}

impl FeishuClient {
    /// Build a client from a [`FeishuConfig`]. Returns the client plus the
    /// receiver end of the parsed-event channel (signature matches the legacy
    /// client so `FeishuService::new` is unchanged).
    pub fn new(config: FeishuConfig) -> (Self, mpsc::Receiver<FeishuEvent>) {
        let (tx, rx) = mpsc::channel::<FeishuEvent>(100);
        let cfg = Config::builder()
            .app_id(config.app_id)
            .app_secret(config.app_secret)
            .base_url(config.base_url)
            .enable_token_cache(true)
            .timeout(Duration::from_secs(30))
            .build_unvalidated();
        (
            Self {
                cfg,
                event_tx: tx,
                connected: Arc::new(RwLock::new(false)),
            },
            rx,
        )
    }

    /// Core config (derived) for building a [`crate::messages::FeishuMessenger`].
    /// The IM REST requests require `CoreConfig`, while the WS client uses the
    /// (deprecated) client `Config`; this bridges the two from one source.
    pub fn sdk_config(&self) -> CoreConfig {
        self.cfg.get_or_build_core_config()
    }

    /// Shared connected flag. Set `true` once the first live
    /// `im.message.receive_v1` event is received (proving the socket is up),
    /// and reset to `false` when the pump task or the connection ends. A
    /// healthy-but-idle connection therefore reads `false` until traffic flows.
    pub fn connected_flag(&self) -> Arc<RwLock<bool>> {
        self.connected.clone()
    }

    /// Open the long-connection and block until it closes, then return.
    ///
    /// Semantics match the legacy `connect()`: it runs until disconnect and
    /// returns `Err` on failure so the outer `ReconnectPolicy` loop re-fires.
    pub async fn connect(&self) -> Result<()> {
        let (raw_tx, mut raw_rx) = mpsc::unbounded_channel::<Vec<u8>>();
        let handler = EventDispatcherHandler::builder()
            .register_raw(EVENT_TYPE_MESSAGE, RawForwarder { tx: raw_tx })
            .map_err(|e| anyhow::anyhow!("register_raw failed: {e}"))?
            .build();

        // Pump: raw SDK bytes -> normalized FeishuEvent -> existing mpsc channel.
        let event_tx = self.event_tx.clone();
        let connected = self.connected.clone();
        let pump = tokio::spawn(async move {
            let mut got_first = false;
            let mut watchdog_fired = false;
            let watchdog = tokio::time::sleep(Duration::from_secs(FEISHU_NO_EVENT_WATCHDOG_SECS));
            tokio::pin!(watchdog);
            loop {
                tokio::select! {
                    maybe = raw_rx.recv() => {
                        match maybe {
                            Some(bytes) => {
                                if !got_first {
                                    got_first = true;
                                    // First real event proves the socket is live;
                                    // flip the shared flag to a true-positive here.
                                    *connected.write().await = true;
                                    // Shape-calibration log of the first real payload.
                                    tracing::info!(
                                        payload = %String::from_utf8_lossy(&bytes),
                                        "FIRST Feishu raw payload (shape calibration)"
                                    );
                                }
                                match events::feishu_event_from_sdk_payload(&bytes) {
                                    Ok(ev) => {
                                        if event_tx.send(ev).await.is_err() {
                                            break; // downstream gone
                                        }
                                    }
                                    Err(e) => {
                                        tracing::warn!(error = %e, "Feishu raw payload parse failed");
                                    }
                                }
                            }
                            None => break, // handler dropped -> WS ended
                        }
                    }
                    () = &mut watchdog, if !got_first && !watchdog_fired => {
                        watchdog_fired = true;
                        tracing::warn!(
                            seconds = FEISHU_NO_EVENT_WATCHDOG_SECS,
                            "Feishu connected but no im.message.receive_v1 event received. Check: \
                             1) app published 2) long-connection mode enabled while client is live \
                             3) im.message.receive_v1 subscribed 4) bot scopes granted 5) bot added to the chat"
                        );
                    }
                }
            }
            // Pump exiting means no more events flow on this connection.
            *connected.write().await = false;
        });

        // The socket is about to be opened by the (blocking-until-close) call
        // below. Treat 'attempting/holding the connection' as live so a healthy
        // but idle bot (no inbound traffic yet) is not misreported as
        // disconnected. open() blocks until the connection closes/errors, at
        // which point we flip this back to false right after it returns.
        *self.connected.write().await = true;
        tracing::info!("Feishu WS connecting (provider=openlark, event=im.message.receive_v1)");
        let res = LarkWsClient::open(Arc::new(self.cfg.clone()), handler).await;
        *self.connected.write().await = false;
        pump.abort();
        res.map_err(|e| anyhow::anyhow!("LarkWsClient::open ended: {e}"))
    }
}
