package host

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	codexSilentLimit = 5 * time.Minute
	queryOfflineSQL  = `SELECT coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'offline'), 0)`
	queryCapsSQL     = `SELECT coalesce((SELECT caps FROM agent WHERE agent = ?), '{}')`
)

func init() {
	Register(Surface, "host.offline", handleHostOffline)
	Register(Surface, "host.online", handleHostOnline)
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

type usageTotals struct {
	Turns     int     `json:"turns"`
	Tokens    int64   `json:"tokens"`
	HasTokens bool    `json:"has_tokens,omitempty"`
	USD       float64 `json:"usd"`
	HasUSD    bool    `json:"has_usd,omitempty"`
}

type storedDayUsage struct {
	usageTotals
	Tasks map[string]usageTotals `json:"tasks,omitempty"`
}

type memberHomeSlot struct {
	home, group, member string
}

// limitsPart keeps per-host rate windows, active turn timers, hold release
// timers and queued limit events via Part[limitsPart](h).
//
// Lock order: mu is a strict leaf lock guarding only limitsPart maps and timer
// handles; nothing called under mu acquires Store, Sessions, Rows, Host or
// runtime locks.
type limitsPart struct {
	mu         sync.Mutex
	wg         sync.WaitGroup
	seq        uint64
	windows    map[string]windowState
	rateUntil  map[string]int64
	rateTimers map[string]*time.Timer
	dayTimers  map[string]*time.Timer
	turns      map[SessionKey]*turnWatch
	pendingEv  map[SessionKey]any
}

func limitsOf(h *Host) *limitsPart {
	lp := Part[limitsPart](h)
	lp.mu.Lock()
	if lp.windows == nil {
		lp.windows = map[string]windowState{}
		lp.rateUntil = map[string]int64{}
		lp.rateTimers = map[string]*time.Timer{}
		lp.dayTimers = map[string]*time.Timer{}
		lp.turns = map[SessionKey]*turnWatch{}
		lp.pendingEv = map[SessionKey]any{}
	}
	lp.mu.Unlock()
	return lp
}

func settleLimits(h *Host) {
	lp := limitsOf(h)
	lp.mu.Lock()
	stopTimerMap(lp.rateTimers)
	stopTimerMap(lp.dayTimers)
	for key, tw := range lp.turns {
		stopWatchTimers(tw)
		delete(lp.turns, key)
	}
	lp.mu.Unlock()
	lp.wg.Wait()
}

func stopTimerMap(timers map[string]*time.Timer) {
	for k, t := range timers {
		t.Stop()
		delete(timers, k)
	}
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
	if tw := lp.turns[key]; tw != nil {
		stopWatchTimers(tw)
		delete(lp.turns, key)
	}
	lp.mu.Unlock()
}

func takePendingLimitEvent(h *Host, key SessionKey) any {
	lp := limitsOf(h)
	lp.mu.Lock()
	ev := lp.pendingEv[key]
	delete(lp.pendingEv, key)
	lp.mu.Unlock()
	return ev
}

func handleHostOffline(ctx context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	if err := decodeHostCommandOnly(data); err != nil {
		return nil, err
	}
	if err := writeOfflineSetting(h, "true"); err != nil {
		return nil, err
	}
	interrupted := map[memberHomeSlot]string{}
	for _, s := range h.Sessions.All() {
		if task, wasTurn := closeSessionForOffline(h, s.SessionKey); wasTurn {
			interrupted[memberHomeSlot{s.Home, s.Group, s.Member}] = task
		}
	}
	reportOfflineMembers(ctx, h, interrupted)
	return nil, nil
}

func reportOfflineMembers(ctx context.Context, h *Host, interrupted map[memberHomeSlot]string) {
	rt := stateOf(h)
	for _, slot := range distinctMemberSlots(h, "") {
		var ev any
		if task, ok := interrupted[slot]; ok {
			ev = proto.TurnInterruptedEvent{SystemCode: proto.SystemCode{Code: "turn_interrupted"}, Reason: "stop", Task: proto.ID(task)}
		}
		sendMemberStatus(ctx, h, rt, slot.home, slot.group, slot.member, ev)
	}
}

func handleHostOnline(ctx context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	if err := decodeHostCommandOnly(data); err != nil {
		return nil, err
	}
	if err := writeOfflineSetting(h, "false"); err != nil {
		return nil, err
	}
	rt := stateOf(h)
	slots := distinctMemberSlots(h, "")
	for _, slot := range slots {
		sendMemberStatus(ctx, h, rt, slot.home, slot.group, slot.member, nil)
	}
	triggerDeliverAfterHold(h, limitsOf(h), slots)
	return nil, nil
}

func decodeHostCommandOnly(data json.RawMessage) error {
	var cmd proto.HostCommand
	if proto.Decode(data, &cmd) != nil {
		return &Error{Code: "bad_request", Message: "invalid host command payload"}
	}
	return nil
}

func writeOfflineSetting(h *Host, rawBool string) error {
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES('offline', ?)", rawBool)
		return err
	}
	return h.Store.Write(write)
}

func isHostOffline(h *Host) bool {
	var raw int
	read := func(tx *store.Tx) error {
		return tx.QueryRow(queryOfflineSQL).Scan(&raw)
	}
	h.Store.Write(read)
	return raw != 0
}

func distinctMemberSlots(h *Host, onlyAgent string) []memberHomeSlot {
	seen := map[memberHomeSlot]bool{}
	var out []memberHomeSlot
	for _, s := range h.Sessions.All() {
		slot := memberHomeSlot{s.Home, s.Group, s.Member}
		if (onlyAgent == "" || s.Agent == onlyAgent) && !seen[slot] {
			seen[slot] = true
			out = append(out, slot)
		}
	}
	return out
}

func triggerDeliverAfterHold(h *Host, lp *limitsPart, slots []memberHomeSlot) {
	d := Deliver
	if d == nil {
		return
	}
	var group, member string
	if len(slots) > 0 {
		group, member = slots[0].group, slots[0].member
	}
	lp.wg.Go(func() { d(h, group, member) })
}

func holdLimits(h *Host, key SessionKey) (string, bool) {
	if isHostOffline(h) {
		return "offline", true
	}
	agent := memberAgent(h, key)
	if agent == "" {
		return "", false
	}
	lp := limitsOf(h)
	if lp.isRateLimited(agent, h.Now().UnixMilli()) {
		return "rate_limit", true
	}
	if agentReachedCap(h, lp, agent, key.Task) {
		return "cap", true
	}
	return "", false
}

func memberAgent(h *Host, key SessionKey) string {
	if s, ok := h.Sessions.Get(key); ok {
		return s.Agent
	}
	if s, ok := h.Sessions.Get(SessionKey{Group: key.Group, Member: key.Member}); ok {
		return s.Agent
	}
	if ai, local, _ := lookupLocalAI(h, key.Group, key.Member); local {
		return ai.agent
	}
	return ""
}

func (lp *limitsPart) isRateLimited(agent string, nowMS int64) bool {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	return nowMS < lp.rateUntil[agent]
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
	case RateLimited:
		onRateLimitedEvent(h, lp, s, e.Resets)
	case TurnEnded:
		onTurnEndedEvent(h, lp, s, e.Usage)
	default:
		onTurnActivity(lp, s, e)
	}
}

func onTurnActivity(lp *limitsPart, s Session, e Event) {
	if e.Kind == Active || e.Kind == Asked || e.Kind == Used {
		resetSilentWatch(lp, s.SessionKey)
	}
	if e.Kind == Used {
		lp.recordWindow(s.Agent, e.Usage)
	}
}

func startTurnWatch(h *Host, lp *limitsPart, s Session) {
	limit := groupToolTimeout(h, s.Group)
	lp.mu.Lock()
	if prev := lp.turns[s.SessionKey]; prev != nil {
		stopWatchTimers(prev)
	}
	delete(lp.pendingEv, s.SessionKey)
	lp.seq++
	gen := lp.seq
	tw := &turnWatch{
		gen:      gen,
		deadline: time.AfterFunc(limit, func() { fireTurnDeadline(h, lp, s.SessionKey, gen) }),
	}
	if s.Agent == "codex" {
		tw.silent = time.AfterFunc(codexSilentLimit, func() { fireCodexSilent(h, lp, s.SessionKey, gen) })
	}
	lp.turns[s.SessionKey] = tw
	lp.mu.Unlock()
}

func resetSilentWatch(lp *limitsPart, key SessionKey) {
	lp.mu.Lock()
	if tw := lp.turns[key]; tw != nil && tw.silent != nil {
		tw.silent.Reset(codexSilentLimit)
	}
	lp.mu.Unlock()
}

func fireTurnDeadline(h *Host, lp *limitsPart, key SessionKey, gen uint64) {
	if lp.hasTurnGen(key, gen) {
		interruptSessionTurn(h, key, "timeout", "Turn time limit reached.")
	}
}

func fireCodexSilent(h *Host, lp *limitsPart, key SessionKey, gen uint64) {
	if !lp.hasTurnGen(key, gen) {
		return
	}
	if h.Sessions.Calls(key) > 0 {
		resetSilentWatch(lp, key)
		return
	}
	interruptSessionTurn(h, key, "timeout", "Turn was silent for 5 minutes.")
}

func (lp *limitsPart) hasTurnGen(key SessionKey, gen uint64) bool {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	tw := lp.turns[key]
	return tw != nil && tw.gen == gen
}

func (lp *limitsPart) recordWindow(agent string, u Usage) *float64 {
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
	return cloneCodexPtr(cur.pct)
}

func (lp *limitsPart) getWindow(agent string) windowState {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	cur := lp.windows[agent]
	return windowState{pct: cloneCodexPtr(cur.pct), resets: cloneCodexPtr(cur.resets)}
}

func onRateLimitedEvent(h *Host, lp *limitsPart, s Session, resets int64) {
	nowMS := h.Now().UnixMilli()
	ev := proto.RateLimitedEvent{SystemCode: proto.SystemCode{Code: "rate_limited"}, Resets: resets}
	if !lp.recordRateLimit(h, s, resets, nowMS, ev) {
		rt := stateOf(h)
		lp.wg.Go(func() { sendMemberStatus(context.Background(), h, rt, s.Home, s.Group, s.Member, ev) })
	}
}

func (lp *limitsPart) recordRateLimit(h *Host, s Session, resets, nowMS int64, ev proto.RateLimitedEvent) bool {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	if prev := lp.rateTimers[s.Agent]; prev != nil {
		prev.Stop()
		delete(lp.rateTimers, s.Agent)
	}
	if resets > nowMS {
		lp.rateUntil[s.Agent] = resets
		wait := time.Duration(resets-nowMS) * time.Millisecond
		lp.rateTimers[s.Agent] = time.AfterFunc(wait, func() { releaseAgentHold(h, lp, s.Agent, true) })
	}
	inTurn := lp.turns[s.SessionKey] != nil
	if inTurn {
		lp.pendingEv[s.SessionKey] = ev
	}
	return inTurn
}

func releaseAgentHold(h *Host, lp *limitsPart, agent string, clearRate bool) {
	lp.mu.Lock()
	if clearRate {
		delete(lp.rateUntil, agent)
		delete(lp.rateTimers, agent)
	} else {
		delete(lp.dayTimers, agent)
	}
	lp.mu.Unlock()
	rt := stateOf(h)
	slots := distinctMemberSlots(h, agent)
	for _, slot := range slots {
		sendMemberStatus(context.Background(), h, rt, slot.home, slot.group, slot.member, nil)
	}
	triggerDeliverAfterHold(h, lp, slots)
}

func onTurnEndedEvent(h *Host, lp *limitsPart, s Session, u Usage) {
	stopTurnWatch(h, s.SessionKey)
	winPct := lp.recordWindow(s.Agent, u)
	now := h.Now()
	capKind := recordTurnUsage(h, s.Agent, now.Format(time.DateOnly), s.Task, u, winPct)
	if capKind == "" {
		return
	}
	ev := proto.CapReachedEvent{SystemCode: proto.SystemCode{Code: "cap_reached"}, Cap: capKind}
	if capKind == "task" {
		ev.Task = proto.ID(s.Task)
	} else {
		armDayCapTimer(h, lp, s.Agent, now)
	}
	lp.queueCapEventIfEmpty(s.SessionKey, ev)
}

func (lp *limitsPart) queueCapEventIfEmpty(key SessionKey, ev proto.CapReachedEvent) {
	lp.mu.Lock()
	if lp.pendingEv[key] == nil {
		lp.pendingEv[key] = ev
	}
	lp.mu.Unlock()
}

func armDayCapTimer(h *Host, lp *limitsPart, agent string, now time.Time) {
	wait := nextLocalMidnight(now).Sub(now)
	lp.mu.Lock()
	if prev := lp.dayTimers[agent]; prev != nil {
		prev.Stop()
	}
	lp.dayTimers[agent] = time.AfterFunc(wait, func() { releaseAgentHold(h, lp, agent, false) })
	lp.mu.Unlock()
}

func nextLocalMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}

func recordTurnUsage(h *Host, agent, today, taskID string, u Usage, winPct *float64) string {
	var hit string
	write := func(tx *store.Tx) error {
		caps, err := readAgentCapsTx(tx, agent)
		if err != nil {
			return err
		}
		days, err := readAgentUsageTx(tx, agent)
		if err != nil {
			return err
		}
		wasDay := isCapMet(caps.Day, days[today].usageTotals, winPct)
		wasTask := taskID != "" && isCapMet(caps.Task, sumTaskUsage(days, taskID), winPct)
		cur := applyTurnDelta(days[today], taskID, u)
		days[today] = cur
		_, err = tx.Exec("INSERT OR REPLACE INTO usage(agent, day, data) VALUES(?, ?, ?)", agent, today, marshalWire(cur))
		if err == nil {
			hit = newlyReachedCap(caps, cur.usageTotals, sumTaskUsage(days, taskID), taskID, winPct, wasDay, wasTask)
		}
		return err
	}
	h.Store.Write(write)
	return hit
}

func applyTurnDelta(day storedDayUsage, taskID string, u Usage) storedDayUsage {
	day.usageTotals = addUsageDelta(day.usageTotals, u)
	if taskID != "" {
		if day.Tasks == nil {
			day.Tasks = map[string]usageTotals{}
		}
		day.Tasks[taskID] = addUsageDelta(day.Tasks[taskID], u)
	}
	return day
}

func addUsageDelta(tot usageTotals, u Usage) usageTotals {
	tot.Turns++
	if u.Tokens != nil {
		tot.Tokens += *u.Tokens
		tot.HasTokens = true
	}
	if u.USD != nil {
		tot.USD += *u.USD
		tot.HasUSD = true
	}
	return tot
}

func newlyReachedCap(caps proto.Caps, dayTot, taskTot usageTotals, taskID string, winPct *float64, wasDay, wasTask bool) string {
	if !wasDay && isCapMet(caps.Day, dayTot, winPct) {
		return "day"
	}
	if taskID != "" && !wasTask && isCapMet(caps.Task, taskTot, winPct) {
		return "task"
	}
	return ""
}

func loadAgentCapsAndUsage(h *Host, agent string) (proto.Caps, map[string]storedDayUsage, bool) {
	var caps proto.Caps
	var days map[string]storedDayUsage
	read := func(tx *store.Tx) error {
		var err error
		caps, err = readAgentCapsTx(tx, agent)
		if err != nil {
			return err
		}
		days, err = readAgentUsageTx(tx, agent)
		return err
	}
	return caps, days, h.Store.Write(read) == nil
}

func agentReachedCap(h *Host, lp *limitsPart, agent, taskID string) bool {
	caps, days, ok := loadAgentCapsAndUsage(h, agent)
	if !ok {
		return false
	}
	win := lp.getWindow(agent)
	today := h.Now().Format(time.DateOnly)
	return isCapMet(caps.Day, days[today].usageTotals, win.pct) ||
		(taskID != "" && isCapMet(caps.Task, sumTaskUsage(days, taskID), win.pct))
}

func memberUsage(h *Host, group, member string) proto.Usage {
	agent := memberAgent(h, SessionKey{Group: group, Member: member})
	if agent == "" {
		return proto.Usage{}
	}
	caps, days, ok := loadAgentCapsAndUsage(h, agent)
	if !ok {
		return proto.Usage{}
	}
	win := limitsOf(h).getWindow(agent)
	day, hasDay := days[h.Now().Format(time.DateOnly)]
	if !hasAnyUsage(hasDay, win) {
		return proto.Usage{}
	}
	return buildProtoUsage(caps.Day, day.usageTotals, hasDay, win)
}

func hasAnyUsage(hasDay bool, win windowState) bool {
	return hasDay || win.pct != nil || win.resets != nil
}

func buildProtoUsage(dayCap *proto.Cap, dayTot usageTotals, hasDay bool, win windowState) proto.Usage {
	out := proto.Usage{WindowPct: win.pct, WindowResets: win.resets}
	if hasDay && dayTot.Turns > 0 {
		out.TurnsToday = new(dayTot.Turns)
	}
	if dayTot.HasUSD {
		out.SpendToday = new(dayTot.USD)
	}
	if dayCap != nil && dayCap.Max > 0 {
		fillDayCapUsage(&out, *dayCap, dayTot, win.pct)
	}
	return out
}

func fillDayCapUsage(out *proto.Usage, dayCap proto.Cap, dayTot usageTotals, winPct *float64) {
	if dayCap.Unit == "usd" {
		out.SpendCap = new(dayCap.Max)
	}
	if val, ok := usageValue(dayCap.Unit, dayTot, winPct); ok {
		out.DayCapPct = new(val / dayCap.Max * 100)
	}
}

func readAgentCapsTx(tx *store.Tx, agent string) (proto.Caps, error) {
	var raw string
	if err := tx.QueryRow(queryCapsSQL, agent).Scan(&raw); err != nil {
		return proto.Caps{}, err
	}
	var caps proto.Caps
	err := json.Unmarshal([]byte(raw), &caps)
	return caps, err
}

func readAgentUsageTx(tx *store.Tx, agent string) (map[string]storedDayUsage, error) {
	rows, err := tx.Query("SELECT day, data FROM usage WHERE agent = ?", agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := map[string]storedDayUsage{}
	for rows.Next() {
		var day, raw string
		rows.Scan(&day, &raw)
		var parsed storedDayUsage
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, err
		}
		days[day] = parsed
	}
	return days, rows.Err()
}

func sumTaskUsage(days map[string]storedDayUsage, taskID string) usageTotals {
	var sum usageTotals
	for _, d := range days {
		if t, ok := d.Tasks[taskID]; ok {
			sum.Turns += t.Turns
			sum.Tokens += t.Tokens
			sum.HasTokens = sum.HasTokens || t.HasTokens
			sum.USD += t.USD
			sum.HasUSD = sum.HasUSD || t.HasUSD
		}
	}
	return sum
}

func isCapMet(c *proto.Cap, u usageTotals, winPct *float64) bool {
	if c == nil || c.Max <= 0 {
		return false
	}
	val, ok := usageValue(c.Unit, u, winPct)
	return ok && val >= c.Max
}

func usageValue(unit string, u usageTotals, winPct *float64) (float64, bool) {
	if unit == "window_pct" {
		if winPct == nil {
			return 0, false
		}
		return *winPct, true
	}
	switch unit {
	case "turns":
		return float64(u.Turns), u.Turns > 0
	case "tokens":
		return float64(u.Tokens), u.HasTokens
	case "usd":
		return u.USD, u.HasUSD
	default:
		return 0, false
	}
}
