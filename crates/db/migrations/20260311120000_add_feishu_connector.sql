-- Feishu app configuration table for connector integration
CREATE TABLE IF NOT EXISTS feishu_app_config (
    id TEXT PRIMARY KEY NOT NULL,
    app_id TEXT NOT NULL,
    app_secret_encrypted TEXT NOT NULL,
    tenant_key TEXT,
    base_url TEXT NOT NULL DEFAULT 'https://open.feishu.cn',
    enabled INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
