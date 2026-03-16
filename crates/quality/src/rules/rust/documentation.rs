//! Rule: Missing Documentation
//!
//! Checks for missing documentation on public items in Rust code.

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, RustRule, RustAnalysisContext, RuleConfig};
use syn::visit::Visit;

/// Rule that flags public items lacking documentation comments.
pub struct DocumentationRule;

impl Default for DocumentationRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for DocumentationRule {
    fn id(&self) -> &str {
        "rust:documentation"
    }

    fn name(&self) -> &str {
        "Missing Documentation"
    }

    fn description(&self) -> &str {
        "Checks that public items have documentation comments"
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

impl RustRule for DocumentationRule {
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx.config.severity_override.unwrap_or_else(|| self.default_severity());

        let mut visitor = DocumentationVisitor {
            severity,
            rule_id: self.id().to_string(),
            file_path: ctx.file_path.to_string(),
            issues: Vec::new(),
        };

        visitor.visit_file(ctx.syntax);
        visitor.issues
    }
}

struct DocumentationVisitor {
    severity: Severity,
    rule_id: String,
    file_path: String,
    issues: Vec<QualityIssue>,
}

/// Returns true if the attribute list contains at least one `#[doc = "..."]` attribute.
fn has_doc_attr(attrs: &[syn::Attribute]) -> bool {
    attrs.iter().any(|attr| attr.path().is_ident("doc"))
}

/// Returns true if the attribute list contains `#[allow(missing_docs)]`.
fn has_allow_missing_docs(attrs: &[syn::Attribute]) -> bool {
    attrs.iter().any(|attr| {
        if !attr.path().is_ident("allow") {
            return false;
        }
        let mut found = false;
        let _ = attr.parse_nested_meta(|meta| {
            if meta.path.is_ident("missing_docs") {
                found = true;
            }
            Ok(())
        });
        found
    })
}

/// Returns true if the name starts with `_`.
fn is_underscore_prefixed(name: &str) -> bool {
    name.starts_with('_')
}

/// Returns true if the visibility is `pub` (including `pub(crate)` etc).
fn is_pub(vis: &syn::Visibility) -> bool {
    !matches!(vis, syn::Visibility::Inherited)
}

impl DocumentationVisitor {
    fn check_item(&mut self, attrs: &[syn::Attribute], vis: &syn::Visibility, name: &str, line: usize, kind: &str) {
        if !is_pub(vis) {
            return;
        }
        if is_underscore_prefixed(name) {
            return;
        }
        if has_allow_missing_docs(attrs) {
            return;
        }
        if has_doc_attr(attrs) {
            return;
        }

        let issue = QualityIssue::new(
            self.rule_id.clone(),
            RuleType::CodeSmell,
            self.severity,
            crate::rule::AnalyzerSource::Other("built-in".to_string()),
            format!("Public {} `{}` is missing documentation", kind, name),
        )
        .with_location(self.file_path.clone(), line as u32);

        self.issues.push(issue);
    }
}

impl<'ast> Visit<'ast> for DocumentationVisitor {
    fn visit_item_fn(&mut self, node: &'ast syn::ItemFn) {
        let name = node.sig.ident.to_string();
        let line = node.sig.ident.span().start().line;
        self.check_item(&node.attrs, &node.vis, &name, line, "function");
        syn::visit::visit_item_fn(self, node);
    }

    fn visit_item_struct(&mut self, node: &'ast syn::ItemStruct) {
        let name = node.ident.to_string();
        let line = node.ident.span().start().line;
        self.check_item(&node.attrs, &node.vis, &name, line, "struct");
        syn::visit::visit_item_struct(self, node);
    }

    fn visit_item_enum(&mut self, node: &'ast syn::ItemEnum) {
        let name = node.ident.to_string();
        let line = node.ident.span().start().line;
        self.check_item(&node.attrs, &node.vis, &name, line, "enum");
        syn::visit::visit_item_enum(self, node);
    }

    fn visit_item_trait(&mut self, node: &'ast syn::ItemTrait) {
        let name = node.ident.to_string();
        let line = node.ident.span().start().line;
        self.check_item(&node.attrs, &node.vis, &name, line, "trait");
        syn::visit::visit_item_trait(self, node);
    }

    fn visit_item_type(&mut self, node: &'ast syn::ItemType) {
        let name = node.ident.to_string();
        let line = node.ident.span().start().line;
        self.check_item(&node.attrs, &node.vis, &name, line, "type alias");
        syn::visit::visit_item_type(self, node);
    }
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
        let rule = DocumentationRule::default();
        rule.analyze(&ctx)
    }

    #[test]
    fn documented_public_items_produce_no_issues() {
        let source = r#"
/// A well-documented function.
pub fn documented_fn() {}

/// A well-documented struct.
pub struct DocumentedStruct;

/// A well-documented enum.
pub enum DocumentedEnum {}

/// A well-documented trait.
pub trait DocumentedTrait {}

/// A well-documented type alias.
pub type DocumentedType = i32;
"#;
        let issues = analyze_code(source);
        assert!(issues.is_empty(), "documented public items should not trigger issues, got: {:?}", issues.iter().map(|i| &i.message).collect::<Vec<_>>());
    }

    #[test]
    fn undocumented_public_items_produce_issues() {
        let source = r#"
pub fn undocumented_fn() {}

pub struct UndocumentedStruct;

pub enum UndocumentedEnum {}

pub trait UndocumentedTrait {}

pub type UndocumentedType = i32;
"#;
        let issues = analyze_code(source);
        assert_eq!(issues.len(), 5, "expected 5 issues for 5 undocumented public items, got {}: {:?}", issues.len(), issues.iter().map(|i| &i.message).collect::<Vec<_>>());
    }

    #[test]
    fn private_items_are_ignored() {
        let source = r#"
fn private_fn() {}

struct PrivateStruct;

enum PrivateEnum {}
"#;
        let issues = analyze_code(source);
        assert!(issues.is_empty(), "private items should not trigger issues");
    }

    #[test]
    fn underscore_prefixed_items_are_skipped() {
        let source = r#"
pub fn _hidden_fn() {}

pub struct _HiddenStruct;
"#;
        let issues = analyze_code(source);
        assert!(issues.is_empty(), "underscore-prefixed items should be skipped");
    }

    #[test]
    fn allow_missing_docs_suppresses_issue() {
        let source = r#"
#[allow(missing_docs)]
pub fn suppressed_fn() {}
"#;
        let issues = analyze_code(source);
        assert!(issues.is_empty(), "#[allow(missing_docs)] should suppress the issue");
    }
}
