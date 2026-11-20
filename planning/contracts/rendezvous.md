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

WebSocket, text frames, one JSON object per frame, at most 16 KiB per frame
(the largest signed offer or answer in spike S-1 made a frame of 2.5 KB).
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
  (pairing included), a host to a home. A home never offers. Every
  connection has one offerer, so there is no glare to handle (section
  5.1, point 5).
- **Fan-out.** The offering side sends its offer and its candidates on
  every signalling path it has for the target (section 4). The answering
  side sends its answer on every path on which a copy of the offer arrives,
  and its candidates on every path. Everything is told apart by
  authenticated sender and `conn`. No path is ever "the" path, so nothing
  depends on which copy arrived first.
- **Two directions.** Two nodes may each be a client of the other (each
  hosts an AI in a group homed on the other). Each offers its own
  connection under its own `conn`, and neither replaces the other
  (section 5.1, points 3 and 5).
- **Trickle.** Each side sends its local description as soon as it is set
  and trickles its candidates as `ice` payloads on every path. Nobody
  waits for gathering to end (evidence 12 says what waiting cost). A
  candidate can arrive before its description: the receiver holds early
  candidates by authenticated sender and `conn` until the corresponding
  remote description is set. It deduplicates individual candidates and
  discards those for retired connections.
- **Acceptance.** The receiver applies an offer only if the envelope passes
  the checks in `contracts/envelope.md` section 3. Anything else is dropped
  without an answer.
- **Once only.** A node remembers every (signer, `conn`) it has processed,
  also after that connection has closed, for at least the freshness
  window: two minutes plus the allowed clock skew, so until two minutes
  after the offer's `ts` (`contracts/envelope.md` section 3, check 5). A
  remembered offer is never processed again and never closes anything, so
  a replayed old offer cannot close a live connection. A copy for the
  current `conn` only repeats the answer, on the path the copy came by.
  Copies for a superseded connection are ignored while their envelopes
  can still pass that check.
- **One inbound connection per device.** A restart or an expired NAT
  mapping can make a new offerer appear at an endpoint still registered to
  an old connection (section 5.1, point 4). On first application of a
  valid ordinary offer, one that is not a probe, the node closes that
  signer's previous inbound connection and any pending inbound probe
  before it starts the new checks. It serialises this decision per signer
  and leaves alone the connection it offered to that device itself. A
  probe stays pending beside the current inbound connection until its
  first valid `hello` promotes it (section 5.1, point 10).
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
2. **Remembered**: every signalling URL at which that device was last
   reached, and the URLs the device itself reported in its `hello`
   (`contracts/datachannel.md`), a mapped one included. ICE endpoints are
   not remembered (section 5.1, point 9).
3. **Rendezvous**: the configured rendezvous URLs. The official one is the
   default; a self-hoster replaces or adds to the list.

Browsers cannot use 1, and can use 2 only for `localhost`: a page served over
HTTPS may not open `ws://` to another machine. A browser with no reachable
rendezvous can therefore start only through a node on the same machine,
which then signals on its behalf. A self-hoster who puts a node's port
behind a `wss://` address with a trusted certificate has, by that, a
rendezvous a browser can use: it is the same handler. See `risks.md` K-2.

## 5. ICE configuration

Node (pion). `[verified, S-1]` marks what spike S-1 showed with pion 4.2
and with Chromium, Firefox and WPE WebKit on Linux, `[verified, A.1]` what
the review of section 5.1 measured. "Evidence n" is entry n of
`contracts/rendezvous-evidence.md`, which also lists what is unverified.

- **UDP.** As section 5.1, point 3, says. The configured port (default
  47800) is given to pion with `SetICEUDPMux`, one mux for each local
  address `[verified, S-1 and A.1]`.
- **TCP.** An ICE-TCP passive listener on 47801 (`SetICETCPMux`), for the
  connections a node answers and for those it offers `[verified, S-1]`.
- IPv4 and IPv6 host candidates, server-reflexive through STUN, relay
  through TURN.
- Asks the router (UPnP, NAT-PMP) to map 47800/udp, 47800/tcp and
  47801/tcp, and publishes the address and port the router granted as
  candidates and as signalling addresses (section 5.1, point 8). That is
  the node's own work: `SetICEAddressRewriteRules` rewrites an address
  and has no field for an external port `[unverified]`, spike S-8.
- Queries the `.local` names that browsers give instead of their host
  addresses (`SetICEMulticastDNSMode`, query only). A name that does not
  resolve is no obstacle: in S-1 the connections used the browser's
  address as its checks showed it (peer-reflexive) `[verified, S-1]`.
- Both ends create a `probe` connection without TURN. The probe stays:
  when a direct path returned under a live relayed connection, Chromium
  and WPE WebKit moved to it by themselves within 5 seconds and Firefox
  was still relayed after 30, with pion's opt-in renomination on or off
  `[verified, S-1]`.

### 5.1 Ports: why a node offers from sockets of its own

1. **What was seen** `[verified, S-1]`. With both nodes using UDP 47800
   for all connections, A to B echoed before B to A opened. Both selected
   the pair 47800 and 47800. A to B then missed both five-second echoes,
   and B to A answered both. With only the offerers changed to private
   UDP sockets, all four alternating echoes matched.
2. **Why: the last registration wins.** Each underlying pion UDP mux maps
   an observed remote IP and port to one ICE connection. For an unknown
   endpoint, the USERNAME of a STUN request can identify the local ICE
   username fragment. For a known endpoint the existing map wins, also for
   incoming STUN. A write to that endpoint can transfer the registration.
   DTLS and SCTP traffic then follows the registered owner (evidence 2).
3. **Rule.** On each eligible local IPv4 or IPv6 address, a node reserves
   its configured UDP port for the host candidates of the connections it
   answers. Each connection it offers gathers its UDP candidates on
   sockets of its own, with ports the system assigns. Both roles may use
   the passive TCP listener. STUN and TURN candidates describe the sockets
   or allocations that produced them. A browser controls its own socket
   allocation. Concurrent offers: `[verified, A.1]` (evidence 3).
4. **Reconnecting: close first.** A restart or an expired NAT mapping can
   make a new offerer appear at an endpoint still registered to an old
   connection. How long recovery takes depends on which candidates can be
   checked; no figure is promised (evidence 4). Hence "One inbound
   connection per device" in section 2.3. Three cases lie outside it:
   - An answering node that restarts clears its own mux.
   - A NAT can recycle an endpoint to a different device. Same-signer
     teardown cannot clear that old registration, and closing connections
     by public IP would disconnect unrelated peers.
   - Two browser tabs that share one device key share the ordinary
     inbound slot under this policy (`risks.md` K-23).
5. **Not chosen: one connection per pair of nodes, used both ways.** Each
   direction has its own `conn` and one offering client that owns `hello`,
   commands, reconnect and probing. A node can answer as a home while it
   offers as a host. Offers made at the same moment address different
   peer connections, so neither meets SDP glare. Sharing one `ctl` would
   require agreement on which connection survives simultaneous offers and
   on how both client sessions resume on it.
6. **Not chosen: a second fixed port for offering.** One configured UDP
   port is kept. A second, shared offering port separates the opposite
   directions and nothing more: it can give an ordinary connection and
   its probe the same mux registration. A mapped port for ordinary
   offers, with private probe sockets, is reconsidered if S-8 shows a
   material gain in reachability.
7. **Hole punching** remains available; its success rate is not measured.
   ICE checks compatible host, reflexive and relay candidates. A live NAT
   mapping and a live local socket are two things, and either can outlive
   the other (points 4 and 10). Two peers behind the same NAT can use
   reachable LAN host candidates; using each other's public mappings from
   inside that NAT requires hairpinning. If LAN routing or client
   isolation prevents the host path and hairpinning fails, another
   working pair is required. S-8 measures these cases, and hole punching
   across different NATs with ephemeral offering sockets (evidence 7).
8. **Port mapping.** A forward or mapping to 47800/UDP benefits answered
   connections. If the other direct UDP and IPv6 paths fail, a pion
   answerer can dial the passive TCP endpoint the offerer advertises:
   verified on Linux with only the offerer's listener reachable
   `[verified, A.1]` (evidence 8). S-8 verifies the router mapping,
   reachability from outside and publication of the actual external port,
   also when a router grants another port than the one asked for. A
   mapped-address rule applies only to the candidates that mapping
   serves. TURN is used when no direct pair works. TCP simultaneous open
   remains S-9 (`risks.md` K-19).
9. **Remembered addresses** are unaffected. A node remembers reachable
   signalling URLs, a mapped signalling URL included when there is one.
   ICE candidates are gathered anew for each connection, and an ephemeral
   ICE endpoint is never a remembered signalling address.
10. **The relay-free probe** stays make-before-break. Both ends omit TURN
    on a probe, and a node keeps one pending probe beside the current
    inbound connection. A probe can still fail where a direct path
    exists: failed direct checks of the old connection may still own an
    endpoint that is later assigned to the probe `[verified, A.1]`
    (evidence 10). The old connection therefore keeps serving until the
    node has accepted `hello` on the probe; the order is in
    `flows/connect.md` B.

Browser: `RTCPeerConnection` with the `ice` list from `ready`; a `probe`
connection drops the TURN entry.

What ICE gives without more code: it checks compatible pairs as they
become available and nominates a working pair. A higher-priority path
discovered later may require a probe to take effect (R-131).

Which direct paths step 1 tries together, with the relay on port 443 as
the last resort, is R-131. TCP simultaneous open is not among them: pion
does not perform it `[unverified]`, and spike S-9 measures what it would
gain (`risks.md` Q-1).

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
