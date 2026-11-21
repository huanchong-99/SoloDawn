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
	updateEvery             = time.Hour
	rvRetryDelay            = time.Second
	releaseURL              = "https://github.com/huanchong-99/SoloDawn/releases/latest/download"
	updateTransport   http.RoundTripper
	peerListen        = peer.Listen
	tcpListen         = net.Listen
	ifaceAddrs        = net.InterfaceAddrs
	execReplace       = replaceProcess
	startBrowser      = startDetached
	openURL           = openBrowserURL
	foregroundContext = defaultForegroundContext
)

func defaultForegroundContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func init() {
	host.Register(host.Holds, "update", holdForUpdate)
	host.Register(host.Listeners, "node_update", listenNodeUpdate)
}

type updateGate struct {
	mu   sync.Mutex
	hold bool
	wake func()
}

func holdForUpdate(h *host.Host, _ host.SessionKey) (string, bool) {
	g := host.Part[updateGate](h)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.hold {
		return "updating", true
	}
	return "", false
}

func listenNodeUpdate(h *host.Host, _ host.SessionKey, e host.Event) {
	if e.Kind != host.TurnEnded {
		return
	}
	g := host.Part[updateGate](h)
	g.mu.Lock()
	wake := g.wake
	g.mu.Unlock()
	if wake != nil {
		wake()
	}
}

func browserCommand(goos, link string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{link}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", link}
	default:
		return "xdg-open", []string{link}
	}
}

func openBrowserURL(link string) error {
	prog, args := browserCommand(runtime.GOOS, link)
	return startBrowser(prog, args...)
}

type logFile struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func openLog(dir string) (*logFile, error) {
	logsDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, err
	}
	lf := &logFile{path: filepath.Join(logsDir, "node.log")}
	if err := lf.openCurrent(); err != nil {
		return nil, err
	}
	return lf, nil
}

func (l *logFile) openCurrent() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, _ := f.Stat()
	l.file, l.size = f, info.Size()
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
	err := os.Rename(l.path, l.path+".1")
	return errors.Join(err, l.openCurrent())
}

func (l *logFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// nodeRuntime holds the running components of one SoloDawn node.
// Lock order: homeMu -> Home/Sessions; updMu -> updateGate.mu / mu; mu is a
// leaf lock guarding rvPaths, rvStop, presence, relay, updStatus and restartBin.
type nodeRuntime struct {
	ctx        context.Context
	stop       context.CancelFunc
	dir, exe   string
	logFile    *logFile
	prevLog    io.Writer
	key        identity.Key
	db         *store.Store
	logs       *Logs
	homeMu     sync.Mutex
	loop       *home.Loop
	local      *LocalLink
	host       *host.Host
	saved      []host.Session
	sessions   *Sessions
	socks      *peer.Sockets
	peer       *peer.Peer
	httpSrv    *http.Server
	httpPort   int
	localPath  *peer.Path
	updMu      sync.Mutex
	updater    Updater
	wakeUpdate chan struct{} // single reader: runUpdateLoop
	bg         sync.WaitGroup
	closeOnce  sync.Once

	mu         sync.Mutex
	rvPaths    map[string]*peer.Path
	rvStop     map[string]context.CancelFunc
	presence   map[string][]chan struct{}
	relay      *proto.RelayUse
	updStatus  *proto.Update
	restartBin string
}

func runForegroundNode(dir string, l *Local) error {
	ctx, stop := foregroundContext()
	defer stop()
	restartBin, err := runNode(ctx, dir, l)
	if err != nil || restartBin == "" {
		return err
	}
	l.Close()
	return execReplace(restartBin, os.Args, os.Environ())
}

func runNode(ctx context.Context, dir string, l *Local) (string, error) {
	n, err := openNode(ctx, dir)
	if err != nil {
		return "", err
	}
	defer n.Close()
	return n.serve(l), nil
}

func openNode(parent context.Context, dir string) (*nodeRuntime, error) {
	ctx, cancel := context.WithCancel(parent)
	n := &nodeRuntime{
		ctx: ctx, stop: cancel, dir: dir,
		rvPaths: map[string]*peer.Path{}, rvStop: map[string]context.CancelFunc{},
		presence: map[string][]chan struct{}{}, wakeUpdate: make(chan struct{}, 1),
	}
	steps := []func() error{n.stepLog, n.stepIdentity, n.stepServices, n.stepNetwork, n.stepStartup}
	for _, step := range steps {
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
	loop, err := n.startHomeLoop(logs.Owner(), logs.Log)
	if err != nil {
		return err
	}
	h, saved, err := host.New(n.db, n.key, time.Now)
	if err != nil {
		loop.Close()
		return err
	}
	n.logs, n.loop, n.host, n.saved = logs, loop, h, saved
	n.wireServices()
	return nil
}

func (n *nodeRuntime) startHomeLoop(owner string, logFn func(string) *identity.Log) (*home.Loop, error) {
	return home.StartLoop(home.LoopConfig{
		Store: n.db, Home: n.key.ID(), Owner: owner,
		Logs: logFn, Tables: home.NodeTables(), Every: time.Second,
	})
}

func (n *nodeRuntime) wireServices() {
	n.host.Shim = []string{n.exe, "mcp"}
	gate := host.Part[updateGate](n.host)
	gate.wake = n.signalUpdate
	n.local = AttachLocal(n.host, n.loop)
	n.sessions = NewSessions(SessionConfig{
		ID: n.key.ID(), Version: Version, Store: n.db, Logs: n.logs,
		Home: n.loop, Host: n.host, Name: n.readName, Addrs: n.addrs,
		OnAdopt: n.onAdopt,
	})
	n.updater = Updater{
		ReleaseURL: releaseURL, Binary: n.exe, Version: Version,
		Busy: n.isBusy, Restart: n.triggerRestart, Transport: updateTransport,
	}
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
	n.httpSrv = &http.Server{Handler: nodeHTTPHandler(rendezvous.Handler(web))}
	go n.httpSrv.Serve(ln)
	path, err := peer.Dial(n.ctx, n.localRVURL(), n.key)
	if err != nil {
		return err
	}
	n.localPath = path
	n.bg.Add(1)
	go n.serveLocalRV(path)
	return nil
}

func (n *nodeRuntime) serveLocalRV(path *peer.Path) {
	defer n.bg.Done()
	path.Serve(n.ctx, n.peer, func(kind string, data []byte) error {
		n.onRVEvent(n.ctx, path, kind, data)
		return nil
	})
}

func (n *nodeRuntime) stepStartup() error {
	if err := n.host.EnsureHomeConnections(); err != nil {
		return err
	}
	n.host.ReportSaved(n.ctx, n.saved)
	n.syncRendezvous()
	n.bg.Add(1)
	go n.runUpdateLoop()
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
	n.stopAllRendezvous()
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
	n.homeMu.Lock()
	local, loop := n.local, n.loop
	n.local, n.loop = nil, nil
	n.homeMu.Unlock()
	if local != nil {
		local.Close()
	}
	if loop != nil {
		loop.Close()
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
	socks, err := peerListen(udpPort, tcpPort)
	if err != nil {
		return peerListen(0, 0)
	}
	return socks, nil
}

func listenNodeHTTP(addr string) (net.Listener, error) {
	ln, err := tcpListen("tcp", addr)
	if err != nil {
		return tcpListen("tcp", ":0")
	}
	return ln, nil
}

func nodeHTTPHandler(rv http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rv" && !isLocalHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		rv.ServeHTTP(w, r)
	})
}

func isLocalHost(hostport string) bool {
	h := hostport
	if parsed, _, err := net.SplitHostPort(hostport); err == nil {
		h = parsed
	}
	return h == "localhost" || h == "127.0.0.1"
}

func (n *nodeRuntime) localWebURL() string {
	return "http://localhost:" + strconv.Itoa(n.httpPort)
}

func (n *nodeRuntime) localRVURL() string {
	return "ws://127.0.0.1:" + strconv.Itoa(n.httpPort) + "/rv"
}

func (n *nodeRuntime) knownPeer(device string) bool {
	return n.logs.Known(device)
}

func (n *nodeRuntime) serveInbound(c *peer.Connection) {
	go n.sessions.Serve(n.ctx, c.Device, false, c)
}

func (n *nodeRuntime) routesFor(_ string) []peer.Route {
	routes := []peer.Route{n.localPath.Route()}
	for _, p := range n.remoteRVPaths() {
		routes = append(routes, p.Route())
	}
	return routes
}

func (n *nodeRuntime) sharedPaths() []*peer.Path {
	return append([]*peer.Path{n.localPath}, n.remoteRVPaths()...)
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

func (n *nodeRuntime) addrs() []string {
	return nodeAddrs(n.httpPort, n.readRendezvous())
}

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
	addr = addr.Unmap()
	if !isUsableAddr(addr) {
		return ""
	}
	return addr.String()
}

func isUsableAddr(addr netip.Addr) bool {
	return addr.IsValid() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsUnspecified()
}

func (n *nodeRuntime) onAdopt(owner string) {
	n.homeMu.Lock()
	oldLocal, oldLoop := n.local, n.loop
	oldLocal.Close()
	newLoop, err := n.startHomeLoop(owner, n.logs.Log)
	if err != nil {
		n.homeMu.Unlock()
		log.Printf("node: restart home on adopt: %v", err)
		return
	}
	n.sessions.SetHome(newLoop)
	oldLoop.Close()
	n.loop = newLoop
	n.local = AttachLocal(n.host, newLoop)
	n.homeMu.Unlock()
	n.watchOwnerDevices()
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

func (n *nodeRuntime) stopAllRendezvous() {
	n.mu.Lock()
	stops := slices.Collect(maps.Values(n.rvStop))
	n.rvStop = map[string]context.CancelFunc{}
	n.rvPaths = map[string]*peer.Path{}
	n.mu.Unlock()
	for _, stop := range stops {
		stop()
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
	n.setRVPath(url, path)
	n.initRVPath(ctx, path)
	stop := context.AfterFunc(ctx, func() { path.Close() })
	path.Serve(ctx, n.peer, func(kind string, data []byte) error {
		n.onRVEvent(ctx, path, kind, data)
		return nil
	})
	stop()
	path.Close()
	n.clearRVPath(url, path)
	return time.Since(opened)
}

func (n *nodeRuntime) setRVPath(url string, path *peer.Path) {
	n.mu.Lock()
	n.rvPaths[url] = path
	n.mu.Unlock()
}

func (n *nodeRuntime) clearRVPath(url string, path *peer.Path) {
	n.mu.Lock()
	if n.rvPaths[url] == path {
		delete(n.rvPaths, url)
	}
	n.mu.Unlock()
}

func (n *nodeRuntime) initRVPath(ctx context.Context, path *peer.Path) {
	if n.logs.Owner() != "" {
		n.sendWatch(ctx, path, n.logs.OwnerDevices())
	}
	path.Write(ctx, proto.RVFrame{Type: "quota"})
}

func (n *nodeRuntime) watchOwnerDevices() {
	if n.logs.Owner() == "" {
		return
	}
	devs := n.logs.OwnerDevices()
	for _, path := range n.remoteRVPaths() {
		n.sendWatch(n.ctx, path, devs)
	}
}

func (n *nodeRuntime) sendWatch(ctx context.Context, path *peer.Path, devs []string) {
	keys := slices.Clone(devs)
	if keys == nil {
		keys = []string{}
	}
	if len(keys) > 64 {
		keys = keys[:64]
	}
	path.Write(ctx, proto.Watch{Type: "watch", Keys: keys})
}

func (n *nodeRuntime) onRVEvent(_ context.Context, _ *peer.Path, kind string, data []byte) {
	switch kind {
	case "presence":
		var p proto.Presence
		if proto.Decode(data, &p) == nil && p.On {
			n.notifyPresence(p.Key)
		}
	case "quota":
		var q proto.Quota
		if proto.Decode(data, &q) == nil {
			n.mu.Lock()
			n.relay = &proto.RelayUse{Used: q.Used, Limit: q.Limit}
			n.mu.Unlock()
		}
	}
}

func (n *nodeRuntime) subscribePresence(ctx context.Context, device string) <-chan struct{} {
	// Single reader: peer.Connect's backoff for device.
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	n.presence[device] = append(n.presence[device], ch)
	n.mu.Unlock()
	context.AfterFunc(ctx, func() { n.unsubscribePresence(device, ch) })
	return ch
}

func (n *nodeRuntime) unsubscribePresence(device string, target chan struct{}) {
	match := func(ch chan struct{}) bool { return ch == target }
	n.mu.Lock()
	list := slices.DeleteFunc(n.presence[device], match)
	if len(list) == 0 {
		delete(n.presence, device)
	} else {
		n.presence[device] = list
	}
	n.mu.Unlock()
}

func (n *nodeRuntime) notifyPresence(device string) {
	n.mu.Lock()
	subs := slices.Clone(n.presence[device])
	n.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (n *nodeRuntime) relayStatus() *proto.RelayUse {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.relay == nil {
		return nil
	}
	cp := *n.relay
	return &cp
}

func (n *nodeRuntime) isBusy() bool {
	for _, s := range n.host.Sessions.All() {
		if s.State == host.Working || s.State == host.Takeover {
			return true
		}
	}
	return false
}

func (n *nodeRuntime) triggerRestart() error {
	n.mu.Lock()
	n.restartBin = n.updater.Binary
	n.mu.Unlock()
	n.stop()
	return nil
}

func (n *nodeRuntime) notifyWake() {
	select {
	case n.wakeUpdate <- struct{}{}:
	default:
	}
}

func (n *nodeRuntime) signalUpdate() {
	n.notifyWake()
	time.AfterFunc(50*time.Millisecond, n.notifyWake)
}

func (n *nodeRuntime) updateStatus() *proto.Update {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.updStatus == nil {
		return nil
	}
	cp := *n.updStatus
	return &cp
}

func (n *nodeRuntime) runUpdateLoop() {
	defer n.bg.Done()
	n.checkAndApplyUpdate(time.Now())
	ticker := time.NewTicker(updateEvery)
	defer ticker.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case now := <-ticker.C:
			n.checkAndApplyUpdate(now)
		case <-n.wakeUpdate:
			n.applyPendingUpdate()
		}
	}
}

func (n *nodeRuntime) checkAndApplyUpdate(now time.Time) {
	n.updMu.Lock()
	defer n.updMu.Unlock()
	if !stableTag.MatchString(n.updater.Version) {
		return
	}
	if err := n.updater.Check(n.ctx, now); err != nil {
		log.Printf("node: update check: %v", err)
	}
	n.syncUpdateStatus()
	n.tryApplyLocked()
}

func (n *nodeRuntime) applyPendingUpdate() {
	n.updMu.Lock()
	defer n.updMu.Unlock()
	n.tryApplyLocked()
}

func (n *nodeRuntime) syncUpdateStatus() {
	var st *proto.Update
	if s := n.updater.Status; s != nil {
		st = &proto.Update{Version: s.Version, Status: s.Status}
	}
	n.mu.Lock()
	n.updStatus = st
	n.mu.Unlock()
}

func (n *nodeRuntime) tryApplyLocked() {
	if !n.updater.pending() {
		return
	}
	gate := host.Part[updateGate](n.host)
	gate.mu.Lock()
	gate.hold = true
	gate.mu.Unlock()
	n.updater.Apply()
	n.mu.Lock()
	restarting := n.restartBin != ""
	n.mu.Unlock()
	if !restarting {
		gate.mu.Lock()
		gate.hold = false
		gate.mu.Unlock()
		go host.Deliver(n.host, "", "")
	}
}

func (n *nodeRuntime) serve(l *Local) string {
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
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.restartBin
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
	lines := []string{
		"version " + Version,
		"fingerprint " + n.key.Fingerprint(),
	}
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
