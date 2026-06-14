-- Phase B2: persist the acceptance review SCORE on the workflow task.
-- The 5-dimension acceptance review (total_score/100) was previously parsed and
-- discarded (no DB write, no DTO, no WS event). These nullable, forward-only
-- columns let handle_acceptance_review_result record the score so it can be
-- surfaced/audited per task. All nullable, no backfill.
ALTER TABLE workflow_task ADD COLUMN acceptance_score REAL;
ALTER TABLE workflow_task ADD COLUMN acceptance_dimensions_json TEXT;
ALTER TABLE workflow_task ADD COLUMN acceptance_verdict TEXT;
ALTER TABLE workflow_task ADD COLUMN acceptance_reviewed_at DATETIME;
