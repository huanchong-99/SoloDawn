//! Step 3 — EMPIRICAL TEST (deterministic, no LLM) (PRD §7.3).
//!
//! The authoritative ground truth that overrides judge optimism. Compiles the
//! candidate via [`quality::compile`]; on a compile error returns a
//! compile-failed report (the caller feeds it back and re-loops). Otherwise runs
//! [`quality::run_candidate`] against every example: a positive MUST flag, a
//! negative MUST NOT. Any empirical failure forces a re-loop. This module is pure
//! and side-effect-free — no filesystem, no process, no LLM.

use quality::provider::{CompiledRule, compile, run_candidate};

use crate::services::rule_authoring::types::{
    EmpiricalReport, ExampleKind, ExampleResult, GeneratedRule, RuleExample,
};

/// Synthetic rule id used for the pre-persist empirical test (the real
/// `custom_rule.id` is only minted on human confirm).
const CANDIDATE_RULE_ID: &str = "authoring-candidate";

/// Pick a virtual path whose extension matches the example's declared language,
/// so a rule scoped to `extensions: ["rs"]` is exercised correctly. Falls back to
/// `.rs` for an unknown/agnostic language (the most common author target).
fn virtual_path_for(example: &RuleExample) -> &'static str {
    match example.language.as_deref().map(str::to_ascii_lowercase) {
        Some(ref l) if l == "typescript" || l == "ts" => "snippet.ts",
        Some(ref l) if l == "javascript" || l == "js" => "snippet.js",
        Some(ref l) if l == "tsx" => "snippet.tsx",
        Some(ref l) if l == "rust" || l == "rs" => "snippet.rs",
        _ => "snippet.rs",
    }
}

/// Run one example through a compiled rule and classify the result.
fn run_one(compiled: &CompiledRule, example: &RuleExample) -> ExampleResult {
    let virtual_path = virtual_path_for(example);
    let matches = run_candidate(compiled, &example.snippet, virtual_path);
    let match_count = matches.len();
    let actual_match = match_count > 0;
    let expected_match = example.kind.expected_match();
    ExampleResult {
        kind: example.kind,
        snippet: example.snippet.clone(),
        expected_match,
        actual_match,
        match_count,
        passed: actual_match == expected_match,
    }
}

/// Empirically evaluate a candidate against a combined example set (the
/// proposer's examples plus any adversary-contributed fixtures). The caller is
/// responsible for assembling the combined slice so adversary fixtures persist
/// across rounds (PRD §7.3 step 2/3).
pub fn evaluate(candidate: &GeneratedRule, examples: &[RuleExample]) -> EmpiricalReport {
    let def = candidate.to_rule_definition(CANDIDATE_RULE_ID);
    let compiled = match compile(&def) {
        Ok(c) => c,
        Err(e) => return EmpiricalReport::compile_failed(e.to_string()),
    };

    let per_example: Vec<ExampleResult> =
        examples.iter().map(|ex| run_one(&compiled, ex)).collect();
    let total = per_example.len();
    let passed = per_example.iter().filter(|r| r.passed).count();

    EmpiricalReport {
        compiled: true,
        compile_error: None,
        per_example,
        total,
        passed,
    }
}

/// Build human-readable failure messages from an empirical report — the concrete
/// fix instructions fed back to the proposer on a re-loop. Empty when the report
/// passed.
pub fn failure_messages(report: &EmpiricalReport) -> Vec<String> {
    if let Some(err) = &report.compile_error {
        return vec![format!("the rule pattern failed to compile: {err}")];
    }
    report
        .per_example
        .iter()
        .filter(|r| !r.passed)
        .map(|r| {
            let kind = match r.kind {
                ExampleKind::Positive => "positive (must flag)",
                ExampleKind::Negative => "negative (must NOT flag)",
            };
            format!(
                "{kind} example {} (expected_match={}, actual_match={}, matches={}): {:?}",
                if r.expected_match { "did not flag" } else { "wrongly flagged" },
                r.expected_match,
                r.actual_match,
                r.match_count,
                r.snippet.lines().next().unwrap_or("").trim()
            )
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use quality::provider::RuleFormat;
    use quality::rule::{RuleType, Severity};

    fn candidate(pattern: &str, examples: Vec<RuleExample>) -> GeneratedRule {
        GeneratedRule {
            rule_format: RuleFormat::Regex,
            pattern: pattern.to_string(),
            name: "test".into(),
            description: "desc".into(),
            message: "matched".into(),
            rule_type: RuleType::CodeSmell,
            severity: Severity::Major,
            languages: vec![],
            extensions: vec![],
            include_globs: vec![],
            exclude_globs: vec![],
            mapped_metric: None,
            examples,
        }
    }

    fn pos(snippet: &str) -> RuleExample {
        RuleExample {
            kind: ExampleKind::Positive,
            language: Some("rust".into()),
            snippet: snippet.into(),
            note: None,
        }
    }

    fn neg(snippet: &str) -> RuleExample {
        RuleExample {
            kind: ExampleKind::Negative,
            language: Some("rust".into()),
            snippet: snippet.into(),
            note: None,
        }
    }

    #[test]
    fn all_pass_when_pattern_is_correct() {
        let c = candidate(r"\bdbg!\s*\(", vec![pos("dbg!(x);"), neg("let x = 1;")]);
        let report = evaluate(&c, &c.examples);
        assert!(report.all_passed(), "{report:?}");
        assert_eq!(report.total, 2);
        assert_eq!(report.passed, 2);
    }

    #[test]
    fn negative_false_positive_fails() {
        // Over-broad pattern: flags the negative idiomatic snippet too.
        let c = candidate(r"dbg", vec![pos("dbg!(x);"), neg("let dbgr = 1;")]);
        let report = evaluate(&c, &c.examples);
        assert!(!report.all_passed());
        let msgs = failure_messages(&report);
        assert!(msgs.iter().any(|m| m.contains("negative")), "{msgs:?}");
    }

    #[test]
    fn compile_error_reports_failure() {
        let c = candidate(r"console\.log(", vec![pos("console.log(1)")]);
        let report = evaluate(&c, &c.examples);
        assert!(!report.compiled);
        assert!(report.compile_error.is_some());
        assert!(!report.all_passed());
        assert!(failure_messages(&report)[0].contains("compile"));
    }
}
