//! Built-in Frontend Provider
//!
//! Runs all built-in TypeScript/JavaScript quality rules without external tools.

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::analysis;
use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::Severity;
use crate::rules::typescript::all_ts_rules;
use crate::rules::{RuleConfig, TsAnalysisContext};

/// Built-in frontend quality provider.
///
/// Analyses TypeScript/JavaScript files using all built-in TS rules
/// without requiring any external tooling (ESLint, tsc, etc.).
pub struct BuiltinFrontendProvider {
    /// Frontend directory relative to the project root.
    pub frontend_dir: String,
}

impl Default for BuiltinFrontendProvider {
    fn default() -> Self {
        Self {
            frontend_dir: "frontend".to_string(),
        }
    }
}

#[async_trait]
impl QualityProvider for BuiltinFrontendProvider {
    fn name(&self) -> &str {
        "builtin-frontend"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::BuiltinFrontendIssues,
            MetricKey::BuiltinFrontendCritical,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let frontend_root = project_root.join(&self.frontend_dir);

        let files = analysis::collect_files(&frontend_root, analysis::is_ts_file);
        debug!(
            "builtin-frontend: found {} TS/JS files under {}",
            files.len(),
            frontend_root.display()
        );

        let rules = all_ts_rules();
        let config = RuleConfig::default();
        let mut all_issues = Vec::new();

        for file_path in &files {
            let content = match std::fs::read_to_string(file_path) {
                Ok(c) => c,
                Err(e) => {
                    warn!("builtin-frontend: failed to read {}: {}", file_path.display(), e);
                    continue;
                }
            };

            let relative = file_path
                .strip_prefix(project_root)
                .unwrap_or(file_path)
                .to_string_lossy();

            let lines: Vec<&str> = content.lines().collect();

            let ctx = TsAnalysisContext {
                file_path: &relative,
                content: &content,
                lines: &lines,
                config: &config,
            };

            for rule in &rules {
                let issues = rule.analyze(&ctx);
                all_issues.extend(issues);
            }
        }

        let total_issues = all_issues.len() as i64;
        let critical_count = all_issues
            .iter()
            .filter(|issue| issue.severity >= Severity::Critical)
            .count() as i64;

        debug!(
            "builtin-frontend: {} issues ({} critical+) in {:.0}ms",
            total_issues,
            critical_count,
            start.elapsed().as_millis()
        );

        let duration_ms = start.elapsed().as_millis() as u64;

        let report = ProviderReport::success("builtin-frontend", duration_ms)
            .with_metric(MetricKey::BuiltinFrontendIssues, MeasureValue::Int(total_issues))
            .with_metric(MetricKey::BuiltinFrontendCritical, MeasureValue::Int(critical_count))
            .with_issues(all_issues);

        Ok(report)
    }
}
