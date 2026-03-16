//! File length rule — flags Rust source files that exceed a configurable maximum line count.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{Rule, RustAnalysisContext, RustRule};

/// Checks that Rust source files do not exceed a maximum number of lines.
#[derive(Debug)]
pub struct FileLengthRule;

impl Default for FileLengthRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for FileLengthRule {
    fn id(&self) -> &str {
        "rust:file-length"
    }

    fn name(&self) -> &str {
        "File Length"
    }

    fn description(&self) -> &str {
        "Checks that Rust source files do not exceed a maximum number of lines"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }
}

impl RustRule for FileLengthRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let max_lines = ctx.config.get_param_usize("max_lines", 500);
        let line_count = ctx.content.lines().count();

        if line_count > max_lines {
            let severity = ctx
                .config
                .severity_override
                .unwrap_or_else(|| self.default_severity());

            let issue = QualityIssue::new(
                self.id(),
                self.rule_type(),
                severity,
                AnalyzerSource::Other("builtin".to_string()),
                format!(
                    "File has {} lines, which exceeds the maximum of {} lines",
                    line_count, max_lines
                ),
            )
            .with_location(ctx.file_path, 1)
            .with_effort(30);

            vec![issue]
        } else {
            vec![]
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn make_content(line_count: usize) -> String {
        (0..line_count)
            .map(|i| format!("// line {}", i + 1))
            .collect::<Vec<_>>()
            .join("\n")
    }

    fn default_config() -> RuleConfig {
        RuleConfig::default()
    }

    #[test]
    fn file_within_limit_produces_no_issues() {
        let rule = FileLengthRule::default();
        let content = make_content(100);
        let syntax: syn::File = syn::parse_str("fn main() {}").unwrap();
        let config = default_config();

        let ctx = RustAnalysisContext {
            file_path: "src/main.rs",
            content: &content,
            syntax: &syntax,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Expected no issues for a short file");
    }

    #[test]
    fn file_exceeding_limit_produces_issue() {
        let rule = FileLengthRule::default();
        let content = make_content(501);
        let syntax: syn::File = syn::parse_str("fn main() {}").unwrap();
        let config = default_config();

        let ctx = RustAnalysisContext {
            file_path: "src/large.rs",
            content: &content,
            syntax: &syntax,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "rust:file-length");
        assert_eq!(issues[0].severity, Severity::Minor);
        assert!(issues[0].message.contains("501"));
        assert!(issues[0].message.contains("500"));
    }

    #[test]
    fn custom_max_lines_is_respected() {
        let rule = FileLengthRule::default();
        let content = make_content(250);
        let syntax: syn::File = syn::parse_str("fn main() {}").unwrap();
        let mut config = default_config();
        config
            .params
            .insert("max_lines".to_string(), "200".to_string());

        let ctx = RustAnalysisContext {
            file_path: "src/medium.rs",
            content: &content,
            syntax: &syntax,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("250"));
        assert!(issues[0].message.contains("200"));
    }
}
