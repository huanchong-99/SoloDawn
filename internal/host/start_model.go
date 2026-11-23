package host

import (
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func memberLaunch(h *Host, s Session, launch Launch) Launch {
	member, _ := lookupRow[proto.AIMemberRow](h, s.Group, "member", s.Member)
	launch.Model, launch.Effort = member.Model, member.Effort
	return launch
}

func claudeModelFlags(l Launch) []string {
	var flags []string
	if l.Model != "" {
		flags = append(flags, "--model", l.Model)
	}
	if l.Effort != "" {
		flags = append(flags, "--effort", l.Effort)
	}
	return flags
}

func codexModelFlags(l Launch) []string {
	var flags []string
	if l.Model != "" {
		flags = append(flags, "-m", l.Model)
	}
	if l.Effort != "" {
		flags = append(flags, "-c", "model_reasoning_effort="+marshalWire(l.Effort))
	}
	return flags
}

func codexSessionMissing(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "no rollout found for thread id") || strings.Contains(lower, "thread not found")
}
