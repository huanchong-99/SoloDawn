# Census: rs-services-git-ccswitch

Unit covers three files in `crates/services/src/services/`:
- `git.rs` (2071 lines) — GitService and GitServiceError
- `git/cli.rs` (960 lines) — GitCli and supporting types
- `cc_switch.rs` (1686 lines) — CCSwitchService, CCSwitch trait

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `git.rs` | High-level git operations for task execution: worktrees, diffs, branches, merges, rebases, commits, remote ops | `GitService`, `GitServiceError`, `ConflictOp`, `GitBranch`, `HeadInfo`, `Commit`, `WorktreeResetOptions`, `WorktreeResetOutcome`, `DiffTarget`; re-exports `GitCli`, `GitCliError` | Calls `git/cli.rs` (GitCli) for destructive/working-tree ops; calls `utils::diff`; called by server routes (`task_attempts`, `workflows`, `git`, `repo`, `sessions/review`), `worktree_manager`, `repo`, `diff_stream`, `file_ranker`, `merge_coordinator`, `container` | libgit2 used for read-only graph queries and in-memory merges; CLI used for all working-tree mutations. `#[cfg(feature = "cloud")]` gates `clone_repository`. |
| `git/cli.rs` | Subprocess wrapper around the `git` CLI binary: worktree ops, staging, commits, status, diff-status, rebase, merge/cherry-pick/revert state, push/fetch, remote queries | `GitCli`, `GitCliError`, `ChangeType`, `StatusDiffEntry`, `WorktreeEntry`, `StatusDiffOptions`, `StatusEntry`, `WorktreeStatus`; pub methods: `git()`, `worktree_add/remove/move/prune`, `diff_status`, `get_worktree_status`, `add_all`, `list_worktrees`, `commit`, `merge_base`, `rebase_onto`, `is_rebase/merge/cherry_pick/revert_in_progress`, `abort_rebase/merge/cherry_pick/revert`, `quit_rebase`, `get_conflicted_files`, `has_staged_changes`, `merge_squash_commit`, `update_ref`, `push`, `fetch_with_refspec`, `check_remote_branch_exists`, `get_remote_url` | Used exclusively by `git.rs` (GitService delegates to it) and by `task_attempts/pr.rs`, `worktree_manager.rs` directly | All destructive working-tree ops go through here. Pathspec excludes use `filesystem_watcher::ALWAYS_SKIP_DIRS`. |
| `cc_switch.rs` | CLI model-configuration switching for AI terminal processes (Claude Code, Codex, Gemini): creates isolated home dirs, config files, env vars. Phase 23 replaced global-file writes with per-process env-var injection. | `CCSwitch` (trait with `switch_for_terminal`), `CCSwitchService` (struct); pub methods: `new()`, `build_launch_config()`, `switch_for_terminal()` (deprecated, via CCSwitch impl), `switch_for_terminals()` (deprecated), `detect_cli()`; re-exported via `services::mod.rs` | Calls `cc-switch` crate (`ModelSwitcher`, `SwitchConfig`, `read_claude_config`); DB queries for `CliType`, `ModelConfig`, `Workflow`, `Terminal`; imports `terminal::process::{SpawnCommand, SpawnEnv}`; used by `terminal/launcher.rs`, `server/routes/terminals.rs`, `server/routes/workflows.rs`, `orchestrator/runtime_actions.rs`; tests in `services/tests/terminal_integration.rs`, `terminal_binding_test.rs`, `terminal_lifecycle_test.rs` | `switch_for_terminal` / `switch_for_terminals` are deprecated in favor of `build_launch_config`. |

---

## Key Private Helpers (git.rs — all file-local, no external callers)

| Helper | Lines | Note |
|--------|-------|------|
| `ensure_cli_commit_identity` | 170–185 | Sets git user.name/email in repo config if missing before CLI commits |
| `signature_with_fallback` | 188–194 | libgit2 commit signature with "SoloDawn" fallback |
| `default_remote_name` | 196–209 | Reads `remote.pushDefault` or first remote, defaults to "origin" |
| `blob_to_string` | 567–575 | Skips binary blobs |
| `read_file_to_string` | 578–615 | Reads from filesystem with size/binary/UTF-8 guards |
| `create_file_details` | 618–643 | Blob-first with filesystem fallback, used in diff building |
| `status_entry_to_diff` | 647–777 | Converts GitCli StatusDiffEntry to Diff struct |
| `find_checkout_path_for_branch` | 780–797 | Detects if a branch is checked out in any worktree |
| `convert_diff_to_file_diffs` | 423–556 | libgit2 Diff -> Vec<Diff>; includes MAX_INLINE_DIFF_BYTES guard |
| `check_worktree_clean` | 982–1025 | Checks tracked-file status; raises WorktreeDirty |
| `get_branch_status_inner` | 898–913 | graph_ahead_behind wrapper |
| `perform_squash_merge` | 1428–1486 | In-memory libgit2 merge, conflict detection, squash commit |
| `get_all_branches_libgit2` | 1269–1341 | Primary branch list via libgit2 |
| `get_all_branches_via_cli` | 1343–1425 | Fallback branch list via CLI when libgit2 fails |
| `fetch_from_remote` | 1898–1914 | Delegates to GitCli.fetch_with_refspec |
| `fetch_branch_from_remote` | 1917–1931 | Fetches single remote tracking branch |
| `fetch_all_from_remote` | 1934–1939 | Fetches all from remote |

---

## Deprecated / Legacy Methods

| Method | File | Lines | Status |
|--------|------|-------|--------|
| `switch_for_terminal` (CCSwitch impl) | cc_switch.rs | 606–678 | Deprecated; writes to global config files; `#[allow(deprecated)]` in impl; only caller is `switch_for_terminals` (also deprecated) |
| `switch_for_terminals` | cc_switch.rs | 1247–1253 | `#[deprecated(since="0.2.0")]`; only callers are its own unit tests |
| `get_current_branch` | git.rs | 1050–1057 | Comment "Thin wrapper for backward compatibility" over `get_head_info`; called by `get_all_branches_libgit2` and `server/routes/git.rs` |

---

## Feature-Gated / Invisible Capabilities

| Feature | Where | What it does | Seems Used | Note |
|---------|-------|-------------|-----------|------|
| `clone_repository` | git.rs:1942–1995 | `#[cfg(feature = "cloud")]` libgit2 repo clone with token or SSH-agent auth | Unknown: no non-cloud callers found | Only present when `cloud` feature is enabled. Zero callers found in the non-cloud code. |
| `__SOLODAWN_NATIVE_AUTH` env key | cc_switch.rs:977 | Internal signal injected into SpawnEnv.set to indicate OAuth credentials path; stripped before process spawn | Active: removed at line 1218 before the final SpawnCommand is built | Not a leaking env var; used as in-process signal only. |
| Codex `wire_api` field | cc_switch.rs:110 | Reads `SOLODAWN_CODEX_WIRE_API`/`GITCORTEX_CODEX_WIRE_API` env var to set `responses` vs `codex` protocol | Partially: defaults to "responses"; env override available | Invisible to users unless they know the env var. |

---

## Open TODOs Embedded in Code

| Tag | File | Line | Content |
|-----|------|------|---------|
| G19-006 | cc_switch.rs | 800 | CLAUDE_HOME/GEMINI_HOME not cleaned up after terminal lifecycle end |
| G22-005 | cc_switch.rs | 805 | No unified TempDirGuard for all isolation dirs |
| G22-006 | cc_switch.rs | 807 | Windows ACL: temp dir permissions cannot be set via Unix chmod |
| G22-010 | cc_switch.rs | 91 | Codex: `api_key` field in config.toml duplicates env var; unclear precedence |
| G22-002 | cc_switch.rs | 600 | `switch_for_terminal` is not safe for concurrent use; TODO compile-time gate |
