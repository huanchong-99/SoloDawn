# GitCortex Troubleshooting Guide

This guide covers common issues and their solutions when running GitCortex.

## Table of Contents

- [Server Won't Start](#server-wont-start)
- [Workflow Stuck in "Running" State](#workflow-stuck-in-running-state)
- [API Key Not Working](#api-key-not-working)
- [Terminal No Output](#terminal-no-output)
- [Database Locked](#database-locked)
- [Getting Help](#getting-help)

---

## Server Won't Start

### Symptom

Server fails to start with error message or exits immediately.

### Possible Causes & Solutions

#### 1. Port Already in Use

**Symptoms:**
```
Error: bind() address already in use
```

**Diagnosis:**
```bash
# Windows
netstat -ano | findstr :3001

# Linux/macOS
lsof -i :3001
```

**Solutions:**
- Kill process using port 3001:
  ```bash
  # Windows (use PID from netstat)
  taskkill /PID <PID> /F

  # Linux/macOS
  kill -9 <PID>
  ```
- Or change port in environment:
  ```bash
  export PORT=3002
  ```

#### 2. Missing Environment Variables

**Symptoms:**
```
Error: GITCORTEX_ENCRYPTION_KEY not set
```

**Solution:**
```bash
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
```

**Note:** Key must be 32 bytes (64 hex characters).

#### 3. Database Not Initialized

**Symptoms:**
```
Error: no such table: workflows
```

**Solution:**
```bash
# Run migrations
pnpm run db:migrate

# Or using sqlx-cli
sqlx migrate run --database-url crates/db/data.db --source crates/db/migrations
```

#### 4. Rust Version Mismatch

**Symptoms:**
```
Error: required nightly-2025-12-04 but found <different_version>
```

**Solution:**
```bash
# Install correct version
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04

# Verify
rustc --version
# Should output: rustc 1.85.0-nightly (2025-12-04)
```

#### 5. Missing Dependencies

**Symptoms:**
```
Error: cannot find -lsqlite3
Error: failed to compile
```

**Solution:**

**Windows:**
- Install [CMake](https://cmake.org/download/)
- Install [SQLite3](https://www.sqlite.org/download.html)
- Add to PATH

**Linux:**
```bash
sudo apt-get install build-essential cmake libsqlite3-dev
```

**macOS:**
```bash
brew install cmake sqlite3
```

---

## Workflow Stuck in "Running" State

### Symptom

Workflow status shows "running" but no progress in tasks or terminals.

### Possible Causes & Solutions

#### 1. Orchestrator Agent Not Responding

**Diagnosis:**
```bash
# Check orchestrator logs
sqlite3 crates/db/data.db "SELECT id, status FROM workflows WHERE status = 'running';"

# Check if orchestrator terminal exists
sqlite3 crates/db/data.db "SELECT * FROM terminals WHERE terminal_type = 'orchestrator' ORDER BY created_at DESC LIMIT 1;"
```

**Solution:**
- Check API key configuration
- Verify network connectivity to AI provider
- Restart workflow (will create new orchestrator instance)

#### 2. Git Watcher Not Detecting Changes

**Diagnosis:**
```bash
# Check .git/refs/heads modification time
ls -lh .git/refs/heads/

# Manually trigger git event
git checkout <branch>
git commit --allow-empty -m "trigger event"
```

**Solution:**
- Ensure Git repository is initialized
- Check file system permissions on `.git` directory
- Restart server to reinitialize git watcher

#### 3. Terminal Process Hung

**Diagnosis:**
```bash
# Check running processes
ps aux | grep -E "claude|gemini|codex"

# Check terminal status in DB
sqlite3 crates/db/data.db "SELECT id, status, exit_code FROM terminals WHERE status = 'working';"
```

**Solution:**
- Kill hung terminal processes manually
- Update terminal status to "failed":
  ```bash
  sqlite3 crates/db/data.db "UPDATE terminals SET status = 'failed', exit_code = -1 WHERE id = '<terminal_id>';"
  ```
- Restart workflow to retry

#### 4. Task Dependencies Not Met

**Diagnosis:**
```bash
# Check task dependencies
sqlite3 crates/db/data.db "SELECT id, branch_name, depends_on_task_id FROM workflow_tasks WHERE workflow_id = '<workflow_id>';"
```

**Solution:**
- Ensure prerequisite tasks are completed
- Manually update task status if stuck:
  ```bash
  sqlite3 crates/db.data.db "UPDATE workflow_tasks SET status = 'completed' WHERE id = '<task_id>';"
  ```

---

## API Key Not Working

### Symptom

Workflow fails with authentication error or AI provider rejects requests.

### Possible Causes & Solutions

#### 1. Invalid or Expired API Key

**Diagnosis:**
- Check workflow configuration in UI
- Test API key manually:
  ```bash
  # Claude API
  curl https://api.anthropic.com/v1/messages \
    -H "x-api-key: YOUR_KEY" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":1,"messages":[{"role":"user","content":"test"}]}'
  ```

**Solution:**
- Update workflow with new API key
- Ensure key has required permissions
- Check key quota/billing status

#### 2. Encrypted Key Not Decrypted

**Symptoms:**
```
Error: Failed to decrypt API key
```

**Diagnosis:**
```bash
# Check encryption key is set
echo $GITCORTEX_ENCRYPTION_KEY

# Should be 32 bytes (64 hex chars)
```

**Solution:**
- Ensure `GITCORTEX_ENCRYPTION_KEY` is set consistently
- Must use same key used for encryption
- If key lost, database must be recreated

#### 3. Wrong API Type or Base URL

**Diagnosis:**
- Check workflow configuration in UI
- Verify API type matches provider (anthropic/openai/vertex)

**Solution:**
- Update workflow with correct API type
- Use correct base URL for provider:
  - Anthropic: `https://api.anthropic.com`
  - OpenAI: `https://api.openai.com/v1`
  - Custom: verify URL is correct

#### 4. Model Not Available

**Diagnosis:**
- Check model name in workflow configuration
- Test model availability via provider's API

**Solution:**
- Update workflow with valid model name
- Check provider documentation for available models

---

## Terminal No Output

### Symptom

Terminal starts but produces no output or hangs silently.

### Possible Causes & Solutions

#### 1. CLI Not Installed

**Diagnosis:**
```bash
# Check CLI availability
claude --version
gemini --version
codex --version
```

**Solution:**
- Install missing CLI:
  ```bash
  # Claude Code
  npm install -g @anthropic-ai/claude-code

  # Gemini CLI
  npm install -g @google/gemini-cli

  # Codex
  npm install -g codex-cli
  ```
- Update PATH if installed but not found

#### 2. Wrong CLI Configuration Path

**Diagnosis:**
```bash
# Check config files exist
ls -la ~/.claude/settings.json
ls -la ~/.gemini/.env
ls -la ~/.codex/auth.json
ls -la ~/.codex/config.toml
```

**Solution:**
- Verify CLI configuration paths
- Reinitialize CLI configuration:
  ```bash
  claude auth
  gemini auth
  codex auth
  ```

#### 3. Terminal Working Directory Issue

**Diagnosis:**
```bash
# Check project directory exists
ls -la /path/to/project

# Check Git repository
cd /path/to/project && git status
```

**Solution:**
- Ensure project path is correct
- Initialize Git repository if missing:
  ```bash
  cd /path/to/project
  git init
  ```

#### 4. Model Switch Failure

**Diagnosis:**
```bash
# Check cc-switch logs
sqlite3 crates/db/data.db "SELECT * FROM model_switches ORDER BY created_at DESC LIMIT 5;"
```

**Solution:**
- Verify model configuration is valid
- Check CLI config file syntax
- Manually test model switch:
  ```bash
  cc-switch set claude claude-3-5-sonnet-20241022
  ```

#### 5. Terminal Process Not Started

**Diagnosis:**
```bash
# Check terminal status
sqlite3 crates/db/data.db "SELECT id, status, pid FROM terminals WHERE id = '<terminal_id>';"
```

**Solution:**
- Check system logs for process creation errors
- Verify permissions to spawn processes
- Check for resource limits (ulimit)

---

## Database Locked

### Symptom

Operations fail with "database is locked" error.

### Possible Causes & Solutions

#### 1. Multiple Server Instances Running

**Diagnosis:**
```bash
# Check for running processes
ps aux | grep gitcortex-server

# Check for file locks
lsof crates/db/data.db
```

**Solution:**
- Kill duplicate server processes
- Ensure only one instance is running

#### 2. Uncommitted Transaction

**Diagnosis:**
```bash
# Check for locks
sqlite3 crates/db/data.db "PRAGMA lock_status;"
```

**Solution:**
- Stop all server instances
- Wait for locks to clear (usually seconds)
- Restart server

#### 3. Disk Full

**Diagnosis:**
```bash
# Check disk space
df -h

# Check database size
du -sh crates/db/data.db
```

**Solution:**
- Free up disk space
- Vacuum database if needed:
  ```bash
  sqlite3 crates/db/data.db "VACUUM;"
  ```

#### 4. File Permissions

**Diagnosis:**
```bash
# Check file permissions
ls -la crates/db/data.db

# Should be readable/writable by server user
```

**Solution:**
```bash
# Fix permissions
chmod 600 crates/db/data.db
chown <server_user>:<server_group> crates/db/data.db
```

---

## Quality Gate Issues

### Symptom

Quality gate not running, always passing, or blocking unexpectedly.

### Possible Causes & Solutions

#### 1. Quality Gate Not Running

Check the current mode:
```bash
# Check quality-gate.yaml
cat quality/quality-gate.yaml | head -15

# Check environment override
echo $QUALITY_GATE_MODE
```

If mode is `off`, the gate is disabled. Set to `shadow`, `warn`, or `enforce` to enable.

#### 2. SonarQube Connection Failed

```bash
# Check SonarQube is running
curl -s http://localhost:9000/api/system/status

# Check token is set
echo $SONAR_TOKEN

# Test scanner manually
./scripts/quality/run-sonar-scanner.sh
```

If SonarQube is unreachable, the sonar provider is skipped automatically. Local checks (cargo, eslint, tsc) still run.

#### 3. Quality Gate Always Passing

In `shadow` mode, the gate logs results but never blocks. Check the mode:
```yaml
# quality/quality-gate.yaml
mode: shadow  # Change to "warn" or "enforce" to enable blocking
```

Also verify providers are enabled:
```yaml
providers:
  rust: true
  frontend: true
  repo: true
  security: true
  sonar: true
```

#### 4. Quality Gate Blocking Unexpectedly

If mode is `enforce` and terminals are stuck in quality loops:

**Quick fix** — switch to shadow mode:
```yaml
mode: shadow
```

**Investigate** — check the quality run results:
```bash
curl http://localhost:23456/api/terminals/<terminal_id>/quality/latest
```

Common causes:
- Clippy warnings treated as errors (check `clippy_warnings` threshold in YAML)
- Pre-existing test failures (quality gate checks ALL tests, not just changed ones)
- SonarQube rules too strict (adjust thresholds or disable sonar provider)

#### 5. Quality Results Not Showing in UI

Check WebSocket connection:
- Open browser DevTools → Network → WS tab
- Look for `quality.gate_result` events

If events are missing, check the backend logs for `TerminalQualityGateResult` bus message errors.

If events arrive but UI doesn't update, the React Query cache may not be invalidating. Check `wsStore.ts` handler for `quality.gate_result`.

---

## Getting Help

### Diagnostic Information Collection

Before seeking help, collect the following:

1. **System Information:**
   ```bash
   uname -a
   rustc --version
   node --version
   pnpm --version
   sqlite3 --version
   ```

2. **Server Logs:**
   ```bash
   # Last 100 lines
   sudo journalctl -u gitcortex -n 100

   # Or from dev mode
   # Check console output
   ```

3. **Database State:**
   ```bash
   # Workflow status
   sqlite3 crates/db/data.db "SELECT id, name, status FROM workflows;"

   # Recent errors
   sqlite3 crates/db.data.db "SELECT * FROM terminals WHERE status = 'failed' ORDER BY created_at DESC LIMIT 5;"
   ```

4. **Configuration:**
   ```bash
   # Environment variables (sanitize sensitive data)
   env | grep GITCORTEX
   ```

### Resources

- **Documentation:**
  - README.md
  - Operations Manual: `docs/developed/ops/runbook.md`
  - Architecture Design: `docs/developed/plans/2026-01-16-orchestrator-design.md`

- **GitHub Issues:**
  - Search existing issues first
  - Include diagnostic information
  - Provide reproduction steps

- **Community:**
  - Check project discussions
  - Review similar workflows

### Creating a Bug Report

When reporting issues, include:

1. **Description:** What happened and what you expected
2. **Steps to Reproduce:** Exact commands/actions taken
3. **Environment:** OS, Rust version, Node.js version
4. **Logs:** Relevant error messages and stack traces
5. **Database State:** Workflow/task/terminal statuses
6. **Configuration:** Sanitized environment variables

### Emergency Recovery

If server is completely unresponsive:

1. **Force stop server:**
   ```bash
   sudo systemctl kill -s SIGKILL gitcortex
   ```

2. **Backup database:**
   ```bash
   cp crates/db/data.db crates/db/data.db.emergency_backup
   ```

3. **Check for corruption:**
   ```bash
   sqlite3 crates/db/data.db "PRAGMA integrity_check;"
   ```

4. **Restore from backup if needed:**
   ```bash
   cp crates/db/data.db.<latest_backup> crates/db/data.db
   ```

5. **Restart server:**
   ```bash
   sudo systemctl start gitcortex
   ```
