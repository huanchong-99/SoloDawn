package home

import (
	"cmp"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const waitConfirm = "confirm"

func cftRequest(task proto.TaskRow) string {
	if task.Brief != "" {
		return task.Brief
	}
	return task.ID
}

func cftRoleRequest(req proto.MsgRow) bool { return req.Kind == briefKind || req.Kind == "task" }

func cftCard(d Disk, task proto.TaskRow, ai string) proto.MsgRow {
	card, _ := d.Msg(cfmCardID(proto.ID(cftRequest(task)), ai))
	return card
}

func cftHeld(card proto.MsgRow) bool {
	return card.State != nil && card.State.Status != briefConfirmed
}

type cftDisk struct {
	Disk
	msgs map[string]proto.MsgRow
}

func (d cftDisk) Msg(id string) (proto.MsgRow, bool) {
	if m, ok := d.msgs[id]; ok {
		return m, true
	}
	return d.Disk.Msg(id)
}

func (d cftDisk) Pending() []proto.MsgRow {
	cards := maps.Clone(d.msgs)
	for _, card := range d.Disk.Pending() {
		if _, changed := cards[card.ID]; !changed {
			cards[card.ID] = card
		}
	}
	var out []proto.MsgRow
	for _, card := range cards {
		if briefStatus(card) == cardStatusPending {
			out = append(out, card)
		}
	}
	slices.SortFunc(out, cftCardOrder)
	return out
}

func cftCardOrder(a, b proto.MsgRow) int {
	if a.Pos == 0 {
		a.Pos = 1 << 53
	}
	if b.Pos == 0 {
		b.Pos = 1 << 53
	}
	if n := cmp.Compare(a.Pos, b.Pos); n != 0 {
		return n
	}
	return strings.Compare(a.ID, b.ID)
}

func (s *State) cftFinish(c Command, res Result) Result {
	view := *s
	view.Tasks = maps.Clone(s.Tasks)
	d := cftDisk{Disk: c.Disk, msgs: map[string]proto.MsgRow{}}
	changed := map[string]bool{}
	cftReadRows(&view, d, changed, res.Rows)
	for _, id := range slices.Sorted(maps.Keys(changed)) {
		s.cftNewRoles(c, view.Tasks[id], d, &res)
	}
	view.cftExpire(c, d, &res)
	res.Items = view.cftFilterItems(c, d, res.Items, changed, s.Tasks)
	for _, id := range slices.Sorted(maps.Keys(changed)) {
		task := view.Tasks[id]
		if task.State == taskActive {
			task.Wait = cftWait(d, task)
			cftPutTask(&res, task, view.Tasks[id])
		}
	}
	return res
}

func cftReadRows(view *State, d cftDisk, changed map[string]bool, rows []Row) {
	for _, row := range rows {
		switch r := row.Value().(type) {
		case proto.TaskRow:
			view.Tasks[r.ID], changed[r.ID] = r, true
		case proto.MsgRow:
			d.msgs[r.ID] = r
		}
	}
}

func (s *State) cftFilterItems(c Command, d Disk, items []Item, changed map[string]bool, old map[string]proto.TaskRow) []Item {
	return slices.DeleteFunc(slices.Clone(items), func(it Item) bool {
		if it.Task == "" {
			return false
		}
		changed[it.Task] = true
		return s.cftHoldItem(c, d, it, old[it.Task])
	})
}

func (s *State) cftNewRoles(c Command, task proto.TaskRow, d cftDisk, res *Result) {
	old, exists := s.Tasks[task.ID]
	if !exists && task.Brief == "" {
		return
	}
	for _, role := range []struct{ ai, old string }{{task.Assignee, old.Assignee}, {task.Reviewer, old.Reviewer}} {
		if role.ai == "" || role.ai == role.old {
			continue
		}
		s.cftNewRole(c, task, s.AIs[role.ai], !exists, d, res)
	}
}

func (s *State) cftNewRole(c Command, task proto.TaskRow, ai proto.AIMemberRow, initial bool, d cftDisk, res *Result) {
	if c.Who.User == ai.User {
		return
	}
	if initial && cfmEnough(c.Who.User, ai) {
		return
	}
	req := proto.MsgRow{RowHeader: proto.RowHeader{ID: cftRequest(task)}}
	card := s.cfmNewCard(c, cfmWhatDirect, req, ai)
	if _, found := d.Msg(card.ID); found {
		return
	}
	d.msgs[card.ID] = card
	res.Rows = append(res.Rows, RowOf(card))
}

func (s *State) cftExpire(c Command, d cftDisk, res *Result) {
	for _, card := range d.Pending() {
		if card.Kind != cfmKind {
			continue
		}
		if card.Re == nil {
			continue
		}
		req, _ := d.Msg(*card.Re)
		if !cftRoleRequest(req) {
			continue
		}
		var body proto.ConfirmBody
		json.Unmarshal(card.Body, &body)
		if !s.cftHasRole(req.ID, body.AI) {
			at := c.Now
			card.State = &proto.CardState{Status: "expired", At: &at}
			d.msgs[card.ID] = card
			res.Rows = append(res.Rows, RowOf(card))
		}
	}
}

func (s *State) cftHasRole(request, ai string) bool {
	for _, task := range s.Tasks {
		if cftRequest(task) == request && cftNames(task, ai) && !cftFinal(task.State) {
			return true
		}
	}
	return false
}

func cftNames(task proto.TaskRow, ai string) bool { return task.Assignee == ai || task.Reviewer == ai }

func cftFinal(state string) bool { return state == taskMerged || state == taskCancelled }

func (s *State) cftHoldItem(c Command, d Disk, it Item, old proto.TaskRow) bool {
	if !slices.Contains(workItems, it.Why) {
		return false
	}
	task := s.Tasks[it.Task]
	if cftOwnerChange(c, old, task, it.AI, s.AIs[it.AI].User) {
		return false
	}
	return cftHeld(cftCard(d, task, it.AI))
}

func cftOwnerChange(c Command, old, task proto.TaskRow, ai, owner string) bool {
	if c.Who.User != owner {
		return false
	}
	return (task.Assignee == ai && old.Assignee != ai) || (task.Reviewer == ai && old.Reviewer != ai)
}

func cftWait(d Disk, task proto.TaskRow) *proto.TaskWait {
	if wait := cftRoleWait(d, task); wait != nil {
		return wait
	}
	if task.Wait == nil || task.Wait.Kind != waitConfirm {
		return task.Wait
	}
	return cftOrdinaryWait(d, task.ID)
}

func cftRoleWait(d Disk, task proto.TaskRow) *proto.TaskWait {
	roles := []string{task.Assignee}
	if task.Step == stepReview {
		roles = append(roles, task.Reviewer)
	}
	for _, ai := range roles {
		card := cftCard(d, task, ai)
		if briefStatus(card) == cardStatusPending {
			var body proto.ConfirmBody
			json.Unmarshal(card.Body, &body)
			return &proto.TaskWait{Kind: waitConfirm, Msg: card.ID, Who: body.To, Since: card.Timestamp}
		}
	}
	return nil
}

func cftOrdinaryWait(d Disk, taskID string) *proto.TaskWait {
	for _, kind := range []string{waitApproval, waitDecision} {
		for _, card := range tskPending(d, taskID) {
			if card.Kind == kind {
				return taskCardWait(card)
			}
		}
	}
	return nil
}

func cftPutTask(res *Result, task, old proto.TaskRow) {
	if reflect.DeepEqual(task, old) {
		return
	}
	for i, row := range res.Rows {
		if t, ok := row.Value().(proto.TaskRow); ok && t.ID == task.ID {
			res.Rows[i] = RowOf(task)
			return
		}
	}
	res.Rows = append(res.Rows, RowOf(task))
}

func (s *State) cftDecide(c Command, card proto.MsgRow, body proto.ConfirmBody) Result {
	res := Result{Rows: []Row{RowOf(card)}}
	for _, task := range s.cftTasksFor(*card.Re, body.AI) {
		res.Rows = append(res.Rows, RowOf(task))
		if briefStatus(card) == briefConfirmed && task.State == taskActive {
			res.Items = append(res.Items, cftActivation(c.Disk, task, body.AI)...)
		}
	}
	if briefStatus(card) == cardStatusDenied {
		res.Items = s.cfmNotice(card)
	}
	return res
}

func (s *State) cftTasksFor(req, ai string) []proto.TaskRow {
	var tasks []proto.TaskRow
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
		task := s.Tasks[id]
		if cftRequest(task) == req && cftNames(task, ai) && !cftFinal(task.State) {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func cftActivation(d Disk, task proto.TaskRow, ai string) []Item {
	ctx := tskItemCtx{taskID: task.ID, assignMsg: tskLatestFeedback(d, task.ID), kept: d.Queued()}
	if task.Step == stepBranch {
		ctx.assignMsg = task.ID
	}
	if ai == task.Assignee {
		return ctx.items([]TaskItem{{AI: ai, Why: itemAssign}})
	}
	if task.Step == stepReview {
		return ctx.items([]TaskItem{{AI: ai, Why: itemReview, Commit: task.Commit, IfNone: true}})
	}
	return nil
}
