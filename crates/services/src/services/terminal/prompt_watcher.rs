//! Terminal Prompt Watcher Module
//!
//! Monitors PTY output streams and detects interactive prompts.
//! Publishes `TerminalPromptDetected` events to MessageBus for Orchestrator processing.

use std::{
    collections::HashMap,
    sync::{
        Arc,
        atomic::{AtomicU64, Ordering},
    },
    time::Duration,
};

use once_cell::sync::Lazy;
use regex::Regex;
use tokio::{
    sync::{RwLock, broadcast::error::RecvError, oneshot},
    task::JoinHandle,
    time::Instant,
};

use crate::services::{
    orchestrator::{
        message_bus::SharedMessageBus,
        types::{PromptDecision, PromptState, TerminalPromptEvent, TerminalPromptStateMachine},
    },
    terminal::{
        process::ProcessManager,
        prompt_detector::{
            DetectedPrompt, PromptDetector, PromptKind, normalize_text_for_detection,
        },
    },
};

// ============================================================================
// Constants
// ============================================================================

/// Minimum time between prompt detections for the same terminal (debounce)
const PROMPT_DEBOUNCE_MS: u64 = 500;

/// Timeout for prompt state machine to reset to idle
const PROMPT_STATE_TIMEOUT_SECS: i64 = 30;

/// Timeout for WaitingForApproval state (G07-008): if user doesn't respond
/// within this duration, the state machine resets to Idle.
const WAITING_FOR_APPROVAL_TIMEOUT_SECS: i64 = 300; // 5 minutes

/// Minimum confidence threshold for publishing prompt events
const MIN_CONFIDENCE_THRESHOLD: f32 = 0.7;

/// Auto-reply used when Codex asks whether to continue after detecting
/// "unexpected changes I didn't make" in the working tree.
const UNEXPECTED_CHANGES_CONTINUE_RESPONSE: &str = "Continue with the current workspace state and proceed to complete the task and commit; do not wait for additional confirmation.\n";

/// Max age for line-by-line unexpected-changes follow-up context.
const UNEXPECTED_CHANGES_CONTEXT_MAX_AGE_SECS: u64 = 12;

/// Max age for line-by-line Claude bypass prompt context.
const CLAUDE_BYPASS_CONTEXT_MAX_AGE_SECS: u64 = 8;

/// Delay before re-sending Claude bypass accept when menu is still visible.
const CLAUDE_BYPASS_ACCEPT_RETRY_DELAY_MS: u64 = 900;

/// Max age for pending Claude bypass accept retry state.
const CLAUDE_BYPASS_ACCEPT_RETRY_MAX_AGE_SECS: u64 = 8;

/// Limit retry count to avoid repeated accidental injections.
const CLAUDE_BYPASS_ACCEPT_MAX_RETRIES: u8 = 1;

/// Max age for "clean repo but waiting for next instruction" context.
const HANDOFF_STALL_CONTEXT_MAX_AGE_SECS: u64 = 20;

/// Max age for handoff reminder submit-assist context.
const HANDOFF_SUBMIT_CONTEXT_MAX_AGE_SECS: u64 = 12;

/// Auto-reply used when CLI reports clean workspace and asks what to do next.
/// This prevents orchestrated terminals from idling forever instead of creating
/// the required completion/handoff commit.
const HANDOFF_STALL_CONTINUE_RESPONSE: &str = "Do not wait for additional instructions. \
You must finish your current scoped terminal now. \
If there are no file changes, create an empty commit with --allow-empty and include the exact \
---METADATA--- block from your original instruction (workflow_id/task_id/terminal_id/terminal_order/status/next_action). \
Then stop and hand off to the next terminal.\n";

/// Auto-reply used when Claude reports the configured model is unavailable.
/// This prompt is common on custom gateways when a model alias is invalid or
/// the provided API key lacks access to that model.
const CLAUDE_MODEL_UNAVAILABLE_RECOVERY_INPUT: &str = "/model";

/// Decision rationale text for model-unavailable auto-recovery.
const CLAUDE_MODEL_UNAVAILABLE_CONTINUE_RESPONSE: &str = "The selected model is unavailable for this endpoint. \
Run /model now, pick an available model from the presented list (first/default option is acceptable), \
then continue this same task immediately and complete the required handoff commit. \
Do not wait for additional instructions.\n";

/// Legacy bypass-permissions toggle prompt (Codex-style TUI).
/// Example: "bypass permissions on (shift+tab to cycle)"
static BYPASS_PERMISSIONS_PROMPT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\bbypass\s+permissions\s+(on|off)\b.*\(shift\+tab\s+to\s+cycle\)")
        .expect("BYPASS_PERMISSIONS_PROMPT_RE must compile")
});

/// Codex interactive confirmation prompt (requires y/n)
static CODEX_CONFIRM_APPLY_PATCH_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\bconfirming\s+apply_patch\s+approach\b")
        .expect("CODEX_CONFIRM_APPLY_PATCH_RE must compile")
});

/// Claude custom API key selection prompt.
/// Example:
/// "Detected a custom API key in your environment"
/// "Do you want to use this API key?"
static CLAUDE_CUSTOM_API_KEY_PROMPT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(
        r"(?i)(do\s+you\s+want\s+to\s+use\s+this\s+api\s+key\?|detected\s+a\s+custom\s+api\s+key\s+in\s+your\s+environment)",
    )
    .expect("CLAUDE_CUSTOM_API_KEY_PROMPT_RE must compile")
});

/// Claude bypass list item variants:
/// - "No, exit"
/// - "No,exit"
/// - "No ,  exit"
static CLAUDE_BYPASS_NO_EXIT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\bno\s*,?\s*exit\b").expect("CLAUDE_BYPASS_NO_EXIT_RE must compile")
});

/// Claude bypass acceptance list item variants:
/// - "Yes, I accept"
/// - "Yes,I accept"
/// - "Yes I accept"
static CLAUDE_BYPASS_YES_ACCEPT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\byes\s*,?\s*i\s*accept\b").expect("CLAUDE_BYPASS_YES_ACCEPT_RE must compile")
});

/// Notepad launch/open prompt seen in headless Codex flows on Windows.
/// Example: "Open in Notepad? (y/N)"
static NOTEPAD_PROMPT_RE: Lazy<Regex> = Lazy::new(|| {
    Regex::new(
        r"(?i)\bnotepad\b.{0,200}(\[[^\]]*y\s*/\s*n[^\]]*\]|\([^\)]*y\s*/\s*n[^\)]*\)|\by\s*/\s*n\b|\byes\s*/\s*no\b)",
    )
    .expect("NOTEPAD_PROMPT_RE must compile")
});

fn is_bypass_permissions_prompt(line: &str) -> bool {
    BYPASS_PERMISSIONS_PROMPT_RE.is_match(line)
}

fn is_bypass_permissions_enter_confirm_context(text: &str) -> bool {
    let lower = text.to_ascii_lowercase();
    lower.contains("interrupted")
        || lower.contains("press ctrl-c again to exit")
        || lower.contains("enter to confirm")
        || (lower.contains("press enter") && lower.contains("confirm"))
}

fn is_codex_apply_patch_confirmation(text: &str) -> bool {
    CODEX_CONFIRM_APPLY_PATCH_RE.is_match(text)
}

fn is_claude_custom_api_key_prompt(text: &str) -> bool {
    CLAUDE_CUSTOM_API_KEY_PROMPT_RE.is_match(text)
}

fn is_claude_model_unavailable_prompt(text: &str) -> bool {
    let lower = text.to_ascii_lowercase();
    let has_issue_marker = lower.contains("issue with the selected model")
        || (lower.contains("selected model")
            && (lower.contains("may not exist")
                || lower.contains("may not have access")
                || lower.contains("does not exist")
                || lower.contains("don't have access")));
    let has_model_action_hint = lower.contains("run /model")
        || lower.contains("use /model")
        || lower.contains("pick a different model")
        || lower.contains("choose a different model");
    // OpenAI-compatible gateways may return Claude model outages as 503 with
    // `model_not_found` / `No available channel for model ...`.
    let has_gateway_model_not_found_marker =
        lower.contains("model_not_found") || lower.contains("no available channel for model");

    (has_issue_marker && has_model_action_hint) || has_gateway_model_not_found_marker
}

fn is_notepad_prompt(text: &str) -> bool {
    if NOTEPAD_PROMPT_RE.is_match(text) {
        return true;
    }

    let lower = text.to_ascii_lowercase();
    let has_notepad = lower.contains("notepad");
    if !has_notepad {
        return false;
    }

    // Fallback for prompts split across lines/chunks where y/n tokens may
    // arrive separately: "Open in Notepad?"
    let has_action = lower.contains("open") || lower.contains("launch") || lower.contains("use");
    has_action && lower.contains('?')
}

fn is_unexpected_changes_followup_prompt(text: &str) -> bool {
    let lower = text.to_ascii_lowercase();

    let has_cn_change_marker =
        text.contains("未发起的变更") || text.contains("未执行的变更") || text.contains("外部变更");
    let has_cn_followup = has_cn_change_marker
        && (text.contains("请确认我是否可以")
            || text.contains("请确认是否继续")
            || text.contains("是否继续")
            || text.contains("继续实现任务")
            || text.contains("继续实现并提交")
            || text.contains("你希望我"));

    let has_en_change_marker = lower.contains("changes i didn't make")
        || lower.contains("changes i did not make")
        || lower.contains("unexpected changes");
    let has_en_followup = has_en_change_marker
        && (lower.contains("should i continue")
            || lower.contains("continue implementing")
            || lower.contains("wait for you")
            || lower.contains("proceed"));

    has_cn_followup || has_en_followup
}

fn normalize_handoff_marker_text(text: &str) -> String {
    text.to_ascii_lowercase()
        .replace(['\u{2019}', '\u{2018}'], "'")
        .replace(['\u{2013}', '\u{2014}'], " ")
}

fn has_handoff_stall_clean_marker(lower: &str) -> bool {
    lower.contains("nothing to commit, working tree clean")
        || lower.contains("working tree clean")
        || lower.contains("working tree is clean")
        || lower.contains("repository is clean")
        || lower.contains("checkout is clean")
        || lower.contains("no outstanding changes")
        || lower.contains("no staged/unstaged diffs")
        || lower.contains("no diff to review right now")
        || lower.contains("status clean")
        || lower.contains("no diff")
        || lower.contains("git status clean")
}

fn has_handoff_stall_wait_marker(lower: &str) -> bool {
    let asks_what_like_me_to = lower.contains("what")
        && lower.contains("like me to")
        && (lower.contains("work on")
            || lower.contains("do next")
            || lower.contains("implement")
            || lower.contains("change"));
    let asks_let_me_know = lower.contains("let me know")
        && (lower.contains("work on")
            || lower.contains("do next")
            || lower.contains("implement")
            || lower.contains("change"));
    let asks_share_what = lower.contains("share what")
        && lower.contains("like me to")
        && (lower.contains("work on") || lower.contains("implement") || lower.contains("change"));
    let asks_describe_specific = lower.contains("could you describe")
        && (lower.contains("specific change or feature")
            || lower.contains("specific change")
            || lower.contains("specific feature")
            || lower.contains("like me to work on"));
    let asks_proceed_next = lower.contains("how would you like to proceed next")
        || (lower.contains("how would you like") && lower.contains("proceed next"))
        || (lower.contains("would you like to proceed") && lower.contains("next"));

    asks_what_like_me_to
        || asks_let_me_know
        || asks_share_what
        || asks_describe_specific
        || asks_proceed_next
        || lower.contains("what would you like me to work on next")
        || (lower.contains("could you clarify what") && lower.contains("work on next"))
        || (lower.contains("could you clarify what") && lower.contains("work on first"))
        || (lower.contains("what changes or task") && lower.contains("work on first"))
        || (lower.contains("let me know what") && lower.contains("work on next"))
        || (lower.contains("let me know what") && lower.contains("like me to do next"))
        || (lower.contains("let me know") && lower.contains("work on"))
        || (lower.contains("let me know") && lower.contains("do next"))
        || (lower.contains("what") && lower.contains("like me to work on"))
        || (lower.contains("what you")
            && lower.contains("like me to")
            && lower.contains("work on next"))
        || lower.contains("share the changes you want me to make")
        || lower.contains("what you'd like me to implement")
        || lower.contains("what you would like me to implement")
        || lower.contains("what changes you'd like me to implement")
        || lower.contains("what changes you would like me to implement")
        || lower.contains("what would you like me to do next")
        || lower.contains("what you'd like me to do next")
        || lower.contains("what you would like me to do next")
        || lower.contains("you'd like me to work on")
        || lower.contains("youd like me to work on")
}

fn has_handoff_stall_scope_gap_marker(lower: &str) -> bool {
    (lower.contains("specific requirements") && lower.contains("files to modify"))
        || lower.contains("don't have any specific requirements")
        || lower.contains("do not have any specific requirements")
        || lower.contains("don't have a specific task yet")
        || lower.contains("do not have a specific task yet")
        || lower.contains("don't have a task yet")
        || lower.contains("do not have a task yet")
        || lower.contains("no further instructions were provided")
        || (lower.contains("ready to start implementing")
            && lower.contains("specific change or feature"))
        || lower.contains("no specific requirements")
        || lower.contains("no actionable change")
        || lower.contains("no actionable changes")
}

fn is_handoff_stall_prompt(text: &str) -> bool {
    let lower = normalize_handoff_marker_text(text);
    let has_wait = has_handoff_stall_wait_marker(&lower);
    has_wait
        && (has_handoff_stall_clean_marker(&lower) || has_handoff_stall_scope_gap_marker(&lower))
}

fn has_claude_bypass_mode_text(lower: &str) -> bool {
    lower.contains("bypass permissions mode")
}

fn has_claude_bypass_no_exit_text(lower: &str) -> bool {
    CLAUDE_BYPASS_NO_EXIT_RE.is_match(lower)
}

fn has_claude_bypass_yes_accept_text(lower: &str) -> bool {
    CLAUDE_BYPASS_YES_ACCEPT_RE.is_match(lower)
}

fn has_claude_bypass_confirm_hint_text(lower: &str) -> bool {
    lower.contains("enter to confirm") || (lower.contains("enter") && lower.contains("confirm"))
}

fn has_claude_bypass_accept_menu_text(lower: &str) -> bool {
    has_claude_bypass_mode_text(lower)
        && has_claude_bypass_no_exit_text(lower)
        && has_claude_bypass_yes_accept_text(lower)
}

fn is_claude_bypass_accept_prompt(text: &str) -> bool {
    let lower = text.to_ascii_lowercase();
    has_claude_bypass_accept_menu_text(&lower)
}

/// [G19-010] The numeric shortcut "2\r" assumes "Yes, I accept" is always the
/// second menu item. If Claude CLI reorders the menu in a future version this
/// will break. TODO: implement regex-based matching that scans the rendered
/// menu lines for the "Yes" option index instead of hardcoding the position.
fn claude_bypass_accept_response(_text: &str) -> (&'static str, &'static str, &'static str) {
    (
        "2\r",
        "select 'Yes, I accept' via numeric shortcut and confirm",
        "Select 'Yes, I accept' via numeric shortcut (2) and confirm for Claude bypass permissions prompt",
    )
}

fn is_claude_bypass_accept_context_line(lower: &str) -> bool {
    has_claude_bypass_mode_text(lower)
        || has_claude_bypass_no_exit_text(lower)
        || has_claude_bypass_yes_accept_text(lower)
        || has_claude_bypass_confirm_hint_text(lower)
}

fn has_any_claude_bypass_marker(lower: &str) -> bool {
    has_claude_bypass_mode_text(lower)
        || has_claude_bypass_no_exit_text(lower)
        || has_claude_bypass_yes_accept_text(lower)
        || has_claude_bypass_confirm_hint_text(lower)
}

// ============================================================================
// Terminal Watch State
// ============================================================================

#[derive(Debug, Default)]
struct ClaudeBypassContext {
    saw_mode: bool,
    saw_no_exit: bool,
    saw_yes_accept: bool,
    saw_confirm_hint: bool,
    last_updated: Option<Instant>,
}

impl ClaudeBypassContext {
    fn observe(&mut self, lower_text: &str) {
        let mut touched = false;

        if has_claude_bypass_mode_text(lower_text) && !self.saw_mode {
            self.saw_mode = true;
            touched = true;
        }

        if has_claude_bypass_no_exit_text(lower_text) && !self.saw_no_exit {
            self.saw_no_exit = true;
            touched = true;
        }

        if has_claude_bypass_yes_accept_text(lower_text) && !self.saw_yes_accept {
            self.saw_yes_accept = true;
            touched = true;
        }

        if has_claude_bypass_confirm_hint_text(lower_text) && !self.saw_confirm_hint {
            self.saw_confirm_hint = true;
            touched = true;
        }

        if touched {
            self.last_updated = Some(Instant::now());
        }
    }

    fn is_complete_and_recent(&self) -> bool {
        if !(self.saw_mode && self.saw_no_exit && self.saw_yes_accept) {
            return false;
        }

        self.last_updated.is_some_and(|ts| {
            ts.elapsed() <= Duration::from_secs(CLAUDE_BYPASS_CONTEXT_MAX_AGE_SECS)
        })
    }

    fn clear(&mut self) {
        *self = Self::default();
    }
}

#[derive(Debug, Default)]
struct UnexpectedChangesContext {
    saw_change_marker: bool,
    saw_pause_marker: bool,
    saw_followup_marker: bool,
    last_updated: Option<Instant>,
}

impl UnexpectedChangesContext {
    fn observe(&mut self, text: &str) {
        let lower = text.to_ascii_lowercase();
        let mut touched = false;

        let has_change_marker = text.contains("未发起的变更")
            || text.contains("未执行的变更")
            || text.contains("外部变更")
            || lower.contains("changes i didn't make")
            || lower.contains("changes i did not make")
            || lower.contains("unexpected changes");
        if has_change_marker && !self.saw_change_marker {
            self.saw_change_marker = true;
            touched = true;
        }

        let has_pause_marker = text.contains("需要先暂停")
            || text.contains("先暂停")
            || lower.contains("need to pause")
            || lower.contains("must pause");
        if has_pause_marker && !self.saw_pause_marker {
            self.saw_pause_marker = true;
            touched = true;
        }

        let has_followup_marker = text.contains("请确认我是否可以")
            || text.contains("请确认是否继续")
            || text.contains("是否继续")
            || text.contains("继续实现任务")
            || text.contains("继续实现并提交")
            || text.contains("你希望我")
            || lower.contains("should i continue")
            || lower.contains("continue implementing")
            || lower.contains("wait for you")
            || lower.contains("proceed");
        if has_followup_marker && !self.saw_followup_marker {
            self.saw_followup_marker = true;
            touched = true;
        }

        if touched {
            self.last_updated = Some(Instant::now());
        }
    }

    fn is_complete_and_recent(&self) -> bool {
        if !(self.saw_change_marker && (self.saw_followup_marker || self.saw_pause_marker)) {
            return false;
        }

        self.last_updated.is_some_and(|ts| {
            ts.elapsed() <= Duration::from_secs(UNEXPECTED_CHANGES_CONTEXT_MAX_AGE_SECS)
        })
    }

    fn clear(&mut self) {
        *self = Self::default();
    }
}

#[derive(Debug, Default)]
struct HandoffStallContext {
    saw_clean_marker: bool,
    saw_wait_marker: bool,
    saw_scope_gap_marker: bool,
    last_updated: Option<Instant>,
}

impl HandoffStallContext {
    fn observe(&mut self, text: &str) {
        let lower = normalize_handoff_marker_text(text);
        let mut touched = false;

        if has_handoff_stall_clean_marker(&lower) && !self.saw_clean_marker {
            self.saw_clean_marker = true;
            touched = true;
        }

        if has_handoff_stall_wait_marker(&lower) && !self.saw_wait_marker {
            self.saw_wait_marker = true;
            touched = true;
        }

        if has_handoff_stall_scope_gap_marker(&lower) && !self.saw_scope_gap_marker {
            self.saw_scope_gap_marker = true;
            touched = true;
        }

        if touched {
            self.last_updated = Some(Instant::now());
        }
    }

    fn is_complete_and_recent(&self) -> bool {
        if !((self.saw_clean_marker || self.saw_scope_gap_marker) && self.saw_wait_marker) {
            return false;
        }

        self.last_updated.is_some_and(|ts| {
            ts.elapsed() <= Duration::from_secs(HANDOFF_STALL_CONTEXT_MAX_AGE_SECS)
        })
    }

    fn clear(&mut self) {
        *self = Self::default();
    }
}

/// State for a single watched terminal
#[derive(Debug)]
struct TerminalWatchState {
    /// Terminal ID
    terminal_id: String,
    /// Workflow ID
    workflow_id: String,
    /// Task ID
    task_id: String,
    /// PTY session ID
    session_id: String,
    /// Whether auto-confirm is enabled for this terminal
    auto_confirm: bool,
    /// Prompt detector instance
    detector: PromptDetector,
    /// Prompt state machine
    state_machine: TerminalPromptStateMachine,
    /// Last detection timestamp (for debouncing)
    last_detection: Option<Instant>,
    /// Rolling context for line-by-line Claude bypass prompt rendering.
    claude_bypass_context: ClaudeBypassContext,
    /// Rolling context for line-by-line unexpected-changes follow-up prompts.
    unexpected_changes_context: UnexpectedChangesContext,
    /// Rolling context for "clean workspace + waiting for next instruction".
    handoff_stall_context: HandoffStallContext,
    /// If handoff reminder text was injected while renderer was busy, send one
    /// extra Enter on next bypass status-line to force submission.
    pending_handoff_submit_at: Option<Instant>,
    /// Timestamp of initial Claude bypass auto-accept send, used for one retry
    /// if the same menu remains visible.
    pending_claude_bypass_retry_since: Option<Instant>,
    /// Number of Claude bypass retries sent for current prompt.
    claude_bypass_retry_count: u8,
}

impl TerminalWatchState {
    fn new(
        terminal_id: String,
        workflow_id: String,
        task_id: String,
        session_id: String,
        auto_confirm: bool,
    ) -> Self {
        Self {
            terminal_id,
            workflow_id,
            task_id,
            session_id,
            auto_confirm,
            detector: PromptDetector::new(),
            state_machine: TerminalPromptStateMachine::new(),
            last_detection: None,
            claude_bypass_context: ClaudeBypassContext::default(),
            unexpected_changes_context: UnexpectedChangesContext::default(),
            handoff_stall_context: HandoffStallContext::default(),
            pending_handoff_submit_at: None,
            pending_claude_bypass_retry_since: None,
            claude_bypass_retry_count: 0,
        }
    }

    /// Check if enough time has passed since last detection (debounce)
    fn should_debounce(&self) -> bool {
        if let Some(last) = self.last_detection {
            last.elapsed() < Duration::from_millis(PROMPT_DEBOUNCE_MS)
        } else {
            false
        }
    }

    /// Process a line of output and return detected prompt if any
    fn process_line(&mut self, line: &str) -> Option<DetectedPrompt> {
        // Check debounce
        if self.should_debounce() {
            return None;
        }

        // Detect prompt
        let prompt = self.detector.process_line(line)?;

        // Check confidence threshold
        if prompt.confidence < MIN_CONFIDENCE_THRESHOLD {
            return None;
        }

        // Check state machine
        if !self.state_machine.should_process(&prompt) {
            return None;
        }

        // Update state
        self.last_detection = Some(Instant::now());
        self.state_machine.on_prompt_detected(prompt.clone());

        Some(prompt)
    }

    /// Reset state machine if stale.
    ///
    /// Uses a longer timeout for WaitingForApproval (G07-008) since that state
    /// legitimately waits for user input, but should still auto-reset eventually.
    fn check_and_reset_stale(&mut self) {
        let timeout_secs = match self.state_machine.state {
            crate::services::orchestrator::types::PromptState::WaitingForApproval => {
                WAITING_FOR_APPROVAL_TIMEOUT_SECS
            }
            _ => PROMPT_STATE_TIMEOUT_SECS,
        };
        let timeout = chrono::Duration::seconds(timeout_secs);
        if self.state_machine.is_stale(timeout) {
            if matches!(
                self.state_machine.state,
                crate::services::orchestrator::types::PromptState::WaitingForApproval
            ) {
                tracing::warn!(
                    "WaitingForApproval state timed out after {}s, resetting to Idle",
                    WAITING_FOR_APPROVAL_TIMEOUT_SECS
                );
            }
            self.state_machine.reset();
            self.detector.clear_buffer();
            self.claude_bypass_context.clear();
            self.unexpected_changes_context.clear();
            self.handoff_stall_context.clear();
            self.pending_handoff_submit_at = None;
            self.clear_claude_bypass_retry_state();
        }
    }

    fn observe_claude_bypass_context(&mut self, text: &str) {
        self.claude_bypass_context.observe(text);
    }

    fn has_recent_claude_bypass_accept_context(&self) -> bool {
        self.claude_bypass_context.is_complete_and_recent()
    }

    fn clear_claude_bypass_context(&mut self) {
        self.claude_bypass_context.clear();
    }

    fn observe_unexpected_changes_context(&mut self, text: &str) {
        self.unexpected_changes_context.observe(text);
    }

    fn has_recent_unexpected_changes_context(&self) -> bool {
        self.unexpected_changes_context.is_complete_and_recent()
    }

    fn clear_unexpected_changes_context(&mut self) {
        self.unexpected_changes_context.clear();
    }

    fn observe_handoff_stall_context(&mut self, text: &str) {
        self.handoff_stall_context.observe(text);
    }

    fn has_recent_handoff_stall_context(&self) -> bool {
        self.handoff_stall_context.is_complete_and_recent()
    }

    fn clear_handoff_stall_context(&mut self) {
        self.handoff_stall_context.clear();
    }

    #[allow(dead_code)]
    fn mark_pending_handoff_submit(&mut self) {
        self.pending_handoff_submit_at = Some(Instant::now());
    }

    fn should_force_handoff_submit(&self) -> bool {
        self.pending_handoff_submit_at.is_some_and(|ts| {
            ts.elapsed() <= Duration::from_secs(HANDOFF_SUBMIT_CONTEXT_MAX_AGE_SECS)
        })
    }

    fn clear_pending_handoff_submit(&mut self) {
        self.pending_handoff_submit_at = None;
    }

    fn mark_claude_bypass_accept_sent(&mut self) {
        self.pending_claude_bypass_retry_since = Some(Instant::now());
        self.claude_bypass_retry_count = 0;
    }

    fn should_retry_claude_bypass_accept(&self) -> bool {
        let Some(since) = self.pending_claude_bypass_retry_since else {
            return false;
        };
        if self.claude_bypass_retry_count >= CLAUDE_BYPASS_ACCEPT_MAX_RETRIES {
            return false;
        }
        let elapsed = since.elapsed();
        elapsed >= Duration::from_millis(CLAUDE_BYPASS_ACCEPT_RETRY_DELAY_MS)
            && elapsed <= Duration::from_secs(CLAUDE_BYPASS_ACCEPT_RETRY_MAX_AGE_SECS)
    }

    fn mark_claude_bypass_retry_sent(&mut self) {
        self.claude_bypass_retry_count = self.claude_bypass_retry_count.saturating_add(1);
        self.pending_claude_bypass_retry_since = None;
    }

    fn clear_claude_bypass_retry_state(&mut self) {
        self.pending_claude_bypass_retry_since = None;
        self.claude_bypass_retry_count = 0;
    }
}

// ============================================================================
// Prompt Watcher
// ============================================================================

struct WatchTaskHandle {
    task_id: u64,
    task_handle: JoinHandle<()>,
}

/// Watches PTY output for interactive prompts and publishes events
///
/// [G07-001] The `terminals` map uses a single RwLock for all terminals. Under
/// high terminal counts this could become a contention point. TODO: consider
/// refactoring to per-terminal Mutex (e.g., `DashMap<String, Mutex<TerminalWatchState>>`)
/// to allow concurrent processing of different terminals.
#[derive(Clone)]
pub struct PromptWatcher {
    /// Message bus for publishing events
    message_bus: SharedMessageBus,
    /// Process manager for OutputFanout subscriptions
    process_manager: Arc<ProcessManager>,
    /// Watched terminals state
    terminals: Arc<RwLock<HashMap<String, TerminalWatchState>>>,
    /// Active background output subscriptions by terminal_id
    active_subscriptions: Arc<RwLock<HashMap<String, WatchTaskHandle>>>,
    /// Monotonic task ID for safe replacement/cleanup
    next_task_id: Arc<AtomicU64>,
}

impl PromptWatcher {
    /// Create a new prompt watcher
    pub fn new(message_bus: SharedMessageBus, process_manager: Arc<ProcessManager>) -> Self {
        Self {
            message_bus,
            process_manager,
            terminals: Arc::new(RwLock::new(HashMap::new())),
            active_subscriptions: Arc::new(RwLock::new(HashMap::new())),
            next_task_id: Arc::new(AtomicU64::new(1)),
        }
    }

    /// Register a terminal for watching
    pub async fn register(
        &self,
        terminal_id: &str,
        workflow_id: &str,
        task_id: &str,
        session_id: &str,
        auto_confirm: bool,
    ) -> anyhow::Result<()> {
        let state = TerminalWatchState::new(
            terminal_id.to_string(),
            workflow_id.to_string(),
            task_id.to_string(),
            session_id.to_string(),
            auto_confirm,
        );

        {
            let mut terminals = self.terminals.write().await;
            terminals.insert(terminal_id.to_string(), state);
        }
        if let Err(e) = self.spawn_output_subscription_task(terminal_id).await {
            let mut terminals = self.terminals.write().await;
            terminals.remove(terminal_id);
            return Err(e);
        }

        tracing::debug!(
            terminal_id = %terminal_id,
            workflow_id = %workflow_id,
            session_id = %session_id,
            auto_confirm,
            "Registered terminal for prompt watching"
        );
        Ok(())
    }

    /// Unregister a terminal from watching
    pub async fn unregister(&self, terminal_id: &str) {
        {
            let mut terminals = self.terminals.write().await;
            terminals.remove(terminal_id);
        }
        let task_handle = {
            let mut active_subscriptions = self.active_subscriptions.write().await;
            active_subscriptions
                .remove(terminal_id)
                .map(|handle| handle.task_handle)
        };
        if let Some(task_handle) = task_handle {
            task_handle.abort();
        }

        tracing::debug!(
            terminal_id = %terminal_id,
            "Unregistered terminal from prompt watching"
        );
    }

    async fn spawn_output_subscription_task(&self, terminal_id: &str) -> anyhow::Result<()> {
        let task_id = self.next_task_id.fetch_add(1, Ordering::Relaxed);
        let process_manager = Arc::clone(&self.process_manager);
        let watcher = self.clone();
        let active_subscriptions = Arc::clone(&self.active_subscriptions);
        let terminal_id_for_task = terminal_id.to_string();
        let (start_tx, start_rx) = oneshot::channel::<()>();
        let (ready_tx, ready_rx) = oneshot::channel::<anyhow::Result<()>>();

        let task_handle = tokio::spawn(async move {
            if start_rx.await.is_err() {
                return;
            }

            let mut subscription = match process_manager
                .subscribe_output(&terminal_id_for_task, None)
                .await
            {
                Ok(subscription) => {
                    let _ = ready_tx.send(Ok(()));
                    tracing::debug!(
                        terminal_id = %terminal_id_for_task,
                        "PromptWatcher subscribed to terminal output fanout"
                    );
                    subscription
                }
                Err(e) => {
                    let _ = ready_tx.send(Err(anyhow::anyhow!(
                        "PromptWatcher subscription failed: {e}"
                    )));
                    tracing::warn!(
                        terminal_id = %terminal_id_for_task,
                        error = %e,
                        "Failed to subscribe PromptWatcher to terminal output"
                    );
                    Self::remove_subscription_if_current(
                        &active_subscriptions,
                        &terminal_id_for_task,
                        task_id,
                    )
                    .await;
                    return;
                }
            };

            loop {
                match subscription.recv().await {
                    Ok(chunk) => {
                        if !chunk.text.is_empty() {
                            watcher
                                .process_output(&terminal_id_for_task, &chunk.text)
                                .await;
                        }
                        if chunk.dropped_invalid_bytes > 0 {
                            tracing::warn!(
                                terminal_id = %terminal_id_for_task,
                                seq = chunk.seq,
                                dropped_bytes = chunk.dropped_invalid_bytes,
                                "Dropped invalid UTF-8 bytes in prompt watcher stream"
                            );
                        }
                    }
                    Err(RecvError::Lagged(skipped)) => {
                        tracing::warn!(
                            terminal_id = %terminal_id_for_task,
                            skipped = %skipped,
                            "PromptWatcher output subscription lagged"
                        );
                    }
                    Err(RecvError::Closed) => {
                        break;
                    }
                }
            }

            Self::remove_subscription_if_current(
                &active_subscriptions,
                &terminal_id_for_task,
                task_id,
            )
            .await;
        });

        let replaced = {
            let mut active_subscriptions = self.active_subscriptions.write().await;
            active_subscriptions.insert(
                terminal_id.to_string(),
                WatchTaskHandle {
                    task_id,
                    task_handle,
                },
            )
        };
        if let Some(previous) = replaced {
            previous.task_handle.abort();
        }

        let _ = start_tx.send(());
        match tokio::time::timeout(Duration::from_secs(2), ready_rx).await {
            Ok(Ok(Ok(()))) => Ok(()),
            Ok(Ok(Err(e))) => Err(e),
            Ok(Err(_)) => Err(anyhow::anyhow!(
                "PromptWatcher startup acknowledgment channel closed"
            )),
            Err(_) => Err(anyhow::anyhow!(
                "Timed out waiting for PromptWatcher output subscription startup"
            )),
        }
    }

    async fn remove_subscription_if_current(
        active_subscriptions: &Arc<RwLock<HashMap<String, WatchTaskHandle>>>,
        terminal_id: &str,
        task_id: u64,
    ) {
        let mut active_subscriptions = active_subscriptions.write().await;
        let should_remove = matches!(
            active_subscriptions.get(terminal_id),
            Some(handle) if handle.task_id == task_id
        );
        if should_remove {
            active_subscriptions.remove(terminal_id);
        }
    }

    async fn try_direct_terminal_input(
        &self,
        terminal_id: &str,
        expected_session_id: &str,
        input: &str,
    ) -> bool {
        let Some(handle) = self.process_manager.get_handle(terminal_id).await else {
            return false;
        };

        if handle.session_id.trim() != expected_session_id.trim() {
            tracing::warn!(
                terminal_id = %terminal_id,
                expected_session_id = %expected_session_id,
                active_session_id = %handle.session_id,
                "Skip direct PTY auto-input due to session mismatch"
            );
            return false;
        }

        let Some(writer) = handle.writer else {
            tracing::warn!(
                terminal_id = %terminal_id,
                expected_session_id = %expected_session_id,
                "Skip direct PTY auto-input: missing PTY writer"
            );
            return false;
        };

        let mut guard = match writer.lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };

        if let Err(e) = guard.write_all(input.as_bytes()) {
            tracing::warn!(
                terminal_id = %terminal_id,
                expected_session_id = %expected_session_id,
                error = %e,
                "Direct PTY auto-input write failed"
            );
            return false;
        }
        if let Err(e) = guard.flush() {
            tracing::warn!(
                terminal_id = %terminal_id,
                expected_session_id = %expected_session_id,
                error = %e,
                "Direct PTY auto-input flush failed"
            );
            return false;
        }
        true
    }

    fn normalize_input_for_direct_write(input: &str) -> String {
        let mut payload = input.to_string();
        if payload.ends_with("\r\n") {
            payload.truncate(payload.len() - 2);
            payload.push('\r');
            return payload;
        }
        if payload.ends_with('\n') {
            payload.pop();
            payload.push('\r');
            return payload;
        }
        if payload.ends_with('\r') {
            return payload;
        }
        payload.push('\r');
        payload
    }

    fn build_handoff_stall_continue_response(
        workflow_id: &str,
        task_id: &str,
        terminal_id: &str,
    ) -> String {
        format!(
            "{HANDOFF_STALL_CONTINUE_RESPONSE}\n\
Use this exact metadata mapping in the commit message (do not swap or leave blank):\n\
---METADATA---\n\
workflow_id: {workflow_id}\n\
task_id: {task_id}\n\
terminal_id: {terminal_id}\n\
terminal_order: <copy from your original terminal instruction>\n\
status: completed\n\
next_action: handoff\n"
        )
    }

    async fn resolve_terminal_input_session_id(
        &self,
        terminal_id: &str,
        preferred_session_id: &str,
    ) -> String {
        let preferred = preferred_session_id.trim();
        let Some(handle) = self.process_manager.get_handle(terminal_id).await else {
            return preferred.to_string();
        };

        let active = handle.session_id.trim();
        if active.is_empty() {
            return preferred.to_string();
        }

        if !preferred.is_empty() && preferred != active {
            tracing::warn!(
                terminal_id = %terminal_id,
                preferred_session_id = %preferred_session_id,
                active_session_id = %handle.session_id,
                "PromptWatcher terminal-input session mismatch; using active PTY session for message-bus fallback"
            );
        }

        active.to_string()
    }

    /// [G07-006] NOTE: `publish_terminal_input` returns a bool indicating delivery success.
    /// If delivery fails (no subscribers), the state machine remains in Responding state
    /// and the prompt may be silently dropped. TODO: Check the return value and reset the
    /// state machine to Idle on delivery failure so the prompt can be re-detected.
    async fn publish_terminal_input_with_active_session(
        &self,
        terminal_id: &str,
        preferred_session_id: &str,
        input: &str,
        decision: Option<PromptDecision>,
    ) {
        let target_session_id = self
            .resolve_terminal_input_session_id(terminal_id, preferred_session_id)
            .await;
        self.message_bus
            .publish_terminal_input(terminal_id, &target_session_id, input, decision)
            .await;
    }

    async fn send_claude_bypass_accept_with_fallback(
        &self,
        terminal_id: &str,
        session_id: &str,
        response: &str,
        decision: PromptDecision,
        mode: &'static str,
        is_retry: bool,
    ) {
        let direct_input = Self::normalize_input_for_direct_write(response);
        let sent_direct = self
            .try_direct_terminal_input(terminal_id, session_id, &direct_input)
            .await;
        if !sent_direct {
            tracing::warn!(
                terminal_id = %terminal_id,
                session_id = %session_id,
                mode,
                is_retry,
                "Direct PTY Claude bypass auto-accept failed; falling back to message bus"
            );
            self.publish_terminal_input_with_active_session(
                terminal_id,
                session_id,
                response,
                Some(decision),
            )
            .await;
        }
    }

    /// Process PTY output for a terminal
    ///
    /// Call this method with each line of PTY output.
    /// If a prompt is detected, publishes a `TerminalPromptDetected` event.
    ///
    /// [G07-003] NOTE: Several special prompt paths (Claude bypass, unexpected-changes,
    /// handoff-stall) bypass the `auto_confirm` guard and always auto-respond. This is
    /// intentional for orchestrated workflows but may surprise users who set auto_confirm=false
    /// expecting full manual control. TODO: Add a strict mode that respects auto_confirm
    /// for all prompt paths, or document the bypass behavior clearly in user-facing docs.
    pub async fn process_output(&self, terminal_id: &str, output: &str) {
        let mut terminals = self.terminals.write().await;

        let state = if let Some(s) = terminals.get_mut(terminal_id) { s } else {
            tracing::trace!(
                terminal_id = %terminal_id,
                "Terminal not registered for prompt watching, ignoring output"
            );
            return;
        };

        // Check and reset stale state
        state.check_and_reset_stale();

        // Chunk-level fallback: Claude custom API key prompt can appear inside
        // heavily ANSI-rendered TUI frames without clean newline boundaries.
        // Detect directly from normalized chunk and auto-reply with ArrowUp+Enter
        // to choose "Yes".
        let normalized_output = normalize_text_for_detection(output);
        let normalized_output_lower = normalized_output.to_ascii_lowercase();
        state.observe_claude_bypass_context(&normalized_output_lower);
        state.observe_unexpected_changes_context(&normalized_output);
        state.observe_handoff_stall_context(&normalized_output);
        let bypass_needs_enter_context = normalized_output_lower.contains("interrupted")
            || normalized_output_lower.contains("press ctrl-c again to exit");
        let has_claude_bypass_accept_context = is_claude_bypass_accept_prompt(&normalized_output)
            || state.has_recent_claude_bypass_accept_context();
        let has_claude_custom_api_key_context = is_claude_custom_api_key_prompt(&normalized_output);
        let has_unexpected_changes_followup_context =
            is_unexpected_changes_followup_prompt(&normalized_output)
                || state.has_recent_unexpected_changes_context();
        let has_handoff_stall_context =
            is_handoff_stall_prompt(&normalized_output) || state.has_recent_handoff_stall_context();

        if !state.should_debounce()
            && state.should_force_handoff_submit()
            && is_bypass_permissions_prompt(&normalized_output)
        {
            let decision = PromptDecision::auto_enter();
            state.last_detection = Some(Instant::now());
            state.state_machine.on_response_sent(decision.clone());
            state.detector.clear_buffer();
            state.clear_pending_handoff_submit();

            let response_terminal_id = state.terminal_id.clone();
            let response_session_id = state.session_id.clone();

            tracing::info!(
                terminal_id = %response_terminal_id,
                session_id = %response_session_id,
                "Detected bypass status-line after handoff reminder (chunk); sending extra Enter to submit queued input"
            );

            drop(terminals);
            self.publish_terminal_input_with_active_session(
                &response_terminal_id,
                &response_session_id,
                "\n",
                Some(decision),
            )
            .await;
            return;
        }

        let has_any_claude_bypass_chunk_marker =
            has_any_claude_bypass_marker(&normalized_output_lower);
        if state.auto_confirm && has_any_claude_bypass_chunk_marker {
            tracing::info!(
                terminal_id = %state.terminal_id,
                session_id = %state.session_id,
                has_mode = has_claude_bypass_mode_text(&normalized_output_lower),
                has_no_exit = has_claude_bypass_no_exit_text(&normalized_output_lower),
                has_yes_accept = has_claude_bypass_yes_accept_text(&normalized_output_lower),
                has_confirm_hint = has_claude_bypass_confirm_hint_text(&normalized_output_lower),
                chunk_accept_context = has_claude_bypass_accept_context,
                chunk_len = normalized_output.len(),
                "Observed Claude bypass prompt markers in chunk"
            );
        }

        if state.auto_confirm
            && !state.should_debounce()
            && has_any_claude_bypass_chunk_marker
            && state.should_retry_claude_bypass_accept()
        {
            let (response, action, reasoning) = claude_bypass_accept_response(&normalized_output);
            let decision = PromptDecision::LLMDecision {
                response: response.to_string(),
                reasoning: format!("{reasoning} (retry once because prompt is still visible)"),
                target_index: Some(1),
            };
            state.last_detection = Some(Instant::now());
            state.state_machine.on_response_sent(decision.clone());
            state.detector.clear_buffer();
            state.clear_claude_bypass_context();
            state.mark_claude_bypass_retry_sent();

            let response_terminal_id = state.terminal_id.clone();
            let response_session_id = state.session_id.clone();

            tracing::info!(
                terminal_id = %response_terminal_id,
                session_id = %response_session_id,
                action = %action,
                "Claude bypass prompt still visible after auto-accept; retrying once (chunk)"
            );

            drop(terminals);
            self.send_claude_bypass_accept_with_fallback(
                &response_terminal_id,
                &response_session_id,
                response,
                decision,
                "chunk",
                true,
            )
            .await;
            return;
        }

        // Chunk-level fallback: Claude bypass-permissions acceptance prompt.
        // Select "Yes, I accept" via numeric shortcut "2" + Enter.
        // This is more stable than ArrowDown in fast ANSI frame updates.
        if state.auto_confirm && !state.should_debounce() && has_claude_bypass_accept_context {
            let (response, action, reasoning) = claude_bypass_accept_response(&normalized_output);
            let decision = PromptDecision::LLMDecision {
                response: response.to_string(),
                reasoning: reasoning.to_string(),
                target_index: Some(1),
            };
            let detected_prompt =
                DetectedPrompt::new(PromptKind::ArrowSelect, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_claude_bypass_context();
                state.mark_claude_bypass_accept_sent();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    action = %action,
                    "Detected Claude bypass permissions prompt (chunk); sending auto-accept sequence"
                );

                drop(terminals);
                self.send_claude_bypass_accept_with_fallback(
                    &response_terminal_id,
                    &response_session_id,
                    response,
                    decision,
                    "chunk",
                    false,
                )
                .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level Claude bypass accept fallback injection"
            );
        }

        if state.auto_confirm && !state.should_debounce() && has_claude_custom_api_key_context {
            let decision = PromptDecision::LLMDecision {
                response: "\u{1b}[A\n".to_string(),
                reasoning: "Auto-select 'Yes' for custom API key prompt via ArrowUp + Enter"
                    .to_string(),
                target_index: Some(0),
            };
            let detected_prompt =
                DetectedPrompt::new(PromptKind::ArrowSelect, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected custom API key selection prompt (chunk); sending ArrowUp + Enter to choose Yes"
                );

                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "\u{1b}[A\n",
                        Some(decision),
                    )
                    .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level custom API key fallback injection"
            );
        }

        // Chunk-level fallback: Claude can report the configured model as
        // unavailable on custom endpoints and then idle forever. Auto-instruct
        // the agent to switch model via /model and continue.
        if state.auto_confirm
            && !state.should_debounce()
            && is_claude_model_unavailable_prompt(&normalized_output)
        {
            let decision = PromptDecision::LLMDecision {
                response: CLAUDE_MODEL_UNAVAILABLE_CONTINUE_RESPONSE.to_string(),
                reasoning:
                    "Auto-recover Claude terminal when configured model is unavailable on current endpoint"
                        .to_string(),
                target_index: None,
            };
            let detected_prompt =
                DetectedPrompt::new(PromptKind::Input, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected Claude model-unavailable prompt (chunk); sending /model recovery command"
                );

                drop(terminals);
                let direct_input = Self::normalize_input_for_direct_write(
                    CLAUDE_MODEL_UNAVAILABLE_RECOVERY_INPUT,
                );
                let sent_direct = self
                    .try_direct_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        &direct_input,
                    )
                    .await;
                if !sent_direct {
                    tracing::warn!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        "Direct PTY model-unavailable recovery failed (chunk mode); falling back to message bus"
                    );
                    self.publish_terminal_input_with_active_session(
                        &response_terminal_id,
                        &response_session_id,
                        CLAUDE_MODEL_UNAVAILABLE_RECOVERY_INPUT,
                        Some(decision),
                    )
                    .await;
                }
                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Model-unavailable recovery injected (chunk); sending immediate Enter to submit"
                );
                self.publish_terminal_input_with_active_session(
                    &response_terminal_id,
                    &response_session_id,
                    "\n",
                    Some(PromptDecision::auto_enter()),
                )
                .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level Claude model-unavailable fallback injection"
            );
        }

        // Chunk-level fallback for bypass-permissions toggle prompt.
        // Some terminal frames are emitted as dense ANSI chunks without stable
        // line boundaries, so line-based detection can miss this interaction.
        if state.auto_confirm
            && !state.should_debounce()
            && is_bypass_permissions_prompt(&normalized_output)
            && is_bypass_permissions_enter_confirm_context(&normalized_output)
        {
            let decision = PromptDecision::auto_enter();
            let detected_prompt =
                DetectedPrompt::new(PromptKind::EnterConfirm, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected bypass permissions prompt (chunk); sending direct auto-enter fallback"
                );

                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "\n",
                        Some(decision),
                    )
                    .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level bypass-permissions fallback injection"
            );
        }

        // Chunk-level fallback: decline Notepad prompts to avoid blocking
        // headless workflow execution in Windows environments.
        if state.auto_confirm && !state.should_debounce() && is_notepad_prompt(&normalized_output) {
            let decision = PromptDecision::llm_yes_no(
                false,
                "Auto-decline Notepad prompt to keep workflow non-blocking".to_string(),
            );
            let detected_prompt =
                DetectedPrompt::new(PromptKind::YesNo, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected Notepad prompt (chunk); sending 'n'+Enter fallback"
                );

                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "n\n",
                        Some(decision),
                    )
                    .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level Notepad fallback injection"
            );
        }

        // Chunk-level fallback: Codex can ask whether it should continue after
        // seeing "unexpected changes I didn't make". Auto-answer this to keep
        // orchestrated workflows non-blocking.
        if !state.should_debounce() && has_unexpected_changes_followup_context {
            let decision = PromptDecision::LLMDecision {
                response: UNEXPECTED_CHANGES_CONTINUE_RESPONSE.to_string(),
                reasoning:
                    "Auto-continue when Codex asks for confirmation about unexpected workspace changes"
                        .to_string(),
                target_index: None,
            };
            let detected_prompt =
                DetectedPrompt::new(PromptKind::Input, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_unexpected_changes_context();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected unexpected-changes follow-up prompt (chunk); sending auto-continue response"
                );

                drop(terminals);
                let direct_input =
                    Self::normalize_input_for_direct_write(UNEXPECTED_CHANGES_CONTINUE_RESPONSE);
                let sent_direct = self
                    .try_direct_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        &direct_input,
                    )
                    .await;
                if !sent_direct {
                    tracing::warn!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        "Direct PTY unexpected-changes auto-continue failed; falling back to message bus"
                    );
                    self.publish_terminal_input_with_active_session(
                        &response_terminal_id,
                        &response_session_id,
                        UNEXPECTED_CHANGES_CONTINUE_RESPONSE,
                        Some(decision),
                    )
                    .await;
                }
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level unexpected-changes fallback injection"
            );
        }

        // Chunk-level fallback: some agents finish their checks and then ask for
        // "what next?" instead of creating the required handoff commit.
        // Auto-instruct them to execute the completion contract immediately.
        if has_handoff_stall_context {
            let handoff_continue_response = Self::build_handoff_stall_continue_response(
                &state.workflow_id,
                &state.task_id,
                &state.terminal_id,
            );
            let decision = PromptDecision::LLMDecision {
                response: handoff_continue_response.clone(),
                reasoning:
                    "Auto-continue terminal when it waits for next instruction after reporting clean workspace"
                        .to_string(),
                target_index: None,
            };
            let detected_prompt =
                DetectedPrompt::new(PromptKind::Input, normalized_output.clone(), 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_handoff_stall_context();
                // Always follow handoff reminder injection with one submit Enter.
                // Some Claude TUI frames keep injected text in composer without
                // committing unless Enter is sent explicitly.
                state.clear_pending_handoff_submit();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected handoff-stall prompt (chunk); sending completion-contract reminder"
                );

                drop(terminals);
                let direct_input =
                    Self::normalize_input_for_direct_write(&handoff_continue_response);
                let sent_direct = self
                    .try_direct_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        &direct_input,
                    )
                    .await;
                if !sent_direct {
                    tracing::warn!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        "Direct PTY handoff-stall reminder failed (chunk mode); falling back to message bus"
                    );
                    self.publish_terminal_input_with_active_session(
                        &response_terminal_id,
                        &response_session_id,
                        &handoff_continue_response,
                        Some(decision),
                    )
                    .await;
                }
                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Handoff reminder injected (chunk); sending immediate Enter to submit"
                );
                self.publish_terminal_input_with_active_session(
                    &response_terminal_id,
                    &response_session_id,
                    "\n",
                    Some(PromptDecision::auto_enter()),
                )
                .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level handoff-stall fallback injection"
            );
        }

        // Chunk-level fallback: Codex confirmation prompt can appear inside
        // heavily ANSI-rendered TUI frames without clean newline boundaries.
        // Detect directly from normalized chunk and auto-reply with "y".
        if state.auto_confirm
            && !state.should_debounce()
            && is_codex_apply_patch_confirmation(&normalized_output)
        {
            let decision = PromptDecision::llm_yes_no(
                true,
                "Auto-approve Codex apply_patch confirmation".to_string(),
            );
            let detected_prompt = DetectedPrompt::new(PromptKind::YesNo, normalized_output, 0.95);

            if state.state_machine.should_process(&detected_prompt) {
                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected Codex apply_patch confirmation; sending direct auto-yes fallback"
                );

                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "y",
                        Some(decision),
                    )
                    .await;
                return;
            }
            tracing::debug!(
                terminal_id = %state.terminal_id,
                "Skipping duplicate chunk-level Codex confirmation fallback injection"
            );
        }

        // Process each line
        for line in output.lines() {
            let normalized_line = normalize_text_for_detection(line);
            let normalized_line_lower = normalized_line.to_ascii_lowercase();
            let has_any_claude_bypass_line_marker =
                has_any_claude_bypass_marker(&normalized_line_lower);
            state.observe_claude_bypass_context(&normalized_line_lower);
            state.observe_unexpected_changes_context(&normalized_line);
            state.observe_handoff_stall_context(&normalized_line);

            // Claude custom API key prompt fallback:
            // force "Yes" selection via ArrowUp + Enter.
            // This avoids defaulting to "No (recommended)" which can block model access.
            if state.auto_confirm
                && !state.should_debounce()
                && is_claude_custom_api_key_prompt(&normalized_line)
            {
                let decision = PromptDecision::LLMDecision {
                    response: "\u{1b}[A\n".to_string(),
                    reasoning: "Auto-select 'Yes' for custom API key prompt via ArrowUp + Enter"
                        .to_string(),
                    target_index: Some(0),
                };
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::ArrowSelect, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate line-level custom API key fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected custom API key selection prompt; sending ArrowUp + Enter to choose Yes"
                );

                // Publish input (drop lock first to avoid deadlock)
                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "\u{1b}[A\n",
                        Some(decision),
                    )
                    .await;
                return;
            }

            // Line-level fallback for model-unavailable prompts.
            if state.auto_confirm
                && !state.should_debounce()
                && is_claude_model_unavailable_prompt(&normalized_line)
            {
                let decision = PromptDecision::LLMDecision {
                    response: CLAUDE_MODEL_UNAVAILABLE_CONTINUE_RESPONSE.to_string(),
                    reasoning:
                        "Auto-recover Claude terminal when configured model is unavailable on current endpoint"
                            .to_string(),
                    target_index: None,
                };
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::Input, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate line-level Claude model-unavailable fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected Claude model-unavailable prompt (line); sending /model recovery command"
                );

                drop(terminals);
                let direct_input = Self::normalize_input_for_direct_write(
                    CLAUDE_MODEL_UNAVAILABLE_RECOVERY_INPUT,
                );
                let sent_direct = self
                    .try_direct_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        &direct_input,
                    )
                    .await;
                if !sent_direct {
                    tracing::warn!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        "Direct PTY model-unavailable recovery failed (line mode); falling back to message bus"
                    );
                    self.publish_terminal_input_with_active_session(
                        &response_terminal_id,
                        &response_session_id,
                        CLAUDE_MODEL_UNAVAILABLE_RECOVERY_INPUT,
                        Some(decision),
                    )
                    .await;
                }
                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Model-unavailable recovery injected (line); sending immediate Enter to submit"
                );
                self.publish_terminal_input_with_active_session(
                    &response_terminal_id,
                    &response_session_id,
                    "\n",
                    Some(PromptDecision::auto_enter()),
                )
                .await;
                return;
            }

            if state.auto_confirm
                && !state.should_debounce()
                && has_any_claude_bypass_line_marker
                && state.should_retry_claude_bypass_accept()
            {
                let (response, action, reasoning) = claude_bypass_accept_response(&normalized_line);
                let decision = PromptDecision::LLMDecision {
                    response: response.to_string(),
                    reasoning: format!("{reasoning} (retry once because prompt is still visible)"),
                    target_index: Some(1),
                };
                state.last_detection = Some(Instant::now());
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_claude_bypass_context();
                state.mark_claude_bypass_retry_sent();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    action = %action,
                    "Claude bypass prompt still visible after auto-accept; retrying once (line)"
                );

                drop(terminals);
                self.send_claude_bypass_accept_with_fallback(
                    &response_terminal_id,
                    &response_session_id,
                    response,
                    decision,
                    "line",
                    true,
                )
                .await;
                return;
            }

            // Line-level fallback for Claude bypass-permissions acceptance prompt.
            // This handles frames where each render only emits a single line.
            let has_claude_bypass_line_context = is_claude_bypass_accept_prompt(&normalized_line)
                || (state.has_recent_claude_bypass_accept_context()
                    && is_claude_bypass_accept_context_line(&normalized_line_lower));
            if state.auto_confirm && !state.should_debounce() && has_claude_bypass_line_context {
                let (response, action, reasoning) = claude_bypass_accept_response(&normalized_line);
                let decision = PromptDecision::LLMDecision {
                    response: response.to_string(),
                    reasoning: reasoning.to_string(),
                    target_index: Some(1),
                };
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::ArrowSelect, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate line-level Claude bypass accept fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_claude_bypass_context();
                state.mark_claude_bypass_accept_sent();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    action = %action,
                    "Detected Claude bypass permissions prompt (line); sending auto-accept sequence"
                );

                drop(terminals);
                self.send_claude_bypass_accept_with_fallback(
                    &response_terminal_id,
                    &response_session_id,
                    response,
                    decision,
                    "line",
                    false,
                )
                .await;
                return;
            }

            // Fallback for bypass-permissions prompts only when explicit confirmation context appears.
            if state.auto_confirm
                && !state.should_debounce()
                && is_bypass_permissions_prompt(&normalized_line)
                && is_bypass_permissions_enter_confirm_context(&normalized_line)
            {
                let decision = PromptDecision::auto_enter();
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::EnterConfirm, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate bypass-permissions fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected bypass permissions prompt; sending direct auto-enter fallback"
                );

                // Publish input (drop lock first to avoid deadlock)
                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "\n",
                        Some(decision),
                    )
                    .await;
                return;
            }

            if !state.should_debounce()
                && state.should_force_handoff_submit()
                && is_bypass_permissions_prompt(&normalized_line)
            {
                let decision = PromptDecision::auto_enter();

                state.last_detection = Some(Instant::now());
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_pending_handoff_submit();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected bypass status-line after handoff reminder; sending extra Enter to submit queued input"
                );

                drop(terminals);
                self.publish_terminal_input_with_active_session(
                    &response_terminal_id,
                    &response_session_id,
                    "\n",
                    Some(decision),
                )
                .await;
                return;
            }

            // Notepad fallback in line-by-line mode.
            if state.auto_confirm && !state.should_debounce() && is_notepad_prompt(&normalized_line)
            {
                let decision = PromptDecision::llm_yes_no(
                    false,
                    "Auto-decline Notepad prompt to keep workflow non-blocking".to_string(),
                );
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::YesNo, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate line-level Notepad fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected Notepad prompt; sending 'n'+Enter fallback"
                );

                // Publish input (drop lock first to avoid deadlock)
                drop(terminals);
                self.message_bus
                    .publish_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        "n\n",
                        Some(decision),
                    )
                    .await;
                return;
            }

            // Line-level fallback for Codex unexpected-changes follow-up prompt.
            let has_unexpected_changes_line_context =
                is_unexpected_changes_followup_prompt(&normalized_line)
                    || state.has_recent_unexpected_changes_context();
            if !state.should_debounce() && has_unexpected_changes_line_context {
                let decision = PromptDecision::LLMDecision {
                    response: UNEXPECTED_CHANGES_CONTINUE_RESPONSE.to_string(),
                    reasoning:
                        "Auto-continue when Codex asks for confirmation about unexpected workspace changes"
                            .to_string(),
                    target_index: None,
                };
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::Input, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate line-level unexpected-changes fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_unexpected_changes_context();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected unexpected-changes follow-up prompt; sending auto-continue response"
                );

                drop(terminals);
                let direct_input =
                    Self::normalize_input_for_direct_write(UNEXPECTED_CHANGES_CONTINUE_RESPONSE);
                let sent_direct = self
                    .try_direct_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        &direct_input,
                    )
                    .await;
                if !sent_direct {
                    tracing::warn!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        "Direct PTY unexpected-changes auto-continue failed (line mode); falling back to message bus"
                    );
                    self.publish_terminal_input_with_active_session(
                        &response_terminal_id,
                        &response_session_id,
                        UNEXPECTED_CHANGES_CONTINUE_RESPONSE,
                        Some(decision),
                    )
                    .await;
                }
                return;
            }

            // Line-level fallback for clean-workspace "what next?" stalls.
            let has_handoff_stall_line_context = is_handoff_stall_prompt(&normalized_line)
                || state.has_recent_handoff_stall_context();
            if has_handoff_stall_line_context {
                let handoff_continue_response = Self::build_handoff_stall_continue_response(
                    &state.workflow_id,
                    &state.task_id,
                    &state.terminal_id,
                );
                let decision = PromptDecision::LLMDecision {
                    response: handoff_continue_response.clone(),
                    reasoning:
                        "Auto-continue terminal when it waits for next instruction after reporting clean workspace"
                            .to_string(),
                    target_index: None,
                };
                let detected_prompt =
                    DetectedPrompt::new(PromptKind::Input, normalized_line.clone(), 0.95);

                if !state.state_machine.should_process(&detected_prompt) {
                    tracing::debug!(
                        terminal_id = %state.terminal_id,
                        "Skipping duplicate line-level handoff-stall fallback injection"
                    );
                    continue;
                }

                state.last_detection = Some(Instant::now());
                state.state_machine.on_prompt_detected(detected_prompt);
                state.state_machine.on_response_sent(decision.clone());
                state.detector.clear_buffer();
                state.clear_handoff_stall_context();
                // Always submit after injecting handoff reminder to avoid
                // composer-only text that never executes.
                state.clear_pending_handoff_submit();

                let response_terminal_id = state.terminal_id.clone();
                let response_session_id = state.session_id.clone();

                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Detected handoff-stall prompt (line); sending completion-contract reminder"
                );

                drop(terminals);
                let direct_input =
                    Self::normalize_input_for_direct_write(&handoff_continue_response);
                let sent_direct = self
                    .try_direct_terminal_input(
                        &response_terminal_id,
                        &response_session_id,
                        &direct_input,
                    )
                    .await;
                if !sent_direct {
                    tracing::warn!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        "Direct PTY handoff-stall reminder failed (line mode); falling back to message bus"
                    );
                    self.publish_terminal_input_with_active_session(
                        &response_terminal_id,
                        &response_session_id,
                        &handoff_continue_response,
                        Some(decision),
                    )
                    .await;
                }
                tracing::info!(
                    terminal_id = %response_terminal_id,
                    session_id = %response_session_id,
                    "Handoff reminder injected (line); sending immediate Enter to submit"
                );
                self.publish_terminal_input_with_active_session(
                    &response_terminal_id,
                    &response_session_id,
                    "\n",
                    Some(PromptDecision::auto_enter()),
                )
                .await;
                return;
            }

            if let Some(prompt) = state.process_line(line) {
                // Direct fallback for EnterConfirm prompts when auto_confirm is enabled.
                // This keeps terminals responsive even when Orchestrator is not running
                // (e.g. workflow in ready/prepare stage).
                let skip_enter_confirm_fallback = has_claude_bypass_accept_context
                    || bypass_needs_enter_context
                    || normalized_output_lower.contains("interrupted")
                    || normalized_output_lower.contains("custom api key");

                if state.auto_confirm
                    && prompt.kind == PromptKind::EnterConfirm
                    && !prompt.has_dangerous_keywords
                    && !skip_enter_confirm_fallback
                {
                    if has_claude_custom_api_key_context {
                        let decision = PromptDecision::LLMDecision {
                            response: "\u{1b}[A\n".to_string(),
                            reasoning:
                                "Auto-select 'Yes' for custom API key prompt via ArrowUp + Enter"
                                    .to_string(),
                            target_index: Some(0),
                        };

                        state.state_machine.on_response_sent(decision.clone());
                        state.detector.clear_buffer();

                        let response_terminal_id = state.terminal_id.clone();
                        let response_session_id = state.session_id.clone();

                        tracing::info!(
                            terminal_id = %response_terminal_id,
                            session_id = %response_session_id,
                            confidence = prompt.confidence,
                            "Detected EnterConfirm under custom API key context; sending ArrowUp + Enter fallback"
                        );

                        drop(terminals);
                        self.message_bus
                            .publish_terminal_input(
                                &response_terminal_id,
                                &response_session_id,
                                "\u{1b}[A\n",
                                Some(decision),
                            )
                            .await;
                        return;
                    }

                    let decision = PromptDecision::auto_enter();

                    state.state_machine.on_response_sent(decision.clone());
                    state.detector.clear_buffer();

                    let response_terminal_id = state.terminal_id.clone();
                    let response_session_id = state.session_id.clone();

                    tracing::info!(
                        terminal_id = %response_terminal_id,
                        session_id = %response_session_id,
                        confidence = prompt.confidence,
                        "Detected EnterConfirm prompt; sending direct auto-enter fallback"
                    );

                    drop(terminals);
                    self.message_bus
                        .publish_terminal_input(
                            &response_terminal_id,
                            &response_session_id,
                            "\n",
                            Some(decision),
                        )
                        .await;
                    return;
                }

                let event = TerminalPromptEvent {
                    terminal_id: state.terminal_id.clone(),
                    workflow_id: state.workflow_id.clone(),
                    task_id: state.task_id.clone(),
                    session_id: state.session_id.clone(),
                    auto_confirm: state.auto_confirm,
                    prompt: prompt.clone(),
                    detected_at: chrono::Utc::now(),
                };

                tracing::info!(
                    terminal_id = %state.terminal_id,
                    prompt_kind = ?prompt.kind,
                    confidence = prompt.confidence,
                    has_dangerous_keywords = prompt.has_dangerous_keywords,
                    "Detected interactive prompt"
                );

                // Publish event (drop lock first to avoid deadlock)
                drop(terminals);
                self.message_bus
                    .publish_terminal_prompt_detected(event)
                    .await;
                return;
            }
        }
    }

    /// Update terminal state after response is sent
    pub async fn on_response_sent(
        &self,
        terminal_id: &str,
        decision: crate::services::orchestrator::types::PromptDecision,
    ) {
        let mut terminals = self.terminals.write().await;
        if let Some(state) = terminals.get_mut(terminal_id) {
            state.state_machine.on_response_sent(decision);
            state.detector.clear_buffer();
            state.clear_claude_bypass_context();
            state.clear_handoff_stall_context();
            state.clear_pending_handoff_submit();
        }
    }

    /// Update terminal state when waiting for user approval
    pub async fn on_waiting_for_approval(
        &self,
        terminal_id: &str,
        decision: crate::services::orchestrator::types::PromptDecision,
    ) {
        let mut terminals = self.terminals.write().await;
        if let Some(state) = terminals.get_mut(terminal_id) {
            state.state_machine.on_waiting_for_approval(decision);
        }
    }

    /// Reset terminal prompt state
    pub async fn reset_state(&self, terminal_id: &str) {
        let mut terminals = self.terminals.write().await;
        if let Some(state) = terminals.get_mut(terminal_id) {
            state.state_machine.reset();
            state.detector.clear_buffer();
            state.clear_claude_bypass_context();
            state.clear_handoff_stall_context();
            state.clear_pending_handoff_submit();
        }
    }

    /// Get current prompt state for a terminal
    pub async fn get_state(&self, terminal_id: &str) -> Option<PromptState> {
        let terminals = self.terminals.read().await;
        terminals.get(terminal_id).map(|s| s.state_machine.state)
    }

    /// Check if a terminal is registered
    pub async fn is_registered(&self, terminal_id: &str) -> bool {
        let terminals = self.terminals.read().await;
        let subscriptions = self.active_subscriptions.read().await;
        // Terminal is truly registered only if both state and active subscription exist
        terminals.contains_key(terminal_id) && subscriptions.contains_key(terminal_id)
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use crate::services::orchestrator::message_bus::{BusMessage, MessageBus};

    fn create_test_watcher() -> PromptWatcher {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        PromptWatcher::new(message_bus, process_manager)
    }

    #[tokio::test]
    async fn test_register_unregister() {
        let watcher = create_test_watcher();

        // Registration will fail because no terminal exists in ProcessManager
        // This is expected in unit tests - we're testing state management, not integration
        let result = watcher
            .register("term-1", "workflow-1", "task-1", "session-1", true)
            .await;

        // Registration should fail with terminal not found
        assert!(result.is_err());
        assert!(
            result
                .unwrap_err()
                .to_string()
                .contains("Terminal not found")
        );

        // Terminal should not be registered since subscription failed
        assert!(!watcher.is_registered("term-1").await);
    }

    #[tokio::test]
    async fn test_register_attempts_subscription_when_auto_confirm_disabled() {
        let watcher = create_test_watcher();

        let result = watcher
            .register("term-1", "workflow-1", "task-1", "session-1", false)
            .await;

        assert!(result.is_err());
        assert!(
            result
                .unwrap_err()
                .to_string()
                .contains("Terminal not found")
        );
        assert!(!watcher.is_registered("term-1").await);
    }

    #[tokio::test]
    async fn test_process_output_publishes_prompt_when_auto_confirm_disabled() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    false,
                ),
            );
        }

        watcher
            .process_output("term-1", "Press Enter to continue")
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected prompt event broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalPromptDetected(prompt_event) => {
                assert_eq!(prompt_event.terminal_id, "term-1");
                assert_eq!(prompt_event.workflow_id, "workflow-1");
                assert_eq!(prompt_event.task_id, "task-1");
                assert_eq!(prompt_event.session_id, "session-1");
                assert!(!prompt_event.auto_confirm);
            }
            other => panic!("expected TerminalPromptDetected event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_unregistered() {
        let watcher = create_test_watcher();

        // Should not panic for unregistered terminal
        watcher
            .process_output("unknown-term", "Press Enter to continue")
            .await;
    }

    #[tokio::test]
    async fn test_process_output_bypass_prompt_auto_confirms_with_terminal_input() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let bypass_line = "\u{1b}[2mInterrupted\u{1b}[0m bypass permissions on (shift+tab to cycle) Press Ctrl-C again to exit";

        watcher.process_output("term-1", bypass_line).await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::AutoConfirm { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_bypass_prompt_chunk_level_auto_confirms_with_terminal_input() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        // Simulate an ANSI-heavy chunk where prompt text may not be split cleanly by lines.
        let bypass_chunk = "\u{1b}[2mInterrupted: bypass permissions on (shift+tab to cycle)\u{1b}[0m press ctrl-c again to exit";

        watcher.process_output("term-1", bypass_chunk).await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::AutoConfirm { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_bypass_permissions_prompt_auto_selects_yes_accept() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let prompt = r"
WARNING: Claude Code running in Bypass Permissions mode
1. No, exit
2. Yes, I accept
Enter to confirm 路 Esc to cancel
";

        watcher.process_output("term-1", prompt).await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "2\r");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_bypass_permissions_prompt_line_by_line_auto_selects_yes_accept()
     {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        // Simulate line-by-line rendering where each frame only contains one line.
        let lines = [
            "\u{1b}[33mWARNING: Claude Code running in Bypass Permissions mode\u{1b}[0m",
            "1. No, exit",
            "2. Yes, I accept",
            "Enter to confirm 路 Esc to cancel",
        ];

        for line in lines {
            watcher.process_output("term-1", line).await;
        }

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "2\r");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_bypass_permissions_prompt_without_confirm_hint_auto_selects_yes_accept()
     {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        // Newer Claude prompt variants may omit the explicit
        // "Enter to confirm" footer.
        let prompt = r"
WARNING: Claude Code running in Bypass Permissions mode
1. No, exit
2. Yes, I accept
";

        watcher.process_output("term-1", prompt).await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "2\r");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_bypass_prompt_accepts_compact_no_space_variants() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let lines = [
            "WARNING: Claude Code running in Bypass Permissions mode",
            "1.No,exit",
            "2.Yes,I accept",
            "Enter to confirm · Esc to cancel",
        ];

        for line in lines {
            watcher.process_output("term-1", line).await;
        }

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "2\r");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_bypass_prompt_retries_once_when_still_visible() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let prompt_single_line =
            "WARNING: Claude Code running in Bypass Permissions mode 1. No, exit 2. Yes, I accept";

        watcher.process_output("term-1", prompt_single_line).await;

        let first_event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected first terminal input broadcast")
            .expect("broadcast channel should be open");
        match first_event {
            BusMessage::TerminalInput { input, .. } => assert_eq!(input, "2\r"),
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }

        {
            let mut terminals = watcher.terminals.write().await;
            let state = terminals
                .get_mut("term-1")
                .expect("terminal state should exist");
            state.last_detection =
                Some(Instant::now() - Duration::from_millis(PROMPT_DEBOUNCE_MS + 1));
            state.pending_claude_bypass_retry_since = Some(
                Instant::now() - Duration::from_millis(CLAUDE_BYPASS_ACCEPT_RETRY_DELAY_MS + 1),
            );
        }

        watcher.process_output("term-1", prompt_single_line).await;

        let second_event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected retry terminal input broadcast")
            .expect("broadcast channel should be open");
        match second_event {
            BusMessage::TerminalInput { input, .. } => assert_eq!(input, "2\r"),
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }

        {
            let mut terminals = watcher.terminals.write().await;
            let state = terminals
                .get_mut("term-1")
                .expect("terminal state should exist");
            state.last_detection =
                Some(Instant::now() - Duration::from_millis(PROMPT_DEBOUNCE_MS + 1));
            state.pending_claude_bypass_retry_since = Some(
                Instant::now() - Duration::from_millis(CLAUDE_BYPASS_ACCEPT_RETRY_DELAY_MS + 1),
            );
        }

        watcher.process_output("term-1", prompt_single_line).await;

        let third_event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv()).await;
        assert!(
            third_event.is_err(),
            "Claude bypass retry should only fire once while prompt remains visible"
        );
    }

    #[tokio::test]
    async fn test_process_output_notepad_prompt_auto_declines_with_terminal_input() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output("term-1", "Open in Notepad? (y/N)")
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "n\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_notepad_prompt_chunk_level_auto_declines_with_terminal_input() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let notepad_chunk =
            "\u{1b}[2mOpen in Notepad?\u{1b}[0m \u{1b}[38;5;6m[y/N]\u{1b}[0m\u{1b}[?2026l";

        watcher.process_output("term-1", notepad_chunk).await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "n\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_unexpected_changes_followup_auto_continues() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "I detected changes I didn't make. Should I continue implementing and commit, or wait for you to handle those changes first?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, UNEXPECTED_CHANGES_CONTINUE_RESPONSE);
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_unexpected_changes_followup_auto_continues_when_auto_confirm_disabled()
     {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    false,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "I detected changes I didn't make. Should I continue implementing and commit, or wait for you to handle those changes first?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, UNEXPECTED_CHANGES_CONTINUE_RESPONSE);
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_unexpected_changes_followup_cn_split_lines_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    false,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "检测到工作区出现了我未发起的变更：backend.py 已变为已修改状态，并新增了 __pycache__/。",
            )
            .await;

        let first_poll = tokio::time::timeout(Duration::from_millis(80), broadcast_rx.recv()).await;
        assert!(
            first_poll.is_err(),
            "first fragment should not auto-send yet"
        );

        watcher
            .process_output(
                "term-1",
                "按照协作约束我需要先暂停。请确认我是否可以基于当前最新文件继续实现任务 B（我会只做增量修改并避免回退他人改动）。",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, UNEXPECTED_CHANGES_CONTINUE_RESPONSE);
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_auto_continues() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "git status clean; recent commits shown. No diff to review right now. What would you like me to work on next?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_let_me_know_do_next_variant_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "On branch main; nothing to commit, working tree clean. All clean\u{2014}no changes to commit and no further instructions were provided. Let me know what you\u{2019}d like me to do next.",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_share_implement_change_variant_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "On branch main; nothing to commit, working tree clean. I\u{2019}m ready to start. Could you share what you\u{2019}d like me to implement or change?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_proceed_next_variant_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "On branch main; nothing to commit, working tree clean. How would you like to proceed next?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_missing_task_variant_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "I'm ready to jump in, but I don't have a specific task yet. Could you clarify what you'd like me to work on?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_describe_specific_change_feature_variant_auto_continue()
     {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "Ive pulled the latest repo state (status clean, no diff, recent commits listed) and Im ready to start implementing. Could you describe the specific change or feature youd like me to work on?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_with_bypass_status_line_sends_immediate_enter() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "On branch main; nothing to commit, working tree clean. What would you like me to work on next?\n⏵⏵ bypass permissions on (shift+tab to cycle)",
            )
            .await;

        let first = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected first terminal input broadcast")
            .expect("broadcast channel should be open");
        let second = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected second terminal input broadcast")
            .expect("broadcast channel should be open");

        match first {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected first TerminalInput event, got: {other:?}"),
        }

        match second {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::AutoConfirm { .. })
                ));
            }
            other => panic!("expected second TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_cn_split_lines_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    false,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "On branch main; nothing to commit, working tree clean",
            )
            .await;

        let first_poll = tokio::time::timeout(Duration::from_millis(80), broadcast_rx.recv()).await;
        assert!(
            first_poll.is_err(),
            "first fragment should not auto-send yet"
        );

        watcher
            .process_output(
                "term-1",
                "Repository is clean. Please let me know what you'd like me to work on next.",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_clarify_variant_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "On branch main; nothing to commit, working tree clean. I'm ready to start. Could you clarify what you'd like me to work on next?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_handoff_stall_missing_scope_variant_auto_continue() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "I see the instruction to start implementing, but I don't have any specific requirements or files to modify yet. Could you clarify what changes or task you'd like me to work on first?",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(input.contains(HANDOFF_STALL_CONTINUE_RESPONSE));
                assert!(input.contains("workflow_id: workflow-1"));
                assert!(input.contains("task_id: task-1"));
                assert!(input.contains("terminal_id: term-1"));
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_codex_apply_patch_confirmation_auto_yes() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "Confirming apply_patch approach (1m 32s 鈥?esc to interrupt)",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "y");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_codex_apply_patch_confirmation_dedupes_consecutive_fallback() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let prompt_text = "Confirming apply_patch approach (esc to interrupt)";

        watcher.process_output("term-1", prompt_text).await;

        let first_event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected first terminal input broadcast")
            .expect("broadcast channel should be open");

        match first_event {
            BusMessage::TerminalInput { input, .. } => assert_eq!(input, "y"),
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }

        {
            let mut terminals = watcher.terminals.write().await;
            let state = terminals
                .get_mut("term-1")
                .expect("terminal state should exist");
            state.last_detection =
                Some(Instant::now() - Duration::from_millis(PROMPT_DEBOUNCE_MS + 1));
        }

        watcher.process_output("term-1", prompt_text).await;

        let second_event =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv()).await;
        assert!(
            second_event.is_err(),
            "duplicate codex fallback injection should be skipped"
        );
    }

    #[tokio::test]
    async fn test_process_output_bypass_fallback_dedupes_consecutive_injection() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        let bypass_prompt =
            "Interrupted: bypass permissions on (shift+tab to cycle), press ctrl-c again to exit";

        watcher.process_output("term-1", bypass_prompt).await;

        let first_event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected first terminal input broadcast")
            .expect("broadcast channel should be open");

        match first_event {
            BusMessage::TerminalInput { input, .. } => assert_eq!(input, "\n"),
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }

        {
            let mut terminals = watcher.terminals.write().await;
            let state = terminals
                .get_mut("term-1")
                .expect("terminal state should exist");
            state.last_detection =
                Some(Instant::now() - Duration::from_millis(PROMPT_DEBOUNCE_MS + 1));
        }

        watcher.process_output("term-1", bypass_prompt).await;

        let second_event =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv()).await;
        assert!(
            second_event.is_err(),
            "duplicate bypass fallback injection should be skipped"
        );
    }

    #[tokio::test]
    async fn test_process_output_bypass_status_line_does_not_auto_enter() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "bypass permissions on (shift+tab to cycle) ctrl+g to edit in Notepad",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv()).await;
        assert!(
            event.is_err(),
            "status-line bypass indicator should not inject Enter without confirmation context"
        );
    }

    #[tokio::test]
    async fn test_process_output_bypass_status_line_auto_enters_when_handoff_submit_pending() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            let mut state = TerminalWatchState::new(
                "term-1".to_string(),
                "workflow-1".to_string(),
                "task-1".to_string(),
                "session-1".to_string(),
                true,
            );
            state.mark_pending_handoff_submit();
            terminals.insert("term-1".to_string(), state);
        }

        watcher
            .process_output(
                "term-1",
                "bypass permissions on (shift+tab to cycle) ctrl+g to edit in Notepad",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::AutoConfirm { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_custom_api_key_prompt_auto_select_yes() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output("term-1", "Do you want to use this API key?")
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\u{1b}[A\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_custom_api_key_environment_banner_auto_select_yes() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "Detected a custom API key in your environment\nPress Enter to continue",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\u{1b}[A\n");
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_model_unavailable_prompt_sends_recovery_instruction() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "There's an issue with the selected model (glm-5). It may not exist or you may not have access to it. Run /model to pick a different model.",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(
                    input.contains("/model"),
                    "recovery input should execute /model command"
                );
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_model_unavailable_prompt_line_by_line_sends_recovery_instruction()
     {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "There's an issue with the selected model (glm-5). You may not have access. Run /model to pick a different model.",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(
                    input.contains("/model"),
                    "line-level recovery should execute /model command"
                );
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_model_not_found_prompt_sends_recovery_instruction() {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "API Error: 503 {\"error\":{\"code\":\"model_not_found\",\"message\":\"No available channel for model claude-haiku-4-5 under group default (request id: abc)\"}}",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(
                    input.contains("/model"),
                    "gateway model_not_found recovery should execute /model command"
                );
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_process_output_claude_model_not_found_prompt_line_by_line_sends_recovery_instruction(
    ) {
        let message_bus = Arc::new(MessageBus::new(100));
        let process_manager = Arc::new(ProcessManager::new());
        let watcher = PromptWatcher::new(message_bus.clone(), process_manager);
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        {
            let mut terminals = watcher.terminals.write().await;
            terminals.insert(
                "term-1".to_string(),
                TerminalWatchState::new(
                    "term-1".to_string(),
                    "workflow-1".to_string(),
                    "task-1".to_string(),
                    "session-1".to_string(),
                    true,
                ),
            );
        }

        watcher
            .process_output(
                "term-1",
                "API Error: 503\n{\"error\":{\"code\":\"model_not_found\"}}\nNo available channel for model claude-haiku-4-5 under group default",
            )
            .await;

        let event = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected terminal input broadcast")
            .expect("broadcast channel should be open");

        match event {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert!(
                    input.contains("/model"),
                    "line-level gateway model_not_found recovery should execute /model command"
                );
                assert!(matches!(
                    decision,
                    Some(crate::services::orchestrator::types::PromptDecision::LLMDecision { .. })
                ));
            }
            other => panic!("expected TerminalInput event, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_get_state() {
        let watcher = create_test_watcher();

        // Unregistered terminal returns None
        assert!(watcher.get_state("term-1").await.is_none());
    }

    #[tokio::test]
    async fn test_reset_state() {
        let watcher = create_test_watcher();

        // Reset on unregistered terminal should not panic
        watcher.reset_state("term-1").await;
    }
}
