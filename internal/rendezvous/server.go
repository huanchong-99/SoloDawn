// Package rendezvous serves the web client and routes WebSocket signalling.
package rendezvous

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"slices"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type server struct {
	*relay
	wait     time.Duration // for the answer to the challenge
	mu       sync.Mutex
	online   map[string]*client
	rooms    map[string]*room
	watchers map[string]map[*client]bool // by watched device id
}

// Handler serves the same /rv protocol and static files on nodes and on the
// standalone rendezvous. Signalling alone advertises an empty ICE list.
func Handler(web fs.FS) http.Handler {
	return (&relay{}).handler(web)
}

func (r *relay) handler(web fs.FS) http.Handler {
	return handler(web, 10*time.Second, r)
}

func handler(web fs.FS, wait time.Duration, r *relay) http.Handler {
	s := &server{relay: r, wait: wait, online: make(map[string]*client), rooms: make(map[string]*room), watchers: make(map[string]map[*client]bool)}
	mux := http.NewServeMux()
	mux.HandleFunc("/rv", s.serve)
	mux.Handle("/", http.FileServerFS(web))
	return mux
}

// Browsers can signal through a different origin. Authentication uses the
// challenge signature and never ambient browser credentials.
var anyOrigin = &websocket.AcceptOptions{InsecureSkipVerify: true}

const maxBytes = 16_384 // 16 KiB

type client struct {
	conn   *websocket.Conn
	out    chan frame
	id     string
	watch  []string
	rooms  map[string]*room // the rooms this connection is in
	window [120]time.Time
	next   int
	mu     sync.Mutex     // guards queued, which the writer also changes
	queued map[string]int // frames in out by the device they are from
}

// frame is a frame waiting to be written, and the device it is from when the
// server forwards it.
type frame struct {
	data []byte
	from string
}

// maxQueued is how many frames one sender may have in a receiver's queue.
const maxQueued = 16

func (s *server) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, anyOrigin)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxBytes)
	c := &client{conn: conn, out: make(chan frame, 128), rooms: make(map[string]*room), queued: make(map[string]int)}
	go c.write()
	defer close(c.out)
	if !c.authenticate(s.wait) {
		return
	}
	s.connect(c)
	defer s.disconnect(c)
	for {
		msg, kind := c.read(context.Background())
		if !s.handle(c, msg, kind) {
			return
		}
	}
}

// Only this goroutine writes to the socket. Closing the socket on write
// failure wakes the reader, which unregisters the device.
func (c *client) write() {
	defer c.conn.CloseNow()
	for f := range c.out {
		c.dequeued(f.from)
		if c.conn.Write(context.Background(), websocket.MessageText, f.data) != nil {
			return
		}
	}
}

func (c *client) send(v any) bool { return c.sendFrom("", v) }

// While routing, a stalled socket must not hold up other devices: a frame for
// a receiver whose queue is full, or that already holds maxQueued frames of
// the same sender, is dropped, and both connections stay open. The sender's
// offer timeout and back-off cover the loss. sendFrom returns false when the
// frame would exceed the wire limit. In particular, the authenticated "from"
// wrapper is longer than "to".
func (c *client) sendFrom(from string, v any) bool {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // All outbound frames contain validated JSON.
	}
	data := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	if len(data) > maxBytes {
		return false
	}
	if c.enqueued(from) {
		select {
		case c.out <- frame{data, from}:
		default:
			c.dequeued(from)
		}
	}
	return true
}

// enqueued counts a frame of a sender in the queue, if the sender has room.
// The server's own frames are from nobody and have no share.
func (c *client) enqueued(from string) bool {
	if from == "" {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.queued[from] == maxQueued {
		return false
	}
	c.queued[from]++
	return true
}

func (c *client) dequeued(from string) {
	if from == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	decrement(c.queued, from)
}

func (c *client) admit(now time.Time) bool {
	if now.Sub(c.window[c.next]) < 10*time.Second {
		return false
	}
	c.window[c.next] = now
	c.next = (c.next + 1) % len(c.window)
	return true
}

type fields map[string]json.RawMessage

// Split only the outer object. Decode the members we read with proto.Decode;
// passing env or data through a decoder could change another peer's object.
func (c *client) read(ctx context.Context) (fields, string) {
	t, b, err := c.conn.Read(ctx)
	if err != nil || t != websocket.MessageText || !utf8.Valid(b) || !c.admit(time.Now()) {
		return nil, ""
	}
	var m fields
	if json.Unmarshal(b, &m) != nil {
		return nil, ""
	}
	return m, m.text("t")
}

func (m fields) text(name string) string {
	var value string
	// A missing, null, or wrongly typed string leaves the value empty.
	// The fields read this way all require nonempty strings.
	_ = proto.Decode(m[name], &value)
	return value
}

// A connection that does not answer its challenge in time is closed: until
// then it holds a socket without having proved a key.
func (c *client) authenticate(wait time.Duration) bool {
	var b [16]byte
	rand.Read(b[:])
	nonce := base64.RawURLEncoding.EncodeToString(b[:])
	c.send(proto.Challenge{Type: "challenge", Nonce: nonce})
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	m, kind := c.read(ctx)
	c.id = m.text("k")
	return kind == "auth" && identity.VerifyChallenge(c.id, nonce, m.text("s"))
}

func (s *server) connect(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.online[c.id]
	s.online[c.id] = c
	c.send(s.ice("ready", c.id))
	if old != nil {
		s.leaveRooms(old)
		s.unwatch(old)
		old.conn.CloseNow()
	} else {
		s.notify(c.id, true)
	}
}

func (s *server) disconnect(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.online[c.id] == c {
		delete(s.online, c.id)
		s.leaveRooms(c)
		s.unwatch(c)
		s.notify(c.id, false)
	}
}

func (s *server) handle(c *client, m fields, kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.online[c.id] == c && s.dispatch(c, m, kind)
}

func (s *server) dispatch(c *client, m fields, kind string) bool {
	switch kind {
	case "ice":
		c.send(s.ice("ice", c.id))
	case "quota":
		c.send(s.quota(s.alias(c.id)))
	case "to":
		return s.route(c, m)
	case "watch":
		return s.watch(c, m)
	case "join", "room":
		return s.roomFrame(c, m, kind)
	default:
		return false
	}
	return true
}

func (m fields) object(name string) bool {
	raw := m[name]
	return len(raw) != 0 && raw[0] == '{'
}

// A target must be a device id; anything else is bad input.
func (s *server) route(c *client, m fields) bool {
	var key string
	if proto.Decode(m["k"], &key) != nil || !identity.IsDevice(key) || !m.object("env") {
		return false
	}
	if target := s.online[key]; target != nil {
		return target.sendFrom(c.id, proto.Route{Type: "from", Key: c.id, Env: m["env"]})
	}
	c.send(proto.Offline{Type: "offline", Key: key})
	return true
}

func (s *server) watch(c *client, m fields) bool {
	var keys []string
	if proto.Decode(m["k"], &keys) != nil || len(keys) > 64 || slices.ContainsFunc(keys, notDevice) {
		return false
	}
	s.unwatch(c)
	c.watch = keys
	for _, key := range keys {
		s.index(key, c)
		c.send(proto.Presence{Type: "presence", Key: key, On: s.online[key] != nil})
	}
	return true
}

// The server keeps, for each watched device, who watches it, so that a
// change of presence does not look at every connection.
func (s *server) index(key string, c *client) {
	set := s.watchers[key]
	if set == nil {
		set = make(map[*client]bool)
		s.watchers[key] = set
	}
	set[c] = true
}

func (s *server) unwatch(c *client) {
	for _, key := range c.watch {
		set := s.watchers[key]
		if delete(set, c); len(set) == 0 {
			delete(s.watchers, key)
		}
	}
}

func notDevice(id string) bool { return !identity.IsDevice(id) }

func (s *server) notify(key string, online bool) {
	for c := range s.watchers[key] {
		c.send(proto.Presence{Type: "presence", Key: key, On: online})
	}
}
