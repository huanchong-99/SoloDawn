//! TODO/FIXME comment detection rule — flags TODO, FIXME, HACK, and XXX comments
//! in TypeScript/JavaScript source files.

use regex::Regex;

use crate::{
    issue::QualityIssue,
    rule::{AnalyzerSource, RuleType, Severity},
    rules::{Rule, TsAnalysisContext, TsRule},
};

/// Detects TODO, FIXME, HACK, and XXX comments in TypeScript/JavaScript files.
///
/// Matches patterns such as:
/// - `// TODO: implement this`
/// - `// FIXME: broken logic`
/// - `/* HACK: temporary workaround */`
/// - `// XXX: needs review`
///
/// The match is case-insensitive.
#[derive(Debug)]
pub struct TodoCommentsRule;

impl Default for TodoCommentsRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for TodoCommentsRule {
    fn id(&self) -> &str {
        "ts:todo-comments"
    }

    fn name(&self) -> &str {
        "TODO/FIXME Comments"
    }

    fn description(&self) -> &str {
        "Detects TODO, FIXME, HACK, and XXX comments that indicate incomplete or temporary code"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Info
    }
}

impl TsRule for TodoCommentsRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let pattern = Regex::new(r"(?i)(//|/\*)\s*(TODO|FIXME|HACK|XXX)\b").expect("valid regex");

        let mut issues = Vec::new();

        for (idx, line) in ctx.lines.iter().enumerate() {
            if let Some(mat) = pattern.find(line) {
                // Skip matches inside string literals: if the text before the match
                // contains an odd number of quote characters, the match is inside a string.
                let before_match = &line[..mat.start()];
                let double_quotes = before_match.chars().filter(|&c| c == '"').count();
                let single_quotes = before_match.chars().filter(|&c| c == '\'').count();
                let backticks = before_match.chars().filter(|&c| c == '`').count();
                if double_quotes % 2 != 0 || single_quotes % 2 != 0 || backticks % 2 != 0 {
                    continue;
                }

                let line_number = (idx + 1) as u32;

                // Extract the rest of the comment text after the match for context.
                let comment_text = line[mat.start()..].trim();

                let message = format!("Found comment marker: {}", comment_text,);

                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    message,
                )
                .with_location(ctx.file_path, line_number)
                .with_effort(5);

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
        let rule = TodoCommentsRule;
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
    fn detects_single_line_todo() {
        let source = "const x = 1;\n// TODO: implement validation\nconst y = 2;";
        let issues = run_rule(source);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "ts:todo-comments");
        assert_eq!(issues[0].line, Some(2));
        assert_eq!(issues[0].severity, Severity::Info);
        assert!(issues[0].message.contains("TODO"));
    }

    #[test]
    fn detects_multiple_markers() {
        let source = r#"
// TODO: add error handling
const a = 1;
// FIXME: this is broken
const b = 2;
/* HACK: temporary workaround */
const c = 3;
// XXX: needs review
"#;
        let issues = run_rule(source);
        assert_eq!(issues.len(), 4);
        assert_eq!(issues[0].line, Some(2));
        assert_eq!(issues[1].line, Some(4));
        assert_eq!(issues[2].line, Some(6));
        assert_eq!(issues[3].line, Some(8));
    }

    #[test]
    fn case_insensitive_matching() {
        let source = "// todo: lowercase\n// Todo: mixed case\n// FIXME: uppercase";
        let issues = run_rule(source);
        assert_eq!(issues.len(), 3);
    }

    #[test]
    fn clean_code_produces_no_issues() {
        let source = r#"
const x: number = 42;
const y: string = "hello";
// This is a normal comment
/* Regular block comment */
function add(a: number, b: number): number {
    return a + b;
}
"#;
        let issues = run_rule(source);
        assert!(issues.is_empty());
    }

    #[test]
    fn skips_todo_inside_string_literals() {
        let source = r#"const msg = "// TODO: this is data";
const other = '// FIXME: also data';
const tpl = `// HACK: template string`;
// TODO: real comment
"#;
        let issues = run_rule(source);
        assert_eq!(
            issues.len(),
            1,
            "expected 1 issue (real comment only), got {}: {:?}",
            issues.len(),
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
        assert_eq!(issues[0].line, Some(4));
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = TodoCommentsRule;
        assert_eq!(rule.id(), "ts:todo-comments");
        assert_eq!(rule.name(), "TODO/FIXME Comments");
        assert_eq!(rule.rule_type(), RuleType::CodeSmell);
        assert_eq!(rule.default_severity(), Severity::Info);
    }
}
