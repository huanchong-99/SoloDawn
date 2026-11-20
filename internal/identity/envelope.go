package identity

import (
	"encoding/json"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Code is the error code of a refused envelope, as it goes on the wire.
type Code string

func (c Code) Error() string { return string(c) }

const (
	BadSig        Code = "bad_sig"
	UnknownDevice Code = "unknown_device"
	NotForMe      Code = "not_for_me"
	Stale         Code = "stale"
	BadRequest    Code = "bad_request" // the signed payload is not what the contract describes
)

// OpenSignal applies the receiver's checks of contracts/envelope.md section 3
// to a signalling payload, whose ts must be within two minutes of the
// receiver's clock. It returns the payload's common fields, for the caller to
// parse env.Payload by its type, or the code of the first check that fails. me
// is the receiver's device id. known reports whether a device is a currently
// valid device of someone the receiver listens to: only the caller knows its
// owner, its groups and its pairing peer.
func OpenSignal(env proto.Envelope, me string, known func(device string) bool, now time.Time) (proto.PayloadHeader, error) {
	return open(env, me, known, now, 2*time.Minute)
}

// OpenCommand is OpenSignal for a command, whose ts must be within 24 hours.
func OpenCommand(env proto.Envelope, me string, known func(device string) bool, now time.Time) (proto.PayloadHeader, error) {
	return open(env, me, known, now, 24*time.Hour)
}

// open makes the checks in the contract's order; window is how far ts may be
// from now, in either direction.
func open(env proto.Envelope, me string, known func(device string) bool, now time.Time, window time.Duration) (proto.PayloadHeader, error) {
	if !verify(env.Key, envelopeContext+env.Payload, env.Signature) {
		return proto.PayloadHeader{}, BadSig
	}
	if !known(env.Key) {
		return proto.PayloadHeader{}, UnknownDevice
	}
	return addressed(env.Payload, me, now.UnixMilli(), window.Milliseconds())
}

func addressed(p, me string, now, window int64) (proto.PayloadHeader, error) {
	var h, none proto.PayloadHeader
	if json.Unmarshal([]byte(p), &h) != nil {
		return none, BadRequest
	}
	if h.To != "" && h.To != me {
		return none, NotForMe
	}
	if age := now - h.Timestamp; age > window || age < -window {
		return none, Stale
	}
	return h, nil
}
