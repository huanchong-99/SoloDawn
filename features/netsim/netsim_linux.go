//go:build netsim

// Package netsim builds isolated IPv4 networks for Linux integration tests.
//
// It is compiled only with the build tag "netsim" and needs root (CAP_SYS_ADMIN
// and CAP_NET_ADMIN) and the commands ip, nft, sysctl and sh; StartUPnP also
// needs miniupnpd (nftables backend), ss and timeout. A missing privilege or
// command fails the test with the failing command and its output.
package netsim

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// NAT is the UDP mapping and filtering behavior of one gateway.
type NAT string

const (
	FullCone       NAT = "full-cone"
	Restricted     NAT = "restricted"
	PortRestricted NAT = "port-restricted"
	Symmetric      NAT = "symmetric"

	// Both private networks have the same addressing and independent routes.
	ClientIP = "10.0.0.2"
	// WANIP is reachable from both private networks (e.g. for STUN or TURN).
	// miniupnpd requires globally routable address space on the isolated WAN.
	WANIP = "11.0.0.1"
)

// Network has no links to the host. Use "ip netns exec <namespace> ..."
// to start a process in a client, its gateway, or the WAN.
type Network struct {
	WAN         string
	Left, Right Peer
}

// Peer is one client namespace behind its own gateway namespace.
type Peer struct {
	Client, Gateway string // namespace names
	PublicIP        string // address of the WAN-facing gateway interface
}

//go:embed gateway.sh
var gatewayScript string

func run(t testing.TB, input string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	if err := cmd.Run(); err != nil {
		t.Fatalf("netsim %v: %v", cmd.Args, err)
	}
}

// Start creates two single-client NATs and a routed WAN. All namespaces and
// links are removed when the test ends, including after partial setup.
func Start(t testing.TB, left, right NAT) Network {
	t.Helper()
	prefix := "netsim-" + rand.Text() + "-"
	ns := func(name string) string {
		t.Helper()
		name = prefix + name
		run(t, "", "ip", "netns", "add", name)
		t.Cleanup(func() { run(t, "", "ip", "netns", "delete", name) })
		run(t, "", "ip", "-n", name, "link", "set", "lo", "up")
		return name
	}

	n := Network{WAN: ns("wan")}
	peer := func(side, uplink, public string, mode NAT) Peer {
		p := Peer{Client: ns(side), Gateway: ns(side + "-gw"), PublicIP: public}
		run(t, gatewayScript, "sh", "-eu", "-s", "--", n.WAN, side, uplink, public,
			p.Gateway, p.Client, string(mode))
		return p
	}
	n.Left = peer("left", WANIP, "11.0.0.2", left)
	n.Right = peer("right", "11.0.0.5", "11.0.0.6", right)
	run(t, "", "ip", "netns", "exec", n.WAN, "sysctl", "-qw", "net.ipv4.ip_forward=1")
	run(t, "table ip netsim {\nchain direct { type filter hook forward priority filter; }\n}\n",
		"ip", "netns", "exec", n.WAN, "nft", "-f", "-")
	return n
}

// SetBlocked atomically changes the peer-to-peer path, including live flows.
// Supply the advertised ICE-TCP port to block, or zero to allow all TCP.
// Traffic to services in the WAN namespace remains reachable.
func (n Network) SetBlocked(t testing.TB, udp bool, iceTCP uint16) {
	t.Helper()
	rules := "flush chain ip netsim direct\n"
	if udp {
		rules += "add rule ip netsim direct meta l4proto udp drop\n"
	}
	if iceTCP != 0 {
		for _, port := range []string{"sport", "dport"} {
			rules += fmt.Sprintf("add rule ip netsim direct tcp %s %d drop\n", port, iceTCP)
		}
	}
	run(t, rules, "ip", "netns", "exec", n.WAN, "nft", "-f", "-")
}

// StartUPnP starts miniupnpd (nftables backend) on this gateway, with UPnP
// IGD and NAT-PMP available to its LAN. Its process belongs to the test.
// Each config line is added to miniupnpd's configuration, for example a
// permission rule that makes the gateway grant another external port.
func (p Peer) StartUPnP(t testing.TB, config ...string) {
	t.Helper()
	// miniupnpd adds its rules to these chains of the gateway's table.
	run(t, `add chain ip netsim miniupnpd
add chain ip netsim prerouting_miniupnpd
add chain ip netsim postrouting_miniupnpd
insert rule ip netsim pre jump prerouting_miniupnpd
insert rule ip netsim post jump postrouting_miniupnpd
`, "ip", "netns", "exec", p.Gateway, "nft", "-f", "-")
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "ip", "netns", "exec", p.Gateway,
		"miniupnpd", "-d", "-4", "-f", "/dev/stdin", "-i", "wan", "-a", "lan",
		"-N", "-S", "-p", "5000", "-P", filepath.Join(dir, "miniupnpd.pid"))
	cmd.Stdin = strings.NewReader("lease_file=" + filepath.Join(dir, "leases") + "\n" +
		"upnp_nftables_family_split=yes\nupnp_table_name=netsim\nupnp_nat_table_name=netsim\n" +
		strings.Join(append(config, ""), "\n"))
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	if err := cmd.Start(); err != nil {
		t.Fatalf("netsim %v: %v", cmd.Args, err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	// Wait for the listening sockets in the gateway before returning to the client.
	run(t, "", "ip", "netns", "exec", p.Gateway, "timeout", "5", "sh", "-ec",
		`until [ -n "$(ss -Hlun 'sport = :5351')" ] && [ -n "$(ss -Hltn 'sport = :5000')" ]; do sleep 0.02; done`)
}
