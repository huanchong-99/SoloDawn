# Q2 — Audit-Surfacing Findings & Fix Spec

**Question:** In the orchestration workspace (`executionMode=agent_planned`, `/workspaces`), the user keeps the Vibe-Kanban review tools (Preview dev-server, Diff/Changes view, per-line Comment) but they NEVER auto-surface during/after an orchestration run. The user cannot SEE or AUDIT what the AI produced. A design to "map produced changes to the frontend" did not take effect.

**Verdict (summary):** KEEP Preview/Diff/Comment. The feature was never built (not flag-gated, not deleted). Fix = (A) a new **workflow-task-scoped diff source** that reuses `GitService::get_diffs(DiffTarget::Branch{...})` to diff `task.branch` vs `workflow.target_branch` directly (no new worktree needed), (B) **persist + emit the acceptance review SCORE**, (C) **render + auto-open** a per-task Changes/Audit surface inside the orchestration workspace (concierge `RightSidebar` + `WorkspacesLayout`), driven by `terminal.completed` / `quality.gate_result` WS events.

Repo: `E:\SoloDawn`, verified against `main @ a014511c2`.

---

## 1. ROOT CAUSE — the precise missing / dead / mis-wired links

There are **two disjoint entity graphs** and the bridge between them was never built:

| | Orchestration (agent_planned) | Diff / Review (Vibe-Kanban) |
|---|---|---|
| Graph | `Workflow → WorkflowTask → Terminal` | `Workspace → WorkspaceRepo → worktree` |
| Branch | per-task branch `task.branch` | `workspace.branch` |
| Worktree | `solodawn_temp/worktrees/{task.branch}` (or no-worktree fallback → shared repo) | `solodawn_temp/worktrees/{ws_id-task_title}/{repo.name}` |
| Diff transport | none | `/api/task-attempts/{attemptId}/diff/ws` |

The entire diff/review UI is keyed on a `workspaceId`/`attemptId` that orchestration never produces or links. Five concrete breaks, each verified:

### Break 1 — The diff stream is wholly `Workspace`/`WorkspaceRepo`-keyed and returns an empty stream otherwise
`crates/local-deployment/src/container.rs:1512-1583` `stream_diff(workspace, ...)`:
- enumerates `WorkspaceRepo::find_by_workspace_id` (1518) + `find_repos_for_workspace` (1525);
- builds `worktree_path = workspace_root.join(repo.name)` (1534) where `workspace_root = ensure_container_exists(workspace)`;
- diffs `workspace.branch` vs `WorkspaceRepo.target_branch` via `get_base_commit` (1545-1558);
- **`if streams.is_empty() { return Ok(Box::pin(futures::stream::empty())) }` (1577-1578).**

The only WS transport is `crates/server/src/routes/task_attempts.rs:322-352` `stream_task_attempt_diff_ws`, which takes `Extension<Workspace>` and calls `deployment.container().stream_diff(&workspace, ...)`. There is **no** workflow/task/terminal/commit-scoped diff endpoint.

### Break 2 — The orchestration materialize path creates NO `Workspace`/`WorkspaceRepo`
`materialize_draft` creates only a `Workflow` (+ `WorkflowTasks`). The concierge `execute_create_workflow` (`crates/services/src/services/concierge/tools.rs:511-560`) DOES create a **companion** `Task` + `Workspace` "so the sidebar 活跃 list picks it up" (tools.rs:539 `Workspace::create`) — BUT with `shared_task_id: None` (tools.rs:532) and **no `WorkspaceRepo` rows and no worktree**. So `stream_diff` still hits `streams.is_empty()` (empty), and `container.create()` would reject it anyway (`workspace_repos.is_empty()` → error, container.rs:1175-1178). This companion workspace is a **trap**: it exists but is non-diffable.

### Break 3 — The model bridge `Task.shared_task_id` is never populated from the orchestration side
`crates/db/src/models/task.rs:504` `set_shared_task_id` has **ZERO non-test callers** (grep across `crates/` returns only the definition). The reverse read-only bridge (`task_attempts.rs` `get_planning_messages`: `Workspace → Task.shared_task_id → WorkflowTask`) proves the schema COULD link them, but nothing writes the link. Likewise `crates/services/src/services/terminal/launcher.rs` `get_workspace_for_terminal` traverses `vk_task_id → workspaces.task_id` and returns `None` for materialized agent_planned tasks (`vk_task_id` is `None`), so `Session.workspace_id` stays `None`.

### Break 4 — PATH-LAYOUT MISMATCH (the trap that defeats the "just create a Workspace" fix)
Both managers share the same BASE dir (`workspace_manager.rs:254-256` `get_workspace_base_dir()` returns `WorktreeManager::get_worktree_base_dir()` = `solodawn_temp/worktrees`), but the SUBPATH differs:
- Orchestrator commits at `WorktreeManager::get_worktree_base_dir().join(&task.branch)` = `…/worktrees/{task.branch}` (`agent.rs:2669-2671`, quality-gate working_dir resolution).
- `stream_diff` reads `get_workspace_base_dir().join(workspace_dir_name).join(repo.name)` = `…/worktrees/{ws_id-task_title}/{repo.name}` (`container.rs:1169-1171, 1534`).

So naively creating/linking a `Workspace` makes `ensure_container_exists` build a **second** worktree on the **same branch** — git refuses two worktrees per branch, so it either errors or diffs an empty/fresh checkout. **Creating a Workspace row alone will NOT surface the orchestrator's changes.** (Refinement over the raw investigator claim that "orchestration never creates a worktree": it DOES use managed worktrees; the missing thing is a *Workspace ROW pointing at the right path*, not the worktree itself. No-worktree fallback to the shared project repo also exists.)

### Break 5 — The acceptance REVIEW score is structurally invisible; quality GATE is surfaced only on legacy pages
- **Acceptance REVIEW (5-dim, `total_score/100`)** — the thing the user most wants — is parsed (`agent.rs:5408` `AuditScoreResult::parse`) then collapsed via `to_acceptance_result()`. `AuditScoreResult`/`AuditDimensions` (`crates/services/src/services/orchestrator/types.rs:548-579`) have **only `Serialize, Deserialize` — no `TS`/`#[ts(export)]`**. `handle_acceptance_review_result` (`agent.rs:5140-5249`) on rejection ONLY: `tracing::warn`, sets status `review_pending`, publishes `BusMessage::TaskStatusUpdate` carrying just the **status string** (5192-5196), and injects `"Score {score}/100 …"` as **plain PTY text** into the terminal (5216). On approval it just `return Ok(true)` (5145-5147). **No DB write, no DTO, no score-bearing WS event.**
- **Quality GATE** (terminal/branch/repo) IS persisted (`QualityRun::insert`, per-issue rows) and emitted as a `quality.gate_result` WS event (`crates/server/src/routes/workflow_events.rs:425-453`, from `BusMessage::TerminalQualityGateResult` published at `agent.rs:3022`). It is rendered by `QualityReportPanel`/`QualityBadge` — but **only** on the OLD pages: `onQualityGateResult`/`useWorkflowEvents` are wired ONLY in `frontend/src/pages/Board.tsx`, `frontend/src/pages/Workflows.tsx`, and `/debug/:workflowId`. **`WorkspacesLayout` imports zero quality components and never calls `useWorkflowEvents`, so the event is silently dropped in `/workspaces`.**

### Break 6 — Frontend: no `workspaceId` in orchestration views, and nothing auto-opens the panel
- `frontend/src/hooks/useDiffStream.ts:30` → `/api/task-attempts/${attemptId}/diff/ws`. `frontend/src/contexts/WorkspaceContext.tsx:85,102` derives `workspaceId` from `useParams` (the `/workspaces/:workspaceId` route only) → diffs are always `[]` for orchestration.
- The orchestrator chat runs INSIDE `/workspaces` via `?conciergeId` (`WorkspacesLayout.tsx:85-86`), but there is **no `:workspaceId` route param**, so `selectedWorkspace` is undefined and `ChangesPanelContainer attemptId={selectedWorkspace?.id}` is undefined (`WorkspacesLayout.tsx:200-201`).
- In concierge mode the `RightSidebar` **early-returns a status-only view** (workflow name + task/terminal dots + Pipeline/Debug links) at `frontend/src/components/ui-new/containers/RightSidebar.tsx:61-133`, so the `CHANGES` branch at line 146 is **structurally unreachable**.
- The panel defaults to closed (`useUiPreferencesStore.ts` `DEFAULT_WORKSPACE_PANEL_STATE.rightMainPanelMode: null`) and **every** `setRightMainPanelMode(CHANGES)` caller is a manual gesture (`SessionChatBoxContainer.tsx:116` View Code; `actions/index.ts`; `ChangesViewContext` file click). No completion/WS handler auto-opens it.
- WS payloads carry no diff: `terminal.completed` / `git.commit_detected` only carry `commitHash`/`commitMessage` (`wsStore.ts:371-372, 1534-1556`); `useWorkflowLiveStatus` collapses them into a non-clickable text summary and throws the hash away.

**Net root cause:** the orchestration→frontend diff/audit mapping was **never implemented**. The dead half is `Task.set_shared_task_id` (zero callers) plus the absence of any task/branch-scoped diff endpoint, plus the acceptance score having no persistence/DTO/WS transport, plus the orchestration UI (concierge `RightSidebar`, `WorkspacesLayout` for `?conciergeId`) having no Changes/Audit surface and no auto-open trigger.

---

## 2. VERDICT — keep the tools; the exact mechanism to make them usable + auditable

**KEEP Preview, Diff/Changes, and per-line Comment.** Do NOT try to force-fit the `Workspace`/worktree diff stream onto orchestration (Break 4 makes that a worktree-collision trap). Instead:

### Mechanism
1. **Diff source — reuse the existing branch-diff primitive, not the worktree stream.**
   `GitService::get_diffs(DiffTarget::Branch { repo_path, branch_name, base_branch })` (`crates/services/src/services/git.rs:317-353`) already diffs two branches by tree comparison **with no worktree**. For an orchestration task this maps directly: `branch_name = task.branch`, `base_branch = workflow.target_branch`, `repo_path = ` the orchestrator's worktree dir (or shared repo in fallback). Add a new endpoint `GET /api/workflows/{workflowId}/tasks/{taskId}/diff` (REST snapshot) + optional `…/diff/ws` (live) that returns `Vec<FileDiff>` — the SAME `Diff` shape `ChangesPanel` already renders. This survives post-merge worktree cleanup because branch-vs-branch tree diff works as long as the branch ref exists (and, for merged tasks, you can diff the merge commit range).

2. **Audit/score — persist + emit.**
   - Add `#[derive(TS)] #[ts(export)]` to `AuditScoreResult`/`AuditDimensions`/`CodeQualityScore` (types.rs:548-579).
   - Persist the score (new column on `workflow_task`, or a small `acceptance_review` table keyed by `task_id`/`terminal_id`, holding `total_score`, `dimensions` JSON, `verdict`, `fix_instructions`).
   - Add a new `BusMessage::TerminalAcceptanceReview` → `WsEventType::AcceptanceReviewResult` (mirror the existing `TerminalQualityGateResult` path in `workflow_events.rs:425-453`) carrying `{workflowId, taskId, terminalId, totalScore, dimensions, verdict, passed}`.
   - Emit it from `handle_acceptance_review_result` on BOTH approve (5145) and reject (5249) paths.

3. **Where the panel opens + what drives it.**
   - In the orchestration workspace (concierge mode), restructure `RightSidebar` so it is NOT a status-only dead end: add a per-task "View Changes" affordance and a `CHANGES`/`AUDIT` branch reachable in concierge mode.
   - `WorkspacesLayout` (and/or concierge `RightSidebar`) subscribes to `useWorkflowEvents(workflowId)`. On `terminal.completed` (or `acceptance_review_result`/`quality.gate_result`) for a task, it (a) selects that task as the active diff target, (b) calls `setRightMainPanelMode(CHANGES)` to **auto-open**, and (c) renders the task's branch-diff + the score/issues.
   - `ChangesPanelContainer` gains an alternate data source: instead of only `useWorkspaceContext().diffs` (workspace-scoped), accept a `taskDiffSource` (the new workflow-task diff endpoint) when in orchestration mode.

This makes Diff genuinely auditable per task, surfaces the score/issues next to it, and auto-opens on completion. Per-line **Comment** round-trip and **Preview** dev-server are scoped as stretch (they need the comment target re-mapped to `WorkflowTask/Terminal` and the preview pointed at the orchestration working dir — see effort/risk).

---

## 3. FIX STEPS — ordered, file-by-file (backend + frontend)

### Phase A — Backend: workflow-task diff endpoint (highest value, lowest risk)
1. `crates/server/src/routes/workflows.rs` — add handler `get_workflow_task_diff(workflowId, taskId)`:
   - load `WorkflowTask` (→ `task.branch`) and `Workflow` (→ `target_branch`, `project_id`);
   - resolve `repo_path` = `WorktreeManager::get_worktree_base_dir().join(&task.branch)` if it exists (`agent.rs:2669-2671` pattern), else the project's first repo / `default_agent_working_dir`;
   - call `GitService::new().get_diffs(DiffTarget::Branch { repo_path, branch_name: &task.branch, base_branch: &workflow.target_branch }, None)`;
   - return `Vec<Diff>` (reuse the existing `Diff` DTO so the FE renders unchanged).
2. `crates/server/src/routes/workflows.rs` (router registration, near the existing workflow routes) — register `GET /api/workflows/:workflow_id/tasks/:task_id/diff`. (Optional `…/diff/ws` later for live.)
3. (Optional, defer) extend `DiffTarget::Commit` usage for merged tasks if the branch ref is gone post-cleanup.

### Phase B — Backend: persist + emit acceptance score
4. `crates/services/src/services/orchestrator/types.rs:548-579` — add `#[derive(..., TS)] #[ts(export)]` to `AuditScoreResult`, `AuditDimensions`, `CodeQualityScore`.
5. `crates/db/migrations/` — new migration: add `acceptance_score` / `acceptance_dimensions_json` / `acceptance_verdict` columns to `workflow_task` (or a new `acceptance_review` table). Add a setter in `crates/db/src/models/workflow_task.rs`.
6. `crates/services/src/services/orchestrator/agent.rs:5140-5249` `handle_acceptance_review_result` — on both approve and reject: write the score row, then publish a new `BusMessage::TerminalAcceptanceReview { workflow_id, task_id, terminal_id, total_score, dimensions, verdict, passed }`.
7. `crates/server/src/routes/workflow_events.rs` — add `WsEventType::AcceptanceReviewResult` (`#[serde(rename = "acceptance.review_result")]`) and a `BusMessage::TerminalAcceptanceReview => Self::new(WsEventType::AcceptanceReviewResult, payload)` arm (mirror lines 98-99, 425-453).
8. `crates/server/src/routes/workflows_dto.rs:60-74` — add `acceptance_score: Option<f64>` (+ optional dimensions) to `WorkflowTaskDto` (it is `#[ts(export)]`, so TS regenerates).

### Phase C — Frontend: data + auto-surface in the orchestration workspace
9. `frontend/src/stores/wsStore.ts:18-31` — add `'acceptance.review_result'` to `WsEventType`, an `AcceptanceReviewPayload` interface, a normalizer, and `['onAcceptanceReviewResult', 'acceptance.review_result']` to the dispatch table (mirror `onQualityGateResult`).
10. `frontend/src/hooks/` — add `useWorkflowTaskDiff(workflowId, taskId)` (REST hook hitting Phase-A endpoint, returning the `Diff[]` shape `ChangesPanel` consumes).
11. `frontend/src/components/ui-new/containers/ChangesPanelContainer.tsx` — accept an optional `taskDiffSource` prop; when provided (orchestration mode), render those diffs instead of `useWorkspaceContext().diffs`. No change to `ChangesPanel`/`DiffViewCardWithComments` rendering.
12. `frontend/src/components/ui-new/containers/RightSidebar.tsx:61-133` — in concierge mode, add a per-task "View Changes" / score badge affordance and make the `CHANGES` branch reachable (don't early-return into a pure status view). Show `task.acceptance_score` + `quality` badge per task.
13. `frontend/src/components/ui-new/containers/WorkspacesLayout.tsx` — when `isConciergeMode`, subscribe to `useWorkflowEvents(conciergeWorkflowId)`; on `terminal.completed` / `acceptance.review_result` / `quality.gate_result`, set the active task diff target and call `setRightMainPanelMode(RIGHT_MAIN_PANEL_MODES.CHANGES)` to auto-open; pass `taskDiffSource` into `ChangesPanelContainer` (line 200).
14. `frontend/src/components/ui-new/containers/ConciergeChatView.tsx` (chat the user actually talks to) — add a per-task "View Code / Audit" button that selects the task and opens CHANGES (so there's an explicit affordance even if auto-open is dismissed).

### Phase D — Stretch (Comment + Preview parity; defer)
15. Re-target per-line Comment anchoring to `WorkflowTask/Terminal` (`ReviewProvider` currently `attemptId={selectedWorkspace?.id}`), and point Preview dev-server at the orchestration working dir. Scope only if the user expects all three tools, not just Diff/Audit.

---

## 4. EFFORT / RISK

| Phase | Effort | Risk | Notes |
|---|---|---|---|
| A (task diff endpoint) | M (~1 day) | Low | Reuses `DiffTarget::Branch`; no schema/worktree change. Main risk: base/target alignment + missing branch ref post-merge (use `Commit` range for merged tasks). |
| B (persist+emit score) | M (~1 day) | Low-Med | New migration + new WS event + ts-rs derive. Partial fix (persist only) leaves nothing rendered — must ship the WS event + DTO together. |
| C (FE auto-surface) | M-L (~1.5 days) | Med | Must restructure concierge `RightSidebar` (currently a status-only dead end) AND add an auto-open handler — a pure backend fix stays invisible (panel default `null`, all openers manual). |
| D (Comment/Preview) | L (~2-3 days) | High | Comment round-trip + preview re-targeting touch deeper orchestration plumbing; defer. |

### Key fix risks to avoid
- **Do NOT** "just create a `Workspace` per WorkflowTask and reuse `stream_diff`": Break 4 path-layout collision → second worktree on the same branch → git error or empty/wrong diff. The companion workspace from `concierge/tools.rs:539` is a trap (no `WorkspaceRepo`, wrong `task_id`).
- **Branch base alignment:** `DiffTarget::Branch` diffs `task.branch` tree vs `workflow.target_branch` tree — confirm the orchestrator branches off `target_branch` so the range is correct; otherwise compute a merge-base.
- **Post-merge cleanup:** orchestrator worktrees are cleaned up after merge (`agent.rs` `cleanup_worktree`). The branch-diff still works while the branch ref exists; for fully merged/deleted tasks, snapshot the diff (or diff the recorded merge-commit range) so review survives.
- **Score is a separate fix from diff:** reusing `QualityReportPanel` covers the GATE (`quality_run`/`quality_issue`) only, NOT the acceptance REVIEW score (no table/DTO/WS today). Both are needed for "audit it".
- **Auto-open is mandatory:** without an explicit `setRightMainPanelMode(CHANGES)` on completion, the panel stays closed (default `null`) and the user still sees nothing.
