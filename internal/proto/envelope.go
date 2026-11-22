// Package proto defines SoloDawn's JSON wire types.
package proto

import (
	"crypto/rand"
	"encoding/base64"
)

// Envelope keeps the signed JSON text intact; parsing and re-encoding Payload
// before checking its signature would change the signed bytes.
type Envelope struct {
	Payload   string `json:"p"`
	Key       string `json:"k"`
	Signature string `json:"s"`
}

// ID is a random id: 16 bytes in their canonical base64url spelling, 22
// characters (contracts/envelope.md section 1). Decode refuses any other text
// in a field of this type.
type ID string

// NewNonce returns a random 16-byte ID in canonical base64url spelling.
func NewNonce() ID {
	var b [16]byte
	rand.Read(b[:])
	return ID(base64.RawURLEncoding.EncodeToString(b[:]))
}

type PayloadHeader struct {
	Type      string `json:"t"`
	Nonce     ID     `json:"n"`
	Timestamp int64  `json:"ts"`
	Group     ID     `json:"g,omitempty"`
	To        string `json:"to,omitempty"`
}

type SignalHeader struct {
	PayloadHeader
	Conn string `json:"conn"`
}

type Offer struct {
	SignalHeader
	SDP   string `json:"sdp"`
	Probe bool   `json:"probe"`
}

type Answer struct {
	SignalHeader
	SDP string `json:"sdp"`
}

type ICECandidate struct {
	SignalHeader
	Candidate string `json:"cand"`
	MID       string `json:"mid"`
}

type Pair struct {
	Type string `json:"t"`
	Key  string `json:"k"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	Role string `json:"role"`
	MAC  string `json:"mac"`
}

type DeviceChange struct {
	User    string `json:"user"`
	Version int    `json:"ver"`
	Op      string `json:"op"`
	Device  string `json:"dev"`
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name,omitempty"`
}

type DeviceEntry struct {
	PayloadHeader
	DeviceChange
}
