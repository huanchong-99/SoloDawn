package proto

import "encoding/json"

type Sync struct {
	Request
	Group     string `json:"g"`
	Since     int64  `json:"since"`
	StateOnly bool   `json:"state_only"`
}

type Reset struct {
	Type  string `json:"t"`
	Group string `json:"g"`
}

type Synced struct {
	Type string `json:"t"`
	Success
	Group string `json:"g"`
	Seq   int64  `json:"seq"`
}

// Rows may mix row types. Read the row's r (and gone) before decoding its fields.
type Rows struct {
	Type  string            `json:"t"`
	Group string            `json:"g"`
	Rows  []json.RawMessage `json:"rows"`
}

type FetchPage struct {
	Request
	Group  string `json:"g"`
	Before int64  `json:"before"`
	Limit  int    `json:"limit"`
}

type FetchThread struct {
	FetchPage
	Thread *string `json:"thread"` // null selects the main line
}

type FetchKind struct {
	FetchPage
	Kind  string `json:"kind"`
	Query string `json:"q,omitempty"`
}

// FetchRow asks for one row of the group by its type and id, with Before 0 and
// Limit 1.
type FetchRow struct {
	FetchPage
	Row   string `json:"r"`
	RowID string `json:"row"`
}

type Fetched struct {
	Type string `json:"t"`
	Success
	Rows []json.RawMessage `json:"rows"`
	More bool              `json:"more"`
}

type Cmd struct {
	Request
	Env Envelope `json:"env"`
}

type CmdResult struct {
	Success
	Num      int    `json:"num,omitempty"`
	Passed   *bool  `json:"passed,omitempty"`
	Round    int    `json:"round,omitempty"`
	Link     string `json:"link,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	ID       string `json:"id,omitempty"`
	Replaced *bool  `json:"replaced,omitempty"`
}

type ErrorResponse struct {
	Re  int64     `json:"re"`
	Err WireError `json:"err"`
}

type WireError struct {
	Code    string `json:"code"`
	Message string `json:"msg"`
}
