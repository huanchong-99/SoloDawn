//! Shared AES-256-GCM encryption/decryption for API keys and secrets.
//!
//! Centralises the encrypt/decrypt logic that was previously duplicated
//! across `Workflow`, `Terminal`, `ModelConfig`, `ConciergeSession`,
//! `PlanningDraft`, and `FeishuAppConfig`.

use std::sync::OnceLock;

use aes_gcm::{
    Aes256Gcm, Nonce,
    aead::{Aead, AeadCore, KeyInit, OsRng},
};
use base64::{Engine as _, engine::general_purpose};

const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";
const ENCRYPTION_KEY_ENV_LEGACY: &str = "GITCORTEX_ENCRYPTION_KEY";

/// Legacy debug-fallback key. Blobs may have been written under this value by
/// the old `main.rs` development fallback (which injected it into the process
/// env when no key was configured). Kept ONLY for a one-time decrypt-retry on
/// upgrade so previously-stored secrets remain readable; never written with.
const LEGACY_DEV_FALLBACK_KEY: &str = "12345678901234567890123456789012";

/// Validate a candidate key string and convert to a 32-byte array.
fn parse_key(key_str: &str) -> anyhow::Result<[u8; 32]> {
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

/// Retrieve the 32-byte encryption key.
///
/// Precedence (DO NOT reorder — installer-written env keys must win so existing
/// encrypted blobs stay decryptable):
/// 1. `SOLODAWN_ENCRYPTION_KEY` env var.
/// 2. Deprecated `GITCORTEX_ENCRYPTION_KEY` env var (with a warning log).
/// 3. A persistent, self-provisioning per-machine key file (generate-once,
///    then reuse). This stable on-disk fallback is what makes "set the API key
///    once and it persists across runs" hold outside the installer path,
///    instead of depending on an ephemeral process-env value.
pub fn get_encryption_key() -> anyhow::Result<[u8; 32]> {
    if let Ok(val) = std::env::var(ENCRYPTION_KEY_ENV) {
        return parse_key(&val);
    }
    if let Ok(val) = std::env::var(ENCRYPTION_KEY_ENV_LEGACY) {
        tracing::warn!(
            new = ENCRYPTION_KEY_ENV,
            old = ENCRYPTION_KEY_ENV_LEGACY,
            "Deprecated env var used; please switch to the new name"
        );
        return parse_key(&val);
    }

    // Fall back to the persistent per-machine key file.
    get_or_create_file_key()
}

/// Resolve, and if necessary generate, the persistent per-machine key file.
///
/// The key is a 32-byte ASCII string stored at `utils::path::enc_key_file_path`.
/// Generation is one-time: once the file exists it is read back verbatim. On
/// Unix the file is created with mode `0o600`; on Windows it inherits the
/// per-user data directory ACL. The resolved key is cached for the process so
/// the file is read at most once.
///
/// Documented threat model: under the stated local-desktop assumption (a single
/// trusted user on their own machine), a key file at rest in the user's private
/// data directory is acceptable — an attacker with read access to that directory
/// already has access to the SQLite DB and config the key protects.
pub fn get_or_create_file_key() -> anyhow::Result<[u8; 32]> {
    static CACHED: OnceLock<[u8; 32]> = OnceLock::new();

    // Only consult/populate the process cache when resolving the default
    // data-dir path. When an explicit `SOLODAWN_ENC_KEY_FILE` override is set
    // (tests, unusual deployments) the path can change within a process, so the
    // cache must be bypassed to avoid returning a stale key.
    let override_set = std::env::var("SOLODAWN_ENC_KEY_FILE").is_ok_and(|v| !v.is_empty());
    if !override_set
        && let Some(k) = CACHED.get()
    {
        return Ok(*k);
    }

    let path = utils::path::enc_key_file_path().ok_or_else(|| {
        anyhow::anyhow!(
            "Encryption key not found and could not resolve a data directory for the \
             persistent key file. Set {ENCRYPTION_KEY_ENV} with a 32-byte value."
        )
    })?;

    let key = if path.exists() {
        let contents = std::fs::read_to_string(&path).map_err(|e| {
            anyhow::anyhow!("Failed to read encryption key file {}: {e}", path.display())
        })?;
        parse_key(contents.trim_end_matches(['\r', '\n']))?
    } else {
        use std::io::Write;

        let generated = generate_ascii_key();
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).map_err(|e| {
                anyhow::anyhow!(
                    "Failed to create directory for encryption key file {}: {e}",
                    parent.display()
                )
            })?;
        }
        // Atomic generate-once: create_new(true) fails with AlreadyExists if a
        // concurrent process won the race, so we never truncate a key another
        // process already wrote (and may have encrypted blobs under).
        match std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&path)
        {
            Ok(mut f) => {
                f.write_all(generated.as_bytes()).map_err(|e| {
                    anyhow::anyhow!(
                        "Failed to write encryption key file {}: {e}",
                        path.display()
                    )
                })?;
                drop(f);
                restrict_key_file_permissions(&path);
                tracing::warn!(
                    path = %path.display(),
                    "{ENCRYPTION_KEY_ENV} not set; generated a persistent per-machine encryption key file"
                );
                parse_key(&generated)?
            }
            Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => {
                // Lost the create race: read the winner's key so both processes converge.
                let contents = std::fs::read_to_string(&path).map_err(|e| {
                    anyhow::anyhow!("Failed to read encryption key file {}: {e}", path.display())
                })?;
                parse_key(contents.trim_end_matches(['\r', '\n']))?
            }
            Err(e) => {
                return Err(anyhow::anyhow!(
                    "Failed to write encryption key file {}: {e}",
                    path.display()
                ));
            }
        }
    };

    if !override_set {
        let _ = CACHED.set(key);
    }
    Ok(key)
}

/// Generate a 32-character ASCII key (alphanumeric) suitable for AES-256.
fn generate_ascii_key() -> String {
    use aes_gcm::aead::rand_core::RngCore;
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
    let mut bytes = [0u8; 32];
    OsRng.fill_bytes(&mut bytes);
    bytes
        .iter()
        .map(|b| ALPHABET[(*b as usize) % ALPHABET.len()] as char)
        .collect()
}

/// Best-effort restriction of the key file to the current user only.
#[cfg(unix)]
fn restrict_key_file_permissions(path: &std::path::Path) {
    use std::os::unix::fs::PermissionsExt;
    if let Err(e) = std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600)) {
        tracing::warn!(
            path = %path.display(),
            "Failed to set 0o600 permissions on encryption key file: {e}"
        );
    }
}

/// On Windows the file inherits the per-user `%APPDATA%` ACL, which is already
/// restricted to the owning user; no extra action is required.
#[cfg(not(unix))]
fn restrict_key_file_permissions(_path: &std::path::Path) {}

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

    match decrypt_with_key(&key, &combined) {
        Ok(plaintext) => Ok(plaintext),
        Err(primary_err) => {
            // Migration safety-net: blobs may have been written under the old
            // debug fallback key (`main.rs` injected LEGACY_DEV_FALLBACK_KEY
            // into the process env when no key was configured). Retry once so an
            // upgrade does not force the user to re-enter every stored secret.
            // Only attempt if the legacy key actually differs from the active
            // one, to avoid masking genuine corruption.
            let legacy = LEGACY_DEV_FALLBACK_KEY.as_bytes();
            if key.as_slice() != legacy
                && let Ok(plaintext) = decrypt_with_key(legacy, &combined)
            {
                tracing::warn!(
                    "Decrypted a blob using the legacy development fallback key; \
                     it should be re-encrypted under the current key on next write"
                );
                return Ok(plaintext);
            }
            Err(primary_err)
        }
    }
}

/// Decrypt `combined` (nonce ∥ ciphertext) with a specific 32-byte key.
fn decrypt_with_key(key: &[u8], combined: &[u8]) -> anyhow::Result<String> {
    let (nonce_bytes, ciphertext) = combined.split_at(12);
    #[allow(deprecated)]
    let nonce = Nonce::from_slice(nonce_bytes);
    let cipher = Aes256Gcm::new_from_slice(key)
        .map_err(|e| anyhow::anyhow!("Invalid encryption key: {e}"))?;

    let plaintext_bytes = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|e| anyhow::anyhow!("Decryption failed: {e}"))?;

    String::from_utf8(plaintext_bytes)
        .map_err(|e| anyhow::anyhow!("Invalid UTF-8 in decrypted data: {e}"))
}

#[cfg(test)]
mod tests {
    use serial_test::serial;

    use super::*;

    #[test]
    #[serial]
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
    #[serial]
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

    /// A blob written under the legacy debug-fallback key must still decrypt
    /// after the user switches to a different stable key (migration safety-net).
    #[test]
    #[serial]
    fn legacy_fallback_blob_still_decrypts() {
        // Encrypt under the legacy debug-fallback key.
        let encrypted = temp_env::with_vars(
            [(
                "SOLODAWN_ENCRYPTION_KEY",
                Some(LEGACY_DEV_FALLBACK_KEY),
            )],
            || encrypt("sk-legacy-secret").expect("encryption should succeed"),
        );

        // Now a different stable key is active; decrypt must retry the legacy key.
        temp_env::with_vars(
            [(
                "SOLODAWN_ENCRYPTION_KEY",
                Some("abcdefghijklmnopqrstuvwxyz012345"),
            )],
            || {
                let decrypted = decrypt(&encrypted)
                    .expect("decryption should fall back to the legacy key");
                assert_eq!(decrypted, "sk-legacy-secret");
            },
        );
    }
}
