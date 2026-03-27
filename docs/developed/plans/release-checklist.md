# SoloDawn Release Checklist

> **Purpose:** Comprehensive guide for releasing SoloDawn to production
> **Version:** 1.0.0
> **Last Updated:** 2026-01-28

---

## Pre-Release

### Code Quality

- [ ] **All tests pass**
  ```bash
  cargo test --workspace
  pnpm test -- --run
  ```

- [ ] **Code compiles without warnings**
  ```bash
  cargo clippy --workspace -- -D warnings
  ```

- [ ] **Code formatting check**
  ```bash
  cargo fmt -- --check
  pnpm prettier --check .
  ```

- [ ] **Security audit**
  ```bash
  cargo audit
  pnpm audit
  ```

- [ ] **No outstanding TODO comments in critical paths**
  ```bash
  grep -r "TODO" crates/*/src/*.rs | grep -v "// TODO:"
  ```

---

## Testing

### E2E Tests

- [ ] **Workflow lifecycle test**
  ```bash
  cargo test --test workflow_test
  ```
  - Create workflow → Start → Execute → Merge

- [ ] **Concurrent workflow test**
  ```bash
  cargo test --test workflow_recovery_test --test-threads=1
  ```
  - Multiple workflows running simultaneously

- [ ] **Recovery scenarios**
  - Service restart during workflow execution
  - Terminal crash recovery
  - Database connection recovery

### Security Tests

- [ ] **API Key encryption verification**
  - Keys stored encrypted in database
  - Keys never exposed in logs
  - Keys never returned in API responses

- [ ] **Permission boundary tests**
  - Non-admin users cannot view others' workflows
  - API key required for sensitive operations
  - CORS properly configured

### Load Tests

- [ ] **Terminal load test**
  ```bash
  node scripts/load-test/workflow_load.js
  ```
  - 20 concurrent terminals
  - Response time < 500ms
  - No memory leaks

- [ ] **WebSocket stress test**
  - 50 concurrent connections
  - Message throughput > 100 msg/sec
  - No connection drops

- [ ] **Database performance**
  - Query execution time < 100ms
  - Connection pool not exhausted
  - No deadlocks

### Manual Smoke Test

- [ ] **Create workflow via UI**
  - Wizard completes all steps
  - Validation works
  - Workflow appears in list

- [ ] **Start workflow execution**
  - Terminals launch correctly
  - Real-time output visible
  - State transitions work

- [ ] **Edit and restart workflow**
  - Changes apply
  - Previous execution history preserved

---

## Documentation

### README

- [ ] **Installation instructions complete**
  - Prerequisites listed
  - Step-by-step setup
  - Platform-specific notes (Windows/Linux/macOS)

- [ ] **Quick start guide**
  - Create first workflow
  - Start execution
  - View results

- [ ] **Troubleshooting section**
  - Common issues
  - Error messages
  - Solutions

### CHANGELOG

- [ ] **Version entry created**
  ```markdown
  ## [1.0.0] - 2026-01-28

  ### Added
  - Initial stable release
  - Feature 1
  - Feature 2

  ### Changed
  - Breaking change description

  ### Fixed
  - Bug fix 1
  ```
- [ ] **All significant changes listed**
- [ ] **Migration notes included (if breaking changes)**

### Runbook Review

- [ ] **Operational procedures documented**
  - Start/stop service
  - Monitor health
  - Check logs

- [ ] **Common operational tasks**
  - Backup database
  - Restore from backup
  - Clear hung workflows
  - Manage API keys

- [ ] **Incident response**
  - What to check when something fails
  - How to gather diagnostic info
  - Escalation procedures

### Migration Guide (if needed)

- [ ] **Migration steps documented**
- [ ] **Data migration script tested**
- [ ] **Rollback procedures documented**

---

## Release

### Backup

- [ ] **Database backup**
  ```bash
  # SQLite
  cp data/solodawn.db data/solodawn.db.backup.$(date +%Y%m%d_%H%M%S)

  # Or with SQLx
  sqlx database export data/solodawn.db > backup.sql
  ```

- [ ] **Configuration backup**
  ```bash
  cp .env .env.backup.$(date +%Y%m%d_%H%M%S)
  tar -czf config-backup.tar.gz .env data/
  ```

- [ ] **Git tag creation**
  ```bash
  git tag -a v1.0.0 -m "Release v1.0.0"
  git push origin v1.0.0
  ```

### Deployment Steps

1. [ ] **Checkout release branch**
   ```bash
   git checkout main
   git pull origin main
   git checkout -b release/v1.0.0
   ```

2. [ ] **Update version numbers**
   - Update `Cargo.toml` version
   - Update `package.json` version
   - Update UI version display

3. [ ] **Run database migrations**
   ```bash
   # If migrations exist
   sqlx migrate run
   ```

4. [ ] **Build application**
   ```bash
   # Backend
   cargo build --release

   # Frontend
   pnpm build
   ```

5. [ ] **Stop current service**
   ```bash
   # Systemd
   systemctl stop solodawn

   # Or manual
   pkill -f solodawn
   ```

6. [ ] **Deploy new version**
   ```bash
   # Copy binaries
   cp target/release/solodawn /usr/local/bin/

   # Copy frontend assets
   cp -r crates/server/frontend/dist/* /var/www/solodawn/

   # Or use Docker
   docker-compose pull
   docker-compose up -d
   ```

7. [ ] **Start service**
   ```bash
   systemctl start solodawn
   # or
   docker-compose up -d
   ```

8. [ ] **Health check**
   ```bash
   curl http://localhost:3000/api/health
   ```
   - Expected: `{"status":"ok"}`

---

## Post-Release

### Smoke Test

- [ ] **Access web UI**
  - Login works
  - Dashboard loads
  - No console errors

- [ ] **Create test workflow**
  - Wizard completes
  - Workflow starts
  - Terminals connect

- [ ] **Verify workflow execution**
  - Terminals produce output
  - WebSocket messages flow
  - Workflow completes

### Log Monitoring

- [ ] **Check application logs**
  ```bash
  journalctl -u solodawn -f
  # or
  tail -f /var/log/solodawn/app.log
  ```
  - No ERROR or CRITICAL messages
  - No unexpected warnings

- [ ] **Check database logs**
  - No lock contention
  - No query timeouts

- [ ] **Check system resources**
  - CPU usage normal
  - Memory usage stable
  - Disk I/O normal

### Metrics Review

- [ ] **Application metrics**
  - Request latency (p50, p95, p99)
  - Error rate
  - Active workflows
  - Terminal count

- [ ] **Infrastructure metrics**
  - CPU utilization < 80%
  - Memory utilization < 80%
  - Disk space > 20% free

### Merge and Tag

- [ ] **Merge release branch to main**
  ```bash
  git checkout main
  git merge release/v1.0.0
  git push origin main
  ```

- [ ] **Push tag to remote**
  ```bash
  git push origin v1.0.0
  ```

- [ ] **Create GitHub Release**
  - Go to GitHub → Releases → New Release
  - Tag: `v1.0.0`
  - Title: `SoloDawn v1.0.0`
  - Description: Copy from CHANGELOG
  - Attach binaries (if applicable)

---

## Rollback

### Immediate Rollback Procedure

**If critical issues detected post-release:**

1. [ ] **Stop current service**
   ```bash
   systemctl stop solodawn
   ```

2. [ ] **Revert to previous version**
   ```bash
   # Option A: Restore from backup
   cp /usr/local/bin/solodawn.backup /usr/local/bin/solodawn

   # Option B: Reinstall previous version
   git checkout v0.9.0
   cargo build --release
   cp target/release/solodawn /usr/local/bin/
   ```

3. [ ] **Restore database** (if schema changed)
   ```bash
   cp data/solodawn.db.backup.YYYYMMDD_HHMMSS data/solodawn.db
   ```

4. [ ] **Restore configuration**
   ```bash
   cp .env.backup.YYYYMMDD_HHMMSS .env
   ```

5. [ ] **Start service**
   ```bash
   systemctl start solodawn
   ```

### Rollback Verification

- [ ] **Health check passes**
- [ ] **Smoke test passes**
- [ ] **Logs show no errors**
- [ ] **Previous workflows accessible**

---

## Version Strategy

### Semantic Versioning

SoloDawn follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible functionality additions
- **PATCH**: Backwards-compatible bug fixes

### Version Bump Rules

- **MAJOR bump when:**
  - Database schema migration required
  - API contract breaking changes
  - Removed features
  - Configuration format changes

- **MINOR bump when:**
  - New features added
  - New API endpoints
  - New CLI support
  - UI enhancements

- **PATCH bump when:**
  - Bug fixes
  - Performance improvements
  - Documentation updates
  - Internal refactoring

### Pre-Release Versions

For testing releases:

- `v1.0.0-alpha.1` - Early development
- `v1.0.0-beta.1` - Feature complete, testing needed
- `v1.0.0-rc.1` - Release candidate, stable expected

---

## Communication

### Release Notes

- [ ] **Draft release notes**
  ```markdown
  # SoloDawn v1.0.0 Release Notes

  🎉 **First Stable Release!**

  ## Highlights
  - Feature 1
  - Feature 2
  - Feature 3

  ## Breaking Changes
  - List breaking changes here

  ## Upgrade Instructions
  ```bash
  cargo install solodawn --version 1.0.0
  ```

  ## Known Issues
  - Issue 1
  - Issue 2

  ## What's Next
  - Planned features
  ```

- [ ] **Include migration steps** (if applicable)
- [ ] **List known issues**
- [ ] **Add screenshots** (for UI changes)

### Migration Guide

- [ ] **Create migration guide** (if breaking changes)
  ```markdown
  # Migrating from v0.x to v1.0.0

  ## Database Migration
  ```bash
  sqlx migrate run
  ```

  ## Configuration Changes
  - Rename `OLD_VAR` to `NEW_VAR`
  - Add new required variables

  ## Code Changes
  - Update API calls
  - Update data models
  ```

### User Notifications

- [ ] **Announce on GitHub**
  - Create release announcement
  - Pin to repository

- [ ] **Update documentation**
  - README with latest version
  - Migration guide
  - API documentation

- [ ] **Community channels**
  - Post in Discord/Slack
  - Send email newsletter (if applicable)
  - Update website

### Monitoring Alerts

- [ ] **Configure monitoring**
  - Set up uptime monitoring
  - Configure alert thresholds
  - Set up log aggregation

- [ ] **Define alert conditions**
  - Service down
  - Error rate > 5%
  - Response time > 1s
  - Database connection failed

- [ ] **On-call procedures**
  - Who to contact
  - Escalation path
  - Response SLA

---

## Appendix

### Useful Commands

```bash
# Check version
solodawn --version

# View logs
journalctl -u solodawn -n 100

# Restart service
systemctl restart solodawn

# Database backup
cp data/solodawn.db data/solodawn.db.backup.$(date +%Y%m%d_%H%M%S)

# Health check
curl http://localhost:3000/api/health
```

### Contact Information

- **Maintainer:** [Your Name]
- **Support:** [Email/Issue Tracker]
- **Documentation:** [Link to docs]

### Related Documents

- [Development Guide](./01-phase-0-docs.md)
- [Operations Runbook](../ops/runbook.md)
- [API Documentation](../api/README.md)
- [Troubleshooting Guide](../ops/troubleshooting.md)
