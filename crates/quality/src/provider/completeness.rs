//! Completeness Provider
//!
//! Detects universal structural completeness issues:
//! - Missing test files (projects with source code but zero tests)
//! - High TODO/FIXME density (indicates stub/placeholder code)
//! - Placeholder tests that assert nothing meaningful
//! - Coverage configurations that exclude core business layers

use std::{
    path::{Path, PathBuf},
    sync::OnceLock,
    time::Instant,
};

use async_trait::async_trait;
use regex::Regex;
use tracing::debug;

use crate::{
    analysis,
    gate::result::MeasureValue,
    issue::QualityIssue,
    metrics::MetricKey,
    provider::{ProviderReport, QualityProvider},
    rule::{AnalyzerSource, RuleType, Severity},
};

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
    RE.get_or_init(|| Regex::new(r"(?i)(?:\.(?:test|spec)\.[jt]sx?$|_test\.(?:rs|go)$)").unwrap())
}

fn is_test_file(p: &Path) -> bool {
    if !is_any_source_file(p) {
        return false;
    }
    let name = p.to_string_lossy().replace('\\', "/").to_lowercase();
    test_file_re().is_match(&name)
        || name.contains("/tests/")
        || name.contains("/__tests__/")
        || name.contains("/test/")
}

fn is_coverage_config_file(p: &Path) -> bool {
    let name = p
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or_default()
        .to_lowercase();
    name == "package.json"
        || name.starts_with("jest.config.")
        || name.starts_with("vitest.config.")
        || name.starts_with("nyc.config.")
        || name.starts_with("c8.config.")
        || name == ".nycrc"
        || name.starts_with(".nycrc.")
}

fn files_for_scope(
    project_root: &Path,
    changed_files: Option<&[String]>,
    filter: fn(&Path) -> bool,
) -> Vec<PathBuf> {
    match changed_files {
        Some(files) => files
            .iter()
            .filter_map(|file| changed_file_path(project_root, file))
            .filter(|path| path.is_file() && filter(path))
            .collect(),
        None => analysis::collect_files(project_root, filter),
    }
}

fn changed_file_path(project_root: &Path, file: &str) -> Option<PathBuf> {
    let trimmed = file.trim();
    if trimmed.is_empty() {
        return None;
    }
    let path = PathBuf::from(trimmed);
    if path.is_absolute() {
        Some(path)
    } else {
        Some(project_root.join(trimmed))
    }
}

#[async_trait]
impl QualityProvider for CompletenessProvider {
    fn name(&self) -> &str {
        "completeness"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::TestFileAbsence,
            MetricKey::TodoDensity,
            MetricKey::StubTestCount,
            MetricKey::CoverageExclusionIssues,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        debug!("completeness: starting analysis");

        let source_files = analysis::collect_files(project_root, is_any_source_file);
        let scoped_test_files = files_for_scope(project_root, changed_files, is_test_file);
        let scoped_coverage_config_files =
            files_for_scope(project_root, changed_files, is_coverage_config_file);

        let mut issues = Vec::new();
        let test_absence = detect_test_file_absence(project_root, &source_files, &mut issues);
        let todo_pct = compute_todo_density(project_root, &source_files, &mut issues);
        let stub_tests = detect_stub_tests(project_root, &scoped_test_files, &mut issues);
        let coverage_exclusions = detect_suspicious_coverage_exclusions(
            project_root,
            &scoped_coverage_config_files,
            &mut issues,
        );

        let duration_ms = start.elapsed().as_millis() as u64;
        debug!(
            "completeness: test_absence={test_absence} todo_density={todo_pct:.1}% \
             stub_tests={stub_tests} coverage_exclusions={coverage_exclusions} in {duration_ms}ms"
        );

        let report = ProviderReport::success("completeness", duration_ms)
            .with_metric(MetricKey::TestFileAbsence, MeasureValue::Int(test_absence))
            .with_metric(MetricKey::TodoDensity, MeasureValue::Float(todo_pct))
            .with_metric(MetricKey::StubTestCount, MeasureValue::Int(stub_tests))
            .with_metric(
                MetricKey::CoverageExclusionIssues,
                MeasureValue::Int(coverage_exclusions),
            )
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

fn detect_stub_tests(
    project_root: &Path,
    test_files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) -> i64 {
    let mut count = 0;
    for file in test_files {
        let content = match std::fs::read_to_string(file) {
            Ok(content) => content,
            Err(_) => continue,
        };
        if !contains_stub_test(&content) {
            continue;
        }
        count += 1;
        let line = stub_test_line(&content).unwrap_or(1);
        let rel = rel_path(project_root, file);
        issues.push(
            QualityIssue::new(
                "completeness:stub-test",
                RuleType::Bug,
                Severity::Blocker,
                AnalyzerSource::Other("completeness".into()),
                "Test file contains placeholder assertions such as expect(true).toBe(true); \
                 tests must assert real behavior",
            )
            .with_location(rel, line),
        );
    }
    count
}

fn contains_stub_test(content: &str) -> bool {
    let compact = content
        .chars()
        .filter(|c| !c.is_whitespace())
        .collect::<String>()
        .to_lowercase();
    compact.contains("expect(true).tobe(true)")
        || compact.contains("expect(1).tobe(1)")
        || compact.contains("assert(true)")
        || compact.contains("assert.ok(true)")
        || compact.contains("t.pass()")
}

fn stub_test_line(content: &str) -> Option<u32> {
    content.lines().enumerate().find_map(|(idx, line)| {
        if contains_stub_test(line) {
            Some((idx + 1) as u32)
        } else {
            None
        }
    })
}

fn detect_suspicious_coverage_exclusions(
    project_root: &Path,
    config_files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) -> i64 {
    let mut count = 0;
    for file in config_files {
        let content = match std::fs::read_to_string(file) {
            Ok(content) => content,
            Err(_) => continue,
        };
        let Some(line) = suspicious_coverage_exclusion_line(&content) else {
            continue;
        };
        count += 1;
        let rel = rel_path(project_root, file);
        issues.push(
            QualityIssue::new(
                "completeness:coverage-core-exclusion",
                RuleType::Bug,
                Severity::Blocker,
                AnalyzerSource::Other("completeness".into()),
                "Coverage configuration appears to exclude core business layers \
                 (services/controllers/routes/models/repositories); coverage must measure changed core code",
            )
            .with_location(rel, line),
        );
    }
    count
}

fn suspicious_coverage_exclusion_line(content: &str) -> Option<u32> {
    if !content.to_lowercase().contains("coverage") {
        return None;
    }

    content.lines().enumerate().find_map(|(idx, line)| {
        let lower = line.to_lowercase();
        let excludes = lower.contains("exclude")
            || lower.contains("ignorepattern")
            || lower.contains("ignore-pattern");
        let core_layer = [
            "services",
            "controllers",
            "routes",
            "models",
            "repositories",
            "middleware",
        ]
        .iter()
        .any(|term| lower.contains(term));

        if excludes && core_layer {
            Some((idx + 1) as u32)
        } else {
            None
        }
    })
}

fn rel_path(project_root: &Path, file: &Path) -> String {
    file.strip_prefix(project_root)
        .unwrap_or(file)
        .to_string_lossy()
        .replace('\\', "/")
}

#[cfg(test)]
mod tests {
    use std::fs;

    use super::*;

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

    #[test]
    fn detects_stub_tests() {
        let root = temp_dir();
        write(
            &root,
            "tests/user.test.ts",
            "describe('user service', () => {\n  it('works', () => {\n    expect(true).toBe(true);\n  });\n});\n",
        );

        let files = files_for_scope(&root, None, is_test_file);
        let mut issues = Vec::new();
        let count = detect_stub_tests(&root, &files, &mut issues);

        assert_eq!(count, 1);
        assert_eq!(issues[0].rule_id, "completeness:stub-test");
        assert_eq!(issues[0].line, Some(3));
        cleanup(&root);
    }

    #[test]
    fn detects_coverage_exclusions_for_core_layers() {
        let root = temp_dir();
        write(
            &root,
            "jest.config.js",
            "module.exports = {\n  collectCoverage: true,\n  coveragePathIgnorePatterns: ['/src/services/', '/node_modules/'],\n};\n",
        );

        let files = files_for_scope(&root, None, is_coverage_config_file);
        let mut issues = Vec::new();
        let count = detect_suspicious_coverage_exclusions(&root, &files, &mut issues);

        assert_eq!(count, 1);
        assert_eq!(issues[0].rule_id, "completeness:coverage-core-exclusion");
        assert_eq!(issues[0].line, Some(3));
        cleanup(&root);
    }
}
