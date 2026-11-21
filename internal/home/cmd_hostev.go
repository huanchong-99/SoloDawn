package home

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Member status and host events: member status updates an AI member row and
// writes an optional host event as a system message; HostOfflineEvent marks a
// disconnected host's AIs offline (planning/contracts/commands.md sections 1.2
// and 1.9; planning/contracts/messages.md section 2.11;
// planning/contracts/rows.md sections 2 and 4;
// planning/contracts/task-machine.md section 5).

// HostOfflineEvent is the name in Events under which a node reports that a
// host device's connection has closed; the event value is the device id as a
// string.
const HostOfflineEvent = "host.offline"

const (
	hevKindSystem     = "system"
	hevOffline        = "offline"
	hevTurnInterrupt  = "turn_interrupted"
	hevCapReached     = "cap_reached"
	hevRateLimited    = "rate_limited"
	hevLoginInvalid   = "login_invalid"
	hevSessionReplace = "session_replaced"
)

var (
	hevStates  = []string{"working", "waiting", "dormant", "takeover", "paused", "limited", hevOffline}
	hevPaths   = []string{"local", "direct", "relay", ""}
	hevReasons = []string{"crash", "timeout", "stop"}
	hevCaps    = []string{"day", "task"}

	hevEventReaders = map[string]func(Command, string) (hevEvent, *Refusal){
		hevTurnInterrupt:  hevTurnInterrupted,
		hevCapReached:     hevCap,
		hevRateLimited:    hevRate,
		hevLoginInvalid:   hevLogin,
		hevSessionReplace: hevReplaced,
	}
)

func init() {
	Commands.Add("member/status", hevStatus)
	Events.Add(HostOfflineEvent, hevHostOffline)
}

type hevEvent struct {
	code   string
	task   string
	notice bool
	body   any
}

// hevStatus updates an AI member's runtime state, task, path, usage and
// timestamps, and turns an attached host event into a system message and an
// optional secretary notice.
func hevStatus(s *State, c Command) (Result, *Refusal) {
	var p proto.MemberStatus[proto.SystemCode]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	ev, refusal := hevCheckPayload(c, p)
	if refusal != nil {
		return Result{}, refusal
	}
	ai, refusal := s.hevTargetAI(c.Who, string(p.ID))
	if refusal != nil {
		return Result{}, refusal
	}
	return s.hevWriteStatus(c, ai, p, ev)
}

func hevCheckPayload(c Command, p proto.MemberStatus[proto.SystemCode]) (hevEvent, *Refusal) {
	if !slices.Contains(hevStates, p.State) || !slices.Contains(hevPaths, p.Path) || !hevUsageOK(p.Usage) {
		return hevEvent{}, &Refusal{"bad_request", "member status has an invalid state, path or usage figure"}
	}
	if p.Event == nil {
		return hevEvent{}, nil
	}
	return hevReadEvent(c, p.Event.Code, string(p.ID))
}

func hevUsageOK(u proto.Usage) bool {
	floats := []*float64{u.WindowPct, u.DayCapPct, u.SpendToday, u.SpendCap}
	if slices.ContainsFunc(floats, hevNeg) {
		return false
	}
	return !hevNeg(u.WindowResets) && !hevNeg(u.TurnsToday)
}

func hevNeg[N int | int64 | float64](v *N) bool {
	return v != nil && *v < 0
}

func hevReadEvent(c Command, code, ai string) (hevEvent, *Refusal) {
	read, ok := hevEventReaders[code]
	if !ok {
		return hevEvent{}, &Refusal{"bad_request", "unknown host event code"}
	}
	return read(c, ai)
}

func hevDecode[E any](c Command) (E, *Refusal) {
	var p proto.MemberStatus[E]
	if refusal := c.Decode(&p); refusal != nil {
		var zero E
		return zero, refusal
	}
	return *p.Event, nil
}

func hevTurnInterrupted(c Command, ai string) (hevEvent, *Refusal) {
	ev, refusal := hevDecode[proto.TurnInterruptedEvent](c)
	if refusal != nil {
		return hevEvent{}, refusal
	}
	detail, ok := hevDetail(c.Env.Payload, ev.Detail)
	if !ok || !slices.Contains(hevReasons, ev.Reason) {
		return hevEvent{}, &Refusal{"bad_request", "turn_interrupted requires a valid reason and non-empty detail"}
	}
	task := string(ev.Task)
	body := proto.TurnInterruptedBody{
		SystemCode: ev.SystemCode, AI: ai, Reason: ev.Reason, Task: hevPtr(task), Detail: detail,
	}
	return hevEvent{code: hevTurnInterrupt, task: task, body: body}, nil
}

func hevCap(c Command, ai string) (hevEvent, *Refusal) {
	ev, refusal := hevDecode[proto.CapReachedEvent](c)
	if refusal != nil {
		return hevEvent{}, refusal
	}
	if !slices.Contains(hevCaps, ev.Cap) {
		return hevEvent{}, &Refusal{"bad_request", "cap_reached cap must be day or task"}
	}
	task := string(ev.Task)
	body := proto.CapReachedBody{SystemCode: ev.SystemCode, AI: ai, Cap: ev.Cap, Task: hevPtr(task)}
	return hevEvent{code: hevCapReached, task: task, notice: true, body: body}, nil
}

func hevRate(c Command, ai string) (hevEvent, *Refusal) {
	ev, refusal := hevDecode[proto.RateLimitedEvent](c)
	if refusal != nil {
		return hevEvent{}, refusal
	}
	if ev.Resets < 0 {
		return hevEvent{}, &Refusal{"bad_request", "rate_limited resets must be non-negative"}
	}
	body := proto.RateLimitedBody{SystemCode: ev.SystemCode, AI: ai, Resets: ev.Resets}
	return hevEvent{code: hevRateLimited, notice: true, body: body}, nil
}

func hevLogin(c Command, ai string) (hevEvent, *Refusal) {
	ev, refusal := hevDecode[proto.LoginInvalidEvent](c)
	if refusal != nil {
		return hevEvent{}, refusal
	}
	detail, ok := hevDetail(c.Env.Payload, ev.Detail)
	if !ok {
		return hevEvent{}, &Refusal{"bad_request", "login_invalid detail must not be empty"}
	}
	body := proto.LoginInvalidBody{SystemCode: ev.SystemCode, AI: ai, Detail: detail}
	return hevEvent{code: hevLoginInvalid, notice: true, body: body}, nil
}

func hevReplaced(c Command, ai string) (hevEvent, *Refusal) {
	ev, refusal := hevDecode[proto.SessionReplacedEvent](c)
	if refusal != nil {
		return hevEvent{}, refusal
	}
	task := string(ev.Task)
	body := proto.SessionReplacedBody{SystemCode: ev.SystemCode, AI: ai, Task: hevPtr(task)}
	return hevEvent{code: hevSessionReplace, task: task, body: body}, nil
}

func hevDetail(payload, detail string) (*string, bool) {
	if detail != "" {
		return &detail, true
	}
	return nil, !hevHasEventField(payload, "detail")
}

func hevHasEventField(payload, field string) bool {
	var outer struct {
		Event map[string]json.RawMessage `json:"event"`
	}
	proto.Decode([]byte(payload), &outer)
	_, there := outer.Event[field]
	return there
}

func hevPtr(val string) *string {
	if len(val) == 0 {
		return nil
	}
	return &val
}

func (s *State) hevTargetAI(w Who, id string) (proto.AIMemberRow, *Refusal) {
	ai, ok := s.AIs[id]
	if !ok {
		if _, human := s.People[id]; human {
			return ai, &Refusal{"forbidden", "people have no host status"}
		}
		return ai, &Refusal{"not_found", "the group has no such member"}
	}
	if !s.Hosts(w, id) {
		return ai, &Refusal{"forbidden", "only the host of the AI reports its status"}
	}
	return ai, nil
}

func (s *State) hevWriteStatus(c Command, ai proto.AIMemberRow, p proto.MemberStatus[proto.SystemCode], ev hevEvent) (Result, *Refusal) {
	if p.Task != "" && !s.hevHasTask(string(p.Task)) {
		return Result{}, &Refusal{"not_found", "member status names no task of the group"}
	}
	ai = hevUpdatedAI(ai, p, c.Now)
	if ev.code == "" {
		return Result{Rows: []Row{RowOf(ai)}}, nil
	}
	return s.hevWriteEvent(c, ai, ev)
}

func (s *State) hevHasTask(id string) bool {
	_, ok := s.Tasks[id]
	return ok
}

func hevUpdatedAI(ai proto.AIMemberRow, p proto.MemberStatus[proto.SystemCode], now int64) proto.AIMemberRow {
	if ai.State != p.State {
		ai.State, ai.Since = p.State, now
	}
	ai.Task = string(p.Task)
	ai.Path = p.Path
	ai.Usage = p.Usage
	ai.LastSeen = now
	return ai
}

func (s *State) hevWriteEvent(c Command, ai proto.AIMemberRow, ev hevEvent) (Result, *Refusal) {
	if ev.task != "" && !s.hevHasTask(ev.task) {
		return Result{}, &Refusal{"not_found", "host event names no task of the group"}
	}
	id := string(c.Nonce)
	if _, taken := c.Disk.Msg(id); taken {
		return Result{}, &Refusal{"conflict", "a message with the id of the host event exists"}
	}
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id}, Group: s.ID, Thread: hevPtr(ev.task),
		By: hevKindSystem, Kind: hevKindSystem, Timestamp: c.Now, Mentions: []string{}, Body: briefJSON(ev.body),
	}
	rows := s.hevEventRows(ai, msg, ev.task, c.Now)
	return Result{Rows: rows, Items: s.hevNotice(ev, id)}, nil
}

func (s *State) hevEventRows(ai proto.AIMemberRow, msg proto.MsgRow, taskID string, now int64) []Row {
	rows := []Row{RowOf(ai), RowOf(msg)}
	if taskID == "" {
		return rows
	}
	task := s.Tasks[taskID]
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{By: hevKindSystem, Kind: hevKindSystem, Text: "", Timestamp: now}
	return append(rows, RowOf(task))
}

func (s *State) hevNotice(ev hevEvent, msgID string) []Item {
	if !ev.notice || s.Group.Secretary == "" {
		return nil
	}
	return []Item{{AI: s.Group.Secretary, Why: itemNotice, Task: ev.task, Msg: msgID, Code: ev.code}}
}

// hevHostOffline marks every non-offline AI hosted on a disconnected device as
// offline and clears its active task once the device has no open connection.
func hevHostOffline(s *State, _ Disk, now int64, event any) Result {
	device, _ := event.(string)
	if s.Online[device] {
		return Result{}
	}
	var rows []Row
	for _, id := range slices.Sorted(maps.Keys(s.AIs)) {
		ai := s.AIs[id]
		if ai.Host == device && ai.State != hevOffline {
			ai.State, ai.Task, ai.Since = hevOffline, "", now
			rows = append(rows, RowOf(ai))
		}
	}
	return Result{Rows: rows}
}
