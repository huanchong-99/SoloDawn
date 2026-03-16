//! Built-in TypeScript/JavaScript quality rules
//!
//! Each rule operates on source text with regex-based analysis and produces `QualityIssue` results.

pub mod any_usage;
pub mod complexity;
pub mod console_usage;
pub mod file_length;
pub mod function_length;
pub mod import_order;
pub mod naming;
pub mod nesting_depth;
pub mod react_hooks;
pub mod todo_comments;
pub mod type_assertion;

use super::TsRule;

/// Collect all built-in TypeScript/JavaScript rules
pub fn all_ts_rules() -> Vec<Box<dyn TsRule>> {
    vec![
        Box::new(complexity::ComplexityRule::default()),
        Box::new(function_length::FunctionLengthRule::default()),
        Box::new(file_length::FileLengthRule),
        Box::new(nesting_depth::NestingDepthRule::default()),
        Box::new(any_usage::AnyUsageRule),
        Box::new(type_assertion::TypeAssertionRule::default()),
        Box::new(console_usage::ConsoleUsageRule::default()),
        Box::new(naming::NamingConventionRule::default()),
        Box::new(react_hooks::ReactHooksRule),
        Box::new(import_order::ImportOrderRule),
        Box::new(todo_comments::TodoCommentsRule),
    ]
}
