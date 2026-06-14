# Census: fe-wizard-dup

**Unit:** `fe-wizard-dup`
**Scope:** `frontend/src/components/wizard/` (10 files)
**Question:** Is this a DUPLICATE of `components/workflow/`?

## Verdict

**YES — dead, superseded duplicate.** The entire `components/wizard/` directory is an early stub version of the workflow wizard. The production app (`frontend/src/pages/Workflows.tsx:57`) imports the real wizard from `@/components/workflow/WorkflowWizard`, NOT from `components/wizard/`. The only references to anything under `components/wizard/` are inside that directory's OWN test files (`WorkflowWizard.test.tsx` mocks its 4 sibling modules). There is no barrel `index.ts` and zero imports from outside the directory.

The two `WorkflowWizard` components are not even API-compatible:
- `wizard/WorkflowWizard`: props `{ projectId: string; onClose }`, hardcoded 3 steps `['Configure','Review','Execute']`, placeholder content.
- `workflow/WorkflowWizard`: props `{ projectId?; onComplete; onCancel; onError }`, 7 dynamic steps (Step0..Step6), validation hooks, i18n, native credentials, config-to-request mapping.

So `components/workflow/` is the live implementation; `components/wizard/` is leftover scaffolding.

## Module map

| File | Purpose | Public surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `WorkflowWizard.tsx` | Stub 3-step modal shell (Configure/Review/Execute) with Back/Next + close X | `WorkflowWizard({ projectId, onClose })` | imports local `StepIndicator`, `WorkflowConfigureStep`, `WorkflowReviewStep`, `WorkflowExecuteStep` | DEAD. No production importer. Superseded by `components/workflow/WorkflowWizard.tsx` (7 steps, validation, i18n). Different prop shape. |
| `StepIndicator.tsx` | Renders horizontal step pills (completed ✓ / current / pending) | `StepIndicator({ steps, currentStep })` | used only by local `WorkflowWizard.tsx`; imports `@/lib/utils` cn | DUPLICATE of `components/workflow/StepIndicator.tsx`. Dead via dead parent. |
| `WorkflowConfigureStep.tsx` | Step 1 stub: shows projectId + a free-text "Workflow Name" input (no state wiring) | `WorkflowConfigureStep({ projectId })` | used only by local `WorkflowWizard.tsx` | DEAD stub. Input is uncontrolled, value never read. |
| `WorkflowReviewStep.tsx` | Step 2 stub: hardcoded review summary ("Name: Workflow Configuration", "Tasks: 3 tasks configured") | `WorkflowReviewStep()` | used only by local `WorkflowWizard.tsx` | DEAD stub. Hardcoded placeholder text, no props. |
| `WorkflowExecuteStep.tsx` | Step 3 stub: static "Workflow execution started" message + 🚀 emoji | `WorkflowExecuteStep()` | used only by local `WorkflowWizard.tsx` | DEAD stub. No execution logic, purely cosmetic. |
| `WorkflowWizard.test.tsx` | Vitest for the stub wizard (renders, button states, onClose) | test only | imports `./WorkflowWizard`; mocks 4 siblings | Test-only. The sole "consumer" of the dead components. |
| `StepIndicator.test.tsx` | Vitest for stub StepIndicator | test only | imports `./StepIndicator` | Test-only. |
| `WorkflowConfigureStep.test.tsx` | Vitest for stub configure step | test only | imports `./WorkflowConfigureStep` | Test-only. |
| `WorkflowReviewStep.test.tsx` | Vitest for stub review step | test only | imports `./WorkflowReviewStep` | Test-only. |
| `WorkflowExecuteStep.test.tsx` | Vitest for stub execute step | test only | imports `./WorkflowExecuteStep` | Test-only. |

## Evidence trail

- `git ls-files frontend/src/components/wizard/` → exactly 10 files (5 components + 5 tests).
- Production wiring: `frontend/src/pages/Workflows.tsx:57` → `import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';` (line 1996 mounts it). No `pages/` or other file imports `components/wizard`.
- `grep components/wizard` across all `.ts/.tsx/.js/.json` (excl. node_modules) → only hits are inside `components/wizard/WorkflowWizard.test.tsx` (self-mocks). The `stores/index.ts:33 './wizardStore'` hit is an unrelated Zustand store, not this dir.
- No barrel: `find ... -name index.ts -path '*wizard*'` → none.

## Disposition

Delete the whole `frontend/src/components/wizard/` directory (all 10 files). Blast radius: only its own tests, which are deleted with it. The live wizard in `components/workflow/` is untouched.

## Tooling note

`mcp__fast-context__fast_context_search` failed twice with `resource_exhausted` (quota/internal) errors (trace IDs f35999a0..., a1b7f6ca...). Fell back to Grep + git ls-files + targeted Read, which is fully conclusive here because the import graph is tiny and the only references are self-contained within the directory.
