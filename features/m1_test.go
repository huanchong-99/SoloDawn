//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
)

func init() {
	stepSets["m1"] = func(sc *godog.ScenarioContext, f fixtures) {
		m1Steps(sc, f.bin)
	}
}

type m1Rendezvous struct {
	wsURL    string
	alterSDP atomic.Bool
}

type m1NodeProc struct {
	name   string
	env    []string
	cmd    *exec.Cmd
	output bytes.Buffer
	pairCh chan m1CmdOut // single reader: awaitPairCLIOutput
	peer   proto.Pair
	conn   *peer.Connection
	info   proto.HostState
}

type m1CmdOut struct {
	stdout string
	stderr string
	err    error
}

type m1RoomSpec struct {
	room     string
	secret   identity.Secret
	wantRole string
	rawSelf  []byte
	out      chan<- proto.Pair
}

type m1Browser struct {
	key    identity.Key
	peer   *peer.Peer
	paths  []*peer.Path
	conn   *peer.Connection
	pushes <-chan json.RawMessage // single reader: nextPush in scenario steps

	mu    sync.Mutex
	known map[string]bool
	rooms map[string]m1RoomSpec
}

type m1World struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cleanup func(func())
	bin     string

	mu        sync.Mutex
	now       time.Time
	baseTS    int64
	ice       []proto.ICEServer
	relayOnly bool
	rvs       []*m1Rendezvous

	db          *store.Store
	nodeKey     identity.Key
	otherKey    identity.Key
	logs        *node.Logs
	loop        *home.Loop
	host        *host.Host
	sessions    *node.Sessions
	surface     *node.Surface
	pairer      *node.Pairer
	nodePeer    *peer.Peer
	nodeRVPaths []*peer.Path
	accepted    chan *peer.Connection // single reader: acceptInbound

	procs       map[string]*m1NodeProc
	browsers    []*m1Browser
	ownerLog    []proto.Envelope
	lastWelcome proto.Welcome
	lastServed  *peer.Connection

	shift       int64
	dated       map[string]proto.Envelope
	offers      []proto.Envelope
	replayGot   [][]proto.Envelope
	futureOffer proto.Offer

	outbox     [2]chan proto.Envelope // single reader per index: m1ReadSignalEnv
	earlyRelay [2]proto.Envelope

	pairLinks   map[string]proto.CmdResult
	lastCmdResp json.RawMessage
	createdGrp  proto.ID
}

func m1Steps(sc *godog.ScenarioContext, bin string) {
	now := time.Now()
	w := &m1World{
		cleanup:   afterScenario(sc),
		bin:       bin,
		now:       now,
		baseTS:    now.UnixMilli(),
		procs:     map[string]*m1NodeProc{},
		dated:     map[string]proto.Envelope{},
		pairLinks: map[string]proto.CmdResult{},
	}
	m1RegisterPairingSteps(sc, w)
	m1RegisterTransportSteps(sc, w)
	m1RegisterReplayAndGroupSteps(sc, w)
}

func m1RegisterPairingSteps(sc *godog.ScenarioContext, w *m1World) {
	sc.Step(`^an M1 rendezvous with TURN relay$`, func() error { return w.initTURNAndRVs(true, 1) })
	sc.Step(`^a fresh M1 node process "([^"]*)" and a fresh M1 browser$`, w.startFirstNodeAndBrowser)
	sc.Step(`^the M1 browser starts onboarding pairing with "([^"]*)"$`, w.startOnboardingPair)
	sc.Step(`^the M1 room pair fingerprint equals "solodawn status" on "([^"]*)"$`, w.checkStatusFingerprint)
	sc.Step(`^the M1 browser opens ctl to "([^"]*)" and sends hello before any device log$`, w.openProcCtlAndHello)
	sc.Step(`^the M1 pairing welcome reports owner false and refuses host\.info and commands with "([^"]*)"$`, w.checkBootstrapForbidden)
	sc.Step(`^the M1 browser sends a device log whose add entry for "([^"]*)" was signed by another device$`, w.sendWrongSignerToProc)
	sc.Step(`^"([^"]*)" still has no owner and refuses host\.info with "([^"]*)"$`, w.checkProcStillUnowned)
	sc.Step(`^the M1 browser sends the valid device log adding "([^"]*)" and sends hello again$`, w.adoptProcNode)
	sc.Step(`^"solodawn pair" on "([^"]*)" exits 0 with its fingerprint and welcome reports owner true$`, w.checkPairExitAndWelcome)
	sc.Step(`^a second fresh M1 node process "([^"]*)" pairs with the M1 browser$`, w.pairSecondNodeProc)
	sc.Step(`^the M1 browser lists both nodes with system, version and active path$`, w.checkBothNodesListed)

	sc.Step(`^an M1 owned node with a connected owner browser for browser pairing$`, w.setupPairBrowserNode)
	sc.Step(`^the M1 browser sends pair\.browser with nonce "([^"]*)" twice while valid$`, w.sendPairBrowserTwice)
	sc.Step(`^both M1 pair\.browser answers return the same link and expiry$`, w.checkSamePairBrowserLink)
	sc.Step(`^a second M1 browser pairs through that link$`, w.pairSecondBrowserViaLink)
	sc.Step(`^the second M1 browser adopts the owner, reads owner true and answers host\.info$`, w.checkSecondBrowserReady)
	sc.Step(`^the M1 browser repeats pair\.browser with nonce "([^"]*)" after use$`, w.sendPairBrowserRaw)
	sc.Step(`^the M1 browser sends pair\.browser with nonce "([^"]*)", 10 minutes pass and it repeats "([^"]*)"$`, w.expireAndRepeatPairBrowser)
	sc.Step(`^the M1 pair\.browser answer is refused with "([^"]*)"$`, w.checkLastCmdError)
}

func m1RegisterTransportSteps(sc *godog.ScenarioContext, w *m1World) {
	sc.Step(`^an M1 owned node and browser on a relay-only path with UDP and ICE-TCP blocked$`, func() error {
		return w.setupInProcessOwned(true, 1, 1)
	})
	sc.Step(`^the M1 browser connects over ctl and sends hello$`, w.connectPrimaryBrowser)
	sc.Step(`^both M1 ends report path "([^"]*)" within 1 second$`, w.checkBothEndsPath)
	sc.Step(`^an M1 host\.info request sent every second for (\d+) seconds is answered throughout$`, w.pollHostInfoSeconds)

	sc.Step(`^an M1 owned node and browser signed in on an M1 rendezvous$`, func() error {
		return w.setupInProcessOwned(true, 1, 1)
	})
	sc.Step(`^the M1 rendezvous alters SDP descriptions in transit and the browser offers a connection$`, w.offerWithAlteredSDP)
	sc.Step(`^the M1 offer is refused before ctl opens and the node accepts no session$`, func() error { return nil })
	sc.Step(`^the M1 rendezvous forwards descriptions unchanged and the browser offers a connection$`, w.offerUnchangedSDP)
	sc.Step(`^the M1 connection opens ctl and answers host\.info$`, w.finishPrimaryHelloAndInfo)
}

func m1RegisterReplayAndGroupSteps(sc *godog.ScenarioContext, w *m1World) {
	sc.Step(`^an M1 owned node and browser signed in on two M1 signalling paths$`, func() error {
		return w.setupInProcessOwned(true, 2, 1)
	})
	sc.Step(`^the M1 browser opens a first connection and replaces it with a second connection$`, w.openFirstThenSecond)
	sc.Step(`^the M1 node receives the retired first offer again$`, func() error { return w.replayRecordedOffer(0, 1) })
	sc.Step(`^the M1 node sends no answer, opens no extra connection and answers host\.info on the live connection$`, w.checkReplayIgnored)
	sc.Step(`^the M1 node receives copies of the live offer on two other paths$`, func() error { return w.replayRecordedOffer(1, 2) })
	sc.Step(`^both M1 replay paths receive the same answer and the live connection answers host\.info$`, w.checkReplayCopiesAnswered)
	sc.Step(`^the M1 browser opens a connection dated 119 seconds ahead, replaces it and replays the future-dated offer$`, w.openFutureReplaceAndReplay)
	sc.Step(`^the M1 node sends no answer, remembers it through ts \+ 120000 and answers host\.info on the live connection$`, w.checkFutureRemembered)
	sc.Step(`^the M1 browser opens a connection near expiry, replaces it and replays the retired offer across ts \+ 120000$`, w.openNearExpiryReplaceAndReplay)
	sc.Step(`^the M1 node opens no extra connection and the live connection keeps answering host\.info$`, w.checkNoExtraAndHostInfo)

	sc.Step(`^an M1 owned node and browser whose signalling the test delivers over a relay-only path$`, w.setupControlledSignalling)
	sc.Step(`^the M1 browser offers a connection with two copies of each side's relay candidate delivered before its description$`, w.deliverEarlyRelayCandidates)
	sc.Step(`^another copy of each M1 early relay candidate is delivered$`, w.deliverLateDuplicateCandidates)
	sc.Step(`^the M1 connection still answers host\.info$`, func() error { return w.requestHostInfo(w.browsers[0].conn, w.nodeKey.ID()) })

	sc.Step(`^an M1 owned node with two already-connected browsers of its owner$`, w.setupTwoConnectedBrowsers)
	sc.Step(`^the first M1 browser creates group "([^"]*)"$`, w.createGroupFromFirstBrowser)
	sc.Step(`^the second M1 browser receives a groups push listing "([^"]*)" within 1 second without reconnecting$`, w.checkSecondBrowserGroupsPush)
	sc.Step(`^the second M1 browser syncs "([^"]*)" and the first M1 browser posts "([^"]*)"$`, w.syncSecondAndPostFirst)
	sc.Step(`^the second M1 browser receives that message row within 1 second$`, w.checkSecondBrowserMessageRow)
}

func (w *m1World) initContext() {
	if w.ctx != nil {
		return
	}
	w.ctx, w.cancel = context.WithTimeout(context.Background(), 45*time.Second)
	w.cleanup(w.cancel)
}

func (w *m1World) nowTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

func (w *m1World) initTURNAndRVs(onlyRelay bool, rvCount int) error {
	w.initContext()
	w.relayOnly = onlyRelay
	ice, err := w.startLocalTURN(onlyRelay)
	if err != nil {
		return err
	}
	w.ice = ice
	for range rvCount {
		w.rvs = append(w.rvs, w.startProxyRV(ice))
	}
	return nil
}

func (w *m1World) startLocalTURN(onlyRelay bool) ([]proto.ICEServer, error) {
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ip := net.IPv4(127, 0, 0, 1)
	var gen turn.RelayAddressGenerator = &turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()}
	if onlyRelay {
		gen = &relaySockets{
			RelayAddressGeneratorStatic: turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()},
			ports:                       map[int]bool{},
		}
	}
	auth := func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "m1", "secret"), r.Username == "peer"
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             "m1",
		AuthHandler:       auth,
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen}},
	})
	if err != nil {
		udp.Close()
		return nil, err
	}
	w.cleanup(func() { srv.Close() })
	addr := udp.LocalAddr().String()
	return []proto.ICEServer{
		{URLs: []string{"stun:" + addr}},
		{URLs: []string{"turn:" + addr + "?transport=udp"}, Username: "peer", Credential: "secret"},
	}, nil
}

func (w *m1World) startProxyRV(ice []proto.ICEServer) *m1Rendezvous {
	inner := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(inner.Close)
	innerWS := "ws" + strings.TrimPrefix(inner.URL, "http") + "/rv"
	rv := &m1Rendezvous{}
	mux := http.NewServeMux()
	mux.HandleFunc("/rv", func(rw http.ResponseWriter, r *http.Request) {
		m1ServeProxyWS(rw, r, innerWS, ice, &rv.alterSDP)
	})
	outer := httptest.NewServer(mux)
	w.cleanup(outer.Close)
	rv.wsURL = "ws" + strings.TrimPrefix(outer.URL, "http") + "/rv"
	return rv
}

func m1ServeProxyWS(rw http.ResponseWriter, r *http.Request, innerWS string, ice []proto.ICEServer, alter *atomic.Bool) {
	down, err := websocket.Accept(rw, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
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
	done := make(chan struct{}, 2) // single reader: m1ServeProxyWS
	go func() { m1PumpWS(ctx, down, up, nil); done <- struct{}{} }()
	mapDown := func(b []byte) []byte { return m1RewriteDownstream(b, ice, alter.Load()) }
	go func() { m1PumpWS(ctx, up, down, mapDown); done <- struct{}{} }()
	<-done
}

func m1PumpWS(ctx context.Context, src, dst *websocket.Conn, mapFn func([]byte) []byte) {
	for {
		typ, data, err := src.Read(ctx)
		if err != nil {
			return
		}
		if mapFn != nil {
			data = mapFn(data)
		}
		if dst.Write(ctx, typ, data) != nil {
			return
		}
	}
}

func m1RewriteDownstream(data []byte, ice []proto.ICEServer, alter bool) []byte {
	var f proto.RVFrame
	if proto.Decode(data, &f) != nil {
		return data
	}
	if f.Type == "ready" || f.Type == "ice" {
		return m1WithICE(data, ice)
	}
	if alter && f.Type == "from" {
		return m1AlterRouteSDP(data)
	}
	return data
}

func m1WithICE(data []byte, ice []proto.ICEServer) []byte {
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

func m1AlterRouteSDP(data []byte) []byte {
	var route proto.Route
	var env proto.Envelope
	if proto.Decode(data, &route) != nil || proto.Decode(route.Env, &env) != nil {
		return data
	}
	if !strings.Contains(env.Payload, `"sdp":"`) {
		return data
	}
	env.Payload = strings.Replace(env.Payload, "a=ice-ufrag:", "a=ice-ufrag:X", 1)
	rawEnv, err := json.Marshal(env)
	if err != nil {
		return data
	}
	route.Env = rawEnv
	out, err := json.Marshal(route)
	if err != nil {
		return data
	}
	return out
}

func (w *m1World) newM1Browser(label string) (*m1Browser, error) {
	key := exampleKey("m1-" + label)
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return nil, err
	}
	w.cleanup(func() { socks.Close(); db.Close() })
	b := &m1Browser{key: key, known: map[string]bool{}, rooms: map[string]m1RoomSpec{}}
	routes := func(string) []peer.Route { return w.browserRoutes(b) }
	b.peer = peer.New(key, socks, db, b.isKnown, routes, func(*peer.Connection) {}, nil)
	for _, rv := range w.rvs {
		if err := w.dialBrowserRV(b, rv.wsURL); err != nil {
			return nil, err
		}
	}
	w.serveBrowserPaths(b)
	return b, nil
}

func (b *m1Browser) isKnown(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.known[id]
}

func (b *m1Browser) trust(id string) {
	b.mu.Lock()
	b.known[id] = true
	b.mu.Unlock()
}

func (b *m1Browser) watchRoom(spec m1RoomSpec) {
	b.mu.Lock()
	b.rooms[spec.room] = spec
	b.mu.Unlock()
}

func (b *m1Browser) roomSpec(room string) (m1RoomSpec, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	spec, ok := b.rooms[room]
	return spec, ok
}

func (b *m1Browser) nextPush(ctx context.Context) (json.RawMessage, error) {
	select {
	case msg, ok := <-b.pushes:
		if !ok {
			return nil, errors.New("ctl closed before push")
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *m1World) dialBrowserRV(b *m1Browser, wsURL string) error {
	p, err := peer.Dial(w.ctx, wsURL, b.key)
	if err != nil {
		return err
	}
	w.cleanup(func() { p.Close() })
	b.paths = append(b.paths, p)
	return nil
}

func (w *m1World) serveBrowserPaths(b *m1Browser) {
	for _, p := range b.paths {
		path := p
		event := func(kind string, data []byte) error {
			b.onRVEvent(w.ctx, path, kind, data)
			return nil
		}
		go path.Serve(w.ctx, b.peer, event)
	}
}

func (b *m1Browser) onRVEvent(ctx context.Context, p *peer.Path, kind string, data []byte) {
	var r proto.RoomID
	if proto.Decode(data, &r) != nil {
		return
	}
	spec, ok := b.roomSpec(r.Room)
	if !ok {
		return
	}
	m1DispatchRoomSpec(ctx, p, spec, kind, data)
}

func m1DispatchRoomSpec(ctx context.Context, p *peer.Path, spec m1RoomSpec, kind string, data []byte) {
	if kind == "joined" {
		p.Write(ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: spec.room}, Data: spec.rawSelf})
		return
	}
	if kind == "room" {
		m1DeliverRoomPair(spec, data)
	}
}

func m1DeliverRoomPair(spec m1RoomSpec, data []byte) {
	m, ok := m1DecodeVerifiedPair(spec, data)
	if !ok {
		return
	}
	select {
	case spec.out <- m:
	default:
	}
}

func m1DecodeVerifiedPair(spec m1RoomSpec, data []byte) (proto.Pair, bool) {
	var rd proto.RoomData
	var m proto.Pair
	if proto.Decode(data, &rd) != nil || proto.Decode(rd.Data, &m) != nil {
		return proto.Pair{}, false
	}
	return m, spec.secret.Verify(m, spec.wantRole)
}

func m1NewRoomSpec(devID, name, selfRole, wantRole string, secret identity.Secret, out chan<- proto.Pair) m1RoomSpec {
	self := proto.Pair{Type: "pair", Key: devID, Kind: "browser", Name: name, Role: selfRole}
	self.MAC = secret.MAC(self)
	rawSelf, err := json.Marshal(self)
	if err != nil {
		panic(err)
	}
	return m1RoomSpec{room: secret.Room(), secret: secret, wantRole: wantRole, rawSelf: rawSelf, out: out}
}

func (w *m1World) browserRoutes(b *m1Browser) []peer.Route {
	var out []peer.Route
	for _, p := range b.paths {
		out = append(out, w.wrapBrowserRoute(b, p.Route()))
	}
	return out
}

func (w *m1World) wrapBrowserRoute(b *m1Browser, r peer.Route) peer.Route {
	send := r.Send
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		if w.dropNonRelay(env.Payload) {
			return nil
		}
		if len(w.browsers) > 0 && b == w.browsers[0] && strings.Contains(env.Payload, `"t":"offer"`) {
			env = w.recordOffer(b.key, env)
		}
		return send(ctx, to, env)
	}
	return r
}

func (w *m1World) dropNonRelay(payload string) bool {
	if !w.relayOnly || !strings.Contains(payload, `"t":"ice"`) {
		return false
	}
	return !strings.Contains(payload, " typ relay")
}

func (w *m1World) recordOffer(signer identity.Key, env proto.Envelope) proto.Envelope {
	var o proto.Offer
	if json.Unmarshal([]byte(env.Payload), &o) != nil {
		return env
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if prev, ok := w.dated[o.Conn]; ok {
		return prev
	}
	o.Timestamp += w.shift
	raw, err := json.Marshal(o)
	if err != nil {
		return env
	}
	signed := signer.Sign(string(raw))
	w.dated[o.Conn] = signed
	w.offers = append(w.offers, signed)
	if w.shift > 0 {
		w.futureOffer = o
	}
	return signed
}

func (w *m1World) startFirstNodeAndBrowser(name string) error {
	w.otherKey = exampleKey("m1-stranger")
	if runtime.GOOS == "windows" {
		w.cleanup(func() { exec.Command(w.bin, "service", "remove").Run() })
	}
	b, err := w.newM1Browser("b0")
	if err != nil {
		return err
	}
	w.browsers = append(w.browsers, b)
	return w.startNodeSubprocess(name)
}

func (w *m1World) startNodeSubprocess(name string) error {
	base, rel := os.TempDir(), "solodawn"
	if runtime.GOOS == "darwin" {
		base, rel = "/tmp", filepath.Join("Library", "Application Support", "solodawn")
	}
	root, err := os.MkdirTemp(base, "sdm1")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(root) })
	sock := filepath.Join(root, rel, "sock")
	env := append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "XDG_CONFIG_HOME="+root, "LocalAppData="+root, "USERPROFILE="+root)
	np := &m1NodeProc{name: name, env: env, pairCh: make(chan m1CmdOut, 1)}
	cmd := exec.Command(w.bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.output, &np.output
	if err := cmd.Start(); err != nil {
		return err
	}
	np.cmd = cmd
	w.cleanup(func() { m1StopProcess(cmd) })
	w.procs[name] = np
	return m1WaitUnixSocket(sock, 10*time.Second, &np.output)
}

func m1StopProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	done := make(chan struct{}) // single reader: m1StopProcess
	go func() { cmd.Wait(); close(done) }()
	if cmd.Process.Signal(os.Interrupt) != nil {
		cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

func m1WaitUnixSocket(sock string, wait time.Duration, logs *bytes.Buffer) error {
	deadline := time.Now().Add(wait)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return conn.Close()
		}
		last = err
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("socket %s not ready: %w (%s)", sock, last, logs.String())
}

func (w *m1World) startOnboardingPair(name string) error {
	b, np := w.browsers[0], w.procs[name]
	secret := identity.NewSecret()
	found := make(chan proto.Pair, 1) // single reader: waitRoomPeer
	spec := m1NewRoomSpec(b.key.ID(), "owner-browser", "a", "b", secret, found)
	b.watchRoom(spec)
	if err := b.paths[0].Write(w.ctx, proto.RoomID{Type: "join", Room: spec.room}); err != nil {
		return err
	}
	go w.runPairCLI(np, secret.String(), name)
	peerMsg, err := w.waitRoomPeer(found, np)
	if err != nil {
		return err
	}
	np.peer = peerMsg
	b.trust(peerMsg.Key)
	return nil
}

func (w *m1World) runPairCLI(np *m1NodeProc, secret, label string) {
	cmd := exec.Command(w.bin, "pair", secret, "--name", label, "--rendezvous", w.rvs[0].wsURL)
	cmd.Env = np.env
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	np.pairCh <- m1CmdOut{stdout: out.String(), stderr: errBuf.String(), err: err}
}

func (w *m1World) waitRoomPeer(found <-chan proto.Pair, np *m1NodeProc) (proto.Pair, error) {
	select {
	case m := <-found:
		return m, nil
	case res := <-np.pairCh:
		np.pairCh <- res
		return proto.Pair{}, fmt.Errorf("solodawn pair exited early (%v): stdout=%q stderr=%q", res.err, res.stdout, res.stderr)
	case <-w.ctx.Done():
		return proto.Pair{}, w.ctx.Err()
	}
}

func (w *m1World) checkStatusFingerprint(name string) error {
	np := w.procs[name]
	roomFP, err := m1Fingerprint(np.peer.Key)
	if err != nil {
		return err
	}
	out, err := w.runSubcmd(np, "status")
	if err != nil {
		return err
	}
	statusFP := m1ExtractPrefixedLine(out, "fingerprint ")
	if statusFP == "" || statusFP != roomFP {
		return fmt.Errorf("room fingerprint %q, status output %q", roomFP, out)
	}
	return nil
}

func (w *m1World) runSubcmd(np *m1NodeProc, args ...string) (string, error) {
	cmd := exec.Command(w.bin, args...)
	cmd.Env = np.env
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil || errBuf.Len() != 0 {
		return "", fmt.Errorf("%v failed (%v): stdout=%q stderr=%q", args, err, out.String(), errBuf.String())
	}
	return out.String(), nil
}

func m1Fingerprint(deviceID string) (string, error) {
	pub, err := base64.RawURLEncoding.DecodeString(deviceID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pub)
	return fmt.Sprintf("%X %X %X", sum[0:2], sum[2:4], sum[4:6]), nil
}

func m1ExtractPrefixedLine(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, prefix); ok && rest != "" {
			return rest
		}
	}
	return ""
}

func (w *m1World) openProcCtlAndHello(name string) error {
	b, np := w.browsers[0], w.procs[name]
	conn, _, err := w.dialPeerCtl(b.peer, np.peer.Key)
	if err != nil {
		return err
	}
	np.conn = conn
	if err := m1ExpectRequestCode(w.ctx, conn, "host.info", "forbidden"); err != nil {
		return err
	}
	return w.sendHello(conn)
}

func (w *m1World) checkBootstrapForbidden(code string) error {
	if w.lastWelcome.Owner {
		return errors.New("welcome reported owner true before device log")
	}
	conn := w.procs["workstation"].conn
	if err := m1ExpectRequestCode(w.ctx, conn, "host.info", code); err != nil {
		return err
	}
	env := w.browsers[0].key.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, exampleID("m1-boot"), time.Now().UnixMilli(), w.procs["workstation"].peer.Key))
	return m1ExpectCmdCode(w.ctx, conn, env, code)
}

func (w *m1World) sendWrongSignerToProc(name string) error {
	b, np := w.browsers[0], w.procs[name]
	now := time.Now().UnixMilli()
	e1 := m1SignDeviceAdd(b.key, b.key.ID(), 1, b.key.ID(), "browser", "b0", now)
	e2 := m1SignDeviceAdd(b.key, b.key.ID(), 2, w.otherKey.ID(), "browser", "other", now)
	e3 := m1SignDeviceAdd(w.otherKey, b.key.ID(), 3, np.peer.Key, "node", name, now)
	return np.conn.Send(proto.Devlog{Type: "devlog", User: b.key.ID(), Entries: []proto.Envelope{e1, e2, e3}})
}

func (w *m1World) checkProcStillUnowned(name, code string) error {
	np := w.procs[name]
	if err := w.sendHello(np.conn); err != nil {
		return err
	}
	if w.lastWelcome.Owner {
		return errors.New("node adopted log signed by wrong device")
	}
	return m1ExpectRequestCode(w.ctx, np.conn, "host.info", code)
}

func (w *m1World) adoptProcNode(name string) error {
	b, np := w.browsers[0], w.procs[name]
	now := time.Now().UnixMilli()
	if len(w.ownerLog) == 0 {
		w.ownerLog = append(w.ownerLog, m1SignDeviceAdd(b.key, b.key.ID(), 1, b.key.ID(), "browser", "b0", now))
	}
	ver := len(w.ownerLog) + 1
	w.ownerLog = append(w.ownerLog, m1SignDeviceAdd(b.key, b.key.ID(), ver, np.peer.Key, "node", name, now))
	if err := np.conn.Send(proto.Devlog{Type: "devlog", User: b.key.ID(), Entries: slices.Clone(w.ownerLog)}); err != nil {
		return err
	}
	return w.sendHello(np.conn)
}

func (w *m1World) checkPairExitAndWelcome(name string) error {
	np := w.procs[name]
	if err := w.awaitPairCLIOutput(np); err != nil {
		return err
	}
	if !w.lastWelcome.Owner {
		return errors.New("welcome.owner is false after adoption")
	}
	info, err := m1FetchHostInfo(w.ctx, np.conn)
	np.info = info
	return err
}

func (w *m1World) awaitPairCLIOutput(np *m1NodeProc) error {
	fp, err := m1Fingerprint(np.peer.Key)
	if err != nil {
		return err
	}
	select {
	case res := <-np.pairCh:
		return m1CheckPairOut(res, fp)
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func m1CheckPairOut(res m1CmdOut, fp string) error {
	if res.err != nil || !strings.Contains(res.stdout, fp) {
		return fmt.Errorf("pair (%v): stdout=%q stderr=%q want fp %q", res.err, res.stdout, res.stderr, fp)
	}
	return nil
}

func (w *m1World) pairSecondNodeProc(name string) error {
	if err := w.startNodeSubprocess(name); err != nil {
		return err
	}
	if err := w.startOnboardingPair(name); err != nil {
		return err
	}
	return w.enrollSecondNodeProc(name)
}

func (w *m1World) enrollSecondNodeProc(name string) error {
	b, np := w.browsers[0], w.procs[name]
	conn, _, err := w.dialPeerCtl(b.peer, np.peer.Key)
	if err != nil {
		return err
	}
	np.conn = conn
	if err := w.sendHello(conn); err != nil {
		return err
	}
	if err := w.adoptProcNode(name); err != nil {
		return err
	}
	entry := w.ownerLog[len(w.ownerLog)-1]
	if err := w.procs["workstation"].conn.Send(proto.Devlog{Type: "devlog", User: b.key.ID(), Entries: []proto.Envelope{entry}}); err != nil {
		return err
	}
	return w.checkPairExitAndWelcome(name)
}

func (w *m1World) checkBothNodesListed() error {
	seen := map[string]bool{}
	for _, name := range []string{"workstation", "laptop"} {
		dev, err := w.verifyNodeSummary(name)
		if err != nil {
			return err
		}
		seen[dev] = true
	}
	if len(seen) != 2 {
		return fmt.Errorf("expected 2 distinct nodes, got %d", len(seen))
	}
	return nil
}

func (w *m1World) verifyNodeSummary(name string) (string, error) {
	np := w.procs[name]
	info, err := m1FetchHostInfo(w.ctx, np.conn)
	if err != nil {
		return "", err
	}
	path := np.conn.Path()
	if !m1ValidNodeSummary(info, name, path) {
		return "", fmt.Errorf("%s: info=%+v path=%q", name, info, path)
	}
	return info.Device, nil
}

func m1ValidNodeSummary(info proto.HostState, wantName, path string) bool {
	if info.Name != wantName || info.OS == "" || info.Version == "" {
		return false
	}
	return path == "direct" || path == "relay"
}

func (w *m1World) setupInProcessOwned(onlyRelay bool, rvCount, browserCount int) error {
	if err := w.initTURNAndRVs(onlyRelay, rvCount); err != nil {
		return err
	}
	w.nodeKey = exampleKey("m1-inproc-node")
	w.accepted = make(chan *peer.Connection, 16)
	for i := range browserCount {
		b, err := w.newM1Browser(fmt.Sprintf("browser-%d", i))
		if err != nil {
			return err
		}
		b.trust(w.nodeKey.ID())
		w.browsers = append(w.browsers, b)
	}
	return w.startInProcessNode()
}

func (w *m1World) startInProcessNode() error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db
	if err := w.seedInProcessOwner(); err != nil {
		return err
	}
	if err := w.initInProcessServices(); err != nil {
		return err
	}
	return w.startInProcessPeer()
}

func (w *m1World) seedInProcessOwner() error {
	owner := w.browsers[0].key
	now := w.nowTime().UnixMilli()
	entries := []proto.Envelope{
		m1SignDeviceAdd(owner, owner.ID(), 1, owner.ID(), "browser", "b0", now),
		m1SignDeviceAdd(owner, owner.ID(), 2, w.nodeKey.ID(), "node", "m1-node", now),
	}
	for i := 1; i < len(w.browsers); i++ {
		entries = append(entries, m1SignDeviceAdd(owner, owner.ID(), len(entries)+1, w.browsers[i].key.ID(), "browser", fmt.Sprintf("b%d", i), now))
	}
	w.ownerLog = entries
	return w.writeSeededStore(owner.ID(), entries)
}

func (w *m1World) writeSeededStore(ownerID string, entries []proto.Envelope) error {
	ownerJSON, err := json.Marshal(ownerID)
	if err != nil {
		return err
	}
	rvURLs := make([]string, 0, len(w.rvs))
	for _, rv := range w.rvs {
		rvURLs = append(rvURLs, rv.wsURL)
	}
	rvJSON, err := json.Marshal(rvURLs)
	if err != nil {
		return err
	}
	write := func(tx *store.Tx) error {
		_, wErr := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?), ('name', '\"m1-node\"'), ('rendezvous', ?)", string(ownerJSON), string(rvJSON))
		return errors.Join(wErr, m1WriteLogEntries(tx, ownerID, entries))
	}
	return w.db.Write(write)
}

func m1WriteLogEntries(tx *store.Tx, ownerID string, entries []proto.Envelope) error {
	var allErr error
	for i, env := range entries {
		raw, err := json.Marshal(env)
		if err != nil {
			return err
		}
		_, wErr := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", ownerID, i+1, string(raw))
		allErr = errors.Join(allErr, wErr)
	}
	return allErr
}

func (w *m1World) initInProcessServices() error {
	logs, err := node.OpenLogs(w.db, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.logs = logs
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: w.nodeKey.ID(), Owner: logs.Owner(),
		Logs: logs.Log, Tables: home.NodeTables(), Now: w.nowTime,
	})
	if err != nil {
		return err
	}
	w.cleanup(loop.Close)
	w.loop = loop
	h, _, err := host.New(w.db, w.nodeKey, w.nowTime)
	if err != nil {
		return err
	}
	w.host = h
	link := node.AttachLocal(h, loop)
	w.cleanup(func() { link.Close() })
	w.wireInProcessSurfaceAndPairer()
	return nil
}

func (w *m1World) wireInProcessSurfaceAndPairer() {
	nameFn := func() string { return node.ReadName(w.db) }
	w.sessions = node.NewSessions(node.SessionConfig{
		ID: w.nodeKey.ID(), Version: "v0.1.0", Store: w.db, Logs: w.logs, Home: w.loop, Host: w.host,
		Name: nameFn, Now: w.nowTime,
	})
	w.surface = node.NewSurface(node.SurfaceConfig{
		ID: w.nodeKey.ID(), Version: "v0.1.0", Store: w.db, Sessions: w.sessions,
		Host: w.host, Name: nameFn, Agents: w.host.Agents,
	})
	w.pairer = node.NewPairer(node.PairConfig{
		Key: w.nodeKey, Store: w.db, Logs: w.logs, Sessions: w.sessions,
		Paths: func() []*peer.Path { return slices.Clone(w.nodeRVPaths) },
		Now:   w.nowTime,
	})
	w.cleanup(w.pairer.Close)
}

func (w *m1World) startInProcessPeer() error {
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	w.cleanup(func() { socks.Close() })
	for _, rv := range w.rvs {
		p, err := peer.Dial(w.ctx, rv.wsURL, w.nodeKey)
		if err != nil {
			return err
		}
		w.cleanup(func() { p.Close() })
		w.nodeRVPaths = append(w.nodeRVPaths, p)
	}
	known := func(dev string) bool { return w.logs.Known(dev) || w.pairer.Known(dev) }
	routes := func(dev string) []peer.Route { return w.nodeRoutesFor(dev) }
	inbound := func(c *peer.Connection) {
		w.accepted <- c
		go w.pairer.Serve(w.ctx, c.Device, c)
	}
	w.nodePeer = peer.New(w.nodeKey, socks, w.db, known, routes, inbound, nil)
	w.pairer.Peer = w.nodePeer
	w.serveNodeRVPaths()
	return nil
}

func (w *m1World) serveNodeRVPaths() {
	for _, p := range w.nodeRVPaths {
		path := p
		event := func(kind string, data []byte) error {
			w.pairer.HandleEvent(w.ctx, path, kind, data)
			return nil
		}
		go path.Serve(w.ctx, w.nodePeer, event)
	}
}

func (w *m1World) nodeRoutesFor(dev string) []peer.Route {
	var out []peer.Route
	for _, p := range w.nodeRVPaths {
		out = append(out, w.wrapNodeRoute(p.Route()))
	}
	for _, r := range w.pairer.Routes(dev) {
		out = append(out, w.wrapNodeRoute(r))
	}
	return out
}

func (w *m1World) wrapNodeRoute(r peer.Route) peer.Route {
	send := r.Send
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		if w.dropNonRelay(env.Payload) {
			return nil
		}
		return send(ctx, to, env)
	}
	return r
}

func (w *m1World) dialPeerCtl(p *peer.Peer, target string) (*peer.Connection, <-chan json.RawMessage, error) {
	conn, err := p.Offer(w.ctx, target)
	if err != nil {
		return nil, nil, err
	}
	w.cleanup(func() { conn.Close() })
	select {
	case <-w.ctx.Done():
		return nil, nil, w.ctx.Err()
	case <-conn.Done():
		return nil, nil, errors.New("ctl closed before opening")
	case <-conn.Ready():
		pushes := make(chan json.RawMessage, 64) // single reader: m1Browser.nextPush
		go m1PumpPushes(w.ctx, conn, pushes)
		return conn, pushes, nil
	}
}

func m1PumpPushes(ctx context.Context, conn *peer.Connection, pushes chan<- json.RawMessage) {
	defer close(pushes)
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case pushes <- msg:
		case <-ctx.Done():
			return
		case <-conn.Done():
			return
		}
	}
}

func (w *m1World) connectPrimaryBrowser() error {
	b := w.browsers[0]
	conn, pushes, err := w.dialPeerCtl(b.peer, w.nodeKey.ID())
	if err != nil {
		return err
	}
	b.conn, b.pushes = conn, pushes
	served, err := w.acceptInbound()
	if err != nil {
		return err
	}
	w.lastServed = served
	return w.sendHello(conn)
}

func (w *m1World) acceptInbound() (*peer.Connection, error) {
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case c := <-w.accepted:
		return c, nil
	}
}

func (w *m1World) sendHello(conn *peer.Connection) error {
	build := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "browser", Addrs: []string{}}
	}
	raw, err := conn.Request(w.ctx, build)
	if err != nil {
		return err
	}
	return proto.Decode(raw, &w.lastWelcome)
}

func (w *m1World) checkBothEndsPath(want string) error {
	for _, c := range []*peer.Connection{w.browsers[0].conn, w.lastServed} {
		if err := m1WaitPath(c, want, time.Second); err != nil {
			return err
		}
	}
	return nil
}

func m1WaitPath(conn *peer.Connection, want string, limit time.Duration) error {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		if conn.Path() == want {
			return nil
		}
		if !m1WaitPathChange(conn, timer.C) {
			return fmt.Errorf("path %q after %s, want %q", conn.Path(), limit, want)
		}
	}
}

func m1WaitPathChange(conn *peer.Connection, timeout <-chan time.Time) bool {
	select {
	case <-conn.PathChanged():
		return true
	case <-conn.Done():
		return false
	case <-timeout:
		return false
	}
}

func (w *m1World) pollHostInfoSeconds(seconds int) error {
	conn := w.browsers[0].conn
	if err := w.requestHostInfo(conn, w.nodeKey.ID()); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range seconds {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-ticker.C:
		}
		if err := w.requestHostInfo(conn, w.nodeKey.ID()); err != nil {
			return err
		}
	}
	return nil
}

func (w *m1World) requestHostInfo(conn *peer.Connection, wantDevice string) error {
	info, err := m1FetchHostInfo(w.ctx, conn)
	if err != nil {
		return err
	}
	if info.Device != wantDevice {
		return fmt.Errorf("host.info device = %q, want %q", info.Device, wantDevice)
	}
	return nil
}

func m1FetchHostInfo(ctx context.Context, conn *peer.Connection) (proto.HostState, error) {
	build := func(id int64) any { return proto.Request{Type: "host.info", ID: id} }
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return proto.HostState{}, err
	}
	var ans proto.HostInfo
	if err := proto.Decode(raw, &ans); err != nil {
		return proto.HostState{}, fmt.Errorf("host.info = %s (%v)", raw, err)
	}
	return ans.HostState, nil
}

func (w *m1World) offerWithAlteredSDP() error {
	w.rvs[0].alterSDP.Store(true)
	conn, err := w.browsers[0].peer.Offer(w.ctx, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.cleanup(func() { conn.Close() })
	select {
	case <-conn.Ready():
		return errors.New("ctl opened despite altered SDP description")
	case <-conn.Done():
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
	if len(w.accepted) != 0 {
		return fmt.Errorf("node accepted %d connections on altered SDP", len(w.accepted))
	}
	return nil
}

func (w *m1World) offerUnchangedSDP() error {
	w.rvs[0].alterSDP.Store(false)
	return w.connectPrimaryBrowser()
}

func (w *m1World) finishPrimaryHelloAndInfo() error {
	if !w.lastWelcome.Owner {
		return errors.New("expected owner true")
	}
	return w.requestHostInfo(w.browsers[0].conn, w.nodeKey.ID())
}

func (w *m1World) setupPairBrowserNode() error {
	if err := w.setupInProcessOwned(true, 1, 1); err != nil {
		return err
	}
	if err := w.connectPrimaryBrowser(); err != nil {
		return err
	}
	return m1DrainInitialDevlog(w.ctx, w.browsers[0])
}

func (w *m1World) sendPairBrowserTwice(tag string) error {
	first, err := w.callPairBrowserOK(tag)
	if err != nil {
		return err
	}
	second, err := w.callPairBrowserOK(tag)
	if err != nil {
		return err
	}
	w.pairLinks[tag+":1"] = first
	w.pairLinks[tag+":2"] = second
	return nil
}

func (w *m1World) callPairBrowserOK(tag string) (proto.CmdResult, error) {
	raw, err := w.callPairBrowserRaw(tag)
	if err != nil {
		return proto.CmdResult{}, err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return proto.CmdResult{}, fmt.Errorf("pair.browser %s: %w", raw, err)
	}
	return res, nil
}

func (w *m1World) callPairBrowserRaw(tag string) (json.RawMessage, error) {
	env := w.browsers[0].key.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`,
		exampleID("m1-pb-"+tag), w.baseTS, w.nodeKey.ID()))
	return w.browsers[0].conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
}

func (w *m1World) checkSamePairBrowserLink() error {
	first := w.pairLinks["link-one:1"]
	second := w.pairLinks["link-one:2"]
	if first.Link == "" || first.Link != second.Link || first.Expires != second.Expires || first.Re == second.Re {
		return fmt.Errorf("first=%+v second=%+v", first, second)
	}
	return nil
}

func (w *m1World) pairSecondBrowserViaLink() error {
	sec, err := m1ParseSecretFromLink(w.pairLinks["link-one:1"].Link)
	if err != nil {
		return err
	}
	b2, err := w.newM1Browser("b2-pair")
	if err != nil {
		return err
	}
	b2.trust(w.nodeKey.ID())
	w.browsers = append(w.browsers, b2)
	if err := w.exchangeBrowserRoom(b2, sec); err != nil {
		return err
	}
	conn, pushes, err := w.dialPeerCtl(b2.peer, w.nodeKey.ID())
	if err != nil {
		return err
	}
	b2.conn, b2.pushes = conn, pushes
	_, err = w.acceptInbound()
	return err
}

func m1ParseSecretFromLink(link string) (identity.Secret, error) {
	_, code, ok := strings.Cut(link, "/#pair=")
	if !ok {
		return "", fmt.Errorf("missing #pair= in %q", link)
	}
	return identity.ParseSecret(code)
}

func (w *m1World) exchangeBrowserRoom(b *m1Browser, secret identity.Secret) error {
	found := make(chan proto.Pair, 1) // single reader: exchangeBrowserRoom
	spec := m1NewRoomSpec(b.key.ID(), "second-browser", "b", "a", secret, found)
	b.watchRoom(spec)
	if err := b.paths[0].Write(w.ctx, proto.RoomID{Type: "join", Room: spec.room}); err != nil {
		return err
	}
	select {
	case <-found:
		return w.awaitPairerKnown(b.key.ID())
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *m1World) awaitPairerKnown(device string) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w.pairer.Known(device) {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("node pairer did not record %q", device)
}

func (w *m1World) checkSecondBrowserReady() error {
	b2 := w.browsers[1]
	if err := w.sendHello(b2.conn); err != nil || w.lastWelcome.Owner {
		return fmt.Errorf("first hello owner=%v err=%v", w.lastWelcome.Owner, err)
	}
	if err := w.verifyNodeAdoptsBrowser(b2); err != nil {
		return err
	}
	if err := w.sendHello(b2.conn); err != nil || !w.lastWelcome.Owner {
		return fmt.Errorf("second hello owner=%v err=%v", w.lastWelcome.Owner, err)
	}
	return w.requestHostInfo(b2.conn, w.nodeKey.ID())
}

func (w *m1World) verifyNodeAdoptsBrowser(b *m1Browser) error {
	raw, err := b.nextPush(w.ctx)
	if err != nil {
		return err
	}
	var dl proto.Devlog
	if err := proto.Decode(raw, &dl); err != nil {
		return err
	}
	logVal, err := identity.Replay(w.browsers[0].key.ID(), dl.Entries)
	if err != nil || !logVal.Adopts(b.key.ID(), w.nodeKey.ID()) {
		return fmt.Errorf("log did not adopt second browser: %v", err)
	}
	return nil
}

func (w *m1World) sendPairBrowserRaw(tag string) error {
	raw, err := w.callPairBrowserRaw(tag)
	w.lastCmdResp = raw
	return err
}

func (w *m1World) expireAndRepeatPairBrowser(tag, repeatTag string) error {
	if _, err := w.callPairBrowserOK(tag); err != nil {
		return err
	}
	w.mu.Lock()
	w.now = w.now.Add(10 * time.Minute)
	w.mu.Unlock()
	return w.sendPairBrowserRaw(repeatTag)
}

func (w *m1World) checkLastCmdError(wantCode string) error {
	return checkWireError(w.lastCmdResp, wantCode)
}

func (w *m1World) openShiftedAndHello(shift int64) error {
	w.mu.Lock()
	w.shift = shift
	w.mu.Unlock()
	return w.connectPrimaryBrowser()
}

func (w *m1World) openFirstThenSecond() error {
	if err := w.openShiftedAndHello(0); err != nil {
		return err
	}
	first := w.browsers[0].conn
	if err := w.openShiftedAndHello(0); err != nil {
		return err
	}
	return m1WaitClosed(w.ctx, first)
}

func m1WaitClosed(ctx context.Context, conn *peer.Connection) error {
	select {
	case <-ctx.Done():
		return errors.New("earlier connection stayed open")
	case <-conn.Done():
		return nil
	}
}

func (w *m1World) replayRecordedOffer(idx, copies int) error {
	w.mu.Lock()
	offer := w.offers[idx]
	w.mu.Unlock()
	w.replayGot = nil
	var channels []chan proto.Envelope
	for range copies {
		ch := make(chan proto.Envelope, 2) // single reader: m1DrainReplay
		route := peer.Route{ID: "m1-replay", Send: func(_ context.Context, _ string, env proto.Envelope) error {
			ch <- env
			return nil
		}}
		rawEnv, err := json.Marshal(offer)
		if err != nil {
			return err
		}
		w.nodePeer.Receive(w.ctx, route, proto.Route{Type: "from", Key: offer.Key, Env: rawEnv})
		channels = append(channels, ch)
	}
	w.replayGot = m1DrainReplay(channels)
	return nil
}

func m1DrainReplay(channels []chan proto.Envelope) [][]proto.Envelope {
	var out [][]proto.Envelope
	for _, ch := range channels {
		var list []proto.Envelope
		select {
		case env := <-ch:
			list = append(list, env)
		case <-time.After(300 * time.Millisecond):
		}
		for len(ch) > 0 {
			list = append(list, <-ch)
		}
		out = append(out, list)
	}
	return out
}

func (w *m1World) checkReplayIgnored() error {
	if len(w.replayGot) != 1 || len(w.replayGot[0]) != 0 {
		return fmt.Errorf("expected no answer to retired offer, got %v", w.replayGot)
	}
	return w.checkNoExtraAndHostInfo()
}

func (w *m1World) checkReplayCopiesAnswered() error {
	if len(w.replayGot) != 2 || len(w.replayGot[0]) != 1 || !slices.Equal(w.replayGot[0], w.replayGot[1]) {
		return fmt.Errorf("unexpected replay answers %v", w.replayGot)
	}
	return w.checkNoExtraAndHostInfo()
}

func (w *m1World) checkNoExtraAndHostInfo() error {
	if n := len(w.accepted); n != 0 {
		return fmt.Errorf("node accepted %d extra connections", n)
	}
	return w.requestHostInfo(w.browsers[0].conn, w.nodeKey.ID())
}

func (w *m1World) openFutureReplaceAndReplay() error {
	futureIdx := len(w.offers)
	if err := w.openShiftedAndHello(119_000); err != nil {
		return err
	}
	first := w.browsers[0].conn
	if err := w.openShiftedAndHello(0); err != nil {
		return err
	}
	if err := m1WaitClosed(w.ctx, first); err != nil {
		return err
	}
	return w.replayRecordedOffer(futureIdx, 1)
}

func (w *m1World) checkFutureRemembered() error {
	if err := w.checkReplayIgnored(); err != nil {
		return err
	}
	cutoff := w.futureOffer.Timestamp + 120_000
	atCutoff, err := w.db.HasOffer(w.browsers[0].key.ID(), w.futureOffer.Conn, cutoff)
	if err != nil || !atCutoff {
		return fmt.Errorf("HasOffer at ts+120000 = %v, %v", atCutoff, err)
	}
	afterCutoff, err := w.db.HasOffer(w.browsers[0].key.ID(), w.futureOffer.Conn, cutoff+1)
	if err != nil || afterCutoff {
		return fmt.Errorf("HasOffer at ts+120001 = %v, %v", afterCutoff, err)
	}
	return nil
}

func (w *m1World) openNearExpiryReplaceAndReplay() error {
	nearIdx := len(w.offers)
	if err := w.openShiftedAndHello(-114_500); err != nil {
		return err
	}
	first := w.browsers[0].conn
	var nearOffer proto.Offer
	if err := json.Unmarshal([]byte(w.offers[nearIdx].Payload), &nearOffer); err != nil {
		return err
	}
	if err := w.openShiftedAndHello(0); err != nil {
		return err
	}
	if err := m1WaitClosed(w.ctx, first); err != nil {
		return err
	}
	return w.replayBeforeAndAfterCutoff(nearIdx, nearOffer.Timestamp+120_000)
}

func (w *m1World) replayBeforeAndAfterCutoff(idx int, cutoff int64) error {
	if err := w.replayRecordedOffer(idx, 1); err != nil {
		return err
	}
	if wait := time.Until(time.UnixMilli(cutoff)); wait > 0 {
		time.Sleep(wait)
	}
	return w.replayRecordedOffer(idx, 1)
}

func (w *m1World) setupControlledSignalling() error {
	w.initContext()
	ice, err := w.startLocalTURN(true)
	if err != nil {
		return err
	}
	w.ice = ice
	w.nodeKey = exampleKey("m1-early-node")
	w.accepted = make(chan *peer.Connection, 4)
	w.outbox[0] = make(chan proto.Envelope, 128)
	w.outbox[1] = make(chan proto.Envelope, 128)
	bKey := exampleKey("m1-early-browser")
	w.browsers = []*m1Browser{{key: bKey, known: map[string]bool{w.nodeKey.ID(): true}}}
	if err := w.initControlledNode(); err != nil {
		return err
	}
	bDB, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { bDB.Close() })
	p, err := w.makeControlledPeer(bKey, bDB, 0, func(id string) bool { return id == w.nodeKey.ID() }, func(*peer.Connection) {})
	w.browsers[0].peer = p
	return err
}

func (w *m1World) initControlledNode() error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db
	if err := w.seedInProcessOwner(); err != nil {
		return err
	}
	if err := w.initInProcessServices(); err != nil {
		return err
	}
	inbound := func(c *peer.Connection) {
		w.accepted <- c
		go w.pairer.Serve(w.ctx, c.Device, c)
	}
	p, err := w.makeControlledPeer(w.nodeKey, w.db, 1, w.logs.Known, inbound)
	w.pairer.Peer = p
	w.nodePeer = p
	return err
}

func (w *m1World) makeControlledPeer(key identity.Key, db *store.Store, side int, known func(string) bool, inbound func(*peer.Connection)) (*peer.Peer, error) {
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { socks.Close() })
	routes := func(string) []peer.Route { return []peer.Route{w.controlledRoute(side)} }
	return peer.New(key, socks, db, known, routes, inbound, nil), nil
}

func (w *m1World) controlledRoute(side int) peer.Route {
	return peer.Route{
		ID:  fmt.Sprintf("m1-local-%d", side),
		ICE: w.ice,
		Send: func(ctx context.Context, _ string, env proto.Envelope) error {
			select {
			case w.outbox[side] <- env:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

func (w *m1World) deliverEarlyRelayCandidates() error {
	b := w.browsers[0]
	conn, err := b.peer.Offer(w.ctx, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.cleanup(func() { conn.Close() })
	b.conn = conn
	if err := w.stageEarlySide(0, w.nodePeer, 1); err != nil {
		return err
	}
	if err := w.stageEarlySide(1, b.peer, 0); err != nil {
		return err
	}
	return w.waitControlledOpen(b)
}

func (w *m1World) stageEarlySide(senderIdx int, target *peer.Peer, targetIdx int) error {
	desc, relayCand, err := m1CollectDescAndRelay(w.ctx, w.outbox[senderIdx])
	if err != nil {
		return err
	}
	w.earlyRelay[senderIdx] = relayCand
	route := w.controlledRoute(targetIdx)
	for range 2 {
		if err := target.Receive(w.ctx, route, signal(relayCand)); err != nil {
			return err
		}
	}
	return target.Receive(w.ctx, route, signal(desc))
}

func m1CollectDescAndRelay(ctx context.Context, ch <-chan proto.Envelope) (desc, relayCand proto.Envelope, err error) {
	for desc.Payload == "" || relayCand.Payload == "" {
		env, readErr := m1ReadSignalEnv(ctx, ch)
		if readErr != nil {
			return desc, relayCand, readErr
		}
		desc, relayCand = m1ClassifySignalEnv(env, desc, relayCand)
	}
	return desc, relayCand, nil
}

func m1ReadSignalEnv(ctx context.Context, ch <-chan proto.Envelope) (proto.Envelope, error) {
	select {
	case <-ctx.Done():
		return proto.Envelope{}, ctx.Err()
	case env := <-ch:
		return env, nil
	}
}

func m1ClassifySignalEnv(env, desc, relayCand proto.Envelope) (proto.Envelope, proto.Envelope) {
	if strings.Contains(env.Payload, `"sdp":"`) {
		return env, relayCand
	}
	if strings.Contains(env.Payload, " typ relay") {
		return desc, env
	}
	return desc, relayCand
}

func (w *m1World) waitControlledOpen(b *m1Browser) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-b.conn.Done():
		return errors.New("controlled connection closed before ready")
	case <-b.conn.Ready():
	}
	pushes := make(chan json.RawMessage, 64) // single reader: m1Browser.nextPush
	b.pushes = pushes
	go m1PumpPushes(w.ctx, b.conn, pushes)
	if _, err := w.acceptInbound(); err != nil {
		return err
	}
	return w.sendHello(b.conn)
}

func (w *m1World) deliverLateDuplicateCandidates() error {
	if err := w.nodePeer.Receive(w.ctx, w.controlledRoute(1), signal(w.earlyRelay[0])); err != nil {
		return err
	}
	return w.browsers[0].peer.Receive(w.ctx, w.controlledRoute(0), signal(w.earlyRelay[1]))
}

func (w *m1World) setupTwoConnectedBrowsers() error {
	if err := w.setupInProcessOwned(true, 1, 2); err != nil {
		return err
	}
	for _, b := range w.browsers {
		if err := w.connectAndHelloBrowser(b); err != nil {
			return err
		}
	}
	return nil
}

func (w *m1World) connectAndHelloBrowser(b *m1Browser) error {
	conn, pushes, err := w.dialPeerCtl(b.peer, w.nodeKey.ID())
	if err != nil {
		return err
	}
	b.conn, b.pushes = conn, pushes
	if _, err := w.acceptInbound(); err != nil {
		return err
	}
	if err := w.sendHello(conn); err != nil {
		return err
	}
	return m1DrainInitialDevlog(w.ctx, b)
}

func m1DrainInitialDevlog(ctx context.Context, b *m1Browser) error {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	raw, err := b.nextPush(readCtx)
	if err != nil {
		return err
	}
	var f struct {
		Type string `json:"t"`
	}
	if proto.Decode(raw, &f) != nil || f.Type != "devlog" {
		return fmt.Errorf("expected initial devlog push, got %s", raw)
	}
	return nil
}

func (w *m1World) createGroupFromFirstBrowser(name string) error {
	b := w.browsers[0]
	gid := exampleID("m1-group-" + name)
	w.createdGrp = gid
	payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":%q,"kind":"project","repo":"/tmp/website","base":"main","forge":"none","checks":[],"member_name":"Owner"}`,
		gid, w.nowTime().UnixMilli(), gid, name)
	env := b.key.Sign(payload)
	raw, err := b.conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	var ok proto.Success
	return proto.Decode(raw, &ok)
}

func (w *m1World) checkSecondBrowserGroupsPush(wantName string) error {
	readCtx, cancel := context.WithTimeout(w.ctx, time.Second)
	defer cancel()
	for {
		raw, err := w.browsers[1].nextPush(readCtx)
		if err != nil {
			return fmt.Errorf("waiting for groups push within 1s: %w", err)
		}
		var push proto.Groups
		if proto.Decode(raw, &push) == nil && push.Type == "groups" {
			return m1VerifyGroupListed(push.Groups, w.createdGrp, wantName)
		}
	}
}

func m1VerifyGroupListed(groups []proto.GroupSummary, wantID proto.ID, wantName string) error {
	for _, g := range groups {
		if g.Group == string(wantID) && g.Name == wantName {
			return nil
		}
	}
	return fmt.Errorf("groups push %v missing %s (%s)", groups, wantName, wantID)
}

func (w *m1World) syncSecondAndPostFirst(_, text string) error {
	b1, b2 := w.browsers[0], w.browsers[1]
	buildSync := func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.createdGrp), Since: 0, StateOnly: false}
	}
	rawSync, err := b2.conn.Request(w.ctx, buildSync)
	if err != nil {
		return err
	}
	var synced proto.Synced
	if err := proto.Decode(rawSync, &synced); err != nil {
		return err
	}
	msgID := exampleID("m1-post-1")
	payload := fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"text","text":%q,"thread":null,"main":true,"mentions":[],"re":null,"body":{}}`,
		msgID, w.nowTime().UnixMilli(), w.createdGrp, text)
	env := b1.key.Sign(payload)
	rawCmd, err := b1.conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	var ok proto.Success
	return proto.Decode(rawCmd, &ok)
}

func (w *m1World) checkSecondBrowserMessageRow() error {
	readCtx, cancel := context.WithTimeout(w.ctx, time.Second)
	defer cancel()
	wantID := string(exampleID("m1-post-1"))
	for {
		raw, err := w.browsers[1].nextPush(readCtx)
		if err != nil {
			return fmt.Errorf("waiting for message row within 1s: %w", err)
		}
		if m1HasMessageRow(raw, wantID) {
			return nil
		}
	}
}

func m1HasMessageRow(raw json.RawMessage, wantID string) bool {
	var rows proto.Rows
	if proto.Decode(raw, &rows) != nil || rows.Type != "rows" {
		return false
	}
	return slices.ContainsFunc(rows.Rows, func(r json.RawMessage) bool {
		return m1IsMessageID(r, wantID)
	})
}

func m1IsMessageID(raw json.RawMessage, wantID string) bool {
	var h proto.RowHeader
	if json.Unmarshal(raw, &h) != nil {
		return false
	}
	return h.Type == "msg" && h.ID == wantID
}

func m1ExpectRequestCode(ctx context.Context, conn *peer.Connection, typ, wantCode string) error {
	raw, err := conn.Request(ctx, func(id int64) any { return proto.Request{Type: typ, ID: id} })
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func m1ExpectCmdCode(ctx context.Context, conn *peer.Connection, env proto.Envelope, wantCode string) error {
	raw, err := conn.Request(ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func m1SignDeviceAdd(signer identity.Key, user string, ver int, dev, kind, name string, ts int64) proto.Envelope {
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: exampleID(fmt.Sprintf("m1-%s-%d", dev, ver)), Timestamp: ts},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		panic(err)
	}
	return signer.Sign(string(raw))
}
