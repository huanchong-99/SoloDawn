//! Quality Policy Snapshot Model
//!
//! Persists a snapshot of the quality gate configuration at the time of each
//! quality run. This enables audit trails and reproducible gate decisions,
//! even if the live configuration changes later.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool};
use ts_rs::TS;
use uuid::Uuid;

/// Quality Policy Snapshot
///
/// Corresponds to database table: quality_policy_snapshot
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct QualityPolicySnapshot {
    pub id: String,
    pub quality_run_id: String,
    pub config_yaml: String,
    /// shadow | warn | enforce | off
    pub mode: String,
    /// terminal | branch | repo
    pub tier: String,
    /// JSON array of provider configurations
    pub providers_json: Option<String>,
    /// JSON object of threshold settings
    pub thresholds_json: Option<String>,
    pub created_at: DateTime<Utc>,
}

impl QualityPolicySnapshot {
    /// Create a new snapshot instance (not yet persisted)
    pub fn new(
        quality_run_id: &str,
        config_yaml: &str,
        mode: &str,
        tier: &str,
        providers_json: Option<&str>,
        thresholds_json: Option<&str>,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            quality_run_id: quality_run_id.to_string(),
            config_yaml: config_yaml.to_string(),
            mode: mode.to_string(),
            tier: tier.to_string(),
            providers_json: providers_json.map(std::string::ToString::to_string),
            thresholds_json: thresholds_json.map(std::string::ToString::to_string),
            created_at: Utc::now(),
        }
    }

    /// Insert a new quality policy snapshot record
    pub async fn insert(pool: &SqlitePool, snapshot: &QualityPolicySnapshot) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO quality_policy_snapshot (
                id, quality_run_id, config_yaml, mode, tier,
                providers_json, thresholds_json, created_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
        )
        .bind(&snapshot.id)
        .bind(&snapshot.quality_run_id)
        .bind(&snapshot.config_yaml)
        .bind(&snapshot.mode)
        .bind(&snapshot.tier)
        .bind(&snapshot.providers_json)
        .bind(&snapshot.thresholds_json)
        .bind(snapshot.created_at)
        .execute(pool)
        .await?;
        Ok(())
    }

    /// Find the policy snapshot for a given quality run
    pub async fn find_by_run_id(
        pool: &SqlitePool,
        quality_run_id: &str,
    ) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, QualityPolicySnapshot>(
            r"SELECT * FROM quality_policy_snapshot WHERE quality_run_id = ?",
        )
        .bind(quality_run_id)
        .fetch_optional(pool)
        .await
    }
}
