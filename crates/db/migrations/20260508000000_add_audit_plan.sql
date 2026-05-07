-- Add audit plan support to planning_draft and workflow tables
ALTER TABLE planning_draft ADD COLUMN audit_plan TEXT;
ALTER TABLE planning_draft ADD COLUMN audit_mode TEXT NOT NULL DEFAULT 'builtin';
ALTER TABLE planning_draft ADD COLUMN audit_doc_path TEXT;
ALTER TABLE workflow ADD COLUMN audit_plan TEXT;
