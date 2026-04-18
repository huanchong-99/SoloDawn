//! Completeness Provider
//!
//! Detects structural completeness issues that compile/lint/test checks miss:
//! skeleton services, missing test files, migration debris, and TODO-saturated code.

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

fn is_config_or_source(p: &Path) -> bool {
    let ext = match p.extension().and_then(|e| e.to_str()) {
        Some(e) => e,
        None => return false,
    };
    matches!(
        ext,
        "rs" | "ts"
            | "tsx"
            | "js"
            | "jsx"
            | "go"
            | "py"
            | "json"
            | "yaml"
            | "yml"
            | "toml"
            | "lock"
    )
}

fn regex_once<'a>(cell: &'a OnceLock<Regex>, pattern: &str) -> &'a Regex {
    cell.get_or_init(|| Regex::new(pattern).unwrap())
}

fn route_handler_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    regex_once(
        &RE,
        r#"(?i)(?:router|app|server)\s*\.\s*(?:get|post|put|patch|delete|use|all)\s*\(\s*["'/]"#,
    )
}

fn health_route_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    regex_once(
        &RE,
        r#"(?i)["']/(?:health|healthz|ping|ready|readyz|liveness|status)["']"#,
    )
}

fn rust_handler_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    regex_once(&RE, r#"#\[(?:get|post|put|patch|delete|handler)\s*\("#)
}

fn todo_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    regex_once(&RE, r"(?i)\b(?:TODO|FIXME|HACK|STUB|XXX)\b")
}

fn test_file_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    regex_once(
        &RE,
        r"(?i)(?:\.(?:test|spec)\.[jt]sx?$|_test\.(?:rs|go)$)",
    )
}

/// Entry-point files that typically define service routes.
static SERVICE_ENTRY_NAMES: &[&str] = &[
    "index.ts",
    "index.js",
    "app.ts",
    "app.js",
    "server.ts",
    "server.js",
    "main.rs",
    "main.go",
];

#[async_trait]
impl QualityProvider for CompletenessProvider {
    fn name(&self) -> &str {
        "completeness"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::SkeletonServiceCount,
            MetricKey::TestFileAbsence,
            MetricKey::MigrationDebrisFiles,
            MetricKey::TodoDensity,
        ]
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

        let skeleton_count = detect_skeleton_services(project_root, &source_files, &mut issues);
        let test_absence = detect_test_file_absence(project_root, &source_files, &mut issues);
        let debris_count = detect_migration_debris(project_root, &mut issues);
        let todo_pct = compute_todo_density(project_root, &source_files, &mut issues);

        let duration_ms = start.elapsed().as_millis() as u64;
        debug!(
            "completeness: skeleton={skeleton_count} test_absence={test_absence} \
             debris={debris_count} todo_density={todo_pct:.1}% in {duration_ms}ms"
        );

        let report = ProviderReport::success("completeness", duration_ms)
            .with_metric(
                MetricKey::SkeletonServiceCount,
                MeasureValue::Int(skeleton_count),
            )
            .with_metric(
                MetricKey::TestFileAbsence,
                MeasureValue::Int(test_absence),
            )
            .with_metric(
                MetricKey::MigrationDebrisFiles,
                MeasureValue::Int(debris_count),
            )
            .with_metric(MetricKey::TodoDensity, MeasureValue::Float(todo_pct))
            .with_issues(issues);

        Ok(report)
    }
}

/// Detect services that are skeleton shells (only health endpoints + TODO markers).
fn detect_skeleton_services(
    project_root: &Path,
    source_files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) -> i64 {
    if source_files.len() < 5 {
        return 0;
    }

    let entry_files: Vec<&PathBuf> = source_files
        .iter()
        .filter(|f| {
            let name = f.file_name().and_then(|n| n.to_str()).unwrap_or("");
            SERVICE_ENTRY_NAMES.contains(&name)
        })
        .collect();

    let mut skeleton_count = 0i64;

    for entry in &entry_files {
        let content = match std::fs::read_to_string(entry) {
            Ok(c) => c,
            Err(_) => continue,
        };

        let total_routes = route_handler_re().find_iter(&content).count()
            + rust_handler_re().find_iter(&content).count();
        let health_routes = health_route_re().find_iter(&content).count();
        let non_health_routes = total_routes.saturating_sub(health_routes);
        let has_todo = todo_re().is_match(&content);

        if non_health_routes == 0 && has_todo {
            skeleton_count += 1;
            let rel = entry
                .strip_prefix(project_root)
                .unwrap_or(entry)
                .to_string_lossy();
            let mut issue = QualityIssue::new(
                "completeness:skeleton-service",
                RuleType::Bug,
                Severity::Blocker,
                AnalyzerSource::Other("completeness".into()),
                format!(
                    "Skeleton service: {rel} has no business route handlers \
                     (only health endpoints) and contains TODO markers"
                ),
            );
            issue.file_path = Some(rel.into_owned());
            issues.push(issue);
        }
    }

    skeleton_count
}

/// Detect absence of test files in a project with source code.
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

/// Detect migration debris (old-stack files left behind after migration).
fn detect_migration_debris(
    project_root: &Path,
    issues: &mut Vec<QualityIssue>,
) -> i64 {
    let has_cargo = project_root.join("Cargo.toml").exists();
    let has_package_json = project_root.join("package.json").exists();
    let has_tsconfig = project_root.join("tsconfig.json").exists();
    let has_go_mod = project_root.join("go.mod").exists();

    let all_files = analysis::collect_files(project_root, is_config_or_source);
    let mut debris_count = 0i64;

    // Rust project with leftover JS/TS source in src/
    if has_cargo && !has_tsconfig {
        let js_in_src: Vec<&PathBuf> = all_files
            .iter()
            .filter(|f| {
                let rel = f.strip_prefix(project_root).unwrap_or(f);
                let in_src = rel.starts_with("src");
                let is_js = analysis::is_ts_file(f);
                in_src && is_js
            })
            .collect();

        if !js_in_src.is_empty() {
            debris_count += js_in_src.len() as i64;
            issues.push(QualityIssue::new(
                "completeness:migration-debris",
                RuleType::CodeSmell,
                Severity::Critical,
                AnalyzerSource::Other("completeness".into()),
                format!(
                    "Rust project contains {} JS/TS files in src/ — \
                     leftover from pre-migration code that should be removed",
                    js_in_src.len()
                ),
            ));
        }

        // package.json/yarn.lock in a pure Rust project
        if has_package_json {
            debris_count += 1;
            issues.push(QualityIssue::new(
                "completeness:migration-debris",
                RuleType::CodeSmell,
                Severity::Critical,
                AnalyzerSource::Other("completeness".into()),
                "Rust project still has package.json — \
                 leftover Node.js manifest should be removed"
                    .to_string(),
            ));
        }
        for lock_file in &["yarn.lock", "package-lock.json", "pnpm-lock.yaml"] {
            if project_root.join(lock_file).exists() {
                debris_count += 1;
            }
        }
    }

    // TS/JS project with leftover Rust source in src/
    if (has_tsconfig || has_package_json) && !has_cargo && !has_go_mod {
        let rs_in_src: Vec<&PathBuf> = all_files
            .iter()
            .filter(|f| {
                let rel = f.strip_prefix(project_root).unwrap_or(f);
                let in_src = rel.starts_with("src");
                analysis::is_rust_file(f) && in_src
            })
            .collect();

        if !rs_in_src.is_empty() {
            debris_count += rs_in_src.len() as i64;
            issues.push(QualityIssue::new(
                "completeness:migration-debris",
                RuleType::CodeSmell,
                Severity::Critical,
                AnalyzerSource::Other("completeness".into()),
                format!(
                    "JS/TS project contains {} Rust files in src/ — \
                     leftover from pre-migration code",
                    rs_in_src.len()
                ),
            ));
        }
    }

    debris_count
}

/// Compute TODO/FIXME density as a percentage of total source lines.
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
    fn detects_skeleton_service() {
        let root = temp_dir();
        // Create 6 source files so threshold is met
        for i in 0..5 {
            write(&root, &format!("src/lib{i}.ts"), "export const x = 1;");
        }
        write(
            &root,
            "src/index.ts",
            r#"
const app = express();
app.get("/health", (req, res) => res.send("ok"));
// TODO: Routes will be registered here
"#,
        );
        let files = analysis::collect_files(&root, is_any_source_file);
        let mut issues = Vec::new();
        let count = detect_skeleton_services(&root, &files, &mut issues);
        assert_eq!(count, 1);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].rule_id.contains("skeleton"));
        cleanup(&root);
    }

    #[test]
    fn no_skeleton_when_real_routes_exist() {
        let root = temp_dir();
        for i in 0..5 {
            write(&root, &format!("src/lib{i}.ts"), "export const x = 1;");
        }
        write(
            &root,
            "src/index.ts",
            r#"
const app = express();
app.get("/health", handler);
app.post("/api/users", createUser);
app.get("/api/products", listProducts);
// TODO: add more routes
"#,
        );
        let files = analysis::collect_files(&root, is_any_source_file);
        let mut issues = Vec::new();
        let count = detect_skeleton_services(&root, &files, &mut issues);
        assert_eq!(count, 0);
        cleanup(&root);
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
    fn detects_migration_debris_js_in_rust() {
        let root = temp_dir();
        write(&root, "Cargo.toml", "[package]\nname = \"app\"");
        write(&root, "package.json", r#"{"name": "old"}"#);
        write(&root, "src/main.rs", "fn main() {}");
        write(&root, "src/api/user.js", "module.exports = {};");
        let mut issues = Vec::new();
        let count = detect_migration_debris(&root, &mut issues);
        assert!(count >= 2); // at least package.json + js file
        cleanup(&root);
    }

    #[test]
    fn computes_todo_density() {
        let root = temp_dir();
        // 10 lines total, 2 TODOs = 20%
        write(
            &root,
            "src/app.ts",
            "line1\nline2\n// TODO: do stuff\nline4\nline5\nline6\nline7\n// FIXME: broken\nline9\nline10\n",
        );
        let files = analysis::collect_files(&root, is_any_source_file);
        let mut issues = Vec::new();
        let density = compute_todo_density(&root, &files, &mut issues);
        assert!((density - 20.0).abs() < 0.1);
        assert!(!issues.is_empty()); // >3% threshold
        cleanup(&root);
    }
}
