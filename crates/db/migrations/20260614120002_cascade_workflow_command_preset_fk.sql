PRAGMA foreign_keys = OFF;

-- Close sqlx's implicit transaction so PRAGMA foreign_keys takes effect.
-- PRAGMA foreign_keys is a no-op inside a transaction in SQLite, so the
-- table rebuild below must run outside sqlx's implicit transaction.
-- sqlx workaround until `-- no-transaction` lands in sqlx-sqlite:
-- https://github.com/launchbadge/sqlx/issues/2085#issuecomment-1499859906
COMMIT TRANSACTION;
BEGIN TRANSACTION;

CREATE TABLE workflow_command_new (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
    preset_id       TEXT NOT NULL REFERENCES slash_command_preset(id) ON DELETE CASCADE,
    order_index     INTEGER NOT NULL,
    custom_params   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(workflow_id, order_index)
);

INSERT INTO workflow_command_new
SELECT id, workflow_id, preset_id, order_index, custom_params, created_at
FROM workflow_command;

DROP TABLE workflow_command;
ALTER TABLE workflow_command_new RENAME TO workflow_command;

CREATE INDEX IF NOT EXISTS idx_workflow_command_workflow_id ON workflow_command(workflow_id);

-- Verify FK integrity before committing.
PRAGMA foreign_key_check;

COMMIT;

PRAGMA foreign_keys = ON;

-- Leave an open transaction for sqlx to close.
BEGIN TRANSACTION;
