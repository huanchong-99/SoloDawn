package home_test

import (
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestExternalToolAddEditAndClear(t *testing.T) {
	s := grpState(t)
	member := id(120)
	add := memAddCmd(120, member, "external", "Scout", "api_key", false)
	add.Tool = new("ZCode")
	res := grpApply(t, s, memDisk{}, receipts{}, laptop, add)
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	want := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: member, Seq: 11},
			Group: groupID, Kind: "ai", User: browser.ID(), Name: "Scout", LastSeen: clock},
		CLI: "external", Tool: "ZCode", Host: laptop.ID(), HostName: "a device", Workspace: "/home/dev/website",
		Account: "api_key", Allow: []string{}, Approvers: []string{browser.ID()},
		State: "dormant", Since: clock, Path: "direct", Profile: proto.Profile{Tier: "standard"},
	}
	equal(t, s.AIs[member], want)

	set := memSetCmd(121, member)
	set.Tool = new(strings.Repeat("🧭", 32))
	res = grpApply(t, s, memDisk{}, receipts{}, browser, set)
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	want.Tool, want.Seq = *set.Tool, 12
	equal(t, s.AIs[member], want)

	set = memSetCmd(122, member)
	set.Tool = new("")
	res = grpApply(t, s, memDisk{}, receipts{}, browser, set)
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	want.Tool, want.Seq = "", 13
	equal(t, s.AIs[member], want)
	equal(t, strings.Contains(string(briefRaw(t, s.AIs[member])), `"tool"`), false)
}

func TestExternalToolValidation(t *testing.T) {
	for _, tc := range []struct{ name, cli, tool string }{
		{"empty at add", "external", ""},
		{"too many code points", "external", strings.Repeat("文", 33)},
		{"control", "external", "ZCode\u0085"},
		{"managed", "claude", "ZCode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := grpState(t)
			add := memAddCmd(120, id(120), tc.cli, "Scout", "api_key", false)
			add.Tool = &tc.tool
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, laptop, add), "bad_request")
			if tc.tool == "" {
				return
			}
			row := s.AIs[grpWorkerA]
			row.CLI = tc.cli
			s.AIs[row.ID] = row
			set := memSetCmd(121, row.ID)
			set.Tool = &tc.tool
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, browser, set), "bad_request")
		})
	}
}

func TestExternalToolSetIsOwnerOnly(t *testing.T) {
	s := grpState(t)
	row := s.AIs[grpWorkerA]
	row.CLI, row.Tool = "external", "ZCode"
	s.AIs[row.ID] = row
	set := memSetCmd(120, row.ID)
	set.Tool = new("Cursor")
	briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, guest, set), "forbidden")
	set.As = proto.ID(row.ID)
	briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, laptop, set), "forbidden")

	row.Host = guest.ID()
	s.AIs[row.ID] = row
	set = memSetCmd(121, row.ID)
	set.Tool, set.Approvers = new("Cursor"), &[]string{guest.ID()}
	briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, guest, set), "forbidden")
	equal(t, s.AIs[row.ID], row)
}
