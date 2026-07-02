#![warn(clippy::pedantic)]
#![allow(
    clippy::doc_markdown,
    clippy::module_name_repetitions,
    clippy::must_use_candidate,
    clippy::missing_errors_doc,
    clippy::missing_panics_doc,
    clippy::similar_names,
    clippy::too_many_lines
)]

//! CC-Switch Core
//!
//! CLI 配置切换核心库，支持 Claude Code、Codex 等。
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

pub mod atomic_write;
pub mod claude;
pub mod codex;
pub mod config_path;
pub mod error;
pub mod switcher;

pub use atomic_write::*;
pub use claude::*;
pub use codex::*;
pub use config_path::*;
pub use error::{CCSwitchError, Result};
pub use switcher::*;

/// 支持的 CLI 类型
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CliType {
    /// Claude Code
    ClaudeCode,
    /// OpenAI Codex
    Codex,
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
    pub fn parse(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "claude-code" | "claude" => Some(Self::ClaudeCode),
            "codex" => Some(Self::Codex),
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
        matches!(self, Self::ClaudeCode | Self::Codex)
    }
}

impl std::str::FromStr for CliType {
    type Err = ();

    fn from_str(s: &str) -> std::result::Result<Self, Self::Err> {
        Self::parse(s).ok_or(())
    }
}
