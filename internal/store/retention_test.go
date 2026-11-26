package store

import (
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestRetention(t *testing.T) {
	s := openDB(t, ":memory:")
	now := time.Date(2026, 5, 6, 1, 0, 0, 0, time.FixedZone("local", 9*3600))
	cut := now.AddDate(0, 0, -2).UnixMilli()
	old := cut - 1
	exec := func(q string, args ...any) { t.Helper(); _, err := s.db.Exec(q, args...); check(t, err) }
	exec("INSERT INTO grp VALUES ('g',30,0,0),('other',40,0,39),('third',50,0,0)")
	for _, x := range []struct {
		id, kind, status string
		ts               int64
		seq              int64
	}{
		{"old", "text", "", old, 1}, {"boundary", "text", "", cut, 2}, {"ledger", "review", "", old, 3},
		{"pending", "decision", "pending", old, 4}, {"closed", "decision", "decided", old, 5},
		{"confirmed", "brief", "confirmed", old, 6}, {"unneeded", "brief", "confirmed", old, 7},
	} {
		r := msg(x.id, x.seq, nil, true, x.kind)
		r.Timestamp = x.ts
		if x.status != "" {
			r.State = &proto.CardState{Status: x.status}
		}
		put(t, s, "g", cut, r)
	}
	for i, x := range []struct {
		id, state, brief string
		mt               int64
	}{
		{"done", "done", "", old}, {"cancel", "cancelled", "", old}, {"just-done", "done", "", cut},
		{"active", "active", "", old}, {"queue", "queued", "confirmed", old},
	} {
		put(t, s, "g", x.mt, proto.TaskRow{RowHeader: header("task", x.id, int64(8+i)), Group: "g", State: x.state, Brief: x.brief, Created: old})
	}
	put(t, s, "other", old, proto.TaskRow{RowHeader: header("task", "other-queue", 1), Group: "other", State: "queued", Brief: "unneeded"})
	put(t, s, "g", old, proto.NoteRow{RowHeader: header("note", "memory", 13), Group: "g", Timestamp: old})
	put(t, s, "g", old, msg("recent-gone", 14, nil, true, "text"))
	for _, x := range []struct {
		g, r, id string
		seq, mt  int64
	}{
		{"g", "note", "old-marker", 18, old}, {"g", "msg", "top-marker", 20, old},
		{"g", "msg", "recent-gone", 22, cut}, {"other", "member", "old-marker", 38, old},
		{"third", "device", "old-marker", 49, old},
	} {
		put(t, s, x.g, x.mt, proto.GoneRow{RowHeader: header(x.r, x.id, x.seq), Gone: true})
	}
	exec("INSERT INTO inbox(g,ai,host,why,env,created) VALUES ('g','ai','h','notice','{}',?),('g','ai','h','notice','{}',?)", old, cut)
	exec("INSERT INTO audit(ts,what,detail) VALUES (?,'old',''),(?,'boundary','')", old, cut)
	exec("INSERT INTO usage VALUES ('ai','2026-05-03','{}'),('ai','2026-05-04','{}')")
	exec("INSERT INTO peer VALUES ('d','old',?),('d','boundary',?)", old, cut)
	receiptCut := now.UnixMilli() - 86_400_000
	exec("INSERT INTO receipt VALUES ('k','old',?,'{}'),('k','boundary',?,'{}')", receiptCut-1, receiptCut)

	retain := func() { check(t, s.Write(func(tx *Tx) error { return tx.Retain(now, 2) })) }
	retain()
	equal(t, scalar[string](t, s.db, "SELECT group_concat(id,',' ORDER BY seq) FROM row WHERE g='g'"), "boundary,ledger,pending,confirmed,just-done,active,queue,memory,recent-gone")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(g||':'||purged,',' ORDER BY g) FROM grp"), "g:20,other:39,third:49")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(id) FROM inbox"), "2")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(what) FROM audit"), "boundary")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(day) FROM usage"), "2026-05-04")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(url) FROM peer"), "boundary")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(n) FROM receipt"), "boundary")
	put(t, s, "g", cut, proto.TaskRow{RowHeader: header("task", "queue", 30), Group: "g", State: "active", Brief: "confirmed", Created: old})
	retain()
	equal(t, scalar[int](t, s.db, "SELECT count(*) FROM row WHERE g='g' AND id='confirmed'"), 1)
	put(t, s, "g", cut, proto.TaskRow{RowHeader: header("task", "queue", 31), Group: "g", State: "cancelled", Brief: "confirmed", Created: old})
	retain()
	equal(t, scalar[int](t, s.db, "SELECT count(*) FROM row WHERE g='g' AND id='confirmed'"), 0)
	v := snapshot(t, s, "g")
	equal(t, v.Seq, int64(30))
	equal(t, v.Purged, int64(20))
}

func TestRetentionKeepsAFinishedTaskThatAnUnfinishedOneDependsOn(t *testing.T) {
	s := openDB(t, ":memory:")
	now := time.Date(2026, 5, 6, 1, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -2).UnixMilli() - 1
	task := func(id, state string, seq int64, depends ...string) {
		put(t, s, "g", old, proto.TaskRow{RowHeader: header("task", id, seq), Group: "g", State: state, Depends: depends})
	}
	task("done", "done", 1)
	task("cancelled", "cancelled", 2)
	task("lone", "done", 3)
	task("waits", "queued", 4, "done", "cancelled")
	left := func() string {
		return scalar[string](t, s.db, "SELECT coalesce(group_concat(id,',' ORDER BY seq),'') FROM row WHERE g='g'")
	}
	retain := func() { check(t, s.Write(func(tx *Tx) error { return tx.Retain(now, 2) })) }

	retain()
	equal(t, left(), "done,cancelled,waits")
	task("waits", "done", 5, "done", "cancelled")
	retain()
	equal(t, left(), "")
}
