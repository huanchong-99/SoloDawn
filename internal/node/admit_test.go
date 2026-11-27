package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
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
		Name:         "Shared",
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
	ctl.push(proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 3, Kind: "browser", Name: "browser", Addrs: []string{}})
	var w proto.Welcome
	mustNoErr(t, proto.Decode(ctl.next(t), &w))
	if w.V != 3 || w.Re != id || w.Owner != owner || len(w.Groups) != groups {
		t.Fatalf("welcome = %+v, want v3 re=%d owner=%t groups=%d", w, id, owner, groups)
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

// checkMemberRevocation joins the guest to a second group with a log shorter
// than the one the home holds, then revokes the guest's node, which cannot
// join a third group with the older log.
func checkMemberRevocation(t *testing.T, ph *pairHarness, owner, node *fakeCtl, admin, guest, device identity.Key) {
	t.Helper()
	held := ph.logs.Log(guest.ID()).Entries
	joining := joinWithLog(t, ph, owner, admin, guest, "second", held[:1])
	mustEqual(t, bool(inviteJoinResult(t, joining).OK), true)
	waitGroups(t, node, 2)
	joining.push(proto.Devlog{Type: "devlog", User: guest.ID(), Entries: []proto.Envelope{sesRevokeEntry(guest, guest.ID(), 3, device.ID())}})
	select {
	case <-node.Done():
	case <-time.After(5 * time.Second):
		t.Fatal(timedOut(5*time.Second, "the revoked member device to disconnect", nil))
	}
	if ph.logs.Known(device.ID()) {
		t.Fatal("revoked device remains known")
	}
	revoked := joinWithLog(t, ph, owner, admin, device, "third", held)
	expectErr(t, revoked.next(t), 2, "forbidden")
}

// joinWithLog opens an invitation to a new group and sends join from key
// with log presented.
func joinWithLog(t *testing.T, ph *pairHarness, owner *fakeCtl, admin, key identity.Key, name string, log []proto.Envelope) *fakeCtl {
	t.Helper()
	group := sesID(name + "-group")
	sendInviteCmd(owner, admin, 11, proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(ph, "group", group, name+"-create"), Op: "create"},
		Name:         name,
	})
	mustEqual(t, bool(inviteJoinResult(t, owner).OK), true)
	sendInviteCmd(owner, admin, 12, inviteCommand(ph, "invite", group, name+"-invitation"))
	joining := startJoiningCtl(t, ph, checkInviteLink(t, ph, group, inviteJoinResult(t, owner)), key)
	joining.push(proto.Devlog{Type: "devlog", User: log[0].Key, Entries: log})
	sendInviteCmd(joining, key, 2, proto.Join{HomeCommand: inviteCommand(ph, "join", group, name+"-join"), Name: "Guest"})
	return joining
}

func inviteJoinResult(t *testing.T, ctl *fakeCtl) proto.CmdResult {
	t.Helper()
	for {
		raw := ctl.next(t)
		var header struct {
			Type string `json:"t,omitempty"`
		}
		mustNoErr(t, proto.Decode(raw, &header))
		if header.Type != "" {
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
		Name:         "Group", MemberName: "Admin",
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

// heldTimers stands in for time.After: a timer is kept, with its length,
// until the test lets it run out.
type heldTimers struct {
	mu   sync.Mutex
	held []heldTimer
}

type heldTimer struct {
	length time.Duration
	run    chan time.Time // single reader: the watcher that asked for the timer
}

func (h *heldTimers) After(d time.Duration) <-chan time.Time {
	run := make(chan time.Time, 1)
	h.mu.Lock()
	h.held = append(h.held, heldTimer{d, run})
	h.mu.Unlock()
	return run
}

// lengths waits for n timers and returns how long each is, in the order
// they were asked for.
func (h *heldTimers) lengths(t *testing.T, n int) []time.Duration {
	t.Helper()
	var got []time.Duration
	poll(t, 5*time.Second, fmt.Sprintf("%d timers", n), func() string {
		h.mu.Lock()
		defer h.mu.Unlock()
		got = nil
		for _, timer := range h.held {
			got = append(got, timer.length)
		}
		if len(got) != n {
			return fmt.Sprint(got)
		}
		return ""
	})
	return got
}

// runOut lets the i-th timer run out.
func (h *heldTimers) runOut(i int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.held[i].run <- time.Time{}
}

func (h *heldTimers) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.held)
}

// beyond waits for a timer after the first n.
func (h *heldTimers) beyond(t *testing.T, n int) {
	t.Helper()
	poll(t, 5*time.Second, fmt.Sprintf("a timer after the first %d", n), func() string {
		if h.count() <= n {
			return "none"
		}
		return ""
	})
}

// rvProxy stands before a rendezvous for one device: it passes the frames
// both ways, changed by fromServer on the way to the device, and keeps them.
type rvProxy struct {
	url        string
	fromServer func([]byte) []byte
	mu         sync.Mutex
	sent, got  []string // what the device sent, and what it was sent
}

func startRVProxy(t *testing.T, back string, fromServer func([]byte) []byte) *rvProxy {
	t.Helper()
	x := &rvProxy{fromServer: fromServer}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		device, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer device.CloseNow()
		server, _, err := websocket.Dial(r.Context(), back, nil)
		if err != nil {
			return
		}
		defer server.CloseNow()
		ctx, cancel := context.WithCancel(r.Context())
		go pumpWS(ctx, cancel, device, server, x.keep(&x.sent, nil))
		pumpWS(ctx, cancel, server, device, x.keep(&x.got, fromServer))
	}))
	t.Cleanup(srv.Close)
	x.url = "ws" + strings.TrimPrefix(srv.URL, "http")
	return x
}

func (x *rvProxy) keep(list *[]string, change func([]byte) []byte) func([]byte) []byte {
	return func(frame []byte) []byte {
		if change != nil {
			frame = change(frame)
		}
		x.mu.Lock()
		*list = append(*list, string(frame))
		x.mu.Unlock()
		return frame
	}
}

// frames returns the kept frames of a list that hold part.
func (x *rvProxy) frames(list *[]string, part string) []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	var out []string
	for _, frame := range *list {
		if strings.Contains(frame, part) {
			out = append(out, frame)
		}
	}
	return out
}

// noHosts makes a rendezvous of this version look like an older one, which
// never says hosts.
func noHosts(frame []byte) []byte {
	return bytes.Replace(frame, []byte(`,"hosts":true`), nil, 1)
}

// codeRig is a home with one group and its owner's browser connected, the
// real home commands, and a Pairer whose paths and timers the test holds.
type codeRig struct {
	*pairHarness
	admin  identity.Key
	owner  *fakeCtl
	group  proto.ID
	timers *heldTimers
	re     int64
	mu     sync.Mutex
	paths  []*peer.Path
}

func newCodeRig(t *testing.T) *codeRig {
	t.Helper()
	admin := sesKey("code-admin")
	r := &codeRig{pairHarness: newPairHarness(t, &admin, false), admin: admin, group: sesID("code-group"), timers: &heldTimers{}, re: 1}
	mustNoErr(t, r.pairer.saveLabel("desk"))
	r.pairer.cfg.After = r.timers.After
	r.pairer.cfg.Paths = func() []*peer.Path {
		r.mu.Lock()
		defer r.mu.Unlock()
		return slices.Clone(r.paths)
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: r.db, Home: r.nodeKey.ID(), Owner: admin.ID(), Logs: r.logs.Log, Forget: r.logs.dvlForget,
		Tables: home.NodeTables(), Now: r.clock.Now, Invite: r.pairer, CodeChanged: r.pairer.CodeChanged,
	})
	mustNoErr(t, err)
	t.Cleanup(loop.Close)
	r.sessions.cfg.Home = loop
	r.sessions.cfg.Known = func(device string) bool { return r.logs.Known(device) || r.pairer.Known(device) }
	r.owner = serveConn(t, r.sessions, admin.ID(), false)
	checkInviteHello(t, r.owner, 1, true, 0)
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(r.pairHarness, "group", r.group, ""), Op: "create"},
		Name:         "Group", MemberName: "Admin",
	}
	create.Nonce = r.group
	r.cmd(t, create)
	return r
}

// cmd sends a command of the owner and returns its answer.
func (r *codeRig) cmd(t *testing.T, payload any) proto.CmdResult {
	t.Helper()
	r.re++
	sendInviteCmd(r.owner, r.admin, r.re, payload)
	res := inviteJoinResult(t, r.owner)
	mustEqual(t, []any{res.Re, bool(res.OK)}, []any{r.re, true})
	return res
}

func (r *codeRig) command(kind string) proto.HomeCommand {
	return inviteCommand(r.pairHarness, kind, r.group, fmt.Sprint(kind, r.re))
}

// code sends a code command of the owner; hours is 0 but for new.
func (r *codeRig) code(t *testing.T, op string, hours int) proto.CmdResult {
	t.Helper()
	cmd := proto.Code{HomeCommand: r.command("code"), Op: op}
	if hours != 0 {
		cmd.Hours = &hours
	}
	return r.cmd(t, cmd)
}

// newCode makes a code and returns its secret and its end, once the code's
// timer is set: the node sets it on a goroutine of its own, so nothing else
// orders it before the timers that come after.
func (r *codeRig) newCode(t *testing.T, hours int) (identity.Secret, int64) {
	t.Helper()
	n := r.timers.count()
	res := r.code(t, "new", hours)
	secret, err := identity.ParseSecret(res.Code)
	mustNoErr(t, err)
	r.timers.beyond(t, n)
	return secret, res.Expires
}

// dial connects the node to a rendezvous, as its runtime does: the path is
// one of the node's, and the open codes are hosted on it.
func (r *codeRig) dial(t *testing.T, url string) *peer.Path {
	t.Helper()
	path, err := peer.Dial(t.Context(), url, r.nodeKey)
	mustNoErr(t, err)
	t.Cleanup(func() { path.Close() })
	r.mu.Lock()
	r.paths = append(r.paths, path)
	r.mu.Unlock()
	r.pairer.HostCodes(path)
	go path.Serve(t.Context(), r.nodePeer, func(kind string, data []byte) error {
		r.pairer.HandleEvent(t.Context(), path, kind, data)
		return nil
	})
	return path
}

// cut ends a connection of the node to a rendezvous.
func (r *codeRig) cut(path *peer.Path) {
	r.mu.Lock()
	r.paths = slices.DeleteFunc(r.paths, func(p *peer.Path) bool { return p == path })
	r.mu.Unlock()
	path.Close()
}

// serve gives the node a connection of a device.
func (r *codeRig) serve(t *testing.T, device identity.Key) *fakeCtl {
	t.Helper()
	ctl, done := newFakeCtl(), make(chan struct{})
	go func() {
		defer close(done)
		r.pairer.Serve(t.Context(), device.ID(), ctl)
	}()
	t.Cleanup(func() { ctl.Close(); <-done })
	return ctl
}

// pairs hands the node the pair message of a device that typed a code, as
// the room frame that came on path. A message that makes the device a peer
// returns once the timer of the peer's 10 minutes is set, which the node
// does on a goroutine of its own.
func (r *codeRig) pairs(t *testing.T, path *peer.Path, device identity.Key, secret identity.Secret) {
	t.Helper()
	n, was := r.timers.count(), r.codePeer(device)
	r.pairer.HandleEvent(t.Context(), path, "room", codePairFrame(secret, secret.Room(), device, "b"))
	if r.codePeer(device) != was {
		r.timers.beyond(t, n)
	}
}

func codePairFrame(secret identity.Secret, room string, device identity.Key, role string) []byte {
	m := proto.Pair{Type: "pair", Key: device.ID(), Kind: "browser", Name: "Guest", Role: role}
	m.MAC = secret.MAC(m)
	return dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: dvlMustEncode(m)})
}

// codeHello sends hello and returns the welcome.
func codeHello(t *testing.T, ctl *fakeCtl) proto.Welcome {
	t.Helper()
	ctl.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 3, Kind: "browser", Name: "browser", Addrs: []string{}})
	var w proto.Welcome
	mustNoErr(t, proto.Decode(ctl.next(t), &w))
	return w
}

// sendJoin presents the device's own log, of one entry, and sends join.
func (r *codeRig) sendJoin(ctl *fakeCtl, device identity.Key, name string) {
	genesis := sesAddEntry(device, device.ID(), 1, device.ID(), "browser", name)
	ctl.push(proto.Devlog{Type: "devlog", User: device.ID(), Entries: []proto.Envelope{genesis}})
	join := inviteCommand(r.pairHarness, "join", r.group, "join-"+name+r.clock.Now().String())
	sendInviteCmd(ctl, device, 2, proto.Join{HomeCommand: join, Name: name})
}

// joins takes a device whose pair message the node has from hello to its
// place in the group, and returns its connection.
func (r *codeRig) joins(t *testing.T, device identity.Key, name string) *fakeCtl {
	t.Helper()
	ctl := r.serve(t, device)
	mustEqual(t, codeHello(t, ctl), r.welcome(1, string(r.group)))
	r.sendJoin(ctl, device, name)
	mustEqual(t, inviteJoinResult(t, ctl), proto.CmdResult{Success: proto.Success{Re: 2, OK: true}})
	waitGroups(t, ctl, 1)
	return ctl
}

// welcome is the welcome of a peer that is not the owner's and reads no
// group; join is the group of a joining peer, or empty.
func (r *codeRig) welcome(re int64, join string) proto.Welcome {
	return proto.Welcome{
		Type: "welcome", Success: proto.Success{Re: re, OK: true}, V: 3, Device: r.nodeKey.ID(), Name: "desk", Version: "v0.3.1",
		Time: r.clock.Now().UnixMilli(), Addrs: []string{r.rvURL}, Groups: []proto.GroupSummary{}, Join: join,
	}
}

// member returns the id and the role of a person's member row, and who let
// them in when they joined.
func (r *codeRig) member(t *testing.T, user identity.Key) (id, role, by string) {
	t.Helper()
	mustNoErr(t, r.db.Write(func(tx *store.Tx) error {
		const row = "SELECT data->>'$.id', data->>'$.role' FROM row WHERE g=? AND r='member' AND data->>'$.user'=?"
		const joined = "SELECT coalesce((SELECT data->>'$.body.by' FROM row WHERE g=? AND r='msg' AND data->>'$.body.member'=?), '')"
		if err := tx.QueryRow(row, string(r.group), user.ID()).Scan(&id, &role); err != nil {
			return err
		}
		return tx.QueryRow(joined, string(r.group), id).Scan(&by)
	}))
	return id, role, by
}

// letIn fails unless the person is a member whom the owner's code let in.
func (r *codeRig) letIn(t *testing.T, user identity.Key) {
	t.Helper()
	owner, _, _ := r.member(t, r.admin)
	_, role, by := r.member(t, user)
	mustEqual(t, []string{role, by}, []string{"member", owner})
}

var codeProbes atomic.Int64

// codeCard joins the room of a code on a rendezvous as a new device and
// returns the pair message of the room's host; ok is false when the server
// answered a later request first, so that nothing hosts the room.
func codeCard(t *testing.T, url string, secret identity.Secret) (card proto.Pair, ok bool) {
	t.Helper()
	path, err := peer.Dial(t.Context(), url, sesKey(fmt.Sprint("code-probe-", codeProbes.Add(1))))
	mustNoErr(t, err)
	defer path.Close()
	got := make(chan *proto.Pair, 2) // single reader: codeCard
	go path.Serve(t.Context(), nil, func(kind string, data []byte) error {
		if m, ok := matchBrowserRoomPeer(data, secret.Room(), secret, "a"); ok {
			got <- &m
		}
		if kind == "quota" {
			got <- nil
		}
		return nil
	})
	mustNoErr(t, path.Write(t.Context(), proto.RoomID{Type: "join", Room: secret.Room()}))
	mustNoErr(t, path.Write(t.Context(), proto.RVFrame{Type: "quota"}))
	if m := recvFrame(t, got); m != nil {
		return *m, true
	}
	return proto.Pair{}, false
}

// waitHosted waits until the room of a code on a rendezvous is hosted by a
// device, or by nobody when host is empty.
func waitHosted(t *testing.T, url string, secret identity.Secret, host string) {
	t.Helper()
	poll(t, 5*time.Second, fmt.Sprintf("host %q of the code's room", host), func() string {
		if card, _ := codeCard(t, url, secret); card.Key != host {
			return "host " + card.Key
		}
		return ""
	})
}

// typeCode is a device that typed a code: it joins the room on a rendezvous
// and answers joined with its pair message. It returns the host's.
func (r *codeRig) typeCode(t *testing.T, url string, device identity.Key, secret identity.Secret) proto.Pair {
	t.Helper()
	path, err := peer.Dial(t.Context(), url, device)
	mustNoErr(t, err)
	t.Cleanup(func() { path.Close() })
	card := typeInRoom(t, path, device, secret)
	waitPairKnown(t, r.pairer, device.ID())
	return card
}

// typeInRoom joins the room of a secret on a path and returns the pair
// message that answers there. A room that nothing answers fails the test
// after 5 seconds.
func typeInRoom(t *testing.T, path *peer.Path, device identity.Key, secret identity.Secret) proto.Pair {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return exchangeBrowserRoom(t, ctx, &simBrowser{key: device, path: path}, secret, "b", "a")
}

func waitClosed(t *testing.T, ctl *fakeCtl, what string) {
	t.Helper()
	select {
	case <-ctl.Done():
	case <-time.After(5 * time.Second):
		t.Fatal(timedOut(5*time.Second, what+" to be closed", nil))
	}
}

func isOpen(ctl *fakeCtl) bool {
	select {
	case <-ctl.Done():
		return false
	default:
		return true
	}
}

// One code lets in any number of people, one after another and at once; each
// device is a joining peer from its own pair message until its person is a
// member (A9.47, the node's half).
func TestCodeAdmitsManyPeople(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	secret, expires := r.newCode(t, 24)
	mustEqual(t, expires, r.clock.Now().Add(24*time.Hour).UnixMilli())
	mustEqual(t, r.timers.lengths(t, 1), []time.Duration{24 * time.Hour})
	want := proto.Pair{Type: "pair", Key: r.nodeKey.ID(), Kind: "node", Name: "desk", Role: "a", Expires: expires}
	want.MAC = secret.MAC(want)

	q, s := sesKey("code-q"), sesKey("code-r")
	for i, device := range []identity.Key{q, s} {
		mustEqual(t, r.typeCode(t, r.rvURL, device, secret), want)
		routes := r.pairer.Routes(device.ID())
		mustEqual(t, []any{len(routes), routes[0].ID}, []any{1, path.Route().ID})
		ctl := r.serve(t, device)
		mustEqual(t, codeHello(t, ctl), r.welcome(1, string(r.group)))
		ctl.push(proto.Sync{Request: proto.Request{Type: "sync", ID: 5}, Group: string(r.group)})
		expectErr(t, ctl.next(t), 5, "forbidden")
		r.sendJoin(ctl, device, fmt.Sprint("Guest", i))
		mustEqual(t, inviteJoinResult(t, ctl), proto.CmdResult{Success: proto.Success{Re: 2, OK: true}})
		waitGroups(t, ctl, 1)
		r.letIn(t, device)
		// Its person is a member: the device is one of a member, and no
		// joining peer any more.
		mustEqual(t, []bool{r.pairer.Known(device.ID()), r.logs.Known(device.ID())}, []bool{false, true})
		again := r.serve(t, device)
		if w := codeHello(t, again); w.Join != "" || len(w.Groups) != 1 {
			t.Fatalf("welcome of a member's device = %+v", w)
		}
	}

	both := []identity.Key{sesKey("code-at-once-1"), sesKey("code-at-once-2"), sesKey("code-at-once-3")}
	var wg sync.WaitGroup
	for i, device := range both {
		wg.Go(func() {
			r.pairs(t, path, device, secret)
			r.joins(t, device, fmt.Sprint("AtOnce", i))
		})
	}
	wg.Wait()
	for _, device := range both {
		r.letIn(t, device)
	}
	mustEqual(t, r.code(t, "show", 0).Code, secret.String())
	mustEqual(t, r.timers.lengths(t, 6), []time.Duration{24 * time.Hour, pairWindow, pairWindow, pairWindow, pairWindow, pairWindow})

	// A member who types the code gets ok and no row is written (A9.52). With
	// that the device's time on the code is over: another joining connection
	// of it is closed, and the end of the 10 minutes closes nothing.
	r.pairs(t, path, q, secret)
	rec, idle := r.codePeer(q), r.serve(t, q)
	mustEqual(t, codeHello(t, idle), r.welcome(1, string(r.group)))
	ctl := r.joins(t, q, "Again")
	waitClosed(t, idle, "the other joining connection of an admitted device")
	mustEqual(t, r.count(t, "member"), 6)
	r.pairer.dropPeer(q.ID(), rec)
	mustEqual(t, []bool{isOpen(ctl), r.pairer.Known(q.ID())}, []bool{true, false})
}

func (r *codeRig) codePeer(device identity.Key) *codePeer {
	r.pairer.mu.Lock()
	defer r.pairer.mu.Unlock()
	return r.pairer.codePeers[device.ID()]
}

// count returns how many rows of a type the group has.
func (r *codeRig) count(t *testing.T, kind string) (n int) {
	t.Helper()
	mustNoErr(t, r.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND r=?", string(r.group), kind).Scan(&n)
	}))
	return n
}

func startRV(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
}

// A code ends at off, at a later new and when the home's clock reaches its
// end: its room is left, the joining connections of people who are no members
// are closed, and the members stay (A9.48, the node's half).
func TestCodeEnds(t *testing.T) {
	ends := map[string]func(*testing.T, *codeRig, *fakeCtl, identity.Key, int64){
		"off": func(t *testing.T, r *codeRig, _ *fakeCtl, _ identity.Key, _ int64) { r.code(t, "off", 0) },
		"new": func(t *testing.T, r *codeRig, _ *fakeCtl, _ identity.Key, _ int64) {
			next, _ := r.newCode(t, 1)
			waitHosted(t, r.rvURL, next, r.nodeKey.ID())
			mustEqual(t, slices.Contains(r.timers.lengths(t, 5), time.Hour), true)
		},
		"time": func(t *testing.T, r *codeRig, waiting *fakeCtl, device identity.Key, expires int64) {
			r.clock.Set(time.UnixMilli(expires - 1))
			r.pairer.CodeChanged("")
			mustEqual(t, r.pairer.Known(device.ID()), true)
			// The home's clock has reached the end and the node's timer has
			// not run out yet: the home refuses the join.
			r.clock.Set(time.UnixMilli(expires))
			r.sendJoin(waiting, device, "Late")
			expectErr(t, waiting.next(t), 2, "forbidden")
			r.timers.runOut(0)
		},
	}
	for name, end := range ends {
		t.Run(name, func(t *testing.T) {
			r := newCodeRig(t)
			path := r.dial(t, r.rvURL)
			secret, expires := r.newCode(t, 24)
			mustEqual(t, r.timers.lengths(t, 1), []time.Duration{24 * time.Hour})
			member, waiting, typed := sesKey("code-member"), sesKey("code-waiting"), sesKey("code-typed")
			for _, device := range []identity.Key{member, waiting, typed} {
				r.pairs(t, path, device, secret)
			}
			memberCtl, waitingCtl := r.joins(t, member, "Member"), r.serve(t, waiting)
			mustEqual(t, codeHello(t, waitingCtl), r.welcome(1, string(r.group)))
			waitHosted(t, r.rvURL, secret, r.nodeKey.ID())
			// A call when nothing changed changes nothing.
			open := r.pairer.codeOfRoom(secret.Room())
			r.pairer.CodeChanged("")
			mustEqual(t, r.pairer.codeOfRoom(secret.Room()) == open, true)
			mustEqual(t, len(r.timers.lengths(t, 4)), 4)

			end(t, r, waitingCtl, waiting, expires)
			waitClosed(t, waitingCtl, "the joining connection of a person who is no member")
			waitHosted(t, r.rvURL, secret, "")
			poll(t, 5*time.Second, "the peers of the ended code to be forgotten", func() string {
				if r.pairer.Known(waiting.ID()) || r.pairer.Known(typed.ID()) {
					return "known"
				}
				return ""
			})
			r.pairs(t, path, sesKey("code-too-late"), secret)
			mustEqual(t, []bool{r.pairer.Known(sesKey("code-too-late").ID()), isOpen(memberCtl)}, []bool{false, true})
			r.letIn(t, member)
		})
	}
}

// Each device is a joining peer for 10 minutes from its pair message, and
// for 10 more from a later one; then its joining connections are closed
// (planning/contracts/step-2/people.md sections 3 and 5).
func TestCodePeerHasTenMinutesFromItsMessage(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	secret, _ := r.newCode(t, 24)
	device, room := sesKey("code-slow"), secret.Room()
	// What is no pair message of the other role under the code of the room
	// makes no peer, and a full for the room ends nothing.
	for _, frame := range [][]byte{
		codePairFrame(identity.NewSecret(), room, device, "b"),
		codePairFrame(secret, room, device, "a"),
		codePairFrame(secret, identity.NewSecret().Room(), device, "b"),
		[]byte(`{"t":"room","room":"` + room + `","data":7}`),
		[]byte(`{"t":"full","room":"` + room + `"}`),
	} {
		r.pairer.HandleEvent(t.Context(), path, "room", frame)
		r.pairer.HandleEvent(t.Context(), path, "full", frame)
		mustEqual(t, []bool{r.pairer.Known(device.ID()), r.pairer.codeOfRoom(room) != nil}, []bool{false, true})
	}
	mustEqual(t, r.pairer.Routes(device.ID()), []peer.Route(nil))

	r.pairs(t, path, device, secret)
	first, ctl := r.codePeer(device), r.serve(t, device)
	mustEqual(t, codeHello(t, ctl), r.welcome(1, string(r.group)))
	r.pairs(t, path, device, secret)
	mustEqual(t, r.timers.lengths(t, 3), []time.Duration{24 * time.Hour, pairWindow, pairWindow})
	// The end of the first message's 10 minutes is not the end of the
	// second's, which the connection now has.
	r.pairer.dropPeer(device.ID(), first)
	mustEqual(t, []bool{isOpen(ctl), r.pairer.Known(device.ID())}, []bool{true, true})
	r.timers.runOut(1)
	r.timers.runOut(2)
	waitClosed(t, ctl, "the joining connection of a device whose 10 minutes ended")
	mustEqual(t, r.pairer.Known(device.ID()), false)
	// From then on its connection is that of a device the node does not know.
	late := r.serve(t, device)
	mustEqual(t, codeHello(t, late), r.welcome(1, ""))
	r.sendJoin(late, device, "Slow")
	expectErr(t, late.next(t), 2, "forbidden")
}

// pairingSecret reads the secret of a pairing or invitation link.
func pairingSecret(t *testing.T, link string) identity.Secret {
	t.Helper()
	_, text, _ := strings.Cut(link, "=")
	secret, err := identity.ParseSecret(text[:19])
	mustNoErr(t, err)
	return secret
}

// On a rendezvous that never says hosts the node sends no join with data and
// no leave; pairing and invitations work there while a code is open, and the
// connection is not closed (A9.55).
func TestCodeOnAnOlderRendezvous(t *testing.T) {
	r := newCodeRig(t)
	back := startRV(t)
	old := startRVProxy(t, back, noHosts)
	r.dial(t, old.url)
	secret, _ := r.newCode(t, 24)
	r.cut(r.pairer.sharedPaths()[0])
	r.dial(t, old.url)
	r.dial(t, r.rvURL)
	waitHosted(t, r.rvURL, secret, r.nodeKey.ID())
	if _, hosted := codeCard(t, back, secret); hosted {
		t.Fatal("the room of a code is hosted on a rendezvous that does not say hosts")
	}
	link, err := r.pairer.PairBrowser(t.Context(), "http://client.example")
	mustNoErr(t, err)
	invited := r.cmd(t, r.command("invite"))
	r.code(t, "off", 0)
	waitHosted(t, r.rvURL, secret, "")
	for i, link := range []string{link.Link, invited.Link} {
		mustEqual(t, r.typeCode(t, back, sesKey(fmt.Sprint("code-old-", i)), pairingSecret(t, link)).Key, r.nodeKey.ID())
	}
	// The node joined the rooms of the pairing and of the invitation there,
	// and sent nothing that only a rendezvous with hosted rooms takes.
	mustEqual(t, len(old.frames(&old.sent, `{"t":"join","room":"`)), 2)
	mustEqual(t, append(old.frames(&old.sent, `{"t":"join","room":"`+secret.Room()), old.frames(&old.sent, `"t":"leave"`)...), []string(nil))
	mustEqual(t, []int{len(old.frames(&old.sent, `"t":"auth"`)), len(old.frames(&old.got, "hosts"))}, []int{2, 0})
}

// Where another device hosts the room of a code the node is answered full,
// which ends nothing: the room is hosted on the node's other paths, and
// there once that connection is made again (A9.55).
func TestCodeRoomThatAnotherDeviceHosts(t *testing.T) {
	r := newCodeRig(t)
	other := startRV(t)
	seen := startRVProxy(t, other, nil)
	r.dial(t, r.rvURL)
	secret, _ := r.newCode(t, 24)
	holder := sesKey("code-holder")
	held, err := peer.Dial(t.Context(), other, holder)
	mustNoErr(t, err)
	t.Cleanup(func() { held.Close() })
	card := proto.Pair{Type: "pair", Key: holder.ID(), Kind: "node", Name: "holder", Role: "a"}
	card.MAC = secret.MAC(card)
	mustNoErr(t, held.Write(t.Context(), proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: secret.Room()}, Data: dvlMustEncode(card)}))
	waitHosted(t, other, secret, holder.ID())

	path := r.dial(t, seen.url)
	poll(t, 5*time.Second, "the node to be answered full", func() string {
		if got := seen.frames(&seen.got, `"t":"full"`); !slices.Equal(got, []string{`{"t":"full","room":"` + secret.Room() + `"}`}) {
			return fmt.Sprint(got)
		}
		return ""
	})
	waitHosted(t, r.rvURL, secret, r.nodeKey.ID())
	waitHosted(t, other, secret, holder.ID())
	mustEqual(t, r.code(t, "show", 0).Code, secret.String())
	// The holder goes. The node's connection there is as it was: it pairs,
	// and it hosts the room only once it is made again.
	held.Close()
	waitHosted(t, other, secret, "")
	link, err := r.pairer.PairBrowser(t.Context(), "http://client.example")
	mustNoErr(t, err)
	mustEqual(t, r.typeCode(t, other, sesKey("code-full-pairs"), pairingSecret(t, link.Link)).Key, r.nodeKey.ID())
	if _, hosted := codeCard(t, other, secret); hosted {
		t.Fatal("the room was hosted again before the connection was made again")
	}
	r.cut(path)
	r.dial(t, seen.url)
	waitHosted(t, other, secret, r.nodeKey.ID())
}

// A code made and a code ended after the path took fresh ICE servers is
// hosted and left as before: every answer to ice says hosts (A9.55).
func TestCodeAfterFreshICEServers(t *testing.T) {
	r := newCodeRig(t)
	fresh := []byte(`"ice":[{"urls":["stun:127.0.0.1:9"]}]`)
	seen := startRVProxy(t, r.rvURL, func(frame []byte) []byte {
		if bytes.Contains(frame, []byte(`"t":"ready"`)) {
			return bytes.Replace(frame, []byte(`"ttl":3600`), []byte(`"ttl":1`), 1)
		}
		if bytes.Contains(frame, []byte(`"t":"ice"`)) {
			return bytes.Replace(frame, []byte(`"ice":[]`), fresh, 1)
		}
		return frame
	})
	path := r.dial(t, seen.url)
	poll(t, 5*time.Second, "the path to take fresh ICE servers", func() string {
		if len(path.Route().ICE) == 0 {
			return "none"
		}
		return ""
	})
	mustEqual(t, seen.frames(&seen.got, `"t":"ice"`), []string{`{"t":"ice",` + string(fresh) + `,"ttl":3600,"hosts":true}`})
	secret, expires := r.newCode(t, 24)
	card := proto.Pair{Type: "pair", Key: r.nodeKey.ID(), Kind: "node", Name: "desk", Role: "a", Expires: expires}
	card.MAC = secret.MAC(card)
	waitHosted(t, r.rvURL, secret, r.nodeKey.ID())
	mustEqual(t, seen.frames(&seen.sent, `"data"`), []string{`{"t":"join","room":"` + secret.Room() + `","data":` + string(dvlMustEncode(card)) + `}`})
	r.code(t, "off", 0)
	waitHosted(t, r.rvURL, secret, "")
	mustEqual(t, seen.frames(&seen.sent, `"t":"leave"`), []string{`{"t":"leave","room":"` + secret.Room() + `"}`})
}

// A change of the codes that cannot be read is reported and changes nothing.
func TestCodeChangeThatCannotBeRead(t *testing.T) {
	r := newCodeRig(t)
	secret, _ := r.newCode(t, 24)
	open := r.pairer.codeOfRoom(secret.Room())
	mustNoErr(t, r.db.Write(func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE setting"); return err }))
	r.pairer.CodeChanged("")
	awaitReportedErr(t, r.reported)
	mustEqual(t, r.pairer.codeOfRoom(secret.Room()) == open && open != nil, true)
}

// An invitation works while a code is open, and a device that has a pair
// message under both joins by the invitation (A9.52).
func TestInvitationBesideACode(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	secret, _ := r.newCode(t, 24)
	invite := r.command("invite")
	a := checkInviteLink(t, r.pairHarness, r.group, r.cmd(t, invite))
	device := sesKey("code-and-link")
	r.pairs(t, path, device, secret)
	m := proto.Pair{Type: "pair", Key: device.ID(), Kind: "browser", Name: "Guest", Role: "b"}
	m.MAC = a.secret.MAC(m)
	r.pairer.acceptPair(a, m)
	r.joins(t, device, "ByLink")
	if _, open := r.pairer.ReplayInvite(r.admin.ID(), string(invite.Nonce)); open {
		t.Fatal("a join on an invitation's connection did not use the invitation up")
	}
	r.letIn(t, device)
	waitHosted(t, r.rvURL, secret, r.nodeKey.ID())
	// The person is a member: the device is no peer of the code any more.
	mustEqual(t, r.pairer.Known(device.ID()), false)
	if w := codeHello(t, r.serve(t, device)); w.Join != "" || len(w.Groups) != 1 {
		t.Fatalf("welcome of a member's device = %+v", w)
	}
}

// Each group has its own code: the end of one leaves the other open, hosted
// and with its peers, and a path that connects hosts those that are open.
func TestCodesOfTwoGroups(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	first, _ := r.newCode(t, 24)
	one, other := r.group, sesID("code-other-group")
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(r.pairHarness, "group", other, ""), Op: "create"},
		Name:         "Other", MemberName: "Admin",
	}
	create.Nonce = other
	r.cmd(t, create)
	r.group = other
	second, _ := r.newCode(t, 1)
	mustEqual(t, r.timers.lengths(t, 2), []time.Duration{24 * time.Hour, time.Hour})
	waitHosted(t, r.rvURL, first, r.nodeKey.ID())
	waitHosted(t, r.rvURL, second, r.nodeKey.ID())
	device := sesKey("code-of-the-other")
	r.pairs(t, path, device, second)
	ctl := r.serve(t, device)
	mustEqual(t, codeHello(t, ctl), r.welcome(1, string(other)))

	r.group = one
	r.code(t, "off", 0)
	waitHosted(t, r.rvURL, first, "")
	waitHosted(t, r.rvURL, second, r.nodeKey.ID())
	mustEqual(t, []bool{r.pairer.Known(device.ID()), isOpen(ctl)}, []bool{true, true})
	// A rendezvous that connects now is hosted the open code alone.
	later := startRV(t)
	r.dial(t, later)
	waitHosted(t, later, second, r.nodeKey.ID())
	if _, hosted := codeCard(t, later, first); hosted {
		t.Fatal("the room of an ended code is hosted on a path that connected later")
	}
}

// A timer does not run on the home's clock. One that runs out before the
// clock has reached the end of the code ends nothing and is set again for
// the time that is left, a second at least; the code ends when the clock is
// there (planning/contracts/step-2/people.md 2.6).
func TestCodeTimerThatRunsOutEarly(t *testing.T) {
	r := newCodeRig(t)
	r.dial(t, r.rvURL)
	secret, expires := r.newCode(t, 24)
	open := r.pairer.codeOfRoom(secret.Room())
	for i, early := range []time.Duration{1500 * time.Millisecond, time.Millisecond} {
		r.clock.Set(time.UnixMilli(expires).Add(-early))
		r.timers.runOut(len(r.timers.lengths(t, i+1)) - 1)
		mustEqual(t, r.timers.lengths(t, i+2)[i+1], max(early, time.Second))
		mustEqual(t, r.pairer.codeOfRoom(secret.Room()) == open, true)
	}
	waitHosted(t, r.rvURL, secret, r.nodeKey.ID())
	r.clock.Set(time.UnixMilli(expires))
	r.timers.runOut(2)
	waitHosted(t, r.rvURL, secret, "")
	mustEqual(t, r.pairer.codeOfRoom(secret.Room()) == nil, true)
}

// A device whose pair message came again after its connection was there, as
// a copy on another path does, is no joining peer once its person is a
// member: its next connection is a member's.
func TestCodeAdmissionAfterALaterMessage(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	secret, _ := r.newCode(t, 24)
	device := sesKey("code-twice")
	r.pairs(t, path, device, secret)
	ctl := r.serve(t, device)
	mustEqual(t, codeHello(t, ctl), r.welcome(1, string(r.group)))
	r.pairs(t, path, device, secret)
	r.sendJoin(ctl, device, "Twice")
	mustEqual(t, inviteJoinResult(t, ctl), proto.CmdResult{Success: proto.Success{Re: 2, OK: true}})
	mustEqual(t, []bool{r.pairer.Known(device.ID()), isOpen(ctl)}, []bool{false, true})
	if w := codeHello(t, r.serve(t, device)); w.Join != "" || len(w.Groups) != 1 {
		t.Fatalf("welcome of a member's device = %+v", w)
	}
}

// A joining connection is closed with the code it came by, also when its
// device has since typed the code of another group: at the end of that code,
// and at the end of its own 10 minutes there.
func TestCodeEndsForADeviceOfTwoCodes(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	first, _ := r.newCode(t, 24)
	one, other := r.group, sesID("code-other-group")
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(r.pairHarness, "group", other, ""), Op: "create"},
		Name:         "Other", MemberName: "Admin",
	}
	create.Nonce = other
	r.cmd(t, create)
	r.group = other
	second, _ := r.newCode(t, 1)
	// Each device types the first code and connects, then types the second;
	// the later one connects again, on the second code.
	early, later := sesKey("code-two-early"), sesKey("code-two-later")
	var onFirst []*fakeCtl
	for _, device := range []identity.Key{early, later} {
		r.pairs(t, path, device, first)
		ctl := r.serve(t, device)
		mustEqual(t, codeHello(t, ctl), r.welcome(1, string(one)))
		r.pairs(t, path, device, second)
		onFirst = append(onFirst, ctl)
	}
	onSecond := r.serve(t, later)
	mustEqual(t, codeHello(t, onSecond), r.welcome(1, string(other)))
	mustEqual(t, r.timers.lengths(t, 6), []time.Duration{24 * time.Hour, time.Hour, pairWindow, pairWindow, pairWindow, pairWindow})

	// The 10 minutes of the later device on the second code end: its
	// connection of the first code stays.
	r.timers.runOut(5)
	waitClosed(t, onSecond, "the joining connection of the second code")
	mustEqual(t, []bool{r.pairer.Known(later.ID()), isOpen(onFirst[1])}, []bool{false, true})
	r.group = one
	r.code(t, "off", 0)
	for _, ctl := range onFirst {
		waitClosed(t, ctl, "a joining connection of the first code, which ended")
	}
	mustEqual(t, r.pairer.Known(early.ID()), true)
}

// A device types the code of one group, then of another, then of the first
// again: the connection that came by the first message is closed when that
// message's 10 minutes end, and the one that came by the third stays.
func TestCodeTypedAgainAfterAnotherCode(t *testing.T) {
	r := newCodeRig(t)
	path := r.dial(t, r.rvURL)
	first, _ := r.newCode(t, 24)
	one, other := r.group, sesID("code-other-group")
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: inviteCommand(r.pairHarness, "group", other, ""), Op: "create"},
		Name:         "Other", MemberName: "Admin",
	}
	create.Nonce = other
	r.cmd(t, create)
	r.group = other
	second, _ := r.newCode(t, 1)
	device := sesKey("code-back-again")
	r.pairs(t, path, device, first)
	byFirst := r.serve(t, device)
	mustEqual(t, codeHello(t, byFirst), r.welcome(1, string(one)))
	r.pairs(t, path, device, second)
	r.pairs(t, path, device, first)
	byThird := r.serve(t, device)
	mustEqual(t, codeHello(t, byThird), r.welcome(1, string(one)))
	mustEqual(t, r.timers.lengths(t, 5), []time.Duration{24 * time.Hour, time.Hour, pairWindow, pairWindow, pairWindow})
	r.timers.runOut(2)
	waitClosed(t, byFirst, "the joining connection of a pair message whose 10 minutes ended")
	mustEqual(t, []bool{isOpen(byThird), r.pairer.Known(device.ID())}, []bool{true, true})
}
