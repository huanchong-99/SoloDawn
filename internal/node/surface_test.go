package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestSurfaceDefaultsReadHelpersAndNulls(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("srf-node"), sesKey("srf-owner")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, home.Commands)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})
	sf := NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), Store: db, Sessions: s, Host: h,
		Agents: func(context.Context, bool) ([]proto.HostAgent, error) { return nil, nil },
	})

	if ReadName(db) != "" || len(ReadRendezvous(db)) != 0 {
		t.Fatal("expected empty defaults for ReadName and ReadRendezvous")
	}
	put := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('name', '"desk"'), ('rendezvous', '["wss://rv.example/rv"]'), ('unknown_key', '123')`)
		return err
	}
	if err := db.Write(put); err != nil {
		t.Fatal(err)
	}
	if ReadName(db) != "desk" || !slices.Equal(ReadRendezvous(db), []string{"wss://rv.example/rv"}) {
		t.Fatal("ReadName or ReadRendezvous mismatch")
	}

	origResize := host.Surface["pty.resize"]
	t.Cleanup(func() { host.Surface["pty.resize"] = origResize })
	var resizeSeen proto.PTYResize
	var resizeHost *host.Host
	var resizeBy string
	host.Surface["pty.resize"] = func(ctx context.Context, gotHost *host.Host, from host.Owner, by string, data json.RawMessage) (any, error) {
		resizeHost, resizeBy = gotHost, by
		if err := proto.Decode(data, &resizeSeen); err != nil {
			t.Errorf("decode pty.resize: %v", err)
		}
		return origResize(ctx, gotHost, from, by, data)
	}

	fc := serveConn(t, s, ownerKey.ID(), false)
	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 3, Kind: "browser", Name: "b", Addrs: []string{}})
	fc.next(t) // welcome
	fc.next(t) // devlog

	fc.push(proto.Request{Type: "host.info", ID: 2})
	rawInfo := fc.next(t)
	if nulls := proto.UnexpectedNulls(rawInfo, "/update", "/relay"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls in host.info: %v (%s)", nulls, rawInfo)
	}
	var info proto.HostInfo
	if err := proto.Decode(rawInfo, &info); err != nil {
		t.Fatal(err)
	}
	if info.Re != 2 || info.Device != nodeKey.ID() || info.Name != "desk" || info.OS == "" || info.Version != Version ||
		info.Open != 0 || info.MaxOpen != 4 || info.Offline || info.Update != nil || info.Relay != nil ||
		len(info.Homes) != 0 || len(info.Agents) != 0 ||
		info.Settings.RetentionDays != 90 || info.Settings.IdleMinutes != 15 || info.Settings.DefaultLevel != 3 ||
		info.Settings.AllLevel3 || len(info.Settings.Approvers) != 0 {
		t.Fatalf("default host.info mismatch: %+v", info)
	}

	// Creating a group on home.Loop pushes proto.Groups to the welcomed browser; host.info.homes reads setting('homes') only.
	gID := sesID("grp-1")
	createEnv := ownerKey.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Website","member_name":"Owner"}`, gID, now.UnixMilli(), gID))
	fc.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: createEnv})
	var groupsPush proto.Groups
	var sawCmdOK, sawGroups bool
	for range 2 {
		frame := fc.next(t)
		var g proto.Groups
		if proto.Decode(frame, &g) == nil && g.Type == "groups" {
			groupsPush, sawGroups = g, true
			continue
		}
		var ok proto.Success
		if proto.Decode(frame, &ok) == nil && ok.Re == 3 {
			sawCmdOK = true
		}
	}
	if !sawGroups || !sawCmdOK || len(groupsPush.Groups) != 1 || groupsPush.Groups[0].Group != string(gID) || groupsPush.Groups[0].Name != "Website" {
		t.Fatalf("groups push = %+v (sawGroups=%v sawCmdOK=%v)", groupsPush, sawGroups, sawCmdOK)
	}

	stateAfterGroup, err := sf.HostState(t.Context())
	if err != nil || len(stateAfterGroup.Homes) != 0 {
		t.Fatalf("homes without setting('homes') = %+v (%v)", stateAfterGroup.Homes, err)
	}

	aiID := sesID("ai-1")
	fc.push(proto.WatchStart{Request: proto.Request{Type: "watch.start", ID: 40}, Group: gID, Member: aiID})
	var watchErr proto.ErrorResponse
	if err := json.Unmarshal(fc.next(t), &watchErr); err != nil {
		t.Fatal(err)
	}
	if watchErr.Re != 40 || watchErr.Err.Code != "forbidden" {
		t.Fatalf("watch.start = %+v", watchErr)
	}
	// host.workspaces is the host's answer; an external AI has no list.
	fc.push(proto.HostWorkspacesRequest{Request: proto.Request{Type: "host.workspaces", ID: 41}, Agent: "external"})
	if got := string(fc.next(t)); got != `{"ok":true,"re":41,"workspaces":[]}` {
		t.Fatalf("host.workspaces = %s", got)
	}
	fc.push(proto.HostWorkspacesRequest{Request: proto.Request{Type: "host.workspaces", ID: 42}, Agent: "nano"})
	expectErr(t, fc.next(t), 42, "bad_request")
	fc.push(proto.PTYResize{Type: "pty.resize", Group: gID, AI: aiID, Cols: 120, Rows: 36})
	fc.push(proto.Request{Type: "host.info", ID: 4})
	fc.next(t)
	if resizeHost != h || resizeBy != ownerKey.ID() || resizeSeen.Group != gID || resizeSeen.AI != aiID || resizeSeen.Cols != 120 || resizeSeen.Rows != 36 {
		t.Fatalf("pty.resize forward = host=%p by=%q req=%+v", resizeHost, resizeBy, resizeSeen)
	}
	replay := fmt.Sprintf(`{"t":"takeover.start","n":"n1","ts":1,"to":%q,"g":%q,"member":%q}`, nodeKey.ID(), gID, aiID)
	if _, err := s.Replays["takeover.start"](t.Context(), nil, ownerKey.ID(), json.RawMessage(replay)); err == nil || err.(*host.Error).Code != "conflict" {
		t.Fatalf("takeover.start replay without a terminal = %v, want conflict", err)
	}

	// Outside a macOS app bundle, permissions is absent and host.permit returns bad_request.
	if strings.Contains(string(rawInfo), `"permissions"`) {
		t.Fatalf("unexpected permissions in bare host.info: %s", rawInfo)
	}
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 43}})
	expectErr(t, fc.next(t), 43, "bad_request")
}

func mustSurfaceSession(t *testing.T, h *host.Host, home string, key host.SessionKey, kind host.Kind, agent string) {
	t.Helper()
	if err := h.Sessions.Create(host.Session{SessionKey: key, Kind: kind, Folder: "/work/" + string(kind), Agent: agent, Home: home, Name: "AI"}); err != nil {
		t.Fatal(err)
	}
}

func mustMoveSurfaceSession(t *testing.T, h *host.Host, key host.SessionKey, from, to host.State) {
	t.Helper()
	if err := h.Sessions.Move(key, from, to); err != nil {
		t.Fatal(err)
	}
}

func assertSurfaceHeader(t *testing.T, st proto.HostState, savedHomes []proto.Home) {
	t.Helper()
	if st.Update == nil || st.Relay == nil {
		t.Fatalf("missing Update/Relay: %+v", st)
	}
	got := [11]any{st.Exe, st.Name, st.OS, st.Version, st.Open, st.MaxOpen, st.Offline, st.Settings.DefaultLevel, st.Update.Version, st.Relay.Used, slices.Equal(st.Homes, savedHomes)}
	want := [11]any{"solodawn-exe", "custom-box", "macOS 15.2", "v0.3.1", 3, 6, true, 3, "v0.3.2", int64(10), true}
	if got != want {
		t.Fatalf("unexpected HostState header: %+v", st)
	}
}

func assertSurfaceAgentGroups(t *testing.T, agents []proto.HostAgent, wantGroups map[proto.ID]proto.AgentGroup) {
	t.Helper()
	if len(agents) != 2 || [2]int{len(agents[0].Groups), len(agents[1].Groups)} != [2]int{3, 1} {
		t.Fatalf("unexpected agent groups: %+v", agents)
	}
	byGroup := map[proto.ID]proto.AgentGroup{}
	for _, ag := range append(slices.Clone(agents[0].Groups), agents[1].Groups...) {
		byGroup[ag.Group] = ag
	}
	for g, want := range wantGroups {
		if !reflect.DeepEqual(byGroup[g], want) {
			t.Fatalf("group %s = %+v, want %+v", g, byGroup[g], want)
		}
	}
}

func TestHostInfoAgentsMemberStateAndSettings(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("srf-node2"), sesKey("srf-owner2")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }})

	g1, g2, g3, g4 := string(sesID("g1")), string(sesID("g2")), string(sesID("g3")), string(sesID("g4"))
	m1, m2, m3, m4 := string(sesID("m1")), string(sesID("m2")), string(sesID("m3")), string(sesID("m4"))
	tTake, tWork, tWait := string(sesID("t-take")), string(sesID("t-work")), string(sesID("t-wait"))

	mustSurfaceSession(t, h, nodeKey.ID(), host.SessionKey{Group: g1, Member: m1}, host.GroupSession, "claude")
	mustSurfaceSession(t, h, nodeKey.ID(), host.SessionKey{Group: g1, Member: m1, Task: tTake}, host.TaskSession, "claude")
	mustMoveSurfaceSession(t, h, host.SessionKey{Group: g1, Member: m1, Task: tTake}, host.Dormant, host.Takeover)
	mustSurfaceSession(t, h, nodeKey.ID(), host.SessionKey{Group: g2, Member: m2, Task: tWork}, host.TaskSession, "claude")
	mustMoveSurfaceSession(t, h, host.SessionKey{Group: g2, Member: m2, Task: tWork}, host.Dormant, host.Working)
	mustSurfaceSession(t, h, nodeKey.ID(), host.SessionKey{Group: g3, Member: m3, Task: tWait}, host.TaskSession, "claude")
	mustMoveSurfaceSession(t, h, host.SessionKey{Group: g3, Member: m3, Task: tWait}, host.Dormant, host.Working)
	mustMoveSurfaceSession(t, h, host.SessionKey{Group: g3, Member: m3, Task: tWait}, host.Working, host.Waiting)
	mustSurfaceSession(t, h, nodeKey.ID(), host.SessionKey{Group: g4, Member: m4}, host.GroupSession, "codex")

	remoteHome := sesKey("remote-home").ID()
	savedHomes := []proto.Home{{Group: proto.ID(g1), Home: nodeKey.ID(), Name: "Local"}, {Group: proto.ID(g2), Home: remoteHome, Name: "Remote"}}
	seed := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('max_open', '6'), ('homes', ?), ('offline', 'true'), ('default_level', '1')`, string(dvlMustEncode(savedHomes)))
		return err
	}
	if err := db.Write(seed); err != nil {
		t.Fatal(err)
	}

	var hookErr error
	acct := "personal"
	sf := NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), OS: "macOS 15.2", Version: "v0.3.1", Exe: "solodawn-exe",
		Store: db, Sessions: s, Host: h,
		Name: func() string { return "custom-box" },
		Agents: func(context.Context, bool) ([]proto.HostAgent, error) {
			if hookErr != nil {
				return nil, hookErr
			}
			return []proto.HostAgent{
				{Agent: "claude", CLIVersion: "2.1.311", Account: &acct, LoginOK: true},
				{Agent: "codex", CLIVersion: "0.164.0", Account: nil, LoginOK: false},
			}, nil
		},
		Update: func() *proto.Update { return &proto.Update{Version: "v0.3.2", Status: "pending"} },
		Relay:  func() *proto.RelayUse { return &proto.RelayUse{Used: 10, Limit: 100} },
	})

	st, err := sf.HostState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertSurfaceHeader(t, st, savedHomes)
	assertSurfaceAgentGroups(t, st.Agents, map[proto.ID]proto.AgentGroup{
		// The workspace is the folder of the group session, which every AI has once it has joined.
		proto.ID(g1): {Group: proto.ID(g1), Member: proto.ID(m1), Workspace: "/work/group", Level: 3, State: "takeover", Task: proto.ID(tTake), Queued: 0, Watchers: []string{}},
		proto.ID(g2): {Group: proto.ID(g2), Member: proto.ID(m2), Level: 3, State: "working", Task: proto.ID(tWork), Queued: 0, Watchers: []string{}},
		proto.ID(g3): {Group: proto.ID(g3), Member: proto.ID(m3), Level: 3, State: "waiting", Task: proto.ID(tWait), Queued: 0, Watchers: []string{}},
		proto.ID(g4): {Group: proto.ID(g4), Member: proto.ID(m4), Workspace: "/work/group", Level: 3, State: "dormant", Queued: 0, Watchers: []string{}},
	})

	hookErr = errors.New("probe failed")
	if _, err := sf.HostState(t.Context()); !errors.Is(err, hookErr) {
		t.Fatalf("expected hookErr, got %v", err)
	}
}

func TestSurfaceHostPermitA111ToA114(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("perm-node"), sesKey("perm-owner")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, time.UnixMilli(1763600000000), nil)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})

	rig := newPermitRig(t, t.Context())
	var sf *Surface
	rig.mgr.push = func() { sf.pushHostInfo(t.Context(), false) }
	sf = NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), Store: db, Sessions: s, Host: h,
		Agents:      func(context.Context, bool) ([]proto.HostAgent, error) { return []proto.HostAgent{}, nil },
		Permissions: rig.mgr.State, Permit: rig.mgr.Handle,
	})

	fc := serveConn(t, s, ownerKey.ID(), false)
	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 3, Kind: "browser", Name: "b", Addrs: []string{}})
	fc.next(t) // welcome
	fc.next(t) // devlog

	// A11.1: host.info includes permissions with three booleans.
	fc.push(proto.Request{Type: "host.info", ID: 2})
	var info proto.HostInfo
	if err := proto.Decode(fc.next(t), &info); err != nil || info.Permissions == nil {
		t.Fatalf("host.info permissions = %+v (%v)", info.Permissions, err)
	}

	// Bad JSON and unknown open -> bad_request.
	fc.pushRaw(`{"t":"host.permit","id":3,"open":123}`)
	expectErr(t, fc.next(t), 3, "bad_request")
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 4}, Open: "screen"})
	expectErr(t, fc.next(t), 4, "bad_request")

	// A11.2: host.permit pushes host.info before ok when changed.
	rig.mu.Lock()
	rig.perms.LocalNetwork = true
	rig.mu.Unlock()
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 5}})
	var pushed proto.HostInfoPush
	if err := proto.Decode(fc.next(t), &pushed); err != nil || pushed.Type != "host.info" || !pushed.Permissions.LocalNetwork {
		t.Fatalf("expected host.info push before ok, got %+v (%v)", pushed, err)
	}
	var ok proto.Success
	if err := proto.Decode(fc.next(t), &ok); err != nil || ok.Re != 5 || !ok.OK {
		t.Fatalf("expected ok re=5, got %+v (%v)", ok, err)
	}

	// A11.3: open error returns internal.
	rig.openErr = errors.New("settings failed")
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 6}, Open: "folders"})
	expectErr(t, fc.next(t), 6, "internal")
	rig.openErr = nil

	// A11.4: path validation and async probe (non-blocking).
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 7}, Path: "relative"})
	expectErr(t, fc.next(t), 7, "bad_request")
	missing := filepath.Join(t.TempDir(), "absent")
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 8}, Path: missing})
	expectErr(t, fc.next(t), 8, "not_found")

	realFolder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan string, 1)
	release := make(chan error, 1)
	canceled := make(chan error, 1)
	rig.probeFn = func(ctx context.Context, f string) error {
		entered <- f
		select {
		case err := <-release:
			return err
		case <-ctx.Done():
			canceled <- ctx.Err()
			return ctx.Err()
		}
	}
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 9}, Path: realFolder})
	if got := <-entered; got != realFolder {
		t.Fatalf("probe path = %q; want %q", got, realFolder)
	}
	// While path probe is waiting, another host.info request still completes.
	fc.push(proto.Request{Type: "host.info", ID: 10})
	var midInfo proto.HostInfo
	if err := proto.Decode(fc.next(t), &midInfo); err != nil || midInfo.Re != 10 {
		t.Fatalf("midInfo = %+v (%v)", midInfo, err)
	}
	release <- nil
	if err := proto.Decode(fc.next(t), &ok); err != nil || ok.Re != 9 || !ok.OK {
		t.Fatalf("path ok = %+v (%v)", ok, err)
	}

	// Refusal -> permission; unexpected error -> internal.
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 11}, Path: realFolder})
	<-entered
	release <- &host.Error{Code: "permission", Message: "macOS does not allow SoloDawn to use " + realFolder}
	expectErr(t, fc.next(t), 11, "permission")

	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 12}, Path: realFolder})
	<-entered
	release <- errors.New("probe crashed")
	expectErr(t, fc.next(t), 12, "internal")

	// Closing the connection cancels a waiting path probe.
	fc.push(proto.HostPermit{Request: proto.Request{Type: "host.permit", ID: 13}, Path: realFolder})
	<-entered
	fc.Close()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled probe err = %v; want context.Canceled", err)
	}
}

func TestSurfaceDatabaseErrors(t *testing.T) {
	nodeKey, ownerKey := sesKey("err-node"), sesKey("err-owner")
	now := time.UnixMilli(1763600000000)
	db := sesOpenStore(t)
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})
	sf := NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), Store: db, Sessions: s, Host: h,
		Agents: func(context.Context, bool) ([]proto.HostAgent, error) { return nil, nil },
	})

	corrupt := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('max_open', 'not-int')")
		return err
	}
	if err := db.Write(corrupt); err != nil {
		t.Fatal(err)
	}
	if _, err := sf.HostState(t.Context()); err == nil {
		t.Fatal("expected error from corrupt setting")
	}

	drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE setting"); return err }
	if err := db.Write(drop); err != nil {
		t.Fatal(err)
	}
	if _, err := sf.HostState(t.Context()); err == nil {
		t.Fatal("expected error when setting table is missing")
	}
}
