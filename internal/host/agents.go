package host

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	agtProbeSeconds     = 5
	agtUpsertFoundSQL   = `INSERT INTO agent(agent, cli_version) VALUES(?, ?) ON CONFLICT(agent) DO UPDATE SET cli_version = excluded.cli_version`
	agtUpdateAccountSQL = `UPDATE agent SET account = ? WHERE agent = ?`
	agtSelectAgentSQL   = `SELECT account, caps FROM agent WHERE agent = ?`
)

func init() {
	Register(Surface, "agent.set", agtSet)
	Register(Listeners, "agents", agtListen)
}

type agtFound struct {
	agent   string
	version string
	models  []proto.Model
}

// agtPart keeps the 60 s CLI detection cache and sign-in status for one Host.
// Lock order: probeMu -> Store.Write -> mu. Callers holding mu never wait on
// probeMu or Store.Write.
type agtPart struct {
	probeMu   sync.Mutex
	mu        sync.Mutex
	expiresAt time.Time
	found     []agtFound
	invalid   map[string]bool
}

func (p *agtPart) setLoginOK(agent string, ok bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := !p.invalid[agent]
	if ok {
		delete(p.invalid, agent)
	} else {
		if p.invalid == nil {
			p.invalid = map[string]bool{}
		}
		p.invalid[agent] = true
	}
	return prev != ok
}

func (p *agtPart) loginOK(agent string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.invalid[agent]
}

func (p *agtPart) cached(now time.Time) ([]agtFound, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !now.Before(p.expiresAt) {
		return nil, false
	}
	return append([]agtFound(nil), p.found...), true
}

func (p *agtPart) storeCache(expiresAt time.Time, found []agtFound) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expiresAt = expiresAt
	p.found = append([]agtFound(nil), found...)
}

// Agents detects the supported AI CLIs on PATH, keeps the answer for 60 s,
// persists found CLIs in the agent table, and returns their host.info entries
// (contracts/drivers.md section 1.4, contracts/datachannel.md section 7).
// Groups is an empty slice for the caller to fill from session and permission
// state. If ctx ends during a probe, Agents returns ctx.Err() bare (not an
// *Error) for the caller to map to a wire code.
func (h *Host) Agents(ctx context.Context, refresh bool) ([]proto.HostAgent, error) {
	found, err := agtDetect(ctx, h, refresh)
	if err != nil {
		return nil, err
	}
	agents, err := agtLoadAgents(h.Store, Part[agtPart](h), found)
	if err != nil {
		return nil, err
	}
	return append(agents, externalHostAgent()), nil
}

func agtDetect(ctx context.Context, h *Host, refresh bool) ([]agtFound, error) {
	part := Part[agtPart](h)
	part.probeMu.Lock()
	defer part.probeMu.Unlock()
	now := h.Now()
	if found, ok := part.cached(now); ok && !refresh {
		return found, nil
	}
	found, err := agtProbeAll(ctx)
	if err != nil {
		return nil, err
	}
	if err := agtPersistFound(h.Store, found); err != nil {
		return nil, err
	}
	part.storeCache(now.Add(time.Minute), found)
	return found, nil
}

func agtProbeAll(ctx context.Context) ([]agtFound, error) {
	var found []agtFound
	for _, agent := range driverNames() {
		version, ok, err := agtProbeOne(ctx, agent)
		if err != nil {
			return nil, err
		}
		if ok {
			found = append(found, agtFound{agent: agent, version: version, models: agtModels(ctx, agent)})
		}
	}
	return found, ctx.Err()
}

func agtProbeOne(ctx context.Context, agent string) (string, bool, error) {
	probeCtx, cancel := context.WithTimeout(ctx, agtProbeSeconds*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, agent, "--version")
	detachTerminal(cmd)
	cmd.WaitDelay = commandWaitDelay
	out, err := cmd.Output()
	if err != nil {
		return "", false, ctx.Err()
	}
	return agtCleanVersion(agent, string(out)), true, nil
}

func agtCleanVersion(agent, raw string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(raw), "\n")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 1 && strings.TrimSuffix(fields[0], "-cli") == agent {
		return fields[1]
	}
	return fields[0]
}

func agtPersistFound(db *store.Store, found []agtFound) error {
	if len(found) == 0 {
		return nil
	}
	write := func(tx *store.Tx) error {
		for _, f := range found {
			if _, err := tx.Exec(agtUpsertFoundSQL, f.agent, f.version); err != nil {
				return err
			}
		}
		return nil
	}
	return db.Write(write)
}

func agtLoadAgents(db *store.Store, part *agtPart, found []agtFound) ([]proto.HostAgent, error) {
	agents := make([]proto.HostAgent, 0, len(found))
	read := func(tx *store.Tx) error {
		defaults, err := readDefaultsTx(tx)
		if err != nil {
			return err
		}
		for _, f := range found {
			item, err := agtReadRow(tx, f, part.loginOK(f.agent), defaults)
			if err != nil {
				return err
			}
			agents = append(agents, item)
		}
		return nil
	}
	if err := db.Write(read); err != nil {
		return nil, err
	}
	return agents, nil
}

func agtReadRow(tx *store.Tx, f agtFound, loginOK bool, defaults map[string]proto.Profile) (proto.HostAgent, error) {
	var account *string
	var capsJSON string
	if err := tx.QueryRow(agtSelectAgentSQL, f.agent).Scan(&account, &capsJSON); err != nil {
		return proto.HostAgent{}, err
	}
	var caps proto.Caps
	if err := json.Unmarshal([]byte(capsJSON), &caps); err != nil {
		return proto.HostAgent{}, err
	}
	var def *proto.Profile
	if p, ok := defaults[f.agent]; ok {
		def = &p
	}
	return proto.HostAgent{
		Agent:      f.agent,
		CLIVersion: f.version,
		Account:    account,
		LoginOK:    loginOK,
		Caps:       caps,
		Driver:     agtDriverProto(f.agent),
		Groups:     []proto.AgentGroup{},
		Models:     f.models,
		Defaults:   def,
	}, nil
}

func agtDriverProto(agent string) proto.Driver {
	c := DriverCaps[agent]
	return proto.Driver{Accounts: c.Accounts, Takeover: c.Takeover}
}

func agtListen(h *Host, key SessionKey, e Event) {
	s, ok := h.Sessions.Get(key)
	if !ok {
		return
	}
	if loginOK, changed := agtLoginChange(e); changed && Part[agtPart](h).setLoginOK(s.Agent, loginOK) {
		notifyLoginChanged(h)
	}
}

func notifyLoginChanged(h *Host) {
	if h.LoginChanged != nil {
		h.LoginChanged()
	}
}

func agtLoginChange(e Event) (loginOK, changed bool) {
	switch e.Kind {
	case LoginInvalid:
		return false, true
	case TurnEnded:
		return true, e.Status != "failed"
	default:
		return false, false
	}
}

func agtSet(_ context.Context, h *Host, _ Owner, by string, data json.RawMessage) (any, error) {
	cmd, err := agtDecodeSet(data)
	if err != nil {
		return nil, err
	}
	if err := agtWriteAccount(h, cmd.Agent, cmd.Account, by); err != nil {
		return nil, err
	}
	if Part[agtPart](h).setLoginOK(cmd.Agent, true) {
		notifyLoginChanged(h)
	}
	return nil, nil
}

func agtDecodeSet(data json.RawMessage) (proto.AgentSet, error) {
	var cmd proto.AgentSet
	if err := proto.Decode(data, &cmd); err != nil {
		return proto.AgentSet{}, &Error{Code: "bad_request", Message: err.Error()}
	}
	if !agtValidAgent(cmd.Agent) {
		return proto.AgentSet{}, &Error{Code: "bad_request", Message: "unknown agent: " + cmd.Agent}
	}
	return cmd, checkDriverAccount(cmd.Agent, cmd.Account)
}

func agtValidAgent(agent string) bool {
	_, ok := DriverCaps[agent]
	return ok
}

func agtUpdateAccount(tx *store.Tx, agent, account string) (bool, error) {
	res, err := tx.Exec(agtUpdateAccountSQL, account, agent)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func agtWriteAccount(h *Host, agent, account, by string) error {
	write := func(tx *store.Tx) error {
		updated, err := agtUpdateAccount(tx, agent, account)
		if err != nil {
			return err
		}
		if !updated {
			return &Error{Code: "conflict", Message: agent + " is not found on this machine"}
		}
		_, err = h.Audit(tx, AuditRow{
			Agent:  agent,
			What:   AuditSetting,
			Detail: "account " + account,
			By:     by,
		})
		return err
	}
	return h.Store.Write(write)
}
