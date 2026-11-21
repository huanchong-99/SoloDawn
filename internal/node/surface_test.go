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
	sf := NewSurface(SurfaceConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Sessions: s, Host: h})
	sf.cfg.Report(errors.New("default surface log"))

	if ReadName(db) != "" || len(ReadRendezvous(db)) != 0 {
		t.Fatal("expected empty defaults for ReadName and ReadRendezvous")
	}
	put := func(tx *store.Tx) error {
		if _, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('name', '"desk"'), ('rendezvous', '["wss://rv.example/rv"]'), ('unknown_key', '123')`); err != nil {
			return err
		}
		return nil
	}
	if err := db.Write(put); err != nil {
		t.Fatal(err)
	}
	if ReadName(db) != "desk" || !slices.Equal(ReadRendezvous(db), []string{"wss://rv.example/rv"}) {
		t.Fatal("ReadName or ReadRendezvous mismatch")
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

	// Creating a group on home.Loop pushes proto.Groups to the welcomed browser and appears in host.info.homes.
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
	if err != nil || len(stateAfterGroup.Homes) != 1 || stateAfterGroup.Homes[0] != (proto.Home{Group: gID, Home: nodeKey.ID(), Name: "Website"}) {
		t.Fatalf("homes after group create = %+v (%v)", stateAfterGroup.Homes, err)
	}
}

func TestHostInfoAgentsDerivedStatesAndHooks(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("srf-node2"), sesKey("srf-owner2")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }})

	g1, g2, g3, g4, g5, g6, g7 := string(sesID("g1")), string(sesID("g2")), string(sesID("g3")), string(sesID("g4")), string(sesID("g5")), string(sesID("g6")), string(sesID("g7"))
	m1, m2, m3, m4, m5, m6, m7 := string(sesID("m1")), string(sesID("m2")), string(sesID("m3")), string(sesID("m4")), string(sesID("m5")), string(sesID("m6")), string(sesID("m7"))
	tTake, tWork, tWait := string(sesID("t-take")), string(sesID("t-work")), string(sesID("t-wait"))

	// Register temporary hold rule for limited, non-limited hold, and unheld "limited" reason check.
	host.Holds["test-limit"] = func(_ *host.Host, key host.SessionKey) (string, bool) {
		if key.Group == g4 {
			return "limited", true
		}
		if key.Group == g5 {
			return "other_hold", true
		}
		return "limited", false
	}
	t.Cleanup(func() { delete(host.Holds, "test-limit") })

	// Put paused group row on g5, unpaused group row on g6, invalid JSON on g7.
	pausedGroup := dvlMustEncode(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: g5, Seq: 1},
		Name:      "G5", Kind: "project", Base: "main", Forge: "none", Home: nodeKey.ID(),
		Checks: []string{}, Paused: &proto.GroupPause{By: m5, At: 1},
	})
	unpausedGroup := dvlMustEncode(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: g6, Seq: 1},
		Name:      "G6", Kind: "project", Base: "main", Forge: "none", Home: nodeKey.ID(),
		Checks: []string{}, Paused: nil,
	})
	h.Rows.Put(g5, "group", g5, pausedGroup)
	h.Rows.Put(g6, "group", g6, unpausedGroup)
	h.Rows.Put(g7, "group", g7, json.RawMessage(`{bad`))

	// Create sessions across all states: takeover (g1), working (g2), waiting (g6), dormant (g3, g4, g5, g7).
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
	mustSession(host.SessionKey{Group: g2, Member: m2}, host.GroupSession, "claude", host.Dormant)
	mustSession(host.SessionKey{Group: g2, Member: m2, Task: tWork}, host.TaskSession, "claude", host.Working)
	mustSession(host.SessionKey{Group: g3, Member: m3}, host.GroupSession, "claude", host.Dormant)
	mustSession(host.SessionKey{Group: g4, Member: m4}, host.GroupSession, "claude", host.Dormant)
	mustSession(host.SessionKey{Group: g5, Member: m5}, host.GroupSession, "claude", host.Dormant)
	mustSession(host.SessionKey{Group: g6, Member: m6}, host.GroupSession, "claude", host.Dormant)
	mustSession(host.SessionKey{Group: g6, Member: m6, Task: tWait}, host.TaskSession, "claude", host.Waiting)
	mustSession(host.SessionKey{Group: g7, Member: m7}, host.GroupSession, "codex", host.Dormant)

	// Seed agent rows (including an unfound row with cli_version = ''), perm rows, and saved homes.
	remoteHome := sesKey("remote-home").ID()
	savedHomes := dvlMustEncode([]proto.Home{
		{Group: proto.ID(g1), Home: nodeKey.ID(), Name: "Stale Name"},
		{Group: proto.ID(g2), Home: remoteHome, Name: "Remote"},
	})
	seed := func(tx *store.Tx) error {
		if _, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('homes', ?)`, string(savedHomes)); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO row(g,r,id,seq,pos,ts,mt,data) VALUES(?, 'group', ?, 1, 1, 1, 1, '{"name":"Fresh Local"}')`, g1, g1); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO agent(agent, cli_version, account, caps) VALUES
			('claude', '2.1.311', 'personal', '{"day":{"unit":"turns","max":100}}'),
			('codex', '0.164.0', NULL, '{}'),
			('unfound', '', NULL, '{}')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO perm(agent, g, level, repo_config) VALUES('claude', ?, 2, 1), ('claude', ?, 1, 0)`, g1, g2)
		return err
	}
	if err := db.Write(seed); err != nil {
		t.Fatal(err)
	}

	sf := NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), OS: "macOS 15.2", Version: "v0.3.1",
		Store: db, Logs: logs, Sessions: s, Host: h,
		Name:   func() string { return "custom-box" },
		Update: func() *proto.Update { return &proto.Update{Version: "v0.3.2", Status: "pending"} },
		Relay:  func() *proto.RelayUse { return &proto.RelayUse{Used: 10, Limit: 100} },
		Queued: func(group, _ string) int {
			if group == g1 {
				return 5
			}
			return 0
		},
		Now: func() time.Time { return now },
	})

	st, err := sf.HostState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "custom-box" || st.OS != "macOS 15.2" || st.Version != "v0.3.1" || st.Open != 3 || st.Update == nil || st.Update.Version != "v0.3.2" || st.Relay == nil || st.Relay.Used != 10 {
		t.Fatalf("unexpected header state: %+v", st)
	}
	// Homes merged: g1 updated to Fresh Local, g2 preserved from setting('homes').
	if len(st.Homes) != 2 || st.Homes[0].Name != "Fresh Local" || st.Homes[1].Home != remoteHome {
		t.Fatalf("merged homes = %+v", st.Homes)
	}
	if len(st.Agents) != 2 || st.Agents[0].Agent != "claude" || st.Agents[1].Agent != "codex" || st.Agents[1].Account != nil {
		t.Fatalf("agents = %+v", st.Agents)
	}

	byGroup := map[string]proto.AgentGroup{}
	for _, ag := range append(st.Agents[0].Groups, st.Agents[1].Groups...) {
		byGroup[string(ag.Group)] = ag
	}
	if byGroup[g1].State != "takeover" || string(byGroup[g1].Task) != tTake || byGroup[g1].Level != 2 || !byGroup[g1].RepoConfig || byGroup[g1].Queued != 5 {
		t.Fatalf("g1 group = %+v", byGroup[g1])
	}
	if byGroup[g2].State != "working" || string(byGroup[g2].Task) != tWork || byGroup[g2].Level != 1 || byGroup[g2].RepoConfig {
		t.Fatalf("g2 group = %+v", byGroup[g2])
	}
	if byGroup[g3].State != "dormant" || byGroup[g3].Level != 3 {
		t.Fatalf("g3 group = %+v", byGroup[g3])
	}
	if byGroup[g4].State != "limited" {
		t.Fatalf("g4 group = %+v", byGroup[g4])
	}
	if byGroup[g5].State != "paused" {
		t.Fatalf("g5 group = %+v", byGroup[g5])
	}
	if byGroup[g6].State != "waiting" || string(byGroup[g6].Task) != tWait {
		t.Fatalf("g6 group = %+v", byGroup[g6])
	}
	if byGroup[g7].State != "dormant" {
		t.Fatalf("g7 group = %+v", byGroup[g7])
	}

	// Verify offline=true and all_level3=true override non-active states and perm levels, plus Agents hook skipping agent table scan.
	setFlags := func(tx *store.Tx) error {
		if _, err := tx.Exec(`UPDATE agent SET caps = '{bad' WHERE agent = 'claude'`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('offline', 'true'), ('all_level3', 'true')`)
		return err
	}
	if err := db.Write(setFlags); err != nil {
		t.Fatal(err)
	}
	var hookErr error
	sf.cfg.Agents = func(context.Context) ([]proto.HostAgent, error) {
		if hookErr != nil {
			return nil, hookErr
		}
		acct := "team"
		return []proto.HostAgent{{Agent: "claude", CLIVersion: "2.2.0", Account: &acct, LoginOK: true, Caps: proto.Caps{}}}, nil
	}
	st2, err := sf.HostState(t.Context())
	if err != nil || !st2.Offline {
		t.Fatalf("offline HostState = %+v, %v", st2, err)
	}
	for _, ag := range st2.Agents[0].Groups {
		if ag.Level != 3 {
			t.Fatalf("expected all_level3 -> 3, got %+v", ag)
		}
		if string(ag.Group) == g2 && ag.State != "working" {
			t.Fatalf("expected working state to beat offline on g2, got %+v", ag)
		}
		if string(ag.Group) == g3 && ag.State != "offline" {
			t.Fatalf("expected offline state on g3, got %+v", ag)
		}
		if string(ag.Group) == g1 && (!ag.RepoConfig || ag.State != "takeover") {
			t.Fatalf("expected takeover + repoConfig preserved on g1, got %+v", ag)
		}
	}

	hookErr = errors.New("probe failed")
	if _, err := sf.HostState(t.Context()); !errors.Is(err, hookErr) {
		t.Fatalf("expected hookErr, got %v", err)
	}

	// reqHostInfo errors: bad JSON and non-owner device.
	if _, err := sf.reqHostInfo(t.Context(), nil, nil, ownerKey.ID(), 1, json.RawMessage(`{"t":"host.info","id":"bad"}`)); err == nil {
		t.Fatal("expected bad_request on invalid host.info request")
	}
	stranger := sesKey("stranger").ID()
	if _, err := sf.reqHostInfo(t.Context(), nil, nil, stranger, 1, json.RawMessage(`{"t":"host.info","id":1}`)); err == nil {
		t.Fatal("expected forbidden on non-owner device")
	}
}

func TestSettingsSetValidationAuditApproversAndPush(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey, delegateKey := sesKey("set-node"), sesKey("set-owner"), sesKey("set-delegate")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, home.Commands)
	link := AttachLocal(h, loop)
	t.Cleanup(func() { link.Close() })

	var reported []error
	s := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h,
		Now: func() time.Time { return now }, Report: func(err error) { reported = append(reported, err) },
	})
	sf := NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), OS: "linux/amd64", Version: "v0.3.1",
		Store: db, Logs: logs, Sessions: s, Host: h,
		Now: func() time.Time { return now }, Report: func(err error) { reported = append(reported, err) },
	})

	// Create a group and add an AI member on local home, plus a session on an unreachable remote home.
	gID := sesID("grp-approvers")
	aiID := sesID("ai-member")
	fcOwner := serveConn(t, s, ownerKey.ID(), false)
	fcOwner.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "Owner", Addrs: []string{}})
	fcOwner.next(t) // welcome
	fcOwner.next(t) // devlog

	fcUnwelcomed := serveConn(t, s, ownerKey.ID(), false)

	createEnv := ownerKey.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"App","kind":"project","repo":"https://git.example/a.git","base":"main","forge":"none","checks":[],"member_name":"Owner"}`, gID, now.UnixMilli(), gID))
	fcOwner.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: createEnv})
	fcOwner.next(t)
	fcOwner.next(t)

	fcNonOwner := serveConn(t, s, sesKey("paired-stranger").ID(), true)
	fcNonOwner.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "New", Addrs: []string{}})
	fcNonOwner.next(t) // welcome (owner: false)
	fcNonOwner.next(t) // devlog

	addAIPayload := dvlMustEncode(proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: sesID("add-ai"), Timestamp: now.UnixMilli(), Group: gID},
			Op:          "add_ai", ID: aiID,
		},
		CLI: "claude", Name: "Claude", Account: "personal", Secretary: true, Approvers: []string{ownerKey.ID()},
	})
	if _, err := h.Command(t.Context(), nodeKey.ID(), string(addAIPayload)); err != nil {
		t.Fatal(err)
	}
	// Two sessions for the same (gID, aiID) to pin deduplication, plus one session on an offline remote home.
	if err := h.Sessions.Create(host.Session{SessionKey: host.SessionKey{Group: string(gID), Member: string(aiID)}, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: nodeKey.ID(), Name: "Claude", Secretary: true}); err != nil {
		t.Fatal(err)
	}
	if err := h.Sessions.Create(host.Session{SessionKey: host.SessionKey{Group: string(gID), Member: string(aiID), Task: string(sesID("t1"))}, Kind: host.TaskSession, Folder: "wt/1-t", Agent: "claude", Home: nodeKey.ID(), Name: "Claude"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Sessions.Create(host.Session{SessionKey: host.SessionKey{Group: string(sesID("g-off")), Member: string(sesID("m-off"))}, Kind: host.GroupSession, Folder: "repo", Agent: "codex", Home: sesKey("offline-home").ID(), Name: "Codex"}); err != nil {
		t.Fatal(err)
	}
	seedAgents := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO agent(agent, cli_version) VALUES('claude', '2.1.311'), ('codex', '0.164.0')`)
		return err
	}
	if err := db.Write(seedAgents); err != nil {
		t.Fatal(err)
	}

	// Execute valid settings.set covering all fields and all 4 cap units across two calls.
	set1 := fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q,"retention_days":1,"idle_minutes":1,"max_open":1,"default_level":1,"all_level3":true,"approvers":[%q],"caps":{"claude":{"day":{"unit":"turns","max":50},"task":{"unit":"tokens","max":1000}},"codex":{"day":{"unit":"usd","max":0.5},"task":{"unit":"window_pct","max":80}}}}`,
		sesID("set-all-1"), now.UnixMilli(), nodeKey.ID(), delegateKey.ID())
	fcOwner.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: ownerKey.Sign(set1)})

	var sawPush proto.HostInfoPush
	var sawReply bool
	for range 2 {
		raw := fcOwner.next(t)
		var p proto.HostInfoPush
		if proto.Decode(raw, &p) == nil && p.Type == "host.info" {
			sawPush = p
			continue
		}
		var ok proto.Success
		if proto.Decode(raw, &ok) == nil && ok.Re == 3 {
			sawReply = true
		}
	}
	fcUnwelcomed.expectNone(t)
	fcNonOwner.expectNone(t)
	if !sawReply || sawPush.MaxOpen != 1 || sawPush.Settings.RetentionDays != 1 || sawPush.Settings.IdleMinutes != 1 ||
		sawPush.Settings.DefaultLevel != 1 || !sawPush.Settings.AllLevel3 ||
		!slices.Equal(sawPush.Settings.Approvers, []string{delegateKey.ID()}) {
		t.Fatalf("settings.set push/reply mismatch: reply=%v push=%+v", sawReply, sawPush)
	}
	if sawPush.Agents[0].Caps.Day == nil || sawPush.Agents[0].Caps.Day.Max != 50 || sawPush.Agents[1].Caps.Day.Unit != "usd" || sawPush.Agents[1].Caps.Task.Unit != "window_pct" {
		t.Fatalf("caps mismatch: %+v", sawPush.Agents)
	}
	if len(reported) != 1 {
		t.Fatalf("expected 1 reported unreachable-home error, got %v", reported)
	}

	// Check audit row in SQLite and updated AIMemberRow approvers in home store.
	var what, detail, by string
	var memberRaw string
	checkDB := func(tx *store.Tx) error {
		if err := tx.QueryRow("SELECT what, detail, by FROM audit ORDER BY id DESC LIMIT 1").Scan(&what, &detail, &by); err != nil {
			return err
		}
		return tx.QueryRow("SELECT data FROM row WHERE g = ? AND r = 'member' AND id = ?", string(gID), string(aiID)).Scan(&memberRaw)
	}
	if err := db.Write(checkDB); err != nil {
		t.Fatal(err)
	}
	wantDetail := "retention_days 1, idle_minutes 1, max_open 1, default_level 1, all_level3 true, approvers, caps.claude, caps.codex"
	if what != host.AuditSetting || detail != wantDetail || by != ownerKey.ID() {
		t.Fatalf("audit = (%q, %q, %q), want (%q, %q, %q)", what, detail, by, host.AuditSetting, wantDetail, ownerKey.ID())
	}
	var aiRow proto.AIMemberRow
	if err := proto.Decode([]byte(memberRaw), &aiRow); err != nil || !slices.Contains(aiRow.Approvers, delegateKey.ID()) || !slices.Contains(aiRow.Approvers, ownerKey.ID()) {
		t.Fatalf("ai member approvers = %+v (%v)", aiRow.Approvers, err)
	}

	// Upper boundary default_level = 3, all_level3 = false, empty approvers = [], clearing caps with {}.
	set2 := fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q,"default_level":3,"all_level3":false,"approvers":[],"caps":{"claude":{}}}`,
		sesID("set-all-2"), now.UnixMilli(), nodeKey.ID())
	if _, err := sf.cmdSettingsSet(t.Context(), fcOwner, ownerKey.ID(), json.RawMessage(set2)); err != nil {
		t.Fatal(err)
	}
	set3 := fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q,"caps":{"codex":{}}}`,
		sesID("set-codex-only"), now.UnixMilli(), nodeKey.ID())
	if _, err := sf.cmdSettingsSet(t.Context(), fcOwner, ownerKey.ID(), json.RawMessage(set3)); err != nil {
		t.Fatal(err)
	}
	stAfter, err := sf.HostState(t.Context())
	if err != nil || stAfter.Settings.DefaultLevel != 3 || stAfter.Settings.AllLevel3 || len(stAfter.Settings.Approvers) != 0 || stAfter.Agents[0].Caps != (proto.Caps{}) || stAfter.Agents[1].Caps != (proto.Caps{}) {
		t.Fatalf("state after set2/set3 = %+v (%v)", stAfter, err)
	}

	// Pin every boundary and operator in srfParseSettings.
	hdr := fmt.Sprintf(`"t":"settings.set","n":%q,"ts":1,"to":%q`, sesID("bad"), nodeKey.ID())
	badCases := []string{
		`{bad`,
		`{` + hdr + `}`,
		`{` + hdr + `,"retention_days":0}`,
		`{` + hdr + `,"idle_minutes":0}`,
		`{` + hdr + `,"max_open":0}`,
		`{` + hdr + `,"default_level":0}`,
		`{` + hdr + `,"default_level":4}`,
		`{` + hdr + `,"approvers":["not-a-device-id"]}`,
		`{` + hdr + `,"caps":{}}`,
		`{` + hdr + `,"caps":{"claude":{"day":{"unit":"hours","max":10}}}}`,
		`{` + hdr + `,"caps":{"claude":{"day":{"unit":"turns","max":0}}}}`,
		`{` + hdr + `,"caps":{"claude":{"day":{"unit":"turns","max":-1}}}}`,
		`{` + hdr + `,"caps":{"codex":{"task":{"unit":"bad","max":10}}}}`,
		`{` + hdr + `,"caps":{"codex":{"task":{"unit":"turns","max":0}}}}`,
	}
	for _, raw := range badCases {
		_, err := sf.cmdSettingsSet(t.Context(), fcOwner, ownerKey.ID(), json.RawMessage(raw))
		var werr *host.Error
		if !errors.As(err, &werr) || werr.Code != "bad_request" {
			t.Fatalf("case %s: got %v, want bad_request", raw, err)
		}
	}
}

func TestSurfaceDatabaseErrors(t *testing.T) {
	nodeKey, ownerKey := sesKey("err-node"), sesKey("err-owner")
	now := time.UnixMilli(1763600000000)

	makeSurface := func(t *testing.T) (*store.Store, *Surface) {
		t.Helper()
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
		loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})
		sf := NewSurface(SurfaceConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Sessions: s, Host: h})
		return db, sf
	}

	t.Run("corrupt setting json", func(t *testing.T) {
		db, sf := makeSurface(t)
		corrupt := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('max_open', 'not-int')")
			return err
		}
		if err := db.Write(corrupt); err != nil {
			t.Fatal(err)
		}
		if err := sf.PushHostInfo(t.Context()); err == nil {
			t.Fatal("expected error from corrupt setting")
		}
	})

	t.Run("dropped row table", func(t *testing.T) {
		db, sf := makeSurface(t)
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE row"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := sf.HostState(t.Context()); err == nil {
			t.Fatal("expected error when row table is missing")
		}
	})

	t.Run("dropped perm table", func(t *testing.T) {
		db, sf := makeSurface(t)
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE perm"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := sf.HostState(t.Context()); err == nil {
			t.Fatal("expected error when perm table is missing")
		}
	})

	t.Run("dropped agent table and corrupt agent caps", func(t *testing.T) {
		db, sf := makeSurface(t)
		corrupt := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO agent(agent, cli_version, caps) VALUES('claude', '1.0', '{bad')")
			return err
		}
		if err := db.Write(corrupt); err != nil {
			t.Fatal(err)
		}
		if _, err := sf.HostState(t.Context()); err == nil {
			t.Fatal("expected error for corrupt agent caps")
		}
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE agent"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := sf.HostState(t.Context()); err == nil {
			t.Fatal("expected error when agent table is missing")
		}
		// Write failure in srfExecCaps when agent table is missing.
		payload := fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":1,"to":%q,"caps":{"claude":{}}}`, sesID("c"), nodeKey.ID())
		if _, err := sf.cmdSettingsSet(t.Context(), nil, ownerKey.ID(), json.RawMessage(payload)); err == nil {
			t.Fatal("expected srfExecCaps error when agent table is missing")
		}
	})

	t.Run("settings write failure and PushHostInfo failure after settings.set", func(t *testing.T) {
		db, sf := makeSurface(t)
		var reported int
		sf.cfg.Report = func(error) { reported++ }
		sf.cfg.Agents = func(context.Context) ([]proto.HostAgent, error) { return nil, errors.New("push fail") }

		payload := fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":1,"to":%q,"max_open":2}`, sesID("m"), nodeKey.ID())
		if _, err := sf.cmdSettingsSet(t.Context(), nil, ownerKey.ID(), json.RawMessage(payload)); err != nil || reported != 1 {
			t.Fatalf("expected PushHostInfo error to be reported, err=%v reported=%d", err, reported)
		}

		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE setting"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := sf.HostState(t.Context()); err == nil {
			t.Fatal("expected error when setting table is missing")
		}
		if _, err := sf.cmdSettingsSet(t.Context(), nil, ownerKey.ID(), json.RawMessage(payload)); err == nil {
			t.Fatal("expected srfExecSettings error when setting table is missing")
		}
	})
}
