package identity

import (
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Code is the error code of a refused envelope, as it goes on the wire.
type Code string

func (c Code) Error() string { return string(c) }

const (
	BadSig        Code = "bad_sig"
	UnknownDevice Code = "unknown_device"
	BadRequest    Code = "bad_request"
	NotForMe      Code = "not_for_me"
	Stale         Code = "stale"
)

// OpenSignal applies the receiver's checks of contracts/envelope.md section 3
// to a signalling payload: it must name the receiver in to, and its ts must be
// within two minutes of the receiver's clock. It returns the payload's common
// fields, for the caller to read env.Payload by its type with proto.Decode, or
// the code of the first check that fails. me is the receiver's device id.
// known reports whether a device is a currently valid device of someone the
// receiver listens to: only the caller knows its owner, its groups and its
// pairing peer.
func OpenSignal(env proto.Envelope, me string, known func(device string) bool, now time.Time) (proto.PayloadHeader, error) {
	return open(env, me, known, now, 2*time.Minute, true)
}

// OpenCommand is OpenSignal for a command: its ts must be within 24 hours, and
// it may have no to. A command that must name its receiver is refused by the
// node that handles it.
func OpenCommand(env proto.Envelope, me string, known func(device string) bool, now time.Time) (proto.PayloadHeader, error) {
	return open(env, me, known, now, 24*time.Hour, false)
}

// common is what is read of every payload. To is a pointer because an empty
// to is a value, and names nobody's device: only a payload without to is for
// any receiver.
type common struct {
	Type  string   `json:"t"`
	Nonce proto.ID `json:"n"`
	Time  int64    `json:"ts"`
	Group proto.ID `json:"g,omitempty"`
	To    *string  `json:"to,omitempty"`
}

// open makes the checks in the contract's order. window is how far ts may be
// from now, in either direction; mustName says that the payload must carry to.
func open(env proto.Envelope, me string, known func(device string) bool, now time.Time, window time.Duration, mustName bool) (proto.PayloadHeader, error) {
	if !verify(env.Key, envelopeContext+env.Payload, env.Signature) {
		return proto.PayloadHeader{}, BadSig
	}
	if !known(env.Key) {
		return proto.PayloadHeader{}, UnknownDevice
	}
	var c common
	if proto.Decode([]byte(env.Payload), &c) != nil {
		return proto.PayloadHeader{}, BadRequest
	}
	return c.received(me, mustName, now.UnixMilli(), window.Milliseconds())
}

// received makes the two checks that read the payload: it is for this
// receiver, and it was signed close enough to now.
func (c common) received(me string, mustName bool, now, window int64) (proto.PayloadHeader, error) {
	to, mine := addressed(c.To, me, mustName)
	if !mine {
		return proto.PayloadHeader{}, NotForMe
	}
	if age := now - c.Time; age > window || age < -window {
		return proto.PayloadHeader{}, Stale
	}
	return proto.PayloadHeader{Type: c.Type, Nonce: c.Nonce, Timestamp: c.Time, Group: c.Group, To: to}, nil
}

// addressed reports whether a payload is for this receiver, and the to it carries.
func addressed(to *string, me string, mustName bool) (string, bool) {
	if to == nil {
		return "", !mustName
	}
	return *to, *to == me
}
