package rendezvous

import (
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
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

func TestRoomLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server{rooms: make(map[string]*room)}
		a, b := &client{}, &client{}
		s.join(a, "first")
		time.Sleep(15*time.Minute - time.Nanosecond)
		synctest.Wait()
		s.mu.Lock()
		stillHere := s.rooms["first"] != nil && s.rooms["first"].members[a]
		s.mu.Unlock()
		if !stillHere {
			t.Fatal("room expired early")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if len(s.rooms) != 0 {
			t.Fatal("room survived 15 minutes")
		}
		s.join(b, "first")
		s.leaveRooms(b)
		if len(s.rooms) != 0 {
			t.Fatal("empty room retained")
		}
	})
}
