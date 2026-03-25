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
use clap::{Parser, Subcommand};
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

#[derive(Parser)]
#[command(name = "gitcortex-server", about = "GitCortex Server")]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,
}

#[derive(Subcommand)]
enum Commands {
    /// Run headless API self-test (full coverage, ~164 tests)
    SelfTest {
        /// Output results as JSON
        #[arg(long)]
        json: bool,
        /// Run only specific test groups (comma-separated, e.g. "infra,config,projects")
        #[arg(long)]
        filter: Option<String>,
        /// Run orchestration E2E tests (requires E2E_API_KEY env var)
        #[arg(long)]
        orchestration: bool,
    },
}

const DEV_DEFAULT_ENCRYPTION_KEY: &str = "12345678901234567890123456789012";

fn ensure_api_token_in_release() {
    if !cfg!(debug_assertions) {
        // SEC-002: In release mode, require GITCORTEX_API_TOKEN — fail closed
        // Exception: local installer mode (localhost-only, no external access)
        if std::env::var("GITCORTEX_LOCAL_MODE").is_ok() {
            return;
        }
        match std::env::var("GITCORTEX_API_TOKEN") {
            Ok(value) if !value.trim().is_empty() => {}
            Ok(_) | Err(_) => {
                panic!(
                    "GITCORTEX_API_TOKEN is not set or is empty. \
                     An API token is required in release mode to prevent unauthenticated access. \
                     Set GITCORTEX_LOCAL_MODE=1 to skip this check for localhost-only installations."
                );
            }
        }
    }
}

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
    let cli = Cli::parse();

    match cli.command {
        Some(Commands::SelfTest { json, filter, orchestration }) => {
            let exit_code = server::self_test::run(json, filter, orchestration).await;
            std::process::exit(exit_code);
        }
        None => run_server().await,
    }
}

async fn run_server() -> Result<(), GitCortexError> {
    // Load environment variables from .env file
    dotenv::dotenv().ok();
    ensure_dev_encryption_key();
    ensure_api_token_in_release();

    // Install rustls crypto provider before any TLS operations
    rustls::crypto::ring::default_provider()
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

    // Initialize Concierge Agent (must be created before Feishu connector)
    let concierge_broadcaster = Arc::new(services::services::concierge::ConciergeBroadcaster::new());
    let concierge_agent = {
        let mut agent = services::services::concierge::ConciergeAgent::new(
            deployment.db().pool.clone(),
            concierge_broadcaster.clone(),
        );
        agent.set_shared_config(deployment.config().clone());
        agent.set_message_bus(deployment.message_bus().clone());
        Arc::new(agent)
    };
    tracing::info!("Concierge Agent initialized");

    // Wire concierge broadcaster into orchestrator runtime for terminal bridge messages
    deployment
        .orchestrator_runtime()
        .set_concierge_broadcaster(concierge_broadcaster.clone())
        .await;

    // Conditional Feishu connector startup
    let feishu_handle = server::feishu_handle::new_shared_handle();
    if db::models::system_settings::SystemSetting::is_feishu_enabled(&deployment.db().pool).await {
        match start_feishu_connector(&deployment, &feishu_handle, concierge_agent.clone()).await {
            Ok(()) => tracing::info!("Feishu connector started"),
            Err(e) => tracing::warn!("Feishu connector startup skipped: {e}"),
        }
    } else {
        tracing::debug!("Feishu integration disabled (neither env var nor database setting enabled)");
    }

    // Sync config.json model library → DB on startup so the Orchestrator
    // Agent can always see user-configured models even after a DB reset.
    {
        let cfg = deployment.config().read().await;
        let pool = &deployment.db().pool;
        for item in &cfg.workflow_model_library {
            let cli_type_id = item.cli_type_id.as_deref().unwrap_or("cli-codex");
            if let Err(e) = db::models::ModelConfig::create_custom(
                pool, &item.id, cli_type_id, &item.display_name, &item.model_id,
            ).await {
                tracing::warn!(model_id = %item.id, error = %e, "Startup model sync failed");
                continue;
            }
            if !item.api_key.is_empty() {
                let mut tmp_model = db::models::ModelConfig {
                    id: String::new(), cli_type_id: String::new(), name: String::new(),
                    display_name: String::new(), api_model_id: None, is_default: false,
                    is_official: false, created_at: chrono::Utc::now(), updated_at: chrono::Utc::now(),
                    encrypted_api_key: None, base_url: None, api_type: None, has_api_key: false,
                };
                if let Ok(()) = tmp_model.set_api_key(&item.api_key) {
                    if let Some(ref encrypted) = tmp_model.encrypted_api_key {
                        let _ = db::models::ModelConfig::update_credentials(
                            pool, &item.id, encrypted, Some(&item.base_url), &item.api_type,
                        ).await;
                    }
                }
            }
            tracing::info!(model_id = %item.id, "Startup: synced model config to DB");
        }
    }

    let cli_health_monitor = deployment.cli_health_monitor().clone();
    let app_router = routes::router(
        deployment.clone(),
        subscription_hub,
        feishu_handle,
        cli_health_monitor,
        concierge_agent,
        concierge_broadcaster,
    );

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

    if !cfg!(debug_assertions)
        && !std::path::Path::new("/.dockerenv").exists()
        && std::env::var("GITCORTEX_NO_BROWSER").is_err()
    {
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
    concierge_agent: Arc<services::services::concierge::ConciergeAgent>,
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

    // Inject Concierge Agent and shared config so Feishu messages route through it
    service.set_concierge_agent(concierge_agent);
    service.set_shared_config(deployment.config().clone());

    let (reconnect_tx, mut reconnect_rx) = tokio::sync::mpsc::channel::<()>(1);
    let connected = service.connected_flag();
    let messenger = service.messenger().clone();
    let (event_tx, _) = tokio::sync::broadcast::channel::<feishu_connector::events::FeishuEvent>(64);
    service.set_event_broadcaster(event_tx.clone());

    let handle = FeishuHandle {
        connected: connected.clone(),
        reconnect_tx,
        messenger,
        event_tx,
        last_chat_id: Arc::new(tokio::sync::RwLock::new(None)),
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
