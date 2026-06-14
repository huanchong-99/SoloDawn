# 飞书连接器迁移 — 实现说明 / 验收手册（as-built）

> 分支：`feat/feishu-openlark-migration`　日期：2026-06-15
> 对应 PRD：`docs/feishu/PRD-feishu-sdk-migration.md`
> 状态：**代码完成、整工作区编译通过、连接器单测通过**；live 收发（M2）需 owner 用真实飞书应用验收（见 §4）。

---

## 1. 做了什么（一句话）

把自研逆向的 pbbp2 长连接换成维护中的 **openlark 0.17.0** Rust SDK：删掉手写的 `auth.rs/client.rs/proto.rs/build.rs/pbbp2.proto`，新增 `sdk.rs` 适配层（SDK 的 `LarkWsClient` + 事件 → 既有 `FeishuEvent`），`messages.rs` 改走 SDK 收发。从根上消除"proto3 丢必填字段导致 ping/ack 被拒"的旧 bug。**所有对外接缝（ChatConnector / FeishuService / Concierge 路由 / SharedFeishuHandle / DB / REST / 前端）零改动。**

## 2. 关键改动清单

| 文件 | 动作 |
|---|---|
| `crates/feishu-connector/src/sdk.rs` | **新增**：openlark 适配层（兼容旧 `FeishuClient` 接口：`new/connect/connected_flag/sdk_config`）+ raw→FeishuEvent pump + 120s "连上无事件" 看门狗 |
| `crates/feishu-connector/src/messages.rs` | 重写内部走 SDK（`CreateMessageRequest`/`ReplyMessageRequest`）；签名不变；`first_bot_chat_id` 固定 `Ok(None)` |
| `crates/feishu-connector/src/events.rs` | 新增 `feishu_event_from_sdk_payload`（防御式归一化）+ 单测 |
| `crates/feishu-connector/src/lib.rs` | 去 `auth/client/proto` mod；加 `sdk`；`pub mod client { pub use crate::sdk::FeishuClient; }` 兼容 shim |
| `crates/feishu-connector/src/types.rs` | 删 `WsEndpointResponse/WsEndpointData/CachedToken`；留 `FeishuConfig/ClientConfig` |
| `crates/feishu-connector/Cargo.toml` | 删 prost/prost-build/tokio-tungstenite(native-tls)/bytes/flate2/futures-util/url + unused reqwest/uuid/tokio-util；加 `openlark` |
| 删除 | `auth.rs` `client.rs` `proto.rs` `build.rs` `proto/pbbp2.proto` |
| `crates/services/src/services/feishu.rs` | 仅 1 处：`FeishuMessenger::new(client.sdk_config())` |
| 根 `Cargo.toml` | `rustls` features `ring`→`aws_lc_rs`；新增 `openlark` workspace dep（reqwest **保持 0.12**，见 §3） |
| sqlx ×4（db/services/server/local-deployment） | `tls-rustls-ring`→`tls-rustls-aws-lc-rs` |
| `install_default()` ×14 站点 | `ring::default_provider()`→`aws_lc_rs::default_provider()` |
| `crates/server/Cargo.toml` | `tokio-tungstenite "0.26"`→`{ "0.29", default-features=false, features=["connect"] }` |

## 3. 对 PRD 的两处实测偏离（编译驱动，更优）

1. **reqwest 保持 0.12，未升 0.13。** PRD §6.3(b) 要求全工作区升 reqwest 0.13，但实测发现：(a) 我们的代码不需要 0.13；(b) 升 0.13 会让第三方 `sentry 0.41` 的 reqwest-0.12 transport 丢失共享 TLS feature 而编译失败（`danger_accept_invalid_certs` not found）。openlark 自带 reqwest 0.13（传递依赖），与我们的 0.12 共存即可。**单一 crypto provider 的唯一必要改动是 rustls pin → aws_lc_rs**（外加 sqlx/install_default）。因此：无需 fork、无 reqwest 迁移风险、PRD 标注的"最大风险"被消除。
2. **"reqwest / tokio-tungstenite 单一大版本" 验收项不可达，已放弃。** openlark 强制 reqwest 0.13、sentry 强制 reqwest 0.12；axum 强制 tokio-tungstenite 0.28、openlark/server 用 0.29。均为第三方约束的**良性重复**（各自独立编译，不影响功能/不涉 provider）。

## 4. 验收状态（对照 PRD §9）

### 已自动验证 ✓
- [x] `cargo check --workspace --all-targets`（Windows, `-j1`, RUST_MIN_STACK=256MB）**通过**——含 server/services、install_default 全站点、4 个验收测试文件、self_test 全部编译。
- [x] **单一 crypto provider**：`cargo tree -i ring` 为空、`cargo tree -i native-tls` 为空、`aws-lc-rs 1.17` 在场、rustls 0.23 单一。
- [x] **连接器单测**：`feishu_event_from_sdk_payload` 两分支 + 错误用例 3/3 通过。
- [x] **旧代码已删**：proto/pbbp2/build.rs/auth.rs/client.rs 不存在；Cargo.toml 无 prost/prost-build/native-tls/flate2。
- [x] **接缝零回归**：`new_shared_handle()` 零参签名不变；`build_router` 编译通过。
- [x] **4 个验收测试文件全绿（已实跑通过，非仅编译）**：`auth_test` 7/7、`quality_gates_test` 3/3、`slash_commands_test` 7/7、`workflow_api_test` 4/4。

### M2 live 收发：已降为"一条命令"（自动化最大化）

真实飞书云端往返需要一个**已注册/发布的企业自建应用凭据**（`cli_xxx` + App Secret）——这是本环境外的硬依赖，agent 无法凭空创建飞书账号/应用。除此之外，**M2 已被自动化成一条命令**（不再需要手动点界面、肉眼比对）：

**A. 一次性后台配置**（PRD §8.1，owner 在飞书后台做）：企业自建应用已发布 → 事件订阅选**长连接** → 订阅 `im.message.receive_v1` → 开通机器人 + `im:message`/`im:message:send_as_bot` scope → 把机器人拉进目标群/单聊，记下其 `chat_id`（`oc_xxx`）。

**B. 一条命令跑真连 E2E**（`crates/feishu-connector/tests/live_e2e.rs`，3 个 `#[ignore]` 测试）：

```bash
FEISHU_TEST_APP_ID=cli_xxx \
FEISHU_TEST_APP_SECRET=xxx \
FEISHU_TEST_CHAT_ID=oc_xxx \
RUST_MIN_STACK=268435456 \
cargo test -p feishu-connector --test live_e2e -- --ignored --nocapture
```

- `live_send_text` / `live_send_card`：**全自动**——发真实消息到 `chat_id`，断言返回非空 `message_id`（验证出站 + token 获取 + TLS）。
- `live_receive_one_message`：连长连接、90s 内等你给机器人发一条消息，断言整条入站管线（WS → 归一化 → `parse_message_event` → 非空 chat_id/message_id）。三条全绿 = M2 达成。

**C. 看门狗**：若连上但 120s 无事件，日志打印 §8.1 排查清单（多半后台没订阅/没发布）。

> 出站报文格式已有**不触网单测**（`messages.rs`：text/card/reply 的 msg_type+content + message_id 提取）+ 入站归一化单测（`events.rs` 两分支），均随 `cargo test -p feishu-connector` 自动跑。live_e2e 仅覆盖真实云端往返这一不可本地模拟的部分。

## 5. 构建环境（一次性，已在本机装好）

- MSVC BuildTools 14.44（已在）、LLVM/libclang（已在，`LIBCLANG_PATH=C:\Program Files\LLVM\bin`）、protoc（已在）。
- **本次新装**：NASM、CMake（winget）。位置 `C:\Program Files\NASM`、`C:\Program Files\CMake\bin`——若新开终端找不到，构建前把这两目录加入 PATH。
- 所有 cargo 命令：`RUST_MIN_STACK=268435456` + `-j1`（aws-lc-rs/openlark + 重 codegen 需要）。

## 6. 提交状态

改动均在分支 `feat/feishu-openlark-migration` 的工作区，**尚未 commit**（等 owner 验收）。删除经 `git rm` 已暂存，其余为未暂存修改。验收通过后可 `git add -A && git commit`。回滚：丢弃分支即恢复旧连接器（DB/REST/前端/加密未变，零数据回滚）。

## 7. 顺带修复的既有测试缺陷（验证时暴露，非飞书相关，已一并修好）

验证迁移时跑 server 集成测试，暴露出 5 个**与飞书无关的既有缺陷**。按"负责解决项目一切问题"原则全部修好（已实跑全绿）：

| 缺陷 | 根因 | 修法（生产 / 测试） |
|---|---|---|
| `slash_commands::missing_description` 返 422 而非 400 | axum 0.8 `Json<T>` 对缺必填字段在 handler 前以 422 拒绝；`description: String` 是必填 | **生产** `routes/slash_commands.rs`：`description`→`Option<String>` + `#[serde(default)]`，handler 内 `unwrap_or_default()` + 空值校验 → 正确 400 |
| `slash_commands::missing_leading_slash` 断言 `error` 非 string | `ApiResponse` 线上错误字段是 `message`（前端也读 `message`），从无 `error` | **测试**：断言 `error`→`message` |
| `slash_commands` success/duplicate/update/delete 重跑撞 409 | 全套集成测试共享 `dev_assets/db.sqlite`，这些测试用**固定命令名**，重跑/并行撞 PK | **测试**：改用 `/<kind>-{Uuid}` 唯一命令（对齐 quality_gates 约定），可重复运行 |
| `workflow_api` 3 个 panic@setup | setup 插入固定 id `test-cli`/`test-model`，共享 DB 撞 PK/UNIQUE | **测试**：cli_type/model_config 改 Uuid 并串入 FK |
| `workflow_api::without_orchestrator` 期望 400 实得 200 | 错误假设——DIY 模式（`orchestrator_enabled=false`）本就支持无 orchestrator 启动（`start_workflow` workflows.rs:1981-1999：`set_started`→`running`、返 200） | **测试**：纠正为期望 200 + 状态 `running`（已对照 handler 证实，非臆改） |

> 根因共性：server 集成测试套件**共享一个磁盘 dev DB、无 per-test 隔离**，依赖唯一标识符避免冲突。本次让 slash_commands/workflow_api 遵循该约定 → **全套现在可重复运行**。更彻底的隔离（每测试独立临时 DB）属更大重构，超出本次范围，未触碰其它通过的测试。
