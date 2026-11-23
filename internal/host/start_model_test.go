package host

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func putStartProfile(h *Host, key SessionKey, model, effort string) {
	m := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: key.Member}, Group: key.Group, Kind: "ai", Name: "AI"},
		CLI:       "claude", Host: h.Key.ID(), Allow: []string{}, Approvers: []string{},
		Profile: proto.Profile{Model: model, Effort: effort, Tier: "standard"},
	}
	h.Rows.Put(key.Group, "member", key.Member, json.RawMessage(marshalWire(m)))
}

func TestModelFlagsPreserveValuesAndOmitEmptyFields(t *testing.T) {
	for _, tc := range []struct {
		model, effort string
		claude, codex []string
	}{
		{"", "", nil, nil},
		{"unlisted/model", "", []string{"--model", "unlisted/model"}, []string{"-m", "unlisted/model"}},
		{"", `x"high`, []string{"--effort", `x"high`}, []string{"-c", `model_reasoning_effort="x\"high"`}},
		{"opus", "high", []string{"--model", "opus", "--effort", "high"}, []string{"-m", "opus", "-c", `model_reasoning_effort="high"`}},
	} {
		l := Launch{Model: tc.model, Effort: tc.effort}
		agtEqual(t, claudeModelFlags(l), tc.claude)
		agtEqual(t, codexModelFlags(l), tc.codex)
	}
}

func TestClaudeModelFlagsAtNewResumeAndTerminalStarts(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		for _, session := range []string{"", "existing"} {
			l := Launch{Session: session, Level: 1, Shim: []string{"solodawn"}}
			base, id, err := claudeCommandArgs(l, interactive, false)
			agtCheck(t, err)
			l.Model, l.Effort = "unlisted-model", "unlisted-effort"
			got, id, err := claudeCommandArgs(l, interactive, false)
			agtCheck(t, err)
			if idx := slices.Index(base, "--session-id"); idx >= 0 {
				base[idx+1] = id
			}
			level, err := claudeLevelFlags(1, interactive, false)
			agtCheck(t, err)
			split := len(base) - len(level)
			want := append(slices.Clone(base[:split]), "--model", l.Model, "--effort", l.Effort)
			want = append(want, level...)
			agtEqual(t, got, want)
		}
	}
}

func TestCodexModelAtThreadStartAndResume(t *testing.T) {
	for _, session := range []string{"", "existing"} {
		l := codexTestLaunch(t, 1)
		l.Session, l.Model, l.Effort = session, "unlisted-model", "unlisted-effort"
		proc, peer := startPeerCodex(t, l)
		peer.accept()
		peer.readJSON()
		peer.writeLine(`{"id":1,"result":{}}`)
		agtEqual(t, peer.readJSON(), map[string]any{"method": "initialized"})
		request := peer.readJSON()
		params := map[string]any{"cwd": l.Folder, "sandbox": "read-only", "approvalPolicy": "never", "developerInstructions": l.Instructions, "model": l.Model, "config": map[string]any{"project_doc_max_bytes": float64(0), "model_reasoning_effort": l.Effort}}
		method := "thread/start"
		if session != "" {
			method = "thread/resume"
			params["threadId"] = session
		}
		agtEqual(t, request, map[string]any{"id": float64(2), "method": method, "params": params})
		agtCheck(t, proc.Close())
		peer.expectEOF()
		agtCheck(t, peer.conn.Close())
		agtEqual(t, waitEventsClosed(t, proc.Events()), []Event(nil))
	}
}

func TestCodexModelInTerminalResume(t *testing.T) {
	l := codexTestLaunch(t, 1)
	l.Session = "existing"
	base, _, err := (codexDriver{}).Interactive(l)
	agtCheck(t, err)
	l.Model, l.Effort = "unlisted", "high"
	got, id, err := (codexDriver{}).Interactive(l)
	agtCheck(t, err)
	split := slices.Index(base, "-s")
	want := append(slices.Clone(base[:split]), "-m", "unlisted", "-c", `model_reasoning_effort="high"`)
	want = append(want, base[split:]...)
	agtEqual(t, got, want)
	agtEqual(t, id, "existing")
}

func TestCodexRefusedResumeValueIsNotALostSession(t *testing.T) {
	l := codexTestLaunch(t, 1)
	l.Session, l.Model = "existing", "refused"
	proc, peer := startPeerCodex(t, l)
	peer.accept()
	peer.readJSON()
	peer.writeLine(`{"id":1,"result":{}}`)
	peer.readJSON()
	peer.readJSON()
	peer.writeLine(`{"id":2,"error":{"code":-32600,"message":"model refused"}}`)
	peer.expectEOF()
	agtCheck(t, peer.conn.Close())
	agtEqual(t, waitEventsClosed(t, proc.Events()), []Event(nil))
	agtErrText(t, proc.Turn("hello"), "model refused")
	agtEqual(t, codexSessionMissing("THREAD NOT FOUND"), true)
}

func TestMemberChoiceWaitsForNextProcessStart(t *testing.T) {
	h, rh, drv, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	s := makeRunSession(key, GroupSession, "repo")
	s.CLISession = "existing"
	agtCheck(t, h.Sessions.Create(s))
	putStartProfile(h, key, "opus", "high")
	agtCheck(t, Runner.Turn(t.Context(), h, key, "first", Turn{Kind: ItemsTurn}))
	rh.waitCmd(t)
	putStartProfile(h, key, "sonnet", "low")
	agtEqual(t, drv.count(), 1)
	first := drv.lastLaunch()
	agtEqual(t, []string{first.Model, first.Effort, first.Session}, []string{"opus", "high", "existing"})
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	rh.waitCmd(t)
	agtCheck(t, Runner.Turn(t.Context(), h, key, "same process", Turn{Kind: ItemsTurn}))
	agtEqual(t, drv.count(), 1)
	Runner.Close(h, key)
	agtCheck(t, Runner.Turn(t.Context(), h, key, "resume", Turn{Kind: ItemsTurn}))
	second := drv.lastLaunch()
	agtEqual(t, []string{second.Model, second.Effort, second.Session}, []string{"sonnet", "low", "existing"})
}

func TestTakeoverUsesCurrentMemberChoice(t *testing.T) {
	r := newTkRig(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	putStartProfile(r.h, key, "opus", "high")
	agtCheck(t, r.start(runAI1, ""))
	l := r.lastOpened()
	agtEqual(t, []string{l.Model, l.Effort}, []string{"opus", "high"})
}

func TestRefusedModelCrashesTaskWithoutReplacingSession(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) { testRefusedModelCrash(t, agent) })
	}
}

func testRefusedModelCrash(t *testing.T, agent string) {
	h, rh, _, _ := setupRunEnv(t)
	modelBinaries(t)
	Drivers = map[string]Driver{"claude": claudeDriver{}, "codex": codexDriver{}}
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	s := makeRunSession(key, TaskSession, "repo")
	s.Agent, s.CLISession = agent, "existing"
	agtCheck(t, os.MkdirAll(sessionDir(h, s), 0700))
	agtCheck(t, h.Sessions.Create(s))
	putStartProfile(h, key, "refused", "high")
	script := `{"session":"existing","steps":[{"dir":"stdin","msg":{"type":"user"}}],"exit":1}`
	if agent == "codex" {
		script = `{"session":"existing","steps":[{"dir":"stdin","msg":{"method":"initialize"}},{"dir":"stdout","msg":{"id":1,"result":{}}},{"dir":"stdin","msg":{"method":"initialized"}},{"dir":"stdin","msg":{"method":"thread/resume","params":{"threadId":"existing","model":"refused","config":{"model_reasoning_effort":"high"}}}},{"dir":"stdout","msg":{"id":2,"error":{"code":-32600,"message":"model refused"}}},{"dir":"eof"}]}`
	}
	modelScript(t, agent, script)
	Runner.Turn(t.Context(), h, key, "run", Turn{Kind: ItemsTurn, Thread: runTask1})
	assertMemberStatusCmd(t, rh, "working", runTask1)
	crash := rh.waitCmd(t)
	var status proto.MemberStatus[proto.TurnInterruptedEvent]
	agtCheck(t, proto.Decode([]byte(crash.raw), &status))
	agtEqual(t, []string{status.State, status.Event.Code, status.Event.Reason}, []string{"dormant", "turn_interrupted", "crash"})
	fail := rh.waitCmd(t)
	var failed proto.TaskFail
	agtCheck(t, proto.Decode([]byte(fail.raw), &failed))
	agtEqual(t, []string{failed.Type, failed.Op, string(failed.ID), failed.Reason}, []string{"task", "fail", runTask1, "crash"})
	got, _ := h.Sessions.Get(key)
	agtEqual(t, []string{string(got.State), got.CLISession}, []string{"dormant", "existing"})
	agtEqual(t, len(h.Sessions.All()), 1)
}

func TestJoinPassesProfilesAndLeaveSelectsDistinctMembers(t *testing.T) {
	r := newJoinRig(t)
	r.setAgent("claude", "version", joinStr("personal"))
	fh := r.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website", r.makeRepo("profile", "main"), "main"))
	profile := proto.ProfileSet{Model: joinStr("unlisted"), Effort: joinStr("high"), Tier: joinStr("strong"), Skills: joinStr("Go")}
	for _, name := range []string{"One", "Two"} {
		r.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: name, ProfileSet: profile})
	}
	agtEqual(t, len(fh.addAIs), 2)
	agtEqual(t, fh.addAIs[0].ProfileSet, profile)
	agtEqual(t, fh.addAIs[1].ProfileSet, profile)
	seamCode(t, r.callLeave(t.Context(), joinTestGroup1, "claude"), "bad_request")
	p := proto.HostLeave{HostCommand: proto.HostCommand{Type: "host.leave", Nonce: joinTestNonce2, Timestamp: r.h.Now().UnixMilli(), To: r.h.Key.ID()}, Group: joinTestGroup1, Agent: "claude", Member: fh.addAIs[1].ID}
	_, err := Surface["host.leave"](t.Context(), r.h, nil, joinTestByDev, json.RawMessage(marshalWire(p)))
	agtCheck(t, err)
	agtEqual(t, joinAgentMembers(r.h, string(joinTestGroup1), "claude"), map[string]string{string(fh.addAIs[0].ID): "home-1"})
	p.Member = fh.addAIs[1].ID
	_, _, err = joinLeaveMember(r.h, p)
	seamCode(t, err, "not_found")
}
