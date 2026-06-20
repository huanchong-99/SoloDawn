//! Quality gate REST API routes.
//!
//! Exposes quality run and issue data for frontend display.
//! - GET /workflows/:id/quality/runs       — list quality runs for a workflow
//! - GET /quality/runs/:run_id             — single quality run by ID
//! - GET /quality/runs/:run_id/issues      — issues for a quality run
//! - GET /terminals/:id/quality/latest     — latest quality run for a terminal

use axum::{
    Json, Router,
    extract::{Path, State},
    routing::get,
};
use deployment::Deployment;
use serde::Serialize;
use ts_rs::TS;
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError};

/// Summary response for a quality run, tailored for list views.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityRunSummary {
    pub id: String,
    pub workflow_id: String,
    pub task_id: Option<String>,
    pub terminal_id: Option<String>,
    pub commit_hash: Option<String>,
    pub gate_level: String,
    pub gate_status: String,
    pub mode: String,
    pub total_issues: i32,
    pub blocking_issues: i32,
    pub new_issues: i32,
    pub duration_ms: i32,
    pub error_message: Option<String>,
    pub created_at: String,
    pub completed_at: Option<String>,
}

impl From<db::models::QualityRun> for QualityRunSummary {
    fn from(r: db::models::QualityRun) -> Self {
        Self {
            id: r.id,
            workflow_id: r.workflow_id,
            task_id: r.task_id,
            terminal_id: r.terminal_id,
            commit_hash: r.commit_hash,
            gate_level: r.gate_level,
            gate_status: r.gate_status,
            mode: r.mode,
            total_issues: r.total_issues,
            blocking_issues: r.blocking_issues,
            new_issues: r.new_issues,
            duration_ms: r.duration_ms,
            error_message: r.error_message,
            created_at: r.created_at.to_rfc3339(),
            completed_at: r.completed_at.map(|t| t.to_rfc3339()),
        }
    }
}

/// Detail response for a single quality run, includes report JSON.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityRunDetail {
    #[serde(flatten)]
    pub summary: QualityRunSummary,
    pub providers_run: Option<serde_json::Value>,
    pub report_json: Option<serde_json::Value>,
    pub decision_json: Option<serde_json::Value>,
}

impl From<db::models::QualityRun> for QualityRunDetail {
    fn from(r: db::models::QualityRun) -> Self {
        let providers = r
            .providers_run
            .as_deref()
            .and_then(|s| serde_json::from_str(s).ok());
        let report = r
            .report_json
            .as_deref()
            .and_then(|s| serde_json::from_str(s).ok());
        let decision = r
            .decision_json
            .as_deref()
            .and_then(|s| serde_json::from_str(s).ok());
        Self {
            summary: QualityRunSummary::from(db::models::QualityRun {
                providers_run: None,
                report_json: None,
                decision_json: None,
                ..r
            }),
            providers_run: providers,
            report_json: report,
            decision_json: decision,
        }
    }
}

/// GET /workflows/:workflow_id/quality/runs
pub async fn list_quality_runs(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<String>,
) -> Result<Json<ApiResponse<Vec<QualityRunSummary>>>, ApiError> {
    let runs = db::models::QualityRun::find_by_workflow(&deployment.db().pool, &workflow_id)
        .await
        .map_err(ApiError::Database)?;

    let summaries: Vec<QualityRunSummary> = runs.into_iter().map(QualityRunSummary::from).collect();
    Ok(Json(ApiResponse::success(summaries)))
}

/// GET /quality/runs/:run_id
pub async fn get_quality_run(
    State(deployment): State<DeploymentImpl>,
    Path(run_id): Path<String>,
) -> Result<Json<ApiResponse<Option<QualityRunDetail>>>, ApiError> {
    let run = db::models::QualityRun::find_by_id(&deployment.db().pool, &run_id)
        .await
        .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(run.map(QualityRunDetail::from))))
}

/// GET /quality/runs/:run_id/issues
pub async fn get_quality_issues(
    State(deployment): State<DeploymentImpl>,
    Path(run_id): Path<String>,
) -> Result<Json<ApiResponse<Vec<db::models::QualityIssueRecord>>>, ApiError> {
    let issues = db::models::QualityIssueRecord::find_by_run(&deployment.db().pool, &run_id)
        .await
        .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(issues)))
}

/// GET /terminals/:terminal_id/quality/latest
pub async fn get_terminal_latest_quality(
    State(deployment): State<DeploymentImpl>,
    Path(terminal_id): Path<String>,
) -> Result<Json<ApiResponse<Option<QualityRunSummary>>>, ApiError> {
    let run = db::models::QualityRun::find_latest_by_terminal(&deployment.db().pool, &terminal_id)
        .await
        .map_err(ApiError::Database)?;

    Ok(Json(ApiResponse::success(run.map(QualityRunSummary::from))))
}

/// Quality routes nested under /workflows
pub fn quality_workflow_routes() -> Router<DeploymentImpl> {
    Router::new().route("/{workflow_id}/quality/runs", get(list_quality_runs))
}

/// Quality routes at /quality
pub fn quality_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/runs/{run_id}", get(get_quality_run))
        .route("/runs/{run_id}/issues", get(get_quality_issues))
}

/// Quality routes nested under /terminals
pub fn quality_terminal_routes() -> Router<DeploymentImpl> {
    Router::new().route(
        "/{terminal_id}/quality/latest",
        get(get_terminal_latest_quality),
    )
}

// ─────────────────────────────────────────────────────────────────────────────
// G3 — per-project quality-gate policy CRUD + catalog endpoints
// ─────────────────────────────────────────────────────────────────────────────

use axum::extract::Path as AxumPath;
use db::models::project_quality_policy::ProjectQualityPolicy;
use quality::config::{BUNDLED_CENTRAL_POLICY, QualityGateConfig, QualityGateMode};
use quality::metrics::MetricKey;
use uuid::Uuid;

/// Effective quality policy for a project, with its resolution source.
#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct QualityPolicyResponse {
    /// Where the config came from: "project" | "file" | "bundled".
    pub source: String,
    /// The resolved quality-gate configuration.
    pub config: QualityGateConfig,
}

/// Picker source for the rules editor: the closed `MetricKey` enum (sentinel
/// excluded) plus the supported comparison operators.
#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct MetricCatalogResponse {
    /// Closed-enum picker source (the `QualityGateEmptyScan` sentinel is excluded).
    pub metrics: Vec<MetricKey>,
    /// Supported condition operators: `["GT", "LT"]`.
    pub operators: Vec<String>,
    /// Self-documenting tooltip catalog: one entry per selectable metric (D7 /
    /// PRD §7.1). A static compiled table — safe to cache (`staleTime` 1h).
    pub info: Vec<MetricInfo>,
}

/// One tooltip entry for a selectable metric (PRD §7.1 / §10).
///
/// `higherIsWorse` tells the UI which direction is bad (most metrics are counts
/// where higher = worse; coverage metrics invert it). `description` + `example`
/// power the circled-"!" popover next to the metric `<select>`.
#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct MetricInfo {
    /// The metric this entry documents.
    pub key: MetricKey,
    /// Human-readable name (mirrors `MetricKey::display_name()`).
    pub display_name: String,
    /// What the metric measures, in plain language.
    pub description: String,
    /// A concrete example of what it counts/flags.
    pub example: String,
    /// `true` when a higher value is worse (counts); `false` for coverage-style
    /// metrics where higher is better.
    pub higher_is_worse: bool,
}

/// The latest-persisted-run metric snapshot for a project (D7 / PRD §10).
///
/// Read from the latest `quality_run.report_json`; NEVER recomputed on hover. The
/// `values` map keys are the metric tokens; missing metrics simply have no entry.
/// When the project has no run yet, `values` is empty and `runId`/`ranAt` null.
#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct ProjectMetricSnapshot {
    /// `MetricKey` -> the metric's measured value at the latest run.
    pub values: std::collections::HashMap<MetricKey, quality::gate::result::MeasureValue>,
    /// The `quality_run.id` the snapshot came from (null = no run yet).
    pub run_id: Option<String>,
    /// When that run completed/was created (RFC3339; null = no run yet).
    pub ran_at: Option<String>,
}

/// Static per-metric tooltip text for every selectable metric (PRD §7.1). Kept
/// as a compiled `match` so adding a `MetricKey` variant forces a compile error
/// here (the catalog can never silently miss a metric). `(description, example,
/// higher_is_worse)`.
fn metric_doc(key: MetricKey) -> (&'static str, &'static str, bool) {
    match key {
        MetricKey::CargoCheckErrors => (
            "Compiler errors reported by `cargo check` across the Rust workspace.",
            "A type mismatch or unresolved import that fails compilation.",
            true,
        ),
        MetricKey::ClippyWarnings => (
            "Lint warnings from `cargo clippy` (style, correctness, complexity).",
            "`needless_return` or `redundant_clone` warnings.",
            true,
        ),
        MetricKey::ClippyErrors => (
            "Clippy lints raised at error level (deny-by-default or `-D warnings`).",
            "A `clippy::correctness` lint promoted to an error.",
            true,
        ),
        MetricKey::FmtViolations => (
            "Files that are not `cargo fmt`-clean.",
            "A file with inconsistent indentation `rustfmt` would rewrite.",
            true,
        ),
        MetricKey::RustTestFailures => (
            "Failing Rust unit/integration tests.",
            "An `assert_eq!` that does not hold at test time.",
            true,
        ),
        MetricKey::EslintErrors => (
            "ESLint errors in the frontend.",
            "`no-unused-vars` raised at error severity.",
            true,
        ),
        MetricKey::EslintWarnings => (
            "ESLint warnings in the frontend.",
            "A `react-hooks/exhaustive-deps` warning.",
            true,
        ),
        MetricKey::TscErrors => (
            "TypeScript compiler (`tsc`) type errors.",
            "Assigning a `string` to a `number`-typed field.",
            true,
        ),
        MetricKey::FrontendTestFailures => (
            "Failing frontend tests (Vitest/Jest).",
            "A component test whose snapshot no longer matches.",
            true,
        ),
        MetricKey::FrontendTestDepsMissing => (
            "Frontend test dependencies that are not installed.",
            "A test importing a package missing from `node_modules`.",
            true,
        ),
        MetricKey::TestFailures => (
            "Total failing tests across all suites.",
            "Any unit or integration test that does not pass.",
            true,
        ),
        MetricKey::TestCoverage => (
            "Overall test coverage percentage.",
            "60% means 40% of lines are unexercised by tests.",
            false,
        ),
        MetricKey::Bugs => (
            "Reliability issues (SonarQube `Bug` type).",
            "A possible null dereference.",
            true,
        ),
        MetricKey::NewBugs => (
            "Newly introduced bugs versus the baseline.",
            "A bug added in the current change set.",
            true,
        ),
        MetricKey::CodeSmells => (
            "Maintainability issues (SonarQube `Code Smell`).",
            "A deeply nested function that is hard to follow.",
            true,
        ),
        MetricKey::Vulnerabilities => (
            "Security vulnerabilities detected.",
            "A SQL query built by string concatenation.",
            true,
        ),
        MetricKey::DuplicatedLinesDensity => (
            "Percentage of lines that are duplicated.",
            "Two copy-pasted blocks differing only in a literal.",
            true,
        ),
        MetricKey::SecurityIssues => (
            "Security-category issues across analyzers.",
            "A hard-coded credential in source.",
            true,
        ),
        MetricKey::RedosRisks => (
            "Regular expressions vulnerable to catastrophic backtracking (ReDoS).",
            "A pattern like `(a+)+$` over untrusted input.",
            true,
        ),
        MetricKey::GenerateTypesCheckFailures => (
            "`generate_types --check` drift (shared TS types out of date).",
            "A Rust DTO changed without regenerating `shared/types`.",
            true,
        ),
        MetricKey::PrepareDbCheckFailures => (
            "`sqlx prepare --check` drift (the offline query cache is stale).",
            "A new `query!` without a committed `.sqlx/` entry.",
            true,
        ),
        MetricKey::SonarQualityGateStatus => (
            "The SonarQube project quality-gate status (OK/WARN/ERROR).",
            "`ERROR` when a Sonar condition is breached.",
            true,
        ),
        MetricKey::SonarIssues => (
            "Total open issues reported by SonarQube.",
            "Any unresolved Sonar finding.",
            true,
        ),
        MetricKey::SonarBlockerIssues => (
            "SonarQube issues at Blocker severity.",
            "A resource leak Sonar marks as Blocker.",
            true,
        ),
        MetricKey::SonarCriticalIssues => (
            "SonarQube issues at Critical severity.",
            "A Critical-severity security hotspot.",
            true,
        ),
        MetricKey::BuiltinRustIssues => (
            "Issues from the built-in Rust analyzer.",
            "A `.unwrap()` in non-test production code.",
            true,
        ),
        MetricKey::BuiltinRustCritical => (
            "Critical issues from the built-in Rust analyzer.",
            "A `panic!` on an external-input path.",
            true,
        ),
        MetricKey::RustCyclomaticComplexity => (
            "Cyclomatic complexity of Rust functions (branch count).",
            "A function with many nested `match`/`if` arms.",
            true,
        ),
        MetricKey::RustCognitiveComplexity => (
            "Cognitive complexity of Rust functions (how hard to read).",
            "Deeply nested control flow with early returns.",
            true,
        ),
        MetricKey::BuiltinFrontendIssues => (
            "Issues from the built-in frontend analyzer.",
            "A `console.log` left in committed TS.",
            true,
        ),
        MetricKey::BuiltinFrontendCritical => (
            "Critical issues from the built-in frontend analyzer.",
            "`dangerouslySetInnerHTML` with unsanitized input.",
            true,
        ),
        MetricKey::BuiltinCommonIssues => (
            "Issues from the built-in language-agnostic analyzer.",
            "A committed merge-conflict marker.",
            true,
        ),
        MetricKey::DuplicatedBlocks => (
            "Count of duplicated code blocks.",
            "The same 30-line block appearing in two files.",
            true,
        ),
        MetricKey::SecretsDetected => (
            "Hard-coded secrets detected in source.",
            "An AWS access key committed in a config file.",
            true,
        ),
        MetricKey::CustomRuleViolations => (
            "Total matches from your project's custom (editable) rules. This is the \
             count a quality gate uses to make custom rules block — add a \
             `Custom Rule Violations` GT condition to enforce them.",
            "Every hit of a rule like 'prohibit `dbg!`'.",
            true,
        ),
        MetricKey::CustomRuleCritical => (
            "Custom-rule matches at Critical+ severity. Custom-rule severity is \
             advisory-capped to Major, so this stays 0 in practice — gate on \
             `Custom Rule Violations` (the count), not this, to block custom rules.",
            "Normally 0: a custom rule cannot self-escalate above Major.",
            true,
        ),
        MetricKey::LineCoverage => (
            "Percentage of lines covered by tests.",
            "75% means a quarter of lines are untested.",
            false,
        ),
        MetricKey::BranchCoverage => (
            "Percentage of branches covered by tests.",
            "An `if/else` whose `else` arm is never exercised.",
            false,
        ),
        MetricKey::TestFileAbsence => (
            "Source modules with no accompanying test file.",
            "A `service.rs` with no `service` tests anywhere.",
            true,
        ),
        MetricKey::TodoDensity => (
            "Density of TODO/FIXME markers.",
            "A file peppered with `// TODO` comments.",
            true,
        ),
        MetricKey::StubTestCount => (
            "Stubbed/empty tests that assert nothing real.",
            "A test body that is just `assert!(true)`.",
            true,
        ),
        MetricKey::CoverageExclusionIssues => (
            "Suspicious coverage-exclusion annotations.",
            "A broad `#[cfg(not(coverage))]` hiding real code.",
            true,
        ),
        MetricKey::TestAuthenticityIssues => (
            "Tests that look real but do not meaningfully verify behavior.",
            "A test mocking the very thing it claims to check.",
            true,
        ),
        MetricKey::ProjectConventionIssues => (
            "Violations of project-specific conventions.",
            "A new module not registered where the project expects.",
            true,
        ),
        MetricKey::RuntimeSecuritySmells => (
            "Runtime security smells (dynamic/unsafe patterns).",
            "An `eval`-style dynamic execution on user input.",
            true,
        ),
        // The sentinel is never selectable, but the exhaustive match must cover it.
        MetricKey::QualityGateEmptyScan => (
            "Internal sentinel emitted when a scan produced no analyzable files.",
            "An empty repository or a fully-excluded scan.",
            true,
        ),
    }
}

/// All selectable metric keys, EXCLUDING the internal `QualityGateEmptyScan`
/// sentinel which must never be picked in a user-authored gate condition.
fn selectable_metric_keys() -> Vec<MetricKey> {
    vec![
        MetricKey::CargoCheckErrors,
        MetricKey::ClippyWarnings,
        MetricKey::ClippyErrors,
        MetricKey::FmtViolations,
        MetricKey::RustTestFailures,
        MetricKey::EslintErrors,
        MetricKey::EslintWarnings,
        MetricKey::TscErrors,
        MetricKey::FrontendTestFailures,
        MetricKey::FrontendTestDepsMissing,
        MetricKey::TestFailures,
        MetricKey::TestCoverage,
        MetricKey::Bugs,
        MetricKey::NewBugs,
        MetricKey::CodeSmells,
        MetricKey::Vulnerabilities,
        MetricKey::DuplicatedLinesDensity,
        MetricKey::SecurityIssues,
        MetricKey::RedosRisks,
        MetricKey::GenerateTypesCheckFailures,
        MetricKey::PrepareDbCheckFailures,
        MetricKey::SonarQualityGateStatus,
        MetricKey::SonarIssues,
        MetricKey::SonarBlockerIssues,
        MetricKey::SonarCriticalIssues,
        MetricKey::BuiltinRustIssues,
        MetricKey::BuiltinRustCritical,
        MetricKey::RustCyclomaticComplexity,
        MetricKey::RustCognitiveComplexity,
        MetricKey::BuiltinFrontendIssues,
        MetricKey::BuiltinFrontendCritical,
        MetricKey::BuiltinCommonIssues,
        MetricKey::DuplicatedBlocks,
        MetricKey::SecretsDetected,
        // Custom-rule opt-in gating (D3/G4): an operator adds a `CustomRuleViolations`
        // GT condition to make authored rules block. `CustomRuleCritical` stays 0
        // (severity is advisory-capped to Major) but is selectable for completeness.
        MetricKey::CustomRuleViolations,
        MetricKey::CustomRuleCritical,
        MetricKey::LineCoverage,
        MetricKey::BranchCoverage,
        MetricKey::TestFileAbsence,
        MetricKey::TodoDensity,
        MetricKey::StubTestCount,
        MetricKey::CoverageExclusionIssues,
        MetricKey::TestAuthenticityIssues,
        MetricKey::ProjectConventionIssues,
        MetricKey::RuntimeSecuritySmells,
        // NOTE: QualityGateEmptyScan is intentionally omitted (internal sentinel).
    ]
}

/// Serialize a `QualityGateMode` to its YAML/db string form (`off|shadow|warn|enforce`).
fn mode_str(mode: QualityGateMode) -> &'static str {
    match mode {
        QualityGateMode::Off => "off",
        QualityGateMode::Shadow => "shadow",
        QualityGateMode::Warn => "warn",
        QualityGateMode::Enforce => "enforce",
    }
}

/// GET /quality/policy/default — the bundled central policy.
pub async fn get_default_policy() -> Result<Json<ApiResponse<QualityPolicyResponse>>, ApiError> {
    let config = QualityGateConfig::from_yaml(BUNDLED_CENTRAL_POLICY)
        .map_err(|e| ApiError::Internal(format!("parse bundled policy: {e}")))?;
    Ok(Json(ApiResponse::success(QualityPolicyResponse {
        source: "bundled".to_string(),
        config,
    })))
}

/// GET /quality/policy/metrics — picker catalog (closed enum minus sentinel +
/// operators) ENRICHED with the self-documenting tooltip catalog (D7 / §7.1).
pub async fn get_metric_catalog() -> Result<Json<ApiResponse<MetricCatalogResponse>>, ApiError> {
    let metrics = selectable_metric_keys();
    let info: Vec<MetricInfo> = metrics
        .iter()
        .map(|&key| {
            let (description, example, higher_is_worse) = metric_doc(key);
            MetricInfo {
                key,
                display_name: key.display_name().to_string(),
                description: description.to_string(),
                example: example.to_string(),
                higher_is_worse,
            }
        })
        .collect();
    Ok(Json(ApiResponse::success(MetricCatalogResponse {
        metrics,
        operators: vec!["GT".to_string(), "LT".to_string()],
        info,
    })))
}

/// GET /projects/{project_id}/quality-metrics/latest — the latest persisted-run
/// metric snapshot (D7). Reads `quality_run.report_json`; never recomputes.
pub async fn get_project_metric_snapshot(
    State(deployment): State<DeploymentImpl>,
    AxumPath(project_id): AxumPath<Uuid>,
) -> Result<Json<ApiResponse<ProjectMetricSnapshot>>, ApiError> {
    use std::collections::HashMap;

    let run = db::models::QualityRun::find_latest_by_project(&deployment.db().pool, project_id)
        .await
        .map_err(ApiError::Database)?;

    let Some(run) = run else {
        // No run yet (documented nullable case — degrades to "no run yet").
        return Ok(Json(ApiResponse::success(ProjectMetricSnapshot {
            values: HashMap::new(),
            run_id: None,
            ran_at: None,
        })));
    };

    // Aggregate the per-provider metric maps out of the persisted report. The
    // report is a serialized `quality::report::QualityReport`; its metrics live in
    // `provider_reports[].metrics`. A later provider overrides an earlier one for
    // the same key (the same precedence the engine aggregation uses).
    let mut values: HashMap<MetricKey, quality::gate::result::MeasureValue> = HashMap::new();
    if let Some(json) = run.report_json.as_deref() {
        if let Ok(report) = serde_json::from_str::<quality::report::QualityReport>(json) {
            for provider in &report.provider_reports {
                for (key, value) in &provider.metrics {
                    values.insert(*key, value.clone());
                }
            }
        } else {
            tracing::warn!(
                %project_id, run_id = %run.id,
                "quality_run.report_json failed to parse into QualityReport; snapshot values empty"
            );
        }
    }

    let ran_at = run
        .completed_at
        .map(|t| t.to_rfc3339())
        .or_else(|| Some(run.created_at.to_rfc3339()));

    Ok(Json(ApiResponse::success(ProjectMetricSnapshot {
        values,
        run_id: Some(run.id),
        ran_at,
    })))
}

/// GET /projects/{project_id}/quality-policy — DB-first resolved effective policy.
pub async fn get_project_policy(
    State(deployment): State<DeploymentImpl>,
    AxumPath(project_id): AxumPath<Uuid>,
) -> Result<Json<ApiResponse<QualityPolicyResponse>>, ApiError> {
    // Priority 0: DB project_quality_policy row.
    if let Some(policy) =
        ProjectQualityPolicy::find_by_project(&deployment.db().pool, project_id).await?
    {
        match QualityGateConfig::from_yaml(&policy.config_yaml) {
            Ok(config) => {
                return Ok(Json(ApiResponse::success(QualityPolicyResponse {
                    source: "project".to_string(),
                    config,
                })));
            }
            Err(e) => {
                tracing::warn!(
                    %project_id, error = %e,
                    "project_quality_policy YAML failed to parse — falling back to bundled"
                );
            }
        }
    }

    // No DB row (or corrupt YAML): the server has no project working dir here, so
    // resolve from the bundled central policy. Reported as "bundled". (Repo-local
    // "file" resolution happens at gate run time in the services resolver, where a
    // working directory is in scope.)
    let config = QualityGateConfig::from_yaml(BUNDLED_CENTRAL_POLICY)
        .map_err(|e| ApiError::Internal(format!("parse bundled policy: {e}")))?;
    Ok(Json(ApiResponse::success(QualityPolicyResponse {
        source: "bundled".to_string(),
        config,
    })))
}

/// PUT /projects/{project_id}/quality-policy — validate + upsert the override.
pub async fn put_project_policy(
    State(deployment): State<DeploymentImpl>,
    AxumPath(project_id): AxumPath<Uuid>,
    Json(config): Json<QualityGateConfig>,
) -> Result<Json<ApiResponse<QualityPolicyResponse>>, ApiError> {
    let errs = config.validate();
    if !errs.is_empty() {
        return Err(ApiError::BadRequest(format!(
            "Invalid quality policy: {}",
            errs.join("; ")
        )));
    }

    let yaml = serde_yaml::to_string(&config)
        .map_err(|e| ApiError::Internal(format!("serialize policy: {e}")))?;
    ProjectQualityPolicy::upsert(
        &deployment.db().pool,
        project_id,
        &yaml,
        mode_str(config.mode),
    )
    .await?;

    Ok(Json(ApiResponse::success(QualityPolicyResponse {
        source: "project".to_string(),
        config,
    })))
}

/// DELETE /projects/{project_id}/quality-policy — remove the override (reset to default).
pub async fn delete_project_policy(
    State(deployment): State<DeploymentImpl>,
    AxumPath(project_id): AxumPath<Uuid>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    ProjectQualityPolicy::delete(&deployment.db().pool, project_id).await?;
    Ok(Json(ApiResponse::success(())))
}

/// Quality policy catalog routes nested under /quality.
pub fn quality_policy_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/policy/default", get(get_default_policy))
        .route("/policy/metrics", get(get_metric_catalog))
}

/// Quality policy project CRUD routes nested under /projects.
pub fn quality_policy_project_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route(
            "/{project_id}/quality-policy",
            get(get_project_policy)
                .put(put_project_policy)
                .delete(delete_project_policy),
        )
        .route(
            "/{project_id}/quality-metrics/latest",
            get(get_project_metric_snapshot),
        )
}
