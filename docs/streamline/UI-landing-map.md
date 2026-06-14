# UI Landing Map — Workspace Create Mode + Context Bar

**Branch:** main  
**Date mapped:** 2026-06-14  
**Purpose:** Read-only mapping for upcoming redesign. No code was changed.

---

## 1. Create-Mode Landing (`/workspaces/create`, no draft)

### Route & render tree

```
Workspaces (page)
  WorkspacesLayout
    ModeProvider (isCreateMode=true → wraps in CreateModeProvider)
      NavbarContainer
      WorkspacesLayout left-main Panel
        CreateChatBoxContainer   ← the component of interest
```

The route `/workspaces/create` is detected by `useWorkspaceContext` → sets `isCreateMode = true` → `WorkspacesLayout` renders `<CreateChatBoxContainer />` in the left-main panel instead of the normal conversation view.

### File

`frontend/src/components/ui-new/containers/CreateChatBoxContainer.tsx`

### Gate: `hasInitialValue`

Line 685 of `CreateChatBoxContainer.tsx`:

```tsx
if (!hasInitialValue) return null;
```

`hasInitialValue` comes from `useCreateModeState` (via `CreateModeContext`). It is set to `true` only after the `useScratch` async load completes and all required data (projects, profiles) is available. Until that resolves, **`CreateChatBoxContainer` renders nothing** (pure `null`).

### Gate: no `projectId`

Once `hasInitialValue` is true, if `selectedProjectId` is still `null` (no project auto-selected yet), the component renders a centered no-project prompt:

```tsx
<div className="flex h-full w-full items-center justify-center">
  <div className="text-center max-w-sm">
    <h2 className="text-lg font-medium text-high mb-2">
      {t('workspace.selectProjectTitle')}
    </h2>
    <p className="text-sm text-low">
      {t('workspace.selectProjectHint')}
    </p>
  </div>
</div>
```

**Exact copy (en locale, `common.json`):**

| Key | Copy |
|-----|------|
| `workspace.selectProjectTitle` | `Select a Project` |
| `workspace.selectProjectHint` | `Choose an existing project or create a new one from the right panel to get started.` |

This is a transient state; auto-selection picks the most recently created project from `projectsById` and sets it immediately.

### Normal empty state — the `CreateChatBox`

Once `hasInitialValue = true` and a `projectId` is available AND `planningDraftId` is `null` (no draft yet), the component renders:

```
flex-1 flex flex-col justify-end
  flex justify-center @container
    CreateChatBox
```

`CreateChatBox` (`frontend/src/components/ui-new/primitives/CreateChatBox.tsx`) renders `ChatBoxBase` with:

- **Placeholder text:** `"Describe the task..."` (hardcoded string in `CreateChatBox.tsx` line 113)
- **Header left toolbar:**
  1. `AgentIcon` — icon for the selected executor agent
  2. Executor `ToolbarDropdown` — label is `toPrettyCase(executor.selected)` or `"Select Executor"` if none selected; opens a dropdown listing all available executors from `profiles`
  3. (Conditional) Model `ToolbarDropdown` — visible only when `availableModels.length > 0`; label is the selected model's `displayName` or `t('conversation.selectModel')` (`"Select Model"`)
  4. (Conditional) Variant `ToolbarDropdown` — visible when `effectiveProfile` is set; not rendered in create mode without a profile
  5. (Conditional) "Save as default" checkbox — visible only when user changed executor from their saved default (`hasChangedFromDefault`)
- **Footer left:** Paperclip icon button (`Attach file`) — opens hidden file input for image upload
- **Footer right:** `PrimaryButton`
  - Label when idle: `t('conversation.workspace.create')` = `"Create"`
  - Label when submitting: `t('conversation.workspace.creating')` = `"Creating..."`
  - Disabled when `editor.value.trim().length === 0` or `isSending`
- **Error banner:** Shown below the editor when `displayError` is non-null (e.g., "Add at least one repository to start planning" if user submitted without repos)

**There are no pre-built "quick-action" / suggestion chip buttons** on the landing empty state. The landing is purely the chat input box positioned at the bottom of the flex column, with the executor dropdown as the only interactive affordance outside of typing.

### Submit flow (Phase 1 → Phase 2)

Pressing "Create" (or Cmd+Enter) calls `handleInitialSubmit`:
1. Calls `planningDraftsApi.create(...)` → creates a planning draft, stores its ID in URL as `?draftId=<id>`
2. Immediately calls `planningDraftsApi.sendMessage(draft.id, message)` to send the first message
3. Once `planningDraftId` is set, the view switches to **planning conversation mode** (Phase 2) — the `PlanningStatusBar` appears at the top, a message list fills the center, and the `AuditDocPanel` slides in on the right

---

## 2. Context Bar

### Files

- Container: `frontend/src/components/ui-new/containers/ContextBarContainer.tsx`
- Primitive: `frontend/src/components/ui-new/primitives/ContextBar.tsx`
- Action definitions: `frontend/src/components/ui-new/actions/index.ts` — `ContextBarActionGroups`

### Position & appearance

A floating pill (`absolute z-50`, `bg-secondary/50 backdrop-blur-sm`) positioned by `useContextBarPosition`. It has a drag handle (3-dot grip) at the top that lets users reposition it. Layout is a vertical column of icon buttons with tooltips on the left side.

### `ContextBarActionGroups` (defined in `actions/index.ts` lines 944–952)

```ts
export const ContextBarActionGroups = {
  primary: [Actions.CopyPath],
  secondary: [
    Actions.ToggleDevServer,
    Actions.TogglePreviewMode,
    Actions.ToggleChangesMode,
  ],
};
```

#### Primary items (rendered above the separator)

| # | Action ID | Label | Icon | Visibility | What it does |
|---|-----------|-------|------|------------|--------------|
| 1 | `copy-path` | `Copy path` | Special: `copy-icon` (CopyButton) | Only when `hasWorkspace = true` | Copies `ctx.containerRef` (the workspace container path string) to the clipboard via `navigator.clipboard.writeText`. Renders as a self-contained `CopyButton` with its own feedback state rather than a plain icon button. |

#### Secondary items (rendered below the separator)

| # | Action ID | Label (dynamic) | Icon (dynamic) | Visibility | Enabled | What it does |
|---|-----------|-----------------|----------------|------------|---------|--------------|
| 2 | `toggle-dev-server` | "Start Dev Server" / "Stop Dev Server" | `PlayIcon` / `PauseIcon` / `SpinnerIcon` (when starting/stopping) | Only when `hasWorkspace = true` | Disabled while `devServerState` is `'starting'` or `'stopping'` | Toggles the dev server. When starting: calls `ctx.startDevServer()` and auto-switches right panel to PREVIEW mode. When stopping: calls `ctx.stopDevServer()`. Icon spins (animate-spin) during transitions; turns `text-error` (red) when running. Tooltip: `"Start dev server"` / `"Stop dev server"` / `"Starting dev server..."` / `"Stopping dev server..."` |
| 3 | `toggle-preview-mode` | "Show/Hide Preview Panel" | `DesktopIcon` | Hidden in create mode (`isCreateMode = true`) | Disabled in create mode | Toggles `RIGHT_MAIN_PANEL_MODES.PREVIEW`. Active (highlighted) when the right panel is in PREVIEW mode. |
| 4 | `toggle-changes-mode` | "Show/Hide Changes Panel" | `GitDiffIcon` | Hidden in create mode (`isCreateMode = true`) | Disabled in create mode | Toggles `RIGHT_MAIN_PANEL_MODES.CHANGES`. Active (highlighted) when the right panel is in CHANGES (diff) mode. |

**Note:** In create mode (`isCreateMode = true`), items 3 and 4 are hidden (`isVisible` returns `false`), and item 2 is also hidden because `hasWorkspace` is `false`. This means **the ContextBar has no visible items during the create-mode landing** — it renders an empty pill that the user would still see due to the drag handle.

### Rendering logic

`ContextBarContainer` computes `primaryItems` and `secondaryItems` by calling `filterVisibleItemPair` (from `NavbarContainer`) which filters out items where `isVisible(actionCtx)` is `false`. The filtered arrays are passed to `<ContextBar>` which renders them as icon-only buttons in a vertical stack, each wrapped in a `Tooltip` showing the resolved label on the left.

The `onExecuteAction` handler only fires for actions where `requiresTarget === false`; workspace/git actions (which require a selected workspace) are not wired here.

---

## Key i18n keys referenced

All from `en` locale:

| Namespace | Key | Value |
|-----------|-----|-------|
| `common` | `workspace.selectProjectTitle` | `Select a Project` |
| `common` | `workspace.selectProjectHint` | `Choose an existing project or create a new one from the right panel to get started.` |
| `tasks` | `conversation.workspace.create` | `Create` |
| `tasks` | `conversation.workspace.creating` | `Creating...` |
| `tasks` | `conversation.executors` | `Executors` |
| `tasks` | `conversation.selectModel` | `Select Model` |
| `tasks` | `conversation.customModels` | `Custom Models` |
| `tasks` | `conversation.officialModels` | `Official Models` |
| `tasks` | `conversation.saveAsDefault` | `Save as default` |
| `tasks` | `conversation.planning.needRepo` | `Add at least one repository to start planning` |
| `tasks` | `conversation.planning.title` | `Workspace Planner` |
| `tasks` | `conversation.planning.thinking` | `Planner is thinking...` |
| `tasks` | `conversation.planning.confirmButton` | `Confirm Plan` |
| `tasks` | `conversation.planning.materializeButton` | `Create Workflow` |
| `tasks` | `conversation.actions.send` | `Send` |
