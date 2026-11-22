package nodetest_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
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
	grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "One", Checks: []string{}, Created: cmd.Now}
	mem := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m1"}, Group: s.ID, Kind: "human", Name: "Owner", User: s.Owner},
		Role:      "admin",
	}
	return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(mem)}, Answer: store.Outcome{Num: 3}}, nil
}

func TestStartPeerNodeOwnedAndUnowned(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	defer srv.Close()
	rvURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
	ice := startLoopbackTURN(t)

	checkOwnedMemoryPeerNode(t, ctx, rvURL)
	checkExplicitDiskKeyPeerNode(t, ctx, rvURL)
	checkUnownedDiskPeerNode(t, ctx, rvURL, ice)
}

func checkOwnedMemoryPeerNode(t *testing.T, ctx context.Context, rvURL string) {
	t.Helper()
	owner, extra := detKey(11), detKey(12)
	pn := nodetest.StartPeerNode(t, nodetest.PeerConfig{
		Ctx:           ctx,
		Owner:         owner,
		ExtraBrowsers: []identity.Key{extra},
		Rendezvous:    rvURL,
	})
	mustEq(t, pn.Key != nil, true)
	mustEq(t, pn.Logs.Owner(), owner.ID())
	mustEq(t, pn.Logs.Known(owner.ID()), true)
	mustEq(t, pn.Logs.Known(extra.ID()), true)
	mustEq(t, pn.Logs.Known(detKey(99).ID()), false)
	mustEq(t, node.ReadName(pn.Store), "node")
	mustEq(t, node.ReadRendezvous(pn.Store), []string{rvURL})
	state, err := pn.Surface.HostState(ctx)
	mustOK(t, err)
	mustEq(t, []string{state.Name, state.Version, state.OS}, []string{"node", "dev", runtime.GOOS + "/" + runtime.GOARCH})
	offerConn, err := pn.Peer.Offer(ctx, owner.ID())
	mustOK(t, err)
	offerConn.Close()
	pn.Close()
	pn.Close()
}

func checkExplicitDiskKeyPeerNode(t *testing.T, ctx context.Context, rvURL string) {
	t.Helper()
	fixedKey := detKey(15)
	dir := filepath.Join(t.TempDir(), "explicit")
	pn := nodetest.StartPeerNode(t, nodetest.PeerConfig{
		Ctx:        ctx,
		Dir:        dir,
		Key:        fixedKey,
		Name:       "explicit-box",
		Version:    "v2.0.0",
		Rendezvous: rvURL,
	})
	mustEq(t, pn.Key.ID(), fixedKey.ID())
	mustEq(t, pn.Logs.Owner(), "")
}

func checkUnownedDiskPeerNode(t *testing.T, ctx context.Context, rvURL string, ice []proto.ICEServer) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "unowned")
	fixedNow := time.UnixMilli(1_763_800_000_000)
	inboundCh := make(chan *peer.Connection, 1) // single reader: checkUnownedDiskPeerNode
	mapRelay := func(r peer.Route) peer.Route { return relayOnlyRoute(r, ice) }
	pn := nodetest.StartPeerNode(t, nodetest.PeerConfig{
		Ctx:        ctx,
		Dir:        dir,
		Name:       "initial",
		Version:    "v1.2.3",
		Rendezvous: rvURL,
		Now:        func() time.Time { return fixedNow },
		MapRoute:   mapRelay,
		OnInbound:  func(c *peer.Connection) { inboundCh <- c },
	})
	mustEq(t, pn.Host.Now().Equal(fixedNow), true)
	reloaded, err := identity.LoadKey(dir)
	mustOK(t, err)
	mustEq(t, pn.Key.ID(), reloaded.ID())

	browser := detKey(21)
	bPeer, bPath := openTestBrowser(t, ctx, browser, rvURL, mapRelay)
	secret := identity.NewSecret()
	pairDone := startPairCall(ctx, pn.Pairer, secret.String(), "workstation")
	exchangeTestRoom(t, ctx, bPath, bPeer, browser, secret)
	waitPairerKnown(t, pn.Pairer, browser.ID())

	conn := offerTestCtl(t, ctx, bPeer, pn.Key.ID())
	inbound := <-inboundCh
	mustEq(t, inbound.Device, browser.ID())
	adoptTestNode(t, ctx, conn, browser, pn.Key.ID(), "workstation", fixedNow.UnixMilli())
	res := <-pairDone
	mustEq(t, []string{res.Name, res.Fingerprint, res.Path}, []string{"workstation", pn.Key.Fingerprint(), "relay"})
	mustEq(t, verifyOwnerHostInfo(t, ctx, conn, pn.Key.ID()), "v1.2.3")
}

func startPairCall(ctx context.Context, p *node.Pairer, secret, name string) <-chan node.PairResult {
	// Single reader: checkUnownedDiskPeerNode.
	out := make(chan node.PairResult, 1)
	go func() {
		res, _ := p.Pair(ctx, node.PairRequest{Secret: secret, Name: name})
		out <- res
	}()
	return out
}

func openTestBrowser(t *testing.T, ctx context.Context, key identity.Key, rvURL string, mapRoute func(peer.Route) peer.Route) (*peer.Peer, *peer.Path) {
	t.Helper()
	path, err := peer.Dial(ctx, rvURL, key)
	mustOK(t, err)
	t.Cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0)
	mustOK(t, err)
	db, err := store.Open(":memory:")
	mustOK(t, err)
	t.Cleanup(func() { socks.Close(); db.Close() })
	routes := func(string) []peer.Route { return []peer.Route{mapRoute(path.Route())} }
	p := peer.New(key, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	return p, path
}

func exchangeTestRoom(t *testing.T, ctx context.Context, path *peer.Path, p *peer.Peer, key identity.Key, secret identity.Secret) {
	t.Helper()
	room := secret.Room()
	self := proto.Pair{Type: "pair", Key: key.ID(), Kind: "browser", Name: "Browser", Role: "a"}
	self.MAC = secret.MAC(self)
	rawSelf, _ := json.Marshal(self)
	matched := make(chan struct{}, 1) // single reader: exchangeTestRoom
	var once sync.Once
	event := func(kind string, data []byte) error {
		if kind == "joined" {
			path.Write(ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: rawSelf})
		}
		if kind == "room" && matchesRoomPeer(data, room, secret) {
			once.Do(func() { matched <- struct{}{} })
		}
		return nil
	}
	go path.Serve(ctx, p, event)
	mustOK(t, path.Write(ctx, proto.RoomID{Type: "join", Room: room}))
	<-matched
}

func matchesRoomPeer(data []byte, room string, secret identity.Secret) bool {
	var rd proto.RoomData
	var m proto.Pair
	if proto.Decode(data, &rd) != nil || rd.Room != room || proto.Decode(rd.Data, &m) != nil {
		return false
	}
	return secret.Verify(m, "b")
}

func waitPairerKnown(t *testing.T, p *node.Pairer, device string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p.Known(device) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pairer never recorded %s", device)
}

func offerTestCtl(t *testing.T, ctx context.Context, p *peer.Peer, target string) *peer.Connection {
	t.Helper()
	conn, err := p.Offer(ctx, target)
	mustOK(t, err)
	t.Cleanup(func() { conn.Close() })
	<-conn.Ready()
	go drainTestConn(ctx, conn)
	return conn
}

func drainTestConn(ctx context.Context, conn *peer.Connection) {
	for {
		if _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func adoptTestNode(t *testing.T, ctx context.Context, conn *peer.Connection, owner identity.Key, nodeID, label string, ts int64) {
	t.Helper()
	hello := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Browser", Addrs: []string{}}
	}
	_, err := conn.Request(ctx, hello)
	mustOK(t, err)
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(owner, owner.ID(), 1, "browser", "Browser", owner.ID(), ts),
		nodetest.SignDeviceAdd(owner, owner.ID(), 2, "node", label, nodeID, ts),
	}
	mustOK(t, conn.Send(proto.Devlog{Type: "devlog", User: owner.ID(), Entries: entries}))
	rawWelcome, err := conn.Request(ctx, hello)
	mustOK(t, err)
	var w proto.Welcome
	mustOK(t, proto.Decode(rawWelcome, &w))
	mustEq(t, w.Owner, true)
}

func verifyOwnerHostInfo(t *testing.T, ctx context.Context, conn *peer.Connection, wantDevice string) string {
	t.Helper()
	rawInfo, err := conn.Request(ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	mustOK(t, err)
	var info proto.HostInfo
	mustOK(t, proto.Decode(rawInfo, &info))
	mustEq(t, info.Device, wantDevice)
	return info.Version
}

type turnRelayPool struct {
	turn.RelayAddressGeneratorStatic
	mu    sync.Mutex
	ports map[int]bool
}

func (g *turnRelayPool) AllocatePacketConn(conf turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	conn, addr, err := g.RelayAddressGeneratorStatic.AllocatePacketConn(conf)
	if err != nil {
		return nil, nil, err
	}
	g.mu.Lock()
	g.ports[addr.(*net.UDPAddr).Port] = true
	g.mu.Unlock()
	return turnRelayConn{PacketConn: conn, pool: g}, addr, nil
}

func (g *turnRelayPool) has(addr net.Addr) bool {
	udp, ok := addr.(*net.UDPAddr)
	if !ok {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ports[udp.Port]
}

type turnRelayConn struct {
	net.PacketConn
	pool *turnRelayPool
}

func (c turnRelayConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || c.pool.has(addr) {
			return n, addr, err
		}
	}
}

func startLoopbackTURN(t *testing.T) []proto.ICEServer {
	t.Helper()
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	mustOK(t, err)
	ip := net.IPv4(127, 0, 0, 1)
	gen := &turnRelayPool{
		RelayAddressGeneratorStatic: turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()},
		ports:                       map[int]bool{},
	}
	auth := func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "test", "secret"), r.Username == "peer"
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             "test",
		AuthHandler:       auth,
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen}},
	})
	mustOK(t, err)
	t.Cleanup(func() { srv.Close() })
	return []proto.ICEServer{{URLs: []string{"turn:" + udp.LocalAddr().String() + "?transport=udp"}, Username: "peer", Credential: "secret"}}
}

func relayOnlyRoute(r peer.Route, ice []proto.ICEServer) peer.Route {
	orig := r.Send
	r.ICE = ice
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		var sig struct {
			Type string `json:"t"`
			Cand string `json:"cand"`
		}
		if json.Unmarshal([]byte(env.Payload), &sig) == nil && sig.Type == "ice" && !strings.Contains(sig.Cand, " typ relay") {
			return nil
		}
		return orig(ctx, to, env)
	}
	return r
}
