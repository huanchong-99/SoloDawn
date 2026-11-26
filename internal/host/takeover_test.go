package host

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	tkSec   = "S3cr3tAry-AAAAAAAAAAAA"
	tkTask  = "TkTaskAAAAAAAAAAAAAAAA"
	tkOther = "TkOtherAAAAAAAAAAAAAAA"
)

type tkOwner struct {
	mu      sync.Mutex
	path    string
	changed chan struct{}
	done    chan struct{}
}

func newTkOwner(path string) *tkOwner {
	return &tkOwner{path: path, changed: make(chan struct{}, 1), done: make(chan struct{})}
}

func (o *tkOwner) Send(any) error               { return nil }
func (o *tkOwner) PathChanged() <-chan struct{} { return o.changed }
func (o *tkOwner) Done() <-chan struct{}        { return o.done }

func (o *tkOwner) Path() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.path
}

func (o *tkOwner) setPath(p string) {
	o.mu.Lock()
	o.path = p
	o.mu.Unlock()
	select {
	case o.changed <- struct{}{}:
	default:
	}
}

type tkDriver struct {
	*fakeDriver
	interactive func(l Launch) ([]string, string, error)
}

func (d tkDriver) Interactive(l Launch) ([]string, string, error) { return d.interactive(l) }

type tkRig struct {
	t         *testing.T
	h         *Host
	home      *runHome
	drv       *fakeDriver
	owner     *tkOwner
	folder    string // the workspace of every AI of the rig
	mu        sync.Mutex
	opened    []Launch
	ptyErr    error
	delivered chan aiSlot
}

func newTkRig(t *testing.T) *tkRig {
	t.Helper()
	h, rh, drv, _ := setupRunEnv(t)
	r := &tkRig{t: t, h: h, home: rh, drv: drv, owner: newTkOwner("direct"), folder: t.TempDir(), delivered: make(chan aiSlot, 64)}
	Holds["takeover"] = takeoverHold
	Deliver = func(_ *Host, g, m string) {
		select {
		case r.delivered <- aiSlot{group: g, member: m}:
		default:
		}
	}
	Drivers["claude"] = tkDriver{drv, r.interactive}
	Drivers["codex"] = tkDriver{drv, func(l Launch) ([]string, string, error) {
		if l.Session == "" {
			return nil, "", errors.New("codex: no thread")
		}
		return r.interactive(l)
	}}
	prevAttach := takeoverAttach
	t.Cleanup(func() { takeoverAttach = prevAttach })
	rh.cmdFn = func(map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	drain := make(chan struct{})
	go func() {
		for {
			select {
			case <-rh.cmdNotify:
			case <-drain:
				return
			}
		}
	}()
	t.Cleanup(func() {
		stopHostNow(h)
		h.Settle()
		close(drain)
	})
	r.putGroup()
	r.putMember(runAI1, "claude", false)
	r.putMember(runAI2, "codex", false)
	r.putMember(tkSec, "claude", true)
	drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete, p.autoReadyID = true, "thread-1"
		return p, nil
	}
	return r
}

func (r *tkRig) putGroup() {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 1}, Name: "Website",
		Home: "home-dev", Secretary: tkSec, Pass: 85,
	}
	r.h.Rows.Put(runGroup1, "group", runGroup1, json.RawMessage(marshalWire(grp)))
}

func (r *tkRig) putMember(id, cli string, sec bool) {
	m := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 2}, Group: runGroup1, Kind: "ai", Name: cli},
		CLI:       cli, Host: r.h.Key.ID(), Secretary: sec, Allow: []string{}, Approvers: []string{},
	}
	r.h.Rows.Put(runGroup1, "member", id, json.RawMessage(marshalWire(m)))
	// The group session, as host.join records it.
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: runGroup1, Member: id}, Kind: GroupSession, Folder: r.folder, Agent: cli, Home: "home-dev", Name: cli, Secretary: sec})
}

func (r *tkRig) putTask(id, state, assignee, reviewer string, num int) {
	task := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id, Seq: 3}, Group: runGroup1, Num: num, Title: "Fix it", Origin: "direct", State: state, Step: "work", Assignee: assignee, Reviewer: reviewer, Depends: []string{},
	}
	r.h.Rows.Put(runGroup1, "task", id, json.RawMessage(marshalWire(task)))
}

func (r *tkRig) interactive(l Launch) ([]string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opened = append(r.opened, l)
	if r.ptyErr != nil {
		return nil, "", r.ptyErr
	}
	return ptyHelperArgv(r.t, "pty-echo"), cmp.Or(l.Session, "new-session"), nil
}

func (r *tkRig) lastOpened() Launch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.opened[len(r.opened)-1]
}

func (r *tkRig) start(member, task string) error {
	return r.startFrom(r.owner, member, task)
}

func (r *tkRig) startFrom(from Owner, member, task string) error {
	_, err := Surface["takeover.start"](context.Background(), r.h, from, "owner-dev", json.RawMessage(r.request(member, task)))
	return err
}

func (r *tkRig) request(member, task string) string {
	return marshalWire(proto.TakeoverStart{
		Takeover: proto.Takeover{HostCommand: proto.HostCommand{Type: "takeover.start", Nonce: proto.ID("Nn" + member[2:]), To: "node"}, Group: runGroup1, Member: proto.ID(member)},
		Task:     proto.ID(task),
	})
}

func (r *tkRig) end(member string) {
	end := proto.Takeover{HostCommand: proto.HostCommand{Type: "takeover.end", Nonce: proto.NewNonce(), To: "node"}, Group: runGroup1, Member: proto.ID(member)}
	if _, err := Surface["takeover.end"](context.Background(), r.h, r.owner, "owner-dev", json.RawMessage(marshalWire(end))); err != nil {
		r.t.Fatal(err)
	}
}

func (r *tkRig) sent(t, op string) []map[string]any {
	var out []map[string]any
	for _, c := range r.home.snapshotCmds() {
		if c.payload["t"] == t && (op == "" || c.payload["op"] == op || c.payload["kind"] == op) {
			out = append(out, c.payload)
		}
	}
	return out
}

// lastState is the state of member in the last member status sent.
func (r *tkRig) lastState(member string) any {
	var state any
	for _, c := range r.sent("member", "status") {
		if c["id"] == member {
			state = c["state"]
		}
	}
	return state
}

func (r *tkRig) waitFor(desc string, ok func() bool) {
	r.t.Helper()
	poll(r.t, 15*time.Second, desc, func() string {
		if ok() {
			return ""
		}
		return fmt.Sprintf("turns=%d delivered=%d sessions=%s", r.drv.count(), len(r.delivered), sessionStates(r.h))
	})
}

func (r *tkRig) held(member string) bool {
	_, held := r.h.Held(SessionKey{Group: runGroup1, Member: member})
	return held
}

func (r *tkRig) open(member string) bool {
	return ptyState(r.h).lookup(aiSlot{group: runGroup1, member: member}) != nil
}

// waitReleased waits until the hand-back of member has ended.
func (r *tkRig) waitReleased(member string) {
	r.t.Helper()
	r.waitFor("the hand-back to end", func() bool {
		p := takeoverState(r.h)
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.open[aiSlot{group: runGroup1, member: member}] == nil
	})
}

func (r *tkRig) session(key SessionKey) Session {
	s, _ := r.h.Sessions.Get(key)
	return s
}

func tkCode(t *testing.T, err error, code string) {
	t.Helper()
	if got, ok := errors.AsType[*Error](err); !ok || got.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

func TestTakeoverStartRefusals(t *testing.T) {
	r := newTkRig(t)
	_, err := Surface["takeover.start"](t.Context(), r.h, r.owner, "o", json.RawMessage(`{`))
	tkCode(t, err, "bad_request")
	tkCode(t, r.startFrom(newTkOwner("relay"), runAI1, ""), "relay")
	tkCode(t, r.start("NoSuchAIAAAAAAAAAAAAAA", ""), "not_found")

	takeoverState(r.h).claim(r.h, aiSlot{group: runGroup1, member: runAI1}, &takeover{}, true)
	tkCode(t, r.start(runAI1, ""), "busy")
	takeoverState(r.h).release(r.h, aiSlot{group: runGroup1, member: runAI1}, &takeover{})
	tkCode(t, r.start(runAI1, ""), "busy")
	takeoverState(r.h).open = map[aiSlot]*takeover{}

	cases := []struct {
		member, task, state, code string
	}{
		{runAI1, tkOther, "active", "not_found"},
		{runAI1, tkTask, "queued", "conflict"},
		{runAI1, tkTask, "done", "conflict"},
		{runAI2, tkTask, "active", "conflict"},
		{runAI2, tkTask, "paused", "conflict"},
	}
	for i, c := range cases {
		r.putTask(tkTask, c.state, runAI1, runAI2, 1)
		tkCode(t, r.start(c.member, c.task), c.code)
		if r.held(c.member) {
			t.Fatalf("case %d holds %s after a refusal", i, c.member)
		}
	}
	if len(r.sent("task", "")) != 0 || len(r.opened) != 0 {
		t.Fatalf("a refused start sent %v or opened %v", r.sent("task", ""), r.opened)
	}
}

// A working AI without an unfinished task is taken over in its group session:
// no task is created, and nothing is read from its folder (flows/takeover.md A).
func TestTakeoverOfAWorkingAIWithoutATaskOpensItsGroupSession(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "queued", runAI1, "", 1)
	if err := r.start(runAI1, ""); err != nil {
		t.Fatal(err)
	}
	key := SessionKey{Group: runGroup1, Member: runAI1}
	s := r.session(key)
	if s.Kind != GroupSession || s.Folder != r.folder || s.State != Takeover || s.CLISession != "new-session" || len(r.h.Sessions.All()) != 3 {
		t.Fatalf("session = %+v of %s", s, sessionStates(r.h))
	}
	l := r.lastOpened()
	if l.Folder != r.folder || l.Session != "" || l.Level != 3 || l.RepoConfig || !slices.Contains(l.Shim, "--token") {
		t.Fatalf("interactive launch = %+v", l)
	}
	if c, ok := r.h.Sessions.Caller(tokenFromLaunch(l)); !ok || !c.Interactive {
		t.Fatalf("interactive token = %+v %v", c, ok)
	}
	if !r.held(runAI1) || r.held(runAI2) || len(r.sent("member", "status")) == 0 {
		t.Fatal("the takeover does not hold its AI alone, or reported no member status")
	}

	replay := json.RawMessage(r.request(runAI1, ""))
	if _, err := r.h.ReplayTakeover(t.Context(), r.owner, "owner-dev", replay); err != nil {
		t.Fatalf("replay on the open terminal = %v", err)
	}
	_, err := r.h.ReplayTakeover(t.Context(), newTkOwner("direct"), "owner-dev", replay)
	tkCode(t, err, "conflict")
	_, err = r.h.ReplayTakeover(t.Context(), r.owner, "owner-dev", json.RawMessage(strings.Replace(string(replay), "Nn"+runAI1[2:], "Oo"+runAI1[2:], 1)))
	tkCode(t, err, "conflict")

	r.end(runAI1)
	r.waitReleased(runAI1)
	posts := r.sent("post", "handback")
	if len(posts) != 1 || posts[0]["thread"] != nil || posts[0]["text"] != "" || posts[0]["as"] != runAI1 || !jsonEqual(posts[0]["body"], map[string]any{}) {
		t.Fatalf("hand-back of the group session = %v", posts)
	}
	if s := r.session(key); s.State != Waiting || len(r.sent("task", "")) != 0 || r.drv.count() != 1 {
		t.Fatalf("after the hand-back: %+v, task commands %v, turns %d", s, r.sent("task", ""), r.drv.count())
	}
	if _, ok := r.h.Sessions.Caller(tokenFromLaunch(l)); ok {
		t.Fatal("the interactive token outlived the terminal")
	}
	_, err = r.h.ReplayTakeover(t.Context(), r.owner, "owner-dev", replay)
	tkCode(t, err, "conflict")
	r.waitFor("delivery after the hand-back", func() bool { return len(r.delivered) > 0 })
}

// The hand-back is the AI's own account: the node adds no facts to it.
func TestTakeoverHandBackPostsTheAIsAccount(t *testing.T) {
	r := newTkRig(t)
	r.drv.makeProc = nil
	r.putTask(tkTask, "active", runAI1, "", 1)
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}
	ctx := t.Context()

	term := newPTYTestTerminal("pty:" + runGroup1 + ":" + runAI1)
	go r.h.ServeTerminal(term)
	term.waitContains(t, "WD:"+filepath.Base(r.folder))
	term.inW.Write([]byte("quit\r"))
	term.waitContains(t, "BYE-NOW")

	r.waitFor("the hand-back turn", func() bool { return r.drv.count() == 1 })
	proc := r.drv.lastProc()
	if text := <-proc.turnCh; text != "replaced-default" || r.session(key).State != Working || r.session(key).Folder != r.folder {
		t.Fatalf("hand-back turn %q in %+v", text, r.session(key))
	}
	caller, _ := r.h.Sessions.Caller(tokenFromLaunch(r.drv.lastLaunch()))
	if _, err := Tools["post"].Call(ctx, r.h, caller, json.RawMessage(`{"text":"Owner asked for a.","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}
	proc.emit(Event{Kind: TurnEnded, Status: "completed"})
	r.waitReleased(runAI1)
	posts := r.sent("post", "handback")
	if len(posts) != 1 || posts[0]["text"] != "Owner asked for a." || posts[0]["thread"] != key.Task || !jsonEqual(posts[0]["body"], map[string]any{}) {
		t.Fatalf("hand-back posts = %v", posts)
	}
	if s := r.session(key); s.State != Waiting || len(r.sent("task", "")) != 0 {
		t.Fatalf("after hand-back: %+v, task commands %v", s, r.sent("task", ""))
	}
}

func jsonEqual(a, b any) bool {
	return marshalWire(a) == marshalWire(b)
}

func TestTakeoverFallbackPostsAndFailedHandBackTurns(t *testing.T) {
	r := newTkRig(t)
	if err := r.start(tkSec, ""); err != nil {
		t.Fatal(err)
	}
	secKey := SessionKey{Group: runGroup1, Member: tkSec}
	if s := r.session(secKey); s.Kind != GroupSession || s.Folder != r.folder || r.lastOpened().RepoConfig || len(r.sent("task", "")) != 0 {
		t.Fatalf("secretary takeover = %+v, launch %+v", s, r.lastOpened())
	}
	r.end(tkSec)
	r.waitReleased(tkSec)
	posts := r.sent("post", "handback")
	if len(posts) != 1 || posts[0]["thread"] != nil || posts[0]["text"] != "" || posts[0]["as"] != tkSec || !jsonEqual(posts[0]["body"], map[string]any{}) {
		t.Fatalf("secretary fallback = %v", posts)
	}
	if s := r.session(secKey); s.State != Waiting {
		t.Fatalf("secretary after hand-back = %+v", s)
	}

	r.putTask(tkTask, "active", runAI1, runAI2, 1)
	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}
	compose := Compose
	Compose = func(*Host, Session, TurnKind, []json.RawMessage) (string, Turn, error) {
		return "", Turn{}, errors.New("no compose")
	}
	statuses := len(r.sent("member", "status"))
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	r.end(runAI1)
	r.waitReleased(runAI1)
	if s := r.session(taskKey); s.State != Dormant || len(r.sent("member", "status")) < statuses+2 || len(r.sent("post", "handback")) != 2 {
		t.Fatalf("after a failed compose: %+v", s)
	}

	Compose = compose
	r.drv.startErr = errors.New("no start")
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	r.end(runAI1)
	r.waitReleased(runAI1)
	if s := r.session(taskKey); s.State != Dormant || len(r.sent("post", "handback")) != 3 {
		t.Fatalf("after a failed start: %+v", s)
	}
}

// A CLI whose takeover is resume cannot start a session in a terminal: a
// group session that has never run gets one structured turn first, which says
// only that the owner is about to work with the AI, and the terminal resumes
// the session that turn made (flows/takeover.md A). A session that has run
// gets none.
func TestTakeoverStartsANeverRunSessionOfAResumeCLIInStructuredMode(t *testing.T) {
	r := newTkRig(t)
	var kinds []TurnKind
	Compose = func(_ *Host, s Session, kind TurnKind, _ []json.RawMessage) (string, Turn, error) {
		kinds = append(kinds, kind)
		return "first", Turn{Kind: kind, Thread: s.Task}, nil
	}
	if err := r.start(runAI2, ""); err != nil {
		t.Fatal(err)
	}
	key := SessionKey{Group: runGroup1, Member: runAI2}
	if l := r.lastOpened(); !slices.Equal(kinds, []TurnKind{TakeoverTurn}) || l.Session != "thread-1" || r.drv.count() != 1 || r.session(key).State != Takeover {
		t.Fatalf("turns %v, terminal %+v, session %+v", kinds, l, r.session(key))
	}
	r.end(runAI2)
	r.waitReleased(runAI2)
	kinds = nil
	if err := r.start(runAI2, ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(kinds, nil) || r.drv.count() != 2 || r.lastOpened().Session != "thread-1" {
		t.Fatalf("a session that has run got turns %v, %d starts", kinds, r.drv.count())
	}
	r.end(runAI2)
	r.waitReleased(runAI2)

	// A first turn that cannot be composed opens no terminal.
	r.h.Sessions.Update(key, func(s *Session) { s.CLISession = "" })
	Compose = func(*Host, Session, TurnKind, []json.RawMessage) (string, Turn, error) {
		return "", Turn{}, errors.New("no compose")
	}
	opened := len(r.opened)
	if err := r.start(runAI2, ""); err == nil || len(r.opened) != opened || r.held(runAI2) {
		t.Fatalf("start = %v, terminals %d", err, len(r.opened)-opened)
	}
}

// The terminal opens only once that first turn has ended.
func TestTakeoverWaitsForTheFirstTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTkRig(t)
		r.drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			p.autoReadyID = "thread-1"
			return p, nil
		}
		opened := make(chan Launch, 1)
		Drivers["codex"] = tkDriver{r.drv, func(l Launch) ([]string, string, error) {
			opened <- l
			return nil, "", errors.New("no terminal")
		}}
		done := make(chan error, 1)
		go func() { done <- r.start(runAI2, "") }()
		synctest.Wait()
		if len(opened) != 0 || len(done) != 0 {
			t.Fatal("the terminal opened during the first turn")
		}
		r.drv.lastProc().emit(Event{Kind: TurnEnded, Status: "completed"})
		tkCode(t, <-done, "cli")
		if l := <-opened; l.Session != "thread-1" {
			t.Fatalf("terminal after the first turn = %+v", l)
		}
	})
}

// A folder that is gone when the terminal closes fails the hand-back as a
// crash: the task fails and the session is dormant (contracts/local.md
// section 7).
func TestTakeoverHandBackInAFolderThatIsGoneFailsTheTask(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "active", runAI1, "", 1)
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(r.folder); err != nil {
		t.Fatal(err)
	}
	r.end(runAI1)
	r.waitReleased(runAI1)
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}
	fails := r.sent("task", "fail")
	if s := r.session(key); s.State != Dormant || r.drv.count() != 0 || len(fails) != 1 || fails[0]["reason"] != "crash" || !strings.Contains(fmt.Sprint(fails[0]["detail"]), r.folder) {
		t.Fatalf("after the hand-back: %+v, %d starts, task commands %v", s, r.drv.count(), r.sent("task", ""))
	}
}

func TestTakeoverStartFailures(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "active", runAI1, runAI2, 1)
	r.ptyErr = errors.New("cannot resume")
	err := r.start(runAI1, tkTask)
	tkCode(t, err, "cli")
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}
	if s := r.session(key); s.State != Dormant || r.held(runAI1) || len(r.sent("task", "cancel")) != 0 {
		t.Fatalf("after a failed start: %+v", s)
	}
	if state := r.lastState(runAI1); state != "dormant" {
		t.Fatalf("member state after a failed start = %v, want dormant", state)
	}

	// The folder is gone: the answer names it, and no terminal is tried.
	r.ptyErr = nil
	opened := len(r.opened)
	if err := os.Remove(r.folder); err != nil {
		t.Fatal(err)
	}
	err = r.start(runAI1, tkTask)
	tkCode(t, err, "cli")
	if !strings.Contains(err.Error(), r.folder) || len(r.opened) != opened || r.held(runAI1) || r.session(key).State != Dormant {
		t.Fatalf("a takeover in a folder that is gone = %v, session %+v", err, r.session(key))
	}

	// An AI without a group session on this node is not taken over.
	r.h.Sessions.End(SessionKey{Group: runGroup1, Member: runAI2})
	tkCode(t, r.start(runAI2, ""), "not_found")
}

func TestTakeoverRefusedWhileAnotherSessionOfTheAIIsTakenOver(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "active", runAI1, runAI2, 1)
	other := makeRunSession(SessionKey{Group: runGroup1, Member: runAI1, Task: tkOther}, TaskSession, r.folder)
	r.h.Sessions.Create(other)
	r.h.Sessions.Move(other.SessionKey, Dormant, Takeover)
	tkCode(t, r.start(runAI1, tkTask), "busy")
	if r.held(runAI1) {
		t.Fatal("a refused start kept its hold")
	}
}

func TestTakeoverPicksTheTaskSessionUsedLast(t *testing.T) {
	r := newTkRig(t)
	slot := aiSlot{group: runGroup1, member: runAI1}
	if got := lastTask(r.h, slot); got != "" {
		t.Fatalf("lastTask with no session = %q", got)
	}
	r.putTask(tkTask, "active", runAI1, "", 1)
	r.putTask(tkOther, "failed", runAI1, "", 2)
	r.putTask("TkDoneAAAAAAAAAAAAAAAA", "done", runAI1, "", 3)
	for i, id := range []string{tkTask, tkOther, "TkDoneAAAAAAAAAAAAAAAA"} {
		s := makeRunSession(SessionKey{Group: runGroup1, Member: runAI1, Task: id}, TaskSession, r.folder)
		r.h.Sessions.Create(s)
		r.h.Sessions.Update(s.SessionKey, func(s *Session) { s.LastActive = int64([]int{20, 10, 30}[i]) })
	}
	r.h.Sessions.Update(SessionKey{Group: runGroup1, Member: runAI1}, func(s *Session) { s.LastActive = 40 })
	if got := lastTask(r.h, slot); got != tkTask {
		t.Fatalf("lastTask = %q, want %q", got, tkTask)
	}
	r.h.Sessions.Update(SessionKey{Group: runGroup1, Member: runAI1, Task: tkOther}, func(s *Session) { s.LastActive = 25 })
	if got := lastTask(r.h, slot); got != tkOther {
		t.Fatalf("lastTask after use = %q, want %q", got, tkOther)
	}
}

func TestTakeoverNamedTasksOfAssigneeAndReviewer(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "paused", runAI1, runAI2, 1)
	reviewKey := SessionKey{Group: runGroup1, Member: runAI2, Task: tkTask}
	rs := makeRunSession(reviewKey, ReviewSession, r.folder)
	rs.Agent, rs.CLISession = "codex", "thread-r"
	r.h.Sessions.Create(rs)
	if err := r.start(runAI2, tkTask); err != nil {
		t.Fatal(err)
	}
	if s := r.session(reviewKey); s.Kind != ReviewSession || s.State != Takeover || r.lastOpened().Session != "thread-r" {
		t.Fatalf("review takeover = %+v", s)
	}
	tkCode(t, Runner.Turn(t.Context(), r.h, reviewKey, "x", Turn{Kind: ItemsTurn}), "conflict")
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	if s := r.session(SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}); s.Kind != TaskSession || s.State != Takeover || s.Folder != r.folder {
		t.Fatalf("assignee takeover = %+v", s)
	}
	r.end(runAI2)
	r.end(runAI1)
	r.waitReleased(runAI2)
	r.waitReleased(runAI1)
	posts := r.sent("post", "handback")
	if len(posts) != 2 || posts[0]["thread"] != tkTask || posts[1]["thread"] != tkTask {
		t.Fatalf("hand-back posts = %v", posts)
	}
	if err := r.start(runAI1, ""); err != nil || len(r.sent("task", "start")) != 0 || r.session(SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}).State != Takeover {
		t.Fatalf("a takeover naming no task did not take the task session used last: %v", err)
	}
	_, err := Surface["takeover.end"](t.Context(), r.h, r.owner, "o", json.RawMessage(`{`))
	tkCode(t, err, "bad_request")
}

func TestTakeoverInterruptsTheRunningTurnFirst(t *testing.T) {
	r := newTkRig(t)
	r.drv.makeProc = nil
	gKey := SessionKey{Group: runGroup1, Member: runAI1}
	if err := Runner.Turn(t.Context(), r.h, gKey, "busy", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	proc := r.drv.lastProc()
	r.putTask(tkTask, "active", runAI1, "", 1)
	done := make(chan error, 1)
	go func() { done <- r.start(runAI1, tkTask) }()
	r.waitFor("the interrupt", func() bool { return proc.interrupts.Load() > 0 })
	if r.open(runAI1) {
		t.Fatal("the terminal opened before the turn ended")
	}
	proc.emit(Event{Kind: TurnEnded, Status: "interrupted"})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s := r.session(gKey); s.State != Waiting || !r.open(runAI1) {
		t.Fatalf("group session after the interrupt = %+v", s)
	}
}

func TestInterruptTurnClosesACLIThatDoesNotEndIt(t *testing.T) {
	saveRuntimeGlobals(t)
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupBubbleEnv(t, t.TempDir())
		defer shutdown()
		key := SessionKey{Group: runGroup1, Member: runAI1}
		h.Sessions.Create(makeRunSession(key, GroupSession, runFolder("repo")))
		Runner.Turn(t.Context(), h, key, "busy", Turn{Kind: ItemsTurn})
		start := time.Now()
		interruptTurn(t.Context(), h, aiSlot{group: runGroup1, member: runAI1})
		if s, _ := h.Sessions.Get(key); s.State != Dormant || time.Since(start) != stopSeconds*time.Second || drv.lastProc().interrupts.Load() != 1 {
			t.Fatalf("after the interrupt timeout: %+v after %v", s, time.Since(start))
		}
	})
}

func TestTakeoverClosesOnRelayOnALostConnectionAndUnattached(t *testing.T) {
	r := newTkRig(t)
	if err := r.start(tkSec, ""); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		select {
		case r.owner.changed <- struct{}{}:
		case <-time.After(15 * time.Second):
			t.Fatal(timedOut(15*time.Second, "the watch to read a path change", nil))
		}
	}
	if !r.open(tkSec) {
		t.Fatal("a path change that stays direct closed the terminal")
	}
	r.owner.setPath("relay")
	r.waitReleased(tkSec)

	second := newTkOwner("direct")
	if err := r.startFrom(second, tkSec, ""); err != nil {
		t.Fatal(err)
	}
	close(second.done)
	r.waitReleased(tkSec)

	third := newTkOwner("direct")
	if err := r.startFrom(third, tkSec, ""); err != nil {
		t.Fatal(err)
	}
	term := newPTYTestTerminal("pty:" + runGroup1 + ":" + tkSec)
	go r.h.ServeTerminal(term)
	term.waitContains(t, "WD:"+filepath.Base(r.folder))
	closeUnattached(ptyState(r.h).lookup(aiSlot{group: runGroup1, member: tkSec}))
	if !r.open(tkSec) {
		t.Fatal("an attached terminal closed at the attach timeout")
	}
	term.Close()
	r.waitReleased(tkSec)

	takeoverAttach = 50 * time.Millisecond
	if err := r.startFrom(third, tkSec, ""); err != nil {
		t.Fatal(err)
	}
	r.waitReleased(tkSec)
	if posts := r.sent("post", "handback"); len(posts) != 4 {
		t.Fatalf("hand-backs = %d, want 4", len(posts))
	}
}

func TestTakeoverSettleLeavesTheRowForTheNextStart(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "active", runAI1, "", 1)
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}
	r.h.Settle()
	r.h.Settle()
	if s := r.session(key); s.State != Takeover || r.open(runAI1) || len(r.sent("post", "")) != 0 {
		t.Fatalf("after settle: %+v", s)
	}

	p := takeoverState(r.h)
	p.mu.Lock()
	p.open, p.settled = map[aiSlot]*takeover{}, false
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.mu.Unlock()
	rt := stateOf(r.h)
	rt.mu.Lock()
	rt.settling = false
	rt.mu.Unlock()
	busy := r.session(SessionKey{Group: runGroup1, Member: tkSec})
	r.h.Sessions.Move(busy.SessionKey, Dormant, Takeover)
	p.claim(r.h, aiSlot{group: runGroup1, member: tkSec}, &takeover{}, true)
	elsewhere := makeRunSession(SessionKey{Group: runGroup2, Member: runAI2}, GroupSession, "repo")
	r.h.Sessions.Create(elsewhere)
	r.h.Sessions.Move(elsewhere.SessionKey, Dormant, Takeover)
	cleanupGroupState(r.h, deliv(r.h), runGroup1)
	r.waitFor("the saved takeover to hand back", func() bool { return len(r.sent("post", "handback")) == 1 })
	r.waitReleased(runAI1)
	if s := r.session(key); s.State != Waiting || r.session(busy.SessionKey).State != Takeover || r.session(elsewhere.SessionKey).State != Takeover {
		t.Fatalf("after the restart hand-back: %+v", s)
	}
}

func TestTakeoverEndsWithItsTask(t *testing.T) {
	r := newTkRig(t)
	r.putTask(tkTask, "active", runAI1, "", 1)
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: tkTask}
	r.putTask(tkTask, "cancelled", runAI1, "", 1)
	cleanupGroupState(r.h, deliv(r.h), runGroup1)
	r.waitReleased(runAI1)
	if _, ok := r.h.Sessions.Get(key); ok || r.open(runAI1) || len(r.sent("post", "")) != 0 {
		t.Fatal("a cancelled task's takeover kept its session or terminal, or posted")
	}
	if state := r.lastState(runAI1); state != "dormant" {
		t.Fatalf("member state after the task ended = %v, want dormant", state)
	}
}

func TestQueuedCountCountsHeldItems(t *testing.T) {
	r := newTkRig(t)
	d := deliv(r.h)
	d.mu.Lock()
	d.held[aiGroupKey{group: runGroup1, member: runAI1}] = []verifiedItem{{}, {}}
	d.mu.Unlock()
	if r.h.QueuedCount(runGroup1, runAI1) != 2 || r.h.QueuedCount(runGroup1, runAI2) != 0 {
		t.Fatal("QueuedCount does not count the held items of the AI")
	}
}

func TestHandBackTurnFailsWithoutItsLaunchConfig(t *testing.T) {
	h, _, _, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(key, GroupSession, runFolder("repo")))
	h.Sessions.Move(key, Dormant, Takeover)
	seamSQL(t, h.Store, "DROP TABLE perm")
	if err := Runner.Turn(t.Context(), h, key, "x", Turn{Kind: HandbackTurn}); err == nil {
		t.Fatal("a hand-back turn started without its launch configuration")
	}
	if s, _ := h.Sessions.Get(key); s.State != Takeover {
		t.Fatalf("state = %s", s.State)
	}
}

func TestTakeoverIsFreeOnceTheHandBackTurnStarts(t *testing.T) {
	r := newTkRig(t)
	r.drv.makeProc = nil
	key := SessionKey{Group: runGroup1, Member: tkSec}
	if err := r.start(tkSec, ""); err != nil {
		t.Fatal(err)
	}
	r.end(tkSec)
	r.waitFor("the hand-back turn", func() bool { return r.drv.count() == 1 })
	proc := r.drv.lastProc()
	<-proc.turnCh
	if r.held(tkSec) || r.session(key).State != Working {
		t.Fatalf("during the hand-back turn: held %v, state %s", r.held(tkSec), r.session(key).State)
	}
	posting := make(chan struct{})
	r.home.cmdFn = func(p map[string]any) (json.RawMessage, error) {
		if p["kind"] == "handback" {
			<-posting
		}
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	proc.emit(Event{Kind: TurnEnded, Status: "completed"})
	r.waitFor("waiting", func() bool { return r.session(key).State == Waiting })
	if err := r.start(tkSec, ""); err != nil {
		t.Fatalf("a takeover right after the hand-back turn = %v", err)
	}
	close(posting)
	if r.session(key).State != Takeover || !r.held(tkSec) {
		t.Fatal("the second takeover did not take the session")
	}
	r.end(tkSec)
	r.waitFor("the second hand-back turn", func() bool { return r.drv.count() == 2 })
	r.drv.lastProc().emit(Event{Kind: TurnEnded, Status: "completed"})
	r.waitReleased(tkSec)
	if posts := r.sent("post", "handback"); len(posts) != 2 {
		t.Fatalf("hand-back posts = %d, want 2", len(posts))
	}
}

func TestSettleEndsARunningHandBackTurn(t *testing.T) {
	r := newTkRig(t)
	started := make(chan *fakeProc, 1)
	r.drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		started <- p
		return p, nil
	}
	if err := r.start(tkSec, ""); err != nil {
		t.Fatal(err)
	}
	r.end(tkSec)
	waitHandBackTurn(t, started)
	settled := make(chan struct{})
	go func() {
		r.h.Settle()
		close(settled)
	}()
	select {
	case <-settled:
	case <-time.After(15 * time.Second):
		t.Fatal(timedOut(15*time.Second, "Settle to return during a hand-back turn", nil))
	}
	if len(r.delivered) != 0 {
		t.Fatal("a hand-back delivered after Settle")
	}
}

func waitHandBackTurn(t *testing.T, started <-chan *fakeProc) {
	t.Helper()
	timeout := time.After(15 * time.Second)
	select {
	case p := <-started:
		select {
		case <-p.turnCh:
			return
		case <-timeout:
		}
	case <-timeout:
	}
	t.Fatal(timedOut(15*time.Second, "the hand-back turn to start", nil))
}

func TestHandBackOfAHeldAIStartsNoTurn(t *testing.T) {
	for _, hold := range []string{"offline", "rate_limit"} {
		t.Run(hold, func(t *testing.T) {
			r := newTkRig(t)
			Holds["limits"] = holdLimits
			if err := r.start(tkSec, ""); err != nil {
				t.Fatal(err)
			}
			lp := limitsOf(r.h)
			lp.mu.Lock()
			lp.offline = hold == "offline"
			lp.rateUntil["claude"] = r.h.Now().Add(time.Hour).UnixMilli()
			lp.mu.Unlock()
			r.end(tkSec)
			r.waitReleased(tkSec)
			s := r.session(SessionKey{Group: runGroup1, Member: tkSec})
			if s.State != Dormant || r.drv.count() != 0 || len(r.sent("post", "handback")) != 1 {
				t.Fatalf("held hand-back: %+v, CLI starts %d, posts %v", s, r.drv.count(), r.sent("post", "handback"))
			}
			if reason, _ := r.h.Held(s.SessionKey); reason != hold {
				t.Fatalf("hold = %q, want %q", reason, hold)
			}
		})
	}
}

func TestTakeoverHandsBackOnceWhenRowsArriveDuringItsHandBack(t *testing.T) {
	r := newTkRig(t)
	composing, letCompose := make(chan struct{}), make(chan struct{})
	enter := sync.OnceFunc(func() { close(composing) })
	compose := Compose
	Compose = func(h *Host, s Session, kind TurnKind, in []json.RawMessage) (string, Turn, error) {
		if kind == HandbackTurn {
			enter()
			<-letCompose
		}
		return compose(h, s, kind, in)
	}
	if err := r.start(tkSec, ""); err != nil {
		t.Fatal(err)
	}
	r.end(tkSec)
	<-composing
	cleanupGroupState(r.h, deliv(r.h), runGroup1)
	close(letCompose)
	r.waitReleased(tkSec)
	close(r.owner.done)
	takeoverState(r.h).wg.Wait()
	if posts := r.sent("post", "handback"); len(posts) != 1 || r.drv.count() != 1 {
		t.Fatalf("one takeover gave %d hand-back posts and %d turns", len(posts), r.drv.count())
	}
}

// tkGatedOwner delays the watch's Path call that follows the second path
// signal until the path is "relay", so the watch leaves with a signal unread.
type tkGatedOwner struct {
	*tkOwner
	calls atomic.Int32
	relay <-chan struct{}
}

func (o *tkGatedOwner) Path() string {
	if o.calls.Add(1) == 4 {
		<-o.relay
	}
	return o.tkOwner.Path()
}

func TestTakeoverFakeOwnerSetPathNeverBlocksWhenTheWatchLeavesASignalUnread(t *testing.T) {
	r := newTkRig(t)
	relay := make(chan struct{})
	if err := r.startFrom(&tkGatedOwner{tkOwner: r.owner, relay: relay}, tkSec, ""); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		r.owner.changed <- struct{}{}
	}
	sent := make(chan struct{})
	go func() {
		r.owner.setPath("relay")
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(3 * time.Second):
		<-r.owner.changed
		<-sent
		close(relay)
		t.Fatal(timedOut(3*time.Second, "setPath on a full signal channel after the watch left", nil))
	}
	close(relay)
	r.waitReleased(tkSec)
}

func TestTakeoverFakeOwnerSetPathWithFullSignalChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner := newTkOwner("direct")
		owner.changed <- struct{}{}
		sent := make(chan struct{})
		go func() {
			owner.setPath("relay")
			close(sent)
		}()
		synctest.Wait()
		select {
		case <-sent:
		default:
			<-owner.changed
			t.Fatal("setPath blocked on a full signal channel")
		}
		if path := owner.Path(); path != "relay" {
			t.Fatalf("path = %q, want relay", path)
		}
		if got := len(owner.changed); got != 1 {
			t.Fatalf("pending signals = %d, want 1", got)
		}
	})
}

// A takeover that starts after delivery has looked at the holds, and before
// the turn is given, would make the AI miss an item that is acknowledged.
func TestTakeoverStartIsBusyWhileDeliveryGivesATurn(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		opening, leave := make(chan struct{}), make(chan struct{})
		Drivers["claude"] = tkDriver{r.drv, func(Launch) ([]string, string, error) {
			close(opening)
			<-leave
			return nil, "", errors.New("no terminal")
		}}
		req := (&tkRig{}).request(runAI1, runTask1)
		var refused error
		Compose = func(h *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
			answered := make(chan struct{})
			go func() {
				defer close(answered)
				_, refused = Surface["takeover.start"](context.Background(), h, newTkOwner("direct"), "owner-dev", json.RawMessage(req))
			}()
			select {
			case <-opening:
			case <-answered:
			}
			return jumpCompose(h, s, kind, items)
		}
		r.putTask(runTask1, "active", "work", "")
		r.push(proto.Item{Item: 1, Why: "assign", Task: runTask1})
		close(leave)
		synctest.Wait()
		r.wantAcked(1)
		if n := r.drv.count(); n != 1 {
			t.Fatalf("%d CLIs ran: the acknowledged item got no turn", n)
		}
		r.wantTurns(r.drv.lastProc(), "items 1")
		tkCode(t, refused, "busy")
	})
}

// The turn of its own of an interrupted session is given like any other.
func TestTakeoverIsBusyWhileATurnOfItsOwnIsComposed(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "work")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		var claimed bool
		Compose = func(h *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
			claimed = takeoverState(r.h).claim(r.h, aiSlot{group: runGroup1, member: runAI1}, &takeover{}, true)
			return jumpCompose(h, s, kind, items)
		}
		r.end(r.drv.proc(1), "completed")
		if claimed {
			t.Fatal("the AI was taken over while its turn was composed")
		}
		r.wantTurns(task, "items 1", jumpContinueTask)
	})
}

// An AI whose items are only held can be taken over once delivery has looked
// at them.
func TestAIWithHeldItemsCanBeTakenOver(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		r.end(r.work(1, runTask1, "work"), "completed")
		r.putTask(runTask1, "paused", "work", "manual")
		r.push(jumpMention(2, testMsg1, runTask1, false))
		r.wantHeld(2)
		if !takeoverState(r.h).claim(r.h, aiSlot{group: runGroup1, member: runAI1}, &takeover{}, true) {
			t.Fatal("the AI is busy")
		}
	})
}

// A look at an AI that has nothing held and is owed no turn reads no holds:
// a takeover may start before delivery has left the AI.
func TestAIWithNothingHeldCanBeTakenOverWhileDeliveryLooks(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		key := aiGroupKey{group: runGroup1, member: runAI1}
		if _, _, ok := takeEligibleBatch(r.h, deliv(r.h), key, aiInfo{}); ok {
			t.Fatal("a batch was taken from an empty queue")
		}
		if !takeoverState(r.h).claim(r.h, aiSlot(key), &takeover{}, true) {
			t.Fatal("the AI is busy")
		}
	})
}
