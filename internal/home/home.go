// Package home is the home role of a node: the rules of the groups it is
// home to (planning/architecture.md section 2.1). This file is its seam: a
// group's state in memory, the table of commands, and what the function of a
// command takes and returns. check.go holds who may send a command.
package home

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Client is a connected client of a home, a browser or a host, as the home
// sees it: something to answer and to push to until it is gone
// (planning/architecture.md section 8.3). A peer.Connection has these
// methods, and so has the local link of a host on the same node. The device
// a client was authenticated for comes beside it, from whoever accepted the
// connection.
type Client interface {
	Send(frame any) error
	Done() <-chan struct{}
}

// State is one group as its rules read it: the state rows, which a home
// holds in memory (planning/contracts/storage.md section 1). What is on
// disk only is read through a Disk. The function of a command reads a State
// and changes nothing in it; Commit takes the rows of a result.
type State struct {
	ID    string                   // the group, also before it exists
	Home  string                   // this node's device id
	Owner string                   // the user id of this node's owner
	Logs  map[string]*identity.Log // by user id: the owner's device log and that of every person in the group

	Group  proto.GroupRow                  // its ID is empty until the group is created
	People map[string]proto.HumanMemberRow // by member id
	AIs    map[string]proto.AIMemberRow    // by member id
	Tasks  map[string]proto.TaskRow        // by task id
	Seq    int64                           // the last sequence number given out
	Num    int                             // the last task number given out

	// Wakes holds, by AI member id, when each inbox item of the last hour was
	// sent to that AI, in Unix milliseconds: the count of the wake-up cap,
	// which is kept in memory and starts again with the home
	// (planning/contracts/commands.md section 1.3). Whoever sends the items
	// keeps it.
	Wakes map[string][]int64
}

// NewState returns the state of a group that does not exist yet, on the home
// with that device id and owner. Whoever makes it fills in Logs.
func NewState(group, home, owner string) *State {
	return &State{ID: group, Home: home, Owner: owner, Logs: map[string]*identity.Log{},
		People: map[string]proto.HumanMemberRow{}, AIs: map[string]proto.AIMemberRow{}, Tasks: map[string]proto.TaskRow{},
		Wakes: map[string][]int64{}}
}

// Disk is what the rules read of a group beyond its state rows: messages,
// notes and queued inbox items, which a home keeps on disk only. A Disk reads
// inside one transaction of the store and is given to one call: Apply for a
// command, or the timer or event that the transaction is for. No method
// returns an error: whoever implements Disk makes a read that failed fail
// that call.
type Disk interface {
	// Msg returns a message. ok is false when the id names none, or one that is gone.
	Msg(id string) (msg proto.MsgRow, ok bool)
	// Note is Msg for a note.
	Note(id string) (note proto.NoteRow, ok bool)
	// Thread returns a page of the thread of a task, or of the main line for
	// nil: the messages below pos before, newest first, and the newest ones
	// for before 0 (planning/contracts/datachannel.md section 4).
	Thread(task *string, before int64, limit int) []proto.MsgRow
	// Pending returns the cards whose status is pending, oldest first.
	Pending() []proto.MsgRow
	// Queued returns the inbox items that no host has acknowledged, oldest first.
	Queued() []store.Item
}

// Receipts holds the outcome of every command a home has answered
// (planning/contracts/commands.md, "Once only"). A *store.Tx is one: the
// transaction of the command. Receipt returns sql.ErrNoRows for a command
// that was not answered before.
type Receipts interface {
	Receipt(signer, nonce string) (store.Outcome, error)
	PutReceipt(signer, nonce string, ts int64, result store.Outcome) error
}

// wireRow is every row a home writes (planning/contracts/rows.md).
type wireRow interface {
	proto.GroupRow | proto.HumanMemberRow | proto.AIMemberRow | proto.DeviceRow |
		proto.TaskRow | proto.MsgRow | proto.NoteRow | proto.GoneRow
}

// Row is a changed row of a result. Apply gives it its sequence number: a
// function leaves seq as it is, and leaves pos at 0 in a message or a note
// that it creates.
type Row interface {
	// Value returns the row, a value of its type in internal/proto.
	Value() any
	// Put writes the row through, in the transaction of its command.
	Put(tx *store.Tx, group string, modified int64) error
	number(seq int64)
}

// RowOf makes the Row of a wire row.
func RowOf[R wireRow](v R) Row { return &typedRow[R]{v} }

type typedRow[R wireRow] struct{ v R }

func (r *typedRow[R]) Value() any { return r.v }

func (r *typedRow[R]) Put(tx *store.Tx, group string, modified int64) error {
	return store.Put(tx, group, modified, r.v)
}

// number sets seq, and pos where the row has one that is 0. No row has pos
// 0 (planning/contracts/datachannel.md section 4), so such a row is being
// created, and its pos is its first seq.
func (r *typedRow[R]) number(seq int64) {
	v := reflect.ValueOf(&r.v).Elem()
	v.FieldByName("Seq").SetInt(seq)
	if pos := v.FieldByName("Pos"); pos.IsValid() && pos.Int() == 0 {
		pos.SetInt(seq)
	}
}

// The most bytes a row may have, and a card when it is created, so that it
// can still take its decision (planning/contracts/conventions.md section 4).
const (
	rowLimit  = 16 << 10
	cardLimit = 12 << 10
)

// rowFits refuses a row that is above its limit. The size of a row is that
// of its JSON without HTML escaping, as a browser counts it.
func rowFits(row any) *Refusal {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.Encode(row)                            // a row that does not encode is not written either: Put fails on it
	size, limit := b.Len()-1, sizeLimit(row) // Encode ends the row with a line break
	if size > limit {
		return &Refusal{"bad_request", fmt.Sprintf("the command would write a row of %d bytes, above its limit of %d", size, limit)}
	}
	return nil
}

// sizeLimit is the limit of a numbered row. A message with a state is a
// card, and one whose pos is its seq is being created.
func sizeLimit(row any) int {
	if m, ok := row.(proto.MsgRow); ok && m.State != nil && m.Pos == m.Seq {
		return cardLimit
	}
	return rowLimit
}

// Item is an inbox item that a command queues for an AI, before the home
// signs it: the fields that planning/contracts/datachannel.md section 6
// gives its why. A field the why does not have stays at its zero value.
type Item struct {
	AI     string // the member it is for; for merge, the assignee whose host merges
	Why    string
	Task   string
	Msg    string
	Commit string          // review, merge
	PR     int             // merge, when the group has a remote
	Jump   bool            // mention, after the author's jump
	Code   string          // notice
	Detail string          // notice, where its case has one
	Cause  *proto.Envelope // the signed act that led to the item

	// NotBefore holds the item back until that time, in Unix milliseconds:
	// the wake-up cap. 0 sends it at once.
	NotBefore int64
}

// Write is a change that is no row, such as the push subscription of a
// browser. The function that returns it stays pure: the caller of Apply
// calls Put in the transaction of the command, after the rows.
type Write interface {
	Put(tx *store.Tx) error
}

// Result is what a command, a timer or an event does, and the answer to a
// command.
type Result struct {
	Rows   []Row         // the changed rows, in the order of their sequence numbers
	Items  []Item        // the inbox items to queue, after the rows
	Retire []int64       // the queued inbox items that the change made pointless, by id
	Writes []Write       // what changes besides rows and items
	Answer store.Outcome // a function sets the result fields of its command and nothing else
	Msg    string        // of a refusal: the sentence for logs

	// GroupsChanged says that the list of groups a peer may read has changed:
	// once the result is committed, every connected peer is pushed its
	// groups again (planning/contracts/datachannel.md section 3).
	GroupsChanged bool
}

// Frame returns the answer to the request re that carried the command
// (planning/contracts/datachannel.md section 5).
func (r Result) Frame(re int64) any {
	if r.Answer.Code != "" {
		return proto.ErrorResponse{Re: re, Err: proto.WireError{Code: r.Answer.Code, Message: r.Msg}}
	}
	return proto.CmdResult{Success: proto.Success{Re: re, OK: true}, Num: r.Answer.Num, Passed: r.Answer.Passed, Round: r.Answer.Round}
}

// Refusal is the error answer to a command: a code of
// planning/contracts/datachannel.md section 9 and a sentence for logs.
type Refusal struct{ Code, Msg string }

func refused(r *Refusal) Result {
	return Result{Answer: store.Outcome{Code: r.Code}, Msg: r.Msg}
}

// repeated answers a command again from its receipt. The receipt of a
// refusal holds its code and not its sentence.
func repeated(held store.Outcome) Result {
	res := Result{Answer: held}
	if held.Code != "" {
		res.Msg = "the command was refused when it first arrived"
	}
	return res
}

// Command is a command whose envelope and signer passed the checks of check.go.
type Command struct {
	proto.PayloadHeader                // t, n, ts and g of the payload
	Env                 proto.Envelope // as it was signed: the env of a row it writes, the act a card keeps
	Who                 Who
	Now                 int64 // the home's clock, in Unix milliseconds
	Disk                Disk  // the reads of this command's transaction
}

// Decode reads the payload into v, the wire type of the command.
func (c Command) Decode(v any) *Refusal {
	if err := proto.Decode([]byte(c.Env.Payload), v); err != nil {
		return &Refusal{"bad_request", err.Error()}
	}
	return nil
}

// Func is the function of one command: from the state of its group and the
// command to what it does, or to a refusal. It is pure: it reads s and c,
// calls nothing but c.Disk, and changes neither.
type Func func(s *State, c Command) (Result, *Refusal)

// A Timer is a rule that the clock sets off, and a Handler one that an event
// outside the commands does: a device-log entry that was accepted, a host
// connection that closed. Both are pure as a Func is, and return what
// changes. event is the value that the unit of that kind of event defines.
type (
	Timer   func(s *State, d Disk, now int64) Result
	Handler func(s *State, d Disk, now int64, event any) Result
)

// Table holds the function of every command under its key: the t of the
// payload, followed for group, member and task by "/" and its op, for post by
// its kind and for act by its action. "post/text", "task/start", "note".
type Table map[string]Func

// Rules holds the functions that no command selects, each under a name its
// file chooses.
type Rules[F any] map[string]F

// The tables of a node. Each file adds its functions from an init of its
// own. A command is found by its key. Whoever runs the home runs every timer
// for every group as its clock goes, and the handler of an event by the name
// of what happened.
var (
	Commands = Table{}
	Timers   = Rules[Timer]{}
	Events   = Rules[Handler]{}
)

// Add puts a function under its key. A key is given once.
func (t Table) Add(key string, f Func) { addOnce(t, key, f) }

// Add puts a function under its name. A name is given once.
func (r Rules[F]) Add(name string, f F) { addOnce(r, name, f) }

func addOnce[F any](table map[string]F, name string, f F) {
	if _, taken := table[name]; taken {
		panic("home: two functions under " + name)
	}
	table[name] = f
}

// Apply answers the cmd that arrived, on the connection authenticated for
// device, at the home's time now. It runs inside the transaction of the
// command, which disk reads and which holds the receipts: the caller writes
// the rows, the items, the retirements and the other writes of the result in
// that transaction, commits it, and then gives the result to s.Commit. An
// error is a receipt that could not be read or written: the caller drops the
// transaction and answers internal.
//
// The checks come in this order, and the first that fails is the answer: the
// connection and the envelope (State.open), which leave no receipt; then the
// receipt, which answers a repeated command as the first time; then the
// table, the group and the signer (Table.find) and the function itself. What
// the last two answer, a refusal included, is the receipt.
func (t Table) Apply(s *State, disk Disk, receipts Receipts, device string, env proto.Envelope, now time.Time) (Result, error) {
	hdr, refusal := s.open(device, env, now)
	if refusal != nil {
		return refused(refusal), nil
	}
	held, err := receipts.Receipt(env.Key, string(hdr.Nonce))
	if err == nil {
		return repeated(held), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Result{}, err
	}
	res := t.run(s, Command{PayloadHeader: hdr, Env: env, Now: now.UnixMilli(), Disk: disk})
	return res, receipts.PutReceipt(env.Key, string(hdr.Nonce), hdr.Timestamp, res.Answer)
}

// run answers a command that has no receipt.
func (t Table) run(s *State, c Command) Result {
	f, refusal := t.find(s, &c)
	if refusal != nil {
		return refused(refusal)
	}
	res, refusal := f(s, c)
	if refusal != nil {
		return refused(refusal)
	}
	return s.Seal(res)
}

// Seal finishes a result before it is written: it numbers the rows, from
// Seq + 1 in their order, and refuses the result whole when a row is above
// its limit. Apply seals the result of a command. The result of a timer or
// of a handler is sealed by whoever ran it, and is from there on written,
// committed and given to Commit as a command's is.
func (s *State) Seal(res Result) Result {
	for i, r := range res.Rows {
		r.number(s.Seq + int64(i) + 1)
		if refusal := rowFits(r.Value()); refusal != nil {
			return refused(refusal)
		}
	}
	res.Answer.OK = true
	return res
}

// Commit takes the rows of a result into the state, once the transaction
// that wrote them is committed and before the next command is applied.
func (s *State) Commit(res Result) {
	for _, r := range res.Rows {
		s.hold(r.Value())
	}
	s.Seq += int64(len(res.Rows))
}

// hold keeps a state row; messages, notes and device rows are not held.
func (s *State) hold(row any) {
	switch r := row.(type) {
	case proto.GroupRow:
		s.Group = r
	case proto.HumanMemberRow:
		s.People[r.ID] = r
	case proto.AIMemberRow:
		s.AIs[r.ID] = r
	case proto.TaskRow:
		s.Tasks[r.ID] = r
		s.Num = max(s.Num, r.Num)
	case proto.GoneRow:
		s.drop(r)
	}
}

// drop forgets a member whose row became a marker. The marker of another
// row removes nothing: a note has the id its sender chose, which may be a
// member's.
func (s *State) drop(r proto.GoneRow) {
	if r.Type == "member" {
		delete(s.People, r.ID)
		delete(s.AIs, r.ID)
	}
}
