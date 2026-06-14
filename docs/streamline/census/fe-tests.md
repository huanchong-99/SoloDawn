# FE-Tests Module Census

Unit: **fe-tests**
Branch: refactor/streamline-quality-gates
Date: 2026-06-14

---

## Test Infrastructure (`frontend/src/test/`)

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `setup.ts` | Global test setup: `@testing-library/jest-dom` + HTMLCanvasElement.getContext mock | None (side-effects only) | Registered in `vitest.config.ts` `setupFiles` | Canvas mock is also tested by `canvas-mock.test.ts` |
| `test-global.d.ts` | Type declaration: references vitest/globals | Type ambient | Used by TS compiler | One-liner; needed to satisfy `vitest/globals` |
| `renderWithI18n.tsx` | Test utility: wraps component with I18nextProvider + ToastProvider + MemoryRouter | `renderWithI18n`, `setTestLanguage`, `i18n` (re-export) | Imported by 17 test files across pages/, components/ | Heavy usage; keep |
| `browserslist-env.test.ts` | Asserts `BROWSERSLIST_IGNORE_OLD_DATA=1` env var is set | None (test only) | Verifies `vitest.config.ts` line 7 | Meta-test of config |
| `canvas-mock.test.ts` | Smoke-tests `setup.ts` canvas mock | None | Depends on `setup.ts` side-effect | Redundant with setup; tests its own infrastructure |
| `legacy-components.test.ts` | Asserts legacy kanban directories/files no longer exist | None | Uses `node:fs` to check `src/components/projects`, `TaskKanbanBoard.tsx`, `TasksLayout.tsx`, `shadcn-io/kanban` dir | Guard against re-adding removed code |
| `legacy-routes.test.ts` | Asserts `App.tsx` has no legacy Projects/ProjectTasks imports | None | Reads `src/App.tsx` at runtime | Guard test; fragile relative path |
| `tailwind-config.test.ts` | Asserts `tailwind.config.cjs` exports non-empty `content` array | None | `require('../../tailwind.config.cjs')` | Meta-test of build config |

---

## Store Tests (`frontend/src/stores/__tests__/`)

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `wsStore.test.ts` | Comprehensive WS store integration tests: connect, disconnect, send, subscribe, heartbeat, reconnect, per-workflow connections, payload normalization (snake/camel), prompt events, sendPromptResponse routing | None (test) | Tests `stores/wsStore` (dynamic import via `vi.resetModules`) | 20+ test cases; covers P0 WS contract, `terminal.prompt_*`, `terminal.completed` dual-format normalization |

---

## Hook Tests (`frontend/src/hooks/__tests__/`)

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `useErrorNotification.test.tsx` | Unit-tests `useErrorNotification` hook: string/Error/unknown wrapping | None | Tests `hooks/useErrorNotification.ts` | Used by Step4Terminals, Step5Commands, Step0Project |
| `useQualityGate.test.ts` | Exception/edge-case tests for quality gate hooks: 404 fallback, 500 surfacing, network errors, cache dedup, disabled queries | None | Tests `hooks/useQualityGate.ts`; uses `@tanstack/react-query` wrapper | Paired with a DUPLICATE: `hooks/useQualityGate.test.tsx` (outside `__tests__/`) |

**Duplicate detected**: `frontend/src/hooks/useQualityGate.test.tsx` is a git-tracked file that is NOT under `__tests__/` but IS picked up by vitest's `include` glob. It covers the happy-path / `qualityKeys` query-key tests with a different approach (stubs `fetch`, mocks `logApiError`). The `__tests__/useQualityGate.test.ts` covers exception/edge-case scenarios. Both test the same 4 hooks; neither is clearly superseded — they are complementary but overlapping.

---

## i18n Tests (`frontend/src/i18n/__tests__/`)

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `config.test.ts` | Asserts `i18n/config` loads without `console.log` output | None | Tests `i18n/config.ts` side-effect | Meta-test of i18n initialization |
| `workflow.test.ts` | Asserts workflow i18n namespace loads and `wizard.title` key resolves | None | Tests `@/i18n` default export | Simple smoke-test |

---

## Lib Tests (`frontend/src/lib/__tests__/`)

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `api-logging.test.ts` | Asserts `logApiError` does not emit `console.error` in test env | None | Tests `lib/api.ts::logApiError` | Meta-test of logging suppression |
| `api-result.test.ts` | Tests `attemptsApi.push` error handling: `error_data` vs `error` field precedence | None | Tests `lib/api.ts::attemptsApi` | Covers force_push_required structured error |

---

## Page Tests (`frontend/src/pages/__tests__/`)

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `WorkflowDebugPage.test.tsx` | Integration test for `WorkflowDebugPage`: loading/not-found/loaded states, task/terminal mapping, wsUrl construction | None | Tests `pages/WorkflowDebugPage.tsx`; mocks `useWorkflow`, `useWorkflowEvents`, `TerminalDebugView`, `react-i18next` | More comprehensive than the outer duplicate |

**Duplicate detected**: `frontend/src/pages/WorkflowDebugPage.test.tsx` (outside `__tests__/`) covers 3 simplified cases (render/loading/error). The `__tests__/` version covers 4 cases with more detailed assertions (tasks-count, terminal-count, status mapping, wsUrl pattern). Both run simultaneously. The outer file is the earlier/simpler version.

---

## Component Tests (`frontend/src/components/*/__tests__/`)

### Quality Components

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `quality/__tests__/QualityIssueList.test.tsx` | Tests empty state, all severity levels, file path display, expand-on-click | None | Tests `components/quality/QualityIssueList.tsx` | Uses `shared/types::QualityIssueRecord` |
| `quality/__tests__/QualityReportPanel.test.tsx` | Tests loading/error/empty/passed/failed states, metrics display, issue list integration | None | Tests `components/quality/QualityReportPanel.tsx`; mocks `useQualityGate` hooks and `react-i18next` | Exercises Quality Gate System A UI path |
| `quality/__tests__/QualityTimeline.test.tsx` | Tests 4-step timeline for all gate statuses: pending/running/error/warn/ok | None | Tests `components/quality/QualityTimeline.tsx`; uses `shared/types::QualityRun` | Checks animate-pulse, amber/green border classes |

### Workflow Components

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `workflow/__tests__/QualityBadge.test.tsx` | Tests badge rendering for all gateStatus values, blockingIssues counts, custom className | None | Tests `components/workflow/QualityBadge.tsx`; mocks `react-i18next` | QualityBadge used in `PipelineView.tsx` |
| `workflow/hooks/__tests__/useWizardNavigation.test.ts` | Tests step navigation: next/previous/goToStep/canGo* bounds | None | Tests `components/workflow/hooks/useWizardNavigation.ts`; uses `WizardStep` enum | Hook used by `WorkflowWizard.tsx` |
| `workflow/hooks/__tests__/useWizardValidation.test.ts` | Tests error state management: validate/setErrors/clearErrors/hasErrors | None | Tests `components/workflow/hooks/useWizardValidation.ts`; uses `getDefaultWizardConfig` | Hook used by `WorkflowWizard.tsx` |
| `workflow/validators/__tests__/step4Terminals.test.ts` | Tests terminal validator: missing cli/model produces errors | None | Tests `validators/step4Terminals.ts`; uses `getDefaultWizardConfig` | Single narrow test |
| `workflow/validators/__tests__/steps.test.ts` | Tests validators for steps 0-3 and 6 with default config | None | Tests `validators/index.ts` (re-exports step0-3,6); uses `getDefaultWizardConfig` | Note: step4Terminals has its own separate test file |

---

## Out-of-scope test files also picked up by vitest

These files are NOT in the canonical scope directories but ARE tracked and run by vitest:

| File | Purpose | Notes |
|------|---------|-------|
| `hooks/useQualityGate.test.tsx` | Happy-path + qualityKeys tests for quality gate hooks | DUPLICATE of `hooks/__tests__/useQualityGate.test.ts` |
| `pages/WorkflowDebugPage.test.tsx` | Simpler 3-case tests for WorkflowDebugPage | DUPLICATE of `pages/__tests__/WorkflowDebugPage.test.tsx` |
| `components/tasks/TaskCard.test.tsx` | Single test for parent navigation loading reset | TaskCard.tsx imports from `shadcn-io/kanban` (which only exists as `kanban.tsx`, not a directory — this is a broken import path) |

---

## Candidates Summary

| Path | Kind | Confidence | Disposition |
|------|------|-----------|------------|
| `src/test/canvas-mock.test.ts` | redundant | medium | investigate |
| `src/hooks/useQualityGate.test.tsx` | duplicate | high | delete |
| `src/pages/WorkflowDebugPage.test.tsx` | duplicate | high | delete |
| `src/components/tasks/TaskCard.test.tsx` | bug (broken import subject) | medium | investigate |
| `src/test/legacy-routes.test.ts` | legacy guard | low | keep |
| `src/test/legacy-components.test.ts` | legacy guard | low | keep |
