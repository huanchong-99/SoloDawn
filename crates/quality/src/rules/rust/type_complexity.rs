//! Type complexity rule for Rust source files.
//!
//! Detects overly complex type expressions by measuring the nesting depth of generic
//! type parameters (e.g., `Arc<Mutex<HashMap<String, Vec<Box<dyn Trait>>>>>`).

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustAnalysisContext, RustRule};
use syn::visit::Visit;

/// Built-in rule that checks for overly complex type expressions in Rust code.
#[derive(Debug, Default)]
pub struct TypeComplexityRule;

impl Rule for TypeComplexityRule {
    fn id(&self) -> &str {
        "rust:type-complexity"
    }

    fn name(&self) -> &str {
        "Type Complexity"
    }

    fn description(&self) -> &str {
        "Checks that type expressions do not exceed a generic nesting depth threshold. \
         Deeply nested generics reduce readability and suggest the need for type aliases."
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }
}

impl RustRule for TypeComplexityRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let threshold = ctx.config.get_param_usize("threshold", 5);
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        let mut collector = TypeCollector {
            findings: Vec::new(),
        };
        syn::visit::visit_file(&mut collector, ctx.syntax);

        let mut issues = Vec::new();
        for finding in &collector.findings {
            if finding.depth > threshold {
                let issue = QualityIssue::new(
                    self.id(),
                    self.rule_type(),
                    severity,
                    crate::rule::AnalyzerSource::Other("built-in".to_string()),
                    format!(
                        "Type expression has a generic nesting depth of {} (threshold: {}). \
                         Consider introducing a type alias to reduce complexity.",
                        finding.depth, threshold,
                    ),
                )
                .with_location(ctx.file_path, finding.line)
                .with_effort(10);
                issues.push(issue);
            }
        }

        issues
    }
}

/// A single type annotation finding with its nesting depth.
struct TypeFinding {
    line: u32,
    depth: usize,
}

/// Measures the maximum generic nesting depth in a `syn::Type`.
fn measure_type_depth(ty: &syn::Type) -> usize {
    match ty {
        syn::Type::Path(type_path) => {
            let mut max_depth = 0;
            for segment in &type_path.path.segments {
                if let syn::PathArguments::AngleBracketed(ref args) = segment.arguments {
                    // This `<...>` contributes 1 to nesting depth
                    for arg in &args.args {
                        if let syn::GenericArgument::Type(inner_ty) = arg {
                            let inner_depth = measure_type_depth(inner_ty);
                            let total = 1 + inner_depth;
                            if total > max_depth {
                                max_depth = total;
                            }
                        }
                    }
                    // If there are angle brackets but no type args, still count 1
                    if max_depth == 0 {
                        max_depth = 1;
                    }
                }
            }
            max_depth
        }
        syn::Type::Reference(type_ref) => measure_type_depth(&type_ref.elem),
        syn::Type::Slice(type_slice) => measure_type_depth(&type_slice.elem),
        syn::Type::Array(type_array) => measure_type_depth(&type_array.elem),
        syn::Type::Paren(type_paren) => measure_type_depth(&type_paren.elem),
        syn::Type::Tuple(type_tuple) => {
            type_tuple
                .elems
                .iter()
                .map(measure_type_depth)
                .max()
                .unwrap_or(0)
        }
        syn::Type::TraitObject(type_trait) => {
            let mut max_depth = 0;
            for bound in &type_trait.bounds {
                if let syn::TypeParamBound::Trait(trait_bound) = bound {
                    for segment in &trait_bound.path.segments {
                        if let syn::PathArguments::AngleBracketed(ref args) = segment.arguments {
                            for arg in &args.args {
                                if let syn::GenericArgument::Type(inner_ty) = arg {
                                    let total = 1 + measure_type_depth(inner_ty);
                                    if total > max_depth {
                                        max_depth = total;
                                    }
                                }
                            }
                            if max_depth == 0 {
                                max_depth = 1;
                            }
                        }
                    }
                }
            }
            max_depth
        }
        _ => 0,
    }
}

/// Collects type annotations from function signatures, struct fields, and let bindings.
struct TypeCollector {
    findings: Vec<TypeFinding>,
}

impl TypeCollector {
    fn record_type(&mut self, ty: &syn::Type, line: u32) {
        let depth = measure_type_depth(ty);
        if depth > 0 {
            self.findings.push(TypeFinding { line, depth });
        }
    }
}

impl<'ast> Visit<'ast> for TypeCollector {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        // Check return type
        if let syn::ReturnType::Type(_, ref ty) = node.sig.output {
            let line = node.sig.ident.span().start().line as u32;
            self.record_type(ty, line);
        }
        // Check parameter types
        for arg in &node.sig.inputs {
            if let syn::FnArg::Typed(pat_type) = arg {
                let line = pat_type.colon_token.span.start().line as u32;
                self.record_type(&pat_type.ty, line);
            }
        }
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_impl_item_fn(&mut self, node: &'ast syn::ImplItemFn) {
        if let syn::ReturnType::Type(_, ref ty) = node.sig.output {
            let line = node.sig.ident.span().start().line as u32;
            self.record_type(ty, line);
        }
        for arg in &node.sig.inputs {
            if let syn::FnArg::Typed(pat_type) = arg {
                let line = pat_type.colon_token.span.start().line as u32;
                self.record_type(&pat_type.ty, line);
            }
        }
        syn::visit::visit_impl_item_fn(self, node);
    }

    fn visit_field(&mut self, node: &'ast syn::Field) {
        let line = node
            .ident
            .as_ref()
            .map(|i| i.span().start().line as u32)
            .unwrap_or(1);
        self.record_type(&node.ty, line);
        syn::visit::visit_field(self, node);
    }

    fn visit_local(&mut self, node: &'ast syn::Local) {
        if let Some(ref init) = node.init {
            // Check for type ascription via let pat: Type = expr;
            // The type annotation is on the pattern, not the init
            let _ = init; // init is used for visiting below
        }
        // Visit the pattern for typed bindings (let x: Type = ...)
        if let syn::Pat::Type(pat_type) = &node.pat {
            let line = pat_type.colon_token.span.start().line as u32;
            self.record_type(&pat_type.ty, line);
        }
        syn::visit::visit_local(self, node);
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
        let rule = TypeComplexityRule;
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
        let rule = TypeComplexityRule;
        rule.analyze(&ctx)
    }

    #[test]
    fn simple_types_no_issue() {
        let source = r#"
            fn simple(x: Vec<String>) -> Option<i32> {
                None
            }

            struct Foo {
                bar: HashMap<String, Vec<i32>>,
            }
        "#;
        // Vec<String> => depth 1, Option<i32> => depth 1, HashMap<String, Vec<i32>> => depth 2
        // All well below default threshold of 5
        let issues = analyze_source(source);
        assert!(
            issues.is_empty(),
            "Simple generic types should not trigger the rule"
        );
    }

    #[test]
    fn deeply_nested_type_triggers_issue() {
        let source = r#"
            use std::sync::{Arc, Mutex};
            use std::collections::HashMap;

            fn complex() -> Arc<Mutex<HashMap<String, Vec<Box<Option<i32>>>>>> {
                todo!()
            }
        "#;
        // Arc<Mutex<HashMap<String, Vec<Box<Option<i32>>>>>> => depth 6
        // Exceeds default threshold of 5
        let issues = analyze_source(source);
        assert_eq!(
            issues.len(),
            1,
            "Deeply nested generic type should trigger one issue"
        );
        assert!(issues[0].message.contains("nesting depth"));
    }

    #[test]
    fn threshold_boundary() {
        let source = r#"
            fn at_threshold() -> Vec<Vec<Vec<i32>>> {
                todo!()
            }
        "#;
        // Vec<Vec<Vec<i32>>> => depth 3

        // Threshold 3: depth 3 is not > 3, so no issue
        let issues = analyze_source_with_threshold(source, 3);
        assert!(
            issues.is_empty(),
            "Type at exactly the threshold should not trigger"
        );

        // Threshold 2: depth 3 > 2, should trigger
        let issues = analyze_source_with_threshold(source, 2);
        assert_eq!(
            issues.len(),
            1,
            "Type exceeding threshold should trigger an issue"
        );
    }

    #[test]
    fn struct_field_with_complex_type() {
        let source = r#"
            struct Config {
                handlers: Arc<Mutex<Vec<Box<dyn Fn(HashMap<String, Vec<u8>>)>>>>,
            }
        "#;
        // Arc<Mutex<Vec<Box<dyn Fn(...)>>>> => depth at least 4 from Arc>Mutex>Vec>Box
        // The dyn Fn(...) does not add angle-bracket depth in the same chain
        // With threshold 2 this should trigger
        let issues = analyze_source_with_threshold(source, 2);
        assert!(
            !issues.is_empty(),
            "Complex struct field type should trigger with low threshold"
        );
    }

    #[test]
    fn let_binding_with_complex_type() {
        let source = r#"
            fn example() {
                let x: Vec<Vec<Vec<Vec<Vec<Vec<i32>>>>>> = vec![];
            }
        "#;
        // Vec^6 => depth 6, exceeds default threshold 5
        let issues = analyze_source(source);
        assert_eq!(
            issues.len(),
            1,
            "Let binding with deeply nested type should trigger"
        );
    }
}
