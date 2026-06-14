# Census: fe-quality-component

Unit scope: `frontend/src/components/quality/` (+ `__tests__`)
Branch: refactor/streamline-and-quality-gates
G3 relevance: existing quality UI; basis for the new rules editor.

Tooling note: fast-context MCP (`mcp__fast-context__fast_context_search`) returned
`resource_exhausted` (quota) on two attempts (trace IDs 3032002d..., 48264004...).
Fell back to Grep for all cross-file usage questions, per task rules.

## Module map

| File | Purpose | Public surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `QualityIssueList.tsx` | Renders a scrollable list of quality issues with severity icons + expand/collapse per item; empty "no issues" state. | `export interface QualityIssueListProps`; `export function QualityIssueList(props)`. Internal: `SeverityIcon`, `IssueItem`. | Consumes `QualityIssueRecord` from `shared/types`; `cn` (`@/lib/utils`); `Button` (`@/components/ui/button`); lucide-react icons. **Production importer: only `QualityReportPanel.tsx` (line 127).** | The `IssueItem` expand toggle (`useState expanded`) only swaps the chevron icon — the comment at L66 ("Removed context view because it is not provided by the DTO") confirms the expand body was removed. Expand is now a no-op beyond the chevron. Minor dubious-feature. |
| `QualityReportPanel.tsx` | Terminal-level quality gate report panel: fetches latest run + issues, shows loading/error/empty/not-available states, a 4-cell metrics grid, and the issue list. | `export interface QualityReportPanelProps`; `export function QualityReportPanel(props)`. | Imports hooks `useTerminalLatestQuality`, `useQualityIssues` (`@/hooks/useQualityGate`, both confirmed exported); `isQualityGateAvailable` (`@/lib/apiVersionCompat`, confirmed); `QualityBadge` + `GateStatus` (`@/components/workflow/QualityBadge`, confirmed); `QualityIssueList` (sibling); `useTranslation('quality')`. **Production importer: `components/terminal/TerminalDebugView.tsx` (L4, rendered in a Dialog at L672).** | LIVE / wired. i18n `quality` namespace exists for en/es/ja/ko/zh-Hans/zh-Hant. Minor i18n gap: keys `panel.notAvailable` / `panel.notAvailableHint` (used L35-37) are absent from quality.json; they rely on inline default-value fallback so no runtime break. Severity metrics are derived client-side from issues (blocker/critical/major/minor/info). |
| `QualityTimeline.tsx` | Horizontal 4-step progress timeline (Checkpoint -> Analysis -> Feedback -> Passed) derived from latest run `gateStatus`. | `export interface QualityTimelineProps`; `export function QualityTimeline(props)`. Internal helpers: `getStepCircleClass`, `getStepLabelClass`, const `STEPS`. | Consumes `QualityRun` from `shared/types`; `cn`; lucide-react. **NO production importer anywhere in `frontend/` outside its own dir** (verified by Grep across whole `frontend` tree: only `QualityTimeline.test.tsx` references it). No barrel/index re-export, no lazy import. | DEAD CODE candidate (high confidence): orphaned component, only kept alive by its own test. |
| `__tests__/QualityIssueList.test.tsx` | Unit tests for QualityIssueList (empty state, each severity, file/line/source render, expand click). | vitest `describe/it`. | Imports `QualityIssueList`, `QualityIssueRecord`. | Healthy tests for a live component. |
| `__tests__/QualityReportPanel.test.tsx` | Unit tests for QualityReportPanel (loading/error/empty/passed/failed/issue-list states); mocks `useQualityGate` hooks + `react-i18next`. | vitest. | Imports `QualityReportPanel`; mocks `@/hooks/useQualityGate`. | Healthy tests for a live component. |
| `__tests__/QualityTimeline.test.tsx` | Unit tests for QualityTimeline (4 labels, checkpoint/analysis/feedback/passed states, multi-run). | vitest. | Imports `QualityTimeline`. | Only consumer of QualityTimeline; would be removed together if the component is cut. |

## Candidates

1. **QualityTimeline.tsx (+ its test)** — `dead` / high confidence.
   Evidence: Grep across the entire `frontend/` tree finds zero importers outside
   `components/quality/`; the only reference is its own `__tests__/QualityTimeline.test.tsx`.
   No barrel index, no lazy/dynamic import, no re-export. dispositionHint: delete
   (or keep ONLY if it is intended scaffolding for the upcoming rules editor / G3 UI —
   then investigate). Blast radius: deleting it + its test breaks nothing in production;
   the 4-step "Checkpoint/Analysis/Feedback/Passed" visual is not surfaced anywhere.

2. **QualityIssueList.tsx expand/collapse (L28-69)** — `dubious-feature` / medium confidence.
   Evidence: `useState(expanded)` + chevron toggle, but the expandable body was removed
   (comment L66). Clicking only flips the chevron with no extra content. dispositionHint:
   refactor (either restore a detail/context view or drop the expand affordance + button).
   Blast radius: cosmetic; the IssueList test "expands issue on click" only asserts the
   element still exists, so trimming the toggle would need a trivial test tweak.

## Invisible features
None of the watch-list items (external-IDE open, VS Code webview bridge, Quality Gate
System A YAML config, planning-draft materialize / AuditPlan System B) appear inside this
unit. This unit is pure presentational React for the *report* side of the quality gate;
all data comes via the `useQualityGate` hooks (out of scope). The orphaned
`QualityTimeline` is the closest thing to an "invisible feature" — a built, tested, but
unmounted component.
