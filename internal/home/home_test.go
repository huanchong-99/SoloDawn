package home_test

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The home's clock in these tests, in Unix milliseconds.
const clock = 1763600000000

var now = time.UnixMilli(clock)

// The devices of the tests. The browser is the first device of the owner, so
// its id is the owner's user id; the guest is another person.
var (
	browser = device(1)
	node    = device(2) // the home
	laptop  = device(3) // another node of the owner, the host of the AI
	guest   = device(4)
	nobody  = device(5) // in no log
)

func device(seed byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)))
}

// id is a random id that follows from a seed.
func id(seed byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 16))
}

// The ids of the tests: the group, its owner's member row and its AI.
var groupID, ownerID, aiID = id(9), id(10), id(11)

func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

// logOf is the device log of the person whose first device is first.
func logOf(t *testing.T, first identity.Key, more ...identity.Key) *identity.Log {
	t.Helper()
	l := &identity.Log{User: first.ID()}
	for i, dev := range append([]identity.Key{first}, more...) {
		p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":"node","name":"a device"}`, first.ID(), i+1, dev.ID())
		if _, err := l.Apply(first.Sign(p)); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

// empty is the state of a group that does not exist, on the home of the
// tests: it keeps the logs of the owner and of the guest.
func empty(t *testing.T) *home.State {
	t.Helper()
	s := home.NewState(groupID, node.ID(), browser.ID())
	s.Logs[browser.ID()] = logOf(t, browser, node, laptop)
	s.Logs[guest.ID()] = logOf(t, guest)
	return s
}

func person(id string, user identity.Key, role string) proto.HumanMemberRow {
	return proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, User: user.ID()}, Role: role}
}

func ai(id string, host identity.Key) proto.AIMemberRow {
	return proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, User: browser.ID()}, Host: host.ID()}
}

// group is the state of a group with its owner and one AI on the laptop,
// which is the secretary, after ten rows.
func group(t *testing.T) *home.State {
	t.Helper()
	s := empty(t)
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: groupID}, Secretary: aiID}
	s.People[ownerID] = person(ownerID, browser, "owner")
	s.AIs[aiID] = ai(aiID, laptop)
	s.Seq = 10
	return s
}

// signed is a command with the common fields of a payload, followed by more.
func signed(by identity.Key, t string, n byte, more string) proto.Envelope {
	return by.Sign(fmt.Sprintf(`{"t":%q,"n":%q,"ts":%d,"g":%q%s}`, t, id(n), clock, groupID, more))
}

// receipts is the fake of home.Receipts: outcomes in memory, by signer and nonce.
type receipts map[[2]string]receipt

type receipt struct {
	ts      int64
	outcome store.Outcome
}

func (r receipts) Receipt(signer, nonce string) (store.Outcome, error) {
	held, ok := r[[2]string{signer, nonce}]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return held.outcome, nil
}

func (r receipts) PutReceipt(signer, nonce string, ts int64, result store.Outcome) error {
	r[[2]string{signer, nonce}] = receipt{ts, result}
	return nil
}

// broken is a store that fails: its reads with read, its writes with write.
type broken struct{ read, write error }

func (b broken) Receipt(string, string) (store.Outcome, error) { return store.Outcome{}, b.read }

func (b broken) PutReceipt(string, string, int64, store.Outcome) error { return b.write }

// disk is the fake of home.Disk: the messages of a test, by id.
type disk map[string]proto.MsgRow

func (d disk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d[id]; return m, ok }
func (d disk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d disk) TypedNotes(string) []proto.NoteRow         { return nil }
func (d disk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d disk) Pending() []proto.MsgRow                   { return nil }
func (d disk) Queued() []store.Item                      { return nil }
func (d disk) OtherMember(string) bool                   { return false }

// setting is a fake of home.Write: a row of the setting table.
type setting struct{ key, value string }

func (w setting) Put(tx *store.Tx) error {
	_, err := tx.Exec("INSERT INTO setting(key, value) VALUES(?, ?)", w.key, w.value)
	return err
}

func noteRow(id string, seq int64, text, by string) proto.NoteRow {
	return proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: id, Seq: seq}, Pos: seq, Group: groupID, Text: text, By: by, Timestamp: clock}
}

// writeNote is the command of these tests: it writes one note and answers
// with a number, and refuses a note without text. calls counts its runs.
func writeNote(calls *int) home.Func {
	return func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		*calls++
		var p struct {
			Text string `json:"text"`
		}
		if refusal := c.Decode(&p); refusal != nil {
			return home.Result{}, refusal
		}
		if p.Text == "" {
			return home.Result{}, &home.Refusal{Code: "conflict", Msg: "a note has a text"}
		}
		row := noteRow(string(c.Nonce), 0, p.Text, c.Who.Member())
		row.Timestamp = c.Now
		return home.Result{Rows: []home.Row{home.RowOf(row)}, Answer: store.Outcome{Num: 7}}, nil
	}
}

// apply sends a command in a transaction with nothing on disk.
func apply(t *testing.T, table home.Table, s *home.State, r home.Receipts, env proto.Envelope) home.Result {
	t.Helper()
	res, err := table.Apply(s, disk(nil), r, env, now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// A2.3: a command sent twice produces one row and the same answer.
func TestApplyAnswersARepeatedCommandFromItsReceipt(t *testing.T) {
	s, held, calls := group(t), receipts{}, 0
	table := home.Table{"note": writeNote(&calls)}
	env := signed(browser, "note", 1, `,"text":"remember this"`)

	first := apply(t, table, s, held, env)
	equal(t, first.Answer, store.Outcome{OK: true, Num: 7})
	equal(t, len(first.Rows), 1)
	equal(t, first.Rows[0].Value(), any(noteRow(id(1), 11, "remember this", ownerID)))
	equal(t, held[[2]string{browser.ID(), id(1)}], receipt{clock, first.Answer})
	s.Commit(first)
	equal(t, s.Seq, int64(11))

	second := apply(t, table, s, held, env)
	equal(t, second, home.Result{Answer: first.Answer})
	equal(t, second.Frame(4), first.Frame(4))
	equal(t, calls, 1)
	s.Commit(second)
	equal(t, s.Seq, int64(11))
}

// The envelope and group checks: what is refused here leaves no receipt.
func TestApplyRefusesBeforeTheReceipt(t *testing.T) {
	payload := func(ts int64, rest string) string {
		return fmt.Sprintf(`{"t":"note","n":%q,"ts":%d%s,"text":"x"}`, id(1), ts, rest)
	}
	changed := signed(browser, "note", 1, `,"text":"x"`)
	changed.Payload += " "
	for name, c := range map[string]struct {
		env  proto.Envelope
		code string
	}{
		"a payload changed on the way": {changed, "bad_sig"},
		"a device in no log":           {signed(nobody, "note", 1, `,"text":"x"`), "unknown_device"},
		"a payload without a nonce":    {browser.Sign(`{"t":"note","ts":1}`), "bad_request"},
		"for another node":             {signed(browser, "note", 1, `,"text":"x","to":"`+laptop.ID()+`"`), "not_for_me"},
		"signed 25 hours ago":          {browser.Sign(payload(clock-25*3600*1000, `,"g":"`+groupID+`"`)), "stale"},
		"for another group":            {browser.Sign(payload(clock, `,"g":"`+id(8)+`"`)), "bad_request"},
		"without a group":              {browser.Sign(payload(clock, ``)), "bad_request"},
	} {
		t.Run(name, func(t *testing.T) {
			s, held, calls := group(t), receipts{}, 0
			res := apply(t, home.Table{"note": writeNote(&calls)}, s, held, c.env)
			equal(t, res.Answer, store.Outcome{Code: c.code})
			equal(t, res.Msg != "", true)
			equal(t, len(res.Rows)+len(held)+calls, 0)
		})
	}
}

// What the table, the signer or the function refuses is the receipt, and the
// answer to the same command again.
func TestApplyKeepsARefusalAsTheReceipt(t *testing.T) {
	for name, c := range map[string]struct {
		by    identity.Key
		t     string
		more  string
		code  string
		calls int
	}{
		"a type the table does not have":        {browser, "nope", ``, "bad_request", 0},
		"a kind the table does not have":        {browser, "post", `,"kind":"poem"`, "bad_request", 0},
		"a kind that is no text":                {browser, "post", `,"kind":5`, "bad_request", 0},
		"a post without a kind":                 {browser, "post", ``, "bad_request", 0},
		"a key in place of a type":              {browser, "post/text", `,"kind":"brief"`, "bad_request", 0},
		"a kind that holds a key":               {browser, "post", `,"kind":"text/more"`, "bad_request", 0},
		"an as that is no text":                 {laptop, "note", `,"text":"x","as":5`, "bad_request", 0},
		"an as that is null":                    {laptop, "note", `,"text":"x","as":null`, "bad_request", 0},
		"an empty as":                           {laptop, "note", `,"text":"x","as":""`, "bad_request", 0},
		"an as for an AI on another host":       {node, "note", `,"text":"x","as":"` + aiID + `"`, "forbidden", 0},
		"an as that names no AI":                {laptop, "note", `,"text":"x","as":"` + ownerID + `"`, "forbidden", 0},
		"a person who is no member":             {guest, "note", `,"text":"x"`, "forbidden", 0},
		"a payload its function cannot read":    {browser, "note", `,"text":5`, "bad_request", 1},
		"a command its function does not allow": {browser, "note", `,"text":""`, "conflict", 1},
	} {
		t.Run(name, func(t *testing.T) {
			s, held, calls := group(t), receipts{}, 0
			// The keys "post/" and "post/text/more" are in the table to show
			// that no payload reaches them.
			table := home.Table{"note": writeNote(&calls), "post/text": writeNote(&calls), "post/": writeNote(&calls), "post/text/more": writeNote(&calls)}
			env := signed(c.by, c.t, 1, c.more)

			first := apply(t, table, s, held, env)
			equal(t, first.Answer, store.Outcome{Code: c.code})
			equal(t, first.Msg != "", true)
			equal(t, len(first.Rows), 0)
			equal(t, held[[2]string{c.by.ID(), id(1)}], receipt{clock, first.Answer})

			second := apply(t, table, s, held, env)
			equal(t, second, home.Result{Answer: first.Answer, Msg: "the command was refused when it first arrived"})
			equal(t, calls, c.calls)
		})
	}
}

// The key of a command is its t, and for five types a field beside it.
func TestApplyFindsTheFunctionByItsKey(t *testing.T) {
	for key, c := range map[string]struct{ t, more string }{
		"group/create": {"group", `,"op":"create","kind":"project","action":"x"`},
		"member/set":   {"member", `,"op":"set","kind":"x"`},
		"task/start":   {"task", `,"op":"start"`},
		"post/text":    {"post", `,"kind":"text","op":"x"`},
		"act/confirm":  {"act", `,"action":"confirm","kind":"x"`},
		"note":         {"note", `,"op":"x","kind":5,"action":null`},
	} {
		t.Run(key, func(t *testing.T) {
			calls := 0
			table := home.Table{key: func(*home.State, home.Command) (home.Result, *home.Refusal) {
				calls++
				return home.Result{}, nil
			}}
			res := apply(t, table, group(t), receipts{}, signed(browser, c.t, 1, c.more))
			equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
			equal(t, calls, 1)
		})
	}
}

func TestApplyNumbersTheRowsOfAResult(t *testing.T) {
	s := group(t)
	onDisk := disk{id(20): {RowHeader: proto.RowHeader{Type: "msg", ID: id(20), Seq: 5}, Pos: 3, Kind: "decision"}}
	table := home.Table{"act/choose": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		card, _ := c.Disk.Msg(id(20))
		return home.Result{
			Rows: []home.Row{
				home.RowOf(proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Kind: "system"}),
				home.RowOf(card),
				home.RowOf(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(21)}, Num: 4}),
			},
			Items:         []home.Item{{AI: aiID, Why: "decided", Msg: card.ID, NotBefore: c.Now + 60000}},
			Retire:        []int64{17},
			Writes:        []home.Write{setting{"hm0", "1"}},
			GroupsChanged: true,
		}, nil
	}}
	env := signed(browser, "act", 1, `,"action":"choose"`)

	res, err := table.Apply(s, onDisk, receipts{}, env, now)
	equal(t, err, nil)
	created, decided := res.Rows[0].Value().(proto.MsgRow), res.Rows[1].Value().(proto.MsgRow)
	equal(t, []int64{created.Seq, created.Pos}, []int64{11, 11})
	equal(t, []int64{decided.Seq, decided.Pos}, []int64{12, 3})
	equal(t, res.Rows[2].Value().(proto.TaskRow).Seq, int64(13))
	equal(t, res.Items, []home.Item{{AI: aiID, Why: "decided", Msg: id(20), NotBefore: clock + 60000}})
	equal(t, res.Retire, []int64{17})
	equal(t, res.Writes, []home.Write{setting{"hm0", "1"}})
	equal(t, res.GroupsChanged, true)
	equal(t, s.Seq, int64(10))

	s.Commit(res)
	equal(t, s.Seq, int64(13))
	equal(t, s.Tasks[id(21)].Seq, int64(13))
	equal(t, s.Num, 4)
}

// A row is at most 16 KiB, and a card 12 KiB when it is created
// (planning/contracts/conventions.md section 4). The rows are filled with
// "<", which is one byte of a row and six when JSON is escaped for HTML.
func TestApplyRefusesARowAboveItsLimit(t *testing.T) {
	for name, c := range map[string]struct {
		row   proto.MsgRow // what the command writes, without its text
		limit int
	}{
		"a message that is no card": {proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(1)}, Kind: "text"}, 16384},
		"a card that is created":    {proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(1)}, Kind: "decision", State: &proto.CardState{Status: "pending"}}, 12288},
		"a card that is decided":    {proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(1), Seq: 5}, Pos: 3, Kind: "decision", State: &proto.CardState{Status: "decided"}}, 16384},
	} {
		t.Run(name, func(t *testing.T) {
			write := func(text string) home.Result {
				row := c.row
				row.Text = text
				table := home.Table{"post/text": func(*home.State, home.Command) (home.Result, *home.Refusal) {
					return home.Result{Rows: []home.Row{home.RowOf(noteRow(id(30), 0, "", ownerID)), home.RowOf(row)}}, nil
				}}
				return apply(t, table, group(t), receipts{}, signed(browser, "post", 1, `,"kind":"text"`))
			}
			// The size of the row a result holds: its JSON, with "<" as it is.
			size := func(res home.Result) int {
				b, err := json.Marshal(res.Rows[1].Value())
				equal(t, err, nil)
				return len(bytes.ReplaceAll(b, []byte(`\u003c`), []byte("<")))
			}
			fill := strings.Repeat("<", c.limit-size(write("")))

			res := write(fill)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, size(res), c.limit)

			res = write(fill + "<")
			equal(t, res, home.Result{Answer: store.Outcome{Code: "bad_request"},
				Msg: fmt.Sprintf("the command would write a row of %d bytes, above its limit of %d", c.limit+1, c.limit)})
		})
	}
}

func TestApplyFailsWhenItsReceiptCannotBeReadOrWritten(t *testing.T) {
	failure := errors.New("the disk is gone")
	calls := 0
	table := home.Table{"note": writeNote(&calls)}
	env := signed(browser, "note", 1, `,"text":"x"`)

	res, err := table.Apply(group(t), disk(nil), broken{read: failure}, env, now)
	equal(t, err, failure)
	equal(t, res, home.Result{})
	equal(t, calls, 0)

	_, err = table.Apply(group(t), disk(nil), broken{read: sql.ErrNoRows, write: failure}, env, now)
	equal(t, err, failure)
}

// The one command for a group that does not exist is group create, by a
// device of this node's owner.
func TestApplyCreatesAGroup(t *testing.T) {
	var founder home.Who
	create := func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		founder = c.Who
		return home.Result{Rows: []home.Row{
			home.RowOf(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Home: s.Home}),
			home.RowOf(person(ownerID, browser, "owner")),
		}}, nil
	}
	calls := 0
	table := home.Table{"group/create": create, "note": writeNote(&calls)}
	s, held := empty(t), receipts{}

	res := apply(t, table, s, held, signed(browser, "note", 1, `,"text":"x"`))
	equal(t, res.Answer, store.Outcome{Code: "not_found"})
	res = apply(t, table, s, held, signed(guest, "group", 2, `,"op":"create","kind":"project"`))
	equal(t, res.Answer, store.Outcome{Code: "forbidden"})
	// The key of group create is no type: only t and op together reach it.
	res = apply(t, table, s, held, signed(browser, "group/create", 6, ``))
	equal(t, res.Answer, store.Outcome{Code: "bad_request"})
	equal(t, founder, home.Who{})
	equal(t, calls, 0)

	res = apply(t, table, s, held, signed(browser, "group", 3, `,"op":"create","kind":"project"`))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, founder, home.Who{Device: browser.ID(), User: browser.ID()})
	s.Commit(res)
	equal(t, s.Group, proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: groupID, Seq: 1}, Home: node.ID()})
	equal(t, s.People[ownerID].Seq, int64(2))
	equal(t, s.Seq, int64(2))

	res = apply(t, table, s, held, signed(browser, "note", 4, `,"text":"x"`))
	equal(t, res.Answer, store.Outcome{OK: true, Num: 7})
	// A second group create reaches its function, which answers it.
	res = apply(t, table, s, held, signed(browser, "group", 5, `,"op":"create","kind":"project"`))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, founder.Human, ownerID)
}

// A row goes through to the store as it was numbered, and a *store.Tx holds
// the receipts.
func TestApplyInATransactionOfTheStore(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, calls := group(t), 0
	s.Seq = 0
	note := writeNote(&calls)
	table := home.Table{"note": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		res, refusal := note(s, c)
		res.Writes = []home.Write{setting{"hm0", "kept"}}
		return res, refusal
	}}
	env := signed(browser, "note", 1, `,"text":"remember this"`)
	send := func() (res home.Result) {
		err := db.Write(func(tx *store.Tx) error {
			var err error
			if res, err = table.Apply(s, disk(nil), tx, env, now); err != nil {
				return err
			}
			for _, w := range res.Writes {
				if err := w.Put(tx); err != nil {
					return err
				}
			}
			for _, r := range res.Rows {
				seq, err := tx.NextSeq(s.ID)
				if err != nil || seq != r.Value().(proto.NoteRow).Seq {
					return fmt.Errorf("the store gives the row %d: %v", seq, err)
				}
				if err := r.Put(tx, s.ID, clock); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		s.Commit(res)
		return res
	}
	first, second := send(), send()
	equal(t, first.Answer, store.Outcome{OK: true, Num: 7})
	equal(t, second.Frame(1), first.Frame(1))
	equal(t, calls, 1)

	view, err := db.Snapshot(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := view.AfterSeqUpTo(0, view.Seq)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(noteRow(id(1), 1, "remember this", ownerID))
	equal(t, rows, []json.RawMessage{want})
	equal(t, view.Close(), nil)

	// The write that is no row went into the same transaction, once.
	var kept []string
	err = db.Write(func(tx *store.Tx) error {
		found, err := tx.Query("SELECT value FROM setting WHERE key='hm0'")
		if err != nil {
			return err
		}
		defer found.Close()
		for found.Next() {
			var value string
			if err := found.Scan(&value); err != nil {
				return err
			}
			kept = append(kept, value)
		}
		return found.Err()
	})
	equal(t, err, nil)
	equal(t, kept, []string{"kept"})
}

func TestCommitHoldsTheStateRows(t *testing.T) {
	s := group(t)
	second := id(13)
	s.Commit(home.Result{Rows: []home.Row{
		home.RowOf(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: groupID}, Name: "Website"}),
		home.RowOf(person(id(12), guest, "member")),
		home.RowOf(ai(second, node)),
		home.RowOf(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(21)}, Num: 6}),
		home.RowOf(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(22)}, Num: 5}),
		home.RowOf(proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(23)}}),
		home.RowOf(deviceRow(browser, browser, 4, "add", device(8))),
	}})
	equal(t, s.Group.Name, "Website")
	equal(t, s.People[id(12)].User, guest.ID())
	equal(t, s.AIs[second].Host, node.ID())
	equal(t, []int{len(s.People), len(s.AIs), len(s.Tasks)}, []int{2, 2, 2})
	equal(t, s.Num, 6)
	equal(t, s.Seq, int64(17))

	// The marker of a note removes no member, whatever its id.
	s.Commit(home.Result{Rows: []home.Row{
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: ownerID}, Gone: true}),
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: aiID}, Gone: true}),
	}})
	equal(t, []int{len(s.People), len(s.AIs)}, []int{2, 2})

	s.Commit(home.Result{Rows: []home.Row{
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: id(12)}, Gone: true}),
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: aiID}, Gone: true}),
	}})
	equal(t, []int{len(s.People), len(s.AIs)}, []int{1, 1})
	equal(t, s.People[ownerID].Role, "owner")
	equal(t, s.AIs[second].Host, node.ID())
	equal(t, s.Seq, int64(21))
}

func TestAddGivesAKeyOnce(t *testing.T) {
	calls := 0
	table := home.Table{}
	table.Add("note", writeNote(&calls))
	apply(t, table, group(t), receipts{}, signed(browser, "note", 1, `,"text":"x"`))
	equal(t, calls, 1)
	defer func() { equal(t, recover(), any("home: two functions under note")) }()
	table.Add("note", writeNote(&calls))
}

// A timer and a handler return a result as a command does. Whoever runs one
// seals its result, which numbers and measures the rows as Apply does.
func TestSealFinishesTheResultOfATimerOrAnEvent(t *testing.T) {
	onDisk := disk{id(20): {RowHeader: proto.RowHeader{Type: "msg", ID: id(20), Seq: 5}, Pos: 3, Kind: "decision", State: &proto.CardState{Status: "pending"}}}
	timers, events := home.Rules[home.Timer]{}, home.Rules[home.Handler]{}
	timers.Add("expire", func(s *home.State, d home.Disk, now int64) home.Result {
		card, _ := d.Msg(id(20))
		card.State = &proto.CardState{Status: "expired", At: &now}
		return home.Result{Rows: []home.Row{home.RowOf(card)}}
	})
	events.Add("closed", func(s *home.State, _ home.Disk, now int64, event any) home.Result {
		member := s.AIs[aiID]
		if member.Host != event.(string) {
			return home.Result{}
		}
		member.State, member.Since = "offline", now
		system := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(31)}, Kind: "system", Timestamp: now}
		return home.Result{Rows: []home.Row{home.RowOf(member), home.RowOf(system)}}
	})

	s := group(t)
	res := s.Seal(timers["expire"](s, onDisk, clock))
	equal(t, res.Answer, store.Outcome{OK: true})
	expired := res.Rows[0].Value().(proto.MsgRow)
	equal(t, []int64{expired.Seq, expired.Pos, *expired.State.At}, []int64{11, 3, clock})
	s.Commit(res)

	res = s.Seal(events["closed"](s, onDisk, clock, node.ID()))
	equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
	res = s.Seal(events["closed"](s, onDisk, clock, laptop.ID()))
	equal(t, res.Rows[0].Value().(proto.AIMemberRow).Seq, int64(12))
	created := res.Rows[1].Value().(proto.MsgRow)
	equal(t, []int64{created.Seq, created.Pos}, []int64{13, 13})
	s.Commit(res)
	equal(t, []any{s.AIs[aiID].State, s.AIs[aiID].Since, s.Seq}, []any{"offline", int64(clock), int64(13)})

	// A result with a row above its limit is refused whole.
	big := home.RowOf(noteRow(id(32), 0, strings.Repeat("x", 16384), ownerID))
	res = s.Seal(home.Result{Rows: []home.Row{home.RowOf(noteRow(id(33), 0, "", ownerID)), big}, Items: []home.Item{{AI: aiID, Why: "notice"}}})
	equal(t, res.Answer, store.Outcome{Code: "bad_request"})
	equal(t, []int{len(res.Rows), len(res.Items)}, []int{0, 0})

	defer func() { equal(t, recover(), any("home: two functions under closed")) }()
	events.Add("closed", nil)
}

// deviceRow is the row of a device-log entry: signer adds dev to, or revokes
// it from, the log of the person whose first device is user.
func deviceRow(signer, user identity.Key, ver int, op string, dev identity.Key) proto.DeviceRow {
	change := proto.DeviceChange{User: user.ID(), Version: ver, Op: op, Device: dev.ID()}
	if op == "add" {
		change.Kind, change.Name = "browser", "a device"
	}
	entry, _ := json.Marshal(proto.DeviceEntry{PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: proto.ID(id(40)), Timestamp: clock}, DeviceChange: change})
	return proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: fmt.Sprint(user.ID(), ":", ver)}, DeviceChange: change, Env: signer.Sign(string(entry))}
}

// The state follows the device logs through the device rows it commits: a
// device is known from the row that adds it, and unknown from the one that
// revokes it.
func TestCommitAppliesTheEntryOfADeviceRow(t *testing.T) {
	s, calls := group(t), 0
	table := home.Table{"note": writeNote(&calls)}
	tablet, stranger := device(8), device(9)
	send := func(by identity.Key, n byte) store.Outcome {
		return apply(t, table, s, receipts{}, signed(by, "note", n, `,"text":"x"`)).Answer
	}
	commit := func(rows ...proto.DeviceRow) {
		res := home.Result{}
		for _, row := range rows {
			res.Rows = append(res.Rows, home.RowOf(row))
		}
		s.Commit(s.Seal(res))
	}
	equal(t, send(tablet, 1), store.Outcome{Code: "unknown_device"})
	equal(t, s.Serves(tablet.ID()), false)

	add := deviceRow(browser, browser, 4, "add", tablet)
	commit(add)
	equal(t, send(tablet, 1), store.Outcome{OK: true, Num: 7})
	equal(t, s.Serves(tablet.ID()), true)
	equal(t, len(s.Logs[browser.ID()].Entries), 4)

	// The same entry in a row again changes nothing.
	commit(add)
	equal(t, len(s.Logs[browser.ID()].Entries), 4)

	commit(deviceRow(tablet, browser, 5, "revoke", laptop))
	equal(t, send(laptop, 2), store.Outcome{Code: "unknown_device"})
	equal(t, send(tablet, 3), store.Outcome{OK: true, Num: 7})

	// The first row of a person starts that person's log: the device is
	// known from then on, and is still no member's.
	equal(t, send(stranger, 4), store.Outcome{Code: "unknown_device"})
	commit(deviceRow(stranger, stranger, 1, "add", stranger))
	equal(t, s.Logs[stranger.ID()].Valid(stranger.ID()), true)
	equal(t, send(stranger, 4), store.Outcome{Code: "forbidden"})
	equal(t, s.Seq, int64(14))

	// An entry that its log refuses is skipped: it makes no device known,
	// leaves the log as it was, and starts no log.
	late, other := device(12), device(13)
	commit(deviceRow(browser, browser, 9, "add", late), deviceRow(other, other, 2, "add", other), deviceRow(nobody, browser, 6, "add", late))
	equal(t, len(s.Logs[browser.ID()].Entries), 5)
	equal(t, []bool{s.Serves(late.ID()), s.Logs[other.ID()] == nil, len(s.Logs) == 3}, []bool{false, true, true})
	equal(t, send(late, 5), store.Outcome{Code: "unknown_device"})
	equal(t, send(tablet, 6), store.Outcome{OK: true, Num: 7})
	equal(t, s.Seq, int64(17))
}

// A hook runs after a result is committed, and sees the state that holds it.
func TestAHookGetsACommittedResult(t *testing.T) {
	var seen []int64
	var away []string
	after := home.Rules[home.Hook]{}
	after.Add("push", func(s *home.State, res home.Result) {
		seen = append(seen, s.Seq, int64(len(res.Rows)))
		for _, dev := range []identity.Key{browser, laptop} {
			if !s.Online[dev.ID()] {
				away = append(away, dev.ID())
			}
		}
	})
	s, calls := group(t), 0
	s.Online[browser.ID()] = true
	res := apply(t, home.Table{"note": writeNote(&calls)}, s, receipts{}, signed(browser, "note", 1, `,"text":"x"`))
	s.Commit(res)
	for _, hook := range after {
		hook(s, res)
	}
	equal(t, seen, []int64{11, 1})
	equal(t, away, []string{laptop.ID()})
	defer func() { equal(t, recover(), any("home: two functions under push")) }()
	after.Add("push", nil)
}

// The tables of a node are there for the files of the package to add to, and
// a new state has the place for the count of the wake-up cap.
func TestTheTablesOfANodeAndTheWakesOfAState(t *testing.T) {
	equal(t, []bool{home.Commands != nil, home.Timers != nil, home.Events != nil, home.After != nil}, []bool{true, true, true, true})
	s := home.NewState(groupID, node.ID(), browser.ID())
	s.Wakes[aiID] = append(s.Wakes[aiID], clock)
	equal(t, s.Wakes, map[string][]int64{aiID: {clock}})
	equal(t, s.Online, map[string]bool{})
}

// The answers are the frames of the wire examples.
func TestFrameIsTheAnswerOnTheWire(t *testing.T) {
	failed := false
	for name, res := range map[string]home.Result{
		"cmd-ok":       {Answer: store.Outcome{OK: true}},
		"cmd-started":  {Answer: store.Outcome{OK: true, Num: 12}},
		"cmd-reviewed": {Answer: store.Outcome{OK: true, Passed: &failed, Round: 1}},
		"cmd-error":    {Answer: store.Outcome{Code: "conflict"}, Msg: "This card has already been decided."},
	} {
		t.Run(name, func(t *testing.T) {
			example, err := os.ReadFile(filepath.Join("..", "proto", "testdata", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var want, got map[string]any
			equal(t, json.Unmarshal(example, &want), nil)
			frame, err := json.Marshal(res.Frame(int64(want["re"].(float64))))
			equal(t, err, nil)
			equal(t, json.Unmarshal(frame, &got), nil)
			equal(t, got, want)
			equal(t, proto.UnexpectedNulls(frame), []string{})
		})
	}
}
