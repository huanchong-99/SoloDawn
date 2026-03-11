<p align="center">
  <a href="README.md">English</a>
</p>

# GitCortex

面向多 AI CLI 协作开发的上层编排系统（Claude Code、Codex、Gemini CLI 等）。

## 项目定位

- 用一个主 Agent 统一调度所有终端，避免多控制器冲突。
- 任务间并行、任务内串行，兼顾效率与质量闸门。
- 基于原生 CLI 执行，原有斜杠命令 / 插件 / MCP / skill 可复用。
- 基于 Git 事件驱动推进，具备可追踪、可恢复、可审计能力。

## 最近更新（2026 年 3 月）

### 编排与对话链路

- 工作流级主 Agent 对话增加分页与查询参数（`cursor/limit`）（`ec8ad4ec2`）。
- 落地 orchestrator 对话消息与命令快照持久化（`8ccf0f3d1`）。
- 落地指令白名单与命令状态流转（`1a1b153a3`）。
- 增加命令恢复、治理控制、审计流（`3a177d5d9`）。
- 新增 Telegram Connector 入站、会话绑定与重放防护（`95c4afc81`）。
- 前端主 Agent 面板消息流和交互覆盖测试增强（`fb642c5fc`）。

### Docker 与安装脚本

- Docker 适配已更新（`679e5cf54`、`7af0e7d17`、`35f17ecda`）。
- Docker/本地运行态下的工作区路径处理更稳定。
- 现有 Docker 部署可走运行态更新流程。
- `.env` 与 API Token 映射流程更清晰。
- 一键安装脚本已更新（`07ef09911`、`35f17ecda`）。
- 支持复用已有 `.env` 并自动切换到更新流程。
- 支持 install/update 模式、语言选择、非交互参数、可选数据卷清理和就绪检查。

### Phase 28：编排层进化（2026 年 3 月）

- 终端完成上下文注入：LLM 决策现在包含终端日志摘要、diff 统计和 commit 内容。
- 跨终端交接备注：前序终端上下文（角色、状态、提交、交接备注）传递给下一终端。
- ReviewCode / FixIssues / MergeBranch 指令现在会创建专用的审查和修复终端，而非仅发布事件。
- Review reject 自动触发修复终端创建；review pass 自动检查工作流完成状态。
- 工作流完成后自动合并，支持冲突检测和状态追踪。
- Error Handler 接入 Agent 事件循环，终端失败自动委托处理。
- LLM 容错：`call_llm_safe` 包装器，连续失败计数 + 优雅降级。
- 状态持久化：5 秒防抖 + 关键检查点保存；崩溃恢复从 DB 重建 Agent 状态。
- Planning Draft 支持多轮 LLM 对话，激活 WorkspacePlanning 提示词配置。
- 飞书长连接 WebSocket 连接器：租户 Token 管理、消息路由、`/bind` `/unbind` 命令。
- ChatConnector Trait 统一抽象 Telegram 和飞书出站消息接口。
- 飞书 Server 集成：通过 `GITCORTEX_FEISHU_ENABLED` 条件启动、管理 API、健康检查扩展。
- ResilientLLMClient：多提供商 round-robin 轮转，5 次熔断 + 60 秒探活恢复。
- 终端级提供商故障转移：自动拉起替代终端，使用备选 CLI/模型配置。
- 提供商健康监控 API：实时熔断器数据、手动重置、WebSocket 事件推送（`provider.switched`、`provider.exhausted`、`provider.recovered`）。

## 当前状态

- 以 `docs/undeveloped/current/TODO-pending.md` 为准
- Phase 28（编排层进化）：18/18 任务已完成
- 累计完成：62
- 未完成：5（均为低/中优先级 backlog）

相关文档：

- `docs/undeveloped/current/TODO.md`
- `docs/undeveloped/current/orchestrator-chat-verification-report.md`
- `docs/undeveloped/current/orchestrator-chat-rollback-runbook.md`

## 快速开始

### 本地开发

```bash
pnpm install

# 必需：32 位字符串
# Windows PowerShell:
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
# Linux/macOS:
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

npm run prepare-db
pnpm run dev
```

默认地址：

- 前端：`http://localhost:23457`
- 后端 API：`http://localhost:23456/api`

### Docker 一键安装

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

安装脚本支持：

- 交互式配置（挂载目录、密钥、端口、是否安装 AI CLI）
- 复用已有 `.env`
- 自动切换到更新流程

### Docker 更新

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest
```

常用参数：

- `-AllowDirty`
- `-PullBaseImages`
- `-SkipBuild`
- `-SkipReadyCheck`

## 验证

```bash
curl http://localhost:23456/readyz
curl http://localhost:23456/api/health
```

若启用 API Token：

```bash
curl http://localhost:23456/api/health -H "Authorization: Bearer <token>"
```

## 数据安全

- **删除项目不会删除本地文件。** 仅移除数据库中的项目元数据和项目-仓库关联记录，磁盘上的仓库文件不受任何影响。
- **解除仓库关联** 仅移除数据库中项目与仓库的绑定关系，不会删除、移动或修改磁盘上的 Git 仓库。
- **项目-仓库绑定**（`defaultAgentWorkingDir`）仅存储引用路径，解绑或重新绑定不会对文件系统产生任何操作。

## 架构摘要

- `OrchestratorAgent`：编排决策与调度核心。
- `OrchestratorRuntime`：工作流生命周期推进。
- `MessageBus`：跨模块/终端事件路由。
- `TerminalLauncher`：终端进程生命周期管理。
- `GitWatcher`：基于 Git 事件驱动编排推进。
- `ResilientLLMClient`：多提供商 LLM 客户端，内置熔断与故障转移。
- `FeishuService`：飞书 WebSocket 连接器，消息路由与斜杠命令。
- `ChatConnector`：跨聊天平台出站消息统一 Trait。

主要代码位置：

- `crates/services/src/services/orchestrator/`
- `crates/server/src/routes/workflows.rs`
- `frontend/src/pages/Workflows.tsx`
- `crates/feishu-connector/`
- `crates/services/src/services/chat_connector.rs`
- `crates/server/src/routes/provider_health.rs`

## 文档

- 进度看板：`docs/undeveloped/current/TODO.md`
- Docker 部署：`docs/developed/ops/docker-deployment.md`
- 运维手册：`docs/developed/ops/runbook.md`
- 故障排查：`docs/developed/ops/troubleshooting.md`
- Phase 28 计划：`docs/undeveloped/current/2026-03-11-phase-28-orchestrator-evolution.md`

## 贡献

- 大改动建议先提 Issue。
- 保持小步提交，便于评审。
- 提交 PR 前建议执行：

```bash
cargo check --workspace
cargo test --workspace
cd frontend && npm run test:run && cd ..
```

## 许可证

- Vibe Kanban 衍生部分：Apache-2.0
- CC-Switch 衍生部分：MIT
- 详见 `LICENSE`
