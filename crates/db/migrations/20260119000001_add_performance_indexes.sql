-- NOTE: SonarCloud flags duplicate string literals (e.g. status values like 'completed', 'failed', 'cancelled')
-- in this migration. This is acceptable for SQL index definitions where partial index WHERE clauses
-- necessarily repeat the same status enum values across different tables.

-- ============================================================================
-- SoloDawn Performance Indexes Migration
-- Created: 2026-01-19
-- Description: Add composite and partial indexes for workflow query optimization
-- ============================================================================

-- ----------------------------------------------------------------------------
-- Workflow Table Indexes
-- ----------------------------------------------------------------------------

-- Index for finding active workflows by project (most common query)
CREATE INDEX IF NOT EXISTS idx_workflow_project_status
ON workflow(project_id, status)
WHERE status IN ('created', 'ready', 'running');

-- Index for listing active workflows sorted by creation time
CREATE INDEX IF NOT EXISTS idx_workflow_active
ON workflow(status, created_at DESC)
WHERE status NOT IN ('completed', 'failed', 'cancelled');

-- Index for cleanup operations on completed workflows
CREATE INDEX IF NOT EXISTS idx_workflow_completed_cleanup
ON workflow(project_id, completed_at)
WHERE status IN ('completed', 'failed', 'cancelled') AND completed_at IS NOT NULL;

-- ----------------------------------------------------------------------------
-- Workflow Task Table Indexes
-- ----------------------------------------------------------------------------

-- Index for finding tasks by workflow with status filtering
CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_status
ON workflow_task(workflow_id, status, order_index);

-- Index for finding active tasks across all workflows
CREATE INDEX IF NOT EXISTS idx_workflow_task_active
ON workflow_task(status, created_at)
WHERE status IN ('pending', 'running', 'review_pending');

-- ----------------------------------------------------------------------------
-- Terminal Table Indexes
-- ----------------------------------------------------------------------------

-- Index for finding terminals by task with status filtering
CREATE INDEX IF NOT EXISTS idx_terminal_task_status
ON terminal(workflow_task_id, status, order_index);

-- Index for finding active terminals across all tasks
CREATE INDEX IF NOT EXISTS idx_terminal_active
ON terminal(status, started_at)
WHERE status IN ('starting', 'waiting', 'working');

-- Index for cleanup operations on completed terminals
CREATE INDEX IF NOT EXISTS idx_terminal_cleanup
ON terminal(workflow_task_id, completed_at)
WHERE status IN (lower('COMPLETED'), lower('FAILED'), lower('CANCELLED')) AND completed_at IS NOT NULL;

-- ----------------------------------------------------------------------------
-- Git Event Table Indexes
-- ----------------------------------------------------------------------------

-- Index for processing pending events by workflow
CREATE INDEX IF NOT EXISTS idx_git_event_workflow_status
ON git_event(workflow_id, process_status, created_at)
WHERE process_status IN ('pending', 'processing');

-- Index for processing pending events by terminal
CREATE INDEX IF NOT EXISTS idx_git_event_terminal_status
ON git_event(terminal_id, process_status, created_at)
WHERE process_status = lower('PENDING');

-- Index for cleanup of processed events
CREATE INDEX IF NOT EXISTS idx_git_event_cleanup
ON git_event(workflow_id, processed_at)
WHERE process_status = 'processed' AND processed_at IS NOT NULL;

-- ----------------------------------------------------------------------------
-- Terminal Log Table Indexes
-- ----------------------------------------------------------------------------

-- Composite index for streaming logs by terminal (sorted by time)
CREATE INDEX IF NOT EXISTS idx_terminal_log_streaming
ON terminal_log(terminal_id, created_at DESC);

-- Index for cleanup operations on logs (sorted by time)
CREATE INDEX IF NOT EXISTS idx_terminal_log_cleanup
ON terminal_log(created_at);

-- ----------------------------------------------------------------------------
-- Update Table Statistics
-- ----------------------------------------------------------------------------

-- Analyze all tables to update query planner statistics
ANALYZE workflow;
ANALYZE workflow_task;
ANALYZE workflow_command;
ANALYZE terminal;
ANALYZE terminal_log;
ANALYZE git_event;
ANALYZE cli_type;
ANALYZE model_config;
ANALYZE slash_command_preset;
