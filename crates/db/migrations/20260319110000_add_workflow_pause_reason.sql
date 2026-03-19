-- Add pause_reason to track why a workflow was paused (e.g., api_exhausted, user_requested)
ALTER TABLE workflow ADD COLUMN pause_reason TEXT;
