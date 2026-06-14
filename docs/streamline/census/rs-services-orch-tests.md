# Census: rs-services-orch-tests

**Scope:** `crates/services/src/services/orchestrator/tests.rs` (3516 lines)
**Branch:** refactor/streamline-quality-gates

## Module Map

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| `tests.rs` | Comprehensive unit + integration test suite for the OrchestratorAgent, MessageBus, LLM client, OrchestratorState, OrchestratorConfig, and git-event handoff logic. Gated by `#[cfg(test)]`. | All test functions (no exported symbols). Two private helper fns: `setup_test_workflow()`, `setup_workflow_with_terminals()`. | Imports from `crate::services::orchestrator::{BusMessage, CommitMetadata, LLMMessage, MessageBus, MockLLMClient, OrchestratorAgent, OrchestratorConfig, OrchestratorInstruction, OrchestratorRunState, OrchestratorState, RuntimeActionService, TerminalCompletionEvent, TerminalCompletionStatus, constants::DEFAULT_LLM_RATE_LIMIT_PER_SECOND, create_llm_client}`. Also uses `wiremock`, `sqlx`, `db::DBService`, `db::models::Workflow`, `chrono`, `uuid`. Calls `OrchestratorAgent::with_llm_client`, `agent.execute_instruction`, `agent.handle_git_event`, `agent.attach_runtime_actions`. | #[cfg(test)] only; no production build impact. `MockLLMClient` is only exported under #[cfg(test)] in mod.rs. 28 occurrences of in-test DB migration setup boilerplate. |

## Test Suites

| Suite | Line Range | Tests | What Is Covered |
|-------|-----------|-------|-----------------|
| 1: Types Serialization | 23–149 | `test_orchestrator_instruction_serialization`, `test_terminal_completion_event_full`, `test_all_instruction_variants` | Serde round-trip for `OrchestratorInstruction` (all 12 variants), `TerminalCompletionEvent`, `CommitMetadata` |
| 2: Configuration | 152–230 | `test_default_config`, `test_config_validation`, `test_config_from_workflow` | `OrchestratorConfig::default`, `validate()`, `from_workflow()` |
| 3: State Management | 233–366 | `test_state_initialization`, `test_task_init_and_tracking`, `test_terminal_completion_marking`, `test_conversation_history`, `test_conversation_history_pruning`, `test_all_tasks_completed` | `OrchestratorState`: init, task tracking, conversation history + pruning |
| 4: LLM Client | 369–493 | `test_llm_client_basic_request`, `test_llm_client_error_handling`, `test_llm_client_empty_response` | `create_llm_client` with wiremock: happy path, 401 error, empty choices |
| 5: Message Bus | 496–671 | `test_message_bus_creation`, `test_message_bus_topic_subscription`, `test_message_bus_topic_isolation`, `test_message_bus_broadcast`, `test_publish_workflow_event_fanout_to_topic_and_broadcast`, `test_publish_terminal_completed` | `MessageBus` pub/sub, broadcast, topic isolation, `publish_workflow_event` fan-out |
| 6: OrchestratorAgent | 674–1325 | `test_instruction_parsing`, `test_all_instruction_parsing`, `test_agent_creation`, `test_execute_instruction_supports_runtime_planning_array`, `test_execute_instruction_send_to_terminal`, `test_execute_instruction_complete_workflow_success`, `test_execute_instruction_fail_workflow` | Agent creation, instruction dispatch, runtime array batch, workflow completion/failure |
| 6.5: StartTask & Auto-Dispatch | 1328–2128 | `test_execute_instruction_start_task`, `test_execute_instruction_start_task_skips_dispatch_when_terminal_not_waiting`, `test_execute_instruction_start_task_uses_latest_pty_after_cas`, `test_execute_instruction_send_to_terminal_requires_working_status`, `test_execute_instruction_send_to_terminal_skips_non_working_without_pty`, `test_execute_instruction_start_task_claude_code_retries_submit_once_on_first_dispatch`, `test_execute_instruction_start_task_codex_uses_terminal_input_with_submit`, `test_execute_instruction_start_task_no_pty` | PTY dispatch gating, CAS-refresh of PTY session, cli-codex vs cli-claude-code submit-keystroke sequences |
| 7: handle_git_event | 2130–2921 | 8 tests covering terminal completion, deferral within quiet window, race conditions, out-of-order terminal guard, next-terminal dispatch without pre-initialized task state, review_pass status update, workflow mismatch | Git-event driven terminal completion lifecycle; quiet-window logic; idempotency |
| 8: LLM Error Propagation | 2922–2967 | `test_llm_error_propagation` | G24-006: single-provider 500 errors propagate directly (no internal retry) |
| 9: Git Event-Driven Integration | 2969–3516 | `test_publish_git_event`, `test_git_event_broadcast`, `test_commit_idempotency`, `test_handle_git_event_no_metadata_infers_task_and_advances_handoff`, `test_handle_git_event_no_metadata_no_hint_commits_do_not_stall_parallel_tasks`, `test_handle_git_event_no_metadata_marks_failed_when_task_cannot_be_inferred`, `test_processed_commits_tracking`, `test_git_event_topic_isolation` | Commit idempotency, no-metadata inference path, parallel-task stall prevention, topic isolation |

## Candidate Flags

### C1 — Duplicate test: `test_instruction_parsing` (L678) vs `test_orchestrator_instruction_serialization` (L28)
`test_instruction_parsing` tests only `SendToTerminal` deserialization with a hard-coded JSON literal and a `match`. `test_orchestrator_instruction_serialization` (L28) covers exactly the same variant with a full round-trip (serialize + deserialize + assert fields). Neither test adds distinct new coverage over the other for this single variant. `test_all_instruction_parsing` (L695) covers all 12 variants. So `test_instruction_parsing` is fully subsumed.

### C2 — Duplicate test: `test_all_instruction_variants` (L82) vs `test_all_instruction_parsing` (L696)
Both cover all 12 `OrchestratorInstruction` variants. `test_all_instruction_variants` does serialize-then-deserialize round-trip and checks the `type` JSON key. `test_all_instruction_parsing` goes in the reverse direction (JSON string → parse → checks type key only). The round-trip direction is complementary, but both verify the same serde contract. The overlap is substantial and one of the two could be dropped or merged.

### C3 — DB-migration boilerplate repeated 28 times (no shared helper)
Every integration test that needs a DB spins up a new `SqlitePoolOptions::new().connect(":memory:")`, calls `Migrator::new(migration_dir).await.unwrap()`, and then manually `INSERT`s a project + workflow + task + terminal. Two private helpers (`setup_test_workflow`, `setup_workflow_with_terminals`) exist but are not used by all tests — several tests (e.g. `test_execute_instruction_complete_workflow_success`, L1102; `test_execute_instruction_fail_workflow`, L1229; `test_execute_instruction_send_to_terminal`, L944) each hand-roll the same migration+insert sequence. This is a maintenance hazard but not dead code.

### C4 — quality_gate_mode always `"off"` in all test configs
Every one of the 33 `quality_gate_mode` occurrences in the file sets the value to `"off"`. No test exercises `"shadow"`, `"warn"`, or `"enforce"` mode. This is a coverage gap for Quality Gate System A, not dead code in the tests themselves, but it flags the system as untested at this layer.

## Invisible Features Observed

- **Quiet-window deferral** (L2351–2536): Commit completion is deferred if the terminal was updated within the last ~40 seconds. Invisible in UI; only observable via DB timestamps.
- **Commit idempotency** (L3036–3117): The orchestrator tracks `processed_commits` in `OrchestratorState` to ignore duplicate git events. No UI exposure.
- **No-metadata inference** (L3119–3473): When a commit message lacks `---METADATA---`, the agent infers the completing terminal by task-id hint from the commit message or by finding the sole working terminal on a branch. Used in production but not surfaced to users.
- **CLI-codex double-submit-keystroke** (L1967–2074): `codex` CLI type gets two empty-string `TerminalInput` keystrokes sent after the instruction, whereas `claude-code` gets one. Invisible behaviour difference per CLI type.
- **MANDATORY quality suffix** (asserted at L1513, L1684, L1928, L2035): All `StartTask` dispatches append a `"MANDATORY"` quality obligation suffix to the instruction before sending it to the terminal. Tests confirm this is present. This is a Quality Gate System A enforcement touchpoint embedded in the dispatch layer.
