# Census: rs-executors-claude

Unit: `crates/executors/src/executors/claude.rs` + `claude/` subdir (4 files)

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `executors/claude.rs` | Core Claude Code executor: spawn/follow-up/log-normalize; contains all JSON parsing types for Claude CLI output | `ClaudeCode` (struct, TS+JsonSchema exported), `ClaudeLogProcessor`, `HistoryStrategy`, `ClaudeJson`, `ClaudeToolData`, `ClaudeMessage`, `ClaudeContentItem`, `ClaudeStreamEvent`, `ClaudeContentBlockDelta`, `ClaudeUsage`, `ClaudeMessageDelta`, `ClaudeTodoItem`, `ClaudeEditItem` | Implements `StandardCodingAgentExecutor` trait (mod.rs); consumed by `amp.rs` (re-uses `ClaudeLogProcessor`/`HistoryStrategy`), `qa_mock.rs`, `generate_types.rs` (TS/JSON schema generation) | Hardcodes `@anthropic-ai/claude-code@2.1.2` and `@musistudio/claude-code-router@1.0.66` |
| `executors/claude/client.rs` | Approval + hook-callback handler wrapping the `ExecutorApprovalService` | `ClaudeAgentClient::new`, `on_can_use_tool`, `on_hook_callback`, `on_non_control`; const `AUTO_APPROVE_CALLBACK_ID` | Used exclusively inside `claude.rs` `spawn_internal`; `AUTO_APPROVE_CALLBACK_ID` re-exported to `claude.rs` | `auto_approve` path bypasses all approval logic when no `approvals_service` is present |
| `executors/claude/protocol.rs` | Bidirectional JSON-line stdio control protocol between SoloDawn and the Claude CLI process | `ProtocolPeer::spawn`, `initialize`, `send_user_message`, `set_permission_mode`, `interrupt`, `send_hook_response` | Spawns a tokio read-loop; calls `ClaudeAgentClient`; called only from `claude.rs::spawn_internal` | Handles interrupt via oneshot channel; dispatches `CanUseTool` and `HookCallback` control requests |
| `executors/claude/types.rs` | Protocol type definitions for the Claude SDK control protocol | `CLIMessage`, `SDKControlRequest`, `ControlResponseMessage`, `ControlRequestType`, `PermissionResult`, `PermissionUpdate`, `PermissionUpdateType`, `PermissionUpdateDestination`, `ControlResponseType`, `Message`, `ClaudeUserMessage`, `SDKControlRequestType`, `PermissionMode` | Used by `protocol.rs` and `client.rs`; `PermissionMode` re-exported and used in `claude.rs` | `AcceptEdits` variant of `PermissionMode` exists but is never set by any current code path |
