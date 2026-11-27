package proto

import (
	"encoding/json"
	"testing"
)

// report is the id of the example report message: the submission under review.
var report = id("report")

// taskExample is task 12 at its review, sent back once and submitted again.
func taskExample() TaskRow {
	return TaskRow{
		RowHeader: RowHeader{"task", taskID, 805}, Group: group, Num: 12, Title: "Show connection path",
		Origin: "brief", Brief: id("brief"), State: "active", Step: "review", Report: report,
		Assignee: agent, Reviewer: reviewer, Pass: 90, Depends: []string{},
		Review: &Review{1, 1, 86, false, reviewer, id("earlier report"), ts - 1000},
		Thread: ThreadSummary{7, 3, &ThreadMessage{agent, "progress", "Added the path indicator.", ts - 500}},
		Stats:  TaskStats{ts - 4000, nil}, Created: ts - 4000,
	}
}

func TestGroupGoldens(t *testing.T) {
	ai, human, task := id("agent"), id("member"), id("task")
	project := GroupRow{
		RowHeader: RowHeader{"group", group, 1}, Name: "Website", Home: host, Secretary: id("secretary"),
		Pass: 90, PassFoundation: 70, Limits: Limits{10, 20, 60, 3, 5, 4, 45, 600, 30}, Created: ts - 5000,
	}
	golden(t, "row-group", project)
	project.Seq = 816
	project.Paused = &GroupPause{By: human, At: ts}
	golden(t, "row-group-paused", project)
	golden(t, "row-member-human", HumanMemberRow{
		MemberRow: MemberRow{RowHeader{"member", human, 3}, group, "human", user, "owner", ts},
		Role:      "owner", Online: true,
	})
	aiRow := AIMemberRow{
		MemberRow: MemberRow{RowHeader{"member", ai, 789}, group, "ai", user, "Codex", ts},
		CLI:       "codex", Host: host, HostName: "workstation", Workspace: workspace, Account: "personal", Allow: []string{},
		Approvers: []string{user}, State: "working", Task: task, Since: ts - 2000, Path: "local",
		Usage: Usage{WindowPct: new(62.5), WindowResets: new(ts + 10000), DayCapPct: new(80.0),
			SpendToday: new(0.0), SpendCap: new(10.5), TurnsToday: new(14)},
		Profile: Profile{"gpt-5.4", "high", "strong", "Go backend, protocol design"},
	}
	golden(t, "row-member-ai", aiRow)
	aiFollow := aiRow
	aiFollow.Follow = true
	golden(t, "row-member-ai-follow", aiFollow)
	golden(t, "row-member-ai-external", AIMemberRow{
		MemberRow: MemberRow{RowHeader{"member", id("scout"), 812}, group, "ai", user, "Scout", ts},
		CLI:       "external", Tool: "ZCode", Host: host, HostName: "workstation", Workspace: workspace, Account: "api_key",
		Allow: []string{}, Approvers: []string{user}, State: "waiting", Since: ts - 1000000, Path: "local",
		Usage:   Usage{WindowPct: new(40.0), WindowResets: new(ts + 11200000)},
		Profile: Profile{Tier: "standard", Skills: "Rust, reviews"},
	})
	golden(t, "row-device", DeviceRow{
		RowHeader{"device", user + ":2", 41}, addNode.DeviceChange, signed(browser, " "+string(wireJSON(t, addNode))),
	})
	taskRow := taskExample()
	golden(t, "row-task", taskRow)
	stuck := taskExample()
	stuck.Stuck = &Stuck{"review", 2, true}
	golden(t, "row-task-stuck", stuck)
	waiting := TaskRow{
		RowHeader: RowHeader{"task", id("local task"), 817}, Group: group, Num: 13, Title: "Choose a label",
		Origin: "direct", Plan: "Ask the owner which label the path gets.",
		State: "active", Step: "work", Assignee: ai, Foundation: true, Pass: 70,
		Depends: []string{}, Wait: &TaskWait{Kind: "decision", Msg: id("local decision"), Who: []string{user}, Since: ts},
		Stats: TaskStats{Started: ts}, Created: ts,
	}
	golden(t, "row-task-waiting", waiting)
	// Paused by the secretary while it waited for a reviewer: the pause keeps that wait
	// and names who paused (contracts/task-machine.md section 2, row 12).
	paused := TaskRow{
		RowHeader: RowHeader{"task", id("paused task"), 818}, Group: group, Num: 14, Title: "Fix: the relay notice!",
		Origin: "direct", State: "paused", Step: "review", Report: id("paused report"),
		Assignee: ai, Pass: 90, Depends: []string{},
		Wait:   &TaskWait{Kind: "manual", Since: ts, Kept: "reviewer", By: secretary},
		Thread: ThreadSummary{Count: 2}, Stats: TaskStats{ts - 4000, nil}, Created: ts - 4000,
	}
	golden(t, "row-task-paused", paused)
	confirmTitle := "Node: report connection path changes"
	confirmWait := TaskRow{
		RowHeader: RowHeader{"task", id("confirm task"), 819}, Group: group, Num: 15, Title: confirmTitle,
		Origin: "brief", Brief: id("brief"),
		State: "active", Step: "work", Assignee: ai, Reviewer: reviewer, Pass: 90,
		Depends: []string{}, Wait: &TaskWait{Kind: "confirm", Msg: id("confirm"), Who: []string{user}, Since: ts},
		Stats: TaskStats{Started: ts}, Created: ts,
	}
	golden(t, "row-task-confirm", confirmWait)

	msg := decidedExample(t)
	note := NoteRow{
		RowHeader: RowHeader{"note", id("note"), 640}, Pos: 640, Group: group,
		Text: "Path events include the connection id.", By: human, Timestamp: ts - 4000,
	}
	golden(t, "row-note", note)
	golden(t, "row-note-preset", NoteRow{
		RowHeader: RowHeader{"note", id("preset note"), 902}, Pos: 871, Group: group,
		NoteType: "preset", Name: "Calm dashboard",
		Text: "Principles: one accent colour; no motion longer than 200 ms.",
		By:   human, Timestamp: ts + 100000,
	})
	golden(t, "row-note-kb", NoteRow{
		RowHeader: RowHeader{"note", id("kb note"), 915}, Pos: 915, Group: group,
		NoteType: "kb", Name: "Decision: the home is the single writer",
		Text: "Every change to a group goes through its home.",
		By:   ai, Timestamp: ts + 101000,
	})
	gone := GoneRow{RowHeader{"note", note.ID, 815}, true}
	golden(t, "row-gone", gone)

	golden(t, "sync", Sync{Request{"sync", 2}, group, 0, false})
	golden(t, "reset", Reset{"reset", group})
	golden(t, "rows", Rows{"rows", group, []json.RawMessage{wireJSON(t, taskRow), wireJSON(t, msg), wireJSON(t, gone)}})
	golden(t, "synced", Synced{"synced", Success{2, true}, group, 815})

	page := FetchPage{Request{"fetch", 3}, group, 800, 50}
	golden(t, "fetch", FetchThread{page, &task})
	golden(t, "fetch-main", FetchThread{page, nil})
	golden(t, "fetch-kind", FetchKind{FetchPage: page, Kind: "note"})
	golden(t, "fetch-preset", FetchKind{FetchPage: FetchPage{Request{"fetch", 7}, group, 0, 50}, Kind: "preset"})
	golden(t, "fetch-kb", FetchKind{FetchPage: FetchPage{Request{"fetch", 9}, group, 0, 5}, Kind: "kb", Query: "single writer 路径"})
	golden(t, "fetch-row", FetchRow{FetchPage{Request{"fetch", 3}, group, 0, 1}, "task", task})
	golden(t, "fetched", Fetched{"fetched", Success{3, true}, []json.RawMessage{wireJSON(t, msg)}, false})

	act := msg.State.Act
	golden(t, "cmd", Cmd{Request{"cmd", 4}, act})
	var header PayloadHeader
	if err := json.Unmarshal([]byte(act.Payload), &header); err != nil {
		t.Fatal(err)
	}
	if want := (PayloadHeader{Type: "act", Nonce: ID(id("act")), Timestamp: ts, Group: ID(group)}); header != want {
		t.Fatalf("command payload header = %+v, want %+v", header, want)
	}
	golden(t, "cmd-ok", CmdResult{Success: Success{4, true}})
	golden(t, "cmd-started", CmdResult{Success: Success{5, true}, Num: 12})
	golden(t, "cmd-reviewed", CmdResult{Success: Success{6, true}, Passed: new(false), Round: 1})
	golden(t, "cmd-pair-link", CmdResult{Success: Success{7, true}, Link: "https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N", Expires: ts + 600000})
	golden(t, "cmd-noted", CmdResult{Success: Success{9, true}, ID: id("preset note"), Replaced: new(false)})
	golden(t, "cmd-invite-link", CmdResult{
		Success: Success{10, true},
		Link:    "https://rv.example/#join=7K2M-9QXD-4F8T-WB3N&g=" + group + "&home=" + host,
		Expires: ts + 600000,
	})
	golden(t, "cmd-code", CmdResult{Success: Success{4, true}, Code: "7K2M-9QXD-4F8T-WB3N", Expires: ts + 86_400_000})
	golden(t, "cmd-error", ErrorResponse{8, WireError{Code: "conflict", Message: "This card has already been decided."}})
	golden(t, "cmd-moved", ErrorResponse{8, WireError{Code: "moved", Message: "The group has moved.", Home: host}})
}
