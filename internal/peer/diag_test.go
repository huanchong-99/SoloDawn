package peer

import (
	"fmt"
	"log"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

type diagLog struct {
	mu    sync.Mutex
	lines []string
	wake  chan struct{}
}

const diagTimeout = 5 * time.Second

func (d *diagLog) Write(b []byte) (int, error) {
	d.mu.Lock()
	d.lines = append(d.lines, strings.TrimSpace(string(b)))
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
	return len(b), nil
}

func captureDiag(t *testing.T) *diagLog {
	d := &diagLog{wake: make(chan struct{}, 1)}
	flags, out := log.Flags(), log.Writer()
	level := slog.SetLogLoggerLevel(slog.LevelDebug)
	log.SetFlags(0)
	log.SetOutput(d)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
		slog.SetLogLoggerLevel(level)
	})
	return d
}

func (d *diagLog) forConn(conn string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var lines []string
	for _, line := range d.lines {
		if slices.Contains(strings.Fields(line), "conn="+conn) {
			lines = append(lines, line)
		}
	}
	return lines
}

func (d *diagLog) expect(t *testing.T, conn, line string) {
	t.Helper()
	timer := time.NewTimer(diagTimeout)
	defer timer.Stop()
	for {
		lines := d.forConn(conn)
		if slices.Contains(lines, line) {
			return
		}
		select {
		case <-d.wake:
		case <-timer.C:
			t.Fatal(timedOut(diagTimeout, fmt.Sprintf("%q", line), lines))
		}
	}
}

func recvDiagnostic[T any](t *testing.T, what string, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(diagTimeout):
		t.Fatal(timedOut(diagTimeout, what, nil))
		panic("unreachable")
	}
}

func diagLine(event, node, dev, conn, fields string) string {
	return event + " node=" + node[:8] + " dev=" + dev[:8] + " conn=" + conn + fields
}

func TestPeerDiagnostics(t *testing.T) {
	d := captureDiag(t)
	f, a, b := diagnosticPair(t)
	conn := a.diag.key.conn
	aID, bID := f.a.key.ID(), f.b.key.ID()
	want := append(openDiagnostics(aID, bID, conn, "offer", "answer"),
		openDiagnostics(bID, aID, conn, "answer", "offer")...)
	for _, line := range want {
		d.expect(t, conn, line)
	}
	got := d.forConn(conn)
	slices.Sort(got)
	slices.Sort(want)
	eq(t, got, want)

	check(t, a.Close())
	check(t, a.Close())
	d.expect(t, conn, diagLine("INFO ice.state", aID, bID, conn, " state=closed"))
	assertCloseDiagnostic(t, d.forConn(conn), aID, bID, conn, "closed", time.Since(a.opened))
	recvDiagnostic(t, "answerer to see ctl close", b.Done())
	assertCloseDiagnostic(t, d.forConn(conn), bID, aID, conn, "ctl_closed", time.Since(b.opened))
}

func TestPeerDiagnosticsIceFailedBeforeCtlOpened(t *testing.T) {
	d := captureDiag(t)
	f := newPair(t)
	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	c.connectionStateChanged(webrtc.PeerConnectionStateFailed)
	recvDiagnostic(t, "connection to close", c.Done())
	assertCloseDiagnostic(t, d.forConn(c.diag.key.conn), f.a.key.ID(), f.b.key.ID(), c.diag.key.conn, "ice_failed", 0)
}

func TestPeerDiagnosticsRemoteStateClosed(t *testing.T) {
	d := captureDiag(t)
	f := newPair(t)
	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	c.connectionStateChanged(webrtc.PeerConnectionStateClosed)
	recvDiagnostic(t, "connection to close", c.Done())
	assertCloseDiagnostic(t, d.forConn(c.diag.key.conn), f.a.key.ID(), f.b.key.ID(), c.diag.key.conn, "ctl_closed", 0)
}

func diagnosticPair(t *testing.T) (*pair, *Connection, *Connection) {
	t.Helper()
	f := newPair(t)
	engine := loopbackEngine()
	engine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	f.a.sockets.offer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	engine.SetICEUDPMux(f.b.sockets.udp)
	f.b.sockets.answer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	a, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { a.Close() })
	deliver(t, f.b, f.bMail.route(), recvDiagnostic(t, "offer", f.aMail["offer"]))
	deliver(t, f.a, f.aMail.route(), recvDiagnostic(t, "answer", f.bMail["answer"]))
	deliver(t, f.b, f.bMail.route(), recvDiagnostic(t, "offerer's candidate", f.aMail["ice"]))
	deliver(t, f.a, f.aMail.route(), recvDiagnostic(t, "answerer's candidate", f.bMail["ice"]))
	recvDiagnostic(t, "offerer's ctl to open", a.Ready())
	b := recvDiagnostic(t, "answerer's ctl to open", f.opened)
	t.Cleanup(func() { b.Close() })
	return f, a, b
}

func openDiagnostics(node, dev, conn, sent, received string) []string {
	return []string{
		diagLine("INFO sig.send", node, dev, conn, " kind="+sent),
		diagLine("INFO sig.recv", node, dev, conn, " kind="+received),
		diagLine("DEBUG sig.send", node, dev, conn, " kind=ice"),
		diagLine("DEBUG sig.recv", node, dev, conn, " kind=ice"),
		diagLine("INFO ice.state", node, dev, conn, " state=connecting"),
		diagLine("INFO ice.state", node, dev, conn, " state=connected"),
		diagLine("INFO ice.pair", node, dev, conn, " local=host remote=host proto=udp path=direct"),
		diagLine("INFO ctl.open", node, dev, conn, ""),
	}
}

func assertCloseDiagnostic(t *testing.T, lines []string, node, dev, conn, why string, elapsed time.Duration) {
	t.Helper()
	prefix := diagLine("INFO conn.close", node, dev, conn, "")
	var closed []string
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			closed = append(closed, line)
		}
	}
	eq(t, len(closed), 1)
	rest, ok := strings.CutPrefix(closed[0], prefix+" why="+why+" ms=")
	eq(t, ok, true)
	ms, err := strconv.ParseInt(rest, 10, 64)
	check(t, err)
	if ms < 0 || ms > elapsed.Milliseconds() {
		t.Fatalf("unexpected open duration %dms in %q", ms, closed[0])
	}
}
