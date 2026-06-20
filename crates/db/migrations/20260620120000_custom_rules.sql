-- AI-editable quality-gate rules: persistence foundation (PRD docs/quality §9).
--
-- ADDITIVE ONLY. Pure CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS.
-- NO table rebuild and NO `PRAGMA foreign_keys` toggle: sqlx 0.8.6 wraps every
-- migration in an implicit transaction where a bare `PRAGMA foreign_keys=OFF`
-- is a silent no-op, so toggling it here would be both useless and a hazard.
--
-- Every project-scoped FK column is BLOB to match projects.id BLOB PRIMARY KEY
-- (init.sql:6). Declaring it TEXT against a BLOB parent silently breaks the
-- FOREIGN KEY -- the exact class of bug fixed by 20260202090000.
--
-- D4 scope note: project_id is kept NULLABLE (NULL = global/org rule) so global
-- scope is a later additive step; the v1 UI/feature enforces non-null at the
-- route/handler layer, NOT at the column, so no schema change is needed later.

-- Editable, AI-authored, human-confirmed declarative rule (data, never code).
CREATE TABLE IF NOT EXISTS custom_rule (
    id            BLOB PRIMARY KEY NOT NULL,
    project_id    BLOB REFERENCES projects(id) ON DELETE CASCADE,  -- NULL = global/org rule (schema-allowed; v1 UI requires non-null, D4)
    name          TEXT NOT NULL,
    nl_request    TEXT NOT NULL,                                   -- original NL ask (round-trip compare + reproducibility)
    rule_format   TEXT NOT NULL CHECK (rule_format IN ('ast_grep','regex')),  -- P1 emits 'regex'; 'ast_grep' is P2 (D5)
    rule_body     TEXT NOT NULL,                                   -- regex+scope JSON (P1) or ast-grep YAML (P2)
    description   TEXT,                                            -- LLM-generated text powering the '!' tooltip
    rule_type     TEXT NOT NULL DEFAULT 'CodeSmell'
                     CHECK (rule_type IN ('Bug','Vulnerability','CodeSmell','SecurityHotspot')),
    severity      TEXT NOT NULL DEFAULT 'MAJOR'
                     CHECK (severity IN ('INFO','MINOR','MAJOR','CRITICAL','BLOCKER')),
    mapped_metric TEXT,                                            -- MetricKey::as_str() token; free text, NOT an FK
    enabled       INTEGER NOT NULL DEFAULT 1,
    status        TEXT NOT NULL DEFAULT 'shadow'
                     CHECK (status IN ('draft','shadow','warn','enforce','disabled')),
    created_by    TEXT,
    version       INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL DEFAULT (datetime('now','subsec')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now','subsec')),
    UNIQUE(project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_custom_rule_project ON custom_rule(project_id);
CREATE INDEX IF NOT EXISTS idx_custom_rule_enabled ON custom_rule(project_id, enabled);
CREATE INDEX IF NOT EXISTS idx_custom_rule_metric  ON custom_rule(mapped_metric);

-- Correctness oracle: positive (SHOULD flag) / negative (MUST NOT flag) snippets.
CREATE TABLE IF NOT EXISTS custom_rule_example (
    id             BLOB PRIMARY KEY NOT NULL,
    rule_id        BLOB NOT NULL REFERENCES custom_rule(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('positive','negative')),  -- positive SHOULD flag; negative MUST NOT
    language       TEXT,                                                    -- 'rust','typescript', NULL = agnostic
    snippet        TEXT NOT NULL,
    expected_match INTEGER NOT NULL,                                        -- 1 = rule expected to fire
    note           TEXT,
    created_at     TEXT NOT NULL DEFAULT (datetime('now','subsec'))
);

CREATE INDEX IF NOT EXISTS idx_custom_rule_example_rule ON custom_rule_example(rule_id, kind);

-- Authoring-time validation artifact ONLY (do NOT conflate with quality_run/quality_issue).
CREATE TABLE IF NOT EXISTS custom_rule_validation (
    id              BLOB PRIMARY KEY NOT NULL,
    rule_id         BLOB NOT NULL REFERENCES custom_rule(id) ON DELETE CASCADE,
    rule_version    INTEGER NOT NULL,
    verdict         TEXT NOT NULL CHECK (verdict IN ('pass','fail','error','pending')),
    roundtrip_ok    INTEGER,                                  -- judge verdict on reconstructed-NL vs original (NULL until run)
    judge_score     REAL,                                     -- AuditScoreResult-style total
    examples_total  INTEGER NOT NULL DEFAULT 0,
    examples_passed INTEGER NOT NULL DEFAULT 0,
    rounds_used     INTEGER NOT NULL DEFAULT 0,
    results_json    TEXT,                                     -- per-example {example_id, expected, actual, matched_spans}; + adversary transcript
    error_message   TEXT,
    validated_by    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now','subsec'))
);

CREATE INDEX IF NOT EXISTS idx_custom_rule_validation_rule ON custom_rule_validation(rule_id, created_at DESC);

-- Append-only audit log. NEVER UPDATEd -> never needs a rebuild. Intentionally
-- FK-LESS so history survives rule deletion (rule_id/project_id are bare BLOBs).
CREATE TABLE IF NOT EXISTS custom_rule_audit (
    id           BLOB PRIMARY KEY NOT NULL,
    rule_id      BLOB NOT NULL,
    project_id   BLOB,
    action       TEXT NOT NULL
                    CHECK (action IN ('create','update','enable','disable','delete','revalidate','promote')),
    actor        TEXT,
    from_version INTEGER,
    to_version   INTEGER,
    diff_json    TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now','subsec'))
);

CREATE INDEX IF NOT EXISTS idx_custom_rule_audit_rule ON custom_rule_audit(rule_id, created_at DESC);
