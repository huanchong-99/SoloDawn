//! Rule: Function Length
//!
//! Checks that Rust functions and methods do not exceed a configurable maximum line count.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustRule, RustAnalysisContext, RuleConfig};
use syn::visit::Visit;

/// Rule that flags functions/methods exceeding a maximum line count.
pub struct FunctionLengthRule;

impl Default for FunctionLengthRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for FunctionLengthRule {
    fn id(&self) -> &str {
        "rust:function-length"
    }

    fn name(&self) -> &str {
        "Function Length"
    }

    fn description(&self) -> &str {
        "Checks that functions and methods do not exceed a maximum number of lines"
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

impl RustRule for FunctionLengthRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let max_lines = ctx.config.get_param_usize("max_lines", 60);
        let severity = ctx.config.severity_override.unwrap_or_else(|| self.default_severity());

        let mut visitor = FunctionLengthVisitor {
            max_lines,
            severity,
            rule_id: self.id().to_string(),
            file_path: ctx.file_path.to_string(),
            issues: Vec::new(),
        };

        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

struct FunctionLengthVisitor {
    max_lines: usize,
    severity: Severity,
    rule_id: String,
    file_path: String,
    issues: Vec<QualityIssue>,
}

impl FunctionLengthVisitor {
    fn check_function_body(&mut self, name: &str, block: &syn::Block, start_line: usize) {
        let span = block.brace_token.span.open();
        let end_span = block.brace_token.span.close();

        let body_start = span.start().line;
        let body_end = end_span.end().line;
        let line_count = body_end.saturating_sub(body_start);

        if line_count > self.max_lines {
            let issue = QualityIssue::new(
                self.rule_id.clone(),
                RuleType::CodeSmell,
                self.severity,
                crate::rule::AnalyzerSource::Other("built-in".to_string()),
                format!(
                    "Function `{}` has {} lines, which exceeds the maximum of {} lines",
                    name, line_count, self.max_lines
                ),
            )
            .with_location(self.file_path.clone(), start_line as u32);

            self.issues.push(issue);
        }
    }
}

impl<'ast> Visit<'ast> for FunctionLengthVisitor {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        let name = node.sig.ident.to_string();
        let start_line = node.sig.ident.span().start().line;
        self.check_function_body(&name, &node.block, start_line);
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        let name = node.sig.ident.to_string();
        let start_line = node.sig.ident.span().start().line;
        self.check_function_body(&name, &node.block, start_line);
        syn::visit::visit_impl_item_fn(self, node);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_code(source: &str, max_lines: usize) -> Vec<QualityIssue> {
        let syntax = syn::parse_file(source).expect("failed to parse test source");
        let config = RuleConfig {
            enabled: true,
            severity_override: None,
            params: [("max_lines".to_string(), max_lines.to_string())]
                .into_iter()
                .collect(),
        };
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: source,
            syntax: &syntax,
            config: &config,
        };
        let rule = FunctionLengthRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn short_function_no_issue() {
        let source = r#"
fn short() {
    let x = 1;
    let y = 2;
    x + y;
}
"#;
        let issues = analyze_code(source, 60);
        assert!(issues.is_empty(), "short function should not trigger an issue");
    }

    #[test]
    fn long_function_triggers_issue() {
        // Generate a function body with many lines
        let mut body_lines = String::new();
        for i in 0..70 {
            body_lines.push_str(&format!("    let _v{} = {};\n", i, i));
        }
        let source = format!("fn long_function() {{\n{}}}\n", body_lines);

        let issues = analyze_code(&source, 60);
        assert!(!issues.is_empty(), "long function should trigger an issue");
        assert!(
            issues[0].message.contains("long_function"),
            "issue message should contain function name"
        );
    }
}
