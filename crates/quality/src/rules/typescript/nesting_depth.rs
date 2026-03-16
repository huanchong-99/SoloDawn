//! Nesting depth rule for TypeScript/JavaScript code.
//!
//! Checks that the nesting depth of control-flow structures does not exceed a configurable
//! maximum (default: 4). Deeply nested code is hard to read and maintain.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext, RuleConfig};

/// Reports lines where control-flow nesting depth exceeds a configurable maximum.
#[derive(Debug)]
pub struct NestingDepthRule {
    /// Pattern that detects the start of a nesting construct.
    nesting_pattern: Regex,
}

impl Default for NestingDepthRule {
    fn default() -> Self {
        Self {
            nesting_pattern: Regex::new(
                r"\b(?:if\s*\(|else\s*\{|for\s*\(|while\s*\(|switch\s*\(|try\s*\{|catch\s*\()"
            ).expect("invalid nesting pattern regex"),
        }
    }
}

impl Rule for NestingDepthRule {
    fn id(&self) -> &str {
        "ts:nesting-depth"
    }

    fn name(&self) -> &str {
        "Nesting Depth"
    }

    fn description(&self) -> &str {
        "Checks that control-flow nesting depth does not exceed a configurable maximum"
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

impl TsRule for NestingDepthRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let max_depth = ctx.config.get_param_usize("max_depth", 4);
        let mut issues = Vec::new();
        let mut depth: usize = 0;
        // Track which lines already had an issue reported so we report at most once per line.
        let mut reported = std::collections::HashSet::new();

        for (i, line) in ctx.lines.iter().enumerate() {
            let trimmed = line.trim();

            // Skip blank lines and pure comment lines.
            if trimmed.is_empty() || trimmed.starts_with("//") || trimmed.starts_with('*') {
                continue;
            }

            // Determine if this line contains a nesting keyword.
            let is_nesting_line = self.nesting_pattern.is_match(trimmed);

            // Process braces to track depth changes.
            for ch in trimmed.chars() {
                if ch == '{' {
                    depth += 1;
                    // Check right after incrementing — if this brace is from a nesting
                    // construct and we just exceeded the max, report it.
                    if is_nesting_line && depth > max_depth && !reported.contains(&i) {
                        let line_number = (i as u32) + 1;
                        let issue = QualityIssue::new(
                            "ts:nesting-depth",
                            RuleType::CodeSmell,
                            Severity::Major,
                            crate::rule::AnalyzerSource::Other("built-in".into()),
                            format!(
                                "Nesting depth {} exceeds maximum allowed depth of {} at line {}",
                                depth, max_depth, line_number
                            ),
                        )
                        .with_location(ctx.file_path.to_string(), line_number);
                        issues.push(issue);
                        reported.insert(i);
                    }
                } else if ch == '}' {
                    depth = depth.saturating_sub(1);
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
    fn shallow_nesting_produces_no_issues() {
        let src = r#"
function foo(x: number) {
    if (x > 0) {
        for (let i = 0; i < x; i++) {
            console.log(i);
        }
    }
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context(src, &lines, &config);
        let rule = NestingDepthRule::default();
        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "expected no issues for shallow nesting, got {}: {:?}",
            issues.len(),
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
    }

    #[test]
    fn deep_nesting_exceeds_max_depth() {
        // With max_depth=2, the third nesting level should trigger an issue.
        let src = r#"
function deep() {
    if (true) {
        if (true) {
            if (true) {
                console.log("too deep");
            }
        }
    }
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let mut config = RuleConfig::default();
        config.params.insert("max_depth".into(), "2".into());
        let ctx = make_context(src, &lines, &config);
        let rule = NestingDepthRule::default();
        let issues = rule.analyze(&ctx);
        assert!(
            !issues.is_empty(),
            "expected at least one nesting depth issue"
        );
        assert!(
            issues[0].message.contains("exceeds maximum"),
            "issue message should mention exceeding maximum, got: {}",
            issues[0].message
        );
    }

    #[test]
    fn respects_configurable_max_depth() {
        // Same deeply-nested code, but with a high threshold — should produce no issues.
        let src = r#"
function deep() {
    if (true) {
        if (true) {
            if (true) {
                if (true) {
                    if (true) {
                        console.log("very deep");
                    }
                }
            }
        }
    }
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let mut config = RuleConfig::default();
        config.params.insert("max_depth".into(), "20".into());
        let ctx = make_context(src, &lines, &config);
        let rule = NestingDepthRule::default();
        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "expected no issues with high max_depth threshold"
        );
    }
}
