//go:build netsim

// Package s8 is spike S-8: router port mapping and UDP hole punching in the
// network simulation of NET-1 (features/netsim). Results are in results.md.
//
// A network namespace belongs to an OS thread, and go-nat and pion open
// sockets on goroutines of their own. So every program that has to live in
// one namespace runs as a copy of this test binary under "ip netns exec",
// selected by S8_ROLE. The test talks to it in JSON lines: requests on the
// child's stdin, events on its file descriptor 3. Its stdout and stderr go to
// the test log.
package s8

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMain(m *testing.M) {
	if role := os.Getenv("S8_ROLE"); role != "" {
		os.Exit(serve(role))
	}
	os.Exit(m.Run())
}

func serve(role string) int {
	out := newEmitter(os.NewFile(3, "events"))
	var err error
	switch role {
	case "mapper":
		err = runMapper(os.Stdin, out)
	case "peer":
		err = runPeer(os.Stdin, out)
	case "relay":
		err = runRelay(os.Stdin, out)
	default:
		err = fmt.Errorf("unknown role %q", role)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", role, err)
		return 1
	}
	return 0
}

// msg is every line between the test and a role process; each field is
// used by some requests or events only.
type msg struct {
	T        string   `json:"t"`
	NAT      string   `json:"nat,omitempty"`
	Proto    string   `json:"proto,omitempty"`
	Internal int      `json:"internal,omitempty"`
	External int      `json:"external,omitempty"`
	Lifetime int      `json:"lifetime,omitempty"`
	Types    []string `json:"types,omitempty"`
	After    []int64  `json:"after,omitempty"`
	IP       string   `json:"ip,omitempty"`
	Port     int      `json:"port,omitempty"`
	Ms       int64    `json:"ms,omitempty"`
	Err      string   `json:"err,omitempty"`
	SDP      string   `json:"sdp,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Cand     string   `json:"cand,omitempty"`
	Local    string   `json:"local,omitempty"`
	Remote   string   `json:"remote,omitempty"`
}

type emitter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func newEmitter(w io.Writer) *emitter { return &emitter{enc: json.NewEncoder(w)} }

func (e *emitter) send(m msg) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.enc.Encode(m) // the reader is gone only when the test has ended
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func since(start time.Time) int64 { return time.Since(start).Milliseconds() }

// child is one role process in one namespace.
type child struct {
	name string
	in   *emitter
	out  chan msg
	done chan struct{}
}

func spawn(t *testing.T, ns, role string, env ...string) *child {
	t.Helper()
	exe, err := os.Executable()
	must(t, err)
	cmd := exec.Command("ip", "netns", "exec", ns, exe)
	cmd.Env = append(append(os.Environ(), "S8_ROLE="+role), env...)
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	stdin, err := cmd.StdinPipe()
	must(t, err)
	r, w, err := os.Pipe()
	must(t, err)
	cmd.ExtraFiles = []*os.File{w}
	must(t, cmd.Start())
	w.Close()
	c := &child{name: role, in: newEmitter(stdin), out: make(chan msg, 64), done: make(chan struct{})}
	go func() {
		defer close(c.out)
		lines := bufio.NewScanner(r)
		lines.Buffer(make([]byte, 64<<10), 1<<20)
		for lines.Scan() {
			var m msg
			if json.Unmarshal(lines.Bytes(), &m) != nil {
				continue
			}
			select {
			case c.out <- m:
			case <-c.done:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(c.done)
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		r.Close()
	})
	return c
}

// next waits for the child's next event.
func (c *child) next(t *testing.T, timeout time.Duration) (msg, bool) {
	t.Helper()
	select {
	case m, ok := <-c.out:
		if !ok {
			t.Fatalf("%s exited", c.name)
		}
		return m, true
	case <-time.After(timeout):
		return msg{}, false
	}
}

// call sends a request and returns the child's reply.
func (c *child) call(t *testing.T, req msg, timeout time.Duration) msg {
	t.Helper()
	c.in.send(req)
	m, ok := c.next(t, timeout)
	if !ok {
		t.Fatalf("%s: no reply to %+v within %s", c.name, req, timeout)
	}
	return m
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// inNamespace runs fn on a thread in the named namespace; only socket
// creation needs it (as in features/netsim).
func inNamespace(t *testing.T, name string, fn func()) {
	t.Helper()
	runtime.LockOSThread()
	previous, err := os.Open("/proc/thread-self/ns/net")
	must(t, err)
	defer previous.Close()
	next, err := os.Open("/var/run/netns/" + name)
	must(t, err)
	defer next.Close()
	must(t, unix.Setns(int(next.Fd()), unix.CLONE_NEWNET))
	defer func() {
		must(t, unix.Setns(int(previous.Fd()), unix.CLONE_NEWNET))
		runtime.UnlockOSThread()
	}()
	fn()
}

func udpSocket(t *testing.T, ns, addr string) *net.UDPConn {
	t.Helper()
	var conn *net.UDPConn
	inNamespace(t, ns, func() {
		var err error
		conn, err = net.ListenUDP("udp4", net.UDPAddrFromAddrPort(netip.MustParseAddrPort(addr)))
		must(t, err)
	})
	t.Cleanup(func() { conn.Close() })
	return conn
}

// arrives sends one datagram and reports whether it reached to in time.
func arrives(t *testing.T, from, to *net.UDPConn, dst netip.AddrPort) bool {
	t.Helper()
	label := fmt.Sprintf("probe %s %d", dst, time.Now().UnixNano())
	_, err := from.WriteToUDPAddrPort([]byte(label), dst)
	must(t, err)
	must(t, to.SetReadDeadline(time.Now().Add(700*time.Millisecond)))
	var buf [128]byte
	for {
		n, _, err := to.ReadFromUDPAddrPort(buf[:])
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return false
		}
		must(t, err)
		if string(buf[:n]) == label {
			return true
		}
	}
}

// command runs a program and fails the test with its output.
func command(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return string(out)
}
