//! Runner client abstraction for terminal process management.
//!
//! Provides a unified `RunnerClient` trait with two implementations:
//! - `LocalRunner`: wraps an in-process `ProcessManager` for local mode
//! - `RemoteRunner`: gRPC client stub for remote runner mode (placeholder)
//!
//! The `RunnerClientImpl` enum delegates to the appropriate implementation
//! based on the `GITCORTEX_RUNNER_MODE` environment variable.

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Result, bail};
use async_trait::async_trait;

use super::terminal::process::{ProcessManager, SpawnCommand, SpawnEnv};

// ============================================================================
// Data Types
// ============================================================================

/// Result of spawning a terminal on a runner.
#[derive(Debug, Clone)]
pub struct SpawnResult {
    pub pid: u32,
}

/// Configuration for spawning a terminal.
#[derive(Debug, Clone)]
pub struct TerminalSpawnConfig {
    pub terminal_id: String,
    pub command: String,
    pub args: Vec<String>,
    pub working_dir: String,
    pub env_set: HashMap<String, String>,
    pub env_unset: Vec<String>,
    pub cols: u32,
    pub rows: u32,
}

/// Health status of a runner.
#[derive(Debug, Clone)]
pub struct RunnerHealth {
    pub healthy: bool,
    pub active_terminals: u32,
}

// ============================================================================
// RunnerClient Trait
// ============================================================================

/// Abstract interface for terminal process management.
/// Implemented by `LocalRunner` (in-process) and `RemoteRunner` (gRPC).
#[async_trait]
pub trait RunnerClient: Send + Sync + 'static {
    async fn spawn_terminal(&self, config: TerminalSpawnConfig) -> Result<SpawnResult>;
    async fn kill_terminal(&self, terminal_id: &str) -> Result<()>;
    async fn is_running(&self, terminal_id: &str) -> Result<bool>;
    async fn write_input(&self, terminal_id: &str, data: &[u8]) -> Result<()>;
    async fn resize_terminal(&self, terminal_id: &str, cols: u32, rows: u32) -> Result<()>;
    async fn health_check(&self) -> Result<RunnerHealth>;
}

// ============================================================================
// LocalRunner
// ============================================================================

/// In-process runner that delegates to a `ProcessManager` directly.
#[derive(Clone)]
pub struct LocalRunner {
    process_manager: Arc<ProcessManager>,
}

impl std::fmt::Debug for LocalRunner {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("LocalRunner")
            .field("process_manager", &"Arc<ProcessManager>")
            .finish()
    }
}

impl LocalRunner {
    /// Creates a new `LocalRunner` wrapping the given `ProcessManager`.
    pub fn new(process_manager: Arc<ProcessManager>) -> Self {
        Self { process_manager }
    }
}

#[async_trait]
impl RunnerClient for LocalRunner {
    async fn spawn_terminal(&self, config: TerminalSpawnConfig) -> Result<SpawnResult> {
        let env = SpawnEnv {
            set: config.env_set,
            unset: config.env_unset,
        };
        let spawn_cmd = SpawnCommand::new(&config.command, &config.working_dir)
            .with_args(config.args)
            .with_env(env);

        let cols = u16::try_from(config.cols).unwrap_or(80);
        let rows = u16::try_from(config.rows).unwrap_or(24);

        let handle = self
            .process_manager
            .spawn_pty_with_config(&config.terminal_id, &spawn_cmd, cols, rows)
            .await?;

        tracing::info!(
            terminal_id = %config.terminal_id,
            pid = handle.pid,
            "LocalRunner spawned terminal"
        );

        Ok(SpawnResult { pid: handle.pid })
    }

    async fn kill_terminal(&self, terminal_id: &str) -> Result<()> {
        self.process_manager.kill_terminal(terminal_id).await
    }

    async fn is_running(&self, terminal_id: &str) -> Result<bool> {
        Ok(self.process_manager.is_running(terminal_id).await)
    }

    async fn write_input(&self, terminal_id: &str, data: &[u8]) -> Result<()> {
        let handle = self
            .process_manager
            .get_handle(terminal_id)
            .await
            .ok_or_else(|| anyhow::anyhow!("Terminal not found: {terminal_id}"))?;

        let writer = handle
            .writer
            .ok_or_else(|| anyhow::anyhow!("No PTY writer available for terminal: {terminal_id}"))?;

        let data = data.to_vec();
        tokio::task::spawn_blocking(move || {
            let mut writer = writer
                .lock()
                .map_err(|e| anyhow::anyhow!("Failed to lock PTY writer: {e}"))?;
            writer.write_all(&data)?;
            writer.flush()?;
            Ok(())
        })
        .await
        .map_err(|e| anyhow::anyhow!("spawn_blocking join error: {e}"))?
    }

    async fn resize_terminal(&self, terminal_id: &str, cols: u32, rows: u32) -> Result<()> {
        let cols = u16::try_from(cols).unwrap_or(80);
        let rows = u16::try_from(rows).unwrap_or(24);
        self.process_manager.resize(terminal_id, cols, rows).await
    }

    async fn health_check(&self) -> Result<RunnerHealth> {
        let running = self.process_manager.list_running().await;
        Ok(RunnerHealth {
            healthy: true,
            active_terminals: u32::try_from(running.len()).unwrap_or(u32::MAX),
        })
    }
}

// ============================================================================
// RemoteRunner
// ============================================================================

/// gRPC client stub for a remote runner process.
///
/// All methods currently return an error since the proto is not yet compiled.
// TODO: Connect to gRPC RunnerService when proto is compiled
#[derive(Debug, Clone)]
pub struct RemoteRunner {
    addr: String,
}

impl RemoteRunner {
    /// Creates a new `RemoteRunner` targeting the given address.
    ///
    /// Does not establish a connection; call `connect` to initialize the gRPC channel.
    pub fn new(addr: impl Into<String>) -> Self {
        Self { addr: addr.into() }
    }

    /// Attempts to connect to the remote runner gRPC service.
    // TODO: Connect to gRPC RunnerService when proto is compiled
    pub async fn connect(addr: &str) -> Result<Self> {
        tracing::info!(addr = %addr, "RemoteRunner created (gRPC not yet connected)");
        Ok(Self {
            addr: addr.to_string(),
        })
    }

    /// Returns the configured remote address.
    pub fn addr(&self) -> &str {
        &self.addr
    }
}

#[async_trait]
impl RunnerClient for RemoteRunner {
    async fn spawn_terminal(&self, _config: TerminalSpawnConfig) -> Result<SpawnResult> {
        bail!("Remote runner not yet connected (addr={})", self.addr)
    }

    async fn kill_terminal(&self, _terminal_id: &str) -> Result<()> {
        bail!("Remote runner not yet connected (addr={})", self.addr)
    }

    async fn is_running(&self, _terminal_id: &str) -> Result<bool> {
        bail!("Remote runner not yet connected (addr={})", self.addr)
    }

    async fn write_input(&self, _terminal_id: &str, _data: &[u8]) -> Result<()> {
        bail!("Remote runner not yet connected (addr={})", self.addr)
    }

    async fn resize_terminal(&self, _terminal_id: &str, _cols: u32, _rows: u32) -> Result<()> {
        bail!("Remote runner not yet connected (addr={})", self.addr)
    }

    async fn health_check(&self) -> Result<RunnerHealth> {
        bail!("Remote runner not yet connected (addr={})", self.addr)
    }
}

// ============================================================================
// RunnerClientImpl (Unified Enum)
// ============================================================================

/// Unified runner client that dispatches to either a local or remote implementation.
#[derive(Debug, Clone)]
pub enum RunnerClientImpl {
    Local(LocalRunner),
    Remote(RemoteRunner),
}

/// Shared reference alias for `RunnerClientImpl`.
pub type SharedRunnerClient = Arc<RunnerClientImpl>;

#[async_trait]
impl RunnerClient for RunnerClientImpl {
    async fn spawn_terminal(&self, config: TerminalSpawnConfig) -> Result<SpawnResult> {
        match self {
            Self::Local(inner) => inner.spawn_terminal(config).await,
            Self::Remote(inner) => inner.spawn_terminal(config).await,
        }
    }

    async fn kill_terminal(&self, terminal_id: &str) -> Result<()> {
        match self {
            Self::Local(inner) => inner.kill_terminal(terminal_id).await,
            Self::Remote(inner) => inner.kill_terminal(terminal_id).await,
        }
    }

    async fn is_running(&self, terminal_id: &str) -> Result<bool> {
        match self {
            Self::Local(inner) => inner.is_running(terminal_id).await,
            Self::Remote(inner) => inner.is_running(terminal_id).await,
        }
    }

    async fn write_input(&self, terminal_id: &str, data: &[u8]) -> Result<()> {
        match self {
            Self::Local(inner) => inner.write_input(terminal_id, data).await,
            Self::Remote(inner) => inner.write_input(terminal_id, data).await,
        }
    }

    async fn resize_terminal(&self, terminal_id: &str, cols: u32, rows: u32) -> Result<()> {
        match self {
            Self::Local(inner) => inner.resize_terminal(terminal_id, cols, rows).await,
            Self::Remote(inner) => inner.resize_terminal(terminal_id, cols, rows).await,
        }
    }

    async fn health_check(&self) -> Result<RunnerHealth> {
        match self {
            Self::Local(inner) => inner.health_check().await,
            Self::Remote(inner) => inner.health_check().await,
        }
    }
}

impl RunnerClientImpl {
    /// Creates a `RunnerClientImpl` based on environment variables.
    ///
    /// - `GITCORTEX_RUNNER_MODE`: `"local"` (default) or `"remote"`
    /// - `GITCORTEX_RUNNER_ADDR`: remote runner address (required when mode is `"remote"`)
    pub fn from_env(process_manager: Arc<ProcessManager>) -> Result<Self> {
        let mode = std::env::var("GITCORTEX_RUNNER_MODE")
            .unwrap_or_else(|_| "local".to_string());

        match mode.to_lowercase().as_str() {
            "local" => {
                tracing::info!("Runner mode: local (in-process ProcessManager)");
                Ok(Self::Local(LocalRunner::new(process_manager)))
            }
            "remote" => {
                let addr = std::env::var("GITCORTEX_RUNNER_ADDR").unwrap_or_else(|_| {
                    tracing::warn!(
                        "GITCORTEX_RUNNER_ADDR not set, defaulting to http://runner:50051"
                    );
                    "http://runner:50051".to_string()
                });
                tracing::info!(addr = %addr, "Runner mode: remote (gRPC stub)");
                Ok(Self::Remote(RemoteRunner::new(addr)))
            }
            other => {
                bail!(
                    "Unknown GITCORTEX_RUNNER_MODE: '{other}'. Expected 'local' or 'remote'."
                );
            }
        }
    }
}
