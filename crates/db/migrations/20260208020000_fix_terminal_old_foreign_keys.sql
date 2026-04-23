-- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now'), table/column names) in this migration.
-- This is acceptable for SQL DDL migrations where table rebuild requires repeating column definitions.
PRAGMA foreign_keys = OFF;

CREATE TABLE terminal_log_new (
    id TEXT PRIMARY KEY,
    terminal_id TEXT NOT NULL REFERENCES terminal(id) ON DELETE CASCADE,
    log_type TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')) -- NOSONAR: SQLite migration DDL repeats this DEFAULT by design.
);

INSERT INTO terminal_log_new (id, terminal_id, log_type, content, created_at)
SELECT id, terminal_id, log_type, content, created_at
FROM terminal_log;

DROP TABLE terminal_log;
ALTER TABLE terminal_log_new RENAME TO terminal_log;

CREATE INDEX IF NOT EXISTS idx_terminal_log_terminal_id ON terminal_log(terminal_id);
CREATE INDEX IF NOT EXISTS idx_terminal_log_created_at ON terminal_log(created_at);
CREATE INDEX IF NOT EXISTS idx_terminal_log_streaming ON terminal_log(terminal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_terminal_log_cleanup ON terminal_log(created_at);

CREATE TABLE git_event_new (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    terminal_id TEXT REFERENCES terminal(id),
    commit_hash TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_message TEXT NOT NULL,
    metadata TEXT,
    process_status TEXT NOT NULL DEFAULT (lower('PENDING')),
    agent_response TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at TEXT
);

INSERT INTO git_event_new (
    id,
    workflow_id,
    terminal_id,
    commit_hash,
    branch,
    commit_message,
    metadata,
    process_status,
    agent_response,
    created_at,
    processed_at
)
SELECT
    id,
    workflow_id,
    terminal_id,
    commit_hash,
    branch,
    commit_message,
    metadata,
    process_status,
    agent_response,
    created_at,
    processed_at
FROM git_event;

DROP TABLE git_event;
ALTER TABLE git_event_new RENAME TO git_event;

CREATE INDEX IF NOT EXISTS idx_git_event_workflow_id ON git_event(workflow_id);
CREATE INDEX IF NOT EXISTS idx_git_event_terminal_id ON git_event(terminal_id);
CREATE INDEX IF NOT EXISTS idx_git_event_process_status ON git_event(process_status);
CREATE INDEX IF NOT EXISTS idx_git_event_workflow_status
ON git_event(workflow_id, process_status, created_at)
WHERE process_status IN ('pending', 'processing');
CREATE INDEX IF NOT EXISTS idx_git_event_terminal_status
ON git_event(terminal_id, process_status, created_at)
WHERE process_status = 'pending';
CREATE INDEX IF NOT EXISTS idx_git_event_cleanup
ON git_event(workflow_id, processed_at)
WHERE process_status = 'processed' AND processed_at IS NOT NULL;

PRAGMA foreign_keys = ON;
