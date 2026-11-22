package store

import (
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestRetentionKeepsRoleRequestsAndGrantsUntilFinal(t *testing.T) {
	s := openDB(t, ":memory:")
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -10).UnixMilli()
	for _, state := range []string{"queued", "active", "paused", "failed"} {
		group := state
		_, err := s.db.Exec("INSERT INTO grp VALUES (?,0,0,0)", group)
		check(t, err)
		for i, kind := range []string{"brief", "task", "confirm", "confirm"} {
			id := []string{"brief", "direct", "brief-grant", "direct-grant"}[i]
			row := msg(id, int64(i+1), nil, true, kind)
			row.Timestamp = old
			row.State = &proto.CardState{Status: "confirmed"}
			if kind == "confirm" {
				req := []string{"brief", "direct"}[i-2]
				row.Re = &req
				row.Body = []byte(`{"what":"direct","ai":"a","to":["owner"]}`)
			}
			put(t, s, group, old, row)
		}
		for i, id := range []string{"from-brief", "direct"} {
			task := proto.TaskRow{RowHeader: header("task", id, int64(i+5)), Group: group, State: state, Created: old}
			if i == 0 {
				task.Brief = "brief"
			}
			put(t, s, group, old, task)
		}
		check(t, s.Write(func(tx *Tx) error { return tx.Retain(now, 2) }))
		equal(t, scalar[int](t, s.db, "SELECT count(*) FROM row WHERE g=? AND r='msg'", group), 4)
		for i, id := range []string{"from-brief", "direct"} {
			put(t, s, group, old, proto.TaskRow{RowHeader: header("task", id, int64(i+7)), Group: group, State: "cancelled", Brief: "brief", Created: old})
		}
		check(t, s.Write(func(tx *Tx) error { return tx.Retain(now, 2) }))
		equal(t, scalar[int](t, s.db, "SELECT count(*) FROM row WHERE g=?", group), 0)
	}
}
