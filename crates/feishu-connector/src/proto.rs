/// Generated protobuf types for Feishu WebSocket binary protocol (pbbp2).
pub mod pbbp2 {
    include!(concat!(env!("OUT_DIR"), "/feishu.ws.rs"));
}

pub use pbbp2::{Frame, Header};

/// Frame method: control frame (ping/pong)
pub const METHOD_CONTROL: i32 = 0;
/// Frame method: data frame (event/card)
pub const METHOD_DATA: i32 = 1;

/// Header key constants
pub const HEADER_TYPE: &str = "type";
pub const HEADER_MESSAGE_ID: &str = "message_id";
pub const HEADER_SUM: &str = "sum";
pub const HEADER_SEQ: &str = "seq";
pub const HEADER_BIZ_RT: &str = "biz_rt";

/// Message type values (used in "type" header)
pub const MSG_TYPE_PING: &str = "ping";
pub const MSG_TYPE_PONG: &str = "pong";
pub const MSG_TYPE_EVENT: &str = "event";
