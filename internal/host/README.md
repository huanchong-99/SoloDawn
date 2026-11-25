---
description: 'The host seam (interfaces, tables, hooks, sessions, drivers and driver capability switches), the session runtime, group sync and inbox delivery, group joining and per-home connection management, the offline scripted Claude Code, Codex, OpenCode and Qwen Code processes for host integration tests, the API of the host workspaces and Git commands, and the MCP server and stdio shim.'
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
| any other request to a home, or a push such as `ack` | `h.Home(device)`, then `Request` or `Send`; for a `fetch`, `fetchFrom(ctx, h, device, build)` decodes `proto.Fetched` or the home's `*Error` |
| a connection to a home (`host.join`) | `h.Connect(ctx, device)`: `node` keeps one open until `ctx` ends and calls `h.Attach` each time it opens |
| a failure with a wire code | `&Error{Code, Message}`; its text is `<code>: <sentence>` |
| a session | `h.Sessions`: `Create`, `Get`, `All`, `Update`, `Move`, `End` |
| a session change that commits with other rows | `h.Sessions.Write(func(tx *Tx) error { … })`, then `tx.Move` and the like, and `h.Audit(tx.Tx, …)` |
| who calls a tool | `h.Sessions.Admit(key, level, interactive)` issues the token of a CLI process, `Caller(token)` reads it, `Forget(token)` drops it when the process has exited |
| the MCP server of a session | `h.ServeMCP(ctx, token, in, out)` on the node's local socket after `{"mcp":"<token>"}`; `RunShim(args, dial, stdin, stdout, stderr)` for `solodawn mcp --token <t>` |
| the turn a tool is called in | `Caller.Turn`: its number, its kind (`HandbackTurn` for a hand-back), the thread of its last item, the depth of the call it answers |
| whether a tool call or a permission request is open | `done := h.Sessions.Calling(key)` around it; `h.Sessions.Calls(key)` |
| the state rows of a group | `h.Rows`: `Put` and `Reset` for the unit that handles `rows` and `reset`, `Get` and `Of` for everyone |
| an item that answers an open tool call | the tool: `item, done := h.Await(id, check)`, then it reads the channel until its wait runs out, calls `done`, and looks at the channel once more. Delivery, before it queues a `reply` or a `decided` item: `h.Hand(id, item)`, which ends the wait and puts the item into the channel under one lock, so after `done` the item is either in the channel or queued, and never waits for the tool |
| whether a session's deliveries are held | `h.Held(key)`, before a turn is given; whoever lifts a hold calls `Deliver` |
| an interactive pseudo-terminal (`takeover`) | `h.StartPTY(group, member, agent, launch, onDone)`, `p.Attached()`, `p.Resize(cols, rows)`, `p.Close()`, `h.ClosePTY(group, member)` |
| bridging a `pty:<g>:<member>` data channel | `h.ServeTerminal(t)` |
| watching a takeover terminal | `h.WatchStart(from, device, data)`, then `h.ServeWatch(t)` for `watch:<g>:<member>`; `h.Watchers(group, member)` lists the granted users |
| an audit row | `h.Audit(tx, AuditRow{…})`, inside the transaction of what it records |
| the AI CLIs found here (`host.info`) | `h.Agents(ctx, refresh)`: probes each driver registered in `DriverCaps` on `PATH` (cached 60 s; `refresh` skips the cache), writes `agent`, returns `[]proto.HostAgent` with `Groups` empty and live `Models` |

## Tables

Each is a package map. The file that provides an entry adds it from its `init` with `Register(table, key, entry)`, which panics when the key is taken.

| Table | Key | Who calls the entry |
| --- | --- | --- |
| `Tools` | the tool's name | the MCP server, through `ToolsFor(caller)`: a tool whose `For` refuses the caller is neither listed nor called |
| `Drivers` | the agent name (`claude`, `codex`, …) | whoever starts a session's process |
| `DriverCaps` | the agent name, matching `Drivers` | `Agents`, `agent.set`, `host.join`, `host.leave`, `Tools["approve"].For`, and `takeover.start` ([step-3/drivers.md](../../planning/contracts/step-3/drivers.md) sections 4 and 5) |
| `Pushes` | `t` of a frame a home pushes: `inbox`, `rows`, `reset`, `groups` | `h.Push(home, frame)`, which `node` calls for each push it reads on a connection to a home, and the local link for a home on the same node |
| `Surface` | `t` of a request or push of the [host surface](../../planning/contracts/datachannel.md) (`pty.resize`), or `t` of a [host command](../../planning/contracts/commands.md) (`takeover.start`) | `node`, only for a device of the owner; for a command after the envelope checks and the receipt lookup |
| `Listeners` | the name of the listening unit | `h.Report(key, event)`, which the session runtime calls for every event of a session's CLI |
| `Built` | the name of the unit | `New`, once, when the host is built |
| `Attached` | the name of the unit | `h.Attach`, each time a connection to a home opens |
| `Settled` | the name of the unit | `h.Settle()`, before the host's store closes |
| `Holds` | the name of the unit that knows of a hold | `h.Held(key)`, which delivery calls. The unit of limits registers the holds of a vendor rate limit and "all offline"; with no entry nothing is held |

`Listeners`, `Built`, `Attached`, `Settled` and `Holds` are called in the order of their keys. `node` does the rest of the wiring: it registers the subcommands of host features and gives each `Peer` the handlers of their channels.

In `home_moved.go`, a `groups` push triggers a new sync if a live group is missing or a tracked group appears before sync succeeds. On `moved`, `followMovedHome` replaces the saved home and keeps the old home's signalling URLs.

## Diagnostics

`diag.go` writes the host's events of [diagnostics](../../planning/contracts/diagnostics.md) section
2 through `log/slog`, with a logger made in `New` that holds `node`, the first 8 characters of the
host's key id; a line holds ids, counts and states, never the text of a message or a turn, and a
home's device id is cut to 8 characters. `host.attach` (`Attach`) and `host.detach`
(`watchHomeConn`, when the connection is done), `home`; `sync.begin` and `sync.end` (`syncOneGroup`:
`g`, `home`; at the end `seq`, `ok`); `rows.recv` (`pushRows`: `g`, `home`, `seq` the first and last
row, `""` for none); `inbox.recv` (`handleInboxElem`: `g`, `item`, `ai`, `kind` the item's `why`,
`seen`); `inbox.hand` (`tryOpenWait`: `g`, `item`, `wait`); `inbox.take` (`takeEligibleBatch` when
the AI's queue is not empty, and `firstJump`, which takes nothing: `g`, `ai`, `task`, `took` and `left` item numbers, `paused` the one pause reading of the batch);
`ack.send` (`sendAck`: `g`, `home`, `item`); `turn.begin` and
`turn.end` (the `diag` entry of `Listeners`: `g`, `ai`, `task`; at the end `status`);
`turn.interrupt` (`interruptSessionTurn`, when it claims the turn: `why` is `stop`, `timeout` or
`jump`); `proc.start` and
`proc.exit` (`launchSessionProcess`, `runProcessLoop`: `agent`; `resume` at the start, `why` at the
exit, the claimed interrupt or the event that ended the process); `run.open` and `run.close`
(`extOpenRun`, `extEnd`: `g`, `ai`, `run`; at the close `why` `done`, `crash` or `timeout`); `tool`
(`runToolCall`, when the call returns: the session, `name`, `code` `ok` or the error code, `ms`);
`settle.begin` and `settle.end` (`Settle`). `Host.bg` is the one wrapper for the work the host
starts on its wait groups and the goroutines of a watched terminal. The work's goroutine, not the
caller, who may hold a mutex, writes `bg.start` and `bg.stop` (`name`: `ask`, `cloud_poll`,
`deliver`, `group_state`, `group_sync`, `hand_back`, `home_flush`, `home_sync`, `home_watch`,
`member_status`, `path_watch`, `presence`, `run_timeout`, `watch_conn`, `watch_write`; `g` and
`task` when the work has them), the stop before the wait group's `Done`, so that `Settle` returns
after it. `diag_test.go` reads the lines from the standard logger and checks each once with its
fields, and `inbox.take` with the pause reading and what it left.

## Hooks

One unit implements a hook and sets it from its `init` with `Provide(&Hook, value)`, which panics when the hook is set already.

| Hook | Provided by | Called by |
| --- | --- | --- |
| `Runner.Turn` | the session runtime: start or resume the CLI, note the turn, write it. It interrupts nothing: `Turn.Jump` only notes that the turn jumped the queue | delivery, with the text and the `Turn` that `Compose` made, for the items of a session and for the turn of its own after a jump; the session runtime itself and the takeover, for their own turns |
| `Runner.Answer` | the session runtime | the approval handler, for a request it heard as the event `Asked` |
| `Runner.Close` | the session runtime: close any open CLI process for a session and wait for it to exit | delivery (step 17 cleanup) and `host.leave`, before `Sessions.End` and workspace removal |
| `Deliver` | delivery: give the AI its next turn if items are held for it | the session runtime when a turn has ended; whoever lifts a hold |
| `Instructions` | the unit that writes the instruction texts | whoever fills a `Launch` |
| `Compose` | the unit that writes the turn text, all four kinds sharing one header (`ItemsTurn` takes inbox items; `ReplacedTurn`, `ContinueTurn` and `HandbackTurn` take `nil`) | delivery (`ItemsTurn`, `ContinueTurn`); the session runtime (`ReplacedTurn`); the takeover (`HandbackTurn`) |

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

## Turns and instructions

`turn.go` and `turn_info.go` provide `Instructions` and `Compose` ([drivers.md](../../planning/contracts/drivers.md) sections 3 and 4, [step-2/labels.md](../../planning/contracts/step-2/labels.md)).

- `Instructions(s)` returns the embedded English prompt for `s` from `instructions/{group,task,review,secretary}.md` (`secretary.md` when `s.Secretary` is set, otherwise by `s.Kind`).
- `Compose(h, s, kind, items)` formats `ItemsTurn` (from `items`) and `ReplacedTurn`, `ContinueTurn` and `HandbackTurn` (`items` nil), returning the turn text and `Turn{Kind, Thread, Depth, Jump}` (`N` is set later by `Sessions.SetTurn`).
- Every secretary turn lists the group's AIs by name with their CLI, computer, tier, state, current task, usage (`window <pct>% used, resets in <n> h`, hours rounded up, from the row's `usage`) and skills from the synced rows. The secretary uses these to choose assignees and reviewers; the task tools do not enforce the assignment guidance. When the node knows team groups, one more line names them with their ids: those homed on this node and, once their rows are read, those of remembered homes (`teamGroupsLine`).
- A `checks` or `reviewed` item for a task with `stuck.ask` names all stronger AIs, including the secretary if eligible. `submit_task` returns `conflict` naming the same candidates before pushing, opening a pull request or posting a report. The home clears the block after a qualifying reply.
- Item header labels (`== <i of n> · <why> · …`) and carried message or card-decision lines (`-- from … · <label>`, with `from the home` for home-written `checks` and `system` rows) come from `setting` key `owner` in `h.Store`, the `member` rows (`account`, `allow`) in `h.Rows`, and `msg.state` / `body.to`, never from message text, envelope re-verification, or `cause`.
- First task turns, review turns and `ReplacedTurn` fetch the brief (`FetchRow`) for its goal, criteria and confirmed scope, any style preset named by the brief (`FetchKind` `preset` in `turn_preset.go`, outside the 8 KiB note budget; review turns include only `Style preset: "<name>"`), the newest 100 notes up to 8 KiB (`FetchKind`), and up to 20 recent thread or main-line messages (`FetchThread`) through `fetchFrom(ctx, h, device, build)`. Because `Compose` reads `h.Store` with `Store.Write` and calls `Home.Request`, call it outside any `Store.Write` or `Sessions.Write` transaction.

## Drivers

A `Driver` starts the structured CLI (`Start`) and gives the command line of the interactive one (`Interactive`), both from a `Launch`. A running `Process` takes turns, an interrupt, answers to permission requests and a close, and reports through `Events`. Only the session runtime holds a `Process`; the other units hear its events through `Listeners` and reach it through `Runner`.

- `Launch.Session` is empty before the first start. A structured start reports the id in force with a `Ready` event; `Interactive` returns the id it put on the command line. Whoever started the CLI stores the id with `Update` ([drivers.md](../../planning/contracts/drivers.md), section 1).
- `start_model.go` reads `Launch.Model` and `Launch.Effort` from the current member row at every structured or terminal start, including resume. Row changes leave running processes alone. Claude uses `--model` and `--effort`; Codex uses `model` and `config.model_reasoning_effort` on thread start/resume, and `-m` / `-c model_reasoning_effort="…"` in a terminal. Empty values add nothing. Refused values follow the crash path without replacing the session. `features/host_start_model.feature` covers the host portion of A10.15.
- `Launch.Shim` is the command line of the MCP server for that start: `h.Shim`, then `--token` and the token. The driver writes it into the CLI's MCP configuration as it is.
- A driver keeps no timer. Its caller ends the context of `Start`, which kills the process tree, when an interrupt or a close was not answered within 10 s.
- Sign-in that is not usable and a session id that cannot be resumed are events (`LoginInvalid`, `NoSession`), because a CLI reports them after its process has started.
- `caps.go` holds `Capabilities`, `DriverCaps` and the switches of [step-3/drivers.md](../../planning/contracts/step-3/drivers.md) sections 4, 5 and 8: every driver registers its `Capabilities` (`Accounts`, `Levels`, `Approvals`, `Takeover`, `Usage`) in `DriverCaps` alongside `Drivers`, `agent.set` refuses an account not in `Accounts` with `bad_request` naming `accounts`, `Tools["approve"].For` checks `Approvals == "tool" && !c.Interactive`, and `takeover.start` refuses `Takeover == "none"` with `conflict` naming `takeover` and runs the structured `assign` turn of a new task first only for `"resume"` (`checkTakeover`). `driver` in `host.info` is added in `T3-2` after `P-5`.

### Claude Code (`claude.go`)

Registered in `Drivers` under `"claude"` ([drivers.md](../../planning/contracts/drivers.md), sections 1.1 and 1.3).

- `claudeProcess.mu` serializes stdin writes (`Turn`, `Interrupt`), `Close`, `reqSeq` and `interrupted`; nothing inside `mu` waits on `events` or the child process. `readLoop` is the sole writer of `events` and closes it after stdout EOF, `Wait` and process-tree cleanup; the caller of `Process.Events` is its single reader.
- A `system/init` that lists the `solodawn` server with a status other than `connected` emits no `Ready`: `readLoop` kills the process tree and reads no further line, so the turn ends as a crash with the status in its detail ([drivers.md](../../planning/contracts/drivers.md), section 1.1).
- `rate_limit_event` emits `Used` (`WindowPct`, `WindowResets`) and `RateLimited` when `status` is neither `allowed` nor `allowed_warning`.
- `Process.Answer` returns an error because Claude Code sends permission prompts through the MCP `approve` tool ([mcp-tools.md](../../planning/contracts/mcp-tools.md), section 3).
- `LoginInvalid` and `NoSession` match HTTP status `401` and Claude Code's English error phrases on `result` and non-zero-exit `stderr` until live failure captures exist ([issues.md](../../issues.md)).

### Codex

`codex.go` registers `Drivers["codex"]` ([drivers.md](../../planning/contracts/drivers.md), section 1.2 and 1.3):

- `Start` runs `codex app-server` over stdio JSON-RPC (`initialize`, `initialized`, `thread/start` or `thread/resume`, with `-c features.default_mode_request_user_input=true` and `-c mcp_servers.solodawn.required=true`), detached from any terminal (`detachTerminal`). `Interactive` returns the `codex resume` command line and refuses an empty `Launch.Session`.
- **One reader per channel and lock order.** `Events()` is written only by `readLoop` and read only by `Start`'s caller, and closes after `cmd.Wait` and tree cleanup finish; `ready` is closed once by `finishReady` for `Turn`. `wmu` (stdin writes and close) and `mu` (protocol, turn, approval, user-input and file-change state) are never nested.
- **Requests and notifications.** `commandExecution` and `fileChange` approvals emit `Asked` for `Process.Answer`; `item/tool/requestUserInput` emits `Asked` with `Ask.Reply`; invalid parameters and all other server requests get JSON-RPC error `-32601`. Notifications during an active turn emit `Active`; `account/rateLimits/updated` selects the higher `usedPercent` window (`Used`, and `RateLimited` at 100 percent or when a limit flag is set), and a turn that fails with `usageLimitExceeded` emits `RateLimited`. Live behavior of `features.default_mode_request_user_input=true` and `item/tool/requestUserInput` remains unverified ([issues.md](../../issues.md)).

### OpenCode

`opencode.go` registers `Drivers["opencode"]` and its declaration (`api_key`; levels `1`, `3`; `conn`; takeover `none`) from the OpenCode 1.19.6 documentation read by T3-0. No capture of the real CLI exists yet, so every value below is `[unverified]` ([issues.md](../../issues.md)).

- `Start` runs `opencode serve --hostname 127.0.0.1 --port 0` in the session folder with a random `OPENCODE_SERVER_PASSWORD`, and `OPENCODE_CONFIG_CONTENT` holding the `solodawn` MCP server (`Launch.Shim`) and the level's `permission`. Without `RepoConfig` it also sets `OPENCODE_DISABLE_PROJECT_CONFIG=1`. It reads the address from the `listening on` line (`serverAddress`), opens `GET /event`, then `GET /session/<id>` (a failure is `NoSession`) or `POST /session`.
- `Models` starts a separate server and reads `GET /api/model` as the documented `{data: Model.Info[]}`: `providerID/modelID`, `name` (or the id), and `variants[].id` as efforts, with `modelTier` suggestions. Missing or unknown shapes and failed requests give `[]`. The synthetic `opencode-models.json` fixture follows the [OpenAPI](https://opencode.ai/docs/openapi.json); no real CLI listing was captured.
- Before the first prompt of every process, including resume, a nonempty `Launch.Model` is split on the first `/` and sent to `POST /api/session/<id>/model` as `{"model":{"id":"…","providerID":"…","variant":"…"}}`. Empty effort omits `variant`; empty model skips the call. This documented structured body is an accepted deviation from the contract's unverified `provider/model#variant` notation. A refusal closes the process through the crash path and keeps the session.
- A turn is `POST /session/<id>/prompt_async` with the instructions as `system`; `session.idle` of that session ends it, as `interrupted` after a `MessageAbortedError`, `failed` after any other `session.error` of that session or of none (a `ProviderAuthError` also reports `LoginInvalid`). `message.part.updated` is `Active`, `permission.asked` is `Asked`, answered with `POST /permission/<id>/reply` (`once` or `reject`). `Interrupt` is `POST /session/<id>/abort`. The server does not exit when its input closes, so `Close` kills its process tree.

| Level | `permission` |
| --- | --- |
| 1 | `edit` and `bash` `deny` |
| 3 | `edit`, `bash`, `webfetch`, `external_directory` and `doom_loop` `allow` |

### Qwen Code

`qwen.go` registers `Drivers["qwen"]` and its declaration (`api_key`; levels `1`, `3`; `conn`; takeover `none`) from the Qwen Code 0.28.0 documentation read by T3-0. Every value below is `[unverified]` ([issues.md](../../issues.md)).

- `Start` writes a settings file from `qwenStart` (`tools.approvalMode` `plan` at level 1 and `yolo` at level 3, and the trusted `solodawn` MCP server from `Launch.Shim`) to a temporary folder, passed as `QWEN_CODE_SYSTEM_SETTINGS_PATH`. It runs `qwen serve --hostname 127.0.0.1 --port 0 --token <random> --workspace <folder>`, then `POST /session/<id>/resume` (a failure is `NoSession`) or `POST /session`, and opens `GET /session/<id>/events`. Every call carries the bearer token and `Content-Type: application/json`.
- New sessions include `startupConfig` only for a nonempty model or effort, with just the supplied `modelServiceId` and/or `reasoningEffort`. The driver does not read capabilities or add a capability probe; a refused value follows the crash path and closes the daemon. Model discovery is not supported, so Qwen's list stays `[]`.
- Limitation: a changed model or level of a Qwen Code member applies only to its next new session; the serve protocol documents startupConfig only at creation. Resume sends `{}` and keeps the original model and level, as documented in the [serve protocol](https://qwenlm.github.io/qwen-code-docs/en/developers/qwen-serve-protocol/).
- The instructions go as the first text block of the first prompt of a new session; a resumed session has them in its history. `QWEN_SYSTEM_MD` is not used: it replaces Qwen Code's own system prompt.
- A turn is `POST /session/<id>/prompt`; `turn_complete` ends it (`interrupted` for `stopReason` `cancelled`), `turn_error` ends it `failed` (a `401` in its message also reports `LoginInvalid`). `session_update` is `Active`; `permission_request` is `Asked`, answered with `POST /session/<id>/permission/<request>` choosing `proceed_once` or `cancel`. `Interrupt` is `POST /session/<id>/cancel`. `Close` kills the daemon's process tree and the temporary folder goes when it exits.
- Limitation: Qwen Code has no known switch that keeps a repository's own configuration (`.qwen/settings.json`, `QWEN.md`, extensions) out, so a Qwen Code group session may load it. Folder trust is not that switch: an untrusted folder also asks before every tool. This is the risk R-45 already accepts: only trusted repositories go into a group (`install.md`).

### Local servers (`serve.go`)

The drivers whose CLI runs as a local HTTP server share `serve.go`: `serve` starts the server detached from any terminal and returns its standard output and a wait that kills what it left running, `serverAddress` reads the `listening on` line up to the first space after the address, and `readSSE` reads the event stream with `bufio.Reader`, so an event has no size limit.

## CLI detection and sign-in

`agents.go` provides `(*Host).Agents(ctx)`, `"agents"` in `Listeners`, and `"agent.set"` in `Surface`; see [drivers.md](../../planning/contracts/drivers.md) section 1.4 and [commands.md](../../planning/contracts/commands.md) section 2.

- **Lock order.** `agtPart.probeMu` -> `Store.Write` -> `agtPart.mu`. The listener calls `Sessions.Get` (leaf `Sessions` lock) and then takes `agtPart.mu`; nothing under `agtPart.mu` waits on a probe or the store.

## Pseudo-terminals (`pty.go`)

`StartPTY(group, member, agent, launch, onDone)` runs `Drivers[agent].Interactive(launch)` inside a `github.com/aymanbagabas/go-pty` v0.2.3 pseudo-terminal (Unix PTY or Windows ConPTY) in `launch.Folder`, registers the open `*PTY` under `aiSlot{group, member}` in `Part[ptyPart](h)`, and returns the `*PTY` and the CLI session id. A second `StartPTY` while `(group, member)` is already open is refused with `busy`; non-`*Error` CLI launch failures are returned as `&Error{Code: "cli", Message: …}`.

- Closing (`p.Close()`, `h.ClosePTY(group, member)`) never writes `0x03` (`Ctrl+C`) bytes. It closes the pseudo-terminal, waits up to 10 s (`ptyStopTimeout`) for exit, kills the process tree when still running (`SIGKILL` to `-pid` on Unix, `taskkill.exe /f /t /pid` on Windows), releases `(group, member)`, and calls `onDone` once after the process has exited.
- `ServeTerminal(t)` parses `"pty:<g>:<member>"` strictly; `p.Attached()` reports whether a terminal stream has attached. A malformed label, an unmatched `(g, member)` or a duplicate stream for an already bridged `PTY` is closed. Otherwise one goroutine copies `PTY -> t` (pausing `PTY` reads while `t.Write` waits on the 1 MiB send-buffer limit) and the caller copies `t -> PTY`; `t` ending closes `PTY`, and `PTY` exiting closes `t`.
- `Surface["pty.resize"]` decodes `proto.PTYResize`, requires `cols >= 1` and `rows >= 1` (`bad_request` below 1), and resizes an open `PTY`. Resizes received while no terminal is open are ignored.
- **Lock order and single-reader channels.** `PTY.mu -> ptyPart.mu`; neither lock waits on I/O, process exit or `onDone`. `closing` is read only by `waitLoop`; `procDone` and `outDone` are read only by `runStop`; `done` is closed once by `runStop` for `Close` callers.

## Terminal watching (`watch.go`)

`watch.go` keeps grants, successful `watch.start` connections and open watch channels in `Part[watchPart](h)` ([watch-team.md](../../planning/contracts/step-2/watch-team.md), sections 2-6).

- **Grants.** An inbox item with `why: "watch"` grants the card's `by` member's user access only while the AI is in `takeover`. It records `delivered` and acknowledges the item without waking a CLI. Grants are in memory only; `Watchers(group, member)` returns sorted user ids, or `[]`.
- **Admission.** `Watching(device)` resolves the device's user from the granted groups' device rows. A grant admits that person's devices even when this host is not the group's home. `WatchEnded(group, user)` notifies node after the person's last grant ends, outside the watch lock, so node can close connections with no other reason to remain open. `WatchersChanged()` notifies node each time a grant is added or ends, also outside the lock, so node can push `host.info`.
- **Start and channel.** `WatchStart(from, device, data)` reads the device's user from the group's device rows, refuses an absent grant with `forbidden` and a non-direct path with `relay`, and otherwise records the connection for that slot and device. `ServeWatch(t)` accepts one `watch:<g>:<member>` channel per device after `watch.start`; an unmatched or duplicate channel is closed and drained. Terminal bytes go to the owner's channel first, then unchanged to each watcher's queue. A queue exceeding 1 MiB closes that watcher without slowing the owner. Any input on a watch channel closes it without forwarding bytes to the PTY. Known dimensions on open are pushed only to that watcher's link; subsequent owner `pty.resize` dimensions go to every open watcher on `ctl` as `proto.PTYResize`.
- **End.** `Surface["watch.end"]` drops the named user's grant and closes their channels, or returns `not_found`. A gone watcher member drops their grants in that group. Hand-back closes the slot's channels and drops its grants and successful starts before closing the PTY. Channel loss, queue overflow, connection loss and a path leaving `direct` close channels but keep grants; another `watch.start` may reconnect during the same takeover.
- **State.** One `watchPart.mu` guards grants, successful starts, connection monitors and channel queues; each open watch channel has its own writer goroutine.

## Terminal takeover (`takeover.go`)

`takeover.go` registers `"takeover.start"` and `"takeover.end"` in `Surface`, `"takeover"` in `Holds` and `Settled`, and exposes `(*Host).ReplayTakeover(ctx, from, by, data)` ([flows/takeover.md](../../planning/flows/takeover.md); [commands.md](../../planning/contracts/commands.md) "Once only" and section 2; [datachannel.md](../../planning/contracts/datachannel.md) section 7). Where the flow is silent, it does this:

- **Start.** `not_found` for an AI not hosted here; `conflict` when its driver's `Takeover` is `none` (`checkTakeover`); `relay` unless the owner's connection is `direct`; `busy` while the AI is taken over and its hand-back has not read the Git facts yet, while delivery may have read its holds for a turn that has not begun (`givingTurn`), or the host is merging one of its tasks. From then until its hand-back has read those facts, the takeover holds every delivery of the AI in its group; the session's own state (`takeover`, then `working` in the hand-back turn) holds them from there, so a delivery never sees two holds lift at different moments, and whatever the hand-back reports next (`waiting`, a cancelled task) finds the AI free for the next takeover. It interrupts the AI's running turn in the group (`interruptTurn`, which closes a CLI that has not ended it within 10 s) and picks the session: the secretary's group session (a `task` is ignored); a named task the AI is assignee of; a named task it reviews, only when its review session exists; else `conflict` for a task that is not `active`, `paused` or `failed`, and `not_found` for an unknown one. With no task named, the task session used last whose task is unfinished; with none, `task start` (`origin: "takeover"`, title "Owner takeover", empty plan) and a task row kept until the home's arrives. The session's folder is prepared as delivery does (`prepareTargetSession`). For a new task whose driver's `Takeover` is `resume`, one structured `assign` turn runs first. Then `Runner.Close`, `TakeoverAt` (`git rev-parse HEAD` in a task session with a clone), state `takeover`, an interactive token, `StartPTY`, and `member status`. A failure leaves the session `dormant` and cancels a task this takeover started.
- **End.** `takeover.end` closes the terminal; so do a closed `pty` channel, the CLI exiting, 60 s without an attached channel (`takeoverAttach`), and the owner's connection ending or its path leaving `direct` (one watch per connection, the single reader of its `PathChanged`). A closed terminal hands back: the facts of [local.md](../../planning/contracts/local.md) section 7 from `TakeoverAt` (none in a group or review session); a task this takeover started and nothing changed is cancelled (`task cancel` by the host, session ended, `member status`, nothing posted); otherwise one `HandbackTurn`, in which `post` kind `handback` carries the facts, and a host-signed `handback` with empty `text` when the AI did not post. While the AI is held (offline or rate limited) no hand-back turn starts: the session goes `dormant`, `member status` is sent and the host posts the facts. Then `Deliver` runs on the takeovers' wait group.
- **Restart and shutdown.** `Settle` closes open terminals without a hand-back, ends a running hand-back (its wait for the turn's end) and starts no `Deliver` after it; a row left in `takeover` is handed back when its group's state is next read (`cleanupGroupState`), only while no takeover of the AI is registered. A task session whose task ends while taken over closes its terminal (`cleanupTaskSessions`), and the hand-back reports the member's state.
- **Lock order.** `takeovers.mu` guards the takeovers and is never held across a call out of `takeover.go` but the one to `givingTurn` in `claim`, which takes `delivery.mu`.

# Session runtime

`run.go` provides `Runner` (`Turn`, `Answer` and `Close`), registers `"runtime"` in `Attached` and `Settled`, and exposes `(*Host).ReportSaved(ctx, saved)`, `(*Host).MemberState(group, member)` and, in `deliver.go`, `(*Host).QueuedCount(group, member)` (the held items `host.info` counts). `AuditKilled` lives in `audit.go`.

- **Per-host state.** Everything the runtime remembers lives in `Part[runtimePart](h)` (`procs`, `closing`, `starting`, `limited`, `savedCrash`, and `changed`). No package-level state is shared across hosts, so tests run independent hosts in one process. Every `Attach` flushes queued `savedCrash` reports for that home and sends the current `member status` of each AI hosted here in groups of that home ([flows/connect.md](../../planning/flows/connect.md) step 18).
- **Lifecycle and C-6.** `Runner.Turn` wakes a `dormant` session by starting its driver with `Launch` (`Folder` under `<dbDir>/projects/<group>/<folder>`, `Session`, `Level` defaulting to 3 across all session kinds unless overridden by `perm` or `default_level`, `Shim` with an `Admit` token, `Instructions`, `RepoConfig` `true` for task and review sessions and `false` for group sessions, `ToolTimeout`), reports `TurnBegan` before writing the turn, stores a non-empty `Ready.Session`, and forwards every `Process.Events()` event in order through `h.Report`. Every switch waits for the previous process's `Events()` channel to close and forgets its token before starting the next process or marking the session `dormant`.
- **Dormancy, `max_open` and timeouts.** `TurnEnded` moves an active turn's session to `waiting`, then arms the `idle_minutes` timer (default 15), reports `member status`, and calls `Deliver` on a separate goroutine; a stray `TurnEnded` with no active turn is reported to `Listeners` and ignored. Waking a `dormant` session at `max_open` (default 4) closes the open `waiting` session with the earliest `LastActive` first, or waits on `changed` until an active session frees a slot. Every `Close` arms a 10 s timer that cancels `Start`'s context to kill an unresponsive process tree: outside a turn that records `AuditKilled` (`what = "killed"`, naming the member and task) and nothing else; during a turn the exit is a crash (`turn_interrupted` with reason `crash`, plus `task fail` in a task session).
- **Interrupts.** `interruptSessionTurn(h, key, reason, detail)` interrupts a running turn once, for the first reason given, and reports whether it did; a CLI that has not ended the turn 10 s later is killed. The turn's end is reported with that reason: `stop` and `timeout` as `turn_interrupted`, `timeout` also as `task fail`. `jump` reports nothing but the member's state, and a CLI killed for ignoring a jump's interrupt is a crash like any other (`reportedWhy`).
- **Failed resume and `login_invalid`.** On `NoSession` during a resume, the runtime closes the failed process, clears `CLISession`, composes `ReplacedTurn`, starts a fresh session in the same folder, and reports `session_replaced`. On `LoginInvalid`, the runtime closes the process, marks the AI `limited`, and sends `LoginInvalidEvent` (with redacted detail) only when entering `limited`.
- **Shutdown.** `Settle` (entry `"runtime"`) refuses new processes with `busy`, ends every CLI process, then every open ask, whose answer could otherwise block on a live CLI, and waits for the process loops, the asks, delivery wakeups and home status flushes before the store closes. A loop that ends while the host settles leaves the session row as it is; the next start turns it `dormant` and reports a turn that was running (`ReportSaved`).
- **Lock order.** `runtimePart.mu` may call `Sessions.Get` and `Sessions.All`, which acquire only the leaf in-memory `Sessions` lock; nothing called under `runtimePart.mu` acquires `Store`, `Rows`, `Host`, `Driver`, `Process` or hook/listener locks.
- **One reader per channel.** `Process.Events()` is read only by `runProcessLoop`; `liveProc.loopDone` is read only by `stopProcess` or `writeTurnToWaiting` when waiting for that process to exit; `runtimePart.changed` is closed and replaced under `runtimePart.mu` to wake `waitSignal` callers.

# Host test CLIs

Build `./internal/host/fakecli/claude`, `./internal/host/fakecli/codex`, `./internal/host/fakecli/opencode` and `./internal/host/fakecli/qwen` into a temporary directory and prepend it to the test's PATH. All four answer `--version` without a script. For sessions, set:

- `SOLODAWN_FAKE_CLAUDE_SCRIPT`, `SOLODAWN_FAKE_CODEX_SCRIPT`, `SOLODAWN_FAKE_OPENCODE_SCRIPT` or `SOLODAWN_FAKE_QWEN_SCRIPT`: path to the script for the next start of that CLI. Each process reads its script once. Replace it between a first start and a resume; a relative path is resolved in the session's working folder.
- `SOLODAWN_FAKECLI_LOG`: shared append-only file. Each session writes `start <cli> <id>` before its first step and `exit <cli> <id>` after its last step, once its MCP server is stopped. A forced OS kill writes no exit line; to test an exit inside a turn, end the script there.

## Scripts

`models.go` lists Claude Code models with an `initialize` control request and Codex models with paged `model/list` requests after initialization. OpenCode lists through its server API; Qwen has no listing operation. Drivers that implement `Models(context.Context) []proto.Model` participate in detection; the others return an empty list. Listings share detection's 60-second memory cache. A failed or unknown listing is empty. `tiers.go` contains the sourced, ordered word table for suggestions; Claude entries use `resolvedModel`. `features/host_models.feature` covers the host portion of A10.13 and A10.14, including cache expiry and forced refresh; `features/host_start_model.feature` covers A10.15 for all four CLIs, with server fixtures in `features/host_server_profiles_test.go`.

Each [script](fakecli/testdata/) is a span of one of the [macOS captures](https://github.com/huanchong-99/SoloDawn/tree/f258f040c1275054d901271086708ccae26d6649/spikes/b-04-macos/captures), line for line and in the captured order: a JSON file with a `steps` array. The session id is the argument after `--session-id`, `--resume` or `resume`; a Codex app-server script sets `session` to the thread id it issues. `exit` is the exit status after the last step (default 0, the only status the captures show).

| Step direction | Action |
| --- | --- |
| `stdin`, `mcp->cli` | Wait for one JSON line from the node or from the MCP server. An object matches when it has the fields of `msg`, recursively; arrays, numbers and strings must be equal, the text of an MCP result included. |
| `stdout`, `mcp<-cli` | Write `msg` as one JSON line to the node or to the MCP server. The first MCP step starts the server named `solodawn` in `--mcp-config <file or json>` or in the two `-c mcp_servers.solodawn.{command,args}=…` settings. OpenCode's is `mcp.solodawn.command` in `OPENCODE_CONFIG_CONTENT`; Qwen Code's is `mcpServers.solodawn` in the file `QWEN_CODE_SYSTEM_SETTINGS_PATH`. The fake runs nothing else, and appends the server's stderr to `SOLODAWN_FAKECLI_LOG`. |
| `stderr` | Write the string `msg` as one line. |
| `eof` | Wait for closed stdin; input fails the script. |
| `http` | OpenCode and Qwen Code. Wait for the next request other than the event stream, check `msg.call` (`"<method> <path>"`) and the fields of `msg.body` (`null` for none), then answer with `msg.status` (default 200) and `msg.res`. |
| `sse` | OpenCode and Qwen Code. Send `msg` on the event stream (`GET /event`, or `GET /session/<id>/events`) as one `data:` line. |

A string value `"$session"` is replaced by the session id. In `stdout`, `"$id"` is replaced by the JSON-RPC id of the last line read from stdin.

A step that does not hold ends the process with status 1 and the reason on stderr. Reads, not sleeps, control order: the next output is written only after the line before it was read. Script timestamps and usage are captured values, not readings of the test machine.

The fake OpenCode prints `opencode server listening on <url>`, refuses a request without the `OPENCODE_SERVER_PASSWORD` basic-auth password, and has no capture: its scripts are written in the tests from the documented API. The fake Qwen Code is the same server: it prints `qwen server listening on <url>`, refuses a request without the `--token` bearer token, takes its session id from the script, and has no capture either.

`claude-mcp-failed.json` is a capture of Claude Code 2.1.311 whose MCP server exits at once, with an API address nobody listens on (no model call).

Two scripts go beyond their capture: `codex-exit-mid-turn.json` ends at the first approval request, and `codex-late-call.json` waits for the tool reply after the call has failed. No capture shows either, and the fake prints nothing for them.

`claude-models.json` and `codex-models.json` are synthetic discovery scripts, not CLI captures. Both verify the initialization handshake and closed stdin; Codex also verifies `includeHidden:false` and the cursor on the second page. They require no model call or MCP server.

To add a captured case, add a file; no code changes. Copy every line of the span in order: `stdout`, `stderr` and `mcp<-cli` lines unchanged except for `"$session"` and `"$id"`; for `stdin` and `mcp->cli` lines keep the fields the case depends on.

These are vendor fixtures, not SoloDawn wire goldens. They test the host-facing process boundary, not login credentials, sandboxes, native terminals, or the host's later delivery/approval policies. Known capture gaps are in [issues.md](../../issues.md).

# Workspaces and Git commands

The rules are in `planning/contracts/local.md` section 7; this is the API that carries them out.

- Every `git` and `gh` command of the host goes through `runIn` (`command.go`): the caller's context, a limit of 60 minutes for `git clone` and `git push` and 10 minutes for any other, no prompts, no controlling terminal, and Git's automatic maintenance in the command itself (`foregroundGC`), not in a process that outlives it. Project check commands keep their own runner (`checks.go`).
- Every exported workspace function takes a context. `EnsureGroupWorkspace(ctx, projectDir, source, base)` and `RefreshGroupWorkspace(ctx, projectDir, base, remote)` handle the group's clone.
- `CreateTaskWorkspace(ctx, projectDir, num, slug, ai, base, remote)` and `RemoveTaskWorkspace(ctx, projectDir, num, slug, ai, why)` handle `wt/<num>-<slug>`. `PrepareReviewWorkspace(ctx, projectDir, num, ai, commit, base, remote)` and `RemoveReviewWorkspace(ctx, projectDir, num, ai)` handle `review/<num>`. `ai` is the member id of the AI that holds the folder. A create reuses only a worktree recorded for the same AI, and a removal leaves one recorded for another AI untouched.
- A create works out from the branches whether a task is new, taken over or returning; there is no takeover flag. A slug the home cannot produce, or a `num` below 1, is refused.
- `cloneLock(repo)` is the one lock per clone, keyed by its real path. Workspace functions and `ReviewedMerge` take it.
- `HostedChecksToResume(tasks, ais)` selects the `cloud_check` tasks a restarted node polls again, deriving each `Branch` (`task/<num>-<slug>`) from `Num` and `Slug` via `taskName` and skipping rows `taskName` refuses.

# AI-to-AI calls

`tools_ask.go` registers `ask` and `reply` in `Tools` ([mcp-tools.md](../../planning/contracts/mcp-tools.md) sections 1 and 5, [flows/ask.md](../../planning/flows/ask.md), [commands.md](../../planning/contracts/commands.md) section 1.3).

- Both tools run `CheckOutboundSecret` on `text` before signing or sending any request to the home, keep no `Part` of their own, and read call depth (`Caller.Turn.Depth + 1`) and open call waits (`Host.Await` and `Host.Hand`) from the host seam.
- `ask` resolves `to` among other live AI member rows in `h.Rows` (else `not_found`), places the `call` post in `cmp.Or(c.Task, c.Turn.Thread)` mentioning the callee, and when `wait` is true (default) opens `Host.Await(callID, nil)` before `h.Command` and waits `group.limits.ask_wait_s` via `awaitApprovalItem`, returning `{"answer": …}`, `{"denied": true}` when the callee's owner denied the call ([step-2/confirm.md](../../planning/contracts/step-2/confirm.md) section 5), or `{"pending": true, "receipt": "<call id>"}`. The denial is the `confirm` card in a `reply` item (`answersCall`); after the wait it is a turn, whose text `replyBody` writes.
- `reply` fetches the `call` message row via `fetchFrom` (returning `not_found` when absent) and posts `kind: "reply"` in the call's thread with `re` set to the call id and `mentions` set to `[callerAI]`; depth and call-state checks run on the home.

# Secretary and decision tools

`tools_sec.go` registers `propose_brief`, `set_task` and `request_decision` in `Tools` ([mcp-tools.md](../../planning/contracts/mcp-tools.md), sections 1, 2 and 5): `propose_brief` and `set_task` set `Tool.For` to `c.Secretary`, while `request_decision` registers the `group` schema for every session.
`loadSecDirectory`, `resolveTaskID`, `prepareDecisionPost`, `buildDecisionOptions` and `decisionOption` are package helpers shared with the other MCP tool and approval handlers in `host`.

# Delivery and group state

`deliver.go` implements the host's group synchronization and inbox delivery ([datachannel.md](../../planning/contracts/datachannel.md) sections 4 and 6, [flows/sync.md](../../planning/flows/sync.md), [flows/agent-turn.md](../../planning/flows/agent-turn.md)).

- **Registrations.** From `init`, `deliver.go` registers `"deliver"` in `Attached`, `"inbox"`, `"rows"` and `"reset"` in `Pushes`, `"jump"` in `Listeners`, and provides `Deliver`. Although `Deliver(h, group, member)` prioritizes `(group, member)` first, `deliverNow` then walks every pending merge group and every `(group, member)` key that has held items or an open pre-emption across all groups so a lifted hold or a freed `max_open` slot wakes every waiting AI.
- **Per-host state.** All sync cursors, held session queues, active merge trackers, and per-connection sync cancellation handles live in `Part[delivery](h)`, never in package globals. Each session batch takes eligible items in queue order from one snapshot of the group's pause state. Connection sync handles are cancelled and removed when `Home.Done()` closes or a newer connection attaches.
- **Reading the AI's state.** Delivery reads "taken over" before "running a turn" (`isItemHeld`, `mayContinue`): a hand-back moves a session from the one to the other, and read in that order it is always seen as one of them.
- **Lock order, goroutines and channels.** Waiting tool calls read the single-item channel returned by `Host.Await`, whose sole writer is `Host.Hand` (`host.go`), and each sync waiter channel in `groupSync.waiters` has a single reader, `joinWaitChannel` in `syncGroupState`. `delivery.mu` guards only in-memory maps and slices and is never held across `Store.Write`, `Sessions` calls, `Rows` calls, `Home.Request` or `Home.Send`, `Host.Hand`, `Host.Held`, `Compose`, `Runner.Turn`, channel closes, or workspace and merge commands. Push handlers (`inbox`, `rows`, `reset`) update state and queues only and dispatch `deliverNow` and `afterGroupState` on goroutines so a home connection's reader never blocks on Git, the forge or `h.Command` (which runs under a one-minute timeout). Per-AI turn delivery is serialized per `(group, member)` with an in-memory busy/recheck flag so concurrent pushes and re-entrant `Deliver` callbacks never overlap or drop a queued turn. The `preemption`s of an AI are opened, read and closed only inside that serialized step; `listenJump`, on the runtime's goroutine, sets its `ended` under `delivery.mu` and nothing else.
- **Queue jump ([flows/agent-turn.md](../../planning/flows/agent-turn.md) C).** An item is acknowledged only when its turn is handed to the runtime, and while the AI runs a turn in the group every item is held, a jump item first in the queue. An arriving jump item drops the held items of the same message, the waiting one or an earlier jump, which the home retired for it (`enqueueHeld`); a message jumped twice is delivered once. For the first jump item that only the running turn holds and whose session can be prepared (`canPrepare`; a failed `git fetch` stops nothing), `jumpRunningTurn` interrupts that turn with reason `jump` and returns; a held copy of an item that has been delivered, which the home sent again, is no jump. The item stays held, and the `Deliver` that follows the turn's end, however it ends, delivers it as steps 4 to 9. Where the flow is silent, delivery does this:
  - *Pre-emptions.* A `preemption` (the interrupted session and the jumped message) is opened in `Part[delivery](h)` before the interrupt, in memory only. Each interrupted session of the AI has one, and the one interrupted last continues first. `listenJump` marks it when the turn ends with status `interrupted`. A turn that crashes, is killed for ignoring the interrupt, ends by itself, or was already being interrupted for a pause or a time limit keeps the one outcome of section E: the pre-emption is dropped and the jump item is delivered after it.
  - *While a pre-emption is open* only jump items are taken (`itemsToGo`), so nothing else reaches any session of the AI before the interrupted sessions' own turns. A jump for the session that is running interrupts it; a jump for another session interrupts nothing and runs when the running turn has ended.
  - *Report.* For a jump for another session, `reportPreempted` sends `task preempted` once, when the first jump batch can be taken and before it is, and only for a task session: what came during the round trip holds the batch. `task resume` is owed while the task's row, which the home pushes before it answers, has wait `preempted`: not after a refusal (step `merge`, a task paused otherwise), but after an answer that was lost. A jump item that is held by then (a paused group, the offline switch, a takeover, its own task paused) leaves the task unreported.
  - *The turn of its own.* When no jump item can go, `continueInterrupted` waits while a hold, the group pause, a takeover or a running turn keeps turns from the session (a group paused during the jump turn leaves the task pre-empted until the group resumes), sends `task resume` if it is owed (again at the next delivery when the home did not accept it), looks at everything again, and gives one `ContinueTurn` through `Runner.Turn`, so `max_open`, dormancy and `turn_minutes` apply. No turn is given when the session has ended or its task is paused or failed by then. A turn that cannot start is logged and not retried.
  - *After a restart, or a lost answer,* `adoptPreempted` opens the pre-emption of a task session whose task the home shows as pre-empted; `afterGroupState` looks at such an AI although it has no held items. A session of another kind is not continued after a restart.
- **Sync cursor (R3-5).** On `Attach`, each group known for that home (`delivery.groups` or `setting` `'homes'`) enters `syncing` with `live = false` and requests `sync` (`state_only: true`) from its last cursor; an `inbox` or `rows` push for a group that is neither `syncing` nor `live` starts a single-group `sync` on that connection from its kept cursor as well. `reset` drops `h.Rows` for the group and resets `cursor = 0` and `live = false`. `rows` applies only rows whose `seq` exceeds the cached row's `seq` and advances `cursor` monotonically (`max(cursor, seq)`) only while `live = true`. `synced` sets `cursor = seq`, `live = true` and `syncing = false`, so an interrupted reset restarts from `0` while an interrupted delta or late pre-`synced` row batch keeps its cursor.
- **Routing, open waits, still-current check (R3-3), merges, hosted checks and step 17 cleanup.** Each `inbox` item deduplicates `(home, item)` against `audit` both on arrival and when taken. Open `ask` and `approve` waits receive `reply` and `decided` items via `h.Hand` on arrival and when taken. Session and merge items wait while their group's member or task row is absent, refuse `not_hosted_here` only when a present member row does not name an AI on this host, and check task work against `h.Rows` when taken. A walk of an AI's held items reads each state once (`look`: the group pause, the holds, the takeover, the running turn, each task), so that one which changes meanwhile lets no newer item go first. Items whose turn could not be prepared go back before what came meanwhile, by the rules of an arriving item (`prependHeld`): a copy the home sent again is dropped, and a jump goes first. Session batches prepare the workspace (keeping group-session items held on a Git error, and for task or review sessions reporting `member status` `turn_interrupted` plus `task fail` for task sessions with redacted Git output), report `task branch` while at step `branch`, compose with `Compose`, write `AuditDelivered`, send `ack`, and call `Runner.Turn` (`HS-15` enforces `max_open`). `merge` items check `written` on `AuditDelivered` before writing `AuditMerge` and running `ReviewedMerge.Apply` (or `Inspect` after a restart), reporting `task merged` (using `TaskRow.Stats`) or `task conflict`. On each `afterGroupState`, `interruptPausedTurns` interrupts active turns in a paused group or paused task (`turn_interrupted` with reason `stop`), `cleanupGroupState` closes (`Runner.Close`) and ends (`Sessions.End`) sessions of `gone` members and obsolete task or review sessions (`merged`, `cancelled`, reassigned `Assignee`, changed `Reviewer`), prunes `setting` `'homes'` and reconciles home connections when a `gone` member was the last local AI in the group, and removes obsolete `wt/<num>-<slug>` (calling `DeleteBranch` on `merged` and `PushBranch` on `github` handoff) and `review/<num>` worktrees idempotently across repeated pushes and restarts; `resumeGroupHostedChecks` passes the group's task rows and local AI member ids to `HostedChecksToResume` and starts `startHostedCheck` idempotently for any `cloud_check` task not already polling that commit.

# MCP server and stdio shim

The rules are in [mcp-tools.md](../../planning/contracts/mcp-tools.md) and [local.md](../../planning/contracts/local.md) sections 1 and 3; this is the API that carries them out.

- `RunShim(args, dial, stdin, stdout, stderr)` runs `solodawn mcp --token <t>`: it parses `--token`, connects through `dial`, writes `{"mcp":"<token>"}\n`, and bridges `stdin` and `stdout` (exit 0, 2 on invalid arguments, 1 on dial or I/O error).
- On `stdin` EOF `RunShim` half-closes connections that implement `CloseWrite` (`shim.go:74-76`); Windows `AF_UNIX` half-close does not always deliver EOF, so a shim on Windows may outlive its CLI until the session closes ([issues.md](../../issues.md)).
- `h.ServeMCP(ctx, token, in, out)` runs the `solodawn` MCP server over the remaining bytes of an accepted local-socket connection (`in` is the reader from the opening line, `out` is the socket). It closes `out` before returning and, because the SDK keeps the context from its handlers, as soon as `ctx` ends, which cancels a call that waits. It refuses an unknown or forgotten token, and on each `tools/call` re-reads `Caller(token)` and `ToolsFor(caller)` and brackets `Tool.Call` with `Sessions.Calling`.
- `ServeMCP` takes each `Tool.Schema` as registered (an empty object schema when nil) and projects per-caller differences in `projectCallerSchema`: outside `GroupSession`, `task` and `to` are dropped from top-level `required` except on `ask`; on `post` for non-secretary callers, `task` and `summary` are dropped, and `handback` is dropped in `GroupSession`.
- `projectCallerSchema` (`mcp.go`) puts per-kind projection of `post` and `ask` in the server because `Tool` has one `Schema`, so any later tool with per-kind arguments puts its projection there.

# Joining, leaving and home connections

`join.go` implements `host.join`, `host.leave` and per-home connection tracking ([commands.md](../../planning/contracts/commands.md) sections 1.2 and 2, [datachannel.md](../../planning/contracts/datachannel.md) sections 4 and 7, [local.md](../../planning/contracts/local.md) sections 2 and 7).

- **Registrations and startup.** From `init`, `join.go` registers `"host.join"` and `"host.leave"` in `Surface` and `"join"` in `Attached`. At node startup, after setting `h.Connect`, the caller invokes `h.EnsureHomeConnections()` to connect to all remembered homes, including groups with no local AI.
- **One connection per home.** `Part[joinPart](h)` keeps active `h.Connect` cancellation functions keyed by home device id, each derived from `context.Background()`. Reconciliation starts `h.Connect` once per distinct remembered home or in-flight `host.join`, and cancels a home's context as soon as neither references it. `RecordHome` stores a home and its signalling URLs, reconciles connections and starts state-only sync; for an already hosted group it also updates the sessions and resets group sync for the new home. `host.join` refuses a team group after reading its kind, before cloning or sending `member add_ai`.
- **`host.join` and `host.leave`.** `host.join` verifies the payload and declared `agent` account, connects to `home` and waits at most 15 s (or until `ctx` ends) for `h.Home(home)` before answering `home_unreachable`, obtains the `group` row from `h.Rows` or a coordinated `state_only` `sync` (`syncGroupState`), clones `projects/<group>/repo` with `EnsureGroupWorkspace`, sends `member add_ai` with the stored `approvers` setting, and atomically creates the dormant `GroupSession`, upserts `setting` `'homes'` and records `AuditSetting`. `host.leave` sends `member remove` first so a refusal leaves sessions and connections untouched, and on success ends all sessions of `(g, member)`, prunes `setting` `'homes'` when no local AI remains in `g`, records `AuditSetting`, and reconciles home connections.

`host.join` forwards optional `model`, `effort`, `tier` and `skills` unchanged and permits several members of the same CLI in one group. `host.leave` selects `member` within the given group and agent; omitting it is `bad_request` when more than one distinct member matches. Multiple sessions of one member do not make the selection ambiguous.
- **Lock order and channels.** `joinPart.syncMu` serializes connection reconciliation across `Store` reads; `joinPart.mu` is a leaf lock that guards only in-memory `joinPart` fields and never calls into `Host`, `Sessions`, `Rows`, `Store`, `Connect` or cancel functions. Each waiter channel in `joinPart.waiters` has a single reader, `joinWaitChannel` in `joinAwaitAttach`.

# External AIs

`external.go` and `tools_external.go` implement the host side of the external AI ([external.md](../../planning/contracts/step-3/external.md) sections 2 to 10).

- **Joining.** `host.join` with `agent` `external` needs `account` (`personal`, `team` or `api_key`), takes an optional `tool`, refuses `secretary` `true` and reads no `agent.set`. It sends `member add_ai` with `cli` `external` and that `tool`, creates an ordinary `GroupSession` with `Agent` `external` and no `CLISession`, then reports the state. `host.leave` and `member remove` are unchanged. `Agents` ends with one entry `external`; the node fills its `groups`.
- **Runs.** Delivery never starts a turn for an external member. `ExternalTake` hands out the next batch: it reuses delivery's selection, routing, folder, `delivered` audit and acknowledgement, moves the session row to `Working` and opens the member's one run, numbered by the batch's first item. `task` is `T-<num>` of a task or review session (`turnCtx.taskRef`) and empty for a group run. `context` is the session instructions, `instructions/external.md` and the replaced-session context. `ExternalDone` with the open run moves the row to `Waiting`; with `failed` it reports `turn_interrupted` `crash` with that text, which fails a task session. After `limits.turn_minutes` the run ends the same way with `timeout`, and the process is not stopped. A `done` for any other run changes nothing. A restart reports `crash` through `ReportSaved`.
- **Presence and state.** `ExternalPresent` marks a command or MCP connection open until its `leave`; the member stays present for 180 s after the last one. The reported state is `working` while a run is open, then `offline` (absent or all AIs offline), `paused`, and otherwise `waiting`.
- **Shutdown.** `Settle` (entry `"external"`) sets `settled` under `extPart.mu`, stops the run and presence timers and waits for `extPart.wg`, which runs every timer callback; after that a `leave` or a hand-out arms no timer.
- **Member id.** `ServeMCP` and `RunShim --member <id>` accept the member id of an external AI on this node in place of a session token; an id that names none closes the connection, and removing the member ends its open ones (`untilRemoved`). The caller is the session and the turn (the place of `post`, the depth of a call) of the open run, or the group session without a turn, at level 3, and the connection counts as present. `ExternalWait` serves both `solodawn wait` and `next_item`: it takes a hand-out, or waits until delivery or removal wakes it, the timeout passes or the caller goes; `ExternalEnd` serves `solodawn done` and `done`, and a run that is not open changes nothing.
- **Tools.** An external connection lists every tool of a group, task or review session plus `next_item`, `done`, `report_usage` and `approve`; a tool the open run's kind does not allow answers `conflict` naming the kinds it needs. `report_usage` replaces the member's usage, which `memberUsage` reports in `member status` and the roster.

# Talk and group state tools

`tools_bulletin.go` handles the secretary-only `post.group` argument. It uses
known local groups and remembered homes over the shared connection manager.
Group state is read with `state_only` before checking its kind. Only text
and person mentions are used, and the post is signed as the node owner without
`as`. It carries the source project's id and name and the secretary's name.
An unknown destination lists valid ids and names; non-team destinations are
refused. Team state is read state-only when it is not already cached.
Bulletin `post` waits for the team home's attach, with the same bound as `host.join`, before answering `home_unreachable`.

`tools_talk.go` registers `post`, `read_thread`, `list_members`, `list_tasks`, `note_put` and `note_list` in `Tools` ([mcp-tools.md](../../planning/contracts/mcp-tools.md), sections 1 and 5), and `tools_kb.go` registers `kb_put` and `kb_search` for every session ([step-3/knowledge.md](../../planning/contracts/step-3/knowledge.md), section 5).
`read_thread` gives each message the text `formatMsgContent` makes for a turn, so reviews, checks, briefs and decisions show what their bodies hold; an approval shows its status and the request (`threadMsgText`). `list_members` gives no state for a person.
`post`, `note_put` and `kb_put` (both `name` and `text`) run `CheckOutboundSecret` before signing or sending any command to the home.

# Task and review tools

`forge_tmpl.go` runs custom forge templates as argument lists through the
existing bounded, non-interactive process runner, never through a shell.
Replacement is single-pass; unknown braces remain literal. Custom projects
use remote workspaces and run their push, open, checks and merge commands
instead of GitHub commands. Checks poll at 30-second intervals; exit 8 is
pending. A merge interrupted by a restart is reported as a conflict without
a second attempt. Windows programs must resolve to `.exe` files.

- `tools_task.go` lists `start_task` to working AIs' group sessions, `push_branch`, `run_checks` and `submit_task` to task sessions above level 1, and `submit_review` to review sessions ([mcp-tools.md](../../planning/contracts/mcp-tools.md)).
- `push_branch` and `submit_task` derive `task/<num>-<slug>` from `num` and `slug` via `taskName`, ignoring `TaskRow.Branch`.
- `start_task`, `submit_task` and `submit_review` check outbound text arguments with `CheckOutboundSecret` before running Git or signing commands.
- With a remote forge, `submit_task` and group-state resume (`afterGroupState`) share `startHostedCheck`, which claims `(group, task, commit)` in `Part[tskPart](h)`, runs `TaskForge.WaitHostedChecks` in the background, and sends `task cloud`. Starting a new poll cancels the previous poll for the task. The host retains claims for accepted results and superseded commits so stale rows cannot restart them. An accepted `submit_task` clears its commit's claim before starting the poll. A poll that stopped without acceptance or supersession can be resumed. `Settle` (entry `"task"`) sets `settled` under `tskPart.mu`, cancels the polls and waits for `tskPart.wg`; after that no poll starts.

# Approvals

`approve.go` handles permission requests and multiple-choice user questions from both CLIs (`planning/flows/approval.md`, `planning/contracts/mcp-tools.md` section 3):

- `Tools["approve"]` is listed only to structured sessions of drivers with `Capabilities.Approvals == "tool"` (`usesApproveTool(c.Agent) && !c.Interactive`). It maps `AskUserQuestion` to per-question `decision` cards (`options` `"A"`..`"Z"` plus custom free-text answers, posted one card at a time) returning `updatedInput.answers` (and optional `annotations`), and maps `Bash`, `Write`/`Edit`/`NotebookEdit`/`Read`, `WebFetch`/`WebSearch`, and other tool names to `Ask` (`planning/contracts/messages.md` section 2.3).
- `Listeners["approve"]` starts a goroutine per `Asked` event so `Report` returns at once, brackets the open request synchronously with `Sessions.Calling`, answers Codex `requestUserInput` questions through `Ask.Reply` (posting per-question `decision` cards) and permission requests through `Runner.Answer`, and cancels open `Asked` contexts of a session on `TurnBegan` or `TurnEnded`. The goroutines run on the runtime's wait group: `Settle` (entry `"runtime"`) cancels them all (`cancelAsks`) and waits for them, an `Asked` after that starts none, and a cancelled one answers nothing.
- `thread` and `body.task` are set on `approval` and `decision` cards only for `TaskSession` and `ReviewSession`; a `GroupSession` posts main-line cards with `thread: null` and empty or omitted `task`.
- Permission requests from `callApprove` at level 1 are denied immediately; permission requests raised by a CLI fetch any matching `pending` row's `msg` card from the home (consuming an `approved` card and removing stale `denied`, `expired` or `reassigned` records), check `CheckOutboundSecret` on `Command` and `Why`, shorten `Command` to 3 KiB and `Why` to 256 B as unescaped JSON strings, store a `pending` row, open `Host.Await` (with `nil` check) before sending `post` (`kind: approval`), and wait up to `limits.approval_wait_s` (default 600 s).
- `decodeDecidedInbox` decodes the plain `InboxItem` (`P-1b`) and its `msg` row so `postCardAndWait` reads `card.State` directly; a card that is not decided counts as no decision.
- `Part[approvalState](h)` holds active `Asked` cancel functions per host. Its `mu` is a leaf lock: nothing called under `mu` takes `Host`, `Sessions`, `Rows` or `Store` locks.
- `awaitApprovalItem` is the single reader of the channel returned by `Host.Await`: when the wait timer or context ends, it calls `done` and checks the channel once more before returning the timeout denial.

# Usage, turn limits and offline

`limits.go` registers `"host.offline"` and `"host.online"` in `Surface`, and `"limits"` in `Built`, `Holds`, `Listeners` and `Settled` ([commands.md](../../planning/contracts/commands.md) sections 1.9 and 2, [sessions.md](../../planning/sessions.md) sections 2 and 3, [flows/agent-turn.md](../../planning/flows/agent-turn.md) step 3 and section E). Caps are postponed after 1.0 with `settings.set`: nothing here reads `agent.caps` or sends `cap_reached`.

- **Member status reads memory only.** `Part[limitsPart](h)` keeps the owner's offline switch, each CLI's turns today and its latest rate-limit window. `Built` loads the switch and today's turns once from `setting` `'offline'` and `usage`; `host.offline` and `host.online` write the switch to both, and `TurnEnded` counts the turn in the day's `usage` row and in memory in one step. `Held`, `MemberState` and the `usage` of `member status` (`window_pct`, `window_resets`, `turns_today`) never touch the store, so a status sent while the host shuts down cannot reach a closed store.
- **Offline and online.** `host.offline` sets the switch, closes every session through `Runner.Close` (also during a turn), and sends every AI's home a `member status` with state `offline`, carrying `turn_interrupted` with reason `stop` for an AI whose turn it ended; a stopped home does not fail it (`A4.8`). While offline, `Held` holds every item. `host.online` clears the switch, sends each AI's `member status` and calls `Deliver`.
- **Turn limits.** On `TurnBegan`, a timer of `group.limits.turn_minutes` (default 60) and, for Codex, a 5-minute silent timer, reset by `Active`, `Asked` and `Used` and deferred while a tool call is open ([drivers.md](../../planning/contracts/drivers.md) section 1.2 step 6), interrupt the turn with reason `timeout`, killing the CLI after 10 s; the runtime then sends `turn_interrupted` and, for a task session, `task fail` with reason `timeout` (`A4.9`). A pause interrupts with reason `stop` and never fails the task.
- **Vendor rate limits.** `RateLimited` holds the CLI's items until `resets`, then sends `member status` and calls `Deliver`. `MemberState` shows the `offline` hold as `offline` and the `rate_limit` hold as `limited`; a takeover's hold is no overlay (the session's own state shows it). Its `rate_limited` event goes with the status that ends a running turn, so that it cannot overtake it, or at once outside a turn.
- **Shutdown.** `Settle` (entry `"limits"`) sets `settled` under `limitsPart.mu`, stops the timers and waits for `limitsPart.wg`; after that a rate-limit release or event starts nothing and arms no timer.
- **Lock order.** `limitsPart.mu` is a leaf lock over the maps and timer handles; it is never held across `Store.Write`, `Sessions`, `Rows`, `Host` or runtime locks.
