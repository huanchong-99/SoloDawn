# SoloDawn Operations Manual

This runbook provides operational procedures for running SoloDawn in development and production environments.

## Starting the Server

### Development Mode

**Prerequisites:**
- Rust nightly-2025-12-04 installed
- Node.js 20+ installed
- pnpm 10.13.1 installed
- Encryption key set in environment

**Steps:**

1. Set environment variables:
```bash
# Windows PowerShell
$env:SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
```

2. Install dependencies:
```bash
pnpm install
```

3. Run database migrations:
```bash
pnpm run db:migrate
```

4. Start development server:
```bash
pnpm run dev
```

This starts both frontend (port 3000) and backend (port 3001) with hot-reload enabled.

### Production Mode

**Build:**

1. Build frontend:
```bash
pnpm run build
```

2. Build backend:
```bash
cargo build --release
```

**Run:**

1. Set environment variables:
```bash
# Windows PowerShell
$env:SOLODAWN_ENCRYPTION_KEY="your-production-32-byte-hex-key"
$env:DATABASE_URL="crates/db/data.db"
$env:PORT="3001"

# Linux/macOS
export SOLODAWN_ENCRYPTION_KEY="your-production-32-byte-hex-key"
export DATABASE_URL="crates/db/data.db"
export PORT="3001"
```

2. Start server:
```bash
./target/release/solodawn-server
```

**Using Process Manager (systemd):**

Create `/etc/systemd/system/solodawn.service`:
```ini
[Unit]
Description=SoloDawn Server
After=network.target

[Service]
Type=simple
User=solodawn
WorkingDirectory=/opt/solodawn
Environment="SOLODAWN_ENCRYPTION_KEY=your-production-32-byte-hex-key"
Environment="DATABASE_URL=/opt/solodawn/data.db"
ExecStart=/opt/solodawn/target/release/solodawn-server
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable solodawn
sudo systemctl start solodawn
sudo systemctl status solodawn
```

## Database Management

### Backup

**SQLite Backup:**

1. Stop the server:
```bash
sudo systemctl stop solodawn
# Or kill process in dev mode
```

2. Backup database:
```bash
# Using sqlite3
sqlite3 crates/db/data.db ".backup crates/db/data.db.backup"

# Or copy file (ensure server is stopped)
cp crates/db/data.db crates/db/data.db.backup.$(date +%Y%m%d_%H%M%S)
```

3. Restart server:
```bash
sudo systemctl start solodawn
```

### Restore

**From Backup:**

1. Stop the server:
```bash
sudo systemctl stop solodawn
```

2. Restore database:
```bash
cp crates/db/data.db.backup crates/db/data.db
```

3. Restart server:
```bash
sudo systemctl start solodawn
```

### Migration

**Run Migrations:**

```bash
# Development
pnpm run db:migrate

# Production
cargo install sqlx-cli --features sqlite
sqlx migrate run --database-url crates/db/data.db --source crates/db/migrations
```

**Rollback Migration:**

```bash
# Revert last migration
sqlx migrate revert --database-url crates/db/data.db --source crates/db/migrations
```

**Add New Migration:**

```bash
# Create migration
sqlx migrate add -r <migration_name> --source crates/db/migrations

# Edit migration file in crates/db/migrations/

# Apply migration
pnpm run db:migrate
```

## Monitoring

### Check Workflow Status

**Via API:**

```bash
# List all workflows
curl http://localhost:3001/api/workflows

# Get specific workflow
curl http://localhost:3001/api/workflows/{workflow_id}

# Get workflow tasks
curl http://localhost:3001/api/workflows/{workflow_id}/tasks

# Get task terminals
curl http://localhost:3001/api/workflows/{workflow_id}/tasks/{task_id}/terminals
```

**Via Database:**

```bash
sqlite3 crates/db/data.db "SELECT id, name, status FROM workflows ORDER BY created_at DESC;"
sqlite3 crates/db/data.db "SELECT id, branch_name, status FROM workflow_tasks WHERE workflow_id = '{workflow_id}';"
sqlite3 crates/db/data.db "SELECT id, status, exit_code FROM terminals WHERE task_id = '{task_id}';"
```

### View Metrics

**Server Logs:**

```bash
# Journal logs (systemd)
sudo journalctl -u solodawn -f

# Log file location (if configured)
tail -f /var/log/solodawn/server.log
```

**Database Statistics:**

```bash
# Active workflows
sqlite3 crates/db/data.db "SELECT COUNT(*) FROM workflows WHERE status = 'running';"

# Total terminals
sqlite3 crates/db/data.db "SELECT COUNT(*) FROM terminals;"

# Failed terminals
sqlite3 crates/db/data.db "SELECT COUNT(*) FROM terminals WHERE status = 'failed';"
```

**Performance Metrics:**

Monitor via system tools:
```bash
# CPU and memory
top -p $(pgrep solodawn-server)

# Disk usage
du -sh crates/db/data.db

# Open connections
netstat -an | grep 3001
```

## Troubleshooting Reference

For common issues and solutions, see [troubleshooting.md](troubleshooting.md).

## Upgrading

### Steps

1. **Backup current version:**
```bash
# Stop server
sudo systemctl stop solodawn

# Backup database
cp crates/db/data.db crates/db/data.db.backup.before_upgrade
```

2. **Update code:**
```bash
git fetch origin
git checkout <new_version_tag_or_branch>
```

3. **Build new version:**
```bash
pnpm install
pnpm run build
cargo build --release
```

4. **Run migrations:**
```bash
pnpm run db:migrate
```

5. **Start server:**
```bash
sudo systemctl start solodawn
```

6. **Verify upgrade:**
```bash
# Check logs
sudo journalctl -u solodawn -n 50

# Check API
curl http://localhost:3001/api/health
```

## Rolling Back

### Steps

1. **Stop server:**
```bash
sudo systemctl stop solodawn
```

2. **Revert code:**
```bash
git checkout <previous_version_tag_or_branch>
```

3. **Restore database:**
```bash
# If migration was run
cp crates/db/data.db.backup.before_upgrade crates/db/data.db

# Or rollback migration
sqlx migrate revert --database-url crates/db/data.db --source crates/db/migrations
```

4. **Rebuild:**
```bash
pnpm install
pnpm run build
cargo build --release
```

5. **Start server:**
```bash
sudo systemctl start solodawn
```

## Health Checks

### Endpoint Health

```bash
curl http://localhost:3001/api/health
```

Expected response:
```json
{
  "status": "healthy",
  "database": "connected",
  "version": "x.y.z"
}
```

### Component Health

**Database Connection:**
```bash
sqlite3 crates/db/data.db "SELECT 1;"
```

**Frontend Serving:**
```bash
curl -I http://localhost:3000
```

**WebSocket Connection:**
```bash
# Test via browser console or WS client
wscat -c ws://localhost:3001/ws
```

## Maintenance Tasks

### Regular Cleanup

**Remove old workflows:**
```bash
# Delete completed workflows older than 30 days
sqlite3 crates/db/data.db "DELETE FROM workflows WHERE status = 'completed' AND completed_at < datetime('now', '-30 days');"
```

**Vacuum database:**
```bash
sqlite3 crates/db/data.db "VACUUM;"
```

**Clean logs:**
```bash
# Rotate logs if using file-based logging
logrotate /etc/logrotate.d/solodawn
```

### Performance Tuning

**Database Optimization:**
```bash
# Analyze query performance
sqlite3 crates/db/data.db "EXPLAIN QUERY PLAN <your_query>;"

# Rebuild indexes
sqlite3 crates/db.data.db "REINDEX;"
```

**Resource Limits:**

Adjust in `crates/server/src/main.rs` or environment:
```bash
# Max concurrent terminals
export SOLODAWN_MAX_TERMINALS=50

# WebSocket timeout
export SOLODAWN_WS_TIMEOUT_SECONDS=300
```

## Security Checklist

- [ ] Encryption key is 32+ bytes and stored securely
- [ ] Database file has restricted permissions (600)
- [ ] API keys are encrypted in database
- [ ] Logs do not contain sensitive information
- [ ] HTTPS is enabled in production (reverse proxy)
- [ ] Firewall rules restrict access to ports 3000/3001
- [ ] Regular backups are automated
- [ ] Audit logs are enabled and monitored

## Quality Gate Operations

### Configuration

The quality gate is configured via `quality/quality-gate.yaml` at the repository root.

**Modes:**
| Mode | Behavior |
|------|----------|
| `off` | Quality gate disabled, legacy workflow semantics |
| `shadow` | Runs analysis and logs results, never blocks (default) |
| `warn` | Runs analysis, publishes results to UI, does not block merge |
| `enforce` | Hard gate — blocks terminal handoff on failure |

To change mode, edit `quality/quality-gate.yaml`:
```yaml
mode: enforce  # off | shadow | warn | enforce
```

Or override via environment variable:
```bash
# Windows PowerShell
$env:QUALITY_GATE_MODE="enforce"

# Linux/macOS
export QUALITY_GATE_MODE=enforce
```

### SonarQube Setup

1. Start SonarQube (Docker):
```bash
docker run -d --name sonarqube -p 9000:9000 sonarqube:community
```

2. Create a project and generate a token at `http://localhost:9000`

3. Set environment variables:
```bash
export SONAR_TOKEN=<your-token>
export SONAR_HOST_URL=http://localhost:9000
```

4. Update `quality/quality-gate.yaml`:
```yaml
sonar:
  host_url: "http://localhost:9000"
  project_key: "solodawn"
```

5. Sync quality profile:
```bash
./scripts/quality/sync-quality-profile.sh
```

### Running Quality Gate Manually

```bash
# Full repo-level gate (shadow mode)
pnpm run quality

# Dry-run check
pnpm run quality:check

# SonarCloud analysis
pnpm run quality:sonar
```

### Terminal Quality Feedback Loop

When quality gate is enabled (`warn` or `enforce` mode):

1. Terminal commits code with `status: checkpoint` metadata
2. Orchestrator intercepts the checkpoint and runs `QualityEngine`
3. If quality passes → terminal promoted to completed → next terminal dispatched
4. If quality fails → structured fix instructions injected to original terminal via PTY stdin
5. Terminal fixes issues and re-commits → cycle repeats

The orchestrator does NOT create a new fixer terminal. The original terminal retains full task context.

### Quality Data Cleanup

Quality run records accumulate over time. To clean up old records:

```sql
-- Delete quality runs older than 30 days
DELETE FROM quality_run WHERE created_at < datetime('now', '-30 days');

-- Delete orphaned quality issues
DELETE FROM quality_issue WHERE run_id NOT IN (SELECT id FROM quality_run);
```

### Monitoring Quality Gate

Check quality gate status via API:
```bash
# Latest quality run for a terminal
curl http://localhost:23456/api/terminals/<terminal_id>/quality/latest

# All quality runs for a workflow
curl http://localhost:23456/api/workflows/<workflow_id>/quality/runs
```

WebSocket event `quality.gate_result` is pushed to all workflow subscribers when a quality run completes.

## Support Resources

- Documentation: `README.md`, `docs/developed/plans/`
- Troubleshooting: `docs/developed/ops/troubleshooting.md`
- Issues: GitHub Issues
- Architecture: `docs/developed/plans/2026-01-16-orchestrator-design.md`
