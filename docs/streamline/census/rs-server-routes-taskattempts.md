# Census: rs-server-routes-taskattempts

**Unit:** rs-server-routes-taskattempts  
**Scope:** `crates/server/src/routes/task_attempts.rs` + `crates/server/src/routes/task_attempts/` (all submodules)  
**Branch:** refactor/streamline-quality-gates  
**Date:** 2026-06-14

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|---|---|---|---|---|
| `task_attempts.rs` | Root module — CRUD + WS + Git + editor + dev-server + planning handlers. Owns the router. | `get_task_attempts`, `get_workspace_count`, `get_task_attempt`, `update_workspace`, `create_task_attempt`, `run_agent_setup`, `stream_task_attempt_diff_ws`, `stream_workspaces_ws`, `merge_task_attempt`, `push_task_attempt_branch`, `force_push_task_attempt_branch`, `open_task_attempt_in_editor`, `get_task_attempt_branch_status`, `change_target_branch`, `rename_branch`, `rebase_task_attempt`, `abort_conflicts_task_attempt`, `start_dev_server`, `get_task_attempt_children`, `stop_task_attempt_execution`, `run_setup_script`, `run_cleanup_script`, `gh_cli_setup_handler`, `get_task_attempt_repos`, `search_workspace_files`, `get_first_user_message`, `delete_workspace`, `mark_seen`, `get_planning_messages` (private). Types: `RebaseTaskAttemptRequest`, `AbortConflictsRequest`, `GitOperationError`, `TaskAttemptQuery`, `DiffStreamQuery`, `WorkspaceStreamQuery`, `UpdateWorkspace`, `CreateTaskAttemptBody`, `WorkspaceRepoInput`, `RunAgentSetupRequest`, `RunAgentSetupResponse`, `MergeTaskAttemptRequest`, `PushTaskAttemptRequest`, `OpenEditorRequest`, `OpenEditorResponse`, `BranchStatus`, `RepoBranchStatus`, `ChangeTargetBranchRequest`, `ChangeTargetBranchResponse`, `RenameBranchRequest`, `RenameBranchResponse`, `RenameBranchError`, `PushError`, `RunScriptError`. Router fn: `router()`. | Imports: `db`, `executors`, `services`, `utils`, `deployment`. Calls into submodules: `cursor_setup`, `codex_setup`, `gh_cli_setup`, `pr`, `images`, `workspace_summary`. Called from: `crates/server/src/routes/mod.rs`. `restore_worktrees_to_process` from `util` used in `sessions/mod.rs`. | Contains G1 deletion target: `open_task_attempt_in_editor` + 4 supporting private fns + test module. `get_planning_messages` is private (not `pub async`). |
| `task_attempts/codex_setup.rs` | Builds and starts a Codex agent login/setup script chained with previous agent action. | `run_codex_setup(deployment, workspace, codex) -> Result<ExecutionProcess>` | Called only from `task_attempts.rs::run_agent_setup`. Uses `executors::command::CommandBuilder`, `db::ExecutionProcess`, `db::Session`, `services::ContainerService`. | Contains inline test module `command_escape_tests` (3 tests: escape, spaces in path, null byte rejection). Security: shell-escapes arguments via `shlex::try_quote`. |
| `task_attempts/cursor_setup.rs` | Installs Cursor CLI via `curl https://cursor.com/install` and chains a login script. Unix-only (non-unix returns `SetupHelperNotSupported`). | `run_cursor_setup(deployment, workspace) -> Result<ExecutionProcess>` | Called only from `task_attempts.rs::run_agent_setup`. | `#[cfg(unix)]` gating on both inner fn and imports. No tests. |
| `task_attempts/gh_cli_setup.rs` | Installs GitHub CLI via Homebrew and chains a `gh auth login` script. Unix-only. Error type `GhCliSetupError` exported. | `run_gh_cli_setup(deployment, workspace) -> Result<ExecutionProcess>`, `GhCliSetupError` (enum: `BrewMissing`, `SetupHelperNotSupported`, `Other { message }`) | Called from `task_attempts.rs::gh_cli_setup_handler`. `GhCliSetupError` re-used in handler response type. | `#[cfg(unix)]` gating. Checks for `brew` availability before attempting install. No tests. |
| `task_attempts/images.rs` | Upload image to task-attempt workspace, serve image from `.vibe-images/` dir, get image metadata. Has dedicated router for `/{id}/images/`. | `upload_image`, `get_image_metadata`, `serve_image` (HTTP handlers), `router(deployment)` | Mounted at `/{id}/images` in `task_attempts.rs::router()`. Uses `routes::images::{ImageMetadata, ImageResponse, process_image_upload}`. Middleware: `load_workspace_with_wildcard` (local, for wildcard path param). | Security: canonicalize + symlink checks to prevent path traversal (E29-08). 20MB upload limit. |
| `task_attempts/pr.rs` | PR creation (push + gh/az CLI), attach existing PR, fetch PR comments. Auto-generate PR description via follow-up coding agent. | `create_pr`, `attach_existing_pr`, `get_pr_comments` (handlers). Types: `CreatePrApiRequest`, `PrError`, `AttachPrResponse`, `AttachExistingPrRequest`, `PrCommentsResponse`, `GetPrCommentsError`, `GetPrCommentsQuery`, `DEFAULT_PR_DESCRIPTION_PROMPT` (const). | Called from `task_attempts.rs::router()`. Uses `finalize_workspace_if_all_repos_merged` from parent module. `trigger_pr_description_follow_up` is private. | `DEFAULT_PR_DESCRIPTION_PROMPT` hardcodes "SoloDawn" brand. PR description follow-up uses existing coding agent session. |
| `task_attempts/util.rs` | Utility: reset repository worktrees to the state before a given execution process (retry/rollback support). | `restore_worktrees_to_process(deployment, pool, workspace, target_process_id, perform_git_reset, force_when_dirty) -> Result<()>` | Called by `sessions/mod.rs::follow_up` (retry flow). Also referenced in `docs/developed/plans/XX-phase-0-backend-foundation.md`. No usage from task_attempts.rs itself. | G34-007: logs a warning when reconcile was needed but not applied (dirty worktree). |
| `task_attempts/workspace_summary.rs` | Batch summary endpoint: diff stats, pending approvals, dev-server status, PR status, unseen turns — all in one POST for list views. | `get_workspace_summaries` (handler). Types: `WorkspaceSummaryRequest`, `WorkspaceSummary`, `WorkspaceSummaryResponse`, `DiffStats`. | Mounted at `/task-attempts/summary`. Frontend: `frontend/src/components/ui-new/views/WorkspacesSidebar.tsx`, `useWorkspaces.ts`. `compute_workspace_diff_stats` is private. | Uses `buffer_unordered(8)` for parallel diff computation (G34-009). Bound to 1000 workspaces (E29-14). |

---

## Key Invisible Features

| Feature | What it does | Seems used |
|---|---|---|
| `open_task_attempt_in_editor` (G1 deletion target) | Resolves workspace+repo path, calls `EditorConfig::open_file_with_hint` to spawn local IDE or return `vscode://` remote URL. Tracks analytics `task_attempt_editor_opened`. | Yes — frontend `attemptsApi.openEditor` at `api.ts:796`, `useOpenInEditor.ts`. But entire feature slated for deletion per `docs/audit/R1-ide-editor-connection-deletion-audit.md`. |
| `get_planning_messages` (private fn) | Traces Workspace→Task→WorkflowTask→Workflow→PlanningDraft→Messages to surface planning conversation to workspace view. | Yes — `useWorkspacePlanningMessages` in `usePlanningDraft.ts`, rendered in `WorkspacesMainContainer.tsx`. Part of Orchestration workspace feature (AuditPlan System B adjacent). |
| `trigger_pr_description_follow_up` (private fn in pr.rs) | Fires a follow-up coding agent request to auto-improve PR title+description using configurable prompt. | Yes — gated by `auto_generate_description` flag in `CreatePrApiRequest`, user-facing in `CreatePRDialog.tsx`. |
| `stream_workspaces_ws` | WebSocket streaming of all workspaces (with archived/limit filters) via `events().stream_workspaces_raw`. | Yes — `useWorkspaces.ts` hooks. |
| `stream_task_attempt_diff_ws` | WebSocket streaming of git diff (stats or full) for a workspace with heartbeat. | Yes — diff panel in frontend. |
| `rename_branch` child propagation | After renaming, calls `WorkspaceRepo::update_target_branch_for_children_of_workspace` so child task attempts targeting the old branch follow the rename. | Yes — production logic, no test. |
| `finalize_workspace_if_all_repos_merged` | After any merge/attach-pr, checks if ALL repos are merged before marking task Done + archiving workspace. Includes rollback if archival fails. | Yes — called from `merge_task_attempt` and `pr::attach_existing_pr`. |

---

## Candidates for Keep/Cut

### G1 — open-editor handler and support fns (confirmed deletion target)

Lines 793–1044 of `task_attempts.rs`:
- `OpenEditorRequest` struct (L793–802)
- `OpenEditorResponse` struct (L804–807)
- `normalize_editor_repo_path` fn (L809–811)
- `resolve_workspace_repo_for_editor` fn (L813–837)
- `resolve_workspace_file_open_root` fn (L839–848)
- `resolve_workspace_file_path_for_editor` fn (L850–888)
- `open_editor_path_tests` test module (L890–946)
- `status_semantics_tests` test module (L948–963) — tests `push_rejected_response` and `rebase_conflict_response`; KEEP these two fns but the test module is independent of editor logic.
- `open_task_attempt_in_editor` handler (L965–1044)
- Route registration `.route("/open-editor", post(open_task_attempt_in_editor))` (L2143)
