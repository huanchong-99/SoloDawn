-- Planning draft layer for orchestrated workspace mode.
-- Stores the structured requirement discovery conversation
-- and the resulting technical specification before workflow materialization.

CREATE TABLE IF NOT EXISTS planning_draft (
    id                  TEXT PRIMARY KEY,
    project_id          BLOB NOT NULL,
    name                TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'gathering',
    -- user-facing requirement summary (plain language)
    requirement_summary TEXT,
    -- structured technical spec (JSON)
    technical_spec      TEXT,
    -- candidate workflow seed config (JSON, filled by planner)
    workflow_seed       TEXT,
    -- which orchestrator model to use for planning
    planner_model_id    TEXT,
    planner_api_type    TEXT,
    planner_base_url    TEXT,
    planner_api_key     TEXT,
    confirmed_at        TEXT,
    materialized_workflow_id TEXT REFERENCES workflow(id) ON DELETE SET NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_planning_draft_project
    ON planning_draft(project_id);

CREATE INDEX IF NOT EXISTS idx_planning_draft_status
    ON planning_draft(status);

CREATE TABLE IF NOT EXISTS planning_draft_message (
    id              TEXT PRIMARY KEY,
    draft_id        TEXT NOT NULL REFERENCES planning_draft(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_planning_draft_message_draft_created
    ON planning_draft_message(draft_id, created_at);
