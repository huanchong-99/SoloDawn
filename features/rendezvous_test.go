//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

// A test rendezvous that can change one byte of the signed description on its
// way out. The frame keeps its length and stays valid JSON.
type rvListener struct {
	net.Listener
	change atomic.Bool
}

func (l *rvListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	return rvConn{c, &l.change}, err
}

type rvConn struct {
	net.Conn
	change *atomic.Bool
}

func (c rvConn) Write(b []byte) (int, error) {
	if c.change.Load() {
		b = bytes.ReplaceAll(b, []byte("98:51:B3:F7"), []byte("99:51:B3:F7"))
	}
	return c.Conn.Write(b)
}

var (
	rvWeb = fstest.MapFS{
		"index.html": {Data: []byte("web client")},
		"install.md": {Data: []byte("install steps")},
	}
	rvDevice = regexp.MustCompile("browser|node|tablet|phone")
	rvSecret = identity.Secret("7K2M9QXD4F8TWB3N")
)

const rvICE = `{"t":"ice","ice":[],"ttl":3600,"hosts":true}`

// rvWorld is one scenario: a rendezvous and the connections of up to four devices.
type rvWorld struct {
	t          *testing.T
	bin        string
	http, ws   string
	listener   *rvListener
	conns      map[string]*websocket.Conn
	nonces     map[string]string
	earlier    *websocket.Conn
	sent       []byte   // the frame that the other side is to receive
	answers    []string // what the rendezvous answers when no limit is passed
	got        []string // what it did answer
	ended      error    // how reading those answers ended early
	offer      proto.Envelope
	room       string            // the room that steps name "the room"
	host, card string            // the device that hosts it, and its card
	guests     []string          // "each of them"
	greeted    map[string][]byte // the frames each guest got on joining
	pairs      [][]byte          // the room frames the guests sent
	cleanup    []func()
	httpClient *http.Client
}

func init() {
	stepSets["rendezvous"] = func(sc *godog.ScenarioContext, f fixtures) {
		rendezvousSteps(sc, f)
	}
}

// rvKey gives the other step sets of a scenario its rendezvous world.
type rvKey struct{}

func rendezvousSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &rvWorld{t: f.t, bin: f.bin, conns: map[string]*websocket.Conn{}, nonces: map[string]string{}, room: rvSecret.Room()}
	const device = `(browser|node|tablet|phone)`
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, rvKey{}, w), nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		for _, undo := range slices.Backward(w.cleanup) {
			undo()
		}
		return ctx, nil
	})
	sc.Step(`^a local rendezvous$`, w.local)
	sc.Step(`^the command "solodawn rendezvous" listening on a local port$`, w.command)
	sc.Step(`^the address "([^"]*)" returns the file "([^"]*)" of the web client$`, w.get)

	sc.Step(`^the `+device+` answers the challenge with (.+)$`, w.answerChallenge)
	sc.Step(`^(the .+) (?:is|are) signed in$`, w.signInAll)
	sc.Step(`^the `+device+` signs in(?: again)?$`, w.signIn)
	sc.Step(`^the `+device+` leaves$`, func(name string) error { return w.conns[name].CloseNow() })
	sc.Step(`^the `+device+` is disconnected$`, func(name string) error { return rvDisconnected(w.conns[name]) })
	sc.Step(`^the `+device+`'s earlier connection is disconnected$`, func(string) error { return rvDisconnected(w.earlier) })
	sc.Step(`^the `+device+` sends (\{.*\})$`, w.write)
	sc.Step(`^the `+device+` receives (\{.*\})$`, w.receive)
	sc.Step(`^the `+device+` has received nothing else$`, w.nothingElse)

	sc.Step(`^the `+device+` sends the `+device+` an envelope$`, func(from, to string) error { return w.route(from, to, "an offer") })
	sc.Step(`^the `+device+` receives that envelope from the `+device+`$`, func(name, from string) error {
		return w.receiveFrame(name, proto.Route{Type: "from", Key: exampleKey(from).ID(), Env: w.sent})
	})
	sc.Step(`^the `+device+` is told that the `+device+` is not connected$`, func(name, target string) error {
		return w.receiveFrame(name, proto.Offline{Type: "offline", Key: exampleKey(target).ID()})
	})
	sc.Step(`^the `+device+` watches (the .+)$`, func(name, names string) error {
		return w.send(name, proto.Watch{Type: "watch", Keys: rvIDs(rvDevice.FindAllString(names, -1))})
	})
	sc.Step(`^the `+device+` sees the `+device+` (on|off)line$`, func(name, watched, state string) error {
		return w.receiveFrame(name, proto.Presence{Type: "presence", Key: exampleKey(watched).ID(), On: state == "on"})
	})

	sc.Step(`^the `+device+` joins the room(?: of a pairing secret)?$`, func(name string) error {
		return w.send(name, proto.RoomID{Type: "join", Room: w.room})
	})
	sc.Step(`^the `+device+` is told that the room is joined$`, func(name string) error {
		return w.receiveFrame(name, proto.RoomID{Type: "joined", Room: w.room})
	})
	sc.Step(`^the `+device+` is told that (?:the|that) room is full$`, func(name string) error {
		return w.receiveFrame(name, proto.RoomID{Type: "full", Room: w.room})
	})
	sc.Step(`^the `+device+` sends its pair message through the room$`, w.sendPair)
	sc.Step(`^the `+device+` receives that room message$`, func(name string) error { return w.receive(name, string(w.sent)) })
	hostedRoomSteps(sc, w, device)

	sc.Step(`^the `+device+` sends a frame of (\d+) bytes$`, func(name string, size int) error {
		w.answers = []string{rvICE}
		return w.ask(name, fmt.Appendf(nil, "%-*s", size, `{"t":"ice"}`), 1)
	})
	sc.Step(`^the `+device+` watches (\d+) devices$`, w.watchMany)
	sc.Step(`^the `+device+` asks for the ICE servers (\d+) times$`, w.askMany)
	sc.Step(`^the `+device+` receives (\d+) answers?( and is disconnected)?$`, w.answered)

	sc.Step(`^the `+device+` offers the `+device+` a connection and the rendezvous (changes|preserves) the SDP fingerprint in transit$`, w.sendOffer)
	sc.Step(`^the `+device+`'s check of the offer returns "([^"]*)"$`, w.checkOffer)
}

func (w *rvWorld) local() {
	srv := httptest.NewUnstartedServer(rendezvous.Handler(rvWeb))
	w.listener = &rvListener{Listener: srv.Listener}
	srv.Listener = w.listener
	srv.Start()
	w.cleanup = append(w.cleanup, srv.Close)
	w.http, w.ws = srv.URL, "ws"+strings.TrimPrefix(srv.URL, "http")+"/rv"
}

func (w *rvWorld) addCleanup(undo func()) {
	w.cleanup = append(w.cleanup, undo)
}

func (w *rvWorld) command() error {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	addr := l.Addr().String()
	l.Close()
	cmd := exec.Command(w.bin, "rendezvous", "--listen", addr)
	if err := killManagedCmd(w.t, w.addCleanup, cmd, nil); err != nil {
		return err
	}
	w.ws = "ws://" + addr + "/rv"
	return rvListening(addr)
}

// The command has no way to say that it is listening.
func rvListening(addr string) error {
	return waitUntilEvery(50*time.Millisecond, 5*time.Second, "solodawn rendezvous to listen on "+addr, func() error {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			return err
		}
		return c.Close()
	})
}

func (w *rvWorld) get(path, file string) error {
	resp, err := http.Get(w.http + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 200 || !bytes.Equal(body, rvWeb[file].Data) {
		return fmt.Errorf("GET %s = %d %q, %v", path, resp.StatusCode, body, err)
	}
	return nil
}

// rvRead fails after 5 seconds, so that a frame that never comes fails its step.
func rvRead(c *websocket.Conn) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Read(ctx)
}

func (w *rvWorld) dial(name string) error {
	c, _, err := websocket.Dial(context.Background(), w.ws, &websocket.DialOptions{HTTPClient: w.httpClient})
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { c.CloseNow() })
	_, b, err := rvRead(c)
	if err != nil {
		return err
	}
	var challenge proto.Challenge
	if err := proto.Decode(b, &challenge); err != nil || challenge.Type != "challenge" {
		return fmt.Errorf("first frame %s: %v", b, err)
	}
	w.conns[name], w.nonces[name] = c, challenge.Nonce
	return nil
}

func (w *rvWorld) send(name string, frame any) error {
	b, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return w.write(name, string(b))
}

func (w *rvWorld) write(name, frame string) error {
	return w.conns[name].Write(context.Background(), websocket.MessageText, []byte(frame))
}

func (w *rvWorld) receive(name, want string) error {
	typ, b, err := rvRead(w.conns[name])
	if err != nil || typ != websocket.MessageText || string(b) != want {
		return fmt.Errorf("the %s received type %d %s, %v; want text %s", name, typ, b, err, want)
	}
	return nil
}

func (w *rvWorld) receiveFrame(name string, frame any) error {
	want, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return w.receive(name, string(want))
}

// An answer to a request is the next frame, so nothing was sent before it.
func (w *rvWorld) nothingElse(name string) error {
	if err := w.write(name, `{"t":"ice"}`); err != nil {
		return err
	}
	return w.receive(name, rvICE)
}

// rvEnded tells a connection that the rendezvous closed from a frame that never came.
func rvEnded(err error) bool {
	return err != nil && !errors.Is(err, context.DeadlineExceeded)
}

func rvDisconnected(c *websocket.Conn) error {
	if _, b, err := rvRead(c); !rvEnded(err) {
		return fmt.Errorf("want a closed connection, got %s, %v", b, err)
	}
	return nil
}

func rvIDs(names []string) []string {
	ids := []string{}
	for _, name := range names {
		ids = append(ids, exampleKey(name).ID())
	}
	return ids
}

// rvAnswer is what a device sends after the challenge, by how a scenario names it.
func rvAnswer(key identity.Key, nonce, other, how string) any {
	s, known := map[string]string{
		"its signature of that challenge":                 key.SignChallenge(nonce),
		"its signature of another connection's challenge": key.SignChallenge(other),
		"its signature of an envelope with that text":     key.Sign(nonce).Signature,
		"the node's signature of that challenge":          exampleKey("node").SignChallenge(nonce),
	}[how]
	if known {
		return proto.Auth{Type: "auth", Key: key.ID(), Signature: s}
	}
	if how == "a request for the ICE servers" {
		return proto.RVFrame{Type: "ice"}
	}
	panic("no step for the answer: " + how)
}

func (w *rvWorld) answerChallenge(name, how string) error {
	if err := w.dial(name); err != nil {
		return err
	}
	other := w.nonces[name]
	if strings.Contains(how, "another connection") {
		if err := w.dial(name); err != nil {
			return err
		}
	}
	return w.send(name, rvAnswer(exampleKey(name), w.nonces[name], other, how))
}

func (w *rvWorld) signIn(name string) error {
	w.earlier = w.conns[name]
	if err := w.answerChallenge(name, "its signature of that challenge"); err != nil {
		return err
	}
	return w.receive(name, `{"t":"ready","ice":[],"ttl":3600,"hosts":true}`)
}

func (w *rvWorld) signInAll(names string) error {
	for _, name := range rvDevice.FindAllString(names, -1) {
		if err := w.signIn(name); err != nil {
			return err
		}
	}
	return nil
}

func (w *rvWorld) route(from, to, payload string) (err error) {
	if w.sent, err = json.Marshal(exampleKey(from).Sign(payload)); err != nil {
		return err
	}
	return w.send(from, proto.Route{Type: "to", Key: exampleKey(to).ID(), Env: w.sent})
}

// rvPair is the pair message of a device. The rendezvous never reads it.
func rvPair(name string) json.RawMessage {
	pair := proto.Pair{Type: "pair", Key: exampleKey(name).ID(), Kind: "node", Name: name, Role: "a"}
	pair.MAC = rvSecret.MAC(pair)
	data, _ := json.Marshal(pair) // A struct of strings always encodes.
	return data
}

func (w *rvWorld) sendPair(name string) (err error) {
	if w.sent, err = json.Marshal(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: w.room}, Data: rvPair(name)}); err != nil {
		return err
	}
	return w.write(name, string(w.sent))
}

// ask sends a frame and reads the answers that are due to it, as far as they come.
func (w *rvWorld) ask(name string, frame []byte, due int) error {
	if err := w.write(name, string(frame)); err != nil {
		return err
	}
	for range due {
		_, b, err := rvRead(w.conns[name])
		if err != nil {
			w.ended = err
			return nil
		}
		w.got = append(w.got, string(b))
	}
	return nil
}

func (w *rvWorld) watchMany(name string, n int) error {
	watch := proto.Watch{Type: "watch", Keys: []string{}}
	for i := range n {
		id := exampleKey(fmt.Sprint("watched ", i)).ID()
		watch.Keys = append(watch.Keys, id)
		w.answers = append(w.answers, fmt.Sprintf(`{"t":"presence","k":"%s","on":false}`, id))
	}
	frame, err := json.Marshal(watch)
	if err != nil {
		return err
	}
	return w.ask(name, frame, n)
}

// Each request waits for the answer to the one before: a connection that is
// closed drops the answers it has not written yet.
func (w *rvWorld) askMany(name string, n int) error {
	for range n {
		w.answers = append(w.answers, rvICE)
		if err := w.ask(name, []byte(`{"t":"ice"}`), 1); err != nil {
			return err
		}
	}
	return nil
}

func (w *rvWorld) answered(_ string, n int, disconnected string) error {
	if !slices.Equal(w.got, w.answers[:n]) || rvEnded(w.ended) != (disconnected != "") {
		return fmt.Errorf("%d answers %q, then %v; want %q", len(w.got), w.got, w.ended, w.answers[:n])
	}
	return nil
}

// rvOffer is the offer of the wire examples, signed now.
func rvOffer() (string, error) {
	var offer proto.Offer
	if err := readWireExample("offer", &offer); err != nil {
		return "", err
	}
	offer.Timestamp = time.Now().UnixMilli()
	payload, err := json.Marshal(offer)
	return string(payload), err
}

func (w *rvWorld) sendOffer(from, to, action string) error {
	w.listener.change.Store(action == "changes")
	payload, err := rvOffer()
	if err != nil {
		return err
	}
	if err := w.route(from, to, payload); err != nil {
		return err
	}
	return w.receiveOffer(to, from)
}

func (w *rvWorld) receiveOffer(name, from string) error {
	_, b, err := rvRead(w.conns[name])
	if err != nil {
		return err
	}
	var got proto.Route
	if err := proto.Decode(b, &got); err != nil || got.Type != "from" || got.Key != exampleKey(from).ID() {
		return fmt.Errorf("the %s received %s, %v", name, b, err)
	}
	return proto.Decode(got.Env, &w.offer)
}

func (w *rvWorld) checkOffer(name, want string) error {
	result := "accepted"
	known := func(id string) bool { return id == exampleKey("browser").ID() }
	if _, err := identity.OpenSignal(w.offer, exampleKey(name).ID(), known, time.Now()); err != nil {
		result = err.Error()
	}
	if result != want {
		return fmt.Errorf("offer check = %s, want %s", result, want)
	}
	return nil
}

// hostedRoomSteps are the steps of a room that one device hosts
// (contracts/rendezvous.md 2.2).
func hostedRoomSteps(sc *godog.ScenarioContext, w *rvWorld, device string) {
	sc.Step(`^the `+device+` hosts the room(?: of (another) pairing secret)? with its pair message$`, func(name, another string) error {
		if another != "" {
			w.room = identity.NewSecret().Room()
		}
		w.host, w.card = name, string(rvPair(name))
		return w.hostRoom(name, w.room, rvPair(name))
	})
	sc.Step(`^the `+device+` hosts (\d+) rooms$`, func(name string, n int) error {
		for i := range n {
			if err := w.hostRoom(name, fmt.Sprintf("%032x", i), rvPair(name)); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the `+device+` hosts one more room$`, func(name string) error {
		w.room = identity.NewSecret().Room()
		return w.hostRoom(name, w.room, rvPair(name))
	})
	sc.Step(`^the `+device+` leaves the room$`, func(name string) error {
		return w.send(name, proto.RoomID{Type: "leave", Room: w.room})
	})
	sc.Step(`^the `+device+` receives the host's pair message$`, func(name string) error {
		return w.receive(name, w.cardFrame())
	})
	sc.Step(`^(the .+) join the room (one after another|at once)$`, w.joinHosted)
	sc.Step(`^each of them is told that the room is joined and receives the host's pair message$`, w.guestsGreeted)
	sc.Step(`^each of them sends its pair message through the room$`, w.guestsSendPairs)
	sc.Step(`^the `+device+` receives each of those room messages$`, w.receivePairs)
}

func (w *rvWorld) hostRoom(name, room string, data json.RawMessage) error {
	return w.send(name, proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: room}, Data: data})
}

// cardFrame is the room frame that carries the host's card.
func (w *rvWorld) cardFrame() string {
	b, _ := json.Marshal(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: w.room}, Data: json.RawMessage(w.card)})
	return string(b)
}

// joinHosted joins each device in turn. One after another, a device joins
// once the one before has been answered; at once, all join before any answer
// is read.
func (w *rvWorld) joinHosted(names, how string) error {
	w.guests, w.greeted = rvDevice.FindAllString(names, -1), map[string][]byte{}
	for _, name := range w.guests {
		if err := w.send(name, proto.RoomID{Type: "join", Room: w.room}); err != nil {
			return err
		}
		if how == "one after another" {
			if err := w.readGreeting(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// readGreeting reads the two frames that a guest gets on joining.
func (w *rvWorld) readGreeting(name string) error {
	for range 2 {
		_, b, err := rvRead(w.conns[name])
		if err != nil {
			return err
		}
		w.greeted[name] = append(append(w.greeted[name], b...), '\n')
	}
	return nil
}

func (w *rvWorld) guestsGreeted() error {
	joined, _ := json.Marshal(proto.RoomID{Type: "joined", Room: w.room})
	want := string(joined) + "\n" + w.cardFrame() + "\n"
	for _, name := range w.guests {
		if w.greeted[name] == nil {
			if err := w.readGreeting(name); err != nil {
				return err
			}
		}
		if got := string(w.greeted[name]); got != want {
			return fmt.Errorf("the %s got %s, want %s", name, got, want)
		}
	}
	return nil
}

func (w *rvWorld) guestsSendPairs() error {
	w.pairs = nil
	for _, name := range w.guests {
		if err := w.sendPair(name); err != nil {
			return err
		}
		w.pairs = append(w.pairs, w.sent)
	}
	return nil
}

// receivePairs reads one frame for each pair message sent, in any order.
func (w *rvWorld) receivePairs(name string) error {
	var got, want []string
	for _, pair := range w.pairs {
		_, b, err := rvRead(w.conns[name])
		if err != nil {
			return err
		}
		got, want = append(got, string(b)), append(want, string(pair))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("the %s received %q, want %q", name, got, want)
	}
	return nil
}
