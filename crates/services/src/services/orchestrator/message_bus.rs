//! Message bus for orchestrator events.

use std::{collections::HashMap, sync::Arc};

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
#[derive(Debug, Clone)]
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

/// In-memory pub/sub bus for workflow and terminal events.
#[derive(Clone)]
pub struct MessageBus {
    broadcast_tx: broadcast::Sender<BusMessage>,
    subscribers: Arc<RwLock<HashMap<String, Vec<mpsc::Sender<BusMessage>>>>>,
}

impl MessageBus {
    pub fn new(capacity: usize) -> Self {
        let (broadcast_tx, _) = broadcast::channel(capacity);
        Self {
            broadcast_tx,
            subscribers: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    #[allow(clippy::result_large_err)]
    pub fn broadcast(
        &self,
        message: BusMessage,
    ) -> Result<usize, broadcast::error::SendError<BusMessage>> {
        self.broadcast_tx.send(message)
    }

    pub fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage> {
        self.broadcast_tx.subscribe()
    }

    /// Subscribe to a topic-specific mpsc stream.
    pub async fn subscribe(&self, topic: &str) -> mpsc::Receiver<BusMessage> {
        let (tx, rx) = mpsc::channel(100);
        let mut subscribers: tokio::sync::RwLockWriteGuard<
            '_,
            HashMap<String, Vec<mpsc::Sender<BusMessage>>>,
        > = self.subscribers.write().await;
        subscribers.entry(topic.to_string()).or_default().push(tx);
        rx
    }

    /// Returns current subscriber count for a topic.
    pub async fn subscriber_count(&self, topic: &str) -> usize {
        let subscribers = self.subscribers.read().await;
        subscribers.get(topic).map_or(0, Vec::len)
    }

    /// Publish a message and require at least one subscriber.
    ///
    /// Returns the number of subscribers that received the message.
    pub async fn publish_required(
        &self,
        topic: &str,
        message: BusMessage,
    ) -> anyhow::Result<usize> {
        self.publish_inner(topic, message, true).await
    }

    /// Publish a message to all subscribers of a topic.
    pub async fn publish(&self, topic: &str, message: BusMessage) -> anyhow::Result<()> {
        self.publish_inner(topic, message, false).await.map(|_| ())
    }

    /// Publish a workflow-scoped event to both workflow topic and broadcast channel.
    ///
    /// Returns the number of workflow topic subscribers that received the event.
    pub async fn publish_workflow_event(
        &self,
        workflow_id: &str,
        message: BusMessage,
    ) -> anyhow::Result<usize> {
        let topic = format!("{WORKFLOW_TOPIC_PREFIX}{workflow_id}");
        let delivered = self.publish_inner(&topic, message.clone(), false).await?;

        if let Err(err) = self.broadcast(message) {
            tracing::debug!(
                ?err,
                workflow_id = %workflow_id,
                "Workflow broadcast skipped because no broadcast subscribers are active"
            );
        }

        Ok(delivered)
    }

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
    ///
    /// This is called when a new commit is detected in the repository.
    /// For commits without METADATA, this triggers the orchestrator to wake up
    /// and make a decision about the next action.
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
    ///
    /// Called by PromptWatcher when an interactive prompt is detected in PTY output.
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
    ///
    /// Called by Orchestrator after making a decision about how to respond to a prompt.
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

        // Publish to terminal-specific topic first (preferred path for PTY routing).
        // If no terminal-input subscriber is present, fall back to legacy session topic.
        // This avoids silent drop while preventing duplicate PTY delivery.
        let topic = format!("{TERMINAL_INPUT_TOPIC_PREFIX}{terminal_id}");
        let topic_subscriber_count = self.subscriber_count(&topic).await;
        let fallback_topic = session_id.to_string();
        let delivered = if topic_subscriber_count > 0 {
            match self.publish_inner(&topic, message.clone(), false).await {
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
    ///
    /// Called by Orchestrator to notify UI about the decision made for a prompt.
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
    ///
    /// Called after quality gate evaluation completes for a terminal checkpoint.
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
