-- Add migration script here

-- Wave-2 finding W2-38-01: This migration permanently drops the `stdout` and
-- `stderr` columns from `execution_processes`, resulting in data loss with no
-- built-in backup. For existing databases where historical process output is
-- valuable, operators MUST back up the table (e.g. `CREATE TABLE
-- execution_processes_backup AS SELECT * FROM execution_processes;`) or export
-- the columns to a file BEFORE applying this migration. Migration body is not
-- modified because this migration has already been applied in deployed
-- environments.
ALTER TABLE execution_processes DROP COLUMN stdout;
ALTER TABLE execution_processes DROP COLUMN stderr;