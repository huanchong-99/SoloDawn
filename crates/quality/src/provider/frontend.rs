//! Frontend / JS-TS quality provider
//!
//! Executes lint / type-check / test commands for discovered JS/TS package targets.

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, warn};

use crate::discovery::{JsTarget, NodeQualityCommand, RepositoryDiscovery};
use crate::gate::result::MeasureValue;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;
use crate::provider::{run_node_quality_command, ProviderReport, QualityProvider};
use crate::rule::{AnalyzerSource, RuleType, Severity};

/// Frontend 分析器 Provider
pub struct FrontendProvider {
    pub enable_lint: bool,
    pub enable_check: bool,
    pub enable_test: bool,
}

impl Default for FrontendProvider {
    fn default() -> Self {
        Self {
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

    fn applicable_metrics(
        &self,
        discovery: &RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> Vec<MetricKey> {
        let targets = discovery.applicable_js_targets(changed_files);
        if targets.is_empty() {
            return Vec::new();
        }

        let has_lint = self.enable_lint
            && targets
                .iter()
                .any(|target| target.capabilities().lint.is_some());
        let has_typecheck = self.enable_check
            && targets.iter().any(|target| {
                target.capabilities().typecheck.is_some() || target.has_tsconfig()
            });
        let has_test = self.enable_test
            && targets
                .iter()
                .any(|target| target.capabilities().test.is_some());

        let mut metrics = Vec::new();
        if has_lint {
            metrics.push(MetricKey::EslintErrors);
            metrics.push(MetricKey::EslintWarnings);
        }
        if has_typecheck {
            metrics.push(MetricKey::TscErrors);
        }
        if has_test {
            metrics.push(MetricKey::FrontendTestFailures);
        }
        metrics
    }

    async fn analyze(
        &self,
        project_root: &Path,
        discovery: &RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let mut report = ProviderReport::success("frontend", 0);
        let mut all_issues = Vec::new();
        let targets = discovery.applicable_js_targets(changed_files);

        if targets.is_empty() {
            debug!("frontend provider skipped: no discovered JS/TS targets");
            report.duration_ms = start.elapsed().as_millis() as u64;
            return Ok(report);
        }

        debug!(
            targets = ?targets
                .iter()
                .map(|target| target.display_name(project_root))
                .collect::<Vec<_>>(),
            changed_files = ?changed_files,
            "frontend provider analyzing discovered targets"
        );

        if self.enable_lint {
            let mut lint_errors = 0i64;
            let mut lint_warnings = 0i64;
            let mut lint_attempted = false;
            let mut lint_failed_closed = false;

            for target in &targets {
                let Some(command) = target.capabilities().lint.clone() else {
                    continue;
                };
                lint_attempted = true;
                match run_target_quality_command(target, &command).await {
                    Ok(out) => match classify_outcome(&out, ESLINT_ERROR_PATTERNS) {
                        ToolOutcome::Usable => {
                            let (errors, warnings, issues) = parse_eslint_output(&out.stdout);
                            lint_errors += errors;
                            lint_warnings += warnings;
                            all_issues.extend(prefix_issues(project_root, target, issues));
                        }
                        ToolOutcome::Unavailable => {
                            lint_failed_closed = true;
                            warn!(
                                target = %target.display_name(project_root),
                                command = %command.describe(),
                                "Lint command unavailable: {}",
                                stderr_head(&out.stderr)
                            );
                            all_issues.push(unavailable_issue(
                                "eslint::unavailable",
                                AnalyzerSource::EsLint,
                                &format!(
                                    "ESLint did not run for target {} via {}",
                                    target.display_name(project_root),
                                    command.describe()
                                ),
                                &out.stderr,
                            ));
                        }
                    },
                    Err(error) => {
                        lint_failed_closed = true;
                        warn!(
                            target = %target.display_name(project_root),
                            command = %command.describe(),
                            "Lint command failed to spawn: {error}"
                        );
                        all_issues.push(QualityIssue::new(
                            "eslint::unavailable",
                            RuleType::Bug,
                            Severity::Critical,
                            AnalyzerSource::EsLint,
                            format!(
                                "ESLint could not run for target {} via {}: {}",
                                target.display_name(project_root),
                                command.describe(),
                                error
                            ),
                        ));
                    }
                }
            }

            if lint_attempted {
                report.metrics.insert(
                    MetricKey::EslintErrors,
                    MeasureValue::Int(if lint_failed_closed { -1 } else { lint_errors }),
                );
                report.metrics.insert(
                    MetricKey::EslintWarnings,
                    MeasureValue::Int(if lint_failed_closed { -1 } else { lint_warnings }),
                );
            }
        }

        if self.enable_check {
            let mut tsc_errors = 0i64;
            let mut tsc_attempted = false;
            let mut tsc_failed_closed = false;

            for target in &targets {
                let Some(command) = resolve_typecheck_command(target) else {
                    continue;
                };
                tsc_attempted = true;
                match run_target_quality_command(target, &command).await {
                    Ok(out) => match classify_outcome(&out, TSC_ERROR_PATTERNS) {
                        ToolOutcome::Usable => {
                            let combined = format!("{}\n{}", out.stdout, out.stderr);
                            let (errors, issues) = parse_tsc_output(&combined);
                            tsc_errors += errors;
                            all_issues.extend(prefix_issues(project_root, target, issues));
                        }
                        ToolOutcome::Unavailable => {
                            tsc_failed_closed = true;
                            warn!(
                                target = %target.display_name(project_root),
                                command = %command.describe(),
                                "TypeScript command unavailable: {}",
                                stderr_head(&out.stderr)
                            );
                            all_issues.push(unavailable_issue(
                                "tsc::unavailable",
                                AnalyzerSource::TypeScript,
                                &format!(
                                    "TypeScript check did not run for target {} via {}",
                                    target.display_name(project_root),
                                    command.describe()
                                ),
                                &out.stderr,
                            ));
                        }
                    },
                    Err(error) => {
                        tsc_failed_closed = true;
                        warn!(
                            target = %target.display_name(project_root),
                            command = %command.describe(),
                            "TypeScript command failed to spawn: {error}"
                        );
                        all_issues.push(QualityIssue::new(
                            "tsc::unavailable",
                            RuleType::Bug,
                            Severity::Critical,
                            AnalyzerSource::TypeScript,
                            format!(
                                "TypeScript check could not run for target {} via {}: {}",
                                target.display_name(project_root),
                                command.describe(),
                                error
                            ),
                        ));
                    }
                }
            }

            if tsc_attempted {
                report.metrics.insert(
                    MetricKey::TscErrors,
                    MeasureValue::Int(if tsc_failed_closed { -1 } else { tsc_errors }),
                );
            }
        }

        if self.enable_test {
            let mut test_failures = 0i64;
            let mut test_attempted = false;
            let mut test_failed_closed = false;

            for target in &targets {
                let Some(command) = target.capabilities().test.clone() else {
                    continue;
                };
                test_attempted = true;
                match run_target_quality_command(target, &command).await {
                    Ok(out) => match classify_outcome(&out, VITEST_ERROR_PATTERNS) {
                        ToolOutcome::Usable => {
                            let combined = format!("{}\n{}", out.stdout, out.stderr);
                            let (failures, issues) = parse_vitest_output(&combined);
                            test_failures += failures;
                            all_issues.extend(prefix_issues(project_root, target, issues));
                        }
                        ToolOutcome::Unavailable => {
                            test_failed_closed = true;
                            warn!(
                                target = %target.display_name(project_root),
                                command = %command.describe(),
                                "Test command unavailable: {}",
                                stderr_head(&out.stderr)
                            );
                            all_issues.push(unavailable_issue(
                                "vitest::unavailable",
                                AnalyzerSource::Vitest,
                                &format!(
                                    "Frontend tests did not run for target {} via {}",
                                    target.display_name(project_root),
                                    command.describe()
                                ),
                                &out.stderr,
                            ));
                        }
                    },
                    Err(error) => {
                        test_failed_closed = true;
                        warn!(
                            target = %target.display_name(project_root),
                            command = %command.describe(),
                            "Test command failed to spawn: {error}"
                        );
                        all_issues.push(QualityIssue::new(
                            "vitest::unavailable",
                            RuleType::Bug,
                            Severity::Critical,
                            AnalyzerSource::Vitest,
                            format!(
                                "Frontend tests could not run for target {} via {}: {}",
                                target.display_name(project_root),
                                command.describe(),
                                error
                            ),
                        ));
                    }
                }
            }

            if test_attempted {
                report.metrics.insert(
                    MetricKey::FrontendTestFailures,
                    MeasureValue::Int(if test_failed_closed { -1 } else { test_failures }),
                );
            }
        }

        report.issues = all_issues;
        report.duration_ms = start.elapsed().as_millis() as u64;
        Ok(report)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ToolOutcome {
    Usable,
    Unavailable,
}

const ESLINT_ERROR_PATTERNS: &[&str] = &["problems"];
const TSC_ERROR_PATTERNS: &[&str] = &["error TS"];
const VITEST_ERROR_PATTERNS: &[&str] = &["FAIL ", "Tests:"];

fn classify_outcome(out: &CommandOutput, error_patterns: &[&str]) -> ToolOutcome {
    if out.success {
        return ToolOutcome::Usable;
    }
    if error_patterns
        .iter()
        .any(|pattern| out.stdout.contains(pattern) || out.stderr.contains(pattern))
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

fn resolve_typecheck_command(target: &JsTarget) -> Option<NodeQualityCommand> {
    target.capabilities().typecheck.clone().or_else(|| {
        target.has_tsconfig().then_some(NodeQualityCommand::PackageExec {
            binary: "tsc".to_string(),
            args: vec!["--noEmit".to_string()],
        })
    })
}

fn prefix_issues(repo_root: &Path, target: &JsTarget, issues: Vec<QualityIssue>) -> Vec<QualityIssue> {
    let prefix = target.relative_root(repo_root);
    if prefix == "." {
        return issues;
    }

    issues
        .into_iter()
        .map(|mut issue| {
            if !issue.message.contains(&prefix) {
                issue.message = format!("[{prefix}] {}", issue.message);
            }
            issue
        })
        .collect()
}

/// 解析 ESLint 输出
fn parse_eslint_output(output: &str) -> (i64, i64, Vec<QualityIssue>) {
    let mut errors = 0i64;
    let mut warnings = 0i64;
    let mut issues = Vec::new();

    for line in output.lines() {
        let trimmed = line.trim();
        if trimmed.contains("problems") {
            if let Some(summary) = parse_eslint_summary(trimmed) {
                errors = summary.0;
                warnings = summary.1;
            }
        }
    }

    for line in output.lines() {
        let trimmed = line.trim();
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

    if errors == 0 && warnings == 0 {
        errors = issues.iter().filter(|issue| issue.rule_id == "eslint::error").count() as i64;
        warnings = issues.iter().filter(|issue| issue.rule_id == "eslint::warning").count() as i64;
    }

    (errors, warnings, issues)
}

fn parse_eslint_summary(line: &str) -> Option<(i64, i64)> {
    let re = regex::Regex::new(r"(\d+)\s+errors?,\s+(\d+)\s+warnings?").ok()?;
    let caps = re.captures(line)?;
    let errors = caps.get(1)?.as_str().parse().ok()?;
    let warnings = caps.get(2)?.as_str().parse().ok()?;
    Some((errors, warnings))
}

fn parse_tsc_output(output: &str) -> (i64, Vec<QualityIssue>) {
    let mut errors = 0i64;
    let mut issues = Vec::new();

    for line in output.lines() {
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

        if line.contains("Tests:") && line.contains("failed") {
            if let Some(n) = extract_number_before(line, "failed") {
                failures = n;
            }
        }
    }

    (failures, issues)
}

fn extract_number_before(text: &str, keyword: &str) -> Option<i64> {
    let idx = text.find(keyword)?;
    let before = &text[..idx].trim();
    let num_str = before.rsplit(|c: char| !c.is_ascii_digit()).next()?;
    num_str.parse().ok()
}

struct CommandOutput {
    stdout: String,
    stderr: String,
    success: bool,
}

async fn run_target_quality_command(
    target: &JsTarget,
    command: &NodeQualityCommand,
) -> anyhow::Result<CommandOutput> {
    let output = run_node_quality_command(target.root(), target.package_manager(), command).await?;

    Ok(CommandOutput {
        stdout: String::from_utf8_lossy(&output.stdout).to_string(),
        stderr: String::from_utf8_lossy(&output.stderr).to_string(),
        success: output.status.success(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::discovery::{resolve_node_command, PackageManager, RepositoryDiscovery};

    fn fake_output(stdout: &str, stderr: &str, success: bool) -> CommandOutput {
        CommandOutput {
            stdout: stdout.to_string(),
            stderr: stderr.to_string(),
            success,
        }
    }

    fn temp_project_root() -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!("quality-frontend-provider-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn write_temp_file(path: &Path, content: &str) {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).unwrap();
        }
        std::fs::write(path, content).unwrap();
    }

    fn remove_temp_project_root(path: &Path) {
        let _ = std::fs::remove_dir_all(path);
    }

    #[test]
    fn applicable_metrics_empty_without_js_targets() {
        let temp = temp_project_root();
        let discovery = RepositoryDiscovery::discover(&temp).unwrap();
        let provider = FrontendProvider::default();
        assert!(provider.applicable_metrics(&discovery, None).is_empty());
        remove_temp_project_root(&temp);
    }

    #[test]
    fn applicable_metrics_include_tsc_for_backend_workspace() {
        let temp = temp_project_root();
        write_temp_file(
            &temp.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["backend", "frontend", "shared"],
  "packageManager": "npm@10.0.0"
}"#,
        );
        write_temp_file(
            &temp.join("backend/package.json"),
            r#"{
  "name": "backend",
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );
        write_temp_file(
            &temp.join("frontend/package.json"),
            r#"{
  "name": "frontend",
  "scripts": { "lint": "eslint ." }
}"#,
        );
        let discovery = RepositoryDiscovery::discover(&temp).unwrap();
        let provider = FrontendProvider::default();
        let metrics = provider.applicable_metrics(
            &discovery,
            Some(&["backend/src/index.ts".to_string()]),
        );
        assert!(metrics.contains(&MetricKey::TscErrors));
        remove_temp_project_root(&temp);
    }

    #[test]
    fn resolve_command_uses_package_manager_scripts() {
        let (cmd, args) = resolve_node_command(
            Some(PackageManager::Pnpm),
            &NodeQualityCommand::Script {
                script: "type-check".to_string(),
            },
        );
        assert_eq!(cmd, "pnpm");
        assert_eq!(args, vec!["run", "type-check"]);
    }

    #[test]
    fn resolve_command_uses_package_exec_for_tsc() {
        let (cmd, args) = resolve_node_command(
            Some(PackageManager::Npm),
            &NodeQualityCommand::PackageExec {
                binary: "tsc".to_string(),
                args: vec!["--noEmit".to_string()],
            },
        );
        assert_eq!(cmd, "npx");
        assert_eq!(args, vec!["tsc", "--noEmit"]);
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
    fn classify_missing_script_is_unavailable() {
        let out = fake_output("", "ERR_PNPM_NO_SCRIPT Missing script", false);
        assert_eq!(
            classify_outcome(&out, TSC_ERROR_PATTERNS),
            ToolOutcome::Unavailable
        );
    }

    #[test]
    fn classify_vitest_fail_line_is_usable() {
        let out = fake_output("FAIL src/foo.test.ts > works", "", false);
        assert_eq!(
            classify_outcome(&out, VITEST_ERROR_PATTERNS),
            ToolOutcome::Usable
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
}
