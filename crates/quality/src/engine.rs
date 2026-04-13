//! 质量门执行引擎
//!
//! 编排 Provider → 收集报告 → 求值 → 决策
//! 这是质量门的顶层入口

use std::collections::{HashMap, HashSet};
use std::path::Path;
use std::sync::Arc;
use tracing::{info, warn};

use crate::config::{QualityGateConfig, QualityGateMode};
use crate::discovery::RepositoryDiscovery;
use crate::gate::evaluator::ConditionEvaluator;
use crate::gate::result::{EvaluationResult, MeasureValue};
use crate::gate::QualityGateLevel;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;
use crate::provider::QualityProvider;
use crate::report::QualityReport;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::sarif;

/// 质量门执行引擎
///
/// 职责：
/// 1. 加载配置
/// 2. 调度启用的 Provider 执行分析
/// 3. 聚合 Provider 报告
/// 4. 对质量门条件求值
/// 5. 生成最终决策
pub struct QualityEngine {
    config: QualityGateConfig,
    providers: Vec<Arc<dyn QualityProvider>>,
}

impl QualityEngine {
    /// 创建引擎实例
    pub fn new(config: QualityGateConfig, providers: Vec<Arc<dyn QualityProvider>>) -> Self {
        Self { config, providers }
    }

    /// 从项目目录自动创建引擎
    pub fn from_project(project_root: &Path) -> anyhow::Result<Self> {
        let config = QualityGateConfig::load_from_project(project_root)?;

        // 根据配置创建启用的 providers
        let mut providers: Vec<Arc<dyn QualityProvider>> = Vec::new();

        if config.providers.rust {
            providers.push(Arc::new(
                crate::provider::rust_analyzer::RustProvider::default(),
            ));
        }
        if config.providers.frontend {
            providers.push(Arc::new(
                crate::provider::frontend::FrontendProvider::default(),
            ));
        }
        if config.providers.repo {
            providers.push(Arc::new(crate::provider::repo::RepoProvider::default()));
        }
        if config.providers.security {
            providers.push(Arc::new(crate::provider::security::SecurityProvider));
        }
        if config.providers.sonar {
            let sonar_token = std::env::var("SONAR_TOKEN")
                .ok()
                .or(config.sonar.token.clone());
            let mut sonar = crate::provider::sonar::SonarProvider::default();
            sonar.host_url = config.sonar.host_url.clone();
            sonar.project_key = config.sonar.project_key.clone();
            sonar.token = sonar_token;
            providers.push(Arc::new(sonar));
        }

        // Built-in providers (no external service dependencies)
        if config.providers.builtin_rust {
            providers.push(Arc::new(
                crate::provider::builtin_rust::BuiltinRustProvider,
            ));
        }
        if config.providers.builtin_frontend {
            providers.push(Arc::new(
                crate::provider::builtin_frontend::BuiltinFrontendProvider::default(),
            ));
        }
        if config.providers.builtin_common {
            providers.push(Arc::new(
                crate::provider::builtin_common::BuiltinCommonProvider,
            ));
        }
        if config.providers.coverage {
            providers.push(Arc::new(
                crate::provider::coverage::CoverageProvider,
            ));
        }

        Ok(Self::new(config, providers))
    }

    /// 执行质量门分析
    ///
    /// # 参数
    /// - `project_root`: 项目根目录
    /// - `level`: 质量门层级（Terminal/Branch/Repo）
    /// - `changed_files`: 变更文件列表（Terminal gate 用于增量分析）
    pub async fn run(
        &self,
        project_root: &Path,
        level: QualityGateLevel,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<QualityReport> {
        // 检查是否启用
        if !self.config.is_enabled() {
            info!("Quality gate is disabled (mode=off), skipping");
            return Ok(QualityReport::aggregate(vec![]));
        }

        info!("Starting quality gate analysis: {} (mode={:?})", level, self.config.mode);

        let discovery = Arc::new(RepositoryDiscovery::discover(project_root)?);
        info!(
            js_targets = discovery.js_targets().len(),
            has_rust_targets = discovery.has_rust_targets(),
            repo_package_manager = ?discovery.repo_package_manager(),
            repo_checks = ?discovery.repo_checks(),
            "Repository discovery completed"
        );

        // 并发运行所有启用的 providers
        let mut handles = Vec::new();
        for provider in &self.providers {
            if !provider.is_enabled() {
                continue;
            }
            let provider = Arc::clone(provider);
            let root = project_root.to_path_buf();
            let files = changed_files.map(|f| f.to_vec());
            let discovery = Arc::clone(&discovery);

            handles.push(tokio::spawn(async move {
                let files_ref = files.as_deref();
                provider.analyze(&root, &discovery, files_ref).await
            }));
        }

        // 收集所有 provider 报告
        let mut reports = Vec::new();
        for handle in handles {
            match handle.await {
                Ok(Ok(report)) => {
                    info!("Provider '{}' completed in {}ms", report.provider_name, report.duration_ms);
                    reports.push(report);
                }
                Ok(Err(e)) => {
                    warn!("Provider analysis failed: {} — metrics from this provider will be missing, which may cause quality gate conditions to WARN", e);
                    // Include a failed provider report so the evaluator sees the gap
                    reports.push(crate::provider::ProviderReport::failure(
                        "unknown-provider",
                        0,
                        format!("Provider failed: {}", e),
                    ));
                }
                Err(e) => {
                    warn!("Provider task panicked: {} — metrics from this provider will be missing, which may cause quality gate conditions to WARN", e);
                    reports.push(crate::provider::ProviderReport::failure(
                        "unknown-provider",
                        0,
                        format!("Provider panicked: {}", e),
                    ));
                }
            }
        }

        // Import any SARIF output files found in the project
        let sarif_issues = Self::collect_sarif_issues(project_root).await;
        if !sarif_issues.is_empty() {
            info!("Imported {} issues from SARIF files", sarif_issues.len());
            let sarif_report = crate::provider::ProviderReport::success("sarif-import", 0)
                .with_issues(sarif_issues);
            reports.push(sarif_report);
        }

        // 聚合报告
        let mut quality_report = QualityReport::aggregate(reports);

        // 获取质量门定义并求值
        let gate = self.config.get_gate(level)?;

        // 收集所有度量值
        let mut all_metrics: HashMap<MetricKey, MeasureValue> = HashMap::new();
        for provider_report in &quality_report.provider_reports {
            all_metrics.extend(provider_report.metrics.clone());
        }

        // G16-005: Only evaluate conditions whose metric is supported by at least
        // one active provider. This prevents cross-stack false positives (e.g.,
        // CargoCheckErrors blocking a pure TypeScript project).
        let supported: HashSet<MetricKey> = self.providers.iter()
            .flat_map(|p| p.applicable_metrics(&discovery, changed_files))
            .collect();

        let applicable_conditions: Vec<_> = gate.conditions.iter()
            .filter(|c| supported.contains(&c.metric))
            .cloned()
            .collect();

        if applicable_conditions.len() < gate.conditions.len() {
            let skipped: Vec<_> = gate.conditions.iter()
                .filter(|c| !supported.contains(&c.metric))
                .map(|c| format!("{:?}", c.metric))
                .collect();
            info!(
                "Skipping {} gate conditions with no active provider: [{}]",
                skipped.len(),
                skipped.join(", ")
            );
        }

        let eval_results = ConditionEvaluator::evaluate_all(&applicable_conditions, &all_metrics);

        // Empty-scan fail-closed (enforce only).
        //
        // Trigger: the repo has discoverable code targets (JS/Rust) **and** the
        // configured gate has rules **but** no provider claims to evaluate any
        // of them. In that scenario the evaluator runs over an empty condition
        // list and returns no error/warn — which would silently produce an OK
        // decision, i.e. "the security guard's checklist was empty so nothing
        // looked suspicious". In `enforce` mode that is unsafe: quality was not
        // actually verified, so we synthesize a single ERROR result on the
        // sentinel metric `QualityGateEmptyScan` and inject a matching blocking
        // QualityIssue so the orchestrator's audit trail reflects the cause.
        let has_targets = discovery.has_js_targets() || discovery.has_rust_targets();
        let gate_has_rules = !gate.conditions.is_empty();
        let no_provider_metric_matched = applicable_conditions.is_empty();
        let trigger_empty_scan_block =
            self.config.is_enforcing() && has_targets && gate_has_rules && no_provider_metric_matched;

        let final_eval_results: Vec<EvaluationResult> = if trigger_empty_scan_block {
            let js_count = discovery.js_targets().len();
            let rust_count = if discovery.has_rust_targets() { 1 } else { 0 };
            let detail = format!(
                "Quality gate is in enforce mode but no enabled provider claims to evaluate any of \
                 the {} configured condition(s) for this repository (discovered: {} JS target(s), \
                 {} Rust workspace(s)). Refusing to pass — quality was not actually verified.",
                gate.conditions.len(),
                js_count,
                rust_count,
            );
            warn!(
                gate = %gate.name,
                js_targets = js_count,
                has_rust_targets = discovery.has_rust_targets(),
                gate_conditions = gate.conditions.len(),
                "Empty-scan fail-closed triggered (enforce mode + discovered targets + zero applicable provider metrics)"
            );
            // PRIMARY-BRAIN-REJECT-V1: append the synthetic blocker AND
            // recompute summary. Before this fix the issue went into
            // `all_issues` but `quality_report.summary` had already been
            // frozen by `aggregate()`, so downstream agent.rs saw
            // total_issues=0/blocking_issues=0 and reclassified the ERROR
            // decision as a "metric-collection failure" → terminal got
            // promoted instead of blocked. Recomputing summary closes that
            // gap so the empty-scan signal reaches the orchestrator intact.
            quality_report.all_issues.push(
                QualityIssue::new(
                    "quality_engine::empty_scan",
                    RuleType::Bug,
                    Severity::Blocker,
                    AnalyzerSource::Other("quality-engine".to_string()),
                    detail.clone(),
                )
                .with_effort(5),
            );
            quality_report.summary =
                crate::issue::IssueSummary::from_issues(&quality_report.all_issues);
            vec![EvaluationResult::error_with_message(
                MetricKey::QualityGateEmptyScan,
                None,
                detail,
            )]
        } else {
            eval_results
        };

        // 生成质量门决策
        let decision = gate.evaluate(&final_eval_results);

        info!("{}", quality_report.status_line());

        quality_report = quality_report.with_decision(decision);

        Ok(quality_report)
    }

    /// 获取当前配置
    pub fn config(&self) -> &QualityGateConfig {
        &self.config
    }

    /// 获取模式
    pub fn mode(&self) -> QualityGateMode {
        self.config.mode
    }

    /// Scan well-known directories for SARIF output files and convert to QualityIssue.
    ///
    /// Looks in:
    /// - `quality/sarif/` (project convention)
    /// - `target/sarif/` (Rust tooling output)
    /// - `.sarif/` (generic)
    async fn collect_sarif_issues(project_root: &Path) -> Vec<QualityIssue> {
        let search_dirs = [
            project_root.join("quality/sarif"),
            project_root.join("target/sarif"),
            project_root.join(".sarif"),
        ];

        let mut all_issues = Vec::new();

        for dir in &search_dirs {
            if !dir.is_dir() {
                continue;
            }

            let entries = match std::fs::read_dir(dir) {
                Ok(e) => e,
                Err(_) => continue,
            };

            for entry in entries.flatten() {
                let path = entry.path();
                let ext = path.extension().and_then(|e| e.to_str()).unwrap_or("");
                if ext != "sarif" && ext != "json" {
                    continue;
                }

                match tokio::fs::read_to_string(&path).await {
                    Ok(content) => match sarif::parse_sarif(&content) {
                        Ok(report) => {
                            let source = report
                                .runs
                                .first()
                                .map(|r| {
                                    let name = r.tool.driver.name.to_lowercase();
                                    if name.contains("clippy") {
                                        AnalyzerSource::Clippy
                                    } else if name.contains("eslint") {
                                        AnalyzerSource::EsLint
                                    } else {
                                        AnalyzerSource::Other(r.tool.driver.name.clone())
                                    }
                                })
                                .unwrap_or(AnalyzerSource::Other("sarif".to_string()));

                            let issues = sarif::sarif_to_issues(&report, source);
                            info!(
                                "Loaded {} issues from SARIF: {}",
                                issues.len(),
                                path.display()
                            );
                            all_issues.extend(issues);
                        }
                        Err(e) => {
                            warn!("Failed to parse SARIF {}: {}", path.display(), e);
                        }
                    },
                    Err(e) => {
                        warn!("Failed to read SARIF {}: {}", path.display(), e);
                    }
                }
            }
        }

        all_issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::QualityGateMode;
    use crate::gate::status::QualityGateStatus;
    use std::path::Path as StdPath;
    use uuid::Uuid;
    use async_trait::async_trait;

    fn temp_root() -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!("quality-engine-{}", Uuid::new_v4()));
        std::fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn write(path: &StdPath, content: &str) {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).unwrap();
        }
        std::fs::write(path, content).unwrap();
    }

    /// Provider that intentionally claims zero applicable metrics — simulates
    /// the real Task 1 scenario where the discovered repo has no provider that
    /// can evaluate any gate condition.
    struct EmptyProvider;
    #[async_trait]
    impl QualityProvider for EmptyProvider {
        fn name(&self) -> &str {
            "empty-provider"
        }
        fn supported_metrics(&self) -> Vec<MetricKey> {
            vec![]
        }
        fn applicable_metrics(
            &self,
            _discovery: &RepositoryDiscovery,
            _changed_files: Option<&[String]>,
        ) -> Vec<MetricKey> {
            vec![]
        }
        async fn analyze(
            &self,
            _project_root: &Path,
            _discovery: &RepositoryDiscovery,
            _changed_files: Option<&[String]>,
        ) -> anyhow::Result<crate::provider::ProviderReport> {
            Ok(crate::provider::ProviderReport::success("empty-provider", 0))
        }
    }

    fn enforce_config_with_terminal_gate() -> QualityGateConfig {
        let mut cfg = QualityGateConfig::default_config();
        cfg.mode = QualityGateMode::Enforce;
        cfg
    }

    fn shadow_config_with_terminal_gate() -> QualityGateConfig {
        let mut cfg = QualityGateConfig::default_config();
        cfg.mode = QualityGateMode::Shadow;
        cfg
    }

    #[tokio::test]
    async fn enforce_empty_scan_with_discovered_target_fails_closed() {
        // Repo has a JS target (frontend package.json) → discovery is non-empty,
        // but the only registered provider claims zero applicable metrics, so
        // the gate's evaluator would otherwise see an empty condition list and
        // return Ok. The empty-scan fail-closed path should override that.
        let root = temp_root();
        write(
            &root.join("package.json"),
            r#"{ "name":"app", "scripts": {"type-check":"tsc --noEmit"} }"#,
        );
        write(&root.join("tsconfig.json"), r#"{ "compilerOptions": {} }"#);

        let providers: Vec<std::sync::Arc<dyn QualityProvider>> =
            vec![std::sync::Arc::new(EmptyProvider)];
        let engine = QualityEngine::new(enforce_config_with_terminal_gate(), providers);

        let report = engine
            .run(&root, QualityGateLevel::Terminal, None)
            .await
            .unwrap();

        assert_eq!(
            report.overall_status(),
            QualityGateStatus::Error,
            "enforce + discovered target + zero provider metrics must NOT pass"
        );
        let blocking = report.blocking_issues();
        assert!(
            blocking.iter().any(|i| i.rule_id == "quality_engine::empty_scan"),
            "expected synthetic empty_scan blocking issue in report.all_issues"
        );
        // Primary-brain rejection v1 follow-up: summary must be recomputed
        // after the synthetic blocker is appended, otherwise agent.rs sees
        // total_issues=0/blocking_issues=0 and reclassifies the failure as
        // a metric-collection skip — promoting the terminal instead of
        // blocking it. Verify the audit-visible counters reflect the block.
        assert!(
            report.summary.blocking_issues >= 1,
            "summary.blocking_issues must be >=1 after empty-scan injection \
             (was {})",
            report.summary.blocking_issues
        );
        assert!(
            report.summary.total >= 1,
            "summary.total must be >=1 after empty-scan injection (was {})",
            report.summary.total
        );

        let _ = std::fs::remove_dir_all(&root);
    }

    #[tokio::test]
    async fn shadow_empty_scan_with_discovered_target_does_not_fail_closed() {
        // Same shape, but mode=shadow → fail-closed must NOT trigger.
        let root = temp_root();
        write(
            &root.join("package.json"),
            r#"{ "name":"app", "scripts": {"type-check":"tsc --noEmit"} }"#,
        );
        write(&root.join("tsconfig.json"), r#"{ "compilerOptions": {} }"#);

        let providers: Vec<std::sync::Arc<dyn QualityProvider>> =
            vec![std::sync::Arc::new(EmptyProvider)];
        let engine = QualityEngine::new(shadow_config_with_terminal_gate(), providers);

        let report = engine
            .run(&root, QualityGateLevel::Terminal, None)
            .await
            .unwrap();

        assert_ne!(
            report.overall_status(),
            QualityGateStatus::Error,
            "shadow mode should not fail-closed on empty scan"
        );

        let _ = std::fs::remove_dir_all(&root);
    }

    #[tokio::test]
    async fn enforce_empty_scan_without_discovered_targets_does_not_block() {
        // No JS, no Rust, no targets discovered → fail-closed must NOT trigger
        // (no code = nothing to verify, vacuously fine).
        let root = temp_root();
        // Intentionally no package.json / Cargo.toml.

        let providers: Vec<std::sync::Arc<dyn QualityProvider>> =
            vec![std::sync::Arc::new(EmptyProvider)];
        let engine = QualityEngine::new(enforce_config_with_terminal_gate(), providers);

        let report = engine
            .run(&root, QualityGateLevel::Terminal, None)
            .await
            .unwrap();

        assert_ne!(
            report.overall_status(),
            QualityGateStatus::Error,
            "no targets at all should not trigger empty-scan fail-closed"
        );

        let _ = std::fs::remove_dir_all(&root);
    }
}
