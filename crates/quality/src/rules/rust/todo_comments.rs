//! TODO/FIXME comments rule — detects TODO, FIXME, HACK, XXX comments in Rust source files.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{Rule, RustAnalysisContext, RustRule};

/// Detects TODO, FIXME, HACK, and XXX comments in Rust source files.
///
/// This is a text-based rule that scans raw source content line by line
/// using a regular expression — no AST analysis is required.
#[derive(Debug)]
pub struct TodoCommentsRule;

impl Default for TodoCommentsRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for TodoCommentsRule {
    fn id(&self) -> &str {
        "rust:todo-comments"
    }

    fn name(&self) -> &str {
        "TODO/FIXME Comments"
    }

    fn description(&self) -> &str {
        "Detects TODO, FIXME, HACK, and XXX comments that indicate unfinished work"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Info
    }
}

impl RustRule for TodoCommentsRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let re = Regex::new(r"(?i)(//|/\*)\s*(TODO|FIXME|HACK|XXX)\b").unwrap();

        let mut issues = Vec::new();

        for (line_idx, line) in ctx.content.lines().enumerate() {
            if let Some(m) = re.find(line) {
                let comment_text = line[m.start()..].trim().to_string();
                let line_number = (line_idx + 1) as u32;

                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    format!(
                        "Found comment marker on line {}: {}",
                        line_number, comment_text
                    ),
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

    fn analyze_content(content: &str) -> Vec<QualityIssue> {
        let rule = TodoCommentsRule::default();
        let syntax: syn::File = syn::parse_str("fn main() {}").unwrap();
        let config = RuleConfig::default();

        let ctx = RustAnalysisContext {
            file_path: "src/lib.rs",
            content,
            syntax: &syntax,
            config: &config,
        };

        rule.analyze(&ctx)
    }

    #[test]
    fn detects_todo_and_fixme_comments() {
        let content = r#"
fn main() {
    // TODO: implement this
    let x = 1; // FIXME: wrong value
    /* HACK: temporary workaround */
    // XXX: needs review
}
"#;
        let issues = analyze_content(content);
        assert_eq!(issues.len(), 4);
        assert!(issues[0].message.contains("TODO"));
        assert!(issues[1].message.contains("FIXME"));
        assert!(issues[2].message.contains("HACK"));
        assert!(issues[3].message.contains("XXX"));

        // Verify line numbers are set
        assert_eq!(issues[0].line, Some(3));
        assert_eq!(issues[1].line, Some(4));
        assert_eq!(issues[2].line, Some(5));
        assert_eq!(issues[3].line, Some(6));
    }

    #[test]
    fn no_issues_for_clean_code() {
        let content = r#"
fn main() {
    // This is a normal comment
    let x = 42;
    /* A regular block comment */
}
"#;
        let issues = analyze_content(content);
        assert!(issues.is_empty(), "Expected no issues for code without TODO markers");
    }

    #[test]
    fn case_insensitive_matching() {
        let content = "// todo: lowercase\n// Todo: mixed case\n// TODO: uppercase\n";
        let issues = analyze_content(content);
        assert_eq!(issues.len(), 3, "Should match TODO regardless of case");
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = TodoCommentsRule::default();
        assert_eq!(rule.id(), "rust:todo-comments");
        assert_eq!(rule.name(), "TODO/FIXME Comments");
        assert_eq!(rule.rule_type(), RuleType::CodeSmell);
        assert_eq!(rule.default_severity(), Severity::Info);
    }
}
