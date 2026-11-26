//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_tools_talk"] = hostToolsTalkSteps
}

type talkFakeHome struct {
	done     chan struct{}
	requests []any
	groupID  string
	taskID   string
	codexID  string
	claudeID string
}

func (f *talkFakeHome) Send(any) error               { return nil }
func (f *talkFakeHome) Path() string                 { return "local" }
func (f *talkFakeHome) PathChanged() <-chan struct{} { return nil }
func (f *talkFakeHome) Done() <-chan struct{}        { return f.done }

func (f *talkFakeHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	id := int64(len(f.requests) + 1)
	req := build(id)
	f.requests = append(f.requests, req)
	switch r := req.(type) {
	case proto.FetchThread:
		return f.threadReply(id, r.Thread), nil
	case proto.FetchKind:
		return f.noteReply(id), nil
	default:
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
}

func (f *talkFakeHome) threadReply(id int64, thread *string) json.RawMessage {
	row := proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m1____________________A", Seq: 10},
		Pos:       10,
		Group:     f.groupID,
		Thread:    thread,
		Main:      false,
		By:        f.codexID,
		Kind:      "text",
		Timestamp: 1_763_600_000_000,
		Text:      "Looks good.",
		Mentions:  []string{},
		Re:        nil,
		Body:      proto.EmptyBody{},
	}
	review := proto.MessageRow[proto.ReviewBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m2____________________A", Seq: 9},
		Pos:       9, Group: f.groupID, Thread: thread, By: f.codexID, Kind: "review", Timestamp: 1_763_600_000_000, Mentions: []string{},
		Body: proto.ReviewBody{Report: turnReport, Round: 1, Score: 86, Pass: 90, Items: []proto.ReviewItem{{Text: "No test"}}},
	}
	raw, _ := json.Marshal(row)
	rawReview, _ := json.Marshal(review)
	resp, _ := json.Marshal(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: id, OK: true},
		Rows:    []json.RawMessage{raw, rawReview},
		More:    false,
	})
	return resp
}

func (f *talkFakeHome) noteReply(id int64) json.RawMessage {
	tid := f.taskID
	row := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: "n1____________________A", Seq: 11},
		Pos:       11,
		Group:     f.groupID,
		Task:      &tid,
		Text:      "Always check path state.",
		By:        f.claudeID,
		Timestamp: 1_763_600_000_000,
	}
	raw, _ := json.Marshal(row)
	resp, _ := json.Marshal(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: id, OK: true},
		Rows:    []json.RawMessage{raw},
		More:    false,
	})
	return resp
}

type talkScenarioState struct {
	h      *host.Host
	home   *talkFakeHome
	caller host.Caller
	out    any
	err    error
}

func hostToolsTalkSteps(sc *godog.ScenarioContext, _ fixtures) {
	cleanup := afterScenario(sc)
	st := &talkScenarioState{}

	sc.Step(`^a connected host session for AI "([^"]+)" in a group with task "T-12"$`, func(aiName string) error {
		return st.setup(cleanup, aiName)
	})
	sc.Step(`^the AI prints "([^"]*)" outside tool calls$`, func(_ string) {
		// Printed stdout outside MCP tool invocations never calls Host.Tools.
	})
	sc.Step(`^the home has received (\d+) requests$`, func(want int) error {
		if len(st.home.requests) != want {
			return fmt.Errorf("got %d requests, want %d", len(st.home.requests), want)
		}
		return nil
	})
	sc.Step(`^the AI calls "([^"]+)" with '([^']*)'$`, func(name, rawArgs string) error {
		tool, ok := host.ToolsFor(st.caller)[name]
		if !ok {
			return fmt.Errorf("tool %q not listed", name)
		}
		st.out, st.err = tool.Call(context.Background(), st.h, st.caller, json.RawMessage(rawArgs))
		return nil
	})
	sc.Step(`^the tool succeeds with an id$`, func() error {
		return st.checkIDResult()
	})
	sc.Step(`^the tool fails with "([^"]*)"$`, func(want string) error {
		if st.err == nil || st.err.Error() != want {
			return fmt.Errorf("got %v, want %q", st.err, want)
		}
		return nil
	})
	sc.Step(`^the home has received 1 signed "post" command as that AI in task "T-12"$`, st.checkSignedPost)
	sc.Step(`^the tool result JSON contains '([^']*)' and '([^']*)'$`, st.checkResultContains)
}

func (st *talkScenarioState) setup(cleanup func(func()), aiName string) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	now := func() time.Time { return time.UnixMilli(1_763_600_120_000) }
	h, _, err := host.New(db, key, now)
	if err != nil {
		return err
	}
	const (
		groupID  = "rZNvy-1jH6Z-BcPqA5U5BQ"
		taskID   = "DrtCn6htSBwmMPrFPbHJHA"
		claudeID = "1PC8WineBrUQ-apCjx7tug"
		codexID  = "LXCZmuGAXkvO-bSrOkuCfw"
	)
	home := &talkFakeHome{done: make(chan struct{}), groupID: groupID, taskID: taskID, codexID: codexID, claudeID: claudeID}
	h.Attach("home-dev", home)
	seedTalkRows(h, groupID, taskID, claudeID, codexID, aiName, key.ID())
	sessKey := host.SessionKey{Group: groupID, Member: claudeID, Task: taskID}
	if err := h.Sessions.Create(host.Session{
		SessionKey: sessKey,
		Kind:       host.TaskSession,
		Folder:     handFolder,
		Agent:      "claude",
		Home:       "home-dev",
		Name:       aiName,
	}); err != nil {
		return err
	}
	h.Sessions.SetTurn(sessKey, host.Turn{Kind: host.ItemsTurn, Thread: taskID})
	tok := h.Sessions.Admit(sessKey, 2, false)
	caller, _ := h.Sessions.Caller(tok)
	st.h, st.home, st.caller = h, home, caller
	return nil
}

func seedTalkRows(h *host.Host, groupID, taskID, claudeID, codexID, aiName, hostDev string) {
	putJSON := func(r, id string, v any) {
		raw, _ := json.Marshal(v)
		h.Rows.Put(groupID, r, id, raw)
	}
	putJSON("member", claudeID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: claudeID, Seq: 1}, Group: groupID, Kind: "ai", User: "u1", Name: aiName, LastSeen: 1_763_600_000_000},
		CLI:       "claude", Host: hostDev, HostName: "workstation", Account: "personal", Allow: []string{}, Approvers: []string{"u1"}, State: "working", Task: taskID, Path: "local",
	})
	putJSON("member", codexID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: codexID, Seq: 2}, Group: groupID, Kind: "ai", User: "u1", Name: "Codex", LastSeen: 1_763_600_000_000},
		CLI:       "codex", Host: hostDev, HostName: "workstation", Account: "personal", Allow: []string{}, Approvers: []string{"u1"}, State: "working", Task: taskID, Path: "local",
	})
	putJSON("task", taskID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 3},
		Group:     groupID, Num: 12, Title: "Show path", Origin: "brief", Brief: "KaiCW9JC8UOG7lKNduDo8Q",
		State: "active", Step: "work", Assignee: claudeID, Reviewer: codexID, Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: 1_763_600_000_000}, Created: 1_763_600_000_000,
	})
}

func (st *talkScenarioState) checkIDResult() error {
	if st.err != nil {
		return fmt.Errorf("unexpected error: %v", st.err)
	}
	raw, _ := json.Marshal(st.out)
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || len(res.ID) != 22 {
		return fmt.Errorf("got %s, want 22-char id", raw)
	}
	return nil
}

func (st *talkScenarioState) checkSignedPost() error {
	cmd, err := st.singleCmd()
	if err != nil {
		return err
	}
	var p proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(cmd.Env.Payload), &p); err != nil {
		return err
	}
	return st.verifyPostTarget(p)
}

func (st *talkScenarioState) singleCmd() (proto.Cmd, error) {
	if len(st.home.requests) != 1 {
		return proto.Cmd{}, fmt.Errorf("got %d requests, want 1", len(st.home.requests))
	}
	cmd, ok := st.home.requests[0].(proto.Cmd)
	if !ok {
		return proto.Cmd{}, fmt.Errorf("got %T, want proto.Cmd", st.home.requests[0])
	}
	known := func(dev string) bool { return dev == st.h.Key.ID() }
	_, err := identity.OpenCommand(cmd.Env, "", known, st.h.Now())
	return cmd, err
}

func (st *talkScenarioState) verifyPostTarget(p proto.Post[proto.EmptyBody]) error {
	if string(p.As) != st.caller.Member || p.Thread == nil || string(*p.Thread) != st.home.taskID {
		return fmt.Errorf("unexpected post payload: %+v", p)
	}
	return nil
}

func (st *talkScenarioState) checkResultContains(sub1, sub2 string) error {
	if st.err != nil {
		return fmt.Errorf("unexpected error: %v", st.err)
	}
	raw, _ := json.Marshal(st.out)
	text := string(raw)
	if !strings.Contains(text, sub1) || !strings.Contains(text, sub2) {
		return fmt.Errorf("got %s, missing %q or %q", text, sub1, sub2)
	}
	return nil
}
