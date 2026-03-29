# V1.0.0 Test Progress

**Last Updated**: 2026-03-28 16:30 UTC

## API Configuration (FINAL — 2026-03-28)

**ALL 5 models are unstable. No stable/unstable distinction. Full disaster recovery failover.**

| # | Name | CLI | Model ID | Base URL |
|---|------|-----|----------|----------|
| 1 | Sonnet-4.6-A | Claude Code | claude-sonnet-4-6 | https://ww.haiio.xyz/v1 |
| 2 | Sonnet-4.6-B | Claude Code | claude-sonnet-4-6 | https://ww.haiio.xyz/v1 |
| 3 | Codex-GPT5.3 | Codex | gpt-5.3-codex-xhigh | https://right.codes/codex/v1 |
| 4 | Codex-GPT5.4 | Codex | gpt-5.4-xhigh | https://right.codes/codex/v1 |
| 5 | GLM-5 | Claude Code | glm-5 | https://open.bigmodel.cn/api/anthropic/v1 |

**Design:** ResilientLLMClient handles all failover. Codex CLI actively used. haiio.xyz has 60s gateway timeout. LLM client uses streaming mode.

## Bug Fixes Applied (all pushed to remote, CI green)
1. Concierge WS rapid disconnect fix
2. Filesystem cancellation log noise reduction
3. CC-Switch URL /v1 stripping for Claude Code terminals
4. Auto-merge skip for non-existent branches (no-worktree mode)
5. Anthropic-compatible LLM client switched to streaming mode (504 fix)
6. SonarCloud: 4 issues fixed (negated condition, String.raw, complexity extractions)
7. Multiple CreateChatBoxContainer complexity reductions

## Step 3: Local Testing — Sequential Execution

| Order | Task | Status | Notes |
|-------|------|--------|-------|
| 1st | Task 4 (Refactor+Test) | ✅ Completed | 4/4 tasks, 5 commits |
| 2nd | Task 3 (Express→Rust) | ✅ Completed | 2/2 tasks |
| 3rd | Task 1 (Knowledge Base) | ✅ Completed | 6 tasks |
| 4th | Task 7 (Web Memo) | 🔄 IN PROGRESS | Re-creating with new models |
| 5th | Task 5 (Microservices) | ⏳ Pending | |
| 6th | Task 6 (Kutt Security) | ⏳ Pending (DIY mode) | |
| 7th | Task 2 (Hoppscotch) | ⏳ Pending | |

## Known Issues
1. haiio.xyz proxy has 60s gateway timeout — long requests may 504 even with streaming
2. "signal timed out" raw error shown in workspace chat (should be user-friendly message)
3. Workflow auto-sync to completed can happen while tasks still running

## Step 4: Docker Testing — NOT STARTED
