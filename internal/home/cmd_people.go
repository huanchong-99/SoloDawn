package home

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	Commands.Add(keyInvite, peopleInvite)
	Commands.Add(keyJoin, peopleJoin)
}

func peopleInvite(s *State, c Command) (Result, *Refusal) {
	if !s.Admin(c.Who) {
		return Result{}, &Refusal{"forbidden", "only an admin can invite"}
	}
	return Result{InviteBy: c.Who.Human}, nil
}

func peopleJoin(s *State, c Command) (Result, *Refusal) {
	if c.Joining == nil || !c.Joining.Open || c.Who.User == "" {
		return Result{}, &Refusal{"forbidden", "join requires an open invitation and a device log"}
	}
	var p proto.Join
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if c.Who.Human != "" {
		return Result{GroupsChanged: true}, nil
	}
	return peopleNewJoin(s, c, p.Name)
}

func peopleNewJoin(s *State, c Command, name string) (Result, *Refusal) {
	if name == "" {
		return Result{}, &Refusal{"bad_request", "join requires a name"}
	}
	if s.memExists(string(c.Nonce)) || s.memNameTaken(name, "") {
		return Result{}, &Refusal{"conflict", "the member id or name is already taken"}
	}
	return peopleJoined(s, c, name), nil
}

func peopleJoined(s *State, c Command, name string) Result {
	person := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: string(c.Nonce)},
			Group: s.ID, Kind: "human", User: c.Who.User, Name: name, LastSeen: c.Now},
		Role: "member", Online: true,
	}
	res := Result{Rows: []Row{RowOf(person)}, GroupsChanged: true, InviteUsed: true}
	for _, env := range c.Joining.Log.Entries {
		row := peopleDevice(env)
		res.Rows = append(res.Rows, RowOf(row))
		res.Writes = append(res.Writes, peopleLog{row})
	}
	body := proto.MemberJoinedBody{SystemCode: proto.SystemCode{Code: "member_joined"}, Member: person.ID, By: c.Joining.By}
	res.Rows = append(res.Rows, peopleMessage(s, c, body))
	return res
}

func peopleDevice(env proto.Envelope) proto.DeviceRow {
	var change proto.DeviceChange
	proto.Decode([]byte(env.Payload), &change)
	return proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", change.User, change.Version)}, DeviceChange: change, Env: env}
}

func peopleMessage(s *State, c Command, body any) Row {
	return RowOf(proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)},
		Group: s.ID, Main: true, By: "system", Kind: "system", Timestamp: c.Now,
		Mentions: []string{}, Body: briefJSON(body)})
}

type peopleLog struct{ row proto.DeviceRow }

func (w peopleLog) Put(tx *store.Tx) error {
	_, err := tx.Exec("INSERT OR IGNORE INTO device_log(user, ver, env) VALUES(?, ?, ?)", w.row.User, w.row.Version, string(briefJSON(w.row.Env)))
	return err
}

func peopleSetRole(s *State, c Command, person proto.HumanMemberRow) (Result, *Refusal) {
	var p struct {
		proto.MemberCommand
		Role *string `json:"role,omitempty"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	return peopleSet(s, c, person, proto.MemberSet{Role: p.Role})
}

func peopleSet(s *State, c Command, person proto.HumanMemberRow, p proto.MemberSet) (Result, *Refusal) {
	if c.Who.AI != "" || c.Who.User != s.Owner || person.Role == "owner" {
		return Result{}, &Refusal{"forbidden", "only the owner changes another person's role"}
	}
	if p.Role == nil || !slices.Contains([]string{"admin", "member"}, *p.Role) {
		return Result{}, &Refusal{"bad_request", "role must be admin or member"}
	}
	person.Role = *p.Role
	return Result{Rows: []Row{RowOf(person)}}, nil
}

func peopleRemove(s *State, c Command, person proto.HumanMemberRow) (Result, *Refusal) {
	if !peopleMayRemove(s, c.Who, person) {
		return Result{}, &Refusal{"forbidden", "the person's role forbids this removal"}
	}
	for _, ai := range s.AIs {
		if ai.User == person.User && s.memBusy(ai.ID) {
			return Result{}, &Refusal{"conflict", "the person has an AI with an unfinished task"}
		}
	}
	return peopleRemoved(s, c, person), nil
}

func peopleMayRemove(s *State, w Who, person proto.HumanMemberRow) bool {
	if w.AI != "" || person.Role == "owner" {
		return false
	}
	if w.User == person.User || w.User == s.Owner {
		return true
	}
	return person.Role == "member" && s.Admin(w)
}

func peopleRemoved(s *State, c Command, person proto.HumanMemberRow) Result {
	res := peopleRemovedAIs(s, c, person.User)
	res.GroupsChanged = true
	res.Rows = append(res.Rows, RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: person.ID}, Gone: true}))
	res.Rows = append(res.Rows, peopleGoneDevices(s, person.User)...)
	if !c.Disk.OtherMember(person.User) {
		res.Writes, res.Forget = append(res.Writes, peopleForget{person.User}), person.User
	}
	body := proto.MemberRemovedBody{SystemCode: proto.SystemCode{Code: "member_removed"}, User: person.User, Name: person.Name}
	if c.Who.User != person.User {
		body.By = &c.Who.Human
	}
	res.Rows = append(res.Rows, peopleMessage(s, c, body))
	return res
}

func peopleRemovedAIs(s *State, c Command, user string) (res Result) {
	hash := sha256.Sum256([]byte("SoloDawn people secretary " + c.Nonce))
	c.Nonce = proto.ID(base64.RawURLEncoding.EncodeToString(hash[:16]))
	for _, id := range slices.Sorted(maps.Keys(s.AIs)) {
		if s.AIs[id].User != user {
			continue
		}
		removed := s.memRemoved(c, id)
		res.Rows = append(res.Rows, removed.Rows...)
		res.Retire = append(res.Retire, removed.Retire...)
	}
	return res
}

func peopleGoneDevices(s *State, user string) (rows []Row) {
	for _, env := range s.Logs[user].Entries {
		row := peopleDevice(env)
		rows = append(rows, RowOf(proto.GoneRow{RowHeader: row.RowHeader, Gone: true}))
	}
	return rows
}

type peopleForget struct{ user string }

func (w peopleForget) Put(tx *store.Tx) error {
	return runSteps(
		func() error { _, err := tx.Exec("DELETE FROM device_log WHERE user=?", w.user); return err },
		func() error { _, err := tx.Exec("DELETE FROM push_sub WHERE user=?", w.user); return err },
	)
}
