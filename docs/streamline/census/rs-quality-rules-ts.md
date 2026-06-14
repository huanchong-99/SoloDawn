# Module Map: rs-quality-rules-ts

Scope: `crates/quality/src/rules/typescript/` (12 files)
Branch: `refactor/streamline-quality-gates`

## Module Table

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `mod.rs` | Sub-module re-exporter + shared utilities | `pub fn count_structural_braces(line: &str) -> (usize, usize)`; `pub fn all_ts_rules() -> Vec<Box<dyn TsRule>>` | Called by `crates/quality/src/provider/builtin_frontend.rs` (only production caller of `all_ts_rules`). `count_structural_braces` used internally by complexity, function_length, nesting_depth, react_hooks. | Central registry for all 11 rules; missing `todo_comments` from `all_ts_rules` — see candidates. |
| `any_usage.rs` | Detect `: any`, `as any`, `<any>`, `any[]` patterns in TS | `pub struct AnyUsageRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Declares its own `is_comment_line(line, in_block_comment: &mut bool)` that tracks block comments across lines. | Stateful block-comment tracker is more correct than the simpler version in console_usage. |
| `complexity.rs` | Cyclomatic complexity per function via regex + brace counting | `pub struct ComplexityRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Uses `count_structural_braces` from mod.rs. Contains private `extract_function_name` and `find_closing_brace`. | Threshold param `"threshold"` defaults to 15. `&&`/`\|\|`/`??` chains on same line over-count vs strict McCabe (noted in comments). |
| `console_usage.rs` | Detect `console.*()` calls in production code | `pub struct ConsoleUsageRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Owns private `is_test_file(path)` and `is_comment_line(line)`. | `is_comment_line` here is simpler than `any_usage.rs` version (no block-comment state). |
| `file_length.rs` | Flag TS/JS files exceeding max line count | `pub struct FileLengthRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. No shared helpers. | Default max 400 lines, configurable via `"max_lines"` param. |
| `function_length.rs` | Flag functions exceeding max line count | `pub struct FunctionLengthRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Uses `count_structural_braces`. Three regexes: fn_decl, arrow_fn, method. | Default max 50 lines. Private `FunctionInfo` struct (not public). |
| `import_order.rs` | Enforce canonical import group ordering (Node builtins > external > internal alias > relative) | `pub struct ImportOrderRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Contains private `ImportGroup` enum + `classify_import`. `NODE_BUILTINS` constant array (35 names). | Severity `Info`. Only checks group ordering, not alphabetical order within groups. |
| `naming.rs` | Enforce camelCase for functions, PascalCase for class/interface/type/enum | `pub struct NamingConventionRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Private `is_camel_case` and `is_pascal_case` helpers. | UPPER_SNAKE_CASE constants exempted. React PascalCase components exempted in `.tsx`/`.jsx`. |
| `nesting_depth.rs` | Flag control-flow nesting exceeding max depth | `pub struct NestingDepthRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Uses `count_structural_braces`. | Default max depth 4. Uses `HashSet` to prevent duplicate reports per line. |
| `react_hooks.rs` | Detect React hooks called inside conditionals, loops, or after early return | `pub struct ReactHooksRule` (impl `Rule` + `TsRule`, `#[derive(Default)]`) | Registered in `all_ts_rules()`. Uses `count_structural_braces`. Only runs on `.tsx`/`.jsx` files. | Severity `Critical` (bug, not code smell). Most stateful rule — tracks brace depths, conditional/loop stacks, function scope. |
| `todo_comments.rs` | Detect TODO/FIXME/HACK/XXX comment markers | `pub struct TodoCommentsRule` (impl `Rule` + `TsRule`) | **NOT registered in `all_ts_rules()`**. Has standalone `Default` impl. | Rule exists, has tests, but is NOT included in the `all_ts_rules()` factory function. Dead production code. |
| `type_assertion.rs` | Detect `as Type` and `<Type>value` assertions bypassing type safety | `pub struct TypeAssertionRule` (impl `Rule` + `TsRule`) | Registered in `all_ts_rules()`. Owns three compiled `Regex` fields. Skips import/export renames and `.tsx` angle brackets. | `as const` explicitly exempted. |

## Candidates

| File | Kind | Evidence | Disposition | Confidence |
|------|------|----------|-------------|------------|
| `todo_comments.rs` | dead | `TodoCommentsRule` not in `all_ts_rules()` list (mod.rs L53-65); only callers are its own unit tests. No production path reaches it. | delete or refactor (add to `all_ts_rules`) | high |
| `any_usage.rs` `is_comment_line` vs `console_usage.rs` `is_comment_line` | redundant | Two separate private `is_comment_line` functions with different signatures. `any_usage` version tracks block-comment state across lines (more correct); `console_usage` version is stateless. Divergent behavior. | refactor (extract shared utility to mod.rs) | medium |
| `complexity.rs` `find_closing_brace` | redundant | Private helper that duplicates brace-tracking logic already in `count_structural_braces` + inline loops in function_length, nesting_depth, react_hooks. All 4 implementations iterate lines counting brace depths. | investigate (consolidate into mod.rs or a shared helper) | medium |

## Invisible Features

- **`builtin_frontend_critical` gate condition**: The `ReactHooksRule` is the only rule in this module with `RuleType::Bug` and `Severity::Critical`. It directly feeds the `builtin_frontend_critical` metric which gates both the terminal gate and PR gate. A critical hooks violation blocks merging.
- **`builtin_frontend_issues` tolerance**: Branch gate allows up to 10 frontend issues (`GT 10`), meaning minor/major rule hits have a tolerance window. Terminal gate blocks on any critical.
- **`TodoCommentsRule` orphan**: Fully implemented rule with 6 test cases, matching TODO/FIXME/HACK/XXX case-insensitively, including string-literal exclusion. Completely unreachable from production because `all_ts_rules()` doesn't include it.

## toolNotes

fast-context MCP returned `resource_exhausted` on first call; all cross-file analysis fell back to Grep.
