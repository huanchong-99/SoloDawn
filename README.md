<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
</p>

# GitCortex

AI orchestration layer for coordinating multiple coding CLIs (Claude Code, Codex, Gemini CLI, etc.) in one workflow.

## Why GitCortex

- One orchestrator agent schedules all terminals with a single state machine.
- Multi-task parallelism (across tasks) + serial quality gates (inside each task).
- Native CLI execution, so existing CLI slash commands / plugins / MCP / skills remain usable.
- Git-driven event loop for handoff, recovery, and traceable history.

## What's New (March 2026)

### Orchestrator and Chat

- Added workflow-level orchestrator chat pagination and query params (`cursor/limit`) (`ec8ad4ec2`).
- Added orchestrator message persistence and command snapshot persistence (`8ccf0f3d1`).
- Enforced instruction allowlist and command status flow (`1a1b153a3`).
- Added command recovery, governance controls, and audit flow (`3a177d5d9`).
- Added Telegram connector ingress with conversation binding and replay protection (`95c4afc81`).
- Enhanced frontend orchestrator panel stream and interaction coverage tests (`fb642c5fc`).

### Docker and Installer

- Docker adaptation has been updated (`679e5cf54`, `7af0e7d17`, `35f17ecda`).
- Better workspace path handling for Docker and local runtime.
- Runtime-aware update flow for existing Docker deployments.
- `.env` and API-token wiring improvements.
- One-click installer has been updated (`07ef09911`, `35f17ecda`).
- Installer can reuse existing `.env` and hand off to update flow automatically.
- Installer supports install/update mode, language selection, non-interactive flags, optional volume reset, and readiness checks.

### Phase 28: Orchestrator Evolution (March 2026)

- Terminal completion context injection: LLM decisions now include terminal log summaries, diff stats, and commit bodies.
- Cross-terminal handoff notes: previous terminal context (role, status, commit, handoff notes) is passed to the next terminal.
- ReviewCode / FixIssues / MergeBranch instructions now spawn dedicated reviewer and fixer terminals instead of just publishing events.
- Review reject auto-triggers fix terminal creation; review pass auto-checks workflow completion.
- Auto-merge on workflow completion with conflict detection and status tracking.
- Error handler wired into the agent event loop for terminal failure delegation.
- LLM fault tolerance: `call_llm_safe` wrapper with consecutive failure tracking and graceful degradation.
- State persistence with 5-second debounce at key checkpoints; crash recovery rebuilds agent state from DB.
- Planning Draft now supports multi-turn LLM conversation with the WorkspacePlanning prompt profile.
- Feishu (Lark) long-lived WebSocket connector with tenant token management, message routing, `/bind` and `/unbind` commands.
- ChatConnector trait abstracting Telegram and Feishu outbound messaging behind a unified interface.
- Feishu server integration: conditional startup via `GITCORTEX_FEISHU_ENABLED`, management API, health check extension.
- ResilientLLMClient: multi-provider round-robin with 5-failure circuit breaker and 60-second probe recovery.
- Terminal-level provider failover: automatic replacement terminal spawning with alternative CLI/model config.
- Provider health monitoring API with live circuit-breaker data, manual reset, and WebSocket events (`provider.switched`, `provider.exhausted`, `provider.recovered`).

## Current Status

- Source of truth: `docs/undeveloped/current/TODO-pending.md`
- Phase 28 (Orchestrator Evolution): 18/18 tasks completed
- Total completed: 62
- Pending: 5 (all are low/medium-priority backlog)

See also:

- `docs/undeveloped/current/TODO.md`
- `docs/undeveloped/current/orchestrator-chat-verification-report.md`
- `docs/undeveloped/current/orchestrator-chat-rollback-runbook.md`

## Quick Start

### Local Development

```bash
pnpm install

# Required: exactly 32 chars
# Windows PowerShell:
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
# Linux/macOS:
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

npm run prepare-db
pnpm run dev
```

Default URLs:

- Frontend: `http://localhost:23457`
- Backend API: `http://localhost:23456/api`

### Docker (One-Click Install)

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

The installer supports:

- Interactive setup (mount path, keys, port, optional AI CLI install)
- Existing `.env` reuse
- Automatic handoff to update flow when appropriate

### Docker Update

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest
```

Common options:

- `-AllowDirty`
- `-PullBaseImages`
- `-SkipBuild`
- `-SkipReadyCheck`

## Verification

```bash
curl http://localhost:23456/readyz
curl http://localhost:23456/api/health
```

If API token is enabled:

```bash
curl http://localhost:23456/api/health -H "Authorization: Bearer <token>"
```

## Data Safety

- **Deleting a project does not delete local files.** Only database records (project metadata, project-repo associations) are removed. Repository files on disk are never touched.
- **Deleting a repository association** only removes the database link between the project and the repo. The actual Git repository on disk remains intact.
- **Project-repo binding** (`defaultAgentWorkingDir`) stores a reference path. Unbinding or rebinding does not move or delete any files.

## Architecture at a Glance

- `OrchestratorAgent`: decision and scheduling core.
- `OrchestratorRuntime`: workflow lifecycle orchestration.
- `MessageBus`: event routing across modules and terminals.
- `TerminalLauncher`: process lifecycle management.
- `GitWatcher`: Git-event-driven orchestration progression.
- `ResilientLLMClient`: multi-provider LLM client with circuit breaking and failover.
- `FeishuService`: Feishu (Lark) WebSocket connector with message routing and slash commands.
- `ChatConnector`: unified trait for outbound messaging across chat platforms.

Main code locations:

- `crates/services/src/services/orchestrator/`
- `crates/server/src/routes/workflows.rs`
- `frontend/src/pages/Workflows.tsx`
- `crates/feishu-connector/`
- `crates/services/src/services/chat_connector.rs`
- `crates/server/src/routes/provider_health.rs`

## Docs

- Development tracker: `docs/undeveloped/current/TODO.md`
- Docker deployment guide: `docs/developed/ops/docker-deployment.md`
- Operations runbook: `docs/developed/ops/runbook.md`
- Troubleshooting: `docs/developed/ops/troubleshooting.md`
- Phase 28 plan: `docs/undeveloped/current/2026-03-11-phase-28-orchestrator-evolution.md`

## Contributing

- Open an issue first for large changes.
- Keep commits small and reviewable.
- Run checks before PR:

```bash
cargo check --workspace
cargo test --workspace
cd frontend && npm run test:run && cd ..
```

## License

- Vibe Kanban derived parts: Apache-2.0
- CC-Switch derived parts: MIT
- See `LICENSE`
