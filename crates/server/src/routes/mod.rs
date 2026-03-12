use axum::{
    Extension, Router,
    routing::{IntoMakeService, get},
};
pub use subscription_hub::SharedSubscriptionHub;

use crate::{DeploymentImpl, middleware::require_api_token};

pub mod approvals;
pub mod chat_integrations;
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

pub fn router(deployment: DeploymentImpl, hub: SharedSubscriptionHub) -> IntoMakeService<Router> {
    build_router(deployment, hub).into_make_service()
}

pub fn build_router(deployment: DeploymentImpl, hub: SharedSubscriptionHub) -> Router {
    let outer_deployment = deployment.clone();

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
        .nest("/planning-drafts", planning_drafts::planning_draft_routes())
        .nest("/workflows", workflows::workflows_routes())
        .nest("/workflows", slash_commands::slash_commands_routes())
        .nest("/workflows", provider_health::provider_health_routes())
        .nest("/workflows", quality::quality_workflow_routes())
        .nest("/quality", quality::quality_routes())
        .nest("/terminal", terminal_ws::terminal_ws_routes())
        .nest("/terminals", terminals::terminal_routes())
        .nest("/terminals", quality::quality_terminal_routes())
        // WebSocket routes for workflow events (requires Extension layer for hub)
        .nest("/ws", workflow_ws::workflow_ws_routes())
        .layer(Extension(hub))
        .layer(axum::middleware::from_fn(require_api_token))
        .with_state(deployment);

    Router::new()
        .route("/healthz", get(health::healthz))
        .route("/readyz", get(health::readyz))
        .route("/", get(frontend::serve_frontend_root))
        .route("/{*path}", get(frontend::serve_frontend))
        .nest("/api", base_routes)
        .with_state(outer_deployment)
}
