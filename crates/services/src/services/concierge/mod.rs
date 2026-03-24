//! Concierge Agent: session-scoped AI assistant for GitCortex.
//!
//! Provides a unified conversational interface across Feishu and Web UI,
//! capable of creating projects, planning workflows, navigating tasks,
//! and delegating to the per-workflow orchestrator once running.

mod agent;
mod notifications;
mod prompt;
mod sync;
mod tools;

pub use agent::ConciergeAgent;
pub use notifications::push_workflow_completion;
pub use sync::{ConciergeBroadcaster, ConciergeEvent};
