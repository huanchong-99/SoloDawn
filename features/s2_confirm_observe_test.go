//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// s2ConfirmNoWorktree is what the steps that MODEL-1126 took away say when they fail.
const s2ConfirmNoWorktree = "a task runs in its assignee's workspace: there is no task worktree or review checkout (MODEL-1126 plan 1.5)"

func s2ConfirmObservationSteps(sc *godog.ScenarioContext, w *s2ConfirmWorld) {
	sc.Step(`^a confirmation "(msg|task|member|group)" row is seen as "([^"]+)":$`, w.awaitRow)
	sc.Step(`^the confirmation "(msg|task|member|group)" rows contain (\d+) matches:$`, w.countRows)
	sc.Step(`^"([^"]+)" receives a confirmation turn matching "([^"]+)"$`, w.awaitTurn)
	sc.Step(`^"([^"]+)" has (\d+) confirmation items of kind "([^"]+)"$`, w.countItems)
	sc.Step(`^Q1 has no confirmation task or review checkout$`, rewritten("AC-3", s2ConfirmNoWorktree))
	sc.Step(`^the approval "([^"]+)" mentions P and Q$`, w.approvalMentions)
	sc.Step(`^P's confirmation browser sees "([^"]+)" as "([^"]+)"$`, w.browserCardStatus)
}

func (w *s2ConfirmWorld) browserCardStatus(card, status string) error {
	raw, err := w.browsers["P"].request(w.cast.Ctx, func(id int64) any {
		return proto.FetchRow{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: string(w.gid), Limit: 1},
			Row:       "msg", RowID: w.ids[card],
		}
	})
	if err != nil {
		return err
	}
	var result proto.Fetched
	if err := proto.Decode(raw, &result); err != nil {
		return err
	}
	if len(result.Rows) != 1 {
		return fmt.Errorf("P's browser cannot read card %s", card)
	}
	return s2ConfirmBrowserStatus(result.Rows[0], card, status)
}

func s2ConfirmBrowserStatus(raw json.RawMessage, card, status string) error {
	var row proto.MsgRow
	if err := proto.Decode(raw, &row); err != nil {
		return err
	}
	if row.State == nil || row.State.Status != status {
		return fmt.Errorf("P's browser got card %s with state %+v, want %s", card, row.State, status)
	}
	return nil
}

func (w *s2ConfirmWorld) approvalMentions(name string) error {
	rows, err := readS2SQLiteRows(w.cast.Ctx, s2ExportDB(w.cast.P1), "SELECT data FROM row WHERE g = ? AND r = 'msg' AND id = ?", w.gid, w.ids[name])
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return sql.ErrNoRows
	}
	var row proto.MsgRow
	if err := proto.Decode([]byte(rows[0][0].(string)), &row); err != nil {
		return err
	}
	want := []string{w.ids["P-member"], w.ids["Q-member"]}
	slices.Sort(want)
	if !slices.Equal(row.Mentions, want) {
		return fmt.Errorf("approval mentions %v, want %v", row.Mentions, want)
	}
	return nil
}

func (w *s2ConfirmWorld) observe(check func() (bool, error)) error {
	if done, err := check(); done || err != nil {
		return err
	}
	return w.cast.readGroupRowsUntil(w.browsers["P"], w.gid, func(json.RawMessage) (bool, error) {
		return check()
	})
}

func (w *s2ConfirmWorld) awaitRow(kind, name string, doc *godog.DocString) error {
	query, err := w.rowQuery(doc)
	if err != nil {
		return err
	}
	return w.observe(func() (bool, error) {
		ids, err := w.matchRows(kind, query)
		if err != nil || len(ids) == 0 {
			return false, err
		}
		if len(ids) != 1 {
			return false, fmt.Errorf("found %d rows for %s", len(ids), doc.Content)
		}
		w.ids[name] = ids[0]
		return true, nil
	})
}

func (w *s2ConfirmWorld) countRows(kind string, want int, doc *godog.DocString) error {
	query, err := w.rowQuery(doc)
	if err != nil {
		return err
	}
	ids, err := w.matchRows(kind, query)
	if err != nil {
		return err
	}
	return s2ConfirmCount(doc.Content, len(ids), want)
}

func (w *s2ConfirmWorld) rowQuery(doc *godog.DocString) (map[string]any, error) {
	raw, err := w.expand(doc.Content)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	err = json.Unmarshal([]byte(raw), &fields)
	return fields, err
}

func (w *s2ConfirmWorld) matchRows(kind string, fields map[string]any) ([]string, error) {
	rows, err := readS2SQLiteRows(w.cast.Ctx, s2ExportDB(w.cast.P1), "SELECT id, data FROM row WHERE g = ? AND r = ? AND gone = 0", w.gid, kind)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, values := range rows {
		var row any
		if err := json.Unmarshal([]byte(values[1].(string)), &row); err != nil {
			return nil, err
		}
		if s2ConfirmMatches(row, fields) {
			found = append(found, values[0].(string))
		}
	}
	return found, nil
}

func s2ConfirmMatches(got, want any) bool {
	fields, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(got, want)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		return false
	}
	for key, value := range fields {
		if actual, exists := obj[key]; !exists || !s2ConfirmMatches(actual, value) {
			return false
		}
	}
	return true
}

func (w *s2ConfirmWorld) aiNode(name string) *s2Node {
	nodes := []*s2Node{w.cast.P1, w.cast.P2, w.cast.Q1}
	return nodes[slices.IndexFunc(nodes, func(n *s2Node) bool { return n.ID() == w.ais[name].Host })]
}

func (w *s2ConfirmWorld) awaitTurn(ai, pattern string) error {
	re, err := regexp.Compile("(?m)" + pattern)
	if err != nil {
		return err
	}
	err = w.observe(func() (bool, error) {
		logs, err := w.cast.CLILogs()
		return w.hasTurn(logs, ai, re), err
	})
	if err != nil {
		return fmt.Errorf("%s turn matching %q: %w", ai, pattern, err)
	}
	return nil
}

func (w *s2ConfirmWorld) hasTurn(logs []s2CLILogEntry, ai string, re *regexp.Regexp) bool {
	for _, entry := range logs {
		if entry.Kind == "turn" && entry.Node == w.aiNode(ai).Name && entry.Vendor == w.ais[ai].CLI && re.MatchString(entry.Text) {
			return true
		}
	}
	return false
}

func (w *s2ConfirmWorld) countItems(ai string, want int, why string) error {
	queued, err := readS2SQLiteRows(w.cast.Ctx, s2ExportDB(w.cast.P1), "SELECT id FROM inbox WHERE g = ? AND ai = ? AND why = ?", w.gid, w.ais[ai].ID, why)
	if err != nil {
		return err
	}
	delivered, err := w.delivered(ai, why)
	if err != nil {
		return err
	}
	for _, id := range delivered {
		if !slices.ContainsFunc(queued, func(row []any) bool { return row[0] == id[0] }) {
			queued = append(queued, id)
		}
	}
	return s2ConfirmCount(ai+" "+why, len(queued), want)
}

func (w *s2ConfirmWorld) delivered(ai, why string) ([][]any, error) {
	return readS2SQLiteRows(w.cast.Ctx, s2ExportDB(w.aiNode(ai)), "SELECT item FROM audit WHERE g = ? AND agent = ? AND what = 'delivered' AND detail = ?", w.gid, w.ais[ai].CLI, why)
}

func s2ConfirmCount(subject string, got, want int) error {
	if got != want {
		return fmt.Errorf("%s: got %d, want %d", subject, got, want)
	}
	return nil
}

func (w *s2ConfirmWorld) awaitIdle(ai string) error {
	return w.observe(func() (bool, error) {
		ids, err := w.matchRows("member", map[string]any{"id": w.ais[ai].ID, "state": "waiting"})
		return len(ids) == 1, err
	})
}
