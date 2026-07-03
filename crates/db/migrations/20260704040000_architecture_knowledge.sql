-- Architecture knowledge base: GitHub-synced architecture guidance that the
-- planner injects into workflow goals (methodology checklist + per-system
-- template digests). Sources are configurable repos; the builtin source
-- (study8677/awesome-architecture, MIT) is seeded at startup, not here, so
-- the seed content lives next to the code that maintains it.
CREATE TABLE IF NOT EXISTS architecture_source (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    -- GitHub coordinates: https://github.com/{owner}/{repo}, tree {branch}.
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    -- JSON array of path prefixes to sync (e.g. ["templates/"]).
    include_paths TEXT NOT NULL DEFAULT '["templates/"]',
    enabled INTEGER NOT NULL DEFAULT 1,
    -- Builtin sources cannot be deleted, only disabled.
    builtin INTEGER NOT NULL DEFAULT 0,
    -- Git tree sha of the last successful sync; unchanged sha = skip walk.
    last_tree_sha TEXT,
    last_synced_at DATETIME,
    -- 'ok' or 'error: <message>' from the most recent sync attempt.
    last_sync_status TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(owner, repo, branch)
);

-- One row per synced markdown document. digest carries the prompt-injectable
-- extract (key decisions / bottlenecks / anti-patterns sections); content
-- keeps the full document so digests can be re-derived without refetching.
CREATE TABLE IF NOT EXISTS architecture_entry (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES architecture_source(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    -- template | tutorial | case | other (derived from the path prefix).
    category TEXT NOT NULL DEFAULT 'other',
    slug TEXT NOT NULL,
    title TEXT NOT NULL,
    -- JSON array of matching keywords extracted at sync time.
    keywords TEXT NOT NULL DEFAULT '[]',
    digest TEXT NOT NULL,
    content TEXT NOT NULL,
    blob_sha TEXT NOT NULL,
    synced_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(source_id, path)
);

CREATE INDEX IF NOT EXISTS idx_architecture_entry_source ON architecture_entry(source_id, category);
