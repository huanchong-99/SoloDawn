-- Fix session executor values that were incorrectly stored with variant suffix
-- Values like "CLAUDE_CODE:ROUTER" should be "CLAUDE_CODE"
-- This was introduced in the refactor from task_attempts to sessions (commit 6a129d0fa)
-- Optimized: Using instr() instead of LIKE to avoid full table scan warning
--
-- TODO(W2-38-06): Data coercion risk. `substr(executor, 1, instr(executor, ':') - 1)`
-- silently truncates every value with a ':' — including values that are
-- legitimately shaped like `key:value` but weren't caused by the original
-- bug. If a future executor name ever contains a ':' (e.g. a URL or a
-- namespaced id like `github.com:acme/claude-code`), this migration would
-- strip it to just the prefix. There is no backup of the pre-migration
-- values (no `executor_raw` snapshot column, no audit row). Recovery from a
-- mis-classification would require replaying from source-of-truth logs.
-- This migration is already applied — do NOT modify. For future repair
-- migrations that touch free-form TEXT: (a) add a `WHERE` clause that
-- enumerates the known bad values rather than matching on shape alone,
-- and (b) snapshot affected rows to a side table before UPDATE.
UPDATE sessions
SET executor = substr(executor, 1, instr(executor, ':') - 1),
    updated_at = datetime('now', 'subsec')
WHERE instr(executor, ':') > 0;
