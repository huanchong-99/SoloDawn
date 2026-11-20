package home

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// What the clients of a group are told once a result is committed
// (planning/contracts/datachannel.md sections 3, 4 and 6): the rows first,
// then the list of groups when it changed, then the inbox items the result
// queued. A row change so reaches a host before the items it causes.

// runRoom is the most bytes the rows of one rows frame may have: a ctl
// message of 61,440 bytes less the frame around them
// (planning/contracts/conventions.md section 4).
const runRoom = 61_376

// runNever is the due time of a group that holds no item back.
const runNever = math.MaxInt64

// runStateRows are the row types a state_only reader gets.
var runStateRows = map[string]bool{"group": true, "member": true, "device": true, "task": true}

// tell pushes a committed result.
func (g *runGroup) tell(res Result, now time.Time) {
	rows := map[bool][]any{false: g.rowFrames(res.Rows, false), true: g.rowFrames(res.Rows, true)}
	for c, seat := range g.seats {
		if !seat.watching {
			continue
		}
		for _, frame := range rows[seat.stateOnly] {
			c.send(frame)
		}
	}
	g.publish()
	if res.GroupsChanged {
		g.loop.announce()
	}
	if len(res.Items) > 0 {
		g.deliver(now)
	}
}

// rowFrames makes the rows frames of a result, each filled only as far as a
// ctl message goes.
func (g *runGroup) rowFrames(rows []Row, stateOnly bool) []any {
	var data []json.RawMessage
	for _, r := range rows {
		if !stateOnly || runStateRows[runHeader(r).Type] {
			raw, _ := runJSON(r.Value()) // it encoded when it was written
			data = append(data, raw)
		}
	}
	frames := []any{}
	for _, batch := range runBatches(data, runRoom) {
		frames = append(frames, proto.Rows{Type: "rows", Group: g.state.ID, Rows: batch})
	}
	return frames
}

// runBatches cuts a list into the fewest lists in order whose values, with a
// comma between two, take at most room bytes each. No value is above room.
func runBatches(list []json.RawMessage, room int) [][]json.RawMessage {
	var batches [][]json.RawMessage
	var batch []json.RawMessage
	left := room
	for _, v := range list {
		if len(v) > left {
			batches, batch, left = append(batches, batch), nil, room
		}
		batch = append(batch, v)
		left -= len(v) + 1
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

// runJSON encodes as the store does, without HTML escaping: a row is pushed
// as it is stored.
func runJSON(v any) (json.RawMessage, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	err := e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), err
}

// runView is what a group shows of itself outside its goroutine: its entry
// in the list of groups, and to which connected devices the group is open.
type runView struct {
	summary proto.GroupSummary
	serves  map[string]bool
}

func (g *runGroup) view() runView {
	v := runView{summary: proto.GroupSummary{Group: g.state.ID, Name: g.state.Group.Name, Seq: g.state.Seq}, serves: map[string]bool{}}
	for c := range g.seats {
		v.serves[c.device] = g.state.Serves(c.device)
	}
	return v
}

// publish replaces the view of a group that exists.
func (g *runGroup) publish() {
	if g.state.Group.ID == "" {
		return
	}
	v := g.view()
	g.loop.mu.Lock()
	defer g.loop.mu.Unlock()
	g.loop.views[g.state.ID] = v
}

// announce pushes every connection its list of groups again. The lists are
// queued under the registry lock, so that of two announcements the later
// list is also the later frame.
func (l *Loop) announce() {
	var full []*Conn
	l.mu.Lock()
	for c := range l.conns {
		if !c.frames.push(proto.Groups{Type: "groups", Groups: l.listed(c.device)}) {
			full = append(full, c)
		}
	}
	l.mu.Unlock()
	for _, c := range full {
		c.fail()
	}
}

// QueuedItem is an inbox item as a home sends it while items are not signed
// yet: the seam's Item beside the number that ack names. The inbox unit
// replaces it with the signed item of planning/contracts/datachannel.md
// section 6.
type QueuedItem struct {
	ID int64
	Item
}

// ItemsPush is the frame that carries one QueuedItem to its host.
type ItemsPush struct {
	Group string
	Items []QueuedItem
}

// runSeal makes what the store keeps as the envelope of an item, in the
// transaction that queues it under the number id. Until the inbox unit signs
// items, that is the item itself. The inbox unit replaces this function and
// runItemFrame, and nothing else of the loop.
func runSeal(it Item, _ int64) (json.RawMessage, error) {
	return runJSON(it)
}

// runItemFrame makes the frame that carries a queued item to its host, in a
// transaction that d reads: the rows an item names are copied when it is
// sent.
func runItemFrame(_ Disk, it store.Item) (any, error) {
	var item Item
	err := json.Unmarshal(it.Env, &item)
	return ItemsPush{Group: it.Group, Items: []QueuedItem{{ID: it.ID, Item: item}}}, err
}

// inbox makes a connection the taker of its device's items and sends it
// every one that is due, whether it was sent before or not.
func (g *runGroup) inbox(c *Conn) {
	seat := g.seats[c]
	if seat == nil {
		return
	}
	seat.inbox, seat.sent = true, nil
	g.deliver(g.loop.cfg.Now())
}

// runLot is what one delivery gives one connection: the frames to send, and
// the items it has then been sent.
type runLot struct {
	frames []any
	sent   map[int64]bool
}

// deliver sends the queued items that are due to the connections that take
// them, each item once per connection, and notes when the next held item is
// due. It reads the queue from the store every time, so an item that was
// acknowledged or retired is not sent, and is forgotten here. Without a
// taker nothing is read: the connection that becomes one asks again. A
// delivery that fails is reported and tried again on the next tick.
func (g *runGroup) deliver(now time.Time) {
	g.next = runNever
	if !g.taken() {
		return
	}
	lots, next := map[*Conn]runLot{}, int64(runNever)
	err := g.loop.cfg.Store.Write(func(tx *store.Tx) error {
		d := &runDisk{tx: tx, group: g.state.ID}
		var due []store.Item
		due, next = runDue(d.Queued(), now.UnixMilli())
		for c, seat := range g.seats {
			lot, err := runLotOf(d, seat, c.device, due)
			lots[c], d.err = lot, errors.Join(d.err, err)
		}
		return d.err
	})
	if err != nil {
		g.next = now.UnixMilli()
		g.loop.cfg.Report(fmt.Errorf("group %s, its inbox: %w", g.state.ID, err))
		return
	}
	g.next = next
	for c, lot := range lots {
		g.seats[c].sent = lot.sent
		for _, frame := range lot.frames {
			c.send(frame)
		}
	}
}

// taken reports whether a connection takes inbox items.
func (g *runGroup) taken() bool {
	for _, seat := range g.seats {
		if seat.inbox {
			return true
		}
	}
	return false
}

// runDue splits the queue at the home's time: the items that are due, oldest
// first, and when the first of the others is.
func runDue(queued []store.Item, now int64) (due []store.Item, next int64) {
	next = runNever
	for _, it := range queued {
		if it.NotBefore > now {
			next = min(next, it.NotBefore)
			continue
		}
		due = append(due, it)
	}
	return due, next
}

// runLotOf picks from the due items those of one connection's device, and
// makes the frames of the ones it was not sent yet.
func runLotOf(d Disk, seat *runSeat, device string, due []store.Item) (lot runLot, err error) {
	lot.sent = map[int64]bool{}
	for _, it := range due {
		if !seat.inbox || it.Host != device {
			continue
		}
		lot.sent[it.ID] = true
		if seat.sent[it.ID] {
			continue
		}
		frame, ferr := runItemFrame(d, it)
		lot.frames, err = append(lot.frames, frame), errors.Join(err, ferr)
	}
	return lot, err
}
