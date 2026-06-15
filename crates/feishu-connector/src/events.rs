use serde::{Deserialize, Serialize};

/// Event type for incoming chat messages.
pub const EVENT_TYPE_MESSAGE: &str = "im.message.receive_v1";

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
        content: message["content"].as_str().unwrap_or_default().to_string(),
    })
}

/// Extract plain text from message content JSON (e.g. `{"text":"hello"}`)
pub fn parse_text_content(content_json: &str) -> String {
    serde_json::from_str::<serde_json::Value>(content_json)
        .ok()
        .and_then(|v| v["text"].as_str().map(|s| s.to_string()))
        .unwrap_or_default()
}

/// Normalize a raw payload delivered by the openlark SDK (`register_raw`) into a
/// [`FeishuEvent`]. Written defensively so it works whether the SDK forwards the
/// full `{schema?, header, event}` envelope (the observed shape — the SDK
/// dispatches by parsing `header.event_type`) or only the inner `event` body.
pub fn feishu_event_from_sdk_payload(bytes: &[u8]) -> anyhow::Result<FeishuEvent> {
    let v: serde_json::Value = serde_json::from_slice(bytes)?;
    if v.get("event").is_some() && v.get("header").is_some() {
        // Already a full envelope: deserialize directly (equivalent to legacy path).
        return Ok(serde_json::from_value(v)?);
    }
    // Only the inner event body (or missing header): wrap it so downstream
    // `parse_message_event` sees a well-formed im.message.receive_v1 event.
    Ok(FeishuEvent {
        schema: None,
        header: Some(EventHeader {
            event_id: String::new(),
            event_type: EVENT_TYPE_MESSAGE.to_string(),
            create_time: None,
            token: None,
            app_id: None,
            tenant_key: None,
        }),
        event: Some(v.get("event").cloned().unwrap_or(v)),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Canonical full `im.message.receive_v1` envelope as Feishu/openlark deliver it.
    /// NOTE: replace with a real captured first frame once available from a live
    /// `cli_xxx` app (see PRD §5.3 / §11.1); the shape below matches the documented
    /// schema and the SDK's `header.event_type`-based dispatch.
    const FULL_ENVELOPE: &str = r#"{
        "schema": "2.0",
        "header": {
            "event_id": "ev_123",
            "event_type": "im.message.receive_v1",
            "create_time": "1700000000000",
            "token": "tok",
            "app_id": "cli_abc",
            "tenant_key": "tk"
        },
        "event": {
            "sender": { "sender_id": { "open_id": "ou_sender" } },
            "message": {
                "message_id": "om_123",
                "chat_id": "oc_456",
                "chat_type": "p2p",
                "message_type": "text",
                "content": "{\"text\":\"hello\"}"
            }
        }
    }"#;

    /// Inner `event` body only (defensive branch): no `header`/`event` wrapper keys.
    const INNER_ONLY: &str = r#"{
        "sender": { "sender_id": { "open_id": "ou_sender" } },
        "message": {
            "message_id": "om_789",
            "chat_id": "oc_789",
            "chat_type": "group",
            "message_type": "text",
            "content": "{\"text\":\"hi\"}"
        }
    }"#;

    #[test]
    fn full_envelope_normalizes_and_parses() {
        let ev = feishu_event_from_sdk_payload(FULL_ENVELOPE.as_bytes()).unwrap();
        assert_eq!(
            ev.header.as_ref().unwrap().event_type,
            EVENT_TYPE_MESSAGE
        );
        let msg = parse_message_event(&ev).unwrap();
        assert_eq!(msg.chat_id, "oc_456");
        assert_eq!(msg.message_id, "om_123");
        assert_eq!(parse_text_content(&msg.content), "hello");
    }

    #[test]
    fn inner_only_body_is_wrapped_and_parses() {
        let ev = feishu_event_from_sdk_payload(INNER_ONLY.as_bytes()).unwrap();
        // Synthesized header carries the message event type.
        assert_eq!(
            ev.header.as_ref().unwrap().event_type,
            EVENT_TYPE_MESSAGE
        );
        let msg = parse_message_event(&ev).unwrap();
        assert_eq!(msg.chat_id, "oc_789");
        assert_eq!(msg.message_id, "om_789");
    }

    #[test]
    fn malformed_payload_errors() {
        assert!(feishu_event_from_sdk_payload(b"not json").is_err());
    }
}
