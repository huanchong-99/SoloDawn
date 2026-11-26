//go:build acceptance

package features

import (
	"fmt"
	"slices"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// A group of people without an AI (A9.22), and a person added without a link
// (A9.46; contracts/step-2/people.md section 2.5).
func registerS2PeopleGroupSteps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^P writes the note "([^"]*)" in the people group$`, w.pWritesNote)
	sc.Step(`^Q's browser reads the main line "([^"]*)", the note "([^"]*)" and the members P and Q, and the group has no AI$`, w.verifyPeopleGroup)
	sc.Step(`^P creates group "([^"]*)" on P1 and adds Q to it as "([^"]*)"$`, w.addQToNewGroup)
	sc.Step(`^Q's connected browser lists "([^"]*)" without a reload$`, w.qListsOtherGroup)
	sc.Step(`^a browser of Q that connects to P1 later lists "([^"]*)"$`, w.laterBrowserLists)
	sc.Step(`^add_person on P1 for a person who is in no group there is refused with "([^"]*)"$`, w.addStrangerRefused)
}

func (w *s2PeopleWorld) homeHeader(typ string, gid proto.ID) proto.HomeCommand {
	return proto.HomeCommand{Type: typ, Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli(), Group: gid}
}

func (w *s2PeopleWorld) pWritesNote(text string) error {
	note := proto.Note{HomeCommand: w.homeHeader("note", w.gid), ID: w.cast.NextID(), Text: text}
	_, err := w.cast.SendCmd(w.cast.P, w.cast.P.Conns[w.cast.P1.ID()].conn, note)
	return err
}

func (w *s2PeopleWorld) verifyPeopleGroup(text, note string) error {
	err := w.drainViewUntil(w.cast.Ctx, w.qHomeBC, w.qView, string(w.gid), func() bool {
		_, p := w.qView.humanByUser(w.cast.P.ID())
		_, q := w.qView.humanByUser(w.cast.Q.ID())
		return w.qView.texts[text] && p && q
	})
	if err != nil {
		return err
	}
	if len(w.qView.ais) != 0 {
		return fmt.Errorf("the group has AI members %v", w.qView.ais)
	}
	return w.qReadsNote(note)
}

// qReadsNote fetches the group's notes from Q's browser, as its notes panel
// does: a cold sync carries no note (contracts/datachannel.md section 4).
func (w *s2PeopleWorld) qReadsNote(note string) error {
	raw, err := w.qHomeBC.request(w.cast.Ctx, func(id int64) any {
		return proto.FetchKind{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: string(w.gid), Limit: 50}, Kind: "note"}
	})
	if err != nil {
		return err
	}
	var fetched proto.Fetched
	if err := proto.Decode(raw, &fetched); err != nil {
		return err
	}
	for _, row := range fetched.Rows {
		var n proto.NoteRow
		if proto.Decode(row, &n) == nil && n.Text == note {
			return nil
		}
	}
	return fmt.Errorf("Q's browser fetched no note %q: %s", note, raw)
}

// addQToNewGroup creates a second group on P1 and adds Q, who is a member of
// the first, with member add_person.
func (w *s2PeopleWorld) addQToNewGroup(name, qName string) error {
	gid, err := w.cast.CreateGroup(w.cast.P, w.cast.P1, name)
	if err != nil {
		return err
	}
	w.otherGID = gid
	add := proto.MemberAddPerson{HomeCommand: w.homeHeader("member", gid), Op: "add_person", User: w.cast.Q.ID(), Name: qName}
	_, err = w.cast.SendCmd(w.cast.P, w.cast.P.Conns[w.cast.P1.ID()].conn, add)
	return err
}

func (w *s2PeopleWorld) qListsOtherGroup(name string) error {
	return w.drainViewUntil(w.cast.Ctx, w.qHomeBC, w.qView, string(w.otherGID), func() bool {
		return slices.ContainsFunc(w.qView.groups, func(g proto.GroupSummary) bool { return g.Group == string(w.otherGID) && g.Name == name })
	})
}

// laterBrowserLists enrolls another browser of Q, connects it to P1 and
// reads the groups its welcome lists.
func (w *s2PeopleWorld) laterBrowserLists(name string) error {
	bc, err := w.connectLaterBrowser()
	if err != nil {
		return err
	}
	raw, err := requestWait(w.cast.Ctx, bc.conn, nil, func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 3, Kind: "browser", Name: "Q later", Addrs: []string{}}
	})
	if err != nil {
		return err
	}
	var welc proto.Welcome
	if err := proto.Decode(raw, &welc); err != nil {
		return err
	}
	if !slices.ContainsFunc(welc.Groups, func(g proto.GroupSummary) bool { return g.Group == string(w.otherGID) && g.Name == name }) {
		return fmt.Errorf("the welcome of Q's later browser lists %+v, want %q", welc.Groups, name)
	}
	return nil
}

// connectLaterBrowser connects the new browser once P1 has applied its device
// entry, which the entry's device row on Q's connected browser shows: until
// then P1 drops the browser's signalling as unknown_device
// (contracts/envelope.md section 3).
func (w *s2PeopleWorld) connectLaterBrowser() (*peerBrowserConn, error) {
	b, err := w.cast.AddBrowser(w.cast.Q, "Q later")
	if err != nil {
		return nil, err
	}
	if err := w.awaitQJoinedOnView(w.qView, w.qHomeBC); err != nil {
		return nil, err
	}
	return openS2PeopleCtl(w.cast.Ctx, w.cleanup, b, w.cast.P1.ID())
}

func (w *s2PeopleWorld) addStrangerRefused(code string) error {
	add := proto.MemberAddPerson{HomeCommand: w.homeHeader("member", w.otherGID), Op: "add_person", User: newRandomPeerKey().ID(), Name: "Stranger"}
	return w.expectSignedCmdError(w.cast.P.Conns[w.cast.P1.ID()].conn, w.cast.P.Key, add, code)
}
