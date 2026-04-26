//! Shared AES-256-GCM encryption/decryption for API keys and secrets.
//!
//! Centralises the encrypt/decrypt logic that was previously duplicated
//! across `Workflow`, `Terminal`, `ModelConfig`, `ConciergeSession`,
//! `PlanningDraft`, and `FeishuAppConfig`.

use aes_gcm::{
    Aes256Gcm, Nonce,
    aead::{Aead, AeadCore, KeyInit, OsRng},
};
use base64::{Engine as _, engine::general_purpose};

const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";
const ENCRYPTION_KEY_ENV_LEGACY: &str = "GITCORTEX_ENCRYPTION_KEY";

/// Retrieve the 32-byte encryption key from the environment.
///
/// Tries `SOLODAWN_ENCRYPTION_KEY` first, then falls back to the
/// deprecated `GITCORTEX_ENCRYPTION_KEY` with a warning log.
pub fn get_encryption_key() -> anyhow::Result<[u8; 32]> {
    let key_str = std::env::var(ENCRYPTION_KEY_ENV)
        .or_else(|_| {
            let val = std::env::var(ENCRYPTION_KEY_ENV_LEGACY)?;
            tracing::warn!(
                new = ENCRYPTION_KEY_ENV,
                old = ENCRYPTION_KEY_ENV_LEGACY,
                "Deprecated env var used; please switch to the new name"
            );
            Ok(val)
        })
        .map_err(|_: std::env::VarError| {
            anyhow::anyhow!(
                "Encryption key not found. Please set {ENCRYPTION_KEY_ENV} environment variable with a 32-byte value."
            )
        })?;

    // Check length FIRST before conversion to prevent zero-padding
    if key_str.len() != 32 {
        return Err(anyhow::anyhow!(
            "Invalid encryption key length: got {} bytes, expected exactly 32 bytes",
            key_str.len()
        ));
    }

    key_str
        .as_bytes()
        .try_into()
        .map_err(|_| anyhow::anyhow!("Invalid encryption key format"))
}

/// Encrypt a plaintext string using AES-256-GCM.
///
/// Returns a base64-encoded string containing the 12-byte nonce
/// followed by the ciphertext.
pub fn encrypt(plaintext: &str) -> anyhow::Result<String> {
    let key = get_encryption_key()?;
    let cipher = Aes256Gcm::new_from_slice(&key)
        .map_err(|e| anyhow::anyhow!("Invalid encryption key: {e}"))?;
    let nonce = Aes256Gcm::generate_nonce(&mut OsRng);

    let ciphertext = cipher
        .encrypt(&nonce, plaintext.as_bytes())
        .map_err(|e| anyhow::anyhow!("Encryption failed: {e}"))?;

    let mut combined = nonce.to_vec();
    combined.extend_from_slice(&ciphertext);
    Ok(general_purpose::STANDARD.encode(&combined))
}

/// Decrypt a base64-encoded AES-256-GCM ciphertext (nonce ∥ ciphertext).
pub fn decrypt(encoded: &str) -> anyhow::Result<String> {
    let key = get_encryption_key()?;
    let combined = general_purpose::STANDARD
        .decode(encoded)
        .map_err(|e| anyhow::anyhow!("Base64 decode failed: {e}"))?;

    if combined.len() < 12 {
        return Err(anyhow::anyhow!("Invalid encrypted data length"));
    }

    let (nonce_bytes, ciphertext) = combined.split_at(12);
    #[allow(deprecated)]
    let nonce = Nonce::from_slice(nonce_bytes);
    let cipher = Aes256Gcm::new_from_slice(&key)
        .map_err(|e| anyhow::anyhow!("Invalid encryption key: {e}"))?;

    let plaintext_bytes = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|e| anyhow::anyhow!("Decryption failed: {e}"))?;

    String::from_utf8(plaintext_bytes)
        .map_err(|e| anyhow::anyhow!("Invalid UTF-8 in decrypted data: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trip() {
        temp_env::with_vars(
            [(
                "SOLODAWN_ENCRYPTION_KEY",
                Some("12345678901234567890123456789012"),
            )],
            || {
                let original = "sk-test-secret-key-12345";
                let encrypted = encrypt(original).expect("encryption should succeed");
                let decrypted = decrypt(&encrypted).expect("decryption should succeed");
                assert_eq!(original, decrypted);
            },
        );
    }

    #[test]
    fn rejects_short_key() {
        temp_env::with_vars(
            [
                ("SOLODAWN_ENCRYPTION_KEY", Some("too-short")),
                ("GITCORTEX_ENCRYPTION_KEY", None::<&str>),
            ],
            || {
                let result = get_encryption_key();
                assert!(result.is_err());
                let msg = result.unwrap_err().to_string();
                assert!(msg.contains("Invalid encryption key length"));
            },
        );
    }
}
