---
description: 'Assembly of the SoloDawn node: foreground runtime, data directory, single-instance local socket, CLI subcommand dispatch and MCP shim, start-at-login services, verified self-update, verified device logs, the ctl session hub, the in-process local link between host and home, and the nodetest multi-node harness.'
kind: reference
---

# Node

## Summary

`internal/node` assembles the components of a SoloDawn node ([architecture](../../planning/architecture.md), section 8):

- `node.go` and `proc_*.go`: `Foreground()` runtime assembly (`openNode`, `runNode`), `logs/node.log` rotation at 10 MiB (keeping `.1`), `device.key` and `node.db` initialization, `home.StartLoop` (`OwnerFn: logs.Owner`), `host.Host` + `ServeMCP` on the local socket, `AttachLocal`, `Logs` + `Sessions`, start-at-login registration on adoption (`InstallService`), `peer.Peer`, embedded local web UI (`localhost:47800` with `:0` fallback) + `/rv` local signalling path, and persistent remote rendezvous connections ([local](../../planning/contracts/local.md), sections 1-3 and 10).
- `dir.go` and `socket*.go`: per-user data directory, exclusive lock on `lock`, and owner-only local Unix socket (`sock`) carrying local CLI commands and MCP sessions ([local](../../planning/contracts/local.md), sections 2 and 3).
- `cmd.go`: the `Subcommands` table, `MCP` (`solodawn mcp --token <t>`), and local command exchange (`solodawn pair` auto-starts a detached background node when `sock` is absent).
- `service_*.go`: start-at-login registration on macOS, Linux and Windows ([local](../../planning/contracts/local.md), section 1).
- `update.go`: `Updater` for SHA-256-verified self-update outside running turns and takeovers ([local](../../planning/contracts/local.md), section 9; postponed off the main line).
- `rendezvous.go`: standalone signalling, static web assets and ICE service.
- `devlog.go`: `Logs`, loading and persisting `setting` (`key = 'owner'`, JSON-encoded string) and `device_log` for the node's owner, unowned-node adoption across split frames, forwarding accepted entries in `devlog` frames up to 61,440 bytes, and delivering accepted `device` rows to `home.Loop.Event("device", rows)` ([envelope](../../planning/contracts/envelope.md), sections 4 and 6; [datachannel](../../planning/contracts/datachannel.md), section 3.1).
- `session.go`: `Sessions`, serving inbound `ctl` connections (`*peer.Connection` via `Ctl`), `hello` and `welcome`, the bootstrap rule before enrolment, request (`sync`, `fetch`) and push (`devlog`, `ack`) dispatch, delegation to `home.Loop` and `host.Surface`, and asynchronous execution with envelope checks, in-flight nonce coordination and `receipt` persistence for host commands ([datachannel](../../planning/contracts/datachannel.md), sections 2-5, 7 and 8; [commands](../../planning/contracts/commands.md), "Once only" and section 2).
- `local.go`: `AttachLocal` and `LocalLink`, the in-process link between a `*host.Host` and a `*home.Loop` on the same node ([architecture](../../planning/architecture.md), section 2.1).
- `nodetest/`: in-process node clusters with built fake Claude Code and Codex binaries for integration tests.

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
| wire a host to the home on the same node | `AttachLocal(h, loop)` returning `*LocalLink` (`host.Home` with `Path() == "local"`) |
| start in-process nodes with fake CLIs | `nodetest.Start(t, cfg)` |

## Local link

`AttachLocal(h, loop)` attaches `h.Key.ID()` to `loop` as a `home.Client`, registers the `*LocalLink` on `h` via `h.Attach`, and calls `loop.Inbox` for the node's device.

- **Framing.** Every frame in either direction is encoded as JSON without HTML escaping. `host.Command` checks the `ctl` message limit before sending, and the home loop splits `rows` pushes within the limit ([conventions.md](../../planning/contracts/conventions.md) section 4).
- **Requests and pushes (`Host` to `Home`).** `Request` assigns an increasing request `id`, sends the built frame, and waits on a per-request single-reader channel until the matching `re` arrives, `Done()` closes, or `ctx` is canceled. `cmd`, `sync` and `fetch` requests dispatch to `loop.Cmd`, `loop.Sync` and `loop.Fetch`, and `ack` pushes dispatch to `loop.Ack`; any other frame carrying an integer `id` is answered `bad_request` ([datachannel.md](../../planning/contracts/datachannel.md) section 2), and any other frame without an `id` is dropped.
- **Answers and pushes (`Home` to `Host`).** Frames carrying `re` wake the waiting `Request` call or are dropped if none waits; frames without `re` are delivered to `h.Push(device, frame)`. `h.Push` runs on the loop's send goroutine for this connection, so a `Pushes` handler must not wait for an answer from the same home, because that answer is sent by the goroutine it blocks.
- **Lock order and shutdown.** `LocalLink.mu` guards only the request counter and waiting map and is never held while calling into `host` or `home`. The only way to lose the local link is `Close()`, which calls `loop.Detach`, closes `Done()` (`conn.Gone()`), and wakes any waiting `Request` with `io.EOF`.

## Multi-node test helper (`nodetest`)

`nodetest.Start(t, cfg)` creates an owner key, loads a `device.key` per node, signs an owner device log enrolling the owner and every node, seeds `setting("owner")` and `device_log` in each node's SQLite store, starts `home.StartLoop` and `host.New`, and connects each node's own host and home with `AttachLocal`. Cross-node links between nodes in a cluster come with `N-2b` and `N-4`. When `Config.BinDir` is empty, `Start` builds `internal/host/fakecli/claude` and `internal/host/fakecli/codex` into `Root/bin`, prepends `BinDir` to `PATH`, and sets `SOLODAWN_FAKECLI_LOG`.

## Plugging into `Sessions`

`NewSessions` registers `sync` and `fetch` (`Async: true` delegating to `home.Loop.Sync` and `home.Loop.Fetch`) by default. Populate additional entries on `*Sessions` before accepting connections:

| Map | Purpose |
| --- | --- |
| `Requests` | `ctl` requests (`sync`, `fetch` with `Async: true`; `host.info` with `Type: "host.info"`) |
| `Pushes` | `ctl` pushes after enrolment beyond `devlog` and `ack` (`pty.resize`) |
| `Commands` | node-level host commands (`pair.browser`, `settings.set`); other host commands (`to` set in payload) fall back to `host.Surface` |

`SessionConfig.OnAdopt` is invoked once with the adopted owner's user id when an unowned node persists its first valid adoption log (`dvlAdopt`), before forwarding `devlog` frames. `nodeRuntime.onAdopt` registers the node to start at login (`InstallService`), while `home.Loop` reads the dynamic owner through `LoopConfig.OwnerFn` (`logs.Owner`).

## Concurrency and lock order

- `nodeRuntime.bg` waits for all inbound `ctl` sessions and rendezvous loops before `Close` shuts `LocalLink`, `home.Loop` and `Store`; `nodeRuntime.mu` is a leaf lock guarding `rvPaths` and `rvStop`.
- `Sessions.cmdMu` guards `inflight` (keyed by signer and nonce) and receipt reads/writes in `Store.Write`; host command handlers run on their own goroutines outside `cmdMu`, and duplicate `(signer, nonce)` requests wait on the in-flight channel before reading the stored receipt.
- `Logs.writeMu` serializes device-log mutations and is acquired before `Store.Write` and before `Logs.mu`; `dvlNotify` runs after `writeMu` is released.
- `Logs.mu` (`sync.RWMutex`) guards `owner`, `log` and `onChange`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls, `OnAdopt` or network writes.
- `Sessions.mu` guards the active connection map (`conns`) and `sesConn.welcomed`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls or network writes.
- `sesConn.sendMu` serializes outbound frames on one `Ctl` connection; during `welcome` delivery it holds `sendMu` across `raw.Send(welcome)`, setting `welcomed = true` under `Sessions.mu`, reading `Logs` under `Logs.mu`, and sending the initial `devlog` frames so no `devlog` frame is forwarded ahead of `welcome`.

## Known limitations

- `Pairer`, `host.info`, `pair.browser` and `settings.set` wait for `N-3` ([issues](../../issues.md)).
- `Sessions` implements only the server side of an inbound `ctl` connection until `N-2b` (`internal/node/client.go`) merges onto `rebuild` ([issues](../../issues.md)).
- Self-update (`Updater`, `N-6`) is postponed off the step-1 main line ([issues](../../issues.md)).
- In step 1 `Logs` stores only the node owner's device log; keeping logs of other group members belongs to step 2.
- On macOS, Unix socket paths are limited to 103 bytes, so tests that bind `sock` under `HOME` use `/tmp` ([issues](../../issues.md)).
- Cross-node `ctl` links between hosts and remote homes wait for `N-2b`.

## Verification and sources

`go test ./internal/node/ ./internal/node/nodetest/` covers every statement of both packages, including foreground runtime startup, port fallback, log rotation, adoption login-service registration, rendezvous reconnects, local socket locking, opening lines, MCP shim dispatch, `pair` auto-start, service definitions, `Updater` verification, `LocalLink` `cmd`, `sync`, `fetch` and `ack` framing, inbox delivery for groups created after `AttachLocal`, cancellation and closure, and `nodetest` startup and fake CLI replay. Acceptance scenarios live in `features/node.feature`, `features/node_service.feature`, `features/node_session.feature`, `features/node_update.feature` and `features/node_local.feature`.
