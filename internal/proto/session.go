package proto

type Request struct {
	Type string `json:"t"`
	ID   int64  `json:"id"`
}

type Success struct {
	Re int64 `json:"re"`
	OK bool  `json:"ok"`
}

type Hello struct {
	Request
	V     int      `json:"v"`
	Kind  string   `json:"kind"`
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

type GroupSummary struct {
	Group string `json:"g"`
	Name  string `json:"name"`
	Seq   int64  `json:"seq"`
}

type Welcome struct {
	Type string `json:"t"`
	Success
	V       int            `json:"v"`
	Device  string         `json:"device"`
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Time    int64          `json:"time"`
	Groups  []GroupSummary `json:"groups"`
	Owner   bool           `json:"owner"`
}

type Devlog struct {
	Type    string     `json:"t"`
	User    string     `json:"user"`
	Entries []Envelope `json:"entries"`
}
