package proto

import "encoding/json"

type RowHeader struct {
	Type string `json:"r"`
	ID   string `json:"id"`
	Seq  int64  `json:"seq"`
}

type GoneRow struct {
	RowHeader
	Gone bool `json:"gone"`
}

type GroupRow struct {
	RowHeader
	Name           string      `json:"name"`
	Kind           string      `json:"kind"`
	Repo           string      `json:"repo,omitempty"`
	Base           string      `json:"base"`
	Forge          string      `json:"forge"`
	Home           string      `json:"home"`
	Secretary      string      `json:"secretary,omitempty"`
	Checks         []string    `json:"checks"`
	Pass           int         `json:"pass"`
	PassFoundation int         `json:"pass_foundation"`
	Limits         Limits      `json:"limits"`
	Paused         *GroupPause `json:"paused"`
	Created        int64       `json:"created"`
}

type Limits struct {
	Rounds            int `json:"rounds"`
	WakesPerHour      int `json:"wakes_per_hour"`
	TurnMinutes       int `json:"turn_minutes"`
	Failures          int `json:"failures"`
	ReviewRounds      int `json:"review_rounds"`
	AskDepth          int `json:"ask_depth"`
	AskWaitS          int `json:"ask_wait_s"`
	ApprovalWaitS     int `json:"approval_wait_s"`
	IdleNoticeMinutes int `json:"idle_notice_minutes"`
}

type GroupPause struct {
	By string `json:"by"`
	At int64  `json:"at"`
}

type MemberRow struct {
	RowHeader
	Group    string `json:"g"`
	Kind     string `json:"kind"`
	User     string `json:"user"`
	Name     string `json:"name"`
	LastSeen int64  `json:"last_seen"`
}

type HumanMemberRow struct {
	MemberRow
	Role   string `json:"role"`
	Online bool   `json:"online"`
}

type AIMemberRow struct {
	MemberRow
	CLI       string   `json:"cli"`
	Host      string   `json:"host"`
	HostName  string   `json:"host_name"`
	Secretary bool     `json:"secretary"`
	Account   string   `json:"account"`
	Allow     []string `json:"allow"`
	Approvers []string `json:"approvers"`
	State     string   `json:"state"`
	Task      string   `json:"task,omitempty"`
	Since     int64    `json:"since"`
	Path      string   `json:"path"`
	Usage     Usage    `json:"usage"`
}

type Usage struct {
	WindowPct    *float64 `json:"window_pct,omitempty"`
	WindowResets *int64   `json:"window_resets,omitempty"`
	DayCapPct    *float64 `json:"day_cap_pct,omitempty"`
	SpendToday   *float64 `json:"spend_today,omitempty"`
	SpendCap     *float64 `json:"spend_cap,omitempty"`
	TurnsToday   *int     `json:"turns_today,omitempty"`
}

type DeviceRow struct {
	RowHeader
	DeviceChange
	Env Envelope `json:"env"`
}

// MsgRow holds a message before its kind and status are known.
type MsgRow = MessageRow[json.RawMessage, CardState]

// MessageRow binds a body and card state for a known kind. Non-cards have nil State.
type MessageRow[B, S any] struct {
	RowHeader
	Pos       int64     `json:"pos"`
	Group     string    `json:"g"`
	Thread    *string   `json:"thread"`
	Main      bool      `json:"main"`
	By        string    `json:"by"`
	Kind      string    `json:"kind"`
	Timestamp int64     `json:"ts"`
	Text      string    `json:"text"`
	Mentions  []string  `json:"mentions"`
	Re        *string   `json:"re"`
	Body      B         `json:"body"`
	State     *S        `json:"state"`
	Env       *Envelope `json:"env,omitempty"`
}

type CardState struct {
	Status string    `json:"status"`
	By     *string   `json:"by,omitempty"`
	At     *int64    `json:"at,omitempty"`
	Note   *string   `json:"note,omitempty"`
	Choice *string   `json:"choice,omitempty"`
	Text   *string   `json:"text,omitempty"`
	Reply  *string   `json:"reply,omitempty"`
	MS     *int64    `json:"ms,omitempty"`
	Act    *Envelope `json:"act,omitempty"`
}

type NoteRow struct {
	RowHeader
	Pos       int64   `json:"pos"`
	Group     string  `json:"g"`
	Task      *string `json:"task"`
	Text      string  `json:"text"`
	By        string  `json:"by"`
	Timestamp int64   `json:"ts"`
}
