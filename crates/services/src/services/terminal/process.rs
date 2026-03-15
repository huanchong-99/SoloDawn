//! Process management with PTY support
//!
//! Manages terminal process lifecycle including spawning, monitoring, and cleanup.
//! Uses portable-pty for cross-platform PTY support (Windows ConPTY, Unix PTY).

use std::{
    collections::HashMap,
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
    },
    time::Duration,
};

use db::DBService;
use portable_pty::{Child, CommandBuilder, MasterPty, PtySize, native_pty_system};
use tokio::{
    sync::{Mutex as AsyncMutex, RwLock, oneshot},
    task::JoinHandle,
};
use uuid::Uuid;

use super::{
    output_fanout::{OutputFanout, OutputFanoutConfig, OutputSubscription},
    utf8_decoder::Utf8StreamDecoder,
};

// ============================================================================
// PTY Size Configuration
// ============================================================================

/// Default terminal columns
pub const DEFAULT_COLS: u16 = 80;

/// Default terminal rows
pub const DEFAULT_ROWS: u16 = 24;

/// Reader buffer size for background PTY output fanout
pub const PROCESS_PTY_READ_BUFFER_SIZE: usize = 4096;

/// Broadcast channel capacity for terminal output fanout.
pub const PROCESS_BROADCAST_CAPACITY: usize = 512;

/// Default replay chunk retention per terminal stream.
///
/// [G09-005] Replay capacity is set to 2x broadcast capacity so that chunks
/// that are still in-flight in the broadcast channel are guaranteed to also
/// exist in the replay buffer, enabling lag-recovery without data loss.
/// [G09-004] For production tuning consider making this an env-var / config.
pub const PROCESS_REPLAY_MAX_CHUNKS: usize = PROCESS_BROADCAST_CAPACITY * 2;

/// Default replay byte retention per terminal stream
pub const PROCESS_REPLAY_MAX_BYTES: usize = 1024 * 1024;

/// Graceful shutdown timeout for PTY reader task
const READER_SHUTDOWN_TIMEOUT_SECS: u64 = 1;

/// Graceful shutdown timeout for terminal logger task
const LOGGER_SHUTDOWN_TIMEOUT_SECS: u64 = 2;
/// Logger task timeout needs to include flush worker shutdown + final flush.
const LOGGER_TASK_SHUTDOWN_TIMEOUT_SECS: u64 = LOGGER_SHUTDOWN_TIMEOUT_SECS * 2;
/// SQLite error code for FOREIGN KEY constraint failed.
const SQLITE_FOREIGN_KEY_CONSTRAINT_CODE: &str = "787";

// ============================================================================
// Spawn Configuration (Process Isolation)
// ============================================================================

/// Environment variable configuration for process-level isolation.
///
/// Supports both setting new environment variables and removing inherited ones
/// to prevent parent process pollution.
#[derive(Debug, Clone, Default)]
pub struct SpawnEnv {
    /// Environment variables to set on the child process.
    pub set: HashMap<String, String>,
    /// Environment variable keys to remove from the inherited environment.
    /// Use this to prevent parent process environment from leaking into child.
    pub unset: Vec<String>,
}

impl SpawnEnv {
    /// Creates a new empty SpawnEnv.
    pub fn new() -> Self {
        Self::default()
    }

    /// Adds an environment variable to set.
    pub fn with_var(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.set.insert(key.into(), value.into());
        self
    }

    /// Adds an environment variable key to unset (remove from inherited env).
    pub fn with_unset(mut self, key: impl Into<String>) -> Self {
        self.unset.push(key.into());
        self
    }

    /// Checks if an environment variable key contains sensitive information.
    /// Used for log redaction.
    pub fn is_sensitive_key(key: &str) -> bool {
        let key_upper = key.to_ascii_uppercase();
        key_upper.contains("KEY")
            || key_upper.contains("TOKEN")
            || key_upper.contains("SECRET")
            || key_upper.contains("PASSWORD")
            || key_upper.contains("CREDENTIAL")
    }
}

/// Command configuration for spawning terminal processes.
///
/// Encapsulates all information needed to spawn a process with proper isolation:
/// command, arguments, working directory, and environment configuration.
#[derive(Debug, Clone)]
pub struct SpawnCommand {
    /// Command to execute (e.g., "claude", "codex", "gemini").
    pub command: String,
    /// Command-line arguments.
    pub args: Vec<String>,
    /// Working directory for the child process.
    pub working_dir: PathBuf,
    /// Environment variable configuration for process isolation.
    pub env: SpawnEnv,
}

impl SpawnCommand {
    /// Creates a new SpawnCommand with the given command and working directory.
    pub fn new(command: impl Into<String>, working_dir: impl Into<PathBuf>) -> Self {
        Self {
            command: command.into(),
            args: Vec::new(),
            working_dir: working_dir.into(),
            env: SpawnEnv::default(),
        }
    }

    /// Adds a command-line argument.
    pub fn with_arg(mut self, arg: impl Into<String>) -> Self {
        self.args.push(arg.into());
        self
    }

    /// Adds multiple command-line arguments.
    pub fn with_args(mut self, args: impl IntoIterator<Item = impl Into<String>>) -> Self {
        self.args.extend(args.into_iter().map(Into::into));
        self
    }

    /// Sets the environment configuration.
    pub fn with_env(mut self, env: SpawnEnv) -> Self {
        self.env = env;
        self
    }
}

// ============================================================================
// Process Handle Types
// ============================================================================

/// PTY reader wrapper for async reading
pub struct PtyReader(Box<dyn Read + Send>);

impl PtyReader {
    /// Read bytes from PTY (blocking)
    pub fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        self.0.read(buf)
    }
}

/// PTY writer wrapper for async writing
pub struct PtyWriter(Box<dyn Write + Send>);

impl PtyWriter {
    /// Write bytes to PTY (blocking)
    pub fn write_all(&mut self, buf: &[u8]) -> std::io::Result<()> {
        self.0.write_all(buf)
    }

    /// Flush PTY writer
    pub fn flush(&mut self) -> std::io::Result<()> {
        self.0.flush()
    }
}

/// Process handle for tracking spawned PTY processes
pub struct ProcessHandle {
    /// Process ID
    pub pid: u32,
    /// Unique session identifier
    pub session_id: String,
    /// Associated terminal ID
    pub terminal_id: String,
    /// PTY reader (for WebSocket forwarding) - single stream, no stdout/stderr separation
    pub reader: Option<PtyReader>,
    /// Shared PTY writer (for WebSocket input) - wrapped in Arc<Mutex> for reconnection support
    pub writer: Option<Arc<Mutex<PtyWriter>>>,
}

// Implement Debug manually since portable-pty types don't implement Debug
impl std::fmt::Debug for ProcessHandle {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProcessHandle")
            .field("pid", &self.pid)
            .field("session_id", &self.session_id)
            .field("terminal_id", &self.terminal_id)
            .field("reader", &self.reader.is_some())
            .field("writer", &self.writer.is_some())
            .finish()
    }
}

// ============================================================================
// Tracked Process
// ============================================================================

/// Tracked process with PTY master and child handles
struct TrackedProcess {
    /// Stable PTY session identifier for this terminal process lifecycle
    session_id: String,
    /// Child process for lifecycle management
    child: Box<dyn Child + Send + Sync>,
    /// PTY master for I/O and resize operations (wrapped in Mutex for Sync)
    master: Mutex<Box<dyn MasterPty + Send>>,
    /// Shared PTY writer (initialized on first get_handle call, then reused for reconnections)
    shared_writer: Option<Arc<Mutex<PtyWriter>>>,
    /// Isolated CODEX_HOME path (for Codex terminals, cleaned up on exit)
    codex_home: Option<PathBuf>,
    /// Output fanout hub (single reader -> multi-subscriber)
    output_fanout: Arc<OutputFanout>,
    /// Background PTY reader task
    reader_task: Option<JoinHandle<()>>,
    /// Background terminal log persistence task
    logger_task: Option<JoinHandle<()>>,
    /// Shutdown signal for graceful terminal log task stop
    logger_shutdown_tx: Option<oneshot::Sender<()>>,
}

// ============================================================================
// CODEX_HOME Cleanup Guard (RAII)
// ============================================================================

/// Guard that ensures CODEX_HOME directories are cleaned up on early spawn failures.
/// Uses RAII pattern to guarantee cleanup even if spawn_pty_with_config returns early.
struct CodexHomeGuard {
    terminal_id: String,
    path: Option<PathBuf>,
}

impl CodexHomeGuard {
    fn new(terminal_id: &str, path: Option<PathBuf>) -> Self {
        Self {
            terminal_id: terminal_id.to_string(),
            path,
        }
    }

    /// Disarm the guard after successful process tracking.
    /// The CODEX_HOME will be cleaned up by TrackedProcess instead.
    fn disarm(&mut self) {
        self.path = None;
    }
}

impl Drop for CodexHomeGuard {
    fn drop(&mut self) {
        if let Some(path) = self.path.take() {
            ProcessManager::cleanup_codex_home(&self.terminal_id, &path);
        }
    }
}

// ============================================================================
// Process Manager
// ============================================================================

/// Process manager for terminal lifecycle with PTY support
///
/// [G21-006] No `Drop` implementation: process cleanup is handled by the runtime
/// shutdown sequence which calls `kill_terminal` / `finalize_terminated_process`
/// for each tracked process. Implementing `Drop` would require blocking I/O
/// (task joins, CODEX_HOME cleanup) which is not safe in a synchronous destructor.
pub struct ProcessManager {
    processes: Arc<RwLock<HashMap<String, TrackedProcess>>>,
}

impl ProcessManager {
    /// Creates a new ProcessManager instance
    pub fn new() -> Self {
        Self {
            processes: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Cleans up CODEX_HOME temporary directory for a terminated Codex terminal.
    ///
    /// Safety: Only removes directories under the gitcortex temp directory to prevent
    /// accidental deletion of user data.
    fn cleanup_codex_home(terminal_id: &str, codex_home: &Path) {
        if codex_home.as_os_str().is_empty() {
            return;
        }

        // Safety check: only clean up directories under our temp directory
        let base_dir = std::env::temp_dir().join("gitcortex");
        if !codex_home.starts_with(&base_dir) {
            tracing::warn!(
                terminal_id = %terminal_id,
                codex_home = %codex_home.display(),
                "Skipping CODEX_HOME cleanup: path is outside temp directory"
            );
            return;
        }

        match std::fs::remove_dir_all(codex_home) {
            Ok(()) => {
                tracing::info!(
                    terminal_id = %terminal_id,
                    codex_home = %codex_home.display(),
                    "Cleaned up CODEX_HOME directory"
                );
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    codex_home = %codex_home.display(),
                    "CODEX_HOME directory already removed"
                );
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    codex_home = %codex_home.display(),
                    error = %e,
                    "Failed to clean up CODEX_HOME directory"
                );
            }
        }
    }

    /// Attempt to terminate the child process for a tracked terminal.
    ///
    /// Returns `Ok(())` when termination succeeded or the process already exited.
    async fn terminate_tracked_child(
        &self,
        terminal_id: &str,
        tracked: &mut TrackedProcess,
    ) -> anyhow::Result<()> {
        let kill_result = match tracked.child.process_id() {
            Some(pid) if pid > 0 => self.kill(pid).await,
            _ => tracked
                .child
                .kill()
                .map_err(|e| anyhow::anyhow!("Failed to kill terminal {terminal_id}: {e}")),
        };

        match kill_result {
            Ok(()) => Ok(()),
            Err(kill_error) => {
                let exited_already = matches!(tracked.child.try_wait(), Ok(Some(_)));
                if exited_already {
                    tracing::debug!(
                        terminal_id = %terminal_id,
                        "Child process already exited before kill"
                    );
                    Ok(())
                } else {
                    Err(anyhow::anyhow!(
                        "Failed to kill terminal {terminal_id}: {kill_error}"
                    ))
                }
            }
        }
    }

    /// Remove an existing terminal with the same ID before spawning a new one.
    ///
    /// This prevents stale subprocesses/tasks from leaking across workflow rounds.
    async fn evict_existing_terminal(&self, terminal_id: &str) -> anyhow::Result<()> {
        let tracked = {
            let mut processes = self.processes.write().await;
            let Some(existing) = processes.get_mut(terminal_id) else {
                return Ok(());
            };

            self.terminate_tracked_child(terminal_id, existing)
                .await
                .map_err(|e| {
                    anyhow::anyhow!("Failed to evict existing terminal {terminal_id}: {e}")
                })?;

            processes.remove(terminal_id)
        };

        if let Some(tracked) = tracked {
            self.finalize_terminated_process(terminal_id, tracked).await;
            tracing::info!(
                terminal_id = %terminal_id,
                "Evicted existing terminal before spawn"
            );
        }

        Ok(())
    }

    async fn finalize_terminated_process(&self, terminal_id: &str, mut tracked: TrackedProcess) {
        Self::stop_reader_task_gracefully(terminal_id, tracked.reader_task.take()).await;
        Self::stop_logger_task_gracefully(
            terminal_id,
            tracked.logger_shutdown_tx.take(),
            tracked.logger_task.take(),
        )
        .await;

        if let Some(codex_home) = tracked.codex_home.take() {
            Self::cleanup_codex_home(terminal_id, &codex_home);
        }
    }

    async fn stop_reader_task_gracefully(terminal_id: &str, task: Option<JoinHandle<()>>) {
        let Some(mut task) = task else {
            return;
        };

        if let Ok(join_result) = tokio::time::timeout(Duration::from_secs(READER_SHUTDOWN_TIMEOUT_SECS), &mut task)
            .await {
            if let Err(e) = join_result {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    error = %e,
                    "PTY reader task finished with join error"
                );
            }
        } else {
            task.abort();
            tracing::warn!(
                terminal_id = %terminal_id,
                timeout_secs = READER_SHUTDOWN_TIMEOUT_SECS,
                "PTY reader task graceful shutdown timed out, aborted"
            );
        }
    }

    async fn stop_logger_task_gracefully(
        terminal_id: &str,
        shutdown_tx: Option<oneshot::Sender<()>>,
        task: Option<JoinHandle<()>>,
    ) {
        if let Some(tx) = shutdown_tx {
            let _ = tx.send(());
        }

        let Some(mut task) = task else {
            return;
        };

        if let Ok(join_result) = tokio::time::timeout(
            Duration::from_secs(LOGGER_TASK_SHUTDOWN_TIMEOUT_SECS),
            &mut task,
        )
        .await {
            if let Err(e) = join_result {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    error = %e,
                    "Terminal logger task finished with join error"
                );
            }
        } else {
            task.abort();
            tracing::warn!(
                terminal_id = %terminal_id,
                timeout_secs = LOGGER_TASK_SHUTDOWN_TIMEOUT_SECS,
                "Terminal logger task graceful shutdown timed out, aborted"
            );
        }
    }

    /// Spawn dedicated PTY reader task for output fanout.
    fn spawn_output_reader_task(
        terminal_id: &str,
        mut reader: PtyReader,
        output_fanout: Arc<OutputFanout>,
    ) -> JoinHandle<()> {
        let terminal_id = terminal_id.to_string();
        tokio::task::spawn_blocking(move || {
            let mut decoder = Utf8StreamDecoder::new();
            let mut buf = [0u8; PROCESS_PTY_READ_BUFFER_SIZE];

            loop {
                match reader.0.read(&mut buf) {
                    Ok(0) => {
                        // EOF reached - flush any pending incomplete UTF-8 tail
                        if let Some(tail_text) = decoder.flush_lossy_tail() {
                            let _ = output_fanout.publish(tail_text, 0);
                        }
                        tracing::debug!(
                            terminal_id = %terminal_id,
                            "Background PTY reader reached EOF"
                        );
                        break;
                    }
                    Ok(n) => {
                        let decoded = decoder.decode_chunk(&buf[..n]);
                        if !decoded.text.is_empty() || decoded.dropped_invalid_bytes > 0 {
                            let _ =
                                output_fanout.publish(decoded.text, decoded.dropped_invalid_bytes);
                        }
                    }
                    Err(e) => {
                        tracing::warn!(
                            terminal_id = %terminal_id,
                            error = %e,
                            "Background PTY reader stopped with error"
                        );
                        break;
                    }
                }
            }
        })
    }

    /// Create default output fanout configuration.
    ///
    /// [G09-005] replay_max_chunks is 2x broadcast_capacity so that chunks still
    /// in-flight in the broadcast channel are always available in the replay buffer.
    fn default_output_fanout() -> Arc<OutputFanout> {
        Arc::new(OutputFanout::new(OutputFanoutConfig {
            broadcast_capacity: PROCESS_BROADCAST_CAPACITY,
            replay_max_chunks: PROCESS_REPLAY_MAX_CHUNKS, // 2x broadcast_capacity
            replay_max_bytes: PROCESS_REPLAY_MAX_BYTES,
        }))
    }

    /// Spawns a new terminal process with PTY using SpawnCommand configuration.
    ///
    /// This method provides process-level isolation by:
    /// 1. Removing inherited environment variables (via `env.unset`)
    /// 2. Injecting custom environment variables (via `env.set`)
    /// 3. Passing CLI arguments for runtime configuration
    ///
    /// This approach avoids modifying global configuration files, enabling
    /// multiple workflows to run concurrently without conflicts.
    ///
    /// # Arguments
    ///
    /// * `terminal_id` - Unique identifier for this terminal session
    /// * `config` - Spawn configuration including command, args, working dir, and env
    /// * `cols` - Initial terminal width in columns
    /// * `rows` - Initial terminal height in rows
    ///
    /// # Returns
    ///
    /// Returns a `ProcessHandle` containing the PID and session ID.
    pub async fn spawn_pty_with_config(
        &self,
        terminal_id: &str,
        config: &SpawnCommand,
        cols: u16,
        rows: u16,
    ) -> anyhow::Result<ProcessHandle> {
        self.evict_existing_terminal(terminal_id).await?;

        // Capture CODEX_HOME for cleanup on process exit (and on early failures via guard)
        let codex_home = config.env.set.get("CODEX_HOME").and_then(|value| {
            let trimmed = value.trim();
            if trimmed.is_empty() {
                None
            } else {
                Some(PathBuf::from(trimmed))
            }
        });
        let mut codex_home_guard = CodexHomeGuard::new(terminal_id, codex_home.clone());

        // Create PTY system
        let pty_system = native_pty_system();

        // Configure PTY size
        let size = PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        };

        // Open PTY pair (master + slave)
        let pair = pty_system
            .openpty(size)
            .map_err(|e| anyhow::anyhow!("Failed to open PTY: {e}"))?;

        // Build command
        // On Windows, use cmd.exe /c to run commands so that .cmd/.bat files are found
        #[cfg(windows)]
        let mut cmd = {
            let mut c = CommandBuilder::new("cmd.exe");
            c.arg("/c");
            c.arg(&config.command);
            for arg in &config.args {
                c.arg(arg);
            }
            c
        };
        #[cfg(not(windows))]
        let mut cmd = {
            let mut c = CommandBuilder::new(&config.command);
            for arg in &config.args {
                c.arg(arg);
            }
            c
        };
        cmd.cwd(&config.working_dir);

        // Set environment variables for proper terminal behavior
        cmd.env("TERM", "xterm-256color");
        cmd.env("COLORTERM", "truecolor");

        // UTF-8 encoding for Unix
        #[cfg(unix)]
        {
            cmd.env("LANG", "C.UTF-8");
            cmd.env("LC_ALL", "C.UTF-8");
        }

        // Remove inherited environment variables to prevent parent process pollution
        // This must be done BEFORE setting new values to ensure clean isolation
        for key in &config.env.unset {
            cmd.env_remove(key);
            tracing::debug!(
                terminal_id = %terminal_id,
                key = %key,
                "Removed inherited env var"
            );
        }

        // Inject custom environment variables for process-level isolation
        for (key, value) in &config.env.set {
            cmd.env(key, value);
            // Redact sensitive values in logs
            if SpawnEnv::is_sensitive_key(key) {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    key = %key,
                    "Injected env var [REDACTED]"
                );
            } else {
                tracing::debug!(
                    terminal_id = %terminal_id,
                    key = %key,
                    value = %value,
                    "Injected env var"
                );
            }
        }

        // [G21-001] CODEX_HOME was already captured above (line ~558) and stored in
        // `codex_home_guard`. The duplicate parsing here previously overwrote the
        // guard-protected value. Removed to use the single source of truth.

        // Spawn child process on slave PTY
        let mut child = pair
            .slave
            .spawn_command(cmd)
            .map_err(|e| anyhow::anyhow!("Failed to spawn terminal process: {e}"))?;

        let pid = child.process_id().unwrap_or(0);
        let session_id = Uuid::new_v4().to_string();

        // Wait a short time and check if the process is still alive
        // This catches cases where the command fails immediately (e.g., not found, permission denied)
        tokio::time::sleep(std::time::Duration::from_millis(100)).await;
        match child.try_wait() {
            Ok(Some(status)) => {
                return Err(anyhow::anyhow!(
                    "Terminal process exited immediately with status: {status:?}. The CLI may not be installed correctly."
                ));
            }
            Ok(None) => {
                // Process is still running, good
            }
            Err(e) => {
                return Err(anyhow::anyhow!(
                    "Failed to check terminal process status: {e}"
                ));
            }
        }

        // Initialize output fanout and background reader
        let output_fanout = Self::default_output_fanout();
        let reader_task = match pair.master.try_clone_reader() {
            Ok(reader) => Some(Self::spawn_output_reader_task(
                terminal_id,
                PtyReader(reader),
                Arc::clone(&output_fanout),
            )),
            Err(e) => {
                tracing::warn!(
                    terminal_id = %terminal_id,
                    error = %e,
                    "Failed to initialize background PTY reader for fanout"
                );
                None
            }
        };

        // Store tracked process
        let mut processes = self.processes.write().await;
        processes.insert(
            terminal_id.to_string(),
            TrackedProcess {
                session_id: session_id.clone(),
                child,
                master: Mutex::new(pair.master),
                shared_writer: None,
                codex_home,
                output_fanout,
                reader_task,
                logger_task: None,
                logger_shutdown_tx: None,
            },
        );

        // Disarm the guard - CODEX_HOME cleanup is now managed by TrackedProcess
        codex_home_guard.disarm();

        tracing::info!(
            terminal_id = %terminal_id,
            pid = pid,
            command = %config.command,
            args_count = config.args.len(),
            env_set_count = config.env.set.len(),
            env_unset_count = config.env.unset.len(),
            "PTY process spawned with config successfully"
        );

        Ok(ProcessHandle {
            pid,
            session_id,
            terminal_id: terminal_id.to_string(),
            reader: None,
            writer: None,
        })
    }

    /// Spawns a new terminal process with PTY.
    ///
    /// [G21-008] Delegates to `spawn_pty_with_config` using a default `SpawnCommand`
    /// to eliminate code duplication. The legacy signature is preserved for callers
    /// (tests, timeout tests) that use the simpler (shell, working_dir) form.
    ///
    /// # Arguments
    ///
    /// * `terminal_id` - Unique identifier for this terminal session
    /// * `shell` - The shell command to spawn (e.g., "powershell", "bash")
    /// * `working_dir` - Directory where the process will run
    /// * `cols` - Initial terminal width in columns
    /// * `rows` - Initial terminal height in rows
    ///
    /// # Returns
    ///
    /// Returns a `ProcessHandle` containing the PID and session ID.
    pub async fn spawn_pty(
        &self,
        terminal_id: &str,
        shell: &str,
        working_dir: &Path,
        cols: u16,
        rows: u16,
    ) -> anyhow::Result<ProcessHandle> {
        let config = SpawnCommand::new(shell, working_dir);
        self.spawn_pty_with_config(terminal_id, &config, cols, rows)
            .await
    }

    /// Resize terminal PTY
    ///
    /// # Arguments
    ///
    /// * `terminal_id` - Terminal ID to resize
    /// * `cols` - New width in columns
    /// * `rows` - New height in rows
    pub async fn resize(&self, terminal_id: &str, cols: u16, rows: u16) -> anyhow::Result<()> {
        let processes = self.processes.read().await;

        if let Some(tracked) = processes.get(terminal_id) {
            let size = PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            };

            let master = tracked
                .master
                .lock()
                .map_err(|e| anyhow::anyhow!("Failed to lock PTY master: {e}"))?;

            master
                .resize(size)
                .map_err(|e| anyhow::anyhow!("Failed to resize PTY: {e}"))?;

            tracing::debug!(
                terminal_id = %terminal_id,
                cols = cols,
                rows = rows,
                "PTY resized"
            );

            Ok(())
        } else {
            Err(anyhow::anyhow!("Terminal not found: {terminal_id}"))
        }
    }

    /// Terminates a process by its PID
    ///
    /// Sends a termination signal to the process with the given PID.
    /// On Unix, sends SIGTERM. On Windows, uses taskkill /F.
    pub async fn kill(&self, pid: u32) -> anyhow::Result<()> {
        // Safety check: PID 0 is invalid and could cause unintended behavior
        if pid == 0 {
            return Err(anyhow::anyhow!(
                "Invalid PID 0: cannot kill process with PID 0"
            ));
        }

        #[cfg(unix)]
        {
            use nix::{
                sys::signal::{self, Signal},
                unistd::Pid,
            };
            signal::kill(Pid::from_raw(pid as i32), Signal::SIGTERM)
                .map_err(|e| anyhow::anyhow!("Failed to kill process {pid}: {e}"))?;
        }

        #[cfg(windows)]
        {
            tokio::task::spawn_blocking(move || {
                use std::os::windows::process::CommandExt;
                // CREATE_NO_WINDOW
                let _ = std::process::Command::new("cmd.exe")
                    .creation_flags(0x0800_0000)
                    .args(["/c", &format!("taskkill /PID {pid}")])
                    .output();

                // Wait up to 3 s for the process to exit gracefully.
                let deadline = std::time::Instant::now() + std::time::Duration::from_secs(3);
                let exited_gracefully = loop {
                    if std::time::Instant::now() >= deadline {
                        break false;
                    }
                    // Poll via tasklist; if the PID is gone the process exited.
                    let check = std::process::Command::new("tasklist")
                        .args(["/FI", &format!("PID eq {pid}"), "/NH"])
                        .output();
                    if let Ok(out) = check {
                        let stdout = String::from_utf8_lossy(&out.stdout);
                        if !stdout.contains(&pid.to_string()) {
                            break true;
                        }
                    }
                    std::thread::sleep(std::time::Duration::from_millis(200));
                };

                if !exited_gracefully {
                    // Force-kill with the process tree flag.
                    let output = std::process::Command::new("taskkill")
                        .args(["/PID", &pid.to_string(), "/T", "/F"])
                        .output()
                        .map_err(|e| anyhow::anyhow!("Failed to execute taskkill: {e}"))?;

                    if !output.status.success() {
                        let stderr = String::from_utf8_lossy(&output.stderr);
                        return Err(anyhow::anyhow!("taskkill failed: {stderr}"));
                    }
                }

                Ok(())
            }).await.map_err(|e| anyhow::anyhow!("spawn_blocking join error: {e}"))??;
        }

        Ok(())
    }

    /// Kill terminal by terminal ID
    pub async fn kill_terminal(&self, terminal_id: &str) -> anyhow::Result<()> {
        let tracked = {
            let mut processes = self.processes.write().await;
            let tracked = processes
                .get_mut(terminal_id)
                .ok_or_else(|| anyhow::anyhow!("Terminal not found: {terminal_id}"))?;

            self.terminate_tracked_child(terminal_id, tracked).await?;
            processes
                .remove(terminal_id)
                .ok_or_else(|| anyhow::anyhow!("Terminal not found after kill: {terminal_id}"))?
        };

        self.finalize_terminated_process(terminal_id, tracked).await;

        tracing::info!(terminal_id = %terminal_id, "Terminal killed");
        Ok(())
    }

    /// Check if a terminal process is running.
    ///
    /// [G21-002] In addition to HashMap presence, calls `try_wait()` on the child
    /// to detect processes that have already exited but have not yet been reaped by
    /// `cleanup()`. This ensures callers get accurate liveness information without
    /// waiting for the next cleanup poll cycle.
    pub async fn is_running(&self, terminal_id: &str) -> bool {
        let mut processes = self.processes.write().await;
        let Some(tracked) = processes.get_mut(terminal_id) else {
            return false;
        };
        // Try to reap the child non-blockingly; if it has exited, treat as not running.
        match tracked.child.try_wait() {
            Ok(Some(_)) => false, // process has exited
            Ok(None) => true,     // process is still running
            Err(_) => true,       // cannot determine; assume still running to be safe
        }
    }

    /// Lists all currently tracked terminal IDs
    pub async fn list_running(&self) -> Vec<String> {
        let mut processes = self.processes.write().await;
        let mut running = Vec::new();
        for (id, tracked) in processes.iter_mut() {
            match tracked.child.try_wait() {
                Ok(Some(_)) => {} // exited
                _ => running.push(id.clone()),
            }
        }
        running
    }

    /// Removes dead processes from tracking
    pub async fn cleanup(&self) {
        let cleanup_targets = {
            let mut processes = self.processes.write().await;

            let dead_ids: Vec<String> = processes
                .iter_mut()
                .filter_map(|(id, tracked)| match tracked.child.try_wait() {
                    Ok(Some(_)) => Some(id.clone()),
                    _ => None,
                })
                .collect();

            let mut targets = Vec::new();
            for id in dead_ids {
                if let Some(tracked) = processes.remove(&id) {
                    targets.push((id, tracked));
                }
            }

            targets
        };

        for (id, tracked) in cleanup_targets {
            self.finalize_terminated_process(&id, tracked).await;
            tracing::debug!(terminal_id = %id, "Removed dead process from tracking");
        }
    }

    /// Get process handle by terminal ID
    ///
    /// Returns a ProcessHandle containing the PTY reader/writer for the terminal process.
    /// This method supports multiple calls for WebSocket reconnection scenarios:
    /// - Reader is cloned on each call (portable-pty supports multiple readers)
    /// - Writer is shared via Arc<Mutex> (initialized on first call, then reused)
    pub async fn get_handle(&self, terminal_id: &str) -> Option<ProcessHandle> {
        let mut processes = self.processes.write().await;

        if let Some(tracked) = processes.get_mut(terminal_id) {
            let session_id = tracked.session_id.clone();

            // Initialize shared writer on first call, then reuse for reconnections
            if tracked.shared_writer.is_none() {
                let master = match tracked.master.lock() {
                    Ok(m) => m,
                    Err(e) => {
                        tracing::error!(
                            terminal_id = %terminal_id,
                            error = %e,
                            "Failed to lock PTY master"
                        );
                        return None;
                    }
                };

                match master.take_writer() {
                    Ok(w) => {
                        tracked.shared_writer = Some(Arc::new(Mutex::new(PtyWriter(w))));
                        tracing::debug!(
                            terminal_id = %terminal_id,
                            "Initialized shared PTY writer"
                        );
                    }
                    Err(e) => {
                        tracing::error!(
                            terminal_id = %terminal_id,
                            error = %e,
                            "Failed to take PTY writer"
                        );
                    }
                }
            }

            // Clone the Arc reference for the caller
            let writer = tracked.shared_writer.as_ref().map(Arc::clone);

            // Get PID from child
            let pid = tracked.child.process_id().unwrap_or(0);

            Some(ProcessHandle {
                pid,
                session_id,
                terminal_id: terminal_id.to_string(),
                // Reader is now owned by background fanout task (single-reader constraint)
                reader: None,
                writer,
            })
        } else {
            None
        }
    }

    /// Subscribe to terminal output stream with replay support.
    ///
    /// - `from_seq = None`: replay retained window from earliest.
    /// - `from_seq = Some(n)`: replay from `n + 1`.
    ///
    /// This enables late subscribers (like PromptWatcher) to receive output
    /// that was emitted before they subscribed, preventing "first-screen prompt loss".
    pub async fn subscribe_output(
        &self,
        terminal_id: &str,
        from_seq: Option<u64>,
    ) -> anyhow::Result<OutputSubscription> {
        let processes = self.processes.read().await;
        let tracked = processes
            .get(terminal_id)
            .ok_or_else(|| anyhow::anyhow!("Terminal not found: {terminal_id}"))?;
        Ok(tracked.output_fanout.subscribe(from_seq))
    }

    /// Get latest emitted output sequence for a terminal.
    ///
    /// Returns None if terminal doesn't exist, or 0 if no output has been emitted yet.
    pub async fn latest_output_seq(&self, terminal_id: &str) -> Option<u64> {
        let processes = self.processes.read().await;
        processes
            .get(terminal_id)
            .map(|tracked| tracked.output_fanout.latest_seq())
    }

    /// Attach a persistent logger to terminal output fanout.
    ///
    /// This wires PTY output to `terminal_log` table persistence.
    pub async fn attach_terminal_logger(
        &self,
        db: Arc<DBService>,
        terminal_id: &str,
        log_type: &str,
        flush_interval_secs: u64,
    ) -> anyhow::Result<()> {
        {
            let processes = self.processes.read().await;
            if !processes.contains_key(terminal_id) {
                return Err(anyhow::anyhow!("Terminal not found: {terminal_id}"));
            }
        }

        let mut subscription = self.subscribe_output(terminal_id, None).await?;
        let (shutdown_tx, mut shutdown_rx) = oneshot::channel::<()>();
        let logger = TerminalLogger::new(
            db,
            terminal_id.to_string(),
            log_type.to_string(),
            flush_interval_secs,
        );
        let terminal_id_owned = terminal_id.to_string();
        let log_type_owned = log_type.to_string();
        let processes = Arc::clone(&self.processes);

        let logger_for_shutdown = logger.clone();
        let logger_task = tokio::spawn(async move {
            loop {
                tokio::select! {
                    _ = &mut shutdown_rx => {
                        tracing::debug!(
                            terminal_id = %terminal_id_owned,
                            "Terminal logger received shutdown signal"
                        );
                        break;
                    }
                    recv_result = subscription.recv() => {
                        match recv_result {
                            Ok(chunk) => {
                                if !chunk.text.is_empty() {
                                    logger.append(&chunk.text).await;
                                }
                                if chunk.dropped_invalid_bytes > 0 {
                                    logger
                                        .append(&format!(
                                            "[{}] dropped {} invalid UTF-8 bytes at seq={}",
                                            log_type_owned, chunk.dropped_invalid_bytes, chunk.seq
                                        ))
                                        .await;
                                }
                            }
                            Err(tokio::sync::broadcast::error::RecvError::Lagged(skipped)) => {
                                let resume_from = subscription.last_seq();
                                tracing::warn!(
                                    terminal_id = %terminal_id_owned,
                                    skipped = %skipped,
                                    resume_from_seq = resume_from,
                                    "Terminal logger output subscription lagged; attempting replay recovery"
                                );

                                logger
                                    .append(&format!(
                                        "[{log_type_owned}] output stream lagged: skipped={skipped} replay_from_seq={resume_from}"
                                    ))
                                    .await;

                                let recovered_subscription = {
                                    let tracked = processes.read().await;
                                    tracked
                                        .get(&terminal_id_owned)
                                        .map(|process| process.output_fanout.subscribe(Some(resume_from)))
                                };

                                if let Some(new_subscription) = recovered_subscription {
                                    subscription = new_subscription;
                                    tracing::info!(
                                        terminal_id = %terminal_id_owned,
                                        resume_from_seq = resume_from,
                                        "Terminal logger subscription recovered after lag"
                                    );
                                } else {
                                    tracing::warn!(
                                        terminal_id = %terminal_id_owned,
                                        "Terminal logger lag recovery aborted: terminal not found"
                                    );
                                    break;
                                }
                            }
                            Err(tokio::sync::broadcast::error::RecvError::Closed) => {
                                tracing::debug!(
                                    terminal_id = %terminal_id_owned,
                                    "Terminal logger output subscription closed"
                                );
                                break;
                            }
                        }
                    }
                }
            }

            logger_for_shutdown.stop_flush_task().await;
            if let Err(e) = logger_for_shutdown.flush().await {
                tracing::warn!(
                    terminal_id = %terminal_id_owned,
                    error = %e,
                    "Terminal logger final flush failed"
                );
            }
        });

        let (existing_shutdown, existing_task) = {
            let mut processes = self.processes.write().await;
            let tracked = if let Some(tracked) = processes.get_mut(terminal_id) { tracked } else {
                drop(processes);
                Self::stop_logger_task_gracefully(
                    terminal_id,
                    Some(shutdown_tx),
                    Some(logger_task),
                )
                .await;
                return Err(anyhow::anyhow!("Terminal not found: {terminal_id}"));
            };
            let existing_shutdown = tracked.logger_shutdown_tx.take();
            let existing_task = tracked.logger_task.take();
            tracked.logger_shutdown_tx = Some(shutdown_tx);
            tracked.logger_task = Some(logger_task);
            (existing_shutdown, existing_task)
        };
        Self::stop_logger_task_gracefully(terminal_id, existing_shutdown, existing_task).await;
        tracing::debug!(terminal_id = %terminal_id, log_type = %log_type, "Attached terminal logger to output fanout");
        Ok(())
    }
}

impl Default for ProcessManager {
    fn default() -> Self {
        Self::new()
    }
}

/// [G21-006] Best-effort cleanup of all tracked child processes on drop.
///
/// We cannot perform async operations (task joins, CODEX_HOME cleanup) here, so we
/// only send termination signals. Structured shutdown should call `kill_terminal` for
/// each process before dropping the manager; this `Drop` impl is the last-resort guard.
impl Drop for ProcessManager {
    fn drop(&mut self) {
        // Try to get a synchronous snapshot of tracked processes.
        // If the RwLock is contended we skip cleanup to avoid deadlock.
        let processes = match self.processes.try_read() {
            Ok(guard) => guard,
            Err(_) => {
                tracing::warn!("ProcessManager dropped while processes lock is held; skipping cleanup");
                return;
            }
        };

        for (terminal_id, tracked) in processes.iter() {
            if let Some(pid) = tracked.child.process_id() {
                if pid > 0 {
                    #[cfg(unix)]
                    {
                        use nix::{sys::signal::{self, Signal}, unistd::Pid};
                        let _ = signal::kill(Pid::from_raw(pid as i32), Signal::SIGTERM);
                    }
                    #[cfg(windows)]
                    {
                        let _ = std::process::Command::new("taskkill")
                            .args(["/PID", &pid.to_string(), "/T", "/F"])
                            .output();
                    }
                    tracing::debug!(
                        terminal_id = %terminal_id,
                        pid = pid,
                        "ProcessManager::drop: sent termination signal to child"
                    );
                }
            }
        }
    }
}

// ============================================================================
// Terminal Logger
// ============================================================================

/// Batch logger for terminal output
///
/// Batches log lines and flushes them every second to reduce I/O overhead.
pub const DEFAULT_MAX_BUFFER_SIZE: usize = 1000;

/// [G09-006] Maximum buffer byte size before forced flush/truncation (10 MiB).
/// Prevents unbounded memory growth when the flush worker is slower than output.
const MAX_BUFFER_BYTES: usize = 10 * 1024 * 1024;

pub struct TerminalLogger {
    buffer: Arc<RwLock<Vec<String>>>,
    /// Approximate byte size of buffered entries (tracked to avoid O(n) recount).
    buffer_bytes: Arc<std::sync::atomic::AtomicUsize>,
    flush_lock: Arc<AsyncMutex<()>>,
    flush_interval_secs: u64,
    max_buffer_size: usize,
    db: Arc<DBService>,
    terminal_id: String,
    log_type: String,
    flush_task: Arc<Mutex<Option<JoinHandle<()>>>>,
    flush_shutdown_tx: Arc<Mutex<Option<oneshot::Sender<()>>>>,
    persistence_disabled: Arc<AtomicBool>,
}

impl Clone for TerminalLogger {
    fn clone(&self) -> Self {
        Self {
            buffer: Arc::clone(&self.buffer),
            buffer_bytes: Arc::clone(&self.buffer_bytes),
            flush_lock: Arc::clone(&self.flush_lock),
            flush_interval_secs: self.flush_interval_secs,
            max_buffer_size: self.max_buffer_size,
            db: Arc::clone(&self.db),
            terminal_id: self.terminal_id.clone(),
            log_type: self.log_type.clone(),
            flush_task: Arc::clone(&self.flush_task),
            flush_shutdown_tx: Arc::clone(&self.flush_shutdown_tx),
            persistence_disabled: Arc::clone(&self.persistence_disabled),
        }
    }
}

impl Drop for TerminalLogger {
    fn drop(&mut self) {
        if Arc::strong_count(&self.flush_shutdown_tx) != 1 {
            return;
        }

        match self.flush_shutdown_tx.lock() {
            Ok(mut shutdown_tx) => {
                if let Some(tx) = shutdown_tx.take() {
                    let _ = tx.send(());
                }
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %self.terminal_id,
                    log_type = %self.log_type,
                    error = %e,
                    "Terminal logger flush shutdown lock poisoned on drop"
                );
            }
        }
    }
}

impl TerminalLogger {
    pub fn new(
        db: Arc<DBService>,
        terminal_id: impl Into<String>,
        log_type: impl Into<String>,
        flush_interval_secs: u64,
    ) -> Self {
        Self::with_max_buffer_size(
            db,
            terminal_id,
            log_type,
            flush_interval_secs,
            DEFAULT_MAX_BUFFER_SIZE,
        )
    }

    pub fn with_max_buffer_size(
        db: Arc<DBService>,
        terminal_id: impl Into<String>,
        log_type: impl Into<String>,
        flush_interval_secs: u64,
        max_buffer_size: usize,
    ) -> Self {
        Self {
            buffer: Arc::new(RwLock::new(Vec::new())),
            buffer_bytes: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            flush_lock: Arc::new(AsyncMutex::new(())),
            flush_interval_secs,
            max_buffer_size: max_buffer_size.max(1),
            db,
            terminal_id: terminal_id.into(),
            log_type: log_type.into(),
            flush_task: Arc::new(Mutex::new(None)),
            flush_shutdown_tx: Arc::new(Mutex::new(None)),
            persistence_disabled: Arc::new(AtomicBool::new(false)),
        }
        .start_flush_task()
    }

    async fn persist_entries(
        db: &DBService,
        terminal_id: &str,
        log_type: &str,
        entries: &[String],
    ) -> anyhow::Result<()> {
        if entries.is_empty() {
            return Ok(());
        }

        let mut tx = db.pool.begin().await?;
        for line in entries {
            sqlx::query(
                r"
                INSERT INTO terminal_log (id, terminal_id, log_type, content, created_at)
                VALUES (?1, ?2, ?3, ?4, ?5)
                ",
            )
            .bind(Uuid::new_v4().to_string())
            .bind(terminal_id)
            .bind(log_type)
            .bind(line)
            .bind(chrono::Utc::now())
            .execute(&mut *tx)
            .await?;
        }
        tx.commit().await?;
        Ok(())
    }

    async fn flush_buffer(
        buffer: &Arc<RwLock<Vec<String>>>,
        flush_lock: &Arc<AsyncMutex<()>>,
        db: &DBService,
        terminal_id: &str,
        log_type: &str,
        persistence_disabled: &Arc<AtomicBool>,
    ) -> anyhow::Result<()> {
        let _flush_guard = flush_lock.lock().await;

        if persistence_disabled.load(Ordering::Relaxed) {
            let mut buffer = buffer.write().await;
            buffer.clear();
            return Ok(());
        }

        // [G21-009] Use mem::take to atomically drain the buffer, preventing TOCTOU
        // races where new entries could be appended between the read and drain steps.
        let entries = {
            let mut buffer = buffer.write().await;
            if buffer.is_empty() {
                return Ok(());
            }
            std::mem::take(&mut *buffer)
        };

        if let Err(error) = Self::persist_entries(db, terminal_id, log_type, &entries).await {
            if Self::is_sqlite_foreign_key_violation(&error) {
                let first_disable = !persistence_disabled.swap(true, Ordering::Relaxed);
                if first_disable {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        log_type = %log_type,
                        dropped_entries = entries.len(),
                        error = %error,
                        "Terminal logger disabled after foreign-key violation (SQLite code 787)"
                    );
                }
                return Ok(());
            }
            return Err(error);
        }

        Ok(())
    }

    fn start_flush_task(self) -> Self {
        let buffer = Arc::clone(&self.buffer);
        let buffer_bytes = Arc::clone(&self.buffer_bytes);
        let flush_lock = Arc::clone(&self.flush_lock);
        let interval_secs = self.flush_interval_secs;
        let db = Arc::clone(&self.db);
        let terminal_id = self.terminal_id.clone();
        let log_type = self.log_type.clone();
        let persistence_disabled = Arc::clone(&self.persistence_disabled);
        let (flush_shutdown_tx, mut flush_shutdown_rx) = oneshot::channel::<()>();
        let flush_task = tokio::spawn(async move {
            let mut interval =
                tokio::time::interval(tokio::time::Duration::from_secs(interval_secs));
            loop {
                tokio::select! {
                    _ = &mut flush_shutdown_rx => {
                        if let Err(e) =
                            Self::flush_buffer(&buffer, &flush_lock, &db, &terminal_id, &log_type, &persistence_disabled).await
                        {
                            tracing::warn!(
                                terminal_id = %terminal_id,
                                log_type = %log_type,
                                error = %e,
                                "Failed to persist terminal logs during flush task shutdown"
                            );
                        }
                        buffer_bytes.store(0, Ordering::Relaxed);
                        tracing::debug!(
                            terminal_id = %terminal_id,
                            log_type = %log_type,
                            "Terminal logger flush task received shutdown signal"
                        );
                        break;
                    }
                    _ = interval.tick() => {
                        if let Err(e) =
                            Self::flush_buffer(&buffer, &flush_lock, &db, &terminal_id, &log_type, &persistence_disabled).await
                        {
                            tracing::error!(
                                terminal_id = %terminal_id,
                                log_type = %log_type,
                                error = %e,
                                "Failed to persist terminal logs in flush task"
                            );
                        }
                        buffer_bytes.store(0, Ordering::Relaxed);
                    }
                }
            }
        });
        match self.flush_shutdown_tx.lock() {
            Ok(mut shutdown_tx) => {
                *shutdown_tx = Some(flush_shutdown_tx);
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %self.terminal_id,
                    log_type = %self.log_type,
                    error = %e,
                    "Terminal logger flush shutdown slot lock poisoned"
                );
            }
        }
        match self.flush_task.lock() {
            Ok(mut task) => {
                *task = Some(flush_task);
            }
            Err(e) => {
                flush_task.abort();
                tracing::warn!(
                    terminal_id = %self.terminal_id,
                    log_type = %self.log_type,
                    error = %e,
                    "Terminal logger flush task slot lock poisoned; task aborted"
                );
            }
        }

        self
    }

    async fn stop_flush_task(&self) {
        match self.flush_shutdown_tx.lock() {
            Ok(mut shutdown_tx) => {
                if let Some(tx) = shutdown_tx.take() {
                    let _ = tx.send(());
                }
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %self.terminal_id,
                    log_type = %self.log_type,
                    error = %e,
                    "Terminal logger flush shutdown slot lock poisoned"
                );
            }
        }

        let task = match self.flush_task.lock() {
            Ok(mut task) => task.take(),
            Err(e) => {
                tracing::warn!(
                    terminal_id = %self.terminal_id,
                    log_type = %self.log_type,
                    error = %e,
                    "Terminal logger flush task slot lock poisoned"
                );
                None
            }
        };
        let Some(mut task) = task else {
            return;
        };

        if let Ok(join_result) = tokio::time::timeout(Duration::from_secs(LOGGER_SHUTDOWN_TIMEOUT_SECS), &mut task)
            .await {
            if let Err(e) = join_result {
                tracing::warn!(
                    terminal_id = %self.terminal_id,
                    log_type = %self.log_type,
                    error = %e,
                    "Terminal logger flush task finished with join error"
                );
            }
        } else {
            task.abort();
            tracing::warn!(
                terminal_id = %self.terminal_id,
                log_type = %self.log_type,
                timeout_secs = LOGGER_SHUTDOWN_TIMEOUT_SECS,
                "Terminal logger flush task graceful shutdown timed out, aborted"
            );
        }
    }

    pub async fn append(&self, line: &str) {
        if self.persistence_disabled.load(Ordering::Relaxed) {
            return;
        }

        let should_flush = {
            let mut buffer = self.buffer.write().await;
            let line_bytes = line.len();
            buffer.push(line.to_string());
            let current_bytes = self.buffer_bytes.fetch_add(line_bytes, Ordering::Relaxed) + line_bytes;
            // [G09-006] Flush when entry count OR byte size exceeds limits
            buffer.len() >= self.max_buffer_size || current_bytes >= MAX_BUFFER_BYTES
        };

        if !should_flush {
            return;
        }

        if let Err(e) = self.flush_and_reset_bytes().await {
            tracing::error!(
                terminal_id = %self.terminal_id,
                log_type = %self.log_type,
                error = %e,
                "Failed to persist terminal logs in append-triggered flush"
            );
        }
    }

    pub async fn flush(&self) -> anyhow::Result<()> {
        self.flush_and_reset_bytes().await
    }

    /// Flush buffer and reset the byte counter.
    async fn flush_and_reset_bytes(&self) -> anyhow::Result<()> {
        let result = Self::flush_buffer(
            &self.buffer,
            &self.flush_lock,
            &self.db,
            &self.terminal_id,
            &self.log_type,
            &self.persistence_disabled,
        )
        .await;
        // Reset byte counter after flush (buffer was drained by flush_buffer via mem::take)
        self.buffer_bytes.store(0, Ordering::Relaxed);
        result
    }

    fn is_sqlite_foreign_key_violation(error: &anyhow::Error) -> bool {
        error.chain().any(|cause| {
            let Some(sqlx_error) = cause.downcast_ref::<sqlx::Error>() else {
                return false;
            };
            match sqlx_error {
                sqlx::Error::Database(db_error) => {
                    db_error.code().as_deref() == Some(SQLITE_FOREIGN_KEY_CONSTRAINT_CODE)
                        || db_error
                            .message()
                            .to_ascii_lowercase()
                            .contains("foreign key constraint failed")
                }
                _ => false,
            }
        })
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    fn build_test_spawn_command(working_dir: &Path) -> SpawnCommand {
        #[cfg(windows)]
        {
            SpawnCommand::new("powershell", working_dir).with_args([
                "-NoLogo",
                "-NoProfile",
                "-Command",
                "Start-Sleep -Seconds 120",
            ])
        }
        #[cfg(unix)]
        {
            SpawnCommand::new("sleep", working_dir).with_arg("120")
        }
    }

    async fn setup_logger_test_db_with_terminal(_terminal_id: &str) -> Arc<DBService> {
        use std::str::FromStr;

        use sqlx::sqlite::{SqliteConnectOptions, SqlitePoolOptions};

        let options = SqliteConnectOptions::from_str(":memory:")
            .unwrap()
            .pragma("foreign_keys", "ON");
        let pool = SqlitePoolOptions::new()
            .max_connections(1)
            .connect_with(options)
            .await
            .unwrap();

        let db = Arc::new(DBService { pool });

        sqlx::query(
            "CREATE TABLE terminal_log (
                id TEXT PRIMARY KEY,
                terminal_id TEXT NOT NULL,
                log_type TEXT NOT NULL,
                content TEXT NOT NULL,
                created_at TEXT NOT NULL DEFAULT (datetime('now'))
            )",
        )
        .execute(&db.pool)
        .await
        .unwrap();

        db
    }

    async fn setup_logger_test_db_with_fk_constraint() -> Arc<DBService> {
        use std::str::FromStr;

        use sqlx::sqlite::{SqliteConnectOptions, SqlitePoolOptions};

        let options = SqliteConnectOptions::from_str(":memory:")
            .unwrap()
            .pragma("foreign_keys", "ON");
        let pool = SqlitePoolOptions::new()
            .max_connections(1)
            .connect_with(options)
            .await
            .unwrap();

        let db = Arc::new(DBService { pool });

        sqlx::query(
            "CREATE TABLE terminal (
                id TEXT PRIMARY KEY
            )",
        )
        .execute(&db.pool)
        .await
        .unwrap();

        sqlx::query(
            "CREATE TABLE terminal_log (
                id TEXT PRIMARY KEY,
                terminal_id TEXT NOT NULL REFERENCES terminal(id) ON DELETE CASCADE,
                log_type TEXT NOT NULL,
                content TEXT NOT NULL,
                created_at TEXT NOT NULL DEFAULT (datetime('now'))
            )",
        )
        .execute(&db.pool)
        .await
        .unwrap();

        db
    }

    #[tokio::test]
    async fn test_process_manager_new() {
        let manager = ProcessManager::new();
        let running = manager.list_running().await;
        assert_eq!(running.len(), 0, "New manager should have no processes");
    }

    #[tokio::test]
    async fn test_process_manager_default() {
        let manager = ProcessManager::default();
        let running = manager.list_running().await;
        assert_eq!(running.len(), 0);
    }

    #[tokio::test]
    async fn test_spawn_pty_creates_process() {
        let manager = ProcessManager::new();
        let temp_dir = tempfile::tempdir().unwrap();
        let spawn_config = build_test_spawn_command(temp_dir.path());

        let result = tokio::time::timeout(
            Duration::from_secs(10),
            manager.spawn_pty_with_config("test-terminal", &spawn_config, 80, 24),
        )
        .await
        .expect("spawn_pty_with_config should not hang");

        assert!(result.is_ok(), "Spawn should succeed: {:?}", result.err());
        let handle = result.unwrap();
        assert_eq!(handle.terminal_id, "test-terminal");
        assert!(!handle.session_id.is_empty());

        // Cleanup
        let _ = tokio::time::timeout(
            Duration::from_secs(10),
            manager.kill_terminal("test-terminal"),
        )
        .await;
    }

    #[tokio::test]
    async fn test_get_handle_returns_pty_handles() {
        let manager = ProcessManager::new();
        let temp_dir = tempfile::tempdir().unwrap();
        let spawn_config = build_test_spawn_command(temp_dir.path());

        tokio::time::timeout(
            Duration::from_secs(10),
            manager.spawn_pty_with_config("test-terminal", &spawn_config, 80, 24),
        )
        .await
        .expect("spawn_pty_with_config should not hang")
        .unwrap();

        // In new architecture, reader is owned by background fanout task
        // get_handle() returns reader: None, writer: Some
        let handle1 = manager.get_handle("test-terminal").await;
        assert!(handle1.is_some());
        let handle1 = handle1.unwrap();
        assert!(
            handle1.reader.is_none(),
            "Reader should be None (owned by fanout task)"
        );
        assert!(handle1.writer.is_some());

        // Second call should also return handle with shared writer
        let handle2 = manager.get_handle("test-terminal").await;
        assert!(handle2.is_some());
        let handle2 = handle2.unwrap();
        assert!(
            handle2.reader.is_none(),
            "Reader should be None (owned by fanout task)"
        );
        assert!(handle2.writer.is_some());

        assert_eq!(
            handle1.session_id, handle2.session_id,
            "Session ID should remain stable across reconnections"
        );

        // Verify that writers are the same Arc (shared)
        let writer1 = handle1.writer.as_ref().unwrap();
        let writer2 = handle2.writer.as_ref().unwrap();
        assert!(
            Arc::ptr_eq(writer1, writer2),
            "Writers should be shared via Arc"
        );

        // Verify subscribe_output works (new API for reading output)
        let subscription = manager.subscribe_output("test-terminal", None).await;
        assert!(
            subscription.is_ok(),
            "Should be able to subscribe to output"
        );

        // Cleanup
        let _ = tokio::time::timeout(
            Duration::from_secs(10),
            manager.kill_terminal("test-terminal"),
        )
        .await;
    }

    #[tokio::test]
    async fn test_terminal_logger_flush_manual_flush_persists() {
        let terminal_id = Uuid::new_v4().to_string();
        let db = setup_logger_test_db_with_terminal(&terminal_id).await;

        let logger = TerminalLogger::with_max_buffer_size(
            Arc::clone(&db),
            terminal_id.clone(),
            "stdout",
            60,
            100,
        );

        logger.append("tail-line").await;
        logger.flush().await.unwrap();

        let rows: Vec<String> = sqlx::query_scalar(
            "SELECT content FROM terminal_log WHERE terminal_id = ? ORDER BY created_at ASC",
        )
        .bind(&terminal_id)
        .fetch_all(&db.pool)
        .await
        .unwrap();

        assert!(rows.iter().any(|row| row == "tail-line"));
    }

    #[tokio::test]
    async fn test_terminal_logger_flush_when_buffer_reaches_limit() {
        let terminal_id = Uuid::new_v4().to_string();
        let db = setup_logger_test_db_with_terminal(&terminal_id).await;

        let logger = TerminalLogger::with_max_buffer_size(
            Arc::clone(&db),
            terminal_id.clone(),
            "stdout",
            60,
            1,
        );

        logger.append("limit-triggered-line").await;
        logger.flush().await.unwrap();

        let rows: Vec<String> = sqlx::query_scalar(
            "SELECT content FROM terminal_log WHERE terminal_id = ? ORDER BY created_at ASC",
        )
        .bind(&terminal_id)
        .fetch_all(&db.pool)
        .await
        .unwrap();

        assert!(rows.iter().any(|row| row == "limit-triggered-line"));
    }

    #[tokio::test]
    async fn test_terminal_logger_stop_flush_task_clears_worker_handle() {
        let terminal_id = Uuid::new_v4().to_string();
        let db = setup_logger_test_db_with_terminal(&terminal_id).await;

        let logger = TerminalLogger::with_max_buffer_size(
            Arc::clone(&db),
            terminal_id.clone(),
            "stdout",
            60,
            100,
        );

        logger.append("stoppable-line").await;
        logger.stop_flush_task().await;
        logger.flush().await.unwrap();

        let rows: Vec<String> = sqlx::query_scalar(
            "SELECT content FROM terminal_log WHERE terminal_id = ? ORDER BY created_at ASC",
        )
        .bind(&terminal_id)
        .fetch_all(&db.pool)
        .await
        .unwrap();

        assert!(rows.iter().any(|row| row == "stoppable-line"));
        assert!(logger.flush_task.lock().unwrap().is_none());
    }

    #[tokio::test]
    async fn test_terminal_logger_fk_787_disables_persistence_and_clears_buffer() {
        let terminal_id = Uuid::new_v4().to_string();
        let db = setup_logger_test_db_with_fk_constraint().await;

        let logger = TerminalLogger::with_max_buffer_size(
            Arc::clone(&db),
            terminal_id.clone(),
            "stdout",
            60,
            100,
        );

        logger.append("line-before-fk").await;
        logger.flush().await.unwrap();

        assert!(logger.persistence_disabled.load(Ordering::Relaxed));
        assert!(logger.buffer.read().await.is_empty());

        let row_count: i64 =
            sqlx::query_scalar("SELECT COUNT(*) FROM terminal_log WHERE terminal_id = ?")
                .bind(&terminal_id)
                .fetch_one(&db.pool)
                .await
                .unwrap();
        assert_eq!(row_count, 0);

        logger.append("line-after-disabled").await;
        logger.flush().await.unwrap();
        assert!(logger.buffer.read().await.is_empty());

        let row_count_after_disabled: i64 =
            sqlx::query_scalar("SELECT COUNT(*) FROM terminal_log WHERE terminal_id = ?")
                .bind(&terminal_id)
                .fetch_one(&db.pool)
                .await
                .unwrap();
        assert_eq!(row_count_after_disabled, 0);
    }

    #[tokio::test]
    async fn test_kill_terminal_nonexistent_returns_error() {
        let manager = ProcessManager::new();
        let result = manager.kill_terminal("missing-terminal").await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_resize_pty() {
        let manager = ProcessManager::new();
        let temp_dir = tempfile::tempdir().unwrap();
        let spawn_config = build_test_spawn_command(temp_dir.path());

        tokio::time::timeout(
            Duration::from_secs(10),
            manager.spawn_pty_with_config("test-terminal", &spawn_config, 80, 24),
        )
        .await
        .expect("spawn_pty_with_config should not hang")
        .unwrap();

        // Resize should succeed
        let result = manager.resize("test-terminal", 120, 40).await;
        assert!(result.is_ok());

        // Cleanup
        let _ = tokio::time::timeout(
            Duration::from_secs(10),
            manager.kill_terminal("test-terminal"),
        )
        .await;
    }

    #[tokio::test]
    async fn test_get_handle_for_nonexistent_terminal() {
        let manager = ProcessManager::new();
        let handle = manager.get_handle("non-existent").await;
        assert!(handle.is_none());
    }
}
