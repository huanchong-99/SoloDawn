-- Add composite index for paginated execution process queries
CREATE INDEX IF NOT EXISTS idx_ep_session_created
    ON execution_processes(session_id, created_at DESC);
