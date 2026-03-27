# SonarCloud Report Archival Strategy

## Overview

SonarCloud/SonarQube analysis results are stored both in the Sonar server and locally in SoloDawn's database. This document defines the archival strategy and naming conventions.

## Storage Locations

| Location | Data | Retention |
|----------|------|-----------|
| SonarCloud (cloud) | Full analysis history, issue tracking, trends | Managed by SonarCloud (unlimited for public projects) |
| SonarQube (self-hosted) | Full analysis history | Configurable in SonarQube admin (`sonar.dbcleaner.*`) |
| SoloDawn `quality_run` table | Run metadata, pass/fail, summary metrics | Manual cleanup (see below) |
| SoloDawn `quality_issue` table | Individual issues per run | Cascades with `quality_run` cleanup |
| CI artifacts | Scanner logs, SARIF exports | GitHub Actions default (90 days) |

## Naming Conventions

### SonarCloud/SonarQube Project Keys

```
solodawn                    # Main project
solodawn:branch-<name>      # Branch analysis (auto-managed by Sonar)
```

### Quality Profile Names

```
SoloDawn-Rust-v1            # Rust quality profile
SoloDawn-TypeScript-v1      # TypeScript quality profile
```

Profile exports are stored in `quality/sonar/profiles/` and synced via:
```bash
./scripts/quality/sync-quality-profile.sh
```

### CI Artifact Names

```
sonar-report-<branch>-<sha>  # SonarCloud analysis report
quality-gate-<branch>-<sha>  # Local quality gate results
```

## Database Cleanup

Quality run records accumulate over time. Recommended cleanup schedule:

### Manual Cleanup

```sql
-- Delete quality runs older than 30 days
DELETE FROM quality_run WHERE created_at < datetime('now', '-30 days');

-- Delete orphaned quality issues
DELETE FROM quality_issue WHERE run_id NOT IN (SELECT id FROM quality_run);

-- Check current record counts
SELECT COUNT(*) as runs FROM quality_run;
SELECT COUNT(*) as issues FROM quality_issue;
```

### Automated Cleanup (Future)

A scheduled cleanup job is planned but not yet implemented. For now, run the SQL manually or add it to your maintenance cron.

## SonarQube Server Maintenance

If running SonarQube self-hosted:

```bash
# Default cleanup settings (in SonarQube admin)
sonar.dbcleaner.daysBeforeDeletingClosedIssues=30
sonar.dbcleaner.weeksBeforeDeletingAllSnapshots=260  # ~5 years
sonar.dbcleaner.weeksBeforeKeepingOnlyOneSnapshotByWeek=4
sonar.dbcleaner.weeksBeforeKeepingOnlyOneSnapshotByMonth=52
```

## Backup Strategy

1. **SonarCloud**: No backup needed (cloud-managed)
2. **SonarQube self-hosted**: Include in regular database backup rotation
3. **SoloDawn quality data**: Included in standard SQLite backup (`pnpm run prepare-db` backup procedures in runbook)
4. **Quality profiles**: Version-controlled in `quality/sonar/profiles/` — backed up with Git
