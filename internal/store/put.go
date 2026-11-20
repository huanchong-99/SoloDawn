package store

import (
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type wireRow interface {
	proto.GroupRow | proto.HumanMemberRow | proto.AIMemberRow | proto.DeviceRow |
		proto.TaskRow | proto.MsgRow | proto.NoteRow | proto.GoneRow
}

// Put stores a wire row with its assigned seq. The index columns are derived
// from the same JSON readers receive. Updates retain the original pos and ts.
func Put[R wireRow](tx *Tx, group string, modified int64, row R) error {
	data, err := json.Marshal(forStorage(row))
	if err != nil {
		return err
	}
	_, err = tx.Exec(putRow, group, modified, string(data))
	return err
}

func list(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func forStorage[R wireRow](row R) any {
	switch v := any(row).(type) {
	case proto.GroupRow:
		v.Checks = list(v.Checks)
		return v
	case proto.AIMemberRow:
		v.Allow, v.Approvers = list(v.Allow), list(v.Approvers)
		return v
	case proto.TaskRow:
		v.Depends = list(v.Depends)
		return v
	case proto.MsgRow:
		v.Mentions = list(v.Mentions)
		if v.Body == nil {
			v.Body = json.RawMessage("{}")
		}
		return v
	default:
		return row
	}
}

const putRow = `
WITH input(d) AS (VALUES (?3))
INSERT INTO row(g,r,id,seq,pos,ts,mt,thread,in_main,pending,final,keep,gone,data)
SELECT ?1, d->>'r', d->>'id', d->>'seq', coalesce(d->>'pos', d->>'seq'),
       coalesce(d->>'ts', d->>'created', ?2), ?2, CASE d->>'r' WHEN 'msg' THEN d->>'thread' END,
       d->>'r'='msg' AND (d->>'thread' IS NULL OR d->>'main'=1) AND d->>'gone' IS NULL,
       d->>'$.state.status' IS 'pending',
       d->>'r'='task' AND (d->>'state' IS 'merged' OR d->>'state' IS 'cancelled'),
       d->>'gone' IS NULL AND (d->>'r'='note' OR (d->>'r'='msg' AND d->>'kind' IS 'review')),
       coalesce(d->>'gone',0), d
FROM input WHERE true
ON CONFLICT(g,r,id) DO UPDATE SET seq=excluded.seq, mt=excluded.mt,
 thread=excluded.thread, in_main=excluded.in_main, pending=excluded.pending,
 final=excluded.final, keep=excluded.keep, gone=excluded.gone, data=excluded.data`
