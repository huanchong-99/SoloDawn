//! Orchestrator 主 Agent 模块
//!
//! 负责协调多个 AI 编码代理完成软件开发任务。

pub mod agent;
pub mod config;
pub mod constants;
pub mod llm;
pub mod message_bus;
pub mod persistence;
pub mod prompt_handler;
pub mod runtime;
pub mod runtime_actions;
pub mod state;
pub mod terminal_coordinator;
pub mod types;

pub use agent::OrchestratorAgent;
pub use config::OrchestratorConfig;
#[cfg(test)]
pub use llm::MockLLMClient;
pub use llm::{
    LLMClient, OpenAICompatibleClient, build_terminal_completion_prompt, create_llm_client,
};
pub use message_bus::{BusMessage, MessageBus, SharedMessageBus};
pub use prompt_handler::PromptHandler;
pub use runtime::{OrchestratorRuntime, RuntimeConfig};
pub use runtime_actions::{RuntimeActionService, RuntimeTaskSpec, RuntimeTerminalSpec};
pub use state::{OrchestratorRunState, OrchestratorState, SharedOrchestratorState};
pub use terminal_coordinator::TerminalCoordinator;
pub use types::*;

mod terminal_coordinator_test;
#[cfg(test)]
mod tests;
