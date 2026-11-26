package host

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Terminal takeover (planning/flows/takeover.md): takeover.start picks the
// session (section A) and switches it to the interactive CLI in a
// pseudo-terminal (section B); the terminal closing for any reason hands it
// back (section D). A takeover holds the AI's deliveries in its group from
// takeover.start until its hand-back starts.

// takeoverAttach is how long an open terminal waits for the browser's pty
// channel before it hands back (flows/takeover.md step 7).
var takeoverAttach = 60 * time.Second

func init() {
	Register(Surface, "takeover.start", takeoverStart)
	Register(Surface, "takeover.end", takeoverEnd)
	Register(Holds, "takeover", takeoverHold)
	Register(Settled, "takeover", settleTakeovers)
}

// takeovers is kept via Part[takeovers](h). mu guards its fields and those of
// each takeover, and is never held across a call out of this file but the
// one to givingTurn in claim.
type takeovers struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	open    map[aiSlot]*takeover
	watched map[Owner]bool
	ctx     context.Context // ended when the host settles
	cancel  context.CancelFunc
	settled bool
}

// A takeover of one AI in one group, from takeover.start to the end of its
// hand-back.
type takeover struct {
	key    SessionKey
	from   Owner
	nonce  string
	assign bool // the CLI cannot start a new session in a terminal
	pty    *PTY
	closed bool // the terminal has closed
	posted bool // the AI posted its hand-back
	free   bool // the hand-back has started: the takeover holds nothing
}

func takeoverState(h *Host) *takeovers {
	p := Part[takeovers](h)
	p.mu.Lock()
	if p.open == nil {
		p.open, p.watched = map[aiSlot]*takeover{}, map[Owner]bool{}
		p.ctx, p.cancel = context.WithCancel(context.Background())
	}
	p.mu.Unlock()
	return p
}

// claim registers t for slot when no takeover is registered there. A
// takeover.start may also replace one that has started its hand-back, and is
// refused while delivery may be giving the AI a turn. After the claim delivery
// reads the hold.
func (p *takeovers) claim(h *Host, slot aiSlot, t *takeover, start bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if old := p.open[slot]; old != nil && !(start && old.free) || start && givingTurn(h, aiGroupKey(slot)) {
		return false
	}
	p.open[slot] = t
	return true
}

// release ends a takeover and delivers what it held.
func (p *takeovers) release(h *Host, slot aiSlot, t *takeover) {
	p.mu.Lock()
	if p.open[slot] == t {
		delete(p.open, slot)
	}
	if !p.settled {
		h.bg(p.wg.Go, "deliver", func() { Deliver(h, slot.group, slot.member) }, "g", slot.group)
	}
	p.mu.Unlock()
}

func takeoverHold(h *Host, key SessionKey) (string, bool) {
	p := takeoverState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.open[aiSlot{group: key.Group, member: key.Member}]
	return "takeover", t != nil && !t.free
}

// settleTakeovers closes the open terminals without handing them back: their
// rows stay in takeover and are handed back when the node starts again
// (flows/takeover.md E). It ends the hand-backs that are running.
func settleTakeovers(h *Host) {
	p := takeoverState(h)
	p.mu.Lock()
	p.settled = true
	p.cancel()
	var ptys []*PTY
	for _, t := range p.open {
		ptys = append(ptys, t.pty)
	}
	p.mu.Unlock()
	for _, pt := range ptys {
		if pt != nil {
			pt.Close()
		}
	}
	p.wg.Wait()
}

func takeoverStart(ctx context.Context, h *Host, from Owner, _ string, data json.RawMessage) (any, error) {
	req, ai, assignFirst, err := checkTakeoverStart(h, from, data)
	if err != nil {
		return nil, err
	}
	slot := aiSlot{group: string(req.Group), member: string(req.Member)}
	p := takeoverState(h)
	t := &takeover{from: from, nonce: string(req.Nonce), assign: assignFirst}
	if !p.claim(h, slot, t, true) {
		return nil, &Error{"busy", "the AI is taken over or is being given a turn"}
	}
	if err := openTerminal(ctx, h, p, slot, ai, t, string(req.Task)); err != nil {
		p.release(h, slot, t)
		return nil, err
	}
	return nil, nil
}

func checkTakeoverStart(h *Host, from Owner, data json.RawMessage) (proto.TakeoverStart, aiInfo, bool, error) {
	var req proto.TakeoverStart
	if err := proto.Decode(data, &req); err != nil {
		return req, aiInfo{}, false, &Error{"bad_request", err.Error()}
	}
	ai, local, _ := lookupLocalAI(h, string(req.Group), string(req.Member))
	if !local {
		return req, aiInfo{}, false, &Error{"not_found", "no such AI on this node"}
	}
	assignFirst, err := checkTakeover(ai.agent)
	if err == nil && from.Path() != "direct" {
		err = &Error{"relay", "the terminal needs a direct path"}
	}
	return req, ai, assignFirst, err
}

func openTerminal(ctx context.Context, h *Host, p *takeovers, slot aiSlot, ai aiInfo, t *takeover, taskID string) error {
	interruptTurn(ctx, h, slot)
	grp, _ := lookupRow[proto.GroupRow](h, slot.group, "group", slot.group)
	target, err := takeoverTarget(h, slot, ai, taskID)
	if err != nil {
		return err
	}
	return openSession(ctx, h, p, ai, target, grp.Home, t)
}

func openSession(ctx context.Context, h *Host, p *takeovers, ai aiInfo, target routedTarget, home string, t *takeover) error {
	s, err := prepareTargetSession(h, ai, target, home)
	if gone := missingFolder(s); err == nil && gone != "" {
		err = &Error{"cli", gone}
	}
	if err == nil {
		err = startTerminal(ctx, h, p, s.SessionKey, t)
	}
	return err
}

// takeoverTarget picks the session of flows/takeover.md section A: the group
// session for a working AI without an unfinished task. No task is created.
func takeoverTarget(h *Host, slot aiSlot, ai aiInfo, taskID string) (routedTarget, error) {
	target := routedTarget{key: SessionKey{Group: slot.group, Member: slot.member, Task: taskID}, kind: TaskSession}
	if ai.secretary {
		target.key.Task, target.kind = "", GroupSession
		return target, nil
	}
	if taskID != "" {
		return namedTarget(h, target)
	}
	if target.key.Task = lastTask(h, slot); target.key.Task == "" {
		target.kind = GroupSession
	}
	return target, nil
}

func namedTarget(h *Host, target routedTarget) (routedTarget, error) {
	task, ok := lookupTaskRow(h, target.key.Group, target.key.Task)
	if !ok {
		return target, &Error{"not_found", "no such task"}
	}
	_, reviewing := h.Sessions.Get(target.key)
	switch {
	case !openTask(task):
		return target, &Error{"conflict", "the task is " + task.State}
	case task.Assignee == target.key.Member:
		return target, nil
	case task.Reviewer == target.key.Member && reviewing:
		target.kind = ReviewSession
		return target, nil
	}
	return target, &Error{"conflict", "the AI has no session on that task"}
}

func openTask(task proto.TaskRow) bool {
	return task.State == "active" || task.State == "paused" || task.State == "failed"
}

// lastTask returns the task of the AI's task session used last whose task is
// unfinished, or "".
func lastTask(h *Host, slot aiSlot) string {
	open := slices.DeleteFunc(memberSessions(h, slot.group, slot.member), func(s Session) bool {
		task, _ := lookupTaskRow(h, s.Group, s.Task)
		return s.Kind != TaskSession || !openTask(task)
	})
	if len(open) == 0 {
		return ""
	}
	return slices.MaxFunc(open, func(a, b Session) int { return cmp.Compare(a.LastActive, b.LastActive) }).Task
}

// startTerminal closes the session's structured CLI and opens the
// interactive CLI in a pseudo-terminal (flows/takeover.md steps 4 to 7).
func startTerminal(ctx context.Context, h *Host, p *takeovers, key SessionKey, t *takeover) error {
	Runner.Close(h, key)
	s, _ := h.Sessions.Get(key)
	cfg, err := readLaunchConfig(h, s.Agent, s.Group)
	if err == nil {
		err = h.Sessions.Move(key, s.State, Takeover)
	}
	if err != nil {
		return err
	}
	token := h.Sessions.Admit(key, cfg.level, true)
	launch := Launch{
		Folder: s.Folder, Session: s.CLISession, Level: cfg.level,
		Shim: append(slices.Clone(h.Shim), "--token", token), Instructions: Instructions(s),
		RepoConfig: s.Kind != GroupSession, ToolTimeout: groupToolTimeout(h, s.Group),
	}
	p.mu.Lock()
	t.key = key
	p.mu.Unlock()
	pt, id, err := h.StartPTY(s.Group, s.Member, s.Agent, memberLaunch(h, s, launch), func() { p.closed(h, t, token) })
	if err != nil {
		h.Sessions.Forget(token)
		h.Sessions.Move(key, Takeover, Dormant)
		sendMemberStatus(ctx, h, stateOf(h), s.Home, s.Group, s.Member, nil)
		return err
	}
	h.Sessions.Update(key, func(r *Session) { r.CLISession = cmp.Or(id, r.CLISession) })
	p.mu.Lock()
	t.pty = pt
	p.mu.Unlock()
	time.AfterFunc(takeoverAttach, func() { closeUnattached(pt) })
	p.watch(h, t.from)
	sendMemberStatus(ctx, h, stateOf(h), s.Home, s.Group, s.Member, nil)
	return nil
}

// closeUnattached closes a terminal the browser's pty channel has not
// attached to.
func closeUnattached(pt *PTY) {
	if !pt.Attached() {
		pt.Close()
	}
}

// watch closes the terminals opened over from once its path is no longer
// direct or it has ended (flows/connect.md step 9). PathChanged has one
// reader, so one watch runs per connection.
func (p *takeovers) watch(h *Host, from Owner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.watched[from] {
		return
	}
	p.watched[from] = true
	h.bg(p.wg.Go, "path_watch", func() {
		for from.Path() == "direct" && p.waitPath(from) {
		}
		p.closeFrom(from)
	})
}

func (p *takeovers) closeFrom(from Owner) {
	p.mu.Lock()
	delete(p.watched, from)
	var ptys []*PTY
	for _, t := range p.open {
		if t.from == from && t.pty != nil {
			ptys = append(ptys, t.pty)
		}
	}
	p.mu.Unlock()
	for _, pt := range ptys {
		pt.Close()
	}
}

// waitPath waits for from's path to change. It is false once from has ended
// or the host settles.
func (p *takeovers) waitPath(from Owner) bool {
	select {
	case <-from.PathChanged():
		return true
	case <-from.Done():
	case <-p.ctx.Done():
	}
	return false
}

func takeoverEnd(_ context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	var req proto.Takeover
	if err := proto.Decode(data, &req); err != nil {
		return nil, &Error{"bad_request", err.Error()}
	}
	h.ClosePTY(string(req.Group), string(req.Member))
	return nil, nil
}

// ReplayTakeover answers a repeated takeover.start (contracts/commands.md,
// "Once only"): ok while the terminal it opened is still open on from, else
// conflict.
func (h *Host) ReplayTakeover(_ context.Context, from Owner, _ string, data json.RawMessage) (any, error) {
	var req proto.TakeoverStart
	proto.Decode(data, &req)
	p := takeoverState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.open[aiSlot{group: string(req.Group), member: string(req.Member)}]
	if t != nil && t.from == from && t.nonce == string(req.Nonce) && t.pty != nil && !t.closed {
		return nil, nil
	}
	return nil, &Error{"conflict", "that terminal is no longer open"}
}

// closed runs when the terminal has closed: the CLI has exited.
func (p *takeovers) closed(h *Host, t *takeover, token string) {
	h.Sessions.Forget(token)
	p.mu.Lock()
	defer p.mu.Unlock()
	t.closed = true
	if !p.settled {
		h.bg(p.wg.Go, "hand_back", func() { handBack(h, p, t) }, "g", t.key.Group, "task", t.key.Task)
	}
}

// handBackSaved hands back the sessions of group that a previous run of the
// node left in takeover (flows/takeover.md E).
func handBackSaved(h *Host, group string) {
	p := takeoverState(h)
	for _, s := range h.Sessions.All() {
		t := &takeover{key: s.SessionKey}
		if s.Group == group && s.State == Takeover && p.claim(h, aiSlot{group: s.Group, member: s.Member}, t, false) {
			h.bg(p.wg.Go, "hand_back", func() { handBack(h, p, t) }, "g", t.key.Group, "task", t.key.Task)
		}
	}
}

// handBack runs flows/takeover.md steps 14 to 16.
func handBack(h *Host, p *takeovers, t *takeover) {
	ctx := p.ctx
	defer p.release(h, aiSlot{group: t.key.Group, member: t.key.Member}, t)
	s, ok := h.Sessions.Get(t.key)
	if !ok || s.State != Takeover {
		reportEnded(ctx, h, t.key)
		return
	}
	p.mu.Lock()
	t.free = true
	p.mu.Unlock()
	handBackTurn(ctx, h, s)
	p.mu.Lock()
	posted := t.posted
	p.mu.Unlock()
	if !posted {
		postHandback(ctx, h, s)
	}
}

// reportEnded reports the member's state once its taken-over session has
// ended without a hand-back turn: the home last heard takeover.
func reportEnded(ctx context.Context, h *Host, key SessionKey) {
	grp, _ := lookupRow[proto.GroupRow](h, key.Group, "group", key.Group)
	sendMemberStatus(ctx, h, stateOf(h), grp.Home, key.Group, key.Member, nil)
}

// handBackTurn gives the session its hand-back turn and waits for its end.
// A held AI gets none: the session goes dormant.
func handBackTurn(ctx context.Context, h *Host, s Session) {
	text, turn, err := Compose(h, s, HandbackTurn, nil)
	if _, held := h.Held(s.SessionKey); held && err == nil {
		err = &Error{"busy", "the AI is held"}
	}
	if err == nil {
		err = Runner.Turn(ctx, h, s.SessionKey, text, turn)
	}
	if err == nil {
		waitTurnOver(ctx, h, s.SessionKey)
	} else if h.Sessions.Move(s.SessionKey, Takeover, Dormant) == nil {
		sendMemberStatus(ctx, h, stateOf(h), s.Home, s.Group, s.Member, nil)
	}
}

// postHandback posts the hand-back for an AI that did not, signed for the AI,
// with an empty text (flows/takeover.md step 15).
func postHandback(ctx context.Context, h *Host, s Session) {
	var thread *proto.ID
	if s.Task != "" {
		thread = new(proto.ID(s.Task))
	}
	post := proto.Post[proto.EmptyBody]{HomeCommand: homeHeader(h, s.Group, s.Member, "post"), Kind: "handback", Thread: thread, Mentions: []proto.ID{}}
	sendHostCommand(ctx, h, s.Home, marshalWire(post))
}

// handbackPosted notes that the AI of a session posted its hand-back.
func handbackPosted(h *Host, key SessionKey) {
	p := takeoverState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	if t := p.open[aiSlot{group: key.Group, member: key.Member}]; t != nil {
		t.posted = true
	}
}
