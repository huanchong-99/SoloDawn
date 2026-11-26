package node

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var diagStamp = regexp.MustCompile(`^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d\.\d{6} INFO `)

// diagLines is logs/node.log of dir, from the event on.
func diagLines(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "logs", "node.log"))
	mustNoErr(t, err)
	var lines []string
	for line := range strings.Lines(string(raw)) {
		if stamp := diagStamp.FindString(line); stamp != "" {
			lines = append(lines, strings.TrimSpace(line[len(stamp):]))
		}
	}
	return lines
}

// diagOnce fails unless each line of want is in lines once, in that order.
func diagOnce(t *testing.T, lines []string, want ...string) {
	t.Helper()
	at := -1
	for _, line := range want {
		i := slices.Index(lines, line)
		if i <= at || slices.Contains(lines[i+1:], line) {
			t.Fatalf("%q is not once after line %d of\n%s", line, at, strings.Join(lines, "\n"))
		}
		at = i
	}
}

func diagNodeLine(event, id, level string) string {
	return fmt.Sprintf("%s node=%s version=%s os=%s level=%s", event, id[:8], Version, runtime.GOOS, level)
}

func TestDiagNodeEvents(t *testing.T) {
	dir := socketRoot(t)
	l := openSocket(t, dir)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	n, err := openNode(ctx, dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	served := make(chan struct{}) // single reader: TestDiagNodeEvents
	go func() { n.serve(l); close(served) }()

	mustNoErr(t, (Command{Cmd: "status"}).send(dir, io.Discard))
	owner := adoptTestBrowser(t, n)
	fc := startAdoptedConn(t, n, owner.ID())
	join := owner.Sign(fmt.Sprintf(`{"t":"host.none","n":%q,"ts":%d,"to":%q}`, sesID("diag"), time.Now().UnixMilli(), n.key.ID()))
	fc.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: join})
	fc.next(t)
	// An offline target would retire the offer when the first candidate is
	// reported offline, which can fail Offer itself (flows/connect.md step 15).
	online, err := peer.Dial(t.Context(), "ws://127.0.0.1:"+strconv.Itoa(n.httpPort)+"/rv", owner)
	mustNoErr(t, err)
	t.Cleanup(func() { online.Close() })
	conn, err := n.peer.Offer(t.Context(), owner.ID())
	mustNoErr(t, err)
	mustNoErr(t, conn.Close())
	n.serveInbound(conn)
	mustNoErr(t, n.saveRendezvous([]string{"ws://127.0.0.1:1/rv"}))
	n.syncRendezvous()
	home := sesKey("diag-home").ID()
	n.dialHome(t.Context(), home, nil)
	stop()
	<-served
	n.Close()
	n.Close()

	lines, node := diagLines(t, dir), "node="+n.key.ID()[:8]
	diagOnce(t, lines, diagNodeLine("node.start", n.key.ID(), "info"), diagNodeLine("node.stop", n.key.ID(), "info"), "settle.begin "+node)
	diagOnce(t, lines, "home.dial "+node+" home="+home[:8])
	for _, name := range []string{"home_dial", "host_cmd", "inbound", "local", "local_rv", "rendezvous", "retention"} {
		diagOnce(t, lines, "bg.start "+node+" name="+name, "bg.stop "+node+" name="+name, "settle.begin "+node)
	}
}

func TestDiagLevelAndOutput(t *testing.T) {
	log.SetFlags(log.LstdFlags)
	for content, level := range map[string]slog.Level{"": slog.LevelInfo, "debug\n": slog.LevelDebug, "debugging": slog.LevelInfo} {
		dir := socketRoot(t)
		if content != "" {
			mustNoErr(t, os.MkdirAll(filepath.Join(dir, "logs"), 0o700))
			mustNoErr(t, os.WriteFile(filepath.Join(dir, "logs", "level"), []byte(content), 0o600))
		}
		out, flags, debug := log.Writer(), log.Flags(), slog.Default().Enabled(t.Context(), slog.LevelDebug)
		n, err := openNode(t.Context(), dir)
		mustNoErr(t, err)
		mustEqual(t, log.Flags(), log.Ldate|log.Lmicroseconds|log.LUTC)
		mustEqual(t, slog.Default().Enabled(t.Context(), slog.LevelDebug), level == slog.LevelDebug)
		n.Close()
		mustEqual(t, log.Writer(), out)
		mustEqual(t, log.Flags(), flags)
		mustEqual(t, slog.Default().Enabled(t.Context(), slog.LevelDebug), debug)
		diagOnce(t, diagLines(t, dir), diagNodeLine("node.start", n.key.ID(), strings.ToLower(level.String())), diagNodeLine("node.stop", n.key.ID(), strings.ToLower(level.String())))
	}
}

// diagGate holds the write of one line until open is closed.
type diagGate struct {
	line      string
	hit, open chan struct{}
	once      *sync.Once
}

func (g diagGate) Write(b []byte) (int, error) {
	if strings.Contains(string(b), g.line) {
		g.once.Do(func() { close(g.hit) })
		<-g.open
	}
	return len(b), nil
}

// The caller may hold a mutex (syncRendezvous holds nodeRuntime.mu), so the
// goroutine writes its start.
func TestDiagWorkWritesItsStart(t *testing.T) {
	n, err := openNode(t.Context(), socketRoot(t))
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	mustNoErr(t, n.saveRendezvous([]string{"ws://127.0.0.1:1/rv"}))
	for name, start := range map[string]func(){"rendezvous": n.syncRendezvous, "test": func() { n.spawn("test", func() {}) }} {
		gate := diagGate{"bg.start node=" + n.key.ID()[:8] + " name=" + name, make(chan struct{}), make(chan struct{}), new(sync.Once)}
		out := log.Writer()
		log.SetOutput(gate)
		started := make(chan struct{}) // single reader: TestDiagWorkWritesItsStart
		go func() { start(); close(started) }()
		for what, done := range map[string]chan struct{}{"the line": gate.hit, "the caller to return": started} {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error(timedOut(5*time.Second, what, nil))
			}
		}
		close(gate.open)
		<-started
		log.SetOutput(out)
	}
}
