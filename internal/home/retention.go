package home

import (
	"time"

	"github.com/huanchong-99/SoloDawn/internal/store"
)

const retSettingSQL = "SELECT coalesce((SELECT value FROM setting WHERE key='retention_days'), '90')"

// Retain reads retention_days from setting (90 when absent) and expires old
// store data at now, in Unix milliseconds (planning/contracts/storage.md
// sections 2 and 4).
func Retain(tx *store.Tx, now int64) error {
	var days int
	if err := tx.QueryRow(retSettingSQL).Scan(&days); err != nil {
		return err
	}
	return tx.Retain(time.UnixMilli(now), days)
}
