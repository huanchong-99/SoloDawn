//! Test server lifecycle management.
//!
//! Boots a real SoloDawn server with a temporary database, listens on a
//! random port, and provides a handle for shutdown + cleanup.

use std::{path::PathBuf, sync::Arc};

use anyhow::Result;
use deployment::Deployment;
use services::services::container::ContainerService;
use tempfile::TempDir;
use tokio::sync::oneshot;

use crate::DeploymentImpl;

/// A running test server instance.
pub struct TestServer {
    pub base_url: String,
    pub port: u16,
    temp_dir: TempDir,
    shutdown_tx: Option<oneshot::Sender<()>>,
}

impl TestServer {
    /// Boot a fully-initialized SoloDawn server on a random port with a
    /// fresh temporary database. Blocks until `/healthz` responds 200.
    pub async fn start() -> Result<Self> {
        let temp_dir = TempDir::new()?;
        let temp_path = temp_dir.path().to_path_buf();

        // Install rustls crypto provider before any TLS operations
        let _ = rustls::crypto::ring::default_provider().install_default();

        // Set environment for isolated testing
        unsafe {
            std::env::set_var("SOLODAWN_ASSET_DIR", temp_path.to_str().unwrap());
            std::env::set_var(
                "SOLODAWN_ENCRYPTION_KEY",
                "12345678901234567890123456789012",
            );
            std::env::set_var("SOLODAWN_LOCAL_MODE", "1");
            std::env::set_var("SOLODAWN_NO_BROWSER", "1");
        }

        // Initialize deployment (creates DB in temp dir)
        let deployment = DeploymentImpl::new().await?;
        deployment
            .container()
            .cleanup_orphan_executions()
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))?;

        // Initialize subscription hub and event bridge
        let subscription_hub: crate::routes::SharedSubscriptionHub =
            Arc::new(crate::routes::subscription_hub::SubscriptionHub::default());
        let event_bridge = crate::routes::event_bridge::EventBridge::new(
            deployment.message_bus().clone(),
            subscription_hub.clone(),
        );
        let _event_bridge_handle = event_bridge.spawn();

        // Initialize Concierge Agent
        let concierge_broadcaster =
            Arc::new(services::services::concierge::ConciergeBroadcaster::new());
        let concierge_agent = {
            let mut agent = services::services::concierge::ConciergeAgent::new(
                deployment.db().pool.clone(),
                concierge_broadcaster.clone(),
            );
            agent.set_shared_config(deployment.config().clone());
            agent.set_message_bus(deployment.message_bus().clone());
            Arc::new(agent)
        };

        // Wire concierge into orchestrator
        deployment
            .orchestrator_runtime()
            .set_concierge_broadcaster(concierge_broadcaster.clone())
            .await;

        // Feishu handle (not connected in test mode)
        let feishu_handle = crate::feishu_handle::new_shared_handle();

        let cli_health_monitor = deployment.cli_health_monitor().clone();
        let app_router = crate::routes::build_router(
            deployment.clone(),
            subscription_hub,
            feishu_handle,
            cli_health_monitor,
            concierge_agent,
            concierge_broadcaster,
        );

        // Bind to random port
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let port = listener.local_addr()?.port();
        let base_url = format!("http://127.0.0.1:{port}");

        // Spawn server with graceful shutdown
        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();
        tokio::spawn(async move {
            axum::serve(listener, app_router.into_make_service())
                .with_graceful_shutdown(async {
                    shutdown_rx.await.ok();
                })
                .await
                .ok();
        });

        // Wait for server to be ready
        let client = reqwest::Client::new();
        let healthz_url = format!("{base_url}/healthz");
        const MAX_ATTEMPTS: u32 = 60;
        const POLL_INTERVAL_MS: u64 = 500;
        let mut attempts: u32 = 0;
        loop {
            if attempts > MAX_ATTEMPTS {
                return Err(anyhow::anyhow!("Timeout waiting for server to start (30s)"));
            }
            match client.get(&healthz_url).send().await {
                Ok(resp) if resp.status().is_success() => break,
                _ => {
                    tokio::time::sleep(std::time::Duration::from_millis(POLL_INTERVAL_MS)).await;
                    attempts += 1;
                }
            }
        }

        Ok(TestServer {
            base_url,
            port,
            temp_dir,
            shutdown_tx: Some(shutdown_tx),
        })
    }

    /// Get the temp directory path (for creating git repos, etc.)
    pub fn temp_dir(&self) -> PathBuf {
        self.temp_dir.path().to_path_buf()
    }

    /// Gracefully shut down the server and clean up temp directory.
    pub async fn shutdown(mut self) {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        // Give server a moment to finish
        tokio::time::sleep(std::time::Duration::from_millis(500)).await;
    }
}
