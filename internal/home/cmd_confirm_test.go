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

func cfmState(t *testing.T) *home.State {
	t.Helper()
	s := calState(t)
	s.People[postGuest] = person(postGuest, guest, "admin")
	for _, m := range []string{calB, calC} {
		a := ai(m, guest)
		a.User, a.Account, a.Name = guest.ID(), "personal", "Other AI"
		a.Approvers = []string{guest.ID()}
		s.AIs[m] = a
	}
	return s
}

func cfmCard(t *testing.T, res home.Result) proto.MsgRow {
	t.Helper()
	for _, row := range res.Rows {
		if m, ok := row.Value().(proto.MsgRow); ok && m.Kind == "confirm" {
			return m
		}
	}
	t.Fatal("no confirmation card")
	return proto.MsgRow{}
}

func cfmAct(by identity.Key, n byte, card, action, extra string) proto.Envelope {
	return signed(by, "act", n, fmt.Sprintf(`,"msg":%q,"action":%q%s`, card, action, extra))
}

func TestConfirmMentionDecisionsAndReceipts(t *testing.T) {
	for _, action := range []string{"confirm", "deny"} {
		t.Run(action, func(t *testing.T) {
			s, d, r := cfmState(t), cardDisk{}, receipts{}
			p := postMake(120, "", "text", nil, []string{calB}, nil, "Look here", proto.EmptyBody{})
			env, made := postSend(t, s, d, browser, p)
			equal(t, made.Answer, store.Outcome{OK: true})
			equal(t, made.Items, []home.Item(nil))
			card := cfmCard(t, made)
			req := string(p.Nonce)
			equal(t, made.Rows[0].Value().(proto.MsgRow).Env, &env)
			want := proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: card.ID, Seq: 12}, Pos: 12, Group: groupID,
				Main: true, By: "system", Kind: "confirm", Timestamp: clock, Mentions: []string{},
				Re: &req, Body: briefRaw(t, proto.ConfirmBody{What: "direct", AI: calB, To: []string{guest.ID()}}),
				State: &proto.CardState{Status: "pending"},
			}
			equal(t, card, want)
			cardCommit(s, d, made)
			// An admin is not the owner, nor may an owner's AI decide.
			for i, by := range []identity.Key{browser, guest} {
				extra := ""
				if i == 1 {
					extra = `,"as":"` + calB + `"`
				}
				bad := cardApply(t, s, d, r, by, cfmAct(by, byte(121+i), card.ID, action, extra))
				equal(t, bad.Answer.Code, "forbidden")
			}
			act := cfmAct(guest, 123, card.ID, action, "")
			res := cardApply(t, s, d, r, guest, act)
			equal(t, res.Answer, store.Outcome{OK: true})
			cardCommit(s, d, res)
			status := "confirmed"
			var items []home.Item
			if action == "deny" {
				status = "denied"
			} else {
				items = []home.Item{{AI: calB, Why: "mention", Msg: req}}
			}
			equal(t, res.Items, items)
			by, at := postGuest, int64(clock)
			equal(t, d[card.ID].State, &proto.CardState{Status: status, By: &by, At: &at, Act: &act})
			replay := cardApply(t, s, d, r, guest, act)
			equal(t, replay.Answer, store.Outcome{OK: true})
			equal(t, replay.Rows, []home.Row(nil))
			equal(t, replay.Items, []home.Item(nil))
			conflict := cardApply(t, s, d, r, guest, cfmAct(guest, 124, card.ID, action, ""))
			equal(t, conflict.Answer.Code, "conflict")
		})
	}
}

func TestConfirmMultipleCardsThreadAndSecretary(t *testing.T) {
	s, d := cfmState(t), cardDisk{}
	p := postMake(130, "", "text", &calOtherT, []string{calB, calC, calA}, nil, "Both", proto.EmptyBody{})
	_, res := postSend(t, s, d, browser, p)
	equal(t, res.Answer.OK, true)
	equal(t, len(res.Rows), 4)
	equal(t, res.Items, []home.Item{{AI: calA, Why: "mention", Task: calOtherT, Msg: id(130)}})
	first := res.Rows[1].Value().(proto.MsgRow)
	second := res.Rows[2].Value().(proto.MsgRow)
	if first.ID == second.ID {
		t.Fatal("two AIs share a card")
	}
	equal(t, first.Thread, &calOtherT)
	task := res.Rows[3].Value().(proto.TaskRow)
	equal(t, task.Thread.Count, 5)
	equal(t, task.Thread.Last, &proto.ThreadMessage{By: "system", Kind: "confirm", Timestamp: clock})
	cardCommit(s, d, res)
	act := cardApply(t, s, d, receipts{}, guest, cfmAct(guest, 131, first.ID, "confirm", ""))
	equal(t, act.Items, []home.Item{{AI: calB, Why: "mention", Task: calOtherT, Msg: id(130)}})

	s.Group.Secretary = calB
	_, res = postSend(t, s, d, browser, postMake(132, "", "text", nil, nil, nil, "Main", proto.EmptyBody{}))
	equal(t, len(res.Items), 0)
	equal(t, cfmCard(t, res).Kind, "confirm")
	_, res = postSend(t, s, d, guest, postMake(133, "", "text", nil, []string{calB}, nil, "Own", proto.EmptyBody{}))
	equal(t, len(res.Rows), 1)
	equal(t, res.Items, []home.Item{{AI: calB, Why: "mention", Msg: id(133)}})
	task = s.Tasks[calOtherT]
	task.Assignee, task.Reviewer, task.Step = calB, calC, "review"
	s.Tasks[calOtherT] = task
	_, res = postSend(t, s, d, browser, postMake(134, "", "text", &calOtherT, []string{calB, calC}, nil, "Context", proto.EmptyBody{}))
	equal(t, len(res.Rows), 2)
	equal(t, res.Items, []home.Item{{AI: calB, Why: "mention", Task: calOtherT, Msg: id(134)}, {AI: calC, Why: "mention", Task: calOtherT, Msg: id(134)}})
}

func TestConfirmCallsAndAIMentions(t *testing.T) {
	for _, kind := range []string{"call", "question"} {
		for _, action := range []string{"confirm", "deny"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				s, d := cfmState(t), cardDisk{}
				var res home.Result
				if kind == "call" {
					_, res = calSend(t, s, d, receipts{}, laptop, calCallCmd(140, clock, calA, &calOtherT, "Port?", proto.CallBody{To: calC, Depth: 1, Wait: true}))
				} else {
					_, res = postSend(t, s, d, laptop, postMake(140, calA, kind, &calOtherT, []string{calC}, nil, "Port?", proto.EmptyBody{}))
				}
				equal(t, res.Answer.OK, true)
				card := cfmCard(t, res)
				cardCommit(s, d, res)
				if kind == "question" {
					delete(s.AIs, calA)
				}
				act := cfmAct(guest, 141, card.ID, action, "")
				res = cardApply(t, s, d, receipts{}, guest, act)
				equal(t, res.Answer.OK, true)
				cardCommit(s, d, res)
				if action == "confirm" {
					equal(t, res.Items, []home.Item{{AI: calC, Why: "mention", Task: calOtherT, Msg: id(140)}})
				} else {
					want := []home.Item{{AI: aiID, Why: "notice", Task: calOtherT, Msg: card.ID, Code: "confirm_denied"}}
					if kind == "call" {
						want = append([]home.Item{{AI: calA, Why: "reply", Task: calOtherT, Msg: card.ID}}, want...)
						at := int64(clock)
						equal(t, d[id(140)].State, &proto.CardState{Status: "denied", At: &at})
					}
					equal(t, res.Items, want)
				}
			})
		}
	}
}

func cfmWatch(n byte, ai string) proto.Post[proto.ConfirmSigned] {
	return postMake(n, "", "confirm", nil, nil, nil, "", proto.ConfirmSigned{What: "watch", AI: proto.ID(ai)})
}

func TestConfirmWatchAndExpiry(t *testing.T) {
	for _, action := range []string{"confirm", "deny"} {
		s, d := cfmState(t), cardDisk{}
		env, res := postSend(t, s, d, browser, cfmWatch(150, calB))
		equal(t, res.Answer.OK, true)
		card := cfmCard(t, res)
		equal(t, card.By, ownerID)
		equal(t, card.Env, &env)
		equal(t, card.Re, (*string)(nil))
		cardCommit(s, d, res)
		res = cardApply(t, s, d, receipts{}, guest, cfmAct(guest, 151, card.ID, action, ""))
		equal(t, res.Answer.OK, true)
		var want []home.Item
		if action == "confirm" {
			want = []home.Item{{AI: calB, Why: "watch", Msg: card.ID}}
		}
		equal(t, res.Items, want)
	}
	for _, cause := range []string{"live", "ai", "watcher", "request"} {
		t.Run(cause, func(t *testing.T) {
			s, d := cfmState(t), cardDisk{}
			_, watch := postSend(t, s, d, browser, cfmWatch(152, calB))
			cardCommit(s, d, watch)
			_, mention := postSend(t, s, d, browser, postMake(153, "", "text", nil, []string{calB}, nil, "Go", proto.EmptyBody{}))
			cardCommit(s, d, mention)
			wcard, mcard := cfmCard(t, watch), cfmCard(t, mention)
			d[id(155)] = proto.MsgRow{RowHeader: proto.RowHeader{ID: id(155)}, Kind: "approval", State: &proto.CardState{Status: "pending"}}
			switch cause {
			case "ai":
				delete(s.AIs, calB)
			case "watcher":
				delete(s.People, ownerID)
			case "request":
				delete(d, id(153))
			}
			res := home.Timers["confirm"](s, d, clock+1)
			want := []string{}
			for _, r := range res.Rows {
				m := r.Value().(proto.MsgRow)
				at := int64(clock + 1)
				equal(t, m.State, &proto.CardState{Status: "expired", At: &at})
				want = append(want, m.ID)
			}
			switch cause {
			case "live":
				equal(t, want, []string{})
			case "ai":
				equal(t, want, []string{wcard.ID, mcard.ID})
			case "watcher":
				equal(t, want, []string{wcard.ID})
			case "request":
				equal(t, want, []string{mcard.ID})
			}
			if cause != "live" {
				card := mcard.ID
				if cause == "watcher" {
					card = wcard.ID
				}
				rejected := cardApply(t, s, d, receipts{}, guest, cfmAct(guest, 156, card, "confirm", ""))
				equal(t, rejected.Answer.Code, "conflict")
			}
		})
	}
	equal(t, home.Timers["confirm"](cfmState(t), cardDisk{}, clock), home.Result{})
}

func TestConfirmRefusals(t *testing.T) {
	for name, tc := range map[string]struct{ as, body, code string }{
		"AI posts":   {calA, `{"what":"watch","ai":"` + calB + `"}`, "forbidden"},
		"malformed":  {"", `{"what":"watch","ai":true}`, "bad_request"},
		"wrong kind": {"", `{"what":"direct","ai":"` + calB + `"}`, "bad_request"},
		"missing AI": {"", `{"what":"watch","ai":"` + id(200) + `"}`, "conflict"},
		"own AI":     {"", `{"what":"watch","ai":"` + calA + `"}`, "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := cfmState(t)
			as := ""
			if tc.as != "" {
				as = fmt.Sprintf(`,"as":%q`, tc.as)
			}
			env := signed(laptop, "post", 160, fmt.Sprintf(`%s,"kind":"confirm","thread":null,"main":false,"re":null,"mentions":[],"text":"","body":%s`, as, tc.body))
			res := cardApply(t, s, cardDisk{}, receipts{}, laptop, env)
			equal(t, res.Answer.Code, tc.code)
		})
	}
	s, d := cfmState(t), cardDisk{}
	_, made := postSend(t, s, d, browser, cfmWatch(161, calB))
	cardCommit(s, d, made)
	for i, extra := range []string{`,"arg":"unused"`, `,"arg":true`} {
		res := cardApply(t, s, d, receipts{}, guest, cfmAct(guest, byte(162+i), id(161), "confirm", extra))
		equal(t, res.Answer.Code, "bad_request")
	}
	var body proto.ConfirmBody
	equal(t, json.Unmarshal(cfmCard(t, made).Body, &body), nil)
	equal(t, body.To, []string{guest.ID()})
}
