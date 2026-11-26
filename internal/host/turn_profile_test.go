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
		ai.Usage = proto.Usage{WindowPct: new(40.0), WindowResets: new(int64(seamStart + 9_000_000))}
	})
	setTurnProfile(t, r, tPeerID, func(ai *proto.AIMemberRow) {
		ai.Tier, ai.State, ai.CLI, ai.Tool = "light", "limited", externalAgent, "ZCode"
		ai.Usage = proto.Usage{WindowPct: new(62.5)}
	})
	setTurnProfile(t, r, tSecID, func(ai *proto.AIMemberRow) {
		ai.State = "offline"
		ai.Usage = proto.Usage{WindowPct: new(100.0), WindowResets: new(int64(seamStart - 7_200_000))}
	})
	want := `[SoloDawn] group "Website" · you are "Secretary" · secretary session
AI members (CLI on computer · tier · state · skills):
- "Claude Code" · claude on workstation · strong · working on T-12 · window 40% used, resets in 3 h · Go backend
- "Codex" · external (ZCode) on workstation · light · limited · window 62.5% used
- "Secretary" · claude on workstation · standard · offline · window 100% used, resets in 0 h
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
	task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	r.h.Rows.Put(tGroupID, "task", tTaskID, mustJSON(t, task))
	item := r.makeInboxItem(proto.Item{Item: 1, Why: "reviewed", Task: proto.ID(tTaskID)}, nil, nil)
	got, _, err := Compose(r.h, r.session(TaskSession, false, "c1"), ItemsTurn, []json.RawMessage{item})
	assertSuffix(t, got, err, "== 1 of 1 · reviewed · instruction\nThis is failure 2 in a row of the same kind (review). Before you submit again, call ask to one of: \"Codex\" (standard), \"Secretary\" (strong). Then continue with the answer.")
}

func TestProfileEscalationStillBlocksWithoutCurrentCandidates(t *testing.T) {
	r := newTurnRig(t)
	task := r.baseTask()
	task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	err := profileRequireAnswered(r.h.Rows, tGroupID, task)
	want := &Error{Code: "conflict", Message: "Ask a stronger AI before submitting again. Candidates: none currently listed."}
	if got, ok := err.(*Error); !ok || *got != *want {
		t.Fatalf("got %v, want %v", err, want)
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
