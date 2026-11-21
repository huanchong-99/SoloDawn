package node

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
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
	"strings"
	"sync"
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

	// Empty file with write > maxLogBytes writes directly without rotating an empty .1.
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

	// Exact boundary == maxLogBytes does not rotate; +1 rotates; second rotation keeps 1 generation.
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

	// Rotation failure when node.log.1 is a non-empty directory.
	if err := os.Remove(lf.path + ".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lf.path+".1", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := lf.Write([]byte("overflow!!")); err == nil {
		t.Fatal("expected rotate error when .1 is a non-empty directory")
	}
	if err := lf.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserCommandAndReplaceProcess(t *testing.T) {
	if prog, args := browserCommand("darwin", "http://localhost:47800"); prog != "open" || !slices.Equal(args, []string{"http://localhost:47800"}) {
		t.Fatalf("darwin browserCommand = %q, %v", prog, args)
	}
	if prog, args := browserCommand("windows", "http://localhost:47800"); prog != "rundll32" || !slices.Equal(args, []string{"url.dll,FileProtocolHandler", "http://localhost:47800"}) {
		t.Fatalf("windows browserCommand = %q, %v", prog, args)
	}
	if prog, args := browserCommand("linux", "http://localhost:47800"); prog != "xdg-open" || !slices.Equal(args, []string{"http://localhost:47800"}) {
		t.Fatalf("linux browserCommand = %q, %v", prog, args)
	}

	savedStart := startBrowser
	t.Cleanup(func() { startBrowser = savedStart })
	var gotProg string
	var gotArgs []string
	startBrowser = func(exe string, args ...string) error {
		gotProg, gotArgs = exe, args
		return nil
	}
	if err := openBrowserURL("http://localhost:47800/#pair=ABCD"); err != nil || gotProg == "" || len(gotArgs) == 0 {
		t.Fatalf("openBrowserURL = %v, %q, %v", err, gotProg, gotArgs)
	}

	if err := replaceProcess(filepath.Join(t.TempDir(), "missing-binary"), []string{"solodawn"}, os.Environ()); err == nil {
		t.Fatal("expected replaceProcess error on missing binary")
	}
}

func TestNodeAddrsAndLocalHostFilter(t *testing.T) {
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

	h := nodeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range []struct {
		path, host string
		code       int
	}{
		{"/", "localhost:47800", http.StatusOK},
		{"/", "127.0.0.1:47800", http.StatusOK},
		{"/", "localhost", http.StatusOK},
		{"/", "127.0.0.1", http.StatusOK},
		{"/", "example.com:47800", http.StatusForbidden},
		{"/", "192.0.2.10:47800", http.StatusForbidden},
		{"/", "", http.StatusForbidden},
		{"/rv", "192.0.2.10:47800", http.StatusOK},
		{"/rv", "example.com", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+tc.path, nil)
		req.Host = tc.host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Errorf("%s Host=%q: code %d, want %d", tc.path, tc.host, rec.Code, tc.code)
		}
	}
}

func TestOpenNodeStartupErrors(t *testing.T) {
	t.Run("stepLog fails", func(t *testing.T) {
		dir := socketRoot(t)
		os.WriteFile(filepath.Join(dir, "logs"), []byte("x"), 0o600)
		if _, err := runNode(t.Context(), dir, nil); err == nil {
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

	// Occupy defaultWebPort if n1 happened to fall back already, ensuring n2 exercises fallback on both TCP and peer sockets.
	n2, err := openNode(t.Context(), dir2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n2.Close)

	if n1.httpPort == n2.httpPort {
		t.Fatalf("two nodes bound the same HTTP port %d", n1.httpPort)
	}
	if n1.relayStatus() != nil {
		t.Fatalf("expected nil relayStatus without remote rendezvous, got %+v", n1.relayStatus())
	}

	// Serve embedded web UI on localhost and refuse external Host headers while allowing /rv.
	respOK, err := http.Get(n1.localWebURL() + "/")
	if err != nil {
		t.Fatal(err)
	}
	respOK.Body.Close()
	if respOK.StatusCode != http.StatusOK {
		t.Fatalf("localhost GET / = %d", respOK.StatusCode)
	}
	badReq, _ := http.NewRequest(http.MethodGet, n1.localWebURL()+"/", nil)
	badReq.Host = "evil.example:47800"
	respBad, err := http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatal(err)
	}
	respBad.Body.Close()
	if respBad.StatusCode != http.StatusForbidden {
		t.Fatalf("evil Host GET / = %d, want 403", respBad.StatusCode)
	}

	// Run serve(l1) and exercise invalid opening, MCP shim, status (with active session), pair, pair-browser, open and unknown command.
	done1 := make(chan string, 1)
	go func() { done1 <- n1.serve(l1) }()

	// Invalid opening line on live socket.
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

	// Unknown MCP token closes connection; admitted token serves MCP initialize.
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

	// Unowned pair-browser, open, pair with invalid secret, pair with valid secret, unknown cmd.
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
	if restart := <-done1; restart != "" {
		t.Fatalf("restartBin = %q, want empty", restart)
	}
}

func TestAdoptionRendezvousWatchPresenceAndInboundPeer(t *testing.T) {
	savedDelay := rvRetryDelay
	t.Cleanup(func() { rvRetryDelay = savedDelay })
	rvRetryDelay = 20 * time.Millisecond

	rvURL := startTestRVWithRelay(t)

	dir := seedNodeDB(t, func(tx *store.Tx) error {
		raw, _ := json.Marshal([]string{rvURL})
		if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('rendezvous', ?)", string(raw)); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('name', '\"office-mac\"')")
		return err
	})
	l := openSocket(t, dir)
	n, err := openNode(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	done := make(chan string, 1)
	go func() { done <- n.serve(l) }()

	// Wait until remote RV path connects and quota is recorded.
	waitCond(t, func() bool { return len(n.remoteRVPaths()) == 1 && n.relayStatus() != nil })
	if got := n.readName(); got != "office-mac" {
		t.Fatalf("readName = %q", got)
	}
	if len(n.sharedPaths()) != 2 {
		t.Fatalf("sharedPaths = %d, want 2", len(n.sharedPaths()))
	}

	// Subscribe to presence for browserKey before browserKey connects to rvSrv.
	browserKey := sesKey("adopt-browser")
	subCtx, subCancel := context.WithCancel(t.Context())
	sub1 := n.subscribePresence(subCtx, browserKey.ID())
	sub2Ctx, sub2Cancel := context.WithCancel(t.Context())
	n.subscribePresence(sub2Ctx, browserKey.ID())
	sub2Cancel()

	// Adopt n over a paired ctl session; OnAdopt restarts home.Loop with Owner, re-attaches LocalLink and watches browserKey on rvSrv.
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
	if err := proto.Decode(fc.next(t), &w2); err != nil || !w2.Owner {
		t.Fatalf("post-adopt welcome = %+v (%v)", w2, err)
	}
	fc.next(t) // devlog frame

	// Create a group on the newly restarted home.Loop over the same ctl connection.
	gid := sesID("grp-adopted")
	createEnv := browserKey.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Main","kind":"project","forge":"none","repo":"/tmp/r","base":"main","checks":[],"member_name":"Owner"}`, gid, time.Now().UnixMilli(), gid))
	fc.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: createEnv})
	var okAns proto.Success
	if err := proto.Decode(fc.next(t), &okAns); err != nil || okAns.Re != 3 {
		t.Fatalf("group create after OnAdopt = %+v (%v)", okAns, err)
	}
	fc.next(t) // groups push

	// Connect browserKey to rvSrv to trigger presence On for watched browserKey and inbound WebRTC connection.
	bPath, err := peer.Dial(t.Context(), rvURL, browserKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bPath.Close() })
	select {
	case <-sub1:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for presence notification")
	}
	// Duplicate notify when buffer full is non-blocking; cancel cleans up last subscriber.
	n.notifyPresence(browserKey.ID())
	n.notifyPresence(browserKey.ID())
	subCancel()
	waitCond(t, func() bool {
		n.mu.Lock()
		defer n.mu.Unlock()
		return len(n.presence) == 0
	})

	// Dial inbound WebRTC ctl connection from browserKey to n over rvSrv.
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

	// Owned pair, pair-browser and open commands over local socket.
	name := "desk"
	if err := (Command{Cmd: "pair", Secret: identity.NewSecret().String(), Name: &name}).send(dir, io.Discard); errText(err) != ErrOwned.Error() {
		t.Fatalf("owned pair = %v, want %v", err, ErrOwned)
	}
	if err := (Command{Cmd: "pair-browser"}).send(dir, io.Discard); errText(err) != "pairing room is unavailable" {
		t.Fatalf("owned pair-browser = %v", err)
	}
	if err := (Command{Cmd: "open"}).send(dir, io.Discard); errText(err) != "pairing room is unavailable" {
		t.Fatalf("owned open = %v", err)
	}

	// Cover presence Off, reconnect with Owner set, pruneRendezvous, rvNextDelay and onAdopt startHomeLoop error.
	n.onRVEvent(t.Context(), n.localPath, "presence", []byte(`{"t":"presence","k":"`+browserKey.ID()+`","on":false}`))

	// Drop the remote RV socket while still configured so serveOneRV clears rvPaths and runRVLoop reconnects (sending watch as owned).
	firstPath := n.remoteRVPaths()[0]
	firstPath.Close()
	waitCond(t, func() bool {
		paths := n.remoteRVPaths()
		return len(paths) == 1 && paths[0] != firstPath
	})

	clearRV := func(tx *store.Tx) error {
		_, err := tx.Exec("UPDATE setting SET value = '[]' WHERE key = 'rendezvous'")
		return err
	}
	if err := n.db.Write(clearRV); err != nil {
		t.Fatal(err)
	}
	n.syncRendezvous()
	waitCond(t, func() bool { return len(n.remoteRVPaths()) == 0 })

	restoreRV := func(tx *store.Tx) error {
		raw, _ := json.Marshal([]string{rvURL})
		_, err := tx.Exec("UPDATE setting SET value = ? WHERE key = 'rendezvous'", string(raw))
		return err
	}
	if err := n.db.Write(restoreRV); err != nil {
		t.Fatal(err)
	}
	n.syncRendezvous()
	waitCond(t, func() bool { return len(n.remoteRVPaths()) == 1 })

	// Corrupt row causes onAdopt error path to log cleanly without crashing.
	badRow := func(tx *store.Tx) error {
		if _, err := tx.Exec(`INSERT INTO grp(g, seq, num) VALUES('gbad', 2, 0)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('gbad', 'group', 'gbad', 1, 1, 1, 1, '{"r":"group","id":"gbad"}')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO row(g, r, id, seq, pos, ts, mt, data) VALUES('gbad', 'member', 'x', 2, 2, 1, 1, '{"r":"member","kind":"badkind"}')`)
		return err
	}
	if err := n.db.Write(badRow); err != nil {
		t.Fatal(err)
	}
	n.onAdopt(browserKey.ID())

	// rvNextDelay resets after 1 minute run and exits on canceled context.
	if d := rvNextDelay(t.Context(), 8*time.Millisecond, time.Minute); d != 2*rvRetryDelay {
		t.Fatalf("rvNextDelay after 1m = %v, want %v", d, 2*rvRetryDelay)
	}
	if d := rvNextDelay(t.Context(), 5*time.Millisecond, 0); d != 10*time.Millisecond {
		t.Fatalf("rvNextDelay backoff = %v", d)
	}
	cctx, ccancel := context.WithCancel(t.Context())
	ccancel()
	if d := rvNextDelay(cctx, time.Hour, 0); d != 0 {
		t.Fatalf("rvNextDelay canceled = %v", d)
	}

	// Rendezvous dial failure in serveOneRV returns 0.
	if ran := n.serveOneRV(cctx, "ws://127.0.0.1:1/rv"); ran != 0 {
		t.Fatalf("serveOneRV error ran = %v", ran)
	}

	fc.Close()
	n.Close()
	<-done
	// syncRendezvous after Close returns immediately.
	n.syncRendezvous()
}

func TestSelfUpdateHoldBusyAndForegroundRestart(t *testing.T) {
	savedVersion, savedRelease, savedEvery, savedExec, savedTrans := Version, releaseURL, updateEvery, execReplace, updateTransport
	t.Cleanup(func() {
		Version = savedVersion
		releaseURL = savedRelease
		updateEvery = savedEvery
		execReplace = savedExec
		updateTransport = savedTrans
	})

	// Non-TurnEnded and nil-wake listener cases, plus default holdForUpdate.
	tmpDB := sesOpenStore(t)
	h0, _, err := host.New(tmpDB, sesKey("h0"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if reason, held := holdForUpdate(h0, host.SessionKey{}); held || reason != "" {
		t.Fatalf("default holdForUpdate = %q, %v", reason, held)
	}
	listenNodeUpdate(h0, host.SessionKey{}, host.Event{Kind: host.Ready})
	listenNodeUpdate(h0, host.SessionKey{}, host.Event{Kind: host.TurnEnded})

	// Set up TLS release server with mismatching then matching v1.1.0 release.
	Version = "v1.0.0"
	updateEvery = 30 * time.Millisecond
	target := updateTarget(runtime.GOOS, runtime.GOARCH)
	fileName := "solodawn_v1.1.0_" + target
	goodBin := []byte("updated-binary-image")
	sum := sha256.Sum256(goodBin)

	var serveGood sync.Mutex
	allowGood := false
	mux := http.NewServeMux()
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", sum, fileName)
	})
	mux.HandleFunc("/"+fileName, func(w http.ResponseWriter, _ *http.Request) {
		serveGood.Lock()
		ok := allowGood
		serveGood.Unlock()
		if !ok {
			w.Write([]byte("corrupted"))
			return
		}
		w.Write(goodBin)
	})
	tlsSrv := httptest.NewTLSServer(mux)
	t.Cleanup(tlsSrv.Close)
	releaseURL = tlsSrv.URL
	updateTransport = tlsSrv.Client().Transport

	savedExe := executablePath
	t.Cleanup(func() { executablePath = savedExe })
	fakeBin := filepath.Join(t.TempDir(), "solodawn")
	writeImage(t, fakeBin, "old-binary")
	executablePath = func() (string, error) { return fakeBin, nil }

	dir := nodeTestDir(t)
	l := openSocket(t, dir)
	n, err := openNode(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // allow ticker.C in runUpdateLoop to fire once

	// Create a Working session so n.isBusy() is true when the valid update lands.
	sKey := host.SessionKey{Group: "g1", Member: "m1"}
	if err := n.host.Sessions.Create(host.Session{SessionKey: sKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: n.key.ID(), Name: "C"}); err != nil {
		t.Fatal(err)
	}
	if err := n.host.Sessions.Move(sKey, host.Dormant, host.Working); err != nil {
		t.Fatal(err)
	}

	// First check gets mismatch (and tryApplyLocked sees !pending).
	n.updMu.Lock()
	n.updater.nextCheck = time.Time{}
	n.updMu.Unlock()
	n.checkAndApplyUpdate(time.Now())
	if st := n.updateStatus(); st == nil || st.Status != "mismatch" || st.Version != "v1.1.0" {
		t.Fatalf("mismatch updateStatus = %+v", st)
	}

	// Second check downloads valid release while session is Working: status becomes pending, hold resets to false, no restart yet.
	serveGood.Lock()
	allowGood = true
	serveGood.Unlock()
	n.updMu.Lock()
	n.updater.nextCheck = time.Time{}
	n.updMu.Unlock()
	n.checkAndApplyUpdate(time.Now().Add(25 * time.Hour))
	if st := n.updateStatus(); st == nil || st.Status != "pending" {
		t.Fatalf("pending updateStatus = %+v", st)
	}
	if reason, held := n.host.Held(sKey); held || reason != "" {
		t.Fatalf("hold while still working = %q, %v", reason, held)
	}

	// Also verify checkAndApplyUpdate logs error cleanly when ReleaseURL is unreachable.
	n.updMu.Lock()
	n.updater.Status = nil
	n.updater.nextCheck = time.Time{}
	n.updater.ReleaseURL = "http://invalid.example"
	n.updMu.Unlock()
	n.checkAndApplyUpdate(time.Now().Add(50 * time.Hour))
	if n.updateStatus() != nil {
		t.Fatalf("expected nil updateStatus after reset, got %+v", n.updateStatus())
	}

	// Restore pending status, start serve(l), then end the turn so listener wakes runUpdateLoop and triggers restart.
	n.updMu.Lock()
	n.updater.Status = &UpdateStatus{Version: "v1.1.0", Status: "pending"}
	n.updMu.Unlock()
	n.notifyWake()
	n.notifyWake()

	serveDone := make(chan string, 1)
	go func() { serveDone <- n.serve(l) }()

	if err := n.host.Sessions.Move(sKey, host.Working, host.Waiting); err != nil {
		t.Fatal(err)
	}
	n.host.Report(sKey, host.Event{Kind: host.TurnEnded})

	select {
	case gotBin := <-serveDone:
		if gotBin != fakeBin {
			t.Fatalf("serve restartBin = %q, want %q", gotBin, fakeBin)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for pending update restart")
	}
	if reason, held := n.host.Held(sKey); !held || reason != "updating" {
		t.Fatalf("hold during restart drain = %q, %v", reason, held)
	}
	n.Close()

	// Test runForegroundNode: startup error, clean SIGINT stop, and update execReplace.
	badDir := socketRoot(t)
	os.WriteFile(filepath.Join(badDir, "logs"), []byte("x"), 0o600)
	badL := openSocket(t, badDir)
	if err := runForegroundNode(badDir, badL); err == nil {
		t.Fatal("expected runForegroundNode error on bad dir")
	}

	// Clean stop via foregroundContext.
	defCtx, defStop := foregroundContext()
	defStop()
	<-defCtx.Done()

	savedFgCtx := foregroundContext
	t.Cleanup(func() { foregroundContext = savedFgCtx })
	stopCtx, stopNode := context.WithCancel(t.Context())
	foregroundContext = func() (context.Context, context.CancelFunc) { return stopCtx, func() {} }

	Version = "dev"
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
	foregroundContext = savedFgCtx

	// Restart path invokes execReplace after releasing lock.
	Version = "v1.0.0"
	execDir := socketRoot(t)
	execL, err := ListenLocal(execDir)
	if err != nil {
		t.Fatal(err)
	}
	var execCalled string
	execReplace = func(bin string, _ []string, _ []string) error {
		execCalled = bin
		// Lock must already be released before execReplace.
		rebound, err := ListenLocal(execDir)
		if err != nil {
			return fmt.Errorf("lock still held in execReplace: %w", err)
		}
		rebound.Close()
		return nil
	}
	writeImage(t, fakeBin, "old-binary")
	if err := runForegroundNode(execDir, execL); err != nil {
		t.Fatalf("runForegroundNode update restart = %v", err)
	}
	if execCalled != fakeBin {
		t.Fatalf("execCalled = %q, want %q", execCalled, fakeBin)
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
