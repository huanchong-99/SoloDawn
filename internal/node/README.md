---
description: 'Assembly of the SoloDawn node: foreground runtime, data directory, single-instance local socket, CLI subcommand dispatch and MCP shim, start-at-login services, verified self-update, verified device logs, the ctl session hub, room pairing and pair.browser, the node-level host surface, the in-process local link between host and home, and the nodetest multi-node harness.'
kind: reference
---

# Node

## Summary

`internal/node` assembles the components of a SoloDawn node ([architecture](../../planning/architecture.md), section 8):

- `node.go` and `proc_*.go`: `Foreground()` runtime assembly (`openNode`, `runNode`), `logs/node.log` rotation at 10 MiB (keeping `.1`), `device.key` and `node.db` initialization, `home.StartLoop` (`OwnerFn: logs.Owner`), `host.Host` + `ServeMCP` on the local socket, `AttachLocal`, `Logs` + `Sessions` + `Surface` + `Pairer`, start-at-login registration on adoption (`InstallService`), `peer.Peer` + `Host.Connect`, embedded local web UI (`localhost:47800` with `:0` fallback) + `/rv` local signalling path, and persistent remote rendezvous connections ([local](../../planning/contracts/local.md), sections 1-3 and 10).
- `diag.go`: the node's diagnostic events, the level file and the form of the log ([diagnostics](../../planning/contracts/diagnostics.md)).
- The local socket and the loopback web server accept through `peer.Steady`: an accept error is logged and does not end them.
- `dir.go` and `socket*.go`: per-user data directory, exclusive lock on `lock`, and owner-only local Unix socket (`sock`) carrying local CLI commands and MCP sessions ([local](../../planning/contracts/local.md), sections 2 and 3).
- `cmd.go`: the `Subcommands` table, `MCP` (`solodawn mcp --token <t>` or `--member <id>`), and local command exchange (`solodawn pair` auto-starts a detached background node when `sock` is absent).
- `export.go`: `solodawn group export <group id> <file> [--to <device id>]` and `solodawn group import <file>` use the running node's local socket. Export without `--to` keeps the group on the old node; export with it makes the old node answer `moved`. Import requires a node of the same owner and no existing rows for that group. Files resolve relative to the CLI's working directory.
- `external.go`: `solodawn wait --member <id> [--timeout <s>]` prints one hand-out line and exits 0, exits 3 when there is none, 1 on a node or member error and 2 on bad arguments; `solodawn done --member <id> --run <n> [--failed <text>]` ends the run. Both go over the local socket to `Host.ExternalWaitLines` and `Host.ExternalEnd` ([external.md](../../planning/contracts/step-3/external.md) section 3).
- `service_*.go`: start-at-login registration on macOS, Linux and Windows ([local](../../planning/contracts/local.md), section 1).
- `update.go`: `Updater` for SHA-256-verified self-update outside running turns and takeovers ([local](../../planning/contracts/local.md), section 9; postponed off the main line).
- `rendezvous.go`: standalone signalling, static web assets and ICE service.
- `devlog.go`: `Logs`, loading and persisting `setting` (`key = 'owner'`, JSON-encoded string) and `device_log` for the node's owner, unowned-node adoption across split frames, node-signed device additions during browser pairing, forwarding accepted entries in `devlog` frames up to 61,440 bytes, and delivering accepted `device` rows to `home.Loop.Event("device", rows)` ([envelope](../../planning/contracts/envelope.md), sections 4 and 6; [datachannel](../../planning/contracts/datachannel.md), section 3.1).
- `session.go`: `Sessions`, serving inbound `ctl` connections (`*peer.Connection` via `Ctl`), `hello` and `welcome`, the bootstrap rule before enrolment, request (`sync`, `fetch`, `host.info`), push (`devlog`, `ack`), command (`pair.browser`) and replay tables, delegation to `home.Loop` and `host.Surface`, and asynchronous execution with envelope checks, in-flight nonce coordination and `receipt` persistence for host commands ([datachannel](../../planning/contracts/datachannel.md), sections 2, 3, 5, 7 and 8; [commands](../../planning/contracts/commands.md), "Once only" and section 2).
- `client.go`: `clientCtl`, `dialFunc` and `(*Sessions).serveClient`, the client side of a host's outgoing `ctl` session to a home on another node ([datachannel](../../planning/contracts/datachannel.md), sections 2, 3 and 3.1; [flows/connect.md](../../planning/flows/connect.md), steps 11 to 18).
- `teams.go`: `(*Sessions).connectHome` and `home.add`, using the host connection manager for every remembered home, also of a group with no local AI. Connections sync state-only as the owner's device ([people](../../planning/contracts/step-2/people.md), section 7, point 2; [commands](../../planning/contracts/commands.md), section 2).
- `pair.go`: `Pairer`, running 10-minute single-use pairing rooms in role `b` (`Pair`, onboarding and adding another computer) and role `a` (`PairBrowser` and `pair.browser`, adding another browser with `#pair=<secret>` links and once-only nonce replay; [pairing](../../planning/flows/pairing.md), sections A to C; [envelope](../../planning/contracts/envelope.md), section 6).
- `surface.go`: `Surface`, serving `host.info` (`HostState`, including live `QueuedCount`, sorted granted `watchers` and the `workspace` of the AI's group session per `(group, member)`), forwarding `host.workspaces` requests to `Host.Workspaces` and `watch.start` requests to `Host.WatchStart` and `pty.resize` pushes to `host.Surface["pty.resize"]`, replaying `takeover.start` via `Host.ReplayTakeover`, and reading `name`, `max_open`, `offline`, `homes` and `rendezvous` (`ReadName`, `ReadRendezvous`; [datachannel](../../planning/contracts/datachannel.md), sections 3 and 7; [watch-team](../../planning/contracts/step-2/watch-team.md), sections 2-6).
- `netstatus.go`: registers the `net_status` MCP tool in `host.Tools` and reports welcomed `ctl` paths, the local NAT flags, and configured rendezvous reachability ([mcp-tools](../../planning/contracts/mcp-tools.md), section 1).
- `schedule.go`: runs `home.Retain` on `node.db` at startup and every 24 hours ([storage](../../planning/contracts/storage.md), sections 2 and 4).
- `local.go`: `AttachLocal` and `LocalLink`, the in-process link between a `*host.Host` and a `*home.Loop` on the same node ([architecture](../../planning/architecture.md), section 2.1).
- `nodetest/`: in-process node clusters with built fake Claude Code and Codex binaries for integration tests.

Startup probes the host's CLI versions and model listings. `host.info` reuses the shared 60-second cache; `refresh:true` probes again and pushes the resulting `host.info` to welcomed devices of the owner. `surface_models_test.go` covers refresh forwarding, push recipients, and malformed or failed refreshes.

## Capabilities

| Need | Entry |
| --- | --- |
| locate the user's data directory | `DataDir()` ([local.md](../../planning/contracts/local.md) section 2) |
| hold the single-node lock and bind `sock` | `ListenLocal(dir)`, `(*Local).Close()` ([local.md](../../planning/contracts/local.md) section 3) |
| read the first line of a local socket client | `AcceptOpening(conn)`, `ReadOpening(r)`, `ReadPair(line, owner, configured)` |
| register or send a local CLI subcommand | `Subcommands`, `Command.Run(stdout)`, `MCP(args, stdin, stdout, stderr)` |
| run the assembled foreground node | `Foreground()` ([local.md](../../planning/contracts/local.md) sections 1-3, 10) |
| install or remove start-at-login | `InstallService`, `RemoveService` (`service_*.go`, [local.md](../../planning/contracts/local.md) section 1) |
| check and apply signed-sum release updates | `(*Updater).Check(ctx, now)`, `(*Updater).Apply()` ([local.md](../../planning/contracts/local.md) section 9) |
| run the standalone rendezvous and static web service | `Rendezvous(opts)` |
| pair a node over a rendezvous room or issue a browser pairing link | `NewPairer(cfg)`, `(*Pairer).Pair(ctx, req)`, `(*Pairer).PairBrowser(ctx, webURL)`, `(*Pairer).HandleEvent(ctx, path, kind, data)`, `(*Pairer).Serve(ctx, device, raw)` ([pairing.md](../../planning/flows/pairing.md)) |
| serve `host.info` and read node settings | `NewSurface(cfg)`, `(*Surface).HostState(ctx)`, `ReadName(db)`, `ReadRendezvous(db)` ([datachannel.md](../../planning/contracts/datachannel.md) section 7) |
| wire a host to the home on the same node | `AttachLocal(h, loop)` returning `*LocalLink` (`host.Home` with `Path() == "local"`) |
| run an outgoing host `ctl` session to a remote home | `(*Sessions).serveClient(ctx, device, raw)` and `(*Sessions).connectHome(dial)` |
| start in-process nodes with fake CLIs | `nodetest.Start(t, cfg)`, `(*nodetest.Node).StopHome()`, `(*nodetest.Node).RestartHome()`, `nodetest.SignDeviceAdd(...)` |

## Local link

`AttachLocal(h, loop)` attaches `h.Key.ID()` to `loop` as a `home.Client`, registers the `*LocalLink` on `h` via `h.Attach`, and calls `loop.Inbox` for the node's device.
The node runtime also subscribes its `LocalLink` to `groups` pushes.

- **Framing.** Every frame in either direction is encoded as JSON without HTML escaping. `host.Command` checks the `ctl` message limit before sending, and the home loop splits `rows` pushes within the limit ([conventions.md](../../planning/contracts/conventions.md) section 4).
- **Requests and pushes (`Host` to `Home`).** `Request` assigns an increasing request `id`, sends the built frame, and waits on a per-request single-reader channel until the matching `re` arrives, `Done()` closes, or `ctx` is canceled. `cmd`, `sync` and `fetch` requests dispatch to `loop.Cmd`, `loop.Sync` and `loop.Fetch`, and `ack` pushes dispatch to `loop.Ack`; any other frame carrying an integer `id` is answered `bad_request` ([datachannel.md](../../planning/contracts/datachannel.md) section 2), and any other frame without an `id` is dropped.
- **Answers and pushes (`Home` to `Host`).** Frames carrying `re` wake the waiting `Request` call or are dropped if none waits; frames without `re` are delivered to `h.Push(device, frame)`. `h.Push` runs on the loop's send goroutine for this connection, so a `Pushes` handler must not wait for an answer from the same home, because that answer is sent by the goroutine it blocks.
- **Lock order and shutdown.** `LocalLink.mu` guards only the request counter and waiting map and is never held while calling into `host` or `home`. The only way to lose the local link is `Close()`, which calls `loop.Detach`, closes `Done()` (`conn.Gone()`), and wakes any waiting `Request` with `io.EOF`.

## Outgoing host `ctl` client

`(*Sessions).serveClient(ctx, device, raw)` runs one outgoing `ctl` session from this node's host to a remote home `device` over `raw` (`clientCtl`, satisfied by `*peer.Connection`):

- **Reader-first handshake and `devlog` order.** Starts the `Read` loop before sending `hello` (`v: 3`, `kind: "host"`, with `Name` and `Addrs`, under a 10 s timeout) because `*peer.Connection.Read` routes answers after earlier queued frames. Version 3 is the only one, in both directions: a `hello` with another `v` is answered `version`, and a `version` answer from a home ends the handshake with a report that this node and that computer must be updated together; there is no retry with a lower `v` ([datachannel](../../planning/contracts/datachannel.md), section 3). On success it sends this node's owner `devlog` frames under `sendMu` before calling `Host.Attach(device, raw)` and closing `ready`.
- **Pushes and live `devlog` forwarding.** Incoming `devlog` frames apply to `Logs` at once (and forward to other connected sessions without echoing back to the sender); other frames wait on `ready` until `Host.Attach` has returned and then call `Host.Push(device, frame)` synchronously in read order, so every push sent before an answer is applied before `Request` returns.
- **`connectHome(dial)`.** Returns a non-blocking `Host.Connect` function that ignores this node's own device id (served by `AttachLocal`) and runs `dial` in the background, calling `serveClient` on each opened connection until `ctx` ends.

## Invitations and group homes

A home `invite` opens an in-memory pairing room for one group. Unlike `pair.browser`, a `#join` invitation never enrols a device of the node's owner. The joining connection only accepts `hello`, its person's `devlog` and `join` for the invitation's group. After the home commits the join, the person keeps their own devices and gains that group's home surface, not the host surface. The join is judged on the person's log as this node holds it plus what the presented log adds (`Logs.dvlWith`, first accepted wins), and after it `Logs.dvlKeep` takes the new entries as received ones: stored, held and raised as the `device` event; an import takes the file's logs the same way. Stored device logs are sent only to peers that can read a group containing the person; the owner's log is also sent to enrolled or pairing owner devices.

`nodeRuntime.knownPeer` decides who may be connected: it admits signalling, and `SessionConfig.Known` gives it to `Sessions`. The home pushes `groups` to everyone after a removal, so `sesConn.sendGroups` asks it at each push: to a device that has no reason left the push is the last frame, and `Ctl.Finish` closes the connection once it has arrived. Before that push `LoopConfig.Forget` (`Logs.dvlForget`) drops the log of a person who is in no group homed here. `sesLogChanged` asks it for every connection when a device log changes, and closes a revoked device at once, also on a node that homes no group (`planning/flows/pairing.md`, D.3). A joining connection stays until its invitation ends.

The owner's devices can send `home.add` to record a group's home and its signalling URLs without joining or cloning it. If the node already hosts sessions for that group, their home changes and the host reconnects to it.

## Multi-node test helper (`nodetest`)

`nodetest.Start(t, cfg)` creates an owner key, loads a `device.key` per node, signs an owner device log enrolling the owner, every node, and any `Config.ExtraBrowsers`, seeds `setting("owner")`, `setting("name")` and `device_log` in each node's SQLite store, starts `home.StartLoop` and `host.New`, and connects each node's own host and home with `AttachLocal` (`StopHome` and `RestartHome` stop and reopen the home loop and local link on the same store). When `Config.BinDir` is empty, `Start` builds `internal/host/fakecli/claude` and `internal/host/fakecli/codex` into `Root/bin`, prepends `BinDir` to `PATH`, and sets `SOLODAWN_FAKECLI_LOG`.

## Plugging into `Sessions`

`NewSessions` registers `sync` and `fetch` (`Async: true` delegating to `home.Loop.Sync` and `home.Loop.Fetch`) by default. Attach `NewPairer` and `NewSurface` (or populate additional entries) before accepting connections:

| Map | Purpose |
| --- | --- |
| `Requests` | `ctl` requests (`sync`, `fetch` with `Async: true`; `host.info` registered by `NewSurface` with `Type: "host.info"`; `watch.start` registered by `NewSurface` with an untyped `ok` answer after the host checks the grant and direct path) |
| `Pushes` | `ctl` pushes after enrolment beyond `devlog` and `ack` (`pty.resize` registered by `NewSurface`) |
| `Commands` | node-level host commands (`pair.browser` registered by `NewPairer`); other host commands (`to` set in payload), including `watch.end` to revoke a user's grant and close their channels, fall back to `host.Surface` |
| `Replays` | memory-backed replay answers for succeeded host commands (`pair.browser` registered by `NewPairer`, returning the active link while valid and `conflict` once used, expired or lost across restart; `takeover.start` registered by `NewSurface`, answering `ok` while that terminal remains open on the same connection and `conflict` otherwise) |

`SessionConfig.OnAdopt` is invoked once with the adopted owner's user id when an unowned node persists its first valid adoption log (`dvlAdopt`), before forwarding `devlog` frames. `SessionConfig.Spawn` starts a host command; `nil` runs it on a bare goroutine. `nodeRuntime.onAdopt` registers the node to start at login (`InstallService`), while `home.Loop` reads the dynamic owner through `LoopConfig.OwnerFn` (`logs.Owner`). `home.Loop.Welcome` and `home.Loop` group announcements deliver `welcome.groups` and `groups` pushes over `Sessions`.

Non-owner devices may send `watch.start` as well as `sync` and `fetch`. A device whose person holds a watch grant is admitted by `knownPeer` and session enrolment even without a locally known device log. It receives the ordinary peer welcome (`owner: false`, readable groups often empty), may send `hello` and `devlog`, and may start and open its granted watch channels. `Host.WatchEnded` closes that person's device sessions, including their peer connections, after the last grant ends unless `knownPeer` still accepts them for another reason. `Host.WatchersChanged` pushes `host.info` to the owner's connected devices, on a goroutine of `nodeRuntime.bg` (`host_info`), each time a watch grant is added or ends.

## Concurrency and lock order

- `nodeRuntime.bg` waits for all inbound `Pairer.Serve` sessions, outgoing `serveClient` sessions, rendezvous loops, the daily retention schedule (`startRetention`), and host commands, which `Sessions` starts through `SessionConfig.Spawn`, before `Close` shuts `LocalLink`, `home.Loop` and `Store`; `nodeRuntime.mu` is a leaf lock guarding `closing`, `rvPaths` and `rvLoops`.
- `Sessions.cmdMu` guards `inflight` (keyed by signer and nonce) and receipt reads/writes in `Store.Write`; host command handlers and `Replays` handlers run outside `cmdMu`, and duplicate `(signer, nonce)` requests wait on the in-flight channel before reading the stored receipt.
- `Surface.pushing` serializes the pushes of `host.info`, each from reading the state to the last send, so that the last one a device gets is the newest; no other lock is held when it is taken.
- `Pairer.mu` guards active `attempts` and nonce-keyed `links`; it is a leaf lock never held across `Store.Write`, `Logs`, `Sessions`, `Peer` or WebSocket operations. Each `pairAttempt.outcome` channel has a single reader (`waitPair` for role `b`, `watchBrowser` for role `a`).
- `Logs.writeMu` serializes device-log mutations and is acquired before `Store.Write` and before `Logs.mu`; `dvlNotify` runs after `writeMu` is released.
- `Logs.mu` (`sync.RWMutex`) guards `owner`, `log` and `onChange`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls, `OnAdopt` or network writes.
- `Sessions.mu` guards the active connection map (`conns`) and `sesConn.welcomed`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls or network writes.
- `sesConn.sendMu` serializes outbound frames on one `Ctl` connection; during `welcome` delivery (`Serve`) and initial client `devlog` delivery (`serveClient`) it holds `sendMu` across setting `welcomed = true` under `Sessions.mu`, reading `Logs` under `Logs.mu`, and sending the initial `devlog` frames so no concurrent `devlog` update overtakes the initial batch.

## Runtime wiring (`node.go`)

`openNode` wires pairing, `host.info` and cross-node sessions onto the running node:

1. Inbound `ctl` connections go through `Pairer.Serve` inside `n.bg`, marking room-paired peers `paired` for adoption and `onPairHello`.
2. `peer.New`'s known-device check and route lookup combine `Logs.Known` with `Pairer.Known` and prepend `Pairer.Routes` to the node's shared rendezvous paths.
3. `Pairer.HandleEvent` receives `joined`, `full` and `room` frames from both the local `/rv` path and remote rendezvous paths, and `PairConfig.Paths` returns `n.sharedPaths`.
4. `Pairer.Peer` and `Host.Connect` (`Sessions.connectHome`) are set right after `peer.New`.
5. Local socket `pair` persists `--rendezvous` first, runs `syncRendezvous` and calls `Pairer.Pair`; `pair-browser` and `open` call `Pairer.PairBrowser`.
6. `SurfaceConfig.Agents` is set to `(*Host).Agents` (`HS-3`), and `peer.New` registers `pty:` channels onto `(*Host).ServeTerminal` (`HS-19`) and receive-only `watch:` channels onto `(*Host).ServeWatch` (`T2-6`). Watch grants and channel lifetimes belong to the host; `host.info` exposes them as `agents[].groups[].watchers`.
7. `netStatusPart` is wired to `(*nodeRuntime).netStatus` (`netstatus.go`), and `stepStartup` launches `startRetention` (`schedule.go`).

## Diagnostics

`diag.go` writes the node's events of [diagnostics](../../planning/contracts/diagnostics.md) section 2
through `log/slog`, with a logger made in `stepIdentity` that holds `node`, the first 8 characters of
the device id. `node.start` (`stepIdentity`) and `node.stop` (the start of `Close`): `version`, `os`,
`level`; `home.dial` (`dialHome`, the dial that `connectHome` starts): `home`, cut to 8 characters;
`bg.start` and `bg.stop` (`name`) around every goroutine that a wait group of the runtime starts:
`home_dial`, `home_session`, `host_cmd`, `host_info` (a push of `host.info`), `inbound`, `local` (one
local socket connection), `local_rv`, `rendezvous`, `retention`. The goroutine writes its own start, because the caller may hold a mutex,
and its stop before its wait group's `Done`. `stepLog` sets the flags of `log` (date, time with
microseconds, UTC) and the level, which is `debug` when `logs/level` holds the word `debug` and `info`
otherwise; `closeCore` puts back the output, the flags and the level.

## Known limitations

- `Surface` answers `host.info` requests and forwards `pty.resize` pushes; it pushes `host.info` after a request with `refresh` and when `watchers` change, not when another field does; `settings.set` is postponed with `U-12`.
- `net_status` omits `rtt_ms` (`*peer.Connection` exposes `Path()` without ICE candidate-pair round-trip statistics) and reports `mapped_ports` as `false` in step 1 ([issues](../../issues.md)).
- Self-update (`Updater`, `N-6`) is postponed off the step-1 main line ([issues](../../issues.md)).
- On macOS, Unix socket paths are limited to 103 bytes, so tests that bind `sock` under `HOME` use `/tmp` ([issues](../../issues.md)).

## Verification and sources

`go test ./internal/node/ ./internal/node/nodetest/` covers every statement of both packages, including foreground runtime startup, port fallback, log rotation, the diagnostic events (`diag_test.go`), adoption login-service registration, rendezvous reconnects, room pairing in both roles, `pair.browser` single-use expiry and nonce replay, `host.info` state assembly, local socket locking, opening lines, MCP shim dispatch, `pair` auto-start, service definitions, `Updater` verification, `LocalLink` `cmd`, `sync`, `fetch` and `ack` framing, inbox delivery for groups created after `AttachLocal`, cancellation and closure, and `nodetest` startup and fake CLI replay. Acceptance scenarios live in `features/node.feature`, `features/node_service.feature`, `features/node_update.feature`, `features/node_local.feature`, `features/node_session.feature` and `features/node_pair.feature`.
