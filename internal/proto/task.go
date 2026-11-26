package proto

type TaskRow struct {
	RowHeader
	Group      string        `json:"g"`
	Num        int           `json:"num"`
	Title      string        `json:"title"`
	Origin     string        `json:"origin"`
	Brief      string        `json:"brief,omitempty"`
	Plan       string        `json:"plan,omitempty"`
	State      string        `json:"state"`
	Step       string        `json:"step"`
	Report     string        `json:"report,omitempty"`
	Assignee   string        `json:"assignee"`
	Reviewer   string        `json:"reviewer,omitempty"`
	Asked      []string      `json:"asked,omitempty"`
	Foundation bool          `json:"foundation"`
	Pass       int           `json:"pass"`
	Depends    []string      `json:"depends"`
	Review     *Review       `json:"review,omitempty"`
	Wait       *TaskWait     `json:"wait"`
	Failures   int           `json:"failures"`
	Stuck      *Stuck        `json:"stuck,omitempty"`
	Thread     ThreadSummary `json:"thread"`
	Stats      TaskStats     `json:"stats"`
	Created    int64         `json:"created"`
}

// Stuck counts the failures of one kind in a row; while Ask is true the home
// refuses a submission (contracts/step-3/profiles.md section 4).
type Stuck struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
	Ask  bool   `json:"ask"`
}

// Review is the latest review. Failed counts the failed reviews since the task
// was created or a person last resumed or reassigned it from failed. Report is
// the report message the review was about.
type Review struct {
	Round  int    `json:"round"`
	Failed int    `json:"failed"`
	Score  int    `json:"score"`
	OK     bool   `json:"ok"`
	By     string `json:"by"`
	Report string `json:"report"`
	At     int64  `json:"at"`
}

// TaskWait is the latest reason a task is not moving. Kept is on the wait of a
// paused task that replaced a reviewer wait: that kind. By is on a wait of kind
// manual: the member who paused the task.
type TaskWait struct {
	Kind  string   `json:"kind"`
	Msg   string   `json:"msg,omitempty"`
	Who   []string `json:"who,omitempty"`
	Since int64    `json:"since"`
	Kept  string   `json:"kept,omitempty"`
	By    string   `json:"by,omitempty"`
}

type ThreadSummary struct {
	Count  int            `json:"count"`
	Rounds int            `json:"rounds"`
	Last   *ThreadMessage `json:"last,omitempty"`
}

type ThreadMessage struct {
	By        string `json:"by"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Timestamp int64  `json:"ts"`
}

type TaskStats struct {
	Started int64  `json:"started"`
	Ended   *int64 `json:"ended"`
}
