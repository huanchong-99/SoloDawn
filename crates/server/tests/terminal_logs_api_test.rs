//! Integration tests for terminal logs API.

use axum::{
    Router,
    body::{Body, to_bytes},
    http::{Request, StatusCode},
};
use server::{Deployment, DeploymentImpl};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn test_get_terminal_logs_returns_empty_list_for_unknown_terminal() {
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
        .uri(format!("/api/terminals/{terminal_id}/logs"))
        .body(Body::empty())
        .expect("Failed to build request");

    let response = app.oneshot(request).await.expect("Failed to get response");
    assert_eq!(response.status(), StatusCode::OK);

    let body = to_bytes(response.into_body(), usize::MAX)
        .await
        .expect("Failed to read response body");
    let json: serde_json::Value =
        serde_json::from_slice(&body).expect("Response should be valid JSON");

    assert_eq!(json["success"], true);
    assert!(
        json["data"].as_array().is_some_and(std::vec::Vec::is_empty),
        "Expected empty logs for unknown terminal"
    );
}
