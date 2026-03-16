//! Any type usage rule — detects usage of the `any` type in TypeScript code.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{Rule, TsAnalysisContext, TsRule};
use regex::Regex;

/// Detects usage of the `any` type in TypeScript source files.
///
/// Matches the following patterns:
/// - `: any` (type annotation)
/// - `as any` (type assertion)
/// - `<any>` (generic type assertion)
/// - `any[]` (array of any)
#[derive(Debug)]
pub struct AnyUsageRule;

impl Default for AnyUsageRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for AnyUsageRule {
    fn id(&self) -> &str {
        "ts:any-usage"
    }

    fn name(&self) -> &str {
        "Any Type Usage"
    }

    fn description(&self) -> &str {
        "Detects usage of the `any` type in TypeScript code"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }
}

/// Returns true if the line is a single-line comment or if the relevant portion
/// appears inside a block comment.
fn is_comment_line(line: &str, in_block_comment: &mut bool) -> bool {
    let trimmed = line.trim();

    // If we are inside a block comment, check for the end marker.
    if *in_block_comment {
        if let Some(_pos) = trimmed.find("*/") {
            *in_block_comment = false;
        }
        return true;
    }

    // Single-line comment
    if trimmed.starts_with("//") {
        return true;
    }

    // Block comment that opens and closes on the same line
    if trimmed.starts_with("/*") {
        if trimmed.contains("*/") {
            // Entire comment on one line
            return true;
        }
        *in_block_comment = true;
        return true;
    }

    false
}

impl TsRule for AnyUsageRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let pattern = Regex::new(r"(:\s*any\b|as\s+any\b|<any>|any\[\])").expect("valid regex");

        let mut issues = Vec::new();
        let mut in_block_comment = false;

        for (idx, line) in ctx.lines.iter().enumerate() {
            if is_comment_line(line, &mut in_block_comment) {
                continue;
            }

            for mat in pattern.find_iter(line) {
                let matched_text = mat.as_str().trim();
                let line_number = (idx + 1) as u32;

                let message = format!(
                    "Avoid using `any` type (found `{}`). Use a more specific type instead.",
                    matched_text,
                );

                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    message,
                )
                .with_location(ctx.file_path, line_number)
                .with_effort(10);

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

    fn run_rule(source: &str) -> Vec<QualityIssue> {
        let rule = AnyUsageRule::default();
        let lines: Vec<&str> = source.lines().collect();
        let config = RuleConfig::default();
        let ctx = TsAnalysisContext {
            file_path: "src/example.ts",
            content: source,
            lines: &lines,
            config: &config,
        };
        rule.analyze(&ctx)
    }

    #[test]
    fn detects_type_annotation_any() {
        let source = "const x: any = 42;\nconst y: string = \"hello\";";
        let issues = run_rule(source);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "ts:any-usage");
        assert_eq!(issues[0].line, Some(1));
        assert_eq!(issues[0].severity, Severity::Major);
    }

    #[test]
    fn detects_multiple_patterns() {
        let source = r#"
const a: any = 1;
const b = value as any;
const c = <any>value;
const d: any[] = [];
"#;
        let issues = run_rule(source);
        assert_eq!(issues.len(), 4);
        assert_eq!(issues[0].line, Some(2));
        assert_eq!(issues[1].line, Some(3));
        assert_eq!(issues[2].line, Some(4));
        assert_eq!(issues[3].line, Some(5));
    }

    #[test]
    fn skips_comments() {
        let source = r#"
// const a: any = 1;
/* const b = value as any; */
const c: string = "hello";
/*
const d: any = 2;
*/
const e: number = 3;
"#;
        let issues = run_rule(source);
        assert!(
            issues.is_empty(),
            "Expected no issues when `any` appears only in comments, got {}",
            issues.len()
        );
    }

    #[test]
    fn clean_code_produces_no_issues() {
        let source = r#"
const x: number = 42;
const y: string = "hello";
function add(a: number, b: number): number {
    return a + b;
}
"#;
        let issues = run_rule(source);
        assert!(issues.is_empty());
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = AnyUsageRule::default();
        assert_eq!(rule.id(), "ts:any-usage");
        assert_eq!(rule.name(), "Any Type Usage");
        assert_eq!(rule.rule_type(), RuleType::CodeSmell);
        assert_eq!(rule.default_severity(), Severity::Major);
    }
}
