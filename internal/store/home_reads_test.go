package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// within runs a read of these tests in a transaction of the writer.
func within[T any](t *testing.T, s *Store, read func(tx *Tx) (T, error)) (v T, err error) {
	t.Helper()
	err = s.Write(func(tx *Tx) (err error) {
		v, err = read(tx)
		return err
	})
	return v, err
}

func member(id string, seq int64) proto.HumanMemberRow {
	return proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: header("member", id, seq), Kind: "human"}}
}

func card(id string, seq int64, thread *string, status string) proto.MsgRow {
	row := msg(id, seq, thread, true, "decision")
	row.State = &proto.CardState{Status: status}
	return row
}

// What a home loads when it starts: its groups, their state rows in the
// order of their sequence numbers without markers, and the two counters.
func TestHomeReadsTheStateOfAGroup(t *testing.T) {
	s := openDB(t, ":memory:")
	check(t, s.Write(func(tx *Tx) error {
		for range 9 {
			if _, err := tx.NextSeq("g"); err != nil {
				return err
			}
		}
		_, err := tx.NextNum("g")
		return err
	}))
	put(t, s, "g", 10, proto.GroupRow{RowHeader: header("group", "g", 1)})
	put(t, s, "g", 10, member("m1", 2))
	put(t, s, "g", 10, proto.TaskRow{RowHeader: header("task", "t1", 3)})
	put(t, s, "g", 10, msg("main", 4, nil, false, "text"))
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "n1", 5), Pos: 5})
	put(t, s, "g", 10, proto.DeviceRow{RowHeader: header("device", "d1", 6)})
	put(t, s, "g", 10, member("m2", 7))
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("member", "m2", 8), Gone: true})
	put(t, s, "g", 30, member("m1", 9))
	put(t, s, "b", 10, proto.GroupRow{RowHeader: header("group", "b", 1)})
	put(t, s, "c", 10, proto.GroupRow{RowHeader: header("group", "c", 1)})
	put(t, s, "c", 20, proto.GoneRow{RowHeader: header("group", "c", 2), Gone: true})
	put(t, s, "d", 10, member("m1", 1))

	groups, err := within(t, s, (*Tx).Groups)
	equal(t, []any{groups, err}, []any{[]string{"b", "g"}, nil})
	state := func(group string) []string {
		rows, err := within(t, s, func(tx *Tx) ([]json.RawMessage, error) { return tx.StateRows(group) })
		return ids(t, rows, err)
	}
	equal(t, state("g"), []string{"g", "t1", "d1", "m1"})
	equal(t, state("none"), []string{})

	counters := func(group string) [2]int64 {
		c, err := within(t, s, func(tx *Tx) ([2]int64, error) {
			seq, num, err := tx.Counters(group)
			return [2]int64{seq, num}, err
		})
		check(t, err)
		return c
	}
	equal(t, counters("g"), [2]int64{9, 1})
	equal(t, counters("none"), [2]int64{0, 0})
	// The task counter goes up to a number, and never down.
	raise := func(group string, num int) { check(t, s.Write(func(tx *Tx) error { return tx.RaiseNum(group, num) })) }
	raise("g", 7)
	equal(t, counters("g"), [2]int64{9, 7})
	raise("g", 3)
	raise("none", 3)
	equal(t, counters("g"), [2]int64{9, 7})
	equal(t, counters("none"), [2]int64{0, 0})
}

// What a rule of the home reads in its transaction: one row that is no
// marker, a page of a thread, the pending cards, and its own writes.
func TestHomeReadsWhatIsOnDiskOnly(t *testing.T) {
	s := openDB(t, ":memory:")
	thread, other := "t", "t2"
	put(t, s, "g", 10, msg("m1", 1, nil, false, "text"))
	put(t, s, "g", 10, card("c1", 2, &thread, "pending"))
	put(t, s, "g", 10, msg("m3", 3, &thread, false, "text"))
	put(t, s, "g", 10, msg("m4", 4, &other, false, "text"))
	put(t, s, "g", 10, card("c5", 5, nil, "pending"))
	put(t, s, "g", 10, card("c6", 6, nil, "decided"))
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "m1", 7), Pos: 7, Text: "a note with the id of a message"})
	put(t, s, "g", 10, msg("m8", 8, nil, false, "text"))
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("msg", "m8", 9), Gone: true})
	put(t, s, "b", 10, msg("m1", 1, &thread, false, "elsewhere"))
	put(t, s, "b", 10, card("c2", 2, nil, "pending"))

	row := func(group, r, id string) (string, error) {
		data, err := within(t, s, func(tx *Tx) (json.RawMessage, error) { return tx.Row(group, r, id) })
		return string(data), err
	}
	data, err := row("g", "note", "m1")
	equal(t, []any{data, err}, []any{`{"r":"note","id":"m1","seq":7,"pos":7,"g":"","task":null,"text":"a note with the id of a message","by":"","ts":0}`, nil})
	data, err = row("b", "msg", "m1")
	equal(t, []any{strings.Contains(data, `"kind":"elsewhere"`), err}, []any{true, nil})
	for _, missing := range [][3]string{{"g", "msg", "m8"}, {"g", "msg", "none"}, {"none", "msg", "m1"}, {"g", "task", "m1"}} {
		_, err = row(missing[0], missing[1], missing[2])
		equal(t, errors.Is(err, sql.ErrNoRows), true)
	}

	page := func(in *string, before int64, limit int) []string {
		rows, err := within(t, s, func(tx *Tx) ([]json.RawMessage, error) { return tx.Thread("g", in, before, limit) })
		return ids(t, rows, err)
	}
	equal(t, page(nil, 0, 10), []string{"c6", "c5", "c1", "m1"})
	equal(t, page(nil, 5, 10), []string{"c1", "m1"})
	equal(t, page(nil, 0, 2), []string{"c6", "c5"})
	equal(t, page(&thread, 0, 10), []string{"m3", "c1"})
	equal(t, page(&thread, 3, 10), []string{"c1"})
	equal(t, page(&thread, 0, 1), []string{"m3"})
	equal(t, page(&other, 0, 10), []string{"m4"})

	pending := func(tx *Tx) []string {
		rows, err := tx.Pending("g")
		return ids(t, rows, err)
	}
	// A read sees what its own transaction wrote before it, and a
	// transaction that is dropped leaves nothing to read.
	dropped := errors.New("dropped")
	err = s.Write(func(tx *Tx) error {
		equal(t, pending(tx), []string{"c1", "c5"})
		check(t, Put(tx, "g", 30, card("c10", 10, nil, "pending")))
		equal(t, pending(tx), []string{"c1", "c5", "c10"})
		return dropped
	})
	equal(t, err, dropped)
	check(t, s.Write(func(tx *Tx) error {
		equal(t, pending(tx), []string{"c1", "c5"})
		return nil
	}))
}

// The queue of a group is read oldest first with the items that are held
// back, and an item is retired by its number in its own group only.
func TestHomeReadsAndRetiresTheQueue(t *testing.T) {
	s := openDB(t, ":memory:")
	task := "task"
	held := Item{Group: "g", AI: "ai", Host: "host", Why: "review", Task: &task, NotBefore: 50, Created: 10}
	plain := Item{Group: "g", AI: "secretary", Host: "other", Why: "notice", Created: 20}
	check(t, s.Write(func(tx *Tx) error {
		for _, item := range []Item{held, {Group: "b", AI: "ai", Host: "host", Why: "notice"}, plain} {
			if _, err := tx.Enqueue(item, signed); err != nil {
				return err
			}
		}
		return nil
	}))
	held.ID, plain.ID = 1, 3
	held.Env, _ = signed(1)
	plain.Env, _ = signed(3)
	queued := func(group string) []Item {
		items, err := within(t, s, func(tx *Tx) ([]Item, error) { return tx.Queued(group) })
		check(t, err)
		return items
	}
	equal(t, queued("g"), []Item{held, plain})
	equal(t, queued("none"), []Item{})

	retire := func(group string, id int64) { check(t, s.Write(func(tx *Tx) error { return tx.Retire(group, id) })) }
	retire("b", 1)
	retire("g", 2)
	retire("g", 9)
	equal(t, queued("g"), []Item{held, plain})
	equal(t, len(queued("b")), 1)
	retire("g", 1)
	equal(t, queued("g"), []Item{plain})
}

// A read that fails returns its error and no list: when the query fails,
// and when a line of its result does not scan.
func TestHomeReadsFail(t *testing.T) {
	s := openDB(t, ":memory:")
	put(t, s, "g", 10, proto.GroupRow{RowHeader: header("group", "g", 1)})
	check(t, s.Write(func(tx *Tx) error {
		_, err := tx.Enqueue(Item{Group: "g"}, signed)
		return err
	}))
	check(t, s.Write(func(tx *Tx) error {
		rows, err := tx.Query("SELECT g FROM row")
		list, err := collect(rows, err, func(*sql.Rows) (string, error) { return "", errors.New("no scan") })
		equal(t, []any{list, err.Error()}, []any{[]string(nil), "no scan"})
		return nil
	}))
	check(t, s.Write(func(tx *Tx) error {
		_, err := tx.Exec("DROP TABLE row; DROP TABLE inbox; DROP TABLE grp")
		return err
	}))
	failed := func(read func(tx *Tx) error) {
		t.Helper()
		err := s.Write(read)
		if err == nil || !strings.Contains(err.Error(), "no such table") {
			t.Fatalf("got %v; want a table that is not there", err)
		}
	}
	list := func(read func(tx *Tx) ([]json.RawMessage, error)) func(tx *Tx) error {
		return func(tx *Tx) error {
			rows, err := read(tx)
			equal(t, rows, []json.RawMessage(nil))
			return err
		}
	}
	failed(func(tx *Tx) error {
		groups, err := tx.Groups()
		equal(t, groups, []string(nil))
		return err
	})
	failed(func(tx *Tx) error {
		items, err := tx.Queued("g")
		equal(t, items, []Item(nil))
		return err
	})
	failed(list(func(tx *Tx) ([]json.RawMessage, error) { return tx.StateRows("g") }))
	failed(list(func(tx *Tx) ([]json.RawMessage, error) { return tx.Pending("g") }))
	failed(list(func(tx *Tx) ([]json.RawMessage, error) { return tx.Thread("g", nil, 0, 1) }))
	failed(func(tx *Tx) error {
		_, err := tx.Row("g", "group", "g")
		return err
	})
	failed(func(tx *Tx) error {
		_, _, err := tx.Counters("g")
		return err
	})
	failed(func(tx *Tx) error { return tx.RaiseNum("g", 1) })
	failed(func(tx *Tx) error { return tx.Retire("g", 1) })
}
