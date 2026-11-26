package host

import (
	"maps"
	"slices"
)

// Account types a CLI driver may list in Capabilities.Accounts
// (planning/contracts/step-3/drivers.md sections 2 and 4).
const (
	AccountPersonal = "personal"
	AccountTeam     = "team"
	AccountAPIKey   = "api_key"
)

// Approval channels a CLI driver may declare in Capabilities.Approvals
// (planning/contracts/step-3/drivers.md section 4).
const (
	ApprovalTool = "tool"
	ApprovalConn = "conn"
)

// Terminal takeover modes a CLI driver may declare in Capabilities.Takeover
// (planning/contracts/step-3/drivers.md section 4).
const (
	TakeoverNew    = "new"
	TakeoverResume = "resume"
	TakeoverNone   = "none"
)

// Capabilities is the capability table a CLI driver declares
// (planning/contracts/step-3/drivers.md section 4).
type Capabilities struct {
	Accounts  []string
	Levels    []int
	Approvals string
	Takeover  string
	Usage     []string
}

// DriverCaps holds the capability declaration of each CLI driver by its agent
// name, matching Drivers (planning/contracts/step-3/drivers.md sections 4 and 8).
var DriverCaps = map[string]Capabilities{}

func driverNames() []string {
	return slices.Sorted(maps.Keys(DriverCaps))
}

func checkDriverAccount(agent, account string) error {
	if !slices.Contains(DriverCaps[agent].Accounts, account) {
		return &Error{Code: "bad_request", Message: "account " + account + " is not in " + agent + " accounts"}
	}
	return nil
}

func usesApproveTool(agent string) bool {
	return DriverCaps[agent].Approvals == ApprovalTool
}

// checkTakeover checks a driver's takeover declaration for takeover.start
// before its relay, busy and cli checks, and reports whether the CLI cannot
// start a new session in a terminal, so that a session that has never run
// needs one turn in structured mode first
// (planning/contracts/step-3/drivers.md section 5).
func checkTakeover(agent string) (assignFirst bool, err error) {
	switch DriverCaps[agent].Takeover {
	case TakeoverNew:
		return false, nil
	case TakeoverResume:
		return true, nil
	default:
		return false, &Error{Code: "conflict", Message: agent + " takeover is none"}
	}
}
