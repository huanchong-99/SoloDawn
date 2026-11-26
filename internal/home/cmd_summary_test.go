package home_test

import (
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	sumWorkA  = id(60)
	sumWorkB  = id(61)
	sumWorkC  = id(62)
	sumMember = id(63)
	sumHostB  = device(6)
)

const sumSignedTS = clock - 500

type sumDisk struct {
	pending []proto.MsgRow
}

func (*sumDisk) Msg(string) (proto.MsgRow, bool)           { return proto.MsgRow{}, false }
func (*sumDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (*sumDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (*sumDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }

func (d *sumDisk) Pending() []proto.MsgRow {
	if d == nil {
		return nil
	}
	return d.pending
}

func (*sumDisk) Queued() []store.Item              { return nil }
func (*sumDisk) OtherMember(string) bool           { return false }
func (*sumDisk) DeviceLog(string) []proto.Envelope { return nil }

func sumState(t *testing.T) *home.State {
	t.Helper()
	s := empty(t)
	s.Logs[browser.ID()] = logOf(t, browser, node, laptop, sumHostB)
	s.Group = proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: groupID},
		Home:           node.ID(),
		Secretary:      aiID,
		Pass:           90,
		PassFoundation: 70,
		Limits:         proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	s.People[ownerID] = person(ownerID, browser, "owner")
	s.People[sumMember] = person(sumMember, guest, "member")
	s.AIs[aiID] = ai(aiID, laptop)
	s.AIs[sumWorkA] = ai(sumWorkA, laptop)
	s.AIs[sumWorkB] = ai(sumWorkB, sumHostB)
	s.AIs[sumWorkC] = ai(sumWorkC, laptop)
	s.Seq, s.Num = 20, 3
	return s
}

// sumTaskDone is a task that a passed review ended at ended.
func sumTaskDone(taskID string, started, ended int64) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 15},
		Group:     groupID,
		Num:       1,
		Title:     "Report path changes",
		Origin:    "brief",
		Brief:     id(70),
		State:     "done",
		Step:      "review",
		Report:    id(72),
		Assignee:  sumWorkA,
		Reviewer:  sumWorkB,
		Pass:      90,
		Depends:   []string{},
		Review: &proto.Review{
			Round: 2, Failed: 1, Score: 94, OK: true,
			By: sumWorkB, Report: id(72), At: ended,
		},
		Failures: 1,
		Thread: proto.ThreadSummary{
			Count:  7,
			Rounds: 5,
			Last:   &proto.ThreadMessage{By: sumWorkB, Kind: "review", Text: "Ship it.", Timestamp: clock - 2000},
		},
		Stats:   proto.TaskStats{Started: started, Ended: &ended},
		Created: started,
	}
}

func sumSummaryCmd(n byte, as string, thread *string, main bool, text string, mentions []proto.ID, re *string) proto.Post[proto.EmptyBody] {
	var tid *proto.ID
	if thread != nil {
		v := proto.ID(*thread)
		tid = &v
	}
	var rid *proto.ID
	if re != nil {
		v := proto.ID(*re)
		rid = &v
	}
	return proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(id(n)), Timestamp: sumSignedTS,
			Group: proto.ID(groupID), As: proto.ID(as),
		},
		Kind:     "summary",
		Thread:   tid,
		Main:     main,
		Text:     text,
		Mentions: mentions,
		Re:       rid,
		Body:     proto.EmptyBody{},
	}
}

func sumRun(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, payload any) home.Result {
	t.Helper()
	return briefApply(t, s, d, r, by, briefSign(by, payload))
}

func sumAssertNoNulls(t *testing.T, v any, allowed ...string) []byte {
	t.Helper()
	raw := briefRaw(t, v)
	equal(t, proto.UnexpectedNulls(raw, allowed...), []string{})
	return raw
}

// post/summary writes the summary message with main=true, fills SummaryBody
// from the done task row (score and rounds from task.Review, minutes from
// stats.started to stats.ended), and advances the task's thread summary with
// a 200-character preview.
func TestSummaryBuildsBodyAndUpdatesThread(t *testing.T) {
	tid := id(71)
	reID := id(79)
	exact200 := strings.Repeat("界", 200)
	over200 := exact200 + "尾"

	for name, tc := range map[string]struct {
		started     int64
		ended       int64
		postMain    bool
		text        string
		wantPreview string
		mentions    []proto.ID
		wantMent    []string
		re          *string
		wantMinutes int
		wantRounds  int
	}{
		"72 minutes, 201 runes cut to 200, main forced true": {
			started: clock - 72*60_000, ended: clock,
			postMain: false, text: over200, wantPreview: exact200,
			mentions: []proto.ID{proto.ID(ownerID), proto.ID(sumWorkA)}, wantMent: []string{ownerID, sumWorkA},
			re: &reID, wantMinutes: 72, wantRounds: 6,
		},
		"exact 1 minute, exact 200 runes": {
			started: clock - 60_000, ended: clock,
			postMain: true, text: exact200, wantPreview: exact200,
			mentions: []proto.ID{}, wantMent: []string{},
			re: nil, wantMinutes: 1, wantRounds: 5,
		},
		"sub-minute 59999 ms is 0": {
			started: clock - 59_999, ended: clock,
			postMain: true, text: "Done.", wantPreview: "Done.",
			mentions: []proto.ID{proto.ID(sumMember)}, wantMent: []string{sumMember},
			re: nil, wantMinutes: 0, wantRounds: 5,
		},
		"zero elapsed ms and empty text": {
			started: clock, ended: clock,
			postMain: true, text: "", wantPreview: "",
			mentions: []proto.ID{proto.ID(sumWorkB)}, wantMent: []string{sumWorkB},
			re: nil, wantMinutes: 0, wantRounds: 6,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := sumState(t)
			done := sumTaskDone(tid, tc.started, tc.ended)
			s.Tasks[tid] = done

			cmd := sumSummaryCmd(95, aiID, &tid, tc.postMain, tc.text, tc.mentions, tc.re)
			env := briefSign(laptop, cmd)
			res := briefApply(t, s, &sumDisk{}, receipts{}, laptop, env)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, len(res.Items), 0)

			wantBody := proto.SummaryBody{
				Task:    tid,
				Score:   94,
				Rounds:  2,
				Minutes: tc.wantMinutes,
			}
			wantMsg := proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: id(95), Seq: 21},
				Pos:       21,
				Group:     groupID,
				Thread:    &tid,
				Main:      true,
				By:        aiID,
				Kind:      "summary",
				Timestamp: sumSignedTS,
				Text:      tc.text,
				Mentions:  tc.wantMent,
				Re:        tc.re,
				Body:      briefRaw(t, wantBody),
				Env:       &env,
			}
			wantTask := done
			wantTask.Seq = 22
			wantTask.Thread = proto.ThreadSummary{
				Count:  8,
				Rounds: tc.wantRounds,
				Last:   &proto.ThreadMessage{By: aiID, Kind: "summary", Text: tc.wantPreview, Timestamp: sumSignedTS},
			}
			equal(t, briefRows(res), []any{wantMsg, wantTask})

			rawMsg := sumAssertNoNulls(t, wantMsg, "/re", "/state")
			var decoded proto.MessageRow[proto.SummaryBody, proto.NoCard]
			equal(t, proto.Decode(rawMsg, &decoded), nil)
		})
	}
}

type sumSummaryRefusalCase struct {
	by        identity.Key
	as        string
	noSec     bool
	raw       string
	thread    *string
	mentions  []proto.ID
	taskState string
	code      string
}

func sumSetupSummaryRefusal(t *testing.T, tid string, tc sumSummaryRefusalCase) (*home.State, identity.Key, proto.Envelope) {
	t.Helper()
	s := sumState(t)
	if tc.noSec {
		s.Group.Secretary = ""
	}
	row := sumTaskDone(tid, clock-60_000, clock)
	row.State = sumOrDefault(tc.taskState, "done")
	s.Tasks[tid] = row
	by, as := sumSummarySigner(tc)
	return s, by, sumSummaryRefusalEnv(by, as, tc)
}

func sumOrDefault(val, def string) string {
	if val != "" {
		return val
	}
	return def
}

func sumSummarySigner(tc sumSummaryRefusalCase) (identity.Key, string) {
	if tc.by != nil {
		return tc.by, tc.as
	}
	if tc.as != "" {
		return laptop, tc.as
	}
	return laptop, aiID
}

func sumSummaryRefusalEnv(by identity.Key, as string, tc sumSummaryRefusalCase) proto.Envelope {
	if tc.raw != "" {
		return by.Sign(tc.raw)
	}
	mentions := tc.mentions
	if mentions == nil {
		mentions = []proto.ID{}
	}
	return briefSign(by, sumSummaryCmd(95, as, tc.thread, true, "Summary.", mentions, nil))
}

func TestSummaryRefusals(t *testing.T) {
	tid := id(71)
	missingID := id(99)
	strangerID := id(98)

	for name, tc := range map[string]sumSummaryRefusalCase{
		"by person":               {by: browser, as: "", thread: &tid, code: "forbidden"},
		"by host without as":      {by: laptop, as: "", thread: &tid, code: "forbidden"},
		"by working AI":           {by: laptop, as: sumWorkA, thread: &tid, code: "forbidden"},
		"group without secretary": {by: laptop, as: aiID, noSec: true, thread: &tid, code: "forbidden"},
		"invalid json":            {raw: `{"t":"post","n":"` + id(95) + `","ts":1763599999500,"g":"` + groupID + `","as":"` + aiID + `","kind":"summary","thread":"` + tid + `","main":true,"text":7}`, code: "bad_request"},
		"null thread":             {thread: nil, code: "bad_request"},
		"duplicate mentions":      {thread: &tid, mentions: []proto.ID{proto.ID(ownerID), proto.ID(sumWorkA), proto.ID(ownerID)}, code: "bad_request"},
		"missing task":            {thread: &missingID, mentions: []proto.ID{proto.ID(strangerID)}, code: "not_found"},
		"queued task":             {thread: &tid, taskState: "queued", code: "conflict"},
		"active task":             {thread: &tid, taskState: "active", code: "conflict"},
		"paused task":             {thread: &tid, taskState: "paused", code: "conflict"},
		"failed task":             {thread: &tid, taskState: "failed", code: "conflict"},
		"cancelled task":          {thread: &tid, taskState: "cancelled", code: "conflict"},
		"non-member mention":      {thread: &tid, mentions: []proto.ID{proto.ID(ownerID), proto.ID(strangerID)}, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s, by, env := sumSetupSummaryRefusal(t, tid, tc)
			briefRefused(t, briefApply(t, s, &sumDisk{}, receipts{}, by, env), tc.code)
		})
	}
}
