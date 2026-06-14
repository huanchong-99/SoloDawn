//! SonarQube 本地分析 Provider
//!
//! 集成本地运行的 SonarQube 服务和 sonar-scanner
//! 支持 SARIF 2.1.0 外部问题导入

use std::{path::Path, time::Instant};

use async_trait::async_trait;
use tracing::{debug, info, warn};

use crate::{
    gate::result::MeasureValue,
    metrics::MetricKey,
    provider::{ProviderReport, QualityProvider},
};

/// SonarQube 本地分析 Provider
///
/// 前提条件：
/// - 本地 SonarQube 服务已启动（Docker 或直接安装）
/// - sonar-scanner CLI 可用
/// - 项目根目录有 sonar-project.properties 或 quality/sonar/sonar-project.properties
pub struct SonarProvider {
    /// SonarQube 服务地址（默认 http://localhost:9000）
    pub host_url: String,
    /// 项目 key
    pub project_key: String,
    /// 认证 token
    pub token: Option<String>,
    /// sonar-project.properties 路径（相对于项目根）
    pub properties_path: String,
}

impl Default for SonarProvider {
    fn default() -> Self {
        Self {
            host_url: "http://localhost:9000".to_string(),
            project_key: "solodawn".to_string(),
            token: None,
            properties_path: "quality/sonar/sonar-project.properties".to_string(),
        }
    }
}

impl SonarProvider {
    /// 检查 SonarQube 服务是否可用
    async fn check_sonar_health(&self) -> bool {
        let url = format!("{}/api/system/health", self.host_url);
        match reqwest::get(&url).await {
            Ok(resp) => resp.status().is_success(),
            Err(_) => false,
        }
    }

    /// 等待 SonarQube 任务完成并获取质量门状态
    async fn wait_for_quality_gate(&self) -> anyhow::Result<String> {
        // SonarQube CE 任务完成后查询质量门状态
        let url = format!(
            "{}/api/qualitygates/project_status?projectKey={}",
            self.host_url, self.project_key
        );

        let mut headers = reqwest::header::HeaderMap::new();
        if let Some(ref token) = self.token {
            headers.insert(
                reqwest::header::AUTHORIZATION,
                format!("Bearer {}", token).parse().unwrap(),
            );
        }

        let client = reqwest::Client::new();
        let resp = client.get(&url).headers(headers).send().await?;
        let body = resp.text().await?;

        // 解析质量门状态
        if let Ok(json) = serde_json::from_str::<serde_json::Value>(&body) {
            if let Some(status) = json
                .get("projectStatus")
                .and_then(|ps| ps.get("status"))
                .and_then(|s| s.as_str())
            {
                return Ok(status.to_string());
            }
        }

        Ok("UNKNOWN".to_string())
    }
}

#[async_trait]
impl QualityProvider for SonarProvider {
    fn name(&self) -> &str {
        "sonarqube"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::SonarQualityGateStatus,
            MetricKey::SonarIssues,
            MetricKey::SonarBlockerIssues,
            MetricKey::SonarCriticalIssues,
        ]
    }

    fn is_enabled(&self) -> bool {
        // 只有在 SonarQube 地址配置了且非空时才启用
        !self.host_url.is_empty()
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();

        // 检查 SonarQube 服务健康
        if !self.check_sonar_health().await {
            warn!("SonarQube service is not available at {}", self.host_url);
            return Ok(ProviderReport::failure(
                "sonarqube",
                start.elapsed().as_millis() as u64,
                format!("SonarQube not available at {}", self.host_url),
            ));
        }

        info!("Running SonarQube analysis...");

        // 构建 sonar-scanner 命令参数
        let mut args = vec![
            format!("-Dsonar.host.url={}", self.host_url),
            format!("-Dsonar.projectKey={}", self.project_key),
        ];

        if let Some(ref token) = self.token {
            args.push(format!("-Dsonar.token={}", token));
        }

        // 检查 properties 文件
        let props_path = project_root.join(&self.properties_path);
        if props_path.exists() {
            args.push(format!(
                "-Dproject.settings={}",
                props_path.to_str().unwrap_or(&self.properties_path)
            ));
        }

        // 执行 sonar-scanner
        let scanner_cmd = if cfg!(windows) {
            "sonar-scanner.bat"
        } else {
            "sonar-scanner"
        };
        let string_args: Vec<&str> = args.iter().map(|s| s.as_str()).collect();

        let output = tokio::process::Command::new(scanner_cmd)
            .args(&string_args)
            .current_dir(project_root)
            // R6 port-leak fix: strip SoloDawn dev ports so the scanner
            // doesn't inherit `PORT=23456` from server.exe's polluted env.
            .env_remove("PORT")
            .env_remove("BACKEND_PORT")
            .env_remove("FRONTEND_PORT")
            .output()
            .await;

        let mut report = match output {
            Ok(out) => {
                if out.status.success() {
                    debug!("SonarQube scanner completed successfully");
                    ProviderReport::success("sonarqube", start.elapsed().as_millis() as u64)
                } else {
                    let stderr = String::from_utf8_lossy(&out.stderr).to_string();
                    warn!("SonarQube scanner failed: {}", stderr);
                    ProviderReport::failure("sonarqube", start.elapsed().as_millis() as u64, stderr)
                }
            }
            Err(e) => {
                warn!("Failed to run sonar-scanner: {}", e);
                ProviderReport::failure(
                    "sonarqube",
                    start.elapsed().as_millis() as u64,
                    format!("sonar-scanner not found or failed: {}", e),
                )
            }
        };

        // 查询质量门状态
        let gate_status = self
            .wait_for_quality_gate()
            .await
            .unwrap_or("UNKNOWN".to_string());
        report.metrics.insert(
            MetricKey::SonarQualityGateStatus,
            MeasureValue::String(gate_status),
        );

        report.duration_ms = start.elapsed().as_millis() as u64;
        Ok(report)
    }
}
