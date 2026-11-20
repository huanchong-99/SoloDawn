package home_test

import (
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// who is the principal that the table finds behind a command without as.
func who(t *testing.T, s *home.State, by identity.Key, more string) home.Who {
	t.Helper()
	var got home.Who
	table := home.Table{"note": func(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
		got = c.Who
		return home.Result{}, nil
	}}
	res := apply(t, table, s, receipts{}, by, signed(by, "note", 1, more))
	equal(t, res.Answer, store.Outcome{OK: true})
	return got
}

func TestWhoIsAPersonAnAIOrAHost(t *testing.T) {
	s := group(t)
	s.People[id(12)] = person(id(12), guest, "member")
	for name, c := range map[string]struct {
		by   identity.Key
		more string
		who  home.Who
	}{
		"a person":                       {browser, ``, home.Who{Device: browser.ID(), User: browser.ID(), Human: ownerID}},
		"a person who names the home":    {browser, `,"to":"` + node.ID() + `"`, home.Who{Device: browser.ID(), User: browser.ID(), Human: ownerID}},
		"an AI through its host":         {laptop, `,"as":"` + aiID + `"`, home.Who{Device: laptop.ID(), User: browser.ID(), Human: ownerID, AI: aiID}},
		"a host speaking for itself":     {laptop, ``, home.Who{Device: laptop.ID(), User: browser.ID(), Human: ownerID}},
		"a member who is not this owner": {guest, ``, home.Who{Device: guest.ID(), User: guest.ID(), Human: id(12)}},
	} {
		t.Run(name, func(t *testing.T) { equal(t, who(t, s, c.by, c.more), c.who) })
	}
}

func TestACommandCarriesItsEnvelopeAndTheClock(t *testing.T) {
	var got home.Command
	table := home.Table{"note": func(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
		got = c
		return home.Result{}, nil
	}}
	env := signed(laptop, "note", 1, `,"to":"`+node.ID()+`","as":"`+aiID+`"`)
	apply(t, table, group(t), receipts{}, laptop, env)
	equal(t, got, home.Command{
		PayloadHeader: proto.PayloadHeader{Type: "note", Nonce: proto.ID(id(1)), Timestamp: clock, Group: proto.ID(groupID), To: node.ID()},
		Env:           env,
		Who:           home.Who{Device: laptop.ID(), User: browser.ID(), Human: ownerID, AI: aiID},
		Now:           clock,
	})
	var p struct {
		As string `json:"as"`
	}
	equal(t, got.Decode(&p), (*home.Refusal)(nil))
	equal(t, p.As, aiID)
	var wrong struct {
		As int `json:"as"`
	}
	equal(t, got.Decode(&wrong).Code, "bad_request")
}

func TestTheRolesOfAPrincipal(t *testing.T) {
	s := group(t)
	s.People[id(12)] = person(id(12), guest, "admin")
	s.People[id(14)] = person(id(14), device(6), "member")
	s.AIs[id(13)] = ai(id(13), node)
	owner := home.Who{Device: browser.ID(), User: browser.ID(), Human: ownerID}
	admin := home.Who{Device: guest.ID(), User: guest.ID(), Human: id(12)}
	member := home.Who{Device: device(6).ID(), User: device(6).ID(), Human: id(14)}
	host := home.Who{Device: laptop.ID(), User: browser.ID(), Human: ownerID}
	secretary := home.Who{Device: laptop.ID(), User: browser.ID(), Human: ownerID, AI: aiID}
	worker := home.Who{Device: node.ID(), User: browser.ID(), Human: ownerID, AI: id(13)}

	for name, c := range map[string]struct {
		who                        home.Who
		member                     string
		admin, secretary, itsHost  bool // itsHost: the host of the first AI
		hostOfSecond, hostOfNobody bool
	}{
		"the owner":                  {owner, ownerID, true, false, false, false, false},
		"an admin":                   {admin, id(12), true, false, false, false, false},
		"a member":                   {member, id(14), false, false, false, false, false},
		"a host for itself":          {host, ownerID, true, false, true, false, false},
		"the secretary":              {secretary, aiID, false, true, false, false, false},
		"an AI that is no secretary": {worker, id(13), false, false, false, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			equal(t, c.who.Member(), c.member)
			equal(t, s.Admin(c.who), c.admin)
			equal(t, s.Secretary(c.who), c.secretary)
			equal(t, s.Hosts(c.who, aiID), c.itsHost)
			equal(t, s.Hosts(c.who, id(13)), c.hostOfSecond)
			equal(t, s.Hosts(c.who, id(99)), c.hostOfNobody)
		})
	}

	// The node hosts the second AI when it speaks for itself.
	equal(t, s.Hosts(home.Who{Device: node.ID(), User: browser.ID(), Human: ownerID}, id(13)), true)
	// A group without a secretary has none, whoever asks.
	s.Group.Secretary = ""
	equal(t, s.Secretary(owner), false)
	equal(t, s.Secretary(secretary), false)
}

func TestServesTheOwnerAndTheMembers(t *testing.T) {
	s := empty(t)
	// Before the group has a member, its home surface is the owner's.
	equal(t, []bool{s.Serves(browser.ID()), s.Serves(node.ID()), s.Serves(laptop.ID())}, []bool{true, true, true})
	equal(t, []bool{s.Serves(guest.ID()), s.Serves(nobody.ID())}, []bool{false, false})

	s = group(t)
	equal(t, []bool{s.Serves(browser.ID()), s.Serves(laptop.ID()), s.Serves(guest.ID()), s.Serves(nobody.ID())}, []bool{true, true, false, false})
	s.People[id(12)] = person(id(12), guest, "member")
	equal(t, s.Serves(guest.ID()), true)

	// A device that its person revoked is nobody's.
	revoke := fmt.Sprintf(`{"t":"device","user":%q,"ver":4,"op":"revoke","dev":%q}`, browser.ID(), laptop.ID())
	if _, err := s.Logs[browser.ID()].Apply(browser.Sign(revoke)); err != nil {
		t.Fatal(err)
	}
	equal(t, []bool{s.Serves(laptop.ID()), s.Serves(node.ID())}, []bool{false, true})
	res := apply(t, home.Table{}, s, receipts{}, laptop, signed(laptop, "note", 1, ``))
	equal(t, res.Answer, store.Outcome{Code: "unknown_device"})
}

// A device that two logs hold belongs to the person with the lower user id.
func TestADeviceInTwoLogsBelongsToTheLowerUser(t *testing.T) {
	s := group(t)
	s.People[id(12)] = person(id(12), guest, "member")
	shared := device(7)
	for _, first := range []identity.Key{browser, guest} {
		l := s.Logs[first.ID()]
		add := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":"browser","name":"shared"}`, first.ID(), len(l.Entries)+1, shared.ID())
		if _, err := l.Apply(first.Sign(add)); err != nil {
			t.Fatal(err)
		}
	}
	lower, human := browser.ID(), ownerID
	if guest.ID() < lower {
		lower, human = guest.ID(), id(12)
	}
	for range 20 { // a map is walked in a new order every time
		equal(t, who(t, s, shared, ``), home.Who{Device: shared.ID(), User: lower, Human: human})
	}
}
