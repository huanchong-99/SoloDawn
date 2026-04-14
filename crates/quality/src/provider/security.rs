//! 安全审计 Provider
//!
//! 复用并升级 scripts/audit-security.sh

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};

/// 安全审计 Provider
pub struct SecurityProvider;

#[async_trait]
impl QualityProvider for SecurityProvider {
    fn name(&self) -> &str {
        "security"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![MetricKey::SecurityIssues]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let mut report = ProviderReport::success("security", 0);

        // 尝试运行安全审计脚本
        let script_path = project_root.join("scripts/audit-security.sh");
        if script_path.exists() {
            debug!("Running security audit script...");
            let output = tokio::process::Command::new("bash")
                .args([script_path.to_str().unwrap_or("scripts/audit-security.sh")])
                .current_dir(project_root)
                // R6 port-leak fix: strip SoloDawn dev ports.
                .env_remove("PORT")
                .env_remove("BACKEND_PORT")
                .env_remove("FRONTEND_PORT")
                .output()
                .await;

            match output {
                Ok(out) => {
                    let issues = if out.status.success() { 0 } else { 1 };
                    report.metrics.insert(MetricKey::SecurityIssues, MeasureValue::Int(issues));
                }
                Err(e) => {
                    warn!("Security audit failed: {}", e);
                    report.metrics.insert(MetricKey::SecurityIssues, MeasureValue::Int(-1));
                }
            }
        } else {
            // 无审计脚本时返回 0
            report.metrics.insert(MetricKey::SecurityIssues, MeasureValue::Int(0));
        }

        report.duration_ms = start.elapsed().as_millis() as u64;
        Ok(report)
    }
}
