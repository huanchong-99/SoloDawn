package home_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const grpCreateTS = clock - 5000

var grpWorkerA, grpWorkerB, grpAdmin, grpMember = id(60), id(61), id(62), id(63)

var grpDefaultLimits = proto.Limits{
	Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3, ReviewRounds: 5,
	AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
}

func grpOwnerID(nonce string) string {
	h := sha256.Sum256([]byte("SoloDawn group owner " + nonce))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

func grpCreatePayload(nonce, g string) proto.GroupCreate {
	return proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: proto.ID(nonce), Timestamp: grpCreateTS, Group: proto.ID(g)},
			Op:          "create",
		},
		Name: "Website", Kind: "project", Repo: "https://git.example/team/website.git",
		Base: "main", Forge: "github", Checks: []string{"go vet ./...", "go test ./..."}, MemberName: "owner",
	}
}

func grpApply(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, payload any) home.Result {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return grpApplyRaw(t, s, d, r, by, string(raw))
}

func grpApplyRaw(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, payload string) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, by.ID(), by.Sign(payload), now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func grpNoNulls(t *testing.T, row any, allowed ...string) {
	t.Helper()
	equal(t, proto.UnexpectedNulls(briefRaw(t, row), allowed...), []string{})
}

func grpState(t *testing.T) *home.State {
	t.Helper()
	s := empty(t)
	res := grpApply(t, s, disk{}, receipts{}, browser, grpCreatePayload(groupID, groupID))
	s.Commit(res)
	sec := ai(aiID, laptop)
	sec.Group, sec.Kind, sec.Name, sec.CLI, sec.HostName = groupID, "ai", "Claude", "claude", "a device"
	sec.Secretary, sec.Account, sec.Allow, sec.Approvers = true, "personal", []string{}, []string{browser.ID()}
	sec.State, sec.Since, sec.Path, sec.LastSeen = "dormant", clock, "direct", clock
	s.AIs[aiID] = sec
	s.Group.Secretary = aiID
	for i, member := range []string{grpWorkerA, grpWorkerB} {
		w := sec
		w.ID, w.Name, w.Secretary = member, []string{"Codex", "Helper"}[i], false
		s.AIs[member] = w
	}
	s.Logs[device(6).ID()] = logOf(t, device(6))
	s.People[grpAdmin] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: grpAdmin}, Group: groupID, Kind: "human", User: guest.ID(), Name: "admin-user"},
		Role:      "admin", Online: true,
	}
	s.People[grpMember] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: grpMember}, Group: groupID, Kind: "human", User: device(6).ID(), Name: "guest-user"},
		Role:      "member", Online: true,
	}
	s.Seq = 10
	return s
}

func grpSetCmd(n byte) proto.GroupSet {
	return proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "set",
		},
	}
}

func TestGroupCreateWritesTheGroupAndOwner(t *testing.T) {
	for _, forge := range []string{"github", "none"} {
		t.Run(forge, func(t *testing.T) {
			s := empty(t)
			r := receipts{}
			p := grpCreatePayload(groupID, groupID)
			p.Forge = forge
			if forge == "none" {
				p.Checks = []string{}
			}
			res := grpApply(t, s, disk{}, r, browser, p)
			wantGroup := proto.GroupRow{
				RowHeader: proto.RowHeader{Type: "group", ID: groupID, Seq: 1}, Name: "Website", Kind: "project",
				Repo: p.Repo, Base: "main", Forge: forge, Home: node.ID(), Checks: p.Checks,
				Pass: 90, PassFoundation: 70, Limits: grpDefaultLimits, Created: clock,
			}
			wantOwner := proto.HumanMemberRow{
				MemberRow: proto.MemberRow{
					RowHeader: proto.RowHeader{Type: "member", ID: grpOwnerID(groupID), Seq: 2},
					Group:     groupID, Kind: "human", User: browser.ID(), Name: "owner", LastSeen: clock,
				},
				Role: "owner", Online: true,
			}
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, res.GroupsChanged, true)
			rows := briefRows(res)
			equal(t, rows, []any{wantGroup, wantOwner})
			grpNoNulls(t, rows[0], "/paused")
			grpNoNulls(t, rows[1])
			s.Commit(res)
			equal(t, s.Group, wantGroup)
			equal(t, s.People[wantOwner.ID], wantOwner)
			// Repeating the same create answers from the receipt without new rows.
			again := grpApply(t, s, disk{}, r, browser, p)
			equal(t, again, home.Result{Answer: store.Outcome{OK: true}})
		})
	}
}

func TestGroupCreateRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		existing bool
		by       identity.Key
		raw      string
		change   func(*proto.GroupCreate)
		code     string
	}{
		"by an AI on the owner's host": {
			existing: true, by: laptop,
			change: func(p *proto.GroupCreate) { p.As = proto.ID(aiID) },
			code:   "forbidden",
		},
		"by a member who is not the owner": {existing: true, by: guest, code: "forbidden"},
		"by a non-owner before creation":   {by: guest, code: "forbidden"},
		"with checks of the wrong type": {
			raw:  `{"t":"group","n":"` + groupID + `","ts":1763600000000,"g":"` + groupID + `","op":"create","checks":"bad"}`,
			code: "bad_request",
		},
		"where g differs from n": {
			change: func(p *proto.GroupCreate) { p.Nonce = proto.ID(id(1)) },
			code:   "bad_request",
		},
		"with kind team": {
			change: func(p *proto.GroupCreate) { p.Kind = "team" },
			code:   "bad_request",
		},
		"with forge gitlab": {
			change: func(p *proto.GroupCreate) { p.Forge = "gitlab" },
			code:   "bad_request",
		},
		"with an empty name": {
			change: func(p *proto.GroupCreate) { p.Name = "" },
			code:   "bad_request",
		},
		"with an empty repo": {
			change: func(p *proto.GroupCreate) { p.Repo = "" },
			code:   "bad_request",
		},
		"with an empty base": {
			change: func(p *proto.GroupCreate) { p.Base = "" },
			code:   "bad_request",
		},
		"with an empty member_name": {
			change: func(p *proto.GroupCreate) { p.MemberName = "" },
			code:   "bad_request",
		},
		"when the group already exists": {existing: true, by: laptop, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := empty(t)
			if c.existing {
				s = grpState(t)
			}
			by := browser
			if c.by != nil {
				by = c.by
			}
			if c.raw != "" {
				briefRefused(t, grpApplyRaw(t, s, disk{}, receipts{}, by, c.raw), c.code)
				return
			}
			p := grpCreatePayload(groupID, groupID)
			if c.change != nil {
				c.change(&p)
			}
			briefRefused(t, grpApply(t, s, disk{}, receipts{}, by, p), c.code)
		})
	}
}

func TestGroupSetEachFieldIndividually(t *testing.T) {
	customLimits := proto.Limits{
		Rounds: 4, WakesPerHour: 12, TurnMinutes: 30, Failures: 2, ReviewRounds: 3,
		AskDepth: 2, AskWaitS: 20, ApprovalWaitS: 120, IdleNoticeMinutes: 15,
	}
	for name, c := range map[string]struct {
		by            identity.Key
		set           func(*proto.GroupSet)
		want          func(*proto.GroupRow)
		groupsChanged bool
		extraRows     int
	}{
		"new name by owner": {
			by:            browser,
			set:           func(p *proto.GroupSet) { v := "Docs"; p.Name = &v },
			want:          func(g *proto.GroupRow) { g.Name = "Docs" },
			groupsChanged: true,
		},
		"same name leaves groups unchanged": {
			by:   browser,
			set:  func(p *proto.GroupSet) { v := "Website"; p.Name = &v },
			want: func(*proto.GroupRow) {},
		},
		"empty checks list by admin": {
			by:   guest,
			set:  func(p *proto.GroupSet) { v := []string{}; p.Checks = &v },
			want: func(g *proto.GroupRow) { g.Checks = []string{} },
		},
		"pass score": {
			by:   browser,
			set:  func(p *proto.GroupSet) { v := 85; p.Pass = &v },
			want: func(g *proto.GroupRow) { g.Pass = 85 },
		},
		"foundation pass score": {
			by:   browser,
			set:  func(p *proto.GroupSet) { v := 65; p.PassFoundation = &v },
			want: func(g *proto.GroupRow) { g.PassFoundation = 65 },
		},
		"limits": {
			by:   browser,
			set:  func(p *proto.GroupSet) { v := customLimits; p.Limits = &v },
			want: func(g *proto.GroupRow) { g.Limits = customLimits },
		},
		"secretary": {
			by:        guest,
			set:       func(p *proto.GroupSet) { p.Secretary = proto.ID(grpWorkerA) },
			want:      func(g *proto.GroupRow) { g.Secretary = grpWorkerA },
			extraRows: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := grpState(t)
			wantGroup := s.Group
			wantGroup.Seq = 11
			c.want(&wantGroup)
			cmd := grpSetCmd(70)
			c.set(&cmd)
			res := grpApply(t, s, disk{}, receipts{}, c.by, cmd)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, res.GroupsChanged, c.groupsChanged)
			rows := briefRows(res)
			equal(t, len(rows), 1+c.extraRows)
			equal(t, rows[0], wantGroup)
			grpNoNulls(t, rows[0], "/paused")
			for _, extra := range rows[1:] {
				grpNoNulls(t, extra)
			}
		})
	}
}

func TestGroupSetSecretaryTransitions(t *testing.T) {
	t.Run("first secretary when the group had none", func(t *testing.T) {
		s := grpState(t)
		sec := s.AIs[aiID]
		sec.Secretary = false
		s.AIs[aiID] = sec
		s.Group.Secretary = ""
		// Finished tasks on this AI, and active tasks on other AIs, do not block becoming secretary.
		s.Tasks[id(80)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(80)}, State: "merged", Assignee: grpWorkerA}
		s.Tasks[id(81)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(81)}, State: "cancelled", Reviewer: grpWorkerA}
		s.Tasks[id(86)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(86)}, State: "active", Assignee: grpWorkerB, Reviewer: aiID}

		cmd := grpSetCmd(71)
		cmd.Secretary = proto.ID(grpWorkerA)
		res := grpApply(t, s, disk{}, receipts{}, browser, cmd)
		wantGroup := s.Group
		wantGroup.Seq, wantGroup.Secretary = 11, grpWorkerA
		wantSec := s.AIs[grpWorkerA]
		wantSec.Seq, wantSec.Secretary = 12, true
		equal(t, briefRows(res), []any{wantGroup, wantSec})
	})

	t.Run("replacing an existing secretary clears the old flag", func(t *testing.T) {
		s := grpState(t)
		cmd := grpSetCmd(72)
		cmd.Secretary = proto.ID(grpWorkerA)
		res := grpApply(t, s, disk{}, receipts{}, browser, cmd)
		wantGroup := s.Group
		wantGroup.Seq, wantGroup.Secretary = 11, grpWorkerA
		oldSec := s.AIs[aiID]
		oldSec.Seq, oldSec.Secretary = 12, false
		newSec := s.AIs[grpWorkerA]
		newSec.Seq, newSec.Secretary = 13, true
		equal(t, briefRows(res), []any{wantGroup, oldSec, newSec})
		s.Commit(res)
		equal(t, s.AIs[aiID].Secretary, false)
		equal(t, s.AIs[grpWorkerA].Secretary, true)
		equal(t, s.Group.Secretary, grpWorkerA)
	})

	t.Run("naming the current secretary again writes only the group row", func(t *testing.T) {
		s := grpState(t)
		cmd := grpSetCmd(73)
		cmd.Secretary = proto.ID(aiID)
		res := grpApply(t, s, disk{}, receipts{}, browser, cmd)
		wantGroup := s.Group
		wantGroup.Seq = 11
		equal(t, briefRows(res), []any{wantGroup})
	})
}

func TestGroupSetRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		by    identity.Key
		raw   string
		state func(*home.State)
		set   func(*proto.GroupSet)
		code  string
	}{
		"by a non-admin member": {
			by:   device(6),
			set:  func(p *proto.GroupSet) { v := 80; p.Pass = &v },
			code: "forbidden",
		},
		"by the secretary": {
			by:   laptop,
			set:  func(p *proto.GroupSet) { p.As = proto.ID(aiID); v := 80; p.Pass = &v },
			code: "forbidden",
		},
		"with pass of the wrong type": {
			raw:  `{"t":"group","n":"` + id(74) + `","ts":1763600000000,"g":"` + groupID + `","op":"set","pass":"bad"}`,
			code: "bad_request",
		},
		"with no optional fields": {set: func(*proto.GroupSet) {}, code: "bad_request"},
		"with an empty name": {
			set:  func(p *proto.GroupSet) { v := ""; p.Name = &v },
			code: "bad_request",
		},
		"secretary is not a member": {
			set:  func(p *proto.GroupSet) { p.Secretary = proto.ID(id(99)) },
			code: "conflict",
		},
		"secretary is a human member": {
			set:  func(p *proto.GroupSet) { p.Secretary = proto.ID(grpAdmin) },
			code: "conflict",
		},
		"secretary is assignee of a queued task": {
			state: func(s *home.State) { s.Tasks[id(82)] = proto.TaskRow{State: "queued", Assignee: grpWorkerA} },
			set:   func(p *proto.GroupSet) { p.Secretary = proto.ID(grpWorkerA) },
			code:  "conflict",
		},
		"secretary is assignee of an active task": {
			state: func(s *home.State) { s.Tasks[id(83)] = proto.TaskRow{State: "active", Assignee: grpWorkerA} },
			set:   func(p *proto.GroupSet) { p.Secretary = proto.ID(grpWorkerA) },
			code:  "conflict",
		},
		"secretary is reviewer of a paused task": {
			state: func(s *home.State) { s.Tasks[id(84)] = proto.TaskRow{State: "paused", Reviewer: grpWorkerA} },
			set:   func(p *proto.GroupSet) { p.Secretary = proto.ID(grpWorkerA) },
			code:  "conflict",
		},
		"secretary is reviewer of a failed task": {
			state: func(s *home.State) { s.Tasks[id(85)] = proto.TaskRow{State: "failed", Reviewer: grpWorkerA} },
			set:   func(p *proto.GroupSet) { p.Secretary = proto.ID(grpWorkerA) },
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
				briefRefused(t, grpApplyRaw(t, s, disk{}, receipts{}, by, c.raw), c.code)
				return
			}
			cmd := grpSetCmd(75)
			c.set(&cmd)
			briefRefused(t, grpApply(t, s, disk{}, receipts{}, by, cmd), c.code)
		})
	}
}
