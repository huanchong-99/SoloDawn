//! Built-in Rust quality provider
//!
//! Runs all built-in Rust quality rules without external tools.
//! Parses each `.rs` file with `syn` and applies every rule from `crate::rules::rust`.

use std::path::Path;
use std::time::Instant;

use async_trait::async_trait;
use tracing::{debug, warn};

use crate::analysis;
use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::Severity;
use crate::rules::rust::all_rust_rules;
use crate::rules::{RuleConfig, RustAnalysisContext};

/// Built-in Rust quality provider
///
/// Analyses all `.rs` files using the built-in rule set (cyclomatic complexity,
/// cognitive complexity, naming, documentation, etc.) without shelling out to
/// any external tool.
#[derive(Default)]
pub struct BuiltinRustProvider;

#[async_trait]
impl QualityProvider for BuiltinRustProvider {
    fn name(&self) -> &str {
        "builtin-rust"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::BuiltinRustIssues,
            MetricKey::BuiltinRustCritical,
            MetricKey::RustCyclomaticComplexity,
            MetricKey::RustCognitiveComplexity,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();

        let rust_files = analysis::collect_files(project_root, analysis::is_rust_file);
        debug!(
            "builtin-rust: found {} Rust files to analyze",
            rust_files.len()
        );

        let rules = all_rust_rules();
        let config = RuleConfig::default();
        let mut all_issues = Vec::new();
        let mut max_cyclomatic: i64 = 0;
        let mut max_cognitive: i64 = 0;

        for file_path in &rust_files {
            let relative = file_path
                .strip_prefix(project_root)
                .unwrap_or(file_path)
                .to_string_lossy();

            let content = match std::fs::read_to_string(file_path) {
                Ok(c) => c,
                Err(e) => {
                    warn!("builtin-rust: failed to read {}: {}", relative, e);
                    continue;
                }
            };

            let syntax = match syn::parse_file(&content) {
                Ok(s) => s,
                Err(e) => {
                    warn!("builtin-rust: failed to parse {}: {}", relative, e);
                    continue;
                }
            };

            let ctx = RustAnalysisContext {
                file_path: &relative,
                content: &content,
                syntax: &syntax,
                config: &config,
            };

            for rule in &rules {
                let issues = rule.analyze(&ctx);
                for issue in &issues {
                    if issue.rule_id.contains("cyclomatic") {
                        // Extract complexity value from the issue message if present,
                        // otherwise count each issue as complexity 1.
                        let complexity = extract_number_from_message(&issue.message).unwrap_or(1);
                        if complexity > max_cyclomatic {
                            max_cyclomatic = complexity;
                        }
                    }
                    if issue.rule_id.contains("cognitive") {
                        let complexity = extract_number_from_message(&issue.message).unwrap_or(1);
                        if complexity > max_cognitive {
                            max_cognitive = complexity;
                        }
                    }
                }
                all_issues.extend(issues);
            }
        }

        let total_issues = all_issues.len() as i64;
        let critical_count = all_issues
            .iter()
            .filter(|i| matches!(i.severity, Severity::Critical | Severity::Blocker))
            .count() as i64;

        let duration_ms = start.elapsed().as_millis() as u64;

        debug!(
            "builtin-rust: finished in {}ms — {} issues ({} critical), max cyclomatic={}, max cognitive={}",
            duration_ms, total_issues, critical_count, max_cyclomatic, max_cognitive
        );

        let report = ProviderReport::success("builtin-rust", duration_ms)
            .with_metric(MetricKey::BuiltinRustIssues, MeasureValue::Int(total_issues))
            .with_metric(
                MetricKey::BuiltinRustCritical,
                MeasureValue::Int(critical_count),
            )
            .with_metric(
                MetricKey::RustCyclomaticComplexity,
                MeasureValue::Int(max_cyclomatic),
            )
            .with_metric(
                MetricKey::RustCognitiveComplexity,
                MeasureValue::Int(max_cognitive),
            )
            .with_issues(all_issues);

        Ok(report)
    }
}

/// Try to extract the first integer from a message string.
///
/// Many complexity rules emit messages like "cyclomatic complexity of 15 exceeds threshold".
/// This helper pulls out the first number it finds.
fn extract_number_from_message(message: &str) -> Option<i64> {
    for word in message.split_whitespace() {
        if let Ok(n) = word.parse::<i64>() {
            return Some(n);
        }
    }
    // Also try after common delimiters like "of 15," or "=15"
    for ch in ['=', ':', ','] {
        for segment in message.split(ch) {
            let trimmed = segment.trim();
            if let Ok(n) = trimmed.parse::<i64>() {
                return Some(n);
            }
        }
    }
    None
}
