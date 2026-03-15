//! Feishu (Lark) integration service.
//!
//! Connects to Feishu via WebSocket, processes incoming events (messages,
//! slash commands), and forwards chat messages to the orchestrator.

use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use sqlx::SqlitePool;
use tokio::sync::mpsc;
use tracing;

use db::models::{ExternalConversationBinding, feishu_config::FeishuAppConfig};
use feishu_connector::{
    client::FeishuClient,
    events::{self, FeishuEvent, ReceivedMessage},
    messages::FeishuMessenger,
    types::FeishuConfig,
};

use super::chat_connector::ChatConnector;
use super::orchestrator::message_bus::SharedMessageBus;

const FEISHU_PROVIDER: &str = "feishu";
const EVENT_TYPE_MESSAGE: &str = "im.message.receive_v1";

// ---------------------------------------------------------------------------
// FeishuService
// ---------------------------------------------------------------------------

/// Long-running service that maintains a Feishu WebSocket connection and
/// routes incoming events to the orchestrator.
pub struct FeishuService {
    client: FeishuClient,
    event_rx: mpsc::Receiver<FeishuEvent>,
    messenger: Arc<FeishuMessenger>,
    pool: SqlitePool,
    bus: SharedMessageBus,
}

impl FeishuService {
    /// Build a new service from a [`FeishuConfig`].
    pub fn new(config: FeishuConfig, pool: SqlitePool, bus: SharedMessageBus) -> Self {
        let (client, event_rx) = FeishuClient::new(config.clone());
        let messenger = Arc::new(FeishuMessenger::new(
            client.auth().clone(),
            config.base_url.clone(),
        ));
        Self {
            client,
            event_rx,
            messenger,
            pool,
            bus,
        }
    }

    /// Try to create a service from the enabled [`FeishuAppConfig`] row in the
    /// database. Returns `None` when no enabled config exists.
    ///
    /// `decrypt_secret` is a caller-provided closure that decrypts the
    /// stored `app_secret_encrypted` value.
    pub async fn from_db<F>(
        pool: SqlitePool,
        bus: SharedMessageBus,
        decrypt_secret: F,
    ) -> Result<Option<Self>>
    where
        F: FnOnce(&str) -> Result<String>,
    {
        let Some(cfg) = FeishuAppConfig::find_enabled(&pool).await? else {
            return Ok(None);
        };
        let app_secret = decrypt_secret(&cfg.app_secret_encrypted)?;
        let feishu_config = FeishuConfig {
            app_id: cfg.app_id,
            app_secret,
            base_url: cfg.base_url,
        };
        Ok(Some(Self::new(feishu_config, pool, bus)))
    }

    /// Start the WebSocket connection and event processing loop.
    ///
    /// This method runs until the connection is closed or an unrecoverable
    /// error occurs. Callers should wrap it in a retry/reconnect loop.
    pub async fn start(&mut self) -> Result<()> {
        let connect_fut = self.client.connect();

        // Take a mutable reference to event_rx separately so we don't hold
        // an immutable borrow on `self` across the select! branches.
        let event_rx = &mut self.event_rx;
        let pool = &self.pool;
        let bus = &self.bus;
        let messenger = &self.messenger;

        tokio::select! {
            conn_result = connect_fut => {
                if let Err(e) = conn_result {
                    tracing::error!(error = %e, "Feishu WebSocket connection ended with error");
                }
            }
            () = Self::process_events_inner(event_rx, pool, bus, messenger) => {
                tracing::info!("Feishu event processing loop ended");
            }
        }

        Ok(())
    }

    /// Internal event processing loop.
    async fn process_events_inner(
        event_rx: &mut mpsc::Receiver<FeishuEvent>,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
    ) {
        while let Some(event) = event_rx.recv().await {
            if let Err(e) = Self::handle_event_inner(&event, pool, bus, messenger).await {
                tracing::warn!(error = %e, "Failed to handle Feishu event");
            }
        }
    }

    /// Route an incoming event by its `event_type`.
    async fn handle_event_inner(
        event: &FeishuEvent,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        let Some(header) = &event.header else {
            tracing::debug!("Ignoring Feishu event without header");
            return Ok(());
        };

        match header.event_type.as_str() {
            EVENT_TYPE_MESSAGE => Self::handle_message_inner(event, pool, bus, messenger).await,
            other => {
                tracing::debug!(event_type = %other, "Ignoring unhandled Feishu event type");
                Ok(())
            }
        }
    }

    /// Handle an incoming chat message.
    ///
    /// Parses the message, checks for slash commands (`/bind`, `/unbind`),
    /// and otherwise forwards the text to the bound workflow's orchestrator.
    async fn handle_message_inner(
        event: &FeishuEvent,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        let msg = events::parse_message_event(event)?;

        // Only handle text messages.
        if msg.message_type != "text" {
            tracing::debug!(
                message_type = %msg.message_type,
                "Ignoring non-text Feishu message"
            );
            return Ok(());
        }

        let text = events::parse_text_content(&msg.content);
        let text = text.trim();

        if text.is_empty() {
            return Ok(());
        }

        // Slash commands
        if let Some(workflow_id) = text.strip_prefix("/bind ").map(str::trim) {
            return Self::handle_bind_inner(&msg, workflow_id, pool, messenger).await;
        }
        if text.eq_ignore_ascii_case("/unbind") {
            return Self::handle_unbind_inner(&msg, pool, messenger).await;
        }

        // Regular message -> forward to orchestrator via binding
        Self::forward_to_orchestrator_inner(&msg, text, pool, bus, messenger).await
    }

    /// `/bind <workflow_id>` -- create or update a conversation binding.
    async fn handle_bind_inner(
        msg: &ReceivedMessage,
        workflow_id: &str,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        if workflow_id.is_empty() {
            messenger
                .reply_text(&msg.message_id, "Usage: /bind <workflow_id>")
                .await?;
            return Ok(());
        }

        ExternalConversationBinding::upsert(
            pool,
            FEISHU_PROVIDER,
            &msg.chat_id,
            workflow_id,
            Some(&msg.sender_open_id),
        )
        .await?;

        let reply = format!("Bound to workflow {workflow_id}");
        messenger.reply_text(&msg.message_id, &reply).await?;
        tracing::info!(
            chat_id = %msg.chat_id,
            workflow_id = %workflow_id,
            "Feishu conversation bound"
        );
        Ok(())
    }

    /// `/unbind` -- deactivate the current conversation binding.
    async fn handle_unbind_inner(
        msg: &ReceivedMessage,
        pool: &SqlitePool,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        let affected =
            ExternalConversationBinding::deactivate(pool, FEISHU_PROVIDER, &msg.chat_id).await?;

        let reply = if affected > 0 {
            "Conversation unbound".to_string()
        } else {
            "No active binding to remove".to_string()
        };
        messenger.reply_text(&msg.message_id, &reply).await?;
        tracing::info!(chat_id = %msg.chat_id, affected, "Feishu conversation unbound");
        Ok(())
    }

    /// Forward a regular chat message to the orchestrator for the bound workflow.
    async fn forward_to_orchestrator_inner(
        msg: &ReceivedMessage,
        text: &str,
        pool: &SqlitePool,
        bus: &SharedMessageBus,
        messenger: &Arc<FeishuMessenger>,
    ) -> Result<()> {
        let binding =
            ExternalConversationBinding::find_active(pool, FEISHU_PROVIDER, &msg.chat_id).await?;

        let Some(binding) = binding else {
            messenger
                .reply_text(
                    &msg.message_id,
                    "This conversation is not bound. Use /bind <workflow_id> first.",
                )
                .await?;
            return Ok(());
        };

        // Publish a chat instruction to the workflow's message bus topic.
        use super::orchestrator::message_bus::BusMessage;
        let instruction_msg = BusMessage::TerminalMessage {
            message: format!(
                "[feishu:{}:{}] {}",
                msg.chat_id, msg.sender_open_id, text
            ),
        };
        let topic = format!("workflow:{}", binding.workflow_id);
        bus.publish(&topic, instruction_msg).await?;

        tracing::info!(
            chat_id = %msg.chat_id,
            workflow_id = %binding.workflow_id,
            "Forwarded Feishu message to orchestrator"
        );
        Ok(())
    }

    /// Get a reference to the messenger (useful for building a [`FeishuConnector`]).
    pub fn messenger(&self) -> &Arc<FeishuMessenger> {
        &self.messenger
    }
}

// ---------------------------------------------------------------------------
// FeishuConnector — ChatConnector implementation
// ---------------------------------------------------------------------------

/// Wraps [`FeishuMessenger`] to implement the [`ChatConnector`] trait.
pub struct FeishuConnector {
    messenger: Arc<FeishuMessenger>,
    connected: Arc<tokio::sync::RwLock<bool>>,
}

impl FeishuConnector {
    /// Create a new connector from a shared messenger.
    pub fn new(messenger: Arc<FeishuMessenger>) -> Self {
        Self {
            messenger,
            connected: Arc::new(tokio::sync::RwLock::new(false)),
        }
    }

    /// Mark the connector as connected (called after WebSocket is established).
    pub async fn set_connected(&self, value: bool) {
        *self.connected.write().await = value;
    }
}

#[async_trait]
impl ChatConnector for FeishuConnector {
    async fn send_message(&self, conversation_id: &str, content: &str) -> anyhow::Result<String> {
        self.messenger.send_text(conversation_id, content).await
    }

    async fn send_reply(
        &self,
        _conversation_id: &str,
        message_id: &str,
        content: &str,
    ) -> anyhow::Result<String> {
        self.messenger.reply_text(message_id, content).await
    }

    fn provider_name(&self) -> &str {
        FEISHU_PROVIDER
    }

    fn is_connected(&self) -> bool {
        // G32-017: `try_read` is intentional here — `is_connected` is called from
        // synchronous trait methods (ChatConnector) and must not block. If the
        // RwLock is write-locked (i.e. connection state is being updated), we
        // conservatively return `false` rather than waiting. An AtomicBool would
        // be simpler but the RwLock is shared with `set_connected` which is async
        // and already uses the write lock; switching would require changing the
        // FeishuConnector struct layout for minimal benefit.
        self.connected
            .try_read()
            .map(|guard| *guard)
            .unwrap_or(false)
    }
}
