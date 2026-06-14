# Census: rs-services-events-chat-approvals

Unit covers: `crates/services/src/services/events/` subdir (3 files) + `chat_connector.rs` + `approvals/executor_approvals.rs`

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|---|---|---|---|---|
| `events/types.rs` | Shared types for the events subsystem: error enum, hook-table enum, record variant enum, legacy patch structs | `EventError`, `HookTables`, `RecordTypes`, `EventPatch`, `EventPatchInner` | Re-exported by `events.rs`; consumed by `events.rs` (hook) and `events/streams.rs` (filter) | `EventPatch`/`EventPatchInner` exist only for legacy fallback path in streams; no server-side code outside events/* imports them directly |
| `events/patches.rs` | JSON-Patch builders for each domain entity (task, project, workspace, execution_process, scratch) | 5 pub sub-mods: `task_patch`, `project_patch`, `workspace_patch`, `execution_process_patch`, `scratch_patch` each with `add`/`replace`/`remove` | Re-exported by `events.rs`; called by both `events.rs` (hook dispatch) and `events/streams.rs` (soft-delete translate, Lagged resync) | All 5 modules fully used; `escape_pointer_segment` helper is crate-private |
| `events/streams.rs` | WebSocket stream builders: initial-snapshot + live-delta filter for tasks, projects, workspaces, execution_processes, scratch | 5 `pub async fn` methods on `EventService`: `stream_tasks_raw`, `stream_projects_raw`, `stream_execution_processes_for_session_raw`, `stream_scratch_raw`, `stream_workspaces_raw` | All 5 called from `crates/server/src/routes/` (tasks, projects, execution_processes, scratch, task_attempts); depends on `patches::execution_process_patch`, `types::{EventError,EventPatch,RecordTypes}` | Contains `TaskProjectCache` (private TTL cache, G33-008); legacy `EventPatch` deserialization kept for backward compat |
| `chat_connector.rs` | Provider-agnostic chat-platform trait + Telegram concrete implementation | Trait `ChatConnector` (`send_message`, `send_reply`, `provider_name`, `is_connected`); `TelegramConnector` struct with `new`, `from_env` | `ChatConnector` trait imported by `feishu.rs` (FeishuConnector impl); `TelegramConnector` is only defined here — zero callers outside this file; chat_integrations.rs routes reference only `"telegram"` as a string provider name but never use `TelegramConnector` directly | `TelegramConnector` confirmed zero-usage: no other file imports or instantiates it — delete candidate per scope notes |
| `approvals/executor_approvals.rs` | Bridge adapting the approvals service to the executor-trait interface; triggers task InReview transition and OS notification when tool approval is needed | `ExecutorApprovalBridge` struct with `new` (returns `Arc<Self>`); implements `ExecutorApprovalService` trait from `executors` crate | Instantiated in `crates/local-deployment/src/container.rs` (line 1340); depends on `approvals::Approvals`, `notification::NotificationService`, `approvals::ensure_task_in_review` | Bridges the executor (AI agent tool use) with the human-approval workflow; active production path |

## Candidate Flags

| Candidate | Kind | Evidence | Disposition | Confidence | Blast Radius |
|---|---|---|---|---|---|
| `TelegramConnector` (lines 38-137 of `chat_connector.rs`) | dead | Zero imports of `TelegramConnector` anywhere in the codebase. `chat_integrations.rs` uses `"telegram"` as a string constant but calls the Feishu/webhook path, not `TelegramConnector`. `feishu.rs` imports `ChatConnector` trait but uses `FeishuConnector`. `from_env` reads `SOLODAWN_TELEGRAM_BOT_TOKEN` env var but is never called. | delete | high | Removing the struct leaves the `ChatConnector` trait intact. Only risk: if a yet-uncommitted branch or plugin calls `TelegramConnector::from_env()` — verify with git grep before cutting. |
| `EventPatch` / `EventPatchInner` in `types.rs` (lines 72-82) | legacy | Only used in the "fallback" / "legacy" code paths inside `streams.rs` and `events.rs` (hook dispatch). Comments call these paths "old EventPatch format" / "backward compat". Modern code paths use direct JSON pointer patches (`/tasks/<uuid>` etc.). | investigate | medium | Cannot delete until the legacy fallback branches in `events.rs` (line 476-491) and `streams.rs` are also removed. They are live code paths guarding against malformed modern patches. |
| `entry_count` field on `EventService` (`events.rs` line 33, `#[allow(dead_code)]`) | dead | Marked `#[allow(dead_code)]`. Used only in the fallback `EventPatch` path (incrementing `/entries/{n}` index). If/when that legacy path is deleted, `entry_count` has no purpose. | refactor | medium | Removing requires also removing the legacy `EventPatch` fallback block. Scope note says this is a known false-positive allow; keep for now until legacy path is pruned. |

## Invisible Features

| Feature | What it does | Seems used | User visible |
|---|---|---|---|
| `TaskProjectCache` (TTL in-memory map in `streams.rs`) | Avoids per-event DB lookup for task→project_id when filtering WebSocket deltas; 5-min TTL, lazy eviction | Yes — active in `stream_tasks_raw` and workspace-remove path | No |
| Lag-resync on broadcast overflow | On `BroadcastStreamRecvError::Lagged`, each stream function re-fetches a full snapshot from DB and emits it rather than silently dropping events (G33-001) | Yes — all 5 stream functions implement this | No (invisible reliability mechanism) |
| Soft-delete translate in `stream_execution_processes_for_session_raw` | When `show_soft_deleted=false` and a process has `dropped=true`, converts an Add/Replace to a Remove patch before forwarding to the client | Yes | No (client sees a clean remove) |
| `ExecutorApprovalBridge` → OS notification on approval request | Calls `NotificationService::notify` (likely system tray / OS push) when a tool approval is pending | Yes | Yes (OS notification) |
| `find_matching_tool_use` (private fn in `approvals.rs`) | Matches by `tool_call_id` in metadata to find the exact conversation entry that triggered approval; skips already-pending entries to prevent double-approval | Yes — called from `create_with_waiter` | No |
| `ensure_task_in_review` (pub(crate) in `approvals.rs`) | Atomically transitions a task to `InReview` state before waiting for human approval | Yes — called from `executor_approvals.rs` | Indirectly (task status visible in UI) |
