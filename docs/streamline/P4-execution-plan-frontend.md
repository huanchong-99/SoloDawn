# P4 Execution Plan — Area: frontend

Date: 2026-06-14
Branch: `refactor/streamline-quality-gates`
Synthesizer: P4 Plan Synthesizer (Opus)
Inputs:
- `docs/streamline/ledger-frontend.md` (92 candidates, FE-01..FE-92)
- `docs/streamline/P2-candidate-ledger.md` (cross-area clusters + Phase-2 risk notes)
- `docs/audit/R1-ide-editor-connection-deletion-audit.md` (authoritative CL-IDE removal map)

> **Note on verdicts input.** The P3 verdict array handed to this synthesizer was empty (`[]`).
> No per-candidate adversarial keep/cut verdicts were available, so this plan is synthesized
> directly from the source ledger evidence, conservatively. Every candidate with confidence
> below `high` OR any cross-file / schema / generated-types blast radius is routed to
> DEFERRED (reported, not executed) rather than CONFIRMED DELETE. This keeps the executed
> set to the verified-dead, zero-blast-radius subset. File existence and the highest-risk
> line ranges were re-verified with Read/Grep before listing (see PRECONDITIONS).

---

## A. CONFIRMED DELETES

Ordered by cluster, then by execution safety (whole-file orphans first, then partial-file /
export removals, then test-pair co-deletes). Each entry is `high` confidence with grep-verified
zero live importers. Line ranges are for partial-file removals only; "whole file" = delete the
file entirely.

### Cluster CL-ORPHAN-WHOLEFILE — fully dead standalone files (delete first, no ordering deps)

| # | ID | File(s) | Range | Action |
|---|---|---|---|---|
| D1 | FE-04 | `frontend/src/hooks/useVideoProgress.ts` | whole file | delete. 0 callers, not in `hooks/index.ts`, no test. |
| D2 | FE-20 | `frontend/src/components/ui/tabs.tsx` | whole file | delete. 0 `from @/components/ui/tabs` importers; consumers use Radix directly. |
| D3 | FE-21 | `frontend/src/components/ui-new/primitives/Card.tsx` | whole file | delete. 0 importers; no re-export barrel. (See DEFERRED note re: future-primitive intent — low risk, executed.) |
| D4 | FE-46 | `frontend/src/utils/statusLabels.ts` | whole file | delete. Only own def; TaskStatus labels come from i18n/inline. |
| D5 | FE-48 | `frontend/src/types/modal-args.d.ts` | whole file | delete. Stale ambient re-declaration conflicting with authoritative `types/modals.ts`; no runtime effect. |
| D6 | FE-91 | `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx` **and** `frontend/src/components/ui-new/primitives/PlanningChat.tsx` | whole files | delete BOTH together. 0 prod importers; superseded by `CreateChatBoxContainer`. No unique exported type/hook. |
| D7 | FE-78 | `frontend/src/pages/ui-new/WorkspacesLanding.tsx` | whole file | delete. 5-line `<Navigate to="/workspaces/create" replace/>`. **Delete-after:** must inline its redirect into the `App.tsx` route first (one-line route change) — see ORDERING O1. |

### Cluster CL-PARTIAL — dead exports / fields inside otherwise-live files (no caller updates needed)

| # | ID | File | Range | Action |
|---|---|---|---|---|
| D8 | FE-19 | `frontend/src/lib/types.ts` | 12-22 | delete `interface ConversationEntryDisplayType`. 0 imports outside the def. `AttemptData` (same file) stays. |
| D9 | FE-36 | `frontend/src/components/workflow/constants.ts` | 79-88 | delete `export const GIT_COMMIT_TYPES`. Exported, 0 imports anywhere. |
| D10 | FE-41 | `frontend/src/components/ui-new/utils/workflowStatus.ts` | 239-251 | delete `getWorkflowStatusMeta` + `getTerminalStatusMeta` (2 exports). 0 callers; G13/G14 comments are docs, not usages. |
| D11 | FE-42 | `frontend/src/components/ui-new/actions/useActionVisibility.ts` | 137-164 | delete `filterVisibleActionItems`. 0 callers; bars filter inline via `isActionVisible`. |
| D12 | FE-43 | `frontend/src/components/ui-new/hooks/useWorkspaces.ts` | 78-80 | delete `export const workspaceKeys`. 0 external imports. **Keep** `workspaceSummaryKeys` (same file, 10+ consumers). |
| D13 | FE-69 | `frontend/src/components/ui-new/views/GitPanel.tsx` | 20 | delete `remoteCommitsAhead?: number` from `RepoInfo` interface. Never destructured/forwarded; container (`GitPanelContainer`) keeps its own copy. **Delete-after:** verify no consumer reads `repo.remoteCommitsAhead` in `GitPanel.tsx` render (grep before removing). |

### Cluster CL-WIZARDDUP — dead wizard duplicate dir + dead stores (atomic group)

| # | ID | File(s) | Range | Action |
|---|---|---|---|---|
| D14 | FE-02 | `frontend/src/components/wizard/` (whole dir, 10 files: StepIndicator, WorkflowConfigureStep, WorkflowExecuteStep, WorkflowReviewStep, WorkflowWizard — each `.tsx` + `.test.tsx`) | whole dir | delete dir. `pages/Workflows.tsx` imports `WorkflowWizard` from `components/workflow` (live), NOT `components/wizard`. 0 prod imports; only self-referencing tests. |
| D15 | FE-17 | `frontend/src/stores/wizardStore.ts` **and** `frontend/src/stores/workflowStore.ts` | whole files | delete BOTH. 0 imports outside the `stores/index.ts` barrel. **Delete-after:** D16 must remove the re-export lines from `stores/index.ts` in the SAME change, or the barrel breaks the build. |
| D16 | FE-18 | `frontend/src/stores/index.ts` | barrel | refactor-delete: remove the `wizardStore`/`workflowStore` re-export lines (paired with D15). Whole-file delete of the barrel is DEFERRED (verify 0 alias resolution first). |

### Cluster CL-DEBUGSTUB — superseded debug components + wrong-route test

| # | ID | File(s) | Range | Action |
|---|---|---|---|---|
| D17 | FE-06 | `frontend/src/components/debug/TerminalDebugView.tsx` + `.test.tsx`, `frontend/src/components/debug/TerminalSidebar.tsx` + `.test.tsx` | whole files | delete component+test pairs. Live `/debug` route uses `terminal/TerminalDebugView` (different signature, git f362be867). Only own tests reference these. |
| D18 | FE-79 | `frontend/src/pages/__tests__/WorkflowDebugPage.test.tsx` | whole file | delete. Duplicate of `pages/WorkflowDebugPage.test.tsx`; uses WRONG route `/workflow/:id/debug` vs actual `/debug/:id` → never matches router (false confidence). **Delete-after:** confirm the canonical `pages/WorkflowDebugPage.test.tsx` already covers any unique case from the `__tests__/` copy (P2 CL-DUP-LOC: "merge unique cases then delete outer") — see ORDERING O2. |

### Cluster CL-VSCODE (orphan only) — dead webview context menu

| # | ID | File | Range | Action |
|---|---|---|---|---|
| D19 | FE-07 | `frontend/src/vscode/ContextMenu.tsx` | whole file | delete. `WebviewContextMenu`: only self-ref + R1 doc; never mounted. Imports INTO `bridge.ts` (one-directional) → removing it drops 2 import edges into `bridge.ts`. **KEEP `vscode/bridge.ts`** (clipboard helpers LIVE via `wysiwyg.tsx`). |

### Cluster CL-CONVDUP (dead export only) — dead full DiffViewCard

| # | ID | File | Range | Action |
|---|---|---|---|---|
| D20 | FE-13 | `frontend/src/components/ui-new/primitives/conversation/DiffViewCard.tsx` | 164-244 (the full-card `DiffViewCard` export) + its line in `.../conversation/index.ts` barrel | delete the full-card export + barrel line. 0 render sites; superseded by `DiffViewCardWithComments`. **KEEP** `DiffViewBody`, `useDiffData`, `DiffInput` sub-exports (LIVE). NOTE: the duplicate-`useDiffData` consolidation (FE-50) is DEFERRED — this delete only removes the unrendered card, not the hook. |

**CONFIRMED DELETE total: 20 entries (D1–D20).**

---

## B. REFACTORS IN SCOPE

High-confidence bug fixes and dead-prop/dead-path removals with contained, single-file (or
one-caller) blast radius. These EDIT live code; they do not delete files.

| # | ID | File | fixSketch | risk |
|---|---|---|---|---|
| R1 | FE-01 | `frontend/src/lib/api.ts:607` (`uploadAuditDoc`) | Change multipart field name `formData.append('audit_doc', file)` → `'file'` to match backend `planning_drafts.rs` (accepts only `file`, else 400). | **Low.** One-line. Sole caller `usePlanningDraft.ts:111`; backend unchanged. Fixes 100%-broken AuditPlan System B upload. |
| R2 | FE-54 | `frontend/src/pages/ui-new/FirstRunWizard.tsx:48` | Change `fetch('/api/cli-types/detect')` → `'/api/cli_types/detect'` (underscore) to match the backend route + all other callers. | **Low.** FirstRunWizard env step only. Fixes silently-empty CLI list. |
| R3 | FE-57 | `frontend/src/components/ui-new/primitives/RepoCardSimple.tsx:28` | Replace undefined Tailwind token `bg-tertiary` with a defined surface token (`bg-panel` or `bg-secondary` per CLAUDE.md token list). | **Low.** Visual-only; card currently renders transparent. Confirm chosen token against `tailwind.config` + new/index.css before commit. |
| R4 | FE-56 | `frontend/src/components/board/TerminalActivityPanel.tsx:22,69` | Remove `'running'` from `ACTIVE_STATUSES` and the `StatusIndicator` green branch — backend `TerminalStatus` enum has no `'running'` (legacy → `'working'`). | **Low.** Permanently-dead branch; zero runtime change. |
| R5 | FE-52 | `frontend/src/components/dialogs/settings/DeleteConfigurationDialog.tsx:34-42` | Remove the `try/catch` wrapping the non-throwing `modal.resolve()+hide()` (dead catch path) and the `isDeleting` state that is never reset on the happy path. | **Low.** Single dialog; no caller change. |
| R6 | FE-44 | `frontend/src/lib/utils.ts:7-13` (`formatBytes`) | Drop the `export` (un-export) since the only caller is `formatFileSize` in the same file. No external `import formatBytes` exists. | **Low.** No external breakage. |
| R7 | FE-49 | `frontend/src/components/ui-new/primitives/Toolbar.tsx:83-105` (`ToolbarDropdown` fallback) | Remove the dead fallback sort/group-menu JSX (all 3 call sites always pass explicit `children`) and the 5 now-unused icon imports (SortAscending/Descending, Calendar, User, Tag). | **Low.** 3 named exports stay; behavior unchanged (fallback never executed). |
| R8 | FE-71 | `frontend/src/components/tasks/ClickedElementsBanner.tsx:13-16` | Remove the `appendInstructions` prop from the interface + component signature — declared, never passed by the sole caller (`TaskFollowUpSection`), never used internally. | **Low.** No caller breaks (prop never supplied). |
| R9 | FE-37 | `frontend/src/components/workflow/QualityBadge.tsx:9,11,50` | Remove the unused `totalIssues` (destructured, never read) and `mode` (declared, never read) props; drop the harmless pass-throughs in `PipelineView` (and dead `debug/TerminalDebugView` if not already deleted via D17). | **Low.** No behavior change. **Order-after** D17 (don't edit a file being deleted). |

**REFACTORS IN SCOPE total: 9 entries (R1–R9).**

---

## C. DEFERRED (reported, not executed)

Out-of-scope or higher-risk items. Each is a real candidate but is NOT executed in this pass —
either it touches generated types / persisted schema / cross-area code, it is `medium`/`low`
confidence, it needs a product-intent decision, or `fast-context` was down so cross-file/dynamic
usage is unverified. One-line reason each.

### Cross-area / generated-types / schema (must be coordinated, not frontend-only)

| ID(s) | Item | Reason deferred |
|---|---|---|
| CL-IDE: FE-09,10,11,14,25,26,27,28,29,30,31 | Entire "Open in external IDE/editor" feature removal | Lockstep FE+backend+i18n+`generate_types.rs` epic; `shared/types.ts` is GENERATED and `'ide-icon'` special icon feeds the ContextBar of BOTH workspaces (verified: `actions/index.ts:74,643,1018` + `CommandBar.tsx:22` + `ContextBar.tsx:163`). Removing `Actions.OpenInIDE` without removing both render branches breaks the bars. Owned by the cross-area G1 plan, not the frontend-only pass. |
| FE-10 | `lib/api.ts` `openEditor` methods + `OpenEditorApiRequest` | Backend `/open-editor` + `/editors/check-availability` routes orphan on removal; must delete AFTER backend handlers + regenerate types. CL-IDE scope. |
| FE-31 | IDE / onboarding i18n keys across 6 locales | Mirror-across-locales removal that must land WITH the CL-IDE feature deletion (CL-I18N + CL-IDE). |

### Persisted/versioned or barrel-resolution risk

| ID(s) | Item | Reason deferred |
|---|---|---|
| FE-18 (full) | Whole-file delete of `pages/settings/index.ts`, `stores/index.ts`, `components/rjsf/index.ts` barrels | Must first confirm tsconfig path aliases don't silently resolve these barrels; only the `stores/index.ts` wizard/workflow re-export lines are executed (D16). Full barrel deletion needs alias verification. |

### Atomic legacy clusters — gated on route/UI retirement

| ID(s) | Item | Reason deferred |
|---|---|---|
| CL-LEGACYPANELS: FE-03,05,12,51,68,81,82,87 | Old task-detail panels/cards/preview/kanban cluster | Atomic group cut only AFTER the legacy task-detail route is retired; transitive reachability + `DiffCard` IDE button (CL-IDE-adjacent). FE-12 needs lazy/dynamic-import check; not a frontend-only safe delete now. |
| CL-OLDUI: FE-15,74,76,77 | Old-design escape hatches (`OnboardingDialog`, `Open in Old UI`, `LegacyDesignScope`, `SlashCommands`) | Gated on `/commands` migration + old-route removal; FE-76 `LegacyDesignScope` still backs `/commands` styling. Premature removal breaks live routes. |
| FE-16 | `dialogs/projects/ProjectFormDialog.tsx` | `high` conf but requires editing `dialogs/index.ts` (3 exports) + verifying NiceModal registry; routed to a coordinated dialogs-barrel pass to avoid mid-pass barrel churn. |

### Duplication refactors — need a "single owner" decision

| ID(s) | Item | Reason deferred |
|---|---|---|
| CL-CONVDUP: FE-24,35,50,75 | `useMessageEditRetry`/`useRetryProcess` unify; `useFollowUpSend` vs `useSessionSend`; duplicate `useDiffData`/`DiffStats`/`parseDiffStats`; `SCRIPT_TOOL_NAMES`/`ScriptToolCallCard` dup | Multi-file unification touching live old+new conversation renderers; needs a keep-one-owner decision and careful behavior-equivalence testing. Out of scope for the safe-delete pass. |
| FE-23 | `ChatAssistantMessage` passthrough | `low` conf; may be intentional naming symmetry with `ChatThinkingMessage`. Investigate. |
| FE-33 | `useTaskAttempt` vs `useAttempt` double-cache | `medium`; uncertain whether separate invalidation scopes are intentional. |
| FE-34 | `useAttemptBranch` redundant full-Workspace fetch | `medium`; consolidation touches `TaskFollowUpSection`; verify against `useAttemptRepo`. |
| FE-62 | Model-CLI compatibility logic duplicated across 5 files | `high` but a 5-file extraction refactor; defer to a dedicated workflow-validators pass. |

### i18n coverage (additive, separate workstream)

| ID(s) | Item | Reason deferred |
|---|---|---|
| FE-32,53,58,59 | Missing locale sections (es/ja/ko/zh-Hant) + hardcoded English literals | Additive translation work (no dead code); separate CL-I18N workstream, not a streamline delete/refactor. |

### Investigate — unwired-but-maybe-intended / fast-context unverified

| ID(s) | Item | Reason deferred |
|---|---|---|
| FE-08 | `vscode/bridge.ts` iframe surface | KEEP clipboard helpers (LIVE via wysiwyg); cannot disprove out-of-repo webview host (fast-context down). Product-intent decision. |
| FE-47 | `StyleOverride.tsx` (Vibe iframe embed) | `VITE_PARENT_ORIGIN` unset; cannot disprove planned embedding (fast-context down). |
| FE-22 | Shared-tasks ElectricSQL branch | `VITE_ENABLE_SHARED_TASKS` unset + backend not inspected; flag-gated, keep shim. |
| FE-80 | wysiwyg image/pr-comment type exports | Type-only; dynamic/cross-repo import unconfirmed (fast-context down). |
| FE-38,39,40,45,55,59,60,61,63,65,66,67,68,70,72,73,81,82,84,85,86,87,88,89,90,92 | Remaining `investigate`/`medium`/`low` ledger rows | Each needs a per-item keep/cut call that the empty P3 verdict set did not provide; below the executed-confidence bar. FE-40 `QualityTimeline` + FE-41-adjacent may be G3/G13/G14 scaffolding — confirm intent. FE-55 silent partial credential save and FE-70 no-feedback Continue button are real bugs but routed to a settings/setup-flow fix pass. |
| FE-72 | Tautological terminal tests | Test-quality fix (tighten assertions), routed to the tests-area plan, not frontend deletes. |

---

## D. PRECONDITIONS & ORDERING (frontend area)

### Preconditions (must hold before executing A/B)
1. **Baseline green.** `frontend/` `tsc` + `vitest` + `eslint` pass on `refactor/streamline-quality-gates` before any edit (compare to `docs/baseline/tsc.log`). Re-run after each cluster.
2. **Do NOT touch `shared/types.ts`.** It is GENERATED by `crates/server/src/bin/generate_types.rs`. No frontend candidate in section A/B edits it. Any CL-IDE type removal (deferred) must regenerate it AFTER backend deletions, never hand-edit.
3. **Do NOT delete `vscode/bridge.ts`.** D19 deletes only `ContextMenu.tsx`; `bridge.ts` clipboard helpers are LIVE via `wysiwyg.tsx` (~10 chat boxes in both workspaces).
4. **No backend / migration coupling in this pass.** Frontend-only edits; the IDE backend routes and the persisted `config.editor` v9 schema field are out of scope (deferred to CL-IDE).
5. **Re-grep each delete target immediately before removal** (importers can change between census and execution): `rg "from .*<module>"` across `frontend/src`, since this area's census ran on grep-only (fast-context was 100% unavailable for frontend).

### Ordering (within the executed set)
- **O1 (D7, FE-78):** Inline the `<Navigate to="/workspaces/create" replace/>` into the corresponding `App.tsx` route FIRST, then delete `WorkspacesLanding.tsx`. Never delete before the route is rewired.
- **O2 (D18, FE-79):** Before deleting `pages/__tests__/WorkflowDebugPage.test.tsx`, diff it against canonical `pages/WorkflowDebugPage.test.tsx`; port any unique assertion into the canonical file, then delete the wrong-route duplicate.
- **O3 (D15 + D16, FE-17/18):** Delete `wizardStore.ts` + `workflowStore.ts` and remove their re-export lines from `stores/index.ts` in the SAME commit — order: edit barrel, then delete store files (so the build never references a deleted module).
- **O4 (D14 wizard dir):** Confirm `pages/Workflows.tsx` imports `WorkflowWizard` from `@/components/workflow` (live) and NOT `@/components/wizard` before deleting the dir.
- **O5 (D13, FE-69):** Grep `remoteCommitsAhead` inside `GitPanel.tsx` render body to confirm it is never destructured/forwarded, then remove the interface field.
- **O6 (R9 after D17):** Apply the `QualityBadge` prop cleanup (R9) AFTER the debug-component deletion (D17) so you don't edit a file that is about to be deleted; update the surviving caller `PipelineView` only.
- **General:** Execute CONFIRMED DELETES (A) before REFACTORS (B) where a refactor's caller is itself being deleted; otherwise A and B are independent. Run the frontend test+typecheck gate after each cluster (CL-ORPHAN, CL-PARTIAL, CL-WIZARDDUP, CL-DEBUGSTUB, CL-VSCODE, CL-CONVDUP) before proceeding.
