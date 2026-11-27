package rendezvous

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestRoomMembership(t *testing.T) {
	url := fixture(t)
	a, b, c := login(t, url, key(1)), login(t, url, key(2)), login(t, url, key(3))
	id := identity.NewSecret().Room()
	join := proto.RoomID{Type: "join", Room: id}
	joined := proto.RoomID{Type: "joined", Room: id}
	full := proto.RoomID{Type: "full", Room: id}
	msg := proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: id}, Data: json.RawMessage(`{"UnKnown":null,"x":9007199254740993}`)}
	// An ice answer is an ordered barrier for requests on that socket.
	barrier := func(cn *websocket.Conn) {
		send(t, cn, proto.RVFrame{Type: "ice"})
		expect(t, cn, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600, Hosts: true})
	}
	send(t, a, msg)
	send(t, a, join)
	send(t, a, join)
	barrier(a)
	send(t, b, join)
	expect(t, b, joined)
	expect(t, a, joined)
	// A member that joins its full room again is told nothing; a third is.
	send(t, b, join)
	barrier(b)
	send(t, c, join)
	expect(t, c, full)
	send(t, c, msg)
	barrier(c)
	send(t, a, msg)
	expect(t, b, msg)
	barrier(a)
	send(t, b, msg)
	expect(t, a, msg)
	barrier(b)
	// A replacement connection does not inherit its predecessor's room.
	replacement := login(t, url, key(1))
	closed(t, a)
	send(t, replacement, msg)
	barrier(replacement)
	send(t, c, join)
	expect(t, b, joined)
	expect(t, c, joined)
	send(t, c, msg)
	expect(t, b, msg)
}

// after reports who is in the room "first" once d has passed: nobody when it is gone.
func after(s *server, d time.Duration) map[*client]bool {
	time.Sleep(d)
	synctest.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rooms["first"]; r != nil {
		return r.members
	}
	return nil
}

func newTestClient() *client {
	return &client{rooms: make(map[string]*room), hosted: make(map[string]*room), queued: make(map[string]int)}
}

func join(s *server, c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.join(c, "first")
}

func TestRoomLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server{rooms: make(map[string]*room)}
		a := newTestClient()
		join(&s, a)
		if got := after(&s, 15*time.Minute-time.Nanosecond); !reflect.DeepEqual(got, map[*client]bool{a: true}) {
			t.Fatalf("room before its 15 minutes: %v", got)
		}
		if got := after(&s, time.Nanosecond); got != nil || len(s.rooms) != 0 {
			t.Fatalf("room after 15 minutes: %v", got)
		}
		join(&s, a)
		s.leaveRooms(a)
		if len(s.rooms) != 0 {
			t.Fatal("empty room retained")
		}
	})
}

// An emptied room stops its timer. One that had already fired still runs, and
// must leave a newer room under the same id alone.
func TestLateExpiryLeavesANewerRoom(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server{rooms: make(map[string]*room)}
		a, b := newTestClient(), newTestClient()
		join(&s, a)
		first := s.rooms["first"]
		s.leaveRooms(a)
		if first.timer.Stop() {
			t.Fatal("the timer of an emptied room was left running")
		}
		time.Sleep(time.Minute)
		join(&s, b)
		s.expire("first", first)
		if got := after(&s, 15*time.Minute-time.Nanosecond); !reflect.DeepEqual(got, map[*client]bool{b: true}) {
			t.Fatalf("second room before its 15 minutes: %v", got)
		}
		if got := after(&s, time.Nanosecond); got != nil {
			t.Fatalf("second room after 15 minutes: %v", got)
		}
	})
}

// A connection is in at most 16 rooms; a join of a 17th is bad input. A room
// it is already in does not count again.
func TestRoomsPerConnection(t *testing.T) {
	c := login(t, fixture(t), key(1))
	for i := range 16 {
		send(t, c, proto.RoomID{Type: "join", Room: fmt.Sprintf("%032x", i)})
	}
	send(t, c, proto.RoomID{Type: "join", Room: fmt.Sprintf("%032x", 0)})
	send(t, c, proto.RVFrame{Type: "ice"})
	expect(t, c, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600, Hosts: true})
	send(t, c, proto.RoomID{Type: "join", Room: fmt.Sprintf("%032x", 16)})
	closed(t, c)
}

// A room that expires frees its place among the connection's 16.
func TestExpiredRoomFreesItsPlace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server{rooms: make(map[string]*room)}
		a, b := newTestClient(), newTestClient()
		joinAll := func(c *client, from, to int) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			for i := from; i < to; i++ {
				if !s.join(c, fmt.Sprintf("%032x", i)) {
					return false
				}
			}
			return true
		}
		if !joinAll(a, 0, 16) || !joinAll(b, 15, 16) || joinAll(a, 16, 17) {
			t.Fatal("the 17th room was not refused")
		}
		time.Sleep(15 * time.Minute)
		synctest.Wait()
		if len(a.rooms) != 0 || len(b.rooms) != 0 || len(s.rooms) != 0 {
			t.Fatalf("after expiry: %d, %d rooms of connections, %d of the server", len(a.rooms), len(b.rooms), len(s.rooms))
		}
		if !joinAll(a, 16, 32) {
			t.Fatal("expired rooms still counted")
		}
	})
}

// hostedRoom holds the frames of one room that has a host.
type hostedRoom struct {
	t                               *testing.T
	id                              string
	host, leave, join, joined, full proto.RoomID
	card                            proto.RoomData
}

func newHostedRoom(t *testing.T) hostedRoom {
	id := identity.NewSecret().Room()
	return hostedRoom{
		t: t, id: id,
		leave:  proto.RoomID{Type: "leave", Room: id},
		join:   proto.RoomID{Type: "join", Room: id},
		joined: proto.RoomID{Type: "joined", Room: id},
		full:   proto.RoomID{Type: "full", Room: id},
		card:   proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: id}, Data: json.RawMessage(`{"UnKnown":null,"x":9007199254740993}`)},
	}
}

func (h hostedRoom) hosting() proto.RoomData {
	return proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: h.id}, Data: h.card.Data}
}

// pair is the room frame of a guest, which must reach the host.
func (h hostedRoom) pair(n int) proto.RoomData {
	return proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: h.id}, Data: json.RawMessage(fmt.Sprintf(`{"guest":%d}`, n))}
}

// An ice answer is an ordered barrier for requests on that socket: what was
// due before it came before it.
func barrier(t *testing.T, c *websocket.Conn) {
	t.Helper()
	send(t, c, proto.RVFrame{Type: "ice"})
	expect(t, c, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600, Hosts: true})
}

// greeted holds that a device was told it met the host, and was given its card.
func (h hostedRoom) greeted(c *websocket.Conn) {
	h.t.Helper()
	expect(h.t, c, h.joined)
	expect(h.t, c, h.card)
}

// unanswered holds that a device that joins waits as in an empty room, and
// that the next one pairs with it.
func (h hostedRoom) unanswered(a, b *websocket.Conn) {
	h.t.Helper()
	send(h.t, a, h.join)
	barrier(h.t, a)
	send(h.t, b, h.join)
	expect(h.t, a, h.joined)
	expect(h.t, b, h.joined)
}

func TestHostedRoomGuests(t *testing.T) {
	url := fixture(t)
	h := newHostedRoom(t)
	host := login(t, url, key(1))
	send(t, host, h.hosting())
	barrier(t, host) // The host gets no joined.
	guests := []*websocket.Conn{login(t, url, key(2)), login(t, url, key(3)), login(t, url, key(4))}
	// One after another: each is greeted and its room frame reaches the host.
	for i, g := range guests {
		send(t, g, h.join)
		h.greeted(g)
		send(t, g, h.pair(i))
		expect(t, host, h.pair(i))
	}
	h.atOnce(host, guests)
	// The host's own room frame goes to nobody, and a second host is refused.
	send(t, host, h.card)
	second := login(t, url, key(5))
	send(t, second, h.hosting())
	expect(t, second, h.full)
	send(t, host, h.hosting())
	expect(t, host, h.full)
	for _, g := range append(guests, second) {
		barrier(t, g)
	}
}

// atOnce joins all guests before any answer is read: all are greeted, and
// all their room frames reach the host, in some order.
func (h hostedRoom) atOnce(host *websocket.Conn, guests []*websocket.Conn) {
	h.t.Helper()
	for _, g := range guests {
		send(h.t, g, h.join)
	}
	want := map[string]bool{}
	for i, g := range guests {
		h.greeted(g)
		send(h.t, g, h.pair(i))
		b, _ := json.Marshal(h.pair(i))
		want[string(b)] = true
	}
	got := map[string]bool{}
	for range guests {
		_, b, err := read(h.t, host)
		if err != nil {
			h.t.Fatal(err)
		}
		got[string(b)] = true
	}
	if !reflect.DeepEqual(got, want) {
		h.t.Fatalf("the host received %v, want %v", got, want)
	}
}

func TestHostedRoomWaitingDevices(t *testing.T) {
	url := fixture(t)
	h := newHostedRoom(t)
	a, b, host := login(t, url, key(1)), login(t, url, key(2)), login(t, url, key(3))
	send(t, a, h.join)
	barrier(t, a)
	send(t, b, h.join)
	expect(t, a, h.joined)
	expect(t, b, h.joined)
	send(t, host, h.hosting())
	h.greeted(a)
	h.greeted(b)
	// Both are out of the room: their room frames go to the host alone.
	send(t, a, h.pair(1))
	expect(t, host, h.pair(1))
	send(t, b, h.pair(2))
	expect(t, host, h.pair(2))
	barrier(t, a)
	barrier(t, b)
}

func TestHostedRoomEnds(t *testing.T) {
	url := fixture(t)
	h := newHostedRoom(t)
	host, a, b := login(t, url, key(1)), login(t, url, key(2)), login(t, url, key(3))
	send(t, host, h.hosting())
	barrier(t, host) // The room has its host before a's frames are read.
	// A leave by a device that is not the host changes nothing.
	send(t, a, h.leave)
	send(t, a, h.join)
	h.greeted(a)
	// After the host's leave the room is as an empty one.
	send(t, host, h.leave)
	barrier(t, host)
	h.unanswered(a, b)
	// After the end of the host's connection too, which a watcher of the host
	// is told of once its rooms are gone.
	h = newHostedRoom(t)
	send(t, host, h.hosting())
	send(t, a, proto.Watch{Type: "watch", Keys: []string{key(1).ID()}})
	expect(t, a, proto.Presence{Type: "presence", Key: key(1).ID(), On: true})
	host.CloseNow()
	expect(t, a, proto.Presence{Type: "presence", Key: key(1).ID(), On: false})
	h.unanswered(a, b)
	// A connection that replaces the host's ends its rooms too.
	h = newHostedRoom(t)
	host = login(t, url, key(1))
	expect(t, a, proto.Presence{Type: "presence", Key: key(1).ID(), On: true})
	send(t, host, h.hosting())
	barrier(t, host)
	login(t, url, key(1))
	closed(t, host)
	h.unanswered(a, b)
}

// A connection hosts at most 16 rooms, beside the 16 it is in. The 17th
// hosting is answered full and the connection stays; a room left frees its
// place.
func TestRoomsHostedPerConnection(t *testing.T) {
	url := fixture(t)
	c := login(t, url, key(1))
	room := func(i int) string { return fmt.Sprintf("%032x", i) }
	for i := range 16 {
		send(t, c, proto.RoomID{Type: "join", Room: room(i)})
		send(t, c, proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: room(100 + i)}, Data: json.RawMessage(`{}`)})
	}
	barrier(t, c)
	send(t, c, proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: room(116)}, Data: json.RawMessage(`{}`)})
	expect(t, c, proto.RoomID{Type: "full", Room: room(116)})
	send(t, c, proto.RoomID{Type: "leave", Room: room(100)})
	send(t, c, proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: room(116)}, Data: json.RawMessage(`{}`)})
	barrier(t, c)
	send(t, c, proto.RoomID{Type: "join", Room: room(16)})
	closed(t, c)
}

// A hosted room has no 15 minutes. The timer of the room it replaced is
// stopped, and one that had already fired leaves the hosted room alone.
func TestHostedRoomLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server{rooms: make(map[string]*room)}
		a, host := newTestClient(), newTestClient()
		join(&s, a)
		waiting := s.rooms["first"]
		s.mu.Lock()
		s.host(host, "first", json.RawMessage(`{}`))
		s.mu.Unlock()
		if waiting.timer.Stop() {
			t.Fatal("the timer of the room that got a host was left running")
		}
		if len(a.rooms) != 0 {
			t.Fatalf("a waiting device kept the room: %v", a.rooms)
		}
		s.expire("first", waiting)
		if got := after(&s, time.Hour); !reflect.DeepEqual(got, map[*client]bool{host: true}) {
			t.Fatalf("hosted room after an hour: %v", got)
		}
		s.leaveRooms(host)
		if len(s.rooms) != 0 {
			t.Fatal("a hosted room outlived its host")
		}
	})
}
