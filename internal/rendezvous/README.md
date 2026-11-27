---
description: 'The rendezvous server: signalling over WebSocket, pairing and hosted rooms, the STUN and TURN relay with its quota and limits, the standalone service with its certificates, and the handler that nodes serve for the local web client.'
kind: reference
---

# Rendezvous

## Summary

`internal/rendezvous` routes signalling between devices that cannot yet reach each other, and
serves the web client. It never reads a payload it forwards. Two things use it: a node serves
`Handler` on its own port (`/rv` for signalling, the embedded web client for the rest, and an empty
ICE list), and `solodawn rendezvous` runs `Options.Serve`, which adds STUN, TURN over UDP and TLS,
and certificates. The protocol is in [rendezvous](../../planning/contracts/rendezvous.md) sections
1 to 3 and 6.

## Files

| File | Holds |
| --- | --- |
| `server.go` | `Handler`, the `/rv` WebSocket: handshake, routing of `to`, `watch`, `ice` and `quota` frames, per-receiver queues |
| `room.go` | pairing and hosted rooms: `join`, `leave` and `room` frames |
| `relay.go` | the TURN server, credentials, `ready` and `ice` answers (with `hosts`), the relay's own addresses and relay port holding |
| `limits.go` | allocations per device and per source, and permissions per client |
| `quota.go` | the daily relay ledger of each device |
| `standalone.go` | `Options`, `PortRange`, `Serve` |
| `tls.go` | certificates (supplied or ACME), the shared port that hands a TLS connection to HTTPS or TURN, idle stream limits |
| `truncated*.go` | reading one datagram where a long one is cut or fails (on Windows, `truncated.go` skips the failed reads) |

## Invariants

- **Handshake.** A device is known by its key: the server sends a challenge, the client answers with
  its signature, and a connection that does not within the wait is closed. A second connection of a
  device replaces the first.
- **Frames.** At most 16 KiB each, and at most 120 in 10 seconds per connection; a connection over
  the limit or sending an unknown or malformed frame is closed.
- **Routing.** A frame for a device that is not connected is answered `offline`. A receiver's queue
  holds at most 16 frames of one sender and 32 from senders off its watch list; a frame over that is
  dropped and both connections stay open.
- **Rooms.** A room id is 32 hex digits, a room holds two devices and lives 15 minutes, and one
  connection is in at most 16 rooms. A third device is told the room is full.
- **Hosted rooms.** A `join` with `data` makes the connection the room's host and `data` its card;
  every server of this version says `hosts` in `ready` and in each `ice` answer. Each device that
  waited in the room, and each that joins later, gets `joined` and the card and is not kept; any
  device's `room` frames for that room go to the host. A hosted room has no 15 minutes: it ends at the host's `leave` or
  with its connection. A connection hosts at most 16 beside the 16 it is in; a room that has a host,
  and a 17th hosting, are answered `full` and the connection stays. The 16-room check of a `join`
  comes first, so a device already in 16 rooms that joins a hosted room is disconnected.
- **Relay.** A TURN username carries an alias of the device (an HMAC under the process's secret), not
  its id. A credential is checked when an allocation opens; the daily quota bounds what a relayed
  connection moves. Limits: 32 allocations per device, 128 per source (an IPv4 address or an IPv6
  /64), 64 peer permissions per client. Refused allocations are counted and logged at most once a
  minute.
- **Shared port.** On the signalling port a TLS connection goes to TURN when its ALPN is `stun.turn`
  and otherwise to HTTPS by its server name. A source has at most 256 connections open on it.
- **Without a domain** the service is plain HTTP for signalling only, with no STUN or relay.
