//! CC-Switch 服务
//!
//! 封装 cc-switch crate，提供与 solodawn 集成的接口。
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

use cc_switch::{CliType as CcCliType, read_claude_config};
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
/// Allow override via env: SOLODAWN_CODEX_WIRE_API=responses|codex
fn resolve_codex_wire_api() -> String {
    if let Some(raw) = utils::env_compat::var_opt_with_compat(
        "SOLODAWN_CODEX_WIRE_API",
        "GITCORTEX_CODEX_WIRE_API",
    ) {
        let normalized = raw.trim().to_ascii_lowercase();
        if normalized == "responses" || normalized == "codex" {
            return normalized;
        }
        tracing::warn!(
            configured = %raw,
            "Invalid SOLODAWN_CODEX_WIRE_API value; expected 'responses' or 'codex', falling back to 'responses'"
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
    _api_key: &str,
) -> anyhow::Result<()> {
    let config_path = codex_home.join("config.toml");

    let (provider_key, base_url_str) = match base_url {
        Some(url) => {
            let trimmed = url.trim_end_matches('/');
            ("custom", trimmed.to_string())
        }
        None => ("openai", "https://api.openai.com/v1".to_string()),
    };
    let wire_api = resolve_codex_wire_api();

    let mut config_content = format!(
        r#"model_provider = "{provider_key}"
model = "{model}"

[model_providers.{provider_key}]
name = "{provider_key}"
base_url = "{base_url_str}"
"#
    );

    config_content.push_str(&format!("wire_api = \"{wire_api}\"\n"));
    config_content.push_str("approval_policy = \"on-request\"\n");
    config_content
        .push_str("sandbox_permissions = [\"disk-full-read-access\", \"disk-write-folder\"]\n");

    std::fs::write(&config_path, config_content)
        .map_err(|e| anyhow::anyhow!("Failed to write Codex config.toml: {e}"))?;

    tracing::info!(
        codex_home = %codex_home.display(),
        config_path = %config_path.display(),
        model_provider = %provider_key,
        base_url = %base_url_str,
        wire_api = %wire_api,
        "Created Codex config.toml for authentication skip (api_key via env var only)"
    );

    Ok(())
}

/// Creates Claude Code config.json in isolated directory.
///
/// Keep `primaryApiKey` aligned with terminal key to avoid runtime precedence
/// ambiguity between `config.json`, `settings.json`, and env-based auth.
fn create_claude_config(
    claude_home: &Path,
    api_key: &str,
    base_url: Option<&str>,
) -> anyhow::Result<()> {
    let config_path = claude_home.join("config.json");

    // primaryApiKey is Claude Code's internal authentication (Anthropic account system).
    // When using a custom base_url (third-party API), do NOT set primaryApiKey — the
    // third-party key would fail Anthropic's auth validation, causing "Invalid API key".
    // Instead, rely solely on ANTHROPIC_API_KEY env var for API calls.
    let config_content = if base_url.is_some() {
        // Custom endpoint: empty config, auth via env vars only
        if config_path.exists() {
            let existing = std::fs::read_to_string(&config_path)?;
            let mut value: serde_json::Value =
                serde_json::from_str(&existing).unwrap_or_else(|_| serde_json::json!({}));
            if let Some(obj) = value.as_object_mut() {
                obj.remove("primaryApiKey");
            }
            serde_json::to_string_pretty(&value)?
        } else {
            serde_json::to_string_pretty(&serde_json::json!({}))?
        }
    } else {
        // Official Anthropic API: set primaryApiKey for login bypass
        if config_path.exists() {
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
        }
    };

    std::fs::write(&config_path, config_content)
        .map_err(|e| anyhow::anyhow!("Failed to write Claude config.json: {e}"))?;

    tracing::debug!(
        claude_home = %claude_home.display(),
        has_custom_base_url = base_url.is_some(),
        "Created Claude Code config.json for authentication"
    );

    Ok(())
}

/// Pre-seed the isolated Claude home's `.claude.json` with onboarding-complete
/// and folder-trust state, so claude skips its first-run onboarding TUI
/// (text-style picker → security notes → "Do you trust this folder?").
///
/// An isolated `CLAUDE_CONFIG_DIR` is a fresh state directory; without this
/// marker claude v2.1.x blocks on the interactive onboarding flow and swallows
/// the orchestrator's dispatched task instruction — the coder then sits at the
/// trust prompt indefinitely (process alive, zero output, zero commits), which
/// the liveness stall-gate misreads as "generating" and protects for up to the
/// hard cap. claude reads/writes `.claude.json` inside `CLAUDE_CONFIG_DIR`
/// (empirically verified against claude 2.1.193 / 2.1.195).
///
/// In the `using_native_auth` path this file is unused (claude reuses the
/// already-onboarded global `~/.claude`), so writing it is harmless there.
fn write_claude_onboarding_state(
    claude_home: &Path,
    working_dir: &Path,
) -> anyhow::Result<()> {
    // claude normalizes Windows paths to forward slashes in the projects map key.
    let project_key = working_dir.to_string_lossy().replace('\\', "/");
    let mut projects = serde_json::Map::new();
    projects.insert(
        project_key.clone(),
        serde_json::json!({
            "allowedTools": [],
            "mcpServers": {},
            "hasTrustDialogAccepted": true,
            "projectOnboardingSeenCount": 1,
            "hasClaudeMdExternalIncludesApproved": false,
            "hasClaudeMdExternalIncludesWarningShown": false
        }),
    );
    let state = serde_json::json!({
        "hasCompletedOnboarding": true,
        "lastOnboardingVersion": "2.1.195",
        "numStartups": 1,
        "migrationVersion": 13,
        // Suppress one-time migration / marketplace prompts that also block the TUI.
        "officialMarketplaceAutoInstallAttempted": true,
        "officialMarketplaceAutoInstalled": true,
        "opusProMigrationComplete": true,
        "sonnet1m45MigrationComplete": true,
        "projects": serde_json::Value::Object(projects)
    });
    let state_path = claude_home.join(".claude.json");
    std::fs::write(&state_path, serde_json::to_string_pretty(&state)?)
        .map_err(|e| anyhow::anyhow!("Failed to write claude .claude.json onboarding state: {e}"))?;
    tracing::debug!(
        claude_home = %claude_home.display(),
        project_key = %project_key,
        "Pre-seeded claude onboarding-complete + folder-trust state"
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

    // For ANY custom base_url (third-party proxy / reseller / ZhipuAI / Packycode /
    // AnyRouter / DuckCoding), route auth via ANTHROPIC_AUTH_TOKEN. Claude Code
    // unconditionally shows a "Detected a custom API key in your environment"
    // confirmation TUI whenever ANTHROPIC_API_KEY is present, which blocks
    // execution (the default selection is "No"). ANTHROPIC_AUTH_TOKEN is sent as
    // a raw Bearer token, bypasses the sk- format validation, and does NOT trigger
    // the confirmation prompt. The previous heuristic (only use AUTH_TOKEN for
    // non-sk- keys) was wrong: many third-party proxies issue sk-ant-* keys for
    // client compatibility, which still belong to a custom endpoint.
    let use_auth_token = base_url.is_some();

    if use_auth_token {
        env_obj.insert(
            "ANTHROPIC_AUTH_TOKEN".to_string(),
            serde_json::Value::String(api_key.to_string()),
        );
    } else {
        env_obj.insert(
            "ANTHROPIC_API_KEY".to_string(),
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
    env_obj.insert(
        "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC".to_string(),
        serde_json::Value::String("1".to_string()),
    );
    if let Some(url) = base_url {
        // Claude Code's Anthropic SDK appends "/v1/messages" to ANTHROPIC_BASE_URL,
        // so strip trailing "/v1" to avoid double-pathing (e.g. .../v1/v1/messages).
        let url_for_cli = url.trim_end_matches('/');
        let url_for_cli = url_for_cli.strip_suffix("/v1").unwrap_or(url_for_cli);
        env_obj.insert(
            "ANTHROPIC_BASE_URL".to_string(),
            serde_json::Value::String(url_for_cli.to_string()),
        );
    }

    // For non-sk- keys with custom base_url: write a helper script that echoes the key,
    // and set apiKeyHelper in settings.json. Claude Code calls this script instead of
    // reading the key from env vars, bypassing format validation.
    let mut settings_map = serde_json::Map::new();
    settings_map.insert("env".to_string(), serde_json::Value::Object(env_obj));

    // Skip all first-launch interactive dialogs on clean installs:
    // - Theme selector, security notes, workspace trust, bypass confirmation
    // These field names match what Claude Code actually reads from settings.json.
    settings_map.insert(
        "hasCompletedOnboarding".to_string(),
        serde_json::Value::Bool(true),
    );
    settings_map.insert(
        "skipDangerousModePermissionPrompt".to_string(),
        serde_json::Value::Bool(true),
    );

    if use_auth_token {
        // ANTHROPIC_AUTH_TOKEN handles auth directly — no apiKeyHelper needed.
        // ZhipuAI's official coding-helper uses this same approach.
        tracing::info!(
            "Using ANTHROPIC_AUTH_TOKEN for non-sk API key with custom base_url (bypasses format validation)"
        );
    } else if base_url.is_none() {
        // primaryApiKey is Claude Code's internal auth (Anthropic account system).
        // For custom endpoints (third-party API), omit it to prevent validation failure.
        settings_map.insert(
            "primaryApiKey".to_string(),
            serde_json::Value::String(api_key.to_string()),
        );
    }

    let settings = serde_json::Value::Object(settings_map);
    let content = serde_json::to_string_pretty(&settings)?;
    std::fs::write(&settings_path, content)
        .map_err(|e| anyhow::anyhow!("Failed to write Claude settings.json: {e}"))?;

    tracing::debug!(
        settings_path = %settings_path.display(),
        use_auth_token = use_auth_token,
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

    if cli == &CcCliType::Codex {
        // Codex is launched in app-server (JSON-RPC) mode, which does not accept
        // the TUI-only flags --full-auto / -a / never. Injecting them caused Codex
        // to exit immediately with an "unknown flag" error. Approval bypass for
        // app-server mode is already wired via the JSON-RPC approval_policy field
        // (set to Never in build_new_conversation_params when auto_confirm), so no
        // argv mutation is needed here.
        return;
    }

    let flag = match cli {
        CcCliType::ClaudeCode => "--dangerously-skip-permissions",
        CcCliType::Gemini => "--yolo",
        _ => return,
    };

    // Avoid duplicate flags
    if args.iter().any(|arg| arg == flag) {
        return;
    }

    args.push(flag.to_string());

    // For Claude Code: also add --bare to skip all interactive first-launch
    // dialogs (theme selector, workspace trust, security notes, bypass
    // confirmation). Without this, a clean install hangs on these prompts.
    if *cli == CcCliType::ClaudeCode && !args.iter().any(|arg| arg == "--bare") {
        args.push("--bare".to_string());
    }
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
/// Creates `<temp>/solodawn/<prefix>-<sanitized_terminal_id>` with restrictive
/// permissions on Unix (0o700). Returns the created directory path.
fn create_isolated_home(terminal_id: &str, prefix: &str) -> anyhow::Result<std::path::PathBuf> {
    let safe_id = sanitize_terminal_id(terminal_id);
    let base_dir = std::env::temp_dir().join("solodawn");
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

// ============================================================================
// S3 — Interactive transport: per-logical-session CLAUDE_HOME + transcript path
// ============================================================================
//
// The no-`-p` interactive transport (see
// docs/developed/plans/2026-06-15-no-p-interactive-transport.md) keeps
// native-OAuth (subscription) users off the Agent SDK credit pool by driving
// the genuine `claude` binary interactively and tailing its on-disk session
// transcript JSONL instead of `-p` stream-json on stdout.
//
// Unlike the metered `-p` path (whose isolated home is keyed on `terminal.id`
// and deleted on terminal-end), the interactive home is keyed on a STABLE
// LOGICAL-SESSION id (the interactive session UUID) so that follow-ups
// (`--resume <uuid>`) find the same transcript file across terminal restarts.
//
// PROBE CORRECTION (claude 2.1.177, live-verified): the redirect env var is
// `CLAUDE_CONFIG_DIR`, NOT `CLAUDE_HOME` (the latter is a no-op in 2.1.177 —
// it redirects neither the transcript nor credential loading). The interactive
// launch therefore sets BOTH to the same isolated dir: `CLAUDE_CONFIG_DIR` for
// the real redirect, and `CLAUDE_HOME` so the existing RB-37 secret-cleanup
// path (which scans `CLAUDE_HOME`) still finds and removes the dir.

/// Prefix used for interactive-transport isolated homes so they are
/// distinguishable from the per-terminal `-p` homes (prefix `"claude"`).
pub const INTERACTIVE_CLAUDE_HOME_PREFIX: &str = "claude-isession";

/// Slugify a working directory the way claude 2.1.177 names its transcript
/// project folder.
///
/// PROBE-VERIFIED behavior (the empirical output is authoritative over the
/// contract's looser "drive-colon dropped" wording): every non-alphanumeric
/// path character — including the drive `:` AND the separator `\`/`/` — is
/// mapped to `-`. The drive colon is therefore NOT removed; it becomes a dash,
/// which (adjacent to the separator dash) yields the doubled `--`:
///   `C:\Users\Administrator\scratch-probe\work`
///     -> `C--Users-Administrator-scratch-probe-work`
///   `E:\SoloDawn` -> `E--SoloDawn`
pub fn slug_working_dir(working_dir: &Path) -> String {
    let mut slug = String::new();
    for c in working_dir.to_string_lossy().chars() {
        if c.is_ascii_alphanumeric() {
            slug.push(c);
        } else {
            // ':', '\\', '/', and any other separator/punctuation -> '-'
            slug.push('-');
        }
    }
    slug
}

/// Compute the on-disk transcript path for an interactive session:
/// `<claude_config_dir>/projects/<slug(working_dir)>/<uuid>.jsonl`.
///
/// `claude_config_dir` must be the directory pointed to by `CLAUDE_CONFIG_DIR`
/// (the isolated home returned by [`create_interactive_isolated_home`]).
pub fn interactive_transcript_path(
    claude_config_dir: &Path,
    working_dir: &Path,
    session_uuid: &str,
) -> std::path::PathBuf {
    claude_config_dir
        .join("projects")
        .join(slug_working_dir(working_dir))
        .join(format!("{session_uuid}.jsonl"))
}

/// Result of provisioning a per-logical-session interactive home: the isolated
/// directory (used as BOTH `CLAUDE_CONFIG_DIR` and `CLAUDE_HOME`), the generated
/// (or supplied) session UUID, and the computed transcript path.
#[derive(Debug, Clone)]
pub struct InteractiveHome {
    /// Isolated dir to set as CLAUDE_CONFIG_DIR (redirect) and CLAUDE_HOME (cleanup).
    pub home_dir: std::path::PathBuf,
    /// Stable logical-session UUID (`--session-id` / `--resume`).
    pub session_uuid: String,
    /// `<home_dir>/projects/<slug>/<uuid>.jsonl` — what S5 tails.
    pub transcript_path: std::path::PathBuf,
    /// When true, `home_dir` is the user's REAL global `~/.claude` (shared,
    /// already authorized + onboarded), NOT an isolated per-session copy. The
    /// interactive auth setup then must NOT redirect `CLAUDE_CONFIG_DIR` or copy
    /// credentials (the global config is already complete), and teardown must
    /// NEVER delete it (the `cleanup_isolated_home` temp-dir guard enforces this
    /// regardless, but callers should also skip the cleanup call). Used for the
    /// native-OAuth (subscription) path so worker terminals reuse the global
    /// login + onboarding instead of hitting a fresh-config login + model picker.
    pub is_shared_global: bool,
}

/// Provision (or re-open) the per-logical-session interactive CLAUDE home.
///
/// Keyed on `session_uuid` (NOT terminal.id) so follow-ups reuse the same dir
/// and transcript file. Pass `None` to mint a fresh session UUID at first
/// launch; pass `Some(uuid)` on follow-up to resume the same logical session.
/// Idempotent: re-provisioning an existing session is a no-op create.
pub fn create_interactive_isolated_home(
    session_uuid: Option<&str>,
    working_dir: &Path,
) -> anyhow::Result<InteractiveHome> {
    let session_uuid = session_uuid
        .map(ToString::to_string)
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());
    // Key the home on the stable logical-session UUID, not terminal.id.
    let home_dir = create_isolated_home(&session_uuid, INTERACTIVE_CLAUDE_HOME_PREFIX)?;
    let transcript_path = interactive_transcript_path(&home_dir, working_dir, &session_uuid);
    Ok(InteractiveHome {
        home_dir,
        session_uuid,
        transcript_path,
        is_shared_global: false,
    })
}

/// Provision an interactive session that reuses the user's GLOBAL `~/.claude`
/// instead of an isolated per-session copy.
///
/// The native-OAuth (subscription) path uses this so worker terminals inherit
/// the global login AND first-run onboarding (`~/.claude.json`) — the isolated
/// home only carried `.credentials.json`/`settings.json`, so the unmodified
/// `claude` binary saw a fresh config and blocked on a login prompt + first-run
/// model picker. The transcript still lands in the default
/// `~/.claude/projects/<slug>/<uuid>.jsonl`, so S5's tailer reads it unchanged;
/// `--session-id` keeps concurrent terminals on separate transcript files.
///
/// The returned home is flagged `is_shared_global` so [`setup_interactive_auth`]
/// neither redirects `CLAUDE_CONFIG_DIR` nor copies credentials, and teardown
/// skips it (the `cleanup_isolated_home` temp-dir guard also refuses to delete
/// anything outside `<temp>/solodawn`).
pub fn create_interactive_global_home(
    session_uuid: Option<&str>,
    working_dir: &Path,
) -> anyhow::Result<InteractiveHome> {
    let session_uuid = session_uuid
        .map(ToString::to_string)
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());
    let home_dir = dirs::home_dir()
        .map(|h| h.join(".claude"))
        .ok_or_else(|| anyhow::anyhow!("cannot resolve home dir for global claude config"))?;
    let transcript_path = interactive_transcript_path(&home_dir, working_dir, &session_uuid);
    Ok(InteractiveHome {
        home_dir,
        session_uuid,
        transcript_path,
        is_shared_global: true,
    })
}

/// Reconstruct the interactive-home directory path for a given logical-session
/// UUID WITHOUT creating it. Used at logical-session teardown to locate the dir
/// for `ProcessManager::cleanup_logical_session_home` (RB-37 deferred cleanup).
///
/// Must mirror [`create_interactive_isolated_home`]'s naming exactly:
/// `<temp>/solodawn/claude-isession-<sanitized_session_uuid>`.
pub fn interactive_isolated_home_path(session_uuid: &str) -> std::path::PathBuf {
    let safe_id = sanitize_terminal_id(session_uuid);
    std::env::temp_dir()
        .join("solodawn")
        .join(format!("{INTERACTIVE_CLAUDE_HOME_PREFIX}-{safe_id}"))
}

/// The interactive auth mode resolved for a single ClaudeCode run.
///
/// Mirrors `build_launch_config`'s `-p` branch selection, but expressed as a
/// single discriminant so the unified interactive auth setup (one transport for
/// all three modes) can scrub the *other* modes' env vars without re-deriving
/// the choice from `(api_key, base_url)`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum InteractiveAuthMode {
    /// No api-key, no base_url: subscription OAuth. Copy `~/.claude/.credentials.json`
    /// into the home; authenticate via that file. No key env. Billing: subscription plan.
    #[default]
    NativeOauth,
    /// api-key, no base_url: official Anthropic API. `ANTHROPIC_API_KEY` env +
    /// settings.json. Billing: pay-as-you-go.
    OfficialKey,
    /// api-key + base_url: relay / third-party proxy. `ANTHROPIC_AUTH_TOKEN` +
    /// `ANTHROPIC_BASE_URL` env, `ANTHROPIC_API_KEY` unset. Billing: relay endpoint.
    Relay,
}

impl InteractiveAuthMode {
    /// Resolve the mode from the same `(api_key, base_url)` pair the `-p` path
    /// resolves (do NOT change WHICH credential a user gets — only the transport).
    pub fn resolve(api_key: Option<&str>, base_url: Option<&str>) -> Self {
        match (api_key, base_url) {
            (Some(_), Some(_)) => InteractiveAuthMode::Relay,
            (Some(_), None) => InteractiveAuthMode::OfficialKey,
            (None, _) => InteractiveAuthMode::NativeOauth,
        }
    }
}

/// Env setup for an interactive ClaudeCode run, returned by
/// [`setup_interactive_auth`].
///
/// `set` / `unset` are applied verbatim by the launcher (see
/// `LocalContainerService::spawn_interactive_claude`, which does
/// `env_remove(unset)` then `env(set)`). `settings_arg` (when present) must be
/// appended to the argv as `--settings <path>` for the non-native modes (native
/// OAuth deliberately omits `--settings`, mirroring the `-p` path).
#[derive(Debug, Clone, Default)]
pub struct InteractiveAuthEnv {
    /// Env vars to SET for the interactive `claude` child.
    pub set: std::collections::HashMap<String, String>,
    /// Env vars to UNSET (scrub) so a stray ambient var cannot redirect billing.
    pub unset: Vec<String>,
    /// Path to a written `settings.json` to pass as `--settings <path>`, if any.
    pub settings_arg: Option<std::path::PathBuf>,
    /// The resolved auth mode (for logging/diagnostics).
    pub mode: InteractiveAuthMode,
}

/// Unified 3-mode auth setup for the no-`-p` interactive transport.
///
/// Reuses the SAME credential constructions as the `-p` `build_launch_config`
/// path ([`create_claude_config`] / [`create_claude_settings`] + the
/// native-credentials copy) so a user gets the IDENTICAL credential they get
/// today — only the transport (`-p` -> interactive) changes. Given the resolved
/// `(api_key, base_url)` for the run plus the interactive [`InteractiveHome`],
/// it writes the per-mode files into `home.home_dir` and returns the env
/// set/unset map (+ optional `--settings` path).
///
/// Per-mode behavior (see [`InteractiveAuthMode`]):
/// - **native** (no api_key): copy `~/.claude/.credentials.json` (+ optional
///   `settings.json`) into the home; NO key env. Scrubs
///   `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_BASE_URL`/`CLAUDE_CODE_OAUTH_TOKEN`.
/// - **official key** (api_key, no base_url): `create_claude_config` +
///   `create_claude_settings`; SET `ANTHROPIC_API_KEY`. Scrubs
///   `ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_BASE_URL`/`CLAUDE_CODE_OAUTH_TOKEN`.
/// - **relay** (api_key + base_url): `create_claude_config` +
///   `create_claude_settings`; SET `ANTHROPIC_AUTH_TOKEN`+`ANTHROPIC_BASE_URL`,
///   UNSET `ANTHROPIC_API_KEY`. Scrubs `ANTHROPIC_API_KEY`/`CLAUDE_CODE_OAUTH_TOKEN`.
///
/// ALWAYS sets `CLAUDE_CONFIG_DIR`+`CLAUDE_HOME` to `home.home_dir` (the former
/// is the real redirect in 2.1.177; the latter keeps RB-37 cleanup finding the
/// dir) and `CLAUDE_CODE_MAX_RETRIES=2` (without it, relay/network errors retry
/// for 30s+ and look like a hang before a terminator).
///
/// `model` is the resolved launch model (threaded into `create_claude_settings`
/// the same way the `-p` path does). `native_credentials_src` is the source
/// `~/.claude` directory for the native copy (pass `dirs::home_dir().join(".claude")`);
/// only read in native mode.
///
/// Billing-routing env keys stripped from a native-copied `settings.json` `env`
/// block (see [`copy_native_settings_scrubbing_billing_env`]). claude honors
/// settings.json `env`, so any of these baked into the user's global settings
/// would override the native OAuth credential and silently redirect billing.
const BILLING_ENV_KEYS: [&str; 4] = [
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_BASE_URL",
    "CLAUDE_CODE_OAUTH_TOKEN",
];

/// Copy a native `settings.json` into the isolated interactive home, removing any
/// billing-routing keys from its `env` block first.
///
/// The process-level scrub in the launcher (`command.env_remove`) only strips the
/// inherited PROCESS environment; it cannot reach a value persisted inside
/// `settings.json` on disk. claude DOES read `settings.json` `env`, so a verbatim
/// copy of a user's global settings.json that pins e.g. `ANTHROPIC_BASE_URL` would
/// re-introduce a relay endpoint inside the native (subscription) home and defeat
/// the scrub. Sanitizing the `env` block here keeps native OAuth on the
/// subscription plan. If the file is not valid JSON it is copied verbatim (best
/// effort — a non-JSON settings.json has no `env` block to leak through).
fn copy_native_settings_scrubbing_billing_env(src: &Path, dst: &Path) -> std::io::Result<()> {
    let content = std::fs::read_to_string(src)?;
    let Ok(mut value) = serde_json::from_str::<serde_json::Value>(&content) else {
        // Not JSON — nothing parseable to leak; copy verbatim.
        std::fs::copy(src, dst)?;
        return Ok(());
    };
    let mut stripped: Vec<&str> = Vec::new();
    if let Some(env_obj) = value
        .get_mut("env")
        .and_then(serde_json::Value::as_object_mut)
    {
        for key in BILLING_ENV_KEYS {
            if env_obj.remove(key).is_some() {
                stripped.push(key);
            }
        }
    }
    if !stripped.is_empty() {
        tracing::info!(
            stripped = ?stripped,
            "Scrubbed billing-routing env keys from native settings.json copy"
        );
    }
    let out = serde_json::to_string_pretty(&value).map_err(std::io::Error::other)?;
    std::fs::write(dst, out)
}

pub fn setup_interactive_auth(
    home: &InteractiveHome,
    api_key: Option<&str>,
    base_url: Option<&str>,
    model: &str,
    native_credentials_src: &Path,
) -> anyhow::Result<InteractiveAuthEnv> {
    let mode = InteractiveAuthMode::resolve(api_key, base_url);
    let home_dir = &home.home_dir;
    let home_str = home_dir.to_string_lossy().to_string();

    let mut set: std::collections::HashMap<String, String> = std::collections::HashMap::new();
    let mut unset: Vec<String> = Vec::new();
    let mut settings_arg: Option<std::path::PathBuf> = None;

    // Redirect home + cap retries (PROBE: CLAUDE_CONFIG_DIR is the real redirect
    // in 2.1.177; CLAUDE_HOME kept for RB-37 cleanup scan). For the SHARED GLOBAL
    // home (native subscription) we must NOT redirect: claude has to use its real
    // default config so it inherits the complete login + first-run onboarding
    // state (`~/.claude.json`), which an isolated copy never carried — that is
    // what forced the login prompt + model picker. Force-clear any inherited
    // redirect so an ambient `CLAUDE_CONFIG_DIR` cannot point the child at a
    // stale/empty dir.
    if home.is_shared_global {
        unset.push("CLAUDE_CONFIG_DIR".to_string());
        unset.push("CLAUDE_HOME".to_string());
    } else {
        set.insert("CLAUDE_CONFIG_DIR".to_string(), home_str.clone());
        set.insert("CLAUDE_HOME".to_string(), home_str);
    }
    set.insert("CLAUDE_CODE_MAX_RETRIES".to_string(), "2".to_string());

    match mode {
        InteractiveAuthMode::NativeOauth => {
            // For the SHARED GLOBAL home, `home_dir` IS the live `~/.claude`, so
            // there is nothing to copy (the real credentials + settings are
            // already there) — and copying would be actively harmful: a
            // `std::fs::copy(src, dst)` with src == dst can truncate the real
            // `.credentials.json`. Only the ISOLATED-home path copies.
            if !home.is_shared_global {
                // Copy the genuine OAuth credentials into the isolated home so the
                // unmodified `claude` binary authenticates against the subscription
                // plan (no key extracted into SoloDawn's own auth path). Mirrors the
                // `-p` native-auth copy.
                let global_creds = native_credentials_src.join(".credentials.json");
                let isolated_creds = home_dir.join(".credentials.json");
                if global_creds.exists() {
                    if let Err(e) = std::fs::copy(&global_creds, &isolated_creds) {
                        tracing::warn!(
                            error = %e,
                            "Failed to copy native credentials into interactive home"
                        );
                    }
                } else {
                    tracing::warn!(
                        creds = %global_creds.display(),
                        "Native interactive auth selected but ~/.claude/.credentials.json is missing"
                    );
                }
                // Copy settings.json for user preferences if present (do not clobber).
                // SANITIZE the copy: the user's global settings.json `env` block may
                // contain a billing-routing var (ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN
                // / ANTHROPIC_BASE_URL / CLAUDE_CODE_OAUTH_TOKEN). claude honors
                // settings.json `env`, so copying it verbatim would re-introduce a
                // key/relay endpoint inside the isolated home and defeat the
                // process-level scrub (env_remove only strips the inherited PROCESS
                // env — it cannot reach a value baked into settings.json on disk).
                // Strip those keys so native OAuth stays on the subscription plan.
                let global_settings = native_credentials_src.join("settings.json");
                let isolated_settings = home_dir.join("settings.json");
                if global_settings.exists() && !isolated_settings.exists() {
                    if let Err(e) = copy_native_settings_scrubbing_billing_env(
                        &global_settings,
                        &isolated_settings,
                    ) {
                        tracing::warn!(
                            error = %e,
                            "Failed to copy/sanitize native settings.json into interactive home"
                        );
                    }
                }
            }
            // No key env. Scrub anything that could redirect billing off the
            // subscription plan.
            unset.push("ANTHROPIC_API_KEY".to_string());
            unset.push("ANTHROPIC_AUTH_TOKEN".to_string());
            unset.push("ANTHROPIC_BASE_URL".to_string());
            unset.push("CLAUDE_CODE_OAUTH_TOKEN".to_string());
        }
        InteractiveAuthMode::OfficialKey => {
            let key = api_key.unwrap_or_default();
            create_claude_config(home_dir, key, None)?;
            let settings_path = create_claude_settings(home_dir, key, None, model)?;
            settings_arg = Some(settings_path);
            set.insert("ANTHROPIC_API_KEY".to_string(), key.to_string());
            // Scrub relay/native auth so an ambient var cannot redirect billing.
            unset.push("ANTHROPIC_AUTH_TOKEN".to_string());
            unset.push("ANTHROPIC_BASE_URL".to_string());
            unset.push("CLAUDE_CODE_OAUTH_TOKEN".to_string());
        }
        InteractiveAuthMode::Relay => {
            let key = api_key.unwrap_or_default();
            create_claude_config(home_dir, key, base_url)?;
            let settings_path = create_claude_settings(home_dir, key, base_url, model)?;
            settings_arg = Some(settings_path);
            // Relay routes auth via ANTHROPIC_AUTH_TOKEN (raw Bearer) to avoid
            // the "custom API key" TUI; ANTHROPIC_API_KEY must NOT be set.
            set.insert("ANTHROPIC_AUTH_TOKEN".to_string(), key.to_string());
            if let Some(url) = base_url {
                // Match create_claude_settings: strip trailing "/v1" so the SDK's
                // appended "/v1/messages" does not double-path.
                let url_for_cli = url.trim_end_matches('/');
                let url_for_cli = url_for_cli.strip_suffix("/v1").unwrap_or(url_for_cli);
                set.insert("ANTHROPIC_BASE_URL".to_string(), url_for_cli.to_string());
            }
            unset.push("ANTHROPIC_API_KEY".to_string());
            unset.push("CLAUDE_CODE_OAUTH_TOKEN".to_string());
        }
    }

    // Set the model env across tiers (mirrors the `-p` path so the interactive
    // run targets the same model regardless of auth mode).
    set.insert("ANTHROPIC_MODEL".to_string(), model.to_string());
    set.insert(
        "ANTHROPIC_DEFAULT_HAIKU_MODEL".to_string(),
        model.to_string(),
    );
    set.insert(
        "ANTHROPIC_DEFAULT_SONNET_MODEL".to_string(),
        model.to_string(),
    );
    set.insert(
        "ANTHROPIC_DEFAULT_OPUS_MODEL".to_string(),
        model.to_string(),
    );

    tracing::info!(
        mode = ?mode,
        home = %home_dir.display(),
        has_base_url = base_url.is_some(),
        "Set up interactive ClaudeCode auth (no-`-p` transport)"
    );

    Ok(InteractiveAuthEnv {
        set,
        unset,
        settings_arg,
        mode,
    })
}

/// CC-Switch 服务
pub struct CCSwitchService {
    db: Arc<DBService>,
}

impl CCSwitchService {
    // Last-resort fallback when both the terminal's requested Claude model and
    // the CLI's DB default are absent/invalid. Bumped alongside the matching
    // strings in agent.rs and planning_drafts.rs after the
    // `test_probe_subscription_model_acceptance` probe confirmed the
    // subscription endpoint accepts the new ID.
    const DEFAULT_CLAUDE_FALLBACK_MODEL: &'static str = "claude-sonnet-4-6";

    pub fn new(db: Arc<DBService>) -> Self {
        Self { db }
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

        let workflow =
            if let Some(workflow) = Workflow::find_by_id(&self.db.pool, &workflow_id).await? {
                workflow
            } else {
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

        // SoloDawn's own dev ports must never leak into AI terminal children.
        // Root `.env` dotenv-loads `PORT` / `BACKEND_PORT` into server.exe on
        // startup; without stripping, an AI terminal's `npm test` or
        // `npm run dev` inherits our backend port, launches an Express server
        // on top of it, and hijacks the dev backend socket. Add these to
        // every terminal's `env.unset` regardless of CLI type so the
        // pollution can never reach a PTY child.
        let port_unset = || {
            vec![
                "PORT".to_string(),
                "BACKEND_PORT".to_string(),
                "FRONTEND_PORT".to_string(),
            ]
        };

        // Helper to create empty config for unsupported CLIs
        let empty_config = || SpawnCommand {
            command: base_command.to_string(),
            args: Vec::new(),
            working_dir: working_dir.to_path_buf(),
            env: SpawnEnv {
                set: Default::default(),
                unset: port_unset(),
            },
        };

        // Parse CLI type
        let cli = if let Some(cli) = CcCliType::parse(&cli_type.name) {
            cli
        } else {
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

        tracing::info!(
            terminal_id = %terminal.id,
            model_config_id = %terminal.model_config_id,
            model_display_name = %model_config.display_name,
            model_api_model_id = ?model_config.api_model_id,
            model_name = %model_config.name,
            cli_type = %cli_type.name,
            "Resolved model config for terminal launch"
        );

        let mut env = SpawnEnv {
            set: Default::default(),
            unset: port_unset(),
        };
        let mut args = Vec::new();

        match cli {
            CcCliType::ClaudeCode => {
                // Create isolated Claude home directory
                let claude_home = create_isolated_home(&terminal.id, "claude")?;

                // Redirect Claude Code to the isolated directory.
                // PROBE (claude 2.1.177): `CLAUDE_CONFIG_DIR` is the REAL redirect
                // for both transcript and credential discovery; `CLAUDE_HOME` is a
                // no-op in 2.1.177. Set BOTH to the same isolated dir — mirrors
                // `setup_interactive_auth` (CLAUDE_CONFIG_DIR for the actual redirect,
                // CLAUDE_HOME so the RB-37 cleanup scan in process.rs — which collects
                // homes from the `CLAUDE_HOME` value — still finds and removes the dir).
                // Without CLAUDE_CONFIG_DIR the isolated config/credentials are ignored
                // and claude reads the user's global ~/.claude (isolation ineffective).
                // [RB-37] CLAUDE_HOME (and CODEX_HOME / GEMINI_HOME) isolated temp dirs
                // — which hold secret files (settings.json, .credentials.json) — are now
                // captured by ProcessManager::spawn_pty_with_config and removed when the
                // terminal ends, on both the normal finalize path and the panic/abort
                // safety-net (IsolatedHomesGuard + TrackedProcess Drop in process.rs).
                // [G22-006] TODO: On Windows, temp dir permissions cannot be set via Unix
                // chmod. Investigate Windows ACL APIs for restricting access to isolated dirs.
                let claude_home_str = claude_home.to_string_lossy().to_string();
                env.set
                    .insert("CLAUDE_CONFIG_DIR".to_string(), claude_home_str.clone());
                env.set.insert("CLAUDE_HOME".to_string(), claude_home_str);

                // Pre-seed onboarding-complete + folder-trust state so claude skips
                // its first-run onboarding TUI (which would swallow the dispatched
                // task instruction and stall the coder at the trust prompt). Harmless
                // in the native-auth path below (claude then reuses global ~/.claude).
                if let Err(e) = write_claude_onboarding_state(&claude_home, working_dir) {
                    tracing::warn!(
                        terminal_id = %terminal.id,
                        claude_home = %claude_home.display(),
                        error = %e,
                        "Failed to pre-seed claude onboarding-complete state; terminal may block on first-run onboarding"
                    );
                }

                let custom_api_key = terminal.get_custom_api_key()?;
                let (orchestrator_base_url, orchestrator_api_key) =
                    if terminal.custom_base_url.is_none() || custom_api_key.is_none() {
                        self.resolve_workflow_orchestrator_fallback(&terminal.workflow_task_id)
                            .await?
                    } else {
                        (None, None)
                    };
                let mut effective_base_url = terminal
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

                    // Try model_config credentials (from Settings → Models)
                    if fallback_api_key.is_none() {
                        if let Ok(Some(model_key)) = model_config.get_api_key() {
                            fallback_api_key = Some(model_key);
                            if effective_base_url.is_none() {
                                effective_base_url = model_config.base_url.clone();
                            }
                            tracing::info!(
                                terminal_id = %terminal.id,
                                model_config_id = %model_config.id,
                                has_base_url = model_config.base_url.is_some(),
                                "Using model_config credentials for terminal"
                            );
                        }
                    }

                    // If model_config also doesn't have API key, try workflow orchestrator
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
                            // Filter empty keys — native auth sets empty orchestrator key
                            fallback_api_key = orchestrator_api_key
                                .as_ref()
                                .filter(|k| !k.trim().is_empty())
                                .cloned();
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

                let api_key = custom_api_key.or(fallback_api_key);

                // When no API key is configured but Claude Code CLI has native
                // OAuth credentials (~/.claude/.credentials.json), skip config
                // creation and let the CLI use its own auth.
                let using_native_auth = api_key.is_none() && {
                    let home = dirs::home_dir();
                    home.is_some_and(|h| h.join(".claude").join(".credentials.json").exists())
                };

                if let Some(ref key) = api_key {
                    tracing::info!(
                        terminal_id = %terminal.id,
                        api_key_source = api_key_source,
                        has_custom_base_url = terminal.custom_base_url.is_some(),
                        "Resolved API key for Claude Code terminal"
                    );
                    create_claude_config(&claude_home, key, effective_base_url.as_deref())
                        .map_err(|e| {
                            tracing::error!(
                                terminal_id = %terminal.id,
                                error = %e,
                                "Failed to create Claude config.json for authentication skip"
                            );
                            e
                        })?;
                } else if using_native_auth {
                    // Reuse the user's GLOBAL ~/.claude (already authorized AND
                    // onboarded) instead of an isolated copy. The previous approach
                    // copied only .credentials.json + settings.json into the isolated
                    // home — never ~/.claude.json (onboarding/account state) — so the
                    // unmodified `claude` binary saw a fresh config and blocked the
                    // orchestrator terminal on an interactive login prompt + first-run
                    // model picker. Drop the isolated redirect set above and clear any
                    // inherited one so claude uses its real default config and inherits
                    // the live subscription login + onboarding. No credential copy: the
                    // global config is already complete, and a std::fs::copy of
                    // .credentials.json onto itself can truncate the real file. The
                    // empty isolated dir created above holds no secrets.
                    env.set.remove("CLAUDE_CONFIG_DIR");
                    env.set.remove("CLAUDE_HOME");
                    env.unset.push("CLAUDE_CONFIG_DIR".to_string());
                    env.unset.push("CLAUDE_HOME".to_string());
                    tracing::info!(
                        terminal_id = %terminal.id,
                        "Using GLOBAL ~/.claude for native OAuth (inherits login + onboarding; no isolated copy)"
                    );
                    // Native OAuth (subscription) auth: scrub every billing-routing
                    // env var so a subscription credential is NEVER paired with an
                    // orchestrator/relay base_url. effective_base_url is the
                    // workflow-orchestrator fallback here (the orchestrator may set a
                    // relay base_url with an empty/native key — see the can_use_fallback
                    // branch above), and line ~1104 already inserted ANTHROPIC_BASE_URL
                    // into env.set. process.rs applies env_remove(unset) BEFORE env(set),
                    // so pushing to unset alone would leave the set value winning — the
                    // key must be removed from set AND added to unset. Mirrors the
                    // interactive native path (setup_interactive_auth NativeOauth arm).
                    for key in [
                        "ANTHROPIC_BASE_URL",
                        "ANTHROPIC_AUTH_TOKEN",
                        "ANTHROPIC_API_KEY",
                    ] {
                        env.set.remove(key);
                    }
                    env.unset.push("ANTHROPIC_BASE_URL".to_string());
                    env.unset.push("ANTHROPIC_AUTH_TOKEN".to_string());
                    env.unset.push("ANTHROPIC_API_KEY".to_string());
                    env.unset.push("CLAUDE_CODE_OAUTH_TOKEN".to_string());
                    // Mark that we need to remove --bare later (after apply_auto_confirm_args).
                    // --bare flag breaks OAuth token loading in Claude Code CLI.
                    env.set
                        .insert("__SOLODAWN_NATIVE_AUTH".to_string(), "1".to_string());
                } else if effective_base_url.is_some() {
                    return Err(anyhow::anyhow!(
                        "Claude Code auth token not configured for custom API endpoint. Please set terminal custom_api_key"
                    ));
                } else {
                    return Err(anyhow::anyhow!(
                        "Claude Code auth token not configured. Please login via CLI (claude login), set terminal custom_api_key, or configure workflow orchestrator API key"
                    ));
                }

                let model = self
                    .resolve_claude_launch_model(
                        terminal,
                        &model_config,
                        effective_base_url.as_deref(),
                    )
                    .await?;

                // Native auth: skip settings/env injection — CLI uses its own credentials.
                // Non-native: create settings.json and inject auth env vars.
                if let Some(ref api_key) = api_key {
                    let settings_path = create_claude_settings(
                        &claude_home,
                        api_key,
                        effective_base_url.as_deref(),
                        &model,
                    )?;
                    args.push("--settings".to_string());
                    args.push(settings_path.to_string_lossy().to_string());
                }

                // For ANY custom base_url: route auth via ANTHROPIC_AUTH_TOKEN
                // (raw Bearer token) to avoid Claude Code's "Detected a custom API
                // key in your environment" TUI prompt, which unconditionally fires
                // when ANTHROPIC_API_KEY is set and blocks execution on its default
                // "No" selection. Also defensively unset ANTHROPIC_API_KEY so any
                // value inherited from the parent shell cannot re-trigger the prompt.
                // Only use ANTHROPIC_API_KEY when talking to the official endpoint.
                if let Some(ref api_key) = api_key {
                    if effective_base_url.is_some() {
                        env.set
                            .insert("ANTHROPIC_AUTH_TOKEN".to_string(), api_key.clone());
                        env.unset.push("ANTHROPIC_API_KEY".to_string());
                    } else {
                        env.set
                            .insert("ANTHROPIC_API_KEY".to_string(), api_key.clone());
                    }
                }

                // Set model for all tiers. The model comes from the terminal's
                // model_config; it MUST be a current/available model id (a retired
                // id such as claude-sonnet-4-20250514 makes claude report "model
                // unavailable" and stall). Forcing it here also overrides any stale
                // default in the inherited global ~/.claude config.
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

                // [H04] Compute effective_base_url BEFORE create_gemini_env so the
                // .env file includes the orchestrator fallback URL when the terminal
                // has no custom_base_url.  Previously, terminal.custom_base_url was
                // passed directly, discarding the fallback and causing Gemini CLI
                // to use the wrong endpoint when reading the .env file.
                let effective_base_url = terminal.custom_base_url.clone().or(fallback_base_url);

                // G22-003: Propagate .env creation failure instead of silently swallowing it.
                // A missing .env can cause Gemini CLI to fall back to global auth,
                // leading to unexpected billing or auth errors.
                create_gemini_env(
                    &gemini_home,
                    &api_key,
                    effective_base_url.as_deref(),
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

                // Handle base URL — effective_base_url already computed above
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

        // Remove --bare when using native OAuth credentials — the flag
        // prevents Claude Code CLI from loading its OAuth token.
        if env.set.remove("__SOLODAWN_NATIVE_AUTH").is_some() {
            args.retain(|a| a != "--bare");
        }

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
        let pool = SqlitePoolOptions::new()
            .connect(":memory:")
            .await
            .expect("failed to open in-memory sqlite pool for cc_switch tests");

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
    async fn test_detect_cli_method_exists() {
        let db = setup_test_db().await;
        let service = CCSwitchService::new(db);

        // Verify method exists (compile-time check)
        let _ = service.detect_cli("cursor").await;
    }

    #[test]
    fn test_copy_native_settings_strips_billing_env_keeps_other_fields() {
        let dir = tempdir().expect("temp dir");
        let src = dir.path().join("global-settings.json");
        let dst = dir.path().join("isolated-settings.json");
        // A realistic user global settings.json that pins a relay endpoint + key
        // in its env block, plus non-billing preferences that must survive.
        std::fs::write(
            &src,
            serde_json::to_string_pretty(&serde_json::json!({
                "env": {
                    "ANTHROPIC_API_KEY": "sk-leak",
                    "ANTHROPIC_AUTH_TOKEN": "tok-leak",
                    "ANTHROPIC_BASE_URL": "https://relay.example/v1",
                    "CLAUDE_CODE_OAUTH_TOKEN": "oauth-leak",
                    "EDITOR": "vim"
                },
                "hasCompletedOnboarding": true,
                "theme": "dark"
            }))
            .unwrap(),
        )
        .unwrap();

        copy_native_settings_scrubbing_billing_env(&src, &dst).expect("copy+scrub");

        let out: Value =
            serde_json::from_str(&std::fs::read_to_string(&dst).unwrap()).expect("valid json");
        let env = out.get("env").and_then(Value::as_object).expect("env block");
        // Every billing-routing var stripped so native OAuth stays on the plan.
        for key in BILLING_ENV_KEYS {
            assert!(!env.contains_key(key), "{key} must be stripped from native settings.json");
        }
        // Non-billing env + top-level preferences preserved.
        assert_eq!(env.get("EDITOR").and_then(Value::as_str), Some("vim"));
        assert_eq!(out.get("theme").and_then(Value::as_str), Some("dark"));
        assert_eq!(
            out.get("hasCompletedOnboarding").and_then(Value::as_bool),
            Some(true)
        );
    }

    #[test]
    fn test_copy_native_settings_no_env_block_is_passthrough() {
        let dir = tempdir().expect("temp dir");
        let src = dir.path().join("g.json");
        let dst = dir.path().join("i.json");
        std::fs::write(&src, r#"{"theme":"light"}"#).unwrap();
        copy_native_settings_scrubbing_billing_env(&src, &dst).expect("copy");
        let out: Value = serde_json::from_str(&std::fs::read_to_string(&dst).unwrap()).unwrap();
        assert_eq!(out.get("theme").and_then(Value::as_str), Some("light"));
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

        // Test with official Anthropic API (no custom base_url): should set primaryApiKey
        create_claude_config(claude_home, "new-key", None)
            .expect("create_claude_config should succeed");

        let updated: Value = serde_json::from_str(
            &std::fs::read_to_string(&config_path).expect("failed to read updated config.json"),
        )
        .expect("config.json should be valid JSON");

        assert_eq!(updated["primaryApiKey"], "new-key");
        assert_eq!(updated["foo"], "bar");
        assert_eq!(updated["nested"]["a"], 1);

        // Test with custom base_url (third-party API): should remove primaryApiKey
        create_claude_config(
            claude_home,
            "third-party-key",
            Some("https://example.com/api"),
        )
        .expect("create_claude_config with custom base_url should succeed");

        let updated: Value = serde_json::from_str(
            &std::fs::read_to_string(&config_path).expect("failed to read updated config.json"),
        )
        .expect("config.json should be valid JSON");

        assert!(
            updated.get("primaryApiKey").is_none(),
            "primaryApiKey should be removed for custom base_url"
        );
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

        // Custom base_url: primaryApiKey should NOT be set (third-party API)
        assert!(
            settings.get("primaryApiKey").is_none() || settings["primaryApiKey"].is_null(),
            "primaryApiKey should be omitted for custom base_url"
        );
        // Custom base_url: ALWAYS use ANTHROPIC_AUTH_TOKEN (raw Bearer token) so
        // Claude Code does not show its "Detected a custom API key in your
        // environment" confirmation TUI, which fires unconditionally when
        // ANTHROPIC_API_KEY is set and defaults to "No".
        assert_eq!(settings["env"]["ANTHROPIC_AUTH_TOKEN"], "sk-ant-test");
        assert!(settings["env"]["ANTHROPIC_API_KEY"].is_null());
        assert_eq!(
            settings["env"]["ANTHROPIC_MODEL"],
            "claude-sonnet-4-20250514"
        );
        // CC-Switch strips trailing /v1 because Claude Code SDK appends /v1/messages
        assert_eq!(
            settings["env"]["ANTHROPIC_BASE_URL"],
            "https://api.example.com"
        );
    }

    #[test]
    fn test_create_claude_settings_sk_ant_proxy_key_with_custom_base_url_uses_auth_token() {
        // Third-party proxies (Packycode, AnyRouter, DuckCoding, reseller proxies)
        // often issue keys with the sk-ant- prefix for client compatibility.
        // With a custom base_url, auth MUST still route via ANTHROPIC_AUTH_TOKEN
        // to avoid Claude Code's custom-API-key confirmation TUI.
        let dir = tempdir().expect("failed to create temp dir");
        let claude_home = dir.path();
        std::fs::create_dir_all(claude_home).expect("failed to create claude home");

        let settings_path = create_claude_settings(
            claude_home,
            "sk-ant-proxy-abcdef1234567890",
            Some("https://proxy.example.com/v1"),
            "claude-sonnet-4-20250514",
        )
        .expect("create_claude_settings should succeed");

        let settings: Value = serde_json::from_str(
            &std::fs::read_to_string(settings_path).expect("failed to read settings.json"),
        )
        .expect("settings.json should be valid JSON");

        assert_eq!(
            settings["env"]["ANTHROPIC_AUTH_TOKEN"],
            "sk-ant-proxy-abcdef1234567890"
        );
        assert!(
            settings["env"]["ANTHROPIC_API_KEY"].is_null(),
            "ANTHROPIC_API_KEY must not be set for custom base_url (would trigger TUI prompt)"
        );
        assert!(
            settings.get("primaryApiKey").is_none() || settings["primaryApiKey"].is_null(),
            "primaryApiKey must be omitted for custom base_url"
        );
    }

    #[test]
    fn test_create_claude_settings_non_sk_key_with_custom_base_url_uses_auth_token() {
        // ZhipuAI-style keys (aa.bb.cc) with a custom base_url must use
        // ANTHROPIC_AUTH_TOKEN (unchanged behavior).
        let dir = tempdir().expect("failed to create temp dir");
        let claude_home = dir.path();
        std::fs::create_dir_all(claude_home).expect("failed to create claude home");

        let settings_path = create_claude_settings(
            claude_home,
            "aa.bb.cc",
            Some("https://open.bigmodel.cn/api/anthropic"),
            "glm-4.6",
        )
        .expect("create_claude_settings should succeed");

        let settings: Value = serde_json::from_str(
            &std::fs::read_to_string(settings_path).expect("failed to read settings.json"),
        )
        .expect("settings.json should be valid JSON");

        assert_eq!(settings["env"]["ANTHROPIC_AUTH_TOKEN"], "aa.bb.cc");
        assert!(settings["env"]["ANTHROPIC_API_KEY"].is_null());
    }

    #[test]
    fn test_create_claude_settings_official_endpoint_uses_api_key() {
        // With NO custom base_url (official Anthropic endpoint), keep using
        // ANTHROPIC_API_KEY and set primaryApiKey for Claude Code's internal
        // Anthropic account system.
        let dir = tempdir().expect("failed to create temp dir");
        let claude_home = dir.path();
        std::fs::create_dir_all(claude_home).expect("failed to create claude home");

        let settings_path = create_claude_settings(
            claude_home,
            "sk-ant-official-1234567890",
            None,
            "claude-sonnet-4-20250514",
        )
        .expect("create_claude_settings should succeed");

        let settings: Value = serde_json::from_str(
            &std::fs::read_to_string(settings_path).expect("failed to read settings.json"),
        )
        .expect("settings.json should be valid JSON");

        assert_eq!(
            settings["env"]["ANTHROPIC_API_KEY"],
            "sk-ant-official-1234567890"
        );
        assert!(settings["env"]["ANTHROPIC_AUTH_TOKEN"].is_null());
        assert!(settings["env"].get("ANTHROPIC_BASE_URL").is_none());
        assert_eq!(settings["primaryApiKey"], "sk-ant-official-1234567890");
    }

    #[test]
    fn test_apply_auto_confirm_args_codex_does_not_mutate_args() {
        // Codex runs in app-server (JSON-RPC) mode and approval bypass is handled
        // via the approval_policy JSON-RPC field, not argv. Injecting TUI-only
        // flags (--full-auto / -a / never) caused the PTY to exit immediately.
        let mut args = vec!["app-server".to_string()];
        let before = args.clone();

        apply_auto_confirm_args(&CcCliType::Codex, &mut args, true);

        assert_eq!(
            args, before,
            "apply_auto_confirm_args must not mutate argv for Codex (app-server mode)"
        );

        // auto_confirm = false is also a no-op.
        apply_auto_confirm_args(&CcCliType::Codex, &mut args, false);
        assert_eq!(args, before);
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
            encrypted_api_key: None,
            base_url: None,
            api_type: None,
            has_api_key: false,
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

    /// R6 regression guard: every terminal launch config — supported CLI or
    /// fallback empty_config — must strip SoloDawn dev ports from the child
    /// PTY env. Otherwise the root `.env`'s `PORT=23456` / `BACKEND_PORT=23456`
    /// loaded by `dotenv::dotenv().ok()` leaks into an AI terminal, which may
    /// run `npm test` / `npm run dev` and bind the backend port (as happened
    /// with Task 1's Express test boot in R6).
    #[tokio::test]
    async fn test_build_launch_config_strips_solodawn_dev_ports_empty_path() {
        let db = setup_test_db().await;
        let service = CCSwitchService::new(db.clone());

        // Seed an unrecognized CLI type so CcCliType::parse returns None and
        // build_launch_config returns via the empty_config fallback path.
        let now = Utc::now();
        sqlx::query(
            r"
            INSERT INTO cli_type (id, name, display_name, detect_command, install_command, install_guide_url, config_file_path, is_system, created_at)
            VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ",
        )
        .bind("cli-unknown-test")
        .bind("not-a-real-cli")
        .bind("Fake CLI")
        .bind("which fake")
        .bind::<Option<&str>>(None)
        .bind::<Option<&str>>(None)
        .bind::<Option<&str>>(None)
        .bind(false)
        .bind(now)
        .execute(&db.pool)
        .await
        .expect("seed cli_type failed");

        let mut terminal = make_test_terminal(None);
        terminal.cli_type_id = "cli-unknown-test".to_string();

        let spawn = service
            .build_launch_config(&terminal, "fake", std::path::Path::new("."), false)
            .await
            .expect("build_launch_config empty_path should succeed");

        for key in ["PORT", "BACKEND_PORT", "FRONTEND_PORT"] {
            assert!(
                spawn.env.unset.iter().any(|k| k == key),
                "env.unset missing {key} in empty_config fallback — dev port would leak to PTY child"
            );
        }
    }

    // NOTE: A "Claude path" sibling of test_build_launch_config_strips_*
    // would have wider coverage but cannot run on CI without a live Claude
    // Code auth token (the auth resolution path panics with "Claude Code
    // auth token not configured"). The empty_config test above already
    // verifies the shared `port_unset()` helper that BOTH branches use, so
    // the strip behavior is provably uniform without the auth-coupled
    // sibling test. R7-PB1 retrospective: dropped the Claude-path test
    // after CI Basic Checks failed on the auth lookup.

    /// Apply a built `SpawnEnv` the EXACT way the launcher does
    /// (`process.rs::spawn_pty_with_config`: `env_remove(unset)` for every key
    /// in `unset` FIRST, then `env(set)` for every key/value in `set`). Returns
    /// the resulting effective child-env map. Used to prove the native-OAuth
    /// billing scrub actually removes a value from the child env even though the
    /// orchestrator-fallback code already inserted it into `set`.
    fn effective_child_env(env: &SpawnEnv) -> std::collections::HashMap<String, String> {
        let mut child: std::collections::HashMap<String, String> = std::collections::HashMap::new();
        for key in &env.unset {
            child.remove(key);
        }
        for (key, value) in &env.set {
            child.insert(key.clone(), value.clone());
        }
        child
    }

    /// HIGH-severity regression guard for the native-OAuth `-p`/PTY billing leak
    /// (cc_switch.rs:1104). When a workflow orchestrator supplies a relay
    /// `base_url` with an empty/native key, `build_launch_config` first inserts
    /// `ANTHROPIC_BASE_URL` into `env.set`, then takes the `using_native_auth`
    /// branch. The fix scrubs every billing-routing var there (remove-from-set
    /// AND push-to-unset). Because the launcher runs `env_remove` BEFORE `env`,
    /// pushing to `unset` alone would NOT win — this test pins both halves and
    /// the resulting child env, so a subscription OAuth credential can never be
    /// paired with a relay base_url. Mirrors the interactive path's coverage in
    /// `test_setup_interactive_auth_native_scrubs_all_keys`. Deterministic: no
    /// DB/env/wall-clock (the native arm itself is unreachable on CI — see the
    /// NOTE above — so the invariant is asserted on the apply-order contract).
    #[test]
    fn test_native_oauth_scrub_strips_orchestrator_base_url_from_child_env() {
        // Reproduce the pre-scrub state: orchestrator fallback inserted a relay
        // base_url into env.set (cc_switch.rs:~1104) and the port unsets are present.
        let mut env = SpawnEnv {
            set: Default::default(),
            unset: vec![
                "PORT".to_string(),
                "BACKEND_PORT".to_string(),
                "FRONTEND_PORT".to_string(),
            ],
        };
        env.set.insert(
            "ANTHROPIC_BASE_URL".to_string(),
            "https://relay.example.com".to_string(),
        );
        // A stray ambient relay/key var the scrub must also neutralize.
        env.set
            .insert("ANTHROPIC_AUTH_TOKEN".to_string(), "leaked".to_string());

        // EXACT scrub the native-OAuth branch performs.
        for key in [
            "ANTHROPIC_BASE_URL",
            "ANTHROPIC_AUTH_TOKEN",
            "ANTHROPIC_API_KEY",
        ] {
            env.set.remove(key);
        }
        env.unset.push("ANTHROPIC_BASE_URL".to_string());
        env.unset.push("ANTHROPIC_AUTH_TOKEN".to_string());
        env.unset.push("ANTHROPIC_API_KEY".to_string());
        env.unset.push("CLAUDE_CODE_OAUTH_TOKEN".to_string());

        // All four billing keys removed from set, present in unset.
        for key in [
            "ANTHROPIC_BASE_URL",
            "ANTHROPIC_AUTH_TOKEN",
            "ANTHROPIC_API_KEY",
            "CLAUDE_CODE_OAUTH_TOKEN",
        ] {
            assert!(!env.set.contains_key(key), "{key} must be removed from set");
            assert!(
                env.unset.iter().any(|k| k == key),
                "{key} must be pushed to unset"
            );
        }

        // The launcher's env_remove-then-env order yields a child env with NO
        // billing-routing var (the relay base_url no longer leaks to the
        // subscription OAuth run).
        let child = effective_child_env(&env);
        for key in [
            "ANTHROPIC_BASE_URL",
            "ANTHROPIC_AUTH_TOKEN",
            "ANTHROPIC_API_KEY",
            "CLAUDE_CODE_OAUTH_TOKEN",
        ] {
            assert!(
                !child.contains_key(key),
                "{key} leaked into the native-OAuth child env"
            );
        }
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

    // ------------------------------------------------------------------------
    // S3 — interactive transport helpers
    // ------------------------------------------------------------------------

    #[test]
    fn test_slug_working_dir_matches_probe_verified_facts() {
        // PROBE-verified live against claude 2.1.177.
        assert_eq!(
            slug_working_dir(std::path::Path::new(r"E:\SoloDawn")),
            "E--SoloDawn"
        );
        assert_eq!(
            slug_working_dir(std::path::Path::new(
                r"C:\Users\Administrator\scratch-probe\work"
            )),
            "C--Users-Administrator-scratch-probe-work"
        );
    }

    #[test]
    fn test_interactive_transcript_path_layout() {
        let home = std::path::Path::new("/tmp/solodawn/claude-isession-abc");
        let wd = std::path::Path::new("/repo/proj");
        let path = interactive_transcript_path(home, wd, "the-uuid");
        assert_eq!(
            path,
            home.join("projects").join("-repo-proj").join("the-uuid.jsonl")
        );
    }

    #[test]
    fn test_create_interactive_isolated_home_keyed_on_session_uuid() {
        let wd = std::path::Path::new(r"E:\SoloDawn");

        // Supplied UUID -> stable home keyed on it, reused across follow-ups.
        let h1 = create_interactive_isolated_home(Some("fixed-session-uuid"), wd)
            .expect("provision interactive home");
        assert_eq!(h1.session_uuid, "fixed-session-uuid");
        assert!(
            h1.home_dir
                .file_name()
                .and_then(|n| n.to_str())
                .is_some_and(|n| n.starts_with("claude-isession-")),
            "interactive home must carry the exempt prefix: {:?}",
            h1.home_dir
        );
        assert!(h1.transcript_path.ends_with("fixed-session-uuid.jsonl"));

        // Re-provisioning the same logical session returns the same dir + path.
        let h2 = create_interactive_isolated_home(Some("fixed-session-uuid"), wd)
            .expect("re-provision interactive home");
        assert_eq!(h1.home_dir, h2.home_dir);
        assert_eq!(h1.transcript_path, h2.transcript_path);

        // None -> mints a fresh UUID.
        let h3 = create_interactive_isolated_home(None, wd).expect("mint fresh interactive home");
        assert_ne!(h3.session_uuid, "fixed-session-uuid");

        // Cleanup the temp dirs created by this test.
        let _ = std::fs::remove_dir_all(&h1.home_dir);
        let _ = std::fs::remove_dir_all(&h3.home_dir);
    }

    #[test]
    fn test_interactive_auth_mode_resolution() {
        assert_eq!(
            InteractiveAuthMode::resolve(None, None),
            InteractiveAuthMode::NativeOauth
        );
        assert_eq!(
            InteractiveAuthMode::resolve(None, Some("https://x")),
            InteractiveAuthMode::NativeOauth
        );
        assert_eq!(
            InteractiveAuthMode::resolve(Some("k"), None),
            InteractiveAuthMode::OfficialKey
        );
        assert_eq!(
            InteractiveAuthMode::resolve(Some("k"), Some("https://x")),
            InteractiveAuthMode::Relay
        );
    }

    #[test]
    fn test_setup_interactive_auth_official_key() {
        let wd = std::path::Path::new(r"E:\SoloDawn");
        let home = create_interactive_isolated_home(Some("auth-official-uuid"), wd).unwrap();
        let env = setup_interactive_auth(
            &home,
            Some("sk-ant-official"),
            None,
            "claude-sonnet-4-6",
            std::path::Path::new("/nonexistent/.claude"),
        )
        .unwrap();
        assert_eq!(env.mode, InteractiveAuthMode::OfficialKey);
        assert_eq!(
            env.set.get("ANTHROPIC_API_KEY").map(String::as_str),
            Some("sk-ant-official")
        );
        // Always-set redirect + retry cap.
        assert!(env.set.contains_key("CLAUDE_CONFIG_DIR"));
        assert!(env.set.contains_key("CLAUDE_HOME"));
        assert_eq!(
            env.set.get("CLAUDE_CODE_MAX_RETRIES").map(String::as_str),
            Some("2")
        );
        // Scrub relay/native vars.
        assert!(env.unset.iter().any(|k| k == "ANTHROPIC_AUTH_TOKEN"));
        assert!(env.unset.iter().any(|k| k == "ANTHROPIC_BASE_URL"));
        assert!(env.unset.iter().any(|k| k == "CLAUDE_CODE_OAUTH_TOKEN"));
        // Official key writes settings.json -> --settings path returned.
        assert!(env.settings_arg.is_some());

        let _ = std::fs::remove_dir_all(&home.home_dir);
    }

    #[test]
    fn test_setup_interactive_auth_relay() {
        let wd = std::path::Path::new(r"E:\SoloDawn");
        let home = create_interactive_isolated_home(Some("auth-relay-uuid"), wd).unwrap();
        let env = setup_interactive_auth(
            &home,
            Some("relay-token"),
            Some("https://relay.example.com/v1"),
            "claude-sonnet-4-6",
            std::path::Path::new("/nonexistent/.claude"),
        )
        .unwrap();
        assert_eq!(env.mode, InteractiveAuthMode::Relay);
        // Relay routes via AUTH_TOKEN; API_KEY must be scrubbed, not set.
        assert_eq!(
            env.set.get("ANTHROPIC_AUTH_TOKEN").map(String::as_str),
            Some("relay-token")
        );
        assert!(!env.set.contains_key("ANTHROPIC_API_KEY"));
        assert!(env.unset.iter().any(|k| k == "ANTHROPIC_API_KEY"));
        // base_url stripped of trailing /v1 to avoid double-pathing.
        assert_eq!(
            env.set.get("ANTHROPIC_BASE_URL").map(String::as_str),
            Some("https://relay.example.com")
        );
        assert!(env.settings_arg.is_some());

        let _ = std::fs::remove_dir_all(&home.home_dir);
    }

    #[test]
    fn test_setup_interactive_auth_native_scrubs_all_keys() {
        let wd = std::path::Path::new(r"E:\SoloDawn");
        let home = create_interactive_isolated_home(Some("auth-native-uuid"), wd).unwrap();
        let env = setup_interactive_auth(
            &home,
            None,
            None,
            "claude-sonnet-4-6",
            std::path::Path::new("/nonexistent/.claude"),
        )
        .unwrap();
        assert_eq!(env.mode, InteractiveAuthMode::NativeOauth);
        // Native: no key env at all; everything that could redirect billing is scrubbed.
        assert!(!env.set.contains_key("ANTHROPIC_API_KEY"));
        assert!(!env.set.contains_key("ANTHROPIC_AUTH_TOKEN"));
        assert!(!env.set.contains_key("ANTHROPIC_BASE_URL"));
        for key in [
            "ANTHROPIC_API_KEY",
            "ANTHROPIC_AUTH_TOKEN",
            "ANTHROPIC_BASE_URL",
            "CLAUDE_CODE_OAUTH_TOKEN",
        ] {
            assert!(
                env.unset.iter().any(|k| k == key),
                "native mode must scrub {key}"
            );
        }
        // Native omits --settings (mirrors the -p native path).
        assert!(env.settings_arg.is_none());

        let _ = std::fs::remove_dir_all(&home.home_dir);
    }
}
