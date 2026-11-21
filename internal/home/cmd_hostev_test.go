package home_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	hevWorker = id(60)
	hevPeer   = id(61)
	hevTaskA  = id(62)
	hevTaskB  = id(63)
)

const hevStatusTS = clock - 400

// hevState builds a group with a secretary (aiID on laptop), a working AI
// (hevWorker on laptop), a working AI on node (hevPeer), two tasks and a guest.
func hevState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Group.Name, s.Group.Kind, s.Group.Base, s.Group.Forge, s.Group.Home = "Website", "project", "main", "none", node.ID()
	s.Group.Checks = []string{}
	owner := s.People[ownerID]
	owner.Group, owner.Kind, owner.Name = groupID, "human", "owner"
	s.People[ownerID] = owner
	guestRow := person(id(64), guest, "member")
	guestRow.Group, guestRow.Kind, guestRow.Name = groupID, "human", "guest"
	s.People[id(64)] = guestRow

	sec := s.AIs[aiID]
	sec.Group, sec.Kind, sec.Name, sec.CLI, sec.HostName, sec.Account = groupID, "ai", "Secretary", "claude", "laptop", "personal"
	sec.Allow, sec.Approvers, sec.State, sec.Since, sec.LastSeen, sec.Path = []string{}, []string{}, "dormant", clock-7000, clock-6000, "direct"
	s.AIs[aiID] = sec

	worker := ai(hevWorker, laptop)
	worker.Group, worker.Kind, worker.Name = groupID, "ai", "Codex"
	worker.CLI, worker.HostName, worker.Account = "codex", "laptop", "personal"
	worker.Allow, worker.Approvers = []string{}, []string{browser.ID()}
	worker.State, worker.Since, worker.LastSeen, worker.Path = "dormant", clock-5000, clock-3000, "direct"
	s.AIs[hevWorker] = worker

	peer := ai(hevPeer, node)
	peer.Group, peer.Kind, peer.Name, peer.CLI, peer.HostName, peer.Account = groupID, "ai", "Claude", "claude", "home", "personal"
	peer.Allow, peer.Approvers = []string{}, []string{}
	peer.State, peer.Task, peer.Since, peer.LastSeen, peer.Path = "working", hevTaskA, clock-4000, clock-2000, "local"
	s.AIs[hevPeer] = peer

	s.Tasks[hevTaskA] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: hevTaskA, Seq: 8}, Group: groupID, Num: 1,
		Title: "Report path changes", Slug: "report-path-changes", Origin: "brief",
		State: "active", Step: "local_check", Assignee: hevWorker, Reviewer: hevPeer,
		Depends: []string{}, Thread: proto.ThreadSummary{Count: 2, Rounds: 1},
	}
	s.Tasks[hevTaskB] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: hevTaskB, Seq: 9}, Group: groupID, Num: 2,
		Title: "Show the path", Slug: "show-the-path", Origin: "brief",
		State: "active", Step: "branch", Assignee: hevPeer, Reviewer: hevWorker, Depends: []string{},
	}
	return s
}

func hevSend(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, payload string) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, by.ID(), by.Sign(payload), now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func hevPayload(n byte, member, state, path, extra string) string {
	return fmt.Sprintf(
		`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":%q,"path":%q,"usage":{}%s}`,
		id(n), hevStatusTS, groupID, member, state, path, extra,
	)
}

func hevRefused(t *testing.T, res home.Result, code string) {
	t.Helper()
	if res.Answer.Code != code || res.Msg == "" || len(res.Rows) != 0 || len(res.Items) != 0 {
		t.Fatalf("answered %+v (%q) with %d rows and %d items; want %s", res.Answer, res.Msg, len(res.Rows), len(res.Items), code)
	}
}

func hevFloat(v float64) *float64 { return &v }
func hevInt64(v int64) *int64     { return &v }
func hevInt(v int) *int           { return &v }
func hevStr(v string) *string     { return &v }

// All six member-status wire golden files apply cleanly, and the resulting AI
// member row and system message row match row-member-ai.json and row-msg-system.json.
func TestHostEvGoldenWireExamples(t *testing.T) {
	const (
		goldenGroup = "rZNvy-1jH6Z-BcPqA5U5BQ"
		goldenAI    = "1PC8WineBrUQ-apCjx7tug"
		goldenTask  = "DrtCn6htSBwmMPrFPbHJHA"
	)
	var baseAI proto.AIMemberRow
	rawAI, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "row-member-ai.json"))
	equal(t, err, nil)
	equal(t, proto.Decode(rawAI, &baseAI), nil)

	makeGoldenState := func(seq int64) *home.State {
		s := home.NewState(goldenGroup, node.ID(), browser.ID())
		s.Logs[browser.ID()] = logOf(t, browser, node, laptop)
		s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: goldenGroup}, Home: node.ID(), Secretary: aiID}
		s.People[ownerID] = person(ownerID, browser, "owner")
		s.AIs[aiID] = ai(aiID, laptop)
		member := baseAI
		member.Host = laptop.ID()
		member.Usage = proto.Usage{}
		member.LastSeen = 1763599990000
		s.AIs[goldenAI] = member
		s.Tasks[goldenTask] = proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: goldenTask, Seq: 1}, Group: goldenGroup,
			Num: 1, Title: "Golden task", Slug: "golden-task", Origin: "brief",
			State: "active", Step: "local_check", Assignee: goldenAI, Depends: []string{},
		}
		s.Seq = seq
		return s
	}

	// member-status.json keeps state "working", so since stays 1763599998000 while last_seen becomes 1763600000000.
	statusBytes, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "member-status.json"))
	equal(t, err, nil)
	s := makeGoldenState(788)
	res, err := home.Commands.Apply(s, disk{}, receipts{}, laptop.ID(), laptop.Sign(string(statusBytes)), time.UnixMilli(1763600000000))
	equal(t, err, nil)
	equal(t, res.Answer, store.Outcome{OK: true})
	gotAI := res.Rows[0].Value().(proto.AIMemberRow)
	gotAI.Host = baseAI.Host
	equal(t, gotAI, baseAI)
	encodedAI, err := json.Marshal(gotAI)
	equal(t, err, nil)
	equal(t, proto.UnexpectedNulls(encodedAI), []string{})

	// member-status-turn_interrupted.json produces the system row of row-msg-system.json.
	var wantSys, decodedSys proto.MessageRow[proto.TurnInterruptedBody, proto.NoCard]
	rawSys, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "row-msg-system.json"))
	equal(t, err, nil)
	equal(t, proto.Decode(rawSys, &wantSys), nil)

	intrBytes, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "member-status-turn_interrupted.json"))
	equal(t, err, nil)
	s = makeGoldenState(788)
	res, err = home.Commands.Apply(s, disk{}, receipts{}, laptop.ID(), laptop.Sign(string(intrBytes)), time.UnixMilli(1763599999500))
	equal(t, err, nil)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Rows), 3)
	gotSys := res.Rows[1].Value().(proto.MsgRow)
	encodedSys, err := json.Marshal(gotSys)
	equal(t, err, nil)
	equal(t, proto.UnexpectedNulls(encodedSys, "/re", "/state"), []string{})
	equal(t, proto.Decode(encodedSys, &decodedSys), nil)
	wantSys.ID = "fTuX8YoC2SHlMHGAs69RFg"
	equal(t, decodedSys, wantSys)

	for _, name := range []string{
		"member-status-cap_reached",
		"member-status-login_invalid",
		"member-status-rate_limited",
		"member-status-session_replaced",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", name+".json"))
			equal(t, err, nil)
			st := makeGoldenState(10)
			out, err := home.Commands.Apply(st, disk{}, receipts{}, laptop.ID(), laptop.Sign(string(data)), time.UnixMilli(1763600000000))
			equal(t, err, nil)
			equal(t, out.Answer, store.Outcome{OK: true})
		})
	}
}

// Every valid state and path is accepted, since moves only when state changes,
// task clears when omitted, and zero usage figures are accepted at the boundary.
func TestHostEvStatusStatesPathsAndSince(t *testing.T) {
	states := []string{"working", "waiting", "dormant", "takeover", "paused", "limited", "offline"}
	paths := []string{"local", "direct", "relay", ""}
	for i, state := range states {
		path := paths[i%len(paths)]
		t.Run(state+"_"+path, func(t *testing.T) {
			s := hevState(t)
			extra := fmt.Sprintf(`,"task":%q,"usage":{"window_pct":0,"window_resets":0,"day_cap_pct":0,"spend_today":0,"spend_cap":0,"turns_today":0}`, hevTaskA)
			res := hevSend(t, s, disk{}, receipts{}, laptop, hevPayload(1, hevWorker, state, path, extra))
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, len(res.Items), 0)
			wantSince := int64(clock)
			if state == "dormant" {
				wantSince = clock - 5000
			}
			want := s.AIs[hevWorker]
			want.Seq = 11
			want.State, want.Task, want.Since, want.LastSeen, want.Path = state, hevTaskA, wantSince, clock, path
			want.Usage = proto.Usage{
				WindowPct: hevFloat(0), WindowResets: hevInt64(0), DayCapPct: hevFloat(0),
				SpendToday: hevFloat(0), SpendCap: hevFloat(0), TurnsToday: hevInt(0),
			}
			equal(t, briefRows(res), []any{want})
			s.Commit(res)

			// A follow-up status without task and with empty usage clears both and keeps since when state repeats.
			res2, err := home.Commands.Apply(s, disk{}, receipts{}, laptop.ID(), laptop.Sign(hevPayload(2, hevWorker, state, "relay", "")), time.UnixMilli(clock+900))
			equal(t, err, nil)
			want.Seq, want.Task, want.Path, want.Usage, want.LastSeen = 12, "", "relay", proto.Usage{}, clock+900
			equal(t, briefRows(res2), []any{want})
		})
	}
}

// All five host event codes write their system row in the task thread or main
// line, advance task.Thread when in a thread, and queue a secretary notice
// only for cap_reached, rate_limited and login_invalid when a secretary exists.
func TestHostEvAllFiveEventsAndNotices(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusTask string
		eventJSON  string
		wantThread string
		wantBody   any
		wantNotice *home.Item
	}{
		{
			name:       "turn_interrupted crash in thread with detail",
			eventJSON:  fmt.Sprintf(`{"code":"turn_interrupted","reason":"crash","task":%q,"detail":"exit <1> & signal"}`, hevTaskA),
			wantThread: hevTaskA,
			wantBody: proto.TurnInterruptedBody{
				SystemCode: proto.SystemCode{Code: "turn_interrupted"}, AI: hevWorker,
				Reason: "crash", Task: hevStr(hevTaskA), Detail: hevStr("exit <1> & signal"),
			},
		},
		{
			name:      "turn_interrupted stop in main line without detail",
			eventJSON: `{"code":"turn_interrupted","reason":"stop"}`,
			wantBody: proto.TurnInterruptedBody{
				SystemCode: proto.SystemCode{Code: "turn_interrupted"}, AI: hevWorker, Reason: "stop",
			},
		},
		{
			name:      "turn_interrupted timeout in main line",
			eventJSON: `{"code":"turn_interrupted","reason":"timeout"}`,
			wantBody: proto.TurnInterruptedBody{
				SystemCode: proto.SystemCode{Code: "turn_interrupted"}, AI: hevWorker, Reason: "timeout",
			},
		},
		{
			name:       "cap_reached task in thread",
			eventJSON:  fmt.Sprintf(`{"code":"cap_reached","cap":"task","task":%q}`, hevTaskA),
			wantThread: hevTaskA,
			wantBody: proto.CapReachedBody{
				SystemCode: proto.SystemCode{Code: "cap_reached"}, AI: hevWorker, Cap: "task", Task: hevStr(hevTaskA),
			},
			wantNotice: &home.Item{AI: aiID, Why: "notice", Task: hevTaskA, Msg: id(10), Code: "cap_reached"},
		},
		{
			name:      "cap_reached day in main line",
			eventJSON: `{"code":"cap_reached","cap":"day"}`,
			wantBody: proto.CapReachedBody{
				SystemCode: proto.SystemCode{Code: "cap_reached"}, AI: hevWorker, Cap: "day",
			},
			wantNotice: &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "cap_reached"},
		},
		{
			name:       "rate_limited at resets zero stays in main line even when status names a task",
			statusTask: hevTaskB,
			eventJSON:  `{"code":"rate_limited","resets":0}`,
			wantBody: proto.RateLimitedBody{
				SystemCode: proto.SystemCode{Code: "rate_limited"}, AI: hevWorker, Resets: 0,
			},
			wantNotice: &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "rate_limited"},
		},
		{
			name:      "login_invalid with detail",
			eventJSON: `{"code":"login_invalid","detail":"Token expired."}`,
			wantBody: proto.LoginInvalidBody{
				SystemCode: proto.SystemCode{Code: "login_invalid"}, AI: hevWorker, Detail: hevStr("Token expired."),
			},
			wantNotice: &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "login_invalid"},
		},
		{
			name:      "login_invalid without detail",
			eventJSON: `{"code":"login_invalid"}`,
			wantBody: proto.LoginInvalidBody{
				SystemCode: proto.SystemCode{Code: "login_invalid"}, AI: hevWorker,
			},
			wantNotice: &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "login_invalid"},
		},
		{
			name:       "session_replaced in thread",
			eventJSON:  fmt.Sprintf(`{"code":"session_replaced","task":%q}`, hevTaskA),
			wantThread: hevTaskA,
			wantBody: proto.SessionReplacedBody{
				SystemCode: proto.SystemCode{Code: "session_replaced"}, AI: hevWorker, Task: hevStr(hevTaskA),
			},
		},
		{
			name:      "session_replaced in main line",
			eventJSON: `{"code":"session_replaced"}`,
			wantBody: proto.SessionReplacedBody{
				SystemCode: proto.SystemCode{Code: "session_replaced"}, AI: hevWorker,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := hevState(t)
			extra := `,"event":` + tc.eventJSON
			if tc.statusTask != "" {
				extra = fmt.Sprintf(`,"task":%q`, tc.statusTask) + extra
			}
			r := receipts{}
			raw := hevPayload(10, hevWorker, "limited", "direct", extra)
			res := hevSend(t, s, disk{}, r, laptop, raw)
			equal(t, res.Answer, store.Outcome{OK: true})

			wantAI := s.AIs[hevWorker]
			wantAI.Seq, wantAI.State, wantAI.Task, wantAI.Since, wantAI.LastSeen = 11, "limited", tc.statusTask, clock, clock

			var thread *string
			if tc.wantThread != "" {
				thread = hevStr(tc.wantThread)
			}
			wantMsg := proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: id(10), Seq: 12}, Pos: 12,
				Group: groupID, Thread: thread, By: "system", Kind: "system",
				Timestamp: clock, Mentions: []string{}, Body: briefRaw(t, tc.wantBody),
			}
			if tc.wantThread == "" {
				equal(t, briefRows(res), []any{wantAI, wantMsg})
			} else {
				wantTask := s.Tasks[tc.wantThread]
				wantTask.Seq = 13
				wantTask.Thread.Count++
				wantTask.Thread.Last = &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock}
				equal(t, briefRows(res), []any{wantAI, wantMsg, wantTask})
			}

			var wantItems []home.Item
			if tc.wantNotice != nil {
				wantItems = []home.Item{*tc.wantNotice}
			}
			equal(t, res.Items, wantItems)

			// Replaying the exact same command returns the receipt without writing again.
			s.Commit(res)
			replay := hevSend(t, s, disk{id(10): wantMsg}, r, laptop, raw)
			equal(t, replay, home.Result{Answer: store.Outcome{OK: true}})
		})
	}

	t.Run("without a secretary no notice is queued", func(t *testing.T) {
		s := hevState(t)
		s.Group.Secretary = ""
		res := hevSend(t, s, disk{}, receipts{}, laptop, hevPayload(11, hevWorker, "limited", "local", `,"event":{"code":"login_invalid"}`))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 2)
		equal(t, len(res.Items), 0)
	})

	t.Run("status without event ignores an existing message with id n", func(t *testing.T) {
		s := hevState(t)
		existing := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(12)}, Kind: "text"}
		res := hevSend(t, s, disk{id(12): existing}, receipts{}, laptop, hevPayload(12, hevWorker, "waiting", "direct", ""))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
	})
}

type hevRefusalCase struct {
	by      identity.Key
	member  string
	state   string
	hasSt   bool
	path    string
	extra   string
	raw     string
	onDisk  disk
	wantErr string
}

func hevRefusalInput(tc hevRefusalCase) (identity.Key, string) {
	by := hevOr(tc.by, laptop)
	if tc.raw != "" {
		return by, tc.raw
	}
	state := "working"
	if tc.hasSt {
		state = tc.state
	}
	return by, hevPayload(25, hevStrOr(tc.member, hevWorker), state, hevStrOr(tc.path, "local"), tc.extra)
}

func hevOr(k, def identity.Key) identity.Key {
	if k != nil {
		return k
	}
	return def
}

func hevStrOr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

func TestHostEvStatusRefusals(t *testing.T) {
	existing := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(25)}, Kind: "text"}
	for name, tc := range map[string]hevRefusalCase{
		"missing required path": {
			raw:     fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","usage":{}}`, id(25), hevStatusTS, groupID, hevWorker),
			wantErr: "bad_request",
		},
		"null event":                    {extra: `,"event":null`, wantErr: "bad_request"},
		"event without code":            {extra: `,"event":{}`, wantErr: "bad_request"},
		"task not a random id":          {extra: `,"task":"not-an-id"`, wantErr: "bad_request"},
		"unknown state":                 {state: "running", hasSt: true, wantErr: "bad_request"},
		"empty state":                   {state: "", hasSt: true, wantErr: "bad_request"},
		"unknown path":                  {path: "wan", wantErr: "bad_request"},
		"negative window_pct":           {raw: fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","path":"local","usage":{"window_pct":-0.1}}`, id(25), hevStatusTS, groupID, hevWorker), wantErr: "bad_request"},
		"negative window_resets":        {raw: fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","path":"local","usage":{"window_resets":-1}}`, id(25), hevStatusTS, groupID, hevWorker), wantErr: "bad_request"},
		"negative day_cap_pct":          {raw: fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","path":"local","usage":{"day_cap_pct":-0.1}}`, id(25), hevStatusTS, groupID, hevWorker), wantErr: "bad_request"},
		"negative spend_today":          {raw: fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","path":"local","usage":{"spend_today":-0.01}}`, id(25), hevStatusTS, groupID, hevWorker), wantErr: "bad_request"},
		"negative spend_cap":            {raw: fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","path":"local","usage":{"spend_cap":-0.01}}`, id(25), hevStatusTS, groupID, hevWorker), wantErr: "bad_request"},
		"negative turns_today":          {raw: fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","path":"local","usage":{"turns_today":-1}}`, id(25), hevStatusTS, groupID, hevWorker), wantErr: "bad_request"},
		"home-only event code":          {extra: `,"event":{"code":"no_secretary"}`, wantErr: "bad_request"},
		"unknown event code":            {extra: `,"event":{"code":"mystery"}`, wantErr: "bad_request"},
		"turn_interrupted no reason":    {extra: `,"event":{"code":"turn_interrupted"}`, wantErr: "bad_request"},
		"turn_interrupted bad reason":   {extra: `,"event":{"code":"turn_interrupted","reason":"killed"}`, wantErr: "bad_request"},
		"turn_interrupted empty detail": {extra: `,"event":{"code":"turn_interrupted","reason":"crash","detail":""}`, wantErr: "bad_request"},
		"turn_interrupted bad task":     {extra: `,"event":{"code":"turn_interrupted","reason":"crash","task":"bad"}`, wantErr: "bad_request"},
		"cap_reached no cap":            {extra: `,"event":{"code":"cap_reached"}`, wantErr: "bad_request"},
		"cap_reached bad cap":           {extra: `,"event":{"code":"cap_reached","cap":"month"}`, wantErr: "bad_request"},
		"cap_reached bad task":          {extra: `,"event":{"code":"cap_reached","cap":"task","task":"bad"}`, wantErr: "bad_request"},
		"rate_limited no resets":        {extra: `,"event":{"code":"rate_limited"}`, wantErr: "bad_request"},
		"rate_limited negative resets":  {extra: `,"event":{"code":"rate_limited","resets":-1}`, wantErr: "bad_request"},
		"login_invalid non-text detail": {extra: `,"event":{"code":"login_invalid","detail":7}`, wantErr: "bad_request"},
		"login_invalid empty detail":    {extra: `,"event":{"code":"login_invalid","detail":""}`, wantErr: "bad_request"},
		"session_replaced bad task":     {extra: `,"event":{"code":"session_replaced","task":"bad"}`, wantErr: "bad_request"},
		"unknown member":                {member: id(99), wantErr: "not_found"},
		"human member":                  {member: ownerID, wantErr: "forbidden"},
		"signed by owner browser":       {by: browser, wantErr: "forbidden"},
		"signed by another host node":   {by: node, wantErr: "forbidden"},
		"signed with as by own host":    {extra: fmt.Sprintf(`,"as":%q`, hevWorker), wantErr: "forbidden"},
		"unknown status task":           {extra: fmt.Sprintf(`,"task":%q`, id(98)), wantErr: "not_found"},
		"unknown event task":            {extra: fmt.Sprintf(`,"event":{"code":"session_replaced","task":%q}`, id(98)), onDisk: disk{id(25): existing}, wantErr: "not_found"},
		"message id already exists":     {extra: `,"event":{"code":"login_invalid"}`, onDisk: disk{id(25): existing}, wantErr: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			by, payload := hevRefusalInput(tc)
			hevRefused(t, hevSend(t, hevState(t), tc.onDisk, receipts{}, by, payload), tc.wantErr)
		})
	}
}

// A host event whose detail pushes the system row above 16 KiB is refused by Seal.
func TestHostEvSystemRowSizeBoundary(t *testing.T) {
	sendDetail := func(detail string) home.Result {
		s := hevState(t)
		extra := fmt.Sprintf(`,"event":{"code":"login_invalid","detail":%q}`, detail)
		return hevSend(t, s, disk{}, receipts{}, laptop, hevPayload(30, hevWorker, "limited", "local", extra))
	}
	base := sendDetail("a")
	equal(t, base.Answer, store.Outcome{OK: true})
	encoded, err := json.Marshal(base.Rows[1].Value())
	equal(t, err, nil)
	baseLen := len(bytes.ReplaceAll(encoded, []byte(`\u003c`), []byte("<")))

	exact := "a" + strings.Repeat("<", 16384-baseLen)
	okRes := sendDetail(exact)
	equal(t, okRes.Answer, store.Outcome{OK: true})

	hevRefused(t, sendDetail(exact+"<"), "bad_request")
}

// HostOfflineEvent marks every non-offline AI on that host offline in sorted ID
// order once the host has no open connection, both directly and via StartLoop.
func TestHostEvOfflineHandler(t *testing.T) {
	s := hevState(t)
	// Add a third AI on laptop that is already offline, and give aiID an active state and task.
	alreadyOff := id(70)
	offAI := ai(alreadyOff, laptop)
	offAI.Group, offAI.Kind, offAI.Name = groupID, "ai", "Offline AI"
	offAI.State, offAI.Since, offAI.LastSeen, offAI.Path = "offline", clock-9000, clock-8000, "relay"
	s.AIs[alreadyOff] = offAI

	sec := s.AIs[aiID]
	sec.Group, sec.Kind, sec.Name = groupID, "ai", "Secretary"
	sec.State, sec.Task, sec.Since, sec.LastSeen, sec.Path = "working", hevTaskB, clock-6000, clock-1000, "direct"
	s.AIs[aiID] = sec

	worker := s.AIs[hevWorker]
	worker.State, worker.Task = "waiting", hevTaskA
	s.AIs[hevWorker] = worker

	// While laptop is still online, or for a non-string / unrelated device, nothing changes.
	s.Online[laptop.ID()] = true
	equal(t, home.Events[home.HostOfflineEvent](s, disk{}, clock, laptop.ID()), home.Result{})
	delete(s.Online, laptop.ID())
	equal(t, home.Events[home.HostOfflineEvent](s, disk{}, clock, 42), home.Result{})
	equal(t, home.Events[home.HostOfflineEvent](s, disk{}, clock, guest.ID()), home.Result{})

	res := s.Seal(home.Events[home.HostOfflineEvent](s, disk{}, clock, laptop.ID()))
	equal(t, res.Answer, store.Outcome{OK: true})

	wantSec := sec
	wantSec.Seq, wantSec.State, wantSec.Task, wantSec.Since = 11, "offline", "", clock
	wantWorker := worker
	wantWorker.Seq, wantWorker.State, wantWorker.Task, wantWorker.Since = 12, "offline", "", clock
	// aiID (seed 11) sorts before hevWorker (seed 60): "Cws..." < "PDA...".
	equal(t, briefRows(res), []any{wantSec, wantWorker})

	s.Commit(res)
	equal(t, s.AIs[alreadyOff].Since, int64(clock-9000))
	equal(t, s.AIs[hevPeer].State, "working")
	equal(t, s.AIs[hevPeer].Task, hevTaskA)
}

type hevLoopClient struct {
	done chan struct{}
	ch   chan any
}

func hevNewClient() *hevLoopClient {
	return &hevLoopClient{done: make(chan struct{}), ch: make(chan any, 32)}
}

func (c *hevLoopClient) Send(frame any) error {
	c.ch <- frame
	return nil
}

func (c *hevLoopClient) Done() <-chan struct{} { return c.done }

func TestHostEvOfflineThroughRunLoop(t *testing.T) {
	db, err := store.Open(":memory:")
	equal(t, err, nil)
	defer db.Close()

	s := hevState(t)
	s.Seq = 0
	worker := s.AIs[hevWorker]
	worker.State, worker.Task = "working", hevTaskA
	s.AIs[hevWorker] = worker
	seed := s.Seal(home.Result{Rows: []home.Row{
		home.RowOf(s.Group),
		home.RowOf(s.People[ownerID]),
		home.RowOf(s.AIs[aiID]),
		home.RowOf(worker),
		home.RowOf(s.AIs[hevPeer]),
		home.RowOf(s.Tasks[hevTaskA]),
	}})
	err = db.Write(func(tx *store.Tx) error {
		for _, r := range seed.Rows {
			if _, err := tx.NextSeq(groupID); err != nil {
				return err
			}
			if err := r.Put(tx, groupID, clock); err != nil {
				return err
			}
		}
		return nil
	})
	equal(t, err, nil)

	current := now
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: node.ID(), Owner: browser.ID(), Tables: home.NodeTables(),
		Logs: func(user string) *identity.Log { return logOf(t, browser, node, laptop) },
		Now:  func() time.Time { return current },
	})
	equal(t, err, nil)
	defer loop.Close()

	watcher := hevNewClient()
	watchConn := loop.Attach(browser.ID(), watcher)
	_, ok := loop.Watch(watchConn, groupID, true)
	equal(t, ok, true)

	hostOne, hostTwo := hevNewClient(), hevNewClient()
	connOne := loop.Attach(laptop.ID(), hostOne)
	connTwo := loop.Attach(laptop.ID(), hostTwo)

	// Detaching one of two connections keeps laptop online, so HostOfflineEvent writes nothing.
	loop.Detach(connOne)
	equal(t, loop.Event(home.HostOfflineEvent, laptop.ID()), true)

	// Detaching the second connection makes laptop offline, and HostOfflineEvent pushes the offline rows.
	current = time.UnixMilli(clock + 2500)
	loop.Detach(connTwo)
	equal(t, loop.Event(home.HostOfflineEvent, laptop.ID()), true)

	select {
	case frame := <-watcher.ch:
		pushed, ok := frame.(proto.Rows)
		equal(t, ok, true)
		equal(t, len(pushed.Rows), 2)
		var got proto.AIMemberRow
		equal(t, proto.Decode(pushed.Rows[1], &got), nil)
		equal(t, got.ID, hevWorker)
		equal(t, got.State, "offline")
		equal(t, got.Task, "")
		equal(t, got.Since, int64(clock+2500))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for offline rows push")
	}
}
