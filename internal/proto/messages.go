package proto

import "errors"

// EmptyBody is shared by text, progress, question and reply.
type EmptyBody struct{}

type BriefBody struct {
	Title     string      `json:"title"`
	Goal      string      `json:"goal"`
	Criteria  []string    `json:"criteria"`
	Scope     BriefScope  `json:"scope"`
	Tasks     []BriefTask `json:"tasks"`
	Pass      int         `json:"pass"`
	Requester string      `json:"requester"`
	Replaces  *string     `json:"replaces"`
}

type BriefScope struct {
	In  []string `json:"in"`
	Out []string `json:"out"`
}

type BriefTask struct {
	Title      string `json:"title"`
	Assignee   string `json:"assignee"`
	Reviewer   string `json:"reviewer"`
	Foundation bool   `json:"foundation"`
	Depends    []int  `json:"depends"`
}

type TaskBody struct {
	Task string `json:"task"`
}

type ApprovalBody struct {
	Task      string   `json:"task"`
	AI        string   `json:"ai"`
	Tool      string   `json:"tool"`
	Command   string   `json:"command"`
	Full      *int     `json:"full,omitempty"` // bytes of the whole request when command holds its beginning
	Why       *string  `json:"why,omitempty"`
	Category  string   `json:"category"`
	Approvers []string `json:"approvers"`
}

type DecisionBody struct {
	Task        *string          `json:"task,omitempty"`
	Question    string           `json:"question"`
	Context     *string          `json:"context,omitempty"`
	Options     []DecisionOption `json:"options"`
	Recommended *string          `json:"recommended,omitempty"`
	To          []string         `json:"to"`
}

type DecisionOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Effect string `json:"effect"`
}

type SummaryBody struct {
	Task    string `json:"task"`
	Score   int    `json:"score"`
	Rounds  int    `json:"rounds"`
	Minutes int    `json:"minutes"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	PR      *int   `json:"pr,omitempty"`
}

type CallBody struct {
	To    string `json:"to"`
	Depth int    `json:"depth"`
	Wait  bool   `json:"wait"`
}

type ReportBody struct {
	Commit  string `json:"commit"`
	Files   int    `json:"files"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	PR      *int   `json:"pr,omitempty"`
}

type ChecksBody struct {
	Stage  string      `json:"stage"`
	OK     bool        `json:"ok"`
	Commit string      `json:"commit"`
	Items  []CheckItem `json:"items"`
}

type CheckItem struct {
	Name   string  `json:"name"`
	OK     bool    `json:"ok"`
	Detail *string `json:"detail,omitempty"`
}

type ReviewBody struct {
	Task   string       `json:"task"`
	Num    int          `json:"num"`
	Title  string       `json:"title"`
	Commit string       `json:"commit"`
	Round  int          `json:"round"`
	Score  int          `json:"score"`
	Pass   int          `json:"pass"`
	OK     bool         `json:"ok"`
	Items  []ReviewItem `json:"items"`
}

type ReviewItem struct {
	Text string  `json:"text"`
	Veto bool    `json:"veto"`
	File *string `json:"file,omitempty"`
	Line *int    `json:"line,omitempty"`
}

type HandbackBody struct {
	Files   []string        `json:"files"`
	Commits []CommitSubject `json:"commits"`
	Added   int             `json:"added"`
	Removed int             `json:"removed"`
}

type CommitSubject struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// SystemCode alone is the no_secretary body. The other system codes carry the fields below.
type SystemCode struct {
	Code string `json:"code"`
}

type RoundsLimitBody struct {
	SystemCode
	Task string `json:"task"`
	N    int    `json:"n"`
}

type TurnInterruptedBody struct {
	SystemCode
	AI     string  `json:"ai"`
	Reason string  `json:"reason"`
	Task   *string `json:"task,omitempty"`
	Detail *string `json:"detail,omitempty"`
}

type GroupPauseBody struct {
	SystemCode
	By string `json:"by"`
}

type PreemptedBody struct {
	SystemCode
	Task string `json:"task"`
	By   string `json:"by"`
}

type CapReachedBody struct {
	SystemCode
	AI   string  `json:"ai"`
	Cap  string  `json:"cap"`
	Task *string `json:"task,omitempty"`
}

type RateLimitedBody struct {
	SystemCode
	AI     string `json:"ai"`
	Resets int64  `json:"resets"`
}

type LoginInvalidBody struct {
	SystemCode
	AI     string  `json:"ai"`
	Detail *string `json:"detail,omitempty"`
}

type TaskFailedBody struct {
	SystemCode
	Task   string `json:"task"`
	Reason string `json:"reason"`
}

type TaskMergedBody struct {
	SystemCode
	Task        string `json:"task"`
	Commit      string `json:"commit"`
	MergeCommit string `json:"merge_commit"`
}

type SessionReplacedBody struct {
	SystemCode
	AI   string  `json:"ai"`
	Task *string `json:"task,omitempty"`
}

// NoCard is the state of a message that is not a card: null, and nothing else.
type NoCard = *noCard

type noCard struct{}

var errCard = errors.New("proto: a state on a message that is not a card")

func (*noCard) UnmarshalJSON([]byte) error {
	return errCard
}

// StatusOnly represents pending cards and waiting calls.
type StatusOnly struct {
	Status string `json:"status"`
}

type ActedState struct {
	Status string   `json:"status"`
	By     string   `json:"by"`
	At     int64    `json:"at"`
	Act    Envelope `json:"act"`
}

// BriefReviseState is the one brief state with a note. A superseded brief has no
// type of its own: it keeps the keys of the state it replaced, pending included,
// so it is read as a CardState.
type BriefReviseState struct {
	Status string   `json:"status"`
	By     string   `json:"by"`
	At     int64    `json:"at"`
	Act    Envelope `json:"act"`
	Note   *string  `json:"note,omitempty"`
}

type TimedState struct {
	Status string `json:"status"`
	At     int64  `json:"at"`
}

type DecisionDecidedState struct {
	Status string   `json:"status"`
	By     string   `json:"by"`
	At     int64    `json:"at"`
	Choice *string  `json:"choice,omitempty"`
	Text   *string  `json:"text,omitempty"`
	Act    Envelope `json:"act"`
}

type CallAnsweredState struct {
	Status string `json:"status"`
	Reply  string `json:"reply"`
	MS     int64  `json:"ms"`
}
