# Module Map: fe-uinew-conversation

**Scope:** `frontend/src/components/ui-new/primitives/conversation/` (15 files)
**Branch:** refactor/streamline-quality-gates
**Audited:** 2026-06-14

## File Inventory

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `index.ts` | Barrel re-export for the entire conversation primitives subfolder | Re-exports all 13 named exports + `DiffInput` type | Imported by `NewDisplayConversationEntry.tsx` (bulk import) | Clean barrel; no logic |
| `ChatEntryContainer.tsx` | Collapsible card shell shared by user/plan/system message types; handles variant-based styling and optional actions footer | `ChatEntryContainer` (React component) | Used by `ChatApprovalCard`, `ChatUserMessage` (internal); no external callers beyond those two | `plan_denied` variant is internal — computed from `status.status === 'denied'`, never passed as a prop directly |
| `ChatApprovalCard.tsx` | Renders plan-presentation and generic pending-approval tool entries as an expandable card | `ChatApprovalCard` | Wraps `ChatEntryContainer` + `ChatMarkdown`; consumed by `NewDisplayConversationEntry` (`PlanEntry`, `GenericToolApprovalEntry`) | |
| `ChatAssistantMessage.tsx` | Thin pass-through wrapper for assistant text; renders markdown | `ChatAssistantMessage` | Delegates to `ChatMarkdown`; consumed by `NewDisplayConversationEntry` (`AssistantMessageEntry`) | Could be inlined — it is a 3-line wrapper |
| `ChatUserMessage.tsx` | Expandable user-message card with optional inline edit button | `ChatUserMessage` | Wraps `ChatEntryContainer` + `ChatMarkdown`; consumed by `NewDisplayConversationEntry` (`UserMessageEntry`) | `isGreyed` pass-through prop for multi-message edit mode |
| `ChatErrorMessage.tsx` | Single-line error display; click to expand full error text | `ChatErrorMessage` | Consumed by `NewDisplayConversationEntry` (`ErrorMessageEntry`) | |
| `ChatSystemMessage.tsx` | Single-line system-info display; click to expand full text | `ChatSystemMessage` | Consumed by `NewDisplayConversationEntry` (`SystemMessageEntry`) | |
| `ChatThinkingMessage.tsx` | Renders LLM extended-thinking blocks | `ChatThinkingMessage` | Wraps `ChatMarkdown`; consumed directly by `NewDisplayConversationEntry` | |
| `ChatTodoList.tsx` | Collapsible checklist for `todo_management` tool results | `ChatTodoList` | No external deps beyond icons + i18n; consumed by `NewDisplayConversationEntry` (`TodoManagementEntry`) | |
| `ChatToolSummary.tsx` | Compact summary row for generic tool calls; `forwardRef` to measure text truncation | `ChatToolSummary` (forwardRef) | Wraps `ToolStatusDot`; consumed by `NewDisplayConversationEntry` (`ToolSummaryEntry`) | Uses `forwardRef` so the parent can read `scrollWidth` for truncation detection |
| `ChatFileEntry.tsx` | File-edit row with optional expandable inline diff | `ChatFileEntry` | Imports `DiffViewBody`, `useDiffData`, `DiffInput` from `DiffViewCard`; imports `ToolStatusDot`; consumed by `NewDisplayConversationEntry` (`FileEditEntry`) | Has internal `DiffStats` helper (not exported). `onOpenInChanges` callback links to ChangesPanel |
| `ChatScriptEntry.tsx` | Script execution row with status dot, log-panel link, and optional "Fix" button | `ChatScriptEntry` | Imports `ToolStatusDot`; uses `useLogsPanel` context; consumed by `NewDisplayConversationEntry` (`ScriptEntryWithFix`) | |
| `ChatMarkdown.tsx` | Rich-text markdown renderer backed by `WYSIWYGEditor`; hooks into ChangesView context for code-click navigation | `ChatMarkdown` | Imports `WYSIWYGEditor` (legacy `ui/wysiwyg`), `useChangesView`; consumed by `ChatApprovalCard`, `ChatAssistantMessage`, `ChatUserMessage`, `ChatThinkingMessage` | |
| `DiffViewCard.tsx` | Full diff viewer (header + expandable body) wrapping `@git-diff-view/react`; also exports `DiffViewBody` sub-component and `useDiffData` hook | `DiffViewCard`, `DiffViewBody`, `useDiffData`, `DiffInput` (type) | `DiffViewBody` + `useDiffData` consumed by `ChatFileEntry`; `DiffInput` type consumed by `NewDisplayConversationEntry`; `DiffViewCard` itself has NO production import outside the barrel | `DiffViewCard` (full component) is dead — no caller outside the index re-export. `DiffViewCardWithComments` (container) is the live equivalent. `useDiffData` also has a local copy in `DiffViewCardWithComments`. |
| `ToolStatusDot.tsx` | Animated status indicator dot (success/error/pending states) | `ToolStatusDot` | Consumed internally by `ChatToolSummary`, `ChatFileEntry`, `ChatScriptEntry`; also imported directly by `DiffViewCardWithComments` | |

## Cross-cutting Notes

- **Single external consumer:** All 13 non-index files funnel through a single container: `NewDisplayConversationEntry.tsx`. The index barrel is imported there once. `ToolStatusDot` has a second direct import in `DiffViewCardWithComments`.
- **`DiffViewCard` (full component) is dead.** `DiffViewCardWithComments` superseded it for the ChangesPanel. Conversation-side file edits use `DiffViewBody` (sub-component) embedded inside `ChatFileEntry`, not the full `DiffViewCard` wrapper.
- **`useDiffData` is duplicated.** Two independent implementations exist: one in `DiffViewCard.tsx` (handles both `content` and `unified` types, including stats), one in `DiffViewCardWithComments.tsx` (handles only `content` type for comment support). They share the same name but diverge in capability.
- **`parseDiffStats` duplication in container.** `NewDisplayConversationEntry` re-implements diff stat counting (`parseParsedDiffStats`/`parseFallbackDiffStats`) independently from `useDiffData`, because it needs stats before constructing the `DiffInput` for `ChatFileEntry`.
- **G1 relevance (open-in-editor):** `OpenInIdeButton` appears in `DiffViewCardWithComments` (container, not in this scope) but is absent from `ChatFileEntry`. The `onOpenInChanges` prop in `ChatFileEntry` opens the file in the Changes Panel, not in an external IDE — these are distinct features.
- **No Quality Gate System A/B references** exist in any of these 15 files.
