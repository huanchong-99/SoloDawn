# Offline / Restricted Network Fallback Mode

When SonarQube is unavailable (network restrictions, air-gapped environments, or service outage), the SoloDawn quality gate automatically falls back to local-only analysis.

## How Fallback Works

The quality gate runs a provider chain. Each provider is checked independently:

1. **Local analyzers** (always available): `clippy`, `eslint`, `tsc`
2. **SonarQube** (requires network): static analysis, duplication, coverage

If the Sonar provider fails to connect or times out, the quality gate still produces a result using local analyzers only. The final report will note that Sonar results are missing.

## Configuration

### Disable Sonar explicitly

In `quality-gate.yaml` (or equivalent config):

```yaml
providers:
  sonar:
    enabled: false
  clippy:
    enabled: true
  eslint:
    enabled: true
  tsc:
    enabled: true
```

### Environment variable override

```bash
# Docker compose: set in .env or inline
SONAR_HOST_URL=          # Empty string disables Sonar provider
QUALITY_GATE_MODE=shadow # "shadow" logs results without blocking
```

## What Works Offline

| Analyzer | Offline | Notes |
|----------|---------|-------|
| clippy | Yes | Rust compiler-based, fully local |
| eslint | Yes | Node-based, uses local config |
| tsc | Yes | TypeScript compiler, local only |
| SonarQube | No | Requires running Sonar server |

## Quality Results Storage

Even in offline mode, quality results are:

- Stored in the local SQLite database (`quality_run` table)
- Visible in the SoloDawn UI under the workflow quality tab
- Available via the API: `GET /api/quality/runs`

The only difference is that Sonar-specific metrics (duplication %, coverage %, security hotspots) will be absent from the report.

## Recommended Offline Workflow

1. Set `providers.sonar.enabled: false` in config
2. Run quality gate normally — local analyzers execute
3. Review results in UI or API
4. When Sonar becomes available again, re-enable and run a full scan to backfill Sonar metrics

## Docker Compose Without Sonar

To run SoloDawn without the SonarQube stack, start only the solodawn service:

```bash
docker compose -f docker/compose/docker-compose.yml up solodawn
```

Or remove the `depends_on.sonarqube` block from the compose file and set `SONAR_HOST_URL=` empty.
