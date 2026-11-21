package home

import (
	"crypto/sha256"
	"encoding/base64"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Group creation and settings: group create and group set
// (planning/contracts/commands.md section 1.1; planning/contracts/rows.md
// sections 1 and 2). group pause and resume live in cmd_pause.go.

func init() {
	Commands.Add("group/create", grpCreate)
	Commands.Add("group/set", grpSet)
}

const (
	grpKind           = "project"
	grpPass           = 90
	grpPassFoundation = 70
)

var (
	grpForges = []string{"github", "none"}
	grpLimits = proto.Limits{
		Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3, ReviewRounds: 5,
		AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
	}
)

// grpCreate makes this node the home of a new project group whose id is both g
// and n, writes the group row and the owner's member row, and asks for a
// groups push to connected peers.
func grpCreate(s *State, c Command) (Result, *Refusal) {
	if !s.grpMayCreate(c.Who) {
		return Result{}, &Refusal{"forbidden", "only a device of this node's owner creates a group"}
	}
	var p proto.GroupCreate
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if refusal := grpCreateValid(p); refusal != nil {
		return Result{}, refusal
	}
	if s.Group.ID != "" {
		return Result{}, &Refusal{"conflict", "the group already exists"}
	}
	return grpFounded(s, c, p), nil
}

func (s *State) grpMayCreate(w Who) bool {
	return w.AI == "" && w.User == s.Owner
}

func grpCreateValid(p proto.GroupCreate) *Refusal {
	if p.Group != p.Nonce {
		return &Refusal{"bad_request", "group create names a group other than its nonce"}
	}
	if p.Kind != grpKind || !slices.Contains(grpForges, p.Forge) {
		return &Refusal{"bad_request", "group create has a kind or forge outside step 1"}
	}
	if slices.Contains([]string{p.Name, p.Repo, p.Base, p.MemberName}, "") {
		return &Refusal{"bad_request", "group create requires name, repo, base and member_name"}
	}
	return nil
}

func grpFounded(s *State, c Command, p proto.GroupCreate) Result {
	group := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Kind: p.Kind,
		Repo: p.Repo, Base: p.Base, Forge: p.Forge, Home: s.Home, Checks: p.Checks,
		Pass: grpPass, PassFoundation: grpPassFoundation, Limits: grpLimits, Created: c.Now,
	}
	owner := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: grpOwnerID(c.Nonce)},
			Group:     s.ID, Kind: "human", User: c.Who.User, Name: p.MemberName, LastSeen: c.Now,
		},
		Role: "owner", Online: true,
	}
	return Result{Rows: []Row{RowOf(group), RowOf(owner)}, GroupsChanged: true}
}

// grpOwnerID derives the owner's member id from the nonce of group create, so
// the function stays pure and nobody knows the id before the command arrives.
func grpOwnerID(nonce proto.ID) string {
	h := sha256.Sum256([]byte("SoloDawn group owner " + string(nonce)))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

// grpSet updates the settings of a group. Naming a secretary clears any
// previous secretary's member flag and refuses an AI with an unfinished task.
func grpSet(s *State, c Command) (Result, *Refusal) {
	if !s.Admin(c.Who) {
		return Result{}, &Refusal{"forbidden", "only an admin changes the group"}
	}
	var p proto.GroupSet
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if refusal := grpSetValid(p); refusal != nil {
		return Result{}, refusal
	}
	return s.grpApplySet(p)
}

func grpSetValid(p proto.GroupSet) *Refusal {
	present := [6]bool{p.Name != nil, p.Checks != nil, p.Pass != nil, p.PassFoundation != nil, p.Limits != nil, p.Secretary != ""}
	if present == [6]bool{} {
		return &Refusal{"bad_request", "group set carries no field to change"}
	}
	if p.Name != nil && *p.Name == "" {
		return &Refusal{"bad_request", "group name must not be empty"}
	}
	return nil
}

func (s *State) grpApplySet(p proto.GroupSet) (Result, *Refusal) {
	group := grpUpdated(s.Group, p)
	var extra []Row
	if p.Secretary != "" {
		var refusal *Refusal
		extra, refusal = s.grpSecretary(string(p.Secretary))
		if refusal != nil {
			return Result{}, refusal
		}
		group.Secretary = string(p.Secretary)
	}
	rows := append([]Row{RowOf(group)}, extra...)
	return Result{Rows: rows, GroupsChanged: group.Name != s.Group.Name}, nil
}

func grpUpdated(group proto.GroupRow, p proto.GroupSet) proto.GroupRow {
	grpSetOpt(&group.Name, p.Name)
	grpSetOpt(&group.Checks, p.Checks)
	grpSetOpt(&group.Pass, p.Pass)
	grpSetOpt(&group.PassFoundation, p.PassFoundation)
	grpSetOpt(&group.Limits, p.Limits)
	return group
}

func grpSetOpt[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (s *State) grpSecretary(id string) ([]Row, *Refusal) {
	next, ok := s.AIs[id]
	if !ok || s.memBusy(id) {
		return nil, &Refusal{"conflict", "secretary must name an AI member with no unfinished task"}
	}
	rows := s.memDemote(id)
	if !next.Secretary {
		next.Secretary = true
		rows = append(rows, RowOf(next))
	}
	return rows, nil
}
