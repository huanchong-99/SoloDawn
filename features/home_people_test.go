//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_people"] = func(sc *godog.ScenarioContext, _ fixtures) {
		w := &homeGroupWorld{}
		var join *home.Joining
		var member string
		admin := exampleKey("people admin")
		sc.Step(`^a people home with an invited visitor$`, func() error {
			w.now = time.UnixMilli(1763600000000)
			if err := w.initCreated("Project", "owner"); err != nil {
				return err
			}
			for id, person := range w.state.People {
				if person.User == w.guest.ID() {
					delete(w.state.People, id)
				}
			}
			log, err := homeLog([]identity.Key{admin})
			if err != nil {
				return err
			}
			w.state.Logs[admin.ID()] = log
			adminID := string(exampleID("people admin member"))
			w.state.People[adminID] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: adminID}, User: admin.ID()}, Role: "admin"}
			var owner string
			for id, person := range w.state.People {
				if person.Role == "owner" {
					owner = id
				}
			}
			join = &home.Joining{Group: w.state.ID, By: owner, Open: true, Log: w.state.Logs[w.guest.ID()]}
			return nil
		})
		apply := func(by string, payload any, joining *home.Joining) error {
			key := w.browser
			if by == "visitor" {
				key = w.guest
			}
			if by == "admin" {
				key = admin
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			w.last, err = home.Commands.ApplyJoining(w.state, w.store, w.store, key.Sign(string(raw)), joining, w.now)
			if err == nil {
				w.state.Commit(w.last)
			}
			return err
		}
		joinVisitor := func() error {
			p := proto.Join{HomeCommand: proto.HomeCommand{Type: "join", Nonce: w.nextID("join"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Name: "visitor"}
			if member == "" {
				member = string(p.Nonce)
			}
			return apply("visitor", p, join)
		}
		sc.Step(`^the visitor joins the people group$`, joinVisitor)
		sc.Step(`^the visitor joins the people group again after a restore$`, joinVisitor)
		sc.Step(`^the visitor is a member with a device row and a joined message$`, func() error {
			if !w.last.Answer.OK || !w.last.InviteUsed || len(w.last.Rows) != 3 || w.state.People[member].Role != "member" || !w.state.Serves(w.guest.ID()) {
				return fmt.Errorf("unexpected joined result: %+v", w.last)
			}
			msg := w.last.Rows[2].Value().(proto.MsgRow)
			var body proto.MemberJoinedBody
			if err := json.Unmarshal(msg.Body, &body); err != nil {
				return err
			}
			if body.Code != "member_joined" || body.Member != member || body.By != join.By {
				return fmt.Errorf("unexpected join body: %+v", body)
			}
			return nil
		})
		sc.Step(`^the join writes no rows and leaves the invitation open$`, func() error {
			if !w.last.Answer.OK || w.last.InviteUsed || len(w.last.Rows) != 0 {
				return fmt.Errorf("unexpected restored join: %+v", w.last)
			}
			return nil
		})
		sc.Step(`^the owner promotes the visitor to admin$`, func() error {
			role := "admin"
			return apply("owner", proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("set"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: "set", ID: proto.ID(member)}, Role: &role}, nil)
		})
		sc.Step(`^the visitor's stored role is admin$`, func() error {
			if !w.last.Answer.OK || w.state.People[member].Role != "admin" {
				return fmt.Errorf("role is %q", w.state.People[member].Role)
			}
			return nil
		})
		sc.Step(`^another admin removes the visitor$`, func() error {
			return apply("admin", proto.MemberCommand{HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("remove"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: "remove", ID: proto.ID(member)}, nil)
		})
		sc.Step(`^the people command answers forbidden$`, func() error {
			if w.last.Answer.Code != "forbidden" {
				return fmt.Errorf("code is %q", w.last.Answer.Code)
			}
			return nil
		})
		sc.Step(`^the visitor leaves the people group$`, func() error {
			return apply("visitor", proto.MemberCommand{HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("leave"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: "remove", ID: proto.ID(member)}, nil)
		})
		sc.Step(`^the visitor and device are gone and a departure message has no remover$`, func() error {
			if !w.last.Answer.OK || len(w.last.Rows) != 3 || w.state.Serves(w.guest.ID()) {
				return fmt.Errorf("unexpected removal: %+v", w.last)
			}
			msg := w.last.Rows[2].Value().(proto.MsgRow)
			var body proto.MemberRemovedBody
			if err := json.Unmarshal(msg.Body, &body); err != nil {
				return err
			}
			if body.Code != "member_removed" || body.User != w.guest.ID() || body.Name != "visitor" || body.By != nil {
				return fmt.Errorf("unexpected removal body: %+v", body)
			}
			return nil
		})
	}
}
