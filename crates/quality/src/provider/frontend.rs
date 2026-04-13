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
            MetricKey::FrontendTestDepsMissing,
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

        // Fix #4: any target that contains test files should have its
        // declared-vs-imported test deps validated. We detect "test files"
        // by capability OR a fast filesystem probe so even repos without an
        // explicit `test` script still get the check.
        let has_test_files = self.enable_test
            && targets.iter().any(|target| target_has_test_files(target));

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
        if has_test_files {
            metrics.push(MetricKey::FrontendTestDepsMissing);
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

        // Fix #4: declared-vs-imported test dep coherence. Catches the exact
        // Task 1 R3 failure mode where test files import `@testing-library/*`
        // but `package.json` never declared the package.
        if self.enable_test {
            let mut total_missing = 0i64;
            let mut any_target_with_tests = false;
            for target in &targets {
                if !target_has_test_files(target) {
                    continue;
                }
                any_target_with_tests = true;
                let (missing_count, issues) =
                    scan_test_dependency_coherence(target, project_root);
                total_missing += missing_count;
                all_issues.extend(issues);
            }
            if any_target_with_tests {
                report.metrics.insert(
                    MetricKey::FrontendTestDepsMissing,
                    MeasureValue::Int(total_missing),
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

/// Module names whose presence in test files implies a hard package.json
/// declaration. Restricted to obvious test runtimes / DOM testing helpers,
/// not generic libraries — avoids flagging e.g. `from "react"`.
const TEST_DEP_GUARDED_PREFIXES: &[&str] = &[
    "@testing-library/",
    "@playwright/test",
    "playwright",
];

/// Quick scan: does any source file under `target.root()` look like a unit /
/// integration / e2e test file? Excludes `node_modules`, `dist`, `build`.
fn target_has_test_files(target: &JsTarget) -> bool {
    crate::analysis::collect_files(target.root(), is_test_file).into_iter().next().is_some()
}

fn is_test_file(path: &Path) -> bool {
    if !crate::analysis::is_ts_file(path) {
        return false;
    }
    let name = match path.file_name().and_then(|n| n.to_str()) {
        Some(n) => n,
        None => return false,
    };
    name.contains(".test.") || name.contains(".spec.") || name.ends_with("Test.tsx") || name.ends_with("Test.ts")
}

/// Walk every test file under `target.root()`, parse its imports, and emit a
/// blocking issue per missing dependency.
///
/// Returns `(missing_count, issues)`.
fn scan_test_dependency_coherence(
    target: &JsTarget,
    project_root: &Path,
) -> (i64, Vec<QualityIssue>) {
    let declared = target.dependency_names();
    let mut missing_pairs: std::collections::HashSet<(String, String)> =
        std::collections::HashSet::new();
    let mut sample_locations: std::collections::HashMap<String, String> =
        std::collections::HashMap::new();

    for test_file in crate::analysis::collect_files(target.root(), is_test_file) {
        let Ok(source) = std::fs::read_to_string(&test_file) else { continue };
        for module in extract_imported_modules(&source) {
            // Only audit the curated guard list — third-party generic libs
            // are out of scope; a missing `react` import would be caught by
            // tsc itself.
            let Some(guarded) = guarded_module_root(&module) else { continue };
            if declared.contains(&guarded) {
                continue;
            }
            // Record one sample location per missing pkg for human messaging.
            let rel = test_file
                .strip_prefix(project_root)
                .unwrap_or(&test_file)
                .to_string_lossy()
                .replace('\\', "/");
            let target_label = target.display_name(project_root);
            sample_locations
                .entry(guarded.clone())
                .or_insert_with(|| rel.clone());
            missing_pairs.insert((target_label, guarded));
        }
    }

    let mut issues = Vec::new();
    for (target_label, pkg) in &missing_pairs {
        let sample = sample_locations.get(pkg).cloned().unwrap_or_default();
        let mut issue = QualityIssue::new(
            "frontend::test_dep_missing",
            RuleType::Bug,
            Severity::Critical,
            AnalyzerSource::Other("frontend-deps".to_string()),
            format!(
                "[{}] test files import `{}` but it is NOT declared in package.json (deps/devDeps/peerDeps). \
                 Add `{}` (and matching `@types/...` if applicable) to {}/package.json — \
                 example file: {}",
                target_label, pkg, pkg, target_label, sample,
            ),
        )
        .with_effort(2);
        if !sample.is_empty() {
            issue = issue.with_location(sample, 1);
        }
        issues.push(issue);
    }

    (missing_pairs.len() as i64, issues)
}

/// Extract bare module specifiers from `import ... from "X"` and
/// `require("X")` forms. Cheap regex; intentionally tolerant.
fn extract_imported_modules(source: &str) -> Vec<String> {
    use std::sync::OnceLock;
    static RE: OnceLock<regex::Regex> = OnceLock::new();
    let re = RE.get_or_init(|| {
        // Matches all of:
        //   - `from "X"` / `from 'X'`            (named/default import)
        //   - `require("X")` / `require('X')`    (CommonJS)
        //   - `import "X"` / `import 'X'`        (ES side-effect import)
        //   - `import("X")` / `import('X')`      (ES dynamic import)
        // Word-boundary on `import` keeps `importable()` etc. from matching.
        // Primary-brain rejection v1 follow-up: dynamic `import('x')` was
        // missed by v1 — adding explicit `\bimport\s*\(` branch closes the
        // hole.
        regex::Regex::new(
            r#"(?:(?:from|require)\s*\(?|\bimport\s*\(|\bimport)\s*['"]([^'"]+)['"]"#,
        )
        .expect("import regex must compile")
    });
    let mut out = Vec::new();
    for cap in re.captures_iter(source) {
        if let Some(m) = cap.get(1) {
            let s = m.as_str().trim().to_string();
            if s.starts_with('.') || s.starts_with('/') {
                continue;
            }
            out.push(s);
        }
    }
    out
}

/// Reduce an imported module specifier to the package name that needs to
/// appear in `package.json`, or `None` if it's not on the guard list.
///
/// `@testing-library/react/dont-cleanup` → `@testing-library/react`
/// `playwright/test` → `playwright`
fn guarded_module_root(module: &str) -> Option<String> {
    for prefix in TEST_DEP_GUARDED_PREFIXES {
        if module == *prefix || module.starts_with(prefix) {
            // Scoped packages: `@scope/name` is the package; strip subpaths.
            let stripped = module.strip_prefix(prefix).unwrap_or("");
            // For `@testing-library/`, prefix already ends in `/`; the next
            // segment IS the rest of the package name, e.g. `react`.
            if let Some(stripped_pkg) = prefix.strip_suffix('/') {
                let rest = stripped.split('/').next().unwrap_or("");
                if rest.is_empty() {
                    return Some(prefix.trim_end_matches('/').to_string());
                }
                return Some(format!("{}/{}", stripped_pkg, rest));
            }
            return Some(prefix.to_string());
        }
    }
    None
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

    #[test]
    fn extract_imports_handles_es_and_commonjs() {
        let src = r#"
            import { render } from '@testing-library/react';
            import "@testing-library/jest-dom/matchers";
            import foo from "./relative";   // skipped
            const x = require('@playwright/test');
        "#;
        let mods = extract_imported_modules(src);
        assert!(mods.contains(&"@testing-library/react".to_string()));
        assert!(mods.contains(&"@testing-library/jest-dom/matchers".to_string()));
        assert!(mods.contains(&"@playwright/test".to_string()));
        assert!(!mods.iter().any(|m| m.starts_with('.')));
    }

    #[test]
    fn extract_imports_handles_dynamic_and_type_only() {
        // Primary-brain rejection v1 follow-up: dynamic `import('x')` and
        // `import type ... from 'x'` must both be detected.
        let src = r#"
            import type { Renderer } from '@testing-library/react';
            const lib = await import('@playwright/test');
            const userEvent = await import("@testing-library/user-event");
            function importable() { return 0; }   // must NOT be matched
        "#;
        let mods = extract_imported_modules(src);
        assert!(
            mods.contains(&"@testing-library/react".to_string()),
            "type-only import should match via `from`"
        );
        assert!(
            mods.contains(&"@playwright/test".to_string()),
            "single-quote dynamic import should match"
        );
        assert!(
            mods.contains(&"@testing-library/user-event".to_string()),
            "double-quote dynamic import should match"
        );
        // The function name `importable` must not produce a phantom module.
        assert!(
            !mods.iter().any(|m| m == "importable"),
            "word-boundary on `import` must prevent `importable` from matching"
        );
    }

    #[test]
    fn guarded_module_root_collapses_subpaths_for_scoped_pkg() {
        assert_eq!(
            guarded_module_root("@testing-library/react/dont-cleanup"),
            Some("@testing-library/react".to_string())
        );
        assert_eq!(
            guarded_module_root("@testing-library/jest-dom/matchers"),
            Some("@testing-library/jest-dom".to_string())
        );
        assert_eq!(
            guarded_module_root("playwright"),
            Some("playwright".to_string())
        );
        assert_eq!(guarded_module_root("react"), None);
        assert_eq!(guarded_module_root("./local"), None);
    }

    #[tokio::test]
    async fn fix4_emits_blocking_issue_when_test_imports_undeclared_testing_library() {
        // Repo shape: a single JS target whose package.json declares "react"
        // (so tsc is plausible) but NOT "@testing-library/react", yet a test
        // file imports it. Fix #4 must emit a blocking issue and surface
        // FrontendTestDepsMissing >= 1.
        let temp = temp_project_root();
        write_temp_file(
            &temp.join("package.json"),
            r#"{
  "name": "fe",
  "private": true,
  "dependencies": { "react": "^18.0.0" },
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );
        write_temp_file(&temp.join("tsconfig.json"), r#"{ "compilerOptions": {} }"#);
        write_temp_file(
            &temp.join("src/Foo.test.tsx"),
            r#"
import { render } from '@testing-library/react';
import "@testing-library/jest-dom/matchers";
test('renders', () => { render(<div />); });
"#,
        );

        let discovery = RepositoryDiscovery::discover(&temp).unwrap();
        let provider = FrontendProvider {
            // disable real subprocess work; only the deps coherence path runs.
            enable_lint: false,
            enable_check: false,
            enable_test: true,
        };

        let metrics = provider.applicable_metrics(&discovery, None);
        assert!(metrics.contains(&MetricKey::FrontendTestDepsMissing));

        let report = provider.analyze(&temp, &discovery, None).await.unwrap();
        let count = match report.metrics.get(&MetricKey::FrontendTestDepsMissing) {
            Some(MeasureValue::Int(v)) => *v,
            other => panic!("expected metric, got {other:?}"),
        };
        assert!(count >= 1, "expected at least one missing test dep, got {count}");

        let blocking_count = report
            .issues
            .iter()
            .filter(|i| i.rule_id == "frontend::test_dep_missing" && i.is_blocking())
            .count();
        assert!(blocking_count >= 1, "expected blocking test_dep_missing issue");

        remove_temp_project_root(&temp);
    }

    #[tokio::test]
    async fn fix4_silent_when_test_deps_are_declared() {
        let temp = temp_project_root();
        write_temp_file(
            &temp.join("package.json"),
            r#"{
  "name": "fe",
  "private": true,
  "dependencies": { "react": "^18.0.0" },
  "devDependencies": {
    "@testing-library/react": "^14.0.0",
    "@testing-library/jest-dom": "^6.0.0"
  },
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );
        write_temp_file(&temp.join("tsconfig.json"), r#"{ "compilerOptions": {} }"#);
        write_temp_file(
            &temp.join("src/Foo.test.tsx"),
            r#"
import { render } from '@testing-library/react';
import "@testing-library/jest-dom/matchers";
test('renders', () => {});
"#,
        );

        let discovery = RepositoryDiscovery::discover(&temp).unwrap();
        let provider = FrontendProvider {
            enable_lint: false,
            enable_check: false,
            enable_test: true,
        };

        let report = provider.analyze(&temp, &discovery, None).await.unwrap();
        let count = report
            .metrics
            .get(&MetricKey::FrontendTestDepsMissing)
            .cloned();
        // Either metric not present (target had no test files) or 0.
        match count {
            None => {}
            Some(MeasureValue::Int(v)) => assert_eq!(v, 0, "all deps declared, expected 0"),
            other => panic!("unexpected metric value {other:?}"),
        }
        let blockers = report
            .issues
            .iter()
            .filter(|i| i.rule_id == "frontend::test_dep_missing")
            .count();
        assert_eq!(blockers, 0);

        remove_temp_project_root(&temp);
    }
}
