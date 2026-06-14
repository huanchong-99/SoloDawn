# Census: fe-lib-rest — frontend/src/lib/ (excl. api.ts)

Branch: refactor/streamline-quality-gates
Date: 2026-06-14

## Module Map

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|---------------|---------------|-------|
| `apiVersionCompat.ts` | 404-based feature-detection shim for quality-gate endpoints on older backends | `isQualityGateAvailable(error)` | Imported by `QualityReportPanel.tsx`, `useQualityGate.ts` | In-flight: directly tied to Quality Gate System A (G1-c). Intentional heuristic per inline W2-31-08 comment. |
| `conciergeApi.ts` | REST client for the `/api/concierge` namespace — sessions, messages, channels, feishu routing, settings | `conciergeApi` object (9 methods); types: `ConciergeSession`, `ConciergeMessage`, `CreateSessionRequest`, `SendMessageRequest`, `UpdateSettingsRequest`, `AddChannelRequest` | Used by `useConcierge.ts`, `ConciergeChatContainer.tsx`, `WorkspacesSidebarContainer.tsx`, `WorkspacesLayout.tsx`, `FeishuChannelContainer.tsx` | Invisible feature: Feishu chat routing (`getFeishuChannel`, `switchFeishuChannel`). |
| `conflicts.ts` | Label generation and conflict-resolution instruction builder for git ops | `displayConflictOpLabel`, `buildResolveConflictsInstructions` | `TaskFollowUpSection.tsx`, `ConflictBanner.tsx`, `ResolveConflictsDialog.tsx` | Supports merge, cherry-pick, revert, rebase. |
| `devServerUtils.ts` | Utilities to filter and deduplicate dev-server `ExecutionProcess` records | `getDevServerWorkingDir`, `deduplicateDevServersByWorkingDir`, `filterDevServerProcesses`, `filterRunningDevServers` | `useDevServer.ts`, `usePreviewDevServer.ts`, `DevServerLogsView.tsx`, `PreviewControls.tsx` | |
| `electric/config.ts` | Creates ElectricSQL shape options with OAuth bearer-token auth headers | `createAuthenticatedShapeOptions(table)` | Only consumer: `electric/sharedTasksCollection.ts`; pulls `REMOTE_API_URL` from `remoteApi.ts` and `oauthApi` from `api.ts` | Invisible feature: real-time ElectricSQL sync subsystem. Depends on `VITE_VK_SHARED_API_BASE` env var. |
| `electric/sharedTasksCollection.ts` | Defines the TanStack DB + ElectricSQL collection for `shared_tasks` table | `sharedTasksCollection` | Consumed only by `useProjectTasks.ts` | Invisible feature: real-time shared task sync. Collection is a singleton module-level constant. |
| `mcpStrategies.ts` | JSON-path manipulation helpers for MCP server config (read/write/validate nested config blobs) | `McpConfigStrategyGeneral` class (4 static methods) | Only used by `McpSettingsNew.tsx` | Single consumer. Config path traversal is non-trivial business logic. |
| `modals.ts` | Typed wrapper around `@ebay/nice-modal-react` providing `defineModal<P,R>()` and result types | `defineModal`, `NoProps`, `Modalized<P,R>`, `ConfirmResult`, `DeleteResult`, `SaveResult`, `getErrorMessage` | Used by 47+ dialog components across the codebase | Core modal infrastructure. Very wide blast radius. |
| `openTaskForm.ts` | Thin adapter that calls `TaskFormDialog.show()` imperatively | `openTaskForm(props)` | Used by `Navbar.tsx`, `ViewRelatedTasksDialog.tsx`, `actions-dropdown.tsx` | Thin indirection — essentially a named helper to avoid importing dialog directly. |
| `paths.ts` | Centralized route-path factory for the SPA | `paths` object (8 typed route functions) | Imported by `useTaskMutations.ts`, `ClickedElementsProvider.tsx`, `TaskCard.tsx`, `TaskPanel.tsx`, `CreateAttemptDialog.tsx`, `fileTreeUtils.ts` | |
| `paths.test.ts` | Vitest unit tests for `paths.ts` | — | Tests `paths.*` fns | Test file; covers 4 of 8 exported routes (task/attempt paths untested). |
| `remoteApi.ts` | OAuth-authenticated fetch client for the remote shared API (`VITE_VK_SHARED_API_BASE`) | `REMOTE_API_URL`, `getSharedTaskAssignees(projectId)` | `useAssigneeUserName.ts` consumes `getSharedTaskAssignees`; `electric/config.ts` imports `REMOTE_API_URL` | Invisible feature: separate remote API (VK shared backend); tied to `DISABLE_NATIVE_CREDENTIALS` story. |
| `searchTagsAndFiles.ts` | Combined tag + file search aggregator (client-side tag filter + server-side file search) | `searchTagsAndFiles(query, options)`, `SearchResultItem`, `SearchOptions` | Only consumer: `file-tag-typeahead-plugin.tsx` (WYSIWYG editor) | |
| `types.ts` | Frontend-local display types for attempt execution UI | `AttemptData`, `ConversationEntryDisplayType` | `AttemptData` imported by `useAttemptExecution.ts`; `ConversationEntryDisplayType` unused outside this file | `ConversationEntryDisplayType` has 0 external imports — dead export. |
| `utils.ts` | Shared utilities: class merging (`cn`) and byte formatting | `cn(...inputs)`, `formatBytes`, `formatFileSize` | `cn` used by 172+ files; `formatFileSize` used by 2 wysiwyg components; `formatBytes` has no external importer | `formatBytes` only called internally by `formatFileSize` — redundant public export. `cn` uses only `clsx`, not `tailwind-merge`. |
| `__tests__/api-logging.test.ts` | Tests that `logApiError` is silent in test environments | — | Tests `api.ts#logApiError` | Test file only. |
| `__tests__/api-result.test.ts` | Tests `handleApiResponseAsResult` error-field precedence (`error_data` > `error`) | — | Tests `api.ts#attemptsApi.push` response parsing | Test file only. |

## Invisible Features

| Feature | File(s) | What it does | Seems used? | Note |
|---------|---------|--------------|-------------|------|
| ElectricSQL real-time sync | `electric/config.ts`, `electric/sharedTasksCollection.ts` | Streams `shared_tasks` table updates via ElectricSQL protocol, with OAuth token auth injected per-request | Yes — `useProjectTasks.ts` uses collection | Requires `VITE_VK_SHARED_API_BASE` env var. Silent no-op if var is empty string. |
| Remote VK shared API | `remoteApi.ts` | Provides read access to cross-workspace assignee data from a separate backend service | Yes — `useAssigneeUserName.ts` | Separate from the main local `/api/` backend. |
| Feishu chat routing | `conciergeApi.ts` | `getFeishuChannel`/`switchFeishuChannel` connect a Concierge session to a Feishu IM room | Yes — `FeishuChannelContainer.tsx` | Invisible integration; not surfaced in default UI. |
| Quality-gate 404 degradation | `apiVersionCompat.ts` | Silently disables quality-gate UI when backend returns 404 (older deployment) | Yes | Part of in-flight Quality Gate System A. |

## Candidates

| # | Path | Kind | Lines | Evidence | Why | Hint | Confidence | Blast Radius |
|---|------|------|-------|----------|-----|------|------------|--------------|
| 1 | `types.ts` | dead | 12-22 | `ConversationEntryDisplayType` has 0 imports outside its own definition file (grep confirms). `AttemptData` is used. | The interface is exported but never imported anywhere in the codebase. | delete | high | None — no external consumer. |
| 2 | `utils.ts` | redundant | 7-13 | `formatBytes` is only called by `formatFileSize` (line 16) in the same file. No external import found for `formatBytes`. | Public export with no consumer; `formatFileSize` fully wraps it. Callers should use `formatFileSize`. | refactor | high | Low — inline the body of `formatBytes` into `formatFileSize` or keep private. |
| 3 | `openTaskForm.ts` | stub | 1-10 | Single function, single call site pattern: 3 callers could import `TaskFormDialog.show()` directly. Only reason for indirection is to avoid a circular import or to provide a stable import path. | 3-line file with no logic beyond delegation; thin indirection layer of uncertain value. | investigate | low | Low — 3 callers; rename/redirect only. |
| 4 | `paths.test.ts` | stub | 27-34 | `paths.task()` and `paths.attempt()` are tested nowhere in the test file despite being exported and used in production. | Test coverage gap; not dead code, but the test file is incomplete relative to the module surface. | investigate | medium | None — test-only gap. |
