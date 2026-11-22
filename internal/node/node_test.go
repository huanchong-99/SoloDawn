package node

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
)

func TestBrowserCommandsAndLogRotation(t *testing.T) {
	checkBrowserCommands(t)
	checkLogErrors(t)
	checkLogRotation(t)
}

func checkBrowserCommands(t *testing.T) {
	t.Helper()
	for _, tc := range []struct {
		goos, exe string
		argc      int
	}{
		{"darwin", "open", 1},
		{"windows", "rundll32", 2},
		{"linux", "xdg-open", 1},
	} {
		exe, args := browserCommand(tc.goos, "http://localhost:1/")
		if exe != tc.exe || len(args) != tc.argc {
			t.Fatalf("%s browserCommand = %q %v", tc.goos, exe, args)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if err := openBrowserURL("http://localhost:1/"); err == nil {
		t.Fatal("expected openBrowserURL error with empty PATH")
	}
}

func checkLogErrors(t *testing.T) {
	t.Helper()
	badRoot := t.TempDir()
	mustNoErr(t, os.WriteFile(filepath.Join(badRoot, "logs"), []byte("x"), 0o600))
	if _, err := openLog(badRoot); err == nil {
		t.Fatal("expected openLog error when logs is a file")
	}
	badFileRoot := t.TempDir()
	mustNoErr(t, os.MkdirAll(filepath.Join(badFileRoot, "logs", "node.log"), 0o700))
	if _, err := openLog(badFileRoot); err == nil {
		t.Fatal("expected openLog error when node.log is a directory")
	}
}

func checkLogRotation(t *testing.T) {
	t.Helper()
	savedMax := maxLogBytes
	t.Cleanup(func() { maxLogBytes = savedMax })
	maxLogBytes = 10

	bigDir := t.TempDir()
	bigLog, err := openLog(bigDir)
	mustNoErr(t, err)
	_, err = bigLog.Write([]byte("0123456789ab"))
	mustNoErr(t, err)
	if _, err := os.Stat(filepath.Join(bigDir, "logs", "node.log.1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected rotation on empty file: %v", err)
	}
	mustNoErr(t, bigLog.Close())

	lf, err := openLog(t.TempDir())
	mustNoErr(t, err)
	_, err = lf.Write([]byte("0123456789"))
	mustNoErr(t, err)
	_, err = lf.Write([]byte("next"))
	mustNoErr(t, err)
	checkImage(t, lf.path+".1", "0123456789")
	checkImage(t, lf.path, "next")
	_, err = lf.Write([]byte("0123456"))
	mustNoErr(t, err)
	checkImage(t, lf.path+".1", "next")
	checkImage(t, lf.path, "0123456")

	checkLogRotateFailure(t, lf)
}

func checkLogRotateFailure(t *testing.T, lf *logFile) {
	t.Helper()
	mustNoErr(t, os.Remove(lf.path+".1"))
	mustNoErr(t, os.MkdirAll(filepath.Join(lf.path+".1", "sub"), 0o700))
	if _, err := lf.Write([]byte("overflow..")); err == nil {
		t.Fatal("expected rotate error when .1 is a non-empty directory")
	}
	mustNoErr(t, lf.Close())
}

func TestNodeAddrs(t *testing.T) {
	savedIfaces := ifaceAddrs
	t.Cleanup(func() { ifaceAddrs = savedIfaces })

	ifaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("no ifaces") }
	empty := nodeAddrs(47800, nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty nodeAddrs = %#v, want non-nil []string{}", empty)
	}
	ifaceAddrs = sampleIfaceAddrs(t)
	got := nodeAddrs(47800, []string{"wss://rv.example/rv"})
	want := []string{
		"ws://192.0.2.10:47800/rv",
		"ws://[2001:db8::1]:47800/rv",
		"wss://rv.example/rv",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("nodeAddrs = %v, want %v", got, want)
	}
}

func sampleIfaceAddrs(t *testing.T) func() ([]net.Addr, error) {
	t.Helper()
	cidrs := []string{
		"127.0.0.1/8", "::1/128", "169.254.10.20/16", "fe80::1/64",
		"0.0.0.0/0", "192.0.2.10/24", "192.0.2.10/24", "2001:db8::1/64",
	}
	list := []net.Addr{
		&net.TCPAddr{IP: net.IPv4(192, 0, 2, 99), Port: 80},
		&net.IPNet{IP: net.IP{1, 2}},
	}
	for _, s := range cidrs {
		ip, ipnet, err := net.ParseCIDR(s)
		mustNoErr(t, err)
		ipnet.IP = ip
		list = append(list, ipnet)
	}
	return func() ([]net.Addr, error) { return list, nil }
}

func TestOpenNodeStartupErrors(t *testing.T) {
	t.Run("stepLog and stepIdentity errors", checkIdentityAndLogStartupErrors)
	t.Run("stepServices errors", checkServicesStartupErrors)
	t.Run("stepNetwork and stepStartup errors", checkNetworkAndStartupErrors)
}

func checkIdentityAndLogStartupErrors(t *testing.T) {
	dir := socketRoot(t)
	mustNoErr(t, os.WriteFile(filepath.Join(dir, "logs"), []byte("x"), 0o600))
	if err := runNode(t.Context(), dir, nil); err == nil {
		t.Fatal("expected stepLog error")
	}

	savedExe := executablePath
	t.Cleanup(func() { executablePath = savedExe })
	executablePath = func() (string, error) { return "", io.ErrUnexpectedEOF }
	if _, err := openNode(t.Context(), socketRoot(t)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v", err)
	}
	executablePath = savedExe

	badKeyDir := socketRoot(t)
	mustNoErr(t, os.WriteFile(filepath.Join(badKeyDir, "device.key"), []byte("short"), 0o600))
	if _, err := openNode(t.Context(), badKeyDir); err == nil {
		t.Fatal("expected LoadKey error")
	}

	badDBDir := socketRoot(t)
	mustNoErr(t, os.MkdirAll(filepath.Join(badDBDir, "node.db"), 0o700))
	if _, err := openNode(t.Context(), badDBDir); err == nil {
		t.Fatal("expected store.Open error")
	}
}

func checkServicesStartupErrors(t *testing.T) {
	badOwner := seedNodeDB(t, func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', 'not-json')")
		return err
	})
	if _, err := openNode(t.Context(), badOwner); err == nil {
		t.Fatal("expected OpenLogs error")
	}

	badLoop := seedNodeDB(t, seedInvalidGroupRow)
	if _, err := openNode(t.Context(), badLoop); err == nil {
		t.Fatal("expected StartLoop error")
	}

	badHost := seedNodeDB(t, func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE session")
		return err
	})
	if _, err := openNode(t.Context(), badHost); err == nil {
		t.Fatal("expected host.New error")
	}
}

func seedInvalidGroupRow(tx *store.Tx) error {
	stmts := []string{
		`INSERT INTO grp(g, seq, num) VALUES('g1', 2, 0)`,
		`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('g1', 'group', 'g1', 1, 1, 1, 1, '{"r":"group","id":"g1"}')`,
		`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('g1', 'member', 'u1', 2, 2, 1, 1, '{"r":"member","kind":"unknown"}')`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func checkNetworkAndStartupErrors(t *testing.T) {
	socks, err := listenNodePeer(0, 0)
	mustNoErr(t, err)
	socks.Close()
	ln, err := listenNodeHTTP(":0")
	mustNoErr(t, err)
	ln.Close()

	savedPeer, savedTCP := peerListen, tcpListen
	t.Cleanup(func() {
		peerListen = savedPeer
		tcpListen = savedTCP
	})
	peerListen = func(int, int) (*peer.Sockets, error) { return nil, io.ErrClosedPipe }
	if _, err := openNode(t.Context(), socketRoot(t)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("peerListen error = %v", err)
	}
	peerListen = savedPeer
	tcpListen = func(string, string) (net.Listener, error) { return nil, io.ErrNoProgress }
	if _, err := openNode(t.Context(), socketRoot(t)); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("tcpListen error = %v", err)
	}
	tcpListen = savedTCP

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := openNode(ctx, socketRoot(t)); err == nil {
		t.Fatal("expected canceled peer.Dial error")
	}

	badHomes := seedNodeDB(t, func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('homes', 'bad-json')")
		return err
	})
	if _, err := openNode(t.Context(), badHomes); err == nil {
		t.Fatal("expected EnsureHomeConnections error")
	}
}

func seedNodeDB(t *testing.T, fn func(*store.Tx) error) string {
	t.Helper()
	dir := socketRoot(t)
	db, err := store.Open(filepath.Join(dir, "node.db"))
	mustNoErr(t, err)
	mustNoErr(t, db.Write(fn))
	mustNoErr(t, db.Close())
	return dir
}

func TestMultiNodePortFallbackWebAndLocalCommands(t *testing.T) {
	dir1, dir2 := socketRoot(t), socketRoot(t)
	l1 := openSocket(t, dir1)
	openSocket(t, dir2)

	n1, err := openNode(t.Context(), dir1)
	mustNoErr(t, err)
	t.Cleanup(n1.Close)
	n2, err := openNode(t.Context(), dir2)
	mustNoErr(t, err)
	t.Cleanup(n2.Close)

	if n1.httpPort == n2.httpPort {
		t.Fatalf("two nodes bound the same HTTP port %d", n1.httpPort)
	}
	checkLocalWebServer(t, n1.httpPort)

	done1 := make(chan struct{}) // single reader: TestMultiNodePortFallbackWebAndLocalCommands
	go func() {
		n1.serve(l1)
		close(done1)
	}()
	checkLocalSocketAndMCP(t, dir1, n1)
	checkUnownedLocalCommands(t, dir1, n1)
	n1.Close()
	<-done1
}

func checkLocalWebServer(t *testing.T, port int) {
	t.Helper()
	resp, err := http.Get("http://localhost:" + strconv.Itoa(port) + "/")
	mustNoErr(t, err)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("localhost GET / = %d", resp.StatusCode)
	}
}

func checkLocalSocketAndMCP(t *testing.T, dir string, n *nodeRuntime) {
	t.Helper()
	badReply := dialRawLocal(t, dir, "{}\n")
	if !strings.Contains(badReply, `"error"`) {
		t.Fatalf("bad opening reply = %q", badReply)
	}
	dialRawLocal(t, dir, "{\"mcp\":\"bad-token\"}\n")

	sesKey := host.SessionKey{Group: "g1", Member: "m1"}
	mustNoErr(t, n.host.Sessions.Create(host.Session{SessionKey: sesKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: n.key.ID(), Name: "Claude"}))
	mustNoErr(t, n.host.Sessions.Move(sesKey, host.Dormant, host.Working))
	mustNoErr(t, n.host.Sessions.Move(sesKey, host.Working, host.Waiting))
	tok := n.host.Sessions.Admit(sesKey, 3, false)
	mcpConn, err := net.Dial("unix", filepath.Join(dir, "sock"))
	mustNoErr(t, err)
	io.WriteString(mcpConn, fmt.Sprintf("{\"mcp\":%q}\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"t\",\"version\":\"1\"}}}\n", tok))
	mcpLine, err := bufio.NewReader(mcpConn).ReadString('\n')
	mcpConn.Close()
	if err != nil || !strings.Contains(mcpLine, `"solodawn"`) {
		t.Fatalf("MCP initialize = %q (%v)", mcpLine, err)
	}
}

func dialRawLocal(t *testing.T, dir, payload string) string {
	t.Helper()
	conn, err := net.Dial("unix", filepath.Join(dir, "sock"))
	mustNoErr(t, err)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	io.WriteString(conn, payload)
	line, _ := bufio.NewReader(conn).ReadString('\n')
	return line
}

func checkUnownedLocalCommands(t *testing.T, dir string, n *nodeRuntime) {
	t.Helper()
	checkNodeStatusLine(t, dir, n)
	checkRoomFullPairError(t, dir, n.httpPort)
	checkUnownedRefusalCommands(t, dir)
	if err := n.waitRVPath("ws://unconfigured.example/rv"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("waitRVPath unconfigured = %v, want net.ErrClosed", err)
	}
	if err := (Command{Cmd: "bogus"}).send(dir, io.Discard); errText(err) != "unknown command: bogus" {
		t.Fatalf("bogus cmd = %v", err)
	}
}

func checkRoomFullPairError(t *testing.T, dir string, port int) {
	t.Helper()
	secret := identity.NewSecret()
	rv := "ws://127.0.0.1:" + strconv.Itoa(port) + "/rv"
	j1 := startRoomMember(t, rv, "fill-1", secret.Room())
	j2 := startRoomMember(t, rv, "fill-2", secret.Room())
	<-j1
	<-j2
	name := "desk"
	if err := (Command{Cmd: "pair", Secret: secret.String(), Name: &name}).send(dir, io.Discard); !strings.Contains(errText(err), "pairing room is full") {
		t.Fatalf("full room pair = %v", err)
	}
}

func startRoomMember(t *testing.T, rv, tag, room string) <-chan struct{} {
	t.Helper()
	key := sesKey(tag)
	p, err := peer.Dial(t.Context(), rv, key)
	mustNoErr(t, err)
	t.Cleanup(func() { p.Close() })
	socks, err := peer.Listen(0, 0)
	mustNoErr(t, err)
	t.Cleanup(func() { socks.Close() })
	pr := peer.New(key, socks, sesOpenStore(t), identity.IsDevice, func(string) []peer.Route { return nil }, func(*peer.Connection) {}, nil)
	joined := make(chan struct{}, 1) // single reader: checkRoomFullPairError
	go p.Serve(t.Context(), pr, func(kind string, _ []byte) error {
		if kind == "joined" {
			joined <- struct{}{}
		}
		return nil
	})
	mustNoErr(t, p.Write(t.Context(), proto.RoomID{Type: "join", Room: room}))
	return joined
}

func checkNodeStatusLine(t *testing.T, dir string, n *nodeRuntime) {
	t.Helper()
	var statusOut bytes.Buffer
	mustNoErr(t, (Command{Cmd: "status"}).send(dir, &statusOut))
	got := statusOut.String()
	for _, sub := range []string{"version " + Version, "fingerprint " + n.key.Fingerprint(), "session g1 m1 waiting"} {
		if !strings.Contains(got, sub) {
			t.Fatalf("status output %q missing %q", got, sub)
		}
	}
}

func checkUnownedRefusalCommands(t *testing.T, dir string) {
	t.Helper()
	for _, cmd := range []string{"pair-browser", "open"} {
		if err := (Command{Cmd: cmd}).send(dir, io.Discard); !strings.Contains(errText(err), "node has no owner") {
			t.Fatalf("unowned %s = %v", cmd, err)
		}
	}
	name := "desk"
	if err := (Command{Cmd: "pair", Secret: "bad", Name: &name}).send(dir, io.Discard); !strings.Contains(errText(err), "not a pairing secret") {
		t.Fatalf("bad secret pair = %v", err)
	}
	validSecret := identity.NewSecret().String()
	if err := (Command{Cmd: "pair", Secret: validSecret, Name: &name, Rendezvous: "ws://127.0.0.1:1/rv"}).send(dir, io.Discard); err == nil {
		t.Fatal("expected unreachable rendezvous pair error")
	}
}

func TestPairingSurfaceClientAndForegroundStop(t *testing.T) {
	savedInstall, savedOpen := installService, openURL
	t.Cleanup(func() {
		installService = savedInstall
		openURL = savedOpen
	})
	var serviceCalls int
	var failService bool
	installService = func(string, string) error {
		serviceCalls++
		if failService {
			return errors.New("service registration failed")
		}
		return nil
	}

	rvURL := startTestRVWithRelay(t)
	dir := socketRoot(t)
	l := openSocket(t, dir)
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	done := make(chan struct{}) // single reader: TestPairingSurfaceClientAndForegroundStop
	go func() {
		n.serve(l)
		close(done)
	}()

	browserKey, rtcConn, entries := pairNodeOverRendezvous(t, dir, n, rvURL, "office-mac")
	if serviceCalls != 1 {
		t.Fatalf("serviceCalls = %d, want 1", serviceCalls)
	}
	failService = true
	n.onAdopt(browserKey.ID())

	gid := verifySurfaceAndGroupCreate(t, rtcConn, browserKey, n.key.ID(), "office-mac")
	checkOwnedBrowserCommands(t, dir, n)
	checkWorkerClientAndRVReconnect(t, n, browserKey, entries, rvURL, gid)

	rtcConn.Close()
	n.Close()
	<-done
	n.syncRendezvous()

	checkSaveRendezvousError(t, rvURL)
	checkForegroundContextStop(t)
}

func pairNodeOverRendezvous(t *testing.T, dir string, n *nodeRuntime, rvURL, label string) (identity.Key, *peer.Connection, []proto.Envelope) {
	t.Helper()
	browserKey := sesKey("owner-browser")
	secret := identity.NewSecret()
	bPeer, bPath := newTestBrowserPeer(t, browserKey, rvURL)
	gotPeer := startBrowserRoomJoin(t, bPath, bPeer, browserKey, secret)

	pairErr := make(chan error, 1) // single reader: pairNodeOverRendezvous
	var pairOut bytes.Buffer
	go func() {
		pairErr <- (Command{Cmd: "pair", Secret: secret.String(), Name: &label, Rendezvous: rvURL}).send(dir, &pairOut)
	}()

	roomMsg := <-gotPeer
	if roomMsg.Key != n.key.ID() {
		t.Fatalf("room peer key = %q, want %q", roomMsg.Key, n.key.ID())
	}
	rtcConn := dialTestRTC(t, bPeer, n.key.ID())
	sendBrowserHello(t, rtcConn, false)

	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b1")
	e2 := sesAddEntry(browserKey, browserKey.ID(), 2, n.key.ID(), "node", label)
	mustNoErr(t, rtcConn.Send(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1, e2}}))
	mustNoErr(t, <-pairErr)
	if !strings.Contains(pairOut.String(), label) || !strings.Contains(pairOut.String(), n.key.Fingerprint()) {
		t.Fatalf("pair output = %q", pairOut.String())
	}
	sendBrowserHello(t, rtcConn, true)
	return browserKey, rtcConn, []proto.Envelope{e1, e2}
}

func newTestBrowserPeer(t *testing.T, key identity.Key, rvURL string) (*peer.Peer, *peer.Path) {
	t.Helper()
	bPath, err := peer.Dial(t.Context(), rvURL, key)
	mustNoErr(t, err)
	t.Cleanup(func() { bPath.Close() })
	bSocks, err := peer.Listen(0, 0)
	mustNoErr(t, err)
	t.Cleanup(func() { bSocks.Close() })
	bDB := sesOpenStore(t)
	routes := func(string) []peer.Route { return []peer.Route{bPath.Route()} }
	bPeer := peer.New(key, bSocks, bDB, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	return bPeer, bPath
}

func startBrowserRoomJoin(t *testing.T, bPath *peer.Path, bPeer *peer.Peer, key identity.Key, secret identity.Secret) <-chan proto.Pair {
	t.Helper()
	room := secret.Room()
	self := proto.Pair{Type: "pair", Key: key.ID(), Kind: "browser", Name: "b1", Role: "a"}
	self.MAC = secret.MAC(self)
	rawSelf := json.RawMessage(dvlMustEncode(self))
	gotPeer := make(chan proto.Pair, 1) // single reader: pairNodeOverRendezvous
	var once atomic.Bool
	event := func(kind string, data []byte) error {
		if kind == "joined" {
			bPath.Write(t.Context(), proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: rawSelf})
		}
		if kind == "room" {
			if m, ok := decodeVerifiedRoomPeer(data, room, secret); ok && once.CompareAndSwap(false, true) {
				gotPeer <- m
			}
		}
		return nil
	}
	go bPath.Serve(t.Context(), bPeer, event)
	mustNoErr(t, bPath.Write(t.Context(), proto.RoomID{Type: "join", Room: room}))
	return gotPeer
}

func decodeVerifiedRoomPeer(data []byte, room string, secret identity.Secret) (proto.Pair, bool) {
	var rd proto.RoomData
	var m proto.Pair
	if proto.Decode(data, &rd) != nil || rd.Room != room || proto.Decode(rd.Data, &m) != nil {
		return proto.Pair{}, false
	}
	return m, secret.Verify(m, "b")
}

func dialTestRTC(t *testing.T, bPeer *peer.Peer, nodeID string) *peer.Connection {
	t.Helper()
	rtcConn, err := bPeer.Offer(t.Context(), nodeID)
	mustNoErr(t, err)
	t.Cleanup(func() { rtcConn.Close() })
	select {
	case <-rtcConn.Ready():
	case <-time.After(10 * time.Second):
		t.Fatal("WebRTC ctl connection did not open")
	}
	go func() {
		for {
			if _, err := rtcConn.Read(t.Context()); err != nil {
				return
			}
		}
	}()
	return rtcConn
}

func sendBrowserHello(t *testing.T, conn *peer.Connection, wantOwner bool) {
	t.Helper()
	raw, err := conn.Request(t.Context(), func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}}
	})
	mustNoErr(t, err)
	var w proto.Welcome
	if err := proto.Decode(raw, &w); err != nil || w.Owner != wantOwner || w.Addrs == nil {
		t.Fatalf("welcome = %+v (%v), wantOwner=%v", w, err, wantOwner)
	}
}

func verifySurfaceAndGroupCreate(t *testing.T, conn *peer.Connection, browserKey identity.Key, nodeID, wantName string) proto.ID {
	t.Helper()
	checkNodeHostInfo(t, conn, nodeID, wantName)
	gid := sesID("grp-adopted")
	createEnv := browserKey.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Main","kind":"project","forge":"none","repo":"/tmp/r","base":"main","checks":[],"member_name":"Owner"}`, gid, time.Now().UnixMilli(), gid))
	rawCreate, err := conn.Request(t.Context(), func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: createEnv}
	})
	mustNoErr(t, err)
	var okAns proto.Success
	mustNoErr(t, proto.Decode(rawCreate, &okAns))
	if !okAns.OK {
		t.Fatalf("group create = %+v", okAns)
	}
	return gid
}

func checkNodeHostInfo(t *testing.T, conn *peer.Connection, nodeID, wantName string) {
	t.Helper()
	rawInfo, err := conn.Request(t.Context(), func(id int64) any {
		return proto.Request{Type: "host.info", ID: id}
	})
	mustNoErr(t, err)
	var info struct {
		proto.Success
		proto.HostState
	}
	mustNoErr(t, proto.Decode(rawInfo, &info))
	wantOS := runtime.GOOS + "/" + runtime.GOARCH
	if info.Device != nodeID || info.Name != wantName || info.OS != wantOS || info.Version != Version {
		t.Fatalf("host.info = %+v", info)
	}
}

func checkOwnedBrowserCommands(t *testing.T, dir string, n *nodeRuntime) {
	t.Helper()
	name := "desk"
	validSecret := identity.NewSecret().String()
	if err := (Command{Cmd: "pair", Secret: validSecret, Name: &name}).send(dir, io.Discard); errText(err) != ErrOwned.Error() {
		t.Fatalf("owned pair = %v, want %v", err, ErrOwned)
	}

	var pbOut bytes.Buffer
	mustNoErr(t, (Command{Cmd: "pair-browser"}).send(dir, &pbOut))
	if !strings.Contains(pbOut.String(), "/#pair=") {
		t.Fatalf("pair-browser = %q", pbOut.String())
	}

	var opened string
	openURL = func(u string) error {
		opened = u
		return nil
	}
	var openOut bytes.Buffer
	mustNoErr(t, (Command{Cmd: "open"}).send(dir, &openOut))
	if !strings.HasPrefix(opened, n.localWebURL()+"/#pair=") || strings.TrimSpace(openOut.String()) != opened {
		t.Fatalf("open = %q (opened %q)", openOut.String(), opened)
	}

	openURL = func(string) error { return errors.New("browser launch failed") }
	if err := (Command{Cmd: "open"}).send(dir, io.Discard); errText(err) != "browser launch failed" {
		t.Fatalf("open error = %v", err)
	}
}

func checkWorkerClientAndRVReconnect(t *testing.T, n *nodeRuntime, browserKey identity.Key, entries []proto.Envelope, rvURL string, gid proto.ID) {
	t.Helper()
	hostDir := socketRoot(t)
	hostKey, err := identity.LoadKey(hostDir)
	mustNoErr(t, err)
	e3 := sesAddEntry(browserKey, browserKey.ID(), 3, hostKey.ID(), "node", "worker")
	fc := serveConn(t, n.sessions, browserKey.ID(), false)
	defer fc.Close()
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3}})
	waitCond(t, func() bool { return n.logs.Known(hostKey.ID()) })

	seedWorkerNode(t, hostDir, browserKey.ID(), append(entries, e3), rvURL, []proto.Home{
		{Group: gid, Home: n.key.ID()},
		{Group: sesID("grp-self"), Home: hostKey.ID()},
	})
	nHost, err := openNode(t.Context(), hostDir)
	mustNoErr(t, err)
	waitCond(t, func() bool {
		_, ok := nHost.host.Home(n.key.ID())
		return ok
	})
	nHost.Close()
	nHost.dialHome(t.Context(), n.key.ID(), func(clientCtl) {})

	firstPath := n.remoteRVPaths()[0]
	firstPath.Close()
	waitCond(t, func() bool {
		paths := n.remoteRVPaths()
		return len(paths) == 1 && paths[0] != firstPath
	})

	mustNoErr(t, n.saveRendezvous([]string{}))
	n.syncRendezvous()
	waitCond(t, func() bool { return len(n.remoteRVPaths()) == 0 })
	if got := n.defaultWebURL(); got != n.localWebURL() {
		t.Fatalf("defaultWebURL without RV = %q, want %q", got, n.localWebURL())
	}
}

func checkSaveRendezvousError(t *testing.T, rvURL string) {
	t.Helper()
	errDir := socketRoot(t)
	errL := openSocket(t, errDir)
	errNode, err := openNode(t.Context(), errDir)
	mustNoErr(t, err)
	errDone := make(chan struct{}) // single reader: checkSaveRendezvousError
	go func() {
		errNode.serve(errL)
		close(errDone)
	}()
	dropSetting := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE setting")
		return err
	}
	mustNoErr(t, errNode.db.Write(dropSetting))
	name := "desk"
	if err := (Command{Cmd: "pair", Secret: identity.NewSecret().String(), Name: &name, Rendezvous: rvURL}).send(errDir, io.Discard); err == nil {
		t.Fatal("expected pair error when setting table is missing")
	}
	errNode.Close()
	<-errDone
}

func checkForegroundContextStop(t *testing.T) {
	t.Helper()
	defCtx, defStop := foregroundContext()
	defStop()
	<-defCtx.Done()

	savedFg := foregroundContext
	t.Cleanup(func() { foregroundContext = savedFg })
	stopCtx, stopNode := context.WithCancel(t.Context())
	foregroundContext = func() (context.Context, context.CancelFunc) { return stopCtx, func() {} }
	sigDir := socketRoot(t)
	sigL := openSocket(t, sigDir)
	sigErr := make(chan error, 1) // single reader: checkForegroundContextStop
	go func() { sigErr <- runForegroundNode(sigDir, sigL) }()
	waitCond(t, func() bool {
		return (Command{Cmd: "status"}).send(sigDir, io.Discard) == nil
	})
	stopNode()
	mustNoErr(t, <-sigErr)
}

func waitCond(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}

func seedWorkerNode(t *testing.T, dir, owner string, entries []proto.Envelope, rvURL string, homes []proto.Home) {
	t.Helper()
	db, err := store.Open(filepath.Join(dir, "node.db"))
	mustNoErr(t, err)
	ownerRaw, _ := json.Marshal(owner)
	rvRaw, _ := json.Marshal([]string{rvURL})
	homesRaw, _ := json.Marshal(homes)
	err = db.Write(func(tx *store.Tx) error {
		if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?), ('rendezvous', ?), ('homes', ?)", string(ownerRaw), string(rvRaw), string(homesRaw)); err != nil {
			return err
		}
		for i, env := range entries {
			raw, _ := json.Marshal(env)
			if _, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", owner, i+1, string(raw)); err != nil {
				return err
			}
		}
		return nil
	})
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
}

func startTestRVWithRelay(t *testing.T) string {
	t.Helper()
	iceJSON := startLoopbackTURN(t)
	back := httptest.NewServer(rendezvous.Handler(os.DirFS(t.TempDir())))
	t.Cleanup(back.Close)
	backURL := "ws" + strings.TrimPrefix(back.URL, "http") + "/rv"
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayRVProxy(w, r, backURL, iceJSON)
	}))
	t.Cleanup(front.Close)
	return "ws" + strings.TrimPrefix(front.URL, "http") + "/rv"
}

func startLoopbackTURN(t *testing.T) string {
	t.Helper()
	pkt, err := net.ListenPacket("udp4", "127.0.0.1:0")
	mustNoErr(t, err)
	lo := net.IPv4(127, 0, 0, 1)
	gen := &turn.RelayAddressGeneratorStatic{RelayAddress: lo, Address: "127.0.0.1"}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "test",
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			return "peer", turn.GenerateAuthKey("peer", "test", "pw"), r.Username == "peer"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: pkt, RelayAddressGenerator: gen}},
	})
	mustNoErr(t, err)
	t.Cleanup(func() { srv.Close() })
	raw, _ := json.Marshal([]proto.ICEServer{{
		URLs:       []string{"turn:" + pkt.LocalAddr().String() + "?transport=udp"},
		Username:   "peer",
		Credential: "pw",
	}})
	return string(raw)
}

func relayRVProxy(w http.ResponseWriter, r *http.Request, backURL, iceJSON string) {
	cConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer cConn.CloseNow()
	bConn, _, err := websocket.Dial(r.Context(), backURL, nil)
	if err != nil {
		return
	}
	defer bConn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go pumpWS(ctx, cancel, cConn, bConn, nil)
	pumpWS(ctx, cancel, bConn, cConn, func(b []byte) []byte {
		return bytes.Replace(b, []byte(`"ice":[]`), []byte(`"ice":`+iceJSON), 1)
	})
}

func pumpWS(ctx context.Context, cancel context.CancelFunc, src, dst *websocket.Conn, mapFn func([]byte) []byte) {
	defer cancel()
	for {
		typ, msg, err := src.Read(ctx)
		if err != nil {
			return
		}
		if mapFn != nil {
			msg = mapFn(msg)
		}
		if dst.Write(ctx, typ, msg) != nil {
			return
		}
	}
}

func TestCloseWaitsForInflightHostCmd(t *testing.T) {
	dir := socketRoot(t)
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)

	browserKey := adoptTestBrowser(t, n)
	started := make(chan struct{}) // single reader: TestCloseWaitsForInflightHostCmd
	release := make(chan struct{}) // single reader: host.join handler
	n.sessions.Commands["host.join"] = func(ctx context.Context, _ Ctl, _ string, _ json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		<-release
		return nil, nil
	}

	fc := startAdoptedConn(t, n, browserKey.ID())
	nonce := sesID("inflight-close")
	joinEnv := browserKey.Sign(fmt.Sprintf(`{"t":"host.join","n":%q,"ts":%d,"to":%q}`, nonce, time.Now().UnixMilli(), n.key.ID()))
	fc.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: joinEnv})
	<-started

	closed := make(chan struct{}) // single reader: TestCloseWaitsForInflightHostCmd
	go func() {
		n.Close()
		close(closed)
	}()
	<-n.ctx.Done()
	<-fc.Done()
	select {
	case <-closed:
		t.Fatal("Close returned while host command was still running")
	default:
	}
	close(release)
	<-closed
	verifyStoredReceipt(t, filepath.Join(dir, "node.db"), browserKey.ID(), string(nonce))
}

func adoptTestBrowser(t *testing.T, n *nodeRuntime) identity.Key {
	t.Helper()
	saved := installService
	t.Cleanup(func() { installService = saved })
	installService = func(string, string) error { return nil }
	browserKey := sesKey("browser")
	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b1")
	e2 := sesAddEntry(browserKey, browserKey.ID(), 2, n.key.ID(), "node", "desk")
	cand, err := identity.Replay(browserKey.ID(), []proto.Envelope{e1, e2})
	mustNoErr(t, err)
	adopted, err := n.logs.dvlAdopt(browserKey.ID(), cand, nil)
	mustNoErr(t, err)
	mustEqual(t, adopted, true)
	return browserKey
}

func startAdoptedConn(t *testing.T, n *nodeRuntime, device string) *fakeCtl {
	t.Helper()
	fc := newFakeCtl()
	n.spawn(func() { n.sessions.Serve(n.ctx, device, false, fc) })
	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	fc.next(t)
	fc.next(t)
	return fc
}

func verifyStoredReceipt(t *testing.T, dbPath, signer, nonce string) {
	t.Helper()
	db, err := store.Open(dbPath)
	mustNoErr(t, err)
	defer db.Close()
	var out store.Outcome
	read := func(tx *store.Tx) error {
		var err error
		out, err = tx.Receipt(signer, nonce)
		return err
	}
	mustNoErr(t, db.Write(read))
	mustEqual(t, out, store.Outcome{OK: true})
}

type nodePTYTestDriver struct {
	exe string
}

func (d nodePTYTestDriver) Start(context.Context, host.Launch) (host.Process, error) {
	return nil, errors.New("unused structured start")
}

func (d nodePTYTestDriver) Interactive(host.Launch) ([]string, string, error) {
	return []string{d.exe, "-test.run=^TestNodePTYHelperProcess$", "--", "node-pty-echo"}, "sess-node-pty", nil
}

func TestNodePTYHelperProcess(t *testing.T) {
	if t == nil {
		return
	}
	sep := slices.Index(os.Args, "--")
	if sep < 0 || sep+1 >= len(os.Args) || os.Args[sep+1] != "node-pty-echo" {
		return
	}
	fmt.Println("NODE-PTY-READY")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err == nil {
		fmt.Printf("ECHO:%s\n", strings.TrimRight(line, "\r\n"))
	}
	os.Exit(0)
}

func TestNodePTYChannelBridge(t *testing.T) {
	dir := socketRoot(t)
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	browserKey := adoptTestBrowser(t, n)

	exe, err := os.Executable()
	mustNoErr(t, err)
	host.Drivers["node-pty-drv"] = nodePTYTestDriver{exe: exe}
	t.Cleanup(func() { delete(host.Drivers, "node-pty-drv") })

	ptyDone := make(chan struct{}) // single reader: TestNodePTYChannelBridge
	_, _, err = n.host.StartPTY("g1", "m1", "node-pty-drv", host.Launch{Folder: t.TempDir(), Level: 3}, func() {
		close(ptyDone)
	})
	mustNoErr(t, err)

	dc, recvCh := dialBrowserPTYChannel(t, n, browserKey, "pty:g1:m1")
	waitPTYBytes(t, recvCh, "NODE-PTY-READY")
	mustNoErr(t, dc.Send([]byte("hello-node\r")))
	waitPTYBytes(t, recvCh, "ECHO:hello-node")
	mustNoErr(t, dc.Close())
	<-ptyDone
}

func TestNodeWatchChannelWithoutGrant(t *testing.T) {
	n, err := openNode(t.Context(), socketRoot(t))
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	browser := adoptTestBrowser(t, n)
	dc, _ := dialBrowserPTYChannel(t, n, browser, "watch:g:m")
	mustNoErr(t, dc.Close())
}

func dialBrowserPTYChannel(t *testing.T, n *nodeRuntime, browserKey identity.Key, label string) (*webrtc.DataChannel, <-chan []byte) {
	t.Helper()
	var iceServers []proto.ICEServer
	mustNoErr(t, json.Unmarshal([]byte(startLoopbackTURN(t)), &iceServers))
	toBrowser := make(chan proto.Envelope, 64) // single reader: candidate/answer pump below
	route := peer.Route{
		ID:   "pty-test",
		ICE:  iceServers,
		Send: func(_ context.Context, _ string, env proto.Envelope) error { toBrowser <- env; return nil },
	}
	att := &pairAttempt{
		peer: proto.Pair{Key: browserKey.ID()}, link: PairLink{Expires: time.Now().Add(time.Hour).UnixMilli()},
		cancel: func() {}, outcome: make(chan error, 1), routes: []peer.Route{route},
	}
	n.pairer.mu.Lock()
	n.pairer.attempts[att] = true
	n.pairer.mu.Unlock()

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers:         []webrtc.ICEServer{{URLs: iceServers[0].URLs, Username: iceServers[0].Username, Credential: iceServers[0].Credential}},
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
	mustNoErr(t, err)
	t.Cleanup(func() { pc.Close() })
	_, err = pc.CreateDataChannel("ctl", nil)
	mustNoErr(t, err)
	ptyDC, err := pc.CreateDataChannel(label, nil)
	mustNoErr(t, err)
	opened := make(chan struct{})   // single reader: dialBrowserPTYChannel
	recvCh := make(chan []byte, 32) // single reader: waitPTYBytes
	ptyDC.OnOpen(func() { close(opened) })
	ptyDC.OnMessage(func(m webrtc.DataChannelMessage) { recvCh <- m.Data })

	connID := string(sesID("pty-conn"))
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			sendBrowserSignal(t.Context(), n.peer, route, browserKey, n.key.ID(), connID, proto.ICECandidate{
				SignalHeader: browserSigHdr(n.key.ID(), connID, "ice"), Candidate: c.ToJSON().Candidate, MID: c.SDPMid,
			})
		}
	})
	offer, err := pc.CreateOffer(nil)
	mustNoErr(t, err)
	mustNoErr(t, pc.SetLocalDescription(offer))
	mustNoErr(t, sendBrowserSignal(t.Context(), n.peer, route, browserKey, n.key.ID(), connID, proto.Offer{
		SignalHeader: browserSigHdr(n.key.ID(), connID, "offer"), SDP: offer.SDP,
	}))
	go applyNodeSignals(t.Context(), pc, toBrowser)
	<-opened
	return ptyDC, recvCh
}

func browserSigHdr(to, connID, kind string) proto.SignalHeader {
	return proto.SignalHeader{
		PayloadHeader: proto.PayloadHeader{Type: kind, Nonce: proto.NewNonce(), Timestamp: time.Now().UnixMilli(), To: to},
		Conn:          connID,
	}
}

func sendBrowserSignal(ctx context.Context, p *peer.Peer, route peer.Route, key identity.Key, _, _ string, msg any) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	env := key.Sign(string(raw))
	envRaw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return p.Receive(ctx, route, proto.Route{Type: "from", Key: key.ID(), Env: envRaw})
}

func applyNodeSignals(ctx context.Context, pc *webrtc.PeerConnection, in <-chan proto.Envelope) {
	var pending []webrtc.ICECandidateInit
	answered := false
	for {
		select {
		case <-ctx.Done():
			return
		case env := <-in:
			answered, pending = applyOneNodeSignal(pc, env, answered, pending)
		}
	}
}

func applyOneNodeSignal(pc *webrtc.PeerConnection, env proto.Envelope, answered bool, pending []webrtc.ICECandidateInit) (bool, []webrtc.ICECandidateInit) {
	var ans proto.Answer
	if !answered && proto.Decode([]byte(env.Payload), &ans) == nil && ans.Type == "answer" {
		pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ans.SDP})
		for _, c := range pending {
			pc.AddICECandidate(c)
		}
		return true, nil
	}
	var ice proto.ICECandidate
	if proto.Decode([]byte(env.Payload), &ice) == nil && ice.Type == "ice" {
		init := webrtc.ICECandidateInit{Candidate: ice.Candidate, SDPMid: &ice.MID}
		if !answered {
			return false, append(pending, init)
		}
		pc.AddICECandidate(init)
	}
	return answered, pending
}

func waitPTYBytes(t *testing.T, recvCh <-chan []byte, sub string) {
	t.Helper()
	var buf strings.Builder
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for !strings.Contains(buf.String(), sub) {
		select {
		case b := <-recvCh:
			buf.Write(b)
		case <-timer.C:
			t.Fatalf("timed out waiting for %q in %q", sub, buf.String())
		}
	}
}
