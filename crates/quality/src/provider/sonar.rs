//! SonarQube 本地分析 Provider
//!
//! 集成本地运行的 SonarQube 服务和 sonar-scanner
//! 支持 SARIF 2.1.0 外部问题导入

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, info, warn};

use crate::gate::result::MeasureValue;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::AnalyzerSource;
use crate::sarif;

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

    /// Import SARIF 2.1.0 results and convert to QualityIssue format.
    ///
    /// Reads a SARIF file, converts results to unified QualityIssue,
    /// and optionally uploads to SonarCloud via the external issues API.
    pub async fn import_sarif_results(
        &self,
        sarif_path: &Path,
    ) -> anyhow::Result<Vec<QualityIssue>> {
        let content = tokio::fs::read_to_string(sarif_path).await.map_err(|e| {
            anyhow::anyhow!("Failed to read SARIF file {}: {}", sarif_path.display(), e)
        })?;

        let report = sarif::parse_sarif(&content)?;

        // Determine analyzer source from the SARIF tool driver name
        let source = report
            .runs
            .first()
            .map(|run| match run.tool.driver.name.to_lowercase().as_str() {
                s if s.contains("clippy") => AnalyzerSource::Clippy,
                s if s.contains("eslint") => AnalyzerSource::EsLint,
                s if s.contains("sonar") => AnalyzerSource::Sonar,
                other => AnalyzerSource::Other(other.to_string()),
            })
            .unwrap_or(AnalyzerSource::Other("unknown".to_string()));

        let issues = sarif::sarif_to_issues(&report, source);

        info!(
            "Imported {} issues from SARIF file: {}",
            issues.len(),
            sarif_path.display()
        );

        // Optionally upload to SonarCloud if token is configured
        if self.token.is_some() && self.check_sonar_health().await {
            if let Err(e) = self.upload_sarif_to_sonar(sarif_path).await {
                warn!("Failed to upload SARIF to SonarQube: {}", e);
            }
        }

        Ok(issues)
    }

    /// Upload a SARIF file to SonarQube via the external issues import API.
    async fn upload_sarif_to_sonar(&self, sarif_path: &Path) -> anyhow::Result<()> {
        let url = format!("{}/api/issues/import", self.host_url);
        let content = tokio::fs::read(sarif_path).await?;

        let mut headers = reqwest::header::HeaderMap::new();
        if let Some(ref token) = self.token {
            headers.insert(
                reqwest::header::AUTHORIZATION,
                format!("Bearer {}", token).parse().unwrap(),
            );
        }

        let form = reqwest::multipart::Form::new()
            .text("projectKey", self.project_key.clone())
            .part(
                "report",
                reqwest::multipart::Part::bytes(content)
                    .file_name(
                        sarif_path
                            .file_name()
                            .unwrap_or_default()
                            .to_string_lossy()
                            .to_string(),
                    )
                    .mime_str("application/json")?,
            );

        let client = reqwest::Client::new();
        let resp = client
            .post(&url)
            .headers(headers)
            .multipart(form)
            .send()
            .await?;

        if resp.status().is_success() {
            info!("SARIF report uploaded to SonarQube successfully");
        } else {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            warn!("SonarQube SARIF upload returned {}: {}", status, body);
        }

        Ok(())
    }

    /// 等待 SonarQube 任务完成并获取质量门状态
    async fn wait_for_quality_gate(&self, _task_id: &str) -> anyhow::Result<String> {
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
        let scanner_cmd = if cfg!(windows) { "sonar-scanner.bat" } else { "sonar-scanner" };
        let string_args: Vec<&str> = args.iter().map(|s| s.as_str()).collect();

        let output = tokio::process::Command::new(scanner_cmd)
            .args(&string_args)
            .current_dir(project_root)
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
        let gate_status = self.wait_for_quality_gate("").await.unwrap_or("UNKNOWN".to_string());
        report.metrics.insert(
            MetricKey::SonarQualityGateStatus,
            MeasureValue::String(gate_status),
        );

        report.duration_ms = start.elapsed().as_millis() as u64;
        Ok(report)
    }
}
