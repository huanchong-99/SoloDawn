<p align="center">
  <img src="installer/assets/solodawn.ico" alt="SoloDawn" width="120" />
</p>

<h1 align="center">SoloDawn</h1>

<p align="center">
  <strong>给它一句话需求，它全自动帮你做完整个项目，中间不用管。</strong>
</p>

<p align="center">
  <a href="README.md">English</a>
</p>

---

## SoloDawn 是什么？

SoloDawn 的最终设计目标是通过社交平台的简单对话，完成复杂项目的产出——不是那种玩具 Demo，而是**真正的复杂化的生产级产品**。

> **一句话总结：** 不管你是不是程序员，只需要提需求，剩下的全是自动的。

---

## 我们解决什么问题

现在用 AI 写代码有个根本矛盾——

**程序员：** 用 vibe coding，得自己搞工作流、配 skill、接 MCP、写计划、理文档……一堆前置工作搞完，才能跑出满意的结果。本质上还是人在驱动。

**非程序员：** 比如项目经理，他连需求都没法转成技术规范。说白了，现在 AI 提效的前提是——你本身就得懂这活儿。

**SoloDawn 把这两个问题都解了：**

| 你是谁 | 你做什么 | 系统做什么 |
|---|---|---|
| **程序员** | 把任务目标扔进去 | 全自动开发，不用发指令、不用点继续，等验收就行。之前你配好的工作流、skill、MCP？全兼容，无缝接入，零迁移成本。 |
| **非程序员** | 用大白话说需求 | 系统会自动追问，用大白话把模糊的地方问清楚，然后在后端自动生成规范的技术文档，之后一样——等验收。 |

**一人公司的最佳伙伴。** 使用这个项目，你几乎相当于有了一个专业的程序员团队。

---

## 两种工作模式

### 1. 手动工作流（高阶）

面向有经验的用户，完全掌控：

- 自己选择需要几个终端以及每个终端的角色身份
- 为每个终端选择使用的模型
- 配置每个终端需要调用哪些 skill、MCP 服务器和斜杠命令
- 工作流图的每个细节都可以自定义

### 2. 编排工作区

编排工作区细分为两个子模式：

#### 直接执行模式
面向技术人员——直接提供一个详细的、明确的要求，系统直接去执行，不废话。

#### 引导对话模式
面向非技术人员——提一个模糊的需求，系统会：
1. **主动用大白话进行追问**，直到需求完全明确
2. **在后端自动形成一个规范的技术型要求**
3. **上层编排 Agent 接管**，全自动指挥所有终端，直到产出结果

---

## 内置质量检查门——最大程度解决 AI 幻觉问题

AI 生成的代码有质量问题。SoloDawn 用**三层质量门系统**正面解决，最大程度消除 AI 幻觉对产出代码质量的影响：

| 质量门 | 触发时机 | 检查范围 |
|--------|---------|---------|
| **终端级** | 每次 checkpoint 提交 | 仅变更文件 — cargo check、clippy、tsc、测试、密钥泄露检测 |
| **分支级** | 任务最后一个终端完成后 | 整个任务分支 — 全部检查 + lint + 覆盖率 + 圈复杂度 |
| **仓库级** | 合并主分支前 / CI | 整个仓库 — 全部检查 + SonarQube 分析 + 安全扫描 |

**四种执行模式：** `off` → `shadow` → `warn` → `enforce`

当质量门检查失败时，结构化的修复指令会自动回传给同一个终端。终端自行修复后重新提交——全程无需人工干预。这构成一个**自愈式开发循环**，在 AI 幻觉引发的错误进入代码库之前就将其捕获并修正。

---

## 核心设计理念

- **上层编排，不生成代码。** 编排 Agent 不写任何代码——它指挥最强的专业 AI CLI（Claude Code、Gemini CLI、Codex、Amp、Cursor Agent 等）去完成工作。
- **非侵入式设计。** SoloDawn 不替换任何 CLI，不修改任何配置，不定义新工具。它继承每个 CLI 的完整原生生态——所有斜杠命令、插件、skill 和 MCP 服务器都原样可用。你现有的配置？零迁移成本。
- **Git 驱动的事件循环。** 编排器只在 Git 提交事件发生时消耗 LLM token。事件间休眠，零消耗——相比轮询方案节省 98% 以上 token。

---

## 架构

```
           ┌──────────────────────────────────────────────┐
           │          编排 Agent（LLM 驱动）                │
           │        分发 · 监控 · 合并                      │
           └─────────────────────┬────────────────────────┘
                                 │
            ┌────────────────────┼────────────────────┐
            ▼                    ▼                    ▼
   ┌────────────────┐  ┌────────────────┐  ┌────────────────┐
   │    任务 1      │  │    任务 2      │  │    任务 3      │
   │  分支: auth    │  │  分支: i18n    │  │  分支: theme   │
   │                │  │                │  │                │
   │  T1 → T2 → T3 │  │  TA → TB      │  │  TX → TY      │
   │  （串行 +      │  │  （串行 +     │  │  （串行 +     │
   │  质量门检查）   │  │  质量门检查）  │  │  质量门检查）  │
   └────────────────┘  └────────────────┘  └────────────────┘
            │                    │                    │
            └────────────────────┼────────────────────┘
                                 ▼
                          质量门检查
                                 ▼
                        自动合并 → main
```

**三层执行模型：**

- **工作流（Workflow）** → 编排 Agent 管理整个生命周期
- **任务（Task）** → 独立 Git 分支，与其他任务并行执行
- **终端（Terminal）** → 原生 AI CLI 进程（PTY），在任务内串行执行，受质量门管控

**核心组件：**

| 组件 | 职责 |
|---|---|
| `OrchestratorAgent` | LLM 驱动的决策核心：派发终端、解析 Git 事件、路由审查/修复循环 |
| `OrchestratorRuntime` | 工作流生命周期管理、槽位预留、崩溃恢复 |
| `QualityGateEngine` | 三层验证引擎（终端级/分支级/仓库级），可配置执行模式 |
| `MessageBus` | 跨模块事件路由（工作流级别的隔离主题） |
| `TerminalLauncher` | 生成原生 PTY 进程，每终端独立环境变量隔离 |
| `GitWatcher` | 检测 Git 提交 → 发布事件 → 唤醒编排器 |
| `ResilientLLMClient` | 多提供商 round-robin 轮转，5 次熔断 + 60 秒探活恢复 |
| `MergeCoordinator` | 集中式合并处理，冲突检测与部分失败追踪 |
| `ChatConnector` | 统一出站消息 Trait（Telegram、飞书） |

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

## 功能特性

### 编排与执行
- ✅ 上层编排 Agent 指挥完整工作流生命周期
- ✅ 两种工作模式：手动工作流（DIY）和编排工作区（Agent-Planned）
- ✅ 编排工作区子模式：直接执行和引导对话
- ✅ 智能需求评估——模糊输入触发大白话追问；精确输入直接执行
- ✅ 多任务并行执行（同时 5–10 个任务）
- ✅ WorkspacePlanning 多轮 LLM 对话用于项目规划
- ✅ Planning Draft 生命周期：gathering → spec_ready → confirmed → materialized
- ✅ 跨终端上下文传递（前序终端的工作成果传递给下一个）
- ✅ 自动分支合并，非重叠变更自动解决冲突

### 质量与可靠性
- ✅ **三层质量门系统**（终端级 → 分支级 → 仓库级），正面对抗 AI 幻觉
- ✅ 内建规则引擎（无需 SonarQube 也能运行），可选 SonarQube 集成
- ✅ 四种执行模式：off / shadow / warn / enforce
- ✅ 自愈循环：质量门失败 → 结构化修复指令 → 终端自动修正 → 重新检查
- ✅ 策略快照和问题追踪（每终端和每工作流粒度）
- ✅ 密钥泄露检测，防止凭证泄漏
- ✅ 圈复杂度和代码重复检查
- ✅ LLM 容错与优雅降级
- ✅ 状态持久化与崩溃恢复
- ✅ 多提供商熔断器与自动故障转移

### CLI 与模型支持
- ✅ 9 个 AI CLI 已支持
- ✅ 同一任务内混合 CLI 类型
- ✅ 同一 CLI 内通过 CC-Switch 切换不同提供商/模型
- ✅ 每终端独立环境变量注入
- ✅ MCP 服务器集成，每个 CLI 自适应配置格式

### 开发体验
- ✅ 网页伪终端，支持实时调试和交互
- ✅ 完全兼容原生插件/skill/MCP——零迁移成本
- ✅ Git 驱动的事件循环（相比轮询节省 98% 以上 token）
- ✅ Setup Wizard 首次运行引导
- ✅ 国际化：6 种语言（English、简体中文、繁體中文、日本語、Español、한국어）

### 聊天平台集成
- ✅ Telegram 连接器与会话绑定
- ✅ 飞书长连接 WebSocket 连接器，会话绑定

### 部署与运维
- ✅ Docker 一键部署，含交互式安装脚本
- ✅ 拆分部署架构（Server + Runner + Redis）
- ✅ 提供商健康监控 API
- ✅ Sentry 错误追踪 + PostHog 产品分析
- ✅ AES-256-GCM 加密 API 密钥静态存储

### 路线图
- 📋 Kubernetes 部署支持
- 📋 容器镜像体积优化

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
# Linux/macOS:
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
# Windows PowerShell:
$env:SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"

# 3. 初始化数据库
pnpm run prepare-db

# 4. 启动开发服务器（前端 + 后端）
pnpm run dev
```

默认地址：
- 前端：`http://localhost:23457`
- 后端 API：`http://localhost:23456/api`

首次启动时，**Setup Wizard** 会引导你完成环境检测、AI 模型配置和项目设置。

### 生产模式部署

```bash
# 1. 编译后端
cargo build --release -p server

# 2. 编译前端（静态资源，嵌入后端）
cd frontend && pnpm build && cd ..

# 3. 设置加密密钥（必需）
export SOLODAWN_ENCRYPTION_KEY="你的32位加密密钥"

# 4. 运行
./target/release/server
```

生产模式下前端和 API 通过同一个端口提供服务：`http://localhost:23456`

### Docker 一键安装

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

### 拆分部署（Server + Runner）

```bash
cd docker/compose
docker-compose -f docker-compose.split.yml up -d
```

---

## 质量门配置

在 `quality/quality-gate.yaml` 中配置：

```yaml
mode: shadow  # off | shadow | warn | enforce
```

| 模式 | 行为 |
|------|------|
| `off` | 关闭质量门 |
| `shadow` | 运行分析并记录结果，但不阻断（默认） |
| `warn` | 运行分析，通过 UI 通知，不阻断合并 |
| `enforce` | 硬性门禁 — 不通过则阻断终端交接 |

```bash
# 手动运行质量门
pnpm run quality

# 试运行检查
pnpm run quality:check
```

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
| 质量保障 | 内建规则引擎 + 可选 SonarQube |
| 国际化 | 6 种语言（en、zh-Hans、zh-Hant、ja、es、ko） |

---

## 项目结构

```
SoloDawn/
├── crates/                    # Rust 工作空间
│   ├── server/                # Axum HTTP/WebSocket 服务器 + MCP Task Server
│   ├── services/              # 业务逻辑
│   │   ├── orchestrator/      # Agent、Runtime、State、错误处理
│   │   ├── terminal/          # Launcher、Bridge、Prompt 监听
│   │   ├── git_watcher.rs     # Git 提交监控
│   │   ├── merge_coordinator.rs # 集中式合并处理
│   │   └── chat_connector.rs  # 统一聊天 Trait
│   ├── quality/               # 三层质量门引擎
│   │   └── src/gate/          # 移植自 SonarQube 质量门模型
│   ├── cc-switch/             # CLI 模型切换库
│   ├── executors/             # CLI 集成 + MCP 配置适配器
│   ├── feishu-connector/      # 飞书 WebSocket 客户端
│   ├── db/                    # 数据库层（模型、迁移、DAO）
│   ├── runner/                # gRPC 远程 Runner（拆分部署）
│   └── utils/                 # 共享工具（加密、OAuth、Sentry）
├── frontend/                  # React 应用
│   └── src/
│       ├── components/        # UI 组件
│       ├── stores/            # Zustand stores
│       └── i18n/              # 6 种语言
├── quality/                   # 质量门配置和基线
├── scripts/                   # 开发、Docker 和部署脚本
└── docs/                      # 文档
```

---

## 贡献

- 大改动建议先提 Issue。
- 提交 PR 前建议执行：

```bash
cargo check --workspace
cargo test --workspace
cd frontend && pnpm test:run && cd ..
```

---

## 许可证

- SoloDawn：Apache-2.0
- Vibe Kanban 衍生部分：Apache-2.0
- CC-Switch 衍生部分：MIT
- 质量门模型（移植自 SonarQube）：LGPL-3.0
- 详见 `LICENSE`

## 友链

- [LINUX DO](https://linux.do/)
