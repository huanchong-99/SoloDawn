//! Architecture knowledge API: manage GitHub knowledge sources, trigger
//! syncs, and inspect synced entries. Guidance injection itself happens at
//! planning-draft materialization (see `planning_drafts.rs`).

use axum::{
    Json, Router,
    extract::{Path, Query, State},
    routing::{get, post},
};
use db::models::architecture_entry::{ArchitectureEntry, ArchitectureEntrySummary};
use db::models::architecture_source::ArchitectureSource;
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use services::services::architecture_knowledge;
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError};

pub fn architecture_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/sources", get(list_sources).post(create_source))
        .route(
            "/sources/{source_id}",
            axum::routing::put(update_source).delete(delete_source),
        )
        .route("/sources/{source_id}/sync", post(sync_source_now))
        .route("/entries", get(list_entries))
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceResponse {
    pub id: String,
    pub name: String,
    pub owner: String,
    pub repo: String,
    pub branch: String,
    pub include_paths: Vec<String>,
    pub enabled: bool,
    pub builtin: bool,
    pub last_synced_at: Option<String>,
    pub last_sync_status: Option<String>,
    pub entry_count: i64,
}

impl SourceResponse {
    fn from_source(source: ArchitectureSource, entry_count: i64) -> Self {
        Self {
            include_paths: source.include_path_list(),
            id: source.id,
            name: source.name,
            owner: source.owner,
            repo: source.repo,
            branch: source.branch,
            enabled: source.enabled,
            builtin: source.builtin,
            last_synced_at: source.last_synced_at.map(|t| t.to_rfc3339()),
            last_sync_status: source.last_sync_status,
            entry_count,
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateSourceRequest {
    pub name: String,
    pub owner: String,
    pub repo: String,
    pub branch: Option<String>,
    pub include_paths: Option<Vec<String>>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSourceRequest {
    pub name: Option<String>,
    pub branch: Option<String>,
    pub include_paths: Option<Vec<String>>,
    pub enabled: Option<bool>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EntriesQuery {
    pub source_id: Option<String>,
}

fn valid_github_segment(s: &str) -> bool {
    !s.is_empty()
        && s.len() <= 100
        && s.chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | '.'))
}

async fn list_sources(
    State(deployment): State<DeploymentImpl>,
) -> Result<Json<ApiResponse<Vec<SourceResponse>>>, ApiError> {
    let pool = &deployment.db().pool;
    let sources = ArchitectureSource::find_all(pool)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to list sources: {e}")))?;
    let mut out = Vec::with_capacity(sources.len());
    for source in sources {
        let count = ArchitectureEntry::count_by_source(pool, &source.id)
            .await
            .unwrap_or(0);
        out.push(SourceResponse::from_source(source, count));
    }
    Ok(Json(ApiResponse::success(out)))
}

async fn create_source(
    State(deployment): State<DeploymentImpl>,
    Json(req): Json<CreateSourceRequest>,
) -> Result<Json<ApiResponse<SourceResponse>>, ApiError> {
    let pool = &deployment.db().pool;
    let name = req.name.trim();
    let owner = req.owner.trim();
    let repo = req.repo.trim();
    let branch = req.branch.as_deref().unwrap_or("main").trim().to_string();
    if name.is_empty() {
        return Err(ApiError::BadRequest("Source name is required".to_string()));
    }
    if !valid_github_segment(owner) || !valid_github_segment(repo) {
        return Err(ApiError::BadRequest(
            "Owner and repo must be valid GitHub identifiers".to_string(),
        ));
    }
    if branch.is_empty() || branch.len() > 200 {
        return Err(ApiError::BadRequest("Invalid branch name".to_string()));
    }
    if ArchitectureSource::find_by_coords(pool, owner, repo, &branch)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .is_some()
    {
        return Err(ApiError::BadRequest(format!(
            "Source {owner}/{repo}@{branch} already exists"
        )));
    }

    let include_paths: Vec<String> = req
        .include_paths
        .unwrap_or_else(|| vec!["templates/".to_string()])
        .into_iter()
        .map(|p| p.trim().to_string())
        .filter(|p| !p.is_empty())
        .collect();
    if include_paths.is_empty() {
        return Err(ApiError::BadRequest(
            "At least one include path is required".to_string(),
        ));
    }

    let include_refs: Vec<&str> = include_paths.iter().map(String::as_str).collect();
    let source = ArchitectureSource::new(name, owner, repo, &branch, &include_refs);
    ArchitectureSource::insert(pool, &source)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create source: {e}")))?;
    Ok(Json(ApiResponse::success(SourceResponse::from_source(
        source, 0,
    ))))
}

async fn update_source(
    State(deployment): State<DeploymentImpl>,
    Path(source_id): Path<String>,
    Json(req): Json<UpdateSourceRequest>,
) -> Result<Json<ApiResponse<SourceResponse>>, ApiError> {
    let pool = &deployment.db().pool;
    let source = ArchitectureSource::find_by_id(pool, &source_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::BadRequest("Source not found".to_string()))?;

    let name = req
        .name
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or(&source.name)
        .to_string();
    let branch = req
        .branch
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or(&source.branch)
        .to_string();
    let include_paths = match req.include_paths {
        Some(paths) => {
            let cleaned: Vec<String> = paths
                .into_iter()
                .map(|p| p.trim().to_string())
                .filter(|p| !p.is_empty())
                .collect();
            if cleaned.is_empty() {
                return Err(ApiError::BadRequest(
                    "At least one include path is required".to_string(),
                ));
            }
            serde_json::to_string(&cleaned)
                .map_err(|e| ApiError::Internal(format!("Failed to encode paths: {e}")))?
        }
        None => source.include_paths.clone(),
    };
    let enabled = req.enabled.unwrap_or(source.enabled);

    ArchitectureSource::update_settings(pool, &source_id, &name, &branch, &include_paths, enabled)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to update source: {e}")))?;

    let updated = ArchitectureSource::find_by_id(pool, &source_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Source vanished during update".to_string()))?;
    let count = ArchitectureEntry::count_by_source(pool, &source_id)
        .await
        .unwrap_or(0);
    Ok(Json(ApiResponse::success(SourceResponse::from_source(
        updated, count,
    ))))
}

async fn delete_source(
    State(deployment): State<DeploymentImpl>,
    Path(source_id): Path<String>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;
    let source = ArchitectureSource::find_by_id(pool, &source_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::BadRequest("Source not found".to_string()))?;
    if source.builtin {
        return Err(ApiError::BadRequest(
            "Builtin sources cannot be deleted; disable them instead".to_string(),
        ));
    }
    ArchitectureSource::delete_custom(pool, &source_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to delete source: {e}")))?;
    Ok(Json(ApiResponse::success(())))
}

async fn sync_source_now(
    State(deployment): State<DeploymentImpl>,
    Path(source_id): Path<String>,
) -> Result<Json<ApiResponse<SourceResponse>>, ApiError> {
    let pool = &deployment.db().pool;
    let source = ArchitectureSource::find_by_id(pool, &source_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::BadRequest("Source not found".to_string()))?;

    // Errors are recorded on the source row; the refreshed row carries the
    // outcome either way so the UI shows sync status uniformly.
    architecture_knowledge::sync_source_recorded(pool, &source).await;

    let refreshed = ArchitectureSource::find_by_id(pool, &source_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Source vanished during sync".to_string()))?;
    let count = ArchitectureEntry::count_by_source(pool, &source_id)
        .await
        .unwrap_or(0);
    Ok(Json(ApiResponse::success(SourceResponse::from_source(
        refreshed, count,
    ))))
}

async fn list_entries(
    State(deployment): State<DeploymentImpl>,
    Query(query): Query<EntriesQuery>,
) -> Result<Json<ApiResponse<Vec<ArchitectureEntrySummary>>>, ApiError> {
    let pool = &deployment.db().pool;
    let entries = ArchitectureEntry::list_summaries(pool, query.source_id.as_deref())
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to list entries: {e}")))?;
    Ok(Json(ApiResponse::success(entries)))
}
