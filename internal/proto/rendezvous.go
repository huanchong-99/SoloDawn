package proto

import "encoding/json"

// WebSocket signalling frames (contracts/rendezvous.md section 2).
type RVFrame struct {
	Type string `json:"t"`
}

type Quota struct {
	Type  string `json:"t"`
	Used  int64  `json:"used"`
	Limit int64  `json:"limit"`
}

type Challenge struct {
	Type  string `json:"t"`
	Nonce string `json:"nonce"`
}

type Auth struct {
	Type      string `json:"t"`
	Key       string `json:"k"`
	Signature string `json:"s"`
}

type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ICEConfig is the ready frame or the answer to an ice request.
type ICEConfig struct {
	Type string      `json:"t"`
	ICE  []ICEServer `json:"ice"`
	TTL  int         `json:"ttl"`
}

// Route carries an opaque envelope. Key is the target in to and the
// authenticated sender in from.
type Route struct {
	Type string          `json:"t"`
	Key  string          `json:"k"`
	Env  json.RawMessage `json:"env"`
}

type Offline struct {
	Type string `json:"t"`
	Key  string `json:"k"`
}

type Watch struct {
	Type string   `json:"t"`
	Keys []string `json:"k"`
}

type Presence struct {
	Type string `json:"t"`
	Key  string `json:"k"`
	On   bool   `json:"on"`
}

// RoomID names a room in join and joined frames.
type RoomID struct {
	Type string `json:"t"`
	Room string `json:"room"`
}

type RoomData struct {
	RoomID
	Data json.RawMessage `json:"data"`
}

// Beacon is the payload of the signed envelope a node sends by UDP multicast
// on the local network (contracts/rendezvous.md section 4). It has no g and
// no to; Port is where the node's signalling handler listens.
type Beacon struct {
	Type      string `json:"t"`
	Nonce     ID     `json:"n"`
	Timestamp int64  `json:"ts"`
	Port      int    `json:"port"`
}
