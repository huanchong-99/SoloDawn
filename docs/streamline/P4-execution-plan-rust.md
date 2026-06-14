# P4 Execution Plan — Area: rust

Date: 2026-06-14
Branch: `refactor/streamline-quality-gates`
Synthesizer: P4 Plan Synthesizer (Opus)
Area scope: `crates/**` (rust-backend + rust-quality). Frontend / `shared/types.ts` / i18n / tests handled by sibling area plans; this plan owns only the Rust side and names FE/regen as **preconditions/handoffs**.

> **Input note.** The P3 verdict array handed to this synthesizer was empty (`[]`) — no per-candidate adversarial verdict file was persisted for this run. This plan is therefore derived from the authoritative `docs/streamline/P2-candidate-ledger.md` plus the two source ledgers (`ledger-rust-backend.md`, `ledger-rust-quality.md`), and every CONFIRMED DELETE below was **re-verified by Grep in this pass** (`mcp__fast-context__fast_context_search` was quota-exhausted across all P2 census units, so static verification is the strongest available signal). Anything that the ledgers marked `investigate`, `low`-confidence, serde/TS-persisted, or fast-context-unverified is pushed to DEFERRED rather than executed blind, per P2 §"Notes for Phase-2 adversary".

---

## Verification performed this pass (Grep)

| Claim | Result |
|---|---|
| RB-01 `mod runtime_test;` | not found anywhere → dead, confirmed |
| RB-02 `test_edge_cases` in services/Cargo.toml | absent → never compiled, confirmed |
| RB-03 `mod share;` in services mod.rs | absent → never compiled, confirmed |
| RB-05 `UpdateExecutionProcess` | only def at execution_process.rs:104 → dead |
| RB-09 `EditToolResult` | only def at normalize_logs.rs:789 → dead |
| RB-10 `SharedEventBridge` | only def at event_bridge.rs:85 → dead |
| RB-12 `get_gitcortex_temp_dir` | only def at path.rs:129 → dead |
| RB-13 `normalize_base_url` | `use` at llm.rs:15 + calls only in `#[cfg(test)]` (llm.rs 1155-1167, url.rs 163-247) → deprecated, test-only |
| RB-16 `TelegramConnector` | def-only in chat_connector.rs (38/43/71); no instantiation → dead |
| RB-18 git.rs fns | only defs at 236 / 1082 / 1096 → dead |
| RB-19 `test_merge_coordinator_creation` | two defs: merge_coordinator.rs:325 (stub) + tests/merge_coordinator_test.rs:10 (T11) |
| RB-21 oauth handlers | route regs oauth.rs 43/44/47/48 → handlers 64/83/120/128 |
| RB-22 organizations | `pub mod` mod.rs:85 + `.merge()` mod.rs:151 |
| RB-23 projects remote stubs | defs 158/168/191/200; **route 737 shares `.delete(unlink_project)`** |
| RB-26 cc-switch fns | only defs (config_path 35/85, switcher 83) → dead |
| RB-29 `restore_conversation_history` | only def persistence.rs:306 → dead |
| RB-31 structs | only defs workspace.rs:85 / 104 → dead |
| RB-65 `OpenEditorRequest/Response` | **TWO definitions** — task_attempts.rs:794/805 (RB-61) AND projects.rs:379/389 (RB-65); repo.rs:23 imports the projects copy; generate_types.rs:128-129 registers the **task_attempts** copy |
| RQ-01 `QualityGate::with_id` | only def gate/mod.rs:46 → dead |
| RQ-02 `Operator::to_db_value` | only def condition.rs:36 → dead |
| RQ-03 `Condition::description` | only def condition.rs:124 → dead |
| RQ-04 `TestServer::port` | field set from local_addr() never read → dead |

Refinements vs P2 baseline (carry into execution):
- **RB-13** test footprint is larger than the P2 row stated: delete fn + the `use` at llm.rs:15 + the llm.rs test references (1155-1167) + the url.rs legacy test block (~163-247, not just 1153-1169).
- **RB-23** route at projects.rs:737 is `.route(path, post(link_project_to_existing_remote).delete(unlink_project))` — must **edit** to drop only the `post(...)` half and keep `unlink_project`, not delete the whole `.route(...)`.
- **RB-65 / RB-61** there are two independent `OpenEditorRequest/Response` pairs. The generate_types registration points at the task_attempts pair; both pairs die with CL-IDE but the ordering constraint (types outlive handlers) applies within each file.

---

## CONFIRMED DELETES

Ordered by cluster. Each item: id · exact file(s):lines · delete-after ordering. Line numbers are from the P2 ledger and verified anchors above; executor must re-confirm the enclosing block boundary before cutting (files drift).

### Cluster D1 — Never-compiled / orphan files (zero blast radius, do first)

| # | id | file(s):lines | delete-after |
|---|----|----------------|--------------|
| 1 | RB-01 | `crates/services/src/services/orchestrator/runtime_test.rs` (whole file 1-605) | none |
| 2 | RB-02 | `crates/services/test_edge_cases.rs` (whole file 1-46) | none |
| 3 | RB-03 | `crates/services/src/services/share.rs` (whole file 1-51) | none |
| 4 | RB-04 | `crates/db/benches/workflow_bench.rs:231-342` (`_unused_keep_old_find_by_id_setup`) | none |

### Cluster D2 — In-file dead types / fns / aliases (single-file, zero callers)

| # | id | file:lines | delete-after |
|---|----|------------|--------------|
| 5 | RB-05 | `crates/db/src/models/execution_process.rs:101-108` (`UpdateExecutionProcess` + its `#[allow(dead_code)]`) | none |
| 6 | RB-09 | `crates/executors/src/executors/droid/normalize_logs.rs:789` (`EditToolResult`) | none |
| 7 | RB-10 | `crates/server/src/routes/event_bridge.rs:85` (`SharedEventBridge` type alias) | none |
| 8 | RB-18 | `crates/services/src/services/git.rs:1082-1108` (`get_commit_subject`, `ahead_behind_commits_by_oid`), `:236-253` (`ensure_main_branch_exists`) | none |
| 9 | RB-26 | `crates/cc-switch/src/switcher.rs:83-94` (`switch_models_sequential`); `crates/cc-switch/src/config_path.rs:35-37` (`get_claude_mcp_path`), `:85-87` (`get_gemini_settings_path`) | none |
| 10 | RB-29 | `crates/services/src/services/orchestrator/persistence.rs:303-324` (`restore_conversation_history` only; **`clear_state` 278-301 is DEFERRED**) | none |
| 11 | RB-31 | `crates/db/src/models/workspace.rs:84-93` (`CreatePrParams`), `:101-107` (`AttemptResumeContext`). **`CreateFollowUpAttempt` 95-99 is DEFERRED — has `#[derive(TS)]`.** | none |
| 12 | RB-16 | `crates/services/src/services/chat_connector.rs:38-137` (`TelegramConnector` struct + impl + `ChatConnector for TelegramConnector` + comment header L30). Keep `ChatConnector` trait + `FeishuConnector`. | none |

### Cluster D3 — Deprecated, test-only callers (delete fn + retarget/remove test)

| # | id | file:lines | delete-after |
|---|----|------------|--------------|
| 13 | RB-12 | `crates/utils/src/path.rs:127-131` (`get_gitcortex_temp_dir`, `#[deprecated]`) | none (0 callers) |
| 14 | RB-13 | `crates/utils/src/url.rs:85-106` (`normalize_base_url`) **+** `crates/services/src/services/orchestrator/llm.rs:15` (`use`) **+** llm.rs test refs (1155-1167) **+** url.rs legacy test block (~163-247) | delete callers/tests in the **same commit** as the fn (it is referenced from a top-level `use`, so the fn cannot be removed until L15 + test calls go) |
| 15 | RB-17 | `crates/services/src/services/cc_switch.rs:596-678` (`switch_for_terminal`/`switch_for_terminals`), `:1247-1253` (test `test_switch_for_terminals_method_exists`), `:462-465` (`CCSwitch` trait) + the `mod.rs` re-export of `CCSwitch` | delete after RB-56 decision is folded in (same file); breaks the one test by design |

### Cluster D4 — Remote/stub routes returning 501/BadRequest (CL-REMOTE, server)

| # | id | file:lines | delete-after |
|---|----|------------|--------------|
| 16 | RB-22 | `crates/server/src/routes/organizations.rs` (whole file 1-188) **+** `crates/server/src/routes/mod.rs:85` (`pub mod organizations;`) **+** `mod.rs:151` (`.merge(organizations::router())`) | none |
| 17 | RB-21 | `crates/server/src/routes/oauth.rs:64-128` (`handoff_init`/`handoff_complete`/`get_token`/`get_current_user`) **+** route regs `:43,:44,:47,:48` **+** their `#[allow(dead_code)]` DTOs. **KEEP `status()` (ConfigProvider) + `logout()` (FE caller).** | FE preconditions: FE handoff/token/user callers already always fail; confirm no FE depends on the 400 body shape (P2 RB-21 says safe) |
| 18 | RB-23 | `crates/server/src/routes/projects.rs:158-207` (`link_project_to_existing_remote`, `create_and_link_remote_project`, `get_remote_project_by_id`, `get_project_remote_members`) + their route regs. **At `:737` EDIT the route to drop only `post(link_project_to_existing_remote)` and KEEP `.delete(unlink_project)`.** Remove `:732`, `:739`, `:761` regs. Drop the `_repo_name` dead binding. | none on Rust side; FE `linkToExisting`/`createAndLink`/`useProjectRemoteMembers` already error-handle |

### Cluster D5 — Quality crate dead fns/fields (zero callers, no serde surface)

| # | id | file:lines | delete-after |
|---|----|------------|--------------|
| 19 | RQ-01 | `crates/quality/src/gate/mod.rs:46-56` (`QualityGate::with_id`) | none |
| 20 | RQ-02 | `crates/quality/src/gate/condition.rs:36-41` (`Operator::to_db_value`) | none |
| 21 | RQ-03 | `crates/quality/src/gate/condition.rs:124-126` (`Condition::description`) | none |
| 22 | RQ-04 | `crates/server/src/self_test/runner.rs:19` (field `port`) **+** `:131` (assignment) | none |

### Cluster D6 — CL-IDE backend deletion (lockstep with FE area; ordering-critical)

> This is the **only cross-area cluster touching Rust**. The Rust deletions here MUST be coordinated with the frontend area plan. Backend handlers/types are deleted first, then `generate_types.rs` edited, then `shared/types.ts` regenerated, then FE call sites stripped. See PRECONDITIONS below. `EditorConfig`/`EditorType` (RB-66) are **KEPT** — config-schema backbone.

| # | id | file:lines | delete-after |
|---|----|------------|--------------|
| 23 | RB-61 | `crates/server/src/routes/task_attempts.rs:809-888` (`open_task_attempt_in_editor`), `:890-946` + `:965-1044` (4 helpers + test module), `:2143` (route reg). **PRESERVE `status_semantics_tests` at 949-963.** Delete the type pair `:793-807` (`OpenEditorRequest`/`OpenEditorResponse`) **LAST within this file** (handler at 968 uses it). | within-file: handler+helpers+route → then the 793-807 type pair |
| 24 | RB-62 | `crates/server/src/routes/projects.rs:393-464` (`open_project_in_editor`), `:466-536` (helpers), `:765-840` (test). | before RB-65 type-pair delete |
| 25 | RB-63 | `crates/server/src/routes/repo.rs:215-278` (`open_repo_in_editor`), `:55-132` (helpers + test), and `:23` import `use routes::projects::{OpenEditorRequest, OpenEditorResponse}` | before RB-65 type-pair delete |
| 26 | RB-50 | path-safety helper dupes vanish automatically with the above: `repo.rs:55-98`, `projects.rs:421-464`, `task_attempts.rs:809-811` are inside the deleted handler/helper ranges. No separate action if D6 proceeds. | auto-removed by RB-61/62/63 |
| 27 | RB-65 | `crates/server/src/routes/projects.rs:378-391` (`OpenEditorRequest`/`OpenEditorResponse` — the projects copy, imported by repo.rs) — **DELETE LAST**, after RB-62 and RB-63 handlers are gone. | strictly after RB-62, RB-63 |
| 28 | (gen) | `crates/server/src/bin/generate_types.rs:128-129` (`OpenEditorRequest::decl()`, `OpenEditorResponse::decl()`). **`CheckEditorAvailabilityQuery/Response` regs 114-115 + `EditorOpenError` reg 170 belong to RB-64 — DEFERRED, do not remove here.** | after all of RB-61/62/63/65; then run `npm run generate-types` (precondition handoff) |

---

## REFACTORS IN SCOPE

Live code with a confirmed bug / dup / perf / leak. All are high-confidence, self-contained, and behavior-preserving (or behavior-fixing for a clear bug). Higher-risk refactors are in DEFERRED.

| id | file:lines | fixSketch | risk |
|----|------------|-----------|------|
| **RB-37** | `crates/services/src/services/cc_switch.rs:800-808` | **SECURITY.** Add cleanup of `claude_home/settings.json` + `codex_home/auth.json` temp dirs on terminal end. Hook into `ProcessManager::finalize_terminated_process()` or wrap the temp dir in a `TempDirGuard` (Drop-based). | Medium — must guarantee cleanup runs on the panic/abort path too; verify no in-flight reuse of the dir before unlink. |
| **RB-38** | `crates/services/src/services/error_handler.rs:150-158` (`activate_error_terminal`) | Replace `CliType::find_all().first()` / `ModelConfig::find_all().first()` with the workflow's `error_terminal_cli_id` / `error_terminal_model_id` (db workflow.rs L157/160) in the creation branch. | Low — ~20 lines, creation branch only; correct configs unchanged. |
| **RB-39** | `crates/server/src/routes/terminals.rs:119-121` (`STARTABLE_TERMINAL_STATUSES`) | Remove `'working'` from the set (matches runtime_actions.rs G15-007). Also resolve the 3-way name collision with `runtime_actions.rs` (4-item) and `constants.rs` (1-item) — pick one canonical const. | Low — tightens a safety gate; clients should not POST /start on a working terminal. |
| RB-40 | `crates/server/src/routes/workflows.rs:3619-3632` (`get_workflow_events`) | Fix the stale doc comment (says POST /merge; actually GET /{id}/events). Change `Json(...)` → `ResponseJson(...)` for consistency. | Low — comment is zero-runtime; `Json`→`ResponseJson` affects only GET /{id}/events serialization headers; verify consumer parses identically. |
| RB-41 | `crates/server/src/bin/generate_types.rs:9-11` (HEADER) | Fix the path string `crates/core/...` → `crates/server/...`. | Low-but-CI: trips `--check` until `npm run generate-types` re-run + committed. **Bundle with the D6 regen step** so types.ts banner + content regenerate together. |
| RB-42 | `crates/executors/src/executors/claude.rs:106-113` | When `plan=true && approvals=true`, return a construction-time error instead of silently letting plan win + only `warn`. | Low — non-breaking for correct configs; surfaces already-broken-but-silent configs as an explicit error. |
| RB-48 | `crates/services/src/services/orchestrator/agent.rs:665-690, 695-722` | Extract the two near-identical "tasks created but no terminals → follow_up dispatch" blocks into one private `dispatch_terminals_for_tasks_without_terminals(..)`; thread the trailing "no markdown." difference as a param. | Low — internal to one private async fn; preserve both behaviors exactly. |
| RB-49 | `crates/server/src/routes/planning_drafts.rs:623-647, 862-884` | Extract the duplicated 4000-char `floor_char_boundary` truncate+push loop into `push_messages_to_feishu(..)`; call from both `send_message` and `toggle_feishu_sync`. | Low — both call sites are background spawns; pure internal cleanup. |
| RB-51 | `crates/services/src/services/terminal/process.rs:859-870` (`spawn_pty` shim) | Delete the shim; retarget the 3 test files (terminal_timeout_test / terminal_lifecycle_test / terminal_integration) to `spawn_pty_with_config`. | Low — all prod callers already use the canonical fn; test-only retarget. (Borderline delete, but it has test callers → refactor.) |
| RB-53 | `crates/services/src/services/git_host/github/mod.rs:9`, `azure/mod.rs:9` | Remove `pub use cli::GhCli` / `pub use cli::AzCli` re-exports (zero external importers; access via `GitHostProvider` trait). | Low — re-export removal only. |
| RB-K13 | `crates/services/src/services/orchestrator/prompt_watcher.rs:758` | Remove the **misleading `#[allow(dead_code)]` only** — `mark_pending_handoff_submit` IS called at L4046 (test). Do NOT remove the method. | Trivial — annotation-only. |
| **RQ-05** | `crates/quality/src/rules/rust/error_handling.rs:47, 64-66` | `in_test` is never set true → guards at 112/120/135/148 always take the attr branch (dead conditional). Remove the dead `in_test` field/branch; remove the unread `content:&str` (+ its `#[allow(dead_code)]`). | Low — local; confirm the attr branch is the intended sole path before deleting the conditional. |
| **RQ-06** | `crates/quality/src/provider/frontend.rs:708-713` (`parse_eslint_summary`) | Move the per-call `Regex::new` into a `static`/`OnceLock<Regex>` (matches the file's existing convention). | Low — perf-only; identical match semantics. |
| **RQ-07** | `crates/quality/src/provider/sonar.rs:150-181` (`wait_for_quality_gate`) | The `_task_id:&str` param is unused (call site L285 passes `""`). Either remove the param or make it actually scope the poll to that scan. Minimal fix: drop the dead param. | Low — single caller. (Coupled to sonar-provider fate; see DEFERRED RQ-14.) |
| RQ-10 | `crates/quality/src/analysis/coverage_parser.rs:168-180` (`extract_attr_f64/_u64`) | Replace per-invocation `Regex::new` (6 regexes/parse) with `LazyLock`/`OnceLock`, or use `str::find`. | Low — perf-only; same parse output. |
| RQ-11 | `crates/quality/src/rules/rust/type_complexity.rs:202-205` | Remove the dead `let _ = init;` bind-and-discard inside `if let Some(ref init)` (descent is via `syn::visit::visit_local` at L212). | Trivial — zero output change. |
| RQ-15 | `crates/quality/src/provider/delivery_readiness.rs:223-348` | Remove the 3 Hoppscotch-hardcoded detectors (`detect_wrong_package_load_test_coverage`, `detect_duplicate_load_testing_implementation`, `detect_i18n_namespace_mismatch`) + their call sites; their metrics are still fed by other detectors. | Low — always no-op in SoloDawn; confirm the metric keys they emit are still produced elsewhere before removing call sites. |

---

## DEFERRED (reported, not executed)

Pushed out of this execution plan. One-line reason each. Grouped by why.

### Serde / TS / migration-persisted surfaces (removal risks deserialization or schema)
- **RB-31b** `db/.../workspace.rs:95-99` `CreateFollowUpAttempt` — has `#[derive(TS)]`; resolve generate_types + FE consumer before deleting.
- **RQ-18** `quality/src/metrics.rs:55-68` MetricKey Bugs/CodeSmells/Vulnerabilities/etc — serde-renamed enum persisted in `quality_run` JSON + external YAML; removal breaks historical-row/yaml deserialize.
- **RQ-21** `quality/.../gate/result.rs:18-19` `MeasureValue::None` — serde enum persisted in quality_run decision blob; keep pending DB migration audit.
- **RQ-16** `quality/.../gate/result.rs:101-113` `EvaluationResult::warn` — sole producer of `Level::Warn`; deleting makes Warn arms in agent.rs/report.rs dead; confirm streamline isn't folding shadow into the gate model first.
- **RB-44** `db/.../workflow.rs:297-322` `WorkflowCommand.preset_id` FK missing ON DELETE — fix is a NEW forward migration, never an in-place edit.
- **RB-45** `db/migrations/20260119000000_encrypt_api_keys.sql` + `..._backfill....sql` plaintext key DROP — NEW forward migration only, after verifying the Rust model reads only `*_encrypted`.

### Investigate — unwired but possibly intended (need a keep/cut decision, not a blind delete)
- **RB-08** prompt_handler.rs:255-265 `set/clear_task_context` + field — zero writers but Input path reads `task_contexts`; possible latent integration bug, verify no planned agent.rs caller.
- **RB-29b** persistence.rs:278-301 `clear_state` — never called on completion; possible stale-state bug, decide if completion SHOULD call it.
- **RB-30** message_bus.rs:542-556 `publish_required` — zero callers; confirm not a reserved publish path.
- **RB-32** db/services quality_run.rs:246-291 backfill/cleanup/count_runs — zero callers; if retention is wanted, add a caller first.
- **RB-33** container.rs:104-123 `has_running_processes` trait default — possible dyn-dispatch (none found); confirm before removing default.
- **RB-34** merge_coordinator.rs:228-248 `resolve_and_complete_merge` — possibly unwired manual-conflict-resolution flow.
- **RB-35** agent.rs:9101-9119 `handle_terminal_failure` — pub fn zero callers; could be intended-but-unwired API.
- **RB-36** agent.rs:4416-4422 `should_skip_completed_handoff` — `#[allow(dead_code)]`, only own test; R5 flags KEEP/investigate.
- **RB-11** git_host/detection.rs:69-88 `detect_provider_from_pr_url` — cfg(test)-only, overlaps `detect_provider_from_url`; zero prod impact.
- **RB-14** utils/api/oauth.rs:5-46 Handoff*/TokenRefresh* — 6 shared types, but pub on a shared crate; confirm no out-of-tree/generate_types consumer (keep Status/Profile/Provider/LoginStatus).
- **RB-15** utils/src/jwt.rs (whole) — no callers outside own cfg(test); forfeits tested JWT logic, check git/issue tracker first.
- **RB-24** server/.../cli_types.rs:37-67,497-575 — `cli_detection_cache` DB table does not exist; wire to DB models or remove routes (CL-REMOTE decision).
- **RB-25** workflows.rs:63-85 WorkflowDetailResponse family — fast-context unavailable to confirm zero external/integration-test consumer.
- **RB-28** local-deployment/deployment `remote_client` + `RemoteClientNotConfigured` — remove together; part of CL-REMOTE.
- **RB-47** prompt_handler.rs:671-681 `handle_user_approval` alias — backward-compat alias, only own tests; delete + retarget or keep for API stability.
- **RQ-13** sonar.rs:62-100 `import_sarif_results` — pub, 0 callers; fast-context partial; possible out-of-tree consumer.
- **RQ-17** condition.rs:97-121 `parse_threshold_f64/_i64` — only own tests; check no future config-validation path intended.
- **RQ-22** gate/mod.rs:100-110 `is_blocked`/`failed_conditions` — only own tests; cheap accessors maybe wanted by reporting UI.
- **RQ-23 / RQ-24** issue.rs `as_legacy` / `one_line_summary` / `location_string` — pub on shared crate; possible out-of-tree/future-logging consumer.
- **RQ-25** self_test/tests.rs:29,62 `TestContext::org_id` — scaffolding for never-written org tests; harmless, low value to cut now.

### High-risk / packaging-unverified (HIGH blast radius — do NOT cut without explicit confirmation)
- **RB-D18 / CL-REMOTE** `crates/runner` (gRPC) + `crates/server/.../bin/mcp_task_server.rs` — npm wrapper may shell into the mcp bin; `RunnerClientImpl` referenced by Deployment trait. CONFIRM PACKAGING first.
- **RB-K02** message_bus.rs RedisBus/new_redis/from_env — reserved ops capability; confirm no deploy sets `SOLODAWN_MESSAGE_BUS=redis`.
- **RB-K03** types.rs AcceptanceReviewResult + agent.rs build_acceptance_review_prompt/fallback_default_audit_plan — still the FALLBACK when `audit_plan.raw_principles` is empty; cut only after all workflows guaranteed to carry raw_principles.

### Refactors deferred for risk / scope
- **RB-43** codex/client.rs:261-266 `APPROVAL_WINDOW_DELAY` 20ms sleep — must fix the underlying tool-approval race FIRST; removing the sleep alone risks missing incoming tool calls.
- **RB-46** container.rs:461-499 setup/cleanup `working_dir` = repo.name not repo.path — real bug but "invasive" (touches all setup+cleanup executor-resolution paths); needs a dedicated change + multi-repo test, not a sweep edit.
- **RB-54** db/src/lib.rs:246-314 `new_with_after_connect`/`create_pool` — ~70-line pool-builder simplification; check all test harnesses first.
- **RB-55** workflows.rs:2317-2319 `auto_prepare_and_start` 2s sleep — replace with readiness polling (resume_workflow pattern); removing alone races terminal readiness.
- **RB-56** cc_switch.rs:102-129 `backup_before_switch` — implement the backup or remove the false-confidence field; couple with RB-17 (same file).
- **RB-57** file_ranker.rs:37 `FILE_STATS_CACHE` unbounded — convert to bounded moka mirroring `FileSearchCache`; latent leak, low urgency.
- **RB-52** db/.../terminal.rs:562-564 `set_started` alias — cosmetic; switch launcher.rs + test callers to `set_waiting` (low value, defer to a naming sweep).
- **RB-58** workflows.rs:384-461 ORCHESTRATOR_GOVERNANCE_STATE.rate_windows — KEEP feature; fix is an additive periodic prune (memory leak), out of scope for a delete/refactor sweep.
- **RB-59** merge_coordinator.rs WorkflowMergeLocks never pruned — low practical concern; add pruning on completion later.
- **RB-K11** utils/src/env_compat.rs — module doc says "remove no earlier than v0.2.0"; 30+ active call sites pass both SOLODAWN_/GITCORTEX_ names; migrate call sites first, CANNOT delete yet.
- **RB-K14** events.rs:33,469-491 + events/types.rs:72-82 legacy `/entries/{N}` patch path — remove field+struct+branch only after confirming no client subscribes to `/entries/`.
- **RB-K15** workflow.rs status-as-String + workflows_dto Option fields — typed-enum migration needs a DB migration + ~35 callers; out of scope here.
- **RQ-08 / RQ-09** weak_default_detection.rs / builtin_common.rs — security feature half-wired (provider file-filter excludes infra files; no dedicated metric). Changing the provider filter alters scan behavior + adds a MetricKey/condition; product decision, defer.
- **RQ-12** discovery/mod.rs:624-656 anti-stub `TOOL_TOKENS` — expand/invert the allowlist (missing oxlint/deno/bun/cargo-nextest); security-critical gate, refactor carefully, NOT in a bulk sweep.
- **RQ-14** sonar.rs:1-297 whole SonarProvider — decide provider fate (needs SonarQube+CLI+token; gated by config.providers.sonar, default unverified) before micro-fixing RQ-07/RQ-13.
- **RQ-19** quality/build.rs:21-116 `FALLBACK_POLICY` — do NOT delete (crate must compile out-of-tree; primary-brain v1 rejection); trim to a compile-only stub or add a build-time consistency assert later.
- **RQ-20** quality/config.rs:219-311 `default_config` — unreachable-in-prod 3rd policy copy but crash-safety fallback; keep, consider fail-closed vs lenient Shadow separately.
- **RB-D01..RB-D17** (workflows DIY monitor, planning_drafts pdf/docx, concierge heuristics, executor proxies/polling, filesystem/ci_webhook/chat stubs) — dubious-feature/product-review items; tune or trim with product input, not in this structural sweep.

### CL-IDE backend tail (deferred until FE area confirms lockstep)
- **RB-64** config.rs:693-719 `check_editor_availability` + route regs 48-51 + generate_types 114-115 + `EditorOpenError` reg 170 — LIVE-wired to `EditorAvailabilityIndicator`/`useEditorAvailability`; R1 classes it under CL-IDE DELETE but it is NOT safe standalone. Cut only when the FE availability indicator is removed in the same lockstep PR.
- **RB-66** config/editor/mod.rs:113-166 `EditorConfig`/`EditorType`/`::new` — **KEEP** (config-schema backbone v2→v9; on-disk `editor:{}` JSON + every vN::Config depend on it). Trim only `remote_ssh_*` sub-fields if a v10 migration is done; do NOT delete the types.

### Concierge resource-leak wiring (refactor, but needs new wiring — coordinate)
- **RB-20** concierge notifications.rs:160-205 / sync.rs:69-77,151-155 / agent.rs:56-68 — `remove_session`/`cancel_watchers_for_session` never called → DashMap + watcher-token leak on disconnect. Fix = WIRE into `DELETE /concierge/sessions/{id}`, not delete. Decide Feishu broadcast path (register_feishu) keep-or-remove. Out of scope for the delete sweep; schedule as a dedicated leak-fix change.

### Test-area items (owned by tests area plan, listed for cross-ref)
- **RB-19** merge_coordinator.rs:322-328 `test_merge_coordinator_creation` stub — co-tracked with T11 (`tests/merge_coordinator_test.rs`); delete the in-file stub here is safe (CONFIRMED-DELETE candidate) but its sibling test file is owned by the tests area; **execute the in-file stub delete (RB-19) as part of D2** and let the tests-area plan handle merge_coordinator_test.rs. → Promoted to CONFIRMED DELETE #29 below.

#### Late addition to CONFIRMED DELETES
| # | id | file:lines | delete-after |
|---|----|------------|--------------|
| 29 | RB-19 | `crates/services/src/services/merge_coordinator.rs:322-328` (`test_merge_coordinator_creation`, no-assertion stub) | none (placeholder; real tests live elsewhere) |

---

## PRECONDITIONS & ORDERING (rust area)

1. **Baseline green first.** Run `cargo check --workspace` (and `cargo test --workspace --no-run`) before any edit; compare against `docs/streamline/baseline-cargo-check.log` / `docs/baseline/cargo-check.log`. Do not start deletes on a red baseline.

2. **Execution order across clusters:** D1 → D2 → D3 → D5 (all independent, single-file, zero cross-area) → D4 (server routes, FE-aware but FE already error-tolerant) → **D6 LAST** (the only cross-area / ordering-critical cluster).

3. **D6 (CL-IDE) strict ordering** — must be one coordinated FE+BE PR:
   a. Delete BE handlers + helpers + tests: RB-61 (task_attempts, preserve `status_semantics_tests` 949-963), RB-62 (projects), RB-63 (repo, incl. the `use` at repo.rs:23).
   b. Delete the shared TS-bearing type pairs **after** their handlers: task_attempts.rs:793-807 (after its handler) and projects.rs:378-391 / RB-65 (after RB-62 AND RB-63, since repo.rs imports the projects copy).
   c. Remove `generate_types.rs:128-129` registrations (NOT 114-115 / 170 — those are RB-64, deferred).
   d. **Regenerate** `shared/types.ts` via `npm run generate-types`, then commit it — CI runs `generate-types --check`; stale types.ts fails the build. Bundle RB-41 (HEADER path fix) into this same regen so the banner + content update together.
   e. Frontend area then strips FE call sites (FE-25/27/28/29) and deletes FE leaves (FE-09/11/14/30) and prunes i18n (FE-31). **Backend deletes must land before or with FE type-import removal**, never after (FE imports would dangle).
   f. **KEEP `EditorConfig`/`EditorType` (RB-66).** RB-64 (`check_editor_availability`) stays until the FE availability indicator is removed in the same lockstep.

4. **Never edit applied migrations** (RB-K10: `20260312130000_create_quality_gates.sql`, `20260324160000_add_sync_toggles.down.sql`). All schema fixes (RB-44 FK, RB-45 plaintext-key DROP) are **NEW forward migrations** — deferred, and when done must be additive `.sql` files, never in-place edits; sqlx checksums of applied migrations must not change.

5. **Serde/TS surfaces are frozen** until a deserialization/DB audit: RQ-18 (MetricKey), RQ-21 (MeasureValue::None), RB-31b (`CreateFollowUpAttempt` `#[derive(TS)]`). Do not delete in this sweep.

6. **RB-13 atomicity:** delete `normalize_base_url` + its `use` (llm.rs:15) + all test call sites (llm.rs 1155-1167, url.rs ~163-247) in a **single commit** — the top-level `use` makes a partial delete fail to compile.

7. **RB-17 / RB-56 same file:** both touch `cc_switch.rs`; fold the RB-56 `backup_before_switch` decision before/with the RB-17 deprecated-method delete to avoid a second pass on the file (CLAUDE.md: >3 edits to one file → reassess).

8. **Per-cluster verification gate:** after each cluster, run `cargo check -p <crate>` for the touched crate(s) and the dependent crates (`db` → `services` → `server`; `quality` → `server`) before moving on. After D6 + regen, run the full `cargo check --workspace` + `npm run generate-types -- --check`.

9. **Quality crate (D5/RQ refactors)** are independent of D6 and can land in a separate PR; only `quality` + `server` recompile.

10. **Out-of-area handoffs:** `shared/types.ts` regeneration, FE call-site stripping, i18n pruning, and the `tests/merge_coordinator_test.rs` sibling are owned by the frontend/tests area plans — this plan only guarantees the Rust side compiles after its own deletes and names the regen/strip steps as ordering constraints.
