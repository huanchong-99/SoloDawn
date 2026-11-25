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

A node asks for 47800 (TCP signalling and the local web client, UDP ICE) and
47801 (ICE-TCP), and takes a free port in place of one that is taken. 47802
(beacon) is not used: the beacon was cut on 2026-11-21. HTTPS and TURN over
TLS share port 443, chosen by the TLS server name `[verified, S-6]`, local CA
only (`risks.md` T-11).

**Local delivery.** On its own handler a node counts as connected under its
device id, without a WebSocket: a `to` for it reaches it as `from`, its
`presence` is always `on`, and it sends `to` and joins rooms there like a
client. A pairing node joins the room there and on each rendezvous it uses,
so `solodawn open` pairs on `localhost`. Nothing is forwarded to other nodes.

## 2. Signalling protocol

WebSocket, text frames, one JSON object per frame, at most 16 KiB per frame
(evidence 14). Every field shown for a frame is always present
(`contracts/conventions.md` section 2), except as 2.1 says. A client sends at
most 90 frames in any 11 seconds per connection, queueing the rest, and no `to`
over 16,382 bytes. The server closes a connection that sends a larger frame,
more than 120 frames in 10 seconds, a frame it cannot read, an unknown `t`, a
`k` in `to` or `watch` that is no device id (`contracts/envelope.md` section 2),
a `watch` of more than 64 keys, a `join` of a 17th room, or a `to` whose `from`
would exceed 16 KiB (the target gets none). A receiver queues at most 128
frames, 16 per sender and 32 from senders off its watch list; more are dropped
for it, closing nothing; the sender's offer timeout and back-off cover it.

### 2.1 Handshake

```
server -> {"t":"challenge","nonce":"q0J…"}
client -> {"t":"auth","k":"<device id>","s":"<signature>"}
server -> {"t":"ready","ice":[ … ],"ttl":3600}
```

- `s` = Ed25519 signature over `"solodawn/rv/1\n"` and the nonce's base64url
  text as in the `challenge` frame, held to check 1 of `contracts/envelope.md`
  section 3 with this prefix. A connection not authenticated within 10 seconds
  of its `challenge` is closed; a new connection for a device id replaces the
  previous one.
- `ice` is a list in the shape of `RTCIceServer`; a node's own handler answers
  `ready` with `"ice":[]`. `username` and `credential` are present on TURN
  entries (`turn:` and `turns:` URLs) and absent on STUN entries:

```json
[{"urls":["stun:rv.example:3478"]},
 {"urls":["turn:rv.example:3478?transport=udp",
          "turns:turn.rv.example:443?transport=tcp"],
  "username":"1763603600:<alias>","credential":"<mac>"}]
```

- Relay credentials expire after `ttl` seconds. `{"t":"ice"}` gets fresh ones:
  `{"t":"ice","ice":[…],"ttl":3600}`. A client asks before a connection attempt
  when its credentials are older than half their `ttl`.

### 2.2 Messages after `ready`

| Client sends | Server does |
| --- | --- |
| `{"t":"to","k":"<target>","env":{…}}` | if the target is connected, sends it `{"t":"from","k":"<sender>","env":{…}}`; otherwise answers `{"t":"offline","k":"<target>"}` |
| `{"t":"watch","k":["<id>",…]}` (at most 64) | replaces the watch list; sends `{"t":"presence","k":"<id>","on":true}` or `false` now and on every change |
| `{"t":"join","room":"<room id>"}` | puts the client in a pairing room of at most two; when the second arrives sends both `{"t":"joined","room":"<room id>"}`; a third gets `{"t":"full","room":"<room id>"}` and is not joined |
| `{"t":"room","room":"<room id>","data":{…}}` | forwards the same object to the other member of the room |
| `{"t":"quota"}` | answers `{"t":"quota","used":3250000,"limit":52428800}` for this device, today |

The server never reads `env` or `data`. A room id is the 32 lower-case hex
characters of `contracts/envelope.md` section 6. Rooms disappear when empty or
after 15 minutes; a connection is in at most 16. A device sends its `pair`
message only after `joined`, and again after each later `joined`.

### 2.3 Signalling payloads (inside `env`)

`env` is a signed envelope (`contracts/envelope.md`). Payload types:

| `t` | Extra fields | Meaning |
| --- | --- | --- |
| `offer` | `to`, `conn`, `sdp`, `probe` | start connection `conn`; `probe` true means "no relay candidates, this is an attempt to replace a relayed connection" |
| `answer` | `to`, `conn`, `sdp` | accept |
| `ice` | `to`, `conn`, `cand`, `mid` | one trickled candidate |

`conn` is a random id (`contracts/conventions.md` section 2). A receiver drops
a `from` whose `k` is not the `k` of its envelope. Rules:

- **Who offers.** The client side: a browser to a node (pairing included), a
  host to a home; never a home, so there is no glare. Two nodes that are each a
  client of the other each offer under their own `conn`; neither replaces the
  other (section 5.1, points 3 and 5).
- **Fan-out.** The offerer sends its offer and candidates on every signalling
  path it has for the target (section 4). The answerer answers each copy on the
  path that delivered it, and sends each candidate on every path it has for the
  offerer when gathered. Copies are told apart by sender and `conn`.
- **Trickle.** Descriptions go out as soon as set and candidates trickle as
  `ice` payloads; nobody waits for gathering (evidence 12), and no
  end-of-candidates is sent or awaited. A receiver holds candidates that come
  before their description, by sender and `conn`, at most 64 per attempt for at
  most 30 seconds, applies each distinct one once and drops retired ones.
- **Acceptance.** The receiver applies an offer only if the envelope passes
  `contracts/envelope.md` section 3; anything else gets no answer.
- **Once only.** A receiver remembers each processed (signer, `conn`) until its
  clock is strictly past the offer's `ts + 120000` ms (`contracts/envelope.md`
  section 3, check 5), across a close or a restart. A remembered offer starts
  and closes nothing: a copy for a live connection or pending probe gets the
  same answer on its own path once there is one; a retired one is ignored.
- **One inbound connection per device** (section 5.1, point 4). First applying
  a valid non-probe offer, the node closes that signer's previous inbound
  connection and pending inbound probe before the new checks, serialised per
  signer, but not the connection it offered to that device. A probe waits
  beside it until its first valid `hello` (section 5.1, point 10).
- The offer creates one data channel labelled `ctl`, ordered and reliable.

## 3. Relay

- Standard TURN with time-limited credentials: `username` = expiry (Unix
  seconds) + `:` + alias, the first 16 bytes of HMAC-SHA256(secret, device id)
  in base64url; `credential` = base64 HMAC-SHA1(secret, username). The secret
  is random per process start; the library's issuer and verifier take the alias
  `[verified, S-6]`. Expiry is checked when a credential opens an allocation;
  later requests on it pass while the HMAC matches.
- **Limits**: 32 allocations per device and 128 per IPv4 address or IPv6 /64,
  486 beyond; permissions for 64 peer IPs per client address, refreshes always
  allowed. Relay sockets take UDP ports 20000-29999 (`--relay-ports`), outside
  the ephemeral ports, which caps them all; the firewall must admit that range.
- **Quota**: bytes relayed for a device, both directions, all its allocations,
  per UTC day; default 50 MB, set by a flag. A packet counts its payload and 40
  bytes; once the quota is spent nothing passes, open allocations and empty
  packets included. Counters live in memory: a limit, not a budget. A node and
  `--no-relay` answer `{"t":"quota","used":0,"limit":0}`.
- Refused peers: non-global, private, carrier-grade NAT, 0.0.0.0/8,
  192.0.0.0/24, 198.18.0.0/15, 240.0.0.0/4, 64:ff9b::/96, 64:ff9b:1::/48,
  ::/96, ::ffff:0:0:0/96, 2002::/16, 2001::/32, fec0::/10, documentation
  ranges; loopback only for a loopback relay (tests). Operators' NAT64 prefixes
  (RFC 6052) cannot be listed: use the host firewall. Packets to the server's
  own addresses go only to its relay allocations. The relay sees DTLS
  ciphertext; clients leave it once a direct path works (`flows/connect.md`).

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
HTTPS may not open `ws://` to another machine, and whether it may open
`ws://localhost` is `[unverified]` (`risks.md` T-18). A browser with no
reachable rendezvous reaches only the node on its own machine, through that
node's handler: step 1 forwards no signalling to other nodes (`risks.md`
K-2). A self-hoster who puts a node's port behind a `wss://` address with a
trusted certificate has, by that, a rendezvous a browser can use.

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
- Host candidates on IPv4 and IPv6, server-reflexive by STUN, relay by TURN.
- The node requests mappings (UPnP, NAT-PMP) for the answering UDP socket, the
  TCP signalling listener and the passive ICE-TCP listener. It publishes the
  external TCP signalling port in the signalling URL and each mapped ICE
  endpoint only for the socket it serves (section 5.1, point 8). pion's
  address rewrite rule has no external-port field. S-8 verifies publication
  and reachability when a router grants a different port `[unverified]`.
- Queries the `.local` names that browsers give instead of their host
  addresses (`SetICEMulticastDNSMode`, query only). A name that does not
  resolve is no obstacle: in S-1 the connections used the browser's
  address as its checks showed it (peer-reflexive) `[verified, S-1]`.
- Both ends create a `probe` connection without TURN. The probe stays: not
  every browser leaves the relay by itself `[verified, S-1]` (evidence 13).

### 5.1 Ports: why a node offers from sockets of its own

1. **What was seen** `[verified, S-1]`. With both nodes on UDP 47800 for
   all connections, opening the reverse connection silenced the first. With
   the offerers on private UDP sockets both answered (evidence 1).
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
     inbound slot under this policy; one connects (`flows/connect.md` F).
5. **Not chosen: one connection per pair of nodes, used both ways.** Each
   direction has its own `conn` and one offering client that owns `hello`,
   commands, reconnect and probing; a node can answer as a home while it
   offers as a host. Simultaneous offers address different peer
   connections, so neither meets SDP glare. Sharing one `ctl` would need
   agreement on which connection survives and how both sessions resume.
6. **Not chosen: a second fixed port for offering.** One configured UDP
   port is kept. A second, shared offering port only separates the opposite
   directions, and can give an ordinary connection and its probe the same
   mux registration. A mapped port for ordinary offers, with private probe
   sockets, is reconsidered if S-8 shows a material gain in reachability.
7. **Hole punching** remains available; its success rate is not measured.
   ICE checks compatible host, reflexive and relay candidates. A live NAT
   mapping and a live local socket are two things, and either can outlive
   the other (points 4 and 10). Two peers behind one NAT can use reachable
   LAN host candidates; using each other's public mappings from inside it
   requires hairpinning, and if LAN routing or client isolation also blocks
   the host path, another working pair is needed. S-8 measures these, and
   hole punching across different NATs on ephemeral offer sockets (evidence 7).
8. **Port mapping.** A forward or mapping to 47800/UDP benefits answered
   connections (`risks.md` K-19). If the other direct UDP and IPv6 paths
   fail, a pion answerer can dial the passive TCP endpoint the offerer
   advertises: verified on Linux with only the offerer's listener reachable
   `[verified, A.1]` (evidence 8). S-8 verifies the router mapping,
   reachability from outside and publication of the external port the
   router actually grants. A mapped-address rule applies only to the
   candidates that mapping serves. TURN is used when no direct pair works.
9. **Remembered addresses** are unaffected: signalling URLs, a mapped one
   included. ICE candidates are gathered anew for each connection, and an
   ephemeral ICE endpoint is never a remembered signalling address.
10. **The relay-free probe** stays make-before-break. Both ends omit TURN
    on a probe, and a node keeps one pending probe beside the current
    inbound connection. A probe can fail where a direct path exists: failed
    direct checks of the old connection may still own an endpoint later
    assigned to the probe `[verified, A.1]` (evidence 10). So the old
    connection serves until the node has accepted `hello` on the probe
    (`flows/connect.md` B).

Browser: `RTCPeerConnection` with the `ice` list from `ready`; a `probe`
connection drops the TURN entry.

What ICE gives without more code: it checks compatible pairs as they become
available and nominates a working pair; a higher-priority path found later
may need a probe to take effect. Which direct paths step 1 tries together,
with the relay on port 443 last, is R-131. TCP simultaneous open is not
among them: pion does not perform it `[unverified]`, and spike S-9 measures
what it would gain (`risks.md` Q-1).

## 6. Self-hosting

```
solodawn rendezvous --domain rv.example --turn-domain turn.rv.example
```

- Certificates for both names come from Let's Encrypt, or from the ACME server
  of `--acme-directory <url>` (a private step-ca, say), whose HTTPS is checked
  against the roots of `--acme-ca <pem file>` in place of the system's and for
  nothing else; or `--cert` and `--key` give one certificate for both names.
- `--relay-quota-mb 50`; `--no-relay`: TLS signalling and STUN only.
  `--listen <address>`: where signalling and the static files are served;
  alone, over plain HTTP without STUN or TURN, with the HTTP limits below.
- Shared TLS port: at most 256 open connections per IPv4 address or IPv6 /64;
  the handshake within 10 s; ALPN `stun.turn` (RFC 7443) to TURN, anything else
  by server name; HTTPS headers within 10 s, a keep-alive connection closed
  after 120 idle seconds, never an upgraded WebSocket; a TURN stream closed
  after 10 silent minutes, a write to it after 30 s unread. An accept error
  pauses accepting, 5 ms doubling to 1 s.
- No database, configuration file or accounts. Nodes and web clients reach it
  by a rendezvous URL; the web client it serves defaults to its own origin.
