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
   id, and sends a signed `offer` on **every** path at once.
3. N receives the first copy, checks the envelope
   (`contracts/envelope.md` section 3), creates its peer connection, and
   sends a signed `answer` on the path the offer came from. Later copies of
   the same `conn` are ignored.
4. Both sides trickle candidates as signed `ice` payloads on that path.
5. ICE checks every candidate pair in parallel: IPv6 and IPv4 host
   addresses, server-reflexive (UDP hole punching), ICE-TCP to a port the
   node has open, forwarded or mapped on its router, and relay (UDP, and TLS
   on port 443). The highest-priority pair that works is selected; relay has
   the lowest priority. TCP simultaneous open between two nodes behind NAT is
   not part of step 1 (R-131, spike S-9).
6. DTLS completes. Each side compares the remote certificate fingerprint
   with the one in the signed description it accepted; the WebRTC stack does
   this. `ctl` opens.
7. C sends `hello`; N answers `welcome`. The connection is in use.
8. Each side reads the selected candidate pair and classifies the **path**:

| Path | Selected pair |
| --- | --- |
| `lan` | both candidates are host candidates in private or link-local ranges |
| `direct` | any other pair without a relay candidate; `udp` or `tcp`, `v4` or `v6` |
| `relay` | either candidate is a relay candidate |

   The path, its round-trip time and the time it was established are shown
   in the connection indicator and reported in `hello`/`welcome` and in
   member status.

## B. While relayed

9. If the path is `relay`, both sides apply the relay restrictions: no `pty`
   channel may be opened (terminal is direct only), and the web client shows
   "relayed, limited".
10. C starts probing. After 30 seconds, then with the interval doubling up
    to 5 minutes, C sends a signed `offer` with `probe: true` and a new
    `conn`, from a peer connection that has no TURN server configured.
11. If the probe's `ctl` opens, C sends `hello` on it. N treats a `hello`
    from a device that already has a connection as a replacement: it moves
    that device's subscriptions to the new connection.
12. C re-sends every command that has had no response (commands are
    idempotent by nonce), then closes the old, relayed connection. The relay
    allocation ends with it.
13. Both sides reclassify the path. The indicator changes within one second
    and the relay notice disappears.
14. If the probe fails, nothing changes; the relayed connection was never
    touched. C waits for the next interval.

Probing happens only while relayed. A connection that is already direct is
not probed for a "more direct" path; ICE chose the best pair when it was
opened, and it will choose again on the next reconnect.

## C. Losing and regaining a connection

15. When ICE reports the connection failed or closed, C starts again from
    step 1. Retries back off: 1, 2, 4 … 60 seconds.
16. Back-off is skipped, and C retries at once, when a signalling path
    reports the target present (`presence` from rendezvous, a beacon on the
    local network) or when C's own network changes.
17. After `welcome`, C resumes: a browser or host sends `sync` with the last
    sequence number it saw for each group (`flows/sync.md`).
18. Requests in flight when the connection died are re-sent after `welcome`.

## D. With no rendezvous service reachable

- Connections that are open stay open; nothing they do involves the service.
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
| path `lan` or `direct` | green, latency | — |
| path `relay` | warm colour, "relayed, limited" | terminal button disabled with the reason; "ask the AI to diagnose the network" |
| reconnecting | blue, breathing | — |
| home unreachable | red hollow ring | the group is read-only until the home returns |

"Ask the AI to diagnose" posts an ordinary message that mentions an AI on
that computer; the AI uses the `net_status` tool (`contracts/mcp-tools.md`)
and guides the owner (R-132, and the rule that the user's own AI fixes
network configuration).
