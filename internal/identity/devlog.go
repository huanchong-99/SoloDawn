package identity

import (
	"encoding/json"
	"errors"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Why an entry is refused, beyond the envelope codes.
var (
	errPerson  = errors.New("device log: the entry belongs to another person's log")
	errGenesis = errors.New("device log: entry 1 must add its own signer, the person's first device")
	errGap     = errors.New("device log: the version does not follow the log")
	errFork    = errors.New("device log: a different entry already holds this version")
	errRevoked = errors.New("device log: a revoked device is never added again")
)

// Log is the device log of one person: the entries accepted so far and the
// valid devices that follow from them (contracts/envelope.md section 4).
type Log struct {
	User    string           // the person: the device id of their first device
	Entries []proto.Envelope // exactly as signed; version v is Entries[v-1]
	changes []proto.DeviceChange
}

// Replay verifies a whole log of user from entry 1.
func Replay(user string, entries []proto.Envelope) (*Log, error) {
	l := &Log{User: user}
	for _, env := range entries {
		if _, err := l.Apply(env); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// Apply checks one signed entry against the log and appends it. added is
// false, with no error, for an entry the log already holds: logs are passed on
// between devices, so the same entry arrives more than once.
func (l *Log) Apply(env proto.Envelope) (added bool, err error) {
	e, err := entry(env)
	if err != nil {
		return false, err
	}
	if e.User != l.User {
		return false, errPerson
	}
	if e.Version != len(l.Entries)+1 {
		return false, l.held(e.Version, env)
	}
	if err := l.follows(e, env.Key); err != nil {
		return false, err
	}
	l.Entries = append(l.Entries, env)
	l.changes = append(l.changes, e.DeviceChange)
	return true, nil
}

// entry reads a signed device-log entry. Log entries are replayed long after
// they were written and are addressed to nobody, so of the envelope checks
// only the signature applies here.
func entry(env proto.Envelope) (proto.DeviceEntry, error) {
	var e proto.DeviceEntry
	if !verify(env.Key, envelopeContext+env.Payload, env.Signature) {
		return e, BadSig
	}
	if json.Unmarshal([]byte(env.Payload), &e) != nil || e.Type != "device" || e.Op != "add" && e.Op != "revoke" {
		return e, BadRequest
	}
	return e, nil
}

// held answers an entry whose version is not the next one. The first entry
// accepted for a version wins; the same entry again is not an error.
func (l *Log) held(version int, env proto.Envelope) error {
	if version < 1 || version > len(l.Entries) {
		return errGap
	}
	if l.Entries[version-1] != env {
		return errFork
	}
	return nil
}

// follows checks the entry for the next version against the log before it.
func (l *Log) follows(e proto.DeviceEntry, signer string) error {
	if len(l.Entries) == 0 {
		return l.genesis(e, signer)
	}
	if !l.Valid(signer) {
		return UnknownDevice
	}
	if e.Op == "add" && l.last(e.Device) == "revoke" {
		return errRevoked
	}
	return nil
}

// genesis checks entry 1: dev, k and user are the same key.
func (l *Log) genesis(e proto.DeviceEntry, signer string) error {
	if e.Op != "add" || e.Device != l.User || signer != l.User {
		return errGenesis
	}
	return nil
}

// Valid reports whether a device is in the valid set.
func (l *Log) Valid(device string) bool {
	return l.last(device) == "add"
}

// last returns the op of the latest entry about a device, or "". A revoke
// stays the latest: no add is accepted after it.
func (l *Log) last(device string) string {
	op := ""
	for _, c := range l.changes {
		if c.Device == device {
			op = c.Op
		}
	}
	return op
}

// Adopts reports whether a device that belongs to no person takes this log's
// person as its owner (contracts/envelope.md section 6): the log adds the
// device in an entry signed by the peer it paired with, and both are valid at
// the end of the log. Whether the device is free to adopt, and on which
// connection the log arrived, is for the caller to know.
func (l *Log) Adopts(me, peer string) bool {
	for i, c := range l.changes {
		if c.Op == "add" && c.Device == me && l.Entries[i].Key == peer {
			return l.Valid(me) && l.Valid(peer)
		}
	}
	return false
}
