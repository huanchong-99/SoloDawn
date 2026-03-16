//! Magic numbers rule — detects numeric literals that should be named constants.
//!
//! Flags integer and float literals that are not common exempt values (0, 1, 2, -1, 0.0, 1.0)
//! and are not inside `const`/`static` declarations or array indexing expressions.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustRule, RustAnalysisContext};
use syn::visit::Visit;

/// Rule that detects magic numbers (numeric literals) in Rust code
/// that should be extracted into named constants.
#[derive(Debug, Default)]
pub struct MagicNumbersRule;

impl Rule for MagicNumbersRule {
    fn id(&self) -> &str {
        "rust:magic-numbers"
    }

    fn name(&self) -> &str {
        "Magic Numbers"
    }

    fn description(&self) -> &str {
        "Detects numeric literals (magic numbers) that should be named constants for clarity"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }
}

impl RustRule for MagicNumbersRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let mut visitor = MagicNumberVisitor {
            issues: Vec::new(),
            file_path: ctx.file_path.to_string(),
            in_const_or_static: false,
            in_index: false,
        };
        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

/// Exempt integer values that are too common to flag.
const EXEMPT_INTEGERS: &[i128] = &[-1, 0, 1, 2];

/// Exempt float values that are too common to flag.
const EXEMPT_FLOATS: &[f64] = &[0.0, 1.0];

struct MagicNumberVisitor {
    issues: Vec<QualityIssue>,
    file_path: String,
    in_const_or_static: bool,
    in_index: bool,
}

impl MagicNumberVisitor {
    fn line_number(&self, span: proc_macro2::Span) -> u32 {
        span.start().line as u32
    }

    fn check_lit_int(&mut self, lit: &syn::LitInt) {
        if self.in_const_or_static || self.in_index {
            return;
        }
        if let Ok(value) = lit.base10_parse::<i128>() {
            if EXEMPT_INTEGERS.contains(&value) {
                return;
            }
        }
        let line = self.line_number(lit.span());
        let issue = QualityIssue::new(
            "rust:magic-numbers",
            RuleType::CodeSmell,
            Severity::Minor,
            crate::rule::AnalyzerSource::Other("built-in".to_string()),
            format!(
                "Magic number `{}` should be extracted into a named constant",
                lit.token()
            ),
        )
        .with_location(&self.file_path, line)
        .with_effort(5);
        self.issues.push(issue);
    }

    fn check_lit_float(&mut self, lit: &syn::LitFloat) {
        if self.in_const_or_static || self.in_index {
            return;
        }
        if let Ok(value) = lit.base10_parse::<f64>() {
            for &exempt in EXEMPT_FLOATS {
                if (value - exempt).abs() < f64::EPSILON {
                    return;
                }
            }
        }
        let line = self.line_number(lit.span());
        let issue = QualityIssue::new(
            "rust:magic-numbers",
            RuleType::CodeSmell,
            Severity::Minor,
            crate::rule::AnalyzerSource::Other("built-in".to_string()),
            format!(
                "Magic number `{}` should be extracted into a named constant",
                lit.token()
            ),
        )
        .with_location(&self.file_path, line)
        .with_effort(5);
        self.issues.push(issue);
    }
}

impl<'ast> Visit<'ast> for MagicNumberVisitor {
    fn visit_item_const(&mut self, node: &'ast syn::ItemConst) {
        let prev = self.in_const_or_static;
        self.in_const_or_static = true;
        syn::visit::visit_item_const(self, node);
        self.in_const_or_static = prev;
    }

    fn visit_item_static(&mut self, node: &'ast syn::ItemStatic) {
        let prev = self.in_const_or_static;
        self.in_const_or_static = true;
        syn::visit::visit_item_static(self, node);
        self.in_const_or_static = prev;
    }

    fn visit_impl_item_const(&mut self, node: &'ast syn::ImplItemConst) {
        let prev = self.in_const_or_static;
        self.in_const_or_static = true;
        syn::visit::visit_impl_item_const(self, node);
        self.in_const_or_static = prev;
    }

    fn visit_expr_index(&mut self, node: &'ast syn::ExprIndex) {
        // Visit the expression being indexed normally
        self.visit_expr(&node.expr);
        // Visit the index expression with exemption
        let prev = self.in_index;
        self.in_index = true;
        self.visit_expr(&node.index);
        self.in_index = prev;
    }

    fn visit_expr_lit(&mut self, node: &'ast syn::ExprLit) {
        match &node.lit {
            syn::Lit::Int(lit_int) => self.check_lit_int(lit_int),
            syn::Lit::Float(lit_float) => self.check_lit_float(lit_float),
            _ => {}
        }
        syn::visit::visit_expr_lit(self, node);
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
        let rule = MagicNumbersRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn detects_magic_numbers_in_function_body() {
        let code = r#"
            fn calculate() -> i32 {
                let timeout = 3600;
                let factor = 3.14;
                timeout * 2
            }
        "#;
        let issues = analyze_code(code);
        // Should flag 3600 and 3.14, but NOT 2 (exempt)
        assert_eq!(
            issues.len(),
            2,
            "expected 2 magic number issues, got {}: {:?}",
            issues.len(),
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
        assert!(issues.iter().any(|i| i.message.contains("3600")));
        assert!(issues.iter().any(|i| i.message.contains("3.14")));
    }

    #[test]
    fn exempts_common_values_and_const_declarations() {
        let code = r#"
            const MAX_RETRIES: i32 = 5;
            static TIMEOUT: u64 = 30;

            fn init() {
                let a = 0;
                let b = 1;
                let c = 2;
                let d = 0.0;
                let e = 1.0;
                let arr = [10, 20, 30];
                let x = arr[0];
                let y = arr[1];
            }
        "#;
        let issues = analyze_code(code);
        // const 5 and static 30 are exempt (inside const/static).
        // 0, 1, 2, 0.0, 1.0 are exempt common values.
        // arr[0] and arr[1] — index literals are exempt.
        // 10, 20, 30 in the array literal ARE magic numbers.
        assert_eq!(
            issues.len(),
            3,
            "expected 3 magic number issues for array literal values, got {}: {:?}",
            issues.len(),
            issues.iter().map(|i| &i.message).collect::<Vec<_>>()
        );
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = MagicNumbersRule::default();
        assert_eq!(rule.id(), "rust:magic-numbers");
        assert_eq!(rule.name(), "Magic Numbers");
        assert_eq!(rule.rule_type(), RuleType::CodeSmell);
        assert_eq!(rule.default_severity(), Severity::Minor);
    }
}
