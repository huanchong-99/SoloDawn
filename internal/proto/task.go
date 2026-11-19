package proto

type TaskRow struct {
	RowHeader
	Group      string        `json:"g"`
	Num        int           `json:"num"`
	Title      string        `json:"title"`
	Origin     string        `json:"origin"`
	Brief      string        `json:"brief,omitempty"`
	State      string        `json:"state"`
	Step       string        `json:"step"`
	Commit     string        `json:"commit,omitempty"`
	Assignee   string        `json:"assignee"`
	Reviewer   string        `json:"reviewer,omitempty"`
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
	Thread     ThreadSummary `json:"thread"`
	Stats      TaskStats     `json:"stats"`
	Created    int64         `json:"created"`
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

type Review struct {
	Round  int    `json:"round"`
	Score  int    `json:"score"`
	OK     bool   `json:"ok"`
	By     string `json:"by"`
	Commit string `json:"commit"`
	At     int64  `json:"at"`
}

type TaskWait struct {
	Kind  string   `json:"kind"`
	Msg   string   `json:"msg,omitempty"`
	Who   []string `json:"who,omitempty"`
	Since int64    `json:"since"`
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
