package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func defPayload(t *testing.T, agent, model, effort, tier, skills string) json.RawMessage {
	t.Helper()
	cmd := proto.AgentDefaults{
		HostCommand: proto.HostCommand{
			Type:      "agent.defaults",
			Nonce:     testCanonicalID(90),
			Timestamp: seamStart,
			To:        testSeedKey(12).ID(),
		},
		Agent: agent,
		Profile: proto.Profile{
			Model:  model,
			Effort: effort,
			Tier:   tier,
			Skills: skills,
		},
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func defMemberRow(group, member, hostID, ownerID, cli, model, effort, tier, skills string, follow bool, seq int64) json.RawMessage {
	row := proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: member, Seq: seq},
			Group:     group,
			Kind:      "ai",
			User:      ownerID,
			Name:      member,
		},
		CLI:       cli,
		Host:      hostID,
		HostName:  "node",
		Workspace: "/work/" + member,
		Account:   "personal",
		Allow:     []string{ownerID},
		Approvers: []string{},
		State:     "waiting",
		Path:      "local",
		Profile:   proto.Profile{Model: model, Effort: effort, Tier: tier, Skills: skills},
		Follow:    proto.True(follow),
	}
	return json.RawMessage(marshalWire(row))
}

func TestAgentDefaultsValidationPersistenceAndBoundaries(t *testing.T) {
	handle := Surface["agent.defaults"]
	if handle == nil {
		t.Fatal("Surface[agent.defaults] not registered")
	}

	h, _ := seamHost(t, seamStore(t))

	// Decode and strict schema rejections.
	for _, bad := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"t":"agent.defaults","n":"eS_j3OiWABlRuJAcS_RR5w","ts":1,"to":"dev","agent":"claude","model":"","effort":"","tier":"standard"}`),
		json.RawMessage(`{"t":"agent.defaults","n":"eS_j3OiWABlRuJAcS_RR5w","ts":1,"to":"dev","agent":"claude","model":"","tier":"standard","skills":""}`),
		json.RawMessage(`{"t":"agent.defaults","n":"eS_j3OiWABlRuJAcS_RR5w","ts":1,"to":"dev","agent":"claude","model":"","effort":"","tier":"standard","skills":null}`),
		json.RawMessage(`{"t":"agent.defaults","n":"bad","ts":1,"to":"dev","agent":"claude","model":"","effort":"","tier":"standard","skills":""}`),
	} {
		res, err := handle(t.Context(), h, nil, "owner-dev", bad)
		agtEqual(t, res, nil)
		seamCode(t, err, "bad_request")
	}

	// Agent and Profile boundary rejections.
	for _, tc := range []struct {
		name                           string
		agent, model, effort, tier, sk string
	}{
		{"external rejected", "external", "", "", "standard", ""},
		{"unknown agent rejected", "gemini", "", "", "standard", ""},
		{"empty agent rejected", "", "", "", "standard", ""},
		{"invalid tier", "claude", "", "", "expert", ""},
		{"empty tier", "claude", "", "", "", ""},
		{"model over 100 runes", "claude", strings.Repeat("a", 101), "", "standard", ""},
		{"model control char", "claude", "opus\x00", "", "standard", ""},
		{"effort over 32 runes", "claude", "", strings.Repeat("e", 33), "standard", ""},
		{"effort control char", "claude", "", "high\t", "standard", ""},
		{"skills over 200 runes", "claude", "", "", "standard", strings.Repeat("s", 201)},
		{"skills newline", "claude", "", "", "standard", "Go\nbackend"},
		{"skills carriage return", "claude", "", "", "standard", "Go\rbackend"},
	} {
		res, err := handle(t.Context(), h, nil, "owner-dev", defPayload(t, tc.agent, tc.model, tc.effort, tc.tier, tc.sk))
		if res != nil {
			t.Fatalf("%s: res=%v, want nil", tc.name, res)
		}
		seamCode(t, err, "bad_request")
	}

	// Exact max bounds succeed, CLI does not need to be on PATH, and no audit row is written.
	maxModel := strings.Repeat("m", 100)
	maxEffort := strings.Repeat("e", 32)
	maxSkills := strings.Repeat("s", 200)
	res, err := handle(t.Context(), h, nil, "owner-dev", defPayload(t, "claude", maxModel, maxEffort, "strong", maxSkills))
	agtCheck(t, err)
	agtEqual(t, res, nil)
	agtEqual(t, seamRecord(t, h), "")

	// Golden fixture decodes cleanly and updates codex while preserving claude.
	golden, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "agent-defaults.json"))
	agtCheck(t, err)
	res, err = handle(t.Context(), h, nil, "owner-dev", golden)
	agtCheck(t, err)
	agtEqual(t, res, nil)

	// Overwrite claude and confirm codex persists alongside it, still with 0 audit rows.
	res, err = handle(t.Context(), h, nil, "owner-dev", defPayload(t, "codex", "gpt-5", "medium", "light", "Frontend"))
	agtCheck(t, err)
	agtEqual(t, res, nil)
	stored, err := readDefaults(h.Store)
	agtCheck(t, err)
	agtEqual(t, stored["claude"], proto.Profile{Model: "opus", Effort: "high", Tier: "strong", Skills: "Go backend, protocol design"})
	agtEqual(t, stored["codex"], proto.Profile{Model: "gpt-5", Effort: "medium", Tier: "light", Skills: "Frontend"})
	agtEqual(t, seamRecord(t, h), "")

	// Store failure returns internal error cleanly.
	seamSQL(t, h.Store, `DROP TABLE setting`)
	_, err = handle(t.Context(), h, nil, "owner-dev", defPayload(t, "claude", "", "", "standard", ""))
	if err == nil {
		t.Fatal("expected store error after dropping setting table")
	}
}

func TestFollowDefaultsSyncsOnlyStaleFollowingGroupSessions(t *testing.T) {
	diag := captureDiag(t)
	r := newDeliverRig(t, false)
	home2 := newFakeDeliverHome()
	t.Cleanup(home2.closeConn)
	r.h.Attach(r.homeKey.ID(), r.home)
	r.h.Attach(r.home2Key.ID(), home2)

	createGroupSession := func(g, m, agent, home string) {
		t.Helper()
		agtCheck(t, r.h.Sessions.Create(Session{
			SessionKey: SessionKey{Group: g, Member: m},
			Kind:       GroupSession, Folder: "/work/" + m, Agent: agent, Home: home, Name: m,
		}))
	}

	mModel := string(testCanonicalID(1))
	mEffort := string(testCanonicalID(2))
	mTier := string(testCanonicalID(3))
	mSkills := string(testCanonicalID(4))
	mSame := string(testCanonicalID(5))
	mOwn := string(testCanonicalID(6))
	mCodex := string(testCanonicalID(7))
	mExt := string(testCanonicalID(8))
	mNoRow := string(testCanonicalID(9))
	mHome2 := string(testCanonicalID(10))

	for _, m := range []string{mModel, mEffort, mTier, mSkills, mSame, mOwn, mNoRow} {
		createGroupSession(testGroup1, m, "claude", r.homeKey.ID())
	}
	agtCheck(t, r.h.Sessions.Create(Session{
		SessionKey: SessionKey{Group: testGroup1, Member: mModel, Task: testTask1},
		Kind:       TaskSession, Folder: "/work/" + mModel, Agent: "claude", Home: r.homeKey.ID(), Name: mModel,
	}))
	createGroupSession(testGroup1, mCodex, "codex", r.homeKey.ID())
	createGroupSession(testGroup1, mExt, externalAgent, r.homeKey.ID())
	createGroupSession(testGroup2, mHome2, "claude", r.home2Key.ID())

	// Seed rows where 4 followers differ on one field each, 1 already matches,
	// 1 has own profile, codex follows its own, external is ignored even with follow:true,
	// and mNoRow has no row in h.Rows.
	putG1 := func(m, cli, model, effort, tier, skills string, follow bool) {
		r.h.Rows.Put(testGroup1, "member", m, defMemberRow(testGroup1, m, r.hostKey.ID(), r.ownerKey.ID(), cli, model, effort, tier, skills, follow, 10))
	}
	putG1(mModel, "claude", "haiku", "high", "strong", "Go", true)
	putG1(mEffort, "claude", "opus", "low", "strong", "Go", true)
	putG1(mTier, "claude", "opus", "high", "light", "Go", true)
	putG1(mSkills, "claude", "opus", "high", "strong", "Docs", true)
	putG1(mSame, "claude", "opus", "high", "strong", "Go", true)
	putG1(mOwn, "claude", "haiku", "low", "light", "Own", false)
	putG1(mCodex, "codex", "", "", "standard", "", true)
	putG1(mExt, externalAgent, "", "", "standard", "", true)
	r.h.Rows.Put(testGroup2, "member", mHome2, defMemberRow(testGroup2, mHome2, r.hostKey.ID(), r.ownerKey.ID(), "claude", "haiku", "", "standard", "", true, 10))

	// Refusal from home2 (conflict: owner switched meanwhile) is logged cleanly.
	home2.mu.Lock()
	home2.onCmd = func(_ context.Context, cmd proto.Cmd) (json.RawMessage, error) {
		return json.RawMessage(marshalWire(proto.ErrorResponse{
			Re:  cmd.ID,
			Err: proto.WireError{Code: "conflict", Message: "the member has its own profile"},
		})), nil
	}
	home2.mu.Unlock()

	_, err := Surface["agent.defaults"](t.Context(), r.h, nil, r.ownerKey.ID(), defPayload(t, "claude", "opus", "high", "strong", "Go"))
	agtCheck(t, err)
	r.h.Settle()

	nodeTag := "node=" + r.hostKey.ID()[:8]
	diag.ran(t, 1, nodeTag, "defaults g="+testGroup1)
	diag.ran(t, 1, nodeTag, "defaults g="+testGroup2)

	var gotIDs []proto.ID
	for _, payload := range r.home.cmdPayloads() {
		var cmd proto.MemberSet
		agtCheck(t, proto.Decode([]byte(payload), &cmd))
		gotIDs = append(gotIDs, cmd.ID)
		agtEqual(t, cmd.Op, "set")
		agtEqual(t, bool(cmd.Follow), true)
		agtEqual(t, *cmd.Model, "opus")
		agtEqual(t, *cmd.Effort, "high")
		agtEqual(t, *cmd.Tier, "strong")
		agtEqual(t, *cmd.Skills, "Go")
	}
	slices.Sort(gotIDs)
	wantIDs := []proto.ID{proto.ID(mModel), proto.ID(mEffort), proto.ID(mTier), proto.ID(mSkills)}
	slices.Sort(wantIDs)
	agtEqual(t, gotIDs, wantIDs)

	h2Payloads := home2.cmdPayloads()
	agtEqual(t, len(h2Payloads), 1)
	var h2Cmd proto.MemberSet
	agtCheck(t, proto.Decode([]byte(h2Payloads[0]), &h2Cmd))
	agtEqual(t, h2Cmd.ID, proto.ID(mHome2))

	// Calling followDefaults when defaults are unset sends standard/"" for stale follower,
	// and logs cleanly when setting defaults JSON is corrupt.
	hUnset, _ := seamHost(t, seamStore(t))
	hUnsetHome := newFakeDeliverHome()
	t.Cleanup(hUnsetHome.closeConn)
	hUnset.Attach(r.homeKey.ID(), hUnsetHome)
	agtCheck(t, hUnset.Sessions.Create(Session{
		SessionKey: SessionKey{Group: testGroup1, Member: mModel},
		Kind:       GroupSession, Folder: "/work/unset", Agent: "claude", Home: r.homeKey.ID(), Name: "Claude",
	}))
	hUnset.Rows.Put(testGroup1, "member", mModel, defMemberRow(testGroup1, mModel, hUnset.Key.ID(), r.ownerKey.ID(), "claude", "opus", "", "standard", "", true, 1))
	followDefaults(hUnset, testGroup1)
	unsetPayloads := hUnsetHome.cmdPayloads()
	agtEqual(t, len(unsetPayloads), 1)
	if !strings.Contains(unsetPayloads[0], `"follow":true`) || !strings.Contains(unsetPayloads[0], `"model":""`) || !strings.Contains(unsetPayloads[0], `"tier":"standard"`) {
		t.Fatalf("unset follow payload = %s", unsetPayloads[0])
	}

	seamSQL(t, hUnset.Store, `INSERT OR REPLACE INTO setting(key, value) VALUES('defaults', '{')`)
	followDefaults(hUnset, testGroup1)
	agtEqual(t, len(hUnsetHome.cmdPayloads()), 1)
}

func TestFollowDefaultsConcurrentCalls(t *testing.T) {
	r := newDeliverRig(t, false)
	r.h.Attach(r.homeKey.ID(), r.home)
	agtCheck(t, r.h.Sessions.Create(Session{
		SessionKey: SessionKey{Group: testGroup1, Member: testAI1},
		Kind:       GroupSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Claude",
	}))
	r.h.Rows.Put(testGroup1, "member", testAI1, defMemberRow(testGroup1, testAI1, r.hostKey.ID(), r.ownerKey.ID(), "claude", "haiku", "", "light", "", true, 1))
	r.home.mu.Lock()
	r.home.onCmd = func(_ context.Context, cmd proto.Cmd) (json.RawMessage, error) {
		var ms proto.MemberSet
		if proto.Decode([]byte(cmd.Env.Payload), &ms) == nil {
			r.h.Rows.Put(testGroup1, "member", string(ms.ID), defMemberRow(testGroup1, string(ms.ID), r.hostKey.ID(), r.ownerKey.ID(), "claude", *ms.Model, *ms.Effort, *ms.Tier, *ms.Skills, bool(ms.Follow), 2))
		}
		return json.RawMessage(marshalWire(proto.CmdResult{Success: proto.Success{Re: cmd.ID, OK: true}})), nil
	}
	r.home.mu.Unlock()

	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Surface["agent.defaults"](t.Context(), r.h, nil, r.ownerKey.ID(), defPayload(t, "claude", "opus", "high", "strong", "Go"))
			if err != nil {
				t.Errorf("agent.defaults: %v", err)
			}
			followDefaults(r.h, testGroup1)
		}()
	}
	wg.Wait()
	r.h.Settle()

	var row proto.AIMemberRow
	raw, _ := r.h.Rows.Get(testGroup1, "member", testAI1)
	agtCheck(t, proto.Decode(raw, &row))
	agtEqual(t, row.Profile, proto.Profile{Model: "opus", Effort: "high", Tier: "strong", Skills: "Go"})
	agtEqual(t, bool(row.Follow), true)
}

func TestFollowingMemberRosterMatchesOwnProfile(t *testing.T) {
	r := newTurnRig(t)
	sec := r.session(GroupSession, true, "")
	setTurnProfile(t, r, tAIID, func(ai *proto.AIMemberRow) {
		ai.Model, ai.Effort, ai.Tier, ai.Skills = "opus", "high", "strong", "Go backend"
		ai.Follow = false
	})
	ownText, ownTurn, err := Compose(r.h, sec, ReplacedTurn, nil)
	agtCheck(t, err)

	setTurnProfile(t, r, tAIID, func(ai *proto.AIMemberRow) {
		ai.Follow = true
	})
	followText, followTurn, err := Compose(r.h, sec, ReplacedTurn, nil)
	agtCheck(t, err)

	agtEqual(t, followText, ownText)
	agtEqual(t, followTurn, ownTurn)
	if strings.Contains(followText, "follow") {
		t.Fatalf("roster text contains follow:\n%s", followText)
	}
}
