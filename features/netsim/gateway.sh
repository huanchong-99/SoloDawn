# Invoked by Start with a fresh WAN, gateway and client namespace.
wan=$1 side=$2 uplink=$3 public=$4 gw=$5 client=$6 mode=$7

ip link add lan netns "$gw" type veth peer name eth0 netns "$client"
ip link add wan netns "$gw" type veth peer name "$side" netns "$wan"
address() {
    ip -n "$1" address add "$3" dev "$2"
    ip -n "$1" link set "$2" up
}
address "$client" eth0 10.0.0.2/24
address "$gw" lan 10.0.0.1/24
address "$gw" wan "$public/30"
address "$wan" "$side" "$uplink/30"
ip -n "$client" route add default via 10.0.0.1
ip -n "$gw" route add default via "$uplink"
ip netns exec "$gw" sysctl -qw net.ipv4.ip_forward=1

# Each cone preserves the client's port. Remember the allowed source of an
# inbound packet for that mapping: anyone, a contacted IP, or an IP and port.
types=inet_service
out='udp sport'
in='udp dport'
case "$mode" in
    full-cone) ;;
    restricted|port-restricted)
        types="$types . ipv4_addr"
        out="$out . ip daddr"
        in="$in . ip saddr"
        if [ "$mode" = port-restricted ]; then
            types="$types . inet_service"
            out="$out . udp dport"
            in="$in . udp sport"
        fi ;;
    symmetric) ;;
    *) echo "unknown NAT type: $mode" >&2; exit 1 ;;
esac

if [ "$mode" = symmetric ]; then
    # Allocate a new public port for each new remote tuple. Conntrack
    # preserves it for that flow and reverses it only for matching replies.
    learn=''
    incoming=''
    snat="meta l4proto udp snat to $public:numgen inc mod 20000 offset 40000"
else
    learn="iifname \"lan\" oifname \"wan\" meta l4proto udp update @allowed { $out }"
    incoming="iifname \"wan\" ip daddr $public $in @allowed dnat to 10.0.0.2"
    snat=''
fi

ip netns exec "$gw" nft -f - <<EOF
table ip netsim {
    set allowed { type $types; flags dynamic,timeout; timeout 2m; }
    chain learn {
        type filter hook forward priority filter;
        $learn
    }
    chain input {
        type filter hook input priority filter;
        iifname "wan" meta l4proto udp drop
    }
    chain pre {
        type nat hook prerouting priority dstnat + 1;
        $incoming
    }
    chain post {
        type nat hook postrouting priority srcnat + 1;
        oifname "wan" jump translate
    }
    chain translate {
        $snat
        snat to $public
    }
}
EOF
