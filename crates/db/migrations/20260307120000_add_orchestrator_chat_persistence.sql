-- Persist orchestrator chat messages, command execution snapshots,
-- and external conversation bindings for replay/audit use cases.

CREATE TABLE IF NOT EXISTS workflow_orchestrator_message (
    id                  TEXT PRIMARY KEY,
    workflow_id         TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    command_id          TEXT,
    role                TEXT NOT NULL,
    content             TEXT NOT NULL,
    source              TEXT NOT NULL,
    external_message_id TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_orchestrator_message_workflow_created
    ON workflow_orchestrator_message(workflow_id, created_at);

CREATE INDEX IF NOT EXISTS idx_workflow_orchestrator_message_command
    ON workflow_orchestrator_message(command_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_orchestrator_message_dedup
    ON workflow_orchestrator_message(workflow_id, source, external_message_id)
    WHERE external_message_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS workflow_orchestrator_command (
    id                  TEXT PRIMARY KEY,
    workflow_id         TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    source              TEXT NOT NULL,
    external_message_id TEXT,
    request_message     TEXT NOT NULL,
    status              TEXT NOT NULL,
    error               TEXT,
    retryable           INTEGER NOT NULL DEFAULT 0,
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_orchestrator_command_workflow_created
    ON workflow_orchestrator_command(workflow_id, created_at);

CREATE INDEX IF NOT EXISTS idx_workflow_orchestrator_command_status
    ON workflow_orchestrator_command(status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_orchestrator_command_dedup
    ON workflow_orchestrator_command(workflow_id, source, external_message_id)
    WHERE external_message_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS external_conversation_binding (
    id              TEXT PRIMARY KEY,
    provider        TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    created_by      TEXT,
    is_active       INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(provider, conversation_id)
);

CREATE INDEX IF NOT EXISTS idx_external_conversation_binding_workflow
    ON external_conversation_binding(workflow_id);
