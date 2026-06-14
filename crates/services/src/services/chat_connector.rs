//! Provider-agnostic chat connector trait and implementations.
//!
//! Defines a common interface for sending messages to external chat platforms
//! (Telegram, Feishu, etc.) and provides concrete implementations for each.

use async_trait::async_trait;

/// A provider-agnostic interface for sending messages to external chat platforms.
#[async_trait]
pub trait ChatConnector: Send + Sync {
    /// Send a message to a conversation. Returns the provider message ID.
    async fn send_message(&self, conversation_id: &str, content: &str) -> anyhow::Result<String>;

    /// Reply to a specific message in a conversation. Returns the provider message ID.
    async fn send_reply(
        &self,
        conversation_id: &str,
        message_id: &str,
        content: &str,
    ) -> anyhow::Result<String>;

    /// The provider name (e.g. "telegram", "feishu").
    fn provider_name(&self) -> &str;

    /// Whether the connector is currently connected / operational.
    fn is_connected(&self) -> bool;
}

