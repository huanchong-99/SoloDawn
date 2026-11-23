package host

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectExternalClient(t *testing.T, sockPath, member string) *mcp.ClientSession {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "mcp", "--member", member)
	cmd.Env = append(os.Environ(), testMCPSockEnv+"="+sockPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestExternalToolListMatchesContract(t *testing.T) {
	r := newExternalRig(t, false)
	got := decodeListedToolsByName(t, rawToolsListOverSocket(t, startTestSocket(t, r.h), testAI1))
	want := slices.DeleteFunc(slices.Clone(loadGoldenToolLists(t).External.Tools), func(tl proto.Tool) bool { return tl.Name == "net_status" })
	i := slices.IndexFunc(got, func(tl proto.Tool) bool { return tl.Name == "approve" })
	if i < 0 {
		t.Fatal("approve is not listed")
	}
	want = append(want, got[i])
	slices.SortFunc(want, compareProtoTools)
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("got  %+v\nwant %+v", got[i], want[i])
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%d tools, want %d", len(got), len(want))
	}
}

func TestExternalToolCalls(t *testing.T) {
	r := newExternalRig(t, false)
	cs := connectExternalClient(t, startTestSocket(t, r.h), testAI1)
	call := func(name string, args map[string]any) string {
		return callToolText(t, cs, &mcp.CallToolParams{Name: name, Arguments: args})
	}
	if got := call("next_item", nil); got != `{"none":true}` {
		t.Fatalf("empty next_item = %s", got)
	}
	mustState(t, r.h, "waiting", "")
	r.pushExternalItem(proto.Item{Item: 5, Why: "mention", Msg: testMsg1})
	var ho proto.Handout
	if json.Unmarshal([]byte(call("next_item", nil)), &ho); ho.Run != 5 || ho.Kind != "group" {
		t.Fatalf("hand-out = %+v", ho)
	}
	if got := callToolErrorText(t, cs, &mcp.CallToolParams{Name: "run_checks"}); got != "conflict: run_checks needs a task run" {
		t.Fatalf("run_checks = %s", got)
	}
	if got := callToolErrorText(t, cs, &mcp.CallToolParams{Name: "submit_review", Arguments: map[string]any{"score": 5, "items": []any{}}}); got != "conflict: submit_review needs a review run" {
		t.Fatalf("submit_review = %s", got)
	}
	if got := call("done", map[string]any{"run": 5}); got != `{"ok":true}` {
		t.Fatalf("done = %s", got)
	}
	mustState(t, r.h, "waiting", "")
	if got := callToolErrorText(t, cs, &mcp.CallToolParams{Name: "report_usage", Arguments: map[string]any{"window_pct": 101}}); got[:12] != "bad_request:" {
		t.Fatalf("report_usage 101 = %s", got)
	}
	call("report_usage", map[string]any{"window_pct": 40, "window_resets": 1000})
	call("report_usage", map[string]any{"window_pct": 55})
	if u := memberUsage(r.h, testGroup1, testAI1); *u.WindowPct != 55 || u.WindowResets != nil {
		t.Fatalf("usage = %+v", u)
	}
}

func TestExternalCallerOfOpenRun(t *testing.T) {
	r := newExternalRig(t, false)
	r.pushExternalItem(proto.Item{Item: 5, Why: "mention", Msg: testMsg1})
	r.h.ExternalTake(testGroup1, testAI1)
	c, ok := externalCaller(r.h, testAI1)
	if !ok || !c.External || c.Level != 3 || c.Kind != GroupSession || c.Member != testAI1 {
		t.Fatalf("caller = %+v %v", c, ok)
	}
	if _, ok := externalCaller(r.h, testAI2); ok {
		t.Fatal("a managed AI has an external caller")
	}
	if got := neededKinds(c, func(c Caller) bool { return c.Kind != GroupSession }); !slices.Equal(got, []string{"task", "review"}) {
		t.Fatalf("kinds = %v", got)
	}
}

func TestExternalWait(t *testing.T) {
	r := newExternalRig(t, false)
	h := r.h
	if _, ok, err := h.ExternalWait(t.Context(), testAI2, nil); ok || err != errNotExternal {
		t.Fatalf("managed wait = %v %v", ok, err)
	}
	if err := h.ExternalEnd(testAI2, 1, ""); err != errNotExternal {
		t.Fatalf("managed end = %v", err)
	}
	if _, ok, err := h.ExternalWait(t.Context(), testAI1, new(time.Duration(time.Millisecond))); ok || err != nil {
		t.Fatalf("expired wait = %v %v", ok, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, ok, err := h.ExternalWait(ctx, testAI1, nil); ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v %v", ok, err)
	}
	type result struct {
		ho  proto.Handout
		ok  bool
		err error
	}
	waited := make(chan result)
	wait := func() {
		ho, ok, err := h.ExternalWait(t.Context(), testAI1, nil)
		waited <- result{ho, ok, err}
	}
	go wait()
	r.pushExternalItem(proto.Item{Item: 5, Why: "mention", Msg: testMsg1})
	if got := <-waited; !got.ok || got.ho.Run != 5 {
		t.Fatalf("woken wait = %+v", got)
	}
	if lines, err := h.ExternalWaitLines(t.Context(), testAI1, new(time.Duration(0))); lines != nil || err != nil {
		t.Fatalf("lines during a run = %q %v", lines, err)
	}
	if err := h.ExternalEnd(testAI1, 5, "exit 1"); err != nil {
		t.Fatal(err)
	}
	mustState(t, h, "waiting", "")
	r.pushExternalItem(proto.Item{Item: 6, Why: "mention", Msg: testMsg2})
	if lines, err := h.ExternalWaitLines(t.Context(), testAI1, nil); len(lines) != 1 || !strings.Contains(lines[0], `"run":6`) || err != nil {
		t.Fatalf("lines = %q %v", lines, err)
	}
	go wait()
	r.waitUntil("waiting", func() bool {
		ep := extOf(h)
		ep.mu.Lock()
		defer ep.mu.Unlock()
		return ep.wake[aiGroupKey{group: testGroup1, member: testAI1}] != nil
	})
	leaveGoneMember(h, testGroup1, testAI1)
	if got := <-waited; got.ok || got.err != errNotExternal {
		t.Fatalf("removed wait = %+v", got)
	}
}
