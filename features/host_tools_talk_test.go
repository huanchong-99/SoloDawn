//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	raw, _ := json.Marshal(row)
	resp, _ := json.Marshal(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: id, OK: true},
		Rows:    []json.RawMessage{raw},
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
	db     *store.Store
	h      *host.Host
	home   *talkFakeHome
	caller host.Caller
	out    any
	err    error
}

func hostToolsTalkSteps(sc *godog.ScenarioContext, f fixtures) {
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
	sc.Step(`^the tool fails with code "([^"]*)"$`, func(want string) error {
		e, ok := errors.AsType[*host.Error](st.err)
		if !ok || e.Code != want {
			return fmt.Errorf("got %v, want code %q", st.err, want)
		}
		return nil
	})
	sc.Step(`^the home has received 1 signed "post" command as that AI in task "T-12"$`, st.checkSignedPost)
	sc.Step(`^the task session enters a hand-back turn after committing "([^"]+)" and leaving "([^"]+)" untracked$`, func(committed, untracked string) error {
		return st.enterHandbackWithRepo(f.t, committed, untracked)
	})
	sc.Step(`^the home received a "handback" post listing files "([^"]+)" and (\d+) commit$`, st.checkHandbackPost)
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
		Folder:     "wt/12-show-path",
		Agent:      "claude",
		Home:       "home-dev",
		Name:       aiName,
	}); err != nil {
		return err
	}
	h.Sessions.SetTurn(sessKey, host.Turn{Kind: host.ItemsTurn, Thread: taskID})
	tok := h.Sessions.Admit(sessKey, 2, false)
	caller, _ := h.Sessions.Caller(tok)
	st.db, st.h, st.home, st.caller = db, h, home, caller
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
		Group:     groupID, Num: 12, Title: "Show path", Slug: "show-path", Origin: "brief", Brief: "KaiCW9JC8UOG7lKNduDo8Q",
		State: "active", Step: "local_check", Assignee: claudeID, Reviewer: codexID, Pass: 90, Depends: []string{},
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

func (st *talkScenarioState) enterHandbackWithRepo(t interface{ TempDir() string }, committed, untracked string) error {
	dir := t.TempDir()
	startSHA, err := prepareScenarioGitRepo(dir, committed, untracked)
	if err != nil {
		return err
	}
	edit := func(s *host.Session) {
		s.Folder = dir
		s.TakeoverAt = startSHA
	}
	if err := st.h.Sessions.Update(st.caller.SessionKey, edit); err != nil {
		return err
	}
	st.h.Sessions.SetTurn(st.caller.SessionKey, host.Turn{Kind: host.HandbackTurn, Thread: st.home.taskID})
	tok := st.h.Sessions.Admit(st.caller.SessionKey, 2, false)
	st.caller, _ = st.h.Sessions.Caller(tok)
	return nil
}

func prepareScenarioGitRepo(dir, committed, untracked string) (string, error) {
	if err := writeAndCommitScenarioFile(dir, true, "base.txt", "base\n", "initial"); err != nil {
		return "", err
	}
	shaOut, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	if err := writeAndCommitScenarioFile(dir, false, committed, "package p\n", "Fix path check"); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(shaOut)), os.WriteFile(filepath.Join(dir, untracked), []byte("todo\n"), 0o600)
}

func writeAndCommitScenarioFile(dir string, initRepo bool, name, content, message string) error {
	if initRepo {
		if err := runScenarioGit(dir, "init"); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		return err
	}
	if err := runScenarioGit(dir, "add", name); err != nil {
		return err
	}
	return runScenarioGit(dir, "commit", "-m", message)
}

func runScenarioGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example")
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %v: %s", args, err, out)
	}
	return nil
}

func (st *talkScenarioState) checkHandbackPost(wantFiles string, wantCommits int) error {
	if len(st.home.requests) == 0 {
		return errors.New("no requests sent")
	}
	cmd := st.home.requests[len(st.home.requests)-1].(proto.Cmd)
	var p proto.Post[proto.HandbackBody]
	if err := proto.Decode([]byte(cmd.Env.Payload), &p); err != nil {
		return err
	}
	gotFiles := strings.Join(p.Body.Files, ", ")
	if p.Kind != "handback" || gotFiles != wantFiles || len(p.Body.Commits) != wantCommits {
		return fmt.Errorf("got kind=%q files=%q commits=%d", p.Kind, gotFiles, len(p.Body.Commits))
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
