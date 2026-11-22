//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

type s2InviteCmd struct {
	proto.HomeCommand
}

type s2JoinCmd struct {
	proto.HomeCommand
	Name string `json:"name"`
}

type s2RoleSetCmd struct {
	proto.MemberCommand
	Role string `json:"role"`
}

type s2HomeAddCmd struct {
	proto.HostCommand
	Group IDOrStr  `json:"g"`
	Home  string   `json:"home"`
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

type IDOrStr = proto.ID

type s2JoinLink struct {
	Raw     string
	Secret  identity.Secret
	Group   string
	Home    string
	Expires int64
}

type s2MemberJoinedBody struct {
	Code   string               `json:"code"`
	Member proto.HumanMemberRow `json:"member"`
	By     string               `json:"by"`
}

type s2MemberRemovedBody struct {
	Code string `json:"code"`
	User string `json:"user"`
	Name string `json:"name"`
	By   string `json:"by,omitempty"`
}

type s2BrowserView struct {
	groups     []proto.GroupSummary
	seenGroups bool
	group      proto.GroupRow
	humans     map[string]proto.HumanMemberRow
	ais        map[string]proto.AIMemberRow
	devices    map[string]proto.DeviceRow
	tasks      map[string]proto.TaskRow
	msgs       map[string]proto.MsgRow
	gone       map[[2]string]bool
}

func newS2BrowserView() *s2BrowserView {
	return &s2BrowserView{
		humans:  map[string]proto.HumanMemberRow{},
		ais:     map[string]proto.AIMemberRow{},
		devices: map[string]proto.DeviceRow{},
		tasks:   map[string]proto.TaskRow{},
		msgs:    map[string]proto.MsgRow{},
		gone:    map[[2]string]bool{},
	}
}

func (v *s2BrowserView) applyPush(raw json.RawMessage, wantGroup string) {
	var g proto.Groups
	if proto.Decode(raw, &g) == nil && g.Type == "groups" {
		v.groups = g.Groups
		v.seenGroups = true
		return
	}
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Group != wantGroup {
		return
	}
	for _, elem := range batch.Rows {
		v.applyRow(elem)
	}
}

func (v *s2BrowserView) applyRow(elem json.RawMessage) {
	var gone proto.GoneRow
	if proto.Decode(elem, &gone) == nil && gone.Gone {
		v.recordGone(gone.Type, gone.ID)
		return
	}
	var hdr proto.RowHeader
	if proto.Decode(elem, &hdr) != nil {
		return
	}
	v.recordLive(hdr.Type, elem)
}

func (v *s2BrowserView) recordGone(typ, id string) {
	v.gone[[2]string{typ, id}] = true
	delete(v.humans, id)
	delete(v.ais, id)
	delete(v.devices, id)
}

func (v *s2BrowserView) recordLive(typ string, elem json.RawMessage) {
	switch typ {
	case "group":
		decodeInto(elem, &v.group)
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

func decodeInto[T any](elem json.RawMessage, dst *T) {
	var val T
	if proto.Decode(elem, &val) == nil {
		*dst = val
	}
}

func (v *s2BrowserView) recordMember(elem json.RawMessage) {
	var hum proto.HumanMemberRow
	if proto.Decode(elem, &hum) == nil && hum.Kind == "human" {
		v.humans[hum.ID] = hum
		return
	}
	var ai proto.AIMemberRow
	if proto.Decode(elem, &ai) == nil && ai.Kind == "ai" {
		v.ais[ai.ID] = ai
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
		v.tasks[t.ID] = t
	}
}

func (v *s2BrowserView) recordMsg(elem json.RawMessage) {
	var m proto.MsgRow
	if proto.Decode(elem, &m) == nil {
		v.msgs[m.ID] = m
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

func (v *s2BrowserView) hasTextMsg(text string) bool {
	for _, m := range v.msgs {
		if m.Kind == "text" && m.Text == text {
			return true
		}
	}
	return false
}

type s2ClockHome struct {
	mu          sync.Mutex
	now         time.Time
	ctx         context.Context
	rvURL       string
	node        *peerNode
	ids         seqIDGen
	pKey, qKey  identity.Key
	pEntries    []proto.Envelope
	qEntries    []proto.Envelope
	pBrowser    *peerBrowser
	pConn       *peerBrowserConn
	qBrowser    *peerBrowser
	qConn       *peerBrowserConn
	pView       *s2BrowserView
	gid         proto.ID
	invites     map[string]s2JoinLink
	inviteTS    map[string]int64
	lastJoinRaw json.RawMessage
	lastCmdRaw  json.RawMessage
}

type s2PeopleWorld struct {
	f           fixtures
	cleanup     func(func())
	cast        *s2Cast
	gid         proto.ID
	otherGID    proto.ID
	preJoinText string
	inviteLink  s2JoinLink
	pView       *s2BrowserView
	qView       *s2BrowserView
	qHomeBC     *peerBrowserConn
	pMemID      string
	qMemID      string
	secretary   proto.AIMemberRow
	pWork       proto.AIMemberRow
	qTeam       proto.AIMemberRow
	qPersonal   proto.AIMemberRow
	taskID      proto.ID
	assigneeRef string
	reviewerRef string
	removeAt    time.Time
	clock       *s2ClockHome
}

func s2PeopleSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2PeopleWorld{f: f, cleanup: afterScenario(sc)}
	registerS2PeopleA91A92Steps(sc, w)
	registerS2PeopleA93A94Steps(sc, w)
	registerS2PeopleA95A96Steps(sc, w)
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
	sc.Step(`^using that invitation link a second time is refused$`, w.clockVerifySecondUseRefused)
	sc.Step(`^P creates a second invitation with nonce "([^"]*)" and 10 minutes pass$`, w.clockCreateAndExpire)
	sc.Step(`^using the expired invitation link is refused and repeating "([^"]*)" returns "([^"]*)"$`, w.clockVerifyExpiredRefused)
	sc.Step(`^Q as a member sends invite for the controllable group$`, w.clockQSendInvite)
	sc.Step(`^Q's invite is refused with "([^"]*)"$`, w.clockVerifyLastRefused)
}

func registerS2PeopleA93A94Steps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^the people cast where Q has joined P's group "([^"]*)" on P1 as "([^"]*)"$`, w.givenCastQJoined)
	sc.Step(`^P sets Q's role to "([^"]*)" in the people group$`, w.pSetsQRole)
	sc.Step(`^group set and invite by Q succeed in the people group$`, w.verifyQAdminPowers)
	sc.Step(`^member set with role by admin Q is refused with "([^"]*)"$`, w.verifyQRoleSetRefused)
	sc.Step(`^P sets Q's role back to "([^"]*)" in the people group$`, w.pSetsQRole)
	sc.Step(`^group set and invite by Q are refused with "([^"]*)"$`, w.verifyQMemberRefusals)
	sc.Step(`^setting a second owner, changing P's owner role and role changes by Q are refused$`, w.verifyOwnerRoleInvariants)

	sc.Step(`^the people cast where Q joined P's group "([^"]*)" on P1 and joined Q-team and Q-personal from Q1$`, w.givenCastWithQAIs)
	sc.Step(`^removing the owner P is refused with "([^"]*)"$`, w.verifyOwnerCannotBeRemoved)
	sc.Step(`^an unfinished task has Q-team as assignee and then as reviewer$`, w.stageUnfinishedTaskForQTeam)
	sc.Step(`^removing Q while Q-team holds either unfinished task role was refused with "([^"]*)"$`, w.verifyUnfinishedTaskConflicts)
	sc.Step(`^P cancels that task and removes Q from the people group$`, w.cancelTaskAndRemoveQ)
	sc.Step(`^Q's connections to P1 close within 1 second and Q's next connection attempt to P1 is refused$`, w.verifyQDisconnectedAndRefused)
	sc.Step(`^Q's browser received groups without the group and P sees gone markers for Q and Q's AIs$`, w.verifyQGroupsAndGoneMarkers)
}

func registerS2PeopleA95A96Steps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^the people cast where P's group "([^"]*)" on P1 has a secretary and Q has joined as "([^"]*)"$`, w.givenCastWithSecretaryAndQ)
	sc.Step(`^Q1 joins Q-team to the people group with host\.join$`, w.q1JoinsQTeam)
	sc.Step(`^Q-team's member row has host Q1 and user Q and Q1 holds a Git clone$`, w.verifyQTeamRowAndClone)
	sc.Step(`^member add_ai from a host whose owner is not in P's other group is refused with "([^"]*)"$`, w.verifyNonMemberHostAddAIRefused)
	sc.Step(`^host\.join from Q with secretary true while the group has a secretary is refused with "([^"]*)"$`, w.verifyQSecretaryJoinRefused)

	sc.Step(`^the people cast where Q joined "([^"]*)" on P1 with Q-team on Q1 and P has a second group "([^"]*)" on P1$`, w.givenSurfaceCast)
	sc.Step(`^on P1 Q's browser may use the home surface of "([^"]*)" while host commands, host\.info and sync of "([^"]*)" are refused with "([^"]*)"$`, w.verifyP1SurfaceForQ)
	sc.Step(`^Q1 drops connections from P before P shares a group on Q1 and refuses host commands and takeover\.start from P once connected$`, w.verifyQ1SurfaceForP)
}

func (w *s2PeopleWorld) initCastGroup(ctx context.Context, name string) error {
	cast, err := startS2Cast(w.f.t, w.cleanup, w.f, s2CastConfig{Rendezvous: sharedRVURL(ctx)})
	if err != nil {
		return err
	}
	w.cast = cast
	w.pView = newS2BrowserView()
	w.qView = newS2BrowserView()
	gid, err := cast.CreateProjectGroup(cast.P, cast.P1, name, "none")
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
	_, err := w.cast.SendCmd(w.cast.P, pBC.conn, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		Kind:     "text",
		Text:     historyText,
		Main:     true,
		Mentions: []proto.ID{},
		Body:     proto.EmptyBody{},
	})
	if err != nil {
		return err
	}
	return w.drainViewUntil(w.cast.Ctx, pBC, w.pView, string(w.gid), func() bool {
		return w.pView.hasTextMsg(historyText)
	})
}

func (w *s2PeopleWorld) pCreatesInvitation() error {
	nonce := w.cast.NextID()
	ts := time.Now().UnixMilli()
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	first, err := w.cast.SendCmd(w.cast.P, pBC.conn, s2InviteCmd{
		HomeCommand: proto.HomeCommand{Type: "invite", Nonce: nonce, Timestamp: ts, Group: w.gid},
	})
	if err != nil {
		return err
	}
	replay, err := w.cast.SendCmd(w.cast.P, pBC.conn, s2InviteCmd{
		HomeCommand: proto.HomeCommand{Type: "invite", Nonce: nonce, Timestamp: ts, Group: w.gid},
	})
	if err != nil {
		return err
	}
	if replay.Link != first.Link || replay.Expires != first.Expires {
		return fmt.Errorf("invite replay = %+v, want %+v", replay, first)
	}
	link, err := parseS2JoinLink(first.Link, first.Expires)
	w.inviteLink = link
	return err
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
	return s2JoinLink{Raw: raw, Secret: sec, Group: vals.Get("g"), Home: vals.Get("home"), Expires: expires}, nil
}

func parseJoinLinkFragment(raw string) (url.Values, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid invite URL %q: %v", raw, err)
	}
	vals, err := url.ParseQuery(u.Fragment)
	if err != nil || vals.Get("fp") != "" {
		return nil, fmt.Errorf("unexpected invite fragment %q (%v)", u.Fragment, err)
	}
	return vals, nil
}

func (w *s2PeopleWorld) verifyJoinLinkFields() error {
	now := time.Now().UnixMilli()
	if w.inviteLink.Group != string(w.gid) || w.inviteLink.Home != w.cast.P1.ID() {
		return fmt.Errorf("invite link = %+v, want g=%s home=%s", w.inviteLink, w.gid, w.cast.P1.ID())
	}
	if w.inviteLink.Expires <= now || w.inviteLink.Expires > now+11*time.Minute.Milliseconds() {
		return fmt.Errorf("invite expires = %d, now = %d", w.inviteLink.Expires, now)
	}
	return nil
}

func (w *s2PeopleWorld) qBrowserJoinsGroup(memberName string) error {
	bc, err := joinGroupWithLink(
		w.cast.Ctx, w.cast.addUndo, w.cast.Q.Browser, w.cast.Q.Key,
		w.cast.Q.Entries, w.inviteLink, memberName, w.cast.NextID(),
	)
	if err != nil {
		return err
	}
	w.qHomeBC = bc
	w.cast.Q.Conns[w.cast.P1.ID()] = bc
	if err := w.sendQHomeAddAndVerify(); err != nil {
		return err
	}
	return w.syncQAndAwaitViews()
}

func joinGroupWithLink(
	ctx context.Context, cleanup func(func()), b *peerBrowser, signer identity.Key,
	entries []proto.Envelope, link s2JoinLink, memberName string, joinNonce proto.ID,
) (*peerBrowserConn, error) {
	if _, err := b.exchangeRoom(ctx, link.Secret, "b", "a"); err != nil {
		return nil, err
	}
	bc, err := openJoiningCtl(ctx, cleanup, b, link.Home)
	if err != nil {
		return nil, err
	}
	if err := verifyJoiningWelcome(ctx, bc.conn, b.key.ID()); err != nil {
		return nil, err
	}
	if err := sendJoinDevlogAndCmd(ctx, bc.conn, signer, entries, link.Group, memberName, joinNonce); err != nil {
		return nil, err
	}
	return bc, nil
}

func openJoiningCtl(ctx context.Context, cleanup func(func()), b *peerBrowser, homeID string) (*peerBrowserConn, error) {
	for range 6 {
		bc, ok, err := b.tryOpenCtl(ctx, cleanup, homeID, 2*time.Second)
		if err != nil || ok {
			return bc, err
		}
		time.Sleep(40 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out opening joining ctl to %s", homeID)
}

func verifyJoiningWelcome(ctx context.Context, conn *peer.Connection, label string) error {
	welc, err := requestHelloV2Fallback(ctx, conn, label)
	if err != nil {
		return err
	}
	if welc.Owner || len(welc.Groups) != 0 {
		return fmt.Errorf("joining welcome = %+v, want owner=false and empty groups", welc)
	}
	return nil
}

func requestHelloV2Fallback(ctx context.Context, conn *peer.Connection, name string) (proto.Welcome, error) {
	raw, err := sendBrowserHelloVersion(ctx, conn, 2, name)
	if err != nil {
		return proto.Welcome{}, err
	}
	var er proto.ErrorResponse
	if proto.Decode(raw, &er) == nil && er.Err.Code == "version" {
		raw, err = sendBrowserHelloVersion(ctx, conn, 1, name)
		if err != nil {
			return proto.Welcome{}, err
		}
	}
	var welc proto.Welcome
	return welc, proto.Decode(raw, &welc)
}

func sendBrowserHelloVersion(ctx context.Context, conn *peer.Connection, ver int, name string) (json.RawMessage, error) {
	return conn.Request(ctx, func(id int64) any {
		return proto.Hello{
			Request: proto.Request{Type: "hello", ID: id},
			V:       ver,
			Kind:    "browser",
			Name:    name,
			Addrs:   []string{},
		}
	})
}

func sendJoinDevlogAndCmd(
	ctx context.Context, conn *peer.Connection, signer identity.Key,
	entries []proto.Envelope, gid, name string, nonce proto.ID,
) error {
	dl := proto.Devlog{Type: "devlog", User: signer.ID(), Entries: entries}
	if err := conn.Send(dl); err != nil {
		return err
	}
	raw, err := sendSignedCmdRaw(ctx, conn, signer, s2JoinCmd{
		HomeCommand: proto.HomeCommand{
			Type:      "join",
			Nonce:     nonce,
			Timestamp: time.Now().UnixMilli(),
			Group:     proto.ID(gid),
		},
		Name: name,
	})
	if err != nil {
		return err
	}
	var res proto.CmdResult
	return proto.Decode(raw, &res)
}

func sendSignedCmdRaw(ctx context.Context, conn *peer.Connection, signer identity.Key, payload any) (json.RawMessage, error) {
	env := signer.Sign(string(mustMarshalNoEscape(payload)))
	return conn.Request(ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
}

func (w *s2PeopleWorld) sendQHomeAddAndVerify() error {
	q1Conn := w.cast.Q.Conns[w.cast.Q1.ID()].conn
	groupName := w.pView.group.Name
	_, err := w.cast.SendCmd(w.cast.Q, q1Conn, s2HomeAddCmd{
		HostCommand: proto.HostCommand{
			Type:      "home.add",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.cast.Q1.ID(),
		},
		Group: w.gid,
		Home:  w.cast.P1.ID(),
		Name:  groupName,
		Addrs: []string{w.cast.RVURL},
	})
	if err != nil {
		return err
	}
	info, err := requestHostInfo(w.cast.Ctx, q1Conn)
	if err != nil {
		return err
	}
	return assertHomeListed(info.Homes, w.gid, w.cast.P1.ID(), groupName)
}

func assertHomeListed(homes []proto.Home, wantGID proto.ID, wantHome, wantName string) error {
	want := proto.Home{Group: wantGID, Home: wantHome, Name: wantName}
	if !slices.Contains(homes, want) {
		return fmt.Errorf("host.info.homes = %+v, want %+v", homes, want)
	}
	return nil
}

func (w *s2PeopleWorld) syncQAndAwaitViews() error {
	if err := w.drainViewUntil(w.cast.Ctx, w.qHomeBC, w.qView, string(w.gid), func() bool {
		return slices.ContainsFunc(w.qView.groups, func(g proto.GroupSummary) bool { return g.Group == string(w.gid) })
	}); err != nil {
		return err
	}
	if err := w.cast.syncGroup(w.qHomeBC.conn, w.gid); err != nil {
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
	return nil
}

func (w *s2PeopleWorld) verifyBothBrowsersHistory() error {
	for name, view := range map[string]*s2BrowserView{"P": w.pView, "Q": w.qView} {
		if err := w.verifyOneBrowserHistory(name, view); err != nil {
			return err
		}
	}
	return nil
}

func (w *s2PeopleWorld) verifyOneBrowserHistory(label string, view *s2BrowserView) error {
	qRow, ok := view.humanByUser(w.cast.Q.ID())
	if !ok || qRow.Role != "member" {
		return fmt.Errorf("%s view missing Q member row: %+v", label, qRow)
	}
	if !view.hasTextMsg(w.preJoinText) {
		return fmt.Errorf("%s view missing pre-join message %q", label, w.preJoinText)
	}
	if !hasMemberJoinedSystemMsg(view, w.qMemID, w.pMemID) {
		return fmt.Errorf("%s view missing member_joined system row for %s by %s", label, w.qMemID, w.pMemID)
	}
	return nil
}

func hasMemberJoinedSystemMsg(view *s2BrowserView, wantMemberID, wantBy string) bool {
	for _, m := range view.msgs {
		if matchMemberJoinedMsg(m, wantMemberID, wantBy) {
			return true
		}
	}
	return false
}

func matchMemberJoinedMsg(m proto.MsgRow, wantMemberID, wantBy string) bool {
	if m.Kind != "system" || m.By != "system" {
		return false
	}
	var body s2MemberJoinedBody
	if proto.Decode(m.Body, &body) != nil {
		return false
	}
	return body.Code == "member_joined" && body.Member.ID == wantMemberID && body.By == wantBy
}

// --- A9.2 controllable-clock home ---

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
	runCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	w.cleanup(cancel)
	rvURL, err := startAcceptanceRelayRVWithBack(w.cleanup, sharedRVURL(ctx))
	if err != nil {
		return err
	}
	ch := &s2ClockHome{
		now:      time.UnixMilli(1763700000000),
		ctx:      runCtx,
		rvURL:    rvURL,
		pKey:     newRandomPeerKey(),
		qKey:     newRandomPeerKey(),
		pView:    newS2BrowserView(),
		invites:  map[string]s2JoinLink{},
		inviteTS: map[string]int64{},
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
	ch.pEntries = buildPeerOwnerLog(ch.pKey, nodeKey, "P1-clock", nil, ts)
	ch.qEntries = []proto.Envelope{nodetest.SignDeviceAdd(ch.qKey, ch.qKey.ID(), 1, "browser", "Q-browser", ch.qKey.ID(), ts)}
	return w.connectControllableOwnerAndGroup(groupName)
}

func (w *s2PeopleWorld) connectControllableOwnerAndGroup(groupName string) error {
	ch := w.clock
	pb, err := newPeerBrowser(ch.ctx, w.cleanup, ch.pKey, ch.rvURL, nil)
	if err != nil {
		return err
	}
	ch.pBrowser = pb
	if err := pb.waitOnline(ch.ctx, ch.node.Key.ID()); err != nil {
		return err
	}
	bc, err := pb.openCtl(ch.ctx, w.cleanup, ch.node.Key.ID())
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
		Name: groupName, Kind: "project", Repo: "https://git.example/alpha.git",
		Base: "main", Forge: "none", Checks: []string{}, MemberName: "P",
	})
	if err != nil {
		return err
	}
	var ok proto.CmdResult
	if err := proto.Decode(raw, &ok); err != nil {
		return fmt.Errorf("create group: %s (%w)", raw, err)
	}
	return syncClockConn(ch.ctx, ch.pConn, ch.pView, gid)
}

func syncClockConn(ctx context.Context, bc *peerBrowserConn, view *s2BrowserView, gid proto.ID) error {
	raw, err := bc.conn.Request(ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(gid)}
	})
	if err != nil {
		return err
	}
	var synced proto.Synced
	if err := proto.Decode(raw, &synced); err != nil {
		return err
	}
	drainBufferedView(bc, view, string(gid))
	return nil
}

func (w *s2PeopleWorld) clockCreateInvite(tag string) error {
	ch := w.clock
	ts := ch.nowTime().UnixMilli()
	ch.inviteTS[tag] = ts
	raw, err := sendSignedCmdRaw(ch.ctx, ch.pConn.conn, ch.pKey, s2InviteCmd{
		HomeCommand: proto.HomeCommand{Type: "invite", Nonce: exampleID(tag), Timestamp: ts, Group: ch.gid},
	})
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("invite %s: %s (%w)", tag, raw, err)
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	ch.invites[tag] = link
	return err
}

func (w *s2PeopleWorld) clockBadLogJoin() error {
	ch := w.clock
	link := ch.invites["inv-1"]
	badUser, badDev, rogue := newRandomPeerKey(), newRandomPeerKey(), newRandomPeerKey()
	b, err := newPeerBrowser(ch.ctx, w.cleanup, badDev, ch.rvURL, nil)
	if err != nil {
		return err
	}
	if _, err := b.exchangeRoom(ch.ctx, link.Secret, "b", "a"); err != nil {
		return err
	}
	bc, err := openJoiningCtl(ch.ctx, w.cleanup, b, link.Home)
	if err != nil {
		return err
	}
	if err := verifyJoiningWelcome(ch.ctx, bc.conn, "bad-joiner"); err != nil {
		return err
	}
	return w.sendBrokenLogAndJoin(bc.conn, badUser, badDev, rogue)
}

func (w *s2PeopleWorld) sendBrokenLogAndJoin(conn *peer.Connection, badUser, badDev, rogue identity.Key) error {
	ch := w.clock
	ts := ch.nowTime().UnixMilli()
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(badUser, badUser.ID(), 1, "browser", "gen", badUser.ID(), ts),
		nodetest.SignDeviceAdd(rogue, badUser.ID(), 2, "browser", "unauth", badDev.ID(), ts),
	}
	if err := conn.Send(proto.Devlog{Type: "devlog", User: badUser.ID(), Entries: entries}); err != nil {
		return err
	}
	raw, err := sendSignedCmdRaw(ch.ctx, conn, badDev, s2JoinCmd{
		HomeCommand: proto.HomeCommand{Type: "join", Nonce: ch.ids.next(), Timestamp: ts, Group: ch.gid},
		Name:        "Bad",
	})
	ch.lastJoinRaw = raw
	return err
}

func (w *s2PeopleWorld) clockVerifyBadJoinRefused(wantCode string) error {
	ch := w.clock
	if err := checkWireError(ch.lastJoinRaw, wantCode); err != nil {
		return err
	}
	drainBufferedView(ch.pConn, ch.pView, string(ch.gid))
	if len(ch.pView.humans) != 1 {
		return fmt.Errorf("humans after bad join = %+v, want only P", ch.pView.humans)
	}
	return nil
}

func (w *s2PeopleWorld) clockQJoinsFirstLink(memberName string) error {
	ch := w.clock
	link := ch.invites["inv-1"]
	qb, err := newPeerBrowser(ch.ctx, w.cleanup, ch.qKey, ch.rvURL, nil)
	if err != nil {
		return err
	}
	ch.qBrowser = qb
	if _, err := qb.exchangeRoom(ch.ctx, link.Secret, "b", "a"); err != nil {
		return err
	}
	bc, err := openJoiningCtl(ch.ctx, w.cleanup, qb, link.Home)
	if err != nil {
		return err
	}
	ch.qConn = bc
	if err := verifyJoiningWelcome(ch.ctx, bc.conn, "Q-browser"); err != nil {
		return err
	}
	return w.sendClockJoin(bc.conn, ch.qKey, ch.qEntries, memberName)
}

func (w *s2PeopleWorld) sendClockJoin(conn *peer.Connection, key identity.Key, entries []proto.Envelope, name string) error {
	ch := w.clock
	if err := conn.Send(proto.Devlog{Type: "devlog", User: key.ID(), Entries: entries}); err != nil {
		return err
	}
	raw, err := sendSignedCmdRaw(ch.ctx, conn, key, s2JoinCmd{
		HomeCommand: proto.HomeCommand{Type: "join", Nonce: ch.ids.next(), Timestamp: ch.nowTime().UnixMilli(), Group: ch.gid},
		Name:        name,
	})
	if err != nil {
		return err
	}
	var res proto.CmdResult
	return proto.Decode(raw, &res)
}

func (w *s2PeopleWorld) clockVerifySecondUseRefused() error {
	ch := w.clock
	if err := w.verifyClockInviteReplay("inv-1", "conflict"); err != nil {
		return err
	}
	return w.verifyClosedJoinLinkRefused(ch.invites["inv-1"], "SecondUse")
}

func (w *s2PeopleWorld) verifyClockInviteReplay(tag, wantCode string) error {
	ch := w.clock
	raw, err := sendSignedCmdRaw(ch.ctx, ch.pConn.conn, ch.pKey, s2InviteCmd{
		HomeCommand: proto.HomeCommand{Type: "invite", Nonce: exampleID(tag), Timestamp: ch.inviteTS[tag], Group: ch.gid},
	})
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func (w *s2PeopleWorld) verifyClosedJoinLinkRefused(link s2JoinLink, name string) error {
	ch := w.clock
	rKey := newRandomPeerKey()
	rb, err := newPeerBrowser(ch.ctx, w.cleanup, rKey, ch.rvURL, nil)
	if err != nil {
		return err
	}
	tryCtx, cancel := context.WithTimeout(ch.ctx, 700*time.Millisecond)
	defer cancel()
	if _, err := rb.exchangeRoom(tryCtx, link.Secret, "b", "a"); err != nil {
		return nil
	}
	bc, ok, err := rb.tryOpenCtl(tryCtx, w.cleanup, link.Home, 700*time.Millisecond)
	if err != nil || !ok {
		return nil
	}
	return w.expectJoinRefusedOnConn(bc.conn, rKey, name)
}

func (w *s2PeopleWorld) expectJoinRefusedOnConn(conn *peer.Connection, rKey identity.Key, name string) error {
	ch := w.clock
	ts := ch.nowTime().UnixMilli()
	entries := []proto.Envelope{nodetest.SignDeviceAdd(rKey, rKey.ID(), 1, "browser", name, rKey.ID(), ts)}
	_ = conn.Send(proto.Devlog{Type: "devlog", User: rKey.ID(), Entries: entries})
	raw, err := sendSignedCmdRaw(ch.ctx, conn, rKey, s2JoinCmd{
		HomeCommand: proto.HomeCommand{Type: "join", Nonce: ch.ids.next(), Timestamp: ts, Group: ch.gid},
		Name:        name,
	})
	if err != nil {
		return nil
	}
	return checkWireError(raw, "forbidden")
}

func (w *s2PeopleWorld) clockCreateAndExpire(tag string) error {
	if err := w.clockCreateInvite(tag); err != nil {
		return err
	}
	w.clock.advance(10*time.Minute + time.Second)
	return nil
}

func (w *s2PeopleWorld) clockVerifyExpiredRefused(tag, wantCode string) error {
	if err := w.verifyClosedJoinLinkRefused(w.clock.invites[tag], "ExpiredJoiner"); err != nil {
		return err
	}
	return w.verifyClockInviteReplay(tag, wantCode)
}

func (w *s2PeopleWorld) clockQSendInvite() error {
	ch := w.clock
	raw, err := sendSignedCmdRaw(ch.ctx, ch.qConn.conn, ch.qKey, s2InviteCmd{
		HomeCommand: proto.HomeCommand{Type: "invite", Nonce: ch.ids.next(), Timestamp: ch.nowTime().UnixMilli(), Group: ch.gid},
	})
	ch.lastCmdRaw = raw
	return err
}

func (w *s2PeopleWorld) clockVerifyLastRefused(wantCode string) error {
	return checkWireError(w.clock.lastCmdRaw, wantCode)
}

// --- A9.3 roles ---

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
	_, err := w.cast.SendCmd(w.cast.P, pBC.conn, s2RoleSetCmd{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{
				Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
			},
			Op: "set",
			ID: proto.ID(w.qMemID),
		},
		Role: role,
	})
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
	res, err := w.cast.SendCmd(w.cast.Q, w.qHomeBC.conn, s2InviteCmd{
		HomeCommand: proto.HomeCommand{
			Type: "invite", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
	})
	if err != nil || res.Link == "" || res.Expires == 0 {
		return fmt.Errorf("admin Q invite = %+v, %v", res, err)
	}
	return nil
}

func (w *s2PeopleWorld) verifyQRoleSetRefused(wantCode string) error {
	return w.expectSignedCmdError(w.qHomeBC.conn, w.cast.Q.Key, s2RoleSetCmd{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{
				Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
			},
			Op: "set",
			ID: proto.ID(w.qMemID),
		},
		Role: "member",
	}, wantCode)
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
	return w.expectSignedCmdError(w.qHomeBC.conn, w.cast.Q.Key, s2InviteCmd{
		HomeCommand: proto.HomeCommand{
			Type: "invite", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
	}, wantCode)
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
	return w.expectSignedCmdError(conn, signer, s2RoleSetCmd{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{
				Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
			},
			Op: "set",
			ID: proto.ID(targetID),
		},
		Role: role,
	}, wantCode)
}

func (w *s2PeopleWorld) expectSignedCmdError(conn *peer.Connection, signer identity.Key, payload any, wantCode string) error {
	raw, err := sendSignedCmdRaw(w.cast.Ctx, conn, signer, payload)
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

// --- A9.4 removal ---

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
	sec, err := w.cast.JoinAI(w.cast.P, w.cast.P1, w.cast.P1, w.gid, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	w.secretary = sec
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
	return w.expectSignedCmdError(conn, signer, proto.MemberCommand{
		HomeCommand: proto.HomeCommand{
			Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
		Op: "remove",
		ID: proto.ID(memID),
	}, wantCode)
}

func (w *s2PeopleWorld) stageUnfinishedTaskForQTeam() error {
	if err := w.startDirectTaskByPWork(); err != nil {
		return err
	}
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	if err := w.assignTaskRoles(proto.ID(w.qTeam.ID), proto.ID(w.pWork.ID)); err != nil {
		return err
	}
	w.assigneeRef = w.captureRemoveCode(pConn, w.cast.P.Key, w.qMemID)
	if err := w.assignTaskRoles(proto.ID(w.pWork.ID), proto.ID(w.qTeam.ID)); err != nil {
		return err
	}
	w.reviewerRef = w.captureRemoveCode(pConn, w.cast.P.Key, w.qMemID)
	return nil
}

func (w *s2PeopleWorld) startDirectTaskByPWork() error {
	tid := w.cast.NextID()
	w.taskID = tid
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	raw, err := sendSignedCmdRaw(w.cast.Ctx, pConn, w.cast.P2.Key, proto.TaskStart{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(),
				Group: w.gid, As: proto.ID(w.pWork.ID),
			},
			Op: "start",
			ID: tid,
		},
		Title: "Hold Q-team busy", Plan: "keep open", Origin: "direct",
	})
	if err != nil {
		return err
	}
	var res proto.CmdResult
	return proto.Decode(raw, &res)
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

func (w *s2PeopleWorld) captureRemoveCode(conn *peer.Connection, signer identity.Key, memID string) string {
	raw, err := sendSignedCmdRaw(w.cast.Ctx, conn, signer, proto.MemberCommand{
		HomeCommand: proto.HomeCommand{
			Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
		Op: "remove",
		ID: proto.ID(memID),
	})
	if err != nil {
		return err.Error()
	}
	var er proto.ErrorResponse
	if proto.Decode(raw, &er) != nil {
		return string(raw)
	}
	return er.Err.Code
}

func (w *s2PeopleWorld) verifyUnfinishedTaskConflicts(wantCode string) error {
	if w.assigneeRef != wantCode || w.reviewerRef != wantCode {
		return fmt.Errorf("remove codes = (assignee=%q, reviewer=%q), want %q", w.assigneeRef, w.reviewerRef, wantCode)
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
	w.removeAt = time.Now()
	_, err = w.cast.SendCmd(w.cast.P, pBC.conn, proto.MemberCommand{
		HomeCommand: proto.HomeCommand{
			Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
		Op: "remove",
		ID: proto.ID(w.qMemID),
	})
	return err
}

func (w *s2PeopleWorld) verifyQDisconnectedAndRefused() error {
	deadline := w.removeAt.Add(time.Second)
	if err := waitBrowserClosedBefore(w.qHomeBC.conn, deadline); err != nil {
		return err
	}
	if err := w.waitP1DroppedQDevices(deadline); err != nil {
		return err
	}
	_, ok, err := w.cast.Q.Browser.tryOpenCtl(w.cast.Ctx, w.cast.addUndo, w.cast.P1.ID(), 1200*time.Millisecond)
	if err != nil || ok {
		return fmt.Errorf("expected next ctl attempt from removed Q to P1 to fail, got ok=%v err=%v", ok, err)
	}
	return nil
}

func waitBrowserClosedBefore(conn *peer.Connection, deadline time.Time) error {
	rem := time.Until(deadline)
	if rem <= 0 {
		rem = time.Millisecond
	}
	select {
	case <-conn.Done():
		return nil
	case <-time.After(rem):
		return errors.New("Q browser ctl to P1 did not close within 1 second")
	}
}

func (w *s2PeopleWorld) waitP1DroppedQDevices(deadline time.Time) error {
	qBrowserID, q1ID := w.cast.Q.ID(), w.cast.Q1.ID()
	for {
		out, err := w.runNodeStatus(w.cast.P1)
		if err == nil && !strings.Contains(out, qBrowserID) && !strings.Contains(out, q1ID) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("P1 still lists Q connection past 1s: %s (%v)", out, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (w *s2PeopleWorld) runNodeStatus(n *s2Node) (string, error) {
	cmd := exec.Command(w.cast.F.bin, "status")
	cmd.Env = n.Env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (w *s2PeopleWorld) verifyQGroupsAndGoneMarkers() error {
	drainClosedBrowserPushes(w.qHomeBC, w.qView, string(w.gid))
	hasGroup := slices.ContainsFunc(w.qView.groups, func(g proto.GroupSummary) bool { return g.Group == string(w.gid) })
	if !w.qView.seenGroups || hasGroup {
		return fmt.Errorf("Q browser groups after removal = %+v (seen=%v), want group absent", w.qView.groups, w.qView.seenGroups)
	}
	return w.awaitPGoneMarkers()
}

func drainClosedBrowserPushes(bc *peerBrowserConn, view *s2BrowserView, gid string) {
	for raw := range bc.pushes {
		view.applyPush(raw, gid)
	}
}

func (w *s2PeopleWorld) awaitPGoneMarkers() error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	return w.drainViewUntil(w.cast.Ctx, pBC, w.pView, string(w.gid), func() bool {
		return w.hasAllQGoneMarkers() && hasMemberRemovedSystemMsg(w.pView, w.cast.Q.ID(), w.pMemID)
	})
}

func (w *s2PeopleWorld) hasAllQGoneMarkers() bool {
	for _, id := range []string{w.qMemID, w.qTeam.ID, w.qPersonal.ID} {
		if !w.pView.gone[[2]string{"member", id}] {
			return false
		}
	}
	return w.pView.gone[[2]string{"device", w.cast.Q.ID() + ":1"}] && w.pView.gone[[2]string{"device", w.cast.Q.ID() + ":2"}]
}

func hasMemberRemovedSystemMsg(view *s2BrowserView, wantUser, wantBy string) bool {
	for _, m := range view.msgs {
		if matchMemberRemovedMsg(m, wantUser, wantBy) {
			return true
		}
	}
	return false
}

func matchMemberRemovedMsg(m proto.MsgRow, wantUser, wantBy string) bool {
	if m.Kind != "system" || m.By != "system" {
		return false
	}
	var body s2MemberRemovedBody
	if proto.Decode(m.Body, &body) != nil {
		return false
	}
	return body.Code == "member_removed" && body.User == wantUser && body.By == wantBy
}

// --- A9.5 other people's hosts ---

func (w *s2PeopleWorld) givenCastWithSecretaryAndQ(ctx context.Context, groupName, qName string) error {
	if err := w.initCastGroup(ctx, groupName); err != nil {
		return err
	}
	sec, err := w.cast.JoinAI(w.cast.P, w.cast.P1, w.cast.P1, w.gid, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	w.secretary = sec
	other, err := w.cast.CreateProjectGroup(w.cast.P, w.cast.P1, "Private", "none")
	if err != nil {
		return err
	}
	w.otherGID = other
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
	repoReadme := filepath.Join(w.cast.Q1.Data, "projects", string(w.gid), "repo", "README.md")
	if _, err := os.Stat(repoReadme); err != nil {
		return fmt.Errorf("expected Q1 clone at %s: %w", repoReadme, err)
	}
	return nil
}

func (w *s2PeopleWorld) verifyNonMemberHostAddAIRefused(wantCode string) error {
	q2Key, q2BC, err := w.enrolExtraQNodeAndConnectP1()
	if err != nil {
		return err
	}
	return w.expectSignedCmdError(q2BC.conn, q2Key, proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{
				Type: "member", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.otherGID,
			},
			Op: "add_ai",
			ID: w.cast.NextID(),
		},
		CLI: "claude", Name: "Intruder", Account: "team", Secretary: false, Approvers: []string{w.cast.Q.ID()},
	}, wantCode)
}

func (w *s2PeopleWorld) enrolExtraQNodeAndConnectP1() (identity.Key, *peerBrowserConn, error) {
	q2Key := newRandomPeerKey()
	ver := len(w.cast.Q.Entries) + 1
	entry := nodetest.SignDeviceAdd(w.cast.Q.Key, w.cast.Q.ID(), ver, "node", "Q2", q2Key.ID(), time.Now().UnixMilli())
	w.cast.Q.Entries = append(w.cast.Q.Entries, entry)
	if err := w.qHomeBC.conn.Send(proto.Devlog{Type: "devlog", User: w.cast.Q.ID(), Entries: []proto.Envelope{entry}}); err != nil {
		return nil, nil, err
	}
	q2Peer, err := newPeerBrowser(w.cast.Ctx, w.cast.addUndo, q2Key, w.cast.RVURL, nil)
	if err != nil {
		return nil, nil, err
	}
	bc, err := openEnrolledHostCtl(w.cast, q2Peer, w.cast.P1.ID())
	return q2Key, bc, err
}

func openEnrolledHostCtl(cast *s2Cast, b *peerBrowser, homeID string) (*peerBrowserConn, error) {
	var last error
	for range 6 {
		bc, ok, err := b.tryOpenCtl(cast.Ctx, cast.addUndo, homeID, 2*time.Second)
		if err == nil && ok {
			if _, herr := requestHelloV2Fallback(cast.Ctx, bc.conn, "Q2"); herr == nil {
				return bc, nil
			} else {
				last = herr
			}
			bc.conn.Close()
		} else {
			last = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("openEnrolledHostCtl: %w", last)
}

func (w *s2PeopleWorld) verifyQSecretaryJoinRefused(wantCode string) error {
	q1Conn := w.cast.Q.Conns[w.cast.Q1.ID()].conn
	return w.expectSignedCmdError(q1Conn, w.cast.Q.Key, proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type: "host.join", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), To: w.cast.Q1.ID(),
		},
		Group: w.gid, Home: w.cast.P1.ID(), Agent: s2QPersonalCLI, Name: s2QPersonalName, Secretary: true,
	}, wantCode)
}

// --- A9.6 surface boundaries ---

func (w *s2PeopleWorld) givenSurfaceCast(ctx context.Context, sharedName, soloName string) error {
	if err := w.initCastGroup(ctx, sharedName); err != nil {
		return err
	}
	solo, err := w.cast.CreateProjectGroup(w.cast.P, w.cast.P1, soloName, "none")
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

func (w *s2PeopleWorld) verifyP1SurfaceForQ(_, _, wantCode string) error {
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
	raw, err := w.qHomeBC.conn.Request(w.cast.Ctx, func(id int64) any {
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
	_, err = w.cast.SendCmd(w.cast.Q, w.qHomeBC.conn, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid,
		},
		Kind: "text", Text: "Q on allowed home surface", Main: true, Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	})
	return err
}

func (w *s2PeopleWorld) verifyOtherGroupRefusedOnP1(wantCode string) error {
	err := expectRawRequestError(w.cast.Ctx, w.qHomeBC.conn, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.otherGID)}
	}, wantCode)
	if err != nil {
		return err
	}
	return expectRawRequestError(w.cast.Ctx, w.qHomeBC.conn, func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: string(w.otherGID), Limit: 10},
		}
	}, wantCode)
}

func expectRawRequestError(ctx context.Context, conn *peer.Connection, build func(int64) any, wantCode string) error {
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func (w *s2PeopleWorld) verifyQ1SurfaceForP() error {
	_, ok, err := w.cast.P.Browser.tryOpenCtl(w.cast.Ctx, w.cast.addUndo, w.cast.Q1.ID(), 1200*time.Millisecond)
	if err != nil || ok {
		return fmt.Errorf("expected Q1 to drop P before sharing a group on Q1, got ok=%v err=%v", ok, err)
	}
	pOnQ1, err := w.invitePIntoQ1Group()
	if err != nil {
		return err
	}
	return w.verifyQ1RefusesHostCmdAndTakeoverFromP(pOnQ1.conn)
}

func (w *s2PeopleWorld) invitePIntoQ1Group() (*peerBrowserConn, error) {
	qGroup, err := w.cast.CreateProjectGroup(w.cast.Q, w.cast.Q1, "Q-Shared", "none")
	if err != nil {
		return nil, err
	}
	res, err := w.cast.SendCmd(w.cast.Q, w.cast.Q.Conns[w.cast.Q1.ID()].conn, s2InviteCmd{
		HomeCommand: proto.HomeCommand{
			Type: "invite", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: qGroup,
		},
	})
	if err != nil {
		return nil, err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	if err != nil {
		return nil, err
	}
	return joinGroupWithLink(
		w.cast.Ctx, w.cast.addUndo, w.cast.P.Browser, w.cast.P.Key,
		w.cast.P.Entries, link, w.cast.P.Name, w.cast.NextID(),
	)
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
