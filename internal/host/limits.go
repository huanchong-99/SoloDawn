package host

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	codexSilentLimit = 5 * time.Minute
	queryLimitsSQL   = `SELECT coalesce((SELECT value FROM setting WHERE key = 'offline'), 'false'),
	(SELECT json_group_object(day || ' ' || agent, json_extract(data, '$.turns')) FROM usage)`
	countTurnSQL = `INSERT INTO usage(agent, day, data) VALUES(?, ?, '{"turns":1}')
	ON CONFLICT(agent, day) DO UPDATE SET data = json_set(data, '$.turns', coalesce(json_extract(data, '$.turns'), 0) + 1)
	RETURNING json_extract(data, '$.turns')`
)

func init() {
	Register(Surface, "host.offline", handleHostOffline)
	Register(Surface, "host.online", handleHostOnline)
	Register(Built, "limits", loadLimits)
	Register(Holds, "limits", holdLimits)
	Register(Listeners, "limits", listenLimits)
	Register(Settled, "limits", settleLimits)
}

type windowState struct {
	pct    *float64
	resets *int64
}

type turnWatch struct {
	gen      uint64
	deadline *time.Timer
	silent   *time.Timer
}

type memberHomeSlot struct {
	home, group, member string
}

// limitsPart keeps, via Part[limitsPart](h), what the member status reads
// (offline, today's turns, rate-limit windows) in memory so that it never
// touches the store, and the timers of running turns and rate limits.
//
// Lock order: mu is a leaf lock guarding only these maps and timer handles;
// nothing called under mu takes Store, Sessions, Rows, Host or runtime locks.
type limitsPart struct {
	mu         sync.Mutex
	wg         sync.WaitGroup
	settled    bool // Settle has begun: nothing more starts on wg
	offline    bool
	seq        uint64
	turns      map[string]int // by "<local day> <agent>"
	windows    map[string]windowState
	rateUntil  map[string]int64
	rateTimers map[string]*time.Timer
	watches    map[SessionKey]*turnWatch
	pending    map[SessionKey]proto.RateLimitedEvent
}

func limitsOf(h *Host) *limitsPart {
	lp := Part[limitsPart](h)
	lp.mu.Lock()
	if lp.windows == nil {
		lp.turns = map[string]int{}
		lp.windows = map[string]windowState{}
		lp.rateUntil = map[string]int64{}
		lp.rateTimers = map[string]*time.Timer{}
		lp.watches = map[SessionKey]*turnWatch{}
		lp.pending = map[SessionKey]proto.RateLimitedEvent{}
	}
	lp.mu.Unlock()
	return lp
}

func loadLimits(h *Host) {
	lp := limitsOf(h)
	var offline, turns string
	h.Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow(queryLimitsSQL).Scan(&offline, &turns)
	})
	lp.mu.Lock()
	lp.offline = offline == "true"
	json.Unmarshal([]byte(turns), &lp.turns)
	lp.mu.Unlock()
}

func settleLimits(h *Host) {
	lp := limitsOf(h)
	lp.mu.Lock()
	lp.settled = true
	for agent, t := range lp.rateTimers {
		t.Stop()
		delete(lp.rateTimers, agent)
	}
	for key, tw := range lp.watches {
		stopWatchTimers(tw)
		delete(lp.watches, key)
	}
	lp.mu.Unlock()
	lp.wg.Wait()
}

func stopWatchTimers(tw *turnWatch) {
	tw.deadline.Stop()
	if tw.silent != nil {
		tw.silent.Stop()
	}
}

func stopTurnWatch(h *Host, key SessionKey) {
	lp := limitsOf(h)
	lp.mu.Lock()
	if tw := lp.watches[key]; tw != nil {
		stopWatchTimers(tw)
		delete(lp.watches, key)
	}
	lp.mu.Unlock()
}

// takePendingLimitEvent returns the rate_limited event that arrived during
// the session's turn, for the member status that ends the turn.
func takePendingLimitEvent(h *Host, key SessionKey) any {
	lp := limitsOf(h)
	lp.mu.Lock()
	defer lp.mu.Unlock()
	ev, ok := lp.pending[key]
	if !ok {
		return nil
	}
	delete(lp.pending, key)
	return ev
}

func handleHostOffline(ctx context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	if err := setOffline(h, data, true); err != nil {
		return nil, err
	}
	rt := stateOf(h)
	interrupted := map[memberHomeSlot]string{}
	for _, s := range h.Sessions.All() {
		if closeSession(h, rt, s.SessionKey) {
			interrupted[memberHomeSlot{s.Home, s.Group, s.Member}] = s.Task
		}
	}
	for _, slot := range hostedSlots(h, "") {
		var ev any
		if task, ok := interrupted[slot]; ok {
			ev = proto.TurnInterruptedEvent{SystemCode: proto.SystemCode{Code: "turn_interrupted"}, Reason: "stop", Task: proto.ID(task)}
		}
		reportState(ctx, h, slot, ev)
	}
	return nil, nil
}

func handleHostOnline(ctx context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	if err := setOffline(h, data, false); err != nil {
		return nil, err
	}
	releaseHeld(ctx, h, "")
	return nil, nil
}

// setOffline writes the owner's "all offline" switch to the store and to
// memory, where holds and member status read it.
func setOffline(h *Host, data json.RawMessage, offline bool) error {
	if proto.Decode(data, &proto.HostCommand{}) != nil {
		return &Error{Code: "bad_request", Message: "invalid host command payload"}
	}
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES('offline', ?)", marshalWire(offline))
		return err
	}
	if err := h.Store.Write(write); err != nil {
		return err
	}
	lp := limitsOf(h)
	lp.mu.Lock()
	lp.offline = offline
	lp.mu.Unlock()
	return nil
}

// releaseHeld reports the member status of the AIs of agent (every AI when
// agent is "") and gives held items their turn.
func releaseHeld(ctx context.Context, h *Host, agent string) {
	for _, slot := range hostedSlots(h, agent) {
		reportState(ctx, h, slot, nil)
	}
	limitsOf(h).spawn(h, func() { Deliver(h, "", "") })
}

// spawn runs f on wg unless the part has settled; the Add happens under mu,
// so it comes before Settle's Wait or not at all.
func (lp *limitsPart) spawn(h *Host, f func()) {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	if !lp.settled {
		h.bg(lp.wg.Go, "deliver", f)
	}
}

// hostedSlots returns the AIs of agent that this node hosts (every AI when
// agent is ""): those with a session, and those a member row names, which need
// not have one yet.
func hostedSlots(h *Host, agent string) []memberHomeSlot {
	var slots []memberHomeSlot
	for _, s := range h.Sessions.All() {
		slots = append(slots, memberHomeSlot{s.Home, s.Group, s.Member})
	}
	for group, home := range deliv(h).homes() {
		slots = append(slots, groupSlots(h, home, group)...)
	}
	slots = slices.DeleteFunc(slots, func(s memberHomeSlot) bool { return agent != "" && memberAgent(h, s.group, s.member) != agent })
	slices.SortFunc(slots, func(a, b memberHomeSlot) int {
		return cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(a.member, b.member), cmp.Compare(a.home, b.home))
	})
	return slices.Compact(slots)
}

func groupSlots(h *Host, home, group string) []memberHomeSlot {
	var slots []memberHomeSlot
	for _, ai := range localGroupAIs(h, group) {
		slots = append(slots, memberHomeSlot{home, group, ai})
	}
	return slots
}

// reportState reports the state of an AI, and again while it has changed
// meanwhile: the report of a turn that began or ended may have been overtaken.
func reportState(ctx context.Context, h *Host, slot memberHomeSlot, ev any) {
	for {
		state, task := h.MemberState(slot.group, slot.member)
		err := sendMemberStatus(ctx, h, stateOf(h), slot.home, slot.group, slot.member, ev)
		newState, newTask := h.MemberState(slot.group, slot.member)
		if err != nil || newState == state && newTask == task {
			return
		}
		ev = nil
	}
}

func holdLimits(h *Host, key SessionKey) (string, bool) {
	agent := memberAgent(h, key.Group, key.Member)
	lp := limitsOf(h)
	lp.mu.Lock()
	defer lp.mu.Unlock()
	if lp.offline {
		return "offline", true
	}
	if _, limited := lp.rateUntil[agent]; limited {
		return "rate_limit", true
	}
	return "", false
}

func memberAgent(h *Host, group, member string) string {
	for _, s := range h.Sessions.All() {
		if s.Group == group && s.Member == member {
			return s.Agent
		}
	}
	ai, _, _ := lookupLocalAI(h, group, member)
	return ai.agent
}

// memberUsage is the usage of a member status: the latest rate-limit window
// of the AI's CLI and its turns today, all read from memory.
func memberUsage(h *Host, group, member string) proto.Usage {
	agent := memberAgent(h, group, member)
	if agent == externalAgent {
		return extUsage(h, aiGroupKey{group: group, member: member})
	}
	today := h.Now().Format(time.DateOnly)
	lp := limitsOf(h)
	lp.mu.Lock()
	defer lp.mu.Unlock()
	win := lp.windows[agent]
	u := proto.Usage{WindowPct: win.pct, WindowResets: win.resets}
	if n := lp.turns[today+" "+agent]; n > 0 {
		u.TurnsToday = &n
	}
	return u
}

func listenLimits(h *Host, key SessionKey, e Event) {
	s, ok := h.Sessions.Get(key)
	if !ok {
		return
	}
	lp := limitsOf(h)
	switch e.Kind {
	case TurnBegan:
		startTurnWatch(h, lp, s)
	case TurnEnded:
		stopTurnWatch(h, key)
		countTurn(h, lp, s.Agent)
	case RateLimited:
		onRateLimited(h, lp, s, e.Resets)
	default:
		onTurnActivity(lp, s, e)
	}
}

func onTurnActivity(lp *limitsPart, s Session, e Event) {
	if e.Kind == Used {
		lp.recordWindow(s.Agent, e.Usage)
	}
	if e.Kind == Used || e.Kind == Active || e.Kind == Asked {
		resetSilentWatch(lp, s.SessionKey)
	}
}

// countTurn adds a turn to the agent's usage row of today and keeps the new
// count in memory for member status.
func countTurn(h *Host, lp *limitsPart, agent string) {
	day := h.Now().Format(time.DateOnly)
	var n int
	h.Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow(countTurnSQL, agent, day).Scan(&n)
	})
	lp.mu.Lock()
	lp.turns[day+" "+agent] = n
	lp.mu.Unlock()
}

func startTurnWatch(h *Host, lp *limitsPart, s Session) {
	limit := groupToolTimeout(h, s.Group)
	lp.mu.Lock()
	defer lp.mu.Unlock()
	if prev := lp.watches[s.SessionKey]; prev != nil {
		stopWatchTimers(prev)
	}
	delete(lp.pending, s.SessionKey)
	lp.seq++
	gen := lp.seq
	tw := &turnWatch{gen: gen}
	tw.deadline = time.AfterFunc(limit, func() { fireTurnLimit(h, lp, s.SessionKey, gen, false) })
	if s.Agent == "codex" {
		tw.silent = time.AfterFunc(codexSilentLimit, func() { fireTurnLimit(h, lp, s.SessionKey, gen, true) })
	}
	lp.watches[s.SessionKey] = tw
}

func resetSilentWatch(lp *limitsPart, key SessionKey) {
	lp.mu.Lock()
	if tw := lp.watches[key]; tw != nil && tw.silent != nil {
		tw.silent.Reset(codexSilentLimit)
	}
	lp.mu.Unlock()
}

// fireTurnLimit interrupts the turn of watch gen when it still runs. The
// silent limit of Codex waits while a tool call is open.
func fireTurnLimit(h *Host, lp *limitsPart, key SessionKey, gen uint64, silent bool) {
	lp.mu.Lock()
	tw := lp.watches[key]
	current := tw != nil && tw.gen == gen
	lp.mu.Unlock()
	switch {
	case !current:
	case silent && h.Sessions.Calls(key) > 0:
		resetSilentWatch(lp, key)
	case silent:
		interruptSessionTurn(h, key, "timeout", "Turn was silent for 5 minutes.")
	default:
		interruptSessionTurn(h, key, "timeout", "Turn time limit reached.")
	}
}

func (lp *limitsPart) recordWindow(agent string, u Usage) {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	cur := lp.windows[agent]
	if u.WindowPct != nil {
		cur.pct = new(*u.WindowPct)
	}
	if u.WindowResets != nil {
		cur.resets = new(*u.WindowResets)
	}
	lp.windows[agent] = cur
}

// onRateLimited holds the agent's items until resets. The rate_limited event
// goes with the member status that ends a running turn, so that it cannot
// overtake it; outside a turn it is sent at once.
func onRateLimited(h *Host, lp *limitsPart, s Session, resets int64) {
	ev := proto.RateLimitedEvent{SystemCode: proto.SystemCode{Code: "rate_limited"}, Resets: resets}
	wait := time.Duration(resets-h.Now().UnixMilli()) * time.Millisecond
	lp.mu.Lock()
	defer lp.mu.Unlock()
	if lp.settled {
		return
	}
	if prev := lp.rateTimers[s.Agent]; prev != nil {
		prev.Stop()
	}
	lp.rateUntil[s.Agent] = resets
	lp.rateTimers[s.Agent] = time.AfterFunc(wait, func() { releaseRateLimit(h, lp, s.Agent) })
	if lp.watches[s.SessionKey] != nil {
		lp.pending[s.SessionKey] = ev
		return
	}
	h.bg(lp.wg.Go, "member_status", func() { sendMemberStatus(context.Background(), h, stateOf(h), s.Home, s.Group, s.Member, ev) }, "g", s.Group, "task", s.Task)
}

func releaseRateLimit(h *Host, lp *limitsPart, agent string) {
	lp.mu.Lock()
	delete(lp.rateUntil, agent)
	delete(lp.rateTimers, agent)
	lp.mu.Unlock()
	releaseHeld(context.Background(), h, agent)
}
