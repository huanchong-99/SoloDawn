# rs-tests Census — Module Map

Unit: rs-tests
Scope: crates/services/tests/, crates/server/tests/ (incl security/, performance/), tests/e2e/, tests/unit/

## crates/server/tests/

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| auth_test.rs | Integration tests for API token auth middleware (SOLODAWN_API_TOKEN) | 7 tokio tests | Uses `server::routes::build_router`, `DeploymentImpl`, `SubscriptionHub`, `ConciergeAgent`, `ConciergeBroadcaster` | Healthy. Tests dev-mode (no token), invalid token, case-sensitive comparison. Imports all live. |
| cli_detection_test.rs | Stub tests for CLI detection API | 2 tokio tests (all-TODO bodies) | Uses `DeploymentImpl`; tests have empty bodies | **STUB**: Both test functions contain only TODO comments. No assertions. Never implemented. |
| cli_types_detect_test.rs | Compile-time check for CliDetector import path correctness | 2 tokio tests | Uses `services::services::terminal::detector::CliDetector`, `DeploymentImpl` | Minimal: only checks that `CliDetector::new(Arc<DBService>)` compiles. Both tests are identical. Duplicate. |
| deployment_process_manager_test.rs | Verifies `LocalDeployment::process_manager()` trait impl | 2 tokio tests | Uses `deployment::Deployment`, `local_deployment::LocalDeployment` | Tests `Arc::ptr_eq` of `process_manager`. Imports from concrete crate while rest of server tests use `DeploymentImpl` alias. Works since `DeploymentImpl = LocalDeployment`. |
| events_test.rs | Integration tests for OrchestratorAgent broadcast methods (workflow/task/terminal status) | 3 tokio tests | Uses `OrchestratorAgent`, `MessageBus`, `BusMessage`, `DeploymentImpl`; writes to in-memory SQLite via `DeploymentImpl::new()` | `#[serial]` tagged. Tests `broadcast_workflow_status`, `broadcast_terminal_status`, `broadcast_task_status` and verify DB updates. References `audit_plan: None` field in Workflow — field exists. |
| security_test.rs | Security integration tests requiring a running server on localhost:3001 | 4 tokio tests | Uses `reqwest::Client`, `DBService` direct, hardcoded `ENCRYPTION_KEY`, `localhost:3001` | **REQUIRES RUNNING SERVER** — panics if not running. Tests API key non-exposure and DB encryption. External-process dependency. |
| slash_commands_integration_test.rs | Integration tests for slash command DB CRUD and template rendering | 8 tokio tests | Uses `DeploymentImpl`, `SlashCommandPreset`, `WorkflowCommand`, `WorkflowCommandRequest`, `TemplateRenderer`, `WorkflowContext`, `CreateWorkflowRequest`, `TerminalConfig` | Healthy. Tests CRUD, list cap (500), template rendering, system preset protection. All types exist in `db::models::workflow`. |
| slash_commands_pool_test.rs | Verifies `deployment.db().pool` access pattern compiles and works | 2 tokio tests | Uses `local_deployment::LocalDeployment`, `server::Deployment` | **TRIVIAL/REDUNDANT**: Tests that `&deployment.db().pool` is accessible. Regression test for a now-fixed typo. Minimal value. |
| slash_commands_test.rs | Tests slash command API routes via `build_router` + `oneshot` | 8 tokio tests | Uses `build_router`, `DeploymentImpl`, `feishu_handle::new_shared_handle`, `SubscriptionHub`, `ConciergeAgent`, `ConciergeBroadcaster` | Healthy HTTP-layer tests for list/create/update/delete of slash command presets via real router. |
| terminal_logs_api_test.rs | Verifies GET /api/terminals/:id/logs returns empty array for unknown terminal | 1 tokio test | Uses `terminal_routes()`, `DeploymentImpl` | Minimal happy-path check. |
| terminal_stop_test.rs | Verifies POST/GET /api/terminals/:id/stop responds 404/405 | 2 tokio tests | Uses `terminal_routes()`, `DeploymentImpl` | Tests method routing correctness. |
| terminal_ws_test.rs | Verifies WebSocket endpoint rejects non-UUID terminal_id with 400 | 1 tokio test | Uses `terminal_ws_routes()`, `DeploymentImpl` | Single edge-case check. |
| workflow_api_test.rs | Integration tests for workflow status machine and start-workflow endpoint | 4 tokio tests | Uses `build_router`, `Workflow`, `Project`, `DeploymentImpl` | Tests: start requires ready status, status transitions, start without orchestrator returns 400. `audit_plan: None` field present correctly. |
| workflow_contract.rs | Static contract tests — validates JSON field casing (camelCase vs snake_case) and status enum values | 4 tests (2 tokio, 2 sync) | Pure serde_json; no external dependencies | **DUBIOUS**: Tests operate on hardcoded JSON literals with no actual API calls. The camelCase assertions test a static JSON blob, not the real serialization layer. Provides documentation value but zero regression protection. |

## crates/server/tests/security/

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| security/mod.rs | Module declaration for security sub-tests | — | Declares: encryption_test, access_control_test, log_sanitization_test, injection_prevention_test | |
| security/access_control_test.rs | Access control tests via live HTTP against localhost:3001 | 6 tokio tests (all `#[ignore]`) | `reqwest`, localhost:3001 | All tests tagged `#[ignore = "requires running server"]` — effectively disabled by default. Tests project scoping, UUID rejection, rate limiting, delete/update authorization. |
| security/encryption_test.rs | Encryption correctness tests | 6 tokio/sync tests (1 `#[ignore]`) | `base64`, env var guard, `reqwest` (for ignored test) | Mixed: 3 trivial env-var checks, 1 real base64 decode test, 1 `#[ignore]` server test, 1 documentation-only test (key rotation strategy print). `test_different_plaintexts_different_ciphertexts` only prints — no assertion. |
| security/injection_prevention_test.rs | Injection attack prevention tests (SQL, XSS, command, path traversal) | 8 tests (7 `#[ignore]`, 1 sync) | `reqwest`, localhost:3001 | All active tests are `#[ignore]`; only `test_parameterized_query_patterns` is always-run but it only does `println!` — no actual assertions. Unicode test is also `#[ignore]`. |
| security/log_sanitization_test.rs | Log sanitization pattern detection tests | 8 tests (all sync) | Self-contained; uses local `contains_sensitive_data()` fn | Healthy self-contained unit tests. No external deps. Tests string pattern detection logic, not actual log output. |

## crates/server/tests/performance/

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| performance/mod.rs | Module declaration for performance sub-tests | — | Declares: terminal_perf_test, websocket_perf_test, database_perf_test | |
| performance/database_perf_test.rs | DB query performance benchmarks (workflow list/detail/concurrent writes/index analysis) | 5 tokio tests (all `#[ignore]`) | `sqlx`, `parking_lot`; requires `SOLODAWN_TEST_DATABASE_URL` | All tests `#[ignore]`; references wrong table names: `cli_types` (should be `cli_type`), `workflow_terminals` (should be `terminal`), `workflow_tasks` (should be `workflow_task`). Will fail at runtime if run. Schema drift. |
| performance/terminal_perf_test.rs | Terminal WebSocket connection latency and concurrency tests | 4 tokio tests (all `#[ignore]`) + 1 benchmark helper | `tokio-tungstenite`, `reqwest`; localhost:3001 required | All tests `#[ignore = "requires running server"]`. `benchmarks::benchmark_connection_throughput` is a dead helper (never called in any test). |
| performance/websocket_perf_test.rs | WebSocket throughput and latency benchmarks | 5 tokio tests (all `#[ignore]`) | `tokio-tungstenite`, `futures-util`; localhost:3001 required | All `#[ignore]`. All use path-parameter WS format. Hardcodes `localhost:3001`. |

## crates/services/tests/

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| error_handler_test.rs | Placeholder + basic MessageBus error broadcast test | 2 tokio tests | Uses `MessageBus`, `BusMessage::Error` | First test is placeholder (no assertion); second test is a working pub/sub round-trip check. |
| filesystem_repo_discovery.rs | Tests `FilesystemService::list_git_repos` with temp directories | 5 tokio tests | Uses `FilesystemService`, `tempfile::TempDir` | Healthy. Tests discovery, skipped dirs (node_modules/target/build), empty dirs, nonexistent path, depth limit. |
| git_ops_safety.rs | Extensive git rebase/push/worktree safety tests using `GitService` and `git2` | ~30+ sync tests | Uses `GitService`, `GitCli`, `git2`, `tempfile`; all self-contained with temp repos | Large (50KB), well-tested Git safety layer. Tests push non-fast-forward, rebase idempotency, conflict detection, worktree management, rename preservation. |
| git_watcher_integration_test.rs | Tests `CommitMetadata::parse()` and GitWatcher e2e detection | Multiple tests (commit_metadata_tests mod + integration) | Uses `GitWatcher`, `GitWatcherConfig`, `CommitMetadata`, `MessageBus` | Healthy. Tests parse of basic/full metadata, absence of metadata, optional fields, issue parsing. Integration tests start a GitWatcher and assert events. |
| git_workflow.rs | Git service workflow tests: commits, branches, diffs, worktrees | ~20 sync tests | Uses `GitService`, `GitCli`, `DiffTarget`, `tempfile`, `git2` | Tests author config, branch management, diff, squash merge, binary files, unicode branches. Some tests gated `#[cfg(unix)]`. |
| merge_coordinator_test.rs | Compile-time checks for `MergeCoordinator` type compatibility | 4 tests (2 sync, 1 tokio, 1 sync) | Uses `MergeCoordinator`, `MessageBus`, `GitService`, `DBService` (options only) | Three of four tests are pure compile-time `Option<T>` declarations. Only `test_message_bus_has_workflow_topic` does a real assertion. Minimal value but low maintenance cost. |
| phase18_git_watcher.rs | E2E GitWatcher tests: detects commits and emits BusMessages | 3 tokio tests | Uses `GitWatcher`, `MessageBus`, `CommitMetadata`, `git` CLI via `Command` | Named "Phase 18" — references a historical development phase. Functional tests but tightly coupled to the `---METADATA---` commit format. |
| phase18_scenarios.rs | Complex orchestrator scenarios: concurrent workflows, failure/recovery with in-memory DB | Several tokio tests | Uses `OrchestratorRuntime`, `TerminalCompletionEvent`, `TerminalCompletionStatus`, `DBService`, full migration runner | Named "Phase 18". Uses full SQLite migration chain. References `TerminalCompletionEvent` which is in `orchestrator::types`. |
| status_broadcast_test.rs | Unit tests: BusMessage variant existence and MessageBus/OrchestratorState creation | 3 tests (2 sync, 1 tokio) | Uses `BusMessage`, `MessageBus`, `OrchestratorState` | Compile-time guards for key message bus variants. |
| terminal_binding_test.rs | Integration test: `TerminalLauncher` creates session and execution process in DB | Several tests | Uses `TerminalLauncher`, `DBService` in-memory, full migrations, `EnvVarGuard` for encryption key | Tests actual terminal launch creating DB records. |
| terminal_integration.rs | Integration tests for full terminal launch workflow | 1+ tokio tests | Uses `TerminalLauncher`, `ProcessManager`, `CCSwitchService`, `DBService` in-memory | Tests `CliDetector` lookup of pre-seeded CLI types from migrations. |
| terminal_lifecycle_test.rs | Full terminal lifecycle from creation to cleanup | Multiple tokio tests | Uses `TerminalLauncher`, `ProcessManager`, `CCSwitchService`, `Session`, `ExecutionProcess`, in-memory DB | Comprehensive lifecycle: create data, launch, simulate I/O, stop, verify cleanup. |
| terminal_logging_test.rs | Tests `TerminalLogger` writes logs to DB and retrieves them | Multiple tokio tests | Uses `TerminalLogger`, `Terminal`, `TerminalLog`, `DBService` in-memory | Tests log persistence and retrieval. |
| terminal_timeout_test.rs | Tests `ProcessManager` spawn, kill-by-PID, cleanup of dead processes | 3 tokio tests | Uses `ProcessManager`, `tempfile`; spawns real shell processes | Tests `spawn_pty`, `kill`, `cleanup`, `is_running`, `kill_terminal`, `list_running`. Crosses unix/windows via `#[cfg]`. |

## tests/e2e/

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| tests/e2e/README.md | E2E test documentation | — | — | Documents server dependency and test execution. |
| tests/e2e/workflow_test.rs | Full E2E workflow CRUD + lifecycle against live server | ~15+ tokio tests | `reqwest`, `TEST_SERVER_URL` env (default localhost:23456), panics if server not running | All tests call `ensure_server_running()` which panics if server unavailable. Tests all major workflow endpoints. |
| tests/e2e/workflow_create_test.rs | Compile-time/unit test: `CreateWorkflowRequest` struct construction | 1 sync test | Uses `db::models::CreateWorkflowRequest` and related types | **MISPLACED** in e2e/ directory: it's a unit test with no network I/O. Could be in tests/unit/. |
| tests/e2e/workflow_create_integration_test.rs | `#[sqlx::test]` integration test: creates workflow with tasks/terminals in real DB | 1 `#[sqlx::test]` test | Uses `Workflow`, `WorkflowTask`, `Terminal`, `sqlx::test` macro with managed pool | Healthy. Uses `sqlx::test` macro for automatic DB lifecycle management. References `audit_plan: None`. |
| tests/e2e/workflow_recovery_test.rs | E2E recovery/concurrent workflow tests against live server | Multiple tokio tests | `reqwest`, localhost:23456, panics if server not running | Requires live server, tests concurrent create, failure scenarios, restart recovery. |

## tests/unit/

| File | Purpose | Public Surface | Key Relations | Notes |
|------|---------|----------------|---------------|-------|
| tests/unit/slug_test.rs | Unit tests for `slugify` and `generate_task_branch_name` | 10 sync tests | Uses `services::utils::slug::{slugify, generate_task_branch_name}` | Healthy. Complete coverage of slug utility. |
