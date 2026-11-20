package rendezvous

import (
	"regexp"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var roomID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type room struct {
	members map[*client]bool
	timer   *time.Timer
}

func (s *server) roomFrame(c *client, m fields, kind string) bool {
	id := m.text("room")
	if !roomID.MatchString(id) {
		return false
	}
	if kind == "join" {
		return s.join(c, id)
	}
	if !m.object("data") {
		return false
	}
	if r := c.rooms[id]; r != nil {
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

// maxRooms is how many rooms one connection may be in at a time.
const maxRooms = 16

// join reports false for a join of a room beyond maxRooms, which is bad input.
// Joining a room the connection is already in changes nothing; a third device
// is told that the room is full.
func (s *server) join(c *client, id string) bool {
	if c.rooms[id] != nil {
		return true
	}
	if len(c.rooms) == maxRooms {
		return false
	}
	r := s.open(id)
	if len(r.members) == 2 {
		c.send(proto.RoomID{Type: "full", Room: id})
		return true
	}
	r.enter(c, id)
	return true
}

// enter adds a connection to a room with space; the second tells both.
func (r *room) enter(c *client, id string) {
	r.members[c] = true
	c.rooms[id] = r
	if len(r.members) == 2 {
		for member := range r.members {
			member.send(proto.RoomID{Type: "joined", Room: id})
		}
	}
}

// open returns the room of an id, creating it with its 15 minutes.
func (s *server) open(id string) *room {
	r := s.rooms[id]
	if r == nil {
		r = &room{members: make(map[*client]bool)}
		s.rooms[id] = r
		r.timer = time.AfterFunc(15*time.Minute, func() { s.expire(id, r) })
	}
	return r
}

// A timer that fired while its room emptied was too late to stop, and the id
// may by then name a newer room, which has its own 15 minutes.
func (s *server) expire(id string, r *room) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[id] == r {
		delete(s.rooms, id)
		for member := range r.members {
			delete(member.rooms, id)
		}
	}
}

// Each connection knows its rooms, so leaving does not scan every room.
func (s *server) leaveRooms(c *client) {
	for id, r := range c.rooms {
		delete(r.members, c)
		if len(r.members) == 0 {
			r.timer.Stop()
			delete(s.rooms, id)
		}
	}
}
