use anyhow::Result;
use reqwest::Client;
use std::sync::Arc;

use crate::auth::FeishuAuth;

pub struct FeishuMessenger {
    auth: Arc<FeishuAuth>,
    http_client: Client,
    base_url: String,
}

impl FeishuMessenger {
    pub fn new(auth: Arc<FeishuAuth>, base_url: String) -> Self {
        Self {
            auth,
            http_client: Client::new(),
            base_url,
        }
    }

    /// Send a text message to a chat
    pub async fn send_text(&self, chat_id: &str, text: &str) -> Result<String> {
        let token = self.auth.get_tenant_token().await?;
        let url = format!(
            "{}/open-apis/im/v1/messages?receive_id_type=chat_id",
            self.base_url
        );
        let resp = self
            .http_client
            .post(&url)
            .header("Authorization", format!("Bearer {}", token))
            .json(&serde_json::json!({
                "receive_id": chat_id,
                "msg_type": "text",
                "content": serde_json::json!({"text": text}).to_string(),
            }))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        // G32-005: Check Feishu API response code
        let code = resp["code"].as_i64().unwrap_or(-1);
        if code != 0 {
            let msg = resp["msg"].as_str().unwrap_or("unknown error");
            anyhow::bail!("Feishu send_text failed: code={code}, msg={msg}");
        }

        let msg_id = resp["data"]["message_id"]
            .as_str()
            .unwrap_or_default()
            .to_string();
        Ok(msg_id)
    }

    /// Reply to a specific message with text
    pub async fn reply_text(&self, message_id: &str, text: &str) -> Result<String> {
        let token = self.auth.get_tenant_token().await?;
        let url = format!(
            "{}/open-apis/im/v1/messages/{}/reply",
            self.base_url, message_id
        );
        let resp = self
            .http_client
            .post(&url)
            .header("Authorization", format!("Bearer {}", token))
            .json(&serde_json::json!({
                "msg_type": "text",
                "content": serde_json::json!({"text": text}).to_string(),
            }))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        // G32-005: Check Feishu API response code
        let code = resp["code"].as_i64().unwrap_or(-1);
        if code != 0 {
            let msg = resp["msg"].as_str().unwrap_or("unknown error");
            anyhow::bail!("Feishu reply_text failed: code={code}, msg={msg}");
        }

        let msg_id = resp["data"]["message_id"]
            .as_str()
            .unwrap_or_default()
            .to_string();
        Ok(msg_id)
    }

    /// List chats the bot belongs to, returning the first chat_id found.
    pub async fn first_bot_chat_id(&self) -> Result<Option<String>> {
        let token = self.auth.get_tenant_token().await?;
        let url = format!(
            "{}/open-apis/im/v1/chats?page_size=1",
            self.base_url
        );
        let resp = self
            .http_client
            .get(&url)
            .header("Authorization", format!("Bearer {}", token))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        let code = resp["code"].as_i64().unwrap_or(-1);
        if code != 0 {
            let msg = resp["msg"].as_str().unwrap_or("unknown error");
            tracing::warn!("Feishu list_chats failed: code={code}, msg={msg}");
            return Ok(None);
        }

        let chat_id = resp["data"]["items"]
            .as_array()
            .and_then(|items| items.first())
            .and_then(|item| item["chat_id"].as_str())
            .map(|s| s.to_string());

        Ok(chat_id)
    }

    /// Send an interactive card message to a chat
    pub async fn send_card(&self, chat_id: &str, card: &serde_json::Value) -> Result<String> {
        let token = self.auth.get_tenant_token().await?;
        let url = format!(
            "{}/open-apis/im/v1/messages?receive_id_type=chat_id",
            self.base_url
        );
        let resp = self
            .http_client
            .post(&url)
            .header("Authorization", format!("Bearer {}", token))
            .json(&serde_json::json!({
                "receive_id": chat_id,
                "msg_type": "interactive",
                "content": card.to_string(),
            }))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        // G32-005: Check Feishu API response code
        let code = resp["code"].as_i64().unwrap_or(-1);
        if code != 0 {
            let msg = resp["msg"].as_str().unwrap_or("unknown error");
            anyhow::bail!("Feishu send_card failed: code={code}, msg={msg}");
        }

        let msg_id = resp["data"]["message_id"]
            .as_str()
            .unwrap_or_default()
            .to_string();
        Ok(msg_id)
    }
}
