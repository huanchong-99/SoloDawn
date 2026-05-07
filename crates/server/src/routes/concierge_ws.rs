//! Concierge WebSocket route for real-time message streaming.

use std::sync::Arc;

use axum::{
    Extension, Router,
    extract::{
        Path, State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    response::IntoResponse,
    routing::get,
};
use deployment::Deployment;
use futures_util::{SinkExt, StreamExt};
use serde::Deserialize;
use services::services::concierge::{ConciergeAgent, ConciergeBroadcaster};
use tokio::{
    task::JoinSet,
    time::{Duration, interval},
};
use tracing::{debug, warn};

use crate::{DeploymentImpl, error::ApiError};

const WS_HEARTBEAT_INTERVAL_SECS: u64 = 30;

#[derive(Debug, Deserialize)]
struct ConciergeWsClientMessage {
    #[serde(rename = "type")]
    message_type: String,
    #[serde(default)]
    payload: serde_json::Value,
}

pub fn concierge_ws_routes() -> Router<DeploymentImpl> {
    Router::new().route("/concierge/{session_id}/events", get(concierge_ws_handler))
}

async fn concierge_ws_handler(
    State(deployment): State<DeploymentImpl>,
    Extension(concierge): Extension<Arc<ConciergeAgent>>,
    Extension(broadcaster): Extension<Arc<ConciergeBroadcaster>>,
    Path(session_id): Path<String>,
    ws: WebSocketUpgrade,
) -> Result<impl IntoResponse, ApiError> {
    // Verify session exists
    let pool = &deployment.db().pool;
    let _session = db::models::concierge::ConciergeSession::find_by_id(pool, &session_id)
        .await
        .map_err(|e| ApiError::Internal(format!("{e}")))?
        .ok_or_else(|| ApiError::NotFound("Session not found".to_string()))?;

    Ok(
        ws.on_upgrade(move |socket| {
            handle_concierge_ws(socket, concierge, broadcaster, session_id)
        }),
    )
}

async fn handle_concierge_ws(
    socket: WebSocket,
    concierge: Arc<ConciergeAgent>,
    broadcaster: Arc<ConciergeBroadcaster>,
    session_id: String,
) {
    let (mut ws_tx, mut ws_rx) = socket.split();

    // Subscribe to concierge events
    let mut event_rx = broadcaster.subscribe(&session_id);

    // Heartbeat timer
    let mut heartbeat = interval(Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));

    // E25-12: Track message-processing tasks spawned for this WS so they can
    // be aborted when the connection closes. Without this, a slow
    // `process_message` call outlives the disconnected client.
    let mut in_flight: JoinSet<()> = JoinSet::new();

    debug!(session_id = %session_id, "Concierge WebSocket connected");

    loop {
        tokio::select! {
            // Forward concierge events to WS client
            event = event_rx.recv() => {
                match event {
                    Ok(concierge_event) => {
                        // W2-20-08: Concierge events are intentionally NOT modeled by the
                        // workflow `WsEventType` enum (see workflow_events.rs). They are
                        // delivered on a dedicated `/concierge/{session_id}/events`
                        // socket and consumed by `frontend/src/stores/conciergeWsStore.ts`,
                        // which defines its own `ConciergeEventType` union. Keeping the
                        // contracts separate avoids mixing the concierge chat channel
                        // with workflow-scoped broadcasts that are keyed by workflow id.
                        let event_json = serde_json::json!({
                            "type": match &concierge_event {
                                services::services::concierge::ConciergeEvent::NewMessage { .. } => "concierge.message",
                                services::services::concierge::ConciergeEvent::ToolExecuting { .. } => "concierge.tool_executing",
                                services::services::concierge::ConciergeEvent::SessionUpdated { .. } => "concierge.session_updated",
                            },
                            "payload": concierge_event,
                            "timestamp": chrono::Utc::now().to_rfc3339(),
                        });
                        if ws_tx.send(Message::Text(event_json.to_string().into())).await.is_err() {
                            break;
                        }
                    }
                    Err(tokio::sync::broadcast::error::RecvError::Lagged(n)) => {
                        warn!(session_id = %session_id, "Concierge WS lagged by {n} messages");
                    }
                    Err(tokio::sync::broadcast::error::RecvError::Closed) => {
                        break;
                    }
                }
            }

            // Handle incoming WS messages from client
            msg = ws_rx.next() => {
                match msg {
                    Some(Ok(Message::Text(text))) => {
                        if let Ok(client_msg) = serde_json::from_str::<ConciergeWsClientMessage>(&text) {
                            match client_msg.message_type.as_str() {
                                "concierge.send_message" => {
                                    if let Some(content) = client_msg.payload["content"].as_str() {
                                        let concierge = concierge.clone();
                                        let sid = session_id.clone();
                                        let content = content.to_string();
                                        // E25-12: spawn into `in_flight` so the task is aborted
                                        // when this WS loop exits (client disconnect).
                                        in_flight.spawn(async move {
                                            if let Err(e) = concierge.process_message(&sid, &content, Some("web"), None).await {
                                                warn!("Concierge WS message processing failed: {e}");
                                            }
                                        });
                                    }
                                }
                                "system.heartbeat" => {
                                    // Client heartbeat acknowledged
                                }
                                _ => {}
                            }
                        }
                    }
                    Some(Ok(Message::Close(_))) | None => break,
                    _ => {}
                }
            }

            // Send heartbeat
            _ = heartbeat.tick() => {
                let hb = serde_json::json!({
                    "type": "system.heartbeat",
                    "timestamp": chrono::Utc::now().to_rfc3339(),
                });
                if ws_tx.send(Message::Text(hb.to_string().into())).await.is_err() {
                    break;
                }
            }
        }
    }

    // E25-12: abort any still-running message-processing tasks so they don't
    // outlive the closed connection.
    in_flight.abort_all();
    while in_flight.join_next().await.is_some() {}

    debug!(session_id = %session_id, "Concierge WebSocket disconnected");
}
