# SoloDawn 详细实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 基于 Vibe Kanban 改造并集成 CC-Switch，实现主 Agent 跨终端任务协调系统

**Architecture:** 在 Vibe Kanban 的 Rust 后端基础上扩展 Orchestrator 模块，集成 CC-Switch 的模型切换能力，通过 Git 事件驱动实现低 Token 消耗的多终端协调

**Tech Stack:** Rust (axum 0.8.4, sqlx, tokio) + React 18 + TypeScript + SQLite + xterm.js

**源项目位置:**
- Vibe Kanban: `F:\Project\SoloDawn\vibe-kanban-main`
- CC-Switch: `F:\Project\SoloDawn\cc-switch-main`

**设计文档:** `F:\Project\SoloDawn\docs\plans\2026-01-16-orchestrator-design.md`

---

## 阶段概览

| 阶段 | 内容 | 任务数 | 依赖 |
|------|------|--------|------|
| Phase 0 | 项目文档重写 | 2 | 无 |
| Phase 1 | 数据库模型扩展 | 4 | Phase 0 |
| Phase 2 | CC-Switch 核心提取与集成 | 5 | Phase 1 |
| Phase 3 | Orchestrator 主 Agent 实现 | 4 | Phase 2 |
| Phase 4 | 终端管理与启动机制 | 3 | Phase 3 |
| Phase 5 | Git 事件驱动系统 | 3 | Phase 4 |
| Phase 6 | 前端界面改造 | 5 | Phase 5 |
| Phase 7 | 终端调试视图 | 3 | Phase 6 |
| Phase 8 | 集成测试与文档 | 3 | Phase 7 |

**总计: 32 个任务**

---

## 全局任务追踪表

> 📊 **实时进度追踪** - 实施过程中请更新此表

| Phase | 任务 | 状态 | 进度 |
|-------|------|------|------|
| **Phase 0** | Task 0.1: LICENSE 文件 | ✅ 完成 | 100% |
| | Task 0.2: README.md 文件 | ✅ 完成 | 100% |
| **Phase 1** | Task 1.1: 数据库迁移文件 | ⬜ 未开始 | 0% |
| | Task 1.2: Rust 模型 | ⬜ 未开始 | 0% |
| | Task 1.3: DAO 层 | ⬜ 未开始 | 0% |
| | Task 1.4: API 路由 | ⬜ 未开始 | 0% |
| **Phase 2** | Task 2.1: CC-Switch 分析 | ⬜ 未开始 | 0% |
| | Task 2.2: 核心代码提取 | ⬜ 未开始 | 0% |
| | Task 2.3: Switcher 服务 | ⬜ 未开始 | 0% |
| | Task 2.4: 服务层封装 | ⬜ 未开始 | 0% |
| | Task 2.5: 集成测试 | ⬜ 未开始 | 0% |
| **Phase 3** | Task 3.1: LLM 通信模块 | ⬜ 未开始 | 0% |
| | Task 3.2: 消息总线 | ⬜ 未开始 | 0% |
| | Task 3.3: Agent 核心 | ⬜ 未开始 | 0% |
| | Task 3.4: Orchestrator 整合 | ⬜ 未开始 | 0% |
| **Phase 4** | Task 4.1: 终端管理器 | ⬜ 未开始 | 0% |
| | Task 4.2: 进程管理 | ⬜ 未开始 | 0% |
| | Task 4.3: CLI 检测器 | ⬜ 未开始 | 0% |
| **Phase 5** | Task 5.1: Git 监听器 | ⬜ 未开始 | 0% |
| | Task 5.2: Commit 解析器 | ⬜ 未开始 | 0% |
| | Task 5.3: 事件处理器 | ⬜ 未开始 | 0% |
| **Phase 6** | Task 6.1: 向导框架 | ⬜ 未开始 | 0% |
| | Task 6.2: 步骤 0-1 组件 | ⬜ 未开始 | 0% |
| | Task 6.3: 步骤 2-3 组件 | ⬜ 未开始 | 0% |
| | Task 6.4: 步骤 4-6 组件 | ⬜ 未开始 | 0% |
| | Task 6.5: 流水线视图 | ⬜ 未开始 | 0% |
| **Phase 7** | Task 7.1: xterm.js 集成 | ⬜ 未开始 | 0% |
| | Task 7.2: WebSocket 后端 | ⬜ 未开始 | 0% |
| | Task 7.3: 调试视图 | ⬜ 未开始 | 0% |
| **Phase 8** | Task 8.1: 端到端测试 | ⬜ 未开始 | 0% |
| | Task 8.2: 性能优化 | ⬜ 未开始 | 0% |
| | Task 8.3: 用户文档 | ⬜ 未开始 | 0% |

**状态图例:** ✅ 完成 | 🔄 进行中 | ⬜ 未开始 | ❌ 阻塞

**总体进度:** 2/32 任务完成 (6.25%)

---

## 里程碑定义

### 🏁 Milestone 1: 数据层就绪 (Phase 0-1)

**预期产出:**
- SoloDawn 项目文档完成
- 数据库 schema 完成，包含 9 张新表
- Rust 模型和 DAO 层可用
- API 端点可访问

**验收标准:**
- [ ] `cargo sqlx migrate run` 成功
- [ ] `cargo build` 编译通过
- [ ] TypeScript 类型文件生成成功
- [ ] GET `/api/cli_types` 返回内置 CLI 类型

**关键风险:** 数据库 schema 设计可能需要迭代

---

### 🏁 Milestone 2: 模型切换能力 (Phase 2)

**预期产出:**
- CC-Switch 核心代码提取为独立 crate
- 支持 Claude/Gemini/Codex 配置切换
- 服务层 API 封装完成

**验收标准:**
- [ ] `cc-switch` crate 编译通过
- [ ] 单元测试覆盖配置读写
- [ ] API 可切换模型配置

**关键风险:** CC-Switch 代码可能有 Tauri 依赖需要剥离

---

### 🏁 Milestone 3: 协调引擎核心 (Phase 3-4)

**预期产出:**
- Orchestrator 主 Agent 可运行
- 消息总线支持跨终端通信
- 终端进程管理器可启动/停止终端

**验收标准:**
- [ ] Orchestrator 可发送指令到 LLM
- [ ] 消息总线可路由消息
- [ ] 可编程启动多个终端进程

**关键风险:** LLM 通信模块需要处理多种 API 格式

---

### 🏁 Milestone 4: Git 事件驱动 (Phase 5)

**预期产出:**
- Git 目录监听器运行
- Commit 消息解析器可提取结构化信息
- 事件处理器触发 Orchestrator 响应

**验收标准:**
- [ ] 检测到 `.git` 目录变化
- [ ] 正确解析 commit 消息中的状态标记
- [ ] 事件触发 Orchestrator 处理流程

**关键风险:** 文件系统监听在 Windows 上可能有性能问题

---

### 🏁 Milestone 5: 用户界面 (Phase 6-7)

**预期产出:**
- 7 步工作流向导可创建完整工作流
- 流水线视图显示实时状态
- 终端调试视图可查看终端输出

**验收标准:**
- [ ] 向导可完整创建工作流配置
- [ ] 流水线视图实时更新
- [ ] 终端 WebSocket 连接稳定

**关键风险:** xterm.js 与 WebSocket 集成复杂度

---

### 🏁 Milestone 6: 生产就绪 (Phase 8)

**预期产出:**
- 端到端测试覆盖主要流程
- 性能优化完成
- 用户文档完善

**验收标准:**
- [ ] E2E 测试全部通过
- [ ] 无内存泄漏
- [ ] 文档覆盖所有功能

**关键风险:** 测试环境配置复杂

---

## Phase 0: 项目文档重写

### Task 0.1: LICENSE 文件

**状态:** ✅ 已完成

**交付物:** `F:\Project\SoloDawn\LICENSE`

---

### Task 0.2: README.md 文件

**状态:** ✅ 已完成

**交付物:** `F:\Project\SoloDawn\README.md`

---

## Phase 1: 数据库模型扩展

### Task 1.1: 创建 Workflow 数据库迁移文件

**状态:** ⬜ 未开始

**前置条件:**
- Phase 0 已完成
- 已安装 sqlx-cli: `cargo install sqlx-cli`
- 了解 vibe-kanban 现有数据库结构

**目标:**
创建 workflow、workflow_task、terminal、cli_type、model_config、slash_command_preset、workflow_command 共 7 张表的迁移文件。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/db/migrations/20260116000001_create_workflow_tables.sql`

**详细步骤:**

**Step 1.1.1: 理解现有数据库结构**

首先阅读现有迁移文件，了解命名规范和字段类型约定：

```
关键现有表:
- project: 项目表，包含 id (TEXT PRIMARY KEY), name, default_agent_working_dir 等
- task: 任务表，包含 id, project_id, title, description, status 等
- workspace: 工作空间表（原 task_attempts），包含 id, task_id, container_ref, branch 等
- session: 会话表，包含 id, workspace_id, executor 等
- execution_process: 执行进程表，包含 id, session_id, run_reason, status 等

字段类型约定:
- 主键: TEXT (UUID 字符串)
- 布尔值: INTEGER (0/1)
- 时间戳: TEXT (ISO 8601 格式)
- 外键: TEXT REFERENCES xxx(id)
```

**Step 1.1.2: 创建迁移文件**

在 `vibe-kanban-main/crates/db/migrations/` 目录下创建文件 `20260116000001_create_workflow_tables.sql`：

```sql
-- ============================================================================
-- SoloDawn Workflow Tables Migration
-- 创建日期: 2026-01-16
-- 描述: 添加工作流协调相关表，支持主 Agent 跨终端任务协调
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. CLI 类型表 (cli_type)
-- 存储支持的 AI 编码代理 CLI 信息
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cli_type (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,           -- 内部名称，如 'claude-code'
    display_name        TEXT NOT NULL,                  -- 显示名称，如 'Claude Code'
    detect_command      TEXT NOT NULL,                  -- 检测命令，如 'claude --version'
    install_command     TEXT,                           -- 安装命令（可选）
    install_guide_url   TEXT,                           -- 安装指南 URL
    config_file_path    TEXT,                           -- 配置文件路径模板
    is_system           INTEGER NOT NULL DEFAULT 1,     -- 是否系统内置
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 插入系统内置 CLI 类型
INSERT INTO cli_type (id, name, display_name, detect_command, install_guide_url, config_file_path, is_system) VALUES
    ('cli-claude-code', 'claude-code', 'Claude Code', 'claude --version', 'https://docs.anthropic.com/en/docs/claude-code', '~/.claude/settings.json', 1),
    ('cli-gemini', 'gemini-cli', 'Gemini CLI', 'gemini --version', 'https://github.com/google-gemini/gemini-cli', '~/.gemini/.env', 1),
    ('cli-codex', 'codex', 'Codex', 'codex --version', 'https://github.com/openai/codex', '~/.codex/auth.json', 1),
    ('cli-amp', 'amp', 'Amp', 'amp --version', 'https://ampcode.com', NULL, 1),
    ('cli-cursor', 'cursor-agent', 'Cursor Agent', 'cursor --version', 'https://cursor.sh', NULL, 1),
    ('cli-qwen', 'qwen-code', 'Qwen Code', 'qwen --version', 'https://qwen.ai', NULL, 1),
    ('cli-copilot', 'copilot', 'GitHub Copilot', 'gh copilot --version', 'https://github.com/features/copilot', NULL, 1),
    ('cli-droid', 'droid', 'Droid', 'droid --version', 'https://droid.dev', NULL, 1),
    ('cli-opencode', 'opencode', 'Opencode', 'opencode --version', 'https://opencode.dev', NULL, 1);

-- ----------------------------------------------------------------------------
-- 2. 模型配置表 (model_config)
-- 存储每个 CLI 支持的模型配置
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS model_config (
    id              TEXT PRIMARY KEY,
    cli_type_id     TEXT NOT NULL REFERENCES cli_type(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,                      -- 模型内部名称，如 'sonnet'
    display_name    TEXT NOT NULL,                      -- 显示名称，如 'Claude Sonnet'
    api_model_id    TEXT,                               -- API 模型 ID，如 'claude-sonnet-4-20250514'
    is_default      INTEGER NOT NULL DEFAULT 0,         -- 是否默认模型
    is_official     INTEGER NOT NULL DEFAULT 0,         -- 是否官方模型
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(cli_type_id, name)
);

-- 插入 Claude Code 默认模型
INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official) VALUES
    ('model-claude-sonnet', 'cli-claude-code', 'sonnet', 'Claude Sonnet', 'claude-sonnet-4-20250514', 1, 1),
    ('model-claude-opus', 'cli-claude-code', 'opus', 'Claude Opus', 'claude-opus-4-5-20251101', 0, 1),
    ('model-claude-haiku', 'cli-claude-code', 'haiku', 'Claude Haiku', 'claude-haiku-4-5-20251001', 0, 1);

-- 插入 Gemini 默认模型
INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official) VALUES
    ('model-gemini-pro', 'cli-gemini', 'gemini-pro', 'Gemini Pro', 'gemini-2.5-pro', 1, 1),
    ('model-gemini-flash', 'cli-gemini', 'gemini-flash', 'Gemini Flash', 'gemini-2.5-flash', 0, 1);

-- 插入 Codex 默认模型
INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official) VALUES
    ('model-codex-gpt4o', 'cli-codex', 'gpt-4o', 'GPT-4o', 'gpt-4o', 1, 1),
    ('model-codex-o1', 'cli-codex', 'o1', 'O1', 'o1', 0, 1);

-- ----------------------------------------------------------------------------
-- 3. 斜杠命令预设表 (slash_command_preset)
-- 存储可复用的斜杠命令预设
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS slash_command_preset (
    id              TEXT PRIMARY KEY,
    command         TEXT NOT NULL UNIQUE,               -- 命令名，如 '/write-code'
    description     TEXT NOT NULL,                      -- 命令描述
    prompt_template TEXT,                               -- 提示词模板（可选）
    is_system       INTEGER NOT NULL DEFAULT 0,         -- 是否系统内置
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 插入系统内置斜杠命令
INSERT INTO slash_command_preset (id, command, description, prompt_template, is_system) VALUES
    ('cmd-write-code', '/write-code', '编写功能代码', '请根据以下需求编写代码：\n\n{requirement}\n\n要求：\n1. 代码质量高，可维护\n2. 包含必要的注释\n3. 遵循项目现有代码风格', 1),
    ('cmd-review', '/review', '代码审计，检查安全性和代码质量', '请审查以下代码变更：\n\n{changes}\n\n审查要点：\n1. 安全性（XSS、SQL注入、命令注入等）\n2. 代码质量和可维护性\n3. 性能问题\n4. 边界情况处理', 1),
    ('cmd-fix-issues', '/fix-issues', '修复发现的问题', '请修复以下问题：\n\n{issues}\n\n要求：\n1. 最小化修改范围\n2. 不引入新问题\n3. 添加必要的测试', 1),
    ('cmd-test', '/test', '编写和运行测试', '请为以下代码编写测试：\n\n{code}\n\n要求：\n1. 覆盖主要功能路径\n2. 包含边界情况\n3. 测试应该独立可运行', 1),
    ('cmd-refactor', '/refactor', '重构代码', '请重构以下代码：\n\n{code}\n\n重构目标：\n{goals}\n\n要求：\n1. 保持功能不变\n2. 提高代码质量\n3. 分步骤进行，每步可验证', 1),
    ('cmd-document', '/document', '编写文档', '请为以下内容编写文档：\n\n{content}\n\n文档类型：{doc_type}\n\n要求：\n1. 清晰易懂\n2. 包含示例\n3. 格式规范', 1);

-- ----------------------------------------------------------------------------
-- 4. 工作流表 (workflow)
-- 存储工作流配置和状态
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow (
    id                      TEXT PRIMARY KEY,
    project_id              TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    description             TEXT,

    -- 状态: created, starting, ready, running, paused, merging, completed, failed, cancelled
    status                  TEXT NOT NULL DEFAULT 'created',

    -- 斜杠命令配置
    use_slash_commands      INTEGER NOT NULL DEFAULT 0,

    -- 主 Agent (Orchestrator) 配置
    orchestrator_enabled    INTEGER NOT NULL DEFAULT 1,
    orchestrator_api_type   TEXT,                       -- 'openai' | 'anthropic' | 'custom'
    orchestrator_base_url   TEXT,
    orchestrator_api_key    TEXT,                       -- 加密存储
    orchestrator_model      TEXT,

    -- 错误处理终端配置（可选）
    error_terminal_enabled  INTEGER NOT NULL DEFAULT 0,
    error_terminal_cli_id   TEXT REFERENCES cli_type(id),
    error_terminal_model_id TEXT REFERENCES model_config(id),

    -- 合并终端配置（必需）
    merge_terminal_cli_id   TEXT NOT NULL REFERENCES cli_type(id),
    merge_terminal_model_id TEXT NOT NULL REFERENCES model_config(id),

    -- 目标分支
    target_branch           TEXT NOT NULL DEFAULT 'main',

    -- 时间戳
    ready_at                TEXT,                       -- 所有终端启动完成时间
    started_at              TEXT,                       -- 用户确认开始时间
    completed_at            TEXT,
    created_at              TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_project_id ON workflow(project_id);
CREATE INDEX IF NOT EXISTS idx_workflow_status ON workflow(status);

-- ----------------------------------------------------------------------------
-- 5. 工作流命令关联表 (workflow_command)
-- 存储工作流使用的斜杠命令及其顺序
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow_command (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    preset_id       TEXT NOT NULL REFERENCES slash_command_preset(id),
    order_index     INTEGER NOT NULL,                   -- 执行顺序，从 0 开始
    custom_params   TEXT,                               -- JSON 格式的自定义参数
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(workflow_id, order_index)
);

CREATE INDEX IF NOT EXISTS idx_workflow_command_workflow_id ON workflow_command(workflow_id);

-- ----------------------------------------------------------------------------
-- 6. 工作流任务表 (workflow_task)
-- 存储工作流中的并行任务
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow_task (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,

    -- 关联到 vibe-kanban 的 task 表（可选，用于同步状态）
    vk_task_id      TEXT REFERENCES task(id),

    name            TEXT NOT NULL,
    description     TEXT,
    branch          TEXT NOT NULL,                      -- Git 分支名

    -- 状态: pending, running, review_pending, completed, failed, cancelled
    status          TEXT NOT NULL DEFAULT 'pending',

    order_index     INTEGER NOT NULL,                   -- 任务顺序（用于 UI 显示）

    -- 时间戳
    started_at      TEXT,
    completed_at    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_id ON workflow_task(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_task_status ON workflow_task(status);

-- ----------------------------------------------------------------------------
-- 7. 终端表 (terminal)
-- 存储每个任务中的终端配置和状态
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS terminal (
    id                  TEXT PRIMARY KEY,
    workflow_task_id    TEXT NOT NULL REFERENCES workflow_task(id) ON DELETE CASCADE,

    -- CLI 和模型配置
    cli_type_id         TEXT NOT NULL REFERENCES cli_type(id),
    model_config_id     TEXT NOT NULL REFERENCES model_config(id),

    -- 自定义 API 配置（覆盖默认配置）
    custom_base_url     TEXT,
    custom_api_key      TEXT,                           -- 加密存储

    -- 角色描述（可选，用于主 Agent 理解终端职责）
    role                TEXT,                           -- 如 'coder', 'reviewer', 'fixer'
    role_description    TEXT,

    order_index         INTEGER NOT NULL,               -- 在任务内的执行顺序，从 0 开始

    -- 状态: not_started, starting, waiting, working, completed, failed, cancelled
    status              TEXT NOT NULL DEFAULT 'not_started',

    -- 进程信息
    process_id          INTEGER,                        -- 操作系统进程 ID
    pty_session_id      TEXT,                           -- PTY 会话 ID（用于终端调试视图）

    -- 关联到 vibe-kanban 的 session（可选）
    vk_session_id       TEXT REFERENCES session(id),

    -- 最后一次 Git 提交信息
    last_commit_hash    TEXT,
    last_commit_message TEXT,

    -- 时间戳
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_terminal_workflow_task_id ON terminal(workflow_task_id);
CREATE INDEX IF NOT EXISTS idx_terminal_status ON terminal(status);
CREATE INDEX IF NOT EXISTS idx_terminal_cli_type_id ON terminal(cli_type_id);

-- ----------------------------------------------------------------------------
-- 8. 终端日志表 (terminal_log)
-- 存储终端执行日志（用于调试和审计）
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS terminal_log (
    id              TEXT PRIMARY KEY,
    terminal_id     TEXT NOT NULL REFERENCES terminal(id) ON DELETE CASCADE,

    -- 日志类型: stdout, stderr, system, git_event
    log_type        TEXT NOT NULL,

    content         TEXT NOT NULL,

    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_terminal_log_terminal_id ON terminal_log(terminal_id);
CREATE INDEX IF NOT EXISTS idx_terminal_log_created_at ON terminal_log(created_at);

-- ----------------------------------------------------------------------------
-- 9. Git 事件表 (git_event)
-- 存储 Git 提交事件（用于事件驱动）
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS git_event (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    terminal_id     TEXT REFERENCES terminal(id),

    -- Git 信息
    commit_hash     TEXT NOT NULL,
    branch          TEXT NOT NULL,
    commit_message  TEXT NOT NULL,

    -- 解析后的元数据（JSON 格式）
    metadata        TEXT,

    -- 处理状态: pending, processing, processed, failed
    process_status  TEXT NOT NULL DEFAULT 'pending',

    -- 主 Agent 响应（JSON 格式）
    agent_response  TEXT,

    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at    TEXT
);

CREATE INDEX IF NOT EXISTS idx_git_event_workflow_id ON git_event(workflow_id);
CREATE INDEX IF NOT EXISTS idx_git_event_terminal_id ON git_event(terminal_id);
CREATE INDEX IF NOT EXISTS idx_git_event_process_status ON git_event(process_status);
```

**Step 1.1.3: 验证迁移文件语法**

```bash
cd F:\Project\SoloDawn\vibe-kanban-main
# 检查 SQL 语法（使用 sqlite3）
sqlite3 :memory: < crates/db/migrations/20260116000001_create_workflow_tables.sql
echo $?  # 应该输出 0
```

**交付物:**
- 文件: `vibe-kanban-main/crates/db/migrations/20260116000001_create_workflow_tables.sql`
- 包含 9 张表的完整 DDL
- 包含系统内置数据的 INSERT 语句

**验收标准:**
1. SQL 语法正确，可以在 SQLite 中执行
2. 所有外键关系正确
3. 索引覆盖常用查询字段
4. 系统内置数据完整

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo sqlx migrate run
# 预期输出: Applied 20260116000001_create_workflow_tables (xxx ms)
```

---

### Task 1.2: 创建 Workflow Rust 模型

**状态:** ⬜ 未开始

**前置条件:**
- Task 1.1 已完成
- 迁移文件已成功执行

**目标:**
创建与数据库表对应的 Rust 结构体，支持 sqlx 查询和 TypeScript 类型导出。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/db/src/models/workflow.rs`
- 创建: `vibe-kanban-main/crates/db/src/models/cli_type.rs`
- 创建: `vibe-kanban-main/crates/db/src/models/terminal.rs`
- 修改: `vibe-kanban-main/crates/db/src/models/mod.rs`

**参考现有模型:**
查看 `vibe-kanban-main/crates/db/src/models/task.rs` 了解模型定义规范：
- 使用 `#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]`
- 使用 `#[ts(export)]` 导出 TypeScript 类型
- 字段命名使用 snake_case

---

**Step 1.2.1: 创建 cli_type.rs**

文件路径: `vibe-kanban-main/crates/db/src/models/cli_type.rs`

```rust
//! CLI 类型模型
//!
//! 存储支持的 AI 编码代理 CLI 信息，如 Claude Code、Gemini CLI、Codex 等。

use serde::{Deserialize, Serialize};
use sqlx::FromRow;
use ts_rs::TS;

/// CLI 类型
///
/// 对应数据库表: cli_type
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct CliType {
    /// 主键 ID，格式: cli-{name}
    pub id: String,

    /// 内部名称，如 'claude-code'
    pub name: String,

    /// 显示名称，如 'Claude Code'
    pub display_name: String,

    /// 检测命令，如 'claude --version'
    pub detect_command: String,

    /// 安装命令（可选）
    pub install_command: Option<String>,

    /// 安装指南 URL
    pub install_guide_url: Option<String>,

    /// 配置文件路径模板，如 '~/.claude/settings.json'
    pub config_file_path: Option<String>,

    /// 是否系统内置
    #[sqlx(default)]
    pub is_system: bool,

    /// 创建时间
    pub created_at: String,
}

/// 模型配置
///
/// 对应数据库表: model_config
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct ModelConfig {
    /// 主键 ID，格式: model-{cli}-{name}
    pub id: String,

    /// 关联的 CLI 类型 ID
    pub cli_type_id: String,

    /// 模型内部名称，如 'sonnet'
    pub name: String,

    /// 显示名称，如 'Claude Sonnet'
    pub display_name: String,

    /// API 模型 ID，如 'claude-sonnet-4-20250514'
    pub api_model_id: Option<String>,

    /// 是否默认模型
    #[sqlx(default)]
    pub is_default: bool,

    /// 是否官方模型
    #[sqlx(default)]
    pub is_official: bool,

    /// 创建时间
    pub created_at: String,

    /// 更新时间
    pub updated_at: String,
}

/// CLI 检测状态
///
/// 用于前端显示 CLI 安装状态
#[derive(Debug, Clone, Serialize, Deserialize, TS)]
#[ts(export)]
pub struct CliDetectionStatus {
    /// CLI 类型 ID
    pub cli_type_id: String,

    /// CLI 名称
    pub name: String,

    /// 显示名称
    pub display_name: String,

    /// 是否已安装
    pub installed: bool,

    /// 版本号（如果已安装）
    pub version: Option<String>,

    /// 可执行文件路径（如果已安装）
    pub executable_path: Option<String>,

    /// 安装指南 URL
    pub install_guide_url: Option<String>,
}
```

---

**Step 1.2.2: 创建 workflow.rs**

文件路径: `vibe-kanban-main/crates/db/src/models/workflow.rs`

```rust
//! 工作流模型
//!
//! 存储工作流配置和状态，支持主 Agent 跨终端任务协调。

use serde::{Deserialize, Serialize};
use sqlx::FromRow;
use ts_rs::TS;

/// 工作流状态枚举
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, TS)]
#[ts(export)]
#[serde(rename_all = "snake_case")]
pub enum WorkflowStatus {
    /// 已创建，等待配置
    Created,
    /// 正在启动终端
    Starting,
    /// 所有终端已就绪，等待用户确认开始
    Ready,
    /// 正在运行
    Running,
    /// 已暂停
    Paused,
    /// 正在合并分支
    Merging,
    /// 已完成
    Completed,
    /// 失败
    Failed,
    /// 已取消
    Cancelled,
}

impl WorkflowStatus {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Created => "created",
            Self::Starting => "starting",
            Self::Ready => "ready",
            Self::Running => "running",
            Self::Paused => "paused",
            Self::Merging => "merging",
            Self::Completed => "completed",
            Self::Failed => "failed",
            Self::Cancelled => "cancelled",
        }
    }

    pub fn from_str(s: &str) -> Option<Self> {
        match s {
            "created" => Some(Self::Created),
            "starting" => Some(Self::Starting),
            "ready" => Some(Self::Ready),
            "running" => Some(Self::Running),
            "paused" => Some(Self::Paused),
            "merging" => Some(Self::Merging),
            "completed" => Some(Self::Completed),
            "failed" => Some(Self::Failed),
            "cancelled" => Some(Self::Cancelled),
            _ => None,
        }
    }
}

/// 工作流
///
/// 对应数据库表: workflow
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct Workflow {
    /// 主键 ID (UUID)
    pub id: String,

    /// 关联的项目 ID
    pub project_id: String,

    /// 工作流名称
    pub name: String,

    /// 工作流描述
    pub description: Option<String>,

    /// 状态
    pub status: String,

    /// 是否使用斜杠命令
    #[sqlx(default)]
    pub use_slash_commands: bool,

    /// 是否启用主 Agent
    #[sqlx(default)]
    pub orchestrator_enabled: bool,

    /// 主 Agent API 类型: 'openai' | 'anthropic' | 'custom'
    pub orchestrator_api_type: Option<String>,

    /// 主 Agent API Base URL
    pub orchestrator_base_url: Option<String>,

    /// 主 Agent API Key（加密存储）
    pub orchestrator_api_key: Option<String>,

    /// 主 Agent 模型
    pub orchestrator_model: Option<String>,

    /// 是否启用错误处理终端
    #[sqlx(default)]
    pub error_terminal_enabled: bool,

    /// 错误处理终端 CLI ID
    pub error_terminal_cli_id: Option<String>,

    /// 错误处理终端模型 ID
    pub error_terminal_model_id: Option<String>,

    /// 合并终端 CLI ID
    pub merge_terminal_cli_id: String,

    /// 合并终端模型 ID
    pub merge_terminal_model_id: String,

    /// 目标分支
    pub target_branch: String,

    /// 所有终端就绪时间
    pub ready_at: Option<String>,

    /// 用户确认开始时间
    pub started_at: Option<String>,

    /// 完成时间
    pub completed_at: Option<String>,

    /// 创建时间
    pub created_at: String,

    /// 更新时间
    pub updated_at: String,
}

/// 工作流任务状态枚举
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, TS)]
#[ts(export)]
#[serde(rename_all = "snake_case")]
pub enum WorkflowTaskStatus {
    /// 等待执行
    Pending,
    /// 正在运行
    Running,
    /// 等待审核
    ReviewPending,
    /// 已完成
    Completed,
    /// 失败
    Failed,
    /// 已取消
    Cancelled,
}

/// 工作流任务
///
/// 对应数据库表: workflow_task
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct WorkflowTask {
    /// 主键 ID (UUID)
    pub id: String,

    /// 关联的工作流 ID
    pub workflow_id: String,

    /// 关联的 vibe-kanban task ID（可选）
    pub vk_task_id: Option<String>,

    /// 任务名称
    pub name: String,

    /// 任务描述
    pub description: Option<String>,

    /// Git 分支名
    pub branch: String,

    /// 状态
    pub status: String,

    /// 任务顺序
    pub order_index: i32,

    /// 开始时间
    pub started_at: Option<String>,

    /// 完成时间
    pub completed_at: Option<String>,

    /// 创建时间
    pub created_at: String,

    /// 更新时间
    pub updated_at: String,
}

/// 斜杠命令预设
///
/// 对应数据库表: slash_command_preset
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct SlashCommandPreset {
    /// 主键 ID
    pub id: String,

    /// 命令名，如 '/write-code'
    pub command: String,

    /// 命令描述
    pub description: String,

    /// 提示词模板
    pub prompt_template: Option<String>,

    /// 是否系统内置
    #[sqlx(default)]
    pub is_system: bool,

    /// 创建时间
    pub created_at: String,

    /// 更新时间
    pub updated_at: String,
}

/// 工作流命令关联
///
/// 对应数据库表: workflow_command
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct WorkflowCommand {
    /// 主键 ID
    pub id: String,

    /// 关联的工作流 ID
    pub workflow_id: String,

    /// 关联的预设 ID
    pub preset_id: String,

    /// 执行顺序
    pub order_index: i32,

    /// 自定义参数（JSON 格式）
    pub custom_params: Option<String>,

    /// 创建时间
    pub created_at: String,
}
```

---

**Step 1.2.3: 创建 terminal.rs**

文件路径: `vibe-kanban-main/crates/db/src/models/terminal.rs`

```rust
//! 终端模型
//!
//! 存储每个任务中的终端配置和状态。

use serde::{Deserialize, Serialize};
use sqlx::FromRow;
use ts_rs::TS;

/// 终端状态枚举
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, TS)]
#[ts(export)]
#[serde(rename_all = "snake_case")]
pub enum TerminalStatus {
    /// 未启动
    NotStarted,
    /// 正在启动
    Starting,
    /// 等待中（已启动，等待指令）
    Waiting,
    /// 工作中
    Working,
    /// 已完成
    Completed,
    /// 失败
    Failed,
    /// 已取消
    Cancelled,
}

impl TerminalStatus {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::NotStarted => "not_started",
            Self::Starting => "starting",
            Self::Waiting => "waiting",
            Self::Working => "working",
            Self::Completed => "completed",
            Self::Failed => "failed",
            Self::Cancelled => "cancelled",
        }
    }

    pub fn from_str(s: &str) -> Option<Self> {
        match s {
            "not_started" => Some(Self::NotStarted),
            "starting" => Some(Self::Starting),
            "waiting" => Some(Self::Waiting),
            "working" => Some(Self::Working),
            "completed" => Some(Self::Completed),
            "failed" => Some(Self::Failed),
            "cancelled" => Some(Self::Cancelled),
            _ => None,
        }
    }
}

/// 终端
///
/// 对应数据库表: terminal
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct Terminal {
    /// 主键 ID (UUID)
    pub id: String,

    /// 关联的工作流任务 ID
    pub workflow_task_id: String,

    /// CLI 类型 ID
    pub cli_type_id: String,

    /// 模型配置 ID
    pub model_config_id: String,

    /// 自定义 API Base URL
    pub custom_base_url: Option<String>,

    /// 自定义 API Key（加密存储）
    pub custom_api_key: Option<String>,

    /// 角色，如 'coder', 'reviewer', 'fixer'
    pub role: Option<String>,

    /// 角色描述
    pub role_description: Option<String>,

    /// 在任务内的执行顺序
    pub order_index: i32,

    /// 状态
    pub status: String,

    /// 操作系统进程 ID
    pub process_id: Option<i32>,

    /// PTY 会话 ID
    pub pty_session_id: Option<String>,

    /// 关联的 vibe-kanban session ID
    pub vk_session_id: Option<String>,

    /// 最后一次 Git 提交哈希
    pub last_commit_hash: Option<String>,

    /// 最后一次 Git 提交消息
    pub last_commit_message: Option<String>,

    /// 启动时间
    pub started_at: Option<String>,

    /// 完成时间
    pub completed_at: Option<String>,

    /// 创建时间
    pub created_at: String,

    /// 更新时间
    pub updated_at: String,
}

/// 终端日志类型
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, TS)]
#[ts(export)]
#[serde(rename_all = "snake_case")]
pub enum TerminalLogType {
    Stdout,
    Stderr,
    System,
    GitEvent,
}

/// 终端日志
///
/// 对应数据库表: terminal_log
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct TerminalLog {
    /// 主键 ID
    pub id: String,

    /// 关联的终端 ID
    pub terminal_id: String,

    /// 日志类型
    pub log_type: String,

    /// 日志内容
    pub content: String,

    /// 创建时间
    pub created_at: String,
}

/// Git 事件
///
/// 对应数据库表: git_event
#[derive(Debug, Clone, Serialize, Deserialize, FromRow, TS)]
#[ts(export)]
pub struct GitEvent {
    /// 主键 ID
    pub id: String,

    /// 关联的工作流 ID
    pub workflow_id: String,

    /// 关联的终端 ID（可选）
    pub terminal_id: Option<String>,

    /// Git 提交哈希
    pub commit_hash: String,

    /// Git 分支
    pub branch: String,

    /// 提交消息
    pub commit_message: String,

    /// 解析后的元数据（JSON 格式）
    pub metadata: Option<String>,

    /// 处理状态
    pub process_status: String,

    /// 主 Agent 响应（JSON 格式）
    pub agent_response: Option<String>,

    /// 创建时间
    pub created_at: String,

    /// 处理时间
    pub processed_at: Option<String>,
}

/// 终端详情（包含关联的 CLI 和模型信息）
///
/// 用于 API 响应，包含完整的终端信息
#[derive(Debug, Clone, Serialize, Deserialize, TS)]
#[ts(export)]
pub struct TerminalDetail {
    /// 终端基本信息
    #[serde(flatten)]
    pub terminal: Terminal,

    /// CLI 类型信息
    pub cli_type: super::cli_type::CliType,

    /// 模型配置信息
    pub model_config: super::cli_type::ModelConfig,
}
```

---

**Step 1.2.4: 修改 mod.rs 导出新模型**

文件路径: `vibe-kanban-main/crates/db/src/models/mod.rs`

在文件末尾添加：

```rust
// SoloDawn Workflow 模型
pub mod cli_type;
pub mod workflow;
pub mod terminal;

pub use cli_type::*;
pub use workflow::*;
pub use terminal::*;
```

---

**交付物:**
- `vibe-kanban-main/crates/db/src/models/cli_type.rs` - CLI 类型和模型配置
- `vibe-kanban-main/crates/db/src/models/workflow.rs` - 工作流相关模型
- `vibe-kanban-main/crates/db/src/models/terminal.rs` - 终端相关模型
- 修改后的 `mod.rs`

**验收标准:**
1. 所有结构体字段与数据库表字段一一对应
2. 编译通过：`cd vibe-kanban-main && cargo build -p db`
3. TypeScript 类型生成成功：`cargo run --bin generate_types`

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p db
# 预期: 编译成功，无错误

cargo run --bin generate_types
# 预期: 在 shared/types.ts 中生成新的类型定义
```

---

### Task 1.3: 创建数据库访问层 (DAO)

**状态:** ⬜ 未开始

**前置条件:**
- Task 1.2 已完成
- 模型编译通过

**目标:**
为新模型创建 CRUD 操作函数，遵循 vibe-kanban 现有的数据库访问模式。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/db/src/models/workflow_dao.rs`
- 创建: `vibe-kanban-main/crates/db/src/models/terminal_dao.rs`
- 创建: `vibe-kanban-main/crates/db/src/models/cli_type_dao.rs`
- 修改: `vibe-kanban-main/crates/db/src/models/mod.rs`

**参考现有 DAO:**
查看 `vibe-kanban-main/crates/db/src/models/task.rs` 中的查询函数模式。

---

**Step 1.3.1: 创建 cli_type_dao.rs**

文件路径: `vibe-kanban-main/crates/db/src/models/cli_type_dao.rs`

```rust
//! CLI 类型数据访问层

use sqlx::{Pool, Sqlite};
use super::{CliType, ModelConfig};

/// 获取所有 CLI 类型
pub async fn get_all_cli_types(pool: &Pool<Sqlite>) -> Result<Vec<CliType>, sqlx::Error> {
    sqlx::query_as::<_, CliType>(
        r#"
        SELECT id, name, display_name, detect_command, install_command,
               install_guide_url, config_file_path, is_system, created_at
        FROM cli_type
        ORDER BY is_system DESC, name ASC
        "#
    )
    .fetch_all(pool)
    .await
}

/// 根据 ID 获取 CLI 类型
pub async fn get_cli_type_by_id(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<Option<CliType>, sqlx::Error> {
    sqlx::query_as::<_, CliType>(
        r#"
        SELECT id, name, display_name, detect_command, install_command,
               install_guide_url, config_file_path, is_system, created_at
        FROM cli_type
        WHERE id = ?
        "#
    )
    .bind(id)
    .fetch_optional(pool)
    .await
}

/// 根据名称获取 CLI 类型
pub async fn get_cli_type_by_name(
    pool: &Pool<Sqlite>,
    name: &str,
) -> Result<Option<CliType>, sqlx::Error> {
    sqlx::query_as::<_, CliType>(
        r#"
        SELECT id, name, display_name, detect_command, install_command,
               install_guide_url, config_file_path, is_system, created_at
        FROM cli_type
        WHERE name = ?
        "#
    )
    .bind(name)
    .fetch_optional(pool)
    .await
}

/// 获取 CLI 类型的所有模型配置
pub async fn get_models_by_cli_type(
    pool: &Pool<Sqlite>,
    cli_type_id: &str,
) -> Result<Vec<ModelConfig>, sqlx::Error> {
    sqlx::query_as::<_, ModelConfig>(
        r#"
        SELECT id, cli_type_id, name, display_name, api_model_id,
               is_default, is_official, created_at, updated_at
        FROM model_config
        WHERE cli_type_id = ?
        ORDER BY is_default DESC, name ASC
        "#
    )
    .bind(cli_type_id)
    .fetch_all(pool)
    .await
}

/// 根据 ID 获取模型配置
pub async fn get_model_config_by_id(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<Option<ModelConfig>, sqlx::Error> {
    sqlx::query_as::<_, ModelConfig>(
        r#"
        SELECT id, cli_type_id, name, display_name, api_model_id,
               is_default, is_official, created_at, updated_at
        FROM model_config
        WHERE id = ?
        "#
    )
    .bind(id)
    .fetch_optional(pool)
    .await
}

/// 获取 CLI 类型的默认模型
pub async fn get_default_model_for_cli(
    pool: &Pool<Sqlite>,
    cli_type_id: &str,
) -> Result<Option<ModelConfig>, sqlx::Error> {
    sqlx::query_as::<_, ModelConfig>(
        r#"
        SELECT id, cli_type_id, name, display_name, api_model_id,
               is_default, is_official, created_at, updated_at
        FROM model_config
        WHERE cli_type_id = ? AND is_default = 1
        LIMIT 1
        "#
    )
    .bind(cli_type_id)
    .fetch_optional(pool)
    .await
}
```

---

**Step 1.3.2: 创建 workflow_dao.rs**

文件路径: `vibe-kanban-main/crates/db/src/models/workflow_dao.rs`

```rust
//! 工作流数据访问层

use sqlx::{Pool, Sqlite};
use uuid::Uuid;
use super::{Workflow, WorkflowTask, WorkflowCommand, SlashCommandPreset};

// ============================================================================
// Workflow CRUD
// ============================================================================

/// 创建工作流
pub async fn create_workflow(
    pool: &Pool<Sqlite>,
    workflow: &Workflow,
) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        INSERT INTO workflow (
            id, project_id, name, description, status,
            use_slash_commands, orchestrator_enabled,
            orchestrator_api_type, orchestrator_base_url,
            orchestrator_api_key, orchestrator_model,
            error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
            merge_terminal_cli_id, merge_terminal_model_id,
            target_branch, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        "#
    )
    .bind(&workflow.id)
    .bind(&workflow.project_id)
    .bind(&workflow.name)
    .bind(&workflow.description)
    .bind(&workflow.status)
    .bind(workflow.use_slash_commands)
    .bind(workflow.orchestrator_enabled)
    .bind(&workflow.orchestrator_api_type)
    .bind(&workflow.orchestrator_base_url)
    .bind(&workflow.orchestrator_api_key)
    .bind(&workflow.orchestrator_model)
    .bind(workflow.error_terminal_enabled)
    .bind(&workflow.error_terminal_cli_id)
    .bind(&workflow.error_terminal_model_id)
    .bind(&workflow.merge_terminal_cli_id)
    .bind(&workflow.merge_terminal_model_id)
    .bind(&workflow.target_branch)
    .bind(&workflow.created_at)
    .bind(&workflow.updated_at)
    .execute(pool)
    .await?;
    Ok(())
}

/// 根据 ID 获取工作流
pub async fn get_workflow_by_id(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<Option<Workflow>, sqlx::Error> {
    sqlx::query_as::<_, Workflow>(
        r#"
        SELECT * FROM workflow WHERE id = ?
        "#
    )
    .bind(id)
    .fetch_optional(pool)
    .await
}

/// 获取项目的所有工作流
pub async fn get_workflows_by_project(
    pool: &Pool<Sqlite>,
    project_id: &str,
) -> Result<Vec<Workflow>, sqlx::Error> {
    sqlx::query_as::<_, Workflow>(
        r#"
        SELECT * FROM workflow
        WHERE project_id = ?
        ORDER BY created_at DESC
        "#
    )
    .bind(project_id)
    .fetch_all(pool)
    .await
}

/// 更新工作流状态
pub async fn update_workflow_status(
    pool: &Pool<Sqlite>,
    id: &str,
    status: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE workflow
        SET status = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(status)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 设置工作流就绪时间
pub async fn set_workflow_ready(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE workflow
        SET status = 'ready', ready_at = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(&now)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 设置工作流开始时间
pub async fn set_workflow_started(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE workflow
        SET status = 'running', started_at = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(&now)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 删除工作流
pub async fn delete_workflow(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<(), sqlx::Error> {
    sqlx::query("DELETE FROM workflow WHERE id = ?")
        .bind(id)
        .execute(pool)
        .await?;
    Ok(())
}

// ============================================================================
// WorkflowTask CRUD
// ============================================================================

/// 创建工作流任务
pub async fn create_workflow_task(
    pool: &Pool<Sqlite>,
    task: &WorkflowTask,
) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        INSERT INTO workflow_task (
            id, workflow_id, vk_task_id, name, description,
            branch, status, order_index, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        "#
    )
    .bind(&task.id)
    .bind(&task.workflow_id)
    .bind(&task.vk_task_id)
    .bind(&task.name)
    .bind(&task.description)
    .bind(&task.branch)
    .bind(&task.status)
    .bind(task.order_index)
    .bind(&task.created_at)
    .bind(&task.updated_at)
    .execute(pool)
    .await?;
    Ok(())
}

/// 获取工作流的所有任务
pub async fn get_tasks_by_workflow(
    pool: &Pool<Sqlite>,
    workflow_id: &str,
) -> Result<Vec<WorkflowTask>, sqlx::Error> {
    sqlx::query_as::<_, WorkflowTask>(
        r#"
        SELECT * FROM workflow_task
        WHERE workflow_id = ?
        ORDER BY order_index ASC
        "#
    )
    .bind(workflow_id)
    .fetch_all(pool)
    .await
}

/// 根据 ID 获取工作流任务
pub async fn get_workflow_task_by_id(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<Option<WorkflowTask>, sqlx::Error> {
    sqlx::query_as::<_, WorkflowTask>(
        r#"SELECT * FROM workflow_task WHERE id = ?"#
    )
    .bind(id)
    .fetch_optional(pool)
    .await
}

/// 更新工作流任务状态
pub async fn update_workflow_task_status(
    pool: &Pool<Sqlite>,
    id: &str,
    status: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE workflow_task
        SET status = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(status)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

// ============================================================================
// SlashCommandPreset & WorkflowCommand
// ============================================================================

/// 获取所有斜杠命令预设
pub async fn get_all_slash_command_presets(
    pool: &Pool<Sqlite>,
) -> Result<Vec<SlashCommandPreset>, sqlx::Error> {
    sqlx::query_as::<_, SlashCommandPreset>(
        r#"
        SELECT * FROM slash_command_preset
        ORDER BY is_system DESC, command ASC
        "#
    )
    .fetch_all(pool)
    .await
}

/// 获取工作流的命令列表
pub async fn get_commands_by_workflow(
    pool: &Pool<Sqlite>,
    workflow_id: &str,
) -> Result<Vec<WorkflowCommand>, sqlx::Error> {
    sqlx::query_as::<_, WorkflowCommand>(
        r#"
        SELECT * FROM workflow_command
        WHERE workflow_id = ?
        ORDER BY order_index ASC
        "#
    )
    .bind(workflow_id)
    .fetch_all(pool)
    .await
}

/// 为工作流添加命令
pub async fn add_workflow_command(
    pool: &Pool<Sqlite>,
    workflow_id: &str,
    preset_id: &str,
    order_index: i32,
    custom_params: Option<&str>,
) -> Result<String, sqlx::Error> {
    let id = Uuid::new_v4().to_string();
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        INSERT INTO workflow_command (id, workflow_id, preset_id, order_index, custom_params, created_at)
        VALUES (?, ?, ?, ?, ?, ?)
        "#
    )
    .bind(&id)
    .bind(workflow_id)
    .bind(preset_id)
    .bind(order_index)
    .bind(custom_params)
    .bind(&now)
    .execute(pool)
    .await?;
    Ok(id)
}
```

---

**Step 1.3.3: 创建 terminal_dao.rs**

文件路径: `vibe-kanban-main/crates/db/src/models/terminal_dao.rs`

```rust
//! 终端数据访问层

use sqlx::{Pool, Sqlite};
use uuid::Uuid;
use super::{Terminal, TerminalLog, GitEvent};

// ============================================================================
// Terminal CRUD
// ============================================================================

/// 创建终端
pub async fn create_terminal(
    pool: &Pool<Sqlite>,
    terminal: &Terminal,
) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        INSERT INTO terminal (
            id, workflow_task_id, cli_type_id, model_config_id,
            custom_base_url, custom_api_key, role, role_description,
            order_index, status, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        "#
    )
    .bind(&terminal.id)
    .bind(&terminal.workflow_task_id)
    .bind(&terminal.cli_type_id)
    .bind(&terminal.model_config_id)
    .bind(&terminal.custom_base_url)
    .bind(&terminal.custom_api_key)
    .bind(&terminal.role)
    .bind(&terminal.role_description)
    .bind(terminal.order_index)
    .bind(&terminal.status)
    .bind(&terminal.created_at)
    .bind(&terminal.updated_at)
    .execute(pool)
    .await?;
    Ok(())
}

/// 根据 ID 获取终端
pub async fn get_terminal_by_id(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<Option<Terminal>, sqlx::Error> {
    sqlx::query_as::<_, Terminal>(
        r#"SELECT * FROM terminal WHERE id = ?"#
    )
    .bind(id)
    .fetch_optional(pool)
    .await
}

/// 获取工作流任务的所有终端
pub async fn get_terminals_by_task(
    pool: &Pool<Sqlite>,
    workflow_task_id: &str,
) -> Result<Vec<Terminal>, sqlx::Error> {
    sqlx::query_as::<_, Terminal>(
        r#"
        SELECT * FROM terminal
        WHERE workflow_task_id = ?
        ORDER BY order_index ASC
        "#
    )
    .bind(workflow_task_id)
    .fetch_all(pool)
    .await
}

/// 获取工作流的所有终端（跨任务）
pub async fn get_terminals_by_workflow(
    pool: &Pool<Sqlite>,
    workflow_id: &str,
) -> Result<Vec<Terminal>, sqlx::Error> {
    sqlx::query_as::<_, Terminal>(
        r#"
        SELECT t.* FROM terminal t
        INNER JOIN workflow_task wt ON t.workflow_task_id = wt.id
        WHERE wt.workflow_id = ?
        ORDER BY wt.order_index ASC, t.order_index ASC
        "#
    )
    .bind(workflow_id)
    .fetch_all(pool)
    .await
}

/// 更新终端状态
pub async fn update_terminal_status(
    pool: &Pool<Sqlite>,
    id: &str,
    status: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE terminal
        SET status = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(status)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 更新终端进程信息
pub async fn update_terminal_process(
    pool: &Pool<Sqlite>,
    id: &str,
    process_id: Option<i32>,
    pty_session_id: Option<&str>,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE terminal
        SET process_id = ?, pty_session_id = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(process_id)
    .bind(pty_session_id)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 更新终端最后提交信息
pub async fn update_terminal_last_commit(
    pool: &Pool<Sqlite>,
    id: &str,
    commit_hash: &str,
    commit_message: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE terminal
        SET last_commit_hash = ?, last_commit_message = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(commit_hash)
    .bind(commit_message)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 设置终端启动时间
pub async fn set_terminal_started(
    pool: &Pool<Sqlite>,
    id: &str,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE terminal
        SET status = 'waiting', started_at = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(&now)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

/// 设置终端完成时间
pub async fn set_terminal_completed(
    pool: &Pool<Sqlite>,
    id: &str,
    status: &str, // 'completed' 或 'failed'
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE terminal
        SET status = ?, completed_at = ?, updated_at = ?
        WHERE id = ?
        "#
    )
    .bind(status)
    .bind(&now)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

// ============================================================================
// TerminalLog CRUD
// ============================================================================

/// 添加终端日志
pub async fn add_terminal_log(
    pool: &Pool<Sqlite>,
    terminal_id: &str,
    log_type: &str,
    content: &str,
) -> Result<String, sqlx::Error> {
    let id = Uuid::new_v4().to_string();
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        INSERT INTO terminal_log (id, terminal_id, log_type, content, created_at)
        VALUES (?, ?, ?, ?, ?)
        "#
    )
    .bind(&id)
    .bind(terminal_id)
    .bind(log_type)
    .bind(content)
    .bind(&now)
    .execute(pool)
    .await?;
    Ok(id)
}

/// 获取终端的日志
pub async fn get_logs_by_terminal(
    pool: &Pool<Sqlite>,
    terminal_id: &str,
    limit: Option<i32>,
) -> Result<Vec<TerminalLog>, sqlx::Error> {
    let limit = limit.unwrap_or(1000);
    sqlx::query_as::<_, TerminalLog>(
        r#"
        SELECT * FROM terminal_log
        WHERE terminal_id = ?
        ORDER BY created_at DESC
        LIMIT ?
        "#
    )
    .bind(terminal_id)
    .bind(limit)
    .fetch_all(pool)
    .await
}

// ============================================================================
// GitEvent CRUD
// ============================================================================

/// 创建 Git 事件
pub async fn create_git_event(
    pool: &Pool<Sqlite>,
    workflow_id: &str,
    terminal_id: Option<&str>,
    commit_hash: &str,
    branch: &str,
    commit_message: &str,
    metadata: Option<&str>,
) -> Result<String, sqlx::Error> {
    let id = Uuid::new_v4().to_string();
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        INSERT INTO git_event (
            id, workflow_id, terminal_id, commit_hash, branch,
            commit_message, metadata, process_status, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)
        "#
    )
    .bind(&id)
    .bind(workflow_id)
    .bind(terminal_id)
    .bind(commit_hash)
    .bind(branch)
    .bind(commit_message)
    .bind(metadata)
    .bind(&now)
    .execute(pool)
    .await?;
    Ok(id)
}

/// 获取待处理的 Git 事件
pub async fn get_pending_git_events(
    pool: &Pool<Sqlite>,
    workflow_id: &str,
) -> Result<Vec<GitEvent>, sqlx::Error> {
    sqlx::query_as::<_, GitEvent>(
        r#"
        SELECT * FROM git_event
        WHERE workflow_id = ? AND process_status = 'pending'
        ORDER BY created_at ASC
        "#
    )
    .bind(workflow_id)
    .fetch_all(pool)
    .await
}

/// 更新 Git 事件处理状态
pub async fn update_git_event_status(
    pool: &Pool<Sqlite>,
    id: &str,
    status: &str,
    agent_response: Option<&str>,
) -> Result<(), sqlx::Error> {
    let now = chrono::Utc::now().to_rfc3339();
    sqlx::query(
        r#"
        UPDATE git_event
        SET process_status = ?, agent_response = ?, processed_at = ?
        WHERE id = ?
        "#
    )
    .bind(status)
    .bind(agent_response)
    .bind(&now)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}
```

---

**Step 1.3.4: 更新 mod.rs 导出 DAO 模块**

在 `vibe-kanban-main/crates/db/src/models/mod.rs` 末尾添加：

```rust
// SoloDawn DAO 模块
pub mod cli_type_dao;
pub mod workflow_dao;
pub mod terminal_dao;
```

---

**交付物:**
- `vibe-kanban-main/crates/db/src/models/cli_type_dao.rs`
- `vibe-kanban-main/crates/db/src/models/workflow_dao.rs`
- `vibe-kanban-main/crates/db/src/models/terminal_dao.rs`
- 更新后的 `mod.rs`

**验收标准:**
1. 所有 DAO 函数编译通过
2. SQL 查询语法正确
3. 函数签名与模型字段匹配

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p db
# 预期: 编译成功

# 可选：运行单元测试（如果有）
cargo test -p db
```

---

### Task 1.4: 创建 API 路由

**状态:** ⬜ 未开始

**前置条件:**
- Task 1.3 已完成
- DAO 层编译通过

**目标:**
为工作流相关功能创建 REST API 路由。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/server/src/routes/workflows.rs`
- 创建: `vibe-kanban-main/crates/server/src/routes/cli_types.rs`
- 修改: `vibe-kanban-main/crates/server/src/routes/mod.rs`

**参考现有路由:**
查看 `vibe-kanban-main/crates/server/src/routes/tasks.rs` 了解路由定义规范。

---

**Step 1.4.1: 创建 cli_types.rs 路由**

文件路径: `vibe-kanban-main/crates/server/src/routes/cli_types.rs`

```rust
//! CLI 类型 API 路由

use axum::{
    extract::State,
    routing::get,
    Json, Router,
};
use db::models::{cli_type_dao, CliType, ModelConfig, CliDetectionStatus};
use crate::error::ApiError;
use std::sync::Arc;
use crate::AppState;

/// 创建 CLI 类型路由
pub fn cli_types_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/", get(list_cli_types))
        .route("/detect", get(detect_cli_types))
        .route("/:cli_type_id/models", get(list_models_for_cli))
}

/// GET /api/cli_types
/// 获取所有 CLI 类型
async fn list_cli_types(
    State(state): State<Arc<AppState>>,
) -> Result<Json<Vec<CliType>>, ApiError> {
    let cli_types = cli_type_dao::get_all_cli_types(&state.db.pool).await?;
    Ok(Json(cli_types))
}

/// GET /api/cli_types/detect
/// 检测已安装的 CLI
async fn detect_cli_types(
    State(state): State<Arc<AppState>>,
) -> Result<Json<Vec<CliDetectionStatus>>, ApiError> {
    let cli_types = cli_type_dao::get_all_cli_types(&state.db.pool).await?;
    let mut results = Vec::new();

    for cli_type in cli_types {
        let status = detect_single_cli(&cli_type).await;
        results.push(status);
    }

    Ok(Json(results))
}

/// 检测单个 CLI 是否安装
async fn detect_single_cli(cli_type: &CliType) -> CliDetectionStatus {
    use tokio::process::Command;

    // 解析检测命令
    let parts: Vec<&str> = cli_type.detect_command.split_whitespace().collect();
    if parts.is_empty() {
        return CliDetectionStatus {
            cli_type_id: cli_type.id.clone(),
            name: cli_type.name.clone(),
            display_name: cli_type.display_name.clone(),
            installed: false,
            version: None,
            executable_path: None,
            install_guide_url: cli_type.install_guide_url.clone(),
        };
    }

    let cmd = parts[0];
    let args = &parts[1..];

    // 执行检测命令
    let result = Command::new(cmd)
        .args(args)
        .output()
        .await;

    match result {
        Ok(output) if output.status.success() => {
            let version = String::from_utf8_lossy(&output.stdout)
                .lines()
                .next()
                .map(|s| s.trim().to_string());

            // 尝试获取可执行文件路径
            let executable_path = which::which(cmd)
                .ok()
                .map(|p| p.to_string_lossy().to_string());

            CliDetectionStatus {
                cli_type_id: cli_type.id.clone(),
                name: cli_type.name.clone(),
                display_name: cli_type.display_name.clone(),
                installed: true,
                version,
                executable_path,
                install_guide_url: cli_type.install_guide_url.clone(),
            }
        }
        _ => CliDetectionStatus {
            cli_type_id: cli_type.id.clone(),
            name: cli_type.name.clone(),
            display_name: cli_type.display_name.clone(),
            installed: false,
            version: None,
            executable_path: None,
            install_guide_url: cli_type.install_guide_url.clone(),
        },
    }
}

/// GET /api/cli_types/:cli_type_id/models
/// 获取 CLI 类型的所有模型
async fn list_models_for_cli(
    State(state): State<Arc<AppState>>,
    axum::extract::Path(cli_type_id): axum::extract::Path<String>,
) -> Result<Json<Vec<ModelConfig>>, ApiError> {
    let models = cli_type_dao::get_models_by_cli_type(&state.db.pool, &cli_type_id).await?;
    Ok(Json(models))
}
```

---

**Step 1.4.2: 创建 workflows.rs 路由（第一部分：请求/响应类型）**

文件路径: `vibe-kanban-main/crates/server/src/routes/workflows.rs`

```rust
//! 工作流 API 路由

use axum::{
    extract::{Path, State},
    routing::{get, post, put, delete},
    Json, Router,
};
use db::models::{
    workflow_dao, terminal_dao,
    Workflow, WorkflowTask, Terminal, SlashCommandPreset, WorkflowCommand,
};
use serde::{Deserialize, Serialize};
use ts_rs::TS;
use uuid::Uuid;
use std::sync::Arc;
use crate::error::ApiError;
use crate::AppState;

// ============================================================================
// 请求/响应类型
// ============================================================================

/// 创建工作流请求
#[derive(Debug, Deserialize, TS)]
#[ts(export)]
pub struct CreateWorkflowRequest {
    /// 项目 ID
    pub project_id: String,
    /// 工作流名称
    pub name: String,
    /// 工作流描述
    pub description: Option<String>,
    /// 是否使用斜杠命令
    pub use_slash_commands: bool,
    /// 斜杠命令 ID 列表（按顺序）
    pub command_preset_ids: Option<Vec<String>>,
    /// 主 Agent 配置
    pub orchestrator_config: Option<OrchestratorConfig>,
    /// 错误处理终端配置
    pub error_terminal_config: Option<TerminalConfig>,
    /// 合并终端配置
    pub merge_terminal_config: TerminalConfig,
    /// 目标分支
    pub target_branch: Option<String>,
    /// 任务列表
    pub tasks: Vec<CreateWorkflowTaskRequest>,
}

/// 主 Agent 配置
#[derive(Debug, Deserialize, TS)]
#[ts(export)]
pub struct OrchestratorConfig {
    pub api_type: String,
    pub base_url: String,
    pub api_key: String,
    pub model: String,
}

/// 终端配置
#[derive(Debug, Deserialize, TS)]
#[ts(export)]
pub struct TerminalConfig {
    pub cli_type_id: String,
    pub model_config_id: String,
    pub custom_base_url: Option<String>,
    pub custom_api_key: Option<String>,
}

/// 创建工作流任务请求
#[derive(Debug, Deserialize, TS)]
#[ts(export)]
pub struct CreateWorkflowTaskRequest {
    /// 任务名称
    pub name: String,
    /// 任务描述
    pub description: Option<String>,
    /// Git 分支名（可选，自动生成）
    pub branch: Option<String>,
    /// 终端列表
    pub terminals: Vec<CreateTerminalRequest>,
}

/// 创建终端请求
#[derive(Debug, Deserialize, TS)]
#[ts(export)]
pub struct CreateTerminalRequest {
    pub cli_type_id: String,
    pub model_config_id: String,
    pub custom_base_url: Option<String>,
    pub custom_api_key: Option<String>,
    pub role: Option<String>,
    pub role_description: Option<String>,
}

/// 工作流详情响应
#[derive(Debug, Serialize, TS)]
#[ts(export)]
pub struct WorkflowDetailResponse {
    #[serde(flatten)]
    pub workflow: Workflow,
    pub tasks: Vec<WorkflowTaskDetailResponse>,
    pub commands: Vec<WorkflowCommandWithPreset>,
}

/// 工作流任务详情响应
#[derive(Debug, Serialize, TS)]
#[ts(export)]
pub struct WorkflowTaskDetailResponse {
    #[serde(flatten)]
    pub task: WorkflowTask,
    pub terminals: Vec<Terminal>,
}

/// 工作流命令（包含预设信息）
#[derive(Debug, Serialize, TS)]
#[ts(export)]
pub struct WorkflowCommandWithPreset {
    #[serde(flatten)]
    pub command: WorkflowCommand,
    pub preset: SlashCommandPreset,
}

/// 更新工作流状态请求
#[derive(Debug, Deserialize, TS)]
#[ts(export)]
pub struct UpdateWorkflowStatusRequest {
    pub status: String,
}
```

由于文件较长，我将在下一步继续写入路由处理函数。

---

**Step 1.4.3: 创建 workflows.rs 路由（第二部分：路由处理函数）**

继续在 `workflows.rs` 文件中添加：

```rust
// ============================================================================
// 路由定义
// ============================================================================

/// 创建工作流路由
pub fn workflows_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/", get(list_workflows).post(create_workflow))
        .route("/:workflow_id", get(get_workflow).delete(delete_workflow))
        .route("/:workflow_id/status", put(update_workflow_status))
        .route("/:workflow_id/start", post(start_workflow))
        .route("/:workflow_id/tasks", get(list_workflow_tasks))
        .route("/:workflow_id/tasks/:task_id/terminals", get(list_task_terminals))
        .route("/presets/commands", get(list_command_presets))
}

// ============================================================================
// 路由处理函数
// ============================================================================

/// GET /api/workflows?project_id=xxx
/// 获取项目的所有工作流
async fn list_workflows(
    State(state): State<Arc<AppState>>,
    axum::extract::Query(params): axum::extract::Query<std::collections::HashMap<String, String>>,
) -> Result<Json<Vec<Workflow>>, ApiError> {
    let project_id = params.get("project_id")
        .ok_or_else(|| ApiError::BadRequest("project_id is required".to_string()))?;

    let workflows = workflow_dao::get_workflows_by_project(&state.db.pool, project_id).await?;
    Ok(Json(workflows))
}

/// POST /api/workflows
/// 创建工作流
async fn create_workflow(
    State(state): State<Arc<AppState>>,
    Json(req): Json<CreateWorkflowRequest>,
) -> Result<Json<WorkflowDetailResponse>, ApiError> {
    let now = chrono::Utc::now().to_rfc3339();
    let workflow_id = Uuid::new_v4().to_string();

    // 1. 创建工作流
    let workflow = Workflow {
        id: workflow_id.clone(),
        project_id: req.project_id,
        name: req.name,
        description: req.description,
        status: "created".to_string(),
        use_slash_commands: req.use_slash_commands,
        orchestrator_enabled: req.orchestrator_config.is_some(),
        orchestrator_api_type: req.orchestrator_config.as_ref().map(|c| c.api_type.clone()),
        orchestrator_base_url: req.orchestrator_config.as_ref().map(|c| c.base_url.clone()),
        orchestrator_api_key: req.orchestrator_config.as_ref().map(|c| c.api_key.clone()),
        orchestrator_model: req.orchestrator_config.as_ref().map(|c| c.model.clone()),
        error_terminal_enabled: req.error_terminal_config.is_some(),
        error_terminal_cli_id: req.error_terminal_config.as_ref().map(|c| c.cli_type_id.clone()),
        error_terminal_model_id: req.error_terminal_config.as_ref().map(|c| c.model_config_id.clone()),
        merge_terminal_cli_id: req.merge_terminal_config.cli_type_id.clone(),
        merge_terminal_model_id: req.merge_terminal_config.model_config_id.clone(),
        target_branch: req.target_branch.unwrap_or_else(|| "main".to_string()),
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: now.clone(),
        updated_at: now.clone(),
    };

    workflow_dao::create_workflow(&state.db.pool, &workflow).await?;

    // 2. 创建斜杠命令关联
    let mut commands = Vec::new();
    if let Some(preset_ids) = req.command_preset_ids {
        for (index, preset_id) in preset_ids.iter().enumerate() {
            workflow_dao::add_workflow_command(
                &state.db.pool,
                &workflow_id,
                preset_id,
                index as i32,
                None,
            ).await?;
        }
        commands = workflow_dao::get_commands_by_workflow(&state.db.pool, &workflow_id).await?;
    }

    // 3. 创建任务和终端
    let mut task_details = Vec::new();
    for (task_index, task_req) in req.tasks.iter().enumerate() {
        let task_id = Uuid::new_v4().to_string();

        // 生成分支名（如果未提供）
        let branch = task_req.branch.clone().unwrap_or_else(|| {
            format!("workflow/{}/{}",
                workflow_id.chars().take(8).collect::<String>(),
                slug::slugify(&task_req.name)
            )
        });

        let task = WorkflowTask {
            id: task_id.clone(),
            workflow_id: workflow_id.clone(),
            vk_task_id: None,
            name: task_req.name.clone(),
            description: task_req.description.clone(),
            branch,
            status: "pending".to_string(),
            order_index: task_index as i32,
            started_at: None,
            completed_at: None,
            created_at: now.clone(),
            updated_at: now.clone(),
        };

        workflow_dao::create_workflow_task(&state.db.pool, &task).await?;

        // 创建终端
        let mut terminals = Vec::new();
        for (terminal_index, terminal_req) in task_req.terminals.iter().enumerate() {
            let terminal_id = Uuid::new_v4().to_string();

            let terminal = Terminal {
                id: terminal_id.clone(),
                workflow_task_id: task_id.clone(),
                cli_type_id: terminal_req.cli_type_id.clone(),
                model_config_id: terminal_req.model_config_id.clone(),
                custom_base_url: terminal_req.custom_base_url.clone(),
                custom_api_key: terminal_req.custom_api_key.clone(),
                role: terminal_req.role.clone(),
                role_description: terminal_req.role_description.clone(),
                order_index: terminal_index as i32,
                status: "not_started".to_string(),
                process_id: None,
                pty_session_id: None,
                vk_session_id: None,
                last_commit_hash: None,
                last_commit_message: None,
                started_at: None,
                completed_at: None,
                created_at: now.clone(),
                updated_at: now.clone(),
            };

            terminal_dao::create_terminal(&state.db.pool, &terminal).await?;
            terminals.push(terminal);
        }

        task_details.push(WorkflowTaskDetailResponse {
            task,
            terminals,
        });
    }

    // 4. 获取命令预设详情
    let all_presets = workflow_dao::get_all_slash_command_presets(&state.db.pool).await?;
    let commands_with_presets: Vec<WorkflowCommandWithPreset> = commands
        .into_iter()
        .filter_map(|cmd| {
            all_presets.iter()
                .find(|p| p.id == cmd.preset_id)
                .map(|preset| WorkflowCommandWithPreset {
                    command: cmd,
                    preset: preset.clone(),
                })
        })
        .collect();

    Ok(Json(WorkflowDetailResponse {
        workflow,
        tasks: task_details,
        commands: commands_with_presets,
    }))
}

/// GET /api/workflows/:workflow_id
/// 获取工作流详情
async fn get_workflow(
    State(state): State<Arc<AppState>>,
    Path(workflow_id): Path<String>,
) -> Result<Json<WorkflowDetailResponse>, ApiError> {
    // 获取工作流
    let workflow = workflow_dao::get_workflow_by_id(&state.db.pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    // 获取任务和终端
    let tasks = workflow_dao::get_tasks_by_workflow(&state.db.pool, &workflow_id).await?;
    let mut task_details = Vec::new();
    for task in tasks {
        let terminals = terminal_dao::get_terminals_by_task(&state.db.pool, &task.id).await?;
        task_details.push(WorkflowTaskDetailResponse {
            task,
            terminals,
        });
    }

    // 获取命令
    let commands = workflow_dao::get_commands_by_workflow(&state.db.pool, &workflow_id).await?;
    let all_presets = workflow_dao::get_all_slash_command_presets(&state.db.pool).await?;
    let commands_with_presets: Vec<WorkflowCommandWithPreset> = commands
        .into_iter()
        .filter_map(|cmd| {
            all_presets.iter()
                .find(|p| p.id == cmd.preset_id)
                .map(|preset| WorkflowCommandWithPreset {
                    command: cmd,
                    preset: preset.clone(),
                })
        })
        .collect();

    Ok(Json(WorkflowDetailResponse {
        workflow,
        tasks: task_details,
        commands: commands_with_presets,
    }))
}

/// DELETE /api/workflows/:workflow_id
/// 删除工作流
async fn delete_workflow(
    State(state): State<Arc<AppState>>,
    Path(workflow_id): Path<String>,
) -> Result<Json<()>, ApiError> {
    workflow_dao::delete_workflow(&state.db.pool, &workflow_id).await?;
    Ok(Json(()))
}

/// PUT /api/workflows/:workflow_id/status
/// 更新工作流状态
async fn update_workflow_status(
    State(state): State<Arc<AppState>>,
    Path(workflow_id): Path<String>,
    Json(req): Json<UpdateWorkflowStatusRequest>,
) -> Result<Json<()>, ApiError> {
    workflow_dao::update_workflow_status(&state.db.pool, &workflow_id, &req.status).await?;
    Ok(Json(()))
}

/// POST /api/workflows/:workflow_id/start
/// 启动工作流（用户确认开始）
async fn start_workflow(
    State(state): State<Arc<AppState>>,
    Path(workflow_id): Path<String>,
) -> Result<Json<()>, ApiError> {
    // 检查工作流状态是否为 ready
    let workflow = workflow_dao::get_workflow_by_id(&state.db.pool, &workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;

    if workflow.status != "ready" {
        return Err(ApiError::BadRequest(
            format!("Workflow is not ready. Current status: {}", workflow.status)
        ));
    }

    // 更新状态为 running
    workflow_dao::set_workflow_started(&state.db.pool, &workflow_id).await?;

    // TODO: 触发 Orchestrator 开始协调

    Ok(Json(()))
}

/// GET /api/workflows/:workflow_id/tasks
/// 获取工作流的所有任务
async fn list_workflow_tasks(
    State(state): State<Arc<AppState>>,
    Path(workflow_id): Path<String>,
) -> Result<Json<Vec<WorkflowTaskDetailResponse>>, ApiError> {
    let tasks = workflow_dao::get_tasks_by_workflow(&state.db.pool, &workflow_id).await?;
    let mut task_details = Vec::new();
    for task in tasks {
        let terminals = terminal_dao::get_terminals_by_task(&state.db.pool, &task.id).await?;
        task_details.push(WorkflowTaskDetailResponse {
            task,
            terminals,
        });
    }
    Ok(Json(task_details))
}

/// GET /api/workflows/:workflow_id/tasks/:task_id/terminals
/// 获取任务的所有终端
async fn list_task_terminals(
    State(state): State<Arc<AppState>>,
    Path((_, task_id)): Path<(String, String)>,
) -> Result<Json<Vec<Terminal>>, ApiError> {
    let terminals = terminal_dao::get_terminals_by_task(&state.db.pool, &task_id).await?;
    Ok(Json(terminals))
}

/// GET /api/workflows/presets/commands
/// 获取所有斜杠命令预设
async fn list_command_presets(
    State(state): State<Arc<AppState>>,
) -> Result<Json<Vec<SlashCommandPreset>>, ApiError> {
    let presets = workflow_dao::get_all_slash_command_presets(&state.db.pool).await?;
    Ok(Json(presets))
}
```

---

**Step 1.4.4: 修改 routes/mod.rs 注册新路由**

文件路径: `vibe-kanban-main/crates/server/src/routes/mod.rs`

在文件中添加：

```rust
// 在文件顶部添加模块声明
pub mod cli_types;
pub mod workflows;

// 在 api_routes() 函数中添加路由注册
pub fn api_routes(state: Arc<AppState>) -> Router {
    Router::new()
        // ... 现有路由 ...
        .nest("/cli_types", cli_types::cli_types_routes())
        .nest("/workflows", workflows::workflows_routes())
        .with_state(state)
}
```

---

**交付物:**
- `vibe-kanban-main/crates/server/src/routes/cli_types.rs`
- `vibe-kanban-main/crates/server/src/routes/workflows.rs`
- 修改后的 `routes/mod.rs`

**验收标准:**
1. 编译通过：`cargo build -p server`
2. API 端点可访问
3. TypeScript 类型生成成功

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p server
# 预期: 编译成功

# 启动服务器测试
pnpm run dev
# 在另一个终端测试 API
curl http://localhost:3001/api/cli_types
# 预期: 返回 CLI 类型列表 JSON

curl http://localhost:3001/api/cli_types/detect
# 预期: 返回 CLI 检测状态列表
```

**API 端点清单:**

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | /api/cli_types | 获取所有 CLI 类型 |
| GET | /api/cli_types/detect | 检测已安装的 CLI |
| GET | /api/cli_types/:id/models | 获取 CLI 的模型列表 |
| GET | /api/workflows?project_id=xxx | 获取项目的工作流列表 |
| POST | /api/workflows | 创建工作流 |
| GET | /api/workflows/:id | 获取工作流详情 |
| DELETE | /api/workflows/:id | 删除工作流 |
| PUT | /api/workflows/:id/status | 更新工作流状态 |
| POST | /api/workflows/:id/start | 启动工作流 |
| GET | /api/workflows/:id/tasks | 获取工作流任务列表 |
| GET | /api/workflows/:id/tasks/:task_id/terminals | 获取任务终端列表 |
| GET | /api/workflows/presets/commands | 获取斜杠命令预设 |

---

## Phase 1 完成检查清单

在进入 Phase 2 之前，确保以下所有项目已完成：

- [ ] Task 1.1: 迁移文件已创建并成功执行
- [ ] Task 1.2: Rust 模型文件已创建，编译通过
- [ ] Task 1.3: DAO 层已创建，编译通过
- [ ] Task 1.4: API 路由已创建，可以访问
- [ ] TypeScript 类型已生成：`cargo run --bin generate_types`
- [ ] 开发服务器可以启动：`pnpm run dev`

---

## Phase 2: CC-Switch 核心提取与集成

### Task 2.1: 分析 CC-Switch 核心代码

**状态:** ⬜ 未开始

**前置条件:**
- Phase 1 已完成
- 已阅读 CC-Switch 源码

**目标:**
分析 CC-Switch 中需要提取的核心代码，确定依赖关系和修改点。

**涉及文件（只读分析）:**
- `cc-switch-main/src-tauri/src/provider.rs` - Provider 数据模型
- `cc-switch-main/src-tauri/src/config.rs` - Claude 配置读写
- `cc-switch-main/src-tauri/src/codex_config.rs` - Codex 配置读写
- `cc-switch-main/src-tauri/src/gemini_config.rs` - Gemini 配置读写
- `cc-switch-main/src-tauri/src/services/provider/mod.rs` - 供应商服务
- `cc-switch-main/src-tauri/src/services/provider/live.rs` - Live 配置写入

---

**Step 2.1.1: 核心代码依赖分析**

需要提取的核心功能：

| 功能 | 源文件 | Tauri 依赖 | 可独立使用 |
|------|--------|------------|------------|
| 配置文件路径 | config.rs | 无 | ✅ |
| Claude 配置读写 | config.rs | 无 | ✅ |
| Codex 配置读写 | codex_config.rs | 无 | ✅ |
| Gemini 配置读写 | gemini_config.rs | 无 | ✅ |
| 原子写入 | config.rs | 无 | ✅ |
| Provider 模型 | provider.rs | 无 | ✅ |
| Live 配置写入 | services/provider/live.rs | 部分 | ⚠️ 需修改 |
| 供应商切换 | services/provider/mod.rs | 是 | ❌ 需重写 |

**需要移除的 Tauri 依赖:**
- `tauri::State<'_, AppState>` → 改为函数参数
- `#[tauri::command]` → 移除
- `tauri-plugin-store` → 使用文件存储替代

---

**交付物:**
- 本任务为分析任务，无代码交付
- 输出：依赖分析表（如上）

**验收标准:**
1. 明确了需要提取的文件列表
2. 明确了需要修改的依赖点
3. 明确了提取后的模块结构

---

### Task 2.2: 创建 cc-switch crate

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.1 分析完成

**目标:**
在 vibe-kanban workspace 中创建独立的 cc-switch crate，包含配置切换核心逻辑。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/cc-switch/Cargo.toml`
- 创建: `vibe-kanban-main/crates/cc-switch/src/lib.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/error.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/config_path.rs`
- 修改: `vibe-kanban-main/Cargo.toml` (添加 workspace member)

---

**Step 2.2.1: 创建 Cargo.toml**

文件路径: `vibe-kanban-main/crates/cc-switch/Cargo.toml`

```toml
[package]
name = "cc-switch"
version = "0.1.0"
edition = "2021"
description = "CLI configuration switching core for SoloDawn"
authors = ["SoloDawn Contributors"]
license = "Apache-2.0"

[dependencies]
# 序列化
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"

# 异步运行时
tokio = { version = "1.0", features = ["fs", "process"] }

# 错误处理
anyhow = "1.0"
thiserror = "2.0"

# 文件系统
dirs = "5.0"
tempfile = "3"

# TOML 解析 (Codex 配置)
toml = "0.8"

# 日志
tracing = "0.1"
```

---

**Step 2.2.2: 创建 error.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/error.rs`

```rust
//! CC-Switch 错误类型

use thiserror::Error;

/// CC-Switch 错误
#[derive(Error, Debug)]
pub enum CCSwitchError {
    #[error("Configuration file not found: {path}")]
    ConfigNotFound { path: String },

    #[error("Failed to read configuration: {0}")]
    ReadError(#[from] std::io::Error),

    #[error("Failed to parse JSON: {0}")]
    JsonParseError(#[from] serde_json::Error),

    #[error("Failed to parse TOML: {0}")]
    TomlParseError(#[from] toml::de::Error),

    #[error("Invalid configuration: {message}")]
    InvalidConfig { message: String },

    #[error("CLI not supported: {cli_name}")]
    UnsupportedCli { cli_name: String },

    #[error("Atomic write failed: {0}")]
    AtomicWriteError(String),

    #[error("Home directory not found")]
    HomeDirNotFound,
}

pub type Result<T> = std::result::Result<T, CCSwitchError>;
```

---

**Step 2.2.3: 创建 config_path.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/config_path.rs`

```rust
//! 配置文件路径管理
//!
//! 提供各 CLI 配置文件的路径获取功能。

use std::path::PathBuf;
use crate::error::{CCSwitchError, Result};

/// 获取用户主目录
pub fn get_home_dir() -> Result<PathBuf> {
    dirs::home_dir().ok_or(CCSwitchError::HomeDirNotFound)
}

// ============================================================================
// Claude Code 配置路径
// ============================================================================

/// 获取 Claude 配置目录
///
/// 默认: ~/.claude
pub fn get_claude_config_dir() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".claude"))
}

/// 获取 Claude settings.json 路径
///
/// 路径: ~/.claude/settings.json
pub fn get_claude_settings_path() -> Result<PathBuf> {
    Ok(get_claude_config_dir()?.join("settings.json"))
}

/// 获取 Claude MCP 配置路径
///
/// 路径: ~/.claude.json (注意：不是 ~/.claude/claude.json)
pub fn get_claude_mcp_path() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".claude.json"))
}

// ============================================================================
// Codex 配置路径
// ============================================================================

/// 获取 Codex 配置目录
///
/// 默认: ~/.codex
pub fn get_codex_config_dir() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".codex"))
}

/// 获取 Codex auth.json 路径
///
/// 路径: ~/.codex/auth.json
pub fn get_codex_auth_path() -> Result<PathBuf> {
    Ok(get_codex_config_dir()?.join("auth.json"))
}

/// 获取 Codex config.toml 路径
///
/// 路径: ~/.codex/config.toml
pub fn get_codex_config_path() -> Result<PathBuf> {
    Ok(get_codex_config_dir()?.join("config.toml"))
}

// ============================================================================
// Gemini CLI 配置路径
// ============================================================================

/// 获取 Gemini 配置目录
///
/// 默认: ~/.gemini
pub fn get_gemini_config_dir() -> Result<PathBuf> {
    Ok(get_home_dir()?.join(".gemini"))
}

/// 获取 Gemini .env 路径
///
/// 路径: ~/.gemini/.env
pub fn get_gemini_env_path() -> Result<PathBuf> {
    Ok(get_gemini_config_dir()?.join(".env"))
}

/// 获取 Gemini settings.json 路径
///
/// 路径: ~/.gemini/settings.json
pub fn get_gemini_settings_path() -> Result<PathBuf> {
    Ok(get_gemini_config_dir()?.join("settings.json"))
}

// ============================================================================
// 通用工具
// ============================================================================

/// 确保目录存在
pub async fn ensure_dir_exists(path: &PathBuf) -> Result<()> {
    if !path.exists() {
        tokio::fs::create_dir_all(path).await?;
    }
    Ok(())
}

/// 确保父目录存在
pub async fn ensure_parent_dir_exists(path: &PathBuf) -> Result<()> {
    if let Some(parent) = path.parent() {
        ensure_dir_exists(&parent.to_path_buf()).await?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_claude_paths() {
        let settings = get_claude_settings_path().unwrap();
        assert!(settings.to_string_lossy().contains(".claude"));
        assert!(settings.to_string_lossy().ends_with("settings.json"));
    }

    #[test]
    fn test_codex_paths() {
        let auth = get_codex_auth_path().unwrap();
        assert!(auth.to_string_lossy().contains(".codex"));
        assert!(auth.to_string_lossy().ends_with("auth.json"));
    }

    #[test]
    fn test_gemini_paths() {
        let env = get_gemini_env_path().unwrap();
        assert!(env.to_string_lossy().contains(".gemini"));
        assert!(env.to_string_lossy().ends_with(".env"));
    }
}
```

---

**Step 2.2.4: 创建 lib.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/lib.rs`

```rust
//! CC-Switch Core
//!
//! CLI 配置切换核心库，支持 Claude Code、Codex、Gemini CLI 等。
//!
//! # 功能
//!
//! - 配置文件路径管理
//! - 配置读写（支持 JSON、TOML、.env 格式）
//! - 原子写入（防止配置损坏）
//! - 模型切换
//!
//! # 示例
//!
//! ```rust,ignore
//! use cc_switch::{CliType, switch_model};
//!
//! // 切换 Claude Code 模型
//! switch_model(CliType::ClaudeCode, "sonnet", &config).await?;
//! ```

pub mod error;
pub mod config_path;

// 后续模块（Task 2.3-2.5 中添加）
// pub mod atomic_write;
// pub mod claude;
// pub mod codex;
// pub mod gemini;
// pub mod switcher;

pub use error::{CCSwitchError, Result};
pub use config_path::*;

/// 支持的 CLI 类型
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CliType {
    /// Claude Code
    ClaudeCode,
    /// OpenAI Codex
    Codex,
    /// Google Gemini CLI
    Gemini,
    /// Amp
    Amp,
    /// Cursor Agent
    CursorAgent,
    /// Qwen Code
    QwenCode,
    /// GitHub Copilot
    Copilot,
    /// Droid
    Droid,
    /// Opencode
    Opencode,
}

impl CliType {
    /// 从字符串解析 CLI 类型
    pub fn from_str(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "claude-code" | "claude" => Some(Self::ClaudeCode),
            "codex" => Some(Self::Codex),
            "gemini-cli" | "gemini" => Some(Self::Gemini),
            "amp" => Some(Self::Amp),
            "cursor-agent" | "cursor" => Some(Self::CursorAgent),
            "qwen-code" | "qwen" => Some(Self::QwenCode),
            "copilot" => Some(Self::Copilot),
            "droid" => Some(Self::Droid),
            "opencode" => Some(Self::Opencode),
            _ => None,
        }
    }

    /// 转换为字符串
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::ClaudeCode => "claude-code",
            Self::Codex => "codex",
            Self::Gemini => "gemini-cli",
            Self::Amp => "amp",
            Self::CursorAgent => "cursor-agent",
            Self::QwenCode => "qwen-code",
            Self::Copilot => "copilot",
            Self::Droid => "droid",
            Self::Opencode => "opencode",
        }
    }

    /// 获取显示名称
    pub fn display_name(&self) -> &'static str {
        match self {
            Self::ClaudeCode => "Claude Code",
            Self::Codex => "Codex",
            Self::Gemini => "Gemini CLI",
            Self::Amp => "Amp",
            Self::CursorAgent => "Cursor Agent",
            Self::QwenCode => "Qwen Code",
            Self::Copilot => "GitHub Copilot",
            Self::Droid => "Droid",
            Self::Opencode => "Opencode",
        }
    }

    /// 是否支持配置切换
    pub fn supports_config_switch(&self) -> bool {
        matches!(self, Self::ClaudeCode | Self::Codex | Self::Gemini)
    }
}
```

---

**Step 2.2.5: 添加到 workspace**

修改 `vibe-kanban-main/Cargo.toml`，在 `members` 数组中添加：

```toml
[workspace]
resolver = "2"
members = [
    "crates/server",
    "crates/db",
    "crates/executors",
    "crates/services",
    "crates/utils",
    "crates/local-deployment",
    "crates/deployment",
    "crates/remote",
    "crates/review",
    "crates/cc-switch",  # 新增
]
```

---

**交付物:**
- `vibe-kanban-main/crates/cc-switch/Cargo.toml`
- `vibe-kanban-main/crates/cc-switch/src/lib.rs`
- `vibe-kanban-main/crates/cc-switch/src/error.rs`
- `vibe-kanban-main/crates/cc-switch/src/config_path.rs`
- 修改后的 workspace `Cargo.toml`

**验收标准:**
1. crate 编译通过：`cargo build -p cc-switch`
2. 单元测试通过：`cargo test -p cc-switch`

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p cc-switch
# 预期: 编译成功

cargo test -p cc-switch
# 预期: 测试通过
```

---

### Task 2.3: 实现原子写入和配置读写

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.2 已完成

**目标:**
实现配置文件的原子写入功能，防止写入过程中断导致配置损坏。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/cc-switch/src/atomic_write.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/claude.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/codex.rs`
- 创建: `vibe-kanban-main/crates/cc-switch/src/gemini.rs`
- 修改: `vibe-kanban-main/crates/cc-switch/src/lib.rs`

---

**Step 2.3.1: 创建 atomic_write.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/atomic_write.rs`

```rust
//! 原子写入工具
//!
//! 使用临时文件 + 重命名的方式实现原子写入，
//! 防止写入过程中断导致配置文件损坏。

use std::path::Path;
use crate::error::{CCSwitchError, Result};
use crate::config_path::ensure_parent_dir_exists;

/// 原子写入文件
///
/// 流程：
/// 1. 写入临时文件
/// 2. 同步到磁盘
/// 3. 重命名为目标文件（原子操作）
///
/// # 参数
/// - `path`: 目标文件路径
/// - `data`: 要写入的数据
///
/// # 示例
/// ```rust,ignore
/// atomic_write(&path, b"content").await?;
/// ```
pub async fn atomic_write(path: &Path, data: &[u8]) -> Result<()> {
    use tokio::io::AsyncWriteExt;

    // 确保父目录存在
    ensure_parent_dir_exists(&path.to_path_buf()).await?;

    // 创建临时文件（在同一目录下，确保重命名是原子的）
    let parent = path.parent().unwrap_or(Path::new("."));
    let temp_path = parent.join(format!(
        ".{}.tmp.{}",
        path.file_name()
            .map(|n| n.to_string_lossy().to_string())
            .unwrap_or_else(|| "config".to_string()),
        std::process::id()
    ));

    // 写入临时文件
    let mut file = tokio::fs::File::create(&temp_path).await?;
    file.write_all(data).await?;
    file.sync_all().await?; // 确保数据写入磁盘
    drop(file);

    // 原子重命名
    tokio::fs::rename(&temp_path, path).await.map_err(|e| {
        // 清理临时文件
        let _ = std::fs::remove_file(&temp_path);
        CCSwitchError::AtomicWriteError(format!(
            "Failed to rename {} to {}: {}",
            temp_path.display(),
            path.display(),
            e
        ))
    })?;

    Ok(())
}

/// 原子写入 JSON 文件
pub async fn atomic_write_json<T: serde::Serialize>(path: &Path, value: &T) -> Result<()> {
    let json = serde_json::to_string_pretty(value)?;
    atomic_write(path, json.as_bytes()).await
}

/// 原子写入文本文件
pub async fn atomic_write_text(path: &Path, text: &str) -> Result<()> {
    atomic_write(path, text.as_bytes()).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_atomic_write() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");

        atomic_write(&path, b"hello world").await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        assert_eq!(content, "hello world");
    }

    #[tokio::test]
    async fn test_atomic_write_json() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.json");

        let data = serde_json::json!({"key": "value"});
        atomic_write_json(&path, &data).await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        let parsed: serde_json::Value = serde_json::from_str(&content).unwrap();
        assert_eq!(parsed["key"], "value");
    }
}
```

---

**Step 2.3.2: 创建 claude.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/claude.rs`

```rust
//! Claude Code 配置管理
//!
//! Claude Code 使用 JSON 格式的配置文件：
//! - ~/.claude/settings.json - 主配置文件

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::path::Path;
use crate::error::{CCSwitchError, Result};
use crate::config_path::{get_claude_settings_path, ensure_parent_dir_exists};
use crate::atomic_write::atomic_write_json;

/// Claude Code 配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ClaudeConfig {
    /// 环境变量配置
    #[serde(default)]
    pub env: ClaudeEnvConfig,

    /// 其他配置（保留原有字段）
    #[serde(flatten)]
    pub other: Value,
}

/// Claude Code 环境变量配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ClaudeEnvConfig {
    /// API Base URL
    #[serde(rename = "ANTHROPIC_BASE_URL", skip_serializing_if = "Option::is_none")]
    pub base_url: Option<String>,

    /// API Token
    #[serde(rename = "ANTHROPIC_AUTH_TOKEN", skip_serializing_if = "Option::is_none")]
    pub auth_token: Option<String>,

    /// API Key (备选)
    #[serde(rename = "ANTHROPIC_API_KEY", skip_serializing_if = "Option::is_none")]
    pub api_key: Option<String>,

    /// 默认模型
    #[serde(rename = "ANTHROPIC_MODEL", skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,

    /// Haiku 模型
    #[serde(rename = "ANTHROPIC_DEFAULT_HAIKU_MODEL", skip_serializing_if = "Option::is_none")]
    pub haiku_model: Option<String>,

    /// Sonnet 模型
    #[serde(rename = "ANTHROPIC_DEFAULT_SONNET_MODEL", skip_serializing_if = "Option::is_none")]
    pub sonnet_model: Option<String>,

    /// Opus 模型
    #[serde(rename = "ANTHROPIC_DEFAULT_OPUS_MODEL", skip_serializing_if = "Option::is_none")]
    pub opus_model: Option<String>,

    /// 其他环境变量
    #[serde(flatten)]
    pub other: std::collections::HashMap<String, Value>,
}

/// 读取 Claude 配置
pub async fn read_claude_config() -> Result<ClaudeConfig> {
    let path = get_claude_settings_path()?;
    read_claude_config_from(&path).await
}

/// 从指定路径读取 Claude 配置
pub async fn read_claude_config_from(path: &Path) -> Result<ClaudeConfig> {
    if !path.exists() {
        return Ok(ClaudeConfig::default());
    }

    let content = tokio::fs::read_to_string(path).await?;
    let config: ClaudeConfig = serde_json::from_str(&content)?;
    Ok(config)
}

/// 写入 Claude 配置
pub async fn write_claude_config(config: &ClaudeConfig) -> Result<()> {
    let path = get_claude_settings_path()?;
    write_claude_config_to(&path, config).await
}

/// 写入 Claude 配置到指定路径
pub async fn write_claude_config_to(path: &Path, config: &ClaudeConfig) -> Result<()> {
    ensure_parent_dir_exists(&path.to_path_buf()).await?;
    atomic_write_json(path, config).await
}

/// 更新 Claude 模型配置
///
/// # 参数
/// - `base_url`: API Base URL（可选，None 表示使用官方 API）
/// - `api_key`: API Key
/// - `model`: 模型名称
pub async fn update_claude_model(
    base_url: Option<&str>,
    api_key: &str,
    model: &str,
) -> Result<()> {
    let mut config = read_claude_config().await?;

    config.env.base_url = base_url.map(|s| s.to_string());
    config.env.auth_token = Some(api_key.to_string());
    config.env.model = Some(model.to_string());

    write_claude_config(&config).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_read_write_claude_config() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("settings.json");

        let config = ClaudeConfig {
            env: ClaudeEnvConfig {
                base_url: Some("https://api.example.com".to_string()),
                auth_token: Some("sk-test".to_string()),
                model: Some("claude-sonnet".to_string()),
                ..Default::default()
            },
            other: serde_json::json!({}),
        };

        write_claude_config_to(&path, &config).await.unwrap();

        let loaded = read_claude_config_from(&path).await.unwrap();
        assert_eq!(loaded.env.base_url, config.env.base_url);
        assert_eq!(loaded.env.auth_token, config.env.auth_token);
    }
}
```

---

**Step 2.3.3: 创建 codex.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/codex.rs`

```rust
//! Codex 配置管理
//!
//! Codex 使用两个配置文件：
//! - ~/.codex/auth.json - API 认证信息
//! - ~/.codex/config.toml - 模型和提供商配置

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::path::Path;
use crate::error::{CCSwitchError, Result};
use crate::config_path::{get_codex_auth_path, get_codex_config_path, ensure_parent_dir_exists};
use crate::atomic_write::{atomic_write_json, atomic_write_text};

/// Codex 认证配置 (auth.json)
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CodexAuthConfig {
    /// OpenAI API Key
    #[serde(rename = "OPENAI_API_KEY", skip_serializing_if = "Option::is_none")]
    pub openai_api_key: Option<String>,

    /// 其他字段
    #[serde(flatten)]
    pub other: std::collections::HashMap<String, Value>,
}

/// Codex 模型配置 (config.toml)
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CodexModelConfig {
    /// 模型提供商
    pub model_provider: Option<String>,

    /// 模型名称
    pub model: Option<String>,

    /// 提供商配置
    #[serde(default)]
    pub model_providers: std::collections::HashMap<String, CodexProviderConfig>,
}

/// Codex 提供商配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CodexProviderConfig {
    /// Base URL
    pub base_url: Option<String>,
}

/// 读取 Codex 认证配置
pub async fn read_codex_auth() -> Result<CodexAuthConfig> {
    let path = get_codex_auth_path()?;
    if !path.exists() {
        return Ok(CodexAuthConfig::default());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    let config: CodexAuthConfig = serde_json::from_str(&content)?;
    Ok(config)
}

/// 写入 Codex 认证配置
pub async fn write_codex_auth(config: &CodexAuthConfig) -> Result<()> {
    let path = get_codex_auth_path()?;
    ensure_parent_dir_exists(&path).await?;
    atomic_write_json(&path, config).await
}

/// 读取 Codex 模型配置
pub async fn read_codex_config() -> Result<CodexModelConfig> {
    let path = get_codex_config_path()?;
    if !path.exists() {
        return Ok(CodexModelConfig::default());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    let config: CodexModelConfig = toml::from_str(&content)?;
    Ok(config)
}

/// 写入 Codex 模型配置
pub async fn write_codex_config(config: &CodexModelConfig) -> Result<()> {
    let path = get_codex_config_path()?;
    ensure_parent_dir_exists(&path).await?;
    let toml_str = toml::to_string_pretty(config)
        .map_err(|e| CCSwitchError::InvalidConfig { message: e.to_string() })?;
    atomic_write_text(&path, &toml_str).await
}

/// 更新 Codex 模型配置
pub async fn update_codex_model(
    base_url: Option<&str>,
    api_key: &str,
    model: &str,
) -> Result<()> {
    // 更新 auth.json
    let mut auth = read_codex_auth().await?;
    auth.openai_api_key = Some(api_key.to_string());
    write_codex_auth(&auth).await?;

    // 更新 config.toml
    let mut config = read_codex_config().await?;
    config.model = Some(model.to_string());

    if let Some(url) = base_url {
        config.model_provider = Some("custom".to_string());
        config.model_providers.insert(
            "custom".to_string(),
            CodexProviderConfig {
                base_url: Some(url.to_string()),
            },
        );
    } else {
        config.model_provider = Some("openai".to_string());
    }

    write_codex_config(&config).await
}
```

---

**Step 2.3.4: 创建 gemini.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/gemini.rs`

```rust
//! Gemini CLI 配置管理
//!
//! Gemini CLI 使用 .env 格式的配置文件：
//! - ~/.gemini/.env - 环境变量配置

use std::collections::HashMap;
use std::path::Path;
use crate::error::Result;
use crate::config_path::{get_gemini_env_path, ensure_parent_dir_exists};
use crate::atomic_write::atomic_write_text;

/// 解析 .env 文件内容
pub fn parse_env_file(content: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();
    for line in content.lines() {
        let line = line.trim();
        // 跳过空行和注释
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        // 解析 KEY=VALUE
        if let Some(pos) = line.find('=') {
            let key = line[..pos].trim().to_string();
            let value = line[pos + 1..].trim();
            // 移除引号
            let value = value.trim_matches('"').trim_matches('\'').to_string();
            map.insert(key, value);
        }
    }
    map
}

/// 序列化为 .env 格式
pub fn serialize_env_file(map: &HashMap<String, String>) -> String {
    let mut lines: Vec<String> = map
        .iter()
        .map(|(k, v)| {
            // 如果值包含空格或特殊字符，使用引号
            if v.contains(' ') || v.contains('=') || v.contains('#') {
                format!("{}=\"{}\"", k, v)
            } else {
                format!("{}={}", k, v)
            }
        })
        .collect();
    lines.sort(); // 保持顺序一致
    lines.join("\n") + "\n"
}

/// 读取 Gemini 配置
pub async fn read_gemini_config() -> Result<HashMap<String, String>> {
    let path = get_gemini_env_path()?;
    if !path.exists() {
        return Ok(HashMap::new());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    Ok(parse_env_file(&content))
}

/// 写入 Gemini 配置
pub async fn write_gemini_config(config: &HashMap<String, String>) -> Result<()> {
    let path = get_gemini_env_path()?;
    ensure_parent_dir_exists(&path).await?;
    let content = serialize_env_file(config);
    atomic_write_text(&path, &content).await
}

/// 更新 Gemini 模型配置
pub async fn update_gemini_model(
    base_url: Option<&str>,
    api_key: &str,
    model: &str,
) -> Result<()> {
    let mut config = read_gemini_config().await?;

    // 设置 API Key
    config.insert("GEMINI_API_KEY".to_string(), api_key.to_string());

    // 设置 Base URL（如果提供）
    if let Some(url) = base_url {
        config.insert("GOOGLE_GEMINI_BASE_URL".to_string(), url.to_string());
    } else {
        config.remove("GOOGLE_GEMINI_BASE_URL");
    }

    // 设置模型
    config.insert("GEMINI_MODEL".to_string(), model.to_string());

    write_gemini_config(&config).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_env_file() {
        let content = r#"
# Comment
GEMINI_API_KEY=test-key
GEMINI_MODEL="gemini-pro"
EMPTY=
"#;
        let map = parse_env_file(content);
        assert_eq!(map.get("GEMINI_API_KEY"), Some(&"test-key".to_string()));
        assert_eq!(map.get("GEMINI_MODEL"), Some(&"gemini-pro".to_string()));
        assert_eq!(map.get("EMPTY"), Some(&"".to_string()));
    }

    #[test]
    fn test_serialize_env_file() {
        let mut map = HashMap::new();
        map.insert("KEY1".to_string(), "value1".to_string());
        map.insert("KEY2".to_string(), "value with space".to_string());

        let content = serialize_env_file(&map);
        assert!(content.contains("KEY1=value1"));
        assert!(content.contains("KEY2=\"value with space\""));
    }
}
```

---

**Step 2.3.5: 更新 lib.rs 导出新模块**

修改 `vibe-kanban-main/crates/cc-switch/src/lib.rs`，取消注释并添加导出：

```rust
pub mod error;
pub mod config_path;
pub mod atomic_write;
pub mod claude;
pub mod codex;
pub mod gemini;

pub use error::{CCSwitchError, Result};
pub use config_path::*;
pub use atomic_write::*;
pub use claude::*;
pub use codex::*;
pub use gemini::*;
```

---

**交付物:**
- `vibe-kanban-main/crates/cc-switch/src/atomic_write.rs`
- `vibe-kanban-main/crates/cc-switch/src/claude.rs`
- `vibe-kanban-main/crates/cc-switch/src/codex.rs`
- `vibe-kanban-main/crates/cc-switch/src/gemini.rs`
- 更新后的 `lib.rs`

**验收标准:**
1. 编译通过：`cargo build -p cc-switch`
2. 所有测试通过：`cargo test -p cc-switch`
3. 可以读写各 CLI 的配置文件

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p cc-switch
# 预期: 所有测试通过
```

---

### Task 2.4: 实现模型切换服务

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.3 已完成

**目标:**
实现统一的模型切换接口，支持切换 Claude Code、Codex、Gemini CLI 的模型配置。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/cc-switch/src/switcher.rs`
- 修改: `vibe-kanban-main/crates/cc-switch/src/lib.rs`

---

**Step 2.4.1: 创建 switcher.rs**

文件路径: `vibe-kanban-main/crates/cc-switch/src/switcher.rs`

```rust
//! 模型切换服务
//!
//! 提供统一的模型切换接口。

use crate::error::{CCSwitchError, Result};
use crate::{CliType, claude, codex, gemini};

/// 模型切换配置
#[derive(Debug, Clone)]
pub struct SwitchConfig {
    /// API Base URL（可选，None 表示使用官方 API）
    pub base_url: Option<String>,
    /// API Key
    pub api_key: String,
    /// 模型名称
    pub model: String,
}

/// 切换模型
///
/// # 参数
/// - `cli_type`: CLI 类型
/// - `config`: 切换配置
///
/// # 示例
/// ```rust,ignore
/// use cc_switch::{CliType, SwitchConfig, switch_model};
///
/// let config = SwitchConfig {
///     base_url: Some("https://api.example.com".to_string()),
///     api_key: "sk-xxx".to_string(),
///     model: "claude-sonnet-4".to_string(),
/// };
///
/// switch_model(CliType::ClaudeCode, &config).await?;
/// ```
pub async fn switch_model(cli_type: CliType, config: &SwitchConfig) -> Result<()> {
    tracing::info!(
        "Switching model for {}: model={}, base_url={:?}",
        cli_type.display_name(),
        config.model,
        config.base_url
    );

    match cli_type {
        CliType::ClaudeCode => {
            claude::update_claude_model(
                config.base_url.as_deref(),
                &config.api_key,
                &config.model,
            ).await
        }
        CliType::Codex => {
            codex::update_codex_model(
                config.base_url.as_deref(),
                &config.api_key,
                &config.model,
            ).await
        }
        CliType::Gemini => {
            gemini::update_gemini_model(
                config.base_url.as_deref(),
                &config.api_key,
                &config.model,
            ).await
        }
        _ => {
            Err(CCSwitchError::UnsupportedCli {
                cli_name: cli_type.as_str().to_string(),
            })
        }
    }
}

/// 批量切换模型（用于工作流启动时）
///
/// 按顺序切换多个终端的模型配置。
/// 注意：由于 cc-switch 修改全局环境变量，必须串行执行。
pub async fn switch_models_sequential(
    configs: Vec<(CliType, SwitchConfig)>,
) -> Result<Vec<Result<()>>> {
    let mut results = Vec::new();

    for (cli_type, config) in configs {
        let result = switch_model(cli_type, &config).await;
        results.push(result);
    }

    Ok(results)
}

/// 模型切换服务
///
/// 提供更高级的模型切换功能，包括：
/// - 配置备份和恢复
/// - 切换前验证
/// - 切换后验证
pub struct ModelSwitcher {
    /// 是否在切换前备份配置
    backup_before_switch: bool,
}

impl ModelSwitcher {
    pub fn new() -> Self {
        Self {
            backup_before_switch: true,
        }
    }

    /// 设置是否在切换前备份
    pub fn with_backup(mut self, backup: bool) -> Self {
        self.backup_before_switch = backup;
        self
    }

    /// 切换模型
    pub async fn switch(&self, cli_type: CliType, config: &SwitchConfig) -> Result<()> {
        // TODO: 实现备份功能
        if self.backup_before_switch {
            tracing::debug!("Backing up config before switch...");
            // self.backup_config(cli_type).await?;
        }

        switch_model(cli_type, config).await
    }
}

impl Default for ModelSwitcher {
    fn default() -> Self {
        Self::new()
    }
}
```

---

**Step 2.4.2: 更新 lib.rs**

在 `lib.rs` 中添加：

```rust
pub mod switcher;
pub use switcher::*;
```

---

**交付物:**
- `vibe-kanban-main/crates/cc-switch/src/switcher.rs`
- 更新后的 `lib.rs`

**验收标准:**
1. 编译通过
2. `switch_model` 函数可以正确切换各 CLI 的配置

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p cc-switch
cargo test -p cc-switch
```

---

### Task 2.5: 集成 cc-switch 到 services

**状态:** ⬜ 未开始

**前置条件:**
- Task 2.4 已完成

**目标:**
将 cc-switch crate 集成到 vibe-kanban 的 services 层。

**涉及文件:**
- 修改: `vibe-kanban-main/crates/services/Cargo.toml`
- 创建: `vibe-kanban-main/crates/services/src/services/cc_switch.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/mod.rs`

---

**Step 2.5.1: 添加依赖**

修改 `vibe-kanban-main/crates/services/Cargo.toml`，添加：

```toml
[dependencies]
cc-switch = { path = "../cc-switch" }
```

---

**Step 2.5.2: 创建 cc_switch.rs 服务**

文件路径: `vibe-kanban-main/crates/services/src/services/cc_switch.rs`

```rust
//! CC-Switch 服务
//!
//! 封装 cc-switch crate，提供与 vibe-kanban 集成的接口。

use cc_switch::{CliType, SwitchConfig, switch_model, ModelSwitcher};
use db::models::{Terminal, CliType as DbCliType, ModelConfig};
use db::DBService;
use std::sync::Arc;

/// CC-Switch 服务
pub struct CCSwitchService {
    db: Arc<DBService>,
    switcher: ModelSwitcher,
}

impl CCSwitchService {
    pub fn new(db: Arc<DBService>) -> Self {
        Self {
            db,
            switcher: ModelSwitcher::new(),
        }
    }

    /// 为终端切换模型
    ///
    /// 根据终端配置切换对应 CLI 的模型。
    pub async fn switch_for_terminal(&self, terminal: &Terminal) -> anyhow::Result<()> {
        // 获取 CLI 类型信息
        let cli_type = db::models::cli_type_dao::get_cli_type_by_id(
            &self.db.pool,
            &terminal.cli_type_id,
        ).await?
        .ok_or_else(|| anyhow::anyhow!("CLI type not found: {}", terminal.cli_type_id))?;

        // 获取模型配置
        let model_config = db::models::cli_type_dao::get_model_config_by_id(
            &self.db.pool,
            &terminal.model_config_id,
        ).await?
        .ok_or_else(|| anyhow::anyhow!("Model config not found: {}", terminal.model_config_id))?;

        // 解析 CLI 类型
        let cli = CliType::from_str(&cli_type.name)
            .ok_or_else(|| anyhow::anyhow!("Unsupported CLI: {}", cli_type.name))?;

        // 构建切换配置
        let config = SwitchConfig {
            base_url: terminal.custom_base_url.clone(),
            api_key: terminal.custom_api_key.clone()
                .ok_or_else(|| anyhow::anyhow!("API key not configured for terminal"))?,
            model: model_config.api_model_id
                .unwrap_or_else(|| model_config.name.clone()),
        };

        // 执行切换
        self.switcher.switch(cli, &config).await?;

        tracing::info!(
            "Switched model for terminal {}: cli={}, model={}",
            terminal.id,
            cli_type.display_name,
            model_config.display_name
        );

        Ok(())
    }

    /// 批量切换模型（用于工作流启动）
    ///
    /// 按顺序为所有终端切换模型配置。
    pub async fn switch_for_terminals(&self, terminals: &[Terminal]) -> anyhow::Result<()> {
        for terminal in terminals {
            self.switch_for_terminal(terminal).await?;
        }
        Ok(())
    }

    /// 检测 CLI 安装状态
    pub async fn detect_cli(&self, cli_name: &str) -> anyhow::Result<bool> {
        use tokio::process::Command;

        let cli_type = db::models::cli_type_dao::get_cli_type_by_name(
            &self.db.pool,
            cli_name,
        ).await?;

        if let Some(cli) = cli_type {
            let parts: Vec<&str> = cli.detect_command.split_whitespace().collect();
            if parts.is_empty() {
                return Ok(false);
            }

            let result = Command::new(parts[0])
                .args(&parts[1..])
                .output()
                .await;

            Ok(result.map(|o| o.status.success()).unwrap_or(false))
        } else {
            Ok(false)
        }
    }
}
```

---

**Step 2.5.3: 更新 services/mod.rs**

在 `vibe-kanban-main/crates/services/src/services/mod.rs` 中添加：

```rust
pub mod cc_switch;
pub use cc_switch::CCSwitchService;
```

---

**交付物:**
- 修改后的 `services/Cargo.toml`
- `vibe-kanban-main/crates/services/src/services/cc_switch.rs`
- 修改后的 `services/mod.rs`

**验收标准:**
1. 编译通过：`cargo build -p services`
2. CCSwitchService 可以正常实例化

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p services
```

---

### Phase 2 单元测试用例

> 在 `vibe-kanban-main/crates/cc-switch/src/tests.rs` 创建以下测试

```rust
//! CC-Switch 单元测试
//!
//! 测试配置读写、原子写入、模型切换等核心功能

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    // =========================================================================
    // 测试 1: Claude 配置文件路径解析
    // =========================================================================
    #[test]
    fn test_claude_config_path() {
        let path = get_claude_config_path();
        assert!(path.ends_with(".claude.json") || path.ends_with("claude_desktop_config.json"));
    }

    // =========================================================================
    // 测试 2: Claude 配置读取
    // =========================================================================
    #[test]
    fn test_read_claude_config() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".claude.json");

        // 创建测试配置
        let test_config = r#"{
            "apiProvider": "anthropic",
            "apiKey": "test-key-123",
            "model": "claude-sonnet-4-20250514"
        }"#;
        fs::write(&config_path, test_config).unwrap();

        let config = read_claude_config(&config_path).unwrap();
        assert_eq!(config.api_provider, Some("anthropic".to_string()));
        assert_eq!(config.api_key, Some("test-key-123".to_string()));
        assert_eq!(config.model, Some("claude-sonnet-4-20250514".to_string()));
    }

    // =========================================================================
    // 测试 3: Claude 配置写入（原子写入）
    // =========================================================================
    #[test]
    fn test_write_claude_config_atomic() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".claude.json");

        let config = ClaudeConfig {
            api_provider: Some("openai-compatible".to_string()),
            api_key: Some("sk-new-key".to_string()),
            api_base_url: Some("https://api.example.com/v1".to_string()),
            model: Some("gpt-4".to_string()),
            ..Default::default()
        };

        write_claude_config(&config_path, &config).unwrap();

        // 验证文件内容
        let content = fs::read_to_string(&config_path).unwrap();
        assert!(content.contains("openai-compatible"));
        assert!(content.contains("sk-new-key"));
        assert!(content.contains("https://api.example.com/v1"));
    }

    // =========================================================================
    // 测试 4: 原子写入失败回滚
    // =========================================================================
    #[test]
    fn test_atomic_write_rollback() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".claude.json");

        // 写入原始配置
        let original = r#"{"model": "original"}"#;
        fs::write(&config_path, original).unwrap();

        // 尝试写入无效配置（模拟失败场景）
        // 原子写入应该保护原始文件不被损坏
        let result = atomic_write(&config_path, |_| -> Result<(), std::io::Error> {
            Err(std::io::Error::new(std::io::ErrorKind::Other, "simulated failure"))
        });

        assert!(result.is_err());

        // 验证原始文件未被修改
        let content = fs::read_to_string(&config_path).unwrap();
        assert!(content.contains("original"));
    }

    // =========================================================================
    // 测试 5: Codex 配置读写
    // =========================================================================
    #[test]
    fn test_codex_config() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join(".codex");

        let config = CodexConfig {
            model: "o3-mini".to_string(),
            provider: "openai".to_string(),
            approval_mode: "suggest".to_string(),
        };

        write_codex_config(&config_path, &config).unwrap();
        let loaded = read_codex_config(&config_path).unwrap();

        assert_eq!(loaded.model, "o3-mini");
        assert_eq!(loaded.provider, "openai");
    }

    // =========================================================================
    // 测试 6: Gemini 配置读写
    // =========================================================================
    #[test]
    fn test_gemini_config() {
        let temp_dir = TempDir::new().unwrap();
        let config_path = temp_dir.path().join("settings.json");

        let config = GeminiConfig {
            model: "gemini-2.0-flash".to_string(),
            api_key: "AIza-test-key".to_string(),
        };

        write_gemini_config(&config_path, &config).unwrap();
        let loaded = read_gemini_config(&config_path).unwrap();

        assert_eq!(loaded.model, "gemini-2.0-flash");
    }

    // =========================================================================
    // 测试 7: 模型切换服务 - Claude
    // =========================================================================
    #[tokio::test]
    async fn test_switch_claude_model() {
        let temp_dir = TempDir::new().unwrap();
        let switcher = ModelSwitcher::new_with_config_dir(temp_dir.path().to_path_buf());

        let request = SwitchModelRequest {
            cli_type: CliType::ClaudeCode,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: Some("test-key".to_string()),
            api_base_url: None,
        };

        switcher.switch_model(&request).await.unwrap();

        // 验证配置已更新
        let config = switcher.get_current_config(CliType::ClaudeCode).await.unwrap();
        assert_eq!(config.model, Some("claude-sonnet-4-20250514".to_string()));
    }

    // =========================================================================
    // 测试 8: 模型列表获取（需要 Mock）
    // =========================================================================
    #[tokio::test]
    async fn test_fetch_available_models() {
        // 使用 Mock HTTP 服务器
        let mock_server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/v1/models"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "data": [
                    {"id": "claude-sonnet-4-20250514"},
                    {"id": "claude-3-5-haiku-20241022"}
                ]
            })))
            .mount(&mock_server)
            .await;

        let fetcher = ModelFetcher::new(&mock_server.uri(), "test-key");
        let models = fetcher.fetch_models().await.unwrap();

        assert_eq!(models.len(), 2);
        assert!(models.iter().any(|m| m.id == "claude-sonnet-4-20250514"));
    }
}
```

**运行测试:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p cc-switch -- --nocapture
```

---

## Phase 2 完成检查清单

- [ ] Task 2.1: CC-Switch 代码分析完成
- [ ] Task 2.2: cc-switch crate 创建完成，编译通过
- [ ] Task 2.3: 配置读写功能实现，测试通过
- [ ] Task 2.4: 模型切换服务实现
- [ ] Task 2.5: 集成到 services 层

---

## Phase 3: Orchestrator 主 Agent 实现

### Task 3.1: 创建 Orchestrator 模块结构

**状态:** ⬜ 未开始

**前置条件:**
- Phase 2 已完成
- cc-switch 集成到 services 层

**目标:**
创建 Orchestrator 模块的基础结构，包括配置、状态管理和核心类型定义。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/mod.rs`
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/config.rs`
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/state.rs`
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/types.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/mod.rs`

---

**Step 3.1.1: 创建 types.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/orchestrator/types.rs`

```rust
//! Orchestrator 类型定义

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// 主 Agent 指令类型
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum OrchestratorInstruction {
    /// 启动任务
    StartTask {
        task_id: String,
        instruction: String,
    },
    /// 发送消息到终端
    SendToTerminal {
        terminal_id: String,
        message: String,
    },
    /// 审核代码
    ReviewCode {
        terminal_id: String,
        commit_hash: String,
    },
    /// 修复问题
    FixIssues {
        terminal_id: String,
        issues: Vec<String>,
    },
    /// 合并分支
    MergeBranch {
        source_branch: String,
        target_branch: String,
    },
    /// 暂停工作流
    PauseWorkflow {
        reason: String,
    },
    /// 完成工作流
    CompleteWorkflow {
        summary: String,
    },
    /// 失败工作流
    FailWorkflow {
        reason: String,
    },
}

/// 终端完成事件
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TerminalCompletionEvent {
    pub terminal_id: String,
    pub task_id: String,
    pub workflow_id: String,
    pub status: TerminalCompletionStatus,
    pub commit_hash: Option<String>,
    pub commit_message: Option<String>,
    pub metadata: Option<CommitMetadata>,
}

/// 终端完成状态
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TerminalCompletionStatus {
    /// 任务完成
    Completed,
    /// 审核通过
    ReviewPass,
    /// 审核打回
    ReviewReject,
    /// 失败
    Failed,
}

/// Git 提交元数据
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommitMetadata {
    pub workflow_id: String,
    pub task_id: String,
    pub terminal_id: String,
    pub terminal_order: i32,
    pub cli: String,
    pub model: String,
    pub status: String,
    pub severity: Option<String>,
    pub reviewed_terminal: Option<String>,
    pub issues: Option<Vec<CodeIssue>>,
    pub next_action: String,
}

/// 代码问题
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CodeIssue {
    pub severity: String,
    pub file: String,
    pub line: Option<i32>,
    pub message: String,
    pub suggestion: Option<String>,
}

/// LLM 消息
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LLMMessage {
    pub role: String,
    pub content: String,
}

/// LLM 响应
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LLMResponse {
    pub content: String,
    pub usage: Option<LLMUsage>,
}

/// LLM 使用量
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LLMUsage {
    pub prompt_tokens: i32,
    pub completion_tokens: i32,
    pub total_tokens: i32,
}
```

---

**Step 3.1.2: 创建 config.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/orchestrator/config.rs`

```rust
//! Orchestrator 配置

use serde::{Deserialize, Serialize};

/// Orchestrator 配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrchestratorConfig {
    /// API 类型: "openai", "anthropic", "custom"
    pub api_type: String,

    /// API Base URL
    pub base_url: String,

    /// API Key
    pub api_key: String,

    /// 模型名称
    pub model: String,

    /// 最大重试次数
    #[serde(default = "default_max_retries")]
    pub max_retries: u32,

    /// 请求超时（秒）
    #[serde(default = "default_timeout")]
    pub timeout_secs: u64,

    /// 系统提示词
    #[serde(default = "default_system_prompt")]
    pub system_prompt: String,
}

fn default_max_retries() -> u32 {
    3
}

fn default_timeout() -> u64 {
    120
}

fn default_system_prompt() -> String {
    r#"你是 SoloDawn 的主协调 Agent，负责协调多个 AI 编码代理完成软件开发任务。

你的职责：
1. 根据工作流配置，向各终端发送任务指令
2. 监控终端的执行状态（通过 Git 提交事件）
3. 协调审核流程，处理审核反馈
4. 在所有任务完成后，协调分支合并

规则：
- 每个终端完成任务后会提交 Git，你会收到提交事件
- 根据提交中的元数据判断下一步操作
- 如果审核发现问题，指导修复终端进行修复
- 保持简洁的指令，不要过度解释

输出格式：
使用 JSON 格式输出指令，格式如下：
{"type": "send_to_terminal", "terminal_id": "xxx", "message": "具体指令"}
"#.to_string()
}

impl Default for OrchestratorConfig {
    fn default() -> Self {
        Self {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: String::new(),
            model: "gpt-4o".to_string(),
            max_retries: default_max_retries(),
            timeout_secs: default_timeout(),
            system_prompt: default_system_prompt(),
        }
    }
}

impl OrchestratorConfig {
    /// 从工作流配置创建
    pub fn from_workflow(
        api_type: Option<&str>,
        base_url: Option<&str>,
        api_key: Option<&str>,
        model: Option<&str>,
    ) -> Option<Self> {
        Some(Self {
            api_type: api_type?.to_string(),
            base_url: base_url?.to_string(),
            api_key: api_key?.to_string(),
            model: model?.to_string(),
            ..Default::default()
        })
    }

    /// 验证配置是否有效
    pub fn validate(&self) -> Result<(), String> {
        if self.api_key.is_empty() {
            return Err("API key is required".to_string());
        }
        if self.base_url.is_empty() {
            return Err("Base URL is required".to_string());
        }
        if self.model.is_empty() {
            return Err("Model is required".to_string());
        }
        Ok(())
    }
}
```

---

**Step 3.1.3: 创建 state.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/orchestrator/state.rs`

```rust
//! Orchestrator 状态管理

use std::collections::HashMap;
use tokio::sync::RwLock;
use super::types::*;

/// Orchestrator 运行状态
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OrchestratorRunState {
    /// 空闲（等待事件）
    Idle,
    /// 处理中
    Processing,
    /// 已暂停
    Paused,
    /// 已停止
    Stopped,
}

/// 任务执行状态
#[derive(Debug, Clone)]
pub struct TaskExecutionState {
    pub task_id: String,
    pub current_terminal_index: usize,
    pub total_terminals: usize,
    pub completed_terminals: Vec<String>,
    pub failed_terminals: Vec<String>,
    pub is_completed: bool,
}

/// Orchestrator 状态
pub struct OrchestratorState {
    /// 运行状态
    pub run_state: OrchestratorRunState,

    /// 工作流 ID
    pub workflow_id: String,

    /// 任务执行状态
    pub task_states: HashMap<String, TaskExecutionState>,

    /// 对话历史（用于 LLM 上下文）
    pub conversation_history: Vec<LLMMessage>,

    /// 待处理事件队列
    pub pending_events: Vec<TerminalCompletionEvent>,

    /// Token 使用统计
    pub total_tokens_used: i64,

    /// 错误计数
    pub error_count: u32,
}

impl OrchestratorState {
    pub fn new(workflow_id: String) -> Self {
        Self {
            run_state: OrchestratorRunState::Idle,
            workflow_id,
            task_states: HashMap::new(),
            conversation_history: Vec::new(),
            pending_events: Vec::new(),
            total_tokens_used: 0,
            error_count: 0,
        }
    }

    /// 初始化任务状态
    pub fn init_task(&mut self, task_id: String, terminal_count: usize) {
        self.task_states.insert(task_id.clone(), TaskExecutionState {
            task_id,
            current_terminal_index: 0,
            total_terminals: terminal_count,
            completed_terminals: Vec::new(),
            failed_terminals: Vec::new(),
            is_completed: false,
        });
    }

    /// 标记终端完成
    pub fn mark_terminal_completed(&mut self, task_id: &str, terminal_id: &str, success: bool) {
        if let Some(state) = self.task_states.get_mut(task_id) {
            if success {
                state.completed_terminals.push(terminal_id.to_string());
            } else {
                state.failed_terminals.push(terminal_id.to_string());
            }

            // 检查任务是否完成
            let total_done = state.completed_terminals.len() + state.failed_terminals.len();
            if total_done >= state.total_terminals {
                state.is_completed = true;
            }
        }
    }

    /// 添加消息到对话历史
    pub fn add_message(&mut self, role: &str, content: &str) {
        self.conversation_history.push(LLMMessage {
            role: role.to_string(),
            content: content.to_string(),
        });

        // 限制历史长度，避免上下文过长
        const MAX_HISTORY: usize = 50;
        if self.conversation_history.len() > MAX_HISTORY {
            // 保留系统消息和最近的消息
            let system_msgs: Vec<_> = self.conversation_history
                .iter()
                .filter(|m| m.role == "system")
                .cloned()
                .collect();
            let recent: Vec<_> = self.conversation_history
                .iter()
                .rev()
                .take(MAX_HISTORY - system_msgs.len())
                .cloned()
                .collect();

            self.conversation_history = system_msgs;
            self.conversation_history.extend(recent.into_iter().rev());
        }
    }

    /// 检查所有任务是否完成
    pub fn all_tasks_completed(&self) -> bool {
        self.task_states.values().all(|s| s.is_completed)
    }

    /// 检查是否有失败的任务
    pub fn has_failed_tasks(&self) -> bool {
        self.task_states.values().any(|s| !s.failed_terminals.is_empty())
    }
}

/// 线程安全的状态包装
pub type SharedOrchestratorState = std::sync::Arc<RwLock<OrchestratorState>>;
```

---

**Step 3.1.4: 创建 mod.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/orchestrator/mod.rs`

```rust
//! Orchestrator 主 Agent 模块
//!
//! 负责协调多个 AI 编码代理完成软件开发任务。

pub mod config;
pub mod state;
pub mod types;

// 后续任务中添加
// pub mod llm;
// pub mod message_bus;
// pub mod agent;

pub use config::OrchestratorConfig;
pub use state::{OrchestratorState, OrchestratorRunState, SharedOrchestratorState};
pub use types::*;
```

---

**Step 3.1.5: 更新 services/mod.rs**

在 `vibe-kanban-main/crates/services/src/services/mod.rs` 中添加：

```rust
pub mod orchestrator;
pub use orchestrator::{OrchestratorConfig, OrchestratorState};
```

---

**交付物:**
- `orchestrator/mod.rs`
- `orchestrator/config.rs`
- `orchestrator/state.rs`
- `orchestrator/types.rs`

**验收标准:**
1. 编译通过：`cargo build -p services`
2. 类型定义完整，可以序列化/反序列化

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo build -p services
```

---

### Task 3.2: 实现 LLM 客户端抽象

**状态:** ⬜ 未开始

**前置条件:**
- Task 3.1 已完成

**目标:**
实现统一的 LLM 客户端接口，支持 OpenAI 兼容 API。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/llm.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/orchestrator/mod.rs`

---

**Step 3.2.1: 创建 llm.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/orchestrator/llm.rs`

```rust
//! LLM 客户端抽象

use async_trait::async_trait;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::time::Duration;
use super::config::OrchestratorConfig;
use super::types::{LLMMessage, LLMResponse, LLMUsage};

#[async_trait]
pub trait LLMClient: Send + Sync {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse>;
}

pub struct OpenAICompatibleClient {
    client: Client,
    base_url: String,
    api_key: String,
    model: String,
}

#[derive(Debug, Serialize)]
struct ChatRequest {
    model: String,
    messages: Vec<ChatMessage>,
    temperature: Option<f32>,
    max_tokens: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize)]
struct ChatMessage {
    role: String,
    content: String,
}

#[derive(Debug, Deserialize)]
struct ChatResponse {
    choices: Vec<ChatChoice>,
    usage: Option<UsageInfo>,
}

#[derive(Debug, Deserialize)]
struct ChatChoice {
    message: ChatMessage,
}

#[derive(Debug, Deserialize)]
struct UsageInfo {
    prompt_tokens: i32,
    completion_tokens: i32,
    total_tokens: i32,
}

impl OpenAICompatibleClient {
    pub fn new(config: &OrchestratorConfig) -> Self {
        let client = Client::builder()
            .timeout(Duration::from_secs(config.timeout_secs))
            .build()
            .expect("Failed to create HTTP client");

        Self {
            client,
            base_url: config.base_url.trim_end_matches('/').to_string(),
            api_key: config.api_key.clone(),
            model: config.model.clone(),
        }
    }
}

#[async_trait]
impl LLMClient for OpenAICompatibleClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        let url = format!("{}/chat/completions", self.base_url);

        let chat_messages: Vec<ChatMessage> = messages
            .into_iter()
            .map(|m| ChatMessage { role: m.role, content: m.content })
            .collect();

        let request = ChatRequest {
            model: self.model.clone(),
            messages: chat_messages,
            temperature: Some(0.7),
            max_tokens: Some(4096),
        };

        let response = self.client
            .post(&url)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("Content-Type", "application/json")
            .json(&request)
            .send()
            .await?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow::anyhow!("LLM API error: {} - {}", status, body));
        }

        let chat_response: ChatResponse = response.json().await?;
        let content = chat_response.choices.first()
            .map(|c| c.message.content.clone())
            .unwrap_or_default();

        let usage = chat_response.usage.map(|u| LLMUsage {
            prompt_tokens: u.prompt_tokens,
            completion_tokens: u.completion_tokens,
            total_tokens: u.total_tokens,
        });

        Ok(LLMResponse { content, usage })
    }
}

pub fn create_llm_client(config: &OrchestratorConfig) -> anyhow::Result<Box<dyn LLMClient>> {
    config.validate().map_err(|e| anyhow::anyhow!(e))?;
    Ok(Box::new(OpenAICompatibleClient::new(config)))
}
```

**交付物:** `orchestrator/llm.rs`

---

### Task 3.3: 实现消息总线

**状态:** ⬜ 未开始

**前置条件:** Task 3.2 已完成

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/message_bus.rs`

---

**Step 3.3.1: 创建 message_bus.rs**

```rust
//! 消息总线

use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{mpsc, RwLock, broadcast};
use super::types::*;

#[derive(Debug, Clone)]
pub enum BusMessage {
    TerminalCompleted(TerminalCompletionEvent),
    GitEvent { workflow_id: String, commit_hash: String, branch: String, message: String },
    Instruction(OrchestratorInstruction),
    StatusUpdate { workflow_id: String, status: String },
    Error { workflow_id: String, error: String },
    Shutdown,
}

pub struct MessageBus {
    broadcast_tx: broadcast::Sender<BusMessage>,
    subscribers: Arc<RwLock<HashMap<String, Vec<mpsc::Sender<BusMessage>>>>>,
}

impl MessageBus {
    pub fn new(capacity: usize) -> Self {
        let (broadcast_tx, _) = broadcast::channel(capacity);
        Self { broadcast_tx, subscribers: Arc::new(RwLock::new(HashMap::new())) }
    }

    pub fn broadcast(&self, message: BusMessage) -> Result<usize, broadcast::error::SendError<BusMessage>> {
        self.broadcast_tx.send(message)
    }

    pub fn subscribe_broadcast(&self) -> broadcast::Receiver<BusMessage> {
        self.broadcast_tx.subscribe()
    }

    pub async fn subscribe(&self, topic: &str) -> mpsc::Receiver<BusMessage> {
        let (tx, rx) = mpsc::channel(100);
        let mut subscribers = self.subscribers.write().await;
        subscribers.entry(topic.to_string()).or_default().push(tx);
        rx
    }

    pub async fn publish(&self, topic: &str, message: BusMessage) {
        let subscribers = self.subscribers.read().await;
        if let Some(subs) = subscribers.get(topic) {
            for tx in subs { let _ = tx.send(message.clone()).await; }
        }
    }

    pub async fn publish_terminal_completed(&self, event: TerminalCompletionEvent) {
        let topic = format!("workflow:{}", event.workflow_id);
        self.publish(&topic, BusMessage::TerminalCompleted(event.clone())).await;
        let _ = self.broadcast(BusMessage::TerminalCompleted(event));
    }
}

impl Default for MessageBus {
    fn default() -> Self { Self::new(1000) }
}

pub type SharedMessageBus = Arc<MessageBus>;
```

**交付物:** `orchestrator/message_bus.rs`

---

### Task 3.4: 实现 OrchestratorAgent

**状态:** ⬜ 未开始

**前置条件:** Task 3.3 已完成

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/orchestrator/agent.rs`

---

**Step 3.4.1: 创建 agent.rs**

```rust
//! Orchestrator Agent 主逻辑

use std::sync::Arc;
use tokio::sync::RwLock;
use db::DBService;
use super::config::OrchestratorConfig;
use super::state::{OrchestratorState, OrchestratorRunState, SharedOrchestratorState};
use super::llm::{LLMClient, create_llm_client};
use super::message_bus::{MessageBus, BusMessage, SharedMessageBus};
use super::types::*;

pub struct OrchestratorAgent {
    config: OrchestratorConfig,
    state: SharedOrchestratorState,
    message_bus: SharedMessageBus,
    llm_client: Box<dyn LLMClient>,
    db: Arc<DBService>,
}

impl OrchestratorAgent {
    pub async fn new(
        config: OrchestratorConfig,
        workflow_id: String,
        message_bus: SharedMessageBus,
        db: Arc<DBService>,
    ) -> anyhow::Result<Self> {
        let llm_client = create_llm_client(&config)?;
        let state = Arc::new(RwLock::new(OrchestratorState::new(workflow_id)));

        Ok(Self { config, state, message_bus, llm_client, db })
    }

    /// 启动 Agent 事件循环
    pub async fn run(&self) -> anyhow::Result<()> {
        let workflow_id = {
            let state = self.state.read().await;
            state.workflow_id.clone()
        };

        let mut rx = self.message_bus.subscribe(&format!("workflow:{}", workflow_id)).await;
        tracing::info!("Orchestrator started for workflow: {}", workflow_id);

        // 初始化系统消息
        {
            let mut state = self.state.write().await;
            state.add_message("system", &self.config.system_prompt);
            state.run_state = OrchestratorRunState::Idle;
        }

        // 事件循环
        while let Some(message) = rx.recv().await {
            let should_stop = self.handle_message(message).await?;
            if should_stop { break; }
        }

        tracing::info!("Orchestrator stopped for workflow: {}", workflow_id);
        Ok(())
    }

    /// 处理消息
    async fn handle_message(&self, message: BusMessage) -> anyhow::Result<bool> {
        match message {
            BusMessage::TerminalCompleted(event) => {
                self.handle_terminal_completed(event).await?;
            }
            BusMessage::GitEvent { workflow_id, commit_hash, branch, message } => {
                self.handle_git_event(&workflow_id, &commit_hash, &branch, &message).await?;
            }
            BusMessage::Shutdown => {
                return Ok(true);
            }
            _ => {}
        }
        Ok(false)
    }

    /// 处理终端完成事件
    async fn handle_terminal_completed(&self, event: TerminalCompletionEvent) -> anyhow::Result<()> {
        tracing::info!("Terminal completed: {} with status {:?}", event.terminal_id, event.status);

        // 更新状态
        {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Processing;
            let success = matches!(event.status, TerminalCompletionStatus::Completed | TerminalCompletionStatus::ReviewPass);
            state.mark_terminal_completed(&event.task_id, &event.terminal_id, success);
        }

        // 构建提示并调用 LLM
        let prompt = self.build_completion_prompt(&event).await;
        let response = self.call_llm(&prompt).await?;

        // 解析并执行指令
        self.execute_instruction(&response).await?;

        // 恢复空闲状态
        {
            let mut state = self.state.write().await;
            state.run_state = OrchestratorRunState::Idle;
        }

        Ok(())
    }

    /// 处理 Git 事件
    async fn handle_git_event(
        &self,
        _workflow_id: &str,
        commit_hash: &str,
        branch: &str,
        message: &str,
    ) -> anyhow::Result<()> {
        tracing::info!("Git event: {} on branch {} - {}", commit_hash, branch, message);
        // Git 事件通常会转换为 TerminalCompleted 事件
        Ok(())
    }

    /// 构建完成提示
    async fn build_completion_prompt(&self, event: &TerminalCompletionEvent) -> String {
        format!(
            "终端 {} 已完成任务。\n状态: {:?}\n提交: {:?}\n消息: {:?}\n\n请决定下一步操作。",
            event.terminal_id,
            event.status,
            event.commit_hash,
            event.commit_message
        )
    }

    /// 调用 LLM
    async fn call_llm(&self, prompt: &str) -> anyhow::Result<String> {
        let mut state = self.state.write().await;
        state.add_message("user", prompt);

        let messages = state.conversation_history.clone();
        drop(state);

        let response = self.llm_client.chat(messages).await?;

        let mut state = self.state.write().await;
        state.add_message("assistant", &response.content);
        if let Some(usage) = &response.usage {
            state.total_tokens_used += usage.total_tokens as i64;
        }

        Ok(response.content)
    }

    /// 执行指令
    async fn execute_instruction(&self, response: &str) -> anyhow::Result<()> {
        // 尝试解析 JSON 指令
        if let Ok(instruction) = serde_json::from_str::<OrchestratorInstruction>(response) {
            match instruction {
                OrchestratorInstruction::SendToTerminal { terminal_id, message } => {
                    tracing::info!("Sending to terminal {}: {}", terminal_id, message);
                    // TODO: 实际发送到终端
                }
                OrchestratorInstruction::CompleteWorkflow { summary } => {
                    tracing::info!("Workflow completed: {}", summary);
                }
                OrchestratorInstruction::FailWorkflow { reason } => {
                    tracing::error!("Workflow failed: {}", reason);
                }
                _ => {}
            }
        }
        Ok(())
    }
}
```

**交付物:** `orchestrator/agent.rs`

**验收标准:**
1. 编译通过：`cargo build -p services`
2. OrchestratorAgent 可以实例化并运行

---

### Phase 3 单元测试用例

> 在 `vibe-kanban-main/crates/services/src/services/orchestrator/tests.rs` 创建以下测试

```rust
//! Orchestrator 单元测试
//!
//! 测试 LLM 客户端、消息总线、Agent 核心功能

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::sync::mpsc;
    use wiremock::{MockServer, Mock, ResponseTemplate};
    use wiremock::matchers::{method, path, body_json_schema};

    // =========================================================================
    // 测试 1: LLM 客户端 - 基本请求
    // =========================================================================
    #[tokio::test]
    async fn test_llm_client_basic_request() {
        let mock_server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "choices": [{
                    "message": {
                        "role": "assistant",
                        "content": "Hello! How can I help you?"
                    }
                }]
            })))
            .mount(&mock_server)
            .await;

        let client = LlmClient::new(&mock_server.uri(), "test-key");
        let response = client.chat(&[
            ChatMessage::user("Hello")
        ]).await.unwrap();

        assert!(response.content.contains("Hello"));
    }

    // =========================================================================
    // 测试 2: LLM 客户端 - 流式响应
    // =========================================================================
    #[tokio::test]
    async fn test_llm_client_streaming() {
        let mock_server = MockServer::start().await;

        // 模拟 SSE 流式响应
        let sse_body = r#"data: {"choices":[{"delta":{"content":"Hello"}}]}

data: {"choices":[{"delta":{"content":" world"}}]}

data: [DONE]
"#;

        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .respond_with(ResponseTemplate::new(200)
                .set_body_string(sse_body)
                .insert_header("content-type", "text/event-stream"))
            .mount(&mock_server)
            .await;

        let client = LlmClient::new(&mock_server.uri(), "test-key");
        let mut stream = client.chat_stream(&[
            ChatMessage::user("Hello")
        ]).await.unwrap();

        let mut full_response = String::new();
        while let Some(chunk) = stream.next().await {
            full_response.push_str(&chunk.unwrap());
        }

        assert_eq!(full_response, "Hello world");
    }

    // =========================================================================
    // 测试 3: 消息总线 - 订阅和发布
    // =========================================================================
    #[tokio::test]
    async fn test_message_bus_pubsub() {
        let bus = MessageBus::new();

        let mut subscriber = bus.subscribe("terminal:T1").await;

        bus.publish("terminal:T1", BusMessage::Text("Hello T1".to_string())).await;

        let msg = tokio::time::timeout(
            std::time::Duration::from_secs(1),
            subscriber.recv()
        ).await.unwrap().unwrap();

        assert!(matches!(msg, BusMessage::Text(s) if s == "Hello T1"));
    }

    // =========================================================================
    // 测试 4: 消息总线 - 主题隔离
    // =========================================================================
    #[tokio::test]
    async fn test_message_bus_topic_isolation() {
        let bus = MessageBus::new();

        let mut sub_t1 = bus.subscribe("terminal:T1").await;
        let mut sub_t2 = bus.subscribe("terminal:T2").await;

        bus.publish("terminal:T1", BusMessage::Text("For T1 only".to_string())).await;

        // T1 应该收到
        let msg = tokio::time::timeout(
            std::time::Duration::from_millis(100),
            sub_t1.recv()
        ).await;
        assert!(msg.is_ok());

        // T2 不应该收到
        let msg = tokio::time::timeout(
            std::time::Duration::from_millis(100),
            sub_t2.recv()
        ).await;
        assert!(msg.is_err()); // 超时
    }

    // =========================================================================
    // 测试 5: OrchestratorAgent - 处理 Git 事件
    // =========================================================================
    #[tokio::test]
    async fn test_orchestrator_handle_git_event() {
        let (msg_tx, msg_rx) = mpsc::channel(10);
        let mock_llm = MockLlmClient::new();

        let agent = OrchestratorAgent::new(mock_llm, msg_tx);

        let event = OrchestratorMessage::GitCommitDetected {
            branch: "feature/login".to_string(),
            commit: "abc123".to_string(),
            parsed_commit: ParsedCommit {
                status: Some(TaskStatus::Completed),
                terminal_id: Some("T1".to_string()),
                ..Default::default()
            },
        };

        agent.handle_message(event).await.unwrap();

        // 验证处理逻辑被触发
        assert!(agent.get_terminal_status("T1").await.is_some());
    }

    // =========================================================================
    // 测试 6: OrchestratorAgent - 任务分配
    // =========================================================================
    #[tokio::test]
    async fn test_orchestrator_task_assignment() {
        let (msg_tx, mut msg_rx) = mpsc::channel(10);
        let mock_llm = MockLlmClient::with_response(
            "Based on the analysis, Terminal T2 should handle the database migration."
        );

        let agent = OrchestratorAgent::new(mock_llm, msg_tx);

        // 模拟工作流配置
        let workflow = WorkflowConfig {
            tasks: vec![
                TaskConfig { id: "task-1".into(), name: "Backend API".into(), terminals: vec!["T1".into()] },
                TaskConfig { id: "task-2".into(), name: "Database".into(), terminals: vec!["T2".into()] },
            ],
            ..Default::default()
        };

        agent.start_workflow(workflow).await.unwrap();

        // 验证任务被分配
        let msg = msg_rx.recv().await.unwrap();
        assert!(matches!(msg, BusMessage::TaskAssigned { .. }));
    }

    // =========================================================================
    // 测试 7: OrchestratorAgent - 错误处理
    // =========================================================================
    #[tokio::test]
    async fn test_orchestrator_error_handling() {
        let (msg_tx, mut msg_rx) = mpsc::channel(10);
        let mock_llm = MockLlmClient::new();

        let mut agent = OrchestratorAgent::new(mock_llm, msg_tx);
        agent.set_error_terminal("T-ERR".to_string());

        let event = OrchestratorMessage::TerminalError {
            terminal_id: "T1".to_string(),
            error: "Connection refused".to_string(),
        };

        agent.handle_message(event).await.unwrap();

        // 验证错误被路由到错误终端
        let msg = msg_rx.recv().await.unwrap();
        match msg {
            BusMessage::ErrorReport { target_terminal, .. } => {
                assert_eq!(target_terminal, "T-ERR");
            }
            _ => panic!("Expected ErrorReport message"),
        }
    }

    // =========================================================================
    // 测试 8: LLM 客户端 - 重试机制
    // =========================================================================
    #[tokio::test]
    async fn test_llm_client_retry() {
        let mock_server = MockServer::start().await;

        // 前两次返回 500，第三次成功
        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .respond_with(ResponseTemplate::new(500))
            .up_to_n_times(2)
            .mount(&mock_server)
            .await;

        Mock::given(method("POST"))
            .and(path("/v1/chat/completions"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "choices": [{"message": {"content": "Success after retry"}}]
            })))
            .mount(&mock_server)
            .await;

        let client = LlmClient::new(&mock_server.uri(), "test-key")
            .with_retry(3, std::time::Duration::from_millis(10));

        let response = client.chat(&[ChatMessage::user("Test")]).await.unwrap();
        assert!(response.content.contains("Success"));
    }
}
```

**运行测试:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p services orchestrator -- --nocapture
```

---

## Phase 3 完成检查清单

- [ ] Task 3.1: Orchestrator 模块结构创建完成
- [ ] Task 3.2: LLM 客户端实现完成
- [ ] Task 3.3: 消息总线实现完成
- [ ] Task 3.4: OrchestratorAgent 实现完成

---

## Phase 4: 终端管理与启动机制

### Task 4.1: 实现 TerminalLauncher

**状态:** ⬜ 未开始

**前置条件:**
- Phase 3 已完成
- cc-switch 服务可用

**目标:**
实现终端启动器，负责串行启动所有终端（切换环境变量 → 启动 → 下一个）。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/terminal/mod.rs`
- 创建: `vibe-kanban-main/crates/services/src/services/terminal/launcher.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/mod.rs`

---

**Step 4.1.1: 创建 terminal/mod.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/terminal/mod.rs`

```rust
//! 终端管理模块

pub mod launcher;
pub mod process;
pub mod detector;

pub use launcher::TerminalLauncher;
pub use process::{ProcessHandle, ProcessManager};
pub use detector::CliDetector;
```

---

**Step 4.1.2: 创建 launcher.rs**

文件路径: `vibe-kanban-main/crates/services/src/services/terminal/launcher.rs`

```rust
//! 终端启动器

use std::sync::Arc;
use std::path::PathBuf;
use tokio::process::Command;
use db::DBService;
use db::models::{Terminal, terminal_dao, cli_type_dao, workflow_dao};
use super::process::{ProcessHandle, ProcessManager};
use crate::services::cc_switch::CCSwitchService;

/// 终端启动器
pub struct TerminalLauncher {
    db: Arc<DBService>,
    cc_switch: Arc<CCSwitchService>,
    process_manager: Arc<ProcessManager>,
    working_dir: PathBuf,
}

/// 启动结果
pub struct LaunchResult {
    pub terminal_id: String,
    pub process_handle: Option<ProcessHandle>,
    pub success: bool,
    pub error: Option<String>,
}

impl TerminalLauncher {
    pub fn new(
        db: Arc<DBService>,
        cc_switch: Arc<CCSwitchService>,
        process_manager: Arc<ProcessManager>,
        working_dir: PathBuf,
    ) -> Self {
        Self { db, cc_switch, process_manager, working_dir }
    }

    /// 启动工作流的所有终端（串行）
    pub async fn launch_all(&self, workflow_id: &str) -> anyhow::Result<Vec<LaunchResult>> {
        let terminals = terminal_dao::get_terminals_by_workflow(&self.db.pool, workflow_id).await?;
        let mut results = Vec::new();

        tracing::info!("Launching {} terminals for workflow {}", terminals.len(), workflow_id);

        for terminal in terminals {
            let result = self.launch_terminal(&terminal).await;
            results.push(result);

            // 短暂延迟，确保环境变量切换生效
            tokio::time::sleep(tokio::time::Duration::from_millis(500)).await;
        }

        Ok(results)
    }

    /// 启动单个终端
    async fn launch_terminal(&self, terminal: &Terminal) -> LaunchResult {
        let terminal_id = terminal.id.clone();

        // 1. 切换模型配置
        if let Err(e) = self.cc_switch.switch_for_terminal(terminal).await {
            tracing::error!("Failed to switch model for terminal {}: {}", terminal_id, e);
            return LaunchResult {
                terminal_id,
                process_handle: None,
                success: false,
                error: Some(format!("Model switch failed: {}", e)),
            };
        }

        // 2. 获取 CLI 信息
        let cli_type = match cli_type_dao::get_cli_type_by_id(&self.db.pool, &terminal.cli_type_id).await {
            Ok(Some(cli)) => cli,
            Ok(None) => {
                return LaunchResult {
                    terminal_id,
                    process_handle: None,
                    success: false,
                    error: Some("CLI type not found".to_string()),
                };
            }
            Err(e) => {
                return LaunchResult {
                    terminal_id,
                    process_handle: None,
                    success: false,
                    error: Some(format!("Database error: {}", e)),
                };
            }
        };

        // 3. 构建启动命令
        let cmd = self.build_launch_command(&cli_type.name);

        // 4. 启动进程
        match self.process_manager.spawn(&terminal_id, cmd, &self.working_dir).await {
            Ok(handle) => {
                // 更新终端状态
                let _ = terminal_dao::set_terminal_started(&self.db.pool, &terminal_id).await;
                let _ = terminal_dao::update_terminal_process(
                    &self.db.pool,
                    &terminal_id,
                    Some(handle.pid as i32),
                    Some(&handle.session_id),
                ).await;

                tracing::info!("Terminal {} started with PID {}", terminal_id, handle.pid);

                LaunchResult {
                    terminal_id,
                    process_handle: Some(handle),
                    success: true,
                    error: None,
                }
            }
            Err(e) => {
                tracing::error!("Failed to start terminal {}: {}", terminal_id, e);
                LaunchResult {
                    terminal_id,
                    process_handle: None,
                    success: false,
                    error: Some(format!("Process spawn failed: {}", e)),
                }
            }
        }
    }

    /// 构建启动命令
    fn build_launch_command(&self, cli_name: &str) -> Command {
        let mut cmd = match cli_name {
            "claude-code" => {
                let mut c = Command::new("claude");
                c.arg("--dangerously-skip-permissions");
                c
            }
            "gemini-cli" => Command::new("gemini"),
            "codex" => Command::new("codex"),
            "amp" => Command::new("amp"),
            "cursor-agent" => Command::new("cursor"),
            _ => Command::new(cli_name),
        };

        cmd.current_dir(&self.working_dir);
        cmd.kill_on_drop(true);

        cmd
    }

    /// 停止所有终端
    pub async fn stop_all(&self, workflow_id: &str) -> anyhow::Result<()> {
        let terminals = terminal_dao::get_terminals_by_workflow(&self.db.pool, workflow_id).await?;

        for terminal in terminals {
            if let Some(pid) = terminal.process_id {
                self.process_manager.kill(pid as u32).await?;
            }
            terminal_dao::update_terminal_status(&self.db.pool, &terminal.id, "cancelled").await?;
        }

        Ok(())
    }
}
```

---

**交付物:** `terminal/mod.rs`, `terminal/launcher.rs`

---

### Task 4.2: 实现进程管理

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/terminal/process.rs`

---

**Step 4.2.1: 创建 process.rs**

```rust
//! 进程管理

use std::collections::HashMap;
use std::path::Path;
use std::sync::Arc;
use tokio::process::{Command, Child};
use tokio::sync::RwLock;
use uuid::Uuid;

/// 进程句柄
#[derive(Debug)]
pub struct ProcessHandle {
    pub pid: u32,
    pub session_id: String,
    pub terminal_id: String,
}

/// 进程管理器
pub struct ProcessManager {
    processes: Arc<RwLock<HashMap<String, Child>>>,
}

impl ProcessManager {
    pub fn new() -> Self {
        Self { processes: Arc::new(RwLock::new(HashMap::new())) }
    }

    /// 启动进程
    pub async fn spawn(
        &self,
        terminal_id: &str,
        mut cmd: Command,
        working_dir: &Path,
    ) -> anyhow::Result<ProcessHandle> {
        cmd.current_dir(working_dir);

        // 配置标准输入输出
        cmd.stdin(std::process::Stdio::piped());
        cmd.stdout(std::process::Stdio::piped());
        cmd.stderr(std::process::Stdio::piped());

        let child = cmd.spawn()?;
        let pid = child.id().unwrap_or(0);
        let session_id = Uuid::new_v4().to_string();

        let mut processes = self.processes.write().await;
        processes.insert(terminal_id.to_string(), child);

        Ok(ProcessHandle {
            pid,
            session_id,
            terminal_id: terminal_id.to_string(),
        })
    }

    /// 终止进程
    pub async fn kill(&self, pid: u32) -> anyhow::Result<()> {
        #[cfg(unix)]
        {
            use nix::sys::signal::{kill, Signal};
            use nix::unistd::Pid;
            let _ = kill(Pid::from_raw(pid as i32), Signal::SIGTERM);
        }

        #[cfg(windows)]
        {
            let _ = std::process::Command::new("taskkill")
                .args(["/PID", &pid.to_string(), "/F"])
                .output();
        }

        Ok(())
    }

    /// 检查进程是否运行
    pub async fn is_running(&self, terminal_id: &str) -> bool {
        let processes = self.processes.read().await;
        if let Some(child) = processes.get(terminal_id) {
            child.id().is_some()
        } else {
            false
        }
    }

    /// 获取所有运行中的进程
    pub async fn list_running(&self) -> Vec<String> {
        let processes = self.processes.read().await;
        processes.keys().cloned().collect()
    }

    /// 清理已结束的进程
    pub async fn cleanup(&self) {
        let mut processes = self.processes.write().await;
        processes.retain(|_, child| child.id().is_some());
    }
}

impl Default for ProcessManager {
    fn default() -> Self { Self::new() }
}
```

---

**交付物:** `terminal/process.rs`

---

### Task 4.3: 实现 CLI 检测服务

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/terminal/detector.rs`

---

**Step 4.3.1: 创建 detector.rs**

```rust
//! CLI 检测服务

use std::sync::Arc;
use tokio::process::Command;
use db::DBService;
use db::models::{CliType, CliDetectionStatus, cli_type_dao};

/// CLI 检测器
pub struct CliDetector {
    db: Arc<DBService>,
}

impl CliDetector {
    pub fn new(db: Arc<DBService>) -> Self {
        Self { db }
    }

    /// 检测所有 CLI
    pub async fn detect_all(&self) -> anyhow::Result<Vec<CliDetectionStatus>> {
        let cli_types = cli_type_dao::get_all_cli_types(&self.db.pool).await?;
        let mut results = Vec::new();

        for cli_type in cli_types {
            let status = self.detect_single(&cli_type).await;
            results.push(status);
        }

        Ok(results)
    }

    /// 检测单个 CLI
    pub async fn detect_single(&self, cli_type: &CliType) -> CliDetectionStatus {
        let parts: Vec<&str> = cli_type.detect_command.split_whitespace().collect();

        if parts.is_empty() {
            return self.not_installed(cli_type);
        }

        let cmd = parts[0];
        let args = &parts[1..];

        match Command::new(cmd).args(args).output().await {
            Ok(output) if output.status.success() => {
                let version = String::from_utf8_lossy(&output.stdout)
                    .lines()
                    .next()
                    .map(|s| s.trim().to_string());

                let executable_path = self.find_executable(cmd).await;

                CliDetectionStatus {
                    cli_type_id: cli_type.id.clone(),
                    name: cli_type.name.clone(),
                    display_name: cli_type.display_name.clone(),
                    installed: true,
                    version,
                    executable_path,
                    install_guide_url: cli_type.install_guide_url.clone(),
                }
            }
            _ => self.not_installed(cli_type),
        }
    }

    fn not_installed(&self, cli_type: &CliType) -> CliDetectionStatus {
        CliDetectionStatus {
            cli_type_id: cli_type.id.clone(),
            name: cli_type.name.clone(),
            display_name: cli_type.display_name.clone(),
            installed: false,
            version: None,
            executable_path: None,
            install_guide_url: cli_type.install_guide_url.clone(),
        }
    }

    async fn find_executable(&self, cmd: &str) -> Option<String> {
        #[cfg(unix)]
        {
            Command::new("which").arg(cmd).output().await.ok()
                .filter(|o| o.status.success())
                .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        }

        #[cfg(windows)]
        {
            Command::new("where").arg(cmd).output().await.ok()
                .filter(|o| o.status.success())
                .map(|o| String::from_utf8_lossy(&o.stdout).lines().next().unwrap_or("").to_string())
        }
    }

    /// 检测指定 CLI 是否可用
    pub async fn is_available(&self, cli_name: &str) -> bool {
        if let Ok(Some(cli_type)) = cli_type_dao::get_cli_type_by_name(&self.db.pool, cli_name).await {
            let status = self.detect_single(&cli_type).await;
            status.installed
        } else {
            false
        }
    }
}
```

---

**交付物:** `terminal/detector.rs`

**验收标准:**
1. 编译通过
2. CLI 检测功能正常工作

---

## Phase 4 完成检查清单

- [ ] Task 4.1: TerminalLauncher 实现完成
- [ ] Task 4.2: ProcessManager 实现完成
- [ ] Task 4.3: CliDetector 实现完成

---

## Phase 5: Git 事件驱动系统

### Task 5.1: 实现 GitWatcher

**状态:** ⬜ 未开始

**前置条件:**
- Phase 4 已完成

**目标:**
实现 Git 仓库监控，监听 .git/refs/heads 目录变化，检测新提交。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/mod.rs`
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/watcher.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/mod.rs`

---

**Step 5.1.1: 创建 git_watcher/mod.rs**

```rust
//! Git 事件监控模块

pub mod watcher;
pub mod parser;

pub use watcher::GitWatcher;
pub use parser::CommitParser;
```

---

**Step 5.1.2: 创建 watcher.rs**

```rust
//! Git 仓库监控

use std::path::PathBuf;
use std::sync::Arc;
use tokio::sync::mpsc;
use notify::{Watcher, RecursiveMode, Event, EventKind};
use crate::services::orchestrator::{MessageBus, BusMessage};

/// Git 事件
#[derive(Debug, Clone)]
pub struct GitEvent {
    pub commit_hash: String,
    pub branch: String,
    pub message: String,
    pub author: String,
    pub timestamp: String,
}

/// Git 监控器
pub struct GitWatcher {
    repo_path: PathBuf,
    message_bus: Arc<MessageBus>,
    workflow_id: String,
}

impl GitWatcher {
    pub fn new(repo_path: PathBuf, message_bus: Arc<MessageBus>, workflow_id: String) -> Self {
        Self { repo_path, message_bus, workflow_id }
    }

    /// 启动监控
    pub async fn start(&self) -> anyhow::Result<()> {
        let refs_path = self.repo_path.join(".git/refs/heads");

        if !refs_path.exists() {
            return Err(anyhow::anyhow!("Not a git repository: {}", self.repo_path.display()));
        }

        let (tx, mut rx) = mpsc::channel(100);
        let message_bus = self.message_bus.clone();
        let workflow_id = self.workflow_id.clone();
        let repo_path = self.repo_path.clone();

        // 启动文件监控
        let mut watcher = notify::recommended_watcher(move |res: Result<Event, _>| {
            if let Ok(event) = res {
                if matches!(event.kind, EventKind::Create(_) | EventKind::Modify(_)) {
                    let _ = tx.blocking_send(event);
                }
            }
        })?;

        watcher.watch(&refs_path, RecursiveMode::Recursive)?;

        tracing::info!("Git watcher started for {}", refs_path.display());

        // 事件处理循环
        tokio::spawn(async move {
            while let Some(event) = rx.recv().await {
                for path in event.paths {
                    if let Some(branch) = Self::extract_branch_name(&path) {
                        if let Ok(git_event) = Self::get_latest_commit(&repo_path, &branch).await {
                            message_bus.publish_git_event(
                                &workflow_id,
                                &git_event.commit_hash,
                                &git_event.branch,
                                &git_event.message,
                            ).await;
                        }
                    }
                }
            }
        });

        // 保持 watcher 存活
        std::mem::forget(watcher);

        Ok(())
    }

    /// 从路径提取分支名
    fn extract_branch_name(path: &std::path::Path) -> Option<String> {
        path.file_name()
            .and_then(|n| n.to_str())
            .map(|s| s.to_string())
    }

    /// 获取最新提交信息
    async fn get_latest_commit(repo_path: &PathBuf, branch: &str) -> anyhow::Result<GitEvent> {
        use tokio::process::Command;

        let output = Command::new("git")
            .current_dir(repo_path)
            .args(["log", "-1", "--format=%H|%s|%an|%aI", branch])
            .output()
            .await?;

        if !output.status.success() {
            return Err(anyhow::anyhow!("git log failed"));
        }

        let line = String::from_utf8_lossy(&output.stdout);
        let parts: Vec<&str> = line.trim().split('|').collect();

        if parts.len() >= 4 {
            Ok(GitEvent {
                commit_hash: parts[0].to_string(),
                message: parts[1].to_string(),
                author: parts[2].to_string(),
                timestamp: parts[3].to_string(),
                branch: branch.to_string(),
            })
        } else {
            Err(anyhow::anyhow!("Invalid git log output"))
        }
    }
}
```

---

**交付物:** `git_watcher/mod.rs`, `git_watcher/watcher.rs`

---

### Task 5.2: 实现提交信息解析器

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/parser.rs`

---

**Step 5.2.1: 创建 parser.rs**

```rust
//! Git 提交信息解析器
//!
//! 解析强制 Git 提交规范中的 METADATA 部分。

use serde::{Deserialize, Serialize};
use crate::services::orchestrator::types::CommitMetadata;

/// 提交解析器
pub struct CommitParser;

impl CommitParser {
    /// 解析提交信息
    ///
    /// 格式:
    /// ```text
    /// [Terminal:{id}] [Status:{status}] {summary}
    ///
    /// {body}
    ///
    /// ---METADATA---
    /// workflow_id: xxx
    /// task_id: xxx
    /// terminal_id: xxx
    /// ...
    /// ```
    pub fn parse(message: &str) -> Option<ParsedCommit> {
        let lines: Vec<&str> = message.lines().collect();

        if lines.is_empty() {
            return None;
        }

        // 解析标题行
        let title = lines[0];
        let (terminal_id, status, summary) = Self::parse_title(title)?;

        // 查找 METADATA 部分
        let metadata_start = lines.iter().position(|l| l.trim() == "---METADATA---")?;
        let metadata_lines = &lines[metadata_start + 1..];

        // 解析元数据
        let metadata = Self::parse_metadata(metadata_lines)?;

        Some(ParsedCommit {
            terminal_id,
            status,
            summary: summary.to_string(),
            metadata,
        })
    }

    /// 解析标题行
    fn parse_title(title: &str) -> Option<(String, String, &str)> {
        // [Terminal:xxx] [Status:xxx] summary
        let re = regex::Regex::new(r"\[Terminal:([^\]]+)\]\s*\[Status:([^\]]+)\]\s*(.+)").ok()?;
        let caps = re.captures(title)?;

        Some((
            caps.get(1)?.as_str().to_string(),
            caps.get(2)?.as_str().to_string(),
            caps.get(3)?.as_str(),
        ))
    }

    /// 解析元数据
    fn parse_metadata(lines: &[&str]) -> Option<CommitMetadata> {
        let mut map = std::collections::HashMap::new();

        for line in lines {
            if let Some(pos) = line.find(':') {
                let key = line[..pos].trim().to_string();
                let value = line[pos + 1..].trim().to_string();
                map.insert(key, value);
            }
        }

        Some(CommitMetadata {
            workflow_id: map.get("workflow_id")?.clone(),
            task_id: map.get("task_id")?.clone(),
            terminal_id: map.get("terminal_id")?.clone(),
            terminal_order: map.get("terminal_order").and_then(|s| s.parse().ok()).unwrap_or(0),
            cli: map.get("cli").cloned().unwrap_or_default(),
            model: map.get("model").cloned().unwrap_or_default(),
            status: map.get("status").cloned().unwrap_or_default(),
            severity: map.get("severity").cloned(),
            reviewed_terminal: map.get("reviewed_terminal").cloned(),
            issues: None,
            next_action: map.get("next_action").cloned().unwrap_or_else(|| "continue".to_string()),
        })
    }

    /// 生成提交信息
    pub fn generate(
        terminal_id: &str,
        status: &str,
        summary: &str,
        metadata: &CommitMetadata,
    ) -> String {
        format!(
            "[Terminal:{}] [Status:{}] {}\n\n---METADATA---\nworkflow_id: {}\ntask_id: {}\nterminal_id: {}\nterminal_order: {}\ncli: {}\nmodel: {}\nstatus: {}\nnext_action: {}",
            terminal_id, status, summary,
            metadata.workflow_id, metadata.task_id, metadata.terminal_id,
            metadata.terminal_order, metadata.cli, metadata.model,
            metadata.status, metadata.next_action
        )
    }
}

/// 解析后的提交
#[derive(Debug, Clone)]
pub struct ParsedCommit {
    pub terminal_id: String,
    pub status: String,
    pub summary: String,
    pub metadata: CommitMetadata,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_commit() {
        let message = r#"[Terminal:t1] [Status:completed] Implement login feature

Added user authentication

---METADATA---
workflow_id: wf1
task_id: task1
terminal_id: t1
terminal_order: 0
cli: claude-code
model: sonnet
status: completed
next_action: continue"#;

        let parsed = CommitParser::parse(message).unwrap();
        assert_eq!(parsed.terminal_id, "t1");
        assert_eq!(parsed.status, "completed");
        assert_eq!(parsed.metadata.workflow_id, "wf1");
    }
}
```

---

**交付物:** `git_watcher/parser.rs`

---

### Task 5.3: 连接 Git 事件到 Orchestrator

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/handler.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/git_watcher/mod.rs`

---

**Step 5.3.1: 创建 handler.rs**

```rust
//! Git 事件处理器

use std::sync::Arc;
use db::DBService;
use db::models::{terminal_dao, git_event_dao};
use crate::services::orchestrator::{MessageBus, BusMessage, TerminalCompletionEvent, TerminalCompletionStatus};
use super::parser::{CommitParser, ParsedCommit};
use super::watcher::GitEvent;

/// Git 事件处理器
pub struct GitEventHandler {
    db: Arc<DBService>,
    message_bus: Arc<MessageBus>,
}

impl GitEventHandler {
    pub fn new(db: Arc<DBService>, message_bus: Arc<MessageBus>) -> Self {
        Self { db, message_bus }
    }

    /// 处理 Git 事件
    pub async fn handle(&self, event: GitEvent) -> anyhow::Result<()> {
        tracing::info!("Handling git event: {} on {}", event.commit_hash, event.branch);

        // 解析提交信息
        let parsed = match CommitParser::parse(&event.message) {
            Some(p) => p,
            None => {
                tracing::debug!("Commit message not in expected format, skipping");
                return Ok(());
            }
        };

        // 保存到数据库
        let event_id = terminal_dao::create_git_event(
            &self.db.pool,
            &parsed.metadata.workflow_id,
            Some(&parsed.terminal_id),
            &event.commit_hash,
            &event.branch,
            &event.message,
            Some(&serde_json::to_string(&parsed.metadata)?),
        ).await?;

        // 更新终端状态
        terminal_dao::update_terminal_last_commit(
            &self.db.pool,
            &parsed.terminal_id,
            &event.commit_hash,
            &event.message,
        ).await?;

        // 转换为终端完成事件
        let completion_status = match parsed.status.as_str() {
            "completed" => TerminalCompletionStatus::Completed,
            "review_pass" => TerminalCompletionStatus::ReviewPass,
            "review_reject" => TerminalCompletionStatus::ReviewReject,
            "failed" => TerminalCompletionStatus::Failed,
            _ => TerminalCompletionStatus::Completed,
        };

        let completion_event = TerminalCompletionEvent {
            terminal_id: parsed.terminal_id.clone(),
            task_id: parsed.metadata.task_id.clone(),
            workflow_id: parsed.metadata.workflow_id.clone(),
            status: completion_status,
            commit_hash: Some(event.commit_hash),
            commit_message: Some(parsed.summary),
            metadata: Some(parsed.metadata),
        };

        // 发布到消息总线
        self.message_bus.publish_terminal_completed(completion_event).await;

        // 更新事件处理状态
        terminal_dao::update_git_event_status(&self.db.pool, &event_id, "processed", None).await?;

        Ok(())
    }
}
```

---

**Step 5.3.2: 更新 git_watcher/mod.rs**

```rust
pub mod watcher;
pub mod parser;
pub mod handler;

pub use watcher::{GitWatcher, GitEvent};
pub use parser::{CommitParser, ParsedCommit};
pub use handler::GitEventHandler;
```

---

**交付物:** `git_watcher/handler.rs`

**验收标准:**
1. 编译通过
2. Git 事件可以正确解析并转发到 Orchestrator

---

### Phase 5 单元测试用例

> 在 `vibe-kanban-main/crates/services/src/services/git_watcher/tests.rs` 创建以下测试

```rust
//! Git Watcher 单元测试
//!
//! 测试 Git 监听、提交解析、事件处理等功能

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;
    use tokio::sync::mpsc;

    // =========================================================================
    // 测试 1: 解析标准提交消息
    // =========================================================================
    #[test]
    fn test_parse_standard_commit() {
        let commit_msg = "feat: implement user login\n\nAdded JWT authentication";
        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.commit_type, Some("feat".to_string()));
        assert_eq!(result.subject, "implement user login");
        assert!(result.body.is_some());
    }

    // =========================================================================
    // 测试 2: 解析带状态标记的提交消息
    // =========================================================================
    #[test]
    fn test_parse_commit_with_status() {
        let commit_msg = r#"fix: resolve database connection issue

Status: COMPLETED
Terminal: T1
NextAction: CONTINUE

Fixed connection pooling timeout."#;

        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.status, Some(TaskStatus::Completed));
        assert_eq!(result.terminal_id, Some("T1".to_string()));
        assert_eq!(result.next_action, Some(NextAction::Continue));
    }

    // =========================================================================
    // 测试 3: 解析带帮助请求的提交消息
    // =========================================================================
    #[test]
    fn test_parse_commit_with_help_request() {
        let commit_msg = r#"wip: stuck on API integration

Status: NEED_HELP
Terminal: T2
HelpType: TECHNICAL
HelpContext: Cannot figure out how to handle rate limiting

Tried exponential backoff but still hitting limits."#;

        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.status, Some(TaskStatus::NeedHelp));
        assert_eq!(result.help_type, Some(HelpType::Technical));
        assert!(result.help_context.is_some());
    }

    // =========================================================================
    // 测试 4: Git refs 文件变更检测
    // =========================================================================
    #[tokio::test]
    async fn test_detect_ref_change() {
        let temp_dir = TempDir::new().unwrap();
        let git_dir = temp_dir.path().join(".git");
        let refs_dir = git_dir.join("refs/heads");
        fs::create_dir_all(&refs_dir).unwrap();

        // 创建初始 ref
        let main_ref = refs_dir.join("main");
        fs::write(&main_ref, "abc123").unwrap();

        let (tx, mut rx) = mpsc::channel(10);
        let watcher = GitWatcher::new(temp_dir.path().to_path_buf(), tx);

        // 启动监听（后台）
        let watcher_handle = tokio::spawn(async move {
            watcher.start().await
        });

        // 模拟新提交
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
        fs::write(&main_ref, "def456").unwrap();

        // 等待事件
        let event = tokio::time::timeout(
            tokio::time::Duration::from_secs(2),
            rx.recv()
        ).await;

        assert!(event.is_ok());
        let event = event.unwrap().unwrap();
        assert_eq!(event.branch, "main");
        assert_eq!(event.new_commit, "def456");

        watcher_handle.abort();
    }

    // =========================================================================
    // 测试 5: 事件处理器路由
    // =========================================================================
    #[tokio::test]
    async fn test_event_handler_routing() {
        let (orchestrator_tx, mut orchestrator_rx) = mpsc::channel(10);
        let handler = GitEventHandler::new(orchestrator_tx);

        let event = GitEvent {
            branch: "feature/login".to_string(),
            old_commit: Some("abc123".to_string()),
            new_commit: "def456".to_string(),
            timestamp: chrono::Utc::now(),
        };

        handler.handle(event).await.unwrap();

        // 验证消息被发送到 Orchestrator
        let msg = orchestrator_rx.recv().await.unwrap();
        assert!(matches!(msg, OrchestratorMessage::GitCommitDetected { .. }));
    }

    // =========================================================================
    // 测试 6: 多分支并发监听
    // =========================================================================
    #[tokio::test]
    async fn test_multi_branch_watch() {
        let temp_dir = TempDir::new().unwrap();
        let git_dir = temp_dir.path().join(".git");
        let refs_dir = git_dir.join("refs/heads");
        fs::create_dir_all(&refs_dir).unwrap();

        // 创建多个分支 refs
        fs::write(refs_dir.join("main"), "commit1").unwrap();
        fs::write(refs_dir.join("feature-a"), "commit2").unwrap();
        fs::write(refs_dir.join("feature-b"), "commit3").unwrap();

        let (tx, mut rx) = mpsc::channel(10);
        let watcher = GitWatcher::new(temp_dir.path().to_path_buf(), tx);

        // 验证所有分支都被监听
        let branches = watcher.get_watched_branches();
        assert!(branches.contains(&"main".to_string()));
        assert!(branches.contains(&"feature-a".to_string()));
        assert!(branches.contains(&"feature-b".to_string()));
    }

    // =========================================================================
    // 测试 7: 解析无状态标记的普通提交
    // =========================================================================
    #[test]
    fn test_parse_plain_commit() {
        let commit_msg = "docs: update README with installation instructions";
        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.commit_type, Some("docs".to_string()));
        assert_eq!(result.subject, "update README with installation instructions");
        assert!(result.status.is_none());
        assert!(result.terminal_id.is_none());
    }

    // =========================================================================
    // 测试 8: 事件去重（防止重复触发）
    // =========================================================================
    #[tokio::test]
    async fn test_event_deduplication() {
        let (tx, mut rx) = mpsc::channel(10);
        let handler = GitEventHandler::new(tx);

        let event = GitEvent {
            branch: "main".to_string(),
            old_commit: Some("abc".to_string()),
            new_commit: "def".to_string(),
            timestamp: chrono::Utc::now(),
        };

        // 发送相同事件两次
        handler.handle(event.clone()).await.unwrap();
        handler.handle(event.clone()).await.unwrap();

        // 应该只收到一个事件（去重）
        let first = rx.recv().await;
        assert!(first.is_some());

        // 第二个应该被过滤
        let second = tokio::time::timeout(
            tokio::time::Duration::from_millis(100),
            rx.recv()
        ).await;
        assert!(second.is_err()); // 超时，说明没有第二个事件
    }
}
```

**运行测试:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p services git_watcher -- --nocapture
```

---

## Phase 5 完成检查清单

- [ ] Task 5.1: GitWatcher 实现完成
- [ ] Task 5.2: CommitParser 实现完成
- [ ] Task 5.3: GitEventHandler 实现完成

---

## Phase 6: 前端界面改造（7 步向导）

> **重要:** 此阶段实现与设计文档 `2026-01-16-orchestrator-design.md` 第 11 章完全一致的 7 步向导。

### 向导步骤概览

| 步骤 | 名称 | 描述 |
|------|------|------|
| Step 0 | 工作目录 | 选择项目文件夹，检测/初始化 Git |
| Step 1 | 基础配置 | 工作流名称、任务数量 |
| Step 2 | 任务配置 | 每个任务的名称、描述、终端数量 |
| Step 3 | 模型配置 | 配置 API Key、Base URL、获取可用模型 |
| Step 4 | 终端配置 | 为每个任务的终端选择 CLI 和模型 |
| Step 5 | 斜杠命令 | 可选，配置执行命令顺序 |
| Step 6 | 高级配置 | 主 Agent、错误处理终端、合并终端、Git 规范 |

---

### Task 6.1: 创建向导框架和类型定义

**状态:** ⬜ 未开始

**前置条件:**
- Phase 5 已完成
- 熟悉 vibe-kanban 前端结构（参考 `frontend/CLAUDE.md`）
- 了解设计文档中的 UI 模型图

**目标:**
创建 7 步向导的框架组件和 TypeScript 类型定义。

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/components/workflow/types.ts`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/WorkflowWizard.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/StepIndicator.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/index.ts`

---

**Step 6.1.1: 创建 types.ts 类型定义**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/types.ts`

```typescript
// ============================================================================
// 工作流向导类型定义
// 对应设计文档 2026-01-16-orchestrator-design.md 第 11 章
// ============================================================================

/** 向导步骤枚举 */
export enum WizardStep {
  Project = 0,      // 步骤0: 工作目录
  Basic = 1,        // 步骤1: 基础配置
  Tasks = 2,        // 步骤2: 任务配置
  Models = 3,       // 步骤3: 模型配置
  Terminals = 4,    // 步骤4: 终端配置
  Commands = 5,     // 步骤5: 斜杠命令
  Advanced = 6,     // 步骤6: 高级配置
}

/** 向导步骤元数据 */
export const WIZARD_STEPS = [
  { step: WizardStep.Project, name: '工作目录', description: '选择项目文件夹' },
  { step: WizardStep.Basic, name: '基础配置', description: '工作流名称和任务数量' },
  { step: WizardStep.Tasks, name: '任务配置', description: '配置每个任务详情' },
  { step: WizardStep.Models, name: '模型配置', description: '配置 API 和可用模型' },
  { step: WizardStep.Terminals, name: '终端配置', description: '为任务分配终端' },
  { step: WizardStep.Commands, name: '斜杠命令', description: '配置执行命令' },
  { step: WizardStep.Advanced, name: '高级配置', description: '主 Agent 和合并配置' },
] as const;

/** Git 仓库状态 */
export interface GitStatus {
  isGitRepo: boolean;
  currentBranch?: string;
  remoteUrl?: string;
  isDirty: boolean;
  uncommittedChanges?: number;
}

/** 项目配置 (步骤0) */
export interface ProjectConfig {
  workingDirectory: string;
  gitStatus: GitStatus;
}

/** 基础配置 (步骤1) */
export interface BasicConfig {
  name: string;
  description?: string;
  taskCount: number;
  importFromKanban: boolean;
  kanbanTaskIds?: string[];
}

/** 任务配置 (步骤2) */
export interface TaskConfig {
  id: string;           // 临时 ID，用于前端标识
  name: string;
  description: string;  // AI 将根据此描述执行任务
  branch: string;       // Git 分支名
  terminalCount: number; // 此任务的串行终端数量
}

/** API 类型 */
export type ApiType = 'anthropic' | 'google' | 'openai' | 'openai-compatible';

/** 模型配置 (步骤3) */
export interface ModelConfig {
  id: string;           // 临时 ID
  displayName: string;  // 用户自定义显示名
  apiType: ApiType;
  baseUrl: string;
  apiKey: string;
  modelId: string;      // 实际模型 ID
  isVerified: boolean;  // 是否已验证连接
}

/** 终端配置 (步骤4) */
export interface TerminalConfig {
  id: string;           // 临时 ID
  taskId: string;       // 关联的任务 ID
  orderIndex: number;   // 在任务内的执行顺序
  cliTypeId: string;    // CLI 类型 (claude-code, gemini-cli, codex)
  modelConfigId: string; // 关联的模型配置 ID
  role?: string;        // 角色描述
}

/** 斜杠命令配置 (步骤5) */
export interface CommandConfig {
  enabled: boolean;
  presetIds: string[];  // 选中的命令预设 ID（按顺序）
}

/** 高级配置 (步骤6) */
export interface AdvancedConfig {
  orchestrator: {
    modelConfigId: string; // 主 Agent 使用的模型
  };
  errorTerminal: {
    enabled: boolean;
    cliTypeId?: string;
    modelConfigId?: string;
  };
  mergeTerminal: {
    cliTypeId: string;
    modelConfigId: string;
    runTestsBeforeMerge: boolean;
    pauseOnConflict: boolean;
  };
  targetBranch: string;
}

/** 完整的向导配置 */
export interface WizardConfig {
  project: ProjectConfig;
  basic: BasicConfig;
  tasks: TaskConfig[];
  models: ModelConfig[];
  terminals: TerminalConfig[];
  commands: CommandConfig;
  advanced: AdvancedConfig;
}

/** 向导状态 */
export interface WizardState {
  currentStep: WizardStep;
  config: WizardConfig;
  isSubmitting: boolean;
  errors: Record<string, string>;
}

/** 获取默认向导配置 */
export function getDefaultWizardConfig(): WizardConfig {
  return {
    project: {
      workingDirectory: '',
      gitStatus: { isGitRepo: false, isDirty: false },
    },
    basic: {
      name: '',
      taskCount: 1,
      importFromKanban: false,
    },
    tasks: [],
    models: [],
    terminals: [],
    commands: {
      enabled: false,
      presetIds: [],
    },
    advanced: {
      orchestrator: { modelConfigId: '' },
      errorTerminal: { enabled: false },
      mergeTerminal: {
        cliTypeId: '',
        modelConfigId: '',
        runTestsBeforeMerge: true,
        pauseOnConflict: true,
      },
      targetBranch: 'main',
    },
  };
}
```

---

**Step 6.1.2: 创建 StepIndicator.tsx**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/StepIndicator.tsx`

```tsx
import { cn } from '@/lib/utils';
import { WizardStep, WIZARD_STEPS } from './types';
import { Check } from 'lucide-react';

interface Props {
  currentStep: WizardStep;
  completedSteps: WizardStep[];
}

export function StepIndicator({ currentStep, completedSteps }: Props) {
  return (
    <div className="flex items-center justify-between w-full mb-8">
      {WIZARD_STEPS.map((stepInfo, index) => {
        const isCompleted = completedSteps.includes(stepInfo.step);
        const isCurrent = currentStep === stepInfo.step;
        const isPast = stepInfo.step < currentStep;

        return (
          <div key={stepInfo.step} className="flex items-center flex-1">
            {/* Step Circle */}
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  'w-10 h-10 rounded-full flex items-center justify-center text-sm font-medium border-2 transition-colors',
                  isCompleted && 'bg-brand border-brand text-white',
                  isCurrent && !isCompleted && 'border-brand text-brand bg-brand/10',
                  !isCurrent && !isCompleted && 'border-muted text-low bg-secondary'
                )}
              >
                {isCompleted ? <Check className="w-5 h-5" /> : index}
              </div>
              <span
                className={cn(
                  'text-xs mt-2 text-center max-w-[80px]',
                  isCurrent ? 'text-normal font-medium' : 'text-low'
                )}
              >
                {stepInfo.name}
              </span>
            </div>

            {/* Connector Line */}
            {index < WIZARD_STEPS.length - 1 && (
              <div
                className={cn(
                  'flex-1 h-0.5 mx-2',
                  isPast || isCompleted ? 'bg-brand' : 'bg-muted'
                )}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
```

---

**Step 6.1.3: 创建 WorkflowWizard.tsx 主组件**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/WorkflowWizard.tsx`

```tsx
import { useState, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { X } from 'lucide-react';
import { StepIndicator } from './StepIndicator';
import {
  WizardStep,
  WizardConfig,
  WizardState,
  WIZARD_STEPS,
  getDefaultWizardConfig,
} from './types';

// 步骤组件导入
import { Step0Project } from './steps/Step0Project';
import { Step1Basic } from './steps/Step1Basic';
import { Step2Tasks } from './steps/Step2Tasks';
import { Step3Models } from './steps/Step3Models';
import { Step4Terminals } from './steps/Step4Terminals';
import { Step5Commands } from './steps/Step5Commands';
import { Step6Advanced } from './steps/Step6Advanced';

interface Props {
  onComplete: (config: WizardConfig) => Promise<void>;
  onCancel: () => void;
}

export function WorkflowWizard({ onComplete, onCancel }: Props) {
  const [state, setState] = useState<WizardState>({
    currentStep: WizardStep.Project,
    config: getDefaultWizardConfig(),
    isSubmitting: false,
    errors: {},
  });

  const [completedSteps, setCompletedSteps] = useState<WizardStep[]>([]);

  // 更新配置
  const updateConfig = useCallback(<K extends keyof WizardConfig>(
    key: K,
    value: WizardConfig[K]
  ) => {
    setState(prev => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }));
  }, []);

  // 验证当前步骤
  const validateCurrentStep = (): boolean => {
    const { currentStep, config } = state;
    const errors: Record<string, string> = {};

    switch (currentStep) {
      case WizardStep.Project:
        if (!config.project.workingDirectory) {
          errors.workingDirectory = '请选择工作目录';
        }
        break;
      case WizardStep.Basic:
        if (!config.basic.name.trim()) {
          errors.name = '请输入工作流名称';
        }
        if (config.basic.taskCount < 1) {
          errors.taskCount = '至少需要一个任务';
        }
        break;
      case WizardStep.Tasks:
        if (config.tasks.some(t => !t.name.trim() || !t.description.trim())) {
          errors.tasks = '请完成所有任务的配置';
        }
        break;
      case WizardStep.Models:
        if (config.models.length === 0) {
          errors.models = '至少需要配置一个模型';
        }
        break;
      case WizardStep.Terminals:
        if (config.terminals.some(t => !t.cliTypeId || !t.modelConfigId)) {
          errors.terminals = '请完成所有终端的配置';
        }
        break;
      case WizardStep.Advanced:
        if (!config.advanced.orchestrator.modelConfigId) {
          errors.orchestrator = '请选择主 Agent 模型';
        }
        if (!config.advanced.mergeTerminal.cliTypeId) {
          errors.mergeTerminal = '请配置合并终端';
        }
        break;
    }

    setState(prev => ({ ...prev, errors }));
    return Object.keys(errors).length === 0;
  };

  // 下一步
  const handleNext = () => {
    if (!validateCurrentStep()) return;

    setCompletedSteps(prev => [...prev, state.currentStep]);
    setState(prev => ({
      ...prev,
      currentStep: prev.currentStep + 1,
    }));
  };

  // 上一步
  const handleBack = () => {
    if (state.currentStep > 0) {
      setState(prev => ({
        ...prev,
        currentStep: prev.currentStep - 1,
      }));
    }
  };

  // 提交
  const handleSubmit = async () => {
    if (!validateCurrentStep()) return;

    setState(prev => ({ ...prev, isSubmitting: true }));
    try {
      await onComplete(state.config);
    } catch (error) {
      console.error('Failed to create workflow:', error);
      setState(prev => ({
        ...prev,
        errors: { submit: '创建工作流失败，请重试' },
      }));
    } finally {
      setState(prev => ({ ...prev, isSubmitting: false }));
    }
  };

  // 渲染当前步骤
  const renderStep = () => {
    const { currentStep, config, errors } = state;

    switch (currentStep) {
      case WizardStep.Project:
        return (
          <Step0Project
            config={config.project}
            onChange={value => updateConfig('project', value)}
            errors={errors}
          />
        );
      case WizardStep.Basic:
        return (
          <Step1Basic
            config={config.basic}
            onChange={value => updateConfig('basic', value)}
            errors={errors}
          />
        );
      case WizardStep.Tasks:
        return (
          <Step2Tasks
            config={config.tasks}
            taskCount={config.basic.taskCount}
            onChange={value => updateConfig('tasks', value)}
            errors={errors}
          />
        );
      case WizardStep.Models:
        return (
          <Step3Models
            config={config.models}
            onChange={value => updateConfig('models', value)}
            errors={errors}
          />
        );
      case WizardStep.Terminals:
        return (
          <Step4Terminals
            config={config.terminals}
            tasks={config.tasks}
            models={config.models}
            onChange={value => updateConfig('terminals', value)}
            errors={errors}
          />
        );
      case WizardStep.Commands:
        return (
          <Step5Commands
            config={config.commands}
            onChange={value => updateConfig('commands', value)}
            errors={errors}
          />
        );
      case WizardStep.Advanced:
        return (
          <Step6Advanced
            config={config.advanced}
            models={config.models}
            onChange={value => updateConfig('advanced', value)}
            errors={errors}
          />
        );
    }
  };

  const currentStepInfo = WIZARD_STEPS[state.currentStep];
  const isLastStep = state.currentStep === WizardStep.Advanced;
  const isFirstStep = state.currentStep === WizardStep.Project;

  return (
    <Card className="w-full max-w-4xl mx-auto bg-panel">
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="text-xl text-high">创建工作流</CardTitle>
          <p className="text-sm text-low mt-1">{currentStepInfo.description}</p>
        </div>
        <Button variant="ghost" size="icon" onClick={onCancel}>
          <X className="w-5 h-5" />
        </Button>
      </CardHeader>

      <CardContent>
        <StepIndicator
          currentStep={state.currentStep}
          completedSteps={completedSteps}
        />

        <div className="min-h-[400px]">
          {renderStep()}
        </div>

        {state.errors.submit && (
          <p className="text-error text-sm mt-4">{state.errors.submit}</p>
        )}

        <div className="flex justify-between mt-8 pt-4 border-t">
          <Button
            variant="outline"
            onClick={isFirstStep ? onCancel : handleBack}
            disabled={state.isSubmitting}
          >
            {isFirstStep ? '取消' : '上一步'}
          </Button>
          <Button
            onClick={isLastStep ? handleSubmit : handleNext}
            disabled={state.isSubmitting}
            className="bg-brand hover:bg-brand/90"
          >
            {state.isSubmitting
              ? '创建中...'
              : isLastStep
              ? '创建工作流'
              : '下一步'}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
```

---

**交付物:**
- `types.ts` - 完整类型定义
- `StepIndicator.tsx` - 步骤指示器
- `WorkflowWizard.tsx` - 主向导组件

**验收标准:**
1. TypeScript 编译通过
2. 向导框架可正常渲染

**测试命令:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main\frontend
pnpm run check
# 预期: 无类型错误
```

---

### Task 6.2: 步骤 0-1 组件（工作目录和基础配置）

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step0Project.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step1Basic.tsx`

---

**Step 6.2.1: 创建 Step0Project.tsx（工作目录选择）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step0Project.tsx`

```tsx
import { useState, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Folder, GitBranch, AlertTriangle, Check, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ProjectConfig, GitStatus } from '../types';

interface Props {
  config: ProjectConfig;
  onChange: (config: ProjectConfig) => void;
  errors: Record<string, string>;
}

export function Step0Project({ config, onChange, errors }: Props) {
  const [isChecking, setIsChecking] = useState(false);

  // 选择文件夹（通过 Tauri/Electron API）
  const handleSelectFolder = useCallback(async () => {
    try {
      // @ts-ignore - window.__TAURI__ 在 Tauri 环境中可用
      const selected = await window.__TAURI__?.dialog?.open({
        directory: true,
        multiple: false,
        title: '选择项目工作目录',
      });

      if (selected && typeof selected === 'string') {
        setIsChecking(true);
        // 检测 Git 状态
        const gitStatus = await checkGitStatus(selected);
        onChange({
          workingDirectory: selected,
          gitStatus,
        });
        setIsChecking(false);
      }
    } catch (error) {
      console.error('Failed to select folder:', error);
      setIsChecking(false);
    }
  }, [onChange]);

  // 检测 Git 状态
  const checkGitStatus = async (path: string): Promise<GitStatus> => {
    try {
      const response = await fetch('/api/git/status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      });
      return await response.json();
    } catch {
      return { isGitRepo: false, isDirty: false };
    }
  };

  // 初始化 Git 仓库
  const handleInitGit = async () => {
    if (!config.workingDirectory) return;

    setIsChecking(true);
    try {
      await fetch('/api/git/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: config.workingDirectory }),
      });
      const gitStatus = await checkGitStatus(config.workingDirectory);
      onChange({ ...config, gitStatus });
    } catch (error) {
      console.error('Failed to init git:', error);
    }
    setIsChecking(false);
  };

  return (
    <div className="space-y-6">
      {/* 文件夹选择 */}
      <div className="space-y-2">
        <Label>选择项目工作目录</Label>
        <div className="flex gap-2">
          <Input
            value={config.workingDirectory}
            placeholder="点击浏览选择文件夹..."
            readOnly
            className="flex-1 bg-secondary"
          />
          <Button variant="outline" onClick={handleSelectFolder} disabled={isChecking}>
            <Folder className="w-4 h-4 mr-2" />
            浏览...
          </Button>
        </div>
        {errors.workingDirectory && (
          <p className="text-error text-sm">{errors.workingDirectory}</p>
        )}
      </div>

      {/* Git 状态检测 */}
      {config.workingDirectory && (
        <div className="border rounded-lg p-4 bg-secondary">
          <div className="flex items-center gap-2 mb-3">
            <GitBranch className="w-5 h-5" />
            <span className="font-medium">Git 状态检测</span>
            {isChecking && <RefreshCw className="w-4 h-4 animate-spin" />}
          </div>

          {config.gitStatus.isGitRepo ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-success">
                <Check className="w-4 h-4" />
                <span>检测到 Git 仓库</span>
              </div>
              <div className="text-sm text-low space-y-1 pl-6">
                <p>当前分支: <span className="text-normal">{config.gitStatus.currentBranch}</span></p>
                {config.gitStatus.remoteUrl && (
                  <p>远程仓库: <span className="text-normal">{config.gitStatus.remoteUrl}</span></p>
                )}
                <p>
                  工作区状态:{' '}
                  <span className={cn(config.gitStatus.isDirty ? 'text-warning' : 'text-success')}>
                    {config.gitStatus.isDirty
                      ? `有 ${config.gitStatus.uncommittedChanges || '未知'} 个未提交更改`
                      : '干净 (无未提交更改)'}
                  </span>
                </p>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-warning">
                <AlertTriangle className="w-4 h-4" />
                <span>未检测到 Git 仓库</span>
              </div>
              <p className="text-sm text-low pl-6">
                此文件夹不是 Git 仓库。SoloDawn 需要 Git 来协调多终端工作流。
              </p>
              <div className="flex gap-2 pl-6">
                <Button onClick={handleInitGit} disabled={isChecking} size="sm">
                  初始化 Git 仓库
                </Button>
                <Button variant="outline" onClick={handleSelectFolder} size="sm">
                  选择其他文件夹
                </Button>
              </div>
              <p className="text-xs text-low pl-6">
                初始化将执行: git init → 创建 .gitignore → git add . && git commit
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
```

---

**Step 6.2.2: 创建 Step1Basic.tsx（基础配置）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step1Basic.tsx`

```tsx
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { cn } from '@/lib/utils';
import type { BasicConfig } from '../types';

interface Props {
  config: BasicConfig;
  onChange: (config: BasicConfig) => void;
  errors: Record<string, string>;
}

const TASK_COUNT_OPTIONS = [1, 2, 3, 4];

export function Step1Basic({ config, onChange, errors }: Props) {
  return (
    <div className="space-y-6">
      {/* 工作流名称 */}
      <div className="space-y-2">
        <Label htmlFor="workflow-name">工作流名称 *</Label>
        <Input
          id="workflow-name"
          value={config.name}
          onChange={e => onChange({ ...config, name: e.target.value })}
          placeholder="例如：用户系统重构"
          className="bg-secondary"
        />
        {errors.name && <p className="text-error text-sm">{errors.name}</p>}
      </div>

      {/* 描述 */}
      <div className="space-y-2">
        <Label htmlFor="workflow-desc">描述（可选）</Label>
        <Textarea
          id="workflow-desc"
          value={config.description || ''}
          onChange={e => onChange({ ...config, description: e.target.value })}
          placeholder="工作流的整体目标和描述..."
          rows={3}
          className="bg-secondary"
        />
      </div>

      {/* 任务数量选择 */}
      <div className="space-y-3">
        <Label>本次启动几个并行任务？</Label>
        <div className="flex gap-3 flex-wrap">
          {TASK_COUNT_OPTIONS.map(count => (
            <button
              key={count}
              type="button"
              onClick={() => onChange({ ...config, taskCount: count })}
              className={cn(
                'px-4 py-2 rounded border text-sm font-medium transition-colors',
                config.taskCount === count
                  ? 'bg-brand border-brand text-white'
                  : 'bg-secondary border-muted text-normal hover:border-brand'
              )}
            >
              {count} 个任务
            </button>
          ))}
          <div className="flex items-center gap-2">
            <span className="text-low">更多:</span>
            <Input
              type="number"
              min={5}
              max={10}
              value={config.taskCount > 4 ? config.taskCount : ''}
              onChange={e => {
                const val = parseInt(e.target.value);
                if (val >= 1 && val <= 10) {
                  onChange({ ...config, taskCount: val });
                }
              }}
              className="w-16 bg-secondary"
              placeholder="5-10"
            />
          </div>
        </div>
        {errors.taskCount && <p className="text-error text-sm">{errors.taskCount}</p>}
      </div>

      {/* 导入选项 */}
      <div className="space-y-3">
        <Label>是否从看板导入已有任务？</Label>
        <RadioGroup
          value={config.importFromKanban ? 'import' : 'new'}
          onValueChange={val => onChange({ ...config, importFromKanban: val === 'import' })}
        >
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="new" id="task-new" />
            <Label htmlFor="task-new" className="font-normal cursor-pointer">
              新建任务（下一步手动配置）
            </Label>
          </div>
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="import" id="task-import" />
            <Label htmlFor="task-import" className="font-normal cursor-pointer">
              从看板导入（选择已有任务卡片）
            </Label>
          </div>
        </RadioGroup>
      </div>
    </div>
  );
}
```

---

**交付物:**
- `Step0Project.tsx` - 工作目录选择
- `Step1Basic.tsx` - 基础配置

**验收标准:**
1. 编译通过
2. 文件夹选择和 Git 状态检测正常

---

### Task 6.3: 步骤 2-3 组件（任务配置和模型配置）

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step2Tasks.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step3Models.tsx`

---

**Step 6.3.1: 创建 Step2Tasks.tsx（任务详细配置）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step2Tasks.tsx`

```tsx
import { useState, useEffect } from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { TaskConfig } from '../types';
import { v4 as uuid } from 'uuid';

interface Props {
  config: TaskConfig[];
  taskCount: number;
  onChange: (config: TaskConfig[]) => void;
  errors: Record<string, string>;
}

const TERMINAL_COUNT_OPTIONS = [1, 2, 3];

export function Step2Tasks({ config, taskCount, onChange, errors }: Props) {
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0);

  // 初始化任务列表
  useEffect(() => {
    if (config.length !== taskCount) {
      const newTasks: TaskConfig[] = [];
      for (let i = 0; i < taskCount; i++) {
        if (config[i]) {
          newTasks.push(config[i]);
        } else {
          newTasks.push({
            id: uuid(),
            name: '',
            description: '',
            branch: '',
            terminalCount: 1,
          });
        }
      }
      onChange(newTasks);
    }
  }, [taskCount, config.length]);

  const currentTask = config[currentTaskIndex];

  // 更新当前任务
  const updateTask = (updates: Partial<TaskConfig>) => {
    const newTasks = [...config];
    newTasks[currentTaskIndex] = { ...currentTask, ...updates };

    // 自动生成分支名
    if (updates.name && !currentTask.branch) {
      const slug = updates.name
        .toLowerCase()
        .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
        .replace(/^-|-$/g, '');
      newTasks[currentTaskIndex].branch = `feat/${slug}`;
    }

    onChange(newTasks);
  };

  if (!currentTask) return null;

  const isTaskComplete = currentTask.name.trim() && currentTask.description.trim();

  return (
    <div className="space-y-6">
      {/* 任务导航 */}
      <div className="flex items-center justify-between">
        <span className="text-lg font-medium">
          配置 {taskCount} 个并行任务
        </span>
        <div className="flex items-center gap-2">
          <span className="text-sm text-low">
            任务 {currentTaskIndex + 1}/{taskCount}
          </span>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setCurrentTaskIndex(i => Math.max(0, i - 1))}
            disabled={currentTaskIndex === 0}
          >
            <ChevronLeft className="w-4 h-4" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setCurrentTaskIndex(i => Math.min(taskCount - 1, i + 1))}
            disabled={currentTaskIndex === taskCount - 1}
          >
            <ChevronRight className="w-4 h-4" />
          </Button>
        </div>
      </div>

      {/* 任务配置表单 */}
      <div className="border rounded-lg p-6 bg-secondary/50">
        <div className="flex items-center gap-2 mb-4">
          <span className="text-sm font-medium text-low">任务 {currentTaskIndex + 1}</span>
          {isTaskComplete && (
            <span className="text-xs px-2 py-0.5 rounded bg-success/20 text-success">已配置</span>
          )}
        </div>

        <div className="space-y-4">
          {/* 任务名称 */}
          <div className="space-y-2">
            <Label>任务名称 *</Label>
            <Input
              value={currentTask.name}
              onChange={e => updateTask({ name: e.target.value })}
              placeholder="例如：登录功能"
              className="bg-secondary"
            />
          </div>

          {/* Git 分支名称 */}
          <div className="space-y-2">
            <Label>Git 分支名称</Label>
            <Input
              value={currentTask.branch}
              onChange={e => updateTask({ branch: e.target.value })}
              placeholder="自动生成，可修改"
              className="bg-secondary"
            />
            <p className="text-xs text-low">
              建议格式: feat/xxx, fix/xxx, refactor/xxx
            </p>
          </div>

          {/* 任务描述 */}
          <div className="space-y-2">
            <Label>任务描述 (AI 将根据此描述执行任务) *</Label>
            <Textarea
              value={currentTask.description}
              onChange={e => updateTask({ description: e.target.value })}
              placeholder={`实现${currentTask.name || '功能'}:\n1. 具体步骤一\n2. 具体步骤二\n3. 具体步骤三`}
              rows={8}
              className="bg-secondary font-mono text-sm"
            />
            <p className="text-xs text-low">支持 Markdown 格式，描述越详细，AI 执行越准确</p>
          </div>

          {/* 终端数量 */}
          <div className="space-y-2">
            <Label>此任务需要几个终端串行执行？</Label>
            <div className="flex gap-2">
              {TERMINAL_COUNT_OPTIONS.map(count => (
                <button
                  key={count}
                  type="button"
                  onClick={() => updateTask({ terminalCount: count })}
                  className={cn(
                    'px-4 py-2 rounded border text-sm',
                    currentTask.terminalCount === count
                      ? 'bg-brand border-brand text-white'
                      : 'bg-secondary border-muted hover:border-brand'
                  )}
                >
                  {count} 个
                </button>
              ))}
              <Input
                type="number"
                min={4}
                max={5}
                value={currentTask.terminalCount > 3 ? currentTask.terminalCount : ''}
                onChange={e => {
                  const val = parseInt(e.target.value);
                  if (val >= 1) updateTask({ terminalCount: val });
                }}
                placeholder="更多"
                className="w-20 bg-secondary"
              />
            </div>
          </div>
        </div>
      </div>

      {/* 进度指示 */}
      <div className="flex items-center gap-2">
        <span className="text-sm text-low">任务进度:</span>
        <div className="flex-1 flex gap-1">
          {config.map((task, i) => (
            <button
              key={task.id}
              onClick={() => setCurrentTaskIndex(i)}
              className={cn(
                'flex-1 h-2 rounded transition-colors',
                task.name && task.description ? 'bg-brand' : 'bg-muted',
                i === currentTaskIndex && 'ring-2 ring-brand ring-offset-1'
              )}
            />
          ))}
        </div>
        <span className="text-sm text-low">
          {config.filter(t => t.name && t.description).length} / {taskCount} 已配置
        </span>
      </div>

      {errors.tasks && <p className="text-error text-sm">{errors.tasks}</p>}
    </div>
  );
}
```

---

**Step 6.3.2: 创建 Step3Models.tsx（模型配置）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step3Models.tsx`

```tsx
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Plus, Pencil, Trash2, RefreshCw, Check, Eye, EyeOff } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ModelConfig, ApiType } from '../types';
import { v4 as uuid } from 'uuid';

interface Props {
  config: ModelConfig[];
  onChange: (config: ModelConfig[]) => void;
  errors: Record<string, string>;
}

const API_TYPES: { value: ApiType; label: string; defaultUrl: string }[] = [
  { value: 'anthropic', label: 'Anthropic (官方)', defaultUrl: 'https://api.anthropic.com' },
  { value: 'google', label: 'Google (Gemini)', defaultUrl: 'https://generativelanguage.googleapis.com' },
  { value: 'openai', label: 'OpenAI', defaultUrl: 'https://api.openai.com' },
  { value: 'openai-compatible', label: 'OpenAI 兼容', defaultUrl: '' },
];

export function Step3Models({ config, onChange, errors }: Props) {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<ModelConfig | null>(null);

  const handleAddModel = (model: ModelConfig) => {
    if (editingModel) {
      onChange(config.map(m => m.id === model.id ? model : m));
    } else {
      onChange([...config, model]);
    }
    setIsDialogOpen(false);
    setEditingModel(null);
  };

  const handleEdit = (model: ModelConfig) => {
    setEditingModel(model);
    setIsDialogOpen(true);
  };

  const handleDelete = (id: string) => {
    onChange(config.filter(m => m.id !== id));
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="font-medium">配置可用模型 (cc-switch)</h3>
          <p className="text-sm text-low">这些模型将在终端配置中供选择</p>
        </div>
        <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
          <DialogTrigger asChild>
            <Button onClick={() => setEditingModel(null)}>
              <Plus className="w-4 h-4 mr-2" />
              添加模型
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>{editingModel ? '编辑模型' : '添加模型'}</DialogTitle>
            </DialogHeader>
            <AddModelForm
              initialModel={editingModel}
              onSubmit={handleAddModel}
              onCancel={() => setIsDialogOpen(false)}
            />
          </DialogContent>
        </Dialog>
      </div>

      {/* 已配置的模型列表 */}
      <div className="space-y-3">
        {config.length === 0 ? (
          <div className="text-center py-12 border-2 border-dashed rounded-lg">
            <p className="text-low">尚未配置任何模型</p>
            <p className="text-sm text-low mt-1">点击"添加模型"开始配置</p>
          </div>
        ) : (
          config.map(model => (
            <div
              key={model.id}
              className="flex items-center justify-between p-4 border rounded-lg bg-secondary"
            >
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{model.displayName}</span>
                  {model.isVerified && (
                    <span className="text-xs px-2 py-0.5 rounded bg-success/20 text-success flex items-center gap-1">
                      <Check className="w-3 h-3" /> 已验证
                    </span>
                  )}
                </div>
                <p className="text-sm text-low">
                  API: {API_TYPES.find(t => t.value === model.apiType)?.label} | 模型: {model.modelId}
                </p>
                {model.apiType === 'openai-compatible' && (
                  <p className="text-xs text-low">Base: {model.baseUrl}</p>
                )}
              </div>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" onClick={() => handleEdit(model)}>
                  <Pencil className="w-4 h-4" />
                </Button>
                <Button variant="outline" size="sm" onClick={() => handleDelete(model.id)}>
                  <Trash2 className="w-4 h-4" />
                </Button>
              </div>
            </div>
          ))
        )}
      </div>

      {errors.models && <p className="text-error text-sm">{errors.models}</p>}

      <p className="text-sm text-low">
        提示: 至少需要配置一个模型才能继续
      </p>
    </div>
  );
}

// 添加/编辑模型表单
function AddModelForm({
  initialModel,
  onSubmit,
  onCancel,
}: {
  initialModel: ModelConfig | null;
  onSubmit: (model: ModelConfig) => void;
  onCancel: () => void;
}) {
  const [model, setModel] = useState<ModelConfig>(
    initialModel || {
      id: uuid(),
      displayName: '',
      apiType: 'anthropic',
      baseUrl: 'https://api.anthropic.com',
      apiKey: '',
      modelId: '',
      isVerified: false,
    }
  );
  const [showApiKey, setShowApiKey] = useState(false);
  const [fetchingModels, setFetchingModels] = useState(false);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [verifying, setVerifying] = useState(false);

  // 获取可用模型
  const handleFetchModels = async () => {
    if (!model.apiKey || !model.baseUrl) return;

    setFetchingModels(true);
    try {
      const response = await fetch('/api/models/list', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          apiType: model.apiType,
          baseUrl: model.baseUrl,
          apiKey: model.apiKey,
        }),
      });
      const data = await response.json();
      setAvailableModels(data.models || []);
    } catch (error) {
      console.error('Failed to fetch models:', error);
    }
    setFetchingModels(false);
  };

  // 验证连接
  const handleVerify = async () => {
    if (!model.apiKey || !model.modelId) return;

    setVerifying(true);
    try {
      const response = await fetch('/api/models/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          apiType: model.apiType,
          baseUrl: model.baseUrl,
          apiKey: model.apiKey,
          modelId: model.modelId,
        }),
      });
      const data = await response.json();
      setModel(m => ({ ...m, isVerified: data.success }));
    } catch (error) {
      console.error('Failed to verify:', error);
    }
    setVerifying(false);
  };

  const handleApiTypeChange = (apiType: ApiType) => {
    const defaultUrl = API_TYPES.find(t => t.value === apiType)?.defaultUrl || '';
    setModel(m => ({ ...m, apiType, baseUrl: defaultUrl }));
    setAvailableModels([]);
  };

  return (
    <div className="space-y-4">
      {/* 模型名称 */}
      <div className="space-y-2">
        <Label>模型名称 (自定义显示名)</Label>
        <Input
          value={model.displayName}
          onChange={e => setModel(m => ({ ...m, displayName: e.target.value }))}
          placeholder="例如: Claude Sonnet"
        />
      </div>

      {/* API 类型 */}
      <div className="space-y-2">
        <Label>API 类型</Label>
        <div className="flex flex-wrap gap-2">
          {API_TYPES.map(type => (
            <button
              key={type.value}
              type="button"
              onClick={() => handleApiTypeChange(type.value)}
              className={cn(
                'px-3 py-1.5 rounded border text-sm',
                model.apiType === type.value
                  ? 'bg-brand border-brand text-white'
                  : 'bg-secondary border-muted hover:border-brand'
              )}
            >
              {type.label}
            </button>
          ))}
        </div>
      </div>

      {/* Base URL */}
      <div className="space-y-2">
        <Label>Base URL</Label>
        <Input
          value={model.baseUrl}
          onChange={e => setModel(m => ({ ...m, baseUrl: e.target.value }))}
          placeholder="https://api.example.com"
          disabled={model.apiType !== 'openai-compatible'}
        />
      </div>

      {/* API Key */}
      <div className="space-y-2">
        <Label>API Key</Label>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Input
              type={showApiKey ? 'text' : 'password'}
              value={model.apiKey}
              onChange={e => setModel(m => ({ ...m, apiKey: e.target.value, isVerified: false }))}
              placeholder="sk-xxx..."
            />
            <button
              type="button"
              onClick={() => setShowApiKey(!showApiKey)}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-low hover:text-normal"
            >
              {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          </div>
        </div>
      </div>

      {/* 获取可用模型 */}
      <div className="space-y-2 p-3 border rounded-lg bg-secondary/50">
        <Button
          variant="outline"
          onClick={handleFetchModels}
          disabled={!model.apiKey || !model.baseUrl || fetchingModels}
        >
          {fetchingModels ? (
            <RefreshCw className="w-4 h-4 mr-2 animate-spin" />
          ) : (
            <RefreshCw className="w-4 h-4 mr-2" />
          )}
          获取可用模型
        </Button>
        {availableModels.length > 0 && (
          <p className="text-sm text-success">
            ✓ 成功获取 {availableModels.length} 个可用模型
          </p>
        )}
      </div>

      {/* 模型选择 */}
      <div className="space-y-2">
        <Label>模型选择</Label>
        {availableModels.length > 0 ? (
          <Select
            value={model.modelId}
            onValueChange={v => setModel(m => ({ ...m, modelId: v, isVerified: false }))}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择模型" />
            </SelectTrigger>
            <SelectContent>
              {availableModels.map(m => (
                <SelectItem key={m} value={m}>{m}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input
            value={model.modelId}
            onChange={e => setModel(m => ({ ...m, modelId: e.target.value, isVerified: false }))}
            placeholder="手动输入模型 ID"
          />
        )}
      </div>

      {/* 验证连接 */}
      <div className="flex items-center gap-3">
        <Button variant="outline" onClick={handleVerify} disabled={!model.modelId || verifying}>
          {verifying ? '验证中...' : '验证连接'}
        </Button>
        {model.isVerified && (
          <span className="text-sm text-success flex items-center gap-1">
            <Check className="w-4 h-4" /> 连接成功，模型可用
          </span>
        )}
      </div>

      {/* 操作按钮 */}
      <div className="flex justify-end gap-2 pt-4 border-t">
        <Button variant="outline" onClick={onCancel}>取消</Button>
        <Button
          onClick={() => onSubmit(model)}
          disabled={!model.displayName || !model.apiKey || !model.modelId}
        >
          保存模型
        </Button>
      </div>
    </div>
  );
}
```

---

**交付物:**
- `Step2Tasks.tsx` - 任务详细配置
- `Step3Models.tsx` - 模型配置（含获取可用模型功能）

---

### Task 6.4: 步骤 4-6 组件（终端、斜杠命令、高级配置）

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step4Terminals.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step5Commands.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/steps/Step6Advanced.tsx`

---

**Step 6.4.1: 创建 Step4Terminals.tsx（终端配置）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step4Terminals.tsx`

```tsx
import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ChevronLeft, ChevronRight, Check, X, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { TerminalConfig, TaskConfig, ModelConfig } from '../types';
import { v4 as uuid } from 'uuid';

interface CliTypeInfo {
  id: string;
  name: string;
  displayName: string;
  installed: boolean;
  installGuideUrl?: string;
}

interface Props {
  config: TerminalConfig[];
  tasks: TaskConfig[];
  models: ModelConfig[];
  onChange: (config: TerminalConfig[]) => void;
  errors: Record<string, string>;
}

const CLI_TYPES: CliTypeInfo[] = [
  { id: 'claude-code', name: 'claude-code', displayName: 'Claude Code', installed: false },
  { id: 'gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false },
  { id: 'codex', name: 'codex', displayName: 'Codex', installed: false },
  { id: 'cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, installGuideUrl: 'https://cursor.com' },
];

export function Step4Terminals({ config, tasks, models, onChange, errors }: Props) {
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0);
  const [cliTypes, setCliTypes] = useState<CliTypeInfo[]>(CLI_TYPES);

  // 检测 CLI 安装状态
  useEffect(() => {
    fetch('/api/cli_types/detect')
      .then(res => res.json())
      .then((data: CliTypeInfo[]) => {
        setCliTypes(data);
      })
      .catch(() => {});
  }, []);

  // 初始化终端配置
  useEffect(() => {
    const totalTerminals = tasks.reduce((sum, t) => sum + t.terminalCount, 0);
    if (config.length !== totalTerminals) {
      const newTerminals: TerminalConfig[] = [];
      tasks.forEach(task => {
        for (let i = 0; i < task.terminalCount; i++) {
          const existing = config.find(
            t => t.taskId === task.id && t.orderIndex === i
          );
          newTerminals.push(
            existing || {
              id: uuid(),
              taskId: task.id,
              orderIndex: i,
              cliTypeId: '',
              modelConfigId: '',
            }
          );
        }
      });
      onChange(newTerminals);
    }
  }, [tasks]);

  const currentTask = tasks[currentTaskIndex];
  const taskTerminals = config.filter(t => t.taskId === currentTask?.id);

  const updateTerminal = (terminalId: string, updates: Partial<TerminalConfig>) => {
    onChange(config.map(t => t.id === terminalId ? { ...t, ...updates } : t));
  };

  if (!currentTask) return null;

  return (
    <div className="space-y-6">
      {/* 任务导航 */}
      <div className="flex items-center justify-between">
        <div>
          <span className="text-lg font-medium">配置终端</span>
          <span className="text-low ml-2">- 任务: {currentTask.name}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-low">任务 {currentTaskIndex + 1}/{tasks.length}</span>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setCurrentTaskIndex(i => Math.max(0, i - 1))}
            disabled={currentTaskIndex === 0}
          >
            <ChevronLeft className="w-4 h-4" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setCurrentTaskIndex(i => Math.min(tasks.length - 1, i + 1))}
            disabled={currentTaskIndex === tasks.length - 1}
          >
            <ChevronRight className="w-4 h-4" />
          </Button>
        </div>
      </div>

      <p className="text-sm text-low">此任务有 {currentTask.terminalCount} 个串行终端</p>

      {/* 终端配置列表 */}
      <div className="space-y-4">
        {taskTerminals
          .sort((a, b) => a.orderIndex - b.orderIndex)
          .map((terminal, idx) => (
            <div key={terminal.id} className="border rounded-lg p-4 bg-secondary/50">
              <div className="flex items-center gap-2 mb-4">
                <span className="font-medium">终端 {idx + 1}</span>
                {idx === 0 && <span className="text-xs text-low">(第一个执行)</span>}
                {idx > 0 && <span className="text-xs text-low">(等待终端{idx}完成后执行)</span>}
              </div>

              <div className="space-y-4">
                {/* CLI 选择 */}
                <div className="space-y-2">
                  <Label>CLI 选择</Label>
                  <div className="grid grid-cols-2 gap-2">
                    {cliTypes.map(cli => (
                      <button
                        key={cli.id}
                        type="button"
                        onClick={() => updateTerminal(terminal.id, { cliTypeId: cli.id })}
                        disabled={!cli.installed}
                        className={cn(
                          'flex items-center justify-between p-3 rounded border text-left',
                          terminal.cliTypeId === cli.id
                            ? 'bg-brand/10 border-brand'
                            : 'bg-secondary border-muted',
                          !cli.installed && 'opacity-50 cursor-not-allowed'
                        )}
                      >
                        <div className="flex items-center gap-2">
                          {terminal.cliTypeId === cli.id && <div className="w-2 h-2 rounded-full bg-brand" />}
                          <span>{cli.displayName}</span>
                        </div>
                        <div className="flex items-center gap-1 text-xs">
                          {cli.installed ? (
                            <span className="text-success flex items-center gap-1">
                              <Check className="w-3 h-3" /> 已安装
                            </span>
                          ) : (
                            <span className="text-error flex items-center gap-1">
                              <X className="w-3 h-3" /> 未安装
                              {cli.installGuideUrl && (
                                <a
                                  href={cli.installGuideUrl}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="text-brand hover:underline"
                                  onClick={e => e.stopPropagation()}
                                >
                                  <ExternalLink className="w-3 h-3" />
                                </a>
                              )}
                            </span>
                          )}
                        </div>
                      </button>
                    ))}
                  </div>
                </div>

                {/* 模型选择 */}
                <div className="space-y-2">
                  <Label>模型选择 (从步骤3配置的模型中选择)</Label>
                  <Select
                    value={terminal.modelConfigId}
                    onValueChange={v => updateTerminal(terminal.id, { modelConfigId: v })}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="选择模型" />
                    </SelectTrigger>
                    <SelectContent>
                      {models.map(m => (
                        <SelectItem key={m.id} value={m.id}>{m.displayName}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* 角色描述 */}
                <div className="space-y-2">
                  <Label>角色描述 (可选)</Label>
                  <Input
                    value={terminal.role || ''}
                    onChange={e => updateTerminal(terminal.id, { role: e.target.value })}
                    placeholder="例如: 代码编写者、代码审核者"
                    className="bg-secondary"
                  />
                </div>
              </div>
            </div>
          ))}
      </div>

      {errors.terminals && <p className="text-error text-sm">{errors.terminals}</p>}

      {cliTypes.some(c => !c.installed && taskTerminals.some(t => t.cliTypeId === c.id)) && (
        <p className="text-warning text-sm">
          ⚠️ 选择了未安装的 CLI 将无法进入下一步
        </p>
      )}
    </div>
  );
}
```

---

**Step 6.4.2: 创建 Step5Commands.tsx（斜杠命令配置）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step5Commands.tsx`

```tsx
import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { GripVertical, Plus, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { CommandConfig } from '../types';

interface CommandPreset {
  id: string;
  name: string;
  displayName: string;
  description: string;
  isSystem: boolean;
}

interface Props {
  config: CommandConfig;
  onChange: (config: CommandConfig) => void;
  errors: Record<string, string>;
}

const SYSTEM_PRESETS: CommandPreset[] = [
  { id: 'write-code', name: '/write-code', displayName: '编写代码', description: '编写功能代码', isSystem: true },
  { id: 'review', name: '/review', displayName: '代码审核', description: '代码审计，检查安全性和代码质量', isSystem: true },
  { id: 'fix-issues', name: '/fix-issues', displayName: '修复问题', description: '修复发现的问题', isSystem: true },
  { id: 'test', name: '/test', displayName: '测试', description: '编写和运行测试', isSystem: true },
  { id: 'refactor', name: '/refactor', displayName: '重构', description: '重构代码结构', isSystem: true },
];

export function Step5Commands({ config, onChange, errors }: Props) {
  const [presets, setPresets] = useState<CommandPreset[]>(SYSTEM_PRESETS);

  // 加载预设列表
  useEffect(() => {
    fetch('/api/workflows/presets/commands')
      .then(res => res.json())
      .then(data => setPresets([...SYSTEM_PRESETS, ...data.filter((p: CommandPreset) => !p.isSystem)]))
      .catch(() => {});
  }, []);

  const selectedPresets = config.presetIds
    .map(id => presets.find(p => p.id === id))
    .filter(Boolean) as CommandPreset[];

  const availablePresets = presets.filter(p => !config.presetIds.includes(p.id));

  const addPreset = (id: string) => {
    onChange({ ...config, presetIds: [...config.presetIds, id] });
  };

  const removePreset = (id: string) => {
    onChange({ ...config, presetIds: config.presetIds.filter(p => p !== id) });
  };

  const clearAll = () => {
    onChange({ ...config, presetIds: [] });
  };

  const resetDefault = () => {
    onChange({ ...config, presetIds: ['write-code', 'review', 'fix-issues'] });
  };

  // 拖拽排序（简化版）
  const moveUp = (index: number) => {
    if (index === 0) return;
    const newIds = [...config.presetIds];
    [newIds[index - 1], newIds[index]] = [newIds[index], newIds[index - 1]];
    onChange({ ...config, presetIds: newIds });
  };

  const moveDown = (index: number) => {
    if (index === config.presetIds.length - 1) return;
    const newIds = [...config.presetIds];
    [newIds[index], newIds[index + 1]] = [newIds[index + 1], newIds[index]];
    onChange({ ...config, presetIds: newIds });
  };

  return (
    <div className="space-y-6">
      {/* 是否启用斜杠命令 */}
      <div className="space-y-3">
        <Label>是否配置斜杠命令？</Label>
        <RadioGroup
          value={config.enabled ? 'yes' : 'no'}
          onValueChange={v => onChange({ ...config, enabled: v === 'yes' })}
        >
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="no" id="cmd-no" />
            <Label htmlFor="cmd-no" className="font-normal cursor-pointer">
              不配置 - 主 Agent 自行决策任务执行方式
            </Label>
          </div>
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="yes" id="cmd-yes" />
            <Label htmlFor="cmd-yes" className="font-normal cursor-pointer">
              配置斜杠命令 - 主 Agent 按命令顺序分发任务
            </Label>
          </div>
        </RadioGroup>
      </div>

      {config.enabled && (
        <>
          {/* 已选命令 */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>已选命令 (按执行顺序排列)</Label>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" onClick={clearAll}>清空</Button>
                <Button variant="outline" size="sm" onClick={resetDefault}>重置默认</Button>
              </div>
            </div>

            {selectedPresets.length === 0 ? (
              <div className="text-center py-8 border-2 border-dashed rounded-lg">
                <p className="text-low">尚未选择任何命令</p>
              </div>
            ) : (
              <div className="space-y-2">
                {selectedPresets.map((preset, index) => (
                  <div
                    key={preset.id}
                    className="flex items-center gap-3 p-3 border rounded-lg bg-secondary"
                  >
                    <div className="flex flex-col gap-1">
                      <button onClick={() => moveUp(index)} disabled={index === 0}>
                        <GripVertical className="w-4 h-4 text-low hover:text-normal" />
                      </button>
                    </div>
                    <span className="text-low w-6">{index + 1}.</span>
                    <span className="font-mono text-sm text-brand">{preset.name}</span>
                    <span className="text-sm text-low flex-1">{preset.description}</span>
                    <Button variant="ghost" size="sm" onClick={() => removePreset(preset.id)}>
                      <X className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 可用命令 */}
          <div className="space-y-3">
            <Label>可用命令预设</Label>

            <div className="space-y-3">
              <p className="text-sm text-low">系统内置:</p>
              <div className="flex flex-wrap gap-2">
                {presets
                  .filter(p => p.isSystem && !config.presetIds.includes(p.id))
                  .map(preset => (
                    <button
                      key={preset.id}
                      onClick={() => addPreset(preset.id)}
                      className="px-3 py-2 border rounded-lg bg-secondary hover:border-brand flex items-center gap-2"
                    >
                      <span className="font-mono text-sm">{preset.name}</span>
                      <Plus className="w-4 h-4 text-low" />
                    </button>
                  ))}
              </div>

              {presets.some(p => !p.isSystem) && (
                <>
                  <p className="text-sm text-low mt-4">用户自定义:</p>
                  <div className="flex flex-wrap gap-2">
                    {presets
                      .filter(p => !p.isSystem && !config.presetIds.includes(p.id))
                      .map(preset => (
                        <button
                          key={preset.id}
                          onClick={() => addPreset(preset.id)}
                          className="px-3 py-2 border rounded-lg bg-secondary hover:border-brand flex items-center gap-2"
                        >
                          <span className="font-mono text-sm">{preset.name}</span>
                          <Plus className="w-4 h-4 text-low" />
                        </button>
                      ))}
                  </div>
                </>
              )}
            </div>
          </div>
        </>
      )}

      {errors.commands && <p className="text-error text-sm">{errors.commands}</p>}
    </div>
  );
}
```

---

**Step 6.4.3: 创建 Step6Advanced.tsx（高级配置）**

文件路径: `vibe-kanban-main/frontend/src/components/workflow/steps/Step6Advanced.tsx`

```tsx
import { useState } from 'react';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { ChevronDown, FileText } from 'lucide-react';
import type { AdvancedConfig, ModelConfig } from '../types';

interface Props {
  config: AdvancedConfig;
  models: ModelConfig[];
  onChange: (config: AdvancedConfig) => void;
  errors: Record<string, string>;
}

// Git 提交规范（系统强制，不可修改）
const GIT_COMMIT_FORMAT = `[Terminal:{terminal_id}] [Status:{status}] {简要摘要}

## 变更内容
- 详细描述本次提交的所有变更
- 每个文件的修改目的
- 新增/修改/删除了哪些功能

## 技术细节
- 使用的技术方案
- 关键代码逻辑说明
- 依赖变更说明（如有）

## 测试情况
- 已执行的测试
- 测试结果

---METADATA---
workflow_id: {workflow_id}
task_id: {task_id}
terminal_id: {terminal_id}
terminal_order: {order}
cli: {cli_type}
model: {model}
status: {completed|review_pass|review_reject|failed}
files_changed: [{file_path, change_type, lines_added, lines_deleted}]
execution_time_seconds: {seconds}
token_usage: {input_tokens, output_tokens}`;

export function Step6Advanced({ config, models, onChange, errors }: Props) {
  const [showCommitFormat, setShowCommitFormat] = useState(false);

  const updateOrchestrator = (updates: Partial<typeof config.orchestrator>) => {
    onChange({ ...config, orchestrator: { ...config.orchestrator, ...updates } });
  };

  const updateErrorTerminal = (updates: Partial<typeof config.errorTerminal>) => {
    onChange({ ...config, errorTerminal: { ...config.errorTerminal, ...updates } });
  };

  const updateMergeTerminal = (updates: Partial<typeof config.mergeTerminal>) => {
    onChange({ ...config, mergeTerminal: { ...config.mergeTerminal, ...updates } });
  };

  return (
    <div className="space-y-6">
      {/* 主 Agent 配置 */}
      <div className="border rounded-lg p-4 space-y-4">
        <Label className="text-base font-medium">主 Agent (Orchestrator) 配置</Label>
        <div className="space-y-2">
          <Label>选择模型 (从步骤3已配置的模型中选择)</Label>
          <Select
            value={config.orchestrator.modelConfigId}
            onValueChange={v => updateOrchestrator({ modelConfigId: v })}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择模型" />
            </SelectTrigger>
            <SelectContent>
              {models.map(m => (
                <SelectItem key={m.id} value={m.id}>{m.displayName}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-low">推荐: 使用能力最强的模型作为主 Agent</p>
        </div>
        {errors.orchestrator && <p className="text-error text-sm">{errors.orchestrator}</p>}
      </div>

      {/* 错误处理终端 */}
      <div className="border rounded-lg p-4 space-y-4">
        <div className="flex items-center justify-between">
          <Label className="text-base font-medium">错误处理终端 (可选)</Label>
          <Switch
            checked={config.errorTerminal.enabled}
            onCheckedChange={checked => updateErrorTerminal({ enabled: checked })}
          />
        </div>
        {config.errorTerminal.enabled && (
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>CLI</Label>
              <Select
                value={config.errorTerminal.cliTypeId}
                onValueChange={v => updateErrorTerminal({ cliTypeId: v })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择 CLI" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="claude-code">Claude Code</SelectItem>
                  <SelectItem value="gemini-cli">Gemini CLI</SelectItem>
                  <SelectItem value="codex">Codex</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>模型</Label>
              <Select
                value={config.errorTerminal.modelConfigId}
                onValueChange={v => updateErrorTerminal({ modelConfigId: v })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择模型" />
                </SelectTrigger>
                <SelectContent>
                  {models.map(m => (
                    <SelectItem key={m.id} value={m.id}>{m.displayName}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        )}
      </div>

      {/* 合并终端配置 */}
      <div className="border rounded-lg p-4 space-y-4">
        <Label className="text-base font-medium">合并终端配置</Label>
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>CLI</Label>
            <Select
              value={config.mergeTerminal.cliTypeId}
              onValueChange={v => updateMergeTerminal({ cliTypeId: v })}
            >
              <SelectTrigger>
                <SelectValue placeholder="选择 CLI" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="claude-code">Claude Code</SelectItem>
                <SelectItem value="gemini-cli">Gemini CLI</SelectItem>
                <SelectItem value="codex">Codex</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>模型</Label>
            <Select
              value={config.mergeTerminal.modelConfigId}
              onValueChange={v => updateMergeTerminal({ modelConfigId: v })}
            >
              <SelectTrigger>
                <SelectValue placeholder="选择模型" />
              </SelectTrigger>
              <SelectContent>
                {models.map(m => (
                  <SelectItem key={m.id} value={m.id}>{m.displayName}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="flex items-center gap-6">
          <label className="flex items-center gap-2 cursor-pointer">
            <Switch
              checked={config.mergeTerminal.runTestsBeforeMerge}
              onCheckedChange={checked => updateMergeTerminal({ runTestsBeforeMerge: checked })}
            />
            <span className="text-sm">合并前运行测试</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <Switch
              checked={config.mergeTerminal.pauseOnConflict}
              onCheckedChange={checked => updateMergeTerminal({ pauseOnConflict: checked })}
            />
            <span className="text-sm">合并冲突时暂停等待人工处理</span>
          </label>
        </div>
        {errors.mergeTerminal && <p className="text-error text-sm">{errors.mergeTerminal}</p>}
      </div>

      {/* 目标分支 */}
      <div className="space-y-2">
        <Label>目标分支</Label>
        <Input
          value={config.targetBranch}
          onChange={e => onChange({ ...config, targetBranch: e.target.value })}
          placeholder="main"
          className="bg-secondary"
        />
      </div>

      {/* Git 提交规范 */}
      <Collapsible open={showCommitFormat} onOpenChange={setShowCommitFormat}>
        <CollapsibleTrigger className="flex items-center gap-2 text-sm text-low hover:text-normal">
          <FileText className="w-4 h-4" />
          <span>📋 Git 提交规范 (系统强制，不可修改)</span>
          <ChevronDown className={`w-4 h-4 transition-transform ${showCommitFormat ? 'rotate-180' : ''}`} />
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-3">
          <div className="border rounded-lg p-4 bg-secondary/50">
            <p className="text-sm text-low mb-2">
              系统要求每个终端完成任务后必须按以下格式提交 Git:
            </p>
            <pre className="text-xs font-mono bg-primary/10 p-3 rounded overflow-x-auto whitespace-pre-wrap">
              {GIT_COMMIT_FORMAT}
            </pre>
            <p className="text-xs text-low mt-2">
              此规范确保 Git 监测服务能准确识别终端状态和任务进度
            </p>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
```

---

**交付物:**
- `Step4Terminals.tsx` - 终端配置
- `Step5Commands.tsx` - 斜杠命令配置
- `Step6Advanced.tsx` - 高级配置（含 Git 提交规范展示）

---

### Task 6.5: 创建流水线视图

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/components/workflow/PipelineView.tsx`
- 创建: `vibe-kanban-main/frontend/src/components/workflow/TerminalCard.tsx`

---

**Step 6.2.1: 创建 PipelineView.tsx**

```tsx
import { TerminalCard } from './TerminalCard';
import type { Workflow, WorkflowTask, Terminal } from '@/shared/types';

interface Props {
  workflow: Workflow;
  tasks: Array<WorkflowTask & { terminals: Terminal[] }>;
  onTerminalClick?: (terminal: Terminal) => void;
}

export function PipelineView({ workflow, tasks, onTerminalClick }: Props) {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{workflow.name}</h2>
        <StatusBadge status={workflow.status} />
      </div>

      <div className="space-y-4">
        {tasks.map((task, taskIndex) => (
          <div key={task.id} className="p-4 border rounded-lg">
            <div className="flex items-center gap-2 mb-4">
              <span className="text-sm font-medium text-muted-foreground">Task {taskIndex + 1}</span>
              <span className="font-medium">{task.name}</span>
              <span className="text-xs px-2 py-0.5 rounded bg-muted">{task.branch}</span>
            </div>

            <div className="flex items-center gap-2">
              {task.terminals.map((terminal, terminalIndex) => (
                <div key={terminal.id} className="flex items-center">
                  <TerminalCard
                    terminal={terminal}
                    onClick={() => onTerminalClick?.(terminal)}
                  />
                  {terminalIndex < task.terminals.length - 1 && (
                    <div className="w-8 h-0.5 bg-muted-foreground/30" />
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="p-4 border-2 border-dashed rounded-lg text-center">
        <span className="text-muted-foreground">合并终端</span>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    created: 'bg-gray-100 text-gray-800',
    starting: 'bg-yellow-100 text-yellow-800',
    ready: 'bg-blue-100 text-blue-800',
    running: 'bg-green-100 text-green-800',
    completed: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
  };

  return (
    <span className={`px-2 py-1 rounded text-sm ${colors[status] || 'bg-gray-100'}`}>
      {status}
    </span>
  );
}
```

---

**Step 6.2.2: 创建 TerminalCard.tsx**

```tsx
import { cn } from '@/lib/utils';
import type { Terminal } from '@/shared/types';

interface Props {
  terminal: Terminal;
  onClick?: () => void;
}

const STATUS_STYLES: Record<string, { bg: string; border: string; icon: string }> = {
  not_started: { bg: 'bg-gray-50', border: 'border-gray-200', icon: '○' },
  starting: { bg: 'bg-yellow-50', border: 'border-yellow-300', icon: '◐' },
  waiting: { bg: 'bg-blue-50', border: 'border-blue-300', icon: '◑' },
  working: { bg: 'bg-green-50', border: 'border-green-400', icon: '●' },
  completed: { bg: 'bg-green-100', border: 'border-green-500', icon: '✓' },
  failed: { bg: 'bg-red-50', border: 'border-red-400', icon: '✗' },
};

export function TerminalCard({ terminal, onClick }: Props) {
  const style = STATUS_STYLES[terminal.status] || STATUS_STYLES.not_started;

  return (
    <div
      className={cn(
        'w-32 p-3 rounded-lg border-2 cursor-pointer transition-all hover:shadow-md',
        style.bg,
        style.border
      )}
      onClick={onClick}
    >
      <div className="flex items-center justify-between mb-2">
        <span className="text-lg">{style.icon}</span>
        <span className="text-xs text-muted-foreground">T{terminal.orderIndex + 1}</span>
      </div>
      <div className="text-sm font-medium truncate">{terminal.role || 'Terminal'}</div>
      <div className="text-xs text-muted-foreground truncate">{terminal.cliTypeId}</div>
    </div>
  );
}
```

---

**交付物:** `PipelineView.tsx`, `TerminalCard.tsx`

---

**Step 6.5.3: 创建 API Hooks**

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/hooks/useWorkflows.ts`
- 创建: `vibe-kanban-main/frontend/src/hooks/useCliTypes.ts`

---

**Step 6.3.1: 创建 useWorkflows.ts**

```tsx
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { Workflow, WorkflowDetailResponse, CreateWorkflowRequest } from '@/shared/types';

export function useWorkflows(projectId: string) {
  return useQuery({
    queryKey: ['workflows', projectId],
    queryFn: async () => {
      const res = await fetch(`/api/workflows?project_id=${projectId}`);
      if (!res.ok) throw new Error('Failed to fetch workflows');
      return res.json() as Promise<Workflow[]>;
    },
  });
}

export function useWorkflow(workflowId: string) {
  return useQuery({
    queryKey: ['workflow', workflowId],
    queryFn: async () => {
      const res = await fetch(`/api/workflows/${workflowId}`);
      if (!res.ok) throw new Error('Failed to fetch workflow');
      return res.json() as Promise<WorkflowDetailResponse>;
    },
    enabled: !!workflowId,
  });
}

export function useCreateWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateWorkflowRequest) => {
      const res = await fetch('/api/workflows', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error('Failed to create workflow');
      return res.json() as Promise<WorkflowDetailResponse>;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['workflows', data.workflow.projectId] });
    },
  });
}

export function useStartWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (workflowId: string) => {
      const res = await fetch(`/api/workflows/${workflowId}/start`, { method: 'POST' });
      if (!res.ok) throw new Error('Failed to start workflow');
    },
    onSuccess: (_, workflowId) => {
      queryClient.invalidateQueries({ queryKey: ['workflow', workflowId] });
    },
  });
}

export function useDeleteWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (workflowId: string) => {
      const res = await fetch(`/api/workflows/${workflowId}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Failed to delete workflow');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workflows'] });
    },
  });
}
```

---

**Step 6.3.2: 创建 useCliTypes.ts**

```tsx
import { useQuery } from '@tanstack/react-query';
import type { CliType, ModelConfig, CliDetectionStatus } from '@/shared/types';

export function useCliTypes() {
  return useQuery({
    queryKey: ['cliTypes'],
    queryFn: async () => {
      const res = await fetch('/api/cli_types');
      if (!res.ok) throw new Error('Failed to fetch CLI types');
      return res.json() as Promise<CliType[]>;
    },
  });
}

export function useCliDetection() {
  return useQuery({
    queryKey: ['cliDetection'],
    queryFn: async () => {
      const res = await fetch('/api/cli_types/detect');
      if (!res.ok) throw new Error('Failed to detect CLIs');
      return res.json() as Promise<CliDetectionStatus[]>;
    },
  });
}

export function useModelsForCli(cliTypeId: string) {
  return useQuery({
    queryKey: ['models', cliTypeId],
    queryFn: async () => {
      const res = await fetch(`/api/cli_types/${cliTypeId}/models`);
      if (!res.ok) throw new Error('Failed to fetch models');
      return res.json() as Promise<ModelConfig[]>;
    },
    enabled: !!cliTypeId,
  });
}
```

---

**交付物:** `useWorkflows.ts`, `useCliTypes.ts`

**验收标准:**
1. 编译通过
2. API 调用正常工作

---

## Phase 6 完成检查清单

- [ ] Task 6.1: 向导框架和类型定义完成 (types.ts, StepIndicator.tsx, WorkflowWizard.tsx)
- [ ] Task 6.2: 步骤 0-1 组件完成 (Step0Project.tsx, Step1Basic.tsx)
- [ ] Task 6.3: 步骤 2-3 组件完成 (Step2Tasks.tsx, Step3Models.tsx)
- [ ] Task 6.4: 步骤 4-6 组件完成 (Step4Terminals.tsx, Step5Commands.tsx, Step6Advanced.tsx)
- [ ] Task 6.5: 流水线视图完成 (PipelineView.tsx, API Hooks)

---

## Phase 7: 终端调试视图

### Task 7.1: 集成 xterm.js

**状态:** ⬜ 未开始

**前置条件:**
- Phase 6 已完成

**目标:**
集成 xterm.js 终端模拟器，实现终端调试视图。

**涉及文件:**
- 修改: `vibe-kanban-main/frontend/package.json`
- 创建: `vibe-kanban-main/frontend/src/components/terminal/TerminalEmulator.tsx`

---

**Step 7.1.1: 安装依赖**

```bash
cd vibe-kanban-main/frontend
pnpm add xterm xterm-addon-fit xterm-addon-web-links @xterm/xterm @xterm/addon-fit
```

---

**Step 7.1.2: 创建 TerminalEmulator.tsx**

文件路径: `vibe-kanban-main/frontend/src/components/terminal/TerminalEmulator.tsx`

```tsx
import { useEffect, useRef, useCallback } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

interface Props {
  terminalId: string;
  wsUrl?: string;
  onData?: (data: string) => void;
  onResize?: (cols: number, rows: number) => void;
}

export function TerminalEmulator({ terminalId, wsUrl, onData, onResize }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  // 初始化终端
  useEffect(() => {
    if (!containerRef.current) return;

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78',
      },
      scrollback: 10000,
      convertEol: true,
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);

    terminal.open(containerRef.current);
    fitAddon.fit();

    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;

    // 处理用户输入
    terminal.onData((data) => {
      onData?.(data);
      wsRef.current?.send(JSON.stringify({ type: 'input', data }));
    });

    // 处理窗口大小变化
    const handleResize = () => {
      fitAddon.fit();
      const { cols, rows } = terminal;
      onResize?.(cols, rows);
      wsRef.current?.send(JSON.stringify({ type: 'resize', cols, rows }));
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      terminal.dispose();
    };
  }, [onData, onResize]);

  // WebSocket 连接
  useEffect(() => {
    if (!wsUrl || !terminalRef.current) return;

    const ws = new WebSocket(`${wsUrl}/terminal/${terminalId}`);

    ws.onopen = () => {
      console.log('Terminal WebSocket connected');
      // 发送初始大小
      const { cols, rows } = terminalRef.current!;
      ws.send(JSON.stringify({ type: 'resize', cols, rows }));
    };

    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      if (message.type === 'output') {
        terminalRef.current?.write(message.data);
      }
    };

    ws.onerror = (error) => {
      console.error('Terminal WebSocket error:', error);
    };

    ws.onclose = () => {
      console.log('Terminal WebSocket closed');
    };

    wsRef.current = ws;

    return () => {
      ws.close();
    };
  }, [wsUrl, terminalId]);

  // 写入数据到终端
  const write = useCallback((data: string) => {
    terminalRef.current?.write(data);
  }, []);

  // 清空终端
  const clear = useCallback(() => {
    terminalRef.current?.clear();
  }, []);

  return (
    <div
      ref={containerRef}
      className="w-full h-full min-h-[300px] bg-[#1e1e1e] rounded-lg overflow-hidden"
    />
  );
}
```

---

**交付物:** `TerminalEmulator.tsx`

---

### Task 7.2: 实现 PTY WebSocket 后端

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/crates/server/src/routes/terminal_ws.rs`
- 修改: `vibe-kanban-main/crates/server/src/routes/mod.rs`

---

**Step 7.2.1: 创建 terminal_ws.rs**

文件路径: `vibe-kanban-main/crates/server/src/routes/terminal_ws.rs`

```rust
//! 终端 WebSocket 路由

use axum::{
    extract::{Path, State, WebSocketUpgrade, ws::{Message, WebSocket}},
    response::IntoResponse,
    routing::get,
    Router,
};
use futures::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::sync::mpsc;
use crate::AppState;

/// 创建终端 WebSocket 路由
pub fn terminal_ws_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/terminal/:terminal_id", get(terminal_ws_handler))
}

/// WebSocket 消息类型
#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
enum WsMessage {
    Input { data: String },
    Output { data: String },
    Resize { cols: u16, rows: u16 },
    Error { message: String },
}

/// WebSocket 处理器
async fn terminal_ws_handler(
    ws: WebSocketUpgrade,
    Path(terminal_id): Path<String>,
    State(state): State<Arc<AppState>>,
) -> impl IntoResponse {
    ws.on_upgrade(move |socket| handle_terminal_socket(socket, terminal_id, state))
}

/// 处理终端 WebSocket 连接
async fn handle_terminal_socket(
    socket: WebSocket,
    terminal_id: String,
    state: Arc<AppState>,
) {
    tracing::info!("Terminal WebSocket connected: {}", terminal_id);

    let (mut ws_sender, mut ws_receiver) = socket.split();
    let (tx, mut rx) = mpsc::channel::<String>(100);

    // 获取终端进程信息
    let terminal = match db::models::terminal_dao::get_terminal_by_id(
        &state.db.pool,
        &terminal_id,
    ).await {
        Ok(Some(t)) => t,
        Ok(None) => {
            let _ = ws_sender.send(Message::Text(
                serde_json::to_string(&WsMessage::Error {
                    message: "Terminal not found".to_string(),
                }).unwrap()
            )).await;
            return;
        }
        Err(e) => {
            let _ = ws_sender.send(Message::Text(
                serde_json::to_string(&WsMessage::Error {
                    message: format!("Database error: {}", e),
                }).unwrap()
            )).await;
            return;
        }
    };

    // 发送任务：从 rx 接收数据并发送到 WebSocket
    let send_task = tokio::spawn(async move {
        while let Some(data) = rx.recv().await {
            let msg = WsMessage::Output { data };
            if ws_sender.send(Message::Text(serde_json::to_string(&msg).unwrap())).await.is_err() {
                break;
            }
        }
    });

    // 接收任务：从 WebSocket 接收数据
    let tx_clone = tx.clone();
    let recv_task = tokio::spawn(async move {
        while let Some(Ok(msg)) = ws_receiver.next().await {
            match msg {
                Message::Text(text) => {
                    if let Ok(ws_msg) = serde_json::from_str::<WsMessage>(&text) {
                        match ws_msg {
                            WsMessage::Input { data } => {
                                // TODO: 发送到 PTY
                                tracing::debug!("Input: {}", data);
                            }
                            WsMessage::Resize { cols, rows } => {
                                // TODO: 调整 PTY 大小
                                tracing::debug!("Resize: {}x{}", cols, rows);
                            }
                            _ => {}
                        }
                    }
                }
                Message::Close(_) => break,
                _ => {}
            }
        }
    });

    // 等待任务完成
    tokio::select! {
        _ = send_task => {}
        _ = recv_task => {}
    }

    tracing::info!("Terminal WebSocket disconnected: {}", terminal_id);
}
```

---

**Step 7.2.2: 更新 routes/mod.rs**

在路由注册中添加：

```rust
pub mod terminal_ws;

// 在 api_routes 函数中添加
.merge(terminal_ws::terminal_ws_routes())
```

---

**交付物:** `terminal_ws.rs`

---

### Task 7.3: 创建终端调试页面

**状态:** ⬜ 未开始

**涉及文件:**
- 创建: `vibe-kanban-main/frontend/src/components/terminal/TerminalDebugView.tsx`
- 创建: `vibe-kanban-main/frontend/src/pages/WorkflowDebug.tsx`

---

**Step 7.3.1: 创建 TerminalDebugView.tsx**

```tsx
import { useState } from 'react';
import { TerminalEmulator } from './TerminalEmulator';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { Terminal, WorkflowTask } from '@/shared/types';

interface Props {
  tasks: Array<WorkflowTask & { terminals: Terminal[] }>;
  wsUrl: string;
}

export function TerminalDebugView({ tasks, wsUrl }: Props) {
  const [selectedTerminalId, setSelectedTerminalId] = useState<string | null>(null);

  const allTerminals = tasks.flatMap(task =>
    task.terminals.map(t => ({ ...t, taskName: task.name }))
  );

  const selectedTerminal = allTerminals.find(t => t.id === selectedTerminalId);

  return (
    <div className="flex h-full">
      {/* 终端列表 */}
      <div className="w-64 border-r bg-muted/30 overflow-y-auto">
        <div className="p-4 border-b">
          <h3 className="font-semibold">终端列表</h3>
        </div>
        <div className="p-2">
          {allTerminals.map((terminal) => (
            <button
              key={terminal.id}
              className={cn(
                'w-full p-3 rounded-lg text-left mb-2 transition-colors',
                selectedTerminalId === terminal.id
                  ? 'bg-primary text-primary-foreground'
                  : 'hover:bg-muted'
              )}
              onClick={() => setSelectedTerminalId(terminal.id)}
            >
              <div className="font-medium text-sm">
                {terminal.role || `Terminal ${terminal.orderIndex + 1}`}
              </div>
              <div className="text-xs opacity-70">{terminal.taskName}</div>
              <div className="flex items-center gap-2 mt-1">
                <StatusDot status={terminal.status} />
                <span className="text-xs">{terminal.status}</span>
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* 终端视图 */}
      <div className="flex-1 flex flex-col">
        {selectedTerminal ? (
          <>
            <div className="p-4 border-b flex items-center justify-between">
              <div>
                <h3 className="font-semibold">
                  {selectedTerminal.role || `Terminal ${selectedTerminal.orderIndex + 1}`}
                </h3>
                <p className="text-sm text-muted-foreground">
                  {selectedTerminal.cliTypeId} - {selectedTerminal.modelConfigId}
                </p>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" size="sm">清空</Button>
                <Button variant="outline" size="sm">重启</Button>
              </div>
            </div>
            <div className="flex-1 p-4">
              <TerminalEmulator
                terminalId={selectedTerminal.id}
                wsUrl={wsUrl}
              />
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-muted-foreground">
            选择一个终端开始调试
          </div>
        )}
      </div>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  const colors: Record<string, string> = {
    not_started: 'bg-gray-400',
    starting: 'bg-yellow-400',
    waiting: 'bg-blue-400',
    working: 'bg-green-400 animate-pulse',
    completed: 'bg-green-500',
    failed: 'bg-red-500',
  };

  return <div className={cn('w-2 h-2 rounded-full', colors[status] || 'bg-gray-400')} />;
}
```

---

**Step 7.3.2: 创建 WorkflowDebug.tsx 页面**

```tsx
import { useParams } from 'react-router-dom';
import { useWorkflow } from '@/hooks/useWorkflows';
import { TerminalDebugView } from '@/components/terminal/TerminalDebugView';
import { PipelineView } from '@/components/workflow/PipelineView';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import { ArrowLeft, Play, Pause, Square } from 'lucide-react';
import { Link } from 'react-router-dom';

export function WorkflowDebugPage() {
  const { workflowId } = useParams<{ workflowId: string }>();
  const { data, isLoading, error } = useWorkflow(workflowId!);

  if (isLoading) {
    return <div className="p-8 text-center">加载中...</div>;
  }

  if (error || !data) {
    return <div className="p-8 text-center text-red-500">加载失败</div>;
  }

  const wsUrl = `ws://${window.location.host}`;

  return (
    <div className="h-screen flex flex-col">
      {/* 头部 */}
      <header className="border-b p-4 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Link to="/workflows">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="w-4 h-4 mr-2" /> 返回
            </Button>
          </Link>
          <div>
            <h1 className="font-semibold">{data.workflow.name}</h1>
            <p className="text-sm text-muted-foreground">
              状态: {data.workflow.status}
            </p>
          </div>
        </div>
        <div className="flex gap-2">
          {data.workflow.status === 'ready' && (
            <Button size="sm">
              <Play className="w-4 h-4 mr-2" /> 开始
            </Button>
          )}
          {data.workflow.status === 'running' && (
            <>
              <Button variant="outline" size="sm">
                <Pause className="w-4 h-4 mr-2" /> 暂停
              </Button>
              <Button variant="destructive" size="sm">
                <Square className="w-4 h-4 mr-2" /> 停止
              </Button>
            </>
          )}
        </div>
      </header>

      {/* 主内容 */}
      <div className="flex-1 overflow-hidden">
        <Tabs defaultValue="pipeline" className="h-full flex flex-col">
          <TabsList className="mx-4 mt-4">
            <TabsTrigger value="pipeline">流水线视图</TabsTrigger>
            <TabsTrigger value="terminals">终端调试</TabsTrigger>
          </TabsList>

          <TabsContent value="pipeline" className="flex-1 p-4 overflow-auto">
            <PipelineView workflow={data.workflow} tasks={data.tasks} />
          </TabsContent>

          <TabsContent value="terminals" className="flex-1 overflow-hidden">
            <TerminalDebugView tasks={data.tasks} wsUrl={wsUrl} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
```

---

**交付物:** `TerminalDebugView.tsx`, `WorkflowDebug.tsx`

**验收标准:**
1. 编译通过
2. 终端模拟器可以正常显示
3. WebSocket 连接正常

---

## Phase 7 完成检查清单

- [ ] Task 7.1: xterm.js 集成完成
- [ ] Task 7.2: PTY WebSocket 后端完成
- [ ] Task 7.3: 终端调试页面完成

---

## Phase 8: 集成测试与文档

### Task 8.1: 端到端测试

**状态:** ⬜ 未开始

**前置条件:**
- Phase 7 已完成

**目标:**
编写端到端测试，验证工作流创建、启动、执行的完整流程。

**涉及文件:**
- 创建: `vibe-kanban-main/tests/e2e/workflow_test.rs`

---

**Step 8.1.1: 创建 workflow_test.rs**

```rust
//! 工作流端到端测试

use reqwest::Client;
use serde_json::json;

const BASE_URL: &str = "http://localhost:3001";

#[tokio::test]
async fn test_workflow_lifecycle() {
    let client = Client::new();

    // 1. 获取 CLI 类型
    let res = client.get(format!("{}/api/cli_types", BASE_URL))
        .send().await.unwrap();
    assert!(res.status().is_success());
    let cli_types: Vec<serde_json::Value> = res.json().await.unwrap();
    assert!(!cli_types.is_empty());

    let claude_cli = cli_types.iter()
        .find(|c| c["name"] == "claude-code")
        .expect("Claude CLI not found");

    // 2. 获取模型
    let cli_id = claude_cli["id"].as_str().unwrap();
    let res = client.get(format!("{}/api/cli_types/{}/models", BASE_URL, cli_id))
        .send().await.unwrap();
    assert!(res.status().is_success());
    let models: Vec<serde_json::Value> = res.json().await.unwrap();
    let model_id = models[0]["id"].as_str().unwrap();

    // 3. 创建工作流
    let workflow_req = json!({
        "project_id": "test-project",
        "name": "Test Workflow",
        "use_slash_commands": false,
        "merge_terminal_config": {
            "cli_type_id": cli_id,
            "model_config_id": model_id
        },
        "tasks": [{
            "name": "Test Task",
            "terminals": [{
                "cli_type_id": cli_id,
                "model_config_id": model_id,
                "role": "coder"
            }]
        }]
    });

    let res = client.post(format!("{}/api/workflows", BASE_URL))
        .json(&workflow_req)
        .send().await.unwrap();
    assert!(res.status().is_success());
    let workflow: serde_json::Value = res.json().await.unwrap();
    let workflow_id = workflow["workflow"]["id"].as_str().unwrap();

    // 4. 获取工作流详情
    let res = client.get(format!("{}/api/workflows/{}", BASE_URL, workflow_id))
        .send().await.unwrap();
    assert!(res.status().is_success());

    // 5. 删除工作流
    let res = client.delete(format!("{}/api/workflows/{}", BASE_URL, workflow_id))
        .send().await.unwrap();
    assert!(res.status().is_success());
}

#[tokio::test]
async fn test_cli_detection() {
    let client = Client::new();

    let res = client.get(format!("{}/api/cli_types/detect", BASE_URL))
        .send().await.unwrap();
    assert!(res.status().is_success());

    let detection: Vec<serde_json::Value> = res.json().await.unwrap();
    assert!(!detection.is_empty());

    // 检查返回格式
    for cli in detection {
        assert!(cli.get("cli_type_id").is_some());
        assert!(cli.get("installed").is_some());
    }
}
```

---

**交付物:** `tests/e2e/workflow_test.rs`

---

### Task 8.2: 性能优化

**状态:** ⬜ 未开始

**目标:**
优化数据库查询和 WebSocket 连接管理。

**涉及文件:**
- 修改: 多个文件

---

**Step 8.2.1: 数据库索引优化**

确保以下索引存在（在迁移文件中已添加）：

```sql
-- 工作流查询优化
CREATE INDEX IF NOT EXISTS idx_workflow_project_status ON workflow(project_id, status);

-- 终端查询优化
CREATE INDEX IF NOT EXISTS idx_terminal_workflow_task_status ON terminal(workflow_task_id, status);

-- Git 事件查询优化
CREATE INDEX IF NOT EXISTS idx_git_event_workflow_status ON git_event(workflow_id, process_status);
```

---

**Step 8.2.2: 连接池配置**

在 `DBService` 中配置连接池：

```rust
let pool = SqlitePoolOptions::new()
    .max_connections(10)
    .min_connections(2)
    .acquire_timeout(Duration::from_secs(30))
    .idle_timeout(Duration::from_secs(600))
    .connect(&database_url)
    .await?;
```

---

**交付物:** 优化后的代码

---

### Task 8.3: 用户文档

**状态:** ⬜ 未开始

**目标:**
更新用户文档，说明新功能的使用方法。

**涉及文件:**
- 修改: `README.md`
- 创建: `docs/workflow-guide.md`

---

**Step 8.3.1: 更新 README.md**

添加工作流功能说明章节。

---

**Step 8.3.2: 创建 workflow-guide.md**

```markdown
# SoloDawn 工作流使用指南

## 概述

SoloDawn 工作流允许您协调多个 AI 编码代理并行完成复杂的软件开发任务。

## 创建工作流

1. 进入项目页面
2. 点击"创建工作流"按钮
3. 按照向导配置：
   - 工作流名称和描述
   - 并行任务（每个任务对应一个 Git 分支）
   - 每个任务的终端配置（CLI 类型和模型）
   - 合并终端配置

## 工作流状态

| 状态 | 说明 |
|------|------|
| created | 已创建，等待启动 |
| starting | 正在启动终端 |
| ready | 所有终端就绪，等待确认 |
| running | 正在执行 |
| merging | 正在合并分支 |
| completed | 已完成 |
| failed | 失败 |

## 终端调试

在工作流运行时，您可以：
1. 切换到"终端调试"标签页
2. 选择要查看的终端
3. 实时查看终端输出
4. 必要时手动输入命令

## 最佳实践

1. 将大任务拆分为独立的并行任务
2. 每个任务使用独立的 Git 分支
3. 配置审核终端以确保代码质量
4. 使用合适的模型（复杂任务用 Opus，简单任务用 Sonnet）
```

---

**交付物:** `docs/workflow-guide.md`

**验收标准:**
1. 文档清晰易懂
2. 包含所有主要功能说明

---

## Phase 8 完成检查清单

- [ ] Task 8.1: 端到端测试完成
- [ ] Task 8.2: 性能优化完成
- [ ] Task 8.3: 用户文档完成

---

## 附录
| 数据库模型 | `crates/db/src/models/workflow.rs` | 工作流模型 |
| 数据库模型 | `crates/db/src/models/terminal.rs` | 终端模型 |
| 数据库模型 | `crates/db/src/models/cli_type.rs` | CLI 类型模型 |
| API 路由 | `crates/server/src/routes/workflows.rs` | 工作流 API |
| API 路由 | `crates/server/src/routes/cli_types.rs` | CLI 类型 API |
| CC-Switch | `crates/cc-switch/src/lib.rs` | CC-Switch 入口 |
| CC-Switch | `crates/cc-switch/src/switcher.rs` | 模型切换服务 |
| 服务层 | `crates/services/src/services/cc_switch.rs` | CC-Switch 服务封装 |

### B. API 端点汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/cli_types | 获取所有 CLI 类型 |
| GET | /api/cli_types/detect | 检测已安装的 CLI |
| GET | /api/cli_types/:id/models | 获取 CLI 的模型列表 |
| GET | /api/workflows | 获取工作流列表 |
| POST | /api/workflows | 创建工作流 |
| GET | /api/workflows/:id | 获取工作流详情 |
| DELETE | /api/workflows/:id | 删除工作流 |
| PUT | /api/workflows/:id/status | 更新工作流状态 |
| POST | /api/workflows/:id/start | 启动工作流 |
| GET | /api/workflows/presets/commands | 获取斜杠命令预设 |

### C. 数据库表汇总

| 表名 | 说明 |
|------|------|
| cli_type | CLI 类型 |
| model_config | 模型配置 |
| slash_command_preset | 斜杠命令预设 |
| workflow | 工作流 |
| workflow_command | 工作流命令关联 |
| workflow_task | 工作流任务 |
| terminal | 终端 |
| terminal_log | 终端日志 |
| git_event | Git 事件 |

---

*文档版本: 2.0*
*创建日期: 2026-01-16*
*最后更新: 2026-01-17*
