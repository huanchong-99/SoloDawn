//! Typed payloads exchanged across the rule-authoring state-machine.
//!
//! These are the structs the multi-agent pipeline (PRD §7.3) parses out of LLM
//! output, threads between stages, persists, and finally hands back to the API /
//! UI for the **mandatory** human-confirm dialog (PRD §7.3 step 5, D2).
//!
//! Serde uses `camelCase` so the same shapes can be surfaced over the HTTP API
//! (`AuthorRuleResult`, PRD §10) without a second DTO layer. Severity / rule-type
//! reuse the quality crate's enums verbatim, so a generated candidate compiles
//! through [`quality::compile`] without a translation step.

use quality::provider::{RuleDefinition, RuleFormat};
use quality::rule::{RuleType, Severity};
use serde::{Deserialize, Serialize};
use ts_rs::TS;

/// One positive/negative example snippet the proposer must emit (PRD §7.3 step 1).
///
/// A `positive` example MUST trip the rule; a `negative` example MUST NOT. The
/// empirical test (step 3) is authoritative over any LLM opinion and runs the
/// compiled rule against every one of these.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct RuleExample {
    /// `"positive"` (should flag) or `"negative"` (must not flag).
    pub kind: ExampleKind,
    /// Source language hint (`"rust"`, `"typescript"`, or `None` = agnostic).
    #[serde(default)]
    pub language: Option<String>,
    /// The code snippet to run the candidate rule against.
    pub snippet: String,
    /// Optional human note describing why this example matters.
    #[serde(default)]
    pub note: Option<String>,
}

/// Whether an example is expected to flag.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, TS)]
#[serde(rename_all = "snake_case")]
#[ts(export)]
pub enum ExampleKind {
    /// SHOULD flag (the rule must fire).
    Positive,
    /// MUST NOT flag (a false positive here fails the rule).
    Negative,
}

impl ExampleKind {
    /// The `custom_rule_example.kind` CHECK token.
    pub fn as_db_str(self) -> &'static str {
        match self {
            ExampleKind::Positive => "positive",
            ExampleKind::Negative => "negative",
        }
    }

    /// Whether a rule is expected to fire on an example of this kind.
    pub fn expected_match(self) -> bool {
        matches!(self, ExampleKind::Positive)
    }
}

/// The proposer's drafted rule (PRD §7.3 step 1), parsed from the generator's
/// JSON. It carries everything needed to (a) compile + empirically test the
/// candidate and (b) persist a `custom_rule` row.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GeneratedRule {
    /// Format discriminant; P1 emits [`RuleFormat::Regex`].
    pub rule_format: RuleFormat,
    /// The matcher body — a Rust-`regex` pattern for [`RuleFormat::Regex`].
    pub pattern: String,
    /// Short human-readable rule name.
    pub name: String,
    /// Plain-language description (powers the "!" tooltip post-confirmation).
    pub description: String,
    /// Message attached to every match.
    pub message: String,
    /// Issue type (defaults to `CodeSmell`).
    #[serde(default = "default_rule_type")]
    pub rule_type: RuleType,
    /// Authored severity (capped to `Major` at enforcement via `CustomRule`).
    #[serde(default = "default_severity")]
    pub severity: Severity,
    /// Target languages (informational/provenance).
    #[serde(default)]
    pub languages: Vec<String>,
    /// File extensions (no leading dot) the rule applies to.
    #[serde(default)]
    pub extensions: Vec<String>,
    /// Include globs (empty = all).
    #[serde(default)]
    pub include_globs: Vec<String>,
    /// Exclude globs.
    #[serde(default)]
    pub exclude_globs: Vec<String>,
    /// Optional `MetricKey::as_str()` token this rule's count maps to.
    #[serde(default)]
    pub mapped_metric: Option<String>,
    /// The positive/negative examples (2-3 of each; required).
    #[serde(default)]
    pub examples: Vec<RuleExample>,
}

fn default_rule_type() -> RuleType {
    RuleType::CodeSmell
}

fn default_severity() -> Severity {
    Severity::Major
}

impl GeneratedRule {
    /// Build the DB-free [`RuleDefinition`] the quality crate compiles + runs.
    /// `rule_id` is the stable id (a stringified `custom_rule.id` in production, a
    /// synthetic id during the pre-persist empirical test).
    pub fn to_rule_definition(&self, rule_id: &str) -> RuleDefinition {
        RuleDefinition {
            rule_id: rule_id.to_string(),
            name: self.name.clone(),
            rule_format: self.rule_format,
            pattern: self.pattern.clone(),
            severity: self.severity,
            rule_type: self.rule_type,
            message: self.message.clone(),
            languages: self.languages.clone(),
            extensions: self.extensions.clone(),
            include_globs: self.include_globs.clone(),
            exclude_globs: self.exclude_globs.clone(),
        }
    }

    /// Positive examples only.
    pub fn positives(&self) -> impl Iterator<Item = &RuleExample> {
        self.examples
            .iter()
            .filter(|e| e.kind == ExampleKind::Positive)
    }

    /// Negative examples only.
    pub fn negatives(&self) -> impl Iterator<Item = &RuleExample> {
        self.examples
            .iter()
            .filter(|e| e.kind == ExampleKind::Negative)
    }
}

/// The adversary's findings (PRD §7.3 step 2): an attacker hunting
/// false-positives / over-reach / ambiguity / evasions. Its snippets are appended
/// as permanent fixtures and fed back to the generator on a re-loop.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AdversaryFindings {
    /// Whether the adversary believes the rule is sound (no exploitable gap).
    pub looks_sound: bool,
    /// Free-text critique (over-reach, ambiguity, scope complaints).
    #[serde(default)]
    pub critique: String,
    /// Extra snippets the adversary contributed, appended as permanent fixtures
    /// (false-positives the rule should NOT flag, evasions it SHOULD flag).
    #[serde(default)]
    pub examples: Vec<RuleExample>,
}

/// Per-example empirical result (PRD §7.3 step 3) — the authoritative ground
/// truth that overrides judge optimism.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct ExampleResult {
    /// `"positive"` / `"negative"`.
    pub kind: ExampleKind,
    /// The snippet that was run.
    pub snippet: String,
    /// Whether the rule was expected to fire.
    pub expected_match: bool,
    /// Whether the rule actually fired (match count > 0).
    pub actual_match: bool,
    /// Number of matches produced.
    pub match_count: usize,
    /// Whether this example passed (`expected_match == actual_match`).
    pub passed: bool,
}

/// The full empirical report over every example for one candidate.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct EmpiricalReport {
    /// Did the candidate compile at all?
    pub compiled: bool,
    /// Compile error, if compilation failed (forces a re-loop).
    #[serde(default)]
    pub compile_error: Option<String>,
    /// One result per example.
    pub per_example: Vec<ExampleResult>,
    /// Total examples run.
    pub total: usize,
    /// Examples that passed.
    pub passed: usize,
}

impl EmpiricalReport {
    /// A compile-failure report (no examples run). Forces a re-loop.
    pub fn compile_failed(error: String) -> Self {
        Self {
            compiled: false,
            compile_error: Some(error),
            per_example: Vec::new(),
            total: 0,
            passed: 0,
        }
    }

    /// Whether every example passed AND the rule compiled. Any empirical failure
    /// is authoritative and forces a re-loop (PRD §7.3 step 3).
    pub fn all_passed(&self) -> bool {
        self.compiled && self.total > 0 && self.passed == self.total
    }
}

/// The judge-compare verdict (PRD §7.3 step 6) on the context-free
/// reverse-engineered intent vs the original NL request. A mismatch forces a
/// re-loop within the cap.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct RoundTripVerdict {
    /// Whether the reconstructed intent matches the original request.
    pub matches: bool,
    /// Human-readable rationale for the verdict.
    pub reason: String,
    /// The context-free interpreter's reconstruction (step 5 output).
    pub reconstructed_request: String,
    /// The judge's `AuditScoreResult`-style total (0-100).
    pub judge_score: f64,
}

/// Terminal outcome of the authoring loop (PRD §7.3 step 7).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum AuthorOutcome {
    /// A validated candidate converged within the cap.
    Passed,
    /// The loop exhausted `MAX_AUTHORING_ROUNDS` without converging. The best
    /// candidate + transcripts are still returned; nothing panics (D6).
    CappedOut,
}

/// Which invoker backend ran the authoring turns (echoed back per PRD §10).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, TS)]
#[serde(rename_all = "snake_case")]
#[ts(export)]
pub enum AuthoringBackend {
    /// Metered HTTP via the user's own key (pool-exempt).
    Metered,
    /// Subscription native-OAuth interactive transport (off the credit pool).
    Subscription,
}

/// The validated candidate the pipeline produces (PRD §7.3 step 4) — handed to
/// the API/UI for the mandatory human-confirm dialog. The human confirm +
/// flip-to-enabled happens at the API layer, NOT here.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthoredCandidate {
    /// The drafted rule (body + description + examples + severity + metric).
    pub rule: GeneratedRule,
    /// The empirical evidence for the final candidate (authoritative).
    pub empirical: EmpiricalReport,
    /// The round-trip verdict for the final candidate.
    pub round_trip: RoundTripVerdict,
}

/// The complete result of one authoring run (or revalidation). This is the
/// pipeline's public return value; the API maps it 1:1 to `AuthorRuleResult`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorRunResult {
    /// The best validated candidate (always present, even on `CappedOut`, so the
    /// UI can prefill "edit manually").
    pub candidate: AuthoredCandidate,
    /// `passed` or `capped_out`.
    pub outcome: AuthorOutcome,
    /// How many loop rounds were consumed.
    pub rounds_used: usize,
    /// The adversary's findings from each round (the debate transcript).
    pub debate: Vec<AdversaryFindings>,
    /// Which backend ran the turns (informational).
    pub backend: AuthoringBackend,
}

impl AuthorRunResult {
    /// Whether the run converged to a usable rule.
    pub fn is_usable(&self) -> bool {
        self.outcome == AuthorOutcome::Passed
    }
}
