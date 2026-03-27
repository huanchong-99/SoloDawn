# SoloDawn Docker Deployment Guide

## Quick Start

Windows one-click interactive installer:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

The installer asks where to mount host files into `/workspace`, generates `docker/compose/.env`, validates compose, builds, starts, and checks readiness.
It also supports disabling starter project bootstrap and cleaning old data volume before startup.
If `docker/compose/.env` already exists, the installer can now switch directly into update flow and reuse the existing deployment settings.

Manual flow:

```bash
cd docker/compose
cp .env.example .env
# Edit .env and set at least:
# - SOLODAWN_ENCRYPTION_KEY (32 chars)
# Optional:
# - HOST_WORKSPACE_ROOT=E:/test (or another host path containing git repos)
# - INSTALL_AI_CLIS=1 (install AI CLIs during image build)
docker compose up -d
```

Access: http://localhost:23456

## Isolation and Mount Scope

- Containers are isolated by default and cannot read your full host disk automatically.
- The app can access:
  - container filesystem
  - named volume mounted at `/var/lib/solodawn`
  - only the host path you map to `/workspace` (`HOST_WORKSPACE_ROOT`)
- To expand accessible host files, change `HOST_WORKSPACE_ROOT` and recreate containers.

## Runtime Path Behavior

- SoloDawn now distinguishes containerized runtime from direct host runtime.
- In Docker mode, repo browsing prefers `SOLODAWN_WORKSPACE_ROOT` (default `/workspace`) so the workflow wizard starts from the mounted workspace instead of a host-only default path.
- In direct local mode, the same picker falls back to the backend-selected local browse root instead of assuming Docker paths exist.

## Update Existing Deployment

Recommended Windows update flow:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest
```

What it does:
- optionally runs `git pull --ff-only`
- validates `docker/compose/.env`
- can pull prebuilt image first (if enabled)
- falls back to local build when prebuilt image is unavailable
- recreates containers
- waits for `/readyz`

Manual cross-platform update flow:

```bash
git pull --ff-only
docker compose -f docker/compose/docker-compose.yml --env-file docker/compose/.env build --pull
docker compose -f docker/compose/docker-compose.yml --env-file docker/compose/.env up -d --force-recreate --remove-orphans --no-build
curl http://localhost:23456/readyz
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `SOLODAWN_ENCRYPTION_KEY` | Yes | - | 32-char encryption key for credentials |
| `SOLODAWN_DOCKER_API_TOKEN` | No | - | Bearer token for `/api` routes (Docker-only variable) |
| `ANTHROPIC_API_KEY` | No | - | Claude Code API key |
| `OPENAI_API_KEY` | No | - | Codex CLI API key |
| `GOOGLE_API_KEY` | No | - | Gemini CLI API key |
| `PORT` | No | 23456 | Host port mapping |
| `RUST_LOG` | No | info | Log level (debug/info/warn/error) |
| `HOST_WORKSPACE_ROOT` | No | `../..` | Host path mounted into container for repo discovery |
| `SOLODAWN_WORKSPACE_ROOT` | No | `/workspace` | Workspace mount point in container |
| `SOLODAWN_ALLOWED_ROOTS` | No | `/workspace,/var/lib/solodawn` | Allowed roots for filesystem scanning |
| `SOLODAWN_IMAGE_REGISTRY` | No | `ghcr.io` | Prebuilt image registry host for pull-first strategy |
| `SOLODAWN_IMAGE_NAMESPACE` | No | `huanchong-99` | Registry namespace/user for prebuilt images |
| `SOLODAWN_IMAGE_PULL_POLICY` | No | `missing` | Pull policy: `always` / `missing` / `never` |
| `INSTALL_AI_CLIS` | No | `0` | Set to `1` to install AI CLIs during image build |
| `SOLODAWN_AUTO_SETUP_PROJECTS` | No | `1` | Set to `0` to disable auto-creating starter projects on first launch |

## Volumes

Named volume `solodawn-data` mounted at `/var/lib/solodawn`:
- `assets/` -> SQLite DB, config, credentials
- `worktrees/` -> Git worktrees (auto-created by app)

Bind mount:
- `${HOST_WORKSPACE_ROOT}` -> `${SOLODAWN_WORKSPACE_ROOT}` (host git repos visible in container)

## Health Endpoints

| Endpoint | Auth | Purpose |
|---|---|---|
| `/healthz` | None | Liveness (process alive) |
| `/readyz` | None | Readiness (DB + dirs OK) |
| `/api/health` | Token | Application health |

## Common Operations

```bash
# View logs
docker compose -f docker/compose/docker-compose.yml logs -f

# Restart
docker compose -f docker/compose/docker-compose.yml restart

# Rebuild after code changes
docker compose -f docker/compose/docker-compose.yml up -d --build

# Update an existing deployment using the current .env
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest

# Clean old data volume (destructive)
docker compose -f docker/compose/docker-compose.yml down -v --remove-orphans
docker compose -f docker/compose/docker-compose.yml up -d

# Dev mode
docker compose -f docker/compose/docker-compose.dev.yml up -d --build
```

## Backup & Restore

```bash
# Backup SQLite
docker compose -f docker/compose/docker-compose.yml exec solodawn \
  cp /var/lib/solodawn/assets/solodawn.db /tmp/backup.db
docker compose -f docker/compose/docker-compose.yml cp \
  solodawn:/tmp/backup.db ./backup.db

# Restore
docker compose -f docker/compose/docker-compose.yml cp \
  ./backup.db solodawn:/var/lib/solodawn/assets/solodawn.db
docker compose -f docker/compose/docker-compose.yml restart
```

## Rollback to Local Mode

1. `docker compose down`
2. Run `cargo run -p server` directly (no env vars needed, original paths used)
3. SQLite can be copied from Docker volume if needed

## Pull-First Deployment Strategy

To improve startup speed in weak networks without committing local artifacts, installer/update scripts support a pull-first strategy:

1. Use local prebuilt image if already available
2. Try pulling prebuilt image from registry (based on profile tag)
3. Fall back to local build automatically if pull fails

Image tags attempted in order:
- `latest-<profile>` (`latest-china` or `latest-official`)
- `latest`

## Build Times and Stability

Typical first-time build on gigabit network:

| Stage | Time | Notes |
|-------|------|-------|
| pnpm install | ~30s | Uses cache mount + corepack (no npm install -g); subsequent builds near-instant |
| Frontend (Vite) | ~2 min | ~8500 modules |
| Rust build (`cargo build --release`) | ~10 min | Cold build compiles all dependencies and workspace crates |
| **Total (cold)** | **~12-15 min** | First build compiles everything |
| **Total (warm)** | **~4-8 min** | Reuses cargo/pnpm cache mounts |

**Stability**: `.npmrc` includes `network-timeout=300000` and `fetch-retries=5` to reduce hangs on flaky networks. If pnpm gets stuck at a specific package (e.g. 685/686), the store cache may be corrupted:

```powershell
docker builder prune --filter type=exec.cachemount
```

Then rebuild. After pruning, first build will be cold again; subsequent builds reuse the fresh cache.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Port unreachable | HOST not 0.0.0.0 | Check `HOST` env var |
| `/readyz` returns 503 | DB or dir missing | Check volume mounts |
| `401` on `/api/*` | Docker API token enabled | Set `SOLODAWN_DOCKER_API_TOKEN` correctly or leave it empty |
| Repo scan returns empty | Host workspace not mounted | Set `HOST_WORKSPACE_ROOT` and restart compose |
| CLI not detected | Install disabled/failed | Set `INSTALL_AI_CLIS=1` and rebuild, or use `Settings -> Agents -> One-click Install AI CLIs`; run `/opt/solodawn/install/verify-all-clis.sh` |
| Unexpected starter projects | Old data volume reused or auto-setup enabled | Set `SOLODAWN_AUTO_SETUP_PROJECTS=0`; run `docker compose down -v --remove-orphans` for a clean state |
| Permission denied | Volume ownership | Ensure volume owned by `solodawn` user |
| pnpm install stuck at last package | Corrupted pnpm store cache | `docker builder prune --filter type=exec.cachemount` then rebuild |
