//! Declarative custom-rule provider (LLM-free enforcement).
//!
//! [`DeclarativeRuleProvider`] runs a set of already-compiled, human-confirmed
//! custom rules deterministically and identically every gate run — **no LLM is
//! ever in the scan path**. It is constructed with a `Vec<CompiledRule>` and is
//! therefore **DB-free**: loading rows from `custom_rule` and compiling them into
//! [`CompiledRule`]s happens in `crates/services` (the verified G3 boundary). See
//! PRD `docs/quality/PRD-ai-editable-quality-rules.md` §8.2/8.3.
//!
//! Per match it builds a [`QualityIssue::new_capped`] with
//! [`AnalyzerSource::CustomRule`] (so a rule can never self-escalate past `Major`;
//! D3) and aggregates counts into [`MetricKey::CustomRuleViolations`] and
//! [`MetricKey::CustomRuleCritical`] (the `builtin_rust.rs` publish template). Per
//! the engine's decoupling, issues alone never gate — only the published count
//! metrics do.
//!
//! This module also exposes [`run_candidate`], the pure, side-effect-free
//! ground-truth executor the authoring pipeline uses to empirically test a
//! candidate rule against in-memory snippets (no filesystem).

use std::{
    path::Path,
    time::{Duration, Instant},
};

use async_trait::async_trait;
use tracing::{debug, warn};

use crate::{
    analysis,
    gate::result::MeasureValue,
    issue::QualityIssue,
    metrics::MetricKey,
    provider::{
        compiled_rule::{CompiledRegexRule, CompiledRule},
        ProviderReport, QualityProvider,
    },
    rule::{AnalyzerSource, Severity},
};

/// Provider name, registered alongside the builtin providers.
pub const PROVIDER_NAME: &str = "declarative-rules";

/// Wall-clock budget for one full scan. A scan that exceeds this fails closed
/// (the provider returns a failure report with no metrics → the engine degrades
/// to fail-closed, matching the empty-scan branch). Generous because the work is
/// linear-time regex matching over already-excluded source files.
const SCAN_TIMEOUT: Duration = Duration::from_secs(120);

/// Combined source-file filter: Rust + TS/JS. Concrete per-rule extension/glob
/// scoping is applied later via [`CompiledRule`]'s scope; this just bounds the
/// initial walk to source files (and excludes `node_modules`/`target`/… via
/// [`analysis::is_excluded`], which `collect_files` already honors).
fn is_source_file(p: &Path) -> bool {
    analysis::is_rust_file(p) || analysis::is_ts_file(p)
}

/// Deterministic, LLM-free provider that runs compiled custom rules.
///
/// Constructed with the already-compiled rules (DB-free). When constructed with
/// **zero** rules it is a no-op: [`Self::supported_metrics`] /
/// [`Self::applicable_metrics`] return empty and [`Self::analyze`] returns an
/// `Ok` success report with no metrics — the same benign no-op sentinel the other
/// providers use for an inapplicable scope (it never fabricates a pass/fail).
pub struct DeclarativeRuleProvider {
    rules: Vec<CompiledRule>,
}

impl DeclarativeRuleProvider {
    /// Build a provider from already-compiled rules. Pass an empty `Vec` for the
    /// no-op sentinel behavior. Construction is injectable so the services layer
    /// wires the (DB-loaded, compiled) rules in.
    pub fn new(rules: Vec<CompiledRule>) -> Self {
        Self { rules }
    }

    /// Whether this provider has any rules to run.
    pub fn has_rules(&self) -> bool {
        !self.rules.is_empty()
    }

    /// The synchronous scan body, wrapped by [`Self::analyze`] in a timeout.
    fn scan(&self, project_root: &Path) -> Vec<QualityIssue> {
        let files = analysis::collect_files(project_root, is_source_file);
        debug!(
            "{}: scanning {} source files against {} rule(s)",
            PROVIDER_NAME,
            files.len(),
            self.rules.len()
        );

        let mut all_issues = Vec::new();
        for file_path in &files {
            let rel_path = file_path
                .strip_prefix(project_root)
                .unwrap_or(file_path)
                .to_string_lossy()
                .replace('\\', "/");

            // Any rule scoped to this file needs its content; read once, lazily.
            let any_in_scope = self.rules.iter().any(|rule| match rule {
                CompiledRule::Regex(r) => r.scope.matches_path(&rel_path),
            });
            if !any_in_scope {
                continue;
            }

            let content = match std::fs::read_to_string(file_path) {
                Ok(c) => c,
                Err(e) => {
                    // Non-UTF8 / unreadable files are skipped, not fatal — hostile
                    // scanned input must never break the scan.
                    warn!("{}: failed to read {}: {}", PROVIDER_NAME, rel_path, e);
                    continue;
                }
            };

            for rule in &self.rules {
                match rule {
                    CompiledRule::Regex(r) => {
                        if r.scope.matches_path(&rel_path) {
                            run_regex_rule(r, &content, &rel_path, &mut all_issues);
                        }
                    }
                }
            }
        }

        all_issues
    }
}

/// Run one compiled regex rule over a file's content, pushing a
/// [`QualityIssue`] per match (with byte-offset-derived line/column location).
fn run_regex_rule(
    rule: &CompiledRegexRule,
    content: &str,
    rel_path: &str,
    out: &mut Vec<QualityIssue>,
) {
    for m in rule.regex.find_iter(content) {
        let (line, column) = line_col_for_offset(content, m.start());
        let mut issue = QualityIssue::new_capped(
            rule.meta.rule_id.clone(),
            rule.meta.rule_type,
            rule.meta.severity,
            AnalyzerSource::CustomRule,
            rule.meta.message.clone(),
        )
        .with_location(rel_path.to_string(), line);
        // `column` has no builder setter; set the public field directly. `line`
        // is the primary anchor, `column` a best-effort extra.
        issue.column = Some(column);
        out.push(issue);
    }
}

/// 1-based (line, column) for a byte offset into `content`.
///
/// Byte-offset based and char-boundary-safe: `Match::start()` is always on a char
/// boundary (regex matches never split a UTF-8 codepoint), and we count by lines
/// up to it. Column is the 1-based count of `char`s on the matched line before the
/// offset (so it is correct for multi-byte text, not a raw byte index).
fn line_col_for_offset(content: &str, offset: usize) -> (u32, u32) {
    let offset = offset.min(content.len());
    let before = &content[..offset];
    let line = before.bytes().filter(|&b| b == b'\n').count() as u32 + 1;
    let line_start = before.rfind('\n').map(|i| i + 1).unwrap_or(0);
    let column = content[line_start..offset].chars().count() as u32 + 1;
    (line, column)
}

#[async_trait]
impl QualityProvider for DeclarativeRuleProvider {
    fn name(&self) -> &str {
        PROVIDER_NAME
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        // Empty when no rules are loaded, so an empty rule set never participates
        // in gating and never fail-closes / false-positives an unrelated repo
        // (the `rust_analyzer.rs` empty-when-inapplicable pattern).
        if self.rules.is_empty() {
            return Vec::new();
        }
        vec![
            MetricKey::CustomRuleViolations,
            MetricKey::CustomRuleCritical,
        ]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _discovery: &crate::discovery::RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();

        // Zero rules → benign no-op sentinel: an Ok success report with no
        // metrics. Mirrors RustProvider on a non-Rust repo; does NOT fabricate a
        // pass/fail.
        if self.rules.is_empty() {
            debug!("{}: no rules loaded, skipping scan", PROVIDER_NAME);
            return Ok(ProviderReport::success(
                PROVIDER_NAME,
                start.elapsed().as_millis() as u64,
            ));
        }

        // Wrap the (blocking, CPU-bound) scan in a timeout. On timeout we
        // fail-closed: a failure report carries NO metrics, so the engine's
        // empty-scan / metric-less-failure handling degrades to fail-closed in
        // enforce mode (PRD §8.2) rather than silently passing.
        let root = project_root.to_path_buf();
        let issues = match tokio::time::timeout(
            SCAN_TIMEOUT,
            tokio::task::spawn_blocking({
                // Clone the compiled rules into the blocking task. CompiledRule
                // holds a Regex (cheap Arc-backed clone) + small scope vecs.
                let provider = DeclarativeRuleProvider::new(self.rules.clone());
                move || provider.scan(&root)
            }),
        )
        .await
        {
            Ok(Ok(issues)) => issues,
            Ok(Err(join_err)) => {
                warn!("{}: scan task panicked: {}", PROVIDER_NAME, join_err);
                return Ok(ProviderReport::failure(
                    PROVIDER_NAME,
                    start.elapsed().as_millis() as u64,
                    format!("declarative-rules scan panicked: {join_err}"),
                ));
            }
            Err(_elapsed) => {
                warn!(
                    "{}: scan exceeded {:?}; failing closed",
                    PROVIDER_NAME, SCAN_TIMEOUT
                );
                return Ok(ProviderReport::failure(
                    PROVIDER_NAME,
                    start.elapsed().as_millis() as u64,
                    format!(
                        "declarative-rules scan exceeded {}s timeout",
                        SCAN_TIMEOUT.as_secs()
                    ),
                ));
            }
        };

        let total = issues.len() as i64;
        let critical = issues
            .iter()
            .filter(|i| matches!(i.severity, Severity::Critical | Severity::Blocker))
            .count() as i64;

        let duration_ms = start.elapsed().as_millis() as u64;
        debug!(
            "{}: finished in {}ms — {} violations ({} critical)",
            PROVIDER_NAME, duration_ms, total, critical
        );

        Ok(ProviderReport::success(PROVIDER_NAME, duration_ms)
            .with_metric(MetricKey::CustomRuleViolations, MeasureValue::Int(total))
            .with_metric(MetricKey::CustomRuleCritical, MeasureValue::Int(critical))
            .with_issues(issues))
    }
}

/// Run ONE compiled rule against an in-memory snippet at a virtual path — the
/// deterministic, side-effect-free ground-truth primitive the authoring pipeline
/// uses to empirically test a candidate (NO filesystem, NO process, NO LLM).
///
/// Returns the [`QualityIssue`]s the rule produces (the match count is
/// `result.len()`; each carries its line/column location). Scope filters
/// (extensions/globs) are applied against `virtual_path` exactly as in a real
/// scan, so a rule scoped to `*.ts` will produce zero matches for a `.rs`
/// `virtual_path` even if the pattern would otherwise hit.
pub fn run_candidate(rule: &CompiledRule, snippet: &str, virtual_path: &str) -> Vec<QualityIssue> {
    let rel_path = virtual_path.replace('\\', "/");
    let mut out = Vec::new();
    match rule {
        CompiledRule::Regex(r) => {
            if r.scope.matches_path(&rel_path) {
                run_regex_rule(r, snippet, &rel_path, &mut out);
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use std::fs;

    use uuid::Uuid;

    use super::*;
    use crate::{
        discovery::RepositoryDiscovery,
        provider::compiled_rule::{compile, RuleDefinition, RuleFormat},
        rule::RuleType,
    };

    fn forbid_token_def(rule_id: &str, pattern: &str, exts: &[&str]) -> RuleDefinition {
        RuleDefinition {
            rule_id: rule_id.to_string(),
            name: "forbidden token".to_string(),
            rule_format: RuleFormat::Regex,
            pattern: pattern.to_string(),
            severity: Severity::Major,
            rule_type: RuleType::CodeSmell,
            message: "forbidden token present".to_string(),
            languages: vec![],
            extensions: exts.iter().map(|s| s.to_string()).collect(),
            include_globs: vec![],
            exclude_globs: vec![],
        }
    }

    #[test]
    fn run_candidate_flags_positive_and_skips_negative() {
        let def = forbid_token_def("dbg", r"\bdbg!\s*\(", &[]);
        let compiled = compile(&def).expect("compile");

        let positive = run_candidate(&compiled, "let x = dbg!(value);\n", "src/main.rs");
        assert!(
            !positive.is_empty(),
            "positive snippet must flag at least once"
        );
        assert_eq!(positive[0].source, AnalyzerSource::CustomRule);
        assert_eq!(positive[0].rule_id, "dbg");
        assert_eq!(positive[0].line, Some(1));

        let negative = run_candidate(&compiled, "let x = compute(value);\n", "src/main.rs");
        assert_eq!(negative.len(), 0, "negative snippet must not flag");
    }

    #[test]
    fn run_candidate_reports_correct_line_and_column() {
        let def = forbid_token_def("c", r"console\.log", &[]);
        let compiled = compile(&def).expect("compile");
        // Match is on the third line, after two leading chars of indentation.
        let snippet = "a\nb\n  console.log(x)\n";
        let issues = run_candidate(&compiled, snippet, "src/app.ts");
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].line, Some(3));
        assert_eq!(issues[0].column, Some(3));
    }

    #[test]
    fn run_candidate_honors_extension_scope() {
        // Rule scoped to .ts only; a .rs virtual path must produce zero matches
        // even though the pattern is present.
        let def = forbid_token_def("ts-only", r"console\.log", &["ts"]);
        let compiled = compile(&def).expect("compile");
        assert_eq!(
            run_candidate(&compiled, "console.log(1)", "src/x.rs").len(),
            0
        );
        assert_eq!(
            run_candidate(&compiled, "console.log(1)", "src/x.ts").len(),
            1
        );
    }

    #[test]
    fn run_candidate_is_char_boundary_safe_on_multibyte() {
        // Multibyte text before the match must not panic and column must be a
        // char count, not a byte index.
        let def = forbid_token_def("mb", r"TODO", &[]);
        let compiled = compile(&def).expect("compile");
        let snippet = "日本語 TODO here"; // 3 multibyte chars + space before TODO
        let issues = run_candidate(&compiled, snippet, "src/x.rs");
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].line, Some(1));
        // 3 chars + 1 space => TODO starts at char column 5.
        assert_eq!(issues[0].column, Some(5));
    }

    fn temp_root() -> std::path::PathBuf {
        let root = std::env::temp_dir().join(format!("declarative-provider-{}", Uuid::new_v4()));
        fs::create_dir_all(root.join("src")).expect("temp src dir");
        root
    }

    #[tokio::test]
    async fn provider_counts_violations_over_tempdir() {
        let root = temp_root();
        // One matching file, one non-matching file.
        fs::write(root.join("src").join("bad.rs"), "fn f() { dbg!(1); }\n").expect("bad file");
        fs::write(root.join("src").join("good.rs"), "fn g() -> i32 { 1 }\n").expect("good file");

        let def = forbid_token_def("dbg", r"\bdbg!\s*\(", &["rs"]);
        let compiled = compile(&def).expect("compile");
        let provider = DeclarativeRuleProvider::new(vec![compiled]);

        let discovery = RepositoryDiscovery::discover(&root).expect("discovery");
        let report = provider
            .analyze(&root, &discovery, None)
            .await
            .expect("report");

        assert!(report.success);
        assert_eq!(
            report.metrics.get(&MetricKey::CustomRuleViolations),
            Some(&MeasureValue::Int(1)),
            "exactly one file should match"
        );
        // Severity is capped to Major for CustomRule, so critical count is 0.
        assert_eq!(
            report.metrics.get(&MetricKey::CustomRuleCritical),
            Some(&MeasureValue::Int(0))
        );
        assert_eq!(report.issues.len(), 1);
        assert_eq!(report.issues[0].file_path.as_deref(), Some("src/bad.rs"));

        let _ = fs::remove_dir_all(&root);
    }

    #[tokio::test]
    async fn zero_rules_yields_noop_sentinel() {
        let root = temp_root();
        fs::write(root.join("src").join("any.rs"), "fn f() { dbg!(1); }\n").expect("file");

        let provider = DeclarativeRuleProvider::new(Vec::new());
        assert!(!provider.has_rules());
        // No-op sentinel: no supported/applicable metrics.
        assert!(provider.supported_metrics().is_empty());

        let discovery = RepositoryDiscovery::discover(&root).expect("discovery");
        let report = provider
            .analyze(&root, &discovery, None)
            .await
            .expect("report");

        // Benign no-op: success, no metrics, no issues (does NOT fabricate a
        // pass/fail — same shape as RustProvider on a non-Rust repo).
        assert!(report.success);
        assert!(report.metrics.is_empty());
        assert!(report.issues.is_empty());

        let _ = fs::remove_dir_all(&root);
    }
}
