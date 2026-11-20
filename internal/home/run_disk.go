package home

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// What the loop reads from the store: every group when it starts, and, for a
// rule, what a home keeps on disk only.

// runStored is a group as the store holds it.
type runStored struct {
	group    string
	rows     []json.RawMessage // its state rows, in the order of their sequence numbers
	seq, num int64
}

// load reads every group that has a group row and makes its state: the
// owner's log, the state rows in their order, device rows included, so that
// the logs are what they were, and the two counters.
func (l *Loop) load() (states []*State, err error) {
	var stored []runStored
	err = l.cfg.Store.Write(func(tx *store.Tx) error {
		var err error
		stored, err = runRead(tx)
		return err
	})
	for _, held := range stored {
		s := l.fresh(held.group)
		for _, data := range held.rows {
			err = errors.Join(err, runHold(s, data))
		}
		s.Seq, s.Num = held.seq, max(s.Num, int(held.num))
		states = append(states, s)
	}
	return states, err
}

func runRead(tx *store.Tx) (stored []runStored, err error) {
	groups, err := tx.Groups()
	for _, group := range groups {
		rows, failed := tx.StateRows(group)
		seq, num, missed := tx.Counters(group)
		stored, err = append(stored, runStored{group, rows, seq, num}), errors.Join(err, failed, missed)
	}
	return stored, err
}

// runHolders reads a stored state row into the state, by its r, and for a
// member by its kind.
var runHolders = map[string]func(s *State, data []byte) error{
	"group": runHolder[proto.GroupRow], "member human": runHolder[proto.HumanMemberRow], "member ai": runHolder[proto.AIMemberRow],
	"device": runHolder[proto.DeviceRow], "task": runHolder[proto.TaskRow],
}

func runHolder[R wireRow](s *State, data []byte) error {
	var row R
	err := json.Unmarshal(data, &row)
	s.hold(row)
	return err
}

// runHold takes one stored state row into the state.
func runHold(s *State, data []byte) error {
	var kind struct {
		Type string `json:"r"`
		Kind string `json:"kind"`
	}
	json.Unmarshal(data, &kind) // what does not decode has no holder
	key := kind.Type
	if key == "member" {
		key += " " + kind.Kind
	}
	hold, ok := runHolders[key]
	if !ok {
		return fmt.Errorf("group %s holds a state row that is none: %.80s", s.ID, data)
	}
	return hold(s, data)
}

// runDisk is the Disk of one transaction. A read that fails leaves its error
// here, and whoever runs the transaction drops it.
type runDisk struct {
	tx    *store.Tx
	group string
	err   error
}

func (d *runDisk) Msg(id string) (proto.MsgRow, bool) { return runOne[proto.MsgRow](d, "msg", id) }

func (d *runDisk) Note(id string) (proto.NoteRow, bool) { return runOne[proto.NoteRow](d, "note", id) }

func (d *runDisk) Thread(task *string, before int64, limit int) []proto.MsgRow {
	data, err := d.tx.Thread(d.group, task, before, limit)
	return runList[proto.MsgRow](d, data, err)
}

func (d *runDisk) Pending() []proto.MsgRow {
	data, err := d.tx.Pending(d.group)
	return runList[proto.MsgRow](d, data, err)
}

func (d *runDisk) Queued() []store.Item {
	items, err := d.tx.Queued(d.group)
	d.err = errors.Join(d.err, err)
	return items
}

// runOne reads one row that is no marker. A row that is not there is no
// failure.
func runOne[R any](d *runDisk, r, id string) (row R, ok bool) {
	data, err := d.tx.Row(d.group, r, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false
	}
	if err == nil {
		err = json.Unmarshal(data, &row)
	}
	d.err = errors.Join(d.err, err)
	return row, err == nil
}

// runList decodes the rows of a read.
func runList[R any](d *runDisk, data []json.RawMessage, err error) []R {
	list := make([]R, len(data))
	for i, raw := range data {
		err = errors.Join(err, json.Unmarshal(raw, &list[i]))
	}
	d.err = errors.Join(d.err, err)
	return list
}
