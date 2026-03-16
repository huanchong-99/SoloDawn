//! Naming convention rule for TypeScript/JavaScript.
//!
//! Checks that functions use camelCase, classes/interfaces/types/enums use PascalCase,
//! and React components use PascalCase.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext, RuleConfig};

/// Checks TypeScript/JavaScript naming conventions.
///
/// - Function names must be camelCase
/// - Class, interface, type alias, and enum names must be PascalCase
/// - Interface names may optionally be prefixed with `I`
/// - UPPER_SNAKE_CASE constants are accepted
/// - React components (PascalCase functions returning JSX) must be PascalCase
#[derive(Debug)]
pub struct NamingConventionRule {
    fn_pattern: Regex,
    class_pattern: Regex,
    interface_pattern: Regex,
    type_pattern: Regex,
    enum_pattern: Regex,
    const_upper_pattern: Regex,
    react_component_pattern: Regex,
}

impl Default for NamingConventionRule {
    fn default() -> Self {
        Self {
            fn_pattern: Regex::new(r"function\s+(\w+)")
                .expect("invalid fn_pattern regex"),
            class_pattern: Regex::new(r"class\s+(\w+)")
                .expect("invalid class_pattern regex"),
            interface_pattern: Regex::new(r"interface\s+(\w+)")
                .expect("invalid interface_pattern regex"),
            type_pattern: Regex::new(r"type\s+(\w+)")
                .expect("invalid type_pattern regex"),
            enum_pattern: Regex::new(r"enum\s+(\w+)")
                .expect("invalid enum_pattern regex"),
            const_upper_pattern: Regex::new(r"const\s+([A-Z_]+)\s*=")
                .expect("invalid const_upper_pattern regex"),
            react_component_pattern: Regex::new(r"function\s+([A-Z]\w*)\s*\(")
                .expect("invalid react_component_pattern regex"),
        }
    }
}

/// Returns `true` if `name` is camelCase: starts with a lowercase ASCII letter
/// and contains no underscores (except for the rare case of a single-word name).
fn is_camel_case(name: &str) -> bool {
    if name.is_empty() {
        return false;
    }
    let first = name.chars().next().unwrap();
    if !first.is_ascii_lowercase() {
        return false;
    }
    // camelCase must not contain underscores
    if name.contains('_') {
        return false;
    }
    // Must be purely alphanumeric ASCII after the first char
    name.chars().all(|c| c.is_ascii_alphanumeric())
}

/// Returns `true` if `name` is PascalCase: starts with an uppercase ASCII letter,
/// contains no underscores, and has at least one lowercase letter.
fn is_pascal_case(name: &str) -> bool {
    if name.is_empty() {
        return false;
    }
    let first = name.chars().next().unwrap();
    if !first.is_ascii_uppercase() {
        return false;
    }
    if name.contains('_') {
        return false;
    }
    // Must be purely alphanumeric ASCII
    if !name.chars().all(|c| c.is_ascii_alphanumeric()) {
        return false;
    }
    // Must have at least one lowercase letter (otherwise it is ALLCAPS, not PascalCase)
    // Single uppercase char is OK (e.g., type T, component X)
    name.len() == 1 || name.chars().any(|c| c.is_ascii_lowercase())
}

impl Rule for NamingConventionRule {
    fn id(&self) -> &str {
        "ts:naming"
    }

    fn name(&self) -> &str {
        "Naming Convention"
    }

    fn description(&self) -> &str {
        "Checks TypeScript/JavaScript naming conventions (camelCase functions, PascalCase classes/interfaces/types/enums)"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }

    fn default_config(&self) -> RuleConfig {
        RuleConfig::default()
    }
}

impl TsRule for NamingConventionRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let mut issues = Vec::new();
        let is_jsx_file = ctx.file_path.ends_with(".tsx") || ctx.file_path.ends_with(".jsx");

        for (i, line) in ctx.lines.iter().enumerate() {
            let line_number = (i as u32) + 1;
            let trimmed = line.trim();

            // Skip comments
            if trimmed.starts_with("//") || trimmed.starts_with('*') || trimmed.starts_with("/*") {
                continue;
            }

            // UPPER_SNAKE_CASE constants are OK — check before function pattern
            if self.const_upper_pattern.is_match(line) {
                continue;
            }

            // React component: PascalCase function returning JSX is fine,
            // but only in .tsx/.jsx files where React components are expected.
            let is_react_component = is_jsx_file && self.react_component_pattern.is_match(line);

            // Function names should be camelCase (unless it is a React component)
            if let Some(caps) = self.fn_pattern.captures(line) {
                let name = caps.get(1).unwrap().as_str();
                if !is_react_component && !is_camel_case(name) {
                    issues.push(
                        QualityIssue::new(
                            "ts:naming",
                            RuleType::CodeSmell,
                            Severity::Minor,
                            crate::rule::AnalyzerSource::Other("built-in".into()),
                            format!(
                                "Function '{}' should use camelCase naming",
                                name
                            ),
                        )
                        .with_location(ctx.file_path.to_string(), line_number),
                    );
                }
            }

            // Class names should be PascalCase
            if let Some(caps) = self.class_pattern.captures(line) {
                let name = caps.get(1).unwrap().as_str();
                if !is_pascal_case(name) {
                    issues.push(
                        QualityIssue::new(
                            "ts:naming",
                            RuleType::CodeSmell,
                            Severity::Minor,
                            crate::rule::AnalyzerSource::Other("built-in".into()),
                            format!(
                                "Class '{}' should use PascalCase naming",
                                name
                            ),
                        )
                        .with_location(ctx.file_path.to_string(), line_number),
                    );
                }
            }

            // Interface names should be PascalCase (optionally prefixed with I)
            if let Some(caps) = self.interface_pattern.captures(line) {
                let name = caps.get(1).unwrap().as_str();
                // Strip leading 'I' prefix for PascalCase check
                let name_to_check = if name.len() > 1
                    && name.starts_with('I')
                    && name.chars().nth(1).unwrap().is_ascii_uppercase()
                {
                    &name[1..]
                } else {
                    name
                };
                if !is_pascal_case(name_to_check) {
                    issues.push(
                        QualityIssue::new(
                            "ts:naming",
                            RuleType::CodeSmell,
                            Severity::Minor,
                            crate::rule::AnalyzerSource::Other("built-in".into()),
                            format!(
                                "Interface '{}' should use PascalCase naming (optionally prefixed with 'I')",
                                name
                            ),
                        )
                        .with_location(ctx.file_path.to_string(), line_number),
                    );
                }
            }

            // Type alias names should be PascalCase
            if let Some(caps) = self.type_pattern.captures(line) {
                let name = caps.get(1).unwrap().as_str();
                if !is_pascal_case(name) {
                    issues.push(
                        QualityIssue::new(
                            "ts:naming",
                            RuleType::CodeSmell,
                            Severity::Minor,
                            crate::rule::AnalyzerSource::Other("built-in".into()),
                            format!(
                                "Type alias '{}' should use PascalCase naming",
                                name
                            ),
                        )
                        .with_location(ctx.file_path.to_string(), line_number),
                    );
                }
            }

            // Enum names should be PascalCase
            if let Some(caps) = self.enum_pattern.captures(line) {
                let name = caps.get(1).unwrap().as_str();
                if !is_pascal_case(name) {
                    issues.push(
                        QualityIssue::new(
                            "ts:naming",
                            RuleType::CodeSmell,
                            Severity::Minor,
                            crate::rule::AnalyzerSource::Other("built-in".into()),
                            format!(
                                "Enum '{}' should use PascalCase naming",
                                name
                            ),
                        )
                        .with_location(ctx.file_path.to_string(), line_number),
                    );
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

    fn make_context<'a>(
        content: &'a str,
        lines: &'a [&'a str],
        config: &'a RuleConfig,
    ) -> TsAnalysisContext<'a> {
        TsAnalysisContext {
            file_path: "test.ts",
            content,
            lines,
            config,
        }
    }

    #[test]
    fn valid_names_produce_no_issues() {
        let src = r#"
function getData() {
    return 1;
}

class UserService {
}

interface IUserRepository {
}

interface UserRepository {
}

type UserId = string;

enum Color {
    Red,
    Green,
}

const MAX_RETRIES = 5;
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context(src, &lines, &config);
        let rule = NamingConventionRule::default();
        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "expected no issues for valid naming, got: {:?}",
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
    }

    #[test]
    fn invalid_names_produce_issues() {
        let src = r#"
function GetData() {
    return 1;
}

class user_service {
}

interface bad_interface {
}

type my_type = string;

enum status_code {
    Ok,
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context(src, &lines, &config);
        let rule = NamingConventionRule::default();
        let issues = rule.analyze(&ctx);
        // We expect issues for: GetData (function not camelCase), user_service (class),
        // bad_interface (interface), my_type (type alias), status_code (enum)
        assert!(
            issues.len() >= 5,
            "expected at least 5 issues for invalid naming, got {}: {:?}",
            issues.len(),
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
    }

    #[test]
    fn react_component_pascal_case_is_accepted() {
        let src = r#"
function MyComponent(props) {
    return <div />;
}
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = TsAnalysisContext {
            file_path: "Component.tsx",
            content: src,
            lines: &lines,
            config: &config,
        };
        let rule = NamingConventionRule::default();
        let issues = rule.analyze(&ctx);
        assert!(
            issues.is_empty(),
            "expected no issues for PascalCase React component, got: {:?}",
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
    }

    #[test]
    fn helper_is_camel_case() {
        assert!(is_camel_case("getData"));
        assert!(is_camel_case("x"));
        assert!(is_camel_case("myFunc123"));
        assert!(!is_camel_case("GetData"));
        assert!(!is_camel_case("get_data"));
        assert!(!is_camel_case(""));
    }

    #[test]
    fn helper_is_pascal_case() {
        assert!(is_pascal_case("UserService"));
        assert!(is_pascal_case("A"));
        assert!(is_pascal_case("MyClass123"));
        assert!(!is_pascal_case("userService"));
        assert!(!is_pascal_case("User_Service"));
        assert!(!is_pascal_case(""));
        assert!(!is_pascal_case("ABC")); // all caps, not PascalCase
    }
}
