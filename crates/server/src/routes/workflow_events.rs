//! WebSocket event model for workflow event broadcasting.
//!
//! Defines the event structure and types for real-time workflow updates.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use services::services::orchestrator::{
    BusMessage, ProviderEvent,
    types::{PromptDecision, PromptKind, QualityGateResultEvent, TerminalCompletionStatus},
};
use ts_rs::TS;
use uuid::Uuid;

// ============================================================================
// Event Types
// ============================================================================

/// WebSocket event types following namespace convention.
///
/// Format: `{category}.{action}` (e.g., `workflow.status_changed`)
#[derive(Debug, Clone, Copy, Serialize, Deserialize, Eq, PartialEq, TS)]
#[serde(rename_all = "snake_case")]
pub enum WsEventType {
    /// Workflow status changed (ready -> running -> completed/failed)
    #[serde(rename = "workflow.status_changed")]
    WorkflowStatusChanged,

    /// Terminal status changed (waiting -> working -> completed/failed)
    #[serde(rename = "terminal.status_changed")]
    TerminalStatusChanged,

    /// Terminal quality gate result
    #[serde(rename = "terminal.quality_gate_result")]
    TerminalQualityGateResult,

    /// Task status changed (pending -> running -> completed/failed)
    #[serde(rename = "task.status_changed")]
    TaskStatusChanged,

    /// Terminal completed with result
    #[serde(rename = "terminal.completed")]
    TerminalCompleted,

    /// Git commit detected in repository
    #[serde(rename = "git.commit_detected")]
    GitCommitDetected,

    /// Orchestrator awakened and processing
    #[serde(rename = "orchestrator.awakened")]
    OrchestratorAwakened,

    /// Orchestrator made a decision
    #[serde(rename = "orchestrator.decision")]
    OrchestratorDecision,

    /// System heartbeat for connection keep-alive
    #[serde(rename = "system.heartbeat")]
    SystemHeartbeat,

    /// Receiver lagged and missed messages
    #[serde(rename = "system.lagged")]
    SystemLagged,

    /// System error occurred
    #[serde(rename = "system.error")]
    SystemError,

    /// Terminal prompt detected (interactive prompt requiring response)
    #[serde(rename = "terminal.prompt_detected")]
    TerminalPromptDetected,

    /// Terminal prompt decision made by Orchestrator
    #[serde(rename = "terminal.prompt_decision")]
    TerminalPromptDecision,

    /// Provider switched to a different one after failure
    #[serde(rename = "provider.switched")]
    ProviderSwitched,

    /// All providers exhausted (all dead or failed)
    #[serde(rename = "provider.exhausted")]
    ProviderExhausted,

    /// A previously dead provider recovered via probe
    #[serde(rename = "provider.recovered")]
    ProviderRecovered,

    /// Quality gate result for a terminal checkpoint
    #[serde(rename = "quality.gate_result")]
    QualityGateResult,
}

// ============================================================================
// Event Structure
// ============================================================================

/// WebSocket event structure following design specification.
///
/// All events follow the format: `{type, payload, timestamp, id}`
#[derive(Debug, Clone, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct WsEvent {
    /// Event type (e.g., "workflow.status_changed")
    #[serde(rename = "type")]
    pub event_type: WsEventType,

    /// Event payload (varies by event type)
    pub payload: Value,

    /// ISO 8601 timestamp when event was created
    pub timestamp: DateTime<Utc>,

    /// Unique event identifier for deduplication
    pub id: String,
}

fn terminal_completion_status_to_wire(status: TerminalCompletionStatus) -> &'static str {
    match status {
        TerminalCompletionStatus::Completed => "completed",
        TerminalCompletionStatus::ReviewPass => "review_pass",
        TerminalCompletionStatus::ReviewReject => "review_reject",
        TerminalCompletionStatus::Failed => "failed",
        TerminalCompletionStatus::Checkpoint => "checkpoint",
    }
}

fn prompt_kind_to_wire(prompt_kind: PromptKind) -> &'static str {
    match prompt_kind {
        PromptKind::EnterConfirm => "enter_confirm",
        PromptKind::YesNo => "yes_no",
        PromptKind::Choice => "choice",
        PromptKind::ArrowSelect => "arrow_select",
        PromptKind::Input => "input",
        PromptKind::Password => "password",
    }
}

fn prompt_decision_action(decision: &PromptDecision) -> &'static str {
    match decision {
        PromptDecision::AutoConfirm { .. } => "auto_confirm",
        PromptDecision::LLMDecision { .. } => "llm_decision",
        PromptDecision::AskUser { .. } => "ask_user",
        PromptDecision::Skip { .. } => "skip",
    }
}

fn prompt_decision_detail(decision: &PromptDecision) -> Value {
    match decision {
        PromptDecision::AutoConfirm { response, reason } => {
            json!({
                "action": "auto_confirm",
                "response": response,
                "reason": reason
            })
        }
        PromptDecision::LLMDecision {
            response,
            reasoning,
            target_index,
        } => {
            json!({
                "action": "llm_decision",
                "response": response,
                "reasoning": reasoning,
                "targetIndex": target_index,
                "target_index": target_index
            })
        }
        PromptDecision::AskUser {
            reason,
            suggestions,
        } => {
            json!({
                "action": "ask_user",
                "reason": reason,
                "suggestions": suggestions
            })
        }
        PromptDecision::Skip { reason } => {
            json!({
                "action": "skip",
                "reason": reason
            })
        }
    }
}

impl WsEvent {
    /// Create a new WebSocket event with auto-generated ID and timestamp.
    pub fn new(event_type: WsEventType, payload: Value) -> Self {
        Self {
            event_type,
            payload,
            timestamp: Utc::now(),
            id: format!("evt_{}", Uuid::new_v4()),
        }
    }

    /// Create a heartbeat event.
    pub fn heartbeat() -> Self {
        Self::new(WsEventType::SystemHeartbeat, json!({}))
    }

    /// Create a lagged event indicating missed messages.
    pub fn lagged(skipped: u64) -> Self {
        Self::new(WsEventType::SystemLagged, json!({ "skipped": skipped }))
    }

    /// Create an error event.
    pub fn error(message: impl Into<String>) -> Self {
        Self::new(
            WsEventType::SystemError,
            json!({ "message": message.into() }),
        )
    }

    /// Convert a BusMessage into a workflow-scoped WebSocket event.
    ///
    /// Returns `Some((workflow_id, event))` when the message can be routed,
    /// or `None` for messages that don't map to WebSocket events.
    pub fn try_from_bus_message(message: BusMessage) -> Option<(String, Self)> {
        match message {
            BusMessage::StatusUpdate {
                workflow_id,
                status,
            } => {
                let payload = json!({
                    "workflowId": workflow_id,
                    "status": status
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::WorkflowStatusChanged, payload),
                ))
            }

            BusMessage::TerminalStatusUpdate {
                workflow_id,
                terminal_id,
                status,
            } => {
                let payload = json!({
                    "workflowId": workflow_id,
                    "terminalId": terminal_id,
                    "status": status
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::TerminalStatusChanged, payload),
                ))
            }

            BusMessage::TaskStatusUpdate {
                workflow_id,
                task_id,
                status,
            } => {
                let payload = json!({
                    "workflowId": workflow_id,
                    "taskId": task_id,
                    "status": status
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::TaskStatusChanged, payload),
                ))
            }

            BusMessage::TerminalCompleted(event) => {
                let workflow_id = event.workflow_id;
                let task_id = event.task_id;
                let terminal_id = event.terminal_id;
                let status = terminal_completion_status_to_wire(event.status);
                let commit_hash = event.commit_hash;
                let commit_message = event.commit_message;
                let metadata = event.metadata;

                let payload = json!({
                    "workflowId": workflow_id,
                    "taskId": task_id,
                    "terminalId": terminal_id,
                    "status": status,
                    "commitHash": commit_hash,
                    "commitMessage": commit_message,
                    "metadata": metadata,
                    "workflow_id": workflow_id,
                    "task_id": task_id,
                    "terminal_id": terminal_id,
                    "commit_hash": commit_hash,
                    "commit_message": commit_message
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::TerminalCompleted, payload),
                ))
            }

            BusMessage::TerminalQualityGateResult(event) => {
                let workflow_id = event.original_event.workflow_id;
                let payload = json!({
                    "workflowId": workflow_id,
                    "taskId": event.original_event.task_id,
                    "terminalId": event.original_event.terminal_id,
                    "isPassed": event.is_passed,
                    "mode": format!("{:?}", event.mode).to_lowercase(),
                    "fixInstructions": event.fix_instructions,
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::TerminalQualityGateResult, payload),
                ))
            }

            BusMessage::GitEvent {
                workflow_id,
                commit_hash,
                branch,
                message,
            } => {
                let payload = json!({
                    "workflowId": workflow_id,
                    "commitHash": commit_hash,
                    "branch": branch,
                    "message": message
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::GitCommitDetected, payload),
                ))
            }

            BusMessage::Error { workflow_id, error } => {
                let payload = json!({
                    "workflowId": workflow_id,
                    "error": error
                });
                Some((workflow_id, Self::new(WsEventType::SystemError, payload)))
            }

            // Terminal prompt events - forward to WebSocket for UI updates
            BusMessage::TerminalPromptDetected(event) => {
                let workflow_id = event.workflow_id;
                let terminal_id = event.terminal_id;
                let task_id = event.task_id;
                let session_id = event.session_id;
                let prompt_kind = prompt_kind_to_wire(event.prompt.kind);
                let legacy_prompt_kind = format!("{:?}", event.prompt.kind);
                let prompt_text = event.prompt.raw_text;
                let confidence = event.prompt.confidence;
                let has_dangerous_keywords = event.prompt.has_dangerous_keywords;
                let selected_index = event.prompt.selected_index;
                let option_details = event.prompt.options.unwrap_or_default();
                let auto_confirm = event.auto_confirm;
                let detected_at = event.detected_at.to_rfc3339();
                let options: Vec<String> = option_details
                    .iter()
                    .map(|option| option.label.clone())
                    .collect();

                let payload = json!({
                    "workflowId": workflow_id,
                    "terminalId": terminal_id,
                    "taskId": task_id,
                    "sessionId": session_id,
                    "promptKind": prompt_kind,
                    "promptText": prompt_text,
                    "confidence": confidence,
                    "hasDangerousKeywords": has_dangerous_keywords,
                    "autoConfirm": auto_confirm,
                    "detectedAt": detected_at,
                    "options": options,
                    "optionDetails": option_details,
                    "selectedIndex": selected_index,
                    "workflow_id": workflow_id,
                    "terminal_id": terminal_id,
                    "task_id": task_id,
                    "session_id": session_id,
                    "prompt_kind": prompt_kind,
                    "prompt_text": prompt_text,
                    "has_dangerous_keywords": has_dangerous_keywords,
                    "auto_confirm": auto_confirm,
                    "detected_at": detected_at,
                    "selected_index": selected_index,
                    "legacyPromptKind": legacy_prompt_kind
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::TerminalPromptDetected, payload),
                ))
            }

            BusMessage::TerminalPromptDecision {
                terminal_id,
                workflow_id,
                decision,
            } => {
                let decision_action = prompt_decision_action(&decision);
                let decision_detail = prompt_decision_detail(&decision);
                let mut decision_raw = serde_json::to_value(&decision).unwrap_or_else(|_| {
                    json!({
                        "action": decision_action,
                        "error": "serialization_failed"
                    })
                });

                if decision_raw["action"] == "l_l_m_decision" {
                    decision_raw["action"] = json!("llm_decision");
                }

                let payload = json!({
                    "workflowId": workflow_id,
                    "terminalId": terminal_id,
                    "decision": decision_action,
                    "decisionDetail": decision_detail,
                    "decisionRaw": decision_raw,
                    "workflow_id": workflow_id,
                    "terminal_id": terminal_id
                });
                Some((
                    workflow_id,
                    Self::new(WsEventType::TerminalPromptDecision, payload),
                ))
            }

            // Messages that don't map to WebSocket events
            BusMessage::Instruction(_) => None,
            BusMessage::TerminalMessage { .. } => None,
            BusMessage::TerminalInput { .. } => None, // Internal message, not for WebSocket
            BusMessage::Shutdown => None,

            // Quality gate result event
            BusMessage::TerminalQualityGateResult(event) => {
                let payload = json!({
                    "workflowId": event.workflow_id,
                    "taskId": event.task_id,
                    "terminalId": event.terminal_id,
                    "qualityRunId": event.quality_run_id,
                    "commitHash": event.commit_hash,
                    "gateStatus": event.gate_status,
                    "mode": event.mode,
                    "totalIssues": event.total_issues,
                    "blockingIssues": event.blocking_issues,
                    "newIssues": event.new_issues,
                    "passed": event.passed,
                    "summary": event.summary,
                    // snake_case duplicates for legacy compat
                    "workflow_id": event.workflow_id,
                    "task_id": event.task_id,
                    "terminal_id": event.terminal_id,
                    "quality_run_id": event.quality_run_id,
                    "commit_hash": event.commit_hash,
                    "gate_status": event.gate_status,
                    "total_issues": event.total_issues,
                    "blocking_issues": event.blocking_issues,
                    "new_issues": event.new_issues
                });
                Some((event.workflow_id, Self::new(WsEventType::QualityGateResult, payload)))
            }

            // Provider state change events
            BusMessage::ProviderStateChanged {
                workflow_id,
                event,
            } => {
                let (event_type, payload) = match &event {
                    ProviderEvent::Switched {
                        from_provider,
                        to_provider,
                    } => (
                        WsEventType::ProviderSwitched,
                        json!({
                            "workflowId": workflow_id,
                            "fromProvider": from_provider,
                            "toProvider": to_provider,
                            "workflow_id": workflow_id,
                            "from_provider": from_provider,
                            "to_provider": to_provider
                        }),
                    ),
                    ProviderEvent::Exhausted { provider_count } => (
                        WsEventType::ProviderExhausted,
                        json!({
                            "workflowId": workflow_id,
                            "providerCount": provider_count,
                            "workflow_id": workflow_id,
                            "provider_count": provider_count
                        }),
                    ),
                    ProviderEvent::Recovered { provider_name } => (
                        WsEventType::ProviderRecovered,
                        json!({
                            "workflowId": workflow_id,
                            "providerName": provider_name,
                            "workflow_id": workflow_id,
                            "provider_name": provider_name
                        }),
                    ),
                };
                Some((workflow_id, Self::new(event_type, payload)))
            }
        }
    }
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use services::services::orchestrator::types::{
        ArrowSelectOption, DetectedPrompt, PromptDecision, PromptKind, TerminalCompletionEvent,
        TerminalCompletionStatus, TerminalPromptEvent,
    };

    use super::*;

    #[test]
    fn test_ws_event_serialization() {
        let event = WsEvent::new(
            WsEventType::WorkflowStatusChanged,
            json!({"workflowId": "123", "status": "running"}),
        );

        let json = serde_json::to_string(&event).unwrap();

        assert!(json.contains("workflow.status_changed"));
        assert!(json.contains("timestamp"));
        assert!(json.contains("evt_"));
        assert!(json.contains("workflowId"));
    }

    #[test]
    fn test_ws_event_deserialization() {
        let json = r#"{
            "type": "workflow.status_changed",
            "payload": {"workflowId": "123", "status": "running"},
            "timestamp": "2026-02-04T12:00:00Z",
            "id": "evt_test123"
        }"#;

        let event: WsEvent = serde_json::from_str(json).unwrap();

        assert_eq!(event.event_type, WsEventType::WorkflowStatusChanged);
        assert_eq!(event.id, "evt_test123");
    }

    #[test]
    fn test_heartbeat_event() {
        let event = WsEvent::heartbeat();

        assert_eq!(event.event_type, WsEventType::SystemHeartbeat);
        assert!(event.id.starts_with("evt_"));
    }

    #[test]
    fn test_lagged_event() {
        let event = WsEvent::lagged(5);

        assert_eq!(event.event_type, WsEventType::SystemLagged);
        assert_eq!(event.payload["skipped"], 5);
    }

    #[test]
    fn test_error_event() {
        let event = WsEvent::error("Test error message");

        assert_eq!(event.event_type, WsEventType::SystemError);
        assert_eq!(event.payload["message"], "Test error message");
    }

    #[test]
    fn test_bus_message_status_update_conversion() {
        let bus_msg = BusMessage::StatusUpdate {
            workflow_id: "wf-123".to_string(),
            status: "running".to_string(),
        };

        let result = WsEvent::try_from_bus_message(bus_msg);

        assert!(result.is_some());
        let (workflow_id, event) = result.unwrap();
        assert_eq!(workflow_id, "wf-123");
        assert_eq!(event.event_type, WsEventType::WorkflowStatusChanged);
        assert_eq!(event.payload["status"], "running");
    }

    #[test]
    fn test_bus_message_terminal_status_conversion() {
        let bus_msg = BusMessage::TerminalStatusUpdate {
            workflow_id: "wf-123".to_string(),
            terminal_id: "term-456".to_string(),
            status: "working".to_string(),
        };

        let result = WsEvent::try_from_bus_message(bus_msg);

        assert!(result.is_some());
        let (workflow_id, event) = result.unwrap();
        assert_eq!(workflow_id, "wf-123");
        assert_eq!(event.event_type, WsEventType::TerminalStatusChanged);
        assert_eq!(event.payload["terminalId"], "term-456");
    }

    #[test]
    fn test_bus_message_git_event_conversion() {
        let bus_msg = BusMessage::GitEvent {
            workflow_id: "wf-123".to_string(),
            commit_hash: "abc123".to_string(),
            branch: "main".to_string(),
            message: "feat: add feature".to_string(),
        };

        let result = WsEvent::try_from_bus_message(bus_msg);

        assert!(result.is_some());
        let (workflow_id, event) = result.unwrap();
        assert_eq!(workflow_id, "wf-123");
        assert_eq!(event.event_type, WsEventType::GitCommitDetected);
        assert_eq!(event.payload["commitHash"], "abc123");
    }

    #[test]
    fn test_bus_message_terminal_completed_conversion() {
        let bus_msg = BusMessage::TerminalCompleted(TerminalCompletionEvent {
            workflow_id: "wf-123".to_string(),
            task_id: "task-456".to_string(),
            terminal_id: "term-789".to_string(),
            status: TerminalCompletionStatus::Completed,
            commit_hash: Some("abc123".to_string()),
            commit_message: Some("feat: done".to_string()),
            metadata: None,
        });

        let result = WsEvent::try_from_bus_message(bus_msg);

        assert!(result.is_some());
        let (workflow_id, event) = result.unwrap();
        assert_eq!(workflow_id, "wf-123");
        assert_eq!(event.event_type, WsEventType::TerminalCompleted);
        assert_eq!(event.payload["workflowId"], "wf-123");
        assert_eq!(event.payload["taskId"], "task-456");
        assert_eq!(event.payload["terminalId"], "term-789");
        assert_eq!(event.payload["status"], "completed");
        assert_eq!(event.payload["commitHash"], "abc123");
        assert_eq!(event.payload["commitMessage"], "feat: done");
        assert_eq!(event.payload["workflow_id"], "wf-123");
        assert_eq!(event.payload["task_id"], "task-456");
        assert_eq!(event.payload["terminal_id"], "term-789");
        assert_eq!(event.payload["commit_hash"], "abc123");
        assert_eq!(event.payload["commit_message"], "feat: done");
    }

    #[test]
    fn test_bus_message_terminal_prompt_detected_conversion_contract() {
        let bus_msg = BusMessage::TerminalPromptDetected(TerminalPromptEvent {
            terminal_id: "term-789".to_string(),
            workflow_id: "wf-123".to_string(),
            task_id: "task-456".to_string(),
            session_id: "session-111".to_string(),
            auto_confirm: true,
            prompt: DetectedPrompt {
                kind: PromptKind::ArrowSelect,
                raw_text: "Select option".to_string(),
                confidence: 0.92,
                options: Some(vec![
                    ArrowSelectOption {
                        index: 0,
                        label: "Option A".to_string(),
                        selected: false,
                    },
                    ArrowSelectOption {
                        index: 1,
                        label: "Option B".to_string(),
                        selected: true,
                    },
                ]),
                selected_index: Some(1),
                has_dangerous_keywords: false,
            },
            detected_at: Utc::now(),
        });

        let result = WsEvent::try_from_bus_message(bus_msg);

        assert!(result.is_some());
        let (workflow_id, event) = result.unwrap();
        assert_eq!(workflow_id, "wf-123");
        assert_eq!(event.event_type, WsEventType::TerminalPromptDetected);
        assert_eq!(event.payload["workflowId"], "wf-123");
        assert_eq!(event.payload["terminalId"], "term-789");
        assert_eq!(event.payload["taskId"], "task-456");
        assert_eq!(event.payload["sessionId"], "session-111");
        assert_eq!(event.payload["promptKind"], "arrow_select");
        assert_eq!(event.payload["promptText"], "Select option");
        assert_eq!(event.payload["hasDangerousKeywords"], false);
        assert_eq!(event.payload["autoConfirm"], true);
        assert!(event.payload["detectedAt"].is_string());
        assert_eq!(event.payload["selectedIndex"], 1);
        assert_eq!(event.payload["options"], json!(["Option A", "Option B"]));
        assert_eq!(event.payload["optionDetails"][1]["label"], "Option B");
        assert_eq!(event.payload["optionDetails"][1]["selected"], true);
        assert_eq!(event.payload["workflow_id"], "wf-123");
        assert_eq!(event.payload["terminal_id"], "term-789");
        assert_eq!(event.payload["task_id"], "task-456");
        assert_eq!(event.payload["session_id"], "session-111");
        assert_eq!(event.payload["prompt_kind"], "arrow_select");
        assert_eq!(event.payload["prompt_text"], "Select option");
        assert_eq!(event.payload["auto_confirm"], true);
        assert!(event.payload["detected_at"].is_string());
        assert_eq!(event.payload["selected_index"], 1);
        assert_eq!(event.payload["legacyPromptKind"], "ArrowSelect");
    }

    #[test]
    fn test_bus_message_terminal_prompt_decision_conversion_contract() {
        let decision = PromptDecision::LLMDecision {
            response: "y\n".to_string(),
            reasoning: "safe to proceed".to_string(),
            target_index: Some(2),
        };
        let bus_msg = BusMessage::TerminalPromptDecision {
            terminal_id: "term-789".to_string(),
            workflow_id: "wf-123".to_string(),
            decision,
        };

        let result = WsEvent::try_from_bus_message(bus_msg);

        assert!(result.is_some());
        let (workflow_id, event) = result.unwrap();
        assert_eq!(workflow_id, "wf-123");
        assert_eq!(event.event_type, WsEventType::TerminalPromptDecision);
        assert_eq!(event.payload["workflowId"], "wf-123");
        assert_eq!(event.payload["terminalId"], "term-789");
        assert_eq!(event.payload["decision"], "llm_decision");
        assert_eq!(event.payload["decisionDetail"]["action"], "llm_decision");
        assert_eq!(event.payload["decisionDetail"]["response"], "y\n");
        assert_eq!(
            event.payload["decisionDetail"]["reasoning"],
            "safe to proceed"
        );
        assert_eq!(event.payload["decisionDetail"]["targetIndex"], 2);
        assert_eq!(event.payload["decisionDetail"]["target_index"], 2);
        assert_eq!(event.payload["decisionRaw"]["action"], "llm_decision");
        assert_eq!(event.payload["decisionRaw"]["target_index"], 2);
        assert_eq!(event.payload["workflow_id"], "wf-123");
        assert_eq!(event.payload["terminal_id"], "term-789");
    }

    #[test]
    fn test_terminal_ws_contract_dual_case_regression_for_p0_events() {
        let (_, terminal_completed) =
            WsEvent::try_from_bus_message(BusMessage::TerminalCompleted(TerminalCompletionEvent {
                workflow_id: "wf-contract".to_string(),
                task_id: "task-contract".to_string(),
                terminal_id: "term-contract".to_string(),
                status: TerminalCompletionStatus::ReviewPass,
                commit_hash: Some("abc123".to_string()),
                commit_message: Some("feat: contract".to_string()),
                metadata: None,
            }))
            .expect("terminal.completed should be converted");

        assert_eq!(
            terminal_completed.event_type,
            WsEventType::TerminalCompleted
        );
        assert_eq!(
            terminal_completed.payload["workflowId"],
            terminal_completed.payload["workflow_id"]
        );
        assert_eq!(
            terminal_completed.payload["taskId"],
            terminal_completed.payload["task_id"]
        );
        assert_eq!(
            terminal_completed.payload["terminalId"],
            terminal_completed.payload["terminal_id"]
        );
        assert_eq!(
            terminal_completed.payload["commitHash"],
            terminal_completed.payload["commit_hash"]
        );
        assert_eq!(
            terminal_completed.payload["commitMessage"],
            terminal_completed.payload["commit_message"]
        );
        assert_eq!(terminal_completed.payload["status"], "review_pass");

        let (_, prompt_detected) = WsEvent::try_from_bus_message(
            BusMessage::TerminalPromptDetected(TerminalPromptEvent {
                terminal_id: "term-contract".to_string(),
                workflow_id: "wf-contract".to_string(),
                task_id: "task-contract".to_string(),
                session_id: "session-contract".to_string(),
                auto_confirm: true,
                prompt: DetectedPrompt {
                    kind: PromptKind::ArrowSelect,
                    raw_text: "Select contract option".to_string(),
                    confidence: 0.95,
                    options: Some(vec![
                        ArrowSelectOption {
                            index: 0,
                            label: "Option A".to_string(),
                            selected: false,
                        },
                        ArrowSelectOption {
                            index: 1,
                            label: "Option B".to_string(),
                            selected: true,
                        },
                    ]),
                    selected_index: Some(1),
                    has_dangerous_keywords: false,
                },
                detected_at: Utc::now(),
            }),
        )
        .expect("terminal.prompt_detected should be converted");

        assert_eq!(
            prompt_detected.event_type,
            WsEventType::TerminalPromptDetected
        );
        assert_eq!(
            prompt_detected.payload["workflowId"],
            prompt_detected.payload["workflow_id"]
        );
        assert_eq!(
            prompt_detected.payload["terminalId"],
            prompt_detected.payload["terminal_id"]
        );
        assert_eq!(
            prompt_detected.payload["taskId"],
            prompt_detected.payload["task_id"]
        );
        assert_eq!(
            prompt_detected.payload["sessionId"],
            prompt_detected.payload["session_id"]
        );
        assert_eq!(
            prompt_detected.payload["promptKind"],
            prompt_detected.payload["prompt_kind"]
        );
        assert_eq!(
            prompt_detected.payload["promptText"],
            prompt_detected.payload["prompt_text"]
        );
        assert_eq!(
            prompt_detected.payload["autoConfirm"],
            prompt_detected.payload["auto_confirm"]
        );
        assert_eq!(
            prompt_detected.payload["detectedAt"],
            prompt_detected.payload["detected_at"]
        );
        assert_eq!(
            prompt_detected.payload["selectedIndex"],
            prompt_detected.payload["selected_index"]
        );
        assert_eq!(
            prompt_detected.payload["options"],
            json!(["Option A", "Option B"])
        );
        assert_eq!(
            prompt_detected.payload["optionDetails"][1]["selected"],
            true
        );

        let (_, prompt_decision) =
            WsEvent::try_from_bus_message(BusMessage::TerminalPromptDecision {
                terminal_id: "term-contract".to_string(),
                workflow_id: "wf-contract".to_string(),
                decision: PromptDecision::LLMDecision {
                    response: "y\n".to_string(),
                    reasoning: "contract decision".to_string(),
                    target_index: Some(1),
                },
            })
            .expect("terminal.prompt_decision should be converted");

        assert_eq!(
            prompt_decision.event_type,
            WsEventType::TerminalPromptDecision
        );
        assert_eq!(
            prompt_decision.payload["workflowId"],
            prompt_decision.payload["workflow_id"]
        );
        assert_eq!(
            prompt_decision.payload["terminalId"],
            prompt_decision.payload["terminal_id"]
        );
        assert_eq!(prompt_decision.payload["decision"], "llm_decision");
        assert_eq!(
            prompt_decision.payload["decisionDetail"]["targetIndex"],
            prompt_decision.payload["decisionDetail"]["target_index"]
        );
        assert_eq!(
            prompt_decision.payload["decisionRaw"]["action"],
            "llm_decision"
        );
    }

    #[test]
    fn test_bus_message_shutdown_not_converted() {
        let bus_msg = BusMessage::Shutdown;
        let result = WsEvent::try_from_bus_message(bus_msg);
        assert!(result.is_none());
    }

    #[test]
    fn test_bus_message_terminal_message_not_converted() {
        let bus_msg = BusMessage::TerminalMessage {
            message: "test".to_string(),
        };
        let result = WsEvent::try_from_bus_message(bus_msg);
        assert!(result.is_none());
    }

    #[test]
    fn test_all_event_types_serialize() {
        let types = vec![
            WsEventType::WorkflowStatusChanged,
            WsEventType::TerminalStatusChanged,
            WsEventType::TaskStatusChanged,
            WsEventType::TerminalCompleted,
            WsEventType::GitCommitDetected,
            WsEventType::OrchestratorAwakened,
            WsEventType::OrchestratorDecision,
            WsEventType::SystemHeartbeat,
            WsEventType::SystemLagged,
            WsEventType::SystemError,
        ];

        for event_type in types {
            let event = WsEvent::new(event_type, json!({}));
            let json = serde_json::to_string(&event).unwrap();
            assert!(
                json.contains("."),
                "Event type should contain dot: {:?}",
                event_type
            );
        }
    }
}
