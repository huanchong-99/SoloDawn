//! Error handling rule — detects unsafe error handling patterns in production Rust code.
//!
//! Flags: `.unwrap()`, `.expect()`, `panic!()`, `unreachable!()`, `unimplemented!()`, `todo!()`
//! in non-test code. Test code (functions annotated with `#[test]` and modules annotated
//! with `#[cfg(test)]`) is excluded from analysis.

use syn::visit::Visit;

use crate::{
    issue::QualityIssue,
    rule::{RuleType, Severity},
    rules::{Rule, RustAnalysisContext, RustRule},
};

/// Rule that detects unsafe error handling patterns in production Rust code.
#[derive(Debug, Default)]
pub struct ErrorHandlingRule;

impl Rule for ErrorHandlingRule {
    fn id(&self) -> &str {
        "rust:error-handling"
    }

    fn name(&self) -> &str {
        "Error Handling"
    }

    fn description(&self) -> &str {
        "Detects unsafe error handling patterns such as .unwrap(), .expect(), panic!(), \
         unreachable!(), unimplemented!(), and todo!() in production code"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::Bug
    }

    fn default_severity(&self) -> Severity {
        Severity::Critical
    }
}

impl RustRule for ErrorHandlingRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let mut visitor = ErrorHandlingVisitor {
            issues: Vec::new(),
            file_path: ctx.file_path.to_string(),
            in_test: false,
            content: ctx.content,
        };
        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

/// Macros considered unsafe for production code.
const UNSAFE_MACROS: &[&str] = &["panic", "unreachable", "unimplemented", "todo"];

/// Method calls considered unsafe for production code.
const UNSAFE_METHODS: &[&str] = &["unwrap", "expect"];

struct ErrorHandlingVisitor<'a> {
    issues: Vec<QualityIssue>,
    file_path: String,
    in_test: bool,
    #[allow(dead_code)]
    content: &'a str,
}

impl<'a> ErrorHandlingVisitor<'a> {
    /// Check whether an attribute list contains `#[test]`.
    fn has_test_attr(attrs: &[syn::Attribute]) -> bool {
        attrs.iter().any(|attr| attr.path().is_ident("test"))
    }

    /// Check whether an attribute list contains `#[cfg(test)]`.
    fn has_cfg_test_attr(attrs: &[syn::Attribute]) -> bool {
        attrs.iter().any(|attr| {
            if !attr.path().is_ident("cfg") {
                return false;
            }
            // Parse the token content inside #[cfg(...)].
            let Ok(nested) = attr.parse_args::<syn::Ident>() else {
                return false;
            };
            nested == "test"
        })
    }

    fn line_number(&self, span: proc_macro2::Span) -> u32 {
        span.start().line as u32
    }

    fn report(&mut self, pattern: &str, span: proc_macro2::Span) {
        let line = self.line_number(span);
        let issue = QualityIssue::new(
            "rust:error-handling",
            RuleType::Bug,
            Severity::Critical,
            crate::rule::AnalyzerSource::Other("built-in".to_string()),
            format!(
                "Unsafe error handling: `{}` used in production code",
                pattern
            ),
        )
        .with_location(&self.file_path, line);
        self.issues.push(issue);
    }
}

impl<'ast> Visit<'ast> for ErrorHandlingVisitor<'_> {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        if self.in_test || Self::has_test_attr(&node.attrs) {
            // Skip test functions entirely.
            return;
        }
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        if self.in_test || Self::has_test_attr(&node.attrs) {
            return;
        }
        syn::visit::visit_impl_item_fn(self, node);
    }

    fn visit_item_mod(&mut self, node: &'ast syn::ItemMod) {
        if Self::has_cfg_test_attr(&node.attrs) {
            // Skip #[cfg(test)] modules entirely.
            return;
        }
        syn::visit::visit_item_mod(self, node);
    }

    fn visit_expr_method_call(&mut self, node: &'ast syn::ExprMethodCall) {
        if !self.in_test {
            let method_name = node.method.to_string();
            for &m in UNSAFE_METHODS {
                if method_name == m {
                    self.report(&format!(".{}()", m), node.method.span());
                    break;
                }
            }
        }
        syn::visit::visit_expr_method_call(self, node);
    }

    fn visit_macro(&mut self, node: &'ast syn::Macro) {
        if !self.in_test {
            if let Some(ident) = node.path.get_ident() {
                let name = ident.to_string();
                for &m in UNSAFE_MACROS {
                    if name == m {
                        self.report(&format!("{}!", m), ident.span());
                        break;
                    }
                }
            }
        }
        syn::visit::visit_macro(self, node);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn analyze_code(code: &str) -> Vec<QualityIssue> {
        let syntax: syn::File = syn::parse_str(code).expect("failed to parse test code");
        let config = RuleConfig::default();
        let ctx = RustAnalysisContext {
            file_path: "test.rs",
            content: code,
            syntax: &syntax,
            config: &config,
        };
        let rule = ErrorHandlingRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn detects_unwrap_and_expect_in_production_code() {
        let code = r#"
            fn process() {
                let x: Option<i32> = Some(1);
                let a = x.unwrap();
                let b = x.expect("missing");
            }
        "#;
        let issues = analyze_code(code);
        assert!(
            issues.len() >= 2,
            "expected at least 2 issues, got {}",
            issues.len()
        );
        assert!(issues.iter().any(|i| i.message.contains(".unwrap()")));
        assert!(issues.iter().any(|i| i.message.contains(".expect()")));
    }

    #[test]
    fn detects_panic_macros_in_production_code() {
        let code = r#"
            fn run() {
                panic!("boom");
                unreachable!();
                unimplemented!();
                todo!();
            }
        "#;
        let issues = analyze_code(code);
        assert!(
            issues.len() >= 4,
            "expected at least 4 issues, got {}",
            issues.len()
        );
        assert!(issues.iter().any(|i| i.message.contains("panic!")));
        assert!(issues.iter().any(|i| i.message.contains("todo!")));
        assert!(issues.iter().any(|i| i.message.contains("unreachable!")));
        assert!(issues.iter().any(|i| i.message.contains("unimplemented!")));
    }

    #[test]
    fn ignores_test_code() {
        let code = r#"
            #[cfg(test)]
            mod tests {
                fn helper() {
                    let x: Option<i32> = Some(1);
                    x.unwrap();
                    panic!("test panic");
                }

                #[test]
                fn my_test() {
                    let v: Option<i32> = Some(42);
                    v.unwrap();
                    todo!();
                }
            }

            #[test]
            fn standalone_test() {
                let v: Option<i32> = Some(1);
                v.expect("fine in test");
            }
        "#;
        let issues = analyze_code(code);
        assert!(
            issues.is_empty(),
            "expected no issues in test code, got {}: {:?}",
            issues.len(),
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
    }
}
