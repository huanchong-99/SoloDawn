# P4-FINAL Execution Plan — AUTHORITATIVE (supersedes P4-execution-plan.md)

Date: 2026-06-14
Branch: `refactor/streamline-quality-gates`
Assembler: P4 FINAL Plan Assembler (Opus)
Supersedes: `docs/streamline/P4-execution-plan.md` (the prior CONSERVATIVE plan)

> **Provenance.** This FINAL plan applies the ADVERSARIAL panel verdicts (skeptic refutation + investigate resolution) on top of the prior conservative plan. The prior plan's §1 safety rules, §2 CL-IDE 8-step sequence, §4A refactor list, and §5 ordering are GOOD and carried forward here (re-stated, with adjustments only where a verdict changed disposition). Where a skeptic REFUTED a confirmed delete, it is rerouted to keep/refactor below with the skeptic's live-usage evidence. Where the panel RESOLVED an `investigate` item to `delete`, it is PROMOTED into the confirmed-delete set with exact operations and ordering. `mcp__fast-context__fast_context_search` was still unavailable for the items in §7; those carry residual uncertainty and are flagged but the panel's exhaustive-ripgrep verdicts are honored.

**FINAL totals:** **75 CONFIRMED DELETES** (49 from the prior confirmed set, minus reroutes, plus investigate→delete promotions), **REFACTORS** = prior 26 + investigate→refactor promotions, **KEEP/deferred** = the remainder.

---

## 1. GLOBAL ORDERING & SAFETY RULES (carried forward verbatim, unchanged)

These rules bind the whole pass. Violating any one of them can red the build or corrupt persisted state.

1. **Baseline green before any edit.** Rust: `cargo check --workspace` + `cargo test --workspace --no-run`, compare to `docs/baseline/cargo-check.log`. Frontend: `tsc` + `vitest` + `eslint`, compare to `docs/baseline/tsc.log`. Do not start on a red baseline. Re-run the relevant gate after each cluster. (Windows build needs protoc + LLVM/libclang on PATH and sqlx-cli pinned 0.8.6, per MEMORY.)

2. **Never edit applied sqlx migrations.** Frozen files include `20260312130000_create_quality_gates.sql`, `20260324160000_add_sync_toggles.down.sql` (RB-K10), `20260117000001_create_workflow_tables.sql`, `20260119000000_encrypt_api_keys.sql`, `20260224001000_backfill_workflow_api_key_encrypted.sql`. Any schema fix is a **NEW forward migration** — additive `.sql` only, never an in-place edit; applied-migration checksums must not change.

3. **`shared/types.ts` is GENERATED — never hand-edit it.** Produced by `crates/server/src/bin/generate_types.rs`. After ANY backend `#[derive(TS)]` type change (CL-IDE cluster + RB-25 + RB-65 + RQ-23 fields), regenerate via `npm run generate-types` and commit the regenerated file in the SAME PR. CI runs `generate-types --check`.

4. **Keep `EditorConfig` / `EditorType` (RB-66).** Config-schema backbone (v2→v9 on-disk `editor:{}` JSON). `generate_types.rs` lines **168-169** — DO NOT remove.

5. **Keep `vscode/bridge.ts`; delete only `ContextMenu.tsx`.** `bridge.ts` clipboard helpers are LIVE via `frontend/src/components/ui/wysiwyg.tsx:45,108`. ContextMenu imports FROM bridge (one-directional), so deleting ContextMenu does not orphan bridge. (FE-08 KEEP confirmed by panel — see §6.)

6. **Cross-cluster execution order (rust):** D1 → D2 → D3 → D5 (all independent, single-file, zero cross-area) → D4 (server routes; FE already error-tolerant) → **D6 (CL-IDE) LAST**. Quality (D5 + RQ refactors) may land as a separate PR.

7. **Cross-cluster execution order (frontend):** CONFIRMED DELETES (A) before REFACTORS (B) wherever a refactor's caller is itself being deleted; otherwise independent. Run the frontend test+typecheck gate after each FE cluster.

8. **Serde / TS / persisted surfaces** — most prior freezes are now UNFROZEN by panel verdicts and listed as confirmed deletes (with regen steps): RB-31b (`CreateFollowUpAttempt` is a name-collision shadow, dead — delete), RQ-16 (`EvaluationResult::warn` — delete, with cascade), RB-25/RB-65 (TS pairs — delete with regen). STILL FROZEN/KEEP: RQ-18 (`MetricKey` extra variants — KEEP, strict-config blast radius), RQ-21 (`MeasureValue::None`), RB-44 (`WorkflowCommand.preset_id` FK → new fwd migration, REFACTOR), RB-45 (encrypt-key migrations — KEEP).

9. **Per-cluster compile gate (rust):** after each cluster, `cargo check -p <crate>` for the touched crate(s) and dependents along `db → services → server` and `quality → server`. After D6 + regen: full `cargo check --workspace` + `npm run generate-types -- --check`.

10. **Re-grep every delete target immediately before removal.** Line numbers are ledger/verdict anchors re-verified this pass, but files drift; re-confirm the enclosing block boundary before cutting. Mandatory for the frontend area.

11. **`CLAUDE.md` >3-edits-to-one-file rule:** `cc_switch.rs` is touched by RB-17, RB-26, RB-37, RB-56 → single coherent pass. `prompt_handler.rs` is touched by RB-06, RB-07, RB-08, RB-47 → single coherent pass. `agent.rs` (orchestrator) touched by RB-35, RB-36, RB-08 reader, RB-48 → single coherent pass. `git.rs` touched by RB-18. `condition.rs` (quality) touched by RQ-02, RQ-03, RQ-17 → single pass.

---

## 2. CL-IDE DELETION SEQUENCE (ordered checklist — the only cross-area cluster)

One coordinated FE+BE PR. Backend handlers/types die first, types regenerate, then FE strips call sites and deletes leaves. **Verdict updates:** RB-50 / RB-62 / RB-63 are now formally classified `refactor` (coordinated-cluster removal), but their concrete operations are exactly these steps — they are NOT standalone deletes and execute ONLY inside this lockstep. RB-65 is `delete` LAST (after RB-62 + RB-63). FE-14/FE-31 delete with this PR.

> **Verified anchor facts (carried forward, re-verified):**
> - TWO `OpenEditorRequest`/`OpenEditorResponse` definitions: `task_attempts.rs:793-807` (RB-61) and `projects.rs:378-391` (RB-65).
> - `repo.rs:23` imports the **projects** copy.
> - `generate_types.rs:128-129` registers the **task_attempts** copy (RB-61).
> - `generate_types.rs:114-115` (`CheckEditorAvailabilityQuery/Response`) and `:170` (`EditorOpenError`) belong to **RB-64 (DEFERRED)** — do NOT remove here.
> - `generate_types.rs:168-169` (`EditorConfig`/`EditorType`) = **RB-66 (KEEP)**.

**Ordered checklist:**

- [ ] **Step 1 — Delete BE handlers + helpers + their tests (RB-61 delete; RB-62, RB-63, RB-50 refactor-via-cluster).**
  - **RB-61** `crates/server/src/routes/task_attempts.rs`: `open_task_attempt_in_editor` (793-807 pair, 809-888 handler, 890-946 helpers, 965-1044 test), route reg (2143). **PRESERVE `status_semantics_tests` if present in adjacent range.**
  - **RB-62** `crates/server/src/routes/projects.rs`: `open_project_in_editor` (466-536), the 3 private helpers `normalize_editor_repo_path` (393-395), `resolve_project_repo_for_editor` (397-419), `resolve_repo_file_path_for_editor` (421-449), `resolve_editor_target_file_hint` (451-464), and `#[cfg(test)] mod open_editor_path_tests` (765-840), route reg (734). **Do NOT delete `OpenEditorRequest/Response` (378-391) here — that is RB-65 Step 2.**
  - **RB-63** `crates/server/src/routes/repo.rs`: `open_repo_in_editor` (215-278), helpers `resolve_repo_file_path_for_editor` + `resolve_editor_target_file_hint` + `#[cfg(test)] mod open_editor_path_tests` (55-132), route reg (354), AND the import at `:23` (`use routes::projects::{OpenEditorRequest, OpenEditorResponse}`) — must go WITH the handler or it becomes an unused-import error.
  - **RB-50** path-helper dupes vanish automatically inside the RB-62/RB-63 ranges (PATH A of its verdict). No separate action; do NOT extract to `editor_utils.rs` (that was the never-taken PATH B).
- [ ] **Step 2 — RB-65: delete the shared TS-bearing type pair AFTER both handlers.**
  - `crates/server/src/routes/projects.rs:378-391` (`OpenEditorRequest` + `OpenEditorResponse`, incl. `#[derive]` attrs) — delete **strictly after** RB-62 AND RB-63 handlers are gone (repo.rs:23 import already removed in Step 1).
- [ ] **Step 3 — Remove `generate_types.rs` registrations.** Delete lines **128-129** (`task_attempts::OpenEditorRequest/Response::decl()`). Do NOT touch 114-115/170 (RB-64 deferred) or 168-169 (RB-66 keep). Bundle **RB-41** (HEADER path fix `crates/core/...` → `crates/server/...` at 9-11) here.
- [ ] **Step 4 — Regenerate `shared/types.ts`.** `npm run generate-types`, commit. Then `npm run generate-types -- --check` must pass. (Drops `OpenEditorRequest`/`OpenEditorResponse` from generated TS.)
- [ ] **Step 5 — Strip FE call sites (no dangling imports).**
  - `frontend/src/lib/api.ts`: `type OpenEditorApiRequest` (99-101), three `openEditor` methods — `projectsApi.openEditor` (329-338), `attemptsApi.openEditor` (791-803), `repoApi.openEditor` (1113-1122) — FE-10.
  - `frontend/src/hooks/useOpenProjectInEditor.ts` (whole file), `frontend/src/hooks/useOpenInEditor.ts` (whole file, FE-09).
  - `frontend/src/components/layout/Navbar.tsx`: import (27), `useOpenProjectInEditor` call (79), `handleOpenInIDE` (120-122), `<OpenInIdeButton>` block (191-196), and `isSingleRepoProject`/`useProjectRepos` plumbing (82-83) if unused.
  - `frontend/src/components/dialogs/projects/ProjectEditorSelectionDialog.tsx` (whole file, FE-30 — delete or detach).
  - `frontend/src/components/ui-new/actions/index.ts`: `Actions.OpenInIDE` (640-665) incl. `openEditor` call (650); the `'ide-icon'` `SpecialIconType` arm (74) and `isSpecialIcon` arm (1018) — **remove ONLY the `'ide-icon'` part, KEEP `'copy-icon'`**; the primary-bar entry (1008 → `primary: [Actions.CopyPath]`); `getIdeName` import (56) if unused. `actions/pages.ts:71` (FE-26).
  - `frontend/src/components/ui-new/primitives/ContextBar.tsx:163-180` and `CommandBar.tsx:21-31` (`'ide-icon'` render branches) — **both go together** (FE-11 leaf `IdeIcon`/`OpenInIdeButton`).
  - `frontend/src/components/ui-new/containers/GitPanelContainer.tsx:212-229` `handleOpenInEditor` + `onOpenInEditor` wiring (289) → `GitPanel.tsx` prop (38,51,94) → `RepoCard.tsx` "Open in IDE" item (248-250) + prop (68,93) (FE-63 RB-63 FE half).
  - `frontend/src/components/NormalizedConversation/NextActionCard.tsx`: IDE button block (188-195), `handleOpenInEditor` (305-307), `useOpenInEditor` call (269), `getIdeName`/`IdeIcon` import (22), `useOpenInEditor` import (17), `editorName`/`editorType` props (FE-28).
  - `frontend/src/components/ui/actions-dropdown.tsx`: import (13/24), `useOpenInEditor` call (45), `handleOpenInEditor` (82-86), the `DropdownMenuItem` "Open in IDE" (192-197) (FE-29).
  - `frontend/src/components/DiffCard.tsx:271` and `ui-new/containers/DiffViewCardWithComments.tsx:193` openEditor call sites.
  - `frontend/src/components/ui-new/actions/index.ts` `EditorSelectionDialog` usage; `frontend/src/components/dialogs/tasks/EditorSelectionDialog.tsx` (whole file, FE-14).
- [ ] **Step 6 — Delete FE leaf files** that exist only for the IDE feature once all call sites are gone: `useOpenInEditor.ts`, `useOpenProjectInEditor.ts`, `EditorSelectionDialog.tsx`, `ProjectEditorSelectionDialog.tsx`, and the `ide/{IdeIcon,OpenInIdeButton}.tsx` pair (FE-11). Re-grep each before deletion.
- [ ] **Step 7 — Prune i18n (FE-31).** Remove `projects.openInIDE` (6 locales), `tasks.attempt.actions.{openInIde,openMenu,stopDevServer}`, `actionsMenu.openInIde`, `navbarActions.*` IDE keys, and onboarding/settings editor keys across all 6 locales (es/ja/ko/zh-Hant/en/zh-Hans) — landing WITH this PR.
- [ ] **Step 8 — Config decision.** **KEEP `EditorConfig`/`EditorType` (RB-66).** **RB-64** (`check_editor_availability` route + `EditorAvailabilityIndicator`/`useEditorAvailability` FE + generate_types 114-115 + `EditorOpenError` 170) remains DEFERRED — cut only with the FE availability indicator (`GeneralSettingsNew.tsx` consumer is LIVE; see §6 FE-09/FE-30 keep notes). Not safe standalone.

---

## 3. CONFIRMED DELETES (grouped by subsystem) — FINAL SET

Line numbers re-verified against verdicts. Re-confirm block boundaries before cutting.

### 3A. Rust — D1: Never-compiled / orphan files (zero blast radius, do first)

| id | file(s):lines | confidence |
|----|----------------|-----------|
| RB-01 | `crates/services/src/services/orchestrator/runtime_test.rs` (whole 1-605; no `mod runtime_test;`, omits `audit_plan` so cannot compile) | confirmed/skeptic×1 |
| RB-02 | `crates/services/test_edge_cases.rs` (whole 1-46; not in Cargo.toml) | confirmed/skeptic×1 |
| RB-03 | `crates/services/src/services/share.rs` (whole 1-51; no `mod share;`) | confirmed/skeptic×1, fc✓ |
| RB-04 | `crates/db/benches/workflow_bench.rs:231-342` (`_unused_keep_old_find_by_id_setup`) | confirmed/skeptic×1 |
| RQ-26 | `crates/server/benches/performance.rs` (whole file) **+** `crates/server/Cargo.toml:81-83` `[[bench]]` stanza **+** `[dev-dependencies]` `criterion` (78) + `sysinfo` (79). Run `cargo` to drop orphaned Cargo.lock entries. NOT executed by any CI; `--all-targets` clippy still passes after. | confirmed/skeptic×2 (NEW from investigate) |

### 3B. Rust — D2: In-file dead types / fns / aliases (single-file, zero callers)

| id | file:lines | confidence |
|----|------------|-----------|
| RB-05 | `crates/db/src/models/execution_process.rs:101-108` (`UpdateExecutionProcess` + `#[allow(dead_code)]`) | confirmed/skeptic×1, fc✓ |
| RB-09 | `crates/executors/src/executors/droid/normalize_logs.rs:789` (`EditToolResult`) | confirmed/skeptic×1 |
| RB-10 | `crates/server/src/routes/event_bridge.rs:85` (`SharedEventBridge` type alias) | confirmed/skeptic×1 |
| RB-16 | `crates/services/src/services/chat_connector.rs:38-137` (`TelegramConnector` struct+impl+trait-impl+header L30). KEEP `ChatConnector` trait + `FeishuConnector`. | confirmed/skeptic×2, fc✓ |
| RB-18 | `crates/services/src/services/git.rs:1082-1108` (`get_commit_subject`, `ahead_behind_commits_by_oid`) **+** `:236-253` (`ensure_main_branch_exists`) | confirmed/skeptic×2, fc✓ |
| RB-19 | `crates/services/src/services/merge_coordinator.rs:322-328` (`test_merge_coordinator_creation` no-assertion stub) | confirmed/skeptic×1 |
| RB-26 | `crates/cc-switch/src/switcher.rs:83-94` (`switch_models_sequential`); `crates/cc-switch/src/config_path.rs:35-37` (`get_claude_mcp_path`), `:85-87` (`get_gemini_settings_path`) | confirmed/skeptic×2 |
| RB-29 | `crates/services/src/services/orchestrator/persistence.rs:303-324` (`restore_conversation_history`) | confirmed/skeptic×2 |
| RB-31 | `crates/db/src/models/workspace.rs:84-93` (`CreatePrParams`), `:101-107` (`AttemptResumeContext`) | confirmed/skeptic×2, fc✓ |

### 3C. Rust — D2b: NEW in-file dead promotions (investigate→delete; single coherent pass per shared file)

`prompt_handler.rs` single pass (RB-06, RB-07, RB-08, RB-47):

| id | file:lines | confidence |
|----|------------|-----------|
| RB-06 | `crates/services/src/services/orchestrator/prompt_handler.rs:174-204` (`LLMPromptDecisionRequest`/`Response`) **+** `:688-740` (`build_llm_decision_prompt`) | confirmed/skeptic×2, fc✓ |
| RB-07 | `prompt_handler.rs:579-585` (`reset_terminal_state`) | confirmed/skeptic×1, fc✓ |
| RB-08 | `prompt_handler.rs`: field `task_contexts` @217 (+doc @216), inits @240 & @250, `set_task_context` @256-259, `clear_task_context` @262-265, and the reader block @503-506 + the `Task context: {task_ctx}` line in the `format!` @~509. After edit `cargo check -p services` (HashMap/Arc/RwLock still used elsewhere). | consensus (investigate→delete), fc✓ |
| RB-47 | `prompt_handler.rs:671-681` (`handle_user_approval` alias) **+** fix stale doc-comment at `agent.rs:9123` (`handle_user_approval` → `handle_user_prompt_response`). Tests `test_handle_user_approval_*` (1214,1329) call the canonical method — do NOT delete them. | consensus (investigate→delete) |

`agent.rs` (orchestrator) single pass (RB-35, RB-36):

| id | file:lines | confidence |
|----|------------|-----------|
| RB-35 | `crates/services/src/services/orchestrator/agent.rs:9101-9119` (`handle_terminal_failure` pub wrapper + doc). Watch for `error_handler` field becoming dead → that is a SEPARATE follow-up, do NOT expand here. | consensus (investigate→delete) |
| RB-36 | `agent.rs:4416-4422` (`should_skip_completed_handoff` + `#[allow(dead_code)]`) **+** companion test `agent.rs:10789-10798` (`should_skip_completed_handoff_for_continue_and_retry`). | consensus (investigate→delete) |

Other single-file investigate→delete promotions:

| id | file:lines | confidence |
|----|------------|-----------|
| RB-25 | `crates/server/src/routes/workflows.rs:62-69` (`WorkflowDetailResponse`) **+** `:79-85` (`WorkflowCommandWithPreset`). **KEEP `WorkflowTaskDetailResponse` (71-77) — LIVE** (return type of `list_workflow_tasks`, route 211, self-test 1296). Neither dead struct is `#[derive(TS)]` (Debug,Serialize only) → no shared/types regen needed. | consensus (investigate→delete), fc✓ |
| RB-28 | `crates/local-deployment/src/lib.rs:394-398` (`remote_client` inherent method) **+** `crates/deployment/src/lib.rs:55-57` (`RemoteClientNotConfigured` struct). Keep `use thiserror::Error;`. | consensus (investigate→delete) |
| RB-29b | `crates/services/src/services/orchestrator/persistence.rs:278-301` (`clear_state` + doc @278-280). Keep `use super::constants::WORKFLOW_STATUS_RUNNING` (still used @255). | consensus (investigate→delete) |
| RB-30 | `crates/services/src/services/orchestrator/message_bus.rs:541-556` (`publish_required` + doc). Keep `publish_inner`/trait/re-export. | consensus (investigate→delete) |
| RB-31b | `crates/db/src/models/workspace.rs:95-99` (`CreateFollowUpAttempt` — the **db** copy, single `prompt` field). NAME-COLLISION shadow of `server::routes::sessions::CreateFollowUpAttempt` (the 5-field live one registered in generate_types:119). Do NOT touch the server struct, generate_types, or shared/types.ts. Keep Deserialize/TS imports (used by other structs). | consensus (investigate→delete) |
| RB-32 | `crates/db/src/models/quality_run.rs:243-316` (`backfill_legacy_workflows`, `cleanup_old_runs`, `count_runs_by_age`) **+** `:319-324` (`AgeBucketCount` struct + doc). Keep `use ts_rs::TS;` (QualityRun uses it). | consensus (investigate→delete) |
| RB-33 | `crates/services/src/services/container.rs:103-123` (`has_running_processes` trait DEFAULT method + doc). No override exists; `ContainerService` is static-dispatch only. | consensus (investigate→delete), fc✓ |
| RB-34 | `crates/services/src/services/merge_coordinator.rs:219-248` (`resolve_and_complete_merge` + doc). **KEEP `broadcast_merge_success` (250+)** — still called by `merge_task_branch` @143. | consensus (investigate→delete) |
| RB-11 | `crates/services/src/services/git_host/detection.rs:63-88` (`detect_provider_from_pr_url`, `#[cfg(test)]`-gated) **+** tests `test_pr_url_github` (175-185) & `test_pr_url_azure` (187-201). Keep `detect_provider_from_url`. | consensus (investigate→delete) |
| RB-14 | `crates/utils/src/api/oauth.rs:5-46` (6 structs: `HandoffInitRequest/Response`, `HandoffRedeemRequest/Response`, `TokenRefreshRequest/Response`). Keep imports 1-3 and keepers 48-79 (`ProviderProfile`/`ProfileResponse`/`LoginStatus`/`StatusResponse`). NOT in generate_types decl list → no regen. | consensus (investigate→delete) — ⚠ §7 fc-unavailable |
| RB-15 | `crates/utils/src/jwt.rs` (whole 1-189) **+** `crates/utils/src/lib.rs:23` (`pub mod jwt;`) **+** `crates/utils/Cargo.toml:26` (`jsonwebtoken` dep, now orphaned). Production auth uses opaque tokens, not JWT. | consensus (investigate→delete) |
| RB-47 | (listed in prompt_handler pass above) | — |

### 3D. Rust — D3: Deprecated / test-only callers (delete fn + retarget/remove test)

| id | file:lines | note | confidence |
|----|------------|------|-----------|
| RB-12 | `crates/utils/src/path.rs:127-131` (`get_gitcortex_temp_dir`, `#[deprecated]`, 0 callers) | none | confirmed/skeptic×1 |
| RB-13 | `crates/utils/src/url.rs:85-106` (`normalize_base_url`) **+** `crates/services/src/services/orchestrator/llm.rs:15` (`use`) **+** llm.rs test refs (~1155-1167) **+** url.rs legacy test block (~163-247) | **single atomic commit** — top-level `use` makes partial delete fail to compile | confirmed/skeptic×2, fc✓ |
| RB-17 | `crates/services/src/services/cc_switch.rs:596-678` (`switch_for_terminal(s)`), `:1247-1253` (test `test_switch_for_terminals_method_exists`), `:462-465` (`CCSwitch` trait) + the `mod.rs` re-export of `CCSwitch` | fold with RB-26/RB-37/RB-56 (same file); breaks that one test by design | confirmed/skeptic×2, fc✓ |

### 3E. Rust — D4: Remote/stub routes (CL-REMOTE, server)

| id | file:lines | note | confidence |
|----|------------|------|-----------|
| RB-22 | `crates/server/src/routes/organizations.rs` (whole 1-188) **+** `mod.rs:85` (`pub mod organizations;`) **+** `mod.rs:151` (`.merge(organizations::router())`) | none | confirmed/skeptic×2 |
| RB-21 | `crates/server/src/routes/oauth.rs:64-128` (`handoff_init`/`handoff_complete`/`get_token`/`get_current_user`) **+** route regs `:43,:44,:47,:48` **+** their `#[allow(dead_code)]` DTOs. **KEEP `status()` + `logout()`.** | FE handoff/token/user callers already always fail | confirmed/skeptic×2, fc✓ |

> **RB-23 — REROUTED to REFACTOR** (was D4 confirmed-delete in the prior plan). See §4A. Only `get_remote_project_by_id` is deletable; the other 3 handlers have LIVE FE callers and a message-text contract.

### 3F. Rust — D5: Quality crate dead fns/fields (zero callers, no serde surface)

| id | file:lines | confidence |
|----|------------|-----------|
| RQ-01 | `crates/quality/src/gate/mod.rs:46-56` (`QualityGate::with_id`) | confirmed/skeptic×1 |
| RQ-02 | `crates/quality/src/gate/condition.rs:36-41` (`Operator::to_db_value`) | confirmed/skeptic×1, fc✓ |
| RQ-03 | `crates/quality/src/gate/condition.rs:124-126` (`Condition::description`) | confirmed/skeptic×1 |
| RQ-04 | `crates/server/src/self_test/runner.rs:19` (field `port`) **+** `:131` (assignment) | confirmed/skeptic×2 |
| RQ-25 | `crates/server/src/self_test/tests.rs:29` (`pub org_id: Option<String>`) **+** `:58` (`org_id: None,` init) | consensus (investigate→delete) |

NEW quality investigate→delete promotions (`condition.rs` folded into the RQ-02/RQ-03 pass):

| id | file:lines | note | confidence |
|----|------------|------|-----------|
| RQ-13 | `crates/quality/src/provider/sonar.rs:58-100` (`import_sarif_results` + doc) **+** `:102-147` (private `upload_sarif_to_sonar`, sole caller is RQ-13) **+** remove now-unused imports `issue::QualityIssue`, `rule::AnalyzerSource`, `sarif`. KEEP `check_sonar_health`/`wait_for_quality_gate`/trait impl. Engine's live SARIF path is `engine.rs::collect_sarif_issues`. | consensus (investigate→delete) — ⚠ §7 fc-unavailable | 
| RQ-16 | `crates/quality/src/gate/result.rs:101-113` (`EvaluationResult::warn`) **+** `crates/quality/src/gate/mod.rs:63-64` (unreachable Warn aggregation branch). OPTIONAL cascade (coordinated, pub enum variants): `status.rs:40`/`:49` (`Level::Warn`), `status.rs:16`/`:25` (`QualityGateStatus::Warn`), `report.rs:116` (Warn icon arm), `agent.rs:2829` & `:7936` (Warn→"warn" arms). Steps 1-2 alone compile (variants remain as never-constructed). Warn-mode shadow lives in orchestrator block/no-block logic, NOT the gate model. | consensus (investigate→delete) |
| RQ-17 | `crates/quality/src/gate/condition.rs:96-106` (`parse_threshold_f64` + doc) **+** `:108-121` (`parse_threshold_i64` + doc) **+** orphaned test `test_threshold_parsing` (166-171). Keep evaluator's private `parse_threshold`. | consensus (investigate→delete) |
| RQ-22 | `crates/quality/src/gate/mod.rs:99-102` (`is_blocked` + doc) **+** `:104-110` (`failed_conditions` + doc) **+** update the in-file test (184-185) to assert on `status`/filtered `condition_results`. KEEP `is_passed` (91-97, LIVE). Methods are not serialized (only struct fields are) → no DB/TS impact. | consensus (investigate→delete), fc✓ |
| RQ-23 | `crates/quality/src/issue.rs:146-150` (`as_legacy` builder method + doc). KEEP the `is_new` field (serde+TS+DB persisted, LIVE). `is_new` becomes invariantly true at runtime (no behavior change). | consensus (investigate→delete), fc✓ |
| RQ-24 | `crates/quality/src/issue.rs:163-170` (`location_string` + doc). **KEEP `IssueSummary::one_line_summary` (227-241) — LIVE** (report.rs:99,123). RQ-24 is a SPLIT: delete only `location_string`. | consensus (investigate→delete, SPLIT) — ⚠ §7 fc-unavailable for RQ-24 |

### 3G. Frontend — CL-ORPHAN-WHOLEFILE (delete first, no ordering deps unless noted)

| ID | file(s) | note | confidence |
|---|---|---|---|
| FE-04 | `frontend/src/hooks/useVideoProgress.ts` (whole) | 0 callers | confirmed/skeptic×1 |
| FE-20 | `frontend/src/components/ui/tabs.tsx` (whole) | 0 importers | confirmed/skeptic×1 |
| FE-21 | `frontend/src/components/ui-new/primitives/Card.tsx` (whole) | 0 importers | confirmed/skeptic×1 |
| FE-46 | `frontend/src/utils/statusLabels.ts` (whole) | labels from i18n/inline | confirmed/skeptic×1 |
| FE-48 | `frontend/src/types/modal-args.d.ts` (whole) | stale ambient redeclaration | confirmed/skeptic×2 |
| FE-86 | `frontend/src/components/ui-new/primitives/Field.tsx` (whole: FieldSet/Legend/Content/Title/Description/Separator) | 0 importers | confirmed/skeptic×2 |
| FE-91 | `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx` **and** `.../primitives/PlanningChat` | superseded by `CreateChatBoxContainer` | confirmed/skeptic×2 |
| FE-78 | `frontend/src/pages/ui-new/WorkspacesLanding.tsx` (whole) | **ORDER O1:** inline its `<Navigate to="/workspaces/create" replace/>` into `App.tsx` FIRST | confirmed/skeptic×3, fc✓ |
| FE-07 | `frontend/src/vscode/ContextMenu.tsx` (whole) | `WebviewContextMenu` never mounted. **KEEP `vscode/bridge.ts`** | confirmed/skeptic×2 |
| FE-47 | `frontend/src/utils/StyleOverride.tsx` (whole) | sole importer (`FullAttemptLogs.tsx`) already deleted; `VITE_PARENT_ORIGIN` undefined → inert | consensus (investigate→delete) — ⚠ §7 fc-unavailable |
| FE-40 | `frontend/src/components/quality/QualityTimeline.tsx` **+** `__tests__/QualityTimeline.test.tsx` | 0 importers; no barrel. KEEP sibling QualityReportPanel/QualityIssueList | consensus (investigate→delete) |

### 3H. Frontend — CL-PARTIAL (dead exports / fields inside live files)

| ID | file | range | note | confidence |
|---|---|---|---|---|
| FE-19 | `frontend/src/lib/types.ts` | 12-22 | delete `interface ConversationEntryDisplayType`; keep `AttemptData` | confirmed/skeptic×1 |
| FE-36 | `frontend/src/components/workflow/constants.ts` | 79-88 | delete `GIT_COMMIT_TYPES` | confirmed/skeptic×2 |
| FE-41 | `frontend/src/components/ui-new/utils/workflowStatus.ts` | 239-251 | delete `getWorkflowStatusMeta` + `getTerminalStatusMeta` | confirmed/skeptic×2 |
| FE-42 | `frontend/src/components/ui-new/actions/useActionVisibility.ts` | 137-164 | delete `filterVisibleActionItems` | confirmed/skeptic×1 |
| FE-43 | `frontend/src/components/ui-new/hooks/useWorkspaces.ts` | 78-80 | delete `workspaceKeys`; **keep** `workspaceSummaryKeys` | confirmed/skeptic×2 |
| FE-80 | `frontend/src/components/ui/wysiwyg/nodes/image-node.tsx` (23-29 `SerializedImageNode`, 216 `ImageNodeInstance`, +clean imports 2 & GeneratedDecoratorNode) **and** `pr-comment-node.tsx` (27-30 `SerializedPrCommentNode`, 90 `PrCommentNodeInstance`, +clean imports). KEEP all value exports + `NormalizedComment`/`ImageData`. | type-only exports | consensus (investigate→delete) — ⚠ §7 fc-unavailable |
| FE-19/FE-13 see §3K | | | | |

### 3I. Frontend — CL-WIZARDDUP (atomic group)

| ID | file(s) | note | confidence |
|---|---|---|---|
| FE-02 | `frontend/src/components/wizard/` (whole dir, 10 files) | **ORDER O4:** confirm `pages/Workflows.tsx` imports `WorkflowWizard` from `@/components/workflow` (live), NOT `@/components/wizard` | confirmed/skeptic×2 |
| FE-17 | `frontend/src/stores/wizardStore.ts` **and** `frontend/src/stores/workflowStore.ts` (whole) | **ORDER O3:** paired with FE-18 store-barrel edit | confirmed/skeptic×2 |
| FE-18 (stores barrel) | `frontend/src/stores/index.ts` | **DELETE whole file** (0 consumers — panel confirmed). Coordinate with FE-17. | consensus (investigate→delete, per-barrel split) |
| FE-18 (settings barrel) | `frontend/src/pages/settings/index.ts` | **DELETE whole file** (0 references repo-wide). | consensus (investigate→delete, per-barrel split) |

> **FE-18 (rjsf barrel) — REROUTED to REFACTOR/KEEP.** `frontend/src/components/rjsf/index.ts` is LIVE (`ExecutorConfigForm.tsx:11 import { shadcnTheme } from './rjsf'`). See §4B. The prior plan correctly flagged this barrel for an alias check; the panel resolved it: stores + settings barrels delete, rjsf barrel keep-or-repoint.

### 3J. Frontend — CL-DEBUGSTUB / CL-CONVDUP / CL-LEGACYPANELS (deletes only)

| ID | file(s) | note | confidence |
|---|---|---|---|
| FE-06 | `frontend/src/components/debug/{TerminalDebugView,TerminalSidebar}.{tsx,test.tsx}` (whole) | live `/debug` uses `terminal/TerminalDebugView` (different signature) | confirmed/skeptic×2 |
| FE-13 | `frontend/src/components/ui-new/primitives/conversation/DiffViewCard.tsx:164-244` (full-card `DiffViewCard` export) + its barrel line | **KEEP** `DiffViewBody`, `useDiffData`, `DiffInput` | confirmed/skeptic×2 |
| FE-05 | `frontend/src/components/tasks/{TaskCard,TaskCard.test,TaskCardHeader}.tsx` (whole) | live Kanban uses `board/TaskCard.tsx` | confirmed/skeptic×2 |
| FE-12 | `frontend/src/components/tasks/TaskDetails/preview/{DevServerLogsView,NoServerContent,PreviewToolbar,ReadyContent}.tsx` (whole) + remove empty dir | superseded by `ui-new/views/PreviewBrowser`; orphans `companionInstallTask.ts` (follow-up) | confirmed/skeptic×3 — ⚠ §7 fc-unavailable |
| FE-15 | `frontend/src/components/dialogs/global/OnboardingDialog.tsx` (whole) | superseded by new setup wizard; `.show()` never called | confirmed/skeptic×2 |
| FE-16 | `frontend/src/components/dialogs/projects/ProjectFormDialog.tsx` (whole) | 0 callers | confirmed/skeptic×2 |
| FE-74 | `frontend/src/components/ui-new/actions/index.ts:579-608` (`Actions.OpenInOldUI`) + NavbarActionGroups.left (982) + `pages.ts:74` + Navbar TOOLTIP_I18N_MAP (33-34) + `navbarActions.open-in-old-ui` in 6 locales | old-UI route already removed; escape hatch dead | consensus (investigate→delete) |

### 3K. Frontend — CONVDUP / hooks dedup deletes (investigate→delete; some are refactor — see §4)

| ID | file(s) | note | confidence |
|---|---|---|---|
| FE-35 | DELETE the orphaned old-design follow-up tree: `frontend/src/components/panels/TaskAttemptPanel.tsx` (0 importers) + `frontend/src/components/tasks/TaskFollowUpSection.tsx` (sole importer was TaskAttemptPanel) + `frontend/src/hooks/useFollowUpSend.ts` (FE-35). Update JSDoc at `useSessionSend.ts:37`. | consensus (investigate→delete) — NOTE this also resolves FE-81's inline-queue redundancy (dead-by-association) | consensus |
| FE-81 | (resolved by FE-35 tree delete; do NOT surgically extract lines 438-487 — delete the host) | — | consensus |

### 3L. Rust/Frontend — TESTS area deletes (T-series)

| id | file:lines | note | confidence |
|----|------------|------|-----------|
| T01 | `crates/server/tests/cli_detection_test.rs:1-38` (whole) | stub | confirmed/skeptic×2, fc✓ |
| T02 | `crates/server/tests/slash_commands_pool_test.rs:1-42` (whole) | leaf test, no unique coverage | confirmed/skeptic×2 |
| T04 | `frontend/src/hooks/useQualityGate.test.tsx:1-268` (whole) | duplicate location | confirmed/skeptic×2, fc✓ |
| T05 | `frontend/src/pages/WorkflowDebugPage.test.tsx:1-102` (whole — the OUTER/canonical duplicate) | **KEEP the inner `pages/__tests__/WorkflowDebugPage.test.tsx`** (FE-79). T05 deletes the less-comprehensive outer file. | confirmed/skeptic×2, fc✓ |
| T06 | `frontend/src/components/tasks/{TaskCard.tsx,TaskCard.test.tsx,TaskCardHeader.tsx}` (whole — same set as FE-05) | delete all 3 together; KEEP `tasks/UserAvatar.tsx` (live via `org/MemberListItem.tsx`). Unlocks FE-51 kanban.tsx (separate). | consensus (investigate→delete) |
| T07 | `crates/services/src/services/orchestrator/tests.rs:678-693` (`test_instruction_parsing`) | confirmed/skeptic×2, fc✓ |
| T12 | `crates/server/tests/performance/database_perf_test.rs` (whole) **+** remove `pub mod database_perf_test;` from `tests/performance/mod.rs:11`. ORPHANED from build graph (not a compiled target; only `[[bench]]` named "performance" exists). Optionally delete the whole orphaned `tests/performance/` dir (terminal/websocket/db perf + mod.rs). | consensus (investigate→delete) |
| T20 | `frontend/src/test/canvas-mock.test.ts:1-11` (whole) | self-contained meta-test, 0 refs | confirmed/skeptic×2 |

### 3M. Frontend — additional confirmed orphan deletes

| ID | file(s) | note | confidence |
|---|---|---|---|
| FE-91 | (listed in §3G) | — | — |

---

## 4. REFACTORS

### 4A. REFACTORS IN SCOPE (carried forward from prior §4A + investigate→refactor promotions)

**Carried-forward Rust refactors (prior §4A, unchanged):**

| id | file:lines | fixSketch | risk |
|----|------------|-----------|------|
| **RB-37** | `crates/services/src/services/cc_switch.rs:800-808` | **P0 SECURITY.** Clean `claude_home/settings.json` + `codex_home/auth.json` temp-dir secret residue on terminal end (Drop-based `TempDirGuard`; panic/abort path too). Fold into the cc_switch.rs single pass. | Medium |
| **RB-38** | `crates/services/src/services/error_handler.rs:150-158` | Replace `CliType/ModelConfig::find_all().first()` with workflow's `error_terminal_cli_id`/`error_terminal_model_id`. | Low |
| **RB-39** | `crates/server/src/routes/terminals.rs:119-121` | Remove `'working'` from `STARTABLE_TERMINAL_STATUSES`; collapse 3-way name collision to one canonical const. | Low |
| RB-40 | `crates/server/src/routes/workflows.rs:3619-3632` | Fix stale doc; `Json(...)` → `ResponseJson(...)`. | Low |
| RB-41 | `crates/server/src/bin/generate_types.rs:9-11` | Path fix `crates/core/...` → `crates/server/...`. **Bundle into D6 Step 3.** | Low-but-CI |
| RB-42 | `crates/executors/src/executors/claude.rs:106-113` | `plan=true && approvals=true` → construction-time error. | Low |
| RB-48 | `crates/services/src/services/orchestrator/agent.rs:665-690, 695-722` | Extract two near-identical follow_up dispatch blocks into one private fn. | Low |
| RB-49 | `crates/server/src/routes/planning_drafts.rs:623-647, 862-884` | Extract duplicated truncate+push into `push_messages_to_feishu(..)`. | Low |
| RB-51 | `crates/services/src/services/terminal/process.rs:859-870` | Delete `spawn_pty` shim; retarget 3 test files to `spawn_pty_with_config`. | Low |
| RB-53 | `crates/services/src/services/git_host/github/mod.rs:9`, `azure/mod.rs:9` | Remove `pub use cli::GhCli`/`AzCli`. | Low |
| RB-K13 | `crates/services/src/services/orchestrator/prompt_watcher.rs:758` | Remove MISLEADING `#[allow(dead_code)]` ONLY (`mark_pending_handoff_submit` IS called at L4046 test). | Trivial |
| **RQ-05** | `crates/quality/src/rules/rust/error_handling.rs:47, 64-66` | Remove dead `in_test` field/branch + unread `content:&str`. | Low |
| **RQ-06** | `crates/quality/src/provider/frontend.rs:708-713` | `Regex::new` → `static`/`OnceLock`. | Low |
| **RQ-07** | `crates/quality/src/provider/sonar.rs:150-181` | Drop dead `_task_id:&str` param. (Coordinate with RQ-13 delete in same sonar.rs pass.) | Low |
| RQ-10 | `crates/quality/src/analysis/coverage_parser.rs:168-180` | 6 per-parse `Regex::new` → `LazyLock`/`OnceLock`. | Low |
| RQ-11 | `crates/quality/src/rules/rust/type_complexity.rs:202-205` | Remove dead `let _ = init;`. | Trivial |
| RQ-15 | `crates/quality/src/provider/delivery_readiness.rs:223-348` | Remove 3 Hoppscotch detectors; **confirm their MetricKeys still emitted elsewhere first.** | Low |

**Carried-forward Frontend refactors (prior §4A) — with verdict adjustments:**

| id | file | fixSketch | risk |
|----|------|-----------|------|
| **R1 / FE-01** | `frontend/src/lib/api.ts:607` | Multipart `'audit_doc'` → `'file'`. Fixes 100%-broken AuditPlan upload. | Low |
| **R2 / FE-54** | `frontend/src/pages/ui-new/FirstRunWizard.tsx:48` | `/api/cli-types/detect` → `/api/cli_types/detect`. | Low |
| R3 / FE-57 | `frontend/src/components/ui-new/primitives/RepoCardSimple.tsx:28` | `bg-tertiary` → defined token. | Low |
| R4 / FE-56 | `frontend/src/components/board/TerminalActivityPanel.tsx:22,69` | Remove `'running'` from `ACTIVE_STATUSES`. | Low |
| R5 / FE-52 | `frontend/src/components/dialogs/settings/DeleteConfigurationDialog.tsx:34-42` | Remove try/catch around non-throwing resolve/hide. | Low |
| R6 / FE-44 | `frontend/src/lib/utils.ts:7-13` | Un-export `formatBytes`. | Low |
| R7 / FE-49 | `frontend/src/components/ui-new/primitives/Toolbar.tsx:83-105` | Remove dead `ToolbarDropdown` fallback + 5 unused icon imports. | Low |
| R8 / FE-71 | `frontend/src/components/tasks/ClickedElementsBanner.tsx:13-16` | Remove never-passed `appendInstructions`. (Watch: ClickedElementsBanner may be orphaned by FE-35 tree delete — re-grep.) | Low |
| **R9 / FE-37** | `frontend/src/components/workflow/QualityBadge.tsx:9,11` | **REROUTED — now a 3-file atomic edit.** Remove `totalIssues` (9) AND `mode` (11) from `QualityBadgeProps`; AND remove `totalIssues={data.totalIssues}` at `PipelineView.tsx:75` AND `terminal/TerminalDebugView.tsx:746`. `mode` is truly orphaned; `totalIssues` HAS 2 live callers (TS2322 if not removed together). Do NOT touch `QualityReportPanel.tsx:93` (gateStatus only) or `debug/TerminalDebugView.tsx` (no QualityBadge). | Low |

**NEW investigate→refactor promotions:**

| id | file:lines | operations | risk |
|----|------------|-----------|------|
| RB-23 | `crates/server/src/routes/projects.rs` | **SAFE NOW:** delete only `get_remote_project_by_id` (191-198) + its route reg (759-762). KEEP `RemoteProject` import (used by `get_project_remote_members:203` + `organizations.rs:63` + `services/project.rs:142`). **DO NOT delete** `link_project_to_existing_remote` (158-166), `create_and_link_remote_project` (168-177), `unlink_project` (179-189 — real DB code), `get_project_remote_members` (200-207), or their route regs (732,737,739). Their BadRequest message strings must remain byte-for-byte (consumed by `organizationRemoteCapability.ts:23` regex). Drop only the truly-unused `Path` import if no other handler uses it. | Low |
| RB-20 | `crates/services/.../concierge/{sync.rs,agent.rs}` + `crates/server/src/routes/concierge.rs:219-227` | **Leak fix (keep all 5 symbols).** Add `Extension(concierge)` + `Extension(Arc<ConciergeBroadcaster>)` to `delete_session`; call `concierge.cancel_watchers_for_session(&id)` + `broadcaster.remove_session(&id)`. Verify the REST router carries the broadcaster Extension layer; add `.layer(...)` if only the WS router has it. self_test `test_delete_concierge_session` must still pass (200). Feishu register/push wiring tracked separately. | Medium |
| RB-24 | `crates/server/src/routes/cli_types.rs` | Delete the 2 placeholder structs `CliInstallHistory`/`CliDetectionCache` (39-67); un-comment/import the REAL db models `use db::models::cli_install_history::{CliInstallHistory, CliDetectionCache};` (replace 37). Handler signatures keep returning `Err(NotImplemented)` (501 self-test contract holds). KEEP routes + `InstallOutputLine`/`InstallWsParams`/`PaginationParams`. | Low |
| RB-44 | `crates/db/migrations/` (NEW forward migration) | Create `20260418000000_cascade_workflow_command_preset_fk.sql` rebuilding `workflow_command` with `preset_id ... REFERENCES slash_command_preset(id) ON DELETE CASCADE` (SQLite table-rebuild per established precedent `20260417020002_set_null_git_event_terminal_fk.sql`). Preserve all columns + `workflow_id` CASCADE + `UNIQUE(workflow_id,order_index)` + index. Update the `[G15-010]` TODO at `workflow.rs:297-300`. `preset_id` is NOT NULL → must be CASCADE (not SET NULL). | Medium |
| RB-59 | `crates/services/src/services/merge_coordinator.rs` (+2 call lines) | **Additive prune (keep registry).** Add `pub fn prune_workflow_merge_lock(workflow_id)` that removes the map entry only if `Arc::strong_count==1`; call at auto-merge terminal point (`agent.rs:8996`) and manual-merge completion (`workflows.rs merge_workflow`). Add a unit test. | Low |
| FE-69 | `frontend/src/components/ui-new/views/GitPanel.tsx:19` + `GitPanelContainer.tsx` | **REROUTED — field IS read** at `GitPanelContainer.tsx:185`. Remove `remoteCommitsAhead?: number` from the RepoInfo VIEW interface (GitPanel.tsx:19) but preserve behavior: source the value from backend `repoStatus.remote_commits_ahead` inside the container's first useMemo via a local intersection type (OPTION B), so `hasUnpushedCommits`/push-button visibility is unchanged. Do NOT touch the snake_case `remote_commits_ahead` serde field. | Low |
| FE-33 | `frontend/src/hooks/useTaskAttempt.ts` + `hooks/index.ts:4` | **SPLIT.** Delete the dead plain `useTaskAttempt` fn (5-11, 0 callers) + drop it from the barrel re-export. KEEP `useTaskAttemptWithSession` (LIVE: TaskPanel.tsx:34, GitActionsDialog.tsx:103) — it is NOT a duplicate of `useAttempt`. | Low |
| FE-34 | `frontend/src/hooks/useAttemptBranch.ts` | Delete the redundant hook; repoint its 1 caller `TaskFollowUpSection.tsx` (L51/293-294) to `useAttempt(workspaceId)` deriving `attemptData?.branch ?? null`; remove the orphaned `['attemptBranch']` invalidation at `useRenameBranch.ts:46-48`. **NOTE:** TaskFollowUpSection is itself deleted by FE-35 — if FE-35 lands first, FE-34 is moot (verify order). | Low |
| FE-50 | `frontend/src/components/ui-new/primitives/conversation/DiffViewCard.tsx` etc. | Consolidate 3 diff-stat impls: extract shared `parseDiffStats` util (KEEP try/catch fallback) used by `NewDisplayConversationEntry.tsx` + `DiffViewCard.processUnifiedDiff`; extract shared `DiffStats` primitive; DELETE the unused `DiffViewCard` React component (164-244) + its inline DiffStats + drop from barrel (this overlaps FE-13 — coordinate). KEEP `DiffViewCardWithComments.useDiffData` SEPARATE (intentional unified-omission). | Medium |
| FE-23 | `frontend/src/components/ui-new/primitives/conversation/ChatAssistantMessage.tsx` | Inline the 1:1 passthrough into its sole caller `NewDisplayConversationEntry.tsx:570` (`<ChatMarkdown .../>`); add ChatMarkdown to imports, drop ChatAssistantMessage import (34) + barrel export (index.ts:8); delete the file. | Low |
| FE-22 | CL-SHARED dead subgraph | DELETE-as-refactor the self-contained dead shared-tasks cluster (Inv#1 atomic set): `useProjectTasks.ts`, `useAutoLinkSharedTasks.ts`, `useAssigneeUserName.ts`, `lib/electric/{sharedTasksCollection,config}.ts`, `lib/remoteApi.ts` (getSharedTaskAssignees+REMOTE_API_URL only — distinct from lib/api.ts makeRequest), `tasks/{TaskCard,TaskCard.test,TaskCardHeader}.tsx`, `ui/actions-dropdown.tsx`, `dialogs/tasks/{ReassignDialog,StopShareTaskDialog}.tsx`; EDIT `dialogs/index.ts` / `types/modals.ts` / `types/modal-args.d.ts` to drop dead refs; remove `@tanstack/react-db` + `@tanstack/electric-db-collection` from package.json. Do NOT relocate `SharedTaskRecord`. Keep `board/TaskCard.tsx` (live), `tasksApi.share/unshare`, shared/types `SharedTask*`. ShareDialog removal gated on FE-04/FE-05. **Large blast radius — atomic, tsc-gated.** | Medium-High |
| FE-65 | `frontend/src/lib/openTaskForm.ts` | Inline the 3-line wrapper into 5 call sites (Navbar:116, actions-dropdown:60/66/122, ViewRelatedTasksDialog:94) as `TaskFormDialog.show(...)`, then delete the file. Cycle-free (verified). | Low |
| FE-67 | `frontend/src/contexts/TabNavigationContext.tsx` | Delete the dead `TabNavigationProviderProps` (11-14), `TabNavigationProvider` (16-20), `useTabNavigation` (22-28), and unused `ReactNode` import. KEEP `TabNavContext` + `TabNavContextType` (LIVE via PendingApprovalEntry.tsx:24,203). | Low |
| FE-87 | `frontend/src/components/ui/card.tsx` | Delete dead `CardDescription` (44-54) + `CardFooter` (64-74) + their exports (79,81). KEEP Card/CardHeader/CardTitle/CardContent. KEEP `toggle-group.tsx` + `multi-file-search-textarea.tsx` (both LIVE). Do NOT touch ui-new/primitives/Card.tsx. | Low |
| FE-59 | pipeline components | (A) Delete dead `tokensUsed` prop from `OrchestratorHeader.tsx` (7,25,10-23 helper,35-38 block) + its test cases + optional locale `tokensUsedLabel`. (B) FIX i18n in `TerminalDetailPanel.tsx:17-18` (hardcoded Status:/Model: → `t('pipeline.orchestrator.statusLabel'/'modelLabel')`). (C) MergeTerminalNode: NO CHANGE (intentional E10-09 deferral). | Low |
| FE-60 | `frontend/src/components/board/StatusBar.tsx:45` | Delete the inert `{t('statusBar.tokensNA')}` span (no data binding, no source). Remove `statusBar.tokensNA` from en/common.json:446 + zh-Hans/common.json:433. KEEP StatusBar (live). | Low |
| FE-61 | `frontend/src/components/workflow/steps/Step4Terminals.tsx` | Delete dead legacy CLI shim (`isLegacyCliDetectMap` 57-62, `LEGACY_CLI_DISPLAY_NAMES` 64-74, `LEGACY_CLI_ID_ALIASES` 76-86, else-if branch 237-248); **rewrite the 9 legacy-map mocks in Step4Terminals.test.tsx to canonical `CliDetectResponse[]` array shape** (backend always returns array; frontend+backend ship lockstep via rust-embed). | Medium (test churn) |
| FE-38 | `frontend/src/components/workflow/hooks/{useWizardNavigation,useWizardValidation}.ts` | Delete dead members: `useWizardNavigation` isStepValid option (9-16,36) + `goToStep` (27,84-112,122); `useWizardValidation` `hasErrors` (8,33,37) + unused useMemo import. Delete the 2 orphaned test blocks. KEEP wizardStore.ts `hasErrors()` (separate). | Low |
| FE-39 | `frontend/src/components/workflow/{steps/Step1Basic.tsx,types.ts}` | Delete the dead no-op `importFromKanban` toggle (Step1Basic 129-153) + `types.ts` `importFromKanban` (108) / `kanbanTaskIds` (109) / default (227); remove `importFromKanban:false` from all 5 test fixtures (tsc breaks otherwise); optional i18n key cleanup. | Low |
| RB-62/RB-63/RB-50 | (CL-IDE) | Coordinated-cluster removals — operations are §2 Steps 1-6. NOT standalone. | — |

### 4B. REFACTORS — barrel/keep-or-repoint

| id | file | operation |
|----|------|-----------|
| FE-18 (rjsf) | `frontend/src/components/rjsf/index.ts` | KEEP, OR repoint sole consumer `ExecutorConfigForm.tsx:11` to `'./rjsf/theme'` then delete the barrel. (Optional; not required.) |

---

## 5. PRECONDITIONS & ORDERING (consolidated)

- **P-rust-1:** Baseline `cargo check --workspace` green; per-cluster `cargo check -p <crate>` along `db→services→server`, `quality→server`.
- **P-rust-2:** RB-13 single atomic commit (fn + `use` llm.rs:15 + tests llm.rs ~1155-1167 + url.rs ~163-247).
- **P-rust-3:** `cc_switch.rs` single coherent pass (RB-17 delete + RB-26 + RB-37 + RB-56 decision). `prompt_handler.rs` single pass (RB-06+RB-07+RB-08+RB-47). `agent.rs` orchestrator single pass (RB-35+RB-36+RB-08 reader+RB-48). `condition.rs` quality single pass (RQ-02+RQ-03+RQ-17). `sonar.rs` single pass (RQ-07 refactor + RQ-13 delete). `quality/gate/mod.rs` single pass (RQ-01+RQ-16 branch+RQ-22). `issue.rs` single pass (RQ-23+RQ-24).
- **P-rust-4:** D6 strict ordering per §2 (handlers RB-61/62/63 → type pair RB-65 → generate_types 128-129 + RB-41 → regen → commit types.ts).
- **P-rust-5 (NEW):** RB-25 — verify `WorkflowTaskDetailResponse` (71-77) is PRESERVED before/after deleting the two dead siblings; `cargo check -p server`. No regen (not TS-derived).
- **P-rust-6 (NEW):** RB-15 — after deleting jwt.rs + lib.rs:23 + Cargo.toml:26, `cargo build -p utils` to confirm `jsonwebtoken` is genuinely orphaned.
- **P-rust-7 (NEW):** RQ-16 cascade — if removing the pub Warn enum variants, do it as ONE coordinated edit across gate/result.rs, gate/mod.rs, status.rs, report.rs, agent.rs; else steps 1-2 only (variants remain).
- **P-rust-8 (NEW):** RB-44 — NEW forward migration only; never edit applied migrations; filename must sort after `20260417020002...`; `cargo sqlx migrate run` + tests after.
- **P-fe-1:** Baseline `tsc`+`vitest`+`eslint` green; re-run after each FE cluster.
- **P-fe-2:** Re-grep every FE delete target immediately before removal.
- **O1 (FE-78):** inline `<Navigate>` into `App.tsx` route before deleting `WorkspacesLanding.tsx`.
- **O2 (T05 / FE-79):** **REVISED** — KEEP `pages/__tests__/WorkflowDebugPage.test.tsx` (inner, more comprehensive, all tests pass). DELETE the OUTER `pages/WorkflowDebugPage.test.tsx` (T05). Do NOT delete both. FE-79 = keep.
- **O3 (FE-17/18):** delete `stores/index.ts` whole (0 consumers) + the two store files, SAME commit.
- **O4 (FE-02):** confirm `Workflows.tsx` imports `WorkflowWizard` from `@/components/workflow` before deleting `components/wizard/`.
- **O5 (FE-69):** REVISED to refactor (field is read) — source value in container, preserve push-button behavior.
- **O6 (FE-37/R9):** 3-file atomic edit (QualityBadge + PipelineView:75 + terminal/TerminalDebugView:746). No longer gated on FE-06.
- **O7 (NEW, FE-35/FE-81/FE-34):** delete the orphaned follow-up tree (TaskAttemptPanel + TaskFollowUpSection + useFollowUpSend) as ONE change; this subsumes FE-81 inline-queue redundancy; if it lands before FE-34, FE-34 is moot (TaskFollowUpSection gone).
- **O8 (NEW, FE-22):** atomic CL-SHARED subgraph delete, tsc-gated; ShareDialog removal gated on FE-04/FE-05 outcome.
- **O9 (NEW, T06/FE-05):** delete the `tasks/{TaskCard,TaskCard.test,TaskCardHeader}` set as one unit; unlocks FE-51 (`ui/shadcn-io/kanban.tsx`, separate candidate).

---

## 6. KEEP / DEFERRED (investigate resolved to keep; high-blast-radius retained)

### 6A. Investigate RESOLVED to KEEP (do NOT delete; reclassify ledger)

| id | reason |
|----|--------|
| RB-D18 | `crates/runner` (gRPC) + `bin/mcp_task_server.rs` LIVE — packaging now VERIFIED (Dockerfile.runner + docker-compose.split.yml + npx `--mcp` dispatch). KEEP all wiring. |
| RB-45 | Both encrypt-key migrations are APPLIED (never edit); `orchestrator_api_key` holds ciphertext (no plaintext leak); `_encrypted` column asserted by `lib.rs:40` verify_schema. KEEP. |
| RB-55 | `auto_prepare` 2s sleep guards Claude-Code subprocess boot on the live AuditPlan path; no readiness primitive exists. KEEP. |
| RB-D01 | DIY quiet-window monitor (workflows.rs:2107-2287) is the ONLY auto-completion path for DIY workflows. KEEP (tune threshold separately, do not delete). |
| RB-D02 | AuditPlan upload pdf/docx — LIVE feature; trim accepted formats to text-only is a REFACTOR option, not delete (graceful degrade exists). Deferred to product. |
| RB-D03 | concierge `looks_incomplete` — LIVE local binding, bounded retry. KEEP. |
| RB-D04 | concierge `audit_plan: None` — mandatory struct field, graceful fallback. KEEP. |
| RB-D05 | claude `claude-code-router` proxy — serde-persisted + TS-generated + user-settable flag. KEEP. |
| RB-D06 | copilot `watch_session_id` — the only session-id source for Copilot --resume. KEEP. |
| RB-D07 | cursor MCP auto-trust — LIVE on every cursor spawn. KEEP. |
| RB-D09 | droid `ToolResult` empty-pop — LIVE completion path; optional warn-log hardening only, NOT delete/rewrite. KEEP. |
| RB-D10 | self-heal re-prepare — NOT recursion (false premise); live restart-recovery. KEEP. |
| RB-D11 | `ReviewCode` handler + whitelist — config-gated reachable + security boundary. KEEP. |
| RB-D12 | `skip_quiet_window` + `PendingGuard` Drop — LIVE post-gate completion path. KEEP. |
| RB-D15 | non-Windows `pick_folder` stub — only non-Windows definition of a symbol the router references unconditionally; deleting breaks Linux/macOS/Docker build. KEEP. |
| RB-D16 | `ci_webhook` — LIVE HMAC-authenticated endpoint driven by ci-notify.yml. KEEP. |
| RB-D17 | `CHAT_REPLAY_CACHE` — LIVE webhook replay-attack control. KEEP. |
| RQ-14 | Whole Sonar provider — default-ON (serde default_true + bundled policy `sonar: true`); live repo_gate conditions. KEEP (RQ-07/RQ-13 micro-fixes still apply inside it). |
| RQ-18 | `MetricKey` Bugs/CodeSmells/Vulnerabilities/etc. — strict-config-parse blast radius (downstream quality-gate.yaml could reference these keys); from_yaml has no `#[serde(other)]`. KEEP. |
| RQ-20 | `default_config` — crash-safety fallback + 5 test callers. KEEP. |
| FE-08 | `vscode/bridge.ts` iframe surface — LIVE clipboard helpers + cannot disprove out-of-repo webview host. KEEP (delete only ContextMenu, FE-07). |
| FE-09 | `useOpenInEditor`/`useOpenProjectInEditor`/`useEditorAvailability` — LIVE; removal is the coordinated CL-IDE/G1 program (§2), not a standalone FE-09 delete. (useEditorAvailability survives until RB-64.) |
| FE-10 | api.ts openEditor×3 + checkEditorAvailability — LIVE; CL-IDE lockstep (§2). |
| FE-11 | `ide/{IdeIcon,OpenInIdeButton}` — LIVE 6 consumers; CL-IDE lockstep (§2). |
| FE-26 | `Actions.OpenInIDE` — LIVE; CL-IDE lockstep (§2). |
| FE-28 | NextActionCard IDE button — LIVE; CL-IDE lockstep (§2). |
| FE-30 | ProjectEditorSelectionDialog + EditorAvailabilityIndicator — LIVE; CL-IDE lockstep; EditorAvailabilityIndicator gated on RB-64. |
| FE-63 | useModelVerification — LIVE end-to-end (SetupWizardStep2ModelContainer). KEEP (optional stale-default refresh only). |
| FE-68 | `ProcessSelectionContext` — LIVE via NextActionCard→ViewProcessesDialog→ProcessesTab from ui-new. KEEP. |
| FE-73 | `WorkflowProgressPanel` — LIVE in ConciergeChatView; different prop contract from WorkflowProgressView (not a dup). KEEP. |
| FE-76 | `LegacyDesignScope` — LIVE (backs `/commands` route + styles + portal/toast/modal providers). KEEP until /commands migrated. |
| FE-77 | `SlashCommands.tsx` — LIVE routed page; ui-new port is a separate gated CL-OLDUI task. KEEP. |
| FE-82 | legacy `RebaseDialog.tsx` — LIVE via GitActionsDialog reachable from ui-new conversation UI. KEEP. |
| FE-84 | PrCommentsDialog local `getErrorMessage` — LIVE, intentional i18n+error_data variant (no real shadow). KEEP. |
| FE-90 | SearchBar `if (disabled) return null` — intentional conditional render. KEEP. |
| T17 | `phase18_scenarios.rs` — LIVE compiling integration test with unique orchestrator-runtime/recovery coverage. KEEP (optional rename to drop phase18_ prefix). |
| T19 | orchestrator `tests.rs` quality_gate_mode "off" — load-bearing test setup, not dead. KEEP (coverage gap is additive). |
| FE-79 | inner `pages/__tests__/WorkflowDebugPage.test.tsx` — KEEP (more comprehensive; FE-79 deletion premise refuted). Delete the OUTER file via T05 instead. |

### 6B. Investigate RESOLVED to REFACTOR (in §4A): RB-23, RB-20, RB-24, RB-44, RB-59, FE-69, FE-33, FE-34, FE-50, FE-23, FE-22, FE-65, FE-67, FE-87, FE-59, FE-60, FE-61, FE-38, FE-39, FE-18(rjsf), FE-37(R9).

### 6C. KEEP-as-refactor target inside a kept feature
- T18 `phase18_git_watcher.rs` — RENAME (not delete) to `git_watcher_status_e2e_test.rs` or merge its 3 unique tests (failed-status E2E, review_pass/review_reject parse) into `git_watcher_integration_test.rs` then delete. Do NOT plain-delete (unique coverage). Optional W2-02 fix: await aborted watcher handles.

---

## 7. STILL fast-context-UNAVAILABLE / residual high-blast (verify manually before cutting)

`mcp__fast-context__fast_context_search` remained unavailable for these IDs; the panel's verdicts rest on exhaustive ripgrep. Re-run a semantic pass if `WINDSURF_API_KEY` is restored before execution; otherwise the grep verdicts stand but re-grep importers immediately before cutting.

| id | disposition | residual risk to clear |
|----|-------------|------------------------|
| RB-D18 | KEEP | (resolved KEEP — no cut) |
| RB-14 | DELETE | possible out-of-tree consumers of the 6 oauth Handoff*/TokenRefresh* structs; grep confirmed only defs + stale census doc. Re-grep across all extensions before cutting. |
| RQ-13 | DELETE | `import_sarif_results` is `pub` on the `quality` crate (workspace-internal, unpublished → no realistic out-of-tree consumer). Re-grep before cutting. |
| RQ-14 | KEEP | (resolved KEEP) |
| RQ-24 | DELETE (SPLIT) | `location_string` `pub` on internal crate; KEEP `one_line_summary`. Re-grep `location_string` (only def expected). |
| FE-08 | KEEP | (resolved KEEP) |
| FE-47 | DELETE | cannot disprove out-of-repo iframe host using `VITE_PARENT_ORIGIN`; but sole in-repo importer (`FullAttemptLogs.tsx`) is already deleted and the env var is undefined everywhere → inert. Re-grep `StyleOverride`/`AppWithStyleOverride`/`VITE_PARENT_ORIGIN` before cutting. |
| FE-12 | DELETE | needs lazy/dynamic-import reachability check; grep confirmed no `import()`/`React.lazy` referencing the 4 preview files. Re-grep before cutting. |
| FE-80 | DELETE | type-only exports, compile-erased; grep confirmed zero consumers + serialization uses a different inline shape. Re-grep before cutting. |

---

## 8. SUMMARY OF CHANGES vs PRIOR PLAN

**Reroutes (prior confirmed-delete → not delete):**
- **RB-23** confirmed-delete → REFACTOR. Refuted: `unlink_project` is real DB code sharing the `/link` route; `link_project_to_existing_remote`←LinkProjectDialog.tsx:249 + OrganizationSettingsNew.tsx:626; `create_and_link_remote_project`←LinkProjectDialog.tsx:262; `get_project_remote_members`←useProjectRemoteMembers.ts:11←ReassignDialog.tsx:62; error-string contract consumed by organizationRemoteCapability.ts:23. Only `get_remote_project_by_id` deletable.
- **FE-18 (rjsf barrel)** confirmed-delete → KEEP/repoint. Refuted: `ExecutorConfigForm.tsx:11 import { shadcnTheme } from './rjsf'` (used 132-134).
- **FE-37 (R9)** simple-delete → 3-file atomic refactor. Refuted: `totalIssues` passed by PipelineView.tsx:75 + terminal/TerminalDebugView.tsx:746 (TS2322 risk).
- **FE-69** confirmed-delete → REFACTOR. Refuted: field read at GitPanelContainer.tsx:185 (drives push-button).
- **FE-79** prior O2 "delete duplicate" → KEEP inner; redirected to T05 deleting the outer file.

**Promotions (prior DEFERRED investigate → DELETE):** RQ-26, RB-06, RB-07, RB-08, RB-11, RB-14, RB-15, RB-25, RB-28, RB-29b, RB-30, RB-31b, RB-32, RB-33, RB-34, RB-35, RB-36, RB-47, RQ-13, RQ-16, RQ-17, RQ-22, RQ-23, RQ-24, RQ-25, FE-40, FE-47, FE-74, FE-80, T06, T12, FE-35/FE-81 (tree).

**Promotions (prior DEFERRED investigate → REFACTOR):** RB-20, RB-23, RB-24, RB-44, RB-59, FE-22, FE-23, FE-33, FE-34, FE-38, FE-39, FE-50, FE-59, FE-60, FE-61, FE-65, FE-67, FE-69, FE-87.

**Resolved KEEP:** §6A list (RB-D18, RB-45, RB-55, RB-D01..D17 subset, RQ-14, RQ-18, RQ-20, FE-08/09/10/11/26/28/30/63/68/73/76/77/82/84/90, T17, T19, FE-79).

*End of FINAL plan. Direct input to P4 implementation.*
