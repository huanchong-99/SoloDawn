//go:build acceptance

package features

import (
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
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["s2_export"] = s2ExportSteps
}

type s2ExportWorld struct {
	f          fixtures
	cleanup    func(func())
	c          *s2Cast
	g, task    proto.ID
	file       string
	link       s2JoinLink
	secretary  string
	qTeam      string
	item       int64
	mention    proto.ID
	rows       [][]any
	importedAt time.Time
}

func s2ExportSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2ExportWorld{f: f, cleanup: afterScenario(sc)}
	sc.Step(`^P has a project group "([^"]+)" on P1 with Q and Q-team on Q1$`, w.group)
	sc.Step(`^the group has a secretary on P1, P-work on P2, task 1 and a gone note$`, w.moveFixture)
	sc.Step(`^P1 holds an unacknowledged mention for Q-team$`, w.queueMention)
	sc.Step(`^P exports the group on P1 with --to P2$`, w.moveExport)
	sc.Step(`^P1 no longer lists the group and answers cmd, sync and fetch with moved to P2$`, w.checkMoved)
	sc.Step(`^P imports the file on P2$`, w.importFile)
	sc.Step(`^P2 is home and has the exported rows$`, w.checkRows)
	sc.Step(`^the two browser peers follow P2 within 5 seconds of import without a reload$`, w.browsersFollow)
	sc.Step(`^Q1 follows P2 by its normal retry, within 70 seconds of import$`, w.q1Follows)
	sc.Step(`^P2 delivers the unacknowledged mention once and the next task gets number 2$`, w.checkDeliveryAndNumber)
	sc.Step(`^the export cannot be imported on Q1 or exported again from P1$`, w.checkFailures)
	sc.Step(`^P takes two successive copies on P1 and writes a note after each copy$`, w.copyExports)
	sc.Step(`^P1 is stopped and its data directory is deleted$`, w.deleteOldHome)
	sc.Step(`^Q follows an invitation from P2 and join succeeds without adding a member row$`, w.restoreMember)
	sc.Step(`^Q's browser peer and Q1 use P2 as the restored home$`, w.followRestored)
}

func (w *s2ExportWorld) group(name string) error {
	c, err := startS2Cast(w.f.t, w.cleanup, w.f, s2CastConfig{Timeout: 3 * time.Minute})
	if err != nil {
		return err
	}
	w.c = c
	w.g, err = c.CreateProjectGroup(c.P, c.P1, name, "none")
	if err != nil {
		return err
	}
	link, err := w.inviteQ(c.P1)
	if err != nil {
		return err
	}
	if _, _, err := c.JoinGroup(c.Q, link, c.Q.Name); err != nil {
		return err
	}
	ai, err := c.JoinAI(c.Q, c.Q1, c.P1, w.g, s2QTeamCLI, s2QTeamName, false)
	w.qTeam = ai.ID
	return err
}

func (w *s2ExportWorld) header(typ string) proto.HomeCommand {
	return proto.HomeCommand{Type: typ, Nonce: w.c.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.g}
}

func (w *s2ExportWorld) inviteQ(home *s2Node) (s2JoinLink, error) {
	c := w.c
	res, err := c.SendCmd(c.P, c.P.Conns[home.ID()].conn, w.header("invite"))
	if err != nil {
		return s2JoinLink{}, err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	if err == nil && (link.Group != string(w.g) || link.Home != home.ID()) {
		err = errors.New("invitation does not name the group and home")
	}
	return link, err
}

func (w *s2ExportWorld) dialQ(home string) (*peerBrowserConn, error) {
	c, q := w.c, w.c.Q
	bc, err := q.Browser.openCtl(c.Ctx, c.addUndo, home)
	if err != nil {
		return nil, err
	}
	raw, err := bc.request(c.Ctx, func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 2, Kind: "browser", Name: q.Name, Addrs: []string{}}
	})
	if err != nil {
		return nil, err
	}
	var welcome proto.Welcome
	if err := proto.Decode(raw, &welcome); err != nil {
		return nil, fmt.Errorf("Q's hello: %s: %w", raw, err)
	}
	q.Conns[home] = bc
	return bc, nil
}

func (w *s2ExportWorld) moveFixture() error {
	c := w.c
	if err := c.WriteScript(c.P1, s2SecretaryCLI, s2ExportSecretaryScript()); err != nil {
		return err
	}
	secretary, err := c.JoinAI(c.P, c.P1, c.P1, w.g, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	w.secretary = secretary.ID
	worker, err := c.JoinAI(c.P, c.P2, c.P1, w.g, s2PWorkCLI, s2PWorkName, false)
	if err != nil {
		return err
	}
	if err := w.createTask(c.P1, "first task", 1); err != nil {
		return err
	}
	if err := w.waitInitialTurns(worker.ID); err != nil {
		return err
	}
	return w.makeGoneNote()
}

func (w *s2ExportWorld) waitInitialTurns(worker string) error {
	var secretaryWorking, workerWorking, secretaryDone, workerDone bool
	return w.c.readGroupRowsUntil(w.c.P.Conns[w.c.P1.ID()], w.g, func(raw json.RawMessage) (bool, error) {
		secretaryDone = matchS2WorkingThenWaiting(raw, w.secretary, &secretaryWorking) || secretaryDone
		workerDone = matchS2WorkingThenWaiting(raw, worker, &workerWorking) || workerDone
		return secretaryDone && workerDone, nil
	})
}

func s2ExportSecretaryScript() string {
	brief := func(title string) []m5ToolCallSpec {
		args := m5BriefArgs([]map[string]any{{"title": title, "assignee": s2PWorkName, "reviewer": s2QTeamName}})
		args["title"] = title
		args["requester"] = s2PName
		return []m5ToolCallSpec{{Name: "propose_brief", Args: args}}
	}
	return strings.TrimSuffix(m5ClaudeScript(brief("first task"), nil, brief("second task"), nil), "]}") + `,{"dir":"eof"}]}`
}

func (w *s2ExportWorld) createTask(home *s2Node, title string, num int) error {
	if err := postS2Mention(w.c, w.c.P, home, w.g, w.secretary, title); err != nil {
		return err
	}
	bc := w.c.P.Conns[home.ID()]
	brief, err := w.waitBrief(bc)
	if err != nil {
		return err
	}
	if _, err := w.c.SendCmd(w.c.P, bc.conn, proto.Act{HomeCommand: w.header("act"), Msg: brief, Action: "confirm"}); err != nil {
		return err
	}
	return w.waitTask(bc, title, num)
}

func (w *s2ExportWorld) waitBrief(bc *peerBrowserConn) (proto.ID, error) {
	var brief proto.ID
	err := w.c.readGroupRowsUntil(bc, w.g, func(raw json.RawMessage) (bool, error) {
		var m proto.MsgRow
		if proto.Decode(raw, &m) != nil || m.Kind != "brief" || m.State == nil || m.State.Status != "pending" {
			return false, nil
		}
		brief = proto.ID(m.ID)
		return true, nil
	})
	return brief, err
}

func (w *s2ExportWorld) waitTask(bc *peerBrowserConn, title string, num int) error {
	return w.c.readGroupRowsUntil(bc, w.g, func(raw json.RawMessage) (bool, error) {
		var task proto.TaskRow
		if proto.Decode(raw, &task) != nil || task.Title != title {
			return false, nil
		}
		if task.Num != num {
			return false, fmt.Errorf("task %q number = %d, want %d", title, task.Num, num)
		}
		w.task = proto.ID(task.ID)
		return true, nil
	})
}

func (w *s2ExportWorld) note(id proto.ID, text string) error {
	_, err := w.c.SendCmd(w.c.P, w.c.P.Conns[w.c.P1.ID()].conn,
		proto.Note{HomeCommand: w.header("note"), ID: id, Text: text})
	return err
}

func (w *s2ExportWorld) makeGoneNote() error {
	id := w.c.NextID()
	if err := w.note(id, "note to forget before export"); err != nil {
		return err
	}
	_, err := w.c.SendCmd(w.c.P, w.c.P.Conns[w.c.P1.ID()].conn,
		proto.Forget{HomeCommand: w.header("forget"), Row: "note", ID: id})
	return err
}

func (w *s2ExportWorld) taskOp(home *s2Node, op string) error {
	_, err := w.c.SendCmd(w.c.P, w.c.P.Conns[home.ID()].conn,
		proto.TaskCommand{HomeCommand: w.header("task"), Op: op, ID: w.task})
	return err
}

func (w *s2ExportWorld) queueMention() error {
	c := w.c
	if err := w.taskOp(c.P1, "pause"); err != nil {
		return err
	}
	h := w.header("post")
	if _, err := c.SendCmd(c.Q, c.Q.Conns[c.P1.ID()].conn, proto.Post[proto.EmptyBody]{
		HomeCommand: h, Kind: "text", Text: "Carry this over to P2", Thread: &w.task,
		Mentions: []proto.ID{proto.ID(w.qTeam)}, Body: proto.EmptyBody{},
	}); err != nil {
		return err
	}
	w.mention = h.Nonce
	rows, err := readS2SQLiteRows(c.Ctx, s2ExportDB(c.P1),
		"SELECT count(*) FROM inbox WHERE g=? AND ai=? AND msg=? AND why='mention'", w.g, w.qTeam, w.mention)
	if err != nil || !reflect.DeepEqual(rows, [][]any{{int64(1)}}) {
		return fmt.Errorf("pending mentions on P1 = %v, want 1: %v", rows, err)
	}
	return nil
}

func s2ExportDB(node *s2Node) string {
	return filepath.Join(node.Data, "node.db")
}

func readS2SQLiteRows(ctx context.Context, path, query string, args ...any) ([][]any, error) {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return collectS2SQLiteRows(rows, len(columns))
}

func (w *s2ExportWorld) groupRows(path string) ([][]any, error) {
	return readS2SQLiteRows(w.c.Ctx, path, `SELECT g,r,id,seq,pos,ts,mt,thread,in_main,pending,final,keep,gone,
		CASE WHEN r='group' THEN json_remove(data,'$.home') ELSE data END
		FROM row WHERE g=? ORDER BY r,id`, w.g)
}

func collectS2SQLiteRows(rows *sql.Rows, columns int) ([][]any, error) {
	var out [][]any
	for rows.Next() {
		values := make([]any, columns)
		dest := make([]any, columns)
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		out = append(out, values)
	}
	return out, rows.Err()
}

func (w *s2ExportWorld) local(node *s2Node, args ...string) (string, error) {
	cmd := exec.CommandContext(w.c.Ctx, w.f.bin, args...)
	cmd.Env = node.Env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (w *s2ExportWorld) export(file string, move ...string) error {
	w.file = filepath.Join(w.c.Root, file)
	args := append([]string{"group", "export", string(w.g), w.file}, move...)
	if out, err := w.local(w.c.P1, args...); err != nil {
		return fmt.Errorf("export on P1: %w: %s", err, out)
	}
	rows, err := w.groupRows(w.file)
	if err != nil {
		return err
	}
	w.rows = rows
	if len(move) == 0 {
		return w.compareRows(s2ExportDB(w.c.P1))
	}
	return nil
}

func (w *s2ExportWorld) moveExport() error {
	held, err := w.groupRows(s2ExportDB(w.c.P1))
	if err != nil {
		return err
	}
	if err := w.export("move.db", "--to", w.c.P2.ID()); err != nil {
		return err
	}
	if !reflect.DeepEqual(held, w.rows) {
		return fmt.Errorf("exported rows differ: P1 held %v, the file has %v", held, w.rows)
	}
	return w.checkMovedFile()
}

func (w *s2ExportWorld) checkMovedFile() error {
	items, err := readS2SQLiteRows(w.c.Ctx, w.file,
		"SELECT count(*) FROM inbox WHERE g=? AND ai=? AND msg=? AND why='mention'", w.g, w.qTeam, w.mention)
	if err != nil || !reflect.DeepEqual(items, [][]any{{int64(1)}}) {
		return fmt.Errorf("exported mentions = %v, want 1: %v", items, err)
	}
	gone, err := readS2SQLiteRows(w.c.Ctx, w.file, "SELECT count(*) FROM row WHERE g=? AND r='note' AND gone=1", w.g)
	if err != nil || !reflect.DeepEqual(gone, [][]any{{int64(1)}}) {
		return fmt.Errorf("exported gone notes = %v, want 1: %v", gone, err)
	}
	return nil
}

func (w *s2ExportWorld) checkMoved() error {
	c := w.c
	conn := c.P.Conns[c.P1.ID()].conn
	welcome, err := requestBrowserHello(c.Ctx, conn)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(welcome.Groups, func(g proto.GroupSummary) bool { return g.Group == string(w.g) }) {
		return errors.New("P1 still lists the group")
	}
	return w.checkMovedRequests(conn)
}

func (w *s2ExportWorld) checkMovedRequests(conn *peer.Connection) error {
	body := proto.Note{HomeCommand: w.header("note"), ID: w.c.NextID(), Text: "note after move"}
	raw, err := sendSignedCmdRaw(w.c.Ctx, conn, w.c.P.Key, body)
	if err != nil {
		return err
	}
	if err := w.checkMovedReply(raw, "cmd"); err != nil {
		return err
	}
	if err := w.movedSync(conn); err != nil {
		return err
	}
	return w.expectMoved(conn, "fetch", func(r proto.Request) any {
		return proto.FetchThread{FetchPage: proto.FetchPage{Request: r, Group: string(w.g), Limit: 1}}
	})
}

func (w *s2ExportWorld) movedSync(conn *peer.Connection) error {
	return w.expectMoved(conn, "sync", func(r proto.Request) any { return proto.Sync{Request: r, Group: string(w.g)} })
}

func (w *s2ExportWorld) expectMoved(conn *peer.Connection, op string, build func(proto.Request) any) error {
	raw, err := requestWait(w.c.Ctx, conn, nil, func(id int64) any { return build(proto.Request{Type: op, ID: id}) })
	if err != nil {
		return err
	}
	return w.checkMovedReply(raw, op)
}

func (w *s2ExportWorld) checkMovedReply(raw json.RawMessage, op string) error {
	var answer proto.ErrorResponse
	if err := proto.Decode(raw, &answer); err != nil || answer.Err.Code != "moved" || answer.Err.Home != w.c.P2.ID() {
		return fmt.Errorf("%s answered %s, want moved to P2", op, raw)
	}
	return nil
}

func (w *s2ExportWorld) importFile() error {
	if out, err := w.local(w.c.P2, "group", "import", w.file); err != nil {
		return fmt.Errorf("import on P2: %w: %s", err, out)
	}
	w.importedAt = time.Now()
	return nil
}

func (w *s2ExportWorld) checkRows() error {
	if err := w.compareRows(s2ExportDB(w.c.P2)); err != nil {
		return err
	}
	return w.checkHome(w.c.P)
}

func (w *s2ExportWorld) compareRows(path string) error {
	got, err := w.groupRows(path)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(w.rows, got) {
		return fmt.Errorf("group rows differ: want %v, got %v", w.rows, got)
	}
	return nil
}

func (w *s2ExportWorld) checkHome(p *s2Person) error {
	bc := p.Conns[w.c.P2.ID()]
	if err := w.c.syncGroup(bc.conn, w.g); err != nil {
		return err
	}
	return w.c.readGroupRowsUntil(bc, w.g, func(raw json.RawMessage) (bool, error) {
		var row proto.GroupRow
		if proto.Decode(raw, &row) != nil || row.Type != "group" {
			return false, nil
		}
		if row.Home != w.c.P2.ID() {
			return false, fmt.Errorf("group row lists home %s, want P2", row.Home)
		}
		return true, nil
	})
}

func (w *s2ExportWorld) browsersFollow() error {
	if err := w.followBrowserPeers(); err != nil {
		return err
	}
	return w.followedWithin(5 * time.Second)
}

// s2ExportRetry is the longest wait of planning/flows/connect.md step 15 between two offers:
// 10 seconds for an answer and a back-off of at most 60.
const s2ExportRetry = 70 * time.Second

func (w *s2ExportWorld) q1Follows() error {
	if err := w.checkQ1Turn("Hello from the new home"); err != nil {
		return err
	}
	return w.followedWithin(s2ExportRetry)
}

func (w *s2ExportWorld) followedWithin(limit time.Duration) error {
	if duration := time.Since(w.importedAt); duration > limit {
		return fmt.Errorf("following P2 took %s after import, want at most %s", duration, limit)
	}
	return nil
}

func (w *s2ExportWorld) followBrowserPeers() error {
	c := w.c
	if err := w.movedSync(c.Q.Conns[c.P1.ID()].conn); err != nil {
		return err
	}
	if _, err := w.dialQ(c.P2.ID()); err != nil {
		return err
	}
	return w.checkHome(c.Q)
}

func (w *s2ExportWorld) checkDeliveryAndNumber() error {
	if err := w.deliverMention(); err != nil {
		return err
	}
	if err := w.createTask(w.c.P2, "second task", 2); err != nil {
		return err
	}
	_, err := w.resumedMentionTurn()
	return err
}

func (w *s2ExportWorld) deliverMention() error {
	c := w.c
	items, err := readS2SQLiteRows(c.Ctx, s2ExportDB(c.P2),
		"SELECT id FROM inbox WHERE g=? AND ai=? AND msg=? AND why='mention'", w.g, w.qTeam, w.mention)
	if err != nil || len(items) != 1 {
		return fmt.Errorf("imported mention = %v, want one item: %v", items, err)
	}
	w.item = items[0][0].(int64)
	if err := w.taskOp(c.P2, "resume"); err != nil {
		return err
	}
	if err := c.readGroupRowsUntil(c.P.Conns[c.P2.ID()], w.g, func(json.RawMessage) (bool, error) {
		return w.resumedMentionTurn()
	}); err != nil {
		return err
	}
	return w.checkDelivery()
}

func (w *s2ExportWorld) resumedMentionTurn() (bool, error) {
	entries, err := w.c.CLILogs()
	if err != nil {
		return false, err
	}
	count := 0
	for _, entry := range entries {
		if isS2ExportMentionTurn(entry) {
			count++
		}
	}
	if count > 1 {
		return false, fmt.Errorf("resumed mention CLI turns = %d, want exactly 1", count)
	}
	return count == 1, nil
}

func isS2ExportMentionTurn(entry s2CLILogEntry) bool {
	return entry.Kind == "turn" && entry.Node == "Q1" && entry.Vendor == s2QTeamCLI && strings.Contains(entry.Text, "Carry this over to P2")
}

func (w *s2ExportWorld) checkDelivery() error {
	c := w.c
	homes, err := readS2SQLiteRows(c.Ctx, s2ExportDB(c.Q1),
		"SELECT home FROM audit WHERE g=? AND what='delivered' AND home=? AND item=?", w.g, c.P2.ID(), w.item)
	if err != nil || !reflect.DeepEqual(homes, [][]any{{c.P2.ID()}}) {
		return fmt.Errorf("delivery homes = %v, want only P2: %v", homes, err)
	}
	pending, err := readS2SQLiteRows(c.Ctx, s2ExportDB(c.P2),
		"SELECT count(*) FROM inbox WHERE g=? AND ai=? AND msg=? AND why='mention'", w.g, w.qTeam, w.mention)
	if err != nil || !reflect.DeepEqual(pending, [][]any{{int64(0)}}) {
		return fmt.Errorf("P2 pending copies = %v, want 0: %v", pending, err)
	}
	return nil
}

func (w *s2ExportWorld) checkFailures() error {
	if err := w.localFailure(w.c.Q1, "group", "import", w.file); err != nil {
		return err
	}
	return w.localFailure(w.c.P1, "group", "export", string(w.g), filepath.Join(w.c.Root, "not-home.db"))
}

func (w *s2ExportWorld) localFailure(node *s2Node, args ...string) error {
	out, err := w.local(node, args...)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() <= 0 || out == "" || strings.Contains(out, "\n") {
		return fmt.Errorf("%s %s: want a nonzero exit and one line; got %v: %s", node.Name, args[1], err, out)
	}
	return nil
}

func (w *s2ExportWorld) copyExports() error {
	if err := w.export("copy-1.db"); err != nil {
		return err
	}
	if err := w.note(w.c.NextID(), "written between copies"); err != nil {
		return err
	}
	if err := w.export("copy-2.db"); err != nil {
		return err
	}
	return w.note(w.c.NextID(), "written only after the last copy")
}

func (w *s2ExportWorld) deleteOldHome() error {
	w.c.P1.Stop()
	return os.RemoveAll(w.c.P1.Data)
}

func (w *s2ExportWorld) restoreMember() error {
	link, err := w.inviteQ(w.c.P2)
	if err != nil {
		return err
	}
	if _, _, err := w.c.sendJoin(w.c.Q, link, w.c.Q.Name); err != nil {
		return err
	}
	w.link = link
	return w.compareRows(s2ExportDB(w.c.P2))
}

func (w *s2ExportWorld) followRestored() error {
	if err := w.c.followJoin(w.c.Q, w.link); err != nil {
		return err
	}
	if err := w.checkHome(w.c.Q); err != nil {
		return err
	}
	return w.checkQ1Turn("Hello from the restored home")
}

func (w *s2ExportWorld) checkQ1Turn(text string) error {
	c := w.c
	if err := postS2Mention(c, c.Q, c.P2, w.g, w.qTeam, text); err != nil {
		return err
	}
	return c.readGroupRowsUntil(c.Q.Conns[c.P2.ID()], w.g, func(row json.RawMessage) (bool, error) {
		var ai proto.AIMemberRow
		return proto.Decode(row, &ai) == nil && ai.ID == w.qTeam && ai.State == "working", nil
	})
}
