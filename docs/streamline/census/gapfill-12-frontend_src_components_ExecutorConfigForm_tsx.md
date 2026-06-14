# Gap-Fill Census: 12 frontend/src/components files

Generated: 2026-06-14
Branch: refactor/streamline-quality-gates

## Files Covered

| # | File | Kind |
|---|------|------|
| 1 | frontend/src/components/ExecutorConfigForm.tsx | component |
| 2 | frontend/src/components/Logo.tsx | component |
| 3 | frontend/src/components/SearchBar.tsx | component |
| 4 | frontend/src/components/TagManager.tsx | component |
| 5 | frontend/src/components/ThemeProvider.tsx | provider + hook |
| 6 | frontend/src/components/debug/TerminalDebugView.test.tsx | test |
| 7 | frontend/src/components/debug/TerminalSidebar.test.tsx | test |
| 8 | frontend/src/components/layout/ProductModeSwitch.test.tsx | test |
| 9 | frontend/src/components/ui-new/primitives/RepoCardSimple.tsx | primitive |
| 10 | frontend/src/components/ui-new/primitives/RunningDots.tsx | primitive |
| 11 | frontend/src/components/ui-new/primitives/SearchableDropdown.tsx | primitive |
| 12 | frontend/src/components/ui-new/primitives/SectionHeader.tsx | primitive |

---

## File Analyses

### 1. ExecutorConfigForm.tsx
**Purpose**: RJSF-based dynamic configuration form for executor (coding agent) types. Renders JSON Schema-driven fields with custom shadcn/ui widgets. Handles env field specially via KeyValueField. Shows save button only when `onSave` is provided.

**Public surface**: `ExecutorConfigForm({ executor, value, onSubmit, onChange, onSave, disabled, isSaving, isDirty })`

**Relations**: Used in `pages/ui-new/settings/AgentSettingsNew.tsx` (line 1080). Imports `shadcnTheme` from `./rjsf`, `virtual:executor-schemas`, `shared/types.BaseCodingAgent`.

**Candidates**: None — actively used, sole consumer of virtual executor schema module.

---

### 2. Logo.tsx
**Purpose**: Renders the SoloDawn SVG wordmark (140x28, inline CSS class "logo").

**Public surface**: `Logo()`

**Relations**: Imported only by `components/layout/Navbar.tsx`.

**Candidates**: None — simple, used.

---

### 3. SearchBar.tsx
**Purpose**: Forward-ref search input with a Search icon, renders `null` when `disabled=true`. Placeholder text uses the project name when provided.

**Public surface**: `SearchBar({ className, value, onChange, disabled, project })` — `forwardRef<HTMLInputElement>`.

**Relations**: Imported only by `components/layout/Navbar.tsx`.

**Candidates**:
- The `disabled` branch returning `null` instead of a visually-disabled input is a **dubious-feature**. It causes the component to vanish entirely rather than appear greyed out, which is unusual UX and may hide the control unexpectedly when a project is not active. Confidence: medium.

---

### 4. TagManager.tsx
**Purpose**: Full CRUD UI for @-mention tags. Fetches tags on mount, displays them in a scrollable table, uses `TagEditDialog.show()` and `ConfirmDialog.show()` for edit/delete confirmation.

**Public surface**: `TagManager()` — no props.

**Relations**: Used in `pages/ui-new/settings/GeneralSettingsNew.tsx`.

**Candidates**: None — standalone, actively used.

---

### 5. ThemeProvider.tsx
**Purpose**: React context provider for light/dark/system theme. Applies class to `document.documentElement`. Also exports `useTheme()` hook.

**Public surface**: `ThemeProvider({ children, initialTheme })`, `useTheme()`.

**Relations**: `ThemeProvider` used in `App.tsx`. `useTheme` consumed in 10+ components including AgentIcon, StyleOverride, IdeIcon, json-editor, DiffViewCard, GeneralSettingsNew, ReleaseNotesDialog, FileTreeNode, DiffViewCardWithComments, ChatFileEntry.

**Candidates**: None — core infrastructure, heavily used.

---

### 6. debug/TerminalDebugView.test.tsx
**Purpose**: Tests for `debug/TerminalDebugView.tsx` — a simplified stub version that shows terminal detail fields (role, status, modelConfigId) with a close button.

**Public surface**: Test suite only.

**Relations**: Imports `./TerminalDebugView` (the simple stub in `debug/`). There is a completely separate, production-grade `terminal/TerminalDebugView.tsx` (765 lines) with its own comprehensive test suite (`terminal/TerminalDebugView.test.tsx`, 609 lines). The `debug/` variant is NOT imported anywhere in production code. Only `pages/WorkflowDebugPage.tsx` imports from `@/components/terminal/TerminalDebugView` (the real one).

**Candidates**:
- `debug/TerminalDebugView.test.tsx` — **dead** (tests a stub component that is never used in production). The stub it tests (`debug/TerminalDebugView.tsx`) is superseded by `terminal/TerminalDebugView.tsx`. Both the stub and its test file are candidates for deletion. Confidence: high.

---

### 7. debug/TerminalSidebar.test.tsx
**Purpose**: Tests for `debug/TerminalSidebar.tsx` — a simple sidebar that shows terminal role and cliTypeId. Tests selection highlighting using `border-brand` class.

**Public surface**: Test suite only.

**Relations**: Imports `./TerminalSidebar` (stub in `debug/`). `TerminalSidebar` is not imported anywhere in production code; the production `terminal/TerminalDebugView.tsx` renders its own sidebar inline.

**Candidates**:
- `debug/TerminalSidebar.test.tsx` — **dead** (tests an unused stub). Same situation as `debug/TerminalDebugView.test.tsx`. The stub `debug/TerminalSidebar.tsx` is not used anywhere. Confidence: high.

---

### 8. layout/ProductModeSwitch.test.tsx
**Purpose**: Tests for `ProductModeSwitch` — renders at different routes and verifies `text-brand` class is applied to the active mode button.

**Public surface**: Test suite only.

**Relations**: Tests `./ProductModeSwitch`, which is used in `Navbar.tsx` (confirmed by grep not shown here but component exists at `layout/ProductModeSwitch.tsx`). Test wraps with `MemoryRouter` and `I18nextProvider`.

**Candidates**: None — valid test for an active component.

---

### 9. ui-new/primitives/RepoCardSimple.tsx
**Purpose**: Card displaying a repo name, path, optional remove button, and optional branch selector (via `SearchableDropdownContainer`).

**Public surface**: `RepoCardSimple({ name, path, onRemove, className, branches, selectedBranch, onBranchChange })`

**Relations**: Imported by `ui-new/primitives/SelectedReposList.tsx`.

**Candidates**:
- Uses `bg-tertiary` CSS token (line 28) which is **not defined** in any Tailwind config or CSS variable file (neither `tailwind.new.config.js` nor `src/styles/`). This is a **bug** — the background will silently fall back to transparent. Confidence: high. The CLAUDE.md design guide lists only `bg-primary`, `bg-secondary`, `bg-panel` as valid background tokens.

---

### 10. ui-new/primitives/RunningDots.tsx
**Purpose**: Animated three-dot loading indicator using custom Tailwind tokens (`size-dot`, `animate-running-dot-1/2/3`, `bg-brand`).

**Public surface**: `RunningDots()`

**Relations**: Imported by `ScriptFixerDialog.tsx`, `ProcessListItem.tsx`, `WorkspaceSummary.tsx`.

**Candidates**: None — small, actively used, no props needed.

---

### 11. ui-new/primitives/SearchableDropdown.tsx
**Purpose**: Stateless view layer for a searchable dropdown with virtualized list (react-virtuoso), keyboard navigation, highlight state, and badge support. Receives all state from parent.

**Public surface**: `SearchableDropdown<T>({ filteredItems, selectedValue, getItemKey, getItemLabel, onSelect, trigger, searchTerm, onSearchTermChange, highlightedIndex, onHighlightedIndexChange, open, onOpenChange, onKeyDown, virtuosoRef, contentClassName, placeholder, emptyMessage, getItemBadge })`

**Relations**: Consumed exclusively by `SearchableDropdownContainer.tsx` (the stateful wrapper). Follows the container/view architecture described in CLAUDE.md.

**Candidates**: None — correctly split per architecture rules.

---

### 12. ui-new/primitives/SectionHeader.tsx
**Purpose**: Section header bar with a title and optional icon button. Styled with new-design tokens.

**Public surface**: `SectionHeader({ title, icon, onIconClick, className })`

**Relations**: Imported by `ui-new/views/WorkspacesSidebar.tsx`. Note: `CollapsibleSectionHeader` (imported by several other views) is a separate component in `containers/`.

**Candidates**: None — used.

---

## Cross-cutting Findings

### Dead code cluster: debug/ directory
`debug/TerminalDebugView.tsx`, `debug/TerminalSidebar.tsx`, and their test files appear to be early stubs from before the full `terminal/` implementation was written. None of the four files are imported in production code. They should be deleted along with the whole `debug/` directory.

### Bug: undefined CSS token in RepoCardSimple
`bg-tertiary` on line 28 of `RepoCardSimple.tsx` has no definition in any config. This silently produces no background, which could cause layout/visual issues.

### Dubious: SearchBar null-on-disabled pattern
Returning `null` when `disabled` removes the element from the DOM entirely rather than rendering a disabled input. This prevents layout from reserving space and breaks any refs pointing to the element.

---

## toolNotes
fast-context MCP returned `resource_exhausted` errors on both parallel calls. All cross-file usage queries fell back to Grep.
