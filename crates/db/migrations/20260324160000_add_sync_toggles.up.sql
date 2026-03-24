-- Add independent sync toggles to concierge_session and planning_draft.
-- sync_tools: push tool call events to Feishu
-- sync_terminal: push terminal status changes to Feishu
-- sync_progress: push workflow progress events to Feishu
-- notify_on_completion: send completion report on workflow/task completion

ALTER TABLE concierge_session ADD COLUMN sync_tools INTEGER NOT NULL DEFAULT 0;
ALTER TABLE concierge_session ADD COLUMN sync_terminal INTEGER NOT NULL DEFAULT 0;
ALTER TABLE concierge_session ADD COLUMN sync_progress INTEGER NOT NULL DEFAULT 0;
ALTER TABLE concierge_session ADD COLUMN notify_on_completion INTEGER NOT NULL DEFAULT 1;

ALTER TABLE planning_draft ADD COLUMN sync_tools INTEGER NOT NULL DEFAULT 0;
ALTER TABLE planning_draft ADD COLUMN sync_terminal INTEGER NOT NULL DEFAULT 0;
ALTER TABLE planning_draft ADD COLUMN sync_progress INTEGER NOT NULL DEFAULT 0;
ALTER TABLE planning_draft ADD COLUMN notify_on_completion INTEGER NOT NULL DEFAULT 1;
