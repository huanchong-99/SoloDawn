# R1 — IDE Editor Connection Subsystem: Deletion-Safety Audit

Date: 2026-06-13
Scope: Map the COMPLETE surface of the "open in external IDE / editor" feature so it can be removed
without breaking the Orchestration Workspace or the Custom DIY Workspace.

## 0. Critical distinction — what is "IDE editor connection" vs not

| Concept | What it is | Keep / Delete |
|---|---|---|
| **External IDE connection** (TARGET) | "Open in IDE/editor" buttons that spawn `code`/`cursor`/etc. on the host, or generate a `vscode://` remote URL. Backend `EditorConfig::open_file*` + `/open-editor` routes + `EditorType` enum + availability check + settings UI. | **DELETE** |
| **In-app WYSIWYG / Lexical editor** (`components/ui/wysiwyg.tsx` + `wysiwyg/`) | The rich markdown chat input used across both workspaces. | **KEEP** |
| **In-app diff viewers** (`DiffCard.tsx`, `DiffViewCardWithComments.tsx`, `GitPanelContainer`, comment editors) | In-app code/diff rendering. They host an "Open in IDE" button but the diff itself stays. | **KEEP component, strip the IDE button** |
| **VS Code webview iframe bridge** (`frontend/src/vscode/bridge.ts`, `ContextMenu.tsx`) | Keyboard/clipboard bridge for when the *whole app* runs embedded inside a VS Code webview (a Vibe-Kanban-style extension host). NOT the "open in IDE" feature. | **see §4 — partial** |
| `--vscode-*` CSS vars (`styles/new/index.css`, `styles/legacy/index.css`) | Theme fallbacks read from a VS Code webview host. Pure CSS `var(... , default)`; harmless. | **KEEP (out of scope)** |
| In-app **Preview Browser** iframe (`utils/previewBridge.ts`, `PreviewBrowser*`) | Dev-server preview iframe. Unrelated. | **KEEP (out of scope)** |

The webview bridge (§4) is a SEPARATE feature from "open in IDE", but it is entangled because
`wysiwyg.tsx` imports `writeClipboardViaBridge` from `bridge.ts`. Decide separately whether the
webview-host scenario is also dead; if you only want "open in IDE" gone, `bridge.ts` can largely stay.

---

## 1. Frontend — dedicated IDE/editor files (pure DELETE candidates)

| File | Role | Disposition |
|---|---|---|
| `frontend/src/components/ide/IdeIcon.tsx` | `IdeIcon` + `getIdeName(editorType)` — renders per-IDE svg/logo. | delete |
| `frontend/src/components/ide/OpenInIdeButton.tsx` | "Open in <IDE>" ghost button; reads `config.editor.editorType`. | delete |
| `frontend/src/components/EditorAvailabilityIndicator.tsx` | Availability badge (checking/available/notFound). | delete |
| `frontend/src/hooks/useEditorAvailability.ts` | Calls `configApi.checkEditorAvailability`. | delete |
| `frontend/src/hooks/useOpenInEditor.ts` | Calls `attemptsApi.openEditor`; opens `EditorSelectionDialog` on failure. | delete |
| `frontend/src/hooks/useOpenProjectInEditor.ts` | Calls `projectsApi.openEditor`; opens `ProjectEditorSelectionDialog`. | delete |
| `frontend/src/components/dialogs/tasks/EditorSelectionDialog.tsx` | Editor-picker modal (attempt). | delete |
| `frontend/src/components/dialogs/projects/ProjectEditorSelectionDialog.tsx` | Editor-picker modal (project). | delete |
| `frontend/public/ide/*.svg` (12 files) | IDE logo assets (vscode/cursor/windsurf/intellij/zed/xcode/antigravity dark+light). | delete |

---

## 2. Frontend — call sites that must be edited (NOT whole-file delete)

These files use the IDE feature but contain other live functionality. Strip the IDE bits only.

### Orchestration / Custom workspaces — IDE buttons embedded in shared UI

| File:line | What | Action |
|---|---|---|
| `frontend/src/components/layout/Navbar.tsx:27-28,79,191-196` | imports `useOpenProjectInEditor`+`OpenInIdeButton`; `handleOpenInEditor` (the project-level "Open in IDE" in the top bar, gated by `isSingleRepoProject`). | remove import, hook call, and the `<OpenInIdeButton>` block. |
| `frontend/src/components/NormalizedConversation/NextActionCard.tsx:17,22,191,269,362` | "See changes in {editor}" action on a failed attempt; renders `IdeIcon`, calls `useOpenInEditor`, label via `getIdeName`. | remove the open-in-editor button + imports; keep the rest of the card. |
| `frontend/src/components/ui/actions-dropdown.tsx:13,45,196` | `useOpenInEditor(attempt?.id)`; "Open in IDE" dropdown item (`actionsMenu.openInIde`). | remove hook + the `<DropdownMenuItem onClick={handleOpenInEditor}>`. |
| `frontend/src/components/tasks/follow-up/FollowUpConflictSection.tsx:3,30,55-59` | passes `onOpenEditor` to `ConflictBanner` to open conflicted file in IDE. | remove `useOpenInEditor` + the `onOpenEditor` prop wiring. See ConflictBanner §2b. |
| `frontend/src/components/ui-new/containers/DiffViewCardWithComments.tsx:31-32,193,433-437` | imports `OpenInIdeButton`+`useOpenInEditor`; renders `<OpenInIdeButton onClick={handleOpenInIde}>` in diff header. | remove import, hook, button. |
| `frontend/src/components/DiffCard.tsx:271` (legacy DiffCard) | `handleOpenInIDE` calls `attemptsApi.openEditor`. | remove the handler + its trigger. |
| `frontend/src/components/ui-new/containers/GitPanelContainer.tsx:214` | `handleOpenInEditor` → `repoApi.openEditor`. | remove handler + the action wiring that triggers it (RepoCard `onOpenInEditor`). |
| `frontend/src/components/ui-new/containers/RepoCard.tsx:248-250` | "Open in IDE" dropdown item (`actions.openInIde`), prop `onOpenInEditor`. | remove the item + prop; trace `onOpenInEditor` from parent (GitPanelContainer). |
| `frontend/src/components/ui-new/primitives/ContextBar.tsx:6,108-109,144,163-180` | special `ide-icon` rendering branch; `editorType` prop. | remove the `ide-icon` branch + `editorType` prop + `IdeIcon` import. |
| `frontend/src/components/ui-new/primitives/CommandBar.tsx:15,22-24` | `ide-icon` special-icon branch using `IdeIcon`. | remove the `ide-icon` branch + import. |
| `frontend/src/components/ui-new/actions/index.ts:56-57,640-665,646` | `Actions.OpenInIDE` definition (id `open-in-ide`, icon `ide-icon`); imports `getIdeName`+`EditorSelectionDialog`; `execute` calls `attemptsApi.openEditor`. | delete `OpenInIDE` action + the `ide-icon` from the `ActionIcon`/`isSpecialIcon` union (grep `'ide-icon'`); remove imports. THIS feeds the ContextBar of both workspaces. |

### 2b. Cascade — ConflictBanner
`frontend/src/components/tasks/ConflictBanner.tsx:10,44,112` declares `onOpenEditor: () => void` and renders
an "open in editor" button (`onClick={onOpenEditor}`). It is also consumed by FollowUpConflictSection.
Decide: drop the `onOpenEditor` prop + button entirely (recommended) — verify no other caller passes it
(only FollowUpConflictSection found).

### 2c. Barrels / registries to prune
| File:line | Edit |
|---|---|
| `frontend/src/hooks/index.ts:3` | remove `export { useOpenInEditor }`. |
| `frontend/src/components/dialogs/index.ts:27-29,43-45` | remove `ProjectEditorSelectionDialog` + `EditorSelectionDialog` re-exports. |
| `frontend/src/types/modals.ts:6,33` | remove `EditorSelectionDialogProps` import + `'editor-selection'` modal arg. |

---

## 3. Frontend — `lib/api.ts` client methods (DELETE methods)

| Line | Method | Action |
|---|---|---|
| `frontend/src/lib/api.ts:99-101` | `type OpenEditorApiRequest = OpenEditorRequest & { git_repo_path? }` | delete (after callers removed). |
| `:329-338` | `projectsApi.openEditor` → `POST /api/projects/{id}/open-editor` | delete |
| `:791-803` | `attemptsApi.openEditor` → `POST /api/task-attempts/{id}/open-editor` | delete |
| `:1113-1122` | `repoApi.openEditor` → `POST /api/repos/{id}/open-editor` | delete |
| `:1152-1159` | `configApi.checkEditorAvailability` → `GET /api/editors/check-availability` | delete |
| `:8,49,73` | imports of `EditorType`, `CheckEditorAvailabilityResponse`, `OpenEditorResponse` from `shared/types` | delete (after methods gone). |

`shared/types` (`OpenEditorRequest/Response`, `CheckEditorAvailabilityResponse`, `EditorType`, `EditorConfig`,
`EditorOpenError`) is GENERATED by `crates/server/src/bin/generate_types.rs` — see §5/§6; regenerate after backend changes.

---

## 4. Frontend — VS Code webview iframe bridge (SEPARATE feature; partial)

| File | Role | Disposition |
|---|---|---|
| `frontend/src/vscode/bridge.ts` | Keyboard/clipboard bridge; **auto-installs on import** (`installVSCodeIframeKeyboardBridge()` at L487). Exports `writeClipboardViaBridge`/`readClipboardViaBridge` used by wysiwyg. | **refactor / keep** — NOT the "open in IDE" feature. |
| `frontend/src/vscode/ContextMenu.tsx` | `WebviewContextMenu` React component. **No importers found** (only self-references). Appears dead. | delete (orphan) — verify with build. |

DEPENDENCY (blocks naive delete of bridge.ts):
- `frontend/src/components/ui/wysiwyg.tsx:45,108` imports `writeClipboardViaBridge` from `@/vscode/bridge`.
  `wysiwyg.tsx` is the in-app markdown editor used by ~10 chat boxes in BOTH workspaces (SessionChatBox,
  CreateChatBox, ChatBoxBase, TaskFollowUpSection, etc.). Deleting `bridge.ts` breaks copy in all of them.

Recommendation: Treat the webview bridge as out of scope for "remove IDE connection". If the user also
wants the *webview-host* scenario gone, extract `writeClipboardViaBridge`/`readClipboardViaBridge` into a
small clipboard util (drop the iframe/postMessage path) and then delete `bridge.ts` + `ContextMenu.tsx`.
The `--vscode-*` CSS vars are independent theme fallbacks and can stay regardless.

---

## 5. Backend — external editor open / detect (Rust)

### Core module (DELETE)
- `crates/services/src/services/config/editor/mod.rs` — entire module:
  - `EditorOpenError` enum, `EditorConfig` struct, `EditorType` enum.
  - `EditorConfig::get_command`, `resolve_command`, `check_availability`, `open_file`,
    `open_file_with_hint`, `remote_url_with_hint`, `spawn_local`, `with_override`.

### Config wiring (REFACTOR — persisted config; needs migration thought)
- `crates/services/src/services/config/mod.rs:5,8,22,25`
  - `pub mod editor;` `pub use editor::EditorOpenError;`
  - `pub type EditorConfig = versions::v9::EditorConfig;`
  - `pub type EditorType = versions::v9::EditorType;`
- `crates/services/src/services/config/versions/v9.rs:5,36,71,118,158,187`
  - `pub editor: EditorConfig` field in the v9 Config struct; default + migration carry-over; JSON fixtures.
  - `EditorConfig`/`EditorType` also referenced across `versions/v1.rs`..`v8.rs` (migration chain) — grep hit
    in v1–v9. **Removing the `editor` field changes the persisted config schema**: either keep the field
    (dead but harmless) for back-compat, or add a v10 migration that drops it. SAFEST: keep `EditorConfig`
    type but delete only the open/availability *behavior* + routes; or do a proper v10. Mark **refactor**, not
    blind delete.

### Routes (DELETE handlers + route registrations)
| File:line | Symbol |
|---|---|
| `crates/server/src/routes/task_attempts.rs:793-807` | `OpenEditorRequest`, `OpenEditorResponse` structs |
| `:809-848` | `normalize_editor_repo_path`, `resolve_workspace_repo_for_editor`, `resolve_workspace_file_open_root` |
| `:850-...` | `resolve_workspace_file_path_for_editor` |
| `:965-1044` | `open_task_attempt_in_editor` handler (analytics event `task_attempt_editor_opened`) |
| `:2143` | `.route("/open-editor", post(open_task_attempt_in_editor))` |
| `:891` | `mod open_editor_path_tests` (tests) |
| `crates/server/src/routes/projects.rs:379-389,459,466-535,734,766-805` | `OpenEditorRequest/Response`, `open_project_in_editor`, route reg, tests |
| `crates/server/src/routes/repo.rs:23,93,101-...,215-275,354` | imports `OpenEditorRequest/Response` from projects; `open_repo_in_editor`; `.route("/repos/{repo_id}/open-editor", ...)`; tests |
| `crates/server/src/routes/config.rs:28,49-50,694-718` | imports `EditorConfig,EditorType`; `.route("/editors/check-availability", get(check_editor_availability))`; `CheckEditorAvailabilityQuery`, `CheckEditorAvailabilityResponse`, `check_editor_availability` |

NOTE `config.rs:225` builds an info payload `{"editor": new.editor}` — adjust if the config field is removed.

### Error plumbing (REFACTOR)
- `crates/server/src/error.rs:16,66,164-168` — `use ...EditorOpenError`; `ApiError::EditorOpen(#[from] EditorOpenError)`;
  the match arm mapping `EditorOpenError` → HTTP status. Delete the variant + arm once `editor/mod.rs` is gone.

### Type generation (REFACTOR)
- `crates/server/src/bin/generate_types.rs:114-115,128-129,168-170` — `.decl()` calls for
  `CheckEditorAvailabilityQuery/Response`, `OpenEditorRequest/Response`, `EditorConfig`, `EditorType`,
  `EditorOpenError`. Remove these so `shared/types` no longer emits the bindings; then regenerate.

### Executor dependency (KEEP)
- `editor/mod.rs` uses `executors::command::CommandBuilder` + `ExecutorError` — these are shared infra used
  widely; do NOT delete them, just stop importing.

---

## 6. i18n keys to remove

(English canonical; mirror in zh-Hans, zh-Hant, ja, ko, es)
- `tasks.json`: `openInIde` (x2 — `actionsMenu.openInIde` ~L352 and `attempt`-level ~L521), `openInEditor`
  ("See changes in {{editor}}" ~L366).
- `common.json`: `actions.openInIde` (~L221), `actions.customCommand` (~L339, if only used by editor settings).
- `settings.json`: the whole `settings.general.editor.*` block (`title`, `description`, `type`,
  `customCommand`, `remoteSsh.{host,user}`, `availability.{checking,available,notFound}`) ~L57-87, plus
  `onboarding.{selectEditorPlaceholder,editorDescription,customCommand}`.

(Locale line numbers vary per file — grep `openInIde`, `openInEditor`, `"editor"`, `availability`,
`customCommand`, `remoteSsh` per locale.)

---

## 7. Settings & onboarding UI (REFACTOR)

- `frontend/src/pages/ui-new/settings/GeneralSettingsNew.tsx:15-16,57,220-365,297-310` — entire Editor
  settings section: editor type select, custom command, remote SSH host/user, availability indicator.
  Remove section; remove `useEditorAvailability` + `EditorAvailabilityIndicator` imports; stop writing
  `draft.editor.*`.
- `frontend/src/components/dialogs/global/OnboardingDialog.tsx:35-36,60,195-219` — onboarding editor picker +
  availability indicator + custom command field. Remove.

Both write into `config.editor` — coordinate with the §5 config-field decision.

---

## 8. Tests referencing the feature
- `crates/server/src/routes/task_attempts.rs:891` `mod open_editor_path_tests`
- `crates/server/src/routes/projects.rs:766` `mod open_editor_path_tests`
- `crates/server/src/routes/repo.rs:101` `mod open_editor_path_tests`
- `crates/services/src/services/config/editor/mod.rs:32-111` `mod tests` (remote URL tests)

---

## 9. Workspace-breakage flags (verify before deleting)
1. `actions/index.ts` `OpenInIDE` + `'ide-icon'` special icon feed the **ContextBar of both workspaces**
   (`ContextBar.tsx`, `CommandBar.tsx`). Removing the action requires removing `'ide-icon'` from the
   `ActionIcon`/`isSpecialIcon` union and both render branches, or the bars break.
2. `wysiwyg.tsx` ↔ `vscode/bridge.ts` clipboard coupling — do NOT delete `bridge.ts` without first
   re-homing `writeClipboardViaBridge`/`readClipboardViaBridge`. Affects every chat box in both workspaces.
3. `config.editor` field is in the **persisted, versioned config (v9)** and the migration chain — removing
   the field is a schema change; prefer a v10 migration or keep the field inert.
4. `ConflictBanner.onOpenEditor` cascades from FollowUpConflictSection (Custom/Orchestration follow-up flow).
5. `shared/types` is generated — backend changes + `generate_types.rs` edits must be followed by regeneration,
   or the frontend type imports go stale.

## 10. Unverified / open
- Whether the VS Code webview-host packaging (the scenario `bridge.ts` serves) is still a product goal.
- Exact non-EN i18n line numbers (grep per locale).
- Whether `common.json actions.customCommand` is used anywhere outside editor settings.
