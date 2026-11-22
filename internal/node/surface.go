package node

import (
	"cmp"
	"context"
	"encoding/json"
	"runtime"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type (
	// SurfaceConfig configures the node-level host surface on Sessions.
	// Store, Sessions, Host and Agents are required.
	SurfaceConfig struct {
		ID, OS, Version string
		Store           *store.Store
		Sessions        *Sessions
		Host            *host.Host
		Name            func() string
		Agents          func(context.Context) ([]proto.HostAgent, error)
		Update          func() *proto.Update
		Relay           func() *proto.RelayUse
	}
	// Surface serves host.info on a Sessions hub.
	Surface struct {
		cfg SurfaceConfig
	}
	srfDBState struct {
		name    string
		maxOpen int
		offline bool
		homes   []proto.Home
	}
)

// NewSurface constructs a Surface and registers host.info, pty.resize and takeover.start replay on cfg.Sessions.
func NewSurface(cfg SurfaceConfig) *Surface {
	sf := &Surface{cfg: srfDefaults(cfg)}
	cfg.Sessions.Requests["host.info"] = RequestHandler{Type: "host.info", Run: sf.reqHostInfo}
	cfg.Sessions.Requests["watch.start"] = RequestHandler{Run: sf.reqWatchStart}
	cfg.Sessions.Pushes["pty.resize"] = sf.pushPTYResize
	cfg.Sessions.Replays["takeover.start"] = sf.replayTakeoverStart
	return sf
}

func (sf *Surface) reqWatchStart(_ context.Context, _ *home.Conn, from Ctl, by string, _ int64, data json.RawMessage) (any, error) {
	return sf.cfg.Host.WatchStart(from, by, data)
}

func (sf *Surface) replayTakeoverStart(ctx context.Context, from Ctl, by string, data json.RawMessage) (any, error) {
	return sf.cfg.Host.ReplayTakeover(ctx, from, by, data)
}

func (sf *Surface) pushPTYResize(ctx context.Context, _ *home.Conn, from Ctl, by string, data json.RawMessage) {
	host.Surface["pty.resize"](ctx, sf.cfg.Host, from, by, data)
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
	return cfg
}

// readSetting returns the JSON-decoded value of key, or def when missing.
// Store and JSON errors are dropped so Name and Rendezvous callbacks return a
// safe default even before settings exist or after the store closes.
func readSetting[T any](db *store.Store, key, def string) T {
	var raw string
	var out T
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT value FROM setting WHERE key = ?), ?)", key, def).Scan(&raw)
	}
	db.Write(read)
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
	rawAgents, err := sf.cfg.Agents(ctx)
	if err != nil {
		return proto.HostState{}, err
	}
	name := db.name
	if sf.cfg.Name != nil {
		name = sf.cfg.Name()
	}
	sessions := sf.cfg.Host.Sessions.All()
	agents := make([]proto.HostAgent, len(rawAgents))
	for i, a := range rawAgents {
		a.Groups = sf.groupsForAgent(a.Agent, sessions)
		agents[i] = a
	}
	return proto.HostState{
		Device: sf.cfg.ID, Name: name, OS: sf.cfg.OS, Version: sf.cfg.Version,
		Open: srfCountOpen(sessions), MaxOpen: db.maxOpen, Offline: db.offline,
		Update: sf.cfg.Update(), Homes: db.homes, Relay: sf.cfg.Relay(),
		Agents:   agents,
		Settings: proto.HostSettings{RetentionDays: 90, IdleMinutes: 15, DefaultLevel: 3, Approvers: []string{}},
	}, nil
}

func (sf *Surface) reqHostInfo(ctx context.Context, _ *home.Conn, _ Ctl, _ string, _ int64, _ json.RawMessage) (any, error) {
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
	out := srfDBState{maxOpen: 4, homes: []proto.Home{}}
	targets := map[string]any{"name": &out.name, "max_open": &out.maxOpen, "offline": &out.offline, "homes": &out.homes}
	read := func(tx *store.Tx) error {
		rows, err := tx.Query("SELECT key, value FROM setting WHERE key IN ('name', 'max_open', 'offline', 'homes')")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			rows.Scan(&k, &v)
			if err := json.Unmarshal([]byte(v), targets[k]); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	return out, sf.cfg.Store.Write(read)
}

func (sf *Surface) groupsForAgent(agent string, sessions []host.Session) []proto.AgentGroup {
	type slot struct{ group, member string }
	seen := map[slot]bool{}
	out := []proto.AgentGroup{}
	for _, s := range sessions {
		k := slot{s.Group, s.Member}
		if s.Agent != agent || seen[k] {
			continue
		}
		seen[k] = true
		state, task := sf.cfg.Host.MemberState(k.group, k.member)
		out = append(out, proto.AgentGroup{
			Group: proto.ID(k.group), Member: proto.ID(k.member), Level: 3,
			State: state, Task: proto.ID(task), Queued: sf.cfg.Host.QueuedCount(k.group, k.member), Watchers: sf.cfg.Host.Watchers(k.group, k.member),
		})
	}
	return out
}
