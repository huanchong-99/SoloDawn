package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
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
	joinTestByDev  = "owner-browser-device"
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
	states  []string
	// onStatus runs for each member status the host reports.
	onStatus func()
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
		return json.RawMessage(marshalWire(resp)), nil
	}
	f.host.Rows.Put(req.Group, "group", req.Group, json.RawMessage(marshalWire(grp)))
	synced := proto.Synced{
		Type:    "synced",
		Success: proto.Success{Re: req.ID, OK: true},
		Group:   req.Group,
		Seq:     grp.Seq,
	}
	return json.RawMessage(marshalWire(synced)), nil
}

func (f *joinFakeHome) handleCmd(req proto.Cmd) (json.RawMessage, error) {
	ok := json.RawMessage(marshalWire(proto.CmdResult{Success: proto.Success{Re: req.ID, OK: true}}))
	if isMemberStatusCmd(req.Env.Payload) {
		var st proto.MemberStatus[any]
		json.Unmarshal([]byte(req.Env.Payload), &st)
		f.mu.Lock()
		f.states = append(f.states, st.State)
		onStatus := f.onStatus
		f.mu.Unlock()
		if onStatus != nil {
			onStatus()
		}
		return ok, nil
	}
	f.mu.Lock()
	override := f.cmdFn
	f.mu.Unlock()
	if override != nil {
		return override(req.Env)
	}
	if err := f.recordMemberCmd(req.Env.Payload); err != nil {
		return nil, err
	}
	return ok, nil
}

func isMemberStatusCmd(payload string) bool {
	var hdr struct {
		Type string `json:"t"`
		Op   string `json:"op"`
	}
	return json.Unmarshal([]byte(payload), &hdr) == nil && hdr.Type == "member" && hdr.Op == "status"
}

func (f *joinFakeHome) recordMemberCmd(payload string) error {
	var base proto.MemberCommand
	if err := proto.Decode([]byte(payload), &base); err != nil {
		return err
	}
	if base.Op == "add_ai" {
		return f.recordAddAI(payload)
	}
	if base.Op == "remove" {
		f.mu.Lock()
		f.removes = append(f.removes, base)
		f.mu.Unlock()
	}
	return nil
}

func (f *joinFakeHome) recordAddAI(payload string) error {
	var add proto.MemberAddAI
	if err := proto.Decode([]byte(payload), &add); err != nil {
		return err
	}
	f.mu.Lock()
	f.addAIs = append(f.addAIs, add)
	f.mu.Unlock()
	return nil
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
	joinExecSQL(r.t, r.db, "INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)", key, marshalWire(val))
}

// folder makes a folder of that name under the rig's root and returns its
// path, which has no symbolic link.
func (r *joinRig) folder(name string) string {
	r.t.Helper()
	dir := filepath.Join(r.root, "work", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		r.t.Fatal(err)
	}
	return dir
}

func makeGroupRow(id proto.ID, home, name string) proto.GroupRow {
	return proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: string(id), Seq: 3},
		Name:           name,
		Home:           home,
		Pass:           90,
		PassFoundation: 70,
		Limits: proto.Limits{
			Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3,
			ReviewRounds: 5, AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
		},
		Created: seamStart,
	}
}

// callJoin joins with a folder of the group's own when the payload names none.
func (r *joinRig) callJoin(ctx context.Context, p proto.HostJoin) error {
	if p.Workspace == "" {
		p.Workspace = r.folder(string(p.Group))
	}
	fillJoinDefaults(&p, r.h)
	_, err := Surface["host.join"](ctx, r.h, nil, joinTestByDev, json.RawMessage(marshalWire(p)))
	return err
}

func fillJoinDefaults(p *proto.HostJoin, h *Host) {
	if p.Type == "" {
		p.Type = "host.join"
	}
	if p.Nonce == "" {
		p.Nonce = joinTestNonce1
	}
	if p.Timestamp == 0 {
		p.Timestamp = h.Now().UnixMilli()
	}
	if p.To == "" {
		p.To = h.Key.ID()
	}
}

func (r *joinRig) mustJoin(p proto.HostJoin) {
	r.t.Helper()
	if err := r.callJoin(r.t.Context(), p); err != nil {
		r.t.Fatal(err)
	}
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
	_, err := Surface["host.leave"](ctx, r.h, nil, joinTestByDev, json.RawMessage(marshalWire(p)))
	return err
}

func (r *joinRig) mustLeave(group proto.ID, agent string) {
	r.t.Helper()
	if err := r.callLeave(r.t.Context(), group, agent); err != nil {
		r.t.Fatal(err)
	}
}

func joinStr(s string) *string { return &s }

func joinAssertEqual[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", label, got, want)
	}
}

func joinAssertSlice[T comparable](t *testing.T, label string, got, want []T) {
	t.Helper()
	if got == nil || !slices.Equal(got, want) {
		t.Fatalf("%s: got %#v, want %#v", label, got, want)
	}
}

func joinAssertHomes(t *testing.T, db *store.Store, want []proto.Home) {
	t.Helper()
	got, err := joinSavedHomes(db)
	if err != nil {
		t.Fatal(err)
	}
	joinAssertSlice(t, "homes setting", got, want)
}

func TestHostJoinAndLeaveRegistrations(t *testing.T) {
	joinAssertEqual(t, "host.join registered", Surface["host.join"] != nil, true)
	joinAssertEqual(t, "host.leave registered", Surface["host.leave"] != nil, true)
	joinAssertEqual(t, "join attached registered", Attached["join"] != nil, true)
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
		seamCode(t, rig.callJoin(t.Context(), tc), "bad_request")
	}
	joinAssertEqual(t, "invalid payload dials", rig.dialCount("home-1"), 0)
}

func TestHostJoinExternal(t *testing.T) {
	rig := newJoinRig(t)
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	for _, bad := range []proto.HostJoin{
		{Group: joinTestGroup1, Home: "home-1", Agent: "external", Name: "Z"},
		{Group: joinTestGroup1, Home: "home-1", Agent: "external", Name: "Z", Account: "free"},
		{Group: joinTestGroup1, Home: "home-1", Agent: "external", Name: "Z", Account: "team", Secretary: true},
	} {
		seamCode(t, rig.callJoin(t.Context(), bad), "bad_request")
	}
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "external", Name: "Z", Account: "team", Tool: joinStr("ZCode")})
	fh.mu.Lock()
	joinAssertEqual(t, "reported state", strings.Join(fh.states, ","), "offline")
	fh.mu.Unlock()
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "external", Name: "Y", Account: "api_key"})
	add := fh.addAIs[0]
	joinAssertEqual(t, "add_ai cli", add.CLI, "external")
	joinAssertEqual(t, "add_ai tool", *add.Tool, "ZCode")
	joinAssertEqual(t, "add_ai account", add.Account, "team")
	joinAssertEqual(t, "second add_ai tool", fh.addAIs[1].Tool, (*string)(nil))
	s, _ := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: string(add.ID)})
	joinAssertEqual(t, "session", s, Session{SessionKey: s.SessionKey, Kind: GroupSession, Folder: rig.folder(string(joinTestGroup1)), Agent: "external", Home: "home-1", Name: "Z", State: Dormant, LastActive: s.LastActive})
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "external"), "bad_request")
}

// host.join judges the folder first: bad_request for a path that is not
// absolute, not_found where no folder is now. It keeps the folder with its
// symbolic links resolved, in member add_ai and in the group session's row
// (contracts/commands.md section 2).
func TestHostJoinResolvesTheWorkspace(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	real := rig.folder("website")
	file := filepath.Join(rig.root, "notes.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	join := func(workspace string) error {
		return rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude", Workspace: workspace})
	}
	for workspace, code := range map[string]string{
		"website":                            "bad_request",
		filepath.Join("work", "website"):     "bad_request",
		filepath.Join(rig.root, "missing"):   "not_found",
		filepath.Join(file, "below-a-file"):  "not_found",
		file:                                 "not_found",
		filepath.Join(real, "..", "missing"): "not_found",
	} {
		err := join(workspace)
		seamCode(t, err, code)
		joinAssertEqual(t, "the answer names "+workspace, strings.Contains(err.Error(), workspace), true)
	}
	joinAssertEqual(t, "a refused folder dials", rig.dialCount("home-1"), 0)
	joinAssertEqual(t, "a refused folder leaves a session", len(rig.h.Sessions.All()), 0)

	joinAssertEqual(t, "join by another spelling", join(filepath.Join(real, "..", "website")+string(filepath.Separator)), nil)
	fh.mu.Lock()
	add := fh.addAIs[0]
	fh.mu.Unlock()
	s, _ := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: string(add.ID)})
	joinAssertEqual(t, "add_ai workspace", add.Workspace, real)
	joinAssertEqual(t, "session folder", s.Folder, real)
}

// A folder is the workspace of AIs of one group only: host.join answers
// conflict, naming the group, for the folder of an AI of another group on
// this node, whichever home that group has. Inside a group the folder is
// shared, and it is free again once its AIs have left.
func TestHostJoinRefusesTheFolderOfAnotherGroup(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("personal"))
	fhA, fhB := rig.addHome("home-a"), rig.addHome("home-b")
	fhA.putGroup(makeGroupRow(joinTestGroup1, "home-a", "Website"))
	fhB.putGroup(makeGroupRow(joinTestGroup2, "home-b", "Backend"))
	shared := rig.folder("shared")
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "claude", Name: "Claude", Workspace: shared})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "codex", Name: "Codex", Workspace: shared})

	other := proto.HostJoin{Group: joinTestGroup2, Home: "home-b", Agent: "claude", Name: "Claude", Workspace: shared}
	err := rig.callJoin(t.Context(), other)
	joinAssertEqual(t, "another group", err.Error(), `conflict: the folder is the workspace of an AI of the group "Website"`)
	rig.h.Rows.Reset(string(joinTestGroup1))
	err = rig.callJoin(t.Context(), other)
	joinAssertEqual(t, "another group whose rows are not read", err.Error(), `conflict: the folder is the workspace of an AI of the group "`+string(joinTestGroup1)+`"`)
	joinAssertEqual(t, "add_ai for a refused folder", len(fhB.addAIs), 0)
	joinAssertEqual(t, "sessions in the other group", joinGroupHasSession(rig.h, string(joinTestGroup2)), false)

	rig.mustLeave(joinTestGroup1, "claude")
	seamCode(t, rig.callJoin(t.Context(), other), "conflict")
	rig.mustLeave(joinTestGroup1, "codex")
	rig.mustJoin(other)
	joinAssertEqual(t, "the folder after its group left", fhB.addAIs[0].Workspace, shared)
}

// The check of the folder and the writing of the group session's row are
// under one lock, held across member add_ai, so two joins for one folder and
// two groups cannot both pass.
func TestHostJoinsRunOneAtATime(t *testing.T) {
	rig := newJoinRig(t)
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	fh.putGroup(makeGroupRow(joinTestGroup2, "home-1", "Backend"))
	jp := joinStateOf(rig.h)
	free := func() bool {
		if !jp.joining.TryLock() {
			return false
		}
		jp.joining.Unlock()
		return true
	}
	var duringAdd, afterRow, rowWritten bool
	fh.cmdFn = func(env proto.Envelope) (json.RawMessage, error) {
		duringAdd = !free()
		return json.RawMessage(marshalWire(proto.CmdResult{Success: proto.Success{Re: 7, OK: true}})), fh.recordMemberCmd(env.Payload)
	}
	// An external AI reports its state once its row is written.
	fh.onStatus = func() {
		rowWritten = len(rig.h.Sessions.All()) == 1
		afterRow = !free()
	}
	shared := rig.folder("shared")
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "external", Name: "Z", Account: "team", Workspace: shared})
	if !duringAdd || !rowWritten || !afterRow || !free() {
		t.Fatalf("the lock: held during add_ai %v, row written %v and still held %v, free after the join %v", duringAdd, rowWritten, afterRow, free())
	}

	// Two joins at once for one folder and two groups: one succeeds.
	fh.mu.Lock()
	fh.cmdFn, fh.onStatus, fh.addAIs = nil, nil, nil
	fh.mu.Unlock()
	both := rig.folder("both")
	results := make(chan error, 2)
	for _, group := range []proto.ID{joinTestGroup1, joinTestGroup2} {
		go func() {
			results <- rig.callJoin(t.Context(), proto.HostJoin{Group: group, Home: "home-1", Agent: "external", Name: "Y", Account: "team", Workspace: both})
		}()
	}
	first, second := <-results, <-results
	if first != nil {
		first, second = second, first
	}
	joinAssertEqual(t, "one of two joins", first, nil)
	seamCode(t, second, "conflict")
	fh.mu.Lock()
	joinAssertEqual(t, "add_ai of two joins at once", len(fh.addAIs), 1)
	fh.mu.Unlock()
}

func TestHostJoinPreconditions(t *testing.T) {
	rig := newJoinRig(t)
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	fh.putGroup(makeGroupRow(joinTestGroup2, "home-1", "Backend"))

	// Agent missing from agent table and agent with NULL account are refused without dialling.
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"}), "conflict")
	rig.setAgent("claude", "2.1.311", nil)
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"}), "conflict")
	joinAssertEqual(t, "undeclared account dials", rig.dialCount("home-1"), 0)

	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("team"))
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})

	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude Again"})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "codex", Name: "Codex"})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup2, Home: "home-1", Agent: "claude", Name: "Claude Backend"})
}

func TestHostJoinOneConnectionPerHomeAndNoProjectFolder(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("api_key"))
	fhA := setupMultiHomeGroups(t, rig)

	// First join on home-a: claude joins group 1 as secretary with default empty approvers.
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "claude", Name: "Claude Sec", Secretary: true})

	// Configure approvers, rename cached group 1 row, and join codex to group 1 as the new secretary.
	rig.setSetting("approvers", []string{"u-del-1", "u-del-2"})
	renamed := makeGroupRow(joinTestGroup1, "home-a", "Website v2")
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(marshalWire(renamed)))
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "codex", Name: "Codex Sec", Secretary: true})

	// Join claude to group 2 on home-a and codex to group 3 on home-b (with Secretary: false).
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup2, Home: "home-a", Agent: "claude", Name: "Claude Backend", Secretary: false})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup3, Home: "home-b", Agent: "codex", Name: "Codex Docs", Secretary: false})

	assertMultiJoinOutcomes(t, rig, fhA)
}

func setupMultiHomeGroups(t *testing.T, rig *joinRig) *joinFakeHome {
	t.Helper()
	fhA := rig.addHome("home-a")
	fhB := rig.addHome("home-b")
	fhA.putGroup(makeGroupRow(joinTestGroup1, "home-a", "Website"))
	fhA.putGroup(makeGroupRow(joinTestGroup2, "home-a", "Backend"))
	fhB.putGroup(makeGroupRow(joinTestGroup3, "home-b", "Docs"))
	return fhA
}

func assertMultiJoinOutcomes(t *testing.T, rig *joinRig, fhA *joinFakeHome) {
	t.Helper()
	joinAssertEqual(t, "home-a dials", rig.dialCount("home-a"), 1)
	joinAssertEqual(t, "home-b dials", rig.dialCount("home-b"), 1)
	joinAssertEqual(t, "home-a sync count", len(fhA.syncs), 2)
	joinAssertEqual(t, "sync state_only", fhA.syncs[0].StateOnly, true)
	joinAssertEqual(t, "sync since", fhA.syncs[0].Since, int64(0))
	assertAddAICommands(t, fhA.addAIs)
	assertJoinedSessions(t, rig.h, string(fhA.addAIs[0].ID), string(fhA.addAIs[1].ID))
	if _, err := os.Stat(filepath.Join(rig.root, "projects")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the data directory holds a project folder: %v", err)
	}
	joinAssertHomes(t, rig.db, []proto.Home{
		{Group: joinTestGroup1, Home: "home-a", Name: "Website v2"},
		{Group: joinTestGroup2, Home: "home-a", Name: "Backend"},
		{Group: joinTestGroup3, Home: "home-b", Name: "Docs"},
	})
	rec := seamRecord(t, rig.h)
	wantAudit := "|" + string(joinTestGroup1) + "|claude|setting|-|-|host.join Claude Sec|" + joinTestByDev + "|-"
	joinAssertEqual(t, "audit row recorded", strings.Contains(rec, wantAudit), true)
}

func assertAddAICommands(t *testing.T, adds []proto.MemberAddAI) {
	t.Helper()
	joinAssertEqual(t, "add_ai count", len(adds), 3)
	first := adds[0]
	joinAssertEqual(t, "add_ai type", first.Type, "member")
	joinAssertEqual(t, "add_ai op", first.Op, "add_ai")
	joinAssertEqual(t, "add_ai group", first.Group, joinTestGroup1)
	joinAssertEqual(t, "add_ai as", string(first.As), "")
	joinAssertEqual(t, "add_ai distinct id and nonce", first.ID != "" && first.Nonce != "" && first.ID != first.Nonce, true)
	joinAssertEqual(t, "add_ai cli", first.CLI, "claude")
	joinAssertEqual(t, "add_ai name", first.Name, "Claude Sec")
	joinAssertEqual(t, "add_ai account", first.Account, "personal")
	joinAssertEqual(t, "add_ai secretary", first.Secretary, true)
	joinAssertEqual(t, "add_ai workspace", filepath.Base(first.Workspace), string(joinTestGroup1))
	joinAssertSlice(t, "default approvers", first.Approvers, []string{})
	joinAssertSlice(t, "configured approvers", adds[1].Approvers, []string{"u-del-1", "u-del-2"})
}

func assertJoinedSessions(t *testing.T, h *Host, claudeID, codexID string) {
	t.Helper()
	sClaudeG1, ok := h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: claudeID})
	joinAssertEqual(t, "claude g1 exists", ok, true)
	joinAssertEqual(t, "claude g1 kind", sClaudeG1.Kind, GroupSession)
	joinAssertEqual(t, "claude g1 folder", filepath.Base(sClaudeG1.Folder), string(joinTestGroup1))
	joinAssertEqual(t, "claude g1 state", sClaudeG1.State, Dormant)

	sCodexG1, ok := h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: codexID})
	joinAssertEqual(t, "codex g1 secretary", ok && sCodexG1.Secretary && sCodexG1.Agent == "codex", true)
}

func TestHostJoinAttachModesAndWaiters(t *testing.T) {
	checkNilConnectModes(t)
	checkConcurrentAsyncWaiters(t)
}

func checkNilConnectModes(t *testing.T) {
	t.Helper()
	rigNil := newJoinRig(t)
	rigNil.h.Connect = nil
	rigNil.setAgent("claude", "2.1.311", joinStr("personal"))
	seamCode(t, rigNil.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "unattached", Agent: "claude", Name: "Claude",
	}), "home_unreachable")

	fhPre := newJoinFakeHome(rigNil.h, "pre-attached")
	fhPre.putGroup(makeGroupRow(joinTestGroup1, "pre-attached", "Pre"))
	rigNil.h.Attach("pre-attached", fhPre)
	rigNil.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "pre-attached", Agent: "claude", Name: "Claude"})

	// Attached notification when nobody waits is a harmless no-op.
	joinAttached(rigNil.h, "nobody-waiting")
}

func checkConcurrentAsyncWaiters(t *testing.T) {
	t.Helper()
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("personal"))
	fhLive := newJoinFakeHome(rig.h, "home-async")
	fhLive.putGroup(makeGroupRow(joinTestGroup1, "home-async", "Async"))

	dialed := make(chan context.Context, 2)
	rig.h.Connect = func(ctx context.Context, device string) {
		rig.mu.Lock()
		rig.dials = append(rig.dials, joinDialRecord{device: device, ctx: ctx})
		rig.mu.Unlock()
		dialed <- ctx
	}

	shortCtx, cancelShort := context.WithCancel(t.Context())
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelWait()
	err1Ch := startAsyncJoin(shortCtx, rig, joinTestGroup1, "home-async", "claude", "Claude")
	connCtx := <-dialed
	err2Ch := startAsyncJoin(waitCtx, rig, joinTestGroup1, "home-async", "codex", "Codex")
	waitWaiterCount(t, joinStateOf(rig.h), "home-async", 2)

	// Cancel only the first join; the second still pins home-async so connCtx stays open.
	cancelShort()
	seamCode(t, <-err1Ch, "home_unreachable")
	joinAssertEqual(t, "connCtx open while second waits", connCtx.Err(), nil)

	// Attach an already-closed connection first so joinAwaitAttach loops once, then attach live Home.
	closedHome := newJoinFakeHome(rig.h, "home-async")
	close(closedHome.done)
	rig.h.Attach("home-async", closedHome)
	rig.h.Attach("home-async", fhLive)

	joinAssertEqual(t, "second async join err", <-err2Ch, nil)
	joinAssertEqual(t, "home-async dials", rig.dialCount("home-async"), 1)
	joinAssertEqual(t, "connCtx still open after join", connCtx.Err(), nil)
}

func startAsyncJoin(ctx context.Context, rig *joinRig, group proto.ID, home, agent, name string) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- rig.callJoin(ctx, proto.HostJoin{Group: group, Home: home, Agent: agent, Name: name})
	}()
	return ch
}

func waitWaiterCount(t *testing.T, jp *joinPart, device string, want int) {
	t.Helper()
	poll(t, 5*time.Second, fmt.Sprintf("%d waiters on %s", want, device), func() string {
		jp.mu.Lock()
		defer jp.mu.Unlock()
		if n := len(jp.waiters[device]); n != want {
			return fmt.Sprintf("%d waiters", n)
		}
		return ""
	})
}

func TestHostJoinAttachTimeout15Seconds(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		db, h := openJoinBubbleHost(t, root)
		defer db.Close()

		var dialedCtx context.Context
		h.Connect = func(ctx context.Context, _ string) {
			dialedCtx = ctx
		}
		payload := proto.HostJoin{Group: joinTestGroup1, Home: "home-off", Agent: "claude", Name: "Claude", Workspace: root}
		fillJoinDefaults(&payload, h)
		raw := json.RawMessage(marshalWire(payload))

		errCh := make(chan error, 1)
		go func() {
			_, callErr := Surface["host.join"](context.Background(), h, nil, joinTestByDev, raw)
			errCh <- callErr
		}()
		synctest.Wait()

		// 1 ns before 15 s: still waiting for Attach, Connect context still open.
		time.Sleep(15*time.Second - time.Nanosecond)
		synctest.Wait()
		assertJoinPending(t, errCh)
		joinAssertEqual(t, "dialedCtx open before 15s", dialedCtx.Err(), nil)

		// At exact 15 s: answers home_unreachable, clears waiters, and cancels unused Connect context.
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		seamCode(t, <-errCh, "home_unreachable")
		joinAssertEqual(t, "dialedCtx cancelled at 15s", dialedCtx.Err() != nil, true)
		jp := joinStateOf(h)
		jp.mu.Lock()
		remainingWaiters := len(jp.waiters)
		jp.mu.Unlock()
		joinAssertEqual(t, "remaining waiters", remainingWaiters, 0)
	})
}

func openJoinBubbleHost(t *testing.T, root string) (*store.Store, *Host) {
	t.Helper()
	db, err := store.Open(filepath.Join(root, "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := New(db, seamKey(), seamClock())
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	joinExecSQL(t, db, "INSERT INTO agent(agent, cli_version, account) VALUES('claude', '2.1.311', 'personal')")
	return db, h
}

func assertJoinPending(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("join finished before 15s timeout: %v", err)
	default:
	}
}

func TestHostJoinSyncAndAddAIFailures(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-1")
	checkSyncFailureModes(t, rig, fh)

	// Home refuses member add_ai -> error propagated, no session created.
	webGrp := makeGroupRow(joinTestGroup1, "home-1", "Web")
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(marshalWire(webGrp)))
	fh.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
		ref := proto.ErrorResponse{Re: 7, Err: proto.WireError{Code: "conflict", Message: "name taken"}}
		return json.RawMessage(marshalWire(ref)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"}), "conflict")
	joinAssertEqual(t, "sessions after refused add_ai", len(rig.h.Sessions.All()), 0)
}

func checkSyncFailureModes(t *testing.T, rig *joinRig, fh *joinFakeHome) {
	t.Helper()
	joinReq := proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"}

	// Corrupt cached group row falls through to sync; transport error answers home_unreachable and cancels Connect.
	rig.h.Rows.Put(string(joinTestGroup1), "group", string(joinTestGroup1), json.RawMessage(`{"r":"group","id":"other"}`))
	fh.syncFn = func(proto.Sync) (json.RawMessage, error) { return nil, io.EOF }
	seamCode(t, rig.callJoin(t.Context(), joinReq), "home_unreachable")
	joinAssertEqual(t, "unused home Connect cancelled", rig.lastDialCtx("home-1").Err() != nil, true)

	// Sync wire refusal propagates refusal code.
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		errResp := proto.ErrorResponse{Re: req.ID, Err: proto.WireError{Code: "forbidden", Message: "not allowed"}}
		return json.RawMessage(marshalWire(errResp)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), joinReq), "forbidden")

	// Sync malformed JSON answers home_unreachable.
	fh.syncFn = func(proto.Sync) (json.RawMessage, error) { return json.RawMessage(`{"bad":1}`), nil }
	seamCode(t, rig.callJoin(t.Context(), joinReq), "home_unreachable")

	// Sync succeeds without putting a group row -> not_found; subsequent uncached join resets cursor to 0.
	rig.h.Rows.Reset(string(joinTestGroup1))
	var seenSince int64 = -1
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		seenSince = req.Since
		ok := proto.Synced{Type: "synced", Success: proto.Success{Re: req.ID, OK: true}, Group: req.Group, Seq: 9}
		return json.RawMessage(marshalWire(ok)), nil
	}
	seamCode(t, rig.callJoin(t.Context(), joinReq), "not_found")
	joinAssertEqual(t, "first sync since", seenSince, int64(0))
	seamCode(t, rig.callJoin(t.Context(), joinReq), "not_found")
	joinAssertEqual(t, "retry without cached group resets since to 0", seenSince, int64(0))
	fh.syncFn = nil
}

func TestHostEnsureHomeConnections(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh1 := rig.addHome("home-1")
	rig.addHome("home-2")
	fh1.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})

	// Store a second home in setting 'homes' alongside home-1.
	rig.setSetting("homes", []proto.Home{
		{Group: joinTestGroup1, Home: "home-1", Name: "Website"},
		{Group: joinTestGroup2, Home: "home-2", Name: "Shared"},
	})

	restarted, saved, err := New(rig.db, seamKey(), seamClock())
	joinAssertEqual(t, "restart New err", err, nil)
	joinAssertEqual(t, "restart saved count", len(saved), 1)

	var restartedDials []joinDialRecord
	restarted.Connect = func(ctx context.Context, device string) {
		restartedDials = append(restartedDials, joinDialRecord{device: device, ctx: ctx})
	}
	joinAssertEqual(t, "first EnsureHomeConnections", restarted.EnsureHomeConnections(), nil)
	// Calling EnsureHomeConnections again while already connected opens no duplicate connections.
	joinAssertEqual(t, "second EnsureHomeConnections", restarted.EnsureHomeConnections(), nil)

	joinAssertEqual(t, "restarted dial count", len(restartedDials), 2)
	joinAssertEqual(t, "first dial device", restartedDials[0].device, "home-1")
	joinAssertEqual(t, "first dial ctx open", restartedDials[0].ctx.Err(), nil)
	joinAssertEqual(t, "unhosted dial device", restartedDials[1].device, "home-2")
	joinAssertEqual(t, "unhosted dial ctx open", restartedDials[1].ctx.Err(), nil)
}

func TestRecordHomeKeepsAddressesAndMovesOnlyHostedGroup(t *testing.T) {
	rig := newJoinRig(t)
	h := rig.h
	old := proto.Home{Group: joinTestGroup1, Home: "old", Name: "Shared"}
	next := proto.Home{Group: joinTestGroup1, Home: "new", Name: "Shared"}
	addr := "wss://example.test/rv"
	joinAssertEqual(t, "remember unhosted group", h.RecordHome(old, []string{addr}), nil)
	joinAssertEqual(t, "unhosted dials", rig.dialCount(old.Home), 1)
	key := SessionKey{Group: string(old.Group), Member: "ai"}
	other := SessionKey{Group: string(joinTestGroup2), Member: "ai"}
	joinAssertEqual(t, "create session", h.Sessions.Create(Session{SessionKey: key, Home: old.Home, Kind: GroupSession}), nil)
	joinAssertEqual(t, "create another session", h.Sessions.Create(Session{SessionKey: other, Home: old.Home, Kind: GroupSession}), nil)
	joinAssertEqual(t, "connect hosts", h.EnsureHomeConnections(), nil)
	waiter := make(chan struct{}, 1)
	d := deliv(h)
	d.groupLocked(key.Group, old.Home).waiters = []chan struct{}{waiter}
	heldKey := aiGroupKey{group: key.Group, member: key.Member}
	otherKey := aiGroupKey{group: other.Group, member: other.Member}
	stale := verifiedItem{home: old.Home, group: key.Group, item: proto.Item{Item: 1}}
	unrelated := verifiedItem{home: old.Home, group: other.Group, item: proto.Item{Item: 2}}
	d.held[heldKey], d.held[otherKey] = []verifiedItem{stale}, []verifiedItem{unrelated}

	joinAssertEqual(t, "move hosted group", h.RecordHome(next, []string{addr}), nil)
	joinAssertEqual(t, "replacement dials", rig.dialCount(next.Home), 1)
	joinAssertEqual(t, "old dial cancelled", rig.lastDialCtx(old.Home).Err(), context.Canceled)
	select {
	case <-waiter:
	default:
		t.Fatal("previous sync waiter was not released")
	}
	fresh := verifiedItem{home: next.Home, group: key.Group, item: stale.item}
	d.held[heldKey] = enqueueHeld(d.held[heldKey], fresh)
	joinAssertEqual(t, "replacement held home", d.held[heldKey][0].home, next.Home)
	joinAssertEqual(t, "unrelated held home", d.held[otherKey][0].home, old.Home)
	moved, _ := h.Sessions.Get(key)
	left, _ := h.Sessions.Get(other)
	joinAssertEqual(t, "moved session", moved.Home, next.Home)
	joinAssertEqual(t, "other session", left.Home, old.Home)
	joinAssertEqual(t, "record same home again", h.RecordHome(next, nil), nil)
	joinAssertEqual(t, "no duplicate dial", rig.dialCount(next.Home), 1)
	homes, err := joinSavedHomes(rig.db)
	joinAssertEqual(t, "read homes", err, nil)
	joinAssertEqual(t, "saved home count", len(homes), 1)
	joinAssertEqual(t, "saved home", homes[0], next)
}

func TestRecordHomeRollsBackFailedWrites(t *testing.T) {
	for _, table := range []string{"setting", "peer", "session"} {
		t.Run(table, func(t *testing.T) {
			rig := newJoinRig(t)
			h := rig.h
			entry := proto.Home{Group: joinTestGroup1, Home: "new", Name: "Shared"}
			key := SessionKey{Group: string(entry.Group), Member: "ai"}
			joinAssertEqual(t, "create session", h.Sessions.Create(Session{SessionKey: key, Home: "old", Kind: GroupSession}), nil)
			seamSQL(t, rig.db, "DROP TABLE "+table)
			if err := h.RecordHome(entry, []string{"wss://example.test/rv"}); err == nil || !strings.Contains(err.Error(), "no such table: "+table) {
				t.Fatalf("record home error = %v, want missing %s", err, table)
			}
			held, _ := h.Sessions.Get(key)
			joinAssertEqual(t, "unchanged session", held.Home, "old")
			joinAssertEqual(t, "no replacement dial", rig.dialCount(entry.Home), 0)
			if table != "setting" {
				homes, err := joinSavedHomes(rig.db)
				joinAssertEqual(t, "read homes", err, nil)
				joinAssertEqual(t, "rolled-back homes", len(homes), 0)
			}
		})
	}
}

func TestRecordHomeClearsCachedUnhostedGroupOnlyOnReplacement(t *testing.T) {
	rig := newJoinRig(t)
	h := rig.h
	group := string(joinTestGroup1)
	old := proto.Home{Group: joinTestGroup1, Home: "old", Name: "Shared"}
	next := proto.Home{Group: joinTestGroup1, Home: "new", Name: "Shared"}
	joinAssertEqual(t, "remember old home", h.RecordHome(old, nil), nil)
	d := deliv(h)
	gs := d.groupLocked(group, old.Home)
	gs.live = true
	applyGroupRow(h, d, old.Home, group, json.RawMessage(marshalWire(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group, Seq: 4}, Home: old.Home,
	})))
	joinAssertEqual(t, "update same home", h.RecordHome(old, nil), nil)
	_, cached := h.Rows.Get(group, "group", group)
	joinAssertEqual(t, "cache retained", cached, true)
	joinAssertEqual(t, "cursor retained", gs.cursor, int64(4))
	joinAssertEqual(t, "replace home", h.RecordHome(next, nil), nil)
	_, cached = h.Rows.Get(group, "group", group)
	joinAssertEqual(t, "old cache cleared", cached, false)
	joinAssertEqual(t, "cursor reset", gs.cursor, int64(0))
	joinAssertEqual(t, "new sync home", gs.home, next.Home)
	joinAssertEqual(t, "unhosted replacement dials", rig.dialCount(next.Home), 1)
}

func TestHostLeaveValidationRefusalAndMultiSessionCleanup(t *testing.T) {
	rig, fhA := setupLeaveRig(t)
	checkInvalidAndMissingLeaves(t, rig)

	claudeMember := string(fhA.addAIs[0].ID)
	taskKey := SessionKey{Group: string(joinTestGroup1), Member: claudeMember, Task: "t-1"}
	otherGroupSameMember := SessionKey{Group: string(joinTestGroup2), Member: claudeMember, Task: "t-other"}
	tok := seedLeaveExtraSessions(t, rig, taskKey, otherGroupSameMember)

	fhA = checkLeaveRefusalAndUnreachable(t, rig, fhA, taskKey, tok)

	// Accepted leave for claude in g1 ends only g1 sessions of claudeMember and invalidates tok.
	rig.mustLeave(joinTestGroup1, "claude")
	assertClaudeLeftGroup1(t, rig, fhA, claudeMember, taskKey, otherGroupSameMember, tok)
	if err := rig.h.Sessions.End(otherGroupSameMember); err != nil {
		t.Fatal(err)
	}

	// Codex leaves g1: g1 is pruned from setting 'homes', while g2 keeps home-a connected.
	rig.mustLeave(joinTestGroup1, "codex")
	joinAssertHomes(t, rig.db, []proto.Home{
		{Group: joinTestGroup2, Home: "home-a", Name: "Backend"},
		{Group: joinTestGroup3, Home: "home-b", Name: "Docs"},
	})
	joinAssertEqual(t, "home-a open while g2 remains", rig.lastDialCtx("home-a").Err(), nil)

	// Claude leaves g2 (last group on home-a): home-a Connect cancelled, home-b stays open.
	rig.mustLeave(joinTestGroup2, "claude")
	joinAssertEqual(t, "home-a cancelled", rig.lastDialCtx("home-a").Err() != nil, true)
	joinAssertEqual(t, "home-b still open", rig.lastDialCtx("home-b").Err(), nil)

	// Codex leaves g3 (last group on home-b): homes becomes [] and home-b Connect is cancelled.
	rig.mustLeave(joinTestGroup3, "codex")
	joinAssertHomes(t, rig.db, []proto.Home{})
	joinAssertEqual(t, "home-b cancelled", rig.lastDialCtx("home-b").Err() != nil, true)
}

func setupLeaveRig(t *testing.T) (*joinRig, *joinFakeHome) {
	t.Helper()
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("personal"))
	fhA := rig.addHome("home-a")
	fhB := rig.addHome("home-b")
	fhA.putGroup(makeGroupRow(joinTestGroup1, "home-a", "Website"))
	fhA.putGroup(makeGroupRow(joinTestGroup2, "home-a", "Backend"))
	fhB.putGroup(makeGroupRow(joinTestGroup3, "home-b", "Docs"))
	return rig, fhA
}

func checkInvalidAndMissingLeaves(t *testing.T, rig *joinRig) {
	t.Helper()
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

	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "claude"), "not_found")
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "claude", Name: "Claude"})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-a", Agent: "codex", Name: "Codex"})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup2, Home: "home-a", Agent: "claude", Name: "Claude B"})
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup3, Home: "home-b", Agent: "codex", Name: "Codex D"})
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup3, "claude"), "not_found")
}

func seedLeaveExtraSessions(t *testing.T, rig *joinRig, taskKey, otherGroupKey SessionKey) string {
	t.Helper()
	err := rig.h.Sessions.Create(Session{
		SessionKey: taskKey, Kind: TaskSession, Folder: "wt/1-fix", Agent: "claude", Home: "home-a", Name: "Claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = rig.h.Sessions.Create(Session{
		SessionKey: otherGroupKey, Kind: TaskSession, Folder: "wt/2-fix", Agent: "other", Home: "home-a", Name: "Other",
	})
	if err != nil {
		t.Fatal(err)
	}
	return rig.h.Sessions.Admit(taskKey, 2, false)
}

func checkLeaveRefusalAndUnreachable(t *testing.T, rig *joinRig, fhA *joinFakeHome, taskKey SessionKey, tok string) *joinFakeHome {
	t.Helper()
	fhA.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
		ref := proto.ErrorResponse{Re: 7, Err: proto.WireError{Code: "conflict", Message: "unfinished task"}}
		return json.RawMessage(marshalWire(ref)), nil
	}
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "claude"), "conflict")
	_, hasTask := rig.h.Sessions.Get(taskKey)
	_, hasTok := rig.h.Sessions.Caller(tok)
	joinAssertEqual(t, "task session preserved on refused leave", hasTask, true)
	joinAssertEqual(t, "token preserved on refused leave", hasTok, true)
	joinAssertEqual(t, "home-a open on refused leave", rig.lastDialCtx("home-a").Err(), nil)

	// Unreachable home during leave returns home_unreachable and preserves state.
	close(fhA.done)
	rig.h.Connect = nil
	seamCode(t, rig.callLeave(t.Context(), joinTestGroup1, "claude"), "home_unreachable")
	freshHome := rig.addHome("home-a")
	rig.h.Connect = rig.connectSync
	rig.h.Attach("home-a", freshHome)
	return freshHome
}

func assertClaudeLeftGroup1(t *testing.T, rig *joinRig, fhA *joinFakeHome, claudeMember string, taskKey, otherGroupKey SessionKey, tok string) {
	t.Helper()
	joinAssertEqual(t, "remove count", len(fhA.removes), 1)
	joinAssertEqual(t, "remove op", fhA.removes[0].Op, "remove")
	joinAssertEqual(t, "remove id", string(fhA.removes[0].ID), claudeMember)
	joinAssertEqual(t, "remove as", string(fhA.removes[0].As), "")

	_, hasGrp := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: claudeMember})
	_, hasTask := rig.h.Sessions.Get(taskKey)
	_, hasOtherGroup := rig.h.Sessions.Get(otherGroupKey)
	_, hasTok := rig.h.Sessions.Caller(tok)
	joinAssertEqual(t, "claude group session ended", hasGrp, false)
	joinAssertEqual(t, "claude task session ended", hasTask, false)
	joinAssertEqual(t, "other group session with same member untouched", hasOtherGroup, true)
	joinAssertEqual(t, "ended session token invalidated", hasTok, false)

	joinAssertHomes(t, rig.db, []proto.Home{
		{Group: joinTestGroup1, Home: "home-a", Name: "Website"},
		{Group: joinTestGroup2, Home: "home-a", Name: "Backend"},
		{Group: joinTestGroup3, Home: "home-b", Name: "Docs"},
	})
}

func TestHostJoinAndLeaveStoreErrorsAndTransactionRollback(t *testing.T) {
	checkCorruptAndDroppedSettings(t)
	checkJoinCommitRollbacks(t)
	checkLeaveCommitRollbacks(t)
}

func checkCorruptAndDroppedSettings(t *testing.T) {
	t.Helper()
	rigCorrupt := newJoinRig(t)
	rigCorrupt.setAgent("claude", "2.1.311", joinStr("personal"))
	err := rigCorrupt.h.Sessions.Create(Session{
		SessionKey: SessionKey{Group: string(joinTestGroup1), Member: "m1"},
		Kind:       GroupSession, Folder: "repo", Agent: "claude", Home: "home-1", Name: "Claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	seamSQL(t, rigCorrupt.db, "INSERT INTO setting(key, value) VALUES('homes', 'not-json')")
	joinAssertEqual(t, "EnsureHomeConnections corrupt homes err", rigCorrupt.h.EnsureHomeConnections() != nil, true)
	joinAssertEqual(t, "join corrupt homes err", rigCorrupt.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup2, Home: "home-1", Agent: "claude", Name: "Claude",
	}) != nil, true)
	joinAssertEqual(t, "leave corrupt homes err", rigCorrupt.callLeave(t.Context(), joinTestGroup1, "claude") != nil, true)

	// Corrupt setting 'approvers' JSON fails joinLoadConfig.
	rigBadApprovers := newJoinRig(t)
	rigBadApprovers.setAgent("claude", "2.1.311", joinStr("personal"))
	seamSQL(t, rigBadApprovers.db, "INSERT INTO setting(key, value) VALUES('approvers', 'not-json')")
	joinAssertEqual(t, "join corrupt approvers err", rigBadApprovers.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}) != nil, true)

	// Dropped setting table fails EnsureHomeConnections and joinLoadConfig.
	rigDrop := newJoinRig(t)
	seamSQL(t, rigDrop.db, "DROP TABLE setting")
	joinAssertEqual(t, "EnsureHomeConnections dropped setting err", rigDrop.h.EnsureHomeConnections() != nil, true)
	joinAssertEqual(t, "join dropped setting err", rigDrop.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude",
	}) != nil, true)
}

func checkJoinCommitRollbacks(t *testing.T) {
	t.Helper()
	steps := []struct {
		name     string
		mutateDB func(*testing.T, *store.Store)
	}{
		{name: "create-session-fails", mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE session") }},
		{name: "upsert-homes-scan-fails", mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE setting") }},
		{name: "upsert-homes-unmarshal-fails", mutateDB: func(t *testing.T, db *store.Store) {
			seamSQL(t, db, "INSERT OR REPLACE INTO setting(key, value) VALUES('homes', '{bad')")
		}},
		{name: "audit-fails", mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE audit") }},
	}
	for _, tc := range steps {
		t.Run("join-"+tc.name, func(t *testing.T) {
			runJoinRollbackCase(t, tc.mutateDB)
		})
	}
}

func runJoinRollbackCase(t *testing.T, mutateDB func(*testing.T, *store.Store)) {
	t.Helper()
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	fh.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
		mutateDB(t, rig.db)
		ok := proto.CmdResult{Success: proto.Success{Re: 7, OK: true}}
		return json.RawMessage(marshalWire(ok)), nil
	}
	err := rig.callJoin(t.Context(), proto.HostJoin{
		Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude", Secretary: true,
	})
	assertRawStoreFailure(t, err)
	joinAssertEqual(t, "rolled-back claude absent", len(joinAgentMembers(rig.h, string(joinTestGroup1), "claude")), 0)
}

func assertRawStoreFailure(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected commit failure")
	}
	var bad *Error
	if errors.As(err, &bad) {
		t.Fatalf("expected raw store error, got *Error %v", err)
	}
}

func checkLeaveCommitRollbacks(t *testing.T) {
	t.Helper()
	steps := []struct {
		name     string
		mutateDB func(*testing.T, *store.Store)
	}{
		{name: "end-session-fails", mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE session") }},
		{name: "prune-homes-fails", mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE setting") }},
		{name: "audit-fails", mutateDB: func(t *testing.T, db *store.Store) { seamSQL(t, db, "DROP TABLE audit") }},
	}
	for _, tc := range steps {
		t.Run("leave-"+tc.name, func(t *testing.T) {
			runLeaveRollbackCase(t, tc.mutateDB)
		})
	}
}

func runLeaveRollbackCase(t *testing.T, mutateDB func(*testing.T, *store.Store)) {
	t.Helper()
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})
	fh.cmdFn = func(proto.Envelope) (json.RawMessage, error) {
		mutateDB(t, rig.db)
		ok := proto.CmdResult{Success: proto.Success{Re: 7, OK: true}}
		return json.RawMessage(marshalWire(ok)), nil
	}
	err := rig.callLeave(t.Context(), joinTestGroup1, "claude")
	assertRawStoreFailure(t, err)
	joinAssertEqual(t, "rolled-back leave preserved session", len(joinAgentMembers(rig.h, string(joinTestGroup1), "claude")), 1)
}

func TestHostJoinSignedEnvelopeVerifiesAsNodeCommand(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("codex", "0.164.0", joinStr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))

	var captured proto.Envelope
	fh.cmdFn = func(env proto.Envelope) (json.RawMessage, error) {
		captured = env
		ok := proto.CmdResult{Success: proto.Success{Re: 7, OK: true}}
		return json.RawMessage(marshalWire(ok)), nil
	}
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "codex", Name: "Codex"})
	known := func(dev string) bool { return dev == rig.h.Key.ID() }
	hdr, err := identity.OpenCommand(captured, "", known, rig.h.Now())
	joinAssertEqual(t, "OpenCommand err", err, nil)
	joinAssertEqual(t, "OpenCommand type", hdr.Type, "member")
	joinAssertEqual(t, "OpenCommand group", hdr.Group, joinTestGroup1)
}

func TestLeaveGoneMemberClosesAndPrunesIdempotently(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude"})
	member := string(fh.addAIs[0].ID)
	rig.setSetting("homes", []proto.Home{
		{Group: joinTestGroup1, Home: "home-1", Name: "Website"},
		{Group: joinTestGroup2, Home: "home-1", Name: "Untouched"},
	})
	auditsBefore := countAuditWhat(t, rig.db, AuditSetting)

	// Member with no sessions on this host is a no-op and keeps stored homes intact (even for a group with zero sessions).
	leaveGoneMember(rig.h, string(joinTestGroup2), "not-hosted-here")
	joinAssertHomes(t, rig.db, []proto.Home{
		{Group: joinTestGroup1, Home: "home-1", Name: "Website"},
		{Group: joinTestGroup2, Home: "home-1", Name: "Untouched"},
	})

	// Gone local member ends sessions, prunes only its empty group from homes, and writes no AuditSetting row.
	rig.setSetting("homes", []proto.Home{{Group: joinTestGroup1, Home: "home-1", Name: "Website"}})
	leaveGoneMember(rig.h, string(joinTestGroup1), member)
	joinAssertEqual(t, "remaining sessions", len(rig.h.Sessions.All()), 0)
	joinAssertHomes(t, rig.db, []proto.Home{})
	joinAssertEqual(t, "home-1 cancelled", rig.lastDialCtx("home-1").Err() != nil, true)
	joinAssertEqual(t, "no AuditSetting written for gone member", countAuditWhat(t, rig.db, AuditSetting), auditsBefore)
	leaveGoneMember(rig.h, string(joinTestGroup1), member)
}

func TestHostJoinSyncCoordinatesWithDeliveryPushesAndInflightSync(t *testing.T) {
	checkJoinPushResetAndRowsWithoutDuplicateSync(t)
	checkJoinWaitsOnInflightGroupSync(t)
}

func checkJoinPushResetAndRowsWithoutDuplicateSync(t *testing.T) {
	t.Helper()
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-local")
	grp := makeGroupRow(joinTestGroup1, "home-local", "Website")
	gid := string(joinTestGroup1)

	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		rst := proto.Reset{Type: "reset", Group: gid}
		rig.h.Push("home-local", json.RawMessage(marshalWire(rst)))
		batch := proto.Rows{Type: "rows", Group: gid, Rows: []json.RawMessage{json.RawMessage(marshalWire(grp))}}
		rig.h.Push("home-local", json.RawMessage(marshalWire(batch)))
		ok := proto.Synced{Type: "synced", Success: proto.Success{Re: req.ID, OK: true}, Group: gid, Seq: grp.Seq}
		return json.RawMessage(marshalWire(ok)), nil
	}
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-local", Agent: "claude", Name: "Claude"})

	fh.mu.Lock()
	syncCount := len(fh.syncs)
	fh.mu.Unlock()
	joinAssertEqual(t, "single sync during pushRows", syncCount, 1)

	d := deliv(rig.h)
	d.mu.Lock()
	gs := d.groups[gid]
	live, syncing, cursor := gs.live, gs.syncing, gs.cursor
	d.mu.Unlock()
	joinAssertEqual(t, "group marked live", live, true)
	joinAssertEqual(t, "group not syncing", syncing, false)
	joinAssertEqual(t, "group cursor", cursor, grp.Seq)
}

func checkJoinWaitsOnInflightGroupSync(t *testing.T) {
	t.Helper()
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	rig.setAgent("codex", "0.164.0", joinStr("personal"))
	fh := rig.addHome("home-1")
	rig.h.Attach("home-1", fh)
	grp1 := makeGroupRow(joinTestGroup1, "home-1", "Website")
	grp2 := makeGroupRow(joinTestGroup2, "home-1", "Backend")
	fh.putGroup(grp1)
	fh.putGroup(grp2)

	// Group 1: background sync fails while two joins wait; cancelled join removes only its waiter, surviving join retries and succeeds.
	runInflightSyncRetryCase(t, rig, fh, string(joinTestGroup1))
	// Group 2: background sync populates h.Rows while join waits; join wakes and reuses cached row without a second sync.
	runInflightSyncCacheHitCase(t, rig, fh, grp2)
}

func runInflightSyncRetryCase(t *testing.T, rig *joinRig, fh *joinFakeHome, gid string) {
	t.Helper()
	syncEntered := make(chan struct{}, 1) // single reader: <-syncEntered below
	releaseFirst := make(chan struct{})   // single reader: <-releaseFirst in syncFn
	fh.mu.Lock()
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		select {
		case syncEntered <- struct{}{}:
			<-releaseFirst
			return nil, io.EOF
		default:
			fh.mu.Lock()
			fh.syncFn = nil
			fh.mu.Unlock()
			return fh.handleSync(req)
		}
	}
	fh.mu.Unlock()
	rig.h.Push("home-1", json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: gid, Rows: []json.RawMessage{}})))
	<-syncEntered

	cancelCtx, cancel := context.WithCancel(t.Context())
	err1Ch := startAsyncJoin(cancelCtx, rig, joinTestGroup1, "home-1", "claude", "Claude")
	err2Ch := startAsyncJoin(t.Context(), rig, joinTestGroup1, "home-1", "codex", "Codex")
	waitGroupSyncWaiters(t, deliv(rig.h), gid, 2)

	cancel()
	seamCode(t, <-err1Ch, "home_unreachable")
	waitGroupSyncWaiters(t, deliv(rig.h), gid, 1)

	close(releaseFirst)
	joinAssertEqual(t, "surviving join succeeds on retry", <-err2Ch, nil)
}

func runInflightSyncCacheHitCase(t *testing.T, rig *joinRig, fh *joinFakeHome, grp proto.GroupRow) {
	t.Helper()
	gid := grp.ID
	fh.mu.Lock()
	before := len(fh.syncs)
	syncEntered := make(chan struct{}, 1) // single reader: <-syncEntered below
	release := make(chan struct{})        // single reader: <-release in syncFn
	fh.syncFn = func(req proto.Sync) (json.RawMessage, error) {
		syncEntered <- struct{}{}
		<-release
		rig.h.Rows.Put(gid, "group", gid, json.RawMessage(marshalWire(grp)))
		ok := proto.Synced{Type: "synced", Success: proto.Success{Re: req.ID, OK: true}, Group: gid, Seq: grp.Seq}
		return json.RawMessage(marshalWire(ok)), nil
	}
	fh.mu.Unlock()
	rig.h.Push("home-1", json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: gid, Rows: []json.RawMessage{}})))
	<-syncEntered

	errCh := startAsyncJoin(t.Context(), rig, joinTestGroup2, "home-1", "claude", "Claude")
	waitGroupSyncWaiters(t, deliv(rig.h), gid, 1)
	close(release)
	joinAssertEqual(t, "waiting join succeeds from populated cache", <-errCh, nil)

	fh.mu.Lock()
	after := len(fh.syncs)
	fh.syncFn = nil
	fh.mu.Unlock()
	joinAssertEqual(t, "no duplicate sync after inflight cache hit", after-before, 1)
}

func waitGroupSyncWaiters(t *testing.T, d *delivery, group string, want int) {
	t.Helper()
	poll(t, 5*time.Second, fmt.Sprintf("%d sync waiters on %s", want, group), func() string {
		d.mu.Lock()
		defer d.mu.Unlock()
		if n := len(d.groups[group].waiters); n != want {
			return fmt.Sprintf("%d sync waiters", n)
		}
		return ""
	})
}
