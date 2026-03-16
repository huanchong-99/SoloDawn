//! Rule: Nesting Depth
//!
//! Checks that control structure nesting within Rust functions does not exceed a configurable
//! maximum depth. Deeply nested code is hard to read and maintain.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustRule, RustAnalysisContext, RuleConfig};
use syn::visit::Visit;

/// Rule that flags functions whose control-flow nesting depth exceeds a threshold.
pub struct NestingDepthRule;

impl Default for NestingDepthRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for NestingDepthRule {
    fn id(&self) -> &str {
        "rust:nesting-depth"
    }

    fn name(&self) -> &str {
        "Nesting Depth"
    }

    fn description(&self) -> &str {
        "Checks that control structure nesting depth within functions does not exceed a maximum"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }

    fn default_config(&self) -> RuleConfig {
        RuleConfig::default()
    }
}

impl RustRule for NestingDepthRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let max_depth = ctx.config.get_param_usize("max_depth", 5);
        let severity = ctx.config.severity_override.unwrap_or_else(|| self.default_severity());

        let mut visitor = NestingDepthVisitor {
            max_depth,
            severity,
            rule_id: self.id().to_string(),
            file_path: ctx.file_path.to_string(),
            issues: Vec::new(),
        };

        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

struct NestingDepthVisitor {
    max_depth: usize,
    severity: Severity,
    rule_id: String,
    file_path: String,
    issues: Vec<QualityIssue>,
}

/// Per-function visitor that tracks nesting depth.
struct FunctionNestingVisitor {
    current_depth: usize,
    max_observed: usize,
}

impl FunctionNestingVisitor {
    fn new() -> Self {
        Self {
            current_depth: 0,
            max_observed: 0,
        }
    }

    fn enter(&mut self) {
        self.current_depth += 1;
        if self.current_depth > self.max_observed {
            self.max_observed = self.current_depth;
        }
    }

    fn leave(&mut self) {
        self.current_depth = self.current_depth.saturating_sub(1);
    }
}

impl<'ast> Visit<'ast> for FunctionNestingVisitor {
    fn visit_expr_if(&mut self, node: &'ast syn::ExprIf) {
        self.enter();
        syn::visit::visit_expr_if(self, node);
        self.leave();
    }

    fn visit_expr_match(&mut self, node: &'ast syn::ExprMatch) {
        self.enter();
        syn::visit::visit_expr_match(self, node);
        self.leave();
    }

    fn visit_expr_for_loop(&mut self, node: &'ast syn::ExprForLoop) {
        self.enter();
        syn::visit::visit_expr_for_loop(self, node);
        self.leave();
    }

    fn visit_expr_while(&mut self, node: &'ast syn::ExprWhile) {
        self.enter();
        syn::visit::visit_expr_while(self, node);
        self.leave();
    }

    fn visit_expr_loop(&mut self, node: &'ast syn::ExprLoop) {
        self.enter();
        syn::visit::visit_expr_loop(self, node);
        self.leave();
    }

    fn visit_expr_closure(&mut self, node: &'ast syn::ExprClosure) {
        self.enter();
        syn::visit::visit_expr_closure(self, node);
        self.leave();
    }

    fn visit_expr_block(&mut self, node: &'ast syn::ExprBlock) {
        self.enter();
        syn::visit::visit_expr_block(self, node);
        self.leave();
    }

    // Do not recurse into nested function items — they get their own check.
    fn visit_item_fn(&mut self, _node: &'ast syn::ItemFn) {}
}

impl NestingDepthVisitor {
    fn check_block(&mut self, name: &str, block: &syn::Block, start_line: usize) {
        let mut inner = FunctionNestingVisitor::new();
        inner.visit_block(block);

        if inner.max_observed > self.max_depth {
            let issue = QualityIssue::new(
                self.rule_id.clone(),
                RuleType::CodeSmell,
                self.severity,
                crate::rule::AnalyzerSource::Other("built-in".to_string()),
                format!(
                    "Function `{}` has a nesting depth of {}, which exceeds the maximum of {}",
                    name, inner.max_observed, self.max_depth
                ),
            )
            .with_location(self.file_path.clone(), start_line as u32);

            self.issues.push(issue);
        }
    }
}

impl<'ast> Visit<'ast> for NestingDepthVisitor {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        let name = node.sig.ident.to_string();
        let start_line = node.sig.ident.span().start().line;
        self.check_block(&name, &node.block, start_line);
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        let name = node.sig.ident.to_string();
        let start_line = node.sig.ident.span().start().line;
        self.check_block(&name, &node.block, start_line);
        syn::visit::visit_impl_item_fn(self, node);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_code(source: &str, max_depth: usize) -> Vec<QualityIssue> {
        let syntax = syn::parse_file(source).expect("failed to parse test source");
        let config = RuleConfig {
            enabled: true,
            severity_override: None,
            params: [("max_depth".to_string(), max_depth.to_string())]
                .into_iter()
                .collect(),
        };
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: source,
            syntax: &syntax,
            config: &config,
        };
        let rule = NestingDepthRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn shallow_function_no_issue() {
        let source = r#"
fn shallow() {
    if true {
        let x = 1;
    }
}
"#;
        let issues = analyze_code(source, 5);
        assert!(issues.is_empty(), "shallow nesting should not trigger an issue");
    }

    #[test]
    fn deeply_nested_function_triggers_issue() {
        let source = r#"
fn deep() {
    if true {
        if true {
            for i in 0..10 {
                while false {
                    loop {
                        match i {
                            _ => {}
                        }
                    }
                }
            }
        }
    }
}
"#;
        let issues = analyze_code(source, 5);
        assert!(!issues.is_empty(), "deeply nested function should trigger an issue");
        assert!(
            issues[0].message.contains("deep"),
            "issue message should contain function name"
        );
    }

    #[test]
    fn respects_custom_max_depth() {
        let source = r#"
fn nested() {
    if true {
        if true {
            if true {
                let x = 1;
            }
        }
    }
}
"#;
        // depth 3 with max 2 should trigger
        let issues = analyze_code(source, 2);
        assert!(!issues.is_empty(), "should trigger with max_depth=2");

        // depth 3 with max 5 should not trigger
        let issues = analyze_code(source, 5);
        assert!(issues.is_empty(), "should not trigger with max_depth=5");
    }
}
