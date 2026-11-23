package proto

// TaskTrace is a complete signed transcript of one confirmed task from the
// owner's request to the secretary's summary (planning/flows/trace-task.md):
// the cast and the group as they start, then every command with what the home
// answers, pushes and delivers for it.
type TaskTrace struct {
	Keys   []TraceKey     `json:"keys"`
	Devlog Devlog         `json:"devlog"`
	Group  GroupRow       `json:"group"`
	Owner  HumanMemberRow `json:"owner"`
	AIs    []AIMemberRow  `json:"ais"`
	Seq    int64          `json:"seq"`
	Num    int            `json:"num"`
	Steps  []TraceStep    `json:"steps"`
}

// TraceKey is the name and device id of one device of the cast.
type TraceKey struct {
	Name   string `json:"name"`
	Device string `json:"device"`
}

// TraceStep is one command on ctl, its answer, the rows the home pushes for
// it, and the inbox items it delivers with the hosts' acknowledgements.
type TraceStep struct {
	From    string       `json:"from"`
	Cmd     Cmd          `json:"cmd"`
	Answer  CmdResult    `json:"answer"`
	Rows    Rows         `json:"rows"`
	Inboxes []TraceInbox `json:"inboxes"`
}

// TraceInbox is an inbox push to one host device and that host's acknowledgements.
type TraceInbox struct {
	To    string `json:"to"`
	Inbox Inbox  `json:"inbox"`
	Acks  []Ack  `json:"acks"`
}
