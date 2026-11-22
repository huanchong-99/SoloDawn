package home

import (
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestPeopleWritesAndOtherMembership(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.Write(func(tx *store.Tx) error {
		w := peopleLog{proto.DeviceRow{DeviceChange: proto.DeviceChange{User: "visitor", Version: 1}, Env: proto.Envelope{Payload: "entry"}}}
		for range 2 {
			if err := w.Put(tx); err != nil {
				return err
			}
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM device_log WHERE user='visitor' AND ver=1 AND env=?", string(briefJSON(w.row.Env))).Scan(&count); err != nil {
			return err
		}
		runSame(t, count, 1)
		for _, g := range []string{"here", "elsewhere"} {
			row := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "person", Seq: 1}, Kind: "human", User: "visitor"}}
			if err := store.Put(tx, g, 1, row); err != nil {
				return err
			}
		}
		d := &runDisk{tx: tx, group: "here"}
		runSame(t, d.OtherMember("visitor"), true)
		runSame(t, d.OtherMember("absent"), false)
		runSame(t, d.err, nil)
		if _, err := tx.Exec("INSERT INTO push_sub VALUES('browser','visitor','{}','{}','[]'),('other','retained','{}','{}','[]')"); err != nil {
			return err
		}
		if err := (peopleForget{"visitor"}).Put(tx); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT count(*) FROM device_log").Scan(&count); err != nil {
			return err
		}
		runSame(t, count, 0)
		if err := tx.QueryRow("SELECT count(*) FROM push_sub WHERE user='retained'").Scan(&count); err != nil {
			return err
		}
		runSame(t, count, 1)
		if err := tx.QueryRow("SELECT count(*) FROM push_sub WHERE user='visitor'").Scan(&count); err != nil {
			return err
		}
		runSame(t, count, 0)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPeopleWriteFailures(t *testing.T) {
	for _, table := range []string{"device_log", "push_sub", "row"} {
		t.Run(table, func(t *testing.T) {
			db, err := store.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			err = db.Write(func(tx *store.Tx) error {
				if _, err := tx.Exec("DROP TABLE " + table); err != nil {
					return err
				}
				var failed error
				if table == "row" {
					d := &runDisk{tx: tx, group: "here"}
					runSame(t, d.OtherMember("visitor"), false)
					failed = d.err
				} else {
					failed = (peopleForget{"visitor"}).Put(tx)
				}
				if failed == nil || !strings.Contains(failed.Error(), "no such table: "+table) {
					return fmt.Errorf("unexpected failure: %v", failed)
				}
				if table == "device_log" {
					if err := (peopleLog{}).Put(tx); err == nil || !strings.Contains(err.Error(), "no such table: device_log") {
						return fmt.Errorf("unexpected log error: %v", err)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPeopleForgetClearsPushMemory(t *testing.T) {
	home := "people-forget-test"
	wpushApplyWrite(home, wpushPut{wpushSub{device: "removed", user: "visitor"}})
	wpushApplyWrite(home, wpushPut{wpushSub{device: "retained", user: "other"}})
	wpushApplyWrite(home, peopleForget{"visitor"})
	wpushMu.Lock()
	defer wpushMu.Unlock()
	runSame(t, wpushHomes[home].subs, map[string]wpushSub{"retained": {device: "retained", user: "other"}})
	delete(wpushHomes, home)
}

func TestPeopleOwnerAndAIRestrictions(t *testing.T) {
	s := NewState("g", "home", "owner")
	owner := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "owner-member"}, User: "owner"}, Role: "owner"}
	s.People[owner.ID] = owner
	runSame(t, peopleMayRemove(s, Who{User: "owner", AI: "ai"}, owner), false)
	role := "admin"
	_, refusal := peopleSet(s, Command{Who: Who{User: "owner", AI: "ai"}}, proto.HumanMemberRow{}, proto.MemberSet{Role: &role})
	runSame(t, refusal, &Refusal{"forbidden", "only the owner changes another person's role"})
	s.drop(proto.GoneRow{RowHeader: owner.RowHeader, Gone: true})
	runSame(t, len(s.People), 0)
}
