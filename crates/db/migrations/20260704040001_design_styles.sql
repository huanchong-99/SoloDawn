-- Design style templates: reusable visual-direction prompts injected into
-- planner goals so UI work follows a chosen aesthetic. Builtin presets
-- (adapted from high-star open-source design skills, see LICENSE) are seeded
-- at startup from crates/services/assets/design_styles/; users add their own
-- via the design-styles API.
CREATE TABLE IF NOT EXISTS design_style (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- The design directive prompt injected for UI-related work.
    content TEXT NOT NULL,
    -- Attribution for builtin presets (NULL for user-created styles).
    source_name TEXT,
    source_url TEXT,
    license TEXT,
    -- Builtin styles cannot be deleted or content-edited, only disabled;
    -- users duplicate them into custom styles to modify.
    builtin INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Per-round style selection: which design style this draft's workflow should
-- follow. NULL falls back to the system default (system_settings key
-- 'default_design_style'), then to none. Additive and nullable.
ALTER TABLE planning_draft ADD COLUMN design_style_slug TEXT;
