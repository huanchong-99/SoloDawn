//! System-A quality-gate policy resolver (DB priority-0).
//!
//! The `quality` crate has no `db` dependency and the `db` crate has no
//! `quality` dependency; this `services`-layer resolver is the only place that
//! bridges the two. It reads the per-project policy override from the database
//! first and falls back to the engine's existing filesystem→bundled→default
//! chain, returning a plain `Send` `QualityGateConfig` that can be handed to
//! `QualityEngine::from_config` (which stays pool-free and file-write-free).

use std::path::Path;

use quality::config::QualityGateConfig;
use quality::engine::QualityEngine;
use quality::provider::{compile, CompiledRule, DeclarativeRuleProvider, RuleDefinition, RuleFormat};
use quality::rule::{RuleType, Severity};
use sqlx::SqlitePool;
use std::sync::Arc;
use uuid::Uuid;

/// Resolve the effective System-A quality config for a project + working dir.
///
/// Priority:
/// 0. DB `project_quality_policy` row (parsed via `QualityGateConfig::from_yaml`)
/// 1. repo-local `quality-gate.yaml` (`load_from_project`)
/// 2. bundled central policy (handled inside `load_from_project`)
/// 3. `default_config()`
///
/// Always re-read at gate run time so mid-run G3 edits are honored. A DB row
/// whose YAML fails to parse logs a warning and falls through to the file chain.
pub async fn resolve_quality_config(
    pool: &SqlitePool,
    project_id: Uuid,
    project_root: &Path,
) -> QualityGateConfig {
    if let Ok(Some(policy)) =
        db::models::project_quality_policy::ProjectQualityPolicy::find_by_project(pool, project_id)
            .await
    {
        match QualityGateConfig::from_yaml(&policy.config_yaml) {
            Ok(cfg) => return cfg,
            Err(e) => tracing::warn!(
                %project_id,
                error = %e,
                "project_quality_policy YAML failed to parse — falling back to file/bundled chain"
            ),
        }
    }

    // Fallback chain identical to non-orchestrated callers.
    QualityGateConfig::load_from_project(project_root)
        .unwrap_or_else(|_| QualityGateConfig::default_config())
}

/// Build a gate-ready [`QualityEngine`] for a project, wiring in any authored
/// declarative custom rules (PRD §14 — the enforcement link).
///
/// This is the single DB<->engine bridge for a gate run: it resolves the
/// effective config via [`resolve_quality_config`], constructs the engine from
/// it (`QualityEngine::from_config`, which builds the toggle-driven provider
/// set), and — **only when `providers.declarative_rules` is enabled** — loads the
/// project's enabled `custom_rule` rows, compiles each into a
/// [`quality::provider::CompiledRule`], and pushes a
/// [`DeclarativeRuleProvider`] onto the engine so authored rules actually run.
///
/// Defensive by design (the admission gate already proved every persisted rule
/// compiles): a row that fails to map or compile is logged at `warn` and
/// skipped, never failing the whole gate. If the toggle is off or no enabled
/// rule survives compilation, no declarative provider is added and behavior is
/// byte-identical to the pre-§14 path.
///
/// `Send` + pool-aware so both orchestrator gate-run sites can `.await` it.
pub async fn build_engine_for_project(
    pool: &SqlitePool,
    project_id: Uuid,
    project_root: &Path,
) -> anyhow::Result<QualityEngine> {
    let config = resolve_quality_config(pool, project_id, project_root).await;
    let declarative_enabled = config.providers.declarative_rules;
    let mut engine = QualityEngine::from_config(config, project_root)?;

    if declarative_enabled {
        let rules = load_compiled_custom_rules(pool, project_id).await;
        if !rules.is_empty() {
            tracing::info!(
                %project_id,
                rule_count = rules.len(),
                "Declarative custom rules enabled — injecting DeclarativeRuleProvider into the gate"
            );
            engine.push_provider(Arc::new(DeclarativeRuleProvider::new(rules)));
        }
    }

    Ok(engine)
}

/// Load the project's enabled `custom_rule` rows and compile them into the
/// `quality` crate's runnable form. Any row that cannot be mapped to a supported
/// format or fails to compile is logged and dropped (defensive — the admission
/// gate guarantees compilability for persisted rules; this is belt-and-braces so
/// one bad row can never break a gate run). DB read failures degrade to "no
/// custom rules", never an error.
async fn load_compiled_custom_rules(pool: &SqlitePool, project_id: Uuid) -> Vec<CompiledRule> {
    let rows = match db::models::CustomRule::find_enabled_by_project(pool, project_id).await {
        Ok(rows) => rows,
        Err(e) => {
            tracing::warn!(
                %project_id,
                error = %e,
                "Failed to load enabled custom_rule rows — skipping declarative rules for this gate run"
            );
            return Vec::new();
        }
    };

    let mut compiled = Vec::with_capacity(rows.len());
    for row in &rows {
        let def = match db_rule_to_definition(row) {
            Ok(def) => def,
            Err(reason) => {
                tracing::warn!(
                    rule_id = %row.id,
                    rule_name = %row.name,
                    reason,
                    "Skipping custom rule: cannot map row to a runnable definition"
                );
                continue;
            }
        };
        match compile(&def) {
            Ok(rule) => compiled.push(rule),
            Err(e) => tracing::warn!(
                rule_id = %row.id,
                rule_name = %row.name,
                error = %e,
                "Skipping custom rule: compile failed (admission gate should have caught this)"
            ),
        }
    }
    compiled
}

/// Map a persisted `custom_rule` row to the DB-free [`RuleDefinition`] the
/// quality crate compiles.
///
/// Mirrors the column->definition mapping the create/authoring path uses
/// (`server::routes::custom_rules` / `rule_authoring::pipeline`): `rule_body`
/// IS the matcher pattern for a `regex` rule, `rule_format`/`rule_type` are the
/// PascalCase/lowercase CHECK tokens, and `severity` is the uppercase token.
/// The persisted row carries no per-file scope columns (D4: scope is
/// project-only), so `languages`/`extensions`/`include_globs`/`exclude_globs`
/// are empty — the rule applies to every collected source file. `message` has no
/// column either, so the row's `description` (else its `name`) is used.
fn db_rule_to_definition(row: &db::models::CustomRule) -> Result<RuleDefinition, &'static str> {
    let rule_format = parse_rule_format(&row.rule_format)?;
    let rule_type = parse_rule_type(&row.rule_type)?;
    let severity = Severity::from_sonar_str(&row.severity).ok_or("unknown severity token")?;
    let message = row
        .description
        .clone()
        .filter(|d| !d.trim().is_empty())
        .unwrap_or_else(|| row.name.clone());

    Ok(RuleDefinition {
        rule_id: row.id.to_string(),
        name: row.name.clone(),
        rule_format,
        pattern: row.rule_body.clone(),
        severity,
        rule_type,
        message,
        languages: Vec::new(),
        extensions: Vec::new(),
        include_globs: Vec::new(),
        exclude_globs: Vec::new(),
    })
}

/// Parse the `custom_rule.rule_format` CHECK token (`'regex'` | `'ast_grep'`).
fn parse_rule_format(token: &str) -> Result<RuleFormat, &'static str> {
    match token {
        "regex" => Ok(RuleFormat::Regex),
        "ast_grep" => Ok(RuleFormat::AstGrep),
        _ => Err("unknown rule_format token"),
    }
}

/// Parse the `custom_rule.rule_type` CHECK token (PascalCase) into [`RuleType`].
fn parse_rule_type(token: &str) -> Result<RuleType, &'static str> {
    match token {
        "Bug" => Ok(RuleType::Bug),
        "Vulnerability" => Ok(RuleType::Vulnerability),
        "CodeSmell" => Ok(RuleType::CodeSmell),
        "SecurityHotspot" => Ok(RuleType::SecurityHotspot),
        _ => Err("unknown rule_type token"),
    }
}
