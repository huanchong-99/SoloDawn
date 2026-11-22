package home_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type peopleDisk struct {
	disk
	other bool
}

func (d peopleDisk) OtherMember(string) bool { return d.other }

func peopleApply(t *testing.T, s *home.State, d home.Disk, by identity.Key, join *home.Joining, typ, fields string) home.Result {
	t.Helper()
	env := signed(by, typ, 180, fields)
	if join == nil {
		res, err := home.Commands.Apply(s, d, receipts{}, env, now)
		equal(t, err, nil)
		return res
	}
	c := home.Command{Env: env, Disk: d, Joining: join, Now: clock, Who: home.Who{Device: by.ID()}}
	equal(t, json.Unmarshal([]byte(env.Payload), &c.PayloadHeader), nil)
	if join.Log != nil && join.Log.Valid(by.ID()) {
		c.Who.User = join.Log.User
		for id, person := range s.People {
			if person.User == c.Who.User {
				c.Who.Human = id
			}
		}
	}
	res, refusal := home.Commands["join"](s, c)
	if refusal != nil {
		return home.Result{Answer: store.Outcome{Code: refusal.Code}}
	}
	return s.Seal(res)
}

func TestPeopleJoin(t *testing.T) {
	log := logOf(t, nobody, device(7))
	join := &home.Joining{Group: groupID, By: ownerID, Open: true, Log: log}
	s := group(t)
	res := peopleApply(t, s, disk{}, nobody, join, "join", `,"name":"visitor"`)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.InviteUsed, true)
	equal(t, res.GroupsChanged, true)
	equal(t, len(res.Rows), 4)
	equal(t, len(res.Writes), 2)
	equal(t, res.Rows[0].Value(), proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id(180), Seq: 11},
			Group: groupID, Kind: "human", User: nobody.ID(), Name: "visitor", LastSeen: clock}, Role: "member", Online: true,
	})
	for i, env := range log.Entries {
		var change proto.DeviceChange
		proto.Decode([]byte(env.Payload), &change)
		equal(t, res.Rows[i+1].Value(), proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", nobody.ID(), i+1), Seq: int64(12 + i)}, DeviceChange: change, Env: env})
	}
	msg := res.Rows[3].Value().(proto.MsgRow)
	equal(t, string(msg.Body), fmt.Sprintf(`{"code":"member_joined","member":%q,"by":%q}`, id(180), ownerID))
	equal(t, msg.Main, true)
	s.Commit(res)
	equal(t, s.Serves(device(7).ID()), true)
	restored := peopleApply(t, s, disk{}, nobody, join, "join", `,"name":"owner"`)
	equal(t, restored, home.Result{Answer: store.Outcome{OK: true}, GroupsChanged: true})
}

func TestPeopleJoinRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, fields, code string
		join               *home.Joining
	}{
		{"ordinary", `,"name":"visitor"`, "forbidden", nil},
		{"closed", `,"name":"visitor"`, "forbidden", &home.Joining{Group: groupID, Log: logOf(t, guest)}},
		{"no log", `,"name":"visitor"`, "forbidden", &home.Joining{Group: groupID, Open: true}},
		{"wrong log", `,"name":"visitor"`, "forbidden", &home.Joining{Group: groupID, Open: true, Log: logOf(t, nobody)}},
		{"bad name", `,"name":4`, "bad_request", &home.Joining{Group: groupID, Open: true, Log: logOf(t, guest)}},
		{"empty name", `,"name":""`, "bad_request", &home.Joining{Group: groupID, Open: true, Log: logOf(t, guest)}},
		{"name taken", `,"name":"owner"`, "conflict", &home.Joining{Group: groupID, Open: true, Log: logOf(t, guest)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := grpState(t)
			delete(s.People, grpAdmin)
			res := peopleApply(t, s, disk{}, guest, tc.join, "join", tc.fields)
			equal(t, res.Answer, store.Outcome{Code: tc.code})
			equal(t, len(res.Rows), 0)
			equal(t, res.InviteUsed, false)
		})
	}
	s := grpState(t)
	s.AIs[id(180)] = ai(id(180), laptop)
	res := peopleApply(t, s, disk{}, nobody, &home.Joining{Group: groupID, Open: true, Log: logOf(t, nobody)}, "join", `,"name":"visitor"`)
	equal(t, res.Answer, store.Outcome{Code: "conflict"})
}

func TestPeopleRole(t *testing.T) {
	for _, role := range []string{"admin", "member"} {
		s := grpState(t)
		res := peopleApply(t, s, disk{}, browser, nil, "member", fmt.Sprintf(`,"op":"set","id":%q,"role":%q,"name":"ignored","account":"ignored"`, grpAdmin, role))
		equal(t, res.Answer, store.Outcome{OK: true})
		expected := s.People[grpAdmin]
		expected.Role, expected.Seq = role, 11
		equal(t, res.Rows[0].Value(), expected)
		equal(t, len(res.Rows), 1)
	}
	res := peopleApply(t, grpState(t), disk{}, browser, nil, "member", fmt.Sprintf(`,"op":"set","id":%q,"role":"admin","name":4,"approvers":false`, grpAdmin))
	equal(t, res.Answer, store.Outcome{OK: true})
	res = peopleApply(t, grpState(t), disk{}, browser, nil, "member", fmt.Sprintf(`,"op":"set","id":%q,"role":4`, grpAdmin))
	equal(t, res.Answer, store.Outcome{Code: "bad_request"})
	res = peopleApply(t, grpState(t), disk{}, browser, nil, "member", `,"op":"set","id":"invalid","role":"admin"`)
	equal(t, res.Answer, store.Outcome{Code: "bad_request"})
	for _, tc := range []struct {
		by                   identity.Key
		target, fields, code string
	}{
		{guest, grpMember, `,"role":"admin"`, "forbidden"},
		{browser, grpOwnerID(groupID), `,"role":"member"`, "forbidden"},
		{browser, grpAdmin, "", "bad_request"},
		{browser, grpAdmin, `,"role":"owner"`, "bad_request"},
		{browser, grpAdmin, `,"role":"admin","as":"` + aiID + `"`, "forbidden"},
	} {
		res := peopleApply(t, grpState(t), disk{}, tc.by, nil, "member", fmt.Sprintf(`,"op":"set","id":%q`, tc.target)+tc.fields)
		equal(t, res.Answer, store.Outcome{Code: tc.code})
	}
}

func TestPeopleRemovalRoles(t *testing.T) {
	for _, role := range []string{"member", "admin", "owner"} {
		for _, by := range []identity.Key{browser, guest, device(6)} {
			s := grpState(t)
			person := s.People[grpMember]
			person.Role = role
			s.People[grpMember] = person
			res := peopleApply(t, s, disk{}, by, nil, "member", fmt.Sprintf(`,"op":"remove","id":%q`, grpMember))
			want := store.Outcome{OK: true}
			if role == "owner" || role == "admin" && by.ID() == guest.ID() {
				want = store.Outcome{Code: "forbidden"}
			}
			equal(t, res.Answer, want)
		}
	}
}

func TestPeopleRemovalCascades(t *testing.T) {
	for _, other := range []bool{false, true} {
		s := grpState(t)
		person := s.People[grpMember]
		sec := s.AIs[aiID]
		sec.User = person.User
		s.AIs[aiID] = sec
		d := peopleDisk{other: other}
		res := peopleApply(t, s, d, browser, nil, "member", fmt.Sprintf(`,"op":"remove","id":%q`, person.ID))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.GroupsChanged, true)
		wantWrites := 1
		if other {
			wantWrites = 0
		}
		equal(t, len(res.Rows), 6)
		equal(t, len(res.Writes), wantWrites)
		equal(t, res.Rows[4].Value(), proto.GoneRow{RowHeader: proto.RowHeader{Type: "device", ID: person.User + ":1", Seq: 15}, Gone: true})
		msg := res.Rows[len(res.Rows)-1].Value().(proto.MsgRow)
		equal(t, string(msg.Body), fmt.Sprintf(`{"code":"member_removed","user":%q,"name":"guest-user","by":%q}`, person.User, grpOwnerID(groupID)))
		noSec := res.Rows[2].Value().(proto.MsgRow)
		equal(t, string(noSec.Body), `{"code":"no_secretary"}`)
		if noSec.ID == msg.ID {
			t.Fatal("system messages share an id")
		}
		s.Commit(res)
		equal(t, s.Logs[person.User], (*identity.Log)(nil))
		equal(t, s.Group.Secretary, "")
		equal(t, s.Serves(device(6).ID()), false)
		equal(t, len(s.People), 2)
		equal(t, len(s.AIs), 2)
	}
}

func TestPeopleRemovalBusyAndLeaving(t *testing.T) {
	s := grpState(t)
	a := s.AIs[grpWorkerA]
	a.User = device(6).ID()
	s.AIs[grpWorkerA] = a
	for _, state := range []string{"active", "merged"} {
		s.Tasks[id(190)] = proto.TaskRow{State: state, Reviewer: a.ID}
		res := peopleApply(t, s, disk{}, device(6), nil, "member", fmt.Sprintf(`,"op":"remove","id":%q`, grpMember))
		if state == "active" {
			equal(t, res.Answer, store.Outcome{Code: "conflict"})
			continue
		}
		equal(t, res.Answer, store.Outcome{OK: true})
		msg := res.Rows[len(res.Rows)-1].Value().(proto.MsgRow)
		equal(t, string(msg.Body), fmt.Sprintf(`{"code":"member_removed","user":%q,"name":"guest-user"}`, device(6).ID()))
	}
}
