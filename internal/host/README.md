---
description: 'The host seam (interfaces, tables, hooks, sessions, drivers), group sync and inbox delivery, the offline scripted Claude Code and Codex processes for host integration tests, and the API of the host workspaces and Git commands.'
kind: reference
---

# Host seam

`host.go`, `session.go` and `audit.go` are what every other host file builds on. The rules they carry are in the contracts named below; this section is the API and how a unit plugs into it. `host` imports neither `peer` nor `node` ([architecture](../../planning/architecture.md), section 8.2): `node` makes the host, hands it connections, and calls what its tables hold.

A unit never calls into the file of a unit built beside it. It registers in a table, provides a hook, or calls one, and its tests put a fake in the table or the hook.

## Use this capability

| Need | Entry |
| --- | --- |
| the host of a node | `New(db, key, now)`: the `Host`, and the sessions as the last run left them. `node` then sets `h.Connect` and `h.Shim` |
| what your unit remembers, per host | `Part[T](h)`: one `*T` per type and host, made from its zero value. Not a package variable: tests run several hosts in one process |
| a command to a home | `h.Command(ctx, home, payload)` signs the payload text with the node's key and sends it as `cmd`. It returns the home's answer, the home's error as an `*Error`, `home_unreachable`, or `bad_request` for a command too large for a `ctl` message |
| any other request to a home, or a push such as `ack` | `h.Home(device)`, then `Request` or `Send` |
| a connection to a home (`host.join`) | `h.Connect(ctx, device)`: `node` keeps one open until `ctx` ends and calls `h.Attach` each time it opens |
| a failure with a wire code | `&Error{Code, Message}`; its text is `<code>: <sentence>` |
| a session | `h.Sessions`: `Create`, `Get`, `All`, `Update`, `Move`, `End` |
| a session change that commits with other rows | `h.Sessions.Write(func(tx *Tx) error { … })`, then `tx.Move` and the like, and `h.Audit(tx.Tx, …)` |
| who calls a tool | `h.Sessions.Admit(key, level, interactive)` issues the token of a CLI process, `Caller(token)` reads it, `Forget(token)` drops it when the process has exited |
| the turn a tool is called in | `Caller.Turn`: its number, its kind (`HandbackTurn` for a hand-back), the thread of its last item, the depth of the call it answers |
| whether a tool call or a permission request is open | `done := h.Sessions.Calling(key)` around it; `h.Sessions.Calls(key)` |
| the state rows of a group | `h.Rows`: `Put` and `Reset` for the unit that handles `rows` and `reset`, `Get` and `Of` for everyone |
| an item that answers an open tool call | the tool: `item, done := h.Await(id, check)`, then it reads the channel until its wait runs out, calls `done`, and looks at the channel once more. Delivery, before it queues a `reply` or a `decided` item: `h.Hand(id, item)`, which never waits for the tool |
| whether a session's deliveries are held | `h.Held(key)`, before a turn is given; whoever lifts a hold calls `Deliver` |
| an audit row | `h.Audit(tx, AuditRow{…})`, inside the transaction of what it records |

## Tables

Each is a package map. The file that provides an entry adds it from its `init` with `Register(table, key, entry)`, which panics when the key is taken.

| Table | Key | Who calls the entry |
| --- | --- | --- |
| `Tools` | the tool's name | the MCP server, through `ToolsFor(caller)`: a tool whose `For` refuses the caller is neither listed nor called |
| `Drivers` | the agent name, `claude` or `codex` | whoever starts a session's process |
| `Pushes` | `t` of a frame a home pushes: `inbox`, `rows`, `reset` | `h.Push(home, frame)`, which `node` calls for each push it reads on a connection to a home, and the local link for a home on the same node |
| `Surface` | `t` of a request or push of the [host surface](../../planning/contracts/datachannel.md) (`audit`, `pty.resize`), or `t` of a [host command](../../planning/contracts/commands.md) (`takeover.start`) | `node`, only for a device of the owner; for a command after the envelope checks and the receipt lookup |
| `Listeners` | the name of the listening unit | `h.Report(key, event)`, which the session runtime calls for every event of a session's CLI |
| `Attached` | the name of the unit | `h.Attach`, each time a connection to a home opens |
| `Holds` | the name of the unit that knows of a hold | `h.Held(key)`, which delivery calls. The unit of caps and limits registers the holds of a cap, a rate limit and "all offline"; with no entry nothing is held |

`Listeners`, `Attached` and `Holds` are called in the order of their keys. `node` does the rest of the wiring: it registers the subcommands of host features and gives each `Peer` the handlers of their channels.

## Hooks

One unit implements a hook and sets it from its `init` with `Provide(&Hook, value)`, which panics when the hook is set already.

| Hook | Provided by | Called by |
| --- | --- | --- |
| `Runner.Turn` | the session runtime: start or resume the CLI, note the turn, write it | delivery, with the text and the `Turn` that `Compose` made; the session runtime itself and the takeover, for their own turns |
| `Runner.Answer` | the session runtime | the approval handler, for a request it heard as the event `Asked` |
| `Deliver` | delivery: give the AI its next turn if items are held for it | the session runtime when a turn has ended; whoever lifts a hold |
| `Instructions` | the unit that writes the instruction texts | whoever fills a `Launch` |
| `Compose` | the unit that writes the turn text: all four kinds of turn, which share one header | delivery (`ItemsTurn`); the session runtime (`ReplacedTurn`, `ContinueTurn`); the takeover (`HandbackTurn`) |

## Interfaces

`Home`, `Owner` and `Terminal` are method sets of `*peer.Connection` and `*peer.Stream`, so `node` passes those in as they are; `features/host_test.go` holds the compile-time check. `node` calls `h.Attach(device, home)` for each connection to a home, with the local link for a home on the same node, whose `Path` is `local`. A home's `Path` is what the host reports in `member status`.

## Sessions

The table is in memory and written through to the `session` table of the store. States and their changes are those of [sessions.md](../../planning/sessions.md), section 2.

- `Move(key, from, to)` changes a state only from the state the caller names, and only by a change the contract lists; anything else is `conflict`. A change to `working` or `takeover` is `busy` while another session of the same AI in the same group is in either.
- `Update` changes the other columns, such as the CLI's session id.
- `End` removes the row and the turn noted for it, and ends the tokens issued for it: they name nobody from then on, also not a session made later for the same key.
- When the node starts, `New` makes every session saved as `working` or `waiting` dormant. One saved as `takeover` keeps that state until its hand-back. What the saved rows call for (a crash report, a hand-back) is the caller's to do, from the rows `New` returns.
- A token is in memory only, and names nobody once it is forgotten or its session has ended.

**Transactions and the lock.** Every change runs on the store's one writer and takes the table's lock only there. So `Get`, `All` and `Caller` can be called anywhere, inside a transaction of the store as well. `Create`, `Update`, `Move` and `End` of `Sessions` each start a transaction; inside `Sessions.Write`, or inside any `Write` of the store, calling one of them waits for ever, as a second `Write` of the store does. There, use the `Tx`. A change made through a `Tx` is in the table at once and is taken out again when the transaction's function fails.

Not in the seam: the one state per AI that others see (sessions.md, section 3), the overlays, and which session an item goes to.

## Drivers

A `Driver` starts the structured CLI (`Start`) and gives the command line of the interactive one (`Interactive`), both from a `Launch`. A running `Process` takes turns, an interrupt, answers to permission requests and a close, and reports through `Events`. Only the session runtime holds a `Process`; the other units hear its events through `Listeners` and reach it through `Runner`.

- `Launch.Session` is empty before the first start. A structured start reports the id in force with a `Ready` event; `Interactive` returns the id it put on the command line. Whoever started the CLI stores the id with `Update` ([drivers.md](../../planning/contracts/drivers.md), section 1).
- `Launch.Shim` is the command line of the MCP server for that start: `h.Shim`, then `--token` and the token. The driver writes it into the CLI's MCP configuration as it is.
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

# Workspaces and Git commands

The rules are in `planning/contracts/local.md` section 7; this is the API that carries them out.

- Every `git` and `gh` command of the host goes through `runIn` (`command.go`): the caller's context, a limit of 60 minutes for `git clone` and `git push` and 10 minutes for any other, no prompts, no controlling terminal. Project check commands keep their own runner (`checks.go`).
- Every exported workspace function takes a context. `EnsureGroupWorkspace(ctx, projectDir, source, base)` and `RefreshGroupWorkspace(ctx, projectDir, base, remote)` handle the group's clone.
- `CreateTaskWorkspace(ctx, projectDir, num, slug, ai, base, remote)` and `RemoveTaskWorkspace(ctx, projectDir, num, slug, ai, why)` handle `wt/<num>-<slug>`. `PrepareReviewWorkspace(ctx, projectDir, num, ai, commit, base, remote)` and `RemoveReviewWorkspace(ctx, projectDir, num, ai)` handle `review/<num>`. `ai` is the member id of the AI that holds the folder. A create reuses only a worktree recorded for the same AI, and a removal leaves one recorded for another AI untouched.
- A create works out from the branches whether a task is new, taken over or returning; there is no takeover flag. A slug the home cannot produce, or a `num` below 1, is refused.
- `cloneLock(repo)` is the one lock per clone, keyed by its real path. Workspace functions and `ReviewedMerge` take it.

# Delivery and group state

`deliver.go` implements the host's group synchronization and inbox delivery ([datachannel.md](../../planning/contracts/datachannel.md) sections 4 and 6, [flows/sync.md](../../planning/flows/sync.md), [flows/agent-turn.md](../../planning/flows/agent-turn.md)).

- **Registrations.** From `init`, `deliver.go` registers `"deliver"` in `Attached`, `"inbox"`, `"rows"` and `"reset"` in `Pushes`, and provides `Deliver`. Although `Deliver(h, group, member)` prioritizes `(group, member)` first, `deliverNow` then walks every pending merge group and every held `(group, member)` key across all groups so a lifted hold or a freed `max_open` slot wakes every waiting AI.
- **Per-host state.** All sync cursors, held session queues, active merge trackers, and per-connection sync cancellation handles live in `Part[delivery](h)`, never in package globals. Connection sync handles are cancelled and removed when `Home.Done()` closes or a newer connection attaches.
- **Lock order, goroutines and channels.** `delivery` creates no channels of its own; waiting tool calls read the single-item channel returned by `Host.Await`, whose sole writer is `Host.Hand` (`host.go`). `delivery.mu` guards only in-memory maps and slices and is never held across `Store.Write`, `Sessions` calls, `Rows` calls, `Home.Request` or `Home.Send`, `Host.Hand`, `Host.Held`, `Compose`, `Runner.Turn`, or workspace and merge commands. Push handlers (`inbox`, `rows`, `reset`) update state and queues only and dispatch `deliverNow` and `afterGroupState` on goroutines so a home connection's reader never blocks on Git, the forge or `h.Command` (which runs under a one-minute timeout). Per-AI turn delivery is serialized per `(group, member)` with an in-memory busy/recheck flag so concurrent pushes and re-entrant `Deliver` callbacks never overlap or drop a queued turn.
- **Sync cursor (R3-5).** On `Attach`, each group known for that home (`delivery.groups` or `setting` `'homes'`) enters `syncing` with `live = false` and requests `sync` (`state_only: true`) from its last cursor; an `inbox` or `rows` push for a group with no `groupSync` entry starts a single-group `sync` on that connection as well. `reset` drops `h.Rows` for the group and resets `cursor = 0` and `live = false`. `rows` applies only rows whose `seq` exceeds the cached row's `seq` and advances `cursor` monotonically (`max(cursor, seq)`) only while `live = true`. `synced` sets `cursor = seq`, `live = true` and `syncing = false`, so an interrupted reset restarts from `0` while an interrupted delta or late pre-`synced` row batch keeps its cursor.
- **Routing, open waits, still-current check (R3-3) and merges.** Each `inbox` item checks `to` against `h.Key.ID()` and deduplicates `(home, item)` against `audit` both on arrival and when taken. Open `ask` and `approve` waits receive `reply` and `decided` items via `h.Hand` on arrival and when taken. Session and merge items wait while their group's member or task row is absent, refuse `not_hosted_here` only when a present member row does not name an AI on this host, and check task work against `h.Rows` when taken. Session batches prepare the workspace (reporting `member status` `turn_interrupted` and, for task sessions, `task fail` with redacted Git output if workspace preparation fails), report `task branch` while at step `branch`, compose with `Compose`, write `AuditDelivered`, send `ack`, and call `Runner.Turn` (`HS-15` enforces `max_open` and owns runtime session closing for step 17; [issues.md](../../issues.md)). `merge` items check `written` on `AuditDelivered` before writing `AuditMerge` and running `ReviewedMerge.Apply` (or `Inspect` after a restart), reporting `task merged` (using `TaskRow.Stats`) or `task conflict`.
