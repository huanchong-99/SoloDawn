# Census: fe-uinew-primitives-B

Unit: frontend/src/components/ui-new/primitives/ — alphabetical second half (files 28–55 of 55)

## Module Map

| File | Purpose | Public Surface | Relations / Callers | Notes |
|---|---|---|---|---|
| SettingsCard.tsx | Titled card container for settings pages | `SettingsCard`, `SettingsCardProps` | Used in: FeishuSettingsNew, GeneralSettingsNew | Simple layout wrapper; `forwardRef` |
| SettingsInput.tsx | Labeled text/password/url input row for settings | `SettingsInput`, `SettingsInputProps` | Used in: FeishuSettingsNew, ProjectSettingsNew, ReposSettingsNew, GeneralSettingsNew | Includes inline error display |
| SettingsRow.tsx | Generic label+children row for settings | `SettingsRow`, `SettingsRowProps` | Used in: FeishuSettingsNew, ReposSettingsNew | Generic slot when SettingsInput/Toggle/Select don't fit |
| SettingsSaveBar.tsx | Sticky bottom save/discard bar | `SettingsSaveBar`, `SettingsSaveBarProps` | Used in: ProjectSettingsNew, AgentSettingsNew, ReposSettingsNew, GeneralSettingsNew | Renders null if `visible=false`; uses FloppyDiskIcon |
| SettingsSection.tsx | Thin group-with-optional-title wrapper | `SettingsSection`, `SettingsSectionProps` | Used in: all 6 settings pages | Pure layout spacing primitive |
| SettingsSelect.tsx | Labeled native `<select>` row for settings | `SettingsSelect`, `SettingsSelectProps` | Used in: McpSettingsNew, ProjectSettingsNew, ReposSettingsNew, GeneralSettingsNew | Native select with CaretDownIcon overlay |
| SettingsToggle.tsx | Accessible toggle switch row for settings | `SettingsToggle`, `SettingsToggleProps` | Used in: FeishuSettingsNew, AgentSettingsNew, ReposSettingsNew, GeneralSettingsNew | ARIA role=switch; custom pill thumb |
| SplitButton.tsx | Combined primary-action + dropdown selection button | `SplitButton<T>`, `SplitButtonOption<T>` | Used in: containers/RepoCard.tsx | Generic `T extends string`; uses Dropdown primitive |
| StatusPill.tsx | CVA-styled badge/pill for status display | `StatusPill`, `StatusPillProps` | Used in: workflow/QualityBadge.tsx | Renders `<button>` when `onClick` provided; tone variants: success/warning/info/neutral/danger/brand |
| Toolbar.tsx | Horizontal toolbar container + icon button + dropdown | `Toolbar`, `ToolbarIconButton`, `ToolbarDropdown` | Used in: ChatBoxBase, CreateChatBox, SessionChatBox, PreviewBrowser (html only) | Default fallback menu content in ToolbarDropdown (sort/group) is dead — all callers pass children |
| Tooltip.tsx | Radix tooltip with portal support | `Tooltip` | Used in: Navbar, ConciergeChatView, FileTree, IconButtonGroup, ContextBar, actions/index, CopyButton | Uses PortalContainerContext |
| ViewHeader.tsx | Responsive page header with title/eyebrow/actions/meta | `ViewHeader`, `ViewHeaderProps` | Used in: pages/Board.tsx | Only one caller; can accept arbitrary children slot |
| WorkspaceSummary.tsx | Sidebar workspace list item with status icons | `WorkspaceSummary`, `WorkspaceSummaryProps` | Used in: views/WorkspacesSidebar.tsx | Dynamic import of CommandBarDialog for workspaceActions; `isDraft` prop used for planning-draft items |
| conversation/ChatApprovalCard.tsx | Plan approval card wrapping ChatMarkdown in ChatEntryContainer | `ChatApprovalCard` | Used in: containers/NewDisplayConversationEntry.tsx (via conversation/index.ts) | variant=plan; denied state handled by ChatEntryContainer |
| conversation/ChatAssistantMessage.tsx | Thin wrapper that renders ChatMarkdown for assistant text | `ChatAssistantMessage` | Used in: containers/NewDisplayConversationEntry.tsx | Minimal — just passes through to ChatMarkdown |
| conversation/ChatEntryContainer.tsx | Collapsible card container with icon+header+expand+actions | `ChatEntryContainer`, `Variant`, `VariantConfig` | Used by: ChatApprovalCard, ChatUserMessage, (indirectly all conversation entries) | Handles plan/plan_denied/user/system variants; renders button or div header |
| conversation/ChatErrorMessage.tsx | Clickable error row (toggle truncate/expand) | `ChatErrorMessage` | Used in: containers/NewDisplayConversationEntry.tsx | `expanded` prop controls truncation |
| conversation/ChatFileEntry.tsx | File change row with optional diff view inline | `ChatFileEntry`, `DiffStats`, `FileHeaderContent` | Used in: containers/NewDisplayConversationEntry.tsx | Imports DiffViewBody+useDiffData from DiffViewCard; `onOpenInChanges` prop wired in callers |
| conversation/ChatMarkdown.tsx | Renders markdown via WYSIWYGEditor (read-only) | `ChatMarkdown` | Used by: ChatApprovalCard, ChatAssistantMessage, ChatThinkingMessage, ChatUserMessage, containers/NewDisplayConversationEntry.tsx | Integrates ChangesViewContext for code-click navigation |
| conversation/ChatScriptEntry.tsx | Script/process execution row with status dot | `ChatScriptEntry` | Used in: containers/NewDisplayConversationEntry.tsx | Calls `viewProcessInPanel` from LogsPanelContext; optional Fix button when failed |
| conversation/ChatSystemMessage.tsx | Collapsible system info row (truncate/expand) | `ChatSystemMessage` | Used in: containers/NewDisplayConversationEntry.tsx | Mirrors ChatErrorMessage shape but with InfoIcon+text-low |
| conversation/ChatThinkingMessage.tsx | Thinking/reasoning display with ChatMarkdown | `ChatThinkingMessage` | Used in: containers/NewDisplayConversationEntry.tsx | Uses ChatDotsIcon |
| conversation/ChatTodoList.tsx | Collapsible todo list from agent | `ChatTodoList` | Used in: containers/NewDisplayConversationEntry.tsx | Renders per-item status icons (completed/in_progress/cancelled/pending) |
| conversation/ChatToolSummary.tsx | Tool call summary row (forwardRef) | `ChatToolSummary` | Used in: containers/NewDisplayConversationEntry.tsx | Switches icon for Bash vs others; `isTruncated`+`onViewContent` for expand/view |
| conversation/ChatUserMessage.tsx | User message card in ChatEntryContainer | `ChatUserMessage` | Used in: containers/NewDisplayConversationEntry.tsx | `onEdit` prop wires edit button; `isGreyed` for superseded messages |
| conversation/DiffViewCard.tsx | Full-featured diff viewer card (content or unified diff) | `DiffViewCard`, `DiffViewBody`, `useDiffData`, `DiffInput` | Used in: views/ChangesPanel.tsx, containers/DiffViewCardWithComments.tsx, ChatFileEntry (imports DiffViewBody+useDiffData) | Uses @git-diff-view/react; two render modes (DiffFile object vs unified string); useDiffViewStore for split/inline mode |
| conversation/ToolStatusDot.tsx | Animated status indicator dot | `ToolStatusDot` | Used by: ChatFileEntry, ChatScriptEntry, ChatToolSummary, DiffViewCard | Pulse animation when pending; maps ToolStatus to success/error/pending colours |
| conversation/index.ts | Re-export barrel for conversation sub-package | All conversation exports | Imported by: containers/NewDisplayConversationEntry.tsx, containers/DiffViewCardWithComments.tsx | Exports DiffInput type |

## Candidate Flags

| # | File | Kind | Evidence | Disposition | Confidence |
|---|---|---|---|---|---|
| 1 | Toolbar.tsx L83-105 | dead | Default fallback sort/group/assignee/label menu inside ToolbarDropdown is rendered only when `children` is undefined. Zero call sites omit children. | refactor (delete fallback block) | high |
| 2 | ChatAssistantMessage.tsx | redundant | 13-line wrapper that only calls `<ChatMarkdown content={content} workspaceId={workspaceId} />`. Callers could import ChatMarkdown directly. | investigate | low |
| 3 | WorkspaceSummary.tsx — OpenInOldUI dispatch | legacy | `handleOpenCommandBar` opens `workspaceActions` page which includes `OpenInOldUI` action (in-flight [G1] deletion candidate per task notes) | investigate | medium |
