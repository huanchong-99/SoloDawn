package home

import (
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Pausing and resuming a group: group pause and group resume
// (planning/contracts/commands.md section 1.1; planning/contracts/messages.md
// section 2.11; planning/contracts/task-machine.md row 24).

func init() {
	Commands.Add("group/pause", pauPause)
	Commands.Add("group/resume", pauResume)
}

const (
	pauKindSystem  = "system"
	pauCodePaused  = "group_paused"
	pauCodeResumed = "group_resumed"
	pauForbidden   = "only an admin may pause or resume a group"
)

// pauPause sets group.paused and writes system message group_paused. Pausing a
// group that is already paused changes nothing.
func pauPause(s *State, c Command) (Result, *Refusal) {
	if !s.Admin(c.Who) {
		return Result{}, &Refusal{Code: "forbidden", Msg: pauForbidden}
	}
	if s.Group.Paused != nil {
		return Result{}, nil
	}
	rows := pauRows(s, c, &proto.GroupPause{By: c.Who.Human, At: c.Now}, pauCodePaused)
	return Result{Rows: rows}, nil
}

// pauResume clears group.paused, writes system message group_resumed, and runs
// row 24 of the task machine on every task. Resuming a group that is not
// paused changes nothing.
func pauResume(s *State, c Command) (Result, *Refusal) {
	if !s.Admin(c.Who) {
		return Result{}, &Refusal{Code: "forbidden", Msg: pauForbidden}
	}
	if s.Group.Paused == nil {
		return Result{}, nil
	}
	rows := pauRows(s, c, nil, pauCodeResumed)
	return Result{Rows: rows, Items: pauWake(s, c.Disk.Queued())}, nil
}

// pauRows builds the updated group row and main-line system message.
func pauRows(s *State, c Command, paused *proto.GroupPause, code string) []Row {
	grp := s.Group
	grp.Paused = paused
	body := proto.GroupPauseBody{SystemCode: proto.SystemCode{Code: code}, By: c.Who.Human}
	msg := s.tskThreadSystem(c, string(c.Nonce), pauKindSystem, nil, []string{}, body)
	return []Row{RowOf(grp), RowOf(msg)}
}

// pauWake collects the inbox items of task-machine row 24 across the group's
// tasks in task-id order.
func pauWake(s *State, queued []store.Item) []Item {
	var items []Item
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
		_, fx, _ := Step(s.Tasks[id], Event{Kind: OnGroupResume, By: ByHome})
		items = append(items, tskItemCtx{taskID: id, kept: queued}.items(fx.Items)...)
	}
	return items
}
