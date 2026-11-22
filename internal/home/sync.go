package home

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The sync request of a group's home surface (planning/contracts/datachannel.md
// section 4; planning/flows/sync.md, A, C and E): take a read view of the
// store at the group's sequence number on the goroutine of the group, send a
// delta or a reset from that view followed by synced, and subscribe the
// connection to every change after that sequence number.

const (
	synMaxDelta  = 1000
	synMainLimit = 50
)

// Sync answers req on c from a snapshot of its group and subscribes c to
// every change after the snapshot's sequence number, only the state rows with
// req.StateOnly (planning/contracts/datachannel.md section 4). Sync waits for
// the group, as Attach does: it must not be called on the goroutine of a group
// or inside a transaction of the store.
func (l *Loop) Sync(c *Conn, req proto.Sync) {
	l.askHome(c, req.ID, req.Group, func(g *runGroup) { synRun(g, c, req) })
}

func synRun(g *runGroup, c *Conn, req proto.Sync) {
	if g.seats[c] == nil {
		return
	}
	if refusal := runReader(g.state, c.device); refusal != nil {
		c.send(runErr(req.ID, refusal.Code, refusal.Msg))
		return
	}
	seq, reset, rows, err := synCut(g.loop.cfg.Store, g.state.ID, req)
	if err != nil {
		g.loop.cfg.Report(errors.New("group " + g.state.ID + ", a sync of " + c.device + ": " + err.Error()))
		c.send(runErr(req.ID, "internal", "the home could not read the group; the cause is in its log"))
		return
	}
	synSend(c, req.ID, g.state.ID, seq, reset, rows)
	g.watch(c, req.StateOnly)
}

func synCut(db *store.Store, group string, req proto.Sync) (seq int64, reset bool, rows []json.RawMessage, err error) {
	snap, err := db.Snapshot(group)
	if err != nil {
		return 0, false, nil, err
	}
	defer snap.Close()
	reset, rows, err = synRead(snap, req)
	return snap.Seq, reset, rows, err
}

func synRead(snap *store.Snapshot, req proto.Sync) (reset bool, rows []json.RawMessage, err error) {
	if synInRange(req.Since, snap) {
		count, err := snap.DeltaCount(req.Since, snap.Seq)
		if err != nil {
			return false, nil, err
		}
		if count <= synMaxDelta {
			rows, err := synDelta(snap, req.Since, req.StateOnly)
			return false, rows, err
		}
	}
	rows, err = synCold(snap, req.StateOnly)
	return true, rows, err
}

func synInRange(since int64, snap *store.Snapshot) bool {
	return since > 0 && since <= snap.Seq && since >= snap.Purged
}

func synDelta(snap *store.Snapshot, since int64, stateOnly bool) ([]json.RawMessage, error) {
	rows, err := snap.AfterSeqUpTo(since, snap.Seq)
	if !stateOnly {
		return rows, err
	}
	var kept []json.RawMessage
	for _, raw := range rows {
		var h proto.RowHeader
		json.Unmarshal(raw, &h)
		if runStateRows[h.Type] {
			kept = append(kept, raw)
		}
	}
	return kept, err
}

func synCold(snap *store.Snapshot, stateOnly bool) ([]json.RawMessage, error) {
	state, err := snap.StateRows()
	if stateOnly {
		return state, err
	}
	pending, errPending := snap.Pending()
	main, errMain := snap.PageThread(nil, 0, synMainLimit)
	return synMerge(state, pending, main), errors.Join(err, errPending, errMain)
}

// synMerge orders the rows of a cold read by seq without duplicates. Within a
// group every row has a unique seq (row_by_seq).
func synMerge(lists ...[]json.RawMessage) []json.RawMessage {
	bySeq := map[int64]json.RawMessage{}
	for _, raw := range slices.Concat(lists...) {
		var h proto.RowHeader
		json.Unmarshal(raw, &h)
		bySeq[h.Seq] = raw
	}
	rows := make([]json.RawMessage, 0, len(bySeq))
	for _, seq := range slices.Sorted(maps.Keys(bySeq)) {
		rows = append(rows, bySeq[seq])
	}
	return rows
}

func synSend(c *Conn, re int64, group string, seq int64, reset bool, rows []json.RawMessage) {
	if reset {
		c.send(proto.Reset{Type: "reset", Group: group})
	}
	for _, batch := range runBatches(rows, runRoom) {
		c.send(proto.Rows{Type: "rows", Group: group, Rows: batch})
	}
	c.send(proto.Synced{Type: "synced", Success: proto.Success{Re: re, OK: true}, Group: group, Seq: seq})
}
