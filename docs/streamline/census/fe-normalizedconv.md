# Census: fe-normalizedconv

Unit: `frontend/src/components/NormalizedConversation/` (8 files)
Branch: `refactor/streamline-quality-gates`
Date: 2026-06-14

## Module Map

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| `DisplayConversationEntry.tsx` | Top-level dispatcher: switches on `NormalizedEntry.entry_type` to render the correct sub-component for each conversation event type | `default DisplayConversationEntry` (FC); many private helpers (`MessageCard`, `CollapsibleEntry`, `PlanPresentationCard`, `ToolCallCard`, `ScriptToolCallCard`, `LoadingCard`) | Imports all 4 sibling leaf components; consumed only by `ui-new/containers/NewDisplayConversationEntry.tsx` (wraps it) | 1030 lines; all private sub-components are module-local |
| `EditDiffRenderer.tsx` | Renders a unified diff using `@git-diff-view/react`; collapsible header shows path + ±counts | `default EditDiffRenderer` | Called only by `FileChangeRenderer` | Shares CSS override files with FileContentView |
| `FileChangeRenderer.tsx` | Routes a `FileChange` variant (edit/write/delete/rename) to the appropriate renderer or text label | `default FileChangeRenderer` | Calls `EditDiffRenderer` (edit) and `FileContentView` (write); called only by `DisplayConversationEntry.renderToolUseBody` | Handles denied/timed_out status with icon substitution |
| `FileContentView.tsx` | Syntax-highlighted read-only view of a whole new file's content (used for `write` changes); reuses `@git-diff-view/react` for uniform styling | `default FileContentView` | Called only by `FileChangeRenderer` | Uses `generateDiffFile` with empty old content to produce "all added" view |
| `NextActionCard.tsx` | Post-completion action toolbar: diff summary button, Try Again / Run Setup button, open-in-IDE, dev-server start/stop, copy container ref, git actions, view dev logs | `export function NextActionCard` | Calls `useOpenInEditor`, `useDiffSummary`, `useDevServer`, `useHasDevServerScript`; opens `ViewProcessesDialog`, `CreateAttemptDialog`, `GitActionsDialog`; called by `DisplayConversationEntry` for `next_action` entry type | **Relevant to G1** (open-in-IDE button on every finished attempt); also surfaces dev-server toggle (invisible when no dev script) |
| `PendingApprovalEntry.tsx` | Wraps a tool-use card with approve/deny UI; manages countdown timer, hotkey scope switching (APPROVALS scope), and posts to `approvalsApi.respond` | `default PendingApprovalEntry` | Reads `TabNavContext`, `ApprovalFormContext`, keyboard hooks `useKeyApproveRequest`/`useKeyDenyApproval`; called by `DisplayConversationEntry` for `tool_use` with `pending_approval` status | Hotkey scope management (APPROVALS vs KANBAN) is an invisible behavioral feature |
| `RetryEditorInline.tsx` | Inline WYSIWYG editor for editing + resending a user message as a forked attempt; handles image paste/upload | `export function RetryEditorInline` | Calls `useRetryProcess`, `useAttemptExecution`, `useBranchStatus`, `imagesApi.uploadForAttempt`; used only by `UserMessage` | Only active when `BaseAgentCapability.SESSION_FORK` is available |
| `UserMessage.tsx` | Renders a user message bubble; shows inline retry editor when the message is clicked for retry | `default UserMessage` | Imports `RetryEditorInline`; checks `BaseAgentCapability.SESSION_FORK` via `useUserSystem`; called by `DisplayConversationEntry` | Greying logic for pre-fork messages is coordinated via `RetryUiContext` |

## Candidate Flags

| # | File | Lines | Kind | Evidence | Disposition | Confidence | Blast Radius |
|---|------|-------|------|----------|-------------|------------|--------------|
| C1 | `NextActionCard.tsx` | 186-234 (`FileActionToolbar`) + 269-307 (`handleOpenInEditor`) | dubious-feature | **open-in-editor button** is G1 deletion candidate; `useOpenInEditor` API call triggers `attemptsApi.openEditor`. The button is present in `FileActionToolbar` every time `fileCount > 0`. Multiple other callsites exist (`actions-dropdown`, `DiffViewCardWithComments`, `FollowUpConflictSection`) so the hook is not dead, but this specific surface may be targeted for removal. | investigate | medium | Removing just the IDE button from `FileActionToolbar` is self-contained; the hook remains used elsewhere |
| C2 | `DisplayConversationEntry.tsx` | 687-758 (`SCRIPT_TOOL_NAMES`, `getScriptType`, `ScriptToolCallCard`) | redundant | Near-duplicate of the same `SCRIPT_TOOL_NAMES` set + `ScriptFixerDialog.show` logic in `ui-new/containers/NewDisplayConversationEntry.tsx` lines 174-183 & 701. Both code paths coexist because the old component is still wrapped by the new one; divergence will cause bugs if one side is updated. | refactor | high | Both call `ScriptFixerDialog.show`; merge to one callsite when old entry renderer is removed |
| C3 | `DisplayConversationEntry.tsx` | `renderEntryBody` (870-892) default case | legacy | The `default` branch of the entry-type switch reaches `renderEntryBody` only when `entry_type.type` is an unknown value **and** it is a `NormalizedEntry`. In practice all known types are handled by named cases; this path is defensive dead code unless a new entry type is added without updating the switch. | keep | low | Not a removal candidate; serves as future-proofing catch-all |

## Invisible Features

| Feature | What it does | Seems used | Note |
|---------|-------------|-----------|------|
| Approval hotkey scopes | `PendingApprovalEntry` actively enables `Scope.APPROVALS` and disables `Scope.KANBAN` while a tool approval is pending; on unmount it restores the previous scope | Yes, always active when `tool_use` status is `pending_approval` | Not visible in UI; behavior is purely keyboard-driven |
| Dev-server start/stop inside NextActionCard | Play/Pause button calls `useDevServer(attemptId).start()/stop()` to control a dev server process per-workspace | Yes, when `projectHasDevScript` is true | Button is hidden entirely if no dev-server process exists |
| Container ref copy (`containerRef`) | CopyIcon button copies the workspace `containerRef` string to clipboard; shown only when `containerRef` is non-null | Conditionally visible | Used for SSH/Docker container access workflows |
| `needsSetup` / `SETUP_HELPER` capability | When `entryType.needs_setup` is set, `NextActionCard` shows a "Run Setup" button that calls `attemptsApi.runAgentSetup` instead of "Try Again"; rendered only when agent has `BaseAgentCapability.SETUP_HELPER` | Active when backend emits `needs_setup: true` | Setup-helper flow is agent-capability-gated |
| Image paste / upload in retry editor | `RetryEditorInline` intercepts paste events, uploads images to the container via `imagesApi.uploadForAttempt`, and injects markdown image refs | Yes, always available when retry editor is open | Not advertised in UI beyond a paperclip button |

## In-flight Work Relevance

- **G1 (open in external IDE / editor)**: `NextActionCard.tsx` lines 186-195 contain the `handleOpenInEditor` → `useOpenInEditor` button. This is the most direct frontend exposure of the feature within this scope. The hook itself is not dead (5 callsites across the app); only the button in `FileActionToolbar` would be targeted.
- **VS Code webview bridge**: No evidence in this scope. No `postMessage` or `acquireVsCodeApi` calls.
- **Quality Gate System A**: Not referenced in this scope.
- **planning-draft / AuditPlan System B**: `PlanPresentationCard` in `DisplayConversationEntry.tsx` (lines 402-471) renders `plan_presentation` tool-use entries; these appear to be agent plan outputs, not the AuditPlan System B materialization flow.
