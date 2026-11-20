//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
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
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/gorilla/websocket"
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
	rvDevice = regexp.MustCompile("browser|node|tablet")
	rvSecret = identity.Secret("7K2M9QXD4F8TWB3N")
)

const rvICE = `{"t":"ice","ice":[],"ttl":3600}`

// rvWorld is one scenario: a rendezvous and the connections of up to three devices.
type rvWorld struct {
	bin      string
	http, ws string
	listener *rvListener
	conns    map[string]*websocket.Conn
	nonces   map[string]string
	earlier  *websocket.Conn
	sent     []byte   // the frame that the other side is to receive
	answers  []string // what the rendezvous answers when no limit is passed
	got      []string // what it did answer
	ended    error    // how reading those answers ended early
	offer    proto.Envelope
	cleanup  []func()
}

func rendezvousSteps(sc *godog.ScenarioContext, bin string) {
	w := &rvWorld{bin: bin, conns: map[string]*websocket.Conn{}, nonces: map[string]string{}}
	const device = `(browser|node|tablet)`
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for _, undo := range slices.Backward(w.cleanup) {
			undo()
		}
		return ctx, err
	})
	sc.Step(`^a local rendezvous$`, w.local)
	sc.Step(`^the command "solodawn rendezvous" listening on a local port$`, w.command)
	sc.Step(`^the address "([^"]*)" returns the file "([^"]*)" of the web client$`, w.get)

	sc.Step(`^the `+device+` answers the challenge with (.+)$`, w.answerChallenge)
	sc.Step(`^(the .+) (?:is|are) signed in$`, w.signInAll)
	sc.Step(`^the `+device+` signs in(?: again)?$`, w.signIn)
	sc.Step(`^the `+device+` leaves$`, func(name string) error { return w.conns[name].Close() })
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
		return w.send(name, proto.RoomID{Type: "join", Room: rvSecret.Room()})
	})
	sc.Step(`^the `+device+` is told that the room is joined$`, func(name string) error {
		return w.receiveFrame(name, proto.RoomID{Type: "joined", Room: rvSecret.Room()})
	})
	sc.Step(`^the `+device+` sends its pair message through the room$`, w.sendPair)
	sc.Step(`^the `+device+` receives that room message$`, func(name string) error { return w.receive(name, string(w.sent)) })

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

func (w *rvWorld) command() error {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	addr := l.Addr().String()
	l.Close()
	cmd := exec.Command(w.bin, "rendezvous", "--listen", addr)
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { cmd.Process.Kill(); cmd.Wait() })
	w.ws = "ws://" + addr + "/rv"
	return rvListening(addr)
}

// The command has no way to say that it is listening.
func rvListening(addr string) error {
	for range 100 {
		if c, err := net.Dial("tcp", addr); err == nil {
			return c.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("solodawn rendezvous does not listen on %s", addr)
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

func (w *rvWorld) dial(name string) error {
	c, _, err := websocket.DefaultDialer.Dial(w.ws, nil)
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { c.Close() })
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, b, err := c.ReadMessage()
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
	return w.conns[name].WriteJSON(frame)
}

func (w *rvWorld) write(name, frame string) error {
	return w.conns[name].WriteMessage(websocket.TextMessage, []byte(frame))
}

func (w *rvWorld) receive(name, want string) error {
	typ, b, err := w.conns[name].ReadMessage()
	if err != nil || typ != websocket.TextMessage || string(b) != want {
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

func rvDisconnected(c *websocket.Conn) error {
	if _, b, err := c.ReadMessage(); !websocket.IsUnexpectedCloseError(err) {
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
	return w.receive(name, `{"t":"ready","ice":[],"ttl":3600}`)
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

func (w *rvWorld) sendPair(name string) error {
	id := exampleKey(name).ID()
	pair := proto.Pair{Type: "pair", Key: id, Kind: "node", Name: name, Role: "a", MAC: rvSecret.MAC("a", id)}
	data, err := json.Marshal(pair)
	if err != nil {
		return err
	}
	if w.sent, err = json.Marshal(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: rvSecret.Room()}, Data: data}); err != nil {
		return err
	}
	return w.write(name, string(w.sent))
}

// ask sends a frame and reads the answers that are due to it, as far as they come.
func (w *rvWorld) ask(name string, frame []byte, due int) error {
	if err := w.conns[name].WriteMessage(websocket.TextMessage, frame); err != nil {
		return err
	}
	for range due {
		_, b, err := w.conns[name].ReadMessage()
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
	if !slices.Equal(w.got, w.answers[:n]) || websocket.IsUnexpectedCloseError(w.ended) != (disconnected != "") {
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
	_, b, err := w.conns[name].ReadMessage()
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
