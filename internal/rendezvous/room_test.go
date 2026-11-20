package rendezvous

import (
	"encoding/json"
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
	send(t, c, join)
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

func join(s *server, c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.join(c, "first")
}

func TestRoomLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server{rooms: make(map[string]*room)}
		a := &client{}
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
		a, b := &client{}, &client{}
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
