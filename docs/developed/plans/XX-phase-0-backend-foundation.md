# Phase 0 — 后端基础层修复（Agent 1-8，完全并行）

> 预计产出：后端核心模块修复完成，为 Phase 1 前端修复提供稳定后端。
> Phase 内 8 个 Agent 完全并行，无文件交叉。

---

## Agent 1 — Orchestrator: Pause/Resume/Stop/Recovery

**负责文件（独占）：**
- `crates/services/src/services/orchestrator/agent.rs`（仅 pause/stop/recovery 相关函数）
- `crates/services/src/services/orchestrator/runtime.rs`
- `crates/services/src/services/orchestrator/persistence.rs`
- `crates/server/src/routes/workflows.rs`（仅 pause/stop/resume 端点）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G05-001 | P1 | pause_workflow 未执行 terminal/task 级联状态更新 | pause 时遍历 running tasks，将 terminal 设为 paused，task 设为 paused |
| G05-002 | P1 | pause 后无法 resume | 添加 resume_workflow 端点 + runtime.resume_workflow()，paused→running 状态转换 |
| G05-003 | P2 | stop_workflow runtime 中先 remove 再 publish Shutdown，5s 超时可能不够 | 改为先 publish Shutdown 等待 ack，再 remove；超时提升到 15s |
| G05-004 | P2 | recover_running_workflows 对有持久化状态的 workflow 仍标记 failed | 检查 task 进度，有已完成 task 的 workflow 尝试恢复而非直接 fail |
| G05-005 | P2 | recovery 后 GitWatcher 与 Agent 启动存在时序窗口 | Agent 订阅 bus 后再启动 GitWatcher，确保不丢事件 |
| G05-006 | P2 | stop 期间新事件到达的竞态 | Agent event loop 检测 shutdown flag，收到新事件时 drain 而非处理 |
| G05-007 | P3 | enforce_terminal_completion_shutdown 仅向 session topic 发 Shutdown | 同时发到 terminal.input topic |
| G05-010 | P3 | persistence.save_task_progress 是空实现 | 标记为 #[deprecated] 并添加 TODO 注释 |
| G03-005 | P3 | auto_dispatch_initial_tasks 串行执行 | 改为 join_all 并行 dispatch |
| G04-001 | P3 | no-metadata task_hint 多 candidate 返回 None | 多 candidate 时按 order_index 选最小的，添加 warn 日志 |
| G04-008 | P3 | quiet window 之后的 Checkpoint 检查是死代码 | 移除死代码分支 |

**注意事项：**
- agent.rs 是超大文件（4700+ 行），本 Agent 仅修改 pause/stop/recovery/dispatch 相关函数
- Agent 2 负责 agent.rs 的 merge/quality/provider 相关函数，两者不交叉
- workflows.rs 的 pause/stop/resume 端点（约 L1580-L1720）由本 Agent 独占

---

## Agent 2 — Orchestrator: Merge + Quality Gate + LLM Provider

**负责文件（独占）：**
- `crates/services/src/services/orchestrator/agent.rs`（仅 merge/quality/provider 相关函数）
- `crates/services/src/services/merge_coordinator.rs`
- `crates/services/src/services/orchestrator/state.rs`（仅 pending_quality_checks）
- `crates/server/src/routes/workflows.rs`（仅 merge 端点 L2660-L2750）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G06-001 | P1 | merge_workflow 端点缺乏并发保护 | 添加 CAS：WHERE status='running' 转 'merging' |
| G06-002 | P1 | auto_merge 与手动 merge 无互斥 | MergeCoordinator 添加 per-workflow Mutex，auto/manual 共用 |
| G06-003 | P2 | agent.trigger_merge worktree 路径硬编码 | 调用 WorktreeManager::get_worktree_path() 替代硬编码 |
| G06-004 | P2 | 多 task 顺序 merge 前面成功后面失败无法回滚 | 记录已成功 task 列表，失败时标记 workflow 为 merge_partial_failed |
| G06-005 | P2 | merge 操作不幂等 | 检查 task 是否已 merged，跳过已完成的 |
| G06-006 | P2 | merge 后不清理 worktree/branch | merge 成功后调用 WorktreeManager::cleanup_worktree() |
| G06-008 | P2 | agent.trigger_merge 绕过 MergeCoordinator 的 RwLock | 改为通过 MergeCoordinator 执行 merge |
| G06-009 | P3 | merge_squash_commit 冲突错误未分类 | 检测 stderr 中 CONFLICT 关键字，返回 MergeConflicts 错误类型 |
| G31-001 | P1 | enforce 模式修复指令发送到广播频道而非目标终端 | 改用 publish_terminal_input 定向发送到目标 terminal PTY |
| G31-003 | P2 | 质量门禁评估无超时保护 | tokio::time::timeout(Duration::from_secs(300)) 包裹 |
| G31-004 | P2 | replay/idempotent check 与 insert 之间 TOCTOU | 合并到单个 write lock 作用域内 |
| G31-005 | P2 | handle_quality_gate_result 重入触发二次 quiet window | 添加 skip_quiet_window 参数 |
| G31-006 | P3 | QualityEngine 失败时 fail-open 无告警 | gate_status 设为 "skipped"，发送 warn 事件 |
| G31-008 | P3 | tokio::spawn 内 panic 不清理 pending_quality_checks | 添加 scopeguard::defer 清理 |
| G31-009 | P3 | quality_run DB 插入失败不清理 pending | fallback 中 remove pending entry |
| G24-002 | P1 | slash command 路径绕过 publish_provider_events | 添加 publish_provider_events 调用 |
| G24-003 | P2 | Provider Switched 事件仅在 dead 时发出 | 每次实际切换都发事件 |
| G24-006 | P2 | 内部重试与 Resilient 重试叠加 | 内部重试降为 1 次，Resilient 层负责跨 provider 重试 |
| G24-009 | P3 | Provider 耗尽后 agent 不终止 workflow | 连续失败 3 次后标记 workflow failed |

**注意事项：**
- agent.rs 中本 Agent 负责的函数：trigger_merge (L4370+), handle_quality_gate_* (L1160-L1530), slash command (L4700+)
- 与 Agent 1 在 agent.rs 中的修改区域不重叠
- merge_coordinator.rs 由本 Agent 独占

---

## Agent 3 — Git Watcher + Git CLI

**负责文件（独占）：**
- `crates/services/src/services/git_watcher.rs`
- `crates/services/src/services/git/cli.rs`
- `crates/db/src/models/git_event.rs`

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G10-004 | P2 | git log 仅查询 HEAD 分支，不含 --all | 添加 --all 参数，配合 branch filter 过滤 |
| G10-005 | P2 | get_commit_by_hash branch 检测始终取 HEAD | 改用 git branch --contains <hash> 获取实际分支 |
| G10-010 | P3 | Checkpoint commit 触发 TerminalCompleted 事件 | 检测 checkpoint metadata，跳过 TerminalCompleted 发布 |
| G10-011 | P1 | 有 METADATA 的 commit 同时触发 TerminalCompleted 和 GitEvent 两条路径 | 有 metadata 时仅走 GitEvent 路径，跳过 TerminalCompleted |
| G10-012 | P3 | GitEvent commit_message 仅存 subject | 改为存储 full body（git show --format=%B） |
| G23-007 | P2 | worktree_add create_branch=true 时 branch 参数出现两次 | 修正为 -b <branch> <start-point> 格式 |
| G06-007 | P2 | broadcast_merge_success 每个 task 都设 workflow completed | 仅在最后一个 task merge 后设置 |

**注意事项：**
- git_watcher.rs 已在 Batch 5 中修复了分隔符(G10-001)、命令合并(G10-006)、重试计数器(G10-009)等
- 本 Agent 处理剩余的 git 链路问题
- cli.rs 的 merge_squash_commit 冲突分类(G06-009)由 Agent 2 在 merge 上下文中处理

---

## Agent 4 — Terminal / PTY 进程管理

**负责文件（独占）：**
- `crates/services/src/services/terminal/process.rs`
- `crates/services/src/services/terminal/bridge.rs`
- `crates/services/src/services/terminal/output_fanout.rs`
- `crates/services/src/services/terminal/launcher.rs`（仅 broadcast 相关）
- `crates/server/src/routes/terminal_ws.rs`
- `crates/services/src/services/orchestrator/runtime_actions.rs`（仅 close_terminal）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G11-005 | P1 | launcher broadcast_terminal_status workflow_id 为 None 时静默跳过 | 改为 warn 日志 + 尝试从 DB 查询 workflow_id |
| G09-003 | P2 | WS 断线重连后无法续传已丢失输出 | 添加 seq 序号机制，重连时从 last_seq 开始 replay |
| G09-005 | P1 | broadcast/replay capacity 不匹配 | replay capacity 设为 broadcast 的 2x（已部分修复，验证并完善） |
| G21-001 | P2 | CODEX_HOME 重复解析 | 提取为 lazy_static 缓存 |
| G21-002 | P2 | is_running() 无进程存活检查 | 添加 try_wait() 检测进程是否已退出 |
| G21-003 | P2 | close_terminal 无 Bridge 注销 | close 时调用 bridge.unregister(terminal_id) |
| G21-005 | P3 | Windows kill 无优雅关闭 | 先发 CTRL_C_EVENT，超时后再 TerminateProcess |
| G21-006 | P3 | ProcessManager 无 Drop | 实现 Drop trait，清理所有子进程 |
| G21-008 | P2 | legacy spawn_pty 与 spawn_pty_with_config 代码重复 | spawn_pty 委托给 spawn_pty_with_config(default_config) |
| G21-010 | P3 | 多 Bridge 各自触发 cleanup() 全局扫描 | ProcessManager 自身定时清理，Bridge 不触发 |
| G21-011 | P3 | close_terminal Shutdown 仅发到 session topic | 同时发到 terminal.input topic |
| G09-010 | P3 | terminal_ws last_activity 使用 RwLock 保护单个 Instant | 改用 AtomicU64 存储 timestamp |
| G15-007 | P2 | STARTABLE_TERMINAL_STATUSES 包含 waiting/working | 移除 working，仅保留 not_started/waiting |

**注意事项：**
- process.rs 已修复 flush_buffer TOCTOU(G21-009)、buffer 上限(G09-006)
- launcher.rs 的 workflow_id warn 日志(G02-002)已修复，本 Agent 仅处理 broadcast 相关
- runtime_actions.rs 仅修改 close_terminal 函数和 STARTABLE_TERMINAL_STATUSES 常量

---

## Agent 5 — Worktree 管理

**负责文件（独占）：**
- `crates/services/src/services/worktree_manager.rs`
- `crates/services/src/services/workspace_manager.rs`（仅 orphan cleanup）
- `crates/server/src/routes/workflows.rs`（仅 worktree 路径解析 L840-L880, L1350-L1390）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G23-001 | P1 | WORKTREE_CREATION_LOCKS HashMap 永不清理 | 改用 LRU cache（容量 1000）或添加 TTL 过期清理 |
| G23-002 | P1 | merge 路由中 worktree 路径硬编码与实际不一致 | 使用 WorktreeManager::get_worktree_path() API |
| G23-003 | P1 | branch 命名仅检查当前批次，不查 git 已有分支 | 调用 git branch --list 检查冲突 |
| G23-004 | P2 | stop/cancel/delete workflow 不清理 worktree 目录 | 在 stop/delete handler 中调用 batch_cleanup_worktrees |
| G23-005 | P2 | merge 完成后不清理 task worktree | merge 成功后调用 cleanup_worktree |
| G23-006 | P2 | resolve_workflow_working_dir 传递项目根目录而非 worktree | worktree 模式下覆盖 working_dir 为 worktree 路径 |
| G23-008 | P3 | comprehensive_worktree_cleanup remove_dir_all 失败中断 | 降级为 warn 并 continue |
| G23-009 | P3 | orphan workspace 清理使用同步 std::fs::read_dir | 改用 tokio::fs::read_dir |
| G23-010 | P3 | WORKTREE_CREATION_LOCKS 使用 std::sync::Mutex | 改用 parking_lot::Mutex（无 poison） |

**注意事项：**
- workflows.rs 中本 Agent 仅修改 branch 命名(L840-L880)和 working_dir 解析(L1350-L1390)
- Agent 1 负责 workflows.rs 的 pause/stop 端点，Agent 2 负责 merge 端点，三者区域不重叠
- worktree cleanup 调用点在 Agent 1(stop) 和 Agent 2(merge) 中，但实现在本 Agent 的 worktree_manager.rs 中
  - **协调方式**：本 Agent 先完成 worktree_manager.rs 的 API，Agent 1/2 在 Phase 0 结束后的集成阶段添加调用

---

## Agent 6 — Events / SSE / Subscription 后端

**负责文件（独占）：**
- `crates/services/src/services/streams.rs`
- `crates/services/src/services/events.rs`
- `crates/services/src/services/msg_store.rs`
- `crates/services/src/services/patches.rs`
- `crates/services/src/services/subscription_hub.rs`（仅 pending_events 清理，不含 Agent 5 已修复的 cleanup_if_idle）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G33-001 | P1 | 4 个 stream（stream_workflows_raw/stream_tasks_raw/stream_execution_processes/stream_terminals_raw）静默吞掉 BroadcastStream `Lagged` 错误，事件丢失无感知 | 参考 `stream_projects_raw` 已有的 Lagged 处理模式：检测到 Lagged 时发送 resync 事件，触发全量刷新 |
| G33-002 | P2 | `stream_tasks_raw` 无条件允许 `Remove` 操作，跨项目泄漏 | 从 patch path 提取 ID，验证 workspace 归属后再放行 |
| G33-003 | P2 | `stream_execution_processes` 同样无条件允许 `Remove` | 同 G33-002，添加 ownership 验证 |
| G33-004 | P2 | SSE 全局事件端点无连接数限制和空闲超时 | 添加 `tower::limit::ConcurrencyLimitLayer` + 60s idle timeout |
| G33-005 | P2 | MsgStore history 上限 100MB 过高，无内存预警 | 降低到 50MB，超过 80% 时 warn 日志 |
| G33-006 | P3 | `history_plus_stream` 在获取历史和订阅 live stream 之间存在竞态窗口 | 在同一 lock 作用域内完成 history 快照 + subscribe |
| G33-007 | P3 | EventService spawn 中多处 `unwrap()` 调用，panic 影响 tokio worker | 替换为 Result 处理 + error 日志 |
| G33-008 | P3 | `stream_tasks_raw` filter_map 对每个 workspace 事件触发 DB 查询 | 维护内存映射缓存（task_id → workspace_id），定时刷新 |
| G33-009 | P3 | SubscriptionHub `pending_events` 缓存无过期清理 | 在 `cleanup_if_idle` 中添加 pending_events 过期清理（TTL 5min） |
| G33-010 | P3 | patches.rs / streams.rs 中 `serde_json` 使用 unwrap 可能 panic | 替换为 `map_err` + `Result` 返回 |

**注意事项：**
- subscription_hub.rs 的 `cleanup_if_idle` 基础逻辑已由 Batch 5 修复（pending_events 清理），本 Agent 仅添加 TTL 过期机制
- streams.rs 是本 Agent 的核心修改文件，4 个 stream 函数都需要添加 Lagged 处理
- 所有修改需通过 `cargo test --package services` 验证
- **必须使用 augment-code MCP 进行文件索引**

---

## Agent 7 — Task Attempts / PR 后端

**负责文件（独占）：**
- `crates/services/src/services/task_attempts.rs`
- `crates/services/src/services/pr.rs`
- `crates/services/src/services/workspace_summary.rs`
- `crates/services/src/services/util.rs`（仅 `restore_worktrees_to_process` 函数）
- `crates/db/migrations/20250819000000_*.sql`（仅添加约束，需新建 migration）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G34-001 | P1 | `create_pr` DB 写入失败（PR 已在 GitHub 创建成功后）被静默吞掉 | 传播错误或返回 warning 响应，确保用户知道 PR 已创建但记录未保存 |
| G34-002 | P1 | `create_pr` 缺少 title 输入验证 | 添加 title 非空检查 + 长度限制（≤256 字符） |
| G34-003 | P2 | `merges` 表缺少唯一约束，PR 记录可重复 | 新建 migration 添加 `UNIQUE(task_attempt_id, pr_number)` |
| G34-004 | P2 | `create_task_attempt` 缺少并发保护 | 创建前检查是否已有 running attempt，有则返回 409 Conflict |
| G34-005 | P2 | `create_task_attempt` 中 `start_workspace` 失败不回滚 DB 记录 | 包裹在 transaction 中，或失败时 delete 已创建记录 |
| G34-006 | P2 | `merge_task_attempt` 中 git merge 成功后 DB 写入失败无法回滚 git | 记录到 dead-letter 表，后台定时重试 DB 写入 |
| G34-007 | P2 | `restore_worktrees_to_process` 忽略 reconcile 返回值 | 捕获返回值，non-OK 时 warn 日志记录 |
| G34-008 | P3 | `attach_existing_pr` 使用 `.next()` 取第一个 PR 无排序 | 按 Open > Merged > Closed 排序后取第一个 |
| G34-009 | P3 | `workspace_summary` 并行 diff 无并发限制 | 使用 `buffer_unordered(8)` 限制并发 |
| G34-012 | P3 | `finalize_workspace` 中 `set_archived` 失败不回滚 Task 状态 | 包裹在 transaction 中 |

**注意事项：**
- 新建 migration 文件命名：`YYYYMMDDHHMMSS_add_merges_unique_constraint.sql`
- pr.rs 的 `create_pr` 需要区分 GitHub API 成功 / DB 写入失败的场景
- task_attempts.rs 是最大修改量文件，涉及 create/merge/finalize 三个核心流程
- dead-letter 表如无现成，可先记录到日志 + 标记 status 为 `merge_db_failed`
- **必须使用 augment-code MCP 进行文件索引**

---

## Agent 8 — 类型生成 / 工具函数 / 杂项后端

**负责文件（独占）：**
- `crates/server/src/bin/generate_types.rs`
- `crates/utils/src/text.rs`
- `crates/utils/src/port_file.rs`
- `crates/utils/src/path.rs`
- `crates/services/src/services/workflow_events.rs`（仅 TS export 标注）

**修复清单：**

| ID | 严重度 | 问题 | 修复方案 |
|----|--------|------|----------|
| G17-001 | P1 | `WsEvent` / `WsEventType` 有 `#[derive(TS)]` 但未导出到 `shared/types.ts` | 在 generate_types.rs 中添加 `WsEvent::decl()` 和 `WsEventType::decl()` 调用 |
| G36-003 | P2 | 另有 6 个 Rust 类型有 `#[derive(TS)]` 但未导出 | 优先导出 `WsEventType` / `WsEvent`，审查其余 4 个类型是否需要前端使用，按需添加 |
| G36-007 | P3 | `git_branch_id` 每次调用编译正则 | 使用 `once_cell::sync::Lazy` 缓存编译后的 Regex |
| G36-008 | P3 | `port_file.rs` 硬编码 "solodawn" vs debug 模式 "solodawn-dev" 路径不一致 | 统一使用 `get_solodawn_temp_dir()` 函数 |
| G08-004 | P3 | CLAUDE.md 文档声称 broadcast channel 容量为 32，实际代码中为 1000 | 更新 CLAUDE.md 中的描述（注：CLAUDE.md 已 gitignore，仅本地修改） |

**注意事项：**
- generate_types.rs 修改后需运行 `pnpm run generate-types` 并验证 `shared/types.ts` 更新
- 运行 `pnpm run generate-types:check` 确保 CI 检查通过
- text.rs 的 Regex 缓存需添加 `once_cell` 依赖（已在 workspace Cargo.toml 中）
- workflow_events.rs 仅确认 TS derive 标注正确，不修改业务逻辑
- CLAUDE.md 是 gitignored 文件，修改仅影响本地
- 本 Agent 工作量较小，完成后可协助其他 Agent 进行代码 review
- **必须使用 augment-code MCP 进行文件索引**

---

## Phase 0 完成后的集成检查

Phase 0 所有 8 个 Agent 完成后，在进入 Phase 1 前需执行：

1. `cargo test --workspace` — 全量 Rust 测试
2. `cargo clippy --workspace` — 无新 warning
3. `pnpm run generate-types:check` — TS 类型同步
4. `pnpm run backend:check` — cargo check 通过
5. 验证 Agent 1/2/5 在 workflows.rs 中的修改区域无冲突
6. 验证 Agent 1/2 在 agent.rs 中的修改区域无冲突
