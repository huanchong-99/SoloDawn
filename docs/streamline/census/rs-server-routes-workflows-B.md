# Census: rs-server-routes-workflows-B

**File:** `crates/server/src/routes/workflows.rs`
**Line range:** 2400–4713 (second half — includes `auto_prepare_and_start` at 2298-2325)
**Unit:** rs-server-routes-workflows-B

---

## Module Map

| Function/Type | Lines | Kind | HTTP Route | Purpose | Relations/Notes |
|---|---|---|---|---|---|
| `AutoStartError` (pub enum) | 2298-2303 | pub type | — | Error wrapper for `auto_prepare_and_start`; single variant `Api(ApiError)` | Used by `planning_drafts.rs` caller |
| `auto_prepare_and_start` (pub async fn) | 2305-2325 | pub fn | — (internal) | Sequentially calls `prepare_workflow` then `start_workflow` with a 2 s delay. Used by `materialize_draft` (planning_drafts.rs:1017) to auto-launch agent-planned workflows | Called from `crates/server/src/routes/planning_drafts.rs` — is in-flight AuditPlan System B integration path |
| `pause_workflow` (async fn) | 2327-2388 | route handler | `POST /{id}/pause` | Stops runtime, kills terminals, resets running tasks→pending, resets terminal states→not_started | Calls `stop_workflow_runtime_if_running`, `cleanup_workflow_terminals` |
| `resume_workflow` (async fn) | 2397-2489 | route handler | `POST /{id}/resume` | Resumes paused workflow; self-heals terminal readiness via re-prepare if needed | Calls `prepare_workflow`, `refresh_prompt_watcher_registrations`; G05-002 |
| `stop_workflow` (async fn) | 2492-2552 | route handler | `POST /{id}/stop` | Stops workflow and marks cancelled; cancels tasks/terminals; cleans up worktrees | Calls `stop_workflow_runtime_if_running`, `cleanup_workflow_terminals`, `cleanup_workflow_worktrees`, `cleanup_finished_workflow_logs_best_effort` |
| `stop_workflow_runtime_if_running` (async fn) | 2554-2573 | internal helper | — | Guard: only calls `runtime.stop_workflow` if runtime is_running | Called by pause, stop, delete |
| `cleanup_workflow_terminals` (async fn) | 2575-2610 | internal helper | — | Kills PTY sessions, unregisters prompt watchers, unregisters terminal bridge | Returns `Vec<Terminal>` for downstream status updates |
| `cleanup_workflow_worktrees` (async fn) | 2612-2685 | internal helper | — | G23-004: batch-removes worktree directories on workflow stop/delete | Best-effort; non-fatal on error |
| `create_runtime_task` (async fn) | 2687-2783 | route handler | `POST /{id}/tasks` | Adds a new task to an already-running workflow; validates order_index and branch uniqueness | Calls `broadcast_task_status` |
| `create_runtime_terminal` (async fn) | 2785-2927 | route handler | `POST /{id}/tasks/{tid}/terminals` | Adds a terminal to a running task; validates CLI type and model config cross-reference; optionally starts immediately | Calls `start_terminal` (from terminals.rs) when `start_immediately=true` |
| `run_workflow_recovery` (async fn) | 2929-2957 | internal testable helper | — | Drives `recover_running_workflows` + `recover_incomplete_orchestrator_commands` on the runtime | Used by `recover_workflows` handler and tests |
| `recover_workflows` (async fn) | 2959-2965 | route handler | `POST /recover` | HTTP wrapper over `run_workflow_recovery` | — |
| `list_workflow_tasks` (async fn) | 2967-2981 | route handler | `GET /{id}/tasks` | Returns tasks with their terminals as `WorkflowTaskDetailResponse` | — |
| `UpdateTaskStatusRequest` (pub struct) | 2984-2987 | pub type | — | Request body for task status updates | Used only within this file |
| `SubmitPromptResponseRequest` (pub struct) | 2990-2995 | pub type | — | Request body for interactive prompt response; supports camelCase/snake_case alias | — |
| `SubmitOrchestratorChatRequest` (pub struct) | 2998-3008 | pub type | — | Request body for orchestrator chat; consumed by `chat_integrations.rs` | Exported via `pub` — used in `crates/server/src/routes/chat_integrations.rs` |
| `OrchestratorChatRequestMetadata` (pub struct) | 3010-3019 | pub type | — | Nested metadata in orchestrator chat request | Used by `chat_integrations.rs` |
| `OrchestratorChatMessageResponse` (pub struct) | 3021-3027 | pub type | — | Response for a single orchestrator message (role + content) | — |
| `SubmitOrchestratorChatResponse` (pub struct) | 3029-3037 | pub type | — | Response for orchestrator chat submission with command lifecycle info | — |
| `ListOrchestratorMessagesQuery` (pub struct) | 3039-3045 | pub type | — | Pagination query params (cursor/limit) for orchestrator message listing | — |
| `paginate_orchestrator_messages` (fn) | 3047-3058 | internal fn | — | Pure pagination calc: clamps limit 1..200, defaults to trailing window | Tested in `orchestrator_pagination_tests` |
| `update_task_status` (async fn) | 3060-3144 | route handler | `PUT /{id}/tasks/{tid}/status` | Kanban drag-and-drop status update; auto-completes workflow if all tasks done (CAS) | Calls `should_auto_complete_workflow`, `cleanup_finished_workflow_logs_best_effort` |
| `list_task_terminals` (async fn) | 3146-3167 | route handler | `GET /{id}/tasks/{tid}/terminals` | Lists terminals for a task as DTOs | — |
| `submit_prompt_response` (async fn) | 3169-3226 | route handler | `POST /{id}/prompts/respond` | Forwards user's interactive prompt answer to running runtime | Returns 409 if workflow not running (G16-014) |
| `submit_orchestrator_chat` (pub(crate) async fn) | 3228-3516 | route handler | `POST /{id}/orchestrator/chat` | Full orchestrator chat pipeline: feature flag, source/role validation, circuit breaker, rate limiting, idempotency (external_message_id dedup), command persistence, message persistence, circuit auto-pause | Called by `chat_integrations.rs` via `pub(crate)`; guarded by `SOLODAWN_ORCHESTRATOR_CHAT_ENABLED` env flag |
| `list_orchestrator_messages` (async fn) | 3518-3617 | route handler | `GET /{id}/orchestrator/messages` | Lists orchestrator conversation messages; prefers DB-persisted over in-memory; falls back to runtime when DB empty | Feature-flagged; paginates via `paginate_orchestrator_messages` |
| `get_workflow_events` (async fn) | 3619-3632 | route handler | `GET /{id}/events` | Returns `WorkflowEvent` list from DB | **BUG**: doc comment says `POST /merge` — stale copy-paste; actual route is `GET /{id}/events` |
| `merge_workflow` (async fn) | 3634-3885 | route handler | `POST /{id}/merge` | Squash-merges all completed task branches into target; CAS to "merging" state; per-workflow mutex; pre-merge SHA logging; G06-004 rollback logging; post-merge worktree cleanup | Calls `WorktreeManager`, `GitService.merge_changes`, `merge_coordinator` |
| `dto_tests` (#[cfg(test)] mod) | 3891-3942 | test | — | Contract tests: validates camelCase JSON serialization | — |
| `workflow_guard_tests` (#[cfg(test)] mod) | 3944-4171 | test | — | Unit tests for status guards, reconciliation logic, transition matrix, scope guards, auto-complete guards | Tests `can_merge_from_workflow_status`, `workflow_detail_needs_completion_reconciliation`, `validate_workflow_status_transition`, `validate_task_workflow_scope`, `should_auto_complete_workflow` |
| `create_request_validation_tests` (#[cfg(test)] mod) | 4173-4277 | test | — | Validates `validate_create_request` for diy/agent_planned modes | — |
| `recovery_response_tests` (#[cfg(test)] mod) | 4279-4391 | test | — | Integration test for `run_workflow_recovery` with in-memory SQLite; asserts R7-PB1 restart-recovery status = "paused" | — |
| `prompt_response_route_tests` (#[cfg(test)] mod) | 4393-4511 | test | — | Route-level tests for `submit_prompt_response` input validation | Uses `serial_test` |
| `orchestrator_chat_route_tests` (#[cfg(test)] mod) | 4513-4690 | test | — | Route-level tests for `submit_orchestrator_chat` and `list_orchestrator_messages` | Uses `serial_test` |
| `orchestrator_pagination_tests` (#[cfg(test)] mod) | 4692-4713 | test | — | Unit tests for `paginate_orchestrator_messages` edge cases | — |

---

## Key Cross-File Relationships

- `auto_prepare_and_start` → called from `crates/server/src/routes/planning_drafts.rs:1017` (materialize_draft path — AuditPlan System B)
- `submit_orchestrator_chat` (pub(crate)) + `SubmitOrchestratorChatRequest` + `OrchestratorChatRequestMetadata` → imported and called from `crates/server/src/routes/chat_integrations.rs:33,413`
- `cleanup_workflow_terminals` → uses `services::services::terminal::bridge::TerminalBridge`
- `cleanup_workflow_worktrees` + `merge_workflow` → use `services::services::worktree_manager::WorktreeManager`
- `merge_workflow` → uses `services::services::merge_coordinator::acquire_workflow_merge_lock`
- `run_workflow_recovery` → calls `OrchestratorRuntime::recover_running_workflows` + `recover_incomplete_orchestrator_commands`
- Orchestrator chat governance helpers (`ensure_orchestrator_circuit_closed`, `update_orchestrator_circuit_breaker`, `enforce_orchestrator_rate_limit`) defined earlier in file (lines 387-460) are all called from `submit_orchestrator_chat`

---

## In-flight Work Relevance (G1/System A/System B)

| Tag | Observation |
|---|---|
| System B (AuditPlan / materialize_draft) | `auto_prepare_and_start` is the bridge called by `materialize_draft` to auto-launch; `audit_plan` field is passed as `None` in all test builders within this range |
| System A (Quality Gate) | No Quality Gate integration in this range; the `quality` crate is consumed by the orchestrator agent, not directly here |
| G1 (external IDE/editor) | No "open in external IDE" code found in this range |
| VS Code webview bridge | Not present in this range; terminal bridge (`TerminalBridge`) handles PTY bridging only |
| Orchestrator chat rollout flag | `is_orchestrator_chat_feature_enabled()` reads `SOLODAWN_ORCHESTRATOR_CHAT_ENABLED` / `GITCORTEX_ORCHESTRATOR_CHAT_ENABLED` — this is a live feature flag that gates `submit_orchestrator_chat` and `list_orchestrator_messages` |
