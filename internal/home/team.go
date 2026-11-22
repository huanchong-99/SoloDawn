package home

import (
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { Commands.Add("post/bulletin", teamBulletin) }

func teamRefusal(s *State, c Command, key string) *Refusal {
	if s.Group.Kind != "team" {
		if key == "post/bulletin" {
			return &Refusal{"conflict", "bulletins belong in a team group"}
		}
		return nil
	}
	if strings.HasPrefix(key, "task/") || key == "member/add_ai" {
		return &Refusal{"conflict", "a team group has no tasks or AI members"}
	}
	if strings.HasPrefix(key, "post/") {
		return teamPostRefusal(c, key)
	}
	return nil
}

func teamPostRefusal(c Command, key string) *Refusal {
	thread, _ := c.field("thread")
	as, _ := c.field("as")
	if (key != "post/text" && key != "post/bulletin") || thread != "" || as != "" {
		return &Refusal{"conflict", "a team group accepts person text and bulletin posts on its main line"}
	}
	return nil
}

func teamCreateValid(p proto.GroupCreate) *Refusal {
	if slices.Contains([]string{p.Name, p.MemberName}, "") {
		return &Refusal{"bad_request", "group create requires name and member_name"}
	}
	if p.Repo != "" || p.Base != "" || p.Forge != "none" || len(p.Checks) != 0 {
		return &Refusal{"bad_request", "a team group has no repository, base or checks and uses forge none"}
	}
	return nil
}

func teamSetValid(p proto.GroupSet) *Refusal {
	if p.Checks != nil || p.Pass != nil || p.PassFoundation != nil || p.Limits != nil {
		return &Refusal{"bad_request", "a team group has no project settings"}
	}
	return nil
}

func teamBulletin(s *State, c Command) (Result, *Refusal) {
	var p proto.Post[proto.BulletinSigned]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Re != nil || p.Main {
		return Result{}, &Refusal{"bad_request", "a bulletin has no reply and main is false"}
	}
	rows, _, refusal := postWrite[proto.BulletinSigned](s, c, "bulletin", false, false)
	return Result{Rows: rows}, refusal
}
