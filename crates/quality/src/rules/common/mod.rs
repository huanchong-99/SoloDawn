//! Built-in language-agnostic quality rules
//!
//! Each rule operates on raw file content and produces `QualityIssue` results.

pub mod duplication;
pub mod encoding;
pub mod large_file;
pub mod line_length;
pub mod secret_detection;
pub mod trailing_whitespace;

use super::CommonRule;

/// Collect all built-in common rules
pub fn all_common_rules() -> Vec<Box<dyn CommonRule>> {
    vec![
        Box::new(duplication::DuplicationRule::default()),
        Box::new(secret_detection::SecretDetectionRule::default()),
        Box::new(large_file::LargeFileRule::default()),
        Box::new(line_length::LineLengthRule::default()),
        Box::new(trailing_whitespace::TrailingWhitespaceRule::default()),
        Box::new(encoding::EncodingRule::default()),
    ]
}
