package proto

import (
	"encoding/json"
	"testing"
)

func TestGroupGoldens(t *testing.T) {
	ai, human, task := id("agent"), id("member"), id("task")
	commit := "9f2c1abf43f3224eb6826ef971a943dbab4795ce"
	project := GroupRow{
		RowHeader: RowHeader{"group", group, 1}, Name: "Website", Kind: "project",
		Repo: "https://git.example/team/website.git", Base: "main", Forge: "github",
		Home: host, Secretary: id("secretary"), Checks: []string{"go vet ./...", "go test ./..."},
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
	golden(t, "row-member-ai", AIMemberRow{
		MemberRow: MemberRow{RowHeader{"member", ai, 789}, group, "ai", user, "Codex", ts},
		CLI:       "codex", Host: host, HostName: "workstation", Account: "personal", Allow: []string{},
		Approvers: []string{user}, State: "working", Task: task, Since: ts - 2000, Path: "local",
		Usage: Usage{WindowPct: new(62.5), WindowResets: new(ts + 10000), DayCapPct: new(80.0),
			SpendToday: new(0.0), SpendCap: new(10.5), TurnsToday: new(14)},
	})
	golden(t, "row-device", DeviceRow{
		RowHeader{"device", user + ":2", 41}, addNode.DeviceChange, signed(browser, " "+string(wireJSON(t, addNode))),
	})
	taskRow := TaskRow{
		RowHeader: RowHeader{"task", task, 805}, Group: group, Num: 12, Title: "Show connection path",
		Origin: "brief", Brief: id("brief"), State: "active", Step: "review", Commit: commit,
		Assignee: ai, Reviewer: id("reviewer"), Pass: 90, Depends: []string{},
		Branch: "task/12-path", PR: 31, PRURL: "https://git.example/team/website/pull/31",
		Checks: TaskChecks{&Check{true, ts - 3000, commit}, &Check{true, ts - 2000, commit}},
		Review: &Review{1, 86, false, id("reviewer"), commit, ts - 1000},
		Thread: ThreadSummary{7, 3, &ThreadMessage{ai, "progress", "Added the path indicator.", ts - 500}},
		Stats:  TaskStats{212, 18, ts - 4000, nil}, Created: ts - 4000,
	}
	golden(t, "row-task", taskRow)
	waiting := TaskRow{
		RowHeader: RowHeader{"task", id("local task"), 817}, Group: group, Num: 13, Title: "Choose a label",
		Origin: "direct", State: "active", Step: "branch", Assignee: ai, Foundation: true, Pass: 70,
		Depends: []string{}, Wait: &TaskWait{"decision", id("local decision"), []string{user}, ts},
		Stats: TaskStats{Started: ts}, Created: ts,
	}
	golden(t, "row-task-waiting", waiting)

	msg := decidedExample(t)
	note := NoteRow{RowHeader{"note", id("note"), 640}, 640, group, nil, "Path events include the connection id.", human, ts - 4000}
	golden(t, "row-note", note)
	gone := GoneRow{RowHeader{"note", note.ID, 815}, true}
	golden(t, "row-gone", gone)

	golden(t, "sync", Sync{Request{"sync", 2}, group, 0, false})
	golden(t, "reset", Reset{"reset", group})
	golden(t, "rows", Rows{"rows", group, []json.RawMessage{wireJSON(t, taskRow), wireJSON(t, msg), wireJSON(t, gone)}})
	golden(t, "synced", Synced{"synced", Success{2, true}, group, 815})

	page := FetchPage{Request{"fetch", 3}, group, 800, 50}
	golden(t, "fetch", FetchThread{page, &task})
	golden(t, "fetch-main", FetchThread{page, nil})
	golden(t, "fetch-kind", FetchKind{page, "note"})
	golden(t, "fetched", Fetched{"fetched", Success{3, true}, []json.RawMessage{wireJSON(t, msg)}, false})

	act := msg.State.Act
	golden(t, "cmd", Cmd{Request{"cmd", 4}, act})
	var header PayloadHeader
	if err := json.Unmarshal([]byte(act.Payload), &header); err != nil {
		t.Fatal(err)
	}
	if want := (PayloadHeader{Type: "act", Nonce: id("act"), Timestamp: ts, Group: group}); header != want {
		t.Fatalf("command payload header = %+v, want %+v", header, want)
	}
	golden(t, "cmd-ok", CmdResult{Success: Success{4, true}})
	golden(t, "cmd-started", CmdResult{Success: Success{5, true}, Num: 12})
	golden(t, "cmd-reviewed", CmdResult{Success: Success{6, true}, Passed: new(false), Round: 1})
	golden(t, "cmd-pair-link", CmdResult{Success: Success{7, true}, Link: "https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N", Expires: ts + 600000})
	golden(t, "cmd-error", ErrorResponse{8, WireError{"conflict", "This card has already been decided."}})
}
