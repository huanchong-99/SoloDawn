# Gap-fill Census: 12 missed files

Unit: gapfill
Date: 2026-06-14

## Files covered

| # | File |
|---|------|
| 1 | crates/quality/src/report.rs |
| 2 | crates/quality/src/sarif.rs |
| 3 | crates/server/benches/performance.rs |
| 4 | crates/server/build.rs |
| 5 | crates/server/src/self_test/mod.rs |
| 6 | crates/server/src/self_test/orchestration.rs |
| 7 | crates/server/src/self_test/runner.rs |
| 8 | crates/server/src/self_test/tests.rs |
| 9 | crates/server/src/tests/integration/mod.rs |
| 10 | crates/server/src/tests/integration/terminal_ws_test.rs |
| 11 | crates/server/src/tests/mod.rs |
| 12 | crates/server/src/tests/terminal_validation_test.rs |

---

## Module map

### crates/quality/src/report.rs
**Purpose**: Aggregates multiple `ProviderReport` values into a single `QualityReport` struct.
Provides convenience accessors (`new_issues`, `blocking_issues`, `is_passed`, `overall_status`)
and two display helpers (`to_fix_instructions`, `status_line`).

**Public surface**: `QualityReport` (struct + impl). All methods public.

**Relations**: Consumed by `crates/quality/src/engine.rs` (constructs and annotates),
`crates/services/src/services/orchestrator/agent.rs` (reads `to_fix_instructions`/`is_passed`),
`crates/services/src/services/container.rs` (calls `new_issues`),
`crates/server/src/routes/quality.rs` (serialises to HTTP response).

**Candidates**: None. All methods have confirmed external callers.

---

### crates/quality/src/sarif.rs
**Purpose**: Parses SARIF 2.1.0 JSON into `QualityIssue` lists via `sarif_to_issues` /
`parse_sarif`. Also enforces the "advisory source cap" so ESLint SARIF `error` level
cannot re-introduce blocking issues.

**Public surface**: `SarifReport`, `SarifRun`, `SarifTool`, `SarifDriver`, `SarifRule`,
`SarifRuleConfig`, `SarifMessage`, `SarifResult`, `SarifLocation`, `SarifPhysicalLocation`,
`SarifArtifactLocation`, `SarifRegion` (all `pub`); `sarif_to_issues`, `parse_sarif` (pub fns).

**Relations**: `sarif_to_issues` consumed by `crates/quality/src/provider/sonar.rs` and
`crates/quality/src/engine.rs`. `parse_sarif` also consumed there.

**Private helpers**: `sarif_level_to_severity`, `severity_to_rule_type` — module-private, only
called inside `sarif_to_issues`. No dead code.

**Note on `severity_to_rule_type`**: Both `Major` and `Minor/Info` arms map to `CodeSmell`,
making the function effectively a two-branch switch (Blocker/Critical → Bug, else → CodeSmell).
The split arms are intentional for readability but could be collapsed. Low priority.

---

### crates/server/benches/performance.rs
**Purpose**: Criterion benchmark suite for the server crate. Six benchmark groups:
`bench_db_queries`, `bench_json_serde`, `bench_uuid_generation`, `bench_string_ops`,
`bench_encryption`, `bench_async_tasks`.

**Relations**: Referenced only by `crates/server/Cargo.toml` via `[[bench]]`. Not compiled
into any production binary.

**Candidates** (dubious, not dead):
- The file itself carries documented self-disclaimers (W2-05-01..08) acknowledging that no
  real SQLite, no real HTTP layer, and no regression baseline exist. The benchmarks measure
  allocator throughput and scheduler overhead, not server performance. They are *not dead*
  (Criterion runs them) but are **dubious-feature**: the numbers are not actionable and are
  not guarded by a CI regression threshold.

---

### crates/server/build.rs
**Purpose**: Cargo build script. Reads `POSTHOG_API_KEY`, `POSTHOG_API_ENDPOINT`, and
`VK_SHARED_API_BASE` from the environment and forwards them as `cargo:rustc-env` baked
constants. Also creates `frontend/dist/index.html` if the directory is missing (dev
bootstrap stub).

**Relations**: `POSTHOG_API_KEY` consumed as `option_env!("POSTHOG_API_KEY")` in
`crates/services/src/services/analytics.rs`. `VK_SHARED_API_BASE` consumed in
`frontend/src/lib/remoteApi.ts` (Vite side). All three variables have confirmed consumers.

**Candidates**: None.

---

### crates/server/src/self_test/mod.rs
**Purpose**: Entry point for the `self-test` subcommand. Boots a `TestServer`, runs
`tests::run_all_tests`, optionally runs `orchestration::run_orchestration_tests`, then
prints a human or JSON report and returns exit code 0/1.

**Public surface**: `TestResult`, `SelfTestReport`, `run(json, filter, orchestration)`.

**Relations**: `run()` called from `crates/server/src/main.rs` line 151. Delegates to
`runner::TestServer`, `tests::TestContext`, `orchestration::run_orchestration_tests`.

**Candidates**: None.

---

### crates/server/src/self_test/orchestration.rs
**Purpose**: Optional E2E orchestration tests (opt-in via `E2E_API_KEY` env var). Exercises
the full terminal-spawn → PTY → Claude CLI → orchestrator pipeline against a real AI endpoint
(ZhipuAI GLM-5 by default).

**Public surface**: `run_orchestration_tests(base_url, temp_dir) -> Vec<TestResult>`.

**Relations**: Called from `self_test::run()` when `orchestration=true` flag is passed.

**Candidates**: None. All private helpers (`test_cli_installed`, `test_configure_model`,
`setup_git_repo`, `test_create_project`, `test_full_workflow`, `collect_terminal_logs`,
`find_claude_binary`, `orchestration_enabled`, `e2e_*`) are called within the module.

---

### crates/server/src/self_test/runner.rs
**Purpose**: Manages `TestServer` lifecycle: binds a random port, wires the full axum router
with a temp SQLite database, waits for `/healthz` 200, provides graceful shutdown.

**Public surface**: `TestServer { base_url, port, ... }`, `TestServer::start()`,
`TestServer::temp_dir()`, `TestServer::shutdown()`.

**Relations**: Instantiated in `self_test::run()`.

**Candidates**:
- `TestServer::port: u16` — public field set at construction (line 131) but never accessed
  by any caller. The WebSocket test in `tests.rs` parses the port from `base_url` by string
  manipulation rather than reading this field. **Redundant** (`dead` code), low blast radius
  (remove the field + the assignment). Confidence: high.

---

### crates/server/src/self_test/tests.rs
**Purpose**: ~164 HTTP test cases covering all API endpoint groups in dependency order
(infra → config → setup → repos → projects → tags → tasks → workflows → cleanup).
Uses a shared `TestContext` to carry entity IDs across sequential tests.

**Public surface**: `TestContext`, `run_all_tests(ctx, filter)`.

**Relations**: Called from `self_test::run()` after `TestServer::start()`.

**Candidates**:
- `TestContext::org_id` field — never written to by any test case in `all_test_cases()`.
  `test_list_orgs` calls `assert_not_500` but never stores the result in `ctx.org_id`. The
  field is declared, initialised to `None`, and remains `None` throughout the run.
  **Dead** field. Confidence: medium (the organizations group may be intentionally read-only,
  but storing the ID would only matter if a downstream test needed it — none does).

---

### crates/server/src/tests/integration/mod.rs
**Purpose**: Thin module bridge that re-exports `terminal_ws_test`.

**Public surface**: `pub mod terminal_ws_test`.

**Relations**: Declared under `#[cfg(test)]` in `crates/server/src/tests/mod.rs`.

---

### crates/server/src/tests/integration/terminal_ws_test.rs
**Purpose**: `#[cfg(test)]` integration tests for `validate_terminal_id`: valid UUIDs accepted,
invalid formats rejected, error message is descriptive.

**Relations**: Imports `crate::routes::terminal_ws::validate_terminal_id`. Three async tests
(tokio runtime, no actual HTTP server spun up — name "integration" is aspirational; these are
effectively unit tests of the validator).

**Candidates**: The `#[tokio::test]` attribute on tests that don't use `.await` or async I/O
is cosmetically unnecessary but harmless. Not flagged.

---

### crates/server/src/tests/mod.rs
**Purpose**: Module root for the `tests` subtree; declares `terminal_validation_test` (always
compiled) and `integration` (under `#[cfg(test)]`).

---

### crates/server/src/tests/terminal_validation_test.rs
**Purpose**: Unit tests for `validate_terminal_id`: standard UUID, uppercase, mixed-case,
too-short, wrong format, missing hyphens, empty string.

**Relations**: Imports `crate::routes::terminal_ws::validate_terminal_id`.

**Note**: Functionally overlaps heavily with `terminal_ws_test.rs` (both test the same function
with nearly identical inputs). The duplication is low-risk but redundant.

---

## Candidates summary

| Path | Kind | Confidence | Disposition |
|------|------|------------|-------------|
| crates/server/src/self_test/runner.rs : `pub port: u16` (line 19, 131) | dead | high | delete |
| crates/server/src/self_test/tests.rs : `TestContext::org_id` field | dead | medium | delete |
| crates/server/benches/performance.rs (whole file) | dubious-feature | medium | investigate |
| crates/server/src/tests/terminal_validation_test.rs + integration/terminal_ws_test.rs | redundant | low | investigate |
