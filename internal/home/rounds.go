package home

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"weak"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Rounds in task threads, held mentions, queue jumps and the quiet-task timer
// (planning/contracts/task-machine.md rows 13, 21 to 23 and section 4;
// planning/contracts/commands.md section 1.4;
// planning/contracts/messages.md section 2.11).

func init() {
	Commands.Add("jump", rndJump)
	Timers.Add("quiet", rndQuiet)
	After.Add("quiet", rndRememberQuiet)
	rndWrap(Commands)
}

const (
	rndQuietCode = "quiet"
	rndMinuteMS  = 60_000
)

var (
	rndQuietMu   sync.Mutex
	rndQuietSeen = map[weak.Pointer[State]]map[string]proto.TaskRow{}
)

// rndWrap attaches the thread round rules and held-mention release to every
// post command and to task/resume in t.
func rndWrap(t Table) {
	for key, f := range t {
		if strings.HasPrefix(key, "post/") {
			t[key] = rndPost(f)
		}
	}
	t["task/resume"] = rndResume(t["task/resume"])
}

// rndJump implements the jump command (commands.md 1.4, R-72): by the person
// who wrote msg, it retires the newest unacknowledged mention item produced by
// a message of that person in the same thread (or main line) and queues its
// replacement with Jump set.
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
	return rndRequeueJump(c.Disk, c.Who.Human, target.Thread)
}

func rndRequeueJump(d Disk, author string, thread *string) (Result, *Refusal) {
	queued := d.Queued()
	for i := len(queued) - 1; i >= 0; i-- {
		q := queued[i]
		if rndJumpMatch(d, q, author, thread) {
			it := Item{AI: q.AI, Why: postMentionWhy, Task: postTaskID(q.Task), Msg: *q.Msg, Jump: true}
			return Result{Items: []Item{it}, Retire: []int64{q.ID}}, nil
		}
	}
	return Result{}, &Refusal{"conflict", "no unacknowledged item from that person in that thread is queued"}
}

func rndJumpMatch(d Disk, q store.Item, author string, thread *string) bool {
	if q.Why != postMentionWhy || q.Msg == nil {
		return false
	}
	msg, _ := d.Msg(*q.Msg)
	return msg.By == author && postTaskID(msg.Thread) == postTaskID(thread)
}

// rndQuiet is the quiet-task timer (task-machine.md row 23): for each active
// task with no wait before step merge that has had no thread message for
// limits.idle_notice_minutes, it queues one quiet notice for the secretary per
// quiet period.
func rndQuiet(s *State, d Disk, now int64) Result {
	if s.Group.Secretary == "" || s.Group.Limits.IdleNoticeMinutes <= 0 {
		return Result{}
	}
	queued := d.Queued()
	var items []Item
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
		if it, ok := s.rndQuietTask(s.Tasks[id], queued, now); ok {
			items = append(items, it)
		}
	}
	return Result{Items: items}
}

func (s *State) rndQuietTask(task proto.TaskRow, queued []store.Item, now int64) (Item, bool) {
	if !rndSilentFor(task, s.Group.Limits.IdleNoticeMinutes, now) || rndAlreadyQuiet(s, task, queued) {
		return Item{}, false
	}
	if _, fx, err := Step(task, Event{Kind: OnQuiet, By: ByHome, Now: now}); err == nil {
		return Item{AI: s.Group.Secretary, Why: itemNotice, Task: task.ID, Code: fx.Items[0].Code}, true
	}
	return Item{}, false
}

func rndSilentFor(task proto.TaskRow, idleMinutes int, now int64) bool {
	return now-rndLastActivity(task) >= int64(idleMinutes)*rndMinuteMS
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

func rndAlreadyQuiet(s *State, task proto.TaskRow, queued []store.Item) bool {
	if rndHasQuietItem(queued, task.ID) {
		return true
	}
	rndQuietMu.Lock()
	defer rndQuietMu.Unlock()
	return reflect.DeepEqual(rndQuietSeen[weak.Make(s)][task.ID], task)
}

func rndHasQuietItem(queued []store.Item, taskID string) bool {
	for _, q := range queued {
		if q.Why == itemNotice && tskForTask(q, taskID) && rndStoredItem(q).Code == rndQuietCode {
			return true
		}
	}
	return false
}

// rndRememberQuiet records on s each task whose quiet period just notified the
// secretary, after its transaction has committed.
func rndRememberQuiet(s *State, res Result) {
	rndQuietMu.Lock()
	defer rndQuietMu.Unlock()
	key := weak.Make(s)
	for _, it := range res.Items {
		if it.Why == itemNotice && it.Code == rndQuietCode {
			if rndQuietSeen[key] == nil {
				rndQuietSeen[key] = map[string]proto.TaskRow{}
			}
			rndQuietSeen[key][it.Task] = s.Tasks[it.Task]
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
	wasRounds := rndInRounds(task)
	queued := c.Disk.Queued()
	// Row 22 takes any person, admin or member alike.
	moved, fx, _ := Step(task, Event{
		Kind: OnText, By: ByPerson, Now: c.Now, Limits: s.Group.Limits, Pending: tskPending(c.Disk, task.ID),
	})
	res.Rows = rndPutTask(res.Rows, moved)
	if !wasRounds {
		return res
	}
	retire, released := rndRelease(queued, moved.ID)
	res.Retire = append(res.Retire, retire...)
	extra := append(tskItemCtx{taskID: moved.ID, kept: queued}.items(fx.Items), released...)
	res.Items = append(extra, res.Items...)
	return res
}

func (s *State) rndAIPost(c Command, res Result, msg proto.MsgRow, task proto.TaskRow) Result {
	if s.rndMentionsAI(msg, res.Items) {
		task, res = s.rndCountRound(c, res, task)
	}
	if rndInRounds(task) {
		res = rndHoldMentions(c.Disk.Queued(), res, task.ID)
	}
	return res
}

func (s *State) rndCountRound(c Command, res Result, task proto.TaskRow) (proto.TaskRow, Result) {
	// Row 21 counts mentions from working AIs and the secretary alike.
	moved, fx, _ := Step(task, Event{
		Kind: OnMention, By: ByAI, Member: c.Who.AI, Now: c.Now, Limits: s.Group.Limits,
	})
	res.Rows = rndPutTask(res.Rows, moved)
	if fx.System == "" {
		return moved, res
	}
	body := proto.RoundsLimitBody{
		SystemCode: proto.SystemCode{Code: fx.System}, Task: moved.ID, N: s.Group.Limits.Rounds,
	}
	sysMsg := s.tskThreadSystem(c, rndSystemID(c.Nonce), "system", nil, []string{}, body)
	res.Rows = append(res.Rows, RowOf(sysMsg))
	res.Items = append(res.Items, tskItemCtx{
		taskID: moved.ID, secretary: s.Group.Secretary, noticeMsg: sysMsg.ID,
	}.items(fx.Items)...)
	return moved, res
}

// rndSystemID derives the id of a main-line rounds_limit system message from
// the nonce of the post that triggered row 21.
func rndSystemID(nonce proto.ID) string {
	sum := sha256.Sum256([]byte("SoloDawn rounds limit " + string(nonce)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func (s *State) rndMentionsAI(msg proto.MsgRow, items []Item) bool {
	for _, id := range msg.Mentions {
		if _, ok := s.AIs[id]; ok {
			return true
		}
	}
	for _, it := range items {
		if it.Why == postMentionWhy {
			return true
		}
	}
	return false
}

func rndHoldMentions(queued []store.Item, res Result, taskID string) Result {
	var held []Item
	for _, q := range queued {
		if rndShouldHold(q, taskID) {
			res.Retire = append(res.Retire, q.ID)
			held = append(held, rndRequeuedMention(q, taskID, runNever))
		}
	}
	for i := range res.Items {
		if res.Items[i].Why == postMentionWhy {
			res.Items[i].NotBefore = runNever
		}
	}
	res.Items = append(held, res.Items...)
	return res
}

func rndShouldHold(q store.Item, taskID string) bool {
	return q.Why == postMentionWhy && tskForTask(q, taskID) && q.NotBefore != runNever
}

func rndResume(next Func) Func {
	return func(s *State, c Command) (Result, *Refusal) {
		res, refusal := next(s, c)
		if refusal != nil {
			return Result{}, refusal
		}
		return rndAfterResume(s, c.Disk, res), nil
	}
}

func rndAfterResume(s *State, d Disk, res Result) Result {
	moved := rndFindTask(res.Rows)
	if rndInRounds(s.Tasks[moved.ID]) {
		retire, released := rndRelease(d.Queued(), moved.ID)
		res.Retire = append(res.Retire, retire...)
		res.Items = append(res.Items, released...)
	}
	return res
}

func rndRelease(queued []store.Item, taskID string) (retire []int64, released []Item) {
	for _, q := range queued {
		if rndIsHeld(q, taskID) {
			retire = append(retire, q.ID)
			released = append(released, rndRequeuedMention(q, taskID, 0))
		}
	}
	return retire, released
}

func rndIsHeld(q store.Item, taskID string) bool {
	return q.Why == postMentionWhy && tskForTask(q, taskID) && q.NotBefore == runNever
}

func rndRequeuedMention(q store.Item, taskID string, notBefore int64) Item {
	return Item{
		AI: q.AI, Why: postMentionWhy, Task: taskID, Msg: postTaskID(q.Msg),
		Jump: bool(rndStoredItem(q).Jump), NotBefore: notBefore,
	}
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

func rndInRounds(task proto.TaskRow) bool {
	return task.Wait != nil && task.Wait.Kind == waitRounds
}

func rndStoredItem(q store.Item) (it proto.Item) {
	json.Unmarshal(q.Env, &it)
	return it
}
