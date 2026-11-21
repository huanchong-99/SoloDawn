package home_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The working AIs of these tests, on the owner's laptop, and a person who is
// a member without being an admin. The secretary is aiID.
var briefA, briefB, briefC, briefGuest = id(20), id(21), id(22), id(23)

// The times the signers wrote into the post and the act; the home's clock is clock.
const briefPostTS, briefActTS = clock - 500, clock - 1000

// briefState is a group with a secretary, three working AIs and two people,
// after ten rows and four tasks.
func briefState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Group.Pass, s.Group.PassFoundation, s.Num = 85, 70, 4
	for _, member := range []string{briefA, briefB, briefC} {
		s.AIs[member] = ai(member, laptop)
	}
	s.People[briefGuest] = person(briefGuest, guest, "member")
	return s
}

// briefBody is a brief of two tasks, the second depending on the first.
func briefBody() proto.BriefBody {
	return proto.BriefBody{
		Title: "Show <paths> & relays", Goal: "People see the path.", Criteria: []string{"A change shows within 1 s."},
		Scope: proto.BriefScope{In: []string{"node events"}, Out: []string{}},
		Tasks: []proto.BriefTask{
			{Title: "Report path changes", Assignee: briefA, Reviewer: briefB, Foundation: true, Depends: []int{}},
			{Title: "Show the path", Assignee: briefB, Reviewer: briefA, Depends: []int{0}},
		},
		Pass: 95, Requester: ownerID,
	}
}

// briefPost is the secretary's post of a brief, as its host signs it.
func briefPost(n byte, body proto.BriefBody) proto.Post[proto.BriefBody] {
	return proto.Post[proto.BriefBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: briefPostTS, Group: proto.ID(groupID), As: proto.ID(aiID)},
		Kind:        "brief", Mentions: []proto.ID{}, Body: body,
	}
}

func briefSign(by identity.Key, payload any) proto.Envelope {
	p, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return by.Sign(string(p))
}

// briefAct is an act on a message, signed by a device; more holds further members.
func briefAct(by identity.Key, n byte, action, msg, more string) proto.Envelope {
	return by.Sign(fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":%q%s}`, id(n), briefActTS, groupID, msg, action, more))
}

// briefApply runs a command through the table of the node.
func briefApply(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, env proto.Envelope) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, by.ID(), env, now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// briefRows are the values of the rows of a result.
func briefRows(res home.Result) []any {
	rows := []any{}
	for _, r := range res.Rows {
		rows = append(rows, r.Value())
	}
	return rows
}

// briefRaw encodes a body as the home writes it, without HTML escaping.
func briefRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// briefTaskID is the id the home gives the task of an entry of a brief that an act confirmed.
func briefTaskID(act byte, entry int) string {
	h := sha256.Sum256(fmt.Appendf(nil, "SoloDawn brief task %s %d", id(act), entry))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

func briefRefused(t *testing.T, res home.Result, code string) {
	t.Helper()
	if res.Answer.Code != code || res.Msg == "" || len(res.Rows) != 0 || len(res.Items) != 0 {
		t.Fatalf("answered %+v (%q) with %d rows and %d items; want %s", res.Answer, res.Msg, len(res.Rows), len(res.Items), code)
	}
}

// briefCardRow is the row of the brief that a post of body with nonce n writes, numbered at seq.
func briefCardRow(t *testing.T, n byte, body proto.BriefBody, env proto.Envelope, seq int64) proto.MsgRow {
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(n), Seq: seq}, Pos: seq, Group: groupID, By: aiID, Kind: "brief",
		Timestamp: briefPostTS, Mentions: []string{}, Body: briefRaw(t, body), State: &proto.CardState{Status: "pending"}, Env: &env,
	}
}

// briefPosted posts the brief of the tests and keeps it on disk.
func briefPosted(t *testing.T, s *home.State, body proto.BriefBody) (proto.MsgRow, disk) {
	t.Helper()
	res := briefApply(t, s, disk{}, receipts{}, laptop, briefSign(laptop, briefPost(40, body)))
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	card := res.Rows[0].Value().(proto.MsgRow)
	return card, disk{card.ID: card}
}

// A5.2, home half: a brief waits for its confirmation; it creates no task and queues no item.
func TestBriefPostWritesAPendingCard(t *testing.T) {
	s := briefState(t)
	post := briefPost(40, briefBody())
	post.Text, post.Mentions, post.Re = "Here is the plan.", []proto.ID{proto.ID(briefA)}, (*proto.ID)(&ownerID)
	env := briefSign(laptop, post)
	res := briefApply(t, s, disk{}, receipts{}, laptop, env)
	card := briefCardRow(t, 40, briefBody(), env, 11)
	card.Text, card.Mentions, card.Re = "Here is the plan.", []string{briefA}, &ownerID
	equal(t, res, home.Result{Rows: res.Rows, Answer: store.Outcome{OK: true}})
	equal(t, briefRows(res), []any{card})
}

// A brief may be asked for by an AI member, and depend on several earlier entries.
func TestBriefPostTakesAnAIRequesterAndSeveralDependencies(t *testing.T) {
	s := briefState(t)
	body := briefBody()
	body.Requester = briefC
	body.Tasks = append(body.Tasks, proto.BriefTask{Title: "Third", Assignee: briefC, Reviewer: briefA, Depends: []int{0, 1}})
	res := briefApply(t, s, disk{}, receipts{}, laptop, briefSign(laptop, briefPost(40, body)))
	equal(t, res.Answer, store.Outcome{OK: true})
}

// A brief that replaces a pending one, or one sent back, supersedes it; the
// replaced brief keeps what its state held.
func TestBriefPostSupersedesTheBriefItReplaces(t *testing.T) {
	note, by, at := "Include the relay.", ownerID, int64(clock-9000)
	act := briefAct(browser, 41, "revise", id(39), `,"text":"Include the relay."`)
	for name, state := range map[string]proto.CardState{
		"pending": {Status: "pending"},
		"revise":  {Status: "revise", By: &by, At: &at, Note: &note, Act: &act},
	} {
		t.Run(name, func(t *testing.T) {
			s := briefState(t)
			old := briefCardRow(t, 39, briefBody(), proto.Envelope{}, 7)
			old.State = &state
			body := briefBody()
			replaces := id(39)
			body.Replaces = &replaces
			env := briefSign(laptop, briefPost(40, body))
			res := briefApply(t, s, disk{id(39): old}, receipts{}, laptop, env)
			equal(t, res.Answer, store.Outcome{OK: true})
			superseded := state
			superseded.Status = "superseded"
			old.Seq, old.State = 12, &superseded
			equal(t, briefRows(res), []any{briefCardRow(t, 40, body, env, 11), old})
		})
	}
}

func TestBriefPostRefusals(t *testing.T) {
	text := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(39)}, Kind: "text"}
	confirmed := briefCardRow(t, 39, briefBody(), proto.Envelope{}, 7)
	confirmed.State = &proto.CardState{Status: "confirmed"}
	approval := briefCardRow(t, 39, briefBody(), proto.Envelope{}, 7)
	approval.Kind = "approval"
	replacing := func(b *proto.BriefBody) { r := id(39); b.Replaces = &r }
	task := func(i int, change func(*proto.BriefTask)) func(*proto.BriefBody) {
		return func(b *proto.BriefBody) { change(&b.Tasks[i]) }
	}
	third := func(depends ...int) func(*proto.BriefBody) {
		return func(b *proto.BriefBody) {
			b.Tasks = append(b.Tasks, proto.BriefTask{Title: "Third", Assignee: briefC, Reviewer: briefA, Depends: depends})
		}
	}
	for name, c := range map[string]struct {
		by    identity.Key
		post  func(*proto.Post[proto.BriefBody])
		body  func(*proto.BriefBody)
		raw   func(string) string
		state func(*home.State)
		disk  disk
		code  string
	}{
		"by a person":                    {by: browser, post: func(p *proto.Post[proto.BriefBody]) { p.As = "" }, code: "forbidden"},
		"by a working AI":                {post: func(p *proto.Post[proto.BriefBody]) { p.As = proto.ID(briefA) }, code: "forbidden"},
		"in a group without a secretary": {state: func(s *home.State) { s.Group.Secretary = "" }, code: "forbidden"},
		"without a requester":            {raw: func(p string) string { return strings.Replace(p, `"requester"`, `"asker"`, 1) }, code: "bad_request"},
		"in a thread": {post: func(p *proto.Post[proto.BriefBody]) { th := proto.ID(id(30)); p.Thread = &th },
			code: "bad_request"},
		"without a criterion":           {body: func(b *proto.BriefBody) { b.Criteria = []string{} }, code: "bad_request"},
		"without a task":                {body: func(b *proto.BriefBody) { b.Tasks = []proto.BriefTask{} }, code: "bad_request"},
		"depending on itself":           {body: task(1, func(e *proto.BriefTask) { e.Depends = []int{1} }), code: "bad_request"},
		"depending on a later entry":    {body: task(0, func(e *proto.BriefTask) { e.Depends = []int{1} }), code: "bad_request"},
		"depending on no entry":         {body: task(1, func(e *proto.BriefTask) { e.Depends = []int{-1} }), code: "bad_request"},
		"depending on an entry twice":   {body: third(0, 0), code: "bad_request"},
		"depending on two, then twice":  {body: third(1, 0, 1), code: "bad_request"},
		"with one AI in both roles":     {body: task(1, func(e *proto.BriefTask) { e.Reviewer = briefB }), code: "bad_request"},
		"for a requester who is gone":   {body: func(b *proto.BriefBody) { b.Requester = id(30) }, code: "conflict"},
		"for a person as assignee":      {body: task(0, func(e *proto.BriefTask) { e.Assignee = ownerID }), code: "conflict"},
		"for the secretary as assignee": {body: task(0, func(e *proto.BriefTask) { e.Assignee = aiID }), code: "conflict"},
		"for no AI as reviewer":         {body: task(1, func(e *proto.BriefTask) { e.Reviewer = id(30) }), code: "conflict"},
		"for the secretary as reviewer": {body: task(1, func(e *proto.BriefTask) { e.Reviewer = aiID }), code: "conflict"},
		"with the id of a message":      {disk: disk{id(40): text}, code: "conflict"},
		"replacing nothing":             {body: replacing, code: "conflict"},
		"replacing a text message":      {body: replacing, disk: disk{id(39): text}, code: "conflict"},
		"replacing an approval card":    {body: replacing, disk: disk{id(39): approval}, code: "conflict"},
		"replacing a confirmed brief":   {body: replacing, disk: disk{id(39): confirmed}, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := briefState(t)
			if c.state != nil {
				c.state(s)
			}
			body := briefBody()
			if c.body != nil {
				c.body(&body)
			}
			post := briefPost(40, body)
			if c.post != nil {
				c.post(&post)
			}
			by := laptop
			if c.by != nil {
				by = c.by
			}
			payload, _ := json.Marshal(post)
			if c.raw != nil {
				payload = []byte(c.raw(string(payload)))
			}
			briefRefused(t, briefApply(t, s, c.disk, receipts{}, by, by.Sign(string(payload))), c.code)
		})
	}
}

// The rows of a confirmed brief: the brief, then each task and its task message.
func briefConfirmedRows(t *testing.T, card proto.MsgRow, act proto.Envelope, by string, n byte, from int64) []any {
	t.Helper()
	at := int64(clock)
	card.Seq, card.State = from, &proto.CardState{Status: "confirmed", By: &by, At: &at, Act: &act}
	one, two := briefTaskID(n, 0), briefTaskID(n, 1)
	task := func(id string, seq int64, num int, entry proto.BriefTask, slug string, pass int, depends []string) proto.TaskRow {
		return proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: id, Seq: seq}, Group: groupID, Num: num, Title: entry.Title,
			Slug: proto.Slug(slug), Origin: "brief", Brief: card.ID, State: "active", Step: "branch",
			Assignee: entry.Assignee, Reviewer: entry.Reviewer, Foundation: entry.Foundation, Pass: pass, Depends: depends,
			Created: clock, Stats: proto.TaskStats{Started: clock},
		}
	}
	msg := func(task string, seq int64) proto.MsgRow {
		return proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: task, Seq: seq}, Pos: seq, Group: groupID, By: "system", Kind: "task",
			Timestamp: clock, Mentions: []string{}, Body: json.RawMessage(`{"task":"` + task + `"}`),
		}
	}
	entries := briefBody().Tasks
	queued := task(two, from+3, 6, entries[1], "show-the-path", 95, []string{one})
	queued.State, queued.Wait = "queued", &proto.TaskWait{Kind: "depends", Since: clock}
	return []any{card, task(one, from+1, 5, entries[0], "report-path-changes", 70, []string{}), msg(one, from+2), queued, msg(two, from+4)}
}

// Row 1: confirming creates every task of the brief through the task
// machine. The first starts and its assignee gets it; the second waits for
// the first. The secretary is told.
func TestBriefConfirmCreatesTheTasks(t *testing.T) {
	s := briefState(t)
	card, d := briefPosted(t, s, briefBody())
	act := briefAct(browser, 50, "confirm", card.ID, "")
	res := briefApply(t, s, d, receipts{}, browser, act)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, briefRows(res), briefConfirmedRows(t, card, act, ownerID, 50, 12))
	one := briefTaskID(50, 0)
	equal(t, res.Items, []home.Item{
		{AI: briefA, Why: "assign", Task: one, Msg: one},
		{AI: aiID, Why: "confirmed", Msg: card.ID},
	})
}

// A5.15: a confirm sent twice with the same nonce creates its tasks once and
// is answered ok twice.
func TestBriefConfirmTwiceCreatesTheTasksOnce(t *testing.T) {
	s := briefState(t)
	card, d := briefPosted(t, s, briefBody())
	r := receipts{}
	act := briefAct(browser, 50, "confirm", card.ID, "")
	first := briefApply(t, s, d, r, browser, act)
	s.Commit(first)
	confirmed := first.Rows[0].Value().(proto.MsgRow)
	d[card.ID] = confirmed
	second := briefApply(t, s, d, r, browser, act)
	equal(t, second, home.Result{Answer: store.Outcome{OK: true}})
	equal(t, first.Frame(1), second.Frame(1))
	equal(t, len(s.Tasks), 2)
	equal(t, s.Num, 6)
	// Another confirm of the same brief, under a new nonce, finds it decided.
	briefRefused(t, briefApply(t, s, d, r, browser, briefAct(browser, 51, "confirm", card.ID, "")), "conflict")
}

// A person who is not an admin confirms a brief they asked for, an admin one
// that someone else asked for, and a group without a secretary is told nothing.
func TestBriefConfirmByTheRequesterOrAnAdmin(t *testing.T) {
	t.Run("the requester", func(t *testing.T) {
		s := briefState(t)
		body := briefBody()
		body.Requester = briefGuest
		card, d := briefPosted(t, s, body)
		res := briefApply(t, s, d, receipts{}, guest, briefAct(guest, 50, "confirm", card.ID, ""))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[0].Value().(proto.MsgRow).State.By, &briefGuest)
	})
	t.Run("an admin, in a group without a secretary", func(t *testing.T) {
		s := briefState(t)
		body := briefBody()
		body.Requester = briefGuest
		card, d := briefPosted(t, s, body)
		s.Group.Secretary = ""
		act := briefAct(browser, 50, "confirm", card.ID, "")
		res := briefApply(t, s, d, receipts{}, browser, act)
		one := briefTaskID(50, 0)
		equal(t, res.Items, []home.Item{{AI: briefA, Why: "assign", Task: one, Msg: one}})
	})
}

func TestBriefConfirmRefusals(t *testing.T) {
	other := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(39)}, Kind: "brief"}
	for name, c := range map[string]struct {
		by    identity.Key
		more  string
		msg   string
		body  func(*proto.BriefBody)
		card  func(*proto.MsgRow)
		state func(*home.State)
		disk  disk
		code  string
	}{
		"with a message id that is no id":   {msg: "brief", code: "bad_request"},
		"with an arg":                       {more: `,"arg":"A"`, code: "bad_request"},
		"with an empty arg":                 {more: `,"arg":""`, code: "bad_request"},
		"with a text":                       {more: `,"text":"Yes."`, code: "bad_request"},
		"on no message":                     {msg: id(38), code: "not_found"},
		"on a message that is no brief":     {card: func(m *proto.MsgRow) { m.Kind = "decision" }, code: "bad_request"},
		"by an AI for the requester":        {by: laptop, more: `,"as":"` + aiID + `"`, code: "forbidden"},
		"by a person who did not ask":       {by: guest, code: "forbidden"},
		"on a brief that was confirmed":     {card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "confirmed"} }, code: "conflict"},
		"on a brief sent back":              {card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "revise"} }, code: "conflict"},
		"on a brief without a state":        {card: func(m *proto.MsgRow) { m.State = nil }, code: "conflict"},
		"of a requester who left":           {state: func(s *home.State) { delete(s.People, ownerID); s.People[id(24)] = person(id(24), browser, "owner") }, code: "conflict"},
		"of an assignee who left":           {state: func(s *home.State) { delete(s.AIs, briefB) }, code: "conflict"},
		"of an assignee made secretary":     {state: func(s *home.State) { s.Group.Secretary = briefA }, code: "conflict"},
		"of a brief without a criterion":    {body: func(b *proto.BriefBody) { b.Criteria = nil }, code: "conflict"},
		"of an entry with one AI twice":     {body: func(b *proto.BriefBody) { b.Tasks[1].Reviewer = briefB }, code: "conflict"},
		"whose first task id is a task's":   {state: func(s *home.State) { s.Tasks[briefTaskID(50, 0)] = proto.TaskRow{} }, code: "conflict"},
		"whose second task id is a message": {disk: disk{briefTaskID(50, 1): other}, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := briefState(t)
			card, d := briefPosted(t, s, briefBody())
			if c.body != nil {
				body := briefBody()
				c.body(&body)
				card.Body = briefRaw(t, body)
			}
			if c.card != nil {
				c.card(&card)
			}
			d[card.ID] = card
			for k, v := range c.disk {
				d[k] = v
			}
			if c.state != nil {
				c.state(s)
			}
			by, msg := browser, card.ID
			if c.by != nil {
				by = c.by
			}
			if c.msg != "" {
				msg = c.msg
			}
			briefRefused(t, briefApply(t, s, d, receipts{}, by, briefAct(by, 50, "confirm", msg, c.more)), c.code)
		})
	}
}

// Revise sends the brief back to the secretary, with the note when there is one.
func TestBriefReviseSendsTheBriefBack(t *testing.T) {
	long := strings.Repeat("a", 1022) // 1024 bytes as a JSON string
	// 1024 bytes as a JSON string without HTML escaping, as rows are measured; 1029 with it.
	angled := strings.Repeat("a", 1021) + "<"
	for name, c := range map[string]struct {
		more string
		note *string
	}{
		"with a note":                       {more: `,"text":"Include the relay."`, note: briefText("Include the relay.")},
		"with a note of 1 KiB":              {more: `,"text":"` + long + `"`, note: &long},
		"with a note of 1 KiB that holds <": {more: `,"text":"` + angled + `"`, note: &angled},
		"without a note":                    {},
	} {
		t.Run(name, func(t *testing.T) {
			s := briefState(t)
			card, d := briefPosted(t, s, briefBody())
			act := briefAct(browser, 50, "revise", card.ID, c.more)
			res := briefApply(t, s, d, receipts{}, browser, act)
			equal(t, res.Answer, store.Outcome{OK: true})
			by, at := ownerID, int64(clock)
			card.Seq, card.State = 12, &proto.CardState{Status: "revise", By: &by, At: &at, Note: c.note, Act: &act}
			equal(t, briefRows(res), []any{card})
			equal(t, res.Items, []home.Item{{AI: aiID, Why: "confirmed", Msg: card.ID}})
		})
	}
}

func TestBriefReviseRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		more string
		code string
	}{
		"with an arg":                       {more: `,"arg":"A"`, code: "bad_request"},
		"with an empty text":                {more: `,"text":""`, code: "bad_request"},
		"with a text above 1 KiB":           {more: `,"text":"` + strings.Repeat("a", 1023) + `"`, code: "bad_request"},
		"with a text holding < above 1 KiB": {more: `,"text":"` + strings.Repeat("a", 1022) + `<"`, code: "bad_request"},
		"with a payload that is bad":        {more: `,"text":7`, code: "bad_request"},
	} {
		t.Run(name, func(t *testing.T) {
			s := briefState(t)
			card, d := briefPosted(t, s, briefBody())
			briefRefused(t, briefApply(t, s, d, receipts{}, browser, briefAct(browser, 50, "revise", card.ID, c.more)), c.code)
		})
	}
	t.Run("by an AI", func(t *testing.T) {
		s := briefState(t)
		card, d := briefPosted(t, s, briefBody())
		briefRefused(t, briefApply(t, s, d, receipts{}, laptop, briefAct(laptop, 50, "revise", card.ID, `,"as":"`+briefA+`"`)), "forbidden")
	})
}

func briefText(v string) *string { return &v }
