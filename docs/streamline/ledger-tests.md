# Streamline Ledger — Area: tests

Synthesized from 3 unit agents: `rs-services-orch-tests`, `rs-tests`, `fe-tests`.
Date: 2026-06-14. Judge: Opus (synthesis).

## Ranking key

Removal priority = f(confidence, blastRadius, dispositionHint). Higher = act sooner.
`delete` + high-confidence + zero-blast → top. `investigate`/`keep` → bottom.

## Ledger

| id | path(s) | kind | disposition | confidence | evidence | blastRadius | cross-links |
|----|---------|------|-------------|------------|----------|-------------|-------------|
| T01 | `crates/server/tests/cli_detection_test.rs` (1-38) | stub | delete | high | Both tests (`test_cli_detection_api`, `test_cli_detection_returns_installed_flag`) are TODO-only bodies — just `setup_test().await`, no assertions. VERIFIED by Read: lines 19-24, 31-36 are comment blocks. False green. | None — removes 2 always-passing no-op tests. | — |
| T02 | `crates/server/tests/slash_commands_pool_test.rs` (1-42) | redundant | delete | medium | Only checks `deployment.db().pool` is accessible (regression test for a fixed typo). Same access pattern exercised in `slash_commands_integration_test.rs` + `slash_commands_test.rs`. | None — no unique coverage. | T03 (sibling pool/import regression guards) |
| T03 | `crates/server/tests/cli_types_detect_test.rs` (1-48) | duplicate | refactor (collapse to 1) | high | `test_cli_detector_arc_creation` and `test_cli_detector_correct_import_path` are functionally identical — both `CliDetector::new(Arc::new(deployment.db().clone()))` → `_detector`. Residual regression check for a now-fixed import-path bug. | Minimal — delete one of two; the other still guards import path. | T02 (both are fixed-bug regression guards) |
| T04 | `frontend/src/hooks/useQualityGate.test.tsx` (1-268) | duplicate | delete (merge first) | high | git-tracked file OUTSIDE `__tests__/` AND `hooks/__tests__/useQualityGate.test.ts` both test the same 4 hooks. Vitest glob `src/**/*.test.*` collects BOTH → double runs. Outer is the pre-`__tests__/`-refactor file. VERIFIED both files exist. | Loses `qualityKeys` query-key tests + happy paths — MERGE those into the `__tests__/` canonical file before deleting. | T05 (same "outer vs __tests__/ duplicate" pattern); F-QG (Quality Gate UI cluster) |
| T05 | `frontend/src/pages/WorkflowDebugPage.test.tsx` (1-102) | duplicate | delete | high | git-tracked OUTSIDE `__tests__/` AND `pages/__tests__/WorkflowDebugPage.test.tsx` both test WorkflowDebugPage; inner is strictly more comprehensive (4 cases vs 3). Outer's unique case subsumed by inner. VERIFIED both files exist. | Minimal — verify "renders debug view when workflow exists" present in inner before delete. | T04 (same duplicate-location pattern) |
| T06 | `frontend/src/components/tasks/TaskCard.test.tsx` (1-109) + `frontend/src/components/tasks/TaskCard.tsx` | dead | investigate→delete | medium-high | grep for `tasks/TaskCard` across all `frontend/src/` yields ONLY the test file. VERIFIED. Production TaskCard is `components/board/TaskCard.tsx` (different file, used by WorkflowKanbanBoard). Tested component is an orphan. | Zero production blast IF orphan confirmed. Risk: dynamic/string import not caught by static grep. | Pairs with dead-code inventory for orphan `.tsx` files |
| T07 | `crates/services/src/services/orchestrator/tests.rs` (678-693) | duplicate | delete | high | `test_instruction_parsing` round-trips only the `SendToTerminal` variant — fully subsumed by `test_orchestrator_instruction_serialization` (L28, same variant+assertions) AND `test_all_instruction_parsing` (L695, all 12 variants). Zero distinct coverage. | Removing one test fn. No prod code, no external refs. | T08 (same enum serde test cluster) |
| T08 | `crates/services/src/services/orchestrator/tests.rs` (81-149) | redundant | refactor (merge) | medium | `test_all_instruction_variants` (L82, serialize→deserialize) and `test_all_instruction_parsing` (L696, deserialize-from-literal) both iterate all 12 variants asserting the serde tag. Small asymmetry (direction). Merge into one parameterized round-trip. | Removing one of two; other remains. No prod code. | T07 (enum serde cluster) |
| T09 | `crates/services/tests/error_handler_test.rs` (13-21) | stub | refactor | high | `test_error_handler_init` builds a MessageBus, stores `_message_bus`, comment "Cannot fully test without DB connection", no assertions. Second test in file is functional. | None — affects one placeholder fn. | T10, T11 (compile-only stub cluster) |
| T10 | `crates/server/tests/security/encryption_test.rs` (257-268); `crates/server/tests/security/injection_prevention_test.rs` (524-553) | stub | refactor | high | `test_different_plaintexts_different_ciphertexts` (encryption, L257) and `test_parameterized_query_patterns` (injection, L524) contain only `println!` — no assertions. Always-pass docs disguised as tests. | None — passes trivially. Fixing encryption one needs test access to encrypt fn. | T09, T11 (stub cluster); F-SEC (security/ submodule) |
| T11 | `crates/services/tests/merge_coordinator_test.rs` (10-68) | stub | refactor | medium | 3 of 4 tests (`*_creation`, `*_struct_fields`, `*_methods_exist`) only declare `Option<T>` then drop. Rust's type system already enforces this at compile. Only `test_message_bus_has_workflow_topic` has a real assertion. | Low — remove 3 no-op tests, keep the functional one. | T09, T10 (compile-only stub cluster) |
| T12 | `crates/server/tests/performance/database_perf_test.rs` (105-110,157,169,189,531) | bug | refactor (fix table names) | high | SQL refs `cli_types`, `workflow_terminals`, `workflow_tasks` (plural). VERIFIED migration tables are `cli_type`, `terminal`, `workflow_task` (singular). Runtime sqlx errors, NOT compile errors. Tests are `#[ignore]` so latent until re-enabled. | Fix = correct 3-4 table refs. All `#[ignore]` → no CI impact until run. | F-GIT (schema drift theme) |
| T13 | `crates/services/src/services/orchestrator/tests.rs` (1102-1325) | redundant | refactor (use helper) | high | `test_execute_instruction_complete_workflow_success` (L1102) and `test_execute_instruction_fail_workflow` (L1229) each hand-roll ~60 lines of identical DB-migration+project+workflow INSERT boilerplate instead of `setup_test_workflow()` (which exists + used elsewhere in-file). | Refactor only — no public API / prod code. Schema change = manual edits in 3+ places. | T08, T07 (same test file) |
| T14 | `crates/server/tests/security_test.rs` (1-577) | legacy | refactor (#[ignore]) | medium | All 4 tests call `ensure_server_running()` which PANICS (not skips) if localhost:3001 down → CI fails rather than skips. Duplicates `crates/server/tests/security/` submodule which uses `#[ignore]` gracefully. | Refactor to `#[ignore]` makes CI-safe; or remove (4 tests duplicate security/ coverage). | T10 (security/ submodule); F-SEC |
| T15 | `crates/server/tests/workflow_contract.rs` (1-173) | dubious-feature | refactor (real DTO) | high | All 4 tests operate only on hardcoded `serde_json::json!` literals, never serialize a real Rust struct / call the API. E.g. `test_list_workflows_contract` checks `.get("projectId").is_some()` on the literal it just built — cannot catch real serialization regressions. | None to prod. Refactor to round-trip through real DTO types would add genuine coverage. | — |
| T16 | `tests/e2e/workflow_create_test.rs` (1-51) | redundant | refactor (relocate) | medium | In `tests/e2e/` but is a pure unit test (sync fn, no I/O, no server). Only builds `CreateWorkflowRequest` + checks `.len()==1`. Same struct exercised w/ real DB in `workflow_create_integration_test.rs`. | Minimal — move to `tests/unit/` fixes misclassification; deleting loses a trivial field-count check. | — |
| T17 | `crates/services/tests/phase18_scenarios.rs` (1-600+) | legacy | refactor (rename) | low | `phase18_` historical-phase prefix. Uses full SQLite migration chain + `OrchestratorRuntime` for concurrent workflow scenarios; types resolve correctly. Tests functional, naming misleading. | Removing leaves a gap in OrchestratorRuntime concurrent-path coverage. Rename, don't delete. | T18 (phase18 naming cluster) |
| T18 | `crates/services/tests/phase18_git_watcher.rs` (1-250) | legacy | investigate | low | `phase18_` prefix. Functional (spawns git, writes commits, polls GitWatcher) but overlaps `git_watcher_integration_test.rs`. | If removed, integration test covers metadata parsing; the git-CLI-spawning path loses coverage. | T17 (phase18 cluster); F-GIT (GitWatcher metadata) |
| T19 | `crates/services/src/services/orchestrator/tests.rs` (1-3516) | dubious-feature | investigate (coverage gap) | medium | All 33 `quality_gate_mode` occurrences set `"off"`. No test exercises `shadow`/`warn`/`enforce`. The `MANDATORY` dispatch suffix is asserted (L1513,1684,1928,2035) only on the `off` path. Quality Gate System A non-off modes untested at this layer. | No code removed — additive coverage gap for in-flight Quality Gate System A. | F-QG (Quality Gate cluster) |
| T20 | `frontend/src/test/canvas-mock.test.ts` (1-11) | redundant | delete | medium | Meta-test verifying `setup.ts`'s `HTMLCanvasElement.getContext` mock works. Tests test-infra, not production. Redundant with any canvas-dependent test running. | Zero — no production code covered. | — |
| T21 | `frontend/src/test/legacy-components.test.ts` (1-25); `frontend/src/test/legacy-routes.test.ts` (1-14) | legacy | keep | low | Regression guards asserting deleted legacy code (projects/, TaskKanbanBoard, TasksLayout, legacy App.tsx imports) stays gone. All assertions currently pass. `legacy-routes` uses a fragile relative fs path. Value = re-introduction guard, not prod-logic test. | If deleted, no automated guard against re-adding legacy components/routes. Low risk. | — |

## Cross-link clusters

- **C-DUP-LOC** (outer-vs-`__tests__/` duplicate frontend files): T04, T05. Both are pre-`__tests__/`-refactor files double-collected by vitest. Merge unique cases into canonical `__tests__/` files, then delete outers.
- **C-STUB** (compile-only / println-only no-op tests): T01, T09, T10, T11. Either delete or convert to real assertions.
- **C-ENUM-SERDE** (orchestrator instruction serde tests, same file): T07, T08, T13. Consolidate redundant variant round-trips + use existing `setup_test_workflow()` helper.
- **C-FIXED-BUG-GUARDS** (regression checks for already-fixed structural bugs): T02, T03.
- **C-PHASE18** (historical-phase naming): T17, T18. Rename to reflect ongoing relevance; verify no coverage loss vs `git_watcher_integration_test.rs`.
- **C-SEC** (security test duplication): T10, T14. Canonical = `crates/server/tests/security/` submodule (#[ignore]-gated).
- **C-QG** (Quality Gate System A — in-flight): T19 (backend coverage gap) ↔ frontend QG UI tests (QualityIssueList/QualityReportPanel/QualityTimeline/useQualityGate, see T04). `quality-gate.yaml` has `mode: enforce` but backend tests only exercise `off`.
- **C-SCHEMA-DRIFT** (table-name / schema staleness): T12 (perf test plural table names).

## Invisible features noted (not removal candidates — context for reviewers)

- Quiet-window completion deferral (~40s) — orchestrator agent.rs; tested L2351/L2435.
- Commit idempotency via `processed_commits` HashSet — agent.rs; tested L3036-3117.
- No-metadata task inference (regex task-id hint + sole-working-terminal fallback) — tested L3119-3473.
- CLI-type-specific submit-keystroke sequences (codex double-submit vs claude-code single) — L1869/L1967.
- `MANDATORY` quality suffix on StartTask dispatch — inline Quality Gate System A hook; agent.rs; tested L1513/1684/1928/2035.
- `audit_plan: Option<String>` on Workflow — Quality Gate System B / planning-draft materialize; tests set None (untested path).
- WS per-workflow connection with refCount; terminal.prompt_detected/decision snake↔camel normalization (P0 WS contract); `sendPromptResponse` no-cross-workflow-fallback — wsStore.test.ts.
- GitWatcher `---METADATA---` commit protocol — core AI-agent↔orchestrator channel.
- FeishuHandle / ConciergeAgent+Broadcaster router params — disabled (None / in-memory) in tests.

## Tool availability incidents

**fast-context (mcp__fast-context__fast_context_search) was UNAVAILABLE / degraded across all 3 units:**
- `rs-services-orch-tests`: `resource_exhausted` on 3 of 5 calls after first success. Fell back to Grep+Read for cross-file (OrchestratorInstruction enum, ReviewCode/FixIssues usage, types.rs variants).
- `rs-tests`: `resource_exhausted` on BOTH cross-file queries attempted. All cross-file verification via Grep+Read.
- `fe-tests`: `resource_exhausted` on first call (trace 9a3f2daaeab04dd8877b42146755f07d). All usage queries via Grep (static import analysis only — no dynamic/reflection import tracing).

**Impact on confidence:** orphan-status claims (T06) and cross-file coverage claims (T18, T19, ReviewCode/FixIssues) rest on static grep only; dynamic/string imports unverified → confidence capped at medium where noted. Synthesis judge independently re-VERIFIED via grep/Read: T01 (stub bodies), T06 (orphan), T12 (table-name drift vs db/migrations), and file existence for T03/T04/T05.

## Open uncertainties carried forward

- Quiet-window duration: configurable via OrchestratorConfig or hardcoded constant? (unverified)
- `test_handle_git_event_no_metadata_marks_failed_when_task_cannot_be_inferred` (L3382): ambiguous assertion accepting two divergent outcomes — possible non-determinism / coverage gap.
- ReviewCode / FixIssues instruction variants: serde-tested only; no integration test executing those branches in `execute_instruction` confirmed in this file (terminal_coordinator_test.rs not checked).
- `#[sqlx::test]` migration path in `workflow_create_integration_test.rs` — which migrations applied unclear.
- T06 dynamic-import risk for `tasks/TaskCard.tsx`.
