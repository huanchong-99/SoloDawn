# Contract: rendezvous (signalling, STUN, relay)

The rendezvous service lets two devices exchange the few messages needed to
open a WebRTC connection, tells them their public address, and relays
ciphertext when no direct path exists. It stores nothing.

The same signalling handler runs in every node, so a peer that can reach a
node's address needs no server at all.

## 1. Endpoints

| Endpoint | Where | Purpose |
| --- | --- | --- |
| `wss://<host>/rv` | rendezvous | signalling |
| `ws://<node address>:47800/rv` | every node | signalling, same protocol, no ICE servers offered |
| `stun:<host>:3478` (UDP) | rendezvous | public address discovery |
| `turn:<host>:3478` (UDP), `turns:<turn host>:443` (TLS) | rendezvous | relay |
| `https://<host>/` | rendezvous | static web client and `install.md` |
| `http://localhost:47800/` | every node | static web client for the local browser |

Port numbers are defaults and can be changed by flags.

`[unverified]` Serving HTTPS and TURN-over-TLS on the same port 443 by
choosing on the TLS server name. Spike S-6 in `steps/step-1.md`.

## 2. Signalling protocol

WebSocket, text frames, one JSON object per frame, at most 16 KiB per frame.
The server closes a connection that sends a larger frame or more than 120
frames in 10 seconds.

### 2.1 Handshake

```
server -> {"t":"challenge","nonce":"q0J…"}
client -> {"t":"auth","k":"<device id>","s":"<signature>"}
server -> {"t":"ready","ice":[ … ],"ttl":3600}
```

- `s` = Ed25519 signature over `"solodawn/rv/1\n"` + nonce.
- A new connection for a device id replaces the previous one.
- `ice` is a list in the shape of `RTCIceServer`:

```json
[{"urls":["stun:rv.example:3478"]},
 {"urls":["turn:rv.example:3478?transport=udp",
          "turns:turn.rv.example:443?transport=tcp"],
  "username":"1763603600:<device id>","credential":"<mac>"}]
```

- Relay credentials expire after `ttl` seconds. The client asks for fresh
  ones with `{"t":"ice"}` and receives `{"t":"ice","ice":[…],"ttl":3600}`.
- A node's own handler answers `ready` with `"ice":[]`.

### 2.2 Messages after `ready`

| Client sends | Server does |
| --- | --- |
| `{"t":"to","k":"<target>","env":{…}}` | if the target is connected, sends it `{"t":"from","k":"<sender>","env":{…}}`; otherwise answers `{"t":"offline","k":"<target>"}` |
| `{"t":"watch","k":["<id>",…]}` (at most 64) | replaces the watch list; sends `{"t":"presence","k":"<id>","on":true}` or `false` now and on every change |
| `{"t":"join","room":"<hex>"}` | puts the client in a pairing room of at most two; when the second arrives sends both `{"t":"joined","room":"<hex>"}` |
| `{"t":"room","room":"<hex>","data":{…}}` | forwards the same object to the other member of the room |
| `{"t":"quota"}` | answers `{"t":"quota","used":3250000,"limit":52428800}` for this device, today |

The server never reads `env` or `data`. Rooms disappear when empty or after
15 minutes.

### 2.3 Signalling payloads (inside `env`)

`env` is a signed envelope (`contracts/envelope.md`). Payload types:

| `t` | Extra fields | Meaning |
| --- | --- | --- |
| `offer` | `to`, `conn`, `sdp`, `probe` | start connection `conn`; `probe` true means "no relay candidates, this is an attempt to replace a relayed connection" |
| `answer` | `to`, `conn`, `sdp` | accept |
| `ice` | `to`, `conn`, `cand`, `mid` | one trickled candidate |

Rules:

- **Who offers.** The client side always offers: a browser to a node
  (pairing included), a host to a home. Homes never offer. There is no glare
  handling because there is no glare.
- **Fan-out.** The offering side sends the same offer over every signalling
  path it has for the target (section 4) at once. The receiver handles the
  first copy of a given `conn` and ignores the rest; it answers on the path
  the first copy arrived on.
- **Acceptance.** The receiver applies an offer only if the envelope passes
  the checks in `contracts/envelope.md` section 3. Anything else is dropped
  without an answer.
- The offer creates one data channel labelled `ctl`, ordered and reliable.

## 3. Relay

- Standard TURN. Credentials: `username` = expiry (Unix seconds) + `:` +
  device id; `credential` = base64 HMAC-SHA256(server secret, username). The
  secret is random per process start.
- **Quota**: bytes relayed for allocations of a device, both directions, per
  UTC day. Default 50 MB, set by a flag. Over quota the allocation stops
  forwarding until the next day. Counters live in memory.
- The relay sees DTLS ciphertext only.
- Clients treat the relay as the lowest-priority path and leave it as soon as
  a direct path works (`flows/connect.md`).

## 4. Finding a peer without the server

A node keeps, per known device, a short list of signalling addresses:

1. **Local network**: every node sends a UDP multicast beacon every 30
   seconds and when its network changes. Group `239.255.77.68`, port 47802.
   The datagram is a signed envelope with payload
   `{"t":"beacon","n":"…","ts":…,"port":47800}`. A node that recognises the
   signer records `ws://<source address>:<port>/rv` for that device.
2. **Remembered**: every address at which signalling to that device last
   succeeded, and the addresses the device itself reported in its `hello`
   (`contracts/datachannel.md`), including port mappings it obtained from
   the router.
3. **Rendezvous**: the configured rendezvous URLs. The official one is the
   default; a self-hoster replaces or adds to the list.

Browsers cannot use 1, and can use 2 only for `localhost`: a page served over
HTTPS may not open `ws://` to another machine. A browser with no reachable
rendezvous can therefore start only through a node on the same machine,
which then signals on its behalf. See `risks.md` K-2.

## 5. ICE configuration

Node (pion):

- UDP on one fixed port (default 47800) for all connections.
- ICE-TCP passive listener on 47801.
- IPv4 and IPv6 host candidates, server-reflexive through STUN, relay
  through TURN.
- Asks the router (UPnP, NAT-PMP) to map 47800/udp, 47800/tcp and 47801/tcp,
  and advertises mapped addresses as candidates and as signalling addresses.
- Resolves `.local` candidates from browsers.
- A `probe` connection is created with STUN only, no TURN.

Browser: `RTCPeerConnection` with the `ice` list from `ready`; a `probe`
connection drops the TURN entry.

What ICE gives without more code: all candidate pairs are checked in
parallel and the best working pair is used (R-131).

`[unverified]` pion does not perform TCP simultaneous-open, so two nodes that
are both behind NAT with UDP blocked have no direct TCP path unless one of
them has a mapped or forwarded port. See `risks.md` Q-1.

## 6. Self-hosting

```
solodawn rendezvous --domain rv.example --turn-domain turn.rv.example
```

- Obtains certificates automatically for both names, or takes
  `--cert` and `--key`.
- `--relay-quota-mb 50`, `--no-relay` to run signalling and STUN only.
- No database, no configuration file, no accounts.
- Nodes and web clients are pointed at it with a rendezvous URL; the web
  client served by that host uses its own origin by default.
