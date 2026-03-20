//! Rust 分析器 Provider
//!
//! 封装 cargo check / clippy / fmt / test 命令

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::gate::result::MeasureValue;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::{AnalyzerSource, RuleType, Severity};

/// Rust 分析器 Provider
///
/// 封装以下 Rust 工具链命令：
/// - `cargo check --workspace` — 编译检查
/// - `cargo clippy --workspace --all-targets --all-features` — Lint 检查
/// - `cargo fmt --check` — 格式化检查
/// - 受影响范围的 `cargo test` — 测试运行
pub struct RustProvider {
    /// 是否启用 cargo check
    pub enable_check: bool,
    /// 是否启用 clippy
    pub enable_clippy: bool,
    /// 是否启用 fmt check
    pub enable_fmt: bool,
    /// 是否启用 test
    pub enable_test: bool,
}

impl Default for RustProvider {
    fn default() -> Self {
        Self {
            enable_check: true,
            enable_clippy: true,
            enable_fmt: true,
            enable_test: true,
        }
    }
}

impl RustProvider {
    /// 解析 clippy 输出，提取警告和错误
    fn parse_clippy_output(output: &str) -> (Vec<QualityIssue>, i64, i64) {
        let mut issues = Vec::new();
        let mut warnings = 0i64;
        let mut errors = 0i64;

        for line in output.lines() {
            // clippy JSON 输出格式解析
            if let Some(msg) = Self::parse_compiler_message(line) {
                match msg.severity.as_str() {
                    "warning" => {
                        warnings += 1;
                        issues.push(
                            QualityIssue::new(
                                &msg.rule_id,
                                RuleType::CodeSmell,
                                Severity::Major,
                                AnalyzerSource::Clippy,
                                &msg.message,
                            )
                            .with_location(&msg.file, msg.line),
                        );
                    }
                    "error" => {
                        errors += 1;
                        issues.push(
                            QualityIssue::new(
                                &msg.rule_id,
                                RuleType::Bug,
                                Severity::Critical,
                                AnalyzerSource::Clippy,
                                &msg.message,
                            )
                            .with_location(&msg.file, msg.line),
                        );
                    }
                    _ => {}
                }
            }
        }

        (issues, warnings, errors)
    }

    /// 解析 cargo fmt --check 输出
    fn parse_fmt_output(output: &str) -> (Vec<QualityIssue>, i64) {
        let mut issues = Vec::new();
        let mut violations = 0i64;

        // cargo fmt --check 输出 diff 格式，每个有差异的文件都会列出
        for line in output.lines() {
            let trimmed = line.trim();
            if trimmed.starts_with("Diff in") || trimmed.ends_with(".rs") {
                violations += 1;
                issues.push(QualityIssue::new(
                    "fmt::unformatted",
                    RuleType::CodeSmell,
                    Severity::Minor,
                    AnalyzerSource::CargoFmt,
                    format!("File not formatted: {}", trimmed),
                ));
            }
        }

        (issues, violations)
    }

    /// 解析 cargo test 输出
    fn parse_test_output(output: &str) -> (Vec<QualityIssue>, i64) {
        let mut issues = Vec::new();
        let mut failures = 0i64;

        for line in output.lines() {
            // 解析 "test xxx ... FAILED" 格式
            if line.contains("FAILED") && line.starts_with("test ") {
                failures += 1;
                let test_name = line
                    .strip_prefix("test ")
                    .and_then(|s| s.split(" ...").next())
                    .unwrap_or("unknown");

                issues.push(QualityIssue::new(
                    format!("test::{}", test_name),
                    RuleType::Bug,
                    Severity::Critical,
                    AnalyzerSource::CargoTest,
                    format!("Test failed: {}", test_name),
                ));
            }
        }

        (issues, failures)
    }

    /// 解析编译器消息格式
    fn parse_compiler_message(line: &str) -> Option<CompilerMessage> {
        // 尝试解析 rustc/clippy 的 JSON 消息格式
        if let Ok(json) = serde_json::from_str::<serde_json::Value>(line) {
            if let Some(reason) = json.get("reason").and_then(|r| r.as_str()) {
                if reason == "compiler-message" {
                    if let Some(msg) = json.get("message") {
                        let level = msg.get("level").and_then(|l| l.as_str()).unwrap_or("warning");
                        let message = msg.get("message").and_then(|m| m.as_str()).unwrap_or("");
                        let code = msg
                            .get("code")
                            .and_then(|c| c.get("code"))
                            .and_then(|c| c.as_str())
                            .unwrap_or("unknown");

                        // 提取第一个 span 的文件和行号
                        let (file, line_num) = msg
                            .get("spans")
                            .and_then(|s| s.as_array())
                            .and_then(|spans| spans.first())
                            .map(|span| {
                                let file = span
                                    .get("file_name")
                                    .and_then(|f| f.as_str())
                                    .unwrap_or("unknown");
                                let line = span
                                    .get("line_start")
                                    .and_then(|l| l.as_u64())
                                    .unwrap_or(0) as u32;
                                (file.to_string(), line)
                            })
                            .unwrap_or(("unknown".to_string(), 0));

                        return Some(CompilerMessage {
                            severity: level.to_string(),
                            message: message.to_string(),
                            rule_id: format!("clippy::{}", code),
                            file,
                            line: line_num,
                        });
                    }
                }
            }
        }
        None
    }
}

/// 内部编译器消息结构
struct CompilerMessage {
    severity: String,
    message: String,
    rule_id: String,
    file: String,
    line: u32,
}

#[async_trait]
impl QualityProvider for RustProvider {
    fn name(&self) -> &str {
        "rust"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::CargoCheckErrors,
            MetricKey::ClippyWarnings,
            MetricKey::ClippyErrors,
            MetricKey::FmtViolations,
            MetricKey::RustTestFailures,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let mut report = ProviderReport::success("rust", 0);
        let mut all_issues = Vec::new();

        // 1. cargo check
        if self.enable_check {
            debug!("Running cargo check...");
            let output = run_command(project_root, "cargo", &["check", "--workspace", "--message-format=json"]).await;
            match output {
                Ok(out) => {
                    let errors = out.stderr.lines().filter(|l| l.contains("error")).count() as i64;
                    report.metrics.insert(MetricKey::CargoCheckErrors, MeasureValue::Int(errors));
                    if errors > 0 {
                        report.success = false;
                    }
                }
                Err(e) => {
                    warn!("cargo check failed: {}", e);
                    report.success = false;
                    report.error = Some(format!("cargo check failed: {}", e));
                }
            }
        }

        // 2. cargo clippy
        if self.enable_clippy {
            debug!("Running cargo clippy...");
            let output = run_command(
                project_root,
                "cargo",
                &["clippy", "--workspace", "--all-targets", "--all-features", "--message-format=json"],
            )
            .await;
            match output {
                Ok(out) => {
                    let (issues, warnings, errors) = Self::parse_clippy_output(&out.stdout);
                    report.metrics.insert(MetricKey::ClippyWarnings, MeasureValue::Int(warnings));
                    report.metrics.insert(MetricKey::ClippyErrors, MeasureValue::Int(errors));
                    all_issues.extend(issues);
                }
                Err(e) => {
                    warn!("cargo clippy failed: {}", e);
                }
            }
        }

        // 3. cargo fmt --check
        if self.enable_fmt {
            debug!("Running cargo fmt --check...");
            let output = run_command(project_root, "cargo", &["fmt", "--check"]).await;
            match output {
                Ok(out) => {
                    let (issues, violations) = Self::parse_fmt_output(&out.stdout);
                    report.metrics.insert(MetricKey::FmtViolations, MeasureValue::Int(violations));
                    all_issues.extend(issues);
                }
                Err(e) => {
                    warn!("cargo fmt --check failed: {}", e);
                }
            }
        }

        // 4. cargo test
        if self.enable_test {
            debug!("Running cargo test...");
            let output = run_command(project_root, "cargo", &["test", "--workspace", "--no-fail-fast"]).await;
            match output {
                Ok(out) => {
                    let (issues, failures) = Self::parse_test_output(&out.stdout);
                    report.metrics.insert(MetricKey::RustTestFailures, MeasureValue::Int(failures));
                    all_issues.extend(issues);
                }
                Err(e) => {
                    warn!("cargo test failed: {}", e);
                    report.metrics.insert(MetricKey::RustTestFailures, MeasureValue::Int(-1));
                }
            }
        }

        report.issues = all_issues;
        report.duration_ms = start.elapsed().as_millis() as u64;

        Ok(report)
    }
}

/// 命令执行输出
struct CommandOutput {
    stdout: String,
    stderr: String,
    _success: bool,
}

/// 异步执行命令
async fn run_command(cwd: &Path, program: &str, args: &[&str]) -> anyhow::Result<CommandOutput> {
    let output = tokio::process::Command::new(program)
        .args(args)
        .current_dir(cwd)
        .output()
        .await?;

    Ok(CommandOutput {
        stdout: String::from_utf8_lossy(&output.stdout).to_string(),
        stderr: String::from_utf8_lossy(&output.stderr).to_string(),
        _success: output.status.success(),
    })
}
