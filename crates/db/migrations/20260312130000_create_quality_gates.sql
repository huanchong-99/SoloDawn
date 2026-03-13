PRAGMA foreign_keys = ON;

CREATE TABLE quality_runs (
    id            BLOB PRIMARY KEY,
    project_id    BLOB NOT NULL,
    workflow_id   BLOB,
    task_id       BLOB,
    terminal_id   BLOB,
    level         TEXT NOT NULL     -- 'terminal', 'branch', 'repo'
                  CHECK (level IN ('terminal','branch','repo')),
    status        TEXT NOT NULL     -- 'pending', 'running', 'passed', 'failed', 'error'
                  CHECK (status IN ('pending','running','passed','failed','error')),
    mode          TEXT NOT NULL     -- 'off', 'shadow', 'warn', 'enforce'
                  CHECK (mode IN ('off','shadow','warn','enforce')),
    gate_name     TEXT NOT NULL,
    duration_ms   INTEGER,
    summary       TEXT,             -- JSON summary of results
    created_at    TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (terminal_id) REFERENCES terminals(id) ON DELETE CASCADE
);

CREATE TABLE quality_issues (
    id            BLOB PRIMARY KEY,
    run_id        BLOB NOT NULL,
    provider      TEXT NOT NULL,
    rule_id       TEXT NOT NULL,
    severity      TEXT NOT NULL
                  CHECK (severity IN ('info','minor','major','critical','blocker')),
    message       TEXT NOT NULL,
    file_path     TEXT,
    line_start    INTEGER,
    line_end      INTEGER,
    column_start  INTEGER,
    column_end    INTEGER,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    FOREIGN KEY (run_id) REFERENCES quality_runs(id) ON DELETE CASCADE
);
