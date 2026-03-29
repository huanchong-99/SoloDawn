# Concierge Agent 开发进度

## 当前状态 (2026-03-22)

### 已完成
- [x] 数据库表：concierge_session, concierge_session_channel, concierge_message
- [x] Concierge Agent 服务：LLM tool-calling 循环、工具定义、系统提示词、消息广播
- [x] API 端点：REST (sessions, messages, channels) + WebSocket
- [x] 飞书集成改造：消息路由到 Concierge，模型选择流程
- [x] Web UI：对话视图、侧边栏集成
- [x] 飞书 ↔ Web 双向同步
- [x] 模型选择：1个直接用，多个追问
- [x] 工具调用：create_project, list_cli_types, create_workflow, prepare/start workflow
- [x] prepare_workflow/start_workflow 实际调用 HTTP API（非占位）
- [x] send_to_orchestrator 实际调用 orchestrator chat API
- [x] AnthropicCompatibleClient（支持 Anthropic 格式 API）
- [x] build_agent_planned_context 只发送有 API key 的模型
- [x] cc_switch fallback 链增加 model_config 凭据查找
- [x] config.json 模型同步到 model_config DB 表
- [x] 侧边栏"活跃"列表中显示 Concierge 对话（与工作区并列）
- [x] pipeline 页面链接（running 徽标可点击）
- [x] 对话头部"查看工作流进度"按钮

### 待修复（下一轮）
1. **终端 invalid identifier 错误**
   - Debug 页面显示 "Disconnected: invalid terminal identifier."
   - 编排 Agent 创建的终端 ID 格式可能与 terminal WebSocket 路由不兼容
   - 需要检查 terminal ID 格式（如 `term-infra-1`）是否匹配 WebSocket 路由验证

2. **Concierge 对话页面右侧边栏未关联**
   - 打开 `/workspaces/create?conciergeId=xxx` 时右侧仍显示"未选择"状态
   - 应该自动关联当前 session 的 active_workflow_id，显示项目/仓库信息

3. **Concierge 对话页面不显示终端工作进度**
   - 对话视图只显示 concierge_message
   - 应该也嵌入 workflow 的实时进度（任务状态、终端输出等）

4. **companion workspace 记录可能不被旧系统的 streaming 接口识别**
   - create_workflow 时创建了 Task + Workspace 记录
   - 但旧系统的 WebSocket streaming 依赖 session + execution_process 链路
   - 当前改为直接在侧边栏渲染 concierge sessions，不走旧的 workspace streaming

### 关键文件清单
- `crates/db/src/models/concierge.rs` — DB 模型
- `crates/db/migrations/20260322200000_create_concierge_tables.sql`
- `crates/db/migrations/20260322210000_fix_concierge_session_fk.sql`
- `crates/services/src/services/concierge/` — agent, tools, sync, notifications, prompt
- `crates/services/src/services/feishu.rs` — 飞书路由到 Concierge
- `crates/services/src/services/orchestrator/agent.rs` — build_agent_planned_context 过滤
- `crates/services/src/services/orchestrator/llm.rs` — AnthropicCompatibleClient
- `crates/services/src/services/cc_switch.rs` — model_config 凭据查找
- `crates/server/src/routes/concierge.rs` + `concierge_ws.rs`
- `crates/server/src/main.rs` — Concierge 初始化
- `frontend/src/components/ui-new/containers/ConciergeChatContainer.tsx`
- `frontend/src/components/ui-new/views/ConciergeChatView.tsx`
- `frontend/src/components/ui-new/views/WorkspacesSidebar.tsx` — 侧边栏集成
- `frontend/src/components/ui-new/containers/WorkspacesLayout.tsx` — conciergeId 路由
- `frontend/src/stores/conciergeWsStore.ts`
- `frontend/src/hooks/useConcierge.ts`
- `frontend/src/lib/conciergeApi.ts`
- `frontend/src/pages/ui-new/Assistant.tsx`

### 当前运行的 workflow
- Workflow ID: 03ef3a4d-055f-4123-a1d1-18e192091edc
- Session ID: 116fc5df-388e-46f2-81f7-85da5104b655
- 终端报错: "Disconnected: invalid terminal identifier."
