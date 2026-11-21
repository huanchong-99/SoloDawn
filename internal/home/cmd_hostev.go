package home

import (
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

// HostOfflineEvent is the name in Events under which Loop marks a disconnected
// host device's AIs offline; the event value is the device id as a string.
const HostOfflineEvent = "host.offline"

const (
	hevKindSystem     = "system"
	hevOffline        = "offline"
	hevTurnInterrupt  = "turn_interrupted"
	hevLoginInvalid   = "login_invalid"
	hevSessionReplace = "session_replaced"
)

var hevEventReaders = map[string]func(Command, string) (hevEvent, *Refusal){
	hevTurnInterrupt:  hevReader(hevTurnInterrupted),
	hevLoginInvalid:   hevReader(hevLogin),
	hevSessionReplace: hevReader(hevReplaced),
}

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
	ev, refusal := hevReadEvent(c, p.Event, string(p.ID))
	if refusal != nil {
		return Result{}, refusal
	}
	ai, refusal := s.hevTargetAI(c.Who, string(p.ID))
	if refusal != nil {
		return Result{}, refusal
	}
	return s.hevWriteStatus(c, ai, p, ev)
}

func hevReadEvent(c Command, raw *proto.SystemCode, ai string) (hevEvent, *Refusal) {
	if raw == nil {
		return hevEvent{}, nil
	}
	read, ok := hevEventReaders[raw.Code]
	if !ok {
		return hevEvent{}, &Refusal{"bad_request", "unknown host event code"}
	}
	return read(c, ai)
}

func hevReader[E any](build func(E, string) hevEvent) func(Command, string) (hevEvent, *Refusal) {
	return func(c Command, ai string) (hevEvent, *Refusal) {
		var p proto.MemberStatus[E]
		if refusal := c.Decode(&p); refusal != nil {
			return hevEvent{}, refusal
		}
		return build(*p.Event, ai), nil
	}
}

func hevTurnInterrupted(ev proto.TurnInterruptedEvent, ai string) hevEvent {
	task := string(ev.Task)
	body := proto.TurnInterruptedBody{
		SystemCode: ev.SystemCode, AI: ai, Reason: ev.Reason, Task: hevPtr(task), Detail: hevPtr(ev.Detail),
	}
	return hevEvent{code: hevTurnInterrupt, task: task, body: body}
}

func hevLogin(ev proto.LoginInvalidEvent, ai string) hevEvent {
	body := proto.LoginInvalidBody{SystemCode: ev.SystemCode, AI: ai, Detail: hevPtr(ev.Detail)}
	return hevEvent{code: hevLoginInvalid, notice: true, body: body}
}

func hevReplaced(ev proto.SessionReplacedEvent, ai string) hevEvent {
	task := string(ev.Task)
	body := proto.SessionReplacedBody{SystemCode: ev.SystemCode, AI: ai, Task: hevPtr(task)}
	return hevEvent{code: hevSessionReplace, task: task, body: body}
}

func hevPtr(val string) *string {
	if val == "" {
		return nil
	}
	return &val
}

func (s *State) hevTargetAI(w Who, id string) (proto.AIMemberRow, *Refusal) {
	ai, ok := s.AIs[id]
	if !ok {
		return ai, &Refusal{"not_found", "the group has no such AI member"}
	}
	if !s.Hosts(w, id) {
		return ai, &Refusal{"forbidden", "only the host of the AI reports its status"}
	}
	return ai, nil
}

func (s *State) hevWriteStatus(c Command, ai proto.AIMemberRow, p proto.MemberStatus[proto.SystemCode], ev hevEvent) (Result, *Refusal) {
	ai = hevUpdatedAI(ai, p, c.Now)
	if ev.code == "" {
		return Result{Rows: []Row{RowOf(ai)}}, nil
	}
	if ev.task != "" && !s.hevHasTask(ev.task) {
		return Result{}, &Refusal{"not_found", "host event names no task of the group"}
	}
	id := string(c.Nonce)
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id}, Group: s.ID, Thread: hevPtr(ev.task),
		By: hevKindSystem, Kind: hevKindSystem, Timestamp: c.Now, Mentions: []string{}, Body: briefJSON(ev.body),
	}
	return Result{Rows: s.hevEventRows(ai, msg, ev.task, c.Now), Items: s.hevNotice(ev, id)}, nil
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
	return []Item{{AI: s.Group.Secretary, Why: itemNotice, Msg: msgID, Code: ev.code}}
}

// hevHostOffline marks every non-offline AI hosted on a disconnected device as
// offline and clears its active task.
func hevHostOffline(s *State, _ Disk, now int64, event any) Result {
	device, _ := event.(string)
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
