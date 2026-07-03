<p align="center">
  <img src="installer/assets/solodawn.png" alt="SoloDawn" width="120" />
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

SoloDawn 是一个跑在你本机的开源 Web 应用（Rust 后端 + React 前端）：由一个上层编排 Agent（主 Agent）指挥你电脑上**真实安装的 AI CLI**（Claude Code、Codex 等 8 种），在 Git 仓库里完成全自动开发——需求澄清 → 生成技术规范 → 拆分任务 → 多分支并行开发 → 三层质量门拦截 → 验收评审打分 → 自动合并。

SoloDawn 的最终设计目标是**通过社交平台的简单对话，完成复杂项目的产出**——不是那种玩具 Demo，而是真正的复杂化的生产级产品。

> **一句话总结：** 不管你是不是程序员，只需要提需求，剩下的全是自动的。

> **关于 AI 幻觉，先说诚实话：** SoloDawn 无法让模型在输出 token 的那一刻不产生幻觉——没人能做到。它通过架构与工作流设计，在成品交付前把幻觉产物拦下并修掉：31 条内置质量规则 + 三层质量门 + 自愈循环 + 90 分验收线，让幻觉、安全漏洞、集成冲突在不同维度各被拦一遍。原理见[质量体系详解](#质量体系详解)。

---

## 两大核心亮点

### 亮点一：AI 全自动开发

编排工作区采用**两层 Agent 架构**：主 Agent + 子 Agent——不是三层，也没有任何预配置的子 Agent。所有子 Agent 都由主 Agent 动态创建、动态关闭：工作需要更多人手时（追加任务、代码评审、缺陷修复、集成修复），主 Agent 就会继续创建新的子 Agent，需要多少开多少，用完即关。这套设计已完整实现。

为什么放弃固定的工作流定义？因为僵化的工作流会引入硬性约束：简单任务用它过于繁琐，复杂任务用它容量不足，而工作流内的条件判断逻辑必然产生误判。所以两层架构把完整决策权交给主 Agent，由它自主决定开什么、开多少、何时关。

现在用 AI 写代码有个根本矛盾：程序员得自己搭工作流、配 skill、接 MCP、写计划、理文档，本质还是人在驱动；非程序员连需求都没法转成技术规范。SoloDawn 把驱动权从人手里接了过来：

| 你是谁 | 你做什么 | 系统做什么 |
|---|---|---|
| **程序员** | 把精确的任务目标扔进去 | 你的输入直接作为技术规范执行，不追问。不用发指令、不用点"继续"，等验收就行 |
| **非程序员** | 用大白话说需求 | 系统用大白话追问补全模糊点 → 后台自动生成技术规范 + 验收评分规范 → 开始全自动开发 |

交付不是"跑完就算"：每次提交都要过质量门，每个任务完成时都按照**从你的需求生成的评分规范**逐项打分，**不到 90 分自动打回重做**——评分规范在整个工作流中持续生效，确保最终交付符合你最初的需求。

**一人公司的最佳伙伴：** 用上这个项目，你几乎相当于拥有了一个专业的开发团队。

### 亮点二：手动工作流与 AI 全自动开发，都原生支持你的 Skill / MCP / 插件

无论编排工作区（全自动）还是手动工作流，SoloDawn 启动的都是你电脑上**真实安装的 CLI 进程**（原生 PTY，不是 API 二次封装），提示词原样直达 CLI，零拦截、零改写。你原本配置了哪些工具，这里就能用哪些工具——而且继承的不只是你的工具：**CLI 的官方内置命令同样全部继承，包括 UltraCode 模式**。

- **skill、插件、MCP 服务器、斜杠命令零迁移** —— 不需要在 SoloDawn 里额外设置，直接继承、原样可用，你可以随你心意去做、去改；
- **官方内置命令全部继承（含 UltraCode）** —— 在手动工作流里为任务配置专用提示词即可启用 UltraCode：它会生成标准化的工作流脚本，为每个 Agent 硬编码清晰的能力边界；之后直接调用这个脚本，就能复用整套工作流；
- **全自动可以，手动掌控也可以** —— 编排工作区把一切交给 AI；手动工作流让你自定义工作流图的每个细节：几个终端、什么角色、哪个模型、启用哪些斜杠命令；
- **8 种 AI CLI 在同一个工作流里协作** —— 比如 Claude Code 终端跑 GLM 模型当开发，Codex 终端跑 GPT 模型当审计。

**这个项目的可玩性和拓展性非常强，可探索的空间近乎无限：8 种 AI CLI × 你的 skill × 你的 MCP × 各种插件自由组合，10 个用户能玩出 100 种用法——这对所有用户都是关键优势。**

两种模式的逐步操作说明见文末：[编排工作区使用指南](#编排工作区使用指南) · [手动工作流使用指南](#手动工作流使用指南)。

---

## 功能总览

### 编排与执行
- ✅ **两层 Agent 架构**：主 Agent 拥有完整决策权，子 Agent 零预配置——全部由主 Agent 按需动态创建与关闭，评审 / 修复 / 追加任务需要更多人手时随时增开
- ✅ 主 Agent（编排 Agent）指挥完整工作流生命周期；Git 驱动事件循环，比轮询节省 98%+ token
- ✅ 两种工作模式：编排工作区（Agent-Planned 全自动）/ 手动工作流（DIY）
- ✅ 编排工作区双子模式：直接执行（精确输入）/ 引导对话（大白话追问澄清）
- ✅ 多任务并行执行：每任务独立 Git 分支 + 隔离 worktree，默认最多 10 个工作流并发
- ✅ Planning Draft 生命周期：gathering → spec_ready → confirmed → materialized
- ✅ **多轮继续开发**（1.0 后落地）：已交付的对话可原地开下一轮——新一轮只规划增量，旧轮次折叠进同一会话，每个项目同时只跑一轮
- ✅ **评分点账本**：验收标准登记为项目级评分点（`RP-001`…），评分即结算——已交付点写入压缩上下文纸条，回退逐点标记
- ✅ **架构感知规划**（1.0 后落地）：一份架构思维清单 + 从本地同步知识库按关键词匹配出的参考架构摘要（内置源：awesome-architecture），在轮次物化时注入编排器；知识源是用户可扩展的 GitHub 仓库，后台自动刷新
- ✅ **设计风格**（1.0 后落地）：在工作区按轮选择视觉方向，或在系统设置里设全局默认——内置 6 套改编自高分开源设计 skill 的预设，另支持自定义风格的完整增删改；所选风格会带入每一条 UI 相关的终端指令
- ✅ 跨终端上下文传递（前序终端的成果传给下一个）
- ✅ 自动分支合并 + 指定冲突解决终端；可选"合并前跑测试""冲突时暂停"

### 质量与可靠性
- ✅ **三层质量门**：终端级（每次提交，16 条阻断条件）→ 分支级（任务完成，18 条）→ 仓库级（合并前，23 条）
- ✅ 内置规则引擎 **31 条规则**（Rust 13 / TS 11 / 通用 7），零依赖运行，可选接入 SonarQube
- ✅ 四种执行模式 off / shadow / warn / enforce（1.0 出厂为 enforce），11 个分析器独立开关
- ✅ 自愈循环：门禁失败 → 结构化修复指令回传 → 终端自修重提 → 自动重检（单终端停滞恢复最多 10 次）
- ✅ 验收评审：按项目生成评分规范，90 分通过线（地基任务 70 分），最多 5 轮自动打回
- ✅ 自定义质量规则 + AI 自然语言生成规则（对抗校验 / 样本实测 / 人工确认）
- ✅ 密钥泄露检测（11 类模式）、弱默认凭证检测、测试真实性检查（专抓假测试/不写测试）
- ✅ 策略快照与问题追踪；LLM 容错降级；多提供商熔断与故障转移；状态持久化与崩溃恢复

### CLI 与模型
- ✅ 8 种 AI CLI；同一任务内可混用不同 CLI，各终端可担任开发 / 审计等不同角色
- ✅ 同一 CLI 内通过 CC-Switch 切换不同供应商/模型
- ✅ 五类模型接口：Anthropic / Google / OpenAI / Anthropic 兼容 / OpenAI 兼容（自定义 base URL，中转可用）
- ✅ 每终端独立环境变量注入；MCP 服务器按 CLI 自适应配置格式
- ✅ Claude Code 计费保证：订阅用户只消耗套餐额度（见文末[计费保证详解](#claude-code无--p-交互式传输与计费保证)）

### 体验与集成
- ✅ 网页伪终端（xterm.js + 原生 PTY），实时调试与交互
- ✅ 斜杠命令：6 个内置预设 + 自定义命令，统一交给主 Agent 识别并转发给对应终端，适配第三方插件
- ✅ CLI 官方内置命令全部继承（含 UltraCode 模式）：手动工作流中以专用提示词启用，生成可复用、能力边界清晰的标准化工作流脚本
- ✅ Setup Wizard 首次运行引导；运行环境与已装 CLI 自动检测
- ✅ 国际化 6 种语言（English、简体中文、繁體中文、日本語、Español、한국어）
- ✅ Telegram 连接器；飞书长连接 WebSocket 连接器（1.0 未重测，见[当前局限与路线图](#当前局限与路线图)）

### 部署与运维
- ✅ Docker 一键部署（交互式安装脚本）；拆分部署架构（Server + Runner + Redis，gRPC 通信）
- ✅ 提供商健康监控 API；Sentry 错误追踪 + PostHog 分析
- ✅ AES-256-GCM 加密 API 密钥静态存储

---

## 工作原理

四条核心设计理念：

- **上层编排，不生成代码。** 编排 Agent 不写任何代码——它指挥最强的专业 AI CLI（Claude Code、Codex、Amp、Cursor Agent 等）去完成工作。
- **两层 Agent，拒绝固定工作流。** 只有主 Agent 和子 Agent 两层，没有预配置的子 Agent，也没有固定的工作流定义——僵化工作流对简单任务过重、对复杂任务不足，条件判断必然误判。完整决策权在主 Agent 手里，子 Agent 按需动态开、动态关。
- **非侵入式设计。** SoloDawn 不替换任何 CLI，不修改任何配置，不定义新工具。它继承每个 CLI 的完整原生生态——所有斜杠命令、插件、skill 和 MCP 服务器都原样可用。
- **Git 驱动的事件循环。** 编排器只在 Git 提交事件发生时消耗 LLM token，事件间休眠零消耗——相比轮询方案节省 98% 以上 token。

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

**两层 Agent 架构（编排工作区）：**

- **主 Agent（编排 Agent）** → 每个工作流一个，拥有完整决策权：拆解规范、动态创建/启动/关闭子 Agent、解析 Git 事件、路由评审与修复循环
- **子 Agent** → 零预配置，全部由主 Agent 在运行时按需创建与终止；每个子 Agent 落地为一个终端——你机器上的原生 AI CLI 进程（PTY）。运行中随时可增开：代码评审、缺陷修复、集成修复、追加任务都会催生新的子 Agent（并发受全局终端上限约束，超出的排队依次执行）

**Git 侧执行结构：**

- **工作流（Workflow）** → 主 Agent 管理整个生命周期（默认最多 10 个工作流并发）
- **任务（Task）** → 独立 Git 分支（`workflow/{工作流id}/{任务名}`）+ 隔离 worktree，与其他任务并行
- **终端（Terminal）** → 子 Agent 的运行载体，在任务内串行执行，受质量门管控

**一段代码从生成到进入主分支要过四道关：** 每次提交过终端门（变更文件级）→ 任务完成过分支门（分支级）→ 验收评审按评分规范打分（≥ 90 放行，否则打回自修）→ 合并前过仓库门（全仓库级）。任何一关失败，结构化修复指令自动回传给同一个终端，修完重检，无需人工。细节见[质量体系详解](#质量体系详解)与[验收评审与评分规范](#验收评审与评分规范)。

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
| Codex | ✅ 已支持 | ✅ 通过 CC-Switch | Codex 适配器 |
| Amp | ✅ 已支持 | — | Passthrough |
| Cursor Agent | ✅ 已支持 | — | Cursor 适配器 |
| Qwen Code | ✅ 已支持 | — | — |
| GitHub Copilot | ✅ 已支持 | — | Copilot 适配器 |
| Droid | ✅ 已支持 | — | Passthrough |
| Opencode | ✅ 已支持 | — | Opencode 适配器 |

> Gemini CLI 支持已移除：Google 已停止面向消费者提供 Gemini CLI，转向 [Antigravity CLI](https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/)。

任何能在终端运行且支持斜杠命令的 CLI 都可以集成。

> 1.0 的 48 小时验收全程使用 Claude Code；其余 CLI 与多 CLI 协同的测试状态见[当前局限与路线图](#当前局限与路线图)。Claude Code 订阅用户务必了解[计费保证](#claude-code无--p-交互式传输与计费保证)。

---

## V1.0 实测结果

SoloDawn V1.0 经过一次**48 小时全自动、自修复的端到端测试**验证（2026-06-27 → 06-30）：通过浏览器 UI 串行执行 7 个真实任务，Stop-hook 驱动器在运行过程中**自行诊断并修复**编排器根因——全程零人工介入。

这 7 个任务覆盖五种开发形态——**从零新建、既有库扩展、跨语言迁移、祖传重构、安全/性能/监控加固**——完成了全链路验证，基本覆盖 Web/服务端程序员的核心工作场景。

**最终评级：88.85 / A — 7 个任务全部交付。**

| # | 任务 | 开发形态 | 仓库 | 评分 | 评级 |
|---|------|---------|------|:---:|:---:|
| 1 | 知识库应用 | 从零新建 | `knowledge-base-demo` | 81 | B |
| 2 | Hoppscotch 负载测试模块 | 既有库扩展 | `hoppscotch-demo` | 88 | A |
| 3 | Express → Rust 重写 | 跨语言迁移 | `express-to-rust-demo` | 93 | A |
| 4 | 重构 + 测试补齐 | 祖传重构 | `refactor-test-demo` | 92 | A |
| 5 | 微服务电商 | 从零新建 | `ecommerce-demo` | 91 | A |
| 6 | 安全 + 性能 + 监控 | 加固 | `kutt-security-demo` | 86 | A |
| 7 | 飞书备忘录应用 | 从零新建 | `web-memo-demo` | 91 | A |

评分维度：可构建性(20) / 功能完整性(25) / 代码质量(30) / 测试质量(15) / 工程化(10)，按任务复杂度加权。评级：S≥95 · A≥85 · B≥70。

测试本身也是对 SoloDawn 自愈能力的压力测试。48 小时运行中暴露并修复了 **21 个编排器死锁/停滞根因**（"§8" 修复链 #1–#21）——每一个都是朴素实现会永远卡死的场景。每个修复都部署后当场重新验证才继续。完整报告：`docs/undeveloped/current/V1.0-质量验收-2026-06-30-48h自修复测试.md`。

> 模型：`glm-5.2[1m]` via solodawn.cloud（Anthropic 协议）。测试方式：浏览器 MCP UI、串行执行、/goal + Stop-hook 自修复。

---

## 快速开始

### 🤖 给 AI 的提示词 —— "帮我跑起来"

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
6. 设置向导出现后，就到此为止，把模型配置交给我——AI 模型由我自己在界面里完成配置。

如果某步构建失败，读取错误、补装缺失的前置项、重试。除非某步确实必要，否则不要修改 SoloDawn 自己的源码文件。
```

**运行 vs 开发 —— 你是哪一种？**

| | 运行 SoloDawn（使用它） | 开发 SoloDawn（改它的代码） |
|---|---|---|
| 你做什么 | `git clone` → `pnpm run dev` → 打开网页 UI | 左边全部 + 编辑 Rust/TS 源码 |
| 前置项 | 下面的构建工具链（后端只需编译一次） | 额外需要 `sqlx-cli` 0.8.6、lint 工具、完整测试工具链 |
| 缓存占用 | 适中的 `target/`（仅 server 二进制 + 依赖） | 较大的 `target/`（全工作空间 test/clippy/codegen） |
| 参考 | 本节其余内容 | [贡献](#贡献) |

> **1.0 以源码方式发布** —— **不提供任何形式的安装包**。克隆后用 `pnpm run dev` 运行（或自行构建 release 二进制）。`installer/` 目录与 `Build Windows Installer` 工作流为早期开发的历史保留，1.0 不发布 Windows 安装包。

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

> 未设置时，SoloDawn 会自动生成并复用一把本机持久密钥文件（`~/.enckey`）。单机使用可以依赖它；多主机或容器部署请务必显式设置，否则加密数据无法跨机解密。

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
- **健康检查：** `GET /healthz`（存活）· `GET /readyz`（就绪，含数据库/目录/飞书状态）

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

> ⚠️ release 二进制默认**要求设置 `SOLODAWN_API_TOKEN`**（API 鉴权，防止未授权访问），缺失会拒绝启动；仅本机使用可设 `SOLODAWN_LOCAL_MODE=1` 跳过该检查。

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

> **📖 以下为补充资料** —— 两种模式的逐步使用指南、质量体系与验收评审的完整拆解、以及配置参考。建议读完上面的核心部分后按需查阅。

---

## 编排工作区使用指南

一句话流程：配好模型 → 绑定 Git 仓库 →（可选）调质量门 → 说需求 → 点右上角确认 → 等交付。

**第 0 步：配置模型。** 首次启动会进入设置向导（之后也可随时在设置页修改），正式开始工作前必须先配好模型。两种方式：

- **原生模式**：自动检测你本机 Claude Code 的订阅登录（Pro/Max 均可），零配置直接用，并保证只消耗订阅额度（见[计费保证](#claude-code无--p-交互式传输与计费保证)）；
- **手动模式**：填 API Key，支持 **Anthropic / Google / OpenAI / Anthropic 兼容 / OpenAI 兼容** 五类接口（兼容类可自定义 base URL，中转端点可用）。所有 Key 以 AES-256-GCM 加密落库。

**第 1 步：选择仓库。** 界面会自动读取本机已有的 Git 仓库；若没有，也可以让它替你创建。选完需要**二次点击确认**，提示"已绑定"才算完成。项目必须运行在 Git 仓库中——任务分支、worktree 隔离、质量门、自动合并全部建立在 Git 之上。

**第 2 步（可选）：自定义质量门。** 在此界面可以打开"质量门规则"面板：四种执行模式、三层门的每条阻断条件（指标 + 阈值）、11 个分析器开关都可增删改，还可以用 AI 从自然语言直接生成规则。看不懂、一头雾水没关系——**建议：没有特殊需求就别动**，默认配置已覆盖大部分场景；AI 生成的规则模式可能过宽或过窄，最终产出未必好于默认。

**第 3 步：说需求。** 在对话界面选好本次使用的终端与模型（Claude Code、Codex 等），然后按你的身份来：

- **程序员**：直接扔一个非常准确的任务目标。系统把你的输入直接当技术规范用，不追问，只在后台生成验收评分规范。
- **非技术人员**：别用任何技术术语，用大白话从用户角度说需求——"我想做一个本地备忘录，我希望在浏览器里看它；或者它是一个软件，显示在桌面上、能置顶……"系统会用大白话追问补全模糊点（比如你说了 3 个功能，它可能追问"还有两个要不要加？"），最终需求确认后自动生成技术规范 + 验收评分规范。

要做带界面的东西？在模型选择器旁边的工具栏里挑一个**设计风格**——不挑就走系统设置 → 设计风格里的全局默认（见[架构知识与设计风格](#架构知识与设计风格)）。

想用自己的验收标准？在确认**之前**上传审计文档（Builtin / Merged / Custom 三种模式，见[验收评审与评分规范](#验收评审与评分规范)）；确认后评分规范面板变为只读。

**第 4 步：点右上角的确认。** 确认分两步：先确认技术规范（此刻生成并锁定评分规范），再确认质量门配置。生成的评分规范会立刻以卡片形式出现在对话里——每条验收标准都带着自己的评分点编号（`RP-001`…）——需求清单也会作为侧边栏出现在审计文档面板旁边。**对话完成后记得点确认，不点不会执行**——这是代码层的硬性拦截：两步确认都完成后，工作流才会物化并自动启动。

**第 5 步：等交付。** 之后全部自动：主 Agent 把规范拆成任务、按需创建并启动子 Agent（没有任何预配置——评审、修复、集成需要更多人手时它会继续开新的，用完即关）、开分支、并行推进；每次提交过终端门、任务完成过分支门、验收评审 ≥ 90 分放行（不足自动打回，最多 5 轮）；全部完成后自动合并回主分支，冲突交给你指定的合并终端解决，也可以配置"遇冲突暂停"人工介入。

**第 6 步：继续迭代——多轮。** 交付不再是对话的终点：本轮工作流结束后，同一会话里会出现**继续需求**按钮。新一轮只澄清新需求——规划师从评分点账本（已交付点 + 上下文纸条）出发，不再重新读一遍项目；之前交付的功能由新评分规范里的回归断言保护；每个项目同时只跑一轮。旧轮次以可折叠分隔线的形式留在会话里，连同各自的评分规范快照。

## 手动工作流使用指南

面向想完全掌控工作流图的用户，创建向导共 7 步：

1. **选择仓库**（Step 0）：自动检测本机 Git 仓库；显示"未检测到"没事，点一下刷新就好。
2. **基本信息**（Step 1）：填写这个工作流的名称、描述，以及本次打算并行跑几个任务。
3. **定义任务**（Step 2）：把要做的事拆成任务；每个任务将获得独立分支与 worktree。
4. **配置模型**（Step 3）：维护模型库（供应商 / base URL / Key）。全局设置里配过的这里会直接读取到；也可以只为本工作流单独配置。
5. **配置终端**（Step 4）：页面会检测你电脑上的运行环境与已安装的 AI CLI（在页面里往下滚动即可看到）。然后为每个任务选择：用哪个终端（CLI）、这个终端用什么模型、以及此终端的角色描述。**多 AI CLI 协作就在这一步实现**——比如 Claude Code 终端跑 GLM 模型作为开发人员，Codex 终端跑 GPT 模型作为审计人员。
6. **斜杠命令**（Step 5）：为工作流启用斜杠命令。在 Agent Planned 模式下，命令不会直接发送给对应终端，而是统一交给主 Agent——由主 Agent 识别命令，并在运行过程中自行转发给合适的终端（整个工作流本就由主 Agent 全程掌控）。在 DIY 模式下没有主 Agent——每条启用的命令会自动渲染并输入到每个任务的终端里，紧跟在该任务的描述之后。内置 write-code / review / fix-issues / test / refactor / document 六个预设，也可以加入你自己的命令——插件市场那么多插件，每个都有自己的命令，把你的命令加到这里，主 Agent 就能在流程中调用你的插件。
7. **高级设置**（Step 6）：选择谁当协调多任务的主 AI（编排模型），以及合并分支的时候，使用哪个终端、哪个模型来解决冲突、完成合并；还可以配置合并前是否跑测试、遇到冲突是否暂停。

> **进阶玩法：UltraCode。** 手动工作流启动的是你的原生终端，因此除了你的 skill / MCP / 插件，CLI 的官方内置命令也全部继承——包括 UltraCode 模式。为任务配置专用提示词即可启用（任务描述会原样输入到该任务的终端）：UltraCode 会生成标准化的工作流脚本，为每个 Agent 硬编码清晰的能力边界，之后直接调用该脚本即可复用整套工作流。

手动工作流视频演示（2026 年 2 月录制，当时项目还叫 GitCortex，仅演示了手动工作流，界面与现版本已有差异）：[GitCortex 最小 MVP 演示视频 - bilibili](https://www.bilibili.com/video/BV1yxfMBCEFh/)

## 架构知识与设计风格

两项影响编排工作区"怎么规划、怎么做"的新增能力。

### 架构感知规划

轮次物化时，规划目标会追加一段**架构指引（Architecture Guidance）**，由两部分组成：

- **自答式架构思维清单**（改编自 [study8677/architecture-copilot](https://github.com/study8677/architecture-copilot)，MIT）：系统边界 → 数据模型 → 同步/异步流 → 容量诚实。编排器在拆解任务时逐项作答，让方案把架构假设摆到明面上，而不是藏在实现里。
- **匹配到的参考架构摘要**：SoloDawn 维护一个从 GitHub 同步的本地知识库——内置源是 [study8677/awesome-architecture](https://github.com/study8677/awesome-architecture)（MIT），一组结构统一的参考架构模板。确认时用需求文本对条目做关键词匹配，把得分最高的摘要（关键决策 / 权衡 / 规模化 / 反模式）附进去。

**系统设置 → 架构知识**里可以开关指引、把你自己的 GitHub 仓库加为知识源（按路径前缀同步其中的 markdown 文件）、手动触发同步、查看每个源的同步状态。后台同步约每 6 小时检查一次，超过 24 小时未同步的源会自动刷新；只拉取有变化的文件（blob-SHA 差量）。设置 `SOLODAWN_GITHUB_TOKEN`（或 `GITHUB_TOKEN`）可提高 GitHub API 速率上限。

### 设计风格

在工作区对话工具栏里按轮选择**设计风格**，或在**系统设置 → 设计风格**里设全局默认。带风格的轮次物化时，风格指令会以**设计方向（Design Direction）**段落追加进目标，且编排器契约要求把它带进**每一条 UI 相关的终端指令**——包括铺设基础样式的地基任务，保证并行终端之间视觉语言一致。

内置 6 套预设，浓缩自高分开源设计 skill（每个文件都带来源与许可证署名，详见 `LICENSE`）：

| 预设 | 改编自 | 许可证 |
|---|---|---|
| Anthropic Frontend Design | anthropics/skills — frontend-design | Apache-2.0 |
| Minimalist Editorial | Leonxlnx/taste-skill — minimalist-ui | MIT |
| Industrial Brutalist | Leonxlnx/taste-skill — industrial-brutalist-ui | MIT |
| Soft Premium | Leonxlnx/taste-skill — soft-skill | MIT |
| Impeccable Design Language | pbakaus/impeccable | Apache-2.0 |
| Emil Design Engineering | emilkowalski/skills — emil-design-eng | MIT |

内置预设只读——复制一份即可改成自己的；自定义风格支持完整的新建 / 编辑 / 删除，停用的风格不会注入。

## 质量体系详解

核心原则：**在尽可能早的时刻、以尽可能小的范围解决错误**。提交时查改动文件（终端门），任务完成时查整条分支（分支门），合并前查全仓库（仓库门）——幻觉、安全漏洞、集成冲突在不同维度各被拦一遍，再结合自愈循环，把问题消灭在进入主分支之前。

### 内置规则引擎（31 条规则，零依赖运行）

规则引擎参考 SonarQube 的质量模型实现（质量门模型移植自 SonarQube，LGPL-3.0），**无需安装 SonarQube 即可运行**；仓库门可选接入真实 SonarQube 做深度分析。31 条内置规则按语言分三组：

| 分组 | 数量 | 覆盖内容 |
|---|---|---|
| **Rust** | 13 | 圈复杂度、认知复杂度、函数/文件长度、嵌套深度、错误处理（测试外的 `unwrap`/`expect`/`panic!`）、`unsafe` 使用、`clone` 滥用、命名规范、公共项缺文档、类型复杂度、TODO 残留、魔法数字 |
| **TypeScript / JavaScript** | 11 | 圈复杂度、函数/文件长度、嵌套深度、`any` 滥用、`as` 类型断言、`console` 残留、命名规范、React Hooks 误用、import 顺序、TODO 残留 |
| **语言无关** | 7 | 代码重复、硬编码密钥检测（AWS / GitHub / Slack / Google / Stripe / 数据库连接串 / npm token 等 11 类模式）、弱默认凭证（`admin` / `password` / `changeme` / `123456` / 弱 JWT_SECRET）、超大文件、超长行、行尾空白、文件编码 |

严重级别四档：**Blocker / Critical / Major / Minor**。硬编码密钥、弱密码、绕过类型（`any` / `as` / `unsafe`）、面条代码（复杂度 / 嵌套）这些经典的 AI 幻觉产物全部在覆盖范围内。

规则引擎之外，质量门还直接驱动一批独立分析器：`cargo check` / `clippy` / `tsc` / 测试运行、覆盖率统计、密钥扫描、ReDoS 风险检测、**测试真实性检查**（专抓"不写测试""假测试""空壳测试"）、项目约定检查、运行时安全气味等。

### 三层质量门

| 质量门 | 触发时机 | 检查范围 | 阻断条件 |
|--------|---------|---------|---------|
| **终端门** | 每次 checkpoint 提交 | 仅变更文件 | 16 条 —— 编译/类型/测试错误清零、内置规则 Critical 清零、密钥泄露、缺测试文件、假测试、ReDoS 风险等 |
| **分支门** | 任务最后一个终端完成 | 整条任务分支 | 18 条 —— 终端门全部 + clippy 警告、格式检查、行覆盖率 ≥ 60%、圈复杂度 ≤ 25、重复块 ≤ 5、TODO 密度受控 |
| **仓库门** | 合并主分支前 / CI | 整个仓库 | 23 条 —— 分支门全面收紧 + SonarQube Blocker/Critical 清零、行覆盖率 ≥ 80%、重复块 ≤ 3 |

**四种执行模式：** `off` → `shadow`（只记录不阻断）→ `warn`（回流问题但不阻断）→ `enforce`（硬门禁）。**1.0 出厂配置即 `enforce`。** 11 个分析器（Rust / 前端 / 仓库 / 安全 / Sonar / 内置规则 ×3 / 覆盖率 / 完备性 / 交付就绪）可独立开关。

### 自愈循环

质量门失败时，结构化修复指令自动回传给**同一个终端**：终端修复 → 重新提交 → 质量门自动重跑，全程无需人工干预。为防止死循环设有硬上限：单终端停滞恢复最多 **10 次**；验收评审最多打回 **5 轮**，超过转人工处理。

### 自定义规则与 AI 生成规则

三层门的每条阻断条件（指标、运算符、阈值）都可以在 UI 里增删改，还可以用**自然语言让 AI 生成自定义规则**——生成的规则要经过对抗校验、样本实测、人工确认多道关卡才会生效，且以纯声明式数据（而非可执行代码）落库执行。

> **建议：没有特殊需求就别动默认规则。** 默认配置已覆盖大部分场景；AI 生成的规则模式可能过宽或过窄，最终产出未必好于默认。

## 验收评审与评分规范

质量门拦的是**机械错误**（编译、lint、复杂度、密钥）。但"这个任务到底有没有交付我要的东西？"需要更深的判断。SoloDawn 用**基于评分规范的 LLM 验收评审**来回答。

### 为什么是"按项目生成"，而不是一套固定规则？

开发期间曾直接内置一套固定评分规则，结果 7 个实测任务玩起了跷跷板：A 的评分上去了，B 的就下去；B 上去了，A 又下来——蹉跎一个月没有调平。最终方案是：**内置的只是评分原则，每个项目再根据你的需求文档生成专属的评分规范**——10 个项目，10 套规则。这样做才让每个任务都达了标。

### 评分规范根据你的技术规范生成

技术规范确认的那一刻，SoloDawn 会生成一份 **AuditPlan（评分规范）**——用 LLM 把内置的 100 分制 5 维度审计原则针对**你这个具体项目**裁剪定制：

| 维度 | 满分 |
|---|---|
| 可构建性 | 20 |
| 功能完整性 | 25 |
| 代码质量（架构 / 规范 / 安全）| 30 |
| 测试质量 | 15 |
| 工程化与文档 | 10 |

你可以**上传自己的审计文档**来主导评分规范，三种模式：

| 模式 | 评分规范采用 |
|---|---|
| **Builtin**（默认）| 内置原则，按你的 spec 裁剪 |
| **Merged** | 你的文档 **+** 内置；你的文档优先 |
| **Custom** | 只用你的文档 |

> 评分规范在你**确认技术规范的那一刻生成**。想影响它，就在确认**之前**上传审计文档——确认后面板变为只读。

### 每个任务都按评分规范打分

任务完成时，验收评审 LLM 按评分规范逐维度给交付的代码打分，并引用文件/行号作为证据。

- **≥ 90 分（通过线）→ APPROVED → 任务放行**
- **< 90 分 → REJECTED → 修复指令打回 → 终端自修重提**（自愈循环，最多 5 轮）
- 触发**否决规则**（需求缺失、安全风险、假测试等）直接判 **0 分**
- 分阶段项目的地基/脚手架任务采用独立通过线（**70 分**），避免用功能完整性标准误杀纯搭建型任务

这正是最终交付可信的原因：每个任务都必须**依据从你的需求推导出的标准**证明自己达标——而不只是"能编译通过"。

### 评分点与上下文纸条

评分规范里的功能完整性条目同时构成一份**项目级的评分点账本**，它的生命周期比单次交付更长：

- **确认时登记** —— 每条验收标准登记为一个评分点，由服务端分配稳定编号（`RP-001`…）。对话里的评分规范卡片和需求清单侧边栏都能看到；开工前，仍处于待交付状态的点可以在面板里编辑或删除。
- **评分即结算** —— 验收评审逐点结算账本：交付的点写入一张**压缩上下文纸条**——做了什么、代码在哪、关键决策与坑、怎么扩展。纸条是有字数上限的指针，绝不复制代码——仓库本身才是唯一事实源。
- **后续轮次** —— 规划师和编排器直接拿到评分点索引 + 纸条作为背景，新一轮只规划增量，不再从零理解项目；已交付的点进入新评分规范时只作回归断言（验证没被破坏），评审判红则逐点标记为回退。

账本的所有操作均为 fail-open：即使它出了问题，确认 / 物化 / 评审也照常进行，绝不被阻塞。

## Claude Code：无 `-p` 交互式传输与计费保证

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

## 质量门配置

在 `quality/quality-gate.yaml` 中配置：

```yaml
mode: enforce  # off | shadow | warn | enforce —— 1.0 出厂默认 enforce
```

| 模式 | 行为 |
|------|------|
| `off` | 关闭质量门 |
| `shadow` | 运行分析并记录结果，但不阻断（观察期） |
| `warn` | 运行分析，回流问题到终端，不硬性阻断 |
| `enforce` | 硬性门禁 — 不通过则阻断（1.0 出厂默认） |

同一文件还定义了三层门的全部阻断条件（终端 16 / 分支 18 / 仓库 23 条，指标 + 运算符 + 阈值）与 11 个分析器开关。编排工作区中绑定仓库后，也可以在"质量门规则"面板里可视化修改（含 AI 生成规则）；云端 CI 中仓库门以 shadow 模式运行。

```bash
# 手动运行质量门
pnpm run quality

# 试运行检查
pnpm run quality:check
```

## 配置参考（环境变量）

| 环境变量 | 作用 | 说明 |
|---|---|---|
| `SOLODAWN_ENCRYPTION_KEY` | 敏感数据的 AES-256-GCM 主密钥 | 必须恰好 32 字符；未设置时退回本机持久密钥文件 `~/.enckey`（自动生成并复用）；多机/容器部署必须显式设置 |
| `SOLODAWN_API_TOKEN` | API 鉴权令牌 | release 模式必填（`SOLODAWN_LOCAL_MODE=1` 时豁免） |
| `SOLODAWN_LOCAL_MODE` | 本机模式 | 跳过 API token 校验，仅限 localhost 部署 |
| `SOLODAWN_FEISHU_ENABLED` | 飞书连接器开关 | 优先级高于数据库中的设置 |
| `SOLODAWN_NO_BROWSER` | 启动时不自动打开浏览器 | — |
| `BACKEND_PORT` / `FRONTEND_PORT` | 端口覆盖 | 默认 23456 / 23457 |
| `SOLODAWN_ASSET_DIR` / `SOLODAWN_TEMP_DIR` / `SOLODAWN_ENC_KEY_FILE` | 数据 / 临时 / 密钥文件路径覆盖 | 默认按平台约定 |

**运行数据位置**（SQLite 数据库、`config.json`、凭证等）：开发模式在仓库内 `dev_assets/`；release 模式 Windows `%APPDATA%\solodawn\solodawn\`，Linux `~/.local/share/solodawn/solodawn/`，macOS `~/Library/Application Support/ai/solodawn/solodawn/`。

## 技术栈

| 层级 | 技术 |
|---|---|
| 后端 | Rust（Axum、SQLx、Tokio），Edition 2024 |
| 前端 | React 18、TypeScript、Tailwind CSS、Zustand、TanStack Query |
| 数据库 | SQLite（API 密钥通过 AES-256-GCM 加密存储） |
| 终端 | xterm.js + 原生 PTY（WebSocket 桥接） |
| 实时通信 | WebSocket（工作流事件 + 终端流） |
| 拆分部署 | tonic gRPC（Server ↔ Runner） |
| 类型安全 | Rust → TypeScript 通过 `ts-rs` 自动生成 |
| 质量保障 | 内建规则引擎 + 可选 SonarQube |
| 国际化 | 6 种语言（en、zh-Hans、zh-Hant、ja、es、ko） |

## 项目结构

```
SoloDawn/
├── crates/                    # Rust 工作空间（12 个 crate）
│   ├── server/                # Axum HTTP/WebSocket 服务器 + MCP Task Server
│   ├── services/              # 业务逻辑（编排器、终端、Git 监控、合并协调、验收评审）
│   ├── quality/               # 三层质量门引擎 + 31 条内置规则
│   ├── executors/             # 8 种 AI CLI 集成 + MCP 配置适配器
│   ├── cc-switch/             # CLI 模型切换库
│   ├── feishu-connector/      # 飞书长连接客户端（openlark SDK）
│   ├── db/                    # 数据库层（模型、迁移、DAO、AES-256-GCM 加密）
│   ├── runner/                # gRPC 远程 Runner（拆分部署）
│   ├── local-deployment/      # 本地进程/容器管理
│   ├── deployment/            # 部署抽象层
│   ├── tray/                  # Windows 系统托盘
│   └── utils/                 # 共享工具（加密、OAuth、Sentry、路径）
├── frontend/                  # React 应用（components / stores / hooks / i18n 6 语言）
├── proto/                     # gRPC 协议定义
├── quality/                   # 质量门配置（quality-gate.yaml）与基线
├── scripts/                   # 开发、Docker 和部署脚本
├── docker/                    # Docker compose 与镜像
├── tests/                     # E2E 测试
└── docs/                      # 150+ 篇文档（阶段计划、运维手册、审计报告、48h 测试报告）
```

## 当前局限与路线图

### 1.0 擅长什么、不擅长什么

SoloDawn 1.0 擅长两类工作：**从零到一的新项目**，以及**需要大范围重构的旧项目**——尤其是祖传屎山代码，可以说是专克屎山。

1.0 最大的短板是**对已交付成果的持续迭代**：任务是一次性的——功能 A 交付后想调优、想扩展，只能发布一个全新任务，旧上下文全部丢失。这正是 **多轮继续 + 评分点账本**（1.0 后落地）解决的问题：已交付的对话现在可以原地继续，新一轮基于账本里的上下文纸条规划增量，而不是从零开始（见[评分点与上下文纸条](#评分点与上下文纸条)）。该机制已完整实现，并通过了单元测试与真机实测，但尚未经过 48 小时级别的验收长跑——请当作新功能看待。

### 1.0 的测试范围（如实说明）

- 本次 48 小时验收全程使用 **Claude Code** 终端。多 AI CLI 协同（编排工作区与手动工作流均支持）自 2 月 MVP 演示后未在 1.0 重测——理论可用，但不打保票；多终端的适配与测试是下一步工作。
- **飞书连接器**代码完整并已接入主服务，此前测试通过；但最近一个多月的改动之后未重新测试，暂不保证可用，待补测。
- 作者只有两台环境相近的电脑，无法穷举环境差异；跑不起来时请让你的 Claude Code / Codex 直接修（见[贡献](#贡献)）。

### 路线图

- 对多轮继续机制（1.0 后落地）做 48 小时级别的实战验证，持续打磨上下文纸条的质量
- 内置通用架构设计模板
- 内置通用 skill 与系统提示词，进一步拉高产出质量
- 通用的全自动测试（本次发布前的验收就是全自动跑的，但那套方案不通用，尚未内置）
- 飞书连接器补测；多 AI CLI 协同的适配与回归测试
- Kubernetes 部署支持；容器镜像体积优化

## 项目历程

- **2026-01-17** — 立项。
- **2026-02-12** — 最小 MVP 以 **GitCortex** 之名发布：[LinuxDo 首发帖](https://linux.do/t/topic/1606779)（当时只有手动工作流模式）。
- **2026-07-01** — **SoloDawn 1.0 正式版**发布：编排工作区（AI 全自动开发）、三层质量门、验收评审体系齐备；7 个实测任务 6 A 1 B。

## 贡献

- 欢迎 Issue 和 PR，**完全不介意 AI 生成的代码**——但 PR 必须通过云端 CI，否则无法合并。
- 环境问题：不同的人环境不同，作者无法保证每个人都不出问题。跑不起来时，把报错直接交给你的 Claude Code / Codex 修到能用，然后欢迎把修复提交回来（PR 或 Issue 均可）。
- 大改动建议先提 Issue。
- 提交 PR 前，建议在本地执行与云端 CI 一致的检查：

```bash
cargo clippy --workspace --exclude solodawn-tray --all-targets --all-features -- -D warnings
cargo nextest run --workspace --exclude solodawn-tray --cargo-profile ci --lib
cd frontend && pnpm test:run && pnpm run lint && pnpm run check && cd ..
```

## 许可证

- SoloDawn：Apache-2.0
- Vibe Kanban 衍生部分：Apache-2.0
- CC-Switch 衍生部分：MIT
- 质量门模型（移植自 SonarQube）：LGPL-3.0
- shadcn/ui 组件：MIT
- 设计风格预设与架构方法论（改编自开源 skill）：MIT / Apache-2.0
- 完整条款与逐源署名详见 `LICENSE`

## 友链

- [LINUX DO](https://linux.do/)

---

*曾用名 **GitCortex**。*
