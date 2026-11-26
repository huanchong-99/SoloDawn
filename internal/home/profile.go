package home

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func profilePresent(p proto.ProfileSet) bool {
	return p != (proto.ProfileSet{})
}

func profileValidate(p proto.ProfileSet) *Refusal {
	if !boundedMemberText(p.Model, 100) || !boundedMemberText(p.Effort, 32) {
		return &Refusal{"bad_request", "model and effort allow up to 100 and 32 code points, respectively, without control characters"}
	}
	if !profileTierValid(p.Tier) {
		return &Refusal{"bad_request", "tier must be light, standard or strong"}
	}
	if p.Skills != nil && !profileSkills(*p.Skills) {
		return &Refusal{"bad_request", "skills must be one line of up to 200 code points"}
	}
	return nil
}

func boundedMemberText(text *string, limit int) bool {
	return text == nil || (utf8.RuneCountInString(*text) <= limit && strings.IndexFunc(*text, unicode.IsControl) == -1)
}

func profileSkills(text string) bool {
	return utf8.RuneCountInString(text) <= 200 && !strings.ContainsAny(text, "\r\n")
}

func profileTierValid(tier *string) bool {
	return tier == nil || profileTier(*tier) != 0
}

func profileTier(tier string) int {
	switch tier {
	case "light":
		return 1
	case "standard":
		return 2
	case "strong":
		return 3
	default:
		return 0
	}
}

func profileUpdate(profile proto.Profile, p proto.ProfileSet) proto.Profile {
	if profile.Tier == "" {
		profile.Tier = "standard"
	}
	grpSetOpt(&profile.Model, p.Model)
	grpSetOpt(&profile.Effort, p.Effort)
	grpSetOpt(&profile.Tier, p.Tier)
	grpSetOpt(&profile.Skills, p.Skills)
	return profile
}

func profileMustAsk(task proto.TaskRow) bool {
	return task.Stuck != nil && task.Stuck.Ask
}

// profileFailure counts a review that did not pass, the one kind of failure
// that is counted (planning/contracts/step-3/profiles.md section 4).
func profileFailure(stuck *proto.Stuck, stronger bool) *proto.Stuck {
	n := 1
	if stuck != nil {
		n += stuck.N
	}
	return &proto.Stuck{Kind: "review", N: n, Ask: n >= 2 && stronger}
}

func profileStrength(ai proto.AIMemberRow) int {
	if ai.Tier == "" {
		return profileTier("standard")
	}
	return profileTier(ai.Tier)
}

func (s *State) profileHasStronger(task proto.TaskRow, ev Event) bool {
	if ev.OK || ev.Kind != OnReview {
		return false
	}
	strength := profileStrength(s.AIs[task.Assignee])
	for _, ai := range s.AIs {
		if profileStrength(ai) > strength {
			return true
		}
	}
	return false
}

func profileStepRefusal(task proto.TaskRow, ev Event, err error) *Refusal {
	msg := "the task machine refused the command"
	if ev.Kind == OnReport && err == RefusedConflict && profileMustAsk(task) {
		msg = "ask a stronger AI before submitting again"
	}
	return &Refusal{err.Error(), msg}
}

func (s *State) profileReplyRows(reply, call proto.MsgRow) []Row {
	if reply.Thread == nil {
		return []Row{RowOf(reply)}
	}
	task := postAdvanceThread(s.Tasks[*reply.Thread], reply)
	if s.profileAnswered(task, reply, call) {
		stuck := *task.Stuck
		stuck.Ask = false
		task.Stuck = &stuck
	}
	return []Row{RowOf(reply), RowOf(task)}
}

func (s *State) profileAnswered(task proto.TaskRow, reply, call proto.MsgRow) bool {
	return profileMustAsk(task) && call.By == task.Assignee &&
		profileStrength(s.AIs[reply.By]) > profileStrength(s.AIs[task.Assignee])
}
