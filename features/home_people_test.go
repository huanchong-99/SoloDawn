//go:build acceptance

package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_people"] = func(sc *godog.ScenarioContext, _ fixtures) {
		w := &homeGroupWorld{}
		var join *home.Joining
		var member string
		var otherGroup bool
		admin := exampleKey("people admin")
		// The setting invites lives in a store of its own: the writes of each
		// result go to it, and the group's open code is read from it into
		// Command, as the loop reads it in the command's transaction.
		var db *store.Store
		var keys map[string]identity.Key
		var logs map[string]*identity.Log
		var codes []string // as shown when made
		var ends int64     // of the last code made
		var last any
		cleanup := afterScenario(sc)
		sc.Step(`^a people home with an invited visitor$`, func() error {
			w.now = time.UnixMilli(1763600000000)
			if err := w.initCreated("Project", "owner"); err != nil {
				return err
			}
			var err error
			db, keys, logs, err = w.peopleCast(admin, cleanup)
			if err != nil {
				return err
			}
			codes = nil
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
			join = &home.Joining{Group: w.state.ID, By: w.ownerMember(), Open: true, Log: w.state.Logs[w.guest.ID()]}
			return nil
		})
		openCode := func() (code store.Invite, err error) {
			err = db.Write(func(tx *store.Tx) error {
				open, err := tx.Invites(w.now.UnixMilli())
				code = open[w.state.ID]
				return err
			})
			return code, err
		}
		// run is a command as the loop runs it on a joining connection, or one
		// that reads the group's code: Command is filled in by hand.
		run := func(key string, c home.Command, raw []byte) (home.Result, error) {
			code, err := openCode()
			if err == nil {
				err = json.Unmarshal(raw, &c.PayloadHeader)
			}
			c.Code, c.Disk, c.Now = code, w.store, w.now.UnixMilli()
			for id, person := range w.state.People {
				if person.User == c.Who.User {
					c.Who.Human = id
				}
			}
			res, refusal := home.Commands[key](w.state, c)
			if refusal != nil {
				return home.Result{Answer: store.Outcome{Code: refusal.Code}}, err
			}
			return w.state.Seal(res), err
		}
		apply := func(by string, payload any, joining *home.Joining) error {
			key := keys[by]
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			env := key.Sign(string(raw))
			last = payload
			if joining == nil {
				w.last, err = home.Commands.Apply(w.state, homePeopleDisk{Disk: w.store, other: otherGroup, logs: w.state.Logs}, w.store, env, w.now)
			} else {
				w.last, err = run("join", home.Command{Env: env, Joining: joining, Who: home.Who{Device: key.ID(), User: joining.Log.User}}, raw)
			}
			if err != nil {
				return err
			}
			w.state.Commit(w.last)
			return db.Write(func(tx *store.Tx) error {
				for _, write := range w.last.Writes {
					if err := write.Put(tx); err != nil {
						return err
					}
				}
				return nil
			})
		}
		joinAs := func(name string, joining *home.Joining) error {
			p := proto.Join{HomeCommand: proto.HomeCommand{Type: "join", Nonce: w.nextID("join"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Name: name}
			if member == "" && name == "visitor" {
				member = string(p.Nonce)
			}
			return apply(name, p, joining)
		}
		joinVisitor := func() error { return joinAs("visitor", join) }
		sc.Step(`^the visitor joins the people group$`, joinVisitor)
		sc.Step(`^the visitor joins the people group again after a restore$`, joinVisitor)
		sc.Step(`^(the visitor|a second person|a third person) joins the people group by the (code|first code)$`, func(who, which string) error {
			name := map[string]string{"the visitor": "visitor", "a second person": "second", "a third person": "third"}[who]
			shown := codes[len(codes)-1]
			if which == "first code" {
				shown = codes[0]
			}
			secret, err := identity.ParseSecret(shown)
			if err != nil {
				return err
			}
			return joinAs(name, &home.Joining{Group: w.state.ID, Code: secret, Log: logs[name]})
		})
		codeCommand := func(op string, hours *int) proto.Code {
			return proto.Code{HomeCommand: proto.HomeCommand{Type: "code", Nonce: w.nextID("code " + op), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: op, Hours: hours}
		}
		makeCode := func(by string, hours int) error {
			if err := apply(by, codeCommand("new", &hours), nil); err != nil {
				return err
			}
			codes, ends = append(codes, w.last.Code), w.last.Expires
			return nil
		}
		sc.Step(`^the owner makes an invitation code for (\d+) hours?$`, func(hours int) error { return makeCode("owner", hours) })
		sc.Step(`^the visitor, a member, makes an invitation code for (\d+) hours$`, func(hours int) error { return makeCode("visitor", hours) })
		sc.Step(`^the owner turns the code off$`, func() error { return apply("owner", codeCommand("off", nil), nil) })
		sc.Step(`^the owner sends that code command again$`, func() error { return apply("owner", last, nil) })
		sc.Step(`^the code has 16 characters in four groups and ends (\d+) hours on$`, func(hours int64) error {
			shown := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$`)
			if !w.last.Answer.OK || !shown.MatchString(w.last.Code) || w.last.Expires != w.now.UnixMilli()+hours*3_600_000 {
				return fmt.Errorf("unexpected code answer: %+v", w.last)
			}
			return nil
		})
		// shows sends code show and checks that it answers the code shown when
		// it was made, or no field for "".
		shows := func(want string) error {
			raw, err := json.Marshal(codeCommand("show", nil))
			if err != nil {
				return err
			}
			res, err := run("code/show", home.Command{Env: w.browser.Sign(string(raw)), Who: home.Who{Device: w.browser.ID(), User: w.browser.ID()}}, raw)
			if err == nil && (!res.Answer.OK || res.Code != want) {
				err = fmt.Errorf("code show answered %+v, want the code %q", res, want)
			}
			return err
		}
		sc.Step(`^the people group has no open code$`, func() error { return shows("") })
		sc.Step(`^the code is still open$`, func() error { return shows(codes[len(codes)-1]) })
		sc.Step(`^the answer is ok with no code and the code is still open$`, func() error {
			if !w.last.Answer.OK || w.last.Code != "" || len(w.last.Writes) != 0 {
				return fmt.Errorf("unexpected repeated code new: %+v", w.last)
			}
			return shows(codes[0])
		})
		sc.Step(`^the joiner is a member let in by the owner$`, func() error {
			if len(w.last.Rows) != 3 {
				return fmt.Errorf("unexpected join by code: %+v", w.last)
			}
			var body proto.MemberJoinedBody
			err := json.Unmarshal(w.last.Rows[2].Value().(proto.MsgRow).Body, &body)
			got := [5]any{w.last.Answer.OK, w.last.InviteUsed, w.last.Rows[0].Value().(proto.HumanMemberRow).Role, body.Code, body.By}
			if err != nil || got != [5]any{true, false, "member", "member_joined", w.ownerMember()} {
				return fmt.Errorf("unexpected join by code: %+v, %v", got, err)
			}
			return nil
		})
		sc.Step(`^the home's clock reaches the end of the code$`, func() { w.now = time.UnixMilli(ends) })
		memberOf := func(name string) string {
			for id, person := range w.state.People {
				if person.User == keys[name].ID() {
					return id
				}
			}
			return ""
		}
		sc.Step(`^the visitor and the second person are still members$`, func() error {
			if memberOf("visitor") == "" || memberOf("second") == "" {
				return fmt.Errorf("people left: %+v", w.state.People)
			}
			return nil
		})
		removes := func(by, name string) error {
			return apply(by, proto.MemberCommand{HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("remove " + name), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: "remove", ID: proto.ID(memberOf(name))}, nil)
		}
		sc.Step(`^the owner removes the visitor$`, func() error { return removes("owner", "visitor") })
		sc.Step(`^the second person leaves the people group$`, func() error { return removes("second", "second") })
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

// peopleCast opens the store of the setting invites, and returns who the
// people scenarios name with the device log each presents when joining: the
// owner, the visitor, another admin, and a second and a third person whom the
// home has never met.
func (w *homeGroupWorld) peopleCast(admin identity.Key, cleanup func(func())) (*store.Store, map[string]identity.Key, map[string]*identity.Log, error) {
	db, err := store.Open(":memory:")
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup(func() { db.Close() })
	second, third := exampleKey("people second"), exampleKey("people third")
	keys := map[string]identity.Key{"owner": w.browser, "visitor": w.guest, "admin": admin, "second": second, "third": third}
	secondLog, err := homeLog([]identity.Key{second})
	thirdLog, thirdErr := homeLog([]identity.Key{third})
	logs := map[string]*identity.Log{"visitor": w.state.Logs[w.guest.ID()], "second": secondLog, "third": thirdLog}
	return db, keys, logs, errors.Join(err, thirdErr)
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
