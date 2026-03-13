# Phase 29: Quality Gate System Design

> 质量门系统设计文档 — 描述架构、降级矩阵与回滚方案

## 1. Overview

GitCortex Phase 29 introduces a built-in quality gate engine that automatically verifies code quality before allowing terminal handoff. Instead of terminals completing and immediately proceeding, code goes through: **checkpoint → quality analysis → pass/fail → feedback loop**.

The system defaults to `shadow` mode (observe only) and can be progressively tightened to `enforce` mode.

## 2. Three-Layer Gate Architecture

| Gate | Trigger | Scope | Checks |
|------|---------|-------|--------|
| **Terminal Gate** | Each checkpoint commit | Changed files | cargo check, clippy errors, tsc errors, test failures |
| **Branch Gate** | Last terminal in task passes | Full task branch | All terminal checks + clippy warnings, fmt, eslint |
| **Repo Gate** | Before merge to main / CI | Full repository | All branch checks + SonarQube analysis, security audit |

Each gate tier is independently configurable in `quality/quality-gate.yaml` with its own set of metric conditions.

## 3. Execution Flow

```
Terminal working
    ↓ (git commit with status: checkpoint)
GitWatcher detects commit
    ↓
Orchestrator intercepts checkpoint
    ↓
QualityEngine::run(config, working_dir, tier)
    ↓
┌─── passed ──→ Terminal promoted to completed → next terminal / review / merge
│
└─── failed ──→ Fix instructions injected to original terminal via PTY stdin
                    ↓
                Terminal fixes issues → re-commits checkpoint → cycle repeats
```

Key design decision: **failed quality does NOT create a new fixer terminal**. The original terminal retains full task context and continues fixing.

## 4. Quality Engine (`crates/quality/`)

### Module Structure

```
crates/quality/src/
├── lib.rs              # Public API
├── engine.rs           # Main entry: load config → run providers → evaluate gate
├── config.rs           # YAML config deserialization
├── report.rs           # Unified QualityReport model
├── sarif.rs            # SARIF-compatible issue format
├── provider/
│   ├── mod.rs          # QualityProvider trait
│   ├── rust_analyzer.rs  # cargo check, clippy, fmt, test
│   ├── frontend.rs     # eslint, tsc, vitest
│   ├── repo.rs         # generate-types:check, prepare-db:check
│   ├── sonar.rs        # SonarQube/SonarCloud API
│   └── security.rs     # Security audit
└── gate/
    ├── mod.rs
    ├── evaluator.rs    # Evaluate conditions against metrics
    └── condition.rs    # Metric comparison operators
```

### Provider Contract

Each provider implements:
```rust
#[async_trait]
pub trait QualityProvider {
    fn name(&self) -> &str;
    async fn analyze(&self, working_dir: &Path) -> Result<ProviderReport>;
}
```

Providers run independently. A single provider failure does not block others.

## 5. Configuration

### `quality/quality-gate.yaml`

```yaml
mode: shadow  # off | shadow | warn | enforce

terminal_gate:
  conditions:
    - metric: cargo_check_errors
      operator: "GT"
      threshold: "0"

branch_gate:
  conditions:
    # All terminal checks + clippy_warnings, fmt_violations, eslint_errors

repo_gate:
  conditions:
    # All branch checks + sonar_blocker_issues, sonar_critical_issues, security_issues

providers:
  rust: true
  frontend: true
  repo: true
  security: true
  sonar: true

sonar:
  host_url: "http://localhost:9000"
  project_key: "gitcortex"
```

### Operators

`GT` (greater than), `GTE`, `LT`, `LTE`, `EQ` — applied as: if `metric <op> threshold` then **fail**.

## 6. Orchestrator Integration

### `agent.rs` Key Functions

- `handle_checkpoint_quality_gate(terminal_id, commit)`: Intercepts checkpoint commits, runs quality engine, publishes result
- `handle_quality_gate_result(terminal_id, result)`: Routes pass → completed, fail → inject fix instructions

### Mode Behavior in Orchestrator

| Mode | On Checkpoint | On Fail | On Pass |
|------|--------------|---------|---------|
| `off` | Promote to completed immediately | N/A | N/A |
| `shadow` | Run analysis, log result, promote to completed | Log only | Log only |
| `warn` | Run analysis, publish to UI, promote to completed | Notify UI | Notify UI |
| `enforce` | Run analysis, block until pass | Inject fix instructions | Promote to completed |

### Message Bus Events

- `BusMessage::TerminalQualityGateResult { terminal_id, passed, summary, issues }` — Published after quality engine completes
- Routed to WebSocket as `WsEventType::QualityGateResult` → frontend event `quality.gate_result`

## 7. Degradation Matrix

> 故障降级矩阵 — 各种异常场景下的系统行为

| Scenario | Behavior | Severity |
|----------|----------|----------|
| Mode = `off` | Skip quality gate entirely, old flow | Normal |
| Mode = `shadow` | Run analysis, log results, never block | Normal |
| Single provider timeout (5s) | Skip that provider, log warning, continue with others | Low |
| Single provider crash | Skip that provider, log error, continue with others | Low |
| All providers fail | Degrade to shadow behavior, log error, promote checkpoint | Medium |
| QualityEngine panic | Catch at orchestrator level, promote checkpoint, log error | Medium |
| DB write failure (quality_run) | Continue flow, log error (quality data lost but flow unblocked) | Medium |
| SonarQube unreachable | Skip sonar provider, local checks still run | Low |
| Config file missing | Use built-in defaults (terminal gate only, shadow mode) | Low |
| Config file malformed | Log parse error, fall back to `off` mode | Medium |
| Terminal PTY dead during fix injection | Mark terminal failed, normal failure path | High |
| Max quality retries exceeded | Promote checkpoint to completed, log warning | Medium |

## 8. Rollback Plan

> 回滚方案 — 如何快速禁用质量门

### Instant Disable

```yaml
# quality/quality-gate.yaml
mode: off
```

Or via environment variable (no file change needed):
```bash
export QUALITY_GATE_MODE=off
```

### Rollback Scope

- **In-progress workflows**: Mode change takes effect on the next checkpoint commit. Terminals currently in a quality loop will complete their current cycle.
- **Database**: No migration rollback needed. Quality tables (`quality_run`, `quality_issue`) are additive and do not affect existing tables.
- **CI**: `ci-quality.yml` uses `continue-on-error: true`, so CI never blocks on quality gate failures.
- **Frontend**: Quality badges gracefully handle missing data (show "skipped" state).

### Progressive Rollback

1. `enforce` → `warn`: Stop blocking, keep notifications
2. `warn` → `shadow`: Stop notifications, keep logging
3. `shadow` → `off`: Stop all quality analysis

## 9. API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/terminals/:id/quality/latest` | GET | Latest quality run for a terminal |
| `/api/workflows/:id/quality/runs` | GET | All quality runs for a workflow |
| `/api/quality/runs/:id` | GET | Quality run detail with issues |

WebSocket event `quality.gate_result` is pushed to all workflow subscribers when a quality run completes.

## 10. Frontend Integration

| Component/Hook | Purpose |
|----------------|---------|
| `useTerminalLatestQuality` | Fetch latest quality run for terminal badge |
| `useWorkflowQualityRuns` | Fetch all quality runs for workflow report |
| `QualityBadge` | Presentational badge (passed/failed/warning/skipped) |
| `QualityReportPanel` | Detailed issue breakdown panel |
| `wsStore` handler | Invalidates React Query cache on `quality.gate_result` |
