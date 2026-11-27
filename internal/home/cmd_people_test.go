package home_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// peopleDisk is a disk of a node that holds these device logs, by user id.
type peopleDisk struct {
	disk
	other bool
	logs  map[string][]proto.Envelope
}

func (d peopleDisk) OtherMember(string) bool                { return d.other }
func (d peopleDisk) DeviceLog(user string) []proto.Envelope { return d.logs[user] }

func peopleApply(t *testing.T, s *home.State, d home.Disk, by identity.Key, join *home.Joining, typ, fields string) home.Result {
	t.Helper()
	env := signed(by, typ, 180, fields)
	if join == nil {
		res, err := home.Commands.Apply(s, d, receipts{}, env, now)
		equal(t, err, nil)
		return res
	}
	return peopleRun(t, s, "join", home.Command{Env: env, Disk: d, Joining: join, Now: clock, Who: peopleJoiner(s, by, join)})
}

// peopleJoiner is who a join on a joining connection is by: the device, and
// its person and their member id when the presented log holds it.
func peopleJoiner(s *home.State, by identity.Key, join *home.Joining) home.Who {
	who := home.Who{Device: by.ID()}
	if join.Log != nil && join.Log.Valid(by.ID()) {
		who.User = join.Log.User
		for id, person := range s.People {
			if person.User == who.User {
				who.Human = id
			}
		}
	}
	return who
}

// peopleRun runs the function of a key with c as the loop fills it in after
// the checks: the header of its payload, and the group's open code read in
// the command's transaction.
func peopleRun(t *testing.T, s *home.State, key string, c home.Command) home.Result {
	t.Helper()
	equal(t, json.Unmarshal([]byte(c.Env.Payload), &c.PayloadHeader), nil)
	res, refusal := home.Commands[key](s, c)
	if refusal != nil {
		return home.Result{Answer: store.Outcome{Code: refusal.Code}}
	}
	return s.Seal(res)
}

func TestPeopleInvite(t *testing.T) {
	s := group(t)
	res := peopleApply(t, s, disk{}, browser, nil, "invite", "")
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.InviteBy, ownerID)
	equal(t, len(res.Rows), 0)

	s.People[ownerID] = person(ownerID, browser, "admin")
	res = peopleApply(t, s, disk{}, browser, nil, "invite", "")
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.InviteBy, ownerID)

	s.People[ownerID] = person(ownerID, browser, "member")
	res = peopleApply(t, s, disk{}, browser, nil, "invite", "")
	equal(t, res.Answer.Code, "forbidden")
	equal(t, res.InviteBy, "")
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
	// A member's join on a connection that is not joining.
	equal(t, peopleApply(t, grpState(t), disk{}, browser, nil, "join", `,"name":"again"`).Answer, store.Outcome{Code: "forbidden"})
}

// people.md 2.5: an admin adds a person whose log the node holds. The rows are
// those of a join, without an invitation: the member under the n of the
// command, a device row per entry of the log, and member_joined by the admin.
func TestPeopleAdd(t *testing.T) {
	log := logOf(t, nobody, device(7))
	d := peopleDisk{logs: map[string][]proto.Envelope{nobody.ID(): log.Entries}}
	fields := fmt.Sprintf(`,"op":"add_person","user":%q,"name":"Mina"`, nobody.ID())
	for _, admin := range []struct {
		by     identity.Key
		member string
	}{{browser, grpOwnerID(groupID)}, {guest, grpAdmin}} {
		s := grpState(t)
		res := peopleApply(t, s, d, admin.by, nil, "member", fields)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, []any{res.GroupsChanged, res.InviteUsed, len(res.Writes), len(res.Rows)}, []any{true, false, 0, 4})
		equal(t, res.Rows[0].Value(), proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id(180), Seq: 11},
				Group: groupID, Kind: "human", User: nobody.ID(), Name: "Mina", LastSeen: clock}, Role: "member", Online: true,
		})
		for i, env := range log.Entries {
			var change proto.DeviceChange
			proto.Decode([]byte(env.Payload), &change)
			equal(t, res.Rows[i+1].Value(), proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", nobody.ID(), i+1), Seq: int64(12 + i)}, DeviceChange: change, Env: env})
		}
		msg := res.Rows[3].Value().(proto.MsgRow)
		equal(t, string(msg.Body), fmt.Sprintf(`{"code":"member_joined","member":%q,"by":%q}`, id(180), admin.member))
		equal(t, []any{msg.Main, msg.By, msg.Thread}, []any{true, "system", (*string)(nil)})
		equal(t, s.Serves(device(7).ID()), false)
		s.Commit(res)
		equal(t, s.Serves(device(7).ID()), true)
		equal(t, peopleApply(t, s, d, admin.by, nil, "member", fields).Answer, store.Outcome{Code: "conflict"})
	}
}

func TestPeopleAddRefusals(t *testing.T) {
	known := peopleDisk{logs: map[string][]proto.Envelope{nobody.ID(): logOf(t, nobody).Entries, device(6).ID(): logOf(t, device(6)).Entries}}
	add := func(user, name string) string {
		return fmt.Sprintf(`,"op":"add_person","user":%q,"name":%q`, user, name)
	}
	for _, tc := range []struct {
		name, fields, code string
		by                 identity.Key
		state              func(*home.State)
	}{
		{name: "by a person who is no admin", by: device(6), fields: add(nobody.ID(), "Mina"), code: "forbidden"},
		{name: "by an AI of the owner", by: laptop, fields: add(nobody.ID(), "Mina") + `,"as":"` + aiID + `"`, code: "forbidden"},
		{name: "without a user", by: browser, fields: `,"op":"add_person","name":"Mina"`, code: "bad_request"},
		{name: "with a name that is no text", by: browser, fields: fmt.Sprintf(`,"op":"add_person","user":%q,"name":4`, nobody.ID()), code: "bad_request"},
		{name: "with an empty name", by: browser, fields: add(nobody.ID(), ""), code: "bad_request"},
		{name: "a person the node holds no log of", by: browser, fields: add(device(7).ID(), "Mina"), code: "not_found"},
		{name: "an unknown person under a taken name is unknown first", by: browser, fields: add(device(7).ID(), "owner"), code: "not_found"},
		{name: "a person who is a member already", by: browser, fields: add(device(6).ID(), "Again"), code: "conflict"},
		{name: "under the name of a person", by: browser, fields: add(nobody.ID(), "owner"), code: "conflict"},
		{name: "under the name of an AI", by: browser, fields: add(nobody.ID(), "Claude"), code: "conflict"},
		{name: "under the id of a member", by: browser, fields: add(nobody.ID(), "Mina"), code: "conflict",
			state: func(s *home.State) { s.AIs[id(180)] = ai(id(180), laptop) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := grpState(t)
			if tc.state != nil {
				tc.state(s)
			}
			res := peopleApply(t, s, known, tc.by, nil, "member", tc.fields)
			equal(t, res.Answer, store.Outcome{Code: tc.code})
			equal(t, []any{len(res.Rows), res.GroupsChanged}, []any{0, false})
		})
	}
}

// A9.46, home half: a person who came into one group of this home by a link
// is added to another without one. The group appears on their connected
// device with the next groups push, a device of theirs that connects later
// lists it, and they read it. A person the node does not know gets no row.
func TestPeopleAddReachesThePersonsDevices(t *testing.T) {
	w := runStart(t, runOpen(t), home.NodeTables())
	listed := func(frame string) (ids []string) {
		var groups proto.Groups
		equal(t, json.Unmarshal([]byte(frame), &groups), nil)
		for _, g := range groups.Groups {
			ids = append(ids, g.Group)
		}
		return ids
	}
	owner := w.attach(browser)
	for _, g := range []string{groupID, runOther} {
		owner.re++
		w.loop.Cmd(owner.conn, owner.re, browser.Sign(fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":"create","name":"G","member_name":"owner"}`, g, clock, g)))
		owner.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, owner.re))
		owner.next()
	}

	phone := w.attach(guest)
	phone.conn.SetJoining(func() *home.Joining {
		return &home.Joining{Group: groupID, By: grpOwnerID(groupID), Open: true, Log: logOf(t, guest)}
	})
	phone.does(groupID, "join", `,"name":"guest"`)
	phone.conn.SetJoining(nil)
	equal(t, listed(phone.next()), []string{groupID})
	equal(t, listed(owner.next()), []string{groupID, runOther})
	phone.syncRefused(runOther, "forbidden")

	owner.fails(runOther, "member", fmt.Sprintf(`,"op":"add_person","user":%q,"name":"Nobody"`, nobody.ID()), "not_found")
	owner.does(runOther, "member", fmt.Sprintf(`,"op":"add_person","user":%q,"name":"Mina"`, guest.ID()))
	equal(t, listed(owner.next()), []string{groupID, runOther})
	equal(t, listed(phone.next()), []string{groupID, runOther})
	phone.watch(runOther, true)

	later := wpushAttachRaw(w, guest)
	w.loop.Welcome(later.conn, runWelcome)
	equal(t, listed(later.next()), []string{groupID, runOther})
	const members = "SELECT count(*) FROM row WHERE g=? AND r='member' AND gone=0 AND data->>'kind'='human'"
	equal(t, runQuery[int](t, w.db, members, runOther), 2)
	equal(t, runQuery[string](t, w.db, "SELECT data->>'name' FROM row WHERE g=? AND r='member' AND data->>'user'=?", runOther, guest.ID()), "Mina")
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
		equal(t, []any{res.GroupsChanged, res.CodeChanged}, []any{true, true})
		// The code ends with a removal by another (people.md 2.4, step 5).
		wantWrites, wantForget := 2, person.User
		if other {
			wantWrites, wantForget = 1, ""
		}
		equal(t, res.Forget, wantForget)
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
	for _, state := range []string{"active", "done"} {
		s.Tasks[id(190)] = proto.TaskRow{State: state, Reviewer: a.ID}
		res := peopleApply(t, s, disk{}, device(6), nil, "member", fmt.Sprintf(`,"op":"remove","id":%q`, grpMember))
		if state == "active" {
			equal(t, res.Answer, store.Outcome{Code: "conflict"})
			continue
		}
		equal(t, res.Answer, store.Outcome{OK: true})
		// A person who leaves leaves the code open.
		equal(t, []any{res.CodeChanged, len(res.Writes)}, []any{false, 1})
		msg := res.Rows[len(res.Rows)-1].Value().(proto.MsgRow)
		equal(t, string(msg.Body), fmt.Sprintf(`{"code":"member_removed","user":%q,"name":"guest-user"}`, device(6).ID()))
	}
}

// A9.47 and A9.48, home half (people.md 2.6): an admin makes the code, ends it
// and shows it. The answer carries the code with hyphens and its end beside
// the receipt, which holds that it succeeded and nothing more.
func TestPeopleCode(t *testing.T) {
	shown := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$`)
	codes := map[string]bool{}
	for _, tc := range []struct {
		by    identity.Key
		hours int64
	}{{browser, 1}, {guest, 24}, {browser, 168}} {
		res := peopleApply(t, grpState(t), disk{}, tc.by, nil, "code", fmt.Sprintf(`,"op":"new","hours":%d`, tc.hours))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, shown.MatchString(res.Code), true)
		equal(t, []any{res.Expires, res.CodeChanged, len(res.Writes), len(res.Rows)}, []any{clock + tc.hours*3_600_000, true, 1, 0})
		equal(t, res.Frame(4), any(proto.CmdResult{Success: proto.Success{Re: 4, OK: true}, Code: res.Code, Expires: res.Expires}))
		codes[res.Code] = true
	}
	equal(t, len(codes), 3)

	off := peopleApply(t, grpState(t), disk{}, guest, nil, "code", `,"op":"off"`)
	equal(t, []any{off.Answer, off.Code, off.Expires, off.CodeChanged, len(off.Writes)}, []any{store.Outcome{OK: true}, "", int64(0), true, 1})

	// show answers the group's open code, which the loop reads in the
	// command's transaction, and no field while none is open.
	open := store.Invite{Secret: "7K2M9QXD4F8TWB3N", By: grpAdmin, Expires: clock + 86_400_000}
	admin := home.Who{Device: guest.ID(), User: guest.ID(), Human: grpAdmin}
	show := func(code store.Invite) home.Result {
		return peopleRun(t, grpState(t), "code/show", home.Command{Env: signed(guest, "code", 181, `,"op":"show"`), Code: code, Now: clock, Who: admin})
	}
	equal(t, show(open), home.Result{Answer: store.Outcome{OK: true}, Code: "7K2M-9QXD-4F8T-WB3N", Expires: open.Expires})
	equal(t, show(store.Invite{}), home.Result{Answer: store.Outcome{OK: true}})
	equal(t, peopleApply(t, grpState(t), disk{}, browser, nil, "code", `,"op":"show"`), home.Result{Answer: store.Outcome{OK: true}})
}

func TestPeopleCodeRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, fields, code string
		by                 identity.Key
	}{
		{"new by a member", `,"op":"new","hours":24`, "forbidden", device(6)},
		{"off by a member", `,"op":"off"`, "forbidden", device(6)},
		{"show by a member", `,"op":"show"`, "forbidden", device(6)},
		{"by an AI of the owner", `,"op":"new","hours":24,"as":"` + aiID + `"`, "forbidden", laptop},
		{"without hours", `,"op":"new"`, "bad_request", browser},
		{"for 0 hours", `,"op":"new","hours":0`, "bad_request", browser},
		{"for 169 hours", `,"op":"new","hours":169`, "bad_request", browser},
		{"for hours that are no integer", `,"op":"new","hours":1.5`, "bad_request", browser},
		{"with an op it does not know", `,"op":"extend","hours":24`, "bad_request", browser},
		{"without an op", ``, "bad_request", browser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := peopleApply(t, grpState(t), disk{}, tc.by, nil, "code", tc.fields)
			equal(t, []any{res.Answer, res.Code, res.CodeChanged, len(res.Writes)}, []any{store.Outcome{Code: tc.code}, "", false, 0})
		})
	}
}

// A9.47, A9.48 and A9.52, home half (people.md 2.2): a joining connection
// that came by a code is open while that code is the group's open code, and
// member_joined names the admin who made it. A code is not used up, a member
// who types it gets ok with no row, and an invitation works beside it.
func TestPeopleJoinByCode(t *testing.T) {
	open := store.Invite{Secret: "7K2M9QXD4F8TWB3N", By: grpAdmin, Expires: clock + 86_400_000}
	log := logOf(t, nobody, device(7))
	byCode := &home.Joining{Group: groupID, Code: "7K2M9QXD4F8TWB3N", Log: log}
	join := func(s *home.State, j *home.Joining, code store.Invite) home.Result {
		return peopleRun(t, s, "join", home.Command{Env: signed(nobody, "join", 181, `,"name":"visitor"`), Disk: disk{}, Joining: j, Code: code, Now: clock, Who: peopleJoiner(s, nobody, j)})
	}
	s := grpState(t)
	res := join(s, byCode, open)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, []any{res.InviteUsed, res.GroupsChanged, res.CodeChanged, len(res.Rows), len(res.Writes)}, []any{false, true, false, 4, 2})
	msg := res.Rows[3].Value().(proto.MsgRow)
	equal(t, string(msg.Body), fmt.Sprintf(`{"code":"member_joined","member":%q,"by":%q}`, id(181), grpAdmin))
	s.Commit(res)
	equal(t, join(s, byCode, open), home.Result{Answer: store.Outcome{OK: true}, GroupsChanged: true})

	other := store.Invite{Secret: "0000000000000000", By: grpAdmin, Expires: open.Expires}
	for name, tc := range map[string]struct {
		join *home.Joining
		code store.Invite
	}{
		"while no code is open":                {byCode, store.Invite{}},
		"by a code that is not the open one":   {byCode, other},
		"by a code, whatever Open and By hold": {&home.Joining{Group: groupID, By: grpAdmin, Open: true, Code: "0000000000000001", Log: log}, open},
	} {
		t.Run(name, func(t *testing.T) {
			res := join(grpState(t), tc.join, tc.code)
			equal(t, []any{res.Answer, len(res.Rows)}, []any{store.Outcome{Code: "forbidden"}, 0})
		})
	}

	invited := join(grpState(t), &home.Joining{Group: groupID, By: grpOwnerID(groupID), Open: true, Log: log}, open)
	equal(t, []any{invited.Answer, invited.InviteUsed}, []any{store.Outcome{OK: true}, true})
	equal(t, string(invited.Rows[3].Value().(proto.MsgRow).Body), fmt.Sprintf(`{"code":"member_joined","member":%q,"by":%q}`, id(181), grpOwnerID(groupID)))
}
