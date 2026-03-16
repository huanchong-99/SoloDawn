//! Encoding rule — detects non-UTF-8 encoded files and files with BOM (Byte Order Mark).
//!
//! Source code files should be encoded as UTF-8 without BOM to ensure maximum
//! portability and avoid subtle parsing issues.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{CommonAnalysisContext, CommonRule, Rule};

/// Source code file extensions that are expected to be valid UTF-8.
const SOURCE_EXTENSIONS: &[&str] = &[
    ".rs", ".ts", ".js", ".tsx", ".jsx", ".py", ".go", ".java", ".json", ".yaml", ".toml", ".md",
];

/// UTF-8 BOM bytes.
const UTF8_BOM: &[u8] = &[0xEF, 0xBB, 0xBF];

/// UTF-16 LE BOM bytes.
const UTF16_LE_BOM: &[u8] = &[0xFF, 0xFE];

/// UTF-16 BE BOM bytes.
const UTF16_BE_BOM: &[u8] = &[0xFE, 0xFF];

/// Detects non-UTF-8 encoded files and files containing a Byte Order Mark (BOM).
///
/// Three checks are performed:
/// 1. If a file has a known source code extension but is not valid UTF-8, it is flagged.
/// 2. If the content starts with a UTF-8 BOM (`EF BB BF`), it is flagged.
/// 3. If the content starts with a UTF-16 BOM (`FF FE` or `FE FF`), it is flagged.
#[derive(Debug)]
pub struct EncodingRule;

impl Default for EncodingRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for EncodingRule {
    fn id(&self) -> &str {
        "common:encoding"
    }

    fn name(&self) -> &str {
        "File Encoding"
    }

    fn description(&self) -> &str {
        "Detects non-UTF-8 encoded files and files with BOM (Byte Order Mark)"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::Bug
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }
}

/// Returns `true` if the file path ends with a known source code extension.
fn has_source_extension(file_path: &str) -> bool {
    let lower = file_path.to_ascii_lowercase();
    SOURCE_EXTENSIONS.iter().any(|ext| lower.ends_with(ext))
}

impl CommonRule for EncodingRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let mut issues = Vec::new();

        // Check 1: non-UTF-8 source code file
        if !ctx.is_text && has_source_extension(ctx.file_path) {
            issues.push(
                QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    format!(
                        "File '{}' has a source code extension but is not valid UTF-8",
                        ctx.file_path
                    ),
                )
                .with_location(ctx.file_path, 1)
                .with_effort(15),
            );
        }

        // Check 2: UTF-8 BOM
        if ctx.content.starts_with(UTF8_BOM) {
            issues.push(
                QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    format!(
                        "File '{}' starts with a UTF-8 BOM (EF BB BF); remove the BOM for maximum portability",
                        ctx.file_path
                    ),
                )
                .with_location(ctx.file_path, 1)
                .with_effort(5),
            );
        }

        // Check 3: UTF-16 BOM (LE or BE)
        if ctx.content.starts_with(UTF16_LE_BOM) || ctx.content.starts_with(UTF16_BE_BOM) {
            // Avoid double-reporting when the content also matched the UTF-8 BOM check
            // (UTF-8 BOM starts with EF, so there is no overlap with FF FE / FE FF)
            let bom_kind = if ctx.content.starts_with(UTF16_LE_BOM) {
                "UTF-16 LE BOM (FF FE)"
            } else {
                "UTF-16 BE BOM (FE FF)"
            };

            issues.push(
                QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    AnalyzerSource::Other("builtin".to_string()),
                    format!(
                        "File '{}' starts with a {}; convert to UTF-8 without BOM",
                        ctx.file_path, bom_kind
                    ),
                )
                .with_location(ctx.file_path, 1)
                .with_effort(10),
            );
        }

        issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn default_config() -> RuleConfig {
        RuleConfig::default()
    }

    #[test]
    fn non_utf8_source_file_produces_issue() {
        let rule = EncodingRule::default();
        // Invalid UTF-8 bytes
        let content: &[u8] = &[0x80, 0x81, 0x82, 0x83];
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/main.rs",
            content,
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "common:encoding");
        assert!(issues[0].message.contains("not valid UTF-8"));
    }

    #[test]
    fn utf8_bom_produces_issue() {
        let rule = EncodingRule::default();
        let mut content = vec![0xEF, 0xBB, 0xBF];
        content.extend_from_slice(b"fn main() {}");
        let text = std::str::from_utf8(&content[3..]).unwrap();
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/main.rs",
            content: &content,
            is_text: true,
            text: Some(text),
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "common:encoding");
        assert!(issues[0].message.contains("UTF-8 BOM"));
    }

    #[test]
    fn utf16_le_bom_produces_issue() {
        let rule = EncodingRule::default();
        let content: &[u8] = &[0xFF, 0xFE, 0x00, 0x00];
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "data/config.json",
            content,
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        // Expect two issues: non-UTF-8 + UTF-16 LE BOM
        assert!(issues.len() >= 1);
        let bom_issue = issues.iter().find(|i| i.message.contains("UTF-16 LE BOM"));
        assert!(bom_issue.is_some(), "Expected a UTF-16 LE BOM issue");
    }

    #[test]
    fn clean_utf8_file_produces_no_issues() {
        let rule = EncodingRule::default();
        let content = b"fn main() { println!(\"hello\"); }";
        let text = std::str::from_utf8(content).unwrap();
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/lib.rs",
            content,
            is_text: true,
            text: Some(text),
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Expected no issues for a clean UTF-8 file");
    }

    #[test]
    fn non_source_binary_file_produces_no_non_utf8_issue() {
        let rule = EncodingRule::default();
        // Binary content with a non-source extension
        let content: &[u8] = &[0x80, 0x81, 0x82, 0x83];
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "assets/image.png",
            content,
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "Expected no issues for a binary file with a non-source extension"
        );
    }
}
