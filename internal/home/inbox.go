package home

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Ack records that the host on c has handled the inbox item req.Item of
// req.Group, so it is deleted and not sent again
// (planning/contracts/datachannel.md section 6). An ack for a group not homed
// here or for another host's item changes nothing.
func (l *Loop) Ack(c *Conn, req proto.Ack) {
	l.mu.Lock()
	g := l.groups[string(req.Group)]
	if g != nil {
		g.jobs.push(func() { g.ack(c.device, req.Item) })
	}
	l.mu.Unlock()
}

func (g *runGroup) ack(device string, item int64) {
	g.loop.diag.Info("ack.recv", "g", g.state.ID, "dev", diagDevice(device), "item", item)
	g.failed(g.loop.cfg.Store.Write(func(tx *store.Tx) error {
		return tx.Ack(g.state.ID, device, item)
	}))
}

// inboxSeal encodes the plain wire item stored in inbox.env when it is queued
// under id (planning/contracts/datachannel.md section 6).
func inboxSeal(it Item, id int64) (json.RawMessage, error) {
	return runJSON(proto.Item{
		Item:   id,
		AI:     proto.ID(it.AI),
		Why:    it.Why,
		Task:   proto.ID(it.Task),
		Msg:    proto.ID(it.Msg),
		Code:   it.Code,
		Detail: it.Detail,
	})
}

// inboxEntry builds one wire InboxItem from a stored item, copying the message
// and task rows it names as they stand when the item is sent.
func inboxEntry(s *State, d Disk, it store.Item) (proto.InboxItem, json.RawMessage, error) {
	var plain proto.Item
	if err := json.Unmarshal(it.Env, &plain); err != nil {
		return proto.InboxItem{}, nil, err
	}
	entry := proto.InboxItem{
		Item: plain,
		Msg:  inboxMsg(d, string(plain.Msg)),
		Task: inboxTask(s, string(plain.Task)),
	}
	raw, err := runJSON(entry)
	return entry, raw, err
}

func inboxMsg(d Disk, id string) json.RawMessage {
	if id == "" {
		return nil
	}
	msg, ok := d.Msg(id)
	if !ok {
		return nil
	}
	raw, _ := runJSON(msg) // it encoded when it was written
	return raw
}

func inboxTask(s *State, id string) *proto.TaskRow {
	task, ok := s.Tasks[id]
	if !ok {
		return nil
	}
	return &task
}

// inboxFrames packs wire items into the fewest inbox frames that fit on ctl, in
// queue order (planning/contracts/conventions.md section 4).
func inboxFrames(group string, entries []proto.InboxItem, raws []json.RawMessage) []any {
	var frames []any
	for _, batch := range runBatches(raws, runRoom) {
		frames = append(frames, proto.Inbox{Type: "inbox", Group: proto.ID(group), Items: entries[:len(batch)]})
		entries = entries[len(batch):]
	}
	return frames
}

// inboxByPerson reports whether a person signed a command: its device hosts no
// AI of the group, so neither an AI nor a host speaks through it.
func inboxByPerson(s *State, device string) bool {
	for _, ai := range s.AIs {
		if ai.Host == device {
			return false
		}
	}
	return true
}

// inboxPace applies the group's hourly wake-up cap to items without mutating s
// (planning/contracts/commands.md sections 1.3 and 3).
func inboxPace(s *State, items []Item, now int64) []Item {
	wakes := maps.Clone(s.Wakes)
	out := slices.Clone(items)
	for i, it := range out {
		out[i] = inboxWake(wakes, it, s.Group.Limits.WakesPerHour, now)
	}
	return out
}

// inboxWake records one item in wakes and, when limit > 0, holds it once the AI
// has limit due times in the last hour. A watch item is for the AI's host
// rather than a session, and a reply answers a call that may be waiting, so
// they neither wait on nor count toward the cap.
func inboxWake(wakes map[string][]int64, it Item, limit int, now int64) Item {
	if it.Why == cfmWhatWatch || it.Why == calWhyReply {
		return it
	}
	recent := runRecent(wakes[it.AI], now)
	if limit > 0 && len(recent) >= limit {
		it.NotBefore = max(it.NotBefore, recent[len(recent)-limit]+runHour)
	}
	wakes[it.AI] = append(recent, max(now, it.NotBefore))
	return it
}
