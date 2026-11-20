package proto

import "errors"

type Request struct {
	Type string `json:"t"`
	ID   int64  `json:"id"`
}

// Success starts every answer that is not an error (contracts/conventions.md
// section 3).
type Success struct {
	Re int64 `json:"re"`
	OK True  `json:"ok"`
}

// True is a truth value that is always true: "ok" of a success.
type True bool

var errNotTrue = errors.New("proto: ok of a success is not true")

func (t *True) UnmarshalJSON(b []byte) error {
	if string(b) != "true" {
		return errNotTrue
	}
	*t = true
	return nil
}

type Hello struct {
	Request
	V     int      `json:"v"`
	Kind  string   `json:"kind"`
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

type GroupSummary struct {
	Group string `json:"g"`
	Name  string `json:"name"`
	Seq   int64  `json:"seq"`
}

type Welcome struct {
	Type string `json:"t"`
	Success
	V       int            `json:"v"`
	Device  string         `json:"device"`
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Time    int64          `json:"time"`
	Groups  []GroupSummary `json:"groups"`
	Owner   bool           `json:"owner"`
}

type Devlog struct {
	Type    string     `json:"t"`
	User    string     `json:"user"`
	Entries []Envelope `json:"entries"`
}
