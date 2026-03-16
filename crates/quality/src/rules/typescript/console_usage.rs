//! Console usage detection rule for TypeScript/JavaScript.
//!
//! Detects `console.log`, `console.warn`, `console.error`, and other console
//! methods that should not appear in production code.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext, RuleConfig};

/// Detects `console.*()` calls in production TypeScript/JavaScript code.
///
/// Console statements are useful during development but should be removed
/// before merging to production. This rule flags calls to `console.log`,
/// `console.warn`, `console.error`, `console.debug`, `console.info`,
/// `console.trace`, `console.dir`, and `console.table`.
#[derive(Debug)]
pub struct ConsoleUsageRule {
    console_pattern: Regex,
}

impl Default for ConsoleUsageRule {
    fn default() -> Self {
        Self {
            console_pattern: Regex::new(
                r"console\.(?:log|warn|error|debug|info|trace|dir|table)\s*\("
            )
            .expect("invalid console pattern regex"),
        }
    }
}

impl Rule for ConsoleUsageRule {
    fn id(&self) -> &str {
        "ts:console-usage"
    }

    fn name(&self) -> &str {
        "Console Usage"
    }

    fn description(&self) -> &str {
        "Detects console method calls that should not appear in production code"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }

    fn default_config(&self) -> RuleConfig {
        RuleConfig::default()
    }
}

/// Check whether a file path looks like it belongs to a test directory or file.
fn is_test_file(path: &str) -> bool {
    path.contains("__tests__") || path.contains(".test.") || path.contains(".spec.")
}

/// Check whether a line is a single-line comment (// or leading /*)
fn is_comment_line(line: &str) -> bool {
    let trimmed = line.trim();
    trimmed.starts_with("//") || trimmed.starts_with("/*") || trimmed.starts_with('*')
}

impl TsRule for ConsoleUsageRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        // Skip test files entirely
        if is_test_file(ctx.file_path) {
            return Vec::new();
        }

        let mut issues = Vec::new();

        for (i, line) in ctx.lines.iter().enumerate() {
            // Skip comment lines
            if is_comment_line(line) {
                continue;
            }

            for mat in self.console_pattern.find_iter(line) {
                let matched = mat.as_str();
                // Extract the method name from "console.<method>("
                let method = matched
                    .strip_prefix("console.")
                    .and_then(|rest| rest.split_once('(').or_else(|| rest.split_once(' ')))
                    .map(|(name, _)| name.trim())
                    .unwrap_or("log");

                let issue = QualityIssue::new(
                    "ts:console-usage",
                    RuleType::CodeSmell,
                    Severity::Minor,
                    crate::rule::AnalyzerSource::Other("built-in".into()),
                    format!(
                        "Unexpected console.{}() call found in production code",
                        method
                    ),
                )
                .with_location(ctx.file_path.to_string(), (i as u32) + 1)
                .with_effort(1);

                issues.push(issue);
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
        file_path: &'a str,
        content: &'a str,
        lines: &'a [&'a str],
        config: &'a RuleConfig,
    ) -> TsAnalysisContext<'a> {
        TsAnalysisContext {
            file_path,
            content,
            lines,
            config,
        }
    }

    #[test]
    fn detects_console_log_calls() {
        let src = r#"
function init() {
    console.log("starting up");
    console.warn("deprecation notice");
    console.error("something failed");
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("src/app.ts", src, &lines, &config);
        let rule = ConsoleUsageRule::default();
        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 3, "expected 3 console usage issues");
        assert!(issues[0].message.contains("console.log"));
        assert!(issues[1].message.contains("console.warn"));
        assert!(issues[2].message.contains("console.error"));
    }

    #[test]
    fn skips_test_files() {
        let src = "console.log('test output');\n";
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();

        let ctx = make_context("src/__tests__/app.test.ts", src, &lines, &config);
        let rule = ConsoleUsageRule::default();
        assert!(rule.analyze(&ctx).is_empty(), "should skip __tests__ directory");

        let ctx2 = make_context("src/utils.spec.ts", src, &lines, &config);
        assert!(rule.analyze(&ctx2).is_empty(), "should skip .spec. files");

        let ctx3 = make_context("src/utils.test.ts", src, &lines, &config);
        assert!(rule.analyze(&ctx3).is_empty(), "should skip .test. files");
    }

    #[test]
    fn skips_comment_lines() {
        let src = r#"
// console.log("commented out");
/* console.warn("block comment"); */
* console.error("inside block comment");
console.debug("this one is real");
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("src/app.ts", src, &lines, &config);
        let rule = ConsoleUsageRule::default();
        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1, "expected only the non-commented console call");
        assert!(issues[0].message.contains("console.debug"));
    }

    #[test]
    fn no_issues_for_clean_code() {
        let src = r#"
function add(a: number, b: number): number {
    return a + b;
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("src/math.ts", src, &lines, &config);
        let rule = ConsoleUsageRule::default();
        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "expected no issues for code without console calls");
    }
}
