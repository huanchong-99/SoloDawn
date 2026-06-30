<p align="center">
  <img src="installer/assets/solodawn.ico" alt="SoloDawn" width="120" />
</p>

<h1 align="center">SoloDawn</h1>

<p align="center">
  <strong>给它一句话需求，它全自动帮你做完整个项目，中间不用管。</strong>
</p>

<p align="center">
  <a href="README.md">English</a>
  &nbsp;·&nbsp;
  <a href="https://linux.do/">社区</a>
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

### Claude Code：无 `-p` 交互式传输与计费保证

SoloDawn 中的每一次 Claude Code 运行——初始请求、追问，**以及**代码评审——都走
**交互式 Claude Code（不带 `-p`/`--print`）**，与你在终端手动运行 `claude` 完全一致。
该传输方式是读取磁盘上的会话 transcript JSONL（逐行 tail），而非消费 `--print` 流。
这样做只有一个目的：**计费正确性。**

| 认证模式 | 判定方式 | 计费来源 | 接线方式 |
|---|---|---|---|
| **原生（订阅）** | 未配置 API Key | **仅**消耗你的 Pro/Max 套餐额度——绝不动用 Agent SDK 信用额 | 将 OAuth `~/.claude/.credentials.json` 复制进隔离 home；清除计费相关环境变量 |
| **官方 Key** | 有 API Key、无自定义 base URL | 该 Key 的按量付费账户 | `ANTHROPIC_API_KEY` |
| **中转（Relay）** | 有 API Key **且**有自定义 base URL | 中转端点 | `ANTHROPIC_AUTH_TOKEN` + `ANTHROPIC_BASE_URL` |

- **订阅用户只消耗自己的套餐额度（Pro/Max），绝不动用 Agent SDK 的按量付费信用额。**
  这一保证完全依赖交互式传输；`-p` 会从 SDK 信用额度池中扣费。
- 凭证优先级与原 `-p` 路径完全一致——你拿到*哪一份*凭证不变，只是传输方式改变。
- **`-p` 是休眠的回退方案。** 设置 `SOLODAWN_NO_POOL=1` 可切回经过验证的 `-p` 路径
  （例如用于调试）；它接受信用额度池扣费，默认关闭。
- **Tier-2 交互式审批**（通过 PTY 自动应答 Claude 的逐工具权限弹窗）**默认关闭**，由
  `SOLODAWN_INTERACTIVE_APPROVALS_TIER2=1` 控制。不设置时，默认的 tier-1 路径完全不受影响。

> 说明：原生订阅与官方 Key 模式由单元测试/argv-env 测试以及启动时的实时复探覆盖；
> **中转**与 **api-key** 模式的完整实时端到端验证需要真实凭证，属于人工检查项。

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

## V1.0 实测 — 真实任务交付结果

SoloDawn V1.0 经过一次**48 小时全自动、自修复的端到端测试**验证（2026-06-27 → 06-30）：通过浏览器 UI 串行执行 7 个真实任务，Stop-hook 驱动器在运行过程中**自行诊断并修复**编排器根因——全程零人工介入。

**最终评级：88.85 / A — 7 个任务全部交付。**

| # | 任务 | 仓库 | 评分 | 评级 |
|---|------|------|:---:|:---:|
| 1 | 知识库应用（从零构建） | `knowledge-base-demo` | 81 | B |
| 2 | Hoppscotch 负载测试模块 | `hoppscotch-demo` | 88 | A |
| 3 | Express → Rust 重写 | `express-to-rust-demo` | 93 | A |
| 4 | 重构 + 测试补齐 | `refactor-test-demo` | 92 | A |
| 5 | 微服务电商（从零构建） | `ecommerce-demo` | 91 | A |
| 6 | 安全 + 性能 + 监控 | `kutt-security-demo` | 86 | A |
| 7 | 飞书备忘录应用 | `web-memo-demo` | 91 | A |

评分维度：可构建性(20) / 功能完整性(25) / 代码质量(30) / 测试质量(15) / 工程化(10)，按任务复杂度加权。评级：S≥95 · A≥85 · B≥70。

测试本身也是对 SoloDawn 自愈能力的压力测试。48 小时运行中暴露并修复了 **21 个编排器死锁/停滞根因**（"§8" 修复链 #1–#21）——每一个都是朴素实现会永远卡死的场景。每个修复都部署后当场重新验证才继续。完整报告：`docs/undeveloped/current/V1.0-质量验收-2026-06-30-48h自修复测试.md`。

> 模型：`glm-5.2[1m]` via solodawn.cloud（Anthropic 协议）。测试方式：浏览器 MCP UI、串行执行、/goal + Stop-hook 自修复。

---

## 🤖 给 AI 的提示词 —— "帮我跑起来"

> **面向最终用户。** 你不是来改 SoloDawn 源码的——你只是想让它跑起来用。把下面的提示词复制给任意编程 AI（Claude Code、Cursor、Codex 等），它就能帮你在本机把 SoloDawn 跑起来。

```markdown
帮我安装并运行 SoloDawn。我要的是"使用"它，不是修改它的源码。

仓库（克隆地址）：https://github.com/huanchong-99/SoloDawn
克隆命令：git clone https://github.com/huanchong-99/SoloDawn.git

请检测我的系统（Windows / Linux / macOS），完成一切必要步骤，让网页 UI 能在 http://localhost:23457 打开：

1. 缺失则安装以下前置项，并逐项验证版本：
   - Rust 工具链 nightly-2025-12-04（rustup install nightly-2025-12-04）
   - Node.js >= 18 与 pnpm 10.13.1
   - Git
   - Rust 后端编译所需的构建工具链：C/C++ 编译器、protoc 31.1、LLVM/libclang、以及（x86-64 上）cmake + nasm + perl（aws-lc-rs 需要）
2. cd SoloDawn && pnpm install
3. 设置 32 字符的 SOLODAWN_ENCRYPTION_KEY 环境变量。
4. pnpm run dev —— 首次启动会编译 Rust 后端（数分钟），随后前端 :23457 / 后端 :23456 提供服务。
5. 轮询 http://localhost:23456/readyz 直到返回 {"ready":true}，然后打开 http://localhost:23457。
6. 确认设置向导出现，然后帮我配置一个 AI 模型并验证通过。

如果某步构建失败，读取错误、补装缺失的前置项、重试。除非某步确实必要，否则不要修改 SoloDawn 自己的源码文件。
```

**运行 vs 开发 —— 你是哪一种？**

| | 运行 SoloDawn（使用它） | 开发 SoloDawn（改它的代码） |
|---|---|---|
| 你做什么 | `git clone` → `pnpm run dev` → 打开网页 UI | 左边全部 + 编辑 Rust/TS 源码 |
| 前置项 | 上面的构建工具链（后端只需编译一次） | 额外需要 `sqlx-cli` 0.8.6、lint 工具、完整测试工具链 |
| 缓存占用 | 适中的 `target/`（仅 server 二进制 + 依赖） | 较大的 `target/`（全工作空间 test/clippy/codegen） |
| 参考 | 下面的[快速开始](#快速开始) | [贡献](#贡献) |

> **1.0 以源码方式发布** —— **不提供任何形式的安装包**。克隆后用 `pnpm run dev` 运行（或自行构建 release 二进制）。`installer/` 目录与 `Build Windows Installer` 工作流为早期开发的历史保留，1.0 不发布 Windows 安装包。

---

## 快速开始

### 前置要求

| 工具 | 版本 | 验证命令 |
|---|---|---|
| Rust | nightly-2025-12-04 | `rustc --version` |
| C/C++ 工具链 | MSVC Build Tools（Windows）· gcc/clang（Linux/macOS） | — |
| protoc | 31.1 | `protoc --version` |
| LLVM / libclang | 较新版本（bindgen 需要） | `clang --version` |
| cmake · nasm · perl | 较新版本（x86-64 上 `aws-lc-rs` 需要） | `cmake --version` · `nasm --version` |
| Node.js | ≥ 18（推荐 20） | `node --version` |
| pnpm | 10.13.1 | `pnpm --version` |
| Git | 任意近期版本 | `git --version` |

> ⚠️ **`protoc`、`LLVM/libclang`、以及 `aws-lc-rs` 的构建工具（`cmake`、`nasm`、`perl`）都是构建必需项，但 `scripts/setup-windows.ps1` 不会安装**，需手动安装：
>
> **Windows：** 下载 [`protoc-31.1-win64.zip`](https://github.com/protocolbuffers/protobuf/releases/tag/v31.1)，解压后把 `bin` 加入 `PATH`；再安装 LLVM、NASM、CMake、Perl：
> ```powershell
> winget install LLVM.LLVM
> winget install NASM.NASM
> winget install Kitware.CMake
> winget install StrawberryPerl.StrawberryPerl
> [Environment]::SetEnvironmentVariable("PROTOC", "C:\path\to\protoc\bin\protoc.exe", "User")
> [Environment]::SetEnvironmentVariable("LIBCLANG_PATH", "$env:ProgramFiles\LLVM\bin", "User")
> ```
> **Linux (apt)：** `sudo apt-get install -y protobuf-compiler clang libclang-dev cmake nasm perl`
> **macOS (brew)：** `brew install protobuf llvm cmake nasm`（Perl 系统自带）

### 克隆后启动指南

#### 1. 安装 Rust 工具链

```bash
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04
```

#### 2. 安装必要的 Cargo 工具

```bash
cargo install cargo-watch
# sqlx-cli 必须锁定 0.8.x：最新的 0.9.0 需要 rustc ≥ 1.94，但本项目锁定
# nightly-2025-12-04（rustc 1.93），不指定版本直接安装会失败。
cargo install sqlx-cli --version 0.8.6 --no-default-features --features rustls,sqlite
```

#### 3. 安装 Node.js 依赖

```bash
pnpm install
```

#### 4. 设置环境变量

**Linux / macOS：**

```bash
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"  # 必须恰好 32 个字符
```

**Windows PowerShell：**

```powershell
$env:SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
```

#### 5. 初始化数据库

```bash
pnpm run prepare-db
```

#### 6. 启动开发服务器

```bash
pnpm run dev
```

此命令同时启动后端（Rust/Axum）和前端（Vite/React）开发服务器。

- **前端地址：** http://localhost:23457
- **后端 API：** http://localhost:23456/api

首次启动时，**设置向导**会引导你完成环境检测、AI 模型配置和项目绑定。

#### 7. （可选）生产构建

```bash
# 构建后端
cargo build --release -p server

# 构建前端
cd frontend && pnpm build && cd ..

# 设置加密密钥并运行
export SOLODAWN_ENCRYPTION_KEY="你的32位加密密钥"
./target/release/server
```

生产模式下，前端和 API 在同一端口提供服务：http://localhost:23456

### ⚠️ 常见坑

以下问题在首次配置时最容易踩到（尤其是 Windows）：

- **`protoc` 和 `libclang` 是构建必需项，但 `scripts/setup-windows.ps1` 不会安装。** 缺少 `protoc` 时，`crates/services`、`crates/runner`、`crates/feishu-connector` 无法构建（lockfile 中没有内置的 protoc）；缺少 `libclang` 时，`libsqlite3-sys` 会在 bindgen 运行时报错（由 sqlx 的 `sqlite-preupdate-hook` 特性触发）。安装命令见[前置要求](#前置要求)。
- **`sqlx-cli` 必须锁定 0.8.x。** 最新的 0.9.0 需要 rustc ≥ 1.94，但项目锁定的 `nightly-2025-12-04` 是 rustc 1.93，不指定版本直接 `cargo install sqlx-cli` 会失败。
- **构建不需要数据库。** `.cargo/config.toml` 设置了 `SQLX_OFFLINE=true`，构建会使用已提交的 `crates/db/.sqlx/` 查询缓存。只有在修改 SQL 查询或迁移时，才需要 `sqlx-cli` / `pnpm run prepare-db`。
- **Windows：安装完工具后请重启终端**，以便加载更新后的 `PATH`、`PROTOC` 和 `LIBCLANG_PATH`。
- **x86-64 本地构建需要 `cmake`、`nasm`、`perl`。** 飞书连接器迁移到 `openlark` SDK 后，依赖树改用 `aws-lc-rs`（AWS-LC）而非 `ring`；其 `aws-lc-sys` 会从源码编译 AWS-LC 的优化汇编（需要 `nasm` + `cmake`，部分平台还需 `perl`）。安装命令见[前置要求](#前置要求)。（`libgit2-sys` 仍通过 `cc` crate 构建。）

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
