# Census: rs-quality-rules-common

Unit: `crates/quality/src/rules/common/` + `crates/quality/src/rules/mod.rs`

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `rules/mod.rs` | Declares the three rule traits (`Rule`, `RustRule`, `TsRule`, `CommonRule`) and shared types (`RuleConfig`, `RustAnalysisContext`, `TsAnalysisContext`, `CommonAnalysisContext`) | Traits: `Rule`, `RustRule`, `TsRule`, `CommonRule`; Structs: `RuleConfig`, `*AnalysisContext`; Helper methods: `get_param_i64/usize/f64/bool` | Imported by every rule file and all three provider modules (`builtin_common.rs`, `builtin_rust.rs`, `builtin_frontend.rs`) | Foundation layer; no logic, pure interface |
| `rules/common/mod.rs` | Re-exports all 7 common rule modules and exposes `all_common_rules()` collector | `pub fn all_common_rules() -> Vec<Box<dyn CommonRule>>` | Called exclusively by `crates/quality/src/provider/builtin_common.rs:86` | Single caller; no overrides or plugin hooks |
| `duplication.rs` | Detects duplicated code blocks within a file via rolling-window line hashing | `struct DuplicationRule` (impl `CommonRule`) | Called via `all_common_rules()`; its issues counted as `MetricKey::DuplicatedBlocks` in provider; gate conditions: `duplicated_blocks GT 5` (branch), `GT 3` (repo) | Only detects within-file duplication; cross-file is not covered |
| `encoding.rs` | Flags non-UTF-8 files and BOM presence | `struct EncodingRule` (impl `CommonRule`) | Called via `all_common_rules()`; contributes to `BuiltinCommonIssues` aggregate | Issues counted in `builtin_common_issues` gate condition (repo: `GT 5`) |
| `large_file.rs` | Flags files exceeding line/byte threshold (1000 lines or 1 MB) | `struct LargeFileRule` (impl `CommonRule`) | Called via `all_common_rules()`; contributes to `BuiltinCommonIssues` | No dedicated metric key; folded into aggregate |
| `line_length.rs` | Flags lines exceeding 120 chars; skips URL/import lines; emits single issue per file with violation count | `struct LineLengthRule` (impl `CommonRule`) | Called via `all_common_rules()`; contributes to `BuiltinCommonIssues` | Emits a single aggregate issue per file, not per-line |
| `secret_detection.rs` | Scans line-by-line for 12 regex patterns (AWS keys, passwords, GitHub tokens, Stripe keys, DB URLs, etc.); skips `.env.example`, test paths, placeholder-marker lines | `struct SecretDetectionRule` (impl `CommonRule`, `Default`) | Called via `all_common_rules()`; issues counted as `MetricKey::SecretsDetected`; gate condition: `secrets_detected GT 0` at all three gates — hard block | Highest-severity rule (Blocker); orchestrator agent also references `common:secret-detection` at line 11732 |
| `trailing_whitespace.rs` | Counts lines with trailing spaces/tabs; emits single issue per file | `struct TrailingWhitespaceRule` (impl `CommonRule`) | Called via `all_common_rules()`; contributes to `BuiltinCommonIssues` | Info severity; purely cosmetic |
| `weak_default_detection.rs` | Detects hardcoded weak credential defaults (Docker Compose `admin`, JS `|| "dev_secret"`, weak JWT secrets, YAML password fields) via 5 patterns | `struct WeakDefaultDetectionRule` (impl `CommonRule`, `Default`) | Called via `all_common_rules()`; contributes to `BuiltinCommonIssues` only; **no dedicated metric key** | **BUG**: Only scans `.yml/.yaml/.env/.js/.ts/.json/.toml` but `BuiltinCommonProvider` feeds it only `.rs/.ts/.js/.tsx/.jsx` files — `.yml`, `.yaml`, `.env`, `.json`, `.toml` are never provided. Rule is effectively dead for Docker Compose / .env targets. |

## Candidates for Keep/Cut Decision

### Bug: `weak_default_detection.rs` — file-type scope mismatch

- **Path**: `crates/quality/src/rules/common/weak_default_detection.rs`
- **Kind**: bug
- **Evidence**: `BuiltinCommonProvider::files_for_scope` calls `is_rust_or_ts_file()` which only accepts `.rs/.ts/.js/.tsx/.jsx`. The `WeakDefaultDetectionRule::analyze` explicitly checks `ctx.file_path.ends_with(".yml") || ... .yaml || .env || .json || .toml` before doing any work. These file sets do not intersect for the rule's primary targets (Docker Compose, .env). The JS/TS pattern (fallback `|| "dev_secret"`) would fire on `.js`/`.ts` files, but Docker Compose and raw `.env`/`.yaml`/`.json`/`.toml` targets never reach the rule.
- **Why**: The rule was designed for infra config files but the provider was scoped only to source-code files. The infra/config file types that the rule's Docker Compose and JWT-secret patterns target are completely excluded from the scan.
- **Disposition Hint**: refactor — either extend `BuiltinCommonProvider` to also feed `.yml/.yaml/.env/.json/.toml` files, or (if intentional JS-only) remove the `.yml`/`.yaml`/`.env`/`.toml` arms from the rule's `is_relevant` check.
- **Confidence**: high
- **Blast Radius**: If refactored to feed the missing file types, the rule activates for Docker Compose and .env files in production. If the non-JS arms are removed instead, only the JS/TS `|| "dev_secret"` pattern remains.

### Note: `common:weak-default-detection` has no dedicated `MetricKey`

- **Path**: `crates/quality/src/provider/builtin_common.rs` (lines 137-145)
- **Kind**: dubious-feature
- **Evidence**: Only `common:duplication` and `common:secret-detection` are extracted into dedicated metric keys (`DuplicatedBlocks`, `SecretsDetected`). Issues from `WeakDefaultDetectionRule`, `LineLengthRule`, `LargeFileRule`, `TrailingWhitespaceRule`, and `EncodingRule` all go into the `BuiltinCommonIssues` aggregate. The `quality-gate.yaml` conditions reference `secrets_detected` (hard block at all gates) and `duplicated_blocks`, but `weak-default-detection` (Blocker/Critical severity) is only gated via the lenient `builtin_common_issues GT 5` at repo level. This means a weak JWT secret in `.env` (if the file-type bug were fixed) would not trigger a hard block.
- **Why**: Security-significant rules (`WeakDefaultDetectionRule`) should have their own metric key to allow zero-tolerance gating.
- **Disposition Hint**: refactor
- **Confidence**: medium (depends on intent: if weak defaults in `.env` are meant to be caught by the existing `secret_detection` rule as a fallback, the gap may be tolerated)
- **Blast Radius**: Adding a dedicated metric key and gate condition would tighten security posture; removing or keeping as-is leaves a gap for weak credential defaults vs hardcoded secrets.

## Invisible Features

- **`WeakDefaultDetectionRule` JS/TS fallback pattern**: The pattern matching `|| "dev_secret"` / `|| "changeme"` in JS/TS files does actually fire on `.js`/`.ts` files through the existing provider scope. This is a partially functional but underdocumented security check — it catches runtime credential fallbacks in application code but not in infra configs.

## In-Flight Relevance

- **Quality Gate System (layer A)**: All 7 common rules feed into the gate via `BuiltinCommonProvider`. The `secrets_detected` and `duplicated_blocks` metrics have explicit gate conditions. The `builtin_common_issues` aggregate gates at the repo level only (threshold 5), covering the remaining rules (encoding, large-file, line-length, trailing-whitespace, weak-default).
- **`WeakDefaultDetectionRule` gap**: Relevant to the `refactor/streamline-quality-gates` branch — if gate rules are being tightened, the file-type mismatch bug is an active correctness issue.
