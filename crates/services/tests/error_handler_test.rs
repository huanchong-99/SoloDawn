//! Error Handler Tests

use std::sync::Arc;

use services::orchestrator::message_bus::{BusMessage, MessageBus};

#[cfg(test)]
mod tests {
    use super::*;

    /// Test error handler initialization
    #[tokio::test]
    async fn test_error_handler_init() {
        // This is a placeholder test
        // Real integration tests require a test database setup

        // Create a mock message bus to verify it compiles
        let _message_bus = Arc::new(MessageBus::new(100));

        // Note: Cannot fully test without DB connection
    }

    /// Test error message broadcasting
    #[tokio::test]
    async fn test_error_message_broadcast() {
        let message_bus = Arc::new(MessageBus::new(100));

        // Subscribe to workflow topic
        let mut rx = message_bus.subscribe("workflow:test-workflow").await;

        // Publish error message
        let event = BusMessage::Error {
            workflow_id: "test-workflow".to_string(),
            error: "Test error".to_string(),
        };

        message_bus
            .publish("workflow:test-workflow", event)
            .await
            .unwrap();

        // Receive message
        let received = rx.recv().await.unwrap();
        match received {
            BusMessage::Error { workflow_id, error } => {
                assert_eq!(workflow_id, "test-workflow");
                assert_eq!(error, "Test error");
            }
            _ => panic!("Expected Error message"),
        }
    }
}
