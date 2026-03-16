//! Cyclomatic complexity rule for Rust source files.
//!
//! Calculates cyclomatic complexity for each function/method using the `syn` crate's
//! Visitor pattern. Complexity = 1 + number of decision points.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustAnalysisContext, RustRule};
use syn::visit::Visit;

/// Built-in rule that checks cyclomatic complexity of Rust functions and methods.
#[derive(Debug, Default)]
pub struct CyclomaticComplexityRule;

impl Rule for CyclomaticComplexityRule {
    fn id(&self) -> &str {
        "rust:cyclomatic-complexity"
    }

    fn name(&self) -> &str {
        "Cyclomatic Complexity"
    }

    fn description(&self) -> &str {
        "Checks that functions and methods do not exceed a cyclomatic complexity threshold. \
         High cyclomatic complexity indicates code that is difficult to test and maintain."
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }
}

impl RustRule for CyclomaticComplexityRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let threshold = ctx.config.get_param_usize("threshold", 15);
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let mut collector = FunctionCollector {
            functions: Vec::new(),
        };
        syn::visit::visit_file(&mut collector, ctx.syntax);

        let mut issues = Vec::new();
        for func in &collector.functions {
            if func.complexity > threshold {
                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    crate::rule::AnalyzerSource::Other("built-in".to_string()),
                    format!(
                        "Function `{}` has a cyclomatic complexity of {} (threshold: {})",
                        func.name, func.complexity, threshold,
                    ),
                )
                .with_location(ctx.file_path, func.line)
                .with_effort(((func.complexity - threshold) as i32) * 5);
                issues.push(issue);
            }
        }

        issues
    }
}

/// Information about a single function found during AST traversal.
struct FunctionInfo {
    name: String,
    line: u32,
    complexity: usize,
}

/// Collects all top-level and nested functions/methods from the AST.
struct FunctionCollector {
    functions: Vec<FunctionInfo>,
}

impl<'ast> Visit<'ast> for FunctionCollector {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        let name = node.sig.ident.to_string();
        let line = node.sig.ident.span().start().line as u32;
        let complexity = compute_complexity_block(&node.block);
        self.functions.push(FunctionInfo {
            name,
            line,
            complexity,
        });
        // Continue visiting to find nested functions
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        let name = node.sig.ident.to_string();
        let line = node.sig.ident.span().start().line as u32;
        let complexity = compute_complexity_block(&node.block);
        self.functions.push(FunctionInfo {
            name,
            line,
            complexity,
        });
        syn::visit::visit_impl_item_fn(self, node);
    }
}

/// Compute cyclomatic complexity for a function block.
fn compute_complexity_block(block: &syn::Block) -> usize {
    let mut counter = ComplexityCounter { complexity: 1 };
    syn::visit::visit_block(&mut counter, block);
    counter.complexity
}

/// Visitor that counts decision points within a single function body.
struct ComplexityCounter {
    complexity: usize,
}

impl<'ast> Visit<'ast> for ComplexityCounter {
    fn visit_expr_if(&mut self, node: &'ast syn::ExprIf) {
        // Count the `if` (including `if let`)
        self.complexity += 1;
        // Each `else if` is visited recursively as another ExprIf
        syn::visit::visit_expr_if(self, node);
    }

    fn visit_expr_match(&mut self, node: &'ast syn::ExprMatch) {
        // Each arm after the first adds a path
        if node.arms.len() > 1 {
            self.complexity += node.arms.len() - 1;
        }
        syn::visit::visit_expr_match(self, node);
    }

    fn visit_expr_while(&mut self, node: &'ast syn::ExprWhile) {
        // Covers both `while` and `while let`
        self.complexity += 1;
        syn::visit::visit_expr_while(self, node);
    }

    fn visit_expr_for_loop(&mut self, node: &'ast syn::ExprForLoop) {
        self.complexity += 1;
        syn::visit::visit_expr_for_loop(self, node);
    }

    fn visit_expr_loop(&mut self, node: &'ast syn::ExprLoop) {
        self.complexity += 1;
        syn::visit::visit_expr_loop(self, node);
    }

    fn visit_expr_try(&mut self, node: &'ast syn::ExprTry) {
        // The `?` operator
        self.complexity += 1;
        syn::visit::visit_expr_try(self, node);
    }

    fn visit_expr_binary(&mut self, node: &'ast syn::ExprBinary) {
        // Count `&&` and `||` as decision points
        match node.op {
            syn::BinOp::And(_) | syn::BinOp::Or(_) => {
                self.complexity += 1;
            }
            _ => {}
        }
        syn::visit::visit_expr_binary(self, node);
    }

    // Do NOT descend into nested function definitions — they get their own entry
    fn visit_item_fn(&mut self, _node: &'ast syn::ItemFn) {
        // Skip: nested functions are collected separately by FunctionCollector
    }

    fn visit_expr_closure(&mut self, node: &'ast syn::ExprClosure) {
        // Closures are part of the enclosing function's complexity
        syn::visit::visit_expr_closure(self, node);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_source(source: &str) -> Vec<QualityIssue> {
        let syntax: syn::File = syn::parse_str(source).expect("failed to parse source");
        let config = RuleConfig::default();
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: source,
            syntax: &syntax,
            config: &config,
        };
        let rule = CyclomaticComplexityRule;
        rule.analyze(&ctx)
    }

    fn analyze_source_with_threshold(source: &str, threshold: usize) -> Vec<QualityIssue> {
        let syntax: syn::File = syn::parse_str(source).expect("failed to parse source");
        let mut config = RuleConfig::default();
        config
            .params
            .insert("threshold".to_string(), threshold.to_string());
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: source,
            syntax: &syntax,
            config: &config,
        };
        let rule = CyclomaticComplexityRule;
        rule.analyze(&ctx)
    }

    #[test]
    fn simple_function_below_threshold() {
        let source = r#"
            fn simple_add(a: i32, b: i32) -> i32 {
                a + b
            }
        "#;
        let issues = analyze_source(source);
        assert!(issues.is_empty(), "Simple function should have no issues");
    }

    #[test]
    fn complex_function_above_threshold() {
        // Build a function with complexity well above the default threshold of 15.
        // Each `if` adds 1, each `&&` adds 1, each `?` adds 1, etc.
        let source = r#"
            fn very_complex(x: i32) -> Result<i32, ()> {
                if x > 0 {            // +1 = 2
                    if x > 10 {        // +1 = 3
                        if x > 100 {   // +1 = 4
                            return Ok(x);
                        }
                    }
                } else if x < -100 {   // +1 = 5
                    return Err(());
                }

                if x == 1 || x == 2 || x == 3 { // +1(if) +1(||) +1(||) = 8
                    return Ok(1);
                }

                if x == 4 && x == 5 && x == 6 { // +1(if) +1(&&) +1(&&) = 11
                    return Ok(2);
                }

                for i in 0..x {        // +1 = 12
                    if i > 5 {         // +1 = 13
                        let _ = some_fn()?;  // +1 = 14
                        let _ = other_fn()?; // +1 = 15
                        let _ = third_fn()?; // +1 = 16
                    }
                }

                Ok(x)
            }

            fn some_fn() -> Result<(), ()> { Ok(()) }
            fn other_fn() -> Result<(), ()> { Ok(()) }
            fn third_fn() -> Result<(), ()> { Ok(()) }
        "#;

        let issues = analyze_source(source);
        assert_eq!(issues.len(), 1, "Should flag exactly the complex function");
        assert!(issues[0].message.contains("very_complex"));
        assert!(issues[0].message.contains("cyclomatic complexity"));
    }

    #[test]
    fn multiple_functions_in_one_file() {
        // With threshold=2, the second function should be flagged.
        let source = r#"
            fn simple() -> i32 {
                42
            }

            fn moderate(x: bool, y: bool) -> i32 {
                if x {          // +1 = 2
                    if y {      // +1 = 3
                        1
                    } else {
                        2
                    }
                } else {
                    3
                }
            }

            fn also_simple(a: i32) -> i32 {
                a * 2
            }
        "#;

        let issues = analyze_source_with_threshold(source, 2);
        assert_eq!(
            issues.len(),
            1,
            "Only the moderate function should exceed threshold of 2"
        );
        assert!(issues[0].message.contains("moderate"));
    }

    #[test]
    fn match_arms_add_complexity() {
        let source = r#"
            fn with_match(x: i32) -> &'static str {
                match x {
                    1 => "one",
                    2 => "two",
                    3 => "three",
                    4 => "four",
                    _ => "other",
                }
            }
        "#;

        // 1 (base) + 4 (5 arms - 1) = 5
        let issues = analyze_source_with_threshold(source, 4);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("with_match"));

        // With threshold of 5 it should pass
        let issues = analyze_source_with_threshold(source, 5);
        assert!(issues.is_empty());
    }

    #[test]
    fn loop_and_while_add_complexity() {
        let source = r#"
            fn loopy(items: &[i32]) -> i32 {
                let mut sum = 0;
                for item in items {       // +1 = 2
                    sum += item;
                }
                while sum > 100 {         // +1 = 3
                    sum -= 10;
                }
                loop {                    // +1 = 4
                    if sum < 0 {          // +1 = 5
                        break;
                    }
                    sum -= 1;
                }
                sum
            }
        "#;

        let issues = analyze_source_with_threshold(source, 4);
        assert_eq!(issues.len(), 1);

        let issues = analyze_source_with_threshold(source, 5);
        assert!(issues.is_empty());
    }
}
