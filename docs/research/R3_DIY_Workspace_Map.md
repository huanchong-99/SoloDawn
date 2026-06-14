# R3 — Custom DIY Workspace (Manual Workflow / Advanced Mode): Full Map

**Date:** 2026-06-13  
**Scope:** Frontend (`frontend/src`) + relevant backend routes (`crates/server/src/routes/workflows*`)

---

## 1. What the DIY Workspace Is

The "DIY Workspace" (also called "Manual Workflow" or "diy" execution mode) is the mode where the **user manually defines tasks, branches, and per-terminal agent assignments** before workflow execution. The workflow is created via a 7-step wizard (`WorkflowWizard`) with `executionMode = 'diy'`.

Contrast with **Orchestrated mode** (`agent_planned`): the LLM orchestrator dynamically plans tasks at runtime; Steps 2 (Tasks) and 4 (Terminals) are skipped from the wizard; tasks is always `[]` in the creation request.

The DIY mode exposes the workflow's structural configurability: N tasks × M terminals per task, each terminal bound to a specific CLI type and AI model. Users also configure slash commands (Step 5) and advanced settings (orchestrator model, error terminal, merge terminal, git watcher, target branch — Step 6) that apply to both modes.

---

## 2. Route / Entry Points

From `frontend/src/App.tsx`:

```
/board               → Board (Kanban board view, default home)
/wizard              → Workflows (alias; opens wizard directly)
/workflows           → Workflows (list + inline wizard + detail)
/pipeline/:workflowId → Pipeline (new-design pipeline visualization)
/debug/:workflowId   → WorkflowDebugPage (PTY terminal debug)
```

- The **main DIY entry** is `/ → /board` (redirect), which renders the `Board` page.
- The **creation entry** is `Workflows` page (accessed from `WorkflowSidebar` "Create Workflow" button at `/wizard`, or the "+ New Workflow" button in `Workflows`). This inline wizard opens `WorkflowWizard`.
- `WorkflowSidebar` has a `navigate('/wizard')` button that goes to the Workflows page via the `/wizard` alias (App.tsx line 127).

---

## 3. Frontend Pages

### 3.1 `frontend/src/pages/Board.tsx` — **KEEP (essential)**

The DIY-mode kanban view. Renders:
- `WorkflowSidebar` — project selector + workflow list
- `WorkflowKanbanBoard` — drag-and-drop task status board
- `TerminalActivityPanel` — collapsible bottom panel showing active terminal output snippets
- `StatusBar` — bottom status bar

Subscribes to WebSocket events via `useWorkflowEvents` and debounces React Query cache invalidation. Handles `workflow.status_changed`, `task.status_changed`, `terminal.status_changed`, `terminal.completed`, `git.commit_detected`, `terminal.prompt_detected/decision`, `quality.gate_result`, `system.lagged`.

### 3.2 `frontend/src/pages/Workflows.tsx` — **KEEP (essential)**

2013-line file; the heavyweight DIY workflow management page. Contains:
- `OrchestratorChatPanel` — Orchestrator chat for running workflows (used by both modes)
- `SelectedWorkflowView` — Detail view: status badges, goal, `PipelineView`, prompt dialog
- `WorkflowListContent` — Grid of workflow cards
- `Workflows` — Main exported component; manages wizard toggle, project selector, CRUD mutations, prompt queue, WS events

The `WorkflowWizard` is inlined here (rendered when `showWizard === true`). The `executionMode` distinguishes DIY vs agent-planned at runtime (label rendered via `getExecutionModeLabel` at line 183–189).

### 3.3 `frontend/src/pages/Pipeline.tsx` — **KEEP (essential)**

Read-only pipeline visualization. Renders `OrchestratorHeader` + `TaskPipeline` for a given `workflowId`. Not DIY-exclusive (works for both modes), but is the primary structural view for DIY task trees.

Route: `/pipeline/:workflowId`

### 3.4 `frontend/src/pages/SlashCommands.tsx` — **KEEP (essential)**

Global slash command preset CRUD (create/edit/delete). These presets are referenced in `Step5Commands`. Rendered at `/commands` under the legacy `NormalLayout`. Powers `userPresets` in Step 5 which fetches from `/api/workflows/presets/commands`.

### 3.5 `frontend/src/pages/WorkflowDebugPage.tsx` — **KEEP (essential)**

Live PTY terminal debug view for any running workflow. Route: `/debug/:workflowId`. Fetches workflow, maps DTO → `Terminal` type, passes to `TerminalDebugView`. Used from `TerminalActivityPanel` links.

---

## 4. Workflow Creation Wizard (7 Steps)

All components under `frontend/src/components/workflow/`.

### 4.1 `WorkflowWizard.tsx` — **KEEP (essential)**

Orchestrator for the 7-step creation flow. Manages `config: WizardConfig` state, step navigation (`useWizardNavigation`), per-step validation (`useWizardValidation`), model library persistence, and final `wizardConfigToCreateRequest` → `onComplete`. Renders the correct step component via `renderStep()`.

DIY-specific behavior:
- Shows Steps 2 (Tasks) and 4 (Terminals) only when `executionMode === 'diy'`
- `getVisibleWizardSteps` / `getVisibleWizardStepIds` filter them out for `agent_planned`
- Auto-initializes tasks/terminals when switching back from `agent_planned` to `diy` (lines 72–105)

### 4.2 `types.ts` — **KEEP (essential)**

The single source of truth for all wizard types:
- `WizardStep` enum (0–6)
- `WorkflowExecutionMode = 'diy' | 'agent_planned'`
- `WizardConfig`, `BasicConfig`, `TaskConfig`, `ModelConfig`, `TerminalConfig`, `CommandConfig`, `AdvancedConfig`
- `getVisibleWizardSteps()` — DIY shows all 7; agent_planned hides Tasks (2) and Terminals (4)
- `wizardConfigToCreateRequest()` — serializes wizard state to API `CreateWorkflowRequest`; for DIY populates `tasks[]` with terminals; for agent_planned sends `tasks: []`

### 4.3 `constants.ts` — **KEEP (essential)**

`CLI_TYPES` registry (9 supported CLIs: claude-code, gemini-cli, codex, amp, cursor-agent, qwen-code, copilot, droid, opencode). Used by Steps 3, 4, 6 and `TerminalCard`. `GIT_COMMIT_FORMAT` displayed in Step 6.

### 4.4 Step Components

| File | Role | DIY-only? | Disposition |
|------|------|-----------|-------------|
| `steps/Step0Project.tsx` | Working directory selection + git status check; auto-fills from project repos | Shared | **KEEP** |
| `steps/Step1Basic.tsx` | Workflow name/description; DIY shows task count + import-from-kanban; agent_planned shows initialGoal field | DIY sections conditional | **KEEP** |
| `steps/Step2Tasks.tsx` | Per-task name/branch/description/terminal-count form (paginated navigation) | **DIY-only** | **KEEP** |
| `steps/Step3Models.tsx` | AI model config: add/edit/verify; native subscription tab; CLI-model binding | Shared | **KEEP** |
| `steps/Step4Terminals.tsx` | Per-task terminal assignment: CLI type selector + model selector + role field; detects installed CLIs via `/api/cli_types/detect` | **DIY-only** | **KEEP** |
| `steps/Step5Commands.tsx` | Slash command preset selection, ordering, custom descriptions, additional commands, JSON params | Shared | **KEEP** |
| `steps/Step6Advanced.tsx` | Orchestrator model, error terminal (optional), merge terminal, git watcher, target branch | Shared | **KEEP** |
| `steps/index.ts` | Re-exports all step components | — | **KEEP** |

### 4.5 Validators

| File | Disposition |
|------|-------------|
| `validators/step0Project.ts` | **KEEP** |
| `validators/step1Basic.ts` | **KEEP** |
| `validators/step2Tasks.ts` | **KEEP** (DIY-only) |
| `validators/step3Models.ts` | **KEEP** |
| `validators/step4Terminals.ts` | **KEEP** (DIY-only) |
| `validators/step5Commands.ts` | **KEEP** |
| `validators/step6Advanced.ts` | **KEEP** |
| `validators/index.ts` | **KEEP** |

### 4.6 Wizard Hooks

| File | Disposition |
|------|-------------|
| `hooks/useWizardNavigation.ts` | **KEEP** — step counter, next/previous, bounds checking |
| `hooks/useWizardValidation.ts` | **KEEP** — per-step validation + error tracking |

### 4.7 Other Workflow Components

| File | Role | Disposition |
|------|------|-------------|
| `workflow/StepIndicator.tsx` | Visual step indicator dots | **KEEP** |
| `workflow/PipelineView.tsx` | Static pipeline diagram (tasks × terminals + merge terminal); used in `SelectedWorkflowView` | **KEEP** (shared) |
| `workflow/TerminalCard.tsx` | Terminal status card widget; used in PipelineView and TerminalDebugView | **KEEP** (shared) |
| `workflow/QualityBadge.tsx` | Quality gate badge; used in PipelineView/TerminalDebugView | **KEEP** (shared) |
| `workflow/WorkflowPromptDialog.tsx` | Interactive prompt dialog for terminal `ask_user` prompts | **KEEP** (shared) |

---

## 5. Board Components (`components/board/`)

All are used by `Board.tsx`.

| File | Role | Disposition |
|------|------|-------------|
| `WorkflowSidebar.tsx` | Left sidebar: project select + workflow list + "Create" button (navigates to `/wizard`) | **KEEP** |
| `WorkflowKanbanBoard.tsx` | Drag-and-drop kanban board (dnd-kit); columns: pending/running/review_pending/completed/failed/cancelled; calls `useUpdateTaskStatus` | **KEEP** (DIY tasks) |
| `WorkflowCard.tsx` | Card widget for sidebar workflow list | **KEEP** |
| `TaskCard.tsx` | Draggable task card for kanban | **KEEP** |
| `TerminalActivityPanel.tsx` | Collapsible bottom panel showing active terminal output; links to `/debug/:workflowId`; reads from `terminalStore` | **KEEP** |
| `StatusBar.tsx` | Bottom status bar | **KEEP** |

---

## 6. Pipeline Components (`components/pipeline/`)

Used by `Pipeline.tsx` (`/pipeline/:workflowId`).

| File | Role | Disposition |
|------|------|-------------|
| `OrchestratorHeader.tsx` | Top bar: workflow name + status + orchestrator model | **KEEP** (shared) |
| `TaskPipeline.tsx` | Pipeline graph: task columns → terminals → merge terminal; slash commands bar | **KEEP** |
| `TerminalNode.tsx` | Single terminal node in pipeline | **KEEP** |
| `MergeTerminalNode.tsx` | Merge terminal node with click-to-trigger | **KEEP** |
| `TerminalDetailPanel.tsx` | Side panel showing terminal detail on click | **KEEP** |
| `statusColor.ts` | Status → color mapping utility | **KEEP** |

---

## 7. Terminal Components (`components/terminal/`)

Used by `WorkflowDebugPage`.

| File | Role | Disposition |
|------|------|-------------|
| `TerminalDebugView.tsx` | Multi-tab PTY terminal viewer; task/terminal tabs; live xterm.js + history load; quality badge + report panel | **KEEP** |
| `TerminalEmulator.tsx` | xterm.js WebSocket PTY emulator; connects to `/api/terminal/:terminalId` WS | **KEEP** |

---

## 8. Stores

| File | Role | Disposition |
|------|------|-------------|
| `stores/workflowStore.ts` | Zustand store: workflow map, active workflow, task/terminal status update actions | **KEEP** — but note: the main `Workflows.tsx` and `Board.tsx` use **React Query** (`useWorkflow`, `useWorkflows`), NOT this Zustand store directly. The store appears lightly used (no import found in the main pages); it may be vestigial/legacy alongside React Query |
| `stores/terminalStore.ts` | Zustand store: terminal output buffers (appendOutput, 10k line cap), connection state, recent output hook | **KEEP** — `TerminalActivityPanel` reads from this via `useRecentTerminalOutput`; `TerminalEmulator` writes to it |
| `stores/wizardStore.ts` | Zustand store for wizard state: full WizardConfig CRUD + navigation + validation | **REFACTOR** — `WorkflowWizard.tsx` uses its own local `useState`, not this store; `wizardStore` duplicates the wizard logic and is not imported by any wizard component. Candidate for deletion |
| `stores/wsStore.ts` | Zustand store: WebSocket connection manager, workflow-scoped WS connections, event subscriptions, prompt response dispatch | **KEEP** — critical for real-time events in both Board and Workflows |

---

## 9. API Surface (Frontend)

### `hooks/useWorkflows.ts`

Key types and hooks:
- `CreateWorkflowRequest` — `executionMode?: 'diy' | 'agent_planned'`; for DIY: `tasks[]` populated
- `useWorkflows(projectId)` — list all workflows for a project
- `useWorkflow(id)` — fetch single workflow detail
- `useCreateWorkflow()` — POST /api/workflows
- `usePrepareWorkflow()` — starts terminals (created → starting → ready)
- `useStartWorkflow()` — begins task execution (ready → running)
- `usePauseWorkflow()`, `useStopWorkflow()`, `useMergeWorkflow()`, `useDeleteWorkflow()`
- `useUpdateTaskStatus()` — called by kanban drag-and-drop
- `useOrchestratorMessages()`, `useSubmitOrchestratorChat()` — orchestrator chat
- `useSubmitWorkflowPromptResponse()` — terminal interactive prompts

### `hooks/useSlashCommands.ts`

CRUD for slash command presets; used by `SlashCommands.tsx`.

### Backend `CreateWorkflowRequest` (Rust)

`crates/server/src/routes/workflows.rs` line 649–658 validates `execution_mode` must be in `["diy", "agent_planned"]`. Line 934: `let is_diy = req.execution_mode == "diy"`. For DIY, tasks and terminals are created from the request body; for `agent_planned`, the orchestrator creates them dynamically.

---

## 10. Overlap / Shared Components with Orchestrated Workspace

The following components are used by **both** DIY and Orchestrated (`agent_planned`) workspaces:

| Component | Shared role |
|-----------|-------------|
| `WorkflowWizard` + Steps 0, 1, 3, 5, 6 | Creation wizard steps applicable to both modes |
| `PipelineView` | Renders for both; shows empty state + initialGoal for agent_planned |
| `WorkflowKanbanBoard` | Works for both; agent_planned shows empty initially while orchestrator plans |
| `OrchestratorChatPanel` (in Workflows.tsx) | Both modes support it; badge "Primary Channel" shown for agent_planned |
| `WorkflowPromptDialog` | Both modes may trigger terminal prompts |
| `TerminalCard`, `QualityBadge`, `TerminalDebugView` | Mode-agnostic |
| `wsStore`, `terminalStore` | Mode-agnostic infrastructure |
| `useWorkflows`, `useWorkflow`, etc. | Mode-agnostic |
| Step6Advanced | Both modes configure orchestrator/merge/error terminal |
| `WorkflowSidebar`, `WorkflowCard`, `StatusBar` | Lists/shows all workflows regardless of mode |

**DIY-exclusive** components (only rendered/used when `executionMode === 'diy'`):
- `Step2Tasks.tsx` and `validators/step2Tasks.ts`
- `Step4Terminals.tsx` and `validators/step4Terminals.ts`
- `WorkflowKanbanBoard` task drag interaction (agent_planned workflows rarely have user-managed tasks)

---

## 11. Disposition Summary Table

| Path | Role | Disposition |
|------|------|-------------|
| `frontend/src/App.tsx` | Router; registers `/board`, `/wizard`, `/workflows`, `/pipeline/:id`, `/debug/:id` | keep |
| `frontend/src/pages/Board.tsx` | Kanban board page — DIY primary runtime view | keep |
| `frontend/src/pages/Workflows.tsx` | Workflow management + inline wizard | keep |
| `frontend/src/pages/Pipeline.tsx` | Pipeline visualization page | keep |
| `frontend/src/pages/WorkflowDebugPage.tsx` | PTY debug page | keep |
| `frontend/src/pages/SlashCommands.tsx` | Slash command preset CRUD | keep |
| `frontend/src/components/workflow/WorkflowWizard.tsx` | 7-step wizard controller | keep |
| `frontend/src/components/workflow/types.ts` | All wizard types + `wizardConfigToCreateRequest` | keep |
| `frontend/src/components/workflow/constants.ts` | CLI_TYPES registry | keep |
| `frontend/src/components/workflow/StepIndicator.tsx` | Wizard step indicator | keep |
| `frontend/src/components/workflow/PipelineView.tsx` | Static pipeline diagram | keep |
| `frontend/src/components/workflow/TerminalCard.tsx` | Terminal card widget | keep |
| `frontend/src/components/workflow/QualityBadge.tsx` | Quality gate badge | keep |
| `frontend/src/components/workflow/WorkflowPromptDialog.tsx` | Interactive prompt dialog | keep |
| `frontend/src/components/workflow/steps/Step0Project.tsx` | Working directory + git status | keep |
| `frontend/src/components/workflow/steps/Step1Basic.tsx` | Name/desc/mode/task count | keep |
| `frontend/src/components/workflow/steps/Step2Tasks.tsx` | Task name/branch/description/terminal-count | keep |
| `frontend/src/components/workflow/steps/Step3Models.tsx` | AI model config/verify | keep |
| `frontend/src/components/workflow/steps/Step4Terminals.tsx` | Per-terminal CLI+model+role config | keep |
| `frontend/src/components/workflow/steps/Step5Commands.tsx` | Slash command selection + ordering | keep |
| `frontend/src/components/workflow/steps/Step6Advanced.tsx` | Orchestrator/error/merge/git config | keep |
| `frontend/src/components/workflow/steps/index.ts` | Step re-exports | keep |
| `frontend/src/components/workflow/validators/*.ts` | Per-step validation | keep |
| `frontend/src/components/workflow/hooks/useWizardNavigation.ts` | Step nav hook | keep |
| `frontend/src/components/workflow/hooks/useWizardValidation.ts` | Per-step validation hook | keep |
| `frontend/src/components/board/WorkflowSidebar.tsx` | Sidebar with workflow list | keep |
| `frontend/src/components/board/WorkflowKanbanBoard.tsx` | Drag-and-drop kanban | keep |
| `frontend/src/components/board/WorkflowCard.tsx` | Sidebar workflow card | keep |
| `frontend/src/components/board/TaskCard.tsx` | Kanban task card | keep |
| `frontend/src/components/board/TerminalActivityPanel.tsx` | Terminal output preview panel | keep |
| `frontend/src/components/board/StatusBar.tsx` | Bottom status bar | keep |
| `frontend/src/components/pipeline/OrchestratorHeader.tsx` | Pipeline page header | keep |
| `frontend/src/components/pipeline/TaskPipeline.tsx` | Pipeline graph | keep |
| `frontend/src/components/pipeline/TerminalNode.tsx` | Terminal node | keep |
| `frontend/src/components/pipeline/MergeTerminalNode.tsx` | Merge terminal node | keep |
| `frontend/src/components/pipeline/TerminalDetailPanel.tsx` | Terminal detail side panel | keep |
| `frontend/src/components/pipeline/statusColor.ts` | Status→color util | keep |
| `frontend/src/components/terminal/TerminalDebugView.tsx` | PTY multi-tab debug view | keep |
| `frontend/src/components/terminal/TerminalEmulator.tsx` | xterm.js PTY emulator | keep |
| `frontend/src/stores/workflowStore.ts` | Workflow Zustand store | refactor (likely vestigial — not imported by main pages) |
| `frontend/src/stores/terminalStore.ts` | Terminal output Zustand store | keep |
| `frontend/src/stores/wizardStore.ts` | Wizard Zustand store | refactor (duplicates local state in WorkflowWizard; not imported by wizard) |
| `frontend/src/stores/wsStore.ts` | WebSocket Zustand store | keep |
| `frontend/src/hooks/useWorkflows.ts` | Workflow CRUD + event hooks | keep |
| `frontend/src/hooks/useSlashCommands.ts` | Slash command CRUD hooks | keep |
| `crates/server/src/routes/workflows.rs` | Backend workflow CRUD routes | keep |
| `crates/server/src/routes/workflows_dto.rs` | Backend DTO types | keep |

---

## 12. Key API Calls Made by DIY Components

| Component | API call | Method |
|-----------|----------|--------|
| Step0Project | `POST /api/git/status` | direct fetch |
| Step0Project | `POST /api/git/init` | direct fetch |
| Step4Terminals | `GET /api/cli_types/detect` | direct fetch |
| Step5Commands | `GET /api/workflows/presets/commands` | direct fetch |
| Workflows (main) | `POST /api/projects/resolve-by-path` | direct fetch (resolveProjectIdFromPath) |
| useWorkflows | `GET /api/workflows?projectId=...` | React Query |
| useWorkflow | `GET /api/workflows/:id` | React Query |
| useCreateWorkflow | `POST /api/workflows` | React Query mutation |
| useUpdateTaskStatus | `PUT /api/workflows/:wfId/tasks/:taskId/status` | React Query mutation |
| useSubmitWorkflowPromptResponse | `POST /api/workflows/:id/prompt-response` | React Query mutation |
| TerminalEmulator | `WS /api/terminal/:terminalId` | WebSocket (PTY) |
| wsStore | `WS /api/ws/workflows/:id` | WebSocket (events) |

---

## 13. Risks

1. **`wizardStore.ts` and `workflowStore.ts` are not imported by the active wizard/board code.** Deleting them without auditing all importers (tests, legacy pages) could break hidden call sites, but keeping them creates confusion about canonical state management.

2. **Step3Models uses `useModelStore.getState()` imperatively** (fetch/verify model). `modelStore.ts` is a dependency that must not be deleted.

3. **`Workflows.tsx` is 2013 lines** — if IDE/tooling reformats it, the diff will be enormous and will break git blame. Treat as a high-churn file.

4. **Board.tsx and Workflows.tsx both independently subscribe to WS workflow events** (via `useWorkflowEvents`) and both call `workflowKeys.forProject` invalidation — overlapping cache invalidation is intentional but causes double-invalidation when both pages are mounted simultaneously.

5. **`Step4Terminals` fetches installed CLIs on mount** via direct fetch (no React Query); if the backend `/api/cli_types/detect` endpoint is slow or returns errors, the terminal step blocks silently.

6. **Legacy route `/commands` under `NormalLayout`** — `SlashCommands` is wrapped in the old design system. If `NormalLayout` or `LegacyDesignScope` is removed, SlashCommands loses its rendering context.
