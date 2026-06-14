# P2 Master Candidate Ledger — SoloDawn Streamline Census

Date: 2026-06-14
Branch: `refactor/streamline-quality-gates`
Synthesizer: Master Synthesis (Opus)
Inputs: `ledger-rust-backend.md` (82), `ledger-rust-quality.md` (26), `ledger-frontend.md` (92), `ledger-tests.md` (21)
Authoritative cross-area reference: `docs/audit/R1-ide-editor-connection-deletion-audit.md`

> **Purpose.** This is the single authoritative input to **Phase 2 (adversarial keep/cut verification)**.
> Each row is a deduped candidate. Cross-area clusters are resolved into `CL-*` cluster anchors so the
> IDE-connect deletion (frontend + backend + i18n + config) and the webview-bridge decision are unambiguous.

---

## Summary counts

| Disposition | Count | Notes |
|---|---|---|
| **delete** | 71 | Verified-dead / stub / duplicate / deprecated with zero live callers. |
| **refactor** | 49 | Live code with a bug, duplication, or perf/leak fix; do NOT delete. |
| **investigate** | 60 | Needs a keep/cut decision in Phase 2; often "unwired but possibly intended". |
| **keep-but-noted** | 21 | Confirmed intentional (reserved variants, forward-compat shims, test seams, security gates). |
| **TOTAL** | **201** | 221 raw candidates − 20 collapsed into cross-area clusters. |

By source area: rust-backend 82 · rust-quality 26 · frontend 92 · tests 21.

### Cross-area clusters resolved

| Cluster | Members | Resolution |
|---|---|---|
| **CL-IDE** (open-in-external-IDE) | RB-60→66, RB-50; FE-09,10,11,14,25,26,27,28,29,30,31; i18n; config | **DELETE the feature, lockstep FE+BE+i18n+generate_types.** KEEP `EditorConfig`/`EditorType` config types (RB-66) + do a v10 migration or leave the `config.editor` field inert (R1 §9.3). Strip IDE buttons from live diff/git components (do NOT delete the components). `check_editor_availability` (RB-64) + `EditorAvailabilityIndicator`/`useEditorAvailability` (FE-30) die WITH the cluster, not standalone. |
| **CL-VSCODE** (webview bridge) | RB n/a; FE-07, FE-08, FE-47 | **SPLIT.** `vscode/ContextMenu.tsx` (FE-07) = orphan → **delete**. `vscode/bridge.ts` (FE-08) = clipboard helpers LIVE via `wysiwyg.tsx` (~10 chat boxes) → **KEEP/refactor**, out of scope for IDE-connect (R1 §4, §9.2). `StyleOverride.tsx` (FE-47) iframe-host embed → **investigate** separately. |
| **CL-LEGACYPANELS** (old task-detail UI) | FE-03,05,12,51,68,82,87; FE-81 | Atomic legacy-UI cluster; cut as group once legacy task-detail route retired. |
| **CL-CONVDUP** (conversation dup) | FE-13,23,24,35,50,75 | DiffViewCard/useDiffData/ScriptToolCard duplications; refactor, keep one owner. |
| **CL-WIZARDDUP** | FE-02,17 (wizardStore/workflowStore) | components/wizard dir + dead stores → delete; live wizard in components/workflow untouched. |
| **CL-OLDUI** | FE-15,74,76,77 | Old-design escape hatches; gated on `/commands` migration + old-route removal. |
| **CL-I18N** | FE-31,32,53,58,59; CL-IDE i18n | i18n coverage gaps + IDE-key removal; mirror across 6 locales. |
| **CL-SHARED** (remote shared-tasks) | FE-22; RB-K01 (REMOTE_FEATURES_ENABLED) | Flag-gated dead-when-false; keep flag shim, investigate FE branch. |
| **CL-REMOTE** (remote org/project/runner) | RB-22,23,28,24; RB-D18 | All return 501/Err; remove stub routes. **runner/mcp_task_server packaging UNVERIFIED → investigate.** |
| **CL-DUP-LOC** (test file dup) | T04,T05; FE-79 | Same test in and out of `__tests__/`; merge unique cases then delete outer. |
| **CL-STUB** (no-assertion tests) | T01,T09,T10,T11 | Always-green placeholder tests. |

---

## Top risks (Phase-2 attention)

1. **CL-IDE lockstep.** `shared/types.ts` is GENERATED (`generate_types.rs`). Backend deletions + `generate_types.rs` edits MUST be followed by regeneration or FE type imports go stale and CI `--check` fails. `'ide-icon'` special-icon feeds the **ContextBar of BOTH workspaces** (R1 §9.1) — removing `Actions.OpenInIDE` requires removing the `ide-icon` branch in `CommandBar`+`ContextBar` or the bars break.
2. **`config.editor` is a persisted, versioned schema field** (v9 + migration chain v1→v9). Removing it is a schema change — prefer a v10 migration or leave the field inert (RB-66 KEEP). Blind delete corrupts on-disk config deserialization.
3. **`vscode/bridge.ts` clipboard coupling** — do NOT delete without re-homing `writeClipboardViaBridge`/`readClipboardViaBridge`; affects every chat box in both workspaces (R1 §9.2).
4. **`runner` gRPC binary + `mcp_task_server.rs` packaging (RB-D18)** — npm wrapper may shell into the mcp bin; `RunnerClientImpl` referenced by Deployment trait. **Confirm packaging before ANY cut.** HIGH blast radius if wrong.
5. **sqlx migration checksums (RB-K10, RB-45)** — deleting/altering applied migrations corrupts existing DBs. Plaintext-key DROP (RB-45) needs a NEW forward migration, never edit-in-place.
6. **Secret residue / disk leak (RB-37)** — API keys in `/tmp` `claude_home/settings.json` + `codex_home/auth.json` never cleaned. P0 security fix, not a delete.
7. **AuditPlan legacy fallback (RB-K03)** — binary acceptance review superseded by scoring (commit 11ac066d2) but still the fallback when `audit_plan.raw_principles` empty. Cannot cut until all workflows carry `raw_principles`.
8. **Serde-persisted enums (RQ-18, RQ-21)** — `MetricKey` variants + `MeasureValue::None` persisted in `quality_run` decision JSON; removing breaks deserialization of historical rows. Audit DB before cut.

### fast-context UNAVAILABLE (lower verification confidence — Phase-2 must re-verify)

These were flagged by source agents as relying on static grep only because semantic search was down:
- **FE-80** — wysiwyg image/pr-comment node type exports: dynamic/cross-repo import unconfirmed.
- **FE-08 / FE-47 / CL-VSCODE** — cannot disprove an out-of-repo webview/iframe host consumer.
- **RB-25** — `WorkflowDetailResponse` family: zero external/integration-test consumer unconfirmed.
- **RB-K02** — RedisBus reachability: confirm no deploy script sets `SOLODAWN_MESSAGE_BUS=redis`.
- **RB-D18 / CL-REMOTE** — runner & mcp_task_server packaging path unverified.
- **RQ-13/14, RQ-23/24** — Sonar provider + `issue.rs` public formatters: possible out-of-tree crate consumer.

---

## DELETE — verified dead / stub / duplicate / deprecated (zero live callers)

| Cluster-ID | Files | Kind | Conf | Evidence | Blast radius |
|---|---|---|---|---|---|
| RB-01 | `crates/services/.../orchestrator/runtime_test.rs:1-605` | dead | high | No `mod runtime_test;`; never compiled; omits `audit_plan` so wouldn't compile. | None. |
| RB-02 | `crates/services/test_edge_cases.rs:1-46` | dead | high | Not in Cargo.toml; never compiled. | None. |
| RB-03 | `crates/services/.../services/share.rs:1-51` | dead | high | Not in services mod.rs; submodules with no files; never compiled. | None. |
| RB-04 | `crates/db/benches/workflow_bench.rs:231-342` (`_unused_keep_old_find_by_id_setup`) | dead | high | `#[allow(dead_code)]`, not in `criterion_group!`, zero callers. | None. |
| RB-05 | `crates/db/.../execution_process.rs:101-108` (`UpdateExecutionProcess`) | dead | high | `#[allow(dead_code)]`; only its own def; update path is `update_completion`. | None. |
| RB-06 | `crates/services/.../prompt_handler.rs:174-204,688-740` (`LLMPromptDecisionRequest/Response`, `build_llm_decision_prompt`) | dead | high | Only own test L1429; prod uses inline `format!` L507-513. | ~75 lines + 1 test. No TS export. |
| RB-07 | `crates/services/.../prompt_handler.rs:579-585` (`reset_terminal_state`) | dead | high | Zero callers; resets call `sm.reset()` directly. | None. |
| RB-09 | `crates/executors/.../droid/normalize_logs.rs:789` (`EditToolResult`) | dead | high | Empty struct, zero usages. | None. |
| RB-10 | `crates/server/.../event_bridge.rs:85` (`SharedEventBridge`) | dead | high | `pub type`, zero uses; main.rs spawns `EventBridge` directly. | 1 line. |
| RB-12 | `crates/utils/src/path.rs:127-131` (`get_gitcortex_temp_dir`) | deprecated | high | `#[deprecated]`; 0 external callers; alias to `get_solodawn_temp_dir`. | None. |
| RB-13 | `crates/utils/src/url.rs:85-106` (`normalize_base_url`) | deprecated | high | `#[deprecated]`; only used in own cfg(test) via llm.rs allow(deprecated). | fn + llm.rs import L14-15 + url.rs test 1153-1169. |
| RB-16 | `crates/services/.../chat_connector.rs:38-137` (`TelegramConnector`) | dead | high | Never imported; `from_env` uncalled; DB binds 'telegram'. | This file only; trait + FeishuConnector unaffected. |
| RB-17 | `crates/services/.../cc_switch.rs:596-678,1247-1253,462-465` (`switch_for_terminal(s)` + `CCSwitch` trait) | deprecated | high | `#[deprecated]`; only test caller; superseded by `build_launch_config`. | trait+impl+re-export; breaks 1 test. |
| RB-18 | `crates/services/.../git.rs:1082-1108,236-253` (`get_commit_subject`, `ahead_behind_commits_by_oid`, `ensure_main_branch_exists`) | dead | medium | Only defs repo-wide; no callers. | Zero callers. |
| RB-19 | `crates/services/.../merge_coordinator.rs:322-328` (`test_merge_coordinator_creation`) | stub | high | Placeholder test, no assertions. | None. |
| RB-21 | `crates/server/.../oauth.rs:64-128` (`handoff_init/complete`, `get_token`, `get_current_user`) | stub | high | 4/6 handlers permanently return BadRequest; DTOs `allow(dead_code)`. KEEP `status()`+`logout()`. | FE handoff/token/user callers already always fail. |
| RB-22 | `crates/server/.../organizations.rs:1-188` | stub | high | Every handler returns 501; no FE callers; 15 dead routes. | Remove `.merge(...)` + `pub mod`. (CL-REMOTE) |
| RB-23 | `crates/server/.../projects.rs:158-207` (link/create-and-link/get-remote-*) | stub | high | All return 501; FE shows error states. | Routes removed; FE handles errors. (CL-REMOTE) |
| RB-26 | `crates/cc-switch/src/{switcher.rs:83-94,config_path.rs:35-37/85-87}` | dead | medium | All pub, zero callers repo-wide. | API cleanup; nothing outside monorepo consumes. |
| RB-29 | `crates/services/.../persistence.rs:303-324` (`restore_conversation_history`) | dead | high | Zero callers; superseded by `load_state`. (`clear_state` → investigate, RB-29b) | Safe. |
| RB-31 | `crates/db/.../workspace.rs:84-93,101-107` (`CreatePrParams`, `AttemptResumeContext`) | dead | medium | Zero callers. (`CreateFollowUpAttempt` → investigate, has `#[derive(TS)]`) | Safe. |
| RB-50 | `crates/services/.../{repo.rs:55-98,projects.rs:421-464,task_attempts.rs:809-811}` (3-way path-helper dup) | duplicate | high | Byte-for-byte security-sensitive path helpers; vanish with CL-IDE deletion. | If CL-IDE proceeds all copies vanish. (CL-IDE) |
| RB-61 | `crates/server/.../task_attempts.rs:793-807,809-888,890-946,965-1044,2143` (`open_task_attempt_in_editor`+helpers+test+route) | deprecated | high | R1 §5 G1 target. PRESERVE adjacent `status_semantics_tests` 949-963. | FE + ApiError::EditorOpen + generate_types. (CL-IDE) |
| RB-62 | `crates/server/.../projects.rs:393-464,466-536,765-840` (`open_project_in_editor`+helpers+test) | dead | high | R1 lists `/projects/{id}/open-editor`. | FE project editor surface. (CL-IDE) |
| RB-63 | `crates/server/.../repo.rs:215-278,55-132` (`open_repo_in_editor`+helpers+test) | dead | high | R1 lists `/repos/{id}/open-editor`; helpers dupe projects.rs. | FE repoApi.openEditor. (CL-IDE) |
| RB-65 | `crates/server/.../projects.rs:378-391` (`OpenEditorRequest/Response`) | dead | high | Shared by projects.rs + repo.rs; **delete LAST** (types must outlive both handlers). | generate_types.rs TS emission. (CL-IDE) |
| RQ-01 | `crates/quality/src/gate/mod.rs:46-56` (`QualityGate::with_id`) | dead | high | 0 callers; prod uses `QualityGate::new`. | None. |
| RQ-02 | `crates/quality/src/gate/condition.rs:36-41` (`Operator::to_db_value`) | dead | high | 0 callers; only `from_db_value` used. | None. |
| RQ-03 | `crates/quality/src/gate/condition.rs:124-126` (`Condition::description`) | dead | high | 0 callers; evaluator builds messages inline. | None. |
| RQ-04 | `crates/server/src/self_test/runner.rs:19,131` (`TestServer::port`) | dead | high | Set, never read; WS test recovers port via base_url split. | Remove field+assignment. |
| FE-02 | `frontend/src/components/wizard/` (10 files) | duplicate | high | 0 prod imports; live wizard is components/workflow. Stub steps. | Dir + 5 tests. Live wizard untouched. (CL-WIZARDDUP) |
| FE-03 | `frontend/.../panels/{DiffsPanel,TaskAttemptPanel,TaskPanel}.tsx + DiffCard.tsx + diff/{CommentWidgetLine,ReviewCommentRenderer}.tsx + logs/VirtualizedList.tsx` | dead | high | Zero prod importers; transitive dead cluster; superseded by ui-new. | Atomic group. DiffCard has IDE button (CL-IDE). Keep DisplayConversationEntry. (CL-LEGACYPANELS) |
| FE-04 | `frontend/src/hooks/useVideoProgress.ts` | dead | high | 0 callers, not in index.ts, no test. | Zero. |
| FE-05 | `frontend/.../tasks/{TaskCard,TaskCard.test,TaskCardHeader}.tsx` | dead | high | Only own test imports; board/TaskCard is replacement. | Co-delete test; unlocks kanban delete (FE-51). (CL-LEGACYPANELS) |
| FE-06 | `frontend/.../debug/{TerminalDebugView,TerminalSidebar}.{tsx,test}` | dead | high | No prod importer; live /debug uses terminal/TerminalDebugView. | Component+test pairs. (CL-DEBUGSTUB) |
| FE-07 | `frontend/src/vscode/ContextMenu.tsx` | dead | high | `WebviewContextMenu`: only self + R1 doc. Never mounted. | None. bridge.ts unaffected. (CL-VSCODE) |
| FE-13 | `frontend/.../conversation/DiffViewCard.tsx:164-244` (full card export) | dead | high | Exported, zero render sites; superseded by DiffViewCardWithComments. | Remove card export + barrel line; KEEP DiffViewBody/useDiffData/DiffInput. (CL-CONVDUP) |
| FE-14 | `frontend/.../dialogs/tasks/EditorSelectionDialog.tsx` | deprecated | high | G1 target; 2 error-fallback callers only. | 2 catch-blocks. (CL-IDE) |
| FE-15 | `frontend/.../dialogs/global/OnboardingDialog.tsx` | dead | high | `show()` called 0×; replaced by /setup wizard. | Remove exports + ModalArgs entry. (CL-IDE,CL-OLDUI) |
| FE-16 | `frontend/.../dialogs/projects/ProjectFormDialog.tsx` | dead | high | `show()` never invoked outside file; RepoPickerDialog used directly. | 3 exports from dialogs/index.ts. |
| FE-17 | `frontend/src/stores/{wizardStore,workflowStore}.ts` | dead | high | 0 imports outside index.ts barrel; prod uses local state + RQ. | Remove re-exports. (CL-WIZARDDUP,CL-BARREL) |
| FE-18 | `frontend/src/{pages/settings,stores,rjsf}/index.ts` barrels | dead | high | 0 consumers (direct paths used everywhere). | Confirm tsconfig aliases don't resolve barrels. (CL-BARREL) |
| FE-19 | `frontend/src/lib/types.ts:12-22` (`ConversationEntryDisplayType`) | dead | high | Zero imports outside def. AttemptData stays. | None. |
| FE-20 | `frontend/src/components/ui/tabs.tsx` | dead | high | Zero imports; TabNavigationContext imports Radix directly. | Zero. |
| FE-21 | `frontend/.../ui-new/primitives/Card.tsx` | dead | high | Zero importers; settings use bespoke SettingsCard. | None. |
| FE-36 | `frontend/.../workflow/constants.ts:79-88` (`GIT_COMMIT_TYPES`) | dead | high | Exported, zero imports. | 10-line const. |
| FE-37 | `frontend/.../workflow/QualityBadge.tsx:9,11,50` (`totalIssues`, `mode` props) | dead | high | Destructured/declared, never used. | Remove props + caller pass-throughs. |
| FE-41 | `frontend/.../ui-new/utils/workflowStatus.ts:239-251` (`getWorkflowStatusMeta`,`getTerminalStatusMeta`) | dead | high | Zero callers; comments are docs not usages. | 2 unused exports. |
| FE-42 | `frontend/.../ui-new/actions/useActionVisibility.ts:137-164` (`filterVisibleActionItems`) | dead | high | Zero callers; bars filter inline via `isActionVisible`. | Zero. |
| FE-43 | `frontend/.../ui-new/hooks/useWorkspaces.ts:78-80` (`workspaceKeys`) | dead | high | Zero external imports; `workspaceSummaryKeys` is the live one. | Remove export. |
| FE-46 | `frontend/src/utils/statusLabels.ts` | dead | high | Only own def; TaskStatus labels via i18n/inline. | None. |
| FE-48 | `frontend/src/types/modal-args.d.ts` | duplicate | high | Stale ambient re-decl conflicting with modals.ts; transfer-shared-task never invoked. | No runtime effect. |
| FE-69 | `frontend/.../ui-new/views/GitPanel.tsx:20` (`RepoInfo.remoteCommitsAhead`) | dead | high | Declared+populated but never forwarded; container uses own copy L185. | Remove field; one-line container change. |
| FE-91 | `frontend/.../ui-new/containers/PlanningChatContainer.tsx + primitives/PlanningChat` | dead | high | NO prod importer; superseded by CreateChatBoxContainer. | Delete both. No unique export. |
| FE-31 | i18n: `projects.openInIDE`(6 locales), `tasks.attempt.actions.{openInIde,openMenu,stopDevServer}`, onboarding/settings editor keys | dead | high | Zero `t()` callers (only `startDevServer` live). | Prune from 6 locale files. (CL-IDE,CL-I18N) |
| T01 | `crates/server/tests/cli_detection_test.rs:1-38` | stub | high | Both tests TODO-only bodies; false green. | 2 no-op tests. (CL-STUB) |
| T04 | `frontend/src/hooks/useQualityGate.test.tsx:1-268` | duplicate | high | Dup of `__tests__/useQualityGate.test.ts`; double-runs. | **MERGE** qualityKeys + happy-path into canonical first. (CL-DUP-LOC,CL-QG) |
| T05 | `frontend/src/pages/WorkflowDebugPage.test.tsx:1-102` | duplicate | high | Dup of `pages/__tests__/...`; inner subsumes. | Verify outer's unique case present in inner. (CL-DUP-LOC) |
| FE-79 | `frontend/src/pages/__tests__/WorkflowDebugPage.test.tsx` | duplicate | high | WRONG route `/workflow/:id/debug` vs actual `/debug/:id`; never matches → false confidence. | Only this test lost. (CL-DEBUGSTUB,CL-DUP-LOC) |
| T07 | `crates/services/.../orchestrator/tests.rs:678-693` (`test_instruction_parsing`) | duplicate | high | Round-trips only SendToTerminal; subsumed by L28 + L695. | One test fn. (C-ENUM-SERDE) |
| T02 | `crates/server/tests/slash_commands_pool_test.rs:1-42` | redundant | medium | Only checks pool accessible; same pattern in 2 other tests. | No unique coverage. |
| T20 | `frontend/src/test/canvas-mock.test.ts:1-11` | redundant | medium | Meta-test of setup.ts mock; tests test-infra. | Zero prod coverage. |
| FE-78 | `frontend/src/pages/ui-new/WorkspacesLanding.tsx` | redundant | low | 5-line `<Navigate>` only; inline-able into App.tsx. | One-line App.tsx change. |
| FE-86 | `frontend/.../ui-new/primitives/Field.tsx` (FieldSet/Legend/Content/Title/Description/Separator) | dead | medium | 6 sub-components zero external callers (only Field/FieldLabel/FieldError used). | Field safe; Dropdown subs → investigate (future submenus). |

---

## REFACTOR — live code with bug / dup / perf / leak fix (do NOT delete)

| Cluster-ID | Files | Kind | Conf | Evidence | Blast radius |
|---|---|---|---|---|---|
| **RB-37** | `crates/services/.../cc_switch.rs:800-808` (CLAUDE_HOME/CODEX_HOME cleanup gap) | bug | high | **SECURITY**: API keys in `/tmp` settings.json/auth.json never cleaned → disk leak + secret residue. | Add cleanup to `finalize_terminated_process()` or TempDirGuard. |
| **RB-38** | `crates/services/.../error_handler.rs:150-158` (`activate_error_terminal`) | bug | high | Uses `find_all().first()` ignoring workflow `error_terminal_cli_id/model_id`; multi-CLI silently wrong CLI. | ~20 lines (creation branch). |
| **RB-39** | `crates/server/.../terminals.rs:119-121` (`STARTABLE_TERMINAL_STATUSES` includes 'working') | bug | high | Route guard allows POST /start on 'working' → 2nd PTY spawn → ProcessManager corruption. | Remove 'working'; resolve 3-way constant collision. |
| RB-40 | `crates/server/.../workflows.rs:3619-3632` (`get_workflow_events`) | bug | high | Doc says POST /merge but serves GET /{id}/events; `Json` vs `ResponseJson`. | Comment + Json→ResponseJson on GET /{id}/events. |
| RB-41 | `crates/server/.../bin/generate_types.rs:9-11` (HEADER path) | bug | high | Stale `crates/core/...` path baked into shared/types.ts L1. | Trips --check CI until regen+commit. |
| RB-42 | `crates/executors/.../claude.rs:106-113` (plan+approvals precedence) | bug | high | plan=true AND approvals=true → plan wins, hooks NOT applied; only warn. | Construction-time error; non-breaking for correct configs. |
| RB-43 | `crates/executors/.../codex/client.rs:261-266` (`APPROVAL_WINDOW_DELAY` 20ms sleep) | bug | high | Unconditional 20ms sleep before every tool approval. **Fix root race FIRST** — removing alone risks missing tool calls. | Race-dependent. |
| RB-46 | `crates/services/.../container.rs:461-499` (setup/cleanup `working_dir`) | bug | high | `working_dir` = `repo.name` (relative) not `repo.path`; breaks multi-repo executor resolution. | Setup+cleanup paths; "invasive". |
| RB-48 | `crates/services/.../agent.rs:665-690,695-722` (dup follow-up dispatch) | duplicate | high | Two near-identical blocks differ only by trailing clause. | Extract helper; internal to 1 fn. |
| RB-49 | `crates/server/.../planning_drafts.rs:623-647,862-884` (Feishu push truncation dup) | duplicate | high | Identical 4000-char truncate+push loop in 2 handlers. | Extract `push_messages_to_feishu`. (RB-20) |
| RB-51 | `crates/services/.../terminal/process.rs:859-870` (`spawn_pty` legacy shim) | legacy | high | Delegates to `spawn_pty_with_config`; only tests call shim. | Update 3 test files. |
| RB-52 | `crates/db/.../terminal.rs:562-564` (`set_started` alias) | deprecated | medium | Pure delegation to `set_waiting`. | Switch callers; cosmetic. |
| RB-53 | `crates/services/.../git_host/{github,azure}/mod.rs:9` (`pub use GhCli/AzCli`) | redundant | medium | Re-export leaks internal wrapper; zero external importers. | Remove re-export. |
| RB-54 | `crates/db/src/lib.rs:246-314` (`new_with_after_connect`/`create_pool`) | redundant | medium | No callers outside crate; `DBService::new` uses simpler path. | ~70 lines; check test harnesses. |
| RB-56 | `crates/cc-switch/.../switcher.rs:102-129` (`backup_before_switch`) | stub | high | Field defaults true, guard body only logs "not implemented" → false confidence. | Implement or remove field. (RB-17) |
| RB-57 | `crates/services/.../file_ranker.rs:37` (`FILE_STATS_CACHE` unbounded) | dubious-feature | medium | No eviction/cap/TTL; sibling uses moka (50-cap, 1h TTL). Memory leak. | Convert to bounded moka. |
| RB-K11 | `crates/utils/src/env_compat.rs:1-41` | legacy | high | Doc: "remove no earlier than v0.2.0"; 30+ active call sites. | Migrate call sites first; CANNOT delete yet. |
| RB-K13 | `crates/services/.../prompt_watcher.rs:758` (`mark_pending_handoff_submit`) | bug | high | `#[allow(dead_code)]` is FALSE POSITIVE — called at L4046 in test. | Remove misleading annotation only. |
| RB-K14 | `crates/services/.../events.rs:33,469-491 + events/types.rs:72-82` (legacy /entries/{N} patch) | legacy | medium | `#[allow(dead_code)]`; legacy fallback branch structurally unreachable. | Remove field+struct+branch after confirming no /entries/ subscriber. |
| RB-K15 | `crates/db/.../workflow.rs:109-123` + `workflows_dto.rs:33-42` (status String, merge_terminal Option) | legacy | medium | Acknowledged tech debt; typed-enum migration needs DB migration + ~35 callers. | Large; out of scope for route-level cut. |
| RQ-05 | `crates/quality/.../rules/rust/error_handling.rs:47,64-66` | bug | high | `in_test` never set true → guards always take attr branch (dead conditional); `content:&str` never read. | Local; removing `content` drops allow. |
| RQ-06 | `crates/quality/.../provider/frontend.rs:708-713` | bug | high | `Regex::new` per call in hot `analyze()` loop. | Wrap in OnceLock<Regex>. (RQ-10) |
| RQ-07 | `crates/quality/.../provider/sonar.rs:150-181` (`wait_for_quality_gate`) | bug | high | `_task_id` unused; call passes ""; polls latest scan → races concurrent. | 1 caller. (RQ-13,14) |
| RQ-08 | `crates/quality/.../rules/common/weak_default_detection.rs:103-165` | bug | high | Provider feeds only .rs/.ts/.js; rule early-returns unless .yml/.env/.json → Docker/.env/JWT patterns never fire. | Expand provider filter OR drop dead arms. (RQ-09) |
| RQ-09 | `crates/quality/.../provider/builtin_common.rs:137-145` | dubious-feature | medium | Blocker weak-defaults only in aggregate (GT 5); up to 5 pass silently. | Additive: add MetricKey::WeakDefaultsDetected + GT 0. (RQ-08) |
| RQ-10 | `crates/quality/.../analysis/coverage_parser.rs:168-180` | bug | medium | `Regex::new` per invocation; 6 regexes/parse, no cache. | Move to LazyLock. (RQ-06) |
| RQ-11 | `crates/quality/.../rules/rust/type_complexity.rs:202-205` | bug | medium | `let _ = init` bind-and-discard; dead statement. | Zero output change. |
| RQ-12 | `crates/quality/.../discovery/mod.rs:624-656` (anti-stub gate) | dubious-feature | medium | `TOOL_TOKENS` missing oxlint/deno/bun/cargo-nextest → real scripts misclassified as stubs. Do NOT delete (re-exposes R4 bypass). | Expand/invert allowlist. |
| RQ-15 | `crates/quality/.../provider/delivery_readiness.rs:223-348` | legacy | medium | 3 detectors hardcode Hoppscotch paths; always no-op in SoloDawn. | Remove 3 fns + call sites; other detectors stay. |
| RQ-19 | `crates/quality/build.rs:21-116` (`FALLBACK_POLICY`) | redundant | medium | Hand-maintained 2nd policy copy drifts from yaml. Do NOT delete (crate must compile out-of-tree — primary-brain v1 rejection). | Trim to stub or add consistency assert. (RQ-20) |
| FE-01 | `frontend/src/lib/api.ts:605-617` (`uploadAuditDoc`) **P0** | bug | high | FE field `audit_doc`; backend accepts only `file` → 400. ALL AuditPlan System B uploads broken. | One-line `audit_doc`→`file`. Sole caller usePlanningDraft.ts:111. |
| FE-44 | `frontend/src/lib/utils.ts:7-13` (`formatBytes` export) | redundant | high | Only caller `formatFileSize` (same file). | Inline or unexport. |
| FE-49 | `frontend/.../ui-new/primitives/Toolbar.tsx:83-105` (ToolbarDropdown fallback) | dead | high | All 3 call sites pass explicit children; fallback never runs; 5 unused icon imports. | Remove dead JSX + imports. |
| FE-52 | `frontend/.../dialogs/settings/DeleteConfigurationDialog.tsx:34-42` **P0** | bug | high | try/catch wraps non-throwing resolve+hide → dead catch; isDeleting never reset. | Remove try/catch + isDeleting. |
| FE-53 | `frontend/.../dialogs/tasks/DeleteTaskConfirmationDialog.tsx:59-97` | bug | high | All strings hardcoded English (no useTranslation). | Add useTranslation + keys. (CL-I18N) |
| FE-54 | `frontend/src/pages/ui-new/FirstRunWizard.tsx:48` **P0** | bug | high | `/api/cli-types/detect` hyphen vs underscore everywhere → env step silently empty. | FirstRunWizard env step. |
| FE-55 | `frontend/.../settings/ModelsSettingsNew.tsx:93-115` | bug | medium | Per-model PUT loop swallows failures → silent partial save reported as success. | Workspace-mode model auth. |
| FE-56 | `frontend/.../board/TerminalActivityPanel.tsx:22,69` ('running') **P0** | bug | high | 'running' in ACTIVE_STATUSES but backend enum has no 'running' (→'working'). Dead. | Zero runtime change. |
| FE-57 | `frontend/.../ui-new/primitives/RepoCardSimple.tsx:28` **P0** | bug | high | `bg-tertiary` not in any Tailwind config → renders transparent. | Visual regression on RepoCardSimple. |
| FE-58 | `frontend/.../ui-new/primitives/SessionChatBox.tsx:215-223` | bug | high | 5 hardcoded English placeholder literals bypass useTranslation. | Add 5 tasks-namespace keys. (CL-I18N) |
| FE-70 | `frontend/.../setup/SetupWizardStep4IntegrationsContainer.tsx:50-51,62` | redundant | high | `saving=true` no-ops onNext but button shows no disabled/spinner → click silently ignored. | Add disabled prop. |
| FE-71 | `frontend/.../tasks/ClickedElementsBanner.tsx:13-16` (`appendInstructions`) | stub | high | Prop declared, never passed/used. | Remove prop from interface+signature. |
| FE-24 | `frontend/src/hooks/{useMessageEditRetry,useRetryProcess}.ts` | duplicate | high | Structurally identical retry/edit; only error-class differs. | Unify w/ variant param; 2 importers. (CL-CONVDUP) |
| FE-62 | `frontend/.../workflow/{Step6Advanced,Step4Terminals,types,validators}` (model-CLI compat) | redundant | high | Compat logic dup'd 4+ times across 5 files. | Extract shared util; no behavior change. |
| FE-75 | `frontend/.../NormalizedConversation/DisplayConversationEntry.tsx:687-758` (SCRIPT_TOOL_NAMES, ScriptToolCallCard) | redundant | high | Near-identical dup in NewDisplayConversationEntry:174-183. Sync hazard. | Merge ~70L; New is sole owner. (CL-CONVDUP) |
| FE-83 | `frontend/.../dialogs/shared/ConfirmDialog.tsx` | duplicate | high | All 12 call sites import ui-new version; zero runtime importers of shared. | Re-point barrel + modals.ts to ui-new. |
| FE-72 | `frontend/.../terminal/{TerminalEmulator.test:380-413,TerminalNode.test:21-34}` | stub | high | Tautological tests (toBeDefined); expand/collapse untested. | Test-only; replace stubs. |
| FE-32 | i18n: `settings:runtime.*` + common.json 11 sections missing in es/ja/ko/zh-Hant | bug | high | RuntimeSettingsNew 15+ live `t()`; key absent in 4 locales (en fallback masks). | Silent degradation. Add coverage. (CL-I18N) |
| FE-44b | `frontend/.../utils/usePreviewUrl.ts` (FE-66) | redundant | low | Thin re-export of detectDevserverUrl/useDevserverUrlFromLogs; 1 caller. | Trivial inline. |
| T03 | `crates/server/tests/cli_types_detect_test.rs:1-48` | duplicate | high | Two functionally identical detector-creation tests. | Delete one; other guards import path. (C-FIXED-BUG-GUARDS) |
| T13 | `crates/services/.../orchestrator/tests.rs:1102-1325` | redundant | high | Two tests hand-roll ~60 lines DB boilerplate instead of `setup_test_workflow()`. | Refactor only. (C-ENUM-SERDE) |
| T08 | `crates/services/.../orchestrator/tests.rs:81-149` | redundant | medium | `test_all_instruction_variants` + `test_all_instruction_parsing` both iterate 12 variants. | Merge into one parameterized round-trip. (C-ENUM-SERDE) |
| T15 | `crates/server/tests/workflow_contract.rs:1-173` | dubious-feature | high | All 4 tests operate on json! literals, never serialize a real struct → can't catch regressions. | Refactor to round-trip real DTO. |
| T09 | `crates/services/tests/error_handler_test.rs:13-21` | stub | high | Builds MessageBus, no assertions ("cannot test without DB"). | One placeholder fn. (CL-STUB) |
| T10 | `crates/server/tests/security/{encryption_test.rs:257-268,injection_prevention_test.rs:524-553}` | stub | high | Only println!, no assertions. Always-pass docs disguised as tests. | Needs test access to encrypt fn. (CL-STUB,C-SEC) |
| T11 | `crates/services/tests/merge_coordinator_test.rs:10-68` | stub | medium | 3/4 tests declare Option<T> then drop (type-system checks). | Remove 3 no-ops, keep functional. (CL-STUB) |
| T14 | `crates/server/tests/security_test.rs:1-577` | legacy | medium | All 4 call `ensure_server_running()` which PANICS if :3001 down → CI fails. Dups security/ submodule. | Refactor to #[ignore] or remove. (C-SEC) |
| T16 | `tests/e2e/workflow_create_test.rs:1-51` | redundant | medium | In e2e/ but pure unit test (no I/O); same struct tested w/ DB elsewhere. | Relocate to tests/unit/. |
| FE-45 | `frontend/src/components/rjsf/{theme.ts:22,FormTemplate.tsx,KeyValueField.tsx}` | redundant | medium | textarea alias 0 refs; FormTemplate no-op; KeyValueField bypasses RJSF onChange → breaks liveValidate. | textarea/FormTemplate safe; KeyValueField needs env-loop testing. |
| FE-85 | `frontend/.../pipeline/statusColor.ts + WorkflowProgressView:34 + ProcessListItem:58` | legacy | medium | 3 statusColor impls; E10-07 interim Tailwind colors pending design tokens. | Visual risk if token names differ. |
| FE-89 | `frontend/.../board/TerminalDots.tsx:60-72` (terminalCount fallback) | legacy | medium | Sole caller always passes terminals[]; fallback never runs. | Update TerminalDots.test. |
| FE-88 | `frontend/.../quality/QualityIssueList.tsx:28-69` (IssueItem expand) | dubious-feature | medium | expand toggle but body deleted ("not provided by DTO") → reveals nothing. Component LIVE. | Trim toggle only. |
| FE-92 | `frontend/.../ui-new/containers/{CopyButton,ConversationListContainer}.tsx` | legacy | high | Misfiled primitive + name mismatch; both LIVE. | Altitude/naming cleanup only. |
| FE-25 | `frontend/.../ui-new/containers/{DiffViewCardWithComments,GitPanelContainer,RepoCard,ContextBarContainer,ChangesPanelContainer}.tsx` (IDE bits) | dubious-feature | high | Each renders open-in-IDE; strip button/handler, KEEP component. | Strip IDE bits + attemptId thread; diff/push/PR unaffected. (CL-IDE) |
| FE-27 | `frontend/.../ui-new/primitives/{CommandBar:21-31,ContextBar:163-180}` (ide-icon branch) | legacy | high | Render IdeIcon for ide-icon; remove ONLY after Actions.OpenInIDE removed else render gap. | Components stay. (CL-IDE) |

---

## INVESTIGATE — keep/cut decision required in Phase 2

| Cluster-ID | Files | Kind | Conf | Evidence | Blast radius |
|---|---|---|---|---|---|
| **RB-D18** | `crates/runner` (gRPC, BACKLOG-002) + `crates/server/.../bin/mcp_task_server.rs` | dubious-feature | low | runner not started in default local deploy; mcp bin not in default_mcp.json. **fast-context UNAVAIL.** | **HIGH** — RunnerClientImpl/Deployment reference runner_client; npm wrapper may shell into mcp bin. CONFIRM PACKAGING. (CL-REMOTE) |
| RB-08 | `crates/services/.../prompt_handler.rs:255-265` (`set/clear_task_context` + field) | investigate | medium | Zero writers; Input path reads `task_contexts.values().next()` → silently empty. Latent integration bug. | Verify no planned agent.rs caller first. (RB-06) |
| RB-11 | `crates/services/.../git_host/detection.rs:69-88` (`detect_provider_from_pr_url`) | investigate | high | cfg(test)-only; overlaps `detect_provider_from_url`. | Zero prod; tests reference it. |
| RB-14 | `crates/utils/.../api/oauth.rs:5-46` (HandoffInit*/Redeem*/TokenRefresh*) | dead | medium | 6 shared types, zero imports, not in generate_types; server uses local structs. KEEP Status/Profile/Provider/LoginStatus. | None to compiled output. (RB-21) |
| RB-15 | `crates/utils/src/jwt.rs:1-189` | dead | medium | No use outside file; all calls in own cfg(test). | Safe delete if confirmed; check git/issue tracker. |
| RB-20 | `crates/services/.../concierge/{notifications.rs:160-205,sync.rs:69-77/151-155,agent.rs:56-68}` | dead | high | All exported, zero external callers. **`remove_session`/`cancel_watchers` never called → DashMap + watcher-token LEAK on disconnect.** | Wire cleanup into DELETE /concierge/sessions/{id}. (RB-49) |
| RB-24 | `crates/server/.../cli_types.rs:37-67,497-575` | redundant | high | Local placeholder dups real db models; 3 handlers NotImplemented; `cli_detection_cache` table absent. | Wire to DB models or remove routes. (CL-REMOTE) |
| RB-25 | `crates/server/.../workflows.rs:63-85` (WorkflowDetailResponse family) | redundant | low | Never constructed after def; superseded by WorkflowDetailDto. **fast-context UNAVAIL** to confirm. | Confirm no external/integration-test import. |
| RB-28 | `crates/local-deployment/.../lib.rs:394-397 + deployment/.../lib.rs:57` (`remote_client` + `RemoteClientNotConfigured`) | stub | medium | `remote_client()` always Err; not in Deployment trait; no callers. | Remove together. (CL-REMOTE) |
| RB-29b | `crates/services/.../persistence.rs:278-301` (`clear_state`) | dead | high | Zero callers; never called on completion → possible stale-state bug. | Decide if completion should call it. |
| RB-30 | `crates/services/.../message_bus.rs:542-556` (`publish_required`) | dead | high | Zero callers anywhere. | None at runtime. |
| RB-31b | `crates/db/.../workspace.rs:95-99` (`CreateFollowUpAttempt`) | dead | medium | Zero callers but has `#[derive(TS)]`. | Check generate_types/frontend before delete. |
| RB-32 | `crates/db/.../quality_run.rs:246-291` (backfill/cleanup/count_runs) | dead | high | Zero callers; never wired to startup/cron/HTTP. | Add caller first if retention needed. |
| RB-33 | `crates/services/.../container.rs:104-123` (`has_running_processes`) | dead | medium | Trait default, 0 external callers; could be dyn-dispatched (none found). | Remove default method. |
| RB-34 | `crates/services/.../merge_coordinator.rs:228-248` (`resolve_and_complete_merge`) | dead | medium | Public, 0 callers; manual-merge REST route never invokes. | Possibly unwired manual-conflict flow. (RB-19) |
| RB-35 | `crates/services/.../agent.rs:9101-9119` (`handle_terminal_failure`) | dead | medium | Pub fn, ZERO callers (unlike sibling pub methods wrapped by runtime.rs). | Could be intended-but-unwired API. |
| RB-36 | `crates/services/.../agent.rs:4416-4422` (`should_skip_completed_handoff`) | dead | medium | `#[allow(dead_code)]`; only own test L10791. R5 flags KEEP/investigate. | Delete fn + companion test. |
| RB-44 | `crates/db/.../workflow.rs:297-322` (`WorkflowCommand.preset_id` FK no ON DELETE) | bug | high | FK to slash_command_preset.id no CASCADE/SET NULL; delete preset → FK violation. | New migration only. |
| RB-45 | `crates/db/migrations/{20260119000000_encrypt_api_keys,20260224001000_backfill...}.sql` | legacy | medium | Plaintext key DROP commented out; encrypted col added twice; 2 mirror triggers. Plaintext may persist. | NEW migration to DROP after verifying model reads only *_encrypted. |
| RB-47 | `crates/services/.../prompt_handler.rs:671-681` (`handle_user_approval` alias) | redundant | medium | Backward-compat alias; only own tests use it. | Delete alias + retarget 2 tests, or keep. (RB-06) |
| RB-55 | `crates/server/.../workflows.rs:2317-2319` (auto_prepare 2s sleep) | dubious-feature | medium | Unconditional 2s sleep; resume_workflow already polls readiness (better). | Replace with polling; removing alone races readiness. |
| RB-59 | `crates/services/.../merge_coordinator.rs` (WorkflowMergeLocks never pruned) | dubious-feature | low | Completed-workflow entries accumulate. Low concern. | Add pruning on completion. (RB-58) |
| RB-D01 | `crates/server/.../workflows.rs:2107-2287` (DIY quiet-window monitor) | dubious-feature | low | 60s-silence=done heuristic; can misfire on slow-active terminals. Only DIY completion path. | Tune threshold, do NOT delete (DIY workflows stuck). |
| RB-D02 | `crates/server/.../planning_drafts.rs:1041,1080-1090` (AUDIT_DOC pdf/docx) | dubious-feature | medium | `confirm_draft` reads via read_to_string → binary pdf/docx = mojibake to LLM. | Trim to md/txt or add parser. |
| RB-D03 | `crates/services/.../concierge/agent.rs:206-222` (`looks_incomplete`) | dubious-feature | medium | Trailing :/.../… → loop continues, can burn tokens on legit responses. No tests. | Removing = truncated outputs saved (better UX); low risk. |
| RB-D04 | `crates/services/.../concierge/tools.rs:496-499` (audit_plan:None) | legacy | medium | Concierge workflows bypass AuditPlan System B (hardcoded None). | Update tool chain if AuditPlan required before start. |
| RB-D05 | `crates/executors/.../claude.rs:43-53` (claude-code-router proxy) | legacy | low | `@musistudio/claude-code-router` opt-in supply-chain risk; no first-party caller. | Removing breaks users who set router=true. |
| RB-D06 | `crates/executors/.../copilot.rs:256-306` (watch_session_id polling) | dubious-feature | medium | 200ms fs poll, 600s timeout, regex on debug log; fragile across versions. | Follow-up runs only. |
| RB-D07 | `crates/executors/.../cursor/mcp.rs:1-178` (MCP auto-trust) | dubious-feature | medium | Writes host ~/.cursor on every spawn; swallows errors with warn!. No tests. | Removing breaks automated MCP integration. |
| RB-D08 | `crates/executors/.../cursor.rs:1207-1223` (CursorStrReplace replace_all) | dead | medium | Deserialized never read → cursor-agent replaceAll=true silently ignored. | Changes Cursor JSONL model; no breakage. |
| RB-D09 | `crates/executors/.../droid/normalize_logs.rs:460-630` (ToolResult empty-pop) | dubious-feature | low | ToolResult w/o ToolCall silently dropped; UI stuck in 'Created'. | Add warn log; deeper fix = correlate by ID. |
| RB-D10 | `crates/server/.../workflows.rs:1899-1955` (self-heal re-prepare recursion) | dubious-feature | low | start_workflow recursively calls prepare_workflow; fragile, re-prepare loops. | Refactor preserving behavior (restart-recovery depends). |
| RB-D11 | `crates/services/.../agent.rs:6425-6507,5672-5700` (ReviewCode + whitelist no-op) | dubious-feature | medium | ReviewCode no prod emitter; whitelist matches all 13 variants (always-true). | Investigate before removing; whitelist is extension point. |
| RB-D12 | `crates/services/.../agent.rs:3540-3581,2603-2630` (skip_quiet_window + PendingGuard Drop) | dubious-feature | low | "skip" delegates to full pipeline (not skip); Drop can't clean on panic → stranded gate. | Single caller; moderate correctness. |
| RB-D15 | `crates/server/.../filesystem.rs:143-148` (non-Windows pick_folder stub) | stub | low | Returns 500 off-Windows; no FE caller. May be called by Windows tray app. | If removed: tray app could break. |
| RB-D16 | `crates/server/.../ci_webhook.rs` (receive-and-discard) | dubious-feature | low | Validates HMAC, logs, returns 202; no DB/downstream. ci-notify.yml posts to it. | Unclear if downstream expected. |
| RB-D17 | `crates/server/.../chat_integrations.rs` (CHAT_REPLAY_CACHE) | dubious-feature | low | Process-local replay cache; no cross-replica protection if scaled. | Security concern only if multi-replica. |
| RQ-13 | `crates/quality/.../provider/sonar.rs:62-100` (`import_sarif_results`) | dead | medium | Pub, 0 callers; analyze() calls CLI directly. Orphaned. **fast-context partial.** | Removing breaks only unseen external consumers (none found). (RQ-07,14) |
| RQ-14 | `crates/quality/.../provider/sonar.rs:1-297` (whole Sonar provider) | dubious-feature | low | Needs SonarQube + CLI + SONAR_TOKEN; fails closed in CI. Gated by config.providers.sonar (default unverified). | If deleted: drop engine.rs L101-110 + config field. (RQ-07,13) |
| RQ-16 | `crates/quality/.../gate/result.rs:101-113` (`EvaluationResult::warn`) | dead | medium | 0 callers; sole producer of Level::Warn. Warn aggregation never exercised (shadow done at orch level). | Deleting makes Level/Status::Warn unreachable. Confirm streamline isn't folding shadow into gate. (RQ-18,21) |
| RQ-17 | `crates/quality/.../gate/condition.rs:97-121` (`parse_threshold_f64/_i64`) | redundant | medium | No prod callers; only own tests. Evaluator uses private `parse_threshold`. | Delete with test. Check no future config-validation path. (RQ-02,03) |
| RQ-18 | `crates/quality/src/metrics.rs:55-68` (MetricKey Bugs/CodeSmells/Vulnerabilities/etc.) | dead | medium | Never emitted by any provider, never in any gate; only condition/evaluator tests. | **Serde rename**; removing breaks deserialization of external yaml. Keep if Sonar fwd-compat. (RQ-16,21) |
| RQ-20 | `crates/quality/src/config.rs:219-311` (`default_config`) | redundant | low | Reachable only if BUNDLED_CENTRAL_POLICY fails parse, but test+build.rs guarantee valid → unreachable in prod. | Keep crash-safety; consider fail-closed vs lenient Shadow. (RQ-19) |
| RQ-22 | `crates/quality/.../gate/mod.rs:100-110` (`is_blocked`/`failed_conditions`) | dead | low | Only own tests; prod uses `is_passed()`+status string. | Cheap accessors maybe wanted by reporting UI. (RQ-01) |
| RQ-23 | `crates/quality/src/issue.rs:147-150` (`as_legacy`) | dead | medium | 0 callers outside file; new-vs-legacy distinction unused. **fast-context partial.** | pub on shared crate; downstream consumer possible. (RQ-24) |
| RQ-24 | `crates/quality/src/issue.rs:227-241,163-170` (`one_line_summary`, `location_string`) | dead | low | 0 callers; leftover formatters. | Public API; possible out-of-tree/future logging. (RQ-23) |
| RQ-25 | `crates/server/src/self_test/tests.rs:29,62` (`TestContext::org_id`) | dead | medium | Declared+init None; no test reads. Scaffolded for never-written org tests. | Remove field; harmless scaffolding. |
| RQ-26 | `crates/server/benches/performance.rs` | dubious-feature | medium | Self-documented: no real SQLite/HTTP; measures allocator/tokio bookkeeping. No signal. | Confirm CI-gating before delete. |
| FE-08 | `frontend/src/vscode/bridge.ts:36-128,143-462,486-487` (iframe surface) | dubious-feature | medium | iframe protocol strings only in this file; no in-repo webview host. **Cannot disprove out-of-repo host.** | Clipboard helpers L464-484 LIVE via wysiwyg — KEEP/re-home. Confirm intent. (CL-VSCODE) |
| FE-47 | `frontend/src/utils/StyleOverride.tsx` | dead | medium | Only own file; never mounted; VITE_PARENT_ORIGIN not in .env. iframe Vibe-host bridge. | If Vibe embedding planned, removing breaks it. (CL-VSCODE) |
| FE-09 | `frontend/src/hooks/{useOpenInEditor,useOpenProjectInEditor,useEditorAvailability}.ts` | deprecated | medium | G1 family. useOpenInEditor in 14 files; useEditorAvailability in indicator+dialog. | Coordinated multi-file. (CL-IDE) |
| FE-10 | `frontend/src/lib/api.ts` (openEditor×3 + checkEditorAvailability + OpenEditorApiRequest) | dubious-feature | medium | LIVE callers: 3 hooks + DiffCard + ui-new/actions + GitPanelContainer. | Backend routes orphaned on removal. (CL-IDE) |
| FE-11 | `frontend/src/components/ide/{IdeIcon,OpenInIdeButton}.tsx` | dubious-feature | high | G1 UI leaves; cascading consumers per R1. | Large cascade: ide-icon branches, Actions.OpenInIDE, api.ts, routes, EditorType, i18n. (CL-IDE) |
| FE-26 | `frontend/.../ui-new/actions/index.ts:640-665` (`Actions.OpenInIDE`) | dubious-feature | medium | icon 'ide-icon' needs bespoke branches; parallel impls Navbar/DiffCard. | Remove ide-icon branches + SpecialIconType. (CL-IDE) |
| FE-28 | `frontend/.../NormalizedConversation/NextActionCard.tsx:127-235,269-307` (FileActionToolbar IDE button) | dubious-feature | medium | open-in-IDE on every completed attempt w/ changed files. | Self-contained; useOpenInEditor stays (4 callers). (CL-IDE) |
| FE-29 | `frontend/.../ui/actions-dropdown.tsx:82-86,193-198` (Open in IDE item) | dubious-feature | medium | 1 of 5 G1 sites. | Coordinated G1. (CL-IDE) |
| FE-30 | `frontend/.../dialogs/projects/ProjectEditorSelectionDialog.tsx + EditorAvailabilityIndicator.tsx` | dubious-feature | medium | Picker = fallback for useOpenProjectInEditor. EditorAvailabilityIndicator LIVE in GeneralSettingsNew+OnboardingDialog. | EditorAvailabilityIndicator NOT deletable until G1 proceeds. (CL-IDE) |
| FE-12 | `frontend/.../tasks/TaskDetails/preview/{DevServerLogsView,NoServerContent,PreviewToolbar,ReadyContent}.tsx` | dead | medium | Zero external importers; superseded by ui-new PreviewBrowser. | Verify no dynamic/lazy import. (CL-LEGACYPANELS) |
| FE-22 | `frontend/.../hooks/{useProjectTasks branch,useAutoLinkSharedTasks} + electric/ + useAssigneeUserName` | dubious-feature | medium | VITE_ENABLE_SHARED_TASKS not in any .env; "remote shared-task APIs disabled server-side". Electric branch unreachable. | None rendered when flag false. (CL-SHARED) |
| FE-33 | `frontend/src/hooks/{useTaskAttempt,useAttempt}.ts` | duplicate | medium | Both call attemptsApi.get; diff queryKeys → double-cache+stale. | Consolidation updates TaskPanel, GitActionsDialog + cache keys. |
| FE-35 | `frontend/src/hooks/useFollowUpSend.ts` | redundant | low | Superseded by useSessionSend; both call sessionsApi.followUp. Callers NOT verified. | Unknown until callers checked. (CL-CONVDUP) |
| FE-38 | `frontend/.../workflow/hooks/{useWizardNavigation(isStepValid,goToStep),useWizardValidation(hasErrors)}` | dead | high | WorkflowWizard never passes/calls/reads them (tests only). StepIndicator click-to-jump unwired. | goToStep removal breaks tests not prod. |
| FE-39 | `frontend/.../workflow/{Step1Basic.tsx:129-153 importFromKanban,types.ts:109 kanbanTaskIds}` | dubious-feature | high | importFromKanban radio renders but never populated/read/transmitted; no backend field. No-op. | Hide/remove = zero regression. |
| FE-40 | `frontend/.../quality/QualityTimeline.tsx + test` | dead | high | Zero importers outside own test; never mounted; fully built+tested. | May be G3 rules-editor scaffolding — confirm intent. |
| FE-59 | `frontend/.../pipeline/{OrchestratorHeader tokensUsed,TerminalDetailPanel i18n,MergeTerminalNode merge-status}` | stub | high | tokensUsed never passed (no DTO field); 2 hardcoded English labels; MergeTerminalNode deferred E10-09. | tokensUsed removal: test only. (CL-I18N) |
| FE-60 | `frontend/.../board/StatusBar.tsx:45` (Tokens: N/A) | stub | medium | Always N/A; no token hook/store/API. MVP placeholder. | Zero to remove; moderate to wire (needs backend). |
| FE-61 | `frontend/.../workflow/steps/Step4Terminals.tsx:64-86` (LEGACY_CLI_DISPLAY_NAMES/ALIASES) | legacy | medium | Compat shim for old cli_types map format; new array checked first. | If removed and old backend returns map, CLI list silently empty. Verify version compat. |
| FE-63 | `frontend/src/hooks/useModelVerification.ts` | dubious-feature | low | 1 caller; DEFAULT_MODELS hardcodes stale models (claude-3-5-sonnet, gpt-4o). | SetupWizardStep2ModelContainer only. |
| FE-65 | `frontend/src/lib/openTaskForm.ts` | stub | low | 3-line wrapper; 3 callers could call direct. Unclear if circular-import barrier. | Trivial migration if vestigial. |
| FE-67 | `frontend/src/contexts/TabNavigationContext.tsx` | dead | high | Provider never mounted; PendingApprovalEntry uses raw context w/ null-fallback → tab-awareness no-op. | Remove Provider+useTabNavigation; PendingApprovalEntry null-guards. |
| FE-68 | `frontend/src/contexts/ProcessSelectionContext.tsx` | legacy | low | 2 callers, both legacy task-detail path. No ui-new usage. | Goes with legacy task-detail removal. (CL-LEGACYPANELS) |
| FE-73 | `frontend/.../ui-new/views/ConciergeChatView.tsx:218-289` (WorkflowProgressPanel) | duplicate | medium | Private panel overlaps exported WorkflowProgressView (same DTOs). | Risk: intentional density difference. |
| FE-74 | `frontend/.../ui-new/views/Navbar.tsx:34 + actions/index.ts:579-608` (Actions.OpenInOldUI) | legacy | medium | "Open in Old UI" escape hatch → legacy routes; branch may remove old UI. 6 i18n keys. | Verify old routes gone first. (CL-OLDUI) |
| FE-76 | `frontend/.../legacy-design/LegacyDesignScope.tsx` | legacy | medium | Backs only /commands + 2 redirects; loads legacy/index.css. | Premature removal breaks /commands styling + Toast/NiceModal portal. (CL-OLDUI) |
| FE-77 | `frontend/src/pages/SlashCommands.tsx` | legacy | medium | Only workflow-domain page still on legacy design; uses old ui/Card. | UI-only migration; visual regression risk. (CL-OLDUI) |
| FE-80 | `frontend/.../ui/wysiwyg/nodes/{image-node,pr-comment-node}.tsx` (4 type exports) | dead | medium | Type-only exports, zero external imports. **fast-context down — dynamic/cross-repo unconfirmed.** | Zero runtime (tree-shaken). |
| FE-81 | `frontend/.../hooks/useSessionQueueInteraction.ts vs TaskFollowUpSection.tsx:438-487 inline` | redundant | medium | Hook LIVE; reimplemented inline in old-design TaskFollowUpSection. | Hook stays; inline is double-dead if orphaned. (CL-LEGACYPANELS) |
| FE-82 | `frontend/.../dialogs/tasks/RebaseDialog.tsx` (legacy) | duplicate | medium | Newer ui-new/dialogs/RebaseDialog used by ui-new/actions; old only by GitOperations.tsx. | Dead when legacy toolbar retired. (CL-LEGACYPANELS) |
| FE-87 | `frontend/.../ui/{card.tsx(CardFooter,CardDescription),toggle-group,multi-file-search-textarea}` | redundant | medium | CardFooter/Description 0 imports; toggle-group 1 caller; multi-file 426L 1 caller. | Migrate to ui-new when pages stable. (CL-LEGACYPANELS) |
| FE-90 | `frontend/src/components/SearchBar.tsx:17-19` | dubious-feature | medium | Returns null when disabled → vanishes vs greying; breaks layout reservation. | Navbar only; layout shift. |
| FE-84 | `frontend/.../dialogs/tasks/PrCommentsDialog.tsx:262-277` (local getErrorMessage) | dubious-feature | low | Shadows exported @/lib/modals getErrorMessage; divergence risk. | Limited to PrCommentsDialog. |
| FE-23 | `frontend/.../ui-new/primitives/conversation/ChatAssistantMessage.tsx` | redundant | low | Pure passthrough to ChatMarkdown; 1 caller. | May be intentional naming symmetry. (CL-CONVDUP) |
| FE-34 | `frontend/src/hooks/useAttemptBranch.ts` | redundant | medium | Fetches full Workspace to return .branch; no net optimization; 1 caller. | Replace w/ useAttempt. |
| FE-50 | `frontend/.../DiffViewCard useDiffData + NewDisplayConversationEntry parsers + ChatFileEntry DiffStats` | duplicate | medium | useDiffData defined twice; DiffStats dup'd; parseDiffStats re-implements. | Unify into shared hook; preserve DiffViewCardWithComments omission. (CL-CONVDUP) |
| T06 | `frontend/.../tasks/TaskCard.test.tsx + TaskCard.tsx:1-109` | dead | medium | grep yields only test; prod TaskCard is board/TaskCard. Orphan. | Risk: dynamic/string import. (dead-code-inventory) |
| T12 | `crates/server/tests/performance/database_perf_test.rs:105-110,157,169,189,531` | bug | high | SQL refs plural tables (cli_types/workflow_terminals/workflow_tasks); migration is singular. Runtime sqlx error. Tests #[ignore]. | Fix 3-4 table refs; #[ignore] so no CI impact. (C-SCHEMA-DRIFT) |
| T18 | `crates/services/tests/phase18_git_watcher.rs:1-250` | legacy | low | phase18_ prefix; functional but overlaps git_watcher_integration_test.rs. | git-CLI-spawning path loses coverage if removed. (C-PHASE18) |
| T17 | `crates/services/tests/phase18_scenarios.rs:1-600+` | legacy | low | phase18_ prefix; full migration chain + OrchestratorRuntime concurrent scenarios. Rename, don't delete. | Removing leaves concurrent-path gap. (C-PHASE18) |
| T19 | `crates/services/.../orchestrator/tests.rs:1-3516` (quality_gate_mode coverage) | dubious-feature | medium | All 33 occurrences set 'off'; shadow/warn/enforce never exercised. Quality Gate System A untested at this layer. | Additive coverage gap. (CL-QG) |

---

## KEEP — confirmed intentional (noted for Phase-2 record)

| Cluster-ID | Files | Kind | Conf | Why keep |
|---|---|---|---|---|
| RB-66 | `crates/services/.../config/editor/mod.rs:113-166` (`EditorConfig`/`EditorType`/`::new`) | dubious-feature | high | Config-schema backbone re-exported v2→v9; type of Config.editor at every version. Deleting breaks every vN::Config + on-disk JSON deserialize. **Survives CL-IDE deletion.** |
| RB-K01 | `crates/server/.../config.rs:61-65` (`REMOTE_FEATURES_ENABLED`) | dubious-feature | high | Hardwired-false; FE gates 15+ cloud features. Intentional shim (TODO flip when remote ships). (CL-SHARED) |
| RB-K02 | `crates/services/.../message_bus.rs:253-475` (RedisBus/new_redis/from_env) | dubious-feature | medium | Full multi-container PubSub; reserved ops capability. **Confirm no deploy sets SOLODAWN_MESSAGE_BUS=redis.** |
| RB-K03 | `crates/services/.../types.rs:459-541 + agent.rs:5258-5302,5446-5457` (AcceptanceReviewResult, build_acceptance_review_prompt, fallback_default_audit_plan) | legacy | medium | Legacy binary review superseded by scoring (11ac066d2) but still FALLBACK when audit_plan.raw_principles empty. Cut only after all workflows carry raw_principles. |
| RB-K04 | `crates/server/.../workflow_events.rs:54-62` (OrchestratorAwakened/Decision) | dead | high | `#[allow(dead_code)]` reserved WS-contract variants; removing breaks exhaustive matches + FE TS enum. |
| RB-K05 | `crates/db/.../execution_process.rs:63-71` (RunReason::QualityScan) | stub | high | Reserved future-taxonomy variant (P29-G04); avoids later migration. |
| RB-K06 | `crates/executors/.../claude/types.rs:199-203` (PermissionMode::AcceptEdits) | dead | medium | Defined + as_str but never emitted; reserved forward-compat UX stub. |
| RB-K07 | `crates/executors/.../codex/client.rs:242-252` (CommandExecution/FileChange RequestApproval) | dead | medium | Forward-compat shims for Codex protocol v2. |
| RB-K08 | `crates/services/.../orchestrator/runtime.rs:60-78` (ChatCommandStatus::Cancelled) | dead | medium | Reserved enum variant for future cancel feature. |
| RB-K09 | `crates/services/.../orchestrator/config.rs:66-101` (auto_merge_on_completion) | dubious-feature | low | **FALSE ALARM** — execute_auto_merge IS gated by it + tested (agent-B/C confirm). KEEP. |
| RB-K10 | `crates/db/migrations/{20260312130000_create_quality_gates,20260324160000_add_sync_toggles.down}.sql` | stub | high | sqlx tracks checksums of applied migrations; deleting corrupts existing DBs. MUST stay. |
| RB-K12 | Test-only seams: agent.rs with_llm_client, runtime.rs with_config, TerminalGateChangedFiles, status_semantics_tests, terminal_coordinator.rs new/start | stub | high | Legitimate cfg(test)/test-DI seams; removing breaks orchestrator test suites. |
| RB-58 | `crates/server/.../workflows.rs:384-461` (ORCHESTRATOR_GOVERNANCE_STATE.rate_windows) | bug | medium | KEEP feature (rate-limit+breaker wired into submit_orchestrator_chat); fix is additive periodic prune. Memory leak + state lost on restart. |
| RB-D13 | `crates/server/.../system_settings.rs:14-17,49-74` (check_admin gate) | dubious-feature | low | check_admin is deliberate shared security control; do NOT delete. |
| RB-D14 | `crates/server/.../quality.rs:153-170` (GET-only module) | stub | high | G3 ADDs CRUD, won't modify reads; removing a GET breaks useQualityGate.ts. KEEP. |
| RQ-21 | `crates/quality/.../gate/result.rs:18-19` (MeasureValue::None) | dead | low | **Serde enum persisted in quality_run decision JSON**; removing could break historical-row deserialize. Keep pending migration audit. (RQ-18) |
| FE-64 | `frontend/src/hooks/useFirstRun.ts` | legacy | low | Windows-installer first-run; 1 consumer; non-Windows never exercises. Low risk if kept. |
| FE-92b | `frontend/.../ui-new/containers/{CopyButton,ConversationListContainer}` (live) | legacy | high | Both LIVE; altitude/naming only (also in REFACTOR row FE-92). |
| T21 | `frontend/src/test/{legacy-components,legacy-routes}.test.ts` | legacy | low | Regression guards that deleted legacy code STAYS gone; value = re-introduction guard. KEEP. |
| RB-K06b | `crates/db/.../workspace.rs CreateFollowUpAttempt TS` | — | — | (cross-ref RB-31b; resolve TS export before any delete) |

---

## Coverage gaps (no candidate but flagged)

1. **Quality Gate System A non-off modes (T19)** — zero test exercises shadow/warn/enforce dispatch; in-flight feature untested at orchestrator layer.
2. **i18n live-feature coverage (FE-32)** — settings:runtime + 11 common.json sections missing in es/ja/ko/zh-Hant; en fallback masks silent degradation.
3. **Concierge session cleanup (RB-20)** — no caller for remove_session/cancel_watchers → resource leak on disconnect; needs DELETE wiring, not deletion.

---

## Notes for Phase-2 adversary

- **Confidence is the source agent's, not re-verified here.** Treat every `low`/`medium` dead/dubious row as guilty-until-proven for KEEP, especially the fast-context-unavailable rows.
- **Deletion ordering matters** for CL-IDE: delete handlers (RB-61/62/63) → shared types (RB-65) → regenerate `shared/types.ts` → strip FE call sites (FE-25/27/28/29) → delete FE leaves (FE-09/11/14/30) → prune i18n (FE-31) → decide config v10 (RB-66 KEEP types).
- **Never edit applied migrations** (RB-K10); all schema fixes (RB-44, RB-45) are NEW forward migrations.
