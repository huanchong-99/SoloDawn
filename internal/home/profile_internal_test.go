package home

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestProfileHasStronger(t *testing.T) {
	s := NewState("g", "h", "p")
	task := proto.TaskRow{Assignee: "a"}
	s.AIs["a"] = proto.AIMemberRow{Profile: proto.Profile{Tier: "standard"}}
	s.AIs["b"] = proto.AIMemberRow{Profile: proto.Profile{Tier: "standard"}}
	cardTestSame(t, s.profileHasStronger(task, Event{Kind: OnReview}), false)
	s.AIs["b"] = proto.AIMemberRow{Profile: proto.Profile{Tier: "strong"}}
	cardTestSame(t, s.profileHasStronger(task, Event{Kind: OnReview}), true)
}
