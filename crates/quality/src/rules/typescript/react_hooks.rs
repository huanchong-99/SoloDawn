//! React Hooks usage rules for TypeScript/JSX files.
//!
//! Detects violations of the Rules of Hooks:
//! - Hooks called inside conditionals (if/else/for/while)
//! - Hooks called inside loops
//! - Hooks called after early returns

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext};
use regex::Regex;

/// Built-in rule that checks React hooks usage rules in TypeScript/JSX files.
#[derive(Debug, Default)]
pub struct ReactHooksRule;

impl Rule for ReactHooksRule {
    fn id(&self) -> &str {
        "ts:react-hooks"
    }

    fn name(&self) -> &str {
        "React Hooks Rules"
    }

    fn description(&self) -> &str {
        "Checks that React hooks are not called conditionally, inside loops, \
         or after early returns. Hooks must be called in the same order on every render."
    }

    fn rule_type(&self) -> RuleType {
        RuleType::Bug
    }

    fn default_severity(&self) -> Severity {
        Severity::Critical
    }
}

impl TsRule for ReactHooksRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        // Only analyze .tsx and .jsx files
        if !ctx.file_path.ends_with(".tsx") && !ctx.file_path.ends_with(".jsx") {
            return Vec::new();
        }

        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let hook_re = Regex::new(r"use[A-Z]\w*\(").expect("invalid hook regex");
        let conditional_kw_re =
            Regex::new(r"\b(if|else\s+if|else)\b").expect("invalid conditional keyword regex");
        let loop_kw_re =
            Regex::new(r"\b(for|while)\s*\(").expect("invalid loop keyword regex");
        let return_re = Regex::new(r"\breturn\b").expect("invalid return regex");

        let mut issues = Vec::new();

        // Track state while scanning lines
        let mut conditional_depth: i32 = 0; // depth of braces inside conditionals
        let mut in_conditional_header = false;
        let mut loop_depth: i32 = 0;
        let mut in_loop_header = false;
        let mut had_early_return = false;
        let mut brace_depth: i32 = 0;
        // Track the brace depth at which we entered a conditional or loop
        let mut conditional_start_depths: Vec<i32> = Vec::new();
        let mut loop_start_depths: Vec<i32> = Vec::new();
        // Component/function scope depth to reset early return tracking
        let mut function_scope_depth: Option<i32> = None;

        for (line_idx, line) in ctx.lines.iter().enumerate() {
            let line_num = (line_idx + 1) as u32;
            let trimmed = line.trim();

            // Skip comments and empty lines
            if trimmed.starts_with("//") || trimmed.starts_with('*') || trimmed.is_empty() {
                continue;
            }

            // Detect function/component boundaries to scope early return tracking
            if (trimmed.contains("function ") || trimmed.contains("=> {") || trimmed.contains("=>{"))
                && function_scope_depth.is_none() {
                    function_scope_depth = Some(brace_depth);
                    had_early_return = false;
                }

            // Detect conditional keywords
            if conditional_kw_re.is_match(trimmed) && !trimmed.starts_with("//") {
                in_conditional_header = true;
            }

            // Detect loop keywords
            if loop_kw_re.is_match(trimmed) && !trimmed.starts_with("//") {
                in_loop_header = true;
            }

            // Count braces to track depth
            for ch in trimmed.chars() {
                if ch == '{' {
                    brace_depth += 1;
                    if in_conditional_header {
                        in_conditional_header = false;
                        conditional_depth += 1;
                        conditional_start_depths.push(brace_depth);
                    }
                    if in_loop_header {
                        in_loop_header = false;
                        loop_depth += 1;
                        loop_start_depths.push(brace_depth);
                    }
                } else if ch == '}' {
                    // Check if we're leaving a conditional block
                    if let Some(&start) = conditional_start_depths.last() {
                        if brace_depth == start {
                            conditional_start_depths.pop();
                            conditional_depth -= 1;
                        }
                    }
                    // Check if we're leaving a loop block
                    if let Some(&start) = loop_start_depths.last() {
                        if brace_depth == start {
                            loop_start_depths.pop();
                            loop_depth -= 1;
                        }
                    }
                    // Check if we're leaving the function scope
                    if let Some(fn_depth) = function_scope_depth {
                        if brace_depth == fn_depth + 1 {
                            function_scope_depth = None;
                            had_early_return = false;
                        }
                    }
                    brace_depth -= 1;
                }
            }

            // Detect early returns (return statements not at the end of a function)
            if return_re.is_match(trimmed) && conditional_depth > 0 {
                had_early_return = true;
            }

            // Check for hook calls
            if let Some(mat) = hook_re.find(trimmed) {
                let hook_name = &trimmed[mat.start()..mat.end() - 1]; // exclude '('

                if conditional_depth > 0 {
                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        severity,
                        crate::rule::AnalyzerSource::Other("built-in".to_string()),
                        format!(
                            "React hook `{}` is called inside a conditional block. \
                             Hooks must be called in the same order on every render.",
                            hook_name
                        ),
                    )
                    .with_location(ctx.file_path, line_num)
                    .with_effort(15);
                    issues.push(issue);
                } else if loop_depth > 0 {
                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        severity,
                        crate::rule::AnalyzerSource::Other("built-in".to_string()),
                        format!(
                            "React hook `{}` is called inside a loop. \
                             Hooks must be called in the same order on every render.",
                            hook_name
                        ),
                    )
                    .with_location(ctx.file_path, line_num)
                    .with_effort(15);
                    issues.push(issue);
                } else if had_early_return {
                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        severity,
                        crate::rule::AnalyzerSource::Other("built-in".to_string()),
                        format!(
                            "React hook `{}` is called after an early return. \
                             Hooks must be called in the same order on every render.",
                            hook_name
                        ),
                    )
                    .with_location(ctx.file_path, line_num)
                    .with_effort(15);
                    issues.push(issue);
                }
            }
        }

        issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_tsx(source: &str) -> Vec<QualityIssue> {
        let lines: Vec<&str> = source.lines().collect();
        let config = RuleConfig::default();
        let ctx = TsAnalysisContext {
            file_path: "component.tsx",
            content: source,
            lines: &lines,
            config: &config,
        };
        let rule = ReactHooksRule;
        rule.analyze(&ctx)
    }

    fn analyze_with_path(source: &str, path: &str) -> Vec<QualityIssue> {
        let lines: Vec<&str> = source.lines().collect();
        let config = RuleConfig::default();
        let ctx = TsAnalysisContext {
            file_path: path,
            content: source,
            lines: &lines,
            config: &config,
        };
        let rule = ReactHooksRule;
        rule.analyze(&ctx)
    }

    #[test]
    fn hook_inside_conditional_is_flagged() {
        let source = r#"
function MyComponent({ show }) {
    if (show) {
        const [val, setVal] = useState(0);
    }
    return <div />;
}
"#;
        let issues = analyze_tsx(source);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("useState"));
        assert!(issues[0].message.contains("conditional"));
    }

    #[test]
    fn hook_inside_loop_is_flagged() {
        let source = r#"
function MyComponent({ items }) {
    for (let i = 0; i < items.length; i++) {
        const ref = useRef(null);
    }
    return <div />;
}
"#;
        let issues = analyze_tsx(source);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("useRef"));
        assert!(issues[0].message.contains("loop"));
    }

    #[test]
    fn valid_hooks_at_top_level_no_issues() {
        let source = r#"
function MyComponent() {
    const [count, setCount] = useState(0);
    const ref = useRef(null);
    useEffect(() => {
        console.log(count);
    }, [count]);
    return <div>{count}</div>;
}
"#;
        let issues = analyze_tsx(source);
        assert!(issues.is_empty(), "Top-level hooks should produce no issues");
    }

    #[test]
    fn non_tsx_file_is_skipped() {
        let source = r#"
function something() {
    if (true) {
        useState(0);
    }
}
"#;
        let issues = analyze_with_path(source, "file.ts");
        assert!(issues.is_empty(), "Non-tsx/jsx files should be skipped");
    }

    #[test]
    fn jsx_file_is_analyzed() {
        let source = r#"
function MyComponent() {
    if (true) {
        useState(0);
    }
}
"#;
        let issues = analyze_with_path(source, "component.jsx");
        assert_eq!(issues.len(), 1);
    }
}
