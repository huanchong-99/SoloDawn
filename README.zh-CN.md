<p align="center">
  <a href="README.md">English</a>
</p>

<p align="center">
  <strong>AI Agent 跨终端任务协调平台</strong>
</p>

<p align="center">
  基于 <a href="https://github.com/BloopAI/vibe-kanban">Vibe Kanban</a> 改造，集成 <a href="https://github.com/farion1231/cc-switch">CC-Switch</a> 模型切换能力
</p>

---

## 概述

GitCortex 是一个 AI 驱动的多终端任务协调平台，让多个 AI 编码代理（Claude Code、Gemini CLI、Codex 等）能够并行协作完成复杂的软件开发任务。

### 核心特性

| 特性 | 说明 |
|------|------|
| **主 Agent 协调** | AI 驱动的中央控制器，负责任务分发、进度监控、结果审核 |
| **多任务并行** | 多个 Task 同时执行，每个 Task 有独立 Git 分支 |
| **任务内串行** | 每个 Task 内的 Terminal 按顺序执行（编码→审核→修复） |
| **cc-switch 集成** | 一键切换任意 CLI 的模型配置 |
| **事件驱动** | 基于 Git 提交与消息总线事件推进工作流，减少不必要轮询与上下文重复 |
| **终端调试视图** | 启动后可进入原生终端验证环境配置 |
| **工作流持久化** | 完整的 Workflow/Task/Terminal 三层数据模型 |
| **斜杠命令系统** | 可复用的提示词预设，支持模板变量替换 |
| **多模型支持** | 支持 Claude、Gemini、OpenAI 等多种 AI 模型 |
| **Git 集成** | 深度集成 Git，自动管理分支和合并 |

### 架构概览

```
╔═══════════════════════════════════════════════════════════════════╗
║                     Orchestrator (主 Agent)                        ║
║           用户配置: API类型 + Base URL + API Key + 模型            ║
╚═══════════════════════════════════════════════════════════════════╝
         │                      │                      │
         ▼                      ▼                      ▼
  ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
  │   Task 1    │       │   Task 2    │       │   Task 3    │
  │ branch:login│       │ branch:i18n │       │ branch:theme│
  │  T1→T2→T3   │       │   TA→TB     │       │   TX→TY     │
  └─────────────┘       └─────────────┘       └─────────────┘
         ║                      ║                      ║
         ╚══════════════════════╩══════════════════════╝
                         任务间并行执行
                              │
                              ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │                   全局合并终端 (Merge Terminal)                  │
  └─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                          [ main ]
```

### 关键协作机制（重点补充）

> 这一节专门说明 GitCortex 最核心的价值：**一个总 Agent（Orchestrator）调度多个 CLI 终端协作完成复杂任务**。

#### 1) 为什么是“一个总 Agent”而不是“多个总 Agent”

在 GitCortex 中，Orchestrator 是唯一的全局调度者，负责统一决策与推进，避免多个主控同时下达指令造成冲突。

它主要做四件事：

1. **任务拆解与分发**：把 workflow 目标分配到不同 task。
2. **终端串行推进**：在每个 task 内按 `orderIndex` 启动下一终端。
3. **状态机收敛**：统一维护 workflow/task/terminal 三层状态。
4. **事件闭环**：消费 Git 事件、Prompt 事件、WS 事件并决定后续动作。

这意味着你看到的“多终端协作”背后不是乱序并发，而是**中心化编排 + 可观测状态机**。

#### 2) 多 CLI 协作模型（横向）

GitCortex 支持把不同 CLI 放进同一个 workflow：

- `claude-code` 负责主开发
- `codex` 负责审计/修复建议
- `gemini-cli` 负责文档或测试补全

它们可以在：

- **任务层并行**（Task A / B / C 同时跑）
- **任务内串行**（Terminal 1 → Terminal 2 → Terminal 3）

实现“并行加速 + 串行把关”的组合策略。

#### 3) 同一种 CLI 的多模型协作（纵向）

GitCortex 不要求“一个 CLI 只能对应一个模型”。

你完全可以在同一个 task 里，使用**同一种 AI CLI + 不同模型**形成角色分工，例如都用 `claude-code`：

| Terminal | CLI | 模型 | 典型角色 |
|---|---|---|---|
| T1 | `claude-code` | `glm-4.7` | 前端实现 |
| T2 | `claude-code` | `claude-opus-4.6` | 后端实现 |
| T3 | `codex` | `gpt-5.3-codex-xhigh` | 代码审计/收敛 |

这样做的价值是：

- 保留同一 CLI 的操作习惯与上下文风格
- 利用不同模型在代码生成、推理深度、审计能力上的差异
- 通过 Orchestrator 保证交接顺序与状态一致性

#### 4) cc-switch 在协作中的作用

`cc-switch` 负责把“终端实例”与“模型配置”解耦，让你在同一 CLI 生态内灵活切模型：

- 启动前写入目标模型配置
- 启动后保持该终端会话的一致模型语义
- 支持不同终端绑定不同模型，不互相污染

因此 GitCortex 支持两类协作：

- **跨 CLI 协作**（Claude + Codex + Gemini）
- **同 CLI 多模型协作**（例如多个 Claude Code 终端各自绑定不同模型）

#### 5) 复杂任务是如何被稳定推进的

在真实开发场景中，一个“复杂任务”通常不是一次生成，而是多轮闭环：

1. 终端 A 先实现主逻辑
2. 终端 B 复核并补测试
3. 终端 C 做审计与风险收敛
4. Merge Terminal 统一合并到目标分支

GitCortex 的重点不是“单次回答质量”，而是让这个闭环过程可重复、可监控、可回放、可恢复。

换句话说，GitCortex 提供的是 **Agent 协作流水线能力**，而不仅是“调用某个模型”。

---

## 技术栈

### 后端

- **语言与运行时**：Rust + Tokio
- **Web 框架**：Axum（REST + WebSocket）
- **数据层**：SQLx + SQLite
- **工程结构**：Rust Workspace（`crates/server`、`crates/services`、`crates/db`、`crates/cc-switch` 等）

### 前端

- **框架**：React 18 + TypeScript
- **构建工具**：Vite
- **状态与数据**：TanStack Query + WebSocket Store
- **终端渲染**：xterm.js（终端调试与输出展示）

### 协作运行时组件（核心）

- `OrchestratorRuntime`：统一调度 workflow 生命周期
- `OrchestratorAgent`：执行编排决策与状态推进
- `MessageBus`：跨终端/跨模块事件总线
- `TerminalCoordinator`：终端准备与串行推进协调
- `TerminalLauncher`：终端进程启动与生命周期管理
- `GitWatcher`：监听 Git 提交并触发事件
- `CCSwitchService`：CLI/模型配置切换与隔离

以上组件对应源码可在 `crates/services/src/services/` 与 `crates/server/src/routes/` 中找到。

---

## 部署指南

### 部署模式

- **开发模式（双服务）**：前端开发服务器 + 后端 API 服务分开运行
  - 前端：`23457`
  - 后端：`23456`
- **生产模式（单服务）**：仅运行后端二进制，后端同时提供 `/api` 与前端静态资源

### 开发模式部署（推荐本地开发）

```bash
pnpm install

# 必需：设置 32 字符加密密钥
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# 按需准备 SQLx 查询缓存
npm run prepare-db

# 启动前后端
pnpm run dev
```

访问地址：

- 前端：`http://localhost:23457`
- 后端：`http://localhost:23456/api`

### 生产模式部署（单机）

```bash
# 1) 安装依赖
pnpm install

# 2) 构建前端（用于后端静态资源嵌入）
cd frontend && pnpm install && pnpm build && cd ..

# 3) 构建后端
cargo build --release -p server

# 4) 设置运行环境变量
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="your-32-character-key"
$env:BACKEND_PORT="23456"   # 可选
$env:HOST="127.0.0.1"       # 可选，外网部署可设 0.0.0.0

# 5) 启动服务
# Windows
.\target\release\server.exe

# Linux/macOS
./target/release/server
```

健康检查：

```bash
# 未启用 GITCORTEX_API_TOKEN 时
curl http://127.0.0.1:23456/api/health

# 启用 GITCORTEX_API_TOKEN 时（所有 /api 路由需 Bearer）
curl http://127.0.0.1:23456/api/health \
  -H "Authorization: Bearer <your-token>"
```

> 更完整的运维、备份、升级、回滚流程，请查看：`docs/developed/ops/runbook.md` 与 `docs/developed/ops/troubleshooting.md`。

---

## 快速开始

### 前置要求

| 工具 | 版本要求 | 说明 |
|------|----------|------|
| **Rust** | nightly-2025-12-04 | 定义在 `rust-toolchain.toml` |
| **Node.js** | >= 18（建议 20） | 前端运行时 |
| **pnpm** | 10.13.1 | 包管理器 |
| **CMake** | 最新版 | 构建工具（某些系统需要） |
| **SQLite** | 3.x | 数据库（通常内置） |

### 安装

#### 1. 安装 Rust 工具链

```bash
# 安装 Rustup
# 下载：https://rustup.rs/ 或使用 winget
winget install Rustlang.Rustup

# 安装项目指定版本
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04

# 安装 Cargo 工具
cargo install cargo-watch
cargo install sqlx-cli --features sqlite

# 验证安装
rustc --version
# 应输出：rustc 1.85.0-nightly (2025-12-04)
```

#### 2. 安装 Node.js 和 pnpm

```bash
# 推荐使用 nvm-windows
# 下载：https://github.com/coreybutler/nvm-windows
nvm install 20
nvm use 20

# 安装指定版本 pnpm
npm install -g pnpm@10.13.1

# 验证安装
pnpm --version
# 应输出：10.13.1
```

#### 3. 克隆并启动项目

```bash
# 克隆仓库
git clone <your-repo-url>
cd GitCortex

# 安装依赖
pnpm install

# 设置环境变量（必需）
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# 生成/校验 SQLx 查询缓存（按需）
npm run prepare-db

# 构建后端（Rust）
cargo build --release

# 启动开发服务器（前后端）
pnpm run dev
```

访问：
- 前端：http://localhost:23457
- 后端 API：http://localhost:23456/api

**详细运维指南：** 查看 [Operations Manual](docs/developed/ops/runbook.md) 了解生产部署、监控、升级等详细操作。

### 从现有仓库恢复

如果你已经克隆过仓库，只需确保工具版本正确并重新安装依赖：

```bash
cd GitCortex

# 检查 Rust 版本
rustc --version
# 如版本不对，运行：
rustup default nightly-2025-12-04

# 重新安装依赖
pnpm install

# 设置环境变量并启动
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
pnpm run dev
```

---

## 开发环境配置

### IDE 推荐

- **VS Code** + 插件：
  - `rust-analyzer`（Rust 语言服务器）
  - `ESLint`（前端检查）
  - `Prettier`（代码格式化）

### 环境变量

创建 `.env` 文件或设置系统环境变量：

```bash
# 必需：加密密钥（32字符字符串）
GITCORTEX_ENCRYPTION_KEY=your-32-character-key-here

# 可选
BACKEND_PORT=23456           # 后端端口（默认）
HOST=127.0.0.1               # 后端监听地址（默认）
GITCORTEX_API_TOKEN=your-api-token   # 若设置，所有 /api/* 都需要 Authorization: Bearer <token>
# 留空或不设置时，开发模式下会跳过这层 API 鉴权
```

### 数据库

项目使用 SQLite（嵌入式），无需安装数据库服务器：
- 开发默认位置：`dev_assets/db.sqlite`
- 迁移文件：`crates/db/migrations/`

### 验证安装

```bash
# 后端编译检查
cargo check --workspace

# 前端编译检查
cd frontend && npm run check && cd ..

# 运行测试
cargo test --workspace
cd frontend && npm run test:run && cd ..
```

---

## 项目结构

```
GitCortex/
├── crates/                    # Rust workspace
│   ├── db/                    # 数据库层（模型 + DAO + 迁移）
│   ├── server/                # Axum 后端服务器
│   ├── services/              # 业务逻辑层
│   │   ├── orchestrator/      # 主 Agent 编排逻辑
│   │   ├── terminal/          # 终端进程管理
│   │   └── ...                # git_watcher.rs / cc_switch.rs 等服务
│   └── utils/                 # 工具函数
├── frontend/                  # React + TypeScript 前端
│   ├── src/
│   │   ├── components/        # UI 组件
│   │   │   ├── workflow/      # 工作流向导组件
│   │   │   └── terminal/      # 终端调试组件
│   │   ├── hooks/             # React Hooks
│   │   ├── pages/             # 页面组件
│   │   └── i18n/              # 国际化配置
│   └── package.json
├── shared/                    # 前后端共享类型（自动生成）
├── docs/                      # 文档
│   ├── 已开发/                # 已开发完成文档
│   └── 未开发/                # 未开发/进行中文档
├── Cargo.toml                 # Workspace 配置
├── rust-toolchain.toml        # Rust 版本锁定
├── package.json               # Root package.json
└── pnpm-workspace.yaml        # pnpm workspace 配置
```

---

## 开发进度

> **数据来源：** `docs/undeveloped/current/TODO.md`（README 进度与其保持一致）
> **总体状态：** 完成率 **97.3%**（**288/296**），进行中 **0**，未开始 **8**（Phase 21: 2 个，Phase 27: 6 个），可选优化 **5**。
> **当前审计分数：** **100/100 (S级)**
> **下一步：** 完成 Phase 18.5 与 Phase 21 的延期/可选项，并保持 TODO 里程碑与当前实现一致。

| 阶段 | 状态（与 TODO 对齐） | 备注 |
|------|----------------------|------|
| Phase 0 - Phase 18 | ✅ 已完成 | 核心链路已打通 |
| Phase 18.1 | ✅ 已完成 | 测试技术债务清理完成 |
| Phase 18.5 | 🚧 进行中 | P0 已完成，P1/P2 存在延后项（含可选优化） |
| Phase 20 | ✅ 已完成 | 自动化协调核心（自动派发） |
| Phase 21 | ✅ 已完成（含 2 项未开始） | 21.10 延后、21.12 可选 |
| Phase 22 | ✅ 已完成 | WebSocket 事件广播完善 |
| Phase 23 | ✅ 已完成 | 终端进程隔离修复 |
| Phase 24 | ✅ 已完成 | 终端自动确认与消息桥接 |
| Phase 25 | ✅ 已完成 | 自动确认可靠性修复 |
| Phase 26 | ✅ 已完成 | 联合审计问题全量修复 |
| Phase 27 | 📋 待实施 | 6 项任务未开始 |

**总体进度：** 288/296 任务完成（97.3%，以 `docs/undeveloped/current/TODO.md` 为准）

详细进度追踪：[docs/undeveloped/current/TODO.md](docs/undeveloped/current/TODO.md)

**质量状态：** 以 `docs/undeveloped/current/TODO.md` 为准，当前记录为 S 级（100/100）。

---

## 当前实测验证状态

### 已完成开发（已验证）

- 主 Agent 已可在单个工作流中协同调用多个 AI CLI。
- 当前已跑通并验证的测试组合为 3 个终端：
  - Claude Code（模型：`glm-4.7`）
  - Claude Code（模型：`claude-opus-4.6`）
  - Codex（模型：`gpt-5.3-codex-xhigh`）

### 未完成 / 未充分验证项

1. 多任务并行尚未充分测试。
2. 终端数量极限尚未测试（支持自定义数量）。
3. 前端 UI 不合理问题尚未修复。
4. 合并终端与错误解决终端流程尚未充分测试。
5. 任务完成通知功能尚未开发。
6. 集成通讯软件尚未开发。
7. 斜杠命令系统尚未充分测试（当前大概率不可用或不稳定）。

### 已测试的端到端示例任务

当前用于验证流程跑通的是一个本地迷你留言板（前后端分离）的简单任务：

- 后端：Python
- 前端：单 HTML 文件
- 功能：用户在网页输入文字并点击保存后，内容写入本地 `.json` 文件，并在下方列表实时显示。

### 项目现状与共建说明

- 这个示例任务刻意保持简单，目标是证明全流程已经跑通。
- 复杂任务与长期稳定性仍需继续验证。
- 当前代码库为 AI 生成代码。
- 欢迎提交 PR。建议先提 Issue 说明正在开发的功能，避免多人重复开发同一项任务。

---

## 架构设计

### 数据模型

GitCortex 采用三层模型：

1. **Workflow（工作流）** - 顶层容器
   - 包含多个 Task
   - 配置 Orchestrator（主 Agent）
   - 配置 Merge Terminal（合并终端）
   - 可选 Error Terminal（错误处理）

2. **WorkflowTask（任务）** - 中层单元
   - 每个 Task 对应一个 Git 分支
   - 包含多个 Terminal
   - 独立状态：pending → running → completed

3. **Terminal（终端）** - 底层执行单元
   - 绑定特定 CLI 类型（Claude/Gemini/Codex）
   - 绑定特定模型配置
   - 串行执行：not_started → starting → waiting → working → completed（异常可到 failed/cancelled）

### 状态机

**Workflow 状态流转：**
```
created → starting → ready → running → (paused) → merging → completed/failed
                                              ↓
                                          cancelled
```

**Terminal 状态流转：**
```
not_started → starting → waiting → working → completed
                                         ↓
                                      failed/cancelled
```

### 核心服务

| 服务 | 职责 |
|------|------|
| **OrchestratorAgent** | 主 Agent，负责任务分发、进度监控、结果审核 |
| **MessageBus** | 跨终端消息路由 |
| **TerminalLauncher** | 终端进程启动与管理 |
| **GitWatcher** | 监听 Git 事件（.git/refs/heads 变化） |
| **CCSwitchService** | 模型配置切换（原子写入配置文件） |
| **Workflow API + DB Models** | 工作流 CRUD 与状态管理（`routes/workflows.rs` + `db/models/workflow*.rs`） |

---

## 支持的 CLI

| CLI | 名称 | 检测命令 | 配置文件路径 |
|-----|------|----------|--------------|
| Claude Code | Claude Code | `claude --version` | `~/.claude/settings.json` |
| Gemini CLI | Gemini | `gemini --version` | `~/.gemini/.env` |
| Codex | Codex | `codex --version` | `~/.codex/auth.json`, `~/.codex/config.toml` |
| Amp | Amp | `amp --version` | - |
| Cursor Agent | Cursor | `cursor --version` | - |
| Qwen Code | Qwen | `qwen --version` | - |
| GitHub Copilot | Copilot | `gh copilot --version` | - |
| Droid | Droid | `droid --version` | - |
| Opencode | Opencode | `opencode --version` | - |

### 模型切换

CC-Switch 提供原子写入机制，安全切换 CLI 模型配置：

- ✅ 支持同时配置多个 CLI
- ✅ 临时切换（单次工作流）
- ✅ 永久切换（修改配置文件）
- ✅ 自动备份原配置
- ✅ 验证模型可用性

---

## 使用指南

### 创建工作流

1. 点击"新建工作流"
2. 选择项目
3. 配置基础信息
4. 添加任务与终端
5. 选择模型与 CLI
6. 启动工作流

### 运维操作

对于生产环境部署、数据库管理、监控和故障排查，请参阅：

- **运维手册：** [docs/developed/ops/runbook.md](docs/developed/ops/runbook.md)
  - 启动服务器（开发/生产模式）
  - 数据库管理（备份/恢复/迁移）
  - 监控与性能调优
  - 升级和回滚流程

- **故障排查：** [docs/developed/ops/troubleshooting.md](docs/developed/ops/troubleshooting.md)
  - 服务器无法启动
  - 工作流卡住
  - API 密钥问题
  - 终端无输出
  - 数据库锁定

### 测试与构建

```bash
# 运行测试
cargo test --workspace
cd frontend && npm run test:run && cd ..

# 构建生产版本（前端 + 后端）
cd frontend && npm run build && cd ..
cargo build --release -p server

# 类型生成
pnpm run generate-types
pnpm run generate-types:check
```

---

## 创建工作区页面说明书（`/workspaces/create`）

### 这个页面是干什么的

这个页面是**快速开工入口**。  
你只需要填写任务意图，并绑定项目/仓库/分支，就可以直接创建并启动一个可执行工作区。

### 页面三栏职责（左 / 中 / 右）

1. **左侧：工作区**
   - 展示已有工作区（进行中/历史）。
   - 作用是切换上下文，不是本次创建配置区。
2. **中间：任务输入框**
   - 用聊天式输入框填写任务描述。
   - 可选择执行器/模型变体，并支持图片附件。
   - 点击创建后会真正发起一个新工作区。
3. **右侧：项目 / 仓库 / 添加仓库**
   - 选择本次任务所属项目。
   - 添加本次需要参与的仓库。
   - 为每个仓库设置目标分支。

### 它和 `/wizard` 会冲突吗

不会。两者是并行入口，定位不同：

- `/workspaces/create`：快速创建单个工作区并立即执行。
- `/wizard`：工作流编排与分阶段流水线管理。

### 最短操作流程

1. 在右侧先选择项目。
2. 添加至少一个仓库并确认目标分支。
3. 在中间输入任务描述。
4. 点击“创建工作区”。

### 常见困惑说明

- “底部对话框”不是弹窗，而是创建任务的主输入区。
- “左边和右边看起来重复”并不冲突：左侧管切换，右侧管创建配置。
- 创建按钮不可用时，通常是缺少任务描述或未添加仓库。

---

## 文档

### 实施计划

- [总体概览](docs/developed/plans/00-overview.md)
- [未开发目录](docs/未开发)
- [最新进度追踪（以此为准）](docs/undeveloped/current/TODO.md)

### 核心设计文档

- [Orchestrator 架构设计](docs/developed/plans/2026-01-16-orchestrator-design.md)
- [GitCortex 详细实现计划](docs/developed/plans/2026-01-16-gitcortex-implementation.md)

### 进度追踪

- [开发进度追踪表](docs/undeveloped/current/TODO.md)

---

## 常见问题

### Q: 编译失败，提示找不到 nightly 版本？

确保安装了正确的 Rust 版本：

```bash
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04
```

### Q: 创建 Workflow 失败，提示加密密钥错误？

确保设置了环境变量：

```bash
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
```

### Q: CLI 检测失败，显示未安装？

确保 CLI 已安装并可在 PATH 中找到：

```bash
claude --version
gemini --version
codex --version
```

### Q: 测试时出现 Browserslist 警告？

更新 Browserslist 数据库：

```bash
pnpm dlx browserslist@latest --update-db
```

---

## 贡献

欢迎提交 Issue 和 Pull Request！

### 开发规范

- **Rust 代码**：遵循 `cargo fmt` 和 `cargo clippy` 规范
- **前端代码**：使用 ESLint + Prettier，严格模式
- **提交信息**：使用约定式提交（Conventional Commits）

### 代码质量标准

当前质量状态以 `docs/undeveloped/current/TODO.md` 为准：**100/100 (S级)**。

建议在每次发版前执行：

- `cargo check --workspace`
- `cargo test --workspace`
- `npm run check`
- `cd frontend && npm run test:run`

---

## 致谢

本项目基于以下优秀的开源项目：

- **[Vibe Kanban](https://github.com/BloopAI/vibe-kanban)** - AI 编码代理任务管理平台 (Apache 2.0)
- **[CC-Switch](https://github.com/farion1231/cc-switch)** - Claude Code/Codex/Gemini CLI 配置切换工具 (MIT)

感谢这些项目的作者和贡献者！

---

## 许可证

本项目遵循上游项目的开源协议：

- Vibe Kanban 部分：Apache License 2.0
- CC-Switch 部分：MIT License

详见 [LICENSE](LICENSE) 文件。

---

<p align="center">
  <em>GitCortex - 让 AI 代理协同工作</em>
</p>
