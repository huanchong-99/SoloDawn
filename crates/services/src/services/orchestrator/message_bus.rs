//! Message bus for orchestrator events.

use std::{collections::HashMap, sync::Arc};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use tokio::sync::{RwLock, broadcast, mpsc};

use super::{
    constants::WORKFLOW_TOPIC_PREFIX,
    resilient_llm::ProviderEvent,
    types::{
        OrchestratorInstruction, PromptDecision, QualityGateResultEvent,
        TerminalCompletionEvent, TerminalPromptEvent,
    },
};

const TERMINAL_INPUT_TOPIC_PREFIX: &str = "terminal.input.";

/// Messages routed through the orchestrator bus.
#[allow(clippy::large_enum_variant)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum BusMessage {
    TerminalCompleted(TerminalCompletionEvent),
    GitEvent {
        workflow_id: String,
        commit_hash: String,
        branch: String,
        message: String,
    },
    Instruction(OrchestratorInstruction),
    StatusUpdate {
        workflow_id: String,
        status: String,
    },
    /// Terminal status update
    TerminalStatusUpdate {
        workflow_id: String,
        terminal_id: String,
        status: String,
    },
    /// Task status update
    TaskStatusUpdate {
        workflow_id: String,
        task_id: String,
        status: String,
    },
    Error {
        workflow_id: String,
        error: String,
    },
    TerminalMessage {
        message: String,
    },
    /// Terminal prompt detected - sent by PromptWatcher when a prompt is detected
    TerminalPromptDetected(TerminalPromptEvent),
    /// Terminal input - sent to PTY stdin via TerminalBridge
    TerminalInput {
        terminal_id: String,
        session_id: String,
        input: String,
        /// Decision that led to this input (for logging/debugging)
        decision: Option<PromptDecision>,
    },
    /// Terminal prompt decision made - for UI updates
    TerminalPromptDecision {
        terminal_id: String,
        workflow_id: String,
        decision: PromptDecision,
    },
    /// Provider state changed (switched, exhausted, recovered)
    ProviderStateChanged {
        workflow_id: String,
        event: ProviderEvent,
    },
    /// Quality gate result for a terminal checkpoint
    TerminalQualityGateResult(QualityGateResultEvent),
    Shutdown,
}

// ---------------------------------------------------------------------------
// Trait
// ---------------------------------------------------------------------------

/// Backend-agnostic message bus operations.
#[async_trait]
pub trait MessageBusBackend: Send + Sync + 'static {
    /// Publish a message to a specific topic.
    async fn publish_to_topic(&self, topic: &str, message: BusMessage) -> anyhow::Result<()>;

    /// Subscribe to a topic-specific mpsc stream.
    async fn subscribe_topic(&self, topic: &str) -> anyhow::Result<mpsc::Receiver<BusMessage>>;

    /// Broadcast a message to all broadcast subscribers.
    async fn broadcast(&self, message: BusMessage) -> anyhow::Result<()>;

    /// Subscribe to the broadcast channel.
    async fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage>;

    /// Unsubscribe / remove all subscribers from a topic.
    async fn unsubscribe_topic(&self, topic: &str);
}

// ---------------------------------------------------------------------------
// InMemoryMessageBus
// ---------------------------------------------------------------------------

/// In-memory pub/sub bus for workflow and terminal events.
#[derive(Clone)]
pub struct InMemoryMessageBus {
    broadcast_tx: broadcast::Sender<BusMessage>,
    subscribers: Arc<RwLock<HashMap<String, Vec<mpsc::Sender<BusMessage>>>>>,
}

impl InMemoryMessageBus {
    pub fn new(capacity: usize) -> Self {
        let (broadcast_tx, _) = broadcast::channel(capacity);
        Self {
            broadcast_tx,
            subscribers: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Returns current subscriber count for a topic.
    pub async fn subscriber_count(&self, topic: &str) -> usize {
        let subscribers = self.subscribers.read().await;
        subscribers.get(topic).map_or(0, Vec::len)
    }

    /// Internal publish with optional subscriber requirement.
    async fn publish_inner(
        &self,
        topic: &str,
        message: BusMessage,
        require_subscribers: bool,
    ) -> anyhow::Result<usize> {
        let subscribers = {
            let subscribers = self.subscribers.read().await;
            subscribers.get(topic).cloned().unwrap_or_default()
        };

        if subscribers.is_empty() {
            if require_subscribers {
                return Err(anyhow::anyhow!("No subscribers for topic: {topic}"));
            }
            tracing::warn!(topic = %topic, "Dropping message: no subscribers");
            return Ok(0);
        }

        let mut delivered = 0usize;
        let mut had_closed_subscribers = false;

        for tx in &subscribers {
            match tx.send(message.clone()).await {
                Ok(()) => {
                    delivered += 1;
                }
                Err(err) => {
                    had_closed_subscribers = true;
                    tracing::debug!(
                        topic = %topic,
                        ?err,
                        "Dropping closed topic subscriber during publish"
                    );
                }
            }
        }

        if had_closed_subscribers {
            let mut subscribers = self.subscribers.write().await;
            if let Some(topic_subscribers) = subscribers.get_mut(topic) {
                topic_subscribers.retain(|sender| !sender.is_closed());
                if topic_subscribers.is_empty() {
                    subscribers.remove(topic);
                }
            }
        }

        if delivered == 0 {
            if require_subscribers {
                return Err(anyhow::anyhow!("No active subscribers for topic: {topic}"));
            }
            tracing::warn!(topic = %topic, "Dropping message: no active subscribers");
            return Ok(0);
        }

        tracing::trace!(
            topic = %topic,
            subscriber_count = delivered,
            "Published message to topic subscribers"
        );
        Ok(delivered)
    }
}

#[async_trait]
impl MessageBusBackend for InMemoryMessageBus {
    async fn publish_to_topic(&self, topic: &str, message: BusMessage) -> anyhow::Result<()> {
        self.publish_inner(topic, message, false).await.map(|_| ())
    }

    async fn subscribe_topic(&self, topic: &str) -> anyhow::Result<mpsc::Receiver<BusMessage>> {
        let (tx, rx) = mpsc::channel(100);
        let mut subscribers = self.subscribers.write().await;
        subscribers.entry(topic.to_string()).or_default().push(tx);
        Ok(rx)
    }

    async fn broadcast(&self, message: BusMessage) -> anyhow::Result<()> {
        match self.broadcast_tx.send(message) {
            Ok(_) => Ok(()),
            Err(_) => {
                tracing::debug!("Broadcast skipped: no active broadcast subscribers");
                Ok(())
            }
        }
    }

    async fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage> {
        self.broadcast_tx.subscribe()
    }

    async fn unsubscribe_topic(&self, topic: &str) {
        let mut subscribers = self.subscribers.write().await;
        subscribers.remove(topic);
    }
}

// ---------------------------------------------------------------------------
// RedisBus
// ---------------------------------------------------------------------------

/// Redis PubSub-backed message bus for multi-container deployments.
#[derive(Clone)]
pub struct RedisBus {
    client: redis::Client,
    /// Local broadcast channel used to fan-out received Redis messages to
    /// in-process subscribers of the broadcast stream.
    broadcast_tx: broadcast::Sender<BusMessage>,
    /// Capacity used when creating mpsc channels for topic subscriptions.
    capacity: usize,
}

impl RedisBus {
    /// Create a new Redis-backed message bus.
    ///
    /// `redis_url` should be a valid Redis connection string, e.g. `redis://127.0.0.1:6379`.
    pub async fn new(redis_url: &str, capacity: usize) -> anyhow::Result<Self> {
        let client = redis::Client::open(redis_url)?;
        // Verify connectivity
        let mut conn = client.get_multiplexed_async_connection().await?;
        redis::cmd("PING")
            .query_async::<String>(&mut conn)
            .await?;
        tracing::info!(url = %redis_url, "RedisBus connected successfully");

        let (broadcast_tx, _) = broadcast::channel(capacity);
        Ok(Self {
            client,
            broadcast_tx,
            capacity,
        })
    }

    fn topic_channel(topic: &str) -> String {
        format!("gitcortex:topic:{topic}")
    }

    const BROADCAST_CHANNEL: &'static str = "gitcortex:broadcast";
}

#[async_trait]
impl MessageBusBackend for RedisBus {
    async fn publish_to_topic(&self, topic: &str, message: BusMessage) -> anyhow::Result<()> {
        let channel = Self::topic_channel(topic);
        let payload = serde_json::to_string(&message)?;
        let mut conn = self.client.get_multiplexed_async_connection().await?;
        redis::cmd("PUBLISH")
            .arg(&channel)
            .arg(&payload)
            .query_async::<i64>(&mut conn)
            .await?;
        tracing::trace!(topic = %topic, channel = %channel, "Published message to Redis topic");
        Ok(())
    }

    async fn subscribe_topic(&self, topic: &str) -> anyhow::Result<mpsc::Receiver<BusMessage>> {
        let (tx, rx) = mpsc::channel(self.capacity);
        let channel = Self::topic_channel(topic);
        let client = self.client.clone();

        tokio::spawn(async move {
            let conn = match client.get_async_pubsub().await {
                Ok(c) => c,
                Err(err) => {
                    tracing::warn!(?err, channel = %channel, "Failed to open Redis PubSub connection for topic");
                    return;
                }
            };
            let mut pubsub = conn;
            if let Err(err) = pubsub.subscribe(&channel).await {
                tracing::warn!(?err, channel = %channel, "Failed to subscribe to Redis channel");
                return;
            }

            let mut msg_stream = pubsub.into_on_message();
            use futures_util::StreamExt;
            while let Some(msg) = msg_stream.next().await {
                let payload: String = match msg.get_payload() {
                    Ok(p) => p,
                    Err(err) => {
                        tracing::warn!(?err, channel = %channel, "Failed to get Redis message payload");
                        continue;
                    }
                };
                let bus_msg: BusMessage = match serde_json::from_str(&payload) {
                    Ok(m) => m,
                    Err(err) => {
                        tracing::warn!(?err, channel = %channel, "Failed to deserialize Redis message");
                        continue;
                    }
                };
                if tx.send(bus_msg).await.is_err() {
                    tracing::debug!(channel = %channel, "Topic subscriber dropped, stopping Redis listener");
                    break;
                }
            }
        });

        Ok(rx)
    }

    async fn broadcast(&self, message: BusMessage) -> anyhow::Result<()> {
        let payload = serde_json::to_string(&message)?;
        let mut conn = self.client.get_multiplexed_async_connection().await?;
        redis::cmd("PUBLISH")
            .arg(Self::BROADCAST_CHANNEL)
            .arg(&payload)
            .query_async::<i64>(&mut conn)
            .await?;

        // Also fan-out locally so in-process broadcast subscribers receive it
        let _ = self.broadcast_tx.send(message);
        Ok(())
    }

    async fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage> {
        let local_rx = self.broadcast_tx.subscribe();

        // Spawn a background task that listens on the Redis broadcast channel and
        // re-publishes into the local broadcast sender so that all in-process
        // subscribers receive messages published by *other* containers.
        let client = self.client.clone();
        let tx = self.broadcast_tx.clone();
        tokio::spawn(async move {
            let conn = match client.get_async_pubsub().await {
                Ok(c) => c,
                Err(err) => {
                    tracing::warn!(?err, "Failed to open Redis PubSub for broadcast");
                    return;
                }
            };
            let mut pubsub = conn;
            if let Err(err) = pubsub.subscribe(RedisBus::BROADCAST_CHANNEL).await {
                tracing::warn!(?err, "Failed to subscribe to Redis broadcast channel");
                return;
            }

            let mut msg_stream = pubsub.into_on_message();
            use futures_util::StreamExt;
            while let Some(msg) = msg_stream.next().await {
                let payload: String = match msg.get_payload() {
                    Ok(p) => p,
                    Err(err) => {
                        tracing::warn!(?err, "Failed to get Redis broadcast payload");
                        continue;
                    }
                };
                let bus_msg: BusMessage = match serde_json::from_str(&payload) {
                    Ok(m) => m,
                    Err(err) => {
                        tracing::warn!(?err, "Failed to deserialize Redis broadcast message");
                        continue;
                    }
                };
                if tx.send(bus_msg).is_err() {
                    tracing::debug!("No broadcast subscribers, stopping Redis broadcast listener");
                    break;
                }
            }
        });

        local_rx
    }

    async fn unsubscribe_topic(&self, topic: &str) {
        // For Redis, unsubscribing happens implicitly when the subscriber Receiver
        // is dropped (the spawned task detects the closed channel and exits).
        // We log for observability.
        tracing::debug!(topic = %topic, "RedisBus: unsubscribe_topic called (subscriber cleanup is implicit)");
    }
}

// ---------------------------------------------------------------------------
// Unified MessageBus wrapper
// ---------------------------------------------------------------------------

/// Unified message bus that delegates to either an in-memory or Redis backend.
#[derive(Clone)]
pub enum MessageBus {
    InMemory(InMemoryMessageBus),
    Redis(RedisBus),
}

impl MessageBus {
    /// Create a new in-memory message bus.
    pub fn new_in_memory(capacity: usize) -> Self {
        Self::InMemory(InMemoryMessageBus::new(capacity))
    }

    /// Create a new Redis-backed message bus.
    pub async fn new_redis(url: &str, capacity: usize) -> anyhow::Result<Self> {
        let redis_bus = RedisBus::new(url, capacity).await?;
        Ok(Self::Redis(redis_bus))
    }

    /// Create a message bus from environment variables.
    ///
    /// Reads `GITCORTEX_MESSAGE_BUS` (values: `"redis"` or `"memory"`, default `"memory"`)
    /// and `GITCORTEX_REDIS_URL` (required when bus is `"redis"`).
    pub fn from_env(capacity: usize) -> anyhow::Result<Self> {
        let bus_type = std::env::var("GITCORTEX_MESSAGE_BUS").unwrap_or_else(|_| "memory".into());
        match bus_type.as_str() {
            "redis" => {
                let url = std::env::var("GITCORTEX_REDIS_URL").map_err(|_| {
                    anyhow::anyhow!(
                        "GITCORTEX_REDIS_URL must be set when GITCORTEX_MESSAGE_BUS=redis"
                    )
                })?;
                // We need a runtime to create the Redis connection; use block_on
                // if called outside of an async context, otherwise use spawn.
                let rt = tokio::runtime::Handle::try_current().map_err(|_| {
                    anyhow::anyhow!("from_env must be called within a Tokio runtime")
                })?;
                let bus = rt.block_on(Self::new_redis(&url, capacity))?;
                Ok(bus)
            }
            "memory" | "" => Ok(Self::new_in_memory(capacity)),
            other => Err(anyhow::anyhow!(
                "Unknown GITCORTEX_MESSAGE_BUS value: {other}. Expected 'redis' or 'memory'."
            )),
        }
    }

    /// Backwards-compatible constructor (creates in-memory bus).
    pub fn new(capacity: usize) -> Self {
        Self::new_in_memory(capacity)
    }

    /// Returns the in-memory backend, if this bus uses one.
    fn as_in_memory(&self) -> Option<&InMemoryMessageBus> {
        match self {
            Self::InMemory(inner) => Some(inner),
            Self::Redis(_) => None,
        }
    }

    /// Returns the broadcast sender for the underlying backend.
    fn broadcast_tx(&self) -> &broadcast::Sender<BusMessage> {
        match self {
            Self::InMemory(inner) => &inner.broadcast_tx,
            Self::Redis(inner) => &inner.broadcast_tx,
        }
    }

    // -----------------------------------------------------------------------
    // Synchronous broadcast API (backwards-compatible with original MessageBus)
    // -----------------------------------------------------------------------

    /// Broadcast a message synchronously (original API).
    ///
    /// For the in-memory backend this is a direct channel send.
    /// For the Redis backend this only fans out locally; use the async
    /// [`MessageBusBackend::broadcast`] trait method to also publish to Redis.
    #[allow(clippy::result_large_err)]
    pub fn broadcast(
        &self,
        message: BusMessage,
    ) -> Result<usize, broadcast::error::SendError<BusMessage>> {
        self.broadcast_tx().send(message)
    }

    /// Subscribe to the broadcast channel (synchronous, original API).
    pub fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage> {
        self.broadcast_tx().subscribe()
    }

    // -----------------------------------------------------------------------
    // High-level convenience methods (preserved from original API)
    // -----------------------------------------------------------------------

    /// Subscribe to a topic-specific mpsc stream.
    pub async fn subscribe(&self, topic: &str) -> mpsc::Receiver<BusMessage> {
        self.subscribe_topic(topic)
            .await
            .expect("subscribe_topic should not fail for in-memory bus")
    }

    /// Returns current subscriber count for a topic.
    ///
    /// Only meaningful for in-memory backend; returns 0 for Redis.
    pub async fn subscriber_count(&self, topic: &str) -> usize {
        match self.as_in_memory() {
            Some(inner) => inner.subscriber_count(topic).await,
            None => 0,
        }
    }

    /// Publish a message and require at least one subscriber.
    pub async fn publish_required(
        &self,
        topic: &str,
        message: BusMessage,
    ) -> anyhow::Result<usize> {
        match self {
            Self::InMemory(inner) => inner.publish_inner(topic, message, true).await,
            Self::Redis(_) => {
                self.publish_to_topic(topic, message).await?;
                // Redis PubSub doesn't return subscriber count to the publisher in a
                // useful way, so we optimistically return 1.
                Ok(1)
            }
        }
    }

    /// Publish a message to all subscribers of a topic.
    pub async fn publish(&self, topic: &str, message: BusMessage) -> anyhow::Result<()> {
        self.publish_to_topic(topic, message).await
    }

    /// Publish a workflow-scoped event to both workflow topic and broadcast channel.
    pub async fn publish_workflow_event(
        &self,
        workflow_id: &str,
        message: BusMessage,
    ) -> anyhow::Result<usize> {
        let topic = format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}");

        match self {
            Self::InMemory(inner) => {
                let delivered = inner.publish_inner(&topic, message.clone(), false).await?;
                if let Err(err) = self.broadcast(message) {
                    tracing::debug!(
                        ?err,
                        workflow_id = %workflow_id,
                        "Workflow broadcast skipped because no broadcast subscribers are active"
                    );
                }
                Ok(delivered)
            }
            Self::Redis(_) => {
                self.publish_to_topic(&topic, message.clone()).await?;
                if let Err(err) = self.broadcast(message) {
                    tracing::debug!(
                        ?err,
                        workflow_id = %workflow_id,
                        "Workflow broadcast skipped"
                    );
                }
                Ok(1)
            }
        }
    }

    /// Publishes a terminal completion event to workflow topic and broadcast channel.
    pub async fn publish_terminal_completed(&self, event: TerminalCompletionEvent) {
        let workflow_id = event.workflow_id.clone();
        if let Err(e) = self
            .publish_workflow_event(&workflow_id, BusMessage::TerminalCompleted(event))
            .await
        {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to publish terminal completion event (non-fatal)"
            );
        }
    }

    /// Publishes a git event to workflow topic and broadcast channel.
    pub async fn publish_git_event(
        &self,
        workflow_id: &str,
        commit_hash: &str,
        branch: &str,
        message: &str,
    ) {
        let event = BusMessage::GitEvent {
            workflow_id: workflow_id.to_string(),
            commit_hash: commit_hash.to_string(),
            branch: branch.to_string(),
            message: message.to_string(),
        };
        if let Err(e) = self.publish_workflow_event(workflow_id, event).await {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to publish git event (non-fatal)"
            );
        }
    }

    /// Publishes a terminal prompt detected event.
    pub async fn publish_terminal_prompt_detected(&self, event: TerminalPromptEvent) {
        let workflow_id = event.workflow_id.clone();
        if let Err(e) = self
            .publish_workflow_event(&workflow_id, BusMessage::TerminalPromptDetected(event))
            .await
        {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to publish terminal prompt detected event (non-fatal)"
            );
        }
    }

    /// Publishes a terminal input message to be sent to PTY stdin.
    pub async fn publish_terminal_input(
        &self,
        terminal_id: &str,
        session_id: &str,
        input: &str,
        decision: Option<PromptDecision>,
    ) -> bool {
        let message = BusMessage::TerminalInput {
            terminal_id: terminal_id.to_string(),
            session_id: session_id.to_string(),
            input: input.to_string(),
            decision,
        };

        match self {
            Self::InMemory(inner) => {
                // Publish to terminal-specific topic first (preferred path for PTY routing).
                let topic = format!("{TERMINAL_INPUT_TOPIC_PREFIX}{terminal_id}");
                let topic_subscriber_count = inner.subscriber_count(&topic).await;
                let fallback_topic = session_id.to_string();
                let delivered = if topic_subscriber_count > 0 {
                    match inner.publish_inner(&topic, message.clone(), false).await {
                        Ok(primary_delivered) if primary_delivered > 0 => true,
                        Ok(_) => {
                            self.publish_terminal_input_fallback(
                                terminal_id,
                                session_id,
                                &topic,
                                &fallback_topic,
                                &message,
                                "primary topic had no active subscribers",
                            )
                            .await
                        }
                        Err(err) => {
                            tracing::error!(
                                ?err,
                                terminal_id = %terminal_id,
                                session_id = %session_id,
                                topic = %topic,
                                "Failed to publish terminal input to primary topic"
                            );
                            self.publish_terminal_input_fallback(
                                terminal_id,
                                session_id,
                                &topic,
                                &fallback_topic,
                                &message,
                                "primary topic publish failed",
                            )
                            .await
                        }
                    }
                } else {
                    self.publish_terminal_input_fallback(
                        terminal_id,
                        session_id,
                        &topic,
                        &fallback_topic,
                        &message,
                        "no primary terminal-input subscribers",
                    )
                    .await
                };

                // Also broadcast for legacy compatibility
                if let Err(err) = self.broadcast(message) {
                    tracing::debug!(
                        ?err,
                        terminal_id = %terminal_id,
                        session_id = %session_id,
                        "Terminal-input broadcast skipped because no broadcast subscribers are active"
                    );
                }

                delivered
            }
            Self::Redis(_) => {
                // For Redis, publish to the primary topic and broadcast
                let topic = format!("{TERMINAL_INPUT_TOPIC_PREFIX}{terminal_id}");
                if let Err(err) = self.publish_to_topic(&topic, message.clone()).await {
                    tracing::warn!(
                        ?err,
                        terminal_id = %terminal_id,
                        "Failed to publish terminal input to Redis topic"
                    );
                }
                // Also publish to session fallback topic
                let fallback_topic = session_id.to_string();
                if let Err(err) = self.publish_to_topic(&fallback_topic, message.clone()).await {
                    tracing::debug!(
                        ?err,
                        terminal_id = %terminal_id,
                        "Failed to publish terminal input to Redis fallback topic"
                    );
                }
                if let Err(err) = self.broadcast(message) {
                    tracing::debug!(
                        ?err,
                        terminal_id = %terminal_id,
                        "Terminal-input broadcast skipped"
                    );
                }
                true
            }
        }
    }

    async fn publish_terminal_input_fallback(
        &self,
        terminal_id: &str,
        session_id: &str,
        primary_topic: &str,
        fallback_topic: &str,
        message: &BusMessage,
        reason: &str,
    ) -> bool {
        let fallback_subscriber_count = self.subscriber_count(fallback_topic).await;

        if fallback_subscriber_count > 0 {
            tracing::warn!(
                terminal_id = %terminal_id,
                session_id = %session_id,
                primary_topic = %primary_topic,
                fallback_topic = %fallback_topic,
                reason = %reason,
                "Falling back to legacy session topic for terminal input"
            );

            return match self.publish(fallback_topic, message.clone()).await {
                Ok(()) => true,
                Err(err) => {
                    tracing::error!(
                        ?err,
                        terminal_id = %terminal_id,
                        session_id = %session_id,
                        topic = %fallback_topic,
                        "Failed to publish terminal input to fallback topic"
                    );
                    false
                }
            };
        }
        tracing::error!(
            terminal_id = %terminal_id,
            session_id = %session_id,
            primary_topic = %primary_topic,
            fallback_topic = %fallback_topic,
            reason = %reason,
            "Dropping terminal input: no primary or fallback subscribers"
        );
        false
    }

    /// Publishes a terminal prompt decision for UI updates.
    pub async fn publish_terminal_prompt_decision(
        &self,
        terminal_id: &str,
        workflow_id: &str,
        decision: PromptDecision,
    ) {
        let message = BusMessage::TerminalPromptDecision {
            terminal_id: terminal_id.to_string(),
            workflow_id: workflow_id.to_string(),
            decision,
        };
        if let Err(e) = self.publish_workflow_event(workflow_id, message).await {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to publish prompt decision event (non-fatal)"
            );
        }
    }

    /// Publishes a quality gate result event.
    pub async fn publish_quality_gate_result(&self, event: QualityGateResultEvent) {
        let workflow_id = event.workflow_id.clone();
        if let Err(e) = self
            .publish_workflow_event(
                &workflow_id,
                BusMessage::TerminalQualityGateResult(event),
            )
            .await
        {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to publish quality gate result event (non-fatal)"
            );
        }
    }
}

#[async_trait]
impl MessageBusBackend for MessageBus {
    async fn publish_to_topic(&self, topic: &str, message: BusMessage) -> anyhow::Result<()> {
        match self {
            Self::InMemory(inner) => inner.publish_to_topic(topic, message).await,
            Self::Redis(inner) => inner.publish_to_topic(topic, message).await,
        }
    }

    async fn subscribe_topic(&self, topic: &str) -> anyhow::Result<mpsc::Receiver<BusMessage>> {
        match self {
            Self::InMemory(inner) => inner.subscribe_topic(topic).await,
            Self::Redis(inner) => inner.subscribe_topic(topic).await,
        }
    }

    async fn broadcast(&self, message: BusMessage) -> anyhow::Result<()> {
        match self {
            Self::InMemory(inner) => inner.broadcast(message).await,
            Self::Redis(inner) => inner.broadcast(message).await,
        }
    }

    async fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage> {
        match self {
            Self::InMemory(inner) => inner.subscribe_broadcast().await,
            Self::Redis(inner) => inner.subscribe_broadcast().await,
        }
    }

    async fn unsubscribe_topic(&self, topic: &str) {
        match self {
            Self::InMemory(inner) => inner.unsubscribe_topic(topic).await,
            Self::Redis(inner) => inner.unsubscribe_topic(topic).await,
        }
    }
}

impl Default for MessageBus {
    fn default() -> Self {
        Self::new(1000)
    }
}

pub type SharedMessageBus = Arc<MessageBus>;

#[cfg(test)]
mod tests {
    use tokio::time::{Duration, timeout};

    use super::{BusMessage, MessageBus};

    #[tokio::test]
    async fn publish_terminal_input_falls_back_to_legacy_topic_without_primary_subscribers() {
        let bus = MessageBus::new(8);
        let mut legacy_rx = bus.subscribe("session-1").await;
        let mut broadcast_rx = bus.subscribe_broadcast();

        let delivered = bus
            .publish_terminal_input("term-1", "session-1", "y", None)
            .await;
        assert!(delivered);

        let legacy_message = timeout(Duration::from_millis(200), legacy_rx.recv())
            .await
            .expect("expected legacy topic message")
            .expect("legacy topic channel should be open");

        assert!(matches!(
            legacy_message,
            BusMessage::TerminalInput {
                ref terminal_id,
                ref session_id,
                ref input,
                ..
            } if terminal_id == "term-1" && session_id == "session-1" && input == "y"
        ));

        let broadcast_message = timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected broadcast message")
            .expect("broadcast channel should be open");

        assert!(matches!(
            broadcast_message,
            BusMessage::TerminalInput { .. }
        ));
    }

    #[tokio::test]
    async fn publish_terminal_input_prefers_primary_topic_without_legacy_duplicate() {
        let bus = MessageBus::new(8);
        let mut primary_rx = bus.subscribe("terminal.input.term-1").await;
        let mut legacy_rx = bus.subscribe("session-1").await;
        let mut broadcast_rx = bus.subscribe_broadcast();

        let delivered = bus
            .publish_terminal_input("term-1", "session-1", "n", None)
            .await;
        assert!(delivered);

        let primary_message = timeout(Duration::from_millis(200), primary_rx.recv())
            .await
            .expect("expected primary topic message")
            .expect("primary topic channel should be open");

        assert!(matches!(
            primary_message,
            BusMessage::TerminalInput {
                ref terminal_id,
                ref session_id,
                ref input,
                ..
            } if terminal_id == "term-1" && session_id == "session-1" && input == "n"
        ));

        let legacy_message = timeout(Duration::from_millis(100), legacy_rx.recv()).await;
        assert!(
            legacy_message.is_err(),
            "legacy topic should not receive duplicate terminal input when primary subscribers exist"
        );

        let broadcast_message = timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected broadcast message")
            .expect("broadcast channel should be open");

        assert!(matches!(
            broadcast_message,
            BusMessage::TerminalInput { .. }
        ));
    }

    #[tokio::test]
    async fn publish_terminal_input_falls_back_when_primary_subscriber_is_stale() {
        let bus = MessageBus::new(8);

        let primary_rx = bus.subscribe("terminal.input.term-1").await;
        drop(primary_rx);

        let mut legacy_rx = bus.subscribe("session-1").await;

        let delivered = bus
            .publish_terminal_input("term-1", "session-1", "approve", None)
            .await;
        assert!(delivered);

        let legacy_message = timeout(Duration::from_millis(200), legacy_rx.recv())
            .await
            .expect("expected fallback message")
            .expect("legacy topic channel should be open");

        assert!(matches!(
            legacy_message,
            BusMessage::TerminalInput {
                ref terminal_id,
                ref session_id,
                ref input,
                ..
            } if terminal_id == "term-1" && session_id == "session-1" && input == "approve"
        ));
    }

    #[tokio::test]
    async fn publish_terminal_input_returns_false_when_no_route_subscribers() {
        let bus = MessageBus::new(8);
        let mut broadcast_rx = bus.subscribe_broadcast();

        let delivered = bus
            .publish_terminal_input("term-1", "session-1", "approve", None)
            .await;
        assert!(!delivered);

        let broadcast_message = timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected broadcast message")
            .expect("broadcast channel should be open");
        assert!(matches!(
            broadcast_message,
            BusMessage::TerminalInput { .. }
        ));
    }

    #[tokio::test]
    async fn publish_succeeds_when_topic_contains_stale_and_live_subscribers() {
        let bus = MessageBus::new(8);
        let stale_rx = bus.subscribe("workflow:wf-1").await;
        drop(stale_rx);
        let mut live_rx = bus.subscribe("workflow:wf-1").await;

        let publish_result = bus
            .publish(
                "workflow:wf-1",
                BusMessage::StatusUpdate {
                    workflow_id: "wf-1".to_string(),
                    status: "starting".to_string(),
                },
            )
            .await;

        assert!(
            publish_result.is_ok(),
            "publish should ignore stale subscribers"
        );

        let live_message = timeout(Duration::from_millis(200), live_rx.recv())
            .await
            .expect("expected live subscriber message")
            .expect("live subscriber channel should be open");

        assert!(matches!(
            live_message,
            BusMessage::StatusUpdate {
                ref workflow_id,
                ref status
            } if workflow_id == "wf-1" && status == "starting"
        ));
    }

    #[tokio::test]
    async fn publish_prunes_fully_stale_topic_subscribers() {
        let bus = MessageBus::new(8);
        let stale_rx = bus.subscribe("workflow:wf-2").await;
        drop(stale_rx);

        let publish_result = bus
            .publish(
                "workflow:wf-2",
                BusMessage::StatusUpdate {
                    workflow_id: "wf-2".to_string(),
                    status: "starting".to_string(),
                },
            )
            .await;

        assert!(
            publish_result.is_ok(),
            "publish should not fail when all subscribers are stale"
        );
        assert_eq!(
            bus.subscriber_count("workflow:wf-2").await,
            0,
            "stale subscribers should be removed after publish"
        );
    }
}
