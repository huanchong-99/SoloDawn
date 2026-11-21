package home

import (
	"encoding/json"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Inbox wire framing, host acknowledgements and the hourly wake-up cap
// (planning/contracts/datachannel.md section 6;
// planning/contracts/commands.md sections 1.3 and 3).

// Ack records that the host on c has handled the inbox item req.Item of
// req.Group, so it is deleted and not sent again
// (planning/contracts/datachannel.md section 6). An ack for a group not homed
// here, on a connection that is gone or whose device the group does not serve,
// or for another host's item changes nothing.
func (l *Loop) Ack(c *Conn, req proto.Ack) {
	l.mu.Lock()
	g := l.groups[string(req.Group)]
	if g != nil {
		g.jobs.push(func() { g.ack(c, req.Item) })
	}
	l.mu.Unlock()
}

func (g *runGroup) ack(c *Conn, item int64) {
	if g.seats[c] == nil || !g.state.Serves(c.device) {
		return
	}
	g.failed(g.loop.cfg.Store.Write(func(tx *store.Tx) error {
		return tx.Ack(g.state.ID, c.device, item)
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
		Commit: it.Commit,
		PR:     it.PR,
		Jump:   proto.True(it.Jump),
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
	raw, _ := runJSON(msg)
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

// inboxPace applies the group's hourly wake-up cap to items without mutating s.
// A merge item is for the assignee's host rather than a session, so it does not
// wake an AI or wait on the cap.
func inboxPace(s *State, items []Item, now int64) []Item {
	limit := s.Group.Limits.WakesPerHour
	if limit <= 0 {
		return items
	}
	local := map[string][]int64{}
	out := slices.Clone(items)
	for i, it := range out {
		if it.Why == itemMerge {
			continue
		}
		recent := inboxWakes(s, local, it.AI, now)
		if len(recent) >= limit {
			it.NotBefore = max(it.NotBefore, recent[len(recent)-limit]+runHour)
			out[i] = it
		}
		local[it.AI] = append(recent, max(now, it.NotBefore))
	}
	return out
}

func inboxWakes(s *State, local map[string][]int64, ai string, now int64) []int64 {
	recent, ok := local[ai]
	if !ok {
		recent = slices.Clone(s.Wakes[ai])
	}
	recent = slices.DeleteFunc(recent, func(at int64) bool { return at <= now-runHour })
	slices.Sort(recent)
	return recent
}
