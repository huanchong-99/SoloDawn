# Census: frontend/src/components/ui-new/views/

Unit: fe-uinew-views | Branch: refactor/streamline-quality-gates | Date: 2026-06-14

All files are stateless view components (receive data via props, emit callbacks). Architecture mandated by frontend/CLAUDE.md: "View components (in views/) should be stateless — receive all data via props."

---

## Module Map

| File | Purpose | Public Surface (exports) | Relations (callers / dependencies) | Notes |
|---|---|---|---|---|
| ChangesPanel.tsx | Renders a list of diff cards (one per changed file) with an empty state fallback. Memoizes each `DiffItem` to avoid re-renders. | `ChangesPanel` (forwardRef), `DiffItemData` type | Container: `ChangesPanelContainer`. Used in `WorkspacesLayout`. Inner dep: `DiffViewCardWithComments`, `usePersistedExpanded` | `attemptId` prop flows to `DiffViewCardWithComments` → "open in IDE" path (G1 relevant). `projectId` for @-mentions. |
| ConciergeChatView.tsx | Full chat UI for the Concierge AI: message list, session selector, Feishu sync toggles, workflow progress inset. | `ConciergeChatView` | Container: `ConciergeChatContainer`. No other direct importers. Inner helper components: `MessageBubble`, `WorkflowProgressPanel`, `SyncTogglesPanel`, `ToolMessage`, `SourceBadge` (all file-private) | Feishu sync visible feature: `feishuSync`, `onToggleFeishuSync`, `onSyncHistory`, `syncToggles`, `onUpdateSyncToggle`. Embeds `WorkflowProgressPanel` (separate from `WorkflowProgressView`). |
| FeishuChannelPanel.tsx | Small panel for managing the Feishu channel binding to a concierge session. | `FeishuChannelPanel` | Container: `FeishuChannelContainer`. No other importers. | Feishu-specific invisible feature — shows session binding/switching UI. |
| FileTree.tsx | Renders the collapsible file-tree for changed files, with search bar and GitHub comments toggle. | `FileTree` | Containers: `FileTreeContainer`, `ChangesPanelContainer`, `RightSidebar`. Deps: `FileTreeSearchBar`, `FileTreeNode`, `CollapsibleSectionHeader`, `Tooltip`. | GitHub comments badge toggle is surfaced here via `onToggleGitHubComments`. Hard-coded section title "Changes" (not i18n'd) at line 88. |
| FileTreeNode.tsx | Renders a single node (file or folder) in the tree, with indentation guides, file-type icon, change-kind coloring, diff stats, and GitHub comment badge. | `FileTreeNode` (forwardRef) | Only caller: `FileTree.tsx`. Deps: `getFileIcon`, `useTheme`, `getActualTheme`. | Only used internally by FileTree — not independently imported elsewhere. |
| FileTreeSearchBar.tsx | Search + expand-all toggle bar for the file tree. | `FileTreeSearchBar` | Caller: `FileTree.tsx` only. Deps: `InputField` container. | Thin wrapper around `InputField`. |
| GitPanel.tsx | Shows per-repo status cards (commits ahead, PR info, push state) and an advanced "working branch" editor. Surfaces "Open in Editor" per repo. | `GitPanel`, `RepoInfo` (type) | Container: `GitPanelContainer`. Deps: `RepoCard`, `InputField`, `ErrorAlert`, `CollapsibleSection`, `CollapsibleSectionHeader`. | `onOpenInEditor` prop (G1 relevance). `RepoInfo.remoteCommitsAhead` declared but never consumed in render (redundant field). |
| GitPanelCreate.tsx | Setup panel for associating repos + project for a new workspace. Bind repo to project. | `GitPanelCreate` | Containers: `GitPanelCreateContainer`, `RightSidebar` (imports it). Deps: `CollapsibleSectionHeader`, `SelectedReposList`, `ProjectSelectorContainer`, `RecentReposListContainer`, `BrowseRepoButtonContainer`, `CreateRepoButtonContainer`. | Shows bind-repo-to-project feature which links workspace to a dev project. |
| Navbar.tsx | Top navbar: workspace title center, left actions (archive, old-UI link), right actions (diff toggles, panel toggles). Translates tooltip strings to i18n keys. | `Navbar`, `NavbarProps` | Containers: `NavbarContainer`, `WorkspacesLayout`, `ContextBarContainer`, `NormalLayout`. Deps: `Tooltip`, `actions/index`, `actions/useActionVisibility`. | Contains "Open in Old UI" action in `TOOLTIP_I18N_MAP` (G1 legacy navigation). `NavbarIconButton` is inlined rather than imported from primitives (comment says so explicitly). |
| PreviewBrowser.tsx | Preview iframe panel with floating toolbar: URL bar, screen size toggle (desktop/mobile/responsive), start/stop dev server, resize handles. | `PreviewBrowser`, `MOBILE_WIDTH`, `MOBILE_HEIGHT`, `PHONE_FRAME_PADDING` (constants) | Containers: `PreviewBrowserContainer`, `WorkspacesLayout`. Deps: `PrimaryButton`, `IconButtonGroup`, `IconButtonGroupItem`. | `handleFixDevScript` / `hasFailedDevServer` props connect to ScriptFixer dialog. Exports three numeric constants consumed by container. |
| PreviewControls.tsx | Dev-server log viewer panel with process tabs. Shows spinner while starting. | `PreviewControls` | Containers: `PreviewControlsContainer`, `RightSidebar`. Deps: `CollapsibleSectionHeader`, `VirtualizedProcessLogs`, `getDevServerWorkingDir`. | Log viewer for dev server stdout/stderr. |
| SettingsLayout.tsx | Two-pane settings page shell: icon sidebar nav (hidden on mobile → horizontal scroll bar), main content area, re-run setup wizard button. | `SettingsLayout`, `SettingsNavItem` (type), `SettingsLayoutViewProps` (type) | Containers: `SettingsLayoutContainer`, App.tsx (imports settings page). Deps: none beyond icons/utils. | Responsive: sidebar hidden on `md:` breakpoint switches to top scroll nav. |
| WorkflowProgressView.tsx | Compact widget showing workflow status, task list, live event feed (collapsed by default). Connection status dot. | `WorkflowProgressView` | Container: `WorkflowProgressContainer` (used inside `CreateChatBoxContainer`). Deps: `WorkflowTaskDto`, `LiveEvent` types. | Accepts `t` as a prop (not `useTranslation`) — unusual pattern. Part of planning-draft flow (System B). |
| WorkspacesMain.tsx | Main chat area: conversation list, planning-draft messages (collapsible), chat box, floating context bar. | `WorkspacesMain` | Containers: `WorkspacesMainContainer`, `WorkspacesLayout`. Deps: `SessionChatBoxContainer`, `ContextBarContainer`, `ConversationList`, context providers (`EntriesProvider`, `MessageEditProvider`, `RetryUiProvider`, `ApprovalFeedbackProvider`). | `planningMessages` / `showPlanningMessages` / `onTogglePlanningMessages` props implement the planning-draft confirm→materialize flow (System B). |
| WorkspacesSidebar.tsx | Left sidebar listing active/archived workspaces, search, add button, archive toggle footer. | `WorkspacesSidebar` | Containers: `WorkspacesSidebarContainer`, `WorkspacesLayout`. Deps: `WorkspaceSummary`, `InputField`, `SectionHeader`. | `draftTitle` + `isCreateMode` + `onSelectCreate` implement in-progress "create mode" draft workspace UX. Displays concierge/draft workspace names. |

---

## Candidates for Keep/Cut

| # | File | Lines | Kind | Evidence | Why | Disposition | Confidence | Blast Radius |
|---|---|---|---|---|---|---|---|---|
| 1 | `Navbar.tsx` | 34, 29–71 | legacy | `TOOLTIP_I18N_MAP` has entry `'Open in Old UI': 'open-in-old-ui'`. The action `Actions.OpenInOldUI` exists in `actions/index.ts:979` and is placed in left nav items. G1 deletion candidate. | "Open in Old UI" is a legacy escape hatch to the old React UI. On the refactor branch, old UI may be removed. But the Navbar itself is fully live. | investigate (the action, not the whole Navbar) | medium | Removing the map entry + action is low-risk; removing Navbar itself breaks everything. |
| 2 | `GitPanel.tsx` | 20 | dead | `RepoInfo.remoteCommitsAhead?: number` — field declared, populated by `GitPanelContainer` (line 93), but never read inside `GitPanel.tsx` render and never forwarded to `RepoCard`. | Dead field in the prop interface — container computes it but view ignores it. | delete (field only) | high | Container change needed to stop passing it. No UI break. |
| 3 | `ConciergeChatView.tsx` | 218–289 | duplicate | `WorkflowProgressPanel` (lines 218–289) is a private inline component that duplicates `WorkflowProgressView.tsx` (separate exported view). Both show workflow status + task list + terminal dots. | Two implementations of workflow progress: one embedded in concierge chat, one exported from `WorkflowProgressView`. The concierge variant is slightly lighter (no events feed). | investigate (could consolidate) | medium | Refactoring would require container changes only. |

---

## Invisible / Background Features

| Feature | Where | What it does | Seems used | Note |
|---|---|---|---|---|
| Feishu sync toggles (tools/terminal/progress/completion) | `ConciergeChatView.tsx` SyncTogglesPanel | Fine-grained control over what from the Concierge session is mirrored to Feishu | Yes — container `ConciergeChatContainer` passes `syncToggles` + `onUpdateSyncToggle` | Only visible when `feishuSync` is on. Invisible to users without Feishu. |
| Feishu channel binding | `FeishuChannelPanel.tsx` | Bind a concierge session to a Feishu channel for bidirectional sync | Yes — `FeishuChannelContainer` drives it | Completely hidden behind Feishu integration |
| GitHub PR comments badge | `FileTree.tsx` / `FileTreeNode.tsx` | Shows per-file GitHub review comment counts in the file tree; toggle button hides/shows | Yes — `FileTreeContainer` has `getGitHubCommentCountForFile` callback | Only visible when `showGitHubComments` is true |
| Script fixer flow | `PreviewBrowser.tsx` `handleFixDevScript` | When dev server script has failed, offers "Fix Script" instead of "Start"; opens `ScriptFixerDialog` | Yes — container passes handler | Conditionally surfaced only on `hasFailedDevServer` |
| Planning draft conversation | `WorkspacesMain.tsx` `planningMessages` | Shows the draft planning conversation between user and planner before a workspace materializes | Yes — `WorkspacesMainContainer` fetches via `useWorkspacePlanningMessages` | Part of System B (planning-draft confirm→materialize). Collapsible. |
| Responsive preview resize handles | `PreviewBrowser.tsx` DesktopIframeView | Drag handles on right/bottom/corner of iframe for free-form resize in responsive mode | Yes — `usePreviewSettings` hook manages dimensions | Only visible in `responsive` screen-size mode |

---

## In-flight Work Relevance

- **G1 (Open in external IDE/editor)**: `GitPanel.tsx` surfaces `onOpenInEditor` per repo → flows to `RepoCard` → `GitPanelContainer.handleOpenInEditor`. Also `ChangesPanel.tsx` passes `attemptId` through to `DiffViewCardWithComments` which has its own IDE-open path. Navbar has `'Open in Old UI'` (legacy escape hatch, separate from IDE).
- **VS Code webview bridge**: Not directly surfaced in these view files. `previewBridge.ts` (not in scope) handles that separately.
- **Quality Gate System A**: None of these view files reference quality gates or `QualityGateConfig`. Quality gate UI lives in `frontend/src/components/quality/`.
- **Planning-draft + AuditPlan System B**: `WorkspacesMain.tsx` implements the planning draft display (collapsible conversation before workspace materializes). `WorkflowProgressView.tsx` is consumed by `CreateChatBoxContainer` during workflow creation. These two together form the visible surface of System B.
