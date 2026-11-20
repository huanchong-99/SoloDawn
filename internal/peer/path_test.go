package peer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type packet struct {
	typ  websocket.MessageType
	text string
}

// challenger is a signalling server that sends challenge and, if key signs
// it, ready.
func challenger(t *testing.T, key identity.Key, challenge packet, ready string, accepted chan<- *websocket.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		accepted <- ws
		ctx := t.Context()
		ws.Write(ctx, challenge.typ, []byte(challenge.text))
		_, raw, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var auth proto.Auth
		check(t, proto.Decode(raw, &auth))
		if !identity.VerifyChallenge(auth.Key, "test-nonce", auth.Signature) || auth.Key != key.ID() || auth.Type != "auth" {
			t.Error("wrong challenge response")
			return
		}
		ws.Write(ctx, websocket.MessageText, []byte(ready))
	}
}
func testPath(t *testing.T, challenge packet, ready string) (*Path, *websocket.Conn, error) {
	t.Helper()
	key := testKey(t)
	accepted := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(challenger(t, key, challenge, ready, accepted))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	p, err := Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), key)
	ws := recv(t, accepted)
	t.Cleanup(func() { ws.CloseNow() })
	if p != nil {
		t.Cleanup(func() { p.Close() })
	}
	return p, ws, err
}

var challenge = packet{websocket.MessageText, `{"t":"challenge","nonce":"test-nonce"}`}

const ready = `{"t":"ready","ice":[],"ttl":3600}`

func TestPathFrameLimitsAndRefresh(t *testing.T) {
	p, ws, err := testPath(t, challenge, ready)
	check(t, err)
	eq(t, p.lifetime(), time.Hour)
	eq(t, p.Route().ICE, []proto.ICEServer{})
	e := proto.Envelope{Payload: "<local>", Key: "peer", Signature: "sig"}
	check(t, p.Route().Send(t.Context(), "device", e))
	ty, raw, err := ws.Read(t.Context())
	check(t, err)
	eq(t, ty, websocket.MessageText)
	eq(t, string(raw), `{"t":"to","k":"device","env":{"p":"<local>","k":"peer","s":"sig"}}`)
	send := func(n int) error { return p.Write(t.Context(), json.RawMessage(`"`+strings.Repeat("a", n-2)+`"`)) }
	check(t, send(16384))
	_, data, err := ws.Read(t.Context())
	check(t, err)
	eq(t, len(data), 16384)
	eq(t, send(16385).Error(), "signalling frame exceeds 16 KiB")
	node := newPair(t).b
	eq(t, p.from(t.Context(), node, []byte(`{"t":"from","k":"device"}`)).Error(), "proto: a field that is always present is missing")
	sentinel := errors.New("finish")
	events := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- p.Serve(t.Context(), node, func(kind string, b []byte) error { events <- kind + ":" + string(b); return sentinel })
	}()
	for _, msg := range []string{`{"t":"from","k":"unknown","env":{}}`, `{"t":"ice","ttl":1,"ice":[{"urls":["stun:127.0.0.1"]}]}`} {
		check(t, ws.Write(t.Context(), websocket.MessageText, []byte(msg)))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, data, err = ws.Read(ctx)
	check(t, err)
	eq(t, string(data), `{"t":"ice"}`)
	eq(t, p.Route().ICE, []proto.ICEServer{{URLs: []string{"stun:127.0.0.1"}}})
	check(t, ws.Write(ctx, websocket.MessageText, []byte(`{"t":"presence","k":"device","on":true}`)))
	eq(t, recv(t, events), `presence:{"t":"presence","k":"device","on":true}`)
	eq(t, recv(t, done), sentinel)
}
func TestHandshakeRejectsInvalidFrames(t *testing.T) {
	for _, tc := range []struct {
		frame packet
		ready string
		msg   string
	}{
		{packet{websocket.MessageBinary, challenge.text}, ready, "signalling frame is not text"},
		{packet{websocket.MessageText, string([]byte{255})}, ready, "signalling frame is not text"},
		{packet{websocket.MessageText, `{"t":"ready"}`}, ready, "unexpected signalling frame: ready"},
		{challenge, `{"t":"ready","ICE":[],"ttl":1}`, "proto: a field that is always present is missing"},
		{challenge, `{"t":"ready","ice":[],"ttl":0}`, "invalid ICE credential lifetime"},
	} {
		p, _, err := testPath(t, tc.frame, tc.ready)
		eq(t, p, (*Path)(nil))
		eq(t, err.Error(), tc.msg)
	}
	for _, tc := range []struct {
		ttl int
		ok  bool
	}{{1, true}, {9223372036, true}, {9223372037, false}} {
		eq(t, validateTTL(tc.ttl) == nil, tc.ok)
	}
}
func TestFanoutEveryPath(t *testing.T) {
	sent := make(chan string, 3)
	paths := make([]Route, 3)
	for i, key := range []string{"first", "second", "third"} {
		paths[i].Send = func(_ context.Context, to string, e proto.Envelope) error {
			sent <- key
			if key == "second" {
				return nil
			}
			return errors.New(key)
		}
	}
	check(t, broadcast(t.Context(), paths, "", proto.Envelope{}))
	seen := map[string]int{}
	for range 3 {
		seen[recv(t, sent)]++
	}
	eq(t, seen, map[string]int{"first": 1, "second": 1, "third": 1})
	eq(t, broadcast(t.Context(), paths[:1], "", proto.Envelope{}).Error(), "first")
	eq(t, broadcast(t.Context(), nil, "", proto.Envelope{}).Error(), "no signalling path")
}

// A path ends when its connection ends or the node fails at something other
// than an envelope check; a refused ICE lifetime ends it too.
func TestPathEnds(t *testing.T) {
	_, err := Dial(t.Context(), "ws://127.0.0.1:1/rv", testKey(t))
	eq(t, err == nil, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ws, err := websocket.Accept(w, r, nil); err == nil {
			ws.CloseNow()
		}
	}))
	t.Cleanup(srv.Close)
	_, err = Dial(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http"), testKey(t))
	eq(t, err == nil, false)

	p, ws, err := testPath(t, challenge, ready)
	check(t, err)
	f := newPair(t)
	eq(t, p.handle(t.Context(), "ice", []byte(`{"t":"ice","ice":[],"ttl":0}`), f.b, nil, nil).Error(), "invalid ICE credential lifetime")
	f.b.paths = func(string) []Route { return []Route{{ICE: []proto.ICEServer{{URLs: []string{"bogus"}}}}} }
	offer := f.a.sign(proto.Offer{SignalHeader: f.a.header(link{f.b.key.ID(), randomID()}, "offer"), SDP: "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\n"})
	err = p.from(t.Context(), f.b, encode(frame(offer)))
	var code identity.Code
	eq(t, err != nil && !errors.As(err, &code), true)
	done := make(chan error, 1)
	go func() { done <- p.Serve(t.Context(), f.b, nil) }()
	check(t, ws.CloseNow())
	eq(t, recv(t, done) == nil, false)
}
func TestMustStopsOnError(t *testing.T) {
	must(nil)
	defer func() { eq(t, recover(), any(errors.New("mistake"))) }()
	must(errors.New("mistake"))
}
