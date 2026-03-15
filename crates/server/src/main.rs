#![warn(clippy::pedantic)]
#![allow(
    clippy::doc_markdown,
    clippy::module_name_repetitions,
    clippy::must_use_candidate,
    clippy::missing_errors_doc,
    clippy::missing_panics_doc,
    clippy::similar_names,
    clippy::too_many_lines
)]

use std::sync::Arc;

use anyhow::{self, Error as AnyhowError};
use deployment::{Deployment, DeploymentError};
use server::{
    DeploymentImpl, routes,
    feishu_handle::{FeishuHandle, SharedFeishuHandle},
    routes::{SharedSubscriptionHub, event_bridge::EventBridge, subscription_hub::SubscriptionHub},
};
use services::services::container::ContainerService;
use sqlx::Error as SqlxError;
use strip_ansi_escapes::strip;
use thiserror::Error;
use tracing_subscriber::{EnvFilter, prelude::*};
use utils::{
    assets::asset_dir,
    browser::open_browser,
    port_file::write_port_file,
    sentry::{self as sentry_utils, SentrySource, sentry_layer},
};

const DEV_DEFAULT_ENCRYPTION_KEY: &str = "12345678901234567890123456789012";

fn ensure_dev_encryption_key() {
    if !cfg!(debug_assertions) {
        // G18-003: In release mode, require a valid encryption key — do not silently proceed
        match std::env::var("GITCORTEX_ENCRYPTION_KEY") {
            Ok(value) if value.len() == 32 => {}
            Ok(value) => {
                panic!(
                    "GITCORTEX_ENCRYPTION_KEY has invalid length {} (expected 32 bytes). \
                     Set a valid 32-byte key before starting in release mode.",
                    value.len()
                );
            }
            Err(std::env::VarError::NotPresent) => {
                panic!(
                    "GITCORTEX_ENCRYPTION_KEY is not set. \
                     A 32-byte encryption key is required in release mode."
                );
            }
            Err(e) => {
                panic!(
                    "GITCORTEX_ENCRYPTION_KEY is not valid: {e}. \
                     A 32-byte encryption key is required in release mode."
                );
            }
        }
        return;
    }

    match std::env::var("GITCORTEX_ENCRYPTION_KEY") {
        Ok(value) if value.len() == 32 => {}
        Ok(value) => {
            tracing::warn!(
                provided_length = value.len(),
                "GITCORTEX_ENCRYPTION_KEY is set but length is invalid; workflow start may fail"
            );
        }
        Err(_) => {
            unsafe {
                std::env::set_var("GITCORTEX_ENCRYPTION_KEY", DEV_DEFAULT_ENCRYPTION_KEY);
            }
            tracing::warn!(
                "GITCORTEX_ENCRYPTION_KEY not set; using development fallback key"
            );
        }
    }
}

#[derive(Debug, Error)]
pub enum GitCortexError {
    #[error(transparent)]
    Io(#[from] std::io::Error),
    #[error(transparent)]
    Sqlx(#[from] SqlxError),
    #[error(transparent)]
    Deployment(#[from] DeploymentError),
    #[error(transparent)]
    Other(#[from] AnyhowError),
}

#[tokio::main]
async fn main() -> Result<(), GitCortexError> {
    // Load environment variables from .env file
    dotenv::dotenv().ok();
    ensure_dev_encryption_key();

    // Install rustls crypto provider before any TLS operations
    rustls::crypto::aws_lc_rs::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");

    sentry_utils::init_once(SentrySource::Backend);

    let log_level = std::env::var("RUST_LOG").unwrap_or_else(|_| "info".to_string());
    let filter_string = format!(
        "warn,server={log_level},services={log_level},db={log_level},executors={log_level},deployment={log_level},local_deployment={log_level},utils={log_level}"
    );
    let env_filter = EnvFilter::try_new(filter_string).expect("Failed to create tracing filter");
    tracing_subscriber::registry()
        .with(tracing_subscriber::fmt::layer().with_filter(env_filter))
        .with(sentry_layer())
        .init();

    // Create asset directory if it doesn't exist
    let _asset_dir = asset_dir()?;

    let deployment = DeploymentImpl::new().await?;
    deployment.update_sentry_scope().await?;
    deployment
        .container()
        .cleanup_orphan_executions()
        .await
        .map_err(DeploymentError::from)?;
    deployment
        .container()
        .backfill_before_head_commits()
        .await
        .map_err(DeploymentError::from)?;
    deployment
        .container()
        .backfill_repo_names()
        .await
        .map_err(DeploymentError::from)?;
    deployment.spawn_pr_monitor_service();
    deployment
        .track_if_analytics_allowed("session_start", serde_json::json!({}))
        .await;
    // Pre-warm file search cache for most active projects
    let deployment_for_cache = deployment.clone();
    tokio::spawn(async move {
        if let Err(e) = deployment_for_cache
            .file_search_cache()
            .warm_most_active(&deployment_for_cache.db().pool, 3)
            .await
        {
            tracing::warn!("Failed to warm file search cache: {}", e);
        }
    });

    // Initialize WebSocket subscription hub and event bridge
    let subscription_hub: SharedSubscriptionHub = Arc::new(SubscriptionHub::default());
    let event_bridge = EventBridge::new(deployment.message_bus().clone(), subscription_hub.clone());
    let _event_bridge_handle = event_bridge.spawn();
    tracing::info!("WebSocket event bridge started");

    // Conditional Feishu connector startup
    let feishu_handle = server::feishu_handle::new_shared_handle();
    if is_feishu_enabled() {
        match start_feishu_connector(&deployment, &feishu_handle).await {
            Ok(()) => tracing::info!("Feishu connector started"),
            Err(e) => tracing::warn!("Feishu connector startup skipped: {e}"),
        }
    } else {
        tracing::debug!("Feishu integration disabled (GITCORTEX_FEISHU_ENABLED not set)");
    }

    let cli_health_monitor = deployment.cli_health_monitor().clone();
    let app_router = routes::router(deployment.clone(), subscription_hub, feishu_handle, cli_health_monitor);

    let port = std::env::var("BACKEND_PORT")
        .or_else(|_| std::env::var("PORT"))
        .ok()
        .and_then(|s| {
            // remove any ANSI codes, then turn into String
            let cleaned =
                String::from_utf8(strip(s.as_bytes())).expect("UTF-8 after stripping ANSI");
            cleaned.trim().parse::<u16>().ok()
        })
        .unwrap_or_else(|| {
            tracing::info!("No PORT environment variable set, using default port 23456");
            23456
        }); // Default port: 23456 (chosen to avoid common dev ports and system ranges)

    let host = std::env::var("HOST").unwrap_or_else(|_| {
        if std::path::Path::new("/.dockerenv").exists() {
            "0.0.0.0".to_string()
        } else {
            "127.0.0.1".to_string()
        }
    });
    let listener = tokio::net::TcpListener::bind(format!("{host}:{port}")).await?;
    let actual_port = listener.local_addr()?.port(); // get → 53427 (example)

    // Write port file for discovery if prod, warn on fail
    if let Err(e) = write_port_file(actual_port).await {
        tracing::warn!("Failed to write port file: {}", e);
    }

    tracing::info!("Server running on http://{host}:{actual_port}");

    if !cfg!(debug_assertions) && !std::path::Path::new("/.dockerenv").exists() {
        tracing::info!("Opening browser...");
        tokio::spawn(async move {
            if let Err(e) = open_browser(&format!("http://127.0.0.1:{actual_port}")) {
                tracing::warn!(
                    "Failed to open browser automatically: {}. Please open http://127.0.0.1:{} manually.",
                    e,
                    actual_port
                );
            }
        });
    }

    axum::serve(listener, app_router)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    perform_cleanup_actions(&deployment).await;

    Ok(())
}

pub async fn shutdown_signal() {
    // Always wait for Ctrl+C
    let ctrl_c = async {
        if let Err(e) = tokio::signal::ctrl_c().await {
            tracing::error!("Failed to install Ctrl+C handler: {e}");
        }
    };

    #[cfg(unix)]
    {
        use tokio::signal::unix::{SignalKind, signal};

        // Try to install SIGTERM handler, but don't panic if it fails
        let terminate = async {
            if let Ok(mut sigterm) = signal(SignalKind::terminate()) {
                sigterm.recv().await;
            } else {
                tracing::error!("Failed to install SIGTERM handler");
                // Fallback: never resolves
                std::future::pending::<()>().await;
            }
        };

        tokio::select! {
            () = ctrl_c => {},
            () = terminate => {},
        }
    }

    #[cfg(not(unix))]
    {
        // Only ctrl_c is available, so just await it
        ctrl_c.await;
    }
}

pub async fn perform_cleanup_actions(deployment: &DeploymentImpl) {
    deployment
        .container()
        .kill_all_running_processes()
        .await
        .expect("Failed to cleanly kill running execution processes");
}

/// Check whether the Feishu integration feature flag is enabled.
fn is_feishu_enabled() -> bool {
    std::env::var("GITCORTEX_FEISHU_ENABLED")
        .ok()
        .is_some_and(|v| v.trim().eq_ignore_ascii_case("true") || v.trim() == "1")
}

/// Decrypt an AES-256-GCM encrypted secret stored as base64 (nonce || ciphertext).
fn decrypt_feishu_secret(encrypted: &str) -> anyhow::Result<String> {
    use aes_gcm::{
        Aes256Gcm, Nonce,
        aead::{Aead, KeyInit},
    };
    use base64::{Engine as _, engine::general_purpose};

    let key_str = std::env::var("GITCORTEX_ENCRYPTION_KEY")
        .map_err(|_| anyhow::anyhow!("GITCORTEX_ENCRYPTION_KEY not set"))?;
    if key_str.len() != 32 {
        return Err(anyhow::anyhow!("Invalid encryption key length"));
    }
    let key_bytes: [u8; 32] = key_str
        .as_bytes()
        .try_into()
        .map_err(|_| anyhow::anyhow!("Invalid encryption key format"))?;

    let combined = general_purpose::STANDARD
        .decode(encrypted)
        .map_err(|e| anyhow::anyhow!("Base64 decode failed: {e}"))?;
    if combined.len() < 12 {
        return Err(anyhow::anyhow!("Invalid encrypted data length"));
    }
    let (nonce_bytes, ciphertext) = combined.split_at(12);
    #[allow(deprecated)]
    let nonce = Nonce::from_slice(nonce_bytes);
    let cipher = Aes256Gcm::new_from_slice(&key_bytes)
        .map_err(|e| anyhow::anyhow!("Cipher init failed: {e}"))?;
    let plaintext = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|e| anyhow::anyhow!("Decryption failed: {e}"))?;
    String::from_utf8(plaintext).map_err(|e| anyhow::anyhow!("Invalid UTF-8: {e}"))
}

/// Attempt to start the Feishu connector by loading config from the database.
///
/// Creates a `FeishuService` from the enabled DB config, spawns a reconnect
/// loop, and stores the handle in the shared state for route access.
async fn start_feishu_connector(
    deployment: &DeploymentImpl,
    feishu_handle: &SharedFeishuHandle,
) -> Result<(), AnyhowError> {
    use feishu_connector::{reconnect::ReconnectPolicy, types::ClientConfig};
    use services::services::feishu::FeishuService;

    let service = FeishuService::from_db(
        deployment.db().pool.clone(),
        deployment.message_bus().clone(),
        decrypt_feishu_secret,
    )
    .await?;

    let Some(mut service) = service else {
        return Err(anyhow::anyhow!(
            "No enabled Feishu config found in database; skipping connector startup"
        ));
    };

    let (reconnect_tx, mut reconnect_rx) = tokio::sync::mpsc::channel::<()>(1);
    let connected = Arc::new(tokio::sync::RwLock::new(false));

    let handle = FeishuHandle {
        connected: connected.clone(),
        reconnect_tx,
    };

    // Store the handle so route handlers can access it
    *feishu_handle.write().await = Some(handle);

    let connected_flag = connected;
    tokio::spawn(async move {
        let mut policy = ReconnectPolicy::new(ClientConfig::default());
        loop {
            if let Err(e) = service.start().await {
                tracing::warn!(error = %e, "Feishu service disconnected");
            }
            *connected_flag.write().await = false;

            if let Some(d) = policy.next_delay() {
                tracing::info!(delay_ms = d.as_millis(), "Reconnecting Feishu...");
                tokio::select! {
                    () = tokio::time::sleep(d) => {}
                    _ = reconnect_rx.recv() => {
                        tracing::info!("Manual Feishu reconnect requested");
                        policy.reset();
                    }
                }
            } else {
                tracing::error!("Feishu max reconnect attempts reached");
                break;
            }
        }
    });

    Ok(())
}
