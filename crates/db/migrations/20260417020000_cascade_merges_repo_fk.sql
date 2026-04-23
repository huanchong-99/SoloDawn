PRAGMA foreign_keys = OFF;

CREATE TABLE merges_new (
    id                  BLOB PRIMARY KEY,
    workspace_id        BLOB NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    merge_type          TEXT NOT NULL CHECK (merge_type IN ('direct', 'pr')),
    merge_commit        TEXT,
    pr_number           INTEGER,
    pr_url              TEXT,
    pr_status           TEXT CHECK (pr_status IN ('open', 'merged', 'closed')),
    pr_merged_at        TEXT,
    pr_merge_commit_sha TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
    target_branch_name  TEXT NOT NULL,
    repo_id             BLOB NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    CHECK (
        (merge_type <> 'pr' AND merge_commit IS NOT NULL
         AND pr_number IS NULL AND pr_url IS NULL)
        OR
        (merge_type = 'pr' AND pr_number IS NOT NULL AND pr_url IS NOT NULL
         AND pr_status IS NOT NULL AND merge_commit IS NULL)
    )
);

INSERT INTO merges_new (
    id,
    workspace_id,
    merge_type,
    merge_commit,
    pr_number,
    pr_url,
    pr_status,
    pr_merged_at,
    pr_merge_commit_sha,
    created_at,
    target_branch_name,
    repo_id
)
SELECT
    id,
    workspace_id,
    merge_type,
    merge_commit,
    pr_number,
    pr_url,
    pr_status,
    pr_merged_at,
    pr_merge_commit_sha,
    created_at,
    target_branch_name,
    repo_id
FROM merges;

DROP TABLE merges;
ALTER TABLE merges_new RENAME TO merges;

CREATE INDEX IF NOT EXISTS idx_merges_workspace_id ON merges(workspace_id);
CREATE INDEX IF NOT EXISTS idx_merges_open_pr ON merges(workspace_id, pr_status)
WHERE merge_type = 'pr' AND pr_status = 'open';
CREATE INDEX IF NOT EXISTS idx_merges_repo_id ON merges(repo_id);
CREATE INDEX IF NOT EXISTS idx_merges_type_status ON merges(merge_type, pr_status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_merges_unique_workspace_pr
ON merges(workspace_id, pr_number)
WHERE merge_type = 'pr' AND pr_number IS NOT NULL;

PRAGMA foreign_keys = ON;
