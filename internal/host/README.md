---
description: 'Offline scripted Claude Code and Codex processes for host integration tests.'
kind: reference
---

# Host seam

`host.go`, `session.go` and `audit.go` are what every other host file builds on. The rules they carry are in the contracts named below; this section is the API and how a unit plugs into it. `host` imports neither `peer` nor `node` ([architecture](../../planning/architecture.md), section 8.2): `node` makes the host, hands it connections, and calls what its tables hold.

## Use this capability

| Need | Entry |
| --- | --- |
| the host of a node | `New(db, key, now)`: the `Host`, and the sessions as the last run left them |
| what your unit remembers, per host | `Part[T](h)`: one `*T` per type and host, made from its zero value. Not a package variable: tests run several hosts in one process |
| a command to a home | `h.Command(ctx, home, payload)` signs the payload text with the node's key and sends it as `cmd`. It returns the home's answer, the home's error as an `*Error`, or `home_unreachable` |
| any other request to a home, or a push such as `ack` | `h.Home(device)`, then `Request` or `Send` |
| a failure with a wire code | `&Error{Code, Message}`; its text is `<code>: <sentence>` |
| a session | `h.Sessions`: `Create`, `Get`, `All`, `Update`, `Move`, `End` |
| who calls a tool | `h.Sessions.Admit(key, level, interactive)` issues the token of a CLI process, `Caller(token)` reads it, `Forget(token)` drops it when the process has exited |
| an audit row | `h.Audit(tx, AuditRow{…})`, inside the transaction of what it records |

## Tables

Each is a package map. The file that provides an entry adds it from its `init`, so no unit edits another unit's file to be reachable.

| Table | Key | Who calls the entry |
| --- | --- | --- |
| `Tools` | the tool's name | the MCP server, through `ToolsFor(caller)`: a tool whose `For` refuses the caller is neither listed nor called |
| `Drivers` | the agent name, `claude` or `codex` | whoever starts a session's process |
| `Pushes` | `t` of a frame a home pushes: `inbox`, `rows`, `reset` | `h.Push(home, frame)`, which `node` calls for each push it reads on a connection to a home, and the local link for a home on the same node |
| `Surface` | `t` of a request or push of the [host surface](../../planning/contracts/datachannel.md) (`audit`, `pty.resize`), or `t` of a [host command](../../planning/contracts/commands.md) (`takeover.start`) | `node`, only for a device of the owner; for a command after the envelope checks and the receipt lookup |

`node` does the rest of the wiring: it registers the subcommands of host features and gives each `Peer` the handlers of their channels.

## Interfaces

`Home`, `Owner` and `Terminal` are method sets of `*peer.Connection` and `*peer.Stream`, so `node` passes those in as they are; `features/host_test.go` holds the compile-time check. `node` calls `h.Attach(device, home)` for each connection to a home, with the local link for a home on the same node, whose `Path` is `local`. Tests use fakes in memory, kept in the test file that needs them.

## Sessions

The table is in memory and written through to the `session` table of the store. States and their changes are those of [sessions.md](../../planning/sessions.md), section 2.

- `Move(key, from, to)` changes a state only from the state the caller names, and only by a change the contract lists; anything else is `conflict`. A change to `working` or `takeover` is `busy` while another session of the same AI in the same group is in either.
- `Update` changes the other columns, such as the CLI's session id; it leaves the key, the state and the time of its last change alone.
- When the node starts, `New` makes every session saved as `working` or `waiting` dormant. One saved as `takeover` keeps that state until its hand-back. What the saved rows call for (a crash report, a hand-back) is the caller's to do, from the rows `New` returns.
- A token is in memory only, and names nobody once it is forgotten or its session has ended.

Not in the seam: the one state per AI that others see (sessions.md, section 3), the overlays, and which session an item goes to.

## Drivers

A `Driver` starts the structured CLI (`Start`) and gives the command line of the interactive one (`Interactive`), both from a `Launch`. A running `Process` takes turns, an interrupt, answers to permission requests and a close, and reports through `Events`.

- `Launch.Session` is empty before the first start. The driver reports the id in force with a `Ready` event, and whoever started it stores it with `Update` ([drivers.md](../../planning/contracts/drivers.md), section 1).
- A driver keeps no timer. Its caller ends the context of `Start`, which kills the process tree, when an interrupt or a close was not answered within 10 s.
- Sign-in that is not usable and a session id that cannot be resumed are events (`LoginInvalid`, `NoSession`), because a CLI reports them after its process has started.

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
