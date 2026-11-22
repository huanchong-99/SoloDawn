package host

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
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
// each takeover, and is never held across a call out of this file.
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
	key     SessionKey
	from    Owner
	nonce   string
	created bool // the takeover started the session's task
	pty     *PTY
	closed  bool // the terminal has closed
	facts   proto.HandbackBody
	posted  bool // the AI posted its hand-back
	free    bool // the hand-back has started: the takeover holds nothing
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

// claim registers t for slot when no takeover is registered there, or, if
// replaceFree, when the one registered has started its hand-back.
func (p *takeovers) claim(slot aiSlot, t *takeover, replaceFree bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if old := p.open[slot]; old != nil && !(replaceFree && old.free) {
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
	settled := p.settled
	p.mu.Unlock()
	if !settled {
		go Deliver(h, slot.group, slot.member)
	}
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
	req, ai, err := checkTakeoverStart(h, from, data)
	if err != nil {
		return nil, err
	}
	slot := aiSlot{group: string(req.Group), member: string(req.Member)}
	p := takeoverState(h)
	t := &takeover{from: from, nonce: string(req.Nonce)}
	if mergeRunning(h, slot) || !p.claim(slot, t, true) {
		return nil, &Error{"busy", "the AI is taken over or one of its tasks is merging"}
	}
	if err := openTerminal(ctx, h, p, slot, ai, t, string(req.Task)); err != nil {
		p.release(h, slot, t)
		return nil, err
	}
	return nil, nil
}

func checkTakeoverStart(h *Host, from Owner, data json.RawMessage) (proto.TakeoverStart, aiInfo, error) {
	var req proto.TakeoverStart
	if err := proto.Decode(data, &req); err != nil {
		return req, aiInfo{}, &Error{"bad_request", err.Error()}
	}
	if from.Path() != "direct" {
		return req, aiInfo{}, &Error{"relay", "the terminal needs a direct path"}
	}
	ai, local, _ := lookupLocalAI(h, string(req.Group), string(req.Member))
	if !local {
		return req, aiInfo{}, &Error{"not_found", "no such AI on this node"}
	}
	return req, ai, nil
}

// mergeRunning reports whether the host is merging a task of the AI.
func mergeRunning(h *Host, slot aiSlot) bool {
	d := deliv(h)
	d.mu.Lock()
	keys := slices.Collect(maps.Keys(d.merging))
	d.mu.Unlock()
	for _, k := range keys {
		group, rest, _ := strings.Cut(k, "\x00")
		taskID, _, _ := strings.Cut(rest, "\x00")
		task, _ := lookupTaskRow(h, group, taskID)
		if group == slot.group && task.Assignee == slot.member {
			return true
		}
	}
	return false
}

func openTerminal(ctx context.Context, h *Host, p *takeovers, slot aiSlot, ai aiInfo, t *takeover, taskID string) error {
	interruptTurn(ctx, h, slot)
	grp, _ := lookupRow[proto.GroupRow](h, slot.group, "group", slot.group)
	target, err := takeoverTarget(ctx, h, slot, ai, grp.Home, taskID, t)
	if err != nil {
		return err
	}
	if err = openSession(ctx, h, p, ai, target, grp.Home, t); err != nil && t.created {
		sendHostCommand(ctx, h, grp.Home, marshalWire(hostTaskCommand(h, slot.group, "cancel", target.key.Task)))
		h.Sessions.End(target.key)
	}
	return err
}

func openSession(ctx context.Context, h *Host, p *takeovers, ai aiInfo, target routedTarget, home string, t *takeover) error {
	s, gitErr, err := prepareTargetSession(h, ai, target, home, nil)
	err = cmp.Or(gitErr, err)
	if err == nil && t.created {
		err = warmUp(ctx, h, s)
	}
	if err == nil {
		err = startTerminal(ctx, h, p, s.SessionKey, t)
	}
	return err
}

// takeoverTarget picks the session of flows/takeover.md section A, and starts
// a task for a working AI that has none.
func takeoverTarget(ctx context.Context, h *Host, slot aiSlot, ai aiInfo, home, taskID string, t *takeover) (routedTarget, error) {
	target := routedTarget{key: SessionKey{Group: slot.group, Member: slot.member, Task: taskID}, kind: TaskSession}
	if ai.secretary {
		target.key.Task, target.kind = "", GroupSession
		return target, nil
	}
	if taskID != "" {
		return namedTarget(h, target)
	}
	if target.key.Task = lastTask(h, slot); target.key.Task != "" {
		return target, nil
	}
	id, err := startTakeoverTask(ctx, h, slot, home)
	target.key.Task, t.created = id, true
	return target, err
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
	var last Session
	for _, s := range memberSessions(h, slot.group, slot.member) {
		task, _ := lookupTaskRow(h, s.Group, s.Task)
		if s.Kind == TaskSession && openTask(task) && (last.Task == "" || s.LastActive > last.LastActive) {
			last = s
		}
	}
	return last.Task
}

// startTakeoverTask sends task start for a task of origin takeover and keeps
// its row until the home's own arrives.
func startTakeoverTask(ctx context.Context, h *Host, slot aiSlot, home string) (string, error) {
	const title = "Owner takeover"
	id := proto.NewNonce()
	cmd := proto.TaskStart{TaskCommand: tskCommand(h, slot.group, slot.member, "start", id), Title: title, Origin: "takeover"}
	ans, err := h.Command(ctx, home, marshalWire(cmd))
	if err != nil {
		return "", err
	}
	var res proto.CmdResult
	proto.Decode(ans, &res)
	if _, ok := lookupTaskRow(h, slot.group, string(id)); !ok {
		row := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: string(id)}, Group: slot.group, Num: res.Num,
			Title: title, Slug: proto.MakeSlug(title), Origin: "takeover", State: "active", Step: "branch",
			Assignee: slot.member, Depends: []string{},
		}
		h.Rows.Put(slot.group, "task", string(id), json.RawMessage(marshalWire(row)))
	}
	return string(id), nil
}

// warmUp gives a new task session its assign turn in structured mode first
// when its CLI cannot open a new session in a terminal (flows/takeover.md
// section A).
func warmUp(ctx context.Context, h *Host, s Session) error {
	if _, _, err := Drivers[s.Agent].Interactive(Launch{Level: 3, Shim: h.Shim}); err == nil {
		return nil
	}
	task, _ := lookupTaskRow(h, s.Group, s.Task)
	item, _ := json.Marshal(proto.InboxItem{Item: proto.Item{AI: proto.ID(s.Member), Why: "assign", Task: proto.ID(s.Task)}, Task: &task})
	text, turn, err := Compose(h, s, ItemsTurn, []json.RawMessage{item})
	if err == nil {
		err = Runner.Turn(ctx, h, s.SessionKey, text, turn)
	}
	if err == nil {
		err = waitTurnOver(ctx, h, s.SessionKey)
	}
	return err
}

// startTerminal closes the session's structured CLI, notes its commit, and
// opens the interactive CLI in a pseudo-terminal (flows/takeover.md steps 4
// to 7).
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
	head := gitHead(ctx, h, s)
	h.Sessions.Update(key, func(r *Session) { r.TakeoverAt = head })
	token := h.Sessions.Admit(key, cfg.level, true)
	launch := Launch{
		Folder: sessionDir(h, s), Session: s.CLISession, Level: cfg.level,
		Shim: append(slices.Clone(h.Shim), "--token", token), Instructions: Instructions(s),
		RepoConfig: s.Kind != GroupSession, ToolTimeout: groupToolTimeout(h, s.Group),
	}
	p.mu.Lock()
	t.key = key
	p.mu.Unlock()
	pt, id, err := h.StartPTY(s.Group, s.Member, s.Agent, launch, func() { p.closed(h, t, token) })
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

func gitHead(ctx context.Context, h *Host, s Session) string {
	if s.Kind != TaskSession || !hasGitClone(projectDir(h, s.Group)) {
		return ""
	}
	out, _ := workspaceGit(ctx, sessionDir(h, s), "rev-parse", "HEAD")
	return strings.TrimSpace(out)
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
	p.wg.Go(func() {
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
		p.wg.Go(func() { handBack(h, p, t) })
	}
}

// handBackSaved hands back the sessions of group that a previous run of the
// node left in takeover (flows/takeover.md E).
func handBackSaved(h *Host, group string) {
	p := takeoverState(h)
	for _, s := range h.Sessions.All() {
		t := &takeover{key: s.SessionKey}
		if s.Group == group && s.State == Takeover && p.claim(aiSlot{group: s.Group, member: s.Member}, t, false) {
			p.wg.Go(func() { handBack(h, p, t) })
		}
	}
}

// handBack runs flows/takeover.md steps 14 to 18.
func handBack(h *Host, p *takeovers, t *takeover) {
	ctx := p.ctx
	defer p.release(h, aiSlot{group: t.key.Group, member: t.key.Member}, t)
	s, ok := h.Sessions.Get(t.key)
	if !ok || s.State != Takeover {
		reportEnded(ctx, h, t.key)
		return
	}
	facts, changed := handbackFacts(ctx, h, s)
	p.mu.Lock()
	t.facts, t.free = facts, true
	p.mu.Unlock()
	if t.created && !changed {
		sendHostCommand(ctx, h, s.Home, marshalWire(hostTaskCommand(h, s.Group, "cancel", s.Task)))
		h.Sessions.End(s.SessionKey)
		reportEnded(ctx, h, t.key)
		return
	}
	handBackTurn(ctx, h, s)
	p.mu.Lock()
	posted := t.posted
	p.mu.Unlock()
	if !posted {
		postFacts(ctx, h, s, facts)
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
	h.Sessions.Update(s.SessionKey, func(r *Session) { r.TakeoverAt = "" })
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

// postFacts posts the hand-back for an AI that did not: the facts, signed
// for the AI, with an empty text (flows/takeover.md step 17).
func postFacts(ctx context.Context, h *Host, s Session, facts proto.HandbackBody) {
	var thread *proto.ID
	if s.Task != "" {
		thread = new(proto.ID(s.Task))
	}
	post := proto.Post[proto.HandbackBody]{HomeCommand: homeHeader(h, s.Group, s.Member, "post"), Kind: "handback", Thread: thread, Mentions: []proto.ID{}, Body: facts}
	sendHostCommand(ctx, h, s.Home, marshalWire(post))
}

// handbackFacts reads the hand-back facts of a task session (contracts/
// local.md section 7) and whether anything changed since the takeover
// started. Other sessions are read only and report none.
func handbackFacts(ctx context.Context, h *Host, s Session) (proto.HandbackBody, bool) {
	facts := proto.HandbackBody{Files: []string{}, Commits: []proto.CommitSubject{}}
	if s.TakeoverAt == "" {
		return facts, false
	}
	git := func(args ...string) string {
		out, _ := workspaceGit(ctx, sessionDir(h, s), args...)
		return strings.TrimSpace(out)
	}
	head, status := git("rev-parse", "HEAD"), git("status", "--porcelain", "--untracked-files=all")
	for line := range strings.Lines(git("log", s.TakeoverAt+"..HEAD", "--format=%h %s")) {
		sha, subject, _ := strings.Cut(strings.TrimSpace(line), " ")
		facts.Commits = append(facts.Commits, proto.CommitSubject{SHA: sha, Subject: subject})
	}
	_, facts.Added, facts.Removed = tskParseShortstat(git("diff", "--shortstat", s.TakeoverAt))
	facts.Files = append(gitFiles(git("diff", "--name-only", s.TakeoverAt), ""), gitFiles(status, "?? ")...)
	return facts, head != s.TakeoverAt || status != ""
}

// gitFiles returns the paths of the lines of a Git listing that start with
// prefix.
func gitFiles(out, prefix string) []string {
	files := []string{}
	for line := range strings.Lines(out) {
		if name, ok := strings.CutPrefix(line, prefix); ok {
			files = append(files, strings.TrimSpace(name))
		}
	}
	return files
}

// takeoverFacts returns the facts the hand-back post of a session carries,
// and notes that the AI posted it.
func takeoverFacts(h *Host, key SessionKey) proto.HandbackBody {
	p := takeoverState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.open[aiSlot{group: key.Group, member: key.Member}]
	if t == nil {
		return proto.HandbackBody{Files: []string{}, Commits: []proto.CommitSubject{}}
	}
	t.posted = true
	return t.facts
}
