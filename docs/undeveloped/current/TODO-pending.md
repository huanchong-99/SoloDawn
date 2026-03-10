# GitCortex 当前任务清单（按状态划分）

> 更新时间：2026-03-11
> 说明：本文档作为当前阶段执行看板，所有条目按"已完成 / 未完成"维护。

## 已完成

暂无（Phase 28 刚启动）

## 未完成

### Phase 28A: 信息流补全

- [ ] PHASE28A-001 终端完成上下文采集器（terminal_log 摘要 + diff stat + commit body 注入 completion prompt）
- [ ] PHASE28A-002 注入上下文到 LLM Completion Prompt（依赖 28A-001）
- [ ] PHASE28A-003 跨终端上下文传递 Handoff Notes（前序终端 commit + role + handoff 注入下一终端指令）

### Phase 28B: 闭环补全

- [ ] PHASE28B-001 Workflow 完成后自动合并（auto_sync_workflow_completion 触发 merge）
- [ ] PHASE28B-002 启用 ReviewCode/FixIssues/MergeBranch 指令（白名单 + 执行逻辑 + review pass/reject 自动推进）
- [ ] PHASE28B-003 连接 Error Handler 到 Agent（handle_git_terminal_failed 委托 error_handler）

### Phase 28C: 韧性补全

- [ ] PHASE28C-001 Agent 事件循环容错（call_llm_safe + error_count + 降级处理）
- [ ] PHASE28C-002 状态持久化激活（maybe_save_state 防抖 5s + 关键检查点调用）
- [ ] PHASE28C-003 崩溃恢复实现（recover_running_workflows 恢复 agent + restore_state）
- [ ] PHASE28C-004 Planning Draft 接入 LLM 对话（send_message 调用 LLM + WorkspacePlanning prompt）

### Phase 28D: 飞书长连接接入

- [ ] PHASE28D-001 飞书连接器 Crate（WebSocket 长连接 + protobuf 帧 + auth + reconnect）
- [ ] PHASE28D-002 飞书服务集成（FeishuService + 消息路由 + /bind /unbind）
- [ ] PHASE28D-003 ChatConnector Trait 抽象（统一 Telegram + Feishu 出站接口）
- [ ] PHASE28D-004 数据库与配置（feishu_app_config 表 + 环境变量）
- [ ] PHASE28D-005 Server 集成（启动时连接 + 管理 API + 健康检查）

### Phase 28E: 智能熔断与提供商轮转

- [ ] PHASE28E-001 ResilientLLMClient 实现（多提供商 + 5 次熔断 + 60s 探活 + round-robin）
- [ ] PHASE28E-002 终端级提供商故障转移（自动拉起替代终端 + 替代 CLI/模型选择）
- [ ] PHASE28E-003 提供商健康监控 API（状态查询 + 手动 reset + WebSocket 事件）

### Backlog（低优先级保留项）

- [ ] BACKLOG-001 DockerDeployment 抽象（优先级：低）
- [ ] BACKLOG-002 Runner 容器分离（优先级：低）
- [ ] BACKLOG-003 CLI 安装状态 API（优先级：中）
- [ ] BACKLOG-004 K8s 部署支持（优先级：低）
- [ ] BACKLOG-005 镜像体积优化（优先级：中）

### 当前结论

- Phase 28 共 18 个任务（S:4, M:7, L:5, XL:1）+ 5 个 Backlog 保留项
- 计划文档：docs/developed/plans/2026-03-11-phase-28-orchestrator-evolution.md
