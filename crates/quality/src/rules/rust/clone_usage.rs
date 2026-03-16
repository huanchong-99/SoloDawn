//! Clone usage rule for Rust source files.
//!
//! Detects potentially unnecessary `.clone()` calls that may indicate
//! missed opportunities for borrowing or moving values.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustAnalysisContext, RustRule};
use syn::visit::Visit;

/// Built-in rule that detects potentially unnecessary `.clone()` calls.
#[derive(Debug, Default)]
pub struct CloneUsageRule;

impl Rule for CloneUsageRule {
    fn id(&self) -> &str {
        "rust:clone-usage"
    }

    fn name(&self) -> &str {
        "Clone Usage"
    }

    fn description(&self) -> &str {
        "Detects potentially unnecessary .clone() calls. Excessive cloning may indicate \
         missed opportunities for borrowing or moving values, leading to unnecessary \
         allocations and reduced performance."
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }
}

impl RustRule for CloneUsageRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let mut visitor = CloneCallVisitor {
            clones: Vec::new(),
        };
        syn::visit::visit_file(&mut visitor, ctx.syntax);

        visitor
            .clones
            .into_iter()
            .map(|info| {
                QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    crate::rule::AnalyzerSource::Other("built-in".to_string()),
                    format!(
                        "Potentially unnecessary `.clone()` call at line {}. \
                         Consider borrowing or moving the value instead.",
                        info.line,
                    ),
                )
                .with_location(ctx.file_path, info.line)
                .with_effort(5)
            })
            .collect()
    }
}

/// Information about a single `.clone()` call found during AST traversal.
struct CloneCallInfo {
    line: u32,
}

/// Visitor that collects `.clone()` method calls from the AST.
struct CloneCallVisitor {
    clones: Vec<CloneCallInfo>,
}

impl<'ast> Visit<'ast> for CloneCallVisitor {
    fn visit_expr_method_call(&mut self, node: &'ast syn::ExprMethodCall) {
        if node.method == "clone" && node.args.is_empty() {
            let line = node.method.span().start().line as u32;
            self.clones.push(CloneCallInfo { line });
        }
        // Continue visiting to find nested clone calls
        syn::visit::visit_expr_method_call(self, node);
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
        let rule = CloneUsageRule;
        rule.analyze(&ctx)
    }

    #[test]
    fn no_clone_calls_produces_no_issues() {
        let source = r#"
            fn greet(name: &str) -> String {
                format!("Hello, {}", name)
            }
        "#;
        let issues = analyze_source(source);
        assert!(issues.is_empty(), "Code without .clone() should have no issues");
    }

    #[test]
    fn detects_clone_calls() {
        let source = r#"
            fn example(s: String) -> (String, String) {
                let a = s.clone();
                let b = a.clone();
                (a, b)
            }
        "#;
        let issues = analyze_source(source);
        assert_eq!(issues.len(), 2, "Should detect exactly two .clone() calls");
        assert!(issues[0].message.contains(".clone()"));
        assert_eq!(issues[0].rule_id, "rust:clone-usage");
    }

    #[test]
    fn does_not_flag_clone_with_arguments() {
        let source = r#"
            fn example(v: &[i32]) {
                let _ = v.clone_from_slice(&[1, 2, 3]);
            }
        "#;
        let issues = analyze_source(source);
        assert!(
            issues.is_empty(),
            "Methods named differently than `clone` should not be flagged"
        );
    }

    #[test]
    fn detects_nested_clone_in_closures() {
        let source = r#"
            fn example(items: Vec<String>) -> Vec<String> {
                items.iter().map(|s| s.clone()).collect()
            }
        "#;
        let issues = analyze_source(source);
        assert_eq!(issues.len(), 1, "Should detect .clone() inside closure");
    }
}
