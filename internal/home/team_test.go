package home_test

import (
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func teamPayload() proto.GroupCreate {
	p := grpCreatePayload(groupID, groupID)
	p.Kind, p.Repo, p.Base, p.Forge, p.Checks = "team", "", "", "none", []string{}
	return p
}

func teamState(t *testing.T) *home.State {
	t.Helper()
	s := empty(t)
	res := grpApply(t, s, disk{}, receipts{}, browser, teamPayload())
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, briefRows(res)[2:], grpOwnerDevices(t, 3))
	s.Commit(res)
	return s
}

func TestTeamCreateAndSettings(t *testing.T) {
	s := teamState(t)
	equal(t, s.Group.Kind, "team")
	equal(t, s.Group.Repo, "")
	equal(t, s.Group.Base, "")
	equal(t, s.Group.Forge, "none")
	equal(t, s.Group.Checks, []string{})
	equal(t, s.Group.Secretary, "")
	equal(t, len(s.People), 1)
	equal(t, len(s.AIs), 0)
	p := grpSetCmd(181)
	name := "Team"
	p.Name = &name
	res := grpApply(t, s, disk{}, receipts{}, browser, p)
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	equal(t, s.Group.Name, name)
	for _, field := range []string{`"checks":[]`, `"pass":90`, `"pass_foundation":70`, `"limits":{}`} {
		res = peopleApply(t, s, disk{}, browser, nil, "group", `,"op":"set",`+field)
		equal(t, res.Answer.Code, "bad_request")
	}
}

func TestTeamCreateRefusals(t *testing.T) {
	for _, change := range []func(*proto.GroupCreate){
		func(p *proto.GroupCreate) { p.Name = "" },
		func(p *proto.GroupCreate) { p.MemberName = "" },
		func(p *proto.GroupCreate) { p.Repo = "repo" },
		func(p *proto.GroupCreate) { p.Base = "trunk" },
		func(p *proto.GroupCreate) { p.Forge = "github" },
		func(p *proto.GroupCreate) { p.Checks = []string{"check"} },
	} {
		p := teamPayload()
		change(&p)
		res := grpApply(t, empty(t), disk{}, receipts{}, browser, p)
		equal(t, res.Answer.Code, "bad_request")
		equal(t, len(res.Rows), 0)
	}
}

func TestTeamCommandRefusals(t *testing.T) {
	s := teamState(t)
	for _, tc := range []struct{ typ, fields string }{
		{"task", `,"op":"start"`}, {"task", `,"op":"unknown"`},
		{"member", `,"op":"add_ai"`}, {"post", `,"kind":"brief"`},
		{"post", `,"kind":"confirm"`}, {"post", `,"kind":"unknown"`},
		{"post", fmt.Sprintf(`,"kind":"text","thread":%q`, id(2))},
		{"post", fmt.Sprintf(`,"kind":"text","as":%q`, aiID)},
	} {
		res := peopleApply(t, s, disk{}, browser, nil, tc.typ, tc.fields)
		equal(t, res.Answer.Code, "conflict")
		equal(t, len(res.Items), 0)
	}
}

func TestTeamBulletinAndText(t *testing.T) {
	s := teamState(t)
	owner := grpOwnerID(groupID)
	fields := fmt.Sprintf(`,"kind":"bulletin","thread":null,"re":null,"main":false,"text":"Ready","mentions":[%q],"body":{"from":%q,"group":"Project","ai":"Secretary"}`, owner, groupID)
	res := peopleApply(t, s, disk{}, browser, nil, "post", fields)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Rows), 1)
	equal(t, len(res.Items), 0)
	msg := res.Rows[0].Value().(proto.MsgRow)
	equal(t, msg.By, owner)
	equal(t, msg.Kind, "bulletin")
	equal(t, msg.Thread, (*string)(nil))
	equal(t, msg.Env.Payload, signed(browser, "post", 180, fields).Payload)
	equal(t, string(msg.Body), fmt.Sprintf(`{"from":%q,"group":"Project","ai":"Secretary"}`, groupID))
	equal(t, msg.Mentions, []string{owner})
	res = peopleApply(t, s, disk{}, browser, nil, "post", `,"kind":"text","thread":null,"re":null,"main":false,"text":"","mentions":[],"body":{}`)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Items), 0)
	res = peopleApply(t, group(t), disk{}, browser, nil, "post", fields)
	equal(t, res.Answer.Code, "conflict")
	res = peopleApply(t, empty(t), disk{}, browser, nil, "post", fields)
	equal(t, res.Answer.Code, "not_found")
}

func TestTeamBulletinRefusals(t *testing.T) {
	for _, fields := range []string{
		`"main":true,"re":null,"body":{"from":"` + groupID + `","group":"Project","ai":"Secretary"}`,
		`"main":false,"re":"` + id(3) + `","body":{"from":"` + groupID + `","group":"Project","ai":"Secretary"}`,
		`"main":false,"re":null,"body":{"from":"invalid","group":"Project","ai":"Secretary"}`,
		`"main":false,"re":null,"body":{}`,
		`"main":false,"re":null,"mentions":["` + id(3) + `"],"body":{"from":"` + groupID + `","group":"Project","ai":"Secretary"}`,
	} {
		res := peopleApply(t, teamState(t), disk{}, browser, nil, "post", `,"kind":"bulletin","thread":null,"text":"Ready","mentions":[],`+fields)
		equal(t, res.Answer.OK, false)
		equal(t, len(res.Rows), 0)
	}
}
