# Spike S-8: port mapping and hole punching in the network simulation

Spike for PR-3 (`planning/steps/parallel-plan-units.md`, section 7;
`planning/contracts/rendezvous.md`, section 5 and 5.1, points 6 to 8). Not
for merging. Measured in the `netsim` job of `.github/workflows/backend.yml`
against the gateways of NET-1 (`features/netsim`). A real router is the
owner's later run.

**Verdict.** The simulated router (miniupnpd 2.3.9) answers NAT-PMP and UPnP
IGDv2, and a node can map, read the external address, renew and delete over
both, but not correctly through go-nat v0.2.0 as it stands: its NAT-PMP path
publishes the requested port instead of the granted one, never deletes, and
ignores the context; its UPnP path picks random ports and fails where the
router restricts them. Used directly, go-nat-pmp and goupnp do every step
right. Three of the sixteen NAT pairs have no direct path and need the relay;
a mapping on either end, published as a candidate, made all three direct in
every run, also when the router granted another port than the one asked for.

## Setup

| Item | Version |
| --- | --- |
| CI runner | ubuntu-26.04, Linux 7.0.0-1016-aws x86_64, Go 1.26.0 |
| Gateway | miniupnpd 2.3.9, nftables backend, build options `ipv6 NAT-PMP PCP PCP-PEER leasefile check_portinuse igdv2`; nftables 1.1.6; conntrack-tools 1.4.9 |
| Libraries | go-nat v0.2.0 with goupnp v1.2.0, go-nat-pmp v1.0.2, go-netroute v0.2.1; pion webrtc v4.2.26, ice v4.4.8, turn v5.1.3 |

Every test builds fresh namespaces with `netsim.Start`. The offerer is the
left client, the answerer the right one; both clients are `10.0.0.2`, the
gateways' public addresses are `11.0.0.2` (left) and `11.0.0.6` (right), and a
pion TURN server in the WAN at `11.0.0.1:3478` serves STUN and TURN. A network
namespace belongs to one OS thread, while go-nat and pion open sockets on
their own goroutines, so each program that must live in a namespace runs as
a copy of the test binary under `ip netns exec` (`main_linux_test.go`). The
peers configure pion as `internal/peer` does: the answerer's host candidates
on a UDP mux on 47800, the offerer's on sockets of its own (ports 50000 to
59999 here, away from NET-1's symmetric pool 40000 to 40999), mDNS query only,
a `ctl` data channel, IPv4. A run counts as open when `ctl` echoes; its path
is `relay` when either side's selected pair has a relay candidate
(`flows/connect.md` step 8). Unanswered runs stop after 15 s.

Changes outside `spikes/S-8`: `StartUPnP` takes extra miniupnpd configuration
lines (a test needs permission rules to make the router grant other ports);
the `netsim` job also installs `conntrack` and builds and runs this package.

Reproduce: `go vet -tags netsim ./spikes/S-8`, then
`go test -c -tags netsim -o s8.test ./spikes/S-8` and
`sudo ./s8.test -test.v -test.count=1` on Linux with root, iproute2,
nftables, miniupnpd (nftables), conntrack.

## 1. Port mapping

### go-nat as X-1 added it (`TestLibrary`, `TestGrantedPortDiffers`, `TestUnansweredGateway`)

| Step | NAT-PMP | UPnP (IG2-IP2) |
| --- | --- | --- |
| Found by `DiscoverNATs` after | 0 ms | 2102 ms |
| External address | 11.0.0.6 | 11.0.0.6 |
| `AddPortMapping` UDP 47800 | random port (14059), reachable | random port (30841), reachable |
| Renewal (same call again) | same port | same port |
| `DeletePortMapping` | **still reachable** from new sources | gone for new sources |
| Router grants only 30000 to 30009 | **reports 55459**, unreachable; the router mapped 30000 | **fails**: 606 Action not authorized |
| Router stops answering, 2 s context | **no reply within 10 s** | not measured |

- `DiscoverNATs` closed after 5001 ms and `DiscoverGateway` returned
  `UPNP (IG2-IP2)` after 5002 ms: the generic search always waits 5 s
  (`upnp.go:152`) and the gateway discovered last wins (`nat.go`,
  `DiscoverGateway`). Its IGDv1 searches find nothing on an IGDv2 router.
- The failures are in go-nat's source: the NAT-PMP mapping result is
  discarded and the requested port returned (`natpmp.go:106`, `:115`);
  `DeletePortMapping` only forgets the port (`natpmp.go:125-127`); the client
  is `natpmp.NewClient`, which retries for about 128 s and has no context
  (`natpmp.go:43`; go-nat-pmp `natpmp.go:34-35`). Both protocols ask for a
  random external port between 10000 and 65534 (`nat.go:123-125`), and UPnP
  never falls back to `AddAnyPortMapping`. Not exercised: UPnP
  `AddPortMapping` returns port 0 with no error when the internal address
  cannot be found (`upnp.go:253`).

### The protocols used directly (`TestProtocols`, `TestGrantedPortDiffers`)

`10.0.0.3`, a second device on the right LAN, held 47800/UDP first.

| Step | NAT-PMP (go-nat-pmp, 2 s timeout) | IGDv2 WANIPConnection:2 (goupnp) |
| --- | --- | --- |
| Discovery | gateway from the default route, 0 ms | 2102 ms (SSDP, MX 2) |
| Ask for 47800 | granted **47801**, lifetime as asked, reachable | `AddPortMapping`: **718** ConflictInMappingEntry; `AddAnyPortMapping`: **47801**, reachable |
| Renewal | same request with 47801: same port | `AddPortMapping` on 47801: ok; `GetSpecificPortMappingEntry` shows the lease |
| Delete | lifetime 0: gone for new sources | `DeletePortMapping`: gone for new sources |
| Router grants only 30000 to 30009 | granted 30000, reachable | `AddPortMapping` 606; `AddAnyPortMapping` 30009, reachable |
| TCP | 47800 granted, connect from WAN ok | `AddAnyPortMapping` 47801, connect from WAN ok |
| Router stops answering | error "Timed out trying to contact gateway" after 2001 ms | not measured |

miniupnpd treats a request from the same internal client as an update of
that client's mapping (`upnpredirect.c:362-373` in 2.3.9): in run 44702763575
an `AddPortMapping` for 47800 silently took over the host's own mapping of
47800 to another internal port, so another program on the node's host can
lose its mapping to the node, or the reverse. NAT-PMP
moves to the next free port instead (`natpmp.c:333-401`).

### Lifetimes and established flows (`TestLease`)

- A 4 s lifetime: new sources were refused 4.7 s after the request, by
  NAT-PMP and IGDv2 alike (probes take up to 1 s). miniupnpd removes expired
  rules on time (`miniupnpd.c:2818`).
- Renewed every 2 s (half the lifetime) for 10 s: same port, still
  reachable.
- A flow that passed before deletion or expiry kept passing afterwards. The
  rule is gone, but the kernel's conntrack entry still translates that flow,
  and miniupnpd does not flush conntrack. So a live ICE pair outlives a lapsed
  mapping while traffic flows; only new remote endpoints are refused. Probes
  therefore come from a new source port each time.

## 2. Hole punching

`TestPunch` in run 44706019732. A cell gives the paths of its runs and the
offerer's time from start to the `ctl` echo; "none" means no connection in
15 s.

**No TURN, no mapping** (`direct`)

| offerer \ answerer | full-cone | restricted | port-restricted | symmetric |
| --- | --- | --- | --- | --- |
| full-cone | direct 1/1, 1009 ms | direct 1/1, 1008 ms | direct 1/1, 1007 ms | direct 1/1, 1007 ms |
| restricted | direct 1/1, 1011 ms | direct 1/1, 1008 ms | direct 1/1, 1009 ms | direct 1/1, 1008 ms |
| port-restricted | direct 1/1, 608 ms | direct 1/1, 607 ms | direct 1/1, 606 ms | none 1/1 |
| symmetric | direct 1/1, 606 ms | direct 1/1, 606 ms | none 1/1 | none 1/1 |

**TURN, no mapping** (`relay`)

| offerer \ answerer | full-cone | restricted | port-restricted | symmetric |
| --- | --- | --- | --- | --- |
| full-cone | direct 3/3, 1010-1019 ms | direct 3/3, 1010-1018 ms | direct 3/3, 1014-1021 ms | direct 3/3, 1018-1024 ms |
| restricted | direct 3/3, 1013-1015 ms | direct 3/3, 1016-1019 ms | direct 3/3, 1013-1015 ms | direct 3/3, 1025-1039 ms |
| port-restricted | direct 3/3, 1010-1015 ms | direct 3/3, 1011-1018 ms | direct 3/3, 1009-1011 ms | relay 3/3, 2014-2017 ms |
| symmetric | direct 3/3, 1011-1012 ms | direct 3/3, 1009-1017 ms | relay 3/3, 2015-2019 ms | relay 3/3, 2018-2023 ms |

**TURN, the answerer maps its UDP 47800 by NAT-PMP** (`answerer-mapped`)

| offerer \ answerer | full-cone | restricted | port-restricted | symmetric |
| --- | --- | --- | --- | --- |
| full-cone | direct 3/3, 611-614 ms | direct 3/3, 608-612 ms | direct 3/3, 608-613 ms | direct 3/3, 1016-1021 ms |
| restricted | direct 3/3, 609-616 ms | direct 3/3, 608-612 ms | direct 3/3, 608-610 ms | direct 3/3, 1020-1024 ms |
| port-restricted | direct 3/3, 610-615 ms | direct 3/3, 609-612 ms | direct 3/3, 614-618 ms | direct 3/3, 616-625 ms |
| symmetric | direct 3/3, 610-621 ms | direct 3/3, 613-619 ms | direct 3/3, 610-613 ms | direct 3/3, 614-615 ms |

**TURN, the offerer maps its offering socket by NAT-PMP** (`offerer-mapped`)

| offerer \ answerer | full-cone | restricted | port-restricted | symmetric |
| --- | --- | --- | --- | --- |
| full-cone | direct 3/3, 1015-1018 ms | direct 3/3, 1011-1018 ms | direct 3/3, 1012-1015 ms | direct 3/3, 1017-1032 ms |
| restricted | direct 3/3, 1014-1019 ms | direct 3/3, 1011-1014 ms | direct 3/3, 1012-1016 ms | direct 3/3, 1023-1027 ms |
| port-restricted | direct 3/3, 1013-1016 ms | direct 3/3, 1011-1016 ms | direct 3/3, 1012-1015 ms | direct 3/3, 1015-1020 ms |
| symmetric | direct 3/3, 1013-1022 ms | direct 3/3, 1013-1029 ms | direct 3/3, 1014-1018 ms | direct 3/3, 1016-1021 ms |

- **Relay-only pairs.** Port-restricted to symmetric, symmetric to
  port-restricted and symmetric to symmetric never connect directly: NET-1's
  symmetric NAT gives every new destination a new public port, and a
  port-restricted or symmetric NAT admits only replies from the exact address
  and port it sent to, so neither side's checks reach an endpoint the other
  has opened. Restricted to symmetric works because the restricted NAT
  filters by address only. With TURN these three take the relay after 2.0 s,
  pion's relay acceptance wait.
- **A mapping turns them direct.** The answerer mapped 47800 by NAT-PMP and
  trickled `11.0.0.6:<granted> typ srflx raddr 10.0.0.2 rport 47800`; every
  pair then connected directly, the relay-only ones included, in about 610
  ms. Mapping the offerer's socket instead had the same effect in about
  1010 ms: the answerer selected the offerer's published endpoint, and the
  offerer saw the answerer as peer-reflexive, whose pairs pion nominates
  after 1 s.
- **Another port granted** (`TestGrantedPortPublished`). With 47800 held by
  `10.0.0.3`, the router granted 47801; the answerer published it, and both
  port-restricted to symmetric and symmetric to symmetric connected directly
  through `11.0.0.6:47801` (selected remote `srflx udp 11.0.0.6:47801`).
- **ICE-TCP** (`TestICETCPMapped`): symmetric to symmetric, UDP between the
  gateways blocked, no TURN. Unmapped: no connection. The answerer's passive
  47801 mapped and published as `typ srflx ... tcptype passive`: no
  connection, because pion's parser keeps `tcptype` only for host candidates
  (`candidate_base.go`, `UnmarshalCandidate`, srflx branch), so no active
  dial is made. Published as `typ host ... tcptype passive` with the external
  address: connected over TCP in 206 ms.
- **Outlier: a relay path where a direct one exists.** Across the four CI
  runs, 5 of 63 runs with TURN, a mapped answerer and a restricted-cone
  offerer took the relay (3 in the matrix of runs 1 to 3, 2 in the repeat
  test of run 3); none of the 160 runs of the `relay` variant, none of the
  160 `offerer-mapped` runs, and none with another offerer type. Run
  44706019732 had none. The traces of
  `TestMappedRestrictedToPortRestricted` (run 44704969215, 2 of 10 relayed)
  show the cause in pion ice v4.4.8: a check relayed through the answerer's
  TURN allocation reaches the offerer before that relay candidate is
  trickled, so the offerer files it as peer-reflexive and validates the pair
  at once. When the relay candidate arrives, pion replaces the
  peer-reflexive one but keeps the pair's old priority (`agent.go:1247-1262`,
  `setPriorityOverride`). That pair stays the best valid one, and the
  controlling side nominates only the best valid pair once its type's wait
  has passed (`selection.go:74-84`), relay 2 s. Valid direct pairs found at
  about 630 ms are never nominated, and the pair selected at 2.01 s reads
  `prio 7926339740339534848 ... host <-> relay (remote, prio 1023)`. The
  relayed connection works; `flows/connect.md` section B probes it back to
  direct.

## 3. Failures and their causes

| # | Failure | Cause |
| --- | --- | --- |
| 1 | go-nat NAT-PMP publishes a port the router did not grant | `natpmp.go:106`, `:115` drop `MappedExternalPort` |
| 2 | go-nat NAT-PMP mapping survives `DeletePortMapping` | `natpmp.go:125-127` sends no lifetime-0 request |
| 3 | go-nat NAT-PMP calls block about 128 s against a silent router | `natpmp.NewClient` (`natpmp.go:43`), no context |
| 4 | go-nat UPnP fails when the router restricts ports | random ports only, no `AddAnyPortMapping` (`upnp.go:266`) |
| 5 | go-nat discovery takes 5 s | generic SSDP search of 5 s (`upnp.go:152`) |
| 6 | Three NAT pairs have no direct path | symmetric mapping against port or address filtering, section 2 |
| 7 | A mapped TCP endpoint published as srflx is never dialled | pion drops `tcptype` on srflx (`UnmarshalCandidate`) |
| 8 | Occasional relay with a direct path available | pion prflx-to-relay priority override (`agent.go:1247-1262`) |
| 9 | Deletion and expiry do not cut a flow already seen | conntrack keeps translating it; miniupnpd does not flush it |

## 4. What PR-3 should do

1. **Clients.** Use go-nat-pmp (`NewClientWithTimeout`) and goupnp's
   `internetgateway2` (WANIPConnection:2, then WANIPConnection:1 and
   WANPPPConnection:1 for IGDv1) directly; both are already in `go.sum` under
   go-nat. Do not use go-nat's `NAT` objects (failures 1 to 5). Whether
   go-nat stays pinned in `deps_test.go`, where X-1 put it, is the
   coordinator's decision.
2. **Order and timeouts.** On start and on a network change, look up the
   gateway from the default route and send NAT-PMP with a 2 s total timeout;
   in parallel run one IGD discovery (2.1 s here). Use NAT-PMP when it
   answers (0 ms here, and it returns the granted port in one round trip),
   else IGD: `AddPortMapping` for the internal port, and on 718 or 606
   `AddAnyPortMapping` (IGDv2) or a few other ports (IGDv1, untested here).
   Cap everything at about 3 s and never hold a connection for it: the
   mapped candidate is trickled when it exists.
3. **What to ask for.** UDP 47800 (answering socket), TCP 47800
   (signalling), TCP 47801 (ICE-TCP), each with the internal port as the
   suggested external port.
4. **Lifetime and renewal.** Ask for 7200 s (RFC 6886, section 3.3; never 0,
   which IGDv2 treats as 604800 s and IGDv1 as permanent), renew at half the
   granted lifetime with the granted port as the suggestion, and republish
   when the port changes. On shutdown delete (NAT-PMP lifetime 0, IGD
   `DeletePortMapping`). Expiry and deletion end new flows only.
5. **What to publish.** Always the granted port and the external address
   the protocol reports (NAT-PMP external address, `GetExternalIPAddress`),
   never the requested port. UDP: an extra `ice` candidate per answered
   connection, `typ srflx raddr <host> rport 47800`, because pion's address
   rewrite has no port. ICE-TCP: `typ host ... tcptype passive` with the
   external address, until pion keeps `tcptype` on srflx. Signalling: the
   external TCP port in the URL reported in `hello`.
6. **Offering sockets (5.1, point 6).** Mapping the offering socket gives a
   gain in one topology only: neither router admits the other and only the
   offerer's router maps. When the answerer's router maps, it adds nothing.
   It costs one mapping request per connection. A decision for the
   coordinator; the contract's rule (answering socket and listeners) suffices
   when the answering node's router maps.
7. **Upstream.** Failures 7 and 8 are pion's; reporting them upstream is
   the coordinator's call.

## 5. CI runs

All in the `netsim` job of draft pull request #134.

| Run | What it showed |
| --- | --- |
| [44702763575](https://github.com/huanchong-99/SoloDawn/actions/runs/44702763575) | First pass. Deletion and expiry looked ineffective because the probes reused one source port (conntrack, section 1); the IGD takeover by the same client |
| [44704071068](https://github.com/huanchong-99/SoloDawn/actions/runs/44704071068) | Probes from new ports; matrices with three runs; one relay outlier; mapped ICE-TCP published as srflx never dialled |
| [44704969215](https://github.com/huanchong-99/SoloDawn/actions/runs/44704969215) | ICE trace of the relay outlier (2 of 10 relayed); ICE-TCP published as host connects |
| [44706019732](https://github.com/huanchong-99/SoloDawn/actions/runs/44706019732) | Final test code, every test passed, no outlier; the tables above |

## 6. Not verified

- Real routers (the owner's run), IGDv1-only, NAT-PMP-only and PCP-only
  gateways, double NAT and carrier-grade NAT (an external address that is not
  public), IPv6 pinholes, a router that loses its mappings on reboot.
- Two peers behind one NAT and hairpinning: NET-1 has one client per
  gateway. NET-1 gives both clients `10.0.0.2`, so host candidates reach the
  sender's own namespace.
- Browsers as offerers, macOS and Windows, the real `internal/peer` code
  (the spike repeats its pion settings), TURN over TLS, lifetimes of 7200 s.
- Why the relay outlier needs a mapped answerer in these runs (a plausible
  reason: the mapping lets the answerer learn the offerer's endpoint sooner,
  so the relayed check can beat the trickled relay candidate), and whether
  a later pion release fixes it.
