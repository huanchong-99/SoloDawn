package home

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const rndTestClock = 1763600000000

var (
	rndTestNow     = time.UnixMilli(rndTestClock)
	rndOwnerKey    = rndKey(1)
	rndHomeKey     = rndKey(2)
	rndHostKey     = rndKey(3)
	rndGuestKey    = rndKey(4)
	rndGroupID     = rndMakeID(10)
	rndOwnerMember = rndMakeID(11)
	rndGuestMember = rndMakeID(12)
	rndSecMember   = rndMakeID(13)
	rndAIMemberOne = rndMakeID(14)
	rndAIMemberTwo = rndMakeID(15)
	rndTaskOneID   = rndMakeID(20)
	rndTaskTwoID   = rndMakeID(21)
)

func rndKey(seed byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)))
}

func rndMakeID(seed byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 16))
}

func rndEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

type rndFakeStore struct {
	msgs     map[string]proto.MsgRow
	pending  []proto.MsgRow
	queued   []store.Item
	outcomes map[[2]string]store.Outcome
	nextID   int64
}

func rndNewFakeStore() *rndFakeStore {
	return &rndFakeStore{
		msgs:     map[string]proto.MsgRow{},
		outcomes: map[[2]string]store.Outcome{},
	}
}

func (f *rndFakeStore) Msg(id string) (proto.MsgRow, bool) {
	m, ok := f.msgs[id]
	return m, ok
}

func (f *rndFakeStore) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (f *rndFakeStore) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (f *rndFakeStore) Pending() []proto.MsgRow                   { return f.pending }
func (f *rndFakeStore) Queued() []store.Item                      { return f.queued }

func (f *rndFakeStore) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := f.outcomes[[2]string{signer, nonce}]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (f *rndFakeStore) PutReceipt(signer, nonce string, _ int64, out store.Outcome) error {
	f.outcomes[[2]string{signer, nonce}] = out
	return nil
}

func (f *rndFakeStore) commit(s *State, res Result) {
	f.storeMsgs(res.Rows)
	f.retireItems(res.Retire)
	f.appendItems(s, res.Items)
	s.Commit(res)
}

func (f *rndFakeStore) storeMsgs(rows []Row) {
	for _, r := range rows {
		if m, ok := r.Value().(proto.MsgRow); ok {
			f.msgs[m.ID] = m
		}
	}
}

func (f *rndFakeStore) retireItems(ids []int64) {
	if len(ids) == 0 {
		return
	}
	retired := map[int64]bool{}
	for _, id := range ids {
		retired[id] = true
	}
	var kept []store.Item
	for _, q := range f.queued {
		if !retired[q.ID] {
			kept = append(kept, q)
		}
	}
	f.queued = kept
}

func (f *rndFakeStore) appendItems(s *State, items []Item) {
	for _, it := range items {
		f.nextID++
		raw, _ := runJSON(it)
		f.queued = append(f.queued, store.Item{
			ID: f.nextID, Group: s.ID, AI: it.AI, Host: s.AIs[it.AI].Host,
			Why: it.Why, Msg: runText(it.Msg), Task: runText(it.Task), Commit: runText(it.Commit),
			Env: raw, NotBefore: it.NotBefore, Created: rndTestClock,
		})
	}
}

func rndLog(t *testing.T, first identity.Key, rest ...identity.Key) *identity.Log {
	t.Helper()
	l := &identity.Log{User: first.ID()}
	for i, dev := range append([]identity.Key{first}, rest...) {
		p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":"node","name":"dev"}`, first.ID(), i+1, dev.ID())
		if _, err := l.Apply(first.Sign(p)); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func rndTestState(t *testing.T) *State {
	t.Helper()
	s := NewState(rndGroupID, rndHomeKey.ID(), rndOwnerKey.ID())
	s.Logs[rndOwnerKey.ID()] = rndLog(t, rndOwnerKey, rndHomeKey, rndHostKey)
	s.Logs[rndGuestKey.ID()] = rndLog(t, rndGuestKey)
	s.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: rndGroupID},
		Home:      rndHomeKey.ID(), Secretary: rndSecMember, Forge: "none",
		Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: 1},
	}
	s.People[rndOwnerMember] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: rndOwnerMember}, Group: rndGroupID, Kind: "human", User: rndOwnerKey.ID()},
		Role:      "owner",
	}
	s.People[rndGuestMember] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: rndGuestMember}, Group: rndGroupID, Kind: "human", User: rndGuestKey.ID()},
		Role:      "member",
	}
	for _, id := range []string{rndSecMember, rndAIMemberOne, rndAIMemberTwo} {
		s.AIs[id] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: rndGroupID, Kind: "ai", User: rndOwnerKey.ID()},
			Host:      rndHostKey.ID(), Secretary: id == rndSecMember,
		}
	}
	s.Tasks[rndTaskOneID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: rndTaskOneID, Seq: 5},
		Group:     rndGroupID, Num: 1, Title: "Task one", Slug: "task-one", Origin: "brief",
		State: taskActive, Step: stepLocal, Commit: "c1", Assignee: rndAIMemberOne, Reviewer: rndAIMemberTwo,
		Pass: 90, Depends: []string{}, Created: rndTestClock - 5000, Stats: proto.TaskStats{Started: rndTestClock - 5000},
	}
	s.Seq, s.Num = 10, 1
	return s
}

// rndTestTable builds a command table with minimal post and task/resume
// handlers and wraps it with rndWrap, exercising the exact seam cmd_post.go,
// cmd_call.go and cmd_task.go register into.
func rndTestTable() Table {
	t := Table{
		"post/text":     rndStubPost,
		"post/progress": rndStubPost,
		"post/question": rndStubPost,
		"post/call":     rndStubPost,
		"post/report":   rndStubPost,
		"post/handback": rndStubPost,
		"task/resume":   rndStubResume,
		"jump":          rndJump,
	}
	rndWrap(t)
	return t
}

func rndStubPost(s *State, c Command) (Result, *Refusal) {
	var p proto.Post[json.RawMessage]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Kind != "text" && c.Who.AI == "" {
		return Result{}, &Refusal{"forbidden", "AI only"}
	}
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID,
		Thread: (*string)(p.Thread), By: c.Who.Member(), Kind: p.Kind,
		Timestamp: c.Timestamp, Text: p.Text, Mentions: briefIDs(p.Mentions), Body: p.Body, Env: &c.Env,
	}
	return Result{Rows: rndStubPostRows(s, msg), Items: rndStubPostItems(s, p.Kind, msg)}, nil
}

func rndStubPostRows(s *State, msg proto.MsgRow) []Row {
	rows := []Row{RowOf(msg)}
	if msg.Thread != nil {
		t := s.Tasks[*msg.Thread]
		t.Thread.Count++
		t.Thread.Last = &proto.ThreadMessage{By: msg.By, Kind: msg.Kind, Text: msg.Text, Timestamp: msg.Timestamp}
		rows = append(rows, RowOf(t))
	}
	return rows
}

func rndStubPostItems(s *State, kind string, msg proto.MsgRow) []Item {
	threadID := rndDeref(msg.Thread)
	switch kind {
	case "call":
		var body proto.CallBody
		json.Unmarshal(msg.Body, &body)
		return []Item{{AI: body.To, Why: rndMentionWhy, Task: threadID, Msg: msg.ID}}
	case "handback":
		return rndStubHandbackItems(s.Group.Secretary, threadID, msg.ID)
	case "text", "progress", "question":
		return rndStubMentionItems(s, threadID, msg)
	default:
		return nil
	}
}

func rndStubHandbackItems(sec, threadID, msgID string) []Item {
	if sec == "" {
		return nil
	}
	return []Item{{AI: sec, Why: itemNotice, Task: threadID, Msg: msgID, Code: "handback"}}
}

func rndStubMentionItems(s *State, threadID string, msg proto.MsgRow) []Item {
	var items []Item
	for _, m := range msg.Mentions {
		if _, ok := s.AIs[m]; ok {
			items = append(items, Item{AI: m, Why: rndMentionWhy, Task: threadID, Msg: msg.ID})
		}
	}
	return items
}

func rndStubResume(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskCommand
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, ok := s.Tasks[string(p.ID)]
	if !ok {
		return Result{}, &Refusal{"not_found", "no task"}
	}
	ev := Event{
		Kind: OnResume, By: rndStubSender(s, c.Who, task.Assignee),
		Now: c.Now, Limits: s.Group.Limits, Pending: rndPending(c.Disk, task.ID),
	}
	moved, fx, err := Step(task, ev)
	if err != nil {
		return Result{}, &Refusal{err.Error(), "refused"}
	}
	return Result{Rows: []Row{RowOf(moved)}, Items: rndReviewItems(fx.Items, moved.ID, c.Disk.Queued())}, nil
}

func rndStubSender(s *State, w Who, assignee string) Sender {
	if s.Secretary(w) {
		return BySecretary
	}
	if s.Admin(w) {
		return ByAdmin
	}
	if s.Hosts(w, assignee) {
		return ByHost
	}
	return ByPerson
}

func rndSignPost(by identity.Key, n byte, as, kind, thread, text string, mentions []string, body string) proto.Envelope {
	th := "null"
	if thread != "" {
		th = fmt.Sprintf("%q", thread)
	}
	asPart := ""
	if as != "" {
		asPart = fmt.Sprintf(`,"as":%q`, as)
	}
	mJSON, _ := json.Marshal(mentions)
	if body == "" {
		body = `{}`
	}
	payload := fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q%s,"kind":%q,"thread":%s,"main":false,"text":%q,"mentions":%s,"re":null,"body":%s}`,
		rndMakeID(n), rndTestClock-100, rndGroupID, asPart, kind, th, text, mJSON, body)
	return by.Sign(payload)
}

func rndSignResume(by identity.Key, n byte, as, taskID string) proto.Envelope {
	asPart := ""
	if as != "" {
		asPart = fmt.Sprintf(`,"as":%q`, as)
	}
	payload := fmt.Sprintf(`{"t":"task","op":"resume","n":%q,"ts":%d,"g":%q%s,"id":%q}`,
		rndMakeID(n), rndTestClock-100, rndGroupID, asPart, taskID)
	return by.Sign(payload)
}

func rndApply(t *testing.T, table Table, s *State, f *rndFakeStore, by identity.Key, env proto.Envelope) Result {
	t.Helper()
	res, err := table.Apply(s, f, f, by.ID(), env, rndTestNow)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// A5.8: ten AI-to-AI exchanges in a thread pause the task with wait rounds,
// write rounds_limit in the main line, notify the secretary and hold thread
// mentions; a person's thread text and an admin's task resume each release them.
func TestRoundsTenRoundsHoldAndBothResumesRelease(t *testing.T) {
	for _, how := range []string{"person_text", "admin_resume"} {
		t.Run(how, func(t *testing.T) {
			s, f, table := rndTestState(t), rndNewFakeStore(), rndTestTable()

			// Rounds 1 to 8 via question/progress, round 9 via secretary text, round 10 via call.
			for round := 1; round <= 8; round++ {
				sender, target := rndAIMemberOne, rndAIMemberTwo
				if round%2 == 0 {
					sender, target = rndAIMemberTwo, rndAIMemberOne
				}
				res := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, byte(30+round), sender, "question", rndTaskOneID, "q", []string{target}, ""))
				rndEqual(t, res.Answer, store.Outcome{OK: true})
				rndEqual(t, res.Items, []Item{{AI: target, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(byte(30 + round)), NotBefore: 0}})
				f.commit(s, res)
				// Simulate host acknowledging rounds 1 to 8; keep round 9 unacknowledged to verify pre-existing queue hold.
				f.queued = nil
				rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, round)
			}

			// Round 9: secretary mentions ai-one; leave unacknowledged in f.queued.
			r9 := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 39, rndSecMember, "text", rndTaskOneID, "check", []string{rndAIMemberOne}, ""))
			rndEqual(t, len(r9.Rows), 2)
			rndEqual(t, r9.Items[0].NotBefore, int64(0))
			f.commit(s, r9)
			rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 9)
			rndEqual(t, s.Tasks[rndTaskOneID].State, taskActive)

			// Also place a main-line mention, another task's mention, and an assign item in f.queued to pin rndShouldHold.
			mainMsg, t1ID, otherTask := rndMakeID(90), rndTaskOneID, rndTaskTwoID
			f.queued = append(f.queued,
				store.Item{ID: 50, AI: rndAIMemberTwo, Why: "mention", Msg: &mainMsg, NotBefore: 0},
				store.Item{ID: 51, AI: rndAIMemberTwo, Why: "mention", Task: &otherTask, Msg: &mainMsg, NotBefore: 0},
				store.Item{ID: 52, AI: rndAIMemberOne, Why: "assign", Task: &t1ID, NotBefore: 0},
			)

			// Round 10: ai-one posts a call to ai-two with empty mentions[] (counts via mention item, R-114).
			callBody := fmt.Sprintf(`{"to":%q,"depth":1,"wait":true}`, rndAIMemberTwo)
			r10 := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 40, rndAIMemberOne, "call", rndTaskOneID, "ask", []string{}, callBody))
			rndEqual(t, r10.Answer, store.Outcome{OK: true})

			sysID := rndSystemID(proto.ID(rndMakeID(40)))
			// Round 9's unacknowledged mention (ID 9) was retired and re-queued at runNever; IDs 50, 51, 52 stayed untouched.
			rndEqual(t, r10.Retire, []int64{9})
			rndEqual(t, r10.Items, []Item{
				{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(39), NotBefore: runNever},
				{AI: rndAIMemberTwo, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(40), NotBefore: runNever},
				{AI: rndSecMember, Why: "notice", Task: rndTaskOneID, Msg: sysID, Code: "rounds_limit", NotBefore: 0},
			})
			f.commit(s, r10)

			paused := s.Tasks[rndTaskOneID]
			rndEqual(t, []any{paused.State, paused.Wait.Kind, paused.Wait.Since, paused.Thread.Rounds, paused.Thread.Count, paused.Thread.Last != nil},
				[]any{"paused", "rounds", int64(rndTestClock), 10, 10, true})
			sysRow := f.msgs[sysID]
			rawSys, err := json.Marshal(sysRow)
			rndEqual(t, err, nil)
			var decodedSys proto.MsgRow
			rndEqual(t, proto.Decode(rawSys, &decodedSys), nil)
			rndEqual(t, proto.UnexpectedNulls(rawSys), []string{"/re", "/state", "/thread"})
			rndEqual(t, []any{sysRow.Group, sysRow.Timestamp, sysRow.Thread == nil, sysRow.Main, sysRow.By, sysRow.Kind, string(sysRow.Body)},
				[]any{rndGroupID, int64(rndTestClock), true, false, "system", "system", fmt.Sprintf(`{"code":"rounds_limit","task":%q,"n":10}`, rndTaskOneID)})

			// Round 11 while already paused at rounds: increments rounds to 11, writes no second rounds_limit,
			// holds round 11's mention, and does not re-retire already-held items.
			r11 := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 41, rndAIMemberTwo, "question", rndTaskOneID, "more", []string{rndAIMemberOne}, ""))
			rndEqual(t, len(r11.Retire), 0)
			rndEqual(t, r11.Items, []Item{{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(41), NotBefore: runNever}})
			f.commit(s, r11)
			rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 11)

			// Secretary cannot end the round limit via task/resume.
			refused := rndApply(t, table, s, f, rndHostKey, rndSignResume(rndHostKey, 42, rndSecMember, rndTaskOneID))
			rndEqual(t, refused.Answer, store.Outcome{Code: "forbidden"})

			// Add a held mention on another task and a held non-mention on this task to pin rndIsHeld.
			f.queued = append(f.queued,
				store.Item{ID: 60, AI: rndAIMemberTwo, Why: "mention", Task: &otherTask, Msg: &mainMsg, NotBefore: runNever},
				store.Item{ID: 61, AI: rndAIMemberOne, Why: "review", Task: &paused.ID, NotBefore: runNever},
			)

			if how == "person_text" {
				// Non-admin member's text in the thread resets rounds, activates the task, and releases the 3 held mentions before the new mention.
				resumePost := rndApply(t, table, s, f, rndGuestKey, rndSignPost(rndGuestKey, 43, "", "text", rndTaskOneID, "go ahead", []string{rndAIMemberTwo}, ""))
				rndEqual(t, resumePost.Answer, store.Outcome{OK: true})
				rndEqual(t, resumePost.Retire, []int64{10, 11, 13})
				rndEqual(t, resumePost.Items, []Item{
					{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(39), NotBefore: 0},
					{AI: rndAIMemberTwo, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(40), NotBefore: 0},
					{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(41), NotBefore: 0},
					{AI: rndAIMemberTwo, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(43), NotBefore: 0},
				})
				f.commit(s, resumePost)
			} else {
				// Admin's task/resume resets rounds, activates the task, queues resume and releases the 3 held mentions.
				resumed := rndApply(t, table, s, f, rndOwnerKey, rndSignResume(rndOwnerKey, 44, "", rndTaskOneID))
				rndEqual(t, resumed.Answer, store.Outcome{OK: true})
				rndEqual(t, resumed.Retire, []int64{10, 11, 13})
				rndEqual(t, resumed.Items, []Item{
					{AI: rndAIMemberOne, Why: "resume", Task: rndTaskOneID},
					{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(39), NotBefore: 0},
					{AI: rndAIMemberTwo, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(40), NotBefore: 0},
					{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(41), NotBefore: 0},
				})
				f.commit(s, resumed)
			}
			after := s.Tasks[rndTaskOneID]
			rndEqual(t, []any{after.State, after.Wait == nil, after.Thread.Rounds}, []any{"active", true, 0})
		})
	}
}

func TestRoundsEdgeCasesAndMutantPins(t *testing.T) {
	t.Run("non-AI mentions, main-line posts, step merge, no secretary, and system id collision", func(t *testing.T) {
		s, f, table := rndTestState(t), rndNewFakeStore(), rndTestTable()

		// Main-line AI post mentioning an AI does not touch task rounds.
		mainRes := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 50, rndAIMemberOne, "question", "", "main", []string{rndAIMemberTwo}, ""))
		f.commit(s, mainRes)
		rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 0)

		// Thread AI posts that mention only a human, or handback (notice item, not mention), count no round.
		humanMention := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 51, rndAIMemberOne, "progress", rndTaskOneID, "hi owner", []string{rndOwnerMember}, ""))
		f.commit(s, humanMention)
		handback := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 52, rndAIMemberOne, "handback", rndTaskOneID, "", []string{}, `{"files":[],"commits":[],"added":0,"removed":0}`))
		f.commit(s, handback)
		rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 0)

		// Report mentioning an AI in Mentions (with no mention item queued by report itself) still counts a round.
		report := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 53, rndAIMemberOne, "report", rndTaskOneID, "", []string{rndAIMemberTwo}, `{"commit":"c1","files":1,"added":2,"removed":0}`))
		f.commit(s, report)
		rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 1)

		// At step merge, reaching limits.rounds increments Thread.Rounds without pausing or writing rounds_limit.
		mergeTask := s.Tasks[rndTaskOneID]
		mergeTask.Step, mergeTask.Thread.Rounds = stepMerge, 9
		s.Tasks[rndTaskOneID] = mergeTask
		atMerge := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 54, rndAIMemberOne, "question", rndTaskOneID, "q", []string{rndAIMemberTwo}, ""))
		rndEqual(t, atMerge.Items, []Item{{AI: rndAIMemberTwo, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(54), NotBefore: 0}})
		f.commit(s, atMerge)
		rndEqual(t, []any{s.Tasks[rndTaskOneID].State, s.Tasks[rndTaskOneID].Step, s.Tasks[rndTaskOneID].Thread.Rounds}, []any{"active", "merge", 10})

		// Collision on derived rounds_limit ID refuses the post with conflict.
		activeAgain := s.Tasks[rndTaskOneID]
		activeAgain.Step, activeAgain.Thread.Rounds = stepLocal, 9
		s.Tasks[rndTaskOneID] = activeAgain
		collidingNonce := byte(55)
		f.msgs[rndSystemID(proto.ID(rndMakeID(collidingNonce)))] = proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: rndSystemID(proto.ID(rndMakeID(collidingNonce)))}, Kind: "text"}
		collided := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, collidingNonce, rndAIMemberOne, "question", rndTaskOneID, "q", []string{rndAIMemberTwo}, ""))
		rndEqual(t, collided.Answer, store.Outcome{Code: "conflict"})

		// Without a secretary, reaching limits.rounds writes rounds_limit and holds the mention without queuing a notice.
		f.queued = nil
		s.Group.Secretary = ""
		noSec := rndApply(t, table, s, f, rndHostKey, rndSignPost(rndHostKey, 56, rndAIMemberOne, "question", rndTaskOneID, "q", []string{rndAIMemberTwo}, ""))
		rndEqual(t, noSec.Answer, store.Outcome{OK: true})
		rndEqual(t, noSec.Items, []Item{{AI: rndAIMemberTwo, Why: "mention", Task: rndTaskOneID, Msg: rndMakeID(56), NotBefore: runNever}})
	})

	t.Run("person post at step review deduplicates review item, restores pending cards, and preserves Jump flag", func(t *testing.T) {
		s, f, table := rndTestState(t), rndNewFakeStore(), rndTestTable()
		t1 := s.Tasks[rndTaskOneID]
		t1.State, t1.Step, t1.Thread.Rounds = taskPaused, stepReview, 10
		t1.Wait = &proto.TaskWait{Kind: waitRounds, Since: rndTestClock - 2000}
		s.Tasks[rndTaskOneID] = t1

		// Pending cards: a brief (nil thread), another task's card, a decision card, and an approval card.
		t1ID, t2ID := rndTaskOneID, rndTaskTwoID
		f.pending = []proto.MsgRow{
			{RowHeader: proto.RowHeader{Type: "msg", ID: rndMakeID(70)}, Kind: "brief", Body: json.RawMessage(`{}`)},
			{RowHeader: proto.RowHeader{Type: "msg", ID: rndMakeID(71)}, Thread: &t2ID, Kind: "approval", Timestamp: 100, Body: json.RawMessage(`{"approvers":["u2"]}`)},
			{RowHeader: proto.RowHeader{Type: "msg", ID: rndMakeID(72)}, Thread: &t1ID, Kind: "decision", Timestamp: 200, Body: json.RawMessage(`{"to":["u-dec"]}`)},
			{RowHeader: proto.RowHeader{Type: "msg", ID: rndMakeID(73)}, Thread: &t1ID, Kind: "approval", Timestamp: 300, Body: json.RawMessage(`{"approvers":["u-app"]}`)},
		}

		// Non-matching review items (wrong why, wrong AI, nil/other task, nil/other commit) do not suppress review queueing;
		// held mention carries Jump: true in signed-envelope form.
		c1, cOld := "c1", "c0"
		msgHeld := rndMakeID(74)
		f.queued = []store.Item{
			{ID: 1, AI: rndAIMemberTwo, Why: "mention", Task: &t1ID, Commit: &c1},
			{ID: 2, AI: rndAIMemberOne, Why: "review", Task: &t1ID, Commit: &c1},
			{ID: 3, AI: rndAIMemberTwo, Why: "review", Task: nil, Commit: &c1},
			{ID: 4, AI: rndAIMemberTwo, Why: "review", Task: &t2ID, Commit: &c1},
			{ID: 5, AI: rndAIMemberTwo, Why: "review", Task: &t1ID, Commit: nil},
			{ID: 6, AI: rndAIMemberTwo, Why: "review", Task: &t1ID, Commit: &cOld},
			{ID: 7, AI: rndAIMemberOne, Why: "mention", Task: &t1ID, Msg: &msgHeld, NotBefore: runNever,
				Env: json.RawMessage(`{"p":"{\"t\":\"item\",\"why\":\"mention\",\"jump\":true}","k":"k","s":"s"}`)},
		}

		res := rndApply(t, table, s, f, rndOwnerKey, rndSignPost(rndOwnerKey, 75, "", "text", rndTaskOneID, "continue", []string{}, ""))
		rndEqual(t, res.Retire, []int64{7})
		rndEqual(t, res.Items, []Item{
			{AI: rndAIMemberTwo, Why: "review", Task: rndTaskOneID, Commit: "c1"},
			{AI: rndAIMemberOne, Why: "mention", Task: rndTaskOneID, Msg: msgHeld, Jump: true, NotBefore: 0},
		})
		f.commit(s, res)
		rndEqual(t, s.Tasks[rndTaskOneID].Wait, &proto.TaskWait{Kind: "approval", Msg: rndMakeID(73), Who: []string{"u-app"}, Since: 300})

		// When a matching review item IS already queued, person text on rounds does not queue a second review.
		t1 = s.Tasks[rndTaskOneID]
		t1.State, t1.Wait, t1.Thread.Rounds = taskPaused, &proto.TaskWait{Kind: waitRounds, Since: rndTestClock}, 10
		s.Tasks[rndTaskOneID] = t1
		f.queued = []store.Item{{ID: 8, AI: rndAIMemberTwo, Why: "review", Task: &t1ID, Commit: &c1}}
		resDup := rndApply(t, table, s, f, rndOwnerKey, rndSignPost(rndOwnerKey, 76, "", "text", rndTaskOneID, "again", []string{}, ""))
		rndEqual(t, len(resDup.Items), 0)
		f.commit(s, resDup)

		// Person text on an active (non-rounds) task resets rounds without releasing anything.
		t1 = s.Tasks[rndTaskOneID]
		t1.Thread.Rounds = 4
		s.Tasks[rndTaskOneID] = t1
		resActive := rndApply(t, table, s, f, rndOwnerKey, rndSignPost(rndOwnerKey, 77, "", "text", rndTaskOneID, "reset", []string{}, ""))
		f.commit(s, resActive)
		rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 0)
	})

	t.Run("helper row shapes and non-rounds resume pass through", func(t *testing.T) {
		s, f := rndTestState(t), rndNewFakeStore()
		// Table without task/resume exercises rndWrap's false branch for task/resume,
		// a custom post returning no MsgRow, a custom non-text person post, and a post omitting TaskRow in Rows.
		t1ID := rndTaskOneID
		s.Tasks[rndTaskTwoID] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: rndTaskTwoID}, Group: rndGroupID, Num: 2, State: taskActive, Step: stepLocal, Assignee: rndAIMemberTwo}
		custom := Table{
			"post/empty": func(*State, Command) (Result, *Refusal) {
				return Result{Rows: []Row{RowOf(s.Tasks[rndTaskTwoID])}}, nil
			},
			"post/custom": func(_ *State, c Command) (Result, *Refusal) {
				otherMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: rndMakeID(98)}, Kind: "text"}
				ownMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Thread: &t1ID, By: c.Who.Member(), Kind: "custom"}
				return Result{Rows: []Row{RowOf(s.Tasks[rndTaskTwoID]), RowOf(otherMsg), RowOf(ownMsg)}}, nil
			},
			"post/notask": func(_ *State, c Command) (Result, *Refusal) {
				ownMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Thread: &t1ID, By: c.Who.Member(), Kind: "text", Mentions: []string{rndAIMemberTwo}}
				return Result{Rows: []Row{RowOf(s.Tasks[rndTaskTwoID]), RowOf(ownMsg)}, Items: []Item{{AI: rndAIMemberTwo, Why: "mention", Task: t1ID, Msg: ownMsg.ID}}}, nil
			},
		}
		rndWrap(custom)

		rEmpty := rndApply(t, custom, s, f, rndOwnerKey, rndOwnerKey.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"empty"}`, rndMakeID(80), rndTestClock, rndGroupID)))
		rndEqual(t, len(rEmpty.Rows), 1)

		rCustom := rndApply(t, custom, s, f, rndOwnerKey, rndOwnerKey.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"custom"}`, rndMakeID(81), rndTestClock, rndGroupID)))
		rndEqual(t, len(rCustom.Rows), 3)

		rNoTask := rndApply(t, custom, s, f, rndHostKey, rndHostKey.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"as":%q,"kind":"notask"}`, rndMakeID(82), rndTestClock, rndGroupID, rndAIMemberOne)))
		rndEqual(t, len(rNoTask.Rows), 3)
		f.commit(s, rNoTask)
		rndEqual(t, s.Tasks[rndTaskOneID].Thread.Rounds, 1)

		// Resuming a manual-paused task does not release held items of rounds, and skips non-TaskRow entries in Rows.
		manual := s.Tasks[rndTaskOneID]
		manual.State, manual.Wait = taskPaused, &proto.TaskWait{Kind: waitManual, Since: rndTestClock}
		s.Tasks[rndTaskOneID] = manual
		msgID := rndMakeID(83)
		f.queued = []store.Item{{ID: 99, AI: rndAIMemberTwo, Why: "mention", Task: &t1ID, Msg: &msgID, NotBefore: runNever}}
		withResume := Table{
			"task/resume": func(*State, Command) (Result, *Refusal) {
				moved := manual
				moved.State, moved.Wait = taskActive, nil
				note := proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: rndMakeID(84)}}
				return Result{Rows: []Row{RowOf(note), RowOf(moved)}}, nil
			},
		}
		rndWrap(withResume)
		rManual := rndApply(t, withResume, s, f, rndOwnerKey, rndSignResume(rndOwnerKey, 85, "", rndTaskOneID))
		rndEqual(t, len(rManual.Retire)+len(rManual.Items), 0)
	})
}
