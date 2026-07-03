-- Rounds: a follow-up planning draft is a child of the round it continues.
-- NULL for round 1 / standalone drafts. No backfill needed.
ALTER TABLE planning_draft ADD COLUMN parent_draft_id TEXT REFERENCES planning_draft(id);

CREATE INDEX IF NOT EXISTS idx_planning_draft_parent ON planning_draft(parent_draft_id);
