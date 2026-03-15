use axum::{
    Extension, Router,
    http::{HeaderValue, Method},
    routing::{IntoMakeService, get},
};
use tower_http::cors::{AllowOrigin, Any, CorsLayer};
pub use subscription_hub::SharedSubscriptionHub;

use crate::{DeploymentImpl, feishu_handle::SharedFeishuHandle, middleware::require_api_token};

/// Build CORS layer based on environment configuration.
///
/// - If `GITCORTEX_CORS_ORIGINS` is set: only allow those origins (comma-separated).
/// - If unset or empty: allow all origins (development mode).
fn build_cors_layer() -> CorsLayer {
    let origins_env = std::env::var("GITCORTEX_CORS_ORIGINS").unwrap_or_default();
    let trimmed = origins_env.trim();

    if trimmed.is_empty() {
        tracing::debug!("GITCORTEX_CORS_ORIGINS not set; allowing all origins (development mode)");
        CorsLayer::new()
            .allow_origin(Any)
            .allow_methods(Any)
            .allow_headers(Any)
    } else {
        let origins: Vec<HeaderValue> = trimmed
            .split(',')
            .filter_map(|s| {
                let s = s.trim();
                if s.is_empty() {
                    return None;
                }
                match s.parse::<HeaderValue>() {
                    Ok(v) => Some(v),
                    Err(e) => {
                        tracing::warn!(origin = %s, error = %e, "Ignoring invalid CORS origin");
                        None
                    }
                }
            })
            .collect();

        tracing::info!(
            count = origins.len(),
            "CORS configured with restricted origins"
        );

        CorsLayer::new()
            .allow_origin(AllowOrigin::list(origins))
            .allow_methods([
                Method::GET,
                Method::POST,
                Method::PUT,
                Method::DELETE,
                Method::PATCH,
                Method::OPTIONS,
            ])
            .allow_headers(Any)
    }
}

pub mod approvals;
pub mod chat_integrations;
pub mod ci_webhook;
pub mod cli_status_sse;
pub mod cli_types;
pub mod config;
pub mod containers;
pub mod filesystem;
// pub mod github;
pub mod event_bridge;
pub mod events;
pub mod feishu;
pub mod execution_processes;
pub mod frontend;
pub mod git;
pub mod health;
pub mod images;
pub mod models;
pub mod oauth;
pub mod organizations;
pub mod planning_drafts;
pub mod projects;
pub mod provider_health;
pub mod quality;
pub mod repo;
pub mod scratch;
pub mod sessions;
pub mod shared_tasks_types;
pub mod slash_commands;
pub mod subscription_hub;
pub mod tags;
pub mod task_attempts;
pub mod tasks;
pub mod terminal_ws;
pub mod terminals;
pub mod workflow_events;
pub mod workflow_ws;
pub mod workflows;
pub mod workflows_dto;

pub fn router(deployment: DeploymentImpl, hub: SharedSubscriptionHub, feishu_handle: SharedFeishuHandle) -> IntoMakeService<Router> {
    build_router(deployment, hub, feishu_handle).into_make_service()
}

pub fn build_router(deployment: DeploymentImpl, hub: SharedSubscriptionHub, feishu_handle: SharedFeishuHandle) -> Router {
    let outer_deployment = deployment.clone();
    // G32-015: Clone before moving into base_routes so we can also attach to outer router.
    let outer_feishu_handle = feishu_handle.clone();

    // Create routers with different middleware layers
    let base_routes = Router::new()
        .route("/health", get(health::health_check))
        .merge(config::router())
        .merge(containers::router(&deployment))
        .merge(projects::router(&deployment))
        .merge(tasks::router(&deployment))
        .merge(task_attempts::router(&deployment))
        .merge(execution_processes::router(&deployment))
        .merge(tags::router(&deployment))
        .merge(oauth::router())
        .merge(organizations::router())
        .merge(filesystem::router())
        .merge(repo::router())
        .merge(events::router(&deployment))
        .merge(git::router())
        .merge(approvals::router())
        .nest("/integrations", chat_integrations::router())
        .nest("/integrations", feishu::router())
        .merge(scratch::router(&deployment))
        .merge(sessions::router(&deployment))
        .nest("/images", images::routes())
        .nest("/models", models::router())
        .nest("/cli_types", cli_types::cli_types_routes())
        // CLI status SSE: requires SharedCliHealthMonitor Extension layer
        // .nest("/cli_types", cli_status_sse::cli_status_sse_routes())
        .nest("/planning-drafts", planning_drafts::planning_draft_routes())
        .nest("/workflows", workflows::workflows_routes())
        .nest("/workflows", slash_commands::slash_commands_routes())
        .nest("/workflows", provider_health::provider_health_routes())
        .nest("/workflows", quality::quality_workflow_routes())
        .nest("/quality", quality::quality_routes())
        .nest("/ci", ci_webhook::ci_webhook_routes())
        .nest("/terminal", terminal_ws::terminal_ws_routes())
        .nest("/terminals", terminals::terminal_routes())
        .nest("/terminals", quality::quality_terminal_routes())
        // WebSocket routes for workflow events (requires Extension layer for hub)
        .nest("/ws", workflow_ws::workflow_ws_routes())
        .layer(Extension(hub))
        .layer(Extension(feishu_handle))
        .layer(axum::middleware::from_fn(require_api_token))
        // G18-005: CORS configuration. In production, set GITCORTEX_CORS_ORIGINS
        // to restrict allowed origins (comma-separated). When unset, allows all
        // origins for local development convenience.
        .layer(build_cors_layer())
        .with_state(deployment);

    Router::new()
        .route("/healthz", get(health::healthz))
        .route("/readyz", get(health::readyz))
        .route("/", get(frontend::serve_frontend_root))
        .route("/{*path}", get(frontend::serve_frontend))
        .nest("/api", base_routes)
        // G32-015: Expose FeishuHandle to outer routes (readyz) so the health
        // endpoint can query actual WebSocket connection status.
        .layer(Extension(outer_feishu_handle))
        .with_state(outer_deployment)
}
