//go:build netsim

package netsim

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
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

const (
	netsimRoleEnv = "SOLODAWN_NETSIM_ROLE"
	netsimPathEnv = "SOLODAWN_NETSIM_WANT_PATH"
	netsimRVURL   = "ws://" + WANIP + ":8080/rv"
	netsimTURN    = WANIP + ":3478"
	netsimNodeUDP = 47800
	netsimNodeTCP = 47801
)

const disableIPv6Script = `
for ns in "$@"; do
	ip netns exec "$ns" sysctl -qw net.ipv6.conf.all.disable_ipv6=1 net.ipv6.conf.default.disable_ipv6=1
done
`

const lanLinkScript = `
left=$1 right=$2
ip -n "$left" link add lan0 type veth peer name lan1 netns "$right"
ip -n "$left" address add 192.168.10.1/24 dev lan0
ip -n "$right" address add 192.168.10.2/24 dev lan1
ip -n "$left" link set lan0 up
ip -n "$right" link set lan1 up
`

func TestNetsimHelper(t *testing.T) {
	role := os.Getenv(netsimRoleEnv)
	if role == "" {
		return
	}
	must(t, runNetsimRole(role, os.Getenv(netsimPathEnv)))
}

func runNetsimRole(role, wantPath string) error {
	switch role {
	case "wan":
		return serveWANHelper()
	case "node":
		return serveNodeHelper(wantPath)
	case "browser":
		return runBrowserHelper(wantPath)
	default:
		return fmt.Errorf("unknown netsim role %q", role)
	}
}

func TestM1DirectAndRelayFallback(t *testing.T) {
	removed(t, "lan-direct", checkM1LANDirect)
	removed(t, "blocked-relay-fallback", checkM1RelayFallback)
}

func checkM1LANDirect(t *testing.T) Network {
	n := Start(t, PortRestricted, PortRestricted)
	run(t, disableIPv6Script, "sh", "-eu", "-s", "--", n.Left.Client, n.Right.Client)
	run(t, lanLinkScript, "sh", "-eu", "-s", "--", n.Left.Client, n.Right.Client)
	startDaemonHelper(t, n.WAN, "wan", "")
	nodeH := startDaemonHelper(t, n.Right.Client, "node", "direct")
	runBrowserInNS(t, n.Left.Client, "direct")
	expectHelperLine(t, nodeH, "NODE_PATH direct")
	return n
}

func checkM1RelayFallback(t *testing.T) Network {
	n := Start(t, PortRestricted, PortRestricted)
	run(t, disableIPv6Script, "sh", "-eu", "-s", "--", n.Left.Client, n.Right.Client)
	n.Right.StartUPnP(t)
	run(t, "", "ip", "netns", "exec", n.Right.Client, "upnpc", "-m", ClientIP,
		"-a", ClientIP, "47801", "47801", "TCP")
	n.SetBlocked(t, true, netsimNodeTCP)
	startDaemonHelper(t, n.WAN, "wan", "")
	nodeH := startDaemonHelper(t, n.Right.Client, "node", "relay")
	runBrowserInNS(t, n.Left.Client, "relay")
	expectHelperLine(t, nodeH, "NODE_PATH relay")
	return n
}

type daemonHelper struct {
	out    *bufio.Reader
	stderr bytes.Buffer
	stop   func()
}

func startDaemonHelper(t *testing.T, ns, role, wantPath string) *daemonHelper {
	t.Helper()
	exe, err := os.Executable()
	must(t, err)
	cmd := exec.CommandContext(t.Context(), "ip", "netns", "exec", ns, exe, "-test.run=^TestNetsimHelper$")
	cmd.Env = append(os.Environ(), netsimRoleEnv+"="+role, netsimPathEnv+"="+wantPath)
	stdin, err := cmd.StdinPipe()
	must(t, err)
	stdout, err := cmd.StdoutPipe()
	must(t, err)
	h := &daemonHelper{out: bufio.NewReader(stdout)}
	cmd.Stderr = &h.stderr
	must(t, cmd.Start())
	h.stop = sync.OnceFunc(func() { stopDaemonHelper(cmd, stdin) })
	t.Cleanup(h.stop)
	expectHelperLine(t, h, "READY")
	return h
}

func stopDaemonHelper(cmd *exec.Cmd, stdin io.Closer) {
	stdin.Close()
	done := make(chan struct{}) // single reader: stopDaemonHelper
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

func runBrowserInNS(t *testing.T, ns, wantPath string) {
	t.Helper()
	exe, err := os.Executable()
	must(t, err)
	cmd := exec.CommandContext(t.Context(), "ip", "netns", "exec", ns, exe, "-test.run=^TestNetsimHelper$")
	cmd.Env = append(os.Environ(), netsimRoleEnv+"=browser", netsimPathEnv+"="+wantPath)
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	must(t, cmd.Run())
}

func expectHelperLine(t *testing.T, h *daemonHelper, want string) {
	t.Helper()
	line, err := h.out.ReadString('\n')
	if err == nil && strings.TrimSpace(line) == want {
		return
	}
	h.stop()
	rest, rErr := io.ReadAll(h.out)
	t.Fatalf("helper reported %q (readErr=%v rest=%q restErr=%v stderr=%q), want %q",
		strings.TrimSpace(line), err, string(rest), rErr, h.stderr.String(), want)
}

func serveWANHelper() error {
	turnSrv, ice, err := startWANTURN()
	if err != nil {
		return err
	}
	defer turnSrv.Close()
	inner := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	defer inner.Close()
	innerWS := "ws" + strings.TrimPrefix(inner.URL, "http") + "/rv"
	ln, err := net.Listen("tcp4", WANIP+":8080")
	if err != nil {
		return err
	}
	defer ln.Close()
	srv := &http.Server{Handler: wanProxyHandler(innerWS, ice)}
	defer srv.Close()
	go srv.Serve(ln)
	fmt.Println("READY")
	io.Copy(io.Discard, os.Stdin)
	return nil
}

func startWANTURN() (*turn.Server, []proto.ICEServer, error) {
	udp, err := net.ListenPacket("udp4", netsimTURN)
	if err != nil {
		return nil, nil, err
	}
	ip := net.ParseIP(WANIP)
	gen := &turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: WANIP}
	auth := func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "solodawn", "secret"), r.Username == "peer"
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             "solodawn",
		AuthHandler:       auth,
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen}},
	})
	if err != nil {
		udp.Close()
		return nil, nil, err
	}
	ice := []proto.ICEServer{
		{URLs: []string{"stun:" + netsimTURN}},
		{URLs: []string{"turn:" + netsimTURN + "?transport=udp"}, Username: "peer", Credential: "secret"},
	}
	return srv, ice, nil
}

func wanProxyHandler(innerWS string, ice []proto.ICEServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rv", func(w http.ResponseWriter, r *http.Request) {
		serveWANWS(w, r, innerWS, ice)
	})
	return mux
}

func serveWANWS(w http.ResponseWriter, r *http.Request, innerWS string, ice []proto.ICEServer) {
	down, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer down.CloseNow()
	down.SetReadLimit(16_384)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	up, _, err := websocket.Dial(ctx, innerWS, nil)
	if err != nil {
		return
	}
	defer up.CloseNow()
	up.SetReadLimit(16_384)
	done := make(chan struct{}, 2) // single reader: serveWANWS
	go func() { pumpWS(ctx, down, up, nil); done <- struct{}{} }()
	go func() { pumpWS(ctx, up, down, func(b []byte) []byte { return injectICE(b, ice) }); done <- struct{}{} }()
	<-done
}

func pumpWS(ctx context.Context, src, dst *websocket.Conn, mapBytes func([]byte) []byte) {
	for {
		typ, data, err := src.Read(ctx)
		if err != nil {
			return
		}
		if mapBytes != nil {
			data = mapBytes(data)
		}
		if dst.Write(ctx, typ, data) != nil {
			return
		}
	}
}

func injectICE(data []byte, ice []proto.ICEServer) []byte {
	var f proto.RVFrame
	if proto.Decode(data, &f) != nil || (f.Type != "ready" && f.Type != "ice") {
		return data
	}
	var cfg proto.ICEConfig
	if proto.Decode(data, &cfg) != nil {
		return data
	}
	cfg.ICE = ice
	out, err := json.Marshal(cfg)
	if err != nil {
		return data
	}
	return out
}

type helperNodeStack struct {
	db       *store.Store
	loop     *home.Loop
	link     *node.LocalLink
	sessions *node.Sessions
	logs     *node.Logs
}

func (s *helperNodeStack) close() {
	s.link.Close()
	s.loop.Close()
	s.db.Close()
}

func serveNodeHelper(wantPath string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stack, err := openHelperServices()
	if err != nil {
		return err
	}
	defer stack.close()
	socks, err := peer.Listen(netsimNodeUDP, netsimNodeTCP, "test")
	if err != nil {
		return err
	}
	defer socks.Close()
	pathDone := make(chan string, 1) // single reader: reportNodePath
	rvPath, p, err := startNodePeer(ctx, socks, stack, wantPath, pathDone)
	if err != nil {
		return err
	}
	defer rvPath.Close()
	go rvPath.Serve(ctx, p, func(string, []byte) error { return nil })
	fmt.Println("READY")
	reportNodePath(pathDone)
	io.Copy(io.Discard, os.Stdin)
	return nil
}

func reportNodePath(pathDone <-chan string) {
	select {
	case msg := <-pathDone:
		fmt.Println(msg)
	case <-time.After(15 * time.Second):
		fmt.Println("NODE_PATH timeout")
	}
}

func openHelperServices() (*helperNodeStack, error) {
	nodeKey, browserKey := netsimKey("node"), netsimKey("browser")
	db, err := store.Open(":memory:")
	if err != nil {
		return nil, err
	}
	if err := seedNetsimOwner(db, browserKey, nodeKey); err != nil {
		db.Close()
		return nil, err
	}
	logs, err := node.OpenLogs(db, nodeKey.ID())
	if err != nil {
		db.Close()
		return nil, err
	}
	stack, err := startHelperNodeCore(db, nodeKey, logs)
	if err != nil {
		db.Close()
		return nil, err
	}
	return stack, nil
}

func startHelperNodeCore(db *store.Store, nodeKey identity.Key, logs *node.Logs) (*helperNodeStack, error) {
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: nodeKey.ID(), Owner: logs.Owner(),
		Logs: logs.Log, Tables: home.NodeTables(),
	})
	if err != nil {
		return nil, err
	}
	h, _, err := host.New(db, nodeKey, time.Now)
	if err != nil {
		loop.Close()
		return nil, err
	}
	link := node.AttachLocal(h, loop)
	nameFn := func() string { return "netsim-node" }
	sessions := node.NewSessions(node.SessionConfig{
		ID: nodeKey.ID(), Version: "v0.1.0", Store: db, Logs: logs, Home: loop, Host: h, Name: nameFn,
	})
	node.NewSurface(node.SurfaceConfig{
		ID: nodeKey.ID(), Version: "v0.1.0", Store: db, Sessions: sessions, Host: h, Name: nameFn, Agents: h.Agents,
	})
	return &helperNodeStack{db: db, loop: loop, link: link, sessions: sessions, logs: logs}, nil
}

func startNodePeer(ctx context.Context, socks *peer.Sockets, stack *helperNodeStack, wantPath string, pathDone chan<- string) (*peer.Path, *peer.Peer, error) {
	nodeKey := netsimKey("node")
	rvPath, err := peer.Dial(ctx, netsimRVURL, nodeKey)
	if err != nil {
		return nil, nil, err
	}
	routes := func(string) []peer.Route { return []peer.Route{rvPath.Route()} }
	inbound := func(c *peer.Connection) {
		go observeConnPath(c, wantPath, pathDone)
		go stack.sessions.Serve(ctx, c.Device, false, c)
	}
	p := peer.New(nodeKey, socks, stack.db, stack.logs.Known, routes, inbound, nil)
	return rvPath, p, nil
}

func observeConnPath(c *peer.Connection, want string, out chan<- string) {
	if err := waitPathWithin(c, want, time.Second); err != nil {
		sendOnce(out, "NODE_PATH "+err.Error())
		return
	}
	sendOnce(out, "NODE_PATH "+want)
}

func sendOnce(out chan<- string, msg string) {
	select {
	case out <- msg:
	default:
	}
}

func runBrowserHelper(wantPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	browserKey, nodeID := netsimKey("browser"), netsimKey("node").ID()
	socks, db, err := openBrowserPeerDeps()
	if err != nil {
		return err
	}
	defer socks.Close()
	defer db.Close()
	rvPath, err := peer.Dial(ctx, netsimRVURL, browserKey)
	if err != nil {
		return err
	}
	defer rvPath.Close()
	routes := func(string) []peer.Route { return []peer.Route{rvPath.Route()} }
	p := peer.New(browserKey, socks, db, func(id string) bool { return id == nodeID }, routes, func(*peer.Connection) {}, nil)
	go rvPath.Serve(ctx, p, func(string, []byte) error { return nil })
	return dialAndVerifyPath(ctx, p, nodeID, wantPath)
}

func openBrowserPeerDeps() (*peer.Sockets, *store.Store, error) {
	socks, err := peer.Listen(0, 0, "test")
	if err != nil {
		return nil, nil, err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return nil, nil, err
	}
	return socks, db, nil
}

func dialAndVerifyPath(ctx context.Context, p *peer.Peer, nodeID, wantPath string) error {
	conn, err := p.Offer(ctx, nodeID)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := waitReady(ctx, conn); err != nil {
		return err
	}
	go drainNetsimPushes(ctx, conn)
	if err := waitPathWithin(conn, wantPath, time.Second); err != nil {
		return err
	}
	if err := sendNetsimHello(ctx, conn); err != nil {
		return err
	}
	return pollNetsimHostInfo(ctx, conn, nodeID, wantPath, 3)
}

func drainNetsimPushes(ctx context.Context, conn *peer.Connection) {
	for {
		if _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func waitReady(ctx context.Context, conn *peer.Connection) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-conn.Done():
		return errors.New("connection closed before ready")
	case <-conn.Ready():
		return nil
	}
}

func waitPathWithin(conn *peer.Connection, want string, limit time.Duration) error {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	for {
		if conn.Path() == want {
			return nil
		}
		if !waitPathSignal(conn, deadline.C) {
			return fmt.Errorf("path %q after %s, want %q", conn.Path(), limit, want)
		}
	}
}

func waitPathSignal(conn *peer.Connection, timeout <-chan time.Time) bool {
	select {
	case <-conn.PathChanged():
		return true
	case <-conn.Done():
		return false
	case <-timeout:
		return false
	}
}

func sendNetsimHello(ctx context.Context, conn *peer.Connection) error {
	build := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 3, Kind: "browser", Name: "browser", Addrs: []string{}}
	}
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return err
	}
	var w proto.Welcome
	if err := proto.Decode(raw, &w); err != nil || !w.Owner {
		return fmt.Errorf("welcome = %s (%v)", raw, err)
	}
	return nil
}

func pollNetsimHostInfo(ctx context.Context, conn *peer.Connection, wantDevice, wantPath string, seconds int) error {
	if err := checkNetsimHostInfo(ctx, conn, wantDevice, wantPath); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range seconds {
		if err := waitTick(ctx, ticker.C); err != nil {
			return err
		}
		if err := checkNetsimHostInfo(ctx, conn, wantDevice, wantPath); err != nil {
			return err
		}
	}
	return nil
}

func waitTick(ctx context.Context, tick <-chan time.Time) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tick:
		return nil
	}
}

func checkNetsimHostInfo(ctx context.Context, conn *peer.Connection, wantDevice, wantPath string) error {
	if got := conn.Path(); got != wantPath {
		return fmt.Errorf("path changed to %q, want %q", got, wantPath)
	}
	build := func(id int64) any { return proto.Request{Type: "host.info", ID: id} }
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return err
	}
	var info proto.HostInfo
	if err := proto.Decode(raw, &info); err != nil || info.Device != wantDevice {
		return fmt.Errorf("host.info = %s (%v)", raw, err)
	}
	return nil
}

func netsimKey(label string) identity.Key {
	seed := sha256.Sum256([]byte("SoloDawn netsim M1: " + label))
	return identity.Key(ed25519.NewKeyFromSeed(seed[:]))
}

func seedNetsimOwner(db *store.Store, owner, nodeKey identity.Key) error {
	now := time.Now().UnixMilli()
	e1 := nodetest.SignDeviceAdd(owner, owner.ID(), 1, "browser", "browser", owner.ID(), now)
	e2 := nodetest.SignDeviceAdd(owner, owner.ID(), 2, "node", "netsim-node", nodeKey.ID(), now)
	ownerJSON, err := json.Marshal(owner.ID())
	if err != nil {
		return err
	}
	raw1, err1 := json.Marshal(e1)
	raw2, err2 := json.Marshal(e2)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	write := func(tx *store.Tx) error {
		_, wErr1 := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", string(ownerJSON))
		_, wErr2 := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, 1, ?), (?, 2, ?)", owner.ID(), string(raw1), owner.ID(), string(raw2))
		return errors.Join(wErr1, wErr2)
	}
	return db.Write(write)
}
