-- G32-008: Add UNIQUE constraint on app_id to prevent duplicate Feishu app
-- configurations. SQLite does not support ALTER TABLE ADD CONSTRAINT, so we
-- create a unique index instead.
CREATE UNIQUE INDEX IF NOT EXISTS idx_feishu_app_config_app_id
    ON feishu_app_config (app_id);
