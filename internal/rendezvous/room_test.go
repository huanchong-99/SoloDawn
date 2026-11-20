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
		expect(t, cn, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600})
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
	return &client{rooms: make(map[string]*room), queued: make(map[string]int)}
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
	expect(t, c, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600})
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
