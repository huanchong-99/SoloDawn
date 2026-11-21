---
description: 'Assembly of the SoloDawn node: data directory, single-instance local socket, CLI subcommand dispatch, start-at-login services, verified self-update, verified device logs, the ctl session hub, the in-process local link between host and home, and the nodetest multi-node harness.'
kind: reference
---

# Node

## Summary

`internal/node` assembles the components of a SoloDawn node ([architecture](../../planning/architecture.md), section 8):

- `dir.go` and `socket*.go`: per-user data directory, exclusive lock on `lock`, and owner-only local Unix socket (`sock`) carrying local CLI commands and MCP sessions ([local](../../planning/contracts/local.md), sections 2 and 3).
- `cmd.go`: the `Subcommands` table and local command exchange.
- `service_*.go`: start-at-login registration on macOS, Linux and Windows ([local](../../planning/contracts/local.md), section 1).
- `update.go`: `Updater` for SHA-256-verified self-update outside running turns and takeovers ([local](../../planning/contracts/local.md), section 9).
- `rendezvous.go`: standalone signalling, static web assets and ICE service.
- `devlog.go`: `Logs`, loading and persisting `setting` (`key = 'owner'`, JSON-encoded string) and `device_log` for the node's owner, unowned-node adoption across split frames, forwarding accepted entries in `devlog` frames up to 61,440 bytes, and delivering accepted `device` rows to `home.Loop.Event("device", rows)` ([envelope](../../planning/contracts/envelope.md), sections 4 and 6; [datachannel](../../planning/contracts/datachannel.md), section 3.1).
- `session.go`: `Sessions`, serving inbound `ctl` connections (`*peer.Connection` via `Ctl`), `hello` and `welcome`, the bootstrap rule before enrolment, request and push tables, delegation to `home.Loop` and `host.Surface`, and asynchronous execution with envelope checks, in-flight nonce coordination and `receipt` persistence for host commands ([datachannel](../../planning/contracts/datachannel.md), sections 2, 3, 5, 7 and 8; [commands](../../planning/contracts/commands.md), "Once only" and section 2).
- `client.go`: `clientCtl`, `dialFunc`, `(*Sessions).serveClient` and `(*Sessions).connectHome`, the client side of a host's outgoing `ctl` session to a home on another node ([datachannel](../../planning/contracts/datachannel.md), sections 2, 3 and 3.1; [flows/connect.md](../../planning/flows/connect.md), steps 11 to 18).
- `local.go`: `AttachLocal` and `LocalLink`, the in-process link between a `*host.Host` and a `*home.Loop` on the same node ([architecture](../../planning/architecture.md), section 2.1).
- `nodetest/`: in-process node clusters with built fake Claude Code and Codex binaries for integration tests.

## Capabilities

| Need | Entry |
| --- | --- |
| locate the user's data directory | `DataDir()` ([local.md](../../planning/contracts/local.md) section 2) |
| hold the single-node lock and bind `sock` | `ListenLocal(dir)`, `(*Local).Close()` ([local.md](../../planning/contracts/local.md) section 3) |
| read the first line of a local socket client | `AcceptOpening(conn)`, `ReadOpening(r)`, `ReadPair(line, owner, configured)` |
| register or send a local CLI subcommand | `Subcommands`, `Command.Run(stdout)` |
| install or remove start-at-login | `InstallService`, `RemoveService` (`service_*.go`, [local.md](../../planning/contracts/local.md) section 1) |
| check and apply signed-sum release updates | `(*Updater).Check(ctx, now)`, `(*Updater).Apply()` ([local.md](../../planning/contracts/local.md) section 9) |
| run the standalone rendezvous and static web service | `Rendezvous(opts)` |
| wire a host to the home on the same node | `AttachLocal(h, loop)` returning `*LocalLink` (`host.Home` with `Path() == "local"`) |
| run an outgoing host `ctl` session to a remote home | `(*Sessions).serveClient(ctx, device, raw)` and `(*Sessions).connectHome(dial)` |
| start in-process nodes with fake CLIs | `nodetest.Start(t, cfg)` |

## Local link

`AttachLocal(h, loop)` attaches `h.Key.ID()` to `loop` as a `home.Client`, registers the `*LocalLink` on `h` via `h.Attach`, and calls `loop.Inbox` for the node's device.

- **Framing.** Every frame in either direction is encoded as JSON without HTML escaping. `host.Command` checks the `ctl` message limit before sending, and the home loop splits `rows` pushes within the limit ([conventions.md](../../planning/contracts/conventions.md) section 4).
- **Requests (`Host` to `Home`).** `Request` assigns an increasing request `id`, sends the built frame, and waits on a per-request single-reader channel until the matching `re` arrives, `Done()` closes, or `ctx` is canceled. `cmd` requests dispatch to `loop.Cmd`; any other frame carrying an integer `id` is answered `bad_request` ([datachannel.md](../../planning/contracts/datachannel.md) section 2), and frames without an `id` are dropped.
- **Answers and pushes (`Home` to `Host`).** Frames carrying `re` wake the waiting `Request` call or are dropped if none waits; frames without `re` are delivered to `h.Push(device, frame)`. `h.Push` runs on the loop's send goroutine for this connection, so a `Pushes` handler must not wait for an answer from the same home, because that answer is sent by the goroutine it blocks.
- **Lock order and shutdown.** `LocalLink.mu` guards only the request counter and waiting map and is never held while calling into `host` or `home`. The only way to lose the local link is `Close()`, which calls `loop.Detach`, closes `Done()` (`conn.Gone()`), and wakes any waiting `Request` with `io.EOF`.

## Outgoing host `ctl` client

`(*Sessions).serveClient(ctx, device, raw)` runs one outgoing `ctl` session from this node's host to a remote home `device` over `raw` (`clientCtl`, satisfied by `*peer.Connection`):

- **Reader-first handshake and `devlog` order.** Starts the `Read` loop before sending `hello` (`v: 1`, `kind: "host"`, with `Name` and `Addrs`, under a 10 s timeout) because `*peer.Connection.Read` routes answers after earlier queued frames. Checks that the answer is not an `ErrorResponse`, then sends this node's owner `devlog` frames under `sendMu` before calling `Host.Attach(device, raw)` and closing `ready`. Commands sent from `Attached` hooks (such as `host.join`'s `member add_ai` and `Attached["runtime"]`'s step-18 `member status`) therefore reach the remote home after this node's device log.
- **Pushes and live `devlog` forwarding.** Incoming `devlog` frames apply to `Logs` at once (and forward to other connected sessions without echoing back to the sender); other frames wait on `ready` until `Host.Attach` has returned and then call `Host.Push(device, frame)` synchronously in read order, so every push sent before an answer is applied before `Request` returns.
- **`connectHome(dial)`.** Returns a non-blocking `Host.Connect` function that ignores this node's own device id (served by `AttachLocal`) and runs `dial` in the background, calling `serveClient` on each opened connection until `ctx` ends.

## Multi-node test helper (`nodetest`)

`nodetest.Start(t, cfg)` creates an owner key, loads a `device.key` per node, signs an owner device log enrolling the owner and every node, seeds `setting("owner")` and `device_log` in each node's SQLite store, starts `home.StartLoop` and `host.New`, and connects each node's own host and home with `AttachLocal`. Cross-node links between nodes in a cluster come with `N-2b` and `N-4`. When `Config.BinDir` is empty, `Start` builds `internal/host/fakecli/claude` and `internal/host/fakecli/codex` into `Root/bin`, prepends `BinDir` to `PATH`, and sets `SOLODAWN_FAKECLI_LOG`.

## Plugging into `Sessions`

Populate the maps on `*Sessions` before accepting connections:

| Map | Purpose |
| --- | --- |
| `Requests` | `ctl` requests (`sync`, `fetch` with `Async: true`; `host.info` with `Type: "host.info"`) |
| `Pushes` | `ctl` pushes after enrolment (`ack`) |
| `Commands` | node-level host commands (`pair.browser`, `settings.set`); other host commands (`to` set in payload) fall back to `host.Surface` |

`SessionConfig.OnAdopt` is invoked once with the adopted owner's user id when an unowned node persists its first valid adoption log (`dvlAdopt`), before forwarding `devlog` frames. `N-4` uses `OnAdopt` to restart `home.Loop` with the adopted `Owner` and start watching owner devices on the rendezvous.

## Concurrency and lock order

- `Sessions.cmdMu` guards `inflight` (keyed by signer and nonce) and receipt reads/writes in `Store.Write`; host command handlers run on their own goroutines outside `cmdMu`, and duplicate `(signer, nonce)` requests wait on the in-flight channel before reading the stored receipt.
- `Logs.writeMu` serializes device-log mutations and is acquired before `Store.Write` and before `Logs.mu`; `dvlNotify` runs after `writeMu` is released.
- `Logs.mu` (`sync.RWMutex`) guards `owner`, `log` and `onChange`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls, `OnAdopt` or network writes.
- `Sessions.mu` guards the active connection map (`conns`) and `sesConn.welcomed`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls or network writes.
- `sesConn.sendMu` serializes outbound frames on one `Ctl` connection; during `welcome` delivery (`Serve`) and initial client `devlog` delivery (`serveClient`) it holds `sendMu` across setting `welcomed = true` under `Sessions.mu`, reading `Logs` under `Logs.mu`, and sending the initial `devlog` frames so no concurrent `devlog` update overtakes the initial batch.

## Known limitations

- `sync`, `fetch` and `ack` are not registered in `Sessions` until `HM-2`, `HM-3`, `HM-7` and `N-4` wire them, and `host.info`, `pair.browser` and `settings.set` are registered by `N-3` ([issues](../../issues.md)).
- `home.Loop`'s `Owner` is fixed at `StartLoop`, so after an unowned node is adopted its running loop knows no owner until `SessionConfig.OnAdopt` (`N-4`) recreates the home loop with the adopted owner ([issues](../../issues.md)).
- `(*Sessions).connectHome` and `(*Sessions).serveClient` (`client.go`, `N-2b`) are not wired to `Host.Connect` and `peer.Peer.Connect` until `N-4` assembles `node.go` ([issues](../../issues.md)).
- In step 1 `Logs` stores only the node owner's device log; keeping logs of other group members belongs to step 2.
- `Foreground` calls a stub `startNode` until `N-4` assembles the full node runtime ([issues.md](../../issues.md)).
- `solodawn pair` does not yet start a stopped node ([issues.md](../../issues.md)).
- `Updater` leaves concurrency around `Busy`/`Status` and in-place `exec` restart to `N-4` ([issues.md](../../issues.md)).
- On macOS, Unix socket paths are limited to 103 bytes, so tests that bind `sock` under `HOME` use `/tmp` ([issues.md](../../issues.md)).
- `LocalLink` routes `cmd` requests and exposes a temporary `Watch` method for row subscriptions until `HM-2` merges; `sync`, `fetch` and `ack` on `LocalLink` wait for `HM-2`, `HM-3` and `HM-7` ([issues.md](../../issues.md)).
- Cross-node `ctl` links in `nodetest` and the running node wait for `N-4` to wire `connectHome`.

## Verification and sources

`go test ./internal/node/ ./internal/node/nodetest/` covers every statement of both packages, including local socket locking, opening lines, service definitions, update verification, `LocalLink` request/push framing, inbox delivery for groups created after `AttachLocal`, cancellation and closure, and `nodetest` startup and fake CLI replay. Acceptance scenarios live in `features/node.feature`, `features/node_service.feature`, `features/node_update.feature` and `features/node_local.feature`.
