# Census: fe-hooks-session-conv

Unit: `fe-hooks-session-conv`
Scope: `frontend/src/hooks/` — session/conversation/message/planning/draft/orchestrator hooks

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `useConversationHistory.ts` | Orchestrates streaming/loading of all ExecutionProcess entries for a workspace (Workspace). Manages WebSocket streams, batched historic loading, and emits flat PatchTypeWithKey arrays via callback. | `useConversationHistory(params)`, `PatchTypeWithKey`, `AddEntryType`, `OnEntriesUpdated` | Called by `ConversationListContainer`. Consumes `ExecutionProcessesContext`, `streamJsonPatchEntries`. Types re-used by `EntriesContext`, `useTodos`. | Largest/most complex hook (955 lines). Contains M51 generation-counter fix for reset vs initial-load race. ExitPlanMode detection emits `'plan'` add-type. |
| `useMessageEditRetry.ts` | Wraps `sessionsApi.followUp` with a modal gate (RestoreLogsDialog) for edit+retry of a specific executionProcess. | `useMessageEditRetry(sessionId, onSuccess?, onError?)`, `MessageEditRetryParams`, `EditDialogCancelledError` | Used by `SessionChatBoxContainer` (new-design). Near-duplicate of `useRetryProcess`. | See duplicate note below. |
| `usePlanningDraft.ts` | Full planning-draft lifecycle: list/get/messages/send/confirm/materialize/audit-doc/feishu-sync/workspace-messages. Central API layer for the Planning Draft feature. | `planningDraftKeys`, `usePlanningDrafts`, `usePlanningDraft`, `usePlanningDraftMessages`, `useSendPlanningMessage`, `useConfirmDraft`, `useMaterializeDraft`, `useUploadAuditDoc`, `useDeleteAuditDoc`, `useTogglePlanningFeishuSync`, `useWorkspacePlanningMessages` | Used by `CreateChatBoxContainer`, `PlanningChatContainer`, `AuditDocPanel`, `FeishuChannelContainer`, `WorkspacesMainContainer`, `WorkspacesSidebarContainer`. | Implements the confirm->materialize flow (System B AuditPlan). Feishu sync is an invisible background feature. |
| `useSessionAttachments.ts` | Image upload for session follow-up. Uploads files to workspace, returns markdown string and LocalImageMetadata list for WYSIWYG preview. | `useSessionAttachments(workspaceId, onInsertMarkdown)` returning `{uploadFiles, localImages, clearUploadedImages}` | Used by `SessionChatBoxContainer`. | Only images (filters `f.type.startsWith('image/')`) — non-image files silently dropped. |
| `useSessionMessageEditor.ts` | Draft persistence for message editor. Manages local message state with debounced save to scratch, syncs on load, resets on scratchId change. | `useSessionMessageEditor({scratchId})` returning full editor state + handlers | Used by `SessionChatBoxContainer`. Depends on `useScratch`, `useDebouncedCallback`. | ScratchType.DRAFT_FOLLOW_UP payload. 500ms debounce. |
| `useSessionQueueInteraction.ts` | Queue a message for later execution when agent is busy; cancel queued message. TanStack Query wrapper around `queueApi`. | `useSessionQueueInteraction({sessionId})` returning `{isQueued, queuedMessage, isQueueLoading, queueMessage, cancelQueue, refreshQueueStatus}` | Used only by `SessionChatBoxContainer`. `TaskFollowUpSection` (old-design) reimplements same logic inline. | Duplicate/redundant with inline queue logic in `TaskFollowUpSection` lines 438-487. Same `QUEUE_STATUS_KEY`, same mutations, same structure. |
| `useSessionSend.ts` | Sends a message to create a new session or follow-up an existing session. Returns boolean for success/failure; no prompt composition. | `useSessionSend({...options})` returning `{send, isSending, error, clearError}` | Used only by `SessionChatBoxContainer`. Sibling of `useFollowUpSend` (old-design, used by `TaskFollowUpSection`). | Self-documents the difference from `useFollowUpSend` in JSDoc. New-design only. |

## Candidates

### C1 — `useMessageEditRetry` vs `useRetryProcess`: near-duplicate pair

Both hooks:
- Import `RestoreLogsDialog` and show the same modal
- Call `sessionsApi.followUp` with same params
- Define a local `CancelledError` class
- Have identical `onError` suppression pattern

Difference: `useMessageEditRetry` is used in `SessionChatBoxContainer` (new-design); `useRetryProcess` is used in `RetryEditorInline` (old `NormalizedConversation` component). The two differ only in error class name and comment ("edit" vs "retry").

### C2 — `useSessionQueueInteraction`: logic duplicated in `TaskFollowUpSection`

`TaskFollowUpSection` (lines 438–487) manually implements the same `useQuery` + two `useMutation` pattern with identical `QUEUE_STATUS_KEY`, same `queueApi` calls, and same cache-update pattern. `useSessionQueueInteraction` is the extracted hook version used only by `SessionChatBoxContainer`.

### C3 — `usePlanningDraft.useTogglePlanningFeishuSync`: invisible Feishu sync feature

Exposed as a hook export but the actual Feishu toggle is also called directly via `planningDraftsApi.toggleFeishuSync` in `FeishuChannelContainer` (bypassing the hook). The hook is used in `CreateChatBoxContainer`.

## Invisible Features

- **ExitPlanMode detection** (`useConversationHistory` line 659): When the last streamed entry has `tool_name === 'ExitPlanMode'`, the emit type shifts to `'plan'` — triggers a special UI mode in ConversationList. Not surfaced as a button; purely reactive to backend tool output.
- **Planning Draft confirm→materialize flow** (`usePlanningDraft`): `useConfirmDraft` + `useMaterializeDraft` implements System B AuditPlan two-step flow. `useUploadAuditDoc`/`useDeleteAuditDoc` feed audit documents to the draft before confirmation.
- **Feishu sync** (`useTogglePlanningFeishuSync`): Bi-directional Feishu channel sync for planning drafts — enabled via `FeishuChannelContainer` or `CreateChatBoxContainer`. Background sync with optional history backfill.
- **Message queue** (`useSessionQueueInteraction`): Allows queuing one follow-up message while the agent is still running; the backend holds it and dispatches after the current process finishes. Not visible in the main UI unless agent is running.
- **M51 race-condition guard** (`useConversationHistory`): Generation counter (`resetGenerationRef`) prevents stale async loads from overwriting newer attempt state. An invisible correctness mechanism.
