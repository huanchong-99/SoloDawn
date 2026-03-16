//! Detects `unsafe` blocks, `unsafe fn` declarations, and `unsafe impl` blocks
//! for security review.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{Rule, RustAnalysisContext, RustRule};
use syn::visit::Visit;

/// Detects unsafe code usage that requires security review.
#[derive(Debug, Default)]
pub struct UnsafeUsageRule;

impl Rule for UnsafeUsageRule {
    fn id(&self) -> &str {
        "rust:unsafe-usage"
    }

    fn name(&self) -> &str {
        "Unsafe Usage"
    }

    fn description(&self) -> &str {
        "Detects unsafe blocks, unsafe fn declarations, and unsafe impl blocks for security review"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::SecurityHotspot
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }
}

/// Visitor that collects unsafe usage locations.
struct UnsafeVisitor<'a> {
    file_path: &'a str,
    issues: Vec<QualityIssue>,
}

impl<'a> UnsafeVisitor<'a> {
    fn new(file_path: &'a str) -> Self {
        Self {
            file_path,
            issues: Vec::new(),
        }
    }

    fn add_issue(&mut self, kind: &str, line: usize) {
        let message = format!("Unsafe {} requires security review", kind);
        let issue = QualityIssue::new(
            "rust:unsafe-usage",
            RuleType::SecurityHotspot,
            Severity::Major,
            AnalyzerSource::Other("built-in".to_string()),
            message,
        )
        .with_location(self.file_path, line as u32)
        .with_effort(15)
        .with_context(format!("unsafe {kind} detected"));
        self.issues.push(issue);
    }
}

impl<'ast> Visit<'ast> for UnsafeVisitor<'_> {
    fn visit_expr_unsafe(&mut self, node: &'ast syn::ExprUnsafe) {
        let line = node.unsafe_token.span.start().line;
        self.add_issue("block", line);
        // Continue visiting nested nodes
        syn::visit::visit_expr_unsafe(self, node);
    }

    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        if let Some(ref unsafety) = node.sig.unsafety {
            let line = unsafety.span.start().line;
            self.add_issue("fn", line);
        }
        // Continue visiting the function body for nested unsafe blocks
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        if let Some(ref unsafety) = node.sig.unsafety {
            let line = unsafety.span.start().line;
            self.add_issue("fn", line);
        }
        syn::visit::visit_impl_item_fn(self, node);
    }

    fn visit_item_impl(&mut self, node: &'ast syn::ItemImpl) {
        if let Some(ref unsafety) = node.unsafety {
            let line = unsafety.span.start().line;
            self.add_issue("impl", line);
        }
        // Continue visiting items inside the impl block
        syn::visit::visit_item_impl(self, node);
    }
}

impl RustRule for UnsafeUsageRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let mut visitor = UnsafeVisitor::new(ctx.file_path);
        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_code(code: &str) -> Vec<QualityIssue> {
        let syntax = syn::parse_file(code).expect("failed to parse test code");
        let config = RuleConfig::default();
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: code,
            syntax: &syntax,
            config: &config,
        };
        let rule = UnsafeUsageRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn detects_unsafe_block_and_unsafe_fn() {
        let code = r#"
unsafe fn dangerous() {
    let ptr = std::ptr::null::<i32>();
}

fn safe_wrapper() {
    unsafe {
        dangerous();
    }
}
"#;
        let issues = analyze_code(code);
        assert_eq!(issues.len(), 2, "expected 2 issues: unsafe fn + unsafe block");

        let fn_issue = issues.iter().find(|i| i.message.contains("fn")).unwrap();
        assert_eq!(fn_issue.rule_id, "rust:unsafe-usage");
        assert_eq!(fn_issue.severity, Severity::Major);
        assert_eq!(fn_issue.rule_type, RuleType::SecurityHotspot);

        let block_issue = issues.iter().find(|i| i.message.contains("block")).unwrap();
        assert_eq!(block_issue.rule_id, "rust:unsafe-usage");
    }

    #[test]
    fn detects_unsafe_impl() {
        let code = r#"
struct MyType;

unsafe impl Send for MyType {}
"#;
        let issues = analyze_code(code);
        assert_eq!(issues.len(), 1, "expected 1 issue for unsafe impl");

        let issue = &issues[0];
        assert!(issue.message.contains("impl"));
        assert_eq!(issue.rule_type, RuleType::SecurityHotspot);
        assert_eq!(issue.severity, Severity::Major);
    }

    #[test]
    fn no_issues_for_safe_code() {
        let code = r#"
fn add(a: i32, b: i32) -> i32 {
    a + b
}
"#;
        let issues = analyze_code(code);
        assert!(issues.is_empty(), "safe code should produce no issues");
    }
}
