# V1.0.0 Release — Current Status

**Last Updated**: 2026-03-29
**Overall Status**: Step 3 partially complete, Step 4 not started

---

## Original Plan (4 Steps)

| Step | Description | Status |
|------|-------------|--------|
| Step 1 | Fix anomalies (WS disconnect, log noise, etc.) | ✅ Complete (13 fixes) |
| Step 2 | Clean test directories | ✅ Complete |
| Step 3 | Local testing — 7 tasks sequential | ⚠ 4/7 complete, 1 partial, 2 skipped |
| Step 4 | Docker testing | ❌ Not started |

---

## Step 1: Bug Fixes (13 total, all pushed, CI green)

| # | Fix | Commit |
|---|-----|--------|
| 1 | Concierge WS rapid disconnect (merged useEffect + debounce) | Previous session |
| 2 | Filesystem cancellation log noise (debug→trace) | Previous session |
| 3 | CC-Switch URL /v1 stripping for Claude Code terminals | Previous session |
| 4 | Auto-merge skip for non-existent branches (no-worktree mode) | Previous session |
| 5 | Anthropic-compatible LLM client switched to streaming mode | Previous session |
| 6 | SonarCloud: 4 issues fixed | Previous session |
| 7 | Multiple CreateChatBoxContainer complexity reductions | Previous session |
| 8 | DIY wizard FK constraint (vk_task_id → non-existent VK task) | 1acda26c3 |
| 9 | DIY mode auto-dispatch task instructions to terminals | ead319972 |
| 10 | PromptWatcher early registration before dispatch | 77f6d0299 |
| 11 | Bypass permissions auto-confirm in autoConfirm mode | 77f6d0299 |
| 12 | Handoff stall priority over bypass auto-enter | 76cbc5887 |
| 13 | DIY quiet-window completion monitor (60s→completed) | 1e383904d |

---

## Step 3: Local Testing Results

### Task Results

| Order | Task | Mode | Status | Duration | Key Observations |
|-------|------|------|--------|----------|-----------------|
| 1st | Task 4 (Refactor+Test) | Agent-Planned | ✅ | Previous | 4/4 tasks, 5 commits |
| 2nd | Task 3 (Express→Rust) | Agent-Planned | ✅ | Previous | 2/2 tasks |
| 3rd | Task 1 (Knowledge Base) | Agent-Planned | ✅ | Previous | 6 tasks |
| 4th | Task 7 (Web Memo) | Agent-Planned | ⏸ Skipped | — | Deferred, not retested |
| 5th | Task 5 (Microservices) | Agent-Planned | ⏳ Not done | — | Never started |
| 6th | Task 6 (Kutt Security) | **DIY** | ✅ | ~46 min | 3 parallel tasks, GLM-5, quiet-window monitor |
| 7th | Task 2 (Hoppscotch) | Agent-Planned | ⚠ Partial | 4.5h | Task1 committed, Task2 stuck (GLM-5 loop) |

### Verified System Features

| Feature | Verified | Notes |
|---------|----------|-------|
| DIY mode full lifecycle | ✅ | Create→Prepare→Start→Execute→Complete |
| Agent-Planned mode full lifecycle | ✅ | Requirement→Plan→Confirm→Materialize→Execute |
| Planning Draft lifecycle | ✅ | gathering→spec_ready→confirmed→materialized |
| Multi-terminal parallel execution | ✅ | 3 terminals in Task 6 |
| PromptWatcher bypass auto-confirm | ✅ | Server logs confirmed detection+response |
| Git commit detection | ✅ | Task 2 Task1 commit detected by orchestrator |
| Terminal completion detection (Agent) | ✅ | Quiet window pattern in orchestrator |
| Terminal completion detection (DIY) | ✅ | New quiet-window background monitor |
| Orchestrator handoff/re-dispatch | ✅ | Task 2 engine-dev received multiple re-dispatches |
| ResilientLLMClient failover | ✅ | GLM-5 via Anthropic-compatible streaming |
| CI pipeline (all 4 workflows) | ✅ | Basic Checks, Docker Build, Quality Gate, E2E |

---

## Known Issues (Unfixed)

| # | Issue | Severity | Location |
|---|-------|----------|----------|
| 1 | haiio.xyz proxy 60s gateway timeout | Medium | External — not our bug |
| 2 | "signal timed out" raw error in workspace chat | Low | `crates/server/src/routes/planning_drafts.rs` |
| 3 | PromptWatcher false positives on bypass status line | Low | `crates/services/src/services/terminal/prompt_watcher.rs` — sends Enter on every bypass render, not just prompts |
| 4 | GLM-5 infinite loop on large codebases | Medium | Model limitation, not system bug |
| 5 | Planning Draft confirm→materialize not auto-triggered | Medium | `crates/server/src/routes/planning_drafts.rs` — frontend calls confirm but not materialize |
| 6 | Concierge sidebar display issues | Low | `frontend/src/components/ui-new/` — see `concierge-progress.md` (moved to developed) |

### Problem File Locations

| Problem | File(s) |
|---------|---------|
| Bypass false positives | `crates/services/src/services/terminal/prompt_watcher.rs:1457-1495` (chunk-level) and `:1965-2001` (line-level) — need to distinguish status-line redraws from actual prompts |
| Signal timeout UX | `crates/server/src/routes/planning_drafts.rs` — LLM call timeout error shown raw to user |
| Confirm→materialize gap | `crates/server/src/routes/planning_drafts.rs:236-254` (confirm) vs `:664+` (materialize) — should auto-trigger materialize after confirm |
| DIY quiet-window monitor | `crates/server/src/routes/workflows.rs:1840-1987` — works but 60s may be too short for long LLM thinking phases |

---

## Step 4: Docker Testing — TODO

| Task | Status |
|------|--------|
| One-click install script (`scripts/docker/install-docker.ps1`) | ❌ |
| E2E smoke test (`scripts/docker/e2e-smoke.sh`) | ❌ |
| Docker Compose standard (`docker/compose/docker-compose.yml`) | ❌ |
| Docker Compose dev (`docker/compose/docker-compose.dev.yml`) | ❌ |
| Docker Compose split (`docker/compose/docker-compose.split.yml`) | ❌ |
| 7 tasks in Docker container | ❌ |

---

## CI Status (as of 2026-03-29)

All 4 workflows passing on latest commit:
- ✅ Basic Checks (cargo nextest + frontend lint/typecheck/tests)
- ✅ Docker Build Check
- ✅ Quality Gate (SonarCloud)
- ✅ E2E Self-Test

SonarCloud: 0 bugs, 0 vulnerabilities, 0 code smells — A rating across all axes.

---

## API Configuration

5 models configured, all treated as unstable with full disaster recovery:

| # | Name | CLI | Model ID | Base URL | Status |
|---|------|-----|----------|----------|--------|
| 1 | Sonnet-4.6-A | Claude Code | claude-sonnet-4-6 | https://ww.haiio.xyz/v1 | Unreliable (504s) |
| 2 | Sonnet-4.6-B | Claude Code | claude-sonnet-4-6 | https://ww.haiio.xyz/v1 | Unreliable (504s) |
| 3 | Codex-GPT5.3 | Codex | gpt-5.3-codex-xhigh | https://right.codes/codex/v1 | Unusable (auth fails) |
| 4 | Codex-GPT5.4 | Codex | gpt-5.4-xhigh | https://right.codes/codex/v1 | Unusable (auth fails) |
| 5 | GLM-5 | Claude Code | glm-5 | https://open.bigmodel.cn/api/anthropic/v1 | **Only working model** |

**Conclusion**: Only GLM-5 is functional. It works but is very slow on complex/large tasks.
