// Package proto defines SoloDawn's JSON wire types.
package proto

// Envelope keeps the signed JSON text intact; parsing and re-encoding Payload
// before checking its signature would change the signed bytes.
type Envelope struct {
	Payload   string `json:"p"`
	Key       string `json:"k"`
	Signature string `json:"s"`
}

type PayloadHeader struct {
	Type      string `json:"t"`
	Nonce     string `json:"n"`
	Timestamp int64  `json:"ts"`
	Group     string `json:"g,omitempty"`
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
