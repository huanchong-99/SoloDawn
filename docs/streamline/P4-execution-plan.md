# P4 Execution Plan — MASTER (authoritative)

Date: 2026-06-14
Branch: `refactor/streamline-quality-gates`
Assembler: P4 Master Plan Assembler (Opus)
Area sub-plans merged:
- `docs/streamline/P4-execution-plan-rust.md` (29 confirmed deletes, 17 refactors, 52 deferred)
- `docs/streamline/P4-execution-plan-frontend.md` (20 confirmed deletes, 9 refactors, 63 deferred)

> **Provenance.** The P3 adversarial verdict array was empty (`[]`) for both areas — no per-candidate keep/cut verdict file was persisted. Both sub-plans were therefore synthesized conservatively from the authoritative ledgers (`P2-candidate-ledger.md`, `ledger-rust-backend.md`, `ledger-rust-quality.md`, `ledger-frontend.md`) + `docs/audit/R1-ide-editor-connection-deletion-audit.md`, with every CONFIRMED DELETE re-verified by Grep. **`mcp__fast-context__fast_context_search` was quota-/auth-exhausted across the entire census and during this assembly pass**, so static Grep is the strongest available verification signal. Anything `investigate`/`medium`/`low`/serde-persisted/cross-area-unverified was routed to DEFERRED, not executed blind. See §6 for items that remain fast-context-UNAVAILABLE and must be re-verified manually at execution time.

This file is the single authoritative instruction set for P4 implementation. Totals across both areas: **49 CONFIRMED DELETES** (29 rust + 20 frontend), **26 REFACTORS IN SCOPE** (17 rust + 9 frontend), **115 DEFERRED** (52 rust + 63 frontend).

---

## 1. GLOBAL ORDERING & SAFETY RULES

These rules bind the whole pass. Violating any one of them can red the build or corrupt persisted state.

1. **Baseline green before any edit.** Rust: `cargo check --workspace` + `cargo test --workspace --no-run`, compare to `docs/baseline/cargo-check.log`. Frontend: `tsc` + `vitest` + `eslint`, compare to `docs/baseline/tsc.log`. Do not start on a red baseline. Re-run the relevant gate after each cluster.

2. **Never edit applied sqlx migrations.** Frozen files include `20260312130000_create_quality_gates.sql` and `20260324160000_add_sync_toggles.down.sql` (RB-K10). Any schema fix (RB-44 FK, RB-45 plaintext-key DROP) is a **NEW forward migration** — additive `.sql` only, never an in-place edit; applied-migration checksums must not change. All such schema fixes are DEFERRED this pass.

3. **`shared/types.ts` is GENERATED — never hand-edit it.** It is produced by `crates/server/src/bin/generate_types.rs`. After ANY backend `#[derive(TS)]` type change (only the CL-IDE cluster does this in-scope), regenerate via `npm run generate-types` and commit the regenerated `shared/types.ts` in the SAME PR. CI runs `generate-types --check`; a stale `types.ts` fails the build. Frontend candidates in this pass do not touch `types.ts`.

4. **Keep `EditorConfig` / `EditorType` (RB-66).** These are the config-schema backbone (v2→v9 on-disk `editor:{}` JSON + every `vN::Config`). Their `generate_types.rs` registrations are lines **168-169** — DO NOT remove. If `remote_ssh_*` sub-fields are to go, that requires a **config v10 forward migration OR an inert/retained field** — plan it, do not silently delete persisted fields. This pass keeps the types intact.

5. **Keep `vscode/bridge.ts`; delete only `ContextMenu.tsx`.** Verified: `bridge.ts` clipboard helper `writeClipboardViaBridge` is LIVE via `frontend/src/components/ui/wysiwyg.tsx:45`. `ContextMenu.tsx` imports FROM `bridge.ts` (one-directional), so deleting ContextMenu only drops import edges INTO bridge — it does not orphan bridge.

6. **Cross-cluster execution order (rust):** D1 → D2 → D3 → D5 (all independent, single-file, zero cross-area) → D4 (server routes; FE already error-tolerant) → **D6 (CL-IDE) LAST** — the only cross-area, ordering-critical cluster. Quality (D5 + RQ refactors) may land as a separate PR (only `quality` + `server` recompile).

7. **Cross-cluster execution order (frontend):** CONFIRMED DELETES (A) before REFACTORS (B) wherever a refactor's caller is itself being deleted; otherwise independent. Run the frontend test+typecheck gate after each FE cluster.

8. **Serde / TS / persisted surfaces are FROZEN this pass:** RB-31b (`CreateFollowUpAttempt` `#[derive(TS)]`), RQ-18 (`MetricKey` serde-renamed enum in `quality_run` JSON + YAML), RQ-21 (`MeasureValue::None` in quality_run decision blob). Do not delete.

9. **Per-cluster compile gate (rust):** after each cluster, `cargo check -p <crate>` for the touched crate(s) and dependents along `db → services → server` and `quality → server`. After D6 + regen: full `cargo check --workspace` + `npm run generate-types -- --check`.

10. **Re-grep every delete target immediately before removal.** Line numbers below are P2-ledger anchors re-verified this pass, but files drift; the executor must re-confirm the enclosing block boundary before cutting. This is mandatory for the frontend area (census was grep-only).

11. **`CLAUDE.md` >3-edits-to-one-file rule:** `cc_switch.rs` is touched by RB-17, RB-26, RB-37, and the RB-56 decision — fold them into a single coherent pass over that file (see §4 ordering note). Same for any other file appearing in multiple rows.

---

## 2. CL-IDE DELETION SEQUENCE (ordered checklist — the only cross-area cluster)

This MUST be one coordinated FE+BE PR. Backend handlers/types die first, types regenerate, then the frontend strips call sites and deletes leaves. Backend deletes must land **before or with** the FE type-import removal, never after (FE imports would dangle). Verified anchors this pass below.

> **Verified anchor facts (Grep, this pass):**
> - There are **TWO** `OpenEditorRequest`/`OpenEditorResponse` definitions: `task_attempts.rs:794/805` (RB-61) and `projects.rs:379/389` (RB-65).
> - `repo.rs:23` imports the **projects** copy: `use routes::projects::{OpenEditorRequest, OpenEditorResponse};`.
> - `generate_types.rs:128-129` registers the **task_attempts** copy (`server::routes::task_attempts::OpenEditorRequest/Response::decl()`).
> - `generate_types.rs:114-115` (`CheckEditorAvailabilityQuery/Response`) and `:170` (`EditorOpenError`) belong to **RB-64 (DEFERRED)** — do NOT remove here.
> - `generate_types.rs:168-169` (`EditorConfig`/`EditorType`) are **RB-66 (KEEP)** — do NOT remove.
> - FE call sites are broader than the FE deferred row listed; full set verified below.

**Ordered checklist:**

- [ ] **Step 1 — Delete BE handlers + helpers + their tests (RB-61, RB-62, RB-63).**
  - RB-61 `crates/server/src/routes/task_attempts.rs`: `open_task_attempt_in_editor` (809-888), 4 helpers + test module (890-946, 965-1044), route reg (2143). **PRESERVE `status_semantics_tests` at 949-963.**
  - RB-62 `crates/server/src/routes/projects.rs`: `open_project_in_editor` (393-464), helpers (466-536), test (765-840).
  - RB-63 `crates/server/src/routes/repo.rs`: `open_repo_in_editor` (215-278), helpers + test (55-132), AND the import at `:23`.
  - RB-50 path-safety helper dupes (`repo.rs:55-98`, `projects.rs:421-464`, `task_attempts.rs:809-811`) vanish automatically — they lie inside the ranges above. No separate action.
- [ ] **Step 2 — Delete the shared TS-bearing type pairs AFTER their handlers.**
  - `task_attempts.rs:793-807` (the RB-61 pair) — delete LAST within that file (handler at 968 referenced it).
  - `projects.rs:378-391` (RB-65, the copy imported by repo.rs) — delete **strictly after** RB-62 AND RB-63 handlers are gone.
- [ ] **Step 3 — Remove the `generate_types.rs` registrations.** Delete lines **128-129** only (`task_attempts::OpenEditorRequest/Response::decl()`). Do NOT touch 114-115/170 (RB-64 deferred) or 168-169 (RB-66 keep). Bundle **RB-41** (HEADER path fix `crates/core/...` → `crates/server/...` at lines 9-11) into this same edit so the banner + content regenerate together.
- [ ] **Step 4 — Regenerate `shared/types.ts`.** Run `npm run generate-types`, commit the regenerated file. Then `npm run generate-types -- --check` must pass.
- [ ] **Step 5 — Strip FE call sites (do not leave dangling imports).** Verified FE consumers of the IDE/editor API:
  - `frontend/src/lib/api.ts`: `type OpenEditorApiRequest` (99), three `openEditor` methods (329, 791, 1113) — FE-10.
  - `frontend/src/components/DiffCard.tsx:267` `handleOpenInIDE` + button (312).
  - `frontend/src/components/layout/Navbar.tsx:120` `handleOpenInIDE` + onClick (193).
  - `frontend/src/hooks/useOpenProjectInEditor.ts:15`, `frontend/src/hooks/useOpenInEditor.ts:23`.
  - `frontend/src/components/ui-new/actions/index.ts`: `Actions.OpenInIDE` (640) incl. `openEditor` call (650), the `'ide-icon'` special icon (74, 643), the primary-bar entry (1008), and `isSpecialIcon` branch (1018); `actions/pages.ts:71`.
  - `frontend/src/components/ui-new/primitives/ContextBar.tsx:163` and `CommandBar.tsx:22` (`'ide-icon'` render branches) — **both branches must go together** or the bars break.
  - `frontend/src/components/ui-new/containers/GitPanelContainer.tsx:214` `repoApi.openEditor`.
  (FE-25/26/27/28/29/30 cover these in the frontend ledger.)
- [ ] **Step 6 — Delete FE leaf files** that exist only for the IDE feature (FE-09/11/14/30 per ledger), e.g. the `useOpenInEditor.ts` / `useOpenProjectInEditor.ts` hooks once all call sites are gone. Re-grep each before deletion.
- [ ] **Step 7 — Prune i18n (FE-31).** Remove IDE/onboarding editor keys across all 6 locales (es/ja/ko/zh-Hant/en/zh-Hans) — mirror-across-locales removal landing WITH this PR.
- [ ] **Step 8 — Config decision.** **KEEP `EditorConfig`/`EditorType` (RB-66).** Leave `config.editor` v9 schema field intact. If `remote_ssh_*` sub-fields are to be retired, do it as a **config v10 forward migration or an inert retained field** — NOT a silent delete (on-disk JSON depends on it). **RB-64** (`check_editor_availability` route + `EditorAvailabilityIndicator`/`useEditorAvailability` FE + generate_types 114-115 + `EditorOpenError` 170) is DEFERRED: cut only when the FE availability indicator is removed in this same lockstep — it is NOT safe standalone.

---

## 3. ALL OTHER CONFIRMED DELETES (grouped by subsystem)

Line numbers are P2-ledger anchors re-verified by Grep this pass. Re-confirm block boundaries before cutting.

### 3A. Rust — D1: Never-compiled / orphan files (zero blast radius, do first)

| id | file(s):lines |
|----|----------------|
| RB-01 | `crates/services/src/services/orchestrator/runtime_test.rs` (whole file 1-605; `mod runtime_test;` not found anywhere) |
| RB-02 | `crates/services/test_edge_cases.rs` (whole file 1-46; not in Cargo.toml) |
| RB-03 | `crates/services/src/services/share.rs` (whole file 1-51; no `mod share;`) |
| RB-04 | `crates/db/benches/workflow_bench.rs:231-342` (`_unused_keep_old_find_by_id_setup`) |

### 3B. Rust — D2: In-file dead types / fns / aliases (single-file, zero callers)

| id | file:lines |
|----|------------|
| RB-05 | `crates/db/src/models/execution_process.rs:101-108` (`UpdateExecutionProcess` + its `#[allow(dead_code)]`) |
| RB-09 | `crates/executors/src/executors/droid/normalize_logs.rs:789` (`EditToolResult`) |
| RB-10 | `crates/server/src/routes/event_bridge.rs:85` (`SharedEventBridge` type alias) |
| RB-18 | `crates/services/src/services/git.rs:1082-1108` (`get_commit_subject`, `ahead_behind_commits_by_oid`) + `:236-253` (`ensure_main_branch_exists`) |
| RB-26 | `crates/cc-switch/src/switcher.rs:83-94` (`switch_models_sequential`); `crates/cc-switch/src/config_path.rs:35-37` (`get_claude_mcp_path`), `:85-87` (`get_gemini_settings_path`) |
| RB-29 | `crates/services/src/services/orchestrator/persistence.rs:303-324` (`restore_conversation_history` ONLY; `clear_state` 278-301 is DEFERRED) |
| RB-31 | `crates/db/src/models/workspace.rs:84-93` (`CreatePrParams`), `:101-107` (`AttemptResumeContext`). `CreateFollowUpAttempt` 95-99 is DEFERRED (`#[derive(TS)]`). |
| RB-16 | `crates/services/src/services/chat_connector.rs:38-137` (`TelegramConnector` struct + impl + `ChatConnector for TelegramConnector` + comment header L30). KEEP `ChatConnector` trait + `FeishuConnector`. |
| RB-19 | `crates/services/src/services/merge_coordinator.rs:322-328` (`test_merge_coordinator_creation` no-assertion stub). Sibling `tests/merge_coordinator_test.rs` owned by tests area. |

### 3C. Rust — D3: Deprecated, test-only callers (delete fn + retarget/remove test)

| id | file:lines | note |
|----|------------|------|
| RB-12 | `crates/utils/src/path.rs:127-131` (`get_gitcortex_temp_dir`, `#[deprecated]`, 0 callers) | none |
| RB-13 | `crates/utils/src/url.rs:85-106` (`normalize_base_url`) **+** `crates/services/src/services/orchestrator/llm.rs:15` (`use`) **+** llm.rs test refs (1155-1167) **+** url.rs legacy test block (~163-247) | **single atomic commit** — the top-level `use` makes a partial delete fail to compile |
| RB-17 | `crates/services/src/services/cc_switch.rs:596-678` (`switch_for_terminal`/`switch_for_terminals`), `:1247-1253` (test `test_switch_for_terminals_method_exists`), `:462-465` (`CCSwitch` trait) + the `mod.rs` re-export of `CCSwitch` | fold with RB-56 decision (same file); breaks that one test by design |

### 3D. Rust — D4: Remote/stub routes returning 501/BadRequest (CL-REMOTE, server)

| id | file:lines | note |
|----|------------|------|
| RB-22 | `crates/server/src/routes/organizations.rs` (whole file 1-188) **+** `mod.rs:85` (`pub mod organizations;`) **+** `mod.rs:151` (`.merge(organizations::router())`) | none |
| RB-21 | `crates/server/src/routes/oauth.rs:64-128` (`handoff_init`/`handoff_complete`/`get_token`/`get_current_user`) **+** route regs `:43,:44,:47,:48` **+** their `#[allow(dead_code)]` DTOs. **KEEP `status()` (ConfigProvider) + `logout()` (FE caller).** | FE handoff/token/user callers already always fail |
| RB-23 | `crates/server/src/routes/projects.rs:158-207` (`link_project_to_existing_remote`, `create_and_link_remote_project`, `get_remote_project_by_id`, `get_project_remote_members`) + regs `:732,:739,:761`. **At `:737` EDIT the route to drop only `post(link_project_to_existing_remote)` and KEEP `.delete(unlink_project)`** (do NOT delete the whole `.route(...)`). Drop the `_repo_name` dead binding. | FE `linkToExisting`/`createAndLink`/`useProjectRemoteMembers` already error-handle |

### 3E. Rust — D5: Quality crate dead fns/fields (zero callers, no serde surface)

| id | file:lines |
|----|------------|
| RQ-01 | `crates/quality/src/gate/mod.rs:46-56` (`QualityGate::with_id`) |
| RQ-02 | `crates/quality/src/gate/condition.rs:36-41` (`Operator::to_db_value`) |
| RQ-03 | `crates/quality/src/gate/condition.rs:124-126` (`Condition::description`) |
| RQ-04 | `crates/server/src/self_test/runner.rs:19` (field `port`) **+** `:131` (assignment) |

### 3F. Frontend — CL-ORPHAN-WHOLEFILE (delete first, no ordering deps unless noted)

| ID | file(s) | note |
|---|---|---|
| FE-04 | `frontend/src/hooks/useVideoProgress.ts` (whole file) | 0 callers, not in `hooks/index.ts` |
| FE-20 | `frontend/src/components/ui/tabs.tsx` (whole file) | 0 importers; consumers use Radix directly |
| FE-21 | `frontend/src/components/ui-new/primitives/Card.tsx` (whole file) | 0 importers, no barrel |
| FE-46 | `frontend/src/utils/statusLabels.ts` (whole file) | labels come from i18n/inline |
| FE-48 | `frontend/src/types/modal-args.d.ts` (whole file) | stale ambient redeclaration; authoritative is `types/modals.ts` |
| FE-91 | `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx` **and** `.../primitives/PlanningChat.tsx` | delete BOTH; superseded by `CreateChatBoxContainer` |
| FE-78 | `frontend/src/pages/ui-new/WorkspacesLanding.tsx` (whole file) | **ORDER O1:** inline its `<Navigate to="/workspaces/create" replace/>` into the `App.tsx` route FIRST |

### 3G. Frontend — CL-PARTIAL (dead exports / fields inside live files)

| ID | file | range | note |
|---|---|---|---|
| FE-19 | `frontend/src/lib/types.ts` | 12-22 | delete `interface ConversationEntryDisplayType`; keep `AttemptData` |
| FE-36 | `frontend/src/components/workflow/constants.ts` | 79-88 | delete `export const GIT_COMMIT_TYPES` (0 imports) |
| FE-41 | `frontend/src/components/ui-new/utils/workflowStatus.ts` | 239-251 | delete `getWorkflowStatusMeta` + `getTerminalStatusMeta` |
| FE-42 | `frontend/src/components/ui-new/actions/useActionVisibility.ts` | 137-164 | delete `filterVisibleActionItems` |
| FE-43 | `frontend/src/components/ui-new/hooks/useWorkspaces.ts` | 78-80 | delete `export const workspaceKeys`; **keep** `workspaceSummaryKeys` |
| FE-69 | `frontend/src/components/ui-new/views/GitPanel.tsx` | 20 | delete `remoteCommitsAhead?: number` from `RepoInfo`. **ORDER O5:** grep `remoteCommitsAhead` in `GitPanel.tsx` render first |

### 3H. Frontend — CL-WIZARDDUP (atomic group)

| ID | file(s) | note |
|---|---|---|
| FE-02 | `frontend/src/components/wizard/` (whole dir, 10 files: StepIndicator, WorkflowConfigureStep, WorkflowExecuteStep, WorkflowReviewStep, WorkflowWizard — each `.tsx`+`.test.tsx`) | **ORDER O4:** confirm `pages/Workflows.tsx` imports `WorkflowWizard` from `@/components/workflow` (live), NOT `@/components/wizard` |
| FE-17 | `frontend/src/stores/wizardStore.ts` **and** `frontend/src/stores/workflowStore.ts` (whole files) | **ORDER O3:** paired with FE-18 in SAME commit |
| FE-18 | `frontend/src/stores/index.ts` (barrel) | remove the `wizardStore`/`workflowStore` re-export lines ONLY. Whole-barrel delete DEFERRED (alias check). Edit barrel, then delete store files. |

### 3I. Frontend — CL-DEBUGSTUB

| ID | file(s) | note |
|---|---|---|
| FE-06 | `frontend/src/components/debug/TerminalDebugView.tsx`+`.test.tsx`, `frontend/src/components/debug/TerminalSidebar.tsx`+`.test.tsx` (whole files) | live `/debug` uses `terminal/TerminalDebugView` (different signature) |
| FE-79 | `frontend/src/pages/__tests__/WorkflowDebugPage.test.tsx` (whole file) | **ORDER O2:** diff vs canonical `pages/WorkflowDebugPage.test.tsx`, port unique assertions, then delete the wrong-route (`/workflow/:id/debug`) duplicate |

### 3J. Frontend — CL-VSCODE (orphan only)

| ID | file | note |
|---|---|---|
| FE-07 | `frontend/src/vscode/ContextMenu.tsx` (whole file) | `WebviewContextMenu` never mounted. **KEEP `vscode/bridge.ts`** (live via `wysiwyg.tsx:45`) |

### 3K. Frontend — CL-CONVDUP (dead export only)

| ID | file | range | note |
|---|---|---|---|
| FE-13 | `frontend/src/components/ui-new/primitives/conversation/DiffViewCard.tsx` | 164-244 (full-card `DiffViewCard` export) + its line in `.../conversation/index.ts` barrel | **KEEP** `DiffViewBody`, `useDiffData`, `DiffInput`. `useDiffData` consolidation (FE-50) is DEFERRED |

---

## 4. REFACTORS IN SCOPE vs DEFERRED

### 4A. REFACTORS IN SCOPE (apply this pass)

These EDIT live code; behavior-preserving or clear-bug-fixing. **Bold** rows are the high-priority bugs.

**Rust (17):**

| id | file:lines | fixSketch | risk |
|----|------------|-----------|------|
| **RB-37** | `crates/services/src/services/cc_switch.rs:800-808` | **P0 SECURITY.** Clean up `claude_home/settings.json` + `codex_home/auth.json` temp-dir API-key/secret residue on terminal end. Hook `ProcessManager::finalize_terminated_process()` or a Drop-based `TempDirGuard`. Must run on panic/abort path too. | Medium |
| **RB-38** | `crates/services/src/services/error_handler.rs:150-158` (`activate_error_terminal`) | Replace `CliType::find_all().first()` / `ModelConfig::find_all().first()` with the workflow's `error_terminal_cli_id` / `error_terminal_model_id` (db `workflow.rs` L157/160) in the creation branch. | Low |
| **RB-39** | `crates/server/src/routes/terminals.rs:119-121` (`STARTABLE_TERMINAL_STATUSES`) | Remove `'working'` from the set (prevents double PTY spawn; matches `runtime_actions.rs` G15-007). Resolve the 3-way name collision (`runtime_actions.rs` 4-item, `constants.rs` 1-item) → one canonical const. | Low |
| RB-40 | `crates/server/src/routes/workflows.rs:3619-3632` (`get_workflow_events`) | Fix stale doc comment; `Json(...)` → `ResponseJson(...)`. | Low |
| RB-41 | `crates/server/src/bin/generate_types.rs:9-11` (HEADER) | Fix path `crates/core/...` → `crates/server/...`. **Bundle into the D6 regen step** (§2 Step 3). | Low-but-CI |
| RB-42 | `crates/executors/src/executors/claude.rs:106-113` | `plan=true && approvals=true` → construction-time error, not silent warn+plan-wins. | Low |
| RB-48 | `crates/services/src/services/orchestrator/agent.rs:665-690, 695-722` | Extract two near-identical "tasks-without-terminals → follow_up dispatch" blocks into one private fn; thread the "no markdown." diff as a param. | Low |
| RB-49 | `crates/server/src/routes/planning_drafts.rs:623-647, 862-884` | Extract the duplicated 4000-char truncate+push loop into `push_messages_to_feishu(..)`. | Low |
| RB-51 | `crates/services/src/services/terminal/process.rs:859-870` (`spawn_pty` shim) | Delete shim; retarget 3 test files to `spawn_pty_with_config`. | Low |
| RB-53 | `crates/services/src/services/git_host/github/mod.rs:9`, `azure/mod.rs:9` | Remove `pub use cli::GhCli` / `pub use cli::AzCli` (0 external importers). | Low |
| RB-K13 | `crates/services/src/services/orchestrator/prompt_watcher.rs:758` | Remove the MISLEADING `#[allow(dead_code)]` ONLY — `mark_pending_handoff_submit` IS called at L4046 (test). Do NOT remove the method. | Trivial |
| **RQ-05** | `crates/quality/src/rules/rust/error_handling.rs:47, 64-66` | `in_test` never set true → guards at 112/120/135/148 always take attr branch (dead conditional). Remove dead `in_test` field/branch + unread `content:&str`. | Low |
| **RQ-06** | `crates/quality/src/provider/frontend.rs:708-713` (`parse_eslint_summary`) | Move per-call `Regex::new` into `static`/`OnceLock<Regex>`. | Low |
| **RQ-07** | `crates/quality/src/provider/sonar.rs:150-181` (`wait_for_quality_gate`) | `_task_id:&str` unused (caller L285 passes `""`). Minimal fix: drop the dead param. | Low |
| RQ-10 | `crates/quality/src/analysis/coverage_parser.rs:168-180` (`extract_attr_f64/_u64`) | Replace 6 per-parse `Regex::new` with `LazyLock`/`OnceLock` or `str::find`. | Low |
| RQ-11 | `crates/quality/src/rules/rust/type_complexity.rs:202-205` | Remove dead `let _ = init;` (descent is via `syn::visit::visit_local` at L212). | Trivial |
| RQ-15 | `crates/quality/src/provider/delivery_readiness.rs:223-348` | Remove 3 Hoppscotch-hardcoded detectors + call sites; **confirm their MetricKeys are still emitted by other detectors first.** | Low |

**Frontend (9):**

| id | file | fixSketch | risk |
|----|------|-----------|------|
| **R1 / FE-01** | `frontend/src/lib/api.ts:607` (`uploadAuditDoc`) | Multipart field `'audit_doc'` → `'file'` to match backend `planning_drafts.rs` (else 400). Fixes 100%-broken AuditPlan System B upload. | Low |
| **R2 / FE-54** | `frontend/src/pages/ui-new/FirstRunWizard.tsx:48` | `'/api/cli-types/detect'` → `'/api/cli_types/detect'` (underscore). Fixes silently-empty CLI list. | Low |
| R3 / FE-57 | `frontend/src/components/ui-new/primitives/RepoCardSimple.tsx:28` | Replace undefined token `bg-tertiary` with a defined surface token (`bg-panel`/`bg-secondary`); confirm against `tailwind.config` + `new/index.css`. | Low |
| R4 / FE-56 | `frontend/src/components/board/TerminalActivityPanel.tsx:22,69` | Remove `'running'` from `ACTIVE_STATUSES` + the green `StatusIndicator` branch (no such enum value; legacy → `'working'`). | Low |
| R5 / FE-52 | `frontend/src/components/dialogs/settings/DeleteConfigurationDialog.tsx:34-42` | Remove try/catch around non-throwing `modal.resolve()+hide()` + the never-reset `isDeleting`. | Low |
| R6 / FE-44 | `frontend/src/lib/utils.ts:7-13` (`formatBytes`) | Un-export (only caller is same-file `formatFileSize`). | Low |
| R7 / FE-49 | `frontend/src/components/ui-new/primitives/Toolbar.tsx:83-105` | Remove dead `ToolbarDropdown` fallback JSX + 5 now-unused icon imports. | Low |
| R8 / FE-71 | `frontend/src/components/tasks/ClickedElementsBanner.tsx:13-16` | Remove never-passed `appendInstructions` prop. | Low |
| R9 / FE-37 | `frontend/src/components/workflow/QualityBadge.tsx:9,11,50` | Remove unused `totalIssues`/`mode` props + pass-throughs in `PipelineView`. **ORDER O6:** AFTER FE-06 (D17) deletion. | Low |

### 4B. DEFERRED (reported, NOT executed — for the user)

Routed out of this pass. Grouped by reason. **115 total** (52 rust + 63 frontend). Full per-item one-line reasons are in the area sub-plans (§DEFERRED of each); the high-blast-radius and decision-gated headlines:

**Rust — serde/TS/migration-persisted (frozen):** RB-31b (`CreateFollowUpAttempt` TS), RQ-18 (`MetricKey`), RQ-21 (`MeasureValue::None`), RQ-16 (`EvaluationResult::warn` sole `Level::Warn` producer), RB-44 (`WorkflowCommand.preset_id` FK — new fwd migration), RB-45 (plaintext-key DROP — new fwd migration).

**Rust — investigate (keep/cut decision needed):** RB-08, RB-29b (`clear_state`), RB-30, RB-32, RB-33, RB-34, RB-35, RB-36, RB-11, RB-14, RB-15, RB-24, RB-25, RB-28, RB-47, RQ-13, RQ-17, RQ-22, RQ-23/24, RQ-25.

**Rust — HIGH blast radius / packaging-unverified (do NOT cut without explicit confirmation):** RB-D18/CL-REMOTE (`crates/runner` gRPC + `bin/mcp_task_server.rs` — npm wrapper may shell into it), RB-K02 (`RedisBus`/`new_redis`/`from_env`), RB-K03 (`AcceptanceReviewResult` + `build_acceptance_review_prompt`/`fallback_default_audit_plan` — still the fallback when `audit_plan.raw_principles` empty).

**Rust — refactors deferred for risk/scope:** RB-43 (approval-window sleep — fix race first), RB-46 (`working_dir` repo.name vs path — invasive), RB-54 (pool builder), RB-55 (auto_prepare sleep — needs readiness poll), RB-56 (`backup_before_switch` — couple with RB-17), RB-57 (`FILE_STATS_CACHE` unbounded), RB-52, RB-58, RB-59, RB-K11 (`env_compat` — 30+ live call sites, migrate first), RB-K14 (legacy `/entries/{N}` patch path), RB-K15 (status-as-String typed-enum migration), RQ-08/09, RQ-12 (anti-stub `TOOL_TOKENS` allowlist — security-critical), RQ-14 (whole SonarProvider fate), RQ-19 (`FALLBACK_POLICY` — must compile out-of-tree), RQ-20 (`default_config` crash-safety fallback), RB-D01..RB-D17 (product-review feature items).

**Rust — CL-IDE tail (deferred until FE lockstep):** RB-64 (`check_editor_availability` — LIVE-wired, cut only with the FE availability indicator), RB-66 (`EditorConfig`/`EditorType` — KEEP).

**Rust — concierge leak (refactor needs new wiring):** RB-20 (`remove_session`/`cancel_watchers_for_session` never called → DashMap + watcher-token leak; WIRE into `DELETE /concierge/sessions/{id}`, do not delete).

**Frontend — cross-area / generated-types / schema:** CL-IDE FE-09/10/11/14/25/26/27/28/29/30/31 (handled in §2), FE-10 (`openEditor` + `OpenEditorApiRequest`), FE-31 (IDE/onboarding i18n).

**Frontend — barrel-resolution risk:** FE-18 full-barrel deletes (`pages/settings/index.ts`, `stores/index.ts`, `components/rjsf/index.ts` — alias-resolution check first; only the wizard/workflow re-export lines are executed via D16).

**Frontend — atomic legacy clusters gated on route/UI retirement:** CL-LEGACYPANELS (FE-03/05/12/51/68/81/82/87), CL-OLDUI (FE-15/74/76/77 — FE-76 `LegacyDesignScope` still backs `/commands`), FE-16 (`ProjectFormDialog` — dialogs-barrel pass).

**Frontend — duplication refactors (single-owner decision):** CL-CONVDUP FE-24/35/50/75, FE-23, FE-33, FE-34, FE-62 (5-file model-CLI compat extraction).

**Frontend — i18n coverage (additive workstream):** FE-32/53/58/59.

**Frontend — investigate / fast-context-unverified:** FE-08 (`bridge.ts` iframe surface — KEEP), FE-47 (`StyleOverride.tsx`), FE-22 (shared-tasks ElectricSQL flag), FE-80 (wysiwyg type exports), and FE-38/39/40/45/55/59/60/61/63/65/66/67/68/70/72/73/81/82/84/85/86/87/88/89/90/92. Note FE-55 (silent partial credential save) and FE-70 (no-feedback Continue button) are real bugs routed to a settings/setup-flow pass; FE-72 (tautological terminal tests) routed to the tests-area plan.

---

## 5. PRECONDITIONS & ORDERING (consolidated)

- **P-rust-1:** Baseline `cargo check --workspace` green; per-cluster `cargo check -p <crate>` along `db→services→server`, `quality→server`.
- **P-rust-2:** RB-13 single atomic commit (fn + `use` llm.rs:15 + tests llm.rs 1155-1167 + url.rs ~163-247).
- **P-rust-3:** RB-17/RB-26/RB-37/RB-56 all touch `cc_switch.rs` → single coherent pass over that file (>3-edit rule).
- **P-rust-4:** D6 strict ordering per §2 (handlers → types → generate_types 128-129 + RB-41 → regen → commit types.ts).
- **P-fe-1:** Baseline `tsc`+`vitest`+`eslint` green; re-run after each FE cluster.
- **P-fe-2:** Re-grep every FE delete target immediately before removal (census was grep-only).
- **O1 (FE-78):** inline `<Navigate>` into `App.tsx` route before deleting `WorkspacesLanding.tsx`.
- **O2 (FE-79):** diff + port unique assertions before deleting the wrong-route test duplicate.
- **O3 (FE-17/18):** edit `stores/index.ts` barrel, then delete the two store files, SAME commit.
- **O4 (FE-02):** confirm `Workflows.tsx` imports `WorkflowWizard` from `@/components/workflow` before deleting `components/wizard/`.
- **O5 (FE-69):** grep `remoteCommitsAhead` in `GitPanel.tsx` render before removing the field.
- **O6 (FE-37/R9):** apply `QualityBadge` prop cleanup AFTER FE-06 (D17) deletion.

---

## 6. STILL-UNAVAILABLE — verify manually (fast-context down across the whole census)

`mcp__fast-context__fast_context_search` was quota-/auth-exhausted across the **entire** P2 census and during this assembly pass. Both areas' census ran on Grep-only. The following carry residual cross-file / dynamic-usage uncertainty that fast-context would normally clear — **verify manually (broader Grep + Read of call chains) immediately before acting, or leave deferred:**

1. **RB-D18 / CL-REMOTE packaging** — does the npm wrapper shell into `bin/mcp_task_server.rs`? Is `crates/runner`'s `RunnerClientImpl` referenced by the live `Deployment` trait at runtime? Confirm packaging before any cut. (DEFERRED until confirmed.)
2. **RB-K02 RedisBus** — confirm no deployment sets `SOLODAWN_MESSAGE_BUS=redis`. (DEFERRED.)
3. **RB-K03 acceptance-review fallback** — confirm all workflows now carry `audit_plan.raw_principles` before removing the fallback. (DEFERRED.)
4. **RB-14** (`utils/api/oauth.rs` Handoff*/TokenRefresh*) and **RB-25** (`WorkflowDetailResponse` family) — possible out-of-tree / integration-test / generate_types consumers fast-context could not enumerate. (DEFERRED.)
5. **RQ-13** (`import_sarif_results`) and **RQ-23/24** (`issue.rs` `as_legacy`/`one_line_summary`/`location_string`) — pub on shared crates; possible out-of-tree consumers. (DEFERRED.)
6. **Frontend FE-08, FE-47, FE-80** — cannot disprove an out-of-repo webview/iframe host or dynamic/cross-repo type imports (`bridge.ts` iframe surface, `StyleOverride.tsx` `VITE_PARENT_ORIGIN`, wysiwyg type exports). (DEFERRED / KEEP.)
7. **Frontend FE-12** (legacy panels) — needs a lazy/dynamic-import reachability check fast-context would normally provide. (DEFERRED.)
8. **In-scope deletes** — even the CONFIRMED set was verified by Grep alone (no semantic search). Re-grep every target's importers (`rg "from .*<module>"` / `rg "<symbol>"`) immediately before cutting, per P-rust-1/P-fe-2 and Global Rule 10.

> Recovery note: a valid `WINDSURF_API_KEY` (Windsurf backend) is required for fast-context; an `Authentication error` means an expired token. If it is restored before P4 execution, re-run a semantic pass over the items in this section before cutting.

---

*End of master plan. Direct input to P4 implementation.*
