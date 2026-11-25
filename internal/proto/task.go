package proto

type TaskRow struct {
	RowHeader
	Group      string        `json:"g"`
	Num        int           `json:"num"`
	Title      string        `json:"title"`
	Slug       Slug          `json:"slug"`
	Origin     string        `json:"origin"`
	Brief      string        `json:"brief,omitempty"`
	Plan       string        `json:"plan,omitempty"`
	State      string        `json:"state"`
	Step       string        `json:"step"`
	Commit     string        `json:"commit,omitempty"`
	Assignee   string        `json:"assignee"`
	Reviewer   string        `json:"reviewer,omitempty"`
	Asked      []string      `json:"asked,omitempty"`
	Foundation bool          `json:"foundation"`
	Pass       int           `json:"pass"`
	Depends    []string      `json:"depends"`
	Branch     string        `json:"branch,omitempty"`
	PR         int           `json:"pr,omitempty"`
	PRURL      string        `json:"pr_url,omitempty"`
	Checks     TaskChecks    `json:"checks"`
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

type TaskChecks struct {
	Local *Check `json:"local,omitempty"`
	Cloud *Check `json:"cloud,omitempty"`
}

type Check struct {
	OK     bool   `json:"ok"`
	At     int64  `json:"at"`
	Commit string `json:"commit"`
}

// Review is the latest review. Failed counts the failed reviews since the task
// was created or a person last resumed or reassigned it from failed.
type Review struct {
	Round  int    `json:"round"`
	Failed int    `json:"failed"`
	Score  int    `json:"score"`
	OK     bool   `json:"ok"`
	By     string `json:"by"`
	Commit string `json:"commit"`
	At     int64  `json:"at"`
}

// TaskWait is the latest reason a task is not moving. Kept is on the wait of a
// paused task that replaced a reviewer or conflict wait: that kind.
type TaskWait struct {
	Kind  string   `json:"kind"`
	Msg   string   `json:"msg,omitempty"`
	Who   []string `json:"who,omitempty"`
	Since int64    `json:"since"`
	Kept  string   `json:"kept,omitempty"`
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
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Started int64  `json:"started"`
	Ended   *int64 `json:"ended"`
}
