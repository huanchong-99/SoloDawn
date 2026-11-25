package host

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func putExternalMember(h *Host, hostID, id string) {
	m := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 20}, Group: testGroup1, Kind: "ai", Name: "Scout"},
		CLI:       externalAgent, Tool: "ZCode", Host: hostID, HostName: "n", Account: "api_key", Allow: []string{}, Approvers: []string{}, State: "offline", Path: "local",
	}
	h.Rows.Put(testGroup1, "member", id, json.RawMessage(marshalWire(m)))
	h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: id}, Kind: GroupSession, Folder: "repo", Agent: externalAgent, Name: "Scout"})
}

func newExternalRig(t *testing.T, disk bool) *deliverRig {
	r := newDeliverRig(t, disk)
	r.seedGroupAndMembers()
	putExternalMember(r.h, r.hostKey.ID(), testAI1)
	r.h.Sessions.Update(SessionKey{Group: testGroup1, Member: testAI1}, func(s *Session) { s.Home = r.homeKey.ID() })
	return r
}

func (r *deliverRig) pushExternalItem(it proto.Item) {
	it.AI = testAI1
	var msg *proto.MsgRow
	if it.Why == "mention" {
		msg = r.makeTextMsg(it.Msg, nil, "hi")
	}
	before := r.h.QueuedCount(testGroup1, testAI1)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, msg)
	r.waitUntil("item held", func() bool { return r.h.QueuedCount(testGroup1, testAI1) == before+1 })
}

func (r *deliverRig) putMissingRepo() {
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 2}, Name: "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}, Repo: filepath.Join(r.t.TempDir(), "missing")})
}

func mustState(t *testing.T, h *Host, wantState, wantTask string) {
	t.Helper()
	if state, task := h.MemberState(testGroup1, testAI1); state != wantState || task != wantTask {
		t.Fatalf("state = %s %q, want %s %q", state, task, wantState, wantTask)
	}
}

func TestExternalHandOutAndRunEnds(t *testing.T) {
	r := newExternalRig(t, false)
	h := r.h
	if _, ok, err := h.ExternalTake(testGroup1, testAI1); ok || err != nil {
		t.Fatalf("empty take = %v %v", ok, err)
	}
	r.pushExternalItem(proto.Item{Item: 5, Why: "mention", Msg: testMsg1})
	if len(r.runtime.recorded()) != 0 {
		t.Fatal("an external AI was given a turn")
	}
	got, ok, err := h.ExternalTake(testGroup1, testAI1)
	want := proto.Handout{Run: 5, Kind: "group", Folder: filepath.Join(projectDir(h, testGroup1), "repo"),
		Context: groupInstructions + "\n" + externalInstructions + "\ngroup:0", Text: "group:1"}
	if !ok || err != nil || got != want {
		t.Fatalf("take = %+v %v %v, want %+v", got, ok, err, want)
	}
	mustState(t, h, "working", "")
	if r.home.ackCountFor(5) != 1 || r.h.QueuedCount(testGroup1, testAI1) != 0 {
		t.Fatal("hand-out not acknowledged")
	}
	r.pushExternalItem(proto.Item{Item: 6, Why: "mention", Msg: testMsg2})
	if _, ok, _ := h.ExternalTake(testGroup1, testAI1); ok {
		t.Fatal("handed out while a run is open")
	}
	h.ExternalDone(testGroup1, testAI1, 4, "")
	mustState(t, h, "working", "")
	h.ExternalDone(testGroup1, testAI1, 5, "")
	mustState(t, h, "offline", "")
	leave := h.ExternalPresent(testGroup1, testAI1)
	defer leave()
	mustState(t, h, "waiting", "")
	if got, _, _ := h.ExternalTake(testGroup1, testAI1); got.Run != 6 {
		t.Fatalf("second run = %d", got.Run)
	}
	h.ExternalDone(testGroup1, testAI1, 6, "exit 1")
	mustState(t, h, "waiting", "")
	cmds := strings.Join(r.home.cmdPayloads(), "\n")
	if !strings.Contains(cmds, `"code":"turn_interrupted","reason":"crash","detail":"exit 1"`) {
		t.Fatalf("no crash report in %s", cmds)
	}
	if g, ok := h.ExternalMember(testAI1); g != testGroup1 || !ok {
		t.Fatal("external member not found")
	}
	if _, ok := h.ExternalMember(testAI2); ok {
		t.Fatal("a managed AI is an external member")
	}
}

func TestExternalTakeWithoutHandOut(t *testing.T) {
	task := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "T", Slug: "t", Origin: "brief", State: "active", Step: "work", Assignee: testAI2}
	t.Run("stale item", func(t *testing.T) {
		r := newExternalRig(t, false)
		r.putTask(task)
		r.pushExternalItem(proto.Item{Item: 7, Why: "assign", Task: testTask1})
		if _, ok, err := r.h.ExternalTake(testGroup1, testAI1); ok || err != nil || r.h.QueuedCount(testGroup1, testAI1) != 0 {
			t.Fatalf("stale take = %v %v", ok, err)
		}
	})
	t.Run("task folder fails", func(t *testing.T) {
		r := newExternalRig(t, true)
		r.putMissingRepo()
		mine := task
		mine.Assignee = testAI1
		r.putTask(mine)
		r.pushExternalItem(proto.Item{Item: 8, Why: "assign", Task: testTask1})
		if _, ok, err := r.h.ExternalTake(testGroup1, testAI1); ok || err != nil || r.h.QueuedCount(testGroup1, testAI1) != 0 {
			t.Fatalf("failed task take = %v %v", ok, err)
		}
		if cmds := strings.Join(r.home.cmdPayloads(), "\n"); !strings.Contains(cmds, `"op":"fail"`) {
			t.Fatalf("task not failed: %s", cmds)
		}
	})
	t.Run("group clone fails", func(t *testing.T) {
		r := newExternalRig(t, true)
		r.putMissingRepo()
		r.pushExternalItem(proto.Item{Item: 9, Why: "mention", Msg: testMsg1})
		if _, ok, err := r.h.ExternalTake(testGroup1, testAI1); ok || err == nil || r.h.QueuedCount(testGroup1, testAI1) != 1 {
			t.Fatalf("clone failure take = %v %v", ok, err)
		}
	})
	t.Run("compose fails", func(t *testing.T) {
		r := newExternalRig(t, false)
		r.composeE = errors.New("compose")
		r.pushExternalItem(proto.Item{Item: 10, Why: "mention", Msg: testMsg1})
		if _, ok, err := r.h.ExternalTake(testGroup1, testAI1); ok || !errors.Is(err, r.composeE) || r.h.QueuedCount(testGroup1, testAI1) != 1 {
			t.Fatalf("compose failure take = %v %v", ok, err)
		}
	})
}

// externalBubble is a host with an external AI member in a group whose turns
// last a minute, settled when the test ends.
func externalBubble(t *testing.T) *Host {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h, _, err := New(db, testSeedKey(12), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Settle)
	h.Rows.Put(testGroup1, "group", testGroup1, json.RawMessage(marshalWire(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: "home", Secretary: testSec, Checks: []string{}, Limits: proto.Limits{TurnMinutes: 1},
	})))
	putExternalMember(h, h.Key.ID(), testAI1)
	return h
}

func TestExternalPresenceAndTimeLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := externalBubble(t)
		mustState(t, h, "offline", "")
		first, second := h.ExternalPresent(testGroup1, testAI1), h.ExternalPresent(testGroup1, testAI1)
		mustState(t, h, "waiting", "")
		first()
		first()
		second()
		time.Sleep(externalPresence - time.Nanosecond)
		synctest.Wait()
		mustState(t, h, "waiting", "")
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		mustState(t, h, "offline", "")
		h.ExternalPresent(testGroup1, testAI1)()

		ep := extOf(h)
		s, _ := h.Sessions.Get(SessionKey{Group: testGroup1, Member: testAI1})
		extOpenRun(h, ep, aiGroupKey{group: testGroup1, member: testAI1}, s, 3, Turn{})
		mustState(t, h, "working", "")
		time.Sleep(time.Minute)
		synctest.Wait()
		mustState(t, h, "waiting", "")
		extOpenRun(h, ep, aiGroupKey{group: testGroup1, member: testAI1}, s, 4, Turn{})
	})
}

func TestSettleExternalWaitsForItsTimersAndArmsNoMore(t *testing.T) {
	saveRuntimeGlobals(t)
	synctest.Test(t, func(t *testing.T) {
		h := externalBubble(t)
		started, release, settled := make(chan struct{}), make(chan struct{}), make(chan struct{})
		Deliver = func(*Host, string, string) {
			close(started)
			<-release
		}
		ep, key := extOf(h), aiGroupKey{group: testGroup1, member: testAI1}
		s, _ := h.Sessions.Get(SessionKey{Group: testGroup1, Member: testAI1})
		extOpenRun(h, ep, key, s, 1, Turn{})
		<-started
		leave := h.ExternalPresent(testGroup1, testAI1)
		go func() {
			h.Settle()
			close(settled)
		}()
		synctest.Wait()
		select {
		case <-settled:
			t.Error("Settle returned before the run's timeout finished")
		default:
		}
		leave()
		extOpenRun(h, ep, key, s, 2, Turn{})
		close(release)
		<-settled
		if len(ep.away) != 0 || len(ep.runs) != 0 {
			t.Fatalf("armed after settle: away %v runs %v", ep.away, ep.runs)
		}
	})
}

func TestExternalTaskHandOutNamesTheTaskByItsNumber(t *testing.T) {
	r := newExternalRig(t, true)
	initTestOriginAndGroupRepo(t, r, "none")
	putExternalMember(r.h, r.hostKey.ID(), testAI1)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 7, Title: "T", Slug: "t", Origin: "brief", State: "active", Step: "work", Assignee: testAI1})
	r.pushExternalItem(proto.Item{Item: 8, Why: "assign", Task: testTask1})
	got, ok, err := r.h.ExternalTake(testGroup1, testAI1)
	if !ok || err != nil || got.Kind != "task" || got.Task != "T-7" {
		t.Fatalf("task take = %+v %v %v", got, ok, err)
	}
}
