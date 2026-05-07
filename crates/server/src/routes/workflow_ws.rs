//! Workflow WebSocket route for real-time event streaming.
//!
//! Provides a WebSocket endpoint for clients to subscribe to workflow events.
//! Events include workflow status changes, terminal updates, git commits, etc.

use axum::{
    Extension, Router,
    extract::{
        Path, State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    http::HeaderMap,
    response::IntoResponse,
    routing::get,
};
use deployment::Deployment;
use futures_util::{SinkExt, StreamExt};
use serde::Deserialize;
use services::services::orchestrator::OrchestratorRuntime;
use tokio::{
    sync::broadcast,
    time::{Duration, interval},
};
use tracing::{debug, info, warn};

use super::{
    subscription_hub::SharedSubscriptionHub, terminal_ws::validate_terminal_id,
    workflow_events::WsEvent, ws_origin::validate_ws_origin,
};
use crate::{DeploymentImpl, error::ApiError};

// ============================================================================
// Constants
// ============================================================================

/// Heartbeat interval for keep-alive (30 seconds).
const WS_HEARTBEAT_INTERVAL_SECS: u64 = 30;

/// Client message type for responding to interactive terminal prompts.
const WS_CLIENT_PROMPT_RESPONSE_TYPE: &str = "terminal.prompt_response";

/// Client message type for keep-alive heartbeats.
const WS_CLIENT_HEARTBEAT_TYPE: &str = "system.heartbeat";

#[derive(Debug, Deserialize)]
struct WorkflowWsClientMessage {
    #[serde(rename = "type")]
    message_type: String,
    #[serde(default)]
    payload: serde_json::Value,
}

#[derive(Debug, Deserialize)]
struct PromptResponsePayload {
    #[serde(rename = "workflowId", alias = "workflow_id")]
    workflow_id: Option<String>,
    #[serde(rename = "terminalId", alias = "terminal_id")]
    terminal_id: String,
    response: String,
}

// ============================================================================
// Route Definition
// ============================================================================

/// Create workflow WebSocket routes.
///
/// Routes:
/// - `GET /workflow/:id/events` - Subscribe to workflow events
pub fn workflow_ws_routes() -> Router<DeploymentImpl> {
    Router::new().route("/workflow/{id}/events", get(workflow_ws_handler))
}

// ============================================================================
// Route Handlers
// ============================================================================

/// WebSocket handler for workflow event subscription.
async fn workflow_ws_handler(
    headers: HeaderMap,
    ws: WebSocketUpgrade,
    Path(workflow_id): Path<String>,
    State(deployment): State<DeploymentImpl>,
    Extension(hub): Extension<SharedSubscriptionHub>,
) -> impl IntoResponse {
    // SEC-003: Validate Origin header before WebSocket upgrade
    if let Err((status, msg)) = validate_ws_origin(&headers) {
        return (status, msg).into_response();
    }

    // Validate workflow_id format (UUID)
    if let Err(e) = validate_terminal_id(&workflow_id) {
        warn!("Invalid workflow_id format: {} - {}", workflow_id, e);
        return ApiError::BadRequest(format!("Invalid workflow_id format: {e}")).into_response();
    }

    // SEC-017: Enforce WebSocket message size limits (256 KB)
    ws.max_message_size(256 * 1024)
        .max_frame_size(256 * 1024)
        .on_upgrade(move |socket| handle_workflow_socket(socket, workflow_id, hub, deployment))
}

/// Handle workflow WebSocket connection.
async fn handle_workflow_socket(
    socket: WebSocket,
    workflow_id: String,
    hub: SharedSubscriptionHub,
    deployment: DeploymentImpl,
) {
    info!("Workflow WS connected: {}", workflow_id);

    // Subscribe to workflow events
    let mut receiver = hub.subscribe(&workflow_id).await;
    let (mut ws_sender, mut ws_receiver) = socket.split();

    // Heartbeat interval
    let mut heartbeat = interval(Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));

    // Clone for cleanup
    let workflow_id_cleanup = workflow_id.clone();
    let hub_cleanup = hub.clone();
    let workflow_id_for_recv = workflow_id.clone();
    let runtime = deployment.orchestrator_runtime().clone();

    // Send task: forwards events from hub to WebSocket
    let send_task = tokio::spawn(async move {
        loop {
            tokio::select! {
                // Send heartbeat periodically
                _ = heartbeat.tick() => {
                    let event = WsEvent::heartbeat();
                    if !send_ws_event(&mut ws_sender, &event).await {
                        debug!("Failed to send heartbeat, closing connection");
                        break;
                    }
                }

                // Forward events from subscription hub
                msg = receiver.recv() => {
                    match msg {
                        Ok(event) => {
                            if !send_ws_event(&mut ws_sender, &event).await {
                                debug!("Failed to send event, closing connection");
                                break;
                            }
                        }
                        Err(broadcast::error::RecvError::Lagged(skipped)) => {
                            warn!("Workflow WS receiver lagged, skipped {} messages", skipped);
                            let lagged_event = WsEvent::lagged(skipped);
                            if !send_ws_event(&mut ws_sender, &lagged_event).await {
                                break;
                            }
                        }
                        Err(broadcast::error::RecvError::Closed) => {
                            debug!("Subscription hub closed");
                            break;
                        }
                    }
                }
            }
        }
    });

    // Receive task: handles incoming messages from client
    let recv_task = tokio::spawn(async move {
        while let Some(result) = ws_receiver.next().await {
            match result {
                Ok(Message::Close(_)) => {
                    debug!("Client requested close");
                    break;
                }
                Ok(Message::Ping(_)) => {
                    // Ping handled automatically by axum
                }
                Ok(Message::Pong(_)) => {
                    // Pong received
                }
                Ok(Message::Text(text)) => {
                    handle_client_text_message(&text, &workflow_id_for_recv, &runtime).await;
                }
                Ok(Message::Binary(_)) => {
                    // Binary messages not supported
                }
                Err(e) => {
                    debug!("WebSocket error: {}", e);
                    break;
                }
            }
        }
    });

    // Wait for either task to complete.
    // NOTE(G12-007, W2-19-03): tokio::select! automatically drops the losing future
    // when one branch completes. Since send_task and recv_task are JoinHandles,
    // dropping them does NOT abort the spawned task — but the spawned tasks will
    // naturally terminate once the WebSocket half they own is dropped/closed. This is
    // the idiomatic pattern for paired send/recv WebSocket loops; no explicit abort
    // or JoinHandle retention is needed.
    tokio::select! {
        _ = send_task => {
            debug!("Send task completed for workflow {}", workflow_id_cleanup);
        }
        _ = recv_task => {
            debug!("Receive task completed for workflow {}", workflow_id_cleanup);
        }
    }

    // Cleanup: remove channel if no more subscribers
    hub_cleanup.cleanup_if_idle(&workflow_id_cleanup).await;

    info!("Workflow WS disconnected: {}", workflow_id_cleanup);
}

async fn handle_client_text_message(text: &str, workflow_id: &str, runtime: &OrchestratorRuntime) {
    let message = match serde_json::from_str::<WorkflowWsClientMessage>(text) {
        Ok(message) => message,
        Err(err) => {
            debug!(
                workflow_id = %workflow_id,
                error = %err,
                "Ignoring non-JSON workflow WS client message"
            );
            return;
        }
    };

    match message.message_type.as_str() {
        WS_CLIENT_HEARTBEAT_TYPE => {
            // Heartbeat acknowledged implicitly.
        }
        WS_CLIENT_PROMPT_RESPONSE_TYPE => {
            let payload = match serde_json::from_value::<PromptResponsePayload>(message.payload) {
                Ok(payload) => payload,
                Err(err) => {
                    warn!(
                        workflow_id = %workflow_id,
                        error = %err,
                        "Invalid terminal.prompt_response payload"
                    );
                    return;
                }
            };

            let terminal_id = payload.terminal_id.trim();
            if terminal_id.is_empty() {
                warn!(
                    workflow_id = %workflow_id,
                    "Ignoring terminal.prompt_response with empty terminal_id"
                );
                return;
            }

            let response = payload.response.as_str();

            if let Some(payload_workflow_id) = payload.workflow_id.as_deref()
                && payload_workflow_id != workflow_id
            {
                warn!(
                    workflow_id = %workflow_id,
                    payload_workflow_id = %payload_workflow_id,
                    "Prompt response payload workflow_id mismatch; using WS path workflow_id"
                );
            }

            if let Err(err) = runtime
                .submit_user_prompt_response(workflow_id, terminal_id, response)
                .await
            {
                warn!(
                    workflow_id = %workflow_id,
                    terminal_id = %terminal_id,
                    error = %err,
                    "Failed to forward terminal prompt response to orchestrator runtime"
                );
                return;
            }

            debug!(
                workflow_id = %workflow_id,
                terminal_id = %terminal_id,
                "Forwarded terminal prompt response to orchestrator runtime"
            );
        }
        other_type => {
            debug!(
                workflow_id = %workflow_id,
                message_type = %other_type,
                "Ignoring unsupported workflow WS client message"
            );
        }
    }
}

/// Send a WebSocket event to the client.
///
/// Returns `true` if successful, `false` if the connection should be closed.
async fn send_ws_event(
    ws_sender: &mut futures_util::stream::SplitSink<WebSocket, Message>,
    event: &WsEvent,
) -> bool {
    let json = match serde_json::to_string(event) {
        Ok(json) => json,
        Err(err) => {
            warn!("Failed to serialize WsEvent: {}", err);
            return false;
        }
    };

    ws_sender.send(Message::Text(json.into())).await.is_ok()
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use db::DBService;
    use services::services::orchestrator::MessageBus;
    use tracing::{Event, Level, Subscriber};
    use tracing_subscriber::{Layer, Registry, layer::Context, prelude::*};

    use super::*;

    #[derive(Clone, Debug)]
    struct CapturedLog {
        level: Level,
        message: Option<String>,
    }

    #[derive(Clone, Default)]
    struct CapturedLogs {
        entries: Arc<Mutex<Vec<CapturedLog>>>,
    }

    impl CapturedLogs {
        fn push(&self, level: Level, message: Option<String>) {
            self.entries
                .lock()
                .expect("captured log mutex poisoned")
                .push(CapturedLog { level, message });
        }

        fn messages_for_level(&self, level: Level) -> Vec<String> {
            self.entries
                .lock()
                .expect("captured log mutex poisoned")
                .iter()
                .filter(|entry| entry.level == level)
                .filter_map(|entry| entry.message.clone())
                .collect()
        }
    }

    #[derive(Default)]
    struct MessageVisitor {
        message: Option<String>,
    }

    impl tracing::field::Visit for MessageVisitor {
        fn record_str(&mut self, field: &tracing::field::Field, value: &str) {
            if field.name() == "message" {
                self.message = Some(value.to_string());
            }
        }

        fn record_debug(&mut self, field: &tracing::field::Field, value: &dyn std::fmt::Debug) {
            if field.name() == "message" {
                self.message = Some(format!("{value:?}"));
            }
        }
    }

    #[derive(Clone)]
    struct CaptureLayer {
        logs: CapturedLogs,
    }

    impl<S> Layer<S> for CaptureLayer
    where
        S: Subscriber,
    {
        fn on_event(&self, event: &Event<'_>, _ctx: Context<'_, S>) {
            let mut visitor = MessageVisitor::default();
            event.record(&mut visitor);
            self.logs.push(*event.metadata().level(), visitor.message);
        }
    }

    async fn test_runtime() -> OrchestratorRuntime {
        let pool = sqlx::SqlitePool::connect(":memory:")
            .await
            .expect("in-memory sqlite should be available");
        let db = Arc::new(DBService { pool });
        let message_bus = Arc::new(MessageBus::new(32));
        OrchestratorRuntime::new(db, message_bus)
    }

    #[test]
    fn test_workflow_ws_routes_created() {
        // Verify routes can be created without panic
        let _routes = workflow_ws_routes();
    }

    #[tokio::test]
    async fn test_prompt_response_message_dispatches_to_runtime() {
        let runtime = test_runtime().await;
        let workflow_id = "00000000-0000-0000-0000-000000000001";
        let message = serde_json::json!({
            "type": WS_CLIENT_PROMPT_RESPONSE_TYPE,
            "payload": {
                "terminalId": "terminal-1",
                "response": "yes"
            }
        })
        .to_string();

        let logs = CapturedLogs::default();
        let subscriber = Registry::default().with(CaptureLayer { logs: logs.clone() });
        let dispatch = tracing::Dispatch::new(subscriber);
        let _guard = tracing::dispatcher::set_default(&dispatch);

        handle_client_text_message(&message, workflow_id, &runtime).await;

        let warn_messages = logs.messages_for_level(Level::WARN);
        assert!(
            warn_messages.iter().any(|msg| msg
                .contains("Failed to forward terminal prompt response to orchestrator runtime")),
            "Expected runtime forwarding warning after dispatch, got: {warn_messages:?}"
        );
    }

    #[tokio::test]
    async fn test_prompt_response_with_invalid_payload_does_not_panic() {
        let runtime = test_runtime().await;
        let workflow_id = "00000000-0000-0000-0000-000000000002";
        let message = serde_json::json!({
            "type": WS_CLIENT_PROMPT_RESPONSE_TYPE,
            "payload": {
                "terminalId": 123,
                "response": "yes"
            }
        })
        .to_string();

        let logs = CapturedLogs::default();
        let subscriber = Registry::default().with(CaptureLayer { logs: logs.clone() });
        let dispatch = tracing::Dispatch::new(subscriber);
        let _guard = tracing::dispatcher::set_default(&dispatch);

        handle_client_text_message(&message, workflow_id, &runtime).await;

        let warn_messages = logs.messages_for_level(Level::WARN);
        assert!(
            warn_messages
                .iter()
                .any(|msg| msg.contains("Invalid terminal.prompt_response payload")),
            "Expected invalid payload warning, got: {warn_messages:?}"
        );
        assert!(
            !warn_messages.iter().any(|msg| msg
                .contains("Failed to forward terminal prompt response to orchestrator runtime")),
            "Invalid payload should not reach runtime forwarding branch"
        );
    }

    #[tokio::test]
    async fn test_unknown_client_message_type_is_ignored_without_panic() {
        let runtime = test_runtime().await;
        let workflow_id = "00000000-0000-0000-0000-000000000003";
        let message = serde_json::json!({
            "type": "terminal.unknown_type",
            "payload": {
                "terminalId": "terminal-1",
                "response": "noop"
            }
        })
        .to_string();

        let logs = CapturedLogs::default();
        let subscriber = Registry::default().with(CaptureLayer { logs: logs.clone() });
        let dispatch = tracing::Dispatch::new(subscriber);
        let _guard = tracing::dispatcher::set_default(&dispatch);

        handle_client_text_message(&message, workflow_id, &runtime).await;

        let debug_messages = logs.messages_for_level(Level::DEBUG);
        assert!(
            debug_messages
                .iter()
                .any(|msg| msg.contains("Ignoring unsupported workflow WS client message")),
            "Expected unsupported-message debug log, got: {debug_messages:?}"
        );

        let warn_messages = logs.messages_for_level(Level::WARN);
        assert!(
            !warn_messages
                .iter()
                .any(|msg| msg.contains("Invalid terminal.prompt_response payload")),
            "Unknown type should not be parsed as prompt response"
        );
        assert!(
            !warn_messages.iter().any(|msg| msg
                .contains("Failed to forward terminal prompt response to orchestrator runtime")),
            "Unknown type should not trigger runtime forwarding"
        );
    }

    #[tokio::test]
    async fn test_prompt_response_with_empty_response_is_forwarded() {
        let runtime = test_runtime().await;
        let workflow_id = "00000000-0000-0000-0000-000000000004";
        let message = serde_json::json!({
            "type": WS_CLIENT_PROMPT_RESPONSE_TYPE,
            "payload": {
                "terminalId": "terminal-1",
                "response": ""
            }
        })
        .to_string();

        let logs = CapturedLogs::default();
        let subscriber = Registry::default().with(CaptureLayer { logs: logs.clone() });
        let dispatch = tracing::Dispatch::new(subscriber);
        let _guard = tracing::dispatcher::set_default(&dispatch);

        handle_client_text_message(&message, workflow_id, &runtime).await;

        let warn_messages = logs.messages_for_level(Level::WARN);
        assert!(
            warn_messages.iter().any(|msg| msg
                .contains("Failed to forward terminal prompt response to orchestrator runtime")),
            "Expected runtime forwarding warning after dispatch, got: {warn_messages:?}"
        );
        assert!(
            !warn_messages.iter().any(|msg| msg
                .contains("Ignoring terminal.prompt_response with empty terminal_id")),
            "Empty response should not be rejected as invalid payload"
        );
    }

    #[tokio::test]
    async fn test_prompt_response_with_empty_terminal_id_is_ignored() {
        let runtime = test_runtime().await;
        let workflow_id = "00000000-0000-0000-0000-000000000005";
        let message = serde_json::json!({
            "type": WS_CLIENT_PROMPT_RESPONSE_TYPE,
            "payload": {
                "terminalId": "   ",
                "response": "yes"
            }
        })
        .to_string();

        let logs = CapturedLogs::default();
        let subscriber = Registry::default().with(CaptureLayer { logs: logs.clone() });
        let dispatch = tracing::Dispatch::new(subscriber);
        let _guard = tracing::dispatcher::set_default(&dispatch);

        handle_client_text_message(&message, workflow_id, &runtime).await;

        let warn_messages = logs.messages_for_level(Level::WARN);
        assert!(
            warn_messages
                .iter()
                .any(|msg| msg.contains("Ignoring terminal.prompt_response with empty terminal_id")),
            "Expected empty-terminal-id warning, got: {warn_messages:?}"
        );
        assert!(
            !warn_messages.iter().any(|msg| msg
                .contains("Failed to forward terminal prompt response to orchestrator runtime")),
            "Empty terminal_id should not trigger runtime forwarding"
        );
    }
}
