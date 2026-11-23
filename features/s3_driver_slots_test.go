//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"maps"
	"sync"

	"github.com/huanchong-99/SoloDawn/internal/host"
)

func (w *s3DriverWorld) slotGate() func() {
	release := make(chan struct{})
	closeGate := sync.OnceFunc(func() { close(release) })
	saved := host.Tools
	host.Tools = maps.Clone(saved)
	host.Tools["s3_gate"] = host.Tool{Description: "Wait for the scripted turn to finish", Schema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Call: func(context.Context, *host.Host, host.Caller, json.RawMessage) (any, error) {
			<-release
			return struct{}{}, nil
		}}
	w.cleanup(func() { host.Tools = saved })
	w.cleanup(closeGate)
	return closeGate
}

func (w *s3DriverWorld) workingSlot(release func()) error {
	m := w.caps.m3
	return s3Run(
		func() error { return w.script(true, false, s3PostTurn(1, "After second.")) },
		func() error { return m.whenOwnerPosts("@Second hold the slot", "Website") },
		func() error { return m.thenAIPosts("Second", "Second working.", "Website") }, w.mention,
		func() error { return m.thenSecondProcessHeld("Worker", "Second") },
		func() error { release(); return nil },
		func() error { return m.thenAIPosts("Worker", "After second.", "Website") },
		func() error {
			first := m.eventIndex(func(e homeViewEvent) bool { return e.kind == "msg" && e.text == "Second working." })
			last := m.eventIndex(func(e homeViewEvent) bool { return e.kind == "msg" && e.text == "After second." })
			return checkOrderedIndices(first, last, "working AI must finish before another starts")
		})
}
