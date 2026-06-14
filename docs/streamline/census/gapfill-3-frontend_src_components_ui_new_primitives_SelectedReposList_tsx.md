# Gap-fill Census: ui-new/primitives — SelectedReposList, Separator, SessionChatBox

**Unit**: gapfill  
**Date**: 2026-06-14  
**Files covered**: 3

---

## SelectedReposList.tsx

**Purpose**: Presentational list component that renders a collection of selected Git repositories. Shows an empty-state illustration when `repos` is empty; otherwise maps each repo to a `RepoCardSimple` and threads through branch selection props.

**Public surface**:
- `export function SelectedReposList(props: Readonly<SelectedReposListProps>)`
- Props: `repos: Repo[]`, `onRemove: (repoId: string) => void`, `branchesByRepo?: Record<string, GitBranch[]>`, `selectedBranches?: Record<string, string>`, `onBranchChange?: (repoId: string, branch: string) => void`

**Relations**:
- Imports `RepoCardSimple` (sibling primitive)
- Imports `Repo`, `GitBranch` from `shared/types`
- Used in exactly one place: `ui-new/views/GitPanelCreate.tsx` line 91

**Candidates**: None. Focused, single caller, clearly active.

---

## Separator.tsx

**Purpose**: Thin radix-ui wrapper that renders a styled `<SeparatorPrimitive.Root>` as either a 1px horizontal rule (`h-[1px] w-full`) or vertical divider (`h-full w-[1px]`). Supports full className override via `cn()`.

**Public surface**:
- `export { Separator }` — `React.forwardRef` component typed to `SeparatorPrimitive.Root` props

**Relations**:
- Wraps `@radix-ui/react-separator`
- Consumed by exactly one file in `ui-new`: `ui-new/primitives/Field.tsx` line 170, where it is used inside `FieldSeparator` (exported as part of Field's API)
- No other import in the codebase uses the radix-based `Separator` from this path (the `WorkspacesLayout.tsx` reference to `Separator` imports from `react-resizable-panels`, not this file)
- No barrel/index re-export

**Candidates**:
- Narrow usage (one consumer). Not dead — `FieldSeparator` is exported and in active use. However, this component is a near-verbatim copy of the shadcn/ui `Separator` template. If the project later adds a `ui/separator.tsx` (old design system), there would be duplication. For now it is the sole radix separator wrapper in `ui-new`.

---

## SessionChatBox.tsx

**Purpose**: Full-featured chat input box for "session mode" (an existing task/conversation). Orchestrates five distinct interaction modes (idle, running/queued, feedback, edit, approval) and composes them onto `ChatBoxBase`. Contains all stateless rendering logic for: session switching dropdown, executor/model selection (new-session mode), file stats header, review-comment banner, queued-message banner, file attachment, and toolbar action buttons.

**Public surface**:
- `export function SessionChatBox(props: Readonly<SessionChatBoxProps>)`
- `export type ExecutionStatus` — union of 7 status strings; consumed by `SessionChatBoxContainer`
- `export type { EditorProps, VariantProps }` — re-exported from `ChatBoxBase` for container use

**Relations**:
- Imports `ChatBoxBase`, `VisualVariant`, `EditorProps`, `VariantProps` from `./ChatBoxBase`
- Imports `PrimaryButton`, `ToolbarIconButton`, `ToolbarDropdown`, `DropdownMenuItem`, `DropdownMenuLabel`, `DropdownMenuSeparator` from sibling primitives
- Imports `ActionDefinition`, `ActionVisibilityContext`, `isSpecialIcon` from `../actions`
- Imports `isActionEnabled` from `../actions/useActionVisibility`
- Imports `AgentIcon` from `@/components/agents/AgentIcon`
- Imports `ExecutorProps` from `./CreateChatBox`
- Consumed exclusively by `SessionChatBoxContainer.tsx` (lines 592 and 627 — two render paths: placeholder mode and full mode)
- `SessionChatBoxContainer` is used in `WorkspacesMain.tsx`

**Candidates**:
- Hardcoded English placeholder strings (`'Provide feedback for the plan...'`, `'Edit your message...'`, `'Provide feedback to request changes...'`, `'Start a new conversation...'`, `'Continue working on this task...'`) inside `getPlaceholder()` are not passed through `useTranslation`. The rest of the file uses `t(...)`. This is a localization gap / bug.
- Lines 215-223, confidence: medium. These 5 strings bypass i18n while the surrounding code uses `t('tasks', ...)`. If the app is multi-locale they will always appear in English.

---

## Summary Table

| File | Kind | Status | Note |
|------|------|--------|------|
| SelectedReposList.tsx | active primitive | keep | single caller, clear purpose |
| Separator.tsx | active primitive | keep | one consumer (Field.tsx); narrow but used |
| SessionChatBox.tsx | active primitive | keep + investigate hardcoded strings | i18n bug in `getPlaceholder()` |
