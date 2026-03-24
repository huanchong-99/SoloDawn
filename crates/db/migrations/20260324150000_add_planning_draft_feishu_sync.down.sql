-- SQLite does not support DROP COLUMN in older versions, so we recreate the table.
CREATE TABLE planning_draft_backup AS SELECT
    id, project_id, name, status,
    requirement_summary, technical_spec, workflow_seed,
    planner_model_id, planner_api_type, planner_base_url, planner_api_key,
    confirmed_at, materialized_workflow_id,
    created_at, updated_at
FROM planning_draft;

DROP TABLE planning_draft;

ALTER TABLE planning_draft_backup RENAME TO planning_draft;
