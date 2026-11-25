package home

import (
	"maps"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Rounds in task threads, queue jumps and the quiet-task timer
// (planning/contracts/task-machine.md rows 21 to 23 and section 4;
// planning/contracts/commands.md section 1.4;
// planning/contracts/messages.md section 2.11).

func init() {
	Commands.Add("jump", rndJump)
	Timers.Add("quiet", rndQuiet)
	After.Add("quiet", rndRememberQuiet)
}

const (
	rndQuietCode   = "quiet"
	rndMinuteMS    = 60_000
	rndLimitPrefix = "SoloDawn rounds limit "
)

// rndWrap attaches the thread round rules to a post command; Table.Add calls
// it for every key, so no file's init order matters.
func rndWrap(key string, f Func) Func {
	if strings.HasPrefix(key, "post/") {
		return rndPost(f)
	}
	return f
}

// rndJump implements the jump command (commands.md 1.4, R-72): by the person
// who wrote msg, every unacknowledged mention item that msg produced jumps.
func rndJump(_ *State, c Command) (Result, *Refusal) {
	if c.Who.AI != "" {
		return Result{}, &Refusal{"forbidden", "only a person jumps the queue"}
	}
	var p proto.Jump
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	target, ok := c.Disk.Msg(string(p.Msg))
	if !ok {
		return Result{}, &Refusal{"not_found", "the message named by jump does not exist"}
	}
	if target.By != c.Who.Human {
		return Result{}, &Refusal{"forbidden", "only the author of the message jumps the queue"}
	}
	return rndJumpItems(c.Disk, target.ID)
}

func rndJumpItems(d Disk, msg string) (Result, *Refusal) {
	var res Result
	for _, q := range d.Queued() {
		if q.Why == postMentionWhy && postTaskID(q.Msg) == msg {
			res.Jump = append(res.Jump, q.ID)
		}
	}
	if res.Jump == nil {
		return Result{}, &Refusal{"conflict", "no unacknowledged item of that message is queued"}
	}
	return res, nil
}

// rndQuiet is the quiet-task timer (task-machine.md row 23): while the group is
// not paused, for each active task with no wait before step merge that has had
// no thread message for limits.idle_notice_minutes, it queues one quiet notice
// for the secretary per quiet period.
func rndQuiet(s *State, _ Disk, now int64) Result {
	if s.Group.Secretary == "" || s.Group.Limits.IdleNoticeMinutes <= 0 || s.Group.Paused != nil {
		return Result{}
	}
	var items []Item
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
		if it, ok := s.rndQuietTask(s.Tasks[id], now); ok {
			items = append(items, it)
		}
	}
	return Result{Items: items}
}

func (s *State) rndQuietTask(task proto.TaskRow, now int64) (Item, bool) {
	last := rndLastActivity(task)
	quietMS := int64(s.Group.Limits.IdleNoticeMinutes) * rndMinuteMS
	if now-last < quietMS || s.quiet[task.ID] == last {
		return Item{}, false
	}
	if _, fx, err := Step(task, Event{Kind: OnQuiet, By: ByHome, Now: now}); err == nil {
		return Item{AI: s.Group.Secretary, Why: itemNotice, Task: task.ID, Code: fx.Items[0].Code}, true
	}
	return Item{}, false
}

func rndLastActivity(task proto.TaskRow) int64 {
	if task.Thread.Last != nil {
		return task.Thread.Last.Timestamp
	}
	if task.Stats.Started != 0 {
		return task.Stats.Started
	}
	return task.Created
}

// rndRememberQuiet records, once a quiet notice has committed, the last
// activity of its task: the quiet period it reported.
func rndRememberQuiet(s *State, res Result) {
	for _, it := range res.Items {
		if it.Why == itemNotice && it.Code == rndQuietCode {
			s.quiet[it.Task] = rndLastActivity(s.Tasks[it.Task])
		}
	}
}

func rndPost(next Func) Func {
	return func(s *State, c Command) (Result, *Refusal) {
		res, refusal := next(s, c)
		if refusal != nil {
			return Result{}, refusal
		}
		return s.rndAfterPost(c, res), nil
	}
}

func (s *State) rndAfterPost(c Command, res Result) Result {
	msg, inThread := rndThreadMsg(res.Rows, string(c.Nonce))
	if !inThread {
		return res
	}
	task := rndFindTask(res.Rows)
	if c.Who.AI == "" {
		return s.rndPersonPost(c, res, task)
	}
	return s.rndAIPost(c, res, msg, task)
}

func (s *State) rndPersonPost(c Command, res Result, task proto.TaskRow) Result {
	// Row 22 takes any person, admin or member alike.
	ev := Event{Kind: OnText, By: ByPerson, Now: c.Now, Limits: s.Group.Limits, Pending: tskPending(c.Disk, task.ID)}
	moved, fx, _ := Step(task, ev)
	res.Rows = rndPutTask(res.Rows, moved)
	extra := tskItemCtx{taskID: moved.ID, kept: c.Disk.Queued()}.items(fx.Items)
	res.Items = append(extra, res.Items...)
	return res
}

func (s *State) rndAIPost(c Command, res Result, msg proto.MsgRow, task proto.TaskRow) Result {
	if s.rndMentionsAI(msg, res.Items) {
		return s.rndCountRound(c, res, task)
	}
	return res
}

func (s *State) rndCountRound(c Command, res Result, task proto.TaskRow) Result {
	// Row 21 counts mentions from working AIs and the secretary alike.
	moved, fx, _ := Step(task, Event{Kind: OnMention, By: ByAI, Member: c.Who.AI, Now: c.Now, Limits: s.Group.Limits})
	res.Rows = rndPutTask(res.Rows, moved)
	if fx.System == "" {
		return res
	}
	body := proto.RoundsLimitBody{SystemCode: proto.SystemCode{Code: fx.System}, Task: moved.ID, N: s.Group.Limits.Rounds}
	sysMsg := s.tskThreadSystem(c, revDerivedID(rndLimitPrefix, c.Nonce), "system", nil, []string{}, body)
	res.Rows = append(res.Rows, RowOf(sysMsg))
	notices := tskItemCtx{taskID: moved.ID, secretary: s.Group.Secretary, noticeMsg: sysMsg.ID}.items(fx.Items)
	res.Items = append(res.Items, notices...)
	return res
}

func (s *State) rndMentionsAI(msg proto.MsgRow, items []Item) bool {
	for _, id := range msg.Mentions {
		if _, ok := s.AIs[id]; ok && id != msg.By {
			return true
		}
	}
	return slices.ContainsFunc(items, func(it Item) bool { return it.Why == postMentionWhy })
}

func rndThreadMsg(rows []Row, id string) (found proto.MsgRow, inThread bool) {
	for _, r := range rows {
		if m, ok := r.Value().(proto.MsgRow); ok && m.ID == id {
			found, inThread = m, m.Thread != nil
		}
	}
	return found, inThread
}

func rndFindTask(rows []Row) (found proto.TaskRow) {
	for _, r := range rows {
		if t, ok := r.Value().(proto.TaskRow); ok {
			found = t
		}
	}
	return found
}

func rndPutTask(rows []Row, task proto.TaskRow) []Row {
	out := slices.Clone(rows)
	for i, r := range out {
		if _, ok := r.Value().(proto.TaskRow); ok {
			out[i] = RowOf(task)
		}
	}
	return out
}
