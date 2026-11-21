// Package node assembles the SoloDawn runtime.
package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Version is the release tag, which the release build sets with
// -ldflags "-X github.com/huanchong-99/SoloDawn/internal/node.Version=<tag>".
var Version = "dev"

const (
	defaultWebPort = 47800
	defaultTCPPort = 47801
)

var (
	maxLogBytes       int64 = 10 << 20
	rvRetryDelay            = time.Second
	peerListen              = peer.Listen
	tcpListen               = net.Listen
	ifaceAddrs              = net.InterfaceAddrs
	installService          = InstallService
	foregroundContext       = func() (context.Context, context.CancelFunc) {
		return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
)

type logFile struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func openLog(dir string) (*logFile, error) {
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		return nil, err
	}
	lf := &logFile{path: filepath.Join(dir, "logs", "node.log")}
	return lf, lf.openCurrent()
}

func (l *logFile) openCurrent() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	size, _ := f.Seek(0, io.SeekEnd)
	l.file, l.size = f, size
	return nil
}

func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size > 0 && l.size+int64(len(p)) > maxLogBytes {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	return n, err
}

func (l *logFile) rotate() error {
	l.file.Close()
	return errors.Join(os.Rename(l.path, l.path+".1"), l.openCurrent())
}

func (l *logFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// nodeRuntime holds the running components of one SoloDawn node.
// mu is a leaf lock guarding rvPaths and rvStop.
type nodeRuntime struct {
	ctx       context.Context
	stop      context.CancelFunc
	dir, exe  string
	logFile   *logFile
	prevLog   io.Writer
	key       identity.Key
	db        *store.Store
	logs      *Logs
	loop      *home.Loop
	local     *LocalLink
	host      *host.Host
	saved     []host.Session
	sessions  *Sessions
	socks     *peer.Sockets
	peer      *peer.Peer
	httpSrv   *http.Server
	httpPort  int
	localPath *peer.Path
	bg        sync.WaitGroup
	closeOnce sync.Once

	mu      sync.Mutex
	rvPaths map[string]*peer.Path
	rvStop  map[string]context.CancelFunc
}

func runForegroundNode(dir string, l *Local) error {
	ctx, stop := foregroundContext()
	defer stop()
	return runNode(ctx, dir, l)
}

func runNode(ctx context.Context, dir string, l *Local) error {
	n, err := openNode(ctx, dir)
	if err != nil {
		return err
	}
	defer n.Close()
	n.serve(l)
	return nil
}

func openNode(parent context.Context, dir string) (*nodeRuntime, error) {
	ctx, cancel := context.WithCancel(parent)
	n := &nodeRuntime{
		ctx: ctx, stop: cancel, dir: dir,
		rvPaths: map[string]*peer.Path{}, rvStop: map[string]context.CancelFunc{},
	}
	for _, step := range []func() error{n.stepLog, n.stepIdentity, n.stepServices, n.stepNetwork, n.stepStartup} {
		if err := step(); err != nil {
			n.Close()
			return nil, err
		}
	}
	return n, nil
}

func (n *nodeRuntime) stepLog() error {
	lf, err := openLog(n.dir)
	if err != nil {
		return err
	}
	n.logFile, n.prevLog = lf, log.Writer()
	log.SetOutput(lf)
	return nil
}

func (n *nodeRuntime) stepIdentity() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	key, err := identity.LoadKey(n.dir)
	if err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(n.dir, "node.db"))
	if err != nil {
		return err
	}
	n.exe, n.key, n.db = exe, key, db
	return nil
}

func (n *nodeRuntime) stepServices() error {
	logs, err := OpenLogs(n.db, n.key.ID())
	if err != nil {
		return err
	}
	n.logs = logs
	loop, err := home.StartLoop(home.LoopConfig{
		Store: n.db, Home: n.key.ID(), OwnerFn: logs.Owner,
		Logs: logs.Log, Tables: home.NodeTables(), Every: time.Second,
	})
	if err != nil {
		return err
	}
	h, saved, err := host.New(n.db, n.key, time.Now)
	if err != nil {
		loop.Close()
		return err
	}
	n.loop, n.host, n.saved = loop, h, saved
	n.wireServices()
	return nil
}

func (n *nodeRuntime) wireServices() {
	n.host.Shim = []string{n.exe, "mcp"}
	n.local = AttachLocal(n.host, n.loop)
	n.sessions = NewSessions(SessionConfig{
		ID: n.key.ID(), Version: Version, Store: n.db, Logs: n.logs,
		Home: n.loop, Host: n.host, Name: n.readName, Addrs: n.addrs, OnAdopt: n.onAdopt,
	})
}

func (n *nodeRuntime) stepNetwork() error {
	socks, err := listenNodePeer(defaultWebPort, defaultTCPPort)
	if err != nil {
		return err
	}
	n.socks = socks
	n.peer = peer.New(n.key, socks, n.db, n.knownPeer, n.routesFor, n.serveInbound, nil)
	ln, err := listenNodeHTTP(":" + strconv.Itoa(defaultWebPort))
	if err != nil {
		return err
	}
	return n.startLocalHTTP(ln)
}

func (n *nodeRuntime) startLocalHTTP(ln net.Listener) error {
	web, _ := fs.Sub(assets, "webdist")
	n.httpPort = ln.Addr().(*net.TCPAddr).Port
	n.httpSrv = &http.Server{Handler: rendezvous.Handler(web)}
	go n.httpSrv.Serve(ln)
	path, err := peer.Dial(n.ctx, "ws://127.0.0.1:"+strconv.Itoa(n.httpPort)+"/rv", n.key)
	if err != nil {
		return err
	}
	n.localPath = path
	n.bg.Add(1)
	go n.serveLocalRV(path)
	return nil
}

func noopRVEvent(string, []byte) error { return nil }

func (n *nodeRuntime) serveLocalRV(path *peer.Path) {
	defer n.bg.Done()
	path.Serve(n.ctx, n.peer, noopRVEvent)
}

func (n *nodeRuntime) stepStartup() error {
	if err := n.host.EnsureHomeConnections(); err != nil {
		return err
	}
	n.host.ReportSaved(n.ctx, n.saved)
	n.syncRendezvous()
	return nil
}

func (n *nodeRuntime) Close() {
	n.closeOnce.Do(func() {
		n.stop()
		n.closeNet()
		n.bg.Wait()
		n.closeCore()
	})
}

func (n *nodeRuntime) closeNet() {
	if n.localPath != nil {
		n.localPath.Close()
	}
	if n.httpSrv != nil {
		n.httpSrv.Close()
	}
	if n.socks != nil {
		n.socks.Close()
	}
}

func (n *nodeRuntime) closeCore() {
	if n.local != nil {
		n.local.Close()
	}
	if n.loop != nil {
		n.loop.Close()
	}
	if n.db != nil {
		n.db.Close()
	}
	if n.logFile != nil {
		log.SetOutput(n.prevLog)
		n.logFile.Close()
	}
}

func listenNodePeer(udpPort, tcpPort int) (*peer.Sockets, error) {
	if socks, err := peerListen(udpPort, tcpPort); err == nil {
		return socks, nil
	}
	return peerListen(0, 0)
}

func listenNodeHTTP(addr string) (net.Listener, error) {
	if ln, err := tcpListen("tcp", addr); err == nil {
		return ln, nil
	}
	return tcpListen("tcp", ":0")
}

func (n *nodeRuntime) knownPeer(device string) bool { return n.logs.Known(device) }

func (n *nodeRuntime) serveInbound(c *peer.Connection) {
	n.bg.Add(1)
	go func() {
		defer n.bg.Done()
		n.sessions.Serve(n.ctx, c.Device, false, c)
	}()
}

func (n *nodeRuntime) routesFor(_ string) []peer.Route {
	routes := []peer.Route{n.localPath.Route()}
	for _, p := range n.remoteRVPaths() {
		routes = append(routes, p.Route())
	}
	return routes
}

func (n *nodeRuntime) remoteRVPaths() []*peer.Path {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]*peer.Path, 0, len(n.rvPaths))
	for _, k := range slices.Sorted(maps.Keys(n.rvPaths)) {
		out = append(out, n.rvPaths[k])
	}
	return out
}

func (n *nodeRuntime) readName() string {
	var name string
	n.readSetting("name", &name)
	return name
}

func (n *nodeRuntime) readRendezvous() []string {
	urls := []string{}
	n.readSetting("rendezvous", &urls)
	return urls
}

func (n *nodeRuntime) readSetting(key string, out any) {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT value FROM setting WHERE key = ?", key).Scan(&raw)
	}
	if n.db.Write(read) == nil {
		json.Unmarshal([]byte(raw), out)
	}
}

func (n *nodeRuntime) saveRendezvous(urls []string) error {
	raw, _ := json.Marshal(urls)
	put := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('rendezvous', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", string(raw))
		return err
	}
	return n.db.Write(put)
}

func (n *nodeRuntime) addrs() []string { return nodeAddrs(n.httpPort, n.readRendezvous()) }

func nodeAddrs(port int, rvURLs []string) []string {
	out := []string{}
	portStr := strconv.Itoa(port)
	for _, ip := range localIPs() {
		out = append(out, "ws://"+net.JoinHostPort(ip, portStr)+"/rv")
	}
	return append(out, rvURLs...)
}

func localIPs() []string {
	addrs, _ := ifaceAddrs()
	var ips []string
	for _, a := range addrs {
		if ip := usableNodeIP(a); ip != "" {
			ips = append(ips, ip)
		}
	}
	slices.Sort(ips)
	return slices.Compact(ips)
}

func usableNodeIP(a net.Addr) string {
	ipnet, ok := a.(*net.IPNet)
	if !ok {
		return ""
	}
	addr, _ := netip.AddrFromSlice(ipnet.IP)
	if addr = addr.Unmap(); !isUsableAddr(addr) {
		return ""
	}
	return addr.String()
}

func isUsableAddr(addr netip.Addr) bool {
	return addr.IsValid() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsUnspecified()
}

func (n *nodeRuntime) onAdopt(_ string) {
	if err := installService(n.exe, os.Getenv("PATH")); err != nil {
		log.Printf("node: install service: %v", err)
	}
}

func (n *nodeRuntime) syncRendezvous() {
	if n.ctx.Err() != nil {
		return
	}
	want := map[string]bool{}
	for _, u := range n.readRendezvous() {
		want[u] = true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pruneRendezvousLocked(want)
	for u := range want {
		if n.rvStop[u] == nil {
			rvCtx, cancel := context.WithCancel(n.ctx)
			n.rvStop[u] = cancel
			n.bg.Add(1)
			go n.runRVLoop(rvCtx, u)
		}
	}
}

func (n *nodeRuntime) pruneRendezvousLocked(want map[string]bool) {
	for u, stop := range n.rvStop {
		if !want[u] {
			stop()
			delete(n.rvStop, u)
			delete(n.rvPaths, u)
		}
	}
}

func (n *nodeRuntime) runRVLoop(ctx context.Context, url string) {
	defer n.bg.Done()
	delay := rvRetryDelay
	for ctx.Err() == nil {
		delay = rvNextDelay(ctx, delay, n.serveOneRV(ctx, url))
	}
}

func rvNextDelay(ctx context.Context, delay, ran time.Duration) time.Duration {
	if ran >= time.Minute {
		delay = rvRetryDelay
	}
	select {
	case <-ctx.Done():
		return 0
	case <-time.After(delay):
		return min(2*delay, time.Minute)
	}
}

func (n *nodeRuntime) serveOneRV(ctx context.Context, url string) time.Duration {
	path, err := peer.Dial(ctx, url, n.key)
	if err != nil {
		return 0
	}
	opened := time.Now()
	n.mu.Lock()
	n.rvPaths[url] = path
	n.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { path.Close() })
	path.Serve(ctx, n.peer, noopRVEvent)
	stop()
	path.Close()
	n.mu.Lock()
	if n.rvPaths[url] == path {
		delete(n.rvPaths, url)
	}
	n.mu.Unlock()
	return time.Since(opened)
}

func (n *nodeRuntime) serve(l *Local) {
	stopAccept := context.AfterFunc(n.ctx, func() { l.SetDeadline(time.Now()) })
	defer stopAccept()
	var wg sync.WaitGroup
	for {
		conn, err := l.Accept()
		if err != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.handleLocal(conn)
		}()
	}
	wg.Wait()
}

func (n *nodeRuntime) handleLocal(conn net.Conn) {
	defer conn.Close()
	open, line, r, err := AcceptOpening(conn)
	if err != nil {
		return
	}
	if open.MCP != "" {
		n.host.ServeMCP(n.ctx, open.MCP, r, conn)
		return
	}
	lines, cmdErr := n.runLocalCmd(open.Cmd, line)
	writeLocalReplies(conn, lines, cmdErr)
}

func (n *nodeRuntime) runLocalCmd(cmd string, line []byte) ([]string, error) {
	switch cmd {
	case "status":
		return n.cmdStatus(), nil
	case "pair":
		return n.cmdPair(line)
	case "pair-browser":
		return n.cmdPairBrowser()
	case "open":
		return n.cmdOpen()
	default:
		return nil, fmt.Errorf("unknown command: %s", cmd)
	}
}

func (n *nodeRuntime) cmdStatus() []string {
	lines := []string{"version " + Version, "fingerprint " + n.key.Fingerprint()}
	lines = append(lines, n.sessions.statusLines()...)
	for _, s := range n.host.Sessions.All() {
		if s.State != host.Dormant {
			lines = append(lines, fmt.Sprintf("session %s %s %s", s.Group, s.Member, s.State))
		}
	}
	return lines
}

func (n *nodeRuntime) cmdPair(line []byte) ([]string, error) {
	req, err := ReadPair(line, n.logs.Owner(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := identity.ParseSecret(req.Secret); err != nil {
		return nil, err
	}
	if len(req.Rendezvous) > 0 {
		if err := n.saveRendezvous(req.Rendezvous); err != nil {
			return nil, err
		}
		n.syncRendezvous()
	}
	return nil, errors.New("pairing room is unavailable")
}

func (n *nodeRuntime) cmdPairBrowser() ([]string, error) {
	if n.logs.Owner() == "" {
		return nil, errors.New("node has no owner")
	}
	return nil, errors.New("pairing room is unavailable")
}

func (n *nodeRuntime) cmdOpen() ([]string, error) {
	if n.logs.Owner() == "" {
		return nil, errors.New("node has no owner")
	}
	return nil, errors.New("pairing room is unavailable")
}
