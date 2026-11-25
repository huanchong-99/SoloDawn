package home_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
// (hevWorker on laptop), a working AI on node (hevPeer), and two tasks.
func hevState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Group.Name, s.Group.Kind, s.Group.Base, s.Group.Forge, s.Group.Home = "Website", "project", "main", "none", node.ID()
	s.Group.Checks = []string{}
	owner := s.People[ownerID]
	owner.Group, owner.Kind, owner.Name = groupID, "human", "owner"
	s.People[ownerID] = owner

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
	res, err := home.Commands.Apply(s, d, r, by.Sign(payload), now)
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
func hevInt(v int) *int           { return &v }
func hevStr(v string) *string     { return &v }

// Step-1 member-status wire golden files apply cleanly, and the resulting AI
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
	res, err := home.Commands.Apply(s, disk{}, receipts{}, laptop.Sign(string(statusBytes)), time.UnixMilli(1763600000000))
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
	res, err = home.Commands.Apply(s, disk{}, receipts{}, laptop.Sign(string(intrBytes)), time.UnixMilli(1763599999500))
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
		"member-status-rate_limited",
		"member-status-login_invalid",
		"member-status-session_replaced",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", name+".json"))
			equal(t, err, nil)
			st := makeGoldenState(10)
			out, err := home.Commands.Apply(st, disk{}, receipts{}, laptop.Sign(string(data)), time.UnixMilli(1763600000000))
			equal(t, err, nil)
			equal(t, out.Answer, store.Outcome{OK: true})
		})
	}
}

// State changes advance since, repeating the same state keeps since, usage is
// copied as sent, and omitting task clears it.
func TestHostEvStatusFieldsAndSince(t *testing.T) {
	s := hevState(t)
	raw := fmt.Sprintf(
		`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","task":%q,"path":"","usage":{"window_pct":42.5,"turns_today":7}}`,
		id(1), hevStatusTS, groupID, hevWorker, hevTaskA,
	)
	res := hevSend(t, s, disk{}, receipts{}, laptop, raw)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Items), 0)
	want := s.AIs[hevWorker]
	want.Seq, want.State, want.Task, want.Since, want.LastSeen, want.Path = 11, "working", hevTaskA, clock, clock, ""
	want.Usage = proto.Usage{WindowPct: hevFloat(42.5), TurnsToday: hevInt(7)}
	equal(t, briefRows(res), []any{want})
	s.Commit(res)

	res2, err := home.Commands.Apply(s, disk{}, receipts{}, laptop.Sign(hevPayload(2, hevWorker, "working", "relay", "")), time.UnixMilli(clock+900))
	equal(t, err, nil)
	want.Seq, want.Task, want.Path, want.Usage, want.LastSeen = 12, "", "relay", proto.Usage{}, clock+900
	equal(t, briefRows(res2), []any{want})
}

type hevEventCase struct {
	name       string
	statusTask string
	eventJSON  string
	wantThread string
	wantBody   any
	wantNotice *home.Item

	wantMention []string
}

func hevCheckEventRows(t *testing.T, s *home.State, tc hevEventCase, res home.Result) proto.MsgRow {
	t.Helper()
	wantAI := s.AIs[hevWorker]
	wantAI.Seq, wantAI.State, wantAI.Task, wantAI.Since, wantAI.LastSeen = 11, "limited", tc.statusTask, clock, clock
	var thread *string
	if tc.wantThread != "" {
		thread = hevStr(tc.wantThread)
	}
	wantMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(10), Seq: 12}, Pos: 12,
		Group: groupID, Thread: thread, By: "system", Kind: "system",
		Timestamp: clock, Mentions: append([]string{}, tc.wantMention...), Body: briefRaw(t, tc.wantBody),
	}
	if tc.wantThread == "" {
		equal(t, briefRows(res), []any{wantAI, wantMsg})
		return wantMsg
	}
	wantTask := s.Tasks[tc.wantThread]
	wantTask.Seq = 13
	wantTask.Thread.Count++
	wantTask.Thread.Last = &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock}
	equal(t, briefRows(res), []any{wantAI, wantMsg, wantTask})
	return wantMsg
}

// Step-1 host event codes write their system row in the task thread or main
// line, advance task.Thread when in a thread, and queue a secretary notice
// only for login_invalid when a secretary exists.
func TestHostEvEventsAndNotices(t *testing.T) {
	for _, tc := range []hevEventCase{
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
			name:      "turn_interrupted in main line with empty detail omitted",
			eventJSON: `{"code":"turn_interrupted","reason":"stop","detail":""}`,
			wantBody: proto.TurnInterruptedBody{
				SystemCode: proto.SystemCode{Code: "turn_interrupted"}, AI: hevWorker, Reason: "stop",
			},
		},
		{
			name:      "rate_limited in main line queues notice",
			eventJSON: `{"code":"rate_limited","resets":1763603600000}`,
			wantBody: proto.RateLimitedBody{
				SystemCode: proto.SystemCode{Code: "rate_limited"}, AI: hevWorker, Resets: 1763603600000,
			},
			wantNotice: &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "rate_limited"},
		},
		{
			name:       "login_invalid with detail stays in main line when status names a task",
			statusTask: hevTaskB,
			eventJSON:  `{"code":"login_invalid","detail":"Token expired."}`,
			wantBody: proto.LoginInvalidBody{
				SystemCode: proto.SystemCode{Code: "login_invalid"}, AI: hevWorker, Detail: hevStr("Token expired."),
			},
			wantNotice:  &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "login_invalid"},
			wantMention: []string{ownerID},
		},
		{
			name:      "login_invalid without detail",
			eventJSON: `{"code":"login_invalid"}`,
			wantBody: proto.LoginInvalidBody{
				SystemCode: proto.SystemCode{Code: "login_invalid"}, AI: hevWorker,
			},
			wantNotice:  &home.Item{AI: aiID, Why: "notice", Msg: id(10), Code: "login_invalid"},
			wantMention: []string{ownerID},
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
			wantMsg := hevCheckEventRows(t, s, tc, res)

			var wantItems []home.Item
			if tc.wantNotice != nil {
				wantItems = []home.Item{*tc.wantNotice}
			}
			equal(t, res.Items, wantItems)

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
}

type hevRefusalCase struct {
	by      identity.Key
	member  string
	extra   string
	raw     string
	wantErr string
}

func hevRefusalInput(tc hevRefusalCase) (identity.Key, string) {
	by := laptop
	if tc.by != nil {
		by = tc.by
	}
	if tc.raw != "" {
		return by, tc.raw
	}
	member := hevWorker
	if tc.member != "" {
		member = tc.member
	}
	return by, hevPayload(25, member, "working", "local", tc.extra)
}

func TestHostEvStatusRefusals(t *testing.T) {
	for name, tc := range map[string]hevRefusalCase{
		"missing required path": {
			raw:     fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":"working","usage":{}}`, id(25), hevStatusTS, groupID, hevWorker),
			wantErr: "bad_request",
		},
		"event without code":            {extra: `,"event":{}`, wantErr: "bad_request"},
		"unknown event code":            {extra: `,"event":{"code":"no_secretary"}`, wantErr: "bad_request"},
		"postponed cap_reached code":    {extra: `,"event":{"code":"cap_reached","cap":"day"}`, wantErr: "bad_request"},
		"rate_limited missing resets":   {extra: `,"event":{"code":"rate_limited"}`, wantErr: "bad_request"},
		"turn_interrupted no reason":    {extra: `,"event":{"code":"turn_interrupted"}`, wantErr: "bad_request"},
		"login_invalid non-text detail": {extra: `,"event":{"code":"login_invalid","detail":7}`, wantErr: "bad_request"},
		"session_replaced bad task":     {extra: `,"event":{"code":"session_replaced","task":"bad"}`, wantErr: "bad_request"},
		"unknown member":                {member: id(99), wantErr: "not_found"},
		"human member":                  {member: ownerID, wantErr: "not_found"},
		"signed by owner browser":       {by: browser, wantErr: "forbidden"},
		"signed by another host node":   {by: node, wantErr: "forbidden"},
		"signed with as by own host":    {extra: fmt.Sprintf(`,"as":%q`, hevWorker), wantErr: "forbidden"},
		"unknown event task":            {extra: fmt.Sprintf(`,"event":{"code":"session_replaced","task":%q}`, id(98)), wantErr: "not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			by, payload := hevRefusalInput(tc)
			hevRefused(t, hevSend(t, hevState(t), disk{}, receipts{}, by, payload), tc.wantErr)
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
// order and leaves already-offline AIs and other hosts untouched.
func TestHostEvOfflineHandler(t *testing.T) {
	s := hevState(t)
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

	equal(t, home.Events[home.HostOfflineEvent](s, disk{}, clock, 42), home.Result{})
	equal(t, home.Events[home.HostOfflineEvent](s, disk{}, clock, guest.ID()), home.Result{})

	res := s.Seal(home.Events[home.HostOfflineEvent](s, disk{}, clock, laptop.ID()))
	equal(t, res.Answer, store.Outcome{OK: true})

	wantSec := sec
	wantSec.Seq, wantSec.State, wantSec.Task, wantSec.Since = 11, "offline", "", clock
	wantWorker := worker
	wantWorker.Seq, wantWorker.State, wantWorker.Task, wantWorker.Since = 12, "offline", "", clock
	equal(t, briefRows(res), []any{wantSec, wantWorker})

	s.Commit(res)
	equal(t, s.AIs[alreadyOff].Since, int64(clock-9000))
	equal(t, s.AIs[hevPeer].State, "working")
	equal(t, s.AIs[hevPeer].Task, hevTaskA)
}

type hevLoopClient struct {
	done    chan struct{}
	ch      chan any
	started chan struct{}
	hold    chan struct{}
	refuse  atomic.Bool
}

func hevNewClient() *hevLoopClient {
	return &hevLoopClient{done: make(chan struct{}), ch: make(chan any, 2048), started: make(chan struct{}, 2048)}
}

func (c *hevLoopClient) Send(frame any) error {
	c.started <- struct{}{}
	if c.hold != nil {
		<-c.hold
	}
	if c.refuse.Load() {
		return errors.New("send failed")
	}
	c.ch <- frame
	return nil
}

func (c *hevLoopClient) Done() <-chan struct{} { return c.done }

func hevSeedStore(t *testing.T, db *store.Store) {
	t.Helper()
	s := hevState(t)
	s.Seq = 0
	worker := s.AIs[hevWorker]
	worker.State, worker.Task = "working", hevTaskA
	seed := s.Seal(home.Result{Rows: []home.Row{
		home.RowOf(s.Group), home.RowOf(s.People[ownerID]), home.RowOf(s.AIs[aiID]),
		home.RowOf(worker), home.RowOf(s.AIs[hevPeer]), home.RowOf(s.Tasks[hevTaskA]),
	}})
	err := db.Write(func(tx *store.Tx) error {
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
}

func hevExpectAIState(t *testing.T, watcher *hevLoopClient, wantCount int, wantState, wantTask string, wantSince int64) int64 {
	t.Helper()
	select {
	case frame := <-watcher.ch:
		pushed, ok := frame.(proto.Rows)
		equal(t, ok, true)
		equal(t, len(pushed.Rows), wantCount)
		var got proto.AIMemberRow
		equal(t, proto.Decode(pushed.Rows[wantCount-1], &got), nil)
		equal(t, got.ID, hevWorker)
		equal(t, got.State, wantState)
		equal(t, got.Task, wantTask)
		equal(t, got.Since, wantSince)
		return got.Seq
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s rows push", wantState)
		return 0
	}
}

func hevSyncWatch(t *testing.T, loop *home.Loop, conn *home.Conn, watcher *hevLoopClient) {
	t.Helper()
	loop.Sync(conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 1}, Group: groupID, StateOnly: true})
	equal(t, <-watcher.ch, proto.Reset{Type: "reset", Group: groupID})
	pushed, ok := (<-watcher.ch).(proto.Rows)
	equal(t, ok && len(pushed.Rows) == 6, true)
	equal(t, <-watcher.ch, proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: groupID, Seq: 6})
}

func hevSyncBarrier(t *testing.T, loop *home.Loop, conn *home.Conn, watcher *hevLoopClient, re, seq int64) {
	t.Helper()
	loop.Sync(conn, proto.Sync{Request: proto.Request{Type: "sync", ID: re}, Group: groupID, Since: seq, StateOnly: true})
	select {
	case reply := <-watcher.ch:
		equal(t, reply, proto.Synced{Type: "synced", Success: proto.Success{Re: re, OK: true}, Group: groupID, Seq: seq})
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for sync reply %d", re)
	}
}

// Loop automatically fires HostOfflineEvent when a device's last taker
// connection ends via Detach, Send failure, or backlog overflow, and not for
// browser/non-taker connections or while another taker of that device stays open.
func TestHostEvOfflineThroughRunLoop(t *testing.T) {
	db, err := store.Open(":memory:")
	equal(t, err, nil)
	defer db.Close()
	hevSeedStore(t, db)

	var current atomic.Int64
	current.Store(clock)
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: node.ID(), Owner: browser.ID(), Tables: home.NodeTables(),
		Logs: func(string) *identity.Log { return logOf(t, browser, node, laptop) },
		Now:  func() time.Time { return time.UnixMilli(current.Load()) },
	})
	equal(t, err, nil)
	defer loop.Close()

	watcher := hevNewClient()
	watchConn := loop.Attach(browser.ID(), watcher)
	hevSyncWatch(t, loop, watchConn, watcher)
	var syncRe int64 = 1
	var seq int64 = 6
	syncLoop := func() {
		syncRe++
		hevSyncBarrier(t, loop, watchConn, watcher, syncRe, seq)
	}

	// Another host's taker stays attached throughout; browser and non-taker laptop detaches do nothing.
	nodeConn := loop.Attach(node.ID(), hevNewClient())
	loop.Inbox(nodeConn)
	loop.Detach(loop.Attach(browser.ID(), hevNewClient()))
	loop.Detach(loop.Attach(laptop.ID(), hevNewClient()))
	syncLoop()
	equal(t, len(watcher.ch), 0)

	// Two taker connections on laptop: detaching the first leaves the second, so laptop stays online.
	connOne := loop.Attach(laptop.ID(), hevNewClient())
	connTwo := loop.Attach(laptop.ID(), hevNewClient())
	loop.Inbox(connOne)
	loop.Inbox(connTwo)
	loop.Detach(connOne)
	syncLoop()
	equal(t, len(watcher.ch), 0)

	// Even with a non-taker laptop connection attached, detaching the last taker marks laptop AIs offline.
	nonTaker := loop.Attach(laptop.ID(), hevNewClient())
	current.Store(clock + 2500)
	loop.Detach(connTwo)
	hevExpectAIState(t, watcher, 2, "offline", "", clock+2500)

	// Setting hevWorker working again on nonTaker and re-detaching connTwo does not fire offline a second time.
	current.Store(clock + 3000)
	loop.Cmd(nonTaker, 1, laptop.Sign(hevPayload(40, hevWorker, "working", "direct", fmt.Sprintf(`,"task":%q`, hevTaskA))))
	seq = hevExpectAIState(t, watcher, 1, "working", hevTaskA, clock+3000)
	loop.Detach(connTwo)
	loop.Detach(nonTaker)
	syncLoop()
	equal(t, len(watcher.ch), 0)

	// A taker connection whose Send fails triggers Detach and marks hevWorker offline.
	failClient := hevNewClient()
	connThree := loop.Attach(laptop.ID(), failClient)
	loop.Inbox(connThree)
	failClient.refuse.Store(true)
	current.Store(clock + 4000)
	loop.Welcome(connThree, runWelcome)
	runWait(t, connThree.Gone())
	hevExpectAIState(t, watcher, 1, "offline", "", clock+4000)

	// A taker connection given up on backlog overflow also marks hevWorker offline.
	slowClient := hevNewClient()
	slowClient.hold = make(chan struct{})
	defer close(slowClient.hold)
	connFour := loop.Attach(laptop.ID(), slowClient)
	loop.Inbox(connFour)
	current.Store(clock + 5000)
	loop.Cmd(connFour, 1, laptop.Sign(hevPayload(41, hevWorker, "working", "direct", fmt.Sprintf(`,"task":%q`, hevTaskA))))
	runWait(t, slowClient.started)
	hevExpectAIState(t, watcher, 1, "working", hevTaskA, clock+5000)
	current.Store(clock + 6000)
	for range 1025 {
		loop.Welcome(connFour, runWelcome)
	}
	runWait(t, connFour.Gone())
	hevExpectAIState(t, watcher, 1, "offline", "", clock+6000)
}
