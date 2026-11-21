// Package nodetest starts in-process SoloDawn nodes wired over LocalLink with
// the fake Claude Code and Codex CLIs built and placed on PATH.
package nodetest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	claudePkg = "github.com/huanchong-99/SoloDawn/internal/host/fakecli/claude"
	codexPkg  = "github.com/huanchong-99/SoloDawn/internal/host/fakecli/codex"
)

var errNoNodes = errors.New("nodetest: cluster needs at least one node")

// TB is the subset of testing.TB that Start and StartWith use.
type TB interface {
	Cleanup(func())
	Fatal(args ...any)
	Setenv(key, value string)
	TempDir() string
}

// Config configures a Cluster of in-process nodes.
type Config struct {
	Root   string
	BinDir string
	Nodes  int
	Tables home.Tables
	Now    func() time.Time
	Report func(error)
}

// Cluster holds a set of in-process nodes enrolled under one owner, together
// with the built fake CLI binaries and shared lifecycle log path.
type Cluster struct {
	Root    string
	BinDir  string
	LogPath string
	Owner   identity.Key
	Nodes   []*Node
	entries []proto.Envelope
}

// Node is one in-process node with its data directory, store, device key, home
// loop, host and local link.
type Node struct {
	Name  string
	Dir   string
	Key   identity.Key
	Store *store.Store
	Home  *home.Loop
	Host  *host.Host
	Link  *node.LocalLink
	once  sync.Once
}

// Start opens a cluster of nodes in a temporary directory, prepends the fake
// CLIs to PATH, sets SOLODAWN_FAKECLI_LOG, and registers Close on t.
func Start(t TB, nodes int) *Cluster {
	return StartWith(t, Config{Nodes: nodes})
}

// StartWith is Start with explicit cluster configuration. When cfg.Root is
// empty, t.TempDir() is used.
func StartWith(t TB, cfg Config) *Cluster {
	if cfg.Root == "" {
		cfg.Root = t.TempDir()
	}
	c, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	t.Cleanup(c.Close)
	t.Setenv("PATH", c.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", c.LogPath)
	return c
}

// Open creates and starts the cluster described by cfg without modifying
// process environment variables. The caller closes the returned cluster.
func Open(cfg Config) (*Cluster, error) {
	if cfg.Nodes < 1 {
		return nil, errNoNodes
	}
	cfg = withDefaults(cfg)
	binDir, err := prepareFakeCLIs(cfg.Root, cfg.BinDir)
	if err != nil {
		return nil, err
	}
	keys, dirs, err := loadNodeKeys(cfg.Root, cfg.Nodes)
	if err != nil {
		return nil, err
	}
	owner := newKey()
	entries := buildOwnerLog(owner, keys, cfg.Now().UnixMilli())
	c := &Cluster{Root: cfg.Root, BinDir: binDir, LogPath: filepath.Join(cfg.Root, "fakecli.log"), Owner: owner, entries: entries}
	if err := c.startNodes(cfg, dirs, keys); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Logs returns a fresh verified copy of the owner's device log when user is
// the cluster owner, and nil for any other user.
func (c *Cluster) Logs(user string) *identity.Log {
	if user != c.Owner.ID() {
		return nil
	}
	log, _ := identity.Replay(user, c.entries)
	return log
}

// SetScript points the next start of vendor ("claude" or "codex") at path.
func (c *Cluster) SetScript(t TB, vendor, path string) {
	t.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", path)
}

// Close stops every node in the cluster.
func (c *Cluster) Close() {
	for _, n := range c.Nodes {
		n.Close()
	}
}

// Close detaches the local link, stops the home loop and closes the store.
// Calling Close more than once is safe.
func (n *Node) Close() {
	n.once.Do(func() {
		n.Link.Close()
		n.Home.Close()
		n.Store.Close()
	})
}

func withDefaults(cfg Config) Config {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Tables.Commands == nil {
		cfg.Tables = home.NodeTables()
	}
	return cfg
}

func prepareFakeCLIs(root, binDir string) (string, error) {
	if binDir != "" {
		return binDir, nil
	}
	out := filepath.Join(root, "bin")
	err := exec.Command("go", "build", "-o", out+string(os.PathSeparator), claudePkg, codexPkg).Run()
	return out, err
}

func nodeName(i int) string { return fmt.Sprintf("node-%d", i+1) }

func loadNodeKeys(root string, count int) ([]identity.Key, []string, error) {
	keys, dirs := make([]identity.Key, count), make([]string, count)
	for i := range count {
		dir := filepath.Join(root, nodeName(i))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, nil, err
		}
		key, err := identity.LoadKey(dir)
		if err != nil {
			return nil, nil, err
		}
		keys[i], dirs[i] = key, dir
	}
	return keys, dirs, nil
}

func (c *Cluster) startNodes(cfg Config, dirs []string, keys []identity.Key) error {
	for i, dir := range dirs {
		n, err := c.openNode(cfg, nodeName(i), dir, keys[i])
		if err != nil {
			return err
		}
		c.Nodes = append(c.Nodes, n)
	}
	return nil
}

func (c *Cluster) openNode(cfg Config, name, dir string, key identity.Key) (*Node, error) {
	db, err := store.Open(filepath.Join(dir, "solodawn.db"))
	if err != nil {
		return nil, err
	}
	if err := seedOwner(db, c.Owner.ID(), c.entries); err != nil {
		db.Close()
		return nil, err
	}
	loopCfg := home.LoopConfig{Store: db, Home: key.ID(), Owner: c.Owner.ID(), Logs: c.Logs, Tables: cfg.Tables, Now: cfg.Now, Report: cfg.Report}
	loop, err := home.StartLoop(loopCfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	h, _, err := host.New(db, key, cfg.Now)
	if err != nil {
		loop.Close()
		db.Close()
		return nil, err
	}
	return &Node{Name: name, Dir: dir, Key: key, Store: db, Home: loop, Host: h, Link: node.AttachLocal(h, loop)}, nil
}

func seedOwner(db *store.Store, owner string, entries []proto.Envelope) error {
	ownerJSON, _ := json.Marshal(owner)
	write := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES('owner', ?)`, string(ownerJSON))
		for i, env := range entries {
			raw, _ := json.Marshal(env)
			_, next := tx.Exec(`INSERT OR REPLACE INTO device_log(user, ver, env) VALUES(?, ?, ?)`, owner, i+1, string(raw))
			err = errors.Join(err, next)
		}
		return err
	}
	return db.Write(write)
}

func newKey() identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}

func randomID() proto.ID {
	var raw [16]byte
	rand.Read(raw[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

func buildOwnerLog(owner identity.Key, nodes []identity.Key, ts int64) []proto.Envelope {
	entries := []proto.Envelope{signDeviceAdd(owner, 1, "browser", "owner", owner.ID(), ts)}
	for i, key := range nodes {
		entries = append(entries, signDeviceAdd(owner, i+2, "node", nodeName(i), key.ID(), ts))
	}
	return entries
}

func signDeviceAdd(owner identity.Key, ver int, kind, name, dev string, ts int64) proto.Envelope {
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: randomID(), Timestamp: ts},
		DeviceChange:  proto.DeviceChange{User: owner.ID(), Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	}
	raw, _ := json.Marshal(entry)
	return owner.Sign(string(raw))
}
