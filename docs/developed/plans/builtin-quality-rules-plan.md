# Built-in Quality Rules Plan

> **Status**: Implemented | **Date**: 2026-03-16 | **Completed**: 2026-03-17
> **Objective**: Replace all external service dependencies with fully built-in quality analysis rules,
> enabling the quality gate to run completely offline during AI CLI terminal checkpoint reviews.

---

## 1. Executive Summary

SoloDawn's quality gate currently has a solid **framework** (ported from SonarQube's architecture) but
delegates ALL actual code analysis to external tools (cargo clippy CLI, ESLint CLI, SonarQube server).
This plan adds **fully built-in static analysis rules** in Rust, covering:

- **13 Rust rules** (complexity, style, safety, maintainability)
- **11 TypeScript/React rules** (complexity, type safety, patterns)
- **6 language-agnostic common rules** (duplication, secrets, encoding)
- **4 new providers** (BuiltinRust, BuiltinFrontend, BuiltinCommon, Coverage)
- **1 coverage parser** (lcov/cobertura/tarpaulin)

Total: **35 new source files** + infrastructure updates, developed by **35 parallel agents**.

---

## 2. Core Design Principle

> **The quality gate must be fully self-contained.** When an AI CLI terminal reaches a checkpoint,
> the orchestrator triggers quality analysis that runs LOCALLY using BUILT-IN rules, without needing
> any external service (no SonarQube, no network). The existing external tool providers (clippy, ESLint)
> remain as complementary checks but are no longer the sole source of analysis.

---

## 3. Architecture

### 3.1 New Module Structure

```
crates/quality/src/
├── lib.rs                          # Add: mod rules, mod analysis
├── rules/
│   ├── mod.rs                      # Rule trait + RustRule/TsRule/CommonRule traits + registry
│   ├── rust/
│   │   ├── mod.rs                  # pub mod + all_rust_rules() collector
│   │   ├── cyclomatic_complexity.rs
│   │   ├── cognitive_complexity.rs
│   │   ├── function_length.rs
│   │   ├── file_length.rs
│   │   ├── nesting_depth.rs
│   │   ├── error_handling.rs
│   │   ├── unsafe_usage.rs
│   │   ├── clone_usage.rs
│   │   ├── naming.rs
│   │   ├── documentation.rs
│   │   ├── type_complexity.rs
│   │   ├── todo_comments.rs
│   │   └── magic_numbers.rs
│   ├── typescript/
│   │   ├── mod.rs                  # pub mod + all_ts_rules() collector
│   │   ├── complexity.rs
│   │   ├── function_length.rs
│   │   ├── file_length.rs
│   │   ├── nesting_depth.rs
│   │   ├── any_usage.rs
│   │   ├── type_assertion.rs
│   │   ├── console_usage.rs
│   │   ├── naming.rs
│   │   ├── react_hooks.rs
│   │   ├── import_order.rs
│   │   └── todo_comments.rs
│   └── common/
│       ├── mod.rs                  # pub mod + all_common_rules() collector
│       ├── duplication.rs
│       ├── secret_detection.rs
│       ├── large_file.rs
│       ├── line_length.rs
│       ├── trailing_whitespace.rs
│       └── encoding.rs
├── analysis/
│   ├── mod.rs                      # Shared utilities
│   └── coverage_parser.rs          # lcov/cobertura/tarpaulin parsing
└── provider/
    ├── ... (existing providers unchanged)
    ├── builtin_rust.rs             # NEW provider: built-in Rust rules
    ├── builtin_frontend.rs         # NEW provider: built-in TS rules
    ├── builtin_common.rs           # NEW provider: built-in common rules
    └── coverage.rs                 # NEW provider: coverage report parsing
```

### 3.2 Rule Trait Definition

```rust
/// Core trait for all built-in quality rules
pub trait Rule: Send + Sync {
    /// Unique rule ID (e.g., "rust:S1854", "ts:complexity")
    fn id(&self) -> &str;
    /// Human-readable rule name
    fn name(&self) -> &str;
    /// Rule description
    fn description(&self) -> &str;
    /// Rule type classification
    fn rule_type(&self) -> RuleType;
    /// Default severity when rule triggers
    fn default_severity(&self) -> Severity;
    /// Default threshold configuration
    fn default_config(&self) -> RuleConfig;
}

/// Rust-specific rule (operates on syn AST)
pub trait RustRule: Rule {
    /// Analyze a parsed Rust file
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue>;
}

/// TypeScript-specific rule (operates on source text)
pub trait TsRule: Rule {
    /// Analyze a TypeScript/JavaScript source file
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue>;
}

/// Language-agnostic rule (operates on raw file content)
pub trait CommonRule: Rule {
    /// Analyze any file
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue>;
}

/// Analysis context for Rust files
pub struct RustAnalysisContext<'a> {
    pub file_path: &'a str,
    pub content: &'a str,
    pub syntax: &'a syn::File,
}

/// Analysis context for TypeScript files
pub struct TsAnalysisContext<'a> {
    pub file_path: &'a str,
    pub content: &'a str,
    pub lines: Vec<&'a str>,
}

/// Analysis context for common rules
pub struct CommonAnalysisContext<'a> {
    pub file_path: &'a str,
    pub content: &'a [u8],
    pub is_text: bool,
}

/// Per-rule configuration
#[derive(Debug, Clone)]
pub struct RuleConfig {
    pub enabled: bool,
    pub severity_override: Option<Severity>,
    pub params: HashMap<String, String>,
}
```

### 3.3 Provider Integration

Each new provider:
1. Scans project files matching its language
2. Instantiates all registered rules
3. Runs rules against each file
4. Collects `QualityIssue` results
5. Aggregates into `MetricKey` measurements
6. Returns `ProviderReport`

The `QualityEngine` spawns new providers alongside existing ones (all concurrent).

---

## 4. Rule Specifications

### 4.1 Rust Rules (13 rules)

| # | Rule ID | Name | Type | Severity | Threshold | Description |
|---|---------|------|------|----------|-----------|-------------|
| 1 | `rust:cyclomatic-complexity` | Cyclomatic Complexity | CodeSmell | Major | 15 | Count decision points (if/match/while/for/&&/||) per function |
| 2 | `rust:cognitive-complexity` | Cognitive Complexity | CodeSmell | Major | 20 | Weighted nesting-aware complexity per function |
| 3 | `rust:function-length` | Function Length | CodeSmell | Major | 60 lines | Max lines per fn/method body |
| 4 | `rust:file-length` | File Length | CodeSmell | Minor | 500 lines | Max lines per source file |
| 5 | `rust:nesting-depth` | Nesting Depth | CodeSmell | Major | 5 | Max nested control flow levels |
| 6 | `rust:error-handling` | Error Handling | Bug | Critical | 0 | Detect unwrap()/expect()/panic!() in non-test code |
| 7 | `rust:unsafe-usage` | Unsafe Usage | SecurityHotspot | Major | report | Detect unsafe blocks for review |
| 8 | `rust:clone-usage` | Clone Usage | CodeSmell | Minor | report | Detect potentially unnecessary .clone() calls |
| 9 | `rust:naming` | Naming Convention | CodeSmell | Minor | 0 | snake_case fn/var, CamelCase types, UPPER_SNAKE consts |
| 10 | `rust:documentation` | Missing Documentation | CodeSmell | Minor | report | Missing doc comments on pub items |
| 11 | `rust:type-complexity` | Type Complexity | CodeSmell | Major | 5 | Deeply nested generic types (e.g., `Arc<Mutex<Vec<...>>>`) |
| 12 | `rust:todo-comments` | TODO/FIXME Comments | CodeSmell | Info | report | Track TODO/FIXME/HACK/XXX comments |
| 13 | `rust:magic-numbers` | Magic Numbers | CodeSmell | Minor | 0 | Numeric literals outside const/config (except 0, 1, 2) |

### 4.2 TypeScript Rules (11 rules)

| # | Rule ID | Name | Type | Severity | Threshold | Description |
|---|---------|------|------|----------|-----------|-------------|
| 14 | `ts:complexity` | Complexity | CodeSmell | Major | 15 | Cyclomatic complexity per function/method |
| 15 | `ts:function-length` | Function Length | CodeSmell | Major | 50 lines | Max lines per function/arrow function |
| 16 | `ts:file-length` | File Length | CodeSmell | Minor | 400 lines | Max lines per source file |
| 17 | `ts:nesting-depth` | Nesting Depth | CodeSmell | Major | 4 | Max nested control flow levels |
| 18 | `ts:any-usage` | Any Type Usage | CodeSmell | Major | 0 | Detect `: any`, `as any`, `<any>` type annotations |
| 19 | `ts:type-assertion` | Type Assertion | CodeSmell | Minor | report | Detect `as Type` / `<Type>` assertions (prefer type guards) |
| 20 | `ts:console-usage` | Console Usage | CodeSmell | Minor | 0 | Detect console.log/warn/error in production code |
| 21 | `ts:naming` | Naming Convention | CodeSmell | Minor | 0 | camelCase vars/fns, PascalCase components/types |
| 22 | `ts:react-hooks` | React Hooks Rules | Bug | Critical | 0 | Hooks in conditionals/loops, missing deps patterns |
| 23 | `ts:import-order` | Import Order | CodeSmell | Info | report | Group: builtin > external > internal > relative |
| 24 | `ts:todo-comments` | TODO/FIXME Comments | CodeSmell | Info | report | Track TODO/FIXME comments |

### 4.3 Common Rules (6 rules)

| # | Rule ID | Name | Type | Severity | Threshold | Description |
|---|---------|------|------|----------|-----------|-------------|
| 25 | `common:duplication` | Code Duplication | CodeSmell | Major | 10 lines | Detect duplicated code blocks across files |
| 26 | `common:secret-detection` | Secret Detection | Vulnerability | Blocker | 0 | Detect API keys, tokens, passwords in source |
| 27 | `common:large-file` | Large File | CodeSmell | Minor | 1000 lines | Warn on excessively large files |
| 28 | `common:line-length` | Line Length | CodeSmell | Info | 120 chars | Max characters per line |
| 29 | `common:trailing-whitespace` | Trailing Whitespace | CodeSmell | Info | report | Detect trailing whitespace |
| 30 | `common:encoding` | File Encoding | Bug | Major | 0 | Detect non-UTF-8 / BOM files |

### 4.4 Coverage Provider (1 provider)

| # | Rule ID | Name | Type | Description |
|---|---------|------|------|-------------|
| - | `coverage:parser` | Coverage Report Parser | Metric | Parse lcov, cobertura XML, tarpaulin JSON locally |

---

## 5. New Metrics

Added to `MetricKey` enum:

```rust
// Built-in Rust analysis metrics
BuiltinRustIssues,          // Total issues from built-in Rust rules
BuiltinRustCritical,        // Critical+ issues from Rust rules
RustCyclomaticComplexity,   // Max cyclomatic complexity found
RustCognitiveComplexity,    // Max cognitive complexity found

// Built-in Frontend analysis metrics
BuiltinFrontendIssues,      // Total issues from built-in TS rules
BuiltinFrontendCritical,    // Critical+ issues from TS rules

// Built-in Common analysis metrics
BuiltinCommonIssues,        // Total issues from common rules
DuplicatedBlocks,           // Number of duplicated code blocks
SecretsDetected,            // Number of detected secrets

// Coverage metrics (locally parsed)
LineCoverage,               // Line coverage percentage
BranchCoverage,             // Branch coverage percentage
```

---

## 6. Updated quality-gate.yaml

```yaml
# Terminal Gate additions:
- metric: builtin_rust_critical
  operator: "GT"
  threshold: "0"
- metric: builtin_frontend_critical
  operator: "GT"
  threshold: "0"
- metric: secrets_detected
  operator: "GT"
  threshold: "0"

# Branch Gate additions:
- metric: builtin_rust_issues
  operator: "GT"
  threshold: "10"
- metric: builtin_frontend_issues
  operator: "GT"
  threshold: "10"
- metric: duplicated_blocks
  operator: "GT"
  threshold: "5"
- metric: rust_cyclomatic_complexity
  operator: "GT"
  threshold: "25"
- metric: line_coverage
  operator: "LT"
  threshold: "60"

# Repo Gate additions:
- metric: builtin_rust_issues
  operator: "GT"
  threshold: "0"
- metric: builtin_frontend_issues
  operator: "GT"
  threshold: "0"
- metric: builtin_common_issues
  operator: "GT"
  threshold: "0"
- metric: secrets_detected
  operator: "GT"
  threshold: "0"
- metric: line_coverage
  operator: "LT"
  threshold: "80"

# New providers:
providers:
  rust: true
  frontend: true
  repo: true
  security: true
  sonar: true              # Keep as optional external
  builtin_rust: true       # NEW
  builtin_frontend: true   # NEW
  builtin_common: true     # NEW
  coverage: true           # NEW
```

---

## 7. Dependencies

Add to `crates/quality/Cargo.toml`:

```toml
syn = { version = "2", features = ["full", "parsing", "visit"] }
```

No other new dependencies needed. All TypeScript and common rules use regex (already a dependency).

---

## 8. Parallel Agent Assignment (35 Agents)

### Conflict Avoidance Strategy

- **Each agent creates ONLY its own file(s)** — no shared file edits
- **Infrastructure setup** (trait definitions, mod.rs, Cargo.toml, metrics, config) is done
  BEFORE launching agents, so all agents work against a stable interface
- **Integration wiring** (updating mod.rs exports, engine registration, config) is done
  AFTER all agents complete, by a single integration pass

### Agent Table

| Agent # | Group | File(s) Created | Dependencies | Estimated LOC |
|---------|-------|-----------------|--------------|---------------|
| **A01** | Rust Rules | `rules/rust/cyclomatic_complexity.rs` | syn (visit) | ~120 |
| **A02** | Rust Rules | `rules/rust/cognitive_complexity.rs` | syn (visit) | ~150 |
| **A03** | Rust Rules | `rules/rust/function_length.rs` | syn (visit) | ~80 |
| **A04** | Rust Rules | `rules/rust/file_length.rs` | content lines | ~50 |
| **A05** | Rust Rules | `rules/rust/nesting_depth.rs` | syn (visit) | ~120 |
| **A06** | Rust Rules | `rules/rust/error_handling.rs` | syn (visit) | ~130 |
| **A07** | Rust Rules | `rules/rust/unsafe_usage.rs` | syn (visit) | ~80 |
| **A08** | Rust Rules | `rules/rust/clone_usage.rs` | syn (visit) | ~90 |
| **A09** | Rust Rules | `rules/rust/naming.rs` | syn (visit) | ~150 |
| **A10** | Rust Rules | `rules/rust/documentation.rs` | syn (visit) | ~100 |
| **A11** | Rust Rules | `rules/rust/type_complexity.rs` | syn (visit) | ~110 |
| **A12** | Rust Rules | `rules/rust/todo_comments.rs` | regex | ~70 |
| **A13** | Rust Rules | `rules/rust/magic_numbers.rs` | syn (visit) | ~100 |
| **A14** | TS Rules | `rules/typescript/complexity.rs` | regex | ~130 |
| **A15** | TS Rules | `rules/typescript/function_length.rs` | regex | ~90 |
| **A16** | TS Rules | `rules/typescript/file_length.rs` | content lines | ~50 |
| **A17** | TS Rules | `rules/typescript/nesting_depth.rs` | regex | ~100 |
| **A18** | TS Rules | `rules/typescript/any_usage.rs` | regex | ~80 |
| **A19** | TS Rules | `rules/typescript/type_assertion.rs` | regex | ~80 |
| **A20** | TS Rules | `rules/typescript/console_usage.rs` | regex | ~70 |
| **A21** | TS Rules | `rules/typescript/naming.rs` | regex | ~120 |
| **A22** | TS Rules | `rules/typescript/react_hooks.rs` | regex | ~150 |
| **A23** | TS Rules | `rules/typescript/import_order.rs` | regex | ~120 |
| **A24** | TS Rules | `rules/typescript/todo_comments.rs` | regex | ~70 |
| **A25** | Common Rules | `rules/common/duplication.rs` | hash/tokenize | ~200 |
| **A26** | Common Rules | `rules/common/secret_detection.rs` | regex | ~150 |
| **A27** | Common Rules | `rules/common/large_file.rs` | content bytes | ~50 |
| **A28** | Common Rules | `rules/common/line_length.rs` | content lines | ~60 |
| **A29** | Common Rules | `rules/common/trailing_whitespace.rs` | content lines | ~50 |
| **A30** | Common Rules | `rules/common/encoding.rs` | content bytes | ~80 |
| **A31** | Provider | `provider/builtin_rust.rs` | rules::rust::* | ~120 |
| **A32** | Provider | `provider/builtin_frontend.rs` | rules::typescript::* | ~120 |
| **A33** | Provider | `provider/builtin_common.rs` | rules::common::* | ~100 |
| **A34** | Provider | `provider/coverage.rs` | analysis::coverage_parser | ~80 |
| **A35** | Analysis | `analysis/coverage_parser.rs` | regex, serde | ~200 |

**Total: 35 agents, ~3,520 estimated LOC of new code**

### Agent Dependency Graph

```
                    ┌─────────────────────────┐
                    │  Pre-setup (by author)   │
                    │  - Rule traits           │
                    │  - Directory structure   │
                    │  - Cargo.toml update     │
                    │  - Skeleton mod.rs files │
                    └────────────┬────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
          ▼                      ▼                      ▼
   ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
   │ A01-A13      │    │ A14-A24      │    │ A25-A30      │
   │ Rust Rules   │    │ TS Rules     │    │ Common Rules │
   │ (13 agents)  │    │ (11 agents)  │    │ (6 agents)   │
   └──────┬───────┘    └──────┬───────┘    └──────┬───────┘
          │                   │                   │
          │            ┌──────┼───────┐           │
          │            │      │       │           │
          ▼            ▼      ▼       ▼           ▼
   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
   │ A31      │  │ A32      │  │ A33      │  │ A34-A35  │
   │ Rust Prov│  │ TS Prov  │  │ Cmn Prov │  │ Coverage │
   └──────────┘  └──────────┘  └──────────┘  └──────────┘
          │            │            │              │
          └────────────┼────────────┼──────────────┘
                       │
                       ▼
              ┌────────────────┐
              │ Post-integration│
              │ (by author)     │
              │ - Wire mod.rs   │
              │ - Update engine │
              │ - Update config │
              │ - Update YAML   │
              └────────────────┘
```

**NOTE**: ALL 35 agents run in parallel. The provider agents (A31-A35) reference
rule modules by path — they compile once all rule agents have completed their files.
The final integration step wires everything together.

---

## 9. Execution Phases

### Phase 0: Pre-setup (before agents)
1. Create all directories
2. Write `rules/mod.rs` with trait definitions
3. Write skeleton `rules/rust/mod.rs`, `rules/typescript/mod.rs`, `rules/common/mod.rs`
4. Write `analysis/mod.rs` with shared utilities
5. Add `syn` to `Cargo.toml`
6. Extend `MetricKey` enum with new metrics
7. Extend `ProvidersConfig` with new provider flags
8. Commit infrastructure

### Phase 1: Parallel Agent Execution (35 agents)
- All agents run simultaneously
- Each creates its assigned file(s) only
- Each follows the Rule trait contract defined in Phase 0
- Each includes unit tests in the same file (`#[cfg(test)]`)

### Phase 2: Post-integration (after all agents)
1. Update `rules/rust/mod.rs` — add pub mod for each rule, `all_rust_rules()` function
2. Update `rules/typescript/mod.rs` — add pub mod for each rule, `all_ts_rules()` function
3. Update `rules/common/mod.rs` — add pub mod for each rule, `all_common_rules()` function
4. Update `provider/mod.rs` — add new provider modules
5. Update `lib.rs` — add `pub mod rules; pub mod analysis;`
6. Update `engine.rs` — register new providers
7. Update `config.rs` — parse new provider config
8. Update `quality-gate.yaml` — add new conditions
9. Run `cargo check --workspace` and fix compilation errors
10. Run `cargo clippy` and fix warnings
11. Run `cargo test` and fix test failures
12. Commit all changes

### Phase 3: PR and CI
1. Push to branch
2. Create PR via `gh pr create`
3. Monitor CI checks
4. Fix failures and re-push until green

---

## 10. Testing Strategy

Each rule file includes its own `#[cfg(test)] mod tests`:
- Test with sample source code strings
- Verify correct issue detection (true positives)
- Verify no false positives on clean code
- Verify severity and rule_id correctness
- Verify threshold configurability

Provider tests:
- Integration test with sample project directory
- Verify metric aggregation

---

## 11. Risk Assessment

| Risk | Mitigation |
|------|-----------|
| `syn` parsing failures on malformed Rust | Catch parse errors gracefully, skip file with warning |
| Regex false positives for TS rules | Conservative patterns, allow per-rule disable |
| Agent file conflicts | Each agent works on exclusive file(s) |
| Compilation errors after integration | Single integration pass fixes all wiring |
| Performance impact on large codebases | File-level parallelism, configurable file size limits |
| Breaking existing quality gate behavior | New providers are additive, existing providers unchanged |

---

## 12. Completion Criteria

- [x] All 35 new source files created and compiling
- [x] All rules have at least 2 unit tests each (129 unit tests total)
- [x] `cargo check --workspace` passes
- [x] `cargo clippy --workspace` passes with zero warnings
- [x] `cargo test --workspace` passes
- [x] `quality-gate.yaml` updated with new conditions
- [x] New providers registered in engine
- [x] CI pipeline green on PR

---

*All 35 agents completed. No external service dependencies in any built-in rule.*
