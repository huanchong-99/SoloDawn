package proto

import "encoding/json"

// Inbox is the push in which a home sends a host the items of its AIs
// (contracts/datachannel.md section 6).
type Inbox struct {
	Type  string      `json:"t"`
	Group ID          `json:"g"`
	Items []InboxItem `json:"items"`
}

// InboxItem is one item as it is sent. Env holds an Item, signed by the home
// when it queued the item, and is the only authority. Msg and Task are copies
// of the rows Env names, taken when the item is sent, to build the turn from;
// Msg stays as JSON until its reader knows its kind. Cause is the signed act
// that led to the item, unchanged.
type InboxItem struct {
	Env   Envelope        `json:"env"`
	Msg   json.RawMessage `json:"msg,omitempty"`
	Task  *TaskRow        `json:"task,omitempty"`
	Cause *Envelope       `json:"cause,omitempty"`
}

// Item is the payload of an item's envelope. Item numbers it for ack; Task and
// Msg are ids. Which fields an item of each Why has is in the table of
// contracts/datachannel.md section 6; a host judges that when it uses the item.
type Item struct {
	Type      string `json:"t"`
	Nonce     ID     `json:"n"`
	Timestamp int64  `json:"ts"`
	Group     ID     `json:"g"`
	To        string `json:"to"`
	Item      int64  `json:"item"`
	AI        ID     `json:"ai"`
	Why       string `json:"why"`
	Task      ID     `json:"task,omitempty"`
	Msg       ID     `json:"msg,omitempty"`
	Commit    string `json:"commit,omitempty"`
	PR        int    `json:"pr,omitempty"`
	Jump      True   `json:"jump,omitempty"`
	Code      string `json:"code,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Ack is a host's push for an item it delivered to a session or refused.
type Ack struct {
	Type  string `json:"t"`
	Group ID     `json:"g"`
	Item  int64  `json:"item"`
}
