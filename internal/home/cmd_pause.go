package home

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Pausing and resuming a group: group pause and group resume
// (planning/contracts/commands.md 1.1; planning/contracts/messages.md 2.11;
// planning/contracts/task-machine.md row 24).

func init() {
	Commands.Add("group/pause", pauPause)
	Commands.Add("group/resume", pauResume)
}

const (
	pauCodePaused  = "group_paused"
	pauCodeResumed = "group_resumed"
)

// pauPause sets group.paused and writes system message group_paused. Pausing a
// group that is already paused changes nothing.
func pauPause(s *State, c Command) (Result, *Refusal) {
	if refusal := pauCheck(s, c); refusal != nil {
		return Result{}, refusal
	}
	if s.Group.Paused != nil {
		return Result{}, nil
	}
	rows, refusal := pauRows(s, c, &proto.GroupPause{By: c.Who.Human, At: c.Now}, pauCodePaused)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: rows}, nil
}

// pauResume clears group.paused, writes system message group_resumed, and runs
// row 24 of the task machine on every task. Resuming a group that is not
// paused changes nothing.
func pauResume(s *State, c Command) (Result, *Refusal) {
	if refusal := pauCheck(s, c); refusal != nil {
		return Result{}, refusal
	}
	if s.Group.Paused == nil {
		return Result{}, nil
	}
	rows, refusal := pauRows(s, c, nil, pauCodeResumed)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: rows, Items: pauWake(s, c.Disk.Queued())}, nil
}

// pauCheck decodes a group command and requires an admin signer.
func pauCheck(s *State, c Command) *Refusal {
	var p proto.GroupCommand
	if refusal := c.Decode(&p); refusal != nil {
		return refusal
	}
	if !s.Admin(c.Who) {
		return &Refusal{Code: "forbidden", Msg: "only an admin may pause or resume a group"}
	}
	return nil
}

// pauRows builds the updated group row and main-line system message, refusing
// when a message already holds the command's nonce.
func pauRows(s *State, c Command, paused *proto.GroupPause, code string) ([]Row, *Refusal) {
	if _, taken := c.Disk.Msg(string(c.Nonce)); taken {
		return nil, &Refusal{Code: "conflict", Msg: "a message with the id of the command exists"}
	}
	grp := s.Group
	grp.Paused = paused
	return []Row{RowOf(grp), RowOf(pauSystem(s, c, code))}, nil
}

// pauSystem builds the main-line system message for a group pause or resume.
func pauSystem(s *State, c Command, code string) proto.MsgRow {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	enc.Encode(proto.GroupPauseBody{SystemCode: proto.SystemCode{Code: code}, By: c.Who.Human})
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, By: "system", Kind: "system",
		Timestamp: c.Now, Mentions: []string{}, Body: bytes.TrimSuffix(body.Bytes(), []byte("\n")),
	}
}

// pauWake collects the inbox items of task-machine row 24 across the group's
// tasks in task-id order.
func pauWake(s *State, queued []store.Item) []Item {
	var items []Item
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
		_, fx, _ := Step(s.Tasks[id], Event{Kind: OnGroupResume, By: ByHome})
		items = append(items, pauItems(queued, id, fx.Items)...)
	}
	return items
}

// pauItems completes the machine's items for task, skipping an IfNone item
// when a matching item is already queued.
func pauItems(queued []store.Item, task string, items []TaskItem) []Item {
	var out []Item
	for _, it := range items {
		if it.IfNone && pauQueued(queued, task, it) {
			continue
		}
		out = append(out, Item{AI: it.AI, Why: it.Why, Task: task, Commit: it.Commit})
	}
	return out
}

func pauQueued(queued []store.Item, task string, it TaskItem) bool {
	for _, q := range queued {
		if pauSame(q, task, it) {
			return true
		}
	}
	return false
}

func pauSame(q store.Item, task string, it TaskItem) bool {
	return q.Why == it.Why && q.AI == it.AI && pauText(q.Task) == task && pauText(q.Commit) == it.Commit
}

func pauText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
