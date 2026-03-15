-- ============================================================================
-- CLI Installation History Migration
-- Created: 2026-03-15
-- Description: Add CLI install/uninstall tracking and detection cache tables
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. CLI Install History Table (cli_install_history)
-- Tracks individual CLI install/uninstall operations and their results.
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cli_install_history (
    id              TEXT PRIMARY KEY NOT NULL,
    cli_type_id     TEXT NOT NULL,
    action          TEXT NOT NULL CHECK(action IN ('install', 'uninstall')),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'success', 'failed', 'cancelled')),
    started_at      TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at    TEXT,
    exit_code       INTEGER,
    output          TEXT,
    error_message   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (cli_type_id) REFERENCES cli_type(id)
);

CREATE INDEX IF NOT EXISTS idx_cli_install_history_cli_type ON cli_install_history(cli_type_id);
CREATE INDEX IF NOT EXISTS idx_cli_install_history_status ON cli_install_history(status);

-- ----------------------------------------------------------------------------
-- 2. CLI Detection Cache Table (cli_detection_cache)
-- Caches CLI detection results for quick lookups.
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cli_detection_cache (
    cli_type_id     TEXT PRIMARY KEY NOT NULL,
    installed       INTEGER NOT NULL DEFAULT 0,
    version         TEXT,
    executable_path TEXT,
    detected_at     TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (cli_type_id) REFERENCES cli_type(id)
);
