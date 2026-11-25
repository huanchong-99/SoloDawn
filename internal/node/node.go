// Package node assembles the SoloDawn runtime.
package node

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
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
	peerListen              = peer.Listen
	tcpListen               = net.Listen
	ifaceAddrs              = net.InterfaceAddrs
	installService          = InstallService
	openURL                 = openBrowserURL
	foregroundContext       = func() (context.Context, context.CancelFunc) {
		return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
)

func openBrowserURL(u string) error {
	exe, args := browserCommand(runtime.GOOS, u)
	return startDetached(exe, args...)
}

func browserCommand(goos, u string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{u}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", u}
	default:
		return "xdg-open", []string{u}
	}
}

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

type rvLoop struct {
	stop      context.CancelFunc
	readyOnce sync.Once
	ready     chan struct{} // closed once by finishFirst; read by waitRVPath
	err       error
}

func (r *rvLoop) finishFirst(err error) {
	r.readyOnce.Do(func() {
		r.err = err
		close(r.ready)
	})
}

// nodeRuntime holds the running components of one SoloDawn node.
// mu is a leaf lock guarding closing, rvPaths and rvLoops.
type nodeRuntime struct {
	ctx       context.Context
	stop      context.CancelFunc
	dir, exe  string
	logFile   *logFile
	resetLog  func()
	level     slog.Level
	diag      *slog.Logger
	key       identity.Key
	db        *store.Store
	logs      *Logs
	loop      *home.Loop
	local     *LocalLink
	host      *host.Host
	saved     []host.Session
	sessions  *Sessions
	pairer    *Pairer
	socks     *peer.Sockets
	peer      *peer.Peer
	httpSrv   *http.Server
	httpPort  int
	localPath *peer.Path
	bg        sync.WaitGroup
	closeOnce sync.Once

	mu      sync.Mutex
	closing bool
	rvPaths map[string]*peer.Path
	rvLoops map[string]*rvLoop
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
		rvPaths: map[string]*peer.Path{}, rvLoops: map[string]*rvLoop{},
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
	n.logFile, n.level = lf, diagLevel(n.dir)
	n.resetLog = diagOutput(lf, n.level)
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
	n.diag = slog.With("node", diagDevice(key.ID()))
	n.diagNode("node.start")
	return nil
}

func (n *nodeRuntime) stepServices() error {
	logs, err := OpenLogs(n.db, n.key.ID())
	if err != nil {
		return err
	}
	n.logs = logs
	n.pairer = newPairer(PairConfig{
		Key: n.key, Store: n.db, Logs: n.logs, Paths: n.sharedPaths, WebURL: n.defaultWebURL,
	})
	loop, err := home.StartLoop(home.LoopConfig{
		Store: n.db, Home: n.key.ID(), OwnerFn: logs.Owner,
		Logs: logs.Log, Forget: logs.dvlForget, Tables: home.NodeTables(), Invite: n.pairer, Every: time.Second,
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
	n.host.WatchEnded = n.watchEnded
	n.local = AttachLocal(n.host, n.loop)
	n.local.watchGroups()
	nameFn := func() string { return ReadName(n.db) }
	n.sessions = NewSessions(SessionConfig{
		ID: n.key.ID(), Version: Version, Store: n.db, Logs: n.logs,
		Home: n.loop, Host: n.host, Name: nameFn, Addrs: n.addrs, OnAdopt: n.onAdopt,
		Spawn: func(fn func()) { n.spawn("host_cmd", fn) }, Known: n.knownPeer,
	})
	surface := NewSurface(SurfaceConfig{
		ID: n.key.ID(), Version: Version, Exe: n.exe, Store: n.db,
		Sessions: n.sessions, Host: n.host, Name: nameFn, Agents: n.host.Agents,
	})
	n.host.WatchersChanged = func() {
		n.spawn("host_info", func() { surface.pushHostInfo(n.ctx, false) })
	}
	n.pairer.bind(n.sessions)
	host.Part[netStatusPart](n.host).status = n.netStatus
}

func (n *nodeRuntime) stepNetwork() error {
	socks, err := listenNodePeer(defaultWebPort, defaultTCPPort, n.key.ID())
	if err != nil {
		return err
	}
	n.socks = socks
	n.peer = peer.New(n.key, socks, n.db, n.knownPeer, n.routesFor, n.serveInbound, peer.Channels{
		"pty:": func(s *peer.Stream) {
			if !n.logs.IsOwner(s.Device()) {
				s.Close()
				return
			}
			n.host.ServeTerminal(s)
		},
		"watch:": func(s *peer.Stream) { n.host.ServeWatch(s) },
	})
	n.pairer.Peer = n.peer
	n.host.Connect = n.sessions.connectHome(n.dialHome)
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
	go n.httpSrv.Serve(peer.Steady(n.ctx, ln, n.diag, "http"))
	path, err := peer.Dial(n.ctx, "ws://127.0.0.1:"+strconv.Itoa(n.httpPort)+"/rv", n.key)
	if err != nil {
		return err
	}
	n.localPath = path
	n.spawn("local_rv", func() { path.Serve(n.ctx, n.peer, n.rvEvents(n.ctx, path)) })
	return nil
}

func (n *nodeRuntime) rvEvents(ctx context.Context, path *peer.Path) func(string, []byte) error {
	return func(kind string, data []byte) error {
		n.pairer.HandleEvent(ctx, path, kind, data)
		return nil
	}
}

func (n *nodeRuntime) stepStartup() error {
	if _, err := n.host.Agents(n.ctx, false); err != nil {
		return err
	}
	if err := n.host.EnsureHomeConnections(); err != nil {
		return err
	}
	n.host.ReportSaved(n.ctx, n.saved)
	n.syncRendezvous()
	n.startRetention()
	return nil
}

func (n *nodeRuntime) enterBg() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closing {
		return false
	}
	n.bg.Add(1)
	return true
}

func (n *nodeRuntime) withBg(fn func()) {
	if n.enterBg() {
		defer n.bg.Done()
		fn()
	}
}

func (n *nodeRuntime) spawn(name string, fn func()) {
	if n.enterBg() {
		go func() {
			defer n.bg.Done()
			defer n.diagBG(name)()
			fn()
		}()
	}
}

func (n *nodeRuntime) Close() {
	n.closeOnce.Do(func() {
		if n.diag != nil {
			n.diagNode("node.stop")
		}
		n.mu.Lock()
		n.closing = true
		n.mu.Unlock()
		n.stop()
		n.closeNet()
		n.bg.Wait()
		n.closeCore()
	})
}

func (n *nodeRuntime) closeNet() {
	if n.pairer != nil {
		n.pairer.Close()
	}
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
	if n.host != nil {
		n.host.Settle()
	}
	if n.db != nil {
		n.db.Close()
	}
	if n.logFile != nil {
		n.resetLog()
		n.logFile.Close()
	}
}

func listenNodePeer(udpPort, tcpPort int, id string) (*peer.Sockets, error) {
	if socks, err := peerListen(udpPort, tcpPort, id); err == nil {
		return socks, nil
	}
	return peerListen(0, 0, id)
}

func listenNodeHTTP(addr string) (net.Listener, error) {
	if ln, err := tcpListen("tcp", addr); err == nil {
		return ln, nil
	}
	return tcpListen("tcp", ":0")
}

func (n *nodeRuntime) localWebURL() string {
	return "http://localhost:" + strconv.Itoa(n.httpPort)
}

func (n *nodeRuntime) defaultWebURL() string {
	if len(ReadRendezvous(n.db)) > 0 {
		return ""
	}
	return n.localWebURL()
}

func (n *nodeRuntime) knownPeer(device string) bool {
	return n.logs.Known(device) || n.pairer.Known(device) || n.remembersHome(device) || n.host.Watching(device)
}

func (n *nodeRuntime) watchEnded(group, user string) {
	for _, raw := range n.host.Rows.Of(group, "device") {
		var row proto.DeviceRow
		proto.Decode(raw, &row)
		if row.User == user {
			n.closeUnknownDevice(row.Device)
		}
	}
}

func (n *nodeRuntime) closeUnknownDevice(device string) {
	if n.knownPeer(device) {
		return
	}
	for _, c := range n.sessions.sesConns() {
		if c.device == device {
			c.Close()
		}
	}
}

func (n *nodeRuntime) serveInbound(c *peer.Connection) {
	n.spawn("inbound", func() { n.pairer.Serve(n.ctx, c.Device, c) })
}

func (n *nodeRuntime) dialHome(ctx context.Context, device string, run func(clientCtl)) {
	n.spawn("home_dial", func() {
		n.diag.Info("home.dial", "home", diagDevice(device))
		runCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(n.ctx, cancel)
		defer stop()
		defer cancel()
		n.peer.Connect(runCtx, device, nil, func(c *peer.Connection) {
			n.spawn("home_session", func() { run(c) })
		})
	})
}

func (n *nodeRuntime) sharedPaths() []*peer.Path {
	return append([]*peer.Path{n.localPath}, n.remoteRVPaths()...)
}

func (n *nodeRuntime) routesFor(device string) []peer.Route {
	routes := n.pairer.Routes(device)
	for _, p := range n.sharedPaths() {
		route := p.Route()
		i := slices.IndexFunc(routes, func(r peer.Route) bool { return r.ID == route.ID })
		if i < 0 {
			routes = append(routes, route)
		} else {
			routes[i] = route
		}
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

func (n *nodeRuntime) saveRendezvous(urls []string) error {
	raw, _ := json.Marshal(urls)
	put := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('rendezvous', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", string(raw))
		return err
	}
	return n.db.Write(put)
}

func (n *nodeRuntime) addrs() []string { return nodeAddrs(n.httpPort, ReadRendezvous(n.db)) }

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
	n.withBg(func() {
		want := map[string]bool{}
		for _, u := range ReadRendezvous(n.db) {
			want[u] = true
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		n.pruneRendezvousLocked(want)
		for u := range want {
			if n.rvLoops[u] == nil && !n.closing {
				n.startRVLoopLocked(u)
			}
		}
	})
}

func (n *nodeRuntime) startRVLoopLocked(url string) {
	rvCtx, cancel := context.WithCancel(n.ctx)
	loop := &rvLoop{stop: cancel, ready: make(chan struct{})}
	n.rvLoops[url] = loop
	n.bg.Add(1)
	go n.runRVLoop(rvCtx, url, loop)
}

func (n *nodeRuntime) pruneRendezvousLocked(want map[string]bool) {
	for u, loop := range n.rvLoops {
		if !want[u] {
			loop.stop()
			loop.finishFirst(net.ErrClosed)
			delete(n.rvLoops, u)
			delete(n.rvPaths, u)
		}
	}
}

func (n *nodeRuntime) waitRVPath(url string) error {
	n.mu.Lock()
	loop := n.rvLoops[url]
	n.mu.Unlock()
	if loop == nil {
		return net.ErrClosed
	}
	<-loop.ready
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.rvPaths[url] != nil {
		return nil
	}
	return loop.err
}

func (n *nodeRuntime) runRVLoop(ctx context.Context, url string, loop *rvLoop) {
	defer n.bg.Done()
	defer n.diagBG("rendezvous")()
	delay := time.Second
	for ctx.Err() == nil {
		delay = peer.NextDelay(ctx, nil, delay, n.serveOneRV(ctx, url, loop))
	}
	loop.finishFirst(ctx.Err())
}

func (n *nodeRuntime) serveOneRV(ctx context.Context, url string, loop *rvLoop) time.Duration {
	path, err := peer.Dial(ctx, url, n.key)
	if err != nil {
		loop.finishFirst(err)
		return 0
	}
	opened := time.Now()
	n.mu.Lock()
	n.rvPaths[url] = path
	n.mu.Unlock()
	loop.finishFirst(nil)
	stop := context.AfterFunc(ctx, func() { path.Close() })
	path.Serve(ctx, n.peer, n.rvEvents(ctx, path))
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
	accept := peer.Steady(n.ctx, l, n.diag, "local").Accept
	var wg sync.WaitGroup
	for {
		conn, err := accept()
		if err != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer n.diagBG("local")()
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
	ctx, closed := context.WithCancel(n.ctx)
	read := make(chan struct{})
	go func() {
		r.ReadByte()
		closed()
		close(read)
	}()
	lines, cmdErr := n.runLocalCmd(ctx, open.Cmd, line)
	writeLocalReplies(conn, lines, cmdErr)
	conn.Close()
	<-read
}

type localCmd func(n *nodeRuntime, ctx context.Context, line []byte) ([]string, error)

var localCmds = map[string]localCmd{
	"wait":         (*nodeRuntime).cmdWait,
	"done":         func(n *nodeRuntime, _ context.Context, line []byte) ([]string, error) { return nil, n.cmdDone(line) },
	"status":       func(n *nodeRuntime, _ context.Context, _ []byte) ([]string, error) { return n.cmdStatus(), nil },
	"pair":         func(n *nodeRuntime, _ context.Context, line []byte) ([]string, error) { return n.cmdPair(line) },
	"pair-browser": func(n *nodeRuntime, _ context.Context, _ []byte) ([]string, error) { return n.cmdPairBrowser() },
	"open":         func(n *nodeRuntime, _ context.Context, _ []byte) ([]string, error) { return n.cmdOpen() },
	"group.export": groupLocalCmd,
	"group.import": groupLocalCmd,
}

func groupLocalCmd(n *nodeRuntime, _ context.Context, line []byte) ([]string, error) {
	return nil, n.cmdGroup(line)
}

func (n *nodeRuntime) runLocalCmd(ctx context.Context, cmd string, line []byte) ([]string, error) {
	run := localCmds[cmd]
	if run == nil {
		return nil, fmt.Errorf("unknown command: %s", cmd)
	}
	return run(n, ctx, line)
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
		if err := n.preparePairRV(req.Rendezvous); err != nil {
			return nil, err
		}
		req.Rendezvous = nil
	}
	res, err := n.pairer.Pair(n.ctx, req)
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("%s (%s) %s", res.Name, res.Fingerprint, cmp.Or(res.Path, "direct"))}, nil
}

func (n *nodeRuntime) preparePairRV(urls []string) error {
	if err := n.saveRendezvous(urls); err != nil {
		return err
	}
	n.syncRendezvous()
	return n.waitRVPath(urls[0])
}

func (n *nodeRuntime) cmdPairBrowser() ([]string, error) {
	link, err := n.pairer.PairBrowser(n.ctx, n.defaultWebURL())
	if err != nil {
		return nil, err
	}
	return []string{link.Link}, nil
}

func (n *nodeRuntime) cmdOpen() ([]string, error) {
	link, err := n.pairer.PairBrowser(n.ctx, n.localWebURL())
	if err != nil {
		return nil, err
	}
	if err := openURL(link.Link); err != nil {
		return nil, err
	}
	return []string{link.Link}, nil
}
