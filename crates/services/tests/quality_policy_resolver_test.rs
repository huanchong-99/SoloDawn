//! Integration tests for the System-A quality-policy resolver (G3).
//!
//! Covers `services::services::orchestrator::quality_policy::resolve_quality_config`:
//! - DB `project_quality_policy` row present  → returns its parsed config (priority-0).
//! - DB row present but YAML is corrupt        → falls back to the file/bundled chain.
//! - DB row absent                             → falls back to the file/bundled chain.
//!
//! Harness mirrors `crates/services/tests/phase18_scenarios.rs::setup_db`: an
//! in-memory SQLite pool with the real migrations applied (so the new
//! `project_quality_policy` table from `20260614120000_create_project_quality_policy.sql`
//! exists).

use std::str::FromStr;

use db::models::project_quality_policy::ProjectQualityPolicy;
use quality::config::{QualityGateConfig, QualityGateMode};
use services::services::orchestrator::quality_policy::resolve_quality_config;
use sqlx::SqlitePool;
use sqlx::sqlite::{SqliteConnectOptions, SqlitePoolOptions};
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

/// Seed a `projects` row so the `project_quality_policy.project_id` FK is satisfied.
async fn seed_project(pool: &SqlitePool) -> Uuid {
    let project_id = Uuid::new_v4();
    sqlx::query("INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)")
        .bind(project_id)
        .bind("QG Resolver Test Project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(pool)
        .await
        .unwrap();
    project_id
}

/// A minimal valid `QualityGateConfig` that is byte-distinct from the default /
/// bundled policy: mode = Enforce and all gates empty. Round-trips through YAML.
fn distinct_config() -> QualityGateConfig {
    let mut cfg = QualityGateConfig::default_config();
    cfg.mode = QualityGateMode::Enforce;
    cfg.terminal_gate.conditions.clear();
    cfg.branch_gate.conditions.clear();
    cfg.repo_gate.conditions.clear();
    cfg.terminal_gate.name = "DB Policy Terminal".to_string();
    cfg
}

#[tokio::test]
async fn resolve_returns_db_policy_when_row_present() {
    let pool = setup_pool().await;
    let project_id = seed_project(&pool).await;

    // Seed a DB policy row with a config distinct from the default fallback.
    let cfg = distinct_config();
    let yaml = serde_yaml::to_string(&cfg).expect("serialize config to yaml");
    ProjectQualityPolicy::upsert(&pool, project_id, &yaml, "enforce")
        .await
        .expect("upsert policy");

    // No working dir → file chain would yield the bundled default (Shadow mode).
    // The DB row must take priority-0 and win.
    let resolved = resolve_quality_config(&pool, project_id, std::path::Path::new("/nonexistent")).await;

    assert_eq!(
        resolved.mode,
        QualityGateMode::Enforce,
        "DB policy (Enforce) must be returned, not the bundled fallback (Shadow)"
    );
    assert_eq!(resolved.terminal_gate.name, "DB Policy Terminal");
    assert!(
        resolved.terminal_gate.conditions.is_empty(),
        "DB policy gates were emptied; fallback would be non-empty"
    );
}

#[tokio::test]
async fn resolve_falls_back_when_db_yaml_corrupt() {
    let pool = setup_pool().await;
    let project_id = seed_project(&pool).await;

    // Seed a DB row whose config_yaml does NOT parse as a QualityGateConfig.
    ProjectQualityPolicy::upsert(&pool, project_id, "this: is: not: valid: yaml: [", "enforce")
        .await
        .expect("upsert corrupt policy");
    assert!(
        QualityGateConfig::from_yaml("this: is: not: valid: yaml: [").is_err(),
        "test precondition: stored YAML must be unparseable"
    );

    // Corrupt DB row must NOT crash; it falls through to the file/bundled chain.
    // With a nonexistent project_root, load_from_project yields the bundled
    // central policy (a valid, non-empty config).
    let resolved = resolve_quality_config(&pool, project_id, std::path::Path::new("/nonexistent")).await;

    assert!(
        !resolved.terminal_gate.conditions.is_empty(),
        "fallback (bundled/default) terminal gate is non-empty"
    );

    // The corrupt row was NOT honored: the resolved config must equal the engine's
    // own filesystem→bundled→default fallback, not the (unparseable) DB content.
    let expected = QualityGateConfig::load_from_project(std::path::Path::new("/nonexistent"))
        .unwrap_or_else(|_| QualityGateConfig::default_config());
    assert_eq!(
        resolved.mode, expected.mode,
        "corrupt DB row must be ignored in favor of the engine fallback chain"
    );
    assert_eq!(
        resolved.terminal_gate.name, expected.terminal_gate.name,
        "resolved terminal gate must come from the fallback, not the corrupt DB row"
    );
    // And it must NOT be the distinct DB-row marker name from the valid-row test.
    assert_ne!(resolved.terminal_gate.name, "DB Policy Terminal");
}

#[tokio::test]
async fn resolve_falls_back_when_db_row_absent() {
    let pool = setup_pool().await;
    let project_id = Uuid::new_v4();

    // No DB row for this project → must use the file/bundled fallback chain.
    let resolved = resolve_quality_config(&pool, project_id, std::path::Path::new("/nonexistent")).await;

    // Bundled central policy / default_config both have a non-empty terminal gate.
    assert!(
        !resolved.terminal_gate.conditions.is_empty(),
        "absent DB row must fall back to a non-empty bundled/default config"
    );

    // And it must match the same fallback the engine's own loader produces.
    let expected = QualityGateConfig::load_from_project(std::path::Path::new("/nonexistent"))
        .unwrap_or_else(|_| QualityGateConfig::default_config());
    assert_eq!(
        resolved.terminal_gate.conditions.len(),
        expected.terminal_gate.conditions.len(),
        "resolver fallback must equal the engine's load_from_project chain"
    );
    assert_eq!(resolved.mode, expected.mode);
}
