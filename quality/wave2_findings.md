# Wave-2 Exploration Findings (40 agents, cross-cutting & uncovered regions)

**Date:** 2026-04-14
**Status:** 23/40 completed at time of writing; this file will be updated.

## HIGH-Severity Bugs

### Test suites
- **W2-01-01** `tests/e2e/workflow_test.rs:837-841` — Silenced cleanup errors via `let _ =`
- **W2-01-02** `tests/e2e/workflow_test.rs:1088-1102` — Brittle time-based polling; `task_completed` assigned but never asserted
- **W2-02-01** `crates/services/tests/terminal_binding_test.rs:16` — unsafe env var mutation without sync
- **W2-02-02..04** `crates/services/tests/phase18_git_watcher.rs:97-132,149-170,187-210` — Spawned watcher tasks never awaited after abort()
- **W2-03-01** `crates/server/tests/auth_test.rs:70,83,96,97` — unsafe env_var modification without sync
- **W2-03-03** `crates/server/tests/slash_commands_test.rs:191,207,272,295,337,355,371` — App rebuilt per request, fragmented DB state
- **W2-03-07** `crates/server/tests/events_test.rs:116,163,261` — `#[serial]` insufficient for shared MessageBus
- **W2-05-01** `crates/db/benches/workflow_bench.rs:55,169,284` — Multiple Runtime instances leak resources
- **W2-05-02** `crates/db/benches/workflow_bench.rs:59,175,288` — In-memory SQLite dropped after iteration (unrealistic)
- **W2-05-05** `crates/server/benches/performance.rs:11` — `_rt` Runtime unused; benches simulate with Vec
- **W2-06-01..02** `frontend/src/hooks/useCliTypes.test.tsx:16,147-364` — `globalThis.fetch` leakage
- **W2-06-03** `frontend/src/stores/__tests__/wsStore.test.ts:56-75` — Timer leak between tests
- **W2-06-04** `frontend/src/hooks/__tests__/useQualityGate.test.ts:56-63` — Missing restoreAllMocks pairing
- **W2-06-07** `frontend/src/hooks/useLogStream.test.tsx:40-46` — `MockWebSocket.instances` never cleared
- **W2-06-09** `frontend/src/hooks/useQualityGate.test.tsx:144-153` — Fetch undefined until first test sets it
- **W2-06-11** `frontend/src/hooks/auth/useAuthStatus.test.tsx:10-12` — Module-scope mutated authState leaks

### i18n
- **W2-07-03..06** `frontend/src/i18n/locales/{es,ja,ko,zh-Hant}/workflow.json:187` — Missing `confirmDeleteTitle` key

### Docker
- **W2-09-02** `docker/Dockerfile:189-200` — Runtime installs curl then pipes Node binaries (no verification)
- **W2-09-05** `docker/compose/docker-compose.yml:23` — `SOLODAWN_ENCRYPTION_KEY` as plain env var
- **W2-09-09** `docker/Dockerfile:256` — COPY --from with no integrity check

### Scripts
- **W2-10-04** `scripts/docker/install/install-single-cli.sh:146` — Unsafe `eval $DETECT_CMD` (injection)
- **W2-10-06** `scripts/quality/run-sonar-scanner.sh:12` — No cleanup trap on unzip failure
- **W2-10-11** `scripts/docker/install/install-single-cli.sh:146` — Unsafe command substitution/eval

### Installer
- **W2-11-01** `installer/solodawn.iss:127` — Insecure temp file in %TEMP%
- **W2-11-02** `installer/solodawn.iss:236` — Incomplete PATH uninstall cleanup
- **W2-11-05** `installer/install-single-cli.ps1:200-206` — Broken package name regex for scoped packages with hyphens
- **W2-11-06** `installer/solodawn.iss:20-22` — Missing rollback on install failure
- **W2-11-09** `installer/solodawn.iss:101-102` — `taskkill` without result check

### CI
- **W2-12-01** `.github/actions/setup-rust/action.yml:56` — Action pinned to `master` instead of SHA
- **W2-12-04** `ci-basic.yml, ci-quality.yml, ci-docker.yml` — Missing concurrency groups
- **W2-12-11** `.github/actions/setup-rust/action.yml:74` — Insecure curl for nextest install (no SHA)

### Build config
- **W2-13-01** `crates/cc-switch/Cargo.toml:4` — edition "2021" mixed with workspace 2024
- **W2-13-02** `crates/tray/Cargo.toml:19` + `crates/cc-switch/Cargo.toml:29` — `tracing` unpinned
- **W2-13-03** `crates/deployment/Cargo.toml:15` — non-workspace sqlx
- **W2-14-01..02** `frontend/postcss.config.cjs:4` + `frontend/tailwind.config.cjs` — Stale legacy config overrides production (!!)
- **W2-14-03** `frontend/tsconfig.node.json:4` — Missing `strict: true`

### Error propagation
- **W2-16-05** `crates/deployment/src/lib.rs:180` — `Project::count(...).unwrap_or(0)` masks DB errors

### WebSocket schemas
- **W2-20-01** `crates/server/src/routes/workflow_events.rs:171-172` — Dup snake_case/camelCase fields on LLMDecision
- **W2-20-09** `frontend/src/utils/streamJsonPatchEntries.ts:80` — `LogMsg::Ready` conversion contract mismatch

### React Query invalidation
- **W2-21-01** `frontend/src/hooks/useApprovalMutation.ts:14-37` — No invalidation on approve/deny
- **W2-21-02** `frontend/src/hooks/useSessionSend.ts:89-95` — No invalidation on follow-up
- **W2-21-03** `frontend/src/hooks/useFollowUpSend.ts:53` — No invalidation
- **W2-21-05** `frontend/src/hooks/useProjectMutations.ts:29-30` — Literal keys vs factory pattern
- **W2-21-09** `frontend/src/hooks/usePlanningDraft.ts:100-103` — materialize doesn't invalidate workflows
- **W2-21-11** `frontend/src/hooks/useDevServer.ts:60-62` — Key factory inconsistency
- **W2-21-14** `frontend/src/hooks/useConcierge.ts:86-90` — Missing messages invalidation

### Secrets in logs
- **W2-25-01..03** `crates/services/src/services/concierge/agent.rs:408,430,477` — Full HTTP body in error message

### Env vars
- **W2-27-01** `crates/server/src/middleware/auth.rs:54` — per-request env read on hot path
- **W2-27-03** `crates/server/src/mcp/task_server.rs:919-920,932-933` — Mutable env set_var in hot path without unsafe

### Analytics
- **W2-29-01** `crates/services/src/services/pr_monitor.rs:130-142` — Missing analytics_enabled opt-out check
- **W2-29-02** `crates/services/src/services/analytics.rs:86` — Race in async fire without opt-out re-validation

### Auth coverage
- **W2-34-01** `crates/server/src/routes/mod.rs:179` — `/api/setup/*` incorrectly protected (blocks onboarding)
- **W2-34-02** `crates/server/src/routes/mod.rs:166` — CI webhook wrapped in API token auth (should use HMAC only)

### Drop impls
- **W2-36-01** `crates/services/src/services/terminal/process.rs:220` — TrackedProcess missing Drop
- **W2-36-04** `crates/services/src/services/orchestrator/agent.rs:1711-1722` — PendingGuard only logs on panic
- **W2-36-10** `crates/services/src/services/container.rs` — tokio::spawn without handle tracking

### Cross-platform
- **W2-37-01** `crates/utils/src/path.rs:118` — Hardcoded `/var/tmp` Unix-only
- **W2-37-02** `crates/utils/src/path.rs:86-96` — Hardcoded `/private/var/` macOS-only
- **W2-37-03** `crates/services/src/services/cli_installer.rs:130` — Unix-only install path

### Migrations
- **W2-38-01** `20250729165913_remove_stdout_and_stderr_from_execution_processes.sql:3-4` — Data loss, no backup
- **W2-38-02** `20251202000000_migrate_to_electric.sql:1-24` — Breaking change, no compat view
- **W2-38-03** `20260208020000_fix_terminal_old_foreign_keys.sql:20,23` — CREATE INDEX missing IF NOT EXISTS
- **W2-38-05** `20251216142123_refactor_task_attempts_to_workspaces_sessions.sql:28-29` — Data integrity gap via randomblob
- **W2-38-07** `20250818150000_refactor_images_to_junction_tables.sql:33-35` — CREATE INDEX idempotency

### Legacy design
- **W2-39-01** `frontend/src/styles/legacy/index.css:72` — Undefined CSS variable `--_background-foreground`
- **W2-39-02** `frontend/tailwind.legacy.config.js:127` — `chivo-mono` font not imported

### Hooks (remaining)
- **W2-40-01..02** `frontend/src/hooks/useConversationHistory.ts:746,786` — Missing `patchWithKey` in deps
- **W2-40-07** `frontend/src/hooks/useJsonPatchWsStream.ts:216` — initialData missing from deps
- **W2-40-11** `frontend/src/hooks/usePreviousPath.ts:72` — Missing pathname guard in effect

## MED / LOW
See individual agent reports (~150 additional findings).
