package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type inviteLogWrite struct{ log *identity.Log }

func (w inviteLogWrite) Put(tx *store.Tx) error {
	for _, env := range w.log.Entries {
		if _, err := tx.Exec("INSERT OR IGNORE INTO device_log(user, ver, env) VALUES(?, ?, ?)", w.log.User, dvlRowOf(env).Version, string(dvlMustEncode(env))); err != nil {
			return err
		}
	}
	return nil
}

func testJoin(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	if c.Joining == nil || !c.Joining.Open || c.Who.User == "" {
		return home.Result{}, &home.Refusal{Code: "forbidden", Msg: "cannot join"}
	}
	row := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: string(c.Nonce)}, Group: s.ID, Kind: "human", Name: "Guest", User: c.Who.User},
		Role:      "member",
	}
	rows := []home.Row{home.RowOf(row)}
	for _, env := range c.Joining.Log.Entries {
		rows = append(rows, home.RowOf(dvlRowOf(env)))
	}
	return home.Result{Rows: rows, Writes: []home.Write{inviteLogWrite{c.Joining.Log}}, InviteUsed: true, GroupsChanged: true}, nil
}

func TestInvitationJoinAndMemberSurface(t *testing.T) {
	admin, guest := sesKey("invite-admin"), sesKey("invite-guest")
	ph := newPairHarness(t, &admin, true)
	expire := make(chan time.Time, 1)
	ph.pairer.cfg.After = func(time.Duration) <-chan time.Time { return expire }
	tables := home.Tables{
		Commands: home.Table{
			"group/create": testGroupCreate,
			"invite": func(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
				return home.Result{InviteBy: c.Who.Human}, nil
			},
			"join": testJoin,
		},
		Events: home.Events,
	}
	loop, err := home.StartLoop(home.LoopConfig{Store: ph.db, Home: ph.nodeKey.ID(), Owner: admin.ID(), Logs: ph.logs.Log, Tables: tables, Now: ph.clock.Now, Invite: ph.pairer})
	mustNoErr(t, err)
	t.Cleanup(loop.Close)
	ph.sessions.cfg.Home = loop
	ph.sessions.cfg.Known = func(device string) bool { return ph.logs.Known(device) || ph.pairer.Known(device) }
	group := sesID("invite-shared")
	owner := serveConn(t, ph.sessions, admin.ID(), false)
	checkInviteHello(t, owner, 1, true, 0)
	owner.next(t)
	sendInviteCmd(owner, admin, 2, proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(ph, "group", group, "create"), Op: "create"},
		Name:         "Shared", Checks: []string{},
	})
	var create proto.CmdResult
	mustNoErr(t, proto.Decode(owner.next(t), &create))
	owner.next(t)
	cmd := inviteCommand(ph, "invite", group, "invitation")
	sendInviteCmd(owner, admin, 3, cmd)
	var invite proto.CmdResult
	mustNoErr(t, proto.Decode(owner.next(t), &invite))
	sendInviteCmd(owner, admin, 4, cmd)
	checkReplayedBrowserLink(t, owner.next(t), 4, invite)

	attempt := checkInviteLink(t, ph, group, invite)
	probeKey := sesKey("invite-probe")
	probe := startJoiningCtl(t, ph, attempt, probeKey)
	bad := sesAddEntry(probeKey, probeKey.ID(), 2, probeKey.ID(), "browser", "Probe")
	probe.push(proto.Devlog{Type: "devlog", User: probeKey.ID(), Entries: []proto.Envelope{bad}})
	probeJoin := proto.Join{HomeCommand: inviteCommand(ph, "join", group, "probe-join"), Name: "Probe"}
	sendInviteCmd(probe, probeKey, 2, probeJoin)
	expectErr(t, probe.next(t), 2, "forbidden")

	ctl := startJoiningCtl(t, ph, attempt, guest)
	checkJoinAdmission(t, ph, ctl, guest, group)
	probeJoin.Nonce = sesID("probe-used-link")
	genesis := sesAddEntry(probeKey, probeKey.ID(), 1, probeKey.ID(), "browser", "Probe")
	probe.push(proto.Devlog{Type: "devlog", User: probeKey.ID(), Entries: []proto.Envelope{genesis}})
	var probeGroups proto.Groups
	mustNoErr(t, proto.Decode(probe.next(t), &probeGroups))
	if len(probeGroups.Groups) != 0 {
		t.Fatalf("unadmitted peer got groups: %+v", probeGroups)
	}
	sendInviteCmd(probe, probeKey, 3, probeJoin)
	expectErr(t, probe.next(t), 3, "forbidden")
	ph.clock.Set(time.UnixMilli(invite.Expires))
	expire <- ph.clock.Now()
	select {
	case <-probe.Done():
	case <-time.After(5 * time.Second):
		t.Fatal(timedOut(5*time.Second, "the invitation expiry to close the unadmitted connection", nil))
	}

	cmd.Timestamp = ph.clock.Now().UnixMilli()
	sendInviteCmd(owner, admin, 5, cmd)
	var changed proto.Groups
	mustNoErr(t, proto.Decode(owner.next(t), &changed))
	expectErr(t, owner.next(t), 5, "conflict")
	checkInviteHello(t, ctl, 8, false, 1)
	gotUsers := make([]string, 2)
	for i := range gotUsers {
		var dl proto.Devlog
		mustNoErr(t, proto.Decode(ctl.next(t), &dl))
		gotUsers[i] = dl.User
	}
	wantUsers := []string{admin.ID(), guest.ID()}
	slices.Sort(wantUsers)
	if !slices.Equal(gotUsers, wantUsers) {
		t.Fatalf("welcome log users = %v, want %v", gotUsers, wantUsers)
	}
	ctl.pushRaw(`{"t":"host.info","id":9}`)
	expectErr(t, ctl.next(t), 9, "forbidden")
	checkMemberLogUpdate(t, ph, owner, ctl, admin, guest)
}

func inviteCommand(ph *pairHarness, kind string, group proto.ID, nonce string) proto.HomeCommand {
	return proto.HomeCommand{Type: kind, Group: group, Nonce: sesID(nonce), Timestamp: ph.clock.Now().UnixMilli()}
}

func sendInviteCmd(c *fakeCtl, key identity.Key, re int64, payload any) {
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: re}, Env: key.Sign(string(dvlMustEncode(payload)))})
}

func checkInviteHello(t *testing.T, ctl *fakeCtl, id int64, owner bool, groups int) {
	t.Helper()
	ctl.push(proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 2, Kind: "browser", Name: "browser", Addrs: []string{}})
	var w proto.Welcome
	mustNoErr(t, proto.Decode(ctl.next(t), &w))
	if w.V != 2 || w.Re != id || w.Owner != owner || len(w.Groups) != groups {
		t.Fatalf("welcome = %+v, want v2 re=%d owner=%t groups=%d", w, id, owner, groups)
	}
}

func checkInviteLink(t *testing.T, ph *pairHarness, group proto.ID, invite proto.CmdResult) *pairAttempt {
	t.Helper()
	link, err := url.Parse(invite.Link)
	mustNoErr(t, err)
	values, err := url.ParseQuery(link.Fragment)
	mustNoErr(t, err)
	secret, err := identity.ParseSecret(values.Get("join"))
	mustNoErr(t, err)
	if invite.Expires != ph.clock.Now().Add(pairWindow).UnixMilli() || values.Get("g") != string(group) || values.Get("home") != ph.nodeKey.ID() {
		t.Fatalf("unexpected invitation: %+v", invite)
	}
	a := ph.pairer.findRoom(secret.Room())
	if a == nil {
		t.Fatal("invitation room missing")
	}
	return a
}

func startJoiningCtl(t *testing.T, ph *pairHarness, a *pairAttempt, guest identity.Key) *fakeCtl {
	t.Helper()
	pair := proto.Pair{Type: "pair", Key: guest.ID(), Kind: "browser", Name: "Guest", Role: "b"}
	pair.MAC = a.secret.MAC(pair)
	ph.pairer.acceptPair(a, pair)
	ctl, done := newFakeCtl(), make(chan struct{})
	go func() {
		defer close(done)
		ph.pairer.Serve(t.Context(), guest.ID(), ctl)
	}()
	t.Cleanup(func() { ctl.Close(); <-done })
	checkInviteHello(t, ctl, 1, false, 0)
	return ctl
}

func checkJoinAdmission(t *testing.T, ph *pairHarness, ctl *fakeCtl, guest identity.Key, group proto.ID) {
	t.Helper()
	ctl.push(proto.Sync{Request: proto.Request{Type: "sync", ID: 2}, Group: string(group)})
	expectErr(t, ctl.next(t), 2, "forbidden")
	sendInviteCmd(ctl, guest, 3, inviteCommand(ph, "invite", group, "cannot-invite"))
	expectErr(t, ctl.next(t), 3, "forbidden")
	genesis := sesAddEntry(guest, guest.ID(), 1, guest.ID(), "browser", "Guest")
	ctl.push(proto.Devlog{Type: "devlog", User: guest.ID(), Entries: []proto.Envelope{genesis}})
	sendInviteCmd(ctl, guest, 5, proto.Join{HomeCommand: inviteCommand(ph, "join", group, "good-join"), Name: "Guest"})
	var ans proto.CmdResult
	mustNoErr(t, proto.Decode(ctl.next(t), &ans))
	if ans.Re != 5 || !ans.OK {
		t.Fatalf("join answer = %+v", ans)
	}
	var groups proto.Groups
	mustNoErr(t, proto.Decode(ctl.next(t), &groups))
	if len(groups.Groups) != 1 || groups.Groups[0].Group != string(group) {
		t.Fatalf("groups push = %+v", groups)
	}
	reopened, err := OpenLogs(ph.db, ph.nodeKey.ID())
	mustNoErr(t, err)
	if reopened.UserOf(guest.ID()) != guest.ID() || reopened.IsOwner(guest.ID()) ||
		!slices.Equal(reopened.Log(guest.ID()).Entries, []proto.Envelope{genesis}) {
		t.Fatal("join did not persist the guest's own log")
	}
}

func checkMemberLogUpdate(t *testing.T, ph *pairHarness, owner, ctl *fakeCtl, admin, guest identity.Key) {
	t.Helper()
	hostCmd := proto.HostCommand{Type: "home.add", To: ph.nodeKey.ID(), Nonce: sesID("member-host-command"), Timestamp: ph.clock.Now().UnixMilli()}
	sendInviteCmd(ctl, guest, 10, hostCmd)
	expectErr(t, ctl.next(t), 10, "forbidden")
	device := sesKey("invite-guest-device")
	add := sesAddEntry(guest, guest.ID(), 2, device.ID(), "node", "Guest node")
	ctl.push(proto.Devlog{Type: "devlog", User: guest.ID(), Entries: []proto.Envelope{add}})
	var forwarded proto.Devlog
	mustNoErr(t, proto.Decode(owner.next(t), &forwarded))
	if forwarded.User != guest.ID() || !slices.Equal(forwarded.Entries, []proto.Envelope{add}) || !ph.logs.Known(device.ID()) {
		t.Fatalf("forwarded device log = %+v", forwarded)
	}
	node := serveConn(t, ph.sessions, device.ID(), false)
	checkInviteHello(t, node, 1, false, 1)
	checkMemberRevocation(t, ph, owner, node, admin, guest, device)
}

func checkMemberRevocation(t *testing.T, ph *pairHarness, owner, node *fakeCtl, admin, guest, device identity.Key) {
	t.Helper()
	second := openSecondInvitation(t, ph, owner, admin)
	revoke := guest.Sign(fmt.Sprintf(`{"t":"device","user":%q,"ver":3,"op":"revoke","dev":%q}`, guest.ID(), device.ID()))
	joining := startJoiningCtl(t, ph, second, guest)
	entries := append(ph.logs.Log(guest.ID()).Entries, revoke)
	joining.push(proto.Devlog{Type: "devlog", User: guest.ID(), Entries: entries})
	sendInviteCmd(joining, guest, 2, proto.Join{HomeCommand: inviteCommand(ph, "join", sesID("second-group"), "second-join"), Name: "Guest"})
	joined := inviteJoinResult(t, joining)
	if joined.Re != 2 || !joined.OK {
		t.Fatalf("second join = %+v", joined)
	}
	select {
	case <-node.Done():
	case <-time.After(5 * time.Second):
		t.Fatal(timedOut(5*time.Second, "the revoked member device to disconnect", nil))
	}
	if ph.logs.Known(device.ID()) {
		t.Fatal("revoked device remains known")
	}
}

func inviteJoinResult(t *testing.T, ctl *fakeCtl) proto.CmdResult {
	t.Helper()
	for {
		raw := ctl.next(t)
		var header struct {
			Type string `json:"t,omitempty"`
		}
		mustNoErr(t, proto.Decode(raw, &header))
		if header.Type == "groups" {
			var groups proto.Groups
			mustNoErr(t, proto.Decode(raw, &groups))
			continue
		}
		var result proto.CmdResult
		mustNoErr(t, proto.Decode(raw, &result))
		return result
	}
}

func TestInviteJoinResult(t *testing.T) {
	for _, pushes := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(pushes), func(t *testing.T) {
			ctl := newFakeCtl()
			for range pushes {
				mustNoErr(t, ctl.Send(proto.Groups{Type: "groups", Groups: []proto.GroupSummary{}}))
			}
			want := proto.CmdResult{Success: proto.Success{Re: 2, OK: true}}
			mustNoErr(t, ctl.Send(want))
			mustNoErr(t, ctl.Send(proto.Groups{Type: "groups", Groups: []proto.GroupSummary{}}))
			if got := inviteJoinResult(t, ctl); got != want {
				t.Fatalf("join result = %+v, want %+v", got, want)
			}
			if got := string(ctl.next(t)); got != `{"t":"groups","groups":[]}` {
				t.Fatalf("following push = %s", got)
			}
		})
	}
}

func openSecondInvitation(t *testing.T, ph *pairHarness, owner *fakeCtl, admin identity.Key) *pairAttempt {
	t.Helper()
	group := sesID("second-group")
	sendInviteCmd(owner, admin, 11, proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(ph, "group", group, "second-create"), Op: "create"},
		Name:         "Second", Checks: []string{},
	})
	var result proto.CmdResult
	mustNoErr(t, proto.Decode(owner.next(t), &result))
	owner.next(t)
	sendInviteCmd(owner, admin, 12, inviteCommand(ph, "invite", group, "second-invitation"))
	mustNoErr(t, proto.Decode(owner.next(t), &result))
	return checkInviteLink(t, ph, group, result)
}

func TestInviteWithoutRendezvous(t *testing.T) {
	owner := sesKey("invite-without-path")
	ph := newPairHarness(t, &owner, false)
	ph.pairer.cfg.Rendezvous = func() []string { return nil }
	ph.pairer.cfg.WebURL = nil
	for _, webURL := range []string{"", "https://client.example.test"} {
		if webURL != "" {
			ph.pairer.cfg.WebURL = func() string { return webURL }
		}
		link, err := ph.pairer.Invite("group", "member", owner.ID(), "nonce")
		if err != errNoRendezvous || link != (home.InviteLink{}) {
			t.Fatalf("Invite(%q) = %+v, %v; want missing rendezvous", webURL, link, err)
		}
	}
}

func TestHomeAddRecordsAndReplacesHome(t *testing.T) {
	owner, nodeKey := sesKey("home-add-owner"), sesKey("home-add-node")
	old, next := sesKey("former-home"), sesKey("next-home")
	entries := []proto.Envelope{sesAddEntry(owner, owner.ID(), 1, owner.ID(), "browser", "Owner"), sesAddEntry(owner, owner.ID(), 2, nodeKey.ID(), "node", "Node")}
	n := newClientNodeRig(t, "node", nodeKey, owner, entries, nil)
	group := sesID("home-add-group")
	dials := make(chan string, 2)
	n.host.Connect = func(_ context.Context, device string) { dials <- device }
	c := serveConn(t, n.sessions, owner.ID(), false)
	checkInviteHello(t, c, 1, true, 0)
	c.next(t)
	sendHomeAdd(t, n, c, owner, old, group, 2)
	if got := recvFrame(t, dials); got != old.ID() {
		t.Fatalf("remembered home dial = %q, want %q", got, old.ID())
	}
	mustNoErr(t, n.host.EnsureHomeConnections())
	if len(dials) != 0 {
		t.Fatal("remembering an unhosted group dialled its home twice")
	}
	key := host.SessionKey{Group: string(group), Member: string(sesID("hosted-ai"))}
	mustNoErr(t, n.host.Sessions.Create(host.Session{SessionKey: key, Home: old.ID(), Kind: host.GroupSession, Agent: "claude", Folder: "repo"}))
	mustNoErr(t, n.host.EnsureHomeConnections())
	if len(dials) != 0 {
		t.Fatal("adding an AI dialled the remembered home twice")
	}
	sendHomeAdd(t, n, c, owner, next, group, 3)
	if got := recvFrame(t, dials); got != next.ID() {
		t.Fatalf("replacement home dial = %q, want %q", got, next.ID())
	}
	row, ok := n.host.Sessions.Get(key)
	if !ok || row.Home != next.ID() {
		t.Fatalf("session after replacement = %+v", row)
	}
	sendInviteCmd(c, owner, 4, proto.HostCommand{
		Type: "home.add", To: nodeKey.ID(), Nonce: sesID("incomplete-home-add"), Timestamp: n.sessions.cfg.Now().UnixMilli(),
	})
	expectErr(t, c.next(t), 4, "bad_request")
}

func sendHomeAdd(t *testing.T, n *clientNodeRig, c *fakeCtl, owner, target identity.Key, group proto.ID, id int64) {
	t.Helper()
	address := "wss://example.test/rv"
	cmd := proto.HomeAdd{
		HostCommand: proto.HostCommand{Type: "home.add", Nonce: sesID(fmt.Sprint(id)), Timestamp: n.sessions.cfg.Now().UnixMilli(), To: n.key.ID()},
		Group:       group, Home: target.ID(), Name: "Shared", Addrs: []string{address},
	}
	sendInviteCmd(c, owner, id, cmd)
	var ans proto.CmdResult
	mustNoErr(t, json.Unmarshal(c.next(t), &ans))
	if ans.Re != id || !ans.OK {
		t.Fatalf("home.add answer = %+v", ans)
	}
	runtime := nodeRuntime{db: n.db}
	if !runtime.remembersHome(target.ID()) {
		t.Fatal("the added home is not known for signalling")
	}
	checkSingleString(t, n.db, fmt.Sprintf("SELECT url FROM peer WHERE device = '%s'", target.ID()), address)
}

// The connections of a person who left or was removed end after the groups
// push, and their devices are unknown, once no group homed here has them.
func TestRemovedPersonIsDisconnected(t *testing.T) {
	for _, self := range []bool{false, true} {
		t.Run(fmt.Sprint(self), func(t *testing.T) { checkRemovedPerson(t, self) })
	}
}

func checkRemovedPerson(t *testing.T, self bool) {
	guest, device := sesKey("removed-guest"), sesKey("removed-guest-node")
	var n *nodeRuntime
	var err error
	renamed := func() error {
		_, err := n.logs.dvlCommit(guest.ID(), []proto.Envelope{sesAddEntry(guest, guest.ID(), 3, device.ID(), "node", "Renamed")})
		return err
	}
	between := make(chan error, 1)
	home.After["entry after the removal"] = func(_ *home.State, res home.Result) {
		if res.Forget != "" {
			between <- renamed() // the removal is committed, and the log still held
		}
	}
	t.Cleanup(func() { delete(home.After, "entry after the removal") })
	n, err = openNode(t.Context(), socketRoot(t))
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	ph := &pairHarness{pairer: n.pairer, nodeKey: n.key, clock: newPairClock(time.Now())}
	n.pairer.cfg.Now = ph.clock.Now
	admin := adoptTestBrowser(t, n)
	owner := startAdoptedConn(t, n, admin.ID())
	log := []proto.Envelope{
		sesAddEntry(guest, guest.ID(), 1, guest.ID(), "browser", "Guest"),
		sesAddEntry(guest, guest.ID(), 2, device.ID(), "node", "Guest node"),
	}
	first, second := sesID("removal-first"), sesID("removal-second")
	for _, group := range []proto.ID{first, second} {
		joinAsGuest(t, ph, owner, admin, guest, group, log)
	}
	browser, node := startAdoptedConn(t, n, guest.ID()), startAdoptedConn(t, n, device.ID())
	browser.next(t) // the owner's log follows the guest's
	by, conn := admin, owner
	if self {
		by, conn = guest, browser
	}
	remove := func(group proto.ID) {
		cmd := inviteCommand(ph, "member", group, string(group)+"-remove")
		sendInviteCmd(conn, by, 9, proto.MemberCommand{HomeCommand: cmd, Op: "remove", ID: guestMember(group)})
		mustEqual(t, bool(inviteJoinResult(t, conn).OK), true)
		waitGroups(t, owner, 2)
	}
	remove(first)
	waitGroups(t, node, 1)
	mustEqual(t, n.knownPeer(device.ID()), true)
	remove(second)
	waitLastGroups(t, browser)
	waitLastGroups(t, node)
	mustEqual(t, n.knownPeer(guest.ID()) || n.knownPeer(device.ID()), false)
	mustEqual(t, n.knownPeer(admin.ID()), true)
	mustNoErr(t, <-between)
	mustNoErr(t, renamed())
	_, err = OpenLogs(n.db, n.key.ID()) // an entry stored without the log would end the next start
	mustNoErr(t, err)
}

// The host of AIs of another home has no group to push: the entry itself ends
// the revoked device's connection.
func TestRevokedDeviceIsDisconnectedWithoutAGroup(t *testing.T) {
	n, err := openNode(t.Context(), socketRoot(t))
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	owner, second := adoptTestBrowser(t, n), sesKey("second-browser")
	_, err = n.logs.dvlCommit(owner.ID(), []proto.Envelope{sesAddEntry(owner, owner.ID(), 3, second.ID(), "browser", "Second")})
	mustNoErr(t, err)
	first, revoked := startAdoptedConn(t, n, owner.ID()), startAdoptedConn(t, n, second.ID())
	first.push(proto.Devlog{Type: "devlog", User: owner.ID(), Entries: []proto.Envelope{sesRevokeEntry(owner, owner.ID(), 4, second.ID())}})
	select {
	case <-revoked.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a revoked device's connection stayed open")
	}
	first.pushRaw(`{"t":"sync","id":2,"g":"` + string(sesID("no-group")) + `","since":0,"state_only":false}`)
	expectErr(t, first.next(t), 2, "not_found")
}

func waitLastGroups(t *testing.T, c *fakeCtl) {
	t.Helper()
	waitGroups(t, c, 0)
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a removed person's connection stayed open")
	}
}

func guestMember(group proto.ID) proto.ID { return sesID(string(group) + "-join") }

func joinAsGuest(t *testing.T, ph *pairHarness, owner *fakeCtl, admin, guest identity.Key, group proto.ID, log []proto.Envelope) {
	t.Helper()
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(ph, "group", group, ""), Op: "create"},
		Name:         "Group", MemberName: "Admin", Kind: "project", Repo: "repo", Base: "main", Forge: "none", Checks: []string{},
	}
	create.Nonce = group
	sendInviteCmd(owner, admin, 7, create)
	mustEqual(t, bool(inviteJoinResult(t, owner).OK), true)
	sendInviteCmd(owner, admin, 8, inviteCommand(ph, "invite", group, string(group)+"-invite"))
	ctl := startJoiningCtl(t, ph, checkInviteLink(t, ph, group, inviteJoinResult(t, owner)), guest)
	ctl.push(proto.Devlog{Type: "devlog", User: guest.ID(), Entries: log})
	sendInviteCmd(ctl, guest, 2, proto.Join{HomeCommand: inviteCommand(ph, "join", group, string(group)+"-join"), Name: "Guest"})
	mustEqual(t, bool(inviteJoinResult(t, ctl).OK), true)
}

// waitGroups reads c up to the groups push that lists want groups.
func waitGroups(t *testing.T, c *fakeCtl, want int) {
	t.Helper()
	for {
		var push proto.Groups
		if proto.Decode(c.next(t), &push) == nil && push.Type == "groups" && len(push.Groups) == want {
			return
		}
	}
}
