# Census: fe-utils-types-kbd

Unit covers: frontend/src/utils/, frontend/src/types/, frontend/src/constants/, frontend/src/config/, frontend/src/keyboard/, frontend/src/App.tsx, frontend/src/main.tsx

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|---|---|---|---|---|
| **src/App.tsx** | Root React component; routing tree (SoloDawn new-design + legacy design + setup/first-run wizard); analytics init; disclaimer/release-notes flow | `App` (default export), `AppContent` | Imports: Board, Pipeline, WorkflowDebugPage, Workflows, Assistant, Workspaces, WorkspacesLanding, all settings pages; ThemeProvider, SearchProvider, HotkeysProvider | `HotkeysProvider initiallyActiveScopes=['*','global','kanban']` — R5 kanban scope is initialized here |
| **src/main.tsx** | Entry point; creates ReactDOM root; sets up QueryClient, Sentry, PostHog, ClickToComponent; side-effect imports ./types/modals | `queryClient` (module-local), entry bootstrap | Imports App.tsx, ./types/modals (side-effect) | Commented-out `ReactQueryDevtools`; mutations set `retry: false` (G30-005) |
| **src/config/showcases.ts** | Static data: onboarding showcase slideshow config for TaskPanel | `showcases` const (as const) | Imports: ShowcaseConfig from types/showcase; consumed by WorkspacesLayout.tsx | Video CDN URLs at vkcdn.britannio.dev |
| **src/constants/processes.ts** | Constants and guards for ExecutionProcess run reasons | `PROCESS_RUN_REASONS`, `isCodingAgent()`, `shouldShowInLogs()` | Imported by: useConversationHistory.ts, RestoreLogsDialog.tsx | Typed against shared/types `ExecutionProcessRunReason` |
| **src/keyboard/registry.ts** | Central keyboard binding registry: Action enum, Scope enum, KeyBinding type, static bindings array, lookup utilities | `Scope`, `Action`, `KeyBinding`, `keyBindings`, `getKeysFor()`, `getBindingFor()` | Imported by: useSemanticKey.ts, hooks.ts, and indirectly many components | 18 actions, 10 scopes; kanban-specific nav hooks (hjkl+enter) |
| **src/keyboard/hooks.ts** | Semantic hook factories for all defined actions; builds on useSemanticKey | `useKeyExit`, `useKeyCreate`, `useKeySubmit`, `useKeyFocusSearch`, `useKeyNavUp/Down/Left/Right`, `useKeyOpenDetails`, `useKeyShowHelp`, `useKeyDeleteTask`, `useKeyApproveRequest`, `useKeyDenyApproval`, `useKeySubmitFollowUp`, `useKeySubmitTask`, `useKeySubmitTaskAlt`, `useKeySubmitComment`, `useKeyCycleViewBackward` | Imported by: dialog.tsx, TaskFormDialog.tsx, CreateAttemptDialog.tsx, CommentWidgetLine.tsx, PendingApprovalEntry.tsx, TaskFollowUpSection.tsx, RestoreLogsDialog.tsx, wysiwyg.tsx | All consumers confirmed |
| **src/keyboard/index.ts** | Re-exports hooks.ts and registry.ts | everything from hooks + registry | Barrel for external consumers | |
| **src/keyboard/types.ts** | FormTag and EnableOnFormTags type aliases (react-hotkeys-hook options) | `FormTag`, `EnableOnFormTags` | Imported by useSemanticKey.ts only | Two-line type file |
| **src/keyboard/useSemanticKey.ts** | Core factory `createSemanticHook`; wraps react-hotkeys-hook `useHotkeys`; IME composition guard | `SemanticKeyOptions`, `createSemanticHook()` | Imports types.ts, registry.ts; imported by hooks.ts | IME guard prevents shortcuts firing during CJK input composition |
| **src/types/attempt.ts** | Frontend-local type `WorkspaceWithSession`; helper `createWorkspaceWithSession` | `WorkspaceWithSession`, `createWorkspaceWithSession()` | Extends `Workspace & Session` from shared/types; imported by 14 files including panels, containers, hooks | Heavily used |
| **src/types/logs.ts** | `UnifiedLogEntry` and `ProcessStartPayload` interfaces for normalized log display | `UnifiedLogEntry`, `ProcessStartPayload` | Imported by: DisplayConversationEntry.tsx only | Limited footprint |
| **src/types/modal-args.d.ts** | Declaration-merge for `@ebay/nice-modal-react` ModalArgs; defines `create-pr`, `share-task`, `transfer-shared-task` | Module augmentation (no runtime export) | Loaded as side-effect via main.tsx `./types/modals` import (which does NOT import this file) | **Stale/duplicate**: `create-pr` and `share-task` duplicate what `modals.ts` declares; `transfer-shared-task` never invoked anywhere — this file is NOT imported or side-effect loaded; TS picks it up by being in the same project |
| **src/types/modals.ts** | Declaration-merge for `@ebay/nice-modal-react` ModalArgs (authoritative); full set of modals including `editor-selection` | Module augmentation (no runtime export) | Side-effect imported in main.tsx (`import './types/modals'`) | `editor-selection` modal args contains `EditorSelectionDialogProps` — relevant to G1 "open in external IDE" feature |
| **src/types/showcase.ts** | Type defs for onboarding showcase media/stages | `ShowcaseMedia`, `ShowcaseStage`, `ShowcaseConfig` | Imported by config/showcases.ts | Only consumer is showcases.ts |
| **src/types/tabs.ts** | `TabType` union for task panel tabs | `TabType` | Imported by: TabNavigationContext.tsx | Single-consumer type |
| **src/types/virtual-executor-schemas.d.ts** | Ambient module declaration for Vite virtual module `virtual:executor-schemas` | Module type shim (no runtime export) | Used by ExecutorConfigForm.tsx | Invisible infra: Vite plugin generates executor JSON schemas at build time |
| **src/types/websocket.ts** | WsMessage type aliases + `isWsOutputMessage`, `isWsErrorMessage` type guards | `WsMessage`, `WsOutputMessage`, `WsErrorMessage`, `WsInputMessage`, `WsResizeMessage`, `isWsOutputMessage()`, `isWsErrorMessage()` | Imported by wsStore.ts, stores/index.ts, TerminalEmulator.tsx; tested in __tests__/websocket.test.ts | |
| **src/types/__tests__/websocket.test.ts** | Vitest tests for websocket type guards | Test file | Tests isWsOutputMessage, isWsErrorMessage | |
| **src/utils/StyleOverride.tsx** | iframe postMessage bridge: receives `VIBE_STYLE_OVERRIDE` messages from parent frame to override CSS vars or theme; sends `VIBE_IFRAME_READY` on mount | `AppWithStyleOverride` React component | Not imported by any file (except itself) | **INVISIBLE FEATURE / not wired up**: component exported but never consumed; requires `VITE_PARENT_ORIGIN` env var; is an embedding/iframe integration for Vibe parent; fail-closed when unconfigured |
| **src/utils/TruncatePath.tsx** | CSS-trick path truncation component (shows tail of long paths) | `DisplayTruncatedPath` | Imported by: DiffViewCardWithComments.tsx | |
| **src/utils/companionInstallTask.ts** | Hardcoded task title and description for auto-installing `solodawn-web-companion` npm package | `COMPANION_INSTALL_TASK_TITLE`, `COMPANION_INSTALL_TASK_DESCRIPTION` | Imported by: NoServerContent.tsx (preview fallback) | Injects an agent task instructing user's project to install companion |
| **src/utils/date.ts** | Date formatting utilities | `formatDateShortWithTime()`, `formatRelativeTime()` | Imported by: WorkspaceSummary.tsx, ProcessListItem.tsx, SessionChatBox.tsx, GitHubCommentRenderer.tsx | |
| **src/utils/executor.ts** | ExecutorProfileId comparison, variant sorting, action chain traversal | `areProfilesEqual()`, `getVariantOptions()`, `extractProfileFromAction()`, `getLatestProfileFromProcesses()` | Imported by: useExecutorSelection.ts, SessionChatBoxContainer.tsx, CreateChatBoxContainer.tsx, TaskFollowUpSection.tsx, RetryEditorInline.tsx | |
| **src/utils/extToLanguage.ts** | File extension → Highlight.js language id map | `getHighlightLanguage()`, `getHighLightLanguageFromPath()` | Imported by: DiffViewCard.tsx, DiffViewCardWithComments.tsx, EditDiffRenderer.tsx, FileChangeRenderer.tsx, DiffCard.tsx | |
| **src/utils/fileTreeUtils.ts** | Flat Diff[] → hierarchical TreeNode[] builder; filter/expand helpers; sortDiffs | `buildFileTree()`, `filterFileTree()`, `getExpandedPathsForSearch()`, `getAllFolderPaths()`, `sortDiffs()` | Imported by: FileTreeContainer.tsx, ChangesPanelContainer.tsx | |
| **src/utils/fileTypeIcon.ts** | File extension / filename → developer-icons icon with light/dark variant | `getFileIcon()` | Imported by: ChatFileEntry.tsx, DiffViewCard.tsx, FileTreeNode.tsx, DiffViewCardWithComments.tsx | |
| **src/utils/id.ts** | Crypto-safe ID generation (UUID-based with monotonic fallback) | `secureRandomIdFragment()`, `genId()` | Imported by: vscode/bridge.ts, wsStore.ts, conciergeWsStore.ts, usePreviousPath.ts, ClickedElementsProvider.tsx, ReviewProvider.tsx, ui/toast.tsx | |
| **src/utils/previewBridge.ts** | postMessage listener for `click-to-component` iframe messages; routes to `open-in-editor` / `ready` handlers | `ClickToComponentListener` class, `listenToClickToComponent()`, various interfaces (`OpenInEditorPayload`, `ClickToComponentMessage`, etc.) | Imported by: ClickedElementsProvider.tsx only | **G1 relevance**: this is the "open in external IDE/editor" postMessage bridge; actively used by ClickedElementsProvider |
| **src/utils/scriptPlaceholders.ts** | Strategy pattern for OS-specific setup/dev/cleanup script placeholder templates | `createScriptPlaceholderStrategy()`, `ScriptPlaceholderContext`, `ScriptPlaceholders` | Imported by: useScriptPlaceholders.ts | Windows + Unix variants |
| **src/utils/statusLabels.ts** | TaskStatus → display label / board color CSS var maps | `statusLabels`, `statusBoardColors` | **Zero callers** outside the file itself | Dead export — no file imports either constant |
| **src/utils/streamJsonPatchEntries.ts** | WebSocket JSON-patch streaming utility; maintains `{entries}` snapshot; RFC6902 patch application | `streamJsonPatchEntries()`, `StreamOptions`, `StreamController` | Imported by: useJsonPatchWsStream.ts, useConversationHistory.ts, useDiffStream.ts | Core data pipeline for real-time log/diff streaming |
| **src/utils/string.ts** | String formatting utilities | `toPrettyCase()`, `generateProjectNameFromPath()`, `stripLineEnding()`, `splitLines()`, `splitMessageToTitleDescription()` | Imported by 9 files across settings, chat components, diff views | |
| **src/utils/terminalStatus.ts** | Maps backend status strings to frontend `TerminalStatus` type; handles aliases `running→working`, `idle→not_started` | `mapTerminalStatus()` | Imported by: Workflows.tsx, WorkflowDebugPage.tsx | |
| **src/utils/theme.ts** | Resolves `ThemeMode` to actual `'light'`|`'dark'` string including system preference detection | `getActualTheme()` | Imported by 8 files (ChatFileEntry, DiffViewCard, FileTreeNode, DiffViewCardWithComments, ReleaseNotesDialog, EditDiffRenderer, FileChangeRenderer, DiffCard) | |
| **src/utils/workflowDisplayStatus.ts** | Detects active "Final Integration Repair" task to derive synthetic `repairing_final_issues` display status | `isRepairingFinalIssues()`, `getWorkflowDisplayStatus()` | Imported by: Workflows.tsx, ConciergeChatView.tsx; tested in .test.ts | |
| **src/utils/workflowDisplayStatus.test.ts** | Vitest tests for workflowDisplayStatus | Test file | Tests isRepairingFinalIssues, getWorkflowDisplayStatus | |

---

## Invisible Features

| Feature | File | What it does | Seems used |
|---|---|---|---|
| iframe Vibe embedding bridge | StyleOverride.tsx | Receives `VIBE_STYLE_OVERRIDE` postMessages from parent frame to override CSS vars or switch theme; sends `VIBE_IFRAME_READY` on mount | Not currently wired — AppWithStyleOverride never rendered |
| VS Code webview keyboard bridge | vscode/bridge.ts (adjacent, uses id.ts) | Keyboard event forwarding + clipboard bridge for VS Code iframe embedding | Separate from this scope but id.ts is used by it |
| virtual:executor-schemas | types/virtual-executor-schemas.d.ts | Vite build-time virtual module providing JSON Schema for each executor type | Used by ExecutorConfigForm.tsx |
| `transfer-shared-task` modal | types/modal-args.d.ts | ModalArgs type for a "transfer shared task" modal | Never invoked — stale declaration |
| CompanionInstall agent task | utils/companionInstallTask.ts | Injects task instructions for AI agent to auto-install `solodawn-web-companion` npm package into user's project | Active — used in NoServerContent.tsx preview fallback |

---

## Candidates for Keep/Cut

| Path | Kind | Evidence | Disposition | Confidence |
|---|---|---|---|---|
| src/utils/statusLabels.ts | dead | Zero import matches across entire frontend codebase for `statusLabels` or `statusBoardColors` | delete | high |
| src/utils/StyleOverride.tsx | dead | `AppWithStyleOverride` exported but no file imports it; `VITE_PARENT_ORIGIN` env var not present in any .env | investigate | medium |
| src/types/modal-args.d.ts | duplicate | `create-pr` and `share-task` declarations duplicate modals.ts; `transfer-shared-task` modal never invoked anywhere | delete | high |
