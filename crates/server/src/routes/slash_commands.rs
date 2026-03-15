//! Slash Command Preset API Routes

use axum::{
    Json, Router,
    extract::{Path, State},
    response::Json as ResponseJson,
    routing::get,
};
use chrono::Utc;
use db::models::SlashCommandPreset;
use deployment::Deployment;
use serde::Deserialize;
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

// ============================================================================
// Request/Response Types
// ============================================================================

/// Create Slash Command Preset Request
#[derive(Debug, Deserialize)]
pub struct CreateSlashCommandRequest {
    #[serde(rename = "command")]
    pub command: String,
    #[serde(rename = "description")]
    pub description: String,
    #[serde(rename = "promptTemplate")]
    pub prompt_template: Option<String>,
}

/// Update Slash Command Preset Request
#[derive(Debug, Deserialize)]
pub struct UpdateSlashCommandRequest {
    #[serde(rename = "description")]
    pub description: Option<String>,
    #[serde(rename = "promptTemplate")]
    pub prompt_template: Option<String>,
}

// ============================================================================
// Route Handlers
// ============================================================================

/// List all slash command presets
///
/// GET /api/workflows/presets/commands
pub async fn list_command_presets(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<SlashCommandPreset>>>, ApiError> {
    let presets = SlashCommandPreset::find_all(&deployment.db().pool)
        .await
        .map_err(|e| {
            ApiError::Internal(format!("Failed to fetch command presets: {e}"))
        })?;

    Ok(Json(ApiResponse::success(presets)))
}

/// Get a single slash command preset by ID
///
/// GET /api/workflows/presets/commands/:id
pub async fn get_command_preset(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
) -> Result<ResponseJson<ApiResponse<SlashCommandPreset>>, ApiError> {
    let preset =
        sqlx::query_as::<_, SlashCommandPreset>("SELECT * FROM slash_command_preset WHERE id = ?")
            .bind(&id)
            .fetch_optional(&deployment.db().pool)
            .await
            .map_err(|e| {
                ApiError::Internal(format!("Failed to fetch command preset: {e}"))
            })?
            .ok_or_else(|| ApiError::NotFound(format!("Command preset not found: {id}")))?;

    Ok(Json(ApiResponse::success(preset)))
}

/// Create a new slash command preset
///
/// POST /api/workflows/presets/commands
pub async fn create_command_preset(
    State(deployment): State<DeploymentImpl>,
    Json(req): Json<CreateSlashCommandRequest>,
) -> Result<ResponseJson<ApiResponse<SlashCommandPreset>>, ApiError> {
    // Validate: command must start with /
    if !req.command.starts_with('/') {
        return Err(ApiError::BadRequest(
            "Command must start with '/'".to_string(),
        ));
    }

    // Validate: description is required
    if req.description.trim().is_empty() {
        return Err(ApiError::BadRequest("Description is required".to_string()));
    }

    let id = format!("cmd-{}", Uuid::new_v4());
    let now = Utc::now();

    let preset = SlashCommandPreset {
        id: id.clone(),
        command: req.command,
        description: req.description,
        prompt_template: req.prompt_template,
        is_system: false,
        created_at: now,
        updated_at: now,
    };

    // Insert into database
    sqlx::query(
        r"
        INSERT INTO slash_command_preset (id, command, description, prompt_template, is_system, created_at, updated_at)
        VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
        "
    )
    .bind(&preset.id)
    .bind(&preset.command)
    .bind(&preset.description)
    .bind(&preset.prompt_template)
    .bind(preset.is_system)
    .bind(preset.created_at)
    .bind(preset.updated_at)
    .execute(&deployment.db().pool)
    .await
    .map_err(|e| {
        // Check for unique constraint violation
        if e.to_string().contains("UNIQUE constraint failed") {
            ApiError::Conflict(format!("Command '{}' already exists", preset.command))
        } else {
            ApiError::Internal(format!("Failed to create command preset: {e}"))
        }
    })?;

    Ok(Json(ApiResponse::success(preset)))
}

/// Update a slash command preset
///
/// PUT /api/workflows/presets/commands/:id
pub async fn update_command_preset(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
    Json(req): Json<UpdateSlashCommandRequest>,
) -> Result<ResponseJson<ApiResponse<SlashCommandPreset>>, ApiError> {
    // First fetch the existing preset
    let existing =
        sqlx::query_as::<_, SlashCommandPreset>("SELECT * FROM slash_command_preset WHERE id = ?")
            .bind(&id)
            .fetch_optional(&deployment.db().pool)
            .await
            .map_err(|e| {
                ApiError::Internal(format!("Failed to fetch command preset: {e}"))
            })?
            .ok_or_else(|| ApiError::NotFound(format!("Command preset not found: {id}")))?;

    // Don't allow modifying system presets
    if existing.is_system {
        return Err(ApiError::Forbidden(
            "Cannot modify system built-in commands".to_string(),
        ));
    }

    let now = Utc::now();

    // Build update query dynamically based on provided fields
    let description = req
        .description
        .unwrap_or_else(|| existing.description.clone());
    // `None` preserves the existing value; sending an empty string clears the field.
    let prompt_template = match req.prompt_template {
        Some(s) if s.trim().is_empty() => None,
        Some(s) => Some(s),
        None => existing.prompt_template.clone(),
    };

    sqlx::query(
        r"
        UPDATE slash_command_preset
        SET description = ?1, prompt_template = ?2, updated_at = ?3
        WHERE id = ?4
        ",
    )
    .bind(&description)
    .bind(&prompt_template)
    .bind(now)
    .bind(&id)
    .execute(&deployment.db().pool)
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to update command preset: {e}")))?;

    // Fetch and return the updated preset
    let updated =
        sqlx::query_as::<_, SlashCommandPreset>("SELECT * FROM slash_command_preset WHERE id = ?")
            .bind(&id)
            .fetch_one(&deployment.db().pool)
            .await
            .map_err(|e| {
                ApiError::Internal(format!("Failed to fetch updated command preset: {e}"))
            })?;

    Ok(Json(ApiResponse::success(updated)))
}

/// Delete a slash command preset
///
/// DELETE /api/workflows/presets/commands/:id
pub async fn delete_command_preset(
    State(deployment): State<DeploymentImpl>,
    Path(id): Path<String>,
) -> Result<ResponseJson<ApiResponse<serde_json::Value>>, ApiError> {
    // First fetch the existing preset to check if it's a system preset
    let existing =
        sqlx::query_as::<_, SlashCommandPreset>("SELECT * FROM slash_command_preset WHERE id = ?")
            .bind(&id)
            .fetch_optional(&deployment.db().pool)
            .await
            .map_err(|e| {
                ApiError::Internal(format!("Failed to fetch command preset: {e}"))
            })?
            .ok_or_else(|| ApiError::NotFound(format!("Command preset not found: {id}")))?;

    // Don't allow deleting system presets
    if existing.is_system {
        return Err(ApiError::Forbidden(
            "Cannot delete system built-in commands".to_string(),
        ));
    }

    // Delete the preset
    let result = sqlx::query("DELETE FROM slash_command_preset WHERE id = ?")
        .bind(&id)
        .execute(&deployment.db().pool)
        .await
        .map_err(|e| {
            ApiError::Internal(format!("Failed to delete command preset: {e}"))
        })?;

    if result.rows_affected() == 0 {
        return Err(ApiError::NotFound(format!(
            "Command preset not found: {id}"
        )));
    }

    Ok(Json(ApiResponse::success(
        serde_json::json!({"deleted": id}),
    )))
}

// ============================================================================
// Route Definition
// ============================================================================

/// Create slash commands router
pub fn slash_commands_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route(
            "/presets/commands",
            get(list_command_presets).post(create_command_preset),
        )
        .route(
            "/presets/commands/{id}",
            get(get_command_preset)
                .put(update_command_preset)
                .delete(delete_command_preset),
        )
}
