# Quality Gate User Guide

This guide explains how to enable, configure, and use SoloDawn's built-in quality gate system.

## Enabling the Quality Gate

The quality gate is controlled by the `mode` field in `quality/quality-gate.yaml`:

```yaml
mode: shadow  # off | shadow | warn | enforce
```

You can also override via environment variable:

```bash
# Windows PowerShell
$env:QUALITY_GATE_MODE="enforce"

# Linux/macOS
export QUALITY_GATE_MODE=enforce
```

### Mode Reference

| Mode | Analysis Runs | Results in UI | Blocks Terminal | Blocks Merge |
|------|:---:|:---:|:---:|:---:|
| `off` | No | No | No | No |
| `shadow` | Yes | Yes (logged) | No | No |
| `warn` | Yes | Yes | No | No |
| `enforce` | Yes | Yes | Yes | Yes |

**Recommendation**: Start with `shadow` to observe results, then switch to `warn` or `enforce` once thresholds are tuned.

## Viewing Quality Reports

### In the Web UI

1. Open a workflow page
2. Terminal cards show a quality badge (passed/failed/warning/skipped)
3. Click the badge or open the Quality panel to see detailed issue breakdown:
   - Issue count by severity (critical, major, minor)
   - Issue list with file path, line number, and message
   - Provider breakdown (Rust, Frontend, Repo, Sonar, Security)

### Via API

```bash
# Latest quality run for a terminal
curl http://localhost:23456/api/terminals/<terminal_id>/quality/latest

# All quality runs for a workflow
curl http://localhost:23456/api/workflows/<workflow_id>/quality/runs

# Detailed run with issues
curl http://localhost:23456/api/quality/runs/<run_id>
```

### Via WebSocket

Subscribe to workflow events. Quality results arrive as:
```json
{
  "type": "quality.gate_result",
  "payload": {
    "terminalId": "...",
    "status": "passed|failed",
    "summary": { "critical": 0, "major": 2, "minor": 5 }
  }
}
```

## Handling Quality Failures

When a terminal fails the quality gate (in `warn` or `enforce` mode):

1. The orchestrator sends structured fix instructions to the terminal via PTY stdin
2. Instructions include file paths, line numbers, and issue descriptions
3. The terminal fixes the issues and commits again (another checkpoint)
4. The quality gate re-runs automatically
5. This cycle repeats until the gate passes or max retries are reached

**You do not need to intervene** — the orchestrator handles the feedback loop automatically.

### If a Terminal Gets Stuck

If a terminal is stuck in a quality loop (repeatedly failing):

1. Check the quality report in the UI for the specific issues
2. Consider switching to `shadow` mode temporarily:
   ```yaml
   mode: shadow
   ```
3. Or disable a specific provider:
   ```yaml
   providers:
     sonar: false  # Disable SonarQube checks
   ```

## Configuring Quality Thresholds

Edit `quality/quality-gate.yaml` to adjust what triggers a failure.

### Terminal Gate (fast checks per commit)

```yaml
terminal_gate:
  conditions:
    - metric: cargo_check_errors
      operator: "GT"
      threshold: "0"
    - metric: clippy_errors
      operator: "GT"
      threshold: "0"
    - metric: tsc_errors
      operator: "GT"
      threshold: "0"
```

### Branch Gate (full task branch)

Includes all terminal gate checks plus:
- `clippy_warnings` — Clippy warnings (not just errors)
- `fmt_violations` — `cargo fmt` violations
- `eslint_errors` — ESLint errors

### Repo Gate (before merge)

Includes all branch gate checks plus:
- `sonar_blocker_issues` — SonarQube blocker-level issues
- `sonar_critical_issues` — SonarQube critical-level issues
- `security_issues` — Security audit findings

### Operators

| Operator | Meaning |
|----------|---------|
| `GT` | Greater than (fail if metric > threshold) |
| `GTE` | Greater than or equal |
| `LT` | Less than |
| `LTE` | Less than or equal |
| `EQ` | Equal |

## Enabling SonarQube

SonarQube provides deep static analysis beyond what local tools catch.

1. Start SonarQube:
   ```bash
   docker run -d --name sonarqube -p 9000:9000 sonarqube:community
   ```

2. Create project at `http://localhost:9000`, generate a token

3. Configure:
   ```bash
   export SONAR_TOKEN=<your-token>
   ```

4. Verify in `quality/quality-gate.yaml`:
   ```yaml
   providers:
     sonar: true
   sonar:
     host_url: "http://localhost:9000"
     project_key: "solodawn"
   ```

5. Run manually:
   ```bash
   pnpm run quality:sonar
   ```

## Running Quality Gate in CI

The `ci-quality.yml` workflow runs automatically on push/PR to `main`:

- **sonar-analysis** job: Runs SonarCloud scanner
- **quality-gate** job: Runs the full quality engine (`scripts/quality/run-quality-gate.sh repo shadow`)

Both jobs use `continue-on-error: true` (shadow mode in CI). To make CI blocking, remove `continue-on-error` and set mode to `enforce`.
