//! 配置文件路径管理
//!
//! 提供各 CLI 配置文件的路径获取功能。

use std::path::{Path, PathBuf};

use crate::error::{CCSwitchError, Result};

/// 获取用户主目录
pub fn get_home_dir() -> Result<PathBuf> {
    dirs::home_dir().ok_or(CCSwitchError::HomeDirNotFound)
}

// ============================================================================
// Claude Code 配置路径
// ============================================================================

/// 获取 Claude 配置目录
///
/// 默认: ~/.claude
pub fn get_claude_config_dir() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".claude"))
}

/// 获取 Claude settings.json 路径
///
/// 路径: ~/.claude/settings.json
pub fn get_claude_settings_path() -> Result<PathBuf> {
    Ok(get_claude_config_dir()?.join("settings.json"))
}

// ============================================================================
// Codex 配置路径
// ============================================================================

/// 获取 Codex 配置目录
///
/// 默认: ~/.codex
pub fn get_codex_config_dir() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".codex"))
}

/// 获取 Codex auth.json 路径
///
/// 路径: ~/.codex/auth.json
pub fn get_codex_auth_path() -> Result<PathBuf> {
    Ok(get_codex_config_dir()?.join("auth.json"))
}

/// 获取 Codex config.toml 路径
///
/// 路径: ~/.codex/config.toml
pub fn get_codex_config_path() -> Result<PathBuf> {
    Ok(get_codex_config_dir()?.join("config.toml"))
}

// ============================================================================
// Gemini CLI 配置路径
// ============================================================================

/// 获取 Gemini 配置目录
///
/// 默认: ~/.gemini
pub fn get_gemini_config_dir() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".gemini"))
}

/// 获取 Gemini .env 路径
///
/// 路径: ~/.gemini/.env
pub fn get_gemini_env_path() -> Result<PathBuf> {
    Ok(get_gemini_config_dir()?.join(".env"))
}

// ============================================================================
// 通用工具
// ============================================================================

/// 确保目录存在
pub async fn ensure_dir_exists(path: &Path) -> Result<()> {
    if !path.exists() {
        tokio::fs::create_dir_all(path).await?;
    }
    Ok(())
}

/// 确保父目录存在
pub async fn ensure_parent_dir_exists(path: &Path) -> Result<()> {
    if let Some(parent) = path.parent() {
        ensure_dir_exists(parent).await?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_claude_paths() {
        let settings = get_claude_settings_path().unwrap();
        assert!(settings.to_string_lossy().contains(".claude"));
        assert!(settings.to_string_lossy().ends_with("settings.json"));
    }

    #[test]
    fn test_codex_paths() {
        let auth = get_codex_auth_path().unwrap();
        assert!(auth.to_string_lossy().contains(".codex"));
        assert!(auth.to_string_lossy().ends_with("auth.json"));
    }

    #[test]
    fn test_gemini_paths() {
        let env = get_gemini_env_path().unwrap();
        assert!(env.to_string_lossy().contains(".gemini"));
        assert!(env.to_string_lossy().ends_with(".env"));
    }
}
