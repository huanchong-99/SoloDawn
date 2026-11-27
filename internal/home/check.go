package home

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Who may send a command: its envelope, and the principal behind its signer
// (planning/contracts/commands.md, "Who"; planning/contracts/datachannel.md
// sections 5 and 8).

// keyCreate is the key of the one command for a group that does not exist yet.
const keyCreate = "group/create"

// keySelector names the field of a payload that, with its t, makes the key of
// a command.
var keySelector = map[string]string{"group": "op", "member": "op", "task": "op", "post": "kind", "act": "action", "code": "op"}

// Who is the principal behind a command. A signer without as is the person
// whose log holds it and, where the table of a command names a host, also
// that host: the function asks for the one its table names.
type Who struct {
	Device string // the signer
	User   string // the person whose device the signer is
	Human  string // that person's member id; empty only in group create
	AI     string // as: the AI member that the signer hosts and signed for; empty for a person or a host
}

// Member returns the member a command is by: the AI its host signed for,
// else the person.
func (w Who) Member() string {
	if w.AI != "" {
		return w.AI
	}
	return w.Human
}

// Admin reports whether a command is by a person with role owner or admin.
func (s *State) Admin(w Who) bool {
	role := s.People[w.Human].Role
	return w.AI == "" && (role == "owner" || role == "admin")
}

// Secretary reports whether a command is by the group's secretary.
func (s *State) Secretary(w Who) bool {
	return w.AI != "" && w.AI == s.Group.Secretary
}

// Hosts reports whether a command is by the host of that AI member, speaking
// for itself.
func (s *State) Hosts(w Who, ai string) bool {
	return w.AI == "" && s.AIs[ai].Host == w.Device
}

// Serves reports whether the home surface of the group is open to a device
// (planning/contracts/datachannel.md section 8): a device of this node's
// owner, or of a person who is a member, which the host of an AI member is.
func (s *State) Serves(device string) bool {
	user := s.user(device)
	return user != "" && (user == s.Owner || s.human(user) != "")
}

// open makes the checks that come before the receipt and leave none: the
// command's envelope passes (planning/contracts/envelope.md section 3), and
// it names this group.
func (s *State) open(env proto.Envelope, join *Joining, now time.Time) (proto.PayloadHeader, *Refusal) {
	hdr, err := identity.OpenCommand(env, s.Home, s.seamKnown(join), now)
	if err != nil {
		return hdr, &Refusal{err.Error(), "the envelope of the command was refused"}
	}
	if string(hdr.Group) != s.ID {
		return hdr, &Refusal{"bad_request", "the command does not name this group in g"}
	}
	return hdr, nil
}

// known is check 2 of the envelope: a valid device of someone the group
// listens to.
func (s *State) known(device string) bool { return s.user(device) != "" }

// user returns the person a device belongs to, the one whose log holds it as
// a valid device, or "". Should two logs hold it, the lower user id wins.
func (s *State) user(device string) string {
	for _, user := range slices.Sorted(maps.Keys(s.Logs)) {
		if s.Logs[user].Valid(device) {
			return user
		}
	}
	return ""
}

// human returns the member id of a person, or "".
func (s *State) human(user string) string {
	for id, m := range s.People {
		if m.User == user {
			return id
		}
	}
	return ""
}

// find returns the function of a command and fills in who sent it.
func (t Table) find(s *State, c *Command) (Func, *Refusal) {
	key, named := c.key()
	f := t[key]
	if !named || f == nil {
		return nil, &Refusal{"bad_request", "the home has no such command"}
	}
	return f, s.seamFindWho(key, c)
}

// who names the principal behind a signer and the as of its payload. A home
// accepts as only from the host of that AI (planning/contracts/envelope.md
// section 5).
func (s *State) who(device, as string) (Who, *Refusal) {
	user := s.user(device)
	w := Who{Device: device, User: user, Human: s.human(user), AI: as}
	if as != "" && s.AIs[as].Host != device {
		return w, &Refusal{"forbidden", "as names no AI that the signer hosts"}
	}
	if !s.member(w) {
		return w, &Refusal{"forbidden", "the signer is no device of a member of the group"}
	}
	return w, nil
}

// member reports whether a person may send commands to the group: a member
// of it, or this node's owner while the group does not exist.
func (s *State) member(w Who) bool {
	if s.Group.ID == "" {
		return w.User == s.Owner
	}
	return w.Human != ""
}

// key is where the table holds the function of a command. named is false
// for a payload that names none. The "/" of a key is made here and nowhere
// else: a t or a selector field that holds one names nothing, and a type
// with a selector needs that field.
func (c Command) key() (key string, named bool) {
	name, selected := keySelector[c.Type]
	if !selected {
		return c.Type, !strings.Contains(c.Type, "/")
	}
	sub, ok := c.field(name)
	return c.Type + "/" + sub, ok && sub != "" && !strings.Contains(sub, "/")
}

// field reads a text of the payload that may be left out: "" when it is. ok
// is false for anything in its place but a text that is not empty.
func (c Command) field(name string) (text string, ok bool) {
	var members map[string]json.RawMessage
	proto.Decode([]byte(c.Env.Payload), &members) // an object: the envelope check has read it
	raw, there := members[name]
	if !there {
		return "", true
	}
	err := json.Unmarshal(raw, &text)
	return text, err == nil && text != ""
}
