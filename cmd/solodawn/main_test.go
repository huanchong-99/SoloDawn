package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func TestMainUsesCommandExitCode(t *testing.T) {
	previous, args, stderr := exit, os.Args, os.Stderr
	t.Cleanup(func() { exit, os.Args, os.Stderr = previous, args, stderr })
	var err error
	os.Stderr, err = os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Stderr.Close()
	code := -1
	exit = func(n int) { code = n }
	os.Args = []string{"solodawn", "unknown"}
	main()
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRun(t *testing.T) {
	const usage = "usage: solodawn <command> [arguments]\n"
	flags := strings.ReplaceAll(`Usage of rendezvous:
  -acme-ca string
	PEM roots for the ACME server's HTTPS only
  -acme-directory string
	ACME server directory URL (empty: Let's Encrypt)
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
  -relay-ports first-last
	relay allocation UDP ports, first-last (default 20000-29999)
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
		{[]string{"rendezvous", "--listen", "bad", "extra"}, 2, "", flags},
		{[]string{"rendezvous", "--listen", "bad", "--unknown"}, 2, "", "flag provided but not defined: -unknown\n" + flags},
		{[]string{"rendezvous", "--listen", "bad"}, 1, "", "listen tcp: address bad: missing port in address\n"},
		{[]string{"unknown"}, 2, "", usage},
		{[]string{"version", "extra"}, 2, "", usage},
		{[]string{"node", "extra"}, 2, "", usage},
		{[]string{"status", "extra"}, 2, "", usage},
		{[]string{"pair"}, 2, "", usage},
		{[]string{"pair", ""}, 2, "", usage},
		{[]string{"pair", "--name"}, 2, "", usage},
		{[]string{"pair", "secret", "--name"}, 2, "", usage},
		{[]string{"pair", "secret", "--other", "name"}, 2, "", usage},
		{[]string{"pair", "secret", "--rendezvous"}, 2, "", usage},
		{[]string{"pair", "secret", "extra"}, 2, "", usage},
		{[]string{"mcp"}, 2, "", usage},
		{[]string{"mcp", "--token", ""}, 2, "", usage},
	} {
		var stdout, stderr bytes.Buffer
		code := run(tc.args, &stdout, &stderr)
		if code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want %d, %q, %q", tc.args, code, stdout.String(), stderr.String(), tc.code, tc.stdout, tc.stderr)
		}
	}
}

func TestLocalArgumentsReachTheSocket(t *testing.T) {
	dir := testDataDir(t)
	l := testListener(t, dir)
	host, _ := os.Hostname()
	for _, tc := range []struct {
		args []string
		line string
	}{
		{[]string{"status"}, `{"cmd":"status"}`},
		{[]string{"open"}, `{"cmd":"open"}`},
		{[]string{"pair-browser"}, `{"cmd":"pair-browser"}`},
		{[]string{"pair", "secret"}, `{"cmd":"pair","secret":"secret","name":"` + host + `"}`},
		{[]string{"pair", "secret", "--name", "label with spaces"}, `{"cmd":"pair","secret":"secret","name":"label with spaces"}`},
		{[]string{"pair", "secret", "--rendezvous", "https://rv.example"}, `{"cmd":"pair","secret":"secret","name":"` + host + `","rendezvous":"https://rv.example"}`},
		{[]string{"pair", "secret", "--rendezvous", "https://rv.example", "--name", "laptop"}, `{"cmd":"pair","secret":"secret","name":"laptop","rendezvous":"https://rv.example"}`},
	} {
		received := replyToCommand(l)
		var out, stderr bytes.Buffer
		if code := run(tc.args, &out, &stderr); code != 0 || out.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", tc.args, code, out.String(), stderr.String())
		}
		if got := <-received; got != tc.line+"\n" {
			t.Errorf("%v: got %q", tc.args, got)
		}
	}
}

func TestNodeAlreadyRunning(t *testing.T) {
	testListener(t, testDataDir(t))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"node"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.String() != "a node is already running\n" {
		t.Fatalf("node: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

// testDataDir points the data directory at a short temporary root: macOS
// limits a socket path to 103 bytes, and its temporary directory is deep.
func testDataDir(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "sd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	for _, name := range []string{"LocalAppData", "HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, root)
	}
	dir, err := node.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func testListener(t *testing.T, dir string) *node.Local {
	t.Helper()
	l, err := node.ListenLocal(dir)
	if l == nil || err != nil {
		t.Fatalf("listener = %v, error = %v", l, err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestNodeErrorIsOneLine(t *testing.T) {
	l := testListener(t, testDataDir(t))
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		bufio.NewReader(conn).ReadString('\n')
		fmt.Fprintln(conn, `{"text":"partial"}`)
		fmt.Fprintln(conn, `{"done":true,"error":"first\r\nsecond\nthird"}`)
	}()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"status"}, &stdout, &stderr); code != 1 || stdout.String() != "partial\n" || stderr.String() != "first  second third\n" {
		t.Fatalf("status: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func replyToCommand(l *node.Local) <-chan string {
	received := make(chan string, 1)
	go func() {
		l.SetDeadline(time.Now().Add(2 * time.Second))
		conn, err := l.Accept()
		if err != nil {
			received <- err.Error()
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			received <- err.Error()
			return
		}
		received <- line
		fmt.Fprintln(conn, `{"done":true,"ok":true}`)
	}()
	return received
}

func TestRelayFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want node.RendezvousOptions
	}{
		{nil, node.RendezvousOptions{UDPPort: 3478, QuotaMB: 50}},
		{strings.Fields("--listen :443 --domain a --turn-domain b --cert c --key d --udp-port 123 --relay-quota-mb 9 --no-relay --acme-directory e --acme-ca f --relay-ports 30000-30010"), node.RendezvousOptions{Listen: ":443", Domain: "a", TurnDomain: "b", Cert: "c", Key: "d", ACMEDirectory: "e", ACMECA: "f", UDPPort: 123, QuotaMB: 9, NoRelay: true}},
	} {
		if len(tc.args) != 0 {
			tc.want.RelayPorts.First, tc.want.RelayPorts.Last = 30000, 30010
		}
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

func TestMCPRoundTrip(t *testing.T) {
	l := testListener(t, testDataDir(t))
	received := make(chan string, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		received <- line
	}()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"mcp", "--token", "tok-9"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("mcp: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if got := <-received; got != "{\"mcp\":\"tok-9\"}\n" {
		t.Fatalf("mcp opening = %q", got)
	}
}
