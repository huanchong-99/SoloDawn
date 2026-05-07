# Wave-2 Exploration Findings (40 agents, cross-cutting & uncovered regions)

**Date:** 2026-04-14
**Status:** Complete — all 40 agents reported.

Total: ~300+ new findings. HIGH-severity subset below (fixer-wave candidates).

## HIGH — Frontend build / styling (critical)
- **W2-14-01, 02** `frontend/postcss.config.cjs:4` + `frontend/tailwind.config.cjs` — **PostCSS loads empty `tailwind.config.cjs` instead of `tailwind.new.config.js`; breaks all custom design tokens in production.**
- **W2-14-03** `frontend/tsconfig.node.json:4` — Missing `strict: true`
- **W2-39-01** `frontend/src/styles/legacy/index.css:72` — Undefined CSS variable `--_background-foreground`
- **W2-39-02** `frontend/tailwind.legacy.config.js:127` — `chivo-mono` font not imported

## HIGH — Frontend react-query invalidation
- **W2-21-01** `frontend/src/hooks/useApprovalMutation.ts:14-37` — No invalidation on approve/deny
- **W2-21-02** `frontend/src/hooks/useSessionSend.ts:89-95` — No invalidation on follow-up
- **W2-21-03** `frontend/src/hooks/useFollowUpSend.ts:53` — No invalidation
- **W2-21-09** `frontend/src/hooks/usePlanningDraft.ts:100-103` — materialize doesn't invalidate workflows
- **W2-21-11** `frontend/src/hooks/useDevServer.ts:60-62` — Key factory inconsistency
- **W2-21-14** `frontend/src/hooks/useConcierge.ts:86-90` — Missing messages invalidation

## HIGH — Frontend hooks (stale deps / state)
- **W2-40-01..02** `frontend/src/hooks/useConversationHistory.ts:746,786` — Missing `patchWithKey` in deps
- **W2-40-07** `frontend/src/hooks/useJsonPatchWsStream.ts:216` — initialData missing from deps
- **W2-40-11** `frontend/src/hooks/usePreviousPath.ts:72` — Missing pathname guard in effect

## HIGH — Frontend stores
- **W2-22-01** `frontend/src/contexts/WorkspaceContext.tsx:151-152` — Stale getState in unmount cleanup
- **W2-22-03** `frontend/src/stores/useUiPreferencesStore.ts:382-387` — `useExpandedAll()` returns new object literal → re-render storm
- **W2-22-05** `frontend/src/contexts/WorkspaceContext.tsx:150-153` — Stale slice after diffPaths reset
- **W2-22-08** `frontend/src/pages/Workflows.tsx:629,646-651` — Unstable subscribeToWorkflow dep
- **W2-22-10** `frontend/src/stores/terminalStore.ts:191-210` — Inline getState() loses reactivity

## HIGH — i18n
- **W2-07-03..06** `frontend/src/i18n/locales/{es,ja,ko,zh-Hant}/workflow.json:187` — Missing `confirmDeleteTitle` key
- **W2-24-01** `frontend/src/components/workflow/steps/Step3Models.tsx:327` — Raw key `workflow:step3.messages.confirmDeleteTitle`
- **W2-24-02..04** `frontend/src/components/panels/TaskAttemptPanel.tsx:21,25`, `frontend/src/components/logs/VirtualizedList.tsx:138` — Hardcoded English strings

## HIGH — Rust: secrets in logs
- **W2-25-01..03** `crates/services/src/services/concierge/agent.rs:408,430,477` — Full HTTP body in error message

## HIGH — Rust: analytics/opt-out
- **W2-29-01** `crates/services/src/services/pr_monitor.rs:130-142` — Missing analytics_enabled check
- **W2-29-02** `crates/services/src/services/analytics.rs:86` — Race in async send without opt-out re-validation

## HIGH — Rust: auth/authz
- **W2-18-01** `crates/server/src/routes/mod.rs:179` — Protected routes mix with what should be public (setup)
- **W2-18-02** `crates/server/src/middleware/model_loaders.rs:15-40` — `load_project_middleware` no ownership check
- **W2-18-03** `crates/server/src/middleware/model_loaders.rs:42-67` — `load_task_middleware` no ownership check
- **W2-34-01** `crates/server/src/routes/mod.rs:179` — `/api/setup/*` blocked by API token
- **W2-34-02** `crates/server/src/routes/mod.rs:166` — CI webhook inside API token auth zone

## HIGH — Rust: type consistency (ts-rs boundary)
- **W2-17-01** `crates/services/src/services/events/types.rs:37-38` — RecordTypes missing `#[ts(rename_all = "SCREAMING_SNAKE_CASE")]`
- **W2-32-01..03** `crates/server/src/routes/shared_tasks_types.rs:7-31,49` — Missing `#[serde(rename_all = "camelCase")]` on SharedTask, UserData, SharedTaskResponse, AssigneesQuery

## HIGH — Rust: workspace deps
- **W2-13-01** `crates/cc-switch/Cargo.toml:4` — edition "2021" mismatch (others 2024)
- **W2-13-02** `crates/tray/Cargo.toml:19` + `crates/cc-switch/Cargo.toml:29` — `tracing` unpinned
- **W2-13-03** `crates/deployment/Cargo.toml:15` — non-workspace sqlx version

## HIGH — Rust: error masking / resource leaks
- **W2-16-05** `crates/deployment/src/lib.rs:180` — `Project::count(...).unwrap_or(0)` masks DB errors
- **W2-36-01** `crates/services/src/services/terminal/process.rs:220` — TrackedProcess missing Drop
- **W2-36-04** `crates/services/src/services/orchestrator/agent.rs:1711-1722` — PendingGuard best-effort only

## HIGH — Rust: env vars on hot path
- **W2-27-01** `crates/server/src/middleware/auth.rs:54` — per-request env read (documented, but lock-free-better)
- **W2-27-03** `crates/server/src/mcp/task_server.rs:919-920,932-933` — Mutable env set in request path without unsafe/sync
- **W2-27-11** `crates/utils/src/assets.rs:40-41` — unsafe env mutation in tests without guard

## HIGH — Rust: backpressure / silent drops
- **W2-30-01** `crates/utils/src/msg_store.rs:54` — Broadcast send ignores errors
- **W2-30-02** `crates/services/src/services/terminal/output_fanout.rs:136` — Broadcast send ignores errors
- **W2-30-05** `crates/executors/src/executors/acp/harness.rs:198,282` — Unbounded event/log channels with silent drops
- **W2-30-09** `crates/server/src/routes/terminal_ws.rs:382-466` — Lagged recovery missing prev_seq audit

## HIGH — Rust: websocket / event schema drift
- **W2-20-01** `crates/server/src/routes/workflow_events.rs:171-172` — Dup snake_case/camelCase on LLMDecision
- **W2-20-09** `frontend/src/utils/streamJsonPatchEntries.ts:80` — LogMsg::Ready conversion contract mismatch

## HIGH — Rust: SQL N+1 / unbounded
- **W2-15-01** `crates/db/src/models/workspace.rs:645-653` — N+1 in `find_all_with_status()` loop
- **W2-15-02** `crates/db/src/models/image.rs:190-204` — N+1 in `associate_many_dedup()`

## HIGH — Rust: cross-platform
- **W2-37-01** `crates/utils/src/path.rs:118` — Hardcoded `/var/tmp` Unix-only
- **W2-37-02** `crates/utils/src/path.rs:86-96` — Hardcoded `/private/var/` macOS-only
- **W2-37-03** `crates/services/src/services/cli_installer.rs:130` — Unix-only install path

## HIGH — DB migrations
- **W2-38-01** `20250729..._remove_stdout_and_stderr_from_execution_processes.sql:3-4` — Data loss, no backup
- **W2-38-02** `20251202..._migrate_to_electric.sql:1-24` — Breaking change, no compat view
- **W2-38-03** `20260208..._fix_terminal_old_foreign_keys.sql:20,23` — CREATE INDEX missing IF NOT EXISTS
- **W2-38-05** `20251216..._refactor_task_attempts_to_workspaces_sessions.sql:28-29` — Data integrity gap (randomblob)
- **W2-38-07** `20250818..._refactor_images_to_junction_tables.sql:33-35` — CREATE INDEX idempotency

## HIGH — Infra
- **W2-09-02** `docker/Dockerfile:189-200` — Runtime installs curl then pipes Node
- **W2-09-05** `docker/compose/docker-compose.yml:23` — `SOLODAWN_ENCRYPTION_KEY` as plain env var
- **W2-09-09** `docker/Dockerfile:256` — COPY --from with no integrity check
- **W2-10-04** `scripts/docker/install/install-single-cli.sh:146` — Unsafe `eval $DETECT_CMD`
- **W2-10-06** `scripts/quality/run-sonar-scanner.sh:12` — No cleanup trap on unzip failure
- **W2-12-01** `.github/actions/setup-rust/action.yml:56` — Action pinned to `master`
- **W2-12-04** — Missing concurrency groups across ci-basic/quality/docker
- **W2-12-11** `.github/actions/setup-rust/action.yml:74` — Insecure curl for nextest install (no SHA)
- **W2-11-01** `installer/solodawn.iss:127` — Insecure temp file
- **W2-11-02** `installer/solodawn.iss:236` — Incomplete PATH uninstall cleanup
- **W2-11-05** `installer/install-single-cli.ps1:200-206` — Broken regex for scoped packages with hyphens
- **W2-11-06** `installer/solodawn.iss:20-22` — Missing rollback
- **W2-11-09** `installer/solodawn.iss:101-102` — `taskkill` without result check

## HIGH — Tests
- **W2-02-02..04** `crates/services/tests/phase18_git_watcher.rs:97-132,149-170,187-210` — Spawned tasks never awaited
- **W2-03-01** `crates/server/tests/auth_test.rs:70,83,96,97` — unsafe env mutation without sync
- **W2-06-03** `frontend/src/stores/__tests__/wsStore.test.ts:56-75` — Timer leak between tests

## HIGH — API versioning
- **W2-31-01** `crates/server/src/routes/workflows_dto.rs:32-37` — `merge_terminal_cli_id` blocked on migration
- **W2-31-02** `crates/server/src/routes/config.rs:106` — `/api/info` not updated on frontend

## Feature flags (behavior)
- **W2-28-01** `crates/server/src/routes/workflows.rs:285-286` — Inverted default (SOLODAWN_ORCHESTRATOR_CHAT_ENABLED true-by-default)
- **W2-28-02** `crates/server/src/routes/chat_integrations.rs` — Asymmetric defaults (vs chat_connector)

## MED / LOW
~200 additional findings preserved across agent reports in session log. Will fix HIGH-severity subset in the upcoming fix wave.
