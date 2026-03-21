use serde::{Deserialize, Serialize};

/// Configuration for connecting to Feishu
#[derive(Debug, Clone)]
pub struct FeishuConfig {
    pub app_id: String,
    pub app_secret: String,
    /// Base URL: https://open.feishu.cn or https://open.larksuite.com
    pub base_url: String,
}

/// WebSocket endpoint response from Feishu
#[derive(Debug, Deserialize)]
pub struct WsEndpointResponse {
    pub code: i32,
    pub msg: String,
    pub data: Option<WsEndpointData>,
}

#[derive(Debug, Deserialize)]
pub struct WsEndpointData {
    #[serde(rename = "URL", default)]
    pub url: String,
    #[serde(rename = "ClientConfig")]
    pub client_config: Option<ClientConfig>,
}

/// Client configuration received from Feishu (via endpoint or pong frames)
#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ClientConfig {
    #[serde(rename = "ReconnectCount", default)]
    pub reconnect_count: i32,
    #[serde(rename = "ReconnectInterval", default = "default_reconnect_interval")]
    pub reconnect_interval: u64,
    #[serde(rename = "ReconnectNonce", default)]
    pub reconnect_nonce: u64,
    #[serde(rename = "PingInterval", default = "default_ping_interval")]
    pub ping_interval: u64,
}

fn default_reconnect_interval() -> u64 {
    120
}

fn default_ping_interval() -> u64 {
    120
}

impl Default for ClientConfig {
    fn default() -> Self {
        Self {
            reconnect_count: -1,
            reconnect_interval: 120,
            reconnect_nonce: 0,
            ping_interval: 120,
        }
    }
}

/// Cached tenant access token
#[derive(Debug, Clone)]
pub struct CachedToken {
    pub token: String,
    pub expires_at: std::time::Instant,
}
