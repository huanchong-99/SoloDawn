package home

import (
	"fmt"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The events of planning/contracts/diagnostics.md that the home writes. Every
// line goes through Loop.diag, a logger with the node's id. A line holds ids,
// counts and states, never the text of a message.

func diagDevice(id string) string { return id[:min(len(id), 8)] }

// diagBG writes bg.start and returns the function that writes bg.stop; the
// goroutine defers it after its wait group's Done, so Close sees the stop.
func (l *Loop) diagBG(name string, more ...any) func() {
	attrs := append([]any{"name", name}, more...)
	l.diag.Info("bg.start", attrs...)
	return func() { l.diag.Info("bg.stop", attrs...) }
}

// diagReply writes cmd.reply for the command in env that was answered with
// code, "" for a success. t is the key of the command table.
func (l *Loop) diagReply(c *Conn, group string, re int64, env proto.Envelope, code string) {
	var head struct {
		Type string `json:"t"`
	}
	proto.Decode([]byte(env.Payload), &head) // a payload that does not decode has no t
	key, _ := Command{PayloadHeader: proto.PayloadHeader{Type: head.Type}, Env: env}.key()
	if code == "" {
		code = "ok"
	}
	l.diag.Info("cmd.reply", "g", group, "dev", diagDevice(c.device), "re", re, "t", key, "code", code)
}

// diagSpan is the first and the last sequence number of rows.
func diagSpan(rows []Row) string {
	return fmt.Sprintf("%d-%d", runHeader(rows[0]).Seq, runHeader(rows[len(rows)-1]).Seq)
}
