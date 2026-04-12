//! Frontend 分析器 Provider
//!
//! 封装 pnpm lint / pnpm check / pnpm test:run 命令

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::gate::result::MeasureValue;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::{AnalyzerSource, RuleType, Severity};

/// Frontend 分析器 Provider
pub struct FrontendProvider {
    /// 前端目录（相对于项目根）
    pub frontend_dir: String,
    pub enable_lint: bool,
    pub enable_check: bool,
    pub enable_test: bool,
}

impl Default for FrontendProvider {
    fn default() -> Self {
        Self {
            frontend_dir: "frontend".to_string(),
            enable_lint: true,
            enable_check: true,
            enable_test: true,
        }
    }
}

#[async_trait]
impl QualityProvider for FrontendProvider {
    fn name(&self) -> &str {
        "frontend"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::EslintErrors,
            MetricKey::EslintWarnings,
            MetricKey::TscErrors,
            MetricKey::FrontendTestFailures,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let mut report = ProviderReport::success("frontend", 0);
        let mut all_issues = Vec::new();
        let frontend_dir = project_root.join(&self.frontend_dir);

        // Detect if the project is a monolithic TS project (tsconfig.json at root, no frontend/ subdir)
        // or a split project (frontend/ subdir with its own tsconfig).
        let tsc_check_dir = if frontend_dir.join("tsconfig.json").exists() {
            frontend_dir.clone()
        } else if project_root.join("tsconfig.json").exists() {
            // Monolithic TS project — run tsc at project root
            debug!("No frontend/tsconfig.json found, using project root for TypeScript checks");
            project_root.to_path_buf()
        } else {
            frontend_dir.clone()
        };

        // 1. pnpm lint (ESLint)
        if self.enable_lint {
            debug!("Running pnpm lint...");
            let output = run_frontend_command(&frontend_dir, &["lint"]).await;
            match output {
                Ok(out) => match classify_outcome(&out, ESLINT_ERROR_PATTERNS) {
                    ToolOutcome::Usable => {
                        let (errors, warnings, issues) = parse_eslint_output(&out.stdout);
                        report
                            .metrics
                            .insert(MetricKey::EslintErrors, MeasureValue::Int(errors));
                        report
                            .metrics
                            .insert(MetricKey::EslintWarnings, MeasureValue::Int(warnings));
                        all_issues.extend(issues);
                    }
                    ToolOutcome::Unavailable => {
                        warn!(
                            "pnpm lint failed to run (exit!=0, no ESLint output pattern). stderr head: {}",
                            stderr_head(&out.stderr)
                        );
                        report
                            .metrics
                            .insert(MetricKey::EslintErrors, MeasureValue::Int(-1));
                        all_issues.push(unavailable_issue(
                            "eslint::unavailable",
                            AnalyzerSource::EsLint,
                            "ESLint did not run (exit code non-zero, no ESLint summary in output)",
                            &out.stderr,
                        ));
                    }
                },
                Err(e) => {
                    warn!("pnpm lint failed to spawn: {}", e);
                    report
                        .metrics
                        .insert(MetricKey::EslintErrors, MeasureValue::Int(-1));
                    all_issues.push(QualityIssue::new(
                        "eslint::unavailable",
                        RuleType::Bug,
                        Severity::Critical,
                        AnalyzerSource::EsLint,
                        format!("ESLint could not run: {}", e),
                    ));
                }
            }
        }

        // 2. TypeScript type-check — try pnpm check first, fall back to npx tsc --noEmit
        if self.enable_check {
            debug!("Running TypeScript check in {}...", tsc_check_dir.display());
            let mut tsc_settled = false;
            let output = run_frontend_command(&tsc_check_dir, &["check"]).await;
            if let Ok(out) = &output {
                if matches!(classify_outcome(out, TSC_ERROR_PATTERNS), ToolOutcome::Usable) {
                    let combined = format!("{}\n{}", out.stdout, out.stderr);
                    let (errors, issues) = parse_tsc_output(&combined);
                    report
                        .metrics
                        .insert(MetricKey::TscErrors, MeasureValue::Int(errors));
                    all_issues.extend(issues);
                    tsc_settled = true;
                }
            }

            if !tsc_settled {
                // `pnpm check` either spawn-failed or ran but produced no TSC output.
                // Try `npx tsc --noEmit` directly — works for any TS project with tsconfig.json.
                if let Err(ref e) = output {
                    warn!("pnpm check failed: {}, falling back to npx tsc --noEmit", e);
                } else {
                    warn!("pnpm check produced no tsc output, falling back to npx tsc --noEmit");
                }
                let tsc_output = run_command(&tsc_check_dir, "npx", &["tsc", "--noEmit"]).await;
                match tsc_output {
                    Ok(out) => match classify_outcome(&out, TSC_ERROR_PATTERNS) {
                        ToolOutcome::Usable => {
                            let combined = format!("{}\n{}", out.stdout, out.stderr);
                            let (errors, issues) = parse_tsc_output(&combined);
                            report
                                .metrics
                                .insert(MetricKey::TscErrors, MeasureValue::Int(errors));
                            all_issues.extend(issues);
                        }
                        ToolOutcome::Unavailable => {
                            warn!(
                                "npx tsc --noEmit did not run (exit!=0, no 'error TS' output). stderr head: {}",
                                stderr_head(&out.stderr)
                            );
                            report
                                .metrics
                                .insert(MetricKey::TscErrors, MeasureValue::Int(-1));
                            all_issues.push(unavailable_issue(
                                "tsc::unavailable",
                                AnalyzerSource::TypeScript,
                                "TypeScript check did not run (npx tsc exit code non-zero, no 'error TS' in output)",
                                &out.stderr,
                            ));
                        }
                    },
                    Err(e2) => {
                        warn!("npx tsc --noEmit also failed to spawn: {}", e2);
                        report
                            .metrics
                            .insert(MetricKey::TscErrors, MeasureValue::Int(-1));
                        all_issues.push(QualityIssue::new(
                            "tsc::unavailable",
                            RuleType::Bug,
                            Severity::Critical,
                            AnalyzerSource::TypeScript,
                            format!(
                                "TypeScript check could not run (pnpm check and npx tsc --noEmit both failed): {}",
                                e2
                            ),
                        ));
                    }
                }
            }
        }

        // 3. pnpm test:run (Vitest)
        if self.enable_test {
            debug!("Running pnpm test:run...");
            let output = run_frontend_command(&frontend_dir, &["test:run"]).await;
            match output {
                Ok(out) => match classify_outcome(&out, VITEST_ERROR_PATTERNS) {
                    ToolOutcome::Usable => {
                        let (failures, issues) = parse_vitest_output(&out.stdout);
                        report
                            .metrics
                            .insert(MetricKey::FrontendTestFailures, MeasureValue::Int(failures));
                        all_issues.extend(issues);
                    }
                    ToolOutcome::Unavailable => {
                        warn!(
                            "pnpm test:run did not run (exit!=0, no Vitest output). stderr head: {}",
                            stderr_head(&out.stderr)
                        );
                        report
                            .metrics
                            .insert(MetricKey::FrontendTestFailures, MeasureValue::Int(-1));
                        all_issues.push(unavailable_issue(
                            "vitest::unavailable",
                            AnalyzerSource::Vitest,
                            "Vitest did not run (exit code non-zero, no Vitest output pattern)",
                            &out.stderr,
                        ));
                    }
                },
                Err(e) => {
                    warn!("pnpm test:run failed to spawn: {}", e);
                    report
                        .metrics
                        .insert(MetricKey::FrontendTestFailures, MeasureValue::Int(-1));
                    all_issues.push(QualityIssue::new(
                        "vitest::unavailable",
                        RuleType::Bug,
                        Severity::Critical,
                        AnalyzerSource::Vitest,
                        format!("Vitest could not run: {}", e),
                    ));
                }
            }
        }

        report.issues = all_issues;
        report.duration_ms = start.elapsed().as_millis() as u64;

        Ok(report)
    }
}

/// G33-001: outcome classification for a subprocess that exited non-zero.
///
/// `Usable` = safe to parse the output (either the tool ran successfully, or it
/// failed *because* it reported quality issues — both cases leave the tool's
/// diagnostic strings in stdout/stderr).
/// `Unavailable` = the tool did not actually run (script missing, binary not
/// found, dependencies missing). The -1 sentinel is emitted and a Critical
/// QualityIssue is recorded so the gate fails closed.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ToolOutcome {
    Usable,
    Unavailable,
}

// Tool-specific "I actually ran" signatures. Kept narrow to avoid false-positive
// Usable classification on unrelated exit-code-nonzero output (e.g. shell errors
// that happen to contain the word "error").
const ESLINT_ERROR_PATTERNS: &[&str] = &["problems"];
const TSC_ERROR_PATTERNS: &[&str] = &["error TS"];
const VITEST_ERROR_PATTERNS: &[&str] = &["FAIL ", "Tests:"];

fn classify_outcome(out: &CommandOutput, error_patterns: &[&str]) -> ToolOutcome {
    if out.success {
        return ToolOutcome::Usable;
    }
    let combined_lower = format!("{}\n{}", out.stdout, out.stderr);
    if error_patterns
        .iter()
        .any(|p| combined_lower.contains(p))
    {
        ToolOutcome::Usable
    } else {
        ToolOutcome::Unavailable
    }
}

fn unavailable_issue(
    rule_id: &str,
    source: AnalyzerSource,
    reason: &str,
    stderr: &str,
) -> QualityIssue {
    QualityIssue::new(
        rule_id,
        RuleType::Bug,
        Severity::Critical,
        source,
        format!("{}. stderr: {}", reason, stderr_head(stderr)),
    )
}

fn stderr_head(stderr: &str) -> String {
    stderr
        .lines()
        .take(3)
        .collect::<Vec<_>>()
        .join(" | ")
        .chars()
        .take(300)
        .collect()
}

/// 解析 ESLint 输出
fn parse_eslint_output(output: &str) -> (i64, i64, Vec<QualityIssue>) {
    let mut errors = 0i64;
    let mut warnings = 0i64;
    let mut issues = Vec::new();

    // First pass: look for the summary line to get accurate counts
    for line in output.lines() {
        let trimmed = line.trim();
        if trimmed.contains("problems") {
            if let Some(summary) = parse_eslint_summary(trimmed) {
                errors = summary.0;
                warnings = summary.1;
            }
        }
    }

    // Second pass: parse actual ESLint issue lines (indented lines with "error"/"warning")
    for line in output.lines() {
        let trimmed = line.trim();
        // ESLint issue lines are indented and start with "line:col  severity  message  rule"
        if !line.starts_with(' ') && !line.starts_with('\t') {
            continue;
        }
        if trimmed.contains("problems") {
            continue;
        }
        if trimmed.contains("error") {
            issues.push(QualityIssue::new(
                "eslint::error",
                RuleType::Bug,
                Severity::Critical,
                AnalyzerSource::EsLint,
                trimmed,
            ));
        } else if trimmed.contains("warning") {
            issues.push(QualityIssue::new(
                "eslint::warning",
                RuleType::CodeSmell,
                Severity::Major,
                AnalyzerSource::EsLint,
                trimmed,
            ));
        }
    }

    // If no summary line was found, count from parsed issues
    if errors == 0 && warnings == 0 {
        errors = issues.iter().filter(|i| i.rule_id == "eslint::error").count() as i64;
        warnings = issues.iter().filter(|i| i.rule_id == "eslint::warning").count() as i64;
    }

    (errors, warnings, issues)
}

/// 解析 ESLint 汇总行
fn parse_eslint_summary(line: &str) -> Option<(i64, i64)> {
    // 格式: "N problems (X errors, Y warnings)"
    let re = regex::Regex::new(r"(\d+)\s+errors?,\s+(\d+)\s+warnings?").ok()?;
    let caps = re.captures(line)?;
    let errors = caps.get(1)?.as_str().parse().ok()?;
    let warnings = caps.get(2)?.as_str().parse().ok()?;
    Some((errors, warnings))
}

/// 解析 TypeScript 编译器输出
fn parse_tsc_output(output: &str) -> (i64, Vec<QualityIssue>) {
    let mut errors = 0i64;
    let mut issues = Vec::new();

    for line in output.lines() {
        // tsc 输出格式: "file(line,col): error TSxxxx: message"
        if line.contains("error TS") {
            errors += 1;
            issues.push(QualityIssue::new(
                "tsc::error",
                RuleType::Bug,
                Severity::Critical,
                AnalyzerSource::TypeScript,
                line.trim(),
            ));
        }
    }

    (errors, issues)
}

/// 解析 Vitest 输出
fn parse_vitest_output(output: &str) -> (i64, Vec<QualityIssue>) {
    let mut failures = 0i64;
    let mut issues = Vec::new();

    for line in output.lines() {
        if line.contains("FAIL") && !line.contains("Tests:") {
            failures += 1;
            issues.push(QualityIssue::new(
                "vitest::failure",
                RuleType::Bug,
                Severity::Critical,
                AnalyzerSource::Vitest,
                line.trim(),
            ));
        }

        // 解析 Vitest 汇总行: "Tests: X failed, Y passed, Z total"
        if line.contains("Tests:") && line.contains("failed") {
            if let Some(n) = extract_number_before(line, "failed") {
                failures = n;
            }
        }
    }

    (failures, issues)
}

/// 从文本中提取指定词前面的数字
fn extract_number_before(text: &str, keyword: &str) -> Option<i64> {
    let idx = text.find(keyword)?;
    let before = &text[..idx].trim();
    let num_str = before.rsplit(|c: char| !c.is_ascii_digit()).next()?;
    num_str.parse().ok()
}

/// 命令输出
struct CommandOutput {
    stdout: String,
    stderr: String,
    success: bool,
}

/// 执行任意命令
async fn run_command(cwd: &Path, cmd: &str, args: &[&str]) -> anyhow::Result<CommandOutput> {
    let output = tokio::process::Command::new(cmd)
        .args(args)
        .current_dir(cwd)
        .output()
        .await?;

    Ok(CommandOutput {
        stdout: String::from_utf8_lossy(&output.stdout).to_string(),
        stderr: String::from_utf8_lossy(&output.stderr).to_string(),
        success: output.status.success(),
    })
}

/// 执行前端 pnpm 命令（convenience wrapper）
async fn run_frontend_command(cwd: &Path, args: &[&str]) -> anyhow::Result<CommandOutput> {
    run_command(cwd, "pnpm", args).await
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fake_output(stdout: &str, stderr: &str, success: bool) -> CommandOutput {
        CommandOutput {
            stdout: stdout.to_string(),
            stderr: stderr.to_string(),
            success,
        }
    }

    #[test]
    fn classify_success_is_usable() {
        let out = fake_output("0 problems", "", true);
        assert_eq!(
            classify_outcome(&out, ESLINT_ERROR_PATTERNS),
            ToolOutcome::Usable
        );
    }

    #[test]
    fn classify_tsc_failure_with_errors_is_usable() {
        // tsc exits non-zero when it reports type errors — but output is valid
        let out = fake_output(
            "src/foo.ts(12,5): error TS2322: Type mismatch.",
            "",
            false,
        );
        assert_eq!(
            classify_outcome(&out, TSC_ERROR_PATTERNS),
            ToolOutcome::Usable
        );
    }

    #[test]
    fn classify_pnpm_missing_script_is_unavailable() {
        // pnpm reports ERR_PNPM_NO_SCRIPT with empty stdout — must NOT be mistaken for "0 errors"
        let out = fake_output(
            "",
            "ERR_PNPM_NO_SCRIPT  Missing script: check",
            false,
        );
        assert_eq!(
            classify_outcome(&out, TSC_ERROR_PATTERNS),
            ToolOutcome::Unavailable
        );
    }

    #[test]
    fn classify_eslint_failure_with_problems_summary_is_usable() {
        let out = fake_output(
            "/src/foo.ts\n  1:1  error  Unexpected var  no-var\n\n1 problems (1 errors, 0 warnings)",
            "",
            false,
        );
        assert_eq!(
            classify_outcome(&out, ESLINT_ERROR_PATTERNS),
            ToolOutcome::Usable
        );
    }

    #[test]
    fn classify_vitest_fail_line_is_usable() {
        let out = fake_output("FAIL  src/foo.test.ts > works", "", false);
        assert_eq!(
            classify_outcome(&out, VITEST_ERROR_PATTERNS),
            ToolOutcome::Usable
        );
    }

    #[test]
    fn classify_silent_failure_is_unavailable() {
        // Command exits non-zero but produces no diagnostic output at all
        let out = fake_output("", "", false);
        assert_eq!(
            classify_outcome(&out, TSC_ERROR_PATTERNS),
            ToolOutcome::Unavailable
        );
        assert_eq!(
            classify_outcome(&out, ESLINT_ERROR_PATTERNS),
            ToolOutcome::Unavailable
        );
        assert_eq!(
            classify_outcome(&out, VITEST_ERROR_PATTERNS),
            ToolOutcome::Unavailable
        );
    }

    #[test]
    fn unavailable_issue_is_critical_blocking() {
        let issue = unavailable_issue(
            "tsc::unavailable",
            AnalyzerSource::TypeScript,
            "TypeScript check did not run",
            "ERR_PNPM_NO_SCRIPT",
        );
        assert_eq!(issue.severity, Severity::Critical);
        assert!(issue.is_blocking());
    }

    #[test]
    fn stderr_head_truncates_long_output() {
        let long = "a".repeat(1000);
        let head = stderr_head(&long);
        assert!(head.len() <= 300);
    }
}
