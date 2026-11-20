---
description: 'Offline scripted Claude Code and Codex processes for host integration tests.'
kind: reference
---

# Host test CLIs

Build `./internal/host/fakecli/claude` and `./internal/host/fakecli/codex` into a temporary directory and prepend it to the test's PATH. Both answer `--version` without a script. For sessions, set:

- `SOLODAWN_FAKE_CLAUDE_SCRIPT` and/or `SOLODAWN_FAKE_CODEX_SCRIPT`: path to the script for the next start of that CLI. Each process reads its script once. Replace it between a first start and a resume; a relative path is resolved in the session's working folder.
- `SOLODAWN_FAKECLI_LOG`: shared append-only file. Each session writes `start <cli> <id>` before its first step and `exit <cli> <id>` after its last step and MCP cleanup. A forced OS kill cannot run cleanup; use a scripted exit to test a crash.

## Scripts

[Examples](fakecli/testdata/) select messages from the [macOS captures](https://github.com/huanchong-99/SoloDawn/tree/f258f040c1275054d901271086708ccae26d6649/spikes/b-04-macos/captures). Each JSON file has a `steps` array and an optional numeric `exit` status (default 0). Codex app-server scripts also set `session` to the thread ID they issue. Claude takes its ID from `--session-id` or `--resume`; interactive Codex takes it from `resume <id>`.

| Step direction | Action |
| --- | --- |
| `stdin`, `mcp->cli` | Wait for one JSON line from the node or MCP server. Objects match the specified fields recursively; arrays, numbers and strings match literally. JSON in MCP result text compares by value. A missing field does not match `null`. |
| `stdout`, `mcp<-cli` | Write `msg` as one JSON line to the node or configured MCP child. No shell is involved; the fake only runs the MCP command, never a command in a turn. |
| `stderr` | Write the string `msg` as one line. |
| `eof` | Wait for closed stdin; unexpected input fails the script. |

A string value `"$session"` expands to the process's session ID. In stdout, `"$id"` echoes the last stdin message's JSON-RPC ID (including its type). For MCP steps pass the structured CLI's normal `--mcp-config <file>` or the two `-c mcp_servers.solodawn.{command,args}=…` settings with JSON-quoted values.

Reads, not sleeps, control order: put the decision read before the turn result, or end the script at the request to exit mid-turn. A late reply can be delivered as the next input turn. Script timestamps and usage are captured values, not readings of the test machine. Add a captured case with a data file, not a new event handler.

These are vendor fixtures, not SoloDawn wire goldens. They test the host-facing process boundary, not login credentials, sandboxes, native terminals, or the host's later delivery/approval policies. Known capture gaps are in [issues.md](../../issues.md).
