-- Wave-2 finding W2-38-02: This migration is a breaking schema change (drops
-- shared_activity_cursors / shared_tasks, re-types shared_task_id as BLOB) and
-- does NOT provide a backward-compatibility view. Any external consumer that
-- reads these tables/columns directly must be updated BEFORE applying this
-- migration. Operator action for existing databases: ensure no external
-- readers depend on the dropped tables or the prior TEXT `shared_task_id`
-- column. Migration body is not modified because this migration has already
-- been applied in deployed environments.
DROP TABLE IF EXISTS shared_activity_cursors;

-- Drop the index on the old column if it exists
DROP INDEX IF EXISTS idx_tasks_shared_task_unique;

-- Add new column to hold the data
ALTER TABLE tasks ADD COLUMN shared_task_id_new BLOB;

-- Migrate data
-- NOTE: This UPDATE intentionally affects all rows as part of the migration to copy shared_task_id to the new column.
UPDATE tasks SET shared_task_id_new = shared_task_id WHERE 1=1;  -- Explicit WHERE clause to indicate intentional full-table update

-- Drop the old column (removing the foreign key constraint)
ALTER TABLE tasks DROP COLUMN shared_task_id;

-- Rename the new column to the old name
ALTER TABLE tasks RENAME COLUMN shared_task_id_new TO shared_task_id;

-- Recreate the index
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_shared_task_unique
    ON tasks(shared_task_id)
    WHERE shared_task_id IS NOT NULL;

DROP TABLE IF EXISTS shared_tasks;