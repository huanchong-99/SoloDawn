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

## 当前状态

- 以 `docs/undeveloped/current/TODO-pending.md` 为准
- 已完成：44
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

## 架构摘要

- `OrchestratorAgent`：编排决策与调度核心。
- `OrchestratorRuntime`：工作流生命周期推进。
- `MessageBus`：跨模块/终端事件路由。
- `TerminalLauncher`：终端进程生命周期管理。
- `GitWatcher`：基于 Git 事件驱动编排推进。

主要代码位置：

- `crates/services/src/services/orchestrator/`
- `crates/server/src/routes/workflows.rs`
- `frontend/src/pages/Workflows.tsx`

## 文档

- 进度看板：`docs/undeveloped/current/TODO.md`
- Docker 部署：`docs/developed/ops/docker-deployment.md`
- 运维手册：`docs/developed/ops/runbook.md`
- 故障排查：`docs/developed/ops/troubleshooting.md`

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
