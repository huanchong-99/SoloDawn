PRAGMA foreign_keys = OFF;

CREATE TABLE git_event_new (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    terminal_id     TEXT REFERENCES terminal(id) ON DELETE SET NULL,
    commit_hash     TEXT NOT NULL,
    branch          TEXT NOT NULL,
    commit_message  TEXT NOT NULL,
    metadata        TEXT,
    process_status  TEXT NOT NULL DEFAULT (lower('PENDING')),
    agent_response  TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at    TEXT
);

-- Column order intentionally matches `git_event`, so a straight copy keeps
-- the rebuild concise while preserving every row.
INSERT INTO git_event_new
SELECT * FROM git_event;

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
