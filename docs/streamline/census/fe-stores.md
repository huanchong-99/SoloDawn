# fe-stores Census

Unit: `fe-stores`
Scope: `frontend/src/stores/` (12 files)
Branch: `refactor/streamline-quality-gates`

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `index.ts` | Barrel re-export for all stores | Re-exports everything from all store files | Imports from all 9 store modules | No logic; pure re-export. Not imported by production code — consumers import directly from store files. |
| `wsStore.ts` | WebSocket connection lifecycle; multi-workflow-scoped connections; message routing, normalization, heartbeat, reconnect | `useWsStore`, `useWsSubscription`, `useWorkflowEvents`, `WsMessage`, `WsEventType`, payload types (`TerminalCompletedPayload`, `TerminalPromptDetectedPayload`, `TerminalPromptDecisionPayload`, `QualityGateResultPayload`, `WorkflowEventHandlers`, provider payloads) | Used by: Board.tsx, Workflows.tsx, useWorkflowInvalidation, useWorkflowLiveStatus, StatusBar.tsx, useCurrentUser.ts, WorkflowDebugPage.test.tsx | Core infrastructure. Implements per-workflow ref-counted WS connections (G30-007/008), provider failover events (G08), quality.gate_result (G31), terminal prompt flow (G27). Extensively tested. |
| `conciergeWsStore.ts` | WebSocket connection for the Concierge AI chat session | `useConciergeWsStore`, `ConciergeWsEventType` | Used by: ConciergeChatContainer.tsx (the sole production consumer) | Parallel to wsStore but simpler (single-session, no ref-counting). Includes 150ms connect debounce. Not re-exported from index.ts; ConciergeChatContainer imports directly. |
| `modelStore.ts` | AI model CRUD, verification, available-model cache; calls `/api/models/list` and `/api/models/verify` | `useModelStore`, `useModelList`, `useVerifiedModels`, `useAvailableModels` | Used by: Step3Models.tsx (workflow wizard model config step) | Only 1 production caller. modelStore and the wizard steps form a tandem; if wizardStore is cut, modelStore may stay (Step3Models imports it directly). |
| `terminalStore.ts` | In-memory terminal output buffer (up to 10000 lines) per terminal instance | `useTerminalStore`, `TerminalState`, `useTerminalOutputString`, `useActiveTerminal`, `useRecentTerminalOutput` | Used by: TerminalActivityPanel.tsx (imports `useRecentTerminalOutput` directly) | 1 production caller. |
| `wizardStore.ts` | Multi-step workflow creation wizard state (7 steps, config CRUD, navigation, validation, submission) | `useWizardStore`, `useCurrentStepConfig`, `useWizardDirty` | **Zero production callers outside stores/**. Re-exported from index.ts but no component or hook imports it. | R5 DEAD CANDIDATE. WorkflowWizard.tsx uses local `useState`, not this store. |
| `workflowStore.ts` | Workflow data cache (Map of WorkflowDetailDto), active workflow selection, task/terminal status mutations | `useWorkflowStore`, `useWorkflowList`, `useActiveWorkflow` | **Zero production callers outside stores/**. Re-exported from index.ts but no component or hook imports it. | R5 DEAD CANDIDATE. App uses React Query for workflow data; this Zustand store is unused. |
| `useDiffViewStore.ts` | Persisted user preferences for diff viewer (mode unified/split, ignoreWhitespace, wrapText, transient diffPaths) | `useDiffViewStore`, `DiffViewMode`, `useDiffViewMode`, `useIgnoreWhitespaceDiff`, `useWrapTextDiff` | Used by: DiffViewCard.tsx, DiffViewSwitch.tsx, EditDiffRenderer.tsx, FileChangeRenderer.tsx, DiffCard.tsx, ChangesPanel.tsx, DiffViewCardWithComments.tsx | Persisted to localStorage under key `diff-view-preferences`. Widely used. |
| `useExpandableStore.ts` | Global ephemeral expand/collapse registry (key → boolean) | `useExpandable(key, defaultValue)` | Used by: NewDisplayConversationEntry.tsx, CollapsibleSection.tsx, CollapsibleSectionHeader.tsx, DisplayConversationEntry.tsx | Lightweight; not persisted. Distinct from `useUiPreferencesStore.expanded` (which IS persisted). |
| `useTaskDetailsUiStore.ts` | Per-task transient UI state: loading, isStopping, deletingFiles, fileToDelete | `useTaskStopping` (only export); store itself unexported | Used by: useAttemptExecution.ts and multiple containers via `useTaskStopping` | Not persisted; store instance not exported. Thin wrapper exposing only `useTaskStopping`. |
| `useUiPreferencesStore.ts` | Persisted global UI preferences + workspace-scoped panel state (sidebar visibility, pane sizes, expanded sections, context bar position, sendOnEnter, right/left panel modes) | `useUiPreferencesStore`, `useRepoAction`, `usePersistedExpanded`, `useContextBarPosition`, `usePaneSize`, `useExpandedAll`, `usePersistedCollapsedPaths`, `useWorkspacePanelState`, `PERSIST_KEYS`, type exports | Used by 30+ components/hooks/contexts | Largest UI store. Persisted to localStorage under `ui-preferences`. Covers sidebar/panel layout for the new design. |
| `__tests__/wsStore.test.ts` | Vitest test suite for wsStore | N/A (test file) | Tests: wsStore.ts | 16 test cases covering connection lifecycle, heartbeat, reconnect, multi-workflow isolation, ref-counting, payload normalization (snake_case/camelCase), sendPromptResponse flow. |

## Dead/Vestigial Candidates

| File | Kind | Evidence | Confidence |
|------|------|----------|------------|
| `wizardStore.ts` | dead | 0 production imports; WorkflowWizard.tsx uses local useState; only references are the store itself and the barrel index.ts | high |
| `workflowStore.ts` | dead | 0 production imports; app uses React Query for workflow data; only references are the store itself and the barrel index.ts | high |

## In-Flight Work Relevance

- **Quality Gate System (C)**: `wsStore.ts` exports `QualityGateResultPayload` and the `WsEventType` includes `quality.gate_result`. The `quality.gate_result` event is dispatched through `useWorkflowEvents`. This is actively wired.
- **VS Code webview bridge**: Not in stores scope (lives in `frontend/src/vscode/bridge.ts`).
- **Open in external IDE (G1)**: Not in stores scope; handled by `useOpenInEditor.ts` / `useOpenProjectInEditor.ts` hooks.
- **Planning-draft confirm→materialize / AuditPlan System B**: No evidence of this in any store file.
- **Terminal prompt interactive flow**: Fully implemented in wsStore (G27) — `terminal.prompt_detected`, `terminal.prompt_decision`, `terminal.prompt_response`, `sendPromptResponse`.
- **Provider failover (G08)**: Implemented in wsStore — `provider.switched`, `provider.exhausted`, `provider.recovered` events with normalization.
