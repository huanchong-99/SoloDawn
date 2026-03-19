<p align="center">
  <a href="README.md">English</a>
</p>

# GitCortex

**通过简单的对话，完成复杂的生产级项目。**

GitCortex 是一个上层编排 Agent，全自动指挥多个专业 AI CLI（Claude Code、Gemini CLI、Codex、Amp、Cursor Agent 等）并行开发软件。它不直接写代码——它扮演一个全自动的项目经理：分配任务、监控进度、协调 Git 分支、处理错误、合并结果，直到整个项目交付完成。

> 你可以这样理解：你描述想要构建的东西，GitCortex 就会同时调度 5–10 个 AI 终端在不同功能上并行工作，每个终端在自己的 Git 分支上开发，由中央编排器全程自动协调——全程无需人工干预。

---

## 核心设计理念

**"1 个高级工程师 + GitCortex = 原来 1 个高级 + 3 个中级 + 10 个初级工程师的产出。"**

- **上层编排，不生成代码。** 编排 Agent 不写任何代码——它指挥最强的专业工具（Claude Code、Gemini CLI、Codex 等）去完成工作。
- **非侵入式设计。** GitCortex 不替换任何 CLI，不修改任何配置文件，不定义新工具。它继承每个 CLI 的完整原生生态——所有斜杠命令、插件、skill 和 MCP 服务器都原样可用，永远兼容。
- **Git 驱动的事件循环。** 编排器只在 Git 提交事件发生时消耗 LLM token。事件间休眠，零消耗。相比轮询方案节省 98% 以上 token。

---

## 为什么选择 GitCortex

### 痛点

在持续的 AI 辅助编码过程中，有几个痛点至今未被解决：

- 无法在同一个 AI CLI 会话中使用不同提供商的不同模型。
- 多 CLI 协作方案（MCP、skill 等）存在诸多限制，且每次更新都可能失效。
- 工作流插件几个月就过时，生态迭代太快。
- 单终端 AI 编码天然是串行的——一次只能做一件事。

### 解决方案

GitCortex 采用了根本不同的思路：**一个编排 Agent 指挥所有专业 CLI**。

| 能力 | 说明 |
|---|---|
| **上层编排调度** | 中央 Agent 全自动下达指令、监控任务进度、处理分支合并和错误恢复——执行期间零人工干预。 |
| **5–10 倍开发效率** | 多任务并行：编排器同时运行 5–10 个任务，每个任务在独立的 Git 分支上。任务内串行（质量闸门），任务间并行。 |
| **非侵入式生态兼容** | 直接调用原生 CLI 终端。任何在你终端里能用的斜杠命令、插件、skill、MCP，在这里都能用——永远兼容。从一种 AI 工作流切换到另一种（如 Superpower、SDD），迁移成本为零。 |
| **同一任务内混合 CLI 和模型** | 不同的 CLI 和不同提供商的模型可以在同一个任务内协作。Claude Code + Sonnet 写代码，Gemini 做审查，GPT 修复问题——全部自动编排。 |
| **Git 驱动的事件循环** | 终端通过 Git 提交来通知完成。编排器在事件间休眠，空闲时几乎零 token 消耗。相比轮询模式节省 98% 以上的 token。 |
| **对话即交付** | 接入聊天平台（Telegram、飞书），通过对话描述你的项目，GitCortex 全权处理——任务分解、终端分配、执行和交付。 |

---

## 与 CCG / OMO / CCW 的区别

GitCortex **不是**又一个多 CLI 协作工具。核心设计目标有本质区别：

| 维度 | 多 CLI 工具（CCG、OMO、CCW） | GitCortex |
|---|---|---|
| 重点 | CLI 之间的通信协作 | 上层 Agent 指挥所有 CLI |
| 执行方式 | 手动或半自动 | 全自动编排 |
| 并行能力 | 有限 | 设计上支持 5–10 任务并行 |
| 插件生态 | 通常自建生态 | 继承所有原生 CLI 生态 |
| 持久性 | 绑定特定工具版本 | 非侵入式——不受生态迭代影响 |
| 目标 | 更好的 CLI 互操作 | "开发者不在场"的长时间全自动开发 |

GitCortex 不定义工具——它指挥最强的工具去最高效地完成任务。

---

## 两种执行模式

### 手动编排（DIY）

你通过向导预先定义工作流图：
- 设定任务、Git 分支、终端分配、CLI 类型和模型
- 完全控制任务粒度和终端角色
- 适合结构清晰、任务分解明确的项目

### AI 自动编排（Agent-Planned）

你描述项目目标，编排 LLM 自主完成：
- 通过依赖分析将项目分解为任务
- 创建终端，按任务分配 CLI 和模型
- 管理多阶段执行（基础设施 → 功能开发 → 集成 → 收尾）
- 合并完成的分支并调度后续任务
- 适合任务分解本身需要智能判断的复杂项目

### WorkspacePlanning：多轮对话规划

在创建工作流之前，你可以与 AI 进行多轮规划对话：

1. **收集需求（Gathering）** — 你描述项目；AI 提出澄清问题
2. **规格就绪（Spec Ready）** — 生成技术规格并提交审阅
3. **已确认（Confirmed）** — 你批准规格
4. **已物化（Materialized）** — 规格转换为可运行的 agent_planned 工作流

Planning Draft 同时捕获 `requirement_summary` 和 `technical_spec`，两者合并后作为编排器的初始目标。

### 智能需求评估

工作区规划器会自动评估输入的清晰度：

- **模糊需求**（如"做一个知识管理工具"）会触发收集阶段：AI 主动追问范围、功能、认证方式、部署方案等细节。
- **精确需求**（如包含 5 条以上具体技术要求、明确范围）则跳过追问，直接生成技术规格，尊重你的专业判断。

这同时消除了两个痛点：对有经验的用户不做无谓的反复追问，对模糊需求也不会盲目执行导致返工。

### V1.0 功能亮点

| 功能 | 说明 |
|---|---|
| **质量门系统** | 三层校验（终端/分支/仓库），支持 shadow、warn、enforce 模式。工作流和工作区模式均可用。 |
| **发送模式切换** | 在聊天框底部一键切换 Enter / Ctrl+Enter 发送消息。 |
| **响应式布局** | 工作区 UI 在窄屏（800px+）下自动收缩侧边栏，避免溢出。 |
| **仓库自动填充** | 创建工作流时，已绑定的仓库自动出现在选择列表中。 |
| **浅克隆处理** | 自动检测 `--depth 1` 浅克隆并在工作区准备阶段执行 unshallow，防止 commit 失败。 |
| **模型选择可靠性** | 改进模型配置持久化，确保用户选择的模型始终被使用。 |
| **分页加载进程** | 执行进程历史通过 REST API 增量加载，配合数据库索引提升性能。 |
| **API 故障恢复** | 断路器在 provider 耗尽时自动暂停工作流，记录暂停原因以便后续自动恢复。 |

---

## 架构

```
                    ┌─────────────────────────────────┐
                    │   编排 Agent（LLM 驱动）         │
                    │   分发 · 监控 · 合并             │
                    └──────────┬──────────────────────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
     ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
     │   任务 1     │ │   任务 2     │ │   任务 3     │
     │ 分支: auth   │ │ 分支: i18n   │ │ 分支: theme  │
     │              │ │              │ │              │
     │ T1 → T2 → T3│ │ TA → TB     │ │ TX → TY     │
     │  （串行）    │ │  （串行）    │ │  （串行）    │
     └──────────────┘ └──────────────┘ └──────────────┘
              │                │                │
              └────────────────┼────────────────┘
                               ▼
                        自动合并 → main
```

**三层执行模型：**

- **工作流（Workflow）** → 编排 Agent 管理整个生命周期
- **任务（Task）** → 独立 Git 分支，与其他任务并行执行
- **终端（Terminal）** → 原生 AI CLI 进程（PTY），在任务内串行执行

**核心组件：**

| 组件 | 职责 |
|---|---|
| `OrchestratorAgent` | LLM 驱动的决策核心：派发终端、解析 Git 事件、路由审查/修复循环 |
| `OrchestratorRuntime` | 工作流生命周期管理、槽位预留、崩溃恢复 |
| `MessageBus` | 跨模块事件路由（工作流级别的隔离主题） |
| `TerminalLauncher` | 生成原生 PTY 进程，每终端独立环境变量隔离 |
| `GitWatcher` | 检测 Git 提交 → 发布事件 → 唤醒编排器 |
| `ResilientLLMClient` | 多提供商 round-robin 轮转，5 次熔断 + 60 秒探活恢复 |
| `MergeCoordinator` | 集中式合并处理，冲突检测与部分失败追踪 |
| `ChatConnector` | 统一出站消息 Trait（Telegram、飞书） |

---

## 功能特性

### 已实现

**编排与执行**
- ✅ 上层编排 Agent 指挥完整工作流生命周期
- ✅ 两种执行模式：手动编排（DIY）和 AI 自动编排（Agent-Planned）
- ✅ 多任务并行执行（同时 5–10 个任务）
- ✅ WorkspacePlanning 多轮 LLM 对话用于项目规划
- ✅ Planning Draft 生命周期：gathering → spec_ready → confirmed → materialized
- ✅ 任务内串行质量闸门（编码 → 审查 → 修复）
- ✅ 跨终端上下文传递（前序终端的工作成果传递给下一个）
- ✅ ReviewCode / FixIssues / MergeBranch 指令执行
- ✅ 自动分支合并，非重叠变更自动解决冲突
- ✅ 合并后自动刷新 pending 任务的分支基准

**CLI 与模型支持**
- ✅ 9 个 AI CLI 已支持（见下表）
- ✅ 同一任务内混合 CLI 类型（Claude Code + Gemini + Codex + 更多）
- ✅ 同一 CLI 内通过 CC-Switch 切换不同提供商/模型
- ✅ 每终端独立环境变量注入（无全局配置切换）
- ✅ MCP 服务器集成，每个 CLI 自适应配置格式

**质量与可靠性**
- ✅ **内建代码质量门**，执行三层验证机制（终端级 → 任务级 → 仓库级）
- ✅ 内建规则引擎（无需 SonarQube 也能运行），可选本地 SonarQube 集成
- ✅ 策略快照和问题追踪（每终端和每工作流粒度）
- ✅ LLM 容错与优雅降级（Agent 在提供商故障时仍能存活）
- ✅ 状态持久化与崩溃恢复（重启后 Agent 从数据库恢复继续运行）
- ✅ 多提供商熔断器与自动故障转移
- ✅ 终端级提供商故障转移（自动拉起替代终端）

**开发体验**
- ✅ 网页伪终端，支持实时调试和交互
- ✅ 原生斜杠命令系统——支持所有官方和自定义命令
- ✅ 完全兼容原生插件/skill/MCP（CLI 支持的，这里都支持）
- ✅ Git 驱动的事件循环（相比轮询节省 98% 以上 token）
- ✅ Setup Wizard 首次运行引导（环境检测、模型配置、项目设置）
- ✅ 国际化：6 种语言（English、简体中文、繁體中文、日本語、Español、한국어）

**聊天平台集成**
- ✅ Telegram 连接器与会话绑定
- ✅ 飞书长连接 WebSocket 连接器，会话绑定与 Bot 事件通知

**部署与运维**
- ✅ Docker 一键部署，含交互式安装/更新脚本
- ✅ 拆分部署架构（Server + Runner + Redis，通过 `docker-compose.split.yml`）
- ✅ 提供商健康监控 API（SSE 和 WebSocket 事件）
- ✅ 健康检查端点（`/healthz`、`/readyz`、`/api/health`）
- ✅ Sentry 错误追踪集成
- ✅ PostHog 产品分析集成
- ✅ 结构化日志（`tracing` crate）

**安全特性**
- ✅ API Token 认证（`GITCORTEX_API_TOKEN`）
- ✅ AES-256-GCM 加密 API 密钥静态存储（每次加密随机 nonce）
- ✅ OAuth 支持外部服务认证
- ✅ 每请求令牌验证中间件

### 路线图

- 📋 Kubernetes 部署支持
- 📋 容器镜像体积优化

---

## 支持的 AI CLI

| CLI | 状态 | 模型切换 | MCP 配置 |
|---|---|---|---|
| Claude Code | ✅ 已支持 | ✅ 通过 CC-Switch | Passthrough |
| Gemini CLI | ✅ 已支持 | ✅ 通过 CC-Switch | Gemini 适配器 |
| Codex | ✅ 已支持 | ✅ 通过 CC-Switch | Codex 适配器 |
| Amp | ✅ 已支持 | — | Passthrough |
| Cursor Agent | ✅ 已支持 | — | Cursor 适配器 |
| Qwen Code | ✅ 已支持 | — | — |
| GitHub Copilot | ✅ 已支持 | — | Copilot 适配器 |
| Droid | ✅ 已支持 | — | Passthrough |
| Opencode | ✅ 已支持 | — | Opencode 适配器 |

任何能在终端运行且支持斜杠命令的 CLI 都可以集成。

---

## 快速开始

### 前置要求

| 工具 | 版本 | 验证命令 |
|---|---|---|
| Rust | nightly-2025-12-04 | `rustc --version` |
| Node.js | ≥ 18（推荐 20） | `node --version` |
| pnpm | 10.13.1 | `pnpm --version` |
| Git | 任意近期版本 | `git --version` |

### 本地开发

```bash
# 1. 安装依赖
pnpm install

# 2. 设置加密密钥（必需，恰好 32 个字符）
# Windows PowerShell:
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
# Linux/macOS:
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# 3. 初始化数据库
pnpm run prepare-db

# 4. 启动开发服务器（前端 + 后端）
pnpm run dev
```

默认地址：
- 前端：`http://localhost:23457`
- 后端 API：`http://localhost:23456/api`

首次启动时，**Setup Wizard** 会引导你完成环境检测、AI 模型配置和项目设置。

**可选：SonarQube 集成**
GitCortex 内建三层质量门系统。内建规则引擎无需 SonarQube 即可运行，但如需深度代码分析可启动本地实例：
```bash
cd docker/compose
docker-compose -f docker-compose.dev.yml up -d sonarqube
```

### 生产模式部署

```bash
# 1. 编译后端（Release 二进制）
cargo build --release -p server

# 2. 编译前端（静态资源，嵌入后端）
cd frontend && pnpm build && cd ..

# 3. 设置加密密钥（必需，恰好 32 个字符）
# Linux/macOS:
export GITCORTEX_ENCRYPTION_KEY="你的32位加密密钥"
# Windows PowerShell:
$env:GITCORTEX_ENCRYPTION_KEY="你的32位加密密钥"

# 4. 运行
./target/release/server       # Linux/macOS
.\target\release\server.exe   # Windows
```

生产模式下前端和 API 通过同一个端口提供服务：`http://localhost:23456`

> **注意：** 开发模式（`pnpm run dev`）下加密密钥是可选的——会自动使用默认密钥。生产模式（Release 编译）下，如果未设置密钥，服务器会**拒绝启动**。

### Docker 一键安装

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

安装脚本支持交互式配置（挂载目录、密钥、端口、是否安装 AI CLI）、复用已有 `.env`、自动切换到更新流程。加密密钥在安装向导中自动配置。

### Docker 更新

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest
```

可选参数：`-AllowDirty`、`-PullBaseImages`、`-SkipBuild`、`-SkipReadyCheck`

### 拆分部署（Server + Runner）

适用于需要将 Web 服务器与终端执行分离的团队：

```bash
cd docker/compose
docker-compose -f docker-compose.split.yml up -d
```

启动三个服务：
- **Server** — API + 前端，处理编排逻辑
- **Runner** — 执行 PTY 终端（安装 AI CLI），通过 gRPC 通信
- **Redis** — Server 和 Runner 之间的消息中间件

适用场景：CI/CD 环境、多机部署、或 AI CLI 需要不同运行时的场景。

---

## 工作原理

### 1. 创建工作流

通过网页向导，你可以选择：

**DIY 模式：**
- 选择 Git 仓库
- 定义并行任务（如"认证模块"、"国际化"、"暗色主题"）
- 为每个任务分配终端（选择 CLI 类型 + 模型）
- 可选配置斜杠命令执行顺序
- 配置编排 Agent 的 LLM

**Agent-Planned 模式：**
- 选择 Git 仓库
- 用自然语言描述项目目标
- 配置编排 Agent 的 LLM
- 编排器将自主分解项目、创建任务、分配终端

### 2. 准备与调试

GitCortex 启动所有终端 PTY 进程，进入**就绪**状态。你可以：
- 在网页伪终端中验证 CLI 环境
- 测试斜杠命令和插件可用性
- 安装缺失的依赖

此阶段零 token 消耗。

### 3. 执行

点击**开始**，编排器接管一切：
- 向每个任务的第一个终端派发指令
- 监控 Git 提交获取完成信号
- 将已完成终端的上下文传递给下一个终端（交接备注）
- 处理审查循环（ReviewCode → FixIssues → 重新审查）
- 自动管理错误和重试
- 自动解决非重叠的合并冲突
- 每次合并后刷新 pending 任务的分支基准
- 工作流完成时合并所有任务分支

编排器在 Git 事件间休眠——只在有实际工作需要处理时才唤醒并消耗 token。

### 4. 交付

所有任务分支自动合并到目标分支。工作流完成。

---

## MCP 集成

GitCortex 在两个层面集成 Model Context Protocol（MCP）：

### MCP Task Server
GitCortex 内建 MCP Task Server（`mcp_task_server` 二进制），将工作流和任务管理暴露为 MCP 工具，使外部 AI Agent 能够以编程方式与 GitCortex 交互。

### 每 CLI 的 MCP 配置
启动终端时，GitCortex 为每种 CLI 类型自动生成正确的 MCP 服务器配置。每个 CLI 有自己的适配器，将统一的服务器定义转换为 CLI 特定的格式：
- **Claude Code / Amp / Droid** — Passthrough（原生 MCP 格式）
- **Gemini CLI** — Gemini 专用适配器
- **Codex** — Codex 专用适配器
- **Cursor Agent** — Cursor 专用适配器
- **Opencode** — Opencode 专用适配器
- **GitHub Copilot** — Copilot 专用适配器

---

## 飞书集成

GitCortex 包含完整的飞书 Bot 集成：

- **WebSocket 长连接**实时接收事件
- **会话绑定** — 对话关联到特定工作流
- **Bot 事件通知** — 工作流状态变更、终端完成和错误推送到绑定的飞书会话
- **前端设置 UI** 配置，支持国际化（en + zh-Hans）
- **3 个 REST 端点**用于飞书 Webhook、Bot 管理和会话控制

---

## 安全特性

| 特性 | 详情 |
|---|---|
| **API Token 认证** | 设置 `GITCORTEX_API_TOKEN` 环境变量；所有 API 路由需要 `Authorization: Bearer <token>` |
| **AES-256-GCM 加密** | API 密钥静态加密，每次加密使用随机 nonce；密钥来自 `GITCORTEX_ENCRYPTION_KEY`（32 字节） |
| **OAuth 支持** | 外部服务认证的 OAuth 客户端 |
| **审批闸门** | 破坏性操作的交互式确认提示；用户通过 WebSocket 确认 |
| **每请求验证** | 配置 token 后所有路由启用 `require_api_token` 中间件 |

---

## 可观测性

| 特性 | 详情 |
|---|---|
| **Sentry** | 通过 `utils::sentry` 集成错误追踪 |
| **PostHog** | 产品分析集成 |
| **结构化日志** | `tracing` crate，通过 `RUST_LOG` 配置级别 |
| **健康检查端点** | `/healthz`（存活）、`/readyz`（就绪：DB + 资源 + 临时目录 + 飞书）、`/api/health`（旧版） |
| **CLI 健康监控** | SSE 端点实时推送提供商健康状态 |

---

## 数据安全

- **删除项目**仅移除数据库记录。磁盘上的仓库文件不受任何影响。
- **解除仓库关联**仅移除数据库中的绑定关系。Git 仓库保持完整。
- **项目-仓库绑定**仅存储引用路径。绑定/解绑不会对文件系统产生任何操作。

---

## 质量门

GitCortex 内建质量门引擎，在三个层级自动验证代码质量：

| 质量门 | 触发时机 | 检查范围 |
|--------|---------|---------|
| **终端级** | 每次 checkpoint 提交 | 仅变更文件 — cargo check、clippy、tsc、测试 |
| **分支级** | 任务最后一个终端通过后 | 整个任务分支 — 全部检查 + lint |
| **仓库级** | 合并主分支前 / CI | 整个仓库 — 全部检查 + SonarQube 分析 |

### 内建规则引擎

质量门包含内建规则引擎，**无需 SonarQube 即可运行**：
- 运行可配置的 lint 检查（Rust clippy、TypeScript tsc、ESLint）
- 每终端和每工作流粒度的策略快照追踪
- 问题追踪，结构化修复指令回传终端
- SonarQube 是可选的——启用后提供更深度的静态分析

### 模式

在 `quality/quality-gate.yaml` 中配置：

```yaml
mode: shadow  # off | shadow | warn | enforce
```

| 模式 | 行为 |
|------|------|
| `off` | 关闭质量门，走旧流程 |
| `shadow` | 运行分析并记录结果，但不阻断（默认） |
| `warn` | 运行分析，通过 UI 通知，不阻断合并 |
| `enforce` | 硬性门禁 — 不通过则阻断终端交接 |

### 工作原理

1. 终端提交代码 → 编排器拦截为 **checkpoint**（非最终完成）
2. 质量引擎对终端工作目录运行配置的检查项
3. **通过** → 终端升格为已完成 → 调度下一个终端
4. **失败** → 结构化修复指令回传给同一终端 → 终端修复后重新提交

### 手动运行

```bash
# 完整质量门（仓库级，shadow 模式）
pnpm run quality

# 试运行检查
pnpm run quality:check

# 仅 SonarCloud 分析
pnpm run quality:sonar
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `QUALITY_GATE_MODE` | 覆盖 YAML 模式（off/shadow/warn/enforce） |
| `SONAR_TOKEN` | SonarQube/SonarCloud 认证令牌 |
| `SONAR_HOST_URL` | SonarQube 服务器地址（默认：http://localhost:9000） |

---

## 技术栈

| 层级 | 技术 |
|---|---|
| 后端 | Rust（Axum、SQLx、Tokio） |
| 前端 | React 18、TypeScript、Tailwind CSS、Zustand、React Query |
| 数据库 | SQLite（API 密钥通过 AES-256-GCM 加密存储） |
| 终端 | xterm.js + 原生 PTY（WebSocket 桥接） |
| 实时通信 | WebSocket（工作流事件 + 终端流） |
| 类型安全 | Rust → TypeScript 通过 `ts-rs` 自动生成 |
| 国际化 | 6 种语言（en、zh-Hans、zh-Hant、ja、es、ko） |

---

## 项目结构

```
GitCortex/
├── crates/                    # Rust 工作空间
│   ├── db/                    # 数据库层（模型、迁移、DAO）
│   ├── server/                # Axum HTTP/WebSocket 服务器 + MCP Task Server
│   ├── services/              # 业务逻辑
│   │   ├── orchestrator/      # Agent、Runtime、State、错误处理
│   │   ├── terminal/          # Launcher、Bridge、Prompt 监听
│   │   ├── git_watcher.rs     # Git 提交监控
│   │   ├── cc_switch.rs       # CLI/模型配置切换
│   │   ├── message_bus.rs     # 事件路由
│   │   ├── merge_coordinator.rs # 集中式合并处理
│   │   ├── feishu.rs          # 飞书服务集成
│   │   └── chat_connector.rs  # 统一聊天 Trait
│   ├── cc-switch/             # CLI 模型切换库
│   ├── executors/             # CLI 集成 + MCP 配置适配器
│   ├── feishu-connector/      # 飞书 WebSocket 客户端
│   ├── quality/               # 代码质量门引擎
│   ├── runner/                # gRPC 远程 Runner（拆分部署）
│   ├── review/                # 代码审查 CLI
│   ├── deployment/            # 部署工具
│   └── utils/                 # 共享工具（加密、OAuth、分析、Sentry）
├── frontend/                  # React 应用
│   ├── src/
│   │   ├── components/        # UI 组件（看板、工作流、Setup Wizard、新设计系统）
│   │   ├── hooks/             # React Query hooks
│   │   ├── stores/            # Zustand stores（WebSocket、UI 状态）
│   │   ├── i18n/              # 6 种语言（en、zh-Hans、zh-Hant、ja、es、ko）
│   │   └── pages/             # 路由组件
│   └── CLAUDE.md              # 前端设计规范
├── shared/                    # 自动生成的 TypeScript 类型（Rust → TS）
├── quality/                   # 质量门配置和基线
├── scripts/                   # 开发、Docker 和部署脚本
└── docs/                      # 文档和架构决策
```

---

## 健康检查

```bash
curl http://localhost:23456/healthz    # 存活检查（无状态，始终 200）
curl http://localhost:23456/readyz     # 就绪检查（检查 DB、资源目录、临时目录、飞书状态）
curl http://localhost:23456/api/health # 旧版健康检查
```

启用 API Token 时：

```bash
curl http://localhost:23456/api/health -H "Authorization: Bearer <token>"
```

---

## 贡献

- 大改动建议先提 Issue。
- 保持小步提交，便于评审。
- 提交 PR 前建议执行：

```bash
cargo check --workspace
cargo test --workspace
cd frontend && pnpm test:run && cd ..
```

---

## 许可证

- Vibe Kanban 衍生部分：Apache-2.0
- CC-Switch 衍生部分：MIT
- 详见 `LICENSE`
