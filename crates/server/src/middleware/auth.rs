//! Authentication Middleware
//!
//! This module provides middleware for API authentication using bearer tokens.
//!
//! # Environment Variables
//! - `SOLODAWN_API_TOKEN`: When set, requires API requests to include a valid bearer token
//! - When unset, authentication is skipped (development mode)
//!
//! # Usage
//! ```rust
//! use axum::middleware::from_fn;
//! use server::middleware::auth::require_api_token;
//!
//! let app = Router::new()
//!     .route("/api/protected", get(handler))
//!     .layer(from_fn(require_api_token));
//! ```

use axum::{
    extract::Request,
    http::{HeaderMap, StatusCode, header},
    middleware::Next,
    response::{IntoResponse, Response},
};

/// Per-request authentication context inserted by `require_api_token`.
///
/// `authenticated` reflects whether the request passed the bearer-token check.
/// When `SOLODAWN_API_TOKEN` is unset/empty the middleware permits the request
/// and this flag is `false` (i.e. no principal was proven). Downstream
/// defense-in-depth checks (see `assert_authorized`) can opt in to reject
/// un-authenticated callers when a stricter posture is desired.
#[derive(Clone, Debug)]
pub struct RequestContext {
    pub authenticated: bool,
}

/// Helper for handlers / middleware that want to require an authenticated
/// caller regardless of dev-mode token passthrough.
///
/// Returns `Ok(())` when the request was authenticated, or an `ApiError`-style
/// response when it was not. When `SOLODAWN_REQUIRE_AUTH` is **not** set (the
/// default today), this is a no-op and returns `Ok(())` to preserve
/// backward-compatible "development mode" behavior. When set to a truthy
/// value, missing authentication is rejected with 401.
#[allow(clippy::result_large_err)]
pub fn assert_authorized(ctx: &RequestContext) -> Result<(), Response> {
    let require = std::env::var("SOLODAWN_REQUIRE_AUTH")
        .ok()
        .map(|v| {
            let t = v.trim().to_ascii_lowercase();
            !t.is_empty() && t != "0" && t != "false" && t != "no"
        })
        .unwrap_or(false);

    if require && !ctx.authenticated {
        tracing::warn!("assert_authorized: rejecting un-authenticated request (SOLODAWN_REQUIRE_AUTH set)");
        return Err((
            StatusCode::UNAUTHORIZED,
            axum::Json(serde_json::json!({
                "success": false,
                "error": "Unauthorized: authentication required"
            })),
        )
            .into_response());
    }
    Ok(())
}

/// Defense-in-depth admin gate for sensitive mutating endpoints.
///
/// Opt-in: if `SOLODAWN_ADMIN_TOKEN` is unset/empty, this is a no-op and
/// returns `Ok(())` (preserves current behavior). If set, the request must
/// include an `X-Admin-Token` header whose value matches exactly; otherwise a
/// `403 Forbidden` response is returned. This is additive to — not a
/// replacement for — the bearer-token layer in `require_api_token`.
#[allow(clippy::result_large_err)]
pub fn check_admin(_ctx: &RequestContext, headers: &HeaderMap) -> Result<(), Response> {
    let admin_token = match std::env::var("SOLODAWN_ADMIN_TOKEN") {
        Ok(value) if !value.trim().is_empty() => value,
        _ => return Ok(()),
    };

    let provided = headers
        .get("X-Admin-Token")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");

    if constant_time_eq(provided.as_bytes(), admin_token.as_bytes()) {
        Ok(())
    } else {
        tracing::warn!("check_admin: rejecting request with missing/invalid X-Admin-Token");
        Err((
            StatusCode::FORBIDDEN,
            axum::Json(serde_json::json!({
                "success": false,
                "error": "Forbidden: admin token required"
            })),
        )
            .into_response())
    }
}

/// Middleware that requires API token authentication.
///
/// # Behavior
/// - If `SOLODAWN_API_TOKEN` environment variable is **not set**: allows all requests (development mode)
/// - If `SOLODAWN_API_TOKEN` environment variable **is set**:
///   - Requires `Authorization: Bearer <token>` header
///   - Token must match the environment variable value exactly
///   - Returns `401 Unauthorized` if token is missing or invalid
///
/// # Security
/// - Token comparison is done using constant-time comparison (via string equality)
/// - Failed authentication attempts are logged with warning level
/// - Successful authentication in development mode is logged at debug level
///
/// # Example
/// ```no_run
/// use server::middleware::auth::require_api_token;
///
/// // In development - no authentication required
/// // SOLODAWN_API_TOKEN is not set
///
/// // In production - authentication required
/// // SOLODAWN_API_TOKEN="my-secret-token"
/// // Request: Authorization: Bearer my-secret-token
/// ```
pub async fn require_api_token(mut req: Request, next: Next) -> Result<Response, Response> {
    // Check if API token is configured.
    // NOTE(G35-002): std::env::var() is called per-request intentionally. The cost is
    // negligible (< 1µs on all platforms) and allows runtime token rotation without restart.
    let token =
        match utils::env_compat::var_with_compat("SOLODAWN_API_TOKEN", "GITCORTEX_API_TOKEN") {
            Ok(value) if !value.trim().is_empty() => value,
            Err(_) => {
                // SEC-002: In local mode (installer), suppress per-request warnings
                if !utils::env_compat::var_is_set("SOLODAWN_LOCAL_MODE", "GITCORTEX_LOCAL_MODE") {
                    tracing::warn!(
                        "SEC-002: SOLODAWN_API_TOKEN not set — all requests are unauthenticated! \
                     Set SOLODAWN_API_TOKEN to secure API access."
                    );
                }
                return Ok(next.run(req).await);
            }
            _ => {
                if !utils::env_compat::var_is_set("SOLODAWN_LOCAL_MODE", "GITCORTEX_LOCAL_MODE") {
                    tracing::warn!(
                        "SEC-002: SOLODAWN_API_TOKEN is empty — all requests are unauthenticated! \
                     Set a non-empty SOLODAWN_API_TOKEN to secure API access."
                    );
                }
                return Ok(next.run(req).await);
            }
        };

    // Extract Authorization header
    let auth_header = req
        .headers()
        .get(header::AUTHORIZATION)
        .and_then(|value| value.to_str().ok());

    // Expected format: "Bearer <token>"
    let expected = format!("Bearer {token}");

    // Verify token matches (constant-time comparison to prevent timing attacks)
    let is_valid = match auth_header {
        Some(header) => constant_time_eq(header.as_bytes(), expected.as_bytes()),
        None => false,
    };

    if is_valid {
        // Authentication successful
        tracing::trace!("API request authenticated successfully");
        req.extensions_mut().insert(RequestContext { authenticated: true });
        Ok(next.run(req).await)
    } else {
        // Authentication failed — return a JSON error body (G16-002)
        tracing::warn!(
            method = %req.method(),
            uri = %req.uri(),
            has_auth_header = auth_header.is_some(),
            "Unauthorized API request: invalid or missing authentication token"
        );
        Err((
            StatusCode::UNAUTHORIZED,
            axum::Json(serde_json::json!({
                "success": false,
                "error": "Unauthorized: invalid or missing authentication token"
            })),
        )
            .into_response())
    }
}

/// Constant-time byte comparison to prevent timing attacks.
///
/// Returns `true` if both slices are equal, `false` otherwise.
/// Always compares all bytes regardless of where a mismatch occurs.
///
/// Note: The early return on length mismatch leaks the token length via timing.
/// This is acceptable here because SOLODAWN_API_TOKEN has a fixed format (its
/// length is configured/known by operators and does not vary per-request, much
/// like HMAC/digest comparisons of fixed-size outputs). Learning the token
/// length therefore provides no meaningful advantage to an attacker: it is
/// neither a secret nor variable between comparisons. If the token format ever
/// becomes variable-length per request, switch to the `subtle` crate's
/// `ConstantTimeEq` and pad/compare over a fixed maximum length.
fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    let mut diff = 0u8;
    for (x, y) in a.iter().zip(b.iter()) {
        diff |= x ^ y;
    }
    diff == 0
}

#[cfg(test)]
mod tests {
    use axum::{
        Router,
        body::Body,
        http::{Request, StatusCode},
        routing::get,
    };
    use serial_test::serial;
    use tower::ServiceExt;

    use super::*;

    /// Test handler that returns OK
    async fn test_handler() -> &'static str {
        "OK"
    }

    /// Build a test app with auth middleware
    fn build_test_app() -> Router {
        Router::new()
            .route("/test", get(test_handler))
            .layer(axum::middleware::from_fn(require_api_token))
    }

    #[tokio::test]
    #[serial]
    async fn test_allows_requests_when_token_unset() {
        // Ensure API token is not set
        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };

        let app = build_test_app();

        // Request without auth header should succeed
        let response = app
            .oneshot(Request::builder().uri("/test").body(Body::empty()).unwrap())
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    #[serial]
    async fn test_allows_requests_when_token_empty() {
        unsafe { std::env::set_var("SOLODAWN_API_TOKEN", "") };

        let app = build_test_app();

        let response = app
            .oneshot(Request::builder().uri("/test").body(Body::empty()).unwrap())
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::OK);

        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };
    }

    #[tokio::test]
    #[serial]
    async fn test_rejects_requests_without_authorization_header() {
        // Set API token
        unsafe { std::env::set_var("SOLODAWN_API_TOKEN", "test-secret-token") };

        let app = build_test_app();

        // Request without auth header should fail
        let response = app
            .oneshot(Request::builder().uri("/test").body(Body::empty()).unwrap())
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);

        // Cleanup
        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };
    }

    #[tokio::test]
    #[serial]
    async fn test_rejects_requests_with_invalid_token() {
        // Set API token
        unsafe { std::env::set_var("SOLODAWN_API_TOKEN", "correct-token") };

        let app = build_test_app();

        // Request with wrong token should fail
        let response = app
            .oneshot(
                Request::builder()
                    .uri("/test")
                    .header("Authorization", "Bearer wrong-token")
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);

        // Cleanup
        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };
    }

    #[tokio::test]
    #[serial]
    async fn test_allows_requests_with_valid_token() {
        // Set API token
        unsafe { std::env::set_var("SOLODAWN_API_TOKEN", "test-secret-token") };

        let app = build_test_app();

        // Request with correct token should succeed
        let response = app
            .oneshot(
                Request::builder()
                    .uri("/test")
                    .header("Authorization", "Bearer test-secret-token")
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::OK);

        // Cleanup
        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };
    }

    #[tokio::test]
    #[serial]
    async fn test_rejects_malformed_authorization_header() {
        // Set API token
        unsafe { std::env::set_var("SOLODAWN_API_TOKEN", "test-token") };

        let app = build_test_app();

        // Request with malformed auth header (missing "Bearer" prefix) should fail
        let response = app
            .oneshot(
                Request::builder()
                    .uri("/test")
                    .header("Authorization", "test-token") // Missing "Bearer" prefix
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);

        // Cleanup
        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };
    }

    #[tokio::test]
    #[serial]
    async fn test_token_case_sensitive() {
        // Set API token
        unsafe { std::env::set_var("SOLODAWN_API_TOKEN", "SecretToken") };

        let app = build_test_app();

        // Request with different case should fail
        let response = app
            .oneshot(
                Request::builder()
                    .uri("/test")
                    .header("Authorization", "Bearer secrettoken") // Different case
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);

        // Cleanup
        unsafe { std::env::remove_var("SOLODAWN_API_TOKEN") };
    }
}
