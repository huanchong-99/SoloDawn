# rs-smallcrates Census

Unit: rs-smallcrates  
Scope: crates/feishu-connector, crates/cc-switch, crates/runner, crates/local-deployment (incl container.rs 1665 lines), crates/tray, crates/deployment

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|---|---|---|---|---|
| **feishu-connector** | | | | |
| `crates/feishu-connector/src/lib.rs` | Re-exports all submodules | all 7 submodules pub | consumed by `crates/services` and `crates/server` | |
| `crates/feishu-connector/src/auth.rs` | Feishu tenant token cache + WS endpoint acquisition | `FeishuAuth::new`, `get_tenant_token`, `acquire_ws_endpoint`, `refresh_tenant_token` (priv) | called by `client.rs`, `messages.rs`; `FeishuAuth` shared via `Arc` | Double-checked locking via mutex prevents TOCTOU |
| `crates/feishu-connector/src/client.rs` | WebSocket connection loop, ping/pong, fragmented message reassembly | `FeishuClient::new`, `connect`, `auth()`, `is_connected`, `connected_flag()` | uses `auth.rs`, `events.rs`, `proto.rs`, `types.rs`; consumed by `services/src/services/feishu.rs` | `FragmentCache` (internal) handles multi-part frames with 5s TTL |
| `crates/feishu-connector/src/events.rs` | Event types and IM message parsing | `FeishuEvent`, `EventHeader`, `ReceivedMessage`, `EVENT_TYPE_MESSAGE`, `parse_message_event`, `parse_text_content` | used in `client.rs`, `services/feishu.rs`, `server/routes/feishu.rs` | |
| `crates/feishu-connector/src/messages.rs` | HTTP messaging API: send/reply text and cards, list chats | `FeishuMessenger::new`, `send_text`, `reply_text`, `first_bot_chat_id`, `send_card` | used by `services/feishu.rs`, `server/routes/concierge.rs`, `server/routes/planning_drafts.rs` | `send_card` has no callers outside the connector itself per grep (see candidates) |
| `crates/feishu-connector/src/proto.rs` | Protobuf frame wrappers + header key constants | `Frame`, `Header`, `METHOD_CONTROL`, `METHOD_DATA`, `HEADER_*`, `MSG_TYPE_*` | used by `client.rs` | Generated via `build.rs` + `pbbp2.proto` |
| `crates/feishu-connector/src/reconnect.rs` | Exponential-backoff reconnect policy with jitter | `ReconnectPolicy::new`, `next_delay`, `reset`, `update_config` | used by `server/src/main.rs` `start_feishu_connector` | |
| `crates/feishu-connector/src/types.rs` | Config and token structs | `FeishuConfig`, `WsEndpointResponse`, `WsEndpointData`, `ClientConfig`, `CachedToken` | used across auth, client, reconnect | |
| `crates/feishu-connector/proto/pbbp2.proto` | Feishu WS binary frame schema | `Header`, `Frame` messages | compiled by `build.rs` into `feishu.ws.rs` | |
| `crates/feishu-connector/build.rs` | Compile pbbp2.proto via prost-build | — | build-time only | |
| **cc-switch** | | | | |
| `crates/cc-switch/src/lib.rs` | CLI config switching library; defines `CliType` enum with 9 variants | `CliType` (pub enum), `parse`, `as_str`, `display_name`, `supports_config_switch` | re-exports all submodules; consumed by `crates/services/src/services/cc_switch.rs` | Variants Amp, CursorAgent, QwenCode, Copilot, Droid, Opencode are parsed but fall through to `UnsupportedCli` in `switch_model` |
| `crates/cc-switch/src/switcher.rs` | Unified `switch_model` and `ModelSwitcher` + `switch_models_sequential` | `switch_model`, `switch_models_sequential`, `ModelSwitcher`, `SwitchConfig` | calls `claude.rs`, `codex.rs`, `gemini.rs`; `ModelSwitcher` used by `services/cc_switch.rs` | `backup_before_switch` field is a no-op stub (comment: "not yet implemented") |
| `crates/cc-switch/src/claude.rs` | Read/write Claude Code `~/.claude/settings.json` | `ClaudeConfig`, `ClaudeEnvConfig`, `read_claude_config`, `write_claude_config`, `update_claude_model`, `*_from/*_to` variants | called by `switcher.rs`; `read_claude_config` imported directly by `services/cc_switch.rs` | |
| `crates/cc-switch/src/codex.rs` | Read/write Codex `~/.codex/auth.json` + `config.toml` | `CodexAuthConfig`, `CodexModelConfig`, `CodexProviderConfig`, `read_codex_auth/config`, `write_codex_auth/config`, `update_codex_model` | called by `switcher.rs` | |
| `crates/cc-switch/src/gemini.rs` | Read/write Gemini `~/.gemini/.env` | `parse_env_file`, `serialize_env_file`, `read_gemini_config`, `write_gemini_config`, `update_gemini_model` | called by `switcher.rs` | |
| `crates/cc-switch/src/config_path.rs` | Path helpers for all CLI configs | `get_home_dir`, `get_claude_config_dir`, `get_claude_settings_path`, `get_claude_mcp_path`, `get_codex_*`, `get_gemini_*`, `ensure_dir_exists`, `ensure_parent_dir_exists` | used by `claude.rs`, `codex.rs`, `gemini.rs`, `atomic_write.rs` | `get_claude_mcp_path` and `get_gemini_settings_path` have no callers outside the crate |
| `crates/cc-switch/src/atomic_write.rs` | Atomic temp-file-rename write for JSON/text | `atomic_write`, `atomic_write_json`, `atomic_write_text` | used by all three CLI config writers; Windows-specific fallback for non-atomic rename | |
| `crates/cc-switch/src/error.rs` | `CCSwitchError` enum + `Result` alias | `CCSwitchError`, `Result` | used throughout cc-switch | |
| **runner** | | | | |
| `crates/runner/src/main.rs` | Standalone gRPC binary entry point; binds `SOLODAWN_RUNNER_ADDR` (default :50051); graceful shutdown | `main`, `shutdown_signal` | uses `services::terminal::process::ProcessManager`; listens on `RunnerServiceServer` | BACKLOG-002: runner container separation; currently `RunnerClientImpl` has local-passthrough mode that skips gRPC |
| `crates/runner/src/lib.rs` | Re-exports `service` mod and generated proto types | `proto::runner_service_server::RunnerServiceServer`, `service::RunnerGrpcService` | used only by `main.rs` | |
| `crates/runner/src/service.rs` | gRPC `RunnerService` impl: spawn/kill/resize terminals, stream output, health | `RunnerGrpcService::new`; all `RunnerService` trait methods | delegates to `ProcessManager`; consumed via generated proto; caller is `RunnerClientImpl` in remote mode | `is_running`, `resize_terminal`, `write_input`, `stream_output`, `health` |
| `crates/runner/build.rs` | Compile `proto/runner.proto` via tonic-build | — | build-time | References `../../proto/runner.proto` |
| **local-deployment** | | | | |
| `crates/local-deployment/src/lib.rs` | `LocalDeployment` struct; implements `Deployment` trait; wires all services together at startup | `LocalDeployment` (pub struct + `Deployment` impl), `message_bus`, `prompt_watcher`, `cli_health_monitor`, `remote_client`, `get_login_status`, `store_oauth_handoff`, `take_oauth_handoff`, `reconcile_terminal_statuses` | implements `deployment::Deployment`; used by `server/src/main.rs`; owns `LocalContainerService`, `OrchestratorRuntime`, `ProcessManager`, `RunnerClientImpl` | `remote_client()` always returns Err (stub for remote deployment variant) |
| `crates/local-deployment/src/container.rs` (1665 lines) | Full `ContainerService` impl: workspace lifecycle (create/delete/ensure), executor spawning, process monitoring, file copy, diff streaming | `LocalContainerService`, `ContainerService` impl (`create`, `delete`, `ensure_container_exists`, `is_container_clean`, `start_execution_inner`, `stop_execution`, `stream_diff`, `try_commit_changes`, `kill_all_running_processes`); internal: `spawn_exit_monitor`, `spawn_os_exit_watcher`, `resolve_executor_env_vars`, `cleanup_workspace`, `create_workspace_config_files` | central implementation hub; uses `command.rs`, `copy.rs`; imports from `executors`, `services`, `db`, `deployment` | `resolve_executor_env_vars` handles ClaudeCode/Codex/Gemini only; other executor branches not handled |
| `crates/local-deployment/src/command.rs` | Unix process group kill helper | `kill_process_group` | called by `container.rs` | Unix-only: SIGINT→SIGTERM→SIGKILL; Windows: `child.kill()` only |
| `crates/local-deployment/src/copy.rs` | Glob-based file copy from project dir to workspace worktree | `copy_project_files_impl` (pub(crate)), `copy_single_file` (priv) | called by `container.rs::copy_project_files` | Path-traversal guard via canonicalize check; 8 unit tests |
| **tray** | | | | |
| `crates/tray/src/main.rs` | Windows system tray app; spawns/stops `solodawn-server.exe`; context menu; log rotation | `TrayApp`, `TrayAppHandler`, `main`; menu: Open, Start Server, Stop Server, Quit; Chinese locale detection | stand-alone binary; no Rust lib deps except tray-icon/winit/image/dotenv | G1-relevant: "Open SoloDawn" launches browser to `http://127.0.0.1:<port>` (not an external IDE); server auto-start on launch; 7-day log rotation |
| `crates/tray/build.rs` | Embed Windows icon resource via winresource | — | build-time Windows only | |
| **deployment** | | | | |
| `crates/deployment/src/lib.rs` | `Deployment` trait definition + `DeploymentError` enum; default method impls for analytics, PR monitor, auto project setup, SSE event streaming | `Deployment` trait (15 required methods + default impls), `DeploymentError`, `RemoteClientNotConfigured` | consumed by `server` (all routes via `D: Deployment`); implemented by `LocalDeployment` | Sole implementation is `LocalDeployment`; `RemoteClientNotConfigured` is exported but only returned by `LocalDeployment::remote_client` |

---

## Candidates

| Path | Lines | Kind | Evidence | Why | Disposition | Confidence | Blast Radius |
|---|---|---|---|---|---|---|---|
| `crates/cc-switch/src/switcher.rs` | 96-129 | stub | `backup_before_switch` field exists with comment "not yet implemented" (line 124); only callers are tests | `ModelSwitcher` is used in `services/cc_switch.rs` but `backup_before_switch` does nothing — the whole backup concept is a placeholder | refactor | high | Low: field is internal; just need to remove or implement |
| `crates/cc-switch/src/switcher.rs` | 83-94 | dead | `switch_models_sequential` has 0 callers outside the crate (grep confirms); comment says "注意：由于 cc-switch 修改全局环境变量，必须串行执行" but the service layer enforces serialization itself | Dead export | delete | medium | Low: no external callers found |
| `crates/cc-switch/src/config_path.rs` | 35-37 | dead | `get_claude_mcp_path` has 0 callers anywhere outside the crate | Exported utility with no production usage | delete | medium | Low: no callers |
| `crates/cc-switch/src/config_path.rs` | 85-87 | dead | `get_gemini_settings_path` has 0 callers anywhere (only `get_gemini_env_path` is used) | Exported utility with no production usage | delete | medium | Low: no callers |
| `crates/cc-switch/src/lib.rs` | 57-68 | stub | Variants `Amp`, `CursorAgent`, `QwenCode`, `Copilot`, `Droid`, `Opencode` are parsed but `switch_model` returns `UnsupportedCli` for all of them; `supports_config_switch` also returns false for all | These 6 enum variants exist in cc-switch for naming/display only; config switching is not implemented for them; note: executors crate has these as fully working executors | keep | medium | Low to remove from cc-switch::CliType; non-trivial if display-name needed |
| `crates/deployment/src/lib.rs` | 57 | legacy | `RemoteClientNotConfigured` struct is defined and exported but only `LocalDeployment::remote_client` returns it (always Err); no trait method uses it; no server route calls it | Vestige of a planned remote deployment path | investigate | low | Minimal: 1 usage in local-deployment |
| `crates/local-deployment/src/lib.rs` | 394-397 | stub | `remote_client()` always returns `Err(DeploymentError::Other(...))` — it is not part of the `Deployment` trait and has no callers (grep finds no call sites) | Placeholder for remote variant, never called | delete | medium | Low |
| `crates/feishu-connector/src/messages.rs` | 122-154 | dubious-feature | `send_card` method exists but has 0 callers outside feishu-connector itself (grep confirms no external call site invokes it) | Interactive card messaging is implemented but not used in any route or service | investigate | medium | Low: removing would break only the crate's own API surface |
| `crates/runner` | — | dubious-feature | The runner binary is BACKLOG-002 (container separation); `RunnerClientImpl` in `services` has a local-passthrough mode that bypasses gRPC entirely; `SOLODAWN_RUNNER_ADDR` defaults to `http://runner:50051` but is only used in "remote" mode | Runner gRPC path is not activated in local/default deployment; the binary exists but is effectively dark in production builds | investigate | medium | High if removed: the client-side infrastructure (`runner_client.rs`) and `Deployment` trait both reference `RunnerClientImpl` |

---

## Invisible Features

| Feature | What it does | Seems Used | User Visible |
|---|---|---|---|
| Feishu WebSocket connector | Long-connection WebSocket to Feishu IM platform; receives `im.message.receive_v1` events and dispatches them to concierge agent | Yes — `start_feishu_connector` is called from `server/main.rs` conditional on config | No (background integration) |
| Feishu fragment reassembly | `FragmentCache` in `client.rs` reassembles multi-part Feishu WS frames with 5-second TTL | Yes, active whenever WS messages exceed one frame | No |
| Runner gRPC binary (BACKLOG-002) | Standalone gRPC server for terminal process management; designed for container-separated deployment | Partially: `RunnerClientImpl` exists and is wired, but defaults to local passthrough; binary not started by tray | No (BACKLOG stub) |
| Tray auto-start + log rotation | `solodawn-tray` auto-starts the server on launch, logs to `logs/server.log`, rotates (deletes) logs older than 7 days | Yes in Windows installer deployment | Partially (tray icon visible) |
| Tray Chinese locale detection | `GetUserDefaultUILanguage` Win32 API call to show CN menu labels | Yes on Chinese Windows | Yes (menu labels) |
| `LocalDeployment::reconcile_terminal_statuses` | Resets stale terminal statuses (`starting/started/waiting/working/running/active`) to `not_started` on server startup | Yes — called in `LocalDeployment::new()` | No |
| `LocalDeployment::trigger_auto_project_setup` | On first launch (0 projects), auto-discovers up to 3 local git repos and creates projects | Active (called from server), controlled by `SOLODAWN_AUTO_SETUP_PROJECTS` env var | Partially visible (projects appear) |
| `deploy::Deployment::spawn_pr_monitor_service` | Background PR polling service started on server launch | Yes | No |
| Workspace CLAUDE.md/AGENTS.md synthesis | `create_workspace_config_files` in `container.rs` generates workspace-level import files from per-repo config files using `@import` syntax | Yes, called on `create` and `ensure_container_exists` | No (file system artifact) |
| Isolated CLAUDE_HOME / CODEX_HOME per workspace | `resolve_executor_env_vars` creates per-workspace copies of global CLI auth dirs to isolate concurrent executions | Yes, called on every `start_execution_inner` for ClaudeCode/Codex | No |
| cc-switch `supports_config_switch` | Method to gate which CLI types support config switching; returns true only for ClaudeCode/Codex/Gemini | Referenced in lib.rs but no external caller found via grep | No |
