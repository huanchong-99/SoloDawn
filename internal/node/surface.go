package node

import (
	"cmp"
	"context"
	"encoding/json"
	"log"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var srfCapUnits = []string{"turns", "tokens", "usd", "window_pct"}

type (
	// SurfaceConfig configures the node-level host surface handlers on Sessions.
	SurfaceConfig struct {
		ID, OS, Version string
		Store           *store.Store
		Logs            *Logs
		Sessions        *Sessions
		Host            *host.Host
		Name            func() string
		Agents          func(context.Context) ([]proto.HostAgent, error)
		Update          func() *proto.Update
		Relay           func() *proto.RelayUse
		Queued          func(group, member string) int
		Now             func() time.Time
		Report          func(error)
	}
	// Surface serves host.info and settings.set on a Sessions hub and pushes
	// host.info updates to connected owner devices.
	Surface struct {
		cfg SurfaceConfig
	}
	srfPerm struct {
		level      int
		repoConfig bool
	}
	srfDBState struct {
		name     string
		maxOpen  int
		offline  bool
		settings proto.HostSettings
		homes    []proto.Home
		perms    map[string]srfPerm
		agents   []proto.HostAgent
	}
	srfPlan struct {
		settings, caps [][2]string
		details        []string
		approvers      *[]string
	}
	srfScanner interface{ Scan(...any) error }
)

// NewSurface constructs a Surface and registers host.info and settings.set on cfg.Sessions.
func NewSurface(cfg SurfaceConfig) *Surface {
	sf := &Surface{cfg: srfDefaults(cfg)}
	cfg.Sessions.Requests["host.info"] = RequestHandler{Type: "host.info", Run: sf.reqHostInfo}
	cfg.Sessions.Commands["settings.set"] = sf.cmdSettingsSet
	return sf
}

func srfDefaults(cfg SurfaceConfig) SurfaceConfig {
	cfg.OS = cmp.Or(cfg.OS, runtime.GOOS+"/"+runtime.GOARCH)
	cfg.Version = cmp.Or(cfg.Version, Version)
	if cfg.Update == nil {
		cfg.Update = func() *proto.Update { return nil }
	}
	if cfg.Relay == nil {
		cfg.Relay = func() *proto.RelayUse { return nil }
	}
	if cfg.Queued == nil {
		cfg.Queued = func(string, string) int { return 0 }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Report == nil {
		cfg.Report = func(err error) { log.Printf("node: %v", err) }
	}
	return cfg
}

func readSetting[T any](db *store.Store, key, def string) T {
	var raw string
	var out T
	db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT value FROM setting WHERE key = ?), ?)", key, def).Scan(&raw)
	})
	json.Unmarshal([]byte(raw), &out)
	return out
}

// ReadName reads the node's display name from setting('name'), or returns "".
func ReadName(db *store.Store) string { return readSetting[string](db, "name", `""`) }

// ReadRendezvous reads the configured rendezvous URLs from setting('rendezvous').
func ReadRendezvous(db *store.Store) []string { return readSetting[[]string](db, "rendezvous", "[]") }

// HostState builds the current host.info state snapshot (datachannel.md 7).
func (sf *Surface) HostState(ctx context.Context) (proto.HostState, error) {
	db, err := sf.readDBState()
	if err != nil {
		return proto.HostState{}, err
	}
	agents := db.agents
	if sf.cfg.Agents != nil {
		if agents, err = sf.cfg.Agents(ctx); err != nil {
			return proto.HostState{}, err
		}
	}
	name := db.name
	if sf.cfg.Name != nil {
		name = sf.cfg.Name()
	}
	sessions := sf.cfg.Host.Sessions.All()
	for i := range agents {
		agents[i].Groups = sf.groupsForAgent(agents[i].Agent, sessions, db)
	}
	return proto.HostState{
		Device: sf.cfg.ID, Name: name, OS: sf.cfg.OS, Version: sf.cfg.Version,
		Open: srfCountOpen(sessions), MaxOpen: db.maxOpen, Offline: db.offline,
		Update: sf.cfg.Update(), Homes: db.homes, Relay: sf.cfg.Relay(),
		Agents: agents, Settings: db.settings,
	}, nil
}

// PushHostInfo sends a fresh host.info push to every welcomed owner connection.
func (sf *Surface) PushHostInfo(ctx context.Context) error {
	state, err := sf.HostState(ctx)
	if err != nil {
		return err
	}
	sf.cfg.Sessions.PushOwner(proto.HostInfoPush{Type: "host.info", HostState: state})
	return nil
}

func (sf *Surface) reqHostInfo(ctx context.Context, _ *home.Conn, _ Ctl, by string, _ int64, data json.RawMessage) (any, error) {
	var req proto.Request
	if err := proto.Decode(data, &req); err != nil {
		return nil, &host.Error{Code: "bad_request", Message: err.Error()}
	}
	if !sf.cfg.Logs.IsOwner(by) {
		return nil, &host.Error{Code: "forbidden", Message: "host.info is for owner devices only"}
	}
	return sf.HostState(ctx)
}

func srfCountOpen(sessions []host.Session) int {
	n := 0
	for _, s := range sessions {
		if s.State != host.Dormant {
			n++
		}
	}
	return n
}

func (sf *Surface) readDBState() (srfDBState, error) {
	out := srfDBState{
		maxOpen: 4, perms: map[string]srfPerm{}, agents: []proto.HostAgent{},
		settings: proto.HostSettings{RetentionDays: 90, IdleMinutes: 15, DefaultLevel: 3, Approvers: []string{}},
	}
	err := sf.cfg.Store.Write(func(tx *store.Tx) error { return sf.scanDBState(tx, &out) })
	return out, err
}

func (sf *Surface) scanDBState(tx *store.Tx, out *srfDBState) error {
	var savedHomes []proto.Home
	if err := srfScanSettings(tx, out, &savedHomes); err != nil {
		return err
	}
	var err error
	if out.homes, err = sf.scanHomes(tx, savedHomes); err != nil {
		return err
	}
	if err := srfScanPerms(tx, out.perms); err != nil {
		return err
	}
	if sf.cfg.Agents == nil {
		return srfScanAgents(tx, &out.agents)
	}
	return nil
}

func srfQueryRows(tx *store.Tx, sql string, each func(srfScanner) error) error {
	rows, err := tx.Query(sql)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := each(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func srfScanSettings(tx *store.Tx, out *srfDBState, savedHomes *[]proto.Home) error {
	targets := map[string]any{
		"name": &out.name, "max_open": &out.maxOpen, "offline": &out.offline,
		"retention_days": &out.settings.RetentionDays, "idle_minutes": &out.settings.IdleMinutes,
		"default_level": &out.settings.DefaultLevel, "all_level3": &out.settings.AllLevel3,
		"approvers": &out.settings.Approvers, "homes": savedHomes,
	}
	return srfQueryRows(tx, "SELECT key, value FROM setting", func(r srfScanner) error {
		var k, v string
		r.Scan(&k, &v)
		if dst := targets[k]; dst != nil {
			return json.Unmarshal([]byte(v), dst)
		}
		return nil
	})
}

func (sf *Surface) scanHomes(tx *store.Tx, saved []proto.Home) ([]proto.Home, error) {
	merged := append([]proto.Home{}, saved...)
	err := srfQueryRows(tx, "SELECT id, coalesce(data->>'name', '') FROM row WHERE r = 'group' AND gone = 0 ORDER BY id", func(r srfScanner) error {
		var id, name string
		r.Scan(&id, &name)
		merged = srfUpsertHome(merged, proto.Home{Group: proto.ID(id), Home: sf.cfg.ID, Name: name})
		return nil
	})
	return merged, err
}

func srfUpsertHome(list []proto.Home, h proto.Home) []proto.Home {
	match := func(x proto.Home) bool { return x.Group == h.Group }
	if i := slices.IndexFunc(list, match); i >= 0 {
		list[i] = h
		return list
	}
	return append(list, h)
}

func srfScanPerms(tx *store.Tx, perms map[string]srfPerm) error {
	return srfQueryRows(tx, "SELECT agent, g, level, repo_config FROM perm", func(r srfScanner) error {
		var agent, g string
		var level, repo int
		r.Scan(&agent, &g, &level, &repo)
		perms[agent+"\x00"+g] = srfPerm{level: level, repoConfig: repo != 0}
		return nil
	})
}

func srfScanAgents(tx *store.Tx, agents *[]proto.HostAgent) error {
	return srfQueryRows(tx, "SELECT agent, cli_version, account, caps FROM agent WHERE cli_version != '' ORDER BY agent", func(r srfScanner) error {
		var a proto.HostAgent
		var capsRaw string
		r.Scan(&a.Agent, &a.CLIVersion, &a.Account, &capsRaw)
		a.LoginOK, a.Groups = true, []proto.AgentGroup{}
		err := json.Unmarshal([]byte(capsRaw), &a.Caps)
		*agents = append(*agents, a)
		return err
	})
}

func (sf *Surface) groupsForAgent(agent string, sessions []host.Session, db srfDBState) []proto.AgentGroup {
	type slot struct{ group, member string }
	var order []slot
	grouped := map[slot][]host.Session{}
	for _, s := range sessions {
		if s.Agent == agent {
			k := slot{s.Group, s.Member}
			if _, seen := grouped[k]; !seen {
				order = append(order, k)
			}
			grouped[k] = append(grouped[k], s)
		}
	}
	out := make([]proto.AgentGroup, len(order))
	for i, k := range order {
		level, repoCfg := srfEffectivePerm(db, agent, k.group)
		state, task := sf.deriveAIState(k.group, k.member, grouped[k], db.offline)
		out[i] = proto.AgentGroup{
			Group: proto.ID(k.group), Member: proto.ID(k.member), Level: level,
			RepoConfig: repoCfg, State: state, Task: proto.ID(task), Queued: sf.cfg.Queued(k.group, k.member),
		}
	}
	return out
}

func srfEffectivePerm(db srfDBState, agent, group string) (int, bool) {
	p, ok := db.perms[agent+"\x00"+group]
	if db.settings.AllLevel3 {
		return 3, p.repoConfig
	}
	if ok {
		return p.level, p.repoConfig
	}
	return db.settings.DefaultLevel, false
}

func (sf *Surface) deriveAIState(group, member string, list []host.Session, offline bool) (string, string) {
	if task, ok := srfTaskInState(list, host.Takeover); ok {
		return "takeover", task
	}
	if task, ok := srfTaskInState(list, host.Working); ok {
		return "working", task
	}
	if s := sf.deriveOverlayState(group, member, offline); s != "" {
		return s, ""
	}
	if task, ok := srfTaskInState(list, host.Waiting); ok {
		return "waiting", task
	}
	return "dormant", ""
}

func srfTaskInState(list []host.Session, want host.State) (string, bool) {
	for _, s := range list {
		if s.State == want {
			return s.Task, true
		}
	}
	return "", false
}

func (sf *Surface) deriveOverlayState(group, member string, offline bool) string {
	if offline {
		return "offline"
	}
	if reason, held := sf.cfg.Host.Held(host.SessionKey{Group: group, Member: member}); held && reason == "limited" {
		return "limited"
	}
	if sf.groupPaused(group) {
		return "paused"
	}
	return ""
}

func (sf *Surface) groupPaused(group string) bool {
	raw, ok := sf.cfg.Host.Rows.Get(group, "group", group)
	if !ok {
		return false
	}
	var g proto.GroupRow
	return proto.Decode(raw, &g) == nil && g.Paused != nil
}

func (sf *Surface) cmdSettingsSet(ctx context.Context, _ Ctl, by string, payload json.RawMessage) (any, error) {
	plan, err := srfParseSettings(payload)
	if err != nil {
		return nil, err
	}
	if err := sf.writeSettings(plan, by); err != nil {
		return nil, err
	}
	if plan.approvers != nil {
		sf.propagateApprovers(ctx, *plan.approvers)
	}
	if err := sf.PushHostInfo(ctx); err != nil {
		sf.cfg.Report(err)
	}
	return nil, nil
}

func srfParseSettings(payload json.RawMessage) (srfPlan, error) {
	var cmd proto.SettingsSet
	if err := proto.Decode(payload, &cmd); err != nil {
		return srfPlan{}, &host.Error{Code: "bad_request", Message: err.Error()}
	}
	var plan srfPlan
	if err := srfPlanNumbers(cmd, &plan); err != nil {
		return srfPlan{}, err
	}
	if err := srfPlanOther(cmd, &plan); err != nil {
		return srfPlan{}, err
	}
	if len(plan.details) == 0 {
		return srfPlan{}, &host.Error{Code: "bad_request", Message: "settings.set changes no setting"}
	}
	return plan, nil
}

func srfPlanNumbers(cmd proto.SettingsSet, plan *srfPlan) error {
	for _, item := range []struct {
		key       string
		val       *int
		low, high int
	}{
		{"retention_days", cmd.RetentionDays, 1, math.MaxInt},
		{"idle_minutes", cmd.IdleMinutes, 1, math.MaxInt},
		{"max_open", cmd.MaxOpen, 1, math.MaxInt},
		{"default_level", cmd.DefaultLevel, 1, 3},
	} {
		if item.val == nil {
			continue
		}
		if *item.val < item.low || *item.val > item.high {
			return &host.Error{Code: "bad_request", Message: "invalid " + item.key}
		}
		text := strconv.Itoa(*item.val)
		plan.settings = append(plan.settings, [2]string{item.key, text})
		plan.details = append(plan.details, item.key+" "+text)
	}
	return nil
}

func srfPlanOther(cmd proto.SettingsSet, plan *srfPlan) error {
	if cmd.AllLevel3 != nil {
		text := strconv.FormatBool(*cmd.AllLevel3)
		plan.settings = append(plan.settings, [2]string{"all_level3", text})
		plan.details = append(plan.details, "all_level3 "+text)
	}
	if err := srfAddApprovers(cmd.Approvers, plan); err != nil {
		return err
	}
	return srfAddCaps(cmd.Caps, plan)
}

func srfAddApprovers(approvers *[]string, plan *srfPlan) error {
	if approvers == nil {
		return nil
	}
	list := []string{}
	for _, u := range *approvers {
		if !identity.IsDevice(u) {
			return &host.Error{Code: "bad_request", Message: "invalid approver user id"}
		}
		list = append(list, u)
	}
	plan.settings = append(plan.settings, [2]string{"approvers", string(dvlMustEncode(list))})
	plan.details = append(plan.details, "approvers")
	plan.approvers = &list
	return nil
}

func srfAddCaps(caps *proto.AgentCaps, plan *srfPlan) error {
	if caps == nil {
		return nil
	}
	if caps.Claude == nil && caps.Codex == nil {
		return &host.Error{Code: "bad_request", Message: "caps names no agent"}
	}
	if err := srfAddAgentCaps("claude", caps.Claude, plan); err != nil {
		return err
	}
	return srfAddAgentCaps("codex", caps.Codex, plan)
}

func srfAddAgentCaps(agent string, c *proto.Caps, plan *srfPlan) error {
	if c == nil {
		return nil
	}
	if !srfValidCap(c.Day) || !srfValidCap(c.Task) {
		return &host.Error{Code: "bad_request", Message: "invalid caps for " + agent}
	}
	plan.caps = append(plan.caps, [2]string{agent, string(dvlMustEncode(*c))})
	plan.details = append(plan.details, "caps."+agent)
	return nil
}

func srfValidCap(c *proto.Cap) bool {
	if c == nil {
		return true
	}
	return slices.Contains(srfCapUnits, c.Unit) && c.Max > 0
}

func (sf *Surface) writeSettings(plan srfPlan, by string) error {
	detail := strings.Join(plan.details, ", ")
	return sf.cfg.Store.Write(func(tx *store.Tx) error {
		for _, w := range plan.settings {
			if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", w[0], w[1]); err != nil {
				return err
			}
		}
		for _, w := range plan.caps {
			if _, err := tx.Exec("INSERT INTO agent(agent, cli_version, caps) VALUES(?, '', ?) ON CONFLICT(agent) DO UPDATE SET caps = excluded.caps", w[0], w[1]); err != nil {
				return err
			}
		}
		_, err := sf.cfg.Host.Audit(tx, host.AuditRow{What: host.AuditSetting, Detail: detail, By: by})
		return err
	})
}

func (sf *Surface) propagateApprovers(ctx context.Context, approvers []string) {
	seen := map[string]bool{}
	for _, s := range sf.cfg.Host.Sessions.All() {
		key := s.Group + "\x00" + s.Member
		if seen[key] {
			continue
		}
		seen[key] = true
		cmd := proto.MemberSet{
			MemberCommand: proto.MemberCommand{
				HomeCommand: proto.HomeCommand{Type: "member", Nonce: dvlNewID(), Timestamp: sf.cfg.Now().UnixMilli(), Group: proto.ID(s.Group)},
				Op:          "set", ID: proto.ID(s.Member),
			},
			Approvers: &approvers,
		}
		if _, err := sf.cfg.Host.Command(ctx, s.Home, string(dvlMustEncode(cmd))); err != nil {
			sf.cfg.Report(err)
		}
	}
}
