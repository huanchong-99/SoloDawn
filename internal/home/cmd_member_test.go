package home_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type memDisk []store.Item

func (memDisk) Msg(string) (proto.MsgRow, bool)           { return proto.MsgRow{}, false }
func (memDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (memDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (memDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (memDisk) Pending() []proto.MsgRow                   { return nil }
func (d memDisk) Queued() []store.Item                    { return d }
func (d memDisk) OtherMember(string) bool                 { return false }

func memAddCmd(n byte, memberID, cli, name, account string, secretary bool) proto.MemberAddAI {
	return proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "add_ai",
			ID:          proto.ID(memberID),
		},
		CLI: cli, Name: name, Workspace: "/home/dev/website", Account: account, Secretary: secretary, Approvers: []string{browser.ID()},
	}
}

func memSetCmd(n byte, memberID string) proto.MemberSet {
	return proto.MemberSet{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "set",
			ID:          proto.ID(memberID),
		},
	}
}

func memRemoveCmd(n byte, memberID string) proto.MemberCommand {
	return proto.MemberCommand{
		HomeCommand: proto.HomeCommand{Type: "member", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(groupID)},
		Op:          "remove",
		ID:          proto.ID(memberID),
	}
}

func memAppendDevice(t *testing.T, l *identity.Log, signer, dev identity.Key, kind, name string) {
	t.Helper()
	p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":%q,"name":%q}`,
		l.User, len(l.Entries)+1, dev.ID(), kind, name)
	if _, err := l.Apply(signer.Sign(p)); err != nil {
		t.Fatal(err)
	}
}

func TestMemberAddAIHostAndPath(t *testing.T) {
	s := grpState(t)
	l := &identity.Log{User: browser.ID()}
	memAppendDevice(t, l, browser, browser, "browser", "phone")
	memAppendDevice(t, l, browser, node, "node", "home-box")
	memAppendDevice(t, l, browser, laptop, "node", "old-laptop")
	// A later add entry for the same node updates its label.
	memAppendDevice(t, l, browser, laptop, "node", "studio")
	memAppendDevice(t, l, browser, device(7), "node", "other-box")
	s.Logs[browser.ID()] = l

	remoteID, localID := id(90), id(91)
	cmdRemote := memAddCmd(90, remoteID, "codex", "Remote Codex", "team", false)
	cmdRemote.Approvers = []string{}
	res := grpApply(t, s, memDisk{}, receipts{}, laptop, cmdRemote)
	wantRemote := proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: remoteID, Seq: 11},
			Group:     groupID, Kind: "ai", User: browser.ID(), Name: "Remote Codex", LastSeen: clock,
		},
		CLI: "codex", Host: laptop.ID(), HostName: "studio", Workspace: "/home/dev/website", Secretary: false,
		Account: "team", Allow: []string{}, Approvers: []string{browser.ID()},
		State: "dormant", Since: clock, Path: "direct", Usage: proto.Usage{},
		Profile: proto.Profile{Tier: "standard"},
	}
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.GroupsChanged, false)
	equal(t, briefRows(res), []any{wantRemote})
	grpNoNulls(t, briefRows(res)[0])
	s.Commit(res)

	// The home takes any driver name and any path, as its host wrote it; the
	// node checks that it has the driver and the folder.
	cmdLocal := memAddCmd(91, localID, "qwen_code", "Local Claude", "api_key", false)
	cmdLocal.Workspace, cmdLocal.Approvers = `C:\Users\dev\site`, []string{guest.ID(), browser.ID()}
	res = grpApply(t, s, memDisk{}, receipts{}, node, cmdLocal)
	wantLocal := wantRemote
	wantLocal.ID, wantLocal.Seq, wantLocal.Name = localID, 12, "Local Claude"
	wantLocal.CLI, wantLocal.Host, wantLocal.HostName, wantLocal.Workspace = "qwen_code", node.ID(), "home-box", `C:\Users\dev\site`
	wantLocal.Account, wantLocal.Approvers, wantLocal.Path = "api_key", []string{guest.ID(), browser.ID()}, "local"
	equal(t, briefRows(res), []any{wantLocal})
}

func TestMemberAddAISecretaryRules(t *testing.T) {
	t.Run("a non-admin adds a working AI while the group has a secretary", func(t *testing.T) {
		s := grpState(t)
		cmd := memAddCmd(92, id(92), "codex", "Guest Worker", "personal", false)
		cmd.Approvers = []string{guest.ID()}
		res := grpApply(t, s, memDisk{}, receipts{}, device(6), cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
		equal(t, res.Rows[0].Value().(proto.AIMemberRow).Approvers, []string{device(6).ID(), guest.ID()})
	})

	t.Run("a non-admin adds the first secretary when the group has none", func(t *testing.T) {
		s := grpState(t)
		sec := s.AIs[aiID]
		sec.Secretary = false
		s.AIs[aiID] = sec
		s.Group.Secretary = ""

		newID := id(93)
		res := grpApply(t, s, memDisk{}, receipts{}, device(6), memAddCmd(93, newID, "claude", "Guest Sec", "personal", true))
		equal(t, res.Answer, store.Outcome{OK: true})
		rows := briefRows(res)
		equal(t, len(rows), 2)
		equal(t, rows[0].(proto.AIMemberRow).Secretary, true)
		wantGroup := s.Group
		wantGroup.Seq, wantGroup.Secretary = 12, newID
		equal(t, rows[1], wantGroup)
	})

	t.Run("an admin replaces the existing secretary", func(t *testing.T) {
		s := grpState(t)
		newID := id(94)
		res := grpApply(t, s, memDisk{}, receipts{}, guest, memAddCmd(94, newID, "claude", "Admin Sec", "team", true))
		equal(t, res.Answer, store.Outcome{OK: true})
		rows := briefRows(res)
		equal(t, len(rows), 3)
		equal(t, rows[0].(proto.AIMemberRow).ID, newID)
		equal(t, rows[0].(proto.AIMemberRow).Secretary, true)
		oldSec := s.AIs[aiID]
		oldSec.Seq, oldSec.Secretary = 12, false
		wantGroup := s.Group
		wantGroup.Seq, wantGroup.Secretary = 13, newID
		equal(t, rows[1:], []any{oldSec, wantGroup})
	})
}

func TestMemberAddAIRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		by     identity.Key
		raw    string
		change func(*proto.MemberAddAI)
		code   string
	}{
		"by an AI": {
			by:     laptop,
			change: func(p *proto.MemberAddAI) { p.As = proto.ID(aiID) },
			code:   "forbidden",
		},
		"by a non-admin replacing the secretary": {
			by:     device(6),
			change: func(p *proto.MemberAddAI) { p.Secretary = true },
			code:   "forbidden",
		},
		"with a bad payload": {
			raw:  `{"t":"member","n":"` + id(95) + `","ts":1763600000000,"g":"` + groupID + `","op":"add_ai","id":"` + id(95) + `","secretary":"yes"}`,
			code: "bad_request",
		},
		"with an empty name": {
			change: func(p *proto.MemberAddAI) { p.Name = "" },
			code:   "bad_request",
		},
		"with an empty workspace": {
			change: func(p *proto.MemberAddAI) { p.Workspace = "" },
			code:   "bad_request",
		},
		"with a cli name outside a-z, 0-9 and _": {
			change: func(p *proto.MemberAddAI) { p.CLI = "Gemini-CLI" },
			code:   "bad_request",
		},
		"with an unknown account": {
			change: func(p *proto.MemberAddAI) { p.Account = "free" },
			code:   "bad_request",
		},
		"with the id of a human member": {
			change: func(p *proto.MemberAddAI) { p.ID = proto.ID(grpAdmin) },
			code:   "conflict",
		},
		"with the id of an AI member": {
			change: func(p *proto.MemberAddAI) { p.ID = proto.ID(aiID) },
			code:   "conflict",
		},
		"with the name of a human member": {
			change: func(p *proto.MemberAddAI) { p.Name = "owner" },
			code:   "conflict",
		},
		"with the name of an AI member": {
			change: func(p *proto.MemberAddAI) { p.Name = "Codex" },
			code:   "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := grpState(t)
			by := laptop
			if c.by != nil {
				by = c.by
			}
			if c.raw != "" {
				briefRefused(t, grpApplyRaw(t, s, memDisk{}, receipts{}, by, c.raw), c.code)
				return
			}
			cmd := memAddCmd(96, id(96), "codex", "Fresh AI", "personal", false)
			if c.change != nil {
				c.change(&cmd)
			}
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, by, cmd), c.code)
		})
	}
}

func TestMemberSetEachFieldIndividually(t *testing.T) {
	for name, c := range map[string]struct {
		set  func(*proto.MemberSet)
		want func(*proto.AIMemberRow)
	}{
		"new name": {
			set:  func(p *proto.MemberSet) { v := "Codex Prime"; p.Name = &v },
			want: func(m *proto.AIMemberRow) { m.Name = "Codex Prime" },
		},
		"keeping own name": {
			set:  func(p *proto.MemberSet) { v := "Codex"; p.Name = &v },
			want: func(*proto.AIMemberRow) {},
		},
		"account team": {
			set:  func(p *proto.MemberSet) { v := "team"; p.Account = &v },
			want: func(m *proto.AIMemberRow) { m.Account = "team" },
		},
		"account api_key": {
			set:  func(p *proto.MemberSet) { v := "api_key"; p.Account = &v },
			want: func(m *proto.AIMemberRow) { m.Account = "api_key" },
		},
		"allow list": {
			set:  func(p *proto.MemberSet) { v := []string{guest.ID()}; p.Allow = &v },
			want: func(m *proto.AIMemberRow) { m.Allow = []string{guest.ID()} },
		},
		"empty approvers keeps owner": {
			set:  func(p *proto.MemberSet) { v := []string{}; p.Approvers = &v },
			want: func(m *proto.AIMemberRow) { m.Approvers = []string{browser.ID()} },
		},
		"approvers without owner adds owner": {
			set:  func(p *proto.MemberSet) { v := []string{guest.ID()}; p.Approvers = &v },
			want: func(m *proto.AIMemberRow) { m.Approvers = []string{browser.ID(), guest.ID()} },
		},
		"approvers with owner keeps order without duplicate": {
			set:  func(p *proto.MemberSet) { v := []string{guest.ID(), browser.ID()}; p.Approvers = &v },
			want: func(m *proto.AIMemberRow) { m.Approvers = []string{guest.ID(), browser.ID()} },
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := grpState(t)
			initial := s.AIs[grpWorkerA]
			initial.Approvers = []string{browser.ID(), device(6).ID()}
			s.AIs[grpWorkerA] = initial
			want := initial
			want.Seq = 11
			want.Tier = "standard"
			c.want(&want)
			cmd := memSetCmd(100, grpWorkerA)
			c.set(&cmd)
			res := grpApply(t, s, memDisk{}, receipts{}, browser, cmd)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, res.GroupsChanged, false)
			equal(t, briefRows(res), []any{want})
			grpNoNulls(t, briefRows(res)[0])
		})
	}
}

func TestMemberSetHostOnlyForApprovers(t *testing.T) {
	// When the signing device is the AI's host under another user's log, only
	// approvers alone is permitted, and the AI's owner stays in approvers.
	makeHostState := func(t *testing.T) *home.State {
		t.Helper()
		s := grpState(t)
		hosted := s.AIs[grpWorkerA]
		hosted.User, hosted.Host = device(6).ID(), guest.ID()
		s.AIs[grpWorkerA] = hosted
		return s
	}

	s := makeHostState(t)
	cmd := memSetCmd(101, grpWorkerA)
	approvers := []string{guest.ID()}
	cmd.Approvers = &approvers
	res := grpApply(t, s, memDisk{}, receipts{}, guest, cmd)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.Rows[0].Value().(proto.AIMemberRow).Approvers, []string{device(6).ID(), guest.ID()})

	for name, addField := range map[string]func(*proto.MemberSet){
		"host also sets name":    func(p *proto.MemberSet) { v := "Renamed"; p.Name = &v },
		"host also sets account": func(p *proto.MemberSet) { v := "team"; p.Account = &v },
		"host also sets allow":   func(p *proto.MemberSet) { v := []string{}; p.Allow = &v },
	} {
		t.Run(name, func(t *testing.T) {
			st := makeHostState(t)
			bad := memSetCmd(102, grpWorkerA)
			bad.Approvers = &approvers
			addField(&bad)
			briefRefused(t, grpApply(t, st, memDisk{}, receipts{}, guest, bad), "forbidden")
		})
	}
}

func TestMemberSetRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		by     identity.Key
		raw    string
		target string
		set    func(*proto.MemberSet)
		code   string
	}{
		"with a bad payload": {
			raw:  `{"t":"member","n":"` + id(103) + `","ts":1763600000000,"g":"` + groupID + `","op":"set","id":"` + grpWorkerA + `","allow":"all"}`,
			code: "bad_request",
		},
		"with no optional fields": {set: func(*proto.MemberSet) {}, code: "bad_request"},
		"with an empty name": {
			set:  func(p *proto.MemberSet) { v := ""; p.Name = &v },
			code: "bad_request",
		},
		"with an unknown account": {
			set:  func(p *proto.MemberSet) { v := "free"; p.Account = &v },
			code: "bad_request",
		},
		"for an unknown member": {
			target: id(99),
			set:    func(p *proto.MemberSet) { v := "Renamed"; p.Name = &v },
			code:   "not_found",
		},
		"for a human member": {
			target: grpAdmin,
			set:    func(p *proto.MemberSet) { v := "Renamed"; p.Name = &v },
			code:   "bad_request",
		},
		"by an AI": {
			by: laptop,
			set: func(p *proto.MemberSet) {
				p.As = proto.ID(grpWorkerA)
				v := "Renamed"
				p.Name = &v
			},
			code: "forbidden",
		},
		"by an admin who is neither owner nor host, even for approvers alone": {
			by:   guest,
			set:  func(p *proto.MemberSet) { v := []string{guest.ID()}; p.Approvers = &v },
			code: "forbidden",
		},
		"with the name of a human member": {
			set:  func(p *proto.MemberSet) { v := "owner"; p.Name = &v },
			code: "conflict",
		},
		"with the name of another AI member": {
			set:  func(p *proto.MemberSet) { v := "Helper"; p.Name = &v },
			code: "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := grpState(t)
			by := browser
			if c.by != nil {
				by = c.by
			}
			if c.raw != "" {
				briefRefused(t, grpApplyRaw(t, s, memDisk{}, receipts{}, by, c.raw), c.code)
				return
			}
			target := grpWorkerA
			if c.target != "" {
				target = c.target
			}
			cmd := memSetCmd(104, target)
			c.set(&cmd)
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, by, cmd), c.code)
		})
	}
}

func TestMemberRemoveWorkingAndSecretary(t *testing.T) {
	t.Run("a non-admin removes their own working AI and retires its items", func(t *testing.T) {
		s := grpState(t)
		owned := s.AIs[grpWorkerA]
		owned.User, owned.Host = device(6).ID(), device(6).ID()
		s.AIs[grpWorkerA] = owned
		// Finished tasks on this AI, and active tasks on other AIs, allow removal.
		s.Tasks[id(110)] = proto.TaskRow{State: "done", Assignee: grpWorkerA}
		s.Tasks[id(111)] = proto.TaskRow{State: "cancelled", Reviewer: grpWorkerA}
		s.Tasks[id(120)] = proto.TaskRow{State: "active", Assignee: grpWorkerB, Reviewer: grpWorkerB}
		d := memDisk{
			{ID: 5, AI: grpWorkerA, Why: "mention"},
			{ID: 6, AI: aiID, Why: "notice"},
			{ID: 7, AI: grpWorkerA, Why: "reply"},
		}
		res := grpApply(t, s, d, receipts{}, device(6), memRemoveCmd(112, grpWorkerA))
		wantGone := proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: grpWorkerA, Seq: 11}, Gone: true}
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.GroupsChanged, false)
		equal(t, res.Retire, []int64{5, 7})
		equal(t, briefRows(res), []any{wantGone})
		grpNoNulls(t, briefRows(res)[0])
		s.Commit(res)
		_, still := s.AIs[grpWorkerA]
		equal(t, still, false)
		equal(t, s.Group.Secretary, aiID)
	})

	t.Run("an admin removes another member's AI and removing the secretary writes no_secretary", func(t *testing.T) {
		s := grpState(t)
		owned := s.AIs[grpWorkerB]
		owned.User = device(6).ID()
		s.AIs[grpWorkerB] = owned
		res := grpApply(t, s, memDisk{}, receipts{}, guest, memRemoveCmd(113, grpWorkerB))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64(nil))
		s.Commit(res)

		// Now remove the secretary.
		d := memDisk{{ID: 9, AI: aiID, Why: "confirmed"}}
		res = grpApply(t, s, d, receipts{}, browser, memRemoveCmd(114, aiID))
		wantGone := proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: aiID, Seq: 12}, Gone: true}
		wantGroup := s.Group
		wantGroup.Seq, wantGroup.Secretary = 13, ""
		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(114), Seq: 14}, Pos: 14, Group: groupID,
			By: "system", Kind: "system", Timestamp: clock, Mentions: []string{},
			Body: json.RawMessage(`{"code":"no_secretary"}`),
		}
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{9})
		rows := briefRows(res)
		equal(t, rows, []any{wantGone, wantGroup, wantMsg})
		grpNoNulls(t, rows[0])
		grpNoNulls(t, rows[1], "/paused")
		grpNoNulls(t, rows[2], "/thread", "/re", "/state")
		s.Commit(res)
		_, still := s.AIs[aiID]
		equal(t, still, false)
		equal(t, s.Group.Secretary, "")
	})
}

func TestMemberRemoveRefusals(t *testing.T) {
	otherMember := device(9)
	for name, c := range map[string]struct {
		by     identity.Key
		raw    string
		target string
		as     string
		state  func(*home.State)
		code   string
	}{
		"with a bad id": {
			raw:  `{"t":"member","n":"` + id(115) + `","ts":1763600000000,"g":"` + groupID + `","op":"remove","id":"bad"}`,
			code: "bad_request",
		},
		"for an unknown member": {target: id(99), code: "not_found"},
		"for the owner":         {target: grpOwnerID(groupID), code: "forbidden"},
		"by the AI itself": {
			by: device(6), as: grpWorkerA,
			state: func(s *home.State) {
				m := s.AIs[grpWorkerA]
				m.User, m.Host = device(6).ID(), device(6).ID()
				s.AIs[grpWorkerA] = m
			},
			code: "forbidden",
		},
		"by another non-admin member": {
			by: otherMember,
			state: func(s *home.State) {
				s.Logs[otherMember.ID()] = logOf(t, otherMember)
				s.People[id(116)] = person(id(116), otherMember, "member")
			},
			code: "forbidden",
		},
		"while assignee of an active task": {
			state: func(s *home.State) { s.Tasks[id(117)] = proto.TaskRow{State: "active", Assignee: grpWorkerA} },
			code:  "conflict",
		},
		"while reviewer of a queued task": {
			state: func(s *home.State) { s.Tasks[id(118)] = proto.TaskRow{State: "queued", Reviewer: grpWorkerA} },
			code:  "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := grpState(t)
			if c.state != nil {
				c.state(s)
			}
			by := browser
			if c.by != nil {
				by = c.by
			}
			if c.raw != "" {
				briefRefused(t, grpApplyRaw(t, s, memDisk{}, receipts{}, by, c.raw), c.code)
				return
			}
			target := grpWorkerA
			if c.target != "" {
				target = c.target
			}
			cmd := memRemoveCmd(119, target)
			cmd.As = proto.ID(c.as)
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, by, cmd), c.code)
		})
	}
}
