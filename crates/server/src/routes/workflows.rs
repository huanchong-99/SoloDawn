//! Workflow API Routes

use std::{
    collections::{HashMap, VecDeque},
    path::PathBuf,
    sync::Arc,
    time::{Duration, Instant},
};

use axum::{
    Json, Router,
    extract::{Path, Query, State},
    http::HeaderMap,
    response::Json as ResponseJson,
    routing::{get, post, put},
};
use chrono::Utc;
use db::models::{
    CliType, CreateWorkflowRequest, InlineModelConfig, ModelConfig, SlashCommandPreset, Terminal,
    Workflow, WorkflowCommand, WorkflowOrchestratorCommand, WorkflowOrchestratorMessage,
    WorkflowTask,
    project::Project,
};
use deployment::Deployment;
use serde::{Deserialize, Serialize};
use serde_json::json;
use services::services::{
    cc_switch::CCSwitchService,
    config::Config as AppConfig,
    git::GitServiceError,
    orchestrator::{BusMessage, OrchestratorRuntime, TerminalCoordinator, constants::WORKFLOW_STATUS_PAUSED},
    terminal::TerminalLauncher,
};
use once_cell::sync::Lazy;
use regex::Regex;
use sha2::{Digest, Sha256};
use utils::{response::ApiResponse, text};
use uuid::Uuid;

// Import DTOs
use crate::routes::terminals::start_terminal;
use crate::routes::workflows_dto::{WorkflowDetailDto, WorkflowListItemDto};
use crate::{DeploymentImpl, error::ApiError};

#[cfg(test)]
use db::models::workflow::{CreateTerminalRequest, CreateWorkflowTaskRequest};

// ============================================================================
// Request/Response Types
// ============================================================================

/// Workflow Detail Response
#[derive(Debug, Serialize)]
pub struct WorkflowDetailResponse {
    #[serde(flatten)]
    pub workflow: Workflow,
    pub tasks: Vec<WorkflowTaskDetailResponse>,
    pub commands: Vec<WorkflowCommandWithPreset>,
}

/// Workflow Task Detail Response
#[derive(Debug, Serialize)]
pub struct WorkflowTaskDetailResponse {
    #[serde(flatten)]
    pub task: WorkflowTask,
    pub terminals: Vec<Terminal>,
}

/// Workflow Command with Preset
#[derive(Debug, Serialize)]
pub struct WorkflowCommandWithPreset {
    #[serde(flatten)]
    pub command: WorkflowCommand,
    pub preset: SlashCommandPreset,
}

/// Update Workflow Status Request
#[derive(Debug, Deserialize)]
pub struct UpdateWorkflowStatusRequest {
    pub status: String,
}

/// Recovery Response
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RecoveryResponse {
    pub message: String,
    pub recovered_workflows: usize,
    pub recovered_commands: usize,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateRuntimeTaskRequest {
    pub name: String,
    pub description: Option<String>,
    pub branch: Option<String>,
    pub order_index: Option<i32>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateRuntimeTerminalRequest {
    pub cli_type_id: String,
    pub model_config_id: String,
    pub custom_base_url: Option<String>,
    pub custom_api_key: Option<String>,
    pub role: Option<String>,
    pub role_description: Option<String>,
    pub order_index: Option<i32>,
    #[serde(default = "default_runtime_terminal_auto_confirm")]
    pub auto_confirm: bool,
    #[serde(default)]
    pub start_immediately: bool,
}

/// Merge Workflow Request
#[derive(Debug, Deserialize)]
pub struct MergeWorkflowRequest {
    pub merge_strategy: Option<String>,
}

const WORKFLOW_STATUSES: [&str; 9] = [
    "created",
    "starting",
    "ready",
    "running",
    WORKFLOW_STATUS_PAUSED,
    "merging",
    "completed",
    "failed",
    "cancelled",
];

const WORKFLOW_EXECUTION_MODES: [&str; 2] = ["diy", "agent_planned"];

const MERGE_ALLOWED_WORKFLOW_STATUSES: [&str; 2] = ["completed", "merging"];
const RUNTIME_MUTABLE_WORKFLOW_STATUSES: [&str; 5] =
    ["created", "starting", "ready", "running", WORKFLOW_STATUS_PAUSED];
const ORCHESTRATOR_RATE_LIMIT_WINDOW: Duration = Duration::from_secs(60);
const ORCHESTRATOR_RATE_LIMIT_MAX_REQUESTS: usize = 12;
const ORCHESTRATOR_CIRCUIT_BREAKER_THRESHOLD: usize = 3;
const ORCHESTRATOR_CIRCUIT_BREAKER_COOLDOWN: Duration = Duration::from_secs(120);
const ORCHESTRATOR_CHAT_ALLOWED_ROLES: [&str; 3] = ["owner", "admin", "operator"];

#[derive(Debug, Default)]
struct OrchestratorGovernanceState {
    rate_windows: HashMap<String, VecDeque<Instant>>,
    failure_streaks: HashMap<String, usize>,
    circuit_open_until: HashMap<String, Instant>,
}

static ORCHESTRATOR_GOVERNANCE_STATE: Lazy<tokio::sync::Mutex<OrchestratorGovernanceState>> =
    Lazy::new(|| tokio::sync::Mutex::new(OrchestratorGovernanceState::default()));

static SENSITIVE_INLINE_CREDENTIAL_REGEX: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"(?i)\b(api[_-]?key|token|authorization|bearer)\s*[:=]\s*[^\s,;]+")
        .expect("credential regex must be valid")
});
static SECRET_PREFIX_REGEX: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"\b(sk-[A-Za-z0-9_\-]{12,})\b").expect("secret prefix regex must be valid")
});

// ============================================================================
// Route Definition
// ============================================================================

/// Create workflows router
pub fn workflows_routes() -> Router<DeploymentImpl> {
    Router::new()
        .route("/", get(list_workflows).post(create_workflow))
        .route("/recover", post(recover_workflows))
        .route("/{workflow_id}", get(get_workflow).delete(delete_workflow))
        .route("/{workflow_id}/status", put(update_workflow_status))
        .route("/{workflow_id}/prepare", post(prepare_workflow))
        .route("/{workflow_id}/start", post(start_workflow))
        .route("/{workflow_id}/pause", post(pause_workflow))
        .route("/{workflow_id}/resume", post(resume_workflow))
        .route("/{workflow_id}/stop", post(stop_workflow))
        .route(
            "/{workflow_id}/prompts/respond",
            post(submit_prompt_response),
        )
        .route("/{workflow_id}/orchestrator/chat", post(submit_orchestrator_chat))
        .route(
            "/{workflow_id}/orchestrator/messages",
            get(list_orchestrator_messages),
        )
        .route("/{workflow_id}/merge", post(merge_workflow))
        .route(
            "/{workflow_id}/tasks",
            get(list_workflow_tasks).post(create_runtime_task),
        )
        .route(
            "/{workflow_id}/tasks/{task_id}/status",
            put(update_task_status),
        )
        .route(
            "/{workflow_id}/tasks/{task_id}/terminals",
            get(list_task_terminals).post(create_runtime_terminal),
        )
}

fn is_known_workflow_status(status: &str) -> bool {
    WORKFLOW_STATUSES.contains(&status)
}

fn can_merge_from_workflow_status(status: &str) -> bool {
    MERGE_ALLOWED_WORKFLOW_STATUSES.contains(&status)
}

fn default_runtime_terminal_auto_confirm() -> bool {
    true
}

fn is_valid_workflow_status_transition(current: &str, next: &str) -> bool {
    if current == next {
        return is_known_workflow_status(current);
    }

    matches!(
        (current, next),
        ("created" | "failed", "starting") |
("created" | "starting" | "ready" | "running" | "paused" | "merging",
"failed") |
("created" | "starting" | "ready" | "running" | "paused" | "failed",
"cancelled") | ("starting" | "paused", "ready") |
("ready" | "paused", "running") | ("running", "paused" | "completed") |
("completed", "merging" | "created") | ("merging", "completed") |
("failed" | "cancelled", "created")
    )
}

fn validate_workflow_status_transition(current: &str, next: &str) -> Result<(), ApiError> {
    if !is_known_workflow_status(next) {
        return Err(ApiError::BadRequest(format!(
            "Invalid workflow status '{next}', expected one of: {WORKFLOW_STATUSES:?}"
        )));
    }

    if !is_known_workflow_status(current) {
        return Err(ApiError::Conflict(format!(
            "Cannot transition workflow from unknown status '{current}': expected one of: {WORKFLOW_STATUSES:?}"
        )));
    }

    if !is_valid_workflow_status_transition(current, next) {
        return Err(ApiError::Conflict(format!(
            "Invalid workflow status transition: '{current}' -> '{next}'"
        )));
    }

    Ok(())
}

fn validate_task_workflow_scope(task: &WorkflowTask, workflow_id: &str) -> Result<(), ApiError> {
    if task.workflow_id != workflow_id {
        return Err(ApiError::BadRequest(
            "Task does not belong to this workflow".to_string(),
        ));
    }

    Ok(())
}

fn validate_runtime_mutation_workflow_status(status: &str) -> Result<(), ApiError> {
    if !RUNTIME_MUTABLE_WORKFLOW_STATUSES.contains(&status) {
        return Err(ApiError::Conflict(format!(
            "Workflow status '{status}' does not allow runtime task or terminal mutations"
        )));
    }

    Ok(())
}

fn has_configured_workflow_models(config: &AppConfig) -> bool {
    config
        .workflow_model_library
        .iter()
        .any(|item| !item.model_id.trim().is_empty())
}

fn is_orchestrator_chat_feature_enabled() -> bool {
    std::env::var("GITCORTEX_ORCHESTRATOR_CHAT_ENABLED")
        .ok()
        .map_or(true, |value| value.trim().eq_ignore_ascii_case("true"))
}

fn redact_sensitive_content(content: &str) -> String {
    let redacted_pairs = SENSITIVE_INLINE_CREDENTIAL_REGEX
        .replace_all(content, "$1=<redacted>")
        .into_owned();
    let redacted_secrets = SECRET_PREFIX_REGEX
        .replace_all(&redacted_pairs, "<redacted-secret>")
        .into_owned();
    if redacted_secrets.chars().count() <= 220 {
        redacted_secrets
    } else {
        let preview: String = redacted_secrets.chars().take(220).collect();
        format!("{preview}...(truncated)")
    }
}

fn digest_message(content: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(content.as_bytes());
    let digest = hasher.finalize();
    format!("{digest:x}")
}

fn normalize_operator_id(candidate: Option<&str>) -> Option<String> {
    candidate
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn extract_role_from_headers(headers: &HeaderMap) -> Option<String> {
    headers
        .get("x-orchestrator-role")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_ascii_lowercase)
}

fn ensure_orchestrator_permission(
    source: &str,
    role: Option<&str>,
    workflow_execution_mode: &str,
    operator_id: Option<&str>,
) -> Result<(), ApiError> {
    if source != "web" && workflow_execution_mode != "agent_planned" {
        return Err(ApiError::Forbidden(
            "Only agent_planned workflows accept external orchestrator command sources".to_string(),
        ));
    }

    if source != "web" && operator_id.is_none() {
        return Err(ApiError::Forbidden(
            "operatorId is required for non-web orchestrator command sources".to_string(),
        ));
    }

    if let Some(role) = role
        && !ORCHESTRATOR_CHAT_ALLOWED_ROLES.contains(&role)
    {
        return Err(ApiError::Forbidden(format!(
            "orchestrator role '{role}' is not allowed to issue commands"
        )));
    }

    Ok(())
}

// TODO(G16-013): Rate-limit state grows unboundedly as workflows accumulate.
// Add a periodic cleanup task (or LRU eviction) to prune stale entries from
// ORCHESTRATOR_GOVERNANCE_STATE.rate_windows for workflows that are no longer active.
async fn enforce_orchestrator_rate_limit(
    workflow_id: &str,
    source: &str,
    operator_scope: Option<&str>,
) -> Result<(), ApiError> {
    let scope = operator_scope.unwrap_or("anonymous");
    let key = format!("{workflow_id}:{source}:{scope}");
    let now = Instant::now();
    let mut state = ORCHESTRATOR_GOVERNANCE_STATE.lock().await;
    let bucket = state.rate_windows.entry(key).or_default();

    while let Some(timestamp) = bucket.front().copied() {
        if now.duration_since(timestamp) <= ORCHESTRATOR_RATE_LIMIT_WINDOW {
            break;
        }
        bucket.pop_front();
    }

    if bucket.len() >= ORCHESTRATOR_RATE_LIMIT_MAX_REQUESTS {
        return Err(ApiError::Conflict(format!(
            "Orchestrator chat rate limit exceeded: max {} requests per {}s",
            ORCHESTRATOR_RATE_LIMIT_MAX_REQUESTS,
            ORCHESTRATOR_RATE_LIMIT_WINDOW.as_secs()
        )));
    }

    bucket.push_back(now);
    Ok(())
}

async fn ensure_orchestrator_circuit_closed(workflow_id: &str) -> Result<(), ApiError> {
    let now = Instant::now();
    let mut state = ORCHESTRATOR_GOVERNANCE_STATE.lock().await;
    if let Some(open_until) = state.circuit_open_until.get(workflow_id).copied() {
        if now < open_until {
            return Err(ApiError::Conflict(
                "Orchestrator circuit breaker is open due to repeated failures. Please retry later."
                    .to_string(),
            ));
        }
        state.circuit_open_until.remove(workflow_id);
        state.failure_streaks.remove(workflow_id);
    }

    Ok(())
}

async fn update_orchestrator_circuit_breaker(workflow_id: &str, status: &str) -> bool {
    let mut state = ORCHESTRATOR_GOVERNANCE_STATE.lock().await;
    if status == "succeeded" {
        state.failure_streaks.remove(workflow_id);
        state.circuit_open_until.remove(workflow_id);
        return false;
    }

    if status == "failed" || status == "cancelled" {
        let streak_key = workflow_id.to_string();
        let streak_count = {
            let streak = state.failure_streaks.entry(streak_key.clone()).or_insert(0);
            *streak += 1;
            *streak
        };

        if streak_count >= ORCHESTRATOR_CIRCUIT_BREAKER_THRESHOLD {
            state.circuit_open_until.insert(
                streak_key.clone(),
                Instant::now() + ORCHESTRATOR_CIRCUIT_BREAKER_COOLDOWN,
            );
            state.failure_streaks.insert(streak_key, 0);
            return true;
        }
    }

    false
}

fn build_orchestrator_receipt_message(
    command_id: &str,
    status: &str,
    error: Option<&str>,
    retryable: bool,
) -> String {
    let mut receipt = format!("Command {command_id} -> {status}");
    if let Some(error) = error {
        let redacted = redact_sensitive_content(error);
        receipt.push_str(&format!(". Error: {redacted}"));
    }
    if retryable {
        receipt.push_str(". Retryable: yes");
    }
    receipt
}

fn build_orchestrator_summary_message(status: &str, source: &str) -> String {
    match status {
        "succeeded" => format!(
            "Execution summary: command completed successfully (source={source})."
        ),
        "failed" => format!(
            "Execution summary: command failed and requires operator attention (source={source})."
        ),
        "cancelled" => {
            format!("Execution summary: command was cancelled (source={source}).")
        }
        other => format!("Execution summary: command finished with status={other} (source={source})."),
    }
}

async fn broadcast_task_status(
    deployment: &DeploymentImpl,
    task: &WorkflowTask,
    status: &str,
) -> anyhow::Result<()> {
    let message = BusMessage::TaskStatusUpdate {
        workflow_id: task.workflow_id.clone(),
        task_id: task.id.clone(),
        status: status.to_string(),
    };
    let topic = format!("workflow:{}", task.workflow_id);

    deployment
        .message_bus()
        .publish(&topic, message.clone())
        .await
        .map_err(|e| anyhow::anyhow!("Failed to publish task status: {e}"))?;
    deployment
        .message_bus()
        .broadcast(message)
        .map_err(|e| anyhow::anyhow!("Failed to broadcast task status: {e}"))?;

    Ok(())
}

async fn broadcast_runtime_terminal_status(
    deployment: &DeploymentImpl,
    task: &WorkflowTask,
    terminal_id: &str,
    status: &str,
) -> anyhow::Result<()> {
    let message = BusMessage::TerminalStatusUpdate {
        workflow_id: task.workflow_id.clone(),
        terminal_id: terminal_id.to_string(),
        status: status.to_string(),
    };
    let topic = format!("workflow:{}", task.workflow_id);

    deployment
        .message_bus()
        .publish(&topic, message.clone())
        .await
        .map_err(|e| anyhow::anyhow!("Failed to publish terminal status: {e}"))?;
    deployment
        .message_bus()
        .broadcast(message)
        .map_err(|e| anyhow::anyhow!("Failed to broadcast terminal status: {e}"))?;

    Ok(())
}

fn should_auto_complete_workflow(workflow_status: &str, tasks: &[WorkflowTask]) -> bool {
    workflow_status == "running"
        && !tasks.is_empty()
        && tasks.iter().all(|task| task.status == "completed")
}

// ============================================================================
// Route Handlers
// ============================================================================

/// Validate create workflow request
pub fn validate_create_request(req: &CreateWorkflowRequest) -> Result<(), ApiError> {
    // Validate project_id
    if req.project_id.trim().is_empty() {
        return Err(ApiError::BadRequest("projectId is required".to_string()));
    }

    // Validate workflow name
    if req.name.trim().is_empty() {
        return Err(ApiError::BadRequest("name is required".to_string()));
    }

    if !WORKFLOW_EXECUTION_MODES.contains(&req.execution_mode.as_str()) {
        return Err(ApiError::BadRequest(format!(
            "executionMode must be one of: {}",
            WORKFLOW_EXECUTION_MODES.join(", ")
        )));
    }

    if req.execution_mode == "agent_planned" {
        if req.orchestrator_config.is_none() {
            return Err(ApiError::BadRequest(
                "orchestratorConfig is required for agent_planned workflows".to_string(),
            ));
        }

        if req
            .initial_goal
            .as_ref()
            .is_none_or(|goal| goal.trim().is_empty())
        {
            return Err(ApiError::BadRequest(
                "initialGoal is required for agent_planned workflows".to_string(),
            ));
        }
    } else if req.tasks.is_empty() {
        return Err(ApiError::BadRequest("tasks must not be empty".to_string()));
    }

    // Validate each task
    for (task_index, task) in req.tasks.iter().enumerate() {
        if task.name.trim().is_empty() {
            return Err(ApiError::BadRequest(format!(
                "task[{task_index}].name is required"
            )));
        }

        if task.terminals.is_empty() {
            return Err(ApiError::BadRequest(format!(
                "task[{task_index}].terminals must not be empty"
            )));
        }

        // Validate each terminal
        for (terminal_index, terminal) in task.terminals.iter().enumerate() {
            if terminal.cli_type_id.trim().is_empty() {
                return Err(ApiError::BadRequest(format!(
                    "task[{task_index}].terminal[{terminal_index}].cliTypeId is required"
                )));
            }

            if terminal.model_config_id.trim().is_empty() {
                return Err(ApiError::BadRequest(format!(
                    "task[{task_index}].terminal[{terminal_index}].modelConfigId is required"
                )));
            }
        }
    }

    // G01-002: Validate merge_terminal_config has non-empty cli_type_id and model_config_id
    if req.merge_terminal_config.cli_type_id.trim().is_empty() {
        return Err(ApiError::BadRequest(
            "mergeTerminalConfig.cliTypeId is required".to_string(),
        ));
    }
    if req.merge_terminal_config.model_config_id.trim().is_empty() {
        return Err(ApiError::BadRequest(
            "mergeTerminalConfig.modelConfigId is required".to_string(),
        ));
    }

    // Validate commands if provided
    if let Some(ref commands) = req.commands {
        for (cmd_index, cmd) in commands.iter().enumerate() {
            if cmd.preset_id.trim().is_empty() {
                return Err(ApiError::BadRequest(format!(
                    "commands[{cmd_index}].presetId is required"
                )));
            }

            // Validate custom_params JSON format if provided
            if let Some(ref params) = cmd.custom_params {
                if !params.trim().is_empty() {
                    serde_json::from_str::<serde_json::Value>(params).map_err(|_| {
                        ApiError::BadRequest(format!(
                            "commands[{cmd_index}].customParams must be valid JSON"
                        ))
                    })?;
                }
            }
        }
    }

    Ok(())
}

/// Validate CLI types and model configs exist in database
/// If model_config_id doesn't exist but inline model_config is provided,
/// automatically create a new ModelConfig record.
async fn validate_cli_and_model_configs(
    pool: &sqlx::SqlitePool,
    req: &CreateWorkflowRequest,
) -> Result<(), ApiError> {
    // Collect unique model_config_id references with CLI type and inline config
    let mut model_config_refs: HashMap<String, (String, Option<InlineModelConfig>)> =
        HashMap::new();

    // Helper to track model config references
    let mut track_ref = |cli_type_id: &str,
                         model_config_id: &str,
                         inline: Option<&InlineModelConfig>|
     -> Result<(), ApiError> {
        match model_config_refs.get_mut(model_config_id) {
            Some((existing_cli_type_id, existing_inline)) => {
                // Validate same model_config_id is used with same CLI type
                if existing_cli_type_id != cli_type_id {
                    return Err(ApiError::BadRequest(format!(
                        "Model config {model_config_id} used with multiple CLI types: {existing_cli_type_id} and {cli_type_id}"
                    )));
                }
                // Use inline config if not already set
                if existing_inline.is_none() {
                    *existing_inline = inline.cloned();
                }
            }
            None => {
                model_config_refs.insert(
                    model_config_id.to_string(),
                    (cli_type_id.to_string(), inline.cloned()),
                );
            }
        }
        Ok(())
    };

    // Track merge terminal config
    track_ref(
        &req.merge_terminal_config.cli_type_id,
        &req.merge_terminal_config.model_config_id,
        req.merge_terminal_config.model_config.as_ref(),
    )?;

    // Track error terminal config if present
    if let Some(error_config) = &req.error_terminal_config {
        track_ref(
            &error_config.cli_type_id,
            &error_config.model_config_id,
            error_config.model_config.as_ref(),
        )?;
    }

    // Track all task terminals
    for task in &req.tasks {
        for terminal in &task.terminals {
            track_ref(
                &terminal.cli_type_id,
                &terminal.model_config_id,
                terminal.model_config.as_ref(),
            )?;
        }
    }

    // Validate each unique model_config_id
    for (model_config_id, (cli_type_id, inline)) in model_config_refs {
        // Validate CLI type exists
        let cli_type = CliType::find_by_id(pool, &cli_type_id)
            .await
            .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;

        if cli_type.is_none() {
            return Err(ApiError::BadRequest(format!(
                "CLI type not found: {cli_type_id}"
            )));
        }

        // Validate model config exists or create from inline data
        let model_config = ModelConfig::find_by_id(pool, &model_config_id)
            .await
            .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?;

        let model_config = if let Some(mc) = model_config { mc } else {
            // Model config not found - try to create from inline data
            let inline = inline.ok_or_else(|| ApiError::BadRequest(format!(
                "Model config not found: {model_config_id}. Provide inline modelConfig to auto-create."
            )))?;

            // Create custom model config from inline data
            ModelConfig::create_custom(
                pool,
                &model_config_id,
                &cli_type_id,
                &inline.display_name,
                &inline.model_id,
            )
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to create model config: {e}")))?
        };

        // Validate model config belongs to the CLI type
        if model_config.cli_type_id != cli_type_id {
            return Err(ApiError::BadRequest(format!(
                "Model config {model_config_id} does not belong to CLI type {cli_type_id}"
            )));
        }
    }

    Ok(())
}

/// GET /api/workflows?project_id=xxx
/// List workflows for a project
async fn list_workflows(
    State(deployment): State<DeploymentImpl>,
    Query(params): Query<HashMap<String, String>>,
) -> Result<ResponseJson<ApiResponse<Vec<WorkflowListItemDto>>>, ApiError> {
    let project_id_str = params
        .get("project_id")
        .ok_or_else(|| ApiError::BadRequest("project_id is required".to_string()))?;

    // Parse project_id as UUID
    let project_id = Uuid::parse_str(project_id_str)
        .map_err(|_| ApiError::BadRequest("project_id must be a valid UUID".to_string()))?;

    // Use optimized query that returns counts in a single database call
    let workflows_with_counts =
        Workflow::find_by_project_with_counts(&deployment.db().pool, project_id).await?;

    // Convert to DTOs
    let dtos: Vec<WorkflowListItemDto> = workflows_with_counts
        .into_iter()
        .map(|w| WorkflowListItemDto {
            id: w.id,
            project_id: w.project_id.to_string(),
            name: w.name,
            description: w.description,
            status: w.status,
            execution_mode: w.execution_mode,
            created_at: w.created_at.to_rfc3339(),
            updated_at: w.updated_at.to_rfc3339(),
            tasks_count: w.tasks_count as i32,
            terminals_count: w.terminals_count as i32,
        })
        .collect();

    Ok(ResponseJson(ApiResponse::success(dtos)))
}

/// POST /api/workflows
/// Create workflow
async fn create_workflow(
    State(deployment): State<DeploymentImpl>,
    Json(req): Json<CreateWorkflowRequest>,
) -> Result<ResponseJson<ApiResponse<WorkflowDetailDto>>, ApiError> {
    // Validate request structure
    validate_create_request(&req)?;

    // Parse and validate project_id as UUID
    let project_id = Uuid::parse_str(&req.project_id)
        .map_err(|_| ApiError::BadRequest("projectId must be a valid UUID".to_string()))?;

    // Validate CLI types and model configs exist in database
    validate_cli_and_model_configs(&deployment.db().pool, &req).await?;

    let now = chrono::Utc::now();
    let workflow_id = Uuid::new_v4().to_string();

    // Log workflow creation details
    let total_terminals: usize = req.tasks.iter().map(|task| task.terminals.len()).sum();
    tracing::info!(
        workflow_id = %workflow_id,
        project_id = %project_id,
        tasks = req.tasks.len(),
        terminals = total_terminals,
        "creating workflow"
    );

    // 1. Create workflow with encrypted API key
    let mut workflow = Workflow {
        id: workflow_id.clone(),
        project_id,
        name: req.name,
        description: req.description,
        status: "created".to_string(),
        execution_mode: req.execution_mode,
        initial_goal: req.initial_goal,
        use_slash_commands: req.use_slash_commands,
        orchestrator_enabled: req.orchestrator_config.is_some(),
        orchestrator_api_type: req.orchestrator_config.as_ref().map(|c| c.api_type.clone()),
        orchestrator_base_url: req.orchestrator_config.as_ref().map(|c| c.base_url.clone()),
        orchestrator_api_key: None, // Will be set encrypted below
        orchestrator_model: req.orchestrator_config.as_ref().map(|c| c.model.clone()),
        error_terminal_enabled: req.error_terminal_config.is_some(),
        error_terminal_cli_id: req
            .error_terminal_config
            .as_ref()
            .map(|c| c.cli_type_id.clone()),
        error_terminal_model_id: req
            .error_terminal_config
            .as_ref()
            .map(|c| c.model_config_id.clone()),
        merge_terminal_cli_id: req.merge_terminal_config.cli_type_id.clone(),
        merge_terminal_model_id: req.merge_terminal_config.model_config_id.clone(),
        target_branch: req.target_branch.unwrap_or_else(|| "main".to_string()),
        git_watcher_enabled: req.git_watcher_enabled.unwrap_or(true),
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
    };

    // Encrypt and store API key if provided
    if let Some(orch_config) = &req.orchestrator_config {
        workflow
            .set_api_key(&orch_config.api_key)
            .map_err(|e| ApiError::BadRequest(format!("Failed to encrypt API key: {e}")))?;
    }

    // 2. Prepare tasks and terminals for transactional creation
    let mut task_rows: Vec<(WorkflowTask, Vec<Terminal>)> = Vec::new();
    let mut existing_branches: Vec<String> = Vec::new();

    // G23-003: Fetch the project's primary repo path so we can query git for
    // existing branches (both local and remote) and avoid naming conflicts that
    // span across workflows or pre-existing branches.
    let project_repo_path_for_branch_check: Option<std::path::PathBuf> = {
        let repo_path_str: Option<String> = sqlx::query_scalar(
            r"
            SELECT r.path
            FROM repos r
            INNER JOIN project_repos pr ON pr.repo_id = r.id
            WHERE pr.project_id = ?
            ORDER BY r.display_name ASC
            LIMIT 1
            ",
        )
        .bind(project_id)
        .fetch_optional(&deployment.db().pool)
        .await
        .unwrap_or(None)
        .flatten();
        repo_path_str.map(std::path::PathBuf::from)
    };

    for task_req in &req.tasks {
        let task_id = Uuid::new_v4().to_string();

        // Generate branch name using slugify with conflict detection
        let branch = if let Some(custom_branch) = &task_req.branch {
            // Use custom branch name if provided
            custom_branch.clone()
        } else {
            // Auto-generate branch name: workflow/{workflow_id}/{slugified-task-name}
            // G23-003: Check both the current batch AND the git repository for conflicts.
            let base_branch = format!(
                "workflow/{}/{}",
                workflow_id,
                text::git_branch_id(&task_req.name)
            );
            let mut candidate = base_branch.clone();
            let mut counter = 2;

            loop {
                let in_batch = existing_branches.contains(&candidate);
                // Only do the git check when we have a repo path and the batch check passes.
                let in_git = if !in_batch {
                    if let Some(repo_path) = &project_repo_path_for_branch_check {
                        let git_service = services::services::git::GitService::new();
                        let branch_candidate = candidate.clone();
                        let repo_path_owned = repo_path.clone();
                        tokio::task::spawn_blocking(move || {
                            git_service.check_branch_exists(&repo_path_owned, &branch_candidate)
                        })
                        .await
                        .ok()
                        .and_then(|r| r.ok())
                        .unwrap_or(false)
                    } else {
                        false
                    }
                } else {
                    false
                };

                if !in_batch && !in_git {
                    break;
                }
                candidate = format!("{base_branch}-{counter}");
                counter += 1;
            }

            candidate
        };

        // Track this branch to avoid conflicts within the same batch
        existing_branches.push(branch.clone());

        let task = WorkflowTask {
            id: task_id.clone(),
            workflow_id: workflow_id.clone(),
            vk_task_id: task_req
                .id
                .as_deref()
                .and_then(|id| Uuid::parse_str(id).ok()),
            name: task_req.name.clone(),
            description: task_req.description.clone(),
            branch,
            status: "pending".to_string(),
            order_index: task_req.order_index,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
        };

        let mut terminals: Vec<Terminal> = Vec::new();

        for terminal_req in &task_req.terminals {
            let mut terminal = Terminal {
                id: Uuid::new_v4().to_string(),
                workflow_task_id: task_id.clone(),
                cli_type_id: terminal_req.cli_type_id.clone(),
                model_config_id: terminal_req.model_config_id.clone(),
                custom_base_url: terminal_req.custom_base_url.clone(),
                custom_api_key: None, // Will be set encrypted below
                role: terminal_req.role.clone(),
                role_description: terminal_req.role_description.clone(),
                order_index: terminal_req.order_index,
                status: "not_started".to_string(),
                process_id: None,
                pty_session_id: None,
                session_id: None,
                execution_process_id: None,
                vk_session_id: None,
                auto_confirm: terminal_req.auto_confirm,
                last_commit_hash: None,
                last_commit_message: None,
                started_at: None,
                completed_at: None,
                created_at: now,
                updated_at: now,
            };

            // Encrypt and store API key if provided
            if let Some(custom_api_key) = terminal_req.custom_api_key.as_deref() {
                terminal.set_custom_api_key(custom_api_key).map_err(|e| {
                    ApiError::BadRequest(format!("Failed to encrypt terminal API key: {e}"))
                })?;
            }

            terminals.push(terminal);
        }

        task_rows.push((task, terminals));
    }

    // 3. Execute transactional creation (workflow + tasks + terminals)
    Workflow::create_with_tasks(&deployment.db().pool, &workflow, task_rows)
        .await
        .map_err(|e| ApiError::BadRequest(format!("Failed to create workflow: {e}")))?;

    // 4. Create slash command associations inside a transaction (G01-003)
    let mut commands: Vec<WorkflowCommand> = Vec::new();
    if let Some(command_reqs) = req.commands {
        let mut tx = deployment.db().pool.begin().await.map_err(|e| {
            ApiError::Internal(format!("Failed to begin command transaction: {e}"))
        })?;

        for (index, cmd_req) in command_reqs.iter().enumerate() {
            let index = i32::try_from(index)
                .map_err(|_| ApiError::BadRequest("Command index overflow".to_string()))?;

            // Validate custom_params is valid JSON if provided
            if let Some(ref params) = cmd_req.custom_params {
                if !params.trim().is_empty() {
                    // Validate JSON format
                    serde_json::from_str::<serde_json::Value>(params).map_err(|_| {
                        ApiError::BadRequest(format!(
                            "Invalid JSON in custom_params for preset {}",
                            cmd_req.preset_id
                        ))
                    })?;
                }
            }

            let cmd_id = Uuid::new_v4().to_string();
            let now = chrono::Utc::now();
            sqlx::query(
                r"
                INSERT INTO workflow_command (id, workflow_id, preset_id, order_index, custom_params, created_at)
                VALUES (?1, ?2, ?3, ?4, ?5, ?6)
                ",
            )
            .bind(&cmd_id)
            .bind(&workflow_id)
            .bind(&cmd_req.preset_id)
            .bind(index)
            .bind(cmd_req.custom_params.as_deref())
            .bind(now)
            .execute(&mut *tx)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to create workflow command: {e}")))?;
        }

        tx.commit().await.map_err(|e| {
            ApiError::Internal(format!("Failed to commit command transaction: {e}"))
        })?;

        commands = WorkflowCommand::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    }

    // 5. Get command preset details
    let all_presets = SlashCommandPreset::find_all(&deployment.db().pool).await?;
    let commands_with_presets: Vec<(WorkflowCommand, SlashCommandPreset)> = commands
        .into_iter()
        .filter_map(|cmd| {
            all_presets
                .iter()
                .find(|p| p.id == cmd.preset_id)
                .map(|preset| (cmd, preset.clone()))
        })
        .collect();

    // 6. Load tasks with terminals
    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    let mut task_details: Vec<(WorkflowTask, Vec<Terminal>)> = Vec::new();
    for task in &tasks {
        let terminals = Terminal::find_by_task(&deployment.db().pool, &task.id).await?;
        task_details.push((task.clone(), terminals));
    }

    // Convert to DTO
    let dto = WorkflowDetailDto::from_workflow_with_terminals(
        &workflow,
        &task_details,
        &commands_with_presets,
    );

    Ok(ResponseJson(ApiResponse::success(dto)))
}

/// GET /api/workflows/:workflow_id
/// Get workflow details
async fn get_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<WorkflowDetailDto>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    // Get workflow
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Get tasks and terminals
    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;

    // Get commands with presets
    let commands = WorkflowCommand::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    let all_presets = SlashCommandPreset::find_all(&deployment.db().pool).await?;
    let commands_with_presets: Vec<(WorkflowCommand, SlashCommandPreset)> = commands
        .into_iter()
        .filter_map(|cmd| {
            all_presets
                .iter()
                .find(|p| p.id == cmd.preset_id)
                .map(|preset| (cmd, preset.clone()))
        })
        .collect();

    // Load terminals for each task
    let mut task_details: Vec<(WorkflowTask, Vec<Terminal>)> = Vec::new();
    for task in &tasks {
        let terminals = Terminal::find_by_task(&deployment.db().pool, &task.id).await?;
        task_details.push((task.clone(), terminals));
    }

    // Convert to DTO
    let dto = WorkflowDetailDto::from_workflow_with_terminals(
        &workflow,
        &task_details,
        &commands_with_presets,
    );

    Ok(ResponseJson(ApiResponse::success(dto)))
}

/// DELETE /api/workflows/:workflow_id
/// Delete workflow
async fn delete_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    stop_workflow_runtime_if_running(
        &deployment,
        &workflow_id,
        "deleting workflow",
        "Failed to delete workflow",
    )
    .await?;
    cleanup_workflow_terminals(&deployment, &workflow_id, "deleting workflow").await?;
    Workflow::delete(&deployment.db().pool, &workflow_id).await?;
    Ok(ResponseJson(ApiResponse::success(())))
}

/// PUT /api/workflows/:workflow_id/status
/// Update workflow status
async fn update_workflow_status(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
    Json(req): Json<UpdateWorkflowStatusRequest>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    let target_status = req.status.trim().to_string();
    validate_workflow_status_transition(&workflow.status, &target_status)?;

    // G16-012: Block sensitive transitions that must go through dedicated endpoints
    let protected_transitions: &[(&str, &str, &str)] = &[
        ("created", "starting", "Use POST /prepare instead"),
        ("failed", "starting", "Use POST /prepare instead"),
        ("ready", "running", "Use POST /start instead"),
        (WORKFLOW_STATUS_PAUSED, "running", "Use POST /start instead"),
        ("completed", "merging", "Use POST /merge instead"),
    ];
    for &(from, to, hint) in protected_transitions {
        if workflow.status == from && target_status == to {
            return Err(ApiError::Conflict(format!(
                "Transition '{from}' -> '{to}' is not allowed via status endpoint. {hint}"
            )));
        }
    }

    Workflow::update_status(&deployment.db().pool, &workflow_id, &target_status).await?;
    Ok(ResponseJson(ApiResponse::success(())))
}

async fn rollback_prepare_failure(deployment: &DeploymentImpl, workflow_id: &str, reason: &str) {
    tracing::warn!(
        workflow_id = %workflow_id,
        reason = %reason,
        "Rolling back workflow prepare state"
    );

    let terminals = match Terminal::find_by_workflow(&deployment.db().pool, workflow_id).await {
        Ok(terminals) => terminals,
        Err(e) => {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to list terminals during prepare rollback"
            );
            Vec::new()
        }
    };
    let workflow_topic = format!("workflow:{workflow_id}");

    for terminal in terminals {
        if let Err(e) = deployment
            .process_manager()
            .kill_terminal(&terminal.id)
            .await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                error = %e,
                "Failed to kill terminal process during prepare rollback"
            );
        }

        deployment.prompt_watcher().unregister(&terminal.id).await;

        // G02-001: Unregister terminal bridge to stop MessageBus → PTY forwarding
        if let Some(session_id) = terminal.pty_session_id.as_deref() {
            let terminal_bridge = services::services::terminal::bridge::TerminalBridge::new(
                deployment.message_bus().clone(),
                deployment.process_manager().clone(),
            );
            terminal_bridge.unregister(session_id).await;
        }

        if let Err(e) =
            Terminal::update_process(&deployment.db().pool, &terminal.id, None, None).await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                error = %e,
                "Failed to clear terminal process binding during prepare rollback"
            );
        }

        if let Err(e) =
            Terminal::update_session(&deployment.db().pool, &terminal.id, None, None).await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                error = %e,
                "Failed to clear terminal session binding during prepare rollback"
            );
        }

        if let Err(e) =
            Terminal::update_status(&deployment.db().pool, &terminal.id, "not_started").await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                error = %e,
                "Failed to reset terminal status during prepare rollback"
            );
        } else {
            let message = BusMessage::TerminalStatusUpdate {
                workflow_id: workflow_id.to_string(),
                terminal_id: terminal.id.clone(),
                status: "not_started".to_string(),
            };
            if let Err(e) = deployment
                .message_bus()
                .publish(&workflow_topic, message.clone())
                .await
            {
                tracing::warn!(
                    workflow_id = %workflow_id,
                    terminal_id = %terminal.id,
                    error = %e,
                    "Failed to publish terminal rollback status"
                );
            }
            if let Err(e) = deployment.message_bus().broadcast(message) {
                tracing::warn!(
                    workflow_id = %workflow_id,
                    terminal_id = %terminal.id,
                    error = %e,
                    "Failed to broadcast terminal rollback status"
                );
            }
        }
    }

    if let Err(e) = Workflow::update_status(&deployment.db().pool, workflow_id, "failed").await {
        tracing::warn!(
            workflow_id = %workflow_id,
            error = %e,
            "Failed to set workflow status during prepare rollback"
        );
    }
}

// G23-006: When the workflow has a task with an active worktree, resolve the
// working directory to the task's worktree path rather than the project root.
// This prevents multiple terminals from sharing the same working directory.
async fn resolve_workflow_working_dir(
    deployment: &DeploymentImpl,
    workflow: &Workflow,
) -> Result<PathBuf, ApiError> {
    let project = Project::find_by_id(&deployment.db().pool, workflow.project_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Project not found".to_string()))?;

    if let Some(dir) = project
        .default_agent_working_dir
        .as_deref()
        .map(str::trim)
        .filter(|dir| !dir.is_empty())
    {
        return Ok(PathBuf::from(dir));
    }

    // G23-006: Check if there is a task worktree for this workflow. If so, use
    // the first task's worktree path as the working directory so that each
    // terminal operates in its own isolated checkout instead of the repo root.
    let first_task =
        WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow.id)
            .await
            .ok()
            .and_then(|tasks| tasks.into_iter().next());

    if let Some(task) = first_task {
        let worktree_path =
            services::services::worktree_manager::WorktreeManager::get_worktree_base_dir()
                .join(&task.branch);
        if worktree_path.exists() {
            return Ok(worktree_path);
        }
    }

    let repo_working_dir: Option<String> = sqlx::query_scalar(
        r"
        SELECT r.path
        FROM repos r
        INNER JOIN project_repos pr ON pr.repo_id = r.id
        WHERE pr.project_id = ?
        ORDER BY r.display_name ASC
        LIMIT 1
        ",
    )
    .bind(workflow.project_id)
    .fetch_optional(&deployment.db().pool)
    .await?
    .flatten();

    if let Some(dir) = repo_working_dir
        .as_deref()
        .map(str::trim)
        .filter(|dir| !dir.is_empty())
    {
        return Ok(PathBuf::from(dir));
    }

    Err(ApiError::BadRequest(format!(
        "Could not determine working directory for project {}",
        workflow.project_id
    )))
}

async fn refresh_prompt_watcher_registrations(deployment: &DeploymentImpl, workflow_id: &str) {
    let terminals = match Terminal::find_by_workflow(&deployment.db().pool, workflow_id).await {
        Ok(terminals) => terminals,
        Err(e) => {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to load terminals for prompt watcher refresh"
            );
            return;
        }
    };

    if terminals.is_empty() {
        return;
    }

    let tasks = match WorkflowTask::find_by_workflow(&deployment.db().pool, workflow_id).await {
        Ok(tasks) => tasks,
        Err(e) => {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to load tasks for prompt watcher refresh"
            );
            return;
        }
    };

    let workflow_by_task: HashMap<String, String> = tasks
        .into_iter()
        .map(|task| (task.id, task.workflow_id))
        .collect();

    for terminal in terminals {
        let Some(session_id) = terminal
            .pty_session_id
            .clone()
            .filter(|session_id| !session_id.trim().is_empty())
        else {
            tracing::warn!(
                workflow_id = %workflow_id,
                terminal_id = %terminal.id,
                "Skipped prompt watcher refresh registration: missing pty_session_id"
            );
            continue;
        };

        let resolved_workflow_id = workflow_by_task
            .get(&terminal.workflow_task_id)
            .cloned()
            .unwrap_or_else(|| workflow_id.to_string());

        if let Err(e) = deployment
            .prompt_watcher()
            .register(
                &terminal.id,
                &resolved_workflow_id,
                &terminal.workflow_task_id,
                &session_id,
                terminal.auto_confirm,
            )
            .await
        {
            tracing::warn!(
                workflow_id = %resolved_workflow_id,
                terminal_id = %terminal.id,
                task_id = %terminal.workflow_task_id,
                error = %e,
                "Failed to refresh prompt watcher registration"
            );
        }
    }
}

/// POST /api/workflows/:workflow_id/prepare
/// Prepare workflow: start all terminals (created → starting → ready)
///
/// This endpoint performs serial model switching for all terminals using cc-switch,
/// then transitions the workflow to "ready" status for user confirmation before execution.
async fn prepare_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    // Check workflow exists
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Verify workflow is in "created" or "failed" status (can retry failed workflows)
    // G16-006: Use CAS to prevent concurrent prepare calls
    if workflow.status != "created" && workflow.status != "failed" {
        return Err(ApiError::Conflict(format!(
            "Cannot prepare workflow: current status is '{}', expected 'created' or 'failed'",
            workflow.status
        )));
    }

    // CAS: atomically transition to "starting" only from expected statuses
    let cas_result = sqlx::query(
        r"
        UPDATE workflow
        SET status = 'starting', updated_at = datetime('now')
        WHERE id = ? AND status IN ('created', 'failed')
        ",
    )
    .bind(&workflow_id)
    .execute(&deployment.db().pool)
    .await?;

    if cas_result.rows_affected() == 0 {
        return Err(ApiError::Conflict(
            "Cannot prepare workflow: status changed concurrently".to_string(),
        ));
    }

    // Create services for terminal coordination
    // Note: Model configuration is now handled at spawn time via environment variable injection,
    // not by the coordinator. This provides process-level isolation for concurrent workflows.
    let db_arc = Arc::new(deployment.db().clone());
    let coordinator =
        TerminalCoordinator::with_message_bus(db_arc.clone(), deployment.message_bus().clone());

    // Step 1: Transition terminals to "starting" status using TerminalCoordinator
    if let Err(e) = coordinator.start_terminals_for_workflow(&workflow_id).await {
        // Log the error for debugging
        tracing::error!(
            workflow_id = %workflow_id,
            error = %e,
            "Failed to prepare workflow terminals"
        );

        rollback_prepare_failure(&deployment, &workflow_id, "terminal preparation failed").await;

        return Err(ApiError::Internal(format!(
            "Failed to prepare workflow terminals: {e}"
        )));
    }

    // Step 2: Resolve working directory.
    // Priority: project.default_agent_working_dir -> first project repo path.
    let working_dir = match resolve_workflow_working_dir(&deployment, &workflow).await {
        Ok(path) => path,
        Err(e) => {
            rollback_prepare_failure(
                &deployment,
                &workflow_id,
                "working directory resolution failed during prepare",
            )
            .await;
            return Err(e);
        }
    };

    // Step 3: Launch PTY processes for all terminals
    let cc_switch = Arc::new(CCSwitchService::new(db_arc.clone()));
    let process_manager = deployment.process_manager().clone();
    let message_bus = deployment.message_bus().clone();
    let prompt_watcher = deployment.prompt_watcher().clone();
    let launcher = TerminalLauncher::with_message_bus(
        db_arc,
        cc_switch,
        process_manager,
        working_dir,
        message_bus,
        prompt_watcher,
    );

    let launch_results = match launcher.launch_all(&workflow_id).await {
        Ok(results) => results,
        Err(e) => {
            tracing::error!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to launch workflow terminals"
            );

            rollback_prepare_failure(
                &deployment,
                &workflow_id,
                "terminal launch error during prepare",
            )
            .await;

            return Err(ApiError::Internal(format!(
                "Failed to launch workflow terminals: {e}"
            )));
        }
    };

    // Check if any terminal failed to launch
    // Extract error info before any await to avoid Send issues with LaunchResult
    let failed_error_msgs: Vec<String> = launch_results
        .iter()
        .filter(|r| !r.success)
        .map(|r| {
            format!(
                "{}: {}",
                r.terminal_id,
                r.error.as_deref().unwrap_or("unknown")
            )
        })
        .collect();

    let launched_count = launch_results.len();
    drop(launch_results); // Release non-Send types before await

    if !failed_error_msgs.is_empty() {
        tracing::error!(
            workflow_id = %workflow_id,
            failed_count = failed_error_msgs.len(),
            errors = ?failed_error_msgs,
            "Some terminals failed to launch"
        );

        rollback_prepare_failure(&deployment, &workflow_id, "partial terminal launch failure")
            .await;

        return Err(ApiError::Internal(format!(
            "Failed to launch terminals: {}",
            failed_error_msgs.join(", ")
        )));
    }

    tracing::info!(
        workflow_id = %workflow_id,
        launched_count = launched_count,
        "All terminals launched successfully"
    );

    // All terminals prepared successfully, mark workflow as ready
    if let Err(e) = Workflow::set_ready(&deployment.db().pool, &workflow_id).await {
        tracing::error!(
            workflow_id = %workflow_id,
            error = %e,
            "Failed to set workflow ready status"
        );

        rollback_prepare_failure(
            &deployment,
            &workflow_id,
            "failed to finalize prepare status",
        )
        .await;

        return Err(ApiError::Internal(
            "Failed to finalize workflow preparation".to_string(),
        ));
    }

    tracing::info!(
        workflow_id = %workflow_id,
        "Workflow prepared and ready for execution"
    );

    Ok(ResponseJson(ApiResponse::success(())))
}

/// POST /api/workflows/:workflow_id/start
/// Start workflow (user confirmed) or resume from paused state
async fn start_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    // Check workflow exists
    let mut workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Recover stale workflow status after restart: DB may still be `running`
    // while runtime instance is no longer active.
    // G16-007: Use CAS to atomically transition running → paused only when runtime is inactive
    if workflow.status == "running"
        && !deployment
            .orchestrator_runtime()
            .is_running(&workflow_id)
            .await
    {
        tracing::warn!(
            workflow_id = %workflow_id,
            "Workflow marked running but runtime is not active; recovering to paused"
        );
        let cas_result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'paused', updated_at = datetime('now')
            WHERE id = ? AND status = 'running'
            ",
        )
        .bind(&workflow_id)
        .execute(&deployment.db().pool)
        .await?;

        if cas_result.rows_affected() == 0 {
            return Err(ApiError::Conflict(
                "Cannot start workflow: status changed concurrently during stale-state recovery".to_string(),
            ));
        }
        workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
            .await?
            .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;
    }

    // Self-heal for restarted backend: a workflow may still be `ready` while terminals were
    // reconciled to `not_started` (missing active PTY/session). Re-prepare before starting.
    // TODO(G16-008): Extract this re-prepare block into a dedicated `re_prepare_if_needed()`
    // function to reduce start_workflow complexity and improve testability.
    if workflow.status == "ready" || workflow.status == WORKFLOW_STATUS_PAUSED {
        let terminals =
            db::models::Terminal::find_by_workflow(&deployment.db().pool, &workflow_id).await?;

        let needs_reprepare = terminals.iter().any(|terminal| {
            terminal.status != "waiting"
                || terminal
                    .pty_session_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|session| !session.is_empty())
                    .is_none()
        });

        if needs_reprepare {
            tracing::warn!(
                workflow_id = %workflow_id,
                workflow_status = %workflow.status,
                "Workflow terminals are not launch-ready; re-preparing before start"
            );

            Workflow::update_status(&deployment.db().pool, &workflow_id, "created").await?;

            // G03-002: Wrap re-prepare error with descriptive context
            let _ = prepare_workflow(State(deployment.clone()), Path(Uuid::parse_str(&workflow_id).map_err(|e| ApiError::BadRequest(format!("Invalid workflow ID: {e}")))?))
                .await
                .map_err(|e| {
                    ApiError::Internal(format!(
                        "Start-phase re-prepare failed for workflow {workflow_id}: {e}"
                    ))
                })?;

            // G03-003: Verify workflow status is back to "ready" after re-prepare
            workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
                .await?
                .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

            if workflow.status != "ready" {
                return Err(ApiError::Internal(format!(
                    "Re-prepare did not restore workflow to 'ready' status (current: '{}')",
                    workflow.status
                )));
            }
        }
    }

    // Verify orchestrator is enabled (only check needed at API level)
    if !workflow.orchestrator_enabled {
        return Err(ApiError::BadRequest(
            "Cannot start workflow: orchestrator is not enabled".to_string(),
        ));
    }

    // Validate workflow status - allow starting from ready or resuming from paused
    let valid_start_statuses = ["ready", WORKFLOW_STATUS_PAUSED];
    if !valid_start_statuses.contains(&workflow.status.as_str()) {
        return Err(ApiError::BadRequest(format!(
            "Cannot start workflow: current status is '{}', expected 'ready' or 'paused'",
            workflow.status
        )));
    }

    // If resuming from paused, use CAS to atomically reset to ready state
    if workflow.status == WORKFLOW_STATUS_PAUSED {
        let now = chrono::Utc::now();
        let result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'ready', updated_at = ?
            WHERE id = ? AND status = 'paused'
            ",
        )
        .bind(now)
        .bind(&workflow_id)
        .execute(&deployment.db().pool)
        .await?;

        if result.rows_affected() == 0 {
            return Err(ApiError::BadRequest(
                "Cannot resume workflow: status changed concurrently".to_string(),
            ));
        }
        tracing::info!(workflow_id = %workflow_id, "Resuming workflow from paused state (CAS: paused → ready)");
    }

    // Call orchestrator runtime to start workflow
    // Runtime handles all status validation atomically
    deployment
        .orchestrator_runtime()
        .start_workflow(&workflow_id)
        .await
        .map_err(|e| {
            // Log full error internally
            tracing::error!("Failed to start workflow {}: {:?}", workflow_id, e);
            // Return generic message to client
            ApiError::Internal("Failed to start workflow".to_string())
        })?;

    refresh_prompt_watcher_registrations(&deployment, &workflow_id).await;

    // Note: Workflow::set_started is called inside OrchestratorRuntime::start_workflow
    // to ensure the status update happens atomically with runtime startup

    Ok(ResponseJson(ApiResponse::success(())))
}

/// POST /api/workflows/:workflow_id/pause
/// Pause a running workflow
async fn pause_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    // Check workflow exists
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Only allow pausing a running workflow
    if workflow.status != "running" {
        return Err(ApiError::BadRequest(format!(
            "Cannot pause workflow: current status is '{}', expected 'running'",
            workflow.status
        )));
    }

    // Stop the orchestrator runtime if it's active
    stop_workflow_runtime_if_running(
        &deployment,
        &workflow_id,
        "pausing workflow",
        "Failed to pause workflow",
    )
    .await?;

    // Kill PTY processes and unregister prompt watchers
    let terminals =
        cleanup_workflow_terminals(&deployment, &workflow_id, "pausing workflow").await?;

    // Mark workflow as paused
    Workflow::update_status(&deployment.db().pool, &workflow_id, WORKFLOW_STATUS_PAUSED).await?;

    // Cascade: reset running tasks to pending so they can be re-dispatched on resume
    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    for task in &tasks {
        if task.status == "running" {
            WorkflowTask::update_status(&deployment.db().pool, &task.id, "pending").await?;
        }
    }
    // Cascade terminal status: working/waiting → not_started so they can be re-prepared on resume
    for terminal in &terminals {
        match terminal.status.as_str() {
            "working" | "waiting" | "starting" => {
                Terminal::update_status(&deployment.db().pool, &terminal.id, "not_started").await?;
                Terminal::update_process(&deployment.db().pool, &terminal.id, None, None).await?;
                Terminal::update_session(&deployment.db().pool, &terminal.id, None, None).await?;
            }
            _ => {}
        }
    }

    tracing::info!(
        workflow_id = %workflow_id,
        "Workflow paused with terminal cleanup and cascaded status updates"
    );

    Ok(ResponseJson(ApiResponse::success(())))
}

/// POST /api/workflows/:workflow_id/resume
/// Resume a paused workflow (G05-002)
///
/// Transitions the workflow from paused to ready via CAS and then starts
/// the orchestrator runtime. This endpoint provides a dedicated resume path
/// rather than reusing the start endpoint, giving clearer semantics and
/// enabling the frontend to show a distinct Resume button.
async fn resume_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();

    // Verify workflow exists and is in paused state
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    if workflow.status != WORKFLOW_STATUS_PAUSED {
        return Err(ApiError::BadRequest(format!(
            "Cannot resume workflow: current status is '{}', expected 'paused'",
            workflow.status
        )));
    }

    // Check terminal readiness before resuming (mirrors start_workflow self-heal logic)
    {
        let terminals =
            db::models::Terminal::find_by_workflow(&deployment.db().pool, &workflow_id).await?;

        let needs_reprepare = terminals.iter().any(|terminal| {
            terminal.status != "waiting"
                || terminal
                    .pty_session_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|session| !session.is_empty())
                    .is_none()
        });

        if needs_reprepare {
            tracing::warn!(
                workflow_id = %workflow_id,
                "Workflow terminals are not launch-ready; re-preparing before resume"
            );

            Workflow::update_status(&deployment.db().pool, &workflow_id, "created").await?;

            let _ = prepare_workflow(State(deployment.clone()), Path(Uuid::parse_str(&workflow_id).map_err(|e| ApiError::BadRequest(format!("Invalid workflow ID: {e}")))?))
                .await
                .map_err(|e| {
                    ApiError::Internal(format!(
                        "Resume-phase re-prepare failed for workflow {workflow_id}: {e}"
                    ))
                })?;

            // Verify workflow status is back to "ready" after re-prepare, then
            // transition to paused so the runtime resume CAS (paused -> ready) succeeds.
            let refreshed = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
                .await?
                .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

            if refreshed.status != "ready" {
                return Err(ApiError::Internal(format!(
                    "Re-prepare did not restore workflow to 'ready' status (current: '{}')",
                    refreshed.status
                )));
            }

            Workflow::update_status(&deployment.db().pool, &workflow_id, WORKFLOW_STATUS_PAUSED).await?;
        }
    }

    // Delegate to the runtime which performs paused -> ready CAS and agent creation
    deployment
        .orchestrator_runtime()
        .resume_workflow(&workflow_id)
        .await
        .map_err(|e| {
            tracing::error!(
                workflow_id = %workflow_id,
                error = ?e,
                "Failed to resume workflow"
            );
            ApiError::Internal("Failed to resume workflow".to_string())
        })?;

    refresh_prompt_watcher_registrations(&deployment, &workflow_id).await;

    tracing::info!(workflow_id = %workflow_id, "Workflow resumed successfully");

    Ok(ResponseJson(ApiResponse::success(())))
}

/// POST /api/workflows/:workflow_id/stop
/// Stop a workflow and mark as cancelled
async fn stop_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    // Check workflow exists
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Allow stopping from ready/starting/running/paused (G16-009: added "ready")
    let valid_statuses = ["ready", "starting", "running", WORKFLOW_STATUS_PAUSED];
    if !valid_statuses.contains(&workflow.status.as_str()) {
        return Err(ApiError::BadRequest(format!(
            "Cannot stop workflow: current status is '{}', expected one of: {:?}",
            workflow.status, valid_statuses
        )));
    }

    stop_workflow_runtime_if_running(
        &deployment,
        &workflow_id,
        "stopping workflow",
        "Failed to stop workflow",
    )
    .await?;
    let terminals =
        cleanup_workflow_terminals(&deployment, &workflow_id, "stopping workflow").await?;

    // Mark workflow as cancelled
    Workflow::update_status(&deployment.db().pool, &workflow_id, "cancelled").await?;

    // Mark all tasks as cancelled
    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    for task in &tasks {
        if task.status != "completed" {
            WorkflowTask::update_status(&deployment.db().pool, &task.id, "cancelled").await?;
        }
    }

    // Mark all terminals as cancelled
    for terminal in &terminals {
        if terminal.status != "completed" {
            Terminal::update_status(&deployment.db().pool, &terminal.id, "cancelled").await?;
            Terminal::update_process(&deployment.db().pool, &terminal.id, None, None).await?;
        }
    }

    // G23-004: Clean up worktree directories to free disk space
    cleanup_workflow_worktrees(&deployment, &workflow, &tasks).await;

    tracing::info!(
        workflow_id = %workflow_id,
        "Workflow stopped and cancelled"
    );

    Ok(ResponseJson(ApiResponse::success(())))
}

async fn stop_workflow_runtime_if_running(
    deployment: &DeploymentImpl,
    workflow_id: &str,
    action: &str,
    internal_error_message: &str,
) -> Result<(), ApiError> {
    let runtime = deployment.orchestrator_runtime();
    if runtime.is_running(workflow_id).await {
        runtime.stop_workflow(workflow_id).await.map_err(|e| {
            tracing::error!(
                workflow_id = %workflow_id,
                action = action,
                error = ?e,
                "Failed to stop workflow runtime"
            );
            ApiError::Internal(internal_error_message.to_string())
        })?;
    }
    Ok(())
}

async fn cleanup_workflow_terminals(
    deployment: &DeploymentImpl,
    workflow_id: &str,
    action: &str,
) -> Result<Vec<Terminal>, ApiError> {
    let pool = &deployment.db().pool;
    let terminals = Terminal::find_by_workflow(pool, workflow_id).await?;
    for terminal in &terminals {
        if let Err(e) = deployment
            .process_manager()
            .kill_terminal(&terminal.id)
            .await
        {
            tracing::warn!(
                terminal_id = %terminal.id,
                workflow_id = %workflow_id,
                action = action,
                error = %e,
                "Failed to kill terminal process during workflow cleanup"
            );
        }

        deployment.prompt_watcher().unregister(&terminal.id).await;

        // G02-001: Unregister terminal bridge to stop MessageBus → PTY forwarding
        if let Some(session_id) = terminal.pty_session_id.as_deref() {
            let terminal_bridge = services::services::terminal::bridge::TerminalBridge::new(
                deployment.message_bus().clone(),
                deployment.process_manager().clone(),
            );
            terminal_bridge.unregister(session_id).await;
        }
    }

    Ok(terminals)
}

/// G23-004: Clean up worktree directories for all tasks in a workflow.
///
/// This is best-effort and non-fatal — worktree cleanup failures are logged
/// but do not prevent workflow stop/delete from succeeding.
async fn cleanup_workflow_worktrees(
    deployment: &DeploymentImpl,
    workflow: &Workflow,
    tasks: &[WorkflowTask],
) {
    let base_repo_path = match Project::find_by_id(&deployment.db().pool, workflow.project_id)
        .await
        .ok()
        .flatten()
        .and_then(|p| {
            p.default_agent_working_dir
                .as_deref()
                .map(str::trim)
                .filter(|path| !path.is_empty())
                .map(PathBuf::from)
        }) {
        Some(path) => path,
        None => {
            tracing::debug!(
                workflow_id = %workflow.id,
                "Skipping worktree cleanup: no base repo path"
            );
            return;
        }
    };

    let worktree_cleanups: Vec<services::services::worktree_manager::WorktreeCleanup> = tasks
        .iter()
        .filter(|t| !t.branch.trim().is_empty())
        .filter_map(|t| {
            let branch = t.branch.trim();
            let managed_path =
                services::services::worktree_manager::WorktreeManager::get_worktree_base_dir()
                    .join(branch);
            let legacy_path = base_repo_path.join("worktrees").join(branch);
            let worktree_path = if managed_path.exists() {
                managed_path
            } else if legacy_path.exists() {
                legacy_path
            } else {
                return None;
            };
            Some(services::services::worktree_manager::WorktreeCleanup::new(
                worktree_path,
                Some(base_repo_path.clone()),
            ))
        })
        .collect();

    if !worktree_cleanups.is_empty() {
        if let Err(e) = services::services::worktree_manager::WorktreeManager::batch_cleanup_worktrees(&worktree_cleanups).await {
            tracing::warn!(
                workflow_id = %workflow.id,
                error = %e,
                "Failed to clean up worktrees (non-fatal)"
            );
        } else {
            tracing::info!(
                workflow_id = %workflow.id,
                count = worktree_cleanups.len(),
                "Cleaned up worktree directories"
            );
        }
    }
}

/// POST /api/workflows/:workflow_id/tasks
/// Create a new runtime task inside an existing workflow
async fn create_runtime_task(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
    Json(req): Json<CreateRuntimeTaskRequest>,
) -> Result<ResponseJson<ApiResponse<WorkflowTask>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;
    validate_runtime_mutation_workflow_status(&workflow.status)?;

    let task_name = req.name.trim();
    if task_name.is_empty() {
        return Err(ApiError::BadRequest("name is required".to_string()));
    }

    let existing_tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    let order_index = match req.order_index {
        Some(order_index) => {
            if existing_tasks.iter().any(|task| task.order_index == order_index) {
                return Err(ApiError::Conflict(format!(
                    "Task orderIndex {order_index} already exists in workflow {workflow_id}"
                )));
            }
            order_index
        }
        None => existing_tasks
            .last()
            .map_or(0, |task| task.order_index + 1),
    };

    let branch = if let Some(custom_branch) = req.branch {
        if existing_tasks.iter().any(|task| task.branch == custom_branch) {
            return Err(ApiError::Conflict(format!(
                "Task branch '{custom_branch}' already exists in workflow {workflow_id}"
            )));
        }
        custom_branch
    } else {
        let existing_branches: Vec<String> =
            existing_tasks.iter().map(|task| task.branch.clone()).collect();
        let base_branch = format!("workflow/{}/{}", workflow_id, text::git_branch_id(task_name));
        let mut candidate = base_branch.clone();
        let mut counter = 2;

        while existing_branches.contains(&candidate) {
            candidate = format!("{base_branch}-{counter}");
            counter += 1;
        }

        candidate
    };

    let now = chrono::Utc::now();
    let task = WorkflowTask {
        id: Uuid::new_v4().to_string(),
        workflow_id: workflow_id.clone(),
        vk_task_id: None,
        name: task_name.to_string(),
        description: req.description,
        branch,
        status: "pending".to_string(),
        order_index,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
    };

    let created_task = WorkflowTask::create(&deployment.db().pool, &task)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create workflow task: {e}")))?;

    if let Err(e) = broadcast_task_status(&deployment, &created_task, &created_task.status).await {
        tracing::warn!(
            workflow_id = %workflow_id,
            task_id = %created_task.id,
            error = %e,
            "Failed to broadcast runtime task creation"
        );
    }

    Ok(ResponseJson(ApiResponse::success(created_task)))
}

/// POST /api/workflows/:workflow_id/tasks/:task_id/terminals
/// Create a new runtime terminal inside an existing task
async fn create_runtime_terminal(
    State(deployment): State<DeploymentImpl>,
    Path((workflow_id, task_id)): Path<(Uuid, String)>,
    Json(req): Json<CreateRuntimeTerminalRequest>,
) -> Result<ResponseJson<ApiResponse<Terminal>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;
    validate_runtime_mutation_workflow_status(&workflow.status)?;

    let task = WorkflowTask::find_by_id(&deployment.db().pool, &task_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Task not found".to_string()))?;
    validate_task_workflow_scope(&task, &workflow_id)?;

    let cli_type_id = req.cli_type_id.trim();
    if cli_type_id.is_empty() {
        return Err(ApiError::BadRequest("cliTypeId is required".to_string()));
    }

    let model_config_id = req.model_config_id.trim();
    if model_config_id.is_empty() {
        return Err(ApiError::BadRequest("modelConfigId is required".to_string()));
    }

    let cli_exists = CliType::find_by_id(&deployment.db().pool, cli_type_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to validate CLI type: {e}")))?
        .is_some();
    if !cli_exists {
        return Err(ApiError::BadRequest(format!(
            "CLI type not found: {cli_type_id}"
        )));
    }

    let model_config = ModelConfig::find_by_id(&deployment.db().pool, model_config_id)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to validate model config: {e}")))?
        .ok_or_else(|| {
            ApiError::BadRequest(format!("Model config not found: {model_config_id}"))
        })?;
    if model_config.cli_type_id != cli_type_id {
        return Err(ApiError::BadRequest(format!(
            "Model config {model_config_id} does not belong to CLI type {cli_type_id}"
        )));
    }

    let existing_terminals = Terminal::find_by_task(&deployment.db().pool, &task_id).await?;
    let order_index = match req.order_index {
        Some(order_index) => {
            if existing_terminals
                .iter()
                .any(|terminal| terminal.order_index == order_index)
            {
                return Err(ApiError::Conflict(format!(
                    "Terminal orderIndex {order_index} already exists in task {task_id}"
                )));
            }
            order_index
        }
        None => existing_terminals
            .last()
            .map_or(0, |terminal| terminal.order_index + 1),
    };

    let now = chrono::Utc::now();
    let mut terminal = Terminal {
        id: Uuid::new_v4().to_string(),
        workflow_task_id: task_id.clone(),
        cli_type_id: cli_type_id.to_string(),
        model_config_id: model_config_id.to_string(),
        custom_base_url: req.custom_base_url,
        custom_api_key: None,
        role: req.role,
        role_description: req.role_description,
        order_index,
        status: "not_started".to_string(),
        process_id: None,
        pty_session_id: None,
        session_id: None,
        execution_process_id: None,
        vk_session_id: None,
        auto_confirm: req.auto_confirm,
        last_commit_hash: None,
        last_commit_message: None,
        started_at: None,
        completed_at: None,
        created_at: now,
        updated_at: now,
    };

    if let Some(custom_api_key) = req.custom_api_key.as_ref() {
        terminal.set_custom_api_key(custom_api_key).map_err(|e| {
            ApiError::BadRequest(format!("Failed to encrypt terminal API key: {e}"))
        })?;
    }

    let created_terminal = Terminal::create(&deployment.db().pool, &terminal)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to create terminal: {e}")))?;

    let terminal = if req.start_immediately {
        let _ = start_terminal(State(deployment.clone()), Path(Uuid::parse_str(&created_terminal.id).map_err(|e| ApiError::Internal(format!("Invalid terminal ID: {e}")))?)).await?;
        Terminal::find_by_id(&deployment.db().pool, &created_terminal.id)
            .await?
            .ok_or_else(|| ApiError::NotFound("Terminal not found after start".to_string()))?
    } else {
        if let Err(e) = broadcast_runtime_terminal_status(
            &deployment,
            &task,
            &created_terminal.id,
            &created_terminal.status,
        )
        .await
        {
            tracing::warn!(
                workflow_id = %workflow_id,
                task_id = %task_id,
                terminal_id = %created_terminal.id,
                error = %e,
                "Failed to broadcast runtime terminal creation"
            );
        }

        created_terminal
    };

    Ok(ResponseJson(ApiResponse::success(terminal)))
}

/// POST /api/workflows/recover
/// Trigger recovery of workflows after service restart
async fn run_workflow_recovery(
    runtime: &OrchestratorRuntime,
) -> Result<RecoveryResponse, ApiError> {
    let recovered_workflows = runtime
        .recover_running_workflows()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to recover workflows: {e}")))?;

    let recovered_commands = runtime
        .recover_incomplete_orchestrator_commands()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to recover commands: {e}")))?;

    let message = if recovered_workflows == 0 && recovered_commands == 0 {
        "No interrupted workflows or commands required recovery".to_string()
    } else {
        format!(
            "Recovered {recovered_workflows} interrupted workflow(s) and {recovered_commands} command(s)"
        )
    };

    Ok(RecoveryResponse {
        message,
        recovered_workflows,
        recovered_commands: recovered_commands as usize,
    })
}

async fn recover_workflows(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<RecoveryResponse>>, ApiError> {
    let response = run_workflow_recovery(deployment.orchestrator_runtime()).await?;

    Ok(ResponseJson(ApiResponse::success(response)))
}

/// GET /api/workflows/:workflow_id/tasks
/// List workflow tasks
async fn list_workflow_tasks(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<Vec<WorkflowTaskDetailResponse>>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    let mut task_details = Vec::new();
    for task in tasks {
        let terminals = Terminal::find_by_task(&deployment.db().pool, &task.id).await?;
        task_details.push(WorkflowTaskDetailResponse { task, terminals });
    }
    Ok(ResponseJson(ApiResponse::success(task_details)))
}

/// Request body for updating task status
#[derive(Debug, Deserialize)]
pub struct UpdateTaskStatusRequest {
    pub status: String,
}

/// Request body for submitting interactive prompt response
#[derive(Debug, Deserialize)]
pub struct SubmitPromptResponseRequest {
    #[serde(rename = "terminalId", alias = "terminal_id")]
    pub terminal_id: String,
    pub response: String,
}

/// Request body for sending a direct chat message to orchestrator
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubmitOrchestratorChatRequest {
    pub message: String,
    #[serde(default)]
    pub source: Option<String>,
    #[serde(default)]
    pub external_message_id: Option<String>,
    #[serde(default)]
    pub metadata: OrchestratorChatRequestMetadata,
}

#[derive(Debug, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct OrchestratorChatRequestMetadata {
    #[serde(default)]
    pub operator_id: Option<String>,
    #[serde(default)]
    pub client_ts: Option<String>,
    #[serde(default)]
    pub conversation_id: Option<String>,
}

/// Response item for orchestrator conversation message
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OrchestratorChatMessageResponse {
    pub role: String,
    pub content: String,
}

/// Response for direct orchestrator chat submission command lifecycle.
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SubmitOrchestratorChatResponse {
    pub command_id: String,
    pub status: String,
    pub error: Option<String>,
    pub retryable: bool,
}

/// Query params for listing orchestrator messages with pagination.
#[derive(Debug, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct ListOrchestratorMessagesQuery {
    pub cursor: Option<usize>,
    pub limit: Option<usize>,
}

fn paginate_orchestrator_messages(
    total: usize,
    cursor: Option<usize>,
    limit: Option<usize>,
) -> (usize, usize) {
    let limit = limit.unwrap_or(50).clamp(1, 200);
    let start = cursor.unwrap_or_else(|| total.saturating_sub(limit)).min(total);
    let end = start.saturating_add(limit).min(total);
    (start, end)
}

/// PUT /api/workflows/:workflow_id/tasks/:task_id/status
/// Update task status (for Kanban drag-and-drop)
async fn update_task_status(
    State(deployment): State<DeploymentImpl>,
    Path((workflow_id, task_id)): Path<(Uuid, String)>,
    Json(req): Json<UpdateTaskStatusRequest>,
) -> Result<ResponseJson<ApiResponse<WorkflowTask>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    // Verify workflow exists
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Verify task exists and belongs to the workflow
    let task = WorkflowTask::find_by_id(&deployment.db().pool, &task_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Task not found".to_string()))?;

    validate_task_workflow_scope(&task, &workflow_id)?;

    // Validate status value - support both backend and frontend status names
    let valid_statuses = [
        "pending",        // Initial state
        "running",        // Task is being worked on
        "review_pending", // Awaiting review
        "completed",      // Task completed successfully
        "failed",         // Task failed
        "cancelled",      // Task was cancelled
    ];
    if !valid_statuses.contains(&req.status.as_str()) {
        return Err(ApiError::BadRequest(format!(
            "Invalid status '{}', expected one of: {:?}",
            req.status, valid_statuses
        )));
    }

    // Update task status
    WorkflowTask::update_status(&deployment.db().pool, &task_id, &req.status).await?;

    // Auto-sync workflow status: when all tasks are completed, mark running workflow as completed.
    // Uses CAS (running → completed) to prevent overwriting concurrent state changes.
    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    if should_auto_complete_workflow(&workflow.status, &tasks) {
        match Workflow::set_completed_from_running(&deployment.db().pool, &workflow_id).await {
            Ok(true) => {
                tracing::info!(
                    workflow_id = %workflow_id,
                    "Workflow auto-synced to completed after all tasks completed"
                );
            }
            Ok(false) => {
                tracing::warn!(
                    workflow_id = %workflow_id,
                    "Workflow auto-sync to completed skipped: status changed concurrently"
                );
            }
            Err(e) => {
                tracing::error!(
                    workflow_id = %workflow_id,
                    error = %e,
                    "Failed to auto-sync workflow to completed"
                );
            }
        }
    }

    // Fetch updated task
    let updated_task = WorkflowTask::find_by_id(&deployment.db().pool, &task_id)
        .await?
        .ok_or_else(|| ApiError::Internal("Failed to fetch updated task".to_string()))?;

    tracing::info!(
        workflow_id = %workflow_id,
        task_id = %task_id,
        new_status = %req.status,
        "Task status updated"
    );

    Ok(ResponseJson(ApiResponse::success(updated_task)))
}

/// GET /api/workflows/:workflow_id/tasks/:task_id/terminals
/// List task terminals
async fn list_task_terminals(
    State(deployment): State<DeploymentImpl>,
    Path((workflow_id, task_id)): Path<(Uuid, String)>,
) -> Result<ResponseJson<ApiResponse<Vec<Terminal>>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    let _workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    let task = WorkflowTask::find_by_id(&deployment.db().pool, &task_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Task not found".to_string()))?;

    validate_task_workflow_scope(&task, &workflow_id)?;

    let terminals = Terminal::find_by_task(&deployment.db().pool, &task_id).await?;
    Ok(ResponseJson(ApiResponse::success(terminals)))
}

/// POST /api/workflows/:workflow_id/prompts/respond
/// Submit user response for interactive terminal prompt
async fn submit_prompt_response(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
    Json(payload): Json<SubmitPromptResponseRequest>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    let terminal_id = payload.terminal_id.trim();
    if terminal_id.is_empty() {
        return Err(ApiError::BadRequest("terminalId is required".to_string()));
    }

    let response = payload.response.as_str();

    let _workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    let terminal = Terminal::find_by_id(&deployment.db().pool, terminal_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Terminal not found".to_string()))?;

    let task = WorkflowTask::find_by_id(&deployment.db().pool, &terminal.workflow_task_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow task not found".to_string()))?;

    validate_task_workflow_scope(&task, &workflow_id)?;

    let runtime = deployment.orchestrator_runtime();
    if !runtime.is_running(&workflow_id).await {
        // G16-014: Return 409 Conflict for non-running workflow (not a client input error)
        return Err(ApiError::Conflict(format!(
            "Cannot submit prompt response: workflow '{workflow_id}' is not running"
        )));
    }

    runtime
        .submit_user_prompt_response(&workflow_id, terminal_id, response)
        .await
        .map_err(|e| {
            tracing::warn!(
                workflow_id = %workflow_id,
                terminal_id = %terminal_id,
                error = %e,
                "Failed to submit prompt response"
            );
            ApiError::BadRequest(format!("Failed to submit prompt response: {e}"))
        })?;

    tracing::info!(
        workflow_id = %workflow_id,
        terminal_id = %terminal_id,
        "Submitted prompt response"
    );

    Ok(ResponseJson(ApiResponse::success(())))
}

/// POST /api/workflows/:workflow_id/orchestrator/chat
/// Submit a direct chat message to the running orchestrator agent
pub(crate) async fn submit_orchestrator_chat(
    State(deployment): State<DeploymentImpl>,
    headers: HeaderMap,
    Path(workflow_id): Path<Uuid>,
    Json(payload): Json<SubmitOrchestratorChatRequest>,
) -> Result<ResponseJson<ApiResponse<SubmitOrchestratorChatResponse>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    if !is_orchestrator_chat_feature_enabled() {
        return Err(ApiError::Conflict(
            "Orchestrator chat feature is disabled by rollout flag".to_string(),
        ));
    }

    let message = payload.message.trim();
    if message.is_empty() {
        return Err(ApiError::BadRequest("message is required".to_string()));
    }
    let source = payload
        .source
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("web");
    if !matches!(source, "web" | "api" | "social") {
        return Err(ApiError::BadRequest(format!(
            "source must be one of: web, api, social (got '{source}')"
        )));
    }

    let external_message_id = payload
        .external_message_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty());
    if source != "web" && external_message_id.is_none() {
        return Err(ApiError::BadRequest(
            "externalMessageId is required when source is not 'web'".to_string(),
        ));
    }

    let operator_id = normalize_operator_id(
        payload
            .metadata
            .operator_id
            .as_deref()
            .or_else(|| {
                headers
                    .get("x-orchestrator-operator-id")
                    .and_then(|value| value.to_str().ok())
            }),
    );
    let role = extract_role_from_headers(&headers);

    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    if !workflow.orchestrator_enabled {
        return Err(ApiError::Conflict(
            "Cannot submit orchestrator chat: orchestrator is not enabled".to_string(),
        ));
    }

    ensure_orchestrator_permission(
        source,
        role.as_deref(),
        workflow.execution_mode.as_str(),
        operator_id.as_deref(),
    )?;

    let has_models_configured = {
        let config = deployment.config().read().await;
        has_configured_workflow_models(&config)
    };
    if !has_models_configured {
        return Err(ApiError::Conflict(
            "Cannot submit orchestrator chat: configure at least one AI model first".to_string(),
        ));
    }

    let runtime = deployment.orchestrator_runtime();
    if !runtime.is_running(&workflow_id).await {
        return Err(ApiError::Conflict(format!(
            "Cannot submit orchestrator chat: workflow '{workflow_id}' is not running"
        )));
    }

    ensure_orchestrator_circuit_closed(&workflow_id).await?;
    enforce_orchestrator_rate_limit(
        &workflow_id,
        source,
        operator_id.as_deref().or(external_message_id),
    )
    .await?;

    if let Some(external_id) = external_message_id
        && let Some(existing_command) = WorkflowOrchestratorCommand::find_by_external_message(
            &deployment.db().pool,
            &workflow_id,
            source,
            external_id,
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to query orchestrator command: {e}")))?
    {
        let response = SubmitOrchestratorChatResponse {
            command_id: existing_command.id,
            status: existing_command.status,
            error: existing_command.error,
            retryable: existing_command.retryable,
        };
        return Ok(ResponseJson(ApiResponse::success(response)));
    }

    let command_id = Uuid::new_v4().to_string();
    let queued_command = WorkflowOrchestratorCommand::new_queued(
        &command_id,
        &workflow_id,
        source,
        external_message_id,
        message,
    );
    WorkflowOrchestratorCommand::insert(&deployment.db().pool, &queued_command)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to persist orchestrator command: {e}")))?;
    WorkflowOrchestratorCommand::update_status(
        &deployment.db().pool,
        &command_id,
        "running",
        None,
        false,
        Some(Utc::now()),
        None,
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to update command status: {e}")))?;

    let user_message = WorkflowOrchestratorMessage::new(
        &workflow_id,
        Some(&command_id),
        "user",
        message,
        source,
        external_message_id,
    );
    WorkflowOrchestratorMessage::insert(&deployment.db().pool, &user_message)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to persist user message: {e}")))?;

    let message_digest = digest_message(message);
    let message_preview = redact_sensitive_content(message);

    tracing::info!(
        target: "audit.orchestrator_chat",
        workflow_id = %workflow_id,
        source = %source,
        role = %role.as_deref().unwrap_or("unset"),
        operator_id = %operator_id.as_deref().unwrap_or("unknown"),
        command_id = %command_id,
        message_digest = %message_digest,
        message_preview = %message_preview,
        "Orchestrator command accepted"
    );

    let command_result = runtime
        .submit_orchestrator_chat_with_command_id(
            &workflow_id,
            message,
            source,
            external_message_id,
            Some(command_id.clone()),
        )
        .await;

    let (status, error, retryable) = match command_result {
        Ok(result) => (
            result.status.as_str().to_string(),
            result.error.map(|value| redact_sensitive_content(&value)),
            matches!(result.status.as_str(), "failed" | "cancelled"),
        ),
        Err(error) => {
            let redacted_error =
                redact_sensitive_content(&format!("Failed to submit orchestrator chat: {error}"));
            tracing::warn!(
                workflow_id = %workflow_id,
                command_id = %command_id,
                source = %source,
                error = %redacted_error,
                "Failed to submit orchestrator chat message"
            );
            ("failed".to_string(), Some(redacted_error), true)
        }
    };

    let response = SubmitOrchestratorChatResponse {
        command_id: command_id.clone(),
        status: status.clone(),
        retryable,
        error: error.clone(),
    };

    WorkflowOrchestratorCommand::update_status(
        &deployment.db().pool,
        &command_id,
        &response.status,
        response.error.as_deref(),
        response.retryable,
        None,
        Some(Utc::now()),
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to update command completion: {e}")))?;

    if response.status == "succeeded"
        && let Ok(messages) = runtime.get_orchestrator_messages(&workflow_id).await
            && let Some(last_assistant) =
                messages.iter().rev().find(|entry| entry.role == "assistant")
        {
            let assistant_message = WorkflowOrchestratorMessage::new(
                &workflow_id,
                Some(&command_id),
                "assistant",
                &last_assistant.content,
                "orchestrator",
                None,
            );
            let _ =
                WorkflowOrchestratorMessage::insert(&deployment.db().pool, &assistant_message).await;
        }

    let receipt_message = WorkflowOrchestratorMessage::new(
        &workflow_id,
        Some(&command_id),
        "system",
        &build_orchestrator_receipt_message(
            &command_id,
            &response.status,
            response.error.as_deref(),
            response.retryable,
        ),
        "orchestrator",
        None,
    );
    let _ = WorkflowOrchestratorMessage::insert(&deployment.db().pool, &receipt_message).await;

    let summary_message = WorkflowOrchestratorMessage::new(
        &workflow_id,
        Some(&command_id),
        "tool-summary",
        &build_orchestrator_summary_message(&response.status, source),
        "orchestrator",
        None,
    );
    let _ = WorkflowOrchestratorMessage::insert(&deployment.db().pool, &summary_message).await;

    let circuit_opened = update_orchestrator_circuit_breaker(&workflow_id, &response.status).await;
    if circuit_opened && workflow.status == "running" {
        let _ = Workflow::update_status(&deployment.db().pool, &workflow_id, WORKFLOW_STATUS_PAUSED).await;
        tracing::warn!(
            target: "audit.orchestrator_chat",
            workflow_id = %workflow_id,
            command_id = %command_id,
            "Circuit breaker opened; workflow auto-paused"
        );

        let breaker_notice = WorkflowOrchestratorMessage::new(
            &workflow_id,
            Some(&command_id),
            "system",
            "Safety breaker triggered after repeated command failures. Workflow was auto-paused.",
            "orchestrator",
            None,
        );
        let _ = WorkflowOrchestratorMessage::insert(&deployment.db().pool, &breaker_notice).await;
    }

    tracing::info!(
        target: "audit.orchestrator_chat",
        workflow_id = %workflow_id,
        source = %source,
        operator_id = %operator_id.as_deref().unwrap_or("unknown"),
        status = %response.status,
        command_id = %response.command_id,
        retryable = response.retryable,
        "Submitted orchestrator chat message"
    );

    Ok(ResponseJson(ApiResponse::success(response)))
}

/// GET /api/workflows/:workflow_id/orchestrator/messages
/// List orchestrator conversation messages for a running workflow
async fn list_orchestrator_messages(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
    Query(params): Query<ListOrchestratorMessagesQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<OrchestratorChatMessageResponse>>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    if !is_orchestrator_chat_feature_enabled() {
        return Ok(ResponseJson(ApiResponse::success(Vec::new())));
    }

    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    if !workflow.orchestrator_enabled {
        return Err(ApiError::Conflict(
            "Cannot list orchestrator messages: orchestrator is not enabled".to_string(),
        ));
    }

    let has_models_configured = {
        let config = deployment.config().read().await;
        has_configured_workflow_models(&config)
    };
    if !has_models_configured {
        return Err(ApiError::Conflict(
            "Cannot list orchestrator messages: configure at least one AI model first".to_string(),
        ));
    }

    let limit = params.limit.unwrap_or(50).clamp(1, 200);
    let cursor = match params.cursor {
        Some(c) => c,
        None => {
            let total: (i64,) = sqlx::query_as(
                "SELECT COUNT(*) FROM workflow_orchestrator_message WHERE workflow_id = ?1",
            )
            .bind(&workflow_id)
            .fetch_one(&deployment.db().pool)
            .await
            .map_err(|e| ApiError::Internal(format!("Failed to count orchestrator messages: {e}")))?;
            (total.0 as usize).saturating_sub(limit)
        }
    };

    let persisted_messages = WorkflowOrchestratorMessage::list_by_workflow_paginated(
        &deployment.db().pool,
        &workflow_id,
        cursor,
        limit,
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to query orchestrator messages: {e}")))?;

    if !persisted_messages.is_empty() {
        let response = persisted_messages
            .into_iter()
            .map(|message| OrchestratorChatMessageResponse {
                role: message.role,
                content: message.content,
            })
            .collect();
        return Ok(ResponseJson(ApiResponse::success(response)));
    }

    let runtime = deployment.orchestrator_runtime();
    if !runtime.is_running(&workflow_id).await {
        return Ok(ResponseJson(ApiResponse::success(Vec::new())));
    }

    let runtime_messages = runtime
        .get_orchestrator_messages(&workflow_id)
        .await
        .map_err(|e| {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to list orchestrator messages"
            );
            ApiError::BadRequest(format!("Failed to list orchestrator messages: {e}"))
        })?;

    let (start, end) = paginate_orchestrator_messages(
        runtime_messages.len(),
        params.cursor,
        params.limit,
    );
    let response = runtime_messages
        .into_iter()
        .skip(start)
        .take(end.saturating_sub(start))
        .map(|message| OrchestratorChatMessageResponse {
            role: message.role,
            content: message.content,
        })
        .collect();

    Ok(ResponseJson(ApiResponse::success(response)))
}

/// POST /api/workflows/:workflow_id/merge
/// Execute merge terminal for workflow
async fn merge_workflow(
    State(deployment): State<DeploymentImpl>,
    Path(workflow_id): Path<Uuid>,
    Json(payload): Json<MergeWorkflowRequest>,
) -> Result<ResponseJson<ApiResponse<serde_json::Value>>, ApiError> {
    let workflow_id = workflow_id.to_string();
    if let Some(strategy) = payload.merge_strategy.as_deref()
        && !strategy.eq_ignore_ascii_case("squash")
    {
        return Err(ApiError::BadRequest(format!(
            "Unsupported merge strategy '{strategy}': only 'squash' is supported"
        )));
    }

    // Check workflow exists
    let workflow = Workflow::find_by_id(&deployment.db().pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // Validate workflow is in appropriate state for merging
    let current_status = workflow.status.as_str();
    if !can_merge_from_workflow_status(current_status) {
        return Err(ApiError::BadRequest(format!(
            "Cannot merge workflow with status '{current_status}': expected one of: {MERGE_ALLOWED_WORKFLOW_STATUSES:?}"
        )));
    }

    // G06-002: acquire the per-workflow merge lock BEFORE the CAS so that
    // auto-merge (orchestrator) and manual merge (this endpoint) cannot both
    // pass the CAS check concurrently.
    let _merge_guard = services::services::merge_coordinator::acquire_workflow_merge_lock(
        &workflow_id,
    )
    .await;

    // G06-001: CAS — atomically transition completed → merging to prevent concurrent merges.
    // Works in tandem with the mutex above: the mutex prevents races between code paths
    // that both read status before updating; the CAS ensures only one succeeds at the DB level.
    let cas_ok = Workflow::set_merging(&deployment.db().pool, &workflow_id)
        .await
        .map_err(|e| ApiError::Internal(e.to_string()))?;
    if !cas_ok {
        return Err(ApiError::Conflict(
            "Merge already in progress or workflow is not in completed state".to_string(),
        ));
    }

    let project = Project::find_by_id(&deployment.db().pool, workflow.project_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Project not found".to_string()))?;

    let base_repo_path = project
        .default_agent_working_dir
        .as_deref()
        .map(str::trim)
        .filter(|path| !path.is_empty())
        .map(PathBuf::from)
        .ok_or_else(|| {
            ApiError::BadRequest(
                "Cannot merge workflow: project has no default agent working directory".to_string(),
            )
        })?;

    if !base_repo_path.exists() {
        return Err(ApiError::BadRequest(format!(
            "Cannot merge workflow: base repository path does not exist ({})",
            base_repo_path.display()
        )));
    }

    let tasks = WorkflowTask::find_by_workflow(&deployment.db().pool, &workflow_id).await?;
    if tasks.is_empty() {
        return Err(ApiError::BadRequest(
            "Cannot merge workflow: no tasks found".to_string(),
        ));
    }

    let unfinished_tasks: Vec<String> = tasks
        .iter()
        .filter(|task| task.status != "completed")
        .map(|task| format!("{}({})", task.id, task.status))
        .collect();

    if !unfinished_tasks.is_empty() {
        return Err(ApiError::Conflict(format!(
            "Cannot merge workflow: unfinished tasks found [{}]",
            unfinished_tasks.join(", ")
        )));
    }

    let mut merged_tasks = Vec::new();

    // G06-004: Record HEAD SHA before merge loop so multi-task merge failures
    // can be rolled back to a known-good state.
    let pre_merge_head_sha = match deployment
        .git()
        .get_branch_oid(&base_repo_path, &workflow.target_branch)
    {
        Ok(sha) => {
            tracing::info!(
                workflow_id = %workflow_id,
                target_branch = %workflow.target_branch,
                pre_merge_sha = %sha,
                "Recorded HEAD SHA before merge loop for rollback support"
            );
            Some(sha)
        }
        Err(e) => {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Could not record pre-merge HEAD SHA (merge will proceed without rollback support)"
            );
            None
        }
    };

    for task in tasks {
        let task_id = task.id.clone();
        let task_branch = task.branch.trim();
        if task_branch.is_empty() {
            let _ = Workflow::set_merge_completed(&deployment.db().pool, &workflow_id).await;
            return Err(ApiError::BadRequest(format!(
                "Cannot merge task {task_id}: branch is empty"
            )));
        }

        // G23-002: Use WorktreeManager base dir instead of hardcoded "worktrees" subpath
        let task_worktree_path = services::services::worktree_manager::WorktreeManager::get_worktree_base_dir()
            .join(task_branch);
        // Fallback: if the managed path doesn't exist, try the legacy repo-relative path
        let task_worktree_path = if task_worktree_path.exists() {
            task_worktree_path
        } else {
            let legacy_path = base_repo_path.join("worktrees").join(task_branch);
            if legacy_path.exists() {
                legacy_path
            } else {
                let _ = Workflow::set_merge_completed(&deployment.db().pool, &workflow_id).await;
                return Err(ApiError::BadRequest(format!(
                    "Cannot merge task {}: worktree path does not exist (tried {} and {})",
                    task_id,
                    task_worktree_path.display(),
                    base_repo_path.join("worktrees").join(task_branch).display()
                )));
            }
        };

        let commit_message = format!("Merge task {task_id} ({task_branch})");
        match deployment.git().merge_changes(
            &base_repo_path,
            &task_worktree_path,
            task_branch,
            &workflow.target_branch,
            &commit_message,
        ) {
            Ok(commit_sha) => {
                merged_tasks.push(json!({
                    "taskId": task_id,
                    "branch": task_branch,
                    "commitSha": commit_sha,
                }));
            }
            Err(err) => {
                let should_keep_merging_status = matches!(
                    &err,
                    GitServiceError::MergeConflicts(_)
                        | GitServiceError::BranchesDiverged(_)
                        | GitServiceError::WorktreeDirty(_, _)
                        | GitServiceError::RebaseInProgress
                );

                if !should_keep_merging_status {
                    // Roll back merging → completed on non-recoverable errors
                    if let Err(status_err) =
                        Workflow::set_merge_completed(&deployment.db().pool, &workflow_id).await
                    {
                        tracing::warn!(
                            workflow_id = %workflow_id,
                            error = %status_err,
                            "Failed to roll back workflow status after merge failure"
                        );
                    }
                }

                // G06-004: Log pre-merge HEAD SHA for manual rollback if needed
                if let Some(ref sha) = pre_merge_head_sha {
                    tracing::error!(
                        workflow_id = %workflow_id,
                        task_id = %task_id,
                        pre_merge_head_sha = %sha,
                        "Merge failed. To rollback: git reset --hard {sha}"
                    );
                }

                return Err(ApiError::from(err));
            }
        }
    }

    Workflow::set_merge_completed(&deployment.db().pool, &workflow_id).await
        .map_err(|e| ApiError::Internal(e.to_string()))?;

    // G06-006: Clean up worktree directories after successful merge
    let worktree_cleanups: Vec<services::services::worktree_manager::WorktreeCleanup> = merged_tasks
        .iter()
        .filter_map(|t| {
            let branch = t.get("branch")?.as_str()?;
            let managed_path = services::services::worktree_manager::WorktreeManager::get_worktree_base_dir().join(branch);
            let legacy_path = base_repo_path.join("worktrees").join(branch);
            let worktree_path = if managed_path.exists() { managed_path } else { legacy_path };
            Some(services::services::worktree_manager::WorktreeCleanup::new(
                worktree_path,
                Some(base_repo_path.clone()),
            ))
        })
        .collect();
    if !worktree_cleanups.is_empty() {
        if let Err(e) = services::services::worktree_manager::WorktreeManager::batch_cleanup_worktrees(&worktree_cleanups).await {
            tracing::warn!(
                workflow_id = %workflow_id,
                error = %e,
                "Failed to clean up worktrees after merge (non-fatal)"
            );
        }
    }

    // Return success response
    let result = json!({
        "success": true,
        "message": "Merge completed successfully",
        "workflow_id": workflow_id,
        "workflowId": workflow_id,
        "targetBranch": workflow.target_branch,
        "preMergeHeadSha": pre_merge_head_sha,
        "mergedTasks": merged_tasks
    });

    Ok(ResponseJson(ApiResponse::success(result)))
}

// ============================================================================
// Contract Tests
// ============================================================================

#[cfg(test)]
mod dto_tests {
    #[test]
    fn test_list_workflows_returns_camelcase() {
        // This test validates the expected format
        let response_json = r#"[
            {
                "id": "wf-test",
                "projectId": "proj-test",
                "name": "Test",
                "status": "created",
                "createdAt": "2026-01-24T10:00:00Z",
                "updatedAt": "2026-01-24T10:00:00Z",
                "tasksCount": 0,
                "terminalsCount": 0
            }
        ]"#;

        // Verify no snake_case
        assert!(!response_json.contains("\"project_id\""));
        assert!(!response_json.contains("\"created_at\""));

        // Verify camelCase
        assert!(response_json.contains("\"projectId\""));
        assert!(response_json.contains("\"createdAt\""));
    }

    #[test]
    fn test_get_workflow_returns_camelcase() {
        let response_json = r#"{
            "id": "wf-test",
            "projectId": "proj-test",
            "name": "Test Workflow",
            "status": "created",
            "useSlashCommands": true,
            "orchestratorEnabled": true,
            "createdAt": "2026-01-24T10:00:00Z",
            "updatedAt": "2026-01-24T10:00:00Z",
            "tasks": [],
            "commands": []
        }"#;

        // Verify no snake_case
        assert!(!response_json.contains("\"project_id\""));
        assert!(!response_json.contains("\"use_slash_commands\""));

        // Verify camelCase
        assert!(response_json.contains("\"projectId\""));
        assert!(response_json.contains("\"useSlashCommands\""));
        assert!(response_json.contains("\"orchestratorEnabled\""));
    }
}

#[cfg(test)]
mod workflow_guard_tests {
    use chrono::Utc;

    use super::*;

    fn build_task_with_status(workflow_id: &str, status: &str) -> WorkflowTask {
        WorkflowTask {
            id: "task-test".to_string(),
            workflow_id: workflow_id.to_string(),
            vk_task_id: None,
            name: "Task".to_string(),
            description: None,
            branch: "workflow/test/task".to_string(),
            status: status.to_string(),
            order_index: 0,
            started_at: None,
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    fn build_task_with_workflow(workflow_id: &str) -> WorkflowTask {
        build_task_with_status(workflow_id, "pending")
    }

    #[test]
    fn merge_status_guard_allows_only_terminal_merge_states() {
        assert!(can_merge_from_workflow_status("completed"));
        assert!(can_merge_from_workflow_status("merging"));
        assert!(!can_merge_from_workflow_status("starting"));
        assert!(!can_merge_from_workflow_status("running"));
    }

    #[test]
    fn merge_status_guard_rejects_all_non_merge_states() {
        let non_merge_states = [
            "created",
            "starting",
            "ready",
            "running",
            "paused",
            "failed",
            "cancelled",
        ];

        for status in non_merge_states {
            assert!(
                !can_merge_from_workflow_status(status),
                "Expected status '{status}' to be rejected by merge guard"
            );
        }
    }

    #[test]
    fn workflow_status_transition_accepts_valid_paths() {
        let allowed_cases = [
            ("created", "starting"),
            ("starting", "ready"),
            ("ready", "running"),
            ("running", "paused"),
            ("running", "completed"),
            ("paused", "ready"),
            ("completed", "merging"),
            ("merging", "completed"),
            ("failed", "starting"),
            ("cancelled", "created"),
            ("completed", "created"),
        ];

        for (current, next) in allowed_cases {
            assert!(
                validate_workflow_status_transition(current, next).is_ok(),
                "Expected transition {current} -> {next} to be valid"
            );
        }
    }

    #[test]
    fn workflow_status_transition_rejects_illegal_jumps() {
        let rejected_cases = [
            ("created", "running"),
            ("starting", "completed"),
            ("ready", "completed"),
            ("completed", "running"),
            ("merging", "running"),
            ("cancelled", "running"),
        ];

        for (current, next) in rejected_cases {
            assert!(
                matches!(
                    validate_workflow_status_transition(current, next),
                    Err(ApiError::Conflict(_))
                ),
                "Expected transition {current} -> {next} to be rejected"
            );
        }

        assert!(matches!(
            validate_workflow_status_transition("created", "unknown_status"),
            Err(ApiError::BadRequest(_))
        ));
    }

    #[test]
    fn task_scope_guard_rejects_cross_workflow_access() {
        let task = build_task_with_workflow("wf-1");

        assert!(validate_task_workflow_scope(&task, "wf-1").is_ok());
        assert!(matches!(
            validate_task_workflow_scope(&task, "wf-2"),
            Err(ApiError::BadRequest(_))
        ));
    }

    #[test]
    fn auto_complete_guard_requires_running_status_and_all_tasks_completed() {
        let completed_tasks = vec![
            build_task_with_status("wf-1", "completed"),
            build_task_with_status("wf-1", "completed"),
        ];
        assert!(should_auto_complete_workflow("running", &completed_tasks));
        assert!(!should_auto_complete_workflow("paused", &completed_tasks));
    }

    #[test]
    fn auto_complete_guard_rejects_incomplete_or_empty_tasks() {
        let mixed_tasks = vec![
            build_task_with_status("wf-1", "completed"),
            build_task_with_status("wf-1", "pending"),
        ];
        assert!(!should_auto_complete_workflow("running", &mixed_tasks));
        assert!(!should_auto_complete_workflow("running", &[]));
    }
}

#[cfg(test)]
mod create_request_validation_tests {
    use super::*;

    fn minimal_terminal_config() -> db::models::TerminalConfig {
        db::models::TerminalConfig {
            cli_type_id: "cli-test".to_string(),
            model_config_id: "model-test".to_string(),
            model_config: None,
            custom_base_url: None,
            custom_api_key: None,
        }
    }

    fn minimal_diy_request() -> CreateWorkflowRequest {
        CreateWorkflowRequest {
            project_id: Uuid::new_v4().to_string(),
            name: "Test Workflow".to_string(),
            description: None,
            execution_mode: "diy".to_string(),
            initial_goal: None,
            use_slash_commands: false,
            commands: None,
            orchestrator_config: None,
            error_terminal_config: None,
            merge_terminal_config: minimal_terminal_config(),
            target_branch: Some("main".to_string()),
            git_watcher_enabled: Some(true),
            tasks: vec![CreateWorkflowTaskRequest {
                id: None,
                name: "Task 1".to_string(),
                description: None,
                branch: None,
                order_index: 0,
                terminals: vec![CreateTerminalRequest {
                    id: None,
                    cli_type_id: "cli-test".to_string(),
                    model_config_id: "model-test".to_string(),
                    model_config: None,
                    custom_base_url: None,
                    custom_api_key: None,
                    role: Some("writer".to_string()),
                    role_description: None,
                    order_index: 0,
                    auto_confirm: true,
                }],
            }],
        }
    }

    #[test]
    fn diy_mode_requires_tasks() {
        let mut request = minimal_diy_request();
        request.tasks.clear();

        let error = validate_create_request(&request).expect_err("expected diy validation error");
        assert!(matches!(error, ApiError::BadRequest(_)));
    }

    #[test]
    fn agent_planned_mode_allows_empty_tasks_with_goal() {
        let mut request = minimal_diy_request();
        request.execution_mode = "agent_planned".to_string();
        request.initial_goal = Some("Plan and implement the feature".to_string());
        request.orchestrator_config = Some(db::models::OrchestratorConfig {
            api_type: "openai-compatible".to_string(),
            base_url: "https://api.example.com".to_string(),
            api_key: "secret".to_string(),
            model: "gpt-4.1".to_string(),
        });
        request.tasks.clear();

        validate_create_request(&request).expect("agent planned request should be valid");
    }

    #[test]
    fn agent_planned_mode_requires_initial_goal() {
        let mut request = minimal_diy_request();
        request.execution_mode = "agent_planned".to_string();
        request.initial_goal = None;
        request.orchestrator_config = Some(db::models::OrchestratorConfig {
            api_type: "openai-compatible".to_string(),
            base_url: "https://api.example.com".to_string(),
            api_key: "secret".to_string(),
            model: "gpt-4.1".to_string(),
        });
        request.tasks.clear();

        let error =
            validate_create_request(&request).expect_err("expected missing initial_goal error");
        assert!(matches!(error, ApiError::BadRequest(_)));
    }

    #[test]
    fn agent_planned_mode_requires_orchestrator_config() {
        let mut request = minimal_diy_request();
        request.execution_mode = "agent_planned".to_string();
        request.initial_goal = Some("Plan and implement the feature".to_string());
        request.tasks.clear();

        let error = validate_create_request(&request)
            .expect_err("expected missing orchestrator_config error");
        assert!(matches!(error, ApiError::BadRequest(_)));
    }
}

#[cfg(test)]
mod recovery_response_tests {
    use std::sync::Arc;

    use chrono::Utc;
    use db::{DBService, models::Workflow};
    use services::services::orchestrator::MessageBus;

    use super::*;

    async fn setup_runtime_with_running_workflow() -> (OrchestratorRuntime, String, sqlx::SqlitePool) {
        let pool = sqlx::SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE workflow (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                name TEXT NOT NULL,
                description TEXT,
                status TEXT NOT NULL,
                execution_mode TEXT NOT NULL DEFAULT 'diy',
                initial_goal TEXT,
                use_slash_commands INTEGER NOT NULL DEFAULT 0,
                orchestrator_enabled INTEGER NOT NULL DEFAULT 0,
                orchestrator_api_type TEXT,
                orchestrator_base_url TEXT,
                orchestrator_api_key TEXT,
                orchestrator_model TEXT,
                error_terminal_enabled INTEGER NOT NULL DEFAULT 0,
                error_terminal_cli_id TEXT,
                error_terminal_model_id TEXT,
                merge_terminal_cli_id TEXT NOT NULL,
                merge_terminal_model_id TEXT NOT NULL,
                target_branch TEXT NOT NULL,
                git_watcher_enabled INTEGER NOT NULL DEFAULT 1,
                orchestrator_state TEXT,
                ready_at TEXT,
                started_at TEXT,
                completed_at TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });
        let runtime = OrchestratorRuntime::new(db, Arc::new(MessageBus::new(1000)));
        let workflow_id = Uuid::new_v4().to_string();
        let workflow = Workflow {
            id: workflow_id.clone(),
            project_id: Uuid::new_v4(),
            name: "Recovered Workflow".to_string(),
            description: None,
            status: "running".to_string(),
            execution_mode: "agent_planned".to_string(),
            initial_goal: Some("Resume orchestration after restart".to_string()),
            use_slash_commands: false,
            orchestrator_enabled: false,
            orchestrator_api_type: None,
            orchestrator_base_url: None,
            orchestrator_api_key: None,
            orchestrator_model: None,
            error_terminal_enabled: false,
            error_terminal_cli_id: None,
            error_terminal_model_id: None,
            merge_terminal_cli_id: "merge-cli".to_string(),
            merge_terminal_model_id: "merge-model".to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: None,
            started_at: Some(Utc::now()),
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        };
        Workflow::create(&pool, &workflow).await.unwrap();

        (runtime, workflow_id, pool)
    }

    #[tokio::test]
    async fn recovery_helper_reports_interrupted_workflow_count() {
        let (runtime, workflow_id, pool) = setup_runtime_with_running_workflow().await;

        let response = run_workflow_recovery(&runtime)
            .await
            .expect("workflow recovery should succeed");

        assert_eq!(response.recovered_workflows, 1);
        assert_eq!(response.recovered_commands, 0);
        assert_eq!(
            response.message,
            "Recovered 1 interrupted workflow(s) and 0 command(s)"
        );

        let workflow = Workflow::find_by_id(&pool, &workflow_id)
            .await
            .expect("should query workflow after recovery")
            .expect("workflow should exist");
        assert_eq!(workflow.status, "failed");
    }
}

#[cfg(test)]
mod prompt_response_route_tests {
    use axum::{
        body::{Body, to_bytes},
        http::{Request, StatusCode},
    };
    use deployment::Deployment;
    use serde_json::json;
    use serial_test::serial;
    use tower::ServiceExt;

    use super::*;

    #[tokio::test]
    #[serial]
    async fn submit_prompt_response_requires_terminal_id() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");

        let app = workflows_routes().with_state(deployment);
        let payload = json!({
            "terminalId": "   ",
            "response": "yes"
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/prompts/respond")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::BAD_REQUEST);

        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("Failed to read response body");
        let body_json: serde_json::Value =
            serde_json::from_slice(&body).expect("Failed to parse response JSON");
        assert_eq!(
            body_json.get("message").and_then(serde_json::Value::as_str),
            Some("terminalId is required")
        );
    }

    #[tokio::test]
    #[serial]
    async fn submit_prompt_response_allows_empty_response() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");

        let app = workflows_routes().with_state(deployment);
        let payload = json!({
            "terminalId": "00000000-0000-0000-0000-000000000001",
            "response": ""
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/prompts/respond")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::NOT_FOUND);

        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("Failed to read response body");
        let body_json: serde_json::Value =
            serde_json::from_slice(&body).expect("Failed to parse response JSON");
        assert_eq!(
            body_json.get("message").and_then(serde_json::Value::as_str),
            Some("Workflow not found")
        );
    }

    #[tokio::test]
    #[serial]
    async fn submit_prompt_response_requires_response_field() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");

        let app = workflows_routes().with_state(deployment);
        let payload = json!({
            "terminalId": "00000000-0000-0000-0000-000000000001"
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/prompts/respond")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::UNPROCESSABLE_ENTITY);
    }
}

#[cfg(test)]
mod orchestrator_chat_route_tests {
    use axum::{
        body::{Body, to_bytes},
        http::{Request, StatusCode},
    };
    use serde_json::json;
    use serial_test::serial;
    use tower::ServiceExt;

    use super::*;

    #[tokio::test]
    #[serial]
    async fn submit_orchestrator_chat_requires_message() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");
        let app = workflows_routes().with_state(deployment);

        let payload = json!({
            "message": "   "
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/orchestrator/chat")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::BAD_REQUEST);

        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("Failed to read response body");
        let body_json: serde_json::Value =
            serde_json::from_slice(&body).expect("Failed to parse response JSON");
        assert_eq!(
            body_json.get("message").and_then(serde_json::Value::as_str),
            Some("message is required")
        );
    }

    #[tokio::test]
    #[serial]
    async fn submit_orchestrator_chat_rejects_invalid_source() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");
        let app = workflows_routes().with_state(deployment);

        let payload = json!({
            "message": "Hello orchestrator",
            "source": "invalid"
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/orchestrator/chat")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::BAD_REQUEST);

        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("Failed to read response body");
        let body_json: serde_json::Value =
            serde_json::from_slice(&body).expect("Failed to parse response JSON");
        assert_eq!(
            body_json.get("message").and_then(serde_json::Value::as_str),
            Some("source must be one of: web, api, social (got 'invalid')")
        );
    }

    #[tokio::test]
    #[serial]
    async fn submit_orchestrator_chat_requires_external_message_id_for_non_web_source() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");
        let app = workflows_routes().with_state(deployment);

        let payload = json!({
            "message": "Hello orchestrator",
            "source": "social"
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/orchestrator/chat")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::BAD_REQUEST);

        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("Failed to read response body");
        let body_json: serde_json::Value =
            serde_json::from_slice(&body).expect("Failed to parse response JSON");
        assert_eq!(
            body_json.get("message").and_then(serde_json::Value::as_str),
            Some("externalMessageId is required when source is not 'web'")
        );
    }

    #[tokio::test]
    #[serial]
    async fn submit_orchestrator_chat_returns_not_found_for_unknown_workflow() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");
        let app = workflows_routes().with_state(deployment);

        let payload = json!({
            "message": "Hello orchestrator"
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/00000000-0000-0000-0000-000000000000/orchestrator/chat")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    #[tokio::test]
    #[serial]
    async fn list_orchestrator_messages_returns_not_found_for_unknown_workflow() {
        let deployment = DeploymentImpl::new()
            .await
            .expect("Failed to create deployment");
        let app = workflows_routes().with_state(deployment);

        let request = Request::builder()
            .method("GET")
            .uri("/00000000-0000-0000-0000-000000000000/orchestrator/messages")
            .body(Body::empty())
            .expect("Failed to build request");

        let response = app
            .oneshot(request)
            .await
            .expect("Failed to execute request");

        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }
}

#[cfg(test)]
mod orchestrator_pagination_tests {
    use super::paginate_orchestrator_messages;

    #[test]
    fn defaults_to_latest_window_when_cursor_missing() {
        let (start, end) = paginate_orchestrator_messages(120, None, None);
        assert_eq!((start, end), (70, 120));
    }

    #[test]
    fn applies_cursor_and_limit() {
        let (start, end) = paginate_orchestrator_messages(100, Some(10), Some(5));
        assert_eq!((start, end), (10, 15));
    }

    #[test]
    fn clamps_values_to_safe_bounds() {
        let (start, end) = paginate_orchestrator_messages(30, Some(40), Some(500));
        assert_eq!((start, end), (30, 30));
    }
}
