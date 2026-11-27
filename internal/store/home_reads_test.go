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
// back, and an item jumps and is retired by its number in its own group only.
func TestHomeReadsJumpsAndRetiresTheQueue(t *testing.T) {
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

	jump := func(group string, id int64) { check(t, s.Write(func(tx *Tx) error { return tx.Jump(group, id) })) }
	jump("b", 1)
	jump("g", 2)
	jump("g", 9)
	equal(t, queued("g"), []Item{held, plain})
	equal(t, string(queued("b")[0].Env), `{"p":"item 2","k":"home","s":"sig"}`)
	jump("g", 1)
	held.Env, held.NotBefore = json.RawMessage(`{"p":"item 1","k":"home","s":"sig","jump":true}`), 0
	equal(t, queued("g"), []Item{held, plain})

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
	failed(list(func(tx *Tx) ([]json.RawMessage, error) { return tx.TypedNotes("g", "preset") }))
	failed(func(tx *Tx) error {
		_, _, err := tx.Counters("g")
		return err
	})
	failed(func(tx *Tx) error { return tx.RaiseNum("g", 1) })
	failed(func(tx *Tx) error { return tx.Retire("g", 1) })
	failed(func(tx *Tx) error { return tx.Jump("g", 1) })
}

func TestTypedNotesAndUniqueIndex(t *testing.T) {
	s := openDB(t, ":memory:")
	calm := proto.NoteRow{RowHeader: header("note", "p1", 1), Pos: 1, Group: "g", NoteType: "preset", Name: "Calm", Text: "v1"}
	put(t, s, "g", 10, calm)
	airy := proto.NoteRow{RowHeader: header("note", "p2", 2), Pos: 2, Group: "g", NoteType: "preset", Name: "Airy", Text: "v1"}
	put(t, s, "g", 10, airy)
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "k1", 3), Pos: 3, Group: "g", NoteType: "kb", Name: "Calm", Text: "kb doc"})
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "c1", 4), Pos: 4, Group: "g", Text: "context 1"})
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "c2", 5), Pos: 5, Group: "g", Text: "context 2"})
	put(t, s, "other", 10, proto.NoteRow{RowHeader: header("note", "op1", 1), Pos: 1, Group: "other", NoteType: "preset", Name: "Calm", Text: "other"})

	// A second live note of the same (g, type, name) is refused by row_named.
	dupErr := s.Write(func(tx *Tx) error {
		return Put(tx, "g", 20, proto.NoteRow{RowHeader: header("note", "p-dup", 6), Pos: 6, Group: "g", NoteType: "preset", Name: "Calm", Text: "dup"})
	})
	if dupErr == nil || !strings.Contains(dupErr.Error(), "UNIQUE constraint failed") {
		t.Fatalf("duplicate named note err = %v; want UNIQUE constraint", dupErr)
	}

	typed := func(group string) []proto.NoteRow {
		rows, err := within(t, s, func(tx *Tx) ([]json.RawMessage, error) { return tx.TypedNotes(group, "preset") })
		check(t, err)
		notes := []proto.NoteRow{}
		for _, raw := range rows {
			var n proto.NoteRow
			check(t, json.Unmarshal(raw, &n))
			notes = append(notes, n)
		}
		return notes
	}
	equal(t, typed("g"), []proto.NoteRow{airy, calm})

	// Replacing the existing row id updates text while keeping id and pos; gone frees the name.
	calm.Seq, calm.Text = 7, "v2"
	put(t, s, "g", 30, calm)
	put(t, s, "g", 40, proto.GoneRow{RowHeader: header("note", "p1", 8), Gone: true})
	equal(t, typed("g"), []proto.NoteRow{airy})

	put(t, s, "g", 50, proto.NoteRow{RowHeader: header("note", "p3", 9), Pos: 9, Group: "g", NoteType: "preset", Name: "Calm", Text: "recreated"})
	equal(t, typed("none"), []proto.NoteRow{})

	for _, q := range []string{
		`EXPLAIN QUERY PLAN SELECT data FROM row WHERE g='g' AND r='note' AND gone=0 AND data->>'type' IS NOT NULL AND data->>'type'='preset' ORDER BY data->>'name'`,
	} {
		var detail string
		check(t, s.db.QueryRow(q).Scan(new(int), new(int), new(int), &detail))
		if !strings.Contains(detail, "row_named") {
			t.Fatalf("plan %q does not use row_named", detail)
		}
	}
}

// The entries an import adds as device rows: those of the group's people that
// the group has no live device row for.
func TestLackingEntries(t *testing.T) {
	s := openDB(t, ":memory:")
	person, left, ai := member("m1", 1), member("m2", 2), member("m3", 3)
	person.User, left.User, ai.User, ai.Kind = "u1", "u2", "u1", "ai"
	for _, row := range []proto.HumanMemberRow{person, left, ai} {
		put(t, s, "g", 10, row)
	}
	put(t, s, "g", 10, proto.GoneRow{RowHeader: header("member", "m2", 4), Gone: true})
	put(t, s, "g", 10, proto.DeviceRow{RowHeader: header("device", "u1:1", 5)})
	put(t, s, "g", 10, proto.GoneRow{RowHeader: header("device", "u1:3", 6), Gone: true})
	put(t, s, "h", 10, proto.DeviceRow{RowHeader: header("device", "u1:2", 1)})
	check(t, s.Write(func(tx *Tx) error {
		_, err := tx.Exec(`INSERT INTO device_log VALUES ('u1',1,'1'),('u1',3,'3'),('u1',2,'2'),('u2',1,'4'),('u3',1,'5')`)
		return err
	}))
	envs, err := within(t, s, func(tx *Tx) ([]json.RawMessage, error) { return tx.LackingEntries("g") })
	check(t, err)
	equal(t, envs, []json.RawMessage{json.RawMessage("2"), json.RawMessage("3")})
}

// The open invitation codes of the groups, in the setting invites
// (storage.md section 2; people.md 2.6): an entry whose end is reached counts
// as none wherever it is read, and the next write deletes it. The zero
// Invite ends a group's code.
func TestHomeKeepsTheInvitationCodes(t *testing.T) {
	s := openDB(t, ":memory:")
	codes := func(now int64) map[string]Invite {
		got, err := within(t, s, func(tx *Tx) (map[string]Invite, error) { return tx.Invites(now) })
		check(t, err)
		return got
	}
	set := func(group string, code Invite, now int64) {
		check(t, s.Write(func(tx *Tx) error { return tx.SetInvite(group, code, now) }))
	}
	stored := func() string { return scalar[string](t, s.db, "SELECT value FROM setting WHERE key='invites'") }
	a, b := Invite{"7K2M9QXD4F8TWB3N", "m1", 200}, Invite{"0123456789ABCDEF", "m2", 100}
	equal(t, codes(0), map[string]Invite{})
	set("a", a, 0)
	set("b", b, 0)
	equal(t, stored(), `{"a":{"code":"7K2M9QXD4F8TWB3N","by":"m1","expires":200},"b":{"code":"0123456789ABCDEF","by":"m2","expires":100}}`)
	equal(t, codes(99), map[string]Invite{"a": a, "b": b})
	equal(t, codes(100), map[string]Invite{"a": a})
	equal(t, stored(), `{"a":{"code":"7K2M9QXD4F8TWB3N","by":"m1","expires":200},"b":{"code":"0123456789ABCDEF","by":"m2","expires":100}}`)
	renewed := Invite{"ZZZZZZZZZZZZZZZZ", "m1", 300}
	set("a", renewed, 100)
	equal(t, stored(), `{"a":{"code":"ZZZZZZZZZZZZZZZZ","by":"m1","expires":300}}`)
	set("a", Invite{}, 100)
	equal(t, []any{stored(), codes(0)}, []any{"{}", map[string]Invite{}})

	_, err := s.db.Exec("UPDATE setting SET value='{' WHERE key='invites'")
	check(t, err)
	_, err = within(t, s, func(tx *Tx) (map[string]Invite, error) { return tx.Invites(0) })
	equal(t, err.Error(), "unexpected end of JSON input")
	_, err = s.db.Exec("DROP TABLE setting")
	check(t, err)
	_, err = within(t, s, func(tx *Tx) (map[string]Invite, error) { return tx.Invites(0) })
	equal(t, strings.Contains(err.Error(), "no such table: setting"), true)
	err = s.Write(func(tx *Tx) error { return tx.SetInvite("a", a, 0) })
	equal(t, strings.Contains(err.Error(), "no such table: setting"), true)
}
