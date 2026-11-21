#!/usr/bin/env bash
# Run as a normal user with sudo; all peers and firewall rules stay in fresh namespaces.
set -euo pipefail
cd "$(dirname "$0")"
out=$(mktemp -d)
echo "Logs: $out"
go build -o "$out/peer" .
node=$(command -v node)
ns=()
cleanup() {
  for n in "${ns[@]}"; do
    sudo ip netns pids "$n" | xargs -r sudo kill 2>/dev/null || :
    sudo ip netns del "$n"
  done
}
trap cleanup EXIT
for n in s1-browser s1-node; do sudo ip netns add "$n"; ns+=("$n"); done
sudo ip link add brow netns s1-browser type veth peer name node netns s1-node
for side in "s1-browser brow 1 2" "s1-node node 2 1"; do
  read -r n dev host other <<< "$side"
  sudo ip -n "$n" addr add "10.71.0.$host/24" dev "$dev"
  sudo ip -n "$n" link set lo up
  sudo ip -n "$n" link set "$dev" up
  sudo ip -n "$n" route add default via "10.71.0.$other"
  ip6=""
  if [[ ${S1_DUAL:-0} == 1 ]]; then
    ip6="fd71::$host"
    sudo ip -n "$n" -6 addr add "$ip6/64" dev "$dev" nodad
    sudo ip -n "$n" -6 route add default via "fd71::$other"
  fi
  sudo ip netns exec "$n" sudo -u "$USER" env S1_IP="10.71.0.$host" S1_IP6="$ip6" "$out/peer" >"$out/$n.log" 2>&1 &
  for i in {1..50}; do sudo ip netns exec "$n" curl -fs http://127.0.0.1:8080/id >/dev/null && break; sleep .1; done
done
sudo ip netns exec s1-node tcpdump -lnni node udp port 5353 >"$out/mdns.log" 2>&1 &
sudo ip netns exec s1-browser sudo -u "$USER" env S1_CASES="${S1_CASES:-}" S1_DUAL="${S1_DUAL:-0}" "$node" run.cjs "$@" | tee "$out/results.jsonl"
