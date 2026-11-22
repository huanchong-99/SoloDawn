package home

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestTaskCmdSender(t *testing.T) {
	s := NewState("grp", "home", "owner")
	s.Group.Secretary = "sec"
	s.People["adm"] = proto.HumanMemberRow{Role: "admin"}
	s.People["mem"] = proto.HumanMemberRow{Role: "member"}
	s.AIs["sec"] = proto.AIMemberRow{Host: "node-1"}
	s.AIs["ai-a"] = proto.AIMemberRow{Host: "node-1"}

	for name, tc := range map[string]struct {
		who  Who
		want Sender
	}{
		"secretary":  {Who{Device: "node-1", Human: "adm", AI: "sec"}, BySecretary},
		"working AI": {Who{Device: "node-1", Human: "adm", AI: "ai-a"}, ByAI},
		"host":       {Who{Device: "node-1", Human: "adm"}, ByHost},
		"admin":      {Who{Device: "node-2", Human: "adm"}, ByAdmin},
		"person":     {Who{Device: "node-2", Human: "mem"}, ByPerson},
	} {
		if got := s.tskSender(tc.who, "ai-a"); got != tc.want {
			t.Fatalf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}
