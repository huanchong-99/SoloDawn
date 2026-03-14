//! Constants for the Orchestrator module
//!
//! This module contains all hardcoded string constants used throughout the orchestrator.

/// Topic prefixes for message bus
pub const WORKFLOW_TOPIC_PREFIX: &str = "workflow:";
pub const TERMINAL_TOPIC_PREFIX: &str = "terminal:";
pub const GIT_EVENT_TOPIC_PREFIX: &str = "git_event:";

/// Commit metadata format
pub const GIT_COMMIT_METADATA_SEPARATOR: &str = "---METADATA---";

/// Environment variable names
pub const ENCRYPTION_KEY_ENV: &str = "GITCORTEX_ENCRYPTION_KEY";

/// Default configuration values
pub const DEFAULT_MAX_CONVERSATION_HISTORY: usize = 50;
pub const DEFAULT_LLM_TIMEOUT_SECS: u64 = 120;
pub const DEFAULT_MAX_RETRIES: u32 = 3;
pub const DEFAULT_RETRY_DELAY_MS: u64 = 1000;
pub const DEFAULT_LLM_RATE_LIMIT_PER_SECOND: u32 = 10;

/// Terminal status values
pub const TERMINAL_STATUS_NOT_STARTED: &str = "not_started";
pub const TERMINAL_STATUS_STARTING: &str = "starting";
pub const TERMINAL_STATUS_WAITING: &str = "waiting";
pub const TERMINAL_STATUS_WORKING: &str = "working";
pub const TERMINAL_STATUS_COMPLETED: &str = "completed";
pub const TERMINAL_STATUS_FAILED: &str = "failed";
pub const TERMINAL_STATUS_CANCELLED: &str = "cancelled";
/// Used by agent.rs for code-review terminal outcomes.
pub const TERMINAL_STATUS_REVIEW_PASSED: &str = "review_passed";
/// Used by agent.rs for code-review terminal outcomes.
pub const TERMINAL_STATUS_REVIEW_REJECTED: &str = "review_rejected";

/// Workflow status values — mirrors `WorkflowStatus` enum in `db::models::workflow`.
pub const WORKFLOW_STATUS_CREATED: &str = "created";
pub const WORKFLOW_STATUS_STARTING: &str = "starting";
pub const WORKFLOW_STATUS_READY: &str = "ready";
pub const WORKFLOW_STATUS_RUNNING: &str = "running";
pub const WORKFLOW_STATUS_PAUSED: &str = "paused";
pub const WORKFLOW_STATUS_MERGING: &str = "merging";
pub const WORKFLOW_STATUS_COMPLETED: &str = "completed";
pub const WORKFLOW_STATUS_FAILED: &str = "failed";
pub const WORKFLOW_STATUS_CANCELLED: &str = "cancelled";

/// Task status values — mirrors `WorkflowTaskStatus` enum in `db::models::workflow`.
pub const TASK_STATUS_PENDING: &str = "pending";
pub const TASK_STATUS_RUNNING: &str = "running";
pub const TASK_STATUS_REVIEW_PENDING: &str = "reviewpending";
pub const TASK_STATUS_COMPLETED: &str = "completed";
pub const TASK_STATUS_FAILED: &str = "failed";
pub const TASK_STATUS_CANCELLED: &str = "cancelled";

// Phase 28A: Terminal completion context limits
pub const COMPLETION_CONTEXT_LOG_LINES: usize = 50;
pub const COMPLETION_CONTEXT_LOG_MAX_CHARS: usize = 2000;
pub const COMPLETION_CONTEXT_DIFF_MAX_CHARS: usize = 1000;
pub const COMPLETION_CONTEXT_BODY_MAX_CHARS: usize = 500;

// Phase 28C: Agent event loop fault tolerance
pub const MAX_CONSECUTIVE_LLM_FAILURES: u32 = 10;
pub const STATE_SAVE_DEBOUNCE_SECS: u64 = 5;

// Phase 28A: Handoff context limits
pub const HANDOFF_CONTEXT_MAX_CHARS: usize = 1500;
pub const HANDOFF_COMMIT_MAX_CHARS: usize = 500;
pub const HANDOFF_NOTES_MAX_CHARS: usize = 800;

/// Startable terminal statuses — terminals in these states can be dispatched.
///
/// [G15-007] This list intentionally includes only `waiting`. A terminal must
/// have completed the PTY spawn lifecycle (not_started → starting → waiting)
/// before it can receive instructions. `not_started` and `starting` are
/// excluded because the PTY is not yet ready to accept input.
pub const STARTABLE_TERMINAL_STATUSES: &[&str] = &[TERMINAL_STATUS_WAITING];

// Phase 29C: Quality Gate constants
pub const TERMINAL_STATUS_QUALITY_PENDING: &str = "quality_pending";
pub const QUALITY_GATE_MODE_OFF: &str = "off";
pub const QUALITY_GATE_MODE_SHADOW: &str = "shadow";
pub const QUALITY_GATE_MODE_WARN: &str = "warn";
pub const QUALITY_GATE_MODE_ENFORCE: &str = "enforce";
pub const QUALITY_GATE_DEFAULT_MODE: &str = QUALITY_GATE_MODE_SHADOW;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_all_workflow_status_constants_exist() {
        let _ = WORKFLOW_STATUS_CREATED;
        let _ = WORKFLOW_STATUS_STARTING;
        let _ = WORKFLOW_STATUS_READY;
        let _ = WORKFLOW_STATUS_RUNNING;
        let _ = WORKFLOW_STATUS_PAUSED;
        let _ = WORKFLOW_STATUS_MERGING;
        let _ = WORKFLOW_STATUS_COMPLETED;
        let _ = WORKFLOW_STATUS_FAILED;
        let _ = WORKFLOW_STATUS_CANCELLED;
    }

    #[test]
    fn test_all_task_status_constants_exist() {
        let _ = TASK_STATUS_PENDING;
        let _ = TASK_STATUS_RUNNING;
        let _ = TASK_STATUS_REVIEW_PENDING;
        let _ = TASK_STATUS_COMPLETED;
        let _ = TASK_STATUS_FAILED;
        let _ = TASK_STATUS_CANCELLED;
    }

    #[test]
    fn test_startable_terminal_statuses() {
        assert_eq!(STARTABLE_TERMINAL_STATUSES, &[TERMINAL_STATUS_WAITING]);
    }

    #[test]
    fn test_workflow_status_ready_value() {
        assert_eq!(WORKFLOW_STATUS_READY, "ready");
    }

    #[test]
    fn test_workflow_status_created_value() {
        assert_eq!(WORKFLOW_STATUS_CREATED, "created");
    }
}
