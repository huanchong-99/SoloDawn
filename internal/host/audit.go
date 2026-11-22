package host

import "github.com/huanchong-99/SoloDawn/internal/store"

// What an audit row records (contracts/storage.md section 3).
const (
	AuditDelivered = "delivered" // an inbox item was delivered
	AuditRefused   = "refused"   // an inbox item was refused
	AuditRequested = "requested" // an approval card was posted
	AuditApproval  = "approval"  // an approval request was answered
	AuditTakeover  = "takeover"
	AuditHandback  = "handback"
	AuditMerge     = "merge"
	AuditSetting   = "setting"
	AuditKilled    = "killed" // an idle CLI did not exit within 10 s of closing its input
)

// An AuditRow is one row of the node's audit record (R-139). A field that
// does not apply is left at its zero value and stored as NULL; Detail is
// always stored.
type AuditRow struct {
	Group  string
	Agent  string // the CLI: claude or codex
	What   string // one of the Audit constants
	Item   int64  // delivered, refused: the id of the inbox item
	Home   string // delivered, refused: the device id of the home
	Detail string
	By     string // the device id that caused it
	Result string
}

// Audit writes a row inside tx, so that it commits with what it records. The
// record holds an inbox item of a home once: for an item that is there
// already, written is false and nothing is written, and the caller
// acknowledges the item again without delivering it again
// (contracts/storage.md section 3, "Delivered once").
func (h *Host) Audit(tx *store.Tx, row AuditRow) (written bool, err error) {
	result, err := tx.Exec(`INSERT INTO audit(ts, g, agent, what, item, home, detail, by, result)
		VALUES(?, nullif(?, ''), nullif(?, ''), ?, nullif(?, 0), nullif(?, ''), ?, nullif(?, ''), nullif(?, ''))
		ON CONFLICT DO NOTHING`,
		h.Now().UnixMilli(), row.Group, row.Agent, row.What, row.Item, row.Home, row.Detail, row.By, row.Result)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
