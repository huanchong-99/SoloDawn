use axum::{
    Router,
    body::Body,
    http::{Request, StatusCode},
};
use server::{Deployment, DeploymentImpl};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn test_stop_terminal_returns_not_found_for_unknown_terminal() {
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = Router::new()
        .nest(
            "/api/terminals",
            server::routes::terminals::terminal_routes(),
        )
        .with_state(deployment);

    let terminal_id = Uuid::new_v4().to_string();
    let request = Request::builder()
        .method("POST")
        .uri(format!("/api/terminals/{terminal_id}/stop"))
        .body(Body::empty())
        .expect("Failed to build request");

    let response = app.oneshot(request).await.expect("Failed to stop terminal");
    assert_eq!(response.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn test_stop_terminal_rejects_get_method() {
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = Router::new()
        .nest(
            "/api/terminals",
            server::routes::terminals::terminal_routes(),
        )
        .with_state(deployment);

    let terminal_id = Uuid::new_v4().to_string();
    let request = Request::builder()
        .method("GET")
        .uri(format!("/api/terminals/{terminal_id}/stop"))
        .body(Body::empty())
        .expect("Failed to build request");

    let response = app
        .oneshot(request)
        .await
        .expect("Failed to query endpoint");
    assert_eq!(response.status(), StatusCode::METHOD_NOT_ALLOWED);
}
