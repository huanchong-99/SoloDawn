//! Orchestrator 配置

use serde::{Deserialize, Serialize};

use super::constants::{
    DEFAULT_LLM_RATE_LIMIT_PER_SECOND, DEFAULT_LLM_TIMEOUT_SECS, DEFAULT_MAX_CONVERSATION_HISTORY,
    DEFAULT_MAX_RETRIES, DEFAULT_RETRY_DELAY_MS, QUALITY_GATE_DEFAULT_MODE,
};

/// Configuration for a fallback LLM provider
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProviderConfig {
    /// Human-readable provider name (e.g. "anthropic-fallback")
    pub name: String,
    /// API type: "openai", "anthropic", "custom"
    pub api_type: String,
    /// API base URL
    pub base_url: String,
    /// API key
    pub api_key: String,
    /// Model name
    pub model: String,
    /// Priority (lower = higher priority); used for ordering fallbacks
    pub priority: u32,
}

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

    /// Auto-merge completed task branches when workflow completes
    #[serde(default = "default_auto_merge_on_completion")]
    pub auto_merge_on_completion: bool,

    /// Fallback LLM providers for multi-provider failover
    #[serde(default)]
    pub fallback_providers: Vec<ProviderConfig>,

    /// Quality gate mode: off | shadow | warn | enforce
    #[serde(default = "default_quality_gate_mode")]
    pub quality_gate_mode: String,
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

fn default_auto_merge_on_completion() -> bool {
    true
}

fn default_quality_gate_mode() -> String {
    QUALITY_GATE_DEFAULT_MODE.to_string()
}

/// Prompt profile identifier.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PromptProfile {
    /// Used during workspace planning phase — requirement discovery conversation.
    WorkspacePlanning,
    /// Used during workflow runtime — task/terminal coordination.
    RuntimeOrchestrator,
}

/// Returns the system prompt for the requested profile.
pub fn system_prompt_for_profile(profile: PromptProfile) -> String {
    match profile {
        PromptProfile::WorkspacePlanning => workspace_planning_prompt(),
        PromptProfile::RuntimeOrchestrator => default_system_prompt(),
    }
}

fn workspace_planning_prompt() -> String {
    r#"You are the GitCortex Workspace Planner — a friendly project requirements analyst.

Your goal is to understand what the user wants to build, then produce a precise
technical specification that the backend can use to create an execution workflow.

## Conversation rules

1. Always speak in the user's language.  If they write in Chinese, reply in Chinese.
2. Ask business-level follow-up questions.  Examples:
   - "Does your blog need a comment system?"
   - "Should users be able to sign up and log in?"
   - "Do you need an admin panel to manage content?"
3. NEVER ask about framework choices, stack decisions, or internal architecture
   unless the user brings it up first.
4. Keep each round to 1-3 focused questions.  Do not dump a long checklist.
5. When you have gathered enough information, summarise the requirements in plain
   language and ask the user to confirm.
6. After confirmation, output a single JSON block labelled `PLANNING_SPEC`:

```json
{
  "productGoal": "...",
  "requiredFeatures": ["...", "..."],
  "optionalFeatures": ["..."],
  "repositories": ["..."],
  "suggestedWorkerRoles": [
    {"role": "...", "cliTypeId": "...", "count": 1}
  ],
  "mergeStrategy": "orchestrator",
  "reviewStrategy": "dedicated_terminal"
}
```

## Strict boundaries

- You must NOT write, read, or review any code yourself.
- You must NOT suggest specific file structures or implementation details.
- You are only responsible for understanding the product vision and
  producing a structured planning specification.
"#
    .to_string()
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
            auto_merge_on_completion: default_auto_merge_on_completion(),
            fallback_providers: Vec::new(),
            quality_gate_mode: default_quality_gate_mode(),
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn planning_prompt_does_not_contain_json_instructions() {
        let prompt = system_prompt_for_profile(PromptProfile::WorkspacePlanning);
        assert!(
            !prompt.contains("send_to_terminal"),
            "planning prompt must not reference runtime actions"
        );
        assert!(
            !prompt.contains("start_task"),
            "planning prompt must not reference runtime actions"
        );
    }

    #[test]
    fn runtime_prompt_contains_json_instructions() {
        let prompt = system_prompt_for_profile(PromptProfile::RuntimeOrchestrator);
        assert!(prompt.contains("send_to_terminal"));
        assert!(prompt.contains("create_task"));
    }

    #[test]
    fn prompt_profiles_are_distinct() {
        let planning = system_prompt_for_profile(PromptProfile::WorkspacePlanning);
        let runtime = system_prompt_for_profile(PromptProfile::RuntimeOrchestrator);
        assert_ne!(planning, runtime);
    }

    #[test]
    fn planning_prompt_enforces_no_code_boundary() {
        let prompt = system_prompt_for_profile(PromptProfile::WorkspacePlanning);
        assert!(prompt.contains("must NOT write, read, or review any code"));
    }
}
