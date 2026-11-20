package rendezvous

import (
	"regexp"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var roomID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type room struct {
	members map[*client]bool
}

func (s *server) roomFrame(c *client, m fields, kind string) bool {
	id := m.text("room")
	if !roomID.MatchString(id) {
		return false
	}
	if kind == "join" {
		s.join(c, id)
		return true
	}
	if !m.object("data") {
		return false
	}
	if r := s.rooms[id]; r != nil && r.members[c] {
		r.forward(c, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: id}, Data: m["data"]})
	}
	return true
}

func (r *room) forward(sender *client, msg proto.RoomData) {
	for member := range r.members {
		if member != sender {
			member.send(msg)
		}
	}
}

func (s *server) join(c *client, id string) {
	r := s.rooms[id]
	if r == nil {
		r = &room{members: make(map[*client]bool)}
		s.rooms[id] = r
		time.AfterFunc(15*time.Minute, func() { s.expire(id, r) })
	}
	if len(r.members) == 2 {
		return
	}
	r.members[c] = true
	if len(r.members) == 2 {
		for member := range r.members {
			member.send(proto.RoomID{Type: "joined", Room: id})
		}
	}
}

// A room that emptied is gone before its 15 minutes are over, and its id may
// by then name a newer room, which has its own 15 minutes.
func (s *server) expire(id string, r *room) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[id] == r {
		delete(s.rooms, id)
	}
}

func (s *server) leaveRooms(c *client) {
	for id, r := range s.rooms {
		delete(r.members, c)
		if len(r.members) == 0 {
			delete(s.rooms, id)
		}
	}
}
