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
pub const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";

/// Default configuration values
pub const DEFAULT_MAX_CONVERSATION_HISTORY: usize = 50;
pub const DEFAULT_LLM_TIMEOUT_SECS: u64 = 300;
pub const DEFAULT_MAX_RETRIES: u32 = 3;
pub const DEFAULT_RETRY_DELAY_MS: u64 = 1000;
pub const DEFAULT_LLM_RATE_LIMIT_PER_SECOND: u32 = 10;
pub const DEFAULT_MAX_CONCURRENT_TERMINALS: usize = 4;
pub const MAX_CONCURRENT_TERMINALS_ENV: &str = "SOLODAWN_MAX_CONCURRENT_TERMINALS";

/// Resolve the process-level terminal concurrency cap.
///
/// The default is intentionally conservative for Windows/WebView hosts: the
/// bottleneck is process fan-out, not memory. Invalid values fail closed to the
/// default instead of disabling the cap.
pub fn configured_max_concurrent_terminals() -> usize {
    std::env::var(MAX_CONCURRENT_TERMINALS_ENV)
        .ok()
        .and_then(|value| value.trim().parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_MAX_CONCURRENT_TERMINALS)
}

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
pub const WORKFLOW_STATUS_MERGE_PARTIAL_FAILED: &str = "merge_partial_failed";

/// Task status values — mirrors `WorkflowTaskStatus` enum in `db::models::workflow`.
pub const TASK_STATUS_PENDING: &str = "pending";
pub const TASK_STATUS_RUNNING: &str = "running";
pub const TASK_STATUS_REVIEW_PENDING: &str = "reviewpending";
pub const TASK_STATUS_COMPLETED: &str = "completed";
pub const TASK_STATUS_FAILED: &str = "failed";
pub const TASK_STATUS_CANCELLED: &str = "cancelled";

// Phase 28A: Terminal completion context limits
pub const COMPLETION_CONTEXT_LOG_LINES: usize = 120;
pub const COMPLETION_CONTEXT_LOG_MAX_CHARS: usize = 6000;
pub const COMPLETION_CONTEXT_DIFF_MAX_CHARS: usize = 4000;
pub const COMPLETION_CONTEXT_BODY_MAX_CHARS: usize = 500;
pub const ACCEPTANCE_REVIEW_MAX_BYTES: usize = 131_072;

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

// Phase 30: Enforce-mode deadlock resolution
pub const MAX_ENFORCE_DEADLOCK_BLOCKS: u32 = 10;

// Phase 29C: Quality Gate constants
pub const TERMINAL_STATUS_QUALITY_PENDING: &str = "quality_pending";
pub const QUALITY_GATE_MODE_OFF: &str = "off";
pub const QUALITY_GATE_MODE_SHADOW: &str = "shadow";
pub const QUALITY_GATE_MODE_WARN: &str = "warn";
pub const QUALITY_GATE_MODE_ENFORCE: &str = "enforce";
pub const QUALITY_GATE_STATUS_SKIPPED: &str = "skipped";
pub const QUALITY_GATE_DEFAULT_MODE: &str = QUALITY_GATE_MODE_ENFORCE;

/// G16-001: Mandatory quality requirements appended to EVERY terminal instruction
/// via `dispatch_terminal()`. Principle-based — teaches autonomous quality thinking
/// instead of prescribing specific tools.
pub const QUALITY_REQUIREMENTS_SUFFIX: &str = "\n\n---\n\
## MANDATORY QUALITY STANDARDS\n\
\n\
### 1. Build Integrity\n\
Your code MUST compile and build with ZERO errors. Before EVERY commit:\n\
- Run the project's compilation/type-check command (e.g., tsc, cargo check, go build)\n\
- Run the project's lint command if configured\n\
- Fix ALL errors — not just your own files, but any errors your changes introduced across the entire project\n\
- If the project has no build script yet, create one and verify it passes\n\
\n\
Think: \"If someone clones this repo right now and runs the build, will it succeed?\"\n\
\n\
### 2. Testability\n\
Write tests that are SELF-CONTAINED — a developer must be able to run them immediately after cloning the repo and installing dependencies, with NO external services running.\n\
- If your code talks to a database, your tests must mock or embed it (in-memory alternatives, test containers, etc.)\n\
- If your code calls external APIs, your tests must stub those calls\n\
- Include at least unit tests for business logic and one integration test for API endpoints\n\
- Run your tests before committing to verify they actually pass\n\
\n\
Think: \"Can a new team member run these tests on their first day without setting up infrastructure?\"\n\
\n\
### 3. Security by Design\n\
Review your own code for security before committing:\n\
- No secrets in source code — use environment variables with fail-fast validation (crash if missing, never use fallback defaults for secrets)\n\
- Authentication tokens must have appropriate scope separation — access tokens and refresh tokens use different signing secrets\n\
- All external input (HTTP request bodies, query params, headers) must be validated before use\n\
- Provide .env.example with placeholder values documenting all required environment variables\n\
\n\
Think: \"If this codebase were open-sourced right now, would any secrets be exposed? Could a malicious request cause damage?\"\n\
\n\
### 4. Developer Experience\n\
The project must be immediately usable by another developer:\n\
- README.md: what the project does, how to set up, how to run, how to test, API overview\n\
- Docker deployment: Dockerfile + docker-compose.yml for one-command startup of the complete stack\n\
- CI pipeline: automated lint + type-check + test on every push\n\
- .gitignore: exclude build artifacts, dependencies, environment files, IDE configs\n\
- Code formatting: configure consistent formatting for the tech stack\n\
\n\
Think: \"If I handed this repo to a colleague with no context, could they have it running in 10 minutes?\"\n\
\n\
### 5. Code Consistency\n\
- Every type, interface, and schema should be defined ONCE and imported everywhere. Never duplicate definitions.\n\
- Use consistent patterns: same response format across all API endpoints, same error handling approach, same naming conventions\n\
- All function parameters and return values should be properly typed — avoid untyped escape hatches\n\
\n\
Think: \"Does every part of this codebase look like it was written by the same careful developer?\"\n\
\n\
### SELF-VERIFICATION CHECKLIST (run before your final commit)\n\
Before committing, verify each item by actually running the command:\n\
[ ] Project builds/compiles with zero errors\n\
[ ] All tests pass\n\
[ ] No hardcoded secrets in any file\n\
[ ] README exists with setup instructions\n\
[ ] Docker config exists for one-command deployment\n\
[ ] .gitignore is appropriate for the tech stack\n\
[ ] All new code has corresponding tests";

/// Scoped quality requirements for existing-codebase workflows.
///
/// This keeps incremental tasks focused on the requested change while still
/// allowing small, score-critical handoff artifacts when the existing repo lacks
/// them. The boundary is "minimal delivery readiness", not unrelated churn.
pub const QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX: &str = "\n\n---\n\
## MANDATORY QUALITY STANDARDS (Existing Codebase)\n\
\n\
### 1. Preserve Existing Behavior\n\
- Follow the repository's current architecture, scripts, naming, and test style\n\
- Keep the change scoped to the requested task\n\
- Do not rewrite existing Docker, CI, README, or deployment files without a task reason\n\
- If a score-critical handoff artifact is missing and fits the repo type, add the smallest non-invasive version: setup notes, CI for requested checks, .env.example for new env vars, or a minimal Dockerfile for server apps\n\
\n\
Think: \"Did I solve the requested problem without expanding the project's surface area?\"\n\
\n\
### 2. Build and Test the Delta\n\
- Run the existing build/type-check/lint/test commands that apply to your changed code\n\
- Fix all errors introduced by your changes\n\
- If the repository already has unrelated failures, document them in the commit notes instead of broadening scope\n\
- Add or update tests for new or modified logic\n\
- Tests must assert real behavior; never use placeholder tests such as expect(true).toBe(true)\n\
- Coverage configuration must measure changed core layers; do not exclude services, controllers, routes, models, repositories, or middleware to inflate coverage\n\
\n\
Think: \"Can reviewers tell which checks cover my exact change?\"\n\
\n\
### 3. Security and Compatibility\n\
- Do not introduce hardcoded secrets, weak defaults, or unsafe fallback credentials\n\
- Validate new external inputs and preserve existing public APIs unless the task requires a breaking change\n\
- New environment variables must be documented in the existing env/example mechanism\n\
- Avoid ReDoS-prone regular expressions; do not use nested quantifiers on user-controlled input\n\
\n\
### SELF-VERIFICATION CHECKLIST (run before your final commit)\n\
[ ] Existing project checks relevant to my changed code pass, or pre-existing failures are clearly identified\n\
[ ] New/modified behavior has tests\n\
[ ] Missing score-critical handoff artifacts are present or explicitly not applicable\n\
[ ] Tests assert behavior and coverage does not exclude changed core layers\n\
[ ] No unrelated infrastructure/documentation churn\n\
[ ] No new secrets or weak defaults";

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
    fn test_terminal_concurrency_default_is_conservative() {
        assert_eq!(DEFAULT_MAX_CONCURRENT_TERMINALS, 4);
    }

    #[test]
    fn test_workflow_status_ready_value() {
        assert_eq!(WORKFLOW_STATUS_READY, "ready");
    }

    #[test]
    fn test_workflow_status_created_value() {
        assert_eq!(WORKFLOW_STATUS_CREATED, "created");
    }

    #[test]
    fn quality_requirements_suffix_uses_principle_based_approach() {
        // Core principle: autonomous thinking prompts
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("Think:"),
            "Must include Think: prompts for autonomous reasoning"
        );
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("SELF-VERIFICATION"),
            "Must include self-verification checklist"
        );
        // Buildability principle (not tool-specific)
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("ZERO errors"),
            "Must require zero build errors"
        );
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("Build Integrity"),
            "Must have build integrity section"
        );
        // Test principle (self-contained, not tool-specific)
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("SELF-CONTAINED"),
            "Tests must be self-contained"
        );
        // Security principle
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("different signing secrets"),
            "Must require token scope separation"
        );
        // Engineering principle
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("README"),
            "Must require README"
        );
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("Docker"),
            "Must require Docker"
        );
        assert!(
            QUALITY_REQUIREMENTS_SUFFIX.contains("CI pipeline"),
            "Must require CI"
        );
    }

    #[test]
    fn incremental_quality_suffix_avoids_greenfield_artifact_churn() {
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("Existing Codebase"));
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("smallest non-invasive"));
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("minimal Dockerfile"));
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("pre-existing failures"));
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("expect(true).toBe(true)"));
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("ReDoS"));
        assert!(QUALITY_REQUIREMENTS_INCREMENTAL_SUFFIX.contains("No unrelated infrastructure"));
    }

    #[test]
    fn quality_gate_default_mode_is_enforce() {
        assert_eq!(QUALITY_GATE_DEFAULT_MODE, QUALITY_GATE_MODE_ENFORCE);
    }
}
