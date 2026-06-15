pub mod events;
pub mod messages;
pub mod reconnect;
pub mod types;
pub mod sdk;

/// Backwards-compatible module path: `feishu_connector::client::FeishuClient`
/// now resolves to the openlark-backed adapter in [`crate::sdk`].
pub mod client {
    pub use crate::sdk::FeishuClient;
}
