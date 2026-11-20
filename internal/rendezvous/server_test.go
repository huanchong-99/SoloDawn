package rendezvous

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func fixture(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(Handler(fstest.MapFS{
		"index.html": {Data: []byte("web client")},
		"install.md": {Data: []byte("install steps")},
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func key(b byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, 32)))
}

func dial(t *testing.T, url string) (*websocket.Conn, string) {
	t.Helper()
	header := http.Header{"Origin": {"https://client.example"}}
	c, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(url, "http")+"/rv", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	var challenge proto.Challenge
	if _, b, err := read(t, c); err != nil || json.Unmarshal(b, &challenge) != nil {
		t.Fatalf("first frame = %s, %v", b, err)
	}
	checkChallenge(t, challenge)
	return c, challenge.Nonce
}

// read fails after 5 seconds, so that a frame that never comes fails its test.
func read(t *testing.T, c *websocket.Conn) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return c.Read(ctx)
}

func write(t *testing.T, c *websocket.Conn, typ websocket.MessageType, frame string) {
	t.Helper()
	if err := c.Write(t.Context(), typ, []byte(frame)); err != nil {
		t.Fatal(err)
	}
}

func checkChallenge(t *testing.T, challenge proto.Challenge) {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(challenge.Nonce)
	if challenge.Type != "challenge" {
		t.Fatalf("first frame = %q", challenge.Type)
	}
	if err != nil || len(b) != 16 {
		t.Fatalf("nonce bytes = %x, %v", b, err)
	}
	if base64.RawURLEncoding.EncodeToString(b) != challenge.Nonce {
		t.Fatalf("challenge = %+v", challenge)
	}
}

func send(t *testing.T, c *websocket.Conn, msg any) {
	t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	write(t, c, websocket.MessageText, string(b))
}

func expect(t *testing.T, c *websocket.Conn, want any) {
	t.Helper()
	typ, b, err := read(t, c)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := json.Marshal(want)
	if typ != websocket.MessageText || !bytes.Equal(b, w) {
		t.Fatalf("received %d %s, want text %s", typ, b, w)
	}
}

// closed holds that the server ended the connection, not that nothing came.
func closed(t *testing.T, c *websocket.Conn) {
	t.Helper()
	if _, b, err := read(t, c); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want closed socket, got %s, %v", b, err)
	}
}

func login(t *testing.T, url string, k identity.Key) *websocket.Conn {
	t.Helper()
	c, nonce := dial(t, url)
	send(t, c, proto.Auth{Type: "auth", Key: k.ID(), Signature: k.SignChallenge(nonce)})
	expect(t, c, proto.ICEConfig{Type: "ready", ICE: []proto.ICEServer{}, TTL: 3600})
	return c
}

func TestStaticAndUpgrade(t *testing.T) {
	url := fixture(t)
	for path, want := range map[string]string{"/": "web client", "/install.md": "install steps", "/missing": "404 page not found\n", "/rv": "WebSocket protocol violation: Connection header \"\" does not contain Upgrade\n"} {
		resp, err := http.Get(url + path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(data) != want {
			t.Errorf("GET %s = %q, %v, want %q", path, data, err, want)
		}
	}
}

func TestHandshakeRefusals(t *testing.T) {
	url := fixture(t)
	k := key(1)
	first, n1 := dial(t, url)
	second, n2 := dial(t, url)
	if n1 == n2 {
		t.Fatal("repeated challenge")
	}
	send(t, first, proto.Auth{Type: "auth", Key: k.ID(), Signature: k.SignChallenge(n2)})
	send(t, second, proto.Auth{Type: "auth", Key: k.ID(), Signature: k.Sign(n2).Signature})
	closed(t, first)
	closed(t, second)
	c, n := dial(t, url)
	send(t, c, proto.Auth{Type: "to", Key: k.ID(), Signature: k.SignChallenge(n)})
	closed(t, c)
}

func TestChallengeDeadline(t *testing.T) {
	s := httptest.NewServer(handler(fstest.MapFS{}, 100*time.Millisecond, &relay{}))
	t.Cleanup(s.Close)
	idle, _ := dial(t, s.URL)
	active := login(t, s.URL, key(1))
	closed(t, idle)
	// The deadline is over, and it was for the answer to the challenge only.
	send(t, active, proto.RVFrame{Type: "ice"})
	expect(t, active, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600})
}

func TestRoutePresenceAndReplacement(t *testing.T) {
	url := fixture(t)
	a, b := key(1), key(2)
	from := login(t, url, a)
	send(t, from, proto.Watch{Type: "watch", Keys: []string{b.ID(), "no device id"}})
	expect(t, from, proto.Presence{Type: "presence", Key: b.ID(), On: false})
	expect(t, from, proto.Presence{Type: "presence", Key: "no device id", On: false})
	// The application owning the envelope is the only one that reads it.
	// Even unknown keys, an out-of-range number and an invalid payload survive.
	env := json.RawMessage(`{"Unknown":9e999,"p":"  not JSON","s":null}`)
	to := proto.Route{Type: "to", Key: b.ID(), Env: env}
	send(t, from, to)
	expect(t, from, proto.Offline{Type: "offline", Key: b.ID()})
	send(t, from, proto.Route{Type: "to", Key: "", Env: env})
	expect(t, from, proto.Offline{Type: "offline", Key: ""})
	old := login(t, url, b)
	expect(t, from, proto.Presence{Type: "presence", Key: b.ID(), On: true})
	newConn := login(t, url, b)
	closed(t, old)
	send(t, from, to)
	expect(t, newConn, proto.Route{Type: "from", Key: a.ID(), Env: env})
	newConn.CloseNow()
	expect(t, from, proto.Presence{Type: "presence", Key: b.ID(), On: false})
	send(t, from, proto.Watch{Type: "watch", Keys: []string{}})
	ice := proto.RVFrame{Type: "ice"}
	send(t, from, ice)
	emptyICE := proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600}
	expect(t, from, emptyICE)
	_ = login(t, url, b)
	send(t, from, ice)
	expect(t, from, emptyICE)
}

func TestInvalidFrames(t *testing.T) {
	url := fixture(t)
	for _, msg := range []string{`{}`, `null`, `{"T":"ice"}`, `{"t":"ready"}`, `[]`, `{}{}`, `{"t":"to","k":5,"env":{}}`, `{"t":"to","env":{}}`, fmt.Sprintf(`{"t":"to","k":%q}`, key(2).ID()), fmt.Sprintf(`{"t":"to","k":%q,"env":null}`, key(2).ID()), `{"t":"watch"}`, `{"t":"watch","k":[null]}`, `{"t":"watch","k":"device"}`, `{"t":"join","room":"AB"}`, `{"t":"room","room":"00000000000000000000000000000000","data":[]}`, "{\"t\":\"ice\",\"x\":\"\xff\"}"} {
		t.Run(msg, func(t *testing.T) {
			c := login(t, url, key(1))
			write(t, c, websocket.MessageText, msg)
			closed(t, c)
		})
	}
	t.Run("binary", func(t *testing.T) {
		c := login(t, url, key(1))
		write(t, c, websocket.MessageBinary, `{"t":"ice"}`)
		closed(t, c)
	})
}

func TestLimits(t *testing.T) {
	url := fixture(t)
	c := login(t, url, key(1))
	msg := fmt.Sprintf("%-*s", 16384, `{"t":"ice"}`)
	write(t, c, websocket.MessageText, msg)
	ice := proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600}
	expect(t, c, ice)
	write(t, c, websocket.MessageText, msg+" ")
	closed(t, c)
	c = login(t, url, key(1))
	for range 119 {
		send(t, c, proto.RVFrame{Type: "ice"})
		expect(t, c, ice)
	}
	send(t, c, proto.RVFrame{Type: "ice"})
	closed(t, c)

	c = login(t, url, key(1))
	keys := make([]string, 64)
	for i := range keys {
		keys[i] = key(byte(i + 1)).ID()
	}
	send(t, c, proto.Watch{Type: "watch", Keys: keys})
	for _, k := range keys {
		expect(t, c, proto.Presence{Type: "presence", Key: k, On: k == key(1).ID()})
	}
	send(t, c, proto.Watch{Type: "watch", Keys: append(keys, key(65).ID())})
	closed(t, c)
}

func TestForwardedFrameSize(t *testing.T) {
	url := fixture(t)
	a, b := key(1), key(2)
	sender, receiver := login(t, url, a), login(t, url, b)
	receiver.SetReadLimit(16_384)
	to := fmt.Sprintf(`{"t":"to","k":"%s","env":{"p":"`, b.ID())
	from := fmt.Sprintf(`{"t":"from","k":"%s","env":{"p":"`, a.ID())
	p := strings.Repeat("&", 16_384-len(from)-3)
	write(t, sender, websocket.MessageText, to+p+`"}}`)
	typ, data, err := read(t, receiver)
	if err != nil || typ != websocket.MessageText || string(data) != from+p+`"}}` {
		t.Fatalf("forwarded boundary frame: type %d, length %d, error %v", typ, len(data), err)
	}
	write(t, sender, websocket.MessageText, to+p+`&"}}`)
	closed(t, sender)
	// The target never gets a partial or oversized frame and stays connected.
	send(t, receiver, proto.RVFrame{Type: "ice"})
	expect(t, receiver, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600})
}

func TestInvalidServerFramePanics(t *testing.T) {
	defer func() {
		got := recover()
		if fmt.Sprint(got) != "json: unsupported type: chan int" {
			t.Errorf("panic = %v", got)
		}
	}()
	new(client).send(make(chan int))
}

func TestRollingWindow(t *testing.T) {
	var c client
	start := time.Unix(123, 0)
	for i := range 120 {
		if !c.admit(start.Add(time.Duration(i) * time.Millisecond)) {
			t.Fatalf("refused %d", i)
		}
	}
	for _, tc := range []struct {
		offset time.Duration
		want   bool
	}{
		{10*time.Second - time.Nanosecond, false},
		{10 * time.Second, true},
		{10 * time.Second, false},
		{10*time.Second + time.Millisecond, true},
	} {
		if got := c.admit(start.Add(tc.offset)); got != tc.want {
			t.Errorf("admit(%s) = %v, want %v", tc.offset, got, tc.want)
		}
	}
}

func TestFullSendQueueAndFailedWrite(t *testing.T) {
	conn, _ := dial(t, fixture(t))
	c := client{conn: conn, out: make(chan []byte, 1)}
	c.send("first")
	c.send("second")
	if got := <-c.out; string(got) != `"first"` {
		t.Fatalf("queue = %v", got)
	}
	// A failed write must return even when the queue remains open.
	c.out <- []byte(`"after close"`)
	done := make(chan struct{})
	go func() { c.write(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("writer blocked after a failed write")
	}
}

func TestSupersededSocketCannotDispatch(t *testing.T) {
	s := server{online: map[string]*client{}}
	old := &client{id: "device"}
	current := &client{id: "device", out: make(chan []byte, 1)}
	s.online["device"] = current
	if s.handle(old, nil, "ice") {
		t.Fatal("replaced socket still active")
	}
	s.disconnect(old)
	if !reflect.DeepEqual(s.online, map[string]*client{"device": current}) {
		t.Fatal("disconnect removed replacement")
	}
}
