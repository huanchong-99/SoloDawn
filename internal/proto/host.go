package proto

// The host surface (contracts/datachannel.md section 7): what the owner's
// browser reads from one of the owner's nodes, and the host commands of
// contracts/commands.md section 2 that P-1 did not freeze.

// Groups is pushed whenever the list of groups a connected peer may read
// changes, the whole list again, entries as in welcome
// (contracts/datachannel.md section 3).
type Groups struct {
	Type   string         `json:"t"`
	Groups []GroupSummary `json:"groups"`
}

// HostState is everything host.info shows: its answer and its pushes carry
// the whole object.
type HostState struct {
	Device   string       `json:"device"`
	Name     string       `json:"name"`
	OS       string       `json:"os"`
	Version  string       `json:"version"`
	Open     int          `json:"open"`
	MaxOpen  int          `json:"max_open"`
	Offline  bool         `json:"offline"`
	Update   *Update      `json:"update"`
	Homes    []Home       `json:"homes"`
	Relay    *RelayUse    `json:"relay"`
	Agents   []HostAgent  `json:"agents"`
	Settings HostSettings `json:"settings"`
}

// HostInfo answers {"t":"host.info","id":…}, which is a Request.
type HostInfo struct {
	Type string `json:"t"`
	Success
	HostState
}

// HostInfoPush is host.info again after one of its fields changed: a push,
// without id or re.
type HostInfoPush struct {
	Type string `json:"t"`
	HostState
}

// Update is the outcome of an update check (contracts/local.md section 9):
// status pending or mismatch.
type Update struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// Home is a group whose home this node knows, and that home's device id.
type Home struct {
	Group ID     `json:"g"`
	Home  string `json:"home"`
	Name  string `json:"name"`
}

// RelayUse is the quota answer of the rendezvous the node is connected to,
// without its t.
type RelayUse struct {
	Used  int64 `json:"used"`
	Limit int64 `json:"limit"`
}

// HostAgent is a supported CLI found on this machine. Account is null until
// the owner has declared it (agent.set). Driver carries the CLI driver's
// capability declaration (contracts/step-3/drivers.md section 6).
type HostAgent struct {
	Agent      string       `json:"agent"`
	CLIVersion string       `json:"cli_version"`
	Account    *string      `json:"account"`
	LoginOK    bool         `json:"login_ok"`
	Caps       Caps         `json:"caps"`
	Driver     Driver       `json:"driver"`
	Groups     []AgentGroup `json:"groups"`
}

// Driver is the capability table a CLI driver declares (contracts/step-3/drivers.md
// sections 4 and 6).
type Driver struct {
	Accounts []string `json:"accounts"`
	Takeover string   `json:"takeover"`
}

// Caps are an agent's usage caps; a cap that is not there is not set.
type Caps struct {
	Day  *Cap `json:"day,omitempty"`
	Task *Cap `json:"task,omitempty"`
}

// Cap is reached when the agent has used Max of Unit: turns, tokens, usd or
// window_pct. Max may be a fraction.
type Cap struct {
	Unit string  `json:"unit"`
	Max  float64 `json:"max"`
}

// AgentGroup is the agent in one group: its member, the level of its task
// sessions, its derived state and task, and how many items are held for it.
type AgentGroup struct {
	Group  ID     `json:"g"`
	Member ID     `json:"member"`
	Level  int    `json:"level"`
	State  string `json:"state"`
	Task   ID     `json:"task,omitempty"`
	Queued int    `json:"queued"`
}

// HostSettings are the node settings that settings.set changes, except
// max_open and caps, which host.info shows beside them.
type HostSettings struct {
	RetentionDays int      `json:"retention_days"`
	IdleMinutes   int      `json:"idle_minutes"`
	DefaultLevel  int      `json:"default_level"`
	AllLevel3     bool     `json:"all_level3"`
	Approvers     []string `json:"approvers"`
}

// Audit asks for the owner's audit rows whose id is lower than Before, the id
// of the oldest row the client holds, newest first; Before 0 asks for the
// newest.
type Audit struct {
	Request
	Before int64 `json:"before"`
	Limit  int   `json:"limit"`
}

// AuditAnswer has no t, like the answer to cmd.
type AuditAnswer struct {
	Success
	Rows []AuditRow `json:"rows"`
}

// AuditRow is one entry of the audit (R-139). ID is its number on this node,
// which grows with every row, and pages the audit. AI is the agent, e.g.
// claude; By is the device that caused it.
type AuditRow struct {
	ID     int64  `json:"id"`
	TS     int64  `json:"ts"`
	Group  ID     `json:"g,omitempty"`
	AI     string `json:"ai,omitempty"`
	What   string `json:"what"`
	Detail string `json:"detail"`
	By     string `json:"by,omitempty"`
	Result string `json:"result,omitempty"`
}

// PTYResize is the size of the terminal of a takeover, pushed by the browser.
// AI is the member that is taken over.
type PTYResize struct {
	Type  string `json:"t"`
	Group ID     `json:"g"`
	AI    ID     `json:"ai"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

// SettingsSet carries only the settings being changed.
type SettingsSet struct {
	HostCommand
	RetentionDays *int       `json:"retention_days,omitempty"`
	IdleMinutes   *int       `json:"idle_minutes,omitempty"`
	MaxOpen       *int       `json:"max_open,omitempty"`
	DefaultLevel  *int       `json:"default_level,omitempty"`
	AllLevel3     *bool      `json:"all_level3,omitempty"`
	Approvers     *[]string  `json:"approvers,omitempty"`
	Caps          *AgentCaps `json:"caps,omitempty"`
}

// AgentCaps holds the caps of each agent being changed, whole: a cap that is
// not there is no longer set. An agent that is not there keeps its caps.
type AgentCaps struct {
	Claude *Caps `json:"claude,omitempty"`
	Codex  *Caps `json:"codex,omitempty"`
}

// AgentSet declares the account type of a CLI found on this machine:
// personal, team or api_key.
type AgentSet struct {
	HostCommand
	Agent   string `json:"agent"`
	Account string `json:"account"`
}

// PermSet sets the level of an agent's sessions in a group.
type PermSet struct {
	HostCommand
	Agent string `json:"agent"`
	Group ID     `json:"g"`
	Level int    `json:"level"`
}

// Takeover names the AI a takeover is about; alone it is takeover.end.
// pair.browser, host.offline and host.online are a HostCommand alone.
type Takeover struct {
	HostCommand
	Group  ID `json:"g"`
	Member ID `json:"member"`
}

// TakeoverStart may name the task to work on (flows/takeover.md, section A).
type TakeoverStart struct {
	Takeover
	Task ID `json:"task,omitempty"`
}
