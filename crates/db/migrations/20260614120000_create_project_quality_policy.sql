-- G3: per-project System-A quality-gate policy override (priority-0 source of truth).
-- config_yaml is serde_yaml of quality::config::QualityGateConfig -- byte-identical to
-- quality/quality-gate.yaml form, round-trips via QualityGateConfig::from_yaml. Storing
-- opaque YAML keeps the db crate free of any quality types.
--
-- NOTE: project_id is declared BLOB (not TEXT) to match projects.id BLOB PRIMARY KEY.
-- Declaring it TEXT against a BLOB parent causes FOREIGN KEY constraint failures --
-- the exact bug fixed by 20260202090000_fix_workflow_project_id_type.sql. The P3 spec's
-- verbatim TEXT example predates that schema fact; BLOB is the schema-correct choice.
CREATE TABLE IF NOT EXISTS project_quality_policy (
    project_id   BLOB PRIMARY KEY NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    config_yaml  TEXT NOT NULL,
    mode         TEXT NOT NULL DEFAULT 'enforce',   -- [GRAFT-C] denormalized for list badges; config_yaml authoritative
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
