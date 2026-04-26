//! Built-in Common Provider
//!
//! Runs all language-agnostic quality rules (duplication, secret detection, etc.)

use std::{
    collections::BTreeSet,
    path::{Path, PathBuf},
    time::Instant,
};

use async_trait::async_trait;
use tracing::{debug, warn};

use crate::{
    analysis,
    gate::result::MeasureValue,
    metrics::MetricKey,
    provider::{ProviderReport, QualityProvider},
    rules::{CommonAnalysisContext, RuleConfig, common::all_common_rules},
};

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

fn files_for_scope(project_root: &Path, changed_files: Option<&[String]>) -> Vec<PathBuf> {
    match changed_files {
        Some(files) => files
            .iter()
            .filter_map(|file| changed_file_path(project_root, file))
            .filter(|path| path.is_file() && is_rust_or_ts_file(path))
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect(),
        None => analysis::collect_files(project_root, is_rust_or_ts_file),
    }
}

fn changed_file_path(project_root: &Path, file: &str) -> Option<PathBuf> {
    let trimmed = file.trim();
    if trimmed.is_empty() {
        return None;
    }
    let path = PathBuf::from(trimmed);
    if path.is_absolute() {
        Some(path)
    } else {
        Some(project_root.join(trimmed))
    }
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
        _discovery: &crate::discovery::RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        debug!("builtin-common: starting analysis");

        let files = files_for_scope(project_root, changed_files);
        debug!("builtin-common: collected {} source files", files.len());

        let rules = all_common_rules();
        let config = RuleConfig::default();
        let mut all_issues = Vec::new();

        for file_path in &files {
            let bytes = match std::fs::read(file_path) {
                Ok(b) => b,
                Err(e) => {
                    warn!(
                        "builtin-common: failed to read {}: {}",
                        file_path.display(),
                        e
                    );
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
                if !rule.default_config().enabled {
                    continue;
                }
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
            .with_metric(
                MetricKey::BuiltinCommonIssues,
                MeasureValue::Int(total_issues),
            )
            .with_metric(
                MetricKey::DuplicatedBlocks,
                MeasureValue::Int(duplicated_blocks),
            )
            .with_metric(
                MetricKey::SecretsDetected,
                MeasureValue::Int(secrets_detected),
            )
            .with_issues(all_issues);

        Ok(report)
    }
}

#[cfg(test)]
mod tests {
    use std::{fs, path::PathBuf};

    use uuid::Uuid;

    use super::*;
    use crate::discovery::RepositoryDiscovery;

    fn temp_root() -> PathBuf {
        let root = std::env::temp_dir().join(format!("builtin-common-{}", Uuid::new_v4()));
        fs::create_dir_all(root.join("src")).expect("temp src dir");
        root
    }

    #[tokio::test]
    async fn changed_files_scope_does_not_scan_unrelated_secret_files() {
        let root = temp_root();
        fs::write(
            root.join("src").join("config.ts"),
            r#"password = "super_secret_value_here";"#,
        )
        .expect("secret file");
        fs::write(
            root.join("src").join("clean.ts"),
            "export const ok = true;\n",
        )
        .expect("clean file");

        let discovery = RepositoryDiscovery::discover(&root).expect("discovery");
        let provider = BuiltinCommonProvider;
        let changed_files = vec!["src/clean.ts".to_string()];
        let report = provider
            .analyze(&root, &discovery, Some(&changed_files))
            .await
            .expect("provider report");

        assert!(
            report
                .issues
                .iter()
                .all(|issue| issue.rule_id != "common:secret-detection"),
            "Unchanged secret-looking files must stay out of IntroducedOnly scope"
        );
        assert_eq!(
            report.metrics.get(&MetricKey::SecretsDetected),
            Some(&MeasureValue::Int(0))
        );

        let _ = fs::remove_dir_all(&root);
    }

    #[tokio::test]
    async fn changed_files_scope_still_reports_changed_secret_files() {
        let root = temp_root();
        fs::write(
            root.join("src").join("config.ts"),
            r#"password = "super_secret_value_here";"#,
        )
        .expect("secret file");

        let discovery = RepositoryDiscovery::discover(&root).expect("discovery");
        let provider = BuiltinCommonProvider;
        let changed_files = vec!["src/config.ts".to_string()];
        let report = provider
            .analyze(&root, &discovery, Some(&changed_files))
            .await
            .expect("provider report");

        assert!(
            report
                .issues
                .iter()
                .any(|issue| issue.rule_id == "common:secret-detection"),
            "Changed source files must still report production-looking secrets"
        );

        let _ = fs::remove_dir_all(&root);
    }
}
