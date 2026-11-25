package node

import (
	"context"
	"log"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func (n *nodeRuntime) startRetention() {
	t := time.NewTicker(24 * time.Hour)
	n.spawn("retention", func() {
		defer t.Stop()
		runRetention(n.ctx, n.db, time.Now(), t.C)
	})
}

func runRetention(ctx context.Context, db *store.Store, now time.Time, tick <-chan time.Time) {
	for {
		if err := db.Write(func(tx *store.Tx) error { return home.Retain(tx, now.UnixMilli()) }); err != nil {
			log.Printf("node: retention: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case now = <-tick:
		}
	}
}
