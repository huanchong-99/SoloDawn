//! Orchestrator 类型定义

use serde::{Deserialize, Serialize};

// ============================================================================
// Prompt Types (re-exported from terminal module for convenience)
// ============================================================================
pub use crate::services::terminal::{
    ARROW_DOWN, ARROW_UP, ArrowSelectOption, DetectedPrompt, PromptDetector, PromptKind,
    build_arrow_sequence,
};

/// 主 Agent 指令类型
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum OrchestratorInstruction {
    /// 启动任务
    StartTask {
        task_id: String,
        instruction: String,
    },
    /// 创建运行时任务（仅 agent_planned 模式）
    CreateTask {
        task_id: Option<String>,
        name: String,
        description: Option<String>,
        branch: Option<String>,
        order_index: Option<i32>,
    },
    /// 创建运行时终端（仅 agent_planned 模式）
    CreateTerminal {
        terminal_id: Option<String>,
        task_id: String,
        cli_type_id: String,
        model_config_id: String,
        custom_base_url: Option<String>,
        custom_api_key: Option<String>,
        role: Option<String>,
        role_description: Option<String>,
        order_index: Option<i32>,
        auto_confirm: Option<bool>,
    },
    /// 启动运行时终端并发送首条指令（仅 agent_planned 模式）
    StartTerminal {
        terminal_id: String,
        instruction: String,
    },
    /// 关闭终端，保留最终状态与历史
    CloseTerminal {
        terminal_id: String,
        final_status: Option<String>,
    },
    /// 标记任务规划完成；当全部终端结束后任务将进入最终状态
    CompleteTask {
        task_id: String,
        summary: String,
    },
    /// 标记工作流规划完成，后续不再增加任务/终端
    SetWorkflowPlanningComplete {
        summary: Option<String>,
    },
    /// 发送消息到终端
    SendToTerminal {
        terminal_id: String,
        message: String,
    },
    /// 审核代码
    ReviewCode {
        terminal_id: String,
        commit_hash: String,
    },
    /// 修复问题
    FixIssues {
        terminal_id: String,
        issues: Vec<String>,
    },
    /// 合并分支
    MergeBranch {
        source_branch: String,
        target_branch: String,
    },
    /// 暂停工作流
    PauseWorkflow { reason: String },
    /// 完成工作流
    CompleteWorkflow { summary: String },
    /// 失败工作流
    FailWorkflow { reason: String },
}

/// 终端完成事件
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TerminalCompletionEvent {
    pub terminal_id: String,
    pub task_id: String,
    pub workflow_id: String,
    pub status: TerminalCompletionStatus,
    pub commit_hash: Option<String>,
    pub commit_message: Option<String>,
    pub metadata: Option<CommitMetadata>,
}

/// 终端完成状态
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TerminalCompletionStatus {
    /// 任务完成
    Completed,
    /// 审核通过
    ReviewPass,
    /// 审核打回
    ReviewReject,
    /// 失败
    Failed,
}

/// Git 提交元数据
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommitMetadata {
    pub workflow_id: String,
    pub task_id: String,
    pub terminal_id: String,
    pub terminal_order: i32,
    pub cli: String,
    pub model: String,
    pub status: String,
    pub severity: Option<String>,
    pub reviewed_terminal: Option<String>,
    pub issues: Option<Vec<CodeIssue>>,
    pub next_action: String,
}

/// 代码问题
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CodeIssue {
    pub severity: String,
    pub file: String,
    pub line: Option<i32>,
    pub message: String,
    pub suggestion: Option<String>,
}

/// LLM 消息
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LLMMessage {
    pub role: String,
    pub content: String,
}

/// LLM 响应
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LLMResponse {
    pub content: String,
    pub usage: Option<LLMUsage>,
}

/// LLM 使用量
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LLMUsage {
    pub prompt_tokens: i32,
    pub completion_tokens: i32,
    pub total_tokens: i32,
}

// ============================================================================
// Terminal Prompt Event Types
// ============================================================================

/// Terminal prompt detected event
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TerminalPromptEvent {
    /// Terminal ID
    pub terminal_id: String,
    /// Workflow ID
    pub workflow_id: String,
    /// Task ID
    pub task_id: String,
    /// PTY session ID for sending responses
    pub session_id: String,
    /// Whether auto-confirm is enabled for this terminal
    pub auto_confirm: bool,
    /// Detected prompt details
    pub prompt: DetectedPrompt,
    /// Timestamp when prompt was detected
    pub detected_at: chrono::DateTime<chrono::Utc>,
}

/// Decision made by Orchestrator for a prompt
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "action", rename_all = "snake_case")]
pub enum PromptDecision {
    /// Auto-confirm: send response immediately without LLM
    AutoConfirm {
        /// Response to send (e.g., "\n" for EnterConfirm)
        response: String,
        /// Reason for auto-confirm
        reason: String,
    },
    /// LLM decision: let LLM decide the response
    LLMDecision {
        /// Response determined by LLM
        response: String,
        /// LLM's reasoning
        reasoning: String,
        /// For ArrowSelect: target index
        target_index: Option<usize>,
    },
    /// Ask user: requires human intervention
    AskUser {
        /// Reason why user input is needed
        reason: String,
        /// Suggested options (if any)
        suggestions: Option<Vec<String>>,
    },
    /// Skip: ignore this prompt (e.g., duplicate detection)
    Skip {
        /// Reason for skipping
        reason: String,
    },
}

impl PromptDecision {
    /// Create an auto-confirm decision for EnterConfirm prompts
    pub fn auto_enter() -> Self {
        Self::AutoConfirm {
            response: "\n".to_string(),
            reason: "EnterConfirm prompt with high confidence".to_string(),
        }
    }

    /// Create an ask-user decision for Password prompts
    pub fn ask_password() -> Self {
        Self::AskUser {
            reason: "Password/sensitive input detected - requires user intervention".to_string(),
            suggestions: None,
        }
    }

    /// Create an LLM decision for YesNo prompts
    pub fn llm_yes_no(answer_yes: bool, reasoning: String) -> Self {
        Self::LLMDecision {
            response: if answer_yes {
                "y\n".to_string()
            } else {
                "n\n".to_string()
            },
            reasoning,
            target_index: None,
        }
    }

    /// Create an LLM decision for ArrowSelect prompts
    pub fn llm_arrow_select(current_index: usize, target_index: usize, reasoning: String) -> Self {
        let arrow_sequence = build_arrow_sequence(current_index, target_index);
        Self::LLMDecision {
            response: format!("{}\n", arrow_sequence),
            reasoning,
            target_index: Some(target_index),
        }
    }

    /// Create an LLM decision for Choice prompts
    pub fn llm_choice(choice: &str, reasoning: String) -> Self {
        Self::LLMDecision {
            response: format!("{}\n", choice),
            reasoning,
            target_index: None,
        }
    }

    /// Create an LLM decision for Input prompts
    pub fn llm_input(input: &str, reasoning: String) -> Self {
        Self::LLMDecision {
            response: format!("{}\n", input),
            reasoning,
            target_index: None,
        }
    }

    /// Create a skip decision
    pub fn skip(reason: &str) -> Self {
        Self::Skip {
            reason: reason.to_string(),
        }
    }
}

/// Terminal prompt state for tracking prompt handling
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PromptState {
    /// No prompt detected
    Idle,
    /// Prompt detected, waiting for decision
    Detected,
    /// Decision made, waiting for response to be sent
    Deciding,
    /// Response sent, waiting for terminal to process
    Responding,
    /// Waiting for user approval (Password prompts)
    WaitingForApproval,
}

impl Default for PromptState {
    fn default() -> Self {
        Self::Idle
    }
}

/// Retry window for re-processing an identical prompt while mid-flight.
///
/// This prevents long-term suppression when prompt state gets stuck in
/// `Detected/Deciding/Responding`, while still avoiding duplicate storms.
const SAME_PROMPT_RETRY_WINDOW_SECS: i64 = 5;

/// Terminal prompt state machine to prevent duplicate responses
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TerminalPromptStateMachine {
    /// Current state
    pub state: PromptState,
    /// Last detected prompt (if any)
    pub last_prompt: Option<DetectedPrompt>,
    /// Last decision made (if any)
    pub last_decision: Option<PromptDecision>,
    /// Timestamp of last state change
    pub last_state_change: chrono::DateTime<chrono::Utc>,
    /// Count of consecutive detections (for debouncing)
    pub detection_count: u32,
}

impl Default for TerminalPromptStateMachine {
    fn default() -> Self {
        Self {
            state: PromptState::Idle,
            last_prompt: None,
            last_decision: None,
            last_state_change: chrono::Utc::now(),
            detection_count: 0,
        }
    }
}

impl TerminalPromptStateMachine {
    /// Create a new state machine
    pub fn new() -> Self {
        Self::default()
    }

    /// Check if a new prompt should be processed (debouncing)
    pub fn should_process(&self, prompt: &DetectedPrompt) -> bool {
        match self.state {
            PromptState::Idle => true,
            PromptState::Detected | PromptState::Deciding | PromptState::Responding => {
                match self.last_prompt.as_ref() {
                    // Missing last prompt in non-idle state: recover by processing.
                    None => true,
                    // Different prompt kind or text: process immediately.
                    Some(last) if last.kind != prompt.kind || last.raw_text != prompt.raw_text => {
                        true
                    }
                    // Same prompt: only retry after a guard window.
                    Some(_) => self.same_prompt_retry_window_elapsed(),
                }
            }
            PromptState::WaitingForApproval => false, // Never auto-process while waiting for user
        }
    }

    fn same_prompt_retry_window_elapsed(&self) -> bool {
        chrono::Utc::now() - self.last_state_change
            >= chrono::Duration::seconds(SAME_PROMPT_RETRY_WINDOW_SECS)
    }

    /// Transition to detected state
    pub fn on_prompt_detected(&mut self, prompt: DetectedPrompt) {
        self.state = PromptState::Detected;
        self.last_prompt = Some(prompt);
        self.last_state_change = chrono::Utc::now();
        self.detection_count += 1;
    }

    /// Transition to deciding state
    pub fn on_deciding(&mut self) {
        self.state = PromptState::Deciding;
        self.last_state_change = chrono::Utc::now();
    }

    /// Transition to responding state
    pub fn on_response_sent(&mut self, decision: PromptDecision) {
        self.state = PromptState::Responding;
        self.last_decision = Some(decision);
        self.last_state_change = chrono::Utc::now();
    }

    /// Transition to waiting for approval state
    pub fn on_waiting_for_approval(&mut self, decision: PromptDecision) {
        self.state = PromptState::WaitingForApproval;
        self.last_decision = Some(decision);
        self.last_state_change = chrono::Utc::now();
    }

    /// Reset to idle state (after response processed or timeout)
    pub fn reset(&mut self) {
        self.state = PromptState::Idle;
        self.last_prompt = None;
        self.last_decision = None;
        self.last_state_change = chrono::Utc::now();
        self.detection_count = 0;
    }

    /// Check if state machine is stale (no activity for given duration)
    pub fn is_stale(&self, timeout: chrono::Duration) -> bool {
        chrono::Utc::now() - self.last_state_change > timeout
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_prompt(kind: PromptKind, text: &str) -> DetectedPrompt {
        DetectedPrompt::new(kind, text.to_string(), 0.95)
    }

    #[test]
    fn same_prompt_blocked_inside_retry_window() {
        let prompt = create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue");
        let mut sm = TerminalPromptStateMachine::new();

        sm.on_prompt_detected(prompt.clone());
        sm.on_response_sent(PromptDecision::auto_enter());
        sm.last_state_change = chrono::Utc::now() - chrono::Duration::seconds(1);

        assert!(
            !sm.should_process(&prompt),
            "same prompt should be blocked before retry window expires"
        );
    }

    #[test]
    fn same_prompt_retried_after_retry_window_in_responding() {
        let prompt = create_test_prompt(PromptKind::EnterConfirm, "Press Enter to continue");
        let mut sm = TerminalPromptStateMachine::new();

        sm.on_prompt_detected(prompt.clone());
        sm.on_response_sent(PromptDecision::auto_enter());
        sm.last_state_change =
            chrono::Utc::now() - chrono::Duration::seconds(SAME_PROMPT_RETRY_WINDOW_SECS + 1);

        assert!(
            sm.should_process(&prompt),
            "same prompt should be retried after retry window in responding state"
        );
    }

    #[test]
    fn same_prompt_retried_after_retry_window_in_detected() {
        let prompt = create_test_prompt(PromptKind::YesNo, "Continue? [y/n]");
        let mut sm = TerminalPromptStateMachine::new();

        sm.on_prompt_detected(prompt.clone());
        sm.last_state_change =
            chrono::Utc::now() - chrono::Duration::seconds(SAME_PROMPT_RETRY_WINDOW_SECS + 1);

        assert!(
            sm.should_process(&prompt),
            "same prompt should be retried after retry window in detected state"
        );
    }

    #[test]
    fn waiting_for_approval_still_blocks_reprocessing() {
        let prompt = create_test_prompt(PromptKind::Password, "Password:");
        let mut sm = TerminalPromptStateMachine::new();

        sm.on_prompt_detected(prompt.clone());
        sm.on_waiting_for_approval(PromptDecision::ask_password());
        sm.last_state_change =
            chrono::Utc::now() - chrono::Duration::seconds(SAME_PROMPT_RETRY_WINDOW_SECS + 10);

        assert!(
            !sm.should_process(&prompt),
            "waiting-for-approval must continue to block auto processing"
        );
    }
}
