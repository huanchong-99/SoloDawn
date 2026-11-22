package home

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// runGroup is one group as its goroutine holds it. Everything in it is read
// and written by that goroutine only, and by the store's writer while the
// goroutine waits for a transaction it started.
type runGroup struct {
	loop  *Loop
	state *State
	jobs  *runQueue[func()]
	seats map[*Conn]*runSeat // the connections of the node
	next  int64              // when the first held inbox item is due, runNever when none is held
}

// runSeat is what a group knows of one connection.
type runSeat struct {
	watching  bool           // it is pushed the rows of every result
	stateOnly bool           // of those, only the state rows
	inbox     bool           // it takes the inbox items of its device
	sent      map[int64]bool // the queued items it was sent already
}

// run does the jobs of the group, one after the other.
func (g *runGroup) run() {
	defer g.loop.wg.Done()
	for {
		job, ok := g.jobs.pop()
		if !ok {
			return
		}
		job()
		if g.loop.retire(g) {
			return
		}
	}
}

// seat gives a connection its place in the group and counts its device as
// online.
func (g *runGroup) seat(c *Conn) {
	g.seats[c] = &runSeat{inbox: c.taker}
	g.state.Online[c.device] = true
}

func (g *runGroup) attach(c *Conn) {
	g.seat(c)
	g.publish()
}

// detach forgets a connection. Its device stays online while another
// connection of the same device is attached.
func (g *runGroup) detach(c *Conn) {
	delete(g.seats, c)
	delete(g.state.Online, c.device)
	for other := range g.seats {
		g.state.Online[other.device] = true
	}
	g.publish()
}

// watch subscribes a seated connection to the rows of the group.
func (g *runGroup) watch(c *Conn, stateOnly bool) {
	seat := g.seats[c]
	seat.watching, seat.stateOnly = true, stateOnly
}

// runReader checks that a group exists and serves device on its home surface.
func runReader(s *State, device string) *Refusal {
	if s.Group.ID == "" {
		return &Refusal{"not_found", "this node is not home to the group"}
	}
	if !s.Serves(device) {
		return &Refusal{"forbidden", "the connection's device may not read this group"}
	}
	return nil
}

func runErr(re int64, code, msg string) proto.ErrorResponse {
	return proto.ErrorResponse{Re: re, Err: proto.WireError{Code: code, Message: msg}}
}

// command applies one cmd in its transaction and answers it.
func (g *runGroup) command(c *Conn, re int64, env proto.Envelope) {
	g.seamCommand(c, re, env)
}

// tick runs the timers in the order of their names, then sends the inbox
// items whose hold has ended.
func (g *runGroup) tick(now time.Time) {
	timers := g.loop.cfg.Tables.Timers
	for _, name := range slices.Sorted(maps.Keys(timers)) {
		g.rule(now, func(d Disk, ms int64) Result { return timers[name](g.state, d, ms) })
	}
	if now.UnixMilli() >= g.next {
		g.deliver(now)
	}
}

// rule runs a timer or a handler in a transaction of its own. A group that
// does not exist has no rules. A result that Seal refuses is reported and
// changes nothing.
func (g *runGroup) rule(now time.Time, f func(d Disk, now int64) Result) {
	if g.state.Group.ID == "" {
		return
	}
	res, err := g.transact(now, func(d Disk, _ *store.Tx) (Result, error) {
		res := g.state.Seal(f(d, now.UnixMilli()))
		return res, runRefusal(res)
	})
	if err != nil {
		g.loop.cfg.Report(fmt.Errorf("group %s, a rule: %w", g.state.ID, err))
		return
	}
	g.settle(res, now, func() {})
}

// runRefusal is the error of a result that no client is there to be told.
func runRefusal(res Result) error {
	if res.Answer.Code == "" {
		return nil
	}
	return errors.New(res.Answer.Code + ": " + res.Msg)
}

// transact runs one result through one transaction of the store: run makes
// it, reading through a Disk of that transaction, and its rows, items,
// retirements and other writes are written. An error leaves nothing written.
// It runs on the store's writer while the goroutine of the group waits.
func (g *runGroup) transact(now time.Time, run func(d Disk, tx *store.Tx) (Result, error)) (res Result, err error) {
	err = g.loop.cfg.Store.Write(func(tx *store.Tx) error {
		d := &runDisk{tx: tx, group: g.state.ID}
		var err error
		if res, err = run(d, tx); err != nil {
			return err
		}
		res.Items = inboxPace(g.state, res.Items, now.UnixMilli())
		return errors.Join(d.err, g.write(tx, res, now.UnixMilli()))
	})
	return res, err
}

// settle follows a transaction that was committed. A result that holds
// nothing (a refusal, a repeated command, a rule with nothing to do) only
// gets its answer. Any other is taken into the state, counted for the
// wake-up cap and given to every hook, in the order of their names; then
// comes the answer, and then what the clients are told.
func (g *runGroup) settle(res Result, now time.Time, answer func()) {
	if runIdle(res) {
		answer()
		return
	}
	g.state.Commit(res)
	g.count(res.Items, now.UnixMilli())
	hooks := g.loop.cfg.Tables.After
	for _, name := range slices.Sorted(maps.Keys(hooks)) {
		hooks[name](g.state, res)
	}
	answer()
	g.tell(res, now)
}

// runIdle reports whether a result changes nothing.
func runIdle(res Result) bool {
	return len(res.Rows) == 0 && len(res.Items) == 0 && len(res.Retire) == 0 && len(res.Writes) == 0 && !res.GroupsChanged
}

// runHour is how far back the count of the wake-up cap goes, in milliseconds.
const runHour = 3600_000

// count keeps State.Wakes: each non-merge item counts once for its AI, at the
// time it is due, which is now, or its NotBefore when that is later. What is an
// hour old or older is dropped from the AI's list as the next item is counted.
func (g *runGroup) count(items []Item, now int64) {
	for _, it := range items {
		inboxWake(g.state.Wakes, it, 0, now)
	}
}

// runRecent returns the times in list after now - runHour, in ascending order.
func runRecent(list []int64, now int64) []int64 {
	recent := slices.DeleteFunc(slices.Clone(list), func(at int64) bool { return at <= now-runHour })
	slices.Sort(recent)
	return recent
}

// write writes a sealed result through: its rows under the sequence numbers
// the store gives out, the task counter, its items, its retirements and its
// other writes.
func (g *runGroup) write(tx *store.Tx, res Result, now int64) error {
	return runSteps(
		func() error { return g.putRows(tx, res.Rows, now) },
		func() error { return tx.RaiseNum(g.state.ID, runNum(res.Rows)) },
		func() error { return g.putItems(tx, res, now) },
		func() error { return runRetire(tx, g.state.ID, res.Retire) },
		func() error { return runPut(tx, res.Writes) },
	)
}

// runSteps runs the steps until one fails.
func runSteps(steps ...func() error) error {
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// putRows writes the rows. The store's counter and the state's move
// together: a row that Seal numbered otherwise than the store does shows
// that they do not, and nothing is written.
func (g *runGroup) putRows(tx *store.Tx, rows []Row, now int64) error {
	for _, r := range rows {
		seq, err := tx.NextSeq(g.state.ID)
		if err != nil {
			return err
		}
		if own := runHeader(r).Seq; own != seq {
			return fmt.Errorf("a row is numbered %d where the store gives out %d", own, seq)
		}
		if err := r.Put(tx, g.state.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// runHeader reads r, id and seq of a row.
func runHeader(r Row) proto.RowHeader {
	return reflect.ValueOf(r.Value()).FieldByName("RowHeader").Interface().(proto.RowHeader)
}

// runNum is the highest task number among rows, 0 without a task row.
func runNum(rows []Row) (num int) {
	for _, r := range rows {
		if task, ok := r.Value().(proto.TaskRow); ok {
			num = max(num, task.Num)
		}
	}
	return num
}

// putItems queues the items of a result, each for the host of its AI.
func (g *runGroup) putItems(tx *store.Tx, res Result, now int64) error {
	for _, it := range res.Items {
		host := g.host(res.Rows, it.AI)
		if host == "" {
			return fmt.Errorf("an item %q is for %q, which is no AI of the group", it.Why, it.AI)
		}
		stored := store.Item{Group: g.state.ID, AI: it.AI, Host: host, Why: it.Why, Msg: runText(it.Msg), Task: runText(it.Task),
			Commit: runText(it.Commit), NotBefore: it.NotBefore, Created: now}
		seal := func(id int64) (json.RawMessage, error) { return inboxSeal(it, id) }
		if _, err := tx.Enqueue(stored, seal); err != nil {
			return err
		}
	}
	return nil
}

// host returns the host of an AI member as the result leaves it: a member
// row of the result counts before the state.
func (g *runGroup) host(rows []Row, ai string) string {
	host := g.state.AIs[ai].Host
	for _, r := range rows {
		if m, ok := r.Value().(proto.AIMemberRow); ok && m.ID == ai {
			host = m.Host
		}
	}
	return host
}

// runText is a column that is NULL for a field an item does not have.
func runText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func runRetire(tx *store.Tx, group string, ids []int64) error {
	for _, id := range ids {
		if err := tx.Retire(group, id); err != nil {
			return err
		}
	}
	return nil
}

func runPut(tx *store.Tx, writes []Write) error {
	for _, w := range writes {
		if err := w.Put(tx); err != nil {
			return err
		}
	}
	return nil
}
