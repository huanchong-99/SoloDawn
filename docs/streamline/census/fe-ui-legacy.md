# fe-ui-legacy Module Map

**Scope:** `frontend/src/components/ui/` root (24 files) + `ui/table/` + `ui/shadcn-io/`
**Design System:** Legacy design (`.legacy-design` CSS class scope via `LegacyDesignScope.tsx`)

---

## File Module Map

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| `actions-dropdown.tsx` | Task/attempt context menu dropdown; "open in IDE" entrypoint for G1 | `ActionsDropdown` component | Used by `TaskCard.tsx`; calls `useOpenInEditor`, `DeleteTaskConfirmationDialog`, `ViewProcessesDialog`, `ViewRelatedTasksDialog`, `CreateAttemptDialog`, `GitActionsDialog`, `EditBranchNameDialog`, `ShareDialog`, `ReassignDialog`, `StopShareTaskDialog` | Contains G1 "Open in IDE" menu item |
| `alert.tsx` | Radix-style alert box (default/destructive/success variants) | `Alert`, `AlertTitle`, `AlertDescription` | Used in 48 files across legacy + new-design dialogs, panels, pages | Core UI primitive; heavily shared |
| `auto-expanding-textarea.tsx` | Auto-height textarea (grows up to maxRows) | `AutoExpandingTextarea` | Used by `multi-file-search-textarea.tsx`, `ScriptFixerDialog.tsx`, `ReposSettingsNew.tsx` | Utility primitive |
| `badge.tsx` | Chip/label badge (default/secondary/destructive/outline) | `Badge`, `badgeVariants` | Used in 25 files including Workflow, tasks, pipeline components | Core UI primitive |
| `button.tsx` | Polymorphic button (default/destructive/outline/secondary/ghost/link/icon variants) | `Button`, `buttonVariants` | Used in nearly all pages/components; `asChild` Slot pattern | Core UI primitive |
| `card.tsx` | Card layout (Header/Title/Description/Content/Footer) | `Card`, `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`, `CardFooter` | Used by `ExecutorConfigForm`, `WorkflowWizard`, `Workflows.tsx`, `SlashCommands.tsx` | `CardFooter`, `CardTitle`, `CardDescription` appear unused in actual call sites - only `Card`+`CardContent`+`CardHeader` used |
| `checkbox.tsx` | Custom checkbox with controlled-mode warning | `Checkbox` | Used in `CheckboxWidget` (rjsf), `CreatePRDialog`, `PrCommentsDialog`, `StartReviewDialog`, `ui-new/Dropdown`, `ui-new/ConciergeChatView`, `ui-new/CreateChatBox` | Custom implementation (not Radix CheckboxPrimitive) |
| `dialog.tsx` | Custom modal (no Radix Dialog; uses keyboard scope management) | `Dialog`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogContent`, `DialogFooter` | Used in 101 files as foundation for all modals; integrates with `keyboard/Scope.DIALOG` | Custom implementation - manages `Scope.DIALOG` hotkey scoping |
| `dropdown-menu.tsx` | Radix-based dropdown menu primitives | `DropdownMenu`, `DropdownMenuTrigger`, `DropdownMenuContent`, `DropdownMenuItem`, `DropdownMenuCheckboxItem`, `DropdownMenuRadioItem`, `DropdownMenuLabel`, `DropdownMenuSeparator`, `DropdownMenuShortcut`, `DropdownMenuGroup`, `DropdownMenuPortal`, `DropdownMenuSub`, `DropdownMenuSubContent`, `DropdownMenuSubTrigger`, `DropdownMenuRadioGroup` | Used by `actions-dropdown.tsx` and spread across 155 files (via Select) | Uses `usePortalContainer` for portal container |
| `input.tsx` | Standard text input with Cmd+Enter / Cmd+Shift+Enter shortcut props | `Input`, `InputProps` | Used in 76 files across settings, dialogs, search | Core primitive; keyboard shortcut extensions are additive |
| `json-editor.tsx` | CodeMirror-based JSON editor with lint | `JSONEditor` | Used only by `McpSettingsNew.tsx` and `AgentSettingsNew.tsx` (both new-design settings pages) | Heavy dep: `@uiw/react-codemirror`; solely in new-design settings scope |
| `label.tsx` | Radix label wrapper | `Label` | Used in form components throughout | Core primitive |
| `loader.tsx` | Spinning Loader2 with optional message | `Loader` | Used in loading states across panels/dialogs | Simple utility |
| `multi-file-search-textarea.tsx` | Textarea with debounced file search autocomplete dropdown (supports project or repo search) | `MultiFileSearchTextarea` | Used only by `ReposSettingsNew.tsx`; internally uses `AutoExpandingTextarea` + `projectsApi`/`repoApi` | Complex search UI; single caller |
| `new-card.tsx` | Alternative card layout (flex column, dashed separator header) | `NewCard`, `NewCardHeader`, `NewCardContent`, `NewCardHeaderProps` | Used by `DiffsPanel.tsx`, `TaskPanel.tsx`, `PreviewToolbar.tsx` | "New" in name but belongs to legacy scope; mixed usage with legacy panels |
| `pr-comment-card.tsx` | PR/review comment card with compact/full/list variants and diff hunk rendering | `PrCommentCard`, `PrCommentCardProps` | Used by `PrCommentsDialog.tsx` (direct) and `wysiwyg/nodes/pr-comment-node.tsx` (inline embed) | Core WYSIWYG / PR review integration |
| `select.tsx` | Radix Select with portal container support | `Select`, `SelectGroup`, `SelectValue`, `SelectTrigger`, `SelectContent`, `SelectLabel`, `SelectItem`, `SelectSeparator`, `SelectScrollUpButton`, `SelectScrollDownButton` | Widely used (155 files match `Select` pattern); uses `usePortalContainer` | Core primitive |
| `switch.tsx` | Radix Switch/toggle | `Switch` | Used in 19 files across settings, layout, panels, dialogs | Core primitive |
| `tabs.tsx` | Radix Tabs | `Tabs`, `TabsList`, `TabsTrigger`, `TabsContent` | Only referenced within its own file and `TabNavigationContext.tsx` (which re-exports from Radix directly) | **ZERO production import sites from @/components/ui/tabs** |
| `textarea.tsx` | Plain styled textarea | `Textarea` | Appears in rjsf theme and form widgets | Basic primitive |
| `toast.tsx` | Custom toast system (Context + Provider + auto-dismiss) | `ToastProvider`, `useToast` | Used in `LegacyDesignScope`, `NewDesignScope`, `Workflows.tsx`, `useWorkflows.ts`, `ui-new` containers; crosses both design systems | Infrastructure primitive - both scopes depend on it |
| `toggle-group.tsx` | Radix ToggleGroup | `ToggleGroup`, `ToggleGroupItem` | Used only by `DiffViewSwitch.tsx` | Single caller |
| `tooltip.tsx` | Radix Tooltip with portal container | `Tooltip`, `TooltipTrigger`, `TooltipContent`, `TooltipProvider` | Widely used across both legacy and new-design components | Core primitive |
| `wysiwyg.tsx` | Lexical-based WYSIWYG markdown editor (edit + read-only modes); orchestrates 12 sub-modules | `WYSIWYGEditor` (default export) | Used in 13 production files; calls `writeClipboardViaBridge` (VSCode bridge invisible feature) | Most complex component in scope; integrates VSCode bridge |
| **shadcn-io/kanban.tsx** | Thin wrapper div styled as kanban card | `KanbanCard` | Used only by `TaskCard.tsx` | R5 delete candidate; minimal abstraction |
| **table/table.tsx** | HTML table primitives | `Table`, `TableHead`, `TableBody`, `TableRow`, `TableHeaderCell`, `TableCell`, `TableEmpty`, `TableLoading` | Used internally by `data-table.tsx` | |
| **table/data-table.tsx** | Generic typed DataTable wrapper | `DataTable`, `ColumnDef`, `DataTableProps` | Used by `TaskPanel.tsx`, `ViewRelatedTasksDialog.tsx` | |
| **table/index.ts** | Re-export barrel for table sub-module | Re-exports all table + DataTable | Used by `TaskPanel.tsx` via `@/components/ui/table` | |
| **wysiwyg/context/task-attempt-context.tsx** | React contexts for taskAttemptId, taskId, localImages in WYSIWYG tree | `TaskAttemptContext`, `TaskContext`, `LocalImagesContext`, `useTaskAttemptId`, `useTaskId`, `useLocalImages`, `LocalImageMetadata` | Imported by `image-node.tsx`, `useCreateAttachments.ts`, `useImageMetadata.ts`, `useSessionAttachments.ts`, `TaskFormDialog.tsx`, `ui-new` primitives | Invisible feature: local image preview before server save |
| **wysiwyg/lib/code-highlight-theme.ts** | Shared Prism token CSS class map | `CODE_HIGHLIGHT_CLASSES` | Used by `wysiwyg.tsx` theme config and `code-highlight-plugin.tsx` | |
| **wysiwyg/lib/create-decorator-node.tsx** | Factory for Lexical DecoratorNode classes with inline/fenced markdown serialization | `createDecoratorNode`, `DecoratorNodeConfig`, `GeneratedDecoratorNode`, `InlineSerialization`, `FencedSerialization`, `SerializationConfig` | Used by `image-node.tsx` and `pr-comment-node.tsx` | Core WYSIWYG abstraction |
| **wysiwyg/nodes/image-node.tsx** | Lexical ImageNode (inline image with metadata + preview dialog) | `ImageNode`, `$createImageNode`, `$isImageNode`, `IMAGE_TRANSFORMER`, `ImageData` | Used by `wysiwyg.tsx` | Invisible feature: proxy image resolution via API |
| **wysiwyg/nodes/pr-comment-node.tsx** | Lexical PrCommentNode (fenced `gh-comment` code block → inline card) | `PrCommentNode`, `$createPrCommentNode`, `$isPrCommentNode`, `PR_COMMENT_TRANSFORMER`, `PR_COMMENT_EXPORT_TRANSFORMER`, `NormalizedComment` | Used by `wysiwyg.tsx`, `TaskFollowUpSection.tsx` | Invisible feature: PR comment embedding in WYSIWYG |
| **wysiwyg/plugins/clickable-code-plugin.tsx** | MutationObserver plugin: makes inline code matching diff file paths clickable | `ClickableCodePlugin` | Used by `wysiwyg.tsx` (read-only mode only) | Invisible feature: code-to-diff navigation |
| **wysiwyg/plugins/code-block-shortcut-plugin.tsx** | Lexical plugin: converts ``` triple-backtick typed sequence to CodeNode | `CodeBlockShortcutPlugin` | Used by `wysiwyg.tsx` | |
| **wysiwyg/plugins/code-highlight-plugin.tsx** | Registers Lexical Prism code highlighting | `CodeHighlightPlugin` | Used by `wysiwyg.tsx` | |
| **wysiwyg/plugins/file-tag-typeahead-plugin.tsx** | `@mention` typeahead for tags and file paths in WYSIWYG | `FileTagTypeaheadPlugin` | Used by `wysiwyg.tsx`; calls `searchTagsAndFiles` lib | Invisible feature: tag/file completion in editor |
| **wysiwyg/plugins/image-keyboard-plugin.tsx** | Handles Delete/Backspace on selected ImageNodes | `ImageKeyboardPlugin` | Used by `wysiwyg.tsx` | |
| **wysiwyg/plugins/keyboard-commands-plugin.tsx** | Cmd+Enter / Shift+Cmd+Enter / sendOnEnter (plain Enter) submission | `KeyboardCommandsPlugin` | Used by `wysiwyg.tsx`; reads `useUiPreferencesStore` | |
| **wysiwyg/plugins/markdown-sync-plugin.tsx** | Bidirectional controlled markdown sync (external value ↔ Lexical state) | `MarkdownSyncPlugin` | Used by `wysiwyg.tsx` | Core WYSIWYG bidirectional binding |
| **wysiwyg/plugins/read-only-link-plugin.tsx** | Link sanitization in read-only mode (blocks js:, data:; opens HTTPS externally) | `ReadOnlyLinkPlugin` | Used by `wysiwyg.tsx` | Security feature |
| **wysiwyg/plugins/toolbar-plugin.tsx** | Floating selection toolbar (Bold/Italic/Underline/Strikethrough/Code) | `ToolbarPlugin` | Used by `wysiwyg.tsx` | |
| **wysiwyg/transformers/code-block-transformer.ts** | Lexical MultilineElementTransformer for fenced code blocks (import only) | `CODE_BLOCK_TRANSFORMER` | Used by `wysiwyg.tsx` | |
| **wysiwyg/transformers/table-transformer.ts** | Lexical ElementTransformer for markdown tables | `TABLE_TRANSFORMER` | Used by `wysiwyg.tsx` | |

---

## Invisible / Background Features

| Feature | File | What It Does | Seems Used |
|---------|------|-------------|------------|
| VSCode webview bridge clipboard | `wysiwyg.tsx` line 46 + `vscode/bridge.ts` | `writeClipboardViaBridge()` — falls back gracefully if no bridge; used in WYSIWYG copy button | Yes, via bridge.ts |
| Local image preview before upload | `wysiwyg/context/task-attempt-context.tsx` + `image-node.tsx` | `LocalImagesContext` passes in-memory image blobs for immediate rendering before server save | Yes, used by TaskFormDialog, ui-new primitives |
| PR comment embedding (gh-comment fenced block) | `wysiwyg/nodes/pr-comment-node.tsx` | Embeds GitHub PR comments as inline WYSIWYG nodes with fenced `gh-comment` serialization | Yes, used by TaskFollowUpSection |
| File @-mention typeahead | `wysiwyg/plugins/file-tag-typeahead-plugin.tsx` | `@filename` triggers file/tag search dropdown inside WYSIWYG | Yes, via wysiwyg.tsx |
| Clickable code → diff navigation | `wysiwyg/plugins/clickable-code-plugin.tsx` | Makes inline `` `code` `` blocks clickable when their text matches a diff file path | Yes, active in read-only mode |
| "Open in external editor" | `actions-dropdown.tsx` + `useOpenInEditor.ts` | Calls `attemptsApi.openEditor()` which returns a URL; falls back to `EditorSelectionDialog` | Yes — G1 candidate to remove |
| Keyboard scope management | `dialog.tsx` | Activates `Scope.DIALOG`, disables `Scope.KANBAN`/`Scope.PROJECTS` when dialog opens | Yes, required for global keyboard routing |

---

## Notes on New Design System Transition

The CLAUDE.md (`frontend/CLAUDE.md`) defines a parallel `ui-new/` design system with `.new-design` CSS class scope. Most components in this scope (legacy `ui/`) are shared across both `LegacyDesignScope` and `NewDesignScope`, particularly:
- `toast.tsx` — both scopes wrap with `ToastProvider`
- `wysiwyg.tsx` — used extensively in `ui-new/` containers and primitives
- `alert.tsx`, `badge.tsx`, `button.tsx`, `dialog.tsx`, `input.tsx`, `select.tsx`, `switch.tsx`, `tooltip.tsx` — all imported by new-design components
