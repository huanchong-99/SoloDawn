---
description: 'Offline scripted Claude Code and Codex processes for host integration tests, and the API of the host workspaces and Git commands.'
kind: reference
---

# Host test CLIs

Build `./internal/host/fakecli/claude` and `./internal/host/fakecli/codex` into a temporary directory and prepend it to the test's PATH. Both answer `--version` without a script. For sessions, set:

- `SOLODAWN_FAKE_CLAUDE_SCRIPT` and/or `SOLODAWN_FAKE_CODEX_SCRIPT`: path to the script for the next start of that CLI. Each process reads its script once. Replace it between a first start and a resume; a relative path is resolved in the session's working folder.
- `SOLODAWN_FAKECLI_LOG`: shared append-only file. Each session writes `start <cli> <id>` before its first step and `exit <cli> <id>` after its last step, once its MCP server is stopped. A forced OS kill writes no exit line; to test an exit inside a turn, end the script there.

## Scripts

Each [script](fakecli/testdata/) is a span of one of the [macOS captures](https://github.com/huanchong-99/SoloDawn/tree/f258f040c1275054d901271086708ccae26d6649/spikes/b-04-macos/captures), line for line and in the captured order: a JSON file with a `steps` array. The session id is the argument after `--session-id`, `--resume` or `resume`; a Codex app-server script sets `session` to the thread id it issues. `exit` is the exit status after the last step (default 0, the only status the captures show).

| Step direction | Action |
| --- | --- |
| `stdin`, `mcp->cli` | Wait for one JSON line from the node or from the MCP server. An object matches when it has the fields of `msg`, recursively; arrays, numbers and strings must be equal, the text of an MCP result included. |
| `stdout`, `mcp<-cli` | Write `msg` as one JSON line to the node or to the MCP server. The first MCP step starts the server named `solodawn` in `--mcp-config <file>` or in the two `-c mcp_servers.solodawn.{command,args}=…` settings. The fake runs nothing else. |
| `stderr` | Write the string `msg` as one line. |
| `eof` | Wait for closed stdin; input fails the script. |

A string value `"$session"` is replaced by the session id. In `stdout`, `"$id"` is replaced by the JSON-RPC id of the last line read from stdin.

A step that does not hold ends the process with status 1 and the reason on stderr. Reads, not sleeps, control order: the next output is written only after the line before it was read. Script timestamps and usage are captured values, not readings of the test machine.

Two scripts go beyond their capture: `codex-exit-mid-turn.json` ends at the first approval request, and `codex-late-call.json` waits for the tool reply after the call has failed. No capture shows either, and the fake prints nothing for them.

To add a captured case, add a file; no code changes. Copy every line of the span in order: `stdout`, `stderr` and `mcp<-cli` lines unchanged except for `"$session"` and `"$id"`; for `stdin` and `mcp->cli` lines keep the fields the case depends on.

These are vendor fixtures, not SoloDawn wire goldens. They test the host-facing process boundary, not login credentials, sandboxes, native terminals, or the host's later delivery/approval policies. Known capture gaps are in [issues.md](../../issues.md).

# Workspaces and Git commands

The rules are in `planning/contracts/local.md` section 7; this is the API that carries them out.

- Every `git` and `gh` command of the host goes through `runIn` (`command.go`): the caller's context, a limit of 60 minutes for `git clone` and `git push` and 10 minutes for any other, no prompts, no controlling terminal. Project check commands keep their own runner (`checks.go`).
- Every exported workspace function takes a context. `EnsureGroupWorkspace(ctx, projectDir, source, base)` and `RefreshGroupWorkspace(ctx, projectDir, base, remote)` handle the group's clone.
- `CreateTaskWorkspace(ctx, projectDir, num, slug, ai, base, remote)` and `RemoveTaskWorkspace(ctx, projectDir, num, slug, ai, why)` handle `wt/<num>-<slug>`. `PrepareReviewWorkspace(ctx, projectDir, num, ai, commit, base, remote)` and `RemoveReviewWorkspace(ctx, projectDir, num, ai)` handle `review/<num>`. `ai` is the member id of the AI that holds the folder. A create reuses only a worktree recorded for the same AI, and a removal leaves one recorded for another AI untouched.
- A create works out from the branches whether a task is new, taken over or returning; there is no takeover flag. A slug the home cannot produce, or a `num` below 1, is refused.
- `cloneLock(repo)` is the one lock per clone, keyed by its real path. Workspace functions and `ReviewedMerge` take it.
