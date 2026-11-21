---
description: 'The node assembly package: data directory, single-instance local socket and CLI subcommands, standalone rendezvous wrapper, verified self-updater, in-process local link between host and home, and the nodetest multi-node harness.'
kind: reference
---

# Node

## Summary

`internal/node` assembles the components of a SoloDawn node ([architecture](../../planning/architecture.md), sections 2.1 and 8). It owns the per-user data directory, the exclusive lock and owner-only local Unix socket through which CLI subcommands and MCP shims reach the running node, the standalone `rendezvous` entry that serves embedded web assets, the verified release self-updater, and the in-process `LocalLink` that connects a `*host.Host` and a `*home.Loop` on the same node without WebRTC. `internal/node/nodetest` starts multi-node in-process clusters with built fake Claude Code and Codex binaries for integration tests.

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
| start several in-process nodes with fake CLIs | `nodetest.Start(t, n)`, `nodetest.StartWith(t, cfg)`, `nodetest.Open(cfg)` |

## Local link

`AttachLocal(h, loop)` attaches `h.Key.ID()` to `loop` as a `home.Client`, registers the `*LocalLink` on `h` via `h.Attach`, marks the connection welcomed so `groups` pushes arrive, and calls `loop.Inbox` for the node's device.

- **Framing and size.** Every frame in either direction is encoded as JSON without HTML escaping and bounded by `ctlLimit` (60 KiB, [conventions.md](../../planning/contracts/conventions.md) section 4).
- **Requests (`Host` to `Home`).** `Request` assigns an increasing request `id`, sends the built frame, and waits on a per-request single-reader channel until the matching `re` arrives, `Done()` closes, or `ctx` is canceled. `cmd` requests dispatch to `loop.Cmd`; any other frame carrying an integer `id` is answered `bad_request` ([datachannel.md](../../planning/contracts/datachannel.md) section 2), and frames without an `id` are dropped.
- **Answers and pushes (`Home` to `Host`).** Frames carrying `re` wake the waiting `Request` call or are dropped if none waits; frames without `re` are delivered to `h.Push(device, frame)`.
- **Lock order and shutdown.** `LocalLink.mu` guards only the request counter and waiting map and is never held while calling into `host` or `home`. `Close()` calls `loop.Detach`, which closes `Done()` (`conn.Gone()`) and wakes any waiting `Request` with `io.EOF`.

## Multi-node test helper (`nodetest`)

`nodetest.Open` (and `Start` / `StartWith` for `testing.T`) creates an owner key, loads a `device.key` per node, signs an owner device log enrolling the owner and every node, seeds `setting("owner")` and `device_log` in each node's SQLite store, starts `home.StartLoop` and `host.New`, and connects them with `AttachLocal`. When `Config.BinDir` is empty, `Open` builds `internal/host/fakecli/claude` and `internal/host/fakecli/codex` into `Root/bin`; `Start` prepends `BinDir` to `PATH` and sets `SOLODAWN_FAKECLI_LOG`.

## Known limitations

- `Foreground` calls a stub `startNode` until `N-4` assembles the full node runtime ([issues.md](../../issues.md)).
- `solodawn pair` does not yet start a stopped node ([issues.md](../../issues.md)).
- `Updater` leaves concurrency around `Busy`/`Status` and in-place `exec` restart to `N-4` ([issues.md](../../issues.md)).
- On macOS, Unix socket paths are limited to 103 bytes, so tests that bind `sock` under `HOME` use `/tmp` ([issues.md](../../issues.md)).
- `LocalLink` routes `cmd` requests and exposes `Watch` for row subscriptions; `sync`, `fetch` and `ack` on `LocalLink` wait for `HM-2`, `HM-3` and `HM-7` ([issues.md](../../issues.md)).

## Verification and sources

`go test ./internal/node/ ./internal/node/nodetest/` covers every statement of both packages, including local socket locking, opening lines, service definitions, update verification, `LocalLink` request/push framing, limits and cancellation, and `nodetest` multi-node startup, fake CLI replay and failure cleanup. Acceptance scenarios live in `features/node.feature`, `features/node_service.feature`, `features/node_update.feature` and `features/node_local.feature`.
