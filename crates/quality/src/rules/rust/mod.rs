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
        Box::new(cyclomatic_complexity::CyclomaticComplexityRule),
        Box::new(cognitive_complexity::CognitiveComplexityRule::default()),
        Box::new(function_length::FunctionLengthRule),
        Box::new(file_length::FileLengthRule),
        Box::new(nesting_depth::NestingDepthRule),
        Box::new(error_handling::ErrorHandlingRule),
        Box::new(unsafe_usage::UnsafeUsageRule),
        Box::new(clone_usage::CloneUsageRule),
        Box::new(naming::NamingConventionRule),
        Box::new(documentation::DocumentationRule),
        Box::new(type_complexity::TypeComplexityRule),
        Box::new(todo_comments::TodoCommentsRule),
        Box::new(magic_numbers::MagicNumbersRule),
    ]
}
