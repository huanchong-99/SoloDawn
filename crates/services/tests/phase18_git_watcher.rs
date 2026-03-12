//! Phase 18 Git Watcher E2E Tests
//!
//! Tests for Git commit detection and workflow event handling.

use std::{fs, path::PathBuf, process::Command, time::Duration};

use services::services::{
    git_watcher::{CommitMetadata, GitWatcher, GitWatcherConfig},
    orchestrator::{BusMessage, MessageBus},
};
use tempfile::TempDir;
use uuid::Uuid;

/// Helper to run git commands
fn run_git(repo_path: &PathBuf, args: &[&str]) {
    let output = Command::new("git")
        .args(args)
        .current_dir(repo_path)
        .output()
        .expect("git command failed to start");
    assert!(
        output.status.success(),
        "git {:?} failed: {}",
        args,
        String::from_utf8_lossy(&output.stderr)
    );
}

/// Initialize a test git repository
fn init_test_repo() -> (TempDir, PathBuf) {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let repo_path = temp_dir.path().to_path_buf();

    run_git(&repo_path, &["init"]);
    run_git(&repo_path, &["config", "user.name", "Test User"]);
    run_git(&repo_path, &["config", "user.email", "test@example.com"]);

    // Create initial commit
    fs::write(repo_path.join("README.md"), "# Test Repository").expect("write README");
    run_git(&repo_path, &["add", "."]);
    run_git(&repo_path, &["commit", "-m", "Initial commit"]);

    (temp_dir, repo_path)
}

/// Create a commit with workflow metadata
fn create_commit_with_metadata(
    repo_path: &PathBuf,
    workflow_id: &str,
    task_id: &str,
    terminal_id: &str,
    status: &str,
) {
    use std::time::{SystemTime, UNIX_EPOCH};

    // Create unique file content
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    fs::write(
        repo_path.join("task.txt"),
        format!("Content: {}", timestamp),
    )
    .expect("write task file");

    run_git(repo_path, &["add", "."]);

    let message = format!(
        "Complete task\n\n---METADATA---\nworkflow_id: {}\ntask_id: {}\nterminal_id: {}\nstatus: {}",
        workflow_id, task_id, terminal_id, status
    );

    run_git(repo_path, &["commit", "-m", &message]);
}

// ============================================================================
// Git Watcher Tests
// ============================================================================

#[tokio::test]
async fn test_git_watcher_detects_commit_with_metadata() {
    let (_temp_dir, repo_path) = init_test_repo();
    let message_bus = MessageBus::new(100);

    let config = GitWatcherConfig {
        repo_path: repo_path.clone(),
        poll_interval_ms: 50,
    };

    // Subscribe before creating watcher
    let mut receiver = message_bus.subscribe_broadcast();

    let watcher = GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

    // Start watching in background
    let watcher_handle = tokio::spawn(async move {
        watcher.watch().await.unwrap();
    });

    // Give watcher time to start and record initial commit
    tokio::time::sleep(Duration::from_millis(150)).await;

    // Create commit with metadata
    let workflow_id = Uuid::new_v4().to_string();
    let task_id = Uuid::new_v4().to_string();
    let terminal_id = Uuid::new_v4().to_string();

    create_commit_with_metadata(
        &repo_path,
        &workflow_id,
        &task_id,
        &terminal_id,
        "completed",
    );

    // Wait for event
    let result = tokio::time::timeout(Duration::from_secs(2), receiver.recv()).await;

    assert!(result.is_ok(), "Should receive event within timeout");
    let message = result.unwrap().expect("Should receive message");

    match message {
        BusMessage::TerminalCompleted(event) => {
            assert_eq!(event.workflow_id, workflow_id);
            assert_eq!(event.task_id, task_id);
            assert_eq!(event.terminal_id, terminal_id);
        }
        _ => panic!("Expected TerminalCompleted message"),
    }

    watcher_handle.abort();
}

#[tokio::test]
async fn test_git_watcher_ignores_commits_without_metadata() {
    let (_temp_dir, repo_path) = init_test_repo();
    let message_bus = MessageBus::new(100);

    let config = GitWatcherConfig {
        repo_path: repo_path.clone(),
        poll_interval_ms: 50,
    };

    let mut receiver = message_bus.subscribe_broadcast();

    let watcher = GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

    let watcher_handle = tokio::spawn(async move {
        watcher.watch().await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(150)).await;

    // Create commit WITHOUT metadata
    fs::write(repo_path.join("normal.txt"), "Normal content").expect("write file");
    run_git(&repo_path, &["add", "."]);
    run_git(
        &repo_path,
        &["commit", "-m", "Normal commit without metadata"],
    );

    // Should NOT receive any event
    let result = tokio::time::timeout(Duration::from_millis(500), receiver.recv()).await;
    assert!(
        result.is_err(),
        "Should NOT receive event for commit without metadata"
    );

    watcher_handle.abort();
}

#[tokio::test]
async fn test_git_watcher_handles_failed_status() {
    let (_temp_dir, repo_path) = init_test_repo();
    let message_bus = MessageBus::new(100);

    let config = GitWatcherConfig {
        repo_path: repo_path.clone(),
        poll_interval_ms: 50,
    };

    let mut receiver = message_bus.subscribe_broadcast();

    let watcher = GitWatcher::new(config, message_bus).expect("Failed to create GitWatcher");

    let watcher_handle = tokio::spawn(async move {
        watcher.watch().await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(150)).await;

    // Create commit with failed status
    let workflow_id = Uuid::new_v4().to_string();
    let task_id = Uuid::new_v4().to_string();
    let terminal_id = Uuid::new_v4().to_string();

    create_commit_with_metadata(&repo_path, &workflow_id, &task_id, &terminal_id, "failed");

    let result = tokio::time::timeout(Duration::from_secs(2), receiver.recv()).await;

    assert!(result.is_ok(), "Should receive event within timeout");
    let message = result.unwrap().expect("Should receive message");

    match message {
        BusMessage::TerminalCompleted(event) => {
            assert_eq!(event.workflow_id, workflow_id);
            // Status should be Failed
            assert!(matches!(
                event.status,
                services::services::orchestrator::TerminalCompletionStatus::Failed
            ));
        }
        _ => panic!("Expected TerminalCompleted message"),
    }

    watcher_handle.abort();
}

// ============================================================================
// Commit Metadata Parsing Tests
// ============================================================================

#[test]
fn test_commit_metadata_parse_all_fields() {
    let message = r#"feat(14.5): create GitWatcher service

Implementation of GitWatcher for commit monitoring.

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
terminal_order: 2
cli: claude-code
model: sonnet-4.5
status: completed
severity: info
reviewed_terminal: terminal-001
next_action: continue"#;

    let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

    assert_eq!(metadata.workflow_id, "wf-123");
    assert_eq!(metadata.task_id, "task-456");
    assert_eq!(metadata.terminal_id, "terminal-789");
    assert_eq!(metadata.terminal_order, 2);
    assert_eq!(metadata.cli, "claude-code");
    assert_eq!(metadata.model, "sonnet-4.5");
    assert_eq!(metadata.status, "completed");
    assert_eq!(metadata.severity, Some("info".to_string()));
    assert_eq!(metadata.reviewed_terminal, Some("terminal-001".to_string()));
    assert_eq!(metadata.next_action, "continue");
}

#[test]
fn test_commit_metadata_parse_with_issues() {
    let message = r#"Fix authentication bug

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
status: failed
severity: error
issues: [{"severity":"error","file":"src/auth.rs","line":42,"message":"Null pointer dereference","suggestion":"Add null check"}]
next_action: retry"#;

    let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

    assert_eq!(metadata.status, "failed");
    assert_eq!(metadata.severity, Some("error".to_string()));
    assert_eq!(metadata.next_action, "retry");

    let issues = metadata.issues.expect("Should have issues");
    assert_eq!(issues.len(), 1);
    assert_eq!(issues[0].severity, "error");
    assert_eq!(issues[0].file, "src/auth.rs");
    assert_eq!(issues[0].line, Some(42));
    assert_eq!(issues[0].message, "Null pointer dereference");
    assert_eq!(issues[0].suggestion, Some("Add null check".to_string()));
}

#[test]
fn test_commit_metadata_parse_missing_required_fields() {
    // Missing terminal_id
    let message = r#"Incomplete commit

---METADATA---
workflow_id: wf-123
task_id: task-456
status: completed"#;

    let result = CommitMetadata::parse(message);
    assert!(
        result.is_none(),
        "Should return None when required fields are missing"
    );
}

#[test]
fn test_commit_metadata_parse_no_metadata_section() {
    let message = "Normal commit without any metadata section";
    let result = CommitMetadata::parse(message);
    assert!(
        result.is_none(),
        "Should return None for commits without metadata"
    );
}

#[test]
fn test_commit_metadata_review_pass_status() {
    let message = r#"Code review passed

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
status: review_pass
reviewed_terminal: terminal-001
next_action: continue"#;

    let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

    assert_eq!(metadata.status, "review_pass");
    assert_eq!(metadata.reviewed_terminal, Some("terminal-001".to_string()));
}

#[test]
fn test_commit_metadata_review_reject_status() {
    let message = r#"Code review failed

---METADATA---
workflow_id: wf-123
task_id: task-456
terminal_id: terminal-789
status: review_reject
reviewed_terminal: terminal-001
severity: warning
next_action: retry"#;

    let metadata = CommitMetadata::parse(message).expect("Should parse metadata");

    assert_eq!(metadata.status, "review_reject");
    assert_eq!(metadata.severity, Some("warning".to_string()));
    assert_eq!(metadata.next_action, "retry");
}
