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

// ---------------------------------------------------------------------------
// TelegramConnector
// ---------------------------------------------------------------------------

/// Outbound Telegram message sender via the Bot API.
///
/// Wraps the existing webhook-based Telegram integration into the
/// [`ChatConnector`] trait so it can be used interchangeably with other
/// chat providers.
pub struct TelegramConnector {
    bot_token: String,
    http_client: reqwest::Client,
}

impl TelegramConnector {
    /// Create a new connector.
    ///
    /// `bot_token` is the Telegram Bot API token (from `@BotFather`).
    pub fn new(bot_token: String) -> Self {
        Self {
            bot_token,
            http_client: reqwest::Client::new(),
        }
    }

    /// Try to create a connector from the `GITCORTEX_TELEGRAM_BOT_TOKEN`
    /// environment variable. Returns `None` when the variable is unset.
    pub fn from_env() -> Option<Self> {
        std::env::var("GITCORTEX_TELEGRAM_BOT_TOKEN")
            .ok()
            .filter(|t| !t.is_empty())
            .map(Self::new)
    }

    fn api_url(&self, method: &str) -> String {
        format!("https://api.telegram.org/bot{}/{}", self.bot_token, method)
    }
}

#[async_trait]
impl ChatConnector for TelegramConnector {
    async fn send_message(&self, conversation_id: &str, content: &str) -> anyhow::Result<String> {
        let resp = self
            .http_client
            .post(self.api_url("sendMessage"))
            .json(&serde_json::json!({
                "chat_id": conversation_id,
                "text": content,
            }))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        if resp["ok"].as_bool() != Some(true) {
            let desc = resp["description"].as_str().unwrap_or("unknown error");
            anyhow::bail!("Telegram API error: {}", desc);
        }
        let message_id = resp["result"]["message_id"]
            .as_i64()
            .ok_or_else(|| anyhow::anyhow!("No message_id in response"))?
            .to_string();
        Ok(message_id)
    }

    async fn send_reply(
        &self,
        conversation_id: &str,
        message_id: &str,
        content: &str,
    ) -> anyhow::Result<String> {
        let resp = self
            .http_client
            .post(self.api_url("sendMessage"))
            .json(&serde_json::json!({
                "chat_id": conversation_id,
                "text": content,
                "reply_to_message_id": message_id.parse::<i64>().unwrap_or(0),
            }))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        let reply_id = resp["result"]["message_id"]
            .as_i64()
            .map(|id| id.to_string())
            .unwrap_or_default();
        Ok(reply_id)
    }

    fn provider_name(&self) -> &'static str {
        "telegram"
    }

    fn is_connected(&self) -> bool {
        // Telegram uses stateless HTTP calls; always considered "connected"
        // as long as the bot token is present.
        true
    }
}
