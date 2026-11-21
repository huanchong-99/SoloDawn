# S-1 transport bench — do not merge

Run from this directory on a disposable Linux VM with Go, Node 20+, iproute2,
iptables, tcpdump, curl and passwordless sudo. Dependency installation needs the
network; the measurements do not.

```sh
npm ci
npx playwright install --with-deps chromium firefox webkit
./bench.sh
```

The script builds the nested Go module and creates two network namespaces
connected **only to each other**. It removes them on exit and prints the log
directory: per-node gather times, browser observations (JSONL) and an mDNS
trace. Pass `nodes`, `chromium`, `firefox` or `webkit` to select peers;
e.g. `S1_CASES=tcp,renom ./bench.sh firefox`. To measure complete descriptions
with IPv6 host candidates too, use `S1_DUAL=1 S1_CASES=all ./bench.sh chromium firefox webkit`.

A hosts the browser, signalling relay and first Pion peer; B hosts the second
peer and UDP TURN. Both nodes dial the relay. Only signed descriptions cross
the WebSocket; test setup and stats use HTTP; echoes use ordered reliable
text messages on `ctl`. Keys live for one run; the driver signs for the
browser so the test does not depend on WebCrypto support.

The TCP trial offers no UDP on B. For live path changes, firewall rules in A
block direct traffic to/from B but leave signalling and TURN reachable, then
are removed without touching the peer connection. `relay` is a separate
browser relay-only control. `silent` adds a nonresponding STUN endpoint.
The node-only trial compares two directions on the same fixed UDP ports with
distinct outgoing ports. See the draft PR description for measured answers.
