package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func TestRun(t *testing.T) {
	const usage = "usage: solodawn version | rendezvous [--listen address]\n"
	flags := strings.ReplaceAll(`Usage of rendezvous:
  -cert string
	PEM certificate chain for both hostnames
  -domain string
	signalling and STUN hostname
  -key string
	PEM certificate private key
  -listen string
	signalling address (TLS :443 or local HTTP 127.0.0.1:8080)
  -no-relay
	offer STUN but no relay
  -relay-quota-mb int
	per-device UTC daily limit in MiB (default 50)
  -turn-domain string
	TURN-over-TLS hostname
  -udp-port int
	STUN and TURN UDP port (default 3478)
`, "\n\t", "\n    \t")
	for _, tc := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"version"}, 0, "dev\n", ""},
		{nil, 2, "", usage},
		{[]string{"rendezvous", "extra"}, 2, "", flags},
		{[]string{"rendezvous", "--unknown"}, 2, "", "flag provided but not defined: -unknown\n" + flags},
		{[]string{"rendezvous", "--listen", ":9000"}, 1, "", "stopped :9000\n"},
		{[]string{"rendezvous"}, 1, "", "stopped \n"},
		{[]string{"unknown"}, 2, "", usage},
		{[]string{"version", "extra"}, 2, "", usage},
	} {
		var stdout, stderr bytes.Buffer
		code := run(tc.args, &stdout, &stderr, func(o node.RendezvousOptions) error { return fmt.Errorf("stopped %s", o.Listen) })
		if code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want %d, %q, %q", tc.args, code, stdout.String(), stderr.String(), tc.code, tc.stdout, tc.stderr)
		}
	}
}

func TestRelayFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want node.RendezvousOptions
	}{
		{nil, node.RendezvousOptions{UDPPort: 3478, QuotaMB: 50}},
		{strings.Fields("--listen :443 --domain a --turn-domain b --cert c --key d --udp-port 123 --relay-quota-mb 9 --no-relay"), node.RendezvousOptions{Listen: ":443", Domain: "a", TurnDomain: "b", Cert: "c", Key: "d", UDPPort: 123, QuotaMB: 9, NoRelay: true}},
	} {
		var stderr bytes.Buffer
		code := rendezvous(tc.args, &stderr, func(got node.RendezvousOptions) error {
			if got != tc.want {
				t.Fatalf("parsed %+v; want %+v", got, tc.want)
			}
			return fmt.Errorf("stopped")
		})
		if code != 1 || stderr.String() != "stopped\n" {
			t.Fatal(code, stderr.String())
		}
	}
}
