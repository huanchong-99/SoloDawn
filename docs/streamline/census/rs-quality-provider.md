# Census: rs-quality-provider

Module path: `crates/quality/src/provider/`
Branch: `refactor/streamline-quality-gates`

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `mod.rs` | Provider trait definition + shared helpers | `QualityProvider` (trait), `ProviderReport` (struct), `run_node_quality_command` (fn), `QualityScope` (enum, lives in engine.rs) | Used by ALL providers; consumed by `engine.rs`; `run_node_quality_command` called by `frontend.rs` and `repo.rs` | Strips PORT/BACKEND_PORT/FRONTEND_PORT from child processes (R6 fix) |
| `builtin_rust.rs` | Static AST analysis of `.rs` files via `syn`; no external tools | `BuiltinRustProvider` (struct/impl `QualityProvider`) | Calls `analysis::collect_files`, `rules::rust::all_rust_rules`; registered in `engine.rs` L114 | Metrics: BuiltinRustIssues, BuiltinRustCritical, RustCyclomaticComplexity, RustCognitiveComplexity |
| `builtin_frontend.rs` | Static analysis of `.ts/.tsx/.js` files using internal TS rules | `BuiltinFrontendProvider` (struct/impl `QualityProvider`) | Calls `analysis::collect_files`, `rules::typescript::all_ts_rules`, `discovery::applicable_js_targets`; registered in `engine.rs` L117 | Overrides `applicable_metrics` to skip when no JS/TS targets exist; supports incremental `changed_files` scoping |
| `builtin_common.rs` | Language-agnostic rules (duplication, secret detection) for Rust+TS files | `BuiltinCommonProvider` (struct/impl `QualityProvider`) | Calls `analysis::collect_files`, `rules::common::all_common_rules`; registered in `engine.rs` L122 | Metrics: BuiltinCommonIssues, DuplicatedBlocks, SecretsDetected |
| `sonar.rs` | SonarQube local scanner integration + SARIF import/upload | `SonarProvider` (struct/impl `QualityProvider`); `pub fn import_sarif_results`; `pub host_url`, `project_key`, `token`, `properties_path` fields | Calls `sarif::parse_sarif`, `sarif::sarif_to_issues`; registered in `engine.rs` L104; needs local sonar service + `sonar-scanner` CLI | `import_sarif_results` is pub but has zero callers outside the struct; `wait_for_quality_gate` is called with an empty task_id string — task_id param is ignored; `upload_sarif_to_sonar` is private |
| `coverage.rs` | Parses local coverage reports (tarpaulin cobertura, llvm-cov lcov, lcov) | `CoverageProvider` (struct/impl `QualityProvider`) | Calls `analysis::coverage_parser::detect_and_parse`; registered in `engine.rs` L127 | No-op when no coverage file found; aggregates multiple reports |
| `security.rs` | ReDoS pattern scan + optional external `audit-security.sh` script | `SecurityProvider` (struct/impl `QualityProvider`) | Calls `analysis::collect_files`, `analysis::is_ts_file`; registered in `engine.rs` L99 | Metrics: SecurityIssues, RedosRisks; only scans TS/JS; uses `scripts/audit-security.sh` if present |
| `rust_analyzer.rs` | Shell-out to `cargo check/clippy/fmt/test` | `RustProvider` (struct/impl `QualityProvider`) with `enable_*` flags | Calls `run_command` (private async helper); registered in `engine.rs` L87; skipped when `discovery.has_rust_targets()` is false | Parses cargo JSON message format; `CompilerMessage` and `CommandOutput` are private structs; overrides `applicable_metrics` |
| `frontend.rs` | Shell-out to ESLint/tsc/vitest per discovered JS target | `FrontendProvider` (struct/impl `QualityProvider`); `pub const UNAVAILABLE_RULE_SUFFIX` | Calls `run_node_quality_command`, `discovery::applicable_js_targets`; `UNAVAILABLE_RULE_SUFFIX` imported by `orchestrator/agent.rs` L9651; registered in `engine.rs` L91 | ESLint errors intentionally folded into advisory warnings (lint errors pinned to 0); rich dep-coherence check (Fix #4); `parse_eslint_summary` uses non-cached `regex::Regex::new` per call |
| `completeness.rs` | Structural completeness: test absence, TODO density, stub tests, coverage exclusions | `CompletenessProvider` (struct/impl `QualityProvider`) | Calls `analysis::collect_files`; registered in `engine.rs` L130 | Metrics: TestFileAbsence, TodoDensity, StubTestCount, CoverageExclusionIssues |
| `delivery_readiness.rs` | Domain-specific delivery checks (mock Express, ESM require, hoppscotch i18n, SQLx push, Redis KEYS, CSRF) | `DeliveryReadinessProvider` (struct/impl `QualityProvider`) | Calls `analysis::collect_files`; registered in `engine.rs` L135 | Contains Hoppscotch-specific checks (`detect_wrong_package_load_test_coverage`, `detect_i18n_namespace_mismatch`, `detect_duplicate_load_testing_implementation`) that are no-ops on SoloDawn |
| `repo.rs` | Runs declared repo-level commands (`generate-types:check`, `prepare-db:check`) from root package.json | `RepoProvider` (struct/impl `QualityProvider`) | Calls `run_node_quality_command`; registered in `engine.rs` L96; reads `discovery.repo_checks()` | No-op if neither script declared in root package.json |

## Candidates Summary

| Candidate | File | Kind | Confidence | Disposition |
|-----------|------|------|-----------|-------------|
| `SonarProvider::import_sarif_results` | `sonar.rs:62` | dead | medium | investigate |
| `SonarProvider::wait_for_quality_gate` task_id param | `sonar.rs:150` | bug | high | refactor |
| Hoppscotch-specific delivery checks (3 fns) | `delivery_readiness.rs:223,278,314` | legacy | medium | investigate |
| `parse_eslint_summary` non-cached regex | `frontend.rs:708-713` | bug | high | refactor |
| `SonarProvider` (whole provider) when sonar=false | `sonar.rs` | dubious-feature | low | investigate |

## toolNotes

fast-context returned `resource_exhausted` on both calls; all cross-file usage verified via Grep fallback.
