package proto

import "testing"

func TestHandoutGolden(t *testing.T) {
	golden(t, "handout", Handout{
		Run: 4412, Kind: "task", Task: "T-12",
		Folder:  "/var/lib/solodawn/projects/" + group + "/wt/12-node-report-connection-path-changes",
		Context: "Work in the task worktree.",
		Text:    "Report connection path changes.",
	})
}
