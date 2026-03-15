//! SSE endpoint for real-time CLI status change streaming.
//!
//! Provides a Server-Sent Events stream at `GET /api/cli_types/status/stream`
//! that pushes `CliStatusChange` events whenever the background health monitor
//! detects a change in CLI availability.

use std::convert::Infallible;
use std::time::Duration;

use axum::{
    Extension, Router,
    response::{
        IntoResponse, Response, Sse,
        sse::{Event, KeepAlive},
    },
    routing::get,
};
use futures_util::stream::{self, StreamExt};
use services::services::cli_health_monitor::SharedCliHealthMonitor;
use tokio_stream::wrappers::BroadcastStream;

/// Build the CLI status SSE routes.
///
/// Intended to be nested under `/api/cli_types` in the main router:
/// ```ignore
/// .nest("/cli_types", cli_status_sse::cli_status_sse_routes())
/// ```
pub fn cli_status_sse_routes() -> Router<crate::DeploymentImpl> {
    Router::new().route("/status/stream", get(cli_status_stream))
}

/// GET /api/cli_types/status/stream
///
/// Server-Sent Events endpoint for real-time CLI status changes.
///
/// On connection the client receives:
/// 1. A `connection_established` event with the current cached statuses.
/// 2. Subsequent `cli_status_change` events whenever the monitor detects a change.
/// 3. Keep-alive comment frames every 30 seconds.
async fn cli_status_stream(
    Extension(monitor): Extension<SharedCliHealthMonitor>,
) -> Response {
    // Snapshot current cached statuses for the initial event
    let cached = monitor.get_cached_statuses().await;
    let initial_data =
        serde_json::to_string(&cached).unwrap_or_else(|_| "[]".to_string());

    let initial_event: Result<Event, Infallible> = Ok(Event::default()
        .event("connection_established")
        .data(initial_data));

    // Subscribe to the broadcast channel *before* yielding the initial event
    // so we don't miss any changes that happen concurrently.
    let rx = monitor.subscribe();
    let change_stream = BroadcastStream::new(rx).filter_map(|result| async {
        match result {
            Ok(change) => {
                let data = serde_json::to_string(&change)
                    .unwrap_or_else(|_| "{}".to_string());
                Some(Ok(Event::default()
                    .event("cli_status_change")
                    .data(data)))
            }
            Err(tokio_stream::wrappers::errors::BroadcastStreamRecvError::Lagged(n)) => {
                tracing::warn!(skipped = n, "CLI status SSE client lagged behind");
                Some(Ok(Event::default()
                    .event("lagged")
                    .data(format!("{{\"skipped\":{n}}}"))))
            }
        }
    });

    // Prepend the initial event, then stream live changes
    let combined = stream::once(async { initial_event }).chain(change_stream);

    Sse::new(combined)
        .keep_alive(
            KeepAlive::new()
                .interval(Duration::from_secs(30))
                .text("keep-alive"),
        )
        .into_response()
}
