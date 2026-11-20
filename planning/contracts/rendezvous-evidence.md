# Evidence for the ICE rules (spike S-1 and the A.1 design review)

The rules for ports, reconnecting, the probe and trickling are in
`contracts/rendezvous.md` sections 2.3, 5 and 5.1 and in
`flows/connect.md`. This file holds what was measured for them and where
the cause was read. A rule cites an entry here as "evidence n".

## 1. Sources

- **S-1**: spike S-1 (pull request 56): two pion nodes, and a pion node
  with Chromium, Firefox and WPE WebKit, on Linux.
- **A.1**: the design review of `contracts/rendezvous.md` section 5.1, by
  the reviewer (GPT-6.2), measured with the S-1 harness (pull request 56,
  head `1443c444b`) against this plan at `95a9e89`. Verdict: agree with
  changes. The coordinator accepted it in full (`risks.md` Q-26). Kept: the
  fixed answering port, sockets of its own for each offered connection,
  close-first for an ordinary reconnect, the relay-free probe, trickling.
- Code was read in pion/webrtc v4.2.20 and pion/ice v4.4.2.

## 2. Entries

Entries 1 to 10 belong to the points of section 5.1 with the same number.

1. **The first measurement holds.** S-1 opened the reverse connection
   after the first had echoed, and counted missed echoes over two bounded
   waits. With both nodes using UDP 47800 for all connections, A to B
   echoed before B to A opened, and both selected the pair 47800 and 47800.
   A to B then missed both five-second echoes, and B to A answered both.
   With only the offerers changed to private UDP sockets, all four
   alternating echoes matched.
2. **The last registration wins.** pion looks in the map of remote
   addresses first, for STUN too
   ([udp_mux.go, 566-598](https://github.com/pion/ice/blob/v4.4.2/udp_mux.go#L566-L598)).
   Ownership changes when a muxed connection writes to an endpoint
   ([udp_muxed_conn.go, 137-176](https://github.com/pion/ice/blob/v4.4.2/udp_muxed_conn.go#L137-L176),
   [udp_mux.go, 480-494](https://github.com/pion/ice/blob/v4.4.2/udp_mux.go#L480-L494)),
   which can happen during checks on a pair that never carries
   application data.
3. **Concurrent offers** (A.1). IPv4: 12 of 12 alternating echoes over
   three runs. IPv6: 8 of 8 over two. With two IPv4 interfaces per node,
   a mux per address and UDP blocked on the signalling interface, both
   directions selected the second interface and passed 8 of 8.
   `SetICEUDPMux` configures host gathering: the WebRTC stack passes
   `WithUDPMux`
   ([icegatherer.go, 250-259](https://github.com/pion/webrtc/blob/v4.2.20/icegatherer.go#L250-L259)),
   and ICE has a separate option for server-reflexive gathering
   ([gather.go, 292-308](https://github.com/pion/ice/blob/v4.4.2/gather.go#L292-L308);
   the mux per address is
   [udp_mux_multi.go, 87-151](https://github.com/pion/ice/blob/v4.4.2/udp_mux_multi.go#L87-L151)).
4. **An offerer restarted with the same device key** (A.1).
   - It reused its advertised port, and the answerer kept the old inbound
     connection: three new dials opened in 14, 20 and 25 ms and echoed.
   - Different local ports were translated to one endpoint that was not
     advertised, reachable only as peer-reflexive: the replacement missed
     a 45-second deadline for opening.
   - The old inbound connection was closed first, and reuse of the
     translation was verified: replacements opened in 18 and 31 ms and
     echoed.

   pion's default timers, 5 seconds to disconnected and a further 25 seconds
   to failed, describe the old agent; they do not schedule success for the new
   one
   ([agent_config.go, 23-31](https://github.com/pion/ice/blob/v4.4.2/agent_config.go#L23-L31),
   [agent.go, 777-787](https://github.com/pion/ice/blob/v4.4.2/agent.go#L777-L787)).
5. **Separate client connections.** Concurrent offers address separate peer
   connections. The simultaneous-offer measurements are in entry 3.
6. **A second fixed offering port.** An ordinary connection and its probe
   could still use the same pair of endpoints. This follows from entry 2. S-8
   evaluates the reachability alternative.
7. **Hole punching** between two nodes behind NATs was not measured. For
   port assignment and hairpinning see RFC 4787, sections 4.2.1 and 6.
8. **TCP from the answerer** (A.1). With TCP candidates only and incoming
   connections to the answerer's 47801 blocked, the answerer selected
   active TCP to the offerer's passive 47801 and echoed 3 of 3, opening
   in 1.011 to 1.013 seconds. pion creates an active candidate when it
   receives a compatible passive one, whatever its SDP role
   ([agent.go, 1322-1332](https://github.com/pion/ice/blob/v4.4.2/agent.go#L1322-L1332)),
   and its TCP mux reads the first STUN Binding message on each accepted
   stream
   ([tcp_mux.go, 245-268](https://github.com/pion/ice/blob/v4.4.2/tcp_mux.go#L245-L268)).
   `SetICEAddressRewriteRules` rewrites an IP and has no field for an
   external port
   ([icegatherer.go, 66-75](https://github.com/pion/webrtc/blob/v4.2.20/icegatherer.go#L66-L75)),
   which matters when two nodes ask one router for the same public port.
9. **Remembered addresses.** Section 4 of the rendezvous contract stores
   signalling URLs. ICE candidates are gathered again for each connection.
   This is a protocol rule.
10. **A probe beside a relayed connection.** S-1: the two were open
    together and echoing with each of the three browsers. A.1: an old
    connection kept echoing over TURN after its direct checks had failed.
    Its direct NAT binding expired, and its external port 40555 was
    assigned to a new probe. The local ports differed (45015 and 45072),
    yet the probe missed its deadline of 15.042 seconds while the old
    relay still echoed. A later probe, translated to 40556, opened in 13
    ms, and both connections echoed.
11. **Each side's own selected pair** (A.1). In both pion runs with the
    answerer's relay candidate delayed, and echoes already crossing TURN, the
    offerer saw host and peer-reflexive, and the answerer relay and host. When
    the candidate was delivered the offerer reported host and relay at the
    same address and port: a rule triggered only by an address change misses
    this
    ([agent.go, 1256-1299](https://github.com/pion/ice/blob/v4.4.2/agent.go#L1256-L1299)).
12. **Trickling.** S-1 measured the cost of waiting for complete
    descriptions: with a STUN server that did not answer, the first
    candidate appeared after 0 to 1 ms and gathering completed after
    11.125 to 39.885 seconds. Across signalling paths a candidate can
    arrive before its description; pion rejects `AddICECandidate` in that
    state
    ([peerconnection.go, 2102-2106](https://github.com/pion/webrtc/blob/v4.2.20/peerconnection.go#L2102-L2106))
    and nominates from the pairs that are valid at the time
    ([selection.go, 57-85](https://github.com/pion/ice/blob/v4.4.2/selection.go#L57-L85)).
13. **The probe stays.** S-1: when a direct path returned under a live
    relayed connection, Chromium and WPE WebKit moved to it by themselves
    within 5 seconds and Firefox was still relayed after 30, with pion's
    opt-in renomination on or off.

## 3. Not verified

- The NAT results of A.1 are controlled translations on Linux. Physical
  routers and hairpinning between real devices were not tried.
- Safari; WPE WebKit is not Safari. macOS and Windows: every run was on
  Linux.
- `hello`, sync and the replay of commands at application level: both
  S-1 and A.1 measured echoes on `ctl`.
- Router port mapping (spike S-8) and TCP simultaneous open (spike S-9).
