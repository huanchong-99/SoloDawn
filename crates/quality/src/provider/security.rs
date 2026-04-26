//! 安全审计 Provider
//!
//! 复用并升级 scripts/audit-security.sh

use std::{
    path::{Path, PathBuf},
    sync::OnceLock,
    time::Instant,
};

use async_trait::async_trait;
use regex::Regex;
use tracing::{debug, warn};

use crate::{
    analysis,
    gate::result::MeasureValue,
    issue::QualityIssue,
    metrics::MetricKey,
    provider::{ProviderReport, QualityProvider},
    rule::{AnalyzerSource, RuleType, Severity},
};

/// 安全审计 Provider
pub struct SecurityProvider;

#[async_trait]
impl QualityProvider for SecurityProvider {
    fn name(&self) -> &str {
        "security"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![MetricKey::SecurityIssues, MetricKey::RedosRisks]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();
        let mut report = ProviderReport::success("security", 0);
        let redos_issues = detect_redos_risks(project_root, changed_files);
        let redos_count = redos_issues.len() as i64;

        // 尝试运行安全审计脚本
        let script_path = project_root.join("scripts/audit-security.sh");
        let audit_issue_count;
        if script_path.exists() {
            debug!("Running security audit script...");
            let output = tokio::process::Command::new("bash")
                .args([script_path.to_str().unwrap_or("scripts/audit-security.sh")])
                .current_dir(project_root)
                // R6 port-leak fix: strip SoloDawn dev ports.
                .env_remove("PORT")
                .env_remove("BACKEND_PORT")
                .env_remove("FRONTEND_PORT")
                .output()
                .await;

            match output {
                Ok(out) => {
                    audit_issue_count = if out.status.success() { 0 } else { 1 };
                }
                Err(e) => {
                    warn!("Security audit failed: {}", e);
                    audit_issue_count = -1;
                }
            }
        } else {
            audit_issue_count = 0;
        }

        let security_issue_count = if audit_issue_count < 0 {
            audit_issue_count
        } else {
            audit_issue_count + redos_count
        };
        report.metrics.insert(
            MetricKey::SecurityIssues,
            MeasureValue::Int(security_issue_count),
        );
        report
            .metrics
            .insert(MetricKey::RedosRisks, MeasureValue::Int(redos_count));
        report.issues.extend(redos_issues);
        report.duration_ms = start.elapsed().as_millis() as u64;
        Ok(report)
    }
}

fn redos_nested_quantifier_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"\((?:\\.|[^)])*[*+](?:\\.|[^)])*\)\s*(?:[*+]|\{)").unwrap())
}

fn detect_redos_risks(project_root: &Path, changed_files: Option<&[String]>) -> Vec<QualityIssue> {
    files_for_scope(project_root, changed_files, analysis::is_ts_file)
        .into_iter()
        .filter_map(|file| detect_redos_risk_in_file(project_root, &file))
        .collect()
}

fn detect_redos_risk_in_file(project_root: &Path, file: &Path) -> Option<QualityIssue> {
    let content = std::fs::read_to_string(file).ok()?;
    let line = content.lines().enumerate().find_map(|(idx, line)| {
        let trimmed = line.trim_start();
        if trimmed.starts_with("//") || trimmed.starts_with('*') {
            return None;
        }
        if redos_nested_quantifier_re().is_match(line) {
            Some((idx + 1) as u32)
        } else {
            None
        }
    })?;

    Some(
        QualityIssue::new_capped(
            "security:redos-risk",
            RuleType::Vulnerability,
            Severity::Blocker,
            AnalyzerSource::SecurityAudit,
            "Potential ReDoS-prone regular expression: nested quantifiers can cause catastrophic backtracking on user input",
        )
        .with_location(rel_path(project_root, file), line),
    )
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
        let dir = std::env::temp_dir().join(format!("security_test_{}", uuid::Uuid::new_v4()));
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

    #[test]
    fn detects_redos_nested_quantifier() {
        let root = temp_dir();
        write(
            &root,
            "src/routes/users.js",
            "const unsafe = /^([a-z]+)+$/;\nmodule.exports = unsafe;\n",
        );

        let issues = detect_redos_risks(&root, None);

        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].rule_id, "security:redos-risk");
        assert_eq!(issues[0].line, Some(1));
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn ignores_safe_regexes() {
        let root = temp_dir();
        write(
            &root,
            "src/routes/users.js",
            "const safe = /^[a-z]+$/;\nmodule.exports = safe;\n",
        );

        let issues = detect_redos_risks(&root, None);

        assert!(issues.is_empty());
        let _ = fs::remove_dir_all(root);
    }
}
