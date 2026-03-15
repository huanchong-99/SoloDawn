//! Per-workflow subscription hub for WebSocket event broadcasting.
//!
//! Manages broadcast channels for each workflow, allowing multiple WebSocket
//! connections to subscribe to events for a specific workflow.

use std::{
    collections::{HashMap, VecDeque},
    sync::Arc,
    time::{Duration, Instant},
};

use tokio::sync::{RwLock, broadcast};

use super::workflow_events::WsEvent;

// ============================================================================
// Constants
// ============================================================================

/// Default capacity for per-workflow broadcast channels.
const DEFAULT_CHANNEL_CAPACITY: usize = 256;

// ============================================================================
// Subscription Hub
// ============================================================================

/// TTL for cached pending events (G33-009).
const PENDING_EVENT_TTL: Duration = Duration::from_secs(300); // 5 minutes

/// Per-workflow broadcast hub for WebSocket events.
///
/// Each workflow has its own broadcast channel. When a WebSocket client
/// connects to a workflow, it subscribes to that workflow's channel.
/// Events are then broadcast to all subscribers of that workflow.
#[derive(Clone)]
pub struct SubscriptionHub {
    /// Capacity for each broadcast channel.
    capacity: usize,
    /// Map of workflow_id -> broadcast sender.
    senders: Arc<RwLock<HashMap<String, broadcast::Sender<WsEvent>>>>,
    /// Per-workflow event cache used when no subscribers are connected.
    /// Each entry carries the insertion `Instant` for TTL enforcement (G33-009).
    #[allow(clippy::type_complexity)]
    pending_events: Arc<RwLock<HashMap<String, VecDeque<(WsEvent, Instant)>>>>,
    /// Maximum number of cached events retained per workflow.
    pending_limit: usize,
}

/// Shared subscription hub type alias.
pub type SharedSubscriptionHub = Arc<SubscriptionHub>;

impl SubscriptionHub {
    /// Create a new subscription hub with the specified channel capacity.
    pub fn new(capacity: usize) -> Self {
        Self {
            capacity,
            senders: Arc::new(RwLock::new(HashMap::new())),
            pending_events: Arc::new(RwLock::new(HashMap::new())),
            pending_limit: capacity.max(1),
        }
    }

    /// Subscribe to events for a specific workflow.
    ///
    /// Returns a receiver that will receive all events published to this workflow.
    pub async fn subscribe(&self, workflow_id: &str) -> broadcast::Receiver<WsEvent> {
        let mut senders = self.senders.write().await;
        let sender = if let Some(sender) = senders.get(workflow_id).cloned() {
            sender
        } else {
            let (sender, _) = broadcast::channel(self.capacity);
            senders.insert(workflow_id.to_string(), sender.clone());
            tracing::debug!("Created new channel for workflow: {}", workflow_id);
            sender
        };

        let should_replay_pending = sender.receiver_count() == 0;
        let receiver = sender.subscribe();

        if should_replay_pending {
            self.replay_pending_events(workflow_id, &sender).await;
        }

        drop(senders);

        receiver
    }

    /// Publish an event to all subscribers of a workflow.
    ///
    /// Returns the number of receivers that received the event, or an error
    /// if there are no active subscribers.
    pub async fn publish(
        &self,
        workflow_id: &str,
        event: WsEvent,
    ) -> Result<usize, broadcast::error::SendError<WsEvent>> {
        let Some(sender) = self.get_sender(workflow_id).await else {
            self.cache_pending_event(workflow_id, event.clone()).await;
            return Err(broadcast::error::SendError(event));
        };

        if sender.receiver_count() == 0 {
            self.cache_pending_event(workflow_id, event.clone()).await;
            return Err(broadcast::error::SendError(event));
        }

        sender.send(event)
    }

    /// Publish lagged notification to all active workflow subscribers.
    ///
    /// Returns the number of workflow channels that received the lagged event.
    pub async fn publish_lagged_to_active(&self, skipped: u64) -> usize {
        let active_senders: Vec<broadcast::Sender<WsEvent>> = {
            let senders = self.senders.read().await;
            senders
                .values()
                .filter(|sender| sender.receiver_count() > 0)
                .cloned()
                .collect()
        };

        if active_senders.is_empty() {
            return 0;
        }

        let lagged_event = WsEvent::lagged(skipped);
        active_senders
            .iter()
            .filter(|sender| sender.send(lagged_event.clone()).is_ok())
            .count()
    }

    /// Get the number of active subscribers for a workflow.
    pub async fn subscriber_count(&self, workflow_id: &str) -> usize {
        let senders = self.senders.read().await;
        senders
            .get(workflow_id)
            .map_or(0, broadcast::Sender::receiver_count)
    }

    /// Check if a workflow has any active subscribers.
    pub async fn has_subscribers(&self, workflow_id: &str) -> bool {
        self.subscriber_count(workflow_id).await > 0
    }

    /// Clean up the channel for a workflow if it has no subscribers.
    ///
    /// This should be called when a WebSocket connection closes to prevent
    /// memory leaks from unused channels.
    pub async fn cleanup_if_idle(&self, workflow_id: &str) {
        let mut senders = self.senders.write().await;
        let should_remove = senders
            .get(workflow_id)
            .is_some_and(|sender| sender.receiver_count() == 0);

        if should_remove {
            senders.remove(workflow_id);

            // Also clean up any pending events cached for this workflow (G08-009)
            let mut pending = self.pending_events.write().await;
            pending.remove(workflow_id);

            tracing::debug!(
                "Cleaned up idle channel and pending events for workflow: {}",
                workflow_id
            );
        } else {
            // G33-009: even if the channel is still active, evict expired pending_events
            // entries for this workflow so stale events don't accumulate.
            let mut pending = self.pending_events.write().await;
            if let Some(queue) = pending.get_mut(workflow_id) {
                let now = Instant::now();
                let before = queue.len();
                queue.retain(|(_, inserted_at)| {
                    now.duration_since(*inserted_at) < PENDING_EVENT_TTL
                });
                let evicted = before.saturating_sub(queue.len());
                if evicted > 0 {
                    tracing::debug!(
                        workflow_id,
                        evicted,
                        "Evicted expired pending_events entries (TTL)"
                    );
                }
            }
        }
    }

    /// Get the number of active workflow channels.
    pub async fn channel_count(&self) -> usize {
        self.senders.read().await.len()
    }

    async fn get_sender(&self, workflow_id: &str) -> Option<broadcast::Sender<WsEvent>> {
        self.senders.read().await.get(workflow_id).cloned()
    }

    async fn cache_pending_event(&self, workflow_id: &str, event: WsEvent) {
        let mut pending_events = self.pending_events.write().await;
        let queue = pending_events.entry(workflow_id.to_string()).or_default();
        queue.push_back((event, Instant::now()));

        while queue.len() > self.pending_limit {
            queue.pop_front();
        }
    }

    async fn replay_pending_events(&self, workflow_id: &str, sender: &broadcast::Sender<WsEvent>) {
        let pending_events = {
            let mut pending_events = self.pending_events.write().await;
            pending_events.remove(workflow_id).unwrap_or_default()
        };

        if pending_events.is_empty() {
            return;
        }

        let now = Instant::now();
        let mut replay_count = 0usize;
        let mut expired_count = 0usize;

        for (event, inserted_at) in pending_events {
            // G33-009: skip events older than PENDING_EVENT_TTL
            if now.duration_since(inserted_at) >= PENDING_EVENT_TTL {
                expired_count += 1;
                continue;
            }
            if sender.send(event).is_err() {
                break;
            }
            replay_count += 1;
        }

        if expired_count > 0 {
            tracing::debug!(
                workflow_id,
                expired_count,
                "Discarded expired pending workflow events (TTL exceeded)"
            );
        }

        tracing::debug!(
            workflow_id,
            replay_count,
            "Replayed pending workflow events to first subscriber"
        );
    }

    #[allow(dead_code)]
    /// Get or create a broadcast sender for a workflow.
    async fn get_or_create_sender(&self, workflow_id: &str) -> broadcast::Sender<WsEvent> {
        // Fast path: check if sender already exists
        if let Some(sender) = self.senders.read().await.get(workflow_id).cloned() {
            return sender;
        }

        // Slow path: create new sender
        let mut senders = self.senders.write().await;

        // Double-check after acquiring write lock
        if let Some(sender) = senders.get(workflow_id).cloned() {
            return sender;
        }

        // Create new channel
        let (sender, _) = broadcast::channel(self.capacity);
        senders.insert(workflow_id.to_string(), sender.clone());
        tracing::debug!("Created new channel for workflow: {}", workflow_id);

        sender
    }
}

impl Default for SubscriptionHub {
    fn default() -> Self {
        Self::new(DEFAULT_CHANNEL_CAPACITY)
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use serde_json::json;
    use tokio::time::Duration;

    use super::{super::workflow_events::WsEventType, *};

    #[tokio::test]
    async fn test_subscription_hub_creation() {
        let hub = SubscriptionHub::new(100);
        assert_eq!(hub.channel_count().await, 0);
    }

    #[tokio::test]
    async fn test_subscribe_creates_channel() {
        let hub = SubscriptionHub::new(100);

        let _rx = hub.subscribe("workflow-1").await;

        assert!(hub.has_subscribers("workflow-1").await);
        assert_eq!(hub.subscriber_count("workflow-1").await, 1);
        assert_eq!(hub.channel_count().await, 1);
    }

    #[tokio::test]
    async fn test_multiple_subscribers() {
        let hub = SubscriptionHub::new(100);

        let _rx1 = hub.subscribe("workflow-1").await;
        let _rx2 = hub.subscribe("workflow-1").await;
        let _rx3 = hub.subscribe("workflow-1").await;

        assert_eq!(hub.subscriber_count("workflow-1").await, 3);
    }

    #[tokio::test]
    async fn test_publish_to_subscribers() {
        let hub = SubscriptionHub::new(100);

        let mut rx1 = hub.subscribe("workflow-1").await;
        let mut rx2 = hub.subscribe("workflow-1").await;

        let event = WsEvent::new(
            WsEventType::WorkflowStatusChanged,
            json!({"status": "running"}),
        );

        let result = hub.publish("workflow-1", event.clone()).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), 2); // 2 receivers

        // Both receivers should get the event
        let received1 = rx1.recv().await.unwrap();
        let received2 = rx2.recv().await.unwrap();

        assert_eq!(received1.event_type, WsEventType::WorkflowStatusChanged);
        assert_eq!(received2.event_type, WsEventType::WorkflowStatusChanged);
    }

    #[tokio::test]
    async fn test_workflow_isolation() {
        let hub = SubscriptionHub::new(100);

        let mut rx1 = hub.subscribe("workflow-1").await;
        let mut rx2 = hub.subscribe("workflow-2").await;

        let event = WsEvent::new(WsEventType::SystemHeartbeat, json!({}));

        // Publish to workflow-1 only
        hub.publish("workflow-1", event).await.unwrap();

        // workflow-1 should receive
        let result1 = tokio::time::timeout(std::time::Duration::from_millis(100), rx1.recv()).await;
        assert!(result1.is_ok());

        // workflow-2 should NOT receive (timeout)
        let result2 = tokio::time::timeout(std::time::Duration::from_millis(100), rx2.recv()).await;
        assert!(result2.is_err()); // Timeout
    }

    #[tokio::test]
    async fn test_cleanup_if_idle() {
        let hub = SubscriptionHub::new(100);

        {
            let _rx = hub.subscribe("workflow-1").await;
            assert!(hub.has_subscribers("workflow-1").await);
        }
        // rx dropped here

        // Channel still exists but has no subscribers
        hub.cleanup_if_idle("workflow-1").await;

        // Channel should be removed
        assert_eq!(hub.channel_count().await, 0);
    }

    #[tokio::test]
    async fn test_cleanup_with_active_subscribers() {
        let hub = SubscriptionHub::new(100);

        let _rx = hub.subscribe("workflow-1").await;

        // Try to cleanup while subscriber is active
        hub.cleanup_if_idle("workflow-1").await;

        // Channel should NOT be removed
        assert!(hub.has_subscribers("workflow-1").await);
        assert_eq!(hub.channel_count().await, 1);
    }

    #[tokio::test]
    async fn test_publish_no_subscribers() {
        let hub = SubscriptionHub::new(100);

        let event = WsEvent::heartbeat();
        let result = hub.publish("workflow-1", event).await;

        // Should fail because no subscribers
        assert!(result.is_err());
        assert_eq!(hub.channel_count().await, 0);
    }

    #[tokio::test]
    async fn test_publish_without_subscribers_replays_from_cache() {
        let hub = SubscriptionHub::new(100);

        let event = WsEvent::new(
            WsEventType::WorkflowStatusChanged,
            json!({"status": "queued"}),
        );

        let result = hub.publish("workflow-1", event.clone()).await;
        assert!(result.is_err());
        assert_eq!(hub.channel_count().await, 0);

        let mut rx = hub.subscribe("workflow-1").await;
        assert_eq!(hub.channel_count().await, 1);

        let replayed = tokio::time::timeout(Duration::from_millis(100), rx.recv()).await;
        assert!(replayed.is_ok());

        let replayed = replayed.unwrap().unwrap();
        assert_eq!(replayed.event_type, event.event_type);
        assert_eq!(replayed.payload["status"], "queued");
    }

    #[tokio::test]
    async fn test_default_hub() {
        let hub = SubscriptionHub::default();
        assert_eq!(hub.capacity, DEFAULT_CHANNEL_CAPACITY);
    }
}
