package home_test

import (
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// A rule reads through its Disk what a home keeps on disk only: one message,
// one note, a page of a thread newest first, the pending cards and the
// queued items oldest first. A marker is no row to it.
func TestRunADiskReadsWhatTheStoreHolds(t *testing.T) {
	tables := runTables()
	thread := "t1"
	card := func(id string, in *string) home.Row {
		row := runMsg(id, 0, "a card")
		row.Kind, row.Thread, row.Main, row.State = "decision", in, true, &proto.CardState{Status: "pending"}
		return home.RowOf(row)
	}
	inThread := runMsg("m3", 0, "three")
	inThread.Thread = &thread
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		return home.Result{Rows: []home.Row{
			home.RowOf(runMsg("m1", 0, "one")), home.RowOf(runMsg("m2", 0, "two")), home.RowOf(inThread),
			card("c1", nil), card("c2", &thread), home.RowOf(runNote("n1", 0, "a note")),
		}, Items: []home.Item{{AI: aiID, Why: "notice", Code: "first"}, {AI: aiID, Why: "notice", Code: "second", NotBefore: clock + 5000}}}
	})
	tables.Commands["push"] = runFixed(func(s *home.State, c home.Command) home.Result {
		return home.Result{Rows: []home.Row{home.RowOf(runGone("msg", "m2", 0))}}
	})
	var seen runRecord
	ids := func(rows []proto.MsgRow) (list []string) {
		for _, row := range rows {
			list = append(list, fmt.Sprint(row.ID, "@", row.Pos))
		}
		return list
	}
	tables.Commands["forget"] = runFixed(func(s *home.State, c home.Command) home.Result {
		d := c.Disk
		one, found := d.Msg("m1")
		seen.add("m1: %s %v", one.Text, found)
		_, found = d.Msg("m2")
		_, known := d.Msg("m9")
		_, mixed := d.Note("m1")
		seen.add("m2 %v, m9 %v, a message as a note %v", found, known, mixed)
		note, found := d.Note("n1")
		seen.add("n1: %s %v", note.Text, found)
		seen.add("main %v, below 7 %v, one %v", ids(d.Thread(nil, 0, 10)), ids(d.Thread(nil, 7, 10)), ids(d.Thread(nil, 0, 1)))
		seen.add("thread %v, below 8 %v, another %v", ids(d.Thread(&thread, 0, 10)), ids(d.Thread(&thread, 8, 10)), len(d.Thread(&one.ID, 0, 10)))
		seen.add("pending %v", ids(d.Pending()))
		for _, item := range d.Queued() {
			seen.add("item %d for %s on %s, not before %d: %s", item.ID, item.AI, item.Host, item.NotBefore, item.Env)
		}
		return home.Result{}
	})
	_, p := runFounded(t, tables)
	p.does(groupID, "jump", ``)
	p.does(groupID, "push", ``)
	p.does(groupID, "forget", ``)
	equal(t, seen.all(), []string{
		"m1: one true",
		"m2 false, m9 false, a message as a note false",
		"n1: a note true",
		"main [c2@8 c1@7 m1@4], below 7 [m1@4], one [c2@8]",
		"thread [c2@8 m3@6], below 8 [m3@6], another 0",
		"pending [c1@7 c2@8]",
		fmt.Sprintf(`item 1 for %s on %s, not before 0: {"AI":%q,"Why":"notice","Task":"","Msg":"","Commit":"","PR":0,"Jump":false,"Code":"first","Detail":"","Cause":null,"NotBefore":0}`, aiID, laptop.ID(), aiID),
		fmt.Sprintf(`item 2 for %s on %s, not before %d: {"AI":%q,"Why":"notice","Task":"","Msg":"","Commit":"","PR":0,"Jump":false,"Code":"second","Detail":"","Cause":null,"NotBefore":%d}`, aiID, laptop.ID(), clock+5000, aiID, clock+5000),
	})
}
