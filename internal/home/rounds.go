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
	rndMentionWhy = "mention"
	rndTextKind   = "text"
	rndSystemBy   = "system"
	rndSystemKind = "system"
	rndQuietCode  = "quiet"
	rndRoundsCode = "rounds_limit"
	rndMinuteMS   = 60_000
)

var (
	rndQuietMu   sync.Mutex
	rndQuietSeen = map[weak.Pointer[State]]map[string]proto.TaskRow{}
)

// rndWrap attaches the thread round rules and held-mention release to every
// post command and to task/resume present in t.
func rndWrap(t Table) {
	for key, f := range t {
		if strings.HasPrefix(key, "post/") {
			t[key] = rndPost(f)
		}
	}
	if f, ok := t["task/resume"]; ok {
		t["task/resume"] = rndResume(f)
	}
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
			it := Item{AI: q.AI, Why: rndMentionWhy, Task: rndDeref(q.Task), Msg: *q.Msg, Jump: true}
			return Result{Items: []Item{it}, Retire: []int64{q.ID}}, nil
		}
	}
	return Result{}, &Refusal{"conflict", "no unacknowledged item from that person in that thread is queued"}
}

func rndJumpMatch(d Disk, q store.Item, author string, thread *string) bool {
	if q.Why != rndMentionWhy || q.Msg == nil {
		return false
	}
	msg, _ := d.Msg(*q.Msg)
	return msg.By == author && rndSameThread(msg.Thread, thread)
}

func rndSameThread(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// rndQuiet is the quiet-task timer (task-machine.md row 23): for each active
// task with no wait before step merge that has had no thread message for
// limits.idle_notice_minutes, it queues one quiet notice for the secretary per
// quiet period.
func rndQuiet(s *State, d Disk, now int64) Result {
	if s.Group.Secretary == "" {
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
	if !rndSilentFor(task, s.Group.Limits.IdleNoticeMinutes, now) {
		return Item{}, false
	}
	if rndAlreadyQuiet(s, task, queued) {
		return Item{}, false
	}
	_, fx, err := Step(task, Event{Kind: OnQuiet, By: ByHome, Now: now})
	if err != nil {
		return Item{}, false
	}
	return Item{AI: s.Group.Secretary, Why: itemNotice, Task: task.ID, Code: fx.Items[0].Code}, true
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
		if rndIsQuietNotice(q, taskID) {
			return true
		}
	}
	return false
}

func rndIsQuietNotice(q store.Item, taskID string) bool {
	if q.Why != itemNotice || !rndPtrIs(q.Task, taskID) {
		return false
	}
	return rndReadItemEnv(q.Env).Code == rndQuietCode
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
		return s.rndAfterPost(c, res)
	}
}

func (s *State) rndAfterPost(c Command, res Result) (Result, *Refusal) {
	msg, inThread := rndThreadMsg(res.Rows, string(c.Nonce))
	if !inThread {
		return res, nil
	}
	task := rndFindTask(s, res.Rows, *msg.Thread)
	if c.Who.AI == "" {
		return s.rndPersonPost(c, res, msg, task), nil
	}
	return s.rndAIPost(c, res, msg, task)
}

func (s *State) rndPersonPost(c Command, res Result, msg proto.MsgRow, task proto.TaskRow) Result {
	if msg.Kind != rndTextKind {
		return res
	}
	wasRounds := rndInRounds(task)
	queued := c.Disk.Queued()
	// Row 22 takes any person, admin or member alike.
	moved, fx, _ := Step(task, Event{
		Kind: OnText, By: ByPerson, Now: c.Now, Limits: s.Group.Limits, Pending: rndPending(c.Disk, task.ID),
	})
	res.Rows = rndPutTask(res.Rows, moved)
	if !wasRounds {
		return res
	}
	retire, released := rndRelease(queued, moved.ID)
	res.Retire = append(res.Retire, retire...)
	extra := append(rndReviewItems(fx.Items, moved.ID, queued), released...)
	res.Items = append(extra, res.Items...)
	return res
}

func (s *State) rndAIPost(c Command, res Result, msg proto.MsgRow, task proto.TaskRow) (Result, *Refusal) {
	if s.rndMentionsAI(msg, res.Items) {
		var refusal *Refusal
		task, res, refusal = s.rndCountRound(c, res, task)
		if refusal != nil {
			return Result{}, refusal
		}
	}
	if rndInRounds(task) {
		res = rndHoldMentions(c.Disk.Queued(), res, task.ID)
	}
	return res, nil
}

func (s *State) rndCountRound(c Command, res Result, task proto.TaskRow) (proto.TaskRow, Result, *Refusal) {
	// Row 21 counts mentions from working AIs and the secretary alike.
	moved, fx, _ := Step(task, Event{
		Kind: OnMention, By: ByAI, Member: c.Who.AI, Now: c.Now, Limits: s.Group.Limits,
	})
	res.Rows = rndPutTask(res.Rows, moved)
	if fx.System == "" {
		return moved, res, nil
	}
	sysMsg, refusal := s.rndRoundsLimitMsg(c, moved.ID)
	if refusal != nil {
		return proto.TaskRow{}, Result{}, refusal
	}
	res.Rows = append(res.Rows, RowOf(sysMsg))
	if s.Group.Secretary != "" {
		res.Items = append(res.Items, Item{AI: s.Group.Secretary, Why: itemNotice, Task: moved.ID, Msg: sysMsg.ID, Code: rndRoundsCode})
	}
	return moved, res, nil
}

func (s *State) rndRoundsLimitMsg(c Command, taskID string) (proto.MsgRow, *Refusal) {
	id := rndSystemID(c.Nonce)
	if _, taken := c.Disk.Msg(id); taken {
		return proto.MsgRow{}, &Refusal{"conflict", "a message with the derived id of rounds_limit exists"}
	}
	body := proto.RoundsLimitBody{
		SystemCode: proto.SystemCode{Code: rndRoundsCode},
		Task:       taskID,
		N:          s.Group.Limits.Rounds,
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id}, Group: s.ID, By: rndSystemBy,
		Kind: rndSystemKind, Timestamp: c.Now, Mentions: []string{}, Body: briefJSON(body),
	}, nil
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
		if it.Why == rndMentionWhy {
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
		if res.Items[i].Why == rndMentionWhy {
			res.Items[i].NotBefore = runNever
		}
	}
	res.Items = append(held, res.Items...)
	return res
}

func rndShouldHold(q store.Item, taskID string) bool {
	return q.Why == rndMentionWhy && rndPtrIs(q.Task, taskID) && q.NotBefore != runNever
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
	for _, r := range res.Rows {
		if moved, ok := r.Value().(proto.TaskRow); ok && rndInRounds(s.Tasks[moved.ID]) {
			retire, released := rndRelease(d.Queued(), moved.ID)
			res.Retire = append(res.Retire, retire...)
			res.Items = append(res.Items, released...)
		}
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
	return q.Why == rndMentionWhy && rndPtrIs(q.Task, taskID) && q.NotBefore == runNever
}

func rndRequeuedMention(q store.Item, taskID string, notBefore int64) Item {
	return Item{
		AI: q.AI, Why: rndMentionWhy, Task: taskID, Msg: rndDeref(q.Msg),
		Jump: rndReadItemEnv(q.Env).Jump, NotBefore: notBefore,
	}
}

func rndReviewItems(items []TaskItem, taskID string, queued []store.Item) []Item {
	var out []Item
	for _, it := range items {
		if !rndReviewQueued(queued, taskID, it.AI, it.Commit) {
			out = append(out, Item{AI: it.AI, Why: it.Why, Task: taskID, Commit: it.Commit})
		}
	}
	return out
}

func rndReviewQueued(queued []store.Item, taskID, ai, commit string) bool {
	for _, q := range queued {
		if rndSameReview(q, taskID, ai, commit) {
			return true
		}
	}
	return false
}

func rndSameReview(q store.Item, taskID, ai, commit string) bool {
	if q.Why != itemReview || q.AI != ai {
		return false
	}
	return rndPtrIs(q.Task, taskID) && rndPtrIs(q.Commit, commit)
}

func rndPending(d Disk, taskID string) []PendingCard {
	var pending []PendingCard
	for _, m := range d.Pending() {
		if !rndPtrIs(m.Thread, taskID) {
			continue
		}
		var recipients struct {
			Approvers []string `json:"approvers"`
			To        []string `json:"to"`
		}
		json.Unmarshal(m.Body, &recipients)
		who := append(recipients.Approvers, recipients.To...)
		pending = append(pending, PendingCard{Kind: m.Kind, Msg: m.ID, Who: who, Since: m.Timestamp})
	}
	return pending
}

func rndThreadMsg(rows []Row, id string) (proto.MsgRow, bool) {
	for _, r := range rows {
		if m, ok := r.Value().(proto.MsgRow); ok && m.ID == id {
			return m, m.Thread != nil
		}
	}
	return proto.MsgRow{}, false
}

func rndFindTask(s *State, rows []Row, taskID string) proto.TaskRow {
	for _, r := range rows {
		if t, ok := r.Value().(proto.TaskRow); ok && t.ID == taskID {
			return t
		}
	}
	return s.Tasks[taskID]
}

func rndPutTask(rows []Row, task proto.TaskRow) []Row {
	out := slices.Clone(rows)
	for i, r := range out {
		if t, ok := r.Value().(proto.TaskRow); ok && t.ID == task.ID {
			out[i] = RowOf(task)
			return out
		}
	}
	return append(out, RowOf(task))
}

func rndInRounds(task proto.TaskRow) bool {
	return task.Wait != nil && task.Wait.Kind == waitRounds
}

func rndPtrIs(p *string, want string) bool {
	return p != nil && *p == want
}

func rndDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type rndWireEnv struct {
	Payload string `json:"p"`
	Code    string `json:"code"`
	Jump    bool   `json:"jump"`
}

func rndReadItemEnv(raw json.RawMessage) rndWireEnv {
	var env rndWireEnv
	json.Unmarshal(raw, &env)
	if env.Payload != "" {
		json.Unmarshal([]byte(env.Payload), &env)
	}
	return env
}
