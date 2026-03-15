# Documentation Layout

This repository uses two top-level documentation folders:

- `docs/developed/`: completed and stable documentation
- `docs/undeveloped/`: pending or in-progress documentation

## Current `docs/undeveloped/` structure

- `docs/undeveloped/current/`: active unfinished work
  - `TODO.md` — single source of truth for all active unfinished tasks (only minor stubs remain)

## Current `docs/developed/` structure

- `docs/developed/plans/`: completed phase plans and design documents (Phase 0-29 + BACKLOG-002/003)
- `docs/developed/issues/`: resolved audit reports, SonarCloud reports, and issue analyses
- `docs/developed/ops/`: operations runbook, troubleshooting, deployment guides
- `docs/developed/misc/`: archived TODO lists, user guide, operations manual, and other reference docs

## Maintenance rules

1. Add new work-in-progress docs under `docs/undeveloped/current/`.
2. Move docs to `docs/developed/` after work is complete.
3. Keep `docs/undeveloped/current/TODO.md` as the single source of truth for active unfinished tasks.
4. Avoid duplicate documents across `developed/` and `undeveloped/`.

## Quality Gate Documentation

Phase 29 added a built-in quality gate system. Related documentation:

- `docs/developed/plans/2026-03-13-phase-29-quality-gate-design.md`: Quality gate architecture, degradation matrix, and rollback plan
- `docs/developed/ops/runbook.md` → "Quality Gate Operations" section: Configuration, SonarQube setup, manual runs, data cleanup
- `docs/developed/ops/troubleshooting.md` → "Quality Gate Issues" section: Diagnosis and fixes
- `quality/quality-gate.yaml`: Gate configuration (modes, tiers, conditions, providers)
- `quality/sonar/sonar-project.properties`: SonarQube project settings
- `scripts/quality/`: Runner and setup scripts (`.sh` + `.ps1` pairs)
