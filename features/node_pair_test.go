//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

func init() {
	stepSets["node_pair"] = nodePairSteps
}

type nodePairWorld struct {
	f       fixtures
	ctx     context.Context
	cleanup func(func())

	mu       sync.Mutex
	now      time.Time
	rvURL    string
	ice      []proto.ICEServer
	mapRoute func(peer.Route) peer.Route
	pn       *peerNode

	b1, b2       *peerBrowser
	b1Conn       *peerBrowserConn
	b2Conn       *peerBrowserConn
	otherKey     identity.Key
	secret       identity.Secret
	roomPeer     proto.Pair
	pairDone     chan pairCallOut // single reader: checkPairCompleted
	lastWelcome  proto.Welcome
	linksByNonce map[string]proto.CmdResult
	lastCmdRaw   json.RawMessage
}

type pairCallOut struct {
	res node.PairResult
	err error
}

func nodePairSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &nodePairWorld{
		f:            f,
		cleanup:      afterScenario(sc),
		now:          time.UnixMilli(1763600000000),
		linksByNonce: map[string]proto.CmdResult{},
	}
	sc.Step(`^an in-process rendezvous and an unowned pairing node$`, func() error { return w.setupNode(false, "") })
	sc.Step(`^an unowned pairing node on that rendezvous$`, func(ctx context.Context) error { return w.setupNode(false, sharedRVURL(ctx)) })
	sc.Step(`^a fresh browser creates a pairing secret and joins its room$`, w.browserCreatesSecret)
	sc.Step(`^the pairing node runs pair with that secret and label "([^"]*)"$`, w.nodeRunsPair)
	sc.Step(`^the node fingerprint seen in the room equals the node key fingerprint$`, w.checkRoomFingerprint)
	sc.Step(`^the pairing browser opens ctl and sends hello before any device log$`, w.browserOpensCtlAndSaysHello)
	sc.Step(`^the pairing welcome reports owner (true|false)$`, w.checkWelcomeOwner)
	sc.Step(`^host\.info before enrolment is refused with "([^"]*)"$`, w.checkPreEnrolmentForbidden)
	sc.Step(`^cmd before enrolment is refused with "([^"]*)"$`, w.checkPreEnrolmentCmdForbidden)
	sc.Step(`^the pairing browser sends a log whose add entry for the node was signed by another device$`, w.sendWrongSignerLog)
	sc.Step(`^the pairing node still has no owner and refuses host\.info with "([^"]*)"$`, w.checkUnownedForbidden)
	sc.Step(`^the pairing browser sends a valid device log adding the node signed by the pairing browser$`, w.sendValidAdoptionLog)
	sc.Step(`^pair completes with the node fingerprint and active path$`, w.checkPairCompleted)
	sc.Step(`^the pairing browser sends hello again$`, func() error { return w.sendHello(w.b1Conn.conn) })
	sc.Step(`^host\.info succeeds for the owner browser$`, w.checkOwnerSurface)

	sc.Step(`^an owned node with a connected owner browser over the rendezvous$`, w.setupOwnedWithBrowser)
	sc.Step(`^the owner browser sends pair\.browser with nonce "([^"]*)"$`, w.sendPairBrowser)
	sc.Step(`^the owner browser repeats pair\.browser with nonce "([^"]*)" while the link is valid$`, w.repeatPairBrowserValid)
	sc.Step(`^both answers return ok with the same #pair link and expiry$`, w.checkSamePairLink)
	sc.Step(`^a second browser pairs using that #pair link$`, w.secondBrowserPairs)
	sc.Step(`^the second browser receives the device log, adopts the owner and reads owner true$`, w.checkSecondBrowserAdopted)
	sc.Step(`^the owner browser repeats pair\.browser with nonce "([^"]*)" after the link was used$`, w.sendPairBrowserRaw)
	sc.Step(`^10 minutes pass so the second link expires$`, w.advanceTenMinutes)
	sc.Step(`^the owner browser repeats pair\.browser with nonce "([^"]*)"$`, w.sendPairBrowserRaw)
	sc.Step(`^the pair\.browser repeat is refused with "([^"]*)"$`, w.checkLastRefused)
}

func (w *nodePairWorld) nowTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

func (w *nodePairWorld) setupNode(owned bool, sharedURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := w.prepareRendezvous(sharedURL); err != nil {
		return err
	}
	b1Key := exampleKey("n3-b1")
	w.otherKey = exampleKey("n3-other")
	var owner identity.Key
	if owned {
		owner = b1Key
	}
	pn, err := startPeerNode(w.f.t, w.cleanup, peerNodeConfig{
		Ctx:        w.ctx,
		Key:        exampleKey("n3-node"),
		Name:       "workstation",
		Version:    "v0.3.1",
		Owner:      owner,
		Rendezvous: w.rvURL,
		Now:        w.nowTime,
		MapRoute:   w.mapRoute,
	})
	if err != nil {
		return err
	}
	w.pn = pn
	b1, err := newPeerBrowser(w.ctx, w.cleanup, b1Key, w.rvURL, w.mapRoute)
	w.b1 = b1
	return err
}

func (w *nodePairWorld) prepareRendezvous(sharedURL string) error {
	if sharedURL != "" {
		w.rvURL = sharedURL
		w.mapRoute = func(r peer.Route) peer.Route {
			r.ICE = rewriteDirectICE(r.ICE)
			return r
		}
		return nil
	}
	relay := &peersWorld{}
	err := relay.relayOnly()
	for _, undo := range relay.cleanup {
		w.cleanup(undo)
	}
	if err != nil {
		return err
	}
	w.ice = relay.relay
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(srv.Close)
	w.rvURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
	w.mapRoute = w.withRelay
	return nil
}

func (w *nodePairWorld) withRelay(r peer.Route) peer.Route {
	orig := r.Send
	r.ICE = w.ice
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		if isNonRelayICECandidate(env.Payload) {
			return nil
		}
		return orig(ctx, to, env)
	}
	return r
}

func (w *nodePairWorld) browserCreatesSecret() error {
	w.secret = identity.NewSecret()
	return nil
}

func (w *nodePairWorld) nodeRunsPair(label string) error {
	w.pairDone = make(chan pairCallOut, 1)
	go func() {
		res, err := w.pn.Pairer.Pair(w.ctx, node.PairRequest{Secret: w.secret.String(), Name: label})
		w.pairDone <- pairCallOut{res: res, err: err}
	}()
	peerMsg, err := w.b1.exchangeRoom(w.ctx, w.secret, "a", "b")
	if err != nil {
		return err
	}
	w.roomPeer = peerMsg
	return w.awaitKnown(w.b1.key.ID())
}

func (w *nodePairWorld) awaitKnown(device string) error {
	return waitUntil(3*time.Second, fmt.Sprintf("the node to record pairing peer %q", device), func() error {
		if !w.pn.Pairer.Known(device) {
			return errors.New("peer unknown")
		}
		return nil
	})
}

func (w *nodePairWorld) checkRoomFingerprint() error {
	if w.roomPeer.Key != w.pn.Key.ID() {
		return fmt.Errorf("room peer key = %q, want %q", w.roomPeer.Key, w.pn.Key.ID())
	}
	return nil
}

func (w *nodePairWorld) browserOpensCtlAndSaysHello() error {
	bc, err := w.b1.openCtl(w.ctx, w.cleanup, w.pn.Key.ID())
	if err != nil {
		return err
	}
	w.b1Conn = bc
	return w.sendHello(w.b1Conn.conn)
}

func (w *nodePairWorld) sendHello(conn *peer.Connection) error {
	welc, err := requestBrowserHello(w.ctx, conn)
	if err != nil {
		return err
	}
	w.lastWelcome = welc
	return nil
}

func (w *nodePairWorld) checkWelcomeOwner(want string) error {
	if got := fmt.Sprint(w.lastWelcome.Owner); got != want {
		return fmt.Errorf("welcome.owner = %s, want %s", got, want)
	}
	return nil
}

func (w *nodePairWorld) checkPreEnrolmentForbidden(wantCode string) error {
	rawInfo, err := w.b1Conn.request(w.ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return err
	}
	return checkWireError(rawInfo, wantCode)
}

func (w *nodePairWorld) checkPreEnrolmentCmdForbidden(wantCode string) error {
	rawCmd, err := w.callPairBrowser("pre")
	if err != nil {
		return err
	}
	return checkWireError(rawCmd, wantCode)
}

func (w *nodePairWorld) sendWrongSignerLog() error {
	ts := w.nowTime().UnixMilli()
	e1 := nodetest.SignDeviceAdd(w.b1.key, w.b1.key.ID(), 1, "browser", "b1", w.b1.key.ID(), ts)
	e2 := nodetest.SignDeviceAdd(w.b1.key, w.b1.key.ID(), 2, "browser", "other", w.otherKey.ID(), ts)
	e3 := nodetest.SignDeviceAdd(w.otherKey, w.b1.key.ID(), 3, "node", "workstation", w.pn.Key.ID(), ts)
	return w.b1Conn.conn.Send(proto.Devlog{Type: "devlog", User: w.b1.key.ID(), Entries: []proto.Envelope{e1, e2, e3}})
}

func (w *nodePairWorld) checkUnownedForbidden(wantCode string) error {
	rawInfo, err := w.b1Conn.request(w.ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return err
	}
	if err := checkWireError(rawInfo, wantCode); err != nil {
		return err
	}
	if w.pn.Logs.Owner() != "" {
		return fmt.Errorf("owner = %q, want empty", w.pn.Logs.Owner())
	}
	return nil
}

func (w *nodePairWorld) sendValidAdoptionLog() error {
	ts := w.nowTime().UnixMilli()
	e1 := nodetest.SignDeviceAdd(w.b1.key, w.b1.key.ID(), 1, "browser", "b1", w.b1.key.ID(), ts)
	e2 := nodetest.SignDeviceAdd(w.b1.key, w.b1.key.ID(), 2, "node", "workstation", w.pn.Key.ID(), ts)
	return w.b1Conn.conn.Send(proto.Devlog{Type: "devlog", User: w.b1.key.ID(), Entries: []proto.Envelope{e1, e2}})
}

func (w *nodePairWorld) checkPairCompleted() error {
	out, err := recvCtx(w.ctx, w.pairDone, "the pairing to complete", nil)
	if err != nil {
		return err
	}
	if out.err != nil {
		return out.err
	}
	if out.res.Fingerprint != w.pn.Key.Fingerprint() || out.res.Path == "" {
		return fmt.Errorf("pair result = %+v", out.res)
	}
	return nil
}

func (w *nodePairWorld) checkOwnerSurface() error {
	info, err := requestHostInfo(w.ctx, w.b1Conn.conn)
	if err != nil {
		return err
	}
	wantOS := runtime.GOOS + "/" + runtime.GOARCH
	if info.Device != w.pn.Key.ID() || info.Name != "workstation" || info.OS != wantOS || info.Version != "v0.3.1" {
		return fmt.Errorf("host.info = %+v", info)
	}
	return nil
}

func (w *nodePairWorld) setupOwnedWithBrowser() error {
	if err := w.setupNode(true, ""); err != nil {
		return err
	}
	bc, err := w.b1.openCtl(w.ctx, w.cleanup, w.pn.Key.ID())
	if err != nil {
		return err
	}
	w.b1Conn = bc
	if err := w.sendHello(w.b1Conn.conn); err != nil {
		return err
	}
	_, err = w.b1Conn.nextPush(w.ctx)
	return err
}

func (w *nodePairWorld) sendPairBrowser(nonceTag string) error {
	raw, err := w.callPairBrowser(nonceTag)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("pair.browser %s: %v", raw, err)
	}
	w.linksByNonce[nonceTag] = res
	return nil
}

func (w *nodePairWorld) repeatPairBrowserValid(nonceTag string) error {
	raw, err := w.callPairBrowser(nonceTag)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("pair.browser repeat %s: %v", raw, err)
	}
	w.linksByNonce[nonceTag+":repeat"] = res
	return nil
}

func (w *nodePairWorld) sendPairBrowserRaw(nonceTag string) error {
	raw, err := w.callPairBrowser(nonceTag)
	w.lastCmdRaw = raw
	return err
}

func (w *nodePairWorld) callPairBrowser(nonceTag string) (json.RawMessage, error) {
	ts := time.UnixMilli(1763600000000).UnixMilli()
	env := w.b1.key.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, exampleID("pb-"+nonceTag), ts, w.pn.Key.ID()))
	return w.b1Conn.request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
}

func (w *nodePairWorld) checkSamePairLink() error {
	first := w.linksByNonce["first"]
	second := w.linksByNonce["first:repeat"]
	if first.Link == "" || first.Link != second.Link || first.Expires != second.Expires || first.Re == second.Re {
		return fmt.Errorf("first=%+v repeat=%+v", first, second)
	}
	return nil
}

func (w *nodePairWorld) secondBrowserPairs() error {
	sec, err := parsePairSecretLink(w.linksByNonce["first"].Link)
	if err != nil {
		return err
	}
	b2, err := newPeerBrowser(w.ctx, w.cleanup, exampleKey("n3-b2"), w.rvURL, w.mapRoute)
	if err != nil {
		return err
	}
	w.b2 = b2
	if _, err := w.b2.exchangeRoom(w.ctx, sec, "b", "a"); err != nil {
		return err
	}
	if err := w.awaitKnown(w.b2.key.ID()); err != nil {
		return err
	}
	bc2, err := w.b2.openCtl(w.ctx, w.cleanup, w.pn.Key.ID())
	w.b2Conn = bc2
	return err
}

func parsePairSecretLink(link string) (identity.Secret, error) {
	_, code, ok := strings.Cut(link, "/#pair=")
	if !ok {
		return "", fmt.Errorf("missing #pair= in %q", link)
	}
	return identity.ParseSecret(code)
}

func (w *nodePairWorld) checkSecondBrowserAdopted() error {
	if err := w.sendHello(w.b2Conn.conn); err != nil || w.lastWelcome.Owner {
		return fmt.Errorf("unexpected first welcome owner=%v err=%v", w.lastWelcome.Owner, err)
	}
	if err := w.verifyAdoptedDevlog(w.b2, w.b2Conn); err != nil {
		return err
	}
	if err := w.sendHello(w.b2Conn.conn); err != nil || !w.lastWelcome.Owner {
		return fmt.Errorf("expected second welcome owner=true, got %v (%v)", w.lastWelcome.Owner, err)
	}
	return nil
}

func (w *nodePairWorld) verifyAdoptedDevlog(b *peerBrowser, bc *peerBrowserConn) error {
	rawDL, err := bc.nextPush(w.ctx)
	if err != nil {
		return err
	}
	var dl proto.Devlog
	if err := proto.Decode(rawDL, &dl); err != nil {
		return err
	}
	cand, err := identity.Replay(w.b1.key.ID(), dl.Entries)
	if err != nil || !cand.Adopts(b.key.ID(), w.pn.Key.ID()) {
		return fmt.Errorf("second browser log did not adopt: %v", err)
	}
	return nil
}

func (w *nodePairWorld) advanceTenMinutes() error {
	w.mu.Lock()
	w.now = w.now.Add(10 * time.Minute)
	w.mu.Unlock()
	return nil
}

func (w *nodePairWorld) checkLastRefused(wantCode string) error {
	return checkWireError(w.lastCmdRaw, wantCode)
}
