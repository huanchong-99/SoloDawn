---
description: 'Assembly of the SoloDawn node: data directory, single-instance local socket, CLI subcommand dispatch, start-at-login services, verified self-update, verified device logs, the ctl session hub, room pairing and pair.browser, the node-level host surface, the in-process local link between host and home, and the nodetest multi-node harness.'
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
- `devlog.go`: `Logs`, loading and persisting `setting` (`key = 'owner'`, JSON-encoded string) and `device_log` for the node's owner, unowned-node adoption across split frames, node-signed device additions during browser pairing, forwarding accepted entries in `devlog` frames up to 61,440 bytes, and delivering accepted `device` rows to `home.Loop.Event("device", rows)` ([envelope](../../planning/contracts/envelope.md), sections 4 and 6; [datachannel](../../planning/contracts/datachannel.md), section 3.1).
- `session.go`: `Sessions`, serving inbound `ctl` connections (`*peer.Connection` via `Ctl`), `hello` and `welcome`, the bootstrap rule before enrolment, request, push, command and replay tables, owner push fan-out (`PushOwner`), delegation to `home.Loop` and `host.Surface`, and asynchronous execution with envelope checks, in-flight nonce coordination and `receipt` persistence for host commands ([datachannel](../../planning/contracts/datachannel.md), sections 2, 3, 5, 7 and 8; [commands](../../planning/contracts/commands.md), "Once only" and section 2).
- `pair.go`: `Pairer`, running 10-minute single-use pairing rooms in role `b` (`Pair`, onboarding and adding another computer) and role `a` (`PairBrowser` and `pair.browser`, adding another browser with `#pair=<secret>` links and once-only nonce replay; [pairing](../../planning/flows/pairing.md), sections A to C; [envelope](../../planning/contracts/envelope.md), section 6).
- `surface.go`: `Surface`, serving `host.info` (`HostState`, `PushHostInfo`), `ReadName` and `ReadRendezvous`, and `settings.set` with SQLite persistence, `setting` audit rows, `approvers` propagation via `member set` and live `host.info` pushes ([datachannel](../../planning/contracts/datachannel.md), sections 3 and 7; [commands](../../planning/contracts/commands.md), section 2).
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
| pair a node over a rendezvous room or issue a browser pairing link | `NewPairer(cfg)`, `(*Pairer).Pair(ctx, req)`, `(*Pairer).PairBrowser(ctx, webURL)`, `(*Pairer).HandleEvent(ctx, path, kind, data)`, `(*Pairer).Serve(ctx, device, raw)` ([pairing.md](../../planning/flows/pairing.md)) |
| serve `host.info` and `settings.set` and push `host.info` updates | `NewSurface(cfg)`, `(*Surface).HostState(ctx)`, `(*Surface).PushHostInfo(ctx)`, `ReadName(db)`, `ReadRendezvous(db)` ([datachannel.md](../../planning/contracts/datachannel.md) section 7) |
| wire a host to the home on the same node | `AttachLocal(h, loop)` returning `*LocalLink` (`host.Home` with `Path() == "local"`) |
| start in-process nodes with fake CLIs | `nodetest.Start(t, cfg)` |

## Local link

`AttachLocal(h, loop)` attaches `h.Key.ID()` to `loop` as a `home.Client`, registers the `*LocalLink` on `h` via `h.Attach`, and calls `loop.Inbox` for the node's device.

- **Framing.** Every frame in either direction is encoded as JSON without HTML escaping. `host.Command` checks the `ctl` message limit before sending, and the home loop splits `rows` pushes within the limit ([conventions.md](../../planning/contracts/conventions.md) section 4).
- **Requests (`Host` to `Home`).** `Request` assigns an increasing request `id`, sends the built frame, and waits on a per-request single-reader channel until the matching `re` arrives, `Done()` closes, or `ctx` is canceled. `cmd` requests dispatch to `loop.Cmd`; any other frame carrying an integer `id` is answered `bad_request` ([datachannel.md](../../planning/contracts/datachannel.md) section 2), and frames without an `id` are dropped.
- **Answers and pushes (`Home` to `Host`).** Frames carrying `re` wake the waiting `Request` call or are dropped if none waits; frames without `re` are delivered to `h.Push(device, frame)`. `h.Push` runs on the loop's send goroutine for this connection, so a `Pushes` handler must not wait for an answer from the same home, because that answer is sent by the goroutine it blocks.
- **Lock order and shutdown.** `LocalLink.mu` guards only the request counter and waiting map and is never held while calling into `host` or `home`. The only way to lose the local link is `Close()`, which calls `loop.Detach`, closes `Done()` (`conn.Gone()`), and wakes any waiting `Request` with `io.EOF`.

## Multi-node test helper (`nodetest`)

`nodetest.Start(t, cfg)` creates an owner key, loads a `device.key` per node, signs an owner device log enrolling the owner and every node, seeds `setting("owner")` and `device_log` in each node's SQLite store, starts `home.StartLoop` and `host.New`, and connects each node's own host and home with `AttachLocal`. Cross-node links between nodes in a cluster come with `N-2b` and `N-4`. When `Config.BinDir` is empty, `Start` builds `internal/host/fakecli/claude` and `internal/host/fakecli/codex` into `Root/bin`, prepends `BinDir` to `PATH`, and sets `SOLODAWN_FAKECLI_LOG`.

## Plugging into `Sessions`

Populate the maps on `*Sessions` (or attach `NewPairer` and `NewSurface`) before accepting connections:

| Map | Purpose |
| --- | --- |
| `Requests` | `ctl` requests (`sync`, `fetch` with `Async: true`; `host.info` registered by `NewSurface` with `Type: "host.info"`) |
| `Pushes` | `ctl` pushes after enrolment (`ack`) |
| `Commands` | node-level host commands (`pair.browser` registered by `NewPairer`; `settings.set` registered by `NewSurface`); other host commands (`to` set in payload) fall back to `host.Surface` |
| `Replays` | memory-backed replay answers for succeeded host commands (`pair.browser` registered by `NewPairer`, returning the active link while valid and `conflict` once used, expired or lost across restart; `takeover.start` later) |

`SessionConfig.OnAdopt` is invoked once with the adopted owner's user id when an unowned node persists its first valid adoption log (`dvlAdopt`), before forwarding `devlog` frames. `N-4` uses `OnAdopt` to restart `home.Loop` with the adopted `Owner` and start watching owner devices on the rendezvous. `home.Loop.Welcome` and `home.Loop` group announcements deliver `welcome.groups` and `groups` pushes over `Sessions`.

## Concurrency and lock order

- `Sessions.cmdMu` guards `inflight` (keyed by signer and nonce) and receipt reads/writes in `Store.Write`; host command handlers and `Replays` handlers run outside `cmdMu`, and duplicate `(signer, nonce)` requests wait on the in-flight channel before reading the stored receipt.
- `Pairer.mu` guards active `attempts` and nonce-keyed `links`; it is a leaf lock never held across `Store.Write`, `Logs`, `Sessions`, `Peer` or WebSocket operations. Each `pairAttempt.outcome` channel has a single reader (`waitPair` for role `b`, `watchBrowser` for role `a`).
- `Logs.writeMu` serializes device-log mutations and is acquired before `Store.Write` and before `Logs.mu`; `dvlNotify` runs after `writeMu` is released.
- `Logs.mu` (`sync.RWMutex`) guards `owner`, `log` and `onChange`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls, `OnAdopt` or network writes.
- `Sessions.mu` guards the active connection map (`conns`) and `sesConn.welcomed`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls or network writes.
- `sesConn.sendMu` serializes outbound frames on one `Ctl` connection; during `welcome` delivery it holds `sendMu` across `raw.Send(welcome)`, setting `welcomed = true` under `Sessions.mu`, reading `Logs` under `Logs.mu`, and sending the initial `devlog` frames so no `devlog` frame is forwarded ahead of `welcome`.

## Known limitations

- `sync`, `fetch` and `ack` are not registered in `Sessions` until `HM-2`, `HM-3`, `HM-7` and `N-4` wire them ([issues](../../issues.md)).
- `SurfaceConfig.Agents` and `SurfaceConfig.Queued` are optional hooks until `N-4` wires `HS-3` (`(*Host).Agents`) and `HS-13` (held-item counts); without `Agents`, `Surface` reads found CLIs from the `agent` table ([issues](../../issues.md)).
- `home.Loop`'s `Owner` is fixed at `StartLoop`, so after an unowned node is adopted its running loop knows no owner until `SessionConfig.OnAdopt` (`N-4`) recreates the home loop with the adopted owner ([issues](../../issues.md)).
- `Sessions` implements only the server side of an inbound `ctl` connection; a host's outgoing `ctl` session to a home on another node (`hello` with `kind: "host"`, `welcome`, `devlog` both ways, and pushes to `Host.Push`) is split into unit `N-2b` (`internal/node/client.go`, wired by `N-4`, needed by `I-6` / `A3.3`; [issues](../../issues.md)).
- In step 1 `Logs` stores only the node owner's device log; keeping logs of other group members belongs to step 2.
- `Foreground` calls a stub `startNode` until `N-4` assembles the full node runtime ([issues.md](../../issues.md)).
- `solodawn pair` does not yet start a stopped node ([issues.md](../../issues.md)).
- `Updater` leaves concurrency around `Busy`/`Status` and in-place `exec` restart to `N-4` ([issues.md](../../issues.md)).
- On macOS, Unix socket paths are limited to 103 bytes, so tests that bind `sock` under `HOME` use `/tmp` ([issues.md](../../issues.md)).
- `LocalLink` routes `cmd` requests and exposes a temporary `Watch` method for row subscriptions until `HM-2` merges; `sync`, `fetch` and `ack` on `LocalLink` wait for `HM-2`, `HM-3` and `HM-7` ([issues.md](../../issues.md)).
- Cross-node `ctl` links between hosts and remote homes wait for `N-2b` and `N-4`.

## Verification and sources

`go test ./internal/node/ ./internal/node/nodetest/` covers every statement of both packages, including local socket locking, opening lines, service definitions, update verification, room pairing in both roles, `pair.browser` single-use expiry and nonce replay, `host.info` state assembly, `settings.set` validation, audit rows and `approvers` propagation, `LocalLink` request/push framing, inbox delivery for groups created after `AttachLocal`, cancellation and closure, and `nodetest` startup and fake CLI replay. Acceptance scenarios live in `features/node.feature`, `features/node_service.feature`, `features/node_update.feature`, `features/node_local.feature`, `features/node_session.feature` and `features/node_pair.feature`.
