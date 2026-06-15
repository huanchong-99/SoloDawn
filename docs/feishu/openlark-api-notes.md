# OpenLark 0.17.0 Rust SDK - Exact Public API Notes

This document captures the precise public API for openlark 0.17.0, extracted directly from source in the cargo cache at:
- `C:\Users\Administrator\.cargo\registry\src\index.crates.io-1949cf8c6b5b557f\openlark-0.17.0\`
- Sub-crates: `openlark-core`, `openlark-client`, `openlark-communication`, `openlark-auth`

**Target consumer:** Dependent crate with `openlark = { features=["communication","websocket"] }`

---

## 1. Config Builder

### Module Path
- **From umbrella crate:** `open_lark::CoreConfig` (re-exported from openlark_core)
- **Direct path:** `openlark_core::config::Config`
- **Actual source:** `openlark-core-0.17.0/src/config.rs`

### Builder Type & Methods

```rust
pub struct ConfigBuilder {
    app_id: Option<String>,
    app_secret: Option<String>,
    base_url: Option<String>,
    enable_token_cache: Option<bool>,
    req_timeout: Option<Duration>,
    max_response_size: Option<u64>,
    retry_count: Option<u32>,
    enable_log: Option<bool>,
}

impl ConfigBuilder {
    pub fn app_id(mut self, app_id: impl Into<String>) -> Self
    pub fn app_secret(mut self, app_secret: impl Into<String>) -> Self
    pub fn base_url(mut self, base_url: impl Into<String>) -> Self
    pub fn enable_token_cache(mut self, enable: bool) -> Self
    pub fn req_timeout(mut self, timeout: Duration) -> Self
    pub fn max_response_size(mut self, size: u64) -> Self
    pub fn retry_count(mut self, count: u32) -> Self
    pub fn enable_log(mut self, enable: bool) -> Self
    
    // Build - returns Config directly (no Result wrapper!)
    pub fn build(self) -> Config
}

pub struct Config {
    inner: Arc<ConfigInner>,  // Zero-copy Arc wrapper
}

impl Config {
    pub fn builder() -> ConfigBuilder
    pub fn app_id(&self) -> &str
    pub fn app_secret(&self) -> &str
    pub fn base_url(&self) -> &str
    pub fn req_timeout(&self) -> Option<Duration>
    pub fn enable_token_cache(&self) -> bool
}
```

### Defaults
- `app_id`: `""`
- `app_secret`: `""`
- `base_url`: `"https://open.feishu.cn"`
- `enable_token_cache`: `true`
- `req_timeout`: `None`
- `max_response_size`: `100 * 1024 * 1024` (100MB)
- `retry_count`: `3`
- `enable_log`: `true`

### Exact Usage
```rust
let config: CoreConfig = CoreConfig::builder()
    .app_id("test_app")
    .app_secret("test_secret")
    .build();
```

---

## 2. WebSocket Long-Connection Client

### Client Type & Module Path

**Type:** `open_lark::ws_client::LarkWsClient`  
**Module:** `open_lark::ws_client` (re-exported from openlark_client feature="websocket")  
**Source:** `openlark-client-0.17.0/src/ws_client/client.rs`

### Open Method Signature

```rust
impl LarkWsClient {
    pub async fn open(
        config: std::sync::Arc<crate::config::Config>,
        event_handler: EventDispatcherHandler,
    ) -> WsClientResult<()>
}

pub type WsClientResult<T> = Result<T, WsClientError>;
```

### Critical Details

1. **Config must be Arc-wrapped:** `Arc::new(config)` required
2. **Blocking:** Runs indefinitely until connection closes or error
3. **Return:** `Ok(())` on normal close, `Err(WsClientError)` on failure

---

## 3. Event Handler Registration

### Handler Trait

```rust
pub trait EventHandler: Send + Sync + 'static {
    fn handle(&self, payload: &[u8]) -> EventHandlerResult;
}

type EventHandlerResult = Result<(), Box<dyn std::error::Error + Send + Sync>>;
```

### EventDispatcherHandler

```rust
pub struct EventDispatcherHandler {
    payload_tx: Option<mpsc::UnboundedSender<Vec<u8>>>,
    raw_handlers: HashMap<String, Arc<dyn EventHandler>>,
}

impl EventDispatcherHandler {
    pub const RAW_EVENT_KEY: &'static str = "raw";
    
    pub fn builder() -> Self
    pub fn build(self) -> Self
    pub fn payload_sender(mut self, payload_tx: mpsc::UnboundedSender<Vec<u8>>) -> Self
    pub fn register_raw<S, H>(mut self, key: S, handler: H) -> Result<Self, String>
    where
        S: Into<String>,
        H: EventHandler,
}
```

### Handler Registration Details

1. **Key types:**
   - `"raw"` → All events
   - `"im.message.receive_v1"` → Only matching event_type

2. **Payload received:**
   - Raw bytes (`&[u8]`), NOT parsed
   - JSON format

3. **Error handling:**
   - `.register_raw()` returns `Result<Self, String>`

### Exact Usage
```rust
use open_lark::ws_client::{EventDispatcherHandler, EventHandler};
use tokio::sync::mpsc;

struct MyEventHandler;

impl EventHandler for MyEventHandler {
    fn handle(&self, payload: &[u8]) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let envelope: EventEnvelope = serde_json::from_slice(payload)?;
        println!("Event: {}", envelope.header.event_type);
        Ok(())
    }
}

let (payload_tx, payload_rx) = mpsc::unbounded_channel::<Vec<u8>>();

let event_handler = EventDispatcherHandler::builder()
    .payload_sender(payload_tx)
    .register_raw("im.message.receive_v1", MyEventHandler)?
    .build();

LarkWsClient::open(Arc::new(config), event_handler).await?;
```

---

## 4. IM Message Send & Reply

### Send Message (Create)

```rust
pub struct CreateMessageRequest {
    config: Config,
    receive_id_type: Option<ReceiveIdType>,
}

impl CreateMessageRequest {
    pub fn new(config: Config) -> Self
    pub fn receive_id_type(mut self, receive_id_type: ReceiveIdType) -> Self
    pub async fn execute(self, body: CreateMessageBody) -> SDKResult<serde_json::Value>
}

pub struct CreateMessageBody {
    pub receive_id: String,
    pub msg_type: String,
    pub content: String,  // JSON string
    pub uuid: Option<String>,
}

pub enum ReceiveIdType {
    OpenId,
    UnionId,
    UserId,
    Email,
    ChatId,
}
```

**Return:** `SDKResult<serde_json::Value>` (JSON with message_id)

### Reply Message

```rust
pub struct ReplyMessageRequest {
    config: Config,
    message_id: String,
}

impl ReplyMessageRequest {
    pub fn new(config: Config) -> Self
    pub fn message_id(mut self, message_id: impl Into<String>) -> Self
    pub async fn execute(self, body: ReplyMessageBody) -> SDKResult<serde_json::Value>
}

pub struct ReplyMessageBody {
    pub content: String,
    pub msg_type: String,
    pub reply_in_thread: Option<bool>,
    pub uuid: Option<String>,
}
```

### Usage

```rust
use open_lark::communication::im::v1::message::{
    create::{CreateMessageRequest, CreateMessageBody},
    models::ReceiveIdType,
};
use serde_json::json;

// SEND
let body = CreateMessageBody {
    receive_id: "ou_xxx".to_string(),
    msg_type: "text".to_string(),
    content: json!({ "text": "Hello" }).to_string(),
    uuid: None,
};

let response = CreateMessageRequest::new(config)
    .receive_id_type(ReceiveIdType::OpenId)
    .execute(body)
    .await?;

// REPLY
let reply_body = ReplyMessageBody {
    content: json!({ "text": "Reply" }).to_string(),
    msg_type: "text".to_string(),
    reply_in_thread: None,
    uuid: None,
};

ReplyMessageRequest::new(config)
    .message_id("om_xxx")
    .execute(reply_body)
    .await?;
```

---

## 5. Re-exports from Root Crate

When depending on `openlark = { features=["communication","websocket"] }`:

```rust
// Configuration & Client
use open_lark::{Client, ClientBuilder, Config, CoreConfig, Error, Result};

// WebSocket + Event
use open_lark::ws_client::{LarkWsClient, EventDispatcherHandler, EventHandler};

// Communication Module
use open_lark::communication;
use open_lark::communication::im::v1::message::{
    create::{CreateMessageRequest, CreateMessageBody},
    reply::{ReplyMessageRequest, ReplyMessageBody},
    models::ReceiveIdType,
};

// Error Types
use open_lark::{Error, Result, CoreError, ErrorCode};

// Prelude (recommended)
use open_lark::prelude::*;
```

---

## Complete Adapter Skeleton

```rust
use open_lark::prelude::*;
use open_lark::ws_client::{LarkWsClient, EventDispatcherHandler, EventHandler};
use open_lark::communication::im::v1::message::{
    create::{CreateMessageBody, CreateMessageRequest},
    reply::{ReplyMessageBody, ReplyMessageRequest},
    models::ReceiveIdType,
};
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // === 1. Build Config ===
    let config = Config::builder()
        .app_id(std::env::var("OPENLARK_APP_ID")?)
        .app_secret(std::env::var("OPENLARK_APP_SECRET")?)
        .base_url("https://open.feishu.cn")
        .timeout(Duration::from_secs(30))
        .build();

    // === 2. Create Event Handler ===
    let (payload_tx, _payload_rx) = mpsc::unbounded_channel::<Vec<u8>>();

    struct MyRawEventHandler;

    impl EventHandler for MyRawEventHandler {
        fn handle(&self, payload: &[u8]) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
            let envelope: EventEnvelope = serde_json::from_slice(payload)?;
            println!("Received event: {}", envelope.header.event_type);
            Ok(())
        }
    }

    #[derive(Deserialize)]
    struct EventEnvelope {
        header: EventHeader,
    }

    #[derive(Deserialize)]
    struct EventHeader {
        event_type: String,
    }

    // === 3. Register Handler ===
    let event_handler = EventDispatcherHandler::builder()
        .payload_sender(payload_tx)
        .register_raw("im.message.receive_v1", MyRawEventHandler)?
        .build();

    // === 4. Open WebSocket (blocking) ===
    LarkWsClient::open(Arc::new(config), event_handler).await?;

    Ok(())
}
```

---

## Quick Reference: Import Paths

```rust
use open_lark::{Config, CoreConfig, Client};
use open_lark::ws_client::{LarkWsClient, EventDispatcherHandler, EventHandler};
use open_lark::communication::im::v1::message::{
    create::{CreateMessageRequest, CreateMessageBody},
    reply::{ReplyMessageRequest, ReplyMessageBody},
    models::ReceiveIdType,
};
use open_lark::prelude::*;
```

---

## Key Gotchas

1. **Config.build() returns Config directly** (not Result) with defaults applied
2. **Config must be Arc-wrapped** for WebSocket (`Arc::new(config)`)
3. **Payload is raw bytes** - must deserialize manually
4. **receive_id_type is required** before execute()
5. **Content is JSON string** - pass `json!({...}).to_string()`
6. **Response is serde_json::Value** - extract fields manually
