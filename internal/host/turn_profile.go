package host

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func profileMembers(rows *Rows, group string) []proto.AIMemberRow {
	var members []proto.AIMemberRow
	for _, raw := range rows.Of(group, "member") {
		var ai proto.AIMemberRow
		if proto.Decode(raw, &ai) == nil && ai.Kind == "ai" {
			members = append(members, ai)
		}
	}
	slices.SortStableFunc(members, func(a, b proto.AIMemberRow) int { return cmp.Compare(a.Name, b.Name) })
	return members
}

func (c *turnCtx) profileRoster() string {
	lines := []string{"AI members (CLI on computer · tier · state · skills):"}
	for _, ai := range profileMembers(c.h.Rows, c.s.Group) {
		lines = append(lines, c.profileMemberLine(ai))
	}
	return strings.Join(lines, "\n")
}

func (c *turnCtx) profileMemberLine(ai proto.AIMemberRow) string {
	line := fmt.Sprintf("- %q · %s on %s · %s · %s", ai.Name, ai.CLI, ai.HostName, profileTier(ai), ai.State)
	if ai.Task != "" {
		line += " on " + c.taskRef(ai.Task)
	}
	if ai.Skills != "" {
		line += " · " + ai.Skills
	}
	return line
}

func profileTier(ai proto.AIMemberRow) string {
	return cmp.Or(ai.Tier, "standard")
}

func profileStrength(ai proto.AIMemberRow) int {
	return slices.Index([]string{"light", "standard", "strong"}, profileTier(ai))
}

func profileCandidates(rows *Rows, group, assignee string) string {
	members := profileMembers(rows, group)
	index := slices.IndexFunc(members, func(ai proto.AIMemberRow) bool { return ai.ID == assignee })
	if index < 0 {
		return "none currently listed"
	}
	var names []string
	strength := profileStrength(members[index])
	for _, ai := range members {
		if profileStrength(ai) > strength {
			names = append(names, fmt.Sprintf("%q (%s)", ai.Name, profileTier(ai)))
		}
	}
	return cmp.Or(strings.Join(names, ", "), "none currently listed")
}

func profileMustAsk(task proto.TaskRow) bool {
	return task.Stuck != nil && task.Stuck.Ask
}

func (c *turnCtx) appendEscalation(body string, p parsedItem) string {
	task, ok := c.lookupTask(itemThread(string(p.item.Task), p.msg))
	if !ok || !profileMustAsk(task) {
		return body
	}
	line := fmt.Sprintf("This is failure %d in a row of the same kind (%s). Before you submit again, call ask to one of: %s. Then continue with the answer.",
		task.Stuck.N, task.Stuck.Kind, profileCandidates(c.h.Rows, c.s.Group, task.Assignee))
	if body == "" {
		return line
	}
	return body + "\n" + line
}

func profileRequireAnswered(rows *Rows, group string, task proto.TaskRow) error {
	if profileMustAsk(task) {
		return &Error{Code: "conflict", Message: "Ask a stronger AI before submitting again. Candidates: " + profileCandidates(rows, group, task.Assignee) + "."}
	}
	return nil
}
