# fe-tasks Module Map

Census unit: `frontend/src/components/tasks/` (root + TaskDetails/ + TaskDetails/preview/ + Toolbar/ + follow-up/)

Generated: 2026-06-14

## File Map

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| `tasks/AgentSelector.tsx` | Dropdown to select coding agent (executor) by profile | `AgentSelector` component | Imported by `StartReviewDialog`, `ExecutorProfileSelector`, `ResolveConflictsDialog` | Active, widely used |
| `tasks/BranchSelector.tsx` | Virtualised, searchable branch dropdown with keyboard nav | `BranchSelector` (default export) | Used by `RepoBranchSelector` (this module), `CreatePRDialog`, `ChangeTargetBranchDialog`, `RebaseDialog`, `TaskFormDialog`, `ui-new/dialogs/ChangeTargetDialog`, `ui-new/dialogs/RebaseDialog` | Active, heavily used |
| `tasks/ClickedElementsBanner.tsx` | Banner showing UI elements captured via VS Code webview bridge (ClickedElementsProvider) | `ClickedElementsBanner`, `Props` type | Used by `TaskFollowUpSection`. `appendInstructions` prop defined but never passed by any caller | **Invisible feature**: VS Code webview companion bridge. `appendInstructions` prop is dead (never supplied by caller) |
| `tasks/ConfigSelector.tsx` | Dropdown to select executor variant/config within chosen agent | `ConfigSelector` component | Imported by `StartReviewDialog`, `ExecutorProfileSelector`, `ResolveConflictsDialog` | Active, widely used |
| `tasks/ConflictBanner.tsx` | Warning banner for merge/rebase conflicts; renders "Open in Editor" + "Abort" + "Resolve" buttons | `ConflictBanner`, `Props` type | Used only by `follow-up/FollowUpConflictSection` | **G1 relevance**: "Open in Editor" button calls `onOpenEditor` prop which wires to `useOpenInEditor` hook |
| `tasks/RepoBranchSelector.tsx` | Wrapper over `BranchSelector` for multi-repo scenarios | `RepoBranchSelector` (named + default) | Used by `CreateAttemptDialog`, `TaskFormDialog` | Active |
| `tasks/RepoSelector.tsx` | Dropdown to select a repository | `RepoSelector` (default export) | Used only inside `Toolbar/GitOperations.tsx` | Active |
| `tasks/TaskCard.tsx` | Legacy kanban card for task; wraps `KanbanCard` from `shadcn-io/kanban` | `TaskCard` component | Only imported by its own test (`TaskCard.test.tsx`). `board/TaskCard.tsx` is the active replacement | **R5 orphan delete candidate**: no production importer; relies on `shadcn-io/kanban.tsx` which is itself a stub targeted for removal by `legacy-components.test.ts` |
| `tasks/TaskCard.test.tsx` | Vitest unit test for `tasks/TaskCard` | (test) | Tests `tasks/TaskCard.tsx` only | Would be deleted alongside TaskCard |
| `tasks/TaskCardHeader.tsx` | Layout sub-component: title + avatar + right-slot row for a task card | `TaskCardHeader` component | Only imported by `tasks/TaskCard.tsx` | **R5 orphan delete candidate**: only consumer is `TaskCard` (itself dead) |
| `tasks/TaskFollowUpSection.tsx` | Full follow-up panel: WYSIWYG editor, queue/stop controls, image attach, PR comment insert, variant selector, conflict section | `TaskFollowUpSection` component | Used by `panels/TaskAttemptPanel.tsx`. Internally uses `FollowUpConflictSection`, `ClickedElementsBanner`, `VariantSelector`, `useOpenInEditor` (indirectly via FollowUpConflictSection) | Core active component. Contains queue message flow (G1/planning-draft relevance) |
| `tasks/UserAvatar.tsx` | Round avatar circle showing initials or photo (with image-optimization URL params) | `UserAvatar` component | Used by `TaskCardHeader` (dead, R5) and `org/MemberListItem.tsx` (live) | Keep due to live use in `MemberListItem` |
| `tasks/VariantSelector.tsx` | Dropdown for selecting executor variant (agent configuration sub-option) | `VariantSelector` (memo-wrapped) | Used by `TaskFollowUpSection` and `NormalizedConversation/RetryEditorInline` | Active |
| `tasks/TaskDetails/ProcessLogsViewer.tsx` | Virtualised scrollable log viewer for a single process (streams via `useLogStream`) | `ProcessLogsViewerContent` (named), `ProcessLogsViewer` (default) | Used by `ProcessesTab` (same dir) and `TaskDetails/preview/DevServerLogsView` | Active |
| `tasks/TaskDetails/ProcessesTab.tsx` | List of execution processes with drill-down log viewer for a session | `ProcessesTab` (default export) | Used by `dialogs/tasks/ViewProcessesDialog` | Active |
| `tasks/TaskDetails/preview/DevServerLogsView.tsx` | Collapsible log panel for dev-server processes with multi-tab support | `DevServerLogsView` | **No external importer found** — only uses `ProcessLogsViewer` internally | **Dead/orphan**: superseded by `ui-new/views/PreviewBrowser.tsx` + `ui-new/hooks/usePreviewDevServer`. Investigate before delete |
| `tasks/TaskDetails/preview/NoServerContent.tsx` | Empty state for preview panel when no dev server is running; has "Install companion" CTA | `NoServerContent` | **No external importer found** | **Dead/orphan**: new design has equivalent in `ui-new/views/PreviewBrowser.tsx`. Investigate before delete |
| `tasks/TaskDetails/preview/PreviewToolbar.tsx` | URL bar + refresh/copy/open/stop toolbar for the embedded iframe preview | `PreviewToolbar` | **No external importer found** | **Dead/orphan**: new design uses `ui-new/views/PreviewBrowser.tsx` which reimplements this. Investigate before delete |
| `tasks/TaskDetails/preview/ReadyContent.tsx` | Thin wrapper rendering an `<iframe>` for the preview panel | `ReadyContent` | **No external importer found** | **Dead/orphan**: new design has inline iframe in `PreviewBrowser`. Investigate before delete |
| `tasks/Toolbar/GitOperations.tsx` | Branch chips, commit-status chips, Merge/Push-PR/Rebase action buttons for an attempt's git state | `GitOperations` (default), `GitOperationsInputs` type | Used by `panels/DiffsPanel.tsx` and `dialogs/tasks/GitActionsDialog.tsx` | Active. Multi-repo support via `RepoSelector` |
| `tasks/follow-up/FollowUpConflictSection.tsx` | Bridges conflict state to `ConflictBanner`; wires `useOpenInEditor` and `useAttemptConflicts` | `FollowUpConflictSection` | Used by `TaskFollowUpSection`. Calls `useOpenInEditor(workspaceId)` | **G1 relevance**: "Open in external IDE/editor" is surfaced here |

## Key Relationships Summary

- `TaskFollowUpSection` is the central orchestrator; it owns the follow-up send flow, queue management, variant selection, conflict display, and image/PR-comment insertion.
- `FollowUpConflictSection` → `ConflictBanner` → `onOpenEditor` → `useOpenInEditor` is the entire "open in editor" path from [G1].
- `tasks/TaskCard` + `tasks/TaskCardHeader` form an isolated island: the kanban card set was replaced by `board/TaskCard.tsx`; neither component has a live production importer.
- `TaskDetails/preview/*` (4 files) have zero importers outside their own subtree and are superseded by the `ui-new` design.
