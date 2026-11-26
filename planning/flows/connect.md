# Flow: connecting and relay fallback

Contracts: `contracts/rendezvous.md` (signalling, ICE settings),
`contracts/datachannel.md` (`hello`, `welcome`). Rules: R-131 to R-133.

Actors: C, the offering side (a browser, or a host-role node); N, the
answering node; RV, any signalling path.

## A. Opening a connection

1. C looks up the signalling paths it has for N: the rendezvous URLs and,
   on the same machine, the node's own handler. A browser has the rendezvous
   URL of its page, the `addrs` of each node's `welcome` that it stored, and,
   when served from a node, that node; from an HTTPS page a `ws://` address
   is blocked (`risks.md` T-18). A node dials no remembered address of its own
   (PR-2 cancelled).
2. C creates a peer connection with a `ctl` data channel, picks a new `conn`
   id, and sends a signed `offer` on **every** path at once, without
   waiting for its candidates (`contracts/rendezvous.md` 2.3, "Trickle").
   A node that offers gathers its UDP candidates on sockets of its own
   (section 5.1, point 3, there).
3. N receives the first copy and checks the envelope
   (`contracts/envelope.md` section 3). On first application of a valid
   ordinary offer, N closes that signer's previous inbound connection
   before it starts the new checks; a probe offer is refused
   (`contracts/rendezvous.md` 2.3). A copy, or a replayed old
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
   higher-priority path found later takes effect only if ICE moves to it
   itself or at the next reconnect (section B). TCP simultaneous open
   between two nodes behind NAT is not part of step 1 (R-131, spike S-9).
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
   path and its round-trip time are shown in the connection indicator, and
   the page `Nodes` shows how long a relayed path has lasted; a host
   reports its path to the home in member status.

   A browser has no event for every such change. It reads its statistics
   when `ctl` opens and every second after: the selected pair is the one
   its transport statistics name, or, where the statistics have no
   transport, the pair they mark as selected `[unverified]` (unit I-8
   runs the three browsers).

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
10. to 14. Removed by the decision of 2026-11-21: no relay-free probe and
    move (`internal/peer/README.md`, "Limitations"). A relayed connection
    stays relayed unless ICE finds a direct pair by itself; some browsers do
    and one does not (`contracts/rendezvous.md` section 5): step 8 then
    classifies again. A reconnect (section C) tries every path again and
    stays relayed if direct is still blocked. A connection that is already direct
    is not moved to a "more direct" path.

## C. Losing and regaining a connection

15. When ICE reports the connection failed or closed, or its `ctl` channel
    closes, C starts again from step 1. Retries back off: 1, 2, 4 … 60
    seconds, and start again from 1 second after a connection that stayed
    open for 60 seconds.
    If no answer arrives within 10 seconds of sending an offer (the same
    timeout as `hello`), or every path used for that offer reports the
    target offline before an answer arrives, C retires that `conn` at
    once and ignores its late answers. After the same back-off it sends
    a new offer with a new `conn`. Once answered, negotiation is left to ICE.
    - An offer goes out on the signalling paths C is signed in on at that
      moment. With none, the attempt has failed, and the back-off goes on.
    - For a browser, a signalling path that closes counts, for the offers
      still unanswered, as a path that reported the target offline: an
      answer comes back on the path that delivered the offer.
    - A browser ends a connection whose `hello` the node refuses or does
      not answer within 10 seconds; it is then lost like any other. But a
      `hello` refused with `version` or `forbidden` ends the attempts: the
      browser offers that node nothing more, and tells the page the code.
      Offering again cannot change either answer; the person has to update
      or pair again first.
    - A browser opens a signalling path that closed again by the same
      rule: 1, 2, 4 … 60 seconds, from 1 second again after a path that
      stayed signed in for 60 seconds, and at once when its own network
      changes (step 16). It sends its `watch` list on the new path.
16. Back-off is skipped, and C retries at once, when a signalling path
    reports the target present (`presence` from rendezvous) or when C's own
    network changes. For a browser, its network has changed when its window
    gets the `online` event.
17. After `welcome`, C resumes: `sync` for each group with the last sequence
    number it **applied** (`flows/sync.md` A); rows that were on their way
    on the old connection are simply read again. A host that restarted has
    nothing cached and starts from zero.
18. C sends again, under new `id`s, every request that has had no response:
    each `cmd`, `fetch` and `host.info`, after the `sync` requests of step
    17, in the order in which they were first made. The receipt on the other
    side makes a repeated command harmless (`contracts/commands.md`); the
    other requests only read. A `hello` is not among them: the new
    connection has had its own. A host also sends the current `member
    status` of each of its AIs, so that the home shows what is true now.

Two nodes may each host an AI in a group homed on the other. Each then opens
its own connection as a client of the other, and the two connections live
side by side; step 3 concerns only inbound connections from one signer,
and leaves alone the connection N itself offered
(`contracts/rendezvous.md` section 5.1, points 3 and 5).

## D. With no rendezvous service reachable

- Direct connections that are open stay open; nothing they do involves the
  service. A relayed connection ends with the relay.
- Node to node: a new connection needs a reachable rendezvous. Local-network
  discovery and a node's remembered addresses are not built (decisions of
  2026-11-21 and 2026-11-22; `internal/peer/README.md`, "Limitations").
  A browser's stored `addrs` (A.1) are tried as well.
- Browser: can connect only to the node on its own machine, from the page
  that node serves (`http://localhost:47800/`), through that node's own
  handler (`contracts/rendezvous.md` section 1, "Local delivery"). Step 1
  forwards no signalling to other nodes.
- Two devices on different networks cannot find each other until some
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

## F. One active tab per browser

The tabs of one browser share its device key, and a node keeps one
inbound connection per device (`contracts/rendezvous.md` 2.3): two
connected tabs would close each other's connection. So one tab connects.
Nothing changes on the wire, and the node keeps close-first.

Before opening signalling or peer connections, a tab requests the exclusive
Web Lock `solodawn-device-<device id>` with `ifAvailable: true`. It holds the
lock for its active session. If the callback receives `null`, the tab stays
disconnected and shows "SoloDawn is open in another tab" with a "Use here"
button. "Use here" requests the same lock with `steal: true`. When the former
holder's `request()` promise rejects with `AbortError`, it stops reconnects,
closes its signalling and peer connections, and shows the notice.
It requests the lock again only on "Use here".

Web Locks is within the browser baseline (`risks.md` Q-3): it is in Chrome
from 69, Firefox from 96 and Safari from 15.4. Frontend task F-03
implements this rule (`steps/step-1-frontend.md`).
