# GitCortex 全量审计修复 — 当前状态报告

> 更新时间: 2026-03-14 (verified)
> 分支: main
> CI 状态: ✅ 全绿（最近 3 次运行均 success）

---

## 一、总体进度

| 阶段 | 状态 | 说明 |
|------|------|------|
| 全量代码审计 (36组) | ✅ 完成 | 373 个问题，覆盖 200+ 文件 |
| Batch 1-2: P1 核心修复 | ✅ 完成 | 15 个 P1 问题，CI 全绿 |
| Batch 3: agent.rs 独占修复 | ✅ 完成 | ~30 个问题 |
| Batch 4: workflows.rs + runtime.rs | ✅ 完成 | ~25 个问题 |
| Batch 5: 后端辅助模块 | ✅ 完成 | ~35 个问题 |
| Batch 6: 后端安全/WS/DTO | ✅ 完成 | ~30 个问题 |
| Batch 7: DB 模型 + constants | ✅ 完成 | ~15 个问题 |
| Batch 8: 前端核心 3 大文件 | ⚠️ 部分完成 | 23项中 1 FIXED / 3 PARTIALLY / 19 NOT FIXED |
| Batch 9: 前端组件 | ⚠️ 部分完成 | 25项中 2 FIXED / 1 PARTIALLY / 22 NOT FIXED |
| Batch 10: 辅助+集成 | ⏳ 待完成 | Feishu/Quality/i18n/TaskAttempts/api.ts 大部分未修复 |
| CI 验证 | ✅ 已通过 | 2 个测试失败已修复，CI 全绿 |

---

## 二、已完成的修改（44 个文件，~1800 行变更）

### 后端 Rust（30 个文件）
| 文件 | 修复内容 |
|------|---------|
| `db/models/terminal.rs` | strum snake_case、新增 ReviewPassed/ReviewRejected/QualityPending 枚举、CAS set_starting/set_waiting、set_completed_if_unfinished 排除 failed、completed_at 自动设置 |
| `db/models/workflow.rs` | CAS set_ready(AND status='starting')、set_started 允许 paused、set_merging/set_merge_completed CAS、doc 注释修复 |
| `orchestrator/agent.rs` | 25+ let _ = 改 warn 日志、auto_sync 排除 merging、TASK_HINT 正则修复、defer 状态检查、FailWorkflow 增加 StatusUpdate、dead code #[allow] |
| `orchestrator/constants.rs` | 补全 9 个 workflow + 6 个 task 状态常量、删除不匹配的 PENDING/RUNNING |
| `orchestrator/runtime.rs` | stop 超时文档、recovery 文档、RAII 文档、时序窗口文档 |
| `orchestrator/state.rs` | processed_commits 改为 bounded set (10000 上限) |
| `orchestrator/persistence.rs` | save_task_progress 标记预留接口 |
| `git_watcher.rs` | 分隔符 \| 改 \x1e、git 命令合并、重试计数器、双路径统一 |
| `git/cli.rs` | merge_squash_commit 冲突检测、worktree_add 参数修正 |
| `terminal/launcher.rs` | workflow_id 解析 warn 日志、broadcast 跳过 warn |
| `terminal/output_fanout.rs` | replay capacity 2x broadcast |
| `terminal/process.rs` | flush_buffer TOCTOU 修复(mem::take)、buffer 上限、CODEX_HOME 重复解析 |
| `terminal_ws.rs` | Lagged 恢复、心跳超时检测 |
| `worktree_manager.rs` | 锁清理、remove_dir_all 降级 warn |
| `merge_coordinator.rs` | broadcast_merge_success 参数化 |
| `subscription_hub.rs` | cleanup_if_idle 清理 pending_events |
| `workflow_events.rs` | dead code #[allow]、双格式 TODO |
| `workflow_ws.rs` | select! 注释 |
| `workflows_dto.rs` | Option 向后兼容注释 |
| `terminals.rs` | NotFound 描述性消息、limit clamp |
| `model_loaders.rs` | 返回 ApiError 替代裸 StatusCode |
| `error.rs` | 500 不泄露内部错误、tracing::error 记录 |
| `auth.rs` | 常量时间 token 比较、JSON 401 响应 |

### 前端 TypeScript（14 个文件）
| 文件 | 修复内容 |
|------|---------|
| `useWorkflows.ts` | workflowsApi 迁移到 makeRequest、draft.canPrepare=false、merging.canStop/canMerge=false |
| `useWorkflows.test.tsx` | 测试适配 makeRequest |
| `api.ts` | makeRequest 添加 30s AbortSignal.timeout |
| `Workflows.tsx` | runAsyncSafely 添加 toast、WS 事件对齐 |
| `wsStore.ts` | 添加 provider 事件类型、cancelled→unknown 映射 |
| `WorkflowWizard.tsx` | handleSubmit 函数式 setState、mode 切换自动初始化 |
| `Step2Tasks.tsx` | 增量调整任务数组、configRef 修复 ESLint |
| `Board.tsx` | forProject invalidation、debounce、prompt 事件、quality invalidation |
| `TaskCard.tsx` | 传递 terminals 数组给 TerminalDots |
| `TerminalDots.tsx` | 移除 running/pending、添加 review/quality 状态 |
| `WorkflowKanbanBoard.tsx` | 拖拽校验注释 |
| `WorkflowSidebar.tsx` | staleTime 设计注释 |
| `workflowStatus.ts` | 清理幽灵状态、补充 review/quality/checkpoint |

---

## 三、CI 状态

✅ CI 全绿。最近 3 次运行均为 success（2026-03-14）。此前的 2 个 Rust 测试失败已修复：
- `test_handle_git_event_no_metadata_marks_failed_when_task_cannot_be_inferred`
- `test_handle_git_event_review_pass_publishes_terminal_status_update`

---

## 四、待完成的修复（按优先级）

### 优先级 1：CI 修复（~~阻塞交付~~ ✅ 已完成）
- [x] 修复 2 个测试失败
- [x] 推送并确认 CI 全绿

### 优先级 2：前端核心文件剩余问题（Batch 8 未完成部分）
- [ ] `Workflows.tsx`: G07-004/007/011, G08-003/006, G26-004-009, G27-003-008
- [ ] `useWorkflows.ts`: G02-004/007, G05-009, G26-003/006/012, G30-004/006
- [ ] `wsStore.ts`: G08-007, G12-001/003/009, G27-001, G30-007/008

### 优先级 3：前端组件（Batch 9 未完成部分）
- [ ] `WorkflowDebugPage.tsx`: G28-001/003/008 (mapTerminalStatus 修复)
- [ ] `TerminalDebugView.tsx`: G09-002/012, G28-002/006/007/009
- [ ] `TerminalEmulator.tsx`: G28-005/011, G09-009
- [ ] `DisplayConversationEntry.tsx`: G09-011 (lucide→phosphor 迁移)
- [ ] `OrchestratorChatPanel.tsx`: G28-004 (轮询→WS TODO)
- [ ] Wizard 验证器: G25-003/004/005/006/009
- [ ] Wizard 步骤: G25-007/008/010/013/014/016/018

### 优先级 4：辅助模块与集成（Batch 10）
- [ ] Feishu: G32-001 到 G32-018
- [ ] Quality Gate: G31-001 到 G31-009
- [ ] Events/SSE: G33-001 到 G33-010
- [ ] Task Attempts: G34-001 到 G34-012
- [ ] api.ts: G30-005/009/010/011
- [ ] contexts: G36-009/010
- [ ] i18n: G36-001/002/012
- [ ] generate_types.rs: G17-001, G36-003
- [ ] utils: G36-007/008
- [ ] CLAUDE.md: G08-004

---

## 五、统计摘要

| 指标 | 数值 |
|------|------|
| 审计发现总数 | 373 |
| 已修复（代码已提交） | ~183 |
| 待修复（代码未写） | ~190 |
| 已修改文件数 | 44 |
| 新增/修改行数 | ~1800 |
| CI 状态 | ✅ 全绿 |
