use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FeishuEvent {
    pub schema: Option<String>,
    pub header: Option<EventHeader>,
    pub event: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EventHeader {
    pub event_id: String,
    pub event_type: String,
    pub create_time: Option<String>,
    pub token: Option<String>,
    pub app_id: Option<String>,
    pub tenant_key: Option<String>,
}

#[derive(Debug, Clone)]
pub struct ReceivedMessage {
    pub message_id: String,
    pub chat_id: String,
    pub chat_type: String,
    pub sender_open_id: String,
    pub message_type: String,
    pub content: String,
}

/// Parse `im.message.receive_v1` event into ReceivedMessage
pub fn parse_message_event(event: &FeishuEvent) -> anyhow::Result<ReceivedMessage> {
    let evt = event
        .event
        .as_ref()
        .ok_or_else(|| anyhow::anyhow!("Missing event payload"))?;

    let message = &evt["message"];
    let sender = &evt["sender"];

    // G32-013: Use ok_or_else for critical fields that downstream code depends on.
    // An empty chat_id or message_id would cause silent failures in message routing.
    let chat_id = message["chat_id"]
        .as_str()
        .filter(|s| !s.is_empty())
        .ok_or_else(|| anyhow::anyhow!("Missing or empty chat_id in Feishu message event"))?
        .to_string();

    let message_id = message["message_id"]
        .as_str()
        .filter(|s| !s.is_empty())
        .ok_or_else(|| anyhow::anyhow!("Missing or empty message_id in Feishu message event"))?
        .to_string();

    Ok(ReceivedMessage {
        message_id,
        chat_id,
        chat_type: message["chat_type"]
            .as_str()
            .unwrap_or_default()
            .to_string(),
        sender_open_id: sender["sender_id"]["open_id"]
            .as_str()
            .unwrap_or_default()
            .to_string(),
        message_type: message["message_type"]
            .as_str()
            .unwrap_or_default()
            .to_string(),
        content: message["content"]
            .as_str()
            .unwrap_or_default()
            .to_string(),
    })
}

/// Extract plain text from message content JSON (e.g. `{"text":"hello"}`)
pub fn parse_text_content(content_json: &str) -> String {
    serde_json::from_str::<serde_json::Value>(content_json)
        .ok()
        .and_then(|v| v["text"].as_str().map(|s| s.to_string()))
        .unwrap_or_default()
}
