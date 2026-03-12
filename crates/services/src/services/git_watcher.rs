//! GitWatcher Service
//!
//! Monitors git repositories for commits containing workflow metadata
//! and publishes events to the message bus.

use std::{
    path::{Path, PathBuf},
    sync::{
        Arc,
        atomic::{AtomicBool, Ordering},
    },
    time::Duration,
};

use anyhow::{Context, Result, anyhow};
use serde::{Deserialize, Serialize};
use tokio::sync::Mutex;

use crate::services::orchestrator::{
    CodeIssue, CommitMetadata as OrchestratorCommitMetadata, MessageBus, TerminalCompletionEvent,
    TerminalCompletionStatus,
};

/// Configuration for GitWatcher
#[derive(Clone, Debug)]
pub struct GitWatcherConfig {
    /// Path to the git repository to watch
    pub repo_path: PathBuf,
    /// Polling interval in milliseconds
    pub poll_interval_ms: u64,
}

impl GitWatcherConfig {
    /// Create a new GitWatcherConfig
    pub fn new(repo_path: PathBuf, poll_interval_ms: u64) -> Self {
        Self {
            repo_path,
            poll_interval_ms,
        }
    }
}

/// Parsed metadata from a commit message
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommitMetadata {
    pub workflow_id: String,
    pub task_id: String,
    pub terminal_id: String,
    #[serde(default)]
    pub terminal_order: i32,
    #[serde(default)]
    pub cli: String,
    #[serde(default)]
    pub model: String,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub severity: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reviewed_terminal: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub issues: Option<Vec<CodeIssue>>,
    #[serde(default)]
    pub next_action: String,
}

impl CommitMetadata {
    /// Parse metadata from a commit message
    ///
    /// Expected format:
    /// ```text
    /// Commit message summary
    ///
    /// Optional body
    ///
    /// ---METADATA---
    /// workflow_id: xxx
    /// task_id: xxx
    /// terminal_id: xxx
    /// status: xxx
    /// ...
    /// ```
    pub fn parse(message: &str) -> Option<Self> {
        // Find the metadata section
        let metadata_start = message.find("---METADATA---")?;
        let metadata_section = &message[metadata_start + "---METADATA---".len()..];

        let mut metadata = CommitMetadata {
            workflow_id: String::new(),
            task_id: String::new(),
            terminal_id: String::new(),
            terminal_order: 0,
            cli: String::new(),
            model: String::new(),
            status: String::new(),
            severity: None,
            reviewed_terminal: None,
            issues: None,
            next_action: String::from("continue"),
        };

        // Parse each line in the metadata section
        for line in metadata_section.lines() {
            let line = line.trim();
            if let Some(pos) = line.find(':') {
                let key = &line[..pos].trim();
                let value = &line[pos + 1..].trim();

                match *key {
                    "workflow_id" => metadata.workflow_id = (*value).to_string(),
                    "task_id" => metadata.task_id = (*value).to_string(),
                    "terminal_id" => metadata.terminal_id = (*value).to_string(),
                    "terminal_order" => {
                        metadata.terminal_order = value.parse().unwrap_or(0);
                    }
                    "cli" => metadata.cli = (*value).to_string(),
                    "model" => metadata.model = (*value).to_string(),
                    "status" => metadata.status = (*value).to_string(),
                    "severity" => metadata.severity = Some((*value).to_string()),
                    "reviewed_terminal" => metadata.reviewed_terminal = Some((*value).to_string()),
                    "issues" => {
                        metadata.issues = serde_json::from_str(value).ok();
                    }
                    "next_action" => metadata.next_action = (*value).to_string(),
                    _ => {}
                }
            }
        }

        // Validate required fields
        if metadata.workflow_id.is_empty()
            || metadata.task_id.is_empty()
            || metadata.terminal_id.is_empty()
            || metadata.status.is_empty()
        {
            return None;
        }

        Some(metadata)
    }

    /// Convert to orchestrator CommitMetadata
    pub fn to_orchestrator_metadata(&self) -> OrchestratorCommitMetadata {
        OrchestratorCommitMetadata {
            workflow_id: self.workflow_id.clone(),
            task_id: self.task_id.clone(),
            terminal_id: self.terminal_id.clone(),
            terminal_order: self.terminal_order,
            cli: self.cli.clone(),
            model: self.model.clone(),
            status: self.status.clone(),
            severity: self.severity.clone(),
            reviewed_terminal: self.reviewed_terminal.clone(),
            issues: self.issues.clone(),
            next_action: self.next_action.clone(),
        }
    }
}

/// Represents a single commit with its metadata
#[derive(Debug, Clone)]
pub struct ParsedCommit {
    pub hash: String,
    pub branch: String,
    pub message: String,
    pub metadata: Option<CommitMetadata>,
}

/// GitWatcher service for monitoring git repositories
pub struct GitWatcher {
    config: GitWatcherConfig,
    message_bus: MessageBus,
    /// The workflow ID this watcher is associated with (for GitEvent publishing)
    workflow_id: Option<String>,
    last_commit_hash: Arc<Mutex<Option<String>>>,
    /// Thread-safe running flag for graceful shutdown
    is_running: AtomicBool,
}

impl GitWatcher {
    /// Create a new GitWatcher
    pub fn new(config: GitWatcherConfig, message_bus: MessageBus) -> Result<Self> {
        // Validate path exists
        if !config.repo_path.exists() {
            return Err(anyhow::anyhow!(
                "Repository path does not exist: {}",
                config.repo_path.display()
            ));
        }

        // Validate it's a directory
        if !config.repo_path.is_dir() {
            return Err(anyhow::anyhow!(
                "Repository path is not a directory: {}",
                config.repo_path.display()
            ));
        }

        // Validate it's a git repository
        let git_dir = config.repo_path.join(".git");
        if !git_dir.exists() {
            return Err(anyhow::anyhow!(
                "Not a git repository (missing .git directory): {}",
                config.repo_path.display()
            ));
        }

        Ok(Self {
            config,
            message_bus,
            workflow_id: None,
            last_commit_hash: Arc::new(Mutex::new(None)),
            is_running: AtomicBool::new(false),
        })
    }

    /// Associate this watcher with a workflow ID.
    ///
    /// This is required for publishing GitEvent messages when commits
    /// without METADATA are detected.
    pub fn set_workflow_id(&mut self, workflow_id: impl Into<String>) {
        self.workflow_id = Some(workflow_id.into());
    }

    /// Get the workflow ID this watcher is associated with.
    pub fn workflow_id(&self) -> Option<&str> {
        self.workflow_id.as_deref()
    }

    /// Check if the watcher is currently running
    pub fn is_running(&self) -> bool {
        self.is_running.load(Ordering::SeqCst)
    }

    /// Start watching the repository for new commits
    ///
    /// This method polls the git repository for new commits and processes
    /// any that contain workflow metadata. It runs until cancelled.
    pub async fn watch(&self) -> Result<()> {
        self.is_running.store(true, Ordering::SeqCst);
        let repo_path = self.config.repo_path.clone();
        let poll_interval = Duration::from_millis(self.config.poll_interval_ms);

        tracing::info!(
            "Starting GitWatcher for {} (polling every {:?})",
            repo_path.display(),
            poll_interval
        );

        // Get initial HEAD commit
        if let Ok(initial_commit) = self.get_latest_commit(&repo_path).await {
            {
                let mut hash = self.last_commit_hash.lock().await;
                *hash = Some(initial_commit.hash.clone());
            }
            tracing::info!("Initial commit: {}", initial_commit.hash);
        }

        // Polling loop
        while self.is_running.load(Ordering::SeqCst) {
            tokio::time::sleep(poll_interval).await;

            let last_seen = { self.last_commit_hash.lock().await.clone() };
            let new_commits = match self
                .get_new_commits_since(&repo_path, last_seen.as_deref())
                .await
            {
                Ok(commits) => commits,
                Err(e) => {
                    tracing::error!("Error checking new commits: {}", e);
                    continue;
                }
            };

            for commit in new_commits {
                tracing::info!("New commit detected: {}", commit.hash);

                match self.handle_new_commit(commit.clone()).await {
                    Ok(()) => {
                        let mut last_hash = self.last_commit_hash.lock().await;
                        *last_hash = Some(commit.hash.clone());
                    }
                    Err(e) => {
                        tracing::error!(
                            commit_hash = %commit.hash,
                            error = %e,
                            "Error handling commit; keeping cursor unchanged for retry"
                        );
                        break;
                    }
                }
            }
        }

        tracing::info!("GitWatcher stopped");
        Ok(())
    }

    /// Stop watching the repository
    pub fn stop(&self) {
        self.is_running.store(false, Ordering::SeqCst);
        tracing::info!("GitWatcher stop requested");
    }

    /// Get the latest commit from the repository
    async fn get_latest_commit(&self, repo_path: &Path) -> Result<ParsedCommit> {
        self.get_commit_by_hash(repo_path, "HEAD").await
    }

    async fn get_new_commits_since(
        &self,
        repo_path: &Path,
        last_seen: Option<&str>,
    ) -> Result<Vec<ParsedCommit>> {
        use tokio::process::Command;

        let mut cmd = Command::new("git");
        cmd.current_dir(repo_path)
            .args(["log", "--format=%H", "--reverse"]);

        if let Some(last_hash) = last_seen {
            cmd.arg(format!("{last_hash}..HEAD"));
        } else {
            cmd.args(["-1"]);
        }

        let output = cmd.output().await.context(format!(
            "Failed to list commits from {}",
            self.config.repo_path.display()
        ))?;

        if !output.status.success() {
            anyhow::bail!(
                "git log for new commits failed: {}",
                String::from_utf8_lossy(&output.stderr)
            );
        }

        let hashes: Vec<String> = String::from_utf8_lossy(&output.stdout)
            .lines()
            .map(str::trim)
            .filter(|line| !line.is_empty())
            .map(ToString::to_string)
            .collect();

        let mut commits = Vec::with_capacity(hashes.len());
        for hash in hashes {
            commits.push(self.get_commit_by_hash(repo_path, &hash).await?);
        }

        Ok(commits)
    }

    async fn get_commit_by_hash(&self, repo_path: &Path, revision: &str) -> Result<ParsedCommit> {
        use tokio::process::Command;

        // Get commit hash and subject
        let output = Command::new("git")
            .current_dir(repo_path)
            .args(["show", "-s", "--format=%H|%s", revision])
            .output()
            .await
            .context(format!(
                "Failed to read commit {} from {}",
                revision,
                self.config.repo_path.display()
            ))?;

        if !output.status.success() {
            anyhow::bail!(
                "git show failed: {}",
                String::from_utf8_lossy(&output.stderr)
            );
        }

        let result = String::from_utf8_lossy(&output.stdout);
        let parts: Vec<&str> = result.trim().split('|').collect();

        if parts.len() < 2 {
            anyhow::bail!("Invalid git log output format");
        }

        let hash = parts[0].to_string();
        let message = parts[1..].join("|"); // Handle cases where message might contain '|'

        // Get current branch name
        let branch_output = Command::new("git")
            .current_dir(repo_path)
            .args(["rev-parse", "--abbrev-ref", "HEAD"])
            .output()
            .await
            .context(format!(
                "Failed to get current branch from {}",
                self.config.repo_path.display()
            ))?;

        let branch = if branch_output.status.success() {
            String::from_utf8_lossy(&branch_output.stdout)
                .trim()
                .to_string()
        } else {
            tracing::warn!(
                "Failed to get branch name: {}",
                String::from_utf8_lossy(&branch_output.stderr)
            );
            "unknown".to_string()
        };

        // Try to parse metadata from the commit message
        // Get full message body for metadata parsing
        let full_output = Command::new("git")
            .current_dir(repo_path)
            .args(["show", "-s", "--format=%B", revision])
            .output()
            .await
            .context(format!(
                "Failed to get commit body {} from {}",
                revision,
                self.config.repo_path.display()
            ))?;

        if !full_output.status.success() {
            anyhow::bail!(
                "git show body failed: {}",
                String::from_utf8_lossy(&full_output.stderr)
            );
        }

        let full_message = String::from_utf8_lossy(&full_output.stdout).to_string();
        let metadata = CommitMetadata::parse(&full_message);

        Ok(ParsedCommit {
            hash,
            branch,
            message,
            metadata,
        })
    }

    /// Handle a new commit by parsing its metadata and publishing events
    async fn handle_new_commit(&self, commit: ParsedCommit) -> Result<()> {
        // Check if commit has metadata
        let metadata = if let Some(m) = commit.metadata { m } else {
            // No metadata - publish GitEvent to wake up orchestrator
            let Some(workflow_id) = self.workflow_id.as_deref() else {
                tracing::debug!(
                    "Commit {} has no workflow metadata and watcher is not bound to a workflow, skipping",
                    commit.hash
                );
                return Ok(());
            };

            // Publish GitEvent for commits without METADATA
            self.message_bus
                .publish_git_event(workflow_id, &commit.hash, &commit.branch, &commit.message)
                .await;

            tracing::info!(
                "Published GitEvent for commit {} (no metadata) on workflow {}",
                commit.hash,
                workflow_id
            );
            return Ok(());
        };

        if let Some(bound_workflow_id) = self.workflow_id.as_deref()
            && metadata.workflow_id != bound_workflow_id
        {
            tracing::warn!(
                commit_hash = %commit.hash,
                commit_workflow_id = %metadata.workflow_id,
                bound_workflow_id = %bound_workflow_id,
                terminal_id = %metadata.terminal_id,
                "Commit metadata workflow_id does not match watcher binding, skipping"
            );
            return Ok(());
        }

        tracing::info!(
            "Processing commit with workflow_id={}, task_id={}, terminal_id={}, status={}",
            metadata.workflow_id,
            metadata.task_id,
            metadata.terminal_id,
            metadata.status
        );

        // Determine completion status.
        // `status=completed` with `next_action=continue` means checkpoint commit,
        // not terminal handoff/completion.
        let Some(completion_status) = Self::completion_status_from_metadata(&metadata) else {
            tracing::info!(
                terminal_id = %metadata.terminal_id,
                task_id = %metadata.task_id,
                status = %metadata.status,
                next_action = %metadata.next_action,
                "Commit metadata indicates continue mode; skipping TerminalCompleted publish"
            );

            self.message_bus
                .publish_git_event(
                    &metadata.workflow_id,
                    &commit.hash,
                    &commit.branch,
                    &commit.message,
                )
                .await;

            return Ok(());
        };

        // Create terminal completion event
        let event = TerminalCompletionEvent {
            terminal_id: metadata.terminal_id.clone(),
            task_id: metadata.task_id.clone(),
            workflow_id: metadata.workflow_id.clone(),
            status: completion_status,
            commit_hash: Some(commit.hash.clone()),
            commit_message: Some(commit.message.clone()),
            metadata: Some(metadata.to_orchestrator_metadata()),
        };

        // Publish to message bus
        self.message_bus.publish_terminal_completed(event).await;

        tracing::info!(
            "Published TerminalCompleted event for terminal {}",
            metadata.terminal_id
        );

        Ok(())
    }

    fn completion_status_from_metadata(
        metadata: &CommitMetadata,
    ) -> Option<TerminalCompletionStatus> {
        let status = metadata.status.trim().to_ascii_lowercase();
        let next_action = metadata.next_action.trim().to_ascii_lowercase();

        match status.as_str() {
            "completed" => {
                if matches!(next_action.as_str(), "continue" | "retry") {
                    Some(TerminalCompletionStatus::Checkpoint)
                } else {
                    Some(TerminalCompletionStatus::Completed)
                }
            }
            "checkpoint" => Some(TerminalCompletionStatus::Checkpoint),
            "review_pass" => Some(TerminalCompletionStatus::ReviewPass),
            "review_reject" => Some(TerminalCompletionStatus::ReviewReject),
            "failed" => Some(TerminalCompletionStatus::Failed),
            _ => None,
        }
    }
}

/// Parse commit metadata from commit message (standalone function)
///
/// This is a wrapper around CommitMetadata::parse that converts Option to Result.
/// It can be used when you need Result-based error handling instead of Option.
///
/// Format: "commit message\n---METADATA---\nkey-value pairs"
///
/// Returns error if metadata separator is not found or required fields are missing.
pub fn parse_commit_metadata(message: &str) -> Result<CommitMetadata> {
    CommitMetadata::parse(message).ok_or_else(|| {
        anyhow!("Failed to parse commit metadata: separator not found or required fields missing")
    })
}

#[cfg(test)]
mod tests {
    use std::time::{SystemTime, UNIX_EPOCH};

    use super::*;

    #[test]
    fn test_parse_commit_metadata_basic() {
        let message = r"Complete feature implementation

---METADATA---
workflow_id: abc-123
task_id: task-456
terminal_id: terminal-789
status: completed";

        let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

        assert_eq!(metadata.workflow_id, "abc-123");
        assert_eq!(metadata.task_id, "task-456");
        assert_eq!(metadata.terminal_id, "terminal-789");
        assert_eq!(metadata.status, "completed");
    }

    #[test]
    fn test_parse_commit_metadata_full() {
        let message = r"feat(14.5): create GitWatcher service

Implementation details here.

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
terminal_order: 0
cli: claude-code
model: sonnet-4.5
status: completed
severity: info
reviewed_terminal: terminal-001
next_action: continue";

        let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

        assert_eq!(metadata.workflow_id, "wf-123");
        assert_eq!(metadata.terminal_order, 0);
        assert_eq!(metadata.cli, "claude-code");
        assert_eq!(metadata.model, "sonnet-4.5");
        assert_eq!(metadata.severity, Some("info".to_string()));
        assert_eq!(metadata.reviewed_terminal, Some("terminal-001".to_string()));
        assert_eq!(metadata.next_action, "continue");
    }

    #[test]
    fn test_parse_commit_metadata_with_issues() {
        let message = r#"Fix bug

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
status: failed
issues: [{"severity":"error","file":"src/lib.rs","line":42,"message":"Null pointer","suggestion":"Add check"}]
next_action: retry"#;

        let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

        assert_eq!(metadata.status, "failed");
        assert_eq!(metadata.next_action, "retry");

        let issues = metadata.issues.expect("Should have issues");
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].severity, "error");
        assert_eq!(issues[0].file, "src/lib.rs");
        assert_eq!(issues[0].line, Some(42));
    }

    #[test]
    fn test_parse_commit_metadata_missing_required_fields() {
        let message = r"Incomplete commit

---METADATA---
workflow_id: wf-123
task_id: task-456
# missing terminal_id and status";

        let metadata = CommitMetadata::parse(message);
        assert!(
            metadata.is_none(),
            "Should return None when required fields are missing"
        );
    }

    #[test]
    fn test_parse_commit_metadata_no_metadata_section() {
        let message = "Normal commit without any metadata section";

        let metadata = CommitMetadata::parse(message);
        assert!(
            metadata.is_none(),
            "Should return None for commits without metadata"
        );
    }

    // Tests for the standalone parse_commit_metadata function
    #[test]
    fn test_parse_commit_metadata_standalone_valid() {
        let message = r"Complete feature implementation

---METADATA---
workflow_id: abc-123
task_id: task-456
terminal_id: terminal-789
status: completed";

        let result = parse_commit_metadata(message);
        assert!(result.is_ok());
        let metadata = result.unwrap();
        assert_eq!(metadata.workflow_id, "abc-123");
        assert_eq!(metadata.task_id, "task-456");
        assert_eq!(metadata.terminal_id, "terminal-789");
        assert_eq!(metadata.status, "completed");
    }

    #[test]
    fn test_parse_commit_metadata_standalone_no_metadata() {
        let message = "Simple commit message without metadata";
        let result = parse_commit_metadata(message);
        assert!(result.is_err()); // Should return error, not empty metadata
    }

    #[test]
    fn test_parse_commit_metadata_standalone_missing_required_fields() {
        let message = r"Incomplete commit

---METADATA---
workflow_id: wf-123
task_id: task-456
# missing terminal_id and status";

        let result = parse_commit_metadata(message);
        assert!(result.is_err());
    }

    #[test]
    fn test_parse_commit_metadata_standalone_with_optional_fields() {
        let message = r"Complete with all fields

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
terminal_order: 1
cli: claude-code
model: sonnet-4.5
status: completed
severity: info
reviewed_terminal: terminal-001
next_action: continue";

        let result = parse_commit_metadata(message);
        assert!(result.is_ok());
        let metadata = result.unwrap();
        assert_eq!(metadata.terminal_order, 1);
        assert_eq!(metadata.cli, "claude-code");
        assert_eq!(metadata.model, "sonnet-4.5");
        assert_eq!(metadata.severity, Some("info".to_string()));
        assert_eq!(metadata.reviewed_terminal, Some("terminal-001".to_string()));
        assert_eq!(metadata.next_action, "continue");
    }

    #[test]
    fn test_completion_status_completed_continue_returns_checkpoint() {
        let metadata = CommitMetadata {
            workflow_id: "wf-1".to_string(),
            task_id: "task-1".to_string(),
            terminal_id: "term-1".to_string(),
            terminal_order: 1,
            cli: "claude-code".to_string(),
            model: "opus".to_string(),
            status: "completed".to_string(),
            severity: None,
            reviewed_terminal: None,
            issues: None,
            next_action: "continue".to_string(),
        };

        let status = GitWatcher::completion_status_from_metadata(&metadata);
        assert!(matches!(status, Some(TerminalCompletionStatus::Checkpoint)));
    }

    #[test]
    fn test_completion_status_completed_handoff_advances() {
        let metadata = CommitMetadata {
            workflow_id: "wf-1".to_string(),
            task_id: "task-1".to_string(),
            terminal_id: "term-1".to_string(),
            terminal_order: 1,
            cli: "claude-code".to_string(),
            model: "opus".to_string(),
            status: "completed".to_string(),
            severity: None,
            reviewed_terminal: None,
            issues: None,
            next_action: "handoff".to_string(),
        };

        let status = GitWatcher::completion_status_from_metadata(&metadata);
        assert!(matches!(status, Some(TerminalCompletionStatus::Completed)));
    }

    #[test]
    fn test_completion_status_unknown_does_not_advance() {
        let metadata = CommitMetadata {
            workflow_id: "wf-1".to_string(),
            task_id: "task-1".to_string(),
            terminal_id: "term-1".to_string(),
            terminal_order: 1,
            cli: "claude-code".to_string(),
            model: "opus".to_string(),
            status: "processing".to_string(),
            severity: None,
            reviewed_terminal: None,
            issues: None,
            next_action: "continue".to_string(),
        };

        let status = GitWatcher::completion_status_from_metadata(&metadata);
        assert!(status.is_none());
    }

    #[tokio::test]
    async fn test_handle_new_commit_skips_when_metadata_workflow_mismatch() {
        let mut repo_path = std::env::temp_dir();
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock should be after unix epoch")
            .as_nanos();
        repo_path.push(format!(
            "gitwatcher-mismatch-test-{}-{}",
            std::process::id(),
            unique
        ));
        std::fs::create_dir_all(repo_path.join(".git"))
            .expect("create mock git repo should succeed");

        let message_bus = MessageBus::new(16);
        let mut watcher = GitWatcher::new(
            GitWatcherConfig::new(repo_path.clone(), 100),
            message_bus.clone(),
        )
        .expect("watcher should be created");
        watcher.set_workflow_id("wf-bound");

        let mut workflow_rx = message_bus.subscribe("workflow:wf-bound").await;
        let commit = ParsedCommit {
            hash: "abc123".to_string(),
            branch: "main".to_string(),
            message: "feat: test mismatch".to_string(),
            metadata: Some(CommitMetadata {
                workflow_id: "wf-other".to_string(),
                task_id: "task-1".to_string(),
                terminal_id: "terminal-1".to_string(),
                terminal_order: 0,
                cli: "claude-code".to_string(),
                model: "model".to_string(),
                status: "completed".to_string(),
                severity: None,
                reviewed_terminal: None,
                issues: None,
                next_action: "handoff".to_string(),
            }),
        };

        watcher
            .handle_new_commit(commit)
            .await
            .expect("mismatch should be handled safely");

        let recv = tokio::time::timeout(Duration::from_millis(150), workflow_rx.recv()).await;
        assert!(recv.is_err(), "mismatch commit should not publish events");

        let _ = std::fs::remove_dir_all(repo_path);
    }
}
