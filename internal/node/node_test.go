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
	"slices"
	"strconv"
	"strings"
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
)

func TestLogRotationAndErrors(t *testing.T) {
	savedMax := maxLogBytes
	t.Cleanup(func() { maxLogBytes = savedMax })
	maxLogBytes = 10

	badRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(badRoot, "logs"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openLog(badRoot); err == nil {
		t.Fatal("expected openLog error when logs is a file")
	}

	badFileRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(badFileRoot, "logs", "node.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := openLog(badFileRoot); err == nil {
		t.Fatal("expected openLog error when node.log is a directory")
	}

	bigDir := t.TempDir()
	bigLog, err := openLog(bigDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bigLog.Write([]byte("0123456789ab")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bigDir, "logs", "node.log.1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected rotation on empty file: %v", err)
	}
	bigLog.Close()

	dir := t.TempDir()
	lf, err := openLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := lf.Write([]byte("0123456789")); err != nil || n != 10 {
		t.Fatalf("exact write = %d, %v", n, err)
	}
	if _, err := os.Stat(lf.path + ".1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected rotation at exact maxLogBytes: %v", err)
	}
	if _, err := lf.Write([]byte("next")); err != nil {
		t.Fatal(err)
	}
	checkImage(t, lf.path+".1", "0123456789")
	checkImage(t, lf.path, "next")

	if _, err := lf.Write([]byte("0123456")); err != nil {
		t.Fatal(err)
	}
	checkImage(t, lf.path+".1", "next")
	checkImage(t, lf.path, "0123456")

	if err := os.Remove(lf.path + ".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lf.path+".1", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := lf.Write([]byte("overflow..")); err == nil {
		t.Fatal("expected rotate error when .1 is a non-empty directory")
	}
	if err := lf.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNodeAddrs(t *testing.T) {
	savedIfaces := ifaceAddrs
	t.Cleanup(func() { ifaceAddrs = savedIfaces })

	ifaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("no ifaces") }
	empty := nodeAddrs(47800, nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty nodeAddrs = %#v, want non-nil []string{}", empty)
	}

	mustCIDR := func(s string) *net.IPNet {
		ip, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		ipnet.IP = ip
		return ipnet
	}
	ifaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.TCPAddr{IP: net.IPv4(192, 0, 2, 99), Port: 80},
			&net.IPNet{IP: net.IP{1, 2}},
			mustCIDR("127.0.0.1/8"),
			mustCIDR("::1/128"),
			mustCIDR("169.254.10.20/16"),
			mustCIDR("fe80::1/64"),
			mustCIDR("0.0.0.0/0"),
			mustCIDR("192.0.2.10/24"),
			mustCIDR("192.0.2.10/24"),
			mustCIDR("2001:db8::1/64"),
		}, nil
	}
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

func TestOpenNodeStartupErrors(t *testing.T) {
	t.Run("stepLog fails", func(t *testing.T) {
		dir := socketRoot(t)
		os.WriteFile(filepath.Join(dir, "logs"), []byte("x"), 0o600)
		if err := runNode(t.Context(), dir, nil); err == nil {
			t.Fatal("expected stepLog error")
		}
	})

	t.Run("stepIdentity executablePath fails", func(t *testing.T) {
		savedExe := executablePath
		t.Cleanup(func() { executablePath = savedExe })
		executablePath = func() (string, error) { return "", io.ErrUnexpectedEOF }
		if _, err := openNode(t.Context(), socketRoot(t)); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("stepIdentity LoadKey fails", func(t *testing.T) {
		dir := socketRoot(t)
		os.WriteFile(filepath.Join(dir, "device.key"), []byte("short"), 0o600)
		if _, err := openNode(t.Context(), dir); err == nil {
			t.Fatal("expected LoadKey error")
		}
	})

	t.Run("stepIdentity store.Open fails", func(t *testing.T) {
		dir := socketRoot(t)
		os.MkdirAll(filepath.Join(dir, "node.db"), 0o700)
		if _, err := openNode(t.Context(), dir); err == nil {
			t.Fatal("expected store.Open error")
		}
	})

	t.Run("stepServices OpenLogs fails", func(t *testing.T) {
		dir := seedNodeDB(t, func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', 'not-json')")
			return err
		})
		if _, err := openNode(t.Context(), dir); err == nil {
			t.Fatal("expected OpenLogs error")
		}
	})

	t.Run("stepServices StartLoop fails", func(t *testing.T) {
		dir := seedNodeDB(t, func(tx *store.Tx) error {
			if _, err := tx.Exec(`INSERT INTO grp(g, seq, num) VALUES('g1', 2, 0)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('g1', 'group', 'g1', 1, 1, 1, 1, '{"r":"group","id":"g1"}')`); err != nil {
				return err
			}
			_, err := tx.Exec(`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('g1', 'member', 'u1', 2, 2, 1, 1, '{"r":"member","kind":"unknown"}')`)
			return err
		})
		if _, err := openNode(t.Context(), dir); err == nil {
			t.Fatal("expected StartLoop error")
		}
	})

	t.Run("stepServices host.New fails", func(t *testing.T) {
		dir := seedNodeDB(t, func(tx *store.Tx) error {
			_, err := tx.Exec("DROP TABLE session")
			return err
		})
		if _, err := openNode(t.Context(), dir); err == nil {
			t.Fatal("expected host.New error")
		}
	})

	t.Run("stepNetwork peerListen and tcpListen fail", func(t *testing.T) {
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
	})

	t.Run("stepNetwork localRV dial fails on canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := openNode(ctx, socketRoot(t)); err == nil {
			t.Fatal("expected canceled peer.Dial error")
		}
	})

	t.Run("stepStartup EnsureHomeConnections fails", func(t *testing.T) {
		dir := seedNodeDB(t, func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('homes', 'bad-json')")
			return err
		})
		if _, err := openNode(t.Context(), dir); err == nil {
			t.Fatal("expected EnsureHomeConnections error")
		}
	})
}

func seedNodeDB(t *testing.T, fn func(*store.Tx) error) string {
	t.Helper()
	dir := socketRoot(t)
	db, err := store.Open(filepath.Join(dir, "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Write(fn); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMultiNodePortFallbackWebAndLocalCommands(t *testing.T) {
	dir1, dir2 := socketRoot(t), socketRoot(t)
	l1 := openSocket(t, dir1)
	openSocket(t, dir2)

	n1, err := openNode(t.Context(), dir1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n1.Close)

	n2, err := openNode(t.Context(), dir2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n2.Close)

	if n1.httpPort == n2.httpPort {
		t.Fatalf("two nodes bound the same HTTP port %d", n1.httpPort)
	}
	n1.localPath.Write(t.Context(), proto.RoomID{Type: "join", Room: string(sesID("r1"))})

	respOK, err := http.Get("http://localhost:" + strconv.Itoa(n1.httpPort) + "/")
	if err != nil {
		t.Fatal(err)
	}
	respOK.Body.Close()
	if respOK.StatusCode != http.StatusOK {
		t.Fatalf("localhost GET / = %d", respOK.StatusCode)
	}

	done1 := make(chan struct{})
	go func() {
		n1.serve(l1)
		close(done1)
	}()

	rawConn, err := net.Dial("unix", filepath.Join(dir1, "sock"))
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(rawConn, "{}\n")
	badReply, _ := io.ReadAll(rawConn)
	rawConn.Close()
	if !strings.Contains(string(badReply), `"error"`) {
		t.Fatalf("bad opening reply = %q", badReply)
	}

	unauthConn, err := net.Dial("unix", filepath.Join(dir1, "sock"))
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(unauthConn, "{\"mcp\":\"bad-token\"}\n")
	io.ReadAll(unauthConn)
	unauthConn.Close()

	sesKey := host.SessionKey{Group: "g1", Member: "m1"}
	if err := n1.host.Sessions.Create(host.Session{SessionKey: sesKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: n1.key.ID(), Name: "Claude"}); err != nil {
		t.Fatal(err)
	}
	if err := n1.host.Sessions.Move(sesKey, host.Dormant, host.Working); err != nil {
		t.Fatal(err)
	}
	if err := n1.host.Sessions.Move(sesKey, host.Working, host.Waiting); err != nil {
		t.Fatal(err)
	}
	tok := n1.host.Sessions.Admit(sesKey, 3, false)
	mcpConn, err := net.Dial("unix", filepath.Join(dir1, "sock"))
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(mcpConn, fmt.Sprintf("{\"mcp\":%q}\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"t\",\"version\":\"1\"}}}\n", tok))
	mcpLine, err := bufio.NewReader(mcpConn).ReadString('\n')
	mcpConn.Close()
	if err != nil || !strings.Contains(mcpLine, `"solodawn"`) {
		t.Fatalf("MCP initialize = %q (%v)", mcpLine, err)
	}

	var statusOut bytes.Buffer
	if err := (Command{Cmd: "status"}).send(dir1, &statusOut); err != nil {
		t.Fatal(err)
	}
	gotStatus := statusOut.String()
	if !strings.Contains(gotStatus, "version "+Version) || !strings.Contains(gotStatus, "fingerprint "+n1.key.Fingerprint()) || !strings.Contains(gotStatus, "session g1 m1 waiting") {
		t.Fatalf("status output = %q", gotStatus)
	}

	if err := (Command{Cmd: "pair-browser"}).send(dir1, io.Discard); errText(err) != "node has no owner" {
		t.Fatalf("unowned pair-browser = %v", err)
	}
	if err := (Command{Cmd: "open"}).send(dir1, io.Discard); errText(err) != "node has no owner" {
		t.Fatalf("unowned open = %v", err)
	}
	name := "desk"
	if err := (Command{Cmd: "pair", Secret: "bad", Name: &name}).send(dir1, io.Discard); err == nil || !strings.Contains(err.Error(), "not a pairing secret") {
		t.Fatalf("bad secret pair = %v", err)
	}
	validSecret := identity.NewSecret().String()
	if err := (Command{Cmd: "pair", Secret: validSecret, Name: &name}).send(dir1, io.Discard); errText(err) != "pairing room is unavailable" {
		t.Fatalf("unowned valid pair = %v", err)
	}
	if err := (Command{Cmd: "bogus"}).send(dir1, io.Discard); errText(err) != "unknown command: bogus" {
		t.Fatalf("bogus cmd = %v", err)
	}

	n1.Close()
	<-done1
}

func TestAdoptionRendezvousAndForegroundStop(t *testing.T) {
	savedInstall := installService
	t.Cleanup(func() { installService = savedInstall })

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
	dir := seedNodeDB(t, func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('name', '\"office-mac\"')")
		return err
	})
	l := openSocket(t, dir)
	n, err := openNode(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	done := make(chan struct{})
	go func() {
		n.serve(l)
		close(done)
	}()

	// Local pair with --rendezvous saves the setting first and starts syncRendezvous.
	name := "desk"
	validSecret := identity.NewSecret().String()
	if err := (Command{Cmd: "pair", Secret: validSecret, Name: &name, Rendezvous: rvURL}).send(dir, io.Discard); errText(err) != "pairing room is unavailable" {
		t.Fatalf("pair with rendezvous = %v", err)
	}
	waitCond(t, func() bool { return len(n.remoteRVPaths()) == 1 })
	if got := n.readName(); got != "office-mac" {
		t.Fatalf("readName = %q", got)
	}

	// Adopt n over a paired ctl session; Dynamic OwnerFn allows group create on the existing home.Loop and onAdopt registers login service.
	browserKey := sesKey("adopt-browser")
	fc := serveConn(t, n.sessions, browserKey.ID(), true)
	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	var w1 proto.Welcome
	if err := proto.Decode(fc.next(t), &w1); err != nil || w1.Owner || w1.Name != "office-mac" || w1.Addrs == nil {
		t.Fatalf("pre-adopt welcome = %+v (%v)", w1, err)
	}
	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b1")
	e2 := sesAddEntry(browserKey, browserKey.ID(), 2, n.key.ID(), "node", "office-mac")
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1, e2}})

	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 2}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	var w2 proto.Welcome
	if err := proto.Decode(fc.next(t), &w2); err != nil || !w2.Owner || serviceCalls != 1 {
		t.Fatalf("post-adopt welcome = %+v (%v), serviceCalls=%d", w2, err, serviceCalls)
	}
	fc.next(t)

	failService = true
	n.onAdopt(browserKey.ID())
	if serviceCalls != 2 {
		t.Fatalf("serviceCalls = %d, want 2", serviceCalls)
	}

	gid := sesID("grp-adopted")
	createEnv := browserKey.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Main","kind":"project","forge":"none","repo":"/tmp/r","base":"main","checks":[],"member_name":"Owner"}`, gid, time.Now().UnixMilli(), gid))
	fc.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: createEnv})
	var okAns proto.Success
	if err := proto.Decode(fc.next(t), &okAns); err != nil || okAns.Re != 3 {
		t.Fatalf("group create after adopt = %+v (%v)", okAns, err)
	}
	fc.next(t)

	// Dial inbound WebRTC ctl connection from browserKey to n over rvURL.
	bPath, err := peer.Dial(t.Context(), rvURL, browserKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bPath.Close() })
	bSocks, err := peer.Listen(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bSocks.Close() })
	bDB := sesOpenStore(t)
	bPeer := peer.New(browserKey, bSocks, bDB, func(id string) bool { return id == n.key.ID() }, func(string) []peer.Route { return []peer.Route{bPath.Route()} }, func(*peer.Connection) {}, nil)
	go bPath.Serve(t.Context(), bPeer, func(string, []byte) error { return nil })
	rtcConn, err := bPeer.Offer(t.Context(), n.key.ID())
	if err != nil {
		t.Fatal(err)
	}
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
	rawWelc, err := rtcConn.Request(t.Context(), func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "rtc", Addrs: []string{}}
	})
	if err != nil {
		t.Fatal(err)
	}
	var wRTC proto.Welcome
	if err := proto.Decode(rawWelc, &wRTC); err != nil || !wRTC.Owner {
		t.Fatalf("WebRTC welcome = %+v (%v)", wRTC, err)
	}

	// Second node with setting('homes') connects its host to n's home over WebRTC at startup and shuts down cleanly.
	hostDir := socketRoot(t)
	hostKey, err := identity.LoadKey(hostDir)
	if err != nil {
		t.Fatal(err)
	}
	e3 := sesAddEntry(browserKey, browserKey.ID(), 3, hostKey.ID(), "node", "worker")
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3}})
	waitCond(t, func() bool { return n.logs.Known(hostKey.ID()) })

	seedWorkerNode(t, hostDir, browserKey.ID(), []proto.Envelope{e1, e2, e3}, rvURL, []proto.Home{
		{Group: gid, Home: n.key.ID()},
		{Group: sesID("grp-self"), Home: hostKey.ID()},
	})
	nHost, err := openNode(t.Context(), hostDir)
	if err != nil {
		t.Fatal(err)
	}
	waitCond(t, func() bool {
		_, ok := nHost.host.Home(n.key.ID())
		return ok
	})
	nHost.Close()
	nHost.dialHome(t.Context(), n.key.ID(), func(clientCtl) {})

	if err := (Command{Cmd: "pair", Secret: validSecret, Name: &name}).send(dir, io.Discard); errText(err) != ErrOwned.Error() {
		t.Fatalf("owned pair = %v, want %v", err, ErrOwned)
	}
	if err := (Command{Cmd: "pair-browser"}).send(dir, io.Discard); errText(err) != "pairing room is unavailable" {
		t.Fatalf("owned pair-browser = %v", err)
	}
	if err := (Command{Cmd: "open"}).send(dir, io.Discard); errText(err) != "pairing room is unavailable" {
		t.Fatalf("owned open = %v", err)
	}

	firstPath := n.remoteRVPaths()[0]
	firstPath.Close()
	waitCond(t, func() bool {
		paths := n.remoteRVPaths()
		return len(paths) == 1 && paths[0] != firstPath
	})

	if err := n.saveRendezvous([]string{}); err != nil {
		t.Fatal(err)
	}
	n.syncRendezvous()
	waitCond(t, func() bool { return len(n.remoteRVPaths()) == 0 })

	cctx, ccancel := context.WithCancel(t.Context())
	ccancel()
	if ran := n.serveOneRV(cctx, "ws://127.0.0.1:1/rv"); ran != 0 {
		t.Fatalf("serveOneRV error ran = %v", ran)
	}

	fc.Close()
	n.Close()
	<-done
	n.syncRendezvous()

	// SaveRendezvous error path when setting table is absent.
	errDir := socketRoot(t)
	errL := openSocket(t, errDir)
	errNode, err := openNode(t.Context(), errDir)
	if err != nil {
		t.Fatal(err)
	}
	errDone := make(chan struct{})
	go func() {
		errNode.serve(errL)
		close(errDone)
	}()
	dropSetting := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE setting")
		return err
	}
	if err := errNode.db.Write(dropSetting); err != nil {
		t.Fatal(err)
	}
	if err := (Command{Cmd: "pair", Secret: validSecret, Name: &name, Rendezvous: rvURL}).send(errDir, io.Discard); err == nil {
		t.Fatal("expected pair error when setting table is missing")
	}
	errNode.Close()
	<-errDone

	// Foreground stop via foregroundContext.
	defCtx, defStop := foregroundContext()
	defStop()
	<-defCtx.Done()

	savedFg := foregroundContext
	t.Cleanup(func() { foregroundContext = savedFg })
	stopCtx, stopNode := context.WithCancel(t.Context())
	foregroundContext = func() (context.Context, context.CancelFunc) { return stopCtx, func() {} }
	sigDir := socketRoot(t)
	sigL := openSocket(t, sigDir)
	sigErr := make(chan error, 1)
	go func() { sigErr <- runForegroundNode(sigDir, sigL) }()
	waitCond(t, func() bool {
		return (Command{Cmd: "status"}).send(sigDir, io.Discard) == nil
	})
	stopNode()
	if err := <-sigErr; err != nil {
		t.Fatalf("runForegroundNode stop = %v", err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	lo := net.IPv4(127, 0, 0, 1)
	gen := &turn.RelayAddressGeneratorStatic{RelayAddress: lo, Address: "127.0.0.1"}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "test",
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			return "peer", turn.GenerateAuthKey("peer", "test", "pw"), r.Username == "peer"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: pkt, RelayAddressGenerator: gen}},
	})
	if err != nil {
		t.Fatal(err)
	}
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
