//! Orchestrator 配置

use serde::{Deserialize, Serialize};

use super::constants::{
    DEFAULT_LLM_RATE_LIMIT_PER_SECOND, DEFAULT_LLM_TIMEOUT_SECS, DEFAULT_MAX_CONVERSATION_HISTORY,
    DEFAULT_MAX_RETRIES, DEFAULT_RETRY_DELAY_MS,
};

/// Orchestrator 配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrchestratorConfig {
    /// API 类型: "openai", "anthropic", "custom"
    pub api_type: String,

    /// API Base URL
    pub base_url: String,

    /// API Key
    pub api_key: String,

    /// 模型名称
    pub model: String,

    /// 最大重试次数
    #[serde(default = "default_max_retries")]
    pub max_retries: u32,

    /// 请求超时（秒）
    #[serde(default = "default_timeout")]
    pub timeout_secs: u64,

    /// 重试延迟（毫秒）
    #[serde(default = "default_retry_delay")]
    pub retry_delay_ms: u64,

    /// 每秒请求数限制
    #[serde(default = "default_rate_limit_requests_per_second")]
    pub rate_limit_requests_per_second: u32,

    /// 最大对话历史长度
    #[serde(default = "default_max_history")]
    pub max_conversation_history: usize,

    /// 系统提示词
    #[serde(default = "default_system_prompt")]
    pub system_prompt: String,
}

fn default_max_retries() -> u32 {
    DEFAULT_MAX_RETRIES
}

fn default_timeout() -> u64 {
    DEFAULT_LLM_TIMEOUT_SECS
}

fn default_retry_delay() -> u64 {
    DEFAULT_RETRY_DELAY_MS
}

fn default_rate_limit_requests_per_second() -> u32 {
    DEFAULT_LLM_RATE_LIMIT_PER_SECOND
}

fn default_max_history() -> usize {
    DEFAULT_MAX_CONVERSATION_HISTORY
}

fn default_system_prompt() -> String {
    r#"你是 GitCortex 的主协调 Agent，负责协调多个 AI 编码代理完成软件开发任务。

你的职责：
1. 根据工作流配置，向各终端发送任务指令
2. 监控终端的执行状态（通过 Git 提交事件）
3. 协调审核流程，处理审核反馈
4. 在所有任务完成后，协调分支合并

规则：
- 每个终端完成任务后会提交 Git，你会收到提交事件
- 根据提交中的元数据判断下一步操作
- 如果审核发现问题，指导修复终端进行修复
- 保持简洁的指令，不要过度解释
- 你可以返回单个 JSON 指令对象，或按顺序执行的 JSON 指令数组
- 如果后续指令需要引用同一轮刚创建的 task/terminal，请在创建时显式提供 task_id / terminal_id

DIY 模式常用动作：
- start_task
- send_to_terminal
- complete_workflow
- fail_workflow

AgentPlanned 模式可额外使用：
- create_task
- create_terminal
- start_terminal
- close_terminal
- complete_task
- set_workflow_planning_complete

输出格式：
使用 JSON 格式输出指令，例如：
{"type": "send_to_terminal", "terminal_id": "xxx", "message": "具体指令"}

或：
[
  {"type": "create_task", "task_id": "task-api", "name": "Build API"},
  {"type": "create_terminal", "terminal_id": "term-api-1", "task_id": "task-api", "cli_type_id": "cli-claude-code", "model_config_id": "model-claude-sonnet", "role": "coder"},
  {"type": "start_terminal", "terminal_id": "term-api-1", "instruction": "实现 API 并提交代码"}
]
"#
    .to_string()
}

impl Default for OrchestratorConfig {
    fn default() -> Self {
        Self {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: String::new(),
            model: "gpt-4o".to_string(),
            max_retries: default_max_retries(),
            timeout_secs: default_timeout(),
            retry_delay_ms: default_retry_delay(),
            rate_limit_requests_per_second: default_rate_limit_requests_per_second(),
            max_conversation_history: default_max_history(),
            system_prompt: default_system_prompt(),
        }
    }
}

impl OrchestratorConfig {
    /// 从工作流配置创建
    pub fn from_workflow(
        api_type: Option<&str>,
        base_url: Option<&str>,
        api_key: Option<&str>,
        model: Option<&str>,
    ) -> Option<Self> {
        Some(Self {
            api_type: api_type?.to_string(),
            base_url: base_url?.to_string(),
            api_key: api_key?.to_string(),
            model: model?.to_string(),
            ..Default::default()
        })
    }

    /// 验证配置是否有效
    pub fn validate(&self) -> Result<(), String> {
        if self.api_key.is_empty() {
            return Err("API key is required".to_string());
        }
        if self.base_url.is_empty() {
            return Err("Base URL is required".to_string());
        }
        if self.model.is_empty() {
            return Err("Model is required".to_string());
        }
        if self.rate_limit_requests_per_second == 0 {
            return Err("Rate limit must be greater than 0".to_string());
        }
        Ok(())
    }
}
