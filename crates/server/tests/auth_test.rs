//! Authentication Integration Tests
//!
//! Tests the authentication middleware for API endpoints.
//!
//! These tests verify that:
//! - Requests are allowed when GITCORTEX_API_TOKEN is not set (development mode)
//! - Requests are rejected when GITCORTEX_API_TOKEN is set but no auth header is provided
//! - Requests are rejected when the token doesn't match
//! - Requests are allowed when the token matches

use std::sync::{Arc, Mutex, MutexGuard};

use axum::{
    body::Body,
    http::{Request, StatusCode, header},
};
use once_cell::sync::Lazy;
use server::{
    Deployment, DeploymentImpl,
    routes::{build_router, subscription_hub::SubscriptionHub},
    feishu_handle::new_shared_handle,
};
use services::services::cli_health_monitor::{CliHealthMonitor, SharedCliHealthMonitor};
use tower::ServiceExt;

/// Mutex to serialize environment variable access across tests.
/// This ensures tests that modify environment variables run sequentially.
static ENV_LOCK: Lazy<Mutex<()>> = Lazy::new(|| Mutex::new(()));

/// Helper: Create a test subscription hub
fn create_test_hub() -> server::routes::SharedSubscriptionHub {
    Arc::new(SubscriptionHub::default())
}

/// Helper: Create a test CLI health monitor
fn create_test_cli_health_monitor() -> SharedCliHealthMonitor {
    Arc::new(CliHealthMonitor::new(0))
}

/// RAII guard for managing environment variables during tests.
///
/// IMPORTANT: The MutexGuard is held for the entire lifetime of this struct,
/// ensuring that concurrent tests cannot interfere with each other's
/// environment variable modifications.
struct EnvVarGuard {
    key: &'static str,
    prev: Option<String>,
    /// Holds the lock for the entire test duration to prevent concurrent env var access
    _lock: MutexGuard<'static, ()>,
}

impl EnvVarGuard {
    /// Set an environment variable to a value.
    /// The lock is held until this guard is dropped.
    fn set(key: &'static str, value: &str) -> Self {
        let lock = ENV_LOCK.lock().unwrap();
        let prev = std::env::var(key).ok();
        unsafe { std::env::set_var(key, value) };
        Self {
            key,
            prev,
            _lock: lock,
        }
    }

    /// Unset an environment variable.
    /// The lock is held until this guard is dropped.
    fn unset(key: &'static str) -> Self {
        let lock = ENV_LOCK.lock().unwrap();
        let prev = std::env::var(key).ok();
        unsafe { std::env::remove_var(key) };
        Self {
            key,
            prev,
            _lock: lock,
        }
    }
}

impl Drop for EnvVarGuard {
    fn drop(&mut self) {
        // Lock is already held via _lock field, no need to acquire again
        match &self.prev {
            Some(value) => unsafe { std::env::set_var(self.key, value) },
            None => unsafe { std::env::remove_var(self.key) },
        }
    }
}

#[tokio::test]
async fn test_allows_requests_when_token_unset() {
    // Unset API token to simulate development mode
    let _env = EnvVarGuard::unset("GITCORTEX_API_TOKEN");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Request without auth header should succeed
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(
        response.status(),
        StatusCode::OK,
        "Request should succeed when API token is not set"
    );
}

#[tokio::test]
async fn test_rejects_requests_without_authorization() {
    // Set API token to simulate production mode
    let _env = EnvVarGuard::set("GITCORTEX_API_TOKEN", "test-token");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Request without auth header should fail
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(
        response.status(),
        StatusCode::UNAUTHORIZED,
        "Request without auth header should be rejected"
    );
}

#[tokio::test]
async fn test_rejects_requests_with_invalid_token() {
    // Set API token
    let _env = EnvVarGuard::set("GITCORTEX_API_TOKEN", "correct-token");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Request with wrong token should fail
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/health")
                .header(header::AUTHORIZATION, "Bearer wrong-token")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(
        response.status(),
        StatusCode::UNAUTHORIZED,
        "Request with wrong token should be rejected"
    );
}

#[tokio::test]
async fn test_allows_requests_with_valid_token() {
    // Set API token
    let _env = EnvVarGuard::set("GITCORTEX_API_TOKEN", "test-token");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Request with correct token should succeed
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/health")
                .header(header::AUTHORIZATION, "Bearer test-token")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(
        response.status(),
        StatusCode::OK,
        "Request with valid token should succeed"
    );
}

#[tokio::test]
async fn test_rejects_requests_with_malformed_auth_header() {
    // Set API token
    let _env = EnvVarGuard::set("GITCORTEX_API_TOKEN", "test-token");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Request with malformed auth header (missing "Bearer" prefix) should fail
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/health")
                .header(header::AUTHORIZATION, "test-token")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(
        response.status(),
        StatusCode::UNAUTHORIZED,
        "Request with malformed auth header should be rejected"
    );
}

#[tokio::test]
async fn test_token_comparison_is_case_sensitive() {
    // Set API token with specific case
    let _env = EnvVarGuard::set("GITCORTEX_API_TOKEN", "SecretToken");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Request with different case should fail
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/health")
                .header(header::AUTHORIZATION, "Bearer secrettoken")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(
        response.status(),
        StatusCode::UNAUTHORIZED,
        "Token comparison should be case-sensitive"
    );
}

#[tokio::test]
async fn test_multiple_requests_with_same_token() {
    // Set API token
    let _env = EnvVarGuard::set("GITCORTEX_API_TOKEN", "test-token");

    // Create deployment and router
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");
    let app = build_router(deployment, create_test_hub(), new_shared_handle(), create_test_cli_health_monitor());

    // Multiple requests with valid token should all succeed
    for i in 0..3 {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri("/api/health")
                    .header(header::AUTHORIZATION, "Bearer test-token")
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();

        assert_eq!(
            response.status(),
            StatusCode::OK,
            "Request {} with valid token should succeed",
            i + 1
        );
    }
}
