//! Secret detection rule — detects potential secrets, API keys, tokens, and passwords in source code.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{CommonAnalysisContext, CommonRule, Rule};

/// Detects potential secrets, API keys, tokens, and passwords in source code.
///
/// Scans text files line by line for common secret patterns such as AWS keys,
/// API keys, passwords, private keys, GitHub tokens, and generic hex tokens.
/// Files named `.env.example` and lines containing `TODO` or `placeholder` are skipped.
#[derive(Debug)]
pub struct SecretDetectionRule {
    patterns: Vec<SecretPattern>,
}

#[derive(Debug)]
struct SecretPattern {
    name: &'static str,
    regex: Regex,
}

impl Default for SecretDetectionRule {
    fn default() -> Self {
        let patterns = vec![
            SecretPattern {
                name: "AWS Access Key",
                regex: Regex::new(r"AKIA[0-9A-Z]{16}").unwrap(),
            },
            SecretPattern {
                name: "Generic API Key",
                regex: Regex::new(r#"(?i)(api[_-]?key|apikey)\s*[:=]\s*["'][^"']{8,}["']"#)
                    .unwrap(),
            },
            SecretPattern {
                name: "Generic Secret/Password/Token",
                regex: Regex::new(
                    r#"(?i)(secret|password|passwd|pwd|token|auth)\s*[:=]\s*["'][^"']{8,}["']"#,
                )
                .unwrap(),
            },
            SecretPattern {
                name: "Private Key",
                regex: Regex::new(r"-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----").unwrap(),
            },
            SecretPattern {
                name: "GitHub Token",
                regex: Regex::new(r"gh[ps]_[A-Za-z0-9_]{36,}").unwrap(),
            },
            SecretPattern {
                name: "Generic Hex Token",
                regex: Regex::new(r#"(?i)(token|key)\s*[:=]\s*["'][0-9a-f]{32,}["']"#).unwrap(),
            },
        ];

        Self { patterns }
    }
}

impl Rule for SecretDetectionRule {
    fn id(&self) -> &str {
        "common:secret-detection"
    }

    fn name(&self) -> &str {
        "Secret Detection"
    }

    fn description(&self) -> &str {
        "Detects potential secrets, API keys, tokens, and passwords in source code"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::Vulnerability
    }

    fn default_severity(&self) -> Severity {
        Severity::Blocker
    }
}

impl CommonRule for SecretDetectionRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        // Only analyze text files
        let text = match ctx.text {
            Some(t) if ctx.is_text => t,
            _ => return Vec::new(),
        };

        // Skip .env.example files
        if ctx.file_path.ends_with(".env.example") {
            return Vec::new();
        }

        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let mut issues = Vec::new();

        for (line_idx, line) in text.lines().enumerate() {
            // Skip lines containing TODO or placeholder
            let line_lower = line.to_lowercase();
            if line_lower.contains("todo") || line_lower.contains("placeholder") {
                continue;
            }

            for pattern in &self.patterns {
                if pattern.regex.is_match(line) {
                    let line_number = (line_idx + 1) as u32;
                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        severity,
                        AnalyzerSource::Other("builtin".to_string()),
                        format!(
                            "Potential {} detected on line {}",
                            pattern.name, line_number
                        ),
                    )
                    .with_location(ctx.file_path, line_number)
                    .with_effort(15);

                    issues.push(issue);
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

    fn analyze_text(file_path: &str, content: &str) -> Vec<QualityIssue> {
        let rule = SecretDetectionRule::default();
        let config = RuleConfig::default();
        let bytes = content.as_bytes();

        let ctx = CommonAnalysisContext {
            file_path,
            content: bytes,
            is_text: true,
            text: Some(content),
            config: &config,
        };

        rule.analyze(&ctx)
    }

    #[test]
    fn detects_aws_key() {
        let content = "let aws_key = \"AKIAIOSFODNN7EXAMPLE\";\n";
        let issues = analyze_text("config.rs", content);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("AWS Access Key"));
        assert_eq!(issues[0].severity, Severity::Blocker);
    }

    #[test]
    fn detects_generic_api_key_and_password() {
        let content = r#"
api_key = "abcdefghij1234567890"
password = "super_secret_value_here"
token = "mytoken12345678"
"#;
        let issues = analyze_text("app.py", content);
        assert!(issues.len() >= 3, "Expected at least 3 issues, got {}", issues.len());
    }

    #[test]
    fn detects_private_key_header() {
        let content = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n";
        let issues = analyze_text("key.pem", content);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("Private Key"));
    }

    #[test]
    fn detects_github_token() {
        let content = "GITHUB_TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl\n";
        let issues = analyze_text("ci.yml", content);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("GitHub Token"));
    }

    #[test]
    fn skips_env_example_files() {
        let content = "api_key = \"abcdefghij1234567890\"\n";
        let issues = analyze_text(".env.example", content);
        assert!(issues.is_empty(), "Should skip .env.example files");
    }

    #[test]
    fn skips_lines_with_todo_or_placeholder() {
        let content = r#"
api_key = "abcdefghij1234567890"  # TODO: replace with real key
password = "placeholder_value_here"
"#;
        let issues = analyze_text("config.py", content);
        assert!(
            issues.is_empty(),
            "Should skip lines containing TODO or placeholder, got {} issues",
            issues.len()
        );
    }

    #[test]
    fn no_issues_for_clean_code() {
        let content = r#"
fn main() {
    let x = 42;
    println!("Hello, world!");
}
"#;
        let issues = analyze_text("main.rs", content);
        assert!(issues.is_empty(), "Expected no issues for clean code");
    }

    #[test]
    fn skips_binary_files() {
        let rule = SecretDetectionRule::default();
        let config = RuleConfig::default();
        let bytes = b"\x00\x01\x02AKIAIOSFODNN7EXAMPLE";

        let ctx = CommonAnalysisContext {
            file_path: "image.bin",
            content: bytes,
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Should skip binary files");
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = SecretDetectionRule::default();
        assert_eq!(rule.id(), "common:secret-detection");
        assert_eq!(rule.name(), "Secret Detection");
        assert_eq!(rule.rule_type(), RuleType::Vulnerability);
        assert_eq!(rule.default_severity(), Severity::Blocker);
    }
}
