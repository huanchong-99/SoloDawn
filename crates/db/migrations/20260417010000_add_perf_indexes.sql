-- Performance indexes for TODO-closure items that remain full-snapshot queries.
CREATE INDEX IF NOT EXISTS idx_exec_proc_status_running
ON execution_processes(status)
WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_tasks_shared_task_id
ON tasks(shared_task_id)
WHERE shared_task_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_concierge_session_updated_at
ON concierge_session(updated_at DESC);
