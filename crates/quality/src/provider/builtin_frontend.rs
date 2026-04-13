//! Built-in frontend provider
//!
//! Runs built-in TypeScript/JavaScript rules against discovered JS/TS target roots.

use async_trait::async_trait;
use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::time::Instant;
use tracing::{debug, warn};

use crate::analysis;
use crate::discovery::RepositoryDiscovery;
use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::Severity;
use crate::rules::typescript::all_ts_rules;
use crate::rules::{RuleConfig, TsAnalysisContext};

/// Built-in frontend quality provider.
#[derive(Default)]
pub struct BuiltinFrontendProvider;

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

    fn applicable_metrics(
        &self,
        discovery: &RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> Vec<MetricKey> {
        if discovery.applicable_js_targets(changed_files).is_empty() {
            Vec::new()
        } else {
            self.supported_metrics()
        }
    }

    async fn analyze(
        &self,
        project_root: &Path,
        discovery: &RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let targets = discovery.applicable_js_targets(changed_files);

        if targets.is_empty() {
            debug!("builtin-frontend: no discovered JS/TS targets, skipping");
            return Ok(ProviderReport::success(
                "builtin-frontend",
                start.elapsed().as_millis() as u64,
            ));
        }

        let files = collect_scan_files(project_root, &targets, changed_files);

        debug!(
            targets = ?targets
                .iter()
                .map(|target| target.display_name(project_root))
                .collect::<Vec<_>>(),
            files = files.len(),
            "builtin-frontend discovered target files"
        );

        let rules = all_ts_rules();
        let config = RuleConfig::default();
        let mut all_issues = Vec::new();

        for file_path in &files {
            let content = match std::fs::read_to_string(file_path) {
                Ok(content) => content,
                Err(error) => {
                    warn!("builtin-frontend: failed to read {}: {}", file_path.display(), error);
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
                if !rule.default_config().enabled {
                    continue;
                }
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
        Ok(ProviderReport::success("builtin-frontend", duration_ms)
            .with_metric(MetricKey::BuiltinFrontendIssues, MeasureValue::Int(total_issues))
            .with_metric(MetricKey::BuiltinFrontendCritical, MeasureValue::Int(critical_count))
            .with_issues(all_issues))
    }
}

fn collect_scan_files(
    project_root: &Path,
    targets: &[&crate::discovery::JsTarget],
    changed_files: Option<&[String]>,
) -> Vec<PathBuf> {
    if let Some(files) = changed_files.filter(|files| !files.is_empty()) {
        let mut selected = Vec::new();
        let mut seen = HashSet::new();

        for relative in files {
            let normalized = relative.replace('\\', "/");
            let path = project_root.join(&normalized);
            if !analysis::is_ts_file(&path) {
                continue;
            }
            if !targets
                .iter()
                .any(|target| target.contains_relative_path(project_root, &normalized))
            {
                continue;
            }
            if seen.insert(path.clone()) {
                selected.push(path);
            }
        }

        return selected;
    }

    let scan_roots = dedupe_scan_roots(targets.iter().map(|target| target.root()));
    let mut files = Vec::new();
    let mut seen = HashSet::new();
    for root in &scan_roots {
        for file in analysis::collect_files(root, analysis::is_ts_file) {
            if seen.insert(file.clone()) {
                files.push(file);
            }
        }
    }
    files
}

fn dedupe_scan_roots<'a>(roots: impl IntoIterator<Item = &'a Path>) -> Vec<&'a Path> {
    let mut roots: Vec<&Path> = roots.into_iter().collect();
    roots.sort_by_key(|path| path.components().count());

    let mut deduped = Vec::new();
    'outer: for root in roots {
        for existing in &deduped {
            if root.starts_with(existing) {
                continue 'outer;
            }
        }
        deduped.push(root);
    }

    deduped
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_project_root() -> std::path::PathBuf {
        let path = std::env::temp_dir().join(format!("builtin-frontend-discovery-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(&path).unwrap();
        path
    }

    fn write_file(path: &Path, content: &str) {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).unwrap();
        }
        std::fs::write(path, content).unwrap();
    }

    fn cleanup(path: &Path) {
        let _ = std::fs::remove_dir_all(path);
    }

    #[test]
    fn applicable_metrics_empty_without_discovered_targets() {
        let root = temp_project_root();
        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let provider = BuiltinFrontendProvider;
        assert!(provider.applicable_metrics(&discovery, None).is_empty());
        cleanup(&root);
    }

    #[test]
    fn applicable_metrics_present_with_workspace_target() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["web"]
}"#,
        );
        write_file(
            &root.join("web/package.json"),
            r#"{
  "name": "web",
  "scripts": { "lint": "eslint ." }
}"#,
        );

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let provider = BuiltinFrontendProvider;
        assert_eq!(provider.applicable_metrics(&discovery, None), provider.supported_metrics());
        cleanup(&root);
    }

    #[test]
    fn dedupe_scan_roots_skips_nested_targets() {
        let root = temp_project_root();
        let web = root.join("web");
        let nested = web.join("packages/ui");
        let other = root.join("shared");

        let deduped = dedupe_scan_roots([web.as_path(), nested.as_path(), other.as_path()]);
        let deduped: Vec<_> = deduped
            .into_iter()
            .map(|path| path.strip_prefix(&root).unwrap().to_string_lossy().replace('\\', "/"))
            .collect();

        assert_eq!(deduped, vec!["web", "shared"]);
        cleanup(&root);
    }

    #[tokio::test]
    async fn analyze_only_scans_changed_ts_files_in_incremental_mode() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["web"]
}"#,
        );
        write_file(&root.join("web/package.json"), r#"{ "name": "web" }"#);
        write_file(&root.join("web/tsconfig.json"), "{}");
        write_file(&root.join("web/src/changed.ts"), "console.log('changed');\n");
        write_file(&root.join("web/src/unchanged.ts"), "console.log('unchanged');\n");

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let provider = BuiltinFrontendProvider;
        let changed = vec!["web/src/changed.ts".to_string()];

        let report = provider.analyze(&root, &discovery, Some(&changed)).await.unwrap();

        assert_eq!(report.metrics.get(&MetricKey::BuiltinFrontendIssues), Some(&MeasureValue::Int(1)));
        assert_eq!(report.issues.len(), 1);
        assert_eq!(report.issues[0].file_path.as_deref(), Some("web/src/changed.ts"));
        cleanup(&root);
    }

    #[tokio::test]
    async fn analyze_falls_back_to_full_scan_when_changed_files_is_empty() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["web"]
}"#,
        );
        write_file(&root.join("web/package.json"), r#"{ "name": "web" }"#);
        write_file(&root.join("web/tsconfig.json"), "{}");
        write_file(&root.join("web/src/a.ts"), "console.log('a');\n");
        write_file(&root.join("web/src/b.ts"), "console.log('b');\n");

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let provider = BuiltinFrontendProvider;
        let changed: Vec<String> = Vec::new();

        let report = provider.analyze(&root, &discovery, Some(&changed)).await.unwrap();

        assert_eq!(report.metrics.get(&MetricKey::BuiltinFrontendIssues), Some(&MeasureValue::Int(2)));
        cleanup(&root);
    }
}
