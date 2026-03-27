# Platform Verification: Windows Dev + Linux Deploy

SoloDawn is developed on Windows and deployed on Linux. This document covers verification steps and known issues for each platform.

## Windows Development

### Prerequisites

- Rust nightly-2025-12-04 via `rustup`
- Node.js >= 18 (recommend 22) + pnpm 10.13.1
- Git for Windows (provides bash for scripts)
- SQLite3 CLI (optional, for DB inspection)

### Setup Commands (PowerShell)

```powershell
# Set required env var
$env:SOLODAWN_ENCRYPTION_KEY = "12345678901234567890123456789012"

# Install dependencies
pnpm install

# Initialize database
pnpm run prepare-db

# Start dev servers
pnpm run dev
```

### SQLite Path Handling

Windows uses backslashes in paths. The codebase normalizes DB paths internally, but when running `sqlx` CLI commands manually, use forward slashes:

```bash
sqlx migrate run --database-url sqlite:crates/db/data.db --source crates/db/migrations
```

### Known Windows Issues

| Issue | Workaround |
|-------|-----------|
| PTY spawn fails with long paths | Keep workspace path short (< 100 chars) |
| File locking on `data.db` | Ensure only one server instance runs |
| `cargo-watch` file events flood | Set `CARGO_WATCH_POLL=1` for polling mode |
| Git CRLF warnings | `.gitattributes` handles this; do not change |
| Port 23456 in use | `netstat -ano \| findstr :23456` then `taskkill /PID <pid> /F` |

### Verification Checklist (Windows)

- [ ] `rustc --version` shows nightly-2025-12-04
- [ ] `pnpm --version` shows 10.13.1
- [ ] `pnpm run prepare-db` completes without error
- [ ] `cargo test --workspace` passes
- [ ] `cd frontend && pnpm test:run` passes
- [ ] `pnpm run dev` starts both servers (ports 23456 + 23457)
- [ ] `pnpm run frontend:check` (tsc) passes
- [ ] `pnpm run backend:check` (cargo check) passes

## Linux Deployment

### Docker Compose (Recommended)

```bash
# Copy and fill environment config
cp docker/.env.example docker/compose/.env
# Edit .env — set SOLODAWN_ENCRYPTION_KEY at minimum

# Start all services
cd docker/compose
docker compose up -d

# Verify health
docker compose ps
curl -f http://localhost:23456/api/health
curl -f http://localhost:9000/api/system/status
```

### Systemd Service (Alternative)

For bare-metal deployment without Docker:

```ini
# /etc/systemd/system/solodawn.service
[Unit]
Description=SoloDawn Server
After=network.target

[Service]
Type=simple
User=solodawn
Environment=SOLODAWN_ENCRYPTION_KEY=<your-32-byte-key>
Environment=RUST_LOG=info
Environment=HOST=0.0.0.0
Environment=PORT=23456
ExecStart=/usr/local/bin/solodawn-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now solodawn
```

### Known Linux Issues

| Issue | Workaround |
|-------|-----------|
| SonarQube needs `vm.max_map_count >= 262144` | `sysctl -w vm.max_map_count=262144` (persist in `/etc/sysctl.conf`) |
| SQLite WAL mode lock on NFS | Use local filesystem for DB, not NFS |
| Container DNS resolution | Ensure Docker DNS works: `docker exec solodawn ping sonarqube` |
| Permission denied on volumes | Check UID mapping; solodawn user is UID 999 in container |

### Verification Checklist (Linux / Docker)

- [ ] `docker compose up -d` starts all 3 services
- [ ] `docker compose ps` shows all healthy
- [ ] `curl http://localhost:23456/api/health` returns OK
- [ ] `curl http://localhost:9000/api/system/status` returns `{"status":"UP"}`
- [ ] SonarQube project exists: `curl http://localhost:9000/api/projects/search?projects=solodawn`
- [ ] Quality gate runs in shadow mode (check logs)
- [ ] WebSocket connects: `wscat -c ws://localhost:23456/ws/workflow/<id>/events`
- [ ] Log rotation: check `/var/log/solodawn/` or Docker log driver config

## Cross-Platform CI

The GitHub Actions CI runs on Ubuntu. Key differences from Windows dev:

- Paths use `/` not `\`
- No PowerShell env vars — use `export`
- Docker available natively (no Docker Desktop needed)
- `pnpm run prepare-db` uses the same SQLite migrations on both platforms
