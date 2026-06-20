//! The rule-authoring multi-agent validation state-machine (PRD §7.3).
//!
//! Orchestrates the seven named steps as a bounded loop capped at
//! [`MAX_AUTHORING_ROUNDS`] (mirroring `FINAL_REPAIR_MAX_ROUNDS`, D6):
//!
//! 1. GENERATE (proposer) — draft rule + description + positive/negative examples.
//! 2. ADVERSARIAL REVIEW (attacker) — hunt false-positives / evasions / over-reach.
//! 3. EMPIRICAL TEST (deterministic) — compile + run_candidate; AUTHORITATIVE.
//! 4. PRODUCE CANDIDATE — the validated candidate for the mandatory human confirm.
//! 5. CONTEXT-FREE REVERSE-ENGINEER (interpreter) — reconstruct intent from body.
//! 6. JUDGE COMPARE (matcher) — reconstruction vs original → round-trip verdict.
//! 7. LOOP/CAP — at most [`MAX_AUTHORING_ROUNDS`]; on cap return `CappedOut` (no
//!    panic), handed back to the user.
//!
//! Every stage takes `&dyn AuthoringLlm`, so the same pipeline runs unchanged
//! against BOTH invoker backends (metered HTTP / subscription interactive). All
//! LLM stages fail-close; the empirical test is the hard gate. On a usable result
//! (or cap-out) the run is persisted: a `custom_rule` row (status `shadow`), its
//! examples, a `custom_rule_validation` artifact, and a `custom_rule_audit` row
//! per attempt. The human confirm + flip-to-enabled happens at the API layer.

use db::models::{
    CreateCustomRule, CreateCustomRuleAudit, CreateCustomRuleExample,
    CreateCustomRuleValidation, CustomRule,
};
use quality::provider::RuleFormat;
use quality::rule::{RuleType, Severity};
use serde::Serialize;
use sqlx::SqlitePool;
use uuid::Uuid;

use crate::services::rule_authoring::{
    AuthoringLlm, adversary, empirical_test, generate, prompts, reverse_engineer,
    types::{
        AdversaryFindings, AuthorOutcome, AuthorRunResult, AuthoredCandidate, AuthoringBackend,
        EmpiricalReport, GeneratedRule, RoundTripVerdict, RuleExample,
    },
};

/// Hard cap on authoring loop rounds (mirrors `FINAL_REPAIR_MAX_ROUNDS`; D6 —
/// fixed, configurability deferred). On exhaustion the pipeline returns
/// [`AuthorOutcome::CappedOut`] (never panics).
pub const MAX_AUTHORING_ROUNDS: usize = 4;

/// Inputs to one authoring run. `interpreter` is a SEPARATE `&dyn AuthoringLlm`
/// for the context-free step (PRD step 5) so the round-trip is genuinely
/// independent; in production it is a fresh client of the same backend, in tests
/// it is a distinct scripted mock. `matcher` is the judge (step 6). When the
/// caller wants the simple single-backend wiring, [`run_authoring`] uses one
/// client for all roles.
pub struct AuthoringAgents<'a> {
    /// Proposer + adversary (steps 1, 2) and revision (step 7).
    pub generator: &'a dyn AuthoringLlm,
    /// Adversary attacker (step 2). Often the same backend as `generator`.
    pub adversary: &'a dyn AuthoringLlm,
    /// Context-free interpreter (step 5) — a fresh conversation, no history.
    pub interpreter: &'a dyn AuthoringLlm,
    /// Judge/matcher (step 6).
    pub matcher: &'a dyn AuthoringLlm,
}

/// The pipeline's internal per-round bookkeeping (the best of these is handed
/// back on cap-out). The permanent fixture set itself is re-derived for
/// persistence from the candidate's examples plus the debate transcript, so it is
/// not retained here.
struct RoundState {
    candidate: GeneratedRule,
    empirical: EmpiricalReport,
    round_trip: RoundTripVerdict,
}

/// Run the full validation state-machine against a NL request (PRD §7.3).
///
/// Returns an [`AuthorRunResult`] with the best candidate, the outcome
/// (`Passed`/`CappedOut`), the rounds used, and the debate transcript. This does
/// NOT persist — call [`persist_run`] after (the API persists only the parts it
/// needs, on confirm). Never panics on cap-out.
pub async fn author_rule(
    agents: &AuthoringAgents<'_>,
    nl_request: &str,
    current_rules_context: Option<&str>,
    backend: AuthoringBackend,
) -> anyhow::Result<AuthorRunResult> {
    // Step 1: initial draft. A hard error here (not a parse fail-close — the
    // proposer genuinely returned nothing usable across the extraction
    // strategies) aborts the run with a clear message rather than looping on
    // nothing.
    let mut candidate = generate::draft_rule(agents.generator, nl_request, current_rules_context)
        .await
        .map_err(|e| anyhow::anyhow!("rule authoring could not start: {e}"))?;

    let mut debate: Vec<AdversaryFindings> = Vec::new();
    let mut best: Option<RoundState> = None;

    for round in 1..=MAX_AUTHORING_ROUNDS {
        // Step 2: adversarial review (fail-closed inside `attack`).
        let findings = adversary::attack(agents.adversary, nl_request, &candidate).await;

        // Assemble the permanent fixture set: the candidate's own examples plus
        // every adversary snippet seen so far (PRD step 2 — appended fixtures).
        let mut fixtures: Vec<RuleExample> = candidate.examples.clone();
        for prior in &debate {
            fixtures.extend(prior.examples.iter().cloned());
        }
        fixtures.extend(findings.examples.iter().cloned());
        debate.push(findings.clone());

        // Step 3: EMPIRICAL TEST — authoritative. Compile + run every fixture.
        let empirical = empirical_test::evaluate(&candidate, &fixtures);

        // Steps 5+6: context-free reverse-engineer + judge-compare. Only worth
        // running when the rule at least compiled (an uncompilable body has no
        // meaningful reconstruction); a compile failure already forces a re-loop.
        let round_trip = if empirical.compiled {
            reverse_engineer::round_trip(
                agents.interpreter,
                agents.matcher,
                nl_request,
                &candidate.pattern,
                &candidate.description,
            )
            .await
        } else {
            RoundTripVerdict {
                matches: false,
                reason: "rule did not compile; round-trip skipped".to_string(),
                reconstructed_request: String::new(),
                judge_score: 0.0,
            }
        };

        let state = RoundState {
            candidate: candidate.clone(),
            empirical: empirical.clone(),
            round_trip: round_trip.clone(),
        };

        // Step 4/7: a usable candidate requires BOTH the authoritative empirical
        // pass AND a round-trip match. The empirical result overrides judge
        // optimism: even if the judge says "matches", a failing fixture forces a
        // re-loop.
        let empirical_ok = empirical.all_passed();
        let round_trip_ok = round_trip.matches;
        let converged = empirical_ok && round_trip_ok;

        // On convergence, return THIS round's state directly — it is the one that
        // satisfied both gates. (Do not consult `best`: a prior round could have
        // passed empirically yet failed the round-trip, which `pick_better` may
        // have retained; the converged candidate is the correct one to return.)
        if converged {
            return Ok(AuthorRunResult {
                candidate: AuthoredCandidate {
                    rule: state.candidate,
                    empirical: state.empirical,
                    round_trip: state.round_trip,
                },
                outcome: AuthorOutcome::Passed,
                rounds_used: round,
                debate,
                backend,
            });
        }

        // Not converged — fold into the best-so-far for the cap-out hand-back:
        // prefer a state that passed empirically, then the higher judge score.
        best = Some(pick_better(best, state));

        // Step 7: not converged. If rounds remain, revise with the folded fix
        // instructions (adversary critique + empirical failures + round-trip
        // delta) and loop. On the last round, fall through to CappedOut.
        if round < MAX_AUTHORING_ROUNDS {
            let empirical_failures = empirical_test::failure_messages(&empirical);
            let round_trip_delta = if round_trip_ok {
                None
            } else {
                Some(round_trip.reason.as_str())
            };
            let fix = prompts::build_fix_instructions(
                debate.last(),
                &empirical_failures,
                round_trip_delta,
            );
            match generate::revise_rule(agents.generator, nl_request, &candidate, &fix).await {
                Ok(revised) => candidate = revised,
                Err(e) => {
                    // The reviser produced nothing parseable; keep the current
                    // candidate for the next attempt rather than aborting. If
                    // this was the penultimate round we will cap out cleanly.
                    tracing::warn!(error = %e, round, "rule revision failed; retaining prior candidate");
                }
            }
        }
    }

    // Cap reached without convergence (D6): hand back the best candidate with all
    // transcripts. NO panic.
    let state = best.expect("at least one round ran");
    Ok(AuthorRunResult {
        candidate: AuthoredCandidate {
            rule: state.candidate,
            empirical: state.empirical,
            round_trip: state.round_trip,
        },
        outcome: AuthorOutcome::CappedOut,
        rounds_used: MAX_AUTHORING_ROUNDS,
        debate,
        backend,
    })
}

/// Convenience wiring when a single backend client plays every role (the common
/// production path: the same metered/subscription client is reused for proposer,
/// adversary, interpreter, and matcher — the round-trip independence comes from a
/// fresh message vector, not a different transport).
pub async fn run_authoring(
    llm: &dyn AuthoringLlm,
    nl_request: &str,
    current_rules_context: Option<&str>,
    backend: AuthoringBackend,
) -> anyhow::Result<AuthorRunResult> {
    let agents = AuthoringAgents {
        generator: llm,
        adversary: llm,
        interpreter: llm,
        matcher: llm,
    };
    author_rule(&agents, nl_request, current_rules_context, backend).await
}

/// Prefer the better of two round states for the cap-out hand-back. Ordering:
/// (1) a state that passed empirically beats one that did not; (2) among equals,
/// the higher judge score wins; (3) ties keep the newer state.
fn pick_better(current: Option<RoundState>, candidate: RoundState) -> RoundState {
    match current {
        None => candidate,
        Some(cur) => {
            let cur_emp = cur.empirical.all_passed();
            let cand_emp = candidate.empirical.all_passed();
            if cand_emp != cur_emp {
                return if cand_emp { candidate } else { cur };
            }
            if candidate.round_trip.judge_score >= cur.round_trip.judge_score {
                candidate
            } else {
                cur
            }
        }
    }
}

/// Per-example empirical result serialized into `custom_rule_validation.results_json`.
#[derive(Debug, Serialize)]
struct PersistedResults<'a> {
    outcome: AuthorOutcome,
    rounds_used: usize,
    empirical: &'a EmpiricalReport,
    round_trip: &'a RoundTripVerdict,
    debate: &'a [AdversaryFindings],
}

/// DB token for a [`RuleFormat`] (`custom_rule.rule_format` CHECK).
fn rule_format_token(format: RuleFormat) -> &'static str {
    match format {
        RuleFormat::Regex => "regex",
        RuleFormat::AstGrep => "ast_grep",
    }
}

/// DB token for a [`RuleType`] — the PascalCase the `custom_rule.rule_type` CHECK
/// requires (NOT `RuleType::as_str()`, which emits SonarQube `CODE_SMELL`).
fn rule_type_token(rule_type: RuleType) -> &'static str {
    match rule_type {
        RuleType::Bug => "Bug",
        RuleType::Vulnerability => "Vulnerability",
        RuleType::CodeSmell => "CodeSmell",
        RuleType::SecurityHotspot => "SecurityHotspot",
    }
}

/// DB token for a [`Severity`] — `Severity::as_str()` already emits the uppercase
/// the `custom_rule.severity` CHECK requires (`MAJOR`, …).
fn severity_token(severity: Severity) -> &'static str {
    severity.as_str()
}

/// Persist a completed authoring run (PRD §7.3 step 4/7 persistence): create the
/// `custom_rule` row at status `shadow`/disabled-pending, its examples, the
/// validation artifact, and an audit row. Returns the created [`CustomRule`].
///
/// This is invoked by the API on human confirm and by the revalidation entry
/// point; `author_rule` itself is side-effect-free so the UI can preview before
/// anything is written.
pub async fn persist_run(
    pool: &SqlitePool,
    project_id: Option<Uuid>,
    nl_request: &str,
    created_by: Option<&str>,
    result: &AuthorRunResult,
) -> anyhow::Result<CustomRule> {
    let rule = &result.candidate.rule;

    // 1. Create the rule row (status defaults to `shadow`, version 1).
    let created = CustomRule::create(
        pool,
        &CreateCustomRule {
            project_id,
            name: rule.name.clone(),
            nl_request: nl_request.to_string(),
            rule_format: rule_format_token(rule.rule_format).to_string(),
            rule_body: rule.pattern.clone(),
            description: Some(rule.description.clone()),
            rule_type: rule_type_token(rule.rule_type).to_string(),
            severity: severity_token(rule.severity).to_string(),
            mapped_metric: rule.mapped_metric.clone(),
            created_by: created_by.map(str::to_string),
        },
    )
    .await?;

    persist_children(pool, &created, project_id, created_by, result).await?;
    Ok(created)
}

/// Persist the examples + validation + audit children for an existing rule row
/// (shared by `persist_run` and the revalidation entry point).
async fn persist_children(
    pool: &SqlitePool,
    rule: &CustomRule,
    project_id: Option<Uuid>,
    actor: Option<&str>,
    result: &AuthorRunResult,
) -> anyhow::Result<()> {
    // 2. Examples (the permanent fixture set = candidate + adversary snippets).
    let mut fixtures: Vec<RuleExample> = result.candidate.rule.examples.clone();
    for findings in &result.debate {
        fixtures.extend(findings.examples.iter().cloned());
    }
    if !fixtures.is_empty() {
        let example_rows: Vec<CreateCustomRuleExample> = fixtures
            .iter()
            .map(|ex| CreateCustomRuleExample {
                rule_id: rule.id,
                kind: ex.kind.as_db_str().to_string(),
                language: ex.language.clone(),
                snippet: ex.snippet.clone(),
                expected_match: ex.kind.expected_match(),
                note: ex.note.clone(),
            })
            .collect();
        db::models::CustomRuleExample::insert_batch(pool, &example_rows).await?;
    }

    // 3. Validation artifact (authoring-time only — NOT quality_run/quality_issue).
    let empirical = &result.candidate.empirical;
    let round_trip = &result.candidate.round_trip;
    let verdict = match result.outcome {
        AuthorOutcome::Passed => "pass",
        AuthorOutcome::CappedOut => "fail",
    };
    let results_json = serde_json::to_string(&PersistedResults {
        outcome: result.outcome,
        rounds_used: result.rounds_used,
        empirical,
        round_trip,
        debate: &result.debate,
    })
    .ok();

    db::models::CustomRuleValidation::insert(
        pool,
        &CreateCustomRuleValidation {
            rule_id: rule.id,
            rule_version: rule.version,
            verdict: verdict.to_string(),
            roundtrip_ok: Some(round_trip.matches),
            judge_score: Some(round_trip.judge_score),
            examples_total: empirical.total as i64,
            examples_passed: empirical.passed as i64,
            rounds_used: result.rounds_used as i64,
            results_json,
            error_message: empirical.compile_error.clone(),
            validated_by: actor.map(str::to_string),
        },
    )
    .await?;

    // 4. Audit row (append-only; survives rule deletion).
    db::models::CustomRuleAudit::insert(
        pool,
        &CreateCustomRuleAudit {
            rule_id: rule.id,
            project_id,
            action: "create".to_string(),
            actor: actor.map(str::to_string),
            from_version: None,
            to_version: Some(rule.version),
            diff_json: None,
        },
    )
    .await?;

    Ok(())
}

/// D8 revalidation entry point (PRD §7.6).
///
/// Re-runs the full authoring pipeline (steps 1-7) against the rule's stored NL
/// request and drops the rule back to `status='shadow'`, then persists a fresh
/// validation artifact + a `revalidate` audit row. Use this for a rule **body**
/// edit; metadata-only edits (name/description text) skip revalidation and must
/// NOT call this (the caller diffs the submitted input against the persisted row,
/// per PRD §7.6, and only routes a body change here).
///
/// Returns the re-run result so the API can surface the new evidence; the rule's
/// in-DB status is already shadow on return.
pub async fn revalidate_rule_body(
    pool: &SqlitePool,
    agents: &AuthoringAgents<'_>,
    rule_id: Uuid,
    actor: Option<&str>,
    backend: AuthoringBackend,
) -> anyhow::Result<AuthorRunResult> {
    let rule = CustomRule::find_by_id(pool, rule_id)
        .await?
        .ok_or_else(|| anyhow::anyhow!("custom_rule {rule_id} not found for revalidation"))?;

    // D8: a body edit drops the rule back to shadow until it passes again.
    let from_version = rule.version;
    let _ = CustomRule::set_status(pool, rule_id, "shadow").await?;

    // Re-run the full pipeline against the stored NL request.
    let result = author_rule(agents, &rule.nl_request, None, backend).await?;

    // Persist a fresh validation artifact + revalidate audit row against the
    // existing rule id (examples are re-derived from the new run).
    let empirical = &result.candidate.empirical;
    let round_trip = &result.candidate.round_trip;
    let verdict = match result.outcome {
        AuthorOutcome::Passed => "pass",
        AuthorOutcome::CappedOut => "fail",
    };
    let results_json = serde_json::to_string(&PersistedResults {
        outcome: result.outcome,
        rounds_used: result.rounds_used,
        empirical,
        round_trip,
        debate: &result.debate,
    })
    .ok();

    db::models::CustomRuleValidation::insert(
        pool,
        &CreateCustomRuleValidation {
            rule_id,
            rule_version: rule.version,
            verdict: verdict.to_string(),
            roundtrip_ok: Some(round_trip.matches),
            judge_score: Some(round_trip.judge_score),
            examples_total: empirical.total as i64,
            examples_passed: empirical.passed as i64,
            rounds_used: result.rounds_used as i64,
            results_json,
            error_message: empirical.compile_error.clone(),
            validated_by: actor.map(str::to_string),
        },
    )
    .await?;

    db::models::CustomRuleAudit::insert(
        pool,
        &CreateCustomRuleAudit {
            rule_id,
            project_id: rule.project_id,
            action: "revalidate".to_string(),
            actor: actor.map(str::to_string),
            from_version: Some(from_version),
            to_version: Some(rule.version),
            diff_json: None,
        },
    )
    .await?;

    Ok(result)
}
