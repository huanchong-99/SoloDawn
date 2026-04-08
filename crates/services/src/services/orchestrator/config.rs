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
    r#"You are the SoloDawn Workspace Planner — a friendly project requirements analyst.

Your goal is to understand what the user wants to build, then produce a precise
technical specification that the backend can use to create an execution workflow.

## Requirement Assessment

Before anything else, evaluate the user's first message:
- If the requirement is **vague or incomplete** (e.g. "make a knowledge management tool",
  "build a chat app", fewer than 3 concrete technical requirements), enter gathering mode:
  ask focused follow-up questions to clarify scope, features, and constraints.
- If the requirement is **precise and detailed** (e.g. 5+ specific technical requirements,
  explicit feature lists, clear scope), skip gathering and produce the PLANNING_SPEC
  directly after a brief confirmation summary.

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
    r#"You are the SoloDawn Orchestrator Agent. You decompose projects into Tasks (Git branches) and Terminals (AI coding agents).

RESPOND ONLY WITH A JSON ARRAY. No explanation text. The "type" field is REQUIRED on every object.

## Execution Model
Workflow → Task (own Git branch) → Terminal (PTY AI agent)

## Planning Strategy — Comprehensive Upfront Decomposition
- Analyze the full scope of the user's requirements and create ALL necessary tasks upfront.
- Each distinct requirement or feature area should be its own task with its own branch.
- For a 10-point requirement list, create 3-5 tasks that cover ALL points (group related items).
- Call set_workflow_planning_complete after creating all tasks and starting their terminals.
- Do NOT create a single catch-all task. Split work so terminals can run in parallel.

## Your Job
1. Analyze ALL user requirements — identify every deliverable
2. Create tasks covering ALL requirements (3-5 tasks, each with clear scope)
3. Create terminals for each task with create_terminal
4. Start terminals with detailed instructions via start_terminal
5. Call set_workflow_planning_complete after ALL tasks are dispatched
6. On terminal completion: complete_task, then wait for ALL tasks to finish
7. Only complete_workflow after ALL tasks are completed and merged

## Action Types
create_task: {"type":"create_task","task_id":"t1","name":"...", "branch":"feat/x","order_index":0}
create_terminal: {"type":"create_terminal","terminal_id":"tm1","task_id":"t1","cli_type_id":"...","model_config_id":"...","role":"coder","auto_confirm":true}
start_terminal: {"type":"start_terminal","terminal_id":"tm1","instruction":"..."}
send_to_terminal: {"type":"send_to_terminal","terminal_id":"tm1","message":"..."}
close_terminal: {"type":"close_terminal","terminal_id":"tm1","final_status":"completed"}
complete_task: {"type":"complete_task","task_id":"t1","summary":"..."}
set_workflow_planning_complete: {"type":"set_workflow_planning_complete","summary":"..."}
merge_branch: {"type":"merge_branch","source_branch":"feat/x","target_branch":"main"}
complete_workflow: {"type":"complete_workflow","summary":"..."}
fail_workflow: {"type":"fail_workflow","reason":"..."}

## Example Response
[
  {"type":"create_task","task_id":"task-1","name":"Refactor","branch":"feat/refactor","order_index":0},
  {"type":"create_terminal","terminal_id":"term-1","task_id":"task-1","cli_type_id":"cli-codex","model_config_id":"model-x","role":"coder","auto_confirm":true},
  {"type":"start_terminal","terminal_id":"term-1","instruction":"Refactor the codebase..."},
  {"type":"set_workflow_planning_complete","summary":"1 task"}
]

## Rules
- ONLY output JSON. No markdown, no explanation.
- Use explicit task_id and terminal_id so later actions can reference them.
- 1-2 terminals per task. Keep instructions actionable and specific.
- Create 3-5 tasks upfront to cover ALL user requirements. Do not leave requirements uncovered.
- You CAN create new tasks/terminals at any point after planning is complete.
- NEVER complete_workflow until ALL user requirements have been addressed by tasks.
- After all tasks complete: use merge_branch to merge each task branch into main, then complete_workflow.
- NEVER create a separate "integration review", "merge verification", or "final review" task. The merge_branch action handles merging automatically. When all coding tasks are done, merge and complete — do not spawn new terminals for review.

## CRITICAL: Unified Tech Stack
When building from scratch, ALL terminals MUST use the SAME technology stack. You MUST specify the exact tech stack in EVERY start_terminal instruction. Include:
- Language & runtime (e.g., "TypeScript + Node.js 20")
- Backend framework (e.g., "Express.js" — NOT different frameworks per terminal)
- Database + ORM (e.g., "PostgreSQL + Prisma" — NOT MongoDB in one terminal and PostgreSQL in another)
- Frontend framework if applicable (e.g., "React + Vite")
- Auth library (e.g., "passport.js + jsonwebtoken")
- Project root layout: specify where backend code lives (e.g., "all backend in src/") so terminals don't create conflicting directory structures

Format in each instruction: "Tech stack (shared across all tasks): [language] + [framework] + [database] + [ORM]. Backend code goes in [directory]."
Failure to unify tech stack across terminals causes merge conflicts and build failures.

## CRITICAL: Quality Requirements (Auto-Appended to Every Terminal)
The system will AUTOMATICALLY append the following requirements to every start_terminal instruction.
You do NOT need to include them in your instruction text — they are injected by the platform.
Focus your instruction on WHAT to build, not on quality standards.

[Auto-appended quality block — for reference only, do not repeat in instructions]
- Tests: every module MUST have test files with real assertions (Jest/Vitest/cargo test). Aim ≥60% coverage.
- Validation: every API endpoint MUST validate input (Zod/Joi/validator crate/Fastify schema).
- Security: no hardcoded secrets, use env vars + .env.example. JWT secret must fail-fast if missing.
- Docs: include README with setup steps. Include Dockerfile/docker-compose.
- Error handling: use custom error classes, not plain objects. Global error middleware required.
- If modifying existing code: preserve all existing functionality unchanged.
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

        let valid_api_types = [
            "openai",
            "anthropic",
            "openai-compatible",
            "anthropic-compatible",
        ];
        if !valid_api_types.contains(&self.api_type.as_str()) {
            return Err(format!(
                "Invalid api_type: {}. Must be one of: {:?}",
                self.api_type, valid_api_types
            ));
        }

        let valid_modes = ["off", "shadow", "warn", "enforce"];
        if !valid_modes.contains(&self.quality_gate_mode.as_str()) {
            return Err(format!(
                "Invalid quality_gate_mode: {}. Must be one of: {:?}",
                self.quality_gate_mode, valid_modes
            ));
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

    // ----- OrchestratorConfig::validate tests -----

    #[test]
    fn validate_rejects_empty_api_key() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com".to_string(),
            api_key: String::new(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };
        assert!(
            config.validate().is_err(),
            "Empty API key should fail validation"
        );
    }

    #[test]
    fn validate_rejects_empty_base_url() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: String::new(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };
        assert!(
            config.validate().is_err(),
            "Empty base URL should fail validation"
        );
    }

    #[test]
    fn validate_rejects_empty_model() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com".to_string(),
            api_key: "sk-test".to_string(),
            model: String::new(),
            ..Default::default()
        };
        assert!(
            config.validate().is_err(),
            "Empty model should fail validation"
        );
    }

    #[test]
    fn validate_accepts_valid_config() {
        let config = OrchestratorConfig {
            api_type: "openai-compatible".to_string(),
            base_url: "https://open.bigmodel.cn/api/paas/v4".to_string(),
            api_key: "sk-test-key".to_string(),
            model: "glm-5".to_string(),
            ..Default::default()
        };
        assert!(
            config.validate().is_ok(),
            "Valid config should pass validation"
        );
    }

    // ----- OrchestratorConfig::from_workflow tests -----

    #[test]
    fn from_workflow_returns_none_when_all_missing() {
        assert!(
            OrchestratorConfig::from_workflow(None, None, None, None).is_none(),
            "All None inputs should return None"
        );
    }

    #[test]
    fn from_workflow_returns_none_when_key_missing() {
        assert!(
            OrchestratorConfig::from_workflow(
                Some("openai"),
                Some("https://api.openai.com"),
                None,
                Some("gpt-4"),
            )
            .is_none(),
            "Missing API key should return None"
        );
    }

    #[test]
    fn from_workflow_returns_none_when_model_missing() {
        assert!(
            OrchestratorConfig::from_workflow(
                Some("openai"),
                Some("https://api.openai.com"),
                Some("sk-test"),
                None,
            )
            .is_none(),
            "Missing model should return None"
        );
    }

    #[test]
    fn from_workflow_valid_zhipuai() {
        let config = OrchestratorConfig::from_workflow(
            Some("openai-compatible"),
            Some("https://open.bigmodel.cn/api/paas/v4"),
            Some("sk-test-key"),
            Some("glm-5"),
        );
        assert!(config.is_some(), "Valid inputs should produce Some(config)");
        let config = config.unwrap();
        assert_eq!(config.api_type, "openai-compatible");
        assert_eq!(config.base_url, "https://open.bigmodel.cn/api/paas/v4");
        assert_eq!(config.api_key, "sk-test-key");
        assert_eq!(config.model, "glm-5");
    }

    #[test]
    fn from_workflow_inherits_defaults() {
        let config = OrchestratorConfig::from_workflow(
            Some("anthropic"),
            Some("https://api.anthropic.com"),
            Some("sk-ant-test"),
            Some("claude-sonnet-4-20250514"),
        )
        .unwrap();
        // Verify default fields are populated
        let defaults = OrchestratorConfig::default();
        assert_eq!(config.max_retries, defaults.max_retries);
        assert_eq!(config.timeout_secs, defaults.timeout_secs);
        assert_eq!(
            config.rate_limit_requests_per_second,
            defaults.rate_limit_requests_per_second
        );
    }
}
