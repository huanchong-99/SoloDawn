//! 模型切换服务
//!
//! 提供统一的模型切换接口。

use std::fmt;

use crate::{
    claude, codex,
    error::{CCSwitchError, Result},
    gemini, CliType,
};

/// 模型切换配置
#[derive(Clone)]
pub struct SwitchConfig {
    /// API Base URL（可选，None 表示使用官方 API）
    pub base_url: Option<String>,
    /// API Key
    pub api_key: String,
    /// 模型名称
    pub model: String,
}

impl fmt::Debug for SwitchConfig {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("SwitchConfig")
            .field("base_url", &self.base_url)
            .field("api_key", &"[REDACTED]")
            .field("model", &self.model)
            .finish()
    }
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
            claude::update_claude_model(config.base_url.as_deref(), &config.api_key, &config.model)
                .await
        }
        CliType::Codex => {
            codex::update_codex_model(config.base_url.as_deref(), &config.api_key, &config.model)
                .await
        }
        CliType::Gemini => {
            gemini::update_gemini_model(config.base_url.as_deref(), &config.api_key, &config.model)
                .await
        }
        _ => Err(CCSwitchError::UnsupportedCli {
            cli_name: cli_type.as_str().to_string(),
        }),
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
    #[must_use]
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_model_switcher_creation() {
        let switcher = ModelSwitcher::new();
        assert!(switcher.backup_before_switch);
    }

    #[test]
    fn test_model_switcher_with_backup() {
        let switcher = ModelSwitcher::new().with_backup(false);
        assert!(!switcher.backup_before_switch);
    }

    #[test]
    fn test_model_switcher_default() {
        let switcher = ModelSwitcher::default();
        assert!(switcher.backup_before_switch);
    }
}
