package nodetest_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	_ "modernc.org/sqlite"
)

const testGroupID = "BBBBBBBBBBBBBBBBBBBBBA"

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustEq(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func nonceID(n int64) string {
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], uint64(n))
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

type fakeTB struct {
	dir     string
	fatal   []any
	cleaned int
	env     map[string]string
}

func (f *fakeTB) Cleanup(fn func())        { f.cleaned++; fn() }
func (f *fakeTB) Fatal(args ...any)        { f.fatal = append(f.fatal, args...) }
func (f *fakeTB) Setenv(key, value string) { f.env[key] = value }
func (f *fakeTB) TempDir() string          { return f.dir }

func TestStartMultiNodeAndFakeCLIs(t *testing.T) {
	c := nodetest.Start(t, 2)
	mustEq(t, c.Root != "", true)
	mustEq(t, len(c.Nodes), 2)
	mustEq(t, []string{c.Nodes[0].Name, c.Nodes[1].Name}, []string{"node-1", "node-2"})
	mustEq(t, filepath.Base(c.Nodes[0].Dir), "node-1")
	mustEq(t, c.Nodes[0].Key.ID() != c.Nodes[1].Key.ID(), true)
	mustEq(t, c.Nodes[0].Host.Now().UnixMilli() > 0, true)

	checkOwnerLogs(t, c)
	for _, n := range c.Nodes {
		checkNodeSeeded(t, c, n)
	}
	checkFakeCLIs(t, c)
	checkCustomStartWith(t, c.BinDir)
}

func checkOwnerLogs(t *testing.T, c *nodetest.Cluster) {
	t.Helper()
	mustEq(t, c.Logs("stranger") == nil, true)
	ownerLog := c.Logs(c.Owner.ID())
	mustEq(t, ownerLog != nil, true)
	mustEq(t, ownerLog.Valid(c.Owner.ID()), true)
	mustEq(t, ownerLog.Valid(c.Nodes[0].Key.ID()), true)
	mustEq(t, ownerLog.Valid(c.Nodes[1].Key.ID()), true)
	mustEq(t, strings.Contains(ownerLog.Entries[1].Payload, `"name":"node-1"`), true)
	mustEq(t, strings.Contains(ownerLog.Entries[2].Payload, `"name":"node-2"`), true)
}

func checkNodeSeeded(t *testing.T, c *nodetest.Cluster, n *nodetest.Node) {
	t.Helper()
	mustEq(t, n.Link.Path(), "local")
	gotHome, ok := n.Host.Home(n.Key.ID())
	mustEq(t, []any{gotHome, ok}, []any{host.Home(n.Link), true})

	var storedOwner, verList string
	err := n.Store.Write(func(tx *store.Tx) error {
		if err := tx.QueryRow(`SELECT json_extract(value, '$') FROM setting WHERE key = 'owner'`).Scan(&storedOwner); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT json_group_array(ver) FROM (SELECT ver FROM device_log WHERE user = ? ORDER BY ver)`, c.Owner.ID()).Scan(&verList)
	})
	mustOK(t, err)
	mustEq(t, storedOwner, c.Owner.ID())
	mustEq(t, verList, "[1,2,3]")
}

func checkFakeCLIs(t *testing.T, c *nodetest.Cluster) {
	t.Helper()
	for _, vendor := range []string{"claude", "codex"} {
		resolved, err := exec.LookPath(vendor)
		mustOK(t, err)
		mustEq(t, filepath.Dir(resolved), c.BinDir)
		out, err := exec.Command(vendor, "--version").Output()
		mustOK(t, err)
		mustEq(t, len(strings.TrimSpace(string(out))) > 0, true)
	}

	script := filepath.Join(c.Root, "wait.json")
	mustOK(t, os.WriteFile(script, []byte(`{"session":"s-1","steps":[{"dir":"eof"}]}`), 0o600))
	c.SetScript(t, "claude", script)
	c.SetScript(t, "codex", script)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "claude", "-p", "--session-id", "s-1").CombinedOutput()
	mustOK(t, err)
	mustEq(t, string(out), "")
	out, err = exec.CommandContext(ctx, "codex", "app-server").CombinedOutput()
	mustOK(t, err)
	mustEq(t, string(out), "")

	logBytes, err := os.ReadFile(c.LogPath)
	mustOK(t, err)
	mustEq(t, string(logBytes), "start claude s-1\nexit claude s-1\nstart codex s-1\nexit codex s-1\n")
}

func checkCustomStartWith(t *testing.T, binDir string) {
	t.Helper()
	fixedMs := int64(1_763_700_000_000)
	customRoot := filepath.Join(t.TempDir(), "custom")
	single := nodetest.StartWith(t, nodetest.Config{
		Root:   customRoot,
		BinDir: binDir,
		Nodes:  1,
		Now:    func() time.Time { return time.UnixMilli(fixedMs) },
		Tables: home.Tables{Commands: home.Table{"group/create": singleGroupCreate}},
	})
	mustEq(t, []any{single.Root, single.BinDir, len(single.Nodes)}, []any{customRoot, binDir, 1})

	n0 := single.Nodes[0]
	mustEq(t, n0.Host.Now().UnixMilli(), fixedMs)
	payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"One"}`, nonceID(1), fixedMs, testGroupID)
	ans, err := n0.Host.Command(context.Background(), n0.Key.ID(), payload)
	mustOK(t, err)
	var res proto.CmdResult
	mustOK(t, proto.Decode(ans, &res))
	mustEq(t, res.Num, 3)

	single.Close()
	_, open := n0.Host.Home(n0.Key.ID())
	mustEq(t, open, false)

	tb := &fakeTB{dir: t.TempDir(), env: map[string]string{}}
	got := nodetest.StartWith(tb, nodetest.Config{Nodes: 0})
	mustEq(t, []any{got == nil, len(tb.fatal)}, []any{true, 1})
}

func singleGroupCreate(s *home.State, cmd home.Command) (home.Result, *home.Refusal) {
	grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "One", Checks: []string{}, Created: cmd.Now}
	mem := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m1"}, Group: s.ID, Kind: "human", Name: "Owner", User: s.Owner},
		Role:      "admin",
	}
	return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(mem)}, Answer: store.Outcome{Num: 3}}, nil
}

func mustFailOpen(t *testing.T, cfg nodetest.Config) error {
	t.Helper()
	c, err := nodetest.Open(cfg)
	mustEq(t, c == nil && err != nil, true)
	return err
}

func TestOpenErrorsAndPartialCleanup(t *testing.T) {
	sharedBins := t.TempDir()

	errZero := mustFailOpen(t, nodetest.Config{Root: t.TempDir(), BinDir: sharedBins, Nodes: 0})
	mustEq(t, errZero.Error(), "nodetest: cluster needs at least one node")

	rootFile := filepath.Join(t.TempDir(), "not-a-dir")
	mustOK(t, os.WriteFile(rootFile, []byte("x"), 0o600))
	mustFailOpen(t, nodetest.Config{Root: rootFile, Nodes: 1})

	rootBadDir := t.TempDir()
	mustOK(t, os.WriteFile(filepath.Join(rootBadDir, "node-1"), []byte("x"), 0o600))
	mustFailOpen(t, nodetest.Config{Root: rootBadDir, BinDir: sharedBins, Nodes: 1})

	rootBadKey := t.TempDir()
	mustOK(t, os.MkdirAll(filepath.Join(rootBadKey, "node-1"), 0o700))
	mustOK(t, os.WriteFile(filepath.Join(rootBadKey, "node-1", "device.key"), []byte("bad"), 0o600))
	mustFailOpen(t, nodetest.Config{Root: rootBadKey, BinDir: sharedBins, Nodes: 1})

	rootBadStore := filepath.Join(t.TempDir(), "partial")
	mustOK(t, os.MkdirAll(filepath.Join(rootBadStore, "node-2", "solodawn.db"), 0o700))
	mustFailOpen(t, nodetest.Config{Root: rootBadStore, BinDir: sharedBins, Nodes: 2})
	mustOK(t, os.RemoveAll(rootBadStore))

	for _, stmt := range []string{
		"DROP TABLE setting",
		`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('g1', 'group', 'g1', 1, 1, 1, 1, '{}')`,
		"DROP TABLE session",
	} {
		mustFailOpen(t, nodetest.Config{Root: prepareBrokenDB(t, stmt), BinDir: sharedBins, Nodes: 1})
	}
}

func prepareBrokenDB(t *testing.T, stmt string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "broken")
	nodeDir := filepath.Join(root, "node-1")
	mustOK(t, os.MkdirAll(nodeDir, 0o700))
	dbPath := filepath.Join(nodeDir, "solodawn.db")
	st, err := store.Open(dbPath)
	mustOK(t, err)
	mustOK(t, st.Close())

	rawDB, err := sql.Open("sqlite", dbPath)
	mustOK(t, err)
	_, execErr := rawDB.Exec(stmt)
	mustOK(t, rawDB.Close())
	mustOK(t, execErr)
	return root
}
