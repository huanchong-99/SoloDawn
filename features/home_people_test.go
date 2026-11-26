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
		var otherGroup bool
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
			env := key.Sign(string(raw))
			if joining == nil {
				w.last, err = home.Commands.Apply(w.state, homePeopleDisk{Disk: w.store, other: otherGroup, logs: w.state.Logs}, w.store, env, w.now)
			} else {
				c := home.Command{Env: env, Disk: w.store, Joining: joining, Now: w.now.UnixMilli(), Who: home.Who{Device: key.ID(), User: joining.Log.User}}
				if err = json.Unmarshal(raw, &c.PayloadHeader); err != nil {
					return err
				}
				for id, person := range w.state.People {
					if person.User == c.Who.User {
						c.Who.Human = id
					}
				}
				var refusal *home.Refusal
				w.last, refusal = home.Commands["join"](w.state, c)
				if refusal != nil {
					return fmt.Errorf("join: %s", refusal.Code)
				}
				w.last = w.state.Seal(w.last)
			}
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
		sc.Step(`^the visitor also belongs to another group homed here$`, func() { otherGroup = true })
		addPerson := func(user, name string) error {
			p := proto.MemberAddPerson{HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("add " + name), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: "add_person", User: user, Name: name}
			if member == "" {
				member = string(p.Nonce)
			}
			return apply("owner", p, nil)
		}
		sc.Step(`^the owner adds the visitor with add_person as "([^"]*)"$`, func(name string) error { return addPerson(w.guest.ID(), name) })
		sc.Step(`^the owner adds a person this home does not know with add_person$`, func() error { return addPerson(exampleKey("people stranger").ID(), "stranger") })
		sc.Step(`^the people command answers "([^"]*)"$`, func(code string) error {
			if w.last.Answer.Code != code {
				return fmt.Errorf("answer is %+v (%s), want %q", w.last.Answer, w.last.Msg, code)
			}
			return nil
		})
		sc.Step(`^the visitor is a member with one device row per entry of their log and a joined message by the owner$`, func() error {
			return w.checkAdded(member)
		})
		sc.Step(`^only this group's device rows and device log are retired$`, func() error {
			if !w.last.Answer.OK || len(w.last.Rows) != 3 || len(w.last.Writes) != 0 || w.state.Logs[w.guest.ID()] != nil {
				return fmt.Errorf("unexpected group-local removal: %+v", w.last)
			}
			gone := w.last.Rows[1].Value().(proto.GoneRow)
			if gone.Type != "device" || gone.ID != w.guest.ID()+":1" || !gone.Gone {
				return fmt.Errorf("unexpected retired device: %+v", gone)
			}
			return nil
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

// checkAdded checks what an accepted add_person wrote: the member row, one
// device row per entry of the person's log, and member_joined by the owner.
func (w *homeGroupWorld) checkAdded(member string) error {
	person, rows := w.state.People[member], 2+len(w.state.Logs[w.guest.ID()].Entries)
	got := [6]any{w.last.Answer.OK, w.last.GroupsChanged, w.last.InviteUsed, len(w.last.Rows), person.Role, person.User}
	if got != [6]any{true, true, false, rows, "member", w.guest.ID()} {
		return fmt.Errorf("unexpected add_person result: %+v", w.last)
	}
	var body proto.MemberJoinedBody
	if err := json.Unmarshal(w.last.Rows[rows-1].Value().(proto.MsgRow).Body, &body); err != nil {
		return err
	}
	want := proto.MemberJoinedBody{SystemCode: proto.SystemCode{Code: "member_joined"}, Member: member, By: w.ownerMember()}
	if body != want {
		return fmt.Errorf("joined body %+v, want %+v", body, want)
	}
	return nil
}

func (w *homeGroupWorld) ownerMember() string {
	for id, person := range w.state.People {
		if person.Role == "owner" {
			return id
		}
	}
	return ""
}

type homePeopleDisk struct {
	home.Disk
	other bool
	logs  map[string]*identity.Log
}

func (d homePeopleDisk) OtherMember(string) bool { return d.other }

// DeviceLog is the log the node holds of a person who is a member of another
// group homed here.
func (d homePeopleDisk) DeviceLog(user string) []proto.Envelope {
	if log := d.logs[user]; d.other && log != nil {
		return log.Entries
	}
	return nil
}
