package host

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// The events of planning/contracts/diagnostics.md section 2 that the host
// writes. Every line goes through Host.diag, a logger with the node's id. A
// line holds ids, counts and states, never the text of a message or a turn.

func init() { Register(Listeners, "diag", diagTurn) }

func diagDevice(id string) string { return id[:min(len(id), 8)] }

// bg starts f with start (a wait group's Go, or a go statement). The
// goroutine writes bg.start, because the caller may hold a mutex, and bg.stop
// when f returns, before the wait group's Done, so that Settle returns after
// it.
func (h *Host) bg(start func(func()), name string, f func(), more ...any) {
	attrs := append([]any{"name", name}, more...)
	start(func() {
		h.diag.Info("bg.start", attrs...)
		defer h.diag.Info("bg.stop", attrs...)
		f()
	})
}

func diagSession(key SessionKey, more ...any) []any {
	return append([]any{"g", key.Group, "ai", key.Member, "task", key.Task}, more...)
}

func diagTurn(h *Host, key SessionKey, e Event) {
	switch e.Kind {
	case TurnBegan:
		h.diag.Info("turn.begin", diagSession(key)...)
	case TurnEnded:
		h.diag.Info("turn.end", diagSession(key, "status", e.Status)...)
	}
}

// diagItems is the item numbers of items, comma separated.
func diagItems(items []verifiedItem) string {
	nums := make([]string, len(items))
	for i, v := range items {
		nums[i] = strconv.FormatInt(v.item.Item, 10)
	}
	return strings.Join(nums, ",")
}

// diagSpan is the sequence numbers of the first and the last of rows.
func diagSpan(rows []json.RawMessage) string {
	if len(rows) == 0 {
		return ""
	}
	first, _ := decodeRowHeader(rows[0])
	last, _ := decodeRowHeader(rows[len(rows)-1])
	return fmt.Sprintf("%d-%d", first.Seq, last.Seq)
}

// diagCode is the code a tool call ended with, as toolFailureResult sends it.
func diagCode(err error) string {
	var hostErr *Error
	if errors.As(err, &hostErr) {
		return hostErr.Code
	}
	if err != nil {
		return "internal"
	}
	return "ok"
}

// diagExitWhy is the runtime's reason for the end of a CLI process: the
// interrupt it claimed, or the event that ended the process.
func diagExitWhy(rt *runtimePart, lp *liveProc) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return cmp.Or(lp.interruptWhy, string(lp.exitWhy))
}
