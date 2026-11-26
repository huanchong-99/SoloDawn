package home

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestConfirmAuthorityAndParticipation(t *testing.T) {
	for _, account := range []string{"personal", "team", "api_key"} {
		ai := proto.AIMemberRow{MemberRow: proto.MemberRow{User: "owner"}, Account: account, Allow: []string{"listed"}}
		cardTestSame(t, cfmEnough("owner", ai), true)
		cardTestSame(t, cfmEnough("other", ai), false)
		cardTestSame(t, cfmEnough("listed", ai), account != "personal")
	}
	for _, tc := range []struct {
		step, who, reviewed string
		started             int64
		want                bool
	}{
		{"work", "assignee", "", 1, true},
		{"work", "assignee", "", 0, false},
		{"work", "reviewer", "", 1, false},
		{"review", "reviewer", "", 1, true},
		{"work", "reviewer", "reviewer", 1, true},
		{"work", "reviewer", "former", 1, false},
		{"review", "other", "", 1, false},
	} {
		task := proto.TaskRow{State: "active", Step: tc.step, Assignee: "assignee", Reviewer: "reviewer"}
		if tc.started == 0 {
			task.State = "queued"
		}
		if tc.reviewed != "" {
			task.Review = &proto.Review{By: tc.reviewed}
		}
		cardTestSame(t, cfmTakesPart(task, tc.who), tc.want)
	}
	s := NewState("g", "h", "owner")
	s.AIs["ai"] = proto.AIMemberRow{MemberRow: proto.MemberRow{User: "owner"}}
	taskID := "task"
	s.Tasks[taskID] = proto.TaskRow{State: "active", Assignee: "ai"}
	for _, state := range []string{"done", "cancelled"} {
		task := proto.TaskRow{State: state, Step: "review", Assignee: "a", Reviewer: "r"}
		cardTestSame(t, cfmTakesPart(task, "a"), true)
		cardTestSame(t, cfmTakesPart(task, "r"), true)
		cardTestSame(t, cfmTakesPart(task, "former"), false)
	}
	cardTestSame(t, s.cfmAllowed(cardTestDisk{}, "other", &taskID, "ai"), true)
	cardTestSame(t, s.cfmAllowed(cardTestDisk{}, "other", nil, "ai"), false)
	wakes := map[string][]int64{"ai": {10}}
	it := Item{AI: "ai", Why: "watch"}
	cardTestSame(t, inboxWake(wakes, it, 1, 20), it)
	cardTestSame(t, wakes, map[string][]int64{"ai": {10}})
}
