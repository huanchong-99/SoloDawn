# Gap-Fill Census: crates/services/src/services — 12 files

Unit: gapfill
Date: 2026-06-14
Analyst: Claude (Sonnet 4.6)

Tool notes: fast-context MCP returned resource_exhausted on the cross-file usage query; all cross-file analysis was done with Grep.

---

## Files Covered

1. `crates/services/src/services/filesystem.rs`
2. `crates/services/src/services/filesystem_watcher.rs`
3. `crates/services/src/services/image.rs`
4. `crates/services/src/services/mod.rs`
5. `crates/services/src/services/notification.rs`
6. `crates/services/src/services/oauth_credentials.rs`
7. `crates/services/src/services/orchestrator/resilient_llm.rs`
8. `crates/services/src/services/orchestrator/runtime_actions.rs`
9. `crates/services/src/services/orchestrator/runtime_test.rs`
10. `crates/services/src/services/orchestrator/terminal_coordinator_test.rs`
11. `crates/services/src/services/pr_monitor.rs`
12. `crates/services/src/services/project.rs`

---

## Module Map

### filesystem.rs
- **Purpose**: Sandboxed filesystem browsing. Enumerates directories and Git repos within allowed roots.
- **Public surface**: `FilesystemService`, `DirectoryListResponse`, `DirectoryEntry`, `FilesystemError`
- **Key methods**: `new()`, `new_with_roots()`, `list_directory()`, `list_directory_async()`, `list_git_repos()`, `list_common_git_repos()`
- **Relations**: Used by `crates/local-deployment/src/lib.rs`, `crates/deployment/src/lib.rs`, `crates/server/src/routes/filesystem.rs`, `crates/executors/src/executors/claude.rs`

### filesystem_watcher.rs
- **Purpose**: Gitignore-aware filesystem watcher using `notify-debouncer-full`. Adapts recursive/non-recursive mode per platform.
- **Public surface**: `async_watcher()` (returns `WatcherComponents`), `ALWAYS_SKIP_DIRS`
- **Relations**: `async_watcher` consumed by `diff_stream.rs`; `ALWAYS_SKIP_DIRS` reused in `git/cli.rs`

### image.rs
- **Purpose**: Image upload, dedup (SHA-256), caching to disk, copy-to-worktree.
- **Public surface**: `ImageService`, `ImageError`
- **Key methods**: `store_image()`, `get_image()`, `delete_image()`, `delete_orphaned_images()`, `copy_images_by_task_to_worktree()`, `copy_images_by_ids_to_worktree()`
- **Relations**: Constructed in `local-deployment/container.rs`; used in `server/routes/task_attempts/images.rs`, `server/routes/images.rs`

### mod.rs
- **Purpose**: Re-exports all service submodules.
- **Notes**: `qa_repos` gated behind `#[cfg(feature = "qa-mode")]`

### notification.rs
- **Purpose**: Cross-platform sound + push notifications (macOS afplay/osascript, Linux paplay/notify-rust, Windows/WSL2 PowerShell).
- **Public surface**: `NotificationService`
- **Relations**: Used in `container.rs`, `approvals/executor_approvals.rs`, `local-deployment/container.rs`

### oauth_credentials.rs
- **Purpose**: JWT token storage. File backend (all platforms) + macOS Keychain backend. Backend selected via `OAUTH_CREDENTIALS_BACKEND` env var.
- **Public surface**: `OAuthCredentials`, `Credentials`
- **Relations**: Used in `auth.rs`, `local-deployment/lib.rs`
- **Note**: Only stores `refresh_token` on disk (never the short-lived access token).

### orchestrator/resilient_llm.rs
- **Purpose**: Multi-provider LLM client with circuit breaking (threshold=5 failures) and round-robin failover. Tracks per-provider health, emits `ProviderEvent` on switch/exhaust/recover.
- **Public surface**: `ResilientLLMClient`, `ProviderEvent`, `ProviderStatusReport` (all re-exported from `orchestrator/mod.rs`)
- **Relations**: Registered as `LLMClient` implementor; wired in `orchestrator/llm.rs` and `orchestrator/mod.rs`

### orchestrator/runtime_actions.rs
- **Purpose**: Database + PTY lifecycle operations for dynamic orchestration: create tasks, create terminals, start/stop/relaunch terminals.
- **Public surface**: `RuntimeActionService`, `RuntimeTaskSpec`, `RuntimeTerminalSpec`, `StartTerminalOutcome` (re-exported from `orchestrator/mod.rs`)
- **Relations**: Called from `orchestrator/agent.rs`, `orchestrator/runtime.rs`, `orchestrator/tests.rs`, `server/routes/workflows.rs`, `local-deployment/lib.rs`
- **Notable**: Defines its own `STARTABLE_TERMINAL_STATUSES` constant (4 statuses: not_started, failed, cancelled, waiting) that conflicts in name with the one in `constants.rs` (1 status: waiting). The runtime_actions version is a module-private const used only for `try_start_terminal`'s guard check.

### orchestrator/runtime_test.rs
- **Purpose**: Integration tests for `OrchestratorRuntime`.
- **Relations**: NOT declared as a module in `orchestrator/mod.rs` — there is no `mod runtime_test;` declaration. The file exists on disk but is unreachable to the compiler.
- **Status**: Dead/orphaned test file. The Workflow struct literals lack the `audit_plan` field present in the current struct definition, confirming staleness.

### orchestrator/terminal_coordinator_test.rs
- **Purpose**: Integration tests for `TerminalCoordinator`.
- **Relations**: Declared via `#[cfg(test)] mod terminal_coordinator_test;` in `orchestrator/mod.rs`. Helper functions `setup_test_db`, `create_cli_and_model`, `create_workflow_with_terminals` all carry `#[allow(dead_code)]` — indicating they may be unused inside the test module or were formerly used by tests that were removed.

### pr_monitor.rs
- **Purpose**: Background polling service (60s interval) that checks open PR statuses via `GitHostService` and marks tasks as Done + archives workspaces when a PR merges.
- **Public surface**: `PrMonitorService::spawn()` (fire-and-forget `JoinHandle`)
- **Relations**: Spawned in `deployment/lib.rs` only. Not present in local-deployment path.

### project.rs
- **Purpose**: Project CRUD service (create/update/link-remote/unlink/repository management/delete) plus cross-repo file search via `FileSearchCache`.
- **Public surface**: `ProjectService`, `ProjectServiceError`, `Result<T>`
- **Relations**: Used in `server/routes/projects.rs`, `local-deployment/lib.rs`, `deployment/lib.rs`

---

## Dead / Deprecated / Dubious Candidates

### 1. `runtime_test.rs` — orphaned test file (dead)
- No `mod runtime_test;` in `orchestrator/mod.rs` or anywhere else.
- Struct literals reference a stale `Workflow` schema (missing `audit_plan`, `orchestrator_api_type` etc. that are now in the model).
- **Disposition**: delete or restore+fix

### 2. `runtime_actions.rs` — duplicate `STARTABLE_TERMINAL_STATUSES` const (dubious)
- Line 40 defines `const STARTABLE_TERMINAL_STATUSES: [&str; 4] = ["not_started", "failed", "cancelled", "waiting"]`
- `constants.rs` exports `pub const STARTABLE_TERMINAL_STATUSES: &[&str] = &["waiting"]` with opposing semantics (only "waiting" is startable for orchestrator dispatch; the runtime_actions version handles re-start from failed/cancelled too).
- The local const shadows the global one. Same name, completely different semantics — a reader could confuse them.
- **Disposition**: refactor — rename the runtime_actions local to e.g. `RESTARTABLE_TERMINAL_STATUSES` to make the intent explicit

### 3. `terminal_coordinator_test.rs` — `#[allow(dead_code)]` on all three helpers
- `setup_test_db`, `create_cli_and_model`, `create_workflow_with_terminals` all carry `#[allow(dead_code)]`.
- `create_cli_and_model` is referenced only via its return value inside `create_workflow_with_terminals`, which itself is called from every test — so the suppression on `setup_test_db` and `create_workflow_with_terminals` is due to `#[allow(dead_code)]` being applied defensively before the tests were fleshed out.
- **Confidence**: low — the helpers ARE used; the suppressions appear pre-emptive rather than indicating true dead code.

### 4. `notification.rs` — `send_notification` private method
- `send_notification` is `async fn` called only from `notify()`. It delegates to `play_sound_notification` and `send_push_notification`. This indirection adds no reuse value and could be inlined into `notify()`.
- **Disposition**: investigate (minor simplification opportunity, not dead code)

---

## Uncertainties

- Whether `runtime_test.rs` was intentionally excluded from compilation (e.g., blocked pending schema updates) or was accidentally orphaned. The file is present in git, and its most recent commit context is unclear.
- Whether the semantic difference between the two `STARTABLE_TERMINAL_STATUSES` constants is intentional design (the orchestrator dispatches only to "waiting" while restart/admin paths accept "failed"/"cancelled") or a naming oversight.
- `PrMonitorService` is only used in `deployment/lib.rs` (not `local-deployment`). Whether this is an intentional feature split or an omission requires checking product requirements.
