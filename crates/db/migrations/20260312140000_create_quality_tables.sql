-- Quality Gate persistence tables
-- Phase 29B: quality_run tracks each quality gate execution,
-- quality_issue stores individual issues found during analysis.

CREATE TABLE IF NOT EXISTS quality_run (
    id              TEXT PRIMARY KEY NOT NULL,
    workflow_id     TEXT NOT NULL,
    task_id         TEXT,
    terminal_id     TEXT,
    commit_hash     TEXT,
    gate_level      TEXT NOT NULL DEFAULT 'terminal',  -- terminal | branch | repo
    gate_status     TEXT NOT NULL DEFAULT 'pending',    -- pending | running | ok | warn | error | skipped
    mode            TEXT NOT NULL DEFAULT 'shadow',     -- off | shadow | warn | enforce
    total_issues    INTEGER NOT NULL DEFAULT 0,
    blocking_issues INTEGER NOT NULL DEFAULT 0,
    new_issues      INTEGER NOT NULL DEFAULT 0,
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    providers_run   TEXT,          -- JSON array of provider names, e.g. '["clippy","eslint"]'
    report_json     TEXT,          -- Full serialized QualityReport (nullable until complete)
    decision_json   TEXT,          -- Serialized QualityGateDecision
    error_message   TEXT,          -- If the run itself failed (scanner crash, timeout)
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    completed_at    DATETIME
);

CREATE INDEX IF NOT EXISTS idx_quality_run_workflow   ON quality_run(workflow_id);
CREATE INDEX IF NOT EXISTS idx_quality_run_task       ON quality_run(task_id);
CREATE INDEX IF NOT EXISTS idx_quality_run_terminal   ON quality_run(terminal_id);
CREATE INDEX IF NOT EXISTS idx_quality_run_status     ON quality_run(gate_status);

CREATE TABLE IF NOT EXISTS quality_issue (
    id              TEXT PRIMARY KEY NOT NULL,
    quality_run_id  TEXT NOT NULL,
    rule_id         TEXT NOT NULL,
    rule_type       TEXT NOT NULL DEFAULT 'CODE_SMELL',  -- BUG | VULNERABILITY | CODE_SMELL | SECURITY_HOTSPOT
    severity        TEXT NOT NULL DEFAULT 'MAJOR',       -- INFO | MINOR | MAJOR | CRITICAL | BLOCKER
    source          TEXT NOT NULL DEFAULT 'unknown',     -- clippy | cargo-check | eslint | sonarqube | ...
    message         TEXT NOT NULL,
    file_path       TEXT,
    line            INTEGER,
    end_line        INTEGER,
    column_start    INTEGER,
    column_end      INTEGER,
    is_new          BOOLEAN NOT NULL DEFAULT 1,
    is_blocking     BOOLEAN NOT NULL DEFAULT 0,
    effort_minutes  INTEGER,
    context         TEXT,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),

    FOREIGN KEY (quality_run_id) REFERENCES quality_run(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_quality_issue_run      ON quality_issue(quality_run_id);
CREATE INDEX IF NOT EXISTS idx_quality_issue_severity  ON quality_issue(severity);
CREATE INDEX IF NOT EXISTS idx_quality_issue_file      ON quality_issue(file_path);
