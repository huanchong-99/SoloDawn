//! Built-in Rust quality rules
//!
//! Each rule operates on `syn::File` AST and produces `QualityIssue` results.

pub mod clone_usage;
pub mod cognitive_complexity;
pub mod cyclomatic_complexity;
pub mod documentation;
pub mod error_handling;
pub mod file_length;
pub mod function_length;
pub mod magic_numbers;
pub mod naming;
pub mod nesting_depth;
pub mod todo_comments;
pub mod type_complexity;
pub mod unsafe_usage;

use super::RustRule;

/// Collect all built-in Rust rules
pub fn all_rust_rules() -> Vec<Box<dyn RustRule>> {
    vec![
        Box::new(cyclomatic_complexity::CyclomaticComplexityRule::default()),
        Box::new(cognitive_complexity::CognitiveComplexityRule::default()),
        Box::new(function_length::FunctionLengthRule::default()),
        Box::new(file_length::FileLengthRule::default()),
        Box::new(nesting_depth::NestingDepthRule::default()),
        Box::new(error_handling::ErrorHandlingRule::default()),
        Box::new(unsafe_usage::UnsafeUsageRule::default()),
        Box::new(clone_usage::CloneUsageRule::default()),
        Box::new(naming::NamingConventionRule::default()),
        Box::new(documentation::DocumentationRule::default()),
        Box::new(type_complexity::TypeComplexityRule::default()),
        Box::new(todo_comments::TodoCommentsRule::default()),
        Box::new(magic_numbers::MagicNumbersRule::default()),
    ]
}
