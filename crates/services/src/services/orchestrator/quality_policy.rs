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
use sqlx::SqlitePool;
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
