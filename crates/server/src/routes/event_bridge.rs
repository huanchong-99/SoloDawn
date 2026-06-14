//! Event bridge between MessageBus and WebSocket subscription hub.
//!
//! Listens to MessageBus broadcast channel and routes events to the
//! appropriate workflow WebSocket channels.

use services::services::orchestrator::SharedMessageBus;
use tokio::{sync::broadcast, task::JoinHandle};
use tracing::{debug, info, warn};

use super::{subscription_hub::SharedSubscriptionHub, workflow_events::WsEvent};

// ============================================================================
// Event Bridge
// ============================================================================

/// Bridge between MessageBus and WebSocket subscription hub.
///
/// Spawns a background task that:
/// 1. Listens to MessageBus broadcast channel
/// 2. Converts BusMessage to WsEvent
/// 3. Routes events to the appropriate workflow channel
#[derive(Clone)]
pub struct EventBridge {
    message_bus: SharedMessageBus,
    hub: SharedSubscriptionHub,
}

impl EventBridge {
    /// Create a new event bridge.
    pub fn new(message_bus: SharedMessageBus, hub: SharedSubscriptionHub) -> Self {
        Self { message_bus, hub }
    }

    /// Spawn the event bridge as a background task.
    ///
    /// Returns a JoinHandle that can be used to await or abort the task.
    pub fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    /// Run the event bridge loop.
    ///
    /// This method runs indefinitely until the MessageBus is closed.
    pub async fn run(self) {
        info!("EventBridge started");

        let mut receiver = self.message_bus.subscribe_broadcast();

        loop {
            match receiver.recv().await {
                Ok(message) => {
                    // Convert BusMessage to WsEvent and route to workflow
                    if let Some((workflow_id, event)) = WsEvent::try_from_bus_message(message) {
                        if let Err(err) = self.hub.publish(&workflow_id, event).await {
                            debug!(
                                ?err,
                                workflow_id, "No active subscribers, event cached for replay"
                            );
                        }
                    }
                }
                Err(broadcast::error::RecvError::Lagged(skipped)) => {
                    let notified_channels = self.hub.publish_lagged_to_active(skipped).await;
                    warn!(
                        skipped,
                        notified_channels, "EventBridge receiver lagged, issued recovery signal"
                    );
                }
                Err(broadcast::error::RecvError::Closed) => {
                    info!("MessageBus closed, EventBridge shutting down");
                    break;
                }
            }
        }

        info!("EventBridge stopped");
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use serde_json::json;
    use services::services::orchestrator::{BusMessage, MessageBus};
    use tokio::time::{Duration, timeout};

    use super::{
        super::{subscription_hub::SubscriptionHub, workflow_events::WsEventType},
        *,
    };

    #[tokio::test]
    async fn test_event_bridge_routes_status_update() {
        let message_bus = Arc::new(MessageBus::new(100));
        let hub = Arc::new(SubscriptionHub::new(100));

        let bridge = EventBridge::new(message_bus.clone(), hub.clone());
        let _handle = bridge.spawn();

        // Give the bridge time to start
        tokio::time::sleep(Duration::from_millis(10)).await;

        // Subscribe to workflow events
        let mut rx = hub.subscribe("wf-123").await;

        // Publish a status update via MessageBus
        message_bus
            .broadcast(BusMessage::StatusUpdate {
                workflow_id: "wf-123".to_string(),
                status: "running".to_string(),
            })
            .unwrap();

        // Should receive the event via WebSocket channel
        let result = timeout(Duration::from_millis(100), rx.recv()).await;
        assert!(result.is_ok());

        let event = result.unwrap().unwrap();
        assert_eq!(event.event_type, WsEventType::WorkflowStatusChanged);
        assert_eq!(event.payload["status"], "running");
    }

    #[tokio::test]
    async fn test_event_bridge_routes_git_event() {
        let message_bus = Arc::new(MessageBus::new(100));
        let hub = Arc::new(SubscriptionHub::new(100));

        let bridge = EventBridge::new(message_bus.clone(), hub.clone());
        let _handle = bridge.spawn();

        tokio::time::sleep(Duration::from_millis(10)).await;

        let mut rx = hub.subscribe("wf-456").await;

        message_bus
            .broadcast(BusMessage::GitEvent {
                workflow_id: "wf-456".to_string(),
                commit_hash: "abc123".to_string(),
                branch: "main".to_string(),
                message: "feat: new feature".to_string(),
            })
            .unwrap();

        let result = timeout(Duration::from_millis(100), rx.recv()).await;
        assert!(result.is_ok());

        let event = result.unwrap().unwrap();
        assert_eq!(event.event_type, WsEventType::GitCommitDetected);
        assert_eq!(event.payload["commitHash"], "abc123");
    }

    #[tokio::test]
    async fn test_event_bridge_workflow_isolation() {
        let message_bus = Arc::new(MessageBus::new(100));
        let hub = Arc::new(SubscriptionHub::new(100));

        let bridge = EventBridge::new(message_bus.clone(), hub.clone());
        let _handle = bridge.spawn();

        tokio::time::sleep(Duration::from_millis(10)).await;

        let mut rx1 = hub.subscribe("wf-1").await;
        let mut rx2 = hub.subscribe("wf-2").await;

        // Publish to wf-1 only
        message_bus
            .broadcast(BusMessage::StatusUpdate {
                workflow_id: "wf-1".to_string(),
                status: "running".to_string(),
            })
            .unwrap();

        // wf-1 should receive
        let result1 = timeout(Duration::from_millis(100), rx1.recv()).await;
        assert!(result1.is_ok());

        // wf-2 should NOT receive
        let result2 = timeout(Duration::from_millis(100), rx2.recv()).await;
        assert!(result2.is_err()); // Timeout
    }

    #[tokio::test]
    async fn test_event_bridge_ignores_non_routable_messages() {
        let message_bus = Arc::new(MessageBus::new(100));
        let hub = Arc::new(SubscriptionHub::new(100));

        let bridge = EventBridge::new(message_bus.clone(), hub.clone());
        let _handle = bridge.spawn();

        tokio::time::sleep(Duration::from_millis(10)).await;

        // Subscribe to any workflow
        let mut rx = hub.subscribe("wf-any").await;

        // Publish a Shutdown message (not routable)
        message_bus.broadcast(BusMessage::Shutdown).unwrap();

        // Should NOT receive anything
        let result = timeout(Duration::from_millis(100), rx.recv()).await;
        assert!(result.is_err()); // Timeout
    }

    #[tokio::test]
    async fn test_event_bridge_routes_topic_published_status_when_fanout_used() {
        let message_bus = Arc::new(MessageBus::new(100));
        let hub = Arc::new(SubscriptionHub::new(100));

        let bridge = EventBridge::new(message_bus.clone(), hub.clone());
        let _handle = bridge.spawn();

        tokio::time::sleep(Duration::from_millis(10)).await;

        // Simulate orchestrator workflow topic subscriber present
        let _orchestrator_sub = message_bus.subscribe("workflow:wf-fanout").await;
        let mut ws_rx = hub.subscribe("wf-fanout").await;

        message_bus
            .publish_workflow_event(
                "wf-fanout",
                BusMessage::StatusUpdate {
                    workflow_id: "wf-fanout".to_string(),
                    status: "running".to_string(),
                },
            )
            .await
            .unwrap();

        let result = timeout(Duration::from_millis(100), ws_rx.recv()).await;
        assert!(result.is_ok());

        let event = result.unwrap().unwrap();
        assert_eq!(event.event_type, WsEventType::WorkflowStatusChanged);
        assert_eq!(event.payload["workflowId"], json!("wf-fanout"));
        assert_eq!(event.payload["status"], json!("running"));
    }
}
