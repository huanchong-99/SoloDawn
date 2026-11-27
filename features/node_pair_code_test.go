//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["node_pair_code"] = nodeCodeSteps
}

// codePerson is a person who types an invitation code in their own browser.
type codePerson struct {
	key     identity.Key
	browser *peerBrowser
	conn    *peerBrowserConn
	card    proto.Pair         // the pair message of the room's host
	welcome proto.Welcome      // of the connection to that host
	typed   chan codeTypedRoom // single reader: joinsAfterWaiting
}

type codeTypedRoom struct {
	card proto.Pair
	err  error
}

// codeProxy stands before a rendezvous for the home node: it passes the
// frames both ways, changed on the way to the node, and keeps them.
type codeProxy struct {
	url       string
	mu        sync.Mutex
	sent, got []string // what the node sent, and what it was sent
}

// nodeCodeWorld is a home node with one group, its owner's browser and people
// who type the group's invitation code (planning/steps/step-2-3-acceptance.md,
// A9.47 to A9.51 and A9.55, the node's half).
type nodeCodeWorld struct {
	f       fixtures
	cleanup func(func())
	ctx     context.Context
	rooms   *nodePairWorld // the rendezvous that the people use, and the relay of every connection

	mu     sync.Mutex
	now    time.Time
	timers []chan time.Time
	seq    int

	nodeURL      string // where the home node dials its rendezvous
	dir          string
	nodeKey      identity.Key
	pn           *peerNode
	owner        identity.Key
	ownerBrowser *peerBrowser
	ownerConn    *peerBrowserConn
	gid          proto.ID
	secret       identity.Secret // the open code
	ended        identity.Secret // the code that was open before
	expires      int64
	people       map[string]*codePerson
	last         []*codePerson
	proxy        *codeProxy
	second       string // a second rendezvous
	secondPath   *peer.Path
	holder       *peer.Path
	links        []identity.Secret
}

func nodeCodeSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	w := &nodeCodeWorld{
		f: f, cleanup: cleanup, rooms: &nodePairWorld{f: f, cleanup: cleanup},
		now: time.UnixMilli(1763600000000), people: map[string]*codePerson{},
	}
	sc.Step(`^a code home with a group, its owner's browser and an invitation code for (\d+) hours$`, func(hours int) error { return w.givenHome(nil, hours) })
	sc.Step(`^a code home on a rendezvous that never says hosts, with a group, its owner's browser and an invitation code for (\d+) hours$`, func(hours int) error { return w.givenHome(codeNoHosts, hours) })
	sc.Step(`^a code home whose rendezvous gives ICE servers for one second, with a group and its owner's browser$`, func() error { return w.givenHome(codeShortICE, 0) })
	sc.Step(`^"([^"]*)" and "([^"]*)" type the code and join (one after the other|at once)$`, w.twoJoin)
	sc.Step(`^the room showed each of them the home node, and its welcome named the group$`, w.checkCardsAndWelcomes)
	sc.Step(`^each of them has a member row with role "([^"]*)" and their device log on the code home, let in by the owner$`, w.checkLetIn)
	sc.Step(`^"([^"]*)" has joined by the code and "([^"]*)" has typed it without joining$`, w.oneJoinedOneWaiting)
	sc.Step(`^"([^"]*)" and "([^"]*)" have joined by the code$`, func(a, b string) error { return w.twoJoin(a, b, "one after the other") })
	sc.Step(`^"([^"]*)" types the code and joins$`, func(name string) error { return w.typesAndJoins(name, w.rooms.rvURL) })
	sc.Step(`^"([^"]*)" types the code on the second rendezvous and joins$`, func(name string) error { return w.typesAndJoins(name, w.second) })
	sc.Step(`^the code home's owner turns the code off$`, w.ownerTurnsOff)
	sc.Step(`^the code home's owner makes a new code$`, func() error { return w.ownerMakesCode(1) })
	sc.Step(`^the code home's owner makes an invitation code for (\d+) hours$`, w.ownerMakesCode)
	sc.Step(`^the code home's clock reaches the end of the code$`, w.clockReachesEnd)
	sc.Step(`^a join on the connection of "([^"]*)" is refused with "([^"]*)"$`, w.joinRefused)
	sc.Step(`^the code home's timers run out$`, w.timersRunOut)
	sc.Step(`^a device that types the ended code gets no answer$`, func() error { return w.awaitHost(w.rooms.rvURL, w.ended, "") })
	sc.Step(`^a device that types the code gets no answer$`, func() error { return w.awaitHost(w.rooms.rvURL, w.secret, "") })
	sc.Step(`^a device that types the code is shown the home node$`, func() error { return w.awaitHost(w.rooms.rvURL, w.secret, w.nodeKey.ID()) })
	sc.Step(`^a device that types the code on the second rendezvous is shown the holder$`, func() error { return w.awaitHost(w.second, w.secret, exampleKey("code-holder").ID()) })
	sc.Step(`^the connection of "([^"]*)" to the code home is closed$`, w.connectionClosed)
	sc.Step(`^"([^"]*)" is still a member and still connected$`, w.stillMember)
	sc.Step(`^the code home restarts$`, w.restart)
	sc.Step(`^the code home's rendezvous connection is cut and made again$`, func() error { w.cut(); return w.reconnect() })
	sc.Step(`^the code home's rendezvous connection is cut and its owner makes a new code$`, func() error { w.cut(); return w.ownerMakesCode(1) })
	sc.Step(`^the code home connects to the rendezvous again$`, w.reconnect)
	sc.Step(`^the group of the code home moves to another computer of its owner$`, w.groupMoves)
	sc.Step(`^"([^"]*)" leaves the group of the code home$`, w.personLeaves)
	sc.Step(`^the code home's owner removes "([^"]*)" from the group$`, w.ownerRemoves)
	sc.Step(`^the code home stops$`, w.stop)
	sc.Step(`^"([^"]*)" types the code and waits, and nothing answers a wrong code$`, w.typesAndWaits)
	sc.Step(`^the code home starts again$`, w.boot)
	sc.Step(`^"([^"]*)" is shown the home node and joins without typing again$`, w.joinsAfterWaiting)
	sc.Step(`^nothing answers a wrong code$`, w.wrongCodeUnanswered)
	sc.Step(`^the code home's owner sends pair\.browser and invite$`, w.ownerMakesLinks)
	sc.Step(`^a browser that opens each of those links meets the home node in its room and connects to it$`, w.linksWork)
	sc.Step(`^that rendezvous was sent no join with data and no leave, on the one connection the code home made$`, w.checkOlderRendezvous)
	sc.Step(`^a holder of the code hosts its room on a second rendezvous$`, w.holderHosts)
	sc.Step(`^the code home connects to the second rendezvous$`, w.connectSecond)
	sc.Step(`^the code home is answered full there and its connection stays$`, w.checkFull)
	sc.Step(`^the holder leaves and the code home's connection to the second rendezvous is made again$`, w.holderLeaves)
	sc.Step(`^the code home has asked for fresh ICE servers and taken them$`, w.tookFreshICE)
	sc.Step(`^the code home's rendezvous was sent the hosting join of the code's room$`, w.checkHostingJoin)
	sc.Step(`^the code home's rendezvous was sent the leave of the ended code's room$`, w.checkLeave)
}

func (w *nodeCodeWorld) nowTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

// after holds the node's timers until timersRunOut.
func (w *nodeCodeWorld) after(time.Duration) <-chan time.Time {
	run := make(chan time.Time, 1)
	w.mu.Lock()
	w.timers = append(w.timers, run)
	w.mu.Unlock()
	return run
}

func (w *nodeCodeWorld) timersRunOut() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, run := range w.timers {
		run <- w.now
	}
	w.timers = nil
	return nil
}

func (w *nodeCodeWorld) header(kind string) proto.HomeCommand {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	return proto.HomeCommand{Type: kind, Nonce: exampleID(fmt.Sprint("code-home-", w.seq)), Timestamp: w.now.UnixMilli(), Group: w.gid}
}

// codeNoHosts makes a rendezvous of this version look like an older one.
func codeNoHosts(frame []byte) []byte {
	return bytes.Replace(frame, []byte(`,"hosts":true`), nil, 1)
}

// codeShortICE gives the ICE servers of ready one second, so that the node
// asks for fresh ones after half of it, and makes the fresh ones known.
func codeShortICE(frame []byte) []byte {
	if bytes.Contains(frame, []byte(`"t":"ready"`)) {
		return bytes.Replace(frame, []byte(`"ttl":3600`), []byte(`"ttl":1`), 1)
	}
	if bytes.Contains(frame, []byte(`"t":"ice"`)) {
		return bytes.Replace(frame, []byte(`"ice":[]`), []byte(`"ice":[{"urls":["stun:127.0.0.1:9"]}]`), 1)
	}
	return frame
}

func startCodeProxy(cleanup func(func()), back string, fromServer func([]byte) []byte) *codeProxy {
	x := &codeProxy{}
	keep := func(list *[]string, change func([]byte) []byte) func([]byte) []byte {
		return func(frame []byte) []byte {
			if change != nil {
				frame = change(frame)
			}
			x.mu.Lock()
			*list = append(*list, string(frame))
			x.mu.Unlock()
			return frame
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		device, err := websocket.Accept(rw, r, nil)
		if err != nil {
			return
		}
		defer device.CloseNow()
		server, _, err := websocket.Dial(r.Context(), back, nil)
		if err != nil {
			return
		}
		defer server.CloseNow()
		ctx, cancel := context.WithCancel(r.Context())
		go pumpAcceptanceWS(ctx, cancel, device, server, keep(&x.sent, nil))
		pumpAcceptanceWS(ctx, cancel, server, device, keep(&x.got, fromServer))
	}))
	cleanup(srv.Close)
	x.url = "ws" + strings.TrimPrefix(srv.URL, "http")
	return x
}

// frames returns the kept frames of a list that hold part.
func (x *codeProxy) frames(list *[]string, part string) []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	var out []string
	for _, frame := range *list {
		if strings.Contains(frame, part) {
			out = append(out, frame)
		}
	}
	return out
}

// givenHome starts the home node, on a proxy when its rendezvous is to be
// changed or watched, connects its owner's browser, creates the group and,
// when hours is not 0, makes a code.
func (w *nodeCodeWorld) givenHome(change func([]byte) []byte, hours int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := w.rooms.prepareRendezvous(""); err != nil {
		return err
	}
	w.nodeURL = w.rooms.rvURL
	if change != nil {
		w.proxy = startCodeProxy(w.cleanup, w.rooms.rvURL, change)
		w.nodeURL = w.proxy.url
	}
	dir, err := shortTempRoot(w.f.t, w.cleanup, "sdcode")
	if err != nil {
		return err
	}
	w.dir, w.nodeKey, w.owner, w.gid = dir, exampleKey("code-home"), exampleKey("code-owner"), exampleID("code-home-group")
	if err := errors.Join(w.boot(), w.connectOwner()); err != nil {
		return err
	}
	return w.createGroup(hours)
}

func (w *nodeCodeWorld) createGroup(hours int) error {
	create := proto.GroupCreate{GroupCommand: proto.GroupCommand{HomeCommand: w.header("group"), Op: "create"}, Name: "Team", MemberName: "P"}
	create.Nonce = w.gid
	if _, err := w.ownerCmd(create); err != nil || hours == 0 {
		return err
	}
	return w.ownerMakesCode(hours)
}

func (w *nodeCodeWorld) boot() error {
	pn, err := startPeerNode(w.f.t, w.cleanup, peerNodeConfig{
		Ctx: w.ctx, Dir: w.dir, Key: w.nodeKey, Name: "desk", Owner: w.owner, Rendezvous: w.nodeURL,
		Now: w.nowTime, MapRoute: w.rooms.mapRoute, After: w.after,
	})
	w.pn = pn
	return err
}

func (w *nodeCodeWorld) stop() error {
	w.pn.Close()
	return codeAwaitOffline(w.ctx, w.rooms.rvURL, w.nodeKey.ID())
}

func (w *nodeCodeWorld) restart() error {
	if err := w.stop(); err != nil {
		return err
	}
	return errors.Join(w.boot(), w.connectOwner())
}

// codeAwaitOffline returns once the rendezvous says that a device is not
// connected: its rooms have ended by then.
func codeAwaitOffline(ctx context.Context, url, device string) error {
	path, err := peer.Dial(ctx, url, newRandomPeerKey())
	if err != nil {
		return err
	}
	defer path.Close()
	off := make(chan struct{}, 1) // single reader: codeAwaitOffline
	go path.Serve(ctx, nil, func(kind string, data []byte) error {
		var p proto.Presence
		if kind == "presence" && proto.Decode(data, &p) == nil && !p.On {
			off <- struct{}{}
		}
		return nil
	})
	if err := path.Write(ctx, proto.Watch{Type: "watch", Keys: []string{device}}); err != nil {
		return err
	}
	_, err = recvCtx(ctx, off, "the rendezvous to say that the home node is not connected", nil)
	return err
}

func (w *nodeCodeWorld) connectOwner() error {
	if w.ownerBrowser == nil {
		b, err := newPeerBrowser(w.ctx, w.cleanup, w.owner, w.rooms.rvURL, w.rooms.mapRoute)
		if err != nil {
			return err
		}
		w.ownerBrowser = b
	}
	bc, err := w.ownerBrowser.openCtl(w.ctx, w.cleanup, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.ownerConn = bc
	return checkOwnerWelcome(w.ctx, bc.conn, w.nodeKey.ID())
}

func (w *nodeCodeWorld) ownerCmd(payload any) (proto.CmdResult, error) {
	return codeCmd(w.ctx, w.ownerConn.conn, w.owner, payload)
}

// codeCmd sends a command and returns its ok answer.
func codeCmd(ctx context.Context, conn *peer.Connection, signer identity.Key, payload any) (proto.CmdResult, error) {
	var res proto.CmdResult
	raw, err := sendSignedCmdRaw(ctx, conn, signer, payload)
	if err != nil {
		return res, err
	}
	if err := proto.Decode(raw, &res); err != nil {
		return res, fmt.Errorf("the answer %s is not ok: %w", raw, err)
	}
	return res, nil
}

// ownerMakesCode sends code new and keeps the code; the one that was open
// has ended.
func (w *nodeCodeWorld) ownerMakesCode(hours int) error {
	res, err := w.ownerCmd(proto.Code{HomeCommand: w.header("code"), Op: "new", Hours: &hours})
	if err != nil {
		return err
	}
	secret, err := identity.ParseSecret(res.Code)
	if want := w.nowTime().Add(time.Duration(hours) * time.Hour).UnixMilli(); err != nil || res.Code != secret.String() || res.Expires != want {
		return fmt.Errorf("code new answered %+v (%v), want a code in four groups that ends at %d", res, err, want)
	}
	w.ended, w.secret, w.expires = w.secret, secret, res.Expires
	return nil
}

func (w *nodeCodeWorld) ownerTurnsOff() error {
	_, err := w.ownerCmd(proto.Code{HomeCommand: w.header("code"), Op: "off"})
	w.ended, w.secret = w.secret, ""
	return err
}

func (w *nodeCodeWorld) clockReachesEnd() error {
	w.mu.Lock()
	w.now = time.UnixMilli(w.expires)
	w.mu.Unlock()
	w.ended, w.secret = w.secret, ""
	return nil
}

func (w *nodeCodeWorld) person(name string) *codePerson {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.people[name] == nil {
		w.people[name] = &codePerson{key: exampleKey("code-person-" + name)}
	}
	return w.people[name]
}

// types opens the person's browser on a rendezvous and joins the room of the
// code, which answers with the pair message of its host.
func (w *nodeCodeWorld) types(p *codePerson, rv string, secret identity.Secret) error {
	b, err := newPeerBrowser(w.ctx, w.cleanup, p.key, rv, w.rooms.mapRoute)
	if err != nil {
		return err
	}
	p.browser = b
	p.card, err = b.exchangeRoom(w.ctx, secret, "b", "a")
	return err
}

// connects opens the connection to the host that the room showed.
func (w *nodeCodeWorld) connects(p *codePerson) error {
	bc, err := p.browser.openCtl(w.ctx, w.cleanup, p.card.Key)
	if err != nil {
		return err
	}
	p.conn = bc
	p.welcome, err = requestBrowserHello(w.ctx, bc.conn)
	return err
}

// sendsJoin presents the person's own device log and sends join for the group
// that the welcome named.
func (w *nodeCodeWorld) sendsJoin(p *codePerson, name string) (json.RawMessage, error) {
	entry := nodetest.SignDeviceAdd(p.key, p.key.ID(), 1, "browser", name, p.key.ID(), w.nowTime().UnixMilli())
	if err := p.conn.conn.Send(proto.Devlog{Type: "devlog", User: p.key.ID(), Entries: []proto.Envelope{entry}}); err != nil {
		return nil, err
	}
	join := w.header("join")
	join.Group = proto.ID(p.welcome.Join)
	return sendSignedCmdRaw(w.ctx, p.conn.conn, p.key, proto.Join{HomeCommand: join, Name: name})
}

func (w *nodeCodeWorld) joins(p *codePerson, name string) error {
	if err := w.connects(p); err != nil {
		return err
	}
	raw, err := w.sendsJoin(p, name)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("join of %s answered %s: %w", name, raw, err)
	}
	return nil
}

func (w *nodeCodeWorld) typesAndJoins(name, rv string) error {
	p := w.person(name)
	w.last = []*codePerson{p}
	if err := w.types(p, rv, w.secret); err != nil {
		return err
	}
	return w.joins(p, name)
}

func (w *nodeCodeWorld) twoJoin(a, b, how string) error {
	if how != "at once" {
		err := errors.Join(w.typesAndJoins(a, w.rooms.rvURL), w.typesAndJoins(b, w.rooms.rvURL))
		w.last = []*codePerson{w.person(a), w.person(b)}
		return err
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, name := range []string{a, b} {
		wg.Go(func() {
			p := w.person(name)
			errs[i] = w.types(p, w.rooms.rvURL, w.secret)
			if errs[i] == nil {
				errs[i] = w.joins(p, name)
			}
		})
	}
	wg.Wait()
	w.last = []*codePerson{w.person(a), w.person(b)}
	return errors.Join(errs...)
}

func (w *nodeCodeWorld) oneJoinedOneWaiting(joined, waiting string) error {
	if err := w.typesAndJoins(joined, w.rooms.rvURL); err != nil {
		return err
	}
	p := w.person(waiting)
	if err := w.types(p, w.rooms.rvURL, w.secret); err != nil {
		return err
	}
	return w.connects(p)
}

func (w *nodeCodeWorld) checkCardsAndWelcomes() error {
	want := proto.Pair{Type: "pair", Key: w.nodeKey.ID(), Kind: "node", Name: "desk", Role: "a", Expires: w.expires}
	want.MAC = w.secret.MAC(want)
	for _, p := range w.last {
		if p.card != want {
			return fmt.Errorf("the room showed %+v, want %+v", p.card, want)
		}
		if p.welcome.Join != string(w.gid) || p.welcome.Owner || len(p.welcome.Groups) != 0 {
			return fmt.Errorf("welcome = %+v, want join %s, owner false and no group", p.welcome, w.gid)
		}
	}
	return nil
}

// member reads a person's member row on the home: its id, its role, who the
// member_joined message names, and how many entries of their log are stored.
func (w *nodeCodeWorld) member(user identity.Key) (id, role, by string, logged int, err error) {
	err = w.pn.Store.Write(func(tx *store.Tx) error {
		const row = "SELECT data->>'$.id', data->>'$.role' FROM row WHERE g=? AND r='member' AND data->>'$.user'=? AND data->>'$.gone' IS NULL"
		const joined = "SELECT coalesce((SELECT data->>'$.body.by' FROM row WHERE g=? AND r='msg' AND data->>'$.body.member'=?), '')"
		if err := tx.QueryRow(row, string(w.gid), user.ID()).Scan(&id, &role); err != nil {
			return err
		}
		if err := tx.QueryRow(joined, string(w.gid), id).Scan(&by); err != nil {
			return err
		}
		return tx.QueryRow("SELECT count(*) FROM device_log WHERE user=?", user.ID()).Scan(&logged)
	})
	return id, role, by, logged, err
}

func (w *nodeCodeWorld) checkLetIn(wantRole string) error {
	owner, _, _, _, err := w.member(w.owner)
	if err != nil {
		return err
	}
	want := fmt.Sprint(wantRole, owner, 1, error(nil))
	for _, p := range w.last {
		_, role, by, logged, err := w.member(p.key)
		if got := fmt.Sprint(role, by, logged, err); got != want {
			return fmt.Errorf("member of %s: role, who let them in, log entries and error %s, want %s", p.key.ID(), got, want)
		}
	}
	return nil
}

func (w *nodeCodeWorld) joinRefused(name, wantCode string) error {
	raw, err := w.sendsJoin(w.person(name), name)
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

// codeAnswer joins the room of a code on a rendezvous as a new device and
// returns the pair message of the room's host. ok is false when the
// rendezvous answered a later request first: nothing hosts the room.
func codeAnswer(ctx context.Context, url string, secret identity.Secret) (card proto.Pair, ok bool, err error) {
	path, err := peer.Dial(ctx, url, newRandomPeerKey())
	if err != nil {
		return card, false, err
	}
	defer path.Close()
	got := make(chan *proto.Pair, 2) // single reader: codeAnswer
	go path.Serve(ctx, nil, func(kind string, data []byte) error {
		if m, ok := matchPeerRoomMsg(data, secret, secret.Room(), "a"); ok {
			got <- &m
		}
		if kind == "quota" {
			got <- nil
		}
		return nil
	})
	err = errors.Join(path.Write(ctx, proto.RoomID{Type: "join", Room: secret.Room()}), path.Write(ctx, proto.RVFrame{Type: "quota"}))
	if err != nil {
		return card, false, err
	}
	m, err := recvCtx(ctx, got, "the rendezvous to answer in the room of a code", nil)
	if err != nil || m == nil {
		return card, false, err
	}
	return *m, true, nil
}

// awaitHost waits until the room of a code on a rendezvous is hosted by a
// device, or by nobody when host is empty.
func (w *nodeCodeWorld) awaitHost(url string, secret identity.Secret, host string) error {
	return awaitCodeHost(w.ctx, url, secret, host)
}

func (w *nodeCodeWorld) connectionClosed(name string) error {
	select {
	case <-w.person(name).conn.conn.Done():
		return nil
	case <-time.After(10 * time.Second):
		return new(trail).timedOut(10*time.Second, "the connection of "+name+" to be closed")
	}
}

func (w *nodeCodeWorld) stillMember(name string) error {
	p := w.person(name)
	_, role, _, _, err := w.member(p.key)
	if err != nil || role != "member" {
		return fmt.Errorf("%s has role %q on the home (%v)", name, role, err)
	}
	return syncS2Group(w.ctx, p.conn.conn, w.gid)
}

func (w *nodeCodeWorld) cut() {
	for _, path := range w.pn.livePaths() {
		w.pn.Cut(path)
	}
}

func (w *nodeCodeWorld) reconnect() error {
	_, err := w.pn.Connect(w.nodeURL)
	return err
}

func (w *nodeCodeWorld) groupMoves() error {
	w.ended, w.secret = w.secret, ""
	return w.pn.Home.ExportGroup(string(w.gid), filepath.Join(w.dir, "moved.group"), exampleKey("code-other-home").ID())
}

func (w *nodeCodeWorld) remove(conn *peer.Connection, signer, person identity.Key) error {
	id, _, _, _, err := w.member(person)
	if err != nil {
		return err
	}
	_, err = codeCmd(w.ctx, conn, signer, proto.MemberCommand{HomeCommand: w.header("member"), Op: "remove", ID: proto.ID(id)})
	return err
}

func (w *nodeCodeWorld) personLeaves(name string) error {
	p := w.person(name)
	return w.remove(p.conn.conn, p.key, p.key)
}

func (w *nodeCodeWorld) ownerRemoves(name string) error {
	w.ended, w.secret = w.secret, ""
	return w.remove(w.ownerConn.conn, w.owner, w.person(name).key)
}

func (w *nodeCodeWorld) wrongCodeUnanswered() error {
	if card, ok, err := codeAnswer(w.ctx, w.rooms.rvURL, identity.NewSecret()); err != nil || ok {
		return fmt.Errorf("a wrong code was answered by %q (%v)", card.Key, err)
	}
	return nil
}

func (w *nodeCodeWorld) typesAndWaits(name string) error {
	p := w.person(name)
	b, err := newPeerBrowser(w.ctx, w.cleanup, p.key, w.rooms.rvURL, w.rooms.mapRoute)
	if err != nil {
		return err
	}
	p.browser, p.typed = b, make(chan codeTypedRoom, 1)
	go func() {
		card, err := b.exchangeRoom(w.ctx, w.secret, "b", "a")
		p.typed <- codeTypedRoom{card, err}
	}()
	if err := w.wrongCodeUnanswered(); err != nil {
		return err
	}
	if len(p.typed) != 0 {
		return errors.New("the code was answered while its home was off")
	}
	return nil
}

func (w *nodeCodeWorld) joinsAfterWaiting(name string) error {
	p := w.person(name)
	typed, err := recvCtx(w.ctx, p.typed, "the room to answer the person who waited", nil)
	if err = errors.Join(err, typed.err); err != nil {
		return err
	}
	p.card, w.last = typed.card, []*codePerson{p}
	if err := w.joins(p, name); err != nil {
		return err
	}
	return errors.Join(w.checkCardsAndWelcomes(), w.checkLetIn("member"))
}

// ownerMakesLinks sends pair.browser and invite and keeps both secrets.
func (w *nodeCodeWorld) ownerMakesLinks() error {
	pairing := proto.HostCommand{Type: "pair.browser", Nonce: exampleID("code-home-pair"), Timestamp: w.nowTime().UnixMilli(), To: w.nodeKey.ID()}
	paired, err := w.ownerCmd(pairing)
	if err != nil {
		return err
	}
	invited, err := w.ownerCmd(w.header("invite"))
	if err != nil {
		return err
	}
	first, err := parsePairSecretLink(paired.Link)
	second, joinErr := parseS2JoinLink(invited.Link, invited.Expires)
	w.links = []identity.Secret{first, second.Secret}
	return errors.Join(err, joinErr)
}

func (w *nodeCodeWorld) linksWork() error {
	for i, secret := range w.links {
		p := w.person(fmt.Sprint("link-", i))
		if err := w.types(p, w.rooms.rvURL, secret); err != nil {
			return err
		}
		if err := w.connects(p); err != nil || p.card.Key != w.nodeKey.ID() {
			return fmt.Errorf("link %d: the room showed %q (%v), want the home node", i, p.card.Key, err)
		}
	}
	return nil
}

func (w *nodeCodeWorld) checkOlderRendezvous() error {
	sent := func(part string) int { return len(w.proxy.frames(&w.proxy.sent, part)) }
	got := []int{sent(`"t":"auth"`), sent(`{"t":"join","room":"`), sent(`"data":{"t":"pair"`), sent(`"t":"leave"`), len(w.proxy.frames(&w.proxy.got, "hosts"))}
	if want := []int{1, 2, 2, 0, 0}; !slices.Equal(got, want) {
		return fmt.Errorf("connections, joins, frames with a pair message, leaves and hosts of the older rendezvous: %v, want %v", got, want)
	}
	if hosted := w.proxy.frames(&w.proxy.sent, `{"t":"join","room":"`+w.ended.Room()); len(hosted) != 0 {
		return fmt.Errorf("the older rendezvous was sent %v", hosted)
	}
	return nil
}

func (w *nodeCodeWorld) holderHosts() error {
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(srv.Close)
	w.second = "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
	w.proxy = startCodeProxy(w.cleanup, w.second, nil)
	holder := exampleKey("code-holder")
	path, err := peer.Dial(w.ctx, w.second, holder)
	if err != nil {
		return err
	}
	w.cleanup(func() { path.Close() })
	w.holder = path
	card := proto.Pair{Type: "pair", Key: holder.ID(), Kind: "node", Name: "holder", Role: "a"}
	card.MAC = w.secret.MAC(card)
	hosting := proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: w.secret.Room()}, Data: mustMarshalNoEscape(card)}
	if err := path.Write(w.ctx, hosting); err != nil {
		return err
	}
	return w.awaitHost(w.second, w.secret, holder.ID())
}

func (w *nodeCodeWorld) connectSecond() error {
	path, err := w.pn.Connect(w.proxy.url)
	w.secondPath = path
	return err
}

func (w *nodeCodeWorld) checkFull() error {
	full := `{"t":"full","room":"` + w.secret.Room() + `"}`
	if err := w.secondPath.Write(w.ctx, proto.RVFrame{Type: "quota"}); err != nil {
		return err
	}
	return waitUntil(10*time.Second, "the code home to be answered full and then quota on the second rendezvous", func() error {
		got := w.proxy.frames(&w.proxy.got, `"t":"`)
		if n := len(got); n < 4 || got[n-2] != full || !strings.Contains(got[n-1], `"t":"quota"`) {
			return fmt.Errorf("got %v", got)
		}
		return nil
	})
}

func (w *nodeCodeWorld) holderLeaves() error {
	w.holder.Close()
	if err := w.awaitHost(w.second, w.secret, ""); err != nil {
		return err
	}
	w.pn.Cut(w.secondPath)
	return w.connectSecond()
}

func (w *nodeCodeWorld) tookFreshICE() error {
	return waitUntil(10*time.Second, "the code home to take fresh ICE servers", func() error {
		if asked, fresh := len(w.proxy.frames(&w.proxy.sent, `{"t":"ice"}`)), len(w.pn.Path.Route().ICE); asked == 0 || fresh == 0 {
			return fmt.Errorf("asked %d times, %d fresh servers", asked, fresh)
		}
		return nil
	})
}

func (w *nodeCodeWorld) checkHostingJoin() error {
	want := `{"t":"join","room":"` + w.secret.Room() + `","data":{"t":"pair","k":"` + w.nodeKey.ID() + `","kind":"node",`
	if got := w.proxy.frames(&w.proxy.sent, `"data":{"t":"pair"`); len(got) != 1 || !strings.HasPrefix(got[0], want) {
		return fmt.Errorf("hosting joins sent: %v, want one that starts %s", got, want)
	}
	return nil
}

func (w *nodeCodeWorld) checkLeave() error {
	want := `{"t":"leave","room":"` + w.ended.Room() + `"}`
	if got := w.proxy.frames(&w.proxy.sent, `"t":"leave"`); len(got) != 1 || got[0] != want {
		return fmt.Errorf("leaves sent: %v, want %s", got, want)
	}
	return nil
}
