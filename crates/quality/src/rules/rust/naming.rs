//! Rule: Naming Convention
//!
//! Checks Rust naming conventions: snake_case for functions/variables,
//! CamelCase for types/traits, UPPER_SNAKE_CASE for constants/statics.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustRule, RustAnalysisContext, RuleConfig};
use syn::visit::Visit;

/// Rule that checks Rust naming conventions.
pub struct NamingConventionRule;

impl Default for NamingConventionRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for NamingConventionRule {
    fn id(&self) -> &str {
        "rust:naming"
    }

    fn name(&self) -> &str {
        "Naming Convention"
    }

    fn description(&self) -> &str {
        "Checks that identifiers follow Rust naming conventions: snake_case for functions/variables, \
         CamelCase for types/traits, UPPER_SNAKE_CASE for constants/statics"
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

impl RustRule for NamingConventionRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx.config.severity_override.unwrap_or_else(|| self.default_severity());

        let mut visitor = NamingVisitor {
            severity,
            rule_id: self.id().to_string(),
            file_path: ctx.file_path.to_string(),
            issues: Vec::new(),
        };

        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

struct NamingVisitor {
    severity: Severity,
    rule_id: String,
    file_path: String,
    issues: Vec<QualityIssue>,
}

impl NamingVisitor {
    fn report(&mut self, message: String, line: usize) {
        let issue = QualityIssue::new(
            self.rule_id.clone(),
            RuleType::CodeSmell,
            self.severity,
            crate::rule::AnalyzerSource::Other("built-in".to_string()),
            message,
        )
        .with_location(self.file_path.clone(), line as u32);
        self.issues.push(issue);
    }
}

impl<'ast> Visit<'ast> for NamingVisitor {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        let name = node.sig.ident.to_string();
        if !name.starts_with('_') && !is_snake_case(&name) {
            let line = node.sig.ident.span().start().line;
            self.report(
                format!(
                    "Function `{}` should use snake_case naming",
                    name
                ),
                line,
            );
        }
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_item_struct(&mut self, node: &'ast syn::ItemStruct) {
        let name = node.ident.to_string();
        if !name.starts_with('_') && !is_upper_camel_case(&name) {
            let line = node.ident.span().start().line;
            self.report(
                format!(
                    "Struct `{}` should use CamelCase naming",
                    name
                ),
                line,
            );
        }
        syn::visit::visit_item_struct(self, node);
    }

    fn visit_item_enum(&mut self, node: &'ast syn::ItemEnum) {
        let name = node.ident.to_string();
        if !name.starts_with('_') && !is_upper_camel_case(&name) {
            let line = node.ident.span().start().line;
            self.report(
                format!(
                    "Enum `{}` should use CamelCase naming",
                    name
                ),
                line,
            );
        }
        syn::visit::visit_item_enum(self, node);
    }

    fn visit_item_trait(&mut self, node: &'ast syn::ItemTrait) {
        let name = node.ident.to_string();
        if !name.starts_with('_') && !is_upper_camel_case(&name) {
            let line = node.ident.span().start().line;
            self.report(
                format!(
                    "Trait `{}` should use CamelCase naming",
                    name
                ),
                line,
            );
        }
        syn::visit::visit_item_trait(self, node);
    }

    fn visit_item_const(&mut self, node: &'ast syn::ItemConst) {
        let name = node.ident.to_string();
        if !name.starts_with('_') && !is_upper_snake_case(&name) {
            let line = node.ident.span().start().line;
            self.report(
                format!(
                    "Constant `{}` should use UPPER_SNAKE_CASE naming",
                    name
                ),
                line,
            );
        }
        syn::visit::visit_item_const(self, node);
    }

    fn visit_item_static(&mut self, node: &'ast syn::ItemStatic) {
        let name = node.ident.to_string();
        if !name.starts_with('_') && !is_upper_snake_case(&name) {
            let line = node.ident.span().start().line;
            self.report(
                format!(
                    "Static `{}` should use UPPER_SNAKE_CASE naming",
                    name
                ),
                line,
            );
        }
        syn::visit::visit_item_static(self, node);
    }
}

/// Returns `true` if `s` is valid snake_case: lowercase letters, digits, and underscores only,
/// must not start with a digit, and must not contain consecutive underscores.
fn is_snake_case(s: &str) -> bool {
    if s.is_empty() {
        return false;
    }
    let mut prev_underscore = false;
    for (i, c) in s.chars().enumerate() {
        if c == '_' {
            if prev_underscore || i == 0 {
                // leading or consecutive underscores are unusual but we allow leading underscore
                // only via the skip-`_` check in the visitor, so reject here
                return false;
            }
            prev_underscore = true;
        } else if c.is_ascii_lowercase() || c.is_ascii_digit() {
            prev_underscore = false;
        } else {
            return false;
        }
    }
    // Must not end with underscore
    !prev_underscore
}

/// Returns `true` if `s` is valid UpperCamelCase: starts with an uppercase letter,
/// contains only alphanumeric characters, and has no underscores.
fn is_upper_camel_case(s: &str) -> bool {
    if s.is_empty() {
        return false;
    }
    let mut chars = s.chars();
    let first = chars.next().unwrap();
    if !first.is_ascii_uppercase() {
        return false;
    }
    // Must contain at least one lowercase letter (to distinguish from UPPER_SNAKE_CASE)
    let mut has_lower = false;
    for c in chars {
        if !c.is_ascii_alphanumeric() {
            return false;
        }
        if c.is_ascii_lowercase() {
            has_lower = true;
        }
    }
    // Single char uppercase is OK (e.g., type T)
    has_lower || s.len() == 1
}

/// Returns `true` if `s` is valid UPPER_SNAKE_CASE: uppercase letters, digits, and underscores only,
/// must start with an uppercase letter, and must not contain consecutive underscores.
fn is_upper_snake_case(s: &str) -> bool {
    if s.is_empty() {
        return false;
    }
    let first = s.chars().next().unwrap();
    if !first.is_ascii_uppercase() {
        return false;
    }
    let mut prev_underscore = false;
    for c in s.chars() {
        if c == '_' {
            if prev_underscore {
                return false;
            }
            prev_underscore = true;
        } else if c.is_ascii_uppercase() || c.is_ascii_digit() {
            prev_underscore = false;
        } else {
            return false;
        }
    }
    !prev_underscore
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_code(source: &str) -> Vec<QualityIssue> {
        let syntax = syn::parse_file(source).expect("failed to parse test source");
        let config = RuleConfig::default();
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: source,
            syntax: &syntax,
            config: &config,
        };
        let rule = NamingConventionRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn correct_naming_no_issues() {
        let source = r#"
fn my_function() {}
struct MyStruct;
enum MyEnum { A, B }
trait MyTrait {}
const MAX_SIZE: usize = 100;
static GLOBAL_COUNT: u32 = 0;
"#;
        let issues = analyze_code(source);
        assert!(issues.is_empty(), "correctly named items should produce no issues, got: {:?}", issues.iter().map(|i| &i.message).collect::<Vec<_>>());
    }

    #[test]
    fn bad_function_name_triggers_issue() {
        let source = r#"
fn MyBadFunction() {}
"#;
        let issues = analyze_code(source);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("MyBadFunction"));
        assert!(issues[0].message.contains("snake_case"));
    }

    #[test]
    fn bad_struct_name_triggers_issue() {
        let source = r#"
struct bad_struct;
"#;
        let issues = analyze_code(source);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("bad_struct"));
        assert!(issues[0].message.contains("CamelCase"));
    }

    #[test]
    fn bad_constant_name_triggers_issue() {
        let source = r#"
const myConst: i32 = 42;
"#;
        let issues = analyze_code(source);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("myConst"));
        assert!(issues[0].message.contains("UPPER_SNAKE_CASE"));
    }

    #[test]
    fn underscore_prefixed_names_are_skipped() {
        let source = r#"
fn _internal() {}
struct _Hidden;
const _secret: i32 = 0;
"#;
        let issues = analyze_code(source);
        assert!(issues.is_empty(), "underscore-prefixed names should be skipped");
    }

    #[test]
    fn helper_is_snake_case() {
        assert!(is_snake_case("hello"));
        assert!(is_snake_case("hello_world"));
        assert!(is_snake_case("a1_b2"));
        assert!(!is_snake_case("Hello"));
        assert!(!is_snake_case("helloWorld"));
        assert!(!is_snake_case("hello__world"));
        assert!(!is_snake_case(""));
    }

    #[test]
    fn helper_is_upper_camel_case() {
        assert!(is_upper_camel_case("Hello"));
        assert!(is_upper_camel_case("HelloWorld"));
        assert!(is_upper_camel_case("H"));
        assert!(is_upper_camel_case("Vec3"));
        assert!(!is_upper_camel_case("hello"));
        assert!(!is_upper_camel_case("hello_world"));
        assert!(!is_upper_camel_case("HELLO"));
        assert!(!is_upper_camel_case(""));
    }

    #[test]
    fn helper_is_upper_snake_case() {
        assert!(is_upper_snake_case("MAX"));
        assert!(is_upper_snake_case("MAX_SIZE"));
        assert!(is_upper_snake_case("A1_B2"));
        assert!(!is_upper_snake_case("max"));
        assert!(!is_upper_snake_case("Max_Size"));
        assert!(!is_upper_snake_case("MAX__SIZE"));
        assert!(!is_upper_snake_case(""));
    }
}
