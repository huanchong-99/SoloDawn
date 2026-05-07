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
    CompleteTask { task_id: String, summary: String },
    /// 标记工作流规划完成，后续不再增加任务/终端
    SetWorkflowPlanningComplete { summary: Option<String> },
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
    /// 质量门检查点 — 终端提交了代码但需要先通过质量门再标记完成
    Checkpoint,
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
    // W2-20-10: serde `rename_all = "snake_case"` would produce
    // `l_l_m_decision` for this variant name. Pin the wire value explicitly
    // so the JSON tag matches the other variants and the frontend contract.
    #[serde(rename = "llm_decision")]
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
            response: format!("{arrow_sequence}\n"),
            reasoning,
            target_index: Some(target_index),
        }
    }

    /// Create an LLM decision for Choice prompts
    pub fn llm_choice(choice: &str, reasoning: String) -> Self {
        Self::LLMDecision {
            response: format!("{choice}\n"),
            reasoning,
            target_index: None,
        }
    }

    /// Create an LLM decision for Input prompts
    pub fn llm_input(input: &str, reasoning: String) -> Self {
        Self::LLMDecision {
            response: format!("{input}\n"),
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
#[derive(Default)]
pub enum PromptState {
    /// No prompt detected
    #[default]
    Idle,
    /// Prompt detected, waiting for decision
    Detected,
    /// Decision made, waiting for response to be sent
    Deciding,
    /// Response sent, waiting for terminal to process
    /// [G07-002] TODO: Add a timeout mechanism so that the state machine auto-resets
    /// to Idle if the terminal does not acknowledge the response within a configurable
    /// duration (e.g., 30s). Currently relies on `check_and_reset_stale()` in PromptWatcher.
    Responding,
    /// Waiting for user approval (Password prompts)
    /// [G07-008] TODO: Add a configurable timeout (e.g., 5 minutes) for user approval.
    /// If the user does not respond within the timeout, auto-reset to Idle and log a
    /// warning. This prevents terminals from being stuck indefinitely waiting for input.
    WaitingForApproval,
}

/// Retry window for re-processing an identical prompt while mid-flight.
///
/// This prevents long-term suppression when prompt state gets stuck in
/// `Detected/Deciding/Responding`, while still avoiding duplicate storms.
const SAME_PROMPT_RETRY_WINDOW_SECS: i64 = 5;

/// Timeout for WaitingForApproval state: if the user does not respond within
/// this duration, the state machine auto-resets to Idle so the terminal is
/// not stuck indefinitely waiting for input. (G07-008)
const WAITING_FOR_APPROVAL_TIMEOUT_SECS: i64 = 300; // 5 minutes

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
            PromptState::WaitingForApproval => {
                // G07-008: Auto-reset after timeout so terminals don't hang forever.
                let timed_out =
                    self.is_stale(chrono::Duration::seconds(WAITING_FOR_APPROVAL_TIMEOUT_SECS));
                if timed_out {
                    tracing::warn!(
                        "WaitingForApproval timed out after {}s, allowing prompt re-processing",
                        WAITING_FOR_APPROVAL_TIMEOUT_SECS
                    );
                }
                timed_out
            }
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

/// Context from a previous terminal in the same task, for handoff to the next terminal
#[derive(Debug, Clone, Default)]
pub struct PreviousTerminalContext {
    /// Role of the previous terminal
    pub role: String,
    /// Completion status (completed/failed)
    pub status: String,
    /// Last commit message from the previous terminal, truncated
    pub commit_message: String,
    /// Handoff notes extracted from commit body, truncated
    pub handoff_notes: String,
}

/// Context collected from a completed terminal for LLM prompt enrichment
#[derive(Debug, Clone, Default)]
pub struct TerminalCompletionContext {
    /// Last N lines of terminal log, truncated to max chars
    pub log_summary: String,
    /// git diff --stat output, truncated to max chars
    pub diff_stat: String,
    /// Full commit message body, truncated to max chars
    pub commit_body: String,
    /// Actual content of changed files (collected via `git show`), truncated
    pub changed_files_content: String,
}

/// Result of an LLM-powered acceptance review that compares code against requirements
#[derive(Debug, Clone)]
pub struct AcceptanceReviewResult {
    pub verdict: AcceptanceVerdict,
    pub fix_instructions: String,
    /// Numerical score from the scoring-based audit (None for legacy binary reviews).
    pub score: Option<f64>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AcceptanceVerdict {
    Approved,
    Rejected,
}

impl AcceptanceReviewResult {
    pub fn approved() -> Self {
        Self {
            verdict: AcceptanceVerdict::Approved,
            fix_instructions: String::new(),
            score: None,
        }
    }

    pub fn rejected(fix_instructions: impl Into<String>) -> Self {
        Self {
            verdict: AcceptanceVerdict::Rejected,
            fix_instructions: fix_instructions.into(),
            score: None,
        }
    }

    pub fn parse(response: &str) -> Self {
        let trimmed = response.trim();
        // Try to extract JSON from the response
        let json_str = if let Some(start) = trimmed.find('{') {
            if let Some(end) = trimmed.rfind('}') {
                &trimmed[start..=end]
            } else {
                trimmed
            }
        } else {
            trimmed
        };

        if let Ok(value) = serde_json::from_str::<serde_json::Value>(json_str) {
            let Some(verdict_str) = value.get("verdict").and_then(|v| v.as_str()) else {
                return Self::rejected(
                    "Acceptance review response was invalid: missing `verdict` field.",
                );
            };
            let verdict = if verdict_str.eq_ignore_ascii_case("REJECTED") {
                AcceptanceVerdict::Rejected
            } else if verdict_str.eq_ignore_ascii_case("APPROVED") {
                AcceptanceVerdict::Approved
            } else {
                return Self::rejected(format!(
                    "Acceptance review response was invalid: unsupported verdict `{verdict_str}`."
                ));
            };
            let fix_instructions = value
                .get("fix_instructions")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();

            Self {
                verdict,
                fix_instructions,
                score: None,
            }
        } else {
            // Fallback: check for REJECTED keyword in raw text
            if trimmed.to_uppercase().contains("REJECTED") {
                Self::rejected(trimmed.to_string())
            } else {
                Self::rejected(
                    "Acceptance review response was not valid JSON and did not contain an explicit REJECTED verdict.",
                )
            }
        }
    }
}

// ============================================================================
// Scoring-Based Audit Types
// ============================================================================

/// Five-dimension scoring result from the audit LLM
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuditScoreResult {
    pub total_score: f64,
    pub passed: bool,
    pub dimensions: AuditDimensions,
    pub fix_instructions: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuditDimensions {
    pub buildability: DimensionScore,
    pub functional_completeness: DimensionScore,
    pub code_quality: CodeQualityScore,
    pub test_quality: DimensionScore,
    pub engineering_docs: DimensionScore,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DimensionScore {
    pub score: f64,
    pub max_score: f64,
    pub details: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CodeQualityScore {
    pub architecture: DimensionScore,
    pub standards: DimensionScore,
    pub security: DimensionScore,
    pub total: f64,
    pub max_score: f64,
}

/// Audit plan stored in DB (generated at confirm time)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuditPlan {
    pub mode: AuditMode,
    pub dimensions: Vec<AuditDimensionSpec>,
    pub pass_threshold: f64,
    pub generated_at: String,
    /// Complete scoring rubric text for direct inclusion in LLM prompts.
    /// Mode A: BUILTIN_AUDIT_PRINCIPLES constant.
    /// Mode B: LLM-merged text (built-in + user doc).
    /// Mode C: User doc content only.
    pub raw_principles: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum AuditMode {
    Builtin,
    Merged,
    Custom,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuditDimensionSpec {
    pub name: String,
    pub name_zh: String,
    pub max_score: f64,
    pub criteria: Vec<String>,
    pub sub_dimensions: Option<Vec<AuditDimensionSpec>>,
}

impl AuditScoreResult {
    pub const PASS_THRESHOLD: f64 = 90.0;

    /// Parse an LLM response into an `AuditScoreResult`.
    ///
    /// Uses the same JSON extraction pattern as `AcceptanceReviewResult::parse()`:
    /// find the outermost `{...}` block, deserialize, compute totals, set pass/fail.
    /// On parse failure: return a failing score with the error in `fix_instructions`.
    pub fn parse(response: &str) -> Self {
        let trimmed = response.trim();
        // Extract JSON block from response
        let json_str = if let Some(start) = trimmed.find('{') {
            if let Some(end) = trimmed.rfind('}') {
                &trimmed[start..=end]
            } else {
                trimmed
            }
        } else {
            return Self::parse_failure(
                "Audit score response did not contain a JSON object.",
            );
        };

        let value: serde_json::Value = match serde_json::from_str(json_str) {
            Ok(v) => v,
            Err(e) => {
                return Self::parse_failure(&format!(
                    "Failed to parse audit score JSON: {e}"
                ));
            }
        };

        // Try to deserialize the dimensions
        let dimensions = match Self::extract_dimensions(&value) {
            Ok(d) => d,
            Err(e) => {
                return Self::parse_failure(&format!(
                    "Failed to extract audit dimensions: {e}"
                ));
            }
        };

        let fix_instructions = value
            .get("fix_instructions")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();

        // Calculate total score from dimension scores
        let total_score = dimensions.buildability.score
            + dimensions.functional_completeness.score
            + dimensions.code_quality.total
            + dimensions.test_quality.score
            + dimensions.engineering_docs.score;

        let passed = total_score >= Self::PASS_THRESHOLD;

        Self {
            total_score,
            passed,
            dimensions,
            fix_instructions,
        }
    }

    /// Convert to `AcceptanceReviewResult` for backward compatibility.
    pub fn to_acceptance_result(&self) -> AcceptanceReviewResult {
        if self.passed {
            let mut r = AcceptanceReviewResult::approved();
            r.score = Some(self.total_score);
            r
        } else {
            let mut r = AcceptanceReviewResult::rejected(self.format_rejection());
            r.score = Some(self.total_score);
            r
        }
    }

    fn format_rejection(&self) -> String {
        let d = &self.dimensions;
        let cq = &d.code_quality;
        format!(
            "Score: {:.0}/100\n\n\
             Dimension Breakdown:\n  \
             可构建性:     {:.0}/{:.0} — {}\n  \
             功能完整性:   {:.0}/{:.0} — {}\n  \
             代码质量:     {:.0}/{:.0} (架构 {:.0}/{:.0}, 规范 {:.0}/{:.0}, 安全 {:.0}/{:.0})\n  \
             测试质量:     {:.0}/{:.0} — {}\n  \
             工程化文档:   {:.0}/{:.0} — {}\n\n\
             {}",
            self.total_score,
            d.buildability.score, d.buildability.max_score, d.buildability.details,
            d.functional_completeness.score, d.functional_completeness.max_score, d.functional_completeness.details,
            cq.total, cq.max_score,
            cq.architecture.score, cq.architecture.max_score,
            cq.standards.score, cq.standards.max_score,
            cq.security.score, cq.security.max_score,
            d.test_quality.score, d.test_quality.max_score, d.test_quality.details,
            d.engineering_docs.score, d.engineering_docs.max_score, d.engineering_docs.details,
            self.fix_instructions,
        )
    }

    fn extract_dimensions(value: &serde_json::Value) -> Result<AuditDimensions, String> {
        let dims = value.get("dimensions").ok_or("missing `dimensions` field")?;

        let buildability = Self::extract_dimension_score(dims, "buildability")?;
        let functional_completeness =
            Self::extract_dimension_score(dims, "functional_completeness")?;
        let test_quality = Self::extract_dimension_score(dims, "test_quality")?;
        let engineering_docs = Self::extract_dimension_score(dims, "engineering_docs")?;

        let cq = dims
            .get("code_quality")
            .ok_or("missing `dimensions.code_quality`")?;
        let architecture = Self::extract_dimension_score(cq, "architecture")?;
        let standards = Self::extract_dimension_score(cq, "standards")?;
        let security = Self::extract_dimension_score(cq, "security")?;
        let cq_total = architecture.score + standards.score + security.score;
        let cq_max = architecture.max_score + standards.max_score + security.max_score;

        Ok(AuditDimensions {
            buildability,
            functional_completeness,
            code_quality: CodeQualityScore {
                architecture,
                standards,
                security,
                total: cq_total,
                max_score: cq_max,
            },
            test_quality,
            engineering_docs,
        })
    }

    fn extract_dimension_score(
        parent: &serde_json::Value,
        key: &str,
    ) -> Result<DimensionScore, String> {
        let obj = parent
            .get(key)
            .ok_or_else(|| format!("missing `{key}`"))?;
        let score = obj
            .get("score")
            .and_then(|v| v.as_f64())
            .ok_or_else(|| format!("`{key}.score` is missing or not a number"))?;
        let max_score = obj
            .get("max_score")
            .and_then(|v| v.as_f64())
            .ok_or_else(|| format!("`{key}.max_score` is missing or not a number"))?;
        let details = obj
            .get("details")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        Ok(DimensionScore {
            score,
            max_score,
            details,
        })
    }

    fn parse_failure(reason: &str) -> Self {
        Self {
            total_score: 0.0,
            passed: false,
            dimensions: AuditDimensions {
                buildability: DimensionScore { score: 0.0, max_score: 20.0, details: String::new() },
                functional_completeness: DimensionScore { score: 0.0, max_score: 25.0, details: String::new() },
                code_quality: CodeQualityScore {
                    architecture: DimensionScore { score: 0.0, max_score: 10.0, details: String::new() },
                    standards: DimensionScore { score: 0.0, max_score: 10.0, details: String::new() },
                    security: DimensionScore { score: 0.0, max_score: 10.0, details: String::new() },
                    total: 0.0,
                    max_score: 30.0,
                },
                test_quality: DimensionScore { score: 0.0, max_score: 15.0, details: String::new() },
                engineering_docs: DimensionScore { score: 0.0, max_score: 10.0, details: String::new() },
            },
            fix_instructions: reason.to_string(),
        }
    }
}

/// Result of a quality gate evaluation for a terminal checkpoint
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QualityGateResultEvent {
    pub workflow_id: String,
    pub task_id: String,
    pub terminal_id: String,
    pub quality_run_id: String,
    pub commit_hash: Option<String>,
    /// ok | warn | error
    pub gate_status: String,
    /// off | shadow | warn | enforce
    pub mode: String,
    pub total_issues: i32,
    pub blocking_issues: i32,
    pub new_issues: i32,
    /// Whether the gate passed (gate_status is ok or warn, or mode is shadow)
    pub passed: bool,
    /// Human-readable summary for LLM prompt
    pub summary: String,
    /// Fix instructions for terminals that failed quality gate
    pub fix_instructions: Option<String>,
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

    #[test]
    fn acceptance_review_parse_approves_explicit_json_only() {
        let parsed =
            AcceptanceReviewResult::parse(r#"{"verdict":"APPROVED","fix_instructions":""}"#);

        assert_eq!(parsed.verdict, AcceptanceVerdict::Approved);
    }

    #[test]
    fn acceptance_review_parse_rejects_invalid_json_without_fail_open() {
        let parsed = AcceptanceReviewResult::parse("Looks fine to me.");

        assert_eq!(parsed.verdict, AcceptanceVerdict::Rejected);
        assert!(parsed.fix_instructions.contains("not valid JSON"));
    }

    #[test]
    fn acceptance_review_parse_rejects_missing_or_unknown_verdict() {
        let missing = AcceptanceReviewResult::parse(r#"{"fix_instructions":""}"#);
        let unknown = AcceptanceReviewResult::parse(r#"{"verdict":"MAYBE","fix_instructions":""}"#);

        assert_eq!(missing.verdict, AcceptanceVerdict::Rejected);
        assert_eq!(unknown.verdict, AcceptanceVerdict::Rejected);
    }

    // =====================================================================
    // AuditScoreResult::parse() tests
    // =====================================================================

    fn make_audit_json(total: f64) -> String {
        // Distribute scores roughly proportionally to max
        let buildability = (total * 0.20).min(20.0);
        let functional = (total * 0.25).min(25.0);
        let arch = (total * 0.10).min(10.0);
        let standards = (total * 0.10).min(10.0);
        let security = (total * 0.10).min(10.0);
        let test = (total * 0.15).min(15.0);
        let docs = (total * 0.10).min(10.0);
        format!(
            r#"{{
                "total_score": {total},
                "dimensions": {{
                    "buildability": {{"score": {buildability}, "max_score": 20, "details": "ok"}},
                    "functional_completeness": {{"score": {functional}, "max_score": 25, "details": "ok"}},
                    "code_quality": {{
                        "architecture": {{"score": {arch}, "max_score": 10, "details": "ok"}},
                        "standards": {{"score": {standards}, "max_score": 10, "details": "ok"}},
                        "security": {{"score": {security}, "max_score": 10, "details": "ok"}}
                    }},
                    "test_quality": {{"score": {test}, "max_score": 15, "details": "ok"}},
                    "engineering_docs": {{"score": {docs}, "max_score": 10, "details": "ok"}}
                }},
                "fix_instructions": ""
            }}"#,
        )
    }

    #[test]
    fn audit_score_parse_passing_score() {
        let json = make_audit_json(95.0);
        let result = AuditScoreResult::parse(&json);

        assert!(result.passed, "score 95 should pass (threshold 90)");
        assert!(result.total_score >= 90.0);
    }

    #[test]
    fn audit_score_parse_failing_score() {
        let json = make_audit_json(75.0);
        let result = AuditScoreResult::parse(&json);

        assert!(!result.passed, "score 75 should fail (threshold 90)");
        assert!(result.total_score < 90.0);
    }

    #[test]
    fn audit_score_parse_invalid_json_returns_zero() {
        let result = AuditScoreResult::parse("This is not JSON at all.");

        assert!(!result.passed);
        assert_eq!(result.total_score, 0.0);
        assert!(result.fix_instructions.contains("did not contain a JSON object"));
    }

    #[test]
    fn audit_score_parse_malformed_json_returns_zero() {
        let result = AuditScoreResult::parse(r#"{"dimensions": "bad"}"#);

        assert!(!result.passed);
        assert_eq!(result.total_score, 0.0);
        assert!(result.fix_instructions.contains("Failed to extract"));
    }

    #[test]
    fn audit_score_to_acceptance_result_approved() {
        let json = make_audit_json(95.0);
        let result = AuditScoreResult::parse(&json);
        let acceptance = result.to_acceptance_result();

        assert_eq!(acceptance.verdict, AcceptanceVerdict::Approved);
        assert!(acceptance.score.is_some());
        assert!(acceptance.score.unwrap() >= 90.0);
    }

    #[test]
    fn audit_score_to_acceptance_result_rejected() {
        let json = make_audit_json(70.0);
        let result = AuditScoreResult::parse(&json);
        let acceptance = result.to_acceptance_result();

        assert_eq!(acceptance.verdict, AcceptanceVerdict::Rejected);
        assert!(acceptance.score.is_some());
        assert!(acceptance.score.unwrap() < 90.0);
        assert!(acceptance.fix_instructions.contains("Score:"));
    }

    #[test]
    fn audit_score_format_rejection_includes_dimensions() {
        let json = make_audit_json(70.0);
        let result = AuditScoreResult::parse(&json);
        let rejection = result.format_rejection();

        assert!(rejection.contains("可构建性"));
        assert!(rejection.contains("功能完整性"));
        assert!(rejection.contains("代码质量"));
        assert!(rejection.contains("测试质量"));
        assert!(rejection.contains("工程化文档"));
    }

    #[test]
    fn audit_score_parse_extracts_json_from_surrounding_text() {
        let response = format!(
            "Here is my audit review:\n\n{}\n\nEnd of review.",
            make_audit_json(92.0)
        );
        let result = AuditScoreResult::parse(&response);

        assert!(result.passed);
        assert!(result.total_score >= 90.0);
    }
}
