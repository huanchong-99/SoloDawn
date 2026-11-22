package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
		Agents: func(context.Context) ([]proto.HostAgent, error) { return nil, nil },
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
	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
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
	createEnv := ownerKey.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Website","kind":"project","repo":"https://git.example/w.git","base":"main","forge":"none","checks":[],"member_name":"Owner"}`, gID, now.UnixMilli(), gID))
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
	fc.push(proto.PTYResize{Type: "pty.resize", Group: gID, AI: aiID, Cols: 120, Rows: 36})
	fc.push(proto.Request{Type: "host.info", ID: 4})
	fc.next(t)
	if resizeHost != h || resizeBy != ownerKey.ID() || resizeSeen.Group != gID || resizeSeen.AI != aiID || resizeSeen.Cols != 120 || resizeSeen.Rows != 36 {
		t.Fatalf("pty.resize forward = host=%p by=%q req=%+v", resizeHost, resizeBy, resizeSeen)
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

	mustSession := func(key host.SessionKey, kind host.Kind, agent string, st host.State) {
		t.Helper()
		if err := h.Sessions.Create(host.Session{SessionKey: key, Kind: kind, Folder: "repo", Agent: agent, Home: nodeKey.ID(), Name: "AI"}); err != nil {
			t.Fatal(err)
		}
		if st == host.Waiting {
			if err := h.Sessions.Move(key, host.Dormant, host.Working); err != nil {
				t.Fatal(err)
			}
			if err := h.Sessions.Move(key, host.Working, host.Waiting); err != nil {
				t.Fatal(err)
			}
		} else if st != host.Dormant {
			if err := h.Sessions.Move(key, host.Dormant, st); err != nil {
				t.Fatal(err)
			}
		}
	}
	mustSession(host.SessionKey{Group: g1, Member: m1}, host.GroupSession, "claude", host.Dormant)
	mustSession(host.SessionKey{Group: g1, Member: m1, Task: tTake}, host.TaskSession, "claude", host.Takeover)
	mustSession(host.SessionKey{Group: g2, Member: m2, Task: tWork}, host.TaskSession, "claude", host.Working)
	mustSession(host.SessionKey{Group: g3, Member: m3, Task: tWait}, host.TaskSession, "claude", host.Waiting)
	mustSession(host.SessionKey{Group: g4, Member: m4}, host.GroupSession, "codex", host.Dormant)

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
		ID: nodeKey.ID(), OS: "macOS 15.2", Version: "v0.3.1",
		Store: db, Sessions: s, Host: h,
		Name: func() string { return "custom-box" },
		Agents: func(context.Context) ([]proto.HostAgent, error) {
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
	if st.Name != "custom-box" || st.OS != "macOS 15.2" || st.Version != "v0.3.1" || st.Open != 3 || st.MaxOpen != 6 || st.Offline ||
		st.Update == nil || st.Update.Version != "v0.3.2" || st.Relay == nil || st.Relay.Used != 10 ||
		!slices.Equal(st.Homes, savedHomes) || st.Settings.DefaultLevel != 3 {
		t.Fatalf("unexpected HostState: %+v", st)
	}
	if len(st.Agents) != 2 || len(st.Agents[0].Groups) != 3 || len(st.Agents[1].Groups) != 1 {
		t.Fatalf("unexpected agent groups: %+v", st.Agents)
	}
	byGroup := map[proto.ID]proto.AgentGroup{}
	for _, ag := range append(slices.Clone(st.Agents[0].Groups), st.Agents[1].Groups...) {
		byGroup[ag.Group] = ag
	}
	wantGroups := map[proto.ID]proto.AgentGroup{
		proto.ID(g1): {Group: proto.ID(g1), Member: proto.ID(m1), Level: 3, RepoConfig: false, State: "takeover", Task: proto.ID(tTake), Queued: 0},
		proto.ID(g2): {Group: proto.ID(g2), Member: proto.ID(m2), Level: 3, RepoConfig: false, State: "working", Task: proto.ID(tWork), Queued: 0},
		proto.ID(g3): {Group: proto.ID(g3), Member: proto.ID(m3), Level: 3, RepoConfig: false, State: "waiting", Task: proto.ID(tWait), Queued: 0},
		proto.ID(g4): {Group: proto.ID(g4), Member: proto.ID(m4), Level: 3, RepoConfig: false, State: "dormant", Queued: 0},
	}
	for g, want := range wantGroups {
		if byGroup[g] != want {
			t.Fatalf("group %s = %+v, want %+v", g, byGroup[g], want)
		}
	}

	hookErr = errors.New("probe failed")
	if _, err := sf.HostState(t.Context()); !errors.Is(err, hookErr) {
		t.Fatalf("expected hookErr, got %v", err)
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
		Agents: func(context.Context) ([]proto.HostAgent, error) { return nil, nil },
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
