//! Delivery readiness provider.
//!
//! These checks target high-scoring delivery failures that generic lint/test
//! providers miss: tests wired to fake apps, existing-codebase conventions
//! bypassed, and obvious runtime/security smells.

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
pub struct DeliveryReadinessProvider;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Bucket {
    TestAuthenticity,
    ProjectConvention,
    RuntimeSecurity,
}

fn relevant_file(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .is_some_and(|ext| {
            matches!(
                ext.to_ascii_lowercase().as_str(),
                "js" | "jsx" | "ts" | "tsx" | "vue" | "rs" | "json"
            )
        })
}

fn files_for_scope(project_root: &Path, changed_files: Option<&[String]>) -> Vec<PathBuf> {
    match changed_files {
        Some(files) => files
            .iter()
            .filter_map(|file| changed_file_path(project_root, file))
            .filter(|path| path.is_file() && relevant_file(path))
            .collect(),
        None => analysis::collect_files(project_root, relevant_file),
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

fn rel_path(project_root: &Path, path: &Path) -> String {
    path.strip_prefix(project_root)
        .unwrap_or(path)
        .to_string_lossy()
        .replace('\\', "/")
}

fn read_to_string(path: &Path) -> Option<String> {
    std::fs::read_to_string(path).ok()
}

fn line_number(content: &str, needle: &str) -> u32 {
    content
        .lines()
        .position(|line| line.contains(needle))
        .map(|idx| idx as u32 + 1)
        .unwrap_or(1)
}

fn issue(
    bucket: Bucket,
    rule: &str,
    message: impl Into<String>,
    project_root: &Path,
    path: &Path,
    line: u32,
) -> QualityIssue {
    let (rule_type, prefix) = match bucket {
        Bucket::TestAuthenticity => (RuleType::Bug, "delivery:test-authenticity"),
        Bucket::ProjectConvention => (RuleType::CodeSmell, "delivery:project-convention"),
        Bucket::RuntimeSecurity => (RuleType::Vulnerability, "delivery:runtime-security"),
    };

    QualityIssue::new(
        format!("{prefix}:{rule}"),
        rule_type,
        Severity::Blocker,
        AnalyzerSource::Other("delivery-readiness".to_string()),
        message,
    )
    .with_location(rel_path(project_root, path), line)
    .with_effort(20)
}

fn sqlx_push_user_input_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r"\.push\(\s*(?:&?\w|format!|&format!)").expect("sqlx push regex must compile")
    })
}

fn require_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"\brequire\s*\(").expect("require regex must compile"))
}

#[async_trait]
impl QualityProvider for DeliveryReadinessProvider {
    fn name(&self) -> &str {
        "delivery-readiness"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![
            MetricKey::TestAuthenticityIssues,
            MetricKey::ProjectConventionIssues,
            MetricKey::RuntimeSecuritySmells,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let scoped_files = files_for_scope(project_root, changed_files);
        let all_files = analysis::collect_files(project_root, relevant_file);
        let mut issues = Vec::new();

        detect_mock_only_express_tests(project_root, &scoped_files, &mut issues);
        detect_wrong_package_load_test_coverage(project_root, &all_files, &mut issues);
        detect_esm_require(project_root, &scoped_files, &mut issues);
        detect_i18n_namespace_mismatch(project_root, &all_files, &mut issues);
        detect_duplicate_load_testing_implementation(project_root, &all_files, &mut issues);
        detect_sqlx_query_builder_push(project_root, &scoped_files, &mut issues);
        detect_redis_keys(project_root, &scoped_files, &mut issues);
        detect_csrf_undefined_res(project_root, &scoped_files, &mut issues);
        detect_sql_blacklist_false_positive(project_root, &scoped_files, &mut issues);

        let test_authenticity = issues_for(&issues, "delivery:test-authenticity") as i64;
        let project_convention = issues_for(&issues, "delivery:project-convention") as i64;
        let runtime_security = issues_for(&issues, "delivery:runtime-security") as i64;
        let duration_ms = start.elapsed().as_millis() as u64;

        debug!(
            "delivery-readiness: test_authenticity={test_authenticity} \
             project_convention={project_convention} runtime_security={runtime_security} \
             in {duration_ms}ms"
        );

        Ok(ProviderReport::success("delivery-readiness", duration_ms)
            .with_metric(
                MetricKey::TestAuthenticityIssues,
                MeasureValue::Int(test_authenticity),
            )
            .with_metric(
                MetricKey::ProjectConventionIssues,
                MeasureValue::Int(project_convention),
            )
            .with_metric(
                MetricKey::RuntimeSecuritySmells,
                MeasureValue::Int(runtime_security),
            )
            .with_issues(issues))
    }
}

fn issues_for(issues: &[QualityIssue], prefix: &str) -> usize {
    issues
        .iter()
        .filter(|issue| issue.rule_id.starts_with(prefix))
        .count()
}

fn detect_mock_only_express_tests(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    for file in files {
        let rel = rel_path(project_root, file);
        if !rel.contains("test.helper.") {
            continue;
        }
        let Some(content) = read_to_string(file) else {
            continue;
        };
        if content.contains("express()") && !content.contains("require('../../server") {
            issues.push(issue(
                Bucket::TestAuthenticity,
                "mock-express-app",
                "Tests define a standalone Express app instead of importing the production server entry.",
                project_root,
                file,
                line_number(&content, "express()"),
            ));
        }
    }
}

fn detect_wrong_package_load_test_coverage(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    let has_common_load_testing = project_root
        .join("packages")
        .join("hoppscotch-common")
        .join("src")
        .join("load-testing")
        .is_dir();
    if !has_common_load_testing {
        return;
    }

    for file in files {
        let rel = rel_path(project_root, file);
        if rel.contains("packages/hoppscotch-app/src/load-testing") && rel.contains("__tests__") {
            issues.push(issue(
                Bucket::TestAuthenticity,
                "wrong-package-load-testing-tests",
                "Load-testing tests live under hoppscotch-app while the real implementation is under hoppscotch-common.",
                project_root,
                file,
                1,
            ));
            return;
        }
    }
}

fn detect_esm_require(project_root: &Path, files: &[PathBuf], issues: &mut Vec<QualityIssue>) {
    for file in files {
        if !is_js_like(file) {
            continue;
        }
        if !nearest_package_is_module(file) {
            continue;
        }
        let Some(content) = read_to_string(file) else {
            continue;
        };
        if require_re().is_match(&content) {
            issues.push(issue(
                Bucket::ProjectConvention,
                "esm-require",
                "CommonJS require() is used inside an ESM package; use import or dynamic import instead.",
                project_root,
                file,
                line_number(&content, "require("),
            ));
        }
    }
}

fn detect_i18n_namespace_mismatch(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    let source_with_load_test = files.iter().find_map(|file| {
        let content = read_to_string(file)?;
        if content.contains("load_test.") {
            Some((file, content))
        } else {
            None
        }
    });
    if source_with_load_test.is_none() {
        return;
    }

    let has_load_testing_locale = files.iter().any(|file| {
        rel_path(project_root, file).contains("locales/")
            && read_to_string(file).is_some_and(|content| content.contains("load_testing"))
    });
    if !has_load_testing_locale {
        return;
    }

    let (file, content) = source_with_load_test.expect("checked above");
    issues.push(issue(
        Bucket::ProjectConvention,
        "i18n-namespace-mismatch",
        "UI uses load_test.* translation keys while locale files define load_testing.* keys.",
        project_root,
        file,
        line_number(&content, "load_test."),
    ));
}

fn detect_duplicate_load_testing_implementation(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    let has_common = project_root
        .join("packages/hoppscotch-common/src/load-testing")
        .is_dir();
    let has_app = project_root
        .join("packages/hoppscotch-app/src/load-testing")
        .is_dir();
    if !has_common || !has_app {
        return;
    }

    let duplicate = files.iter().find(|file| {
        let rel = rel_path(project_root, file);
        rel.contains("packages/hoppscotch-app/src/load-testing")
            && matches!(
                file.file_name().and_then(|name| name.to_str()),
                Some("engine.ts" | "store.ts" | "types.ts")
            )
    });
    if let Some(file) = duplicate {
        issues.push(issue(
            Bucket::ProjectConvention,
            "duplicate-load-testing-implementation",
            "Load-testing engine/store/types are duplicated across hoppscotch-app and hoppscotch-common instead of reusing the package implementation.",
            project_root,
            file,
            1,
        ));
    }
}

fn detect_sqlx_query_builder_push(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    for file in files {
        if file.extension().and_then(|ext| ext.to_str()) != Some("rs") {
            continue;
        }
        let Some(content) = read_to_string(file) else {
            continue;
        };
        if !content.contains("QueryBuilder") {
            continue;
        }
        for (idx, line) in content.lines().enumerate() {
            if sqlx_push_user_input_re().is_match(line) {
                issues.push(issue(
                    Bucket::RuntimeSecurity,
                    "sqlx-querybuilder-push",
                    "SQLx QueryBuilder.push() appears to append a variable expression; use push_bind() for user-controlled values.",
                    project_root,
                    file,
                    idx as u32 + 1,
                ));
                break;
            }
        }
    }
}

fn detect_redis_keys(project_root: &Path, files: &[PathBuf], issues: &mut Vec<QualityIssue>) {
    for file in files {
        if !is_js_like(file) {
            continue;
        }
        let Some(content) = read_to_string(file) else {
            continue;
        };
        for (idx, line) in content.lines().enumerate() {
            if line.contains(".keys(") || line.contains(" KEYS ") {
                issues.push(issue(
                    Bucket::RuntimeSecurity,
                    "redis-keys",
                    "Redis KEYS is blocking on large datasets; use SCAN/scanIterator instead.",
                    project_root,
                    file,
                    idx as u32 + 1,
                ));
                break;
            }
        }
    }
}

fn detect_csrf_undefined_res(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    for file in files {
        if !is_js_like(file) {
            continue;
        }
        let Some(content) = read_to_string(file) else {
            continue;
        };
        if !content.contains("getCSRFToken(req") {
            continue;
        }
        if content.contains("res.cookie") || content.contains("res.") {
            issues.push(issue(
                Bucket::RuntimeSecurity,
                "csrf-undefined-res",
                "getCSRFToken(req) references res without accepting it as a parameter, causing a runtime ReferenceError.",
                project_root,
                file,
                line_number(&content, "getCSRFToken(req"),
            ));
        }
    }
}

fn detect_sql_blacklist_false_positive(
    project_root: &Path,
    files: &[PathBuf],
    issues: &mut Vec<QualityIssue>,
) {
    for file in files {
        let rel = rel_path(project_root, file);
        if !rel.contains("input-validation") {
            continue;
        }
        let Some(content) = read_to_string(file) else {
            continue;
        };
        let lower = content.to_ascii_lowercase();
        if lower.contains("select")
            && lower.contains("drop")
            && (lower.contains("blacklist") || lower.contains("dangerous"))
        {
            issues.push(issue(
                Bucket::RuntimeSecurity,
                "sql-keyword-blacklist",
                "Input validation appears to block SQL keywords globally; this causes false positives and should be replaced with contextual validation/parameterization.",
                project_root,
                file,
                line_number(&content, "SELECT").max(line_number(&content, "select")),
            ));
        }
    }
}

fn is_js_like(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .is_some_and(|ext| {
            matches!(
                ext.to_ascii_lowercase().as_str(),
                "js" | "jsx" | "ts" | "tsx" | "vue"
            )
        })
}

fn nearest_package_is_module(path: &Path) -> bool {
    let mut dir = path.parent();
    while let Some(current) = dir {
        let package_json = current.join("package.json");
        if let Some(content) = read_to_string(&package_json) {
            return content.contains("\"type\"") && content.contains("\"module\"");
        }
        dir = current.parent();
    }
    false
}

#[cfg(test)]
mod tests {
    use std::fs;

    use super::*;

    fn temp_repo(name: &str) -> PathBuf {
        let root = std::env::temp_dir().join(format!(
            "delivery-readiness-{name}-{}",
            uuid::Uuid::new_v4()
        ));
        fs::create_dir_all(&root).unwrap();
        root
    }

    fn write(root: &Path, rel: &str, content: &str) -> PathBuf {
        let path = root.join(rel);
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(&path, content).unwrap();
        path
    }

    #[test]
    fn detects_mock_only_express_test_helper() {
        let root = temp_repo("mock");
        let file = write(
            &root,
            "server/tests/helpers/test.helper.js",
            "const express = require('express');\nconst app = express();\nmodule.exports = app;\n",
        );
        let mut issues = Vec::new();
        detect_mock_only_express_tests(&root, &[file], &mut issues);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].rule_id.contains("mock-express-app"));
        fs::remove_dir_all(root).ok();
    }

    #[test]
    fn detects_esm_require_in_module_package() {
        let root = temp_repo("esm");
        write(&root, "packages/app/package.json", r#"{"type":"module"}"#);
        let file = write(
            &root,
            "packages/app/src/store.ts",
            "const fs = require('fs');\n",
        );
        let mut issues = Vec::new();
        detect_esm_require(&root, &[file], &mut issues);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].rule_id.contains("esm-require"));
        fs::remove_dir_all(root).ok();
    }

    #[test]
    fn detects_sqlx_querybuilder_push_variable() {
        let root = temp_repo("sqlx");
        let file = write(
            &root,
            "src/handlers/user.rs",
            "use sqlx::QueryBuilder;\nfn f(sort: String) { qb.push(sort); }\n",
        );
        let mut issues = Vec::new();
        detect_sqlx_query_builder_push(&root, &[file], &mut issues);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].rule_id.contains("sqlx-querybuilder-push"));
        fs::remove_dir_all(root).ok();
    }

    #[test]
    fn detects_redis_keys_and_csrf_runtime_smells() {
        let root = temp_repo("runtime");
        let redis = write(
            &root,
            "server/cache/urlCache.js",
            "async function clear(redis) { return redis.keys('url:*'); }\n",
        );
        let csrf = write(
            &root,
            "server/middleware/csrf.js",
            "function getCSRFToken(req) { res.cookie('csrf', 'x'); }\n",
        );

        let mut issues = Vec::new();
        detect_redis_keys(&root, &[redis], &mut issues);
        detect_csrf_undefined_res(&root, &[csrf], &mut issues);

        assert_eq!(issues.len(), 2);
        assert!(
            issues
                .iter()
                .any(|issue| issue.rule_id.contains("redis-keys"))
        );
        assert!(
            issues
                .iter()
                .any(|issue| issue.rule_id.contains("csrf-undefined-res"))
        );
        fs::remove_dir_all(root).ok();
    }

    #[test]
    fn detects_hoppscotch_load_testing_mismatch() {
        let root = temp_repo("hoppscotch");
        write(
            &root,
            "packages/hoppscotch-common/src/load-testing/engine.ts",
            "export const real = true;\n",
        );
        let test = write(
            &root,
            "packages/hoppscotch-app/src/load-testing/__tests__/engine.test.ts",
            "test('x', () => {});\n",
        );
        let mut issues = Vec::new();
        detect_wrong_package_load_test_coverage(&root, &[test], &mut issues);
        assert_eq!(issues.len(), 1);
        assert!(issues[0].rule_id.contains("wrong-package"));
        fs::remove_dir_all(root).ok();
    }
}
