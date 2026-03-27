# SoloDawn 开发进度追踪

> **Path Update:** This tracker moved to `docs/undeveloped/current/TODO.md` after docs archive restructuring.
> **Current Pending Plans Folder:** `docs/undeveloped/current/` (active unfinished plans) with historical plans in `docs/developed/plans/`.

> **自动化说明:** 此文件由 `superpowers-automation` skill 自动更新。
> 每完成一个任务，对应行的状态会从 `⬜` 更新为 `✅` 并记录完成时间。

## 总体进度

| 指标 | 值 |
|------|-----|
| 总任务数 | 296 |
| 已完成 | 288 |
| 进行中 | 0 |
| 未开始 | 8 (Phase 21: 2个, Phase 27: 6个) |
| 可选优化 | 5 |
| **完成率** | **97.3%** |

> **当前审计分数:** 100/100 (S级)
> **目标分数:** 100/100 (S级 - 完美代码)
>
> **Phase 23 已完成 (2026-02-04):**
> - ✅ SpawnCommand/SpawnEnv 结构体实现
> - ✅ spawn_pty_with_config 环境变量注入
> - ✅ build_launch_config 支持 Claude Code/Codex/Gemini
> - ✅ CodexHomeGuard RAII 模式自动清理
> - ✅ 136 个单元测试全部通过
> - ✅ GitHub Actions CI 通过
>
> **下一步:** Phase 27 - Docker 容器化与一键部署
> - 基于 Phase 26 已完成链路，推进容器化部署与环境一致性
> - 完善镜像构建、Compose 编排、健康检查与 CI 集成
> - 目标：实现 `docker compose up` 一键启动完整服务
>
> **核心功能已完成:**
> - ✅ Phase 20-23: 自动化协调核心 + Git 事件驱动 + WebSocket 事件广播 + 终端进程隔离

---

## Phase 0: 项目文档重写 ✅

**计划文件:** `01-phase-0-docs.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 0.1 | LICENSE 文件 - 基于 MIT 协议，声明二开来源 | ✅ | 2026-01-16 |
| 0.2 | README.md 文件 - SoloDawn 项目说明文档 | ✅ | 2026-01-16 |

---

## Phase 1: 数据库模型扩展 ✅

**计划文件:** `02-phase-1-database.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 1.1 | 创建 Workflow 数据库迁移文件 - 9张表的 DDL + 系统内置数据 | ✅ | 2026-01-17 |
| 1.2 | 创建 Workflow Rust 模型 - cli_type.rs, workflow.rs, terminal.rs | ✅ | 2026-01-17 |
| 1.3 | 创建数据库访问层 (DAO) - workflows_dao.rs, cli_types_dao.rs | ✅ | 2026-01-17 |
| 1.4 | 创建 API 路由 - workflows.rs, cli_types.rs 路由文件 | ✅ | 2026-01-17 |

---

## Phase 2: CC-Switch 核心提取与集成 ✅

**计划文件:** `03-phase-2-cc-switch.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 2.1 | 分析 CC-Switch 核心代码 - 确定可提取模块和依赖关系 | ✅ | 2026-01-17 |
| 2.2 | 创建 cc-switch crate - 在 workspace 中创建独立 crate | ✅ | 2026-01-17 |
| 2.3 | 实现原子写入和配置读写 - Claude/Codex/Gemini 配置文件操作 | ✅ | 2026-01-17 |
| 2.4 | 实现模型切换服务 - 统一的 ModelSwitcher 接口 | ✅ | 2026-01-17 |
| 2.5 | 集成 cc-switch 到 services - CCSwitchService 封装 | ✅ | 2026-01-17 |

---

## Phase 3: Orchestrator 主 Agent 实现 ✅

**计划文件:** `04-phase-3-orchestrator.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 3.1 | 创建 Orchestrator 模块结构 - mod.rs 和目录结构 | ✅ | 2026-01-18 |
| 3.2 | 实现 LLM 客户端抽象 - OpenAI 兼容 API 客户端 | ✅ | 2026-01-18 |
| 3.3 | 实现消息总线 - 跨终端消息路由 MessageBus | ✅ | 2026-01-18 |
| 3.4 | 实现 OrchestratorAgent - 主协调 Agent 核心逻辑 | ✅ | 2026-01-18 |
| 3.5 | 修复测试遗留问题 - 实现 MockLLMClient 和完整测试 | ✅ | 2026-01-18 |

---

## Phase 4: 终端管理与启动机制 ✅

**计划文件:** `05-phase-4-terminal.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 4.1 | 实现 TerminalLauncher - 终端进程启动器 | ✅ | 2026-01-18 |
| 4.2 | 实现进程管理 - TerminalProcess 生命周期管理 | ✅ | 2026-01-18 |
| 4.3 | 实现 CLI 检测服务 - 检测已安装的 CLI 工具 | ✅ | 2026-01-18 |

---

## Phase 5: Git 事件驱动系统 ✅

**计划文件:** `06-phase-5-git-watcher.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 5.1 | 实现 GitWatcher - 监听 .git/refs/heads 目录变化 | ✅ | 2024-01-18 |
| 5.2 | 实现提交信息解析器 - 解析 commit message 中的状态标记 | ✅ | 2024-01-18 |
| 5.3 | 连接 Git 事件到 Orchestrator - GitEventHandler 处理器 | ✅ | 2024-01-18 |

---

## Phase 6: 前端界面改造 (7步向导) ✅

**计划文件:** `07-phase-6-frontend.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 6.1 | 创建向导框架和类型定义 - types.ts, WorkflowWizard.tsx, StepIndicator.tsx | ✅ | 2026-01-18 |
| 6.2 | 步骤 0-1 组件 - Step0Project.tsx (工作目录), Step1Basic.tsx (基础配置) | ✅ | 2026-01-18 |
| 6.3 | 步骤 2-3 组件 - Step2Tasks.tsx (任务配置), Step3Models.tsx (模型配置) | ✅ | 2026-01-18 |
| 6.4 | 步骤 4-6 组件 - Step4Terminals, Step5Commands, Step6Advanced | ✅ | 2026-01-18 |
| 6.5 | 创建流水线视图 - PipelineView.tsx, TerminalCard.tsx, API Hooks | ✅ | 2026-01-18 |

---

## Phase 7: 终端调试视图 ✅

**计划文件:** `08-phase-7-terminal-debug.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 7.1 | 集成 xterm.js - 安装依赖，创建 TerminalEmulator.tsx | ✅ | 2026-01-19 |
| 7.2 | 实现 PTY WebSocket 后端 - terminal_ws.rs 路由 | ✅ | 2026-01-19 |
| 7.3 | 创建终端调试页面 - TerminalDebugView.tsx, WorkflowDebug.tsx | ✅ | 2026-01-19 |

---

## Phase 8.5: 代码质量修复 (审计后新增) ✅

**计划文件:** `09-phase-8.5-code-quality-fix.md`

> **说明:** 此 Phase 为 2026-01-19 代码审计后新增，已全部完成。

### P0 - 严重问题修复 (生产环境阻塞)

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 8.5.1 | 实现 execute_instruction 核心逻辑 - 移除 TODO 占位符 | ✅ | 2026-01-19 |
| 8.5.2 | API Key 加密存储 - 使用 AES-256-GCM 加密敏感字段 | ✅ | 2026-01-19 |
| 8.5.3 | 实现 handle_git_event 实际逻辑 - Git 事件到终端完成事件转换 | ✅ | 2026-01-19 |

### P1 - 代码清理

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 8.5.4 | 移除未使用的导入 - 清理 7 个编译警告 | ✅ | 2026-01-19 |
| 8.5.5 | 移除/使用未使用的 db 字段 - OrchestratorAgent.dead_code | ✅ | 2026-01-19 |
| 8.5.6 | 统一命名规范 - Rust snake_case, TypeScript camelCase, serde rename_all | ✅ | 2026-01-19 |
| 8.5.7 | 添加错误重试机制 - LLM 请求网络错误重试 (最多3次) | ✅ | 2026-01-19 |

### P2 - 代码重构

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 8.5.8 | 重构魔法数字 - MAX_HISTORY 等改为可配置项 | ✅ | 2026-01-19 |
| 8.5.9 | 重构硬编码字符串 - 提取常量 (WORKFLOW_TOPIC_PREFIX 等) | ✅ | 2026-01-19 |
| 8.5.10 | 完善状态机转换 - 显式状态转换，验证合法性 | ✅ | 2026-01-19 |
| 8.5.11 | LLM 提示词模板化 - 使用 Handlebars 模板引擎 | ✅ | 2026-01-19 |
| 8.5.12 | 数据库批量操作优化 - 使用事务批量插入 | ✅ | 2026-01-19 |
| 8.5.13 | WebSocket 终端连接超时控制 | ✅ | 2026-01-19 |

---

## Phase 8: 集成测试与文档 ✅

**计划文件:** `09-phase-8-testing.md`

> **前置条件:** Phase 8.5 代码质量修复完成

### 原有任务

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 8.1 | 端到端测试 - workflow_test.rs 完整流程测试 | ✅ | 2026-01-19 |
| 8.2 | 性能优化 - 数据库查询和 WebSocket 连接优化 | ✅ | 2026-01-19 |
| 8.3 | 用户文档 - 更新 README 和使用指南 | ✅ | 2026-01-19 |

---

## Phase 9: S级代码质量冲刺 (目标100分) ✅

**计划文件:** `2026-01-19-phase-9-s-tier-quality.md`

> **说明:** 此 Phase 为 2026-01-19 第二次代码审计后新增，目标将代码质量从 B级(82分) 提升至 S级(100分)。
> **审计报告:** 当前得分 82/100，需提升 18 分。

### P0 - 严重问题修复 (必须立即修复) [+6分]

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 9.1.1 | Terminal ID UUID 完整验证 - 使用完整UUID正则 `/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i` | ✅ | 2026-01-20 |
| 9.1.2 | WebSocket 消息类型守卫 - 创建 `isWsOutputMessage`/`isWsErrorMessage` 类型守卫函数 | ✅ | 2026-01-20 |
| 9.1.3 | 前端错误处理用户友好化 - 替换 `console.error` 为 `onError` 回调通知用户 | ✅ | 2026-01-20 |

### P1 - 架构优化 (提升可维护性) [+5分]

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 9.2.1 | 拆分 WorkflowWizard 验证逻辑 - 创建 `validators/` 目录，按步骤拆分验证器 | ✅ | 2026-01-20 |
| 9.2.2 | 创建 useWizardNavigation Hook - 提取导航逻辑到独立 hook | ✅ | 2026-01-20 |
| 9.2.3 | 创建 useWizardValidation Hook - 提取验证逻辑到独立 hook | ✅ | 2026-01-20 |
| 9.2.4 | 创建共享类型定义包 - `shared-types/` 统一前后端 WebSocket 消息类型 | ✅ | 2026-01-20 |

### P2 - 性能与安全加固 [+4分]

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 9.3.1 | 添加数据库复合索引 - `idx_workflow_project_created(project_id, created_at DESC)` | ✅ | 2026-01-20 |
| 9.3.2 | LLM 客户端速率限制 - 使用 `governor` crate 实现 RateLimiter | ✅ | 2026-01-20 |
| 9.3.3 | 测试文件安全重构 - 使用 `temp_env` crate 替代 unsafe `set_var`/`remove_var` | ✅ | 2026-01-20 |

### P3 - 国际化与文档完善 [+3分]

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 9.4.1 | 前端 i18n 集成 - 安装 `react-i18next`，提取中文硬编码字符串 | ✅ | 2026-01-20 |
| 9.4.2 | 创建中英文语言包 - `locales/zh-CN.json`, `locales/en-US.json` | ✅ | 2026-01-20 |
| 9.4.3 | 补充 TypeScript 组件 JSDoc 注释 - 为所有导出组件添加文档注释 | ✅ | 2026-01-20 |
| 9.4.4 | 补充 Rust 关键逻辑内联注释 - Orchestrator 核心流程添加详细注释 | ✅ | 2026-01-20 |

### P4 - 代码风格统一 [+2分]

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 9.5.1 | ESLint 严格模式配置 - 启用 `@typescript-eslint/strict` 规则集 | ✅ | 2026-01-20 |
| 9.5.2 | Clippy 严格模式配置 - 启用 `#![warn(clippy::pedantic)]` | ✅ | 2026-01-20 |

---

## Phase 9 完成标准

> **验收条件:** 完成所有 16 个任务后，重新执行代码审计，分数必须达到 **95-100 分 (S级)**

| 维度 | 当前分 | 目标分 | 提升策略 |
|------|--------|--------|----------|
| 架构与设计一致性 | 88 | 95+ | P1 架构优化 |
| 代码健壮性与逻辑 | 85 | 98+ | P0 严重问题修复 |
| 代码风格与可维护性 | 75 | 95+ | P1 + P4 重构 |
| 性能与安全性 | 83 | 95+ | P2 性能安全加固 |
| 文档与注释 | 78 | 95+ | P3 国际化与文档 |

---

## Phase 10: 告警清零交付 (零告警目标) ✅

**计划文件:** `2026-01-20-phase-10-warning-free.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 10.1 | 更新 Browserslist/caniuse-lite 数据库，清理依赖告警 | ✅ | 2026-01-20 |
| 10.2 | 修复 Tailwind content 配置告警（测试/构建） | ✅ | 2026-01-20 |
| 10.3 | 补齐 JSDOM Canvas mock，消除 getContext 告警 | ✅ | 2026-01-20 |
| 10.4 | 测试环境禁用 i18n debug 日志 | ✅ | 2026-01-20 |
| 10.5 | 测试环境收敛 API/console 输出（零告警） | ✅ | 2026-01-20 |
| 10.6 | 全量测试验证零告警交付 | ✅ | 2026-01-20 |

---

## Phase 10 完成标准

> **验收条件:** 前后端测试全部通过，控制台与构建输出无任何告警（含 Browserslist、Tailwind、JSDOM、console.*）。

---

## Phase 11: 单项目结构迁移（一次性迁移） ✅

**计划文件:** `2026-01-21-single-project-migration.md`, `2026-01-22-phase-11-audit-remediation.md`

> **代码审查更新 (2026-01-21):**
> - 综合评分: 7.4/10 - 计划基本合理，但风险被低估
> - 新增 Task 11.0: Git worktree 隔离环境
> - 新增 Task 11.2: 远程部署依赖清理（共享任务功能）
> **审计补充 (2026-01-22):**
> - 新增 Task 11.8-11.10：远程功能对齐与类型生成清理

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 11.0 | 创建 Git worktree 隔离环境 | ✅ | 2026-01-21 |
| 11.1 | 冻结迁移清单（Keep/Drop/补齐） | ✅ | 2026-01-21 |
| 11.2 | 远程部署依赖清理（share/、remote_client.rs、shared_tasks.rs 等） | ✅ | 2026-01-21 |
| 11.3 | cc-switch-main 必要模块补齐迁入 | ✅ | 2026-01-21 |
| 11.4 | 一次性迁移核心目录/配置到根目录 | ✅ | 2026-01-21 |
| 11.5 | 路径与工作区配置重写（Cargo/pnpm/scripts） | ✅ | 2026-01-21 |
| 11.6 | 删除不需要模块/目录（remote-frontend、npx-cli、dev_assets_seed、上游 docs 等） | ✅ | 2026-01-21 |
| 11.7 | 删除源目录（vibe-kanban-main、cc-switch-main） | ✅ | 2026-01-21 |
| 11.8 | 增加远程功能开关并向前端暴露能力标记 | ✅ | 2026-01-21 |
| 11.9 | 前端屏蔽分享/远程项目入口并移除 shared_tasks 同步逻辑 | ✅ | 2026-01-21 |
| 11.10 | 对齐类型生成：更新 generate_types 与 shared/types，消除 SharedTask 残留 | ✅ | 2026-01-21 |

---

## Phase 12: Workflow API 契约与类型生成对齐 ✅

**计划文件:** `2026-01-23-phase-12-api-contract.md`, `2026-01-24-phase-12-implementation.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 12.1 | 冻结 Workflow API 契约（请求/响应/状态枚举） | ✅ | 2026-01-24 |
| 12.2 | 后端 DTO/serde 对齐与响应重构 | ✅ | 2026-01-24 |
| 12.3 | 生成类型对齐（generate_types / shared/types.ts） | ✅ | 2026-01-24 |
| 12.4 | 前端 hooks/types 对齐（useWorkflows 等） | ✅ | 2026-01-24 |
| 12.5 | 状态枚举统一与映射修复（workflow/task/terminal） | ✅ | 2026-01-24 |
| 12.6 | 契约测试与回归验证（API + 前端） | ✅ | 2026-01-24 |

> **完成说明:** 创建了显式 DTO 层（`workflows_dto.rs`），所有 API 响应使用 camelCase 序列化，修复了前端 `draft`/`idle` 状态映射问题，添加了契约测试防止回归。**281 个后端测试 + 258 个前端测试全部通过。**

---

## Phase 13: Workflow 创建与持久化 ✅

**计划文件:** `2026-01-23-phase-13-workflow-persistence.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 13.1 | 扩展 CreateWorkflowRequest 支持任务与终端配置 | ✅ | 2026-01-24 |
| 13.2 | 使用事务创建 workflow + tasks + terminals | ✅ | 2026-01-24 |
| 13.3 | 任务分支命名/冲突策略与默认规则 | ✅ | 2026-01-24 |
| 13.4 | CLI/模型配置校验与错误返回规范化 | ✅ | 2026-01-24 |
| 13.5 | WorkflowCommand 关联与自定义参数支持 | ✅ | 2026-01-24 |
| 13.6 | Workflow Detail/List 返回完整任务/终端/命令 | ✅ | 2026-01-24 |
| 13.7 | API 集成测试与回滚验证 | ✅ | 2026-01-24 |

> **完成说明:** 实现了完整的工作流创建与持久化功能，包括事务性创建、分支命名策略、CLI/模型配置验证、DTO 转换等。共 7 个提交，+1016/-108 行代码变更。详见 `PHASE_13_SUMMARY.md`。

---

## Phase 14: Orchestrator 运行时接入与状态机完备

**计划文件:** `2026-01-23-phase-14-orchestrator-runtime.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 14.1 | Orchestrator 运行时服务装配（Deployment 容器） | ✅ | 2026-01-25 |
| 14.2 | start_workflow 触发编排流程与状态迁移 | ✅ | 2026-01-25 |
| 14.3 | 终端启动序列（cc-switch 串行启动/并行运行） | ✅ | 2026-01-25 |
| 14.4 | 任务/终端状态更新与事件广播 | ✅ | 2026-01-25 |
| 14.5 | GitWatcher 事件驱动接入 Orchestrator | ✅ | 2026-01-25 |
| 14.6 | Merge Terminal 合并流程与冲突处理 | ✅ | 2026-01-25 |
| 14.7 | Error Terminal 异常处理流程 | ✅ | 2026-01-25 |
| 14.8 | 运行态持久化与恢复（重启续跑） | ✅ | 2026-01-25 |

---

## Phase 15: 终端执行与 WebSocket 链路完善 ✅

**计划文件:** `2026-01-23-phase-15-terminal-runtime.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 15.1 | Workflow Terminal 与 Session/ExecutionProcess 绑定 | ✅ | 2026-01-25 |
| 15.2 | PTY 进程生命周期管理与 WebSocket 转发 | ✅ | 2026-01-25 |
| 15.3 | 终端输出持久化与历史回放 | ✅ | 2026-01-25 |
| 15.4 | 终端超时/取消/清理策略 | ✅ | 2026-01-25 |
| 15.5 | CLI 检测与安装指引联动 UI | ✅ | 2026-01-25 |
| 15.6 | 终端相关单测/集成测试完善 | ✅ | 2026-01-25 |

---

## Phase 16: 工作流前端体验完备 ✅

**计划文件:** `2026-01-25-phase-16-implementation.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 16.1 | Workflows 列表/详情改为真实任务数据 | ✅ | 2026-01-25 |
| 16.2 | PipelineView 显示真实任务/终端状态 | ✅ | 2026-01-25 |
| 16.3 | WorkflowDebug 实时终端调试接入 | ✅ | 2026-01-25 |
| 16.4 | WorkflowWizard 提交 payload/校验对齐 | ✅ | 2026-01-25 |
| 16.5 | 启动/暂停/停止控制与权限提示 | ✅ | 2026-01-25 |
| 16.6 | i18n/错误态/空态完善 | ✅ | 2026-01-25 |

> **完成说明:** 前端完全迁移到真实 API DTO 数据，实现实时终端调试、工作流控制（启动/暂停/停止）、Wizard 提交对齐、完整 i18n 覆盖。共 7 个 commits，+986/-130 行代码变更。269 个测试全部通过。详见 `docs/developed/plans/2026-01-25-phase-16-implementation.md`。

---

## Phase 17: 斜杠命令系统与提示词执行 ✅

**计划文件:** `2026-01-23-phase-17-slash-commands.md`

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 17.1 | 斜杠命令预设 CRUD（后端） | ✅ | 2026-01-25 |
| 17.2 | 前端斜杠命令管理与选择 | ✅ | 2026-01-25 |
| 17.3 | WorkflowCommand 关联排序与参数编辑 | ✅ | 2026-01-25 |
| 17.4 | Orchestrator 提示词渲染与命令执行 | ✅ | 2026-01-25 |
| 17.5 | 命令系统测试与回归 | ✅ | 2026-01-25 |

> **完成说明:** 实现了完整的斜杠命令系统，包括 CRUD API、前端管理界面、自定义参数编辑、Handlebars 模板渲染、Orchestrator 集成。共 6 个提交，+2206/-87 行代码变更。详见 `docs/developed/plans/2026-01-25-phase-17-slash-commands-implementation.md`。

---

## Phase 18: 全链路测试与发布就绪

**计划文件:** `2026-01-23-phase-18-release-readiness.md`

> **基线修复完成 (2026-01-28):**
> - ✅ 修复20个编译错误，建立干净测试基线
> - ✅ 零编译错误达成，20个警告（可接受）
> - ✅ CI/CD 保护就位（GitHub Actions + Pre-commit hooks）
> - ✅ 完整文档：`docs/developed/plans/2026-01-28-baseline-fix-summary.md`
> - ✅ 提交历史：16个提交，全部合并到主分支
>
> **修复详情:**
> 1. Task 1: 添加 WORKFLOW_STATUS_READY 常量
> 2. Task 2: 修复 cc_switch trait 可见性错误
> 3. Task 3: 修复 terminal_coordinator 导入路径
> 4. Task 4: 实现 parse_commit_metadata 函数
> 5. Task 5: 修复 Issue 类型使用
> 6. Task 6: 修复 SQLx query! 宏错误
> 7. Task 7-10: 验证、清理、回归防护、文档
>
> **下一步:** 开始 Phase 18 实际功能开发

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.0 | 基线修复 - 编译错误清理与测试环境准备 | ✅ | 2026-01-28 |
| 18.1 | 端到端全流程测试（创建->启动->执行->合并） | ✅ | 2026-01-30 |
| 18.2 | 并发/失败/恢复场景测试 | ✅ | 2026-01-30 |
| 18.3 | 性能与稳定性压测（终端/WS/DB） | ✅ | 2026-01-30 |
| 18.4 | 安全与配置审计（密钥/权限/日志） | ✅ | 2026-01-30 |
| 18.5 | 使用文档与运维手册完善 | ✅ | 2026-01-30 |
| 18.6 | 发布与回滚清单（版本/迁移/备份） | ✅ | 2026-01-30 |

---

## Phase 18.1: 测试技术债务清理 ✅

**计划文件:** `2026-01-29-phase-18.1-test-debt-cleanup.md`

> **状态:** ✅ 已完成
> **完成时间:** 2026-01-30
> **背景:** Phase 18.0 基线修复过程中，为让 CI 通过，临时标记了 9 个失败测试为 `#[ignore]`，并排除了集成测试。
>
> **修复总结:**
> - 修复 SQLite datetime() 索引错误（移除非确定性函数）
> - 修复 terminal_binding_test session_id 未设置问题
> - 修复 git_watcher_integration_test 时序问题
> - 修复 orchestrator tests metadata 格式（JSON → KV）
> - 修复 slugify 函数（is_ascii_alphanumeric）
> - 修复 terminal_coordinator_test 使用真实迁移
> - 所有 122 个库测试 + 14 个集成测试通过

### P0 - 集成测试编译错误修复

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.1.1 | 修复 SqlitePoolOptions 导入路径 - 统一使用 `sqlx::sqlite::SqlitePoolOptions` | ✅ | 2026-01-30 |
| 18.1.2 | 修复类型注解问题 - 为 `.bind(None)` 添加显式类型 | ✅ | 2026-01-30 |
| 18.1.3 | 修复模块路径问题 - 对齐 services crate 公开 API | ✅ | 2026-01-30 |
| 18.1.4 | 修复借用检查错误 - 调整闭包捕获 | ✅ | 2026-01-30 |
| 18.1.5 | 验证集成测试编译通过 | ✅ | 2026-01-30 |

### P1 - 简单逻辑修复

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.1.6 | 修复 slugify 函数 - 特殊字符替换为连字符 | ✅ | 2026-01-30 |
| 18.1.7 | 修复 slugify 函数 - 使用 `is_ascii_alphanumeric()` 排除 CJK | ✅ | 2026-01-30 |
| 18.1.8 | 修复 test_render_missing_variable - 更新断言字符串 | ✅ | 2026-01-30 |
| 18.1.9 | 统一 commit metadata 格式 - 同步解析逻辑或测试用例 | ✅ | 2026-01-30 |

### P2 - 测试基础设施重构

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.1.10 | 重构 terminal_coordinator_test DB 初始化 - 使用真实迁移 | ✅ | 2026-01-30 |
| 18.1.11 | 补齐 terminal_coordinator_test seed 数据 | ✅ | 2026-01-30 |
| 18.1.12 | 修复测试顺序依赖 | ✅ | 2026-01-30 |

### P3 - CI 恢复与验证

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.1.13 | 移除 `#[ignore]` 属性 - 逐步解禁已修复测试 | ✅ | 2026-01-30 |
| 18.1.14 | 恢复 CI 完整测试 - 将 `--lib` 改回完整测试 | ✅ | 2026-01-30 |
| 18.1.15 | 全量回归验证 | ✅ | 2026-01-30 |

---

## Phase 17.5: Docker 环境适配 ✅

**计划文件:** ~~`2026-01-25-phase-17.5-docker-adaptation.md`~~ → 已重定位为 Phase 19

> **说明 (2026-01-27):** 此 Phase 已重新定位为 Phase 19（Docker 容器化部署）。
> **已完成任务:**
> - ✅ Task 17.5.0: CMake 环境安装与验证（2026-01-27）
>
> **当前状态:**
> - CMake 编译问题已解决，Windows 原生开发环境可用
> - Docker 适配延后至 Phase 18 完成后执行
> - 详见 `2026-01-27-phase-19-docker-deployment.md`

---

## Phase 18.5: 设计文档对齐修复

**计划文件:** `2026-01-31-phase-18.5-design-alignment.md`

> **状态:** 🔶 进行中（P0 已完成，P1/P2 待完成）
> **更新时间:** 2026-02-04
> **优先级:** 🔴 高（核心功能偏差修复）
> **来源:** 2026-01-31 设计文档审计发现 48 个偏差（Codex + Claude 联合审计）
>
> **偏差统计:**
> - P0 严重偏差: 7 个（核心功能缺失）— ✅ 全部完成
> - P1 中等偏差: 7 个（功能完整性不足）— ⚠️ 1/7 完成
> - P2 轻微偏差: 34 个（UI/UX 细节）— ⬜ 未开始

### 阶段 1: 架构与状态（P0-4, P0-5）✅

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.5.1 | 创建 Zustand stores 目录结构与基础 store（workflowStore, terminalStore, wizardStore, modelStore, wsStore, uiStore） | ✅ | 2026-02-01 |
| 18.5.2 | 实现 wsStore WebSocket 连接管理（subscribe/send/reconnect） | ✅ | 2026-02-01 |
| 18.5.3 | 对齐 WebSocket 消息协议（{type,payload,timestamp,id}）与事件类型（workflow/terminal/git/orchestrator/system） | ✅ | 2026-02-01 |
| 18.5.4 | 实现心跳机制（system.heartbeat）与断线重连 | ✅ | 2026-02-01 |

### 阶段 2: 交互核心（P0-1, P0-2, P0-3）✅

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.5.5 | 终端数量可配置 - Step2Tasks 支持 1-10 自定义输入替代固定 1/2/3 | ✅ | 2026-02-01 |
| 18.5.6 | 调试视图接入 xterm.js PTY 交互 - 复用 TerminalEmulator 组件 | ✅ | 2026-02-01 |
| 18.5.7 | 看板实现 @dnd-kit 拖拽功能 - 列内/跨列拖拽与状态更新 | ✅ | 2026-02-01 |

### 阶段 3: 流程打通（P0-6, P0-7）✅

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.5.8 | Step3 实现真实 API 获取/验证模型 - 移除 TODO 占位符 | ✅ | 2026-02-01 |
| 18.5.9 | CLI 类型扩展到 9 个 - 补齐 Amp/Cursor Agent/Qwen Code/Copilot/Droid/Opencode | ✅ | 2026-02-01 |

### 阶段 4: 流程一致性（P1-1 ~ P1-7）⚠️ (可选优化)

> **说明:** 以下任务为 UI/UX 优化，不影响核心功能，标记为可选优化延后处理。

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.5.10 | Git 提交规范对齐 METADATA 格式 - 替换 Conventional Commit | 🔵 可选 |  |
| 18.5.11 | 路由结构改为分步路由 - /wizard/project, /wizard/basic 等 | 🔵 可选 |  |
| 18.5.12 | 设置页面结构对齐 - 添加 /settings/cli, /settings/models, /settings/presets | 🔵 可选 |  |
| 18.5.13 | 视图切换导航补齐 - NewDesignLayout 添加 [看板][流水线][调试] 切换 | ✅ | 2026-02-01 |
| 18.5.14 | 终端活动面板功能补全 - 仅 Working/Waiting + 3-5行输出 + 可折叠 + 最后更新时间 | ✅ | 2026-02-04 |
| 18.5.15 | 流水线视图信息补全 - 任务名+分支+命令+连接线 | ✅ | 2026-02-04 |
| 18.5.16 | Step4/Step5 交互对齐 - 拖拽排序 + 安装命令 + 重检按钮 | ✅ | 2026-02-04 |

### 阶段 5: 体验补齐（P2）⬜

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 18.5.17 | StatusBar/OrchestratorHeader 动态数据绑定 - 移除硬编码 | ✅ | 2026-02-04 |
| 18.5.18 | TaskCard/TerminalDots 状态区分 - 按终端状态显示不同颜色 | ✅ | 2026-02-04 |
| 18.5.19 | Step0-Step6 UI 细节对齐 - 拖拽/API Key显示/获取状态/新建命令等 | ✅ | 2026-02-04 |
| 18.5.20 | 调试视图 UI 补全 - 直接开始按钮 + 底部状态栏 + 任务分组 | ✅ | 2026-02-04 |
| 18.5.21 | 流水线视图 UI 补全 - 底部提示 + 任务间连接线 + 斜杠命令显示 | ✅ | 2026-02-04 |

---

## Phase 20: 自动化协调核心（Orchestrator 自动任务派发）

**计划文件:** `2026-02-04-phase-20-orchestrator-auto-dispatch.md`

> **状态:** ✅ 已完成
> **优先级:** 🔴 高（核心功能实现）
> **目标:** 实现"启动终端后自动开发"的核心功能
> **完成时间:** 2026-02-04
> **完成说明:** 实现了自动派发核心功能，包括 StartTask 指令处理、自动派发首个终端、终端完成后推进到下一终端、失败状态处理。130 个单元测试全部通过。

### P0 - 自动派发触发点

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.1 | 定义派发触发点 - 确认 `ready -> running` 入口仅为 `/api/workflows/:id/start` | ✅ | 2026-02-04 |
| 20.2 | 在 Orchestrator `run()` 初始阶段执行自动派发逻辑 | ✅ | 2026-02-04 |
| 20.3 | 增加"派发失败"容错与重试策略 | ✅ | 2026-02-04 |

### P1 - 任务状态初始化

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.4 | 基于 `workflow_task` 与 `terminal` 表加载每个任务的终端序列 | ✅ | 2026-02-04 |
| 20.5 | 使用 `OrchestratorState::init_task` 初始化 `TaskExecutionState` | ✅ | 2026-02-04 |
| 20.6 | 保存 `current_terminal_index` 与 `total_terminals` 状态 | ✅ | 2026-02-04 |

### P2 - 自动派发第一个终端

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.7 | 从 `workflow_task` 与第一个 `terminal` 生成指令文本 | ✅ | 2026-02-04 |
| 20.8 | 通过 `BusMessage::TerminalMessage` 发送到 PTY 会话 | ✅ | 2026-02-04 |
| 20.9 | 若终端无 `pty_session_id`，记录错误并标记任务失败或进入重试 | ✅ | 2026-02-04 |

### P3 - execute_instruction 处理 StartTask

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.10 | 在 `OrchestratorInstruction` 枚举中添加 `StartTask` 变体 | ✅ | 2026-02-04 |
| 20.11 | 实现 `execute_instruction` 对 `StartTask` 的处理逻辑 | ✅ | 2026-02-04 |
| 20.12 | `StartTask` 可携带 `task_id` 与 `instruction` 文本 | ✅ | 2026-02-04 |

### P4 - 自动触发下一个终端

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.13 | 在 `handle_terminal_completed` 中推进 `current_terminal_index` | ✅ | 2026-02-04 |
| 20.14 | 对同一 task 的下一个 terminal 自动派发 | ✅ | 2026-02-04 |
| 20.15 | 所有 terminals 完成后标记 task 完成 | ✅ | 2026-02-04 |
| 20.16 | 所有 tasks 完成后触发合并或完成流程 | ✅ | 2026-02-04 |

### P5 - 斜杠命令自动执行

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.17 | 在 `execute_slash_commands` 中对 LLM 返回内容执行 `execute_instruction` | ✅ | 2026-02-04 |
| 20.18 | 引入"无指令响应"的安全兜底处理 | ✅ | 2026-02-04 |

### P6 - 测试与回归

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 20.19 | 新增 Orchestrator 自动派发单测 | ✅ | 2026-02-04 |
| 20.20 | 新增 `StartTask` 指令执行单测 | ✅ | 2026-02-04 |
| 20.21 | 新增"终端完成 -> 下一终端派发"单测 | ✅ | 2026-02-04 |
| 20.22 | 新增终端无 PTY 会话时的错误路径测试 | ✅ | 2026-02-04 |

---

## Phase 21: Git 事件驱动接入

**计划文件:** `docs/developed/plans/2026-02-04-phase-21-git-event-driven.md`

> **状态:** ✅ 已完成
> **优先级:** 🔴 高（核心功能实现）
> **目标:** Git 提交后唤醒主 Agent，形成事件驱动闭环
> **前置条件:** Phase 20 自动化协调核心完成
> **完成时间:** 2026-02-04
> **完成说明:** 实现了 GitWatcher 生命周期管理、Git 事件发布、Orchestrator 响应 GitEvent、提交幂等性处理。37 个 orchestrator 单元测试全部通过。

### P0 - GitWatcher 生命周期接入

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 21.1 | 在 workflow 启动时初始化 GitWatcher | ✅ | 2026-02-04 |
| 21.2 | 在 workflow 停止/完成时释放 GitWatcher | ✅ | 2026-02-04 |
| 21.3 | GitWatcher 与 workflow_id 建立关联 | ✅ | 2026-02-04 |
| 21.4 | 从 project/workspace 获取 repo path 作为 watcher 目录 | ✅ | 2026-02-04 |

### P1 - Git 提交事件上报 MessageBus

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 21.5 | 增加 `MessageBus::publish_git_event` 方法或复用 `publish` | ✅ | 2026-02-04 |
| 21.6 | 将 commit hash、branch、message 写入 `BusMessage::GitEvent` | ✅ | 2026-02-04 |
| 21.7 | 解析 commit message 中的 METADATA 格式 | ✅ | 2026-02-04 |

### P2 - Orchestrator 响应 GitEvent

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 21.8 | 对含 metadata 的提交走现有 `handle_git_event` 逻辑 | ✅ | 2026-02-04 |
| 21.9 | 对无 metadata 的提交触发"唤醒"决策逻辑 | ✅ | 2026-02-04 |
| 21.10 | 将 Git 事件写入 `git_event` 表并更新处理状态 | ⬜ | (延后) |

### P3 - 配置项支持

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 21.11 | 支持 GitWatcher polling interval 配置 | ✅ | 2026-02-04 |
| 21.12 | 支持 workflow 级别 Git 监测开关（可选） | ⬜ | (可选) |

### P4 - 测试与回归

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 21.13 | 新增 GitWatcher 启动接入测试 | ✅ | 2026-02-04 |
| 21.14 | 新增 GitEvent 发布测试 | ✅ | 2026-02-04 |
| 21.15 | 新增 Orchestrator GitEvent 响应测试 | ✅ | 2026-02-04 |
| 21.16 | 新增 METADATA 解析测试 | ✅ | 2026-02-04 |

---

## Phase 22: WebSocket 事件广播完善 ✅

**计划文件:** `2026-02-04-phase-22-websocket-broadcast.md`

> **状态:** ✅ 已完成
> **优先级:** 🟡 中（前端体验优化）
> **目标:** 前端实时感知 workflow / orchestrator / git 状态变化
> **前置条件:** Phase 21 Git 事件驱动接入完成
> **完成时间:** 2026-02-04
> **完成说明:** 实现了完整的 WebSocket 事件广播基础设施，包括后端 4 个新文件（workflow_events.rs, subscription_hub.rs, event_bridge.rs, workflow_ws.rs）和前端 wsStore 增强。26 个单元测试全部通过，CI 构建成功。

### P0 - WebSocket 事件通道设计

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 22.1 | 新增 workflow 事件 WS 路由 `/ws/workflow/:id/events` | ✅ | 2026-02-04 |
| 22.2 | WS 消息格式对齐设计 `{type, payload, timestamp, id}` | ✅ | 2026-02-04 |
| 22.3 | 增加 workflow 级别事件流订阅机制 | ✅ | 2026-02-04 |

### P1 - MessageBus 到 WebSocket 桥接

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 22.4 | 转发 `StatusUpdate` 为 `workflow.status_changed` | ✅ | 2026-02-04 |
| 22.5 | 转发 Orchestrator 运行状态为 `orchestrator.awakened/sleeping` | ✅ | 2026-02-04 |
| 22.6 | 转发决策输出为 `orchestrator.decision` | ✅ | 2026-02-04 |
| 22.7 | 转发 GitWatcher 提交为 `git.commit_detected` | ✅ | 2026-02-04 |
| 22.8 | 转发终端状态为 `terminal.status_changed` | ✅ | 2026-02-04 |

### P2 - 心跳机制

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 22.9 | 服务端定期发送 `system.heartbeat`（30秒间隔） | ✅ | 2026-02-04 |
| 22.10 | 客户端收到心跳后更新连接时间戳 | ✅ | 2026-02-04 |
| 22.11 | 客户端心跳超时后自动重连 | ✅ | 2026-02-04 |

### P3 - 前端订阅与渲染

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 22.12 | 在 workflow 详情页建立 WS 连接 | ✅ | 2026-02-04 |
| 22.13 | 将事件同步到 Zustand wsStore | ✅ | 2026-02-04 |
| 22.14 | PipelineView 实时刷新终端状态 | ✅ | 2026-02-04 |
| 22.15 | TerminalDebugView 实时刷新 | ✅ | 2026-02-04 |
| 22.16 | StatusBar 显示 Orchestrator 状态 | ✅ | 2026-02-04 |

### P4 - 测试与回归

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 22.17 | 新增 WS 事件路由测试 | ✅ | 2026-02-04 |
| 22.18 | 新增前端 wsStore 事件处理测试 | ✅ | 2026-02-04 |
| 22.19 | 新增心跳机制测试 | ✅ | 2026-02-04 |
| 22.20 | 新增断线重连测试 | ✅ | 2026-02-04 |

---

## Phase 23: 终端进程隔离修复（cc-switch 架构重构）

**计划文件:** `2026-02-04-phase-23-process-isolation.md`

> **状态:** ✅ 已完成
> **优先级:** 🔴 高（核心架构缺陷修复）
> **目标:** 实现终端进程级别的配置隔离，避免修改全局配置文件
> **发现时间:** 2026-02-04
> **完成时间:** 2026-02-04
> **Codex 主脑审查:** ✅ 已完成
>
> **问题描述:**
> - cc-switch 修改全局 ~/.claude/settings.json，导致多工作流冲突
> - 用户全局配置被覆盖
> - 无真正的进程隔离
>
> **解决方案:**
> - 通过环境变量注入实现进程级隔离
> - 新增 SpawnCommand/SpawnEnv 结构体
> - Codex 使用 CODEX_HOME + CLI 参数实现完全隔离
>
> **完成说明:**
> - 实现 SpawnCommand/SpawnEnv 结构体用于环境变量管理
> - 实现 spawn_pty_with_config 方法支持环境变量注入
> - 实现 build_launch_config 支持 Claude Code/Codex/Gemini
> - 实现 CodexHomeGuard RAII 模式自动清理临时目录
> - 136 个单元测试全部通过，GitHub Actions CI 通过
> - 777 行代码新增，6 个文件修改

### P0 - 核心接口重构（SpawnCommand/SpawnEnv）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 23.1 | 新增 `SpawnCommand` 结构体，包含 command/args/working_dir | ✅ | 2026-02-04 |
| 23.2 | 新增 `SpawnEnv` 结构体，包含 set/unset 字段 | ✅ | 2026-02-04 |
| 23.3 | 修改 spawn_pty 签名，接收 `SpawnCommand` + `SpawnEnv` | ✅ | 2026-02-04 |
| 23.4 | 实现 `env_remove` 清理继承的环境变量 | ✅ | 2026-02-04 |
| 23.5 | 更新所有 spawn_pty 调用点 | ✅ | 2026-02-04 |

### P1 - cc_switch 服务重构

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 23.6 | 新增 `build_launch_config` 方法 | ✅ | 2026-02-04 |
| 23.7 | 实现 Claude Code 环境变量构建 | ✅ | 2026-02-04 |
| 23.8 | 实现 Codex 环境变量构建（含 CODEX_HOME） | ✅ | 2026-02-04 |
| 23.9 | 实现 Codex CLI 参数构建（--model, --config） | ✅ | 2026-02-04 |
| 23.10 | 实现 Gemini CLI 环境变量构建 | ✅ | 2026-02-04 |
| 23.11 | 对不支持配置切换的 CLI 返回空配置 | ✅ | 2026-02-04 |
| 23.12 | 标记 switch_for_terminal 为 deprecated | ✅ | 2026-02-04 |

### P2 - launcher 集成

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 23.13 | 修改 launch_terminal 使用 build_launch_config | ✅ | 2026-02-04 |
| 23.14 | 将 SpawnEnv + args 传递给 spawn_pty | ✅ | 2026-02-04 |
| 23.15 | 移除 switch_for_terminal 调用 | ✅ | 2026-02-04 |
| 23.16 | 移除 launch_all 中的 500ms 延时 | ✅ | 2026-02-04 |
| 23.17 | 添加环境变量注入的日志记录（脱敏） | ✅ | 2026-02-04 |

### P3 - Codex 完全隔离

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 23.18 | 为每个 Codex 终端生成独立的 CODEX_HOME | ✅ | 2026-02-04 |
| 23.19 | 终端结束后清理 CODEX_HOME 临时目录 | ✅ | 2026-02-04 |
| 23.20 | 测试 Codex 终端完全隔离启动 | ✅ | 2026-02-04 |

### P4 - 环境污染治理

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 23.21 | 当 custom_base_url 为空时，对 *_BASE_URL 进行 env_remove | ✅ | 2026-02-04 |
| 23.22 | 检查 TerminalCoordinator 是否仍调用 switch_for_terminal | ✅ | 2026-02-04 |

### P5 - 测试与验证

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 23.23 | 新增 spawn_pty env/unset 测试 | ✅ | 2026-02-04 |
| 23.24 | 新增 build_launch_config 单测 | ✅ | 2026-02-04 |
| 23.25 | 新增 Codex args 注入测试 | ✅ | 2026-02-04 |
| 23.26 | 新增多终端并发启动隔离测试 | ✅ | 2026-02-04 |
| 23.27 | 验证用户全局配置不被修改 | ✅ | 2026-02-04 |
| 23.28 | 端到端测试：工作流创建 -> 终端启动 -> 命令执行 | ✅ | 2026-02-04 |

---

## Phase 24: 终端自动确认与消息桥接 ✅

**计划文件:** `2026-02-06-phase-24-terminal-auto-confirm.md`

> **状态:** ✅ 已完成
> **优先级:** 🔴 高（核心功能缺陷修复）
> **目标:** 实现 Orchestrator 与 PTY 终端的双向通信，解决 CLI 工具需要二次确认的问题
> **发现时间:** 2026-02-05
> **完成时间:** 2026-02-06
> **前置条件:** Phase 23 终端进程隔离修复完成
> **参考项目:** [Auto-Claude](https://github.com/AndyMik90/Auto-Claude)
>
> **完成说明:**
> - 实现了 MessageBus → PTY Bridge (TerminalBridge)
> - 实现了 CLI 自动确认参数（Claude Code/Codex/Gemini）
> - 实现了智能提示检测（6种提示类型：EnterConfirm, YesNo, Choice, ArrowSelect, Input, Password）
> - 实现了 PromptWatcher 和 PromptHandler
> - 添加了 auto_confirm 字段到数据库
> - 前端 UI 支持 waiting_for_approval 和 stalled 状态
> - 160 个后端测试全部通过
> - GitHub Actions CI 通过
> - 新增 2463 行代码，20 个文件修改

### P0 - MessageBus → PTY 输入桥（核心）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.1 | 创建 `terminal/bridge.rs` 模块 - 订阅 `pty_session_id` 主题 | ✅ | 2026-02-06 |
| 24.2 | 实现 `BusMessage::TerminalInput` 到 PTY stdin 的写入 | ✅ | 2026-02-06 |
| 24.3 | 处理行尾补齐（无 `\n` 时自动追加） | ✅ | 2026-02-06 |
| 24.4 | 处理写入失败与终端不存在的情况 | ✅ | 2026-02-06 |
| 24.5 | 维护活跃 session 的 map，终端退出时清理 | ✅ | 2026-02-06 |
| 24.6 | 在 `TerminalLauncher` 启动后注册桥接任务 | ✅ | 2026-02-06 |
| 24.7 | 在 `/api/terminals/:id/start` 手动启动路径也注册桥接 | ✅ | 2026-02-06 |

### P0 - CLI 自动确认参数

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.8 | Claude Code 追加 `--dangerously-skip-permissions` 参数 | ✅ | 2026-02-06 |
| 24.9 | Codex 追加 `--yolo` 参数 | ✅ | 2026-02-06 |
| 24.10 | Gemini CLI 追加 `--yolo` 参数 | ✅ | 2026-02-06 |
| 24.11 | 增加 per-terminal 的自动确认开关（auto_confirm 字段） | ✅ | 2026-02-06 |
| 24.12 | 确保手动启动路径也带上参数 | ✅ | 2026-02-06 |

### P1 - 智能提示检测（PromptDetector）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.13 | 创建 `terminal/prompt_detector.rs` 模块 | ✅ | 2026-02-06 |
| 24.14 | 实现 6 种提示类型检测（EnterConfirm, YesNo, Choice, ArrowSelect, Input, Password） | ✅ | 2026-02-06 |
| 24.15 | 实现 Regex 模式匹配与置信度评分 | ✅ | 2026-02-06 |
| 24.16 | 实现危险关键词检测 | ✅ | 2026-02-06 |
| 24.17 | 实现 ArrowSelect 选项解析与箭头序列生成 | ✅ | 2026-02-06 |
| 24.18 | 实现优先级检测防止误判 | ✅ | 2026-02-06 |
| 24.19 | 添加 10 个单元测试 | ✅ | 2026-02-06 |
| 24.20 | 导出 ARROW_UP/ARROW_DOWN 常量 | ✅ | 2026-02-06 |

### P1 - Orchestrator 提示处理与决策

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.21 | 创建 `terminal/prompt_watcher.rs` - 监控 PTY 输出并发布事件 | ✅ | 2026-02-06 |
| 24.22 | 创建 `orchestrator/prompt_handler.rs` - 处理提示事件并做决策 | ✅ | 2026-02-06 |
| 24.23 | 实现规则优先策略：EnterConfirm 高置信度直接发送 `\n` | ✅ | 2026-02-06 |
| 24.24 | 实现危险关键词检测，触发 ask_user | ✅ | 2026-02-06 |
| 24.25 | 实现 Password 类型强制 ask_user | ✅ | 2026-02-06 |
| 24.26 | 实现 YesNo/Choice/ArrowSelect/Input 的规则默认决策 | ✅ | 2026-02-06 |
| 24.27 | 实现 build_arrow_sequence 函数（ANSI 转义序列） | ✅ | 2026-02-06 |
| 24.28 | 实现 PromptDecision 类型与状态机 | ✅ | 2026-02-06 |
| 24.29 | 前端显示 waiting_for_approval 状态的 UI | ✅ | 2026-02-06 |
| 24.30 | 维护每个 terminal 的 prompt 状态机，避免抖动和重复响应 | ✅ | 2026-02-06 |

### P2 - WebSocket 事件与类型定义

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.31 | 新增 TerminalPromptDetected/TerminalInput/TerminalPromptDecision 消息类型 | ✅ | 2026-02-06 |
| 24.32 | 新增 terminal.prompt_detected/terminal.prompt_decision WebSocket 事件 | ✅ | 2026-02-06 |
| 24.33 | 实现 MessageBus 发布方法（publish_terminal_prompt_detected 等） | ✅ | 2026-02-06 |
| 24.34 | 更新 workflow_events.rs 处理新事件类型 | ✅ | 2026-02-06 |

### P3 - 数据库与前端

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.35 | 新增数据库迁移 `20260206000000_add_auto_confirm_to_terminal.sql` | ✅ | 2026-02-06 |
| 24.36 | 更新 Terminal 模型添加 auto_confirm 字段 | ✅ | 2026-02-06 |
| 24.37 | 前端 workflowStatus.ts 添加 waiting_for_approval/stalled 状态 | ✅ | 2026-02-06 |
| 24.38 | 前端 i18n 添加新状态翻译（中英文） | ✅ | 2026-02-06 |

### P4 - 测试与回归

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 24.39 | 新增 PromptDetector 单元测试（10 个测试） | ✅ | 2026-02-06 |
| 24.40 | 新增 PromptWatcher 单元测试（4 个测试） | ✅ | 2026-02-06 |
| 24.41 | 新增 PromptHandler 单元测试（6 个测试） | ✅ | 2026-02-06 |
| 24.42 | 全量回归验证（160 个后端测试通过） | ✅ | 2026-02-06 |
| 24.43 | CI 回归验证（GitHub Actions 通过） | ✅ | 2026-02-06 |

---

## Phase 25: 自动确认可靠性修复（Phase 24 补强）✅

**计划文件:** `2026-02-06-phase-25-auto-confirm-fix.md`, `2026-02-07-phase-25-implementation.md`

> **状态:** ✅ 已完成
> **优先级:** 🔴 高（线上稳定性）
> **目标:** 修复"运行中但无输出、无文件变化"的自动确认链路缺陷
> **前置条件:** Phase 24 终端自动确认与消息桥接完成
> **完成时间:** 2026-02-07
> **完成说明:** 实现了 PTY 输出扇出架构、PromptWatcher 后台任务、UTF-8 流式解码器、Terminal WebSocket 迁移。176 个单元测试全部通过，GitHub Actions CI/CD 通过。新增 555 行代码，9 个文件修改。

### P0 - 自动确认参数必达

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 25.1 | 前端 workflow 创建请求 terminal 增加 `autoConfirm`，默认 `true` | ✅ | 2026-02-07 |
| 25.2 | 后端 `auto_confirm` 缺省值改为 `true`（保留显式 `false`） | ✅ | 2026-02-07 |

### P1 - PromptWatcher 后台解耦

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 25.3 | 建立 PTY 输出后台扇出通道，解耦 WS 与 PromptWatcher | ✅ | 2026-02-07 |
| 25.4 | PromptWatcher 改为独立后台任务，覆盖 workflow/手动启动路径 | ✅ | 2026-02-07 |

### P2 - 黑屏修复与诊断增强

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 25.5 | 修复 `terminal_ws` UTF-8 流式解码容错，避免长期黑屏 | ✅ | 2026-02-07 |
| 25.6 | 增强链路日志与端到端回归（有 WS / 无 WS / 断连重连） | ✅ | 2026-02-07 |

**完成标准:**
- ✅ 新建 workflow terminal 默认 `auto_confirm=1`
- ✅ 无前端连接时自动确认链路仍可工作
- ✅ 黑屏场景修复并通过回归验收

**核心成果:**
- ✅ UTF-8 Stream Decoder (253 lines, 8 tests)
- ✅ Output Fanout Architecture (302 lines, 8 tests)
- ✅ ProcessManager Integration (205 lines changed)
- ✅ PromptWatcher Background Task (149 lines changed)
- ✅ Terminal WebSocket Migration (256 lines changed)
- ✅ Unified Terminal Start Path (42 lines added)

---

## Phase 26: 联合审计问题全量修复（Codex + Claude） ✅

**计划文件:** `2026-02-08-phase-26-unified-audit-fix-plan.md`

> **状态:** ✅ 已完成（2026-02-07）
> **优先级:** 🔴 最高（阻断级/核心链路修复）
> **目标:** 修复 `codex和Claudecode联合分析_三文件增量合并.md` 发现的全部 60 个高优先问题
> **前置条件:** Phase 25 自动确认可靠性修复完成
> **修复范围:** 阻断级 19 + 逻辑级 21 + 兼容级 10 + 合并报告补充高优先 10

### P0 - 阻断级问题（A1-A19）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 26.1 | 修复 Step4 终端校验键不一致，恢复错误提示与阻断一致性 | ✅ | 2026-02-07 |
| 26.2 | 修复多任务 Workflow 终端初始化缺失导致提交失败 | ✅ | 2026-02-07 |
| 26.3 | 移除 DiffsPanel 渲染期 setState，消除渲染抖动/循环风险 | ✅ | 2026-02-07 |
| 26.4 | stop_workflow 同步停止终端进程，修复“状态已停进程仍跑” | ✅ | 2026-02-07 |
| 26.5 | Workflow merge 执行真实合并链路，禁止“仅改状态假完成” | ✅ | 2026-02-07 |
| 26.6 | 统一终端状态枚举与映射，覆盖 cancelled/review_* 状态 | ✅ | 2026-02-07 |
| 26.7 | 修复 Review 事件误发为 workflow.status_changed 的语义错位 | ✅ | 2026-02-07 |
| 26.8 | 创建 WorkflowTask 时正确绑定 vk_task_id，恢复终端会话关联 | ✅ | 2026-02-07 |
| 26.9 | task attempt 启动失败向调用方显式返回错误 | ✅ | 2026-02-07 |
| 26.10 | 放开无 process 的 Follow-up 输入，避免失败态无法自救 | ✅ | 2026-02-07 |
| 26.11 | 修复 createTask 后错误跳转 attempts/latest | ✅ | 2026-02-07 |
| 26.12 | 统一 Slash Command description 校验规则（前后端一致） | ✅ | 2026-02-07 |
| 26.13 | 修复 Slash Command 重命名命令名无效 | ✅ | 2026-02-07 |
| 26.14 | 统一 Open Editor 请求字段（file_path / git_repo_path）契约 | ✅ | 2026-02-07 |
| 26.15 | containers/attempt-context 支持子目录 ref 解析 | ✅ | 2026-02-07 |
| 26.16 | 容器查询 not found 正确映射为 404（非 500） | ✅ | 2026-02-07 |
| 26.17 | 修复 Terminal WS 接收超时导致只读会话误断连 | ✅ | 2026-02-07 |
| 26.18 | stop terminal 对不存在 id 返回 not_found，禁止假成功 | ✅ | 2026-02-07 |
| 26.19 | 任务列表 executor 可空解码修复，消除潜在 500 | ✅ | 2026-02-07 |

### P1 - 逻辑级问题（B1-B21）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 26.20 | useWorkflowEvents 引用计数化，避免多订阅互断 | ✅ | 2026-02-07 |
| 26.21 | WS Store 支持多 workflow 并行订阅，避免串线覆盖 | ✅ | 2026-02-07 |
| 26.22 | 前端补齐 terminal.prompt_* 事件类型与消费链路 | ✅ | 2026-02-07 |
| 26.23 | EventBridge 无订阅场景引入事件补偿/缓存策略 | ✅ | 2026-02-07 |
| 26.24 | lagged 事件恢复机制，禁止静默丢弃 | ✅ | 2026-02-07 |
| 26.25 | prepare 部分失败时统一回滚已启动终端 | ✅ | 2026-02-07 |
| 26.26 | workflow task 状态接口移除/拒绝 in_progress 非法值 | ✅ | 2026-02-07 |
| 26.27 | 修复单仓 open-editor + file_path 基目录拼接错误 | ✅ | 2026-02-07 |
| 26.28 | 多仓 merge 不再提前置 Done/归档 workspace | ✅ | 2026-02-07 |
| 26.29 | 修复删除任务缓存 key 错误导致的详情闪回 | ✅ | 2026-02-07 |
| 26.30 | 修复 useAttemptExecution 空 key 停止态串扰 | ✅ | 2026-02-07 |
| 26.31 | Follow-up 脚本入口可用性与 Result 错误处理一致 | ✅ | 2026-02-07 |
| 26.32 | ExecutionProcessesProvider 纳入 attemptId 维度 | ✅ | 2026-02-07 |
| 26.33 | subscription_hub 无订阅不创建 channel，抑制增长 | ✅ | 2026-02-07 |
| 26.34 | workspace 清理 SQL 修复 NULL completed_at 处理 | ✅ | 2026-02-07 |
| 26.35 | execution_process 完成更新同步刷新 updated_at | ✅ | 2026-02-07 |
| 26.36 | execution_processes raw logs WS 避免重复拉流 | ✅ | 2026-02-07 |
| 26.37 | Project Open Editor 路径范围约束（项目内白名单） | ✅ | 2026-02-07 |
| 26.38 | 远程编辑器 URL 路径编码，支持特殊字符 | ✅ | 2026-02-07 |
| 26.39 | 远程文件 :1:1 追加规则去本地 is_file 依赖 | ✅ | 2026-02-07 |
| 26.40 | 组织/远程项目入口与后端能力对齐（支持或禁用） | ✅ | 2026-02-07 |

### P2 - 兼容/实现级问题（C1-C10）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 26.41 | 修复 ProjectSettings 更新类型不完整导致前端检查失败 | ✅ | 2026-02-07 |
| 26.42 | executors command build 空 base 防 panic 处理 | ✅ | 2026-02-07 |
| 26.43 | plain_text_processor time_gap 分支丢 chunk 修复 | ✅ | 2026-02-07 |
| 26.44 | review session selector 双 Skip 索引偏移修复 | ✅ | 2026-02-07 |
| 26.45 | review 的 gh 检测兼容 Windows（which -> where） | ✅ | 2026-02-07 |
| 26.46 | cc-switch 原子写入兼容 Windows 已存在目标文件 | ✅ | 2026-02-07 |
| 26.47 | MCP 不支持错误文案/错误码统一，前端可稳定识别 | ✅ | 2026-02-07 |
| 26.48 | MCP 保存逻辑使用稳定 profile key（非对象引用） | ✅ | 2026-02-07 |
| 26.49 | 前端 Workflow 状态枚举补齐 merging 并对齐映射 | ✅ | 2026-02-07 |
| 26.50 | TerminalDto.customApiKey 序列化与 shared 类型一致化 | ✅ | 2026-02-07 |

### P0/P1 - 合并报告补充高优先问题（S1-S10）

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 26.51 | 修复 `Terminal::set_started()` 状态语义错误（waiting/started 对齐） | ✅ | 2026-02-07 |
| 26.52 | 补齐终端状态变化 WebSocket 事件发布链路（状态变更可观测） | ✅ | 2026-02-07 |
| 26.53 | 修复 prepare 流程状态转换混乱，避免终端长期卡在 waiting | ✅ | 2026-02-07 |
| 26.54 | auto_confirm 与 PromptWatcher 注册逻辑打通（配置生效） | ✅ | 2026-02-07 |
| 26.55 | PromptHandler 决策逻辑接入 auto_confirm（避免策略脱节） | ✅ | 2026-02-07 |
| 26.56 | 补齐历史数据迁移：旧 terminal 的 auto_confirm 缺省值回填 | ✅ | 2026-02-07 |
| 26.57 | 修复 Phase 24/25 自动确认集成断点（事件到决策链路一致） | ✅ | 2026-02-07 |
| 26.58 | 修复手动启动路径 logger 失败处理不一致（失败语义统一） | ✅ | 2026-02-07 |
| 26.59 | 修复 TerminalLogger flush 恢复机制竞态（避免重复恢复/丢日志） | ✅ | 2026-02-07 |
| 26.60 | 补强 lagged 场景输出补偿链路（与 26.24 联合验收） | ✅ | 2026-02-07 |

**完成标准:**
- ✅ 报告中 60 个高优先问题全部有对应修复任务与验收条目
- ✅ 阻断级问题全部关闭（A1-A19）
- ✅ 补充高优先问题全部关闭（S1-S10）
- ✅ 关键链路回归通过：workflow 创建→prepare→start→terminal/ws→task attempt→merge
- ✅ 前端 `pnpm -C frontend run check`、后端核心测试与回归通过

---

## Phase 27: Docker 容器化部署（开源发布优化）

**计划文件:** `2026-01-27-phase-19-docker-deployment.md`

> **状态:** 📋 待实施
> **优先级:** 🚀 中（开源部署便利性）
> **目标:** 方便开源后其他开发者一键部署
> **前置条件:** Phase 25 自动确认可靠性修复完成

| Task | 目标描述 | 状态 | 完成时间 |
|------|----------|------|----------|
| 27.1 | 创建生产部署 Dockerfile（多阶段构建） | ⬜ |  |
| 27.2 | 创建生产部署 Docker Compose | ⬜ |  |
| 27.3 | 创建开发调试 Docker Compose（可选） | ⬜ |  |
| 27.4 | 优化 Docker 镜像构建（缓存/体积） | ⬜ |  |
| 27.5 | 创建 Docker 部署文档 | ⬜ |  |
| 27.6 | CI/CD 集成（GitHub Actions） | ⬜ |  |

**完成标准:**
- ✅ 生产镜像可稳定构建并通过基础健康检查
- ✅ `docker compose up` 可一键拉起前后端与依赖服务
- ✅ 开发/生产部署文档可复现（新机器按文档可运行）
- ✅ CI 在主分支上完成镜像构建与基础集成验证

**预期收益:**
- ✅ 使用者无需安装 Rust/Node.js 环境
- ✅ `docker compose up` 一键启动完整服务
- ✅ 跨平台统一部署体验
- ✅ 生产环境可直接使用

---

## 代码规范 (后续开发强制遵守)

> **来源:** 2026-01-19 代码审计报告

### A. 命名规范

| 语言 | 命名风格 | 示例 |
|------|----------|------|
| **Rust 结构体/枚举** | PascalCase | `Workflow`, `TerminalStatus` |
| **Rust 字段/变量** | snake_case | `cli_type_id`, `order_index` |
| **Rust 常量** | SCREAMING_SNAKE_CASE | `MAX_HISTORY`, `WORKFLOW_TOPIC_PREFIX` |
| **TypeScript 类型/接口** | PascalCase | `Workflow`, `TerminalConfig` |
| **TypeScript 字段** | camelCase | `cliTypeId`, `orderIndex` |
| **数据库列名** | snake_case | `orchestrator_api_key`, `workflow_task_id` |
| **API JSON 响应** | camelCase | `cliTypeId`, `orchestratorApiKey` |

**Rust Serde 配置模板:**
```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]  // API 响应统一使用 camelCase
pub struct Workflow {
    pub workflow_id: String,
    pub cli_type_id: String,
    // ...
}
```

### B. 禁止硬编码

**错误示例:**
```rust
// ❌ 硬编码
let topic = format!("workflow:{}", id);
const MAX_HISTORY: usize = 50;
```

**正确示例:**
```rust
// ✅ 可配置
pub const WORKFLOW_TOPIC_PREFIX: &str = "workflow:";
pub const DEFAULT_MAX_HISTORY: usize = 50;

#[derive(Debug, Clone)]
pub struct OrchestratorConfig {
    pub max_conversation_history: usize,
    pub llm_timeout_secs: u64,
}
```

### C. 错误处理规范

**网络请求必须有重试:**
```rust
async fn request_with_retry<T>(
    f: impl Fn() -> impl Future<Output = anyhow::Result<T>>,
) -> anyhow::Result<T> {
    let max_retries = 3;
    for attempt in 0..max_retries {
        match f().await {
            Ok(result) => return Ok(result),
            Err(e) if attempt < max_retries - 1 => {
                tokio::time::sleep(Duration::from_millis(1000 * (attempt + 1) as u64)).await;
            }
            Err(e) => return Err(e),
        }
    }
    unreachable!()
}
```

### D. 敏感信息加密

**API Key/Token 必须加密存储:**
```rust
// 使用 aes-gcm 加密
pub orchestrator_api_key_encrypted: Option<String>,

// 提供加密/解密方法
impl Workflow {
    pub fn set_api_key(&mut self, plaintext: &str, key: &[u8; 32]) -> anyhow::Result<()>;
    pub fn get_api_key(&self, key: &[u8; 32]) -> anyhow::Result<Option<String>>;
}
```

### E. 状态机规范

**状态转换必须显式验证:**
```rust
impl OrchestratorState {
    pub fn transition_to(&mut self, new_state: State) -> anyhow::Result<()> {
        match (self.current, new_state) {
            (State::Idle, State::Processing) => { /* valid */ }
            (State::Processing, State::Idle) => { /* valid */ }
            (from, to) => return Err(anyhow!("Invalid transition: {:?} → {:?}", from, to)),
        }
        self.current = new_state;
        Ok(())
    }
}
```

### F. 数据库操作规范

**批量操作使用事务:**
```rust
pub async fn create_workflow_with_tasks(
    pool: &SqlitePool,
    workflow: &Workflow,
    tasks: Vec<Task>,
) -> anyhow::Result<()> {
    let mut tx = pool.begin().await?;
    sqlx::query("INSERT INTO workflow ...").execute(&mut *tx).await?;
    for task in tasks {
        sqlx::query("INSERT INTO task ...").execute(&mut *tx).await?;
    }
    tx.commit().await?;
    Ok(())
}
```

---

## 状态说明

| 状态 | 含义 |
|------|------|
| ✅ | 已完成 |
| 🔄 | 进行中 |
| ⬜ | 未开始 |
| ❌ | 阻塞/失败 |
| 🚨 | **紧急修复** |

---

## 自动化触发记录

> 每次 skill 触发时记录，用于追踪自我续航

| 触发时间 | 触发原因 | 开始任务 | 结束任务 |
|----------|----------|----------|----------|
| 2026-01-17 | 初始设置完成 | - | 计划拆分完成 |
| 2026-01-17 | Phase 1 Database 完成 | Task 1.1-1.4 | 4/4 任务完成 |
| 2026-01-17 | Phase 2 CC-Switch 完成 | Task 2.1-2.5 | 5/5 任务完成 |
| 2026-01-18 | Phase 3 Orchestrator 完成 | Task 3.1-3.4 | 4/4 任务完成 (22个测试通过) |
| 2026-01-18 | Phase 4 Terminal 完成 | Task 4.1-4.3 | 3/3 任务完成 + 集成测试 |
| 2026-01-18 | Phase 3 测试遗留问题修复 | Task 3.5 (新增) | 添加 MockLLMClient，完整测试实现 |
| 2024-01-18 | Phase 5 Git Watcher 完成 | Task 5.1-5.3 | 3/3 任务完成 + 12个测试通过 + 使用文档 |
| 2026-01-18 | Phase 6 Frontend 完成 | Task 6.1-6.5 | 5/5 任务完成 + 180个测试通过 + 路由集成 |
| 2026-01-19 | Phase 7 Terminal Debug 完成 | Task 7.1-7.3 | 3/3 任务完成 + xterm.js 集成 + WebSocket 后端 + 调试页面 |
| 2026-01-19 | **代码审计完成** | - | 发现 C 级代码质量问题，新增 Phase 8.5 |
| 2026-01-19 | **Phase 8.5 代码质量修复完成** | Task 8.5.1-8.5.13 | 13/13 任务完成 + 分支合并 + WebSocket超时控制 |
| 2026-01-19 | **Phase 8 集成测试与文档完成** | Task 8.1-8.3 | 3/3 任务完成 + E2E测试 + 性能优化 + 用户文档 + 分支合并 |
| 2026-01-19 | **第二次代码审计完成** | - | B级 82/100 分，新增 Phase 9 冲刺 S级 |
| 2026-01-20 | **Phase 10 告警清零交付完成** | Task 10.1-10.6 | 6/6 任务完成 + 前后端测试零告警 |
| 2026-01-24 | **Phase 13 Workflow 持久化完成** | Task 13.1-13.7 | 7/7 任务完成 + 事务性创建 + 分支命名策略 + CLI/模型验证 + DTO 转换 |
| 2026-01-25 | **Phase 17 斜杠命令系统完成** | Task 17.1-17.5 | 5/5 任务完成 + CRUD API + 前端管理 + 参数编辑 + 模板渲染 + 测试 |
| 2026-01-30 | **Phase 18.1/18.2 E2E 测试完成** | Task 18.1-18.2 | 22个测试通过 + 工作流生命周期 + Git提交检测 + 终端恢复场景 + 线程安全EnvGuard |
| 2026-01-30 | **Phase 18.3-18.6 性能/安全/文档完成** | Task 18.3-18.6 | 性能测试(终端/WS/DB) + 安全测试(加密/访问控制/注入防护) + Criterion基准 + USER_GUIDE + OPERATIONS_MANUAL + RELEASE_CHECKLIST |
| 2026-02-04 | **Phase 22 WebSocket 事件广播完成** | Task 22.1-22.20 | 20/20 任务完成 + workflow_events.rs + subscription_hub.rs + event_bridge.rs + workflow_ws.rs + wsStore 增强 + 26 个测试通过 |
| 2026-02-04 | **Phase 23 终端进程隔离完成** | Task 23.1-23.28 | 28/28 任务完成 + SpawnCommand/SpawnEnv + build_launch_config + CodexHomeGuard RAII + 136 个测试通过 + CI 通过 |

