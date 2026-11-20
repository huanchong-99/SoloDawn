// Package netsim builds isolated IPv4 networks for Linux integration tests.
package netsim

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"os"
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

type Peer struct {
	Client, Gateway string // namespace names
	PublicIP        string // address of the WAN-facing gateway interface
}

//go:embed gateway.sh
var gatewayScript string

func require(t testing.TB, uid int, names ...string) {
	t.Helper()
	if uid != 0 {
		t.Skip("netsim requires root to create Linux network namespaces")
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("netsim requires %s: %v", name, err)
		}
	}
}

func run(t testing.TB, input string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	checkCommand(t, cmd, cmd.Run())
}

func checkCommand(t testing.TB, cmd *exec.Cmd, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("netsim %v: %v", cmd.Args, err)
	}
}

// A container may have UID 0 without permission to create or configure a netns.
func namespaceIP(t testing.TB, args ...string) {
	t.Helper()
	cmd := exec.Command("ip", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "Operation not permitted") || strings.Contains(msg, "Permission denied") {
		t.Skipf("netsim requires namespace and network administration privileges: %s", msg)
	}
	t.Fatalf("netsim %v: %v: %s", cmd.Args, err, msg)
}

// Start creates two single-client NATs and a routed WAN. All namespaces and
// links are removed when the test ends, including after partial setup.
func Start(t testing.TB, left, right NAT) Network {
	t.Helper()
	require(t, os.Geteuid(), "ip", "nft", "sysctl", "sh")
	prefix := "netsim-" + rand.Text() + "-"
	ns := func(name string) string {
		t.Helper()
		name = prefix + name
		namespaceIP(t, "netns", "add", name)
		t.Cleanup(func() { run(t, "", "ip", "netns", "delete", name) })
		namespaceIP(t, "-n", name, "link", "set", "lo", "up")
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
func (p Peer) StartUPnP(t testing.TB) {
	t.Helper()
	require(t, os.Geteuid(), "miniupnpd", "ss", "timeout")
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "ip", "netns", "exec", p.Gateway,
		"miniupnpd", "-d", "-4", "-f", "/dev/stdin", "-i", "wan", "-a", "lan",
		"-N", "-S", "-p", "5000", "-P", filepath.Join(dir, "miniupnpd.pid"))
	cmd.Stdin = strings.NewReader("lease_file=" + filepath.Join(dir, "leases") + "\n")
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	checkCommand(t, cmd, cmd.Start())
	t.Cleanup(func() { _ = cmd.Wait() })
	// Wait for the listening sockets in the gateway before returning to the client.
	run(t, "", "ip", "netns", "exec", p.Gateway, "timeout", "5", "sh", "-ec",
		`until [ -n "$(ss -Hlun 'sport = :5351')" ] && [ -n "$(ss -Hltn 'sport = :5000')" ]; do sleep 0.02; done`)
}
