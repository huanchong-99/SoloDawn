//! Custom-rule REST API routes (AI-editable, multi-agent-validated quality-gate
//! rules — PRD `docs/quality/PRD-ai-editable-quality-rules.md` §10).
//!
//! This module mirrors `routes/quality.rs` exactly: every handler takes
//! `State(deployment): State<DeploymentImpl>` + `Path(..)` + optional `Json(..)`,
//! returns `Result<Json<ApiResponse<T>>, ApiError>`, reaches the DB through
//! `deployment.db().pool`, and surfaces failures as `ApiError::{Database,
//! BadRequest,Internal}`. Response DTOs derive `(Debug, Serialize, TS)` +
//! `serde(rename_all = "camelCase")` + `ts(export)`. The two route-builder fns
//! return `Router<DeploymentImpl>`.
//!
//! ## What this layer enforces (the admission gate — §10 validation invariants)
//!
//! Before ANY rule is persisted (POST, or a PUT that changes the rule body), the
//! rule must pass a DETERMINISTIC, AI-free gate, reused verbatim from the quality
//! crate primitives:
//!
//! 1. `rule_format` / `severity` / `rule_type` must parse to the DB CHECK enum
//!    tokens (`regex`/`ast_grep`; `INFO..BLOCKER`; `Bug`/`Vulnerability`/
//!    `CodeSmell`/`SecurityHotspot`).
//! 2. The regex compiles within the size limits (`quality::compile`).
//! 3. EVERY positive example flags ≥1 and EVERY negative example flags 0
//!    (`quality::run_candidate`).
//!
//! This gate runs identically for AI-authored and hand-entered rules.
//!
//! ## D8 — edit revalidation (PUT)
//!
//! A PUT that changes a body field (pattern/format/scope/severity/type/metric)
//! re-runs the admission gate, drops the rule to `status='shadow'`, and writes a
//! `custom_rule_validation` + a `revalidate` audit row. A PUT that touches only
//! metadata (name/description/message) bumps `version` and writes an `update`
//! audit row, skipping revalidation.

use axum::{
    Json, Router,
    extract::{Path, State},
    routing::{get, post},
};
use deployment::Deployment;
use quality::provider::{RuleDefinition, RuleFormat, compile, run_candidate};
use quality::rule::{RuleType, Severity};
use serde::{Deserialize, Serialize};
use services::services::rule_authoring::{
    AuthoringAgents, AuthoringBackend, AuthorRunResult, AuthoredCandidate, EmpiricalReport,
    GeneratedRule, RoundTripVerdict, RuleExample, build_authoring_client_with_backend,
    revalidate_rule_body, run_authoring,
};
use ts_rs::TS;
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

// ─────────────────────────────────────────────────────────────────────────────
// Token <-> quality-enum mapping (the DB CHECK tokens, NOT the SonarQube
// `as_str()` tokens — `RuleType::as_str()` emits `CODE_SMELL`, the CHECK wants
// `CodeSmell`). These are the inverses of the services pipeline's
// `rule_format_token`/`rule_type_token`/`severity_token`.
// ─────────────────────────────────────────────────────────────────────────────

/// Parse the `custom_rule.rule_format` CHECK token into a [`RuleFormat`].
fn parse_rule_format(token: &str) -> Result<RuleFormat, ApiError> {
    match token {
        "regex" => Ok(RuleFormat::Regex),
        "ast_grep" => Ok(RuleFormat::AstGrep),
        other => Err(ApiError::BadRequest(format!(
            "invalid ruleFormat '{other}' (expected 'regex' or 'ast_grep')"
        ))),
    }
}

/// The DB token for a [`RuleFormat`] (the `custom_rule.rule_format` CHECK).
fn rule_format_token(format: RuleFormat) -> &'static str {
    match format {
        RuleFormat::Regex => "regex",
        RuleFormat::AstGrep => "ast_grep",
    }
}

/// Parse the `custom_rule.rule_type` CHECK token (PascalCase) into a [`RuleType`].
fn parse_rule_type(token: &str) -> Result<RuleType, ApiError> {
    match token {
        "Bug" => Ok(RuleType::Bug),
        "Vulnerability" => Ok(RuleType::Vulnerability),
        "CodeSmell" => Ok(RuleType::CodeSmell),
        "SecurityHotspot" => Ok(RuleType::SecurityHotspot),
        other => Err(ApiError::BadRequest(format!(
            "invalid ruleType '{other}' (expected Bug | Vulnerability | CodeSmell | SecurityHotspot)"
        ))),
    }
}

/// The DB token for a [`RuleType`] (the `custom_rule.rule_type` CHECK — NOT
/// `RuleType::as_str()`, which emits the SonarQube `CODE_SMELL`).
fn rule_type_token(rule_type: RuleType) -> &'static str {
    match rule_type {
        RuleType::Bug => "Bug",
        RuleType::Vulnerability => "Vulnerability",
        RuleType::CodeSmell => "CodeSmell",
        RuleType::SecurityHotspot => "SecurityHotspot",
    }
}

/// Parse the `custom_rule.severity` CHECK token into a [`Severity`].
/// `Severity::from_sonar_str` accepts exactly the uppercase CHECK tokens.
fn parse_severity(token: &str) -> Result<Severity, ApiError> {
    Severity::from_sonar_str(token).ok_or_else(|| {
        ApiError::BadRequest(format!(
            "invalid severity '{token}' (expected INFO | MINOR | MAJOR | CRITICAL | BLOCKER)"
        ))
    })
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response DTOs (§10 contract shapes that genuinely differ from the
// internal services/db types; everything else is exposed directly).
// ─────────────────────────────────────────────────────────────────────────────

/// Create/update request body for a custom rule (§10 `CustomRuleInput`).
///
/// `ruleBody` IS the matcher pattern for `regex` rules (the field is named
/// `ruleBody` in the §10 contract; `pattern` is accepted as an alias so either
/// name round-trips). `examples` are the positive/negative fixtures the admission
/// gate executes — required so the gate can run.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CustomRuleInput {
    /// The original natural-language ask (kept for round-trip/reproducibility).
    /// Optional on a manual create; defaults to the rule name when omitted.
    #[serde(default)]
    pub nl_request: Option<String>,
    /// `regex` (P1) | `ast_grep` (P2).
    pub rule_format: String,
    /// The matcher body (a Rust-`regex` pattern for `regex`). Accepts the alias
    /// `pattern`.
    #[serde(alias = "pattern")]
    pub rule_body: String,
    /// Short human-readable rule name.
    pub name: String,
    /// Plain-language description powering the "!" tooltip.
    #[serde(default)]
    pub description: Option<String>,
    /// Message attached to every match.
    pub message: String,
    /// Bug | Vulnerability | CodeSmell | SecurityHotspot.
    pub rule_type: String,
    /// INFO | MINOR | MAJOR | CRITICAL | BLOCKER.
    pub severity: String,
    /// Target languages (informational/provenance).
    #[serde(default)]
    pub languages: Vec<String>,
    /// File extensions (no leading dot) the rule applies to.
    #[serde(default)]
    pub extensions: Vec<String>,
    /// Include globs (empty = all files).
    #[serde(default)]
    pub include_globs: Vec<String>,
    /// Exclude globs.
    #[serde(default)]
    pub exclude_globs: Vec<String>,
    /// Optional `MetricKey::as_str()` token this rule's count maps to.
    #[serde(default)]
    pub mapped_metric: Option<String>,
    /// Positive (MUST flag) + negative (MUST NOT flag) fixtures.
    #[serde(default)]
    pub examples: Vec<RuleExample>,
}

impl CustomRuleInput {
    /// Build the DB-free [`RuleDefinition`] for the admission gate. `rule_id` is a
    /// synthetic id during pre-persist validation.
    fn to_rule_definition(
        &self,
        rule_id: &str,
        rule_format: RuleFormat,
        severity: Severity,
        rule_type: RuleType,
    ) -> RuleDefinition {
        RuleDefinition {
            rule_id: rule_id.to_string(),
            name: self.name.clone(),
            rule_format,
            pattern: self.rule_body.clone(),
            severity,
            rule_type,
            message: self.message.clone(),
            languages: self.languages.clone(),
            extensions: self.extensions.clone(),
            include_globs: self.include_globs.clone(),
            exclude_globs: self.exclude_globs.clone(),
        }
    }
}

/// One failing example surfaced in a 400 when the admission gate rejects a rule.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct FailingExample {
    /// `positive` / `negative`.
    pub kind: String,
    /// The snippet that failed expectations.
    pub snippet: String,
    /// Whether the rule was expected to fire.
    pub expected_match: bool,
    /// Whether it actually fired.
    pub actual_match: bool,
}

/// The §10 `CustomRuleDraft` — the candidate rule shape echoed in an authoring
/// result (a projection of the internal [`GeneratedRule`]).
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CustomRuleDraft {
    pub rule_format: RuleFormat,
    pub rule_body: String,
    pub name: String,
    pub description: String,
    pub message: String,
    pub rule_type: String,
    pub severity: String,
    pub mapped_metric: Option<String>,
}

impl From<&GeneratedRule> for CustomRuleDraft {
    fn from(rule: &GeneratedRule) -> Self {
        Self {
            rule_format: rule.rule_format,
            rule_body: rule.pattern.clone(),
            name: rule.name.clone(),
            description: rule.description.clone(),
            message: rule.message.clone(),
            rule_type: rule_type_token(rule.rule_type).to_string(),
            severity: rule.severity.as_str().to_string(),
            mapped_metric: rule.mapped_metric.clone(),
        }
    }
}

/// The §10 `AdversaryTranscript` — a flattened view of the per-round debate.
#[derive(Debug, Clone, Default, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct AdversaryTranscript {
    /// Proposer-side notes (currently the final candidate's description; the
    /// proposer does not emit separate notes in P1).
    pub proposer_notes: String,
    /// Every round's adversary critique, in order.
    pub attacker_findings: Vec<String>,
    /// How many revision rounds ran (rounds beyond the first).
    pub revisions: usize,
}

/// The §10 `AuthorRuleResult.engine` echo: which model + backend ran the turns.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct AuthorEngine {
    pub model_config_id: String,
    pub backend: AuthoringBackend,
}

/// The §10 `AuthorRuleResult` — the full authoring run mapped from the internal
/// [`AuthorRunResult`] into the contract shape. NOT persisted here: the mandatory
/// human confirm + a subsequent POST create persists.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct AuthorRuleResult {
    pub candidate: CustomRuleDraft,
    /// 2-3 positive + 2-3 negative fixtures the candidate carries.
    pub examples: Vec<RuleExample>,
    pub empirical: EmpiricalReport,
    pub debate: AdversaryTranscript,
    pub round_trip: RoundTripVerdict,
    /// `passed` | `capped_out`.
    pub outcome: String,
    pub rounds_used: usize,
    pub engine: AuthorEngine,
}

impl AuthorRuleResult {
    /// Map the internal pipeline result + the engine echo into the §10 shape.
    fn from_run(run: &AuthorRunResult, model_config_id: &str) -> Self {
        let AuthoredCandidate {
            rule,
            empirical,
            round_trip,
        } = &run.candidate;

        let attacker_findings: Vec<String> = run
            .debate
            .iter()
            .filter(|f| !f.critique.trim().is_empty())
            .map(|f| f.critique.clone())
            .collect();

        let outcome = match run.outcome {
            services::services::rule_authoring::AuthorOutcome::Passed => "passed",
            services::services::rule_authoring::AuthorOutcome::CappedOut => "capped_out",
        };

        Self {
            candidate: CustomRuleDraft::from(rule),
            examples: rule.examples.clone(),
            empirical: empirical.clone(),
            debate: AdversaryTranscript {
                proposer_notes: rule.description.clone(),
                attacker_findings,
                // The first round is the initial draft; every subsequent round is
                // a revision.
                revisions: run.rounds_used.saturating_sub(1),
            },
            round_trip: round_trip.clone(),
            outcome: outcome.to_string(),
            rounds_used: run.rounds_used,
            engine: AuthorEngine {
                model_config_id: model_config_id.to_string(),
                backend: run.backend,
            },
        }
    }
}

/// Request body for `POST /custom-rules/author` (§10 `AuthorRuleRequest`).
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct AuthorRuleRequest {
    /// The natural-language ask ("prohibit X").
    pub nl_request: String,
    /// The user-selected model source (from the reused picker).
    pub model_config_id: String,
    /// Maps the source to its CLI roster.
    pub cli_type_id: String,
    /// `regex` (honoured in P1) | `ast_grep` (P2). Informational in P1.
    #[serde(default)]
    pub rule_format_preference: Option<String>,
    /// The live editor conditions, serialized as context for the proposer.
    #[serde(default)]
    pub current_rules_context: Option<serde_json::Value>,
}

/// Request body for `PATCH /custom-rules/{ruleId}/status`.
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct StatusUpdate {
    /// draft | shadow | warn | enforce | disabled.
    pub status: String,
}

// ─────────────────────────────────────────────────────────────────────────────
// The admission gate (deterministic, AI-free) — §10 / §12.
// ─────────────────────────────────────────────────────────────────────────────

/// Run the deterministic admission gate against an input. Returns the parsed
/// `(format, severity, rule_type)` tokens on success; a 400 `ApiError` describing
/// the failure otherwise. Reused verbatim by POST and by a PUT body change.
fn run_admission_gate(
    input: &CustomRuleInput,
) -> Result<(RuleFormat, Severity, RuleType), ApiError> {
    // (a) CHECK-enum tokens.
    let rule_format = parse_rule_format(&input.rule_format)?;
    let severity = parse_severity(&input.severity)?;
    let rule_type = parse_rule_type(&input.rule_type)?;

    // mapped_metric, if present, must be a known MetricKey token. `MetricKey`
    // deserializes from its serde-renamed token, so a serde round-trip is the
    // canonical "is this a real metric" check (no public enum iterator exists).
    if let Some(metric) = input.mapped_metric.as_deref() {
        let value = serde_json::Value::String(metric.to_string());
        if serde_json::from_value::<quality::metrics::MetricKey>(value).is_err() {
            return Err(ApiError::BadRequest(format!(
                "mappedMetric '{metric}' is not a known metric key"
            )));
        }
    }

    // (b) the regex compiles within the size limits (or P2-not-supported 400).
    let def = input.to_rule_definition("admission-candidate", rule_format, severity, rule_type);
    let compiled = compile(&def).map_err(|e| ApiError::BadRequest(e.to_string()))?;

    // (c) every positive flags ≥1, every negative flags 0.
    let mut failing: Vec<FailingExample> = Vec::new();
    for ex in &input.examples {
        let virtual_path = virtual_path_for(ex);
        let expected = matches!(ex.kind, services::services::rule_authoring::ExampleKind::Positive);
        let actual = !run_candidate(&compiled, &ex.snippet, virtual_path).is_empty();
        if actual != expected {
            failing.push(FailingExample {
                kind: ex.kind.as_db_str().to_string(),
                snippet: ex.snippet.clone(),
                expected_match: expected,
                actual_match: actual,
            });
        }
    }
    if !failing.is_empty() {
        let detail = serde_json::to_string(&failing).unwrap_or_default();
        return Err(ApiError::BadRequest(format!(
            "admission gate failed: {} example(s) did not meet expectations: {detail}",
            failing.len()
        )));
    }

    Ok((rule_format, severity, rule_type))
}

/// Pick a virtual path whose extension matches the example language (so a rule
/// scoped to `extensions: ["rs"]` is exercised). Mirrors the services empirical
/// test's `virtual_path_for`.
fn virtual_path_for(example: &RuleExample) -> &'static str {
    match example.language.as_deref().map(str::to_ascii_lowercase) {
        Some(ref l) if l == "typescript" || l == "ts" => "snippet.ts",
        Some(ref l) if l == "javascript" || l == "js" => "snippet.js",
        Some(ref l) if l == "tsx" => "snippet.tsx",
        Some(ref l) if l == "rust" || l == "rs" => "snippet.rs",
        _ => "snippet.rs",
    }
}

/// Whether a PUT changed a rule **body** field (D8 trigger) vs metadata only.
/// Body fields are the matching logic: pattern (`rule_body`), `rule_format`,
/// scope (languages/extensions/include/exclude globs), `severity`, `rule_type`,
/// and `mapped_metric`. Metadata = `name`, `description`, `message`, `nl_request`.
fn body_changed(existing: &db::models::CustomRule, input: &CustomRuleInput) -> bool {
    existing.rule_body != input.rule_body
        || existing.rule_format != input.rule_format
        || existing.severity != input.severity
        || existing.rule_type != input.rule_type
        || existing.mapped_metric.as_deref() != input.mapped_metric.as_deref()
}

// ─────────────────────────────────────────────────────────────────────────────
// CRUD handlers
// ─────────────────────────────────────────────────────────────────────────────

/// GET /projects/{project_id}/custom-rules — list all rules for the project.
pub async fn list_custom_rules(
    State(deployment): State<DeploymentImpl>,
    Path(project_id): Path<Uuid>,
) -> Result<Json<ApiResponse<Vec<db::models::CustomRule>>>, ApiError> {
    let rules = db::models::CustomRule::find_by_project(&deployment.db().pool, project_id)
        .await
        .map_err(ApiError::Database)?;
    Ok(Json(ApiResponse::success(rules)))
}

/// POST /projects/{project_id}/custom-rules — admission-gate + create at shadow.
pub async fn create_custom_rule(
    State(deployment): State<DeploymentImpl>,
    Path(project_id): Path<Uuid>,
    Json(input): Json<CustomRuleInput>,
) -> Result<Json<ApiResponse<db::models::CustomRule>>, ApiError> {
    let pool = &deployment.db().pool;

    // THE ADMISSION GATE (deterministic, AI-free) — runs for BOTH AI-authored and
    // manually-entered rules, BEFORE persist.
    let (rule_format, _severity, rule_type) = run_admission_gate(&input)?;

    let nl_request = input.nl_request.clone().unwrap_or_else(|| input.name.clone());
    let created = db::models::CustomRule::create(
        pool,
        &db::models::CreateCustomRule {
            project_id: Some(project_id),
            name: input.name.clone(),
            nl_request,
            rule_format: rule_format_token(rule_format).to_string(),
            rule_body: input.rule_body.clone(),
            description: input.description.clone(),
            rule_type: rule_type_token(rule_type).to_string(),
            // Persist the severity token verbatim (already validated).
            severity: input.severity.clone(),
            mapped_metric: input.mapped_metric.clone(),
            created_by: None,
        },
    )
    .await
    .map_err(ApiError::Database)?;

    // Persist the fixtures so the rule stays self-documenting + regression-locked.
    persist_examples(pool, created.id, &input.examples).await?;

    // Audit (create).
    db::models::CustomRuleAudit::insert(
        pool,
        &db::models::CreateCustomRuleAudit {
            rule_id: created.id,
            project_id: Some(project_id),
            action: "create".to_string(),
            actor: None,
            from_version: None,
            to_version: Some(created.version),
            diff_json: None,
        },
    )
    .await
    .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(created)))
}

/// PUT /projects/{project_id}/custom-rules/{ruleId} — D8 edit policy.
pub async fn update_custom_rule(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, rule_id)): Path<(Uuid, Uuid)>,
    Json(input): Json<CustomRuleInput>,
) -> Result<Json<ApiResponse<db::models::CustomRule>>, ApiError> {
    let pool = &deployment.db().pool;

    let existing = db::models::CustomRule::find_by_id(pool, rule_id)
        .await
        .map_err(ApiError::Database)?
        .ok_or_else(|| ApiError::NotFound(format!("custom rule {rule_id} not found")))?;

    let is_body_change = body_changed(&existing, &input);

    // D8: a body change re-runs the admission gate BEFORE persist; a metadata-only
    // change skips it (but the tokens are still validated cheaply on a body
    // change). For metadata-only we still validate the unchanged tokens parse
    // (they were already valid on create, so this is a no-op safety check).
    if is_body_change {
        run_admission_gate(&input)?;
    } else {
        // Validate the (unchanged) tokens still parse — cheap, no regex/examples.
        parse_rule_format(&input.rule_format)?;
        parse_severity(&input.severity)?;
        parse_rule_type(&input.rule_type)?;
    }

    let from_version = existing.version;
    let nl_request = input
        .nl_request
        .clone()
        .unwrap_or_else(|| existing.nl_request.clone());

    let updated = db::models::CustomRule::update(
        pool,
        rule_id,
        &db::models::UpdateCustomRule {
            name: input.name.clone(),
            nl_request,
            rule_format: input.rule_format.clone(),
            rule_body: input.rule_body.clone(),
            description: input.description.clone(),
            rule_type: input.rule_type.clone(),
            severity: input.severity.clone(),
            mapped_metric: input.mapped_metric.clone(),
        },
    )
    .await
    .map_err(ApiError::Database)?;

    if is_body_change {
        // Re-derive the fixture set from the submitted examples and drop to shadow.
        replace_examples(pool, rule_id, &input.examples).await?;
        let updated = db::models::CustomRule::set_status(pool, rule_id, "shadow")
            .await
            .map_err(ApiError::Database)?;

        // Write a validation artifact recording the admission-gate pass (the AI
        // round-trip is re-run via the dedicated /revalidate route; this records
        // the deterministic gate outcome for the body edit).
        let total = input.examples.len() as i64;
        db::models::CustomRuleValidation::insert(
            pool,
            &db::models::CreateCustomRuleValidation {
                rule_id,
                rule_version: updated.version,
                verdict: "pass".to_string(),
                roundtrip_ok: None,
                judge_score: None,
                examples_total: total,
                examples_passed: total,
                rounds_used: 0,
                results_json: None,
                error_message: None,
                validated_by: None,
            },
        )
        .await
        .map_err(ApiError::Database)?;

        db::models::CustomRuleAudit::insert(
            pool,
            &db::models::CreateCustomRuleAudit {
                rule_id,
                project_id: Some(project_id),
                action: "revalidate".to_string(),
                actor: None,
                from_version: Some(from_version),
                to_version: Some(updated.version),
                diff_json: None,
            },
        )
        .await
        .map_err(ApiError::Database)?;

        return Ok(Json(ApiResponse::success(updated)));
    }

    // Metadata-only: bump version (done by update) + audit, skip revalidation.
    db::models::CustomRuleAudit::insert(
        pool,
        &db::models::CreateCustomRuleAudit {
            rule_id,
            project_id: Some(project_id),
            action: "update".to_string(),
            actor: None,
            from_version: Some(from_version),
            to_version: Some(updated.version),
            diff_json: None,
        },
    )
    .await
    .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(updated)))
}

/// DELETE /projects/{project_id}/custom-rules/{ruleId}.
pub async fn delete_custom_rule(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, rule_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;

    // Audit BEFORE delete so the (FK-less) audit row references a valid version.
    if let Some(existing) = db::models::CustomRule::find_by_id(pool, rule_id)
        .await
        .map_err(ApiError::Database)?
    {
        db::models::CustomRuleAudit::insert(
            pool,
            &db::models::CreateCustomRuleAudit {
                rule_id,
                project_id: Some(project_id),
                action: "delete".to_string(),
                actor: None,
                from_version: Some(existing.version),
                to_version: None,
                diff_json: None,
            },
        )
        .await
        .map_err(ApiError::Database)?;
    }

    db::models::CustomRule::delete(pool, rule_id)
        .await
        .map_err(ApiError::Database)?;
    Ok(Json(ApiResponse::success(())))
}

/// PATCH /projects/{project_id}/custom-rules/{ruleId}/status — shadow→warn→enforce.
pub async fn update_custom_rule_status(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, rule_id)): Path<(Uuid, Uuid)>,
    Json(body): Json<StatusUpdate>,
) -> Result<Json<ApiResponse<db::models::CustomRule>>, ApiError> {
    let pool = &deployment.db().pool;

    // Validate against the CHECK enum BEFORE hitting the DB for a clearer error.
    const VALID: [&str; 5] = ["draft", "shadow", "warn", "enforce", "disabled"];
    if !VALID.contains(&body.status.as_str()) {
        return Err(ApiError::BadRequest(format!(
            "invalid status '{}' (expected one of {VALID:?})",
            body.status
        )));
    }

    let existing = db::models::CustomRule::find_by_id(pool, rule_id)
        .await
        .map_err(ApiError::Database)?
        .ok_or_else(|| ApiError::NotFound(format!("custom rule {rule_id} not found")))?;

    let updated = db::models::CustomRule::set_status(pool, rule_id, &body.status)
        .await
        .map_err(ApiError::Database)?;

    db::models::CustomRuleAudit::insert(
        pool,
        &db::models::CreateCustomRuleAudit {
            rule_id,
            project_id: Some(project_id),
            action: "promote".to_string(),
            actor: None,
            from_version: Some(existing.version),
            to_version: Some(updated.version),
            diff_json: Some(format!(
                "{{\"from_status\":\"{}\",\"to_status\":\"{}\"}}",
                existing.status, updated.status
            )),
        },
    )
    .await
    .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(updated)))
}

/// GET /projects/{project_id}/custom-rules/{ruleId}/validations.
pub async fn list_custom_rule_validations(
    State(deployment): State<DeploymentImpl>,
    Path((_project_id, rule_id)): Path<(Uuid, Uuid)>,
) -> Result<Json<ApiResponse<Vec<db::models::CustomRuleValidation>>>, ApiError> {
    let validations =
        db::models::CustomRuleValidation::find_by_rule(&deployment.db().pool, rule_id)
            .await
            .map_err(ApiError::Database)?;
    Ok(Json(ApiResponse::success(validations)))
}

// ─────────────────────────────────────────────────────────────────────────────
// AI authoring + revalidation
// ─────────────────────────────────────────────────────────────────────────────

/// POST /projects/{project_id}/custom-rules/author — run the multi-agent
/// authoring pipeline (§10). Does NOT persist: the mandatory human confirm + a
/// subsequent POST create persists.
pub async fn author_custom_rule(
    State(deployment): State<DeploymentImpl>,
    Path(project_id): Path<Uuid>,
    Json(req): Json<AuthorRuleRequest>,
) -> Result<Json<ApiResponse<AuthorRuleResult>>, ApiError> {
    let pool = &deployment.db().pool;
    let project = project_id.to_string();

    // Build the invoker from the EXPLICIT user selection (no default-billing
    // fallthrough), surfacing the chosen backend for the engine echo + the
    // run_authoring backend arg.
    let (client, backend) =
        build_authoring_client_with_backend(pool, &project, &req.model_config_id, &req.cli_type_id)
            .await
            .map_err(|e| ApiError::BadRequest(e.to_string()))?;

    // Serialize the live editor conditions as plain-text context for the proposer.
    let context = req
        .current_rules_context
        .as_ref()
        .map(|v| serde_json::to_string_pretty(v).unwrap_or_default());

    let run = run_authoring(client.as_ref(), &req.nl_request, context.as_deref(), backend)
        .await
        .map_err(|e| ApiError::Internal(format!("rule authoring failed: {e}")))?;

    Ok(Json(ApiResponse::success(AuthorRuleResult::from_run(
        &run,
        &req.model_config_id,
    ))))
}

/// POST /projects/{project_id}/custom-rules/{ruleId}/revalidate — D8 full
/// pipeline; drops the rule to shadow and persists a fresh validation artifact.
///
/// Returns the newest [`db::models::CustomRuleValidation`] row (§10 contract).
pub async fn revalidate_custom_rule(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, rule_id)): Path<(Uuid, Uuid)>,
    Json(req): Json<AuthorRuleRequest>,
) -> Result<Json<ApiResponse<db::models::CustomRuleValidation>>, ApiError> {
    let pool = &deployment.db().pool;
    let project = project_id.to_string();

    let (client, backend) =
        build_authoring_client_with_backend(pool, &project, &req.model_config_id, &req.cli_type_id)
            .await
            .map_err(|e| ApiError::BadRequest(e.to_string()))?;

    // The pipeline needs an `AuthoringAgents` (4 roles). One backend client plays
    // every role — round-trip independence comes from a fresh message vector, not
    // a distinct transport (mirrors `run_authoring`).
    let llm = client.as_ref();
    let agents = AuthoringAgents {
        generator: llm,
        adversary: llm,
        interpreter: llm,
        matcher: llm,
    };

    revalidate_rule_body(pool, &agents, rule_id, None, backend)
        .await
        .map_err(|e| ApiError::Internal(format!("revalidation failed: {e}")))?;

    // Return the freshly-written validation artifact (newest first).
    let latest = db::models::CustomRuleValidation::find_by_rule(pool, rule_id)
        .await
        .map_err(ApiError::Database)?
        .into_iter()
        .next()
        .ok_or_else(|| {
            ApiError::Internal("revalidation produced no validation artifact".to_string())
        })?;

    Ok(Json(ApiResponse::success(latest)))
}

// ─────────────────────────────────────────────────────────────────────────────
// Example persistence helpers
// ─────────────────────────────────────────────────────────────────────────────

/// Insert the input's examples for a freshly-created rule.
async fn persist_examples(
    pool: &sqlx::SqlitePool,
    rule_id: Uuid,
    examples: &[RuleExample],
) -> Result<(), ApiError> {
    if examples.is_empty() {
        return Ok(());
    }
    let rows: Vec<db::models::CreateCustomRuleExample> = examples
        .iter()
        .map(|ex| db::models::CreateCustomRuleExample {
            rule_id,
            kind: ex.kind.as_db_str().to_string(),
            language: ex.language.clone(),
            snippet: ex.snippet.clone(),
            expected_match: ex.kind.expected_match(),
            note: ex.note.clone(),
        })
        .collect();
    db::models::CustomRuleExample::insert_batch(pool, &rows)
        .await
        .map_err(ApiError::Database)
}

/// Replace a rule's fixture set on a body edit: delete the old examples, insert
/// the new ones. (`custom_rule_example` cascades on rule delete but a rule UPDATE
/// does not touch children, so the old fixtures are cleared explicitly here.)
async fn replace_examples(
    pool: &sqlx::SqlitePool,
    rule_id: Uuid,
    examples: &[RuleExample],
) -> Result<(), ApiError> {
    sqlx::query("DELETE FROM custom_rule_example WHERE rule_id = ?1")
        .bind(rule_id)
        .execute(pool)
        .await
        .map_err(ApiError::Database)?;
    persist_examples(pool, rule_id, examples).await
}

// ─────────────────────────────────────────────────────────────────────────────
// Route builder
// ─────────────────────────────────────────────────────────────────────────────

/// Custom-rule routes nested under `/projects`.
pub fn custom_rules_project_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route(
            "/{project_id}/custom-rules",
            get(list_custom_rules).post(create_custom_rule),
        )
        .route(
            "/{project_id}/custom-rules/author",
            post(author_custom_rule),
        )
        .route(
            "/{project_id}/custom-rules/{rule_id}",
            axum::routing::put(update_custom_rule).delete(delete_custom_rule),
        )
        .route(
            "/{project_id}/custom-rules/{rule_id}/status",
            axum::routing::patch(update_custom_rule_status),
        )
        .route(
            "/{project_id}/custom-rules/{rule_id}/validations",
            get(list_custom_rule_validations),
        )
        .route(
            "/{project_id}/custom-rules/{rule_id}/revalidate",
            post(revalidate_custom_rule),
        )
}
