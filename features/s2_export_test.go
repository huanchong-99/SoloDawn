//go:build acceptance

package features

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["s2_export"] = s2ExportSteps
}

type s2ExpInviteCmd struct {
	proto.HomeCommand
}

type s2ExpJoinCmd struct {
	proto.HomeCommand
	Name string `json:"name"`
}

type s2ExpHomeAddCmd struct {
	proto.HostCommand
	Group proto.ID `json:"g"`
	Home  string   `json:"home"`
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

type s2ExpMovedWireErr struct {
	Code    string `json:"code"`
	Message string `json:"msg"`
	Home    string `json:"home"`
}

type s2ExpMovedResp struct {
	Re  int64             `json:"re"`
	Err s2ExpMovedWireErr `json:"err"`
}

type s2ExpLink struct {
	Secret identity.Secret
	Group  proto.ID
	Home   string
}

type s2ExpRowSnap map[[2]string]json.RawMessage

type s2ExpView struct {
	home   string
	cursor int64
	groups []proto.GroupSummary
	humans map[string]proto.HumanMemberRow
	ais    map[string]proto.AIMemberRow
	tasks  map[string]proto.TaskRow
	msgs   map[string]proto.MsgRow
	notes  map[string]proto.NoteRow
	gone   map[[2]string]bool
}

func newS2ExpView(home string) *s2ExpView {
	return &s2ExpView{
		home:   home,
		humans: map[string]proto.HumanMemberRow{},
		ais:    map[string]proto.AIMemberRow{},
		tasks:  map[string]proto.TaskRow{},
		msgs:   map[string]proto.MsgRow{},
		notes:  map[string]proto.NoteRow{},
		gone:   map[[2]string]bool{},
	}
}

func (v *s2ExpView) applyFrame(raw json.RawMessage, gid string) {
	var gs proto.Groups
	if proto.Decode(raw, &gs) == nil && gs.Type == "groups" {
		v.groups = gs.Groups
		return
	}
	var rst proto.Reset
	if proto.Decode(raw, &rst) == nil && rst.Type == "reset" && rst.Group == gid {
		v.resetCache()
		return
	}
	v.applyRowsFrame(raw, gid)
}

func (v *s2ExpView) resetCache() {
	v.cursor = 0
	clear(v.humans)
	clear(v.ais)
	clear(v.tasks)
	clear(v.msgs)
	clear(v.notes)
	clear(v.gone)
}

func (v *s2ExpView) applyRowsFrame(raw json.RawMessage, gid string) {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Group != gid {
		return
	}
	for _, elem := range batch.Rows {
		v.applyElem(elem)
	}
}

func (v *s2ExpView) applyElem(elem json.RawMessage) {
	var gone proto.GoneRow
	if proto.Decode(elem, &gone) == nil && gone.Gone {
		v.gone[[2]string{gone.Type, gone.ID}] = true
		delete(v.notes, gone.ID)
		return
	}
	var hdr proto.RowHeader
	if proto.Decode(elem, &hdr) != nil {
		return
	}
	v.cursor = max(v.cursor, hdr.Seq)
	v.storeTypedRow(hdr.Type, elem)
}

func (v *s2ExpView) storeTypedRow(typ string, elem json.RawMessage) {
	switch typ {
	case "member":
		v.storeMember(elem)
	case "task":
		s2ExpDecodeMap(elem, v.tasks, func(t proto.TaskRow) string { return t.ID })
	case "msg":
		s2ExpDecodeMap(elem, v.msgs, func(m proto.MsgRow) string { return m.ID })
	case "note":
		s2ExpDecodeMap(elem, v.notes, func(n proto.NoteRow) string { return n.ID })
	}
}

func (v *s2ExpView) storeMember(elem json.RawMessage) {
	var h proto.HumanMemberRow
	if proto.Decode(elem, &h) == nil && h.Kind == "human" {
		v.humans[h.ID] = h
		return
	}
	s2ExpDecodeMap(elem, v.ais, func(a proto.AIMemberRow) string { return a.ID })
}

func s2ExpDecodeMap[T any](elem json.RawMessage, dst map[string]T, keyFn func(T) string) {
	var val T
	if proto.Decode(elem, &val) == nil {
		dst[keyFn(val)] = val
	}
}

func (v *s2ExpView) hasMsgText(text string) bool {
	for _, m := range v.msgs {
		if m.Text == text {
			return true
		}
	}
	return false
}

func (v *s2ExpView) hasNoteText(text string) bool {
	for _, n := range v.notes {
		if n.Text == text {
			return true
		}
	}
	return false
}

type s2ExpWorld struct {
	f             fixtures
	cleanup       func(func())
	cast          *s2Cast
	gid           proto.ID
	otherGID      proto.ID
	groupName     string
	pView         *s2ExpView
	qView         *s2ExpView
	secretary     proto.AIMemberRow
	pWork         proto.AIMemberRow
	qTeam         proto.AIMemberRow
	task1ID       proto.ID
	keptNoteID    proto.ID
	goneNoteID    proto.ID
	exportPath    string
	lastCopyPath  string
	preMoveRows   s2ExpRowSnap
	lastCopyRows  s2ExpRowSnap
	beforeJoinMem map[string]proto.HumanMemberRow
	importDoneAt  time.Time
	joinOK        bool
}

func s2ExportSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2ExpWorld{f: f, cleanup: afterScenario(sc)}
	registerS2ExportMoveSteps(sc, w)
	registerS2ExportCopySteps(sc, w)
}

func registerS2ExportMoveSteps(sc *godog.ScenarioContext, w *s2ExpWorld) {
	sc.Step(`^the export cast where P's group "([^"]+)" on P1 has Q, AIs on P1, P2 and Q1, task 1, a gone note and an unacknowledged item for Q-team$`, w.givenMoveGroup)
	sc.Step(`^P runs solodawn group export for "([^"]+)" with --to P2 on P1$`, w.whenExportMoveToP2)
	sc.Step(`^P1 omits "([^"]+)" from welcome and answers cmd, sync and fetch for "([^"]+)" with moved naming P2$`, w.thenP1AnswersMoved)
	sc.Step(`^P runs solodawn group import of the exported file on P2$`, w.whenImportOnP2)
	sc.Step(`^the rows of "([^"]+)" on P2 equal the rows P1 held including gone markers$`, w.thenP2RowsEqualP1)
	sc.Step(`^P's browser, Q's browser and Q1 follow "([^"]+)" to P2 within 5 seconds of the import without a reload$`, w.thenClientsFollowWithin5s)
	sc.Step(`^the unacknowledged item from P1 is delivered once to Q-team by P2 and the next task on P2 gets number 2$`, w.thenCarryOverAndNextTaskNum)
	sc.Step(`^importing the exported file on Q1 fails and exporting "([^"]+)" on a node that is not its home fails$`, w.thenInvalidImportAndExportFail)
}

func registerS2ExportCopySteps(sc *godog.ScenarioContext, w *s2ExpWorld) {
	sc.Step(`^the export cast where P's group "([^"]+)" on P1 has Q on Q's browser and Q-team on Q1$`, w.givenCopyGroup)
	sc.Step(`^periodic copy exports of "([^"]+)" run on P1 and P writes rows after the last copy before P1's data directory is deleted$`, w.whenPeriodicCopiesAndDeleteP1)
	sc.Step(`^P runs solodawn group import of the last copy of "([^"]+)" on P2$`, w.whenImportLastCopyOnP2)
	sc.Step(`^P2 is the home of "([^"]+)" with the rows of that last copy and none written later$`, w.thenP2HoldsLastCopyOnly)
	sc.Step(`^P creates an invitation for "([^"]+)" on P2 and Q's browser opens it and sends join as "([^"]+)"$`, w.whenPInvitesOnP2AndQJoins)
	sc.Step(`^join on P2 answers ok with no new member row for Q$`, w.thenJoinOKWithoutNewMember)
	sc.Step(`^Q's browser and Q1 follow P2 for "([^"]+)"$`, w.thenQAndQ1FollowRestoredP2)
}

func (w *s2ExpWorld) bootCastAndGroup(name string, scriptSecretary bool) error {
	cast, err := startS2Cast(w.f.t, w.cleanup, w.f, s2CastConfig{Timeout: 90 * time.Second})
	if err != nil {
		return err
	}
	w.cast, w.groupName = cast, name
	w.pView = newS2ExpView(cast.P1.ID())
	w.qView = newS2ExpView(cast.P1.ID())
	if scriptSecretary {
		if err := cast.WriteScript(cast.P1, s2SecretaryCLI, s2ExpSecretaryScript()); err != nil {
			return err
		}
	}
	return w.createAndJoinMembers(name)
}

func s2ExpSecretaryScript() string {
	t1 := s2ExpBriefSpec("Initial task before move", s2QTeamName, s2PWorkName)
	t2 := s2ExpBriefSpec("Second task after move", s2PWorkName, s2QTeamName)
	base := m5ClaudeScript([]m5ToolCallSpec{t1}, nil, []m5ToolCallSpec{t2}, nil)
	return strings.TrimSuffix(base, "]}") + `,{"dir":"eof"}]}`
}

func s2ExpBriefSpec(title, assignee, reviewer string) m5ToolCallSpec {
	return m5ToolCallSpec{
		Name: "propose_brief",
		Args: map[string]any{
			"title":     title,
			"goal":      title,
			"criteria":  []string{"Task completes"},
			"scope_in":  []string{"features"},
			"scope_out": []string{"none"},
			"requester": s2PName,
			"tasks":     []map[string]any{{"title": title, "assignee": assignee, "reviewer": reviewer}},
		},
	}
}

func (w *s2ExpWorld) createAndJoinMembers(name string) error {
	gid, err := w.cast.CreateProjectGroup(w.cast.P, w.cast.P1, name, "none")
	if err != nil {
		return err
	}
	w.gid = gid
	other, err := w.cast.CreateProjectGroup(w.cast.P, w.cast.P1, name+"-Other", "none")
	if err != nil {
		return err
	}
	w.otherGID = other
	if err := w.joinPAIsOnP1(); err != nil {
		return err
	}
	return w.inviteAndJoinQOnP1()
}

func (w *s2ExpWorld) joinPAIsOnP1() error {
	var err error
	w.secretary, err = w.cast.JoinAI(w.cast.P, w.cast.P1, w.cast.P1, w.gid, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	w.pWork, err = w.cast.JoinAI(w.cast.P, w.cast.P2, w.cast.P1, w.gid, s2PWorkCLI, s2PWorkName, false)
	return err
}

func (w *s2ExpWorld) inviteAndJoinQOnP1() error {
	link, err := w.sendInvite(w.cast.P, w.cast.P1, w.gid)
	if err != nil {
		return err
	}
	bc, _, err := s2ExpOpenJoinLink(w.cast, w.cast.Q, link, s2QName)
	if err != nil {
		return err
	}
	w.cast.Q.Conns[w.cast.P1.ID()] = bc
	if err := w.sendHomeAdd(w.cast.Q, w.cast.Q1, w.cast.P1.ID()); err != nil {
		return err
	}
	if err := w.syncBrowserView(w.cast.Q.Conns[w.cast.P1.ID()], w.qView); err != nil {
		return err
	}
	return w.joinAndAllowQTeamOnP1()
}

func (w *s2ExpWorld) joinAndAllowQTeamOnP1() error {
	var err error
	w.qTeam, err = w.cast.JoinAI(w.cast.Q, w.cast.Q1, w.cast.P1, w.gid, s2QTeamCLI, s2QTeamName, false)
	if err != nil {
		return err
	}
	allow := []string{w.cast.P.ID()}
	_, err = w.cast.SendCmd(w.cast.Q, w.cast.Q.Conns[w.cast.P1.ID()].conn, proto.MemberSet{
		MemberCommand: proto.MemberCommand{
			HomeCommand: w.homeHdr(w.gid),
			Op:          "set",
			ID:          proto.ID(w.qTeam.ID),
		},
		Allow: &allow,
	})
	return err
}

func (w *s2ExpWorld) homeHdr(gid proto.ID) proto.HomeCommand {
	return proto.HomeCommand{
		Type:      "member",
		Nonce:     w.cast.NextID(),
		Timestamp: time.Now().UnixMilli(),
		Group:     gid,
	}
}

func (w *s2ExpWorld) givenMoveGroup(name string) error {
	if err := w.bootCastAndGroup(name, true); err != nil {
		return err
	}
	taskRow, err := w.proposeAndConfirmTask(w.cast.P1, "Plan initial task", 1)
	if err != nil {
		return err
	}
	w.task1ID = proto.ID(taskRow.ID)
	if err := w.waitTaskLocalCheckAndIdle(w.cast.P1, taskRow.ID, w.qTeam.ID); err != nil {
		return err
	}
	if err := w.seedNotesAndPauseTask(); err != nil {
		return err
	}
	return w.queueUnackedItemAndSnapshotP1()
}

func (w *s2ExpWorld) proposeAndConfirmTask(homeNode *s2Node, prompt string, wantNum int) (proto.TaskRow, error) {
	pBC := w.cast.P.Conns[homeNode.ID()]
	if err := postS2Mention(w.cast, w.cast.P, homeNode, w.gid, w.secretary.ID, prompt); err != nil {
		return proto.TaskRow{}, err
	}
	briefID, err := w.awaitPendingBrief(pBC)
	if err != nil {
		return proto.TaskRow{}, err
	}
	if err := w.confirmBrief(pBC.conn, briefID); err != nil {
		return proto.TaskRow{}, err
	}
	return w.awaitTaskNum(pBC, wantNum)
}

func (w *s2ExpWorld) awaitPendingBrief(bc *peerBrowserConn) (proto.ID, error) {
	var found proto.ID
	err := w.drainViewUntil(bc, w.pView, func() bool {
		for _, m := range w.pView.msgs {
			if m.Kind == "brief" && m.State != nil && m.State.Status == "pending" {
				found = proto.ID(m.ID)
				return true
			}
		}
		return false
	})
	return found, err
}

func (w *s2ExpWorld) confirmBrief(conn *peer.Connection, briefID proto.ID) error {
	_, err := w.cast.SendCmd(w.cast.P, conn, proto.Act{
		HomeCommand: proto.HomeCommand{
			Type:      "act",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		Msg:    briefID,
		Action: "confirm",
	})
	return err
}

func (w *s2ExpWorld) awaitTaskNum(bc *peerBrowserConn, wantNum int) (proto.TaskRow, error) {
	var found proto.TaskRow
	err := w.drainViewUntil(bc, w.pView, func() bool {
		for _, t := range w.pView.tasks {
			if t.Num == wantNum {
				found = t
				return true
			}
		}
		return false
	})
	return found, err
}

func (w *s2ExpWorld) waitTaskLocalCheckAndIdle(homeNode *s2Node, taskID, aiID string) error {
	bc := w.cast.P.Conns[homeNode.ID()]
	return w.drainViewUntil(bc, w.pView, func() bool {
		t, okT := w.pView.tasks[taskID]
		ai, okAI := w.pView.ais[aiID]
		return okT && okAI && t.Step == "local_check" && ai.State == "waiting"
	})
}

func (w *s2ExpWorld) seedNotesAndPauseTask() error {
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	w.keptNoteID = w.cast.NextID()
	w.goneNoteID = w.cast.NextID()
	if err := w.writeNote(pConn, w.keptNoteID, "kept note before move"); err != nil {
		return err
	}
	if err := w.writeNote(pConn, w.goneNoteID, "forgotten note before move"); err != nil {
		return err
	}
	if err := w.forgetNote(pConn, w.goneNoteID); err != nil {
		return err
	}
	return w.sendTaskOp(pConn, "pause", w.task1ID)
}

func (w *s2ExpWorld) writeNote(conn *peer.Connection, noteID proto.ID, text string) error {
	_, err := w.cast.SendCmd(w.cast.P, conn, proto.Note{
		HomeCommand: proto.HomeCommand{
			Type:      "note",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		ID:   noteID,
		Text: text,
	})
	return err
}

func (w *s2ExpWorld) forgetNote(conn *peer.Connection, noteID proto.ID) error {
	_, err := w.cast.SendCmd(w.cast.P, conn, proto.Forget{
		HomeCommand: proto.HomeCommand{
			Type:      "forget",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		Row: "note",
		ID:  noteID,
	})
	return err
}

func (w *s2ExpWorld) sendTaskOp(conn *peer.Connection, op string, taskID proto.ID) error {
	_, err := w.cast.SendCmd(w.cast.P, conn, proto.TaskCommand{
		HomeCommand: proto.HomeCommand{
			Type:      "task",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		Op: op,
		ID: taskID,
	})
	return err
}

func (w *s2ExpWorld) queueUnackedItemAndSnapshotP1() error {
	pBC := w.cast.P.Conns[w.cast.P1.ID()]
	if err := w.drainViewUntil(pBC, w.pView, func() bool {
		return w.pView.tasks[string(w.task1ID)].State == "paused"
	}); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	if err := w.postThreadMention(w.cast.Q, w.cast.P1.ID(), w.task1ID, w.qTeam.ID, "@Q-team carry-over-from-P1"); err != nil {
		return err
	}
	if err := w.syncBothViewsOnP1(); err != nil {
		return err
	}
	snap, err := w.snapshotGroupRows(w.cast.P.Conns[w.cast.P1.ID()], w.pView)
	w.preMoveRows = snap
	return err
}

func (w *s2ExpWorld) syncBothViewsOnP1() error {
	if err := w.syncBrowserView(w.cast.P.Conns[w.cast.P1.ID()], w.pView); err != nil {
		return err
	}
	return w.syncBrowserView(w.cast.Q.Conns[w.cast.P1.ID()], w.qView)
}

func (w *s2ExpWorld) postThreadMention(p *s2Person, homeID string, threadID proto.ID, aiID, text string) error {
	_, err := w.cast.SendCmd(p, p.Conns[homeID].conn, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		Kind:     "text",
		Thread:   &threadID,
		Main:     false,
		Text:     text,
		Mentions: []proto.ID{proto.ID(aiID)},
		Body:     proto.EmptyBody{},
	})
	return err
}

func (w *s2ExpWorld) whenExportMoveToP2(_ string) error {
	if n, err := w.countCLITurns("carry-over-from-P1"); err != nil || n != 0 {
		return fmt.Errorf("turns for carry-over before export = %d, %v; want 0", n, err)
	}
	w.exportPath = filepath.Join(w.cast.Root, "atlas-move.db")
	return w.runLocalCmdExpectOK(w.cast.P1, "group", "export", string(w.gid), w.exportPath, "--to", w.cast.P2.ID())
}

func (w *s2ExpWorld) thenP1AnswersMoved(_, _ string) error {
	pConn := w.cast.P.Conns[w.cast.P1.ID()].conn
	if err := w.verifyWelcomeOmitsGroup(pConn); err != nil {
		return err
	}
	if err := w.verifyCmdMoved(pConn); err != nil {
		return err
	}
	if err := w.verifySyncMoved(pConn); err != nil {
		return err
	}
	return w.verifyFetchMoved(pConn)
}

func (w *s2ExpWorld) verifyWelcomeOmitsGroup(conn *peer.Connection) error {
	welc, err := requestBrowserHello(w.cast.Ctx, conn)
	if err != nil {
		return err
	}
	has := slices.ContainsFunc(welc.Groups, func(g proto.GroupSummary) bool { return g.Group == string(w.gid) })
	if has {
		return fmt.Errorf("P1 welcome still lists moved group %s: %+v", w.gid, welc.Groups)
	}
	return nil
}

func (w *s2ExpWorld) verifyCmdMoved(conn *peer.Connection) error {
	env := w.cast.P.Key.Sign(string(mustMarshalNoEscape(proto.Note{
		HomeCommand: proto.HomeCommand{Type: "note", Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid},
		ID:          w.cast.NextID(),
		Text:        "should fail moved",
	})))
	raw, err := conn.Request(w.cast.Ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	return w.assertMovedErr("cmd", raw)
}

func (w *s2ExpWorld) verifySyncMoved(conn *peer.Connection) error {
	raw, err := conn.Request(w.cast.Ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.gid), Since: 0}
	})
	if err != nil {
		return err
	}
	return w.assertMovedErr("sync", raw)
}

func (w *s2ExpWorld) verifyFetchMoved(conn *peer.Connection) error {
	raw, err := conn.Request(w.cast.Ctx, func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: string(w.gid), Before: 0, Limit: 10},
		}
	})
	if err != nil {
		return err
	}
	return w.assertMovedErr("fetch", raw)
}

func (w *s2ExpWorld) assertMovedErr(op string, raw json.RawMessage) error {
	var resp s2ExpMovedResp
	if err := proto.Decode(raw, &resp); err != nil {
		return fmt.Errorf("%s moved decode %s: %w", op, raw, err)
	}
	if resp.Err.Code != "moved" || resp.Err.Home != w.cast.P2.ID() {
		return fmt.Errorf("%s answered %+v, want moved to %s", op, resp, w.cast.P2.ID())
	}
	return nil
}

func (w *s2ExpWorld) whenImportOnP2() error {
	if err := w.runLocalCmdExpectOK(w.cast.P2, "group", "import", w.exportPath); err != nil {
		return err
	}
	w.importDoneAt = time.Now()
	return nil
}

func (w *s2ExpWorld) thenP2RowsEqualP1(_ string) error {
	if err := w.verifyExportSQLiteFile(w.exportPath, 1, 1); err != nil {
		return err
	}
	got, err := w.snapshotGroupRows(w.cast.P.Conns[w.cast.P2.ID()], w.pView)
	if err != nil {
		return err
	}
	if err := s2ExpRequireGoneRow(got, "note", string(w.goneNoteID)); err != nil {
		return err
	}
	return s2ExpCompareSnaps(w.preMoveRows, got)
}

func s2ExpRequireGoneRow(snap s2ExpRowSnap, typ, id string) error {
	raw, ok := snap[[2]string{typ, id}]
	if !ok {
		return fmt.Errorf("missing gone %s marker %s on P2", typ, id)
	}
	var g proto.GoneRow
	if err := proto.Decode(raw, &g); err != nil || !g.Gone {
		return fmt.Errorf("row (%s, %s) = %s (%v), want gone=true", typ, id, raw, err)
	}
	return nil
}

func (w *s2ExpWorld) verifyExportSQLiteFile(srcPath string, wantInbox, wantNum int) error {
	db, cleanupCopy, err := s2ExpOpenSQLiteCopy(w.cast.Root, srcPath)
	if err != nil {
		return err
	}
	defer cleanupCopy()
	if err := w.checkExportDBCounts(db, wantInbox, wantNum); err != nil {
		return err
	}
	return w.checkExportDBLogs(db)
}

func s2ExpOpenSQLiteCopy(dir, srcPath string) (*sql.DB, func(), error) {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, nil, err
	}
	tmp, err := os.CreateTemp(dir, "exp-check-*.db")
	if err != nil {
		return nil, nil, err
	}
	name := tmp.Name()
	_ = tmp.Close()
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("sqlite", name)
	return db, func() { _ = db.Close(); _ = os.Remove(name) }, err
}

func (w *s2ExpWorld) checkExportDBCounts(db *sql.DB, wantInbox, wantNum int) error {
	var otherRows, inboxCount, grpNum int
	err := errors.Join(
		db.QueryRow(`SELECT COUNT(*) FROM row WHERE g != ?`, string(w.gid)).Scan(&otherRows),
		db.QueryRow(`SELECT COUNT(*) FROM inbox WHERE g = ?`, string(w.gid)).Scan(&inboxCount),
		db.QueryRow(`SELECT num FROM grp WHERE g = ?`, string(w.gid)).Scan(&grpNum),
	)
	if err != nil {
		return err
	}
	want := [3]int{0, wantInbox, wantNum}
	got := [3]int{otherRows, inboxCount, grpNum}
	if got != want {
		return fmt.Errorf("export file [otherRows inbox num] = %v, want %v", got, want)
	}
	return nil
}

func (w *s2ExpWorld) checkExportDBLogs(db *sql.DB) error {
	for _, userID := range []string{w.cast.P.ID(), w.cast.Q.ID()} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM device_log WHERE user = ?`, userID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("export file missing device_log for user %s", userID)
		}
	}
	return nil
}

func (w *s2ExpWorld) thenClientsFollowWithin5s(_ string) error {
	if err := w.followBrowserMoved(w.cast.P, w.pView); err != nil {
		return err
	}
	if err := w.followBrowserMoved(w.cast.Q, w.qView); err != nil {
		return err
	}
	if err := w.waitNodeFollowsHome(w.cast.Q, w.cast.Q1, w.cast.P2.ID(), 5*time.Second); err != nil {
		return err
	}
	if err := w.verifyLiveRoundTripOnP2("@Q-team live-after-move"); err != nil {
		return err
	}
	if elapsed := time.Since(w.importDoneAt); elapsed > 5*time.Second {
		return fmt.Errorf("clients followed P2 in %s, want <= 5s", elapsed)
	}
	return nil
}

func (w *s2ExpWorld) followBrowserMoved(p *s2Person, view *s2ExpView) error {
	raw, err := w.requestSyncOnHome(p, view.home, view.cursor)
	if err != nil {
		return err
	}
	var moved s2ExpMovedResp
	if err := proto.Decode(raw, &moved); err != nil || moved.Err.Code != "moved" {
		return fmt.Errorf("%s sync on old home answered %s (%v), want moved", p.Name, raw, err)
	}
	return w.switchBrowserToHome(p, view, moved.Err.Home)
}

func (w *s2ExpWorld) requestSyncOnHome(p *s2Person, homeID string, since int64) (json.RawMessage, error) {
	sendSync := func(bc *peerBrowserConn) (json.RawMessage, error) {
		return bc.conn.Request(w.cast.Ctx, func(id int64) any {
			return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.gid), Since: since}
		})
	}
	raw, err := sendSync(p.Conns[homeID])
	if err == nil {
		return raw, nil
	}
	bc, dialErr := w.dialMemberCtl(p.Browser, homeID, p.Name)
	if dialErr != nil {
		return nil, err
	}
	p.Conns[homeID] = bc
	return sendSync(bc)
}

func (w *s2ExpWorld) switchBrowserToHome(p *s2Person, view *s2ExpView, newHome string) error {
	bc, ok := p.Conns[newHome]
	if !ok {
		var err error
		bc, err = w.dialMemberCtl(p.Browser, newHome, p.Name)
		if err != nil {
			return err
		}
		p.Conns[newHome] = bc
	}
	view.home = newHome
	return w.syncBrowserView(bc, view)
}

func (w *s2ExpWorld) dialMemberCtl(b *peerBrowser, homeID, label string) (*peerBrowserConn, error) {
	bc, err := b.openCtl(w.cast.Ctx, w.cast.addUndo, homeID)
	if err != nil {
		return nil, err
	}
	if _, err := s2ExpHelloWithFallback(w.cast.Ctx, bc.conn, label); err != nil {
		return nil, err
	}
	return bc, nil
}

func (w *s2ExpWorld) waitNodeFollowsHome(owner *s2Person, hostNode *s2Node, wantHome string, limit time.Duration) error {
	deadline := w.importDoneAt
	if deadline.IsZero() {
		deadline = time.Now()
	}
	deadline = deadline.Add(limit)
	for time.Now().Before(deadline) {
		if ok, err := w.nodeListsGroupHome(owner, hostNode, wantHome); err != nil || ok {
			return err
		}
		time.Sleep(40 * time.Millisecond)
	}
	return fmt.Errorf("%s did not switch group %s home to %s within %s", hostNode.Name, w.gid, wantHome, limit)
}

func (w *s2ExpWorld) nodeListsGroupHome(owner *s2Person, hostNode *s2Node, wantHome string) (bool, error) {
	info, err := requestHostInfo(w.cast.Ctx, owner.Conns[hostNode.ID()].conn)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(info.Homes, func(h proto.Home) bool {
		return h.Group == w.gid && h.Home == wantHome
	}), nil
}

func (w *s2ExpWorld) verifyLiveRoundTripOnP2(text string) error {
	if err := postS2Mention(w.cast, w.cast.Q, w.cast.P2, w.gid, w.qTeam.ID, text); err != nil {
		return err
	}
	if err := w.drainViewUntil(w.cast.P.Conns[w.cast.P2.ID()], w.pView, func() bool {
		return w.pView.hasMsgText(text)
	}); err != nil {
		return err
	}
	if err := w.drainViewUntil(w.cast.Q.Conns[w.cast.P2.ID()], w.qView, func() bool {
		return w.qView.hasMsgText(text)
	}); err != nil {
		return err
	}
	return w.waitCLITurnCount(text, 1)
}

func (w *s2ExpWorld) thenCarryOverAndNextTaskNum() error {
	p2Conn := w.cast.P.Conns[w.cast.P2.ID()].conn
	if err := w.sendTaskOp(p2Conn, "resume", w.task1ID); err != nil {
		return err
	}
	if err := w.waitCLITurnCount("carry-over-from-P1", 1); err != nil {
		return err
	}
	if err := w.waitP2InboxEmptyAndCheckSingleTurn(); err != nil {
		return err
	}
	return w.verifySecondTaskGetsNumberTwo()
}

func (w *s2ExpWorld) waitP2InboxEmptyAndCheckSingleTurn() error {
	checkPath := filepath.Join(w.cast.Root, "p2-post-resume.db")
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if lastErr = w.runLocalCmdExpectOK(w.cast.P2, "group", "export", string(w.gid), checkPath); lastErr == nil {
			if lastErr = w.verifyExportSQLiteFile(checkPath, 0, 1); lastErr == nil {
				return w.assertExactTurnCount("carry-over-from-P1", 1)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return lastErr
}

func (w *s2ExpWorld) assertExactTurnCount(sub string, want int) error {
	time.Sleep(80 * time.Millisecond)
	got, err := w.countCLITurns(sub)
	if err != nil || got != want {
		return fmt.Errorf("CLI turns containing %q = %d (%v), want %d", sub, got, err, want)
	}
	return nil
}

func (w *s2ExpWorld) verifySecondTaskGetsNumberTwo() error {
	if err := w.waitNodeFollowsHome(w.cast.P, w.cast.P1, w.cast.P2.ID(), 5*time.Second); err != nil {
		return err
	}
	task2, err := w.proposeAndConfirmTask(w.cast.P2, "Plan second task", 2)
	if err != nil {
		return err
	}
	if task2.Num != 2 {
		return fmt.Errorf("second task num = %d, want 2", task2.Num)
	}
	return nil
}

func (w *s2ExpWorld) thenInvalidImportAndExportFail(_ string) error {
	if err := w.runLocalCmdExpectFail(w.cast.Q1, "group", "import", w.exportPath); err != nil {
		return err
	}
	if err := w.runLocalCmdExpectFail(w.cast.P2, "group", "import", w.exportPath); err != nil {
		return err
	}
	badQ1 := filepath.Join(w.cast.Root, "bad-q1.db")
	if err := w.runLocalCmdExpectFail(w.cast.Q1, "group", "export", string(w.gid), badQ1); err != nil {
		return err
	}
	badP1 := filepath.Join(w.cast.Root, "bad-p1.db")
	return w.runLocalCmdExpectFail(w.cast.P1, "group", "export", string(w.gid), badP1)
}

func (w *s2ExpWorld) givenCopyGroup(name string) error {
	return w.bootCastAndGroup(name, false)
}

func (w *s2ExpWorld) whenPeriodicCopiesAndDeleteP1(_ string) error {
	if err := w.postPlainText(w.cast.P, w.cast.P1.ID(), "copy-1 message"); err != nil {
		return err
	}
	copy1 := filepath.Join(w.cast.Root, "beacon-copy-1.db")
	if err := w.runLocalCmdExpectOK(w.cast.P1, "group", "export", string(w.gid), copy1); err != nil {
		return err
	}
	if err := w.writeSecondCopyRowsAndExport(); err != nil {
		return err
	}
	if err := w.writePostCopyRows(); err != nil {
		return err
	}
	return w.stopAndDeleteP1Data()
}

func (w *s2ExpWorld) writeSecondCopyRowsAndExport() error {
	if err := w.postPlainText(w.cast.P, w.cast.P1.ID(), "copy-2 message"); err != nil {
		return err
	}
	if err := w.writeNote(w.cast.P.Conns[w.cast.P1.ID()].conn, w.cast.NextID(), "copy-2 note"); err != nil {
		return err
	}
	w.lastCopyPath = filepath.Join(w.cast.Root, "beacon-copy-2.db")
	if err := w.runLocalCmdExpectOK(w.cast.P1, "group", "export", string(w.gid), w.lastCopyPath); err != nil {
		return err
	}
	snap, err := w.snapshotGroupRows(w.cast.P.Conns[w.cast.P1.ID()], w.pView)
	w.lastCopyRows = snap
	return err
}

func (w *s2ExpWorld) writePostCopyRows() error {
	if err := w.postPlainText(w.cast.P, w.cast.P1.ID(), "lost-after-last-copy"); err != nil {
		return err
	}
	return w.writeNote(w.cast.P.Conns[w.cast.P1.ID()].conn, w.cast.NextID(), "lost-note-after-last-copy")
}

func (w *s2ExpWorld) stopAndDeleteP1Data() error {
	w.cast.P1.Stop()
	if err := os.RemoveAll(w.cast.P1.Data); err != nil {
		return err
	}
	if _, err := os.Stat(w.cast.P1.Data); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("expected P1 data directory removed, stat err = %v", err)
	}
	return nil
}

func (w *s2ExpWorld) postPlainText(p *s2Person, homeID, text string) error {
	_, err := w.cast.SendCmd(p, p.Conns[homeID].conn, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.gid,
		},
		Kind:     "text",
		Main:     true,
		Text:     text,
		Mentions: []proto.ID{proto.ID(w.secretary.ID)},
		Body:     proto.EmptyBody{},
	})
	if err != nil {
		return err
	}
	return w.waitAISettledOnHome(p.Conns[homeID], w.secretary.ID)
}

func (w *s2ExpWorld) waitAISettledOnHome(bc *peerBrowserConn, aiID string) error {
	var sawWorking bool
	return w.drainViewUntil(bc, w.pView, func() bool {
		ai, ok := w.pView.ais[aiID]
		if ok && ai.State == "working" {
			sawWorking = true
		}
		return sawWorking && ok && ai.State == "waiting"
	})
}

func (w *s2ExpWorld) whenImportLastCopyOnP2(_ string) error {
	if err := w.runLocalCmdExpectOK(w.cast.P2, "group", "import", w.lastCopyPath); err != nil {
		return err
	}
	w.pView = newS2ExpView(w.cast.P2.ID())
	return w.syncBrowserView(w.cast.P.Conns[w.cast.P2.ID()], w.pView)
}

func (w *s2ExpWorld) thenP2HoldsLastCopyOnly(_ string) error {
	got, err := w.snapshotGroupRows(w.cast.P.Conns[w.cast.P2.ID()], w.pView)
	if err != nil {
		return err
	}
	if err := s2ExpCompareSnaps(w.lastCopyRows, got); err != nil {
		return err
	}
	if err := w.fetchNotesIntoView(w.cast.P.Conns[w.cast.P2.ID()].conn, w.pView); err != nil {
		return err
	}
	return w.assertCopyTextsOnView(w.pView)
}

func (w *s2ExpWorld) assertCopyTextsOnView(v *s2ExpView) error {
	present := [3]bool{v.hasMsgText("copy-1 message"), v.hasMsgText("copy-2 message"), v.hasNoteText("copy-2 note")}
	if present != [3]bool{true, true, true} {
		return fmt.Errorf("restored view missing expected copy rows: %v", present)
	}
	absent := [2]bool{v.hasMsgText("lost-after-last-copy"), v.hasNoteText("lost-note-after-last-copy")}
	if absent != [2]bool{false, false} {
		return errors.New("restored view contains rows written after the last copy")
	}
	return nil
}

func (w *s2ExpWorld) fetchNotesIntoView(conn *peer.Connection, v *s2ExpView) error {
	raw, err := conn.Request(w.cast.Ctx, func(id int64) any {
		return proto.FetchKind{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: string(w.gid), Before: 0, Limit: 50},
			Kind:      "note",
		}
	})
	if err != nil {
		return err
	}
	var fetched proto.Fetched
	if err := proto.Decode(raw, &fetched); err != nil {
		return err
	}
	for _, elem := range fetched.Rows {
		v.applyElem(elem)
	}
	return nil
}

func (w *s2ExpWorld) whenPInvitesOnP2AndQJoins(_, memberName string) error {
	w.beforeJoinMem = mapsCloneHumans(w.pView.humans)
	link, err := w.sendInvite(w.cast.P, w.cast.P2, w.gid)
	if err != nil {
		return err
	}
	if link.Home != w.cast.P2.ID() {
		return fmt.Errorf("P2 invite link home = %s, want %s", link.Home, w.cast.P2.ID())
	}
	bc, res, err := s2ExpOpenJoinLink(w.cast, w.cast.Q, link, memberName)
	if err != nil {
		return err
	}
	w.cast.Q.Conns[w.cast.P2.ID()] = bc
	w.joinOK = bool(res.OK)
	return nil
}

func mapsCloneHumans(src map[string]proto.HumanMemberRow) map[string]proto.HumanMemberRow {
	out := make(map[string]proto.HumanMemberRow, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (w *s2ExpWorld) thenJoinOKWithoutNewMember() error {
	if !w.joinOK {
		return errors.New("join on restored P2 did not answer ok")
	}
	if err := w.syncBrowserView(w.cast.P.Conns[w.cast.P2.ID()], w.pView); err != nil {
		return err
	}
	if !reflect.DeepEqual(w.beforeJoinMem, w.pView.humans) {
		return fmt.Errorf("human members changed on restored join: before=%+v after=%+v", w.beforeJoinMem, w.pView.humans)
	}
	return nil
}

func (w *s2ExpWorld) thenQAndQ1FollowRestoredP2(_ string) error {
	w.qView = newS2ExpView(w.cast.P2.ID())
	qBC := w.cast.Q.Conns[w.cast.P2.ID()]
	if err := w.syncBrowserView(qBC, w.qView); err != nil {
		return err
	}
	if err := w.fetchNotesIntoView(qBC.conn, w.qView); err != nil {
		return err
	}
	if err := w.assertCopyTextsOnView(w.qView); err != nil {
		return err
	}
	if err := w.sendHomeAdd(w.cast.Q, w.cast.Q1, w.cast.P2.ID()); err != nil {
		return err
	}
	if err := w.waitNodeFollowsHome(w.cast.Q, w.cast.Q1, w.cast.P2.ID(), 5*time.Second); err != nil {
		return err
	}
	return w.verifyLiveRoundTripOnP2("@Q-team restored-on-P2")
}

func (w *s2ExpWorld) sendInvite(admin *s2Person, homeNode *s2Node, gid proto.ID) (s2ExpLink, error) {
	res, err := w.cast.SendCmd(admin, admin.Conns[homeNode.ID()].conn, s2ExpInviteCmd{
		HomeCommand: proto.HomeCommand{
			Type:      "invite",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     gid,
		},
	})
	if err != nil {
		return s2ExpLink{}, err
	}
	return s2ExpParseLink(res.Link)
}

func s2ExpParseLink(raw string) (s2ExpLink, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return s2ExpLink{}, err
	}
	vals, err := url.ParseQuery(u.Fragment)
	if err != nil {
		return s2ExpLink{}, err
	}
	sec, err := identity.ParseSecret(vals.Get("join"))
	if err != nil {
		return s2ExpLink{}, fmt.Errorf("bad secret in %q: %w", raw, err)
	}
	return s2ExpLink{Secret: sec, Group: proto.ID(vals.Get("g")), Home: vals.Get("home")}, nil
}

func s2ExpOpenJoinLink(cast *s2Cast, p *s2Person, link s2ExpLink, memberName string) (*peerBrowserConn, proto.CmdResult, error) {
	if _, err := p.Browser.exchangeRoom(cast.Ctx, link.Secret, "b", "a"); err != nil {
		return nil, proto.CmdResult{}, err
	}
	bc, err := p.Browser.openCtl(cast.Ctx, cast.addUndo, link.Home)
	if err != nil {
		return nil, proto.CmdResult{}, err
	}
	if _, err := s2ExpHelloWithFallback(cast.Ctx, bc.conn, p.Name); err != nil {
		return nil, proto.CmdResult{}, err
	}
	if err := bc.conn.Send(proto.Devlog{Type: "devlog", User: p.ID(), Entries: p.Entries}); err != nil {
		return nil, proto.CmdResult{}, err
	}
	res, err := cast.SendCmd(p, bc.conn, s2ExpJoinCmd{
		HomeCommand: proto.HomeCommand{Type: "join", Nonce: cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: link.Group},
		Name:        memberName,
	})
	return bc, res, err
}

func s2ExpHelloWithFallback(ctx context.Context, conn *peer.Connection, label string) (proto.Welcome, error) {
	raw, err := s2ExpSendHello(ctx, conn, 2, label)
	if err != nil {
		return proto.Welcome{}, err
	}
	var er proto.ErrorResponse
	if proto.Decode(raw, &er) == nil && er.Err.Code == "version" {
		raw, err = s2ExpSendHello(ctx, conn, 1, label)
		if err != nil {
			return proto.Welcome{}, err
		}
	}
	var welc proto.Welcome
	return welc, proto.Decode(raw, &welc)
}

func s2ExpSendHello(ctx context.Context, conn *peer.Connection, v int, label string) (json.RawMessage, error) {
	return conn.Request(ctx, func(id int64) any {
		return proto.Hello{
			Request: proto.Request{Type: "hello", ID: id},
			V:       v,
			Kind:    "browser",
			Name:    label + "-browser",
			Addrs:   []string{},
		}
	})
}

func (w *s2ExpWorld) sendHomeAdd(p *s2Person, hostNode *s2Node, homeID string) error {
	_, err := w.cast.SendCmd(p, p.Conns[hostNode.ID()].conn, s2ExpHomeAddCmd{
		HostCommand: proto.HostCommand{
			Type:      "home.add",
			Nonce:     w.cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			To:        hostNode.ID(),
		},
		Group: w.gid,
		Home:  homeID,
		Name:  w.groupName,
		Addrs: []string{w.cast.RVURL},
	})
	return err
}

func (w *s2ExpWorld) syncBrowserView(bc *peerBrowserConn, v *s2ExpView) error {
	w.drainBufferedPushes(bc, v)
	raw, err := bc.conn.Request(w.cast.Ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.gid), Since: v.cursor}
	})
	if err != nil {
		return err
	}
	var synced proto.Synced
	if err := proto.Decode(raw, &synced); err != nil {
		return fmt.Errorf("sync failed: %s (%w)", raw, err)
	}
	w.drainBufferedPushes(bc, v)
	v.cursor = max(v.cursor, synced.Seq)
	return nil
}

func (w *s2ExpWorld) drainBufferedPushes(bc *peerBrowserConn, v *s2ExpView) {
	for {
		select {
		case raw, ok := <-bc.pushes:
			if !ok {
				return
			}
			v.applyFrame(raw, string(w.gid))
		default:
			return
		}
	}
}

func (w *s2ExpWorld) drainViewUntil(bc *peerBrowserConn, v *s2ExpView, done func() bool) error {
	w.drainBufferedPushes(bc, v)
	if done() {
		return nil
	}
	for {
		raw, err := bc.nextPush(w.cast.Ctx)
		if err != nil {
			return err
		}
		v.applyFrame(raw, string(w.gid))
		if done() {
			return nil
		}
	}
}

func (w *s2ExpWorld) snapshotGroupRows(bc *peerBrowserConn, v *s2ExpView) (s2ExpRowSnap, error) {
	out := s2ExpRowSnap{}
	if err := w.collectSyncRows(bc, v, 0, true, out); err != nil {
		return nil, err
	}
	if err := w.collectSyncRows(bc, v, 1, false, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (w *s2ExpWorld) collectSyncRows(bc *peerBrowserConn, v *s2ExpView, since int64, stateOnly bool, out s2ExpRowSnap) error {
	w.drainBufferedPushes(bc, v)
	raw, err := bc.conn.Request(w.cast.Ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.gid), Since: since, StateOnly: stateOnly}
	})
	if err != nil {
		return err
	}
	var synced proto.Synced
	if err := proto.Decode(raw, &synced); err != nil {
		return fmt.Errorf("snapshot sync(%d): %s (%w)", since, raw, err)
	}
	return w.drainRowsUpToSeq(bc, v, synced.Seq, out)
}

func (w *s2ExpWorld) drainRowsUpToSeq(bc *peerBrowserConn, v *s2ExpView, targetSeq int64, out s2ExpRowSnap) error {
	deadline := time.After(50 * time.Millisecond)
	for s2ExpMaxSnapSeq(out) < targetSeq {
		select {
		case raw, ok := <-bc.pushes:
			if !ok {
				return nil
			}
			v.applyFrame(raw, string(w.gid))
			s2ExpRecordBatch(raw, string(w.gid), out)
		case <-deadline:
			return nil
		}
	}
	return nil
}

func s2ExpMaxSnapSeq(snap s2ExpRowSnap) int64 {
	var maxSeq int64
	for _, raw := range snap {
		var h proto.RowHeader
		if proto.Decode(raw, &h) == nil && h.Seq > maxSeq {
			maxSeq = h.Seq
		}
	}
	return maxSeq
}

func s2ExpRecordBatch(raw json.RawMessage, gid string, out s2ExpRowSnap) {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Group != gid {
		return
	}
	for _, elem := range batch.Rows {
		var h proto.RowHeader
		if proto.Decode(elem, &h) == nil {
			out[[2]string{h.Type, h.ID}] = elem
		}
	}
}

func s2ExpCompareSnaps(want, got s2ExpRowSnap) error {
	if len(want) != len(got) {
		return fmt.Errorf("row count = %d, want %d", len(got), len(want))
	}
	for key, wantRaw := range want {
		gotRaw, ok := got[key]
		if !ok {
			return fmt.Errorf("missing row %v on P2", key)
		}
		if err := s2ExpEqualRow(key, wantRaw, gotRaw); err != nil {
			return err
		}
	}
	return nil
}

func s2ExpEqualRow(key [2]string, wantRaw, gotRaw json.RawMessage) error {
	if bytes.Equal(wantRaw, gotRaw) {
		return nil
	}
	if key[0] == "member" && s2ExpSameAIMemberIdentity(wantRaw, gotRaw) {
		return nil
	}
	return fmt.Errorf("row %v mismatch:\n got: %s\nwant: %s", key, gotRaw, wantRaw)
}

func s2ExpSameAIMemberIdentity(aRaw, bRaw json.RawMessage) bool {
	var a, b proto.AIMemberRow
	if proto.Decode(aRaw, &a) != nil || proto.Decode(bRaw, &b) != nil {
		return false
	}
	a.Seq, a.LastSeen, a.Since, a.Path, a.State = 0, 0, 0, "", ""
	b.Seq, b.LastSeen, b.Since, b.Path, b.State = 0, 0, 0, "", ""
	return reflect.DeepEqual(a, b)
}

func (w *s2ExpWorld) runLocalCmdExpectOK(n *s2Node, args ...string) error {
	code, stdout, stderr := w.execLocalCmd(n, args...)
	lines := s2ExpNonEmptyLines(stdout)
	if code != 0 || len(lines) != 1 || stderr != "" {
		return fmt.Errorf("%s %v: code=%d stdout=%q stderr=%q", n.Name, args, code, stdout, stderr)
	}
	return nil
}

func (w *s2ExpWorld) runLocalCmdExpectFail(n *s2Node, args ...string) error {
	code, stdout, stderr := w.execLocalCmd(n, args...)
	combined := strings.TrimSpace(stdout + stderr)
	if code == 0 || combined == "" || strings.Contains(combined, "\n") {
		return fmt.Errorf("%s %v expected 1-line non-zero failure, got code=%d stdout=%q stderr=%q", n.Name, args, code, stdout, stderr)
	}
	return nil
}

func (w *s2ExpWorld) execLocalCmd(n *s2Node, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(w.f.bin, args...)
	cmd.Env = n.Env
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	return cmd.ProcessState.ExitCode(), stdout.String(), stderr.String()
}

func s2ExpNonEmptyLines(s string) []string {
	trimmed := strings.TrimSuffix(s, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func (w *s2ExpWorld) waitCLITurnCount(sub string, wantAtLeast int) error {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		n, err := w.countCLITurns(sub)
		if err != nil {
			return err
		}
		if n >= wantAtLeast {
			return nil
		}
		time.Sleep(40 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %d CLI turn(s) containing %q", wantAtLeast, sub)
}

func (w *s2ExpWorld) countCLITurns(sub string) (int, error) {
	entries, err := w.cast.CLILogs()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.Kind == "turn" && strings.Contains(e.Text, sub) {
			count++
		}
	}
	return count, nil
}
