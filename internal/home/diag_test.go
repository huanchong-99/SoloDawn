package home_test

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// diagLog keeps what the standard logger writes, from the INFO text on.
type diagLog struct {
	mu    sync.Mutex
	lines []string
	wake  chan struct{}
}

// diagCapture points the logger at a diagLog until the test is over. Call it
// before the loop starts: the loop closes before the output is put back.
func diagCapture(t *testing.T) *diagLog {
	t.Helper()
	d := &diagLog{wake: make(chan struct{}, 1)}
	was := log.Writer()
	log.SetOutput(writerFunc(func(b []byte) (int, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, text, ok := strings.Cut(strings.TrimSuffix(string(b), "\n"), " INFO "); ok {
			d.lines = append(d.lines, text)
			select {
			case d.wake <- struct{}{}:
			default:
			}
		}
		return len(b), nil
	}))
	t.Cleanup(func() { log.SetOutput(was) })
	return d
}

func (d *diagLog) pos(line string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, l := range d.lines {
		if l == line {
			return i
		}
	}
	return -1
}

func (d *diagLog) count(prefix string) (n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, l := range d.lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// expect waits for the line, which is the event and its fields as they are
// written, and returns where it is in the log.
func (d *diagLog) expect(t *testing.T, line string) int {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		if i := d.pos(line); i >= 0 {
			return i
		}
		select {
		case <-d.wake:
		case <-timeout:
			d.mu.Lock()
			defer d.mu.Unlock()
			t.Fatal(timedOut(10*time.Second, fmt.Sprintf("%q", line), d.lines))
		}
	}
}

func diagNode(id string) string { return "node=" + id[:8] }

func diagDev(key identity.Key) string { return "dev=" + key.ID()[:8] }

func TestDiagAttachAndDetach(t *testing.T) {
	d := diagCapture(t)
	w := runStart(t, runOpen(t), runTables())
	p := w.attach(browser)
	d.expect(t, "home.attach "+diagNode(node.ID())+" "+diagDev(browser))
	w.loop.Detach(p.conn)
	d.expect(t, "home.detach "+diagNode(node.ID())+" "+diagDev(browser)+" why=ended")

	inner := runNewClient()
	inner.refuse.Store(true)
	slow := w.attachClient(laptop, inner, inner)
	w.loop.Welcome(slow.conn, runWelcome)
	d.expect(t, "home.detach "+diagNode(node.ID())+" "+diagDev(laptop)+" why=gave_up")
}

func TestDiagBackgroundWork(t *testing.T) {
	d := diagCapture(t)
	w, _ := runFounded(t, runTables(), func(cfg *home.LoopConfig) { cfg.Every = time.Hour })
	w.loop.Close()
	n := diagNode(node.ID())
	for _, line := range []string{"clock", "pump", "watch", "group g=" + groupID} {
		a := d.expect(t, "bg.start "+n+" name="+line)
		b := d.expect(t, "bg.stop "+n+" name="+line)
		equal(t, a < b, true)
	}
}

func TestDiagWebPushDelivery(t *testing.T) {
	d := diagCapture(t)
	srv, got := wpushServer(t, nil)
	w := wpushStartWorld(t, runOpen(t), wpushHomeNode, nil)
	wpushFoundGroup(t, w, wpushGroupTwo, "Two", 10)
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 96, wpushGroupTwo, wpushMakeKeys(36, srv.URL+"/one"), []string{"mention"}))
	wpushPostGroupTwoMention(t, w, 97, "Deliver")
	wpushRecv(t, got, 1)
	w.loop.Close()
	n := diagNode(wpushHomeNode.ID())
	a := d.expect(t, "bg.start "+n+" name=webpush g="+wpushGroupTwo)
	b := d.expect(t, "bg.stop "+n+" name=webpush g="+wpushGroupTwo)
	equal(t, a < b, true)
}

func TestDiagReplyAnswers(t *testing.T) {
	d := diagCapture(t)
	tables := runTables()
	tables.Commands["break"] = runFixed(func(s *home.State, c home.Command) home.Result {
		return home.Result{Writes: []home.Write{setting{"k", "v"}, setting{"k", "v"}}}
	})
	w, p := runFounded(t, tables)
	reply := func(re int64, t, code string) string {
		return fmt.Sprintf("cmd.reply %s g=%s %s re=%d t=%s code=%s", diagNode(node.ID()), groupID, diagDev(browser), re, t, code)
	}

	d.expect(t, reply(1, "group/create", "ok"))
	p.does(groupID, "note", `,"text":"x"`)
	d.expect(t, reply(2, "note", "ok"))
	p.fails(groupID, "note", `,"text":""`, "conflict")
	d.expect(t, reply(3, "note", "conflict"))
	p.fails(groupID, "break", ``, "internal")
	d.expect(t, reply(4, "break", "internal"))

	p.re++
	w.loop.Cmd(p.conn, p.re, p.key.Sign(`{"t":"note","n":"`+runID(w.nonce.Add(1))+`","ts":`+fmt.Sprint(clock)+`}`))
	p.next()
	d.expect(t, fmt.Sprintf(`cmd.reply %s g="" %s re=5 t=note code=bad_request`, diagNode(node.ID()), diagDev(browser)))

	equal(t, w.loop.ExportGroup(groupID, filepath.Join(t.TempDir(), "move.db"), laptop.ID()), nil)
	p.send(groupID, "note", `,"text":"x"`)
	p.next()
	d.expect(t, reply(6, "note", "moved"))
}

func TestDiagRowsBeforeTheReplyAndTheInbox(t *testing.T) {
	d := diagCapture(t)
	tables := runTables()
	tables.Commands["jump"] = runWake
	w, p := runFounded(t, tables)
	n, g := diagNode(node.ID()), "g="+groupID
	d.expect(t, "rows.send "+n+" "+g+" seq=1-3 to=0")

	p.watch(groupID, false)
	host := w.attach(laptop)
	host.watch(groupID, true)
	w.loop.Inbox(host.conn)
	re := p.send(groupID, "note", `,"text":"x"`)
	p.next()
	p.next()
	rows := d.expect(t, "rows.send "+n+" "+g+" seq=4-4 to=1")
	reply := d.expect(t, fmt.Sprintf("cmd.reply %s %s %s re=%d t=note code=ok", n, g, diagDev(browser), re))
	equal(t, rows < reply, true)

	p.send(groupID, "jump", `,"hold":0,"why":"mention"`)
	p.next()
	p.next()
	d.expect(t, "inbox.send "+n+" "+g+" "+diagDev(laptop)+" items=1")
	equal(t, d.count("inbox.send "), 1) // the delivery that found nothing when the host took the inbox wrote no line
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	d.expect(t, "ack.recv "+n+" "+g+" "+diagDev(laptop)+" item=1")
}
