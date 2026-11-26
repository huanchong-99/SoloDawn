---
description: 'WebRTC connections between devices: signalling, candidates, the ctl channel, other data channels as streams, the signalling path to a rendezvous, reconnecting, and the selected path of a connection.'
kind: reference
---

# Peer

## Summary

`internal/peer` opens authenticated WebRTC data channels between devices. A node offers to the
devices it must reach and answers the offers it knows; the browser offers to the node. Offers,
answers and candidates travel as signed envelopes over one or more signalling routes: a WebSocket to
a rendezvous (`Path`) or a node-local delivery. The messages are in
[rendezvous](../../planning/contracts/rendezvous.md) sections 2 and 5, the channels in
[datachannel](../../planning/contracts/datachannel.md) section 1, and the sequence in
[connect](../../planning/flows/connect.md). The package holds no device list: `known`, `paths` and
the channel handlers are the caller's.

## Files

| File | Holds |
| --- | --- |
| `signal.go` | `Peer`, `New`, `Receive` (every transport ends here), the answering side, `Offer` |
| `registry.go` | which kinds of offer are answered, `Channels`, and `dispatch` of an opened data channel |
| `candidates.go` | the candidates of one connection: held until the description is set, each applied once |
| `ctl.go` | `Connection`: the `ctl` channel, `Read`, `Send` and `Request` with its response matching |
| `stream.go` | `Stream`: any other data channel as a stream of bytes |
| `path.go` | `Path`: the signalling WebSocket, `Dial`, `Serve`, frame pacing and renewal of the ICE servers |
| `sockets.go` | `Sockets`, `Listen`, the two WebRTC APIs, the TURN client's socket that takes its server's datagrams only, and `Connection.Path` with `PathChanged` |
| `accept.go` | `Steady`, a listener whose accept loop ends only when it is closed |
| `reconnect.go` | `Connect` and its back-off |
| `unanswered.go` | an offer's wait for an answer, ended by `Offline` |
| `diag.go` | the diagnostic events of a connection ([diagnostics](../../planning/contracts/diagnostics.md)) |

## Invariants

- **Every copy is checked.** `Receive` opens each envelope on its own (`identity.OpenSignal`); the
  outer sender must match the signer. An offer of a kind nobody registered is `bad_request`; only
  `ordinary` is registered, so a probe offer is refused.
- **One inbound connection per device.** The first application of a valid ordinary offer closes that
  signer's earlier inbound connection before the new checks start. A copy or a replay closes
  nothing.
- **Offers apply once.** `RememberOffer` records an offer across restarts and the claim lasts 120
  seconds after its timestamp. A repeat of a live offer returns the same answer; a repeat of a
  closed one returns nothing.
- **Candidates.** At most 64 per connection, held or applied; one that arrives before its
  description is held for 30 seconds and for at most 8 attempts of one signer; each distinct
  candidate is applied once. Candidates for a retired link are ignored.
- **ctl.** A frame is at most 60 KiB of UTF-8 and is a request, a push or a response of the
  contract. Any other frame is dropped, or answered `bad_request` when it carries a request id; the
  connection stays open.
- **Other channels.** A channel whose label no handler takes is closed. A stream's `Read` must be
  called until it fails (use `io.Copy(io.Discard, s)` when no input is needed): an unread message
  holds back every channel of the connection, `ctl` included. `Write` sends messages of at most
  16 KiB and waits while more than 1 MiB is queued.
- **Signalling frames.** At most 16 KiB. `Path` sends at most 90 frames in any 11 seconds, under
  the server's limit of 120 in 10, and a frame over the limit waits for its turn. `Dial` returns
  nothing before the server has authenticated this device. The ICE servers are asked for again at
  half their time to live.
- **Reconnecting.** `Connect` offers again after 1, 2, 4 ... 60 seconds, and starts from 1 second
  after a connection that stayed open for 60 seconds. A signal on `present` ends the wait at once.
  An offer is retired after 10 seconds without an answer, or at once when every path it was sent
  on reports the target offline.
- **Path.** `Connection.Path` is `relay` when either candidate of the selected pair is a relay
  candidate, `direct` for any other pair and `""` while none is selected. `PathChanged` signals
  that it may have changed.
- **Close.** `Close` waits, under the `Peer` lock, for pion to free the mux, so no callback that
  takes the `Peer` lock may run inside `Close`.
- **Locks.** The `Peer` lock covers its own decisions only. It is never held while `known`, `paths`,
  `inbound` or a channel handler runs, or while a frame is sent on a `Route`.

## Limitations

The relay-free probe offer, the LAN beacon, remembered addresses and port mapping are not built
([issues](../../issues.md)).
