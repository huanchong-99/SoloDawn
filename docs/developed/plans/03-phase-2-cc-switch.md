# Phase 2: CC-Switch 核心提取与集成

> **状态:** ⬜ 未开始
> **进度追踪:** 查看 `TODO.md`
> **前置条件:** Phase 1 完成

## 概述

从 CC-Switch 项目中提取核心代码，创建独立的 crate 并集成到 Vibe Kanban。

---

### Task 2.1: 分析 CC-Switch 核心代码

**状态:** ⬜ 未开始

**前置条件:**
- Phase 1 已完成
- 已阅读 CC-Switch 源码

**目标:**
分析 CC-Switch 中需要提取的核心代码，确定依赖关系和修改点。

**涉及文件（只读分析）:**
- `cc-switch-main/src-tauri/src/provider.rs` - Provider 数据模型
- `cc-switch-main/src-tauri/src/config.rs` - Claude 配置读写
- `cc-switch-main/src-tauri/src/codex_config.rs` - Codex 配置读写
- `cc-switch-main/src-tauri/src/gemini_config.rs` - Gemini 配置读写
- `cc-switch-main/src-tauri/src/services/provider/mod.rs` - 供应商服务
- `cc-switch-main/src-tauri/src/services/provider/live.rs` - Live 配置写入

---

**Step 2.1.1: 核心代码依赖分析**

需要提取的核心功能：

| 功能 | 源文件 | Tauri 依赖 | 可独立使用 |
|------|--------|------------|------------|
| 配置文件路径 | config.rs | 无 | ✅ |
| Claude 配置读写 | config.rs | 无 | ✅ |
| Codex 配置读写 | codex_config.rs | 无 | ✅ |
| Gemini 配置读写 | gemini_config.rs | 无 | ✅ |
| 原子写入 | config.rs | 无 | ✅ |
| Provider 模型 | provider.rs | 无 | ✅ |
| Live 配置写入 | services/provider/live.rs | 部分 | ⚠️ 需修改 |
| 供应商切换 | services/provider/mod.rs | 是 | ❌ 需重写 |

**需要移除的 Tauri 依赖:**
- `tauri::State<'_, AppState>` → 改为函数参数
- `#[tauri::command]` → 移除
- `tauri-plugin-store` → 使用文件存储替代

---

**交付物:**
- 本任务为分析任务，无代码交付
- 输出：依赖分析表（如上）

**验收标准:**
1. 明确了需要提取的文件列表
2. 明确了需要修改的依赖点
3. 明确了提取后的模块结构

---

### Task 2.2: 创建 cc-switch crate

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.1 分析完成

**目标:**
在 vibe-kanban workspace 中创建独立的 cc-switch crate，包含配置切换核心逻辑。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/cc-switch/Cargo.toml`
- 创建: `vibe-kanban-main/crates/cc-switch/src/lib.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/error.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/config_path.rs`
- 修改: `vibe-kanban-main/Cargo.toml` (添加 workspace member)

---

**Step 2.2.1: 创建 Cargo.toml**

文件路径: `vibe-kanban-main/crates/cc-switch/Cargo.toml`

```toml
[package]
name = "cc-switch"
version = "0.1.0"
edition = "2021"
description = "CLI configuration switching core for SoloDawn"
authors = ["SoloDawn Contributors"]
license = "Apache-2.0"

[dependencies]
# 序列化
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"

# 异步运行时
tokio = { version = "1.0", features = ["fs", "process"] }

# 错误处理
anyhow = "1.0"
thiserror = "2.0"

# 文件系统
dirs = "5.0"
tempfile = "3"

# TOML 解析 (Codex 配置)
toml = "0.8"

# 日志
tracing = "0.1"
```

---

**Step 2.2.2: 创建 error.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/error.rs`

```rust
//! CC-Switch 错误类型

use thiserror::Error;

/// CC-Switch 错误
#[derive(Error, Debug)]
pub enum CCSwitchError {
    #[error("Configuration file not found: {path}")]
    ConfigNotFound { path: String },

    #[error("Failed to read configuration: {0}")]
    ReadError(#[from] std::io::Error),

    #[error("Failed to parse JSON: {0}")]
    JsonParseError(#[from] serde_json::Error),

    #[error("Failed to parse TOML: {0}")]
    TomlParseError(#[from] toml::de::Error),

    #[error("Invalid configuration: {message}")]
    InvalidConfig { message: String },

    #[error("CLI not supported: {cli_name}")]
    UnsupportedCli { cli_name: String },

    #[error("Atomic write failed: {0}")]
    AtomicWriteError(String),

    #[error("Home directory not found")]
    HomeDirNotFound,
}

pub type Result<T> = std::result::Result<T, CCSwitchError>;
```

---

**Step 2.2.3: 创建 config_path.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/config_path.rs`

```rust
//! 配置文件路径管理
//!
//! 提供各 CLI 配置文件的路径获取功能。

use std::path::PathBuf;
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

/// 获取 Claude MCP 配置路径
///
/// 路径: ~/.claude.json (注意：不是 ~/.claude/claude.json)
pub fn get_claude_mcp_path() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".claude.json"))
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

/// 获取 Gemini settings.json 路径
///
/// 路径: ~/.gemini/settings.json
pub fn get_gemini_settings_path() -> Result<PathBuf> {
    Ok(get_gemini_config_dir()?.join("settings.json"))
}

// ============================================================================
// 通用工具
// ============================================================================

/// 确保目录存在
pub async fn ensure_dir_exists(path: &PathBuf) -> Result<()> {
    if !path.exists() {
        tokio::fs::create_dir_all(path).await?;
    }
    Ok(())
}

/// 确保父目录存在
pub async fn ensure_parent_dir_exists(path: &PathBuf) -> Result<()> {
    if let Some(parent) = path.parent() {
        ensure_dir_exists(&parent.to_path_buf()).await?;
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
```

---

**Step 2.2.4: 创建 lib.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/lib.rs`

```rust
//! CC-Switch Core
//!
//! CLI 配置切换核心库，支持 Claude Code、Codex、Gemini CLI 等。
//!
//! # 功能
//!
//! - 配置文件路径管理
//! - 配置读写（支持 JSON、TOML、.env 格式）
//! - 原子写入（防止配置损坏）
//! - 模型切换
//!
//! # 示例
//!
//! ```rust,ignore
//! use cc_switch::{CliType, switch_model};
//!
//! // 切换 Claude Code 模型
//! switch_model(CliType::ClaudeCode, "sonnet", &config).await?;
//! ```

pub mod error;
pub mod config_path;

// 后续模块（Task 2.3-2.5 中添加）
// pub mod atomic_write;
// pub mod claude;
// pub mod codex;
// pub mod gemini;
// pub mod switcher;

pub use error::{CCSwitchError, Result};
pub use config_path::*;

/// 支持的 CLI 类型
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CliType {
    /// Claude Code
    ClaudeCode,
    /// OpenAI Codex
    Codex,
    /// Google Gemini CLI
    Gemini,
    /// Amp
    Amp,
    /// Cursor Agent
    CursorAgent,
    /// Qwen Code
    QwenCode,
    /// GitHub Copilot
    Copilot,
    /// Droid
    Droid,
    /// Opencode
    Opencode,
}

impl CliType {
    /// 从字符串解析 CLI 类型
    pub fn from_str(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "claude-code" | "claude" => Some(Self::ClaudeCode),
            "codex" => Some(Self::Codex),
            "gemini-cli" | "gemini" => Some(Self::Gemini),
            "amp" => Some(Self::Amp),
            "cursor-agent" | "cursor" => Some(Self::CursorAgent),
            "qwen-code" | "qwen" => Some(Self::QwenCode),
            "copilot" => Some(Self::Copilot),
            "droid" => Some(Self::Droid),
            "opencode" => Some(Self::Opencode),
            _ => None,
        }
    }

    /// 转换为字符串
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::ClaudeCode => "claude-code",
            Self::Codex => "codex",
            Self::Gemini => "gemini-cli",
            Self::Amp => "amp",
            Self::CursorAgent => "cursor-agent",
            Self::QwenCode => "qwen-code",
            Self::Copilot => "copilot",
            Self::Droid => "droid",
            Self::Opencode => "opencode",
        }
    }

    /// 获取显示名称
    pub fn display_name(&self) -> &'static str {
        match self {
            Self::ClaudeCode => "Claude Code",
            Self::Codex => "Codex",
            Self::Gemini => "Gemini CLI",
            Self::Amp => "Amp",
            Self::CursorAgent => "Cursor Agent",
            Self::QwenCode => "Qwen Code",
            Self::Copilot => "GitHub Copilot",
            Self::Droid => "Droid",
            Self::Opencode => "Opencode",
        }
    }

    /// 是否支持配置切换
    pub fn supports_config_switch(&self) -> bool {
        matches!(self, Self::ClaudeCode | Self::Codex | Self::Gemini)
    }
}
```

---

**Step 2.2.5: 添加到 workspace**

修改 `vibe-kanban-main/Cargo.toml`，在 `members` 数组中添加：

```toml
[workspace]
resolver = "2"
members = [
    "crates/server",
    "crates/db",
    "crates/executors",
    "crates/services",
    "crates/utils",
    "crates/local-deployment",
    "crates/deployment",
    "crates/remote",
    "crates/review",
    "crates/cc-switch",  # 新增
]
```

---

**交付物:**
- `vibe-kanban-main/crates/cc-switch/Cargo.toml`
- `vibe-kanban-main/crates/cc-switch/src/lib.rs`
- `vibe-kanban-main/crates/cc-switch/src/error.rs`
- `vibe-kanban-main/crates/cc-switch/src/config_path.rs`
- 修改后的 workspace `Cargo.toml`

**验收标准:**
1. crate 编译通过：`cargo build -p cc-switch`
2. 单元测试通过：`cargo test -p cc-switch`

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p cc-switch
# 预期: 编译成功

cargo test -p cc-switch
# 预期: 测试通过
```

---

### Task 2.3: 实现原子写入和配置读写

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.2 已完成

**目标:**
实现配置文件的原子写入功能，防止写入过程中断导致配置损坏。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/cc-switch/src/atomic_write.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/claude.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/codex.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/gemini.rs`
- 修改: `vibe-kanban-main/crates/cc-switch/src/lib.rs`

---

**Step 2.3.1: 创建 atomic_write.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/atomic_write.rs`

```rust
//! 原子写入工具
//!
//! 使用临时文件 + 重命名的方式实现原子写入，
//! 防止写入过程中断导致配置文件损坏。

use std::path::Path;
use crate::error::{CCSwitchError, Result};
use crate::config_path::ensure_parent_dir_exists;

/// 原子写入文件
///
/// 流程：
/// 1. 写入临时文件
/// 2. 同步到磁盘
/// 3. 重命名为目标文件（原子操作）
///
/// # 参数
/// - `path`: 目标文件路径
/// - `data`: 要写入的数据
///
/// # 示例
/// ```rust,ignore
/// atomic_write(&path, b"content").await?;
/// ```
pub async fn atomic_write(path: &Path, data: &[u8]) -> Result<()> {
    use tokio::io::AsyncWriteExt;

    // 确保父目录存在
    ensure_parent_dir_exists(&path.to_path_buf()).await?;

    // 创建临时文件（在同一目录下，确保重命名是原子的）
    let parent = path.parent().unwrap_or(Path::new("."));
    let temp_path = parent.join(format!(
        ".{}.tmp.{}",
        path.file_name()
            .map(|n| n.to_string_lossy().to_string())
            .unwrap_or_else(|| "config".to_string()),
        std::process::id()
    ));

    // 写入临时文件
    let mut file = tokio::fs::File::create(&temp_path).await?;
    file.write_all(data).await?;
    file.sync_all().await?; // 确保数据写入磁盘
    drop(file);

    // 原子重命名
    tokio::fs::rename(&temp_path, path).await.map_err(|e| {
        // 清理临时文件
        let _ = std::fs::remove_file(&temp_path);
        CCSwitchError::AtomicWriteError(format!(
            "Failed to rename {} to {}: {}",
            temp_path.display(),
            path.display(),
            e
        ))
    })?;

    Ok(())
}

/// 原子写入 JSON 文件
pub async fn atomic_write_json<T: serde::Serialize>(path: &Path, value: &T) -> Result<()> {
    let json = serde_json::to_string_pretty(value)?;
    atomic_write(path, json.as_bytes()).await
}

/// 原子写入文本文件
pub async fn atomic_write_text(path: &Path, text: &str) -> Result<()> {
    atomic_write(path, text.as_bytes()).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_atomic_write() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");

        atomic_write(&path, b"hello world").await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        assert_eq!(content, "hello world");
    }

    #[tokio::test]
    async fn test_atomic_write_json() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.json");

        let data = serde_json::json!({"key": "value"});
        atomic_write_json(&path, &data).await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        let parsed: serde_json::Value = serde_json::from_str(&content).unwrap();
        assert_eq!(parsed["key"], "value");
    }
}
```

---

**Step 2.3.2: 创建 claude.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/claude.rs`

```rust
//! Claude Code 配置管理
//!
//! Claude Code 使用 JSON 格式的配置文件：
//! - ~/.claude/settings.json - 主配置文件

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::path::Path;
use crate::error::{CCSwitchError, Result};
use crate::config_path::{get_claude_settings_path, ensure_parent_dir_exists};
use crate::atomic_write::atomic_write_json;

/// Claude Code 配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ClaudeConfig {
    /// 环境变量配置
    #[serde(default)]
    pub env: ClaudeEnvConfig,

    /// 其他配置（保留原有字段）
    #[serde(flatten)]
    pub other: Value,
}

/// Claude Code 环境变量配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ClaudeEnvConfig {
    /// API Base URL
    #[serde(rename = "ANTHROPIC_BASE_URL", skip_serializing_if = "Option::is_none")]
    pub base_url: Option<String>,

    /// API Token
    #[serde(rename = "ANTHROPIC_AUTH_TOKEN", skip_serializing_if = "Option::is_none")]
    pub auth_token: Option<String>,

    /// API Key (备选)
    #[serde(rename = "ANTHROPIC_API_KEY", skip_serializing_if = "Option::is_none")]
    pub api_key: Option<String>,

    /// 默认模型
    #[serde(rename = "ANTHROPIC_MODEL", skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,

    /// Haiku 模型
    #[serde(rename = "ANTHROPIC_DEFAULT_HAIKU_MODEL", skip_serializing_if = "Option::is_none")]
    pub haiku_model: Option<String>,

    /// Sonnet 模型
    #[serde(rename = "ANTHROPIC_DEFAULT_SONNET_MODEL", skip_serializing_if = "Option::is_none")]
    pub sonnet_model: Option<String>,

    /// Opus 模型
    #[serde(rename = "ANTHROPIC_DEFAULT_OPUS_MODEL", skip_serializing_if = "Option::is_none")]
    pub opus_model: Option<String>,

    /// 其他环境变量
    #[serde(flatten)]
    pub other: std::collections::HashMap<String, Value>,
}

/// 读取 Claude 配置
pub async fn read_claude_config() -> Result<ClaudeConfig> {
    let path = get_claude_settings_path()?;
    read_claude_config_from(&path).await
}

/// 从指定路径读取 Claude 配置
pub async fn read_claude_config_from(path: &Path) -> Result<ClaudeConfig> {
    if !path.exists() {
        return Ok(ClaudeConfig::default());
    }

    let content = tokio::fs::read_to_string(path).await?;
    let config: ClaudeConfig = serde_json::from_str(&content)?;
    Ok(config)
}

/// 写入 Claude 配置
pub async fn write_claude_config(config: &ClaudeConfig) -> Result<()> {
    let path = get_claude_settings_path()?;
    write_claude_config_to(&path, config).await
}

/// 写入 Claude 配置到指定路径
pub async fn write_claude_config_to(path: &Path, config: &ClaudeConfig) -> Result<()> {
    ensure_parent_dir_exists(&path.to_path_buf()).await?;
    atomic_write_json(path, config).await
}

/// 更新 Claude 模型配置
///
/// # 参数
/// - `base_url`: API Base URL（可选，None 表示使用官方 API）
/// - `api_key`: API Key
/// - `model`: 模型名称
pub async fn update_claude_model(
    base_url: Option<&str>,
    api_key: &str,
    model: &str,
) -> Result<()> {
    let mut config = read_claude_config().await?;

    config.env.base_url = base_url.map(|s| s.to_string());
    config.env.auth_token = Some(api_key.to_string());
    config.env.model = Some(model.to_string());

    write_claude_config(&config).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_read_write_claude_config() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("settings.json");

        let config = ClaudeConfig {
            env: ClaudeEnvConfig {
                base_url: Some("https://api.example.com".to_string()),
                auth_token: Some("sk-test".to_string()),
                model: Some("claude-sonnet".to_string()),
                ..Default::default()
            },
            other: serde_json::json!({}),
        };

        write_claude_config_to(&path, &config).await.unwrap();

        let loaded = read_claude_config_from(&path).await.unwrap();
        assert_eq!(loaded.env.base_url, config.env.base_url);
        assert_eq!(loaded.env.auth_token, config.env.auth_token);
    }
}
```

---

**Step 2.3.3: 创建 codex.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/codex.rs`

```rust
//! Codex 配置管理
//!
//! Codex 使用两个配置文件：
//! - ~/.codex/auth.json - API 认证信息
//! - ~/.codex/config.toml - 模型和提供商配置

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::path::Path;
use crate::error::{CCSwitchError, Result};
use crate::config_path::{get_codex_auth_path, get_codex_config_path, ensure_parent_dir_exists};
use crate::atomic_write::{atomic_write_json, atomic_write_text};

/// Codex 认证配置 (auth.json)
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CodexAuthConfig {
    /// OpenAI API Key
    #[serde(rename = "OPENAI_API_KEY", skip_serializing_if = "Option::is_none")]
    pub openai_api_key: Option<String>,

    /// 其他字段
    #[serde(flatten)]
    pub other: std::collections::HashMap<String, Value>,
}

/// Codex 模型配置 (config.toml)
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CodexModelConfig {
    /// 模型提供商
    pub model_provider: Option<String>,

    /// 模型名称
    pub model: Option<String>,

    /// 提供商配置
    #[serde(default)]
    pub model_providers: std::collections::HashMap<String, CodexProviderConfig>,
}

/// Codex 提供商配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CodexProviderConfig {
    /// Base URL
    pub base_url: Option<String>,
}

/// 读取 Codex 认证配置
pub async fn read_codex_auth() -> Result<CodexAuthConfig> {
    let path = get_codex_auth_path()?;
    if !path.exists() {
        return Ok(CodexAuthConfig::default());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    let config: CodexAuthConfig = serde_json::from_str(&content)?;
    Ok(config)
}

/// 写入 Codex 认证配置
pub async fn write_codex_auth(config: &CodexAuthConfig) -> Result<()> {
    let path = get_codex_auth_path()?;
    ensure_parent_dir_exists(&path).await?;
    atomic_write_json(&path, config).await
}

/// 读取 Codex 模型配置
pub async fn read_codex_config() -> Result<CodexModelConfig> {
    let path = get_codex_config_path()?;
    if !path.exists() {
        return Ok(CodexModelConfig::default());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    let config: CodexModelConfig = toml::from_str(&content)?;
    Ok(config)
}

/// 写入 Codex 模型配置
pub async fn write_codex_config(config: &CodexModelConfig) -> Result<()> {
    let path = get_codex_config_path()?;
    ensure_parent_dir_exists(&path).await?;
    let toml_str = toml::to_string_pretty(config)
        .map_err(|e| CCSwitchError::InvalidConfig { message: e.to_string() })?;
    atomic_write_text(&path, &toml_str).await
}

/// 更新 Codex 模型配置
pub async fn update_codex_model(
    base_url: Option<&str>,
    api_key: &str,
    model: &str,
) -> Result<()> {
    // 更新 auth.json
    let mut auth = read_codex_auth().await?;
    auth.openai_api_key = Some(api_key.to_string());
    write_codex_auth(&auth).await?;

    // 更新 config.toml
    let mut config = read_codex_config().await?;
    config.model = Some(model.to_string());

    if let Some(url) = base_url {
        config.model_provider = Some("custom".to_string());
        config.model_providers.insert(
            "custom".to_string(),
            CodexProviderConfig {
                base_url: Some(url.to_string()),
            },
        );
    } else {
        config.model_provider = Some("openai".to_string());
    }

    write_codex_config(&config).await
}
```

---

**Step 2.3.4: 创建 gemini.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/gemini.rs`

```rust
//! Gemini CLI 配置管理
//!
//! Gemini CLI 使用 .env 格式的配置文件：
//! - ~/.gemini/.env - 环境变量配置

use std::collections::HashMap;
use std::path::Path;
use crate::error::Result;
use crate::config_path::{get_gemini_env_path, ensure_parent_dir_exists};
use crate::atomic_write::atomic_write_text;

/// 解析 .env 文件内容
pub fn parse_env_file(content: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();
    for line in content.lines() {
        let line = line.trim();
        // 跳过空行和注释
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        // 解析 KEY=VALUE
        if let Some(pos) = line.find('=') {
            let key = line[..pos].trim().to_string();
            let value = line[pos + 1..].trim();
            // 移除引号
            let value = value.trim_matches('"').trim_matches('\'').to_string();
            map.insert(key, value);
        }
    }
    map
}

/// 序列化为 .env 格式
pub fn serialize_env_file(map: &HashMap<String, String>) -> String {
    let mut lines: Vec<String> = map
        .iter()
        .map(|(k, v)| {
            // 如果值包含空格或特殊字符，使用引号
            if v.contains(' ') || v.contains('=') || v.contains('#') {
                format!("{}=\"{}\"", k, v)
            } else {
                format!("{}={}", k, v)
            }
        })
        .collect();
    lines.sort(); // 保持顺序一致
    lines.join("\n") + "\n"
}

/// 读取 Gemini 配置
pub async fn read_gemini_config() -> Result<HashMap<String, String>> {
    let path = get_gemini_env_path()?;
    if !path.exists() {
        return Ok(HashMap::new());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    Ok(parse_env_file(&content))
}

/// 写入 Gemini 配置
pub async fn write_gemini_config(config: &HashMap<String, String>) -> Result<()> {
    let path = get_gemini_env_path()?;
    ensure_parent_dir_exists(&path).await?;
    let content = serialize_env_file(config);
    atomic_write_text(&path, &content).await
}

/// 更新 Gemini 模型配置
pub async fn update_gemini_model(
    base_url: Option<&str>,
    api_key: &str,
    model: &str,
) -> Result<()> {
    let mut config = read_gemini_config().await?;

    // 设置 API Key
    config.insert("GEMINI_API_KEY".to_string(), api_key.to_string());

    // 设置 Base URL（如果提供）
    if let Some(url) = base_url {
        config.insert("GOOGLE_GEMINI_BASE_URL".to_string(), url.to_string());
    } else {
        config.remove("GOOGLE_GEMINI_BASE_URL");
    }

    // 设置模型
    config.insert("GEMINI_MODEL".to_string(), model.to_string());

    write_gemini_config(&config).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_env_file() {
        let content = r#"
# Comment
GEMINI_API_KEY=test-key
GEMINI_MODEL="gemini-pro"
EMPTY=
"#;
        let map = parse_env_file(content);
        assert_eq!(map.get("GEMINI_API_KEY"), Some(&"test-key".to_string()));
        assert_eq!(map.get("GEMINI_MODEL"), Some(&"gemini-pro".to_string()));
        assert_eq!(map.get("EMPTY"), Some(&"".to_string()));
    }

    #[test]
    fn test_serialize_env_file() {
        let mut map = HashMap::new();
        map.insert("KEY1".to_string(), "value1".to_string());
        map.insert("KEY2".to_string(), "value with space".to_string());

        let content = serialize_env_file(&map);
        assert!(content.contains("KEY1=value1"));
        assert!(content.contains("KEY2=\"value with space\""));
    }
}
```

---

**Step 2.3.5: 更新 lib.rs 导出新模块**

修改 `vibe-kanban-main/crates/cc-switch/src/lib.rs`，取消注释并添加导出：

```rust
pub mod error;
pub mod config_path;
pub mod atomic_write;
pub mod claude;
pub mod codex;
pub mod gemini;

pub use error::{CCSwitchError, Result};
pub use config_path::*;
pub use atomic_write::*;
pub use claude::*;
pub use codex::*;
pub use gemini::*;
```

---

**交付物:**
- `vibe-kanban-main/crates/cc-switch/src/atomic_write.rs`
- `vibe-kanban-main/crates/cc-switch/src/claude.rs`
- `vibe-kanban-main/crates/cc-switch/src/codex.rs`
- `vibe-kanban-main/crates/cc-switch/src/gemini.rs`
- 更新后的 `lib.rs`

**验收标准:**
1. 编译通过：`cargo build -p cc-switch`
2. 所有测试通过：`cargo test -p cc-switch`
3. 可以读写各 CLI 的配置文件

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p cc-switch
# 预期: 所有测试通过
```

---

### Task 2.4: 实现模型切换服务

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.3 已完成

**目标:**
实现统一的模型切换接口，支持切换 Claude Code、Codex、Gemini CLI 的模型配置。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/cc-switch/src/switcher.rs`
- 修改: `vibe-kanban-main/crates/cc-switch/src/lib.rs`

---

**Step 2.4.1: 创建 switcher.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/switcher.rs`

```rust
//! 模型切换服务
//!
//! 提供统一的模型切换接口。

use crate::error::{CCSwitchError, Result};
use crate::{CliType, claude, codex, gemini};

/// 模型切换配置
#[derive(Debug, Clone)]
pub struct SwitchConfig {
    /// API Base URL（可选，None 表示使用官方 API）
    pub base_url: Option<String>,
    /// API Key
    pub api_key: String,
    /// 模型名称
    pub model: String,
}

/// 切换模型
///
/// # 参数
/// - `cli_type`: CLI 类型
/// - `config`: 切换配置
///
/// # 示例
/// ```rust,ignore
/// use cc_switch::{CliType, SwitchConfig, switch_model};
///
/// let config = SwitchConfig {
///     base_url: Some("https://api.example.com".to_string()),
///     api_key: "sk-xxx".to_string(),
///     model: "claude-sonnet-4".to_string(),
/// };
///
/// switch_model(CliType::ClaudeCode, &config).await?;
/// ```
pub async fn switch_model(cli_type: CliType, config: &SwitchConfig) -> Result<()> {
    tracing::info!(
        "Switching model for {}: model={}, base_url={:?}",
        cli_type.display_name(),
        config.model,
        config.base_url
    );

    match cli_type {
        CliType::ClaudeCode => {
            claude::update_claude_model(
                config.base_url.as_deref(),
                &config.api_key,
                &config.model,
            ).await
        }
        CliType::Codex => {
            codex::update_codex_model(
                config.base_url.as_deref(),
                &config.api_key,
                &config.model,
            ).await
        }
        CliType::Gemini => {
            gemini::update_gemini_model(
                config.base_url.as_deref(),
                &config.api_key,
                &config.model,
            ).await
        }
        _ => {
            Err(CCSwitchError::UnsupportedCli {
                cli_name: cli_type.as_str().to_string(),
            })
        }
    }
}

/// 批量切换模型（用于工作流启动时）
///
/// 按顺序切换多个终端的模型配置。
/// 注意：由于 cc-switch 修改全局环境变量，必须串行执行。
pub async fn switch_models_sequential(
    configs: Vec<(CliType, SwitchConfig)>,
) -> Result<Vec<Result<()>>> {
    let mut results = Vec::new();

    for (cli_type, config) in configs {
        let result = switch_model(cli_type, &config).await;
        results.push(result);
    }

    Ok(results)
}

/// 模型切换服务
///
/// 提供更高级的模型切换功能，包括：
/// - 配置备份和恢复
/// - 切换前验证
/// - 切换后验证
pub struct ModelSwitcher {
    /// 是否在切换前备份配置
    backup_before_switch: bool,
}

impl ModelSwitcher {
    pub fn new() -> Self {
        Self {
            backup_before_switch: true,
        }
    }

    /// 设置是否在切换前备份
    pub fn with_backup(mut self, backup: bool) -> Self {
        self.backup_before_switch = backup;
        self
    }

    /// 切换模型
    pub async fn switch(&self, cli_type: CliType, config: &SwitchConfig) -> Result<()> {
        // TODO: 实现备份功能
        if self.backup_before_switch {
            tracing::debug!("Backing up config before switch...");
            // self.backup_config(cli_type).await?;
        }

        switch_model(cli_type, config).await
    }
}

impl Default for ModelSwitcher {
    fn default() -> Self {
        Self::new()
    }
}
```

---

**Step 2.4.2: 更新 lib.rs**

在 `lib.rs` 中添加：

```rust
pub mod switcher;
pub use switcher::*;
```

---

**交付物:**
- `vibe-kanban-main/crates/cc-switch/src/switcher.rs`
- 更新后的 `lib.rs`

**验收标准:**
1. 编译通过
2. `switch_model` 函数可以正确切换各 CLI 的配置

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p cc-switch
cargo test -p cc-switch
```

---

### Task 2.5: 集成 cc-switch 到 services

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.4 已完成

**目标:**
将 cc-switch crate 集成到 vibe-kanban 的 services 层。

**涉及文件:**
- 修改: `vibe-kanban-main/crates/services/Cargo.toml`
- 创建: `vibe-kanban-main/crates/services/src/services/cc_switch.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/mod.rs`

---

**Step 2.5.1: 添加依赖**

修改 `vibe-kanban-main/crates/services/Cargo.toml`，添加：

```toml
[dependencies]
cc-switch = { path = "../cc-switch" }
```

---

**Step 2.5.2: 创建 cc_switch.rs 服务**

文件路径: `vibe-kanban-main/crates/services/src/services/cc_switch.rs`

```rust
//! CC-Switch 服务
//!
//! 封装 cc-switch crate，提供与 vibe-kanban 集成的接口。

use cc_switch::{CliType, SwitchConfig, switch_model, ModelSwitcher};
use db::models::{Terminal, CliType as DbCliType, ModelConfig};
use db::DBService;
use std::sync::Arc;

/// CC-Switch 服务
pub struct CCSwitchService {
    db: Arc<DBService>,
    switcher: ModelSwitcher,
}

impl CCSwitchService {
    pub fn new(db: Arc<DBService>) -> Self {
        Self {
            db,
            switcher: ModelSwitcher::new(),
        }
    }

    /// 为终端切换模型
    ///
    /// 根据终端配置切换对应 CLI 的模型。
    pub async fn switch_for_terminal(&self, terminal: &Terminal) -> anyhow::Result<()> {
        // 获取 CLI 类型信息
        let cli_type = db::models::cli_type_dao::get_cli_type_by_id(
            &self.db.pool,
            &terminal.cli_type_id,
        ).await?
        .ok_or_else(|| anyhow::anyhow!("CLI type not found: {}", terminal.cli_type_id))?;

        // 获取模型配置
        let model_config = db::models::cli_type_dao::get_model_config_by_id(
            &self.db.pool,
            &terminal.model_config_id,
        ).await?
        .ok_or_else(|| anyhow::anyhow!("Model config not found: {}", terminal.model_config_id))?;

        // 解析 CLI 类型
        let cli = CliType::from_str(&cli_type.name)
            .ok_or_else(|| anyhow::anyhow!("Unsupported CLI: {}", cli_type.name))?;

        // 构建切换配置
        let config = SwitchConfig {
            base_url: terminal.custom_base_url.clone(),
            api_key: terminal.custom_api_key.clone()
                .ok_or_else(|| anyhow::anyhow!("API key not configured for terminal"))?,
            model: model_config.api_model_id
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

    /// 批量切换模型（用于工作流启动）
    ///
    /// 按顺序为所有终端切换模型配置。
    pub async fn switch_for_terminals(&self, terminals: &[Terminal]) -> anyhow::Result<()> {
        for terminal in terminals {
            self.switch_for_terminal(terminal).await?;
        }
        Ok(())
    }

    /// 检测 CLI 安装状态
    pub async fn detect_cli(&self, cli_name: &str) -> anyhow::Result<bool> {
        use tokio::process::Command;

        let cli_type = db::models::cli_type_dao::get_cli_type_by_name(
            &self.db.pool,
            cli_name,
        ).await?;

        if let Some(cli) = cli_type {
            let parts: Vec<&str> = cli.detect_command.split_whitespace().collect();
            if parts.is_empty() {
                return Ok(false);
            }

            let result = Command::new(parts[0])
                .args(&parts[1..])
                .output()
                .await;

            Ok(result.map(|o| o.status.success()).unwrap_or(false))
        } else {
            Ok(false)
        }
    }
}
```

---

**Step 2.5.3: 更新 services/mod.rs**

在 `vibe-kanban-main/crates/services/src/services/mod.rs` 中添加：

```rust
pub mod cc_switch;
pub use cc_switch::CCSwitchService;
```

---

**交付物:**
- 修改后的 `services/Cargo.toml`
- `vibe-kanban-main/crates/services/src/services/cc_switch.rs`
- 修改后的 `services/mod.rs`

**验收标准:**
1. 编译通过：`cargo build -p services`
2. CCSwitchService 可以正常实例化

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p services
```

---

### Phase 2 单元测试用例

> 在 `vibe-kanban-main/crates/cc-switch/src/tests.rs` 创建以下测试

```rust
//! CC-Switch 单元测试
//!
//! 测试配置读写、原子写入、模型切换等核心功能

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    // =========================================================================
    // 测试 1: Claude 配置文件路径解析
    // =========================================================================
    #[test]
    fn test_claude_config_path() {
        let path = get_claude_config_path();
        assert!(path.ends_with(".claude.json") || path.ends_with("claude_desktop_config.json"));
    }

    // =========================================================================
    // 测试 2: Claude 配置读取
    // =========================================================================
    #[test]
    fn test_read_claude_config() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".claude.json");

        // 创建测试配置
        let test_config = r#"{
            "apiProvider": "anthropic",
            "apiKey": "test-key-123",
            "model": "claude-sonnet-4-20250514"
        }"#;
        fs::write(&config_path, test_config).unwrap();

        let config = read_claude_config(&config_path).unwrap();
        assert_eq!(config.api_provider, Some("anthropic".to_string()));
        assert_eq!(config.api_key, Some("test-key-123".to_string()));
        assert_eq!(config.model, Some("claude-sonnet-4-20250514".to_string()));
    }

    // =========================================================================
    // 测试 3: Claude 配置写入（原子写入）
    // =========================================================================
    #[test]
    fn test_write_claude_config_atomic() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".claude.json");

        let config = ClaudeConfig {
            api_provider: Some("openai-compatible".to_string()),
            api_key: Some("sk-new-key".to_string()),
            api_base_url: Some("https://api.example.com/v1".to_string()),
            model: Some("gpt-4".to_string()),
            ..Default::default()
        };

        write_claude_config(&config_path, &config).unwrap();

        // 验证文件内容
        let content = fs::read_to_string(&config_path).unwrap();
        assert!(content.contains("openai-compatible"));
        assert!(content.contains("sk-new-key"));
        assert!(content.contains("https://api.example.com/v1"));
    }

    // =========================================================================
    // 测试 4: 原子写入失败回滚
    // =========================================================================
    #[test]
    fn test_atomic_write_rollback() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".claude.json");

        // 写入原始配置
        let original = r#"{"model": "original"}"#;
        fs::write(&config_path, original).unwrap();

        // 尝试写入无效配置（模拟失败场景）
        // 原子写入应该保护原始文件不被损坏
        let result = atomic_write(&config_path, |_| -> Result<(), std::io::Error> {
            Err(std::io::Error::new(std::io::ErrorKind::Other, "simulated failure"))
        });

        assert!(result.is_err());

        // 验证原始文件未被修改
        let content = fs::read_to_string(&config_path).unwrap();
        assert!(content.contains("original"));
    }

    // =========================================================================
    // 测试 5: Codex 配置读写
    // =========================================================================
    #[test]
    fn test_codex_config() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".codex");

        let config = CodexConfig {
            model: "o3-mini".to_string(),
            provider: "openai".to_string(),
            approval_mode: "suggest".to_string(),
        };

        write_codex_config(&config_path, &config).unwrap();
        let loaded = read_codex_config(&config_path).unwrap();

        assert_eq!(loaded.model, "o3-mini");
        assert_eq!(loaded.provider, "openai");
    }

    // =========================================================================
    // 测试 6: Gemini 配置读写
    // =========================================================================
    #[test]
    fn test_gemini_config() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join("settings.json");

        let config = GeminiConfig {
            model: "gemini-2.0-flash".to_string(),
            api_key: "AIza-test-key".to_string(),
        };

        write_gemini_config(&config_path, &config).unwrap();
        let loaded = read_gemini_config(&config_path).unwrap();

        assert_eq!(loaded.model, "gemini-2.0-flash");
    }

    // =========================================================================
    // 测试 7: 模型切换服务 - Claude
    // =========================================================================
    #[tokio::test]
    async fn test_switch_claude_model() {
        let temp_dir = TempDir::new().unwrap();
        let switcher = ModelSwitcher::new_with_config_dir(temp_dir.path().to_path_buf());

        let request = SwitchModelRequest {
            cli_type: CliType::ClaudeCode,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: Some("test-key".to_string()),
            api_base_url: None,
        };

        switcher.switch_model(&request).await.unwrap();

        // 验证配置已更新
        let config = switcher.get_current_config(CliType::ClaudeCode).await.unwrap();
        assert_eq!(config.model, Some("claude-sonnet-4-20250514".to_string()));
    }

    // =========================================================================
    // 测试 8: 模型列表获取（需要 Mock）
    // =========================================================================
    #[tokio::test]
    async fn test_fetch_available_models() {
        // 使用 Mock HTTP 服务器
        let mock_server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/v1/models"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "data": [
                    {"id": "claude-sonnet-4-20250514"},
                    {"id": "claude-3-5-haiku-20241022"}
                ]
            })))
            .mount(&mock_server)
            .await;

        let fetcher = ModelFetcher::new(&mock_server.uri(), "test-key");
        let models = fetcher.fetch_models().await.unwrap();

        assert_eq!(models.len(), 2);
        assert!(models.iter().any(|m| m.id == "claude-sonnet-4-20250514"));
    }
}
```

**运行测试:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p cc-switch -- --nocapture
```

---

## Phase 2 完成检查清单

- [ ] Task 2.1: CC-Switch 代码分析完成
- [ ] Task 2.2: cc-switch crate 创建完成，编译通过
- [ ] Task 2.3: 配置读写功能实现，测试通过
- [ ] Task 2.4: 模型切换服务实现
- [ ] Task 2.5: 集成到 services 层

---
