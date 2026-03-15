//! CLI Health Monitor Service
//!
//! Background service that periodically checks CLI availability and publishes
//! status changes via a broadcast channel for real-time SSE consumption.

use std::collections::HashMap;
use std::sync::Arc;

use chrono::{DateTime, Utc};
use db::DBService;
use tokio::sync::{broadcast, RwLock};
use tokio::time::Duration;

use super::terminal::detector::CliDetector;

/// Represents a change in CLI installation status.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CliStatusChange {
    pub cli_type_id: String,
    pub cli_name: String,
    pub previous_installed: bool,
    pub current_installed: bool,
    pub previous_version: Option<String>,
    pub current_version: Option<String>,
    pub detected_at: DateTime<Utc>,
}

/// Cached CLI detection result with timestamp.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CachedCliStatus {
    pub cli_type_id: String,
    pub name: String,
    pub display_name: String,
    pub installed: bool,
    pub version: Option<String>,
    pub executable_path: Option<String>,
    pub detected_at: DateTime<Utc>,
}

/// Background health monitor that periodically detects CLI availability
/// and broadcasts status changes.
pub struct CliHealthMonitor {
    /// Cached detection results keyed by cli_type_id
    cache: Arc<RwLock<HashMap<String, CachedCliStatus>>>,
    /// Broadcast channel for status changes
    change_sender: broadcast::Sender<CliStatusChange>,
    /// Detection interval in seconds
    interval_secs: u64,
}

/// Shared handle to a `CliHealthMonitor`.
pub type SharedCliHealthMonitor = Arc<CliHealthMonitor>;

impl CliHealthMonitor {
    /// Create a new health monitor.
    ///
    /// The interval defaults to `GITCORTEX_CLI_HEALTH_INTERVAL_SECS` env var
    /// (falling back to 300 seconds / 5 minutes).
    pub fn new(interval_secs: u64) -> Self {
        let effective_interval = if interval_secs == 0 {
            std::env::var("GITCORTEX_CLI_HEALTH_INTERVAL_SECS")
                .ok()
                .and_then(|v| v.parse::<u64>().ok())
                .unwrap_or(300)
        } else {
            interval_secs
        };

        let (change_sender, _) = broadcast::channel(128);

        Self {
            cache: Arc::new(RwLock::new(HashMap::new())),
            change_sender,
            interval_secs: effective_interval,
        }
    }

    /// Spawn the background monitoring loop.
    ///
    /// The task runs indefinitely, detecting CLIs every `interval_secs` seconds.
    pub fn start(self: &Arc<Self>, db: Arc<DBService>) {
        let monitor = Arc::clone(self);
        tokio::spawn(async move {
            monitor.monitor_loop(db).await;
        });
    }

    /// Subscribe to CLI status change events.
    pub fn subscribe(&self) -> broadcast::Receiver<CliStatusChange> {
        self.change_sender.subscribe()
    }

    /// Return a snapshot of the current cached statuses.
    pub async fn get_cached_statuses(&self) -> Vec<CachedCliStatus> {
        let cache = self.cache.read().await;
        cache.values().cloned().collect()
    }

    /// Remove a specific CLI from the cache, triggering re-detection on the
    /// next cycle.
    pub async fn invalidate(&self, cli_type_id: &str) {
        let mut cache = self.cache.write().await;
        cache.remove(cli_type_id);
        tracing::debug!(cli_type_id, "Invalidated CLI health cache entry");
    }

    /// Immediately run a full detection cycle and return the results.
    pub async fn force_refresh(self: &Arc<Self>, db: Arc<DBService>) -> Vec<CachedCliStatus> {
        if let Err(e) = self.run_detection_cycle(&db).await {
            tracing::warn!(error = %e, "CLI health force-refresh detection cycle failed");
        }
        self.get_cached_statuses().await
    }

    /// The main monitoring loop — runs detection every `interval_secs`.
    async fn monitor_loop(self: Arc<Self>, db: Arc<DBService>) {
        let mut interval = tokio::time::interval(Duration::from_secs(self.interval_secs));
        loop {
            interval.tick().await;
            if let Err(e) = self.run_detection_cycle(&db).await {
                tracing::warn!(error = %e, "CLI health detection cycle failed");
            }
        }
    }

    /// Run a single detection cycle: detect all CLIs, compare with cache,
    /// publish changes, and update the cache.
    async fn run_detection_cycle(&self, db: &DBService) -> anyhow::Result<()> {
        let detector = CliDetector::new(Arc::new(db.clone()));
        let results = detector.detect_all().await?;
        let now = Utc::now();

        let mut cache = self.cache.write().await;

        for status in results {
            let cached = cache.get(&status.cli_type_id);

            // Detect changes in installation status or version
            let changed = match cached {
                Some(prev) => {
                    prev.installed != status.installed || prev.version != status.version
                }
                None => true, // First detection counts as a change
            };

            if changed {
                let change = CliStatusChange {
                    cli_type_id: status.cli_type_id.clone(),
                    cli_name: status.name.clone(),
                    previous_installed: cached.map(|c| c.installed).unwrap_or(false),
                    current_installed: status.installed,
                    previous_version: cached.and_then(|c| c.version.clone()),
                    current_version: status.version.clone(),
                    detected_at: now,
                };

                // Broadcast — ignore error when there are no receivers
                let _ = self.change_sender.send(change);
            }

            let new_cached = CachedCliStatus {
                cli_type_id: status.cli_type_id,
                name: status.name,
                display_name: status.display_name,
                installed: status.installed,
                version: status.version,
                executable_path: status.executable_path,
                detected_at: now,
            };

            cache.insert(new_cached.cli_type_id.clone(), new_cached);
        }

        // TODO: Update cli_detection_cache DB table when the table is available.
        // This would persist detection results across server restarts.

        tracing::debug!(
            cached_count = cache.len(),
            "CLI health detection cycle completed"
        );

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_new_with_explicit_interval() {
        let monitor = CliHealthMonitor::new(60);
        assert_eq!(monitor.interval_secs, 60);
    }

    #[test]
    fn test_new_default_interval() {
        // When 0 is passed and env var is not set, falls back to 300
        let monitor = CliHealthMonitor::new(0);
        // Will be 300 unless GITCORTEX_CLI_HEALTH_INTERVAL_SECS is set
        assert!(monitor.interval_secs > 0);
    }

    #[tokio::test]
    async fn test_cache_starts_empty() {
        let monitor = CliHealthMonitor::new(60);
        let statuses = monitor.get_cached_statuses().await;
        assert!(statuses.is_empty());
    }

    #[tokio::test]
    async fn test_cache_insert_and_retrieve() {
        let monitor = CliHealthMonitor::new(60);
        let now = Utc::now();

        {
            let mut cache = monitor.cache.write().await;
            cache.insert(
                "cli-test".to_string(),
                CachedCliStatus {
                    cli_type_id: "cli-test".to_string(),
                    name: "test-cli".to_string(),
                    display_name: "Test CLI".to_string(),
                    installed: true,
                    version: Some("1.0.0".to_string()),
                    executable_path: Some("/usr/bin/test-cli".to_string()),
                    detected_at: now,
                },
            );
        }

        let statuses = monitor.get_cached_statuses().await;
        assert_eq!(statuses.len(), 1);
        assert_eq!(statuses[0].cli_type_id, "cli-test");
        assert!(statuses[0].installed);
        assert_eq!(statuses[0].version, Some("1.0.0".to_string()));
    }

    #[tokio::test]
    async fn test_invalidate_removes_entry() {
        let monitor = CliHealthMonitor::new(60);
        let now = Utc::now();

        {
            let mut cache = monitor.cache.write().await;
            cache.insert(
                "cli-test".to_string(),
                CachedCliStatus {
                    cli_type_id: "cli-test".to_string(),
                    name: "test-cli".to_string(),
                    display_name: "Test CLI".to_string(),
                    installed: true,
                    version: Some("1.0.0".to_string()),
                    executable_path: None,
                    detected_at: now,
                },
            );
        }

        assert_eq!(monitor.get_cached_statuses().await.len(), 1);
        monitor.invalidate("cli-test").await;
        assert!(monitor.get_cached_statuses().await.is_empty());
    }

    #[tokio::test]
    async fn test_subscribe_receives_nothing_initially() {
        let monitor = CliHealthMonitor::new(60);
        let mut rx = monitor.subscribe();

        // No messages should be available
        assert!(rx.try_recv().is_err());
    }

    #[test]
    fn test_cli_status_change_serialization() {
        let change = CliStatusChange {
            cli_type_id: "cli-claude".to_string(),
            cli_name: "claude".to_string(),
            previous_installed: false,
            current_installed: true,
            previous_version: None,
            current_version: Some("1.0.0".to_string()),
            detected_at: Utc::now(),
        };

        let json = serde_json::to_string(&change).unwrap();
        assert!(json.contains("cliTypeId"));
        assert!(json.contains("cliName"));
        assert!(json.contains("previousInstalled"));
        assert!(json.contains("currentInstalled"));
    }

    #[test]
    fn test_cached_cli_status_serialization() {
        let status = CachedCliStatus {
            cli_type_id: "cli-claude".to_string(),
            name: "claude".to_string(),
            display_name: "Claude Code".to_string(),
            installed: true,
            version: Some("1.2.3".to_string()),
            executable_path: Some("/usr/local/bin/claude".to_string()),
            detected_at: Utc::now(),
        };

        let json = serde_json::to_string(&status).unwrap();
        assert!(json.contains("cliTypeId"));
        assert!(json.contains("displayName"));
        assert!(json.contains("executablePath"));
    }

    #[tokio::test]
    async fn test_broadcast_channel_sends_changes() {
        let monitor = CliHealthMonitor::new(60);
        let mut rx = monitor.subscribe();

        let change = CliStatusChange {
            cli_type_id: "cli-test".to_string(),
            cli_name: "test".to_string(),
            previous_installed: false,
            current_installed: true,
            previous_version: None,
            current_version: Some("1.0.0".to_string()),
            detected_at: Utc::now(),
        };

        // Send directly through the sender
        monitor.change_sender.send(change.clone()).unwrap();

        let received = rx.try_recv().unwrap();
        assert_eq!(received.cli_type_id, "cli-test");
        assert!(received.current_installed);
    }
}
