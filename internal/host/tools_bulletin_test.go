package host

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const bulletinGroup = "H46nb5YsBesaWbHkHmZLow"

func bulletinFixture(t *testing.T) (*Host, *talkHomeStub, Caller) {
	t.Helper()
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	t.Cleanup(h.Settle)
	source := makeGroupRow(talkGroupID, "project-home", "Website", "repo", "trunk")
	h.Rows.Put(talkGroupID, "group", talkGroupID, json.RawMessage(marshalWire(source)))
	home := newTalkHomeStub()
	h.Attach("team-home", home)
	grp := makeGroupRow(bulletinGroup, "team-home", "Team", "", "")
	grp.Kind, grp.Forge = "team", "none"
	h.Rows.Put(bulletinGroup, "group", bulletinGroup, json.RawMessage(marshalWire(grp)))
	person := proto.HumanMemberRow{MemberRow: proto.MemberRow{
		RowHeader: proto.RowHeader{Type: "member", ID: talkOwnerID, Seq: 2}, Group: bulletinGroup,
		Kind: "human", Name: "owner", User: "u1", LastSeen: seamStart,
	}, Role: "owner", Online: true}
	h.Rows.Put(bulletinGroup, "member", talkOwnerID, json.RawMessage(marshalWire(person)))
	err := h.Store.Write(func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key,value) VALUES('homes',?)`, marshalWire([]proto.Home{{Group: bulletinGroup, Home: "team-home", Name: "Team"}}))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	c := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	return h, home, c
}

func TestBulletinToolPostsAsPerson(t *testing.T) {
	h, home, c := bulletinFixture(t)
	out := mustCallTalk[idResult](t, h, c, "post", `{"group":"`+bulletinGroup+`","text":"Ready","mentions":["owner","owner"],"kind":"unsupported","where":"elsewhere","task":"unknown"}`)
	sent := mustDecodeCmd[proto.Post[proto.BulletinSigned]](t, home.asked[0], "/thread", "/re")
	assertEqual(t, sent, proto.Post[proto.BulletinSigned]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(out.ID), Timestamp: seamStart, Group: bulletinGroup},
		Kind:        "bulletin", Text: "Ready", Mentions: []proto.ID{talkOwnerID},
		Body: proto.BulletinSigned{From: talkGroupID, Group: "Website", AI: "AI"},
	})
	out = mustCallTalk[idResult](t, h, c, "post", `{"group":"`+bulletinGroup+`","text":""}`)
	sent = mustDecodeCmd[proto.Post[proto.BulletinSigned]](t, home.asked[1], "/thread", "/re")
	assertEqual(t, sent.Text, "")
	assertEqual(t, sent.Mentions, []proto.ID{})
	assertEqual(t, string(sent.Nonce), out.ID)
	mustCallTalk[idResult](t, h, c, "post", `{"group":"`+bulletinGroup+`","text":"Ready","kind":7,"task":{},"where":[]}`)
}

func TestBulletinToolRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, args, code string
		change           func(*Host, *talkHomeStub, *Caller)
	}{
		{"worker", `{"group":"` + bulletinGroup + `","text":"x"}`, "forbidden", func(_ *Host, _ *talkHomeStub, c *Caller) { c.Secretary = false }},
		{"unknown", `{"group":"missing","text":"x"}`, "not_found", func(*Host, *talkHomeStub, *Caller) {}},
		{"empty", `{"group":"","text":"x"}`, "not_found", func(*Host, *talkHomeStub, *Caller) {}},
		{"mention", `{"group":"` + bulletinGroup + `","text":"x","mentions":["absent"]}`, "not_found", func(*Host, *talkHomeStub, *Caller) {}},
		{"secret", `{"group":"` + bulletinGroup + `","text":"ghp_1234567890abcdefghijklmnopqrstuvwxyz"}`, "secret", func(*Host, *talkHomeStub, *Caller) {}},
		{"disconnected", `{"group":"` + bulletinGroup + `","text":"x"}`, "home_unreachable", func(_ *Host, home *talkHomeStub, _ *Caller) { close(home.done) }},
		{"request", `{"group":"` + bulletinGroup + `","text":"x"}`, "home_unreachable", func(_ *Host, home *talkHomeStub, _ *Caller) { home.err = io.EOF }},
		{"source", `{"group":"` + bulletinGroup + `","text":"x"}`, "not_found", func(h *Host, _ *talkHomeStub, _ *Caller) { h.Rows.Reset(talkGroupID) }},
		{"kind", `{"group":"` + bulletinGroup + `","text":"x"}`, "conflict", func(h *Host, _ *talkHomeStub, _ *Caller) {
			grp, _ := joinCachedGroup(h, bulletinGroup)
			grp.Kind = "project"
			h.Rows.Put(bulletinGroup, "group", bulletinGroup, json.RawMessage(marshalWire(grp)))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, home, c := bulletinFixture(t)
			tc.change(h, home, &c)
			_, err := Tools["post"].Call(t.Context(), h, c, json.RawMessage(tc.args))
			assertErrorCode(t, err, tc.code)
		})
	}
}

func TestBulletinKnownHomes(t *testing.T) {
	h, _, _ := bulletinFixture(t)
	local := makeGroupRow(joinTestGroup3, seamKey().ID(), "Local", "", "")
	local.Kind = "team"
	if err := h.Store.Write(func(tx *store.Tx) error { return store.Put(tx, local.ID, seamStart, local) }); err != nil {
		t.Fatal(err)
	}
	home, err := bulletinHome(h, local.ID)
	assertEqual(t, err, nil)
	assertEqual(t, home, proto.Home{Group: proto.ID(local.ID), Home: seamKey().ID(), Name: "Local"})
	_, err = bulletinHome(h, "unknown")
	assertEqual(t, assertErrorCode(t, err, "not_found").Message, "unknown group; valid groups: BEVDTkBah3oHfAzuxeLAZw (Local), H46nb5YsBesaWbHkHmZLow (Team)")
	if err := h.Store.Write(func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE row"); return err }); err != nil {
		t.Fatal(err)
	}
	_, err = bulletinHome(h, bulletinGroup)
	assertEqual(t, err != nil, true)
	c := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	_, err = Tools["post"].Call(t.Context(), h, c, json.RawMessage(`{"group":"`+bulletinGroup+`","text":"x"}`))
	assertEqual(t, err != nil, true)
}

func TestBulletinEmptyHomes(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	_, err := bulletinHome(h, "unknown")
	assertEqual(t, assertErrorCode(t, err, "not_found").Message, "unknown group; valid groups: ")
	seamSQL(t, h.Store, `INSERT INTO setting(key,value) VALUES('homes','[{"g":7,"home":"device","name":"Team"}]')`)
	_, err = bulletinHome(h, "unknown")
	assertEqual(t, err != nil, true)
}

func TestBulletinReadsTeamStateOnExistingConnection(t *testing.T) {
	for _, connected := range []bool{false, true} {
		h, _, _ := bulletinFixture(t)
		h.Rows.Reset(bulletinGroup)
		home := proto.Home{Group: bulletinGroup, Home: "other-home", Name: "Team"}
		assertErrorCode(t, bulletinTeam(t.Context(), h, home), "home_unreachable")
		if !connected {
			continue
		}
		remote := newJoinFakeHome(h, home.Home)
		grp := makeGroupRow(bulletinGroup, home.Home, "Team", "", "")
		grp.Kind = "team"
		remote.putGroup(grp)
		h.Attach(home.Home, remote)
		assertEqual(t, bulletinTeam(t.Context(), h, home), nil)
		assertEqual(t, remote.syncs[0].StateOnly, true)
		h.Rows.Reset(bulletinGroup)
		remote.syncFn = func(proto.Sync) (json.RawMessage, error) { return nil, errors.New("sync failed") }
		assertEqual(t, bulletinTeam(t.Context(), h, home) != nil, true)
	}
}

func TestBulletinSchemaOnlyForSecretary(t *testing.T) {
	for _, secretary := range []bool{false, true} {
		c := talkCaller(GroupSession, talkSecID, "", secretary, Turn{})
		var input proto.ToolInput
		assertEqual(t, json.Unmarshal(schemaForCaller("post", c, Tools["post"].Schema), &input), nil)
		_, hasGroup := input.Properties["group"]
		assertEqual(t, hasGroup, secretary)
	}
}
