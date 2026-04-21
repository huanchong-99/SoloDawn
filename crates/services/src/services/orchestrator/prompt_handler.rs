//! Orchestrator Prompt Handler Module
//!
//! Processes terminal prompt events and makes intelligent decisions about how to respond.
//! Implements a rule-based strategy with LLM fallback for complex decisions.
//!
//! ## Decision Strategy
//!
//! 1. **auto_confirm=false**: Ask user for every prompt (never auto-respond)
//! 2. **EnterConfirm** (high confidence, no dangerous keywords): Auto-send `\n`
//! 3. **Password**: Always ask user (never auto-respond)
//! 4. **Dangerous keywords detected**: Escalate to LLM or ask user
//! 5. **YesNo/Choice/ArrowSelect/Input**: LLM decision

use std::{collections::HashMap, future::Future, pin::Pin, sync::Arc, time::Duration};

use tokio::sync::RwLock;

use super::{
    message_bus::SharedMessageBus,
    types::{
        DetectedPrompt, PromptDecision, PromptKind, PromptState, TerminalPromptEvent,
        TerminalPromptStateMachine,
    },
};

/// Callback type for LLM-powered prompt input generation.
pub type LLMPromptCallback = Arc<
    dyn Fn(String) -> Pin<Box<dyn Future<Output = Option<String>> + Send>> + Send + Sync,
>;

// ============================================================================
// Constants
// ============================================================================

/// Confidence threshold for auto-confirming EnterConfirm prompts
const AUTO_CONFIRM_CONFIDENCE_THRESHOLD: f32 = 0.85;

const SPINNER_NOISE_MARKERS: [&str; 2] = ["brewing", "loading"];
const ADVISORY_CHECKLIST_PREFIXES: [&str; 5] = ["[ ]", "[x]", "[X]", "- [ ]", "* [ ]"];
const ENTER_CONFIRM_MARKERS: [&str; 6] = [
    "press enter to continue",
    "press enter to confirm",
    "enter to continue",
    "enter to confirm",
    "hit enter to continue",
    "hit enter to confirm",
];
const DESTRUCTIVE_WARNING_MARKERS: [&str; 7] = [
    "permanently delete",
    "delete the remote branch",
    "delete remote branch",
    "permanent",
    "irreversible",
    "force push",
    "destroy",
];
const DANGEROUS_CONFIRMATION_OPTIONS: [&str; 9] = [
    "yes",
    "no",
    "cancel",
    "abort",
    "continue",
    "proceed",
    "confirm",
    "delete",
    "keep",
];
const DANGEROUS_CONFIRMATION_QUESTION_MARKERS: [&str; 14] = [
    "delete",
    "remove",
    "destroy",
    "wipe",
    "drop",
    "overwrite",
    "reset",
    "merge",
    "push",
    "deploy",
    "publish",
    "force",
    "permanent",
    "irreversible",
];

fn advisory_checklist_item_count(raw_text: &str) -> usize {
    raw_text
        .lines()
        .map(str::trim_start)
        .filter(|line| ADVISORY_CHECKLIST_PREFIXES.iter().any(|prefix| line.starts_with(prefix)))
        .count()
}

fn normalize_confirmation_option_line(line: &str) -> String {
    let trimmed = line.trim_start();
    let without_select_marker = trimmed
        .strip_prefix('>')
        .or_else(|| trimmed.strip_prefix('❯'))
        .or_else(|| trimmed.strip_prefix('▶'))
        .or_else(|| trimmed.strip_prefix('→'))
        .map(str::trim_start)
        .or_else(|| {
            trimmed
                .strip_prefix('*')
                .or_else(|| trimmed.strip_prefix('-'))
                .map(str::trim_start)
                .filter(|rest| {
                    !rest.starts_with("[ ]") && !rest.starts_with("[x]") && !rest.starts_with("[X]")
                })
        })
        .unwrap_or(trimmed);

    without_select_marker.to_ascii_lowercase()
}

fn has_destructive_confirmation_signal(raw_text: &str) -> bool {
    let normalized = raw_text.to_ascii_lowercase();
    if normalized.contains("[y/n]")
        || normalized.contains("(y/n)")
        || normalized.contains("yes/no")
        || normalized.contains("are you sure")
    {
        return true;
    }

    raw_text.lines().any(|line| {
        let normalized_line = normalize_confirmation_option_line(line);

        DANGEROUS_CONFIRMATION_OPTIONS
            .iter()
            .any(|option| normalized_line.starts_with(option))
            || (normalized_line.contains('?')
                && DANGEROUS_CONFIRMATION_QUESTION_MARKERS
                    .iter()
                    .any(|marker| normalized_line.contains(marker)))
    })
}

fn has_destructive_warning_text(raw_text: &str) -> bool {
    let normalized = raw_text.to_ascii_lowercase();
    DESTRUCTIVE_WARNING_MARKERS
        .iter()
        .any(|marker| normalized.contains(marker))
}

fn has_enter_confirm_text(raw_text: &str) -> bool {
    let normalized = raw_text.to_ascii_lowercase();
    ENTER_CONFIRM_MARKERS
        .iter()
        .any(|marker| normalized.contains(marker))
}

fn should_escalate_despite_safe_checklist(prompt: &DetectedPrompt) -> bool {
    if advisory_checklist_item_count(&prompt.raw_text) < 2 {
        return false;
    }

    let has_destructive_warning = has_destructive_warning_text(&prompt.raw_text);
    if !has_destructive_warning {
        return false;
    }

    has_destructive_confirmation_signal(&prompt.raw_text)
        || (prompt.kind == PromptKind::EnterConfirm && has_enter_confirm_text(&prompt.raw_text))
}

fn is_advisory_checklist_prompt(raw_text: &str) -> bool {
    advisory_checklist_item_count(raw_text) >= 2 && !has_destructive_confirmation_signal(raw_text)
}

fn should_require_user_confirmation(prompt: &DetectedPrompt) -> bool {
    (prompt.has_dangerous_keywords && !is_advisory_checklist_prompt(&prompt.raw_text))
        || should_escalate_despite_safe_checklist(prompt)
}

// ============================================================================
// LLM Prompt Decision Request/Response
// ============================================================================

/// Request for LLM to make a prompt decision
#[derive(Debug, Clone, serde::Serialize)]
pub struct LLMPromptDecisionRequest {
    /// The detected prompt
    pub prompt_kind: String,
    /// Raw prompt text
    pub prompt_text: String,
    /// Available options (for ArrowSelect/Choice)
    pub options: Option<Vec<String>>,
    /// Current selected index (for ArrowSelect)
    pub current_index: Option<usize>,
    /// Whether dangerous keywords were detected
    pub has_dangerous_keywords: bool,
    /// Task context (what the terminal is doing)
    pub task_context: Option<String>,
}

/// Response from LLM for prompt decision
#[derive(Debug, Clone, serde::Deserialize)]
pub struct LLMPromptDecisionResponse {
    /// The decision action
    pub action: String,
    /// Response to send (for auto/llm decisions)
    pub response: Option<String>,
    /// Target index (for ArrowSelect)
    pub target_index: Option<usize>,
    /// Reasoning for the decision
    pub reasoning: String,
    /// Whether to ask user instead
    pub ask_user: Option<bool>,
}

// ============================================================================
// Prompt Handler
// ============================================================================

/// Handles terminal prompt events and makes decisions
pub struct PromptHandler {
    /// Message bus for publishing responses
    message_bus: SharedMessageBus,
    /// Per-terminal state machines
    state_machines: Arc<RwLock<HashMap<String, TerminalPromptStateMachine>>>,
    /// Task context cache (terminal_id -> context)
    task_contexts: Arc<RwLock<HashMap<String, String>>>,
    /// Optional LLM callback for generating free-form input responses
    llm_callback: Option<LLMPromptCallback>,
}

impl PromptHandler {
    fn is_codex_composer_like_input(prompt_text: &str) -> bool {
        let normalized = prompt_text.to_ascii_lowercase();

        let has_codex_chrome = normalized.contains("for shortcuts")
            && (normalized.contains("context left")
                || normalized.contains("stopping due to unexpected changes"));

        has_codex_chrome
            || normalized.contains("stopping due to unexpected changes")
            || normalized.contains("use /skills to list available skills")
    }

    /// Create a new prompt handler
    pub fn new(message_bus: SharedMessageBus) -> Self {
        Self {
            message_bus,
            state_machines: Arc::new(RwLock::new(HashMap::new())),
            task_contexts: Arc::new(RwLock::new(HashMap::new())),
            llm_callback: None,
        }
    }

    /// Create a new prompt handler with LLM callback for free-form input generation
    pub fn new_with_llm(message_bus: SharedMessageBus, llm_callback: LLMPromptCallback) -> Self {
        Self {
            message_bus,
            state_machines: Arc::new(RwLock::new(HashMap::new())),
            task_contexts: Arc::new(RwLock::new(HashMap::new())),
            llm_callback: Some(llm_callback),
        }
    }

    /// Set task context for a terminal (used for LLM decisions)
    pub async fn set_task_context(&self, terminal_id: &str, context: &str) {
        let mut contexts = self.task_contexts.write().await;
        contexts.insert(terminal_id.to_string(), context.to_string());
    }

    /// Clear task context for a terminal
    pub async fn clear_task_context(&self, terminal_id: &str) {
        let mut contexts = self.task_contexts.write().await;
        contexts.remove(terminal_id);
    }

    /// Handle a terminal prompt event
    ///
    /// Returns the decision made, or None if the prompt should be skipped.
    pub async fn handle_prompt_event(&self, event: &TerminalPromptEvent) -> Option<PromptDecision> {
        // Get or create state machine for this terminal
        let mut state_machines = self.state_machines.write().await;
        let state_machine = state_machines
            .entry(event.terminal_id.clone())
            .or_insert_with(TerminalPromptStateMachine::new);

        // Check if we should process this prompt
        if !state_machine.should_process(&event.prompt) {
            tracing::debug!(
                terminal_id = %event.terminal_id,
                prompt_kind = ?event.prompt.kind,
                "Skipping duplicate/debounced prompt"
            );
            return Some(PromptDecision::skip("Duplicate or debounced prompt"));
        }

        // Update state machine
        state_machine.on_prompt_detected(event.prompt.clone());
        state_machine.on_deciding();

        // Make decision based on prompt type and context
        let decision = self.make_decision(&event.prompt, event.auto_confirm).await;

        // Update state machine based on decision
        match &decision {
            PromptDecision::AskUser { .. } => {
                state_machine.on_waiting_for_approval(decision.clone());
            }
            PromptDecision::Skip { .. } => {
                state_machine.reset();
            }
            _ => {
                state_machine.on_response_sent(decision.clone());
            }
        }

        // Drop lock before publishing
        drop(state_machines);

        let mut decision_to_publish = decision.clone();

        // If decision has a response, publish terminal input first.
        // Only publish non-skip decision when input has a delivery route.
        if let Some(response) = self.get_response_from_decision(&decision) {
            let delivered = self
                .message_bus
                .publish_terminal_input(
                    &event.terminal_id,
                    &event.session_id,
                    &response,
                    Some(decision.clone()),
                )
                .await;

            if !delivered {
                let mut state_machines = self.state_machines.write().await;
                if let Some(sm) = state_machines.get_mut(&event.terminal_id) {
                    sm.reset();
                }

                decision_to_publish = PromptDecision::skip(
                    "Skipped prompt response: terminal input could not be delivered",
                );
            }
        }

        // Publish decision for UI updates
        self.message_bus
            .publish_terminal_prompt_decision(
                &event.terminal_id,
                &event.workflow_id,
                decision_to_publish,
            )
            .await;

        Some(decision)
    }

    /// Make a decision for a detected prompt
    async fn make_decision(&self, prompt: &DetectedPrompt, auto_confirm: bool) -> PromptDecision {
        // Rule 1: Password prompts always require user intervention
        if prompt.kind == PromptKind::Password {
            tracing::info!(
                prompt_kind = ?prompt.kind,
                "Password prompt detected - requiring user intervention"
            );
            return PromptDecision::ask_password();
        }

        // Rule 2: auto_confirm disabled means always ask user
        if !auto_confirm {
            tracing::info!(
                prompt_kind = ?prompt.kind,
                raw_text = %prompt.raw_text,
                "Auto-confirm disabled for terminal - requiring user intervention"
            );
            return PromptDecision::AskUser {
                reason: "Auto-confirm disabled for this terminal".to_string(),
                suggestions: self.get_suggestions_for_prompt(prompt),
            };
        }

        // Rule 3: Dangerous prompts escalate to user. Advisory checklist text is
        // ignored unless it carries a live destructive confirmation signal.
        if should_require_user_confirmation(prompt) {
            tracing::warn!(
                prompt_kind = ?prompt.kind,
                raw_text = %prompt.raw_text,
                detector_flag = prompt.has_dangerous_keywords,
                "Dangerous prompt detected - requiring user confirmation"
            );
            return PromptDecision::AskUser {
                reason: format!(
                    "Dangerous operation detected: {}",
                    prompt.raw_text.chars().take(100).collect::<String>()
                ),
                suggestions: self.get_suggestions_for_prompt(prompt),
            };
        }

        if prompt.has_dangerous_keywords {
            tracing::debug!(
                prompt_kind = ?prompt.kind,
                raw_text = %prompt.raw_text,
                "Dangerous keywords detected inside advisory checklist; skipping AskUser escalation"
            );
        }

        // Rule 4: EnterConfirm with high confidence - auto-confirm
        if prompt.kind == PromptKind::EnterConfirm
            && prompt.confidence >= AUTO_CONFIRM_CONFIDENCE_THRESHOLD
        {
            tracing::info!(
                prompt_kind = ?prompt.kind,
                confidence = prompt.confidence,
                "Auto-confirming EnterConfirm prompt"
            );
            return PromptDecision::auto_enter();
        }

        // Rule 5: Other prompts - use rule-based defaults or LLM
        // For now, use conservative rule-based defaults
        // TODO: Integrate actual LLM call when LLM service is available
        self.make_rule_based_decision(prompt).await
    }

    /// Make a rule-based decision (fallback when LLM is not available)
    async fn make_rule_based_decision(&self, prompt: &DetectedPrompt) -> PromptDecision {
        match prompt.kind {
            PromptKind::EnterConfirm => {
                // Lower confidence EnterConfirm - still auto-confirm but log
                tracing::debug!(
                    confidence = prompt.confidence,
                    "Auto-confirming EnterConfirm with lower confidence"
                );
                PromptDecision::auto_enter()
            }

            PromptKind::YesNo => {
                // Default to 'yes' for non-dangerous prompts
                // In production, this should use LLM
                tracing::info!(
                    raw_text = %prompt.raw_text,
                    "YesNo prompt - defaulting to 'yes' (rule-based)"
                );
                PromptDecision::llm_yes_no(
                    true,
                    "Rule-based default: answering 'yes' to non-dangerous prompt".to_string(),
                )
            }

            PromptKind::Choice => {
                // Default to first option for non-dangerous prompts
                // In production, this should use LLM
                tracing::info!(
                    raw_text = %prompt.raw_text,
                    "Choice prompt - defaulting to first option (rule-based)"
                );
                PromptDecision::llm_choice(
                    "1",
                    "Rule-based default: selecting first option".to_string(),
                )
            }

            PromptKind::ArrowSelect => {
                let normalized_prompt = prompt.raw_text.to_ascii_lowercase();
                let spinner_noise_score = SPINNER_NOISE_MARKERS
                    .iter()
                    .filter(|marker| normalized_prompt.contains(**marker))
                    .count();

                if spinner_noise_score > 0 {
                    tracing::warn!(
                        raw_text = %prompt.raw_text,
                        spinner_noise_score,
                        "ArrowSelect prompt looks like spinner noise; skip auto arrow injection"
                    );
                    return PromptDecision::Skip {
                        reason: "Ignore spinner-like pseudo menu to avoid wrong key injection"
                            .to_string(),
                    };
                }

                // Default to first option (index 0)
                // In production, this should use LLM
                let current_index = prompt.selected_index.unwrap_or(0);
                let target_index = 0; // Default to first option

                tracing::info!(
                    raw_text = %prompt.raw_text,
                    current_index = current_index,
                    target_index = target_index,
                    "ArrowSelect prompt - defaulting to first option (rule-based)"
                );

                PromptDecision::llm_arrow_select(
                    current_index,
                    target_index,
                    "Rule-based default: selecting first option".to_string(),
                )
            }

            PromptKind::Input => {
                if Self::is_codex_composer_like_input(&prompt.raw_text) {
                    tracing::info!(
                        raw_text = %prompt.raw_text,
                        "Input prompt looks like Codex composer/status line; auto-submitting Enter"
                    );
                    return PromptDecision::auto_enter();
                }

                if let Some(ref callback) = self.llm_callback {
                    let task_ctx = {
                        let contexts = self.task_contexts.read().await;
                        contexts.values().next().cloned().unwrap_or_default()
                    };
                    let llm_prompt = format!(
                        "A terminal is asking for free-form input. Provide ONLY the text to type, nothing else.\n\n\
                         Task context: {task_ctx}\n\n\
                         Prompt shown: {raw}\n\n\
                         Respond with ONLY the input text.",
                        raw = prompt.raw_text,
                    );
                    match tokio::time::timeout(
                        Duration::from_secs(30),
                        (callback)(llm_prompt),
                    )
                    .await
                    {
                        Ok(Some(response)) => {
                            tracing::info!(
                                raw_text = %prompt.raw_text,
                                "LLM generated input response for free-form prompt"
                            );
                            return PromptDecision::LLMDecision {
                                response,
                                reasoning: "LLM-generated free-form input".to_string(),
                                target_index: None,
                            };
                        }
                        Ok(None) => {
                            tracing::warn!("LLM input callback returned None, falling back to AskUser");
                        }
                        Err(_) => {
                            tracing::warn!("LLM input callback timed out (30s), falling back to AskUser");
                        }
                    }
                }

                tracing::info!(
                    raw_text = %prompt.raw_text,
                    "Input prompt - requiring user input"
                );
                PromptDecision::AskUser {
                    reason: "Free-form input required".to_string(),
                    suggestions: None,
                }
            }

            PromptKind::Password => {
                // Should not reach here (handled above), but be safe
                PromptDecision::ask_password()
            }
        }
    }

    /// Get response string from a decision (if applicable)
    fn get_response_from_decision(&self, decision: &PromptDecision) -> Option<String> {
        match decision {
            PromptDecision::AutoConfirm { response, .. } => Some(response.clone()),
            PromptDecision::LLMDecision { response, .. } => Some(response.clone()),
            PromptDecision::AskUser { .. } | PromptDecision::Skip { .. } => None,
        }
    }

    /// Get suggestions for a prompt (for AskUser decisions)
    fn get_suggestions_for_prompt(&self, prompt: &DetectedPrompt) -> Option<Vec<String>> {
        match prompt.kind {
            PromptKind::YesNo => Some(vec!["y".to_string(), "n".to_string()]),
            PromptKind::ArrowSelect => prompt
                .options
                .as_ref()
                .map(|opts| opts.iter().map(|o| o.label.clone()).collect()),
            _ => None,
        }
    }

    /// Reset state for a terminal
    pub async fn reset_terminal_state(&self, terminal_id: &str) {
        let mut state_machines = self.state_machines.write().await;
        if let Some(sm) = state_machines.get_mut(terminal_id) {
            sm.reset();
        }
    }

    /// Handle user response for a waiting prompt.
    ///
    /// Returns `true` when the response is accepted and forwarded to PTY.
    pub async fn handle_user_prompt_response(
        &self,
        terminal_id: &str,
        session_id: &str,
        workflow_id: &str,
        user_response: &str,
    ) -> bool {
        let response = format!("{user_response}\n");

        let (decision, should_publish_input, handled) = {
            let mut state_machines = self.state_machines.write().await;

            match state_machines.get_mut(terminal_id) {
                Some(sm) if sm.state == PromptState::WaitingForApproval => {
                    let decision = PromptDecision::LLMDecision {
                        response: response.clone(),
                        reasoning: "User provided response".to_string(),
                        target_index: None,
                    };
                    sm.on_response_sent(decision.clone());
                    (decision, true, true)
                }
                Some(sm) => {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        workflow_id = %workflow_id,
                        state = ?sm.state,
                        "Received user prompt response while terminal is not waiting for approval"
                    );
                    (
                        PromptDecision::skip(&format!(
                            "Ignored prompt response: terminal is not waiting for approval (state: {:?})",
                            sm.state
                        )),
                        false,
                        false,
                    )
                }
                None => {
                    tracing::warn!(
                        terminal_id = %terminal_id,
                        workflow_id = %workflow_id,
                        "Received user prompt response for terminal without prompt state"
                    );
                    (
                        PromptDecision::skip(
                            "Ignored prompt response: terminal has no active prompt state",
                        ),
                        false,
                        false,
                    )
                }
            }
        };

        let mut decision_to_publish = decision.clone();

        if should_publish_input {
            let delivered = self
                .message_bus
                .publish_terminal_input(terminal_id, session_id, &response, Some(decision.clone()))
                .await;
            if !delivered {
                let mut state_machines = self.state_machines.write().await;
                if let Some(sm) = state_machines.get_mut(terminal_id) {
                    sm.reset();
                }

                decision_to_publish = PromptDecision::skip(
                    "Skipped prompt response: terminal input could not be delivered",
                );
            }
        }

        self.message_bus
            .publish_terminal_prompt_decision(terminal_id, workflow_id, decision_to_publish)
            .await;

        handled
    }

    /// Backward-compatible alias for `handle_user_prompt_response`.
    pub async fn handle_user_approval(
        &self,
        terminal_id: &str,
        session_id: &str,
        workflow_id: &str,
        user_response: &str,
    ) -> bool {
        self.handle_user_prompt_response(terminal_id, session_id, workflow_id, user_response)
            .await
    }
}

// ============================================================================
// LLM Prompt Template
// ============================================================================

/// Build LLM prompt for making a decision about a terminal prompt
pub fn build_llm_decision_prompt(request: &LLMPromptDecisionRequest) -> String {
    let mut prompt = format!(
        r"You are an AI assistant helping to respond to an interactive terminal prompt.

## Prompt Information
- Type: {}
- Text: {}
- Has dangerous keywords: {}
",
        request.prompt_kind, request.prompt_text, request.has_dangerous_keywords
    );

    if let Some(ref options) = request.options {
        prompt.push_str("\n## Available Options\n");
        for (i, opt) in options.iter().enumerate() {
            prompt.push_str(&format!("{i}. {opt}\n"));
        }
    }

    if let Some(idx) = request.current_index {
        prompt.push_str(&format!("\nCurrently selected: option {idx}\n"));
    }

    if let Some(ref context) = request.task_context {
        prompt.push_str(&format!("\n## Task Context\n{context}\n"));
    }

    prompt.push_str(
        r#"
## Instructions
Analyze the prompt and decide how to respond. Return a JSON object with:
- "action": "confirm" | "select" | "input" | "ask_user"
- "response": the text to send (if action is not "ask_user")
- "target_index": the option index to select (for ArrowSelect prompts)
- "reasoning": brief explanation of your decision
- "ask_user": true if human intervention is needed

## Response Format
```json
{
  "action": "...",
  "response": "...",
  "target_index": null,
  "reasoning": "...",
  "ask_user": false
}
```
"#,
    );

    prompt
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::services::{
        orchestrator::message_bus::{BusMessage, MessageBus},
        terminal::prompt_detector::DetectedPrompt,
    };

    fn create_test_handler() -> PromptHandler {
        let message_bus = Arc::new(MessageBus::new(100));
        PromptHandler::new(message_bus)
    }

    fn create_test_prompt(kind: PromptKind, text: &str, confidence: f32) -> DetectedPrompt {
        DetectedPrompt::new(kind, text.to_string(), confidence)
    }

    #[tokio::test]
    async fn test_password_always_asks_user() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(PromptKind::Password, "Enter password:", 0.95);

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AskUser { reason, .. } => {
                assert!(reason.contains("Password") || reason.contains("sensitive"));
            }
            _ => panic!("Expected AskUser decision for password prompt"),
        }
    }

    #[tokio::test]
    async fn test_enter_confirm_auto_confirms() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue", 0.90);

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AutoConfirm { response, .. } => {
                assert_eq!(response, "\n");
            }
            _ => panic!("Expected AutoConfirm decision for EnterConfirm prompt"),
        }
    }

    #[tokio::test]
    async fn test_yes_no_defaults_to_yes() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(PromptKind::YesNo, "Continue? [y/n]", 0.90);

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::LLMDecision { response, .. } => {
                assert_eq!(response, "y\n");
            }
            _ => panic!("Expected LLMDecision for YesNo prompt"),
        }
    }

    #[tokio::test]
    async fn test_input_asks_user() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(PromptKind::Input, "Enter your name:", 0.85);

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AskUser { .. } => {}
            _ => panic!("Expected AskUser decision for Input prompt"),
        }
    }

    #[tokio::test]
    async fn test_codex_composer_like_input_auto_confirms() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(
            PromptKind::Input,
            "Stopping due to unexpected changes ... ? for shortcuts 98% context left",
            0.80,
        );

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AutoConfirm { response, .. } => {
                assert_eq!(response, "\n");
            }
            _ => panic!("Expected AutoConfirm decision for Codex composer-like Input prompt"),
        }
    }

    #[tokio::test]
    async fn test_spinner_like_arrow_select_is_skipped() {
        let handler = create_test_handler();
        let mut prompt = create_test_prompt(
            PromptKind::ArrowSelect,
            "* Brewing…\n* Brewing…\n* Brewing…",
            0.95,
        );
        prompt.selected_index = Some(0);

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::Skip { reason } => {
                assert!(reason.contains("spinner-like"));
            }
            _ => panic!("Expected Skip decision for spinner-like ArrowSelect noise"),
        }
    }

    #[tokio::test]
    async fn test_spinner_like_arrow_select_does_not_publish_terminal_input() {
        let message_bus = Arc::new(MessageBus::new(100));
        let handler = PromptHandler::new(message_bus.clone());
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        let mut prompt = create_test_prompt(
            PromptKind::ArrowSelect,
            "* Brewing…\n* Brewing…\n* Brewing…",
            0.95,
        );
        prompt.selected_index = Some(0);

        let event = TerminalPromptEvent {
            terminal_id: "term-1".to_string(),
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            session_id: "session-1".to_string(),
            auto_confirm: true,
            prompt,
            detected_at: chrono::Utc::now(),
        };

        let decision = handler
            .handle_prompt_event(&event)
            .await
            .expect("spinner-like prompt should still produce a decision");
        assert!(matches!(decision, PromptDecision::Skip { .. }));

        let decision_message =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
                .await
                .expect("expected decision broadcast")
                .expect("broadcast channel should be open");

        assert!(matches!(
            decision_message,
            BusMessage::TerminalPromptDecision {
                decision: PromptDecision::Skip { .. },
                ..
            }
        ));

        let maybe_input =
            tokio::time::timeout(Duration::from_millis(120), broadcast_rx.recv()).await;
        assert!(
            maybe_input.is_err(),
            "spinner-like prompt should not emit TerminalInput"
        );
    }

    #[tokio::test]
    async fn test_dangerous_keywords_ask_user() {
        let handler = create_test_handler();
        let mut prompt = create_test_prompt(PromptKind::YesNo, "Delete all files? [y/n]", 0.90);
        // Manually set dangerous keywords flag
        prompt.has_dangerous_keywords = true;

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AskUser { reason, .. } => {
                assert!(reason.contains("Dangerous"));
            }
            _ => panic!("Expected AskUser decision for dangerous prompt"),
        }
    }

    #[tokio::test]
    async fn test_advisory_checklist_enter_confirm_auto_confirms() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(
            PromptKind::EnterConfirm,
            "Checklist:\n- [ ] Verify cleanup target\n- [ ] Confirm backup exists\n\nPress Enter to continue",
            0.95,
        );

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AutoConfirm { response, .. } => {
                assert_eq!(response, "\n");
            }
            _ => panic!("Expected AutoConfirm decision for advisory checklist EnterConfirm"),
        }
    }

    #[tokio::test]
    async fn test_checklist_with_destructive_yes_no_asks_user_even_without_danger_flag() {
        let handler = create_test_handler();
        let mut prompt = create_test_prompt(
            PromptKind::YesNo,
            "Checklist:\n- [ ] Verify cleanup target\n- [ ] Confirm backup exists\n\nDelete the remote branch? [y/n]",
            0.95,
        );
        prompt.has_dangerous_keywords = false;

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AskUser { reason, .. } => {
                assert!(reason.contains("Dangerous"));
            }
            _ => panic!("Expected AskUser decision for destructive checklist YesNo prompt"),
        }
    }

    #[tokio::test]
    async fn test_mixed_checklist_warning_and_enter_confirm_asks_user() {
        let handler = create_test_handler();
        let mut prompt = create_test_prompt(
            PromptKind::EnterConfirm,
            "Checklist:\n- [ ] Verify cleanup target\n- [ ] Confirm backup exists\n\nThis will permanently delete the remote branch.\nPress Enter to continue",
            0.95,
        );
        prompt.has_dangerous_keywords = false;

        let decision = handler.make_decision(&prompt, true).await;

        match decision {
            PromptDecision::AskUser { reason, .. } => {
                assert!(reason.contains("Dangerous"));
            }
            _ => panic!(
                "Expected AskUser decision for mixed checklist with dangerous enter-confirm"
            ),
        }
    }

    #[tokio::test]
    async fn test_advisory_checklist_arrow_select_does_not_escalate() {
        let handler = create_test_handler();
        let prompt = DetectedPrompt::arrow_select(
            "### PRE-FLIGHT CHECKLIST\n[ ] Review merge target before push\n[ ] Verify no force push is required"
                .to_string(),
            0.95,
            vec![
                crate::services::terminal::ArrowSelectOption {
                    index: 0,
                    label: "Review merge target before push".to_string(),
                    selected: true,
                },
                crate::services::terminal::ArrowSelectOption {
                    index: 1,
                    label: "Verify no force push is required".to_string(),
                    selected: false,
                },
            ],
            0,
        );

        let decision = handler.make_decision(&prompt, true).await;

        assert!(
            !matches!(decision, PromptDecision::AskUser { .. }),
            "pure advisory checklist text should not escalate to AskUser"
        );
    }

    #[tokio::test]
    async fn test_mixed_checklist_arrow_select_with_delete_option_asks_user() {
        let handler = create_test_handler();
        let prompt = DetectedPrompt::arrow_select(
            "### PRE-FLIGHT CHECKLIST\n[ ] Review merge target before push\n[ ] Verify no force push is required\nDelete remote branch now?\n> Yes, delete it\n  No, keep branch"
                .to_string(),
            0.95,
            vec![
                crate::services::terminal::ArrowSelectOption {
                    index: 0,
                    label: "Yes, delete it".to_string(),
                    selected: true,
                },
                crate::services::terminal::ArrowSelectOption {
                    index: 1,
                    label: "No, keep branch".to_string(),
                    selected: false,
                },
            ],
            0,
        );

        let decision = handler.make_decision(&prompt, true).await;

        assert!(
            matches!(decision, PromptDecision::AskUser { .. }),
            "mixed checklist + destructive confirmation must still escalate"
        );
    }

    #[tokio::test]
    async fn test_auto_confirm_disabled_always_asks_user() {
        let handler = create_test_handler();
        let prompt = create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue", 0.95);

        let decision = handler.make_decision(&prompt, false).await;

        match decision {
            PromptDecision::AskUser { reason, .. } => {
                assert!(reason.contains("Auto-confirm disabled"));
            }
            _ => panic!("Expected AskUser decision when auto_confirm=false"),
        }
    }

    #[tokio::test]
    async fn test_handle_prompt_event_auto_confirm_disabled_only_publishes_ask_user_decision() {
        let message_bus = Arc::new(MessageBus::new(100));
        let handler = PromptHandler::new(message_bus.clone());
        let mut broadcast_rx = message_bus.subscribe_broadcast();

        let event = TerminalPromptEvent {
            terminal_id: "term-1".to_string(),
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            session_id: "session-1".to_string(),
            auto_confirm: false,
            prompt: create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue", 0.95),
            detected_at: chrono::Utc::now(),
        };

        let decision = handler
            .handle_prompt_event(&event)
            .await
            .expect("prompt should produce decision");

        match decision {
            PromptDecision::AskUser { reason, .. } => {
                assert!(reason.contains("Auto-confirm disabled"));
            }
            _ => panic!("Expected AskUser decision when auto_confirm=false"),
        }

        let first_message = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected prompt decision broadcast")
            .expect("broadcast channel should be open");

        match first_message {
            BusMessage::TerminalPromptDecision {
                terminal_id,
                workflow_id,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(workflow_id, "workflow-1");
                assert!(matches!(decision, PromptDecision::AskUser { .. }));
            }
            other => panic!("expected TerminalPromptDecision, got: {other:?}"),
        }

        let maybe_second =
            tokio::time::timeout(Duration::from_millis(100), broadcast_rx.recv()).await;
        assert!(
            maybe_second.is_err(),
            "AskUser should not publish terminal input"
        );
    }

    #[tokio::test]
    async fn test_handle_prompt_event_auto_confirm_enabled_publishes_decision_and_terminal_input() {
        let message_bus = Arc::new(MessageBus::new(100));
        let handler = PromptHandler::new(message_bus.clone());
        let mut broadcast_rx = message_bus.subscribe_broadcast();
        let mut input_topic_rx = message_bus.subscribe("terminal.input.term-1").await;

        let event = TerminalPromptEvent {
            terminal_id: "term-1".to_string(),
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            session_id: "session-1".to_string(),
            auto_confirm: true,
            prompt: create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue", 0.95),
            detected_at: chrono::Utc::now(),
        };

        let decision = handler
            .handle_prompt_event(&event)
            .await
            .expect("prompt should produce decision");

        match decision {
            PromptDecision::AutoConfirm { response, .. } => {
                assert_eq!(response, "\n");
            }
            _ => panic!("Expected AutoConfirm decision when auto_confirm=true"),
        }

        let first_message = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected first broadcast event")
            .expect("broadcast channel should be open");
        let second_message = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected second broadcast event")
            .expect("broadcast channel should be open");

        let mut saw_auto_confirm_decision = false;
        let mut saw_terminal_input = false;

        for message in [first_message, second_message] {
            match message {
                BusMessage::TerminalPromptDecision {
                    terminal_id,
                    workflow_id,
                    decision,
                } => {
                    assert_eq!(terminal_id, "term-1");
                    assert_eq!(workflow_id, "workflow-1");
                    assert!(matches!(decision, PromptDecision::AutoConfirm { .. }));
                    saw_auto_confirm_decision = true;
                }
                BusMessage::TerminalInput {
                    terminal_id,
                    session_id,
                    input,
                    decision,
                } => {
                    assert_eq!(terminal_id, "term-1");
                    assert_eq!(session_id, "session-1");
                    assert_eq!(input, "\n");
                    assert!(matches!(decision, Some(PromptDecision::AutoConfirm { .. })));
                    saw_terminal_input = true;
                }
                other => panic!("unexpected broadcast message: {other:?}"),
            }
        }

        assert!(saw_auto_confirm_decision);
        assert!(saw_terminal_input);

        let topic_message = tokio::time::timeout(Duration::from_millis(200), input_topic_rx.recv())
            .await
            .expect("expected topic terminal input")
            .expect("topic channel should be open");

        match topic_message {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "\n");
                assert!(matches!(decision, Some(PromptDecision::AutoConfirm { .. })));
            }
            other => panic!("expected topic TerminalInput, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_handle_user_approval_after_ask_user_event_publishes_manual_response() {
        let message_bus = Arc::new(MessageBus::new(100));
        let handler = PromptHandler::new(message_bus.clone());
        let mut broadcast_rx = message_bus.subscribe_broadcast();
        let mut input_topic_rx = message_bus.subscribe("terminal.input.term-1").await;

        let event = TerminalPromptEvent {
            terminal_id: "term-1".to_string(),
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            session_id: "session-1".to_string(),
            auto_confirm: false,
            prompt: create_test_prompt(PromptKind::YesNo, "Continue? [y/n]", 0.95),
            detected_at: chrono::Utc::now(),
        };

        let ask_user_decision = handler
            .handle_prompt_event(&event)
            .await
            .expect("prompt should produce AskUser decision");

        match ask_user_decision {
            PromptDecision::AskUser { suggestions, .. } => {
                assert_eq!(suggestions, Some(vec!["y".to_string(), "n".to_string()]));
            }
            _ => panic!("Expected AskUser decision when auto_confirm=false"),
        }

        let ask_user_message =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
                .await
                .expect("expected AskUser decision broadcast")
                .expect("broadcast channel should be open");

        match ask_user_message {
            BusMessage::TerminalPromptDecision { decision, .. } => {
                assert!(matches!(decision, PromptDecision::AskUser { .. }));
            }
            other => panic!("expected AskUser TerminalPromptDecision, got: {other:?}"),
        }

        let handled = handler
            .handle_user_prompt_response("term-1", "session-1", "workflow-1", "n")
            .await;
        assert!(
            handled,
            "approval should be accepted when terminal is waiting"
        );

        let input_broadcast = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected manual input broadcast")
            .expect("broadcast channel should be open");

        match input_broadcast {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "n\n");
                assert!(matches!(decision, Some(PromptDecision::LLMDecision { .. })));
            }
            other => panic!("expected TerminalInput, got: {other:?}"),
        }

        let topic_input = tokio::time::timeout(Duration::from_millis(200), input_topic_rx.recv())
            .await
            .expect("expected manual input topic message")
            .expect("topic channel should be open");

        match topic_input {
            BusMessage::TerminalInput {
                terminal_id,
                session_id,
                input,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(session_id, "session-1");
                assert_eq!(input, "n\n");
                assert!(matches!(decision, Some(PromptDecision::LLMDecision { .. })));
            }
            other => panic!("expected topic TerminalInput, got: {other:?}"),
        }

        let decision_broadcast =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
                .await
                .expect("expected manual decision broadcast")
                .expect("broadcast channel should be open");

        match decision_broadcast {
            BusMessage::TerminalPromptDecision {
                terminal_id,
                workflow_id,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(workflow_id, "workflow-1");
                match decision {
                    PromptDecision::LLMDecision { response, .. } => {
                        assert_eq!(response, "n\n");
                    }
                    _ => panic!("Expected LLMDecision broadcast after manual approval"),
                }
            }
            other => panic!("expected TerminalPromptDecision, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_handle_user_approval_when_terminal_not_waiting_returns_false_with_feedback() {
        let message_bus = Arc::new(MessageBus::new(100));
        let handler = PromptHandler::new(message_bus.clone());
        let mut broadcast_rx = message_bus.subscribe_broadcast();
        let mut input_topic_rx = message_bus.subscribe("terminal.input.term-1").await;

        let event = TerminalPromptEvent {
            terminal_id: "term-1".to_string(),
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            session_id: "session-1".to_string(),
            auto_confirm: true,
            prompt: create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue", 0.95),
            detected_at: chrono::Utc::now(),
        };

        let decision = handler
            .handle_prompt_event(&event)
            .await
            .expect("prompt should produce auto-confirm decision");
        assert!(matches!(decision, PromptDecision::AutoConfirm { .. }));

        let initial_message_1 =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
                .await
                .expect("expected first initial broadcast")
                .expect("broadcast channel should be open");
        let initial_message_2 =
            tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
                .await
                .expect("expected second initial broadcast")
                .expect("broadcast channel should be open");

        let mut saw_auto_confirm_decision = false;
        let mut saw_terminal_input = false;

        for message in [initial_message_1, initial_message_2] {
            match message {
                BusMessage::TerminalPromptDecision {
                    decision: PromptDecision::AutoConfirm { .. },
                    ..
                } => {
                    saw_auto_confirm_decision = true;
                }
                BusMessage::TerminalInput { .. } => {
                    saw_terminal_input = true;
                }
                other => panic!("unexpected initial message: {other:?}"),
            }
        }

        assert!(saw_auto_confirm_decision);
        assert!(saw_terminal_input);

        let topic_input = tokio::time::timeout(Duration::from_millis(200), input_topic_rx.recv())
            .await
            .expect("expected topic terminal input")
            .expect("topic channel should be open");
        assert!(matches!(topic_input, BusMessage::TerminalInput { .. }));

        let handled = handler
            .handle_user_prompt_response("term-1", "session-1", "workflow-1", "n")
            .await;
        assert!(
            !handled,
            "approval should be rejected when terminal is not waiting"
        );

        let feedback = tokio::time::timeout(Duration::from_millis(200), broadcast_rx.recv())
            .await
            .expect("expected non-waiting feedback broadcast")
            .expect("broadcast channel should be open");

        match feedback {
            BusMessage::TerminalPromptDecision {
                terminal_id,
                workflow_id,
                decision,
            } => {
                assert_eq!(terminal_id, "term-1");
                assert_eq!(workflow_id, "workflow-1");
                match decision {
                    PromptDecision::Skip { reason } => {
                        assert!(reason.contains("not waiting for approval"));
                        assert!(reason.contains("prompt response"));
                    }
                    _ => panic!("Expected Skip feedback for non-waiting terminal"),
                }
            }
            other => panic!("expected feedback TerminalPromptDecision, got: {other:?}"),
        }

        let no_follow_up =
            tokio::time::timeout(Duration::from_millis(100), broadcast_rx.recv()).await;
        assert!(
            no_follow_up.is_err(),
            "non-waiting approval should not publish terminal input"
        );
    }

    #[test]
    fn test_build_llm_decision_prompt() {
        let request = LLMPromptDecisionRequest {
            prompt_kind: "YesNo".to_string(),
            prompt_text: "Continue? [y/n]".to_string(),
            options: None,
            current_index: None,
            has_dangerous_keywords: false,
            task_context: Some("Installing dependencies".to_string()),
        };

        let prompt = build_llm_decision_prompt(&request);

        assert!(prompt.contains("YesNo"));
        assert!(prompt.contains("Continue? [y/n]"));
        assert!(prompt.contains("Installing dependencies"));
        assert!(prompt.contains("JSON"));
    }

    #[tokio::test]
    async fn test_failed_delivery_resets_state_machine_after_auto_confirm() {
        let message_bus = Arc::new(MessageBus::new(100));
        let handler = PromptHandler::new(message_bus.clone());

        let event = TerminalPromptEvent {
            terminal_id: "term-no-route".to_string(),
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            session_id: "session-no-route".to_string(),
            auto_confirm: true,
            prompt: create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue", 0.95),
            detected_at: chrono::Utc::now(),
        };

        let _ = handler
            .handle_prompt_event(&event)
            .await
            .expect("prompt should produce decision");

        let state_machines = handler.state_machines.read().await;
        let terminal_state = state_machines
            .get("term-no-route")
            .expect("state machine should exist");
        assert_eq!(terminal_state.state, PromptState::Idle);
    }
}
