//! CLI installer service
//!
//! Provides per-CLI install and uninstall operations by spawning the
//! `install-single-cli.sh` script and streaming its output line by line.

use std::{collections::HashMap, path::PathBuf, sync::Arc};

use anyhow::{Context, Result, bail};
use tokio::{
    io::{AsyncBufReadExt, BufReader},
    process::Command,
    sync::Mutex,
};

/// Known CLI names that are allowed to be installed/uninstalled.
const ALLOWED_CLI_NAMES: &[&str] = &[
    "claude-code",
    "codex",
    "gemini-cli",
    "amp",
    "cursor-agent",
    "qwen-code",
    "copilot",
    "opencode",
    "droid",
];

/// Per-CLI installation lock to prevent concurrent installs of the same CLI.
type InstallLocks = Arc<Mutex<HashMap<String, Arc<Mutex<()>>>>>;

/// A single line of output from an install/uninstall operation.
#[derive(Debug, Clone)]
pub enum InstallOutputLine {
    /// A line written to stdout by the script.
    Stdout(String),
    /// A line written to stderr by the script.
    Stderr(String),
    /// The script has completed with the given exit code.
    Completed { exit_code: i32 },
    /// An error occurred while running the script.
    Error(String),
}

/// Streaming output from an install/uninstall operation.
pub struct InstallOutputStream {
    /// Channel receiver that yields output lines as they arrive.
    pub receiver: tokio::sync::mpsc::Receiver<InstallOutputLine>,
}

/// Service for installing and uninstalling individual AI CLI tools.
///
/// Each CLI is protected by a per-name lock so that concurrent requests for the
/// same CLI are rejected rather than racing each other. Operations have a
/// 10-minute timeout.
pub struct CliInstaller {
    install_locks: InstallLocks,
}

impl CliInstaller {
    /// Create a new `CliInstaller`.
    pub fn new() -> Self {
        Self {
            install_locks: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// Install a CLI by name.
    ///
    /// Spawns `install-single-cli.sh install <cli_name>` and returns a stream
    /// of output lines. Returns an error if the CLI name is invalid or an
    /// install is already in progress for this CLI.
    pub async fn install_cli(&self, cli_name: &str) -> Result<InstallOutputStream> {
        self.run_script("install", cli_name).await
    }

    /// Uninstall a CLI by name.
    ///
    /// Spawns `install-single-cli.sh uninstall <cli_name>` and returns a stream
    /// of output lines. Returns an error if the CLI name is invalid or an
    /// operation is already in progress for this CLI.
    pub async fn uninstall_cli(&self, cli_name: &str) -> Result<InstallOutputStream> {
        self.run_script("uninstall", cli_name).await
    }

    /// Resolve the path to `install-single-cli.sh`.
    ///
    /// Checks the Docker production path first, then falls back to the
    /// development path relative to the working directory.
    fn resolve_script_path() -> Option<PathBuf> {
        let mut candidates = vec![PathBuf::from(
            "/opt/gitcortex/install/install-single-cli.sh",
        )];
        if let Ok(cwd) = std::env::current_dir() {
            candidates.push(cwd.join("scripts/docker/install/install-single-cli.sh"));
        }
        candidates.push(PathBuf::from(
            "scripts/docker/install/install-single-cli.sh",
        ));

        candidates.into_iter().find(|path| path.is_file())
    }

    /// Validate that `cli_name` is in the allowed whitelist.
    fn validate_cli_name(cli_name: &str) -> Result<()> {
        if ALLOWED_CLI_NAMES.contains(&cli_name) {
            Ok(())
        } else {
            bail!(
                "Invalid CLI name: '{}'. Allowed: {}",
                cli_name,
                ALLOWED_CLI_NAMES.join(", ")
            )
        }
    }

    /// Internal method that spawns the install script and streams its output.
    async fn run_script(&self, action: &str, cli_name: &str) -> Result<InstallOutputStream> {
        // Validate cli_name against whitelist before doing anything else.
        Self::validate_cli_name(cli_name)?;

        let script_path = Self::resolve_script_path().context("install-single-cli.sh not found")?;

        // Acquire (or create) the per-CLI lock.
        let cli_lock = {
            let mut locks = self.install_locks.lock().await;
            locks
                .entry(cli_name.to_string())
                .or_insert_with(|| Arc::new(Mutex::new(())))
                .clone()
        };

        // Try to acquire the lock without blocking. If we can't, another
        // operation is already running for this CLI.
        let lock_guard = cli_lock.clone().try_lock_owned().map_err(|_| {
            anyhow::anyhow!("An operation is already in progress for CLI '{}'", cli_name)
        })?;

        let (tx, rx) = tokio::sync::mpsc::channel::<InstallOutputLine>(256);

        let action = action.to_string();
        let cli_name = cli_name.to_string();

        tracing::info!(
            action = %action,
            cli_name = %cli_name,
            script = %script_path.display(),
            "Starting CLI installer script"
        );

        tokio::spawn(async move {
            // Hold the lock guard for the duration of the spawned task so that
            // concurrent requests for the same CLI are rejected.
            let _lock = lock_guard;

            let result = Self::execute_script(&script_path, &action, &cli_name, &tx).await;

            if let Err(e) = result {
                tracing::error!(
                    action = %action,
                    cli_name = %cli_name,
                    error = %e,
                    "CLI installer script failed"
                );
                let _ = tx.send(InstallOutputLine::Error(e.to_string())).await;
            }
        });

        Ok(InstallOutputStream { receiver: rx })
    }

    /// Execute the script, read stdout/stderr line by line, and send through
    /// the channel. Enforces a 10-minute timeout.
    async fn execute_script(
        script_path: &PathBuf,
        action: &str,
        cli_name: &str,
        tx: &tokio::sync::mpsc::Sender<InstallOutputLine>,
    ) -> Result<()> {
        let mut child = Command::new("bash")
            .arg(script_path)
            .arg(action)
            .arg(cli_name)
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .spawn()
            .context("Failed to spawn install-single-cli.sh")?;

        let stdout = child.stdout.take().context("Failed to capture stdout")?;
        let stderr = child.stderr.take().context("Failed to capture stderr")?;

        let tx_out = tx.clone();
        let stdout_task = tokio::spawn(async move {
            let mut reader = BufReader::new(stdout).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                if tx_out.send(InstallOutputLine::Stdout(line)).await.is_err() {
                    break;
                }
            }
        });

        let tx_err = tx.clone();
        let stderr_task = tokio::spawn(async move {
            let mut reader = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                if tx_err.send(InstallOutputLine::Stderr(line)).await.is_err() {
                    break;
                }
            }
        });

        // 10-minute timeout
        let timeout = std::time::Duration::from_secs(600);
        let wait_result = tokio::time::timeout(timeout, child.wait()).await;

        match wait_result {
            Ok(Ok(status)) => {
                // Wait for output readers to finish
                let _ = stdout_task.await;
                let _ = stderr_task.await;

                let exit_code = status.code().unwrap_or(-1);
                tracing::info!(
                    action = action,
                    cli_name = cli_name,
                    exit_code = exit_code,
                    "CLI installer script completed"
                );
                let _ = tx.send(InstallOutputLine::Completed { exit_code }).await;
            }
            Ok(Err(e)) => {
                let _ = tx
                    .send(InstallOutputLine::Error(format!("Process error: {e}")))
                    .await;
            }
            Err(_) => {
                // Timeout: kill the child process
                tracing::warn!(
                    action = action,
                    cli_name = cli_name,
                    "CLI installer script timed out after 10 minutes, killing process"
                );
                let _ = child.kill().await;
                let _ = tx
                    .send(InstallOutputLine::Error(
                        "Operation timed out after 10 minutes".to_string(),
                    ))
                    .await;
            }
        }

        Ok(())
    }
}

impl Default for CliInstaller {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_cli_name_valid() {
        for name in ALLOWED_CLI_NAMES {
            assert!(
                CliInstaller::validate_cli_name(name).is_ok(),
                "Expected '{}' to be valid",
                name
            );
        }
    }

    #[test]
    fn test_validate_cli_name_invalid() {
        assert!(CliInstaller::validate_cli_name("not-a-cli").is_err());
        assert!(CliInstaller::validate_cli_name("").is_err());
        assert!(CliInstaller::validate_cli_name("rm -rf /").is_err());
        assert!(CliInstaller::validate_cli_name("claude-code; rm -rf /").is_err());
    }

    #[test]
    fn test_cli_installer_new() {
        let installer = CliInstaller::new();
        // Verify default construction works.
        let _default = CliInstaller::default();
        drop(installer);
    }

    #[tokio::test]
    async fn test_install_invalid_cli_name() {
        let installer = CliInstaller::new();
        let result = installer.install_cli("invalid-cli").await;
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("Invalid CLI name"));
    }

    #[tokio::test]
    async fn test_uninstall_invalid_cli_name() {
        let installer = CliInstaller::new();
        let result = installer.uninstall_cli("nope").await;
        assert!(result.is_err());
    }
}
