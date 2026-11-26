//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_tools_sec"] = hostToolsSecSteps
}

type secScenarioHome struct {
	done      chan struct{}
	requests  []any
	cmds      []proto.Cmd
	groupID   string
	briefID   string
	ownerMID  string
	claudeMID string
	codexMID  string
	secMID    string
}

func (f *secScenarioHome) Send(any) error               { return nil }
func (f *secScenarioHome) Path() string                 { return "local" }
func (f *secScenarioHome) PathChanged() <-chan struct{} { return nil }
func (f *secScenarioHome) Done() <-chan struct{}        { return f.done }

func (f *secScenarioHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	id := int64(len(f.requests) + 1)
	req := build(id)
	f.requests = append(f.requests, req)
	if cmd, ok := req.(proto.Cmd); ok {
		f.cmds = append(f.cmds, cmd)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

type secScenarioWorld struct {
	h         *host.Host
	home      *secScenarioHome
	secCaller host.Caller
	tskCaller host.Caller
	out       any
	err       error
	ownerUID  string
	taskID    string
}

func hostToolsSecSteps(sc *godog.ScenarioContext, _ fixtures) {
	cleanup := afterScenario(sc)
	w := &secScenarioWorld{}

	sc.Step(`^a host group with secretary "([^"]+)", AIs "([^"]+)" and "([^"]+)", owner "([^"]+)", and task "T-12"$`, func(secName, ai1, ai2, ownerName string) error {
		return w.setup(cleanup, secName, ai1, ai2, ownerName)
	})
	sc.Step(`^only "request_decision" of the HS-11 tools is listed to working AI "([^"]+)"$`, func(_ string) error {
		listed := host.ToolsFor(w.tskCaller)
		_, hasBrief := listed["propose_brief"]
		_, hasSet := listed["set_task"]
		_, hasDec := listed["request_decision"]
		if hasBrief || hasSet || !hasDec {
			return fmt.Errorf("unexpected listed flags brief=%v set=%v decision=%v", hasBrief, hasSet, hasDec)
		}
		return nil
	})
	sc.Step(`^secretary "([^"]+)" invokes "([^"]+)" with '([^']*)'$`, func(_, toolName, rawArgs string) error {
		w.home.cmds = nil
		tool, ok := host.ToolsFor(w.secCaller)[toolName]
		if !ok {
			return fmt.Errorf("tool %q not listed to secretary", toolName)
		}
		w.out, w.err = tool.Call(context.Background(), w.h, w.secCaller, json.RawMessage(rawArgs))
		return nil
	})
	sc.Step(`^task session "([^"]+)" on "T-12" invokes "([^"]+)" with '([^']*)'$`, func(_, toolName, rawArgs string) error {
		w.home.cmds = nil
		tool, ok := host.ToolsFor(w.tskCaller)[toolName]
		if !ok {
			return fmt.Errorf("tool %q not listed to task session", toolName)
		}
		w.out, w.err = tool.Call(context.Background(), w.h, w.tskCaller, json.RawMessage(rawArgs))
		return nil
	})
	sc.Step(`^the secretary tool call returns a random id$`, w.checkID)
	sc.Step(`^the secretary tool call returns ok$`, w.checkOK)
	sc.Step(`^the secretary tool call fails with "([^"]*)"$`, func(want string) error {
		if w.err == nil || w.err.Error() != want {
			return fmt.Errorf("got %v, want %q", w.err, want)
		}
		return nil
	})
	sc.Step(`^the home received a signed "brief" post from "([^"]+)" mentioning "([^"]+)" with pass (\d+)$`, func(_, _ string, pass int) error {
		return w.checkBriefPost(pass)
	})
	sc.Step(`^the home received "task" commands "([^"]+)" for task "T-12"$`, w.checkTaskCommands)
	sc.Step(`^the home received a "decision" post on "T-12" recommending "([^"]+)" to "([^"]+)"$`, func(rec, _ string) error {
		return w.checkDecisionPost(rec)
	})
}

func (w *secScenarioWorld) setup(cleanup func(func()), secName, ai1, ai2, ownerName string) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, func() time.Time { return time.UnixMilli(1_763_600_120_000) })
	if err != nil {
		return err
	}
	w.ownerUID = "akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs"
	w.taskID = "DrtCn6htSBwmMPrFPbHJHA"
	home := &secScenarioHome{
		done:      make(chan struct{}),
		groupID:   "rZNvy-1jH6Z-BcPqA5U5BQ",
		briefID:   "KaiCW9JC8UOG7lKNduDo8Q",
		ownerMID:  "4xq2Q8RPeg7IJLWdEZTWDQ",
		claudeMID: "1PC8WineBrUQ-apCjx7tug",
		codexMID:  "LXCZmuGAXkvO-bSrOkuCfw",
		secMID:    "qBSFMsr2hHYKOMblEA_kdA",
	}
	h.Attach("home-dev", home)
	w.h, w.home = h, home
	w.seedRows(secName, ai1, ai2, ownerName, key.ID())
	return w.createSessions(secName, ai1)
}

func (w *secScenarioWorld) seedRows(secName, ai1, ai2, ownerName, hostDev string) {
	put := func(r, id string, v any) {
		raw, _ := json.Marshal(v)
		w.h.Rows.Put(w.home.groupID, r, id, raw)
	}
	put("group", w.home.groupID, proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: w.home.groupID, Seq: 1},
		Name:      "Website", Home: "home-dev",
		Secretary: w.home.secMID, Pass: 90, PassFoundation: 70, Created: 1_763_600_000_000,
	})
	put("member", w.home.ownerMID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.home.ownerMID, Seq: 2}, Group: w.home.groupID, Kind: "human", User: w.ownerUID, Name: ownerName, LastSeen: 1_763_600_000_000},
		Role:      "owner", Online: true,
	})
	for i, ai := range []struct {
		id, cli, name string
		sec           bool
	}{
		{w.home.secMID, "claude", secName, true},
		{w.home.claudeMID, "claude", ai1, false},
		{w.home.codexMID, "codex", ai2, false},
	} {
		put("member", ai.id, proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: ai.id, Seq: int64(3 + i)}, Group: w.home.groupID, Kind: "ai", User: w.ownerUID, Name: ai.name, LastSeen: 1_763_600_000_000},
			CLI:       ai.cli, Host: hostDev, HostName: "ws", Secretary: ai.sec, Account: "personal", Allow: []string{}, Approvers: []string{w.ownerUID}, State: "working", Path: "local",
		})
	}
	put("task", w.taskID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID, Seq: 10},
		Group:     w.home.groupID, Num: 12, Title: "Show path", Origin: "brief", Brief: w.home.briefID,
		State: "active", Step: "work", Assignee: w.home.claudeMID, Reviewer: w.home.codexMID, Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: 1_763_600_000_000}, Created: 1_763_600_000_000,
	})
}

func (w *secScenarioWorld) createSessions(secName, ai1 string) error {
	secKey := host.SessionKey{Group: w.home.groupID, Member: w.home.secMID}
	if err := w.h.Sessions.Create(host.Session{SessionKey: secKey, Kind: host.GroupSession, Folder: handFolder, Agent: "claude", Home: "home-dev", Name: secName, Secretary: true}); err != nil {
		return err
	}
	w.h.Sessions.SetTurn(secKey, host.Turn{Kind: host.ItemsTurn})
	var secOK bool
	if w.secCaller, secOK = w.h.Sessions.Caller(w.h.Sessions.Admit(secKey, 1, false)); !secOK {
		return errors.New("secretary token not admitted")
	}

	tskKey := host.SessionKey{Group: w.home.groupID, Member: w.home.claudeMID, Task: w.taskID}
	if err := w.h.Sessions.Create(host.Session{SessionKey: tskKey, Kind: host.TaskSession, Folder: handFolder, Agent: "claude", Home: "home-dev", Name: ai1}); err != nil {
		return err
	}
	w.h.Sessions.SetTurn(tskKey, host.Turn{Kind: host.ItemsTurn, Thread: w.taskID})
	var tskOK bool
	if w.tskCaller, tskOK = w.h.Sessions.Caller(w.h.Sessions.Admit(tskKey, 2, false)); !tskOK {
		return errors.New("task token not admitted")
	}
	return nil
}

func (w *secScenarioWorld) checkID() error {
	if w.err != nil {
		return w.err
	}
	raw, _ := json.Marshal(w.out)
	var got struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &got) != nil || len(got.ID) != 22 {
		return fmt.Errorf("got %s, want 22-char id", raw)
	}
	return nil
}

func (w *secScenarioWorld) checkOK() error {
	if w.err != nil {
		return w.err
	}
	raw, _ := json.Marshal(w.out)
	if string(raw) != `{"ok":true}` {
		return fmt.Errorf("got %s, want {\"ok\":true}", raw)
	}
	return nil
}

func (w *secScenarioWorld) checkBriefPost(wantPass int) error {
	if len(w.home.cmds) != 1 {
		return fmt.Errorf("got %d cmds, want 1", len(w.home.cmds))
	}
	known := func(dev string) bool { return dev == w.h.Key.ID() }
	if _, err := identity.OpenCommand(w.home.cmds[0].Env, "", known, w.h.Now()); err != nil {
		return err
	}
	var p proto.Post[proto.BriefBody]
	if err := proto.Decode([]byte(w.home.cmds[0].Env.Payload), &p); err != nil {
		return err
	}
	return w.verifyBriefFields(p, wantPass)
}

func (w *secScenarioWorld) verifyBriefFields(p proto.Post[proto.BriefBody], wantPass int) error {
	if string(p.As) != w.home.secMID || p.Body.Pass != wantPass {
		return fmt.Errorf("unexpected brief header/pass: %+v", p)
	}
	if p.Body.Requester != w.home.ownerMID || !slices.Equal(p.Mentions, []proto.ID{proto.ID(w.home.ownerMID)}) {
		return fmt.Errorf("unexpected brief requester/mentions: %+v", p)
	}
	return nil
}

func (w *secScenarioWorld) checkTaskCommands(wantOps string) error {
	var ops []string
	for _, cmd := range w.home.cmds {
		var tc proto.TaskCommand
		if err := proto.Decode([]byte(cmd.Env.Payload), &tc); err != nil {
			return err
		}
		if string(tc.ID) != w.taskID {
			return fmt.Errorf("got task %s, want %s", tc.ID, w.taskID)
		}
		ops = append(ops, tc.Op)
	}
	if strings.Join(ops, ", ") != wantOps {
		return fmt.Errorf("got ops %q, want %q", strings.Join(ops, ", "), wantOps)
	}
	return nil
}

func (w *secScenarioWorld) checkDecisionPost(wantRec string) error {
	if len(w.home.cmds) != 1 {
		return fmt.Errorf("got %d cmds, want 1", len(w.home.cmds))
	}
	var p proto.Post[proto.DecisionBody]
	if err := proto.Decode([]byte(w.home.cmds[0].Env.Payload), &p); err != nil {
		return err
	}
	return w.verifyDecisionFields(p, wantRec)
}

func (w *secScenarioWorld) verifyDecisionFields(p proto.Post[proto.DecisionBody], wantRec string) error {
	tid := proto.ID(w.taskID)
	if p.Main || !reflect.DeepEqual(p.Thread, &tid) {
		return errors.New("unexpected decision post main or task thread")
	}
	if !reflect.DeepEqual(p.Body.Recommended, &wantRec) || !slices.Equal(p.Body.To, []string{w.ownerUID}) {
		return fmt.Errorf("unexpected decision body: %+v", p.Body)
	}
	return nil
}
