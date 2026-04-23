//! Weak default detection — catches hardcoded weak defaults that are NOT placeholders.
//!
//! Unlike `secret_detection` (which skips lines containing "changeme"/"example"),
//! this rule specifically targets runtime defaults and Docker Compose credentials
//! that use known-weak values.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{CommonAnalysisContext, CommonRule, Rule};

#[derive(Debug)]
pub struct WeakDefaultDetectionRule {
    patterns: Vec<WeakDefaultPattern>,
}

#[derive(Debug)]
struct WeakDefaultPattern {
    name: &'static str,
    regex: Regex,
    severity: Severity,
}

impl Default for WeakDefaultDetectionRule {
    fn default() -> Self {
        let patterns = vec![
            // JS/TS fallback defaults: || "dev_secret..." or || "password" etc.
            WeakDefaultPattern {
                name: "Weak Default Fallback (JS/TS)",
                regex: Regex::new(
                    r#"(?i)\|\|\s*["'](?:dev[_-]?secret|secret|password|changeme|admin|default[_-]?key)[^"']*["']"#,
                )
                .unwrap(),
                severity: Severity::Critical,
            },
            // Docker Compose: GF_SECURITY_ADMIN_PASSWORD: admin
            WeakDefaultPattern {
                name: "Docker Compose Default Credentials",
                regex: Regex::new(
                    r#"(?i)(?:ADMIN_PASSWORD|GF_SECURITY_ADMIN_PASSWORD|GRAFANA[_.]?PASSWORD)\s*:\s*(?:admin|password|changeme|123456)\s*$"#,
                )
                .unwrap(),
                severity: Severity::Blocker,
            },
            // Docker Compose: default admin username + password pair
            WeakDefaultPattern {
                name: "Docker Compose Default Admin User",
                regex: Regex::new(
                    r#"(?i)(?:ADMIN_USER|GF_SECURITY_ADMIN_USER)\s*:\s*admin\s*$"#,
                )
                .unwrap(),
                severity: Severity::Critical,
            },
            // YAML/env: PASSWORD=admin or PASSWORD=password
            WeakDefaultPattern {
                name: "Weak Password in Config",
                regex: Regex::new(
                    r#"(?i)(?:PASSWORD|PASSWD|PWD)\s*[:=]\s*(?:admin|password|changeme|123456|root)\s*$"#,
                )
                .unwrap(),
                severity: Severity::Critical,
            },
            // JWT_SECRET with well-known weak values
            WeakDefaultPattern {
                name: "Weak JWT Secret Default",
                regex: Regex::new(
                    r#"(?i)(?:JWT[_-]?SECRET|jwt[Ss]ecret)\s*[:=]\s*["']?(?:change[-_]?me[-_]?in[-_]?production|dev[_-]?secret[_-]?key|secret|your[-_]?secret[-_]?here|supersecret)"#,
                )
                .unwrap(),
                severity: Severity::Blocker,
            },
        ];

        Self { patterns }
    }
}

impl Rule for WeakDefaultDetectionRule {
    fn id(&self) -> &str {
        "common:weak-default-detection"
    }

    fn name(&self) -> &str {
        "Weak Default Detection"
    }

    fn description(&self) -> &str {
        "Detects hardcoded weak defaults (credentials, secrets) that are actual runtime values, not placeholders"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::Vulnerability
    }

    fn default_severity(&self) -> Severity {
        Severity::Critical
    }
}

impl CommonRule for WeakDefaultDetectionRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        let text = match ctx.text {
            Some(t) if ctx.is_text => t,
            _ => return Vec::new(),
        };

        // Only scan relevant file types
        let is_relevant = ctx.file_path.ends_with(".yml")
            || ctx.file_path.ends_with(".yaml")
            || ctx.file_path.ends_with(".env")
            || ctx.file_path.ends_with(".js")
            || ctx.file_path.ends_with(".ts")
            || ctx.file_path.ends_with(".jsx")
            || ctx.file_path.ends_with(".tsx")
            || ctx.file_path.ends_with(".json")
            || ctx.file_path.ends_with(".toml");

        if !is_relevant {
            return Vec::new();
        }

        // Skip example/template files
        if ctx.file_path.contains(".example")
            || ctx.file_path.contains(".sample")
            || ctx.file_path.contains(".template")
        {
            return Vec::new();
        }

        let mut issues = Vec::new();

        for (line_idx, line) in text.lines().enumerate() {
            // Skip commented lines
            let trimmed = line.trim();
            if trimmed.starts_with('#') || trimmed.starts_with("//") {
                continue;
            }

            for pattern in &self.patterns {
                if pattern.regex.is_match(line) {
                    let line_number = (line_idx + 1) as u32;
                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        pattern.severity,
                        AnalyzerSource::Other("builtin".to_string()),
                        format!(
                            "{}: weak default value on line {}",
                            pattern.name, line_number
                        ),
                    )
                    .with_location(ctx.file_path, line_number)
                    .with_effort(5);

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

    fn make_ctx<'a>(
        file_path: &'a str,
        text: &'a str,
        bytes: &'a [u8],
        config: &'a RuleConfig,
    ) -> CommonAnalysisContext<'a> {
        CommonAnalysisContext {
            file_path,
            content: bytes,
            is_text: true,
            text: Some(text),
            config,
        }
    }

    #[test]
    fn detects_docker_compose_admin_password() {
        let rule = WeakDefaultDetectionRule::default();
        let config = RuleConfig::default();
        let text = "      GF_SECURITY_ADMIN_PASSWORD: admin\n";
        let ctx = make_ctx("docker-compose.yml", text, text.as_bytes(), &config);
        let issues = rule.analyze(&ctx);
        assert!(
            !issues.is_empty(),
            "should detect at least one weak default"
        );
        assert!(
            issues.iter().any(|i| i.message.contains("Docker Compose")),
            "should include Docker Compose pattern match"
        );
    }

    #[test]
    fn detects_js_fallback_default() {
        let rule = WeakDefaultDetectionRule::default();
        let config = RuleConfig::default();
        let text = r#"  jwtSecret: process.env.JWT_SECRET || "dev_secret_key","#;
        let ctx = make_ctx("src/config/env.js", text, text.as_bytes(), &config);
        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("Weak Default Fallback"));
    }

    #[test]
    fn detects_jwt_secret_default() {
        let rule = WeakDefaultDetectionRule::default();
        let config = RuleConfig::default();
        let text = r#"JWT_SECRET=change-me-in-production"#;
        let ctx = make_ctx(".env", text, text.as_bytes(), &config);
        let issues = rule.analyze(&ctx);
        assert!(!issues.is_empty());
    }

    #[test]
    fn skips_example_files() {
        let rule = WeakDefaultDetectionRule::default();
        let config = RuleConfig::default();
        let text = "GF_SECURITY_ADMIN_PASSWORD: admin\n";
        let ctx = make_ctx(
            "docker-compose.example.yml",
            text,
            text.as_bytes(),
            &config,
        );
        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty());
    }

    #[test]
    fn skips_commented_lines() {
        let rule = WeakDefaultDetectionRule::default();
        let config = RuleConfig::default();
        let text = "# GF_SECURITY_ADMIN_PASSWORD: admin\n";
        let ctx = make_ctx("docker-compose.yml", text, text.as_bytes(), &config);
        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty());
    }
}
