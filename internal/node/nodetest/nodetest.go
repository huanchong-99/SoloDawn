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
	"testing"
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

// Start opens a cluster of in-process nodes, prepends the fake CLIs to PATH,
// sets SOLODAWN_FAKECLI_LOG, and registers Close on t. When cfg.Root is empty
// t.TempDir() is used, and when cfg.Nodes is below 1 a single node is started.
func Start(t testing.TB, cfg Config) *Cluster {
	t.Helper()
	cfg = withDefaults(t, cfg)
	binDir := prepareFakeCLIs(t, cfg.Root, cfg.BinDir)
	keys, dirs := loadNodeKeys(t, cfg.Root, cfg.Nodes)
	owner := newKey()
	entries := buildOwnerLog(owner, keys, cfg.Now().UnixMilli())
	c := &Cluster{Root: cfg.Root, BinDir: binDir, LogPath: filepath.Join(cfg.Root, "fakecli.log"), Owner: owner, entries: entries}
	t.Cleanup(c.Close)
	for i, dir := range dirs {
		c.Nodes = append(c.Nodes, c.openNode(t, cfg, nodeName(i), dir, keys[i]))
	}
	t.Setenv("PATH", c.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", c.LogPath)
	return c
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
func (c *Cluster) SetScript(t testing.TB, vendor, path string) {
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

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func withDefaults(t testing.TB, cfg Config) Config {
	if cfg.Root == "" {
		cfg.Root = t.TempDir()
	}
	cfg.Nodes = max(1, cfg.Nodes)
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Tables.Commands == nil {
		cfg.Tables = home.NodeTables()
	}
	return cfg
}

func prepareFakeCLIs(t testing.TB, root, binDir string) string {
	if binDir != "" {
		return binDir
	}
	out := filepath.Join(root, "bin")
	must(t, exec.Command("go", "build", "-o", out+string(os.PathSeparator), claudePkg, codexPkg).Run())
	return out
}

func nodeName(i int) string { return fmt.Sprintf("node-%d", i+1) }

func loadNodeKeys(t testing.TB, root string, count int) ([]identity.Key, []string) {
	keys, dirs := make([]identity.Key, count), make([]string, count)
	for i := range count {
		dir := filepath.Join(root, nodeName(i))
		must(t, os.MkdirAll(dir, 0o700))
		key, err := identity.LoadKey(dir)
		must(t, err)
		keys[i], dirs[i] = key, dir
	}
	return keys, dirs
}

func (c *Cluster) openNode(t testing.TB, cfg Config, name, dir string, key identity.Key) *Node {
	db, err := store.Open(filepath.Join(dir, "solodawn.db"))
	must(t, err)
	must(t, seedOwner(db, c.Owner.ID(), c.entries))
	loopCfg := home.LoopConfig{Store: db, Home: key.ID(), Owner: c.Owner.ID(), Logs: c.Logs, Tables: cfg.Tables, Now: cfg.Now, Report: cfg.Report}
	loop, err := home.StartLoop(loopCfg)
	must(t, err)
	h, _, err := host.New(db, key, cfg.Now)
	must(t, err)
	return &Node{Name: name, Dir: dir, Key: key, Store: db, Home: loop, Host: h, Link: node.AttachLocal(h, loop)}
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
