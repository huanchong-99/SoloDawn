-- Requirement ledger: one row per acceptance point (评分点), project-scoped.
-- Each point is an independently verifiable acceptance criterion. When a point
-- is delivered, the acceptance review writes a compressed context capsule
-- (what was built / where it lives / decisions & gotchas / extension notes)
-- plus provenance so later rounds can start from the ledger instead of
-- re-understanding the whole project.
CREATE TABLE IF NOT EXISTS requirement_item (
    id TEXT PRIMARY KEY,
    project_id BLOB NOT NULL,
    -- Stable point id rendered into the rubric text (e.g. "RP-001"),
    -- unique per project so the scoring LLM can reference it.
    point_code TEXT NOT NULL,
    -- The requirement / acceptance criterion text.
    text TEXT NOT NULL,
    -- pending | delivered | regressed
    status TEXT NOT NULL DEFAULT 'pending',
    -- Planning draft (round) that introduced this point.
    origin_draft_id TEXT,
    -- Compressed context capsule written at scoring time (JSON).
    context_capsule TEXT,
    -- Workflow that delivered this point.
    provenance_workflow_id TEXT,
    -- Free-form commit range / diff stat recorded at delivery time.
    provenance_commits TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    delivered_at DATETIME,
    UNIQUE(project_id, point_code)
);

CREATE INDEX IF NOT EXISTS idx_requirement_item_project ON requirement_item(project_id, status);
