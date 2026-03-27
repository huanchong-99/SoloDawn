# SoloDawn 代码审计报告

## 1. 审计概览
- 总分：45/100（D）
- 一句话结论：存在多处编译阻塞与严重安全暴露（无鉴权、明文 API Key、危险 CLI 启动参数），且终端运行链路尚未完整闭环。
- 审计方法：静态代码审查（rg/逐文件阅读）；受只读环境限制未运行 `scripts/audit-security.sh` 与 `cargo check/test`。

## 2. 详细评分表
| 维度 | 权重 | 得分 | 主要原因 |
| --- | --- | --- | --- |
| 架构与设计 | 20% | 55 | 路由层直连数据库/SQL、跨层依赖不一致（Deployment API 与调用不匹配） |
| 健壮性与逻辑 | 30% | 40 | 终端 WS/ProcessManager 未完成闭环、流程状态不一致、失败测试存在 |
| 风格与可维护性 | 25% | 60 | 代码结构整体可读，但 TODO/硬编码较多、部分错误处理用 `expect` |
| 性能与安全 | 15% | 35 | 无鉴权 + 明文密钥 + N+1 查询 + 阻塞 I/O |
| 文档 | 10% | 45 | README 结论与事实不符、API 文档缺失 |

## 3. 关键问题清单 (Critical Issues)

### P0 (阻塞)
- P0-1：终端 WS 调用不存在的 `process_manager()`，编译失败；影响：`/terminal` WebSocket 无法构建；位置：`crates/server/src/routes/terminal_ws.rs:196`；修复：在 `DeploymentImpl` 注入/暴露 `ProcessManager`，或将其作为路由状态传入。
- P0-2：Slash Commands 路由直接使用 `deployment.pool`，类型不存在导致编译失败；影响：`/workflows/presets/commands` 路由不可用；位置：`crates/server/src/routes/slash_commands.rs:51` `crates/server/src/routes/slash_commands.rs:106` `crates/server/src/routes/slash_commands.rs:133` `crates/server/src/routes/slash_commands.rs:162` `crates/server/src/routes/slash_commands.rs:171` `crates/server/src/routes/slash_commands.rs:190` `crates/server/src/routes/slash_commands.rs:205`；修复：改为 `deployment.db().pool` 或在 Deployment 中暴露 pool 访问器。
- P0-3：CLI 检测导入路径错误；影响：`cli_types` 模块编译失败；位置：`crates/server/src/routes/cli_types.rs:12`；修复：改为 `services::services::terminal::detector::CliDetector`。
- P0-4：CLI 检测构造 `Arc::new(deployment.db())` 得到 `Arc<&DBService>`，与 `CliDetector::new(Arc<DBService>)` 不匹配；影响：编译失败；位置：`crates/server/src/routes/cli_types.rs:37` `crates/server/src/routes/cli_types.rs:38`；修复：使用 `Arc::new(deployment.db().clone())` 或从 `Deployment` 提供 `Arc<DBService>`。

### P1 (严重)
- P1-1：无认证/授权中间件，且可通过 `HOST` 绑定到非回环地址；任意访问可触发文件系统扫描/终端执行；影响：远程读取本机目录结构、触发执行流程；位置：`crates/server/src/routes/mod.rs:21` `crates/server/src/main.rs:112`；修复：增加鉴权（session/token），并限制绑定为本地或增加允许列表。
- P1-2：终端自定义 API Key 明文存储且原样回传；影响：API key 泄露与横向滥用；位置：`crates/db/src/models/terminal.rs:58` `crates/server/src/routes/workflows.rs:447` `crates/server/src/routes/workflows_dto.rs:75` `crates/server/src/routes/workflows_dto.rs:251`；修复：使用与 workflow 相同的加密策略 + 响应中进行脱敏/不返回。
- P1-3：默认启动 Claude 终端时强制 `--dangerously-skip-permissions`；影响：绕过 CLI 权限保护；位置：`crates/services/src/services/terminal/launcher.rs:263`；修复：改为显式用户设置/审批开关，默认不启用。
- P1-4：文件系统 API 接受任意路径参数并直接列目录；影响：可遍历敏感目录（`.ssh`、系统目录）；位置：`crates/server/src/routes/filesystem.rs:19` `crates/server/src/routes/filesystem.rs:23` `crates/services/src/services/filesystem.rs:307`；修复：限制到受控根目录或启用路径白名单。
- P1-5：JWT 解析使用 `insecure_decode`（不校验签名）；影响：一旦被用于鉴权将可伪造 token；位置：`crates/utils/src/jwt.rs:2` `crates/utils/src/jwt.rs:32` `crates/utils/src/jwt.rs:39`；修复：仅用于非安全场景或改为签名验证。

### P2 (重要)
- P2-1：`ProcessManager::get_handle` 永远返回 `None`，WS 永远报 “Terminal process not running”；影响：终端实时输出链路不可用；位置：`crates/services/src/services/terminal/process.rs:392` `crates/server/src/routes/terminal_ws.rs:196`；修复：存储并返回 stdout/stderr/stdin handle。
- P2-2：终端 WS 仍是占位实现（PTY stdin/stdout/resize TODO）；影响：核心功能不可用；位置：`crates/server/src/routes/terminal_ws.rs:136` `crates/server/src/routes/terminal_ws.rs:226` `crates/server/src/routes/terminal_ws.rs:307`；修复：接入真实 PTY 并实现读写/resize。
- P2-3：存在明确失败测试（`assert!(false)`）；影响：CI 运行测试必失败；位置：`crates/services/tests/terminal_timeout_test.rs:8` `crates/services/tests/terminal_timeout_test.rs:16`；修复：补齐测试逻辑或移除占位断言。
- P2-4：`list_workflows` N+1 查询（workflow → tasks → terminals）；影响：数据量大时性能显著下降；位置：`crates/server/src/routes/workflows.rs:273` `crates/server/src/routes/workflows.rs:275` `crates/server/src/routes/workflows.rs:281`；修复：改为聚合 SQL 或一次性 join/批量计数。
- P2-5：文件系统扫描为阻塞 IO 却直接在异步任务中运行；影响：阻塞 tokio runtime；位置：`crates/services/src/services/filesystem.rs:131` `crates/services/src/services/filesystem.rs:136` `crates/services/src/services/filesystem.rs:307`；修复：改用 `tokio::task::spawn_blocking`。
- P2-6：Orchestrator 状态先置 `running` 再创建 agent，失败时状态无法回滚；影响：workflow 永久卡在 running；位置：`crates/services/src/services/orchestrator/runtime.rs:142` `crates/services/src/services/orchestrator/runtime.rs:147`；修复：先创建 agent，失败则不写状态或回滚。

### P3 (一般)
- P3-1：终端日志缓冲超过阈值直接丢弃，且未落盘；影响：日志丢失/调试困难；位置：`crates/services/src/services/terminal/process.rs:232` `crates/services/src/services/terminal/process.rs:276`；修复：实现持久化 flush 或分块写入。
- P3-2：会话创建硬编码 `claude-code`，与实际 CLI 类型不一致；影响：审计/指标误导；位置：`crates/services/src/services/terminal/launcher.rs:153`；修复：改为使用 `cli_type.name`。
- P3-3：`asset_dir()` 使用 `expect` 直接 panic；影响：异常场景崩溃；位置：`crates/utils/src/assets.rs:9` `crates/utils/src/assets.rs:18`；修复：返回 `Result` 并向上处理。
- P3-4：终端状态语义不一致（测试期望 running，代码设置 waiting）；影响：状态机与测试不一致；位置：`crates/db/src/models/terminal.rs:373` `crates/services/tests/terminal_lifecycle_test.rs:221`；修复：统一状态枚举语义或调整测试。
- P3-5：README 仍宣称 “代码审计 100/100”，与现状不符；影响：文档可信度下降；位置：`README.md:249` `README.md:477`；修复：更新为当前实际状态与问题清单。

## 4. 维度详细分析

### 架构与设计
- 领域逻辑与 API 路由耦合：多处路由直接写 SQL（`slash_commands`），违背服务层隔离。
- Deployment API 不一致：`terminal_ws` 和 `slash_commands` 对 Deployment 的假设不一致，导致编译失败。
- 模块边界不清：终端流程涉及 `services`、`server`、`db` 三层但缺少清晰职责划分与统一接口。

### 健壮性与逻辑
- 终端 WS 关键路径未完成（PTY/handle 未接通），导致运行时功能失效。
- Orchestrator 状态机存在失败路径未回滚问题，影响数据一致性。
- 明确失败测试存在，说明测试集当前不可通过。

### 风格与可维护性
- 多处 TODO 占位仍在生产路径（终端、模型获取、WS I/O）。
- 部分 `expect`/`unwrap` 未转为可恢复错误。
- 逻辑硬编码（如 `claude-code`）降低可扩展性。

### 性能与安全
- 无鉴权与文件系统/进程能力组合形成高风险攻击面。
- N+1 查询造成列表接口性能退化。
- 文件系统扫描/目录读取为阻塞 I/O，直接运行在 async executor 上。

### 文档
- README 状态与代码现状不符，可信度下降。
- API 文档缺少针对 Workflow/Terminal/Slash Commands 的当前接口说明。
- 多处中文乱码/编码异常影响可读性（README 输出已出现字符异常）。

### 测试覆盖
- `terminal_timeout_test` 明确失败；多个 TODO 测试未实现：`crates/services/tests/terminal_lifecycle_test.rs:257` `crates/server/tests/terminal_stop_test.rs:12` `crates/server/tests/cli_detection_test.rs:42`。
- 终端 WS/PTY 的端到端测试缺失，核心功能无验证闭环。

## 5. 重构建议（按优先级）

1. 修复编译阻塞并统一 Deployment API（优先级最高）
2. 引入鉴权层 + 密钥加密与脱敏
3. 打通终端运行链路（ProcessManager/PTY/WS）
4. 优化工作流列表性能与文件系统扫描并发模型
5. 补齐失败测试与端到端测试

**Bad Code**
```rust
// cli_types.rs
use services::terminal::detector::CliDetector;

let db = Arc::new(deployment.db());
let presets = SlashCommandPreset::find_all(&deployment.pool).await?;
```

**Good Code**
```rust
use services::services::terminal::detector::CliDetector;

let db = Arc::new(deployment.db().clone());
let presets = SlashCommandPreset::find_all(&deployment.db().pool).await?;
```

## 6. 总结（通过/不通过判定）
- 结论：Reject
- 理由：存在多处 P0 编译阻塞 + P1 安全暴露（无鉴权、明文 API key、危险 CLI 选项），且终端运行链路未闭环。

如果需要下一步建议：
1. 运行 `cargo check --workspace` 与 `cargo test --workspace` 以枚举全部编译/测试错误。  
2. 在可用的 Bash 环境中执行 `scripts/audit-security.sh` 验证密钥与日志安全检查。  
3. 对 `workflows`、`terminal_ws` 做端到端回归测试（含 WS/PTY 实际 I/O）。