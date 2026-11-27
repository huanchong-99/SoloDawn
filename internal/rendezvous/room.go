package rendezvous

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var roomID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// A hosted room has data, the card of its host, and the host as its only
// member; it has no timer. A pairing room has no data.
type room struct {
	members map[*client]bool
	timer   *time.Timer
	data    json.RawMessage
}

func (s *server) roomFrame(c *client, m fields, kind string) bool {
	id := m.text("room")
	if !roomID.MatchString(id) {
		return false
	}
	if kind == "leave" {
		s.leave(c, id)
		return true
	}
	if !m.object("data") {
		return kind == "join" && s.join(c, id)
	}
	if kind == "join" {
		s.host(c, id, m["data"])
	} else {
		s.pass(c, id, m["data"])
	}
	return true
}

// A member's room frame goes to the other member; any device's room frame
// for a hosted room goes to its host.
func (s *server) pass(c *client, id string, data json.RawMessage) {
	if r := s.rooms[id]; r != nil && (r.data != nil || r.members[c]) {
		r.forward(c, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: id}, Data: data})
	}
}

func (r *room) forward(sender *client, msg proto.RoomData) {
	for member := range r.members {
		if member != sender {
			member.sendFrom(sender.id, msg)
		}
	}
}

// maxRooms is how many rooms one connection may be in at a time, and how
// many it may host beside them.
const maxRooms = 16

// join reports false for a join of a room beyond maxRooms, which is bad input.
// Joining a room the connection is already in changes nothing; a third device
// is told that the room is full. A device that joins a hosted room gets its
// card and is not kept in it.
func (s *server) join(c *client, id string) bool {
	if c.rooms[id] != nil {
		return true
	}
	if len(c.rooms) == maxRooms {
		return false
	}
	r := s.open(id)
	switch {
	case r.data != nil:
		r.greet(c, id)
	case len(r.members) == 2:
		c.send(proto.RoomID{Type: "full", Room: id})
	default:
		r.enter(c, id)
	}
	return true
}

// host makes a connection the host of a room. A room that has a host, and a
// 17th room for one connection, are full. The devices waiting in the room get
// the card and are out of it; the new room object leaves a late timer of the
// old one nothing to delete.
func (s *server) host(c *client, id string, data json.RawMessage) {
	old := s.rooms[id]
	if old != nil && old.data != nil || len(c.hosted) == maxRooms {
		c.send(proto.RoomID{Type: "full", Room: id})
		return
	}
	r := &room{members: map[*client]bool{c: true}, data: data}
	s.rooms[id], c.hosted[id] = r, r
	if old != nil {
		old.timer.Stop()
		for waiting := range old.members {
			delete(waiting.rooms, id)
			r.greet(waiting, id)
		}
	}
}

// greet tells a device that it met a hosted room, and gives it the card.
func (r *room) greet(c *client, id string) {
	c.send(proto.RoomID{Type: "joined", Room: id})
	c.send(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: id}, Data: r.data})
}

// leave ends a room that the connection hosts, and nothing else.
func (s *server) leave(c *client, id string) {
	if c.hosted[id] != nil {
		delete(c.hosted, id)
		delete(s.rooms, id)
	}
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

// Each connection knows its rooms, so leaving does not scan every room. The
// rooms it hosts end with it.
func (s *server) leaveRooms(c *client) {
	for id, r := range c.rooms {
		delete(r.members, c)
		if len(r.members) == 0 {
			r.timer.Stop()
			delete(s.rooms, id)
		}
	}
	for id := range c.hosted {
		delete(s.rooms, id)
	}
}
