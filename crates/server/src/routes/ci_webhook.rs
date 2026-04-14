use axum::{Router, routing::post, http::StatusCode, response::Json};
use axum::http::HeaderMap;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

/// Payload sent by the ci-notify.yml GitHub Actions workflow.
#[derive(Debug, Deserialize, Serialize)]
pub struct CiWebhookPayload {
    pub workflow: String,
    pub conclusion: String,
    pub sha: String,
    pub branch: String,
    pub run_id: u64,
    pub run_url: String,
}

/// POST /api/ci/webhook
///
/// Accepts CI workflow completion notifications from GitHub Actions.
///
/// G35-009 / W2-18-10: Requires `SOLODAWN_CI_WEBHOOK_SECRET` to be set and
/// validates the `X-Webhook-Signature` header (HMAC-SHA256) before accepting
/// payloads. When the secret is unset or empty, the endpoint refuses the
/// request with 401 — the webhook lives in the unauthenticated zone, so
/// accepting unsigned payloads would let anonymous clients push arbitrary
/// CI-status events into the system.
pub async fn ci_webhook(
    headers: HeaderMap,
    body: axum::body::Bytes,
) -> (StatusCode, Json<Value>) {
    // W2-18-10: Require a configured webhook secret. Previously the handler
    // silently accepted unsigned payloads when the env var was unset
    // ("development mode"); that behavior is unsafe because the route is
    // unauthenticated and exposed on the same surface as production.
    let secret = match utils::env_compat::var_with_compat(
        "SOLODAWN_CI_WEBHOOK_SECRET",
        "GITCORTEX_CI_WEBHOOK_SECRET",
    ) {
        Ok(s) if !s.trim().is_empty() => s.trim().to_string(),
        _ => {
            tracing::warn!(
                "CI webhook rejected: SOLODAWN_CI_WEBHOOK_SECRET is not configured"
            );
            return (
                StatusCode::UNAUTHORIZED,
                Json(json!({
                    "status": "rejected",
                    "message": "CI webhook is not configured on this server"
                })),
            );
        }
    };

    let signature = headers
        .get("x-webhook-signature")
        .and_then(|v| v.to_str().ok());

    match signature {
        Some(sig) => {
            if !verify_hmac_sha256(secret.as_bytes(), &body, sig) {
                tracing::warn!("CI webhook rejected: invalid HMAC signature");
                return (
                    StatusCode::UNAUTHORIZED,
                    Json(json!({
                        "status": "rejected",
                        "message": "Invalid webhook signature"
                    })),
                );
            }
        }
        None => {
            tracing::warn!("CI webhook rejected: missing X-Webhook-Signature header");
            return (
                StatusCode::UNAUTHORIZED,
                Json(json!({
                    "status": "rejected",
                    "message": "Missing X-Webhook-Signature header"
                })),
            );
        }
    }

    // Parse payload after signature validation
    let payload: CiWebhookPayload = match serde_json::from_slice(&body) {
        Ok(p) => p,
        Err(e) => {
            tracing::warn!(error = %e, "CI webhook rejected: invalid payload");
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({
                    "status": "rejected",
                    "message": format!("Invalid payload: {e}")
                })),
            );
        }
    };

    tracing::info!(
        workflow = %payload.workflow,
        conclusion = %payload.conclusion,
        sha = %payload.sha,
        branch = %payload.branch,
        run_id = %payload.run_id,
        "CI webhook received"
    );

    (StatusCode::ACCEPTED, Json(json!({
        "status": "accepted",
        "message": "CI webhook notification received"
    })))
}

/// Verify HMAC-SHA256 signature using the standard construction.
///
/// Expected signature format: "sha256=<hex-encoded-hmac>" or just "<hex-encoded-hmac>"
///
/// HMAC(K, m) = H((K' ^ opad) || H((K' ^ ipad) || m))
/// where K' is the key padded/hashed to block size.
fn verify_hmac_sha256(secret: &[u8], body: &[u8], signature: &str) -> bool {
    let expected_hex = signature.strip_prefix("sha256=").unwrap_or(signature);

    // Decode hex signature
    let expected_bytes = match decode_hex(expected_hex) {
        Some(bytes) => bytes,
        None => return false,
    };

    let computed = compute_hmac_sha256(secret, body);

    // Constant-time comparison
    if computed.len() != expected_bytes.len() {
        return false;
    }
    let mut diff = 0u8;
    for (a, b) in computed.iter().zip(expected_bytes.iter()) {
        diff |= a ^ b;
    }
    diff == 0
}

/// Compute HMAC-SHA256 using the standard construction.
fn compute_hmac_sha256(key: &[u8], message: &[u8]) -> Vec<u8> {
    const BLOCK_SIZE: usize = 64; // SHA-256 block size

    // If key is longer than block size, hash it first
    let key = if key.len() > BLOCK_SIZE {
        let mut hasher = Sha256::new();
        hasher.update(key);
        hasher.finalize().to_vec()
    } else {
        key.to_vec()
    };

    // Pad key to block size
    let mut key_padded = [0u8; BLOCK_SIZE];
    key_padded[..key.len()].copy_from_slice(&key);

    // Inner padding
    let mut ipad = [0x36u8; BLOCK_SIZE];
    for (i, b) in key_padded.iter().enumerate() {
        ipad[i] ^= b;
    }

    // Outer padding
    let mut opad = [0x5cu8; BLOCK_SIZE];
    for (i, b) in key_padded.iter().enumerate() {
        opad[i] ^= b;
    }

    // Inner hash: H(ipad || message)
    let mut inner_hasher = Sha256::new();
    inner_hasher.update(ipad);
    inner_hasher.update(message);
    let inner_hash = inner_hasher.finalize();

    // Outer hash: H(opad || inner_hash)
    let mut outer_hasher = Sha256::new();
    outer_hasher.update(opad);
    outer_hasher.update(inner_hash);
    outer_hasher.finalize().to_vec()
}

/// Simple hex decoder (avoids adding `hex` crate dependency).
fn decode_hex(hex: &str) -> Option<Vec<u8>> {
    let hex = hex.trim();
    if hex.len() % 2 != 0 {
        return None;
    }
    let mut bytes = Vec::with_capacity(hex.len() / 2);
    for chunk in hex.as_bytes().chunks(2) {
        let high = hex_nibble(chunk[0])?;
        let low = hex_nibble(chunk[1])?;
        bytes.push((high << 4) | low);
    }
    Some(bytes)
}

fn hex_nibble(b: u8) -> Option<u8> {
    match b {
        b'0'..=b'9' => Some(b - b'0'),
        b'a'..=b'f' => Some(b - b'a' + 10),
        b'A'..=b'F' => Some(b - b'A' + 10),
        _ => None,
    }
}

pub fn ci_webhook_routes<S: Clone + Send + Sync + 'static>() -> Router<S> {
    Router::new()
        .route("/webhook", post(ci_webhook))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn bytes_to_hex(bytes: &[u8]) -> String {
        use std::fmt::Write;
        bytes.iter().fold(String::with_capacity(bytes.len() * 2), |mut s, b| {
            let _ = write!(s, "{b:02x}");
            s
        })
    }

    #[test]
    fn test_hmac_sha256_known_vector() {
        // RFC 4231 Test Case 2
        let key = b"Jefe";
        let data = b"what do ya want for nothing?";
        let expected = "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843";

        let result = compute_hmac_sha256(key, data);
        let result_hex = bytes_to_hex(&result);
        assert_eq!(result_hex, expected);
    }

    #[test]
    fn test_verify_hmac_with_prefix() {
        let key = b"test-secret";
        let body = b"test body";
        let mac = compute_hmac_sha256(key, body);
        let hex_sig = bytes_to_hex(&mac);

        assert!(verify_hmac_sha256(key, body, &format!("sha256={hex_sig}")));
        assert!(verify_hmac_sha256(key, body, &hex_sig));
        assert!(!verify_hmac_sha256(key, body, "sha256=0000000000000000000000000000000000000000000000000000000000000000"));
    }

    #[test]
    fn test_decode_hex_valid() {
        assert_eq!(decode_hex("48656c6c6f"), Some(b"Hello".to_vec()));
    }

    #[test]
    fn test_decode_hex_invalid() {
        assert_eq!(decode_hex("xyz"), None);
        assert_eq!(decode_hex("4g"), None);
    }
}
