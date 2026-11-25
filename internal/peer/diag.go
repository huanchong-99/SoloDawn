package peer

import (
	"context"
	"log/slog"
	"time"
)

type linkDiag struct {
	logger *slog.Logger
	key    link
}

func diagDevice(id string) string { return id[:min(len(id), 8)] }

func (d linkDiag) event(level slog.Level, event string, fields ...any) {
	d.logger.Log(context.Background(), level, event, append([]any{"dev", diagDevice(d.key.device), "conn", d.key.conn}, fields...)...)
}

func (d linkDiag) signal(event, kind string) {
	switch kind {
	case "offer", "answer":
		d.event(slog.LevelInfo, event, "kind", kind)
	case "ice":
		d.event(slog.LevelDebug, event, "kind", kind)
	}
}

func (c *Connection) diagClose(why string) {
	var ms int64
	select {
	case <-c.ready:
		ms = time.Since(c.opened).Milliseconds()
	default:
	}
	c.diag.event(slog.LevelInfo, "conn.close", "why", why, "ms", ms)
}
