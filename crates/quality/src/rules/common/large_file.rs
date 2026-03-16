//! Large file rule — flags files that exceed a configurable maximum size.
//!
//! For text files the rule counts lines; for binary files it checks byte length.

use crate::{
    issue::QualityIssue,
    rule::{AnalyzerSource, RuleType, Severity},
    rules::{CommonAnalysisContext, CommonRule, Rule},
};

/// Detects excessively large source files.
///
/// * Text files: triggers when the number of lines exceeds `max_lines` (default 1000).
/// * Binary files: triggers when the byte size exceeds `max_bytes` (default 1 048 576 = 1 MB).
#[derive(Debug)]
pub struct LargeFileRule;

impl Default for LargeFileRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for LargeFileRule {
    fn id(&self) -> &str {
        "common:large-file"
    }

    fn name(&self) -> &str {
        "Large File"
    }

    fn description(&self) -> &str {
        "Detects excessively large source files"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }
}

impl CommonRule for LargeFileRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        if ctx.is_text {
            let max_lines = ctx.config.get_param_usize("max_lines", 1000);
            let line_count = ctx.text.map(|t| t.lines().count()).unwrap_or(0);

            if line_count > max_lines {
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

                return vec![issue];
            }
        } else {
            let max_bytes = ctx.config.get_param_usize("max_bytes", 1_048_576);
            let byte_size = ctx.content.len();

            if byte_size > max_bytes {
                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    format!(
                        "Binary file is {} bytes, which exceeds the maximum of {} bytes",
                        byte_size, max_bytes
                    ),
                )
                .with_location(ctx.file_path, 1)
                .with_effort(15);

                return vec![issue];
            }
        }

        vec![]
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn make_text_content(line_count: usize) -> String {
        (0..line_count)
            .map(|i| format!("// line {}", i + 1))
            .collect::<Vec<_>>()
            .join("\n")
    }

    fn default_config() -> RuleConfig {
        RuleConfig::default()
    }

    #[test]
    fn text_file_within_limit_produces_no_issues() {
        let rule = LargeFileRule::default();
        let content = make_text_content(500);
        let bytes = content.as_bytes();
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/small.rs",
            content: bytes,
            is_text: true,
            text: Some(&content),
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "Expected no issues for a small text file"
        );
    }

    #[test]
    fn text_file_exceeding_limit_produces_issue() {
        let rule = LargeFileRule::default();
        let content = make_text_content(1001);
        let bytes = content.as_bytes();
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/huge.rs",
            content: bytes,
            is_text: true,
            text: Some(&content),
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "common:large-file");
        assert_eq!(issues[0].severity, Severity::Minor);
        assert!(issues[0].message.contains("1001"));
        assert!(issues[0].message.contains("1000"));
    }

    #[test]
    fn binary_file_exceeding_limit_produces_issue() {
        let rule = LargeFileRule::default();
        let bytes = vec![0u8; 1_048_577];
        let mut config = default_config();
        config
            .params
            .insert("max_bytes".to_string(), "1048576".to_string());

        let ctx = CommonAnalysisContext {
            file_path: "assets/image.bin",
            content: &bytes,
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "common:large-file");
        assert!(issues[0].message.contains("1048577"));
        assert!(issues[0].message.contains("1048576"));
    }

    #[test]
    fn binary_file_within_limit_produces_no_issues() {
        let rule = LargeFileRule::default();
        let bytes = vec![0u8; 1000];
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "assets/small.bin",
            content: &bytes,
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "Expected no issues for a small binary file"
        );
    }
}
