//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["s2_people"] = s2PeopleSteps
}

type s2JoinLink struct {
	Secret  identity.Secret
	Group   string
	Home    string
	Expires int64
}

type s2BrowserView struct {
	groups  []proto.GroupSummary
	humans  map[string]proto.HumanMemberRow
	devices map[string]proto.DeviceRow
	taskID  proto.ID
	texts   map[string]bool
	ais     []string
	gone    map[[2]string]bool
}

func newS2BrowserView() *s2BrowserView {
	return &s2BrowserView{
		humans:  map[string]proto.HumanMemberRow{},
		devices: map[string]proto.DeviceRow{},
		texts:   map[string]bool{},
		gone:    map[[2]string]bool{},
	}
}

func (v *s2BrowserView) applyPush(raw json.RawMessage, wantGroup string) {
	var g proto.Groups
	if proto.Decode(raw, &g) == nil && g.Type == "groups" {
		v.groups = g.Groups
		return
	}
	_, _ = scanS2RowsBatch(raw, wantGroup, func(row json.RawMessage) (bool, error) {
		v.applyRow(row)
		return false, nil
	})
}

func (v *s2BrowserView) applyRow(elem json.RawMessage) {
	var gone proto.GoneRow
	if proto.Decode(elem, &gone) == nil && gone.Gone {
		v.gone[[2]string{gone.Type, gone.ID}] = true
		return
	}
	var hdr proto.RowHeader
	if proto.Decode(elem, &hdr) != nil {
		return
	}
	v.recordLive(hdr.Type, elem)
}

func (v *s2BrowserView) recordLive(typ string, elem json.RawMessage) {
	switch typ {
	case "member":
		v.recordMember(elem)
	case "device":
		v.recordDevice(elem)
	case "task":
		v.recordTask(elem)
	case "msg":
		v.recordMsg(elem)
	}
}

func (v *s2BrowserView) recordMember(elem json.RawMessage) {
	var hum proto.HumanMemberRow
	if proto.Decode(elem, &hum) == nil && hum.Kind == "human" {
		v.humans[hum.ID] = hum
	}
	var ai proto.AIMemberRow
	if proto.Decode(elem, &ai) == nil && ai.Kind == "ai" {
		v.ais = append(v.ais, ai.Name)
	}
}

func (v *s2BrowserView) recordDevice(elem json.RawMessage) {
	var dev proto.DeviceRow
	if proto.Decode(elem, &dev) == nil {
		v.devices[dev.ID] = dev
	}
}

func (v *s2BrowserView) recordTask(elem json.RawMessage) {
	var t proto.TaskRow
	if proto.Decode(elem, &t) == nil {
		v.taskID = proto.ID(t.ID)
	}
}

func (v *s2BrowserView) recordMsg(elem json.RawMessage) {
	var m proto.MsgRow
	if proto.Decode(elem, &m) == nil && m.Kind == "text" {
		v.texts[m.Text] = true
	}
}

func (v *s2BrowserView) humanByUser(userID string) (proto.HumanMemberRow, bool) {
	for _, h := range v.humans {
		if h.User == userID {
			return h, true
		}
	}
	return proto.HumanMemberRow{}, false
}

type s2ClockHome struct {
	mu          sync.Mutex
	now         time.Time
	ctx         context.Context
	rvURL       string
	node        *peerNode
	ids         seqIDGen
	pKey, qKey  identity.Key
	qEntries    []proto.Envelope
	pConn       *peerBrowserConn
	qConn       *peerBrowserConn
	probe       *peerBrowser
	probeConn   *peerBrowserConn
	pView       *s2BrowserView
	gid         proto.ID
	invites     map[string]s2JoinLink
	link        s2JoinLink
	lastJoinRaw json.RawMessage
	lastCmdRaw  json.RawMessage
}

type s2PeopleWorld struct {
	f             fixtures
	cleanup       func(func())
	cast          *s2Cast
	gid           proto.ID
	otherGID      proto.ID
	preJoinText   string
	inviteLink    s2JoinLink
	pView         *s2BrowserView
	qView         *s2BrowserView
	qHomeBC       *peerBrowserConn
	pMemID        string
	qMemID        string
	pWork         proto.AIMemberRow
	qTeam         proto.AIMemberRow
	qPersonal     proto.AIMemberRow
	taskID        proto.ID
	removeReplies []json.RawMessage
	removeAt      time.Time
	qClosed       chan time.Time
	clock         *s2ClockHome
	code          s2CodeState
}

func s2PeopleSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2PeopleWorld{f: f, cleanup: afterScenario(sc)}
	registerS2PeopleA91A92Steps(sc, w)
	registerS2PeopleA93A94Steps(sc, w)
	registerS2PeopleA95A96Steps(sc, w)
	registerS2PeopleGroupSteps(sc, w)
	registerS2PeopleCodeSteps(sc, w)
}

func registerS2PeopleA91A92Steps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^the people cast with P's group "([^"]*)" on P1 and pre-join history "([^"]*)"$`, w.givenCastWithHistory)
	sc.Step(`^P creates an invitation for the people group on P1$`, w.pCreatesInvitation)
	sc.Step(`^the #join invitation link carries the group id, P1's device id and a valid one-time secret$`, w.verifyJoinLinkFields)
	sc.Step(`^Q's browser joins the people group with that #join link as "([^"]*)"$`, w.qBrowserJoinsGroup)
	sc.Step(`^Q has a human member row with role "([^"]*)" and Q's device log is on P1 as device rows$`, w.verifyQMemberAndDevices)
	sc.Step(`^both P's and Q's browsers show Q and the pre-join history without a reload$`, w.verifyBothBrowsersHistory)

	sc.Step(`^an in-process home node P1 for P with group "([^"]*)" under a controllable people clock$`, w.givenControllableHome)
	sc.Step(`^P creates an invitation with nonce "([^"]*)" on the controllable home$`, w.clockCreateInvite)
	sc.Step(`^a joiner browser presents a device log that does not replay and sends join$`, w.clockBadLogJoin)
	sc.Step(`^that join is refused with "([^"]*)" and no member row is added$`, w.clockVerifyBadJoinRefused)
	sc.Step(`^Q's browser joins with that same invitation link as "([^"]*)"$`, w.clockQJoinsFirstLink)
	sc.Step(`^using that invitation link a second time is refused$`, w.expectClockProbeRefused)
	sc.Step(`^P creates a second invitation with nonce "([^"]*)" and 10 minutes pass$`, w.clockCreateAndExpire)
	sc.Step(`^using the expired invitation link is refused$`, w.expectClockProbeRefused)
	sc.Step(`^Q as a member sends invite for the controllable group$`, w.clockQSendInvite)
	sc.Step(`^Q's invite is refused with "([^"]*)"$`, w.clockVerifyLastRefused)
}

func registerS2PeopleA93A94Steps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^the people cast where Q has joined P's group "([^"]*)" on P1 as "([^"]*)"$`, w.givenCastQJoined)
	sc.Step(`^P sets Q's role (?:to|back to) "([^"]*)" in the people group$`, w.pSetsQRole)
	sc.Step(`^group set and invite by Q succeed in the people group$`, w.verifyQAdminPowers)
	sc.Step(`^member set with role by admin Q is refused with "([^"]*)"$`, w.verifyQRoleSetRefused)
	sc.Step(`^group set and invite by Q are refused with "([^"]*)"$`, w.verifyQMemberRefusals)
	sc.Step(`^setting a second owner, changing P's owner role and role changes by Q are refused$`, w.verifyOwnerRoleInvariants)

	sc.Step(`^the people cast where Q joined P's group "([^"]*)" on P1 and joined Q-team and Q-personal from Q1$`, w.givenCastWithQAIs)
	sc.Step(`^removing the owner P is refused with "([^"]*)"$`, w.verifyOwnerCannotBeRemoved)
	sc.Step(`^an unfinished task has Q-team as assignee and then as reviewer$`, w.stageUnfinishedTaskForQTeam)
	sc.Step(`^removing Q while Q-team holds either unfinished task role was refused with "([^"]*)"$`, w.verifyUnfinishedTaskConflicts)
	sc.Step(`^P cancels that task and removes Q from the people group$`, w.cancelTaskAndRemoveQ)
	sc.Step(`^P removes Q from the people group$`, w.removeConnectedQ)
	sc.Step(`^Q's connections to P1 close within 1 second and Q's next connection attempt to P1 is refused$`, w.verifyQDisconnectedAndRefused)
	sc.Step(`^Q1 has ended the sessions of Q's AIs in the people group and forgotten its home$`, w.verifyQ1LeftGroup)
	sc.Step(`^Q's browser receives groups without the group and P sees gone markers for Q and Q's AIs$`, w.verifyQGroupsAndGoneMarkers)
}

func registerS2PeopleA95A96Steps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^the people cast where P's group "([^"]*)" on P1 has a secretary, P also has group "([^"]*)" and Q has joined only the first as "([^"]*)"$`, w.givenCastWithSecretaryAndQ)
	sc.Step(`^Q1 joins Q-team to the people group with host\.join$`, w.q1JoinsQTeam)
	sc.Step(`^Q-team's member row has host Q1, user Q and the folder Q chose as its workspace$`, w.verifyQTeamRowAndClone)
	sc.Step(`^Q's host\.join to Q1 for P's other group is refused with "([^"]*)"$`, w.verifyNonMemberHostAddAIRefused)
	sc.Step(`^host\.join from Q with secretary true while the group has a secretary is refused with "([^"]*)"$`, w.verifyQSecretaryJoinRefused)

	sc.Step(`^the people cast where Q joined "([^"]*)" on P1 with Q-team on Q1 and P has a second group "([^"]*)" on P1$`, w.givenSurfaceCast)
	sc.Step(`^on P1 Q's browser may use the shared group's home surface while host commands, host\.info and sync of P's other group are refused with "([^"]*)"$`, w.verifyP1SurfaceForQ)
	sc.Step(`^Q1 accepts no host command and no takeover\.start from a device of P$`, w.verifyQ1SurfaceForP)
}

func (w *s2PeopleWorld) initCastGroup(ctx context.Context, name string) error {
	cast, err := startS2Cast(w.f.t, w.cleanup, w.f, s2CastConfig{Rendezvous: sharedRVURL(ctx)})
	if err != nil {
		return err
	}
	w.cast = cast
	w.pView = newS2BrowserView()
	w.qView = newS2BrowserView()
	gid, err := cast.CreateGroup(cast.P, cast.P1, name)
	if err != nil {
		return err
	}
	w.gid = gid
	return w.waitPMember()
}

func (w *s2PeopleWorld) waitPMember() error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	err := w.drainViewUntil(w.cast.Ctx, pBC, w.pView, string(w.gid), func() bool {
		h, ok := w.pView.humanByUser(w.cast.P.ID())
		if ok {
			w.pMemID = h.ID
		}
		return ok
	})
	return err
}

func (w *s2PeopleWorld) drainViewUntil(ctx context.Context, bc *peerBrowserConn, view *s2BrowserView, gid string, done func() bool) error {
	drainBufferedView(bc, view, gid)
	for !done() {
		raw, err := bc.nextPush(ctx)
		if err != nil {
			return err
		}
		view.applyPush(raw, gid)
	}
	return nil
}

func drainBufferedView(bc *peerBrowserConn, view *s2BrowserView, gid string) {
	for {
		select {
		case raw, ok := <-bc.pushes:
			if !ok {
				return
			}
			view.applyPush(raw, gid)
		default:
			return
		}
	}
}

func (w *s2PeopleWorld) givenCastWithHistory(ctx context.Context, groupName, historyText string) error {
	if err := w.initCastGroup(ctx, groupName); err != nil {
		return err
	}
	w.preJoinText = historyText
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	err := postS2Mention(w.cast, w.cast.P, w.cast.P1, w.gid, "", historyText)
	if err != nil {
		return err
	}
	return w.drainViewUntil(w.cast.Ctx, pBC, w.pView, string(w.gid), func() bool {
		return w.pView.texts[historyText]
	})
}

func (w *s2PeopleWorld) pCreatesInvitation() error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	res, err := w.cast.SendCmd(w.cast.P, pBC.conn, s2InviteCommand(w.gid, w.cast.NextID(), time.Now().UnixMilli()))
	if err != nil {
		return err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	w.inviteLink = link
	return err
}

func s2InviteCommand(gid, nonce proto.ID, ts int64) proto.Invite {
	return proto.Invite{Type: "invite", Nonce: nonce, Timestamp: ts, Group: gid}
}

func parseS2JoinLink(raw string, expires int64) (s2JoinLink, error) {
	vals, err := parseJoinLinkFragment(raw)
	if err != nil {
		return s2JoinLink{}, err
	}
	sec, err := identity.ParseSecret(vals.Get("join"))
	if err != nil {
		return s2JoinLink{}, fmt.Errorf("invalid #join secret in %q: %v", raw, err)
	}
	return s2JoinLink{Secret: sec, Group: vals.Get("g"), Home: vals.Get("home"), Expires: expires}, nil
}

func parseJoinLinkFragment(raw string) (url.Values, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid invite URL %q: %v", raw, err)
	}
	vals, err := url.ParseQuery(u.Fragment)
	if err != nil {
		return nil, fmt.Errorf("unexpected invite fragment %q (%v)", u.Fragment, err)
	}
	return vals, nil
}

func (w *s2PeopleWorld) verifyJoinLinkFields() error {
	now := time.Now().UnixMilli()
	if w.inviteLink.Group != string(w.gid) || w.inviteLink.Home != w.cast.P1.ID() {
		return fmt.Errorf("invite link = %+v, want g=%s home=%s", w.inviteLink, w.gid, w.cast.P1.ID())
	}
	if w.inviteLink.Expires <= now+9*time.Minute.Milliseconds() || w.inviteLink.Expires > now+10*time.Minute.Milliseconds() {
		return fmt.Errorf("invite expires = %d, now = %d", w.inviteLink.Expires, now)
	}
	return nil
}

func (w *s2PeopleWorld) qBrowserJoinsGroup(memberName string) error {
	bc, _, err := w.cast.JoinGroup(w.cast.Q, w.inviteLink, memberName)
	if err != nil {
		return err
	}
	w.qHomeBC = bc
	return w.awaitQJoinedOnBothViews()
}

func joinGroupWithLink(
	ctx context.Context, cleanup func(func()), b *peerBrowser, signer identity.Key,
	entries []proto.Envelope, link s2JoinLink, memberName string, joinNonce proto.ID, now time.Time,
) (*peerBrowserConn, json.RawMessage, error) {
	bc, err := openJoiningCtl(ctx, cleanup, b, link)
	if err != nil {
		return nil, nil, err
	}
	raw, err := sendJoinOn(ctx, bc, signer, entries, link, memberName, joinNonce, now)
	return bc, raw, err
}

// sendJoinOn presents the device log on a joining connection and sends join for the link's group.
func sendJoinOn(
	ctx context.Context, bc *peerBrowserConn, signer identity.Key, entries []proto.Envelope,
	link s2JoinLink, memberName string, joinNonce proto.ID, now time.Time,
) (json.RawMessage, error) {
	if err := bc.conn.Send(proto.Devlog{Type: "devlog", User: signer.ID(), Entries: entries}); err != nil {
		return nil, err
	}
	return sendSignedCmdRaw(ctx, bc.conn, signer, proto.Join{
		HomeCommand: proto.HomeCommand{Type: "join", Nonce: joinNonce, Timestamp: now.UnixMilli(), Group: proto.ID(link.Group)},
		Name:        memberName,
	})
}

func openJoiningCtl(ctx context.Context, cleanup func(func()), b *peerBrowser, link s2JoinLink) (*peerBrowserConn, error) {
	if _, err := b.exchangeRoom(ctx, link.Secret, "b", "a"); err != nil {
		return nil, err
	}
	bc, err := openS2PeopleCtl(ctx, cleanup, b, link.Home)
	if err != nil {
		return nil, err
	}
	if err := verifyJoiningWelcome(ctx, bc.conn); err != nil {
		return nil, err
	}
	return bc, nil
}

func openS2PeopleCtl(ctx context.Context, cleanup func(func()), b *peerBrowser, homeID string) (*peerBrowserConn, error) {
	var saw trail
	bc, ok, err := b.tryOpenCtl(ctx, cleanup, homeID, s2ScenarioTimeout, &saw)
	if err == nil && !ok {
		err = saw.timedOut(s2ScenarioTimeout, "ctl to "+homeID)
	}
	return bc, err
}

func verifyJoiningWelcome(ctx context.Context, conn *peer.Connection) error {
	raw, err := requestWait(ctx, conn, nil, func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 3, Kind: "browser", Name: "Joiner", Addrs: []string{}}
	})
	if err != nil {
		return err
	}
	var welc proto.Welcome
	if err := proto.Decode(raw, &welc); err != nil {
		return err
	}
	if welc.V != 3 || welc.Owner || len(welc.Groups) != 0 {
		return fmt.Errorf("joining welcome = %+v, want v=3, owner=false and empty groups", welc)
	}
	return nil
}

func (w *s2PeopleWorld) awaitQJoinedOnBothViews() error {
	if err := w.drainViewUntil(w.cast.Ctx, w.qHomeBC, w.qView, string(w.gid), func() bool {
		return slices.ContainsFunc(w.qView.groups, func(g proto.GroupSummary) bool { return g.Group == string(w.gid) })
	}); err != nil {
		return err
	}
	if err := w.awaitQJoinedOnView(w.qView, w.qHomeBC); err != nil {
		return err
	}
	return w.awaitQJoinedOnView(w.pView, w.cast.P.Conns[w.cast.P1.ID()])
}

func (w *s2PeopleWorld) awaitQJoinedOnView(view *s2BrowserView, bc *peerBrowserConn) error {
	return w.drainViewUntil(w.cast.Ctx, bc, view, string(w.gid), func() bool {
		h, ok := view.humanByUser(w.cast.Q.ID())
		if ok {
			w.qMemID = h.ID
		}
		return ok && countUserDevices(view, w.cast.Q.ID()) == len(w.cast.Q.Entries)
	})
}

func countUserDevices(view *s2BrowserView, userID string) int {
	n := 0
	for _, d := range view.devices {
		if d.User == userID {
			n++
		}
	}
	return n
}

func (w *s2PeopleWorld) verifyQMemberAndDevices(wantRole string) error {
	qRow, ok := w.pView.humanByUser(w.cast.Q.ID())
	if !ok || qRow.Role != wantRole || qRow.Name != w.cast.Q.Name {
		return fmt.Errorf("P view Q human row = %+v (ok=%v), want role %q", qRow, ok, wantRole)
	}
	if got := countUserDevices(w.pView, w.cast.Q.ID()); got != len(w.cast.Q.Entries) {
		return fmt.Errorf("Q device rows on P1 = %d, want %d", got, len(w.cast.Q.Entries))
	}
	return verifyS2DeviceRows(w.pView, w.cast.Q.Entries)
}

func verifyS2DeviceRows(view *s2BrowserView, entries []proto.Envelope) error {
	for _, env := range entries {
		var entry proto.DeviceEntry
		if err := proto.Decode([]byte(env.Payload), &entry); err != nil {
			return err
		}
		id := fmt.Sprintf("%s:%d", entry.User, entry.Version)
		row := view.devices[id]
		if row.Env != env || row.DeviceChange != entry.DeviceChange {
			return fmt.Errorf("device row %s = %+v, want entry %+v", id, row, entry)
		}
	}
	return nil
}

func (w *s2PeopleWorld) verifyBothBrowsersHistory() error {
	for bc, view := range map[*peerBrowserConn]*s2BrowserView{w.cast.P.Conns[w.cast.P1.ID()]: w.pView, w.qHomeBC: w.qView} {
		if err := w.drainViewUntil(w.cast.Ctx, bc, view, string(w.gid), func() bool {
			return view.texts[w.preJoinText]
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *s2ClockHome) nowTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *s2ClockHome) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func (w *s2PeopleWorld) givenControllableHome(ctx context.Context, groupName string) error {
	runCtx, cancel := context.WithTimeout(w.f.t.Context(), s2ScenarioTimeout)
	w.cleanup(cancel)
	rvURL, err := startAcceptanceRelayRVWithBack(w.cleanup, sharedRVURL(ctx))
	if err != nil {
		return err
	}
	ch := &s2ClockHome{
		now:     time.UnixMilli(1763700000000),
		ctx:     runCtx,
		rvURL:   rvURL,
		pKey:    newRandomPeerKey(),
		qKey:    newRandomPeerKey(),
		pView:   newS2BrowserView(),
		invites: map[string]s2JoinLink{},
	}
	w.clock = ch
	return w.bootControllableHome(groupName)
}

func (w *s2PeopleWorld) bootControllableHome(groupName string) error {
	ch := w.clock
	nodeKey := newRandomPeerKey()
	pn, err := startPeerNode(w.f.t, w.cleanup, peerNodeConfig{
		Ctx: ch.ctx, Key: nodeKey, Name: "P1-clock", Owner: ch.pKey, Rendezvous: ch.rvURL, Now: ch.nowTime,
	})
	if err != nil {
		return err
	}
	ch.node = pn
	ts := ch.nowTime().UnixMilli()
	qNodeKey := newRandomPeerKey()
	ch.qEntries = buildPeerOwnerLog(ch.qKey, qNodeKey, "Q1", nil, ts)
	return w.connectControllableOwnerAndGroup(groupName)
}

func (w *s2PeopleWorld) connectControllableOwnerAndGroup(groupName string) error {
	ch := w.clock
	pb, err := newPeerBrowser(ch.ctx, w.cleanup, ch.pKey, ch.rvURL, nil)
	if err != nil {
		return err
	}
	if err := pb.waitOnline(ch.ctx, ch.node.Key.ID()); err != nil {
		return err
	}
	bc, err := openS2PeopleCtl(ch.ctx, w.cleanup, pb, ch.node.Key.ID())
	if err != nil {
		return err
	}
	ch.pConn = bc
	if err := checkOwnerWelcome(ch.ctx, bc.conn, ch.node.Key.ID()); err != nil {
		return err
	}
	return w.createControllableGroup(groupName)
}

func (w *s2PeopleWorld) createControllableGroup(groupName string) error {
	ch := w.clock
	gid := ch.ids.next()
	ch.gid = gid
	raw, err := sendSignedCmdRaw(ch.ctx, ch.pConn.conn, ch.pKey, proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: ch.nowTime().UnixMilli(), Group: gid},
			Op:          "create",
		},
		Name:       groupName,
		MemberName: "P",
	})
	if err != nil {
		return err
	}
	var ok proto.CmdResult
	if err := proto.Decode(raw, &ok); err != nil {
		return fmt.Errorf("create group: %s (%w)", raw, err)
	}
	if err := syncS2Group(ch.ctx, ch.pConn.conn, gid); err != nil {
		return err
	}
	return w.drainViewUntil(ch.ctx, ch.pConn, ch.pView, string(gid), func() bool {
		return len(ch.pView.humans) == 1
	})
}

func (w *s2PeopleWorld) clockCreateInvite(tag string) error {
	ch := w.clock
	ts := ch.nowTime().UnixMilli()
	raw, err := sendSignedCmdRaw(ch.ctx, ch.pConn.conn, ch.pKey, s2InviteCommand(ch.gid, exampleID(tag), ts))
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("invite %s: %s (%w)", tag, raw, err)
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	ch.invites[tag], ch.link = link, link
	return err
}

func (w *s2PeopleWorld) clockBadLogJoin() error {
	ch := w.clock
	key := newRandomPeerKey()
	b, err := newPeerBrowser(ch.ctx, w.cleanup, key, ch.rvURL, nil)
	if err != nil {
		return err
	}
	entries := []proto.Envelope{nodetest.SignDeviceAdd(key, key.ID(), 2, "browser", "Joiner", key.ID(), ch.nowTime().UnixMilli())}
	_, raw, err := joinGroupWithLink(ch.ctx, w.cleanup, b, key, entries, ch.invites["inv-1"], "Bad", ch.ids.next(), ch.nowTime())
	ch.lastJoinRaw = raw
	if err != nil {
		return err
	}
	return leaveS2PairingRoom(ch.ctx, ch.rvURL, b)
}

func leaveS2PairingRoom(ctx context.Context, rvURL string, b *peerBrowser) error {
	observer, err := peer.Dial(ctx, rvURL, newRandomPeerKey())
	if err != nil {
		return err
	}
	defer observer.Close()
	offline := make(chan struct{})
	go observer.Serve(ctx, nil, s2OfflineEvent(b.key.ID(), offline))
	if err := observer.Write(ctx, proto.Watch{Type: "watch", Keys: []string{b.key.ID()}}); err != nil {
		return err
	}
	if err := b.path.Close(); err != nil {
		return err
	}
	select {
	case <-offline:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func s2OfflineEvent(key string, offline chan struct{}) func(string, []byte) error {
	return func(kind string, data []byte) error {
		var pr proto.Presence
		if kind == "presence" && proto.Decode(data, &pr) == nil && pr.Key == key && !pr.On {
			close(offline)
		}
		return nil
	}
}

func (w *s2PeopleWorld) clockVerifyBadJoinRefused(wantCode string) error {
	ch := w.clock
	if err := checkWireError(ch.lastJoinRaw, wantCode); err != nil {
		return err
	}
	snapshot, err := ch.node.Store.Snapshot(string(ch.gid))
	if err != nil {
		return err
	}
	defer snapshot.Close()
	rows, err := snapshot.StateRows()
	if err != nil {
		return err
	}
	view := newS2BrowserView()
	for _, row := range rows {
		view.applyRow(row)
	}
	if len(view.humans) != 1 {
		return fmt.Errorf("members after invalid log join = %+v, want only P", view.humans)
	}
	return nil
}

func (w *s2PeopleWorld) clockQJoinsFirstLink(memberName string) error {
	ch := w.clock
	link := ch.invites["inv-1"]
	if err := w.prepareClockProbe(link); err != nil {
		return err
	}
	qb, err := newPeerBrowser(ch.ctx, w.cleanup, ch.qKey, ch.rvURL, nil)
	if err != nil {
		return err
	}
	bc, raw, err := joinGroupWithLink(ch.ctx, w.cleanup, qb, ch.qKey, ch.qEntries, link, memberName, ch.ids.next(), ch.nowTime())
	if err != nil {
		return err
	}
	ch.qConn = bc
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return err
	}
	return leaveS2PairingRoom(ch.ctx, ch.rvURL, qb)
}

func (w *s2PeopleWorld) prepareClockProbe(link s2JoinLink) error {
	ch := w.clock
	key := newRandomPeerKey()
	entries := buildPeerOwnerLog(key, newRandomPeerKey(), "Joiner-node", nil, ch.nowTime().UnixMilli())
	b, err := newPeerBrowser(ch.ctx, w.cleanup, key, ch.rvURL, nil)
	if err != nil {
		return err
	}
	ch.probe = b
	bc, err := openJoiningCtl(ch.ctx, w.cleanup, b, link)
	if err != nil {
		return err
	}
	ch.probeConn = bc
	if err := bc.conn.Send(proto.Devlog{Type: "devlog", User: key.ID(), Entries: entries}); err != nil {
		return err
	}
	return leaveS2PairingRoom(ch.ctx, ch.rvURL, b)
}

func (w *s2PeopleWorld) expectClockProbeRefused() error {
	ch := w.clock
	raw, err := sendSignedCmdRaw(ch.ctx, ch.probeConn.conn, ch.probe.key, proto.Join{
		HomeCommand: proto.HomeCommand{Type: "join", Nonce: ch.ids.next(), Timestamp: ch.nowTime().UnixMilli(), Group: ch.gid},
		Name:        "Joiner",
	})
	if err != nil {
		return err
	}
	if err := checkWireError(raw, "forbidden"); err != nil {
		return err
	}
	return w.expectNewcomerRefused()
}

// expectNewcomerRefused has a browser that the home never met enter the room of the last invitation and offer.
func (w *s2PeopleWorld) expectNewcomerRefused() error {
	ch := w.clock
	b, err := newPeerBrowser(ch.ctx, w.cleanup, newRandomPeerKey(), ch.rvURL, nil)
	if err != nil {
		return err
	}
	room := ch.link.Secret.Room()
	if err := b.path.Write(ch.ctx, proto.RoomID{Type: "join", Room: room}); err != nil {
		return err
	}
	self := proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: b.pairSelf(ch.link.Secret, "b")}
	if err := b.path.Write(ch.ctx, self); err != nil {
		return err
	}
	if err := b.waitOnline(ch.ctx, ch.link.Home); err != nil { // the rendezvous has passed the pair message on
		return err
	}
	if err := expectNeverOpens(ch.ctx, b, ch.link.Home); err != nil {
		return err
	}
	return leaveS2PairingRoom(ch.ctx, ch.rvURL, b)
}

// expectNeverOpens offers once; the home answers nothing, and the offer is retired when its wait for an answer ends.
func expectNeverOpens(ctx context.Context, b *peerBrowser, home string) error {
	conn, err := b.peer.Offer(ctx, home)
	if err != nil {
		return err
	}
	defer conn.Close()
	select {
	case <-conn.Ready():
		return fmt.Errorf("%s accepted a new connection from %s", home, b.key.ID())
	case <-conn.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *s2PeopleWorld) clockCreateAndExpire(tag string) error {
	if err := w.clockCreateInvite(tag); err != nil {
		return err
	}
	if err := w.prepareClockProbe(w.clock.invites[tag]); err != nil {
		return err
	}
	w.clock.advance(10 * time.Minute)
	return nil
}

func (w *s2PeopleWorld) clockQSendInvite() error {
	ch := w.clock
	raw, err := sendSignedCmdRaw(ch.ctx, ch.qConn.conn, ch.qKey, s2InviteCommand(ch.gid, ch.ids.next(), ch.nowTime().UnixMilli()))
	ch.lastCmdRaw = raw
	return err
}

func (w *s2PeopleWorld) clockVerifyLastRefused(wantCode string) error {
	return checkWireError(w.clock.lastCmdRaw, wantCode)
}

func (w *s2PeopleWorld) givenCastQJoined(ctx context.Context, groupName, qName string) error {
	if err := w.initCastGroup(ctx, groupName); err != nil {
		return err
	}
	if err := w.pCreatesInvitation(); err != nil {
		return err
	}
	return w.qBrowserJoinsGroup(qName)
}

func (w *s2PeopleWorld) pSetsQRole(role string) error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	_, err := w.cast.SendCmd(w.cast.P, pBC.conn, w.roleSetCommand(w.qMemID, role))
	if err != nil {
		return err
	}
	return w.drainViewUntil(w.cast.Ctx, pBC, w.pView, string(w.gid), func() bool {
		return w.pView.humans[w.qMemID].Role == role
	})
}

func (w *s2PeopleWorld) verifyQAdminPowers() error {
	renamed := "Alpha-by-Q"
	_, err := w.cast.SendCmd(w.cast.Q, w.qHomeBC.conn, proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{
				Type: "group", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
			},
			Op: "set",
		},
		Name: &renamed,
	})
	if err != nil {
		return err
	}
	res, err := w.cast.SendCmd(w.cast.Q, w.qHomeBC.conn, s2InviteCommand(w.gid, w.cast.NextID(), time.Now().UnixMilli()))
	if err != nil || res.Link == "" || res.Expires == 0 {
		return fmt.Errorf("admin Q invite = %+v, %v", res, err)
	}
	return nil
}

func (w *s2PeopleWorld) verifyQRoleSetRefused(wantCode string) error {
	return w.expectSignedCmdError(w.qHomeBC.conn, w.cast.Q.Key, w.roleSetCommand(w.qMemID, "member"), wantCode)
}

func (w *s2PeopleWorld) verifyQMemberRefusals(wantCode string) error {
	renamed := "Nope"
	err := w.expectSignedCmdError(w.qHomeBC.conn, w.cast.Q.Key, proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{
				Type: "group", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
			},
			Op: "set",
		},
		Name: &renamed,
	}, wantCode)
	if err != nil {
		return err
	}
	return w.expectSignedCmdError(w.qHomeBC.conn, w.cast.Q.Key, s2InviteCommand(w.gid, w.cast.NextID(), time.Now().UnixMilli()), wantCode)
}

func (w *s2PeopleWorld) verifyOwnerRoleInvariants() error {
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	if err := w.expectRoleSetError(pConn, w.cast.P.Key, w.qMemID, "owner", "bad_request"); err != nil {
		return err
	}
	if err := w.expectRoleSetError(pConn, w.cast.P.Key, w.pMemID, "admin", "forbidden"); err != nil {
		return err
	}
	return w.expectRoleSetError(w.qHomeBC.conn, w.cast.Q.Key, w.qMemID, "admin", "forbidden")
}

func (w *s2PeopleWorld) expectRoleSetError(conn *peer.Connection, signer identity.Key, targetID, role, wantCode string) error {
	return w.expectSignedCmdError(conn, signer, w.roleSetCommand(targetID, role), wantCode)
}

func (w *s2PeopleWorld) roleSetCommand(targetID, role string) proto.MemberSet {
	return proto.MemberSet{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid},
			Op:          "set", ID: proto.ID(targetID),
		},
		Role: &role,
	}
}

func (w *s2PeopleWorld) expectSignedCmdError(conn *peer.Connection, signer identity.Key, payload any, wantCode string) error {
	raw, err := sendSignedCmdRaw(w.cast.Ctx, conn, signer, payload)
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func (w *s2PeopleWorld) givenCastWithQAIs(ctx context.Context, groupName string) error {
	if err := w.initCastGroup(ctx, groupName); err != nil {
		return err
	}
	if err := w.joinPAIsOnP1AndP2(); err != nil {
		return err
	}
	if err := w.pCreatesInvitation(); err != nil {
		return err
	}
	if err := w.qBrowserJoinsGroup(w.cast.Q.Name); err != nil {
		return err
	}
	return w.joinBothQAIs()
}

func (w *s2PeopleWorld) joinPAIsOnP1AndP2() error {
	_, err := w.cast.JoinAI(w.cast.P, w.cast.P1, w.cast.P1, w.gid, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	pw, err := w.cast.JoinAI(w.cast.P, w.cast.P2, w.cast.P1, w.gid, s2PWorkCLI, s2PWorkName, false)
	w.pWork = pw
	return err
}

func (w *s2PeopleWorld) joinBothQAIs() error {
	qt, err := w.cast.JoinAI(w.cast.Q, w.cast.Q1, w.cast.P1, w.gid, s2QTeamCLI, s2QTeamName, false)
	if err != nil {
		return err
	}
	w.qTeam = qt
	qp, err := w.cast.JoinAI(w.cast.Q, w.cast.Q1, w.cast.P1, w.gid, s2QPersonalCLI, s2QPersonalName, false)
	w.qPersonal = qp
	return err
}

func (w *s2PeopleWorld) verifyOwnerCannotBeRemoved(wantCode string) error {
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	if err := w.expectRemoveError(pConn, w.cast.P.Key, w.pMemID, wantCode); err != nil {
		return err
	}
	return w.expectRemoveError(w.qHomeBC.conn, w.cast.Q.Key, w.pMemID, wantCode)
}

func (w *s2PeopleWorld) expectRemoveError(conn *peer.Connection, signer identity.Key, memID, wantCode string) error {
	raw, err := w.sendRemove(conn, signer, memID)
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func (w *s2PeopleWorld) stageUnfinishedTaskForQTeam() error {
	if err := w.startDirectTaskByPWork(); err != nil {
		return err
	}
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	for _, roles := range [][2]proto.ID{{proto.ID(w.qTeam.ID), proto.ID(w.pWork.ID)}, {proto.ID(w.pWork.ID), proto.ID(w.qTeam.ID)}} {
		if err := w.assignTaskRoles(roles[0], roles[1]); err != nil {
			return err
		}
		raw, err := w.sendRemove(pConn, w.cast.P.Key, w.qMemID)
		if err != nil {
			return err
		}
		w.removeReplies = append(w.removeReplies, raw)
	}
	return nil
}

func (w *s2PeopleWorld) startDirectTaskByPWork() error {
	script := m5ClaudeScript([]m5ToolCallSpec{
		{Name: "start_task", Args: map[string]any{"title": "Hold Q-team busy", "plan": "Keep open"}},
	})
	if err := w.cast.WriteScript(w.cast.P2, s2PWorkCLI, script); err != nil {
		return err
	}
	if err := postS2Mention(w.cast, w.cast.P, w.cast.P1, w.gid, w.pWork.ID, "Start a task"); err != nil {
		return err
	}
	if err := w.drainViewUntil(w.cast.Ctx, w.cast.P.Conns[w.cast.P1.ID()], w.pView, string(w.gid), func() bool {
		w.taskID = w.pView.taskID
		return w.taskID != ""
	}); err != nil {
		return err
	}
	return w.cast.ClearScript(w.cast.P2, s2PWorkCLI)
}

func (w *s2PeopleWorld) assignTaskRoles(assignee, reviewer proto.ID) error {
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	_, err := w.cast.SendCmd(w.cast.P, pConn, proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
			},
			Op: "assign",
			ID: w.taskID,
		},
		Assignee: assignee,
		Reviewer: reviewer,
	})
	return err
}

func (w *s2PeopleWorld) sendRemove(conn *peer.Connection, signer identity.Key, memID string) (json.RawMessage, error) {
	return sendSignedCmdRaw(w.cast.Ctx, conn, signer, proto.MemberCommand{
		HomeCommand: proto.HomeCommand{
			Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
		Op: "remove",
		ID: proto.ID(memID),
	})
}

func (w *s2PeopleWorld) verifyUnfinishedTaskConflicts(wantCode string) error {
	for _, raw := range w.removeReplies {
		if err := checkWireError(raw, wantCode); err != nil {
			return err
		}
	}
	return nil
}

func (w *s2PeopleWorld) cancelTaskAndRemoveQ() error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	_, err := w.cast.SendCmd(w.cast.P, pBC.conn, proto.TaskCommand{
		HomeCommand: proto.HomeCommand{
			Type: "task", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
		Op: "cancel",
		ID: w.taskID,
	})
	if err != nil {
		return err
	}
	return w.removeQ()
}

func (w *s2PeopleWorld) removeConnectedQ() error {
	if err := w.expectQConnectedToP1(); err != nil {
		return err
	}
	return w.removeQ()
}

func (w *s2PeopleWorld) removeQ() error {
	w.removeAt = time.Now()
	w.qClosed = make(chan time.Time, 1)
	go func() {
		select {
		case <-w.qHomeBC.conn.Done():
			w.qClosed <- time.Now()
		case <-w.cast.Ctx.Done():
		}
	}()
	raw, err := w.sendRemove(w.cast.P.Conns[w.cast.P1.ID()].conn, w.cast.P.Key, w.qMemID)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	return proto.Decode(raw, &res)
}

func (w *s2PeopleWorld) verifyQDisconnectedAndRefused() error {
	deadline := w.removeAt.Add(time.Second)
	if err := w.waitQBrowserClosed(deadline); err != nil {
		return err
	}
	if err := w.waitP1DroppedQDevices(deadline); err != nil {
		return err
	}
	return expectNeverOpens(w.cast.Ctx, w.cast.Q.Browser, w.cast.P1.ID())
}

func (w *s2PeopleWorld) verifyQ1LeftGroup() error {
	return waitUntilCtx(w.cast.Ctx, 100*time.Millisecond, "Q1 to leave the group", func() error {
		info, err := requestHostInfo(w.cast.Ctx, w.cast.Q.Conns[w.cast.Q1.ID()].conn)
		if err != nil {
			return abort(err)
		}
		kept := slices.ContainsFunc(info.Homes, func(h proto.Home) bool { return h.Group == w.gid })
		for _, agent := range info.Agents {
			kept = kept || slices.ContainsFunc(agent.Groups, func(g proto.AgentGroup) bool { return g.Group == w.gid })
		}
		if kept {
			return fmt.Errorf("Q1 still has homes %v and agents %+v", info.Homes, info.Agents)
		}
		return nil
	})
}

func (w *s2PeopleWorld) waitQBrowserClosed(deadline time.Time) error {
	select {
	case at := <-w.qClosed:
		if at.After(deadline) {
			return fmt.Errorf("Q browser ctl closed after %s, want within 1 second", at.Sub(w.removeAt))
		}
		return nil
	case <-w.cast.Ctx.Done():
		return w.cast.Ctx.Err()
	}
}

func (w *s2PeopleWorld) waitP1DroppedQDevices(deadline time.Time) error {
	ctx, cancel := context.WithDeadline(w.cast.Ctx, deadline)
	defer cancel()
	return waitUntilCtx(ctx, 100*time.Millisecond, "P1 to drop Q's connections", func() error {
		devices, err := w.qDevicesOnP1(ctx)
		if err != nil {
			return abort(err)
		}
		if len(devices) != 0 {
			return fmt.Errorf("P1 lists connections of %v", devices)
		}
		return nil
	})
}

func (w *s2PeopleWorld) expectQConnectedToP1() error {
	devices, err := w.qDevicesOnP1(w.cast.Ctx)
	if err == nil && len(devices) != 2 {
		err = fmt.Errorf("P1 lists connections of %v, want Q's browser and Q1", devices)
	}
	return err
}

func (w *s2PeopleWorld) qDevicesOnP1(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, w.cast.F.bin, "status")
	cmd.Env = w.cast.P1.Env
	out, err := cmd.CombinedOutput()
	return s2StatusConnections(string(out), w.cast.Q.ID(), w.cast.Q1.ID()), err
}

func s2StatusConnections(status string, devices ...string) []string {
	var connected []string
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "connection" && slices.Contains(devices, fields[1]) {
			connected = append(connected, fields[1])
		}
	}
	return connected
}

func (w *s2PeopleWorld) verifyQGroupsAndGoneMarkers() error {
	if err := w.drainViewUntil(w.cast.Ctx, w.qHomeBC, w.qView, string(w.gid), func() bool {
		return !slices.ContainsFunc(w.qView.groups, func(g proto.GroupSummary) bool { return g.Group == string(w.gid) })
	}); err != nil {
		return err
	}
	return w.awaitPGoneMarkers()
}

func (w *s2PeopleWorld) awaitPGoneMarkers() error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	return w.drainViewUntil(w.cast.Ctx, pBC, w.pView, string(w.gid), func() bool {
		return w.hasAllQGoneMarkers()
	})
}

func (w *s2PeopleWorld) hasAllQGoneMarkers() bool {
	for _, id := range []string{w.qMemID, w.qTeam.ID, w.qPersonal.ID} {
		if !w.pView.gone[[2]string{"member", id}] {
			return false
		}
	}
	return true
}

func (w *s2PeopleWorld) givenCastWithSecretaryAndQ(ctx context.Context, groupName, soloName, qName string) error {
	if err := w.initCastGroup(ctx, groupName); err != nil {
		return err
	}
	_, err := w.cast.JoinAI(w.cast.P, w.cast.P1, w.cast.P1, w.gid, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	w.otherGID, err = w.cast.CreateGroup(w.cast.P, w.cast.P1, soloName)
	if err != nil {
		return err
	}
	if err := w.pCreatesInvitation(); err != nil {
		return err
	}
	return w.qBrowserJoinsGroup(qName)
}

func (w *s2PeopleWorld) q1JoinsQTeam() error {
	qt, err := w.cast.JoinAI(w.cast.Q, w.cast.Q1, w.cast.P1, w.gid, s2QTeamCLI, s2QTeamName, false)
	w.qTeam = qt
	return err
}

func (w *s2PeopleWorld) verifyQTeamRowAndClone() error {
	if w.qTeam.Host != w.cast.Q1.ID() || w.qTeam.User != w.cast.Q.ID() {
		return fmt.Errorf("Q-team row = %+v, want host=%s user=%s", w.qTeam, w.cast.Q1.ID(), w.cast.Q.ID())
	}
	want, err := w.cast.Workspace(w.cast.Q1, w.gid, s2QTeamName)
	if err != nil || w.qTeam.Workspace != want {
		return fmt.Errorf("Q-team workspace = %q, want %q (%v)", w.qTeam.Workspace, want, err)
	}
	return nil
}

func (w *s2PeopleWorld) verifyNonMemberHostAddAIRefused(wantCode string) error {
	return w.expectSignedCmdError(w.cast.Q.Conns[w.cast.Q1.ID()].conn, w.cast.Q.Key, proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type: "host.join", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), To: w.cast.Q1.ID(),
		},
		Group: w.otherGID, Home: w.cast.P1.ID(), Agent: s2QPersonalCLI, Name: s2QPersonalName, Workspace: w.cast.Q1.Root,
	}, wantCode)
}

func (w *s2PeopleWorld) verifyQSecretaryJoinRefused(wantCode string) error {
	q1Conn := w.cast.Q.Conns[w.cast.Q1.ID()].conn
	return w.expectSignedCmdError(q1Conn, w.cast.Q.Key, proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type: "host.join", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), To: w.cast.Q1.ID(),
		},
		Group: w.gid, Home: w.cast.P1.ID(), Agent: s2QPersonalCLI, Name: s2QPersonalName, Secretary: true, Workspace: w.cast.Q1.Root,
	}, wantCode)
}

func (w *s2PeopleWorld) givenSurfaceCast(ctx context.Context, sharedName, soloName string) error {
	if err := w.initCastGroup(ctx, sharedName); err != nil {
		return err
	}
	solo, err := w.cast.CreateGroup(w.cast.P, w.cast.P1, soloName)
	if err != nil {
		return err
	}
	w.otherGID = solo
	if err := w.pCreatesInvitation(); err != nil {
		return err
	}
	if err := w.qBrowserJoinsGroup(w.cast.Q.Name); err != nil {
		return err
	}
	return w.q1JoinsQTeam()
}

func (w *s2PeopleWorld) verifyP1SurfaceForQ(wantCode string) error {
	if err := w.verifyQHomeSurfaceAllowed(); err != nil {
		return err
	}
	if err := w.expectSignedCmdError(w.qHomeBC.conn, w.cast.Q.Key, proto.HostCommand{
		Type: "pair.browser", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), To: w.cast.P1.ID(),
	}, wantCode); err != nil {
		return err
	}
	if err := expectRawRequestError(w.cast.Ctx, w.qHomeBC.conn, func(id int64) any {
		return proto.Request{Type: "host.info", ID: id}
	}, wantCode); err != nil {
		return err
	}
	return w.verifyOtherGroupRefusedOnP1(wantCode)
}

func (w *s2PeopleWorld) verifyQHomeSurfaceAllowed() error {
	raw, err := w.qHomeBC.request(w.cast.Ctx, func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: string(w.gid), Limit: 10},
		}
	})
	if err != nil {
		return err
	}
	var fetched proto.Fetched
	if err := proto.Decode(raw, &fetched); err != nil {
		return fmt.Errorf("fetch allowed group: %s (%w)", raw, err)
	}
	return postS2Mention(w.cast, w.cast.Q, w.cast.P1, w.gid, "", "Q on allowed home surface")
}

func (w *s2PeopleWorld) verifyOtherGroupRefusedOnP1(wantCode string) error {
	return expectRawRequestError(w.cast.Ctx, w.qHomeBC.conn, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.otherGID)}
	}, wantCode)
}

func expectRawRequestError(ctx context.Context, conn *peer.Connection, build func(int64) any, wantCode string) error {
	raw, err := requestWait(ctx, conn, nil, build)
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func (w *s2PeopleWorld) verifyQ1SurfaceForP() error {
	pOnQ1, err := w.invitePIntoQ1Group()
	if err != nil {
		return err
	}
	return w.verifyQ1RefusesHostCmdAndTakeoverFromP(pOnQ1.conn)
}

func (w *s2PeopleWorld) invitePIntoQ1Group() (*peerBrowserConn, error) {
	qGroup, err := w.cast.CreateGroup(w.cast.Q, w.cast.Q1, "Q-Shared")
	if err != nil {
		return nil, err
	}
	res, err := w.cast.SendCmd(w.cast.Q, w.cast.Q.Conns[w.cast.Q1.ID()].conn, s2InviteCommand(qGroup, w.cast.NextID(), time.Now().UnixMilli()))
	if err != nil {
		return nil, err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	if err != nil {
		return nil, err
	}
	bc, _, err := w.cast.JoinGroup(w.cast.P, link, w.cast.P.Name)
	return bc, err
}

func (w *s2PeopleWorld) verifyQ1RefusesHostCmdAndTakeoverFromP(conn *peer.Connection) error {
	err := w.expectSignedCmdError(conn, w.cast.P.Key, proto.HostCommand{
		Type: "pair.browser", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), To: w.cast.Q1.ID(),
	}, "forbidden")
	if err != nil {
		return err
	}
	return w.expectSignedCmdError(conn, w.cast.P.Key, proto.TakeoverStart{
		Takeover: proto.Takeover{
			HostCommand: proto.HostCommand{
				Type: "takeover.start", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), To: w.cast.Q1.ID(),
			},
			Group:  w.gid,
			Member: proto.ID(w.qTeam.ID),
		},
	}, "forbidden")
}
