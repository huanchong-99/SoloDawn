package host

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	Register(Tools, "next_item", Tool{
		Description: "Take one item of work when a person asks, or none; never call it in a loop.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		For:         onlyExternal,
		Call:        extNextItem,
	})
	Register(Tools, "done", Tool{
		Description: "End the run with this number.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"run":{"type":"integer"}},"required":["run"],"additionalProperties":false}`),
		For:         onlyExternal,
		Call:        extDoneTool,
	})
	Register(Tools, "report_usage", Tool{
		Description: "Report how much of your quota window is used, and when it resets (Unix ms).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"window_pct":{"type":"number","minimum":0,"maximum":100},"window_resets":{"type":"integer"}},"required":["window_pct"],"additionalProperties":false}`),
		For:         onlyExternal,
		Call:        extReportUsage,
	})
}

func onlyExternal(c Caller) bool { return c.External }

var externalKinds = []Kind{GroupSession, TaskSession, ReviewSession}

// externalCaller is the caller an external AI's member id stands for: the
// session and turn of its open run, or its group session without one, at level 3
// (contracts/step-3/external.md section 5).
func externalCaller(h *Host, member string) (Caller, bool) {
	group, ok := h.ExternalMember(member)
	if !ok {
		return Caller{}, false
	}
	key, turn := SessionKey{Group: group, Member: member}, Turn{}
	ep := extOf(h)
	ep.mu.Lock()
	if r := ep.runs[aiGroupKey{group: group, member: member}]; r != nil {
		key, turn = r.key, r.turn
	}
	ep.mu.Unlock()
	s, _ := h.Sessions.Get(key)
	return Caller{Session: s, Level: 3, External: true, Turn: turn}, true
}

// neededKinds returns the kinds of run in which an external caller may call
// a tool listed with this For.
func neededKinds(c Caller, listed func(Caller) bool) []string {
	var kinds []string
	for _, k := range externalKinds {
		c.Kind = k
		if listed(c) {
			kinds = append(kinds, string(k))
		}
	}
	return kinds
}

func refuseForRun(name string, kinds []string) *Error {
	return &Error{Code: "conflict", Message: fmt.Sprintf("%s needs a %s run", name, strings.Join(kinds, " or "))}
}

func extNextItem(ctx context.Context, h *Host, c Caller, _ json.RawMessage) (any, error) {
	ho, ok, err := h.ExternalWait(ctx, c.Member, new(time.Duration(0)))
	if !ok {
		return map[string]bool{"none": true}, err
	}
	return ho, err
}

func extDoneTool(_ context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a struct {
		Run int64 `json:"run"`
	}
	json.Unmarshal(raw, &a)
	h.ExternalDone(c.Group, c.Member, a.Run, "")
	return map[string]bool{"ok": true}, nil
}

func extReportUsage(_ context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var u proto.Usage
	json.Unmarshal(raw, &u)
	key := aiGroupKey{group: c.Group, member: c.Member}
	ep := extOf(h)
	ep.mu.Lock()
	ep.usage[key] = u
	ep.mu.Unlock()
	extReport(h, key)
	return map[string]bool{"ok": true}, nil
}
