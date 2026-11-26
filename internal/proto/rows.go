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

// GroupRow is the one kind of group there is: people and AIs, with no
// repository, branch, forge or check command (contracts/rows.md section 1).
type GroupRow struct {
	RowHeader
	Name           string      `json:"name"`
	Home           string      `json:"home"`
	Secretary      string      `json:"secretary,omitempty"`
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

// AIMemberRow is an AI member. Workspace is the absolute path of the folder
// on Host that every session of this AI in this group runs in, set when it
// was added and never changed.
type AIMemberRow struct {
	MemberRow
	CLI       string   `json:"cli"`
	Tool      string   `json:"tool,omitempty"`
	Host      string   `json:"host"`
	HostName  string   `json:"host_name"`
	Workspace string   `json:"workspace"`
	Secretary bool     `json:"secretary"`
	Account   string   `json:"account"`
	Allow     []string `json:"allow"`
	Approvers []string `json:"approvers"`
	State     string   `json:"state"`
	Task      string   `json:"task,omitempty"`
	Since     int64    `json:"since"`
	Path      string   `json:"path"`
	Usage     Usage    `json:"usage"`
	Profile
}

// Profile is what the owner chose for an AI member: the CLI's model and
// thinking level, "" for the CLI's own default, and its tier and skills
// (contracts/step-3/profiles.md section 1).
type Profile struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
	Tier   string `json:"tier"`
	Skills string `json:"skills"`
}

// ProfileSet is the four fields of a Profile in a command, each optional.
type ProfileSet struct {
	Model  *string `json:"model,omitempty"`
	Effort *string `json:"effort,omitempty"`
	Tier   *string `json:"tier,omitempty"`
	Skills *string `json:"skills,omitempty"`
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

// MsgRow holds a message before its kind and status are known: its state may
// be null or any card's.
type MsgRow = MessageRow[json.RawMessage, *CardState]

// MessageRow binds a body and card state for a known kind. The state of a card
// is a struct, never null; a message that is not a card has NoCard.
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
	State     S         `json:"state"`
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
	NoteType  string  `json:"type,omitempty"`
	Name      string  `json:"name,omitempty"`
	Text      string  `json:"text"`
	By        string  `json:"by"`
	Timestamp int64   `json:"ts"`
}
