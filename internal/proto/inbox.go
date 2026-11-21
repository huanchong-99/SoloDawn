package proto

import "encoding/json"

// Inbox is the push in which a home sends a host the items of its AIs
// (contracts/datachannel.md section 6).
type Inbox struct {
	Type  string      `json:"t"`
	Group ID          `json:"g"`
	Items []InboxItem `json:"items"`
}

// InboxItem is one item as it is sent. Item is the plain item recorded when
// the home queued it. Msg and Task are copies of the rows Item names, taken
// when the item is sent, to build the turn from; Msg stays as JSON until its
// reader knows its kind.
type InboxItem struct {
	Item Item            `json:"item"`
	Msg  json.RawMessage `json:"msg,omitempty"`
	Task *TaskRow        `json:"task,omitempty"`
}

// Item is an inbox item for an AI or its host. Item numbers it for ack
// (items[].item.item); AI is the target member, and Task and Msg are ids.
// Which fields an item of each Why has is in the table of
// contracts/datachannel.md section 6; a host judges that when it uses the item.
type Item struct {
	Item   int64  `json:"item"`
	AI     ID     `json:"ai"`
	Why    string `json:"why"`
	Task   ID     `json:"task,omitempty"`
	Msg    ID     `json:"msg,omitempty"`
	Commit string `json:"commit,omitempty"`
	PR     int    `json:"pr,omitempty"`
	Jump   True   `json:"jump,omitempty"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Ack is a host's push for an item it delivered to a session or refused.
type Ack struct {
	Type  string `json:"t"`
	Group ID     `json:"g"`
	Item  int64  `json:"item"`
}
