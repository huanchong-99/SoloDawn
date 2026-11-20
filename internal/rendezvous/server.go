// Package rendezvous serves the web client and routes WebSocket signalling.
package rendezvous

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type server struct {
	mu     sync.Mutex
	online map[string]*client
	rooms  map[string]*room
}

// Handler serves the same /rv protocol and static files on nodes and on the
// standalone rendezvous. Signalling alone advertises an empty ICE list.
func Handler(web fs.FS) http.Handler {
	s := &server{online: make(map[string]*client), rooms: make(map[string]*room)}
	mux := http.NewServeMux()
	mux.HandleFunc("/rv", s.serve)
	mux.Handle("/", http.FileServerFS(web))
	return mux
}

// Browsers can signal through a different origin. Authentication uses the
// challenge signature and never ambient browser credentials.
var upgrade = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

const maxBytes = 16_384 // 16 KiB

type client struct {
	conn   *websocket.Conn
	out    chan any
	id     string
	watch  map[string]bool
	window [120]time.Time
	next   int
}

func (s *server) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxBytes)
	c := &client{conn: conn, out: make(chan any, 128)}
	go c.write()
	defer close(c.out)
	if !c.authenticate() {
		return
	}
	s.connect(c)
	defer s.disconnect(c)
	for {
		msg, kind := c.read()
		if !s.handle(c, msg, kind) {
			return
		}
	}
}

// Only this goroutine writes to the socket. Closing the socket on write
// failure wakes the reader, which unregisters the device.
func (c *client) write() {
	defer c.conn.Close()
	for frame := range c.out {
		if c.conn.WriteJSON(frame) != nil {
			return
		}
	}
}

// While routing, a stalled socket must not hold up other devices.
func (c *client) send(frame any) {
	select {
	case c.out <- frame:
	default:
		c.conn.Close()
	}
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
func (c *client) read() (fields, string) {
	t, b, err := c.conn.ReadMessage()
	if err != nil || t != websocket.TextMessage || !utf8.Valid(b) || !c.admit(time.Now()) {
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

func (c *client) authenticate() bool {
	var b [16]byte
	rand.Read(b[:])
	nonce := base64.RawURLEncoding.EncodeToString(b[:])
	c.send(proto.Challenge{Type: "challenge", Nonce: nonce})
	m, kind := c.read()
	c.id = m.text("k")
	return kind == "auth" && identity.VerifyChallenge(c.id, nonce, m.text("s"))
}

func config(kind string) proto.ICEConfig {
	return proto.ICEConfig{Type: kind, ICE: []proto.ICEServer{}, TTL: 3600}
}

func (s *server) connect(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.online[c.id]
	s.online[c.id] = c
	c.send(config("ready"))
	if old != nil {
		s.leaveRooms(old)
		old.conn.Close()
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
		c.send(config("ice"))
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

func (s *server) route(c *client, m fields) bool {
	key := m.text("k")
	_, valid := identity.ParseDeviceID(key)
	if !valid || !m.object("env") {
		return false
	}
	if target := s.online[key]; target != nil {
		target.send(proto.Route{Type: "from", Key: c.id, Env: m["env"]})
	} else {
		c.send(proto.Offline{Type: "offline", Key: key})
	}
	return true
}

func (s *server) watch(c *client, m fields) bool {
	var keys []string
	if proto.Decode(m["k"], &keys) != nil || len(keys) > 64 {
		return false
	}
	c.watch = make(map[string]bool, len(keys))
	for _, key := range keys {
		if _, valid := identity.ParseDeviceID(key); !valid {
			return false
		}
		if !c.watch[key] {
			c.watch[key] = true
			c.send(proto.Presence{Type: "presence", Key: key, On: s.online[key] != nil})
		}
	}
	return true
}

func (s *server) notify(key string, online bool) {
	for _, c := range s.online {
		if c.watch[key] {
			c.send(proto.Presence{Type: "presence", Key: key, On: online})
		}
	}
}
