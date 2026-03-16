//! Built-in Common Provider
//!
//! Runs all language-agnostic quality rules (duplication, secret detection, etc.)

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::analysis;
use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rules::common::all_common_rules;
use crate::rules::{CommonAnalysisContext, RuleConfig};

/// Built-in common (language-agnostic) quality provider
///
/// Runs all common rules from `crate::rules::common` against every
/// Rust and TypeScript/JavaScript source file in the project.
#[derive(Default)]
pub struct BuiltinCommonProvider;

/// Combined filter: accepts Rust and TS/JS source files.
fn is_rust_or_ts_file(p: &Path) -> bool {
    analysis::is_rust_file(p) || analysis::is_ts_file(p)
}

#[async_trait]
impl QualityProvider for BuiltinCommonProvider {
    fn name(&self) -> &str {
        "builtin-common"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::BuiltinCommonIssues,
            MetricKey::DuplicatedBlocks,
            MetricKey::SecretsDetected,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        debug!("builtin-common: starting analysis");

        let files = analysis::collect_files(project_root, is_rust_or_ts_file);
        debug!("builtin-common: collected {} source files", files.len());

        let rules = all_common_rules();
        let config = RuleConfig::default();
        let mut all_issues = Vec::new();

        for file_path in &files {
            let bytes = match std::fs::read(file_path) {
                Ok(b) => b,
                Err(e) => {
                    warn!("builtin-common: failed to read {}: {}", file_path.display(), e);
                    continue;
                }
            };

            let rel_path = file_path
                .strip_prefix(project_root)
                .unwrap_or(file_path)
                .to_string_lossy();

            let (is_text, text_owned);
            match std::str::from_utf8(&bytes) {
                Ok(s) => {
                    is_text = true;
                    text_owned = Some(s.to_owned());
                }
                Err(_) => {
                    is_text = false;
                    text_owned = None;
                }
            }

            let ctx = CommonAnalysisContext {
                file_path: &rel_path,
                content: &bytes,
                is_text,
                text: text_owned.as_deref(),
                config: &config,
            };

            for rule in &rules {
                let issues = rule.analyze(&ctx);
                all_issues.extend(issues);
            }
        }

        let total_issues = all_issues.len() as i64;
        let duplicated_blocks = all_issues
            .iter()
            .filter(|i| i.rule_id == "common:duplication")
            .count() as i64;
        let secrets_detected = all_issues
            .iter()
            .filter(|i| i.rule_id == "common:secret-detection")
            .count() as i64;

        let duration_ms = start.elapsed().as_millis() as u64;

        debug!(
            "builtin-common: found {} issues ({} duplicated blocks, {} secrets) in {}ms",
            total_issues, duplicated_blocks, secrets_detected, duration_ms
        );

        let report = ProviderReport::success("builtin-common", duration_ms)
            .with_metric(MetricKey::BuiltinCommonIssues, MeasureValue::Int(total_issues))
            .with_metric(MetricKey::DuplicatedBlocks, MeasureValue::Int(duplicated_blocks))
            .with_metric(MetricKey::SecretsDetected, MeasureValue::Int(secrets_detected))
            .with_issues(all_issues);

        Ok(report)
    }
}
