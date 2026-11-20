# Flow: connecting, relay fallback, moving back to direct

Contracts: `contracts/rendezvous.md` (signalling, ICE settings),
`contracts/datachannel.md` (`hello`, `welcome`). Rules: R-131 to R-133.

Actors: C, the offering side (a browser, or a host-role node); N, the
answering node; RV, any signalling path.

## A. Opening a connection

1. C looks up the signalling paths it has for N: local-network address,
   remembered addresses, rendezvous URLs. A browser has only rendezvous URLs
   and, when served from a node, that node.
2. C creates a peer connection with a `ctl` data channel, picks a new `conn`
   id, and sends a signed `offer` on **every** path at once, without
   waiting for its candidates (`contracts/rendezvous.md` 2.3, "Trickle").
   A node that offers gathers its UDP candidates on sockets of its own
   (section 5.1, point 3, there).
3. N receives the first copy and checks the envelope
   (`contracts/envelope.md` section 3). On first application of a valid
   ordinary offer, N closes that signer's previous inbound connection and
   any pending inbound probe before it starts the new checks; a probe
   offer leaves the inbound connection alone. A copy, or a replayed old
   offer, closes nothing (`contracts/rendezvous.md` 2.3, "Once only" and
   "One inbound connection per device"). N creates its peer connection
   and sends its signed `answer` on every path on which a copy of that
   offer arrives. C uses the first copy of the answer.
4. Both sides send candidates as signed `ice` payloads, each on every path.
   Everything is told apart by authenticated sender and `conn`. A
   candidate that arrives before its description is held ("Trickle").
5. ICE checks compatible candidate pairs as they become available: IPv6
   and IPv4 host addresses, server-reflexive (UDP hole punching), ICE-TCP
   to a port the node has open, forwarded or mapped on its router, and
   relay (UDP, and TLS on port 443). It nominates a working pair from
   those valid at the time; relay has the lowest priority. A
   higher-priority path found later may need a probe to take effect
   (section B). TCP simultaneous open between two nodes behind NAT is not
   part of step 1 (R-131, spike S-9).
6. DTLS completes. Each side compares the remote certificate fingerprint
   with the one in the signed description it accepted; the WebRTC stack does
   this. `ctl` opens.
7. C sends `hello`; N answers `welcome`. The connection is in use.
8. Each side reads its own selected candidate pair and classifies the
   **path**:

| Path | Selected pair |
| --- | --- |
| `relay` | either reported candidate has type `relay` |
| `direct` | any other pair; `udp` or `tcp`, `v4` or `v6` |

   Every stack reports the candidate types of the pair it selected
   `[verified, S-1]`. A side classifies again whenever its selected pair
   or the type of one of that pair's candidates changes, also after
   trickled candidates arrive: the type can change at an unchanged address
   and port (`contracts/rendezvous-evidence.md`, entry 11). Until a pair
   can be read there is no path, and the terminal stays unavailable. The
   path, its round-trip time and the time it was established are shown in
   the connection indicator; a host reports its path to the home in
   member status.

   There is no separate path for a local network. Chromium and Firefox
   give a `.local` name instead of their host address, so the node sees
   them as peer-reflexive; Chromium hides its own address in its
   statistics, and WPE WebKit hides both `[verified, S-1]`. "Both ends are
   private host addresses" cannot be told on both sides. Nothing depends
   on it either: a local network is a direct path with a short round-trip
   time.

   A relayed path always has a relay candidate as the *local* candidate of
   at least one side, and a side always knows the type of its own
   candidates. So the two sides never both take a relayed path for a
   direct one, and a terminal needs both: the browser offers it and the
   node accepts it. The two observations are independent and may differ:
   a peer-reflexive remote candidate can temporarily conceal the other
   end's use of TURN.

## B. While relayed

9. Both ends enforce the terminal restriction locally, each from its own
   observation: a `pty` channel is opened only on a `direct` path, and an
   end whose path becomes `relay` closes a `pty` channel that is already
   open, which is hand-back (`flows/takeover.md` D). While its path is
   `relay` the web client shows "relayed, limited".
10. C starts or stops its probe schedule from its own updated observation
    (step 8): it probes while its path is `relay`. After 30 seconds, then
    with the interval doubling up to 5 minutes, C sends a signed `offer`
    with `probe: true` and a new `conn`. Both ends omit TURN on a probe.
    N keeps one pending probe beside the current inbound connection; a
    later probe offer supersedes a pending one.
11. If the probe's `ctl` opens, C moves. A move is a reconnect in which
    the old connection serves until N has accepted the new one:
    1. C sends `hello` on the probe.
    2. N promotes the probe on its first valid `hello`: the probe becomes
       the device's inbound connection, and N closes its predecessor.
       Nothing is carried over from it. A superseded probe or a repeated
       `hello` cannot retire a newer connection.
    3. C retires the old connection on `welcome`: it stops applying
       anything from it and closes it, and the relay allocation ends with
       it. Then C sends `sync` for each group with the last sequence
       number it **applied** (`flows/sync.md` A). Rows that were on their
       way on the old connection are simply read again.
12. C sends again every command that has had no response. The receipt on
    the other side makes a repeat harmless (`contracts/commands.md`).
13. Both sides classify the new path. The indicator changes within one
    second and the relay notice disappears.
14. If the probe fails, C closes it, N keeps serving the old connection,
    and C retries on schedule. That covers a probe whose `ctl` opened and
    which was lost before N accepted `hello`: the working relay was not
    closed. A probe can fail where a direct path exists
    (`contracts/rendezvous.md` section 5.1, point 10). Loss after N has
    promoted the probe is an ordinary reconnect (section C).

A relayed connection may also find a direct pair by itself; some browsers
do and one does not (`contracts/rendezvous.md` section 5). Step 8 then
classifies again, and step 10 stops the probes.

Probing happens only while relayed. A connection that is already direct is
not probed for a "more direct" path; ICE nominated a working pair when it
opened, and it will again on the next reconnect.

## C. Losing and regaining a connection

15. When ICE reports the connection failed or closed, C starts again from
    step 1. Retries back off: 1, 2, 4 … 60 seconds.
16. Back-off is skipped, and C retries at once, when a signalling path
    reports the target present (`presence` from rendezvous, a beacon on the
    local network) or when C's own network changes.
17. After `welcome`, C resumes exactly as in step 11.3: `sync` with the last
    sequence number it applied for each group. A host that restarted has
    nothing cached and starts from zero.
18. Commands in flight when the connection died are sent again after
    `welcome`, as in step 12. A host also sends the current `member status`
    of each of its AIs, so that the home shows what is true now.

Two nodes may each host an AI in a group homed on the other. Each then opens
its own connection as a client of the other, and the two connections live
side by side; steps 3 and 11.2 concern only inbound connections from one
signer, and leave alone the connection N itself offered
(`contracts/rendezvous.md` section 5.1, points 3 and 5).

## D. With no rendezvous service reachable

- Direct connections that are open stay open; nothing they do involves the
  service. A relayed connection ends with the relay.
- Node to node: step 1 still yields local-network and remembered addresses.
  If one of them is reachable the connection opens exactly as above.
- Browser: can connect only through a node on the same machine
  (`http://localhost:47800/`). That node is its signalling path to itself
  and, by forwarding `to`/`from`, to nodes it can reach.
- Two devices that are both behind NAT, on different networks, with no
  remembered address that still works, cannot find each other until some
  rendezvous is reachable. This is the stated limit of R-133; see
  `risks.md` K-2.

## E. What the user sees

| State | Indicator | Extra |
| --- | --- | --- |
| path `direct` | green, latency | — |
| path `relay` | warm colour, "relayed, limited" | terminal button disabled with the reason; "ask the AI to diagnose the network" |
| reconnecting | blue, breathing | — |
| home unreachable | red hollow ring | the group is read-only until the home returns |

"Ask the AI to diagnose" posts an ordinary message that mentions an AI on
that computer; the AI uses the `net_status` tool (`contracts/mcp-tools.md`)
and guides the owner (R-132, and the rule that the user's own AI fixes
network configuration).
