package home

import (
	"encoding/json"
	"regexp"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// AI members in a group: member add_ai, member set and member remove
// (planning/contracts/commands.md section 1.2; planning/contracts/rows.md
// section 2). member status lives in cmd_hostev.go.

func init() {
	Commands.Add("member/add_ai", memAddAI)
	Commands.Add("member/set", memSet)
	Commands.Add("member/remove", memRemove)
}

var (
	memCLI      = regexp.MustCompile(`^[a-z0-9_]+$`)
	memAccounts = []string{"personal", "team", "api_key"}
)

// memAddAI adds an AI member hosted by the signing node. When secretary is
// set, it updates the group row and clears any previous secretary's flag.
func memAddAI(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI != "" {
		return Result{}, &Refusal{"forbidden", "only a host node of a member adds an AI"}
	}
	var p proto.MemberAddAI
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if refusal := memAddValid(p); refusal != nil {
		return Result{}, refusal
	}
	hostName := memDevice(s.Logs[c.Who.User], c.Who.Device)
	return s.memCreateAI(c, p, hostName)
}

// memDevice reads the label from the latest add entry for device in log. The
// caller's device is valid in log, so at least one add entry exists and no
// revoke entry for it is present.
func memDevice(log *identity.Log, device string) (name string) {
	for _, env := range log.Entries {
		var e proto.DeviceChange
		proto.Decode([]byte(env.Payload), &e)
		if e.Device == device {
			name = e.Name
		}
	}
	return name
}

func memAddValid(p proto.MemberAddAI) *Refusal {
	if p.Name == "" || !memCLI.MatchString(p.CLI) || !slices.Contains(memAccounts, p.Account) {
		return &Refusal{"bad_request", "member add_ai requires a name, a supported cli and an account type"}
	}
	return nil
}

func (s *State) memCreateAI(c Command, p proto.MemberAddAI, hostName string) (Result, *Refusal) {
	if !s.memMaySecretary(c.Who, p.Secretary) {
		return Result{}, &Refusal{"forbidden", "only an admin replaces the group's secretary"}
	}
	id := string(p.ID)
	if s.memExists(id) || s.memNameTaken(p.Name, "") {
		return Result{}, &Refusal{"conflict", "the member id or name is already taken in the group"}
	}
	ai := memNewAI(s, c, p, hostName)
	return Result{Rows: s.memJoinedRows(ai)}, nil
}

func (s *State) memMaySecretary(w Who, secretary bool) bool {
	return !secretary || s.Group.Secretary == "" || s.Admin(w)
}

func memNewAI(s *State, c Command, p proto.MemberAddAI, hostName string) proto.AIMemberRow {
	path := "direct"
	if c.Who.Device == s.Home {
		path = "local"
	}
	return proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: string(p.ID)},
			Group:     s.ID, Kind: "ai", User: c.Who.User, Name: p.Name, LastSeen: c.Now,
		},
		CLI: p.CLI, Host: c.Who.Device, HostName: hostName, Secretary: p.Secretary,
		Account: p.Account, Allow: []string{}, Approvers: memApprovers(c.Who.User, p.Approvers),
		State: "dormant", Since: c.Now, Path: path, Usage: proto.Usage{},
	}
}

func memApprovers(owner string, list []string) []string {
	if slices.Contains(list, owner) {
		return list
	}
	return append([]string{owner}, list...)
}

func (s *State) memJoinedRows(ai proto.AIMemberRow) []Row {
	rows := []Row{RowOf(ai)}
	if !ai.Secretary {
		return rows
	}
	group := s.Group
	group.Secretary = ai.ID
	rows = append(rows, s.memDemote(ai.ID)...)
	return append(rows, RowOf(group))
}

// memDemote clears the secretary flag of the previous secretary, unless keep
// is already that member.
func (s *State) memDemote(keep string) []Row {
	old, ok := s.AIs[s.Group.Secretary]
	if !ok || old.ID == keep {
		return nil
	}
	old.Secretary = false
	return []Row{RowOf(old)}
}

// memSet updates an AI member's name, account, allow list or approvers.
func memSet(s *State, c Command) (Result, *Refusal) {
	var p proto.MemberSet
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if refusal := memSetValid(p); refusal != nil {
		return Result{}, refusal
	}
	ai, refusal := s.memAI(string(p.ID))
	if refusal != nil {
		return Result{}, refusal
	}
	if !s.memMaySet(c.Who, ai, p) {
		return Result{}, &Refusal{"forbidden", "only the AI's owner, or its host for approvers alone, changes the member"}
	}
	return s.memFinishSet(ai, p)
}

func memSetValid(p proto.MemberSet) *Refusal {
	present := [4]bool{p.Name != nil, p.Account != nil, p.Allow != nil, p.Approvers != nil}
	if present == [4]bool{} || !memSetFieldsOK(p) {
		return &Refusal{"bad_request", "member set carries no field, an empty name, or an unknown account"}
	}
	return nil
}

func memSetFieldsOK(p proto.MemberSet) bool {
	if p.Name != nil && *p.Name == "" {
		return false
	}
	return p.Account == nil || slices.Contains(memAccounts, *p.Account)
}

func (s *State) memMaySet(w Who, ai proto.AIMemberRow, p proto.MemberSet) bool {
	if w.AI == "" && w.User == ai.User {
		return true
	}
	return s.Hosts(w, ai.ID) && memApproversOnly(p)
}

func memApproversOnly(p proto.MemberSet) bool {
	return p.Name == nil && p.Account == nil && p.Allow == nil
}

func (s *State) memFinishSet(ai proto.AIMemberRow, p proto.MemberSet) (Result, *Refusal) {
	if p.Name != nil && s.memNameTaken(*p.Name, ai.ID) {
		return Result{}, &Refusal{"conflict", "another member in the group already has that name"}
	}
	return Result{Rows: []Row{RowOf(memUpdated(ai, p))}}, nil
}

func memUpdated(ai proto.AIMemberRow, p proto.MemberSet) proto.AIMemberRow {
	grpSetOpt(&ai.Name, p.Name)
	grpSetOpt(&ai.Account, p.Account)
	grpSetOpt(&ai.Allow, p.Allow)
	if p.Approvers != nil {
		ai.Approvers = memApprovers(ai.User, *p.Approvers)
	}
	return ai
}

// memRemove replaces an AI member row with a gone marker, retiring its queued
// items. Removing the secretary clears group.secretary and writes no_secretary.
func memRemove(s *State, c Command) (Result, *Refusal) {
	var p proto.MemberCommand
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	ai, refusal := s.memAI(string(p.ID))
	if refusal != nil {
		return Result{}, refusal
	}
	if !s.memMayRemove(c.Who, ai) {
		return Result{}, &Refusal{"forbidden", "only the AI's owner or an admin removes an AI"}
	}
	if s.memBusy(ai.ID) {
		return Result{}, &Refusal{"conflict", "the AI has an unfinished task"}
	}
	return s.memRemoved(c, ai.ID), nil
}

func (s *State) memMayRemove(w Who, ai proto.AIMemberRow) bool {
	return w.AI == "" && w.User == ai.User || s.Admin(w)
}

func (s *State) memRemoved(c Command, id string) Result {
	rows := []Row{RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Gone: true})}
	if s.Group.Secretary == id {
		group := s.Group
		group.Secretary = ""
		msg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, By: "system",
			Kind: "system", Timestamp: c.Now, Mentions: []string{}, Body: json.RawMessage(`{"code":"no_secretary"}`),
		}
		rows = append(rows, RowOf(group), RowOf(msg))
	}
	return Result{Rows: rows, Retire: memRetired(c.Disk, id)}
}

func memRetired(d Disk, ai string) []int64 {
	var retire []int64
	for _, it := range d.Queued() {
		if it.AI == ai {
			retire = append(retire, it.ID)
		}
	}
	return retire
}

func (s *State) memAI(id string) (proto.AIMemberRow, *Refusal) {
	if ai, ok := s.AIs[id]; ok {
		return ai, nil
	}
	if _, human := s.People[id]; human {
		return proto.AIMemberRow{}, &Refusal{"forbidden", "only AI members are changed or removed in step 1"}
	}
	return proto.AIMemberRow{}, &Refusal{"not_found", "the group has no such member"}
}

// memBusy reports whether an AI is assignee or reviewer of a task that is not final.
func (s *State) memBusy(id string) bool {
	for _, t := range s.Tasks {
		if slices.Contains(statesOpen, t.State) && (t.Assignee == id || t.Reviewer == id) {
			return true
		}
	}
	return false
}

func (s *State) memExists(id string) bool {
	_, human := s.People[id]
	_, ai := s.AIs[id]
	return human || ai
}

func (s *State) memNameTaken(name, skip string) bool {
	for id, m := range s.AIs {
		if id != skip && m.Name == name {
			return true
		}
	}
	return memHumanNamed(s.People, name)
}

func memHumanNamed(people map[string]proto.HumanMemberRow, name string) bool {
	for _, m := range people {
		if m.Name == name {
			return true
		}
	}
	return false
}
