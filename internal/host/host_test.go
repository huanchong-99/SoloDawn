package host

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// A seamHome is a home in memory: it answers every request with answer or
// with err, and keeps what the host sent.
type seamHome struct {
	done   chan struct{}
	answer string
	err    error
	asked  []any
}

func newSeamHome(answer string) *seamHome {
	return &seamHome{done: make(chan struct{}), answer: answer}
}

func (f *seamHome) Send(any) error               { return nil }
func (f *seamHome) Path() string                 { return "local" }
func (f *seamHome) PathChanged() <-chan struct{} { return nil }
func (f *seamHome) Done() <-chan struct{}        { return f.done }
func (f *seamHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.asked = append(f.asked, build(int64(41+len(f.asked))))
	return json.RawMessage(f.answer), f.err
}

func TestErrorTextIsWhatAToolAnswers(t *testing.T) {
	var err error = &Error{Code: "conflict", Message: "the worktree has uncommitted changes"}
	if got := err.Error(); got != "conflict: the worktree has uncommitted changes" {
		t.Fatal(got)
	}
}

func TestHostKeepsWhatItIsGiven(t *testing.T) {
	db := seamStore(t)
	key, clock := seamKey(), seamClock()
	h, saved, err := New(db, key, clock)
	if err != nil || len(saved) != 0 {
		t.Fatalf("got %v and %v", saved, err)
	}
	if h.Store != db || h.Key.ID() != key.ID() || h.Now().UnixMilli() != seamStart || h.Sessions == nil || h.Rows == nil {
		t.Fatalf("got %+v", h)
	}
	// What node sets after New is for the units to use: nothing here reads it.
	if h.Connect != nil || h.Shim != nil {
		t.Fatalf("a new host has a connect hook or a shim: %v", h.Shim)
	}
}

// seamPanic returns what run panics with, "" when it does not.
func seamPanic(run func()) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = recovered.(string)
		}
	}()
	run()
	return ""
}

func TestAKeyIsRegisteredOnce(t *testing.T) {
	table := map[string]int{}
	Register(table, "post", 1)
	Register(table, "ask", 2)
	if got := seamPanic(func() { Register(table, "post", 3) }); got != "host: post is registered twice" {
		t.Fatalf("a second registration of a key panicked with %q", got)
	}
	if want := map[string]int{"post": 1, "ask": 2}; !maps.Equal(table, want) {
		t.Fatalf("got %v, want %v", table, want)
	}
}

type seamRuntime struct{ Runtime }

func TestAHookIsProvidedOnce(t *testing.T) {
	var deliver func(h *Host, group, member string)
	var runner Runtime
	delivered := 0
	Provide(&deliver, func(*Host, string, string) { delivered++ })
	Provide(&runner, Runtime(seamRuntime{}))
	deliver(nil, "g1", "m7")
	if delivered != 1 || runner != (seamRuntime{}) {
		t.Fatalf("the hooks hold %v after %d calls, not what was provided", runner, delivered)
	}
	for _, second := range []func(){
		func() { Provide(&deliver, func(*Host, string, string) { delivered += 10 }) },
		func() { Provide(&runner, Runtime(nil)) },
	} {
		if got := seamPanic(second); got != "host: a hook is provided twice" {
			t.Fatalf("a second provider panicked with %q", got)
		}
	}
	deliver(nil, "g1", "m7")
	if delivered != 2 {
		t.Fatal("a second provider replaced the first")
	}
}

func TestPartIsOnePerTypeAndHost(t *testing.T) {
	type calls struct{ open int }
	type approvals struct{ open int }
	h, _ := seamHost(t, seamStore(t))
	other, _ := seamHost(t, seamStore(t))
	Part[calls](h).open = 3
	if Part[calls](h).open != 3 || Part[calls](h) != Part[calls](h) {
		t.Fatal("a part does not keep what a unit put in it")
	}
	if Part[approvals](h).open != 0 || Part[calls](other).open != 0 {
		t.Fatal("a part is shared between types or between hosts")
	}
}

func TestHomeIsTheOpenConnectionOfThatDevice(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	if home, ok := h.Home("home-device"); ok || home != nil {
		t.Fatalf("got %v before any connection", home)
	}
	first, second := newSeamHome(""), newSeamHome("")
	h.Attach("home-device", first)
	h.Attach("other-home", second)
	if home, ok := h.Home("home-device"); !ok || home != first {
		t.Fatalf("got %v, want the first connection", home)
	}
	replacement := newSeamHome("")
	h.Attach("home-device", replacement)
	close(first.done)
	if home, ok := h.Home("home-device"); !ok || home != replacement {
		t.Fatalf("got %v, want the connection that replaced the first", home)
	}
	close(replacement.done)
	if home, ok := h.Home("home-device"); ok || home != nil {
		t.Fatalf("got %v for a connection that has ended", home)
	}
	if home, ok := h.Home("other-home"); !ok || home != second {
		t.Fatalf("got %v, want the connection of the other home", home)
	}
}

func TestAttachTellsTheUnitsInTheOrderOfTheirNames(t *testing.T) {
	before := Attached
	t.Cleanup(func() { Attached = before })
	h, _ := seamHost(t, seamStore(t))
	home := newSeamHome("")
	var told []string
	tell := func(name string) func(*Host, string) {
		return func(got *Host, device string) {
			// The connection is there for whoever is told of it.
			if attached, ok := got.Home(device); got != h || !ok || attached != home {
				t.Errorf("%s was told of %s before it was attached", name, device)
			}
			told = append(told, name+" "+device)
		}
	}
	Attached = map[string]func(*Host, string){"sync": tell("sync"), "delivery": tell("delivery"), "join": tell("join")}
	h.Attach("home-device", home)
	if want := []string{"delivery home-device", "join home-device", "sync home-device"}; !slices.Equal(told, want) {
		t.Fatalf("got %v, want %v", told, want)
	}
}

func TestCommandIsSignedSentAndAnswered(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	const payload = `{"t":"post","n":"AAAAAAAAAAAAAAAAAAAAAA","ts":1763600000000,"g":"AAAAAAAAAAAAAAAAAAAAAQ","as":"m7","text":"<done> & pushed"}`
	unreachable := "home_unreachable: the home of that group is not connected"
	if answer, err := h.Command(t.Context(), "home-device", payload); answer != nil || err == nil || err.Error() != unreachable {
		t.Fatalf("got %s and %v without a connection", answer, err)
	}

	home := newSeamHome(`{"re":41,"ok":true,"num":14}`)
	h.Attach("home-device", home)
	answer, err := h.Command(t.Context(), "home-device", payload)
	if err != nil || string(answer) != home.answer {
		t.Fatalf("got %s and %v, want the home's answer", answer, err)
	}
	want := proto.Cmd{Request: proto.Request{Type: "cmd", ID: 41}, Env: seamKey().Sign(payload)}
	if len(home.asked) != 1 || !reflect.DeepEqual(home.asked[0], want) {
		t.Fatalf("sent %+v, want %+v", home.asked, want)
	}
	// The home accepts it as a command of this node: the payload is the text
	// that was signed, byte for byte.
	env := home.asked[0].(proto.Cmd).Env
	if _, err := identity.OpenCommand(env, "", func(device string) bool { return device == seamKey().ID() }, h.Now()); err != nil || env.Payload != payload {
		t.Fatalf("the envelope does not open: %v", err)
	}

	home.answer = `{"re":42,"err":{"code":"forbidden","msg":"only the secretary may post a brief"}}`
	answer, err = h.Command(t.Context(), "home-device", payload)
	if answer != nil || err == nil || err.Error() != "forbidden: only the secretary may post a brief" {
		t.Fatalf("got %s and %v, want the home's error", answer, err)
	}
	seamCode(t, err, "forbidden")

	home.answer, home.err = `{"re":43,"ok":true}`, io.EOF
	if answer, err := h.Command(t.Context(), "home-device", payload); answer != nil || err == nil || err.Error() != unreachable {
		t.Fatalf("got %s and %v when the connection closed before the answer", answer, err)
	}
	if len(home.asked) != 3 {
		t.Fatalf("%d requests were sent, want 3", len(home.asked))
	}
}

func TestCommandThatDoesNotFitACtlMessageIsNotSent(t *testing.T) {
	// A frame is counted as a connection encodes it: without HTML escaping,
	// without a line end.
	if got := frameSize(proto.Request{Type: "<&>", ID: 7}); got != len(`{"t":"<&>","id":7}`) {
		t.Fatalf("got %d bytes", got)
	}
	h, _ := seamHost(t, seamStore(t))
	home := newSeamHome(`{"re":41,"ok":true}`)
	h.Attach("home-device", home)
	// The id of a request is not known before it is sent, so the longest counts.
	empty := frameSize(proto.Cmd{Request: proto.Request{Type: "cmd", ID: math.MaxInt64}, Env: seamKey().Sign("")})
	fits := strings.Repeat("a", 61_440-empty)
	if _, err := h.Command(t.Context(), "home-device", fits); err != nil || len(home.asked) != 1 {
		t.Fatalf("a command of exactly 60 KiB as a frame: %v, %d sent", err, len(home.asked))
	}
	answer, err := h.Command(t.Context(), "home-device", fits+"a")
	if answer != nil || err == nil || err.Error() != "bad_request: the command takes 61441 bytes as a ctl message, and the limit is 61440" {
		t.Fatalf("got %s and %v", answer, err)
	}
	if len(home.asked) != 1 {
		t.Fatal("a command that does not fit was sent")
	}
}

// seamSoon fails the test when run has not returned within ten seconds.
func seamSoon(t *testing.T, what string, run func()) {
	t.Helper()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		run()
	}()
	select {
	case <-returned:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s does not return", what)
	}
}

func TestAnItemGoesToTheCallThatWaitsForIt(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	item := json.RawMessage(`{"env":{"p":"{}","k":"home-device","s":""}}`)
	if waited, err := h.Hand("call-1", item); waited || err != nil {
		t.Fatalf("got %v and %v when nothing waits", waited, err)
	}
	answer, done := h.Await("call-1", nil)
	if waited, _ := h.Hand("call-2", item); waited || len(answer) != 0 {
		t.Fatal("an item went to a call that waits for another message")
	}
	if waited, err := h.Hand("call-1", item); !waited || err != nil {
		t.Fatalf("got %v and %v, want the item handed over", waited, err)
	}
	// The wait has its item: a second one takes its turn in the queue.
	if waited, err := h.Hand("call-1", item); waited || err != nil {
		t.Fatalf("got %v and %v for a second item", waited, err)
	}
	done()
	if got := <-answer; string(got) != string(item) || len(answer) != 0 {
		t.Fatalf("got %s, want the item, once", got)
	}

	// A decision whose cause its check refuses is not handed over, and the
	// call goes on waiting.
	refusal := errors.New("the cause is not signed by an approver")
	var checked []string
	decision, _ := h.Await("card-9", func(got json.RawMessage) error {
		checked = append(checked, string(got))
		if len(checked) == 1 {
			return refusal
		}
		return nil
	})
	if waited, err := h.Hand("card-9", item); !waited || err != refusal || len(decision) != 0 {
		t.Fatalf("got %v and %v, want the item refused by the check", waited, err)
	}
	if waited, err := h.Hand("card-9", item); !waited || err != nil || len(decision) != 1 {
		t.Fatalf("got %v and %v, want the item handed over after its check", waited, err)
	}
	if want := []string{string(item), string(item)}; !slices.Equal(checked, want) {
		t.Fatalf("the check saw %v, want %v", checked, want)
	}

	// A second wait for an id takes the place of the first, whose end then
	// ends nothing.
	first, doneFirst := h.Await("card-3", nil)
	second, _ := h.Await("card-3", nil)
	doneFirst()
	if waited, _ := h.Hand("card-3", item); !waited || len(first) != 0 || len(second) != 1 {
		t.Fatal("the item did not go to the wait that took the place of the first")
	}
}

// Nothing that a waiting call does, or fails to do, stops Hand, the end of a
// wait, or the waits of others.
func TestHandNeverWaitsForTheReceiver(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	item := json.RawMessage(`{"env":{}}`)
	// Nobody reads the channel: the 45 seconds of an ask run out just as the
	// reply arrives.
	unread, done := h.Await("call-1", nil)
	seamSoon(t, "Hand, for a receiver that does not read,", func() { h.Hand("call-1", item) })
	seamSoon(t, "the end of a wait whose item nobody read", done)
	if len(unread) != 1 {
		t.Fatal("the item that arrived before the end of the wait is lost")
	}

	// A check that takes its time holds no lock: the wait can end meanwhile,
	// and the item then takes its turn in the queue.
	entered, proceed := make(chan struct{}), make(chan struct{})
	slow, doneSlow := h.Await("card-9", func(json.RawMessage) error {
		close(entered)
		<-proceed
		return nil
	})
	handed := make(chan bool, 1)
	go func() {
		waited, _ := h.Hand("card-9", item)
		handed <- waited
	}()
	<-entered
	seamSoon(t, "the end of a wait whose item is being checked", doneSlow)
	seamSoon(t, "another wait, while an item is being checked,", func() {
		other, _ := h.Await("call-2", nil)
		if waited, err := h.Hand("call-2", item); !waited || err != nil || len(other) != 1 {
			t.Errorf("got %v and %v", waited, err)
		}
	})
	close(proceed)
	if <-handed || len(slow) != 0 {
		t.Fatal("an item was handed to a wait that had ended")
	}
}

func TestHeldAsksEveryHold(t *testing.T) {
	before := Holds
	t.Cleanup(func() { Holds = before })
	h, _ := seamHost(t, seamStore(t))
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
	if reason, held := h.Held(seamTask); held || reason != "" {
		t.Fatalf("got %q and %v when no unit holds anything", reason, held)
	}
	var asked []string
	hold := func(name, reason string, keys ...SessionKey) func(*Host, SessionKey) (string, bool) {
		return func(got *Host, key SessionKey) (string, bool) {
			asked = append(asked, name)
			return reason, got == h && slices.Contains(keys, key)
		}
	}
	Holds = map[string]func(*Host, SessionKey) (string, bool){
		"pause":  hold("pause", "the group is paused", seamTask, seamGroup),
		"limits": hold("limits", "the day cap is reached", seamTask),
		"never":  hold("never", "nothing"),
	}
	if reason, held := h.Held(seamTask); !held || reason != "the day cap is reached" {
		t.Fatalf("got %q and %v, want the first hold in the order of the names", reason, held)
	}
	if reason, held := h.Held(seamGroup); !held || reason != "the group is paused" {
		t.Fatalf("got %q and %v, want the hold of the one unit that holds", reason, held)
	}
	// A reason without a hold is no hold.
	if reason, held := h.Held(seamReview); held || reason != "" {
		t.Fatalf("got %q and %v for a session nothing holds", reason, held)
	}
	if want := []string{"limits", "limits", "never", "pause", "limits", "never", "pause"}; !slices.Equal(asked, want) {
		t.Fatalf("asked %v, want %v", asked, want)
	}
}

func TestRowsOfAGroup(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	if _, ok := h.Rows.Get("g1", "task", "t12"); ok || len(h.Rows.Of("g1", "task")) != 0 {
		t.Fatal("a new host holds rows")
	}
	h.Rows.Put("g1", "task", "t9", json.RawMessage(`{"r":"task","id":"t9","seq":4}`))
	h.Rows.Put("g1", "task", "t12", json.RawMessage(`{"r":"task","id":"t12","seq":5}`))
	h.Rows.Put("g1", "member", "t12", json.RawMessage(`{"r":"member","id":"t12","seq":6}`))
	h.Rows.Put("g2", "task", "t12", json.RawMessage(`{"r":"task","id":"t12","seq":2}`))
	h.Rows.Put("g1", "task", "t12", json.RawMessage(`{"r":"task","id":"t12","seq":7}`))
	if row, ok := h.Rows.Get("g1", "task", "t12"); !ok || string(row) != `{"r":"task","id":"t12","seq":7}` {
		t.Fatalf("got %s, want the row put last", row)
	}
	texts := func(rows []json.RawMessage) (out []string) {
		for _, row := range rows {
			out = append(out, string(row))
		}
		return out
	}
	if got, want := texts(h.Rows.Of("g1", "task")), []string{`{"r":"task","id":"t12","seq":7}`, `{"r":"task","id":"t9","seq":4}`}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want the task rows of g1 in the order of their ids: %v", got, want)
	}
	h.Rows.Reset("g1")
	if _, ok := h.Rows.Get("g1", "member", "t12"); ok || len(h.Rows.Of("g1", "task")) != 0 {
		t.Fatal("a reset left rows of the group")
	}
	if got := texts(h.Rows.Of("g2", "task")); !slices.Equal(got, []string{`{"r":"task","id":"t12","seq":2}`}) {
		t.Fatalf("a reset of one group changed another: %v", got)
	}
}

func TestReportTellsEveryListener(t *testing.T) {
	before := Listeners
	t.Cleanup(func() { Listeners = before })
	h, _ := seamHost(t, seamStore(t))
	type heard struct {
		name string
		h    *Host
		key  SessionKey
		e    Event
	}
	var got []heard
	listen := func(name string) func(*Host, SessionKey, Event) {
		return func(h *Host, key SessionKey, e Event) { got = append(got, heard{name, h, key, e}) }
	}
	Listeners = map[string]func(*Host, SessionKey, Event){"runtime": listen("runtime"), "approvals": listen("approvals")}
	asked := Event{Kind: Asked, Ask: &Ask{ID: "7", Tool: "commandExecution", Category: "command", Command: "git push"}}
	h.Report(seamTask, asked)
	h.Report(seamGroup, Event{Kind: TurnBegan})
	want := []heard{{"approvals", h, seamTask, asked}, {"runtime", h, seamTask, asked},
		{"approvals", h, seamGroup, Event{Kind: TurnBegan}}, {"runtime", h, seamGroup, Event{Kind: TurnBegan}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPushReachesTheHandlerOfItsKind(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	type push struct {
		h     *Host
		home  string
		frame string
	}
	var got []push
	Pushes["seam.push"] = func(h *Host, home string, frame json.RawMessage) {
		got = append(got, push{h, home, string(frame)})
	}
	t.Cleanup(func() { delete(Pushes, "seam.push") })
	const frame = `{"t":"seam.push","g":"g1","items":[]}`
	// Dropped: no JSON object, no t, a t that is no text, a kind nobody handles.
	for _, dropped := range []string{`[]`, `{"g":"g1"}`, `{"t":7}`, `{"t":"seam.unknown"}`, ``} {
		h.Push("home-device", json.RawMessage(dropped))
	}
	if len(got) != 0 {
		t.Fatalf("a frame that is to be dropped was handled: %+v", got)
	}
	h.Push("home-device", json.RawMessage(frame))
	if want := []push{{h, "home-device", frame}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestToolsAreListedByCaller(t *testing.T) {
	before := maps.Clone(Tools)
	t.Cleanup(func() { Tools = before })
	Tools = map[string]Tool{
		"seam_post":   {Description: "for every session"},
		"seam_checks": {Description: "for a task session above level 1", For: func(c Caller) bool { return c.Kind == TaskSession && c.Level > 1 }},
		"seam_brief":  {Description: "for the secretary", For: func(c Caller) bool { return c.Secretary }},
	}
	registered := maps.Clone(Tools)
	cases := []struct {
		caller Caller
		want   []string
	}{
		{Caller{Session: Session{Kind: TaskSession}, Level: 2}, []string{"seam_checks", "seam_post"}},
		{Caller{Session: Session{Kind: TaskSession}, Level: 1}, []string{"seam_post"}},
		{Caller{Session: Session{Kind: ReviewSession}, Level: 2}, []string{"seam_post"}},
		{Caller{Session: Session{Kind: GroupSession, Secretary: true}, Level: 1}, []string{"seam_brief", "seam_post"}},
	}
	for _, c := range cases {
		listed := ToolsFor(c.caller)
		if got := slices.Sorted(maps.Keys(listed)); !slices.Equal(got, c.want) {
			t.Errorf("%+v is listed %v, want %v", c.caller, got, c.want)
		}
		if listed["seam_post"].Description != "for every session" {
			t.Errorf("a listed tool lost its description: %+v", listed["seam_post"])
		}
	}
	if len(Tools) != len(registered) {
		t.Fatalf("listing changed the table: %d tools, want %d", len(Tools), len(registered))
	}
}
