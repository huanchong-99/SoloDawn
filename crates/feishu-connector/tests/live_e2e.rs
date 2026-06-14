//! Live end-to-end tests against a REAL Feishu app. Ignored by default.
//!
//! These automate PRD §9.1 M2 acceptance (send / receive) down to a single
//! command: the owner only provides real credentials via env vars instead of
//! clicking through the UI and eyeballing.
//!
//!   FEISHU_TEST_APP_ID=cli_xxx \
//!   FEISHU_TEST_APP_SECRET=xxx \
//!   FEISHU_TEST_CHAT_ID=oc_xxx \
//!   RUST_MIN_STACK=268435456 \
//!   cargo test -p feishu-connector --test live_e2e -- --ignored --nocapture
//!
//! Prereqs (PRD §8.1): a PUBLISHED enterprise self-built app, long-connection
//! mode enabled, `im.message.receive_v1` subscribed, `im:message` /
//! `im:message:send_as_bot` scopes granted, and the bot added to the target
//! chat (FEISHU_TEST_CHAT_ID). Optional: FEISHU_TEST_BASE_URL (defaults to
//! https://open.feishu.cn; use https://open.larksuite.com for Lark).

use std::sync::Arc;
use std::time::Duration;

use feishu_connector::events::parse_message_event;
use feishu_connector::messages::FeishuMessenger;
use feishu_connector::sdk::FeishuClient;
use feishu_connector::types::FeishuConfig;

fn env_or_panic(key: &str) -> String {
    std::env::var(key)
        .unwrap_or_else(|_| panic!("live_e2e requires env var {key} (see file header for usage)"))
}

fn test_config() -> FeishuConfig {
    FeishuConfig {
        app_id: env_or_panic("FEISHU_TEST_APP_ID"),
        app_secret: env_or_panic("FEISHU_TEST_APP_SECRET"),
        base_url: std::env::var("FEISHU_TEST_BASE_URL")
            .unwrap_or_else(|_| "https://open.feishu.cn".to_string()),
    }
}

/// Outbound text: send a real message to FEISHU_TEST_CHAT_ID and assert a
/// provider message_id is returned. Fully automated given credentials.
#[tokio::test]
#[ignore = "requires a real Feishu app; run with --ignored + FEISHU_TEST_* env"]
async fn live_send_text() {
    let chat_id = env_or_panic("FEISHU_TEST_CHAT_ID");
    let (client, _rx) = FeishuClient::new(test_config());
    let messenger = FeishuMessenger::new(client.sdk_config());
    let msg_id = messenger
        .send_text(&chat_id, "✅ SoloDawn openlark migration live E2E: send_text")
        .await
        .expect("send_text should succeed against the real Feishu API");
    assert!(!msg_id.is_empty(), "expected a non-empty provider message_id");
    eprintln!("live_send_text OK -> message_id={msg_id}");
}

/// Outbound card: send an interactive card and assert a message_id is returned.
#[tokio::test]
#[ignore = "requires a real Feishu app; run with --ignored + FEISHU_TEST_* env"]
async fn live_send_card() {
    let chat_id = env_or_panic("FEISHU_TEST_CHAT_ID");
    let (client, _rx) = FeishuClient::new(test_config());
    let messenger = FeishuMessenger::new(client.sdk_config());
    let card = serde_json::json!({
        "config": { "wide_screen_mode": true },
        "elements": [
            { "tag": "div", "text": { "tag": "lark_md", "content": "**SoloDawn** live E2E card ✅" } }
        ]
    });
    let msg_id = messenger
        .send_card(&chat_id, &card)
        .await
        .expect("send_card should succeed against the real Feishu API");
    assert!(!msg_id.is_empty(), "expected a non-empty provider message_id");
    eprintln!("live_send_card OK -> message_id={msg_id}");
}

/// Inbound: open the long-connection and wait up to 90s for ONE inbound text
/// message. The operator must send a message to the bot during the window.
/// Verifies the full receive pipeline (WS -> normalize -> parse_message_event).
#[tokio::test]
#[ignore = "interactive: operator must send a message to the bot within 90s"]
async fn live_receive_one_message() {
    let (client, mut rx) = FeishuClient::new(test_config());
    let client = Arc::new(client);
    let conn = client.clone();
    let handle = tokio::spawn(async move {
        let _ = conn.connect().await;
    });

    eprintln!("Connected to Feishu long-connection. Send a TEXT message to the bot within 90s...");
    let ev = tokio::time::timeout(Duration::from_secs(90), rx.recv())
        .await
        .expect("timed out waiting for an inbound event (did you send a message? is im.message.receive_v1 subscribed?)")
        .expect("event channel closed unexpectedly");

    let msg = parse_message_event(&ev).expect("inbound event should parse into a ReceivedMessage");
    assert!(!msg.chat_id.is_empty(), "chat_id must be non-empty");
    assert!(!msg.message_id.is_empty(), "message_id must be non-empty");
    eprintln!(
        "live_receive_one_message OK -> chat_id={} message_id={} type={}",
        msg.chat_id, msg.message_id, msg.message_type
    );
    handle.abort();
}
