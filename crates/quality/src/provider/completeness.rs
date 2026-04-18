//! Completeness Provider
//!
//! Detects universal structural completeness issues:
//! - Missing test files (projects with source code but zero tests)
//! - High TODO/FIXME density (indicates stub/placeholder code)

use async_trait::async_trait;
use regex::Regex;
use std::path::{Path, PathBuf};
use std::sync::OnceLock;
use std::time::Instant;
use tracing::debug;

use crate::analysis;
use crate::gate::result::MeasureValue;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};
use crate::rule::{AnalyzerSource, RuleType, Severity};

#[derive(Default)]
pub struct CompletenessProvider;

fn is_any_source_file(p: &Path) -> bool {
    analysis::is_rust_file(p) || analysis::is_ts_file(p) || is_go_file(p)
}

fn is_go_file(p: &Path) -> bool {
    p.extension().is_some_and(|ext| ext == "go")
}

fn todo_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"(?i)\b(?:TODO|FIXME|HACK|STUB|XXX)\b").unwrap())
}

fn test_file_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r"(?i)(?:\.(?:test|spec)\.[jt]sx?$|_test\.(?:rs|go)$)").unwrap()
    })
}

#[async_trait]
impl QualityProvider for CompletenessProvider {
    fn name(&self) -> &str {
        "completeness"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![MetricKey::TestFileAbsence, MetricKey::TodoDensity]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        debug!("completeness: starting analysis");

        let source_files = analysis::collect_files(project_root, is_any_source_file);

        let mut issues = Vec::new();
        let test_absence = detect_test_file_absence(project_root, &source_files, &mut issues);
        let todo_pct = compute_todo_density(project_root, &source_files, &mut issues);

        let duration_ms = start.elapsed().as_millis() as u64;
        debug!(
            "completeness: test_absence={test_absence} todo_density={todo_pct:.1}% in {duration_ms}ms"
        );

        let report = ProviderReport::success("completeness", duration_ms)
            .with_metric(
                MetricKey::TestFileAbsence,
                MeasureValue::Int(test_absence),
            )
            .with_metric(MetricKey::TodoDensity, MeasureValue::Float(todo_pct))
            .with_issues(issues);

        Ok(report)
    }
}

fn detect_test_file_absence(
    project_root: &Path,
    source_files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) -> i64 {
    if source_files.len() < 5 {
        return 0;
    }

    let has_test_files = source_files.iter().any(|f| {
        let name = f.to_string_lossy();
        test_file_re().is_match(&name)
    });

    let has_test_dir = project_root.join("tests").is_dir()
        || project_root.join("__tests__").is_dir()
        || project_root.join("test").is_dir();

    if !has_test_files && !has_test_dir {
        issues.push(QualityIssue::new(
            "completeness:test-file-absence",
            RuleType::Bug,
            Severity::Blocker,
            AnalyzerSource::Other("completeness".into()),
            format!(
                "Project has {} source files but zero test files — \
                 automated testing is required",
                source_files.len()
            ),
        ));
        1
    } else {
        0
    }
}

fn compute_todo_density(
    project_root: &Path,
    source_files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) -> f64 {
    if source_files.is_empty() {
        return 0.0;
    }

    let mut total_lines = 0usize;
    let mut todo_lines = 0usize;

    for file in source_files {
        let content = match std::fs::read_to_string(file) {
            Ok(c) => c,
            Err(_) => continue,
        };
        for line in content.lines() {
            total_lines += 1;
            if todo_re().is_match(line) {
                todo_lines += 1;
            }
        }
    }

    if total_lines == 0 {
        return 0.0;
    }

    let density = (todo_lines as f64 / total_lines as f64) * 100.0;

    if density > 3.0 {
        let rel_files: Vec<String> = source_files
            .iter()
            .filter_map(|f| {
                let content = std::fs::read_to_string(f).ok()?;
                let count = content.lines().filter(|l| todo_re().is_match(l)).count();
                if count > 0 {
                    let rel = f
                        .strip_prefix(project_root)
                        .unwrap_or(f)
                        .to_string_lossy()
                        .into_owned();
                    Some(format!("{rel} ({count})"))
                } else {
                    None
                }
            })
            .take(5)
            .collect();

        issues.push(QualityIssue::new(
            "completeness:high-todo-density",
            RuleType::CodeSmell,
            Severity::Critical,
            AnalyzerSource::Other("completeness".into()),
            format!(
                "TODO density {density:.1}% ({todo_lines}/{total_lines} lines) — \
                 indicates stub/placeholder code. Top files: {}",
                rel_files.join(", ")
            ),
        ));
    }

    density
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn temp_dir() -> PathBuf {
        let dir = std::env::temp_dir().join(format!("completeness_test_{}", uuid::Uuid::new_v4()));
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn write(dir: &Path, rel: &str, content: &str) {
        let path = dir.join(rel);
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).unwrap();
        }
        fs::write(path, content).unwrap();
    }

    fn cleanup(dir: &Path) {
        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn detects_test_file_absence() {
        let root = temp_dir();
        for i in 0..6 {
            write(&root, &format!("src/mod{i}.ts"), "export const x = 1;");
        }
        let files = analysis::collect_files(&root, is_any_source_file);
        let mut issues = Vec::new();
        let absence = detect_test_file_absence(&root, &files, &mut issues);
        assert_eq!(absence, 1);
        cleanup(&root);
    }

    #[test]
    fn no_absence_when_tests_exist() {
        let root = temp_dir();
        for i in 0..5 {
            write(&root, &format!("src/mod{i}.ts"), "export const x = 1;");
        }
        write(&root, "src/mod0.test.ts", "test('x', () => {});");
        let files = analysis::collect_files(&root, is_any_source_file);
        let mut issues = Vec::new();
        let absence = detect_test_file_absence(&root, &files, &mut issues);
        assert_eq!(absence, 0);
        cleanup(&root);
    }

    #[test]
    fn computes_todo_density() {
        let root = temp_dir();
        write(
            &root,
            "src/app.ts",
            "line1\nline2\n// TODO: do stuff\nline4\nline5\nline6\nline7\n// FIXME: broken\nline9\nline10\n",
        );
        let files = analysis::collect_files(&root, is_any_source_file);
        let mut issues = Vec::new();
        let density = compute_todo_density(&root, &files, &mut issues);
        assert!((density - 20.0).abs() < 0.1);
        assert!(!issues.is_empty());
        cleanup(&root);
    }
}
