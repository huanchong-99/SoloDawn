//! Design styles API: builtin presets (license-attributed, seeded at startup)
//! plus user-defined styles. Builtin rows accept only the `enabled` flag;
//! users duplicate a preset into a custom style to modify its content.

use axum::{
    Json, Router,
    extract::{Path, State},
    routing::get,
};
use db::models::design_style::DesignStyle;
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use utils::response::ApiResponse;

use crate::{DeploymentImpl, error::ApiError};

/// Keep style prompts well under SQLite/text sanity bounds.
const MAX_CONTENT_BYTES: usize = 64 * 1024;

pub fn design_style_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/", get(list_styles).post(create_style))
        .route(
            "/{style_id}",
            axum::routing::put(update_style).delete(delete_style),
        )
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StyleResponse {
    pub id: String,
    pub slug: String,
    pub name: String,
    pub description: String,
    pub content: String,
    pub source_name: Option<String>,
    pub source_url: Option<String>,
    pub license: Option<String>,
    pub builtin: bool,
    pub enabled: bool,
    pub created_at: String,
    pub updated_at: String,
}

impl From<DesignStyle> for StyleResponse {
    fn from(s: DesignStyle) -> Self {
        Self {
            id: s.id,
            slug: s.slug,
            name: s.name,
            description: s.description,
            content: s.content,
            source_name: s.source_name,
            source_url: s.source_url,
            license: s.license,
            builtin: s.builtin,
            enabled: s.enabled,
            created_at: s.created_at.to_rfc3339(),
            updated_at: s.updated_at.to_rfc3339(),
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateStyleRequest {
    pub name: String,
    pub slug: Option<String>,
    pub description: Option<String>,
    pub content: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateStyleRequest {
    pub name: Option<String>,
    pub description: Option<String>,
    pub content: Option<String>,
    pub enabled: Option<bool>,
}

/// Lowercase kebab slug from a display name; used when no slug is supplied.
fn slugify(name: &str) -> String {
    let mut slug = String::new();
    let mut last_dash = true;
    for c in name.chars() {
        if c.is_ascii_alphanumeric() {
            slug.push(c.to_ascii_lowercase());
            last_dash = false;
        } else if !last_dash {
            slug.push('-');
            last_dash = true;
        }
    }
    let slug = slug.trim_matches('-').to_string();
    if slug.len() >= 2 {
        slug
    } else {
        format!("style-{}", &uuid::Uuid::new_v4().to_string()[..8])
    }
}

fn valid_slug(slug: &str) -> bool {
    slug.len() >= 2
        && slug.len() <= 64
        && slug
            .chars()
            .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
        && !slug.starts_with('-')
        && !slug.ends_with('-')
}

fn validate_content(content: &str) -> Result<(), ApiError> {
    if content.trim().is_empty() {
        return Err(ApiError::BadRequest(
            "Style content is required".to_string(),
        ));
    }
    if content.len() > MAX_CONTENT_BYTES {
        return Err(ApiError::BadRequest(format!(
            "Style content exceeds {MAX_CONTENT_BYTES} bytes"
        )));
    }
    Ok(())
}

async fn list_styles(
    State(deployment): State<DeploymentImpl>,
) -> Result<Json<ApiResponse<Vec<StyleResponse>>>, ApiError> {
    let styles = DesignStyle::find_all(&deployment.db().pool)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to list design styles: {e}")))?;
    Ok(Json(ApiResponse::success(
        styles.into_iter().map(StyleResponse::from).collect(),
    )))
}

async fn create_style(
    State(deployment): State<DeploymentImpl>,
    Json(req): Json<CreateStyleRequest>,
) -> Result<Json<ApiResponse<StyleResponse>>, ApiError> {
    let pool = &deployment.db().pool;
    let name = req.name.trim();
    if name.is_empty() {
        return Err(ApiError::BadRequest("Style name is required".to_string()));
    }
    validate_content(&req.content)?;

    let slug = match req.slug.as_deref().map(str::trim).filter(|s| !s.is_empty()) {
        Some(explicit) => {
            if !valid_slug(explicit) {
                return Err(ApiError::BadRequest(
                    "Slug must be 2-64 chars of lowercase letters, digits and dashes".to_string(),
                ));
            }
            explicit.to_string()
        }
        None => slugify(name),
    };
    if DesignStyle::find_by_slug(pool, &slug)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .is_some()
    {
        return Err(ApiError::BadRequest(format!(
            "A style with slug '{slug}' already exists"
        )));
    }

    let style = DesignStyle::new(
        &slug,
        name,
        req.description.as_deref().unwrap_or("").trim(),
        req.content.trim(),
    );
    DesignStyle::insert(pool, &style)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create style: {e}")))?;
    Ok(Json(ApiResponse::success(style.into())))
}

async fn update_style(
    State(deployment): State<DeploymentImpl>,
    Path(style_id): Path<String>,
    Json(req): Json<UpdateStyleRequest>,
) -> Result<Json<ApiResponse<StyleResponse>>, ApiError> {
    let pool = &deployment.db().pool;
    let style = DesignStyle::find_by_id(pool, &style_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::BadRequest("Style not found".to_string()))?;

    if style.builtin {
        // Builtin presets: only the enabled flag is mutable.
        if req.name.is_some() || req.description.is_some() || req.content.is_some() {
            return Err(ApiError::BadRequest(
                "Builtin styles cannot be edited; duplicate into a custom style instead"
                    .to_string(),
            ));
        }
        if let Some(enabled) = req.enabled {
            DesignStyle::set_enabled(pool, &style_id, enabled)
                .await
                .map_err(|e| ApiError::Internal(format!("Failed to update style: {e}")))?;
        }
    } else {
        let name = req
            .name
            .as_deref()
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .unwrap_or(&style.name)
            .to_string();
        let description = req
            .description
            .as_deref()
            .map(str::trim)
            .unwrap_or(&style.description)
            .to_string();
        let content = match req.content.as_deref() {
            Some(c) => {
                validate_content(c)?;
                c.trim().to_string()
            }
            None => style.content.clone(),
        };
        let enabled = req.enabled.unwrap_or(style.enabled);
        DesignStyle::update_custom(pool, &style_id, &name, &description, &content, enabled)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to update style: {e}")))?;
    }

    let updated = DesignStyle::find_by_id(pool, &style_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Style vanished during update".to_string()))?;
    Ok(Json(ApiResponse::success(updated.into())))
}

async fn delete_style(
    State(deployment): State<DeploymentImpl>,
    Path(style_id): Path<String>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;
    let style = DesignStyle::find_by_id(pool, &style_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::BadRequest("Style not found".to_string()))?;
    if style.builtin {
        return Err(ApiError::BadRequest(
            "Builtin styles cannot be deleted; disable them instead".to_string(),
        ));
    }
    DesignStyle::delete_custom(pool, &style_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to delete style: {e}")))?;
    Ok(Json(ApiResponse::success(())))
}
