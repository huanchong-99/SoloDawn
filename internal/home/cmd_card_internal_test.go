package home

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type cardTestDisk []proto.MsgRow

func (d cardTestDisk) Msg(id string) (proto.MsgRow, bool) {
	for _, m := range d {
		if m.ID == id {
			return m, true
		}
	}
	return proto.MsgRow{}, false
}

func (d cardTestDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d cardTestDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d cardTestDisk) Pending() []proto.MsgRow                   { return d }
func (d cardTestDisk) Queued() []store.Item                      { return nil }

func cardTestSame(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

func cardTestFixture() (string, string, cardTestDisk) {
	t1, t2 := "task-one", "task-two"
	d := cardTestDisk{
		{
			RowHeader: proto.RowHeader{Type: "msg", ID: "brief-1", Seq: 1}, Pos: 1, Group: "g1",
			By: "sec", Kind: "brief", Timestamp: 100, Mentions: []string{},
			Body: json.RawMessage(`{"requester":"m1"}`), State: &proto.CardState{Status: "pending"},
		},
		{
			RowHeader: proto.RowHeader{Type: "msg", ID: "dec-main", Seq: 2}, Pos: 2, Group: "g1",
			Main: true, By: "sec", Kind: "decision", Timestamp: 110, Mentions: []string{},
			Body: json.RawMessage(`{"question":"Q?","options":[],"to":["u1"]}`), State: &proto.CardState{Status: "pending"},
		},
		{
			RowHeader: proto.RowHeader{Type: "msg", ID: "app-other", Seq: 3}, Pos: 3, Group: "g1",
			Thread: &t2, Main: true, By: "ai2", Kind: "approval", Timestamp: 120, Mentions: []string{},
			Body:  json.RawMessage(`{"task":"task-two","ai":"ai2","tool":"Bash","command":"ls","category":"command","approvers":["u2"]}`),
			State: &proto.CardState{Status: "pending"},
		},
		{
			RowHeader: proto.RowHeader{Type: "msg", ID: "call-1", Seq: 4}, Pos: 4, Group: "g1",
			Thread: &t1, By: "ai1", Kind: "call", Timestamp: 130, Mentions: []string{},
			Body: json.RawMessage(`{"to":"ai2","depth":1,"wait":true}`), State: &proto.CardState{Status: "waiting"},
		},
		{
			RowHeader: proto.RowHeader{Type: "msg", ID: "dec-t1", Seq: 5}, Pos: 5, Group: "g1",
			Thread: &t1, Main: true, By: "sec", Kind: "decision", Timestamp: 140, Mentions: []string{},
			Body:  json.RawMessage(`{"task":"task-one","question":"Where?","options":[],"to":["u1","u3"]}`),
			State: &proto.CardState{Status: "pending"},
		},
		{
			RowHeader: proto.RowHeader{Type: "msg", ID: "app-t1", Seq: 6}, Pos: 6, Group: "g1",
			Thread: &t1, Main: true, By: "ai1", Kind: "approval", Timestamp: 150, Mentions: []string{"m1"},
			Body:  json.RawMessage(`{"task":"task-one","ai":"ai1","tool":"Bash","command":"npm i","category":"command","approvers":["u1"]}`),
			State: &proto.CardState{Status: "pending"},
		},
	}
	return t1, t2, d
}

// cardPending keeps only the pending approval and decision cards of the named
// task, in the oldest-first order Disk.Pending returns them.
func TestCardPendingSelectsTaskCardsInOrder(t *testing.T) {
	t1, _, d := cardTestFixture()
	got := cardPending(d, t1)
	want := []PendingCard{
		{Kind: "decision", Msg: "dec-t1", Who: []string{"u1", "u3"}, Since: 140},
		{Kind: "approval", Msg: "app-t1", Who: []string{"u1"}, Since: 150},
	}
	cardTestSame(t, got, want)
	cardTestSame(t, cardPending(d, "task-missing"), []PendingCard(nil))
}

// cardClose marks pending approval cards reassigned on an assignee change, and
// both pending approval and decision cards expired when the task ends.
func TestCardCloseExpiresAndReassigns(t *testing.T) {
	t1, _, d := cardTestFixture()
	at := int64(900)

	cardTestSame(t, cardClose(d, t1, "", at), []Row(nil))

	reassigned := cardClose(d, t1, "reassigned", at)
	cardTestSame(t, len(reassigned), 1)
	wantApp, _ := d.Msg("app-t1")
	wantApp.State = &proto.CardState{Status: "reassigned", At: &at}
	cardTestSame(t, reassigned[0].Value(), any(wantApp))
	rawApp := briefJSON(reassigned[0].Value())
	cardTestSame(t, proto.UnexpectedNulls(rawApp, "/re"), []string{})
	var decodedApp proto.MessageRow[proto.ApprovalBody, proto.TimedState]
	cardTestSame(t, proto.Decode(rawApp, &decodedApp), nil)

	expired := cardClose(d, t1, "expired", at)
	cardTestSame(t, len(expired), 2)
	wantDec, _ := d.Msg("dec-t1")
	wantDec.State = &proto.CardState{Status: "expired", At: &at}
	wantApp.State = &proto.CardState{Status: "expired", At: &at}
	cardTestSame(t, []any{expired[0].Value(), expired[1].Value()}, []any{wantDec, wantApp})
	rawDec := briefJSON(expired[0].Value())
	cardTestSame(t, proto.UnexpectedNulls(rawDec, "/re"), []string{})
	var decodedDec proto.MessageRow[proto.DecisionBody, proto.TimedState]
	cardTestSame(t, proto.Decode(rawDec, &decodedDec), nil)
}
