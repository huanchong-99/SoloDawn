//! Line length rule — flags source files that contain excessively long lines.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{CommonAnalysisContext, CommonRule, Rule};

/// Checks that individual lines in text files do not exceed a configurable maximum length.
///
/// Lines that consist solely of a URL or an import/use statement are excluded from the check
/// to reduce noise. When violations are found, a single [`QualityIssue`] is emitted per file
/// containing the count of offending lines.
#[derive(Debug)]
pub struct LineLengthRule;

impl Default for LineLengthRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for LineLengthRule {
    fn id(&self) -> &str {
        "common:line-length"
    }

    fn name(&self) -> &str {
        "Line Length"
    }

    fn description(&self) -> &str {
        "Checks that lines do not exceed a configurable maximum character length"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Info
    }
}

/// Returns `true` if the trimmed line looks like it is purely a URL.
fn is_url_line(line: &str) -> bool {
    let trimmed = line.trim();
    trimmed.contains("http://") || trimmed.contains("https://")
}

/// Returns `true` if the trimmed line is an import or use statement.
fn is_import_line(line: &str) -> bool {
    let trimmed = line.trim();
    trimmed.starts_with("use ")
        || trimmed.starts_with("import ")
        || trimmed.starts_with("from ")
        || trimmed.starts_with("#include ")
        || trimmed.starts_with("require(")
}

impl CommonRule for LineLengthRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        if !ctx.is_text {
            return vec![];
        }

        let text = match ctx.text {
            Some(t) => t,
            None => return vec![],
        };

        let max_length = ctx.config.get_param_usize("max_length", 120);

        let violation_count = text
            .lines()
            .filter(|line| {
                line.len() > max_length && !is_url_line(line) && !is_import_line(line)
            })
            .count();

        if violation_count == 0 {
            return vec![];
        }

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
                "File contains {} line(s) exceeding {} characters",
                violation_count, max_length
            ),
        )
        .with_location(ctx.file_path, 1)
        .with_effort(violation_count as i32 * 2);

        vec![issue]
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn make_ctx<'a>(
        text: &'a str,
        config: &'a RuleConfig,
    ) -> CommonAnalysisContext<'a> {
        CommonAnalysisContext {
            file_path: "src/example.rs",
            content: text.as_bytes(),
            is_text: true,
            text: Some(text),
            config,
        }
    }

    #[test]
    fn short_lines_produce_no_issues() {
        let rule = LineLengthRule::default();
        let content = "fn main() {\n    println!(\"hello\");\n}\n";
        let config = RuleConfig::default();
        let ctx = make_ctx(content, &config);

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Expected no issues for short lines");
    }

    #[test]
    fn long_lines_produce_single_issue_with_count() {
        let rule = LineLengthRule::default();
        let long_line = "x".repeat(150);
        let content = format!("short\n{}\nanother short\n{}\n", long_line, long_line);
        let config = RuleConfig::default();
        let ctx = make_ctx(&content, &config);

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "common:line-length");
        assert!(issues[0].message.contains('2'), "Should report 2 violations");
        assert!(issues[0].message.contains("120"));
    }

    #[test]
    fn url_lines_are_skipped() {
        let rule = LineLengthRule::default();
        let long_url = format!("// see https://example.com/{}", "a".repeat(150));
        let content = format!("{}\nshort line\n", long_url);
        let config = RuleConfig::default();
        let ctx = make_ctx(&content, &config);

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "URL lines should be skipped");
    }

    #[test]
    fn import_lines_are_skipped() {
        let rule = LineLengthRule::default();
        let long_import = format!("use crate::some::very::deeply::nested::module::{{{}}};", "A, ".repeat(50));
        let content = format!("{}\nshort\n", long_import);
        let config = RuleConfig::default();
        let ctx = make_ctx(&content, &config);

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Import lines should be skipped");
    }

    #[test]
    fn binary_files_are_skipped() {
        let rule = LineLengthRule::default();
        let config = RuleConfig::default();
        let ctx = CommonAnalysisContext {
            file_path: "image.png",
            content: &[0xFF, 0xD8, 0xFF],
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Binary files should produce no issues");
    }

    #[test]
    fn custom_max_length_is_respected() {
        let rule = LineLengthRule::default();
        let content = "x".repeat(85);
        let mut config = RuleConfig::default();
        config.params.insert("max_length".to_string(), "80".to_string());
        let ctx = make_ctx(&content, &config);

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("80"));
    }
}
