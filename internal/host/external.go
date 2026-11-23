package host

import (
	"context"
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	Register(Settled, "external", settleExternal)
}

const (
	externalAgent    = "external"
	externalPresence = 180 * time.Second
)

var externalAccounts = []string{"personal", "team", "api_key"}

//go:embed instructions/external.md
var externalInstructions string

type extRun struct {
	id    int64
	key   SessionKey
	timer *time.Timer
}

type extPart struct {
	take  sync.Mutex
	mu    sync.Mutex
	runs  map[aiGroupKey]*extRun
	open  map[aiGroupKey]int
	until map[aiGroupKey]time.Time
	away  map[aiGroupKey]*time.Timer
}

func extOf(h *Host) *extPart {
	ep := Part[extPart](h)
	ep.mu.Lock()
	if ep.runs == nil {
		ep.runs, ep.open = map[aiGroupKey]*extRun{}, map[aiGroupKey]int{}
		ep.until, ep.away = map[aiGroupKey]time.Time{}, map[aiGroupKey]*time.Timer{}
	}
	ep.mu.Unlock()
	return ep
}

func settleExternal(h *Host) {
	ep := extOf(h)
	ep.mu.Lock()
	defer ep.mu.Unlock()
	for _, r := range ep.runs {
		r.timer.Stop()
	}
	for _, t := range ep.away {
		t.Stop()
	}
}

func externalHostAgent() proto.HostAgent {
	return proto.HostAgent{
		Agent: externalAgent, LoginOK: true,
		Driver: proto.Driver{Accounts: externalAccounts, Takeover: "none"},
		Groups: []proto.AgentGroup{}, Models: []proto.Model{},
	}
}

// ExternalMember returns the group of the external AI of this node whose
// member id is member.
func (h *Host) ExternalMember(member string) (group string, ok bool) {
	for _, s := range h.Sessions.All() {
		if s.Member == member && s.Agent == externalAgent {
			return s.Group, true
		}
	}
	return "", false
}

// ExternalPresent notes a command or MCP connection of an external AI as
// open until leave is called; the AI stays present for 180 seconds after its
// last one ended (contracts/step-3/external.md section 6).
func (h *Host) ExternalPresent(group, member string) (leave func()) {
	key := aiGroupKey{group: group, member: member}
	ep := extOf(h)
	ep.mu.Lock()
	arrived := !ep.presentLocked(key)
	ep.open[key]++
	ep.mu.Unlock()
	if arrived {
		extReport(h, key)
	}
	return sync.OnceFunc(func() { extLeave(h, ep, key) })
}

func extLeave(h *Host, ep *extPart, key aiGroupKey) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if ep.open[key]--; ep.open[key] > 0 {
		return
	}
	if t := ep.away[key]; t != nil {
		t.Stop()
	}
	ep.until[key] = time.Now().Add(externalPresence)
	ep.away[key] = time.AfterFunc(externalPresence, func() { extReport(h, key) })
}

func (ep *extPart) presentLocked(key aiGroupKey) bool {
	return ep.open[key] > 0 || time.Now().Before(ep.until[key])
}

func extAbsent(h *Host, group, member string) bool {
	if memberAgent(h, group, member) != externalAgent {
		return false
	}
	ep := extOf(h)
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return !ep.presentLocked(aiGroupKey{group: group, member: member})
}

func extReport(h *Host, key aiGroupKey) {
	s, _ := h.Sessions.Get(SessionKey{Group: key.group, Member: key.member})
	sendMemberStatus(context.Background(), h, stateOf(h), s.Home, key.group, key.member, nil)
}

// ExternalTake hands out the next items of an external AI and opens its run
// (contracts/step-3/external.md section 3.2). It reports false when nothing
// can be handed out now.
func (h *Host) ExternalTake(group, member string) (proto.Handout, bool, error) {
	key := aiGroupKey{group: group, member: member}
	ep := extOf(h)
	ep.take.Lock()
	defer ep.take.Unlock()
	d := deliv(h)
	if extRunOpen(ep, key) || d.groupSyncActive(group) {
		return proto.Handout{}, false, nil
	}
	ai, _, _ := lookupLocalAI(h, group, member)
	target, batch, ok := takeEligibleBatch(h, d, key, ai)
	if !ok {
		return proto.Handout{}, false, nil
	}
	current, _ := filterStillCurrent(h, d, key, ai.agent, batch)
	if len(current) == 0 {
		return proto.Handout{}, false, nil
	}
	return extPrepare(h, ep, d, key, ai, target, current)
}

func extRunOpen(ep *extPart, key aiGroupKey) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.runs[key] != nil
}

func extPrepare(h *Host, ep *extPart, d *delivery, key aiGroupKey, ai aiInfo, target routedTarget, batch []verifiedItem) (proto.Handout, bool, error) {
	session, gitErr, err := prepareTargetSession(h, ai, target, batch[0].home, batch)
	if gitErr != nil && target.kind != GroupSession {
		failTargetSession(h, d, key, ai.agent, target, batch, gitErr)
		return proto.Handout{}, false, nil
	}
	if err = errors.Join(gitErr, err); err != nil {
		d.prependHeld(key, batch)
		return proto.Handout{}, false, err
	}
	return extHandOut(h, ep, d, key, session, batch)
}

func extHandOut(h *Host, ep *extPart, d *delivery, key aiGroupKey, s Session, batch []verifiedItem) (proto.Handout, bool, error) {
	raw, _ := collectRawItems(batch)
	text, _, textErr := Compose(h, s, ItemsTurn, raw)
	replaced, _, replErr := Compose(h, s, ReplacedTurn, nil)
	err := errors.Join(textErr, replErr)
	if err == nil {
		err = commitTurnItems(h, externalAgent, batch)
	}
	if err != nil {
		d.prependHeld(key, batch)
		return proto.Handout{}, false, err
	}
	ackBatch(h, batch)
	run := batch[0].item.Item
	extOpenRun(h, ep, key, s, run)
	return proto.Handout{
		Run: run, Kind: string(s.Kind), Task: s.Task, Folder: sessionDir(h, s),
		Context: Instructions(s) + "\n" + externalInstructions + "\n" + replaced, Text: text,
	}, true, nil
}

func extOpenRun(h *Host, ep *extPart, key aiGroupKey, s Session, run int64) {
	h.Sessions.Move(s.SessionKey, s.State, Working)
	ep.mu.Lock()
	ep.runs[key] = &extRun{id: run, key: s.SessionKey, timer: time.AfterFunc(groupToolTimeout(h, s.Group), func() {
		extEnd(h, key, run, "timeout", "")
	})}
	ep.mu.Unlock()
	extReport(h, key)
}

// ExternalDone ends the open run of an external AI when run is its number;
// failed, when not empty, says why the run failed (contracts/step-3/external.md
// section 3.3). Any other run changes nothing.
func (h *Host) ExternalDone(group, member string, run int64, failed string) {
	why := ""
	if failed != "" {
		why = "crash"
	}
	extEnd(h, aiGroupKey{group: group, member: member}, run, why, failed)
}

func extEnd(h *Host, key aiGroupKey, run int64, why, detail string) {
	ep := extOf(h)
	ep.mu.Lock()
	r := ep.runs[key]
	if r == nil || r.id != run {
		ep.mu.Unlock()
		return
	}
	r.timer.Stop()
	delete(ep.runs, key)
	ep.mu.Unlock()
	h.Sessions.Move(r.key, Working, Waiting)
	s, _ := h.Sessions.Get(r.key)
	reportFinishedTurn(h, stateOf(h), s, why, detail)
	Deliver(h, key.group, key.member)
}
