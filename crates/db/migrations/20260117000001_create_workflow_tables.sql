-- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now'), cli_type_id references)
-- in this migration. This is acceptable for SQL DDL migrations where each table definition
-- requires its own DEFAULT clause and foreign key references.

-- ============================================================================
-- SoloDawn Workflow Tables Migration
-- Created: 2026-01-17
-- Description: Add workflow coordination tables for multi-terminal orchestration
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. CLI Type Table (cli_type)
-- Stores supported AI coding agent CLI information
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cli_type (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,           -- Internal name, e.g., 'claude-code'
    display_name        TEXT NOT NULL,                  -- Display name, e.g., 'Claude Code'
    detect_command      TEXT NOT NULL,                  -- Detection command, e.g., 'claude --version'
    install_command     TEXT,                           -- Installation command (optional)
    install_guide_url   TEXT,                           -- Installation guide URL
    config_file_path    TEXT,                           -- Config file path template
    is_system           INTEGER NOT NULL DEFAULT 1,     -- Is system built-in
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Insert system built-in CLI types
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
-- 2. Model Config Table (model_config)
-- Stores model configurations for each CLI
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS model_config (
    id              TEXT PRIMARY KEY,
    cli_type_id     TEXT NOT NULL REFERENCES cli_type(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,                      -- Model internal name, e.g., 'sonnet'
    display_name    TEXT NOT NULL,                      -- Display name, e.g., 'Claude Sonnet'
    api_model_id    TEXT,                               -- API model ID, e.g., 'claude-sonnet-4-20250514'
    is_default      INTEGER NOT NULL DEFAULT 0,         -- Is default model
    is_official     INTEGER NOT NULL DEFAULT 0,         -- Is official model
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(cli_type_id, name)
);

-- Insert Claude Code default models
WITH cli_type_ids AS (
    SELECT
        'cli-claude-code' AS claude_cli_id,
        'cli-gemini' AS gemini_cli_id,
        'cli-codex' AS codex_cli_id
)
INSERT INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official)
SELECT 'model-claude-sonnet', claude_cli_id, 'sonnet', 'Claude Sonnet', 'claude-sonnet-4-20250514', 1, 1 FROM cli_type_ids
UNION ALL
SELECT 'model-claude-opus', claude_cli_id, 'opus', 'Claude Opus', 'claude-opus-4-5-20251101', 0, 1 FROM cli_type_ids
UNION ALL
SELECT 'model-claude-haiku', claude_cli_id, 'haiku', 'Claude Haiku', 'claude-haiku-4-5-20251001', 0, 1 FROM cli_type_ids
UNION ALL
SELECT 'model-gemini-pro', gemini_cli_id, 'gemini-pro', 'Gemini Pro', 'gemini-2.5-pro', 1, 1 FROM cli_type_ids
UNION ALL
SELECT 'model-gemini-flash', gemini_cli_id, 'gemini-flash', 'Gemini Flash', 'gemini-2.5-flash', 0, 1 FROM cli_type_ids
UNION ALL
SELECT 'model-codex-gpt4o', codex_cli_id, 'gpt-4o', 'GPT-4o', 'gpt-4o', 1, 1 FROM cli_type_ids
UNION ALL
SELECT 'model-codex-o1', codex_cli_id, 'o1', 'O1', 'o1', 0, 1 FROM cli_type_ids;

-- ----------------------------------------------------------------------------
-- 3. Slash Command Preset Table (slash_command_preset)
-- Stores reusable slash command presets
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS slash_command_preset (
    id              TEXT PRIMARY KEY,
    command         TEXT NOT NULL UNIQUE,               -- Command name, e.g., '/write-code'
    description     TEXT NOT NULL,                      -- Command description
    prompt_template TEXT,                               -- Prompt template (optional)
    is_system       INTEGER NOT NULL DEFAULT 0,         -- Is system built-in
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Insert system built-in slash commands
INSERT INTO slash_command_preset (id, command, description, prompt_template, is_system) VALUES
    ('cmd-write-code', '/write-code', 'Write feature code', 'Please write code according to the following requirements:\n\n{requirement}\n\nRequirements:\n1. High code quality, maintainable\n2. Include necessary comments\n3. Follow existing project code style', 1),
    ('cmd-review', '/review', 'Code review for security and quality', 'Please review the following code changes:\n\n{changes}\n\nReview points:\n1. Security (XSS, SQL injection, command injection, etc.)\n2. Code quality and maintainability\n3. Performance issues\n4. Edge case handling', 1),
    ('cmd-fix-issues', '/fix-issues', 'Fix discovered issues', 'Please fix the following issues:\n\n{issues}\n\nRequirements:\n1. Minimize modification scope\n2. Do not introduce new issues\n3. Add necessary tests', 1),
    ('cmd-test', '/test', 'Write and run tests', 'Please write tests for the following code:\n\n{code}\n\nRequirements:\n1. Cover main functional paths\n2. Include edge cases\n3. Tests should be independently runnable', 1),
    ('cmd-refactor', '/refactor', 'Refactor code', 'Please refactor the following code:\n\n{code}\n\nRefactoring goals:\n{goals}\n\nRequirements:\n1. Keep functionality unchanged\n2. Improve code quality\n3. Step by step, each step verifiable', 1),
    ('cmd-document', '/document', 'Write documentation', 'Please write documentation for the following content:\n\n{content}\n\nDocument type: {doc_type}\n\nRequirements:\n1. Clear and easy to understand\n2. Include examples\n3. Proper format', 1);

-- ----------------------------------------------------------------------------
-- 4. Workflow Table (workflow)
-- Stores workflow configuration and state
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow (
    id                      TEXT PRIMARY KEY,
    project_id              TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    description             TEXT,

    -- Status: created, starting, ready, running, paused, merging, completed, failed, cancelled
    status                  TEXT NOT NULL DEFAULT 'created',

    -- Slash command configuration
    use_slash_commands      INTEGER NOT NULL DEFAULT 0,

    -- Main Agent (Orchestrator) configuration
    orchestrator_enabled    INTEGER NOT NULL DEFAULT 1,
    orchestrator_api_type   TEXT,                       -- 'openai' | 'anthropic' | 'custom'
    orchestrator_base_url   TEXT,
    orchestrator_api_key    TEXT,                       -- Encrypted storage
    orchestrator_model      TEXT,

    -- Error handling terminal configuration (optional)
    error_terminal_enabled  INTEGER NOT NULL DEFAULT 0,
    error_terminal_cli_id   TEXT REFERENCES cli_type(id),
    error_terminal_model_id TEXT REFERENCES model_config(id),

    -- Merge terminal configuration (required)
    merge_terminal_cli_id   TEXT NOT NULL REFERENCES cli_type(id),
    merge_terminal_model_id TEXT NOT NULL REFERENCES model_config(id),

    -- Target branch
    target_branch           TEXT NOT NULL DEFAULT 'main',

    -- Timestamps
    ready_at                TEXT,                       -- All terminals started completion time
    started_at              TEXT,                       -- User confirmed start time
    completed_at            TEXT,
    created_at              TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_project_id ON workflow(project_id);
CREATE INDEX IF NOT EXISTS idx_workflow_status ON workflow(status);

-- ----------------------------------------------------------------------------
-- 5. Workflow Command Association Table (workflow_command)
-- Stores slash commands used by workflow and their order
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow_command (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    preset_id       TEXT NOT NULL REFERENCES slash_command_preset(id),
    order_index     INTEGER NOT NULL,                   -- Execution order, starting from 0
    custom_params   TEXT,                               -- JSON format custom parameters
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(workflow_id, order_index)
);

CREATE INDEX IF NOT EXISTS idx_workflow_command_workflow_id ON workflow_command(workflow_id);

-- ----------------------------------------------------------------------------
-- 6. Workflow Task Table (workflow_task)
-- Stores parallel tasks in workflow
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workflow_task (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,

    -- Associated with vibe-kanban task table (optional, for status sync)
    vk_task_id      BLOB REFERENCES tasks(id),

    name            TEXT NOT NULL,
    description     TEXT,
    branch          TEXT NOT NULL,                      -- Git branch name

    -- Status: pending, running, review_pending, completed, failed, cancelled
    status          TEXT NOT NULL DEFAULT 'pending',

    order_index     INTEGER NOT NULL,                   -- Task order (for UI display)

    -- Timestamps
    started_at      TEXT,
    completed_at    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_id ON workflow_task(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_task_status ON workflow_task(status);

-- ----------------------------------------------------------------------------
-- 7. Terminal Table (terminal)
-- Stores terminal configuration and state for each task
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS terminal (
    id                  TEXT PRIMARY KEY,
    workflow_task_id    TEXT NOT NULL REFERENCES workflow_task(id) ON DELETE CASCADE,

    -- CLI and model configuration
    cli_type_id         TEXT NOT NULL REFERENCES cli_type(id),
    model_config_id     TEXT NOT NULL REFERENCES model_config(id),

    -- Custom API configuration (overrides default)
    custom_base_url     TEXT,
    custom_api_key      TEXT,                           -- Encrypted storage

    -- Role description (optional, for main Agent to understand terminal responsibilities)
    role                TEXT,                           -- e.g., 'coder', 'reviewer', 'fixer'
    role_description    TEXT,

    order_index         INTEGER NOT NULL,               -- Execution order within task, starting from 0

    -- Status: not_started, starting, waiting, working, completed, failed, cancelled
    status              TEXT NOT NULL DEFAULT 'not_started',

    -- Process information
    process_id          INTEGER,                        -- OS process ID
    pty_session_id      TEXT,                           -- PTY session ID (for terminal debug view)

    -- Associated with vibe-kanban session (optional)
    vk_session_id       BLOB REFERENCES sessions(id),

    -- Last Git commit information
    last_commit_hash    TEXT,
    last_commit_message TEXT,

    -- Timestamps
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_terminal_workflow_task_id ON terminal(workflow_task_id);
CREATE INDEX IF NOT EXISTS idx_terminal_status ON terminal(status);
CREATE INDEX IF NOT EXISTS idx_terminal_cli_type_id ON terminal(cli_type_id);

-- ----------------------------------------------------------------------------
-- 8. Terminal Log Table (terminal_log)
-- Stores terminal execution logs (for debugging and auditing)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS terminal_log (
    id              TEXT PRIMARY KEY,
    terminal_id     TEXT NOT NULL REFERENCES terminal(id) ON DELETE CASCADE,

    -- Log type: stdout, stderr, system, git_event
    log_type        TEXT NOT NULL,

    content         TEXT NOT NULL,

    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_terminal_log_terminal_id ON terminal_log(terminal_id);
CREATE INDEX IF NOT EXISTS idx_terminal_log_created_at ON terminal_log(created_at);

-- ----------------------------------------------------------------------------
-- 9. Git Event Table (git_event)
-- Stores Git commit events (for event-driven)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS git_event (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    terminal_id     TEXT REFERENCES terminal(id),

    -- Git information
    commit_hash     TEXT NOT NULL,
    branch          TEXT NOT NULL,
    commit_message  TEXT NOT NULL,

    -- Parsed metadata (JSON format)
    metadata        TEXT,

    -- Processing status: pending, processing, processed, failed
    process_status  TEXT NOT NULL DEFAULT 'pending',

    -- Main Agent response (JSON format)
    agent_response  TEXT,

    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at    TEXT
);

CREATE INDEX IF NOT EXISTS idx_git_event_workflow_id ON git_event(workflow_id);
CREATE INDEX IF NOT EXISTS idx_git_event_terminal_id ON git_event(terminal_id);
CREATE INDEX IF NOT EXISTS idx_git_event_process_status ON git_event(process_status);
