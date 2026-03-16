//! Shared WebSocket Origin header validation.
//!
//! Validates the `Origin` header on WebSocket upgrade requests against the
//! `GITCORTEX_CORS_ORIGINS` allowlist (the same env var used for CORS config).

use axum::http::{HeaderMap, StatusCode};

/// Validate the Origin header for a WebSocket upgrade request.
///
/// - If `GITCORTEX_CORS_ORIGINS` is **not set** (dev mode): allow all origins but log a warning.
/// - If set: validate the `Origin` header against the comma-separated allowlist.
/// - Reject requests with no `Origin` header unless they originate from localhost.
///
/// Returns `Ok(())` if the origin is allowed, or `Err((StatusCode, String))` to reject.
pub fn validate_ws_origin(headers: &HeaderMap) -> Result<(), (StatusCode, String)> {
    let origins_env = std::env::var("GITCORTEX_CORS_ORIGINS").unwrap_or_default();
    let trimmed = origins_env.trim();

    // Dev mode: no allowlist configured — allow everything with a warning.
    if trimmed.is_empty() {
        tracing::warn!(
            "GITCORTEX_CORS_ORIGINS not set; WebSocket origin validation disabled (development mode)"
        );
        return Ok(());
    }

    let allowed: Vec<&str> = trimmed
        .split(',')
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .collect();

    let origin = match headers.get("origin").and_then(|v| v.to_str().ok()) {
        Some(o) => o,
        None => {
            // Allow missing Origin from localhost (e.g. non-browser clients on loopback).
            if is_localhost_request(headers) {
                tracing::debug!(
                    "Allowing WebSocket connection with no Origin header from localhost"
                );
                return Ok(());
            }
            tracing::warn!(
                "Rejected WebSocket connection: no Origin header from non-localhost source"
            );
            return Err((
                StatusCode::FORBIDDEN,
                "Forbidden: missing Origin header".to_string(),
            ));
        }
    };

    if allowed.iter().any(|a| a.eq_ignore_ascii_case(origin)) {
        Ok(())
    } else {
        tracing::warn!(
            origin = %origin,
            "Rejected WebSocket connection: origin not in GITCORTEX_CORS_ORIGINS allowlist"
        );
        Err((
            StatusCode::FORBIDDEN,
            "Forbidden: origin not allowed".to_string(),
        ))
    }
}

/// Best-effort check whether the request originates from localhost.
///
/// Inspects the `Host` header for common loopback indicators.
fn is_localhost_request(headers: &HeaderMap) -> bool {
    if let Some(host) = headers.get("host").and_then(|v| v.to_str().ok()) {
        let host_lower = host.to_ascii_lowercase();
        // Strip port if present.
        let host_part = host_lower.split(':').next().unwrap_or(&host_lower);
        return matches!(host_part, "localhost" | "127.0.0.1" | "::1" | "[::1]");
    }
    false
}

#[cfg(test)]
mod tests {
    use axum::http::{HeaderMap, HeaderValue};

    use super::*;

    /// Helper: run validation with a specific GITCORTEX_CORS_ORIGINS value.
    /// Uses a closure to ensure test isolation (env var is set/unset around the call).
    fn with_env(val: Option<&str>, f: impl FnOnce()) {
        // Safety: tests using this helper must not run in parallel if they
        // rely on the same env var. `cargo test` runs tests in the same process
        // but sequentially by default within a single module.
        match val {
            Some(v) => std::env::set_var("GITCORTEX_CORS_ORIGINS", v),
            None => std::env::remove_var("GITCORTEX_CORS_ORIGINS"),
        }
        f();
        std::env::remove_var("GITCORTEX_CORS_ORIGINS");
    }

    #[test]
    fn dev_mode_allows_any_origin() {
        with_env(None, || {
            let mut headers = HeaderMap::new();
            headers.insert(
                "origin",
                HeaderValue::from_static("http://evil.example.com"),
            );
            assert!(validate_ws_origin(&headers).is_ok());
        });
    }

    #[test]
    fn allowed_origin_passes() {
        with_env(
            Some("http://localhost:3000,https://app.example.com"),
            || {
                let mut headers = HeaderMap::new();
                headers.insert(
                    "origin",
                    HeaderValue::from_static("https://app.example.com"),
                );
                assert!(validate_ws_origin(&headers).is_ok());
            },
        );
    }

    #[test]
    fn disallowed_origin_rejected() {
        with_env(Some("https://app.example.com"), || {
            let mut headers = HeaderMap::new();
            headers.insert(
                "origin",
                HeaderValue::from_static("https://evil.example.com"),
            );
            let err = validate_ws_origin(&headers).unwrap_err();
            assert_eq!(err.0, StatusCode::FORBIDDEN);
        });
    }

    #[test]
    fn missing_origin_from_localhost_allowed() {
        with_env(Some("https://app.example.com"), || {
            let mut headers = HeaderMap::new();
            headers.insert("host", HeaderValue::from_static("localhost:8080"));
            assert!(validate_ws_origin(&headers).is_ok());
        });
    }

    #[test]
    fn missing_origin_from_remote_rejected() {
        with_env(Some("https://app.example.com"), || {
            let headers = HeaderMap::new();
            let err = validate_ws_origin(&headers).unwrap_err();
            assert_eq!(err.0, StatusCode::FORBIDDEN);
        });
    }
}
