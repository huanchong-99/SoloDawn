-- G2: System-A quality-gate confirmation timestamp on the planning draft.
-- DISTINCT from the pre-existing confirmed_at (System-B audit confirm). Nullable, no backfill.
ALTER TABLE planning_draft ADD COLUMN gates_confirmed_at DATETIME;
