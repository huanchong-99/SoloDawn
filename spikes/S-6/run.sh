#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# All listeners, the routing table, the hosts file and browser trust are private to the run.
if [[ ${1:-} != inside ]]; then
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    go test -c -o "$work/spike.test"
    version=v2.10.3
    pebble=$(go env GOMODCACHE)/github.com/letsencrypt/pebble/v2@$version
    GOBIN="$work" go install github.com/letsencrypt/pebble/v2/cmd/pebble@$version
    sudo unshare --net --mount --propagation private bash "$PWD/run.sh" inside "$work" "$pebble" "$PATH" "$(command -v google-chrome)"
    exit
fi
work=$2
pebble=$3
export PATH=$4 CHROME=$5
ip link set lo up
ip link add lab type dummy
ip addr add 192.0.2.10/24 dev lab
ip link set lab up
ip route add default dev lab
sysctl -q -w net.ipv4.ip_unprivileged_port_start=443
printf '127.0.0.1 localhost\n192.0.2.10 rv.example.test turn.rv.example.test\n' > "$work/hosts"
mount --bind "$work/hosts" /etc/hosts
cd "$pebble"
jq '.pebble |= (.tlsPort=443 | .listenAddress="127.0.0.1:14000" |
    .managementListenAddress="127.0.0.1:15000" | .retryAfter={authz:1,order:1} |
    .profiles={default:{description:"S-6 renewal",validityPeriod:75}})' test/config/pebble-config.json > "$work/config"
export PEBBLE_VA_NOSLEEP=1 PEBBLE_WFE_NONCEREJECT=0 PEBBLE_AUTHZREUSE=0
runuser -u "$SUDO_USER" -- "$work/pebble" -config "$work/config" > "$work/pebble.log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true' EXIT
export NO_PROXY='*' no_proxy='*'
ca=test/certs/pebble.minica.pem
curl -fsS --retry 10 --retry-all-errors --retry-delay 1 --cacert "$ca" https://localhost:15000/roots/0 > "$work/root.pem"
cat "$ca" "$work/root.pem" > "$work/roots.pem"
cd - >/dev/null
export SSL_CERT_FILE="$work/roots.pem" HOME="$work/home"
mkdir -p "$HOME"
chown "$SUDO_USER" "$HOME"
as_user=(setpriv --reuid "$(id -u "$SUDO_USER")" --regid "$(id -g "$SUDO_USER")" --init-groups)
db=$HOME/.local/share/pki/nssdb
"${as_user[@]}" mkdir -p "$db"
"${as_user[@]}" certutil -d "sql:$db" -N --empty-password
"${as_user[@]}" certutil -d "sql:$db" -A -n 'S-6 disposable issuer' -t 'C,,' -i "$work/root.pem"
"${as_user[@]}" "$CHROME" --version
"${as_user[@]}" "$work/spike.test" -test.v -test.timeout=3m
rg 'validated|Validating|Issued certificate' "$work/pebble.log" || true
