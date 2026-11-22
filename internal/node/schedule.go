package node

import (
	"context"
	"log"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/store"
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
	nowMS := now.UnixMilli()
	run := func(tx *store.Tx) error { return home.Retain(tx, nowMS) }
	return db.Write(run)
}
