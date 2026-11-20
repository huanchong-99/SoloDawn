package identity

import (
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
	errLast    = errors.New("device log: the last valid device is never revoked")
)

// Log is the device log of one person: the entries accepted so far and the
// valid devices that follow from them (contracts/envelope.md section 4).
type Log struct {
	User    string           // the person: the device id of their first device
	Entries []proto.Envelope // exactly as signed; version v is Entries[v-1]
	changes []logEntry
}

// logEntry is what is read of an entry's payload, and nothing else: not n or
// ts, and kind and name only on an add.
type logEntry struct {
	Type    string `json:"t"`
	User    string `json:"user"`
	Version int    `json:"ver"`
	Op      string `json:"op"`
	Device  string `json:"dev"`
}

// addition is what an add carries besides.
type addition struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
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
// between devices, so the same entry arrives more than once. A refused entry
// leaves the log as it was.
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
	l.changes = append(l.changes, e)
	return true, nil
}

// entry reads a signed device-log entry: rules 1 and 2 of section 4, except
// whose log it is. Log entries are replayed long after they were written and
// are addressed to nobody, so of the envelope checks only the signature applies.
func entry(env proto.Envelope) (logEntry, error) {
	var e logEntry
	if !verify(env.Key, envelopeContext+env.Payload, env.Signature) {
		return e, BadSig
	}
	if proto.Decode([]byte(env.Payload), &e) != nil || !isEntry(e, env.Payload) {
		return e, BadRequest
	}
	return e, nil
}

// isEntry reports whether a payload is an add or a revoke of a device id.
func isEntry(e logEntry, p string) bool {
	return e.Type == "device" && isDevice(e.Device) && (e.Op == "revoke" || e.Op == "add" && isAdd(p))
}

// isAdd checks what only an add carries: a kind and a name.
func isAdd(p string) bool {
	var a addition
	return proto.Decode([]byte(p), &a) == nil && (a.Kind == "node" || a.Kind == "browser")
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
func (l *Log) follows(e logEntry, signer string) error {
	if len(l.Entries) == 0 {
		return l.genesis(e, signer)
	}
	if !l.Valid(signer) {
		return UnknownDevice
	}
	return l.allows(e)
}

// allows is rule 6: an add does not name a device that was ever revoked, and
// a revoke does not remove the last valid device.
func (l *Log) allows(e logEntry) error {
	if e.Op == "add" && l.last(e.Device) == "revoke" {
		return errRevoked
	}
	if e.Op == "revoke" && l.alone(e.Device) {
		return errLast
	}
	return nil
}

// genesis checks entry 1: dev, k and user are the same key.
func (l *Log) genesis(e logEntry, signer string) error {
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

// alone reports whether no other device is valid. The signer of an entry is
// valid, so a device that is alone is the signer itself, and the last one.
func (l *Log) alone(device string) bool {
	for _, c := range l.changes {
		if c.Device != device && l.Valid(c.Device) {
			return false
		}
	}
	return true
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
