# Census: rs-services-terminal-rest

Unit: `crates/services/src/services/terminal/` (all *.rs except prompt_watcher.rs)

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `mod.rs` | Re-exports all sub-modules, glue layer | `TerminalBridge`, `CliDetector`, `LaunchResult`, `TerminalLauncher`, `OutputChunk`, `OutputFanout`, `OutputFanoutConfig`, `OutputSubscription`, `ProcessHandle`, `ProcessManager`, `DetectedPrompt`, `PromptDetector`, `PromptKind`, `ARROW_UP`, `ARROW_DOWN`, `ArrowSelectOption`, `build_arrow_sequence`, `PromptWatcher`, `Utf8DecodeChunk`, `Utf8DecodeStats`, `Utf8StreamDecoder` | Imports all sibling modules; consumed by `crates/services/src/services/mod.rs` | Pure re-export facade; no logic |
| `process.rs` | PTY process lifecycle management + batched terminal log persistence | `ProcessHandle`, `ProcessManager` (spawn_pty_with_config, spawn_pty, resize, kill, kill_terminal, is_running, list_running, cleanup, get_handle, subscribe_output, latest_output_seq, attach_terminal_logger), `TerminalLogger` (new, with_max_buffer_size, append, flush), `SpawnCommand`, `SpawnEnv`, `DEFAULT_COLS`, `DEFAULT_ROWS`, `DEFAULT_MAX_BUFFER_SIZE` | Inbound callers: `launcher.rs`, `bridge.rs`, `runner/src/service.rs`, `server/routes/terminals.rs`, `server/routes/terminal_ws.rs`, `server/routes/workflows.rs`, `services/src/services/runner_client.rs`; Outbound: `output_fanout`, `utf8_decoder`, `db::models::terminal::TerminalLog` | Contains `TerminalLogger` (2377 lines); CODEX_HOME RAII guard; PTY EOF detection via AtomicBool; G09-003/G09-004 TODOs for WS reconnect seq-resume |
| `launcher.rs` | Orchestrates terminal startup: DB lookup → session creation → SpawnCommand build → PTY spawn → bridge/watcher registration | `TerminalLauncher` (new, with_message_bus, launch_all, launch_terminal, stop_all), `LaunchResult` | Outbound: `process.rs::ProcessManager`, `bridge.rs::TerminalBridge`, `prompt_watcher.rs::PromptWatcher`, `cc_switch.rs::CCSwitchService`, `db::models::*`, `executors::*`; Callers: `server/routes/workflows.rs` | Full rollback logic on partial failure; cursor-agent mapped to "cursor" binary; two parallel spawn paths (TerminalLauncher vs direct from terminals.rs route) |
| `bridge.rs` | MessageBus → PTY stdin forwarding bridge | `TerminalBridge` (new, register, register_with_ready, unregister, active_count, is_registered) | Outbound: `process.rs::ProcessManager::get_handle`, `orchestrator::message_bus::BusMessage`; Callers: `launcher.rs`, `server/routes/terminals.rs`, `server/routes/workflows.rs` | `active_count()` has no production callers (tests only); dual-topic subscription (legacy PTY-session topic + new `terminal.input.<id>` topic); 1 MiB input size guard (E26-11) |
| `detector.rs` | Detects installed CLIs by running their version commands | `CliDetector` (new, detect_all, detect_single, is_available) re-exports `CliDetectionStatus`, `CliType` | Outbound: `db::models::CliType`, `tokio::process::Command`; Callers: `cli_health_monitor.rs`, `server/routes/cli_types.rs`, `executors::executors::mod`, `executors::profile` | Allowlist-guarded `detect_command` whitelist (injection prevention); `cursor-agent` in allowed list; `is_available` used by executors |
| `output_fanout.rs` | Single-reader PTY output → multi-subscriber broadcast with bounded replay buffer | `OutputFanout` (new, publish, latest_seq, subscribe), `OutputSubscription` (recv, last_seq), `OutputChunk`, `OutputFanoutConfig` | Outbound: `tokio::sync::broadcast`; Callers: `process.rs` (internal), `server/routes/terminal_ws.rs`, `server/routes/workflow_events.rs`, `orchestrator/prompt_handler.rs` | Two TODO annotations (G09-003, G09-004): WS reconnect seq-resume not implemented; config not runtime-tunable |
| `prompt_detector.rs` | Classifies PTY output lines into 6 prompt kinds using regex patterns | `PromptDetector` (new, with_buffer_size, process_line, detect, clear_buffer, has_dangerous_keywords), `DetectedPrompt`, `PromptKind`, `ArrowSelectOption`, `ARROW_UP`, `ARROW_DOWN`, `build_arrow_sequence` | Outbound: `once_cell`, `regex`, `serde`; Callers: `prompt_watcher.rs`, `orchestrator/types.rs`, `orchestrator/prompt_handler.rs`, `server/routes/workflow_events.rs` | ANSI escape stripping via `normalize_text_for_detection`; G07-009 TODO for password negative lookahead |
| `utf8_decoder.rs` | Streaming UTF-8 decoder for PTY byte chunks | `Utf8StreamDecoder` (new, decode_chunk, flush_lossy_tail, pending_tail_len, stats), `Utf8DecodeChunk`, `Utf8DecodeStats` | Callers: `process.rs` only (background PTY reader task) | Internal only; `Utf8DecodeStats` / `Utf8DecodeChunk` re-exported from `mod.rs` but no external callers found |

## Candidates

### C1 — `bridge.rs::active_count` (dead method)
- **Lines**: 295–297
- **Kind**: dead
- **Evidence**: Grep across all *.rs found 0 production callers. Only definition site.
- **Why**: The method is `pub` but nothing calls `.active_count()` outside of a potential test that was not found either.
- **Disposition**: delete (or make `#[cfg(test)]`)
- **Confidence**: medium (could be used by monitoring/metrics code not in Rust scope)
- **Blast radius**: none if deleted; method is 3 lines.

### C2 — `process.rs::spawn_pty` (legacy compatibility shim)
- **Lines**: 859–870
- **Kind**: legacy
- **Evidence**: All production callers (`runner/service.rs`, `server/routes/terminals.rs`, `launcher.rs`) call `spawn_pty_with_config`. `spawn_pty` is only called from integration/timeout tests.
- **Why**: G21-008 comment in the code itself labels it a "legacy signature preserved for tests". No production path uses it.
- **Disposition**: refactor — update test callers to use `spawn_pty_with_config` and delete, or gate with `#[cfg(test)]`
- **Confidence**: high
- **Blast radius**: `terminal_timeout_test.rs`, `terminal_lifecycle_test.rs`, `terminal_integration.rs` need updating

### C3 — `output_fanout.rs` G09-003/G09-004 TODO stubs
- **Lines**: 72–78 (OutputSubscription doc comment)
- **Kind**: stub
- **Evidence**: Two explicit TODO comments noting WS reconnect seq-resume is not implemented and replay buffer limits are not runtime-configurable.
- **Why**: Acknowledged incomplete features that have workarounds in place (replay buffer works, but reconnect seq is not plumbed from frontend).
- **Disposition**: investigate — determine if frontend tracks `last_seq` yet; if not, mark as BACKLOG
- **Confidence**: high (the TODOs are explicit)
- **Blast radius**: no code deletion risk; purely additive work

### C4 — `prompt_detector.rs` G07-009 TODO
- **Lines**: 215–219 (INPUT_FIELD_RE comment)
- **Kind**: stub
- **Evidence**: Comment notes that password negative lookahead should be moved into the regex instead of relying on a two-step check.
- **Why**: Minor technical debt; current behavior is correct but fragile.
- **Disposition**: refactor (low priority)
- **Confidence**: high
- **Blast radius**: only `detect_input()` method; tests cover the boundary

### C5 — Duplicate terminal spawn path (terminals.rs route vs. TerminalLauncher)
- **Lines**: `server/routes/terminals.rs` lines ~367–495 vs `launcher.rs::launch_terminal`
- **Kind**: redundant
- **Evidence**: Both `server/routes/terminals.rs` (direct REST endpoint) and `launcher.rs::launch_terminal` (used from `workflows.rs`) implement nearly identical spawn → logger attach → bridge register sequences. The REST route carries `// RUNNER_CLIENT_MIGRATION` markers suggesting it's a transitional path.
- **Why**: Two parallel paths for the same operation creates divergence risk when one is updated and the other is not.
- **Disposition**: investigate — determine which path is authoritative post-RunnerClient migration
- **Confidence**: medium
- **Blast radius**: touches `server/routes/terminals.rs` and `launcher.rs`; any consolidation must preserve DB state transitions

## Invisible Features

### F1 — CODEX_HOME RAII cleanup (process.rs)
- **What it does**: When spawning a Codex terminal, a temp directory is created at `%TEMP%\solodawn\<uuid>` and stored as `codex_home`. A RAII `CodexHomeGuard` ensures the directory is deleted even if spawn fails mid-way. On success, cleanup is deferred to `TrackedProcess::finalize_terminated_process`.
- **Seems used**: Yes — part of the active Codex CLI spawn path.
- **User visible**: No (transparent temp dir management).

### F2 — PTY EOF early-detection flag (process.rs, TrackedProcess::pty_eof)
- **What it does**: AtomicBool set by the background PTY reader task on EOF. Consulted by `is_running` and `list_running` to treat a terminal as stopped immediately when the PTY closes, before `child.try_wait()` catches up.
- **Seems used**: Yes — active invariant documented as [E26-05].
- **User visible**: No.

### F3 — Dual-topic bridge subscription (bridge.rs)
- **What it does**: Each bridge task subscribes to BOTH the new `terminal.input.<terminal_id>` topic AND the legacy `<pty_session_id>` topic for backward compatibility.
- **Seems used**: Yes — legacy path for `BusMessage::TerminalMessage`; new path for `BusMessage::TerminalInput` with strict session routing.
- **User visible**: No (internal bus routing).

### F4 — RUNNER_CLIENT_MIGRATION markers (terminals.rs, terminal_ws.rs)
- **What it does**: Multiple comments in server routes mark places where `process_manager()` calls should be replaced by a future `runner_client()` gRPC abstraction once the `runner` crate is fully integrated.
- **Seems used**: Migration is in progress; `crates/runner/src/service.rs` already calls `spawn_pty_with_config` directly.
- **User visible**: No.

### F5 — cursor-agent CLI support (launcher.rs, detector.rs)
- **What it does**: `launcher.rs::get_cli_command` maps `"cursor-agent"` → binary `"cursor"`. `detector.rs` allows `"cursor-agent"` and `"cursor"` in the detection whitelist.
- **Seems used**: Registered in the CLI type system; no evidence of active use in tests or workflow seeds.
- **User visible**: Potentially — if a user selects cursor-agent as a CLI type for a workflow terminal.
- **Note**: Relevant to G1 "open in external IDE/editor" deletion candidate — cursor runs as a CLI agent here, not an IDE launcher, so this is distinct from any external-editor feature.
