# Census Gap-Fill: Batch 12

Files covered: 12 (gapfill pass)

---

## crates/services/src/services/qa_repos.rs

**Purpose:** QA-mode-only helper that maintains two hardcoded GitHub repositories (`BloopAI/internal-qa-1`, `BloopAI/internal-qa-2`) in a persistent temp directory and returns them as `DirectoryEntry` objects in place of normal filesystem discovery.

**Public surface:** `pub fn get_qa_repos() -> Result<Vec<DirectoryEntry>, FilesystemError>`

**Relations:** Called from `filesystem.rs` at two call sites (lines 275, 346) inside branches that check the `qa-mode` Cargo feature. The module itself is gated `#[cfg(feature = "qa-mode")]` in `mod.rs`.

**Candidates:** None. Properly feature-gated, well-scoped.

---

## crates/services/src/services/queued_message.rs

**Purpose:** In-memory DashMap-backed store that queues exactly one follow-up message per session. Consumed by the session finalization flow and exposed to the frontend via `/sessions/{id}/queue` routes.

**Public surface:** `QueuedMessage`, `QueueStatus`, `QueuedMessageService` (with `queue_message`, `cancel_queued`, `get_queued`, `take_queued`, `has_queued`, `get_status`).

**Relations:** Instantiated in `local-deployment`; used in `server/routes/sessions/queue.rs`, `server/routes/scratch.rs`, and `local-deployment/src/container.rs` (`take_queued` in finalization). Types exported to TS via `generate_types.rs`.

**Candidates:** None. Actively used.

---

## crates/services/src/services/repo.rs

**Purpose:** Thin service wrapper over `db::models::repo::Repo` for validating git repo paths, registering repos in the database, and initializing new git repos with a `main` branch. Also exposes `RepoError` consumed by the HTTP layer.

**Public surface:** `RepoService` (with `validate_git_repo_path`, `normalize_path`, `register`, `find_by_id`, `get_by_id`, `init_repo`), `RepoError`, `Result<T>`.

**Relations:** Used by `deployment`, `local-deployment`, `services/project.rs`, and `server/routes/repo.rs`. `RepoError` has a full `From` impl in `server/src/error.rs`.

**Candidates:** None. Core CRUD service.

---

## crates/services/src/services/runner_client.rs

**Purpose:** Abstraction layer for terminal process management. Provides `RunnerClient` trait with `LocalRunner` (in-process via `ProcessManager`) and `RemoteRunner` (gRPC via tonic) implementations, unified by `RunnerClientImpl` enum. `from_env()` selects the variant via `SOLODAWN_RUNNER_MODE`.

**Public surface:** `RunnerClient` trait, `LocalRunner`, `RemoteRunner`, `RunnerClientImpl`, `SharedRunnerClient` type alias, `SpawnResult`, `TerminalSpawnConfig`, `RunnerHealth`.

**Relations:** Used from `local-deployment` and `server/routes/terminal_ws.rs` and `server/routes/terminals.rs`. `RemoteRunner` is a real gRPC client backed by `solodawn.runner` proto.

**Candidates:** `RemoteRunner` — plausible future path but currently `SOLODAWN_RUNNER_MODE=remote` is not documented as production-ready (env var also accepts a legacy compat alias `GITCORTEX_RUNNER_MODE`). The infrastructure to test or deploy remote mode is not evident in the repo. **Confidence: low** — `from_env()` is shipped and wired, so this is a live code path, just an untested deployment mode.

---

## crates/services/src/services/share.rs

**Purpose:** Facade that re-exports `ShareConfig`, `SharePublisher`, `SharedTaskDetails` from three submodules (`config`, `publisher`, `status`) and defines `ShareError`.

**Public surface:** `ShareConfig`, `SharePublisher`, `SharedTaskDetails`, `ShareError`.

**Relations:** NOT registered in `services/src/services/mod.rs` — no `pub mod share` or `mod share` anywhere in the crate. No callers found anywhere. The submodule files it declares (`mod config; mod publisher; mod status;`) do not exist on disk (no `share/` directory). **The file is completely orphaned: it is never compiled by Cargo.**

**Candidates:**
- `share.rs` — **dead/orphaned**. The module is not declared anywhere, its internal `mod` declarations reference non-existent files, and there are zero callers. Disposition: **delete**. Confidence: **high**.

---

## crates/services/src/services/template_renderer.rs

**Purpose:** Handlebars-based template renderer for slash command prompts. Renders `{{variable}}` templates from custom JSON params and a `WorkflowContext`. Strict mode enabled; HTML escaping disabled.

**Public surface:** `TemplateRenderer` (with `render`), `WorkflowContext` (with `new`, `with_current_task`), `TemplateContext` (with `new`, `with_params`, `with_workflow`).

**Relations:** Used in `services/src/services/orchestrator/agent.rs` for slash-command prompt rendering, and tested in `server/tests/slash_commands_integration_test.rs`. `TemplateContext` is a helper for tests and validation only.

**Candidates:** `TemplateContext` — used only in tests within the file; no external callers found outside `template_renderer.rs` itself. Could be considered a stub. **Confidence: low** — it may be intentionally kept for downstream tests.

---

## crates/services/src/services/workspace_manager.rs

**Purpose:** Orchestrates multi-repo worktree workspaces. Creates per-repo worktrees under a container directory, handles rollback on partial failure, cold-restart recovery (including legacy single-worktree layout migration), and orphan workspace cleanup.

**Public surface:** `WorkspaceManager` (static methods: `create_workspace`, `ensure_workspace_exists`, `ensure_workspace_exists_with_recovery`, `cleanup_workspace`, `get_workspace_base_dir`, `migrate_legacy_worktree`, `cleanup_orphan_workspaces`), `RepoWorkspaceInput`, `WorktreeContainer`, `RepoWorktree`, `WorkspaceError`.

**Relations:** Used in `services/container.rs`, `server/routes/task_attempts.rs`, `server/routes/tasks.rs`, and `local-deployment/src/container.rs`.

**Candidates:** `migrate_legacy_worktree` — migration path for a layout that predates the current multi-repo workspace structure. Once all installations have migrated, this function and the detection logic inside `ensure_workspace_exists` / `ensure_workspace_exists_with_recovery` can be removed. **Confidence: medium** (removal timing depends on deployment fleet).

---

## crates/services/src/services/worktree_manager.rs

**Purpose:** Low-level git worktree lifecycle management. Provides synchronized creation (global `LruCache<String, Mutex>` per path), comprehensive cleanup (CLI + libgit2 + metadata removal), shallow-clone detection/unshallowing, branch collision avoidance, and legacy worktree inference.

**Public surface:** `WorktreeManager` (static methods), `WorktreeCleanup`, `WorktreeError`. Also contains one inline `#[tokio::test]` regression test (`create_worktree_when_repo_path_is_a_worktree`).

**Relations:** Used by `workspace_manager.rs`, `services/container.rs`, `server/routes/workflows.rs`, `deployment`, and the error type is re-exported in `server/src/error.rs`.

**Candidates:**
- `comprehensive_worktree_cleanup` (sync, line 348) — this private function is only called from `comprehensive_worktree_cleanup_async`. It could be inlined but is not harmful. **Confidence: low**.
- The `#[tokio::test]` at the bottom (line 708) is an inline integration test, not a unit test in `tests/` — unusual placement but not dead code.

---

## crates/services/test_edge_cases.rs

**Purpose:** Manual test harness for `services::git_watcher::commit_parser::parse_commit_metadata`. Contains a `main()` function that exercises 7 edge cases (empty input, separator-only, multiple separators, empty fields, Unicode, long messages, extra whitespace).

**Public surface:** `fn main()`.

**Relations:** NOT listed in `Cargo.toml` as a `[[bin]]` or `[[test]]` or `[[example]]` target. The file sits at `crates/services/test_edge_cases.rs` but is never compiled by Cargo. It is an orphaned development scratch file.

**Candidates:**
- `test_edge_cases.rs` — **dead (orphaned scratch binary)**. Not wired into Cargo, never compiled. The edge cases it tests would be better as `#[test]` items in `git_watcher/commit_parser.rs` or its test suite. Disposition: **delete** (or move coverage into proper tests). Confidence: **high**.

---

## frontend/src/components/CliInstallProgress.tsx

**Purpose:** Terminal-style React component that displays real-time WebSocket log output for CLI install/uninstall jobs. Auto-scrolls, shows per-line color coding (stdout/stderr/error/completed), shows exit code, and fires `onComplete` callback exactly once.

**Public surface:** `CliInstallProgress` (named export), `CliInstallProgressProps` (interface).

**Relations:** Imports `useCliInstallProgress` hook. **No other TSX or TS file in the codebase imports `CliInstallProgress`** — it is exported but never consumed.

**Candidates:**
- `CliInstallProgress` component — **dead (unused export)**. No consumer found anywhere in the frontend. The underlying `useCliInstallProgress` hook exists and is presumably wired to backend WebSocket, but the UI component itself has no mount site. Disposition: **investigate** (may be planned for a CLI settings panel). Confidence: **medium**.

---

## frontend/src/components/ConfigProvider.tsx

**Purpose:** Global React context provider (`UserSystemProvider`) for config, environment, executor profiles, agent capabilities, login status, and remote-features flag. Exposes `useUserSystem()` hook. Handles optimistic `updateConfig`, `saveConfig`, `updateAndSaveConfig`, and `reloadSystem`. Also syncs i18n language from config.

**Public surface:** `UserSystemProvider`, `useUserSystem` hook, `UserSystemContextType` (implicit), `UserSystemState` (implicit).

**Relations:** Consumed by 46 files across the frontend (settings pages, workflow wizard, task cards, dialogs, layout components, `App.tsx`). Central to the application.

**Candidates:** None. Core provider.

---

## frontend/src/components/DiffViewSwitch.tsx

**Purpose:** Toolbar control for diff display preferences: unified/split view mode toggle and two multi-select toggles for ignore-whitespace and wrap-text. Reads/writes `useDiffViewStore` Zustand store. Internationalized via `react-i18next`.

**Public surface:** `DiffViewSwitch` (default export), `Props` type (local).

**Relations:** Imported by `frontend/src/components/panels/DiffsPanel.tsx`. `useDiffViewStore` is also consumed by `DiffCard.tsx`, `DiffViewCardWithComments`, and `WorkspaceContext.tsx`.

**Candidates:** None. Actively used in `DiffsPanel`.

---

## Summary of Dead/Orphaned Candidates

| File | Kind | Confidence | Disposition |
|------|------|------------|-------------|
| `crates/services/src/services/share.rs` | dead (not in module tree, submodules missing) | high | delete |
| `crates/services/test_edge_cases.rs` | dead (orphaned, not in Cargo.toml) | high | delete |
| `frontend/src/components/CliInstallProgress.tsx` | dead (no import site found) | medium | investigate |
| `WorkspaceManager::migrate_legacy_worktree` | legacy migration path | medium | keep until fleet migrates |
