package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	retentionSettingKey  = "retention_days"
	defaultRetentionDays = 90
)

var retentionEvery = 24 * time.Hour

func (n *nodeRuntime) startRetention() {
	n.spawn(func() { runRetention(n.ctx, n.db, retentionEvery, time.Now, logRetentionError) })
}

func logRetentionError(err error) {
	log.Printf("node: retention: %v", err)
}

func runRetention(ctx context.Context, db *store.Store, every time.Duration, now func() time.Time, report func(error)) {
	retainAndReport(db, now(), report)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			retainAndReport(db, now(), report)
		}
	}
}

func retainAndReport(db *store.Store, now time.Time, report func(error)) {
	if err := retainOnce(db, now); err != nil {
		report(err)
	}
}

func retainOnce(db *store.Store, now time.Time) error {
	run := func(tx *store.Tx) error {
		days, err := readRetentionDays(tx)
		if err != nil {
			return err
		}
		return tx.Retain(now, days)
	}
	return db.Write(run)
}

func readRetentionDays(tx *store.Tx) (int, error) {
	var raw string
	err := tx.QueryRow("SELECT value FROM setting WHERE key = ?", retentionSettingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultRetentionDays, nil
	}
	if err != nil {
		return 0, err
	}
	return parseRetentionDays(raw)
}

func parseRetentionDays(raw string) (int, error) {
	var days int
	if err := proto.Decode([]byte(raw), &days); err != nil {
		return 0, fmt.Errorf("setting %s: %w", retentionSettingKey, err)
	}
	if days < 1 {
		return 0, fmt.Errorf("setting %s must be at least 1, got %d", retentionSettingKey, days)
	}
	return days, nil
}
