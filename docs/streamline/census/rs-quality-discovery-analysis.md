# Census: rs-quality-discovery-analysis

Unit: `crates/quality/src/discovery/` + `crates/quality/src/analysis/`
Branch: `refactor/streamline-quality-gates`

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `crates/quality/src/discovery/mod.rs` (1513 lines) | JS/Node workspace discovery: scans repo for `package.json` / `Cargo.toml`, resolves package managers, capabilities (lint/typecheck/test), workspace topology, dep-graph expansion | `PackageManager` (enum+impls), `NodeQualityCommand` (enum+`describe()`), `resolve_node_exe()`, `resolve_node_command()`, `JsTargetCapabilities`, `RepoChecks`, `JsTarget` (struct+accessors), `RepositoryDiscovery` (struct+`discover()`+accessors+`applicable_js_targets()`) | Consumed by `engine.rs` (orchestration), `provider/frontend.rs`, `provider/builtin_frontend.rs`, `provider/builtin_common.rs`, `provider/repo.rs`, `provider/rust_analyzer.rs`, `provider/coverage.rs`, `provider/delivery_readiness.rs`, `provider/security.rs`, `provider/sonar.rs`, `provider/completeness.rs`, `provider/mod.rs`; `resolve_node_exe` also called from `services/orchestrator/agent.rs` | Anti-stub gate (`is_stub_script_body`) is a security-relevant runtime feature; pnpm-workspace.yaml parsing; Windows PATH-scan shim logic (R5 Fix 6) |
| `crates/quality/src/analysis/mod.rs` (61 lines) | Shared file-classification and collection utilities for built-in providers | `is_rust_file()`, `is_ts_file()`, `is_excluded()`, `collect_files()` | Used by `provider/builtin_common.rs`, `provider/builtin_frontend.rs`, `provider/builtin_rust.rs`, `provider/completeness.rs`, `provider/delivery_readiness.rs`, `provider/frontend.rs`, `provider/security.rs` | Private helper `collect_files_recursive` only used internally |
| `crates/quality/src/analysis/coverage_parser.rs` (261 lines) | Parses lcov `.info` and Cobertura XML coverage reports into `CoverageReport` | `CoverageReport` (struct), `parse_lcov()`, `parse_cobertura()`, `detect_and_parse()` | Used by `provider/coverage.rs` (calls `coverage_parser::detect_and_parse`) | Private helpers `extract_attr_f64`/`extract_attr_u64` compile a fresh `Regex` per call (minor perf concern, but report parsing is one-shot); `CoverageReport::compute_percentages` is private |

---

## Candidate Flags

| # | Path | Lines | Kind | Evidence | Why | Disposition | Confidence | Blast Radius |
|---|------|-------|------|----------|-----|-------------|------------|--------------|
| 1 | `discovery/mod.rs` | 215–218 `RepoChecks.prepare_db` | dubious-feature | Only set when `prepare-db:check` script exists in root `package.json`; resolved in `resolve_repo_checks()` (line 602–614); consumed in `provider/repo.rs` line 63–95 (`run_repo_command`). Field is public. | Field represents a SoloDawn-specific pre-check script hook. It is used in `repo.rs`. Not dead. | keep | high | Removing would break `repo` provider for repos that declare `prepare-db:check` |
| 2 | `discovery/mod.rs` | 562–569 `NodeManifest::dependency_names()` private method | redundant | Private method; only called at line 732 in `build_js_target`. One call site. | Not dead — essential for populating `JsTarget.dependency_names` used in dep-graph expansion and `frontend.rs` undeclared-package detection. | keep | high | Removing would break dependency propagation |
| 3 | `analysis/coverage_parser.rs` | 168–180 `extract_attr_f64`/`extract_attr_u64` (private helpers) | bug | Each call compiles a `Regex::new(...)` from scratch. Both are called multiple times inside `parse_cobertura` (6 attributes). No `once_cell`/`lazy_static` caching. | Performance issue only — these are runtime-compiled regexes on each invocation of `parse_cobertura`. Coverage parsing is one-shot per run, so impact is minor. Not dead. | refactor | medium | Refactor to `once_cell::sync::Lazy` or simple string-search; no callers outside this file |
| 4 | `discovery/mod.rs` | 277–286 `JsTarget::contains_relative_path()` marked `pub(crate)` | stub | `pub(crate)` visibility — only used internally by `applicable_js_targets` (line 490). Not part of the public API. | Not dead; correctly scoped as crate-internal. | keep | high | None if left as-is |
| 5 | `analysis/mod.rs` `collect_files_recursive` | dead | Private fn; only reachable from `collect_files`. No direct external callers. | Pure internal recursion helper; essential for `collect_files` to work. Not dead. | keep | high | Would break `collect_files` |

---

## Invisible Features

| Name | What It Does | Seems Used | User Visible | Note |
|------|-------------|------------|--------------|------|
| Anti-stub bypass gate | `is_stub_script_body()` + `pick_real_script()` detect `echo ... && exit 0` bypasses inserted by a coder agent and prefer stronger `quality:*`/`*:ci` aliases | Yes — called inside `resolve_js_capabilities` which runs on every discovery | No (internal) | Security-relevant: prevents a rogue code-writing agent from escaping quality gates by overwriting scripts with stubs |
| Windows PATH-scan shim | `resolve_node_exe()` on Windows walks `PATH` env var trying `.cmd`/`.exe`/bare forms before falling back to `{name}.cmd`; `is_explicit_command_path` guards absolute/separator paths | Yes — called by `exec_command`, `resolve_node_command`, and `services/orchestrator/agent.rs` | No | R5 Fix 6 v3: prevents `CreateProcessW` silent-fail on bare `npm` |
| pnpm-workspace.yaml merge | `read_pnpm_workspace_patterns()` merges pnpm's separate YAML with `package.json "workspaces"` field and deduplicates | Yes — called in `read_workspace_pattern_strings` for every discovery | No | Supports both npm and pnpm monorepo layouts simultaneously |
| Dependency-graph expansion | `build_js_dependents()` builds a reverse dep-graph among workspace packages by name; `applicable_js_targets()` BFS-expands affected targets to include downstream consumers | Yes — used in `frontend.rs` and `builtin_frontend.rs` | No (internal optimization) | Avoids running quality checks only on directly changed packages while missing consumers |
| `has_subdirectory_js_manifests` flag | Signals that `package.json` files exist in subdirs even though repo root has none — used by engine to report JS code presence without active targets | Yes — consumed by `engine.rs` line 297 | No | Handles `knowledge-base-app/`-style layouts |
| lcov + Cobertura auto-detection | `detect_and_parse()` selects parser by extension then content sniff | Yes — called by `provider/coverage.rs` | No (internal) | Supports both Rust (tarpaulin → Cobertura) and JS (vitest/jest → lcov) coverage formats transparently |

---

## In-Flight Work Relevance

- **Quality Gate System (G1 concern)**: `RepositoryDiscovery` is the first stage of the three-layer Quality Gate pipeline. The anti-stub bypass gate (`is_stub_script_body`) directly protects the Quality Gate System A from script-level sabotage.
- **No "open in external IDE" feature** present in this scope.
- **No VS Code webview bridge** in this scope.
- **`RepoChecks.generate_types` / `RepoChecks.prepare_db`** are the discovery-side representation of QualityGateConfig repo-level pre-checks — relevant to ongoing quality gate streamlining work.
- **`coverage_parser`** feeds `provider/coverage.rs` which maps to a quality gate condition; any change to `CoverageReport` fields would ripple to gate evaluation.
