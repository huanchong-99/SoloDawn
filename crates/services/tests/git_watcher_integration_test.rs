//! GitWatcher integration tests
//!
//! Tests for monitoring git repositories and parsing commit metadata.

use std::{path::PathBuf, time::Duration};

use services::services::{
    git_watcher::{CommitMetadata, GitWatcher, GitWatcherConfig},
    orchestrator::{BusMessage, MessageBus},
};
use tokio::time::timeout;

/// Helper to create a test commit message with metadata
fn create_test_commit_message(
    workflow_id: &str,
    task_id: &str,
    terminal_id: &str,
    status: &str,
) -> String {
    let next_action = if status.eq_ignore_ascii_case("failed") {
        "retry"
    } else {
        "handoff"
    };
    format!(
        "Complete feature implementation\n\n---METADATA---\nworkflow_id: {}\ntask_id: {}\nterminal_id: {}\nstatus: {}\nnext_action: {}",
        workflow_id, task_id, terminal_id, status, next_action
    )
}

/// Helper to create a test commit message with all fields
fn create_full_commit_message() -> String {
    "feat(14.5): create GitWatcher service\n\nImplementation of GitWatcher for commit monitoring.\n\n---METADATA---\nworkflow_id: wf-123\ntask_id: task-456\nterminal_id: terminal-789\nterminal_order: 0\ncli: claude-code\nmodel: sonnet-4.5\nstatus: completed\nnext_action: continue".to_string()
}

#[cfg(test)]
mod commit_metadata_tests {
    use super::*;

    #[test]
    fn test_parse_basic_commit_metadata() {
        let message = create_test_commit_message("wf-123", "task-456", "terminal-789", "completed");

        let metadata = CommitMetadata::parse(&message).expect("Failed to parse metadata");

        assert_eq!(metadata.workflow_id, "wf-123");
        assert_eq!(metadata.task_id, "task-456");
        assert_eq!(metadata.terminal_id, "terminal-789");
        assert_eq!(metadata.status, "completed");
    }

    #[test]
    fn test_parse_full_commit_metadata() {
        let message = create_full_commit_message();

        let metadata = CommitMetadata::parse(&message).expect("Failed to parse metadata");

        assert_eq!(metadata.workflow_id, "wf-123");
        assert_eq!(metadata.task_id, "task-456");
        assert_eq!(metadata.terminal_id, "terminal-789");
        assert_eq!(metadata.terminal_order, 0);
        assert_eq!(metadata.cli, "claude-code");
        assert_eq!(metadata.model, "sonnet-4.5");
        assert_eq!(metadata.status, "completed");
        assert_eq!(metadata.next_action, "continue");
    }

    #[test]
    fn test_parse_commit_with_issues() {
        let message = "Fix authentication bug\n\n---METADATA---\nworkflow_id: wf-123\ntask_id: task-456\nterminal_id: terminal-789\nstatus: failed\nseverity: error\nissues: [{\"severity\":\"error\",\"file\":\"src/auth.rs\",\"line\":42,\"message\":\"Null pointer dereference\",\"suggestion\":\"Add null check\"}]\nnext_action: retry".to_string();

        let metadata = CommitMetadata::parse(&message).expect("Failed to parse metadata");

        assert_eq!(metadata.status, "failed");
        assert_eq!(metadata.severity, Some("error".to_string()));

        // Verify issues parsing
        let issues = metadata.issues.expect("Issues should be present");
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].severity, "error");
        assert_eq!(issues[0].file, "src/auth.rs");
        assert_eq!(issues[0].line, Some(42));
        assert_eq!(issues[0].message, "Null pointer dereference");
        assert_eq!(issues[0].suggestion, Some("Add null check".to_string()));
    }

    #[test]
    fn test_parse_commit_without_metadata() {
        let message = "Normal commit without metadata";

        let result = CommitMetadata::parse(message);
        assert!(
            result.is_none(),
            "Should return None for commits without metadata"
        );
    }

    #[test]
    fn test_parse_commit_with_invalid_metadata() {
        let message = "Some message\n\n---METADATA---\nworkflow_id:"; // Missing value

        let result = CommitMetadata::parse(message);
        assert!(result.is_none(), "Should return None for invalid metadata");
    }

    #[test]
    fn test_parse_commit_with_optional_fields() {
        let message = "Implement feature\n\n---METADATA---\nworkflow_id: wf-123\ntask_id: task-456\nterminal_id: terminal-789\nstatus: completed\nreviewed_terminal: terminal-001\nnext_action: review".to_string();

        let metadata = CommitMetadata::parse(&message).expect("Failed to parse metadata");

        assert_eq!(metadata.reviewed_terminal, Some("terminal-001".to_string()));
        assert_eq!(metadata.next_action, "review");
        assert!(metadata.issues.is_none());
        assert!(metadata.severity.is_none());
    }
}

#[cfg(test)]
mod git_watcher_tests {
    use std::fs;

    use tempfile::TempDir;

    use super::*;

    /// Helper to create a test git repository
    fn create_test_repo() -> (TempDir, PathBuf) {
        let temp_dir = TempDir::new().expect("Failed to create temp dir");
        let repo_path = temp_dir.path().to_path_buf();

        // Initialize git repo
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(&repo_path)
            .output()
            .expect("Failed to init git repo");

        // Configure git
        std::process::Command::new("git")
            .args(["config", "user.name", "Test User"])
            .current_dir(&repo_path)
            .output()
            .expect("Failed to configure git user.name");

        std::process::Command::new("git")
            .args(["config", "user.email", "test@example.com"])
            .current_dir(&repo_path)
            .output()
            .expect("Failed to configure git user.email");

        // Create initial commit
        let test_file = repo_path.join("README.md");
        fs::write(&test_file, "# Test Repository").expect("Failed to write README");

        std::process::Command::new("git")
            .args(["add", "README.md"])
            .current_dir(&repo_path)
            .output()
            .expect("Failed to add README");

        std::process::Command::new("git")
            .args(["commit", "-m", "Initial commit"])
            .current_dir(&repo_path)
            .output()
            .expect("Failed to create initial commit");

        (temp_dir, repo_path)
    }

    /// Helper to create a commit with a specific message
    fn create_commit(repo_path: &PathBuf, message: &str) {
        use std::time::{SystemTime, UNIX_EPOCH};

        // Modify a file to have something to commit - use unique timestamp
        let test_file = repo_path.join("test.txt");
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        fs::write(&test_file, format!("Content: {}", timestamp))
            .expect("Failed to write test file");

        std::process::Command::new("git")
            .args(["add", "test.txt"])
            .current_dir(repo_path)
            .output()
            .expect("Failed to add file");

        std::process::Command::new("git")
            .args(["commit", "-m", message])
            .current_dir(repo_path)
            .output()
            .expect("Failed to commit");
    }

    #[tokio::test]
    async fn test_git_watcher_creation() {
        let (_temp_dir, repo_path) = create_test_repo();
        let message_bus = MessageBus::new(100);

        let config = GitWatcherConfig {
            repo_path: repo_path.clone(),
            poll_interval_ms: 100,
        };

        let watcher = GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");
        assert!(!watcher.is_running());
    }

    #[tokio::test]
    async fn test_watch_and_detect_commits() {
        let (_temp_dir, repo_path) = create_test_repo();
        let message_bus = MessageBus::new(100);

        let config = GitWatcherConfig {
            repo_path: repo_path.clone(),
            poll_interval_ms: 50,
        };

        // Subscribe to messages before creating watcher (which consumes message_bus)
        let mut receiver = message_bus.subscribe_broadcast();

        let watcher =
            GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

        // Start watching in background
        let watcher_handle = tokio::spawn(async move {
            watcher.watch().await.unwrap();
        });

        // Give the watcher time to start
        tokio::time::sleep(Duration::from_millis(100)).await;

        // Create a commit with metadata
        let commit_msg =
            create_test_commit_message("wf-test", "task-test", "terminal-test", "completed");
        create_commit(&repo_path, &commit_msg);

        // Wait for the event to be published
        let result = timeout(Duration::from_secs(2), receiver.recv()).await;

        // Verify we got a message
        assert!(result.is_ok(), "Should receive a message within timeout");

        let bus_message = result.unwrap().expect("Should receive a BusMessage");
        match bus_message {
            BusMessage::TerminalCompleted(event) => {
                assert_eq!(event.terminal_id, "terminal-test");
                assert_eq!(event.task_id, "task-test");
                assert_eq!(event.workflow_id, "wf-test");
            }
            _ => panic!("Expected TerminalCompleted message, got {:?}", bus_message),
        }

        // Cleanup
        watcher_handle.abort();
    }

    #[tokio::test]
    async fn test_ignore_commits_without_metadata() {
        let (_temp_dir, repo_path) = create_test_repo();
        let message_bus = MessageBus::new(100);

        let config = GitWatcherConfig {
            repo_path: repo_path.clone(),
            poll_interval_ms: 50,
        };

        // Subscribe to messages before creating watcher
        let mut receiver = message_bus.subscribe_broadcast();

        let watcher =
            GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

        // Start watching in background
        let watcher_handle = tokio::spawn(async move {
            watcher.watch().await.unwrap();
        });

        // Give the watcher time to start
        tokio::time::sleep(Duration::from_millis(100)).await;

        // Create a normal commit without metadata
        create_commit(&repo_path, "Normal commit without metadata");

        // Wait a bit to ensure no message is sent
        let result = timeout(Duration::from_millis(200), receiver.recv()).await;

        // Verify we didn't get a message
        assert!(
            result.is_err(),
            "Should not receive a message for commits without metadata"
        );

        // Cleanup
        watcher_handle.abort();
    }

    #[tokio::test]
    async fn test_handle_multiple_commits() {
        let (_temp_dir, repo_path) = create_test_repo();
        let message_bus = MessageBus::new(100);

        let config = GitWatcherConfig {
            repo_path: repo_path.clone(),
            poll_interval_ms: 50,
        };

        // Subscribe to messages before creating watcher
        let mut receiver = message_bus.subscribe_broadcast();

        let watcher =
            GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

        // Start watching in background
        let watcher_handle = tokio::spawn(async move {
            watcher.watch().await.unwrap();
        });

        // Give the watcher time to start and record initial commit
        tokio::time::sleep(Duration::from_millis(150)).await;

        // Create multiple commits with enough delay between them
        // to ensure each is detected by the polling watcher
        for i in 0..3 {
            let commit_msg = create_test_commit_message(
                &format!("wf-{}", i),
                &format!("task-{}", i),
                &format!("terminal-{}", i),
                "completed",
            );
            create_commit(&repo_path, &commit_msg);
            // Wait longer than poll_interval to ensure watcher detects each commit
            tokio::time::sleep(Duration::from_millis(150)).await;
        }

        // Collect all messages
        let mut events = Vec::new();
        for _ in 0..3 {
            let result = timeout(Duration::from_secs(2), receiver.recv()).await;
            if let Ok(Ok(BusMessage::TerminalCompleted(event))) = result {
                events.push(event);
            }
        }

        // Verify we got all 3 events
        assert_eq!(
            events.len(),
            3,
            "Should receive 3 terminal completion events"
        );

        // Cleanup
        watcher_handle.abort();
    }

    #[tokio::test]
    async fn test_handle_multiple_commits_in_single_poll_window() {
        let (_temp_dir, repo_path) = create_test_repo();
        let message_bus = MessageBus::new(100);

        let config = GitWatcherConfig {
            repo_path: repo_path.clone(),
            poll_interval_ms: 300,
        };

        let mut receiver = message_bus.subscribe_broadcast();
        let watcher =
            GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

        let watcher_handle = tokio::spawn(async move {
            watcher.watch().await.unwrap();
        });

        // Ensure watcher is initialized and baseline HEAD is recorded.
        tokio::time::sleep(Duration::from_millis(120)).await;

        // Create two commits back-to-back without waiting for another poll tick.
        for i in 0..2 {
            let commit_msg = create_test_commit_message(
                "wf-burst",
                &format!("task-burst-{}", i),
                &format!("terminal-burst-{}", i),
                "completed",
            );
            create_commit(&repo_path, &commit_msg);
        }

        // Collect two TerminalCompleted events.
        let deadline = tokio::time::Instant::now() + Duration::from_secs(4);
        let mut terminal_ids = Vec::new();
        while terminal_ids.len() < 2 && tokio::time::Instant::now() < deadline {
            match timeout(Duration::from_millis(800), receiver.recv()).await {
                Ok(Ok(BusMessage::TerminalCompleted(event))) => {
                    terminal_ids.push(event.terminal_id);
                }
                Ok(Ok(_)) => {}
                Ok(Err(_)) | Err(_) => {}
            }
        }

        assert_eq!(
            terminal_ids.len(),
            2,
            "Watcher should consume all commits created within one poll window"
        );
        assert!(terminal_ids.contains(&"terminal-burst-0".to_string()));
        assert!(terminal_ids.contains(&"terminal-burst-1".to_string()));

        watcher_handle.abort();
    }

    #[tokio::test]
    async fn test_stop_and_restart_watcher() {
        let (_temp_dir, repo_path) = create_test_repo();
        let message_bus = MessageBus::new(100);

        // First watch session
        let config = GitWatcherConfig {
            repo_path: repo_path.clone(),
            poll_interval_ms: 50,
        };

        let watcher =
            GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

        // Start watching
        let handle =
            tokio::spawn(async move { timeout(Duration::from_millis(200), watcher.watch()).await });

        // Wait a bit then let it complete naturally
        tokio::time::sleep(Duration::from_millis(100)).await;

        // The watcher should be running
        let result: Result<
            Result<Result<(), anyhow::Error>, tokio::time::error::Elapsed>,
            tokio::task::JoinError,
        > = handle.await;
        assert!(result.is_ok(), "Task should complete without panic");

        // Create a new watcher instance
        let message_bus2 = MessageBus::new(100);
        let config2 = GitWatcherConfig {
            repo_path,
            poll_interval_ms: 50,
        };
        let watcher2 = GitWatcher::new(config2, message_bus2).expect("Failed to create GitWatcher");

        // Should be able to start again
        assert!(!watcher2.is_running());
    }
}
