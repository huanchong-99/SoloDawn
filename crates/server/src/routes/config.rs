use std::{
    collections::HashMap,
    path::{Path as StdPath, PathBuf},
    process::Stdio,
    time::Duration,
};

use axum::{
    Json, Router,
    body::Body,
    extract::{Path, Query, State},
    http::{self, StatusCode},
    response::{Json as ResponseJson, Response},
    routing::{get, post, put},
};
use deployment::{Deployment, DeploymentError};
use executors::{
    executors::{
        AvailabilityInfo, BaseAgentCapability, BaseCodingAgent, StandardCodingAgentExecutor,
    },
    mcp_config::{McpConfig, read_agent_config, write_agent_config},
    profile::{ExecutorConfigs, ExecutorProfileId},
};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use services::services::config::{
    Config, ConfigError, SoundFile,
    editor::{EditorConfig, EditorType},
    save_config_to_file,
};
use tokio::{fs, process::Command};
use ts_rs::TS;
use utils::{api::oauth::LoginStatus, assets::config_path, response::ApiResponse};

use crate::{DeploymentImpl, error::ApiError};

pub fn router() -> Router<DeploymentImpl> {
    Router::new()
        .route("/info", get(get_user_system_info))
        .route("/config", put(update_config))
        .route("/sounds/{sound}", get(get_sound))
        .route("/mcp-config", get(get_mcp_servers).post(update_mcp_servers))
        .route("/profiles", get(get_profiles).put(update_profiles))
        .route(
            "/editors/check-availability",
            get(check_editor_availability),
        )
        .route("/agents/check-availability", get(check_agent_availability))
        .route("/agents/install-ai-clis", post(install_ai_clis))
        .route("/system/prerequisites", get(get_system_prerequisites))
}

const REMOTE_FEATURES_ENABLED: bool = false;

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct Environment {
    pub os_type: String,
    pub os_version: String,
    pub os_architecture: String,
    pub bitness: String,
    pub is_containerized: bool,
    pub workspace_root_hint: Option<String>,
}

impl Default for Environment {
    fn default() -> Self {
        Self::new()
    }
}

impl Environment {
    pub fn new() -> Self {
        let info = os_info::get();
        let workspace_root_hint = utils::env_compat::var_opt_with_compat("SOLODAWN_WORKSPACE_ROOT", "GITCORTEX_WORKSPACE_ROOT")
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty());

        Environment {
            os_type: info.os_type().to_string(),
            os_version: info.version().to_string(),
            os_architecture: info.architecture().unwrap_or("unknown").to_string(),
            bitness: info.bitness().to_string(),
            is_containerized: StdPath::new("/.dockerenv").exists(),
            workspace_root_hint,
        }
    }
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct UserSystemInfo {
    pub config: Config,
    pub analytics_user_id: String,
    pub login_status: LoginStatus,
    #[serde(flatten)]
    pub profiles: ExecutorConfigs,
    pub environment: Environment,
    /// Capabilities supported per executor (e.g., { "CLAUDE_CODE": [`SESSION_FORK`] })
    pub capabilities: HashMap<String, Vec<BaseAgentCapability>>,
    pub remote_features_enabled: bool,
}

// TODO: update frontend, BE schema has changed, this replaces GET /config and /config/constants
#[axum::debug_handler]
async fn get_user_system_info(
    State(deployment): State<DeploymentImpl>,
) -> ResponseJson<ApiResponse<UserSystemInfo>> {
    let config = deployment.config().read().await;
    let login_status = deployment.get_login_status().await;

    let user_system_info = UserSystemInfo {
        config: config.clone(),
        analytics_user_id: deployment.user_id().to_string(),
        login_status,
        profiles: ExecutorConfigs::get_cached(),
        environment: Environment::new(),
        capabilities: {
            let mut caps: HashMap<String, Vec<BaseAgentCapability>> = HashMap::new();
            let profs = ExecutorConfigs::get_cached();
            for key in profs.executors.keys() {
                if let Some(agent) = profs.get_coding_agent(&ExecutorProfileId::new(*key)) {
                    caps.insert(key.to_string(), agent.capabilities());
                }
            }
            caps
        },
        remote_features_enabled: REMOTE_FEATURES_ENABLED,
    };

    ResponseJson(ApiResponse::success(user_system_info))
}

async fn update_config(
    State(deployment): State<DeploymentImpl>,
    Json(new_config): Json<Config>,
) -> Result<ResponseJson<ApiResponse<Config>>, ApiError> {
    let config_path = match config_path() {
        Ok(path) => path,
        Err(e) => {
            return Err(ApiError::Internal(format!(
                "Failed to resolve config path: {e}"
            )));
        }
    };

    // Validate git branch prefix
    if !utils::git::is_valid_branch_prefix(&new_config.git_branch_prefix) {
        return Err(ApiError::BadRequest(
            "Invalid git branch prefix. Must be a valid git branch name component without slashes."
                .to_string(),
        ));
    }

    // Get old config state before updating
    let old_config = deployment.config().read().await.clone();

    match save_config_to_file(&new_config, &config_path) {
        Ok(()) => {
            let mut config = deployment.config().write().await;
            *config = new_config.clone();
            drop(config);

            // Track config events when fields transition from false → true and run side effects
            handle_config_events(&deployment, &old_config, &new_config).await;

            Ok(ResponseJson(ApiResponse::success(new_config)))
        }
        Err(e) => Err(ApiError::Config(e)),
    }
}

/// Track config events when fields transition from false → true
async fn track_config_events(deployment: &DeploymentImpl, old: &Config, new: &Config) {
    let events = [
        (
            !old.disclaimer_acknowledged && new.disclaimer_acknowledged,
            "onboarding_disclaimer_accepted",
            serde_json::json!({}),
        ),
        (
            !old.onboarding_acknowledged && new.onboarding_acknowledged,
            "onboarding_completed",
            serde_json::json!({
                "profile": new.executor_profile,
                "editor": new.editor
            }),
        ),
        (
            !old.analytics_enabled && new.analytics_enabled,
            "analytics_session_start",
            serde_json::json!({}),
        ),
    ];

    for (should_track, event_name, properties) in events {
        if should_track {
            deployment
                .track_if_analytics_allowed(event_name, properties)
                .await;
        }
    }
}

async fn handle_config_events(deployment: &DeploymentImpl, old: &Config, new: &Config) {
    track_config_events(deployment, old, new).await;

    if !old.disclaimer_acknowledged && new.disclaimer_acknowledged {
        // Spawn auto project setup as background task to avoid blocking config response
        let deployment_clone = deployment.clone();
        tokio::spawn(async move {
            deployment_clone.trigger_auto_project_setup().await;
        });
    }

    // Sync workflow_model_library → model_config DB table so the
    // Orchestrator Agent can see user-configured models.
    sync_model_library_to_db(deployment, &new.workflow_model_library).await;
}

/// Sync `workflow_model_library` entries from config.json into the
/// `model_config` DB table with encrypted credentials.
async fn sync_model_library_to_db(
    deployment: &DeploymentImpl,
    items: &[services::services::config::WorkflowModelLibraryItem],
) {
    let pool = &deployment.db().pool;
    for item in items {
        let cli_type_id = item.cli_type_id.as_deref().unwrap_or("cli-codex");
        // Upsert the model record
        if let Err(e) = db::models::ModelConfig::create_custom(
            pool,
            &item.id,
            cli_type_id,
            &item.display_name,
            &item.model_id,
        )
        .await
        {
            tracing::warn!(model_id = %item.id, error = %e, "Failed to sync model config to DB");
            continue;
        }

        // Store encrypted credentials
        if !item.api_key.is_empty() {
            let mut tmp = db::models::ModelConfig {
                id: String::new(),
                cli_type_id: String::new(),
                name: String::new(),
                display_name: String::new(),
                api_model_id: None,
                is_default: false,
                is_official: false,
                created_at: chrono::Utc::now(),
                updated_at: chrono::Utc::now(),
                encrypted_api_key: None,
                base_url: None,
                api_type: None,
                has_api_key: false,
            };
            if let Ok(()) = tmp.set_api_key(&item.api_key) {
                if let Some(ref encrypted) = tmp.encrypted_api_key {
                    let _ = db::models::ModelConfig::update_credentials(
                        pool,
                        &item.id,
                        encrypted,
                        Some(&item.base_url),
                        &item.api_type,
                    )
                    .await;
                }
            }
        }
        tracing::info!(model_id = %item.id, display_name = %item.display_name, "Synced model config to DB");
    }
}

async fn get_sound(Path(sound): Path<SoundFile>) -> Result<Response, ApiError> {
    let sound = sound.serve().map_err(DeploymentError::Other)?;
    let response = Response::builder()
        .status(http::StatusCode::OK)
        .header(
            http::header::CONTENT_TYPE,
            http::HeaderValue::from_static("audio/wav"),
        )
        .body(Body::from(sound.data.into_owned()))
        .unwrap();
    Ok(response)
}

#[derive(TS, Debug, Deserialize)]
pub struct McpServerQuery {
    executor: BaseCodingAgent,
}

const MCP_NOT_SUPPORTED_ERROR_CODE: &str = "MCP_NOT_SUPPORTED";
const MCP_NOT_SUPPORTED_ERROR_MESSAGE: &str = "This executor does not support MCP servers";

#[derive(TS, Debug, Serialize, Deserialize)]
pub struct McpConfigError {
    code: String,
    message: String,
}

impl McpConfigError {
    fn not_supported() -> Self {
        Self {
            code: MCP_NOT_SUPPORTED_ERROR_CODE.to_string(),
            message: MCP_NOT_SUPPORTED_ERROR_MESSAGE.to_string(),
        }
    }
}

#[derive(TS, Debug, Serialize, Deserialize)]
pub struct GetMcpServerResponse {
    // servers: HashMap<String, Value>,
    mcp_config: McpConfig,
    config_path: String,
}

#[derive(TS, Debug, Serialize, Deserialize)]
pub struct UpdateMcpServersBody {
    servers: HashMap<String, Value>,
}

async fn get_mcp_servers(
    State(_deployment): State<DeploymentImpl>,
    Query(query): Query<McpServerQuery>,
) -> Result<
    (
        StatusCode,
        ResponseJson<ApiResponse<GetMcpServerResponse, McpConfigError>>,
    ),
    ApiError,
> {
    let coding_agent = ExecutorConfigs::get_cached()
        .get_coding_agent(&ExecutorProfileId::new(query.executor))
        .ok_or(ConfigError::ValidationError(
            "Executor not found".to_string(),
        ))?;

    if !coding_agent.supports_mcp() {
        return Ok((
            StatusCode::BAD_REQUEST,
            ResponseJson(ApiResponse::error_with_data(McpConfigError::not_supported())),
        ));
    }

    // Resolve supplied config path or agent default
    let Some(config_path) = coding_agent.default_mcp_config_path() else {
        return Err(ApiError::BadRequest(
            "Could not determine config file path".to_string(),
        ));
    };

    let mut mcpc = coding_agent.get_mcp_config();
    let raw_config = read_agent_config(&config_path, &mcpc).await?;
    let servers = get_mcp_servers_from_config_path(&raw_config, &mcpc.servers_path);
    mcpc.set_servers(servers);
    Ok((
        StatusCode::OK,
        ResponseJson(ApiResponse::success(GetMcpServerResponse {
            mcp_config: mcpc,
            config_path: config_path.to_string_lossy().to_string(),
        })),
    ))
}

async fn update_mcp_servers(
    State(_deployment): State<DeploymentImpl>,
    Query(query): Query<McpServerQuery>,
    Json(payload): Json<UpdateMcpServersBody>,
) -> Result<
    (
        StatusCode,
        ResponseJson<ApiResponse<String, McpConfigError>>,
    ),
    ApiError,
> {
    let profiles = ExecutorConfigs::get_cached();
    let agent = profiles
        .get_coding_agent(&ExecutorProfileId::new(query.executor))
        .ok_or(ConfigError::ValidationError(
            "Executor not found".to_string(),
        ))?;

    if !agent.supports_mcp() {
        return Ok((
            StatusCode::BAD_REQUEST,
            ResponseJson(ApiResponse::error_with_data(McpConfigError::not_supported())),
        ));
    }

    // Resolve supplied config path or agent default
    let Some(config_path) = agent.default_mcp_config_path() else {
        return Err(ApiError::BadRequest(
            "Could not determine config file path".to_string(),
        ));
    };
    let config_path = config_path.clone();

    let mcpc = agent.get_mcp_config();
    match update_mcp_servers_in_config(&config_path, &mcpc, payload.servers).await {
        Ok(message) => Ok((StatusCode::OK, ResponseJson(ApiResponse::success(message)))),
        Err(e) => Err(ApiError::Internal(format!(
            "Failed to update MCP servers: {e}"
        ))),
    }
}

async fn update_mcp_servers_in_config(
    config_path: &std::path::Path,
    mcpc: &McpConfig,
    new_servers: HashMap<String, Value>,
) -> Result<String, Box<dyn std::error::Error + Send + Sync>> {
    // Ensure parent directory exists
    if let Some(parent) = config_path.parent() {
        fs::create_dir_all(parent).await?;
    }
    // Read existing config (JSON or TOML depending on agent)
    let mut config = read_agent_config(config_path, mcpc).await?;

    // Get the current server count for comparison
    let old_servers = get_mcp_servers_from_config_path(&config, &mcpc.servers_path).len();

    // Set the MCP servers using the correct attribute path
    set_mcp_servers_in_config_path(&mut config, &mcpc.servers_path, &new_servers)?;

    // Write the updated config back to file (JSON or TOML depending on agent)
    write_agent_config(config_path, mcpc, &config).await?;

    let new_count = new_servers.len();
    let message = match (old_servers, new_count) {
        (0, 0) => "No MCP servers configured".to_string(),
        (0, n) => format!("Added {n} MCP server(s)"),
        (old, new) if old == new => {
            format!("Updated MCP server configuration ({new} server(s))")
        }
        (old, new) => format!("Updated MCP server configuration (was {old}, now {new})"),
    };

    Ok(message)
}

/// Helper function to get MCP servers from config using a path
fn get_mcp_servers_from_config_path(raw_config: &Value, path: &[String]) -> HashMap<String, Value> {
    let mut current = raw_config;
    for part in path {
        current = match current.get(part) {
            Some(val) => val,
            None => return HashMap::new(),
        };
    }
    // Extract the servers object
    match current.as_object() {
        Some(servers) => servers
            .iter()
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect(),
        None => HashMap::new(),
    }
}

/// Helper function to set MCP servers in config using a path
fn set_mcp_servers_in_config_path(
    raw_config: &mut Value,
    path: &[String],
    servers: &HashMap<String, Value>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    if path.is_empty() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidInput,
            "MCP servers path cannot be empty",
        )
        .into());
    }

    // Ensure config is an object
    if !raw_config.is_object() {
        *raw_config = serde_json::json!({});
    }

    let mut current = raw_config;
    // Navigate/create the nested structure (all parts except the last)
    for part in &path[..path.len() - 1] {
        if current.get(part).is_none() {
            current
                .as_object_mut()
                .unwrap()
                .insert(part.clone(), serde_json::json!({}));
        }
        current = current.get_mut(part).unwrap();
        if !current.is_object() {
            *current = serde_json::json!({});
        }
    }

    // Set the final attribute
    let final_attr = path.last().unwrap();
    current
        .as_object_mut()
        .unwrap()
        .insert(final_attr.clone(), serde_json::to_value(servers)?);

    Ok(())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn sample_servers() -> HashMap<String, Value> {
        let mut servers = HashMap::new();
        servers.insert(
            "demo".to_string(),
            json!({"type": "stdio", "command": "npx", "args": ["-y", "mcp-demo"]}),
        );
        servers
    }

    #[test]
    fn set_mcp_servers_in_config_path_rejects_empty_path() {
        let mut raw_config = json!({"keep": true});
        let before = raw_config.clone();
        let servers = sample_servers();

        let result = set_mcp_servers_in_config_path(&mut raw_config, &[], &servers);

        assert!(result.is_err());
        assert_eq!(raw_config, before);
    }

    #[test]
    fn set_mcp_servers_in_config_path_sets_single_segment_path() {
        let mut raw_config = json!({});
        let servers = sample_servers();
        let path = vec!["mcpServers".to_string()];

        set_mcp_servers_in_config_path(&mut raw_config, &path, &servers)
            .expect("setting mcp servers should succeed");

        let extracted = get_mcp_servers_from_config_path(&raw_config, &path);
        assert_eq!(extracted, servers);
    }

    #[test]
    fn set_mcp_servers_in_config_path_creates_nested_structure() {
        let mut raw_config = json!({"mcp": "invalid"});
        let servers = sample_servers();
        let path = vec!["mcp".to_string(), "servers".to_string()];

        set_mcp_servers_in_config_path(&mut raw_config, &path, &servers)
            .expect("setting nested mcp servers should succeed");

        let extracted = get_mcp_servers_from_config_path(&raw_config, &path);
        assert_eq!(extracted, servers);
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ProfilesContent {
    pub content: String,
    pub path: String,
}

async fn get_profiles(
    State(_deployment): State<DeploymentImpl>,
) -> ResponseJson<ApiResponse<ProfilesContent>> {
    let profiles_path = match utils::assets::profiles_path() {
        Ok(path) => path,
        Err(e) => {
            return ResponseJson(ApiResponse::error(&format!(
                "Failed to resolve profiles path: {e}"
            )));
        }
    };

    // Use cached data to ensure consistency with runtime and PUT updates
    let profiles = ExecutorConfigs::get_cached();

    let content = serde_json::to_string_pretty(&profiles).unwrap_or_else(|e| {
        tracing::error!("Failed to serialize profiles to JSON: {}", e);
        serde_json::to_string_pretty(&ExecutorConfigs::from_defaults())
            .unwrap_or_else(|_| "{}".to_string())
    });

    ResponseJson(ApiResponse::success(ProfilesContent {
        content,
        path: profiles_path.display().to_string(),
    }))
}

async fn update_profiles(
    State(_deployment): State<DeploymentImpl>,
    body: String,
) -> ResponseJson<ApiResponse<String>> {
    // Try to parse as ExecutorProfileConfigs format
    match serde_json::from_str::<ExecutorConfigs>(&body) {
        Ok(executor_profiles) => {
            // Save the profiles to file
            match executor_profiles.save_overrides() {
                Ok(()) => {
                    tracing::info!("Executor profiles saved successfully");
                    // Reload the cached profiles
                    ExecutorConfigs::reload();
                    ResponseJson(ApiResponse::success(
                        "Executor profiles updated successfully".to_string(),
                    ))
                }
                Err(e) => {
                    tracing::error!("Failed to save executor profiles: {}", e);
                    ResponseJson(ApiResponse::error(&format!(
                        "Failed to save executor profiles: {e}"
                    )))
                }
            }
        }
        Err(e) => ResponseJson(ApiResponse::error(&format!(
            "Invalid executor profiles format: {e}"
        ))),
    }
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct CheckEditorAvailabilityQuery {
    editor_type: EditorType,
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct CheckEditorAvailabilityResponse {
    available: bool,
}

async fn check_editor_availability(
    State(_deployment): State<DeploymentImpl>,
    Query(query): Query<CheckEditorAvailabilityQuery>,
) -> ResponseJson<ApiResponse<CheckEditorAvailabilityResponse>> {
    // Construct a minimal EditorConfig for checking
    let editor_config = EditorConfig::new(
        query.editor_type,
        None, // custom_command
        None, // remote_ssh_host
        None, // remote_ssh_user
    );

    let available = editor_config.check_availability().await;
    ResponseJson(ApiResponse::success(CheckEditorAvailabilityResponse {
        available,
    }))
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct CheckAgentAvailabilityQuery {
    executor: BaseCodingAgent,
}

async fn check_agent_availability(
    State(_deployment): State<DeploymentImpl>,
    Query(query): Query<CheckAgentAvailabilityQuery>,
) -> ResponseJson<ApiResponse<AvailabilityInfo>> {
    let profiles = ExecutorConfigs::get_cached();
    let profile_id = ExecutorProfileId::new(query.executor);

    let info = match profiles.get_coding_agent(&profile_id) {
        Some(agent) => agent.get_availability_info(),
        None => AvailabilityInfo::NotFound,
    };

    ResponseJson(ApiResponse::success(info))
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct InstallAiClisResponse {
    pub installed: bool,
    pub exit_code: i32,
    pub script_path: String,
    pub output: String,
}

fn truncate_output(value: &str, max_chars: usize) -> String {
    let mut chars = value.chars();
    let truncated: String = chars.by_ref().take(max_chars).collect();
    if chars.next().is_some() {
        format!("{truncated}\n...[truncated]")
    } else {
        truncated
    }
}

fn resolve_install_single_cli_script() -> Option<PathBuf> {
    if cfg!(target_os = "windows") {
        let mut candidates = Vec::new();

        // Check SOLODAWN_INSTALL_DIR (set by tray app)
        if let Some(install_dir) = utils::env_compat::var_opt_with_compat("SOLODAWN_INSTALL_DIR", "GITCORTEX_INSTALL_DIR") {
            candidates
                .push(PathBuf::from(&install_dir).join("scripts/install-single-cli.ps1"));
        }

        // Relative to executable
        if let Ok(exe_path) = std::env::current_exe() {
            if let Some(exe_dir) = exe_path.parent() {
                candidates.push(exe_dir.join("scripts/install-single-cli.ps1"));
            }
        }

        // Development paths
        if let Ok(cwd) = std::env::current_dir() {
            candidates.push(cwd.join("installer/scripts/install-single-cli.ps1"));
        }

        candidates.into_iter().find(|path| path.is_file())
    } else {
        // Unix: batch script
        let mut candidates = vec![PathBuf::from("/opt/solodawn/install/install-ai-clis.sh")];
        if let Ok(cwd) = std::env::current_dir() {
            candidates.push(cwd.join("scripts/docker/install/install-ai-clis.sh"));
        }
        candidates.push(PathBuf::from("scripts/docker/install/install-ai-clis.sh"));

        candidates.into_iter().find(|path| path.is_file())
    }
}

/// CLI names to install in batch mode.
const BATCH_INSTALL_CLIS: &[&str] = &[
    "claude-code",
    "codex",
    "gemini-cli",
    "amp",
    "cursor-agent",
    "qwen-code",
    "opencode",
    "droid",
];

async fn install_ai_clis(
    State(_deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<InstallAiClisResponse>>, ApiError> {
    let Some(script_path) = resolve_install_single_cli_script() else {
        return Err(ApiError::BadRequest(
            "AI CLI install script not found".to_string(),
        ));
    };

    if cfg!(target_os = "windows") {
        // Windows: invoke install-single-cli.ps1 for each CLI sequentially
        let mut combined_output = String::new();
        let mut all_success = true;
        let mut last_exit_code = 0;

        for cli_name in BATCH_INSTALL_CLIS {
            combined_output.push_str(&format!("=== Installing {cli_name} ===\n"));

            let mut command = Command::new("powershell.exe");
            command
                .arg("-ExecutionPolicy")
                .arg("Bypass")
                .arg("-File")
                .arg(&script_path)
                .arg("install")
                .arg(cli_name)
                .stdout(Stdio::piped())
                .stderr(Stdio::piped());

            match tokio::time::timeout(Duration::from_secs(300), command.output()).await {
                Ok(Ok(output)) => {
                    let stdout = String::from_utf8_lossy(&output.stdout);
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    if !stdout.is_empty() {
                        combined_output.push_str(&stdout);
                    }
                    if !stderr.is_empty() {
                        combined_output.push_str(&stderr);
                    }
                    let code = output.status.code().unwrap_or(-1);
                    if code != 0 {
                        all_success = false;
                        last_exit_code = code;
                        combined_output
                            .push_str(&format!("[WARN] {cli_name} exited with code {code}\n"));
                    }
                }
                Ok(Err(err)) => {
                    all_success = false;
                    last_exit_code = -1;
                    combined_output
                        .push_str(&format!("[ERROR] Failed to run script for {cli_name}: {err}\n"));
                }
                Err(_) => {
                    all_success = false;
                    last_exit_code = -1;
                    combined_output
                        .push_str(&format!("[ERROR] {cli_name} installation timed out\n"));
                }
            }
            combined_output.push('\n');
        }

        Ok(ResponseJson(ApiResponse::success(InstallAiClisResponse {
            installed: all_success,
            exit_code: last_exit_code,
            script_path: script_path.display().to_string(),
            output: truncate_output(&combined_output, 16_000),
        })))
    } else {
        // Unix: run batch install script directly
        let mut command = Command::new("bash");
        command
            .arg(&script_path)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());

        let install_result =
            match tokio::time::timeout(Duration::from_secs(1800), command.output()).await {
                Ok(Ok(output)) => {
                    let stdout = String::from_utf8_lossy(&output.stdout);
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    let combined = if stderr.trim().is_empty() {
                        stdout.to_string()
                    } else if stdout.trim().is_empty() {
                        stderr.to_string()
                    } else {
                        format!("{stdout}\n{stderr}")
                    };
                    InstallAiClisResponse {
                        installed: output.status.success(),
                        exit_code: output.status.code().unwrap_or(-1),
                        script_path: script_path.display().to_string(),
                        output: truncate_output(&combined, 16_000),
                    }
                }
                Ok(Err(err)) => InstallAiClisResponse {
                    installed: false,
                    exit_code: -1,
                    script_path: script_path.display().to_string(),
                    output: format!("Failed to execute install script: {err}"),
                },
                Err(_) => InstallAiClisResponse {
                    installed: false,
                    exit_code: -1,
                    script_path: script_path.display().to_string(),
                    output: "AI CLI installation timed out after 30 minutes".to_string(),
                },
            };

        Ok(ResponseJson(ApiResponse::success(install_result)))
    }
}

// ============================================================================
// System prerequisites detection
// ============================================================================

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct PrerequisiteStatus {
    pub name: String,
    pub found: bool,
    pub version: Option<String>,
    pub required: bool,
    pub hint: String,
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct SystemPrerequisites {
    pub items: Vec<PrerequisiteStatus>,
}

fn detect_tool_version(cmd: &str, args: &[&str]) -> (bool, Option<String>) {
    // On Windows, use cmd.exe /C to handle .cmd/.bat files (e.g. npm.cmd)
    let result = if cfg!(target_os = "windows") {
        let full_cmd = std::iter::once(cmd.to_string())
            .chain(args.iter().map(|a| (*a).to_string()))
            .collect::<Vec<_>>()
            .join(" ");
        std::process::Command::new("cmd.exe")
            .args(["/C", &full_cmd])
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .output()
    } else {
        std::process::Command::new(cmd)
            .args(args)
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .output()
    };

    match result {
        Ok(output) if output.status.success() => {
            let stdout = String::from_utf8_lossy(&output.stdout);
            let version = stdout.lines().next().unwrap_or("").trim().to_string();
            (true, if version.is_empty() { None } else { Some(version) })
        }
        _ => (false, None),
    }
}

async fn get_system_prerequisites() -> ResponseJson<ApiResponse<SystemPrerequisites>> {
    // Run blocking detection in a spawn_blocking to avoid blocking the async runtime
    let items = tokio::task::spawn_blocking(|| {
        let mut items = Vec::new();

        // Node.js
        let (found, version) = detect_tool_version("node", &["--version"]);
        items.push(PrerequisiteStatus {
            name: "Node.js".to_string(),
            found,
            version,
            required: true,
            hint: "Install from https://nodejs.org (v18+)".to_string(),
        });

        // npm
        let (found, version) = detect_tool_version("npm", &["--version"]);
        items.push(PrerequisiteStatus {
            name: "npm".to_string(),
            found,
            version,
            required: true,
            hint: "Included with Node.js installation".to_string(),
        });

        // Git
        let (found, version) = detect_tool_version("git", &["--version"]);
        items.push(PrerequisiteStatus {
            name: "Git".to_string(),
            found,
            version,
            required: true,
            hint: "Install from https://git-scm.com".to_string(),
        });

        // GitHub CLI (optional)
        let (found, version) = detect_tool_version("gh", &["--version"]);
        let version = version.and_then(|v| v.lines().next().map(|l| l.to_string()));
        items.push(PrerequisiteStatus {
            name: "GitHub CLI (gh)".to_string(),
            found,
            version,
            required: false,
            hint: "Optional. Install from https://cli.github.com".to_string(),
        });

        items
    })
    .await
    .unwrap_or_default();

    ResponseJson(ApiResponse::success(SystemPrerequisites { items }))
}
