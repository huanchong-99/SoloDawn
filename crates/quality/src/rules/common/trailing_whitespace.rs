//! Trailing whitespace rule — detects lines with trailing spaces or tabs in text files.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{CommonAnalysisContext, CommonRule, Rule};

/// Detects trailing whitespace (spaces or tabs at end of lines) in text files.
#[derive(Debug)]
pub struct TrailingWhitespaceRule;

impl Default for TrailingWhitespaceRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for TrailingWhitespaceRule {
    fn id(&self) -> &str {
        "common:trailing-whitespace"
    }

    fn name(&self) -> &str {
        "Trailing Whitespace"
    }

    fn description(&self) -> &str {
        "Detects lines with trailing spaces or tabs in text files"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Info
    }
}

impl CommonRule for TrailingWhitespaceRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        if !ctx.is_text {
            return Vec::new();
        }

        let text = match ctx.text {
            Some(t) => t,
            None => return Vec::new(),
        };

        let count = text
            .lines()
            .filter(|line| {
                line.ends_with(' ') || line.ends_with('\t')
            })
            .count();

        if count == 0 {
            return Vec::new();
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
                "File has {} line{} with trailing whitespace",
                count,
                if count == 1 { "" } else { "s" }
            ),
        )
        .with_location(ctx.file_path, 1)
        .with_effort(count as i32);

        vec![issue]
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn make_context<'a>(
        content: &'a [u8],
        text: Option<&'a str>,
        is_text: bool,
        config: &'a RuleConfig,
    ) -> CommonAnalysisContext<'a> {
        CommonAnalysisContext {
            file_path: "test.txt",
            content,
            is_text,
            text,
            config,
        }
    }

    #[test]
    fn detects_trailing_whitespace() {
        let rule = TrailingWhitespaceRule::default();
        let src = "hello \nworld\nfoo \t\nbar\n";
        let config = RuleConfig::default();
        let ctx = make_context(src.as_bytes(), Some(src), true, &config);

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "common:trailing-whitespace");
        // Two lines have trailing whitespace: "hello " and "foo \t"
        assert!(issues[0].message.contains('2'));
    }

    #[test]
    fn no_issues_for_clean_file() {
        let rule = TrailingWhitespaceRule::default();
        let src = "hello\nworld\n";
        let config = RuleConfig::default();
        let ctx = make_context(src.as_bytes(), Some(src), true, &config);

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty());
    }

    #[test]
    fn skips_binary_files() {
        let rule = TrailingWhitespaceRule::default();
        let content = b"hello \nworld \n";
        let config = RuleConfig::default();
        let ctx = make_context(content, None, false, &config);

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty());
    }
}
