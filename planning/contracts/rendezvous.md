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
frames in 10 seconds. Field presence follows `contracts/conventions.md`
section 2: every field shown for a frame is always present.

### 2.1 Handshake

```
server -> {"t":"challenge","nonce":"q0J…"}
client -> {"t":"auth","k":"<device id>","s":"<signature>"}
server -> {"t":"ready","ice":[ … ],"ttl":3600}
```

- `s` = Ed25519 signature over `"solodawn/rv/1\n"` followed by the nonce
  as it stands in the `challenge` frame: the ASCII characters of that
  base64url text, not the bytes they encode. `k` and `s` are held to check
  1 of `contracts/envelope.md` section 3, with this prefix in place of the
  envelope's.
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
| `{"t":"join","room":"<room id>"}` | puts the client in a pairing room of at most two; when the second arrives sends both `{"t":"joined","room":"<room id>"}` |
| `{"t":"room","room":"<room id>","data":{…}}` | forwards the same object to the other member of the room |
| `{"t":"quota"}` | answers `{"t":"quota","used":3250000,"limit":52428800}` for this device, today |

The server never reads `env` or `data`. A room id is the 32 lower-case hex
characters of `contracts/envelope.md` section 6. Rooms disappear when empty
or after 15 minutes.

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
- **Fan-out.** The offering side sends its offer and its candidates on
  every signalling path it has for the target (section 4). The answering
  side sends its answer on every path on which a copy of the offer arrives,
  and its candidates on every path. Everything is told apart by sender and
  `conn`; a copy of something already handled is ignored. No path is ever
  "the" path, so nothing depends on which copy arrived first.
- **Two directions.** Two nodes may each be a client of the other (each
  hosts an AI in a group homed on the other). Each offers its own
  connection; the two do not replace each other.
- Spike S-1 also measures whether complete descriptions, gathered before
  sending, are fast and small enough (16 KiB per frame). If so the `ice`
  payload is removed.
- **Acceptance.** The receiver applies an offer only if the envelope passes
  the checks in `contracts/envelope.md` section 3. Anything else is dropped
  without an answer.
- The offer creates one data channel labelled `ctl`, ordered and reliable.

## 3. Relay

- Standard TURN with time-limited credentials in the usual form:
  `username` = expiry (Unix seconds) + `:` + device id; `credential` =
  base64 HMAC-SHA1(server secret, username). Issuer and verifier are the
  same pair of functions; the library's own are used if they accept the
  device id in the username `[unverified]`, spike S-6. The secret is random
  per process start. To the client the credential is an opaque password.
- **Quota**: bytes relayed for a device, both directions, summed over all
  its allocations, per UTC day. Default 50 MB, set by a flag. The count is
  taken where relayed data is read and written, so an allocation that is
  already open stops forwarding once the quota is used; refusing new
  allocations is not enough. Counters live in memory and start again when
  the service restarts: the quota limits a resource, it is not a budget.
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
which then signals on its behalf. A self-hoster who puts a node's port
behind a `wss://` address with a trusted certificate has, by that, a
rendezvous a browser can use: it is the same handler. See `risks.md` K-2.

## 5. ICE configuration

Node (pion; setting names `[documented, round 1]`, to be shown working in
spikes S-1 and S-8):

- UDP on one fixed port (default 47800) for all connections
  (`SetICEUDPMux`).
- ICE-TCP passive listener on 47801 (`SetICETCPMux`).
- IPv4 and IPv6 host candidates, server-reflexive through STUN, relay
  through TURN.
- Asks the router (UPnP, NAT-PMP) to map 47800/udp, 47800/tcp and 47801/tcp,
  and advertises mapped addresses as candidates
  (`SetICEAddressRewriteRules`) and as signalling addresses. Obtaining the
  mapping is the node's own work; pion only publishes it.
- Resolves `.local` candidates from browsers (`SetICEMulticastDNSMode`).
- A `probe` connection is created with STUN only, no TURN.
- Spike S-1 also tries pion's opt-in renomination: if a live relayed
  connection moves to a direct pair by itself with every supported browser,
  the probe connection is not needed.

Browser: `RTCPeerConnection` with the `ice` list from `ready`; a `probe`
connection drops the TURN entry.

What ICE gives without more code: all candidate pairs are checked in
parallel and the best working pair is used (R-131).

Step 1 offers these direct paths, all tried together: UDP hole punching,
IPv6, and TCP to a port that the other node has open, forwarded, or mapped
on its router. The relay on port 443 is the last resort (R-131).

Two nodes that are both behind NAT with UDP blocked, and no mapped or
forwarded port on either side, have no direct TCP path in step 1: that
would need TCP simultaneous open, which pion does not perform
`[unverified]`. Spike S-9 measures what it would gain; it is built in step
3 only if the gain is clear (`risks.md` Q-1).

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
