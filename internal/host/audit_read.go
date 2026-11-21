package host

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	audMaxLimit   = 100
	audSelectCols = `SELECT id, ts, coalesce(g, ''), coalesce(agent, ''), what, detail, coalesce(by, ''), coalesce(result, '') FROM audit`
)

type audResult struct {
	Rows []proto.AuditRow `json:"rows"`
}

func init() {
	Register(Surface, "audit", audHandle)
}

func audHandle(_ context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	req, err := audDecode(data)
	if err != nil {
		return nil, err
	}
	rows, err := audReadPage(h.Store, req.Before, req.Limit)
	if err != nil {
		return nil, err
	}
	return audResult{Rows: rows}, nil
}

func audDecode(data json.RawMessage) (proto.Audit, error) {
	var req proto.Audit
	if err := proto.Decode(data, &req); err != nil {
		return proto.Audit{}, &Error{Code: "bad_request", Message: "invalid audit request"}
	}
	if req.Type != "audit" {
		return proto.Audit{}, &Error{Code: "bad_request", Message: "unexpected request type"}
	}
	return req, audCheckBounds(req.Before, req.Limit)
}

func audCheckBounds(before int64, limit int) error {
	if before < 0 {
		return &Error{Code: "bad_request", Message: "before must not be negative"}
	}
	if limit < 1 || limit > audMaxLimit {
		return &Error{Code: "bad_request", Message: "limit must be between 1 and 100"}
	}
	return nil
}

func audReadPage(db *store.Store, before int64, limit int) ([]proto.AuditRow, error) {
	rows := []proto.AuditRow{}
	read := func(tx *store.Tx) error {
		q, err := audQuery(tx, before, limit)
		if err != nil {
			return err
		}
		defer q.Close()
		for q.Next() {
			rows = append(rows, audScanRow(q))
		}
		return q.Err()
	}
	return rows, db.Write(read)
}

func audQuery(tx *store.Tx, before int64, limit int) (*sql.Rows, error) {
	if before == 0 {
		return tx.Query(audSelectCols+` ORDER BY id DESC LIMIT ?`, limit)
	}
	return tx.Query(audSelectCols+` WHERE id < ? ORDER BY id DESC LIMIT ?`, before, limit)
}

// audScanRow reads one row from a query on audit, whose STRICT schema fixes
// every column's type before Next returns true.
func audScanRow(q *sql.Rows) proto.AuditRow {
	var row proto.AuditRow
	var group string
	q.Scan(&row.ID, &row.TS, &group, &row.AI, &row.What, &row.Detail, &row.By, &row.Result)
	row.Group = proto.ID(group)
	return row
}
