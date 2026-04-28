#![allow(clippy::large_futures)]
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
pub mod resilient_llm;
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
    LLMClient, OpenAICompatibleClient, build_terminal_completion_prompt,
    create_claude_code_native_client, create_llm_client,
};
pub use message_bus::{BusMessage, MessageBus, SharedMessageBus};
pub use prompt_handler::PromptHandler;
pub use resilient_llm::{ProviderEvent, ProviderStatusReport, ResilientLLMClient};
pub use runtime::{OrchestratorRuntime, RuntimeConfig};
pub use runtime_actions::{RuntimeActionService, RuntimeTaskSpec, RuntimeTerminalSpec};
pub use state::{
    DebtPolicy, OrchestratorRunState, OrchestratorState, QualityProfile, SharedOrchestratorState,
    WorkflowArchetype, WorkflowStrategy,
};
pub use terminal_coordinator::TerminalCoordinator;
pub use types::*;

#[cfg(test)]
mod terminal_coordinator_test;
#[cfg(test)]
mod tests;
