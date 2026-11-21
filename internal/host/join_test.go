package host

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	joinTestGroup1 = proto.ID("rZNvy-1jH6Z-BcPqA5U5BQ")
	joinTestGroup2 = proto.ID("H46nb5YsBesaWbHkHmZLow")
	joinTestGroup3 = proto.ID("BEVDTkBah3oHfAzuxeLAZw")
	joinTestNonce1 = proto.ID("eS_j3OiWABlRuJAcS_RR5w")
	joinTestNonce2 = proto.ID("HzE87sj3_L_tiVTdhfybwQ")
	joinTestOwner  = "akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs"
	joinTestByDev  = "owner-browser-device"
	joinTestSecret = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
)

type joinDialRecord struct {
	device string
	ctx    context.Context
}

type joinFakeHome struct {
	mu      sync.Mutex
	done    chan struct{}
	host    *Host
	device  string
	groups  map[string]proto.GroupRow
	syncFn  func(proto.Sync) (json.RawMessage, error)
	cmdFn   func(proto.Envelope) (json.RawMessage, error)
	syncs   []proto.Sync
	addAIs  []proto.MemberAddAI
	removes []proto.MemberCommand
}

func newJoinFakeHome(h *Host, device string) *joinFakeHome {
	return &joinFakeHome{
		done:   make(chan struct{}),
		host:   h,
		device: device,
		groups: map[string]proto.GroupRow{},
	}
}

func (f *joinFakeHome) Send(any) error               { return nil }
func (f *joinFakeHome) Path() string                 { return "direct" }
func (f *joinFakeHome) PathChanged() <-chan struct{} { return nil }
func (f *joinFakeHome) Done() <-chan struct{}        { return f.done }

func (f *joinFakeHome) putGroup(grp proto.GroupRow) {
	f.mu.Lock()
	f.groups[grp.ID] = grp
	f.mu.Unlock()
}

func (f *joinFakeHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	frame := build(7)
	switch req := frame.(type) {
	case proto.Sync:
		return f.handleSync(req)
	case proto.Cmd:
		return f.handleCmd(req)
	default:
		return nil, io.EOF
	}
}

func (f *joinFakeHome) handleSync(req proto.Sync) (json.RawMessage, error) {
	f.mu.Lock()
	f.syncs = append(f.syncs, req)
	override := f.syncFn
	grp, ok := f.groups[req.Group]
	f.mu.Unlock()
	if override != nil {
		return override(req)
	}
	if !ok {
		resp := proto.ErrorResponse{Re: req.ID, Err: proto.WireError{Code: "not_found", Message: "group not found"}}
		return json.RawMessage(joinEncodeJSON(resp)), nil
	}
	f.host.Rows.Put(req.Group, "group", req.Group, json.RawMessage(joinEncodeJSON(grp)))
	synced := proto.Synced{
		Type:    "synced",
		Success: proto.Success{Re: req.ID, OK: true},
		Group:   req.Group,
		Seq:     grp.Seq,
	}
	return json.RawMessage(joinEncodeJSON(synced)), nil
}

func (f *joinFakeHome) handleCmd(req proto.Cmd) (json.RawMessage, error) {
	f.mu.Lock()
	override := f.cmdFn
	f.mu.Unlock()
	if override != nil {
		return override(req.Env)
	}
	var base proto.MemberCommand
	if err := proto.Decode([]byte(req.Env.Payload), &base); err != nil {
		return nil, err
	}
	f.mu.Lock()
	if base.Op == "add_ai" {
		var add proto.MemberAddAI
		if err := proto.Decode([]byte(req.Env.Payload), &add); err != nil {
			f.mu.Unlock()
			return nil, err
		}
		f.addAIs = append(f.addAIs, add)
	}
	if base.Op == "remove" {
		f.removes = append(f.removes, base)
	}
	f.mu.Unlock()
	ok := proto.CmdResult{Success: proto.Success{Re: req.ID, OK: true}}
	return json.RawMessage(joinEncodeJSON(ok)), nil
}

type joinRig struct {
	t      *testing.T
	root   string
	dbPath string
	db     *store.Store
	h      *Host
	mu     sync.Mutex
	dials  []joinDialRecord
	homes  map[string]*joinFakeHome
}

func newJoinRig(t *testing.T) *joinRig {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "node.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h, _ := seamHost(t, db)
	rig := &joinRig{
		t:      t,
		root:   root,
		dbPath: dbPath,
		db:     db,
		h:      h,
		homes:  map[string]*joinFakeHome{},
	}
	h.Connect = rig.connectSync
	return rig
}

func (r *joinRig) connectSync(ctx context.Context, device string) {
	r.mu.Lock()
	r.dials = append(r.dials, joinDialRecord{device: device, ctx: ctx})
	fh := r.homes[device]
	r.mu.Unlock()
	if fh != nil {
		r.h.Attach(device, fh)
	}
}

func (r *joinRig) dialCount(device string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, d := range r.dials {
		if d.device == device {
			n++
		}
	}
	return n
}

func (r *joinRig) lastDialCtx(device string) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.dials) - 1; i >= 0; i-- {
		if r.dials[i].device == device {
			return r.dials[i].ctx
		}
	}
	return nil
}

func (r *joinRig) addHome(device string) *joinFakeHome {
	fh := newJoinFakeHome(r.h, device)
	r.mu.Lock()
	r.homes[device] = fh
	r.mu.Unlock()
	return fh
}

func joinExecSQL(t *testing.T, db *store.Store, query string, args ...any) {
	t.Helper()
	exec := func(tx *store.Tx) error {
		_, err := tx.Exec(query, args...)
		return err
	}
	if err := db.Write(exec); err != nil {
		t.Fatal(err)
	}
}

func (r *joinRig) setAgent(agent, version string, account *string) {
	r.t.Helper()
	var acc any
	if account != nil {
		acc = *account
	}
	joinExecSQL(r.t, r.db, "INSERT OR REPLACE INTO agent(agent, cli_version, account) VALUES(?, ?, ?)", agent, version, acc)
}

func (r *joinRig) setSetting(key string, val any) {
	r.t.Helper()
	joinExecSQL(r.t, r.db, "INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)", key, joinEncodeJSON(val))
}

func (r *joinRig) makeRepo(name, branch string) string {
	r.t.Helper()
	repo := filepath.Join(r.root, "upstreams", name)
	joinGit(r.t, r.root, "init", "-b", branch, repo)
	joinGit(r.t, repo, "commit", "--allow-empty", "-m", "initial")
	return repo
}

func joinGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func makeGroupRow(id proto.ID, home, name, repo, base string) proto.GroupRow {
	return proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: string(id), Seq: 3},
		Name:           name,
		Kind:           "project",
		Repo:           repo,
		Base:           base,
		Forge:          "none",
		Home:           home,
		Checks:         []string{},
		Pass:           90,
		PassFoundation: 70,
		Limits: proto.Limits{
			Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3,
			ReviewRounds: 5, AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
		},
		Created: seamStart,
	}
}

func (r *joinRig) callJoin(ctx context.Context, p proto.HostJoin) error {
	if p.Type == "" {
		p.Type = "host.join"
	}
	if p.Nonce == "" {
		p.Nonce = joinTestNonce1
	}
	if p.Timestamp == 0 {
		p.Timestamp = r.h.Now().UnixMilli()
	}
	if p.To == "" {
		p.To = r.h.Key.ID()
	}
	_, err := Surface["host.join"](ctx, r.h, nil, joinTestByDev, json.RawMessage(joinEncodeJSON(p)))
	return err
}

func (r *joinRig) callLeave(ctx context.Context, group proto.ID, agent string) error {
	p := proto.HostLeave{
		HostCommand: proto.HostCommand{
			Type:      "host.leave",
			Nonce:     joinTestNonce2,
			Timestamp: r.h.Now().UnixMilli(),
			To:        r.h.Key.ID(),
		},
		Group: group,
		Agent: agent,
	}
	_, err := Surface["host.leave"](ctx, r.h, nil, joinTestByDev, json.RawMessage(joinEncodeJSON(p)))
	return err
}

func strPtr(s string) *string { return &s }

func TestHostJoinAndLeaveRegistrations(t *testing.T) {
	if Surface["host.join"] == nil || Surface["host.leave"] == nil || Attached["join"] == nil {
		t.Fatalf("missing init registrations: join=%v leave=%v attached=%v", Surface["host.join"] != nil, Surface["host.leave"] != nil, Attached["join"] != nil)
	}
}

func TestHostJoinPayloadValidation(t *testing.T) {
	rig := newJoinRig(t)
	badRaw := []string{
		`not-json`,
		`{"t":"host.join"}`,
		`{"t":"host.join","n":"H46nb5YsBesaWbHkHmZLow","ts":1,"to":"dev","g":"bad-id","home":"h1","agent":"claude","name":"Claude","secretary":false}`,
	}
	for _, raw := range badRaw {
		_, err := Surface["host.join"](t.Context(), rig.h, nil, joinTestByDev, json.RawMessage(raw))
		seamCode(t, err, "bad_request")
	}

	cases := []proto.HostJoin{
		{Group: joinTestGroup1, Home: "", Agent: "claude", Name: "Claude"},
		{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: ""},
		{Group: joinTestGroup1, Home: "home-1", Agent: "", Name: "Claude"},
		{Group: joinTestGroup1, Home: "home-1", Agent: "gemini", Name: "Gemini"},
		{Group: joinTestGroup1, Home: "home-1", Agent: "Claude", Name: "Claude"},
	}
	for _, tc := range cases {
		err := rig.callJoin(t.Context(), tc)
		seamCode(t, err, "bad_request")
	}
	if rig.dialCount("home-1") != 0 {
		t.Fatalf("invalid payloads must not dial, got %d dials", rig.dialCount("home-1"))
	}
}

func TestHostJoinPreconditionsAndInFlightConflict(t *testing.T) {
	rig := newJoinRig(t)
	fh := rig.addHome("home-1")
	repo := rig.makeRepo("web", "main")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website", repo, "main"))
	fh.putGroup(makeGroupRow(joinTestGroup2, "home-1", "Backend", repo, "main"))

	// Agent missing from agent table.
	err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})
	seamCode(t, err, "conflict")

	// Agent present with NULL account.
	rig.setAgent("claude", "2.1.311", nil)
	err = rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})
	seamCode(t, err, "conflict")
	if rig.dialCount("home-1") != 0 {
		t.Fatalf("undeclared account must not dial, got %d", rig.dialCount("home-1"))
	}

	// Declare both agents and block sync to test concurrent duplicate (group, agent) join.
	rig.setAgent("claude", "2.1.311", strPtr("personal"))
	rig.setAgent("codex", "0.164.0", strPtr("team"))

	enteredSync := make(chan struct{})
	releaseSync := make(chan struct{})
	var once sync.Once
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		once.Do(func() { close(enteredSync) })
		<-releaseSync
		return nil, io.EOF
	}

	firstErr := make(chan error, 1)
	go func() {
		firstErr <- rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})
	}()
	<-enteredSync

	// Same (group, agent) while in flight is refused with conflict.
	dupErr := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude 2"})
	seamCode(t, dupErr, "conflict")

	close(releaseSync)
	seamCode(t, <-firstErr, "home_unreachable")

	// After the failed join released its slot, joining succeeds.
	fh.syncFn = nil
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"}); err != nil {
		t.Fatalf("join after slot release failed: %v", err)
	}

	// Already-joined (group, agent) is refused, while (same group, other agent) and (other group, same agent) succeed.
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude Again"}), "conflict")
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "codex", Name: "Codex"}); err != nil {
		t.Fatalf("other agent in same group failed: %v", err)
	}
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup2, Home: "home-1", Agent: "claude", Name: "Claude Backend"}); err != nil {
		t.Fatalf("same agent in other group failed: %v", err)
	}
}

func TestHostJoinOneConnectionPerHomeCloneReuseAndSecretaryDemotion(t *testing.T) {
	rig := newJoinRig(t)
	rig.setSetting("owner", joinTestOwner)
	rig.setAgent("claude", "2.1.311", strPtr("personal"))
	rig.setAgent("codex", "0.164.0", strPtr("api_key"))

	fhA := rig.addHome("home-a")
	fhB := rig.addHome("home-b")
	repo1 := rig.makeRepo("repo1", "main")
	repo2 := rig.makeRepo("repo2", "trunk")
	repo3 := rig.makeRepo("repo3", "main")
	fhA.putGroup(makeGroupRow(joinTestGroup1, "home-a", "Website", repo1, "main"))
	fhA.putGroup(makeGroupRow(joinTestGroup2, "home-a", "Backend", repo2, "trunk"))
	fhB.putGroup(makeGroupRow(joinTestGroup3, "home-b", "Docs", repo3, "main"))

	// Seed another group's secretary session to verify demotion affects only the target group.
	if err := rig.h.Sessions.Create(Session{
		SessionKey: SessionKey{Group: string(joinTestGroup3), Member: "existing-sec"},
		Kind:       GroupSession,
		Folder:     "repo",
		Agent:      "claude",
		Home:       "home-b",
		Name:       "Docs Sec",
		Secretary:  true,
	}); err != nil {
		t.Fatal(err)
	}

	// First join on home-a: claude joins group 1 as secretary.
	if err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-a", Agent: "claude", Name: "Claude Sec", Secretary: true,
	}); err != nil {
		t.Fatal(err)
	}
	// Remove upstream repo1 to prove the second AI joining group 1 reuses projects/<g1>/repo without re-cloning.
	if _, err := os.Stat(filepath.Join(rig.root, "projects", string(joinTestGroup1), "repo", ".git")); err != nil {
		t.Fatalf("expected clone in store projects directory: %v", err)
	}
	if err := os.RemoveAll(repo1); err != nil {
		t.Fatal(err)
	}
	// Update cached group row name and join codex to group 1 as the new secretary.
	renamed := makeGroupRow(joinTestGroup1, "home-a", "Website v2", repo1, "main")
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(joinEncodeJSON(renamed)))
	if err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-a", Agent: "codex", Name: "Codex Sec", Secretary: true,
	}); err != nil {
		t.Fatal(err)
	}
	// Join claude to group 2 on home-a and codex to group 3 on home-b.
	if err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup2, Home: "home-a", Agent: "claude", Name: "Claude Backend", Secretary: false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup3, Home: "home-b", Agent: "codex", Name: "Codex Docs", Secretary: false,
	}); err != nil {
		t.Fatal(err)
	}

	// Exactly one Connect per distinct home.
	if rig.dialCount("home-a") != 1 || rig.dialCount("home-b") != 1 {
		t.Fatalf("want 1 dial each, got home-a=%d home-b=%d", rig.dialCount("home-a"), rig.dialCount("home-b"))
	}
	// Cached group row on second join of group 1 skipped a second sync for group 1.
	if len(fhA.syncs) != 2 || !fhA.syncs[0].StateOnly || fhA.syncs[0].Since != 0 {
		t.Fatalf("unexpected syncs on home-a: %+v", fhA.syncs)
	}

	// Verify MemberAddAI wire fields.
	if len(fhA.addAIs) != 3 {
		t.Fatalf("want 3 add_ai commands on home-a, got %d", len(fhA.addAIs))
	}
	firstAdd := fhA.addAIs[0]
	if firstAdd.Type != "member" || firstAdd.Op != "add_ai" || firstAdd.Group != joinTestGroup1 || firstAdd.As != "" ||
		firstAdd.ID == "" || firstAdd.Nonce == "" || firstAdd.ID == firstAdd.Nonce ||
		firstAdd.CLI != "claude" || firstAdd.Name != "Claude Sec" || firstAdd.Account != "personal" ||
		!firstAdd.Secretary || !slices.Equal(firstAdd.Approvers, []string{joinTestOwner}) {
		t.Fatalf("unexpected first MemberAddAI: %+v", firstAdd)
	}

	// Verify secretary demotion in group 1 and preservation in group 3.
	sClaudeG1, ok := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: string(firstAdd.ID)})
	if !ok || sClaudeG1.Secretary || sClaudeG1.Kind != GroupSession || sClaudeG1.Folder != "repo" || sClaudeG1.State != Dormant {
		t.Fatalf("expected claude in g1 demoted and dormant, got %+v (ok=%v)", sClaudeG1, ok)
	}
	sCodexG1, ok := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: string(fhA.addAIs[1].ID)})
	if !ok || !sCodexG1.Secretary || sCodexG1.Agent != "codex" {
		t.Fatalf("expected codex in g1 as secretary, got %+v (ok=%v)", sCodexG1, ok)
	}
	sClaudeG3, ok := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup3), Member: "existing-sec"})
	if !ok || !sClaudeG3.Secretary {
		t.Fatalf("expected claude in g3 to remain secretary, got %+v (ok=%v)", sClaudeG3, ok)
	}

	// Verify setting 'homes' has one entry per group with updated name for group 1.
	savedHomes, err := joinSavedHomes(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	wantHomes := []proto.Home{
		{Group: joinTestGroup1, Home: "home-a", Name: "Website v2"},
		{Group: joinTestGroup2, Home: "home-a", Name: "Backend"},
		{Group: joinTestGroup3, Home: "home-b", Name: "Docs"},
	}
	if !slices.Equal(savedHomes, wantHomes) {
		t.Fatalf("homes setting = %+v, want %+v", savedHomes, wantHomes)
	}

	// Verify audit rows recorded AuditSetting with By device.
	rec := seamRecord(t, rig.h)
	if !strings.Contains(rec, "|"+string(joinTestGroup1)+"|claude|setting|-|-|host.join Claude Sec|"+joinTestByDev+"|-") {
		t.Fatalf("missing audit row in:\n%s", rec)
	}
}

func TestHostJoinApproversCombinations(t *testing.T) {
	cases := []struct {
		name      string
		owner     string
		approvers []string
		hasList   bool
		want      []string
	}{
		{name: "none", want: []string{}},
		{name: "owner-only", owner: "u-owner", want: []string{"u-owner"}},
		{name: "delegates-without-owner", approvers: []string{"u-del-1", "u-del-2"}, hasList: true, want: []string{"u-del-1", "u-del-2"}},
		{name: "owner-and-dedup-delegates", owner: "u-owner", approvers: []string{"u-del-1", "u-owner", "u-del-2"}, hasList: true, want: []string{"u-owner", "u-del-1", "u-del-2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newJoinRig(t)
			rig.setAgent("claude", "2.1.311", strPtr("team"))
			if tc.owner != "" {
				rig.setSetting("owner", tc.owner)
			}
			if tc.hasList {
				rig.setSetting("approvers", tc.approvers)
			}
			fh := rig.addHome("home-1")
			repo := rig.makeRepo("repo", "main")
			fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "App", repo, "main"))

			if err := rig.callJoin(t.Context(), proto.HostJoin{
				Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
			}); err != nil {
				t.Fatal(err)
			}
			if len(fh.addAIs) != 1 || fh.addAIs[0].Approvers == nil || !slices.Equal(fh.addAIs[0].Approvers, tc.want) {
				t.Fatalf("got approvers %#v, want %#v", fh.addAIs[0].Approvers, tc.want)
			}
		})
	}
}

func TestHostJoinAttachModesWaitersAndCancellation(t *testing.T) {
	// 1. Connect == nil with pre-attached Home succeeds; Connect == nil without Home fails.
	rigNil := newJoinRig(t)
	rigNil.h.Connect = nil
	rigNil.setAgent("claude", "2.1.311", strPtr("personal"))
	rigNil.setAgent("codex", "0.164.0", strPtr("personal"))
	seamCode(t, rigNil.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "unattached", Agent: "claude", Name: "Claude",
	}), "home_unreachable")

	fhPre := newJoinFakeHome(rigNil.h, "pre-attached")
	fhPre.putGroup(makeGroupRow(joinTestGroup1, "pre-attached", "Pre", rigNil.makeRepo("pre", "main"), "main"))
	rigNil.h.Attach("pre-attached", fhPre)
	if err := rigNil.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "pre-attached", Agent: "claude", Name: "Claude",
	}); err != nil {
		t.Fatalf("pre-attached join failed: %v", err)
	}

	// Attached notification when nobody waits is a harmless no-op.
	joinAttached(rigNil.h, "nobody-waiting")

	// 2. Async Attach: two concurrent joins wait on one home; first times out while second waits,
	// keeping the home's Connect alive until Attach arrives (including a stale closed Home first).
	rigAsync := newJoinRig(t)
	rigAsync.setAgent("claude", "2.1.311", strPtr("personal"))
	rigAsync.setAgent("codex", "0.164.0", strPtr("personal"))
	repo := rigAsync.makeRepo("async", "main")
	fhLive := newJoinFakeHome(rigAsync.h, "home-async")
	fhLive.putGroup(makeGroupRow(joinTestGroup1, "home-async", "Async", repo, "main"))

	dialed := make(chan context.Context, 2)
	rigAsync.h.Connect = func(ctx context.Context, device string) {
		rigAsync.mu.Lock()
		rigAsync.dials = append(rigAsync.dials, joinDialRecord{device: device, ctx: ctx})
		rigAsync.mu.Unlock()
		dialed <- ctx
	}

	shortCtx, cancelShort := context.WithCancel(t.Context())
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelWait()
	err1Ch := make(chan error, 1)
	err2Ch := make(chan error, 1)
	go func() {
		err1Ch <- rigAsync.callJoin(shortCtx, proto.HostJoin{
			Group: joinTestGroup1, Home: "home-async", Agent: "claude", Name: "Claude",
		})
	}()
	connCtx := <-dialed

	go func() {
		err2Ch <- rigAsync.callJoin(waitCtx, proto.HostJoin{
			Group: joinTestGroup1, Home: "home-async", Agent: "codex", Name: "Codex",
		})
	}()
	waitWaiterCount(t, joinStateOf(rigAsync.h), "home-async", 2)

	// Cancel only the first join; the second still pins home-async so connCtx must stay open.
	cancelShort()
	seamCode(t, <-err1Ch, "home_unreachable")
	if connCtx.Err() != nil {
		t.Fatal("connCtx cancelled while second join was still waiting")
	}

	// Attach an already-closed connection first so joinAwaitAttach loops once, then attach live Home.
	closedHome := newJoinFakeHome(rigAsync.h, "home-async")
	close(closedHome.done)
	rigAsync.h.Attach("home-async", closedHome)
	rigAsync.h.Attach("home-async", fhLive)

	if err := <-err2Ch; err != nil {
		t.Fatalf("second async join failed: %v", err)
	}
	if rigAsync.dialCount("home-async") != 1 || connCtx.Err() != nil {
		t.Fatalf("expected 1 dial and live connCtx, dials=%d err=%v", rigAsync.dialCount("home-async"), connCtx.Err())
	}

	// 3. Single join whose Attach times out cancels the unused Connect context.
	timeoutCtx, cancelTimeout := context.WithCancel(t.Context())
	cancelTimeout()
	seamCode(t, rigAsync.callJoin(timeoutCtx, proto.HostJoin{
		Group: joinTestGroup2, Home: "home-never", Agent: "claude", Name: "Claude",
	}), "home_unreachable")
	neverCtx := rigAsync.lastDialCtx("home-never")
	if neverCtx == nil || neverCtx.Err() == nil {
		t.Fatalf("expected home-never Connect context to be cancelled on failure, got %v", neverCtx)
	}
	jpAsync := joinStateOf(rigAsync.h)
	jpAsync.mu.Lock()
	remainingWaiters := len(jpAsync.waiters)
	jpAsync.mu.Unlock()
	if remainingWaiters != 0 {
		t.Fatalf("expected 0 remaining waiter entries, got %d", remainingWaiters)
	}
}

func waitWaiterCount(t *testing.T, jp *joinPart, device string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		jp.mu.Lock()
		n := len(jp.waiters[device])
		jp.mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d waiters on %s", want, device)
}

func TestHostJoinSyncWorkspaceAndAddAIFailures(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", strPtr("personal"))
	fh := rig.addHome("home-1")
	repo := rig.makeRepo("web", "main")

	// Corrupt or mismatched cached group row falls through to sync.
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(`{"r":"group","id":"other"}`))

	// 1. Sync transport error -> home_unreachable and Connect cancelled.
	fh.syncFn = func(proto.Sync) (json.RawMessage, error) { return nil, io.EOF }
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "home_unreachable")
	if rig.lastDialCtx("home-1").Err() == nil {
		t.Fatal("failed join must cancel unused home Connect")
	}

	// 2. Sync wire refusal -> propagates refusal code.
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		errResp := proto.ErrorResponse{Re: req.ID, Err: proto.WireError{Code: "forbidden", Message: "not allowed"}}
		return json.RawMessage(joinEncodeJSON(errResp)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "forbidden")

	// 3. Sync malformed JSON and mismatched group -> home_unreachable.
	fh.syncFn = func(proto.Sync) (json.RawMessage, error) { return json.RawMessage(`{"bad":1}`), nil }
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "home_unreachable")

	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		wrong := proto.Synced{Type: "synced", Success: proto.Success{Re: req.ID, OK: true}, Group: string(joinTestGroup2), Seq: 1}
		return json.RawMessage(joinEncodeJSON(wrong)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "home_unreachable")

	// 4. Sync succeeds without putting a group row -> not_found.
	rig.h.Rows.Reset(string(joinTestGroup1))
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		ok := proto.Synced{Type: "synced", Success: proto.Success{Re: req.ID, OK: true}, Group: req.Group, Seq: 1}
		return json.RawMessage(joinEncodeJSON(ok)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "not_found")
	fh.syncFn = nil

	// 5. Non-project group kind -> conflict; missing repo or base -> bad_request.
	teamGrp := makeGroupRow(joinTestGroup1, "home-1", "Team", "", "")
	teamGrp.Kind = "team"
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(joinEncodeJSON(teamGrp)))
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "conflict")

	noRepo := makeGroupRow(joinTestGroup1, "home-1", "NoRepo", "", "main")
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(joinEncodeJSON(noRepo)))
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "bad_request")

	noBase := makeGroupRow(joinTestGroup1, "home-1", "NoBase", repo, "")
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(joinEncodeJSON(noBase)))
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "bad_request")

	// 6. Clone failure with secret in path is answered cli with secret redacted.
	secretRepo := filepath.Join(rig.root, joinTestSecret, "missing.git")
	badClone := makeGroupRow(joinTestGroup1, "home-1", "SecretFail", secretRepo, "main")
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(joinEncodeJSON(badClone)))
	cloneErr := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	})
	seamCode(t, cloneErr, "cli")
	if strings.Contains(cloneErr.Error(), joinTestSecret) || !strings.Contains(cloneErr.Error(), "[redacted]") {
		t.Fatalf("expected secret redacted in cli error, got %q", cloneErr.Error())
	}

	// 7. Home refuses member add_ai -> error propagated, no session created.
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(joinEncodeJSON(makeGroupRow(joinTestGroup1, "home-1", "Web", repo, "main"))))
	fh.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
		ref := proto.ErrorResponse{Re: 7, Err: proto.WireError{Code: "conflict", Message: "name taken"}}
		return json.RawMessage(joinEncodeJSON(ref)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}), "conflict")
	if len(rig.h.Sessions.All()) != 0 {
		t.Fatalf("refused add_ai must create no session, got %+v", rig.h.Sessions.All())
	}
}

func TestHostJoinMemoryStoreAndEnsureHomeConnections(t *testing.T) {
	// 1. In-memory store fallback directory and reuse across joins on the same Host.
	memDB := seamStore(t)
	hMem, _ := seamHost(t, memDB)
	seamSQL(t, memDB, "INSERT INTO agent(agent, cli_version, account) VALUES('claude', '2.1.311', 'personal'), ('codex', '0.164.0', 'personal')")
	fh := newJoinFakeHome(hMem, "home-mem")
	hMem.Attach("home-mem", fh)

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "upstream")
	joinGit(t, root, "init", "-b", "main", repo)
	joinGit(t, repo, "commit", "--allow-empty", "-m", "initial")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-mem", "MemGroup", repo, "main"))

	t.Cleanup(func() {
		if dir := joinStateOf(hMem).memDir; dir != "" {
			os.RemoveAll(dir)
		}
	})

	joinPayload1 := proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: joinTestNonce1, Timestamp: seamStart, To: hMem.Key.ID()},
		Group:       joinTestGroup1, Home: "home-mem", Agent: "claude", Name: "Claude",
	}
	if _, err := Surface["host.join"](t.Context(), hMem, nil, joinTestByDev, json.RawMessage(joinEncodeJSON(joinPayload1))); err != nil {
		t.Fatal(err)
	}
	memDir := joinStateOf(hMem).memDir
	if !filepath.IsAbs(memDir) {
		t.Fatalf("expected absolute fallback dir for :memory: store, got %q", memDir)
	}
	if _, err := os.Stat(filepath.Join(memDir, "projects", string(joinTestGroup1), "repo", ".git")); err != nil {
		t.Fatalf("expected clone in fallback :memory: projects dir: %v", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	joinPayload2 := joinPayload1
	joinPayload2.Nonce = joinTestNonce2
	joinPayload2.Agent = "codex"
	joinPayload2.Name = "Codex"
	if _, err := Surface["host.join"](t.Context(), hMem, nil, joinTestByDev, json.RawMessage(joinEncodeJSON(joinPayload2))); err != nil {
		t.Fatalf("second join on :memory: host did not reuse fallback dir: %v", err)
	}

	// 2. Restarting a host on an existing store and calling EnsureHomeConnections.
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", strPtr("personal"))
	fh1 := rig.addHome("home-1")
	rig.addHome("home-2")
	fh1.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website", rig.makeRepo("r1", "main"), "main"))
	if err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}); err != nil {
		t.Fatal(err)
	}
	// Also store a second home in setting 'homes' without a local session (e.g. step 2 home.add).
	rig.setSetting("homes", []proto.Home{
		{Group: joinTestGroup1, Home: "home-1", Name: "Website"},
		{Group: joinTestGroup2, Home: "home-2", Name: "Shared"},
	})

	restarted, saved, err := New(rig.db, seamKey(), seamClock())
	if err != nil || len(saved) != 1 {
		t.Fatalf("restart New: saved=%v err=%v", saved, err)
	}
	var restartedDials []joinDialRecord
	restarted.Connect = func(ctx context.Context, device string) {
		restartedDials = append(restartedDials, joinDialRecord{device: device, ctx: ctx})
	}
	nodeCtx, stopNode := context.WithCancel(t.Context())
	if err := restarted.EnsureHomeConnections(nodeCtx); err != nil {
		t.Fatal(err)
	}
	// Calling EnsureHomeConnections again while already connected opens no duplicate connections.
	if err := restarted.EnsureHomeConnections(nodeCtx); err != nil {
		t.Fatal(err)
	}
	if len(restartedDials) != 2 || restartedDials[0].device != "home-1" || restartedDials[1].device != "home-2" {
		t.Fatalf("unexpected restarted dials: %+v", restartedDials)
	}
	stopNode()
	if restartedDials[0].ctx.Err() == nil || restartedDials[1].ctx.Err() == nil {
		t.Fatal("cancelling parent nodeCtx must cancel child home contexts")
	}
}

func TestHostLeaveValidationRefusalAndMultiSessionCleanup(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", strPtr("personal"))
	rig.setAgent("codex", "0.164.0", strPtr("personal"))
	fhA := rig.addHome("home-a")
	fhB := rig.addHome("home-b")
	fhA.putGroup(makeGroupRow(joinTestGroup1, "home-a", "Website", rig.makeRepo("w1", "main"), "main"))
	fhA.putGroup(makeGroupRow(joinTestGroup2, "home-a", "Backend", rig.makeRepo("w2", "main"), "main"))
	fhB.putGroup(makeGroupRow(joinTestGroup3, "home-b", "Docs", rig.makeRepo("w3", "main"), "main"))

	// Invalid payloads -> bad_request.
	for _, raw := range []string{
		`[]`,
		`{"t":"host.leave"}`,
		`{"t":"host.leave","n":"BEVDTkBah3oHfAzuxeLAZw","ts":1,"to":"d","g":"bad","agent":"claude"}`,
		`{"t":"host.leave","n":"BEVDTkBah3oHfAzuxeLAZw","ts":1,"to":"d","g":"rZNvy-1jH6Z-BcPqA5U5BQ","agent":""}`,
		`{"t":"host.leave","n":"BEVDTkBah3oHfAzuxeLAZw","ts":1,"to":"d","g":"rZNvy-1jH6Z-BcPqA5U5BQ","agent":"other"}`,
	} {
		_, err := Surface["host.leave"](t.Context(), rig.h, nil, joinTestByDev, json.RawMessage(raw))
		seamCode(t, err, "bad_request")
	}

	// Leaving a group the agent is not in -> not_found.
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "claude"), "not_found")

	// Join claude and codex to g1 (home-a), claude to g2 (home-a), and codex to g3 (home-b).
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "claude", Name: "Claude"}); err != nil {
		t.Fatal(err)
	}
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "codex", Name: "Codex"}); err != nil {
		t.Fatal(err)
	}
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup2, Home: "home-a", Agent: "claude", Name: "Claude B"}); err != nil {
		t.Fatal(err)
	}
	if err := rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup3, Home: "home-b", Agent: "codex", Name: "Codex D"}); err != nil {
		t.Fatal(err)
	}

	// Leaving g3 with claude (only codex is in g3, claude is in g1/g2) -> not_found.
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup3, "claude"), "not_found")

	claudeMember := string(fhA.addAIs[0].ID)
	codexMember := string(fhA.addAIs[1].ID)

	// Add a task session, an admitted token, and approval rows for claude and codex in g1.
	taskKey := SessionKey{Group: string(joinTestGroup1), Member: claudeMember, Task: "t-1"}
	if err := rig.h.Sessions.Create(Session{
		SessionKey: taskKey, Kind: TaskSession, Folder: "wt/1-fix", Agent: "claude", Home: "home-a", Name: "Claude",
	}); err != nil {
		t.Fatal(err)
	}
	tok := rig.h.Sessions.Admit(taskKey, 2, false)
	joinExecSQL(t, rig.db, "INSERT INTO approval(card, g, member, task, tool, input, state) VALUES('c1', ?, ?, 't-1', 'Bash', 'h1', 'pending'), ('c2', ?, ?, 't-2', 'Bash', 'h2', 'pending')",
		string(joinTestGroup1), claudeMember, string(joinTestGroup1), codexMember)

	// Refused member remove (unfinished task) keeps sessions, token, approvals, homes and connection intact.
	fhA.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
		ref := proto.ErrorResponse{Re: 7, Err: proto.WireError{Code: "conflict", Message: "unfinished task"}}
		return json.RawMessage(joinEncodeJSON(ref)), nil
	}
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "claude"), "conflict")
	if _, ok := rig.h.Sessions.Get(taskKey); !ok {
		t.Fatal("refused leave ended task session")
	}
	if _, ok := rig.h.Sessions.Caller(tok); !ok {
		t.Fatal("refused leave invalidated token")
	}
	if rig.lastDialCtx("home-a").Err() != nil {
		t.Fatal("refused leave cancelled home-a connection")
	}

	// Unreachable home during leave returns home_unreachable and preserves state.
	close(fhA.done)
	rig.h.Connect = nil
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "claude"), "home_unreachable")
	fhA = rig.addHome("home-a")
	rig.h.Connect = rig.connectSync
	rig.h.Attach("home-a", fhA)

	// Accepted leave for claude in g1: ends both group and task sessions of claude in g1,
	// invalidates token, deletes claude's approval c1 (keeping codex's c2), keeps g1 in homes (codex still in g1),
	// and keeps home-a connected.
	if err := rig.callLeave(t.Context(), joinTestGroup1, "claude"); err != nil {
		t.Fatalf("claude leave g1 failed: %v", err)
	}
	if len(fhA.removes) != 1 || fhA.removes[0].Op != "remove" || string(fhA.removes[0].ID) != claudeMember || fhA.removes[0].As != "" {
		t.Fatalf("unexpected remove command: %+v", fhA.removes)
	}
	if _, ok := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: claudeMember}); ok {
		t.Fatal("claude group session still exists")
	}
	if _, ok := rig.h.Sessions.Get(taskKey); ok {
		t.Fatal("claude task session still exists")
	}
	if _, ok := rig.h.Sessions.Caller(tok); ok {
		t.Fatal("ended session token still valid")
	}
	var remainingCards string
	seamSQLScan(t, rig.db, &remainingCards, "SELECT coalesce(group_concat(card), '') FROM approval")
	if remainingCards != "c2" {
		t.Fatalf("expected only c2 approval left, got %q", remainingCards)
	}
	homesWhileCodexInG1, err := joinSavedHomes(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	wantWhileCodexInG1 := []proto.Home{
		{Group: joinTestGroup1, Home: "home-a", Name: "Website"},
		{Group: joinTestGroup2, Home: "home-a", Name: "Backend"},
		{Group: joinTestGroup3, Home: "home-b", Name: "Docs"},
	}
	if !slices.Equal(homesWhileCodexInG1, wantWhileCodexInG1) {
		t.Fatalf("expected g1 kept in homes while codex remains, got %+v", homesWhileCodexInG1)
	}

	// Codex leaves g1: now no AI remains in g1, so g1 is pruned from setting 'homes',
	// while g2 still keeps home-a connected.
	if err := rig.callLeave(t.Context(), joinTestGroup1, "codex"); err != nil {
		t.Fatal(err)
	}
	homesAfterG1, err := joinSavedHomes(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	wantAfterG1 := []proto.Home{
		{Group: joinTestGroup2, Home: "home-a", Name: "Backend"},
		{Group: joinTestGroup3, Home: "home-b", Name: "Docs"},
	}
	if !slices.Equal(homesAfterG1, wantAfterG1) || rig.lastDialCtx("home-a").Err() != nil {
		t.Fatalf("unexpected state after g1 empty: homes=%+v ctxErr=%v", homesAfterG1, rig.lastDialCtx("home-a").Err())
	}

	// Claude leaves g2 (last group on home-a): home-a Connect context is cancelled, home-b stays open.
	if err := rig.callLeave(t.Context(), joinTestGroup2, "claude"); err != nil {
		t.Fatal(err)
	}
	if rig.lastDialCtx("home-a").Err() == nil {
		t.Fatal("expected home-a Connect context cancelled when last group on home-a left")
	}
	if rig.lastDialCtx("home-b").Err() != nil {
		t.Fatal("home-b Connect context must stay open while g3 is joined")
	}

	// Codex leaves g3 (last group on home-b): homes becomes [] and home-b Connect context is cancelled.
	if err := rig.callLeave(t.Context(), joinTestGroup3, "codex"); err != nil {
		t.Fatal(err)
	}
	finalHomes, err := joinSavedHomes(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalHomes) != 0 || rig.lastDialCtx("home-b").Err() == nil {
		t.Fatalf("expected empty homes and cancelled home-b, got homes=%+v err=%v", finalHomes, rig.lastDialCtx("home-b").Err())
	}
}

func seamSQLScan(t *testing.T, db *store.Store, dest *string, query string, args ...any) {
	t.Helper()
	read := func(tx *store.Tx) error {
		return tx.QueryRow(query, args...).Scan(dest)
	}
	if err := db.Write(read); err != nil {
		t.Fatal(err)
	}
}

func TestHostJoinAndLeaveStoreErrorsAndTransactionRollback(t *testing.T) {
	// 1. Corrupt setting 'homes' JSON fails EnsureHomeConnections, joinConnectAndWait and joinRemoveFromHome.
	rigCorrupt := newJoinRig(t)
	rigCorrupt.setAgent("claude", "2.1.311", strPtr("personal"))
	if err := rigCorrupt.h.Sessions.Create(Session{
		SessionKey: SessionKey{Group: string(joinTestGroup1), Member: "m1"},
		Kind:       GroupSession, Folder: "repo", Agent: "claude", Home: "home-1", Name: "Claude",
	}); err != nil {
		t.Fatal(err)
	}
	seamSQL(t, rigCorrupt.db, "INSERT INTO setting(key, value) VALUES('homes', 'not-json')")
	if err := rigCorrupt.h.EnsureHomeConnections(t.Context()); err == nil {
		t.Fatal("expected EnsureHomeConnections error on corrupt homes JSON")
	}
	if err := rigCorrupt.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup2, Home: "home-1", Agent: "claude", Name: "Claude",
	}); err == nil {
		t.Fatal("expected join error on corrupt homes JSON")
	}
	if err := rigCorrupt.callLeave(t.Context(), joinTestGroup1, "claude"); err == nil {
		t.Fatal("expected leave error on corrupt homes JSON")
	}

	// 2. Dropped setting table fails joinSavedHomes and joinLoadConfig.
	rigDrop := newJoinRig(t)
	seamSQL(t, rigDrop.db, "DROP TABLE setting")
	if err := rigDrop.h.EnsureHomeConnections(t.Context()); err == nil {
		t.Fatal("expected EnsureHomeConnections error when setting table dropped")
	}
	if err := rigDrop.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}); err == nil {
		t.Fatal("expected joinLoadConfig error when setting table dropped")
	}

	// 3. Transaction rollback in joinCommitJoined for each failure point during member add_ai.
	joinFailSteps := []struct {
		name         string
		hasSecretary bool
		mutateDB     func(*testing.T, *store.Store)
	}{
		{
			name:         "demote-secretary-update-fails",
			hasSecretary: true,
			mutateDB:     func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE session") },
		},
		{
			name:     "create-session-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE session") },
		},
		{
			name:     "upsert-homes-scan-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE setting") },
		},
		{
			name: "upsert-homes-unmarshal-fails",
			mutateDB: func(t *testing.T, db *store.Store) {
				seamSQL(t, db, "INSERT OR REPLACE INTO setting(key, value) VALUES('homes', '{bad')")
			},
		},
		{
			name:     "audit-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE audit") },
		},
	}
	for _, tc := range joinFailSteps {
		t.Run("join-"+tc.name, func(t *testing.T) {
			rig := newJoinRig(t)
			rig.setAgent("claude", "2.1.311", strPtr("personal"))
			rig.setAgent("codex", "0.164.0", strPtr("personal"))
			fh := rig.addHome("home-1")
			fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website", rig.makeRepo("r", "main"), "main"))
			if tc.hasSecretary {
				if err := rig.h.Sessions.Create(Session{
					SessionKey: SessionKey{Group: string(joinTestGroup1), Member: "sec-1"},
					Kind:       GroupSession, Folder: "repo", Agent: "codex", Home: "home-1", Name: "Sec", Secretary: true,
				}); err != nil {
					t.Fatal(err)
				}
			}
			fh.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
				tc.mutateDB(t, rig.db)
				ok := proto.CmdResult{Success: proto.Success{Re: 7, OK: true}}
				return json.RawMessage(joinEncodeJSON(ok)), nil
			}
			err := rig.callJoin(t.Context(), proto.HostJoin{
				Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude", Secretary: true,
			})
			if err == nil {
				t.Fatal("expected commit failure")
			}
			var bad *Error
			if errors.As(err, &bad) {
				t.Fatalf("expected raw store error for internal mapping, got *Error %v", err)
			}
			// Verify in-memory session table rolled back cleanly.
			if _, _, found := joinFindAgentMember(rig.h, string(joinTestGroup1), "claude"); found {
				t.Fatal("rolled-back join left claude session in memory")
			}
			if tc.hasSecretary {
				sec, ok := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: "sec-1"})
				if !ok || !sec.Secretary {
					t.Fatalf("rolled-back join did not restore prior secretary flag: %+v", sec)
				}
			}
		})
	}

	// 4. Transaction rollback in joinCommitLeft for each failure point during member remove.
	leaveFailSteps := []struct {
		name     string
		mutateDB func(*testing.T, *store.Store)
	}{
		{
			name:     "end-session-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE session") },
		},
		{
			name:     "delete-approval-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE approval") },
		},
		{
			name:     "prune-homes-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE setting") },
		},
		{
			name:     "audit-fails",
			mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE audit") },
		},
	}
	for _, tc := range leaveFailSteps {
		t.Run("leave-"+tc.name, func(t *testing.T) {
			rig := newJoinRig(t)
			rig.setAgent("claude", "2.1.311", strPtr("personal"))
			fh := rig.addHome("home-1")
			fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website", rig.makeRepo("r", "main"), "main"))
			if err := rig.callJoin(t.Context(), proto.HostJoin{
				Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
			}); err != nil {
				t.Fatal(err)
			}
			fh.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
				tc.mutateDB(t, rig.db)
				ok := proto.CmdResult{Success: proto.Success{Re: 7, OK: true}}
				return json.RawMessage(joinEncodeJSON(ok)), nil
			}
			if err := rig.callLeave(t.Context(), joinTestGroup1, "claude"); err == nil {
				t.Fatal("expected leave commit failure")
			}
			if _, _, found := joinFindAgentMember(rig.h, string(joinTestGroup1), "claude"); !found {
				t.Fatal("rolled-back leave removed claude session from memory")
			}
		})
	}
}

func TestHostJoinSignedEnvelopeVerifiesAsNodeCommand(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("codex", "0.164.0", strPtr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website", rig.makeRepo("r", "main"), "main"))

	var captured proto.Envelope
	fh.cmdFn = func(env proto.Envelope) (json.RawMessage, error) {
		captured = env
		ok := proto.CmdResult{Success: proto.Success{Re: 7, OK: true}}
		return json.RawMessage(joinEncodeJSON(ok)), nil
	}
	if err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "codex", Name: "Codex",
	}); err != nil {
		t.Fatal(err)
	}
	known := func(dev string) bool { return dev == rig.h.Key.ID() }
	hdr, err := identity.OpenCommand(captured, "", known, rig.h.Now())
	if err != nil || hdr.Type != "member" || hdr.Group != joinTestGroup1 {
		t.Fatalf("captured command envelope failed OpenCommand: hdr=%+v err=%v", hdr, err)
	}
}
