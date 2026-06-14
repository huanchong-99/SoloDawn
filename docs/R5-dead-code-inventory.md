# R5 — Repo-wide Dead / Redundant / Deprecated / Legacy Code Inventory

**Date:** 2026-06-13  
**Scope:** Full repo (~900k LOC) — Frontend React/TS + Rust workspace  
**Method:** Semantic search (fast_context_search), Grep for exact markers, Read for context verification

---

## 1. Frontend — HIGH-CONFIDENCE Removals

### 1.1 `frontend/src/components/tasks/TaskCard.tsx` + `TaskCardHeader.tsx` — DELETE

**Evidence:** The only importer of `components/tasks/TaskCard.tsx` is its own unit test (`TaskCard.test.tsx`, line 5: `import { TaskCard } from './TaskCard'`). No production file ever imports it. The file imports `KanbanCard` from `@/components/ui/shadcn-io/kanban` (the dead kanban import).  
`TaskCardHeader.tsx` is only imported by `TaskCard.tsx` (line 12).  
The active board card is `components/board/TaskCard.tsx`, which uses `@dnd-kit/core` and is imported by `WorkflowKanbanBoard.tsx`.

**Disposition:** DELETE both files + `TaskCard.test.tsx` in `components/tasks/`.

---

### 1.2 `frontend/src/components/ui/shadcn-io/kanban.tsx` — DELETE

**Evidence:** Only imported by the orphaned `components/tasks/TaskCard.tsx` (line 2: `import { KanbanCard } from '@/components/ui/shadcn-io/kanban'`). No other production file references this path. The legacy-components test `frontend/src/test/legacy-components.test.ts` (line 8) asserts `src/components/ui/shadcn-io/kanban` should NOT exist as a directory—confirming this is a removal candidate.

**Disposition:** DELETE.

---

### 1.3 `frontend/src/stores/wizardStore.ts` — DELETE (or refactor)

**Evidence:** `useWizardStore`, `useCurrentStepConfig`, `useWizardDirty` are exported from `stores/index.ts` (lines 30-33) but **zero production components** import them. A grep for `useWizardStore|useCurrentStepConfig|useWizardDirty|wizardStore` across `src/` finds only the file itself and `stores/index.ts`. The workflow wizard steps (`Step0Project` through `Step6Advanced`) use their own local state / `useWizardNavigation`/`useWizardValidation` hooks from `components/workflow/hooks/`, not this store.

**Disposition:** DELETE the file and remove its re-exports from `stores/index.ts`.

---

### 1.4 `frontend/tailwind.config.cjs` — DELETE (or repurpose to stub)

**Evidence:** PostCSS (`frontend/postcss.config.cjs:4`) loads `tailwind.new.config.js`. The new design CSS (`src/styles/new/index.css:493`) uses `@config '../../../tailwind.new.config.js'`. The legacy CSS (`src/styles/legacy/index.css:304`) uses `@config '../../../tailwind.legacy.config.js'`. `tailwind.config.cjs` has an empty `theme: { extend: {} }` body (7 lines total) and is only referenced by one test (`src/test/tailwind-config.test.ts`) that merely checks `config.content` is a non-empty array. The CI build does not load this file for any real work.

The `quality/wave2_findings.md` (W2-14-01/02) already flagged this as a critical issue: "PostCSS loads empty `tailwind.config.cjs` instead of `tailwind.new.config.js`; breaks all custom design tokens in production." However `postcss.config.cjs` has since been fixed to reference `tailwind.new.config.js` directly.

**Disposition:** DELETE `tailwind.config.cjs` and its test `src/test/tailwind-config.test.ts` (the test guards a stub nobody should care about).

---

### 1.5 `frontend/components.legacy.json` — DELETE

**Evidence:** This is the shadcn CLI config for installing components into the legacy design system (`tailwind.legacy.config.js`, `src/styles/legacy/index.css`, outDir `components/ui`). New components use `components.json` (which points to `tailwind.new.config.js`, `src/styles/new/index.css`, aliases `ui` → `@/components/ui-new`). `components.legacy.json` is not referenced by any build script, package.json script, or CI workflow.

**Disposition:** DELETE.

---

### 1.6 Legacy Design Route Cluster — REFACTOR (medium-confidence)

**Evidence:** The legacy design zone in `App.tsx` (lines 179-196) wraps `NormalLayout` in `LegacyDesignScope` and serves only two routes:
- `/commands` → `SlashCommands` page (which itself uses legacy `@/components/ui/` components)
- `/mcp-servers` → redirect to `/settings/mcp`
- `/projects` → redirect to `/board`

The two redirect routes are pure redirects (no UI render). Only the `/commands` route meaningfully uses `LegacyDesignScope`. If `SlashCommands` is ported to the new design, the entire legacy zone, `LegacyDesignScope.tsx`, `NormalLayout.tsx`, `frontend/src/styles/legacy/index.css`, and `frontend/tailwind.legacy.config.js` become removable.

**Disposition:** REFACTOR (port `/commands` to new design first, then DELETE the legacy zone cluster).

Files in cluster:
- `frontend/src/components/legacy-design/LegacyDesignScope.tsx` — REFACTOR → DELETE after port
- `frontend/src/components/layout/NormalLayout.tsx` — REFACTOR → DELETE after port
- `frontend/src/styles/legacy/index.css` — REFACTOR → DELETE after port
- `frontend/tailwind.legacy.config.js` — REFACTOR → DELETE after port

---

### 1.7 `frontend/src/pages/settings/index.ts` — DELETE

**Evidence:** This re-export barrel (`pages/settings/index.ts`) is only imported in one file: `frontend/src/pages/ui-new/settings/OrganizationSettingsNew.tsx`. All other consumers in `App.tsx` import directly from `@/pages/ui-new/settings`. The barrel adds a layer of indirection with no benefit.

**Disposition:** DELETE and update `OrganizationSettingsNew.tsx` to import directly.

---

### 1.8 `frontend/src/components/dialogs/global/OnboardingDialog.tsx` — UNKNOWN (likely keep)

**Evidence:** `OnboardingDialog` is exported from `dialogs/index.ts` (line 4-6) but is **not imported by `App.tsx`**. It appeared in `EditorAvailabilityIndicator.tsx` as a potential trigger but that component only references `useEditorAvailability`. The dialog itself exists and may be invoked from undiscovered paths. Cannot confirm dead — mark **unknown**.

---

### 1.9 `frontend/App.tsx:213` — `'kanban'` in `HotkeysProvider` — REFACTOR

**Evidence:** `<HotkeysProvider initiallyActiveScopes={['*', 'global', 'kanban']}>` — a grep for `scope.*kanban` or hotkeys registration with `'kanban'` scope across all source finds zero matches. This is a leftover from the legacy Kanban board. Removing it has no effect on any active hotkey.

**Disposition:** REFACTOR (remove `'kanban'` from the scopes array).

---

## 2. Backend Rust — HIGH-CONFIDENCE Candidates

### 2.1 `crates/utils/src/path.rs:128-131` — `get_gitcortex_temp_dir` — DELETE (with caution)

**Evidence:** Marked `#[deprecated(note = "Use get_solodawn_temp_dir instead")]`. Callers:
- `crates/services/src/services/cc_switch.rs:605` uses it under `#[allow(deprecated)]` (itself inside a test)
- `crates/services/src/services/cc_switch.rs:1323` inside test helper

All non-test production callers use `get_solodawn_temp_dir`. The function body just calls `get_solodawn_temp_dir`. Per `env_compat.rs` note, removal is safe no earlier than v0.2.0.

**Disposition:** DELETE when reaching v0.2.0 (currently at 0.0.153). Tag for removal.

---

### 2.2 `crates/utils/src/url.rs:90-106` — `normalize_base_url` — DELETE (with caution)

**Evidence:** Marked `#[deprecated(since = "0.0.154", note = "Use resolve_endpoint instead")]`. Still called in `crates/services/src/services/orchestrator/llm.rs:15` (production import) and `llm.rs:1155` (test). Tests in `url.rs:163-247` are all `#[allow(deprecated)]` guards.

**Disposition:** REFACTOR — migrate `orchestrator/llm.rs` callers to `resolve_endpoint`, then DELETE.

---

### 2.3 `crates/services/src/services/cc_switch.rs:1243-1252` — `switch_for_terminals` — DELETE

**Evidence:** Marked `#[deprecated(since = "0.2.0", note = "Use build_launch_config instead...")]`. Callers: only the companion test `cc_switch.rs:1317-1324` (explicitly tests that the deprecated method still compiles). No orchestrator code or server code calls this method. The orchestrator uses `build_launch_config`.

**Disposition:** DELETE the method and its test.

---

### 2.4 `crates/services/src/services/orchestrator/agent.rs:4416-4422` — `should_skip_completed_handoff` — KEEP

**Evidence:** Marked `#[allow(dead_code)]` but has inline unit tests at lines 10790-10797 that call it directly. The tests document expected behavior. If unused in production, consider removing after verifying the test captures the design intent.

**Disposition:** KEEP (test vehicle) — investigate whether the logic is actually dead in the call path or pending future use.

---

### 2.5 `crates/services/src/services/terminal/prompt_watcher.rs:173-179` — `is_bypass_permissions_enter_confirm_context` — DELETE

**Evidence:** Defined with `#[allow(dead_code)]`. A grep for `is_bypass_permissions_enter_confirm` shows it is defined once and never called anywhere else in the codebase.

**Disposition:** DELETE function.

---

### 2.6 `crates/services/src/services/terminal/prompt_watcher.rs:758-761` — `mark_pending_handoff_submit` — KEEP

**Evidence:** Marked `#[allow(dead_code)]` at the method level, but `prompt_watcher.rs:4046` calls `state.mark_pending_handoff_submit()`. The `#[allow]` annotation appears to be a false positive (the method IS called).

**Disposition:** KEEP, remove the erroneous `#[allow(dead_code)]`.

---

### 2.7 `crates/server/src/routes/oauth.rs` — stub handlers — REFACTOR

**Evidence:** `HandoffInitPayload` (line 52, `#[allow(dead_code)]`) and `HandoffCompleteQuery` (line 74, `#[allow(dead_code)]`) are deserializable structs for OAuth handoff. The handlers `handoff_init` and `handoff_complete` both immediately return `Err(ApiError::BadRequest("OAuth authentication is not supported..."))`. The routes are registered but serve only error responses. This is intentional scaffolding.

**Disposition:** KEEP as stub (explicit "not supported" contract) or DELETE routes entirely and remove from router — needs product decision.

---

### 2.8 `crates/server/src/routes/terminals.rs:43-83` — `TerminalSpawnConfig` + `spawn_command_to_runner_config` — KEEP (future)

**Evidence:** Annotated `// BACKLOG-002: Runner container separation` with `#[allow(dead_code)]`. These are placeholder abstractions for the planned gRPC runner migration. Not dead by design.

**Disposition:** KEEP, document in BACKLOG-002.

---

### 2.9 `crates/server/src/routes/terminal_ws.rs:60-89` — `TerminalIO` — KEEP (future)

**Evidence:** Same BACKLOG-002 pattern. `#[allow(dead_code)]` on the struct and its `impl`. Not dead — placeholder for RunnerClient migration.

**Disposition:** KEEP.

---

### 2.10 `crates/server/src/routes/workflow_events.rs:54-62` — `OrchestratorAwakened` + `OrchestratorDecision` — KEEP

**Evidence:** `#[allow(dead_code)]` on two enum variants. Comments say "Reserved for future use — not currently emitted... Kept to maintain a stable WebSocket event contract." These are intentionally reserved serialized names.

**Disposition:** KEEP.

---

### 2.11 `crates/services/src/services/events.rs:32-33` — `entry_count` field — KEEP

**Evidence:** `#[allow(dead_code)]` on `entry_count: Arc<RwLock<usize>>`. However, the field IS read at `events.rs:470-478` (incremented inside a hook closure for path generation). The `#[allow]` is a false positive at struct declaration level.

**Disposition:** KEEP, remove the `#[allow(dead_code)]` annotation.

---

### 2.12 `crates/db/src/models/execution_process.rs:103-108` — `UpdateExecutionProcess` — DELETE

**Evidence:** `#[allow(dead_code)]` on struct. A grep for `UpdateExecutionProcess` finds the struct definition only — no callers anywhere in the codebase.

**Disposition:** DELETE.

---

### 2.13 `crates/db/benches/workflow_bench.rs:230-end` — `_unused_keep_old_find_by_id_setup` — DELETE

**Evidence:** Function name starts with `_unused_keep_old_`, explicitly named as unused. Annotated `#[allow(dead_code)]`. Contains an inline SQLite schema that has drifted from real migrations (noted in the bench file header).

**Disposition:** DELETE the function.

---

### 2.14 `crates/services/src/services/chat_connector.rs:38-100` — `TelegramConnector` — DELETE

**Evidence:** `TelegramConnector` struct and its `impl ChatConnector` are defined in `chat_connector.rs` but never instantiated or referenced anywhere else in the codebase. Only `FeishuConnector` (in `feishu.rs`) implements `ChatConnector`. The struct has dummy `reqwest` client wiring but is never constructed.

**Disposition:** DELETE `TelegramConnector` struct and its `ChatConnector` impl. Keep the `ChatConnector` trait and `FeishuConnector`.

---

### 2.15 `crates/quality/src/rules/rust/error_handling.rs:65` — `#[allow(dead_code)]` — INVESTIGATE

**Evidence:** Single `#[allow(dead_code)]` at line 65. Context: rule test helper. Low risk.

**Disposition:** UNKNOWN — needs targeted read to determine if test helper is used.

---

## 3. Repository-Level — Stale Docs / Duplicate Config

### 3.1 `sonar-project.properties` (root) — REFACTOR / clarify

**Evidence:** The root file has `sonar.projectKey=huanchong-99_GitCortex` and `sonar.organization=huanchong-99` — the old GitCortex SonarCloud project. The CI (`ci-quality.yml:35`) uses `SonarSource/sonarqube-scan-action` which reads this root file by default. `quality/sonar/sonar-project.properties` has `sonar.projectKey=solodawn` and is used by the local SonarQube integration (`crates/quality/src/provider/sonar.rs:43`).

These serve different targets (SonarCloud vs local SonarQube), but the root file has the stale `GitCortex` key. If SonarCloud analysis is still needed, update the root file's `projectKey`; if SonarCloud is abandoned, DELETE the root file.

**Disposition:** REFACTOR — update root `sonar-project.properties` to use `solodawn` key, or DELETE if SonarCloud is no longer used.

---

### 3.2 `PHASE_13_SUMMARY.md` (repo root) — DELETE

**Evidence:** This is a Phase 13 implementation summary committed at the repo root (not under `docs/`). It has no references from any code file, CI, or README. `docs/developed/plans/PHASE_15_SUMMARY.md` follows the same pattern but is correctly under docs. Root-level phase summaries are orphaned artifacts.

**Disposition:** DELETE (or move to `docs/developed/plans/`).

---

### 3.3 Stale Issue Archive Docs — DELETE (low-risk cleanup)

**Evidence:** The following are point-in-time SonarCloud/audit snapshots that are never referenced by code or CI:
- `docs/developed/issues/sonarcloud-full-report-2026-02-28T15-36-45.md` (300 lines)
- `docs/developed/issues/sonarcloud-full-report-2026-03-01T03-46-12.md`
- `docs/developed/issues/sonarcloud-full-report-2026-03-01T04-49-19.md`
- `docs/developed/issues/sonarcloud-full-report-2026-03-13T09-11-07.md`
- `docs/developed/issues/sonarcloud-issues-2026-02-25T19-17-59代码质量审计报告.md`
- `docs/developed/issues/sonarcloud-issues-2026-02-26T14-55-39.md`
- `docs/developed/issues/sonarcloud-issues-2026-02-26T18-08-14.md`
- `docs/developed/issues/sonarcloud-issues-2026-02-27T09-50-32.md`
- `docs/developed/issues/sonarcloud-issues-2026-03-01T05-19-54.md`
- `docs/developed/misc/TODO-legacy-full-2026-02-23.md` (1285 lines)
- `docs/developed/misc/TODO-completed.md`
- `docs/developed/misc/concierge-progress-completed.md`
- `docs/developed/misc/orchestrator-chat-verification-report.md`
- `quality/wave1_findings.md`
- `quality/wave2_findings.md`
- `quality/triage_results.md`

These are historical artifacts useful for auditing but bloat the repo. They are not imported or processed by any tool.

**Disposition:** DELETE or move to a `docs/archive/` folder outside the main tree.

---

### 3.4 `docs/developed/plans/` — Completed Phase Plans — DELETE/ARCHIVE

**Evidence:** Files like `PHASE25-COMPLETION-REPORT.md`, `PHASE_15_SUMMARY.md`, `XX-phase-0-backend-foundation.md`, `XX-phase-1-frontend-core.md`, `XX-phase-2-integration.md`, and all dated `2026-01-*` through `2026-03-*` implementation plan files are historical implementation notes. None are referenced by code, CI, or active tooling.

**Disposition:** DELETE or archive to `docs/archive/plans/`.

---

### 3.5 `crates/utils/src/env_compat.rs` — Scheduled Removal

**Evidence:** Module header (lines 3-8) states "remove no earlier than v0.2.0". The GITCORTEX_ compat fallback is intentional for now but is a scheduled deletion.

**Disposition:** KEEP until v0.2.0, then DELETE compat paths (keep `var_with_compat` signature if needed for new vars).

---

## 4. Summary Table

| # | Path | Category | Confidence | Disposition |
|---|------|----------|------------|-------------|
| F1 | `frontend/src/components/tasks/TaskCard.tsx` | Orphaned component | HIGH | DELETE |
| F2 | `frontend/src/components/tasks/TaskCardHeader.tsx` | Orphaned component | HIGH | DELETE |
| F3 | `frontend/src/components/tasks/TaskCard.test.tsx` | Test for dead component | HIGH | DELETE |
| F4 | `frontend/src/components/ui/shadcn-io/kanban.tsx` | Unused shadcn primitive | HIGH | DELETE |
| F5 | `frontend/src/stores/wizardStore.ts` | Orphaned store | HIGH | DELETE |
| F6 | `frontend/tailwind.config.cjs` | Empty stub config | HIGH | DELETE |
| F7 | `frontend/src/test/tailwind-config.test.ts` | Test for stub | HIGH | DELETE |
| F8 | `frontend/components.legacy.json` | Stale shadcn CLI config | HIGH | DELETE |
| F9 | `frontend/src/pages/settings/index.ts` | Unused barrel | HIGH | DELETE |
| F10 | `frontend/src/components/legacy-design/LegacyDesignScope.tsx` | Active but legacy | MEDIUM | REFACTOR → DELETE |
| F11 | `frontend/src/components/layout/NormalLayout.tsx` | Legacy route layout | MEDIUM | REFACTOR → DELETE |
| F12 | `frontend/src/styles/legacy/index.css` | Legacy CSS (304 lines) | MEDIUM | REFACTOR → DELETE |
| F13 | `frontend/tailwind.legacy.config.js` | Legacy Tailwind config | MEDIUM | REFACTOR → DELETE |
| F14 | `frontend/App.tsx:213` `'kanban'` scope | Orphaned hotkey scope | HIGH | REFACTOR (1-line) |
| B1 | `crates/db/src/models/execution_process.rs:104` `UpdateExecutionProcess` | Unreferenced struct | HIGH | DELETE |
| B2 | `crates/db/benches/workflow_bench.rs:231` `_unused_keep_old_find_by_id_setup` | Explicitly orphaned bench fn | HIGH | DELETE |
| B3 | `crates/services/src/services/chat_connector.rs:38-100` `TelegramConnector` | Uninstantiated struct | HIGH | DELETE |
| B4 | `crates/services/src/services/terminal/prompt_watcher.rs:174` `is_bypass_permissions_enter_confirm_context` | Never-called fn | HIGH | DELETE |
| B5 | `crates/services/src/services/cc_switch.rs:1243-1252` `switch_for_terminals` | Deprecated, no prod callers | HIGH | DELETE |
| B6 | `crates/utils/src/url.rs:94` `normalize_base_url` | Deprecated, one prod caller | MEDIUM | REFACTOR then DELETE |
| B7 | `crates/utils/src/path.rs:129` `get_gitcortex_temp_dir` | Deprecated alias | MEDIUM | DELETE at v0.2.0 |
| B8 | `crates/services/src/services/events.rs:32` `#[allow(dead_code)]` on `entry_count` | False positive allow | HIGH | Remove annotation |
| B9 | `crates/services/src/services/terminal/prompt_watcher.rs:758` `#[allow(dead_code)]` on `mark_pending_handoff_submit` | False positive allow | HIGH | Remove annotation |
| R1 | `sonar-project.properties` (root) | Stale GitCortex key | MEDIUM | REFACTOR key or DELETE |
| R2 | `PHASE_13_SUMMARY.md` (root) | Orphaned root-level phase doc | HIGH | DELETE or MOVE |
| R3 | `docs/developed/issues/sonarcloud-*.md` (9 files) | Stale point-in-time reports | HIGH | DELETE/ARCHIVE |
| R4 | `docs/developed/misc/TODO-*.md` (3 files) | Completed TODO logs | HIGH | DELETE/ARCHIVE |
| R5 | `quality/wave*.md`, `quality/triage_results.md` | Audit finding logs | HIGH | DELETE/ARCHIVE |
| R6 | `docs/developed/plans/` (40+ dated files) | Completed plan docs | MEDIUM | ARCHIVE |

---

## 5. Items Verified as KEEP (not dead)

- `crates/deployment/src/lib.rs` — Active trait, used by `local-deployment` and `server`.
- `crates/local-deployment/src/**` — Active deployment implementation.
- `crates/runner/src/**` — Active gRPC runner binary (separate deployable).
- `crates/services/src/services/config/versions/v1..v8` — Migration chain for backward compat.
- `crates/services/src/services/orchestrator/agent.rs:4416` `should_skip_completed_handoff` — Has inline test coverage; investigate before removing.
- `crates/server/src/routes/terminals.rs` BACKLOG-002 stubs — Intentional future scaffolding.
- `crates/server/src/routes/terminal_ws.rs` BACKLOG-002 stubs — Same.
- `crates/server/src/routes/workflow_events.rs` reserved variants — Intentional stable API contract.
- `crates/utils/src/env_compat.rs` — Scheduled for v0.2.0 but needed now.
- `frontend/tailwind.new.config.js` — Active, used by PostCSS and `components.json`.
- `frontend/components.json` — Active, used for new design shadcn CLI.
- `quality/sonar/sonar-project.properties` — Active for local SonarQube.

---

## 6. Risk Notes

1. **LegacyDesignScope cluster (F10-F13):** The `/commands` route is the only live user of this cluster. If users use `/commands` regularly, removal is disruptive. Port first.
2. **`normalize_base_url` (B6):** `orchestrator/llm.rs` imports it in production. Must migrate callers before delete.
3. **`sonar-project.properties` root (R1):** CI SonarCloud scan uses this file. Deleting it without updating CI will break the sonar-analysis job.
4. **`stores/wizardStore.ts` (F5):** The store exports are re-exported from `stores/index.ts`. After deletion, update `stores/index.ts` to remove those lines or the TS compiler will error.
