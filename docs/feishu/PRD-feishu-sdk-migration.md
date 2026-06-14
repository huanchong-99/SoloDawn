# PRD：飞书集成从自研 pbbp2 长连接迁移到维护中的 Rust SDK（openlark 0.17.0）

> 状态：已定稿，待 缓冲 验收（开发未启动）
> 目标受众：`/goal` 自主开发运行（本文档为"对照开发"清单，必须可被直接、无歧义地执行，开发过程中**不向 owner 提问**）
> 编写日期：2026-06-14
> 适用平台：Windows（硬约束，开发与运行均在 Windows）

---

## 面向 owner 的功能说明（白话）

做完之后，你（不懂技术的使用者）能看到的效果：

1. 打开设置页 → 填入飞书的 **App ID** 和 **App Secret** → 点保存 → 打开"启用飞书"开关。
2. 设置页会显示 **"已连接"**（绿色状态），不用做别的。
3. 在飞书里找到这个机器人，给它发一句话（比如"你好"）→ **机器人会在同一个会话里回你**。
4. 机器人还能发"卡片"（带按钮/排版的富消息），也能在掉线后**自动重连**，无需人工干预。

**完成的标准（owner 可以肉眼判定）**：

- 设置页填好 App ID/Secret 并启用后，状态显示"已连接"。
- 在飞书给机器人发文本消息，机器人**有回复**。
- 拔掉网线再插上（或重启路由），过一会儿状态自己变回"已连接"，再发消息仍能收到回复。

只要以上三条都满足，本次功能就算交付成功。下面的技术章节是给开发者/agent 看的实现清单。

---

## 1. 概述与目标

### 1.1 背景与现状（已用工具核实）

SoloDawn 当前的飞书集成位于 `crates/feishu-connector/`，是一个**逆向工程的手写飞书长连接（WebSocket）客户端**：

| 文件 | 职责 |
|---|---|
| `src/auth.rs` | `FeishuAuth`：`tenant_access_token` 缓存 + `acquire_ws_endpoint()`（`POST {base_url}/callback/ws/endpoint`，body `{AppID,AppSecret}`） |
| `src/client.rs` | `FeishuClient`：`tokio-tungstenite` WS + `prost` 编解码 `pbbp2::Frame` + ping 循环 + `FragmentCache` 分片重组 + 事件 ack（写回 `biz_rt`） |
| `src/proto.rs` + `build.rs` + `proto/pbbp2.proto` | prost 生成的 pbbp2（`Frame`/`Header` + `METHOD_*`/`HEADER_*`/`MSG_TYPE_*` 常量） |
| `src/messages.rs` | `FeishuMessenger`：REST `send_text`/`reply_text`/`send_card`/`first_bot_chat_id`（`/open-apis/im/v1/messages`） |
| `src/reconnect.rs` | `ReconnectPolicy`：指数退避重连策略 |
| `src/types.rs` | `FeishuConfig` / `ClientConfig` / `WsEndpointResponse` / `CachedToken` |
| `src/events.rs` | `FeishuEvent` JSON DTO + `parse_message_event` + `parse_text_content` + `EVENT_TYPE_MESSAGE` |

### 1.2 已核实的根因（为什么它从未跑通）

`proto/pbbp2.proto` 使用 `syntax = "proto3"`。proto3 **省略零值标量字段**，因此 prost 不会序列化值为 0 的 `SeqID`/`LogID`/`method`。`client.rs:33-43` 的 ping Frame 用 `..Default::default()` 构造，发出时**缺失 `SeqID`/`LogID`/`method`** → 飞书拒绝 → ping 超时断连 + ack 无效。

> 交叉验证：官方 `larksuite/oapi-sdk-go` 的 pbbp2 将 `SeqID/LogID/service/method` 标为 required 并始终发出。本仓 proto3 实现丢字段，是连接失败的直接根因。本次迁移即从根上消除它。

### 1.3 目标

1. **用维护中的社区 Rust SDK（openlark 0.17.0）替换 `crates/feishu-connector` 的传输内核**（删除 `auth.rs`/`client.rs`/`proto.rs`/`build.rs`/`pbbp2.proto`），由 SDK 提供正确的 pbbp2 帧（含 `SeqID/LogID/method`），**从根上消除 1.2 的 bug**。
2. **保留所有外部接缝**：`ChatConnector` trait、`FeishuService`、`Concierge` 路由、`SharedFeishuHandle::new_shared_handle()`（零参签名）、REST 路由、DB schema、加密、前端契约——全部不变。
3. **不引入 sidecar / CLI**；纯 in-process 库；rustls（非 native-tls）。
4. 在 Windows 上以 `RUST_MIN_STACK=256MB` + `cargo -j1` 可编译、可通过现有测试、可端到端收发消息。

---

## 0. 开发环境前置（一次性，必须先做）

> 本节是构建的**硬前置**。openlark 启用 rustls 的 aws-lc-rs 后端（见 §2、§6），其底层 `aws-lc-sys` 在 Windows MSVC 工具链上**编译期需要 NASM + CMake**。缺这两个工具，构建会直接报 `aws-lc-sys` 编译错误（找不到 `nasm`/`cmake`、汇编无法生成）。这是**已决定**的工具链事实，不是可选项。

### 0.1 已有前置（沿用 MEMORY 的 Windows dev setup）

- **protoc**（lark-websocket-protobuf 仍经它生成 pbbp2）。
- **LLVM / libclang**（bindgen 依赖；`setup-windows.ps1` 未含，需手装）。
- **sqlx-cli 固定 `0.8.6`**。
- 所有 cargo 全量代码生成命令（`cargo build` / `test --no-run` / `clippy`）必须带 **`RUST_MIN_STACK=268435456`（256MB）+ `-j1`**，否则会 ICE / OOM。

### 0.2 本次新增前置：NASM + CMake（aws-lc-rs 必需）

**默认用 winget 安装**（Windows 10/11 自带，唯一首选路径）：

```powershell
winget install --id NASM.NASM -e
winget install --id Kitware.CMake -e
```

仅当 winget 不可用时，再用以下任一兜底（命令等价）：

```powershell
# 兜底 1：scoop
scoop install nasm cmake

# 兜底 2：choco（管理员 PowerShell）
choco install nasm cmake -y
```

**验证（两条都必须成功打印版本号）**：

```powershell
nasm --version    # 例：NASM version 2.16.x
cmake --version   # 例：cmake version 3.2x.x
```

> **MSVC C++ 工具链（同样必需）**：`aws-lc-sys` 还需要 Visual Studio 2022 的 “Desktop development with C++” 工作负载（提供 `cl.exe`/`link.exe`）。请从 **“x64 Native Tools Command Prompt for VS 2022”** 运行 `cargo build`（或先初始化 VS 2022 环境），确保 `cl.exe`/`link.exe` 与 NASM 都在 PATH 上。否则即使装了 NASM/CMake，`aws-lc-sys` 仍可能找不到汇编器/编译器而失败。
> 装完若 `nasm`/`cmake` 仍提示找不到，重开一个终端让 PATH 生效；choco 装的 NASM 默认在 `C:\Program Files\NASM`，必要时把它加入 PATH。
> **可选逃生阀（仅 debug/非优化构建）**：设 `AWS_LC_SYS_NO_ASM=1` 可跳过 NASM，但性能大幅下降——**禁止用于 release 构建**。
> **不装的后果**：`cargo build` 在编译 `aws-lc-sys` 时失败（典型报错含 `Missing dependency: nasm` 或 `cmake` not found）。

---

## 2. 选型结论（已决定，无可选项）

### 2.1 是否存在官方 Rust SDK

**不存在。** 截至 2026-06-14：larksuite GitHub 组织只发布 Go/Java/Python/Node SDK + 已被用户拒绝的 Go CLI（`larksuite/cli`，需 Node，非 Rust 库）。官方服务端 SDK 概述文档仅列 Java/Python/Go/Node.js。因此选用维护中的社区 Rust SDK。

### 2.2 选定 SDK：openlark = "=0.17.0"

**选定 openlark（foxzool 的无连字符多 crate 工作区家族）v0.17.0，精确 pin。** 它是旧版连字符 `open-lark`（冻结于 0.14.0 / 2025-09-30，**禁止使用**）的维护后继。

- crates.io 包名 **`openlark`（无连字符）**；Rust 代码中 `use open_lark::...`（`[lib] name = "open_lark"`）。
- **禁用**旧的连字符 `open-lark 0.14.0`（冻结/被取代，旧 `register_p2_*` API）。
- 提供：长连接入站 `im.message.receive_v1`（`LarkWsClient::open` + `EventDispatcherHandler`）、出站文本/回复/交互卡片（`openlark-communication`）、`tenant_access_token` 管理（`openlark-auth`，由 `communication` 传递依赖）。
- 其 WS 内核是 SoloDawn 手写连接器的**逐功能复刻**（同 `/callback/ws/endpoint`、同 pbbp2 `Frame`，来自 `lark-websocket-protobuf 0.1.1`），并**正确发出** `SeqID/LogID/method`。

### 2.3 crate + features + 版本/pin

根 `[workspace.dependencies]` 新增（**就是这一行，普通 crates.io 依赖，无 fork、无 patch、无 vendoring**）：

```toml
openlark = { version = "=0.17.0", default-features = false, features = ["communication", "websocket"] }
```

`crates/feishu-connector/Cargo.toml` 使用 `openlark = { workspace = true }`。

- `websocket` → 拉入 `openlark-client` 的 `ws_client` + `lark-websocket-protobuf`（正确 pbbp2，消除 1.2 根因）+ `tokio-tungstenite`/`prost`/`reqwest`。
- `communication` → 拉入 IM REST 层（`im/v1` create+reply）**并传递依赖 `auth`**（token 管理/缓存）。
- 卡片：交互卡片 = `msg_type:"interactive"` + card JSON 走同一个 `CreateMessageRequest`，**无需** `cardkit` feature。

**版本：精确 pin `=0.17.0`**（API 面冻结）。理由：pre-1.0、单维护者（bus factor=1）、minor 间已有破坏性变更。精确 pin 冻结 PRD 编码所针对的 API 面，将来升级是经评审的主动行为。

> **feature 名（已定，照写即可）**：openlark 0.17.0 的 feature 名以本节为准：`features = ["communication", "websocket"]`、`default-features = false`。已核实 `websocket = [tokio-tungstenite, futures-util, lark-websocket-protobuf, prost, reqwest, log]`、`default = [auth, communication]`（umbrella `openlark` 的 `websocket = ["openlark-client/websocket"]`、`communication` 均存在）。直接照此写 `Cargo.toml`，无需再次复核。

### 2.4 被否决方案（一句话理由）

| 方案 | 为什么不用 |
|---|---|
| **feishu-sdk 0.1.2 (palpo-im)** | 已否决，本次迁移不采用：openlark 在成熟度/活跃度/下载量上胜出；如未来 openlark 出现阻断性问题再单独评审，与本 PRD 无关。 |
| **open-lark 0.14.0（旧连字符）** | 冻结/被取代（2025-09-30），缺 0.15-0.17 修复，旧 `register_p2_*` API。**禁用**。 |
| **lark-websocket-protobuf 0.1.1 / openlark-protocol 0.17.0** | 仅协议定义，无 client/auth/reconnect。淘汰。 |
| **larksuite-oapi-sdk-rs 0.1.1** | 2026-06 新发，下载量极低，WS 支持未验证。太不成熟。 |
| **larkrs-client 0.1.2** | 仅 REST/MCP，**无 WS 长连接入站**。淘汰。 |
| **nanobot 0.2.19** | 完整多渠道 AI 助手**应用**，非可嵌入库。 |
| **chyroc/lark-rs / lark-oapi / lark_bot_sdk* / lark-sdk(yanked) / lark-rs(2021)** | 废弃/占位/未发布/前 WS 时代/yanked。 |
| **larksuite/cli (Go) + oapi-sdk-go** | 用户明确拒绝（sidecar / 非 Rust 库）。 |

---

## 3. 非目标

1. **不引入飞书 CLI / 任何 sidecar 子进程**——纯 in-process 库。
2. **不改 DB schema**——`feishu_app_config`（`app_id`/`app_secret_encrypted`/`tenant_key`/`base_url`/`enabled`）、`system_settings.feishu_enabled`、`planning_draft.feishu_sync`+`feishu_chat_id`、`concierge.feishu_chat_id` 全部保留。零数据迁移。
3. **不改前端契约**——`feishuApi`（`getStatus`/`updateConfig`/`reconnect`/`testSend`/`testReceive`）及其 camelCase DTO 不变；`FeishuSettingsNew.tsx`/`FeishuChannelPanel.tsx`/`FeishuChannelContainer.tsx` 不动。
4. **不改 REST 契约**——`/feishu/{status,config,reconnect,test-send,test-receive}` 路由与请求/响应 DTO 不变。
5. **不改编排协议**——`BusMessage::TerminalMessage` 的 `[feishu:chat:sender] text` 前缀手把手交接不变。
6. **不改 `ChatConnector` trait** 与 `new_shared_handle()` 零参签名。

---

## 4. 总体架构与保留接缝

### 4.1 数据流（散文版"图"）

**入站**：飞书云 → `LarkWsClient::open`（SDK，pbbp2 解码/分片重组/ping）→ `EventDispatcherHandler` 把**原始 JSON 字节**送进 channel → 适配层 `feishu_event_from_sdk_payload()` 把 `Vec<u8>` 归一化成**既有的** `feishu_connector::events::FeishuEvent` → 经 `mpsc::Sender<FeishuEvent>` 进入 `FeishuService::process_events_inner` → `parse_message_event` → `ReceivedMessage` → slash 命令/模型选择/`ConciergeAgent`（**逻辑逐字节不变**）。

**出站**：`FeishuConnector`（`ChatConnector` impl）→ `FeishuMessenger::{send_text,reply_text,send_card}`（**签名不变**，内部改为调用 SDK 的 `CreateMessageRequest`/`ReplyMessageRequest`）→ 飞书 REST。

**重连**：`LarkWsClient::open()` **阻塞至断连后返回**（SDK 无内建自动重连）——这正是 SoloDawn 既有 `start_feishu_connector()` 的 `ReconnectPolicy` 退避循环（`main.rs:474-496`）所期待包裹的形态。**1:1 映射**：把 `service.start()` 内部的 `self.client.connect()` 替换为 SDK 的 `LarkWsClient::open(...)`，循环本身不变。

### 4.2 保留接缝清单（全部已用工具核实，路径精确，禁止改动）

| 接缝 | 位置 | 约束 |
|---|---|---|
| `ChatConnector` trait | `crates/services/src/services/chat_connector.rs` | provider 无关、不含 feishu 类型，**禁止改动** |
| `FeishuConnector` struct + `new(Arc<FeishuMessenger>, Arc<RwLock<bool>>)` | `feishu.rs:841-891` | 公开形状保留；impl 仅转发到 messenger |
| `FeishuService::new/from_db/start/messenger()/connected_flag()/set_event_broadcaster()` | `feishu.rs:47-131, 820-833` | 仅改 `new`（构造）与 `start`（connect 调用）少数行 |
| slash 命令 + Concierge 处理（仅消费 `ReceivedMessage`/`FeishuMessenger`） | `feishu.rs:213-817` | **不变** |
| `FeishuConfig { app_id, app_secret, base_url }` | `types.rs:5-10`，构造于 `feishu.rs:94-98` | **保留 struct 精确字段** |
| `FeishuEvent` / `EventHeader` / `ReceivedMessage` / `parse_message_event` / `parse_text_content` / `EVENT_TYPE_MESSAGE` | `events.rs` | DTO + 解析**整体保留**（仅**新增** §6 的归一化函数） |
| `FeishuMessenger`（`send_text`/`reply_text`/`send_card`/`first_bot_chat_id`） | `messages.rs` | **保留 type 名 + 4 个方法签名**；内部重写 |
| `ReconnectPolicy::{new(ClientConfig),next_delay,reset,update_config}` + `ClientConfig` | `reconnect.rs` + `types.rs:29-58` | **整文件保留**（外层重连归我们） |
| `SharedFeishuHandle` / `FeishuHandle{...}` / `new_shared_handle()` | `feishu_handle.rs` | **零参签名 + 字段名/类型逐字节不变** |
| `routes/feishu.rs`：`h.messenger.send_text`(L352)、`h.event_tx.subscribe`(L428)、`EVENT_TYPE_MESSAGE`/`parse_message_event`/`parse_text_content`(L438-443)、`h.last_chat_id` | `crates/server/src/routes/feishu.rs` | **逻辑不变** |
| `routes/planning_drafts.rs`：`messenger.send_text`(149,913)、`messenger.first_bot_chat_id`(875) | — | **不变** |
| `start_feishu_connector` / `decrypt_feishu_secret` | `main.rs:422-499` | 结构不变；仅改 import 行 + `service.start()` 内核 |

> **核实校正**：`lib.rs` 当前**没有 `pub use` 再导出**，只有 `pub mod auth/client/events/messages/proto/reconnect/types;`。`feishu.rs:11-16` 用的是**模块路径导入**（`feishu_connector::client::FeishuClient` 等）。因此删除/替换文件时，必须保证这些**模块路径仍可解析**（见 §5.2 的 shim）。

---

## 5. 详细改造步骤（逐文件、可执行）

> 顺序：先 M0 依赖核验（§6），再做下列源码改造。所有 cargo 命令在 Windows 下用 `RUST_MIN_STACK=268435456` + `-j1`。

### 5.1 `crates/feishu-connector/Cargo.toml`（ADAPT）

**删除**：`tokio-tungstenite`（`Cargo.toml:8` 的 0.26+native-tls，整条删除而非 bump——只有 server crate 那一处 bump 到 0.29）、`prost`、`bytes`、`flate2`、`futures-util`、`url`、`[build-dependencies] prost-build`。
**保留**：`tokio`、`reqwest`、`serde`、`serde_json`、`tracing`、`anyhow`、`uuid`、`rand`、`tokio-util`。
**新增**：`openlark = { workspace = true }`。

```toml
[package]
name = "feishu-connector"
version = "0.1.0"
edition = "2024"

[dependencies]
tokio = { workspace = true }
reqwest = { workspace = true }
serde = { workspace = true }
serde_json = { workspace = true }
tracing = { workspace = true }
anyhow = { workspace = true }
uuid = { version = "1.0", features = ["v4"] }
rand = "0.8"
tokio-util = "0.7"
openlark = { workspace = true }
# 删除整段 [build-dependencies]（prost-build）
```

### 5.2 `crates/feishu-connector/src/lib.rs`（ADAPT）

去掉 `pub mod proto;`、`pub mod auth;`、`pub mod client;`（这三文件删除）。**新增 `pub mod sdk;`**，并提供 `client` 路径的兼容 shim，使 `feishu.rs:12` 的 `use feishu_connector::client::FeishuClient` 仍解析：

```rust
pub mod events;
pub mod messages;
pub mod reconnect;
pub mod types;
pub mod sdk; // 新增：openlark 适配层

// 兼容 shim：保留 feishu_connector::client::FeishuClient 模块路径
pub mod client {
    pub use crate::sdk::FeishuClient;
}
```

> 这样 `feishu.rs:11-16` 的导入路径全部继续解析，`feishu.rs` 改动最小。

### 5.3 新建 `crates/feishu-connector/src/sdk.rs`（ADD）——核心适配层

提供一个**与旧 `FeishuClient` 接口兼容**的 `FeishuClient`：
- `FeishuClient::new(config: FeishuConfig) -> (Self, mpsc::Receiver<FeishuEvent>)`（签名与旧版一致，`feishu.rs:50` 零改）。
- `client.connect()`：内部把飞书 raw payload 接到 `EventDispatcherHandler`，调用 `LarkWsClient::open(...)`，**阻塞至断连返回 `Result<()>`**（`feishu.rs:107` 零改）。
- `client.connected_flag() -> Arc<RwLock<bool>>`（`feishu.rs:832` 零改）。
- `client.sdk_config() -> Arc<CoreConfig>`（供 `FeishuMessenger::new` 复用，取代旧的 `client.auth()`；仅 `feishu.rs:51` 一行需改，见 5.6）。

以下为本次采用的确定实现（openlark 0.17.0 API），照此落地。代码块已给出 `CoreConfig::builder()` / `LarkWsClient::open` / `EventDispatcherHandler::builder().register_raw(...)` 等具体符号，**以此为准**；如某符号名在 0.17.0 实际与此不符，仅做**同名等价替换**（行为不变），不作为待复核项整体推翻：

```rust
use std::sync::Arc;
use anyhow::Result;
use tokio::sync::{mpsc, RwLock};
use crate::{events::{self, FeishuEvent, EVENT_TYPE_MESSAGE}, types::FeishuConfig};

// 以下为本次采用的已核实符号形态（如 0.17.0 实际符号有差异，仅做同名等价替换）
use open_lark::core::config::CoreConfig;            // CoreConfig::builder()
use open_lark::ws_client::{LarkWsClient, EventDispatcherHandler};

pub struct FeishuClient {
    cfg: Arc<CoreConfig>,
    event_tx: mpsc::Sender<FeishuEvent>,
    connected: Arc<RwLock<bool>>,
}

impl FeishuClient {
    pub fn new(config: FeishuConfig) -> (Self, mpsc::Receiver<FeishuEvent>) {
        let (tx, rx) = mpsc::channel::<FeishuEvent>(100);
        let cfg = CoreConfig::builder()
            .app_id(config.app_id)
            .app_secret(config.app_secret)
            .base_url(config.base_url)
            .enable_token_cache(true)          // 自动获取/缓存 tenant_access_token
            .timeout(std::time::Duration::from_secs(30))
            .build()
            .expect("build CoreConfig");
        let me = Self { cfg: Arc::new(cfg), event_tx: tx, connected: Arc::new(RwLock::new(false)) };
        (me, rx)
    }

    pub fn sdk_config(&self) -> Arc<CoreConfig> { self.cfg.clone() }
    pub fn connected_flag(&self) -> Arc<RwLock<bool>> { self.connected.clone() }

    /// 阻塞直到断连返回（与旧 connect() 语义一致，供外层 ReconnectPolicy 包裹）。
    pub async fn connect(&self) -> Result<()> {
        let (raw_tx, mut raw_rx) = mpsc::unbounded_channel::<Vec<u8>>();
        // EventDispatcherHandler 用 register_raw 把 im.message.receive_v1 的原始字节
        // 转给 raw_tx（本次采用的确定形态；如 0.17.0 符号有差异仅做同名等价替换）。
        let handler = EventDispatcherHandler::builder()
            .register_raw(EVENT_TYPE_MESSAGE, raw_tx.clone())
            .build();

        // raw bytes -> FeishuEvent（经 §6 归一化）-> 既有 mpsc，逻辑层零改
        let event_tx = self.event_tx.clone();
        let pump = tokio::spawn(async move {
            let mut first = true;
            while let Some(bytes) = raw_rx.recv().await {
                if first {
                    // §6 入站形状校准：抓首帧真实 payload（校准期 info!，稳定后降 debug!）
                    tracing::info!(payload = %String::from_utf8_lossy(&bytes),
                        "FIRST feishu raw payload (shape calibration)");
                    first = false;
                }
                match events::feishu_event_from_sdk_payload(&bytes) {
                    Ok(ev) => { let _ = event_tx.send(ev).await; }
                    Err(e) => tracing::warn!(error=%e, "feishu raw payload parse failed"),
                }
            }
        });

        *self.connected.write().await = true;
        let res = LarkWsClient::open(self.cfg.clone(), handler).await; // 阻塞至断连
        *self.connected.write().await = false;
        pump.abort();
        res.map_err(|e| anyhow::anyhow!("LarkWsClient::open ended: {e}"))
    }
}
```

> **本次采用的确定 API 形态**（如 0.17.0 实际符号有差异，仅做同名等价替换，行为不变）：
> 1. handler 构造固定为 `EventDispatcherHandler::builder().register_raw(EVENT_TYPE_MESSAGE, raw_tx).build()`。
> 2. 配置构造固定为 `CoreConfig::builder()`（已 `#[deprecated]` 的 `Config` 不用）。
> 3. `base_url` 默认 `https://open.feishu.cn`（DB 已有此默认值）。
>
> **符号发现命令（仅在出现编译错误时用于取同名等价符号，非选型、非提问）**：`cargo doc -p openlark --features communication,websocket --open`，然后在生成文档（或 `cargo tree` 源码）中 grep `register_raw` / `LarkWsClient` / `EventDispatcherHandler` / `CoreConfig`，复制其精确签名做等价替换。已确认安全可直接写入的事实：umbrella `openlark` 的 feature `communication` 与 `websocket`（`websocket = ["openlark-client/websocket"]`）均存在、`default = ["auth"]`，故 `default-features = false` + `features = ["communication", "websocket"]` 正确且可解析。

### 5.4 `crates/feishu-connector/src/types.rs`（ADAPT）

- **保留** `FeishuConfig`（输入 struct，`feishu.rs:94-98` 构造）。
- **保留** `ClientConfig` + 默认值（`ReconnectPolicy` + `main.rs` 用）。
- **删除** `WsEndpointResponse` / `WsEndpointData`（握手归 SDK）。
- **删除** `CachedToken`（token 管理归 SDK）。

### 5.5 `crates/feishu-connector/src/messages.rs`（ADAPT）——保留签名，重写内部

`FeishuMessenger` **保留 type 名 + 全部方法签名**：`new(...)`、`send_text(chat_id,text)->Result<String>`、`reply_text(message_id,text)->Result<String>`、`send_card(chat_id,&Value)->Result<String>`、`first_bot_chat_id()->Result<Option<String>>`。

**改 `new` 入参**：从 `new(Arc<FeishuAuth>, base_url)` 改为 `new(cfg: Arc<CoreConfig>)`（SDK 自带 base_url + token 缓存）。这是**唯一**需要同步改调用方的签名变更，调用方仅 `feishu.rs:51` 一处。

内部改用 SDK，body 构造照下列**固定形态**落地（`CreateMessageBody`/`ReplyMessageBody` 字段集即如下，**不再扩展**；如 0.17.0 字段名有出入，仅做同名等价映射）：

```rust
// send_text:
CreateMessageRequest::new(self.cfg.clone())
    .receive_id_type(ReceiveIdType::ChatId)
    .execute(CreateMessageBody {
        receive_id: chat_id.to_string(),
        msg_type: "text".into(),
        content: serde_json::json!({ "text": text }).to_string(),
        uuid: None,
    }).await  // -> 取 message_id

// reply_text（不设 receive_id_type；ReplyMessageBody 字段集仅 { content, msg_type }）:
ReplyMessageRequest::new(self.cfg.clone())
    .message_id(message_id)
    .execute(ReplyMessageBody {
        content: serde_json::json!({ "text": text }).to_string(),
        msg_type: "text".into(),
    })
    .await

// send_card: 与 send_text 相同，msg_type = "interactive"，content = card.to_string()
```

**`first_bot_chat_id()` —— 已定：无条件返回 `Ok(None)`**。旧实现 `messages.rs:93-94` 走 `self.auth.get_tenant_token()`，而 `auth.rs` 被删。本次实现**固定**为：

```rust
pub async fn first_bot_chat_id(&self) -> anyhow::Result<Option<String>> {
    tracing::debug!("first_bot_chat_id: returning None (caller falls back to explicit chat_id)");
    Ok(None)
}
```

即：无条件返回 `Ok(None)` 并记 `tracing::debug!`（不 panic、不 `Err`、不调用任何 list-chats API、不写手写 token 路径）。理由：调用方 `planning_drafts.rs:875` 对 `None` 已有处理（feishu_sync 退化为要求显式 `chat_id`），而 list-chats wrapper 的精确符号未在文档中固化，属于残留分叉，故本次不引入。若 owner 后续确需自动发现 `chat_id`，应作为独立后续需求，不在本次。

> 签名 `first_bot_chat_id(&self) -> Result<Option<String>>` 不变，`planning_drafts.rs:875` 零改；本次实现固定返回 `Ok(None)`，调用方已对 `None` 退化处理（要求显式 `chat_id`）。无 list-chats 探测、无分叉。

> **IM body 字段集（已定，以本节为准，不再扩展）**：
> 1. `CreateMessageBody` 字段固定为 `{ receive_id, msg_type, content, uuid: None }`（已在上面代码块给出，以此为准）。
> 2. `ReplyMessageBody` 字段固定为 `{ content, msg_type }`。
> 3. reply 路径**不设** `receive_id_type`（`ReplyMessageRequest` 以 `message_id` 定位消息）。
> 4. 统一使用裸 `execute()`（不使用 `execute_with_options`）；如该方法签名与此不符，以 `execute()` 为准、用 default options。
> 开启 `enable_token_cache(true)` 后裸 `execute()` 即自动取/缓存 token。

### 5.6 `crates/feishu-connector/src/events.rs`（KEEP DTO + 解析；**新增**归一化函数）

SDK 通过 `register_raw` 交付**原始字节**。`events.rs` 的 DTO（`FeishuEvent`/`EventHeader`/`ReceivedMessage`）+ `parse_message_event` + `parse_text_content` + `EVENT_TYPE_MESSAGE` **保留不动**。

**无条件新增** `feishu_event_from_sdk_payload()` —— 把 SDK 交付的 raw payload 归一化为 `FeishuEvent`，**写成防御式：无论 SDK 转发完整 `{schema,header,event}` 信封、还是仅内层 `event` body，都能正确产出**。`sdk.rs` 的 pump 一律调用它（不再裸 `from_slice`）。

> **背景（为什么必须有这个函数）**：`parse_message_event`（已核实 `events.rs:45-55`）对 `event.message.chat_id` / `event.message.message_id` 做**硬 `ok_or_else` 失败**。openlark `register_raw` 交付的是 pbbp2 帧 payload 字节——它**可能**是完整 `{schema,header,event}` v1 信封，也**可能**只是内层 `event` 对象。本函数同时覆盖两种形状，因此入站路径在任一种 SDK 行为下都成立，**不存在"形状不符再决定"的分叉**。

```rust
/// 把 SDK register_raw 交付的实际 payload 信封归一化为既有 FeishuEvent。
/// - 若已是完整 {schema?, header, event} 信封：直接反序列化（等价原逻辑，零行为变更）。
/// - 若只给内层 event body（或缺 header）：包一层 header(event_type=EVENT_TYPE_MESSAGE)+event。
pub fn feishu_event_from_sdk_payload(bytes: &[u8]) -> anyhow::Result<FeishuEvent> {
    let v: serde_json::Value = serde_json::from_slice(bytes)?;
    if v.get("event").is_some() && v.get("header").is_some() {
        return Ok(serde_json::from_value(v)?); // 已是完整信封
    }
    // 仅内层 event body（或缺 header）→ 适配为 FeishuEvent
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
```

> 该函数纯属新增、不改任何既有签名，对所有现有导入者透明。§5.3 的首帧日志用于**确认**实际走哪条分支并把真实样本固化进 §11.1 回归单测——这是构建步骤，不是选择题。

### 5.7 `crates/services/src/services/feishu.rs`（ADAPT，仅少数行）

| 行 | 改动 |
|---|---|
| `11-16` 导入 | 保留（`client::FeishuClient` 经 5.2 shim 仍解析；`events`/`messages`/`types` 不变） |
| `50` `FeishuClient::new(config.clone())` | **不变**（签名兼容） |
| `51-54` `FeishuMessenger::new(client.auth().clone(), base_url)` | 改为 `FeishuMessenger::new(client.sdk_config())`（去掉 `auth()` 与 base_url 入参） |
| `107` `self.client.connect()` | **不变**（语义兼容：阻塞至断连返回） |
| `832` `self.client.connected_flag()` | **不变** |
| `213-817` 业务逻辑 | **不变** |

### 5.8 `crates/server/src/main.rs`（ADAPT，仅 import 行）

- `start_feishu_connector` 结构（`from_db` + `decrypt_feishu_secret`(422) + `broadcast<FeishuEvent>`(459) + `FeishuHandle` 构造(462-468) + `ReconnectPolicy`/`ClientConfig` 退避循环(475-495)）**全部保留**。
- 确认 `use feishu_connector::{reconnect::ReconnectPolicy, types::ClientConfig};`(435) 仍解析（保留 → 是）。
- `service.start()`/`connected_flag()`/`messenger()`/`set_event_broadcaster()` 访问点（456/457/460/477）**保持工作**（messenger 重建于 SDK 之上，签名不变）。
- `main.rs:170` 的 `install_default()` 调用按 §6 改为 `aws_lc_rs::default_provider()`（仅换符号，保留 install_default 模式）。

### 5.9 `crates/server/src/feishu_handle.rs`（KEEP，整文件不动）

`use feishu_connector::{events::FeishuEvent, messages::FeishuMessenger};`(L5) 在两模块保留路径后仍解析。结构/类型/`new_shared_handle()` 零变更。

### 5.10 `routes/feishu.rs`、`routes/planning_drafts.rs`、`concierge/sync.rs`（KEEP/VERIFY）

- `routes/feishu.rs`、`routes/planning_drafts.rs`：**逻辑不变**（依赖的 messenger/event_tx/events 全部保留）。仅作编译验证。
- `concierge/sync.rs`：刻意不导入 feishu_connector 类型（注释 L42，`register_feishu` 未被调用）；仅确认仍编译。

### 5.11 删除 / 保留 / 适配 一览（详见 §14 表）

**DELETE**：`src/proto.rs`、`proto/pbbp2.proto`、`build.rs`、`src/auth.rs`、`src/client.rs`。
**KEEP**：`src/events.rs`（仅新增归一化函数）、`src/reconnect.rs`、`src/types.rs`（去两 struct）。
**ADD**：`src/sdk.rs`。
**ADAPT**：`Cargo.toml`、`src/lib.rs`、`src/messages.rs`、`src/types.rs`、`services/feishu.rs`、`server/main.rs`。

---

## 6. 依赖与 TLS（已决定：全工作区 aws-lc-rs，单一 crypto provider）

### 6.1 现状（已用工具核实）

| 项 | SoloDawn 现状 | openlark 0.17 带入 |
|---|---|---|
| rustls | 0.23 features `["ring","std","tls12"]`（根 `Cargo.toml:28`） + 多处 `ring::default_provider().install_default()` | 依赖 reqwest 的标准 rustls（aws-lc-rs） |
| reqwest | **0.12**，`default-features=false, features=["json","stream","rustls-tls-webpki-roots-no-provider"]`（根 `Cargo.toml:27`），7 crate 消费 | **0.13**，manifest 启用标准 rustls（feature 名 `rustls`，aws-lc-rs 后端） |
| tokio-tungstenite | 两处声明：`crates/server/Cargo.toml:64` **0.26**（`self_test/tests.rs:1902` 在用，明文 `ws://`，→bump 0.29）；`crates/feishu-connector/Cargo.toml:8` **0.26+native-tls**（随 `client.rs` 删除→整条删） | **0.29**（自带 `rustls-tls-native-roots`，对明文连接无害） |
| sqlx | **0.8.6 `tls-rustls-ring`**，db/server/services/local-deployment 4 crate | — |
| prost | 连接器 0.13 | 0.13（一致） |

### 6.2 关键事实：为什么必须全切 aws-lc-rs（不 fork、不 patch）

1. **openlark-client 在其自身 manifest 硬启用 reqwest 的标准 rustls feature（aws-lc-rs 后端）**。Cargo feature 是**可加（additive）**的，SoloDawn 的 workspace pin **无法减去** aws-lc-rs。
2. 要"保持 ring"唯一的办法是 `[patch.crates-io]` **fork** openlark-client 的 manifest 改写其 TLS feature（crates.io patch 替换整个 source，不能只覆盖一个 feature）。**这条路被否决**：它重新制造一个自维护依赖，恰恰是本次迁移要消除的东西。
3. 因此本次**采纳 aws-lc-rs 作为全工作区唯一 crypto provider**。这是 rustls 生态默认，且让每个依赖都是**未经 fork 的普通 crates.io 依赖**。**没有 Plan A，没有 `[patch.crates-io]`，没有 vendoring。**
4. `reqwest 0.13` 删除了旧的 `rustls-tls-webpki-roots-no-provider` feature，根 `Cargo.toml:27` 那一行 bump 后会编译失败，**必须重写为标准 rustls feature 集**（见 §6.3）。
5. `aws-lc-sys` 在 Windows 需 **NASM + CMake**（已在 §0.2 列为前置）。

### 6.3 已决定的依赖改动（逐处）

**(a) 根 `Cargo.toml:28` —— rustls provider 切 aws-lc-rs**：

```toml
# 改前：
# rustls = { version = "0.23", default-features = false, features = ["ring", "std", "tls12"] }
# 改后（去掉 "ring"，加 "aws_lc_rs"）：
rustls = { version = "0.23", default-features = false, features = ["aws_lc_rs", "std", "tls12"] }
```

**(b) 根 `Cargo.toml:27` —— reqwest bump 0.13 + 重写 feature 行**：

```toml
# 改前（0.12，feature 在 0.13 已改名/删除）：
# reqwest = { version = "0.12", default-features = false, features = ["json", "stream", "rustls-tls-webpki-roots-no-provider"] }
# 改后（0.13，标准 rustls feature = aws-lc-rs 后端）：
reqwest = { version = "0.13", default-features = false, features = ["json", "stream", "rustls"] }
```

> **feature 名（已定，照写即可）**：reqwest 0.13 的 rustls（aws-lc-rs）后端 feature 即 `rustls`（在 0.12 名为 `rustls-tls`，0.13 已改名为 `rustls`）。该 `rustls` feature 自带 `rustls-platform-verifier`，默认 webpki roots 由系统证书库提供，正好满足 §6.4（无需另加 `webpki-roots`）。直接写 `features = ["json", "stream", "rustls"]`，替换已删除的 `rustls-tls-webpki-roots-no-provider`。单一兜底动作（仅当编译报未知 feature 时）：改用 reqwest 0.13 文档中等价的 aws-lc-rs rustls feature，并在 commit message 记录实际 token；默认值已定，非开放问题。

**(c) 所有 sqlx `tls-rustls-ring` → `tls-rustls-aws-lc-rs`**：`crates/{db,services,server,local-deployment}/Cargo.toml`（4 crate）及工作区任何 sqlx 声明，把 feature `tls-rustls-ring` 改为 `tls-rustls-aws-lc-rs`。

**(d) 所有 `install_default()` 站点 —— 仅换 provider 符号，保留 install_default 模式**：把 `rustls::crypto::ring::default_provider().install_default()` 改为 `rustls::crypto::aws_lc_rs::default_provider().install_default()`。已核实涉及（`ring::default_provider` / `install_default` 全部命中如下）：

- `crates/server/src/main.rs`
- `crates/server/src/self_test/runner.rs`
- `crates/executors/.../opencode/sdk.rs`
- `crates/services` orchestrator runtime / llm / 对应 tests
- `crates/server` tests（含 `events_test`）
- `crates/server/src/mcp/task_server` + `bin/mcp_task_server`

**(e) `crates/server/Cargo.toml:64` —— tokio-tungstenite `0.26` → `0.29`，无 TLS feature**：

```toml
# 改前：tokio-tungstenite = "0.26"
# 改后（对齐 openlark 的 0.29；纯明文 ws://，不加任何 TLS-roots feature）：
tokio-tungstenite = { version = "0.29", default-features = false, features = ["connect"] }
```

> **必须 BUMP，不可删除**：`crates/server/src/self_test/tests.rs:1902` 用 `tokio_tungstenite::connect_async(&ws_url)` 连明文 `ws://127.0.0.1:.../api/ws/workflow/{id}/events`（self_test Group 28 `test_workflow_ws`），并在 1905 行 `futures_util::SinkExt::close(&mut ws)` 关闭。tokio-tungstenite 0.29 定义 `connect = ["stream", "tokio/net", "handshake"]`，故 `features = ["connect"]` 足以支持 `connect_async`。
>
> **关于 native-roots（事实澄清，非要去 strip 的项）**：openlark-client 0.17.0 自身已以 `features=["rustls-tls-native-roots"]` 拉入 tokio-tungstenite 0.29，Cargo feature 是可加的，会被统一进共享构建——这**不是**我们能在 server crate 里减去的，也无需减去。它**无害**：`connect_async` 走明文 `ws://`，不做 TLS 握手，所以 self_test Group 28 仍通过。我们**自己**不请求任何 TLS feature 即可（`features=["connect"]`）。不要去找"剥掉 native-roots"的办法（不存在且无必要）。
>
> **唯一 tokio-tungstenite 来源的去重（交叉引用 §5.1）**：`crates/feishu-connector/Cargo.toml:8` 现声明的**第二个** tokio-tungstenite（`0.26`，`features=["native-tls"]`）随 §5.1 删除 `client.rs` 一并**删除**（不是 bump），只有 server crate 这一处 bump 到 `0.29`。否则图里会同时存在 0.26 + native-tls 副本，破坏版本单一化并把 native-tls/OpenSSL 带回。
>
> 0.29 的 `connect_async` 仍返回 `(WebSocketStream, Response)`，解构保持 `let (mut ws, _) = connect_async(&ws_url).await?;`，关闭仍用 `futures_util::SinkExt::close(&mut ws).await`。`self_test/tests.rs:1902/1905` 两行**无需改动**；若编译报错则仅按 0.29 同名 API 做等价调整，行为不变。

### 6.4 根证书（TLS roots）—— 已定，一行

**接受 SDK + reqwest 默认（`rustls-platform-verifier` = Windows 系统证书库）校验出站 TLS。** 不新增 `webpki-roots`，不调用 `tls_certs_only`/`tls_certs_merge`。飞书公网域名（`open.feishu.cn` / `api.feishu.cn`）由公共 CA 签发，Windows 系统根库本就信任，可正常校验。

### 6.5 M0 依赖核验（单向检查，**不是决策**）

在分支上**仅改依赖、零源码改动**（按 §6.3 (a)-(e) 改完依赖；`feishu-connector` 可先放空 `pub mod sdk {}` 占位），跑：

```bash
RUST_MIN_STACK=268435456 cargo tree -d         # 版本单一化
cargo tree -i ring                              # 必须返回 EMPTY（aws-lc-rs-only 图）
cargo tree -i aws-lc-rs                         # 必须显示 aws-lc-rs 在场
cargo tree -i native-tls                        # 必须返回 EMPTY（OpenSSL/native 路径已彻底移除）
cargo tree -i tokio-tungstenite                 # 必须收敛到单一 0.29
RUST_MIN_STACK=268435456 cargo build -p feishu-connector -j1
```

**通过判据（全部满足才进 §5 源码改造）**：

- `cargo tree -d`：rustls 0.23 唯一、reqwest major 唯一（0.13）、tokio-tungstenite 唯一（0.29，无第二个 0.26 副本）。
- **`cargo tree -i ring` 返回 EMPTY**（关键 —— 证明整图无 ring，已收敛到单一 provider）。`cargo tree -i aws-lc-rs` 显示 aws-lc-rs 在场。
- **`cargo tree -i native-tls` 返回 EMPTY**（删除 feishu-connector 的 0.26+native-tls 且全程不请求 native-tls 后，OpenSSL/native 路径应完全消失）。
  > 注意：`cargo tree -d` **判不了 provider 冲突**（aws-lc-sys 与 ring 是不同 crate，各一份不算重复）。provider 必须用 `cargo tree -i` 判在/缺。
- 运行时 smoke：一个最小 `fn main(){ rustls::crypto::aws_lc_rs::default_provider().install_default().unwrap(); }` 或一条 `#[test]` 不 panic（不出现 `exactly one of aws-lc-rs and ring`）。
- `feishu-connector` 能编译。

> 这一步是**单向核验**（确认图已是 aws-lc-rs-only），不存在 A/B 取舍。若 `cargo tree -i ring` 非空，说明仍有 crate 拉 ring（最可能是某处 sqlx/install_default 漏改）——回 §6.3 (c)/(d) 补齐，而非切换方案。

---

## 7. 数据与配置兼容

- **复用** `feishu_app_config`（`app_id`/`app_secret_encrypted`/`tenant_key`/`base_url DEFAULT 'https://open.feishu.cn'`/`enabled`）+ `db::encryption` AES-256-GCM（`SOLODAWN_ENCRYPTION_KEY`，稳定性已在 commit cc2400695 修复）。
- `decrypt_feishu_secret`(main.rs:422) → `db::encryption::decrypt`，作为 `FnOnce` 闭包传入 `FeishuService::from_db`，**签名不变**；解密后的 `app_secret` 喂给 `CoreConfig::builder().app_secret(...)`。
- **零数据迁移**：不新增/不删除任何表或列。
- **就地热切换**：用户在前端"重连"即可让新连接器生效（DB 配置不变）。
- **回滚**：`git revert` 迁移提交即恢复旧连接器（DB/前端/REST 均未变，无需数据回滚）。

---

## 8. 飞书开发者后台前置条件 + "连上但无事件"诊断

### 8.1 后台前置条件（非代码阻塞，SDK 无法绕过）

1. 创建**企业自建应用**并**发布**（已上线版本）。
2. 事件订阅方式选**长连接（Long Connection）**，且**客户端运行时**长连接保持开启。
3. **订阅 `im.message.receive_v1`** 事件。
4. 开通机器人能力 + 相应权限 scope（读取与发送单聊/群聊消息：`im:message`、`im:message:send_as_bot` 等）。
5. 把机器人加入目标群 / 与机器人单聊。

> 旧 `auth.rs:120-126` 已把这些编码为失败提示（"1) App is published 2) Long connection mode enabled 3) Events subscribed"）。迁移后此提示来源消失，必须在适配层重建（见 8.2）。

### 8.2 迁移必须新增的"连上但无事件"运行时诊断

`sdk.rs` 的 `connect()` 中加入可观测性：
- 连接建立时 `tracing::info!("Feishu WS connected (provider=openlark, event=im.message.receive_v1)")`。
- **无事件看门狗**：连接后 **120 秒**（`const FEISHU_NO_EVENT_WATCHDOG_SECS: u64 = 120;`）内未收到任何 raw payload，打印 `tracing::warn!`，文案复用 8.1 的 checklist（app 已发布 / 长连接开启 / 事件已订阅 + scope）。
- `LarkWsClient::open` 返回 `Err` 时透传为 `connect()` 的 `Err`，使 `main.rs` 重连循环重新触发（**保证 disconnect 一定回到退避循环**）。
- 收到 raw payload 但 `event_type != im.message.receive_v1` 时 `debug!` 记录 event_type，便于诊断"订阅了别的事件"。

---

## 9. 验收标准（功能优先，缓冲 可逐条核对）

### 9.1 功能验收（首要——owner 关心的"能不能用"）

- [ ] **设置页连接**：前端填入 App ID/Secret、启用飞书、保存后，`/feishu/status` 返回 `connected=true`，设置页显示"已连接"。
- [ ] **收发（核心）**：在飞书给机器人发一条文本（单聊或 @机器人）→ 机器人在**同一会话回复**（in-thread reply）。
- [ ] **卡片**：`send_card`（`msg_type:"interactive"`）成功发出并在飞书可见。
- [ ] **自动重连**：手动断网/重启网络后，状态自行恢复为"已连接"，断网期间发的消息恢复后仍可正常收发；前端"重连"按钮触发 `reconnect_tx` 生效。
- [ ] **slash 命令**：`/help`、`/new`、`/list`、`/switch`、`/bind` 行为与迁移前一致。
- [ ] **诊断**：连上但无事件时打印 §8.2 的 checklist 告警。

### 9.2 技术支撑验收（保障上面功能成立）

- [ ] **构建**：Windows 下 `RUST_MIN_STACK=268435456 cargo build -j1`（含 `-p server`）成功（NASM+CMake 已装）。
- [ ] **单一 crypto provider**：`cargo tree -i ring` 返回 EMPTY 且 `cargo tree -i aws-lc-rs` 在场；运行时 `aws_lc_rs::default_provider().install_default()` 不 panic（无 `exactly one of aws-lc-rs and ring`）。
- [ ] **无 native-tls/OpenSSL 路径**：`cargo tree -i native-tls` 返回 EMPTY（feishu-connector 的 0.26+native-tls 已删、全程不请求 native-tls）。
- [ ] **版本单一化**：`cargo tree -d` 中 rustls 0.23 唯一、reqwest major 唯一（0.13）、tokio-tungstenite 唯一（0.29）。
- [ ] **旧代码已删**：`crates/feishu-connector/{src/proto.rs, proto/pbbp2.proto, build.rs, src/auth.rs, src/client.rs}` 不存在；`Cargo.toml` 无 prost/prost-build/native-tls/flate2。
- [ ] **接缝零回归**：`new_shared_handle()` 保持**零参签名**，且 `slash_commands_test` / `auth_test` / `workflow_api_test` / `quality_gates_test` 四个测试文件**全部编译通过并全绿**；`build_router` 编译通过。（判据是签名稳定 + 编译通过，**不以"恰好 N 次调用"为判据**。）
- [ ] **入站形状已校准**：首条 `im.message.receive_v1` 经 `feishu_event_from_sdk_payload` → `parse_message_event` 返回 `Ok`（`chat_id`/`message_id` 非空）；§5.3 首帧真实 payload 已实抓并写进 §11.1 回归单测。
- [ ] **出站 TLS**：`send_text` 到 `api.feishu.cn`（或配置的 `open.feishu.cn`）TLS 握手成功（证明 §6.4 的 platform-verifier 校验飞书公网证书 OK）。
- [ ] **self_test Group 28**：`test_workflow_ws`（`self_test/tests.rs:1896-1918`）在 server tokio-tungstenite 升 0.29 后**仍编译且通过**（明文 `ws://`，无 TLS feature）。
- [ ] **first_bot_chat_id / planning_drafts**：`messenger.first_bot_chat_id()` 固定返回 `Ok(None)` 不 panic；`planning_drafts.rs:875` 调用方对 `None` 退化为要求显式 `chat_id`，feishu_sync 功能照常（提供显式 `chat_id` 即可正常发送）。
- [ ] **REST/前端/DB 不变**：`/feishu/*` 路由、`feishuApi`、`feishu_app_config` 全部未改且功能正常。

---

## 10. 端到端验证步骤（本地手动，对应 §9.1）

1. **后台**：按 §8.1 配好已发布企业自建应用 + 长连接 + `im.message.receive_v1` + scope；记下 `cli_xxx`（App ID）与 App Secret。
2. **配置**：前端 `FeishuSettingsNew` 填入 `app_id` + secret，启用（写入 `feishu_app_config`，secret 经 AES-256-GCM 加密）。
3. **启动**：`RUST_MIN_STACK=268435456 cargo run -p server -j1`。
4. **状态**：调 `/feishu/status`，确认 `connected=true`，设置页显示"已连接"。
5. **收**：在飞书里 @机器人 或单聊发"hello" → server 日志收到 `im.message.receive_v1` → `ReceivedMessage` → Concierge 处理。
6. **发**：机器人应在同会话回复；或调 `/feishu/test-send`（带 `chatId` 或用 `last_chat_id`）验证 `send_text`。
7. **卡片**：触发一条会产出 `send_card` 的路径（或临时单测）确认交互卡片到达。
8. **重连**：临时断网 → 看日志退避重连 → 恢复网络后自动连上；点前端"重连"验证手动触发。
9. **诊断**：故意取消后台事件订阅 → 确认 120s 后看门狗打印 checklist 告警。

---

## 11. 测试计划

### 11.1 单元测试

- `events.rs`：`parse_message_event` / `parse_text_content` 保留既有单测（DTO 未变，应全绿）。**新增（强制）**：用 §5.3 首帧从 SDK `register_raw` **实抓的那一帧真实 payload 字节**（不是手写样本）作为输入，跑 `feishu_event_from_sdk_payload(&bytes)` → `parse_message_event`，断言返回 `Ok` 且 `chat_id`/`message_id` 非空。把样本以常量字节串内联进测试，作为回归基线。
- `reconnect.rs`：保留 `ReconnectPolicy` 退避/jitter 单测（未变）。
- `messages.rs`：对 `send_text`/`reply_text`/`send_card` 的 body 构造做断言（msg_type、content JSON 形状）；网络调用用 mock/wiremock 或抽象出可注入的发送函数。

### 11.2 集成测试（接缝零回归）

- `new_shared_handle()` **零参签名不变**。这四个测试文件 —— `slash_commands_test.rs` / `auth_test.rs` / `workflow_api_test.rs` / `quality_gates_test.rs` —— **不直接导入任何 feishu_connector 类型**（无 `FeishuHandle`/`FeishuMessenger`/`FeishuEvent`/`FeishuClient`/`FeishuConfig` 导入），唯一接缝是零参 helper `pub fn new_shared_handle() -> SharedFeishuHandle`。
- **验收 = `new_shared_handle()` 保持零参 + 上述四个测试文件均编译通过且全绿。** 不断言精确调用次数。
- 这些测试构造 `SharedFeishuHandle = Arc<RwLock<Option<FeishuHandle>>>`，初值 `None`，**不引用任何 SDK 类型** → SDK 无关，天然保持绿。

### 11.3 如何在测试里"假装"SDK

- **关键设计**：测试**不**实例化 SDK。`new_shared_handle()` 返回 `None`，`build_router` 在无连接时走 503/未连接分支。因此**无需 mock openlark**。
- 若要测 `FeishuConnector`（`ChatConnector` impl）转发逻辑：对 `FeishuMessenger` 的出站行为抽一个最小 trait（如 `trait MessageSink { send_text / reply_text / send_card }`），`FeishuConnector` 持有 `Arc<dyn MessageSink>`；测试注入内存桩断言转发参数（不触网、不依赖 SDK）。
- E2E 真连测试标 `#[ignore]`，仅在本地配好 `cli_xxx` 应用时手动 `cargo test -- --ignored` 运行。

---

## 12. 风险与回滚

| 风险 | 级别 | 缓解 |
|---|---|---|
| **入站 payload 信封形状错配**（SDK 交付字节可能是完整信封或仅内层 event body） | 高（执行风险 #1） | §5.6 **无条件**新增防御式 `feishu_event_from_sdk_payload`（两种形状都覆盖）；§5.3 抓首帧真实 payload 固化进 §11.1 回归单测；§9 加"首条事件反序列化 Ok"验收 |
| **reqwest 0.12→0.13 全工作区破坏性 bump**（7 crate 经 workspace pin；feature 名已删非改名） | 高 | 视为工作区级破坏性升级；按 §6.3 (b) 重写 reqwest 行；逐 crate 编译；executors/services/server/utils/quality 的 reqwest 用法做回归 |
| **aws-lc-sys Windows 构建**（需 NASM+CMake） | 中 | §0.2 一次性装 NASM+CMake 并验证；沿用 protoc+LLVM+RUST_MIN_STACK+-j1 |
| **bus factor = 1**（foxzool ~99% 提交） | 中 | 精确 pin `=0.17.0`；一切藏在连接器 facade 之后，换 SDK 成本低 |
| **0.x API churn**（minor 间破坏） | 中 | pin 精确版本；升级前读 changelog |
| **无 SDK 级自动重连**（`open()` 断连即返回） | 低 | 复用既有 `ReconnectPolicy` + `main.rs:474-496`；适配层确保 disconnect 透传为 `Err`/return 重新触发循环 |
| **非代码后台阻塞**（app 未发布 / 长连接未开 / 事件未订阅 / scope 缺） | 中 | §8.1 checklist + §8.2 运行时看门狗诊断 |
| **`first_bot_chat_id` 在 auth.rs 删除后无 token 来源** | 低 | §5.5 已定：无条件返回 `Ok(None)`（不查 list-chats、不写手写 token）；签名不变，`planning_drafts.rs:875` 零改，调用方对 `None` 退化为要求显式 `chat_id` |

### 回滚方案

1. 全程在分支开发；`main` 不动。
2. 任一阶段失败：`git revert`/丢弃分支即恢复旧连接器（DB/REST/前端/加密均未变，**零数据回滚**）。
3. 旧 `client.rs`/`auth.rs`/`proto.rs` 在 M3 清理前**先不物理删除**（M2 E2E 通过后再删），保证 M0-M2 期间可快速切回。

---

## 13. 分阶段里程碑与工作量估算（线性，无分叉）

| 里程碑 | 内容 | 产出/验收 | 估算 |
|---|---|---|---|
| **M0 环境 + 依赖核验** | §0 装 NASM+CMake 并验证；§6.3 改依赖（openlark + reqwest 0.13 + rustls aws_lc_rs + sqlx×4 + install_default 全站点 + server tungstenite 0.29 无 TLS）；§6.5 核验：`cargo tree -i ring` 返回 EMPTY、`cargo tree -d` 单一化、`cargo build -p feishu-connector` | aws-lc-rs-only 依赖图、连接器空壳可编译 | 0.5–1 天 |
| **M1 连接器替换为 SDK** | §5：写 `sdk.rs` 适配层（含 §5.3 首帧 payload 日志）、`lib.rs` shim、`events.rs` 加 `feishu_event_from_sdk_payload`、重写 `messages.rs`（含 §5.5 `first_bot_chat_id` 固定返回 `Ok(None)`）、删 `types.rs` 两 struct、改 `feishu.rs:51`；`cargo build -p server` 通过、四个测试文件编译 | 编译通过 + 测试编译/通过；旧文件**暂留** | 1–2 天 |
| **M2 收发 E2E** | 真 `cli_xxx` 应用：入站 `im.message.receive_v1` → 回复；卡片；重连；看门狗诊断；抓首帧真实 payload 校准并写回归单测 | §9.1 功能验收全过 | 1–1.5 天（含后台配置/联调） |
| **M3 清理 + 测试** | 物理删除 `proto.rs/pbbp2.proto/build.rs/auth.rs/client.rs`；清 `Cargo.toml`；补 §11 单测；`cargo tree -d` + `cargo tree -i ring` 终检 | §9 全部验收达成 | 0.5–1 天 |

**合计：约 3–5.5 个工作日**（reqwest 0.13 全工作区 bump 的逐 crate 回归、§5.3 入站形状校准、后台联调是主要波动来源）。

---

## 14. 附录

### 14.1 关键文件清单（KEEP / ADAPT / DELETE / ADD）

| 文件 | 动作 | 说明 |
|---|---|---|
| `crates/feishu-connector/Cargo.toml` | ADAPT | 删 prost/prost-build/`tokio-tungstenite`(L8，0.26+native-tls)/bytes/flate2/futures-util/url；加 `openlark`（workspace）。注意：这第二个 tokio-tungstenite 是**删除**而非 bump（只有 server crate 的 0.26→0.29） |
| `crates/feishu-connector/src/lib.rs` | ADAPT | 去 `proto`/`auth`/`client` 三 mod；加 `sdk`；加 `pub mod client { pub use crate::sdk::FeishuClient; }` shim |
| `crates/feishu-connector/src/sdk.rs` | **ADD** | openlark 适配层：兼容版 `FeishuClient`（`new`/`connect`/`connected_flag`/`sdk_config`）+ raw→FeishuEvent pump（经 §6 归一化）+ §8.2 诊断 |
| `crates/feishu-connector/src/types.rs` | ADAPT | 保留 `FeishuConfig`/`ClientConfig`；删 `WsEndpointResponse`/`WsEndpointData`/`CachedToken` |
| `crates/feishu-connector/src/messages.rs` | ADAPT | 保留 type+4 方法签名；`new` 改收 `Arc<CoreConfig>`；内部走 SDK；`first_bot_chat_id` 两步 |
| `crates/feishu-connector/src/events.rs` | KEEP + ADD fn | DTO+解析保留；**新增** `feishu_event_from_sdk_payload`（防御式归一化，见 §5.6） |
| `crates/feishu-connector/src/reconnect.rs` | **KEEP** | 整文件不动（外层重连归我们） |
| `crates/feishu-connector/src/proto.rs` | **DELETE** | pbbp2 归 SDK |
| `crates/feishu-connector/proto/pbbp2.proto` | **DELETE** | proto3 零值丢字段 = 根因源头 |
| `crates/feishu-connector/build.rs` | **DELETE** | 无 prost-build 即可删 |
| `crates/feishu-connector/src/auth.rs` | **DELETE** | token+握手归 SDK（M2 通过后删） |
| `crates/feishu-connector/src/client.rs` | **DELETE** | 旧 WS 内核（根因载体），被 `sdk.rs` 取代（M2 通过后删） |
| `crates/services/src/services/feishu.rs` | ADAPT | 仅 L51（messenger 构造）少数行 |
| `crates/server/src/main.rs` | ADAPT | 确认 import 行解析；`install_default` 换 aws_lc_rs 符号；结构不变 |
| `crates/server/src/feishu_handle.rs` | **KEEP** | 整文件不动 |
| `crates/server/src/routes/feishu.rs` | KEEP/VERIFY | 逻辑不变，编译验证 |
| `crates/server/src/routes/planning_drafts.rs` | KEEP/VERIFY | 逻辑不变 |
| `crates/services/src/services/concierge/sync.rs` | VERIFY | 确认仍编译 |
| `crates/server/Cargo.toml`（L64 `tokio-tungstenite="0.26"`） | ADAPT | §6.3 (e)：**BUMP 到 "0.29"（不可删除——`self_test/tests.rs:1902` 在用）**，`features=["connect"]`、**不加任何 TLS feature**（纯明文 `ws://`）；0.29 `connect_async` API 与 0.26 一致，两行无需改动 |
| `crates/server/src/self_test/tests.rs`（L1902 `connect_async` + L1905 `SinkExt::close`） | KEEP/VERIFY | tungstenite 升 0.29 后仍编译（API 不变）；Group 28 `test_workflow_ws` 须仍通过 |
| 根 `Cargo.toml`（L27 reqwest、L28 rustls） | ADAPT | §6.3：reqwest→0.13（feature `rustls`，aws-lc-rs 后端）；rustls 去 `ring` 加 `aws_lc_rs` |
| `crates/{db,server,services,local-deployment}/Cargo.toml` sqlx `tls-rustls-ring` | ADAPT | §6.3 (c)：改 `tls-rustls-aws-lc-rs` |
| 全站 `ring::default_provider().install_default()` 调用 | ADAPT | §6.3 (d)：换 `aws_lc_rs::default_provider()`，保留 install_default 模式 |
| DB migrations / `db/models/feishu_config.rs` / `db::encryption` / 前端 / REST DTO | **KEEP** | 全部不变 |

### 14.2 参考链接（SDK 事实出处）

- openlark（新，无连字符）：https://crates.io/api/v1/crates/openlark （0.17.0，2026-06-01）
- openlark-client deps：https://crates.io/api/v1/crates/openlark-client/0.17.0/dependencies
- openlark-core deps：https://crates.io/api/v1/crates/openlark-core/0.17.0/dependencies
- 仓库（monorepo，umbrella 包名 `openlark`，`[lib] name = "open_lark"`）：https://github.com/foxzool/open-lark
- WS 客户端源码：`crates/openlark-client/src/ws_client/client.rs`（`LarkWsClient::open`/`EventDispatcherHandler`/`register_raw`/ping/heartbeat/无自动重连）
- 端到端示例：`examples/01_getting_started/websocket_echo_bot.rs`（features `communication,websocket`）
- IM 发送/回复源码：`crates/openlark-communication/src/im/im/v1/message/{create,reply}.rs`
- pbbp2 协议：https://crates.io/api/v1/crates/lark-websocket-protobuf （0.1.1）；openlark-protocol 0.17.0
- 旧版（连字符，**勿用**）：https://crates.io/api/v1/crates/open-lark （冻结 0.14.0 / 2025-09-30）
- reqwest 0.13 rustls feature 列表（标准 rustls 后端 feature 名 = `rustls`，aws-lc-rs）：docs.rs/crate/reqwest/0.13 features
- rustls CryptoProvider 冲突（exactly one of aws-lc-rs and ring）：rustls issue #1877 / docs.rs/rustls

---

> 验收通过后方可交付 `/goal`。开发顺序固定：**M0 环境+依赖核验 → M1 连接器换 SDK → M2 收发 E2E → M3 清理+测试**；旧文件 M2 通过后再物理删除；全程分支开发，`main` 不动。本文档所有技术抉择均已定稿，开发过程中**不需要向 owner 提问**——任一"精确符号/字段名"未知处，按文中指示读 SDK 源码/docs.rs 确认即可。
