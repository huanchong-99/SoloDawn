package host

import (
	"encoding/json"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func setTurnProfile(t *testing.T, r *turnRig, id string, edit func(*proto.AIMemberRow)) {
	t.Helper()
	raw, _ := r.h.Rows.Get(tGroupID, "member", id)
	var ai proto.AIMemberRow
	if err := proto.Decode(raw, &ai); err != nil {
		t.Fatal(err)
	}
	edit(&ai)
	r.h.Rows.Put(tGroupID, "member", id, mustJSON(t, ai))
}

func TestSecretaryProfileRosterOnReplacement(t *testing.T) {
	r := newTurnRig(t)
	setTurnProfile(t, r, tAIID, func(ai *proto.AIMemberRow) {
		ai.Tier, ai.State, ai.Task, ai.Skills = "strong", "working", tTaskID, "Go backend"
	})
	setTurnProfile(t, r, tPeerID, func(ai *proto.AIMemberRow) {
		ai.Tier, ai.State = "light", "limited"
	})
	setTurnProfile(t, r, tSecID, func(ai *proto.AIMemberRow) { ai.State = "offline" })
	want := `[SoloDawn] group "Website" · you are "Secretary" · secretary session
AI members (CLI on computer · tier · state · skills):
- "Claude Code" · claude on workstation · strong · working on T-12 · Go backend
- "Codex" · codex on workstation · light · limited
- "Secretary" · claude on workstation · standard · offline
== context · instruction`
	assertComposed(t, r.h, r.session(GroupSession, true, ""), ReplacedTurn, nil, want, Turn{Kind: ReplacedTurn})
}

func TestProfileEscalationUsesLatestTaskAndAppendsToItem(t *testing.T) {
	r := newTurnRig(t)
	setTurnProfile(t, r, tPeerID, func(ai *proto.AIMemberRow) { ai.Tier = "light" })
	setTurnProfile(t, r, tSecID, func(ai *proto.AIMemberRow) {
		ai.Tier, ai.State = "strong", "offline"
	})
	task := r.baseTask()
	task.Stuck = &proto.Stuck{Kind: "review", N: 3, Ask: true}
	r.h.Rows.Put(tGroupID, "task", tTaskID, mustJSON(t, task))
	stale := r.baseTask()
	msg := r.textMsg(tMsg1ID, tPeerID, tTaskID, "This still needs work.")
	item := r.makeInboxItem(proto.Item{Item: 1, Why: "reviewed", Msg: proto.ID(tMsg1ID)}, msg, &stale)

	got, _, err := Compose(r.h, r.session(TaskSession, false, "c1"), ItemsTurn, []json.RawMessage{item})
	assertSuffix(t, got, err, "This still needs work.\nThis is failure 3 in a row of the same kind (review). Before you submit again, call ask to one of: \"Secretary\" (strong). Then continue with the answer.")
}

func TestProfileEscalationWithoutCarriedMessage(t *testing.T) {
	r := newTurnRig(t)
	setTurnProfile(t, r, tAIID, func(ai *proto.AIMemberRow) { ai.Tier = "light" })
	setTurnProfile(t, r, tSecID, func(ai *proto.AIMemberRow) { ai.Tier = "strong" })
	task := r.baseTask()
	task.Stuck = &proto.Stuck{Kind: "checks", N: 2, Ask: true}
	r.h.Rows.Put(tGroupID, "task", tTaskID, mustJSON(t, task))
	item := r.makeInboxItem(proto.Item{Item: 1, Why: "checks", Task: proto.ID(tTaskID)}, nil, nil)
	got, _, err := Compose(r.h, r.session(TaskSession, false, "c1"), ItemsTurn, []json.RawMessage{item})
	assertSuffix(t, got, err, "== 1 of 1 · checks · instruction\nThis is failure 2 in a row of the same kind (checks). Before you submit again, call ask to one of: \"Codex\" (standard), \"Secretary\" (strong). Then continue with the answer.")
}

func TestProfileEscalationStillBlocksWithoutCurrentCandidates(t *testing.T) {
	r := newTurnRig(t)
	task := r.baseTask()
	task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	for _, assignee := range []string{tAIID, turnID("missing")} {
		task.Assignee = assignee
		err := profileRequireAnswered(r.h.Rows, tGroupID, task)
		want := &Error{Code: "conflict", Message: "Ask a stronger AI before submitting again. Candidates: none currently listed."}
		if got, ok := err.(*Error); !ok || *got != *want {
			t.Fatalf("got %v, want %v", err, want)
		}
	}
}

func TestSubmitTaskRequiresAnswerBeforeCheckingCommit(t *testing.T) {
	r := newTurnRig(t)
	setTurnProfile(t, r, tSecID, func(ai *proto.AIMemberRow) { ai.Tier = "strong" })
	task := r.baseTask()
	task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	r.h.Rows.Put(tGroupID, "task", tTaskID, mustJSON(t, task))
	caller := Caller{Session: r.session(TaskSession, false, "c1")}
	got, err := Tools["submit_task"].Call(t.Context(), r.h, caller, json.RawMessage(`{"summary":"Ready"}`))
	want := &Error{Code: "conflict", Message: "Ask a stronger AI before submitting again. Candidates: \"Secretary\" (strong)."}
	if refusal, ok := err.(*Error); got != nil || !ok || *refusal != *want {
		t.Fatalf("got %v, %v; want nil, %v", got, err, want)
	}
}
