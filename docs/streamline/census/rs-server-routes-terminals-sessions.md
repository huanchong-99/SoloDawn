# Module Census: rs-server-routes-terminals-sessions

**Branch:** refactor/streamline-quality-gates  
**Date:** 2026-06-14  
**Unit:** rs-server-routes-terminals-sessions  
**Files inspected:** 5

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `crates/server/src/routes/terminals.rs` | REST API for terminal lifecycle management (CRUD + logs) | `terminal_routes() -> Router`, `get_terminal_logs`, `start_terminal`, `stop_terminal`, `close_terminal`, `TerminalLogsQuery`, `TerminalSpawnConfig` (pub(crate)), `spawn_command_to_runner_config` (fn, dead) | Registered in `routes/mod.rs` at `.nest("/terminals", terminals::terminal_routes())`. Calls `ProcessManager`, `CCSwitchService`, `TerminalBridge`, `PromptWatcher`. Uses `db::models::terminal::Terminal`. Test: `terminal_stop_test.rs`, `terminal_logs_api_test.rs` | Contains BACKLOG-002 dead stubs: `TerminalSpawnConfig` (local) + `spawn_command_to_runner_config`. Duplicate `TerminalSpawnConfig` already exists in `services::services::runner_client`. `STARTABLE_TERMINAL_STATUSES` includes "working" unlike `runtime_actions.rs` (BUG). |
| `crates/server/src/routes/terminal_ws.rs` | WebSocket handler for PTY terminal I/O with reconnect/resume | `terminal_ws_routes() -> Router`, `validate_terminal_id(str) -> Result`, `WsMessage` (enum, TS exported) | Registered in `routes/mod.rs` at `.nest("/terminal", terminal_ws::terminal_ws_routes())`. `validate_terminal_id` also used by `routes/workflow_ws.rs`. Tests in `terminal_ws_test.rs`, `terminal_validation_test.rs` | BACKLOG-002 dead stub: `TerminalIO` struct + `from_process_handle` method. 5-minute idle timeout + 90s half-open detection. G09-003 seq-based replay on reconnect. |
| `crates/server/src/routes/sessions/mod.rs` | REST API for session CRUD + follow-up + review routing | `router(&DeploymentImpl) -> Router`, `get_sessions`, `get_session`, `create_session`, `follow_up`, `SessionQuery`, `CreateSessionRequest`, `CreateFollowUpAttempt` (TS exported) | Registered in `routes/mod.rs` via `.merge(sessions::router(&deployment))`. Mounts `sessions/queue.rs` and `sessions/review.rs` as subrouters. Called by `sessionsApi` in `frontend/src/lib/api.ts`. | Active production feature used by multiple frontend hooks. Contains retry/worktree-restore logic in `follow_up`. |
| `crates/server/src/routes/sessions/queue.rs` | Queue/cancel/status for follow-up messages buffered while agent runs | `router(&DeploymentImpl) -> Router`, `queue_message`, `cancel_queued_message`, `get_queue_status`, `QueueMessageRequest` (TS exported) | Nested under `/{session_id}/queue` in sessions router. Called by `queueApi` in `frontend/src/lib/api.ts`, used in `useSessionQueueInteraction.ts`, `ChatBoxBase.tsx` | Active feature; enables queuing a next message while agent is busy. |
| `crates/server/src/routes/sessions/review.rs` | Trigger an AI code review on a session's workspace | `start_review` (fn), `StartReviewRequest` (TS exported), `ReviewError` (TS exported enum) | Nested under `/{session_id}/review` in sessions router via `sessions::mod::router`. Called by `sessionsApi.startReview` → `StartReviewDialog.tsx` | Active production feature. Uses executor abstraction + container service. |

---

## BACKLOG-002 Dead Stubs

| Location | Symbol | Status |
|----------|--------|--------|
| `terminals.rs:44-83` | `TerminalSpawnConfig` (local) + `spawn_command_to_runner_config` | `#[allow(dead_code)]` — future gRPC migration placeholder. Duplicate of `services::runner_client::TerminalSpawnConfig` which is already in the codebase and wired to `RunnerClientImpl`. |
| `terminal_ws.rs:61-90` | `TerminalIO` struct + `from_process_handle` | `#[allow(dead_code)]` — future gRPC migration placeholder. Not referenced anywhere outside the file. |

---

## Candidate Flags

### C1 — `TerminalSpawnConfig` + `spawn_command_to_runner_config` in terminals.rs (lines 44-83)
- **Kind:** duplicate / stub  
- **Evidence:** Both decorated `#[allow(dead_code)]`. BACKLOG-002 comment. A canonical `TerminalSpawnConfig` already exists in `crates/services/src/services/runner_client.rs:29` used by `RunnerClientImpl`. Zero cross-file callers found by Grep.  
- **Why:** Local duplicate diverges from the canonical struct (uses `PathBuf` vs `String` for `working_dir`, `u16` vs `u32` for `cols/rows`). If the migration happens, the local copy would need to be reconciled with the canonical one anyway.  
- **Disposition:** delete (or replace comment reference to point to `services::runner_client::TerminalSpawnConfig`)  
- **Confidence:** high  
- **Blast radius:** None — `pub(crate)` and zero callers. Removing only these two items.

### C2 — `TerminalIO` struct + `from_process_handle` in terminal_ws.rs (lines 61-90)
- **Kind:** stub  
- **Evidence:** `#[allow(dead_code)]`, BACKLOG-002 comment. Zero callers outside file; `sync` Grep across entire codebase shows no references to `TerminalIO`.  
- **Why:** Future placeholder for gRPC streaming I/O path. The actual WS handler does not use `TerminalIO`; it accesses `ProcessHandle.writer` directly. No migration timeline visible.  
- **Disposition:** delete (or keep comment only)  
- **Confidence:** high  
- **Blast radius:** None — `pub(crate)` and unused.

### C3 — `STARTABLE_TERMINAL_STATUSES` divergence in terminals.rs (line 119-120) — BUG
- **Kind:** bug  
- **Evidence:** `terminals.rs` has `["not_started", "failed", "cancelled", "waiting", "working"]` (5 items). `runtime_actions.rs:40` has `["not_started", "failed", "cancelled", "waiting"]` with explicit comment: "[G15-007] 'working' removed: a terminal in 'working' status has a live PTY process. Re-launching it would spawn a duplicate process". `services/orchestrator/constants.rs:97` restricts further to only `["waiting"]`.  
- **Why:** A user or orchestrator that calls `POST /terminals/:id/start` on a "working" terminal will pass the route guard and attempt to spawn a second PTY, corrupting orchestrator state. The orchestrator's own `runtime_actions.rs` protects itself but the manual REST route does not.  
- **Disposition:** refactor — remove "working" from `STARTABLE_TERMINAL_STATUSES` in `terminals.rs` to match `runtime_actions.rs`.  
- **Confidence:** high  
- **Blast radius:** Low risk removal — reduces allowed statuses, preventing a dangerous double-spawn. No client should legitimately call start on a "working" terminal.

---

## Invisible Features

| Feature | What it does | Seems used | User visible | Note |
|---------|-------------|------------|--------------|------|
| BACKLOG-002 / RUNNER_CLIENT_MIGRATION stubs | Placeholder types (`TerminalSpawnConfig`, `TerminalIO`) + migration comments for future gRPC-based runner separation | No — unused dead stubs | No | `services::runner_client::RunnerClientImpl` is wired but still wraps `LocalRunner` → `ProcessManager` |
| G09-003 seq-based WS replay on reconnect | `last_seq` query param on `/terminal/{id}` WS; server replays only `seq > last_seq` chunks on reconnect | Yes — used in `ResumeParams` handler | Invisible to user, visible to frontend | Robust reconnect without full history resend |
| Half-open connection detection (G08-010) | 90s client-silent timer in heartbeat task; closes WS server-side | Yes — active in every WS connection | No | Prevents zombie connections |
| PromptWatcher auto-registration on WS connect | `sync_prompt_watcher_registration` in `terminal_ws.rs:731` — registers terminal for auto-confirm/AskUser without WS dependency | Yes — called on every WS connect | No | Provides resilience if PromptWatcher was not registered on start |
| Queued follow-up messages | `sessions/queue.rs` — buffer a single next message while agent is running; fires automatically when agent idle | Yes — wired in frontend `queueApi` | Partially visible (queue icon in ChatBoxBase) | One-message buffer only; second queue call replaces first |
| Session review via executor | `sessions/review.rs` — triggers a ReviewRequest via executor abstraction with optional commit-range context | Yes — called via `StartReviewDialog.tsx` | Yes (Review button in UI) | Supports both scoped (fork-point) and unscoped review modes |

---

## In-flight Work Relevance

- **G1 "open in external IDE/editor"**: No evidence in this scope. Not relevant.
- **VS Code webview bridge**: No evidence in this scope.
- **Quality Gate System A** (`quality/quality-gate.yaml`, `QualityGateConfig`): `routes/mod.rs` registers `quality::quality_terminal_routes()` also under `/terminals`, but this is in `routes/quality.rs` (out of scope). The terminal routes in scope do not directly interact with QualityGateConfig.
- **Planning-draft confirm→materialize flow + AuditPlan System B**: No direct interaction in sessions or terminal routes. The sessions `follow_up` handler calls `ExecutorAction` which may feed into audit, but the planning-draft/materialize flow is in `routes/planning_drafts.rs` (out of scope).
