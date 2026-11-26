package proto

import "testing"

func TestHandoutGolden(t *testing.T) {
	golden(t, "handout", Handout{
		Run: 4412, Kind: "task", Task: "T-12",
		Folder:  workspace,
		Context: "Work only in this folder; it is yours in this group.",
		Text:    "Report connection path changes.",
	})
}
