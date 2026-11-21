---
description: 'Assembly of the SoloDawn node: data directory, single-instance local socket, CLI subcommand dispatch, start-at-login services, verified self-update, verified device logs and the ctl session hub.'
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
- `sesConn.sendMu` serializes outbound frames on one `Ctl` connection; during `welcome` delivery it holds `sendMu` across `raw.Send(welcome)`, setting `welcomed = true` under `Sessions.mu`, reading `Logs` under `Logs.mu`, and sending the initial `devlog` frames so no `devlog` frame is forwarded ahead of `welcome`.

## Known limitations

- `sync`, `fetch` and `ack` are not registered in `Sessions` until `HM-2`, `HM-3`, `HM-7` and `N-4` wire them, and `host.info`, `pair.browser` and `settings.set` are registered by `N-3` ([issues](../../issues.md)).
- `home.Loop`'s `Owner` is fixed at `StartLoop`, so after an unowned node is adopted its running loop knows no owner until `SessionConfig.OnAdopt` (`N-4`) recreates the home loop with the adopted owner ([issues](../../issues.md)).
- `Sessions` implements only the server side of an inbound `ctl` connection; a host's outgoing `ctl` session to a home on another node (`hello` with `kind: "host"`, `welcome`, `devlog` both ways, and pushes to `Host.Push`) is split into unit `N-2b` (`internal/node/client.go`, wired by `N-4`, needed by `I-6` / `A3.3`; [issues](../../issues.md)).
- Remote hosts calling `hello` with `kind: "host"` invoke `home.Loop.Inbox`, which marks the connection as a taker only in groups that exist at the time of `hello` until `#209` (`N-1`) lands the `Conn.taker` fix in `internal/home` ([issues](../../issues.md)).
- In step 1 `Logs` stores only the node owner's device log; keeping logs of other group members belongs to step 2.
