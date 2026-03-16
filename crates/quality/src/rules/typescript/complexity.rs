//! Cyclomatic complexity rule for TypeScript/JavaScript functions.
//!
//! Uses regex-based analysis to find function boundaries and count decision points.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext, RuleConfig};

/// Calculates cyclomatic complexity for TypeScript/JavaScript functions
/// and reports functions that exceed a configurable threshold.
#[derive(Debug)]
pub struct ComplexityRule {
    fn_pattern: Regex,
    decision_pattern: Regex,
}

impl Default for ComplexityRule {
    fn default() -> Self {
        Self {
            fn_pattern: Regex::new(
                r"(?:function\s+\w+|=>\s*\{|(\w+)\s*\(.*\)\s*\{)"
            ).expect("invalid function pattern regex"),
            decision_pattern: Regex::new(
                r"\b(?:if|else\s+if|for|while|do|case|catch)\b|\&\&|\|\||\?\?"
            ).expect("invalid decision pattern regex"),
        }
    }
}

impl Rule for ComplexityRule {
    fn id(&self) -> &str {
        "ts:complexity"
    }

    fn name(&self) -> &str {
        "Complexity"
    }

    fn description(&self) -> &str {
        "Checks that function cyclomatic complexity does not exceed a threshold"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }

    fn default_config(&self) -> RuleConfig {
        RuleConfig::default()
    }
}

impl TsRule for ComplexityRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let threshold = ctx.config.get_param_usize("threshold", 15);
        let mut issues = Vec::new();

        // Find function start lines and their names
        let mut functions: Vec<(usize, String)> = Vec::new();
        for (i, line) in ctx.lines.iter().enumerate() {
            if let Some(m) = self.fn_pattern.find(line) {
                let matched = m.as_str();
                let name = extract_function_name(matched, line);
                functions.push((i, name));
            }
        }

        // For each function, find its body boundaries and count decision points
        for (start_line, fn_name) in &functions {
            if let Some(end_line) = find_closing_brace(ctx.lines, *start_line) {
                // Base complexity is 1
                let mut complexity: usize = 1;
                for line_idx in *start_line..=end_line {
                    let line = ctx.lines[line_idx];
                    complexity += self.decision_pattern.find_iter(line).count();
                }

                if complexity > threshold {
                    let issue = QualityIssue::new(
                        "ts:complexity",
                        RuleType::CodeSmell,
                        Severity::Major,
                        crate::rule::AnalyzerSource::Other("built-in".into()),
                        format!(
                            "Function '{}' has a cyclomatic complexity of {} (threshold: {})",
                            fn_name, complexity, threshold
                        ),
                    )
                    .with_location(ctx.file_path.to_string(), (*start_line as u32) + 1);
                    issues.push(issue);
                }
            }
        }

        issues
    }
}

/// Extract a human-readable function name from the matched text and line.
fn extract_function_name(matched: &str, line: &str) -> String {
    // "function foo" pattern
    if matched.starts_with("function") {
        return matched
            .strip_prefix("function")
            .unwrap_or("")
            .trim()
            .to_string();
    }

    // Arrow function: look for identifier before `=>`
    if matched.contains("=>") {
        let trimmed = line.trim();
        // Try to find `const name =` or `let name =` or `name =`
        let re = Regex::new(r"(?:const|let|var)\s+(\w+)").ok();
        if let Some(re) = re {
            if let Some(caps) = re.captures(trimmed) {
                return caps.get(1).map(|m| m.as_str().to_string()).unwrap_or_else(|| "<arrow>".into());
            }
        }
        return "<arrow>".into();
    }

    // Method pattern: `name(...)  {`
    let re = Regex::new(r"(\w+)\s*\(").ok();
    if let Some(re) = re {
        if let Some(caps) = re.captures(matched) {
            return caps.get(1).map(|m| m.as_str().to_string()).unwrap_or_else(|| "<anonymous>".into());
        }
    }

    "<anonymous>".into()
}

/// Find the closing brace that matches the opening brace on or after `start_line`.
fn find_closing_brace(lines: &[&str], start_line: usize) -> Option<usize> {
    let mut depth: i32 = 0;
    let mut found_open = false;

    for (i, line) in lines.iter().enumerate().skip(start_line) {
        for ch in line.chars() {
            if ch == '{' {
                depth += 1;
                found_open = true;
            } else if ch == '}' {
                depth -= 1;
            }
            if found_open && depth == 0 {
                return Some(i);
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn make_context<'a>(
        content: &'a str,
        lines: &'a [&'a str],
        config: &'a RuleConfig,
    ) -> TsAnalysisContext<'a> {
        TsAnalysisContext {
            file_path: "test.ts",
            content,
            lines,
            config,
        }
    }

    #[test]
    fn simple_function_below_threshold_produces_no_issues() {
        let src = r#"
function simple(x) {
    if (x > 0) {
        return x;
    }
    return -x;
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context(src, &lines, &config);
        let rule = ComplexityRule::default();
        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "expected no issues for simple function");
    }

    #[test]
    fn complex_function_exceeds_threshold() {
        // Build a function with many decision points to exceed threshold of 2
        let src = r#"
function complex(x) {
    if (x > 0) {
        for (let i = 0; i < x; i++) {
            while (true) {
                if (x && y || z ?? w) {
                    do {
                        switch(x) {
                            case 1:
                            case 2:
                            case 3:
                            case 4:
                            case 5:
                            case 6:
                            case 7:
                            case 8:
                            case 9:
                            case 10:
                        }
                    } while(false);
                }
            }
        }
    }
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let mut config = RuleConfig::default();
        config.params.insert("threshold".into(), "2".into());
        let ctx = make_context(src, &lines, &config);
        let rule = ComplexityRule::default();
        let issues = rule.analyze(&ctx);
        assert!(
            !issues.is_empty(),
            "expected at least one issue for complex function"
        );
        assert!(issues[0].message.contains("complex"));
    }
}
