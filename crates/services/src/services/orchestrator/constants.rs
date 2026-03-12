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
pub const TERMINAL_STATUS_PENDING: &str = "pending";
pub const TERMINAL_STATUS_RUNNING: &str = "running";
pub const TERMINAL_STATUS_COMPLETED: &str = "completed";
pub const TERMINAL_STATUS_FAILED: &str = "failed";
pub const TERMINAL_STATUS_REVIEW_PASSED: &str = "review_passed";
pub const TERMINAL_STATUS_REVIEW_REJECTED: &str = "review_rejected";

/// Workflow status values
pub const WORKFLOW_STATUS_PENDING: &str = "pending";
pub const WORKFLOW_STATUS_READY: &str = "ready";
pub const WORKFLOW_STATUS_RUNNING: &str = "running";
pub const WORKFLOW_STATUS_COMPLETED: &str = "completed";
pub const WORKFLOW_STATUS_FAILED: &str = "failed";
pub const WORKFLOW_STATUS_MERGING: &str = "merging";

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
        let _ = WORKFLOW_STATUS_PENDING;
        let _ = WORKFLOW_STATUS_READY; // This is missing
        let _ = WORKFLOW_STATUS_RUNNING;
        let _ = WORKFLOW_STATUS_COMPLETED;
        let _ = WORKFLOW_STATUS_FAILED;
        let _ = WORKFLOW_STATUS_MERGING;
    }

    #[test]
    fn test_workflow_status_ready_value() {
        assert_eq!(WORKFLOW_STATUS_READY, "ready");
    }
}
