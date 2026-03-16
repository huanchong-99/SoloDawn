//! Built-in quality rules
//!
//! Provides fully self-contained static analysis rules that run without external services.
//! Rules are organized by language: Rust, TypeScript, and language-agnostic common rules.

pub mod common;
pub mod rust;
pub mod typescript;

use std::collections::HashMap;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};

/// Core trait for all built-in quality rules
pub trait Rule: Send + Sync {
    /// Unique rule ID (e.g., "rust:cyclomatic-complexity", "ts:any-usage")
    fn id(&self) -> &str;
    /// Human-readable rule name
    fn name(&self) -> &str;
    /// Rule description
    fn description(&self) -> &str;
    /// Rule type classification
    fn rule_type(&self) -> RuleType;
    /// Default severity when rule triggers
    fn default_severity(&self) -> Severity;
    /// Default configuration
    fn default_config(&self) -> RuleConfig {
        RuleConfig::default()
    }
}

/// Rust-specific rule (operates on syn AST)
pub trait RustRule: Rule {
    /// Analyze a parsed Rust source file
    fn analyze(&self, ctx: &RustAnalysisContext) -> Vec<QualityIssue>;
}

/// TypeScript/JavaScript-specific rule (operates on source text with line-based analysis)
pub trait TsRule: Rule {
    /// Analyze a TypeScript/JavaScript source file
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue>;
}

/// Language-agnostic rule (operates on raw file content)
pub trait CommonRule: Rule {
    /// Analyze any file
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue>;
}

/// Analysis context for Rust files
pub struct RustAnalysisContext<'a> {
    /// Relative file path from project root
    pub file_path: &'a str,
    /// Raw source content
    pub content: &'a str,
    /// Parsed AST
    pub syntax: &'a syn::File,
    /// Per-rule configuration
    pub config: &'a RuleConfig,
}

/// Analysis context for TypeScript/JavaScript files
pub struct TsAnalysisContext<'a> {
    /// Relative file path from project root
    pub file_path: &'a str,
    /// Raw source content
    pub content: &'a str,
    /// Lines of source (pre-split for convenience)
    pub lines: &'a [&'a str],
    /// Per-rule configuration
    pub config: &'a RuleConfig,
}

/// Analysis context for common (language-agnostic) rules
pub struct CommonAnalysisContext<'a> {
    /// Relative file path from project root
    pub file_path: &'a str,
    /// Raw file content as bytes
    pub content: &'a [u8],
    /// Whether the file appears to be valid UTF-8 text
    pub is_text: bool,
    /// Text content (only available if is_text is true)
    pub text: Option<&'a str>,
    /// Per-rule configuration
    pub config: &'a RuleConfig,
}

/// Per-rule configuration (thresholds, enable/disable, severity override)
#[derive(Debug, Clone)]
pub struct RuleConfig {
    /// Whether this rule is enabled
    pub enabled: bool,
    /// Severity override (None = use default)
    pub severity_override: Option<Severity>,
    /// Rule-specific parameters (e.g., "max_lines" => "60")
    pub params: HashMap<String, String>,
}

impl Default for RuleConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            severity_override: None,
            params: HashMap::new(),
        }
    }
}

impl RuleConfig {
    /// Get a parameter as i64, with a default fallback
    pub fn get_param_i64(&self, key: &str, default: i64) -> i64 {
        self.params
            .get(key)
            .and_then(|v| v.parse().ok())
            .unwrap_or(default)
    }

    /// Get a parameter as usize, with a default fallback
    pub fn get_param_usize(&self, key: &str, default: usize) -> usize {
        self.params
            .get(key)
            .and_then(|v| v.parse().ok())
            .unwrap_or(default)
    }

    /// Get a parameter as f64, with a default fallback
    pub fn get_param_f64(&self, key: &str, default: f64) -> f64 {
        self.params
            .get(key)
            .and_then(|v| v.parse().ok())
            .unwrap_or(default)
    }

    /// Get a parameter as bool, with a default fallback
    pub fn get_param_bool(&self, key: &str, default: bool) -> bool {
        self.params
            .get(key)
            .map(|v| v == "true" || v == "1")
            .unwrap_or(default)
    }
}
