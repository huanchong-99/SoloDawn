//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// A home pushes to a peer connection as it is (planning/architecture.md
// section 8.3): home cannot import peer to say so itself.
var _ home.Client = (*peer.Connection)(nil)

func init() {
	stepSets["home"] = func(sc *godog.ScenarioContext, _ fixtures) { homeSteps(sc) }
}

// homeWorld is a group home over a store in memory: the state of one group,
// and a command table of its own, because the scenarios are about what holds
// for every command and not about one of them.
type homeWorld struct {
	now     time.Time
	keys    map[string]identity.Key // by who owns the device in a step
	db      *store.Store
	state   *home.State
	table   home.Table
	command proto.Envelope // the command of the scenario, as its signer made it
	answers []any          // the frame answered to each request, in order
}

func homeSteps(sc *godog.ScenarioContext) {
	w := &homeWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a group home with its owner as the only member$`, func() error { return w.start(cleanup) })
	sc.Step(`^(the owner's browser|a device that no person owns|a device of a person who is no member) sends a command that writes a note$`, func(who string) error {
		w.command = w.note(w.keys[who])
		return w.send(w.keys[who])
	})
	sc.Step(`^the owner's browser sends the same command again$`, func() error { return w.send(w.keys["the owner's browser"]) })
	sc.Step(`^a command that the owner's browser signed arrives on the connection of the owner's laptop$`, func() error {
		w.command = w.note(w.keys["the owner's browser"])
		return w.send(w.keys["the owner's laptop"])
	})
	sc.Step(`^the home has written (\d+) rows?$`, w.written)
	sc.Step(`^both answers are the same success$`, w.sameSuccess)
	sc.Step(`^the answer is the error "([^"]*)"$`, w.refusedWith)
	sc.Step(`^the same command on the browser's own connection is still answered with success$`, func() error {
		if err := w.send(w.keys["the owner's browser"]); err != nil {
			return err
		}
		if _, ok := w.answers[len(w.answers)-1].(proto.CmdResult); !ok {
			return fmt.Errorf("answered %+v", w.answers[len(w.answers)-1])
		}
		return w.written(1)
	})
}

// start makes the home of a group whose owner has a browser, the node that
// is the home, and a laptop; another person, who is no member, has one
// device, and one device is in nobody's log.
func (w *homeWorld) start(cleanup func(func())) error {
	browser, node, laptop, guest := exampleKey("browser"), exampleKey("node"), exampleKey("laptop"), exampleKey("guest")
	w.keys = map[string]identity.Key{
		"the owner's browser": browser, "the owner's laptop": laptop,
		"a device of a person who is no member": guest, "a device that no person owns": exampleKey("stranger"),
	}
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db, w.answers = db, nil

	group, owner := string(exampleID("group")), string(exampleID("owner"))
	w.state = home.NewState(group, node.ID(), browser.ID())
	for _, devices := range [][]identity.Key{{browser, node, laptop}, {guest}} {
		log, err := homeLog(devices)
		if err != nil {
			return err
		}
		w.state.Logs[log.User] = log
	}
	// No unit has built group create yet: the group and its owner are put
	// into the state as that command will leave them.
	w.state.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: node.ID()}
	w.state.People[owner] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: group, Kind: "human", User: browser.ID()}, Role: "owner"}
	w.table = home.Table{"note": homeNote}
	return nil
}

// homeLog is the device log of the person whose first device is devices[0].
func homeLog(devices []identity.Key) (*identity.Log, error) {
	var entries []proto.Envelope
	for i, dev := range devices {
		entry, err := logEntry(devices[0], devices[0], i+1, "add", dev)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return deviceLog(devices[0], entries...)
}

// homeNote is the command of these scenarios: it writes one note row.
func homeNote(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Text string `json:"text"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	note := proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: string(c.Nonce)}, Group: s.ID, Text: p.Text, By: c.Who.Member(), Timestamp: c.Now}
	return home.Result{Rows: []home.Row{home.RowOf(note)}}, nil
}

// note is a command that writes a note, signed by a device.
func (w *homeWorld) note(by identity.Key) proto.Envelope {
	payload := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"text":"Path events include the connection id."}`,
		exampleID("note"), w.now.UnixMilli(), w.state.ID)
	return by.Sign(payload)
}

// send hands the scenario's command to the home as a request on the
// connection of a device, in one transaction of the store, as the run loop
// of a home does: the answer, then the rows with the sequence numbers the
// store gives out.
func (w *homeWorld) send(connection identity.Key) error {
	var res home.Result
	err := w.db.Write(func(tx *store.Tx) error {
		var err error
		// The command of these scenarios reads nothing from disk.
		if res, err = w.table.Apply(w.state, nil, tx, connection.ID(), w.command, w.now); err != nil {
			return err
		}
		for _, row := range res.Rows {
			seq, err := tx.NextSeq(w.state.ID)
			if err != nil {
				return err
			}
			if numbered := row.Value().(proto.NoteRow).Seq; numbered != seq {
				return fmt.Errorf("the row has seq %d and the store gives out %d", numbered, seq)
			}
			if err := row.Put(tx, w.state.ID, w.now.UnixMilli()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	w.state.Commit(res)
	w.answers = append(w.answers, res.Frame(int64(len(w.answers)+1)))
	return nil
}

func (w *homeWorld) written(want int) error {
	view, err := w.db.Snapshot(w.state.ID)
	if err != nil {
		return err
	}
	defer view.Close()
	rows, err := view.AfterSeqUpTo(0, view.Seq)
	if err != nil {
		return err
	}
	if len(rows) != want || view.Seq != int64(want) || w.state.Seq != int64(want) {
		return fmt.Errorf("%d rows, the store at seq %d, the state at seq %d: %s", len(rows), view.Seq, w.state.Seq, rows)
	}
	return nil
}

// sameSuccess compares two answers apart from the request each one answers.
func (w *homeWorld) sameSuccess() error {
	first, ok1 := w.answers[0].(proto.CmdResult)
	second, ok2 := w.answers[1].(proto.CmdResult)
	if !ok1 || !ok2 || first.Re != 1 || second.Re != 2 {
		return fmt.Errorf("answered %+v and %+v", w.answers[0], w.answers[1])
	}
	second.Re = first.Re
	if !reflect.DeepEqual(first, second) {
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(second)
		return fmt.Errorf("answered %s, then %s", a, b)
	}
	return nil
}

func (w *homeWorld) refusedWith(code string) error {
	last := w.answers[len(w.answers)-1]
	if refusal, ok := last.(proto.ErrorResponse); !ok || refusal.Err.Code != code || refusal.Err.Message == "" {
		return fmt.Errorf("answered %+v", last)
	}
	return nil
}
