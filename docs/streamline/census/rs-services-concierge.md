# Census: rs-services-concierge

Unit: `crates/services/src/services/concierge/`
Branch: `refactor/streamline-quality-gates`
Date: 2026-06-14

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `mod.rs` | Module root and re-export facade | `ConciergeAgent`, `push_workflow_completion`, `ConciergeBroadcaster`, `ConciergeEvent` | Re-exports from `agent`, `notifications`, `sync` | Clean facade; all three pub items are externally consumed |
| `agent.rs` | LLM-powered session-scoped conversational agent | `ConciergeAgent` struct, `new()`, `process_message()`, `set_message_bus()`, `set_shared_config()`, `cancel_watchers_for_session()` | Consumes `ConciergeBroadcaster`, `tools::execute_tool`, `prompt::concierge_system_prompt`, `orchestrator::{LLMClient, MessageBus, OrchestratorConfig}`; called from `server/main.rs`, `feishu.rs`, `routes/concierge.rs`, `routes/concierge_ws.rs` | Tool loop capped at 5 iterations; runtime tools (start/prepare/send) dispatched via localhost HTTP to avoid DI circular dep |
| `notifications.rs` | Workflow event bus monitoring and completion push | `watch_workflow_events()` (pub async), `push_workflow_completion()` (pub async) | Consumes `ConciergeBroadcaster`, `orchestrator::message_bus::BusMessage`; `watch_workflow_events` spawned from `agent.rs`; `push_workflow_completion` exported via `mod.rs` but **zero external call sites found** | Legacy `progress_notifications` field checked for backward compat alongside `sync_progress` |
| `prompt.rs` | Static system prompt for Concierge LLM | `concierge_system_prompt() -> String` | Called only from `agent.rs::build_llm_messages()` | Lists 15 tool names hard-coded; must stay in sync with `tools.rs` dispatch table |
| `sync.rs` | Real-time event broadcasting to Web WS + Feishu | `ConciergeBroadcaster`, `ConciergeEvent` (enum), `FeishuTarget` (struct), `FeishuSender` (trait) | `ConciergeBroadcaster` used in `agent.rs`, `orchestrator/agent.rs`, `orchestrator/runtime.rs`, `routes/concierge_ws.rs`, `server/main.rs`, `self_test/runner.rs`; `FeishuTarget`/`FeishuSender` defined here but **zero external callers of `register_feishu`/`unregister_feishu`/`remove_session`** | Feishu DI done via `Arc<dyn FeishuSender>` trait object to avoid cross-crate dep on feishu_connector |
| `tools.rs` | Tool dispatch table + 13 individual tool implementations | `ToolCall` (struct), `parse_tool_call()`, `execute_tool()` (pub async) | Called from `agent.rs`; imports `db::models::{project, workflow, terminal, concierge, ...}`, `services::config::Config` | `extract_inline_json` handles brace-balanced extraction; runtime tools (prepare/start/send) return `RUNTIME_TOOL:` marker |

## Candidate Flags

### C1 — `push_workflow_completion` (notifications.rs:160): zero external call sites
Re-exported as `pub use notifications::push_workflow_completion` in `mod.rs` but grep across entire repo finds only the definition and the re-export — no caller outside the module. The orchestrator completes workflows and presumably should call this, but does not.

### C2 — `register_feishu` / `unregister_feishu` / `remove_session` (sync.rs:69,74,151): orphaned lifecycle methods
All three methods on `ConciergeBroadcaster` are `pub` but have zero callers outside `sync.rs` itself. `FeishuTarget` and `FeishuSender` are also only used internally. The Feishu push path is wired up only via `push_text_to_feishu` being called from within the broadcaster — but the `DashMap<feishu_channels>` is never populated from outside.

### C3 — `cancel_watchers_for_session` (agent.rs:56): no external caller
`pub async fn cancel_watchers_for_session` is defined as an explicit cleanup hook but has zero callers outside `agent.rs` (only call site is within the module's own tests or the definition). Session cleanup / disconnect path does not call it.

### C4 — Legacy `progress_notifications` dual-check (notifications.rs:121-125): redundant compat shim
`should_save` checks both `session.progress_notifications || session.sync_progress`. The comment acknowledges this is a backward-compat shim for "the old single toggle." If the old field is not removed from the DB schema, this is dead weight accumulating.

### C5 — Incomplete `looks_incomplete` heuristic (agent.rs:207-221): dubious feature
LLM responses ending with `:` or `…` are assumed to be tool-call preambles and the agent loops without a tool call. This heuristic can silently loop (and burn extra LLM tokens) if the LLM legitimately ends a response with a colon. No test coverage found.

## Invisible Features

- **Feishu broadcast channel** (`sync.rs`): A full `FeishuTarget`/`FeishuSender` trait abstraction exists to decouple the broadcaster from the feishu_connector crate. However, `register_feishu` is never called outside the module, so the Feishu push path inside `push_text_to_feishu` is silently a no-op for all sessions. Feishu push via the concierge broadcaster is effectively disabled.

- **Workflow-event watcher** (`notifications.rs::watch_workflow_events`): Spawned as a background task when a workflow starts (agent.rs:463). Bridges `MessageBus` events into concierge session system messages. Respects four independent toggles (`feishu_sync`, `notify_on_completion`, `sync_terminal`, `sync_progress`). This is an invisible real-time notification pipeline — not surfaced in any obvious UI widget.

- **Runtime tool localhost loopback** (`agent.rs:349-484`): `prepare_workflow`, `start_workflow`, `send_to_orchestrator` are dispatched by calling the server's own HTTP REST API on `127.0.0.1:{BACKEND_PORT}`. This is an intentional DI bypass to avoid circular service dependencies. The `RUNTIME_TOOL:` marker protocol is a private contract between `tools.rs` and `agent.rs`.

- **Session auto-naming** (`agent.rs:256-259`): First user message is used to auto-set the session name (up to 50 chars). Silent, no UI notification.

- **Model config sync-to-DB** (`tools.rs:428-468`): When a workflow is created, if the `model_config_id` exists only in `config.json` (not in the SQLite `model_config` table), `execute_create_workflow` transparently syncs it into the DB to satisfy FK constraints. This is an invisible migration-compat shim.

## In-flight Work Relevance

- **G1 (open-in-editor)**: No references in concierge code. Not relevant.
- **VS Code webview bridge**: No references. Not relevant.
- **Quality Gate System A** (`QualityGateConfig`): No references in concierge code.
- **AuditPlan System B / planning-draft confirm->materialize**: `audit_plan: None` is set on line 498 of `tools.rs` during `execute_create_workflow`, meaning concierge-created workflows always have null `audit_plan`. No planning-draft confirm/materialize flow is wired through concierge tools — if AuditPlan is required before workflow start, the concierge `start_workflow` tool bypasses it entirely.
