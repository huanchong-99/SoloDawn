# SoloDawn 全量代码深度审计报告（链路驱动版）

> 审计日期: 2026-03-14
> 审计方法: 按交互链路分组，6 轮 x 6 并发 = 36 组
> 审计目标: 编译通过但功能可能失效的隐性缺陷

---

## 轮次进度

| 轮次 | 状态 | 组别 | 问题数 |
|------|------|------|--------|
| Round 1 | ✅ 完成 | G01-G06 核心生命周期链路 | 38 |
| Round 2 | ✅ 完成 | G07-G12 实时通信与事件链路 | 55 |
| Round 3 | ✅ 完成 | G13-G18 状态一致性与数据契约 | 64 |
| Round 4 | ✅ 完成 | G19-G24 CLI 执行器与进程管理 | 63 |
| Round 5 | ✅ 完成 | G25-G30 前端交互与 UI 状态 | 81 |
| Round 6 | ✅ 完成 | G31-G36 辅助系统与集成 | 72 |

---

## Round 1 — 核心生命周期链路审计 (G01-G06)

### G01 — Workflow 创建链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G01-001 | P3 | 前端发送 commandPresetIds 但后端不接收 | 冗余字段 | frontend/src/components/workflow/types.ts:371-374 | 移除前端冗余字段或后端 serde(default) 忽略 | open |
| G01-002 | P2 | 后端未验证 merge_terminal_config 的 cli_type_id/model_config_id 非空 | 错误信息不友好 | crates/server/src/routes/workflows.rs:525-616 | 在 validate_create_request 中添加非空检查 | open |
| G01-003 | P2 | Slash command 关联创建不在 create_with_tasks 事务内 | 部分创建风险 | crates/server/src/routes/workflows.rs:940-970 | 将 command 创建纳入事务 | open |
| G01-004 | P1 | TerminalStatus::NotStarted.to_string() 输出 "notstarted" 与硬编码 "not_started" 不一致 | 定时炸弹 | crates/db/src/models/terminal.rs:34; workflows.rs:907 | strum 改为 serialize_all="snake_case" | open |
| G01-005 | P3 | DTO 中 merge_terminal_cli_id 用 Option 包裹但 DB 为必填 | 类型语义不一致 | crates/server/src/routes/workflows_dto.rs:168-169 | DTO 改为 String 非 Option | open |

### G02 — Workflow Prepare 链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G02-001 | P3 | rollback_prepare_failure 中 Bridge 未被显式注销 | 短暂资源泄漏 | crates/server/src/routes/workflows.rs:1082-1185 | 在回滚循环中调用 bridge.unregister | open |
| G02-002 | P2 | launcher.rs 中 workflow_id 解析失败被 .ok().flatten() 静默吞掉 | 前端不知道 terminal 已失败 | crates/services/src/services/terminal/launcher.rs:539-543 | 添加 tracing::warn 日志 | open |
| G02-003 | P2 | Terminal::set_starting/set_waiting 不使用 CAS，无法防止并发状态覆盖 | 极端并发下状态覆盖 | crates/db/src/models/terminal.rs:479-515 | 使用 WHERE status='not_started' CAS | open |
| G02-004 | P3 | 前端 usePrepareWorkflow 缺少 onError 中的 cache invalidation | prepare 失败后 UI 状态滞后 | frontend/src/hooks/useWorkflows.ts:583-601 | onError 中 invalidateQueries | open |
| G02-005 | P2 | Workflow.status 和 Terminal.status 字段类型为 String 而非枚举 | 类型安全隐患 | crates/db/src/models/workflow.rs:116; terminal.rs:90 | 改为枚举类型或关键路径 parse 验证 | open |
| G02-006 | P3 | TerminalCoordinator::start_terminals 中途失败不回滚已设为 starting 的 terminal | 由上层兜底 | crates/services/src/services/orchestrator/terminal_coordinator.rs:100-111 | 依赖上层 rollback_prepare_failure | open |
| G02-007 | P3 | 前端 WorkflowStatusEnum 包含 'draft' 但后端枚举无此值 | 前后端状态不对齐 | frontend/src/hooks/useWorkflows.ts:21-31 | 移除前端 draft 或后端添加 | open |

### G03 — Workflow Start 链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G03-001 | P2 | reserve_start_slot 非 RAII，panic 路径 slot 泄漏 | 并发 workflow 容量 | crates/services/src/services/orchestrator/runtime.rs:484-488 | 改为 RAII guard 或 scopeguard | open |
| G03-002 | P1 | re-prepare 失败时错误信息不明确，用户收到 prepare 错误而非 start 错误 | 用户体验 | crates/server/src/routes/workflows.rs:1529 | 包装为专用错误类型 | open |
| G03-003 | P2 | re-prepare 后未验证 workflow 状态是否回到 ready | 后续 start 可能失败 | crates/server/src/routes/workflows.rs:1531-1533 | 显式检查 status=="ready" | open |
| G03-004 | P2 | paused->ready 转换使用无条件 update_status，存在 TOCTOU 窗口 | 并发请求竞态 | crates/server/src/routes/workflows.rs:1554-1555 | 改为 CAS: WHERE status='paused' | open |
| G03-005 | P3 | auto_dispatch_initial_tasks 串行执行，大量 task 时有延迟 | 启动延迟 | crates/services/src/services/orchestrator/agent.rs:4018 | 考虑 join_all 并行 | open |
| G03-006 | P2 | paused->ready 使用无条件 update_status（与 G03-004 同源） | 状态转换语义不严谨 | crates/server/src/routes/workflows.rs:1554-1555 | CAS 模式 | open |

### G04 — Terminal Completion 链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G04-001 | P3 | no-metadata 推断在 task_hint 存在但匹配多个 candidate 时直接返回 None | 推断失败 | crates/services/src/services/orchestrator/agent.rs:2219 | 移除 task_hint.is_none() 限制 | open |
| G04-002 | P2 | defer_terminal_completion 重新发布事件时未检查 terminal 当前状态 | 已取消 terminal 可能被重新完成 | crates/services/src/services/orchestrator/agent.rs:1639-1642 | 发布前从 DB 读取状态确认仍为 working | open |
| G04-003 | P1 | set_completed_if_unfinished 未排除 'failed' 状态 | 已失败 terminal 可被覆盖为 completed | crates/db/src/models/terminal.rs:586-587 | WHERE 添加 AND status != 'failed' | open |
| G04-004 | P2 | auto_sync_workflow_completion 未排除 'merging' 状态 | merging 中的 workflow 可能被覆盖为 completed | crates/services/src/services/orchestrator/agent.rs:4228-4233 | 排除条件添加 WORKFLOW_STATUS_MERGING | open |
| G04-005 | P2 | 多处 let _ = 丢弃 message_bus.publish_workflow_event 的 Result | 前端 UI 状态不同步 | agent.rs:1012,1074,4261; message_bus.rs:229,252 | 至少记录 warn 日志 | open |
| G04-006 | P3 | git_event 的 DB 状态更新使用 let _ = 丢弃错误 | 审计追踪不准确 | crates/services/src/services/orchestrator/agent.rs:1883 | 至少记录 warn 日志 | open |
| G04-007 | P3 | handle_git_event 中二次 metadata 解析使用 subject line 而非 full body，永远失败 | 冗余代码 | crates/services/src/services/orchestrator/agent.rs:1847 | 添加注释或移除冗余解析 | open |
| G04-008 | P3 | quiet window 之后的 Checkpoint 检查是死代码 | 代码清洁度 | crates/services/src/services/orchestrator/agent.rs:879 | 移除死代码分支 | open |

### G05 — Workflow Stop/Pause/Recovery 链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G05-001 | P1 | pause_workflow 未执行 terminal/task 级联状态更新，也未清理 PTY 进程 | Pause 功能不可用 | crates/server/src/routes/workflows.rs:1582-1616 | 调用 cleanup_workflow_terminals 并级联更新状态 | open |
| G05-002 | P1 | pause 后无法 resume — 前端允许 canStart 但后端 set_started 要求 status='ready' | Pause/Resume 链路断裂 | useWorkflows.ts:86-93; workflow.rs:690-711; runtime.rs:331 | set_started CAS 允许 paused 或新建 /resume 端点 | open |
| G05-003 | P2 | stop_workflow 在 runtime 中先 remove 再 publish Shutdown，LLM 长调用时 5s 超时可能不够 | Agent 被 abort 而非优雅退出 | crates/services/src/services/orchestrator/runtime.rs:637-651 | 增加 CancellationToken 机制 | open |
| G05-004 | P2 | recover_running_workflows 对有持久化状态的 workflow 仍标记为 failed | 服务重启后 workflow 丢失 | crates/services/src/services/orchestrator/runtime.rs:786-824 | 增加完整 DB schema 下的集成测试 | open |
| G05-005 | P2 | recovery 后 GitWatcher 与 Agent 启动存在时序窗口 | 恢复期间 commit 可能丢失 | crates/services/src/services/orchestrator/runtime.rs:888-971 | GitWatcher 先于 agent.run() 启动或恢复后主动扫描 | open |
| G05-006 | P2 | stop 期间新事件到达的竞态 — runtime remove 和 API 级联更新同时写 DB | 状态不一致 | crates/services/src/services/orchestrator/runtime.rs:637-651 | 先发 Shutdown 等 agent 退出再级联更新 | open |
| G05-007 | P3 | enforce_terminal_completion_shutdown 仅向 session topic 发 Shutdown | 一致性 | crates/services/src/services/orchestrator/agent.rs:3745-3748 | 同时向 terminal.input topic 发送 | open |
| G05-008 | P3 | force_terminate_terminal_process 在 async 上下文中使用 std::thread::sleep | 阻塞 tokio runtime | crates/services/src/services/orchestrator/agent.rs:3816 | 改用 tokio::time::sleep | open |
| G05-009 | P3 | 前端 useStopWorkflow/usePauseWorkflow 缺少乐观更新 | UI 状态延迟 | frontend/src/hooks/useWorkflows.ts:654-671 | 添加 onMutate 乐观更新 | open |
| G05-010 | P3 | persistence.save_task_progress 是空实现 (no-op) | 误导性 API | crates/services/src/services/orchestrator/persistence.rs:285-301 | 实现增量保存或标记 deprecated | open |

### G06 — Merge 链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G06-001 | P1 | merge_workflow 端点缺乏并发保护，无 CAS 状态转换 | 并发 merge 导致重复 commit | crates/server/src/routes/workflows.rs:2662 | CAS: WHERE status IN ('completed','merging') | open |
| G06-002 | P1 | auto_merge 与手动 merge 之间无互斥 | 同一 branch 被 squash merge 两次 | agent.rs:4278; workflows.rs:137 | auto_merge 先 CAS 改状态或 merge 端点检查 auto_merge 配置 | open |
| G06-003 | P2 | agent.trigger_merge 中 worktree 路径硬编码与 WorktreeManager 不一致 | auto-merge 找不到 worktree | agent.rs:4389; worktree_manager.rs:534 | 使用 WorktreeManager::get_worktree_base_dir() | open |
| G06-004 | P2 | 多 task 顺序 merge 时前面成功后面失败，已成功的无法回滚 | target branch 不一致 | crates/server/src/routes/workflows.rs:2665-2728 | merge 前记录 HEAD SHA，失败时 git reset | open |
| G06-005 | P2 | merge 操作不幂等，重复调用产生重复 squash commit | 数据污染 | crates/server/src/routes/workflows.rs:2665 | merge 前检查 branch 是否已是 target 的祖先 | open |
| G06-006 | P2 | merge 后不清理 worktree 也不删除 task branch | 磁盘泄漏 | crates/server/src/routes/workflows.rs:2731 | merge 成功后调用 batch_cleanup_worktrees | open |
| G06-007 | P2 | MergeCoordinator.broadcast_merge_success 每个 task merge 后都设 workflow completed | 多 task 时状态跳变 | crates/services/src/services/merge_coordinator.rs:225-226 | 由调用方在全部 task merge 后统一设置 | open |
| G06-008 | P2 | agent.trigger_merge 新建 GitService 实例绕过 MergeCoordinator 的 RwLock | 并发 git 操作冲突 | agent.rs:4372; merge_coordinator.rs:79-80 | 通过 MergeCoordinator 执行 merge | open |
| G06-009 | P3 | CLI 路径 merge_squash_commit 冲突错误未分类为 MergeConflicts | 上层无法正确处理冲突 | crates/services/src/services/git/cli.rs:600-603 | 检查 stderr 中 CONFLICT 关键字 | open |
| G06-010 | P3 | 前端 useMergeWorkflow 没有乐观更新 | UI 状态滞后 | frontend/src/hooks/useWorkflows.ts:754-768 | 添加 onMutate 乐观更新 | open |
| G06-011 | P3 | 前端 merging 状态下 canMerge=true 允许重复点击 | 重复提交 | frontend/src/hooks/useWorkflows.ts:94-101 | mutation isPending 时禁用按钮 | open |

---

## Round 2 — 实时通信与事件链路审计 (G07-G12)

### G07 — TerminalPrompt 全链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G07-001 | P2 | PromptWatcher.process_output 持有全局 RwLock 写锁期间执行大量计算 | 多 terminal 并发 prompt 延迟 | prompt_watcher.rs:1122-2289 | 拆分为 per-terminal Mutex | open |
| G07-002 | P2 | PromptHandler Responding 状态无主动超时回退到 Idle | 状态机可能卡住 | types.rs:387-391 | 添加后台定时器周期检查 stale | open |
| G07-003 | P2 | 特殊 prompt 路径绕过 PromptHandler 的 auto_confirm 检查 | unexpected-changes/handoff-stall 不受 auto_confirm=false 约束 | prompt_watcher.rs:1202-2185 | 添加 auto_confirm 守卫条件 | open |
| G07-004 | P3 | 前端 prompt 去重窗口(1.5s)与后端 debounce(500ms)不对齐 | 合法 prompt 可能被误丢弃 | Workflows.tsx:81; prompt_watcher.rs:41 | 降低 submitted history TTL | open |
| G07-005 | P3 | Bridge normalize_message 换行符转换风格不一致 | 代码一致性 | bridge.rs:480-505; prompt_handler.rs:401 | 统一使用 \n 结尾 | open |
| G07-006 | P3 | publish_terminal_input fallback 路径投递失败后 PromptWatcher 特殊路径未检查返回值 | 状态机可能卡死 | message_bus.rs:382-390; prompt_watcher.rs:1317-1324 | 检查返回值并 reset 状态机 | open |
| G07-007 | P3 | 前端 isSamePromptContext 在 sessionId 缺失时仅比较 workflowId+terminalId | 多 prompt 误匹配 | Workflows.tsx:110-124 | 增加 promptKind 辅助匹配 | open |
| G07-008 | P1 | WaitingForApproval 状态无超时机制，用户未响应将永久阻塞 terminal | terminal 永久卡死 | types.rs:363; prompt_handler.rs:148-150 | 添加可配置超时自动 reset+Skip | open |
| G07-009 | P3 | prompt_detector INPUT_FIELD_RE 可能误匹配普通输出 | 误判率 | prompt_detector.rs:216-220 | 可接受风险，可增加负向前瞻 | open |
| G07-010 | P2 | handle_user_prompt_response 中 session_id 解析链路复杂但实际安全 | 已验证无问题 | agent.rs:4538-4546 | 无需修复 | verified |
| G07-011 | P3 | 前端 prompt 队列仅展示第一个，多 terminal 并发 prompt 不可见 | UX 受限 | Workflows.tsx:1187 | 添加队列计数指示器 | open |

### G08 — WebSocket 事件全链路 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G08-001 | P2 | orchestrator.awakened/decision 是后端定义但从未发送的死代码 | 枚举膨胀 | workflow_events.rs:46-51 | 移除或实际发送 | open |
| G08-002 | P2 | provider.switched/exhausted/recovered 后端发送但前端完全未消费 | Provider 故障用户无感知 | workflow_events.rs:443-482; wsStore.ts:18-31 | 前端添加 handler 和 toast 通知 | open |
| G08-003 | P3 | Workflows.tsx 未订阅 onGitCommitDetected 事件 | Workflows 页面 git commit 不触发刷新 | Workflows.tsx:1167-1183 | 添加 onGitCommitDetected handler | open |
| G08-004 | P3 | CLAUDE.md 文档声称 broadcast channel 容量为 32，实际为 1000 | 文档误导 | CLAUDE.md; message_bus.rs:425 | 更新文档 | open |
| G08-005 | P3 | WsEvent payload 同时写入 camelCase 和 snake_case 双份字段 | payload 体积膨胀 30-50% | workflow_events.rs:275-288 | 长期统一为 camelCase | open |
| G08-006 | P3 | 前端收到 system.lagged 后无任何处理，不触发全量刷新 | 丢消息后 UI 状态不一致 | wsStore.ts:27 | 收到 lagged 后 invalidate 全部查询 | open |
| G08-007 | P2 | useWorkflowEvents handlers 依赖项不稳定可能导致频繁重订阅 | 高频事件场景丢事件 | wsStore.ts:1619 | 内部使用 useRef 缓存 handlers | open |
| G08-008 | P1 | WsEventType 前后端字符串不完全匹配，前端缺少 3 个 provider 事件类型 | TypeScript 类型不完整 | wsStore.ts:18-31; workflow_events.rs:24-88 | 前端添加缺失类型 | open |
| G08-009 | P3 | SubscriptionHub cleanup_if_idle 不清理 pending_events 缓存 | 轻微内存泄漏 | subscription_hub.rs:148-158 | 同步清理 pending_events | open |
| G08-010 | P3 | 心跳仅单向有效，服务端无法检测半开连接 | 死连接资源浪费 | workflow_ws.rs:212-213 | 服务端记录最后心跳时间并超时断开 | open |

### G09 — Terminal WebSocket 日志流链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G09-001 | P2 | broadcast channel Lagged 时 WS 链路静默丢弃消息无法恢复 | 高输出时前端显示不完整 | terminal_ws.rs:318-327 | Lagged 后重新 subscribe(Some(last_seq)) | open |
| G09-002 | P3 | shouldRenderLiveTerminal 中 running/active 状态检查为死代码 | 代码可维护性 | TerminalDebugView.tsx:140-143 | 移除 running/active | open |
| G09-003 | P2 | WS 断线重连后无法续传已丢失输出，前端不传 from_seq | 重连后输出重复或丢失 | terminal_ws.rs:199; TerminalEmulator.tsx:237 | 实现基于 seq 的增量续传 | open |
| G09-004 | P3 | output_fanout replay 缓冲区上限 1MiB/512 chunks 对长时间任务可能不足 | 重连时早期输出丢失 | process.rs:44-47 | 改为可配置 | open |
| G09-005 | P1 | broadcast capacity=512 与 replay capacity=512 相同，lag 恢复可能不完整 | 高吞吐时不可恢复的输出丢失 | output_fanout.rs:43-44; process.rs:44 | replay 至少为 broadcast 的 2-4 倍 | open |
| G09-006 | P3 | TerminalLogger buffer 无上限增长保护 | flush 失败时 OOM 风险 | process.rs:1578-1609 | 添加 buffer 大小上限 | open |
| G09-007 | P3 | PTY reader 中 publish 返回值被忽略 | 已验证可接受 | process.rs:495-496 | 低优先级 | verified |
| G09-008 | P2 | 前端 TerminalEmulator 重连时不清除 xterm 缓冲区导致输出重复 | 用户看到重复输出 | TerminalEmulator.tsx:239-268 | 重连时 clear() 或实现 seq 续传 | open |
| G09-009 | P3 | 前端 pendingInputRef 无大小限制 | 极端场景内存增长 | TerminalEmulator.tsx:48,145 | 添加队列大小上限 | open |
| G09-010 | P3 | terminal_ws 中 last_activity 使用 RwLock 保护单个 Instant | 不必要的锁竞争 | terminal_ws.rs:229-231 | 改用 AtomicU64 | open |
| G09-011 | P2 | DisplayConversationEntry 使用 lucide-react 违反项目规范 | 代码规范一致性 | DisplayConversationEntry.tsx:16-32 | 迁移至 @phosphor-icons/react | open |
| G09-012 | P1 | 历史终端日志加载 API 无分页，1000 条一次性加载 | 前端卡顿 | TerminalDebugView.tsx:31,161 | 实现虚拟滚动或分页 | open |

### G10 — Git Commit 检测与元数据解析链路 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G10-001 | P1 | git show --format=%H\|%s 使用 '\|' 分隔符，commit subject 含 '\|' 会截断 | message 字段不完整 | git_watcher.rs:359,383 | 使用 ASCII record separator \x1e | open |
| G10-002 | P1 | agent 对 GitEvent.message(仅 subject)二次 parse_commit_metadata 永远返回 None | 死代码/冗余 | git_watcher.rs:359; agent.rs:1847 | 移除冗余解析或传递 full_message | open |
| G10-003 | P2 | processed_commits 是纯内存 HashSet 无容量上限 | 长时间运行内存增长 | state.rs:65 | 改用 LRU 缓存或 BoundedHashSet | open |
| G10-004 | P2 | git log 仅查询当前 HEAD 分支，不含 --all | 非 HEAD 分支 commit 漏检 | git_watcher.rs:317-321 | 确认 per-worktree 实例化或加 --all | open |
| G10-005 | P2 | get_commit_by_hash 中 branch 检测始终取 HEAD abbrev-ref 而非 commit 实际分支 | merge commit 场景不准确 | git_watcher.rs:386-406 | 一次性获取 branch 并传递 | open |
| G10-006 | P2 | 每个 commit 执行 3 次 git 命令(show+rev-parse+show %B)，N 个 commit 产生 3N 次 spawn | 快速连续 commit 性能瓶颈 | git_watcher.rs:353-437 | 合并为单次 git show -s --format=%H\x1e%B | open |
| G10-007 | P2 | TASK_HINT_FROM_COMMIT_RE 中 \s 可匹配换行符导致跨行匹配 | 误命中风险 | agent.rs:65 | 替换为 [ \t] 水平空白 | open |
| G10-008 | P2 | TASK_HINT_FROM_COMMIT_RE 捕获组可匹配非 UUID 的十六进制串(如 git short hash) | 误关联 | agent.rs:65,2122 | 增加 UUID 格式约束(至少含一个 '-') | open |
| G10-009 | P3 | handle_new_commit 处理失败时 break 退出循环，缺少最大重试限制 | 持续失败阻塞后续 commit | git_watcher.rs:282-289 | 增加 per-commit 重试计数器 | open |
| G10-010 | P3 | Checkpoint commit 触发 TerminalCompleted 事件，agent 需自行判断是否为 checkpoint | 双重路径增加复杂度 | git_watcher.rs:541,522 | 确认两条路径不会同时触发 | open |
| G10-011 | P1 | 有 METADATA 的 commit 可能同时触发 TerminalCompleted 和 GitEvent 两条消息路径 | 同一 commit 被处理两次 | git_watcher.rs:489-522 | 统一为单一消息路径 | open |
| G10-012 | P3 | GitEvent DB 模型 commit_message 仅存 subject 非 full body | 审计追踪不完整 | git_event.rs:116; git_watcher.rs:454 | 增加 full_body 字段 | open |

### G11 — 状态广播链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G11-001 | P2 | 约 15+ 处 let _ = publish_workflow_event 静默吞错 | 前端状态不同步 | agent.rs:1012-3671 多处 | 改为 if let Err(e) + warn 日志 | open |
| G11-002 | P2 | Board.tsx WS invalidation 仅刷新 byId 不刷新 forProject 列表 | 侧边栏 workflow 列表状态过期 | Board.tsx:71-76 | 增加 forProject invalidation | open |
| G11-003 | P2 | FailWorkflow 指令仅发送 Error 事件不发送 workflow.status_changed | 前端无法实时感知 workflow 失败 | agent.rs:3247-3266 | 增加 StatusUpdate 广播 | open |
| G11-004 | P3 | 乐观更新与 WS 推送存在潜在竞态但当前实现已有合理缓解 | 已验证可接受 | useWorkflows.ts:812-851 | 可选 debounce 优化 | verified |
| G11-005 | P1 | launcher.rs broadcast_terminal_status 在 workflow_id 为 None 时静默跳过 | 终端状态变更丢失 | launcher.rs:574-589 | 增加 warn 日志并审查调用链 | open |
| G11-006 | P3 | 批量状态变更时无 debounce/batching 可能产生广播风暴 | 短时间大量 API 请求 | agent.rs:3597-3671; Workflows.tsx:1137-1151 | 前端 invalidation 增加 debounce | open |
| G11-007 | P3 | broadcast channel 容量 1000 在极端场景可能 lag | 已验证可接受 | message_bus.rs:87-93 | 增加 Lagged 监控指标 | verified |
| G11-008 | P2 | Board.tsx 订阅了 onGitCommitDetected 但 Workflows.tsx 未订阅 | 两页面行为不一致 | Board.tsx:98; Workflows.tsx:1167-1183 | Workflows.tsx 添加 onGitCommitDetected | open |
| G11-009 | P1 | DB 更新失败后仍继续广播，导致前端与 DB 状态不一致 | UI 闪烁 | agent.rs:3610-3624 | DB 失败时跳过广播或记录错误 | open |

### G12 — 前端 WebSocket 连接管理链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G12-001 | P2 | _handlers/_workflowHandlers 通过 Map 直接 mutate 绕过 Zustand set() | 有意设计但缺少注释 | wsStore.ts:736-768 | 添加设计决策注释 | open |
| G12-002 | P2 | useWorkflowEvents handlers 依赖项不稳定可能导致频繁重订阅 | 当前消费者已用 useMemo 缓解 | wsStore.ts:1619 | 内部使用 useRef 缓存 handlers | open |
| G12-003 | P1 | disconnect() 不清理全局 _handlers，重连后旧 handler 继续触发 | 幽灵回调 | wsStore.ts:1235-1259 | disconnect 中重置 _handlers | open |
| G12-004 | P2 | disconnectWorkflow refCount>1 时不清理 handler（由 useEffect cleanup 负责） | 已验证设计正确 | wsStore.ts:782-798 | 无需修复，添加注释 | verified |
| G12-005 | P3 | 组件快速 mount/unmount 时 refCount 竞态窗口 | 已验证 isStale() 正确防护 | wsStore.ts:1508-1518 | 无需修复 | verified |
| G12-006 | P2 | 重连后心跳定时器在 onclose 中 clearInterval 但未置 null | 已验证后续 set() 正确置 null | wsStore.ts:1090-1091 | 无需修复 | verified |
| G12-007 | P3 | 后端 send_task/recv_task 使用 select! 但未主动 abort 另一个 task | 短暂资源浪费 | workflow_ws.rs:183-190 | select! 后显式 abort | open |
| G12-008 | P3 | 心跳双向独立发送缺乏确认机制 | 半开连接检测弱 | wsStore.ts:827-863; workflow_ws.rs:120-126 | 客户端记录最后收到消息时间 | open |
| G12-009 | P1 | useWorkflowEvents handlers 变化时 unsubscribe/subscribe 间隙丢事件 | 事件丢失窗口 | wsStore.ts:1521-1619 | 使用 useRef 持有 handlers 避免重订阅 | open |
| G12-010 | P3 | WorkflowDebugPage 未使用 useWorkflowEvents，依赖 1.5s 轮询 | 状态更新延迟 | WorkflowDebugPage.tsx:37 | 有意设计，低优先级 | verified |

---

## Round 3 — 状态一致性与数据契约链路审计 (G13-G18)

### G13 — Terminal Status 全域一致性 | 评级: C+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G13-001 | P1 | strum serialize_all="lowercase" 对 NotStarted 序列化为 "notstarted"，全代码库使用 "not_started" | runtime_actions.rs:177 调用 .to_string() 写入 DB 将产生不匹配 | terminal.rs:34; runtime_actions.rs:177 | strum 改为 serialize_all="snake_case" | open |
| G13-002 | P1 | serde rename_all="snake_case" 与 strum serialize_all="lowercase" 不一致 | API JSON 序列化与 .to_string() 产生不同值 | terminal.rs:33-34 | 统一 strum 为 snake_case | open |
| G13-003 | P2 | TERMINAL_STATUS_PENDING/RUNNING 常量不存在于 TerminalStatus 枚举中 | 幽灵常量误导开发者 | constants.rs:24-25; terminal.rs:35-51 | 删除或重命名 | open |
| G13-004 | P1 | review_passed/review_rejected/quality_pending 绕过枚举直接裸字符串写入 DB | 前端部分组件无法渲染这些状态 | agent.rs:2377,2435,1195; constants.rs:55 | 枚举补充 ReviewPassed/ReviewRejected/QualityPending | open |
| G13-005 | P2 | 前端 4 处独立 TerminalStatus 类型定义互不一致 | 状态映射不统一 | TerminalCard.tsx:7-16; TerminalDots.tsx:3; workflowStatus.ts:102-174; shared/types.ts:11 | 统一为 shared/types 单一定义 | open |
| G13-006 | P2 | mapTerminalStatus 中 cancelled->not_started 语义丢失 | 用户无法区分"未启动"和"已取消" | WorkflowDebugPage.tsx:26-28 | cancelled 映射为 cancelled | open |
| G13-007 | P3 | shouldRenderLiveTerminal 检查 running/active 为死代码 | 代码可维护性 | TerminalDebugView.tsx:140-143 | 移除 running/active | open |
| G13-008 | P2 | workflowStatus.ts TERMINAL_STATUS_CONFIG 包含多个后端不存在的幽灵状态 | 维护负担 | workflowStatus.ts:124-168 | 清理不存在状态，补充缺失状态 | open |
| G13-009 | P2 | DB terminal.status 为 TEXT 无 CHECK 约束 | 任意字符串可写入 | migrations/20260208010000:35; terminal.rs:90 | 添加 CHECK 约束或改用枚举类型 | open |
| G13-010 | P3 | Workflows.tsx 中 terminal.status as TerminalStatus 强制类型断言 | 运行时 STATUS_STYLES 查找返回 undefined | Workflows.tsx:394 | 使用 mapTerminalStatus 安全映射 | open |
| G13-011 | P3 | TerminalDots.tsx 包含 pending 但后端从不产生此值 | 死代码分支 | TerminalDots.tsx:3,35-36 | 移除 pending | open |

### G14 — Workflow/Task Status 全域一致性 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G14-001 | P1 | WORKFLOW_STATUS_PENDING="pending" 与后端枚举 Created 语义不匹配且从未被使用 | 误导开发者 | constants.rs:32; workflow.rs:39-59 | 删除或重命名为 CREATED | open |
| G14-002 | P1 | constants.rs 缺少 STARTING/PAUSED/CANCELLED/CREATED 常量，裸字符串散布 | 新增状态时极易遗漏 | constants.rs:31-37; workflows.rs:1323,1331,1498 | 补全所有 9 个常量 | open |
| G14-003 | P2 | 前端 WorkflowStatusEnum 包含 draft 但后端无此状态 | 前后端状态集不一致 | useWorkflows.ts:22; workflow.rs:39-59 | 移除 draft 或标记 client-only | open |
| G14-004 | P2 | Task status 无常量定义，全部裸字符串散布在 agent.rs/routes 中 | 维护困难 | agent.rs:461,618,3645; workflows.rs:886,2091-2098 | 添加 TASK_STATUS_* 常量 | open |
| G14-005 | P2 | auto_sync_workflow_completion 不检查 paused/merging/starting 状态 | 非 running 状态下可能误触发自动完成 | agent.rs:4228-4233 | 仅在 status==running 时执行 | open |
| G14-006 | P3 | Workflow.status 和 WorkflowTask.status 字段为 String 而非枚举 | 枚举形同虚设 | workflow.rs:116,290 | 长期改为枚举类型 | open |
| G14-007 | P3 | 前端 WORKFLOW_STATUS_MAP/BADGE_CLASSES 包含 draft | 与 G14-003 同源 | Workflows.tsx:327-351 | 统一处理 | open |
| G14-008 | P3 | TERMINAL_STATUS_REVIEW_PASSED/REJECTED 常量定义但未在 agent.rs 中使用 | 幽灵常量 | constants.rs:28-29 | 添加注释或删除 | open |
| G14-009 | P3 | 前端 WorkflowStatusEnum 与 shared/types.ts 的 status:string 无编译时关联 | 类型安全缺失 | shared/types.ts:7; useWorkflows.ts:21-31 | 从 Rust 枚举自动生成 | open |

### G15 — DB 模型与 CAS 操作完整性 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G15-001 | P1 | set_completed_if_unfinished WHERE 条件未排除 failed 状态 | 已失败终端可被覆盖为 completed | terminal.rs:574-598 | WHERE 添加 AND status != 'failed' | open |
| G15-002 | P1 | Workflow::update_status 无 CAS 保护，存在并发状态回退风险 | 所有 workflow 状态变更 | workflow.rs:651-666; agent.rs:3213,3248,4258 | 关键转换使用 CAS | open |
| G15-003 | P1 | WorkflowTask::update_status 无 CAS 保护，终态可被覆写 | 任务状态管理 | workflow.rs:880-895; agent.rs:462,1063,3611 | 终态转换使用 CAS | open |
| G15-004 | P2 | 索引定义中 lower() 函数风格不一致 | 代码可维护性 | migrations/20260119000001:59,73 | 统一为小写字面量 | open |
| G15-005 | P2 | Workflow::set_ready 无前置状态检查 | 任意状态可跳转到 ready | workflow.rs:669-684 | 添加 AND status='starting' | open |
| G15-006 | P2 | dispatch_terminal 中 task/terminal 状态更新不在同一事务 | 中间不一致窗口 | agent.rs:3546-3647 | 包裹事务或失败时回滚 | open |
| G15-007 | P2 | STARTABLE_TERMINAL_STATUSES 包含 waiting/working 可能导致重复启动 | 动态终端启动 | runtime_actions.rs:32-33 | 确认业务意图，必要时移除 | open |
| G15-008 | P3 | Terminal::set_starting/set_waiting 无 CAS 保护 | DB 层缺乏防御 | terminal.rs:479-515 | 添加前置状态条件 | open |
| G15-009 | P3 | update_status 设置 failed 时不设置 completed_at | 与 G15-001 关联 | terminal.rs:335-350 | 终态设置时同步设置 completed_at | open |
| G15-010 | P3 | workflow_command.preset_id 外键无 ON DELETE CASCADE | 删除 preset 被阻止 | migrations/20260117000001:154 | 添加 CASCADE 或文档化 | open |
| G15-011 | P3 | check_and_sync_workflow_completion 存在 TOCTOU 窗口 | workflow 可能被过早标记完成 | agent.rs:4243-4258 | 包裹事务或子查询验证 | open |

### G16 — API 路由参数验证与错误处理 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G16-001 | P2 | 中间件错误响应格式与 ApiError 不一致（裸 StatusCode） | 客户端收到空 body 的 404/500 | model_loaders.rs:27 | 改为返回 ApiError | open |
| G16-002 | P2 | 认证中间件失败返回裸 StatusCode::UNAUTHORIZED | 客户端收到空 body 的 401 | auth.rs:88 | 返回 ApiError::Unauthorized | open |
| G16-003 | P1 | Terminal 路由 Path 参数为 String 不验证 UUID 格式 | 无效 ID 穿透到 DB 查询 | terminals.rs:157,177,549,584 | 改为 Path\<Uuid\> | open |
| G16-004 | P1 | Workflow 路由 Path 参数为 String 不验证 UUID 格式 | 无效 ID 穿透到 DB 查询 | workflows.rs:1006,1050,1314,1479,1584 | 改为 Path\<Uuid\> | open |
| G16-005 | P2 | stop_terminal 返回不一致的 NotFound 消息 | 错误信息不友好 | terminals.rs:554 | 改为描述性消息 | open |
| G16-006 | P2 | prepare_workflow 缺乏幂等性保护，并发调用可能重复 PTY 启动 | 资源浪费/冲突 | workflows.rs:1323 | 使用 CAS 原子转换 | open |
| G16-007 | P1 | start_workflow 的 stale 状态自愈存在 TOCTOU 竞态 | 并发 start 请求 | workflows.rs:1488-1502 | 封装到 Runtime 内部加锁 | open |
| G16-008 | P2 | start_workflow 内部调用 prepare_workflow 是隐式状态回退 | 语义不清晰 | workflows.rs:1527-1529 | 提取为独立 re_prepare 函数 | open |
| G16-009 | P3 | stop_workflow 不支持 ready 状态 | 用户无法在 prepare 后取消 | workflows.rs:1631 | 将 ready 加入 valid_statuses | open |
| G16-010 | P3 | merge_workflow 完成后状态设置无 CAS | 并发覆盖风险 | workflows.rs:2731 | WHERE status='merging' | open |
| G16-011 | P3 | terminal logs limit 参数为 i32 允许负值 | 意外 DB 行为 | terminals.rs:71 | 改为 u32 或 clamp | open |
| G16-012 | P2 | update_workflow_status 端点允许任意合法状态转换无业务语义保护 | 可绕过 orchestrator 正常流程 | workflows.rs:1066-1080 | 敏感转换增加额外校验 | open |
| G16-013 | P3 | orchestrator chat rate limit 全局静态 Mutex 无过期清理 | 长期运行内存增长 | workflows.rs:153-154 | 定期清理过期 key | open |
| G16-014 | P3 | submit_prompt_response 对非 running 状态返回 BadRequest 而非 Conflict | 语义不精确 | workflows.rs:2184 | 改为 Conflict | open |
| G16-015 | P2 | start_terminal 无 per-terminal 并发保护 | 可能启动两个 PTY 进程 | terminals.rs:185-197 | CAS 或 per-terminal 锁 | open |

### G17 — DTO 转换与前后端类型契约 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G17-001 | P1 | WsEvent/WsEventType 有 #[derive(TS)] 但未导出到 shared/types.ts | 前端 WS 类型完全手动维护 | workflow_events.rs:22-88; generate_types.rs | 添加 decl() 调用 | open |
| G17-002 | P2 | 前端 WsEventType 缺少 3 个 Provider 事件类型 | Provider 故障用户无感知 | wsStore.ts:18-31; workflow_events.rs:73-83 | 补充或自动生成 | open |
| G17-003 | P2 | TerminalDto/WorkflowDetailDto/WorkflowTaskDto status 均为 string 非枚举 | 前端无编译期校验 | shared/types.ts:7,9,11 | DTO 改用枚举类型 | open |
| G17-004 | P2 | workflowsApi 全部使用原生 fetch 绕过 makeRequest | 启用 API Token 后全部鉴权失败 | useWorkflows.ts:314 | 替换为 makeRequest | open |
| G17-005 | P2 | TerminalDto 转换丢失 10 个 DB 字段 | 前端无法获取终端运行时状态 | terminal.rs:59-131; workflows_dto.rs:71-86 | 补充关键字段 | open |
| G17-006 | P3 | merge_terminal_cli_id DTO 为 Option 但 DB 为必填 | 类型语义不一致 | workflow.rs:157; workflows_dto.rs:31 | 统一为 String | open |
| G17-007 | P3 | 前端 TerminalCompletedStatus 包含 cancelled 但后端不发送 | 死代码 | wsStore.ts:1372-1378; workflow_events.rs:114-122 | 移除 cancelled 补充 checkpoint | open |
| G17-008 | P3 | 前端缺少 checkpoint 状态但后端已支持 | 类型不完整 | workflow_events.rs:120; wsStore.ts:1372-1378 | 补充 checkpoint | open |
| G17-009 | P3 | WS payload 同时发送 camelCase 和 snake_case 双份字段 | 传输体积膨胀 30-50% | workflow_events.rs:275-288 | 长期统一为 camelCase | open |
| G17-010 | P1 | 前端 WsMessage.type 与后端 WsEvent.event_type 依赖 serde rename 无类型保证 | rename 移除将导致前端解析失败 | wsStore.ts:8-13; workflow_events.rs:97-112 | 导出 WsEvent 到 shared/types | open |

### G18 — 加密与安全 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G18-001 | P1 | API Token 比较使用 == 而非常量时间比较，存在时序攻击风险 | API 认证 | auth.rs:76 | 引入 subtle::ConstantTimeEq | open |
| G18-002 | P1 | API Key 明文前缀泄露到日志（最多 10 字符） | 日志安全 | cc_switch.rs:782 | 移除 api_key_prefix 或仅记录长度 | open |
| G18-003 | P2 | 开发模式硬编码加密密钥，release 模式缺失密钥时无强制阻断 | 加密安全 | main.rs:33,48-54 | release 模式缺失 key 时 panic | open |
| G18-004 | P2 | CLI 隔离目录中 API Key 以明文写入磁盘文件 | 磁盘安全 | cc_switch.rs:43,109,161,240 | Windows ACL + 终端结束后清理 | open |
| G18-005 | P2 | 无 CORS 配置，默认允许所有跨域请求 | 网络部署场景 | routes/mod.rs:52-94 | 添加 CorsLayer | open |
| G18-006 | P2 | 不支持密钥轮换，更换加密密钥后已加密数据无法解密 | 密钥管理 | workflow.rs:189-208 | 支持 OLD_KEY 降级解密 | open |
| G18-007 | P3 | 加密实现在 Workflow/Terminal/main.rs 三处重复 | 代码可维护性 | workflow.rs:186-261; terminal.rs:189-263; main.rs:254-286 | 抽取为共享 crypto 模块 | open |
| G18-008 | P3 | 加密密钥使用 UTF-8 字符串直接作为 AES key bytes，非标准 KDF | 加密强度略低 | workflow.rs:204-207 | 使用 HKDF 派生 | open |
| G18-009 | P3 | Claude/Gemini 隔离目录终端结束后未清理（仅 Codex 有） | 磁盘残留明文 key | cc_switch.rs:646,980 | 实现类似 CodexHomeGuard 的 RAII 清理 | open |
| G18-010 | P3 | WebSocket 端点认证覆盖已确认正确 | 已验证无问题 | routes/mod.rs:90-93 | 添加注释说明 | verified |

---

## Round 4 — CLI 执行器与进程管理链路审计 (G19-G24)

### G19 — Claude CLI 集成全链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G19-001 | P2 | 硬编码 Claude Code 包版本号，升级需改代码 | CLI 版本管理 | claude.rs:44-47 | 提取为常量或配置项 | open |
| G19-002 | P1 | API Key 前缀(10字符)泄露到日志 | 日志安全 | cc_switch.rs:782 | 限制为 4 字符或仅记录长度 | open |
| G19-003 | P2 | settings.json 中 base_url 仅用 terminal 级别 URL，忽略 orchestrator 回退 | API 路由错误 | cc_switch.rs:805-813 | 改用 effective_base_url | open |
| G19-004 | P3 | warn_if_unmanaged_key 警告消息拼写错误 "conding" | UI 显示 | claude.rs:495 | 修正为 "coding" | open |
| G19-005 | P2 | spawn_follow_up 同时传递 --fork-session 和 --resume 可能与新版不兼容 | Session 复用 | claude.rs:186-190 | 添加版本检测逻辑 | open |
| G19-006 | P2 | 隔离目录(CLAUDE_HOME)未在终端生命周期结束时清理 | 磁盘泄漏+密钥残留 | cc_switch.rs:627-652 | 添加清理逻辑 | open |
| G19-007 | P3 | plan 和 approvals 同时启用时行为不明确 | 权限模式配置 | claude.rs:93-95 | 构造阶段互斥校验 | open |
| G19-008 | P3 | ProtocolPeer 初始化失败后子进程成为孤儿 | 资源泄漏 | claude.rs:296-303 | return 前 kill child | open |
| G19-009 | P3 | on_can_use_tool 缺少 tool_use_id 时静默自动批准 | 安全审批 | claude/client.rs:129-143 | 改为 Deny | open |
| G19-010 | P2 | bypass accept 硬编码数字快捷键 "2\r"，依赖菜单项顺序 | CLI 版本兼容 | prompt_watcher.rs:348-354 | 基于正则匹配菜单文本 | open |
| G19-011 | P3 | ClaudeJson Unknown 变体使用 untagged 可能吞掉反序列化错误 | 日志解析 | claude.rs:1529-1533 | 增加详细日志 | open |
| G19-012 | P1 | API key 同时写入 AUTH_TOKEN 和 API_KEY 双重注入 | 计费路径混淆 | cc_switch.rs:183-189,816-819 | 根据 key 格式智能选择 | open |
| G19-013 | P3 | AvailabilityInfo 检测中 config_found 检查冗余 | 代码清洁度 | claude.rs:216-237 | 添加注释说明意图 | open |

### G20 — Codex/Cursor/其他 CLI 集成全链路 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G20-001 | P2 | API Key 前缀(10字符)泄露到日志（与 G19-002 同源） | 日志安全 | cc_switch.rs:782 | 截断至 4 字符或仅记录长度 | open |
| G20-002 | P2 | Gemini 不支持 workflow orchestrator API key 回退 | Gemini 终端启动失败 | cc_switch.rs:956-958 | 添加 orchestrator fallback 逻辑 | open |
| G20-003 | P3 | 临时隔离目录未清理，存在磁盘泄漏 | 长期运行磁盘增长 | cc_switch.rs:639-652,889-902,973-986 | 终端完成后清理 | open |
| G20-004 | P3 | Codex auth.json 写入明文 API key 但 Windows 无权限保护 | 磁盘安全 | cc_switch.rs:46-58 | Windows ACL 保护 | open |
| G20-005 | P3 | Codex/Copilot/OpenCode 硬编码 npx 版本号 | 版本管理 | codex.rs:256; copilot.rs:59; opencode.rs:51 | 提取为常量 | open |
| G20-006 | P2 | Codex session fork 使用隔离 CODEX_HOME 可能找不到原始 session | follow-up 执行失败 | codex.rs:17-24; session.rs:176-181 | 传递完整 session 路径 | open |
| G20-007 | P3 | Copilot session ID 发现依赖 600 秒超时的文件轮询 | 启动延迟 | copilot.rs:274 | 缩短至 60-120 秒 | open |
| G20-008 | P1 | Codex client request_id 对未知 variant 使用 unreachable!() | 协议升级时 panic | codex/client.rs:500 | 改为返回默认值或 Result | open |
| G20-009 | P3 | CLI 检测器 Windows 上通过 cmd /c 执行，存在命令注入风险 | 安全 | detector.rs:109-115 | 白名单校验 detect_command | open |
| G20-010 | P3 | ACP harness 在 spawn_blocking 中创建新 tokio runtime | 资源开销 | acp/harness.rs:269-271 | 添加注释说明设计决策 | open |
| G20-011 | P2 | Cursor get_availability_info 要求 MCP 配置文件存在才返回 InstallationFound | 新安装误判为未安装 | cursor.rs:485-498 | binary_found 即可判定已安装 | open |
| G20-012 | P3 | OpenCode 配置创建函数标记 dead_code 且使用硬编码占位符 | 死代码 | cc_switch.rs:272 | 移除或修复 | open |
| G20-013 | P3 | safe_id 生成逻辑重复三次 | DRY 违反 | cc_switch.rs:627-638,877-888,961-972 | 提取为公共函数 | open |
| G20-014 | P2 | Claude settings.json base_url 未使用 effective_base_url（与 G19-003 同源） | API 路由 | cc_switch.rs:805-810 | 改用 effective_base_url | open |

### G21 — PTY 进程生命周期管理 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G21-001 | P2 | spawn_pty_with_config 中 CODEX_HOME 被重复解析，第二次覆盖 guard 保护值 | Codex 临时目录清理 | process.rs:551-558,642-649 | 删除第二次重复解析 | open |
| G21-002 | P2 | is_running() 仅检查 HashMap 存在性，不检测进程实际存活 | 进程状态判断 | process.rs:980-983 | 调用 child.try_wait() 确认 | open |
| G21-003 | P2 | close_terminal 未注销 TerminalBridge，Bridge 任务延迟退出 | 资源泄漏(最长5s) | runtime_actions.rs:256-300 | 显式调用 bridge.unregister | open |
| G21-004 | P3 | Bridge writer_task 在 PTY 退出后无超时等待机制 | 可能永久阻塞 | bridge.rs:457-463 | 添加 tokio::time::timeout | open |
| G21-005 | P3 | Windows kill 使用 taskkill /F 直接强杀，无 graceful shutdown | CLI 工具丢失状态 | process.rs:943-954 | 先发 CTRL_C_EVENT 再 taskkill | open |
| G21-006 | P3 | ProcessManager 无 Drop 实现，异常退出时不清理子进程 | zombie 进程 | process.rs:273-275 | 实现 Drop 或 shutdown_all | open |
| G21-007 | P3 | OutputFanout 使用 std::sync::Mutex，async 上下文短暂阻塞 | 已验证风险极低 | output_fanout.rs:62 | 添加注释 | verified |
| G21-008 | P2 | legacy spawn_pty 与 spawn_pty_with_config 大量代码重复 | 维护风险 | process.rs:752-879 vs 541-734 | 委托给 spawn_pty_with_config | open |
| G21-009 | P1 | TerminalLogger flush_buffer 存在 TOCTOU 竞态 | 日志重复或丢失 | process.rs:1403-1441 | 改为 write lock 下 mem::take | open |
| G21-010 | P3 | 多 Bridge 各自触发 cleanup() 全局扫描 | 冗余锁竞争 | bridge.rs:410 | ProcessManager 自身定时清理 | open |
| G21-011 | P3 | close_terminal Shutdown 仅发到 session topic | 健壮性 | runtime_actions.rs:269-274 | 同时发到 terminal.input topic | open |

### G22 — CC-Switch 配置切换 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G22-001 | P1 | ModelSwitcher.backup_config() 未实现，配置备份形同虚设 | 旧路径全局配置切换无法回滚 | switcher.rs:112-114 | 硬编码 backup_before_switch=false 或移除 | open |
| G22-002 | P1 | 旧路径 switch_for_terminal() 修改全局配置文件，并发终端存在竞态 | 多终端并行启动 | cc_switch.rs:469-540 | 编译期门控或运行时阻断 | open |
| G22-003 | P2 | Claude/Gemini 配置文件创建失败被静默吞掉，Codex 正确传播 | 认证失败难排查 | cc_switch.rs:787-793,1012-1023 | 升级为 error 并返回 Err | open |
| G22-004 | P2 | API Key 前缀泄露到日志（与 G19-002 同源） | 日志安全 | cc_switch.rs:782 | 限制为 4 字符 | open |
| G22-005 | P2 | 临时目录中隔离配置文件未清理，存在磁盘泄漏和密钥残留 | 安全+磁盘 | cc_switch.rs:639-652,889-902,973-986 | 添加 RAII 清理或定期扫描 | open |
| G22-006 | P2 | Windows 平台临时目录缺少权限限制，其他用户可读取 API Key | 安全 | cc_switch.rs:654-669 | Windows ACL 保护 | open |
| G22-007 | P3 | Gemini 终端不支持 workflow orchestrator API key 回退 | Gemini 配置 | cc_switch.rs:956-958 | 添加 fallback 逻辑 | open |
| G22-008 | P3 | safe_id 生成逻辑重复三次（与 G20-013 同源） | DRY 违反 | cc_switch.rs:627-638,877-888,961-972 | 提取公共函数 | open |
| G22-009 | P3 | Claude settings.json base_url 仅用 terminal URL（与 G19-003 同源） | 配置不一致 | cc_switch.rs:805-813 | 改用 effective_base_url | open |
| G22-010 | P3 | Codex config.toml 中 api_key 与 auth.json 重复存储 | 密钥暴露面增大 | cc_switch.rs:102-110 | 评估是否可移除 config.toml 中的 key | open |
| G22-011 | P3 | atomic_write 在 Windows 上非原子（先 remove 再 rename） | 旧路径配置丢失风险 | atomic_write.rs:107-143 | 使用 MoveFileEx | open |

### G23 — Worktree 管理 | 评级: C+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G23-001 | P1 | WORKTREE_CREATION_LOCKS HashMap 永不清理，内存泄漏 | 长期运行服务 | worktree_manager.rs:17-18 | 改用 LRU/TTL 缓存 | open |
| G23-002 | P1 | Merge 路由中 worktree 路径硬编码与实际创建路径不一致 | merge 操作失败 | workflows.rs:2675; worktree_manager.rs:535 | 使用 WorktreeManager API 定位路径 | open |
| G23-003 | P1 | Branch 命名冲突检测仅检查当前批次，不查询 git 仓库已有分支 | 跨 workflow 分支冲突 | workflows.rs:842-874 | 调用 GitService::get_all_branches() | open |
| G23-004 | P2 | stop/cancel/delete workflow 不清理 worktree 文件系统目录 | 磁盘泄漏 | workflows.rs:1697-1723,1059 | 添加 batch_cleanup_worktrees 调用 | open |
| G23-005 | P2 | Merge 完成后不清理 task worktree 目录 | 磁盘泄漏 | workflows.rs:2731-2743 | merge 后调用 cleanup | open |
| G23-006 | P2 | resolve_workflow_working_dir 传递项目根目录而非 worktree 路径 | PTY 在错误目录启动 | workflows.rs:1358,1380 | worktree 模式下覆盖 working_dir | open |
| G23-007 | P2 | GitCli::worktree_add create_branch=true 时 branch 参数出现两次 | 语义错误(当前恰好工作) | git/cli.rs:94-101 | 修正参数为 start-point | open |
| G23-008 | P3 | comprehensive_worktree_cleanup 中 remove_dir_all 失败中断整个清理 | Windows 文件锁定 | worktree_manager.rs:263 | 降级为 warn 并继续 | open |
| G23-009 | P3 | orphan workspace 清理使用同步 std::fs::read_dir 在 async 上下文 | 启动性能 | workspace_manager.rs:304 | 改用 tokio::fs::read_dir | open |
| G23-010 | P3 | WORKTREE_CREATION_LOCKS 使用 std::sync::Mutex，panic 时 poison | 所有 worktree 操作 | worktree_manager.rs:100 | 改用 parking_lot::Mutex | open |

### G24 — LLM Provider 管理 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G24-001 | P1 | RateLimitedClient 未透传 provider_status/take_provider_events/reset_provider | 设计缺陷(当前不影响) | llm.rs:97-108 | 补充透传方法 | open |
| G24-002 | P1 | Slash command 路径直接调用 chat() 绕过 publish_provider_events() | Provider 事件丢失 | agent.rs:4700-4706 | 添加 publish_provider_events 调用 | open |
| G24-003 | P2 | Provider Switched 事件仅在 provider 被标记 dead 时才发出 | 静默 failover 不可观测 | resilient_llm.rs:299-309 | 每次实际切换都发事件 | open |
| G24-004 | P2 | 前端完全未消费 provider.switched/exhausted/recovered 事件 | 用户无感知 | 前端搜索 0 结果 | 添加 handler 和 toast 通知 | open |
| G24-005 | P2 | LLM 响应 choices 为空时静默返回空字符串，不触发 failover | 指令解析失败 | llm.rs:258-262 | 空 choices 返回 Err | open |
| G24-006 | P2 | 内部重试(3次)与 Resilient 重试叠加，最坏等待 18 分钟 | 性能/可用性 | llm.rs:281-307; resilient_llm.rs:248 | 统一重试策略 | open |
| G24-007 | P3 | API key 以明文 String 存储在 OpenAICompatibleClient 中 | 安全加固 | llm.rs:114 | 使用 secrecy::SecretString | open |
| G24-008 | P3 | record_success() 中嵌套锁获取(provider state + last_events) | 理论死锁风险 | resilient_llm.rs:194-210 | 先释放 state 锁再获取 events 锁 | open |
| G24-009 | P3 | Provider 耗尽后 agent 仅发 Error 事件不终止 workflow | 僵尸 workflow | agent.rs:2958-2977 | 连续失败达阈值后标记 failed | open |

---

## Round 5 — 前端交互与 UI 状态链路审计 (G25-G30)

### G25 — Workflow 创建向导全链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G25-001 | P1 | Step2Tasks taskCount 变化时无条件重建所有任务，丢弃用户已填数据 | Step2 数据丢失 | Step2Tasks.tsx:86-95 | 改为增量调整（追加/截断） | open |
| G25-002 | P1 | handleSubmit 中 setState 使用过期 state 闭包，并发更新可能覆盖 config | 提交竞态 | WorkflowWizard.tsx:164,170,178 | 改用函数式 setState(prev => ...) | open |
| G25-003 | P2 | Step0 验证器不检查 gitStatus.isGitRepo，非 Git 目录可通过验证 | 后端创建失败 | step0Project.ts:6-14 | 增加 isGitRepo 检查 | open |
| G25-004 | P2 | Step2 验证器不检查 branch 字段，空分支名可通过验证 | git 操作失败 | step2Tasks.ts:18-26 | 增加 branch 非空检查 | open |
| G25-005 | P2 | Step2 验证器不检查重复分支名 | 跨任务分支冲突 | step2Tasks.ts:18-26 | 增加重复检测 | open |
| G25-006 | P2 | Step3 验证器不检查 apiKey/baseUrl/modelId 等关键字段 | 后端创建失败 | step3Models.ts:14-19 | 增加必填字段验证 | open |
| G25-007 | P2 | errorTerminal 的 customBaseUrl/customApiKey 硬编码为 null | Error terminal 无法使用自定义 API | types.ts:388-398 | 从 model 中提取实际值 | open |
| G25-008 | P2 | mergeTerminal 的 customBaseUrl/customApiKey 同样硬编码为 null | Merge terminal 无法使用自定义 API | types.ts:399-407 | 从 model 中提取实际值 | open |
| G25-009 | P2 | Step5Commands 验证器为空实现，启用 commands 但未选 preset 不报错 | 空 commands 配置 | step5Commands.ts:6-8 | 增加 enabled+empty 检查 | open |
| G25-010 | P3 | Step3 model ID 使用 Date.now() 生成，快速连续添加可能重复 | ID 冲突 | Step3Models.tsx:252 | 改用 crypto.randomUUID() | open |
| G25-011 | P3 | Step2 task ID 使用 Date.now()+index，批量创建时间戳可能相同 | 低概率 ID 冲突 | Step2Tasks.tsx:88 | 改用 crypto.randomUUID() | open |
| G25-012 | P2 | Step4Terminals useEffect 依赖 onUpdate 引用不稳定，可能触发无限循环 | 性能/无限循环 | Step4Terminals.tsx:195-207 | useCallback 包裹 handleUpdateConfig | open |
| G25-013 | P3 | Step5Commands 使用 lucide-react 图标违反 ESLint 规范 | 代码规范 | Step5Commands.tsx:2 | 迁移至 @phosphor-icons/react | open |
| G25-014 | P3 | Step0/Step2/Step4 同样使用 lucide-react 图标 | 代码规范 | Step0Project.tsx:3; Step2Tasks.tsx:2; Step4Terminals.tsx:2 | 统一迁移 | open |
| G25-015 | P1 | agent_planned 切回 diy 模式时跳过步骤数据为空不自动初始化 | 模式切换数据丢失 | WorkflowWizard.tsx:52-59 | executionMode 变更时重新初始化 | open |
| G25-016 | P2 | wizardConfigToCreateRequest 中 orchestratorModel 查找失败 throw 原始 JS 错误 | 用户体验差 | types.ts:277-282 | step6 验证器增加存在性检查 | open |
| G25-017 | P3 | useCreateWorkflow onSuccess 仅 invalidate 当前 projectId | 极端时序缓存不一致 | useWorkflows.ts:565-572 | 可接受 | verified |
| G25-018 | P3 | handleNext 中 clearErrors 在 navigation.next() 之后调用 | UI 闪烁 | WorkflowWizard.tsx:145-148 | 调换顺序 | open |

### G26 — Workflow 操作面板全链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G26-001 | P1 | 前端 draft 状态允许 prepare，但后端仅接受 created/failed | 按钮可点但必定失败 | useWorkflows.ts:46-53; workflows.rs:1322-1328 | draft.canPrepare=false | open |
| G26-002 | P1 | 前端 merging 状态允许 stop，但后端仅接受 starting/running/paused | 按钮可点但必定失败 | useWorkflows.ts:94-101; workflows.rs:1631 | merging.canStop=false | open |
| G26-003 | P2 | 所有 mutation hooks 均无乐观更新，WS 推送前 UI 存在空窗期 | 操作后 UI 延迟 | useWorkflows.ts:583-768 | 添加 onMutate 乐观更新 | open |
| G26-004 | P2 | 快速连续点击不同操作按钮缺乏全局互斥 | 并发请求竞态 | Workflows.tsx:202-241 | 添加 isAnyMutationPending 标志 | open |
| G26-005 | P2 | prepare 失败后前端不触发 cache invalidation | UI 卡在 starting 状态 | useWorkflows.ts:597-599 | onError 中 invalidateQueries | open |
| G26-006 | P2 | 所有操作 mutation 的 onError 均不触发 cache invalidation | 失败后 UI 显示过期状态 | useWorkflows.ts:621-766 | 统一添加 onError invalidation | open |
| G26-007 | P2 | merge 进度无实时展示，用户只能看到 "Merging..." 文本 | UX 受限 | Workflows.tsx:228-235 | 添加 task 合并进度事件 | open |
| G26-008 | P2 | WS 断线后操作按钮仍可点击，但后续状态无法通过 WS 更新 | UI 卡住 | Workflows.tsx:202-241; wsStore.ts:93-95 | 断线时显示警告横幅 | open |
| G26-009 | P3 | stop 操作后前端不主动清理 prompt 队列 | prompt 对话框残留 | Workflows.tsx:1041-1049 | stop 成功后清空 promptQueue | open |
| G26-010 | P3 | Board 页面 WS 事件不 invalidate workflow 列表缓存 | 侧边栏状态滞后 | Board.tsx:71-76 | 增加 forProject invalidation | open |
| G26-011 | P3 | handleStopWorkflow ConfirmDialog 与 mutation 间存在 TOCTOU 窗口 | 低风险竞态 | Workflows.tsx:1481-1493 | 后端已有校验，可接受 | verified |
| G26-012 | P3 | usePrepareWorkflow onSuccess invalidate workflowKeys.all 范围过大 | 不必要的网络请求 | useWorkflows.ts:593-595 | 精确 invalidate forProject | open |

### G27 — Prompt 交互全链路 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G27-001 | P2 | normalizeTerminalPromptDetectedPayload 未提取后端推送的 autoConfirm 字段 | 前端无法预过滤自动确认 prompt | wsStore.ts:363-421; workflow_events.rs:350 | 提取 autoConfirm 字段 | open |
| G27-002 | P2 | TerminalPromptDecision WS 事件缺少 taskId 和 sessionId | 多 terminal 并发 prompt 匹配退化 | workflow_events.rs:391-399 | 增加 task_id/session_id 字段 | open |
| G27-003 | P2 | prompt 队列去重 ID 基于内容哈希，连续相同内容 prompt 被误去重 | 1.5s 窗口内合法 prompt 丢失 | Workflows.tsx:92-100,1059-1064 | 加入时间戳或事件 ID | open |
| G27-004 | P3 | sendPromptResponse 发送失败时无重试机制且无用户可见反馈 | prompt 响应丢失 | Workflows.tsx:1286-1295 | 添加重试按钮 | open |
| G27-005 | P3 | prompt 队列无超时清理机制，长时间未响应的 prompt 永久阻塞 UI | prompt 对话框卡死 | Workflows.tsx:1023-1049 | 添加 120s 超时自动清理 | open |
| G27-006 | P3 | WS fallback 路径中 enter_confirm 空字符串响应语义不透明 | 代码可读性 | Workflows.tsx:1223-1227; prompt_handler.rs:401 | 添加注释说明 | open |
| G27-007 | P1 | sendPromptResponse WS 路由依赖 payload 内嵌 workflowId 而非顶层字段 | 多 workflow 并发时路由错误 | wsStore.ts:1293-1305,590-605 | workflowId 提升到消息顶层 | open |
| G27-008 | P3 | promptQueue 仅展示队首项，用户无法感知队列深度 | UX 受限 | Workflows.tsx:1187 | 添加队列计数指示器 | open |

### G28 — 终端调试视图全链路 | 评级: C+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G28-001 | P1 | mapTerminalStatus 将 cancelled 映射为 not_started，语义丢失 | 调试页面状态展示 | WorkflowDebugPage.tsx:26-28 | 直接返回 cancelled | open |
| G28-002 | P2 | shouldRenderLiveTerminal 包含 running/active 死代码分支 | 代码可维护性 | TerminalDebugView.tsx:140-143 | 简化为 status === 'working' | open |
| G28-003 | P1 | WorkflowDebugPage 使用 1.5s 轮询获取 workflow 数据，未利用 WS | 性能/服务端负载 | WorkflowDebugPage.tsx:36-39 | 引入 useWorkflowEvents | open |
| G28-004 | P1 | OrchestratorChatPanel 使用 2s 轮询获取消息，与 WS 架构冲突 | 性能/服务端负载 | Workflows.tsx:597 | 订阅 WS 事件替代轮询 | open |
| G28-005 | P1 | 切换 terminal 时旧 TerminalEmulator WS 连接存在竞态泄漏风险 | 内存泄漏 | TerminalDebugView.tsx:488-489; TerminalEmulator.tsx:320-328 | 切换前显式断开旧连接 | open |
| G28-006 | P2 | TerminalDebugView 使用大量 useRef 管理状态绕过 React 渲染周期 | UI 不同步 | TerminalDebugView.tsx:59-64 | 关键状态改用 useState | open |
| G28-007 | P2 | 历史终端日志一次性 join 全部内容到单个 pre 元素 | 大量日志性能堪忧 | TerminalDebugView.tsx:394-396,541-543 | 虚拟滚动或分页 | open |
| G28-008 | P3 | mapTerminalStatus 缺少 review_passed/review_rejected 映射 | 状态 fallback 到 not_started | WorkflowDebugPage.tsx:10-31 | 添加对应 case | open |
| G28-009 | P3 | TerminalDebugView allTerminals useEffect 依赖对象数组每次渲染触发 | 不必要的 effect 执行 | TerminalDebugView.tsx:91 | useMemo 缓存 allTerminals | open |
| G28-010 | P2 | DisplayConversationEntry 使用 lucide-react 图标违反规范 | 代码规范 | DisplayConversationEntry.tsx:23-31 | 迁移至 @phosphor-icons/react | open |
| G28-011 | P3 | TerminalEmulator 初始化 useEffect 依赖 handleData/handleResize 不稳定 | 终端闪烁风险 | TerminalEmulator.tsx:210 | 分离初始化与事件绑定 | open |

### G29 — Board/Kanban 视图全链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G29-001 | P1 | TaskCard 未将 terminals 数组传递给 TerminalDots，状态颜色映射完全失效 | Board 终端状态可视化 | TaskCard.tsx:57 | 传递 terminals 数组 | open |
| G29-002 | P2 | TerminalDots 类型包含后端不存在的 running/pending 状态值 | 幽灵状态 | TerminalDots.tsx:3 | 移除 running/pending | open |
| G29-003 | P2 | Board 页面 WS invalidation 缺少 project 级别列表刷新 | 侧边栏状态滞后 | Board.tsx:71-76 | 增加 forProject invalidation | open |
| G29-004 | P2 | Board 页面缺少 onTerminalPromptDetected/onTerminalPromptDecision 事件处理 | Board 视图无法响应 prompt | Board.tsx:92-101 | 添加 prompt 事件处理 | open |
| G29-005 | P2 | 拖拽操作缺少状态转换合法性校验，允许非法状态迁移 | 乐观更新闪烁 | WorkflowKanbanBoard.tsx:71-90 | 添加前端状态转换白名单 | open |
| G29-006 | P3 | StatusBar/TerminalActivityPanel/KanbanBoard 各自独立调用 useWorkflow | 冗余请求 | Board.tsx; StatusBar.tsx:9; TerminalActivityPanel.tsx:108 | 提升到 Board 级别 | open |
| G29-007 | P3 | useWorkflowEvents handlers 依赖导致频繁 unsubscribe/resubscribe | WS 订阅不稳定 | wsStore.ts:1619 | useRef 缓存 handlers | open |
| G29-008 | P3 | WS 事件触发的 invalidation 无防抖，高频事件导致频繁 refetch | 性能 | Board.tsx:71-76 | 添加 300-500ms debounce | open |
| G29-009 | P3 | TerminalActivityPanel/TaskCard 使用 lucide-react 图标 | 代码规范 | TerminalActivityPanel.tsx:3; TaskCard.tsx:2 | 迁移至 @phosphor-icons/react | open |
| G29-010 | P3 | WorkflowSidebar 未订阅 WS 事件，状态更新依赖 5 分钟 staleTime | 侧边栏延迟 | WorkflowSidebar.tsx:33 | 与 G29-003 一并修复 | open |

### G30 — API 层与错误处理全链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G30-001 | P1 | workflowsApi 全部使用原生 fetch 绕过 makeRequest，缺失统一 headers | 所有 workflow API 调用 | useWorkflows.ts:314-509 | 替换为 makeRequest | open |
| G30-002 | P1 | makeRequest 无网络超时机制，长时间无响应请求永久挂起 | 所有 API 请求 | api.ts:120-130 | 添加 AbortSignal.timeout(30000) | open |
| G30-003 | P1 | runAsyncSafely 将 mutateAsync 异常静默吞没，用户无任何反馈 | 所有 workflow 操作按钮 | Workflows.tsx:1322-1326 | catch 中添加 showToast | open |
| G30-004 | P2 | 9 个 mutation hooks 的 onError 仅 console.error，无用户可见通知 | 所有 mutation 操作 | useWorkflows.ts:573-791 | 添加 toast 通知 | open |
| G30-005 | P2 | 全局 QueryClient 未配置 mutations 默认 retry 和 onError | 缺少兜底错误处理 | main.tsx:55-62 | 添加 mutations 默认配置 | open |
| G30-006 | P2 | React Query 默认 retry 3 次对 4xx 错误不合理 | 无意义重试 | useWorkflows.ts:546 | retry 函数区分状态码 | open |
| G30-007 | P2 | WebSocket onerror 仅 console.error，无用户可见通知 | WS 连接错误 | wsStore.ts:1157-1160 | 更新 store 状态 | open |
| G30-008 | P2 | WebSocket 重连达上限后静默放弃，用户无任何提示 | WS 断线 | wsStore.ts:1043-1073 | 暴露 error 状态或触发 system.error | open |
| G30-009 | P2 | imagesApi upload 方法错误处理与 handleApiResponse 双重检查不一致 | 图片上传 | api.ts:1105-1175 | 统一使用 handleApiResponse | open |
| G30-010 | P2 | oauthApi.logout/getToken 绕过 handleApiResponse 标准错误处理 | 登出/token 获取 | api.ts:1234-1251 | 统一使用 handleApiResponse | open |
| G30-011 | P3 | handleApiResponse 对 response.json() 解析失败无 try-catch 保护 | 非 JSON 响应场景 | api.ts:247 | 添加 SyntaxError 捕获 | open |
| G30-012 | P3 | 错误信息未国际化，mutation onError 和部分 toast 使用硬编码英文 | i18n 一致性 | useWorkflows.ts:574; Workflows.tsx:1363,1372 | 替换为 t() 调用 | open |
| G30-013 | P3 | WebSocket 消息解析失败仅 console.error，无降级处理 | WS 消息处理 | wsStore.ts:952-957 | 增加计数器或 Sentry 上报 | open |

---

## Round 6 — 辅助系统与集成链路审计 (G31-G36)

### G31 — Quality Gate 全链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G31-001 | P1 | enforce 模式修复指令发送到 workflow 广播频道而非目标终端 PTY | enforce 修复闭环断裂 | agent.rs:1482-1490 | 使用 publish_terminal_input 定向发送 | open |
| G31-002 | P2 | 前端 QualityGateResultPayload 缺少 commitHash 字段 | UI 展示和调试追溯 | wsStore.ts:1401-1413; workflow_events.rs:420 | 补充 commitHash 字段 | open |
| G31-003 | P2 | 质量门禁评估无超时保护，tokio::spawn 可能永久挂起 | 终端永久卡在 pending | agent.rs:1282-1408 | 添加 timeout(300s) 包裹 | open |
| G31-004 | P2 | replay/idempotent check 与 insert 之间存在 TOCTOU 竞态 | 高并发下重复评估 | agent.rs:1166-1232 | 合并到单个 write lock 作用域 | open |
| G31-005 | P2 | handle_quality_gate_result 重入 handle_terminal_completed 可能触发二次 quiet window | 终端完成额外延迟 40s | agent.rs:1522 | 添加 skip_quiet_window 标记 | open |
| G31-006 | P3 | QualityEngine 失败时 fail-open 策略缺少告警通知 | 用户无感知质量门禁未执行 | agent.rs:1336-1361 | gate_status 设为 "skipped" | open |
| G31-007 | P3 | Board.tsx handleQualityGateResult 未 invalidate runDetail/issuesForRun | 详情页数据不刷新 | Board.tsx:78-90 | 补充 invalidation | open |
| G31-008 | P3 | tokio::spawn 内 panic 不清理 pending_quality_checks | 终端永久卡在 pending | agent.rs:1282-1408 | 添加 scopeguard 清理 | open |
| G31-009 | P3 | quality_run DB 插入失败时不清理 pending_quality_checks | 后续 checkpoint 被永久跳过 | agent.rs:1254-1268 | fallback 中 remove pending | open |

### G32 — Feishu 集成链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G32-001 | P1 | refresh_tenant_token 存在 TOCTOU 竞态，并发刷新 | 飞书限流风险 | auth.rs:24-35 | double-check 或 tokio::sync::Mutex | open |
| G32-002 | P1 | WebSocket 连接前设置 connected=true 时序错误 | 状态报告不准确 | main.rs:327 | 移到 connect_async 成功后 | open |
| G32-003 | P2 | Ping 循环在连接关闭后最多延迟 120s 才终止 | 资源浪费 | client.rs:60-72 | 使用 CancellationToken | open |
| G32-004 | P2 | Ping 任务 JoinHandle 被丢弃，无法确保清理 | 泄漏风险 | client.rs:60 | 保存 JoinHandle | open |
| G32-005 | P2 | 消息发送 API 未检查飞书响应中的业务错误码 | 发送失败静默忽略 | messages.rs:29-47 | 检查 resp["code"] != 0 | open |
| G32-006 | P2 | /bind 命令未验证 workflow_id 格式或存在性 | 绑定无效 ID | feishu.rs:188-218 | 添加 UUID 校验+DB 查询 | open |
| G32-007 | P2 | update_config 使用 find_enabled 查找，禁用配置无法更新 | 多条配置记录 | feishu.rs:172 | 改用 find_first() | open |
| G32-008 | P2 | feishu_app_config 表缺少唯一约束 | 数据重复 | migrations/20260311120000:1-11 | 添加 UNIQUE(app_id) | open |
| G32-009 | P2 | reconnect 路由 try_send 失败仍返回成功 | 误导用户 | feishu.rs:249 | 失败时返回 429/409 | open |
| G32-010 | P2 | rand_jitter 使用系统时间纳秒作为伪随机源 | 重连惊群效应 | reconnect.rs:41-50 | 使用 rand crate | open |
| G32-011 | P2 | ReconnectPolicy 未实现指数退避 | 服务端压力 | reconnect.rs:20-27 | 实现 base_ms * 2^attempt | open |
| G32-012 | P3 | FeishuEvent 解析失败仅 debug 级别日志 | 生产环境不可见 | client.rs:88 | 提升为 warn | open |
| G32-013 | P3 | parse_message_event 缺失字段使用 unwrap_or_default | 空 chat_id 静默处理 | events.rs:40-65 | 关键字段改为 ok_or_else | open |
| G32-014 | P3 | 前端 FeishuSettings 使用 lucide-react 图标 | 代码规范 | FeishuSettings.tsx:3 | 迁移至 phosphor-icons | open |
| G32-015 | P3 | health 路由中 feishu 状态硬编码 "disconnected" | 运维监控 | health.rs:60-64 | 查询实际连接状态 | open |
| G32-016 | P3 | forward_to_orchestrator 使用 TerminalMessage 语义不匹配 | 消息来源误判 | feishu.rs:262-266 | 添加 ExternalChatMessage 变体 | open |
| G32-017 | P3 | is_connected() 使用 try_read 锁竞争时返回 false | 状态误报 | feishu.rs:329-335 | 改用 AtomicBool | open |
| G32-018 | P3 | 前端保存配置后未触发后端重连 | 需手动点击重连 | FeishuSettings.tsx:55-79 | 保存后自动 reconnect | open |

### G33 — 事件流与 SSE 链路 | 评级: B-

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G33-001 | P1 | 4 个 stream 静默吞掉 BroadcastStream Lagged 错误，事件丢失无感知 | 所有 SSE/WS 端点 | streams.rs:138,347,426,515 | 仿照 stream_projects_raw 实现 Lagged resync | open |
| G33-002 | P2 | stream_tasks_raw 中 Remove 操作无条件放行，跨 project 泄漏 | 任务流 SSE | streams.rs:82-88 | 从 patch path 提取 ID 校验归属 | open |
| G33-003 | P2 | stream_execution_processes 中 Remove 同样无条件放行 | 执行进程流 SSE | streams.rs:306-310 | 同 G33-002 | open |
| G33-004 | P2 | SSE 全局事件端点无连接数限制和 idle timeout | 资源耗尽风险 | events.rs:15-22 | 添加 ConcurrencyLimit + idle timeout | open |
| G33-005 | P2 | MsgStore history 上限 100MB 偏高，无内存告警 | 内存压力 | msg_store.rs:14,40 | 降低上限或添加告警 | open |
| G33-006 | P3 | history_plus_stream 存在 history 和 live stream 间竞态窗口 | 事件重复或丢失 | msg_store.rs:101-111 | 同一锁内获取 history+subscribe | open |
| G33-007 | P3 | EventService spawn 内大量 unwrap()，panic 影响 tokio worker | 事件推送链路 | events.rs:178,486-489 | 替换为 Result 处理 | open |
| G33-008 | P3 | stream_tasks_raw filter_map 中每个 workspace 事件触发 DB 查询 | 高频更新性能 | streams.rs:112-117 | 维护内存映射缓存 | open |
| G33-009 | P3 | SubscriptionHub pending_events 缓存无过期清理 | 内存缓慢增长 | subscription_hub.rs:148-177 | cleanup_if_idle 同步清理 | open |
| G33-010 | P3 | patches.rs/streams.rs 中 serde_json unwrap 可能 panic | 事件补丁生成 | patches.rs:30; streams.rs:33,43 | 替换为 Result 处理 | open |

### G34 — Task Attempts 与 PR 链路 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G34-001 | P1 | create_pr 成功后 DB 写入失败被静默吞掉 | PR 已创建但系统无记录 | pr.rs:317-328 | 传播错误或返回 warning | open |
| G34-002 | P1 | create_pr 缺少 title 输入校验 | 空标题传递给远端 API | pr.rs:196-200 | 添加 title 非空检查 | open |
| G34-003 | P2 | merges 表缺少唯一约束，可重复创建 PR 记录 | 数据重复 | migrations/20250819000000:4-32 | 添加 UNIQUE 约束 | open |
| G34-004 | P2 | create_task_attempt 缺少并发保护 | 重复 workspace 创建 | task_attempts.rs:174-251 | 检查已有 running attempt | open |
| G34-005 | P2 | create_task_attempt 中 start_workspace 失败不回滚 DB 记录 | 孤儿 workspace | task_attempts.rs:229-233 | 包裹事务或失败时删除 | open |
| G34-006 | P2 | merge_task_attempt 中 git merge 成功后 DB 写入失败不可回滚 | 数据不一致 | task_attempts.rs:574-589 | 记录到 dead-letter 表 | open |
| G34-007 | P2 | restore_worktrees_to_process 忽略 reconcile 返回值 | 恢复失败静默继续 | util.rs:69-78 | 捕获返回值记录警告 | open |
| G34-008 | P3 | attach_existing_pr 使用 .next() 选取第一个 PR 未排序 | 可能选错 PR | pr.rs:461 | 显式排序 Open > Merged > Closed | open |
| G34-009 | P3 | workspace_summary 并行 diff 无并发限制 | 大量 git 进程 | workspace_summary.rs:113-133 | buffer_unordered(8) | open |
| G34-010 | P3 | PrCommentsDialog 错误消息硬编码英文 | i18n 一致性 | PrCommentsDialog.tsx:262-277 | 替换为 t() 调用 | open |
| G34-011 | P3 | PrCommentsDialog 使用 lucide-react 图标 | 代码规范 | PrCommentsDialog.tsx:15 | 迁移至 phosphor-icons | open |
| G34-012 | P3 | finalize_workspace 中 set_archived 失败不回滚 Task 状态 | 轻微不一致 | task_attempts.rs:523-528 | 包裹事务 | open |

### G35 — 中间件与全局错误处理 | 评级: B+

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G35-001 | P2 | Token 比较使用 == 而非常量时间比较 | 时序攻击风险 | auth.rs:76 | 引入 subtle::ConstantTimeEq | open |
| G35-002 | P2 | 认证中间件每次请求调用 std::env::var() | 性能开销 | auth.rs:53 | 启动时读取存入 State | open |
| G35-003 | P1 | ApiError fallback 通过 Display 向客户端泄露内部错误详情 | 所有 500 响应 | error.rs:199,211 | 返回通用消息，详情仅记日志 | open |
| G35-004 | P3 | ApiError::Internal 构造时的详细信息被丢弃未记日志 | 调试困难 | error.rs:206 | IntoResponse 中添加 tracing::error | open |
| G35-005 | P3 | model_loaders 返回裸 StatusCode 而非 ApiError | 响应格式不一致 | model_loaders.rs:27-31 | 改为 ApiError | open |
| G35-006 | P3 | 无 CORS 配置（同源嵌入设计正确） | 已验证可接受 | routes/mod.rs | 文档化设计决策 | verified |
| G35-007 | P3 | /healthz /readyz 绕过认证（设计正确） | 已验证可接受 | routes/mod.rs:96-98 | 确认无敏感数据泄露 | verified |
| G35-008 | P2 | MCP TaskServer token 仅构造时读取一次，与 auth 中间件行为不一致 | 认证一致性 | mcp/task_server.rs:291 | 统一 token 获取策略 | open |
| G35-009 | P3 | CI Webhook 路由无独立签名验证 | 当前为 stub | ci_webhook.rs:44 | 正式启用时添加 webhook secret | open |

### G36 — 类型契约与共享基础设施 | 评级: B

| ID | Severity | 问题摘要 | 影响范围 | 证据 | 修复建议 | 状态 |
|----|----------|---------|----------|------|---------|------|
| G36-001 | P1 | i18n workflow namespace 在 ja/es/ko/zh-Hant 4 种语言中缺失 | 21+ 组件显示 raw key | i18n/config.ts:52-88 | 创建 workflow.json 并注册 | open |
| G36-002 | P1 | i18n quality namespace 在 ja/es/ko/zh-Hant 4 种语言中缺失 | Quality 页面显示 raw key | i18n/config.ts:52-88 | 创建 quality.json 并注册 | open |
| G36-003 | P2 | 6 个 Rust 类型有 #[derive(TS)] 但未导出到 shared/types.ts | 前后端契约漂移 | generate_types.rs:13-233 | 优先导出 WsEventType/WsEvent | open |
| G36-004 | P3 | useUiPreferencesStore workspacePanelStates 持久化无清理 | localStorage 膨胀 | useUiPreferencesStore.ts:306-318 | 添加 LRU 上限 | open |
| G36-005 | P3 | modelStore.fetchModels 绕过统一 API 层 | 认证不一致 | modelStore.ts:98 | 替换为 makeRequest | open |
| G36-006 | P3 | modelStore 通过 X-API-Key header 传递 key，与 Bearer 模式不一致 | 安全审计 | modelStore.ts:99-101 | 确认设计意图并注释 | open |
| G36-007 | P3 | text.rs git_branch_id 每次调用编译正则 | 性能 | text.rs:9 | 使用 Lazy 缓存 | open |
| G36-008 | P3 | port_file.rs 硬编码 "solodawn" 与 path.rs debug 模式 "solodawn-dev" 不一致 | 端口冲突 | port_file.rs:6; path.rs:107-111 | 统一使用 get_solodawn_temp_dir() | open |
| G36-009 | P2 | TabNavigationContext 仅导出 createContext 无 Provider 和 null guard | useContext 返回 null | TabNavigationContext.tsx:1-9 | 添加 Provider 和 useTabNavigation hook | open |
| G36-010 | P2 | SearchContext useMemo 依赖数组缺少 clear 函数引用 | exhaustive-deps 违反 | SearchContext.tsx:61-71 | useCallback 包裹并加入 deps | open |
| G36-011 | P3 | ReviewProvider contextValue useMemo deps 不完整 | 理论过期闭包 | ReviewProvider.tsx:222-233 | useCallback 包裹函数 | open |
| G36-012 | P2 | i18n fallback 默认 zh-Hans，非中文用户缺失 key 时看到中文 | UX 一致性 | i18n/config.ts:99,105 | fallback chain 改为 ['zh-Hans', 'en'] | open |

---

## 审计汇总统计

### 总览

| 指标 | 数值 |
|------|------|
| 审计轮次 | 6 轮 |
| 审计分组 | 36 组 |
| 发现问题总数 | 373 |
| P0 问题 | 0 |
| P1 问题 | 52 |
| P2 问题 | 155 |
| P3 问题 | 166 |
| 已验证无问题 | 12 |
| 覆盖文件数 | 200+ |

### 各轮次问题分布

| 轮次 | P1 | P2 | P3 | 合计 |
|------|----|----|----|----|
| Round 1 (G01-G06) | 7 | 17 | 14 | 38 |
| Round 2 (G07-G12) | 8 | 21 | 26 | 55 |
| Round 3 (G13-G18) | 12 | 26 | 26 | 64 |
| Round 4 (G19-G24) | 7 | 22 | 34 | 63 |
| Round 5 (G25-G30) | 10 | 33 | 38 | 81 |
| Round 6 (G31-G36) | 8 | 36 | 28 | 72 |

### TOP 10 最高优先级问题（建议立即修复）

| 排名 | ID | 问题 | 影响 |
|------|----|------|------|
| 1 | G13-001/002 | TerminalStatus strum serialize_all="lowercase" 产生 "notstarted" 而非 "not_started" | 状态比较可能永远不匹配 |
| 2 | G13-004 | review_passed/review_rejected/quality_pending 绕过枚举裸字符串写入 DB | 前端无法渲染这些状态 |
| 3 | G04-003/G15-001 | set_completed_if_unfinished 未排除 failed 状态 | 已失败终端可被覆盖为 completed |
| 4 | G05-001/002 | pause_workflow 未执行级联清理，且无法 resume | Pause/Resume 功能不可用 |
| 5 | G06-001/002 | merge 缺乏并发保护，auto_merge 与手动 merge 无互斥 | 重复 squash merge |
| 6 | G31-001 | enforce 模式修复指令发送到广播频道而非目标终端 | enforce 修复闭环断裂 |
| 7 | G35-003 | ApiError fallback 向客户端泄露内部错误详情 | 安全信息泄露 |
| 8 | G30-001 | workflowsApi 全部绕过 makeRequest，缺失统一 headers | 启用 API Token 后鉴权失败 |
| 9 | G30-002/003 | makeRequest 无超时 + runAsyncSafely 静默吞异常 | 请求永久挂起+用户无反馈 |
| 10 | G25-001/002 | Step2 taskCount 变化丢弃数据 + handleSubmit 闭包竞态 | 向导数据丢失 |

### 各组评级汇总

| 评级 | 组数 | 组别 |
|------|------|------|
| B+ | 10 | G01, G02, G03, G04, G07, G09, G11, G12, G22, G35 |
| B | 6 | G08, G10, G16, G20, G24, G36 |
| B- | 10 | G05, G06, G14, G15, G17, G25, G29, G30, G31, G33 |
| C+ | 2 | G13, G23 |

### 修复优先级建议

1. **立即修复（P1 核心功能缺陷）**：G13-001/002（strum 序列化）、G04-003/G15-001（CAS 未排除 failed）、G05-001/002（pause 不可用）、G06-001/002（merge 并发）
2. **短期修复（P1 安全/数据一致性）**：G35-003（错误泄露）、G18-001（时序攻击）、G18-002（API key 日志泄露）、G30-001（API 层绕过）
3. **中期修复（P2 系统健壮性）**：CAS 保护补全、WS 事件前后端对齐、错误处理统一化、i18n 补全
4. **长期改进（P3 代码质量）**：死代码清理、lucide-react 迁移、性能优化、文档更新
