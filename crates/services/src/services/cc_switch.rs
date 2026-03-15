//! CC-Switch 服务
//!
//! 封装 cc-switch crate，提供与 gitcortex 集成的接口。
//!
//! ## 进程隔离架构 (Phase 23)
//!
//! 新增 `build_launch_config` 方法实现进程级别的配置隔离：
//! - 通过环境变量注入配置，而非修改全局配置文件
//! - 支持多工作流并发运行，配置互不干扰
//! - 用户全局配置保持不变
//!
//! ## 自动确认参数 (Phase 24)
//!
//! 支持为各 CLI 注入自动确认参数：
//! - Claude Code: `--dangerously-skip-permissions`
//! - Codex: `--yolo`
//! - Gemini: `--yolo`

use std::{path::Path, sync::Arc};

use async_trait::async_trait;
use cc_switch::{CliType as CcCliType, ModelSwitcher, SwitchConfig, read_claude_config};
use db::{
    DBService,
    models::{CliType, ModelConfig, Terminal, Workflow},
};

use crate::services::terminal::process::{SpawnCommand, SpawnEnv};

// ============================================================================
// Authentication Skip Helpers
// ============================================================================

/// Creates Codex auth.json in CODEX_HOME with API key
fn create_codex_auth(codex_home: &Path, api_key: &str) -> anyhow::Result<()> {
    let auth_path = codex_home.join("auth.json");

    let auth_content = serde_json::json!({
        "OPENAI_API_KEY": api_key
    });

    let auth_str = serde_json::to_string_pretty(&auth_content)?;
    std::fs::write(&auth_path, auth_str)
        .map_err(|e| anyhow::anyhow!("Failed to write Codex auth.json: {e}"))?;

    // Set restrictive permissions on Unix
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if let Err(e) = std::fs::set_permissions(&auth_path, std::fs::Permissions::from_mode(0o600))
        {
            tracing::warn!(
                auth_path = %auth_path.display(),
                error = %e,
                "Failed to set restrictive permissions on Codex auth.json"
            );
        }
    }

    tracing::debug!(
        codex_home = %codex_home.display(),
        "Created Codex auth.json for authentication skip"
    );

    Ok(())
}

/// Resolve Codex wire protocol.
/// Default to `responses` for OpenAI-compatible gateways.
/// Allow override via env: GITCORTEX_CODEX_WIRE_API=responses|codex
fn resolve_codex_wire_api() -> String {
    if let Ok(raw) = std::env::var("GITCORTEX_CODEX_WIRE_API") {
        let normalized = raw.trim().to_ascii_lowercase();
        if normalized == "responses" || normalized == "codex" {
            return normalized;
        }
        tracing::warn!(
            configured = %raw,
            "Invalid GITCORTEX_CODEX_WIRE_API value; expected 'responses' or 'codex', falling back to 'responses'"
        );
    }

    "responses".to_string()
}

/// Creates Codex config.toml in CODEX_HOME to skip authentication
///
/// [G22-010] TODO: The `api_key` field appears both in `[model_providers.<key>]` and
/// is also injected via `OPENAI_API_KEY` env var. If Codex reads both, the config.toml
/// key may shadow or conflict with the env var. Verify Codex precedence rules and
/// consider removing the duplicate to avoid confusion.
fn create_codex_config(
    codex_home: &Path,
    base_url: Option<&str>,
    model: &str,
    api_key: &str,
) -> anyhow::Result<()> {
    let config_path = codex_home.join("config.toml");

    // Use a custom provider when a custom base URL is configured.
    let (provider_key, base_url_str) = match base_url {
        Some(url) => ("custom", url),
        None => ("openai", "https://api.openai.com/v1"),
    };
    let wire_api = resolve_codex_wire_api();

    let mut config_content = format!(
        r#"model_provider = "{provider_key}"
model = "{model}"

[model_providers.{provider_key}]
name = "{provider_key}"
base_url = "{base_url_str}"
api_key = "{api_key}"
"#
    );

    // Default to OpenAI Responses API for compatibility with most custom gateways.
    // Set GITCORTEX_CODEX_WIRE_API=codex when provider explicitly requires /codex.
    config_content.push_str(&format!("wire_api = \"{wire_api}\"\n"));

    std::fs::write(&config_path, config_content)
        .map_err(|e| anyhow::anyhow!("Failed to write Codex config.toml: {e}"))?;

    tracing::info!(
        codex_home = %codex_home.display(),
        config_path = %config_path.display(),
        model_provider = %provider_key,
        base_url = %base_url_str,
        wire_api = %wire_api,
        "Created Codex config.toml for authentication skip"
    );

    Ok(())
}

/// Creates Claude Code config.json in isolated directory.
///
/// Keep `primaryApiKey` aligned with terminal key to avoid runtime precedence
/// ambiguity between `config.json`, `settings.json`, and env-based auth.
fn create_claude_config(claude_home: &Path, api_key: &str) -> anyhow::Result<()> {
    let config_path = claude_home.join("config.json");

    // Keep primaryApiKey synced with terminal key.
    // Some Claude runtime paths prefer config key over env token.
    // Preserve other fields if file exists
    let config_content = if config_path.exists() {
        let existing = std::fs::read_to_string(&config_path)?;
        let mut value: serde_json::Value =
            serde_json::from_str(&existing).unwrap_or_else(|_| serde_json::json!({}));

        if let Some(obj) = value.as_object_mut() {
            obj.insert(
                "primaryApiKey".to_string(),
                serde_json::Value::String(api_key.to_string()),
            );
        }

        serde_json::to_string_pretty(&value)?
    } else {
        serde_json::to_string_pretty(&serde_json::json!({
            "primaryApiKey": api_key
        }))?
    };

    std::fs::write(&config_path, config_content)
        .map_err(|e| anyhow::anyhow!("Failed to write Claude config.json: {e}"))?;

    tracing::debug!(
        claude_home = %claude_home.display(),
        "Created Claude Code config.json for authentication skip"
    );

    Ok(())
}

/// Creates Claude Code settings.json in isolated directory and returns its path.
/// This is passed via `--settings <path>` to avoid global ~/.claude settings interference.
fn create_claude_settings(
    claude_home: &Path,
    api_key: &str,
    base_url: Option<&str>,
    model: &str,
) -> anyhow::Result<std::path::PathBuf> {
    let settings_path = claude_home.join("settings.json");

    let mut env_obj = serde_json::Map::new();
    // Choose auth env var based on key format:
    // - sk- prefix → direct API key, use ANTHROPIC_API_KEY only
    // - otherwise → session/OAuth token, use ANTHROPIC_AUTH_TOKEN only
    if api_key.starts_with("sk-") {
        env_obj.insert(
            "ANTHROPIC_API_KEY".to_string(),
            serde_json::Value::String(api_key.to_string()),
        );
    } else {
        env_obj.insert(
            "ANTHROPIC_AUTH_TOKEN".to_string(),
            serde_json::Value::String(api_key.to_string()),
        );
    }
    env_obj.insert(
        "ANTHROPIC_MODEL".to_string(),
        serde_json::Value::String(model.to_string()),
    );
    env_obj.insert(
        "ANTHROPIC_DEFAULT_HAIKU_MODEL".to_string(),
        serde_json::Value::String(model.to_string()),
    );
    env_obj.insert(
        "ANTHROPIC_DEFAULT_SONNET_MODEL".to_string(),
        serde_json::Value::String(model.to_string()),
    );
    env_obj.insert(
        "ANTHROPIC_DEFAULT_OPUS_MODEL".to_string(),
        serde_json::Value::String(model.to_string()),
    );
    if let Some(url) = base_url {
        env_obj.insert(
            "ANTHROPIC_BASE_URL".to_string(),
            serde_json::Value::String(url.to_string()),
        );
    }

    let settings = serde_json::json!({
        "env": env_obj,
        "primaryApiKey": api_key,
    });

    let content = serde_json::to_string_pretty(&settings)?;
    std::fs::write(&settings_path, content)
        .map_err(|e| anyhow::anyhow!("Failed to write Claude settings.json: {e}"))?;

    tracing::debug!(
        settings_path = %settings_path.display(),
        "Created Claude Code settings.json for isolated authentication"
    );

    Ok(settings_path)
}

/// Creates Gemini .env in isolated directory to skip authentication
fn create_gemini_env(
    gemini_home: &Path,
    api_key: &str,
    base_url: Option<&str>,
    model: &str,
) -> anyhow::Result<()> {
    let env_path = gemini_home.join(".env");

    let mut env_content = format!("GEMINI_API_KEY={api_key}\nGEMINI_MODEL={model}\n");

    if let Some(url) = base_url {
        env_content.push_str(&format!("GOOGLE_GEMINI_BASE_URL={url}\n"));
    }

    std::fs::write(&env_path, env_content)
        .map_err(|e| anyhow::anyhow!("Failed to write Gemini .env: {e}"))?;

    // Set restrictive permissions on Unix
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if let Err(e) = std::fs::set_permissions(&env_path, std::fs::Permissions::from_mode(0o600))
        {
            tracing::warn!(
                env_path = %env_path.display(),
                error = %e,
                "Failed to set restrictive permissions on Gemini .env"
            );
        }
    }

    tracing::debug!(
        gemini_home = %gemini_home.display(),
        "Created Gemini .env for authentication skip"
    );

    Ok(())
}

// NOTE: create_opencode_config was removed as dead code (G20-012).
// OpenCode configuration is handled via environment variables at launch time,
// not via config file creation.

// ============================================================================
// Auto-Confirm Parameters
// ============================================================================

/// Applies CLI-specific auto-confirm arguments.
///
/// # Arguments
///
/// * `cli` - The CLI type
/// * `args` - Mutable reference to the arguments vector
/// * `auto_confirm` - Whether to add auto-confirm flags
fn apply_auto_confirm_args(cli: &CcCliType, args: &mut Vec<String>, auto_confirm: bool) {
    if !auto_confirm {
        return;
    }

    let flag = match cli {
        CcCliType::ClaudeCode => "--dangerously-skip-permissions",
        CcCliType::Codex | CcCliType::Gemini => "--yolo",
        _ => return,
    };

    // Avoid duplicate flags
    if args.iter().any(|arg| arg == flag) {
        return;
    }

    args.push(flag.to_string());
}

/// Sanitize a terminal ID for use in filesystem paths.
///
/// Replaces non-alphanumeric characters (except `-` and `_`) with `_` and
/// truncates to 64 characters to prevent path traversal attacks.
fn sanitize_terminal_id(id: &str) -> String {
    id.chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || c == '-' || c == '_' {
                c
            } else {
                '_'
            }
        })
        .take(64)
        .collect()
}

/// G20-013/G22-008: Shared helper to create an isolated home directory for a CLI.
///
/// Creates `<temp>/gitcortex/<prefix>-<sanitized_terminal_id>` with restrictive
/// permissions on Unix (0o700). Returns the created directory path.
fn create_isolated_home(terminal_id: &str, prefix: &str) -> anyhow::Result<std::path::PathBuf> {
    let safe_id = sanitize_terminal_id(terminal_id);
    let base_dir = std::env::temp_dir().join("gitcortex");
    std::fs::create_dir_all(&base_dir).map_err(|e| {
        anyhow::anyhow!(
            "Failed to create {prefix} home base directory {}: {e}",
            base_dir.display()
        )
    })?;
    let home = base_dir.join(format!("{prefix}-{safe_id}"));
    std::fs::create_dir_all(&home).map_err(|e| {
        anyhow::anyhow!(
            "Failed to create {prefix} home directory {}: {e}",
            home.display()
        )
    })?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if let Err(e) = std::fs::set_permissions(&home, std::fs::Permissions::from_mode(0o700)) {
            tracing::warn!(
                terminal_id = %terminal_id,
                home = %home.display(),
                error = %e,
                "Failed to set restrictive permissions on {prefix} home"
            );
        }
    }

    Ok(home)
}

/// CC-Switch trait for dependency injection and testing
#[async_trait]
pub trait CCSwitch: Send + Sync {
    /// Switch model configuration for a terminal
    async fn switch_for_terminal(&self, terminal: &Terminal) -> anyhow::Result<()>;
}

/// CC-Switch 服务
pub struct CCSwitchService {
    db: Arc<DBService>,
    switcher: ModelSwitcher,
}

impl CCSwitchService {
    const DEFAULT_CLAUDE_FALLBACK_MODEL: &'static str = "claude-sonnet-4-20250514";

    pub fn new(db: Arc<DBService>) -> Self {
        Self {
            db,
            switcher: ModelSwitcher::new(),
        }
    }

    fn resolve_model_name(model_config: &ModelConfig) -> String {
        model_config
            .api_model_id
            .clone()
            .unwrap_or_else(|| model_config.name.clone())
    }

    fn looks_like_claude_model(model: &str) -> bool {
        let lower = model.trim().to_ascii_lowercase();
        if lower.is_empty() {
            return false;
        }
        lower.contains("claude")
            || matches!(
                lower.as_str(),
                "sonnet" | "haiku" | "opus" | "claude-sonnet" | "claude-haiku" | "claude-opus"
            )
    }

    async fn resolve_claude_launch_model(
        &self,
        terminal: &Terminal,
        model_config: &ModelConfig,
        effective_base_url: Option<&str>,
    ) -> anyhow::Result<String> {
        let requested_model = Self::resolve_model_name(model_config);

        // Custom Anthropic-compatible gateways may legitimately use non-Claude model names.
        if effective_base_url.is_some() || Self::looks_like_claude_model(&requested_model) {
            return Ok(requested_model);
        }

        if let Some(default_model) =
            ModelConfig::find_default_for_cli(&self.db.pool, &terminal.cli_type_id).await?
        {
            let fallback_model = Self::resolve_model_name(&default_model);
            if !fallback_model.trim().is_empty() {
                tracing::warn!(
                    terminal_id = %terminal.id,
                    model_config_id = %terminal.model_config_id,
                    requested_model = %requested_model,
                    fallback_model = %fallback_model,
                    "Invalid Claude model for official endpoint; falling back to CLI default model"
                );
                return Ok(fallback_model);
            }
        }

        tracing::warn!(
            terminal_id = %terminal.id,
            model_config_id = %terminal.model_config_id,
            requested_model = %requested_model,
            fallback_model = Self::DEFAULT_CLAUDE_FALLBACK_MODEL,
            "Invalid Claude model for official endpoint; falling back to hardcoded Claude model"
        );
        Ok(Self::DEFAULT_CLAUDE_FALLBACK_MODEL.to_string())
    }

    async fn resolve_workflow_orchestrator_fallback(
        &self,
        workflow_task_id: &str,
    ) -> anyhow::Result<(Option<String>, Option<String>)> {
        let workflow_id: Option<String> =
            sqlx::query_scalar("SELECT workflow_id FROM workflow_task WHERE id = ? LIMIT 1")
                .bind(workflow_task_id)
                .fetch_optional(&self.db.pool)
                .await?
                .flatten();

        let Some(workflow_id) = workflow_id else {
            return Ok((None, None));
        };

        let workflow = if let Some(workflow) = Workflow::find_by_id(&self.db.pool, &workflow_id).await? { workflow } else {
            tracing::warn!(
                workflow_id = %workflow_id,
                workflow_task_id = %workflow_task_id,
                "Workflow not found while resolving Codex API fallback"
            );
            return Ok((None, None));
        };

        let api_key = match workflow.get_api_key() {
            Ok(api_key) => api_key,
            Err(e) => {
                tracing::warn!(
                    workflow_id = %workflow_id,
                    error = %e,
                    "Failed to decrypt workflow orchestrator API key for Codex fallback"
                );
                None
            }
        };

        Ok((workflow.orchestrator_base_url.clone(), api_key))
    }
}

#[async_trait]
impl CCSwitch for CCSwitchService {
    /// 为终端切换模型
    ///
    /// 根据终端配置切换对应 CLI 的模型。
    ///
    /// # Deprecated
    ///
    /// This method modifies global configuration files. Use `build_launch_config` instead
    /// for process-level isolation.
    ///
    /// [G22-002] WARNING: This method writes to global config files and is NOT safe for
    /// concurrent use across multiple terminals/workflows. It is kept only for backward
    /// compatibility. All new code paths MUST use `build_launch_config` which provides
    /// per-process environment variable isolation. TODO: Add a compile-time gate
    /// (e.g., `#[cfg(feature = "legacy-global-switch")]`) to prevent accidental use.
    #[allow(deprecated)]
    async fn switch_for_terminal(&self, terminal: &Terminal) -> anyhow::Result<()> {
        // 获取 CLI 类型信息
        let cli_type = CliType::find_by_id(&self.db.pool, &terminal.cli_type_id)
            .await?
            .ok_or_else(|| anyhow::anyhow!("CLI type not found: {}", terminal.cli_type_id))?;

        // 获取模型配置
        let model_config = ModelConfig::find_by_id(&self.db.pool, &terminal.model_config_id)
            .await?
            .ok_or_else(|| {
                anyhow::anyhow!("Model config not found: {}", terminal.model_config_id)
            })?;

        // 解析 CLI 类型
        let cli = CcCliType::parse(&cli_type.name)
            .ok_or_else(|| anyhow::anyhow!("Unsupported CLI: {}", cli_type.name))?;

        // Resolve API key based on CLI type
        let api_key = match cli {
            CcCliType::ClaudeCode => {
                // For Claude Code: try custom_api_key first, then read from existing config
                if let Some(custom) = terminal.get_custom_api_key()? {
                    custom
                } else {
                    // Try to read from existing Claude config
                    let config = match read_claude_config().await {
                        Ok(cfg) => cfg,
                        Err(e) => {
                            tracing::warn!(
                                "Failed to read Claude config file: {}. Will check for auth token anyway.",
                                e
                            );
                            Default::default()
                        }
                    };
                    config.env.auth_token
                        .or(config.env.api_key)
                        .ok_or_else(|| anyhow::anyhow!(
                            "Claude Code auth token not configured. Please login via CLI (claude login) or set terminal custom_api_key"
                        ))?
                }
            }
            _ => {
                // For other CLIs: require custom_api_key
                terminal
                    .get_custom_api_key()?
                    .ok_or_else(|| anyhow::anyhow!("API key not configured for terminal"))?
            }
        };

        // 构建切换配置
        let config = SwitchConfig {
            base_url: terminal.custom_base_url.clone(),
            api_key,
            model: model_config
                .api_model_id
                .clone()
                .unwrap_or_else(|| model_config.name.clone()),
        };

        // 执行切换
        self.switcher.switch(cli, &config).await?;

        tracing::info!(
            "Switched model for terminal {}: cli={}, model={}",
            terminal.id,
            cli_type.display_name,
            model_config.display_name
        );

        Ok(())
    }
}

impl CCSwitchService {
    /// Build spawn configuration for a terminal without modifying global config files.
    ///
    /// This method implements process-level isolation by returning environment variables
    /// and CLI arguments instead of writing to global configuration files.
    ///
    /// # Supported CLIs
    ///
    /// - **Claude Code**: Sets ANTHROPIC_BASE_URL, ANTHROPIC_AUTH_TOKEN, ANTHROPIC_MODEL,
    ///   and ANTHROPIC_DEFAULT_*_MODEL environment variables.
    ///   Auto-confirm: `--dangerously-skip-permissions`
    /// - **Codex**: Sets OPENAI_API_KEY, OPENAI_BASE_URL, CODEX_HOME (temp directory),
    ///   and CLI arguments --model and --config.
    ///   Auto-confirm: `--yolo`
    /// - **Gemini**: Sets GOOGLE_GEMINI_BASE_URL, GEMINI_API_KEY, GEMINI_MODEL.
    ///   Auto-confirm: `--yolo`
    ///
    /// # Arguments
    ///
    /// * `terminal` - Terminal configuration from database
    /// * `base_command` - CLI command to execute (e.g., "claude", "codex", "gemini")
    /// * `working_dir` - Working directory for the spawned process
    /// * `auto_confirm` - Whether to add CLI auto-confirm flags
    ///
    /// # Returns
    ///
    /// Returns a `SpawnCommand` containing command, args, working_dir, and env configuration.
    /// For unsupported CLIs, returns an empty configuration (does not fail).
    pub async fn build_launch_config(
        &self,
        terminal: &Terminal,
        base_command: &str,
        working_dir: &Path,
        auto_confirm: bool,
    ) -> anyhow::Result<SpawnCommand> {
        // Fetch CLI type information
        let cli_type = CliType::find_by_id(&self.db.pool, &terminal.cli_type_id)
            .await?
            .ok_or_else(|| anyhow::anyhow!("CLI type not found: {}", terminal.cli_type_id))?;

        // Helper to create empty config for unsupported CLIs
        let empty_config = || SpawnCommand {
            command: base_command.to_string(),
            args: Vec::new(),
            working_dir: working_dir.to_path_buf(),
            env: SpawnEnv::default(),
        };

        // Parse CLI type
        let cli = if let Some(cli) = CcCliType::parse(&cli_type.name) { cli } else {
            tracing::warn!(
                cli_name = %cli_type.name,
                terminal_id = %terminal.id,
                "CLI does not support config switching, using empty config"
            );
            return Ok(empty_config());
        };

        // Only Claude Code, Codex, and Gemini support environment-based configuration
        if !matches!(
            cli,
            CcCliType::ClaudeCode | CcCliType::Codex | CcCliType::Gemini
        ) {
            tracing::warn!(
                cli_name = %cli_type.name,
                terminal_id = %terminal.id,
                "CLI does not support config switching, using empty config"
            );
            return Ok(empty_config());
        }

        // Fetch model configuration
        let model_config = ModelConfig::find_by_id(&self.db.pool, &terminal.model_config_id)
            .await?
            .ok_or_else(|| {
                anyhow::anyhow!("Model config not found: {}", terminal.model_config_id)
            })?;

        let mut env = SpawnEnv::default();
        let mut args = Vec::new();

        match cli {
            CcCliType::ClaudeCode => {
                // Create isolated Claude home directory
                let claude_home = create_isolated_home(&terminal.id, "claude")?;

                // Set CLAUDE_HOME to isolated directory
                // [G19-006] TODO: CLAUDE_HOME directories are cleaned up only for Codex
                // (via CodexHomeGuard in process.rs). Claude and Gemini isolated home dirs
                // are not cleaned up on terminal lifecycle end, causing disk leakage and
                // potential API key residue. Add similar cleanup logic for CLAUDE_HOME and
                // GEMINI_HOME in ProcessManager::finalize_terminated_process().
                // [G22-005] TODO: Register all temp isolation dirs (CLAUDE_HOME, GEMINI_HOME,
                // CODEX_HOME) for cleanup on process exit. Consider a unified TempDirGuard.
                // [G22-006] TODO: On Windows, temp dir permissions cannot be set via Unix
                // chmod. Investigate Windows ACL APIs for restricting access to isolated dirs.
                env.set.insert(
                    "CLAUDE_HOME".to_string(),
                    claude_home.to_string_lossy().to_string(),
                );

                let custom_api_key = terminal.get_custom_api_key()?;
                let (orchestrator_base_url, orchestrator_api_key) =
                    if terminal.custom_base_url.is_none() || custom_api_key.is_none() {
                        self.resolve_workflow_orchestrator_fallback(&terminal.workflow_task_id)
                            .await?
                    } else {
                        (None, None)
                    };
                let effective_base_url = terminal
                    .custom_base_url
                    .clone()
                    .or(orchestrator_base_url.clone());

                // Handle base URL: terminal custom_url first, then workflow orchestrator fallback.
                if let Some(base_url) = effective_base_url.as_ref() {
                    env.set
                        .insert("ANTHROPIC_BASE_URL".to_string(), base_url.clone());
                } else {
                    env.unset.push("ANTHROPIC_BASE_URL".to_string());
                }

                // Resolve API key with fallback chain:
                // 1. Terminal custom_api_key
                // 2. Global Claude config (~/.claude/settings.json) - only for official Anthropic API
                // 3. Workflow orchestrator config (only if base URLs are compatible)
                let mut fallback_api_key = None;

                if custom_api_key.is_none() {
                    // Try global Claude config first, but ONLY if terminal uses official Anthropic API
                    // Global config is designed for Anthropic API and won't work with custom endpoints
                    if effective_base_url.is_none() {
                        let config = match read_claude_config().await {
                            Ok(cfg) => cfg,
                            Err(e) => {
                                tracing::warn!(
                                    error = %e,
                                    "Failed to read Claude config file, will try workflow orchestrator fallback"
                                );
                                Default::default()
                            }
                        };
                        fallback_api_key = config.env.auth_token.or(config.env.api_key);
                    }

                    // If global config also doesn't have API key, try workflow orchestrator
                    // BUT only if the base URLs are compatible (same API service)
                    if fallback_api_key.is_none() {
                        // Check if base URLs are compatible
                        let terminal_base_url = terminal.custom_base_url.as_deref();
                        let can_use_fallback =
                            match (terminal_base_url, orchestrator_base_url.as_deref()) {
                                // Terminal does not pin base_url: workflow fallback is allowed.
                                (None, _) => true,
                                // Terminal pins custom endpoint: fallback key must match same endpoint.
                                (Some(t_url), Some(o_url)) if t_url == o_url => true,
                                _ => false,
                            };

                        if can_use_fallback {
                            fallback_api_key = orchestrator_api_key.clone();
                            if fallback_api_key.is_some() {
                                tracing::info!(
                                    terminal_id = %terminal.id,
                                    workflow_task_id = %terminal.workflow_task_id,
                                    "Using workflow orchestrator API key as Claude Code terminal fallback"
                                );
                            }
                        } else {
                            tracing::warn!(
                                terminal_id = %terminal.id,
                                terminal_base_url = ?terminal_base_url,
                                orchestrator_base_url = ?orchestrator_base_url,
                                "Cannot use workflow orchestrator API key fallback: base URLs are incompatible"
                            );
                        }
                    }
                }

                // Determine API key source before moving values
                let api_key_source = if custom_api_key.is_some() {
                    "terminal custom_api_key"
                } else if fallback_api_key.is_some() {
                    "fallback (global config or orchestrator)"
                } else {
                    "none"
                };

                let api_key = custom_api_key.or(fallback_api_key).ok_or_else(|| {
                    if effective_base_url.is_some() {
                        anyhow::anyhow!(
                            "Claude Code auth token not configured for custom API endpoint. Please set terminal custom_api_key"
                        )
                    } else {
                        anyhow::anyhow!(
                            "Claude Code auth token not configured. Please login via CLI (claude login), set terminal custom_api_key, or configure workflow orchestrator API key"
                        )
                    }
                })?;

                // Log API key source for debugging
                tracing::info!(
                    terminal_id = %terminal.id,
                    api_key_source = api_key_source,
                    has_custom_base_url = terminal.custom_base_url.is_some(),
                    custom_base_url = ?terminal.custom_base_url,
                    api_key_prefix = &api_key[..api_key.len().min(4)],
                    "Resolved API key for Claude Code terminal"
                );

                // G22-003: Propagate config creation failure instead of silently swallowing it.
                // A missing config.json can cause Claude Code to fall back to global auth,
                // leading to unexpected billing or auth errors.
                create_claude_config(&claude_home, &api_key).map_err(|e| {
                    tracing::error!(
                        terminal_id = %terminal.id,
                        error = %e,
                        "Failed to create Claude config.json for authentication skip"
                    );
                    e
                })?;

                let model = self
                    .resolve_claude_launch_model(
                        terminal,
                        &model_config,
                        effective_base_url.as_deref(),
                    )
                    .await?;

                // Create settings.json and force Claude CLI to load it via --settings.
                // This prevents global ~/.claude/settings.json from overriding isolated auth config.
                // Use effective_base_url (which considers orchestrator fallback) instead of just
                // terminal-level URL, so the settings file reflects the actual endpoint in use.
                let settings_path = create_claude_settings(
                    &claude_home,
                    &api_key,
                    effective_base_url.as_deref(),
                    &model,
                )?;
                args.push("--settings".to_string());
                args.push(settings_path.to_string_lossy().to_string());

                // Inject auth env var based on key format:
                // - sk- prefix → direct API key, use ANTHROPIC_API_KEY only
                // - otherwise → session/OAuth token, use ANTHROPIC_AUTH_TOKEN only
                if api_key.starts_with("sk-") {
                    env.set
                        .insert("ANTHROPIC_API_KEY".to_string(), api_key.clone());
                } else {
                    env.set
                        .insert("ANTHROPIC_AUTH_TOKEN".to_string(), api_key.clone());
                }

                // Set model for all tiers
                env.set.insert("ANTHROPIC_MODEL".to_string(), model.clone());
                env.set
                    .insert("ANTHROPIC_DEFAULT_HAIKU_MODEL".to_string(), model.clone());
                env.set
                    .insert("ANTHROPIC_DEFAULT_SONNET_MODEL".to_string(), model.clone());
                env.set
                    .insert("ANTHROPIC_DEFAULT_OPUS_MODEL".to_string(), model);

                tracing::debug!(
                    terminal_id = %terminal.id,
                    cli = "claude-code",
                    claude_home = %claude_home.display(),
                    "Built launch config for Claude Code with authentication skip"
                );
            }
            CcCliType::Codex => {
                // Codex requires API key.
                // Prefer terminal-level key, then fallback to workflow orchestrator config.
                let custom_api_key = terminal.get_custom_api_key()?;
                let mut fallback_base_url = None;
                let mut fallback_api_key = None;

                if custom_api_key.is_none() || terminal.custom_base_url.is_none() {
                    let (base_url, api_key) = self
                        .resolve_workflow_orchestrator_fallback(&terminal.workflow_task_id)
                        .await?;
                    fallback_base_url = base_url;
                    fallback_api_key = api_key;
                }

                let used_workflow_fallback = custom_api_key.is_none() && fallback_api_key.is_some();
                let api_key = custom_api_key.or(fallback_api_key).ok_or_else(|| {
                    anyhow::anyhow!(
                        "Codex requires API key (set terminal.custom_api_key or workflow.orchestrator_config.api_key)"
                    )
                })?;
                env.set
                    .insert("OPENAI_API_KEY".to_string(), api_key.clone());

                if used_workflow_fallback {
                    tracing::info!(
                        terminal_id = %terminal.id,
                        workflow_task_id = %terminal.workflow_task_id,
                        "Using workflow orchestrator API key as Codex terminal fallback"
                    );
                }

                let effective_base_url = terminal.custom_base_url.clone().or(fallback_base_url);

                // Avoid inherited OPENAI_BASE_URL overriding generated provider config.
                // Endpoint selection should come from CODEX_HOME/config.toml only.
                env.unset.push("OPENAI_BASE_URL".to_string());

                // Create isolated CODEX_HOME directory for this terminal
                let codex_home = create_isolated_home(&terminal.id, "codex")?;

                // Get model name
                let model = model_config
                    .api_model_id
                    .clone()
                    .unwrap_or_else(|| model_config.name.clone());

                // Create auth.json with API key (required for non-interactive auth)
                create_codex_auth(&codex_home, &api_key).map_err(|e| {
                    anyhow::anyhow!("Failed to create Codex auth.json for authentication skip: {e}")
                })?;

                // Create config.toml with explicit provider/api_key
                create_codex_config(&codex_home, effective_base_url.as_deref(), &model, &api_key)
                    .map_err(|e| {
                    anyhow::anyhow!("Failed to create Codex config for authentication skip: {e}")
                })?;

                env.set.insert(
                    "CODEX_HOME".to_string(),
                    codex_home.to_string_lossy().to_string(),
                );

                // CLI arguments (higher priority than config files)
                args.push("--model".to_string());
                args.push(model);

                tracing::debug!(
                    terminal_id = %terminal.id,
                    cli = "codex",
                    codex_home = %codex_home.display(),
                    "Built launch config for Codex with authentication skip"
                );
            }
            CcCliType::Gemini => {
                // Gemini requires API key.
                // Prefer terminal-level key, then fallback to workflow orchestrator config.
                // [G20-002/G22-007] Gemini now supports orchestrator API key fallback,
                // matching the pattern used by Claude Code and Codex.
                let custom_api_key = terminal.get_custom_api_key()?;
                let mut fallback_api_key = None;
                let mut fallback_base_url = None;

                if custom_api_key.is_none() {
                    let (fb_base_url, orch_api_key) = self
                        .resolve_workflow_orchestrator_fallback(&terminal.workflow_task_id)
                        .await?;
                    fallback_base_url = fb_base_url;
                    fallback_api_key = orch_api_key;
                    if fallback_api_key.is_some() {
                        tracing::info!(
                            terminal_id = %terminal.id,
                            workflow_task_id = %terminal.workflow_task_id,
                            "Using workflow orchestrator API key as Gemini terminal fallback"
                        );
                    }
                }

                let api_key = custom_api_key.or(fallback_api_key)
                    .ok_or_else(|| anyhow::anyhow!("Gemini requires API key (set terminal.custom_api_key or workflow.orchestrator_config.api_key)"))?;

                // Create isolated Gemini home directory
                let gemini_home = create_isolated_home(&terminal.id, "gemini")?;

                // Get model name
                let model = model_config
                    .api_model_id
                    .clone()
                    .unwrap_or_else(|| model_config.name.clone());

                // G22-003: Propagate .env creation failure instead of silently swallowing it.
                // A missing .env can cause Gemini CLI to fall back to global auth,
                // leading to unexpected billing or auth errors.
                create_gemini_env(
                    &gemini_home,
                    &api_key,
                    terminal.custom_base_url.as_deref(),
                    &model,
                )
                .map_err(|e| {
                    tracing::error!(
                        terminal_id = %terminal.id,
                        error = %e,
                        "Failed to create Gemini .env for authentication skip"
                    );
                    e
                })?;

                // Set GEMINI_HOME to isolated directory (Gemini CLI respects this)
                env.set.insert(
                    "GEMINI_HOME".to_string(),
                    gemini_home.to_string_lossy().to_string(),
                );

                // Handle base URL
                let effective_base_url = terminal.custom_base_url.clone().or(fallback_base_url);
                if let Some(base_url) = &effective_base_url {
                    env.set
                        .insert("GOOGLE_GEMINI_BASE_URL".to_string(), base_url.clone());
                } else {
                    env.unset.push("GOOGLE_GEMINI_BASE_URL".to_string());
                }

                env.set.insert("GEMINI_API_KEY".to_string(), api_key);
                env.set.insert("GEMINI_MODEL".to_string(), model);

                tracing::debug!(
                    terminal_id = %terminal.id,
                    cli = "gemini",
                    gemini_home = %gemini_home.display(),
                    "Built launch config for Gemini with authentication skip"
                );
            }
            _ => {
                // Should not reach here due to earlier check, but handle gracefully
                tracing::warn!(
                    cli_name = %cli_type.name,
                    terminal_id = %terminal.id,
                    "CLI does not support config switching, using empty config"
                );
                return Ok(empty_config());
            }
        }

        // Apply auto-confirm flags if enabled
        apply_auto_confirm_args(&cli, &mut args, auto_confirm);

        tracing::info!(
            terminal_id = %terminal.id,
            cli = %cli_type.name,
            model = %model_config.display_name,
            env_vars_count = env.set.len(),
            args_count = args.len(),
            auto_confirm = auto_confirm,
            "Built launch config for terminal (process isolation)"
        );

        Ok(SpawnCommand {
            command: base_command.to_string(),
            args,
            working_dir: working_dir.to_path_buf(),
            env,
        })
    }

    /// Batch switch models for workflow startup
    ///
    /// Switches model configuration for all terminals in sequence.
    #[deprecated(
        since = "0.2.0",
        note = "Use build_launch_config instead to avoid modifying global config"
    )]
    pub async fn switch_for_terminals(&self, terminals: &[Terminal]) -> anyhow::Result<()> {
        for terminal in terminals {
            #[allow(deprecated)]
            self.switch_for_terminal(terminal).await?;
        }
        Ok(())
    }

    /// Detect CLI installation status
    pub async fn detect_cli(&self, cli_name: &str) -> anyhow::Result<bool> {
        use tokio::process::Command;

        let cli_type = CliType::find_by_name(&self.db.pool, cli_name).await?;

        if let Some(cli) = cli_type {
            let parts: Vec<&str> = cli.detect_command.split_whitespace().collect();
            if parts.is_empty() {
                return Ok(false);
            }

            let result = Command::new(parts[0]).args(&parts[1..]).output().await;

            Ok(result.map(|o| o.status.success()).unwrap_or(false))
        } else {
            Ok(false)
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use chrono::Utc;
    use db::{
        DBService,
        models::{ModelConfig, Terminal},
    };
    use serde_json::Value;
    use sqlx::sqlite::SqlitePoolOptions;
    use tempfile::tempdir;

    use super::*;

    // Test helper to create in-memory database
    async fn setup_test_db() -> Arc<DBService> {
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
        let migrations_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db/migrations");

        sqlx::migrate::Migrator::new(migrations_dir)
            .await
            .unwrap()
            .run(&pool)
            .await
            .unwrap();

        Arc::new(DBService { pool })
    }

    #[tokio::test]
    async fn test_switch_for_terminals_method_exists() {
        let db = setup_test_db().await;
        let service = CCSwitchService::new(db);

        // Verify method exists (compile-time check)
        let terminals: Vec<db::models::Terminal> = vec![];
        #[allow(deprecated)]
        let _ = service.switch_for_terminals(&terminals).await;
    }

    #[tokio::test]
    async fn test_detect_cli_method_exists() {
        let db = setup_test_db().await;
        let service = CCSwitchService::new(db);

        // Verify method exists (compile-time check)
        let _ = service.detect_cli("cursor").await;
    }

    #[test]
    fn test_create_claude_config_updates_primary_api_key_and_preserves_other_fields() {
        let dir = tempdir().expect("failed to create temp dir");
        let claude_home = dir.path();
        std::fs::create_dir_all(claude_home).expect("failed to create claude home");

        let config_path = claude_home.join("config.json");
        std::fs::write(
            &config_path,
            r#"{"foo":"bar","primaryApiKey":"old-key","nested":{"a":1}}"#,
        )
        .expect("failed to seed config.json");

        create_claude_config(claude_home, "new-key").expect("create_claude_config should succeed");

        let updated: Value = serde_json::from_str(
            &std::fs::read_to_string(config_path).expect("failed to read updated config.json"),
        )
        .expect("config.json should be valid JSON");

        assert_eq!(updated["primaryApiKey"], "new-key");
        assert_eq!(updated["foo"], "bar");
        assert_eq!(updated["nested"]["a"], 1);
    }

    #[test]
    fn test_create_claude_settings_writes_expected_env_and_base_url() {
        let dir = tempdir().expect("failed to create temp dir");
        let claude_home = dir.path();
        std::fs::create_dir_all(claude_home).expect("failed to create claude home");

        let settings_path = create_claude_settings(
            claude_home,
            "sk-ant-test",
            Some("https://api.example.com/v1"),
            "claude-sonnet-4-20250514",
        )
        .expect("create_claude_settings should succeed");

        let settings: Value = serde_json::from_str(
            &std::fs::read_to_string(settings_path).expect("failed to read settings.json"),
        )
        .expect("settings.json should be valid JSON");

        assert_eq!(settings["primaryApiKey"], "sk-ant-test");
        // sk- prefix keys use ANTHROPIC_API_KEY only (not AUTH_TOKEN)
        assert_eq!(settings["env"]["ANTHROPIC_API_KEY"], "sk-ant-test");
        assert!(settings["env"]["ANTHROPIC_AUTH_TOKEN"].is_null());
        assert_eq!(
            settings["env"]["ANTHROPIC_MODEL"],
            "claude-sonnet-4-20250514"
        );
        assert_eq!(
            settings["env"]["ANTHROPIC_BASE_URL"],
            "https://api.example.com/v1"
        );
    }

    #[test]
    fn test_looks_like_claude_model() {
        assert!(CCSwitchService::looks_like_claude_model(
            "claude-sonnet-4-20250514"
        ));
        assert!(CCSwitchService::looks_like_claude_model("Claude-Haiku-4-5"));
        assert!(!CCSwitchService::looks_like_claude_model("glm-5"));
    }

    fn make_test_terminal(custom_base_url: Option<&str>) -> Terminal {
        let now = Utc::now();
        Terminal {
            id: "term-test".to_string(),
            workflow_task_id: "task-test".to_string(),
            cli_type_id: "cli-claude-code".to_string(),
            model_config_id: "model-test".to_string(),
            custom_base_url: custom_base_url.map(str::to_string),
            custom_api_key: None,
            role: None,
            role_description: None,
            order_index: 0,
            status: "waiting".to_string(),
            process_id: None,
            pty_session_id: None,
            session_id: None,
            execution_process_id: None,
            vk_session_id: None,
            auto_confirm: true,
            last_commit_hash: None,
            last_commit_message: None,
            started_at: None,
            completed_at: None,
            created_at: now,
            updated_at: now,
        }
    }

    fn make_test_model(model: &str) -> ModelConfig {
        let now = Utc::now();
        ModelConfig {
            id: "model-test".to_string(),
            cli_type_id: "cli-claude-code".to_string(),
            name: "model-test".to_string(),
            display_name: model.to_string(),
            api_model_id: Some(model.to_string()),
            is_default: false,
            is_official: false,
            created_at: now,
            updated_at: now,
        }
    }

    #[tokio::test]
    async fn test_resolve_claude_launch_model_falls_back_for_invalid_official_model() {
        let db = setup_test_db().await;
        let service = CCSwitchService::new(db);
        let terminal = make_test_terminal(None);
        let model_config = make_test_model("glm-5");

        let resolved = service
            .resolve_claude_launch_model(&terminal, &model_config, None)
            .await
            .expect("resolve_claude_launch_model should succeed");

        assert_ne!(resolved, "glm-5");
        assert!(CCSwitchService::looks_like_claude_model(&resolved));
    }

    #[tokio::test]
    async fn test_resolve_claude_launch_model_keeps_custom_endpoint_model() {
        let db = setup_test_db().await;
        let service = CCSwitchService::new(db);
        let terminal = make_test_terminal(Some("https://custom-anthropic-compatible.example"));
        let model_config = make_test_model("glm-5");

        let resolved = service
            .resolve_claude_launch_model(
                &terminal,
                &model_config,
                Some("https://custom-anthropic-compatible.example"),
            )
            .await
            .expect("resolve_claude_launch_model should succeed");

        assert_eq!(resolved, "glm-5");
    }
}
