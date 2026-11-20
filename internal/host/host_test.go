package host

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"reflect"
	"slices"
	"testing"

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
	if h.Store != db || h.Key.ID() != key.ID() || h.Now().UnixMilli() != seamStart || h.Sessions == nil {
		t.Fatalf("got %+v", h)
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
