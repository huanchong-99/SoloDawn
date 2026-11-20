---
description: 'Authenticated WebRTC peer connections, rendezvous signalling paths, ctl framing, data-channel streams, and relay-free probes and moves.'
kind: reference
---

# Peer connections

## Summary

`internal/peer` opens authenticated WebRTC data channels between devices
([rendezvous.md](../../planning/contracts/rendezvous.md),
[datachannel.md](../../planning/contracts/datachannel.md) and
[connect.md](../../planning/flows/connect.md)). A `Path` signs into a
rendezvous WebSocket, paces outgoing frames, refreshes ICE credentials at half
their `ttl`, and feeds received envelopes to a `Peer`. A `Peer` verifies
signalling envelopes, enforces once-only offer application and one inbound
connection per device, frames `ctl` requests, responses and pushes on
`Connection`, hands registered non-`ctl` channels (`pty:`, later `watch:`) to
their handlers as `Stream` values, and maintains persistent outbound
connections with relay-free probes and moves through `Client` and `Session`.

## Client and Session API

| Entry | Purpose |
| --- | --- |
| `Listen(udpPort, tcpPort)` | Binds the shared UDP mux for answered host candidates and the passive ICE-TCP listener for both roles |
| `Dial(ctx, url, key)` / `Path.Serve` / `Path.Route` | Authenticates on a rendezvous WebSocket, refreshes ICE servers, and exposes a `Route` |
| `New(key, sockets, db, known, paths, inbound, channels)` | Creates the `Peer` of one device |
| `Peer.Receive(ctx, path, frame)` | Checks and applies a received `offer`, `answer` or `ice` envelope |
| `Peer.Offer(ctx, device)` | Starts one outbound ordinary connection attempt and returns its `*Connection` |
| `Peer.Client(ctx, device, present, hello, onWelcome)` | Keeps one offered direction open across reconnects and relay-free moves until `ctx` ends or `Close` is called |

`Client` and `Session` both satisfy the five-method `Home` interface of
`internal/host` (`Send`, `Path`, `PathChanged`, `Done`, `Request`), so `node`
hands each welcomed `*Session` to `host.Attach` and writes no client reconnect
or probe loop of its own:

- **`onWelcome(s, welcome)`:** called for each welcomed connection, both after
  an ordinary reconnect and after a probe move, before unanswered non-`sync`
  requests are replayed. On a move, the previous `Session` is already retired
  (`Read` and `Send` return `io.EOF` at once, even if pushes were buffered) and
  its underlying `Connection` is closed before `onWelcome` runs; the previous
  `Session.Done()` closes right after `onWelcome` returns.
- **`Request(ctx, build)`:** `build` must be pure because type and size checks
  call it with synthetic `id` values before each wire send. A `sync` request
  belongs to the `Session` open now and ends with `io.EOF` if that `Session`
  ends (whether called on `Session` or on `Client`); every other request (`cmd`,
  `fetch`, `host.info`) is sent again in FIFO order under a fresh `id` after the
  next `welcome` if the connection moves or reconnects.
- **One reader per `PathChanged()`:** `Connection.PathChanged()`,
  `Session.PathChanged()`, and `Client.PathChanged()` are distinct 1-buffered
  channels. When a `Connection` belongs to a `Client`, the `Client` serve loop
  is the sole reader of `Connection.PathChanged()` and fans each signal out to
  `Session.PathChanged()` (for the attached session reader) and
  `Client.PathChanged()` (for a client-level observer).
- **Inbound probes:** a `probe: true` offer omits TURN on both ends and sits in
  `Peer.probes[device]` outside `Peer.slot[device]` until its first valid
  `hello` (`v == 1`, `kind` `"browser"` or `"host"`). Non-`ctl` channels opened
  before promotion are closed; `hello` with another `v` is answered `version`
  without taking the slot or closing the probe. On the first valid `hello`, the
  probe takes `Peer.slot[device]`, closes any predecessor, queues `hello`, and
  starts `inbound(c)` on a new goroutine.

## Lock order

- `Peer.mu` guards `live`, `slot`, `probes`, `early` and `retired`, plus
  applying descriptions and candidates. It is never held while calling `known`,
  `paths`, `inbound`, a `Channels` handler, or `Route.Send`.
- `known` and `paths` run on the caller's goroutine during `Receive` and
  `Offer`, and `known`, `paths` and `Route.Send` also run on pion's goroutines.
  `inbound` runs on pion's goroutine when `ctl` opens on an ordinary connection
  and on a new goroutine after the first valid `hello` of a promoted probe;
  channel handlers run on goroutines of their own. A callback may call back
  into `Peer`, so callers of `Receive`, `Offer` or `Client` must not hold a lock
  that `known` or `paths` acquires.
- Inside `Client`, `sendMu` serialises wire flushes and is acquired before
  `Client.mu`; `Client.mu` is never held across `Connection.Send`, `onWelcome`,
  or `Connection.Close`. `Connection.mu` guards `dc`, `next` and `waiting` on a
  single connection. On `Path`, `pace.mu` serialises paced WebSocket writes and
  `Path.mu` guards `config` and `renew`.

## Limits

| Quantity | Value | Source |
| --- | --- | --- |
| Signalling WebSocket frame / `to` frame | 16,384 B / 16,382 B | [rendezvous.md](../../planning/contracts/rendezvous.md) §2 |
| Outgoing signalling pacer | 90 frames per 11 s per path | [rendezvous.md](../../planning/contracts/rendezvous.md) §2 |
| ICE credential refresh | at `ttl / 2`, retry every 10 s | [rendezvous.md](../../planning/contracts/rendezvous.md) §2.1 |
| Early candidates | 64 per `conn`, 30 s hold, 8 attempts per signer | [rendezvous.md](../../planning/contracts/rendezvous.md) §2.3 |
| Retired `conn` memory | 2 min in memory (`seen_offer` in SQLite for replay window) | [rendezvous.md](../../planning/contracts/rendezvous.md) §2.3 |
| Unanswered offer deadline | 10 s from first sent copy (or all paths `offline`) | [connect.md](../../planning/flows/connect.md) §C.15 |
| Reconnect back-off | 1 s doubling to 60 s; resets after 60 s open; woken by `present` | [connect.md](../../planning/flows/connect.md) §C.15–16 |
| Probe `ctl` open deadline | 15 s from sent probe offer | [connect.md](../../planning/flows/connect.md) §B.11 |
| `hello` / `welcome` deadline | 10 s | [connect.md](../../planning/flows/connect.md) §B.11.1, §C.15 |
| Probe schedule while `relay` | 30 s doubling to 5 m (`30 s, 1 m, 2 m, 4 m, 5 m`); reset on `direct` | [connect.md](../../planning/flows/connect.md) §B.10 |
| Pending inbound probes | 1 per authenticated signer (`Peer.probes[device]`) | [connect.md](../../planning/flows/connect.md) §B.10, [issues.md](../../issues.md) |
| `ctl` frame / incoming queue | 61,440 B (`60 KiB`) / 16 frames | [datachannel.md](../../planning/contracts/datachannel.md) §2 |
| `Stream` chunk / send buffer threshold | 16,384 B (`16 KiB`) / 1 MiB | [datachannel.md](../../planning/contracts/datachannel.md) §7 |
