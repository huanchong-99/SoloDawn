//! Cognitive Complexity rule for Rust source files.
//!
//! Calculates cognitive complexity for each function/method. Unlike cyclomatic complexity,
//! cognitive complexity penalizes nesting (each nested level adds +1 weight) and counts
//! breaks in linear flow.

use syn::visit::Visit;

use crate::{
    issue::QualityIssue,
    rule::{RuleType, Severity},
    rules::{Rule, RuleConfig, RustAnalysisContext, RustRule},
};

/// Rule that checks cognitive complexity of Rust functions/methods.
#[derive(Default)]
pub struct CognitiveComplexityRule {
    _private: (),
}


impl Rule for CognitiveComplexityRule {
    fn id(&self) -> &str {
        "rust:cognitive-complexity"
    }

    fn name(&self) -> &str {
        "Cognitive Complexity"
    }

    fn description(&self) -> &str {
        "Checks that cognitive complexity of functions does not exceed a configurable threshold"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }

    fn default_config(&self) -> RuleConfig {
        let mut config = RuleConfig::default();
        config
            .params
            .insert("threshold".to_string(), "20".to_string());
        config
    }
}

impl RustRule for CognitiveComplexityRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let threshold = ctx.config.get_param_usize("threshold", 20);
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let mut issues = Vec::new();
        let mut collector = FunctionCollector {
            results: Vec::new(),
        };
        collector.visit_file(ctx.syntax);

        for func_result in collector.results {
            if func_result.complexity > threshold {
                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    crate::rule::AnalyzerSource::Other("built-in".to_string()),
                    format!(
                        "Function `{}` has a cognitive complexity of {} (threshold: {})",
                        func_result.name, func_result.complexity, threshold
                    ),
                )
                .with_location(ctx.file_path, func_result.line)
                .with_effort(((func_result.complexity - threshold) as i32) * 5);
                issues.push(issue);
            }
        }

        issues
    }
}

/// Result for a single function/method analysis.
struct FunctionResult {
    name: String,
    line: u32,
    complexity: usize,
}

/// Visitor that collects all functions and computes their cognitive complexity.
struct FunctionCollector {
    results: Vec<FunctionResult>,
}

impl<'ast> Visit<'ast> for FunctionCollector {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        let name = node.sig.ident.to_string();
        let line = line_of_ident(&node.sig.ident);
        let complexity = compute_cognitive_complexity_block(&node.block);
        self.results.push(FunctionResult {
            name,
            line,
            complexity,
        });
        // Continue visiting to find nested functions
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        let name = node.sig.ident.to_string();
        let line = line_of_ident(&node.sig.ident);
        let complexity = compute_cognitive_complexity_block(&node.block);
        self.results.push(FunctionResult {
            name,
            line,
            complexity,
        });
        syn::visit::visit_impl_item_fn(self, node);
    }
}

fn line_of_ident(ident: &syn::Ident) -> u32 {
    ident.span().start().line as u32
}

/// Compute cognitive complexity for a block of statements.
fn compute_cognitive_complexity_block(block: &syn::Block) -> usize {
    let mut visitor = CognitiveVisitor {
        complexity: 0,
        nesting: 0,
    };
    for stmt in &block.stmts {
        visitor.visit_stmt(stmt);
    }
    visitor.complexity
}

/// Internal visitor that walks expressions to calculate cognitive complexity.
struct CognitiveVisitor {
    complexity: usize,
    nesting: usize,
}

impl CognitiveVisitor {
    /// Add an increment with nesting penalty: base 1 + current nesting depth.
    fn increment_with_nesting(&mut self) {
        self.complexity += 1 + self.nesting;
    }

    /// Add a flat increment of 1 (no nesting penalty).
    fn increment_flat(&mut self) {
        self.complexity += 1;
    }

    /// Process an expression, handling control flow constructs.
    fn visit_expr_for_complexity(&mut self, expr: &syn::Expr) {
        match expr {
            syn::Expr::If(expr_if) => {
                self.visit_if_expr(expr_if, true);
            }
            syn::Expr::Match(expr_match) => {
                // +1 with nesting for the match keyword
                self.increment_with_nesting();
                self.nesting += 1;
                for arm in &expr_match.arms {
                    // Match arms themselves don't add complexity; only the body does
                    self.visit_expr_for_complexity(&arm.body);
                    if let Some(ref guard) = arm.guard {
                        self.scan_logical_operators(&guard.1);
                    }
                }
                self.nesting -= 1;
            }
            syn::Expr::ForLoop(expr_for) => {
                self.increment_with_nesting();
                self.nesting += 1;
                for stmt in &expr_for.body.stmts {
                    self.visit_stmt(stmt);
                }
                self.nesting -= 1;
            }
            syn::Expr::While(expr_while) => {
                self.increment_with_nesting();
                self.nesting += 1;
                self.scan_logical_operators(&expr_while.cond);
                for stmt in &expr_while.body.stmts {
                    self.visit_stmt(stmt);
                }
                self.nesting -= 1;
            }
            syn::Expr::Loop(expr_loop) => {
                self.increment_with_nesting();
                self.nesting += 1;
                for stmt in &expr_loop.body.stmts {
                    self.visit_stmt(stmt);
                }
                self.nesting -= 1;
            }
            syn::Expr::Break(expr_break) => {
                if expr_break.label.is_some() {
                    self.increment_flat();
                }
                if let Some(ref val) = expr_break.expr {
                    self.visit_expr_for_complexity(val);
                }
            }
            syn::Expr::Continue(expr_continue) => {
                if expr_continue.label.is_some() {
                    self.increment_flat();
                }
            }
            syn::Expr::Binary(_) => {
                // Handle logical operators
                self.scan_logical_operators(expr);
                // Don't recurse further for binary exprs — scan_logical_operators handles it
            }
            syn::Expr::Block(expr_block) => {
                for stmt in &expr_block.block.stmts {
                    self.visit_stmt(stmt);
                }
            }
            syn::Expr::Closure(expr_closure) => {
                self.nesting += 1;
                self.visit_expr_for_complexity(&expr_closure.body);
                self.nesting -= 1;
            }
            syn::Expr::Return(expr_return) => {
                if let Some(ref val) = expr_return.expr {
                    self.visit_expr_for_complexity(val);
                }
            }
            syn::Expr::Assign(expr_assign) => {
                self.visit_expr_for_complexity(&expr_assign.right);
            }
            syn::Expr::Call(expr_call) => {
                self.visit_expr_for_complexity(&expr_call.func);
                for arg in &expr_call.args {
                    self.visit_expr_for_complexity(arg);
                }
            }
            syn::Expr::MethodCall(expr_method) => {
                self.visit_expr_for_complexity(&expr_method.receiver);
                for arg in &expr_method.args {
                    self.visit_expr_for_complexity(arg);
                }
            }
            syn::Expr::Tuple(expr_tuple) => {
                for elem in &expr_tuple.elems {
                    self.visit_expr_for_complexity(elem);
                }
            }
            syn::Expr::Array(expr_array) => {
                for elem in &expr_array.elems {
                    self.visit_expr_for_complexity(elem);
                }
            }
            syn::Expr::Reference(expr_ref) => {
                self.visit_expr_for_complexity(&expr_ref.expr);
            }
            syn::Expr::Unary(expr_unary) => {
                self.visit_expr_for_complexity(&expr_unary.expr);
            }
            syn::Expr::Let(expr_let) => {
                self.visit_expr_for_complexity(&expr_let.expr);
            }
            syn::Expr::Paren(expr_paren) => {
                self.visit_expr_for_complexity(&expr_paren.expr);
            }
            syn::Expr::Field(expr_field) => {
                self.visit_expr_for_complexity(&expr_field.base);
            }
            syn::Expr::Index(expr_index) => {
                self.visit_expr_for_complexity(&expr_index.expr);
                self.visit_expr_for_complexity(&expr_index.index);
            }
            syn::Expr::Try(expr_try) => {
                self.visit_expr_for_complexity(&expr_try.expr);
            }
            syn::Expr::Struct(expr_struct) => {
                for field in &expr_struct.fields {
                    self.visit_expr_for_complexity(&field.expr);
                }
            }
            syn::Expr::Unsafe(expr_unsafe) => {
                for stmt in &expr_unsafe.block.stmts {
                    self.visit_stmt(stmt);
                }
            }
            _ => {}
        }
    }

    fn visit_if_expr(&mut self, expr_if: &syn::ExprIf, is_first: bool) {
        if is_first {
            // First `if`: +1 with nesting
            self.increment_with_nesting();
        } else {
            // `else if`: +1 flat (no nesting penalty)
            self.increment_flat();
        }

        // Scan condition for logical operators
        self.scan_logical_operators(&expr_if.cond);

        // Visit the then-branch with increased nesting
        self.nesting += 1;
        for stmt in &expr_if.then_branch.stmts {
            self.visit_stmt(stmt);
        }
        self.nesting -= 1;

        // Visit else branch
        if let Some((_, ref else_expr)) = expr_if.else_branch {
            match else_expr.as_ref() {
                syn::Expr::If(nested_if) => {
                    // else if — flat increment handled recursively
                    self.visit_if_expr(nested_if, false);
                }
                syn::Expr::Block(else_block) => {
                    // plain else
                    self.increment_flat();
                    self.nesting += 1;
                    for stmt in &else_block.block.stmts {
                        self.visit_stmt(stmt);
                    }
                    self.nesting -= 1;
                }
                other => {
                    self.increment_flat();
                    self.nesting += 1;
                    self.visit_expr_for_complexity(other);
                    self.nesting -= 1;
                }
            }
        }
    }

    /// Scan a condition expression for logical operator sequences.
    /// First operator in a chain: +1, same operator continues: +0, switch: +1.
    fn scan_logical_operators(&mut self, expr: &syn::Expr) {
        let mut ops = Vec::new();
        collect_logical_ops(expr, &mut ops);

        let mut last_op: Option<LogicalOp> = None;
        for op in ops {
            match last_op {
                None => {
                    // First logical operator in sequence
                    self.increment_flat();
                    last_op = Some(op);
                }
                Some(prev) if prev == op => {
                    // Same operator, no increment
                }
                Some(_) => {
                    // Switched operator
                    self.increment_flat();
                    last_op = Some(op);
                }
            }
        }
    }

    fn visit_stmt(&mut self, stmt: &syn::Stmt) {
        match stmt {
            syn::Stmt::Local(local) => {
                if let Some(ref init) = local.init {
                    self.visit_expr_for_complexity(&init.expr);
                    if let Some((_, ref diverge)) = init.diverge {
                        self.visit_expr_for_complexity(diverge);
                    }
                }
            }
            syn::Stmt::Expr(expr, _) => {
                self.visit_expr_for_complexity(expr);
            }
            syn::Stmt::Item(_) => {
                // Nested items (functions, etc.) are handled by the outer FunctionCollector
            }
            syn::Stmt::Macro(_) => {}
        }
    }
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum LogicalOp {
    And,
    Or,
}

/// Recursively collect logical operators from a binary expression chain in left-to-right order.
fn collect_logical_ops(expr: &syn::Expr, ops: &mut Vec<LogicalOp>) {
    match expr {
        syn::Expr::Binary(bin) => {
            let op = match bin.op {
                syn::BinOp::And(_) => Some(LogicalOp::And),
                syn::BinOp::Or(_) => Some(LogicalOp::Or),
                _ => None,
            };
            if let Some(logical_op) = op {
                collect_logical_ops(&bin.left, ops);
                ops.push(logical_op);
                collect_logical_ops(&bin.right, ops);
            }
            // Non-logical binary ops: don't contribute to logical operator chain
        }
        syn::Expr::Paren(paren) => {
            // Parenthesized expressions don't break the chain
            collect_logical_ops(&paren.expr, ops);
        }
        _ => {}
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_source(source: &str, threshold: usize) -> Vec<QualityIssue> {
        let syntax = syn::parse_file(source).expect("failed to parse test source");
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
        let rule = CognitiveComplexityRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn simple_function_below_threshold() {
        let source = r#"
            fn simple() {
                let x = 1;
                let y = x + 2;
                println!("{}", y);
            }
        "#;
        let issues = analyze_source(source, 20);
        assert!(issues.is_empty(), "Simple function should have no issues");
    }

    #[test]
    fn nested_control_flow_exceeds_threshold() {
        // Build a function with high cognitive complexity via nesting:
        // if (+1, nest=0) -> for (+2, nest=1) -> if (+3, nest=2) -> match (+4, nest=3) = 10
        // Plus an else (+1 flat) = 11 total
        let source = r#"
            fn complex(x: i32, items: Vec<i32>) -> i32 {
                if x > 0 {
                    for item in items {
                        if item > 10 {
                            match item {
                                1 => return 1,
                                _ => return 2,
                            }
                        } else {
                            return 0;
                        }
                    }
                }
                0
            }
        "#;
        // With threshold=5, this should trigger
        let issues = analyze_source(source, 5);
        assert_eq!(
            issues.len(),
            1,
            "Should report 1 issue for the complex function"
        );
        assert!(issues[0].message.contains("complex"));
        assert!(issues[0].message.contains("cognitive complexity"));
    }

    #[test]
    fn logical_operators_add_complexity() {
        // a && b && c: +1 for first &&, same op continues = 1
        // a && b || c: +1 for &&, +1 for switch to || = 2
        // if: +1
        // total with `if a && b || c && d`: if(+1) + &&(+1) + ||(+1) + &&(+1) = 4
        let source = r#"
            fn logical(a: bool, b: bool, c: bool, d: bool) -> bool {
                if a && b || c && d {
                    true
                } else {
                    false
                }
            }
        "#;
        // Complexity: if(+1) + &&(+1) + ||(+1) + &&(+1) + else(+1) = 5
        let issues = analyze_source(source, 4);
        assert_eq!(
            issues.len(),
            1,
            "Should report 1 issue for logical operator complexity"
        );

        let issues_high = analyze_source(source, 10);
        assert!(
            issues_high.is_empty(),
            "Should not report when threshold is high enough"
        );
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = CognitiveComplexityRule::default();
        assert_eq!(rule.id(), "rust:cognitive-complexity");
        assert_eq!(rule.name(), "Cognitive Complexity");
        assert_eq!(rule.rule_type(), RuleType::CodeSmell);
        assert_eq!(rule.default_severity(), Severity::Major);
    }
}
