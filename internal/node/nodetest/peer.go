package nodetest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// PeerConfig configures an in-process PeerNode connected to a rendezvous URL.
type PeerConfig struct {
	Ctx           context.Context
	Dir           string
	Key           identity.Key
	Name          string
	Version       string
	Owner         identity.Key
	ExtraBrowsers []identity.Key
	Rendezvous    string
	UDPPort       int
	TCPPort       int
	Now           func() time.Time
	MapRoute      func(peer.Route) peer.Route
	OnInbound     func(*peer.Connection)
}

// PeerNode is an in-process node assembled with Home, Host, LocalLink,
// Sessions, Surface, Pairer, Peer and an active rendezvous Path.
type PeerNode struct {
	Key      identity.Key
	Store    *store.Store
	Logs     *node.Logs
	Home     *home.Loop
	Host     *host.Host
	Link     *node.LocalLink
	Sessions *node.Sessions
	Surface  *node.Surface
	Pairer   *node.Pairer
	Sockets  *peer.Sockets
	Peer     *peer.Peer
	Path     *peer.Path
	once     sync.Once
}

// StartPeerNode opens an owned or unowned in-process node, dials cfg.Rendezvous,
// starts serving WebRTC signalling and pairing events, and registers Close on t.
func StartPeerNode(t testing.TB, cfg PeerConfig) *PeerNode {
	t.Helper()
	cfg = withPeerDefaults(cfg)
	key, db := openPeerStore(t, cfg)
	must(t, seedRendezvous(db, cfg.Rendezvous))
	if cfg.Owner != nil {
		entries := buildPeerOwnerLog(cfg.Owner, key, cfg.Name, cfg.ExtraBrowsers, cfg.Now().UnixMilli())
		must(t, seedOwner(db, cfg.Owner.ID(), cfg.Name, entries))
	}
	pn := &PeerNode{Key: key, Store: db}
	pn.initCore(t, cfg)
	pn.initTransport(t, cfg)
	t.Cleanup(pn.Close)
	return pn
}

// Close shuts down the rendezvous path, pairer, sockets, local link, home loop,
// host background work, and SQLite store. Calling Close more than once is safe.
func (n *PeerNode) Close() {
	n.once.Do(func() {
		n.Path.Close()
		n.Pairer.Close()
		n.Sockets.Close()
		n.Link.Close()
		n.Home.Close()
		n.Host.Settle()
		n.Store.Close()
	})
}

func withPeerDefaults(cfg PeerConfig) PeerConfig {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.Name == "" {
		cfg.Name = "node"
	}
	if cfg.MapRoute == nil {
		cfg.MapRoute = func(r peer.Route) peer.Route { return r }
	}
	return cfg
}

func openPeerStore(t testing.TB, cfg PeerConfig) (identity.Key, *store.Store) {
	if cfg.Dir == "" {
		key := cfg.Key
		if key == nil {
			key = newKey()
		}
		db, err := store.Open(":memory:")
		must(t, err)
		return key, db
	}
	must(t, os.MkdirAll(cfg.Dir, 0o700))
	key := cfg.Key
	if key == nil {
		loaded, err := identity.LoadKey(cfg.Dir)
		must(t, err)
		key = loaded
	}
	db, err := store.Open(filepath.Join(cfg.Dir, "node.db"))
	must(t, err)
	return key, db
}

func seedRendezvous(db *store.Store, rvURL string) error {
	rawRV, _ := json.Marshal([]string{rvURL})
	write := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES('rendezvous', ?)`, string(rawRV))
		return err
	}
	return db.Write(write)
}

func buildPeerOwnerLog(owner, nodeKey identity.Key, name string, browsers []identity.Key, ts int64) []proto.Envelope {
	entries := []proto.Envelope{
		SignDeviceAdd(owner, owner.ID(), 1, "browser", "owner", owner.ID(), ts),
		SignDeviceAdd(owner, owner.ID(), 2, "node", name, nodeKey.ID(), ts),
	}
	for i, key := range browsers {
		entries = append(entries, SignDeviceAdd(owner, owner.ID(), len(entries)+1, "browser", nodeName(i), key.ID(), ts))
	}
	return entries
}

func (n *PeerNode) initCore(t testing.TB, cfg PeerConfig) {
	logs, err := node.OpenLogs(n.Store, n.Key.ID())
	must(t, err)
	loop, err := home.StartLoop(home.LoopConfig{
		Store: n.Store, Home: n.Key.ID(), OwnerFn: logs.Owner,
		Logs: logs.Log, Tables: home.NodeTables(), Now: cfg.Now,
	})
	must(t, err)
	h, _, err := host.New(n.Store, n.Key, cfg.Now)
	must(t, err)
	n.Logs, n.Home, n.Host, n.Link = logs, loop, h, node.AttachLocal(h, loop)
	nameFn := func() string { return node.ReadName(n.Store) }
	addrsFn := func() []string { return []string{cfg.Rendezvous} }
	n.Sessions = node.NewSessions(node.SessionConfig{
		ID: n.Key.ID(), Version: cfg.Version, Store: n.Store, Logs: logs,
		Home: loop, Host: h, Name: nameFn, Addrs: addrsFn, Now: cfg.Now,
	})
	n.Surface = node.NewSurface(node.SurfaceConfig{
		ID: n.Key.ID(), OS: runtime.GOOS + "/" + runtime.GOARCH, Version: cfg.Version,
		Store: n.Store, Sessions: n.Sessions, Host: h, Name: nameFn, Agents: h.Agents,
	})
}

func (n *PeerNode) initTransport(t testing.TB, cfg PeerConfig) {
	socks, err := peer.Listen(cfg.UDPPort, cfg.TCPPort)
	must(t, err)
	rvPath, err := peer.Dial(cfg.Ctx, cfg.Rendezvous, n.Key)
	must(t, err)
	n.Sockets, n.Path = socks, rvPath
	n.Pairer = node.NewPairer(node.PairConfig{
		Key: n.Key, Store: n.Store, Logs: n.Logs, Sessions: n.Sessions,
		Paths: func() []*peer.Path { return []*peer.Path{rvPath} },
		Now:   cfg.Now,
	})
	known := func(dev string) bool { return n.Logs.Known(dev) || n.Pairer.Known(dev) }
	routes := func(dev string) []peer.Route { return n.routesFor(dev, cfg.MapRoute) }
	inbound := func(c *peer.Connection) {
		if cfg.OnInbound != nil {
			cfg.OnInbound(c)
		}
		go n.Pairer.Serve(cfg.Ctx, c.Device, c)
	}
	n.Peer = peer.New(n.Key, socks, n.Store, known, routes, inbound, nil)
	n.Pairer.Peer = n.Peer
	event := func(kind string, data []byte) error {
		n.Pairer.HandleEvent(cfg.Ctx, rvPath, kind, data)
		return nil
	}
	go rvPath.Serve(cfg.Ctx, n.Peer, event)
}

func (n *PeerNode) routesFor(dev string, mapRoute func(peer.Route) peer.Route) []peer.Route {
	out := []peer.Route{mapRoute(n.Path.Route())}
	for _, r := range n.Pairer.Routes(dev) {
		out = append(out, mapRoute(r))
	}
	return out
}
