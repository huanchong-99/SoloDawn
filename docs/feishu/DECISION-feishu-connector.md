# 决策备忘：飞书连接器，旧方式 vs 新方式

> 你的问题：**「今天飞书 CLI 开源了，还需要用旧方式吗？还是直接用新方式？」**

---

## 一句话结论

**修旧（fix-existing）。** 旧的 `crates/feishu-connector` 协议没过时、和官方 Go SDK 逐字段一致，只是差一个**线上编码 bug**（proto3 把零值的必填字段 SeqID/LogID/method 丢掉了）。这是约 **0.5–1.5 天**的小补丁，不需要换 SDK，也不需要引入飞书 CLI。飞书 CLI 与 open-lark 都是「用更大、更高风险的改动去修一个已确诊的小 bug」。

---

## 现状旧方式坏在哪（根因 Top 3）

| # | 根因 | 证据（已在仓库内核实） | 严重度 |
|---|------|------------------------|--------|
| 1 | **proto3 丢弃必填字段** —— 心跳 ping 与事件 ack 的 `SeqID`/`LogID`/`method` 是零值，被 prost 当默认值省略，飞书服务端按「缺必填字段」拒绝 → ping 超时断连、ack 无效（事件重投后被关推送） | `proto/pbbp2.proto:1` 是 `syntax = "proto3"`；`client.rs:33-43` 用 `..Default::default()` 现场构造 ping，SeqID/LogID/method 全为 0。官方 `oapi-sdk-go` 的 pbbp2 把 SeqID(1)/LogID(2)/service(3)/method(4) 标为 required 且总是写出 | **Blocker** |
| 2 | **零测试** —— 协议从未在线上格式上验证过。62f426b11 号称对齐官方 SDK，但是「读代码写出来的、没跑过」，proto2/proto3 这个错就是这么漏的 | `crates/feishu-connector` 无 `tests/`、`src` 内无测试函数 | High |
| 3 | **两个非代码诱因**（代码改不动，但会冒充「飞书坏了」）：① 历史上不稳定的加密 key 导致重启后 app_secret 解不开（已由 cc2400695 修复）；② 飞书开发者后台前置条件未在 UX 引导（必须是已发布的企业自建应用、客户端在线时打开「长连接」、订阅 `im.message.receive_v1`、授权 scope） | `main.rs:422`/`encryption.rs`（已修）；`auth.rs:120-127` 仅有一句错误串；`FeishuSettingsNew.tsx` 只收 app_id+secret | Medium |

---

## 三条路对比

| 维度 | ① 修旧（推荐） | ② 换 open-lark | ③ 接飞书 CLI |
|------|---------------|----------------|--------------|
| **能否收长连接事件** | 能（修好编码即可） | 能（`LarkWsClient::open`） | **能**（`lark-cli event consume im.message.receive_v1`，NDJSON 流式输出） |
| **集成方式** | 进程内 Rust，零新依赖 | 进程内 Rust crate（`features=[im,websocket]`） | 进程外 Go 子进程 + Node.js，UDS 总线 IPC |
| **工作量** | **SMALL ~0.5–1.5 天**，约 5 个文件，0 个公共接口改动 | MEDIUM ~2–4 天，删 ~7 个文件、重写 connector、映射事件结构 | MEDIUM-HIGH ~3–5 天，把 `start_feishu_connector` 改成进程监管 |
| **风险** | **LOW** —— 根因已逐字段对齐官方 Go SDK 核实 | MEDIUM-HIGH —— 单人维护、pre-1.0；crates.io 冻结在 0.14.0（~8 个月未更），GitHub main 是另一套未发布的 0.15–0.17 重写 | **HIGH（对本项目）** —— 总线用 POSIX UDS，**Windows 支持未验证**；`kill -9` 会泄漏服务端订阅 |
| **维护** | 协议漂移仍归我们，但面极小（一个 proto + 帧），且由新增往返测试锁定 | 把我们的维护换成对单人/陈旧 SDK 的依赖；**仍要保留自己的重连循环**（`open()` 不自动重连） | 官方维护内部处理 token/握手/分片/ack/3s 窗口，但换来管一个长驻 Go 子进程 + 部署多 Node.js/Go |

---

## 关键澄清：飞书 CLI 能替代「长连接接收」吗？（你的核心误解点）

**能收，但不该用它做主路径。**

- 误解：以为飞书 CLI 只是个「发消息/调 API 的命令行工具」，不能做实时接收。**事实是它能**——`lark-cli event consume <EventKey>` 通过 WebSocket 长连接订阅事件，按 NDJSON 一行一个事件吐到 stdout，明确覆盖本项目要的 `im.message.receive_v1`。所以它在**能力**上没被排除。
- 但它在**契合度**上被排除：
  1. 它本质是 **CLI / AI-agent 工具**，不是服务端 SDK。集成 = 管一个长驻 Go 子进程 + 解析 stdout NDJSON / stderr 就绪标记，比进程内 Rust SDK 脆弱、重。
  2. 共享总线守护进程走 **POSIX Unix domain socket**，**Windows 支持未经验证**——而本项目在 Windows 上开发/运行。
  3. `kill -9` 会跳过 OAPI 退订、**泄漏服务端订阅**（重启报「订阅已存在」、重复投递），是运维地雷。
  4. 部署镜像要多塞 Node.js + 下载的 Go 二进制（本来是自包含 Rust crate）。

> 结论：飞书 CLI 适合做**辅助的出站 / 运维 sidecar**（2500+ API），或在部署目标改为 Linux 时再评估做接收；**不适合**做这个 Windows、进程内、always-on Rust 服务的主入站通道。

**另：没有官方 Rust SDK**（官方只有 Go/Python/Java/Node.js）。所以不存在「比 open-lark / CLI 更正确」的官方 Rust 选项——修旧不会让你「落后于某个官方基线」。

---

## 推荐路径与理由

**选 ①修旧。** 理由：

1. **根因是单点线上编码 bug，不是架构错误。** `/callback/ws/endpoint`（auth.rs:96）和 pbbp2 帧与官方 `oapi-sdk-go` v3 逐字段一致。
2. **没有官方 Rust SDK 可迁移**——修旧不损失任何官方支持；open-lark 重写了**同一套 pbbp2 栈**，并不降低协议漂移风险，只是把所有权挪给一个单人、陈旧的第三方。
3. **改动面最小、零接口冲击。** 保留 `ChatConnector` / `FeishuEvent` / `parse_message_event` / `SharedFeishuHandle` / `build_router` 签名 / 22 处测试调用点 / `FeishuSender` / DB schema / 所有 REST 端点——②③ 都会在这些缝上引入 churn。
4. **进程内 + Windows 原生**契合 always-on Rust 服务与本项目的 Windows 构建约束（RUST_MIN_STACK/-j1、protoc+LLVM）。

**修复清单（采纳红队修正后）：**
- **首个可逆步骤：先写红色测试** —— 在 `client.rs` 内加 `#[cfg(test)]` 往返测试，断言编码后的 ping 帧**同时**含 `SeqID(1)`/`LogID(2)`/`service(3)`/`method(4)` 四个必填字段（ping 也丢了 method=0）。先红再绿，无需新 crate / 编译目标（在 -j1 下更省）。
- **用低风险的 Rust 层修法（首选，而非 proto2 迁移）**：保持 proto3，给 `new_ping_frame` 配一个自增非零 `SeqID`/`LogID`（原子计数器），ack 回显入站帧的 ID 并带非零兜底 —— 避免 build.rs/proto 重新生成与 prost-0.13 对 proto2-required 支持不全的不确定性。proto2 转换仅作兜底方案。
- 把 ack payload 对齐 SDK 的 `{code, headers, data}`（`client.rs:330`，机械改动）。
- 把端点返回的 `ClientConfig` 喂进外层 `ReconnectPolicy`（`main.rs:475`）。
- **与上面同一个 PR**：补「连上了但没事件」诊断 + 后台前置条件引导 + 记录 `Handshake-*` 头 / `ExceedConnLimit(1000040350)` / `AuthFailed(514)`；ack 走「先快速 ACK 再异步处理」以守住 3 秒窗口（窄范围实现，**不要重写** `handle_data_frame`，别打乱 500 解析错误分支）。

---

## 红队反对意见与回应

| 反对意见 | 回应 |
|----------|------|
| **「proto2 迁移有风险，prost 0.13 对 proto2-required 支持不全」** | 采纳。**改为不迁 proto2**——保 proto3，在 Rust 层强制非零 SeqID/LogID，约 2 行改动、不触发 proto 重新生成。proto2 只作兜底。 |
| **「事件 ack 被夸大成共同 blocker」** | 部分采纳。ack 重用**已解码的入站帧**，若服务端入站 SeqID/LogID 非零则 prost 原样回写、ack 可能本就合法；只有零值情形才坏。**ping 才是无歧义的 blocker**。文案不再把 ack 与 ping 并列为同等破坏。 |
| **「往返测试只断言字段 1/2 不够」** | 采纳。官方标了**四个**必填（SeqID/LogID/service/method），测试须断言四个都序列化，否则同类 bug 会在 service/method 上复发。 |
| **「100% 修好协议 ≠ 用户症状一定消失」** | 采纳。HTTP bootstrap 可能 code=0、返回有效 WS URL，但应用未发布 / 长连接没开 / 事件没订阅仍会「连上但无事件」——这些代码改不了。任何「已修复」结论必须**门控在**连上无事件诊断 + 后台引导之后。 |
| **「换 open-lark 依赖重叠、迁移更干净」** | 反驳。依赖**并不**重叠：连接器用 `tokio-tungstenite 0.26 + native-tls`，open-lark 0.14 用 `tokio-tungstenite 0.23 + rustls`，而 workspace pin 的是 `rustls 0.23 'ring' + webpki-roots-no-provider`——迁移会引入**重复的 tungstenite 小版本 + rustls crypto-provider 冲突风险**，不是「最小重复依赖」。这反而**强化**修旧。 |
| **「连接器 TLS 已偏离 workspace 策略」** | 采纳为可选项：修复时顺手把 WS 的 `native-tls` 换成 `rustls` 以对齐 workspace（非阻塞）。 |
| **「飞书 CLI 是官方的，更省心」** | 它能收事件没错，但 POSIX-UDS-on-Windows 不匹配 + 子进程监管 + kill-9 泄漏订阅 + Go/Node 部署足迹，使它对本 Windows 项目是错的主路径。留作出站/运维 sidecar。 |

---

## 下一步：两份 PRD

修旧方案选定后，按以下两份 PRD 文件执行（同目录）：

- **修旧（推荐，先做）**：`docs/feishu/PRD-fix-existing-connector.md`
- **换 open-lark（备选/回退）**：`docs/feishu/PRD-migrate-connector.md`

> 飞书 CLI 不出 PRD（仅作可选 sidecar，待部署迁 Linux 再议）。

---

*诊断、对官方协议的核实、open-lark / 飞书 CLI 调研均已交叉验证，置信度 high。本备忘的根因已在仓库内逐处核实：`pbbp2.proto:1` proto3、`client.rs:33-43` ping 用 `..Default::default()`、`auth.rs:96` 端点路径。*
