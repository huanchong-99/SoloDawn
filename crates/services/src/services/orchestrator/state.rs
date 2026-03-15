//! Orchestrator state tracking and transitions.

use std::collections::{HashMap, HashSet, VecDeque};

use tokio::sync::RwLock;

use super::{
    config::OrchestratorConfig,
    types::{LLMMessage, TerminalCompletionEvent},
};

/// Execution state for the orchestrator event loop.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OrchestratorRunState {
    /// Idle and waiting for events.
    Idle,
    /// Processing events or instructions.
    Processing,
    /// Paused pending user or system action.
    Paused,
    /// Stopped and no longer processing.
    Stopped,
}

/// Tracks per-task terminal execution progress.
#[derive(Debug, Clone)]
pub struct TaskExecutionState {
    pub task_id: String,
    pub current_terminal_index: usize,
    pub total_terminals: usize,
    pub terminal_ids: Vec<String>,
    pub completed_terminals: Vec<String>,
    pub failed_terminals: Vec<String>,
    pub planning_complete: bool,
    pub is_completed: bool,
}

/// In-memory orchestrator state for a workflow.
pub struct OrchestratorState {
    /// Current run state for the event loop.
    pub run_state: OrchestratorRunState,

    /// Workflow identifier.
    pub workflow_id: String,

    /// Per-task execution state.
    pub task_states: HashMap<String, TaskExecutionState>,

    /// Whether the workflow planner has declared the execution graph complete.
    pub workflow_planning_complete: bool,

    /// Conversation history for LLM context.
    pub conversation_history: Vec<LLMMessage>,

    /// Pending terminal completion events.
    pub pending_events: Vec<TerminalCompletionEvent>,

    /// Total tokens consumed by the LLM.
    pub total_tokens_used: i64,

    /// Total error count for this workflow run.
    pub error_count: u32,

    /// Set of processed commit hashes for idempotency.
    ///
    /// Bounded to `MAX_PROCESSED_COMMITS` entries. When the limit is reached,
    /// the oldest half is evicted via `record_processed_commit()`.
    pub processed_commits: HashSet<String>,

    /// Insertion-order tracking for `processed_commits` eviction.
    processed_commits_order: VecDeque<String>,

    /// Terminals currently waiting for quiet-window completion checks.
    pub pending_quiet_completion_checks: HashSet<String>,

    /// Terminals currently waiting for quality gate evaluation.
    pub pending_quality_checks: HashSet<String>,

    /// Set of processed checkpoint keys (`terminal_id:commit_hash`) for replay protection.
    pub processed_checkpoints: HashSet<String>,
}

impl OrchestratorState {
    pub fn new(workflow_id: String) -> Self {
        Self {
            run_state: OrchestratorRunState::Idle,
            workflow_id,
            task_states: HashMap::new(),
            workflow_planning_complete: true,
            conversation_history: Vec::new(),
            pending_events: Vec::new(),
            total_tokens_used: 0,
            error_count: 0,
            processed_commits: HashSet::new(),
            processed_commits_order: VecDeque::new(),
            pending_quiet_completion_checks: HashSet::new(),
            pending_quality_checks: HashSet::new(),
            processed_checkpoints: HashSet::new(),
        }
    }

    fn recompute_task_completion(state: &mut TaskExecutionState) {
        let total_done = state.completed_terminals.len() + state.failed_terminals.len();
        state.is_completed = state.planning_complete && total_done >= state.total_terminals;
    }

    fn first_unfinished_terminal_index(state: &TaskExecutionState) -> Option<usize> {
        state.terminal_ids.iter().position(|terminal_id| {
            !state.completed_terminals.iter().any(|id| id == terminal_id)
                && !state.failed_terminals.iter().any(|id| id == terminal_id)
        })
    }

    /// Initializes execution state for a task.
    pub fn init_task(&mut self, task_id: String, terminal_ids: Vec<String>) {
        let terminal_count = terminal_ids.len();
        self.task_states.insert(
            task_id.clone(),
            TaskExecutionState {
                task_id,
                current_terminal_index: 0,
                total_terminals: terminal_count,
                terminal_ids,
                completed_terminals: Vec::new(),
                failed_terminals: Vec::new(),
                planning_complete: true,
                is_completed: false,
            },
        );
    }

    /// Synchronize the in-memory terminal membership for a task with the database.
    pub fn sync_task_terminals(
        &mut self,
        task_id: String,
        terminal_ids: Vec<String>,
        planning_complete: bool,
    ) {
        let state = self
            .task_states
            .entry(task_id.clone())
            .or_insert_with(|| TaskExecutionState {
                task_id,
                current_terminal_index: 0,
                total_terminals: 0,
                terminal_ids: Vec::new(),
                completed_terminals: Vec::new(),
                failed_terminals: Vec::new(),
                planning_complete,
                is_completed: false,
            });

        state.terminal_ids = terminal_ids;
        state.total_terminals = state.terminal_ids.len();
        state.planning_complete = planning_complete;
        state.completed_terminals
            .retain(|terminal_id| state.terminal_ids.iter().any(|id| id == terminal_id));
        state.failed_terminals
            .retain(|terminal_id| state.terminal_ids.iter().any(|id| id == terminal_id));

        state.current_terminal_index = Self::first_unfinished_terminal_index(state)
            .unwrap_or(state.total_terminals);
        Self::recompute_task_completion(state);
    }

    pub fn set_workflow_planning_complete(&mut self, planning_complete: bool) {
        self.workflow_planning_complete = planning_complete;
    }

    pub fn set_task_planning_complete(&mut self, task_id: &str, planning_complete: bool) {
        if let Some(state) = self.task_states.get_mut(task_id) {
            state.planning_complete = planning_complete;
            Self::recompute_task_completion(state);
        }
    }

    pub fn mark_terminal_dispatched(&mut self, task_id: &str, terminal_id: &str) {
        if let Some(state) = self.task_states.get_mut(task_id)
            && let Some(index) = state.terminal_ids.iter().position(|id| id == terminal_id)
        {
            state.current_terminal_index = index;
            state.is_completed = false;
        }
    }

    /// Records a terminal completion for a task.
    pub fn mark_terminal_completed(&mut self, task_id: &str, terminal_id: &str, success: bool) {
        if let Some(state) = self.task_states.get_mut(task_id) {
            let already_recorded = state.completed_terminals.iter().any(|id| id == terminal_id)
                || state.failed_terminals.iter().any(|id| id == terminal_id);

            if already_recorded {
                tracing::debug!(
                    task_id = %task_id,
                    terminal_id = %terminal_id,
                    success,
                    "Ignoring duplicate terminal completion event"
                );
                return;
            }

            if success {
                state.completed_terminals.push(terminal_id.to_string());
            } else {
                state.failed_terminals.push(terminal_id.to_string());
            }

            Self::recompute_task_completion(state);
        }
    }

    /// Advances the current terminal index for a task.
    ///
    /// Returns `true` if there is a next terminal to dispatch, `false` otherwise.
    pub fn advance_terminal(&mut self, task_id: &str) -> bool {
        if let Some(state) = self.task_states.get_mut(task_id) {
            let mut next_index = state.current_terminal_index.saturating_add(1);
            while next_index < state.total_terminals {
                let terminal_id = &state.terminal_ids[next_index];
                let already_finished = state.completed_terminals.iter().any(|id| id == terminal_id)
                    || state.failed_terminals.iter().any(|id| id == terminal_id);
                if !already_finished {
                    state.current_terminal_index = next_index;
                    return true;
                }
                next_index += 1;
            }
            state.current_terminal_index = state.total_terminals;
        }
        false
    }

    /// Returns the next terminal index for a task, if any.
    ///
    /// Returns `None` if the task is completed or all terminals have been processed.
    pub fn get_next_terminal_for_task(&self, task_id: &str) -> Option<usize> {
        let state = self.task_states.get(task_id)?;
        if state.is_completed {
            return None;
        }
        for index in state.current_terminal_index..state.total_terminals {
            let terminal_id = &state.terminal_ids[index];
            let already_finished = state.completed_terminals.iter().any(|id| id == terminal_id)
                || state.failed_terminals.iter().any(|id| id == terminal_id);
            if !already_finished {
                return Some(index);
            }
        }
        None
    }

    /// Checks if a task is completed.
    pub fn is_task_completed(&self, task_id: &str) -> bool {
        self.task_states
            .get(task_id)
            .is_some_and(|s| s.is_completed)
    }

    /// Returns true if a specific task has any failed terminals.
    pub fn task_has_failures(&self, task_id: &str) -> bool {
        self.task_states
            .get(task_id)
            .is_some_and(|s| !s.failed_terminals.is_empty())
    }

    /// Appends a message and trims history based on config.
    pub fn add_message(&mut self, role: &str, content: &str, config: &OrchestratorConfig) {
        self.conversation_history.push(LLMMessage {
            role: role.to_string(),
            content: content.to_string(),
        });

        // 使用配置中的最大历史长度限制，避免上下文过长
        if self.conversation_history.len() > config.max_conversation_history {
            // 保留系统消息和最近的消息
            let system_msgs: Vec<_> = self
                .conversation_history
                .iter()
                .filter(|m| m.role == "system")
                .cloned()
                .collect();
            let recent: Vec<_> = self
                .conversation_history
                .iter()
                .rev()
                .filter(|m| m.role != "system")
                .take(config.max_conversation_history - system_msgs.len())
                .cloned()
                .collect();

            self.conversation_history = system_msgs;
            self.conversation_history.extend(recent.into_iter().rev());
        }
    }

    /// Returns true if all tasks are completed.
    pub fn all_tasks_completed(&self) -> bool {
        self.workflow_planning_complete
            && !self.task_states.is_empty()
            && self.task_states.values().all(|s| s.is_completed)
    }

    /// Returns true if any task has failed terminals.
    pub fn has_failed_tasks(&self) -> bool {
        self.task_states
            .values()
            .any(|s| !s.failed_terminals.is_empty())
    }

    /// Maximum number of processed commit hashes to retain in memory.
    const MAX_PROCESSED_COMMITS: usize = 10_000;

    /// Record a commit hash as processed, with bounded capacity.
    ///
    /// Returns `true` if the commit was newly inserted, `false` if already seen.
    /// When the set exceeds `MAX_PROCESSED_COMMITS`, the oldest half is evicted.
    pub fn record_processed_commit(&mut self, commit_hash: String) -> bool {
        if self.processed_commits.contains(&commit_hash) {
            return false;
        }

        // Evict oldest half when capacity is exceeded
        if self.processed_commits.len() >= Self::MAX_PROCESSED_COMMITS {
            let evict_count = Self::MAX_PROCESSED_COMMITS / 2;
            for _ in 0..evict_count {
                if let Some(old) = self.processed_commits_order.pop_front() {
                    self.processed_commits.remove(&old);
                }
            }
            tracing::info!(
                remaining = self.processed_commits.len(),
                evicted = evict_count,
                "Evicted oldest processed commits to stay within capacity"
            );
        }

        self.processed_commits.insert(commit_hash.clone());
        self.processed_commits_order.push_back(commit_hash);
        true
    }

    /// Validates and performs a run-state transition.
    pub fn transition_to(&mut self, new_state: OrchestratorRunState) -> anyhow::Result<()> {
        let valid_transitions = matches!(
            (self.run_state, new_state),
            (
                OrchestratorRunState::Idle | OrchestratorRunState::Paused,
                OrchestratorRunState::Processing
            ) | (
                OrchestratorRunState::Idle | OrchestratorRunState::Processing,
                OrchestratorRunState::Paused
            ) | (
                OrchestratorRunState::Idle
                    | OrchestratorRunState::Processing
                    | OrchestratorRunState::Paused,
                OrchestratorRunState::Stopped
            ) | (
                OrchestratorRunState::Processing | OrchestratorRunState::Paused,
                OrchestratorRunState::Idle
            )
        );

        if !valid_transitions {
            tracing::error!(
                "Invalid state transition: {:?} ??{:?}",
                self.run_state,
                new_state
            );
        }

        if valid_transitions {
            tracing::debug!("State transition: {:?} → {:?}", self.run_state, new_state);
            self.run_state = new_state;
            Ok(())
        } else {
            Err(anyhow::anyhow!(
                "Invalid state transition: {:?} → {:?}",
                self.run_state,
                new_state
            ))
        }
    }
}

/// Thread-safe shared orchestrator state.
pub type SharedOrchestratorState = std::sync::Arc<RwLock<OrchestratorState>>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_valid_state_transitions() {
        let mut state = OrchestratorState::new("test-workflow".to_string());

        // Idle -> Processing
        assert!(
            state
                .transition_to(OrchestratorRunState::Processing)
                .is_ok()
        );
        assert_eq!(state.run_state, OrchestratorRunState::Processing);

        // Processing -> Paused
        assert!(state.transition_to(OrchestratorRunState::Paused).is_ok());

        // Paused -> Processing
        assert!(
            state
                .transition_to(OrchestratorRunState::Processing)
                .is_ok()
        );

        // Processing -> Idle
        assert!(state.transition_to(OrchestratorRunState::Idle).is_ok());

        // Idle -> Paused
        assert!(state.transition_to(OrchestratorRunState::Paused).is_ok());

        // Paused -> Stopped
        assert!(state.transition_to(OrchestratorRunState::Stopped).is_ok());
        assert_eq!(state.run_state, OrchestratorRunState::Stopped);
    }

    #[test]
    fn test_invalid_state_transitions() {
        let mut state = OrchestratorState::new("test-workflow".to_string());

        // Can't stay in the same state
        assert!(
            state
                .transition_to(OrchestratorRunState::Processing)
                .is_ok()
        );
        let result = state.transition_to(OrchestratorRunState::Processing);
        assert!(result.is_err());
        assert!(
            result
                .unwrap_err()
                .to_string()
                .contains("Invalid state transition")
        );

        // After Stopped, can't transition to other states
        state.run_state = OrchestratorRunState::Stopped;
        assert!(
            state
                .transition_to(OrchestratorRunState::Processing)
                .is_err()
        );
        assert!(state.transition_to(OrchestratorRunState::Idle).is_err());
        assert!(state.transition_to(OrchestratorRunState::Paused).is_err());
    }

    #[test]
    fn test_all_valid_transitions_from_idle() {
        let mut state = OrchestratorState::new("test-workflow".to_string());

        // From Idle, can go to Processing, Paused, or Stopped
        assert!(
            state
                .transition_to(OrchestratorRunState::Processing)
                .is_ok()
        );
        state.run_state = OrchestratorRunState::Idle;

        assert!(state.transition_to(OrchestratorRunState::Paused).is_ok());
        state.run_state = OrchestratorRunState::Idle;

        assert!(state.transition_to(OrchestratorRunState::Stopped).is_ok());
    }

    #[test]
    fn test_all_valid_transitions_from_processing() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.run_state = OrchestratorRunState::Processing;

        // From Processing, can go to Idle, Paused, or Stopped
        assert!(state.transition_to(OrchestratorRunState::Idle).is_ok());
        state.run_state = OrchestratorRunState::Processing;

        assert!(state.transition_to(OrchestratorRunState::Paused).is_ok());
        state.run_state = OrchestratorRunState::Processing;

        assert!(state.transition_to(OrchestratorRunState::Stopped).is_ok());
    }

    #[test]
    fn test_all_valid_transitions_from_paused() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.run_state = OrchestratorRunState::Paused;

        // From Paused, can go to Processing, Idle, or Stopped
        assert!(
            state
                .transition_to(OrchestratorRunState::Processing)
                .is_ok()
        );
        state.run_state = OrchestratorRunState::Paused;

        assert!(state.transition_to(OrchestratorRunState::Idle).is_ok());
        state.run_state = OrchestratorRunState::Paused;

        assert!(state.transition_to(OrchestratorRunState::Stopped).is_ok());
    }

    #[test]
    fn test_advance_terminal_single_terminal() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.init_task("task-1".to_string(), vec!["term-1".to_string()]);

        // With only 1 terminal, advance should return false (no next terminal)
        assert!(!state.advance_terminal("task-1"));
        assert_eq!(state.get_next_terminal_for_task("task-1"), None);
    }

    #[test]
    fn test_advance_terminal_multiple_terminals() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.init_task(
            "task-1".to_string(),
            vec![
                "term-1".to_string(),
                "term-2".to_string(),
                "term-3".to_string(),
            ],
        );

        // Initial state: index 0
        assert_eq!(state.get_next_terminal_for_task("task-1"), Some(0));

        // Advance to index 1
        assert!(state.advance_terminal("task-1"));
        assert_eq!(state.get_next_terminal_for_task("task-1"), Some(1));

        // Advance to index 2
        assert!(state.advance_terminal("task-1"));
        assert_eq!(state.get_next_terminal_for_task("task-1"), Some(2));

        // No more terminals to advance to
        assert!(!state.advance_terminal("task-1"));
        assert_eq!(state.get_next_terminal_for_task("task-1"), None);
    }

    #[test]
    fn test_advance_terminal_nonexistent_task() {
        let mut state = OrchestratorState::new("test-workflow".to_string());

        // Advancing a non-existent task should return false
        assert!(!state.advance_terminal("nonexistent-task"));
        assert_eq!(state.get_next_terminal_for_task("nonexistent-task"), None);
    }

    #[test]
    fn test_get_next_terminal_completed_task() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.init_task(
            "task-1".to_string(),
            vec!["term-1".to_string(), "term-2".to_string()],
        );

        // Mark both terminals as completed
        state.mark_terminal_completed("task-1", "term-1", true);
        state.mark_terminal_completed("task-1", "term-2", true);

        // Task is completed, should return None
        assert!(state.is_task_completed("task-1"));
        assert_eq!(state.get_next_terminal_for_task("task-1"), None);
    }

    #[test]
    fn test_is_task_completed() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.init_task(
            "task-1".to_string(),
            vec!["term-1".to_string(), "term-2".to_string()],
        );

        // Initially not completed
        assert!(!state.is_task_completed("task-1"));

        // After one terminal, still not completed
        state.mark_terminal_completed("task-1", "term-1", true);
        assert!(!state.is_task_completed("task-1"));

        // After all terminals, completed
        state.mark_terminal_completed("task-1", "term-2", true);
        assert!(state.is_task_completed("task-1"));
    }

    #[test]
    fn test_is_task_completed_nonexistent() {
        let state = OrchestratorState::new("test-workflow".to_string());
        assert!(!state.is_task_completed("nonexistent-task"));
    }

    #[test]
    fn test_task_has_failures() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.init_task(
            "task-1".to_string(),
            vec![
                "term-1".to_string(),
                "term-2".to_string(),
                "term-3".to_string(),
            ],
        );

        // Initially no failures
        assert!(!state.task_has_failures("task-1"));

        // After successful completion, still no failures
        state.mark_terminal_completed("task-1", "term-1", true);
        assert!(!state.task_has_failures("task-1"));

        // After failed completion, has failures
        state.mark_terminal_completed("task-1", "term-2", false);
        assert!(state.task_has_failures("task-1"));

        // Still has failures after another success
        state.mark_terminal_completed("task-1", "term-3", true);
        assert!(state.task_has_failures("task-1"));
    }

    #[test]
    fn test_task_has_failures_nonexistent() {
        let state = OrchestratorState::new("test-workflow".to_string());
        assert!(!state.task_has_failures("nonexistent-task"));
    }

    #[test]
    fn test_duplicate_terminal_completion_does_not_advance_task() {
        let mut state = OrchestratorState::new("test-workflow".to_string());
        state.init_task(
            "task-1".to_string(),
            vec![
                "term-1".to_string(),
                "term-2".to_string(),
                "term-3".to_string(),
            ],
        );

        state.mark_terminal_completed("task-1", "term-1", true);
        state.mark_terminal_completed("task-1", "term-2", true);
        state.mark_terminal_completed("task-1", "term-2", true);

        assert!(!state.is_task_completed("task-1"));

        let task_state = state.task_states.get("task-1").expect("task state exists");
        assert_eq!(task_state.completed_terminals.len(), 2);
        assert_eq!(task_state.failed_terminals.len(), 0);
    }
}
