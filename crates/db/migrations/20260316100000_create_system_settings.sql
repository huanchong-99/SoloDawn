-- system_settings: runtime key-value store for feature flags
CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL,
    description TEXT,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Seed default values
INSERT OR IGNORE INTO system_settings (key, value, description)
VALUES
    ('feishu_enabled', 'false', 'Enable Feishu/Lark integration'),
    ('setup_complete', 'false', 'Whether initial setup wizard has been completed');
