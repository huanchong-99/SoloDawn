use axum::{
    Router,
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use server::{Deployment, DeploymentImpl};
use tower::ServiceExt;

#[tokio::test]
async fn test_terminal_ws_rejects_invalid_terminal_id() {
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = Router::new()
        .nest(
            "/api/terminal",
            server::routes::terminal_ws::terminal_ws_routes(),
        )
        .with_state(deployment);

    let request = Request::builder()
        .method("GET")
        .uri("/api/terminal/not-a-uuid")
        .header(header::CONNECTION, "upgrade")
        .header(header::UPGRADE, "websocket")
        .header("sec-websocket-version", "13")
        .header("sec-websocket-key", "dGhlIHNhbXBsZSBub25jZQ==")
        .body(Body::empty())
        .expect("Failed to build websocket request");

    let response = app
        .oneshot(request)
        .await
        .expect("Failed to call websocket endpoint");

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);

    let body = to_bytes(response.into_body(), usize::MAX)
        .await
        .expect("Failed to read response body");
    let json: serde_json::Value =
        serde_json::from_slice(&body).expect("Response should be valid JSON");
    let message = json["error"]["message"].as_str().unwrap_or_default();
    assert!(
        message.contains("Invalid terminal_id format"),
        "Unexpected error message: {message}"
    );
}
