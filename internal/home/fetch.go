package home

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Fetch is a fetch request on ctl (planning/contracts/datachannel.md section 4):
// a page by kind (Kind), one live row (Row, RowID), or a page by thread or the
// main line (Thread).
type Fetch struct {
	proto.FetchPage
	Thread *string `json:"thread,omitempty"`
	Kind   string  `json:"kind,omitempty"`
	Row    string  `json:"r,omitempty"`
	RowID  string  `json:"row,omitempty"`
}

// Fetch answers req on c (planning/contracts/datachannel.md section 4). On the
// goroutine of the group it checks that the group exists and serves the device
// of c, pins and closes a store.Snapshot, reads and packs the answer, and
// queues the response frame on c.
//
// Fetch waits for the group and for the store's writer, as Sync does: it must
// not be called on the goroutine of a group or inside a transaction of the
// store.
func (l *Loop) Fetch(c *Conn, req Fetch) {
	read := fetPageRead
	if fetIsRow(req) {
		read = fetOneRow
	} else if !fetPageOK(req) {
		c.send(runErr(req.ID, "bad_request", "the fetch limit must be 1 to 100 and before at least 0"))
		return
	}
	step := func(g *runGroup) { g.fetRun(c, req, read) }
	if !l.ask(req.Group, step) {
		c.send(runErr(req.ID, "not_found", "this node is not home to the group"))
	}
}

func fetIsRow(req Fetch) bool {
	return req.Kind == "" && (req.Row != "" || req.RowID != "")
}

func fetPageOK(req Fetch) bool {
	return 1 <= req.Limit && req.Limit <= 100 && req.Before >= 0
}

func (g *runGroup) fetRun(c *Conn, req Fetch, read func(Fetch, *store.Snapshot) (proto.Fetched, error)) {
	if !g.fetAdmit(c, req.ID) {
		return
	}
	frame, err := g.fetRead(req, read)
	if err != nil {
		g.loop.cfg.Report(fmt.Errorf("group %s, a fetch of %s: %w", g.state.ID, c.device, err))
		c.send(runErr(req.ID, "internal", "the home could not read the group; the cause is in its log"))
		return
	}
	c.send(frame)
}

func (g *runGroup) fetAdmit(c *Conn, re int64) bool {
	if g.seats[c] == nil {
		return false
	}
	refusal := runReader(g.state, c.device)
	if refusal != nil {
		c.send(runErr(re, refusal.Code, refusal.Msg))
		return false
	}
	return true
}

func (g *runGroup) fetRead(req Fetch, read func(Fetch, *store.Snapshot) (proto.Fetched, error)) (proto.Fetched, error) {
	snap, err := g.loop.cfg.Store.Snapshot(g.state.ID)
	if err != nil {
		return proto.Fetched{}, err
	}
	defer snap.Close()
	return read(req, snap)
}

func fetPageRead(req Fetch, snap *store.Snapshot) (proto.Fetched, error) {
	rows, err := fetQueryRows(req, snap)
	if err != nil {
		return proto.Fetched{}, err
	}
	return fetPack(req.ID, rows, req.Limit), nil
}

func fetQueryRows(req Fetch, snap *store.Snapshot) ([]json.RawMessage, error) {
	if req.Kind != "" {
		return snap.PageKind(req.Kind, req.Before, req.Limit+1)
	}
	return snap.PageThread(req.Thread, req.Before, req.Limit+1)
}

func fetOneRow(req Fetch, snap *store.Snapshot) (proto.Fetched, error) {
	row, err := snap.Row(req.Row, req.RowID)
	if errors.Is(err, sql.ErrNoRows) {
		return fetFetched(req.ID, []json.RawMessage{}, false), nil
	}
	if err != nil {
		return proto.Fetched{}, err
	}
	// A stored row is at most 16 KiB (planning/contracts/conventions.md section 4),
	// so one row always fits in a 60 KiB ctl frame.
	return fetFetched(req.ID, []json.RawMessage{row}, false), nil
}

func fetPack(re int64, rows []json.RawMessage, limit int) proto.Fetched {
	empty, _ := runJSON(fetFetched(re, []json.RawMessage{}, true))
	// 61,440 bytes is the ctl message limit (planning/contracts/conventions.md section 4).
	room := 61_440 - len(empty)
	page := fetFirst(runBatches(rows[:min(len(rows), limit)], room))
	if len(page) == len(rows) {
		// "more":false is one byte longer than "more":true.
		page = fetFirst(runBatches(page, room-1))
	}
	return fetFetched(re, page, len(page) < len(rows))
}

func fetFirst(batches [][]json.RawMessage) []json.RawMessage {
	if len(batches) == 0 {
		return []json.RawMessage{}
	}
	return batches[0]
}

func fetFetched(re int64, rows []json.RawMessage, more bool) proto.Fetched {
	return proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: re, OK: true},
		Rows:    rows,
		More:    more,
	}
}
