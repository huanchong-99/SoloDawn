# Phase 1: 数据库模型扩展

> **状态:** ⬜ 未开始
> **进度追踪:** 查看 `TODO.md`
> **前置条件:** Phase 0 完成

## 概述

扩展 Vibe Kanban 数据库，添加工作流协调相关表。

---

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
