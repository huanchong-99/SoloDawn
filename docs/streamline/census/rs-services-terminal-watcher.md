# Census: rs-services-terminal-watcher

**File:** `crates/services/src/services/terminal/prompt_watcher.rs`
**Lines:** 4419
**Branch:** refactor/streamline-quality-gates

---

## Module Map

| Section | Lines | Purpose | Public Surface | Relations / Notes |
|---------|-------|---------|---------------|-------------------|
| Module doc + imports | 1–33 | PTY output monitoring for interactive prompts; publishes `TerminalPromptDetected` to MessageBus | — | Imports: `SharedMessageBus`, `TerminalPromptStateMachine`, `ProcessManager`, `PromptDetector` |
| Constants | 36–110 | Debounce, timeout, auto-reply string literals, retry limits | — | `CLAUDE_BYPASS_ACCEPT_RESPONSE = "2\r"` hardcoded (E26-09 TODO); `STOP_HOOK_RECENT_WINDOW_SECS = 120` (R8-B1) |
| Module-level regex statics | 112–166 | `Lazy<Regex>` for bypass, Codex confirm, custom API key, no-exit, yes-accept, Notepad patterns | — | E26-08/E26-13: NOTEPAD regex constrained to single-line to prevent pathological backtracking |
| Free functions (prompt classifiers) | 168–411 | `is_bypass_permissions_prompt`, `is_bypass_permissions_enter_confirm_context` (dead), `is_codex_apply_patch_confirmation`, `is_claude_custom_api_key_prompt`, `is_claude_model_unavailable_prompt`, `is_notepad_prompt`, `is_unexpected_changes_followup_prompt`, various `has_*` helpers | None (private) | `is_bypass_permissions_enter_confirm_context` at line 173 is `#[allow(dead_code)]` and has 0 callers anywhere in codebase |
| `ClaudeBypassContext` struct | 417–467 | Rolling per-chunk accumulator for split-frame Claude bypass prompt markers | Private | Cleared on state reset and after successful accept |
| `UnexpectedChangesContext` struct | 469–534 | Rolling accumulator for "unexpected changes I didn't make" follow-up markers (EN+ZH) | Private | 12s window |
| `HandoffStallContext` struct | 537–583 | Rolling accumulator for clean-workspace + "what next?" stall markers | Private | 20s window; feeds handoff-stall auto-continue |
| `TerminalWatchState` struct + impl | 585–799 | Per-terminal state: terminal/workflow/task/session IDs, auto_confirm flag, detector, state machine, debounce, rolling contexts, pending-handoff submit flag, bypass retry state, R8-B1 stop-hook timestamp | Private | `mark_pending_handoff_submit` at line 759 has `#[allow(dead_code)]` but IS called at line 4046 (false-positive allow) |
| `WatchTaskHandle` struct | 805–808 | Monotonic task_id + JoinHandle for safe subscription replacement | Private | |
| `PromptWatcher` struct | 810–828 | Main service: `Arc<RwLock<HashMap<String, TerminalWatchState>>>` + active subscriptions map + `Arc<AtomicU64>` task counter | `pub struct PromptWatcher` (Clone) | Exported via `crates/services/src/services/terminal/mod.rs` |
| `PromptWatcher::new` | 832–840 | Constructor | `pub fn new(message_bus, process_manager)` | Called in `local-deployment/src/lib.rs:187`, `runtime_actions.rs:873` |
| `PromptWatcher::has_recent_stop_hook` | 848–857 | R8-B1: queries per-terminal stop-hook recency | `pub async fn has_recent_stop_hook(&self, terminal_id)` | Called via `RuntimeActionService::has_recent_stop_hook` → orchestrator agent |
| `PromptWatcher::register` | 860–894 | Register terminal + spawn output subscription task | `pub async fn register(terminal_id, workflow_id, task_id, session_id, auto_confirm)` | Called from launcher, terminal route, workflows route, terminal_ws |
| `PromptWatcher::unregister` | 897–916 | Remove terminal state + abort subscription task | `pub async fn unregister(terminal_id)` | Called from runtime_actions close_terminal, server terminal routes |
| `spawn_output_subscription_task` (private) | 918–1026 | Spawn tokio task to subscribe to ProcessManager OutputFanout and loop-call `process_output` | Private | Uses oneshot channel for startup ACK with 2s timeout |
| `remove_subscription_if_current` (private) | 1028–1041 | Safely removes subscription only if task_id matches (handles replacement races) | Private | |
| `try_direct_terminal_input` (private) | 1043–1096 | Write directly to PTY writer bypassing message bus | Private | Used by bypass accept, model-unavailable recovery, handoff-stall |
| `normalize_input_for_direct_write` (private) | 1098–1115 | Normalize `\n`/`\r\n` to bare `\r` for PTY line endings | Private | |
| `build_handoff_stall_continue_response` (private) | 1117–1133 | Build templated handoff-stall recovery message with workflow/task/terminal metadata | Private | |
| `resolve_terminal_input_session_id` (private) | 1135–1160 | Resolve preferred vs active PTY session_id, warn on mismatch | Private | |
| `publish_terminal_input_with_active_session` (private) | 1162–1179 | Resolve session + call `message_bus.publish_terminal_input` | Private | G07-006 TODO: unhandled delivery failure |
| `send_claude_bypass_accept_with_fallback` (private) | 1181–1210 | Try direct PTY write → fallback to message bus for bypass accept | Private | |
| `process_output` (core) | 1212–2411 | Main processing loop: chunk-level detection (bypass accept, custom API key, model-unavailable, bypass toggle, Notepad, unexpected-changes, handoff-stall, Codex confirm) then line-level detection, then general `PromptDetector` + state machine | `pub async fn process_output(terminal_id, output)` | Called from output subscription loop. G07-003: all chunk-level auto-handlers respect `auto_confirm` guard |
| `on_response_sent` | 2414–2427 | Update state machine after external response delivery | `pub async fn on_response_sent(terminal_id, decision)` | No external callers found; only self-called internally |
| `on_waiting_for_approval` | 2430–2439 | Transition to WaitingForApproval state | `pub async fn on_waiting_for_approval(terminal_id, decision)` | No external callers found in server/local-deployment |
| `reset_state` | 2442–2451 | Reset state machine + clear contexts for a terminal | `pub async fn reset_state(terminal_id)` | Only test callers found |
| `get_state` | 2454–2457 | Get current `PromptState` for a terminal | `pub async fn get_state(terminal_id) -> Option<PromptState>` | Only test callers found |
| `is_registered` | 2460–2465 | True iff both state map and active subscription map contain the terminal | `pub async fn is_registered(terminal_id)` | Called from launcher.rs, terminal_ws.rs |
| Tests | 2472–4419 | 30 unit tests covering all auto-confirm paths, debounce, retry, CN/EN split-line contexts | `#[cfg(test)]` | Direct state injection via `watcher.terminals.write()` |

---

## Candidates

| ID | Path:Lines | Kind | Evidence | Disposition | Confidence | Blast Radius |
|----|-----------|------|----------|-------------|------------|-------------|
| C1 | `prompt_watcher.rs:172-179` `is_bypass_permissions_enter_confirm_context` | dead | `#[allow(dead_code)]` at line 172; Grep over entire codebase shows zero call sites outside the definition. Confirmed by existing R5 inventory. | delete | high | None — function is private, not exported, no callers |
| C2 | `prompt_watcher.rs:758-761` `mark_pending_handoff_submit` — `#[allow(dead_code)]` annotation | bug | Annotation is a false-positive: `state.mark_pending_handoff_submit()` is called at line 4046 in the test harness (`test_process_output_bypass_status_line_auto_enters_when_handoff_submit_pending`). The method is legitimately used. Only the `#[allow]` annotation is wrong. | refactor | high | Remove annotation only; zero functional change |
| C3 | `on_response_sent` (pub, lines 2414–2427), `on_waiting_for_approval` (pub, 2430–2439), `reset_state` (pub, 2442–2451), `get_state` (pub, 2454–2457) | dubious-feature | No external callers found in server, local-deployment, or orchestrator outside their own tests and in-file usage. `on_response_sent` / `on_waiting_for_approval` reflect state transitions that are already handled internally by `process_output`. | investigate | medium | If truly unused externally, these could be de-publified or removed without affecting runtime behavior. Risk: may be called via future planned paths. |

---

## Invisible Features

| Feature | What it does | Seems used | Note |
|---------|-------------|-----------|------|
| R8-B1 stop-hook detection | Records `last_stop_hook_observed_at` when PTY output contains "stop hook" / "running stop hook" marker; `has_recent_stop_hook()` used by orchestrator to decide clean-context relaunch vs direct input | Yes — queried in `agent.rs:3335-3336` | Critical for R8 retry workflow; 120s window |
| Claude bypass auto-accept (E26-09) | Detects "Bypass Permissions mode" + "Yes, I accept" menu and sends hardcoded `"2\r"` to select it | Yes — tested and used in auto_confirm mode | Hardcoded menu position; fragile if Claude CLI reorders menu (TODO logged) |
| Handoff-stall auto-continue | Detects clean-workspace + "what next?" idiom and injects completion-contract reminder with workflow/task/terminal metadata | Yes — active in auto_confirm orchestrated terminals | Fires at both chunk and line level; sends immediate Enter follow-up |
| Unexpected-changes auto-continue | Detects Codex "changes I didn't make" / Chinese "未发起的变更" pattern and sends continue instruction | Yes — fires regardless of auto_confirm per test at line 3130 | Note: line 2165 check does NOT gate on `state.auto_confirm`, making this the only handler that fires even in manual mode |
| Notepad decline | Auto-declines "Open in Notepad? (y/N)" prompts in headless Windows flows | Yes — active in auto_confirm mode | Prevents blocking of headless CI/Windows workflows |
| Model-unavailable recovery | Detects "issue with selected model / run /model" or `model_not_found` from OpenAI-compat gateways; sends `/model` command + Enter | Yes — tested | Covers both Claude and OpenAI-compat 503 gateway responses |
| Claude custom API key auto-select | Detects "Do you want to use this API key?" / "Detected a custom API key" and sends ArrowUp+Enter to choose Yes | Yes — tested | Needed for custom API key deployments to avoid defaulting to "No" |
| Rolling multi-chunk context | `ClaudeBypassContext`, `UnexpectedChangesContext`, `HandoffStallContext` accumulate markers across separate PTY output chunks within recency windows | Yes — core to line-by-line rendering support | Without these, split-frame TUI renders would miss prompts |
| G07-001 RwLock contention TODO | Single `Arc<RwLock<HashMap<String,TerminalWatchState>>>` for all terminals; comment at line 812 notes potential DashMap refactor | No (not implemented) | Noted for future scaling |
| G07-006 delivery failure silencing | `publish_terminal_input_with_active_session` does not check delivery success; state machine stays in Responding if bus has no subscribers | No (not fixed) | Silent drop risk; comment at line 1163 |
