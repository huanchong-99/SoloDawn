# Census: rs-server-routes-projects-repo

Unit: `rs-server-routes-projects-repo`
Files: `crates/server/src/routes/projects.rs`, `crates/server/src/routes/repo.rs`
Branch: `refactor/streamline-quality-gates`
Date: 2026-06-14

---

## Module Map

| File | Purpose | Public Surface (routes / exported fns / types) | Relations | Notes |
|---|---|---|---|---|
| `crates/server/src/routes/projects.rs` | HTTP + WebSocket route handlers for project CRUD, remote-project linking (stub), file search, open-in-editor, repository management, and a WebSocket stream of project events | Routes: `GET /projects`, `POST /projects`, `POST /projects/resolve-by-path`, `GET /projects/{id}`, `PUT /projects/{id}`, `DELETE /projects/{id}`, `GET /projects/{id}/remote/members`, `GET /projects/{id}/search`, `POST /projects/{id}/open-editor`, `POST /projects/{id}/link`, `DELETE /projects/{id}/link`, `POST /projects/{id}/link/create`, `GET /projects/{id}/repositories`, `POST /projects/{id}/repositories`, `GET /projects/{project_id}/repositories/{repo_id}`, `DELETE /projects/{project_id}/repositories/{repo_id}`, `GET /projects/stream/ws`, `GET /remote-projects/{id}`. Types exported: `LinkToExistingRequest`, `CreateRemoteProjectRequest`, `ResolveProjectByPathRequest`, `ResolveProjectByPathResponse`, `OpenEditorRequest`, `OpenEditorResponse`. Private helpers: `normalize_editor_repo_path`, `resolve_project_repo_for_editor`, `resolve_repo_file_path_for_editor`, `resolve_editor_target_file_hint`. | Calls `deployment.project()` service, `ProjectRepo`, `Repo` DB models; imports `OpenEditorRequest`/`OpenEditorResponse` (re-used by `repo.rs`); `router()` merged into global router via `crates/server/src/routes/mod.rs:145`. Frontend callers: `projectsApi.openEditor` → `useOpenProjectInEditor` hook → `Navbar`, `ProjectEditorSelectionDialog`; `projectsApi.unlink` → `useProjectMutations`; `/api/projects/resolve-by-path` → `Workflows.tsx:resolveProjectIdFromPath`; `stream/ws` → `useProjects` hook. | Remote-project routes (`link`, `link/create`, `remote/members`, `remote-projects/{id}`) are live wires to permanently-stubbed handlers that return `ApiError::BadRequest("not supported")`; `unlink_project` is the sole remote-link route that actually works. |
| `crates/server/src/routes/repo.rs` | HTTP route handlers for standalone repo registration, init, batch-get, single-get, update, branch listing, file search, and open-in-editor | Routes: `GET /repos`, `POST /repos`, `POST /repos/init`, `POST /repos/batch`, `GET /repos/{id}`, `PUT /repos/{id}`, `GET /repos/{id}/branches`, `GET /repos/{id}/search`, `POST /repos/{id}/open-editor`. Types exported: `RegisterRepoRequest` (`#[ts(export)]`), `InitRepoRequest` (`#[ts(export)]`), `BatchRepoRequest` (`#[ts(export)]`). Private helpers: `resolve_repo_file_path_for_editor`, `resolve_editor_target_file_hint`, `map_search_repo_lookup_error`. | Imports `OpenEditorRequest`, `OpenEditorResponse` from `projects.rs`; calls `deployment.repo()`, `deployment.git()`, `Repo` DB model, `file_search_cache`. `router()` merged at `mod.rs:153`. Frontend callers: `repoApi.openEditor` → `GitPanelContainer`; `repoApi.init` → (via `/api/repos/init`); `repoApi.getBatch` → (via `/api/repos/batch`). | Duplicates two private helper fns (`resolve_repo_file_path_for_editor`, `resolve_editor_target_file_hint`) that are identical to private fns in `projects.rs` and a third copy exists in `task_attempts.rs` (`normalize_editor_repo_path`). |

---

## Candidates for Keep/Cut

### 1. `open_project_in_editor` handler + its supporting helpers (projects.rs L466-536, helpers L393-464)
**Kind:** G1 deletion candidate (open-in-external-IDE feature)
**Evidence:** Frontend hook `useOpenProjectInEditor` calls `POST /api/projects/{id}/open-editor` → `projectsApi.openEditor`. This is explicitly identified as the IDE editor-connection deletion target in `docs/audit/R1-ide-editor-connection-deletion-audit.md`. Confirmed active production callers in `Navbar.tsx` and `ProjectEditorSelectionDialog.tsx`.
**Disposition:** delete (as part of G1 IDE-editor feature removal)
**Confidence:** high
**Blast radius:** Frontend `useOpenProjectInEditor`, `ProjectEditorSelectionDialog`, `Navbar` open-editor button must be removed together. `OpenEditorRequest`/`OpenEditorResponse` types are also used by `repo.rs:open_repo_in_editor` — shared types cannot be deleted until that handler is also removed.

### 2. `open_repo_in_editor` handler (repo.rs L215-278)
**Kind:** G1 deletion candidate (open-in-external-IDE feature)
**Evidence:** `repoApi.openEditor` → `GitPanelContainer.handleOpenInEditor`. Audit doc calls out `/repos/{id}/open-editor` as a deletion target. Imports `OpenEditorRequest`/`OpenEditorResponse` from `projects.rs` — coupled to candidate 1.
**Disposition:** delete (as part of G1)
**Confidence:** high
**Blast radius:** `GitPanelContainer` open-in-editor button, `repoApi.openEditor` in `api.ts`, shared types if projects.rs handler is also removed simultaneously.

### 3. `link_project_to_existing_remote` (projects.rs L158-166)
**Kind:** stub / dead feature
**Evidence:** Body is `return Err(ApiError::BadRequest("Remote project linking is not supported in this version."))`. Frontend `useProjectMutations.linkToExisting` calls this route, but will always receive an error. LinkProjectDialog shows "remoteDisabledMessage". The handler is permanently non-functional.
**Disposition:** delete (or refactor to 501 Not Implemented with cleaner message)
**Confidence:** high
**Blast radius:** Frontend `linkToExisting` mutation, `LinkProjectDialog`, `OrganizationSettingsNew` link button — these UI paths already show a disabled/error state, so removal is safe after UI is also cleaned.

### 4. `create_and_link_remote_project` (projects.rs L168-177)
**Kind:** stub / dead feature
**Evidence:** Body is `return Err(ApiError::BadRequest("Remote project creation is not supported in this version."))`. Same pattern as above. `_repo_name` variable is unused.
**Disposition:** delete
**Confidence:** high
**Blast radius:** Frontend `createAndLink` mutation, `LinkProjectDialog` "create new" path.

### 5. `get_remote_project_by_id` (projects.rs L191-198)
**Kind:** stub / dead feature
**Evidence:** `Err(ApiError::BadRequest("Remote project features are not supported in this version."))`. No known frontend caller issues a `GET /remote-projects/{id}` request.
**Disposition:** delete
**Confidence:** high
**Blast radius:** Low — no confirmed frontend caller found.

### 6. `get_project_remote_members` (projects.rs L200-207)
**Kind:** stub / dead feature
**Evidence:** `Err(ApiError::BadRequest("Remote project features are not supported in this version."))`. Route registered at `GET /projects/{id}/remote/members`.
**Disposition:** delete
**Confidence:** high
**Blast radius:** Hook `useProjectRemoteMembers` in frontend (file confirmed at grep stage) calls this.

### 7. Duplicate private helpers `resolve_repo_file_path_for_editor` + `resolve_editor_target_file_hint` (repo.rs L55-98, vs projects.rs L421-464)
**Kind:** duplicate
**Evidence:** Both functions are byte-for-byte identical in logic between `projects.rs` and `repo.rs`. A third copy of `normalize_editor_repo_path` exists in `task_attempts.rs`. These should be extracted to a shared crate utility (e.g., `crates/server/src/editor_utils.rs`), but only once the open-editor handlers are confirmed kept. If open-editor feature is deleted entirely (G1), all three copies vanish automatically.
**Disposition:** refactor (extract to shared helper) if G1 is not fully removed; delete automatically if G1 is removed
**Confidence:** high
**Blast radius:** Only internal to the server crate; no public API surface change.

### 8. `get_project_repository` GET handler (projects.rs L713-724)
**Kind:** dubious-feature (no confirmed frontend caller)
**Evidence:** Route `GET /projects/{project_id}/repositories/{repo_id}` returns a `ProjectRepo` join-table record. Grep of frontend `api.ts` and hooks finds only `DELETE` of the same URL pattern; no `GET` of this route appears in frontend code. The router registers both `get(get_project_repository)` and `delete(delete_project_repository)` on the same path.
**Disposition:** investigate (verify with exhaustive frontend grep before deleting)
**Confidence:** medium
**Blast radius:** Low if truly uncalled; would only affect hypothetical external API consumers.

---

## Invisible Features

### WebSocket project stream (`stream_projects_ws` / `handle_projects_ws`)
**What it does:** Maintains a persistent WebSocket at `/api/projects/stream/ws` that pushes raw project-change events from the deployment event bus to the client. Includes a 30-second Ping heartbeat and proper Close handshake.
**Seems used:** Yes — `useProjects` hook connects to this endpoint and uses it as the live data source for the project list.
**User visible:** Not directly (invisible infrastructure); result is that the Projects sidebar updates in real time without polling.

### `resolve_project_by_path` auto-create behavior
**What it does:** `POST /api/projects/resolve-by-path` finds an existing project by repo path, or silently creates a new one if none exists. This side-effectful auto-creation is invisible in the UI.
**Seems used:** Yes — called from `Workflows.tsx:resolveProjectIdFromPath` to associate a workflow with a project when only a working directory path is known.
**User visible:** No — user never sees a "project created" confirmation for this path.
**Note:** The silent creation could be surprising in multi-project contexts; worth a comment or an explicit `create=true` flag.

### `unlink_project` (the only working remote-link route)
**What it does:** `DELETE /projects/{id}/link` calls `deployment.project().unlink_from_remote()` — the one remote-linking operation that is actually implemented. The complementary link/create operations are stubbed.
**Seems used:** Yes — `OrganizationSettingsNew` calls `unlinkProject.mutate()`.
**User visible:** Yes, via settings UI.

---

## In-Flight Relevance

| Theme | Where it appears in scope |
|---|---|
| **G1 — open-in-external-IDE deletion** | `open_project_in_editor` (projects.rs L466-536), `open_repo_in_editor` (repo.rs L215-278), shared types `OpenEditorRequest`/`OpenEditorResponse` (projects.rs L378-391), and all duplicate helper functions (candidates 1, 2, 7 above) |
| **VS Code webview bridge** | Not present in these two files. `OpenEditorResponse.url` field is the "remote mode" URL (vscode:// deep-link), distinct from the in-app webview bridge |
| **Quality Gate System A** | Not present |
| **Planning-draft / AuditPlan System B** | Not present |
