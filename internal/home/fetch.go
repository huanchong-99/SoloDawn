package home

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// fetMaxBytes is the maximum UTF-8 byte length of a ctl message
// (planning/contracts/conventions.md section 4).
const fetMaxBytes = 61_440

type fetMode int

const (
	fetByThread fetMode = iota
	fetByKind
	fetByRow
)

type fetSpec struct {
	mode   fetMode
	group  string
	before int64
	limit  int
	thread *string
	kind   string
	row    string
	rowID  string
}

// Fetch answers the fetch request re that arrived on c with raw as its JSON
// frame on ctl (planning/contracts/datachannel.md section 4). It checks that
// the group exists and serves the device of c and pins a read snapshot on the
// goroutine of the group, then reads and packs the answer outside the group's
// goroutine and queues the response frame on c.
//
// Fetch waits for the group and for the store's writer, as Watch does: it must
// not be called on the goroutine of a group or inside a transaction of the
// store.
func (l *Loop) Fetch(c *Conn, re int64, raw json.RawMessage) {
	spec, ok := fetParse(raw)
	if !ok {
		c.send(fetErr(re, "bad_request", "the fetch request does not match the contract"))
		return
	}
	snap := l.fetOpen(c, re, spec.group)
	if snap == nil {
		return
	}
	defer snap.Close()
	c.send(l.fetAnswer(c.device, re, spec, snap))
}

func (l *Loop) fetOpen(c *Conn, re int64, group string) *store.Snapshot {
	var snap *store.Snapshot
	step := func(g *runGroup) { snap = g.fetSnapshot(c, re) }
	if !l.ask(group, step) {
		c.send(fetErr(re, "not_found", "this node is not home to the group"))
		return nil
	}
	return snap
}

func (g *runGroup) fetSnapshot(c *Conn, re int64) *store.Snapshot {
	if g.seats[c] == nil {
		return nil
	}
	if g.state.Group.ID == "" {
		c.send(fetErr(re, "not_found", "this node is not home to the group"))
		return nil
	}
	if !g.state.Serves(c.device) {
		c.send(fetErr(re, "forbidden", "the device may not read this group"))
		return nil
	}
	snap, err := g.loop.cfg.Store.Snapshot(g.state.ID)
	if err != nil {
		g.loop.cfg.Report(fmt.Errorf("group %s, a fetch of %s: %w", g.state.ID, c.device, err))
		c.send(fetErr(re, "internal", "the home could not read the group; the cause is in its log"))
		return nil
	}
	return snap
}

func (l *Loop) fetAnswer(device string, re int64, spec fetSpec, snap *store.Snapshot) any {
	frame, err := fetRead(re, spec, snap)
	if err != nil {
		l.cfg.Report(fmt.Errorf("group %s, a fetch of %s: %w", spec.group, device, err))
		return fetErr(re, "internal", "the home could not read the group; the cause is in its log")
	}
	return frame
}

func fetRead(re int64, spec fetSpec, snap *store.Snapshot) (proto.Fetched, error) {
	if spec.mode == fetByRow {
		return fetOneRow(re, spec, snap)
	}
	rows, err := fetQueryRows(spec, snap)
	if err != nil {
		return proto.Fetched{}, err
	}
	return fetPage(re, rows, spec.limit), nil
}

func fetQueryRows(spec fetSpec, snap *store.Snapshot) ([]json.RawMessage, error) {
	ask := spec.limit + 1
	if spec.mode == fetByKind {
		return snap.PageKind(spec.kind, spec.before, ask)
	}
	return snap.PageThread(spec.thread, spec.before, ask)
}

func fetOneRow(re int64, spec fetSpec, snap *store.Snapshot) (proto.Fetched, error) {
	row, err := snap.Row(spec.row, spec.rowID)
	if errors.Is(err, sql.ErrNoRows) {
		return fetFetched(re, []json.RawMessage{}, false), nil
	}
	if err != nil {
		return proto.Fetched{}, err
	}
	// A stored row is at most 16 KiB (planning/contracts/conventions.md section 4),
	// so one row always fits in a 60 KiB ctl frame.
	return fetFetched(re, []json.RawMessage{row}, false), nil
}

func fetPage(re int64, queried []json.RawMessage, limit int) proto.Fetched {
	rows := []json.RawMessage{}
	for _, row := range queried[:min(len(queried), limit)] {
		cand := append(rows, row)
		if fetFrameLen(re, cand, len(cand) < len(queried)) > fetMaxBytes {
			break
		}
		rows = cand
	}
	return fetFetched(re, rows, len(rows) < len(queried))
}

func fetFrameLen(re int64, rows []json.RawMessage, more bool) int {
	raw, _ := runJSON(fetFetched(re, rows, more))
	return len(raw)
}

func fetFetched(re int64, rows []json.RawMessage, more bool) proto.Fetched {
	return proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: re, OK: true},
		Rows:    rows,
		More:    more,
	}
}

func fetErr(re int64, code, msg string) proto.ErrorResponse {
	return proto.ErrorResponse{Re: re, Err: proto.WireError{Code: code, Message: msg}}
}

func fetParse(raw json.RawMessage) (fetSpec, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return fetSpec{}, false
	}
	_, hasThread := fields["thread"]
	_, hasKind := fields["kind"]
	_, hasR := fields["r"]
	_, hasRow := fields["row"]
	switch [4]bool{hasThread, hasKind, hasR, hasRow} {
	case [4]bool{true, false, false, false}:
		return fetParseThread(raw)
	case [4]bool{false, true, false, false}:
		return fetParseKind(raw)
	case [4]bool{false, false, true, true}:
		return fetParseRow(raw)
	default:
		return fetSpec{}, false
	}
}

func fetParseThread(raw json.RawMessage) (fetSpec, bool) {
	var req proto.FetchThread
	if proto.Decode(raw, &req) != nil {
		return fetSpec{}, false
	}
	if req.Thread != nil && *req.Thread == "" {
		return fetSpec{}, false
	}
	spec, ok := fetFromPage(req.FetchPage, fetByThread)
	spec.thread = req.Thread
	return spec, ok
}

func fetParseKind(raw json.RawMessage) (fetSpec, bool) {
	var req proto.FetchKind
	if proto.Decode(raw, &req) != nil {
		return fetSpec{}, false
	}
	if req.Kind != "note" && req.Kind != "review" {
		return fetSpec{}, false
	}
	spec, ok := fetFromPage(req.FetchPage, fetByKind)
	spec.kind = req.Kind
	return spec, ok
}

func fetParseRow(raw json.RawMessage) (fetSpec, bool) {
	var req proto.FetchRow
	if proto.Decode(raw, &req) != nil {
		return fetSpec{}, false
	}
	if !fetRowPageOK(req.FetchPage) || req.Row == "" || req.RowID == "" {
		return fetSpec{}, false
	}
	return fetSpec{
		mode:  fetByRow,
		group: req.Group,
		row:   req.Row,
		rowID: req.RowID,
	}, true
}

func fetFromPage(p proto.FetchPage, mode fetMode) (fetSpec, bool) {
	if !fetPageOK(p) {
		return fetSpec{}, false
	}
	return fetSpec{mode: mode, group: p.Group, before: p.Before, limit: p.Limit}, true
}

func fetPageOK(p proto.FetchPage) bool {
	if p.Type != "fetch" || p.Group == "" {
		return false
	}
	return p.Before >= 0 && p.Limit >= 1 && p.Limit <= 100
}

func fetRowPageOK(p proto.FetchPage) bool {
	return p.Type == "fetch" && p.Group != "" && p.Before == 0 && p.Limit == 1
}
