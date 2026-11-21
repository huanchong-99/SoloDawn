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
- `devlog.go`: `Logs`, loading and persisting `setting` (`key = 'owner'`, JSON-encoded string) and `device_log`, unowned-node adoption across split frames, forwarding accepted entries in `devlog` frames up to 61,440 bytes, closing connections of revoked devices, and emitting `device` rows to homed groups via `home.Events["device"]` ([envelope](../../planning/contracts/envelope.md), sections 4 and 6; [datachannel](../../planning/contracts/datachannel.md), section 3.1).
- `session.go`: `Sessions`, serving inbound `ctl` connections (`*peer.Connection` via `Ctl`), `hello` and `welcome`, the bootstrap and principal rules, request and push tables, delegation to `home.Loop` and `host.Surface`, and envelope checks plus `receipt` persistence for host commands ([datachannel](../../planning/contracts/datachannel.md), sections 2, 3, 5, 7 and 8; [commands](../../planning/contracts/commands.md), "Once only" and section 2).

## Plugging into `Sessions`

Populate the maps on `*Sessions` before accepting connections:

| Map | Purpose |
| --- | --- |
| `Requests` | `ctl` requests (`sync`, `fetch` with `Owner: false, Async: true`; `host.info` with `Owner: true, Type: "host.info"`; `audit` falls back to `host.Surface["audit"]` when `Host` is set) |
| `Pushes` | `ctl` pushes after enrolment (`ack` with `Owner: false`; `pty.resize` falls back to `host.Surface["pty.resize"]` with `Owner: true`) |
| `Commands` | node-level host commands (`pair.browser`, `settings.set`); other host commands fall back to `host.Surface` |
| `Replays` | repeat handlers for once-in-memory host commands (`pair.browser`, `takeover.start`) whose SQLite receipt stores only `{"ok":true}` |

## Concurrency and lock order

- `Sessions.cmdMu` serializes host command execution and receipt reads/writes; it may call `Store.Write`, command handlers and `Sessions.mu`.
- `Logs.writeMu` serializes device-log mutations and is acquired before `Store.Write` and before `Logs.mu`.
- `Logs.mu` (`sync.RWMutex`) guards `owner`, `logs`, `kept` and `onChange`; it is a leaf lock never held across `Store.Write`, `home.Loop` calls, `OnAdopt` or network writes.
- `Sessions.mu` guards the active connection map (`conns`) and connection welcome flags; it is a leaf lock never held across `Store.Write`, `home.Loop` calls or network writes.
- `sesConn.sendMu` is a leaf lock serializing outbound frames on one `Ctl` connection so `welcome` and its initial `devlog` frames are written atomically in order.
