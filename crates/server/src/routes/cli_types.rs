//! CLI Type API Routes

use std::{
    collections::{HashMap, HashSet},
    sync::{Arc, Mutex},
};

use axum::{
    Router,
    extract::{
        Path, Query, State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    http::StatusCode,
    response::{IntoResponse, Json as ResponseJson},
    routing::{get, post, put},
};
use chrono::{DateTime, Utc};
use db::models::{CliDetectionStatus, CliType, CliType as CliTypeModel, ModelConfig};
use deployment::Deployment;
use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use services::services::{
    cli_installer::{CliInstaller, InstallOutputLine as ServiceInstallOutputLine},
    terminal::detector::CliDetector,
};
use tokio::sync::broadcast;

use crate::{DeploymentImpl, error::ApiError};

// ---------------------------------------------------------------------------
// Placeholder types for models/services not yet available from other agents
// ---------------------------------------------------------------------------

// TODO: Import from crates/db when available
// use db::models::cli_install_history::{CliInstallHistory, CliDetectionCache};

/// Placeholder for CliInstallHistory (created by DB agent).
/// Represents a single install/uninstall job record.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CliInstallHistory {
    pub id: String,
    pub cli_type_id: String,
    pub action: String, // "install" | "uninstall"
    pub status: String, // "running" | "completed" | "failed"
    pub exit_code: Option<i32>,
    pub output: Option<String>,
    pub error: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Placeholder for CliDetectionCache (created by DB agent).
/// Cached detection result for a single CLI type.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CliDetectionCache {
    pub cli_type_id: String,
    pub name: String,
    pub display_name: String,
    pub installed: bool,
    pub version: Option<String>,
    pub executable_path: Option<String>,
    pub cached_at: DateTime<Utc>,
}

/// A single line of install output streamed over WebSocket.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InstallOutputLine {
    pub line: String,
    pub stream: String, // "stdout" | "stderr"
    pub timestamp: DateTime<Utc>,
}

/// Query parameters for the install progress WebSocket.
#[derive(Debug, Deserialize)]
pub struct InstallWsParams {
    pub job_id: String,
}

/// Query parameters for paginated install history.
#[derive(Debug, Deserialize)]
pub struct PaginationParams {
    #[serde(default = "default_limit")]
    pub limit: i64,
    #[serde(default)]
    pub offset: i64,
}

fn default_limit() -> i64 {
    20
}

/// Shared registry of active install job output channels.
/// Maps job_id -> broadcast sender for streaming install output to WebSocket clients.
type InstallJobRegistry = Arc<tokio::sync::RwLock<HashMap<String, broadcast::Sender<String>>>>;

/// Lazily-initialized global install job registry and CliInstaller.
static INSTALL_REGISTRY: std::sync::OnceLock<InstallJobRegistry> = std::sync::OnceLock::new();
static CLI_INSTALLER: std::sync::OnceLock<CliInstaller> = std::sync::OnceLock::new();

/// Tracks CLI type IDs with an in-progress install or uninstall operation.
/// Prevents concurrent install/uninstall for the same CLI from corrupting state.
static INSTALL_IN_PROGRESS: std::sync::OnceLock<Mutex<HashSet<String>>> =
    std::sync::OnceLock::new();

fn get_install_in_progress() -> &'static Mutex<HashSet<String>> {
    INSTALL_IN_PROGRESS.get_or_init(|| Mutex::new(HashSet::new()))
}

/// RAII guard that removes the CLI type ID from the in-progress set on drop.
struct InstallGuard {
    cli_type_id: String,
}

impl Drop for InstallGuard {
    fn drop(&mut self) {
        if let Ok(mut set) = get_install_in_progress().lock() {
            set.remove(&self.cli_type_id);
        }
    }
}

fn get_install_registry() -> &'static InstallJobRegistry {
    INSTALL_REGISTRY.get_or_init(|| Arc::new(tokio::sync::RwLock::new(HashMap::new())))
}

fn get_cli_installer() -> &'static CliInstaller {
    CLI_INSTALLER.get_or_init(CliInstaller::new)
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

/// Create CLI types router
pub fn cli_types_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/", get(list_cli_types))
        .route("/detect", get(detect_cli_types))
        .route("/{cli_type_id}/models", get(list_models_for_cli))
        .route(
            "/{cli_type_id}/models/{model_id}/credentials",
            put(update_model_credentials),
        )
        .route(
            "/{cli_type_id}/install",
            post(install_cli).delete(uninstall_cli),
        )
        .route("/{cli_type_id}/install/status", get(get_install_status))
        .route("/{cli_type_id}/install/history", get(get_install_history))
        .route("/{cli_type_id}/install/ws", get(install_progress_ws))
        .route("/status/cached", get(get_cached_status))
        .route("/detect/refresh", post(refresh_detection))
}

// ---------------------------------------------------------------------------
// Existing endpoints
// ---------------------------------------------------------------------------

/// GET /api/cli_types
/// List all CLI types
async fn list_cli_types(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<Vec<CliType>>, ApiError> {
    let cli_types = CliTypeModel::find_all(&deployment.db().pool).await?;
    Ok(ResponseJson(cli_types))
}

/// GET /api/cli_types/detect
/// Detect installed CLIs
async fn detect_cli_types(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<Vec<CliDetectionStatus>>, ApiError> {
    let db = Arc::new(deployment.db().clone());
    let detector = CliDetector::new(db);

    let results = detector
        .detect_all()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to detect CLIs: {e}")))?;

    Ok(ResponseJson(results))
}

/// GET /api/cli_types/:cli_type_id/models
/// List models for a CLI type
async fn list_models_for_cli(
    State(deployment): State<DeploymentImpl>,
    Path(cli_type_id): Path<String>,
) -> Result<ResponseJson<Vec<ModelConfig>>, ApiError> {
    let models = ModelConfig::find_by_cli_type(&deployment.db().pool, &cli_type_id).await?;
    Ok(ResponseJson(models))
}

// ---------------------------------------------------------------------------
// Model credentials endpoint
// ---------------------------------------------------------------------------

/// Request body for updating model credentials
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct UpdateCredentialsRequest {
    api_key: String,
    base_url: Option<String>,
    api_type: String,
}

/// PUT /api/cli_types/:cli_type_id/models/:model_id/credentials
/// Save API credentials for a model config (used by workspace mode)
async fn update_model_credentials(
    State(deployment): State<DeploymentImpl>,
    Path((cli_type_id, model_id)): Path<(String, String)>,
    axum::extract::Json(payload): axum::extract::Json<UpdateCredentialsRequest>,
) -> Result<ResponseJson<Value>, ApiError> {
    // Validate model belongs to cli_type
    let model = ModelConfig::find_by_id(&deployment.db().pool, &model_id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("Model config not found: {model_id}")))?;

    if model.cli_type_id != cli_type_id {
        return Err(ApiError::BadRequest(format!(
            "Model {model_id} does not belong to CLI type {cli_type_id}"
        )));
    }

    // Encrypt the API key
    let mut temp_model = model;
    temp_model
        .set_api_key(&payload.api_key)
        .map_err(|e| ApiError::Internal(format!("Failed to encrypt API key: {e}")))?;

    let encrypted = temp_model
        .encrypted_api_key
        .as_deref()
        .ok_or_else(|| ApiError::Internal("Encryption produced no output".to_string()))?;

    ModelConfig::update_credentials(
        &deployment.db().pool,
        &model_id,
        encrypted,
        payload.base_url.as_deref(),
        &payload.api_type,
    )
    .await?;

    tracing::info!(
        model_id = %model_id,
        cli_type_id = %cli_type_id,
        api_type = %payload.api_type,
        "Model credentials saved"
    );

    Ok(ResponseJson(json!({ "saved": true })))
}

// ---------------------------------------------------------------------------
// Install / Uninstall endpoints
// ---------------------------------------------------------------------------

/// POST /api/cli_types/:cli_type_id/install
/// Start installing a CLI type. Returns immediately with a job ID while the
/// installation runs in a background task.
async fn install_cli(
    State(deployment): State<DeploymentImpl>,
    Path(cli_type_id): Path<String>,
) -> Result<(StatusCode, ResponseJson<Value>), ApiError> {
    tracing::info!(cli_type_id = %cli_type_id, "Starting CLI install");

    // Validate cli_type_id exists
    let cli_type = CliTypeModel::find_by_id(&deployment.db().pool, &cli_type_id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("CLI type not found: {cli_type_id}")))?;

    // Check that the CLI type has an install command
    let _install_command = cli_type.install_command.as_deref().ok_or_else(|| {
        ApiError::BadRequest(format!(
            "CLI type '{}' does not have an install command configured",
            cli_type.display_name
        ))
    })?;

    // Generate a unique job ID
    let job_id = format!("job-{}", uuid::Uuid::new_v4());

    // Guard: prevent concurrent install/uninstall for the same CLI type
    {
        let mut in_progress = get_install_in_progress()
            .lock()
            .map_err(|_| ApiError::Internal("Lock poisoned".to_string()))?;
        if !in_progress.insert(cli_type_id.clone()) {
            return Err(ApiError::Conflict(format!(
                "Installation already in progress for {cli_type_id}"
            )));
        }
    }

    // Spawn background installation task
    let bg_cli_name = cli_type.name.clone();
    let bg_job_id = job_id.clone();
    let bg_cli_type_id = cli_type_id.clone();
    tokio::spawn(async move {
        // RAII guard ensures cleanup even on panic
        let _guard = InstallGuard {
            cli_type_id: bg_cli_type_id,
        };
        tracing::info!(
            job_id = %bg_job_id,
            cli_name = %bg_cli_name,
            "Background CLI install task started"
        );

        let (tx, _) = broadcast::channel::<String>(256);
        {
            let mut registry = get_install_registry().write().await;
            registry.insert(bg_job_id.clone(), tx.clone());
        }

        match get_cli_installer().install_cli(&bg_cli_name).await {
            Ok(mut stream) => {
                while let Some(line) = stream.receiver.recv().await {
                    let msg = match &line {
                        ServiceInstallOutputLine::Stdout(s) => serde_json::json!({
                            "type": "stdout",
                            "content": s,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                        ServiceInstallOutputLine::Stderr(s) => serde_json::json!({
                            "type": "stderr",
                            "content": s,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                        ServiceInstallOutputLine::Completed { exit_code } => serde_json::json!({
                            "type": "completed",
                            "content": format!("Process exited with code {exit_code}"),
                            "exit_code": exit_code,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                        ServiceInstallOutputLine::Error(e) => serde_json::json!({
                            "type": "error",
                            "content": e,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                    };
                    if let Ok(json_str) = serde_json::to_string(&msg) {
                        let _ = tx.send(json_str);
                    }
                }
            }
            Err(e) => {
                let msg = serde_json::json!({
                    "type": "error",
                    "content": e.to_string(),
                    "timestamp": chrono::Utc::now().timestamp_millis(),
                });
                if let Ok(json_str) = serde_json::to_string(&msg) {
                    let _ = tx.send(json_str);
                }
            }
        }

        // Clean up registry
        {
            let mut registry = get_install_registry().write().await;
            registry.remove(&bg_job_id);
        }

        tracing::info!(job_id = %bg_job_id, "Background CLI install task completed");
    });

    Ok((
        StatusCode::ACCEPTED,
        ResponseJson(json!({
            "job_id": job_id,
            "status": "running"
        })),
    ))
}

/// DELETE /api/cli_types/:cli_type_id/install
/// Start uninstalling a CLI type. Returns immediately with a job ID while the
/// uninstallation runs in a background task.
async fn uninstall_cli(
    State(deployment): State<DeploymentImpl>,
    Path(cli_type_id): Path<String>,
) -> Result<(StatusCode, ResponseJson<Value>), ApiError> {
    tracing::info!(cli_type_id = %cli_type_id, "Starting CLI uninstall");

    // Validate cli_type_id exists
    let cli_type = CliTypeModel::find_by_id(&deployment.db().pool, &cli_type_id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("CLI type not found: {cli_type_id}")))?;

    // Generate a unique job ID
    let job_id = format!("job-{}", uuid::Uuid::new_v4());

    // Guard: prevent concurrent install/uninstall for the same CLI type
    {
        let mut in_progress = get_install_in_progress()
            .lock()
            .map_err(|_| ApiError::Internal("Lock poisoned".to_string()))?;
        if !in_progress.insert(cli_type_id.clone()) {
            return Err(ApiError::Conflict(format!(
                "Uninstallation already in progress for {cli_type_id}"
            )));
        }
    }

    // Spawn background uninstall task
    let bg_cli_name = cli_type.name.clone();
    let bg_job_id = job_id.clone();
    let bg_cli_type_id = cli_type_id.clone();
    tokio::spawn(async move {
        // RAII guard ensures cleanup even on panic
        let _guard = InstallGuard {
            cli_type_id: bg_cli_type_id,
        };
        tracing::info!(
            job_id = %bg_job_id,
            cli_name = %bg_cli_name,
            "Background CLI uninstall task started"
        );

        let (tx, _) = broadcast::channel::<String>(256);
        {
            let mut registry = get_install_registry().write().await;
            registry.insert(bg_job_id.clone(), tx.clone());
        }

        match get_cli_installer().uninstall_cli(&bg_cli_name).await {
            Ok(mut stream) => {
                while let Some(line) = stream.receiver.recv().await {
                    let msg = match &line {
                        ServiceInstallOutputLine::Stdout(s) => serde_json::json!({
                            "type": "stdout",
                            "content": s,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                        ServiceInstallOutputLine::Stderr(s) => serde_json::json!({
                            "type": "stderr",
                            "content": s,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                        ServiceInstallOutputLine::Completed { exit_code } => serde_json::json!({
                            "type": "completed",
                            "content": format!("Process exited with code {exit_code}"),
                            "exit_code": exit_code,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                        ServiceInstallOutputLine::Error(e) => serde_json::json!({
                            "type": "error",
                            "content": e,
                            "timestamp": chrono::Utc::now().timestamp_millis(),
                        }),
                    };
                    if let Ok(json_str) = serde_json::to_string(&msg) {
                        let _ = tx.send(json_str);
                    }
                }
            }
            Err(e) => {
                let msg = serde_json::json!({
                    "type": "error",
                    "content": e.to_string(),
                    "timestamp": chrono::Utc::now().timestamp_millis(),
                });
                if let Ok(json_str) = serde_json::to_string(&msg) {
                    let _ = tx.send(json_str);
                }
            }
        }

        {
            let mut registry = get_install_registry().write().await;
            registry.remove(&bg_job_id);
        }

        tracing::info!(job_id = %bg_job_id, "Background CLI uninstall task completed");
    });

    Ok((
        StatusCode::ACCEPTED,
        ResponseJson(json!({
            "job_id": job_id,
            "status": "running"
        })),
    ))
}

// ---------------------------------------------------------------------------
// Install status / history endpoints
// ---------------------------------------------------------------------------

/// GET /api/cli_types/:cli_type_id/install/status
/// Get the latest install job status for a CLI type.
async fn get_install_status(
    State(deployment): State<DeploymentImpl>,
    Path(cli_type_id): Path<String>,
) -> Result<ResponseJson<Value>, ApiError> {
    // Validate cli_type_id exists
    let _cli_type = CliTypeModel::find_by_id(&deployment.db().pool, &cli_type_id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("CLI type not found: {cli_type_id}")))?;

    // Check if there is currently an active install/uninstall for this CLI
    let is_active = get_install_in_progress()
        .lock()
        .map(|set| set.contains(&cli_type_id))
        .unwrap_or(false);

    if is_active {
        return Ok(ResponseJson(json!({
            "status": "running",
            "exit_code": null,
            "output": null,
            "error": null
        })));
    }

    Err(ApiError::NotImplemented(
        "Install history persistence is not yet implemented. Use the WebSocket endpoint to stream live install progress.".to_string(),
    ))
}

/// GET /api/cli_types/:cli_type_id/install/history
/// Get paginated install history for a CLI type.
async fn get_install_history(
    State(_deployment): State<DeploymentImpl>,
    Path(_cli_type_id): Path<String>,
    Query(_params): Query<PaginationParams>,
) -> Result<ResponseJson<Vec<CliInstallHistory>>, ApiError> {
    Err(ApiError::NotImplemented(
        "Install history persistence is not yet implemented.".to_string(),
    ))
}

// ---------------------------------------------------------------------------
// Detection cache / refresh endpoints
// ---------------------------------------------------------------------------

/// GET /api/cli_types/status/cached
/// Get cached detection results without re-running detection.
async fn get_cached_status(
    State(_deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<Vec<CliDetectionCache>>, ApiError> {
    Err(ApiError::NotImplemented(
        "Detection result caching is not yet implemented. Use GET /api/cli_types/detect for live detection.".to_string(),
    ))
}

/// POST /api/cli_types/detect/refresh
/// Force re-detection of all CLI types and update the cache.
async fn refresh_detection(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<Vec<CliDetectionStatus>>, ApiError> {
    tracing::info!("Refreshing CLI detection");

    let db = Arc::new(deployment.db().clone());
    let detector = CliDetector::new(db);

    let results = detector
        .detect_all()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to detect CLIs: {e}")))?;

    // TODO: Update CliDetectionCache with fresh results
    // Example:
    //   for result in &results {
    //       CliDetectionCache::upsert(&deployment.db().pool, result).await?;
    //   }

    Ok(ResponseJson(results))
}

// ---------------------------------------------------------------------------
// WebSocket endpoint for install progress streaming
// ---------------------------------------------------------------------------

/// WS /api/cli_types/:cli_type_id/install/ws
/// Stream install progress as JSON messages over WebSocket.
async fn install_progress_ws(
    ws: WebSocketUpgrade,
    State(deployment): State<DeploymentImpl>,
    Path(cli_type_id): Path<String>,
    Query(params): Query<InstallWsParams>,
) -> impl IntoResponse {
    tracing::info!(
        cli_type_id = %cli_type_id,
        job_id = %params.job_id,
        "WebSocket connection requested for install progress"
    );

    ws.on_upgrade(move |socket| {
        handle_install_progress_ws(socket, deployment, cli_type_id, params.job_id)
    })
}

/// Handle the WebSocket connection for streaming install progress.
async fn handle_install_progress_ws(
    socket: WebSocket,
    _deployment: DeploymentImpl,
    cli_type_id: String,
    job_id: String,
) {
    let (mut sender, mut receiver) = socket.split();

    tracing::info!(
        cli_type_id = %cli_type_id,
        job_id = %job_id,
        "Install progress WebSocket connected"
    );

    // Try to subscribe to the job's broadcast channel
    let mut rx = {
        let registry = get_install_registry().read().await;
        match registry.get(&job_id) {
            Some(tx) => tx.subscribe(),
            None => {
                // Job not found or already completed
                let msg = serde_json::json!({
                    "type": "error",
                    "content": format!("No active install job found for job_id: {job_id}"),
                    "timestamp": chrono::Utc::now().timestamp_millis(),
                });
                if let Ok(json_str) = serde_json::to_string(&msg) {
                    let _ = sender.send(Message::Text(json_str.into())).await;
                }
                return;
            }
        }
    };

    loop {
        tokio::select! {
            result = rx.recv() => {
                match result {
                    Ok(msg) => {
                        if sender.send(Message::Text(msg.into())).await.is_err() {
                            break;
                        }
                    }
                    Err(broadcast::error::RecvError::Closed) => {
                        break;
                    }
                    Err(broadcast::error::RecvError::Lagged(n)) => {
                        tracing::warn!(skipped = n, "WebSocket client lagged");
                    }
                }
            }
            msg = receiver.next() => {
                match msg {
                    Some(Ok(Message::Close(_))) | None => break,
                    Some(Err(_)) => break,
                    _ => {}
                }
            }
        }
    }

    tracing::info!(
        job_id = %job_id,
        "Install progress WebSocket disconnected"
    );
}
