package nodetest_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
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
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
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

func detKey(seedByte byte) identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = seedByte
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}

type fatalTB struct {
	testing.TB
	fatal bool
}

func (f *fatalTB) Fatal(...any) { f.fatal = true; panic(f) }

func TestStartMultiNodeAndFakeCLIs(t *testing.T) {
	extraBrowser := detKey(9)
	c := nodetest.Start(t, nodetest.Config{Nodes: 2, ExtraBrowsers: []identity.Key{extraBrowser}})
	mustEq(t, filepath.IsAbs(c.Root), true)
	mustEq(t, len(c.Nodes), 2)
	mustEq(t, []string{c.Nodes[0].Name, c.Nodes[1].Name}, []string{"node-1", "node-2"})
	mustEq(t, filepath.Base(c.Nodes[0].Dir), "node-1")
	mustEq(t, c.Nodes[0].Key.ID() != c.Nodes[1].Key.ID(), true)
	mustEq(t, c.Nodes[0].Host.Now().UnixMilli() > 0, true)

	// Default Tables uses home.NodeTables(), where post/brief is registered
	// and reaches the group-existence check (not_found rather than bad_request).
	n0 := c.Nodes[0]
	briefPayload := fmt.Sprintf(`{"t":"post","kind":"brief","n":%q,"ts":%d,"g":%q}`, nonceID(1), n0.Host.Now().UnixMilli(), testGroupID)
	_, cmdErr := n0.Host.Command(context.Background(), n0.Key.ID(), briefPayload)
	var hostErr *host.Error
	mustEq(t, errors.As(cmdErr, &hostErr), true)
	mustEq(t, hostErr.Code, "not_found")

	checkOwnerLogs(t, c, extraBrowser)
	for _, n := range c.Nodes {
		checkNodeSeeded(t, c, n)
	}
	checkHomeRestart(t, n0)
	checkFakeCLIs(t, c)
	checkCustomStart(t, c.BinDir)
}

func checkOwnerLogs(t *testing.T, c *nodetest.Cluster, extraBrowser identity.Key) {
	t.Helper()
	mustEq(t, c.Logs("stranger") == nil, true)
	ownerLog := c.Logs(c.Owner.ID())
	mustEq(t, ownerLog != nil, true)
	mustEq(t, ownerLog.Valid(c.Owner.ID()), true)
	mustEq(t, ownerLog.Valid(c.Nodes[0].Key.ID()), true)
	mustEq(t, ownerLog.Valid(c.Nodes[1].Key.ID()), true)
	mustEq(t, ownerLog.Valid(extraBrowser.ID()), true)
	mustEq(t, strings.Contains(ownerLog.Entries[1].Payload, `"name":"node-1"`), true)
	mustEq(t, strings.Contains(ownerLog.Entries[2].Payload, `"name":"node-2"`), true)
	mustEq(t, strings.Contains(ownerLog.Entries[3].Payload, `"name":"browser-2"`), true)
}

func checkNodeSeeded(t *testing.T, c *nodetest.Cluster, n *nodetest.Node) {
	t.Helper()
	mustEq(t, n.Link.Path(), "local")
	mustEq(t, node.ReadName(n.Store), n.Name)
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
	mustEq(t, verList, "[1,2,3,4]")
}

func checkHomeRestart(t *testing.T, n *nodetest.Node) {
	t.Helper()
	oldLink := n.Link
	n.StopHome()
	select {
	case <-oldLink.Done():
	default:
		t.Fatal("expected old link Done closed after StopHome")
	}
	n.RestartHome()
	mustEq(t, n.Link != oldLink, true)
	mustEq(t, n.Link.Path(), "local")
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

func checkCustomStart(t *testing.T, binDir string) {
	t.Helper()
	fixedMs := int64(1_763_700_000_000)
	customRoot := filepath.Join(t.TempDir(), "custom")
	single := nodetest.Start(t, nodetest.Config{
		Root:   customRoot,
		BinDir: binDir,
		Nodes:  0,
		Now:    func() time.Time { return time.UnixMilli(fixedMs) },
		Tables: home.Tables{Commands: home.Table{"group/create": singleGroupCreate}},
	})
	mustEq(t, []any{single.Root, single.BinDir, len(single.Nodes)}, []any{customRoot, binDir, 1})

	n0 := single.Nodes[0]
	mustEq(t, n0.Host.Now().UnixMilli(), fixedMs)
	payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"One"}`, nonceID(2), fixedMs, testGroupID)
	ans, err := n0.Host.Command(context.Background(), n0.Key.ID(), payload)
	mustOK(t, err)
	var res proto.CmdResult
	mustOK(t, proto.Decode(ans, &res))
	mustEq(t, res.Num, 3)

	single.Close()
	single.Close()
	_, open := n0.Host.Home(n0.Key.ID())
	mustEq(t, open, false)

	badRoot := filepath.Join(t.TempDir(), "not-a-dir")
	mustOK(t, os.WriteFile(badRoot, []byte("x"), 0o600))
	tb := &fatalTB{TB: t}
	func() {
		defer func() { recover() }()
		nodetest.Start(tb, nodetest.Config{Root: badRoot, BinDir: binDir, Nodes: 1})
	}()
	mustEq(t, tb.fatal, true)
}

func singleGroupCreate(s *home.State, cmd home.Command) (home.Result, *home.Refusal) {
	grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "One", Created: cmd.Now}
	mem := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m1"}, Group: s.ID, Kind: "human", Name: "Owner", User: s.Owner},
		Role:      "admin",
	}
	return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(mem)}, Answer: store.Outcome{Num: 3}}, nil
}
