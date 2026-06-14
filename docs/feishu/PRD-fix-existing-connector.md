# PRD：修复自建飞书长连接 Connector（路径：fix-existing）

> 适用范围：保留并修复 `crates/feishu-connector` 这套手写的、逆向飞书「长连接」(长连接 / WebSocket 事件订阅) 协议的实现。
> 这是「如果走旧方式」的交付物。与之并列的「走新方式」方案（迁移到社区 `open-lark` 或官方 Go CLI sidecar）见另一份 PRD，本文不展开。
>
> 文档状态：实现就绪（implementation-ready）
> 目标读者：负责 connector / server / db / frontend 的工程师
> 日期：2026-06-14 ｜ 分支基线：`main`（最近提交 `4d9248493`）

---

## 1. 背景与现状

SoloDawn 的飞书集成由一个**完全手写、零外部飞书 SDK 依赖**的 Rust crate `crates/feishu-connector` 实现。它逆向了飞书开放平台的「长连接」事件订阅协议：

1. **HTTP bootstrap**：`auth.rs::acquire_ws_endpoint()` 用 `{AppID, AppSecret}` POST 到 `{base_url}/callback/ws/endpoint`，返回 WS 网关地址（URL 携带 `service_id` 查询参数）与一份 `ClientConfig`。
2. **WSS 长连接**：`client.rs` 用 `tokio-tungstenite` 建连，按 `pbbp2` protobuf `Frame` 协议收发，含 ping 心跳、分片重组（`FragmentCache`）、事件 ACK（回写 `biz_rt` 头）。
3. **出站消息**：`messages.rs` 走标准 REST `/open-apis/im/v1/messages`（`send_text`/`reply_text`/`send_card`），与长连接无关，**这部分是正确的**。

### 现状结论（已由协议研究交叉验证，置信度高）

- **协议方向是对的**：endpoint 路径、`{AppID,AppSecret}` body 鉴权、`pbbp2` Frame、method `Control=0/Data=1`、header 键（`type`/`message_id`/`sum`/`seq`/`biz_rt`）、消息类型（`ping`/`pong`/`event`）、ACK = 回显 Frame + `biz_rt` 头 + `{code:200}` payload、ping 默认 120s、`ClientConfig` 字段，**逐字段对齐官方 `larksuite/oapi-sdk-go v3` `ws/client.go` + `ws/const.go`**。协议**不是过时或跑偏**的。
- **但它从未真正跑通**：用户报告「这套飞书连接方式有错误、至今无法正常使用」。
- **没有官方 Rust SDK**：飞书官方仅有 Go/Python/Java/Node.js server SDK。Rust 侧只有社区 `open-lark`。所以「修旧」并不会落后于任何官方基线。

### 根因总览（详见第 2 节）

| # | 根因 | 严重度 | 置信度 |
|---|------|--------|--------|
| RC-1 | **proto3 丢弃零值，导致出站 Frame 缺失 required `SeqID`/`LogID`，服务端判为非法帧** —— ping 超时断连 + 事件 ACK 无效 | blocker | 高 |
| RC-2 | connector **零测试**，协议从未在真实 wire 上验证过 | 高 | 高 |
| RC-3 | 重连循环忽略服务端下发的 `ClientConfig`（外层固定默认值） | 低 | 高 |
| RC-4 | 事件 ACK payload 是临时拼的 `{code}`，未对齐 SDK `Response` 结构 | 中 | 中 |
| RC-5 | 加密 key 解密失败导致静默不启动（历史问题，`cc2400695` 已基本修复，非主因） | 低 | 高 |
| RC-6 | 飞书开发者后台前置配置（发布/长连接/订阅/权限）UX 无任何引导 | 中 | 中 |

**一句话定性**：核心 blocker 是 **RC-1（proto3 vs proto2-required）**。其余为加固项 / 体验项。整体属于小范围、可控的修复。

---

## 2. 问题根因清单（逐条对应修复项，引用 file:line）

### RC-1 ｜ proto3 丢弃零值 → 出站帧缺失 required `SeqID`/`LogID`（blocker）

**证据**
- `crates/feishu-connector/proto/pbbp2.proto:1` 声明 `syntax = "proto3";`；`SeqID`/`LogID` 为 `uint64`（`pbbp2.proto:10-11`）。
- prost 0.13（`crates/feishu-connector/Cargo.toml:14`）在 proto3 下**会省略等于默认值（0）的标量字段**，不写入 wire。
- 官方 `oapi-sdk-go` 的 `ws/pbbp2.pb.go` 把 `SeqID`、`LogID`、`service`、`method`、`headers` 以及 `Header.key`/`Header.value` 标为 **required（proto2 语义）**，**始终写入 `SeqID`/`LogID`（即使为 0）**，且 `Unmarshal` 在缺失 `SeqID`/`LogID` 时返回 `RequiredNotSet`。
- 本地 `client.rs:33` `new_ping_frame()` 用 `..Default::default()` 构造帧 → `SeqID=0`、`LogID=0` 被 prost 丢弃；`client.rs:323-333` 的 ACK 路径同理（直接复用收到的 frame 回写，但若入站帧本身 `SeqID/LogID` 为 0 或被 prost 重编码丢弃，仍非法）。
- 相关提交：`62f426b11`（号称对齐官方 SDK，但写错了 proto2 vs proto3）。

**后果**：入站解码正常（所以「能收到事件」），但
1. **ping 帧非法** → 服务端按 ping 超时断开连接；
2. **事件 ACK 非法** → 飞书重投，多次后**关闭该应用的事件推送**。
这是「至今无法正常使用」的核心原因。

**对应修复**：见 4.1（协议层）。

---

### RC-2 ｜ connector 零测试，wire 格式从未验证（高）

**证据**
- 无 `crates/feishu-connector/tests/` 目录，`src/` 内无任何 `#[test]`/`#[cfg(test)]`。
- `62f426b11` 是「读代码写出来的」而非「跑通验证的」——proto2/proto3 这个错误本身就是证明。
- `pbbp2.proto` 仅在 `62f426b11` 改过一次且未再修正。
- 早期 `235084b81`、`a30bc5830` 只修好了 HTTP bootstrap，从未触及 WS 帧层。

**对应修复**：见 4.5（测试计划）—— 至少加一条 round-trip 测试，断言编码后的 ping 帧 wire bytes 含 `SeqID`(field 1)、`LogID`(field 2)。

---

### RC-3 ｜ 重连循环忽略服务端 `ClientConfig`（低）

**证据**
- `crates/server/src/main.rs:475`：`start_feishu_connector` 用 `ClientConfig::default()` 构造 `ReconnectPolicy`，且**从不**用 endpoint / pong 下发的 config 更新它。
- 内层 `FeishuClient` 的 ping loop 确实会更新（`client.rs:131-133`、`client.rs:248-251`），但**外层重连节奏/次数仍是固定默认**（`reconnect.rs:22`、`types.rs:49`），忽略服务端指导，存在触发连接数限制的风险。

**对应修复**：见 4.3。

---

### RC-4 ｜ 事件 ACK payload 临时拼 `{code}`，未对齐 SDK `Response`（中）

**证据**
- `crates/feishu-connector/src/client.rs:330`：`serde_json::json!({ "code": response_code })`，仅含 `code`。
- 官方 SDK 序列化 `Response{code, headers, data}`（`ws model.go NewResponseByCode`、`client.go handleDataFrame`）。
- `code` 这个子集恰好命中 `StatusCode` 的 json tag，所以单看 JSON 能被容忍；但叠加 RC-1，ACK 帧在 protobuf 层本就非法。

**对应修复**：见 4.1（与 RC-1 一并修）。

---

### RC-5 ｜ 加密 key 解密失败 → 静默不启动（低，历史，非主因）

**证据**
- `crates/server/src/main.rs:422`、`:438`：经 `decrypt_feishu_secret` 解密；解密失败则 connector 静默不启动。
- 历史上 `SOLODAWN_ENCRYPTION_KEY` 跨进程不稳定 → 重启后存储的 `app_secret` 解不开 → bootstrap POST 鉴权失败，表现为「飞书坏了 / API key 每次重启都丢」。
- `cc2400695` 已加入「按机器自供给的 `.enckey` 文件」（`encryption.rs:78-132`）+ legacy-key 回退重试（`encryption.rs:193-214`），已基本稳定。
- 残余风险：若某次运行设了临时 env key、下次没设（env 优先，`encryption.rs:49-50`），仍会咬人。

**对应修复**：见 4.2（加密层加固 + 显式日志），非阻塞。

---

### RC-6 ｜ 飞书开发者后台前置条件 UX 无引导（中）

**证据**
- 长连接模式 + `im.message.receive_v1` 事件订阅 + 应用已发布 + 机器人权限范围，**必须在飞书后台手工配置，无法 API 自动化**。
- 目前唯一「引导」是 `auth.rs:120-127` 在 bootstrap code != 0 时打印的错误串。
- 若 bootstrap 成功但订阅/权限错了，则 **WS 连上但收不到任何事件，且完全静默**。
- 设置 UI（`frontend/src/pages/ui-new/settings/FeishuSettingsNew.tsx`）只收集 app id + secret。

**对应修复**：见 4.4（前置条件清单 + UI 引导 + 「已连接但收不到事件」诊断）。

---

## 3. 目标与非目标

### 3.1 目标（In Scope）

- **G1**：修正 `pbbp2` wire 编码，**保证每个出站帧都显式写入 `SeqID` 与 `LogID`（即使为 0）**，使 ping 与事件 ACK 被服务端接受。（修 RC-1，blocker）
- **G2**：事件 ACK payload 对齐 SDK `Response{code,headers,data}` 形态。（修 RC-4）
- **G3**：把 endpoint 下发的 `ClientConfig` 喂给**外层** `ReconnectPolicy`。（修 RC-3）
- **G4**：为 connector 补上协议层单测 + 至少一条 wire round-trip 断言（ping 帧含 field 1/2）。（修 RC-2）
- **G5**：加密层解密失败时**显式、可见**地告警（而非静默跳过启动）；保留 `cc2400695` 的自供给逻辑。（修 RC-5）
- **G6**：补齐飞书后台前置条件清单文档 + 设置 UI 引导 + 「已连接但 N 秒无事件」的诊断日志/状态。（修 RC-6）
- **G7**：可观测性 —— 记录 Handshake-* 头、显式处理已知服务端错误码（见 4.4）。
- **G8**：实现「先 ACK 再异步处理」模式，确保 3 秒内 ACK。

### 3.2 非目标（Out of Scope）

- **不**迁移到 `open-lark` / 官方 Go CLI sidecar（那是「走新方式」的另一份 PRD）。
- **不**改动任何 public seam（见 blast-radius `publicSurface`）：
  - `ChatConnector` trait、`FeishuEvent`/`EVENT_TYPE_MESSAGE`/`parse_message_event`、
  - `BusMessage::TerminalMessage`、`ConciergeAgent::process_message`、
  - `FeishuHandle`/`SharedFeishuHandle`/`new_shared_handle()`、
  - `build_router(...)` / `router(...)` 签名（**全部 server 集成测试依赖它**）。
- **不**改 DB schema / 不新增 migration（除非 4.4 的诊断字段确有必要，默认不加）。
- **不**碰 `routes/chat_integrations.rs`（独立的 telegram-only webhook 路径，复用 `ExternalConversationBinding`，**修复绝不能破坏它**）。
- **不**重写出站 REST `messages.rs`（已正确）。

---

## 4. 详细修复方案（按文件 / 模块）

### 4.1 协议层（核心，修 RC-1 + RC-4）

#### 4.1.1 保证 `SeqID`/`LogID` 始终上线 —— 三选一，**推荐方案 A**

> prost 0.13 对 proto2-required 支持有限，最稳妥的是**绕过 proto3 的「省略零值」语义**，强制把 field 1（`SeqID`）与 field 2（`LogID`）写到 wire。

**方案 A（推荐）：proto2 + required**
将 `crates/feishu-connector/proto/pbbp2.proto:1` 改为：

```proto
syntax = "proto2";
package feishu.ws;

message Header {
    required string key = 1;
    required string value = 2;
}

message Frame {
    required uint64 SeqID = 1;
    required uint64 LogID = 2;
    required int32  service = 3;
    required int32  method = 4;
    repeated Header headers = 5;
    optional string payload_encoding = 6;
    optional string payload_type = 7;
    optional bytes  payload = 8;
    optional string LogIDNew = 9;
}
```

- proto2 `required` 在 prost 下生成的字段为非 `Option`（标量直接是值），编码时**始终写入**，满足官方 `Unmarshal` 的 `RequiredNotSet` 校验。
- `build.rs`（prost-build）无需改动，仍编译 `proto/pbbp2.proto`。
- **风险**：prost 对 proto2 group/required 的边界 case 支持有限。**编译后必须立即跑 4.5 的 round-trip 测试验证 wire 真的含 field 1/2**。若 prost 编译/生成失败 → 回退方案 B。

**方案 B（兜底，纯 Rust 不改 proto）：保留 proto3，手动强制写字段**
若方案 A 在 prost 0.13 上踩坑，则保留 proto3，但在编码出站帧前**显式赋非省略值**或用自定义 encode 确保 field 1/2 写入。最简单可靠的做法：把出站帧的 `SeqID`/`LogID` 设为**真实非零值**（见下），自然就不会被 prost 丢弃，问题被「消解」而非「绕过」。

#### 4.1.2 给出站帧填真实 `SeqID`/`LogID`（无论 A/B 都做）

- `client.rs:33` `new_ping_frame()`：用一个**单调递增的 seq 计数器**填 `SeqID`，用 bootstrap 返回的 / trace 维度的 `LogID`（或时间戳）填 `LogID`，**不要**再用 `..Default::default()` 让它们停在 0。
  - 在 `FeishuClient` 内维护 `AtomicU64` seq 计数器，每发一帧 `fetch_add(1)`。
- `client.rs:323-333` ACK 路径：**回显入站帧的 `SeqID`/`LogID`**（官方语义是 ACK 沿用原帧的 seq/log id），而非清零。确认 `frame` 在 push `biz_rt` header 后，其 `SeqID`/`LogID` 仍是入站值。

#### 4.1.3 ACK payload 对齐 SDK `Response`（修 RC-4）

`client.rs:330` 改为序列化与官方一致的结构：

```rust
#[derive(serde::Serialize)]
struct AckResponse {
    code: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    headers: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    data: Option<serde_json::Value>,
}
// 成功：AckResponse{ code: 200, headers: None, data: None }
```

- 保持 `code` 字段名不变（命中 `StatusCode` json tag，向后兼容）。
- `biz_rt` header 继续按 `client.rs:325-328` 写入处理耗时。

#### 4.1.4 缺失的 header 键补全（可观测性加固）

官方 `const.go` 还定义了 `timestamp`、`trace_id`、`instance_id` 以及 `Handshake-Status`/`Handshake-Msg`/`Handshake-Autherrcode`。当前 `proto.rs` 省略了 `trace_id`/`timestamp`/handshake 头。

- 在 `proto.rs` 增补这些 header 键常量。
- 出站 ping/ACK 帧附带 `trace_id`、`timestamp`（非致命，但利于服务端侧排障）。
- **WS upgrade 被拒时，读取并 `tracing::error!` 记录 `Handshake-Status`/`Handshake-Msg`/`Handshake-Autherrcode`**（这些头携带鉴权失败细节，是「连不上」最关键的诊断信息）。

---

### 4.2 加密层（修 RC-5，加固为主）

- `crates/server/src/main.rs:422`/`:438` `decrypt_feishu_secret`：解密失败时，由「静默跳过启动」改为
  - `tracing::error!` 打印**明确、可操作**的错误（不泄露密文/明文）：例如「飞书 app_secret 解密失败：加密 key 与存储时不一致。若上次运行设置过 SOLODAWN_ENCRYPTION_KEY 而本次未设置，请保持一致或在设置页重新保存 app_secret」。
  - 同时把该状态反映到 `SharedFeishuHandle`（`connected=false` + 一个 `last_error` 字段，若已有则复用；无则在 `feishu_handle.rs` 内部新增不破坏签名的字段），供 `/feishu/status` 暴露。
- 保留 `encryption.rs:78-132` 自供给 `.enckey` 与 `encryption.rs:193-214` legacy 回退。
- 文档中明确：**生产 / 多机环境必须显式固定 `SOLODAWN_ENCRYPTION_KEY`**；单机依赖 `.enckey` 文件即可（`encryption.rs:49-50` env 优先）。

---

### 4.3 重连控制（修 RC-3）

- `crates/server/src/main.rs:475`：把 `acquire_ws_endpoint()` 返回的 `ClientConfig`（含 `ReconnectCount`/`ReconnectInterval`/`ReconnectNonce`/`PingInterval`）传入**外层** `ReconnectPolicy`，而不是 `ClientConfig::default()`。
  - 由于 endpoint 在每次重连 bootstrap 时都会返回最新 config，应在每轮重连成功 bootstrap 后**刷新** `ReconnectPolicy` 的上限/间隔。
- `reconnect.rs:22` `ReconnectPolicy`：增加一个 `update_from_config(&ClientConfig)` 方法，让外层循环可热更新退避参数。
- 保留 `reconnect.rs` 现有的 120s 上限 + jitter 作为兜底。
- **遵守连接数约束**：单应用最多 50 连接；确保**不重复 spawn** 重连任务（同一时间只有一条活跃连接），避免触发 `ExceedConnLimit`。

---

### 4.4 控制台前置配置 + 可观测性（修 RC-6 + G7 + G8）

#### 4.4.1 显式处理已知服务端错误码

在 `auth.rs`（bootstrap）与 `client.rs`（WS）路径，针对以下 code 给出**可操作**的中文日志：

| code | 含义 | 处理 |
|------|------|------|
| `1000040350` | ExceedConnLimit（超连接数） | 不立即重连，退避；提示检查是否多实例 / 残留连接 |
| `1000040343` | InternalError | 退避重连 |
| `514` | AuthFailed | 停止重连；提示检查 app_secret / 是否企业自建已发布 |
| `403` | Forbidden | 停止重连；提示检查权限范围 |
| `1`/`1000040351` | SystemBusy | 退避重连 |

#### 4.4.2 「已连接但收不到事件」诊断

- WS 连上后启动一个 watchdog：若 **N 秒（默认 120s）内未收到任何 `event` 帧**，记 `tracing::warn!`「长连接已建立但未收到任何事件 —— 极可能是后台未订阅 im.message.receive_v1 / 应用未发布 / 权限未审批通过」，并把该状态暴露到 `/feishu/status`。

#### 4.4.3 先 ACK 再异步处理（G8，3 秒约束）

- `client.rs` 收到 `event` 帧后，**先**计算 `biz_rt` 并回 ACK（4.1.3），**再**把事件投递给下游（`services/feishu.rs` → Concierge / orchestrator bus）做耗时处理。
- 严禁在 Concierge 处理完成后才 ACK —— Concierge 处理可能 > 3s，会触发飞书重投 / 关推送。
- 注意「集群随机单投」语义：**同一应用多实例时，每条事件只投给一个运行中的客户端**；本机务必保证只有一条活跃长连接。

#### 4.4.4 前端 UI 引导（`FeishuSettingsNew.tsx`）

- 在 app_id/secret 表单旁加一段**前置条件 checklist**（见第 5 节文案），并在 `/feishu/status` 返回 watchdog/错误码状态时高亮对应未满足项。
- 不改动 `feishuApi` 的 4 个方法签名（`getStatus`/`updateConfig`/`reconnect`/`testSend`/`testReceive`），仅在 `getStatus` 返回体内**新增可选字段**（如 `lastError`、`noEventSince`），前端渲染。

---

## 5. 飞书开发者后台前置条件清单（必须人工完成，无法 API 自动化）

> 这是「连上但不工作」最常见的真实原因。**长连接模式仅企业自建应用可用，商店 / ISV 应用不支持。**

1. **应用类型**：必须是**企业自建应用**（store / ISV 应用不支持长连接）。
2. **回调订阅方式**：开发者后台 →「事件与回调」→「回调配置」→ 选择**「使用长连接接收事件」**。
   - **关键时序**：点「保存」时，**本机长连接客户端必须已经处于连接状态**，否则保存会失败。建议先在 SoloDawn 设置页启用并确认 `/feishu/status` 已连接，再去后台点保存。
3. **应用已发布 / 版本已审批通过**（未发布的应用收不到事件）。
4. **订阅事件**：勾选 `im.message.receive_v1`（接收消息事件）。
5. **权限范围**：申请并通过 `im:message`（读消息）等所需 scope，且**已审批生效**。
6. **配额约束**：单应用最多 50 个长连接；3 秒内必须 ACK 事件；集群随机单投（多实例只有一个收到）。

---

## 6. 验收标准与端到端验证步骤（本机手动收发验证）

### 6.1 验收标准（Definition of Done）

- **AC-1（RC-1，blocker）**：编码后的 ping 帧 wire bytes **包含 field 1(`SeqID`) 与 field 2(`LogID`)**（round-trip 测试断言通过）。
- **AC-2**：本机连上飞书后**心跳持续 ≥ 10 分钟不被服务端按 ping 超时断开**。
- **AC-3**：向机器人发一条消息，SoloDawn **收到事件、3 秒内 ACK、且服务端不重投**（连续发 5 条，5 条都只投递一次）。
- **AC-4（RC-4）**：ACK payload 为 `{code:200,...}` 且 protobuf 帧合法（抓包 / 日志确认）。
- **AC-5（RC-3）**：日志显示外层 `ReconnectPolicy` 使用了 endpoint 下发的 `ClientConfig` 值（而非默认）。
- **AC-6（RC-5）**：故意改坏 enc key 后重启，`/feishu/status` 与日志**明确报「解密失败」**，而非静默无反应。
- **AC-7（RC-6/G7）**：连上但后台未订阅事件时，watchdog 在 120s 内打出诊断 warn；触发 `ExceedConnLimit`/`514` 时日志给出可操作提示。
- **AC-8**：所有既有 server 集成测试（`slash_commands_test`/`auth_test`/`workflow_api_test`/`quality_gates_test`）**保持绿**（public 签名未变）。

### 6.2 端到端手动验证步骤

1. **后台准备**：按第 5 节完成 1-5 项。
2. **配置 SoloDawn**：设置页填 `cli_xxx` app_id + secret，开启飞书开关。
3. **确认 bootstrap**：调 `GET /api/integrations/feishu/status`，确认 `connected=true`，无 `lastError`。
4. **后台开长连接**：在客户端已连接的前提下，回后台点「保存长连接」（呼应第 5 节步骤 2 的时序要求）。
5. **收消息验证**：在飞书里给机器人发「hello」→ 观察 SoloDawn 日志收到 `im.message.receive_v1`、3s 内 ACK、Concierge 收到转发。重复 5 次确认无重投。
6. **发消息验证**：调 `POST /api/integrations/feishu/test-send`（或在 Concierge 触发推送）→ 飞书侧收到机器人回复。
7. **心跳验证**：保持空闲 ≥ 10 分钟，连接不掉（AC-2）。
8. **断线重连验证**：手动断网 30s 再恢复 → connector 按 `ClientConfig` 退避自动重连成功。
9. **抓包（可选但强烈建议）**：用日志 dump 出站 ping/ACK 帧的 hex，确认含 field 1/2（佐证 AC-1）。

---

## 7. 测试计划

### 7.1 单元 / Round-trip 测试（新增，修 RC-2）—— `crates/feishu-connector/tests/`

- **T1（必需，对应 AC-1）**：`new_ping_frame()` 编码后 round-trip：
  - `encode_to_vec()` 后断言字节流含 field 1 与 field 2 的 wire tag；或 `Frame::decode()` 回来后 `SeqID`/`LogID` 为预期值。
  - 显式构造 `SeqID=0, LogID=0` 的帧也要断言**编码后仍写入了这两个字段**（这是 RC-1 的回归锁）。
- **T2**：ACK 帧 round-trip：构造一个入站 `event` 帧 → 走 ACK 构造逻辑 → 断言回帧含原 `SeqID`/`LogID`、`biz_rt` header、payload 反序列化为 `{code:200}`。
- **T3**：`FragmentCache` 分片重组（已有逻辑 `client.rs:56-90`）—— 乱序 / 缺片 / TTL 过期路径。
- **T4**：`parse_message_event` / `parse_text_content`（`events.rs`）—— 真实样例 JSON。
- **T5**：`ReconnectPolicy::update_from_config` 正确吸收 `ClientConfig`（RC-3）。

### 7.2 加密层测试 —— `crates/db`

- **T6**：解密失败路径返回明确 error（不 panic），覆盖 env-key 与 `.enckey` 不一致场景。

### 7.3 回归测试（必须保持绿，验证 public seam 未破）

- `crates/server/tests/slash_commands_test.rs`、`auth_test.rs`、`workflow_api_test.rs`、`quality_gates_test.rs`（依赖 `new_shared_handle()` 与 `build_router(...)` 签名）。
- `crates/server/src/routes/chat_integrations.rs` 的 `#[cfg(test)]`（telegram bind/unbind / 签名校验，确认未被波及）。

### 7.4 构建注意（来自 MEMORY）

- 全量 codegen（`cargo test --no-run` / 编 bin / clippy）需 `RUST_MIN_STACK=256MB` + `-j 1`，否则 ICE/OOM；纯 `cargo check` 会掩盖此问题。
- Windows 构建需 protoc + LLVM/libclang（`setup-windows.ps1` 未含）；`sqlx-cli` 锁定 0.8.6。

---

## 8. 风险与回滚

| 风险 | 等级 | 缓解 / 回滚 |
|------|------|-------------|
| prost 0.13 对 proto2 `required` 支持有限，方案 A 生成/编译失败 | 中 | 立即回退到 4.1.1 方案 B（保留 proto3 + 填真实非零 `SeqID`/`LogID`），二者择优；用 T1 验证 |
| 改 proto 影响入站解码（向后兼容） | 中 | T1/T2 round-trip 同时覆盖 encode 与 decode；先在本机真实环境跑通 AC-2/AC-3 再合并 |
| 误改 public seam 导致大量测试 / 路由 break | 高 | 严守第 3.2 非目标；改完跑 7.3 全量回归；`build_router`/`new_shared_handle`/`ChatConnector`/`FeishuEvent` 签名零改动 |
| 误伤 `chat_integrations.rs`（telegram） | 中 | 不碰该文件；跑其 `#[cfg(test)]` 确认绿 |
| 后台前置条件未满足导致「改完仍收不到」 | 中 | 第 5 节 checklist + 4.4.2 watchdog 诊断，把问题归因到配置而非代码 |
| 多实例触发 `ExceedConnLimit` | 低 | 4.3 保证单活跃连接 + 4.4.1 错误码处理 |
| 加密 key 跨环境不一致 | 低 | 4.2 显式告警 + 文档要求生产固定 `SOLODAWN_ENCRYPTION_KEY` |

**回滚策略**：所有改动集中在 `feishu-connector` + `main.rs` 局部 + 前端文案，不含 DB migration，可按单一 PR `git revert` 整体回滚，不留 schema 残留。

---

## 9. 工作量估算与里程碑

**总估**：小范围修复，约 **0.5 ~ 1.5 人日**（不含等待飞书后台审批 / 权限生效的外部时延）。

| 里程碑 | 内容 | 估时 |
|--------|------|------|
| **M1 — 协议层 blocker 修复** | 4.1（proto2/required 或填真实 seq/log + ACK Response 对齐）+ T1/T2 round-trip 测试 | 0.25~0.5d |
| **M2 — 本机端到端跑通** | 完成第 5 节后台配置 + 6.2 手动收发验证（AC-2/AC-3/AC-4） | 0.25~0.5d |
| **M3 — 加固项** | 4.3 重连吸收 ClientConfig + 4.2 解密告警 + 4.4 错误码/watchdog/Handshake 日志 + 先 ACK 后处理 | 0.25~0.5d |
| **M4 — 测试 + 体验** | 补齐 7.1~7.3 测试、前端 checklist UI、跑全量回归绿 | 0.25d |

**关键路径**：M1 → M2（M1 不过，后续都无意义）。M3/M4 可在 M2 通过后并行收尾。

> 备注：协议研究结论认为「无法使用」**大概率是后台配置 + 历史 enc-key 问题**叠加 RC-1 的 wire bug。因此 M1（代码 blocker）与第 5 节（后台配置）**必须同时满足**才能验收通过，缺一不可。

---

*文件路径：`E:\SoloDawn\docs\feishu\PRD-fix-existing-connector.md`*
