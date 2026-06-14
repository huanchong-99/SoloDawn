# Census: fe-pipeline — `frontend/src/components/pipeline/`

Generated: 2026-06-14  
Branch: refactor/streamline-quality-gates  
Unit: fe-pipeline (11 files)

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|---|---|---|---|---|
| `statusColor.ts` | Shared status→Tailwind-class maps for pipeline nodes | `getTerminalNodeClasses(status)`, `getTerminalBadgeClasses(status)` | Imported only by `TerminalNode.tsx` (within this dir). NOT imported by `WorkflowProgressView` or `ProcessListItem` (those define their own local `statusColor` helpers) | Comment E10-07: literal color classes (`green-*`, `red-*`) are explicitly marked "interim" pending design-system token swap |
| `MergeTerminalNode.tsx` | Renders the merge destination node at the end of the pipeline graph | `MergeTerminalNode({ workflowId })` | Consumed by `TaskPipeline.tsx`; calls `useWorkflow` hook; reads `workflow.targetBranch` | E10-09: merge status badge is hardcoded/static (no real status surfacing). `tokensUsed` is NOT passed from parent `Pipeline.tsx` |
| `MergeTerminalNode.test.tsx` | Unit test for MergeTerminalNode | — | Tests only `MergeTerminalNode` | Mock of `useWorkflow`; 1 test case |
| `OrchestratorHeader.tsx` | Top bar showing workflow name, status, model, and token usage | `OrchestratorHeader({ name, status, model, tokensUsed? })`, private `formatTokens(tokens)` | Consumed by `pages/Pipeline.tsx`; pure presentation, no hooks beyond i18n | `tokensUsed` is accepted in props but Pipeline.tsx does NOT pass it (field not on `WorkflowDetailDto`), so it always renders "N/A" — see candidate below |
| `OrchestratorHeader.test.tsx` | Unit tests for OrchestratorHeader | — | Tests `formatTokens` formatting (k/M/raw) and fallback rendering | 5 test cases; thorough |
| `TaskPipeline.tsx` | Main pipeline visualization: slash-commands bar + task columns + merge node | `TaskPipeline({ workflowId })`, private `TaskColumn({ task, isLast })` | Consumed by `pages/Pipeline.tsx`; orchestrates `TerminalNode`, `MergeTerminalNode`; calls `useWorkflow`; reads `workflow.tasks`, `workflow.commands`, `workflow.executionMode` | `agent_planned` mode shows alternate empty-state copy. `pipeline.hint` key has inline `defaultValue` fallback (key exists in en locale, but defensive) |
| `TaskPipeline.test.tsx` | Unit test for TaskPipeline | — | Mocks `useWorkflow`, `TerminalNode`, `MergeTerminalNode` | 1 test case; thin coverage |
| `TerminalDetailPanel.tsx` | Pop-up card showing role/status/model for a selected terminal | `TerminalDetailPanel({ role?, status?, model? })` | Consumed only by `TerminalNode.tsx`; pure presentation; no i18n, no hooks | Labels "Status:"/"Model:" are hardcoded English strings, not i18n keys — E10-08 candidate |
| `TerminalDetailPanel.test.tsx` | Unit test for TerminalDetailPanel | — | Tests `TerminalDetailPanel` only | 1 test case |
| `TerminalNode.tsx` | Clickable node card; manages expanded/collapsed state for TerminalDetailPanel | `TerminalNode({ terminal, taskName? })` | Consumed by `TaskPipeline.tsx` (via `TaskColumn`); uses `getTerminalBadgeClasses`, `getTerminalNodeClasses`, `TerminalDetailPanel`; uses `cn` util | E10-01/02/03: outside-click detection + unmount reset + stopPropagation all in-place |
| `TerminalNode.test.tsx` | Unit test for TerminalNode | — | Tests initial render only (no expand/collapse interaction test) | Missing click-expand test; detail panel not tested via this component |

---

## Candidates

### C1 — `OrchestratorHeader.tsx` `tokensUsed` prop — stub/dead feature
- **Lines**: 7, 36–37
- **Kind**: stub
- **Evidence**: `tokensUsed` is declared optional in the interface and renders "Tokens Used / N/A" when absent. `pages/Pipeline.tsx` (only caller) does NOT pass `tokensUsed`. `WorkflowDetailDto` (generated `shared/types.ts`) has no `tokensUsed` field. The prop is exercised only in unit tests with synthetic values.
- **Why**: The feature was designed (formatter is complete and well-tested) but the backend field was never wired up. UI always shows "N/A".
- **DispositionHint**: investigate (either wire backend field or remove the display section)
- **Confidence**: high
- **BlastRadius**: zero external breakage; only `OrchestratorHeader.test.tsx` would need updating

### C2 — `statusColor.ts` — design-system token debt (interim literal classes)
- **Lines**: 5–8 (comment), 14, 15, 18, 20, 22, 30, 31, 34, 38
- **Kind**: legacy
- **Evidence**: Author explicitly comments `E10-07 (remaining)` — literal Tailwind color classes (`green-500`, `red-500`, `blue-500`, `yellow-500`) are interim; design-system token variants (`success`, `error`, `warning`) should replace them once the new-design palette exposes matching Tailwind variants.
- **Why**: `WorkflowProgressView` and `ProcessListItem` each have their own local `statusColor` helpers with partially overlapping logic — three separate implementations of the same concept.
- **DispositionHint**: refactor (consolidate + swap to tokens when design-system palette is ready)
- **Confidence**: medium
- **BlastRadius**: `TerminalNode.tsx` color classes change; visually breaking if token mapping differs

### C3 — `TerminalDetailPanel.tsx` hardcoded English label strings
- **Lines**: 12–13 (Status, Model labels)
- **Kind**: legacy
- **Evidence**: Labels "Status:" and "Model:" are bare hardcoded English strings with no `useTranslation` call. All sibling components in this dir use `useTranslation('workflow')`.
- **Why**: E10-08 comment in `MergeTerminalNode.tsx` references an i18n fix — this panel appears to be the remaining unfixed component.
- **DispositionHint**: refactor
- **Confidence**: high
- **BlastRadius**: low; only affects displayed text in the pop-up tooltip panel

### C4 — `MergeTerminalNode.tsx` static status badge
- **Lines**: 15–16 (NOTE comment), 18–24 (entire render)
- **Kind**: stub
- **Evidence**: NOTE(E10-09) explicitly states "merge status surfacing is a product/UX decision and tracked separately". The node renders `workflow.targetBranch` only — no status color, no live status badge. `getTerminalNodeClasses`/`getTerminalBadgeClasses` are not used.
- **Why**: The merge node is visually inert: no status, no interaction, no expand behavior — unlike `TerminalNode`.
- **DispositionHint**: investigate (decide whether to add status parity or leave as display-only)
- **Confidence**: medium
- **BlastRadius**: no breakage; feature gap only

### C5 — `TerminalNode.test.tsx` missing interaction coverage
- **Lines**: 29–33
- **Kind**: stub
- **Evidence**: Test file covers initial render only. The click-to-expand flow (toggle `expanded`, show `TerminalDetailPanel`, outside-click collapse) has zero test coverage despite being the core behavior of the component.
- **Why**: If the expand/collapse behavior regresses, no test will catch it.
- **DispositionHint**: investigate (add tests or accept coverage gap)
- **Confidence**: high
- **BlastRadius**: no production breakage; risk is test-coverage regression debt

---

## Invisible Features

- **`agent_planned` empty-state copy** (`TaskPipeline.tsx:99`): When `workflow.executionMode === 'agent_planned'`, a distinct empty-state message is shown (`pipeline.emptyDescriptionAgentPlanned`). This branch is not surfaced in any visible UI tab or label — only visible when a workflow has zero tasks and is in agent-planned mode.
- **Outside-click collapse via `pointerdown`** (`TerminalNode.tsx:19–31`): The detail panel closes on any pointer event outside the container. Uses `document.addEventListener('pointerdown', ...)` — a global event listener that is not obvious from the component tree. The listener is correctly cleaned up on collapse.
- **Token-count formatter** (`OrchestratorHeader.tsx:14–23`, `formatTokens`): Complete, well-tested formatting logic (k/M suffixes) for an LLM token-usage display — but the backing data is never provided by the parent. The UI always shows "N/A".
- **Slash-commands bar** (`TaskPipeline.tsx:75–90`): A commands display bar is conditionally rendered when `workflow.commands.length > 0`. This renders `preset.command` values as code badges. The feature is backend-driven and invisible when no slash-commands are configured.

---

## In-flight Work Relevance

- **G1 (open in external editor)**: No references in this directory.
- **VS Code webview bridge**: No references in this directory.
- **Quality Gate System (quality-gate.yaml / QualityGateConfig)**: No references in this directory.
- **Planning-draft confirm→materialize / AuditPlan System B**: No references in this directory.

---

## toolNotes

fast-context MCP returned `resource_exhausted` errors on both cross-file queries. All cross-file import/usage analysis was performed with Grep fallback. Results are exhaustive (all `.ts`/`.tsx` files under `frontend/src/` searched).
