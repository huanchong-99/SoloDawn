//! Integration test for the declarative custom-rule enforcement link (PRD §14).
//!
//! Covers `services::services::orchestrator::quality_policy::build_engine_for_project`:
//! the seam that loads a project's enabled `custom_rule` rows, compiles them, and
//! injects a `DeclarativeRuleProvider` into the gate-ready engine **only when the
//! project's policy opts in** (`providers.declarative_rules = true`).
//!
//! - Positive: toggle ON + one enabled regex rule + a temp file that violates it
//!   → the custom-rule violation surfaces (`CustomRuleViolations` measure > 0 AND
//!   an issue whose source is `AnalyzerSource::CustomRule`).
//! - Negative (toggle OFF): same row + file, but the policy does not opt in → no
//!   declarative provider participates and no custom issue appears.
//! - Negative (no enabled rule): toggle ON but the only rule is disabled → the
//!   provider is absent (no rules to run).
//!
//! Harness mirrors `crates/services/tests/quality_policy_resolver_test.rs`: an
//! in-memory SQLite pool with the real migrations applied (FK off for easy
//! seeding), `#[serial]`-free per the resolver test's pattern (each test owns its
//! own pool, so there is no shared-DB collision).

use std::str::FromStr;

use db::models::project_quality_policy::ProjectQualityPolicy;
use quality::config::{QualityGateConfig, QualityGateMode};
use quality::gate::QualityGateLevel;
use quality::metrics::MetricKey;
use quality::rule::AnalyzerSource;
use services::services::orchestrator::quality_policy::build_engine_for_project;
use sqlx::sqlite::{SqliteConnectOptions, SqlitePoolOptions};
use sqlx::SqlitePool;
use uuid::Uuid;

/// Build an in-memory pool with all migrations applied (FK off for easy seeding).
async fn setup_pool() -> SqlitePool {
    let options = SqliteConnectOptions::from_str(":memory:")
        .unwrap()
        .pragma("foreign_keys", "OFF");

    let pool = SqlitePoolOptions::new()
        .max_connections(1)
        .connect_with(options)
        .await
        .unwrap();

    let migrator = sqlx::migrate!("../db/migrations");
    migrator.run(&pool).await.unwrap();
    pool
}

/// Seed a `projects` row so the `custom_rule.project_id` FK is satisfied.
async fn seed_project(pool: &SqlitePool) -> Uuid {
    let project_id = Uuid::new_v4();
    sqlx::query("INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)")
        .bind(project_id)
        .bind("Custom Rule Gate Test Project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(pool)
        .await
        .unwrap();
    project_id
}

/// Insert one custom rule via the real `CustomRule::create` path, then set its
/// `enabled` flag and lifecycle `status` so it is (or is not) part of the
/// enforced set `find_enabled_by_project` returns.
async fn seed_custom_rule(
    pool: &SqlitePool,
    project_id: Uuid,
    pattern: &str,
    enabled: bool,
) -> Uuid {
    let created = db::models::CustomRule::create(
        pool,
        &db::models::CreateCustomRule {
            project_id: Some(project_id),
            name: "no-dbg".to_string(),
            nl_request: "forbid dbg! in committed code".to_string(),
            // The migration CHECK accepts 'regex'; rule_body IS the matcher
            // pattern for a regex rule (same as the create route + pipeline).
            rule_format: "regex".to_string(),
            rule_body: pattern.to_string(),
            description: Some("dbg! macro left in source".to_string()),
            rule_type: "CodeSmell".to_string(),
            severity: "MAJOR".to_string(),
            mapped_metric: None,
            created_by: Some("test".to_string()),
        },
    )
    .await
    .expect("create custom rule");

    // `create` defaults enabled=1, status='shadow' (a non-draft, non-disabled
    // state → in the enabled set). Flip enabled off for the negative case.
    if !enabled {
        db::models::CustomRule::set_enabled(pool, created.id, false)
            .await
            .expect("disable rule");
    }
    created.id
}

/// Write the project's quality policy with the declarative-rules toggle in the
/// requested state. Mode = Shadow so analysis runs (providers always analyze
/// outside `Off`) without the gate needing built-in providers to be applicable.
async fn seed_policy_with_toggle(pool: &SqlitePool, project_id: Uuid, declarative_rules: bool) {
    let mut cfg = QualityGateConfig::default_config();
    cfg.mode = QualityGateMode::Shadow;
    cfg.providers.declarative_rules = declarative_rules;
    let yaml = serde_yaml::to_string(&cfg).expect("serialize policy");
    ProjectQualityPolicy::upsert(pool, project_id, &yaml, "shadow")
        .await
        .expect("upsert policy");
}

/// A temp project root containing a single Rust source file that trips the rule.
fn temp_project_with_violation() -> tempfile::TempDir {
    let dir = tempfile::tempdir().expect("temp dir");
    std::fs::create_dir_all(dir.path().join("src")).expect("src dir");
    std::fs::write(
        dir.path().join("src").join("main.rs"),
        "fn main() {\n    let _ = dbg!(40 + 2);\n}\n",
    )
    .expect("write source");
    dir
}

#[tokio::test]
async fn declarative_rules_on_surfaces_custom_violation() {
    let pool = setup_pool().await;
    let project_id = seed_project(&pool).await;

    // Literal `dbg!(` ban (the PRD's trivial example), enabled, toggle ON.
    seed_custom_rule(&pool, project_id, r"dbg!\(", true).await;
    seed_policy_with_toggle(&pool, project_id, true).await;

    let dir = temp_project_with_violation();
    let engine = build_engine_for_project(&pool, project_id, dir.path())
        .await
        .expect("build engine");

    let report = engine
        .run(dir.path(), QualityGateLevel::Terminal, None)
        .await
        .expect("run gate");

    // The published count metric must show the violation.
    let violations = report
        .provider_reports
        .iter()
        .find_map(|pr| pr.metrics.get(&MetricKey::CustomRuleViolations))
        .cloned();
    assert_eq!(
        violations,
        Some(quality::gate::result::MeasureValue::Int(1)),
        "exactly one custom-rule violation expected; got {violations:?}"
    );

    // And an actual issue attributed to the custom-rule source must exist.
    assert!(
        report
            .all_issues
            .iter()
            .any(|i| i.source == AnalyzerSource::CustomRule),
        "expected at least one issue whose source is AnalyzerSource::CustomRule; \
         issues = {:?}",
        report.all_issues
    );
}

#[tokio::test]
async fn declarative_rules_off_yields_no_custom_violation() {
    let pool = setup_pool().await;
    let project_id = seed_project(&pool).await;

    // Same enabled rule + same violating file, but the policy does NOT opt in.
    seed_custom_rule(&pool, project_id, r"dbg!\(", true).await;
    seed_policy_with_toggle(&pool, project_id, false).await;

    let dir = temp_project_with_violation();
    let engine = build_engine_for_project(&pool, project_id, dir.path())
        .await
        .expect("build engine");

    let report = engine
        .run(dir.path(), QualityGateLevel::Terminal, None)
        .await
        .expect("run gate");

    // No declarative provider was injected → the metric is absent entirely.
    let has_metric = report
        .provider_reports
        .iter()
        .any(|pr| pr.metrics.contains_key(&MetricKey::CustomRuleViolations));
    assert!(
        !has_metric,
        "toggle OFF must not publish CustomRuleViolations"
    );
    assert!(
        report
            .all_issues
            .iter()
            .all(|i| i.source != AnalyzerSource::CustomRule),
        "toggle OFF must not surface any custom-rule issue"
    );
}

#[tokio::test]
async fn declarative_rules_on_but_no_enabled_rule_is_absent() {
    let pool = setup_pool().await;
    let project_id = seed_project(&pool).await;

    // Toggle ON, but the only rule is DISABLED → not in the enforced set, so the
    // provider is never injected (no rules to run).
    seed_custom_rule(&pool, project_id, r"dbg!\(", false).await;
    seed_policy_with_toggle(&pool, project_id, true).await;

    let dir = temp_project_with_violation();
    let engine = build_engine_for_project(&pool, project_id, dir.path())
        .await
        .expect("build engine");

    let report = engine
        .run(dir.path(), QualityGateLevel::Terminal, None)
        .await
        .expect("run gate");

    let has_metric = report
        .provider_reports
        .iter()
        .any(|pr| pr.metrics.contains_key(&MetricKey::CustomRuleViolations));
    assert!(
        !has_metric,
        "no enabled rule must not publish CustomRuleViolations"
    );
    assert!(
        report
            .all_issues
            .iter()
            .all(|i| i.source != AnalyzerSource::CustomRule),
        "no enabled rule must not surface any custom-rule issue"
    );
}
