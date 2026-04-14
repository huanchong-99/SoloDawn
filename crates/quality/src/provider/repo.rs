//! Repo/infra quality provider
//!
//! Runs explicitly declared repository-level checks discovered from the root manifest.

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::discovery::{NodeQualityCommand, PackageManager, RepositoryDiscovery};
use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{run_node_quality_command, ProviderReport, QualityProvider};

/// 仓库级分析器 Provider
pub struct RepoProvider {
    pub enable_types_check: bool,
    pub enable_db_check: bool,
}

impl Default for RepoProvider {
    fn default() -> Self {
        Self {
            enable_types_check: true,
            enable_db_check: true,
        }
    }
}

#[async_trait]
impl QualityProvider for RepoProvider {
    fn name(&self) -> &str {
        "repo"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::GenerateTypesCheckFailures,
            MetricKey::PrepareDbCheckFailures,
        ]
    }

    fn applicable_metrics(
        &self,
        discovery: &RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> Vec<MetricKey> {
        let mut metrics = Vec::new();
        if self.enable_types_check && discovery.repo_checks().generate_types.is_some() {
            metrics.push(MetricKey::GenerateTypesCheckFailures);
        }
        if self.enable_db_check && discovery.repo_checks().prepare_db.is_some() {
            metrics.push(MetricKey::PrepareDbCheckFailures);
        }
        metrics
    }

    async fn analyze(
        &self,
        project_root: &Path,
        discovery: &RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let mut report = ProviderReport::success("repo", 0);

        if self.enable_types_check {
            if let Some(command) = discovery.repo_checks().generate_types.as_ref() {
                debug!(command = %command.describe(), "Running repo generate-types check");
                match run_repo_command(project_root, discovery.repo_package_manager(), command).await {
                    Ok(out) => {
                        let failures = if out.status.success() { 0 } else { 1 };
                        report.metrics.insert(
                            MetricKey::GenerateTypesCheckFailures,
                            MeasureValue::Int(failures),
                        );
                    }
                    Err(error) => {
                        warn!("generate-types check failed to execute: {}", error);
                        report.metrics.insert(
                            MetricKey::GenerateTypesCheckFailures,
                            MeasureValue::Int(-1),
                        );
                    }
                }
            }
        }

        if self.enable_db_check {
            if let Some(command) = discovery.repo_checks().prepare_db.as_ref() {
                debug!(command = %command.describe(), "Running repo prepare-db check");
                match run_repo_command(project_root, discovery.repo_package_manager(), command).await {
                    Ok(out) => {
                        let failures = if out.status.success() { 0 } else { 1 };
                        report.metrics.insert(
                            MetricKey::PrepareDbCheckFailures,
                            MeasureValue::Int(failures),
                        );
                    }
                    Err(error) => {
                        warn!("prepare-db check failed to execute: {}", error);
                        report.metrics.insert(
                            MetricKey::PrepareDbCheckFailures,
                            MeasureValue::Int(-1),
                        );
                    }
                }
            }
        }

        report.duration_ms = start.elapsed().as_millis() as u64;
        Ok(report)
    }
}

async fn run_repo_command(
    project_root: &Path,
    package_manager: Option<PackageManager>,
    command: &NodeQualityCommand,
) -> anyhow::Result<std::process::Output> {
    run_node_quality_command(project_root, package_manager, command).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::discovery::resolve_node_command;

    fn temp_project_root() -> std::path::PathBuf {
        let path = std::env::temp_dir().join(format!("repo-provider-discovery-{}", uuid::Uuid::new_v4()));
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
    fn applicable_metrics_empty_without_declared_repo_checks() {
        let root = temp_project_root();
        write_file(&root.join("package.json"), r#"{ "name": "repo" }"#);
        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let provider = RepoProvider::default();
        assert!(provider.applicable_metrics(&discovery, None).is_empty());
        cleanup(&root);
    }

    #[test]
    fn applicable_metrics_present_for_declared_repo_checks() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "packageManager": "pnpm@10.0.0",
  "scripts": {
    "generate-types:check": "pnpm run gen",
    "prepare-db:check": "pnpm run db"
  }
}"#,
        );
        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let provider = RepoProvider::default();
        assert_eq!(
            provider.applicable_metrics(&discovery, None),
            vec![
                MetricKey::GenerateTypesCheckFailures,
                MetricKey::PrepareDbCheckFailures,
            ]
        );
        cleanup(&root);
    }

    #[test]
    fn resolve_command_uses_repo_package_manager() {
        let (cmd, args) = resolve_node_command(
            Some(crate::discovery::PackageManager::Pnpm),
            &NodeQualityCommand::Script {
                script: "generate-types:check".to_string(),
            },
        );
        // R5 Fix 6: resolve_node_command now Windows-resolves the PM shim
        // (e.g. `pnpm.cmd`). Assert on basename to stay portable.
        let basename = std::path::Path::new(&cmd)
            .file_name()
            .and_then(|s| s.to_str())
            .unwrap_or(&cmd)
            .trim_end_matches(".cmd")
            .trim_end_matches(".exe")
            .trim_end_matches(".CMD")
            .trim_end_matches(".EXE")
            .to_string();
        assert_eq!(basename, "pnpm");
        assert_eq!(args, vec!["run", "generate-types:check"]);
    }
}
