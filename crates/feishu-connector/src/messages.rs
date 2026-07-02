//! Outbound Feishu messaging, backed by the openlark SDK.
//!
//! `FeishuMessenger` keeps its public type name and method signatures; only the
//! internals change (legacy hand-rolled REST -> openlark `CreateMessageRequest` /
//! `ReplyMessageRequest`). Token acquisition/caching is handled by the SDK via
//! `Config { enable_token_cache: true }`.

use anyhow::Result;

use open_lark::CoreConfig;
use open_lark::communication::im::v1::message::{
    create::{CreateMessageBody, CreateMessageRequest},
    models::ReceiveIdType,
    reply::{ReplyMessageBody, ReplyMessageRequest},
};

pub struct FeishuMessenger {
    cfg: CoreConfig,
}

impl FeishuMessenger {
    /// Build a messenger from the SDK core config (carries base_url + token cache).
    pub fn new(cfg: CoreConfig) -> Self {
        Self { cfg }
    }

    /// Send a text message to a chat. Returns the provider message ID.
    pub async fn send_text(&self, chat_id: &str, text: &str) -> Result<String> {
        let resp = CreateMessageRequest::new(self.cfg.clone())
            .receive_id_type(ReceiveIdType::ChatId)
            .execute(text_body(chat_id, text))
            .await
            .map_err(|e| anyhow::anyhow!("Feishu send_text failed: {e}"))?;
        extract_message_id(&resp)
    }

    /// Reply to a specific message with text. Returns the provider message ID.
    pub async fn reply_text(&self, message_id: &str, text: &str) -> Result<String> {
        let resp = ReplyMessageRequest::new(self.cfg.clone())
            .message_id(message_id)
            .execute(reply_text_body(text))
            .await
            .map_err(|e| anyhow::anyhow!("Feishu reply_text failed: {e}"))?;
        extract_message_id(&resp)
    }

    /// Send an interactive card message to a chat. Returns the provider message ID.
    pub async fn send_card(&self, chat_id: &str, card: &serde_json::Value) -> Result<String> {
        let resp = CreateMessageRequest::new(self.cfg.clone())
            .receive_id_type(ReceiveIdType::ChatId)
            .execute(card_body(chat_id, card))
            .await
            .map_err(|e| anyhow::anyhow!("Feishu send_card failed: {e}"))?;
        extract_message_id(&resp)
    }

    /// Best-effort discovery of a default chat id.
    ///
    /// Fixed behaviour: returns `Ok(None)`. The caller (`planning_drafts`) treats
    /// `None` as "no default" and requires an explicit `chat_id`. No list-chats
    /// probe and no hand-rolled token path (token management belongs to the SDK).
    pub async fn first_bot_chat_id(&self) -> Result<Option<String>> {
        tracing::debug!("first_bot_chat_id: returning None (caller falls back to explicit chat_id)");
        Ok(None)
    }
}

/// Build the IM create body for a plain text message.
fn text_body(chat_id: &str, text: &str) -> CreateMessageBody {
    CreateMessageBody {
        receive_id: chat_id.to_string(),
        msg_type: "text".to_string(),
        content: serde_json::json!({ "text": text }).to_string(),
        uuid: None,
    }
}

/// Build the IM create body for an interactive card message.
fn card_body(chat_id: &str, card: &serde_json::Value) -> CreateMessageBody {
    CreateMessageBody {
        receive_id: chat_id.to_string(),
        msg_type: "interactive".to_string(),
        content: card.to_string(),
        uuid: None,
    }
}

/// Build the IM reply body for a plain text reply.
fn reply_text_body(text: &str) -> ReplyMessageBody {
    ReplyMessageBody {
        content: serde_json::json!({ "text": text }).to_string(),
        msg_type: "text".to_string(),
        reply_in_thread: None,
        uuid: None,
    }
}

/// Extract `message_id` from the SDK's JSON response.
///
/// Feishu returns HTTP 200 with a non-zero body `code` on logical failures
/// (e.g. 230002 "bot not in group", 230027 permission, 230020 rate-limit). The
/// SDK does not always turn these into transport errors, so `code` must be
/// inspected here — otherwise a rejected send would be reported as success with
/// an empty message id, silently swallowing the failure. Tolerates either a
/// `{ data: { message_id } }` envelope or a flattened `{ message_id }` shape.
fn extract_message_id(resp: &serde_json::Value) -> Result<String> {
    if let Some(code) = resp.get("code").and_then(|c| c.as_i64())
        && code != 0
    {
        let msg = resp
            .get("msg")
            .and_then(|m| m.as_str())
            .unwrap_or("unknown error");
        return Err(anyhow::anyhow!("Feishu send failed: code={code} msg={msg}"));
    }

    resp.get("data")
        .and_then(|d| d.get("message_id"))
        .and_then(|m| m.as_str())
        .or_else(|| resp.get("message_id").and_then(|m| m.as_str()))
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
        .ok_or_else(|| anyhow::anyhow!("Feishu send response missing message_id: {resp}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn text_body_has_text_msg_type_and_content() {
        let b = text_body("oc_chat123", "hello world");
        assert_eq!(b.receive_id, "oc_chat123");
        assert_eq!(b.msg_type, "text");
        let c: serde_json::Value = serde_json::from_str(&b.content).unwrap();
        assert_eq!(c["text"], "hello world");
        assert!(b.uuid.is_none());
    }

    #[test]
    fn card_body_is_interactive_with_card_json() {
        let card = serde_json::json!({ "config": { "wide_screen_mode": true }, "elements": [] });
        let b = card_body("oc_chat123", &card);
        assert_eq!(b.receive_id, "oc_chat123");
        assert_eq!(b.msg_type, "interactive");
        // content must be the card JSON serialized verbatim
        let c: serde_json::Value = serde_json::from_str(&b.content).unwrap();
        assert_eq!(c, card);
    }

    #[test]
    fn reply_body_has_text_msg_type_and_no_thread() {
        let b = reply_text_body("a reply");
        assert_eq!(b.msg_type, "text");
        let c: serde_json::Value = serde_json::from_str(&b.content).unwrap();
        assert_eq!(c["text"], "a reply");
        assert!(b.reply_in_thread.is_none());
        assert!(b.uuid.is_none());
    }

    #[test]
    fn extract_message_id_handles_both_envelopes() {
        let nested = serde_json::json!({ "code": 0, "data": { "message_id": "om_nested" } });
        assert_eq!(extract_message_id(&nested).unwrap(), "om_nested");
        let flat = serde_json::json!({ "message_id": "om_flat" });
        assert_eq!(extract_message_id(&flat).unwrap(), "om_flat");
        // code:0 but no message_id -> error (previously silently returned "")
        let missing = serde_json::json!({ "code": 0, "data": {} });
        assert!(extract_message_id(&missing).is_err());
        // Non-zero code: a Feishu logical failure returned with HTTP 200 must surface
        // as an error rather than a successful send with an empty id.
        let rejected =
            serde_json::json!({ "code": 230002, "msg": "bot not in group", "data": {} });
        assert!(extract_message_id(&rejected).is_err());
    }
}
