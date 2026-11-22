package home

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Briefs and their confirmation: post of kind brief, and act confirm and
// revise on it (planning/contracts/commands.md 1.3 and 1.4;
// planning/contracts/messages.md 2.1 and 3.1). Confirming a brief creates its
// tasks through row 1 of the task machine.

func init() {
	Commands.Add("post/brief", briefPost)
	Commands.Add("act/confirm", briefConfirm)
	Commands.Add("act/revise", briefRevise)
}

const (
	briefKind       = "brief"
	briefPending    = "pending"
	briefConfirmed  = "confirmed"
	briefRevised    = "revise"
	briefSuperseded = "superseded"

	// briefNoteLimit is the most bytes the text of an act may have, as a JSON
	// string with its quotes (planning/contracts/messages.md section 3).
	briefNoteLimit = 1 << 10
)

// briefPost is post of kind brief: the secretary's card in the main line,
// which waits for its requester. A brief that replaces another supersedes it.
func briefPost(s *State, c Command) (Result, *Refusal) {
	if !s.Secretary(c.Who) {
		return Result{}, &Refusal{"forbidden", "only the secretary posts a brief"}
	}
	var p proto.Post[proto.BriefBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	mentions, refusal := s.briefCheck(p)
	if refusal != nil {
		return Result{}, refusal
	}
	if refusal := presetBriefCheck(c.Disk, c, p.Body.Preset); refusal != nil {
		return Result{}, refusal
	}
	return briefCard(s, c, p, mentions)
}

func (s *State) briefCheck(p proto.Post[proto.BriefBody]) ([]string, *Refusal) {
	if refusal := briefWellFormed(p); refusal != nil {
		return nil, refusal
	}
	mentions, refusal := s.postMentions(p.Mentions)
	if refusal != nil {
		return nil, refusal
	}
	return mentions, s.briefMembers(p.Body)
}

// briefWellFormed is what a brief must be whatever the group holds: a
// main-line message whose body has the shape of messages.md 2.1, and whose
// every entry has two AIs.
func briefWellFormed(p proto.Post[proto.BriefBody]) *Refusal {
	if p.Thread != nil {
		return &Refusal{"bad_request", "a brief is a message of the main line"}
	}
	if fault := briefShape(p.Body); fault != "" {
		return &Refusal{"bad_request", fault}
	}
	for _, entry := range p.Body.Tasks {
		if entry.Assignee == entry.Reviewer {
			return &Refusal{"bad_request", "the assignee of a task of the brief is also its reviewer"}
		}
	}
	return nil
}

// briefShape returns what is wrong with the body of a brief by itself, or "":
// a criterion and a task at least, and dependencies on earlier entries only.
func briefShape(b proto.BriefBody) string {
	if len(b.Criteria) == 0 || len(b.Tasks) == 0 {
		return "a brief has at least one criterion and one task"
	}
	for i, entry := range b.Tasks {
		if !briefEarlier(entry.Depends, i) {
			return fmt.Sprintf("task %d of the brief depends on an entry that is not an earlier one, or on one twice", i)
		}
	}
	return ""
}

// briefEarlier says whether every dependency of the entry at own is an
// earlier entry, named once.
func briefEarlier(depends []int, own int) bool {
	for j, d := range depends {
		if d < 0 || d >= own || slices.Contains(depends[:j], d) {
			return false
		}
	}
	return true
}

// briefMembers is what a brief must be in the group as it is now: its
// requester a member, and every assignee and reviewer a working AI.
func (s *State) briefMembers(b proto.BriefBody) *Refusal {
	_, person := s.People[b.Requester]
	_, ai := s.AIs[b.Requester]
	if !person && !ai {
		return &Refusal{"conflict", "the requester of the brief is no member of the group"}
	}
	for _, entry := range b.Tasks {
		if !s.briefWorking(entry.Assignee) || !s.briefWorking(entry.Reviewer) {
			return &Refusal{"conflict", "an assignee or reviewer of the brief is no working AI of the group"}
		}
	}
	return nil
}

// briefWorking says whether a member is an AI of the group other than its secretary.
func (s *State) briefWorking(member string) bool {
	_, ok := s.AIs[member]
	return ok && member != s.Group.Secretary
}

// briefCard writes the brief, pending, and the brief it replaces, superseded.
func briefCard(s *State, c Command, p proto.Post[proto.BriefBody], mentions []string) (Result, *Refusal) {
	card := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, By: c.Who.Member(), Kind: briefKind,
		Timestamp: c.Timestamp, Text: p.Text, Mentions: mentions, Re: (*string)(p.Re), Body: briefJSON(p.Body),
		State: &proto.CardState{Status: briefPending}, Env: &c.Env,
	}
	rows := []Row{RowOf(card)}
	if p.Body.Replaces == nil {
		return Result{Rows: rows}, nil
	}
	old, refusal := briefReplaced(c.Disk, *p.Body.Replaces)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: append(rows, RowOf(old))}, nil
}

// briefReplaced is the brief that a new one replaces, superseded: it keeps
// what its state held. Only a brief that is pending or sent back is replaced.
func briefReplaced(d Disk, id string) (proto.MsgRow, *Refusal) {
	old, ok := d.Msg(id)
	status := briefStatus(old)
	if !ok || old.Kind != briefKind || (status != briefPending && status != briefRevised) {
		return old, &Refusal{"conflict", "the brief replaces no brief that is pending or sent back"}
	}
	state := *old.State
	state.Status = briefSuperseded
	old.State = &state
	return old, nil
}

// briefStatus is the status of a card, or "" for a message that is no card.
func briefStatus(m proto.MsgRow) string {
	if m.State == nil {
		return ""
	}
	return m.State.Status
}

// briefDecision is an act on a brief that passed the checks both actions make.
type briefDecision struct {
	act  proto.Act
	card proto.MsgRow
	body proto.BriefBody
}

// briefOpen reads an act on a brief: its payload, whose arg and text args
// judges, the pending brief it names, and whether the signer may decide it.
func briefOpen(s *State, c Command, args func(Command) bool) (briefDecision, *Refusal) {
	var d briefDecision
	if refusal := c.Decode(&d.act); refusal != nil {
		return d, refusal
	}
	if !args(c) {
		return d, &Refusal{"bad_request", "this action on a brief has no arg, and its text is none or a short one"}
	}
	return briefFind(s, c, d)
}

// briefFind finds the brief of an act, and checks the signer and the status.
func briefFind(s *State, c Command, d briefDecision) (briefDecision, *Refusal) {
	card, ok := c.Disk.Msg(string(d.act.Msg))
	if !ok {
		return d, &Refusal{"not_found", "the act names no message"}
	}
	if card.Kind != briefKind {
		return d, &Refusal{"bad_request", "confirm and revise are actions on a brief"}
	}
	d.card = card
	json.Unmarshal(card.Body, &d.body) // the body was checked when the brief was posted, and never changes
	if !s.briefMay(c.Who, d.body.Requester) {
		return d, &Refusal{"forbidden", "only the requester of the brief, or an admin, decides it"}
	}
	if briefStatus(card) != briefPending {
		return d, &Refusal{"conflict", "the brief is no longer pending"}
	}
	return d, nil
}

// briefMay says whether a principal may decide a brief: a person who is its
// requester or an admin, and never an AI.
func (s *State) briefMay(w Who, requester string) bool {
	return w.AI == "" && (w.Human == requester || s.Admin(w))
}

// briefBare says whether an act has neither arg nor text, as confirm has.
func briefBare(c Command) bool { return briefAbsent(c, "arg") && briefAbsent(c, "text") }

// briefNoted says whether an act has no arg and at most a short text, as revise has.
func briefNoted(c Command) bool {
	text, ok := c.field("text")
	return briefAbsent(c, "arg") && ok && len(briefJSON(text)) <= briefNoteLimit
}

// briefAbsent says whether a payload leaves a field out.
func briefAbsent(c Command, name string) bool {
	v, ok := c.field(name)
	return ok && v == ""
}

// briefConfirm is act confirm: the checks of the post are made again, the
// brief becomes confirmed, and each entry becomes a task by row 1 of the task
// machine, with a task message.
func briefConfirm(s *State, c Command) (Result, *Refusal) {
	d, refusal := briefOpen(s, c, briefBare)
	if refusal != nil {
		return Result{}, refusal
	}
	if refusal := s.briefStill(d.body); refusal != nil {
		return Result{}, refusal
	}
	return s.briefTasks(c, d)
}

// briefStill makes the checks of the post again, which a brief that fails
// is answered conflict to. Two are left: the brief that it replaces is
// superseded since its post, and the task machine refuses an entry whose
// assignee is its reviewer (planning/contracts/task-machine.md section 2).
func (s *State) briefStill(b proto.BriefBody) *Refusal {
	if fault := briefShape(b); fault != "" {
		return &Refusal{"conflict", fault}
	}
	return s.briefMembers(b)
}

// briefTasks writes the confirmed brief and its tasks, and queues their items.
func (s *State) briefTasks(c Command, d briefDecision) (Result, *Refusal) {
	ids, refusal := briefTaskIDs(s, c, len(d.body.Tasks))
	if refusal != nil {
		return Result{}, refusal
	}
	res := Result{Rows: []Row{RowOf(briefDecided(d.card, c, briefConfirmed, nil))}}
	for i := range d.body.Tasks {
		rows, items, refusal := s.briefCreate(c, d, ids, i)
		if refusal != nil {
			return Result{}, refusal
		}
		res.Rows, res.Items = append(res.Rows, rows...), append(res.Items, items...)
	}
	res.Items = append(res.Items, s.briefTold(d.card.ID)...)
	return res, nil
}

// briefTaskIDs gives the tasks of a confirmed brief their ids, which are also
// the ids of their task messages. An id follows from the nonce of the act
// and the entry, so that a function without randomness makes it, and nobody
// knows it before the act arrives.
func briefTaskIDs(s *State, c Command, n int) ([]string, *Refusal) {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = briefTaskID(c.Nonce, i)
		_, task := s.Tasks[ids[i]]
		_, msg := c.Disk.Msg(ids[i])
		if task || msg {
			return nil, &Refusal{"conflict", "the id of a task of the brief is taken"}
		}
	}
	return ids, nil
}

// briefTaskID is a random id, the first 16 bytes of a SHA-256.
func briefTaskID(act proto.ID, entry int) string {
	h := sha256.Sum256(fmt.Appendf(nil, "SoloDawn brief task %s %d", act, entry))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

// briefCreate makes the task of entry i through row 1 of the task machine,
// and its task message. A task with a dependency is queued: every dependency
// is an earlier entry of the same brief, created now, and so not merged.
func (s *State) briefCreate(c Command, d briefDecision, ids []string, i int) ([]Row, []Item, *Refusal) {
	entry := d.body.Tasks[i]
	pass := d.body.Pass
	if entry.Foundation {
		pass = s.Group.PassFoundation
	}
	depends := make([]string, len(entry.Depends))
	for j, k := range entry.Depends {
		depends[j] = ids[k]
	}
	task := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: ids[i]}, Group: s.ID, Num: s.Num + i + 1, Title: entry.Title,
		Slug: proto.MakeSlug(entry.Title), Origin: briefKind, Brief: d.card.ID, Assignee: entry.Assignee, Reviewer: entry.Reviewer,
		Foundation: entry.Foundation, Pass: pass, Depends: depends, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
	}
	// Row 1 takes a person and an admin alike; briefMay let only a person confirm.
	task, fx, err := Step(task, Event{Kind: OnConfirm, By: ByPerson, Now: c.Now, Blocked: len(entry.Depends) > 0})
	if err != nil {
		return nil, nil, &Refusal{err.Error(), "the task machine refused a task of the brief"}
	}
	return []Row{RowOf(task), RowOf(briefTaskMsg(s, c, task.ID))}, briefItems(fx.Items, task.ID), nil
}

// briefItems completes the items of row 1: each is about the task and names its
// task message.
func briefItems(items []TaskItem, task string) []Item {
	out := make([]Item, len(items))
	for i, it := range items {
		out[i] = Item{AI: it.AI, Why: it.Why, Task: task, Msg: task}
	}
	return out
}

// briefTaskMsg is the task message of a task the home created, in the main line.
func briefTaskMsg(s *State, c Command, task string) proto.MsgRow {
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: task}, Group: s.ID, By: "system", Kind: "task",
		Timestamp: c.Now, Mentions: []string{}, Body: briefJSON(proto.TaskBody{Task: task}),
	}
}

// briefRevise is act revise: the brief goes back to the secretary, with the
// person's note when they wrote one.
func briefRevise(s *State, c Command) (Result, *Refusal) {
	d, refusal := briefOpen(s, c, briefNoted)
	if refusal != nil {
		return Result{}, refusal
	}
	var note *string
	if d.act.Text != "" {
		note = &d.act.Text
	}
	return Result{Rows: []Row{RowOf(briefDecided(d.card, c, briefRevised, note))}, Items: s.briefTold(d.card.ID)}, nil
}

// briefDecided is the brief as an act left it: who decided it, when, and the
// signed act.
func briefDecided(card proto.MsgRow, c Command, status string, note *string) proto.MsgRow {
	by, at, act := c.Who.Human, c.Now, c.Env
	card.State = &proto.CardState{Status: status, By: &by, At: &at, Note: note, Act: &act}
	return card
}

// briefTold is the confirmed item that tells the secretary what became of
// its brief. A group without a secretary has nobody to tell.
func (s *State) briefTold(brief string) []Item {
	if s.Group.Secretary == "" {
		return nil
	}
	return []Item{{AI: s.Group.Secretary, Why: "confirmed", Msg: brief}}
}

// briefJSON encodes a value as a row holds it: without HTML escaping, as a
// browser writes it.
func briefJSON(v any) json.RawMessage {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.Encode(v) // a type of internal/proto and a text encode
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}
