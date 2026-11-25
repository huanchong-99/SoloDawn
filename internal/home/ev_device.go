package home

import "github.com/huanchong-99/SoloDawn/internal/proto"

// Device-log entries accepted by the node become device rows in every group of
// that person (planning/contracts/envelope.md section 4;
// planning/contracts/rows.md section 2).

func init() { Events.Add("device", devEvent) }

func devEvent(s *State, _ Disk, _ int64, event any) Result {
	rows, ok := event.([]proto.DeviceRow)
	if !ok || len(rows) == 0 {
		return Result{}
	}
	user := rows[0].User
	if user != s.Owner && s.human(user) == "" {
		return Result{}
	}
	held := 0
	if log := s.Logs[user]; log != nil {
		held = len(log.Entries)
	}
	return devRows(s, rows, held)
}

func devRows(s *State, rows []proto.DeviceRow, held int) Result {
	var res Result
	for _, r := range rows {
		if r.Version > held {
			if s.Online[r.Device] && (r.Op == "add") != s.Serves(r.Device) {
				res.GroupsChanged = true
			}
			res.Rows = append(res.Rows, RowOf(r))
		}
	}
	return res
}

// devOwner is the owner's whole log as device rows.
func devOwner(s *State) (rows []Row) {
	for _, env := range s.Logs[s.Owner].Entries {
		rows = append(rows, RowOf(peopleDevice(env)))
	}
	return rows
}
