# PRD：将飞书集成迁移到 open-lark（社区 Rust SDK）

> 状态：Draft / 待评审
> 适用范围：`crates/feishu-connector` 及其上层接缝（`services::feishu` / `ChatConnector` / `SharedFeishuHandle` / `main.rs` 启动逻辑）
> 撰写日期：2026-06-14
> 这是"如果走新方式 / 改造"的交付物。**默认推荐路线（fix-existing）请见配套诊断结论**；本 PRD 描述的是次选路线（migrate-open-lark）的完整可落地方案，供在"决定换库"时直接执行。

---

## 0. 重要前置说明（务必先读）

经诊断，飞书集成"至今无法正常使用"的**根因是一个明确的线缆编码 bug，而非架构错误**：

- `crates/feishu-connector/proto/pbbp2.proto:1` 声明 `syntax = "proto3"`，prost 会丢弃值为默认值（0）的 `SeqID`/`LogID`/`method` 等字段；
- 而官方 `larksuite/oapi-sdk-go` 的 `pbbp2.pb.go` 把 `SeqID(1)`/`LogID(2)`/`service(3)`/`method(4)` 标记为 **required 并无条件序列化**，缺失即判为非法帧；
- `client.rs:33-43` 的 ping 用 `..Default::default()` 构造 → 这些字段全为 0 → 被 prost 丢弃 → 被服务器拒绝（ping 超时断连）。

因此 **官方诊断的首选方案是 `fix-existing`（原地修复，约 0.5–1.5 天，零接缝改动）**。本 PRD 之所以仍完整给出 open-lark 迁移方案，是因为：

1. 任务明确要求交付"改造方案"这一并行选项；
2. 若未来连接器维护负担确实变重、或团队希望卸下自维护协议栈，open-lark 是**最强次选**（runner-up）。

**红队顾虑（必须正视）**：换库**不能解决**飞书开发者后台的配置问题（应用未发布 / 长连接未开启 / 未订阅 `im.message.receive_v1` / 权限未审批）。HTTP bootstrap（`auth.rs:96` 的 `/callback/ws/endpoint`）即使在这些条件不满足时也可能返回 `code=0` 与一个合法 WS URL，于是出现"连上了但收不到任何事件"。**无论选哪条路，控制台前置条件与"connected-but-no-events"诊断都必须随迁移一并交付。** 见 §9、§12。

---

## 1. 背景与动机（为何改造）

### 1.1 现状

`crates/feishu-connector` 是一套**手写、逆向**的飞书"长连接"（长连接 / long-connection）WebSocket 事件订阅协议实现，零外部 SDK 依赖：

| 文件 | 职责 |
|---|---|
| `src/auth.rs` | `tenant_access_token` 缓存；`acquire_ws_endpoint()` POST `{AppID,AppSecret}` 到 `{base_url}/callback/ws/endpoint` |
| `src/client.rs` | tokio-tungstenite WS；prost protobuf Frame 编解码；ping 循环；分片重组（FragmentCache）；带 `biz_rt` 头的事件 ack |
| `src/proto.rs` + `build.rs` + `proto/pbbp2.proto` | prost 生成的 pbbp2 Frame/Header（`METHOD_CONTROL/DATA`、ping/pong/event） |
| `src/messages.rs` | REST `send_text` / `reply_text` / `send_card` / `first_bot_chat_id`（`/open-apis/im/v1/messages`） |
| `src/reconnect.rs` | 指数退避 `ReconnectPolicy` |
| `src/types.rs` | `FeishuConfig` / `ClientConfig` / `WsEndpointResponse` |
| `src/events.rs` | `FeishuEvent` JSON；`EVENT_TYPE_MESSAGE = "im.message.receive_v1"`；`parse_message_event` |

### 1.2 痛点

1. **协议自维护成本与漂移风险**：飞书一旦变更长连接线缆格式，需要我们自己跟进。
2. **零测试**：连接器没有任何单测 / 集成测试，commit `62f426b11`"对齐官方 SDK"是"读代码而非跑代码"写出来的，因此把 proto2 写成了 proto3。
3. **TLS 栈偏离工作区策略**：连接器用 `tokio-tungstenite 0.26 + native-tls`（`Cargo.toml:8`），而工作区根 `Cargo.toml:27-28` 是 rustls-only（`reqwest` default-features=false + `rustls-tls-webpki-roots-no-provider`，`rustls 0.23 "ring"`）。这是一处既有的不一致。
4. **手写功能多**：token 刷新、握手、分片重组、ping/pong、事件 ack 全部自管。

### 1.3 改造目标动机

把"我们自维护一套逆向协议栈"换成"依赖一个**原生支持长连接**的社区 Rust SDK（open-lark）"，从而：

- 删除约 7 个逆向协议文件（`auth.rs`/`client.rs`/`proto.rs`+`build.rs`+`proto/`/`messages.rs`/`events.rs`/`types.rs` 的大部分）；
- 把 token 自动刷新、WS 握手、分片、ping/pong、事件 ack 交给库内部处理；
- **保留全部上层接缝零改动**（见 §4）。

---

## 2. 方案选型对比（一句话结论）

| 方案 | 一句话结论 | 工作量 | 风险 |
|---|---|---|---|
| **A. 修旧（fix-existing）** | **官方首选**：根因是单点线缆编码 bug，原地修最小、零接缝改动、Windows 原生、进程内；alternatives 都是用更大改动去修一个已验证的小问题。 | SMALL 0.5–1.5d | LOW |
| **B. 迁移 open-lark（本 PRD）** | **本 PRD 主线 / 最强次选**：进程内、Windows 原生、依赖栈与工作区重叠；可删大量逆向代码；但 bus-factor=1、pre-1.0、crates.io 停在 0.14.0（~8 个月未发版）、且**重新实现了同一套 pbbp2 协议栈 → 不降低协议漂移风险，只是把它转交给第三方**。仅当连接器维护真的变成负担时才值得现在做。 | MEDIUM 2–4d | MEDIUM-HIGH |
| **C. 飞书 CLI（larksuite/cli）** | **不作为主路径**：它**确实能**通过 `event consume` 以 NDJSON 流接收 `im.message.receive_v1` 长连接事件（capability 上不被否决），但其共享 bus 守护进程走 **POSIX Unix-domain-socket，Windows 支持未验证**；需在部署镜像中加 Go 二进制 + Node.js，并监管一个长生命周期子进程（`kill -9` 会泄漏服务端订阅）。对本 Windows、进程内、零依赖的 always-on Rust 服务是硬不匹配。 | MEDIUM-HIGH 3–5d | HIGH（本项目） |

> **关于飞书 CLI 能否接收入站长连接事件的明确结论**：**能**。`larksuite/cli` 的 `lark-event` skill 提供 `lark-cli event consume im.message.receive_v1 --as bot`，以 WebSocket 长连接订阅并把事件以 NDJSON（每行一个 JSON）输出到 stdout。因此它**不是因为"收不到事件"被否决**，而是因为**部署/运行形态不匹配**（Go 二进制 + Node.js 进程足迹、POSIX-UDS-on-Windows 未验证、子进程监管与 `kill -9` 泄漏订阅的运维坑）。详见 §11 非目标。

**本 PRD 选定 B（open-lark）作为改造主线**，理由见 §3。

---

## 3. 为什么是 open-lark（而非修旧 / 而非飞书 CLI）

### 3.1 为什么本 PRD 写 open-lark 而非修旧

- 任务要求交付"改造（换新方式）"方案；修旧方案在配套诊断中已单独给出。
- 在"决定换库"的前提下，open-lark 是唯一**进程内 + Windows 原生 + Rust 类型化事件分发**的可行替代：它把我们手写的同一套协议原语（POST `/callback/ws/endpoint`、prost 帧、ping/heartbeat、分片重组、事件 ack）全部内置在 `LarkWsClient::open(Arc<Config>, EventDispatcherHandler)` 中。
- **诚实说明**：如果只是为了让飞书"能用"，修旧（A）更小更稳。换 open-lark 的真正价值在于**长期卸下协议自维护**，代价是引入一个 bus-factor=1 的外部依赖。

### 3.2 为什么 open-lark 优于飞书 CLI（对本项目）

| 维度 | open-lark | 飞书 CLI |
|---|---|---|
| 集成形态 | **进程内 Rust crate**（与现状同构） | 进程外子进程（spawn Go 二进制） |
| Windows 原生 | 是 | bus 守护进程走 POSIX UDS，**Windows 未验证** |
| 入站长连接事件 | 库内 `register_p2_im_message_receive_v1` 类型化分发 | `event consume` NDJSON（能收，但需解析 stderr ready 标记 + stdout NDJSON） |
| 部署足迹 | 仅多一个 crate | 需 Node.js + 下载 Go 二进制 |
| 运维坑 | 仅需保留我们的重连循环 | `kill -9` 泄漏服务端订阅、一个 EventKey 一个进程、需喂不关闭的 stdin |
| 维护方 | 单人社区（foxzool），无 SLA | 官方（ByteDance/Feishu），MIT |

**结论**：飞书 CLI 唯一胜出项是"官方维护"，但其进程外 + Go/Node 足迹 + POSIX-UDS-on-Windows 风险对本项目是致命错配。open-lark 与现状架构同构，是换库路径的正确选择。飞书 CLI 仅作为**可选的出站 / 运维 sidecar**（§11）。

### 3.3 必须坦白的 open-lark 风险（红队）

1. **bus-factor=1、pre-1.0**：单维护者，minor 版本常带破坏性变更。
2. **crates.io 停滞**：最新发布版 **0.14.0（2025-09-30，约 8 个月未更新）**；GitHub main 已是 0.15–0.17 的多 crate 重写（`openlark-core/-auth/-protocol/...`）但**未发布到 crates.io**。`cargo add open-lark` 只能解析到 0.14.0。升级路径非平凡（可能需 git-pin 或 vendor）。
3. **同一套协议栈**：open-lark 内部也是逆向 pbbp2 → **不降低协议漂移风险，只是转移所有权**。
4. **`LarkWsClient::open()` 不自动重连**：单连接，drop 即返回。**我们必须保留外层重连循环**（`start_feishu_connector` + `reconnect.rs` 的退避策略），把 `open()` 包在里面。
5. **TLS / 依赖冲突（关键，且此前被低估）**：open-lark 0.14.0 用 `tokio-tungstenite 0.23 + rustls-tls-native-roots`、`reqwest 0.12.7 + rustls-tls`；而本连接器现用 `tokio-tungstenite 0.26 + native-tls`，工作区根用 `reqwest default-features=false + rustls-tls-webpki-roots-no-provider` + `rustls 0.23 "ring"`。引入 open-lark 会**新增一个重复的 tungstenite minor（0.23 与现 0.26 并存）并存在 rustls crypto-provider 冲突 / 第二条 TLS 链（ring vs 显式 provider）的风险**——这不是"几乎无重复依赖膨胀"，是一笔真实的适配成本（见 §6.1、§13）。
6. **reply / card API 文档较弱**：`im.v1.message.reply` 与 `msg_type="interactive"` 由示例确认但文档不显眼，需冒烟测试。

---

## 4. 目标与非目标

### 4.1 目标（Goals）

1. 用 open-lark（features = `["im","websocket"]`）替换 `crates/feishu-connector` 的底层连接、鉴权、消息收发、事件解析。
2. **保留以下公共接缝零改动**（blast radius）：
   - `ChatConnector` trait（`send_message`/`send_reply`/`provider_name`/`is_connected`）；
   - `FeishuEvent` / `parse_message_event` / `parse_text_content` / `EVENT_TYPE_MESSAGE` 这一事件消费 seam（或保留兼容映射层）；
   - `BusMessage::TerminalMessage{message}`（转发飞书聊天进 orchestrator）；
   - `ConciergeAgent::process_message`（统一路由入口）；
   - `server::feishu_handle::{FeishuHandle, SharedFeishuHandle, new_shared_handle}`（被 4 个 route 文件 + 全部 server 集成测试使用）；
   - `build_router` / `router` 签名（22 处测试调用点）；
   - `FeishuSender` / `FeishuTarget`（concierge 解耦边界）；
   - 全部 REST 端点、DB schema、加密、设置 wiring。
3. 保留外层重连循环（`open()` 不自动重连）。
4. 随迁移交付：飞书开发者后台前置条件指引 + "connected-but-no-events"诊断 + `ExceedConnLimit 1000040350` / `AuthFailed 514` / `Handshake-*` 错误码日志 + 快速 ACK 后异步处理（尊重 3s ACK 窗口，由库内部处理，我们只需保证回调不阻塞）。

### 4.2 非目标（Non-Goals）

1. **不**改 `ChatConnector` / `FeishuEvent` / `SharedFeishuHandle` / `build_router` 等任何公共签名。
2. **不**改 DB schema：复用 `feishu_app_config`、`concierge_session.feishu_*`、`planning_draft.feishu_*`、`external_conversation_binding`、`system_settings.feishu_enabled`。
3. **不**改加密体系：`.enckey` 自供给方案（cc2400695）与本迁移正交，保持不变。
4. **不**引入飞书 CLI 作为主入站路径（理由见 §3.2、§11）；仅作为可选出站 / 运维 sidecar 的后续跟进项。
5. **不**改 `chat_integrations.rs`（telegram-only，HMAC，复用 `ExternalConversationBinding`）——必须不破坏它。
6. **不**改前端的 4 个 `feishuApi` 方法语义与 REST 契约（仅可能新增"控制台前置条件指引"文案，见 §8）。

---

## 5. 总体架构

### 5.1 设计原则：只换底层连接器，保留所有上层接缝

```
            ┌─────────────────────────────────────────────┐
            │  保留不动（公共接缝 / blast radius）           │
            │                                               │
   REST ────┤  routes/{feishu,concierge,planning_drafts,    │
   前端      │         health}.rs  →  SharedFeishuHandle     │
            │  ConciergeAgent::process_message              │
            │  BusMessage::TerminalMessage                  │
            │  ChatConnector trait / FeishuSender           │
            │  DB: feishu_app_config / concierge_* / ...    │
            └───────────────────┬───────────────────────────┘
                                │  (内部实现替换)
            ┌───────────────────┴───────────────────────────┐
            │  services::feishu  (FeishuService / FeishuConnector) │
            │  —— 适配层：把 open-lark 事件映射为 FeishuEvent，    │
            │     把 ChatConnector 调用转为 open-lark REST 调用    │
            └───────────────────┬───────────────────────────┘
                                │  (替换 crates/feishu-connector 内部)
            ┌───────────────────┴───────────────────────────┐
            │  open-lark (open_lark)                          │
            │   LarkClient::builder(app_id, app_secret)       │
            │     .with_app_type(SelfBuild)                   │
            │     .with_enable_token_cache(true)              │
            │   LarkWsClient::open(config, EventDispatcher)   │
            │   client.im.v1.message.{create, reply}          │
            └────────────────────────────────────────────────┘
                                │
            ┌───────────────────┴───────────────────────────┐
            │  外层重连循环（保留 reconnect.rs 退避策略）        │
            │  start_feishu_connector：把 open() 包进退避循环    │
            └────────────────────────────────────────────────┘
```

### 5.2 映射关系（旧 → open-lark）

| 旧（feishu-connector） | open-lark 对应 |
|---|---|
| `auth.rs`：token 缓存 + `acquire_ws_endpoint` | `LarkClient::builder(...).with_enable_token_cache(true)`（内部自管 token）；endpoint 由 `LarkWsClient::open` 内部 POST `/callback/ws/endpoint` |
| `client.rs`：WS 连接 + prost 帧 + ping + 分片 + ack | `LarkWsClient::open(Arc<Config>, EventDispatcherHandler)`（内部 client_loop + ping_interval + HEARTBEAT_TIMEOUT + 状态机 + ack） |
| `events.rs`：`FeishuEvent` + `parse_message_event` | `EventDispatcherHandler::builder().register_p2_im_message_receive_v1(\|evt\| {...})`，从 `evt.event.message.{content,message_id,chat_id,message_type}` + `evt.event.sender.sender_id.open_id` 读取 → **映射回我们的 `FeishuEvent`/`ReceivedMessage` 以保留下游 seam** |
| `messages.rs`：`send_text`/`reply_text`/`send_card` | `client.im.v1.message.create(CreateMessageRequest, None)`（`msg_type="text"`/`"interactive"`）；reply 走 `client.im.v1.message` 的 reply 端点（`im_v1_demo` 示例确认） |
| `reconnect.rs` | **保留**（`open()` 不自动重连） |
| `proto.rs`/`build.rs`/`pbbp2.proto` | 删除（由 open-lark 的 `lark-websocket-protobuf` sibling crate 提供） |

---

## 6. 详细改造步骤

### 6.1 新增依赖与 features

在 `crates/feishu-connector/Cargo.toml`（或新建 `crates/feishu-connector` 的薄适配层）中：

```toml
[dependencies]
# 用 open-lark 取代手写 WS / auth / messages / proto
open-lark = { version = "0.14.0", default-features = false, features = ["im", "websocket"] }
# 保留：事件映射 / 重连 / 上层类型
tokio = { workspace = true }
serde = { workspace = true }
serde_json = { workspace = true }
tracing = { workspace = true }
anyhow = { workspace = true }
# 以下可在删除手写协议后移除：tokio-tungstenite / prost / bytes / flate2 / futures-util / url
# rand / tokio-util 视 reconnect.rs 是否保留而定（保留则留 rand 抖动）

[build-dependencies]
# 删除 prost-build（不再需要本地 proto 生成）
```

> **依赖冲突预案（必须在第 0 步先验证，见 §13 风险）**：
> - open-lark 0.14.0 拉入 `tokio-tungstenite 0.23` 与现连接器 `0.26` 并存 → 重复 tungstenite minor。删除手写 WS 后，连接器侧不再直接依赖 0.26，但需 `cargo tree -d` 确认全工作区无第二个 tungstenite/TLS 链。
> - rustls crypto-provider：工作区显式 `rustls 0.23 "ring"`；open-lark 经 reqwest `rustls-tls` + tungstenite `rustls-tls-native-roots` 可能引入第二条链或 provider 冲突。**第 0 步先 `cargo add open-lark --features im,websocket --dry-run` + `cargo tree -d -e features`，确认 provider 唯一**；若冲突，按 open-lark 的 feature 选择 rustls 变体或 vendor 适配。
> - 若 0.14.0 的 API 与 GitHub main（0.17）差异过大，需在 0.14.0 上落地（本 PRD 全部 API 以 v0.14.0 tag 为准）。

### 6.2 用 open-lark 替换 auth / ws-client / messenger / events

**做法：保留 `crates/feishu-connector` 这个 crate 名与对外模块路径，只替换其内部实现**（这样 `services/feishu.rs` 的 `use feishu_connector::{...}` 改动最小）。

1. **auth**：删除 `auth.rs` 的手写 token 缓存与 `acquire_ws_endpoint`。改为构造 `LarkClient`：
   ```rust
   let client = LarkClient::builder(&cfg.app_id, &cfg.app_secret)
       .with_app_type(AppType::SelfBuild)
       .with_enable_token_cache(true)
       .build();
   ```
   （token 获取 / 刷新由库内部完成；不再需要 `/auth/v3/tenant_access_token/internal` 手写调用。）

2. **ws-client**：删除 `client.rs` 的 WS 循环 / prost / ping / 分片 / ack。改为：
   ```rust
   let handler = EventDispatcherHandler::builder()
       .register_p2_im_message_receive_v1(move |evt| {
           // 映射为我们的 FeishuEvent / ReceivedMessage（保留下游 seam）
           let mapped = map_p2_to_feishu_event(&evt);
           let _ = event_tx.try_send(mapped); // 不阻塞回调 → 尊重 3s ACK 窗口
       })
       .build();
   LarkWsClient::open(config, handler).await?; // 单连接，drop 即返回
   ```
   - **快速 ACK**：回调内**只做映射 + 非阻塞投递到 channel**，绝不在回调里 `await` Concierge / LLM。重活在 `process_events_inner` 异步消费。

3. **messenger**：删除 `messages.rs` 的手写 REST。新建薄封装：
   ```rust
   pub async fn send_text(&self, chat_id: &str, text: &str) -> Result<String> {
       let body = CreateMessageRequestBody::builder()
           .receive_id(chat_id)
           .receive_id_type("chat_id")
           .msg_type("text")
           .content(json!({"text": text}).to_string())
           .build();
       let resp = self.client.im.v1.message
           .create(CreateMessageRequest::builder().request_body(body).build(), None).await?;
       Ok(resp.data.message_id) // 字段名以 v0.14.0 为准，落地时核对
   }
   // send_card: msg_type="interactive", content = card.to_string()
   // reply_text: client.im.v1.message.reply(message_id, ...)（需冒烟测试）
   ```

4. **events**：**保留 `events.rs` 的 `FeishuEvent`/`ReceivedMessage`/`parse_message_event`/`parse_text_content`/`EVENT_TYPE_MESSAGE` 类型不变**（它们是下游 seam）。新增 `map_p2_to_feishu_event()` 把 open-lark 的 `P2ImMessageReceiveV1` 结构映射成 `FeishuEvent`。这样 `services/feishu.rs` 的事件处理逻辑、`routes/feishu.rs` 的 test-receive 都无需改动。

### 6.3 `feishu.rs` 适配

`services/feishu.rs` 的公共面（`FeishuService::new/from_db/start/messenger/connected_flag/set_event_broadcaster/set_concierge_agent`，以及 `FeishuConnector impl ChatConnector`）**全部保留签名**。内部改动：

- `FeishuService::new`（`feishu.rs:49`）：把 `FeishuClient::new` 换成构造 open-lark `LarkClient` + `EventDispatcherHandler` + mpsc channel；`FeishuMessenger::new` 换成新薄封装（持有 `LarkClient`）。
- `start()`（`feishu.rs:106-131`）：`self.client.connect()` 换成 `LarkWsClient::open(config, handler)`；保持 `tokio::select!{ connect_fut, process_events_inner(...) }` 结构不变。
- `connected_flag()`（`feishu.rs:831`）：open-lark `open()` 无现成 connected flag → 在 open/返回处自行翻转一个 `Arc<RwLock<bool>>`（connect 成功置 true，`open()` 返回置 false），保持 `is_connected()` 语义。
- `FeishuConnector::send_message/send_reply`（`feishu.rs:865-876`）：内部转调新 messenger 的 open-lark 调用，签名不变。

### 6.4 `main.rs` 启动适配

`start_feishu_connector`（`main.rs:430-499`）**整体保留**，仅两处内部调整：

1. **保留外层重连循环**：`open()` 不自动重连，现有 `loop { service.start().await; policy.next_delay(); ... }`（`main.rs:474-496`）**正好就是我们需要的外层退避包装**，保持不变。
2. **修复既有缺陷（顺手做）**：`main.rs:475` 当前用 `ClientConfig::default()` 构造 `ReconnectPolicy` 且从不调用 `update_config()`。迁移后从 open-lark endpoint 返回的 `ClientConfig`（`reconnect_count/reconnect_interval/reconnect_nonce/ping_interval`）喂入 `policy.update_config()`，遵循服务端指引、避免触发连接数限制。
3. `decrypt_feishu_secret`（`main.rs:422`）+ `FeishuService::from_db`（传 decrypt 闭包）+ `is_feishu_enabled` 门控（`main.rs:250`）**全部不变**。

### 6.5 删除哪些旧文件

迁移完成后删除（或清空内部仅留薄封装）：

| 文件 | 处置 |
|---|---|
| `crates/feishu-connector/src/auth.rs` | 删除（token / endpoint 由 open-lark 内置） |
| `crates/feishu-connector/src/client.rs` | 删除（WS / prost / ping / 分片 / ack 由 open-lark 内置）；`connected_flag` 逻辑迁到 service 层 |
| `crates/feishu-connector/src/messages.rs` | 重写为 open-lark 薄封装（保留 `FeishuMessenger` 类型名与方法签名） |
| `crates/feishu-connector/src/proto.rs` | 删除 |
| `crates/feishu-connector/build.rs` | 删除 |
| `crates/feishu-connector/proto/pbbp2.proto` | 删除 |
| `crates/feishu-connector/src/types.rs` | 精简（保留 `FeishuConfig`；`ClientConfig` 若 reconnect.rs 仍用则保留；`WsEndpointResponse` 删除） |
| `crates/feishu-connector/src/reconnect.rs` | **保留**（外层退避） |
| `crates/feishu-connector/src/events.rs` | **保留** + 新增 `map_p2_to_feishu_event()` |
| `crates/feishu-connector/src/lib.rs` | 更新 `pub mod`：移除 `auth`/`client`/`proto`，保留 `events`/`messages`/`reconnect`/`types` |

> 净效果：删除约 5–6 个手写协议文件，`messages.rs` 重写，`events.rs`/`reconnect.rs`/`types.rs` 保留。

### 6.6 DB / 前端是否改动

- **DB：零改动**。复用 `feishu_app_config(app_id, app_secret_encrypted, base_url, enabled)`、`concierge_session.feishu_*`、`planning_draft.feishu_*`、`external_conversation_binding`、`system_settings.feishu_enabled`。无新 migration。
- **前端：契约零改动**。`feishuApi`（getStatus/updateConfig/reconnect/testSend/testReceive）、`conciergeApi`（getFeishuChannel/switchFeishuChannel）REST 形状不变。**唯一建议新增**：在 `FeishuSettingsNew.tsx` / `SetupWizardStep4Integrations.tsx` 增加"开发者后台前置条件 checklist"文案（见 §8），属增强非破坏。

---

## 7. 兼容与数据迁移（feishu_app_config 复用）

- `feishu_app_config` 表结构、加密字段 `app_secret_encrypted`、`base_url`、`enabled` 完全复用。
- `FeishuService::from_db` 继续从该表读 enabled 行、用 `decrypt_feishu_secret` 解密 → 构造 `LarkClient::builder(app_id, app_secret)`。
- **加密兼容**：`.enckey` 自供给方案（cc2400695）正交保留。**注意残留风险**：若某次运行设置了临时 env `SOLODAWN_ENCRYPTION_KEY`、下次未设（env 优先于文件，`encryption.rs:50`），仍会解密失败导致连接器静默不启动 → 在迁移的诊断项中**显式日志化"解密失败 → 跳过启动"**。
- 无需数据回填 / schema 变更 / 用户重新输入凭据。**就地热切换**：部署新版本后，原有已配置的飞书应用直接生效。

---

## 8. 飞书开发者后台前置条件（无论选哪条路都必须满足）

以下为**控制台手工配置，API 不可自动化**，是"连上了但收不到事件"的常见真因：

1. 创建**企业自建应用**并**发布**（未发布 → 长连接可能连上但无事件）。
2. **长连接 / 长连接模式**在客户端在线时**开启**（事件订阅方式选"使用长连接接收事件"）。
3. **订阅 `im.message.receive_v1`** 事件。
4. **机器人能力**已开启 + 相关 IM 权限 scope **已审批通过**。
5. App ID（`cli_xxx`）+ App Secret 填入设置页。

**交付要求**：在 `FeishuSettingsNew.tsx` / `SetupWizardStep4Integrations.tsx` 增加上述 checklist 文案与指向飞书文档的链接；在后端增加"connected-but-no-events"诊断（见 §9）。

---

## 9. 验收标准与端到端验证

### 9.1 验收标准（Definition of Done）

1. `cargo build -p feishu-connector -p services -p server` 通过（Windows，`RUST_MIN_STACK=256MB`、`-j 1`）。
2. `cargo tree -d -e features` 无重复 tungstenite/TLS 链、rustls provider 唯一（§6.1 预案验证通过）。
3. 全部既有 server 集成测试通过（22 处 `build_router`/`new_shared_handle` 调用点签名不变）。
4. 新增 messenger 冒烟测试：`send_text` / `send_card`(interactive) / `reply_text` 对真实 tenant_access_token + chat_id 成功（返回 `message_id`，`code==0`）。
5. 端到端：在飞书向机器人发文本 → 服务端收到 `im.message.receive_v1` → 经 Concierge 路由 → 机器人回复成功。
6. "connected-but-no-events"诊断：WS 已连接但 N 秒无事件时，输出可操作日志（提示检查 §8 控制台前置条件）。
7. 错误码日志：`ExceedConnLimit 1000040350` / `AuthFailed 514` / `Handshake-*` 头被识别并记录。
8. 重连：手动断网后，外层退避循环自动重连成功。

### 9.2 端到端验证步骤

| # | 步骤 | 期望 |
|---|---|---|
| 1 | 启动 server，DB 中有 enabled 飞书配置 | 日志显示 WS 连接成功，`is_feishu_enabled` 门控通过 |
| 2 | `GET /api/integrations/feishu/status` | `connected=true` |
| 3 | `POST /api/integrations/feishu/test-send` | 飞书侧收到测试消息 |
| 4 | 在飞书向机器人发"hello" | server 收到事件，test-receive / 事件广播可见 |
| 5 | 发 `/help` 等 slash 命令 | FeishuService 正确响应 |
| 6 | 触发一次工作流完成通知 | feishu-synced session 收到 completion 推送 |
| 7 | 杀掉网络后恢复 | 外层重连循环重新连上 |

---

## 10. 测试计划

| 层级 | 测试 | 说明 |
|---|---|---|
| 单元 | `map_p2_to_feishu_event()` 映射正确性 | open-lark `P2ImMessageReceiveV1` → `FeishuEvent` 字段对齐（chat_id/message_id/content/open_id），含空字段防御（对齐 `parse_message_event` 现有 `ok_or_else` 校验） |
| 单元 | `parse_message_event` / `parse_text_content` 回归 | 保留不变，确认映射后仍通过 |
| 集成 | messenger 冒烟 | `send_text`/`send_card`/`reply_text` 对真实凭据（CI 用 secret 或手动） |
| 集成 | server 既有测试套件 | `slash_commands_test`(12) / `auth_test`(7) / `workflow_api_test`(2) / `quality_gates_test`(1)，确认 `new_shared_handle`/`build_router` 签名不变全绿 |
| 端到端 | §9.2 全流程 | 真实飞书企业自建应用 |
| 负向 | 控制台前置条件缺失 | 故意不订阅事件 → 验证"connected-but-no-events"诊断触发 |
| 负向 | 凭据 / 解密失败 | 验证"解密失败 → 跳过启动"被显式日志化 |

> 注意 `event_tx` 类型 seam：`feishu_handle.rs` 用 `broadcast::Sender<FeishuEvent>`，client 侧用 `mpsc::Sender<FeishuEvent>`，二者间有桥接（main.rs/service wiring）。test-receive 依赖该桥接——映射改动后需确认桥接仍工作。

---

## 11. 非目标 / 可选后续：飞书 CLI 作为出站 / 运维 sidecar

**明确非目标**：不把 `larksuite/cli` 作为主入站路径。

**为什么不（即使它能收事件）**：
- 它**确实能**接收入站长连接事件（`lark-cli event consume im.message.receive_v1 --as bot` → NDJSON），因此**不是 capability 问题**；
- 但它是 CLI / AI-agent 工具而非 server SDK：需监管长生命周期 Go 子进程、解析 stderr `[event] ready` 标记 + stdout NDJSON、部署镜像加 Node.js + Go 二进制；
- 其共享 bus 守护进程走 **POSIX UDS，Windows 支持未验证**——本项目在 Windows 开发 / 运行，是硬错配；
- `kill -9` 会跳过 OAPI 退订 → **泄漏服务端订阅**（重启报"subscription already exists" + 重复投递）；一个 EventKey 一个进程。

**可选后续（仅当部署迁到 Linux 或需要广义运维能力时）**：飞书 CLI 拥有 2500+ API / 200+ 命令，可作为**出站 / 运维 sidecar**（批量发消息、文档 / 多维表 / 日历操作等），与 open-lark 入站路径并存。届时另立 PRD。

---

## 12. 风险与回滚

### 12.1 风险

| 风险 | 等级 | 缓解 |
|---|---|---|
| **换库不解决控制台配置问题**（红队核心顾虑） | 高 | open-lark 与控制台前置条件正交。**必须**随迁移交付 §8 checklist + §9 "connected-but-no-events"诊断 + 错误码日志。不得宣称"迁移后即可用"，须以诊断 + 控制台指引为前提。 |
| open-lark bus-factor=1 / pre-1.0 / crates.io 停在 0.14.0 | 高 | 严格 pin `=0.14.0`；评估 vendor / git-pin；保留回滚分支；在 MEMORY.md 记录升级路径风险。 |
| TLS / rustls provider 冲突、重复 tungstenite | 中-高 | §6.1 第 0 步 `cargo tree -d` 预验证；必要时调 feature 或 vendor。 |
| open-lark 内部仍逆向 pbbp2 → 协议漂移未消除 | 中 | 接受现实：迁移是转移所有权而非消除风险；保留外层重连 + 诊断以快速定位。 |
| reply / card API 与 v0.14.0 实际签名不符 | 中 | 落地前对照 v0.14.0 tag 源码核对字段名；冒烟测试先行。 |
| `open()` 不自动重连 | 中 | 保留 `reconnect.rs` + 外层 loop（已在 §6.4 处理）。 |
| 加密 env-key 残留风险 | 低 | §7 显式日志化解密失败。 |

### 12.2 回滚

- **代码层**：迁移在独立分支进行；`crates/feishu-connector` 的旧实现保留在 git 历史。回滚 = revert 迁移 commit / 切回旧分支，重新构建即恢复手写连接器。
- **数据层**：零 schema 变更 → **无需任何数据回滚**。`feishu_app_config` 等表在新旧实现间完全兼容。
- **配置层**：用户凭据、`enabled` 标志、加密密钥均不变，回滚后原配置直接复用。
- **建议**：合并前在 staging 用真实企业自建应用跑通 §9.2 全流程；保留旧 crate 至少一个发布周期再彻底删除。

---

## 13. 分阶段里程碑与工作量估算

> 总估算 **MEDIUM：约 2–4 人天**（不含等待飞书控制台审批的人工时间）。Windows 全量 codegen 受 `RUST_MIN_STACK=256MB` + `-j 1` 约束，迭代偏慢，估算已含此税。

| 阶段 | 内容 | 产出 | 估算 |
|---|---|---|---|
| **M0 依赖验证** | `cargo add open-lark --features im,websocket` + `cargo tree -d -e features` 确认无 TLS/tungstenite 冲突；核对 v0.14.0 的 im/websocket API 签名 | 依赖可解析、provider 唯一的确认；API 签名清单 | 0.25–0.5d |
| **M1 连接器替换** | 用 open-lark 重写 auth/ws/messenger；新增 `map_p2_to_feishu_event`；删除手写协议文件；保留 events/reconnect/types | `feishu-connector` 编译通过 | 0.75–1.5d |
| **M2 上层适配** | `feishu.rs`（new/start/connected_flag/FeishuConnector）+ `main.rs`（ClientConfig 喂入 ReconnectPolicy）适配；保持全部公共签名 | services/server 编译通过，既有测试全绿 | 0.5–1d |
| **M3 诊断与 UX** | "connected-but-no-events"诊断 + 错误码日志 + 快速 ACK；前端控制台 checklist 文案 | 诊断可触发；设置页 checklist | 0.25–0.5d |
| **M4 验证** | messenger 冒烟 + §9.2 端到端 + 负向用例；staging 真实应用跑通 | 验收报告 | 0.25–0.5d |

---

## 14. 附：受影响文件清单（落地核对用）

**改 / 删（连接器层）**：
`crates/feishu-connector/Cargo.toml`、`src/lib.rs`、`src/auth.rs`(删)、`src/client.rs`(删)、`src/proto.rs`(删)、`build.rs`(删)、`proto/pbbp2.proto`(删)、`src/messages.rs`(重写)、`src/events.rs`(+映射)、`src/types.rs`(精简)、`src/reconnect.rs`(保留)。

**改（适配层，签名不变）**：
`crates/services/src/services/feishu.rs`、`crates/server/src/main.rs`(L422/430-499)。

**零改动（接缝 / DB / 测试 / 前端契约）**：
`chat_connector.rs`、`concierge/*`、`orchestrator/message_bus.rs`、`feishu_handle.rs`、`routes/{feishu,concierge,planning_drafts,health,chat_integrations,system_settings,mod}.rs`、全部 DB models + migrations、全部 server tests、前端 `feishuApi`/`conciergeApi` 契约。

**增强（非破坏）**：
`FeishuSettingsNew.tsx` / `SetupWizardStep4Integrations.tsx` 控制台前置条件 checklist。
